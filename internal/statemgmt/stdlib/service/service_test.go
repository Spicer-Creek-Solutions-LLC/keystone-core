// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider drives Module tests without touching systemctl.
type fakeProvider struct {
	svcs        map[string]ServiceInfo
	startErr    error
	stopErr     error
	enableErr   error
	disableErr  error
	startCalls  []string
	stopCalls   []string
	enableCalls []string
	disableCall []string
}

func newFake(seed ...ServiceInfo) *fakeProvider {
	f := &fakeProvider{svcs: map[string]ServiceInfo{}}
	for _, s := range seed {
		f.svcs[s.Name] = s
	}
	return f
}

func (f *fakeProvider) Lookup(name string) (*ServiceInfo, error) {
	s, ok := f.svcs[name]
	if !ok {
		return &ServiceInfo{Name: name, Exists: false}, nil
	}
	cp := s
	return &cp, nil
}
func (f *fakeProvider) Start(_ context.Context, name string) error {
	f.startCalls = append(f.startCalls, name)
	if f.startErr != nil {
		return f.startErr
	}
	s := f.svcs[name]
	s.Active = true
	f.svcs[name] = s
	return nil
}
func (f *fakeProvider) Stop(_ context.Context, name string) error {
	f.stopCalls = append(f.stopCalls, name)
	if f.stopErr != nil {
		return f.stopErr
	}
	s := f.svcs[name]
	s.Active = false
	f.svcs[name] = s
	return nil
}
func (f *fakeProvider) Enable(_ context.Context, name string) error {
	f.enableCalls = append(f.enableCalls, name)
	if f.enableErr != nil {
		return f.enableErr
	}
	s := f.svcs[name]
	s.Enabled = true
	f.svcs[name] = s
	return nil
}
func (f *fakeProvider) Disable(_ context.Context, name string) error {
	f.disableCall = append(f.disableCall, name)
	if f.disableErr != nil {
		return f.disableErr
	}
	s := f.svcs[name]
	s.Enabled = false
	f.svcs[name] = s
	return nil
}

func declFor(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID: "service:" + name, Module: "service", Name: name, State: state, Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- parseParams / validate ---------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("nginx", StateRunning, map[string]any{"enabel": true}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param error", err)
	}
}

func TestParseParams_EnableNotBool(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("nginx", StateRunning, map[string]any{"enable": "yes"}))
	if err == nil {
		t.Error("non-bool enable should be rejected")
	}
}

func TestParseParams_EnableSetVsUnset(t *testing.T) {
	t.Parallel()
	p1, _ := parseParams(declFor("nginx", StateRunning, nil))
	if p1.HasEnable {
		t.Error("HasEnable should be false when enable not set")
	}
	p2, _ := parseParams(declFor("nginx", StateRunning, map[string]any{"enable": false}))
	if !p2.HasEnable || p2.Enable {
		t.Errorf("enable:false → HasEnable=true, Enable=false; got %+v", p2)
	}
}

