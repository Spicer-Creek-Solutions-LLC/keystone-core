// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ---- parseSystemctlShow ------------------------------------------

func TestParseSystemctlShow_RunningEnabled(t *testing.T) {
	t.Parallel()
	out := "LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n"
	info, err := parseSystemctlShow("nginx", out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !info.Exists || !info.Active || !info.Enabled {
		t.Errorf("info = %+v, want exists/active/enabled", info)
	}
}

func TestParseSystemctlShow_StoppedDisabled(t *testing.T) {
	t.Parallel()
	out := "LoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n"
	info, _ := parseSystemctlShow("nginx", out)
	if !info.Exists || info.Active || info.Enabled {
		t.Errorf("info = %+v, want exists/inactive/disabled", info)
	}
}

func TestParseSystemctlShow_NotFound(t *testing.T) {
	t.Parallel()
	out := "LoadState=not-found\nActiveState=inactive\nUnitFileState=\n"
	info, err := parseSystemctlShow("ghost", out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Exists {
		t.Error("not-found LoadState should mean Exists=false")
	}
}

func TestParseSystemctlShow_Masked(t *testing.T) {
	t.Parallel()
	out := "LoadState=masked\nActiveState=inactive\nUnitFileState=masked\n"
	info, _ := parseSystemctlShow("foo", out)
	if !info.Exists {
		t.Error("masked unit should still report Exists=true")
	}
	if info.Active || info.Enabled {
		t.Errorf("masked → not active / not enabled; got %+v", info)
	}
}

func TestParseSystemctlShow_Static(t *testing.T) {
	t.Parallel()
	// static units start via their dependent units; we treat them
	// as "enabled" for the boot-state compare.
	out := "LoadState=loaded\nActiveState=active\nUnitFileState=static\n"
	info, _ := parseSystemctlShow("dbus", out)
	if !info.Enabled {
		t.Error("static unit should be treated as enabled")
	}
}

func TestParseSystemctlShow_EmptyUnitFileState(t *testing.T) {
	t.Parallel()
	// Some units have no [Install] section → empty UnitFileState.
	out := "LoadState=loaded\nActiveState=active\nUnitFileState=\n"
	info, _ := parseSystemctlShow("ephemeral", out)
	if info.Enabled {
		t.Error("empty UnitFileState should mean not enabled")
	}
}

func TestParseSystemctlShow_FailedActiveState(t *testing.T) {
	t.Parallel()
	out := "LoadState=loaded\nActiveState=failed\nUnitFileState=enabled\n"
	info, _ := parseSystemctlShow("crashloop", out)
	if info.Active {
		t.Error("failed ActiveState should mean Active=false")
	}
	if !info.Enabled {
		t.Error("enabled should still be reported")
	}
}

func TestParseSystemctlShow_MalformedLine(t *testing.T) {
	t.Parallel()
	_, err := parseSystemctlShow("x", "LoadState=loaded\nthis-has-no-equals\n")
	if err == nil {
		t.Error("expected error on malformed line")
	}
}

func TestParseSystemctlShow_MissingLoadState(t *testing.T) {
	t.Parallel()
	_, err := parseSystemctlShow("x", "ActiveState=active\nUnitFileState=enabled\n")
	if err == nil {
		t.Error("expected error when LoadState absent")
	}
}

func TestParseSystemctlShow_UnknownLoadState(t *testing.T) {
	t.Parallel()
	_, err := parseSystemctlShow("x", "LoadState=banana\nActiveState=active\n")
	if err == nil {
		t.Error("expected error on unknown LoadState")
	}
}

func TestParseSystemctlShow_UnknownUnitFileState(t *testing.T) {
	t.Parallel()
	_, err := parseSystemctlShow("x", "LoadState=loaded\nActiveState=active\nUnitFileState=wat\n")
	if err == nil {
		t.Error("expected error on unknown UnitFileState")
	}
}

// ---- systemdProvider arg formation -------------------------------

type capturingRunner struct {
	calls [][]string // each entry: [bin, args...]
	err   error
}

func (c *capturingRunner) run(_ context.Context, bin string, args []string) error {
	c.calls = append(c.calls, append([]string{bin}, args...))
	return c.err
}

func newSystemdForTest(r commandRunner, l showLookupFn) *systemdProvider {
	return &systemdProvider{systemctl: "/usr/bin/systemctl", runner: r, showLookup: l}
}

func TestSystemdProvider_StartStopEnableDisable_ArgFormation(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newSystemdForTest(cr.run, nil)
	ctx := context.Background()
	if err := p.Start(ctx, "nginx"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(ctx, "nginx"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.Enable(ctx, "nginx"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if err := p.Disable(ctx, "nginx"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	want := [][]string{
		{"/usr/bin/systemctl", "start", "nginx"},
		{"/usr/bin/systemctl", "stop", "nginx"},
		{"/usr/bin/systemctl", "enable", "nginx"},
		{"/usr/bin/systemctl", "disable", "nginx"},
	}
	if len(cr.calls) != len(want) {
		t.Fatalf("calls = %d, want %d", len(cr.calls), len(want))
	}
	for i := range want {
		if !sliceEq(cr.calls[i], want[i]) {
			t.Errorf("call %d = %v, want %v", i, cr.calls[i], want[i])
		}
	}
}

func TestSystemdProvider_RunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{err: errors.New("systemctl: Failed to start nginx.service")}
	p := newSystemdForTest(cr.run, nil)
	err := p.Start(context.Background(), "nginx")
	if err == nil || !strings.Contains(err.Error(), "Failed to start") {
		t.Errorf("err = %v, want runner's error", err)
	}
}

func TestSystemdProvider_Lookup_DispatchesToParser(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, error) {
		return "LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n", nil
	}
	p := newSystemdForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists || !info.Active || !info.Enabled {
		t.Errorf("info = %+v", info)
	}
}

func TestSystemdProvider_Lookup_ShowError(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("systemctl: Failed to connect to bus")
	}
	p := newSystemdForTest(nil, fake)
	_, err := p.Lookup("nginx")
	if err == nil {
		t.Fatal("expected error from show failure")
	}
}

// ---- execRun + realShowLookup ------------------------------------

func TestExecRun_ExitError(t *testing.T) {
	t.Parallel()
	err := execRun(context.Background(), "/bin/false", nil)
	if err == nil {
		t.Fatal("expected exit-1 error")
	}
	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("err = %v, want \"exit\" in message", err)
	}
}

func TestExecRun_BinaryNotFound(t *testing.T) {
	t.Parallel()
	err := execRun(context.Background(), "/no/such/bin", nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRealShowLookup_BinaryNotFound(t *testing.T) {
	t.Parallel()
	_, err := realShowLookup(context.Background(), "/no/such/systemctl", "nginx")
	if err == nil {
		t.Fatal("expected lookup error")
	}
}

// ---- detect + undetectedProvider ---------------------------------

func TestDefaultProvider_ReturnsProvider(t *testing.T) {
	t.Parallel()
	if defaultProvider(defaultSystemdRunDir, defaultOpenrcRunDir) == nil {
		t.Fatal("defaultProvider returned nil")
	}
}

func TestUndetectedProvider_LookupNotExists(t *testing.T) {
	t.Parallel()
	info, err := (&undetectedProvider{}).Lookup("nginx")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Exists {
		t.Error("undetectedProvider should report Exists=false")
	}
}

func TestUndetectedProvider_MutatingOpsNoBackend(t *testing.T) {
	t.Parallel()
	p := &undetectedProvider{}
	ctx := context.Background()
	for _, fn := range []func() error{
		func() error { return p.Start(ctx, "nginx") },
		func() error { return p.Stop(ctx, "nginx") },
		func() error { return p.Enable(ctx, "nginx") },
		func() error { return p.Disable(ctx, "nginx") },
	} {
		if !errors.Is(fn(), ErrNoBackend) {
			t.Error("expected ErrNoBackend from mutating op")
		}
	}
}

func TestDetect_SystemdRunDirMissing_FallsBackToLookPath(t *testing.T) {
	t.Parallel()
	// Pass missing paths; defaultProvider should still try
	// LookPath("systemctl") then the OpenRC binaries. On a CI host
	// with systemctl installed it returns a systemdProvider; without,
	// undetected (or openrc if those binaries exist). Either way it's
	// a Provider.
	if defaultProvider("/no/such/run/systemd/system", "/no/such/run/openrc") == nil {
		t.Fatal("defaultProvider returned nil with missing run dir")
	}
}

// ---- helpers -----------------------------------------------------

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
