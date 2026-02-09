package secrets

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// MockBackend is a mock implementation of SecretBackend for testing.
type MockBackend struct {
	name        string
	backendType BackendType
	healthy     bool
	secrets     map[string]*Secret

	readCount    atomic.Int64
	readFailures atomic.Int64
	failNextN    atomic.Int32
	delay        time.Duration
	err          error
}

func NewMockBackend(name string, backendType BackendType) *MockBackend {
	return &MockBackend{
		name:        name,
		backendType: backendType,
		healthy:     true,
		secrets:     make(map[string]*Secret),
	}
}

func (m *MockBackend) Type() BackendType                { return m.backendType }
func (m *MockBackend) Name() string                     { return m.name }
func (m *MockBackend) Healthy(ctx context.Context) bool { return m.healthy }

func (m *MockBackend) SetHealthy(healthy bool)  { m.healthy = healthy }
func (m *MockBackend) SetDelay(d time.Duration) { m.delay = d }
func (m *MockBackend) SetError(err error)       { m.err = err }
func (m *MockBackend) FailNextN(n int)          { m.failNextN.Store(int32(n)) }

func (m *MockBackend) AddSecret(path string, data map[string]interface{}) {
	m.secrets[path] = &Secret{
		Path:    path,
		Backend: m.backendType,
		Type:    SecretTypeStatic,
		Data:    data,
	}
}

func (m *MockBackend) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	m.readCount.Add(1)

	// Check for forced failures
	if m.failNextN.Load() > 0 {
		m.failNextN.Add(-1)
		m.readFailures.Add(1)
		return nil, ErrBackendUnavailable
	}

	// Apply delay
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}

	// Check for error
	if m.err != nil {
		m.readFailures.Add(1)
		return nil, m.err
	}

	// Look up secret
	secret, ok := m.secrets[req.Path]
	if !ok {
		return nil, ErrSecretNotFound
	}

	return secret, nil
}

func (m *MockBackend) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	return m.Read(ctx, req)
}

func (m *MockBackend) List(ctx context.Context, prefix string) ([]string, error) {
	names := make([]string, 0, len(m.secrets))
	for path := range m.secrets {
		names = append(names, path)
	}
	return names, nil
}

func (m *MockBackend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	return nil, ErrLeaseNotFound
}

func (m *MockBackend) RevokeLease(ctx context.Context, leaseID string) error {
	return ErrLeaseNotFound
}

func (m *MockBackend) Close() error { return nil }

// Ensure MockBackend implements SecretBackend.
var _ SecretBackend = (*MockBackend)(nil)

func TestCircuitBreaker(t *testing.T) {
	t.Run("starts closed", func(t *testing.T) {
		cb := NewCircuitBreaker(nil)
		if cb.State() != CircuitStateClosed {
			t.Errorf("expected closed state, got %v", cb.State())
		}
	})

	t.Run("opens after failures", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			FailureThreshold:    3,
			SuccessThreshold:    2,
			OpenDuration:        100 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}
		cb := NewCircuitBreaker(config)

		// Record failures
		for i := 0; i < 3; i++ {
			cb.RecordFailure()
		}

		if cb.State() != CircuitStateOpen {
			t.Errorf("expected open state, got %v", cb.State())
		}

		if cb.AllowRequest() {
			t.Error("expected request to be blocked")
		}
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			FailureThreshold:    2,
			SuccessThreshold:    1,
			OpenDuration:        50 * time.Millisecond,
			HalfOpenMaxRequests: 1,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()

		if cb.State() != CircuitStateOpen {
			t.Fatalf("expected open state, got %v", cb.State())
		}

		// Wait for timeout
		time.Sleep(60 * time.Millisecond)

		// Should transition to half-open
		if !cb.AllowRequest() {
			t.Error("expected request to be allowed in half-open state")
		}

		if cb.State() != CircuitStateHalfOpen {
			t.Errorf("expected half-open state, got %v", cb.State())
		}
	})

	t.Run("closes after success in half-open", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			FailureThreshold:    2,
			SuccessThreshold:    1,
			OpenDuration:        10 * time.Millisecond,
			HalfOpenMaxRequests: 2,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)
		cb.AllowRequest() // Transition to half-open

		// Record success
		cb.RecordSuccess()

		if cb.State() != CircuitStateClosed {
			t.Errorf("expected closed state, got %v", cb.State())
		}
	})

	t.Run("reopens after failure in half-open", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			FailureThreshold:    2,
			SuccessThreshold:    1,
			OpenDuration:        10 * time.Millisecond,
			HalfOpenMaxRequests: 2,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)
		cb.AllowRequest() // Transition to half-open

		// Record failure
		cb.RecordFailure()

		if cb.State() != CircuitStateOpen {
			t.Errorf("expected open state, got %v", cb.State())
		}
	})
}