func TestValidate_BadUnitName(t *testing.T) {
	t.Parallel()
	bad := []string{"", "with spaces", "evil;rm -rf", "pipe|cat", "back`tick`"}
	for _, name := range bad {
		decl := &statemgmt.Declaration{ID: "service:" + name, Module: "service", Name: name, State: StateRunning}
		p, err := parseParams(decl)
		if err != nil {
			continue
		}
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestValidate_AcceptsCommonUnits(t *testing.T) {
	t.Parallel()
	good := []string{"nginx", "nginx.service", "getty@tty1.service", "system.slice", "docker.socket", "logrotate.timer"}
	for _, name := range good {
		decl := &statemgmt.Declaration{ID: "service:" + name, Module: "service", Name: name, State: StateRunning}
		p, err := parseParams(decl)
		if err != nil {
			t.Errorf("parse %q: %v", name, err)
			continue
		}
		if err := p.validate(); err != nil {
			t.Errorf("good name %q rejected: %v", name, err)
		}
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "service" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 2 {
		t.Errorf("ValidStates = %v", m.ValidStates())
	}
}

func TestModule_ImplementsOptionalInterfaces(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = &Module{}
	var _ statemgmt.DriftSeverityModule = &Module{}
}

func TestModule_DriftSeverity(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	// Diff mentioning "active" → HIGH.
	if got := m.DriftSeverity(declFor("nginx", StateRunning, nil), &statemgmt.ModuleCheckResult{Diff: "active false → true"}); got != statemgmt.DriftSeverityHigh {
		t.Errorf("active-diff severity = %v, want high", got)
	}
	// Diff mentioning only "enabled" → MEDIUM.
	if got := m.DriftSeverity(declFor("nginx", StateRunning, nil), &statemgmt.ModuleCheckResult{Diff: "enabled false → true"}); got != statemgmt.DriftSeverityMedium {
		t.Errorf("enable-diff severity = %v, want medium", got)
	}
	// No check → fall back to HIGH.
	if got := m.DriftSeverity(declFor("nginx", StateRunning, nil), nil); got != statemgmt.DriftSeverityHigh {
		t.Errorf("nil-check severity = %v, want high", got)
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	if New() == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewWithProvider_UsesInjected(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true})
	m := NewWithProvider(f)
	res, err := m.Check(context.Background(), declFor("nginx", StateRunning, nil))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.Matches {
		t.Errorf("seeded running service should match; diff = %q", res.Diff)
	}
}

// ---- Check / diffCheck -------------------------------------------

func TestCheck_UnitNotFound(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	_, err := m.Check(context.Background(), declFor("ghost", StateRunning, nil))
	if !errors.Is(err, ErrUnitNotFound) {
		t.Errorf("err = %v, want ErrUnitNotFound", err)
	}
}

func TestCheck_RunningActive(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, nil))
	if !res.Matches {
		t.Errorf("running + active should match; diff = %q", res.Diff)
	}
}

func TestCheck_RunningInactive(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, nil))
	if res.Matches {
		t.Error("running + inactive should drift")
	}
	if !strings.Contains(res.Diff, "active") {
		t.Errorf("diff should mention active; got %q", res.Diff)
	}
}

func TestCheck_StoppedActive(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateStopped, nil))
	if res.Matches {
		t.Error("stopped + active should drift")
	}
}

func TestCheck_StoppedInactive(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateStopped, nil))
	if !res.Matches {
		t.Errorf("stopped + inactive should match; diff = %q", res.Diff)
	}
}

func TestCheck_EnableMatch(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: true}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true}))
	if !res.Matches {
		t.Errorf("running+active + enable=true + enabled should match; diff = %q", res.Diff)
	}
}

func TestCheck_EnableMismatch(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: false}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true}))
	if res.Matches {
		t.Error("enable=true + disabled should drift")
	}
	if !strings.Contains(res.Diff, "enabled") {
		t.Errorf("diff should mention enabled; got %q", res.Diff)
	}
}

func TestCheck_EnableUnset_IgnoresBootState(t *testing.T) {
	t.Parallel()
	// Service is enabled at boot but the decl says nothing about
	// enable → only running-state compared → match.
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: true}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, nil))
	if !res.Matches {
		t.Errorf("enable-unset should ignore boot state; diff = %q", res.Diff)
	}
}

func TestCheck_BothAxesDrift(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false, Enabled: false}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true}))
	if res.Matches {
		t.Error("both axes drifting should report drift")
	}
	if !strings.Contains(res.Diff, "active") || !strings.Contains(res.Diff, "enabled") {
		t.Errorf("diff should cite both axes; got %q", res.Diff)
	}
}

// ---- Apply via fakeProvider --------------------------------------

func TestApply_UnitNotFound(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, err := m.Apply(context.Background(), declFor("ghost", StateRunning, nil))
	if !errors.Is(err, ErrUnitNotFound) {
		t.Errorf("err = %v, want ErrUnitNotFound", err)
	}
	if res.Success {
		t.Error("Success should be false on unit-not-found")
	}
}

func TestApply_RunningInactive_CallsStart(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false})
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("nginx", StateRunning, nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed should be true")
	}
	if len(f.startCalls) != 1 || f.startCalls[0] != "nginx" {
		t.Errorf("Start calls = %v, want [nginx]", f.startCalls)
	}
}

