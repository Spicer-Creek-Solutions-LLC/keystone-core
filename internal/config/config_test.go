package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Test loading with no config file (should use defaults)
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	// Verify defaults
	if cfg.Server.ListenAddr != DefaultServerListenAddr {
		t.Errorf("Expected listen addr %s, got %s", DefaultServerListenAddr, cfg.Server.ListenAddr)
	}

	if cfg.Server.GRPCPort != DefaultGRPCPort {
		t.Errorf("Expected gRPC port %d, got %d", DefaultGRPCPort, cfg.Server.GRPCPort)
	}

	if cfg.Server.HTTPPort != DefaultHTTPPort {
		t.Errorf("Expected HTTP port %d, got %d", DefaultHTTPPort, cfg.Server.HTTPPort)
	}

	if cfg.NATS.Mode != DefaultNATSMode {
		t.Errorf("Expected NATS mode %s, got %s", DefaultNATSMode, cfg.NATS.Mode)
	}

	if cfg.Storage.Backend != DefaultStorageBackend {
		t.Errorf("Expected storage backend %s, got %s", DefaultStorageBackend, cfg.Storage.Backend)
	}

	if cfg.Agent.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Errorf("Expected heartbeat interval %v, got %v", DefaultHeartbeatInterval, cfg.Agent.HeartbeatInterval)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	// Create a temporary config file
	tmpFile, err := os.CreateTemp("", "kscore-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `
server:
  listenaddr: "127.0.0.1"
  grpcport: 9999
  httpport: 8888

nats:
  mode: external
  url: "nats://localhost:4222"

storage:
  backend: postgresql
  postgresql:
    dsn: "postgres://user:pass@localhost/db"

agent:
  heartbeatinterval: 60s
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify values from file
	if cfg.Server.ListenAddr != "127.0.0.1" {
		t.Errorf("Expected listen addr 127.0.0.1, got %s", cfg.Server.ListenAddr)
	}

	if cfg.Server.GRPCPort != 9999 {
		t.Errorf("Expected gRPC port 9999, got %d", cfg.Server.GRPCPort)
	}

	if cfg.NATS.Mode != NATSModeExternal {
		t.Errorf("Expected NATS mode external, got %s", cfg.NATS.Mode)
	}

	if cfg.Storage.Backend != StorageBackendPostgreSQL {
		t.Errorf("Expected storage backend postgresql, got %s", cfg.Storage.Backend)
	}

	if cfg.Agent.HeartbeatInterval != 60*time.Second {
		t.Errorf("Expected heartbeat interval 60s, got %v", cfg.Agent.HeartbeatInterval)
	}
}

func TestConfig_Validate_ValidEmbeddedMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid embedded config failed validation: %v", err)
	}
}

func TestConfig_Validate_ValidExternalMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeExternal,
			URL:  "nats://localhost:4222",
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid external config failed validation: %v", err)
	}
}

func TestConfig_Validate_ValidLeafMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeLeaf,
			Embedded: NATSEmbeddedConfig{
				LeafNodeURLs: []string{"nats://parent:7422"},
			},
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid leaf config failed validation: %v", err)
	}
}

func TestConfig_Validate_InvalidNATSMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: "invalid-mode",
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid NATS mode")
	}
}

func TestConfig_Validate_ExternalModeWithoutURL(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeExternal,
			URL:  "", // Missing URL
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for external mode without URL")
	}
}

func TestConfig_Validate_LeafModeWithoutParentURLs(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeLeaf,
			Embedded: NATSEmbeddedConfig{
				LeafNodeURLs: []string{}, // Empty
			},
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for leaf mode without parent URLs")
	}
}

func TestConfig_Validate_InvalidStorageBackend(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: "invalid-backend",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for invalid storage backend")
	}
}

func TestConfig_Validate_PostgreSQLWithoutDSN(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendPostgreSQL,
			PostgreSQL: PostgreSQLConfig{
				DSN: "", // Missing DSN
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for PostgreSQL without DSN")
	}
}

func TestConfig_Validate_InvalidPorts(t *testing.T) {
	testCases := []struct {
		name     string
		grpcPort int
		httpPort int
	}{
		{"gRPC port too low", 0, 8080},
		{"gRPC port too high", 70000, 8080},
		{"HTTP port too low", 9090, -1},
		{"HTTP port too high", 9090, 100000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					GRPCPort: tc.grpcPort,
					HTTPPort: tc.httpPort,
				},
				NATS: NATSConfig{
					Mode: NATSModeEmbedded,
				},
				Storage: StorageConfig{
					Backend: StorageBackendSQLite,
				},
			}

			if err := cfg.Validate(); err == nil {
				t.Errorf("Expected validation error for %s", tc.name)
			}
		})
	}
}

func TestNATSMode_String(t *testing.T) {
	if string(NATSModeEmbedded) != "embedded" {
		t.Errorf("Expected 'embedded', got %s", NATSModeEmbedded)
	}
	if string(NATSModeExternal) != "external" {
		t.Errorf("Expected 'external', got %s", NATSModeExternal)
	}
	if string(NATSModeLeaf) != "leaf" {
		t.Errorf("Expected 'leaf', got %s", NATSModeLeaf)
	}
}

func TestStorageBackend_String(t *testing.T) {
	if string(StorageBackendSQLite) != "sqlite" {
		t.Errorf("Expected 'sqlite', got %s", StorageBackendSQLite)
	}
	if string(StorageBackendPostgreSQL) != "postgresql" {
		t.Errorf("Expected 'postgresql', got %s", StorageBackendPostgreSQL)
	}
}

// IPv6 support tests (Epic 18)

func TestValidateAddressFamily(t *testing.T) {
	tests := []struct {
		name    string
		af      string
		field   string
		wantErr bool
	}{
		{"empty (uses default)", "", "server", false},
		{"prefer_ipv4", "prefer_ipv4", "server", false},
		{"prefer_ipv6", "prefer_ipv6", "nats.embedded", false},
		{"ipv4_only", "ipv4_only", "agent", false},
		{"ipv6_only", "ipv6_only", "server", false},
		{"invalid", "invalid", "server", true},
		{"typo", "prefer-ipv6", "server", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddressFamily(tt.af, tt.field)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAddressFamily(%q, %q) error = %v, wantErr %v", tt.af, tt.field, err, tt.wantErr)
			}
		})
	}
}

func TestServerConfig_GetAddressFamilyPreference(t *testing.T) {
	tests := []struct {
		name     string
		af       string
		expected string
	}{
		{"default (empty)", "", "prefer_ipv4"},
		{"prefer_ipv4", "prefer_ipv4", "prefer_ipv4"},
		{"prefer_ipv6", "prefer_ipv6", "prefer_ipv6"},
		{"ipv4_only", "ipv4_only", "ipv4_only"},
		{"ipv6_only", "ipv6_only", "ipv6_only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ServerConfig{AddressFamily: tt.af}
			pref := cfg.GetAddressFamilyPreference()
			// Just check it returns a valid preference (actual value tested in netutil)
			if pref < 0 || pref > 3 {
				t.Errorf("GetAddressFamilyPreference() returned invalid value: %v", pref)
			}
		})
	}
}

func TestServerConfig_GetEffectiveListenAddrs(t *testing.T) {
	tests := []struct {
		name          string
		listenAddr    string
		listenAddrs   []string
		addressFamily string
		expected      []string
	}{
		{
			name:       "single address",
			listenAddr: "0.0.0.0",
			expected:   []string{"0.0.0.0"},
		},
		{
			name:        "multiple addresses",
			listenAddrs: []string{"[::]:8080", "0.0.0.0:8080"},
			expected:    []string{"[::]:8080", "0.0.0.0:8080"},
		},
		{
			name:        "listenAddrs takes precedence",
			listenAddr:  "127.0.0.1",
			listenAddrs: []string{"[::]:8080"},
			expected:    []string{"[::]:8080"},
		},
		{
			name:          "default prefer_ipv4",
			addressFamily: "prefer_ipv4",
			expected:      []string{DefaultServerListenAddr},
		},
		{
			name:          "default ipv6_only",
			addressFamily: "ipv6_only",
			expected:      []string{DefaultServerListenAddr6},
		},
		{
			name:          "default prefer_ipv6",
			addressFamily: "prefer_ipv6",
			expected:      []string{DefaultServerListenAddr6, DefaultServerListenAddr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ServerConfig{
				ListenAddr:    tt.listenAddr,
				ListenAddrs:   tt.listenAddrs,
				AddressFamily: tt.addressFamily,
			}
			result := cfg.GetEffectiveListenAddrs()
			if len(result) != len(tt.expected) {
				t.Errorf("GetEffectiveListenAddrs() = %v, want %v", result, tt.expected)
				return
			}
			for i, addr := range result {
				if addr != tt.expected[i] {
					t.Errorf("GetEffectiveListenAddrs()[%d] = %q, want %q", i, addr, tt.expected[i])
				}
			}
		})
	}
}