func TestHealthMonitor(t *testing.T) {
	t.Run("registers and tracks backends", func(t *testing.T) {
		hm := NewHealthMonitor(&HealthMonitorConfig{
			CheckInterval:      10 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		})

		backend := NewMockBackend("test", BackendTypeVault)
		hm.Register("test", backend)

		health, ok := hm.GetHealth("test")
		if !ok {
			t.Fatal("expected backend to be registered")
		}

		if health.State != HealthStateUnknown {
			t.Errorf("expected unknown state, got %v", health.State)
		}
	})

	t.Run("detects healthy backends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hm := NewHealthMonitor(&HealthMonitorConfig{
			CheckInterval:      10 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		})

		backend := NewMockBackend("test", BackendTypeVault)
		backend.SetHealthy(true)
		hm.Register("test", backend)

		if err := hm.Start(ctx); err != nil {
			t.Fatal(err)
		}
		defer hm.Stop()

		// Wait for health check
		time.Sleep(30 * time.Millisecond)

		if !hm.IsHealthy("test") {
			t.Error("expected backend to be healthy")
		}
	})

	t.Run("detects unhealthy backends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hm := NewHealthMonitor(&HealthMonitorConfig{
			CheckInterval:      10 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		})

		backend := NewMockBackend("test", BackendTypeVault)
		backend.SetHealthy(false)
		hm.Register("test", backend)

		if err := hm.Start(ctx); err != nil {
			t.Fatal(err)
		}
		defer hm.Stop()

		// Wait for health check
		time.Sleep(30 * time.Millisecond)

		if hm.IsHealthy("test") {
			t.Error("expected backend to be unhealthy")
		}
	})
}