func TestApply_RunningActive_NoStart(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true})
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("nginx", StateRunning, nil))
	if res.Changed {
		t.Error("already-running should be Changed=false")
	}
	if len(f.startCalls) != 0 {
		t.Errorf("Start should not fire; got %v", f.startCalls)
	}
}

func TestApply_EnableOnly_CallsEnableNotStart(t *testing.T) {
	t.Parallel()
	// Service already running; only boot-enablement drifts.
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: false})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.enableCalls) != 1 {
		t.Errorf("Enable calls = %d, want 1", len(f.enableCalls))
	}
	if len(f.startCalls) != 0 {
		t.Errorf("Start should NOT fire when only enable drifted; got %v", f.startCalls)
	}
}

func TestApply_BothAxes_CallsStartAndEnable(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false, Enabled: false})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.startCalls) != 1 || len(f.enableCalls) != 1 {
		t.Errorf("expected Start+Enable each once; got start=%v enable=%v", f.startCalls, f.enableCalls)
	}
}

func TestApply_Stopped_CallsStop(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateStopped, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.stopCalls) != 1 {
		t.Errorf("Stop calls = %d, want 1", len(f.stopCalls))
	}
}

func TestApply_StoppedAndDisable_CallsStopAndDisable(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: true})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateStopped, map[string]any{"enable": false})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.stopCalls) != 1 || len(f.disableCall) != 1 {
		t.Errorf("expected Stop+Disable; got stop=%v disable=%v", f.stopCalls, f.disableCall)
	}
}

func TestApply_StoppedDisable_AlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false, Enabled: false})
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("nginx", StateStopped, map[string]any{"enable": false}))
	if res.Changed {
		t.Error("converged should be Changed=false")
	}
	total := len(f.startCalls) + len(f.stopCalls) + len(f.enableCalls) + len(f.disableCall)
	if total != 0 {
		t.Errorf("no provider calls expected; got %d", total)
	}
}

func TestApply_StartError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false})
	f.startErr = errors.New("systemctl: Failed to start nginx.service")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("nginx", StateRunning, nil))
	if err == nil || !strings.Contains(err.Error(), "Failed to start") {
		t.Errorf("err = %v, want provider error", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
}

func TestApply_EnableError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: false})
	f.enableErr = errors.New("systemctl: Failed to enable nginx.service")
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("nginx", StateRunning, map[string]any{"enable": true}))
	if err == nil || !strings.Contains(err.Error(), "Failed to enable") {
		t.Errorf("err = %v, want provider error", err)
	}
}

// ---- End-to-end --------------------------------------------------

func TestModule_EndToEnd_RunningLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: false, Enabled: false})
	m := newModuleWith(f)
	decl := declFor("nginx", StateRunning, map[string]any{"enable": true})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Apply")
	}
	res, _ := m.Apply(context.Background(), decl)
	if res.Changed {
		t.Error("re-Apply should be Changed=false")
	}
}

func TestModule_EndToEnd_StoppedLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake(ServiceInfo{Name: "nginx", Exists: true, Active: true, Enabled: true})
	m := newModuleWith(f)
	decl := declFor("nginx", StateStopped, map[string]any{"enable": false})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Apply")
	}
}

// ---- Validate wrapper + sentinels --------------------------------

func TestModule_Validate_PassesGood(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("nginx", StateRunning, map[string]any{"enable": true})); err != nil {
		t.Errorf("Validate should pass; got %v", err)
	}
}

func TestModule_Validate_RejectsBad(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("evil;rm", StateRunning, nil)); err == nil {
		t.Error("shell-metachar name should be rejected")
	}
}

func TestSentinels(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("IsUnsupportedOS sentinel mismatch")
	}
	if !IsNoBackend(ErrNoBackend) {
		t.Error("IsNoBackend sentinel mismatch")
	}
	if !IsUnitNotFound(ErrUnitNotFound) {
		t.Error("IsUnitNotFound sentinel mismatch")
	}
	if IsUnitNotFound(errors.New("other")) {
		t.Error("unrelated error matched IsUnitNotFound")
	}
}
