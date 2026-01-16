package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectExistingInstall(t *testing.T) {
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "server.yaml")
	if err := os.WriteFile(marker, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	if !DetectExistingInstall([]string{marker}) {
		t.Fatal("expected existing install detection")
	}

	if DetectExistingInstall([]string{filepath.Join(tmpDir, "missing")}) {
		t.Fatal("expected no install detection")
	}
}

func TestDetectResources(t *testing.T) {
	info, err := detectResources()
	if err != nil {
		t.Fatalf("detectResources returned error: %v", err)
	}
	if info.CPUCount <= 0 {
		t.Fatalf("expected CPU count > 0, got %d", info.CPUCount)
	}
	if info.MemoryTotalMB == 0 {
		t.Fatal("expected memory total > 0")
	}
}
