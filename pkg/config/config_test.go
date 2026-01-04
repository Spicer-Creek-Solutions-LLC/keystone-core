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
	// Valid addresses
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
