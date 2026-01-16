package vm

import (
	"context"
	"os"
	"testing"
	"time"
)

const envVMTests = "KSCORE_VM_TESTS"

// RunVMTests loads provider config and runs VM scenarios.
func RunVMTests(t *testing.T, configPath string, scenarios []func(*testing.T, Provider, *Config)) {
	t.Helper()

	if os.Getenv(envVMTests) != "1" {
		t.Skipf("VM tests disabled (set %s=1)", envVMTests)
	}

	provider, cfg, err := LoadProvider(configPath)
	if err != nil {
		t.Fatalf("load vm provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	if err := provider.Setup(ctx); err != nil {
		t.Fatalf("vm provider setup: %v", err)
	}
	defer func() {
		_ = provider.Cleanup(ctx)
	}()

	for _, scenario := range scenarios {
		scenario(t, provider, cfg)
	}
}
