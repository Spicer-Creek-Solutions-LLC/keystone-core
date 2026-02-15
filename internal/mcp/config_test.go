package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.yaml")

	content := `
server:
  address: "localhost:9443"
auth:
  method: apikey
  api_key: "test-key-123"
features:
  max_target_count: 50
  default_profile: read_only
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Server.Address != "localhost:9443" {
		t.Errorf("Address = %q, want localhost:9443", cfg.Server.Address)
	}
	if cfg.Auth.Method != "apikey" {
		t.Errorf("Method = %q, want apikey", cfg.Auth.Method)
	}
	if cfg.Auth.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, want test-key-123", cfg.Auth.APIKey)
	}
	if cfg.Features.MaxTargetCount != 50 {
		t.Errorf("MaxTargetCount = %d, want 50", cfg.Features.MaxTargetCount)
	}
	if cfg.Profile() != ProfileReadOnly {
		t.Errorf("Profile() = %q, want read_only", cfg.Profile())
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/mcp.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     MCPConfig
		wantErr bool
	}{
		{
			name: "valid apikey",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "apikey", APIKey: "key"},
			},
		},
		{
			name: "valid jwt",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "jwt", JWTToken: "token"},
			},
		},
		{
			name: "valid mtls",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "mtls", TLSCert: "/cert.pem", TLSKey: "/key.pem"},
			},
		},
		{
			name: "missing address",
			cfg: MCPConfig{
				Auth: AuthConfig{Method: "apikey", APIKey: "key"},
			},
			wantErr: true,
		},
		{
			name: "missing method",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
			},
			wantErr: true,
		},
		{
			name: "apikey without key",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "apikey"},
			},
			wantErr: true,
		},
		{
			name: "jwt without token",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "jwt"},
			},
			wantErr: true,
		},
		{
			name: "mtls without cert",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "mtls", TLSKey: "/key.pem"},
			},
			wantErr: true,
		},
		{
			name: "unknown method",
			cfg: MCPConfig{
				Server: ServerConfig{Address: "localhost:9443"},
				Auth:   AuthConfig{Method: "magic"},
			},
			wantErr: true,
		},
		{
			name: "bad profile",
			cfg: MCPConfig{
				Server:   ServerConfig{Address: "localhost:9443"},
				Auth:     AuthConfig{Method: "apikey", APIKey: "key"},
				Features: FeaturesConfig{DefaultProfile: "superadmin"},
			},
			wantErr: true,
		},
		{
			name: "negative max_target_count",
			cfg: MCPConfig{
				Server:   ServerConfig{Address: "localhost:9443"},
				Auth:     AuthConfig{Method: "apikey", APIKey: "key"},
				Features: FeaturesConfig{MaxTargetCount: -1},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProfile_Default(t *testing.T) {
	cfg := &MCPConfig{
		Server: ServerConfig{Address: "localhost:9443"},
		Auth:   AuthConfig{Method: "apikey", APIKey: "key"},
	}
	if cfg.Profile() != ProfileOpsSafe {
		t.Errorf("default Profile() = %q, want ops_safe", cfg.Profile())
	}
}