func TestNATSEmbeddedConfig_GetEffectiveHost(t *testing.T) {
	tests := []struct {
		name          string
		host          string
		addressFamily string
		expected      string
	}{
		{"explicit host", "192.168.1.1", "", "192.168.1.1"},
		{"explicit IPv6 host", "::1", "", "::1"},
		{"default prefer_ipv4", "", "prefer_ipv4", DefaultNATSEmbeddedHost},
		{"default prefer_ipv6", "", "prefer_ipv6", DefaultNATSEmbeddedHost6},
		{"default ipv6_only", "", "ipv6_only", DefaultNATSEmbeddedHost6},
		{"default ipv4_only", "", "ipv4_only", DefaultNATSEmbeddedHost},
		{"default empty", "", "", DefaultNATSEmbeddedHost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NATSEmbeddedConfig{
				Host:          tt.host,
				AddressFamily: tt.addressFamily,
			}
			result := cfg.GetEffectiveHost()
			if result != tt.expected {
				t.Errorf("GetEffectiveHost() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConfig_Validate_InvalidAddressFamily(t *testing.T) {
	tests := []struct {
		name   string
		server string
		nats   string
		agent  string
	}{
		{"invalid server", "invalid", "", ""},
		{"invalid nats", "", "bad-value", ""},
		{"invalid agent", "", "", "wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					GRPCPort:      9090,
					HTTPPort:      8080,
					AddressFamily: tt.server,
				},
				NATS: NATSConfig{
					Mode: NATSModeEmbedded,
					Embedded: NATSEmbeddedConfig{
						AddressFamily: tt.nats,
					},
				},
				Storage: StorageConfig{
					Backend: StorageBackendSQLite,
				},
				Agent: AgentConfig{
					AddressFamily: tt.agent,
				},
			}

			if err := cfg.Validate(); err == nil {
				t.Error("Expected validation error for invalid address family")
			}
		})
	}
}

func TestConfig_Validate_ListenAddrs(t *testing.T) {
	// Valid addresses (TLS enabled to allow non-loopback - this test is about address format)
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort:    9090,
			HTTPPort:    8080,
			ListenAddrs: []string{"[::]:8080", "0.0.0.0:8080"},
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
		TLS: TLSConfig{
			Enabled: true, // Required for non-loopback addresses
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Unexpected validation error for valid listen addresses: %v", err)
	}

	// Invalid address - malformed IPv6 brackets
	cfg.Server.ListenAddrs = []string{"[::]:8080", "[::1:8080"}
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for malformed listen address")
	}
}

func TestConfig_Validate_AdvertiseAddrs(t *testing.T) {
	// Valid addresses
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
		Agent: AgentConfig{
			AdvertiseAddrs: []string{"192.168.1.100", "2001:db8::1"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Unexpected validation error for valid advertise addresses: %v", err)
	}

	// Invalid address - malformed IPv6
	cfg.Agent.AdvertiseAddrs = []string{"192.168.1.100", "[2001:db8::1"}
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for malformed advertise address")
	}
}

func TestConfig_Validate_WebhookConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server: ServerConfig{
				GRPCPort: 9090,
				HTTPPort: 8080,
			},
			NATS: NATSConfig{
				Mode: NATSModeEmbedded,
			},
			Storage: StorageConfig{
				Backend: StorageBackendSQLite,
			},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "invalid webhook port",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
				cfg.Webhook.Port = 0
			},
			errContains: "invalid webhook port",
		},
		{
			name: "invalid webhook auth type",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
				cfg.Webhook.Port = 8081
				cfg.Webhook.AuthType = "token"
			},
			errContains: "invalid webhook auth type",
		},
		{
			name: "missing HMAC secret",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
				cfg.Webhook.Port = 8081
				cfg.Webhook.AuthType = "hmac"
			},
			errContains: "HMAC secret",
		},
		{
			name: "missing bearer token",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
				cfg.Webhook.Port = 8081
				cfg.Webhook.AuthType = "bearer"
			},
			errContains: "bearer token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_PolicyConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server: ServerConfig{
				GRPCPort: 9090,
				HTTPPort: 8080,
			},
			NATS: NATSConfig{
				Mode: NATSModeEmbedded,
			},
			Storage: StorageConfig{
				Backend: StorageBackendSQLite,
			},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "invalid policy engine",
			config: func(cfg *Config) {
				cfg.Policy.Enabled = true
				cfg.Policy.Engine = "rego"
			},
			errContains: "invalid policy engine",
		},
		{
			name: "invalid enforcement mode",
			config: func(cfg *Config) {
				cfg.Policy.Enabled = true
				cfg.Policy.Engine = "opa"
				cfg.Policy.EnforcementMode = "block"
			},
			errContains: "invalid enforcement mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_AuthConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server: ServerConfig{
				GRPCPort: 9090,
				HTTPPort: 8080,
			},
			NATS: NATSConfig{
				Mode: NATSModeEmbedded,
			},
			Storage: StorageConfig{
				Backend: StorageBackendSQLite,
			},
			TLS: TLSConfig{
				Enabled: true,
				CAFile:  "/etc/keystone-core/ca.crt",
			},
		}
	}

	longKey := "12345678901234567890123456789012"

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "apikey requires keys",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "apikey"
				cfg.Auth.APIKey.Keys = map[string]APIKeyConfig{}
			},
			errContains: "at least one API key",
		},
		{
			name: "apikey too short",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "apikey"
				cfg.Auth.APIKey.Keys = map[string]APIKeyConfig{
					"short": {Name: "short", Role: "admin"},
				}
			},
			errContains: "too short",
		},
		{
			name: "apikey invalid role",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "apikey"
				cfg.Auth.APIKey.Keys = map[string]APIKeyConfig{
					longKey: {Name: "bad-role", Role: "owner"},
				}
			},
			errContains: "invalid role",
		},
		{
			name: "jwt requires secret or public key",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "jwt"
			},
			errContains: "JWT secret or public key file",
		},
		{
			name: "jwt secret and public key mutually exclusive",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "jwt"
				cfg.Auth.JWT.Secret = "secret"
				cfg.Auth.JWT.PublicKeyFile = "/etc/keystone-core/jwt.pub"
			},
			errContains: "mutually exclusive",
		},
		{
			name: "mtls requires TLS enabled",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "mtls"
				cfg.TLS.Enabled = false
			},
			errContains: "TLS must be enabled",
		},
		{
			name: "mtls requires CA file",
			config: func(cfg *Config) {
				cfg.Auth.Enabled = true
				cfg.Auth.Type = "mtls"
				cfg.TLS.CAFile = ""
			},
			errContains: "CA file must be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_LoggingConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server: ServerConfig{
				GRPCPort: 9090,
				HTTPPort: 8080,
			},
			NATS: NATSConfig{
				Mode: NATSModeEmbedded,
			},
			Storage: StorageConfig{
				Backend: StorageBackendSQLite,
			},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "invalid log level",
			config: func(cfg *Config) {
				cfg.Logging.Level = "trace"
			},
			errContains: "invalid log level",
		},
		{
			name: "invalid log format",
			config: func(cfg *Config) {
				cfg.Logging.Format = "console"
			},
			errContains: "invalid log format",
		},
		{
			name: "invalid log output",
			config: func(cfg *Config) {
				cfg.Logging.Output = "file"
			},
			errContains: "invalid log output",
		},
		{
			name: "invalid syslog network",
			config: func(cfg *Config) {
				cfg.Logging.Output = "syslog"
				cfg.Logging.Syslog.Network = "http"
			},
			errContains: "invalid syslog network",
		},
		{
			name: "invalid syslog facility",
			config: func(cfg *Config) {
				cfg.Logging.Output = "syslog"
				cfg.Logging.Syslog.Facility = "local9"
			},
			errContains: "invalid syslog facility",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_TLSForNonLoopback(t *testing.T) {
	// Base config helper
	baseConfig := func() *Config {
		return &Config{
			Server: ServerConfig{
				GRPCPort: 9090,
				HTTPPort: 8080,
			},
			NATS: NATSConfig{
				Mode: NATSModeEmbedded,
			},
			Storage: StorageConfig{
				Backend: StorageBackendSQLite,
			},
			TLS: TLSConfig{
				Enabled: false,
			},
		}
	}

	tests := []struct {
		name        string
		listenAddr  string
		tlsEnabled  bool
		allowInsec  bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "TLS enabled with any address - should pass",
			listenAddr: "0.0.0.0",
			tlsEnabled: true,
			wantErr:    false,
		},
		{
			name:       "loopback IPv4 without TLS - should pass",
			listenAddr: "127.0.0.1",
			tlsEnabled: false,
			wantErr:    false,
		},
		{
			name:       "loopback IPv6 without TLS - should pass",
			listenAddr: "::1",
			tlsEnabled: false,
			wantErr:    false,
		},
		{
			name:        "unspecified IPv4 without TLS - should fail",
			listenAddr:  "0.0.0.0",
			tlsEnabled:  false,
			wantErr:     true,
			errContains: "SECURITY ERROR",
		},
		{
			name:        "unspecified IPv6 without TLS - should fail",
			listenAddr:  "::",
			tlsEnabled:  false,
			wantErr:     true,
			errContains: "SECURITY ERROR",
		},
		{
			name:        "non-loopback IPv4 without TLS - should fail",
			listenAddr:  "192.168.1.100",
			tlsEnabled:  false,
			wantErr:     true,
			errContains: "SECURITY ERROR",
		},
		{
			name:        "non-loopback IPv6 without TLS - should fail",
			listenAddr:  "2001:db8::1",
			tlsEnabled:  false,
			wantErr:     true,
			errContains: "SECURITY ERROR",
		},
		{
			name:       "AllowInsecureNonLoopback bypasses check",
			listenAddr: "0.0.0.0",
			tlsEnabled: false,
			allowInsec: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.Server.ListenAddr = tt.listenAddr
			cfg.TLS.Enabled = tt.tlsEnabled
			cfg.Server.AllowInsecureNonLoopback = tt.allowInsec

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected validation error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error should contain %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestConfig_Validate_TLSMinVersion(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	jetstreamDir := filepath.Join(t.TempDir(), "jetstream")
	cfg.NATS.Embedded.StoreDir = jetstreamDir
	cfg.NATS.JetStream.StoreDir = jetstreamDir
	cfg.Auth.Enabled = false
	cfg.TLS.MinVersion = "1.2"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error for valid tls.minversion: %v", err)
	}

	cfg.TLS.MinVersion = "1.1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid tls.minversion")
	}

	cfg.TLS.MinVersion = "1.3"
	cfg.Logging.Syslog.TLS.MinVersion = "1.1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid logging.syslog.tls.minversion")
	}
}

