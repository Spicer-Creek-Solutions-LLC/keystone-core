package scenarios

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestBootstrapBlueprintCatalogDryRun(t *testing.T) {
	root, cfg, platforms, agentBin := requireBootstrapEnv(t)
	timeout := scenarioTimeout(cfg, "demo", 10*time.Minute)

	blueprints := []string{
		"kscore/demo",
		"kscore/monitoring-stack",
		"kscore/security-baseline",
		"kscore/metrics-only",
	}

	for _, platform := range platforms {
		platform := platform
		for _, blueprint := range blueprints {
			blueprint := blueprint
			t.Run(platform.Name+"-"+blueprint, func(t *testing.T) {
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
				ensureBlueprintDir(ctx, t, env)
				blueprintsDir := filepath.Join(root, "examples", "blueprints", "kscore")
				if err := env.CopyDir(ctx, blueprintsDir, "/etc/keystone-core/blueprints"); err != nil {
					t.Fatalf("failed to copy blueprints: %v", err)
				}

				result := execBootstrap(ctx, t, env,
					"--mode", "demo",
					"--apply-blueprint", blueprint,
					"--blueprints-dir", "/etc/keystone-core/blueprints",
					"--non-interactive",
					"--dry-run",
				)
				requireExecSuccess(t, result, "blueprint dry-run")
				requireOutputContains(t, result.Stdout+result.Stderr, "blueprint blueprints/"+blueprint+" loaded", "blueprint output")
			})
		}
	}
}
