package scenarios

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestConfigLoads(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("failed to locate repo root: %v", err)
	}

	cfg, err := framework.LoadConfig(filepath.Join(root, "test", "bootstrap", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Platforms) == 0 {
		t.Fatal("expected at least one platform")
	}
	if len(cfg.Scenarios) == 0 {
		t.Fatal("expected at least one scenario")
	}
}

func TestConfigIncludesDemoScenario(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("failed to locate repo root: %v", err)
	}
	cfg, err := framework.LoadConfig(filepath.Join(root, "test", "bootstrap", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !hasScenario(cfg, "demo") {
		t.Fatal("expected demo scenario")
	}
	if !hasScenario(cfg, "production-single") {
		t.Fatal("expected production-single scenario")
	}
}

func hasScenario(cfg *framework.Config, name string) bool {
	for _, scenario := range cfg.Scenarios {
		if scenario.Name == name {
			return true
		}
	}
	return false
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", os.ErrNotExist
		}
		wd = parent
	}
}
