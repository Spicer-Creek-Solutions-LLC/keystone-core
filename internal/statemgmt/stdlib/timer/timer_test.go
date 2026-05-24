// SPDX-License-Identifier: Apache-2.0

package timer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "systemd_timer:" + name,
		Module: "systemd_timer",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type fakeProvider struct {
	units    map[string]string // unit name → file content
	status   map[string]*TimerStatus
	reloads  int
	enables  []string
	disables []string
	writes   []string
	removes  []string

	readErr   error
	writeErr  error
	reloadErr error
	statusErr error
	enableErr error
}

func newFake() *fakeProvider {
	return &fakeProvider{units: map[string]string{}, status: map[string]*TimerStatus{}}
}
func (f *fakeProvider) ReadUnit(name string) (string, bool, error) {
	if f.readErr != nil {
		return "", false, f.readErr
	}
	c, ok := f.units[name]
	return c, ok, nil
}
func (f *fakeProvider) WriteUnit(name, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.units[name] = content
	f.writes = append(f.writes, name)
	return nil
}
func (f *fakeProvider) RemoveUnit(name string) error {
	delete(f.units, name)
	f.removes = append(f.removes, name)
	return nil
}
func (f *fakeProvider) DaemonReload(context.Context) error {
	if f.reloadErr != nil {
		return f.reloadErr
	}
	f.reloads++
	return nil
}
func (f *fakeProvider) Status(_ context.Context, name string) (*TimerStatus, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	if st, ok := f.status[name]; ok {
		return st, nil
	}
	return &TimerStatus{Exists: false}, nil
}
func (f *fakeProvider) EnableNow(_ context.Context, name string) error {
	if f.enableErr != nil {
		return f.enableErr
	}
	f.enables = append(f.enables, name)
	f.status[name] = &TimerStatus{Exists: true, Enabled: true, Active: true}
	return nil
}
func (f *fakeProvider) DisableStop(_ context.Context, name string) error {
	f.disables = append(f.disables, name)
	f.status[name] = &TimerStatus{Exists: true, Enabled: false, Active: false}
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("backup", StatePresent, map[string]any{"oncalendar": "daily"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_Defaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("backup", StatePresent, map[string]any{"on_calendar": "daily"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Service != "backup.service" || p.Enable != true || p.Persistent != false {
		t.Errorf("unexpected defaults: %+v", p)
	}
	if p.Description != "Keystone-managed timer backup" {
		t.Errorf("default description = %q", p.Description)
	}
}

func TestParse_TypeErrors(t *testing.T) {
	t.Parallel()
	for _, p := range []map[string]any{
		{"on_calendar": 1}, {"service": 2}, {"persistent": "yes"}, {"description": 3}, {"enable": "no"},
	} {
		if _, err := parseParams(decl("backup", StatePresent, p)); err == nil {
			t.Errorf("%v: expected type error", p)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok", decl("b", StatePresent, map[string]any{"on_calendar": "daily"}), false},
		{"present needs on_calendar", decl("b", StatePresent, nil), true},
		{"multiline on_calendar", decl("b", StatePresent, map[string]any{"on_calendar": "a\nb"}), true},
		{"multiline description", decl("b", StatePresent, map[string]any{"on_calendar": "daily", "description": "a\nb"}), true},
		{"bad service unit", decl("b", StatePresent, map[string]any{"on_calendar": "daily", "service": "a b"}), true},
		{"bad name", decl("a b", StatePresent, map[string]any{"on_calendar": "daily"}), true},
		{"absent ok", decl("b", StateAbsent, nil), false},
		{"absent rejects on_calendar", decl("b", StateAbsent, map[string]any{"on_calendar": "daily"}), true},
		{"absent rejects service", decl("b", StateAbsent, map[string]any{"service": "x.service"}), true},
		{"absent rejects persistent", decl("b", StateAbsent, map[string]any{"persistent": true}), true},
		{"absent rejects description", decl("b", StateAbsent, map[string]any{"description": "x"}), true},
		{"absent allows enable", decl("b", StateAbsent, map[string]any{"enable": false}), false},
		{"bad state", decl("b", "frob", nil), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// --- renderTimerUnit ---------------------------------------------------

func TestRenderTimerUnit(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(decl("backup", StatePresent, map[string]any{
		"on_calendar": "*-*-* 02:00:00", "persistent": true, "description": "Nightly backup",
	}))
	got := renderTimerUnit(p)
	for _, want := range []string{
		managedHeader, "[Unit]", "Description=Nightly backup",
		"[Timer]", "OnCalendar=*-*-* 02:00:00", "Persistent=true", "Unit=backup.service",
		"[Install]", "WantedBy=timers.target",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered unit missing %q\n---\n%s", want, got)
		}
	}
	// persistent omitted when false
	p2, _ := parseParams(decl("backup", StatePresent, map[string]any{"on_calendar": "daily"}))
	if strings.Contains(renderTimerUnit(p2), "Persistent=") {
		t.Errorf("Persistent= should be absent when persistent is false:\n%s", renderTimerUnit(p2))
	}
	// deterministic
	a, b := renderTimerUnit(p2), renderTimerUnit(p2)
	if a != b {
		t.Error("renderTimerUnit not deterministic")
	}
}

// --- Check / Apply -----------------------------------------------------

func presentDecl(name string, extra map[string]any) *statemgmt.Declaration {
	params := map[string]any{"on_calendar": "daily"}
	for k, v := range extra {
		params[k] = v
	}
	return decl(name, StatePresent, params)
}

func TestCheckApply_Present_FullCycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := presentDecl("backup", nil)

	// Check: missing → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("missing timer should drift")
	}

	// Apply: writes the unit, daemon-reloads, enables --now
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if f.units["backup.timer"] == "" {
		t.Fatal("unit file not written")
	}
	if f.reloads != 1 || len(f.enables) != 1 {
		t.Errorf("expected one reload + one enable; reloads=%d enables=%v", f.reloads, f.enables)
	}

	// Check: now matches
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged timer should match, diff=%q", r.Diff)
	}

	// Apply again: no-op (no extra reload/enable)
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("second apply should be a no-op")
	}
	if f.reloads != 1 || len(f.enables) != 1 {
		t.Errorf("no-op apply touched systemd: reloads=%d enables=%v", f.reloads, f.enables)
	}

	// content drift (persistent flag changes) → rewrite + reload
	d2 := presentDecl("backup", map[string]any{"persistent": true})
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Error("content change should drift")
	}
	sr, _ = m.Apply(context.Background(), d2)
	if !sr.Changed || f.reloads != 2 {
		t.Errorf("content change should rewrite + reload; changed=%v reloads=%d", sr.Changed, f.reloads)
	}
	if !strings.Contains(f.units["backup.timer"], "Persistent=true") {
		t.Error("rewritten unit missing Persistent=true")
	}
}

func TestCheckApply_Present_StateDriftOnly(t *testing.T) {
	t.Parallel()
	f := newFake()
	d := presentDecl("backup", nil)
	// Pre-seed the correct file but with the timer disabled+inactive.
	p, _ := parseParams(d)
	f.units["backup.timer"] = renderTimerUnit(p)
	f.status["backup.timer"] = &TimerStatus{Exists: true, Enabled: false, Active: false}
	m := NewWithProvider(f)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("file ok but timer disabled → should drift")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("enabling the timer should count as a change")
	}
	if len(f.writes) != 0 || f.reloads != 0 {
		t.Errorf("file was already correct; no write/reload expected: writes=%v reloads=%d", f.writes, f.reloads)
	}
	if len(f.enables) != 1 {
		t.Errorf("expected one enable --now, got %v", f.enables)
	}
}

func TestCheckApply_Present_EnableFalse(t *testing.T) {
	t.Parallel()
	f := newFake()
	d := presentDecl("backup", map[string]any{"enable": false})
	p, _ := parseParams(d)
	f.units["backup.timer"] = renderTimerUnit(p)
	// timer is currently enabled+active — drift, because enable:false
	f.status["backup.timer"] = &TimerStatus{Exists: true, Enabled: true, Active: true}
	m := NewWithProvider(f)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("enable:false but timer active → should drift")
	}
	sr, _ := m.Apply(context.Background(), d)
	if !sr.Changed || len(f.disables) != 1 {
		t.Errorf("expected disable --now; changed=%v disables=%v", sr.Changed, f.disables)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should converge after disable")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.units["old.timer"] = "whatever\n"
	f.status["old.timer"] = &TimerStatus{Exists: true, Enabled: true, Active: true}
	m := NewWithProvider(f)
	d := decl("old", StateAbsent, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("present timer should drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	if _, ok := f.units["old.timer"]; ok {
		t.Error("unit file not removed")
	}
	if len(f.disables) != 1 || f.reloads != 1 {
		t.Errorf("expected disable + reload on removal; disables=%v reloads=%d", f.disables, f.reloads)
	}

	// already absent → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a missing timer should be a no-op")
	}

	// Check on a missing timer → match
	r, _ = m.Check(context.Background(), decl("ghost", StateAbsent, nil))
	if !r.Matches {
		t.Error("absent on a missing timer should match")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	// write error
	f := newFake()
	f.writeErr = errors.New("disk full")
	if _, err := NewWithProvider(f).Apply(context.Background(), presentDecl("b", nil)); err == nil {
		t.Error("write error should propagate")
	}
	// reload error after a successful write
	f = newFake()
	f.reloadErr = errors.New("dbus down")
	if _, err := NewWithProvider(f).Apply(context.Background(), presentDecl("b", nil)); err == nil {
		t.Error("reload error should propagate")
	}
	// enable error
	f = newFake()
	f.enableErr = errors.New("masked unit")
	sr, err := NewWithProvider(f).Apply(context.Background(), presentDecl("b", nil))
	if err == nil {
		t.Fatal("enable error should propagate")
	}
	if sr == nil || sr.Success {
		t.Error("result should report failure")
	}
	// status error during Check
	f = newFake()
	p, _ := parseParams(presentDecl("b", nil))
	f.units["b.timer"] = renderTimerUnit(p)
	f.statusErr = errors.New("no pid 1 systemd")
	if _, err := NewWithProvider(f).Check(context.Background(), presentDecl("b", nil)); err == nil {
		t.Error("status error should propagate from Check")
	}
	// read error
	f = newFake()
	f.readErr = errors.New("eperm")
	if _, err := NewWithProvider(f).Check(context.Background(), presentDecl("b", nil)); err == nil {
		t.Error("read error should propagate from Check")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "systemd_timer" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("systemd_timer should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("b", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(presentDecl("b", nil), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(presentDecl("b", nil)); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("b", StatePresent, nil)); err == nil {
		t.Error("present-without-on_calendar should be rejected")
	}

	// Test() via a fake provider
	fm := NewWithProvider(newFake())
	ok, err := fm.Test(context.Background(), decl("ghost", StateAbsent, nil))
	if err != nil || !ok {
		t.Errorf("Test on absent-and-missing: ok=%v err=%v", ok, err)
	}
	ok, err = fm.Test(context.Background(), presentDecl("b", nil))
	if err != nil || ok {
		t.Errorf("Test on a not-yet-present timer should be false: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoBackend(ErrNoBackend) || IsNoBackend(errors.New("x")) {
		t.Error("IsNoBackend")
	}
}