func TestBackendGroup(t *testing.T) {
	t.Run("selects primary backend", func(t *testing.T) {
		hm := NewHealthMonitor(nil)
		bg := NewBackendGroup("test", hm, nil)

		primary := NewMockBackend("primary", BackendTypeVault)
		secondary := NewMockBackend("secondary", BackendTypeAWS)

		bg.AddBackend("primary", primary, true)
		bg.AddBackend("secondary", secondary, false)

		backend, name, err := bg.GetActiveBackend()
		if err != nil {
			t.Fatal(err)
		}

		if name != "primary" {
			t.Errorf("expected primary, got %s", name)
		}

		if backend.Name() != "primary" {
			t.Errorf("expected primary backend, got %s", backend.Name())
		}
	})

	t.Run("failover to secondary", func(t *testing.T) {
		ctx := context.Background()
		hm := NewHealthMonitor(&HealthMonitorConfig{
			CheckInterval:      100 * time.Millisecond,
			Timeout:            100 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		})

		bg := NewBackendGroup("test", hm, &FailoverPolicy{
			Enabled:       true,
			MaxAttempts:   3,
			PreferHealthy: true,
		})

		primary := NewMockBackend("primary", BackendTypeVault)
		primary.SetHealthy(false)

		secondary := NewMockBackend("secondary", BackendTypeAWS)
		secondary.SetHealthy(true)

		bg.AddBackend("primary", primary, true)
		bg.AddBackend("secondary", secondary, false)

		backend, name, err := bg.SelectBackend(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if name != "secondary" {
			t.Errorf("expected secondary after failover, got %s", name)
		}

		if backend.Name() != "secondary" {
			t.Errorf("expected secondary backend, got %s", backend.Name())
		}
	})

	t.Run("execute with failover", func(t *testing.T) {
		ctx := context.Background()
		hm := NewHealthMonitor(nil)
		bg := NewBackendGroup("test", hm, &FailoverPolicy{
			Enabled:       true,
			MaxAttempts:   3,
			PreferHealthy: true,
			RetryDelay:    10 * time.Millisecond,
		})

		primary := NewMockBackend("primary", BackendTypeVault)
		primary.SetError(ErrBackendUnavailable)

		secondary := NewMockBackend("secondary", BackendTypeAWS)
		secondary.AddSecret("test/secret", map[string]interface{}{"key": "value"})

		bg.AddBackend("primary", primary, true)
		bg.AddBackend("secondary", secondary, false)

		var result *Secret
		err := bg.ExecuteWithFailover(ctx, func(backend SecretBackend) error {
			secret, err := backend.Read(ctx, &SecretRequest{Path: "test/secret"})
			if err != nil {
				return err
			}
			result = secret
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

		if result == nil {
			t.Fatal("expected result")
		}

		if result.Data["key"] != "value" {
			t.Errorf("expected value, got %v", result.Data["key"])
		}
	})
}

func TestRetryer(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		r := NewRetryer(&RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
		})

		attempts := 0
		err := r.Execute(context.Background(), func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("retries on failure", func(t *testing.T) {
		r := NewRetryer(&RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2,
		})

		attempts := 0
		err := r.Execute(context.Background(), func() error {
			attempts++
			if attempts < 3 {
				return ErrBackendUnavailable
			}
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("fails after max attempts", func(t *testing.T) {
		r := NewRetryer(&RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
		})

		attempts := 0
		err := r.Execute(context.Background(), func() error {
			attempts++
			return ErrBackendUnavailable
		})

		if err == nil {
			t.Fatal("expected error")
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("does not retry non-retryable errors", func(t *testing.T) {
		r := NewRetryer(&RetryConfig{
			MaxAttempts:        3,
			InitialDelay:       10 * time.Millisecond,
			NonRetryableErrors: []error{ErrSecretNotFound},
		})

		attempts := 0
		err := r.Execute(context.Background(), func() error {
			attempts++
			return ErrSecretNotFound
		})

		if !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("expected ErrSecretNotFound, got %v", err)
		}

		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		r := NewRetryer(&RetryConfig{
			MaxAttempts:  10,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   1.0, // Constant delay
		})

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()

		attempts := 0
		err := r.Execute(ctx, func() error {
			attempts++
			return ErrBackendUnavailable
		})

		// With 50ms delays and 80ms timeout, should get ~2 attempts before timeout
		// Should get context deadline exceeded
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected deadline exceeded, got %v", err)
		}

		// Should have fewer than max attempts
		if attempts >= 10 {
			t.Errorf("expected context cancellation to stop retries early, got %d attempts", attempts)
		}
	})
}

func TestRetryableBackend(t *testing.T) {
	t.Run("retries read operations", func(t *testing.T) {
		mock := NewMockBackend("test", BackendTypeVault)
		mock.AddSecret("test/secret", map[string]interface{}{"key": "value"})
		mock.FailNextN(2) // Fail first 2 attempts

		rb := NewRetryableBackend(mock, &RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
		})

		secret, err := rb.Read(context.Background(), &SecretRequest{Path: "test/secret"})
		if err != nil {
			t.Fatal(err)
		}

		if secret.Data["key"] != "value" {
			t.Errorf("expected value, got %v", secret.Data["key"])
		}

		if mock.readCount.Load() != 3 {
			t.Errorf("expected 3 read attempts, got %d", mock.readCount.Load())
		}
	})
}

func TestErrorTranslator(t *testing.T) {
	tests := []struct {
		name         string
		backend      BackendType
		err          error
		expectedCode ErrorCode
		retryable    bool
	}{
		{
			name:         "vault permission denied",
			backend:      BackendTypeVault,
			err:          errors.New("permission denied"),
			expectedCode: ErrorCodeAccessDenied,
			retryable:    false,
		},
		{
			name:         "vault rate limit",
			backend:      BackendTypeVault,
			err:          errors.New("rate limit exceeded code=429"),
			expectedCode: ErrorCodeRateLimit,
			retryable:    true,
		},
		{
			name:         "aws not found",
			backend:      BackendTypeAWS,
			err:          errors.New("ResourceNotFoundException: secret not found"),
			expectedCode: ErrorCodeNotFound,
			retryable:    false,
		},
		{
			name:         "aws throttling",
			backend:      BackendTypeAWS,
			err:          errors.New("Throttling: rate exceeded"),
			expectedCode: ErrorCodeRateLimit,
			retryable:    true,
		},
		{
			name:         "azure forbidden",
			backend:      BackendTypeAzure,
			err:          errors.New("Forbidden: code=403"),
			expectedCode: ErrorCodeAccessDenied,
			retryable:    false,
		},
		{
			name:         "gcp not found",
			backend:      BackendTypeGCP,
			err:          errors.New("rpc error: code = NotFound desc = secret not found"),
			expectedCode: ErrorCodeNotFound,
			retryable:    false,
		},
		{
			name:         "gcp unavailable",
			backend:      BackendTypeGCP,
			err:          errors.New("rpc error: code = Unavailable desc = service temporarily unavailable"),
			expectedCode: ErrorCodeBackendUnavailable,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewErrorTranslator(tt.backend)
			secretErr := translator.Translate(tt.err, "test/path")

			if secretErr.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, secretErr.Code)
			}

			if secretErr.Retryable != tt.retryable {
				t.Errorf("expected retryable=%v, got %v", tt.retryable, secretErr.Retryable)
			}

			if secretErr.Backend != tt.backend {
				t.Errorf("expected backend %s, got %s", tt.backend, secretErr.Backend)
			}
		})
	}
}

func TestSecretErrorIs(t *testing.T) {
	tests := []struct {
		name     string
		err      *SecretError
		target   error
		expected bool
	}{
		{
			name:     "not found matches",
			err:      &SecretError{Code: ErrorCodeNotFound},
			target:   ErrSecretNotFound,
			expected: true,
		},
		{
			name:     "access denied matches",
			err:      &SecretError{Code: ErrorCodeAccessDenied},
			target:   ErrAccessDenied,
			expected: true,
		},
		{
			name:     "authorization matches access denied",
			err:      &SecretError{Code: ErrorCodeAuthorization},
			target:   ErrAccessDenied,
			expected: true,
		},
		{
			name:     "backend unavailable matches",
			err:      &SecretError{Code: ErrorCodeBackendUnavailable},
			target:   ErrBackendUnavailable,
			expected: true,
		},
		{
			name:     "different codes don't match",
			err:      &SecretError{Code: ErrorCodeNotFound},
			target:   ErrAccessDenied,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if errors.Is(tt.err, tt.target) != tt.expected {
				t.Errorf("errors.Is returned %v, expected %v", !tt.expected, tt.expected)
			}
		})
	}
}

func TestBackendFactory(t *testing.T) {
	t.Run("validates config", func(t *testing.T) {
		tests := []struct {
			name    string
			config  *BackendConfig
			wantErr bool
		}{
			{
				name: "valid vault config",
				config: &BackendConfig{
					Type:    "vault",
					Enabled: true,
					Vault: &VaultBackendConfig{
						Address: "https://vault.example.com:8200",
					},
				},
				wantErr: false,
			},
			{
				name: "missing vault address",
				config: &BackendConfig{
					Type:    "vault",
					Enabled: true,
					Vault:   &VaultBackendConfig{},
				},
				wantErr: true,
			},
			{
				name: "valid aws config",
				config: &BackendConfig{
					Type:    "aws_secrets_manager",
					Enabled: true,
					AWS: &AWSBackendConfig{
						Region: "us-west-2",
					},
				},
				wantErr: false,
			},
			{
				name: "missing aws region",
				config: &BackendConfig{
					Type:    "aws_secrets_manager",
					Enabled: true,
					AWS:     &AWSBackendConfig{},
				},
				wantErr: true,
			},
			{
				name: "valid azure config",
				config: &BackendConfig{
					Type:    "azure_keyvault",
					Enabled: true,
					Azure: &AzureBackendConfig{
						VaultURL: "https://myvault.vault.azure.net/",
					},
				},
				wantErr: false,
			},
			{
				name: "valid gcp config",
				config: &BackendConfig{
					Type:    "gcp_secret_manager",
					Enabled: true,
					GCP: &GCPBackendConfig{
						ProjectID: "my-project",
					},
				},
				wantErr: false,
			},
			{
				name: "unknown type",
				config: &BackendConfig{
					Type:    "unknown",
					Enabled: true,
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				configs := map[string]*BackendConfig{
					"test": tt.config,
				}
				err := ValidateConfig(&Config{Backends: configs})

				if (err != nil) != tt.wantErr {
					t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})
}

func TestMultiBackendRouting(t *testing.T) {
	t.Run("routes by prefix", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{
			DefaultBackend: "vault",
			Routing: []RoutingRule{
				{Prefix: "aws/", Backend: "aws"},
				{Prefix: "azure/", Backend: "azure"},
				{Prefix: "gcp/", Backend: "gcp"},
			},
		})

		vaultBackend := NewMockBackend("vault", BackendTypeVault)
		vaultBackend.AddSecret("kv/myapp/config", map[string]interface{}{"vault": "secret"})

		awsBackend := NewMockBackend("aws", BackendTypeAWS)
		awsBackend.AddSecret("myapp/db-creds", map[string]interface{}{"aws": "secret"})

		azureBackend := NewMockBackend("azure", BackendTypeAzure)
		azureBackend.AddSecret("myapp/api-key", map[string]interface{}{"azure": "secret"})

		gcpBackend := NewMockBackend("gcp", BackendTypeGCP)
		gcpBackend.AddSecret("myapp/service-account", map[string]interface{}{"gcp": "secret"})

		_ = broker.RegisterBackend("vault", vaultBackend)
		_ = broker.RegisterBackend("aws", awsBackend)
		_ = broker.RegisterBackend("azure", azureBackend)
		_ = broker.RegisterBackend("gcp", gcpBackend)

		tests := []struct {
			path            string
			expectedBackend string
			expectedKey     string
		}{
			{"vault/kv/myapp/config", "vault", "vault"},
			{"aws/myapp/db-creds", "aws", "aws"},
			{"azure/myapp/api-key", "azure", "azure"},
			{"gcp/myapp/service-account", "gcp", "gcp"},
			{"kv/myapp/config", "vault", "vault"}, // falls back to default
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				secret, err := broker.Read(context.Background(), &SecretRequest{Path: tt.path})
				if err != nil {
					t.Fatal(err)
				}

				if _, ok := secret.Data[tt.expectedKey]; !ok {
					t.Errorf("expected secret from %s backend", tt.expectedBackend)
				}
			})
		}
	})

	t.Run("handles backend failures", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{
			DefaultBackend: "vault",
		})

		vaultBackend := NewMockBackend("vault", BackendTypeVault)
		vaultBackend.SetHealthy(false)
		_ = broker.RegisterBackend("vault", vaultBackend)

		_, err := broker.Read(context.Background(), &SecretRequest{Path: "kv/myapp"})
		if !errors.Is(err, ErrBackendUnavailable) {
			t.Errorf("expected ErrBackendUnavailable, got %v", err)
		}
	})
}
