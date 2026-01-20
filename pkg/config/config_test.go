package config

import (
	"os"
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
			name:        "single address",
			listenAddr:  "0.0.0.0",
			expected:    []string{"0.0.0.0"},
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
				cfg.Webhook.AuthType = "token"
			},
			errContains: "invalid webhook auth type",
		},
		{
			name: "missing HMAC secret",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
				cfg.Webhook.AuthType = "hmac"
			},
			errContains: "HMAC secret",
		},
		{
			name: "missing bearer token",
			config: func(cfg *Config) {
				cfg.Webhook.Enabled = true
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
				CAFile:  "/etc/kscore/ca.crt",
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
				cfg.Auth.JWT.PublicKeyFile = "/etc/kscore/jwt.pub"
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
		name         string
		config       NATSConfig
		wantField    string
		wantMessage  string
		wantHint     string
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

			natsErr, ok := err.(*NATSConfigError)
			if !ok {
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
