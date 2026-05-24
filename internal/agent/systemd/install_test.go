// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func tmpInstallOpts(t *testing.T) Options {
	t.Helper()
	return Options{
		UnitDir:  t.TempDir(),
		UnitName: DefaultUnitName,
		Runner:   NewFakeRunner(),
		Logger:   quietLogger(),
	}
}

func TestInstall_FreshWrite(t *testing.T) {
	opts := tmpInstallOpts(t)
	res, err := Install(context.Background(), Params{}, opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Created {
		t.Error("Created = false, want true")
	}
	if res.Updated {
		t.Error("Updated = true, want false")
	}
	if !res.Reloaded {
		t.Error("Reloaded = false, want true")
	}
	if res.Enabled || res.Started {
		t.Errorf("Enabled=%v Started=%v, want both false (Options.Enable/Start unset)",
			res.Enabled, res.Started)
	}
	if _, err := os.Stat(res.UnitPath); err != nil {
		t.Errorf("unit file missing: %v", err)
	}

	fake := opts.Runner.(*FakeRunner)
	got := fake.CallNames()
	want := []string{"systemctl daemon-reload"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestInstall_EnableAndStartCombines(t *testing.T) {
	opts := tmpInstallOpts(t)
	opts.Enable = true
	opts.Start = true
	res, err := Install(context.Background(), Params{}, opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Enabled || !res.Started {
		t.Errorf("Enabled=%v Started=%v, want both true", res.Enabled, res.Started)
	}

	fake := opts.Runner.(*FakeRunner)
	got := fake.CallNames()
	want := []string{
		"systemctl daemon-reload",
		"systemctl enable --now keystone-core-agent.service",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestInstall_EnableOnly(t *testing.T) {
	opts := tmpInstallOpts(t)
	opts.Enable = true
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	got := fake.CallNames()
	want := []string{
		"systemctl daemon-reload",
		"systemctl enable keystone-core-agent.service",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestInstall_StartOnly(t *testing.T) {
	opts := tmpInstallOpts(t)
	opts.Start = true
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	got := fake.CallNames()
	want := []string{
		"systemctl daemon-reload",
		"systemctl start keystone-core-agent.service",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	// Reset call recorder for the second pass.
	fake := opts.Runner.(*FakeRunner)
	fake.Calls = nil

	res, err := Install(context.Background(), Params{}, opts)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if res.Created || res.Updated || res.Reloaded {
		t.Errorf("re-run should be no-op; got %+v", res)
	}
	if got := fake.CallNames(); len(got) != 0 {
		t.Errorf("re-run fired systemctl calls: %v", got)
	}
}

func TestInstall_DetectsContentChange(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	// Reset call recorder.
	opts.Runner.(*FakeRunner).Calls = nil

	// Mutate params (User+Group) → content changes → rewrite.
	res, err := Install(context.Background(), Params{
		User:  "keystone-core",
		Group: "keystone-core",
	}, opts)
	if err != nil {
		t.Fatalf("Install (changed): %v", err)
	}
	if res.Created {
		t.Error("Created = true, want false")
	}
	if !res.Updated {
		t.Error("Updated = false, want true")
	}
	if !res.Reloaded {
		t.Error("Reloaded = false, want true")
	}
}

func TestInstall_DryRun_NoSideEffects(t *testing.T) {
	opts := tmpInstallOpts(t)
	opts.DryRun = true
	opts.Enable = true
	res, err := Install(context.Background(), Params{}, opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(res.UnitPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry run wrote a file: %v", err)
	}
	if got := opts.Runner.(*FakeRunner).CallNames(); len(got) != 0 {
		t.Errorf("dry run fired systemctl calls: %v", got)
	}
}

func TestUninstall_FullCycle(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Reset call recorder.
	opts.Runner.(*FakeRunner).Calls = nil

	res, err := Uninstall(context.Background(), opts)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !res.Stopped || !res.Disabled || !res.Removed || !res.Reloaded {
		t.Errorf("res = %+v, want all four true", res)
	}
	if _, err := os.Stat(res.UnitPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unit file still present: %v", err)
	}

	fake := opts.Runner.(*FakeRunner)
	got := fake.CallNames()
	want := []string{
		"systemctl stop keystone-core-agent.service",
		"systemctl disable keystone-core-agent.service",
		"systemctl daemon-reload",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestUninstall_AlreadyGone(t *testing.T) {
	opts := tmpInstallOpts(t)
	res, err := Uninstall(context.Background(), opts)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !res.NoUnit {
		t.Errorf("NoUnit = false on missing unit; got %+v", res)
	}
	if got := opts.Runner.(*FakeRunner).CallNames(); len(got) != 0 {
		t.Errorf("Uninstall on missing unit fired calls: %v", got)
	}
}

func TestUninstall_TolerantOfStopDisableErrors(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	fake.Calls = nil
	// Stop + disable both error (e.g. service was never active) —
	// uninstall should still remove the unit + daemon-reload.
	fake.Responses["systemctl stop keystone-core-agent.service"] = FakeResponse{Err: errors.New("inactive")}
	fake.Responses["systemctl disable keystone-core-agent.service"] = FakeResponse{Err: errors.New("not enabled")}

	res, err := Uninstall(context.Background(), opts)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !res.Removed || !res.Reloaded {
		t.Errorf("res = %+v, want Removed+Reloaded even when stop/disable errored", res)
	}
	if res.Stopped || res.Disabled {
		t.Errorf("Stopped=%v Disabled=%v: should report false when systemctl errored",
			res.Stopped, res.Disabled)
	}
}

func TestStatus_NoUnitInstalled(t *testing.T) {
	opts := tmpInstallOpts(t)
	res, err := Status(context.Background(), opts)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.UnitPresent {
		t.Error("UnitPresent = true on missing unit")
	}
	if got := opts.Runner.(*FakeRunner).CallNames(); len(got) != 0 {
		t.Errorf("Status on missing unit fired calls: %v", got)
	}
}

func TestStatus_ActiveEnabled(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	fake.Calls = nil
	fake.Responses["systemctl is-active keystone-core-agent.service"] = FakeResponse{Output: []byte("active\n")}
	fake.Responses["systemctl is-enabled keystone-core-agent.service"] = FakeResponse{Output: []byte("enabled\n")}

	res, err := Status(context.Background(), opts)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.UnitPresent || !res.Active || !res.Enabled {
		t.Errorf("res = %+v", res)
	}
	if res.ActiveState != "active" || res.EnabledState != "enabled" {
		t.Errorf("ActiveState=%q EnabledState=%q", res.ActiveState, res.EnabledState)
	}
}

func TestStatus_InactiveDisabled(t *testing.T) {
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	fake.Calls = nil
	fake.Responses["systemctl is-active keystone-core-agent.service"] = FakeResponse{Output: []byte("inactive\n"), Err: errors.New("exit 3")}
	fake.Responses["systemctl is-enabled keystone-core-agent.service"] = FakeResponse{Output: []byte("disabled\n"), Err: errors.New("exit 1")}

	res, err := Status(context.Background(), opts)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.Active || res.Enabled {
		t.Errorf("res = %+v, want Active=false Enabled=false", res)
	}
	if res.ActiveState != "inactive" || res.EnabledState != "disabled" {
		t.Errorf("ActiveState=%q EnabledState=%q", res.ActiveState, res.EnabledState)
	}
}

func TestStatus_StaticEnabled(t *testing.T) {
	// systemctl is-enabled returns "static" for units without
	// [Install] sections; we treat that as "enabled enough".
	opts := tmpInstallOpts(t)
	if _, err := Install(context.Background(), Params{}, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake := opts.Runner.(*FakeRunner)
	fake.Calls = nil
	fake.Responses["systemctl is-active keystone-core-agent.service"] = FakeResponse{Output: []byte("active\n")}
	fake.Responses["systemctl is-enabled keystone-core-agent.service"] = FakeResponse{Output: []byte("static\n")}

	res, err := Status(context.Background(), opts)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.Enabled {
		t.Errorf("Enabled = false on 'static' state; got %+v", res)
	}
}

func TestAtomicWriteFile_NoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.service")
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" || (len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] != ".service" && e.Name() != "out.service") {
			// no .tmp.<pid>.<ts> leftover
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}
