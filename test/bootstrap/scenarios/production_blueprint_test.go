package scenarios

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestProductionBootstrapWithBlueprintsDryRun(t *testing.T) {
	root, cfg, platforms, agentBin := requireBootstrapEnv(t)
	timeout := scenarioTimeout(cfg, "production-single", 15*time.Minute)

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
			ensureBlueprintDir(t, ctx, env)
			blueprintsDir := filepath.Join(root, "examples", "blueprints", "kscore")
			if err := env.CopyDir(ctx, blueprintsDir, "/etc/kscore/blueprints"); err != nil {
				t.Fatalf("failed to copy blueprints: %v", err)
			}

			result := execBootstrap(t, ctx, env,
				"--mode", "production",
				"--cluster-name", "test-cluster",
				"--node-role", "control-plane",
				"--storage-backend", "postgres",
				"--postgres-host", "127.0.0.1",
				"--postgres-port", "5432",
				"--postgres-database", "kscore",
				"--postgres-user", "kscore",
				"--postgres-password", "testpass",
				"--apply-blueprint", "kscore/production-cluster",
				"--blueprint-param", "kscore/production-cluster:control_plane_nodes=[cp1]",
				"--blueprint-param", "kscore/production-cluster:postgres_host=127.0.0.1",
				"--blueprint-param", "kscore/production-cluster:postgres_password=testpass",
				"--blueprints-dir", "/etc/kscore/blueprints",
				"--non-interactive",
				"--dry-run",
			)
			requireExecSuccess(t, result, "production bootstrap with blueprints")
			requireOutputContains(t, result.Stdout+result.Stderr, "blueprint blueprints/kscore/production-cluster loaded", "production blueprint output")
		})
	}
}