func TestConfig_Validate_TLSForNonLoopback_MultipleAddrs(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: 9090,
			HTTPPort: 8080,
			// Multiple addresses - one loopback, one non-loopback
			ListenAddrs: []string{"127.0.0.1", "192.168.1.100"},
		},
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
		TLS: TLSConfig{
			Enabled: false,
		},
	}

	// Should fail because one address is non-loopback
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for mixed loopback/non-loopback addresses without TLS")
	}

	// Enable TLS - should pass
	cfg.TLS.Enabled = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("Unexpected error with TLS enabled: %v", err)
	}
}

// Helper to check if string contains substring
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

// Tests for ProductionWarnings

func TestConfig_ProductionWarnings_EmbeddedNATS(t *testing.T) {
	cfg := &Config{
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
		},
		Storage: StorageConfig{
			Backend: StorageBackendPostgreSQL,
		},
		TLS: TLSConfig{
			Enabled: true,
		},
	}

	warnings := cfg.ProductionWarnings()

	// Should have warning for embedded NATS
	found := false
	for _, w := range warnings {
		if w.Component == "nats" && w.Level == "warning" {
			found = true
			if !contains(w.Message, "Embedded NATS mode") {
				t.Errorf("NATS warning should mention embedded mode, got: %s", w.Message)
			}
			break
		}
	}
	if !found {
		t.Error("Expected warning for embedded NATS mode")
	}
}

func TestConfig_ProductionWarnings_SQLite(t *testing.T) {
	cfg := &Config{
		NATS: NATSConfig{
			Mode: NATSModeExternal,
			URL:  "nats://localhost:4222",
		},
		Storage: StorageConfig{
			Backend: StorageBackendSQLite,
		},
		TLS: TLSConfig{
			Enabled: true,
		},
	}

	warnings := cfg.ProductionWarnings()

	// Should have warning for SQLite
	found := false
	for _, w := range warnings {
		if w.Component == "storage" && w.Level == "warning" {
			found = true
			if !contains(w.Message, "SQLite") {
				t.Errorf("Storage warning should mention SQLite, got: %s", w.Message)
			}
			break
		}
	}
	if !found {
		t.Error("Expected warning for SQLite storage")
	}
}

func TestConfig_ProductionWarnings_TLSDisabledWithProduction(t *testing.T) {
	cfg := &Config{
		NATS: NATSConfig{
			Mode: NATSModeExternal,
			URL:  "nats://localhost:4222",
		},
		Storage: StorageConfig{
			Backend: StorageBackendPostgreSQL,
		},
		TLS: TLSConfig{
			Enabled: false,
		},
	}

	warnings := cfg.ProductionWarnings()

	// Should have critical warning for TLS disabled with production config
	found := false
	for _, w := range warnings {
		if w.Component == "tls" && w.Level == "critical" {
			found = true
			if !contains(w.Message, "TLS is disabled") {
				t.Errorf("TLS warning should mention disabled, got: %s", w.Message)
			}
			break
		}
	}
	if !found {
		t.Error("Expected critical warning for TLS disabled with production config")
	}
}

func TestConfig_ProductionWarnings_NoWarningsForProduction(t *testing.T) {
	cfg := &Config{
		NATS: NATSConfig{
			Mode: NATSModeExternal,
			URL:  "nats://localhost:4222",
		},
		Storage: StorageConfig{
			Backend: StorageBackendPostgreSQL,
		},
		TLS: TLSConfig{
			Enabled: true,
		},
	}

	warnings := cfg.ProductionWarnings()

	// Production config should have no warnings
	if len(warnings) != 0 {
		t.Errorf("Expected no warnings for production config, got %d: %+v", len(warnings), warnings)
	}
}

func TestConfig_ProductionWarnings_JetStreamDefault(t *testing.T) {
	cfg := &Config{
		NATS: NATSConfig{
			Mode: NATSModeEmbedded,
			JetStream: JetStreamConfig{
				Enabled:    true,
				MaxStorage: 0, // Default/unset
			},
		},
		Storage: StorageConfig{
			Backend: StorageBackendPostgreSQL,
		},
		TLS: TLSConfig{
			Enabled: true,
		},
	}

	warnings := cfg.ProductionWarnings()

	// Should have info warning for JetStream default storage
	found := false
	for _, w := range warnings {
		if w.Component == "jetstream" && w.Level == "info" {
			found = true
			if !contains(w.Message, "default storage limit") {
				t.Errorf("JetStream warning should mention default storage limit, got: %s", w.Message)
			}
			break
		}
	}
	if !found {
		t.Error("Expected info warning for JetStream default storage limit")
	}
}

func TestGetEffectiveNATSMaxMemory(t *testing.T) {
	// Test default value
	cfg := &Config{}
	if m := getEffectiveNATSMaxMemory(cfg); m != EmbeddedNATSMaxMemoryDefault {
		t.Errorf("Expected default memory %d, got %d", EmbeddedNATSMaxMemoryDefault, m)
	}

	// Test custom value
	cfg.NATS.Embedded.MaxMemory = 512 * 1024 * 1024
	if m := getEffectiveNATSMaxMemory(cfg); m != 512*1024*1024 {
		t.Errorf("Expected custom memory %d, got %d", 512*1024*1024, m)
	}
}

func TestGetEffectiveNATSMaxConnections(t *testing.T) {
	// Test default value
	cfg := &Config{}
	if c := getEffectiveNATSMaxConnections(cfg); c != EmbeddedNATSMaxConnectionsDefault {
		t.Errorf("Expected default connections %d, got %d", EmbeddedNATSMaxConnectionsDefault, c)
	}

	// Test custom value
	cfg.NATS.Embedded.MaxConnections = 5000
	if c := getEffectiveNATSMaxConnections(cfg); c != 5000 {
		t.Errorf("Expected custom connections %d, got %d", 5000, c)
	}
}

// Tests for NATS configuration validation with clear error messages

func TestNATSConfig_Validate_InvalidURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errHint string
	}{
		{
			name:    "valid nats URL",
			url:     "nats://localhost:4222",
			wantErr: false,
		},
		{
			name:    "valid tls URL",
			url:     "tls://secure.nats.io:4222",
			wantErr: false,
		},
		{
			name:    "valid websocket URL",
			url:     "ws://localhost:8080",
			wantErr: false,
		},
		{
			name:    "valid secure websocket URL",
			url:     "wss://localhost:8080",
			wantErr: false,
		},
		{
			name:    "comma-separated URLs",
			url:     "nats://host1:4222,nats://host2:4222",
			wantErr: false,
		},
		{
			name:    "invalid scheme",
			url:     "http://localhost:4222",
			wantErr: true,
			errHint: "Use format: nats://host:port",
		},
		{
			name:    "missing scheme",
			url:     "localhost:4222",
			wantErr: true,
			errHint: "unsupported URL scheme",
		},
		{
			name:    "missing host",
			url:     "nats://",
			wantErr: true,
			errHint: "URL host is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &NATSConfig{
				Mode: NATSModeExternal,
				URL:  tt.url,
			}

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for URL %q, got nil", tt.url)
					return
				}
				if tt.errHint != "" && !contains(err.Error(), tt.errHint) {
					t.Errorf("error should contain hint %q, got: %v", tt.errHint, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for URL %q: %v", tt.url, err)
				}
			}
		})
	}
}

