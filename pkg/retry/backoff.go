// Package retry provides retry utilities with configurable backoff strategies.
// NOTE: API/ABI is not finalized and may change without notice.
// It is designed for handling transient failures in distributed systems,
// particularly for command execution timeouts and network failures.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter does not require crypto randomness
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// Strategy defines a backoff strategy
type Strategy interface {
	// NextBackoff returns the duration to wait before the next retry.
	// Returns 0 if no more retries should be attempted.
	NextBackoff(attempt int) time.Duration
	// Reset resets the strategy to its initial state
	Reset()
	// Clone creates a copy of the strategy for concurrent use
	Clone() Strategy
}

// ExponentialBackoff implements exponential backoff with optional jitter
type ExponentialBackoff struct {
	// InitialInterval is the first retry interval
	InitialInterval time.Duration
	// MaxInterval is the maximum interval cap
	MaxInterval time.Duration
	// Multiplier is the factor by which the interval increases
	Multiplier float64
	// MaxRetries is the maximum number of retries (0 = unlimited)
	MaxRetries int
	// Jitter adds randomization to prevent thundering herd
	Jitter bool
	// JitterFactor is the maximum jitter as a fraction of the interval (0.0-1.0)
	JitterFactor float64

	mu      sync.Mutex
	attempt int
}

// DefaultExponentialBackoff returns a default exponential backoff configuration
func DefaultExponentialBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      5,
		Jitter:          true,
		JitterFactor:    0.2,
	}
}

// NewExponentialBackoff creates a new exponential backoff with custom settings
func NewExponentialBackoff(initial, maxVal time.Duration, multiplier float64, maxRetries int) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialInterval: initial,
		MaxInterval:     maxVal,
		Multiplier:      multiplier,
		MaxRetries:      maxRetries,
		Jitter:          true,
		JitterFactor:    0.2,
	}
}

// NextBackoff calculates the next backoff interval
func (eb *ExponentialBackoff) NextBackoff(attempt int) time.Duration {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Check max retries
	if eb.MaxRetries > 0 && attempt >= eb.MaxRetries {
		return 0
	}

	// Calculate base interval: initial * multiplier^attempt
	interval := float64(eb.InitialInterval) * math.Pow(eb.Multiplier, float64(attempt))

	// Cap at maxVal interval
	if time.Duration(interval) > eb.MaxInterval {
		interval = float64(eb.MaxInterval)
	}

	// Add jitter if enabled
	if eb.Jitter && eb.JitterFactor > 0 {
		jitter := interval * eb.JitterFactor
		// Random jitter between -jitter and +jitter
		//nolint:gosec // G404: math/rand used for backoff jitter timing, not security
		interval += (rand.Float64()*2-1) * jitter // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter does not require crypto randomness
	}

	return time.Duration(interval)
}

// Reset resets the attempt counter
func (eb *ExponentialBackoff) Reset() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.attempt = 0
}

// Clone creates a copy of the strategy
func (eb *ExponentialBackoff) Clone() Strategy {
	return &ExponentialBackoff{
		InitialInterval: eb.InitialInterval,
		MaxInterval:     eb.MaxInterval,
		Multiplier:      eb.Multiplier,
		MaxRetries:      eb.MaxRetries,
		Jitter:          eb.Jitter,
		JitterFactor:    eb.JitterFactor,
	}
}

// LinearBackoff implements linear backoff
type LinearBackoff struct {
	// InitialInterval is the first retry interval
	InitialInterval time.Duration
	// Increment is added to each subsequent interval
	Increment time.Duration
	// MaxInterval is the maximum interval cap
	MaxInterval time.Duration
	// MaxRetries is the maximum number of retries (0 = unlimited)
	MaxRetries int
}

// DefaultLinearBackoff returns a default linear backoff configuration
func DefaultLinearBackoff() *LinearBackoff {
	return &LinearBackoff{
		InitialInterval: 1 * time.Second,
		Increment:       1 * time.Second,
		MaxInterval:     30 * time.Second,
		MaxRetries:      5,
	}
}

