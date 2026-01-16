package platforms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/framework"
)

func TestBootstrapPlatformMatrixDryRun(t *testing.T) {
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

			if !strings.Contains(result.Stdout+result.Stderr, "package installation plan") {
				t.Fatalf("missing package installation plan output for %s", platform.Name)
			}

			if expected := expectedPackageManager(platform.Name); expected != "" {
				if !strings.Contains(result.Stdout+result.Stderr, "package manager: "+expected) {
					t.Fatalf("expected package manager %q not found for %s\noutput:\n%s",
						expected, platform.Name, result.Stdout+result.Stderr)
				}
			}
			if expected := expectedInitSystem(platform.Name); expected != "" {
				if !strings.Contains(result.Stdout+result.Stderr, "init system: "+expected) {
					t.Fatalf("expected init system %q not found for %s\noutput:\n%s",
						expected, platform.Name, result.Stdout+result.Stderr)
				}
			}
		})
	}
}

func expectedPackageManager(platformName string) string {
	switch {
	case strings.HasPrefix(platformName, "ubuntu"),
		strings.HasPrefix(platformName, "debian"):
		return "apt"
	case strings.HasPrefix(platformName, "rhel"),
		strings.HasPrefix(platformName, "rocky"),
		strings.HasPrefix(platformName, "fedora"):
		return "dnf"
	case strings.HasPrefix(platformName, "alpine"):
		return "apk"
	default:
		return ""
	}
}

func expectedInitSystem(platformName string) string {
	switch {
	case strings.HasPrefix(platformName, "alpine"):
		return "unknown"
	case strings.HasPrefix(platformName, "ubuntu"),
		strings.HasPrefix(platformName, "debian"),
		strings.HasPrefix(platformName, "rhel"),
		strings.HasPrefix(platformName, "rocky"),
		strings.HasPrefix(platformName, "fedora"):
		return "systemd"
	default:
		return ""
	}
}