func TestNATSConfig_Validate_ErrorMessages(t *testing.T) {
	tests := []struct {
		name        string
		config      NATSConfig
		wantField   string
		wantMessage string
		wantHint    string
	}{
		{
			name:        "empty mode",
			config:      NATSConfig{Mode: ""},
			wantField:   "mode",
			wantMessage: "NATS mode is required",
			wantHint:    "Set to 'embedded'",
		},
		{
			name:        "invalid mode",
			config:      NATSConfig{Mode: "invalid"},
			wantField:   "mode",
			wantMessage: "invalid NATS mode",
			wantHint:    "Valid modes are",
		},
		{
			name: "external without URL",
			config: NATSConfig{
				Mode: NATSModeExternal,
				URL:  "",
			},
			wantField:   "url",
			wantMessage: "NATS URL is required for external mode",
			wantHint:    "Provide the URL",
		},
		{
			name: "leaf without parent URLs",
			config: NATSConfig{
				Mode: NATSModeLeaf,
			},
			wantField:   "embedded.leafnodeurls",
			wantMessage: "leaf node parent URLs are required",
			wantHint:    "parent NATS cluster",
		},
		{
			name: "negative max reconnects",
			config: NATSConfig{
				Mode:          NATSModeExternal,
				URL:           "nats://localhost:4222",
				MaxReconnects: -5,
			},
			wantField:   "maxreconnects",
			wantMessage: "invalid max reconnects",
			wantHint:    "Use -1 for unlimited",
		},
		{
			name: "both token and credential",
			config: NATSConfig{
				Mode:       NATSModeExternal,
				URL:        "nats://localhost:4222",
				Token:      "secret",
				Credential: "/path/to/creds",
			},
			wantField:   "authentication",
			wantMessage: "both token and credential",
			wantHint:    "Use either",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			var natsErr *NATSConfigError
			if !errors.As(err, &natsErr) {
				t.Fatalf("expected *NATSConfigError, got %T", err)
			}

			if natsErr.Field != tt.wantField {
				t.Errorf("Field = %q, want %q", natsErr.Field, tt.wantField)
			}

			if !contains(natsErr.Message, tt.wantMessage) {
				t.Errorf("Message should contain %q, got %q", tt.wantMessage, natsErr.Message)
			}

			if !contains(natsErr.Hint, tt.wantHint) {
				t.Errorf("Hint should contain %q, got %q", tt.wantHint, natsErr.Hint)
			}

			// Verify Error() includes all parts
			errStr := err.Error()
			if !contains(errStr, natsErr.Field) {
				t.Errorf("Error() should contain field, got: %s", errStr)
			}
			if !contains(errStr, natsErr.Hint) {
				t.Errorf("Error() should contain hint, got: %s", errStr)
			}
		})
	}
}

