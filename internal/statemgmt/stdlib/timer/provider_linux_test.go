//go:build linux

package timer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withUnitDir points unitDir at a fresh tempdir for the duration of
// the test. Callers must NOT t.Parallel() — unitDir is a package var.
func withUnitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := unitDir
	unitDir = dir
	t.Cleanup(func() { unitDir = old })
	return dir
}

func TestSystemdProvider_UnitFileOps(t *testing.T) {
	dir := withUnitDir(t)
	// nested subdir so WriteUnit exercises mkdirAll
	unitDir = filepath.Join(dir, "systemd", "system")
	p := &systemdProvider{systemctl: "systemctl", run: func(context.Context, string, []string) (string, error) { return "", nil }}

	// missing → ("", false, nil)
	if c, ok, err := p.ReadUnit("backup.timer"); err != nil || ok || c != "" {
		t.Fatalf("missing: c=%q ok=%v err=%v", c, ok, err)
	}
	// write → read back
	if err := p.WriteUnit("backup.timer", "hello\n"); err != nil {
		t.Fatal(err)
	}
	if c, ok, err := p.ReadUnit("backup.timer"); err != nil || !ok || c != "hello\n" {
		t.Fatalf("after write: c=%q ok=%v err=%v", c, ok, err)
	}
	// the temp file should not linger
	if _, err := os.Stat(filepath.Join(unitDir, "backup.timer.keystone.tmp")); !os.IsNotExist(err) {
		t.Error("atomic-write temp file lingered")
	}
	// remove → gone; removing again is a no-op
	if err := p.RemoveUnit("backup.timer"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.ReadUnit("backup.timer"); ok {
		t.Error("file not removed")
	}
	if err := p.RemoveUnit("backup.timer"); err != nil {
		t.Errorf("remove of a missing unit should be a no-op, got %v", err)
	}
}

func TestParseShow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		out     string
		want    TimerStatus
		wantErr bool
	}{
		{"loaded enabled active", "LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n", TimerStatus{Exists: true, Enabled: true, Active: true}, false},
		{"loaded disabled inactive", "LoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n", TimerStatus{Exists: true, Enabled: false, Active: false}, false},
		{"static counts as enabled", "LoadState=loaded\nActiveState=active\nUnitFileState=static\n", TimerStatus{Exists: true, Enabled: true, Active: true}, false},
		{"empty unit-file-state", "LoadState=loaded\nActiveState=inactive\nUnitFileState=\n", TimerStatus{Exists: true, Enabled: false, Active: false}, false},
		{"not found", "LoadState=not-found\nActiveState=inactive\nUnitFileState=\n", TimerStatus{Exists: false}, false},
		{"masked", "LoadState=masked\nActiveState=inactive\nUnitFileState=masked\n", TimerStatus{Exists: true}, false},
		{"unknown loadstate", "LoadState=weird\n", TimerStatus{}, true},
		{"unknown unitfilestate", "LoadState=loaded\nActiveState=active\nUnitFileState=bogus\n", TimerStatus{}, true},
		{"missing loadstate", "ActiveState=active\n", TimerStatus{}, true},
		{"unparseable line", "LoadState=loaded\nthis has no equals\n", TimerStatus{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseShow("x.timer", tc.out)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if *got != tc.want {
				t.Errorf("got %+v want %+v", *got, tc.want)
			}
		})
	}
}

type recCall struct{ args []string }

func recRunner(out map[int]string) (commandRunner, *[]recCall) {
	var calls []recCall
	run := func(_ context.Context, _ string, args []string) (string, error) {
		i := len(calls)
		calls = append(calls, recCall{args: args})
		return out[i], nil
	}
	return run, &calls
}

func TestSystemdProvider_SystemctlArgs(t *testing.T) {
	t.Parallel()
	run, calls := recRunner(map[int]string{1: "LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n"})
	p := &systemdProvider{systemctl: "systemctl", run: run}

	if err := p.DaemonReload(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := p.Status(context.Background(), "backup.timer")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || !st.Active || !st.Exists {
		t.Errorf("Status parsed wrong: %+v", st)
	}
	if err := p.EnableNow(context.Background(), "backup.timer"); err != nil {
		t.Fatal(err)
	}
	if err := p.DisableStop(context.Background(), "backup.timer"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"daemon-reload",
		"show backup.timer -p LoadState -p ActiveState -p UnitFileState",
		"enable --now backup.timer",
		"disable --now backup.timer",
	}
	if len(*calls) != len(want) {
		t.Fatalf("got %d calls, want %d", len(*calls), len(want))
	}
	for i, w := range want {
		if got := strings.Join((*calls)[i].args, " "); got != w {
			t.Errorf("call %d args = %q, want %q", i, got, w)
		}
	}
}

func TestSystemdProvider_StatusErrorPropagates(t *testing.T) {
	t.Parallel()
	run := func(context.Context, string, []string) (string, error) { return "", errors.New("no pid 1 systemd") }
	p := &systemdProvider{systemctl: "systemctl", run: run}
	if _, err := p.Status(context.Background(), "x.timer"); err == nil {
		t.Error("Status should propagate a runner error")
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/systemctl", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("echo = %q", out)
	}
}

func TestNoSystemdProvider(t *testing.T) {
	t.Parallel()
	p := &noSystemdProvider{}
	if c, ok, err := p.ReadUnit("x.timer"); err != nil || ok || c != "" {
		t.Errorf("ReadUnit: c=%q ok=%v err=%v", c, ok, err)
	}
	if err := p.WriteUnit("x.timer", "y"); !errors.Is(err, ErrNoBackend) {
		t.Errorf("WriteUnit err = %v", err)
	}
	if err := p.RemoveUnit("x.timer"); !errors.Is(err, ErrNoBackend) {
		t.Errorf("RemoveUnit err = %v", err)
	}
	if err := p.DaemonReload(context.Background()); !errors.Is(err, ErrNoBackend) {
		t.Errorf("DaemonReload err = %v", err)
	}
	if _, err := p.Status(context.Background(), "x.timer"); !errors.Is(err, ErrNoBackend) {
		t.Errorf("Status err = %v", err)
	}
	if err := p.EnableNow(context.Background(), "x.timer"); !errors.Is(err, ErrNoBackend) {
		t.Errorf("EnableNow err = %v", err)
	}
	if err := p.DisableStop(context.Background(), "x.timer"); !errors.Is(err, ErrNoBackend) {
		t.Errorf("DisableStop err = %v", err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
