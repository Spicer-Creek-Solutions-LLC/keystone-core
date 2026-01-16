package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestDemoBootstrapIdempotent(t *testing.T) {
	_, cfg, platforms, agentBin := requireBootstrapEnv(t)
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

			first := execBootstrap(t, ctx, env,
				"--mode", "demo",
				"--non-interactive",
				"--dry-run",
			)
			requireExecSuccess(t, first, "demo bootstrap dry-run (first)")

			second := execBootstrap(t, ctx, env,
				"--mode", "demo",
				"--non-interactive",
				"--dry-run",
			)
			requireExecSuccess(t, second, "demo bootstrap dry-run (second)")
		})
	}
}
