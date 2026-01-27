package secrets

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// InitialDelay is the initial delay before the first retry.
	InitialDelay time.Duration `json:"initial_delay,omitempty"`

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration `json:"max_delay,omitempty"`

	// Multiplier is the exponential backoff multiplier.
	Multiplier float64 `json:"multiplier,omitempty"`

	// Jitter adds randomness to delays (0-1 as percentage of delay).
	Jitter float64 `json:"jitter,omitempty"`

	// RetryableErrors is a list of errors that should be retried.
	RetryableErrors []error `json:"-"`

	// NonRetryableErrors is a list of errors that should not be retried.
	NonRetryableErrors []error `json:"-"`

	// ShouldRetry is a custom function to determine if an error should be retried.
	ShouldRetry func(err error) bool `json:"-"`
}

// DefaultRetryConfig returns a default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		NonRetryableErrors: []error{
			ErrSecretNotFound,
			ErrAccessDenied,
			ErrInvalidPath,
			ErrLeaseNotFound,
			ErrLeaseNotRenewable,
		},
	}
}

// VaultRetryConfig returns retry configuration optimized for Vault.
func VaultRetryConfig() *RetryConfig {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 5
	cfg.InitialDelay = 200 * time.Millisecond
	cfg.NonRetryableErrors = append(cfg.NonRetryableErrors,
		errors.New("permission denied"),
		errors.New("invalid token"),
	)
	return cfg
}

// AWSRetryConfig returns retry configuration optimized for AWS.
func AWSRetryConfig() *RetryConfig {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 100 * time.Millisecond
	cfg.Jitter = 0.2 // AWS recommends jitter for distributed systems
	return cfg
}

// AzureRetryConfig returns retry configuration optimized for Azure.
func AzureRetryConfig() *RetryConfig {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 250 * time.Millisecond
	return cfg
}

// GCPRetryConfig returns retry configuration optimized for GCP.
func GCPRetryConfig() *RetryConfig {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 100 * time.Millisecond
	cfg.Jitter = 0.1
	return cfg
}

// Retryer provides retry functionality with exponential backoff.
type Retryer struct {
	config *RetryConfig
	rng    *rand.Rand
}