func TestNATSConfig_Validate_EmbeddedSettings(t *testing.T) {
	tests := []struct {
		name    string
		config  NATSConfig
		wantErr bool
		errHint string
	}{
		{
			name: "valid embedded config",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					Port:           4222,
					MaxConnections: 1000,
					MaxMemory:      1024 * 1024 * 100,
				},
			},
			wantErr: false,
		},
		{
			name: "zero port uses default",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					Port: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "negative port",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					Port: -1,
				},
			},
			wantErr: true,
			errHint: "Port must be between",
		},
		{
			name: "port too high",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					Port: 70000,
				},
			},
			wantErr: true,
			errHint: "Port must be between",
		},
		{
			name: "negative max connections",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					MaxConnections: -1,
				},
			},
			wantErr: true,
			errHint: "cannot be negative",
		},
		{
			name: "invalid address family",
			config: NATSConfig{
				Mode: NATSModeEmbedded,
				Embedded: NATSEmbeddedConfig{
					AddressFamily: "invalid",
				},
			},
			wantErr: true,
			errHint: "Valid options:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errHint != "" && !contains(err.Error(), tt.errHint) {
					t.Errorf("error should contain %q, got: %v", tt.errHint, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestJetStreamConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  JetStreamConfig
		wantErr bool
	}{
		{
			name: "disabled jetstream",
			config: JetStreamConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid enabled jetstream",
			config: JetStreamConfig{
				Enabled:    true,
				MaxStorage: 1024 * 1024 * 1024, // 1GB
			},
			wantErr: false,
		},
		{
			name: "zero max storage (auto)",
			config: JetStreamConfig{
				Enabled:    true,
				MaxStorage: 0,
			},
			wantErr: false,
		},
		{
			name: "negative max storage",
			config: JetStreamConfig{
				Enabled:    true,
				MaxStorage: -100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfig_Validate_ProfilingConfig(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
			NATS:    NATSConfig{Mode: NATSModeEmbedded},
			Storage: StorageConfig{Backend: StorageBackendSQLite},
		}
	}

	t.Run("disabled by default is valid", func(t *testing.T) {
		cfg := base()
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("enabled with listen address is valid", func(t *testing.T) {
		cfg := base()
		cfg.Profiling = ProfilingConfig{
			Enabled: true,
			Listen:  "localhost:6060",
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("enabled without listen address is invalid", func(t *testing.T) {
		cfg := base()
		cfg.Profiling = ProfilingConfig{
			Enabled: true,
			Listen:  "",
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected validation error for missing listen address")
		}
	})
}

func TestConfig_Redact(t *testing.T) {
	cfg := &Config{
		Auth: AuthConfig{
			JWT: JWTAuthConfig{
				Secret:   "super-secret-jwt-key",
				Issuer:   "kscore",
				Audience: "api",
			},
			APIKey: APIKeyAuthConfig{
				HeaderName: "X-API-Key",
				Keys: map[string]APIKeyConfig{
					"real-api-key-1": {Name: "admin", Role: "admin", Enabled: true},
					"real-api-key-2": {Name: "reader", Role: "readonly", Enabled: true},
				},
			},
		},
		NATS: NATSConfig{
			Mode:       NATSModeEmbedded,
			Token:      "nats-token-secret",
			Credential: "nats-cred-file",
		},
		Webhook: WebhookConfig{
			Enabled:     true,
			HMACSecret:  "hmac-shared-secret",
			BearerToken: "bearer-token-secret",
		},
		Server: ServerConfig{
			ListenAddr: "0.0.0.0",
			HTTPPort:   8080,
		},
	}

	redacted := cfg.Redact()

	// Sensitive fields should be redacted
	if redacted.Auth.JWT.Secret != "[REDACTED]" {
		t.Errorf("JWT secret not redacted: %s", redacted.Auth.JWT.Secret)
	}
	if redacted.NATS.Token != "[REDACTED]" {
		t.Errorf("NATS token not redacted: %s", redacted.NATS.Token)
	}
	if redacted.NATS.Credential != "[REDACTED]" {
		t.Errorf("NATS credential not redacted: %s", redacted.NATS.Credential)
	}
	if redacted.Webhook.HMACSecret != "[REDACTED]" {
		t.Errorf("webhook HMAC secret not redacted: %s", redacted.Webhook.HMACSecret)
	}
	if redacted.Webhook.BearerToken != "[REDACTED]" {
		t.Errorf("webhook bearer token not redacted: %s", redacted.Webhook.BearerToken)
	}

	// API keys: the actual key strings should not appear, count should be preserved
	if len(redacted.Auth.APIKey.Keys) != 2 {
		t.Errorf("expected 2 redacted API keys, got %d", len(redacted.Auth.APIKey.Keys))
	}
	for k := range redacted.Auth.APIKey.Keys {
		if k == "real-api-key-1" || k == "real-api-key-2" {
			t.Errorf("API key value not redacted: %s", k)
		}
	}

	// Non-sensitive fields should be preserved
	if redacted.Auth.JWT.Issuer != "kscore" {
		t.Errorf("JWT issuer changed: %s", redacted.Auth.JWT.Issuer)
	}
	if redacted.Server.ListenAddr != "0.0.0.0" {
		t.Errorf("server listen addr changed: %s", redacted.Server.ListenAddr)
	}
	if redacted.Server.HTTPPort != 8080 {
		t.Errorf("server HTTP port changed: %d", redacted.Server.HTTPPort)
	}
	if redacted.Webhook.Enabled != true {
		t.Error("webhook enabled changed")
	}

	// Original should not be modified
	if cfg.Auth.JWT.Secret != "super-secret-jwt-key" {
		t.Error("original config was modified by Redact()")
	}
	if cfg.NATS.Token != "nats-token-secret" {
		t.Error("original NATS token was modified by Redact()")
	}
}

func TestConfig_Redact_EmptySecrets(t *testing.T) {
	cfg := &Config{}
	redacted := cfg.Redact()

	// Empty strings should stay empty, not become "[REDACTED]"
	if redacted.Auth.JWT.Secret != "" {
		t.Errorf("expected empty JWT secret to stay empty, got: %s", redacted.Auth.JWT.Secret)
	}
	if redacted.NATS.Token != "" {
		t.Errorf("expected empty NATS token to stay empty, got: %s", redacted.NATS.Token)
	}
	if redacted.Webhook.HMACSecret != "" {
		t.Errorf("expected empty HMAC secret to stay empty, got: %s", redacted.Webhook.HMACSecret)
	}
}

// Tests for AgentManagementConfig, ExecutionConfig, StateManagementConfig (Epic 45 Phase 1)

func TestLoadConfig_Defaults_NewSections(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	// AgentManagement defaults
	if cfg.AgentManagement.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Errorf("AgentManagement.HeartbeatInterval = %v, want %v", cfg.AgentManagement.HeartbeatInterval, DefaultHeartbeatInterval)
	}
	if cfg.AgentManagement.HeartbeatTimeout != DefaultAgentMgmtHeartbeatTimeout {
		t.Errorf("AgentManagement.HeartbeatTimeout = %v, want %v", cfg.AgentManagement.HeartbeatTimeout, DefaultAgentMgmtHeartbeatTimeout)
	}
	if cfg.AgentManagement.StaleThreshold != DefaultAgentMgmtStaleThreshold {
		t.Errorf("AgentManagement.StaleThreshold = %d, want %d", cfg.AgentManagement.StaleThreshold, DefaultAgentMgmtStaleThreshold)
	}
	if cfg.AgentManagement.MonitorInterval != DefaultAgentMgmtMonitorInterval {
		t.Errorf("AgentManagement.MonitorInterval = %v, want %v", cfg.AgentManagement.MonitorInterval, DefaultAgentMgmtMonitorInterval)
	}
	if cfg.AgentManagement.MetadataRefresh != DefaultMetadataInterval {
		t.Errorf("AgentManagement.MetadataRefresh = %v, want %v", cfg.AgentManagement.MetadataRefresh, DefaultMetadataInterval)
	}
	if cfg.AgentManagement.MaxConcurrentCommands != DefaultAgentMgmtMaxConcurrentCmds {
		t.Errorf("AgentManagement.MaxConcurrentCommands = %d, want %d", cfg.AgentManagement.MaxConcurrentCommands, DefaultAgentMgmtMaxConcurrentCmds)
	}

	// Execution defaults
	if cfg.Execution.DefaultTimeout != DefaultExecDefaultTimeout {
		t.Errorf("Execution.DefaultTimeout = %v, want %v", cfg.Execution.DefaultTimeout, DefaultExecDefaultTimeout)
	}
	if cfg.Execution.MaxTimeout != DefaultExecMaxTimeout {
		t.Errorf("Execution.MaxTimeout = %v, want %v", cfg.Execution.MaxTimeout, DefaultExecMaxTimeout)
	}
	if cfg.Execution.BatchSize != DefaultExecBatchSize {
		t.Errorf("Execution.BatchSize = %d, want %d", cfg.Execution.BatchSize, DefaultExecBatchSize)
	}
	if cfg.Execution.StreamingBuffer != DefaultExecStreamingBuffer {
		t.Errorf("Execution.StreamingBuffer = %d, want %d", cfg.Execution.StreamingBuffer, DefaultExecStreamingBuffer)
	}
	if cfg.Execution.ResultRetention != DefaultExecResultRetention {
		t.Errorf("Execution.ResultRetention = %v, want %v", cfg.Execution.ResultRetention, DefaultExecResultRetention)
	}

	// StateManagement defaults
	if cfg.StateManagement.DefaultTimeout != DefaultStateMgmtDefaultTimeout {
		t.Errorf("StateManagement.DefaultTimeout = %v, want %v", cfg.StateManagement.DefaultTimeout, DefaultStateMgmtDefaultTimeout)
	}
	if cfg.StateManagement.MaxConcurrent != DefaultStateMgmtMaxConcurrent {
		t.Errorf("StateManagement.MaxConcurrent = %d, want %d", cfg.StateManagement.MaxConcurrent, DefaultStateMgmtMaxConcurrent)
	}
	if cfg.StateManagement.DriftCheckInterval != 0 {
		t.Errorf("StateManagement.DriftCheckInterval = %v, want 0", cfg.StateManagement.DriftCheckInterval)
	}
	if cfg.StateManagement.ResultRetention != DefaultStateMgmtResultRetention {
		t.Errorf("StateManagement.ResultRetention = %v, want %v", cfg.StateManagement.ResultRetention, DefaultStateMgmtResultRetention)
	}
}

func TestLoadConfig_FromFile_NewSections(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "kscore-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `
server:
  listenaddr: "127.0.0.1"
  grpcport: 9090
  httpport: 8080
nats:
  mode: embedded
storage:
  backend: sqlite
agentmanagement:
  heartbeatinterval: 45s
  heartbeattimeout: 90s
  stalethreshold: 5
  monitorinterval: 15s
  metadatarefresh: 10m
  maxconcurrentcommands: 20
execution:
  defaulttimeout: 10m
  maxtimeout: 2h
  batchsize: 25
  batchdelay: 1s
  streamingbuffer: 200
  resultretention: 48h
statemanagement:
  defaulttimeout: 15m
  maxconcurrent: 10
  driftcheckinterval: 30m
  resultretention: 168h
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// AgentManagement from file
	if cfg.AgentManagement.HeartbeatInterval != 45*time.Second {
		t.Errorf("AgentManagement.HeartbeatInterval = %v, want 45s", cfg.AgentManagement.HeartbeatInterval)
	}
	if cfg.AgentManagement.HeartbeatTimeout != 90*time.Second {
		t.Errorf("AgentManagement.HeartbeatTimeout = %v, want 90s", cfg.AgentManagement.HeartbeatTimeout)
	}
	if cfg.AgentManagement.StaleThreshold != 5 {
		t.Errorf("AgentManagement.StaleThreshold = %d, want 5", cfg.AgentManagement.StaleThreshold)
	}
	if cfg.AgentManagement.MonitorInterval != 15*time.Second {
		t.Errorf("AgentManagement.MonitorInterval = %v, want 15s", cfg.AgentManagement.MonitorInterval)
	}
	if cfg.AgentManagement.MetadataRefresh != 10*time.Minute {
		t.Errorf("AgentManagement.MetadataRefresh = %v, want 10m", cfg.AgentManagement.MetadataRefresh)
	}
	if cfg.AgentManagement.MaxConcurrentCommands != 20 {
		t.Errorf("AgentManagement.MaxConcurrentCommands = %d, want 20", cfg.AgentManagement.MaxConcurrentCommands)
	}

	// Execution from file
	if cfg.Execution.DefaultTimeout != 10*time.Minute {
		t.Errorf("Execution.DefaultTimeout = %v, want 10m", cfg.Execution.DefaultTimeout)
	}
	if cfg.Execution.MaxTimeout != 2*time.Hour {
		t.Errorf("Execution.MaxTimeout = %v, want 2h", cfg.Execution.MaxTimeout)
	}
	if cfg.Execution.BatchSize != 25 {
		t.Errorf("Execution.BatchSize = %d, want 25", cfg.Execution.BatchSize)
	}
	if cfg.Execution.BatchDelay != 1*time.Second {
		t.Errorf("Execution.BatchDelay = %v, want 1s", cfg.Execution.BatchDelay)
	}
	if cfg.Execution.StreamingBuffer != 200 {
		t.Errorf("Execution.StreamingBuffer = %d, want 200", cfg.Execution.StreamingBuffer)
	}
	if cfg.Execution.ResultRetention != 48*time.Hour {
		t.Errorf("Execution.ResultRetention = %v, want 48h", cfg.Execution.ResultRetention)
	}

	// StateManagement from file
	if cfg.StateManagement.DefaultTimeout != 15*time.Minute {
		t.Errorf("StateManagement.DefaultTimeout = %v, want 15m", cfg.StateManagement.DefaultTimeout)
	}
	if cfg.StateManagement.MaxConcurrent != 10 {
		t.Errorf("StateManagement.MaxConcurrent = %d, want 10", cfg.StateManagement.MaxConcurrent)
	}
	if cfg.StateManagement.DriftCheckInterval != 30*time.Minute {
		t.Errorf("StateManagement.DriftCheckInterval = %v, want 30m", cfg.StateManagement.DriftCheckInterval)
	}
	if cfg.StateManagement.ResultRetention != 168*time.Hour {
		t.Errorf("StateManagement.ResultRetention = %v, want 168h", cfg.StateManagement.ResultRetention)
	}
}

func TestConfig_Validate_AgentManagementConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
			NATS:    NATSConfig{Mode: NATSModeEmbedded},
			Storage: StorageConfig{Backend: StorageBackendSQLite},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "negative heartbeat interval",
			config: func(cfg *Config) {
				cfg.AgentManagement.HeartbeatInterval = -1 * time.Second
			},
			errContains: "heartbeatinterval cannot be negative",
		},
		{
			name: "negative heartbeat timeout",
			config: func(cfg *Config) {
				cfg.AgentManagement.HeartbeatTimeout = -1 * time.Second
			},
			errContains: "heartbeattimeout cannot be negative",
		},
		{
			name: "timeout less than interval",
			config: func(cfg *Config) {
				cfg.AgentManagement.HeartbeatInterval = 60 * time.Second
				cfg.AgentManagement.HeartbeatTimeout = 30 * time.Second
			},
			errContains: "heartbeattimeout",
		},
		{
			name: "negative stale threshold",
			config: func(cfg *Config) {
				cfg.AgentManagement.StaleThreshold = -1
			},
			errContains: "stalethreshold cannot be negative",
		},
		{
			name: "negative monitor interval",
			config: func(cfg *Config) {
				cfg.AgentManagement.MonitorInterval = -1 * time.Second
			},
			errContains: "monitorinterval cannot be negative",
		},
		{
			name: "negative metadata refresh",
			config: func(cfg *Config) {
				cfg.AgentManagement.MetadataRefresh = -1 * time.Second
			},
			errContains: "metadatarefresh cannot be negative",
		},
		{
			name: "negative max concurrent commands",
			config: func(cfg *Config) {
				cfg.AgentManagement.MaxConcurrentCommands = -1
			},
			errContains: "maxconcurrentcommands cannot be negative",
		},
		{
			name: "valid config",
			config: func(cfg *Config) {
				cfg.AgentManagement.HeartbeatInterval = 30 * time.Second
				cfg.AgentManagement.HeartbeatTimeout = 60 * time.Second
				cfg.AgentManagement.StaleThreshold = 3
				cfg.AgentManagement.MonitorInterval = 10 * time.Second
				cfg.AgentManagement.MetadataRefresh = 5 * time.Minute
				cfg.AgentManagement.MaxConcurrentCommands = 10
			},
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_ExecutionConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
			NATS:    NATSConfig{Mode: NATSModeEmbedded},
			Storage: StorageConfig{Backend: StorageBackendSQLite},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "negative default timeout",
			config: func(cfg *Config) {
				cfg.Execution.DefaultTimeout = -1 * time.Second
			},
			errContains: "defaulttimeout cannot be negative",
		},
		{
			name: "negative max timeout",
			config: func(cfg *Config) {
				cfg.Execution.MaxTimeout = -1 * time.Second
			},
			errContains: "maxtimeout cannot be negative",
		},
		{
			name: "default exceeds max",
			config: func(cfg *Config) {
				cfg.Execution.DefaultTimeout = 2 * time.Hour
				cfg.Execution.MaxTimeout = 1 * time.Hour
			},
			errContains: "cannot exceed maxtimeout",
		},
		{
			name: "negative batch size",
			config: func(cfg *Config) {
				cfg.Execution.BatchSize = -1
			},
			errContains: "batchsize cannot be negative",
		},
		{
			name: "negative batch delay",
			config: func(cfg *Config) {
				cfg.Execution.BatchDelay = -1 * time.Second
			},
			errContains: "batchdelay cannot be negative",
		},
		{
			name: "negative streaming buffer",
			config: func(cfg *Config) {
				cfg.Execution.StreamingBuffer = -1
			},
			errContains: "streamingbuffer cannot be negative",
		},
		{
			name: "negative result retention",
			config: func(cfg *Config) {
				cfg.Execution.ResultRetention = -1 * time.Hour
			},
			errContains: "resultretention cannot be negative",
		},
		{
			name: "valid config",
			config: func(cfg *Config) {
				cfg.Execution.DefaultTimeout = 5 * time.Minute
				cfg.Execution.MaxTimeout = 1 * time.Hour
				cfg.Execution.BatchSize = 10
				cfg.Execution.StreamingBuffer = 100
				cfg.Execution.ResultRetention = 24 * time.Hour
			},
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestConfig_Validate_StateManagementConfig(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
			NATS:    NATSConfig{Mode: NATSModeEmbedded},
			Storage: StorageConfig{Backend: StorageBackendSQLite},
		}
	}

	tests := []struct {
		name        string
		config      func(*Config)
		errContains string
	}{
		{
			name: "negative default timeout",
			config: func(cfg *Config) {
				cfg.StateManagement.DefaultTimeout = -1 * time.Second
			},
			errContains: "defaulttimeout cannot be negative",
		},
		{
			name: "negative max concurrent",
			config: func(cfg *Config) {
				cfg.StateManagement.MaxConcurrent = -1
			},
			errContains: "maxconcurrent cannot be negative",
		},
		{
			name: "negative drift check interval",
			config: func(cfg *Config) {
				cfg.StateManagement.DriftCheckInterval = -1 * time.Second
			},
			errContains: "driftcheckinterval cannot be negative",
		},
		{
			name: "negative result retention",
			config: func(cfg *Config) {
				cfg.StateManagement.ResultRetention = -1 * time.Hour
			},
			errContains: "resultretention cannot be negative",
		},
		{
			name: "valid config with drift check",
			config: func(cfg *Config) {
				cfg.StateManagement.DefaultTimeout = 15 * time.Minute
				cfg.StateManagement.MaxConcurrent = 10
				cfg.StateManagement.DriftCheckInterval = 30 * time.Minute
				cfg.StateManagement.ResultRetention = 168 * time.Hour
			},
			errContains: "",
		},
		{
			name: "valid config with drift check disabled",
			config: func(cfg *Config) {
				cfg.StateManagement.DriftCheckInterval = 0
			},
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.config(cfg)

			err := cfg.Validate()
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if !contains(err.Error(), tt.errContains) {
				t.Fatalf("Expected error to contain %q, got %v", tt.errContains, err)
			}
		})
	}
}

func TestAgentManagementConfig_Validate_Direct(t *testing.T) {
	// Valid config
	cfg := &AgentManagementConfig{
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  60 * time.Second,
		StaleThreshold:    3,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}

	// Zero values are valid (use defaults)
	cfg = &AgentManagementConfig{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero-value config failed: %v", err)
	}
}

func TestExecutionConfig_Validate_Direct(t *testing.T) {
	// Zero max timeout means no limit - default can be anything
	cfg := &ExecutionConfig{
		DefaultTimeout: 10 * time.Minute,
		MaxTimeout:     0,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("config with no max timeout limit failed: %v", err)
	}

	// Zero default timeout means no default applied
	cfg = &ExecutionConfig{
		DefaultTimeout: 0,
		MaxTimeout:     1 * time.Hour,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("config with no default timeout failed: %v", err)
	}
}

func TestStateManagementConfig_Validate_Direct(t *testing.T) {
	cfg := &StateManagementConfig{
		DefaultTimeout:     15 * time.Minute,
		MaxConcurrent:      10,
		DriftCheckInterval: 30 * time.Minute,
		ResultRetention:    7 * 24 * time.Hour,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}
}

// Phase 2 tests: Events, GitOps, Security/Authorization config

func TestLoadConfig_Defaults_Phase2Sections(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	// Events defaults
	if !cfg.Events.Enabled {
		t.Error("Events.Enabled should default to true")
	}
	if cfg.Events.Retention != DefaultEventsRetention {
		t.Errorf("Events.Retention = %v, want %v", cfg.Events.Retention, DefaultEventsRetention)
	}
	if cfg.Events.MaxBytes != DefaultEventsMaxBytes {
		t.Errorf("Events.MaxBytes = %d, want %d", cfg.Events.MaxBytes, DefaultEventsMaxBytes)
	}
	if cfg.Events.MaxMessages != DefaultEventsMaxMessages {
		t.Errorf("Events.MaxMessages = %d, want %d", cfg.Events.MaxMessages, DefaultEventsMaxMessages)
	}
	if cfg.Events.PublisherBufferSize != DefaultEventsPublisherBufferSize {
		t.Errorf("Events.PublisherBufferSize = %d, want %d", cfg.Events.PublisherBufferSize, DefaultEventsPublisherBufferSize)
	}
	if cfg.Events.SubscriberBufferSize != DefaultEventsSubscriberBufferSize {
		t.Errorf("Events.SubscriberBufferSize = %d, want %d", cfg.Events.SubscriberBufferSize, DefaultEventsSubscriberBufferSize)
	}
	if cfg.Events.SubscriberAckWait != DefaultEventsSubscriberAckWait {
		t.Errorf("Events.SubscriberAckWait = %v, want %v", cfg.Events.SubscriberAckWait, DefaultEventsSubscriberAckWait)
	}

	// GitOps defaults
	if cfg.GitOps.GitSync.Enabled {
		t.Error("GitOps.GitSync.Enabled should default to false")
	}

	// Authorization defaults
	if !cfg.Auth.Authorization.Enabled {
		t.Error("Auth.Authorization.Enabled should default to true")
	}
	if !cfg.Auth.Authorization.DefaultDeny {
		t.Error("Auth.Authorization.DefaultDeny should default to true")
	}
}

func TestLoadConfig_FromFile_Phase2Sections(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "kscore-config-phase2-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `
server:
  listenaddr: "127.0.0.1"
  grpcport: 9090
  httpport: 8080
nats:
  mode: embedded
storage:
  backend: sqlite
events:
  enabled: true
  retention: 72h
  maxbytes: 5368709120
  maxmessages: 500000
  publisherbuffersize: 512
  subscriberbuffersize: 128
  subscriberackwait: 60s
gitops:
  gitsync:
    enabled: true
    repositories:
      - url: "https://github.com/org/repo.git"
        branch: "production"
        interval: 10m
        path: "states/"
      - url: "https://github.com/org/infra.git"
        branch: "main"
        interval: 5m
auth:
  enabled: false
  authorization:
    enabled: false
    defaultdeny: false
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Events from file
	if !cfg.Events.Enabled {
		t.Error("Events.Enabled should be true")
	}
	if cfg.Events.Retention != 72*time.Hour {
		t.Errorf("Events.Retention = %v, want 72h", cfg.Events.Retention)
	}
	if cfg.Events.MaxBytes != 5368709120 {
		t.Errorf("Events.MaxBytes = %d, want 5368709120", cfg.Events.MaxBytes)
	}
	if cfg.Events.MaxMessages != 500000 {
		t.Errorf("Events.MaxMessages = %d, want 500000", cfg.Events.MaxMessages)
	}
	if cfg.Events.PublisherBufferSize != 512 {
		t.Errorf("Events.PublisherBufferSize = %d, want 512", cfg.Events.PublisherBufferSize)
	}
	if cfg.Events.SubscriberBufferSize != 128 {
		t.Errorf("Events.SubscriberBufferSize = %d, want 128", cfg.Events.SubscriberBufferSize)
	}
	if cfg.Events.SubscriberAckWait != 60*time.Second {
		t.Errorf("Events.SubscriberAckWait = %v, want 60s", cfg.Events.SubscriberAckWait)
	}

	// GitOps from file
	if !cfg.GitOps.GitSync.Enabled {
		t.Error("GitOps.GitSync.Enabled should be true")
	}
	if len(cfg.GitOps.GitSync.Repositories) != 2 {
		t.Fatalf("GitOps.GitSync.Repositories length = %d, want 2", len(cfg.GitOps.GitSync.Repositories))
	}
	repo := cfg.GitOps.GitSync.Repositories[0]
	if repo.URL != "https://github.com/org/repo.git" {
		t.Errorf("repo[0].URL = %q, want https://github.com/org/repo.git", repo.URL)
	}
	if repo.Branch != "production" {
		t.Errorf("repo[0].Branch = %q, want production", repo.Branch)
	}
	if repo.Interval != 10*time.Minute {
		t.Errorf("repo[0].Interval = %v, want 10m", repo.Interval)
	}
	if repo.Path != "states/" {
		t.Errorf("repo[0].Path = %q, want states/", repo.Path)
	}

	// Authorization from file
	if cfg.Auth.Authorization.Enabled {
		t.Error("Auth.Authorization.Enabled should be false")
	}
	if cfg.Auth.Authorization.DefaultDeny {
		t.Error("Auth.Authorization.DefaultDeny should be false")
	}
}

func TestConfig_Validate_EventsConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid defaults",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "negative retention",
			modify: func(c *Config) {
				c.Events.Retention = -1 * time.Hour
			},
			wantErr: true,
			errMsg:  "events.retention",
		},
		{
			name: "negative max bytes",
			modify: func(c *Config) {
				c.Events.MaxBytes = -1
			},
			wantErr: true,
			errMsg:  "events.maxbytes",
		},
		{
			name: "negative max messages",
			modify: func(c *Config) {
				c.Events.MaxMessages = -1
			},
			wantErr: true,
			errMsg:  "events.maxmessages",
		},
		{
			name: "negative publisher buffer",
			modify: func(c *Config) {
				c.Events.PublisherBufferSize = -1
			},
			wantErr: true,
			errMsg:  "events.publisherbuffersize",
		},
		{
			name: "negative subscriber buffer",
			modify: func(c *Config) {
				c.Events.SubscriberBufferSize = -1
			},
			wantErr: true,
			errMsg:  "events.subscriberbuffersize",
		},
		{
			name: "negative ack wait",
			modify: func(c *Config) {
				c.Events.SubscriberAckWait = -1 * time.Second
			},
			wantErr: true,
			errMsg:  "events.subscriberrackwait",
		},
		{
			name: "zero values are valid",
			modify: func(c *Config) {
				c.Events.Retention = 0
				c.Events.MaxBytes = 0
				c.Events.MaxMessages = 0
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
				NATS:    NATSConfig{Mode: NATSModeEmbedded},
				Storage: StorageConfig{Backend: StorageBackendSQLite},
			}
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfig_Validate_GitOpsConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "disabled is valid",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "enabled with valid repos",
			modify: func(c *Config) {
				c.GitOps.GitSync.Enabled = true
				c.GitOps.GitSync.Repositories = []GitSyncRepository{
					{URL: "https://github.com/org/repo.git", Branch: "main", Interval: 5 * time.Minute},
				}
			},
			wantErr: false,
		},
		{
			name: "enabled with empty repo URL",
			modify: func(c *Config) {
				c.GitOps.GitSync.Enabled = true
				c.GitOps.GitSync.Repositories = []GitSyncRepository{
					{URL: "", Branch: "main"},
				}
			},
			wantErr: true,
			errMsg:  "url is required",
		},
		{
			name: "negative repo interval",
			modify: func(c *Config) {
				c.GitOps.GitSync.Enabled = true
				c.GitOps.GitSync.Repositories = []GitSyncRepository{
					{URL: "https://github.com/org/repo.git", Interval: -1 * time.Minute},
				}
			},
			wantErr: true,
			errMsg:  "interval cannot be negative",
		},
		{
			name: "enabled with no repos is valid",
			modify: func(c *Config) {
				c.GitOps.GitSync.Enabled = true
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server:  ServerConfig{GRPCPort: 9090, HTTPPort: 8080},
				NATS:    NATSConfig{Mode: NATSModeEmbedded},
				Storage: StorageConfig{Backend: StorageBackendSQLite},
			}
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEventsConfig_Validate_Direct(t *testing.T) {
	cfg := &EventsConfig{
		Enabled:              true,
		Retention:            48 * time.Hour,
		MaxBytes:             5 * 1024 * 1024 * 1024,
		MaxMessages:          500000,
		PublisherBufferSize:  512,
		SubscriberBufferSize: 128,
		SubscriberAckWait:    45 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}
}

func TestGitOpsConfig_Validate_Direct(t *testing.T) {
	cfg := &GitOpsConfig{
		GitSync: GitSyncConfig{
			Enabled: true,
			Repositories: []GitSyncRepository{
				{URL: "https://github.com/org/repo.git", Branch: "main", Interval: 5 * time.Minute},
				{URL: "https://github.com/org/infra.git", Path: "deploy/"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}

	// Disabled config with no repos should pass
	cfg = &GitOpsConfig{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled config failed: %v", err)
	}
}

func TestAuthorizationConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	// Authorization should default to enabled with default deny
	if !cfg.Auth.Authorization.Enabled {
		t.Error("Auth.Authorization.Enabled should default to true")
	}
	if !cfg.Auth.Authorization.DefaultDeny {
		t.Error("Auth.Authorization.DefaultDeny should default to true")
	}
}

func TestLoadConfig_EnvOverrides_AgentManagement(t *testing.T) {
	t.Setenv("KSCORE_AGENT_MGMT_HEARTBEAT_INTERVAL", "45s")
	t.Setenv("KSCORE_AGENT_MGMT_HEARTBEAT_TIMEOUT", "120s")
	t.Setenv("KSCORE_AGENT_MGMT_STALE_THRESHOLD", "5")
	t.Setenv("KSCORE_AGENT_MGMT_MONITOR_INTERVAL", "20s")
	t.Setenv("KSCORE_AGENT_MGMT_METADATA_REFRESH", "10m")
	t.Setenv("KSCORE_AGENT_MGMT_MAX_CONCURRENT_COMMANDS", "8")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.AgentManagement.HeartbeatInterval != 45*time.Second {
		t.Errorf("HeartbeatInterval = %v, want 45s", cfg.AgentManagement.HeartbeatInterval)
	}
	if cfg.AgentManagement.HeartbeatTimeout != 120*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 120s", cfg.AgentManagement.HeartbeatTimeout)
	}
	if cfg.AgentManagement.StaleThreshold != 5 {
		t.Errorf("StaleThreshold = %d, want 5", cfg.AgentManagement.StaleThreshold)
	}
	if cfg.AgentManagement.MonitorInterval != 20*time.Second {
		t.Errorf("MonitorInterval = %v, want 20s", cfg.AgentManagement.MonitorInterval)
	}
	if cfg.AgentManagement.MetadataRefresh != 10*time.Minute {
		t.Errorf("MetadataRefresh = %v, want 10m", cfg.AgentManagement.MetadataRefresh)
	}
	if cfg.AgentManagement.MaxConcurrentCommands != 8 {
		t.Errorf("MaxConcurrentCommands = %d, want 8", cfg.AgentManagement.MaxConcurrentCommands)
	}
}

func TestLoadConfig_EnvOverrides_Execution(t *testing.T) {
	t.Setenv("KSCORE_EXEC_DEFAULT_TIMEOUT", "10m")
	t.Setenv("KSCORE_EXEC_MAX_TIMEOUT", "2h")
	t.Setenv("KSCORE_EXEC_BATCH_SIZE", "25")
	t.Setenv("KSCORE_EXEC_BATCH_DELAY", "2s")
	t.Setenv("KSCORE_EXEC_STREAMING_BUFFER", "200")
	t.Setenv("KSCORE_EXEC_RESULT_RETENTION", "48h")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Execution.DefaultTimeout != 10*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 10m", cfg.Execution.DefaultTimeout)
	}
	if cfg.Execution.MaxTimeout != 2*time.Hour {
		t.Errorf("MaxTimeout = %v, want 2h", cfg.Execution.MaxTimeout)
	}
	if cfg.Execution.BatchSize != 25 {
		t.Errorf("BatchSize = %d, want 25", cfg.Execution.BatchSize)
	}
	if cfg.Execution.BatchDelay != 2*time.Second {
		t.Errorf("BatchDelay = %v, want 2s", cfg.Execution.BatchDelay)
	}
	if cfg.Execution.StreamingBuffer != 200 {
		t.Errorf("StreamingBuffer = %d, want 200", cfg.Execution.StreamingBuffer)
	}
	if cfg.Execution.ResultRetention != 48*time.Hour {
		t.Errorf("ResultRetention = %v, want 48h", cfg.Execution.ResultRetention)
	}
}

func TestLoadConfig_EnvOverrides_StateManagement(t *testing.T) {
	t.Setenv("KSCORE_STATE_DEFAULT_TIMEOUT", "15m")
	t.Setenv("KSCORE_STATE_MAX_CONCURRENT", "10")
	t.Setenv("KSCORE_STATE_DRIFT_CHECK_INTERVAL", "30m")
	t.Setenv("KSCORE_STATE_RESULT_RETENTION", "336h")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.StateManagement.DefaultTimeout != 15*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 15m", cfg.StateManagement.DefaultTimeout)
	}
	if cfg.StateManagement.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want 10", cfg.StateManagement.MaxConcurrent)
	}
	if cfg.StateManagement.DriftCheckInterval != 30*time.Minute {
		t.Errorf("DriftCheckInterval = %v, want 30m", cfg.StateManagement.DriftCheckInterval)
	}
	if cfg.StateManagement.ResultRetention != 336*time.Hour {
		t.Errorf("ResultRetention = %v, want 336h", cfg.StateManagement.ResultRetention)
	}
}

func TestLoadConfig_EnvOverrides_Events(t *testing.T) {
	t.Setenv("KSCORE_EVENTS_ENABLED", "false")
	t.Setenv("KSCORE_EVENTS_RETENTION", "48h")
	t.Setenv("KSCORE_EVENTS_MAX_BYTES", "5368709120")
	t.Setenv("KSCORE_EVENTS_MAX_MESSAGES", "500000")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Events.Enabled {
		t.Error("Events.Enabled = true, want false")
	}
	if cfg.Events.Retention != 48*time.Hour {
		t.Errorf("Retention = %v, want 48h", cfg.Events.Retention)
	}
	if cfg.Events.MaxBytes != 5368709120 {
		t.Errorf("MaxBytes = %d, want 5368709120", cfg.Events.MaxBytes)
	}
	if cfg.Events.MaxMessages != 500000 {
		t.Errorf("MaxMessages = %d, want 500000", cfg.Events.MaxMessages)
	}
}

func TestLoadConfig_EnvOverrides_Authorization(t *testing.T) {
	t.Setenv("KSCORE_AUTHZ_ENABLED", "false")
	t.Setenv("KSCORE_AUTHZ_DEFAULT_DENY", "false")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Auth.Authorization.Enabled {
		t.Error("Authorization.Enabled = true, want false")
	}
	if cfg.Auth.Authorization.DefaultDeny {
		t.Error("Authorization.DefaultDeny = true, want false")
	}
}

// Kubernetes operator config tests (Epic 48)

func TestKubernetesOperatorConfig_Validate(t *testing.T) {
	t.Run("disabled_skips_validation", func(t *testing.T) {
		cfg := KubernetesOperatorConfig{Enabled: false}
		if err := cfg.Validate(); err != nil {
			t.Errorf("disabled config should not fail: %v", err)
		}
	})

	t.Run("valid_config", func(t *testing.T) {
		cfg := KubernetesOperatorConfig{
			Enabled:                 true,
			Namespace:               "kscore-system",
			LeaderElection:          true,
			LeaderElectionID:        "kscore-operator",
			ReconcileInterval:       1 * time.Minute,
			MaxConcurrentReconciles: 3,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("valid config failed: %v", err)
		}
	})

	t.Run("reconcile_interval_too_short", func(t *testing.T) {
		cfg := KubernetesOperatorConfig{
			Enabled:                 true,
			ReconcileInterval:       5 * time.Second,
			MaxConcurrentReconciles: 1,
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for short reconcile interval")
		}
	})

	t.Run("zero_max_concurrent", func(t *testing.T) {
		cfg := KubernetesOperatorConfig{
			Enabled:                 true,
			ReconcileInterval:       1 * time.Minute,
			MaxConcurrentReconciles: 0,
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for zero max concurrent")
		}
	})

	t.Run("leader_election_without_id", func(t *testing.T) {
		cfg := KubernetesOperatorConfig{
			Enabled:                 true,
			LeaderElection:          true,
			LeaderElectionID:        "",
			ReconcileInterval:       1 * time.Minute,
			MaxConcurrentReconciles: 1,
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for leader election without ID")
		}
	})
}

func TestLoadConfig_OperatorDefaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	if cfg.Operator.Enabled != DefaultOperatorEnabled {
		t.Errorf("Operator.Enabled = %v, want %v", cfg.Operator.Enabled, DefaultOperatorEnabled)
	}
	if cfg.Operator.LeaderElection != DefaultOperatorLeaderElection {
		t.Errorf("Operator.LeaderElection = %v, want %v", cfg.Operator.LeaderElection, DefaultOperatorLeaderElection)
	}
	if cfg.Operator.LeaderElectionID != DefaultOperatorLeaderElectionID {
		t.Errorf("Operator.LeaderElectionID = %q, want %q", cfg.Operator.LeaderElectionID, DefaultOperatorLeaderElectionID)
	}
	if cfg.Operator.ReconcileInterval != DefaultOperatorReconcileInterval {
		t.Errorf("Operator.ReconcileInterval = %v, want %v", cfg.Operator.ReconcileInterval, DefaultOperatorReconcileInterval)
	}
	if cfg.Operator.MaxConcurrentReconciles != DefaultOperatorMaxConcurrentReconciles {
		t.Errorf("Operator.MaxConcurrentReconciles = %d, want %d", cfg.Operator.MaxConcurrentReconciles, DefaultOperatorMaxConcurrentReconciles)
	}
}

