package azure

import (
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.AuthMethod != AuthMethodDefault {
		t.Errorf("expected AuthMethod to be %s, got %s", AuthMethodDefault, cfg.AuthMethod)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout to be 30s, got %v", cfg.Timeout)
	}
	if cfg.Cloud != "public" {
		t.Errorf("expected Cloud to be 'public', got %s", cfg.Cloud)
	}
	if cfg.RetryOptions == nil {
		t.Error("expected RetryOptions to be set")
	} else {
		if cfg.RetryOptions.MaxRetries != 3 {
			t.Errorf("expected MaxRetries to be 3, got %d", cfg.RetryOptions.MaxRetries)
		}
	}
}

func TestDefaultBackendConfig(t *testing.T) {
	cfg := DefaultBackendConfig()

	if cfg.Name != "azure" {
		t.Errorf("expected Name to be 'azure', got %s", cfg.Name)
	}
	if cfg.PathPrefix != "azure/" {
		t.Errorf("expected PathPrefix to be 'azure/', got %s", cfg.PathPrefix)
	}
	if cfg.DefaultCacheTTL != 5*time.Minute {
		t.Errorf("expected DefaultCacheTTL to be 5m, got %v", cfg.DefaultCacheTTL)
	}
	if !cfg.JSONKeys {
		t.Error("expected JSONKeys to be true")
	}
	if cfg.ClientConfig == nil {
		t.Error("expected ClientConfig to be set")
	}
}

func TestAuthMethodValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *ClientConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "service principal missing tenant_id",
			config: &ClientConfig{
				VaultURL:   "https://test.vault.azure.net",
				AuthMethod: AuthMethodServicePrincipal,
				ClientID:   "client-id",
				ClientSecret: "secret",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "service principal missing client_id",
			config: &ClientConfig{
				VaultURL:   "https://test.vault.azure.net",
				AuthMethod: AuthMethodServicePrincipal,
				TenantID:   "tenant-id",
				ClientSecret: "secret",
			},
			wantErr:     true,
			errContains: "client_id is required",
		},
		{
			name: "service principal missing client_secret",
			config: &ClientConfig{
				VaultURL:   "https://test.vault.azure.net",
				AuthMethod: AuthMethodServicePrincipal,
				TenantID:   "tenant-id",
				ClientID:   "client-id",
			},
			wantErr:     true,
			errContains: "client_secret is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createCredential(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *ClientConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "config is required",
		},
		{
			name: "missing vault_url",
			config: &ClientConfig{
				AuthMethod: AuthMethodDefault,
			},
			wantErr:     true,
			errContains: "vault_url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(t.Context(), tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSecretValueMethods(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "GetMap success",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Value: `{"key1":"value1","key2":"value2"}`,
				}
				m, err := sv.GetMap()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if m["key1"] != "value1" {
					t.Errorf("expected key1=value1, got %v", m["key1"])
				}
			},
		},
		{
			name: "GetMap invalid JSON",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Value: "not json",
				}
				_, err := sv.GetMap()
				if err == nil {
					t.Error("expected error for invalid JSON")
				}
			},
		},
		{
			name: "GetJSON success",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Value: `{"username":"admin","password":"secret"}`,
				}
				var creds struct {
					Username string `json:"username"`
					Password string `json:"password"`
				}
				err := sv.GetJSON(&creds)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if creds.Username != "admin" {
					t.Errorf("expected username=admin, got %s", creds.Username)
				}
				if creds.Password != "secret" {
					t.Errorf("expected password=secret, got %s", creds.Password)
				}
			},
		},
		{
			name: "IsExpired with nil attributes",
			fn: func(t *testing.T) {
				sv := &SecretValue{}
				if sv.IsExpired() {
					t.Error("expected IsExpired to be false with nil attributes")
				}
			},
		},
		{
			name: "IsExpired with zero time",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{},
				}
				if sv.IsExpired() {
					t.Error("expected IsExpired to be false with zero expiry")
				}
			},
		},
		{
			name: "IsExpired with past time",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{
						Expires: time.Now().Add(-time.Hour),
					},
				}
				if !sv.IsExpired() {
					t.Error("expected IsExpired to be true for past expiry")
				}
			},
		},
		{
			name: "IsExpired with future time",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{
						Expires: time.Now().Add(time.Hour),
					},
				}
				if sv.IsExpired() {
					t.Error("expected IsExpired to be false for future expiry")
				}
			},
		},
		{
			name: "IsActive with nil attributes",
			fn: func(t *testing.T) {
				sv := &SecretValue{}
				if !sv.IsActive() {
					t.Error("expected IsActive to be true with nil attributes")
				}
			},
		},
		{
			name: "IsActive when disabled",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{
						Enabled: false,
					},
				}
				if sv.IsActive() {
					t.Error("expected IsActive to be false when disabled")
				}
			},
		},
		{
			name: "IsActive when not yet valid",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{
						Enabled:   true,
						NotBefore: time.Now().Add(time.Hour),
					},
				}
				if sv.IsActive() {
					t.Error("expected IsActive to be false when not yet valid")
				}
			},
		},
		{
			name: "IsActive when fully valid",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Attributes: &SecretAttributes{
						Enabled:   true,
						NotBefore: time.Now().Add(-time.Hour),
						Expires:   time.Now().Add(time.Hour),
					},
				}
				if !sv.IsActive() {
					t.Error("expected IsActive to be true when fully valid")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestExtractNameFromID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string
	}{
		{
			name:     "full URL with version",
			id:       "https://myvault.vault.azure.net/secrets/mysecret/abc123def456",
			expected: "mysecret",
		},
		{
			name:     "full URL without version",
			id:       "https://myvault.vault.azure.net/secrets/mysecret",
			expected: "mysecret",
		},
		{
			name:     "key URL with version",
			id:       "https://myvault.vault.azure.net/keys/mykey/version123",
			expected: "mykey",
		},
		{
			name:     "certificate URL",
			id:       "https://myvault.vault.azure.net/certificates/mycert/v1",
			expected: "mycert",
		},
		{
			name:     "short path with type",
			id:       "secrets/mysecret",
			expected: "mysecret",
		},
		{
			name:     "empty string",
			id:       "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNameFromID(tt.id)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractVersionFromID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string
	}{
		{
			name:     "full URL with version",
			id:       "https://myvault.vault.azure.net/secrets/mysecret/abc123def456",
			expected: "abc123def456",
		},
		{
			name:     "full URL without version",
			id:       "https://myvault.vault.azure.net/secrets/mysecret",
			expected: "",
		},
		{
			name:     "key URL with version",
			id:       "https://myvault.vault.azure.net/keys/mykey/version123",
			expected: "version123",
		},
		{
			name:     "empty string",
			id:       "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromID(tt.id)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTranslateError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name: "not found error",
			err: &azcore.ResponseError{
				StatusCode: http.StatusNotFound,
			},
			expected: secrets.ErrSecretNotFound,
		},
		{
			name: "forbidden error",
			err: &azcore.ResponseError{
				StatusCode: http.StatusForbidden,
			},
			expected: secrets.ErrAccessDenied,
		},
		{
			name: "unauthorized error",
			err: &azcore.ResponseError{
				StatusCode: http.StatusUnauthorized,
			},
			expected: secrets.ErrAccessDenied,
		},
		{
			name: "service unavailable error",
			err: &azcore.ResponseError{
				StatusCode: http.StatusServiceUnavailable,
			},
			expected: secrets.ErrBackendUnavailable,
		},
		{
			name: "gateway timeout error",
			err: &azcore.ResponseError{
				StatusCode: http.StatusGatewayTimeout,
			},
			expected: secrets.ErrBackendUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translateError(tt.err)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil error, got %v", result)
				}
				return
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBackendType(t *testing.T) {
	b := &Backend{
		name: "test-azure",
	}

	if b.Type() != secrets.BackendTypeAzure {
		t.Errorf("expected BackendTypeAzure, got %s", b.Type())
	}

	if b.Name() != "test-azure" {
		t.Errorf("expected 'test-azure', got %s", b.Name())
	}
}

func TestResolveSecretName(t *testing.T) {
	tests := []struct {
		name       string
		pathPrefix string
		path       string
		expected   string
	}{
		{
			name:       "with prefix",
			pathPrefix: "azure/",
			path:       "azure/mysecret",
			expected:   "mysecret",
		},
		{
			name:       "without prefix",
			pathPrefix: "azure/",
			path:       "other/mysecret",
			expected:   "other/mysecret",
		},
		{
			name:       "empty prefix",
			pathPrefix: "",
			path:       "mysecret",
			expected:   "mysecret",
		},
		{
			name:       "trailing slashes only",
			pathPrefix: "azure/",
			path:       "azure/mysecret/",
			expected:   "mysecret",
		},
		{
			name:       "nested path",
			pathPrefix: "azure/",
			path:       "azure/app/config/db",
			expected:   "app/config/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{
				config: &BackendConfig{
					PathPrefix: tt.pathPrefix,
				},
			}
			result := b.resolveSecretName(tt.path)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValueToSecret(t *testing.T) {
	tests := []struct {
		name     string
		jsonKeys bool
		value    *SecretValue
		check    func(t *testing.T, s *secrets.Secret)
	}{
		{
			name:     "JSON secret",
			jsonKeys: true,
			value: &SecretValue{
				ID:          "https://vault.azure.net/secrets/mysecret/v1",
				Name:        "mysecret",
				Version:     "v1",
				Value:       `{"username":"admin","password":"secret"}`,
				ContentType: "application/json",
				Attributes: &SecretAttributes{
					Enabled: true,
					Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Backend != secrets.BackendTypeAzure {
					t.Errorf("expected BackendTypeAzure, got %s", s.Backend)
				}
				if s.Data["username"] != "admin" {
					t.Errorf("expected username=admin, got %v", s.Data["username"])
				}
				if s.Data["password"] != "secret" {
					t.Errorf("expected password=secret, got %v", s.Data["password"])
				}
				if s.Metadata["version"] != "v1" {
					t.Errorf("expected version=v1, got %s", s.Metadata["version"])
				}
			},
		},
		{
			name:     "non-JSON secret",
			jsonKeys: true,
			value: &SecretValue{
				ID:      "https://vault.azure.net/secrets/apikey/v1",
				Name:    "apikey",
				Version: "v1",
				Value:   "my-api-key-12345",
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Data["value"] != "my-api-key-12345" {
					t.Errorf("expected value=my-api-key-12345, got %v", s.Data["value"])
				}
			},
		},
		{
			name:     "JSON disabled",
			jsonKeys: false,
			value: &SecretValue{
				ID:    "https://vault.azure.net/secrets/data/v1",
				Value: `{"key":"value"}`,
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Data["value"] != `{"key":"value"}` {
					t.Errorf("expected raw JSON, got %v", s.Data["value"])
				}
			},
		},
		{
			name:     "with tags",
			jsonKeys: true,
			value: &SecretValue{
				ID:    "https://vault.azure.net/secrets/tagged/v1",
				Value: "secret-value",
				Tags: map[string]string{
					"environment": "production",
					"app":         "myapp",
				},
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Metadata["tag:environment"] != "production" {
					t.Errorf("expected tag:environment=production, got %s", s.Metadata["tag:environment"])
				}
				if s.Metadata["tag:app"] != "myapp" {
					t.Errorf("expected tag:app=myapp, got %s", s.Metadata["tag:app"])
				}
			},
		},
		{
			name:     "with expiry",
			jsonKeys: true,
			value: &SecretValue{
				ID:    "https://vault.azure.net/secrets/expiring/v1",
				Value: "temp-secret",
				Attributes: &SecretAttributes{
					Enabled: true,
					Expires: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				},
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.ExpiresAt.IsZero() {
					t.Error("expected ExpiresAt to be set")
				}
				expectedExpiry := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
				if !s.ExpiresAt.Equal(expectedExpiry) {
					t.Errorf("expected ExpiresAt=%v, got %v", expectedExpiry, s.ExpiresAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{
				config: &BackendConfig{
					JSONKeys: tt.jsonKeys,
				},
			}
			result := b.valueToSecret("test/path", tt.value)
			tt.check(t, result)
		})
	}
}

func TestMultiTenantConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *MultiTenantConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "tenant config is required",
		},
		{
			name: "missing tenant_id",
			config: &MultiTenantConfig{
				TenantID: "",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseConfig := &BackendConfig{
				ClientConfig: &ClientConfig{
					VaultURL: "https://test.vault.azure.net",
				},
			}
			_, err := NewMultiTenantBackend(t.Context(), tt.config, baseConfig)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrivateLinkConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *PrivateLinkConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "private link config is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseConfig := &BackendConfig{
				ClientConfig: &ClientConfig{
					VaultURL: "https://test.vault.azure.net",
				},
			}
			_, err := NewPrivateLinkBackend(t.Context(), tt.config, baseConfig)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildClientOptions(t *testing.T) {
	cfg := &ClientConfig{
		Timeout: 60 * time.Second,
		RetryOptions: &RetryOptions{
			MaxRetries:    5,
			RetryDelay:    2 * time.Second,
			MaxRetryDelay: 1 * time.Minute,
		},
	}

	opts := buildClientOptions(cfg)

	if opts.Transport == nil {
		t.Error("expected Transport to be set")
	}
	if opts.Retry.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", opts.Retry.MaxRetries)
	}
	if opts.Retry.RetryDelay != 2*time.Second {
		t.Errorf("expected RetryDelay=2s, got %v", opts.Retry.RetryDelay)
	}
	if opts.Retry.MaxRetryDelay != 1*time.Minute {
		t.Errorf("expected MaxRetryDelay=1m, got %v", opts.Retry.MaxRetryDelay)
	}
}

func TestBackendClose(t *testing.T) {
	// Test closing a backend with nil client
	b := &Backend{}
	err := b.Close()
	if err != nil {
		t.Errorf("unexpected error closing backend with nil client: %v", err)
	}
}

func TestLeaseOperationsNotSupported(t *testing.T) {
	b := &Backend{}

	// Test RenewLease returns not found
	lease, err := b.RenewLease(t.Context(), "test-lease", time.Hour)
	if lease != nil {
		t.Error("expected nil lease")
	}
	if err != secrets.ErrLeaseNotFound {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}

	// Test RevokeLease returns not found
	err = b.RevokeLease(t.Context(), "test-lease")
	if err != secrets.ErrLeaseNotFound {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
