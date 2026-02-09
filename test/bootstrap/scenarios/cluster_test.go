package scenarios

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestProductionClusterDryRun(t *testing.T) {
	_, cfg, platforms, agentBin := requireBootstrapEnv(t)
	timeout := scenarioTimeout(cfg, "production-cluster", 20*time.Minute)
	nodeCount := scenarioNodes(cfg, "production-cluster", 3)

	for _, platform := range platforms {
		platform := platform
		t.Run(platform.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			for i := 0; i < nodeCount; i++ {
				env, err := framework.NewDockerEnvForPlatform(cfg, platform.Name, t.TempDir())
				if err != nil {
					t.Fatalf("failed to configure docker env: %v", err)
				}

				if err := env.Start(ctx); err != nil {
					t.Fatalf("failed to start docker env: %v", err)
				}

				if err := env.CopyFile(ctx, agentBin, "/usr/local/bin/kscore-agent"); err != nil {
					stopEnv(env)
					t.Fatalf("failed to copy agent binary: %v", err)
				}

				role := "control-plane"
				if i > 0 {
					role = "control-plane"
				}
				args := []string{
					"--mode", "production",
					"--cluster-name", "test-cluster",
					"--node-role", role,
					"--storage-backend", "postgres",
					"--postgres-host", "127.0.0.1",
					"--postgres-port", "5432",
					"--postgres-database", "kscore",
					"--postgres-user", "kscore",
					"--postgres-password", "testpass",
					"--non-interactive",
					"--dry-run",
				}
				if i > 0 {
					args = append(args,
						"--join", "https://cp1:8443",
						"--join-token", "test-token",
					)
				}

				label := fmt.Sprintf("production cluster dry-run node %d", i+1)
				result := execBootstrap(ctx, t, env, args...)
				requireExecSuccess(t, result, label)
				requireOutputContains(t, result.Stdout+result.Stderr, "bootstrap configuration validated", label)

				stopEnv(env)
			}
		})
	}
}

func stopEnv(env *framework.DockerEnv) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
	defer stopCancel()
	_ = env.Stop(stopCtx)
}
