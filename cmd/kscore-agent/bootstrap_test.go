// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

// listenLocal binds an ephemeral TCP port for the
// validator/verifier's dial probes. Returns "host:port" and a
// cleanup func.
func listenLocal(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// runBootstrapInProcess executes the bootstrap subcommand in
// the current process with the given args. cfgPath points at a
// minimal kscore-agent.yaml the daemon-side config loader can
// parse. Returns combined stderr (where slog writes) for
// diagnostic asserts.
func runBootstrapInProcess(t *testing.T, ctx context.Context, args []string) (string, error) {
	t.Helper()
	cmd := newCommand()
	cmd.SetContext(ctx)
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{}) // stdout would be the TUI canvas; we never reach it
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stderr.String(), err
}

// writeAgentConfig drops a config.LoadValidate-able YAML into
// tmpDir. Bootstrap loads this for daemon-side defaults
// (logging, NATS cluster name fallback) — the bootstrap subcmd
// doesn't actually need NATS to be running, just for the
// loader to accept the file.
func writeAgentConfig(t *testing.T, tmpDir, joinURL string) string {
	t.Helper()
	path := filepath.Join(tmpDir, "agent.yaml")
	body := `mode: development
agent:
  id: ci-agent
nats:
  mode: external
  clustername: ci
  urls:
    - ` + joinURL + `
logging:
  level: error
  format: text
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestBootstrap_NonInteractiveHappyPath(t *testing.T) {
	addr, stop := listenLocal(t)
	defer stop()
	joinURL := "nats://" + addr

	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, joinURL)
	out := filepath.Join(tmp, "rendered-agent.yaml")
	statePath := filepath.Join(tmp, "bootstrap.json")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stderr, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--mode", "demo",
		"--cluster-name", "ci",
		"--agent-id", "agent-ci-1",
		"--node-role", "worker",
		"--join", joinURL,
		"--config-path", out,
		"--state-path", statePath,
	})
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nstderr:\n%s", err, stderr)
	}

	// Rendered agent config exists + parses.
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("rendered config missing: %v", err)
	}
	if _, err := config.Load(out); err != nil {
		t.Errorf("rendered config doesn't load: %v", err)
	}
	// State file was written and is loadable.
	if _, err := os.Stat(statePath); err != nil {
		t.Errorf("state file missing: %v", err)
	}
}

func TestBootstrap_NonInteractiveRejectsMissingMode(t *testing.T) {
	tmp := t.TempDir()
	addr, stop := listenLocal(t)
	defer stop()
	cfgPath := writeAgentConfig(t, tmp, "nats://"+addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--cluster-name", "ci",
		"--join", "nats://" + addr,
	})
	if err == nil {
		t.Fatal("expected error from missing --mode")
	}
	if !strings.Contains(err.Error(), "--mode") {
		t.Errorf("err = %v, want mention of --mode", err)
	}
}

func TestBootstrap_NonInteractiveRejectsMissingJoin(t *testing.T) {
	tmp := t.TempDir()
	addr, stop := listenLocal(t)
	defer stop()
	cfgPath := writeAgentConfig(t, tmp, "nats://"+addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--mode", "demo",
		"--cluster-name", "ci",
		// We have to override --join from the daemon config
		// since cfgPath sets it. Pass empty explicitly.
		"--join", "",
	})
	if err == nil {
		t.Fatal("expected error from missing --join")
	}
	if !strings.Contains(err.Error(), "--join") {
		t.Errorf("err = %v, want mention of --join", err)
	}
}

func TestBootstrap_NonInteractiveRejectsProductionMode(t *testing.T) {
	tmp := t.TempDir()
	addr, stop := listenLocal(t)
	defer stop()
	cfgPath := writeAgentConfig(t, tmp, "nats://"+addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--mode", "production",
		"--cluster-name", "ci",
		"--agent-id", "x",
		"--join", "nats://" + addr,
		"--config-path", filepath.Join(tmp, "out.yaml"),
	})
	if err == nil {
		t.Fatal("expected v1.x deferral error for production mode")
	}
	if !strings.Contains(err.Error(), "v1.x") {
		t.Errorf("err = %v, want it to mention v1.x deferral", err)
	}
}

func TestBootstrap_EnvVarFallback_RequiresMode(t *testing.T) {
	tmp := t.TempDir()
	addr, stop := listenLocal(t)
	defer stop()
	cfgPath := writeAgentConfig(t, tmp, "nats://"+addr)

	t.Setenv("KSCORE_BOOTSTRAP_MODE", "production")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Mode env reads through; production rejected by ValidateForV10.
	_, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--cluster-name", "ci",
		"--agent-id", "x",
		"--join", "nats://" + addr,
		"--config-path", filepath.Join(tmp, "out.yaml"),
	})
	if err == nil {
		t.Fatal("expected error: production mode (via env) is deferred")
	}
	if !strings.Contains(err.Error(), "v1.x") {
		t.Errorf("err = %v, want v1.x deferral", err)
	}
}

func TestBootstrap_FlagBeatsEnv(t *testing.T) {
	tmp := t.TempDir()
	addr, stop := listenLocal(t)
	defer stop()
	cfgPath := writeAgentConfig(t, tmp, "nats://"+addr)

	// Env says production (would reject); flag says demo.
	// Flag wins → ValidateForV10 accepts.
	t.Setenv("KSCORE_BOOTSTRAP_MODE", "production")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out := filepath.Join(tmp, "out.yaml")
	statePath := filepath.Join(tmp, "bootstrap.json")
	_, err := runBootstrapInProcess(t, ctx, []string{
		"bootstrap",
		"--config", cfgPath,
		"--non-interactive",
		"--mode", "demo", // wins over env
		"--cluster-name", "ci",
		"--agent-id", "x",
		"--join", "nats://" + addr,
		"--config-path", out,
		"--state-path", statePath,
	})
	if err != nil {
		t.Fatalf("err = %v, want nil (flag --mode demo should override env production)", err)
	}
}
