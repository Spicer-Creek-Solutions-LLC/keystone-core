package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDeploymentMode(t *testing.T) {
	mode, err := ParseDeploymentMode("production")
	if err != nil {
		t.Fatalf("ParseDeploymentMode returned error: %v", err)
	}
	if mode != DeploymentModeProduction {
		t.Fatalf("expected production, got %s", mode)
	}

	if _, err := ParseDeploymentMode("nope"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestDefaultDeploymentConfig(t *testing.T) {
	cfg, ok := DefaultDeploymentConfig(DeploymentModeDemo)
	if !ok {
		t.Fatal("expected demo config")
	}
	if cfg.StorageBackend != "sqlite" {
		t.Fatalf("expected sqlite storage, got %s", cfg.StorageBackend)
	}
}

func TestDetectModeFromPaths(t *testing.T) {
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "server.yaml")
	if err := os.WriteFile(marker, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	mode := DetectModeFromPaths([]string{marker})
	if mode != DeploymentModeProduction {
		t.Fatalf("expected production mode, got %s", mode)
	}

	mode = DetectModeFromPaths([]string{filepath.Join(tmpDir, "missing")})
	if mode != DeploymentModeDemo {
		t.Fatalf("expected demo mode, got %s", mode)
	}
}