func TestLoadConfig_OperatorEnvOverrides(t *testing.T) {
	t.Setenv("KSCORE_OPERATOR_ENABLED", "true")
	t.Setenv("KSCORE_OPERATOR_NAMESPACE", "test-ns")
	t.Setenv("KSCORE_OPERATOR_LEADER_ELECTION", "false")
	t.Setenv("KSCORE_OPERATOR_LEADER_ELECTION_ID", "custom-id")
	t.Setenv("KSCORE_OPERATOR_RECONCILE_INTERVAL", "5m")
	t.Setenv("KSCORE_OPERATOR_MAX_CONCURRENT_RECONCILES", "10")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !cfg.Operator.Enabled {
		t.Error("Operator.Enabled = false, want true")
	}
	if cfg.Operator.Namespace != "test-ns" {
		t.Errorf("Operator.Namespace = %q, want %q", cfg.Operator.Namespace, "test-ns")
	}
	if cfg.Operator.LeaderElection {
		t.Error("Operator.LeaderElection = true, want false")
	}
	if cfg.Operator.LeaderElectionID != "custom-id" {
		t.Errorf("Operator.LeaderElectionID = %q, want %q", cfg.Operator.LeaderElectionID, "custom-id")
	}
	if cfg.Operator.ReconcileInterval != 5*time.Minute {
		t.Errorf("Operator.ReconcileInterval = %v, want %v", cfg.Operator.ReconcileInterval, 5*time.Minute)
	}
	if cfg.Operator.MaxConcurrentReconciles != 10 {
		t.Errorf("Operator.MaxConcurrentReconciles = %d, want %d", cfg.Operator.MaxConcurrentReconciles, 10)
	}
}