// NextBackoff calculates the next backoff interval
func (lb *LinearBackoff) NextBackoff(attempt int) time.Duration {
	if lb.MaxRetries > 0 && attempt >= lb.MaxRetries {
		return 0
	}

	interval := lb.InitialInterval + time.Duration(attempt)*lb.Increment
	if interval > lb.MaxInterval {
		interval = lb.MaxInterval
	}
	return interval
}

// Reset resets the strategy (no-op for stateless strategies)
func (lb *LinearBackoff) Reset() {}

// Clone creates a copy of the strategy
func (lb *LinearBackoff) Clone() Strategy {
	return &LinearBackoff{
		InitialInterval: lb.InitialInterval,
		Increment:       lb.Increment,
		MaxInterval:     lb.MaxInterval,
		MaxRetries:      lb.MaxRetries,
	}
}

// ConstantBackoff implements constant interval backoff
type ConstantBackoff struct {
	Interval   time.Duration
	MaxRetries int
}

// NextBackoff returns the constant interval
func (cb *ConstantBackoff) NextBackoff(attempt int) time.Duration {
	if cb.MaxRetries > 0 && attempt >= cb.MaxRetries {
		return 0
	}
	return cb.Interval
}

// Reset resets the strategy (no-op)
func (cb *ConstantBackoff) Reset() {}

// Clone creates a copy of the strategy
func (cb *ConstantBackoff) Clone() Strategy {
	return &ConstantBackoff{
		Interval:   cb.Interval,
		MaxRetries: cb.MaxRetries,
	}
}

// RetryableError wraps an error and indicates whether it should be retried
type RetryableError struct {
	Err       error
	Retryable bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a new retryable error
func NewRetryableError(err error, retryable bool) *RetryableError {
	return &RetryableError{Err: err, Retryable: retryable}
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return retryableErr.Retryable
	}
	// Default: transient errors are often retryable
	return true
}

// Config configures retry behavior
type Config struct {
	Strategy Strategy
	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, err error, nextBackoff time.Duration)
	// IsRetryable determines if an error should be retried
	IsRetryable func(error) bool
}

// DefaultConfig returns a default retry configuration
func DefaultConfig() *Config {
	return &Config{
		Strategy:    DefaultExponentialBackoff(),
		IsRetryable: IsRetryable,
	}
}

// Retrier executes operations with retry logic
type Retrier struct {
	config *Config
}

// NewRetrier creates a new retrier
func NewRetrier(config *Config) *Retrier {
	if config == nil {
		config = DefaultConfig()
	}
	if config.IsRetryable == nil {
		config.IsRetryable = IsRetryable
	}
	return &Retrier{config: config}
}

// Result holds the result of a retry operation
type Result struct {
	Attempts int
	LastErr  error
	Success  bool
}

// Do executes the function with retries
func (r *Retrier) Do(ctx context.Context, fn func(ctx context.Context) error) Result {
	result := Result{}

	for {
		result.Attempts++

		err := fn(ctx)
		if err == nil {
			result.Success = true
			return result
		}

		result.LastErr = err

		// Check if error is retryable
		if !r.config.IsRetryable(err) {
			return result
		}

		// Calculate next backoff
		backoff := r.config.Strategy.NextBackoff(result.Attempts - 1)
		if backoff == 0 {
			// No more retries
			return result
		}

		// Call retry callback if configured
		if r.config.OnRetry != nil {
			r.config.OnRetry(result.Attempts, err, backoff)
		}

		// Wait for backoff or context cancellation
		if err := wait.ForContext(ctx, backoff); err != nil {
			result.LastErr = err
			return result
		}
	}
}

