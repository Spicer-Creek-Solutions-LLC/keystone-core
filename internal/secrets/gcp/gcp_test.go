package gcp

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "https://www.googleapis.com/auth/cloud-platform" {
		t.Errorf("expected default scopes, got %v", cfg.Scopes)
	}
}

func TestDefaultBackendConfig(t *testing.T) {
	cfg := DefaultBackendConfig()

	if cfg.Name != "gcp" {
		t.Errorf("expected Name to be 'gcp', got %s", cfg.Name)
	}
	if cfg.PathPrefix != "gcp/" {
		t.Errorf("expected PathPrefix to be 'gcp/', got %s", cfg.PathPrefix)
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

func TestBuildClientOptionsValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *ClientConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "service account missing key file",
			config: &ClientConfig{
				ProjectID:  "test-project",
				AuthMethod: AuthMethodServiceAccount,
			},
			wantErr:     true,
			errContains: "service_account_key_file",
		},
		{
			name: "impersonation missing service account",
			config: &ClientConfig{
				ProjectID:  "test-project",
				AuthMethod: AuthMethodImpersonation,
			},
			wantErr:     true,
			errContains: "impersonate_service_account is required",
		},
		{
			name: "unsupported auth method",
			config: &ClientConfig{
				ProjectID:  "test-project",
				AuthMethod: AuthMethod("invalid"),
			},
			wantErr:     true,
			errContains: "unsupported auth method",
		},
		{
			name: "valid default auth",
			config: &ClientConfig{
				ProjectID:  "test-project",
				AuthMethod: AuthMethodDefault,
			},
			wantErr: false,
		},
		{
			name: "valid workload identity",
			config: &ClientConfig{
				ProjectID:  "test-project",
				AuthMethod: AuthMethodWorkloadIdentity,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildClientOptions(tt.config)
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
			name: "missing project_id",
			config: &ClientConfig{
				AuthMethod: AuthMethodDefault,
			},
			wantErr:     true,
			errContains: "project_id is required",
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
			name: "GetString",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Data:       []byte("my-secret-value"),
					DataString: "my-secret-value",
				}
				if sv.GetString() != "my-secret-value" {
					t.Errorf("expected 'my-secret-value', got %q", sv.GetString())
				}
			},
		},
		{
			name: "GetString from Data",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Data: []byte("my-secret-value"),
				}
				if sv.GetString() != "my-secret-value" {
					t.Errorf("expected 'my-secret-value', got %q", sv.GetString())
				}
			},
		},
		{
			name: "GetMap success",
			fn: func(t *testing.T) {
				sv := &SecretValue{
					Data: []byte(`{"key1":"value1","key2":"value2"}`),
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
					Data: []byte("not json"),
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
					Data: []byte(`{"username":"admin","password":"secret"}`),
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
			name: "IsEnabled",
			fn: func(t *testing.T) {
				sv := &SecretValue{State: SecretVersionStateEnabled}
				if !sv.IsEnabled() {
					t.Error("expected IsEnabled to be true")
				}
				if sv.IsDisabled() {
					t.Error("expected IsDisabled to be false")
				}
				if sv.IsDestroyed() {
					t.Error("expected IsDestroyed to be false")
				}
			},
		},
		{
			name: "IsDisabled",
			fn: func(t *testing.T) {
				sv := &SecretValue{State: SecretVersionStateDisabled}
				if sv.IsEnabled() {
					t.Error("expected IsEnabled to be false")
				}
				if !sv.IsDisabled() {
					t.Error("expected IsDisabled to be true")
				}
				if sv.IsDestroyed() {
					t.Error("expected IsDestroyed to be false")
				}
			},
		},
		{
			name: "IsDestroyed",
			fn: func(t *testing.T) {
				sv := &SecretValue{State: SecretVersionStateDestroyed}
				if sv.IsEnabled() {
					t.Error("expected IsEnabled to be false")
				}
				if sv.IsDisabled() {
					t.Error("expected IsDisabled to be false")
				}
				if !sv.IsDestroyed() {
					t.Error("expected IsDestroyed to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestExtractVersionFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full resource name with version",
			input:    "projects/my-project/secrets/my-secret/versions/5",
			expected: "5",
		},
		{
			name:     "full resource name with latest",
			input:    "projects/my-project/secrets/my-secret/versions/latest",
			expected: "latest",
		},
		{
			name:     "no version",
			input:    "projects/my-project/secrets/my-secret",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractSecretShortName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full resource name",
			input:    "projects/my-project/secrets/my-secret",
			expected: "my-secret",
		},
		{
			name:     "short name only",
			input:    "my-secret",
			expected: "my-secret",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSecretShortName(tt.input)
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
			name:     "not found error",
			err:      status.Error(codes.NotFound, "secret not found"),
			expected: secrets.ErrSecretNotFound,
		},
		{
			name:     "permission denied error",
			err:      status.Error(codes.PermissionDenied, "permission denied"),
			expected: secrets.ErrAccessDenied,
		},
		{
			name:     "unauthenticated error",
			err:      status.Error(codes.Unauthenticated, "not authenticated"),
			expected: secrets.ErrAccessDenied,
		},
		{
			name:     "unavailable error",
			err:      status.Error(codes.Unavailable, "service unavailable"),
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
			if !errors.Is(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBackendType(t *testing.T) {
	b := &Backend{
		name: "test-gcp",
	}

	if b.Type() != secrets.BackendTypeGCP {
		t.Errorf("expected BackendTypeGCP, got %s", b.Type())
	}

	if b.Name() != "test-gcp" {
		t.Errorf("expected 'test-gcp', got %s", b.Name())
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
			pathPrefix: "gcp/",
			path:       "gcp/mysecret",
			expected:   "mysecret",
		},
		{
			name:       "without prefix",
			pathPrefix: "gcp/",
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
			name:       "trailing slashes",
			pathPrefix: "gcp/",
			path:       "gcp/mysecret/",
			expected:   "mysecret",
		},
		{
			name:       "nested path",
			pathPrefix: "gcp/",
			path:       "gcp/app/config/db",
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
				Name:       "projects/test/secrets/mysecret/versions/1",
				SecretName: "mysecret",
				Version:    "1",
				Data:       []byte(`{"username":"admin","password":"secret"}`),
				State:      SecretVersionStateEnabled,
				CreateTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Backend != secrets.BackendTypeGCP {
					t.Errorf("expected BackendTypeGCP, got %s", s.Backend)
				}
				if s.Data["username"] != "admin" {
					t.Errorf("expected username=admin, got %v", s.Data["username"])
				}
				if s.Data["password"] != "secret" {
					t.Errorf("expected password=secret, got %v", s.Data["password"])
				}
				if s.Metadata["version"] != "1" {
					t.Errorf("expected version=1, got %s", s.Metadata["version"])
				}
				if s.Metadata["state"] != string(SecretVersionStateEnabled) {
					t.Errorf("expected state=ENABLED, got %s", s.Metadata["state"])
				}
			},
		},
		{
			name:     "non-JSON secret",
			jsonKeys: true,
			value: &SecretValue{
				Name:       "projects/test/secrets/apikey/versions/1",
				SecretName: "apikey",
				Version:    "1",
				Data:       []byte("my-api-key-12345"),
				State:      SecretVersionStateEnabled,
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
				Name:  "projects/test/secrets/data/versions/1",
				Data:  []byte(`{"key":"value"}`),
				State: SecretVersionStateEnabled,
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Data["value"] != `{"key":"value"}` {
					t.Errorf("expected raw JSON, got %v", s.Data["value"])
				}
			},
		},
		{
			name:     "with etag",
			jsonKeys: true,
			value: &SecretValue{
				Name:  "projects/test/secrets/tagged/versions/1",
				Data:  []byte("secret-value"),
				State: SecretVersionStateEnabled,
				Etag:  "abc123",
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Metadata["etag"] != "abc123" {
					t.Errorf("expected etag=abc123, got %s", s.Metadata["etag"])
				}
			},
		},
		{
			name:     "disabled version",
			jsonKeys: true,
			value: &SecretValue{
				Name:  "projects/test/secrets/disabled/versions/1",
				Data:  []byte("secret-value"),
				State: SecretVersionStateDisabled,
			},
			check: func(t *testing.T, s *secrets.Secret) {
				if s.Metadata["state"] != string(SecretVersionStateDisabled) {
					t.Errorf("expected state=DISABLED, got %s", s.Metadata["state"])
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

func TestCrossProjectConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *CrossProjectConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "cross project config is required",
		},
		{
			name: "missing project_id",
			config: &CrossProjectConfig{
				ProjectID: "",
			},
			wantErr:     true,
			errContains: "project_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseConfig := &BackendConfig{
				ClientConfig: &ClientConfig{
					ProjectID: "base-project",
				},
			}
			_, err := NewCrossProjectBackend(t.Context(), tt.config, baseConfig)
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

func TestVPCServiceControlsConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *VPCServiceControlsConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "vpc service controls config is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseConfig := &BackendConfig{
				ClientConfig: &ClientConfig{
					ProjectID: "test-project",
				},
			}
			_, err := NewVPCServiceControlsBackend(t.Context(), tt.config, baseConfig)
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
	if !errors.Is(err, secrets.ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}

	// Test RevokeLease returns not found
	err = b.RevokeLease(t.Context(), "test-lease")
	if !errors.Is(err, secrets.ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestVersionNumber(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected int64
		wantErr  bool
	}{
		{
			name:     "latest",
			version:  "latest",
			expected: -1,
			wantErr:  false,
		},
		{
			name:     "numeric version",
			version:  "5",
			expected: 5,
			wantErr:  false,
		},
		{
			name:     "large version",
			version:  "1234567890",
			expected: 1234567890,
			wantErr:  false,
		},
		{
			name:    "invalid version",
			version: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := VersionNumber(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %d, got %d", tt.expected, result)
				}
			}
		})
	}
}

func TestRotationConfig(t *testing.T) {
	cfg := &RotationConfig{
		NextRotationTime: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		RotationPeriod:   24 * 30 * time.Hour, // 30 days
	}

	if cfg.NextRotationTime.IsZero() {
		t.Error("expected NextRotationTime to be set")
	}
	if cfg.RotationPeriod != 24*30*time.Hour {
		t.Errorf("expected RotationPeriod to be 30 days, got %v", cfg.RotationPeriod)
	}
}

func TestCMEKConfig(t *testing.T) {
	cfg := &CMEKConfig{
		KMSKeyName: "projects/my-project/locations/us-central1/keyRings/my-keyring/cryptoKeys/my-key",
	}

	if cfg.KMSKeyName == "" {
		t.Error("expected KMSKeyName to be set")
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
