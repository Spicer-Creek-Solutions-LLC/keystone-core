package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestProductionBootstrapSingleNodeDryRun(t *testing.T) {
	_, cfg, platforms, agentBin := requireBootstrapEnv(t)
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

			result := execBootstrap(ctx, t, env,
				"--mode", "production",
				"--cluster-name", "test-cluster",
				"--node-role", "control-plane",
				"--storage-backend", "postgres",
				"--postgres-host", "127.0.0.1",
				"--postgres-port", "5432",
				"--postgres-database", "kscore",
				"--postgres-user", "kscore",
				"--postgres-password", "testpass",
				"--non-interactive",
				"--dry-run",
			)
			requireExecSuccess(t, result, "production bootstrap dry-run")
			requireOutputContains(t, result.Stdout+result.Stderr, "bootstrap configuration validated", "production bootstrap output")
		})
	}
}

func TestProductionBootstrapJoinDryRun(t *testing.T) {
	_, cfg, platforms, agentBin := requireBootstrapEnv(t)
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

			result := execBootstrap(ctx, t, env,
				"--mode", "production",
				"--cluster-name", "test-cluster",
				"--node-role", "agent",
				"--join", "https://cp1:8443",
				"--join-token", "test-token",
				"--storage-backend", "postgres",
				"--postgres-host", "127.0.0.1",
				"--postgres-port", "5432",
				"--postgres-database", "kscore",
				"--postgres-user", "kscore",
				"--postgres-password", "testpass",
				"--non-interactive",
				"--dry-run",
			)
			requireExecSuccess(t, result, "production join dry-run")
			requireOutputContains(t, result.Stdout+result.Stderr, "bootstrap configuration validated", "production join output")
		})
	}
}