// DoWithValue executes a function that returns a value with retries
func DoWithValue[T any](ctx context.Context, r *Retrier, fn func(ctx context.Context) (T, error)) (T, Result) {
	var value T
	result := Result{}

	for {
		result.Attempts++

		v, err := fn(ctx)
		if err == nil {
			result.Success = true
			return v, result
		}

		result.LastErr = err
		value = v

		// Check if error is retryable
		if !r.config.IsRetryable(err) {
			return value, result
		}

		// Calculate next backoff
		backoff := r.config.Strategy.NextBackoff(result.Attempts - 1)
		if backoff == 0 {
			return value, result
		}

		// Call retry callback if configured
		if r.config.OnRetry != nil {
			r.config.OnRetry(result.Attempts, err, backoff)
		}

		// Wait for backoff or context cancellation
		if err := wait.ForContext(ctx, backoff); err != nil {
			result.LastErr = err
			return value, result
		}
	}
}

// CommandRetryConfig is specialized configuration for command execution retries
type CommandRetryConfig struct {
	// RetryOnTimeout enables retry for timed-out commands
	RetryOnTimeout bool
	// RetryOnFailure enables retry for failed commands (non-zero exit code)
	RetryOnFailure bool
	// RetryOnNetworkError enables retry for network errors
	RetryOnNetworkError bool
	// Strategy is the backoff strategy
	Strategy Strategy
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// RetryableExitCodes are exit codes that should be retried
	RetryableExitCodes []int32
}

// DefaultCommandRetryConfig returns default command retry configuration
func DefaultCommandRetryConfig() *CommandRetryConfig {
	return &CommandRetryConfig{
		RetryOnTimeout:      true,
		RetryOnFailure:      false, // Don't retry failed commands by default
		RetryOnNetworkError: true,
		Strategy:            DefaultExponentialBackoff(),
		MaxRetries:          3,
		RetryableExitCodes:  []int32{}, // Empty by default
	}
}

// ShouldRetryCommand determines if a command should be retried
func (c *CommandRetryConfig) ShouldRetryCommand(err error, exitCode int32, isTimeout bool) bool {
	if isTimeout && c.RetryOnTimeout {
		return true
	}

	if err != nil && c.RetryOnNetworkError {
		// Check for network-related errors
		errStr := err.Error()
		networkErrors := []string{
			"connection refused",
			"connection reset",
			"timeout",
			"no route to host",
			"network unreachable",
		}
		for _, ne := range networkErrors {
			if contains(errStr, ne) {
				return true
			}
		}
	}

	if c.RetryOnFailure && exitCode != 0 {
		// Check if exit code is in retryable list
		if len(c.RetryableExitCodes) == 0 {
			return true // Retry all non-zero exit codes
		}
		for _, code := range c.RetryableExitCodes {
			if exitCode == code {
				return true
			}
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Stats tracks retry statistics
type Stats struct {
	mu              sync.RWMutex
	TotalAttempts   int64
	TotalRetries    int64
	TotalSuccesses  int64
	TotalFailures   int64
	RetriesByReason map[string]int64
}

// NewStats creates a new retry stats tracker
func NewStats() *Stats {
	return &Stats{
		RetriesByReason: make(map[string]int64),
	}
}

// RecordAttempt records an attempt
func (s *Stats) RecordAttempt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalAttempts++
}

// RecordRetry records a retry
func (s *Stats) RecordRetry(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalRetries++
	s.RetriesByReason[reason]++
}

// RecordSuccess records a success
func (s *Stats) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalSuccesses++
}

// RecordFailure records a final failure
func (s *Stats) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalFailures++
}

// Snapshot returns a copy of the current stats
func (s *Stats) Snapshot() *Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := &Stats{
		TotalAttempts:   s.TotalAttempts,
		TotalRetries:    s.TotalRetries,
		TotalSuccesses:  s.TotalSuccesses,
		TotalFailures:   s.TotalFailures,
		RetriesByReason: make(map[string]int64),
	}
	for k, v := range s.RetriesByReason {
		stats.RetriesByReason[k] = v
	}
	return stats
}

// String returns a string representation of the stats
func (s *Stats) String() string {
	snap := s.Snapshot()
	return fmt.Sprintf("RetryStats{attempts=%d, retries=%d, successes=%d, failures=%d}",
		snap.TotalAttempts, snap.TotalRetries, snap.TotalSuccesses, snap.TotalFailures)
}
