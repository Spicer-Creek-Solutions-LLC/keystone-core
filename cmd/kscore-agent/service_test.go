// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent/systemd"
)

// runServiceInProcess executes the service subcommand in the
// current process with the given args + a stubbed Runner.
// Returns combined stdout + stderr (both go via the cobra
// streams) and the Execute error.
func runServiceInProcess(t *testing.T, ctx context.Context, args []string, fake *systemd.FakeRunner) (string, string, error) {
	t.Helper()
	prev := runnerFactory
	runnerFactory = func() systemd.Runner { return fake }
	t.Cleanup(func() { runnerFactory = prev })

	cmd := newCommand()
	cmd.SetContext(ctx)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestService_LinuxOnlyGuardOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux skip — guard fires on non-Linux only")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := runServiceInProcess(t, ctx, []string{
		"service", "install",
		"--config", cfgPath,
	}, systemd.NewFakeRunner())
	if err == nil || !strings.Contains(err.Error(), "Linux-only") {
		t.Errorf("err = %v, want Linux-only refusal", err)
	}
}

func TestServiceInstall_DryRun(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fake := systemd.NewFakeRunner()
	_, _, err := runServiceInProcess(t, ctx, []string{
		"service", "install",
		"--config", cfgPath,
		"--unit-dir", unitDir,
		"--dry-run",
	}, fake)
	if err != nil {
		t.Fatalf("dry-run install failed: %v", err)
	}
	if got := fake.CallNames(); len(got) != 0 {
		t.Errorf("dry-run fired systemctl: %v", got)
	}
}

func TestServiceInstall_HappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fake := systemd.NewFakeRunner()
	_, _, err := runServiceInProcess(t, ctx, []string{
		"service", "install",
		"--config", cfgPath,
		"--unit-dir", unitDir,
		"--enable",
	}, fake)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	got := fake.CallNames()
	want := []string{
		"systemctl daemon-reload",
		"systemctl enable keystone-core-agent.service",
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("calls[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestServiceUninstall_NoUnit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fake := systemd.NewFakeRunner()
	_, _, err := runServiceInProcess(t, ctx, []string{
		"service", "uninstall",
		"--config", cfgPath,
		"--unit-dir", unitDir,
	}, fake)
	if err != nil {
		t.Fatalf("uninstall on missing unit: %v", err)
	}
	if got := fake.CallNames(); len(got) != 0 {
		t.Errorf("uninstall on missing unit fired calls: %v", got)
	}
}

func TestServiceStatus_NoUnitNonZero(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fake := systemd.NewFakeRunner()
	stdout, _, err := runServiceInProcess(t, ctx, []string{
		"service", "status",
		"--config", cfgPath,
		"--unit-dir", unitDir,
	}, fake)
	if err == nil {
		t.Fatal("expected nonzero exit when no unit is installed")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("err = %v, want mention of not installed", err)
	}
	if !strings.Contains(stdout, "present: false") {
		t.Errorf("stdout missing 'present: false':\n%s", stdout)
	}
}

func TestServiceStatus_ActiveAndEnabled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()
	fake := systemd.NewFakeRunner()

	// Pre-install the unit so Status sees it present.
	if _, err := systemd.Install(context.Background(), systemd.Params{},
		systemd.Options{
			UnitDir:  unitDir,
			UnitName: systemd.DefaultUnitName,
			Runner:   fake,
			Logger:   discardLog(),
		}); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	fake.Calls = nil
	fake.Responses["systemctl is-active "+systemd.DefaultUnitName] = systemd.FakeResponse{Output: []byte("active\n")}
	fake.Responses["systemctl is-enabled "+systemd.DefaultUnitName] = systemd.FakeResponse{Output: []byte("enabled\n")}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, err := runServiceInProcess(t, ctx, []string{
		"service", "status",
		"--config", cfgPath,
		"--unit-dir", unitDir,
	}, fake)
	if err != nil {
		t.Errorf("status returned error on active+enabled: %v", err)
	}
	if !strings.Contains(stdout, "active:  active (true)") {
		t.Errorf("stdout missing active line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "enabled: enabled (true)") {
		t.Errorf("stdout missing enabled line:\n%s", stdout)
	}
}

func TestServiceInstall_EnvVarFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only subcommand")
	}
	tmp := t.TempDir()
	cfgPath := writeAgentConfig(t, tmp, "nats://127.0.0.1:4222")
	unitDir := t.TempDir()

	t.Setenv("KSCORE_SERVICE_ENABLE", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fake := systemd.NewFakeRunner()
	_, _, err := runServiceInProcess(t, ctx, []string{
		"service", "install",
		"--config", cfgPath,
		"--unit-dir", unitDir,
	}, fake)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	got := fake.CallNames()
	wantEnable := "systemctl enable keystone-core-agent.service"
	found := false
	for _, c := range got {
		if c == wantEnable {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("env-driven --enable didn't fire; calls = %v", got)
	}
}