// NewRetryer creates a new retryer.
func NewRetryer(config *RetryConfig) *Retryer {
	if config == nil {
		config = DefaultRetryConfig()
	}

	return &Retryer{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute executes the operation with retries.
func (r *Retryer) Execute(ctx context.Context, op func() error) error {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Execute the operation
		err := op()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !r.shouldRetry(err) {
			return err
		}

		// Don't sleep after the last attempt
		if attempt == r.config.MaxAttempts-1 {
			break
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Wait with context awareness
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// ExecuteWithResult executes the operation with retries and returns a result.
func (r *Retryer) ExecuteWithResult(ctx context.Context, op func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	var result interface{}

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Execute the operation
		res, err := op()
		if err == nil {
			return res, nil
		}

		lastErr = err
		result = res

		// Check if we should retry
		if !r.shouldRetry(err) {
			return result, err
		}

		// Don't sleep after the last attempt
		if attempt == r.config.MaxAttempts-1 {
			break
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Wait with context awareness
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return result, lastErr
}

// shouldRetry determines if the error should be retried.
func (r *Retryer) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check custom retry function first
	if r.config.ShouldRetry != nil {
		return r.config.ShouldRetry(err)
	}

	// Check non-retryable errors
	for _, nonRetryable := range r.config.NonRetryableErrors {
		if errors.Is(err, nonRetryable) {
			return false
		}
	}

	// Check retryable errors (if specified)
	if len(r.config.RetryableErrors) > 0 {
		for _, retryable := range r.config.RetryableErrors {
			if errors.Is(err, retryable) {
				return true
			}
		}
		return false
	}

	// Default to retryable
	return true
}

// calculateDelay calculates the delay for a given attempt.
func (r *Retryer) calculateDelay(attempt int) time.Duration {
	// Calculate base delay with exponential backoff
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt))

	// Apply max delay cap
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Apply jitter
	if r.config.Jitter > 0 {
		jitterRange := delay * r.config.Jitter
		jitter := (r.rng.Float64() * 2 * jitterRange) - jitterRange
		delay += jitter
	}

	// Ensure non-negative
	if delay < 0 {
		delay = 0
	}

	return time.Duration(delay)
}

// BackendRetryPolicy provides retry policies for different backends.
type BackendRetryPolicy struct {
	// policies maps backend types to their retry configurations.
	policies map[BackendType]*RetryConfig
}

// NewBackendRetryPolicy creates a new backend retry policy.
func NewBackendRetryPolicy() *BackendRetryPolicy {
	return &BackendRetryPolicy{
		policies: map[BackendType]*RetryConfig{
			BackendTypeVault: VaultRetryConfig(),
			BackendTypeAWS:   AWSRetryConfig(),
			BackendTypeAzure: AzureRetryConfig(),
			BackendTypeGCP:   GCPRetryConfig(),
		},
	}
}

// GetPolicy returns the retry policy for a backend type.
func (brp *BackendRetryPolicy) GetPolicy(backendType BackendType) *RetryConfig {
	if config, ok := brp.policies[backendType]; ok {
		return config
	}
	return DefaultRetryConfig()
}

// SetPolicy sets the retry policy for a backend type.
func (brp *BackendRetryPolicy) SetPolicy(backendType BackendType, config *RetryConfig) {
	brp.policies[backendType] = config
}

// GetRetryer returns a retryer for a backend type.
func (brp *BackendRetryPolicy) GetRetryer(backendType BackendType) *Retryer {
	return NewRetryer(brp.GetPolicy(backendType))
}

// RetryableBackend wraps a backend with automatic retry support.
type RetryableBackend struct {
	backend SecretBackend
	retryer *Retryer
}

// NewRetryableBackend creates a new retryable backend wrapper.
func NewRetryableBackend(backend SecretBackend, config *RetryConfig) *RetryableBackend {
	if config == nil {
		// Use backend-specific default config
		switch backend.Type() {
		case BackendTypeVault:
			config = VaultRetryConfig()
		case BackendTypeAWS:
			config = AWSRetryConfig()
		case BackendTypeAzure:
			config = AzureRetryConfig()
		case BackendTypeGCP:
			config = GCPRetryConfig()
		default:
			config = DefaultRetryConfig()
		}
	}

	return &RetryableBackend{
		backend: backend,
		retryer: NewRetryer(config),
	}
}

// Type returns the backend type.
func (rb *RetryableBackend) Type() BackendType {
	return rb.backend.Type()
}

// Name returns the backend name.
func (rb *RetryableBackend) Name() string {
	return rb.backend.Name()
}

// Healthy checks if the backend is healthy.
func (rb *RetryableBackend) Healthy(ctx context.Context) bool {
	return rb.backend.Healthy(ctx)
}

// Read reads a secret with automatic retries.
func (rb *RetryableBackend) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	result, err := rb.retryer.ExecuteWithResult(ctx, func() (interface{}, error) {
		return rb.backend.Read(ctx, req)
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Secret), nil
}

// ReadDynamic reads a dynamic secret with automatic retries.
func (rb *RetryableBackend) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	result, err := rb.retryer.ExecuteWithResult(ctx, func() (interface{}, error) {
		return rb.backend.ReadDynamic(ctx, req)
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Secret), nil
}

// List lists secrets with automatic retries.
func (rb *RetryableBackend) List(ctx context.Context, prefix string) ([]string, error) {
	result, err := rb.retryer.ExecuteWithResult(ctx, func() (interface{}, error) {
		return rb.backend.List(ctx, prefix)
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.([]string), nil
}

// RenewLease renews a lease with automatic retries.
func (rb *RetryableBackend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	result, err := rb.retryer.ExecuteWithResult(ctx, func() (interface{}, error) {
		return rb.backend.RenewLease(ctx, leaseID, increment)
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Lease), nil
}

// RevokeLease revokes a lease with automatic retries.
func (rb *RetryableBackend) RevokeLease(ctx context.Context, leaseID string) error {
	return rb.retryer.Execute(ctx, func() error {
		return rb.backend.RevokeLease(ctx, leaseID)
	})
}

// Close closes the backend.
func (rb *RetryableBackend) Close() error {
	return rb.backend.Close()
}

// Ensure RetryableBackend implements SecretBackend.
var _ SecretBackend = (*RetryableBackend)(nil)

// RetryStats contains retry statistics.
type RetryStats struct {
	// TotalAttempts is the total number of attempts.
	TotalAttempts int64 `json:"total_attempts"`

	// SuccessfulAttempts is the number of successful first attempts.
	SuccessfulAttempts int64 `json:"successful_attempts"`

	// RetriedAttempts is the number of attempts that required retries.
	RetriedAttempts int64 `json:"retried_attempts"`

	// FailedAttempts is the number of attempts that failed after all retries.
	FailedAttempts int64 `json:"failed_attempts"`

	// TotalRetries is the total number of retry attempts.
	TotalRetries int64 `json:"total_retries"`

	// TotalDelayTime is the total time spent waiting for retries.
	TotalDelayTime time.Duration `json:"total_delay_time"`
}
