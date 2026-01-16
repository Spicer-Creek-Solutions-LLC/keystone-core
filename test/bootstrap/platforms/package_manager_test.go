package platforms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestPackagePlanIncludesCorePackages(t *testing.T) {
	cfg, platforms, agentBin := requireBootstrapEnv(t)
	timeout := scenarioTimeout(cfg, "demo", 10*time.Minute)

	for _, platform := range platforms {
		platform := platform
		t.Run(platform.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			env, err := framework.NewDockerEnvForPlatform(cfg, platform.Name, t.TempDir())
			if err != nil {
				t.Fatalf("failed to configure docker env: %v", err)
			}

			if err := env.Start(ctx); err != nil {
				t.Fatalf("failed to start docker env: %v", err)
			}
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
				defer stopCancel()
				_ = env.Stop(stopCtx)
			}()

			if err := env.CopyFile(ctx, agentBin, "/usr/local/bin/kscore-agent"); err != nil {
				t.Fatalf("failed to copy agent binary: %v", err)
			}

			result := env.Exec(ctx, "/usr/local/bin/kscore-agent", "bootstrap",
				"--mode", "demo",
				"--non-interactive",
				"--dry-run",
			)
			if result.Error != nil || result.ExitCode != 0 {
				t.Fatalf("bootstrap dry-run failed (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
					result.ExitCode, result.Error, result.Stdout, result.Stderr)
			}

			output := result.Stdout + result.Stderr
			if !strings.Contains(output, "- kscore-server") || !strings.Contains(output, "- kscore-agent") {
				t.Fatalf("expected core packages missing from plan for %s\noutput:\n%s", platform.Name, output)
			}
		})
	}
}
