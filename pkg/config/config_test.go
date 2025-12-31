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
