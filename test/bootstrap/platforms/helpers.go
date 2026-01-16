package platforms

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

const (
	envBootstrapTests   = "KSCORE_BOOTSTRAP_TESTS"
	envPlatformFilter   = "KSCORE_TEST_PLATFORM"
	envAgentBinary      = "KSCORE_BOOTSTRAP_AGENT_BIN"
	defaultAgentRelPath = "build/bin"
)

func requireBootstrapEnv(t *testing.T) (*framework.Config, []framework.Platform, string) {
	t.Helper()

	if os.Getenv(envBootstrapTests) != "1" {
		t.Skipf("skipping bootstrap tests (set %s=1)", envBootstrapTests)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available in PATH")
	}

	root, err := framework.RepoRoot()
	if err != nil {
		t.Fatalf("failed to locate repo root: %v", err)
	}

	cfg, err := framework.LoadConfig(filepath.Join(root, "test", "bootstrap", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	platforms, err := cfg.EnabledPlatforms(os.Getenv(envPlatformFilter))
	if err != nil {
		t.Fatalf("failed to select platforms: %v", err)
	}

	agentBin := resolveAgentBinary(root)
	if agentBin == "" {
		t.Skipf("kscore-agent binary not found (set %s or run make agent)", envAgentBinary)
	}

	return cfg, platforms, agentBin
}

func resolveAgentBinary(root string) string {
	if bin := os.Getenv(envAgentBinary); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
		return ""
	}

	path := filepath.Join(root, defaultAgentRelPath, runtime.GOOS, runtime.GOARCH, "kscore-agent")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func scenarioTimeout(cfg *framework.Config, name string, fallback time.Duration) time.Duration {
	for _, scenario := range cfg.Scenarios {
		if scenario.Name == name && scenario.Timeout != "" {
			if parsed, err := time.ParseDuration(scenario.Timeout); err == nil {
				return parsed
			}
		}
	}
	return fallback
}
