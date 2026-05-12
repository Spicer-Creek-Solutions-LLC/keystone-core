package kmod

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider drives Module tests without touching /proc/modules
// or modprobe.
type fakeProvider struct {
	loaded        map[string]bool
	persist       map[string]bool
	loadErr       error
	unloadErr     error
	addErr        error
	removeErr     error
	loadCalls     []string
	unloadCalls   []string
	addCalls      []string
	removeCalls   []string
}

func newFake() *fakeProvider {
	return &fakeProvider{loaded: map[string]bool{}, persist: map[string]bool{}}
}

func (f *fakeProvider) Loaded(name string) (bool, error) { return f.loaded[normalizeName(name)], nil }
func (f *fakeProvider) Load(_ context.Context, name string) error {
	f.loadCalls = append(f.loadCalls, name)
	if f.loadErr != nil {
		return f.loadErr
	}
	f.loaded[normalizeName(name)] = true
	return nil
}
func (f *fakeProvider) Unload(_ context.Context, name string) error {
	f.unloadCalls = append(f.unloadCalls, name)
	if f.unloadErr != nil {
		return f.unloadErr
	}
	delete(f.loaded, normalizeName(name))
	return nil
}
func (f *fakeProvider) PersistExists(name string) (bool, error) {
	return f.persist[normalizeName(name)], nil
}
func (f *fakeProvider) AddPersist(name string) error {
	f.addCalls = append(f.addCalls, name)
	if f.addErr != nil {
		return f.addErr
	}
	f.persist[normalizeName(name)] = true
	return nil
}
func (f *fakeProvider) RemovePersist(name string) error {
	f.removeCalls = append(f.removeCalls, name)
	if f.removeErr != nil {
		return f.removeErr
	}
	delete(f.persist, normalizeName(name))
	return nil
}

func declFor(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID: "kernel_module:" + name, Module: "kernel_module", Name: name, State: state, Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- params / validate -------------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("br_netfilter", StatePresent, map[string]any{"persit": true}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestParseParams_NormalizesDashName(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("br-netfilter", StatePresent, nil))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Name != "br_netfilter" {
		t.Errorf("Name = %q, want underscored form", p.Name)
	}
}

func TestParseParams_PersistDefaultsTrue(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("overlay", StatePresent, nil))
	if !p.Persist {
		t.Error("persist should default to true")
	}
}

func TestParseParams_PersistNotBool(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("overlay", StatePresent, map[string]any{"persist": "yes"}))
	if err == nil {
		t.Error("non-bool persist should be rejected")
	}
}

func TestValidate_BadModuleName(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "evil;rm", "with space", "pipe|cat"} {
		p, _ := parseParams(declFor(bad, StatePresent, nil))
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "kernel_module" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 2 {
		t.Errorf("ValidStates = %v", m.ValidStates())
	}
}

func TestModule_Interfaces(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = &Module{}
	var _ statemgmt.DriftSeverityModule = &Module{}
}

func TestModule_DriftSeverity(t *testing.T) {
	t.Parallel()
	m := &Module{}
	// "loaded" in diff → HIGH.
	if got := m.DriftSeverity(declFor("x", StatePresent, nil), &statemgmt.ModuleCheckResult{Diff: "not loaded; want loaded"}); got != statemgmt.DriftSeverityHigh {
		t.Errorf("loaded-diff severity = %v, want high", got)
	}
	// only persist drift → MEDIUM.
	if got := m.DriftSeverity(declFor("x", StatePresent, nil), &statemgmt.ModuleCheckResult{Diff: "persist entry missing"}); got != statemgmt.DriftSeverityMedium {
		t.Errorf("persist-diff severity = %v, want medium", got)
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	if New() == nil {
		t.Fatal("nil")
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_PresentNotLoaded(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if res.Matches {
		t.Error("not-loaded should drift for state=present")
	}
	if !strings.Contains(res.Diff, "not loaded") {
		t.Errorf("diff should mention not-loaded; got %q", res.Diff)
	}
}

func TestCheck_PresentLoadedAndPersisted(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	f.persist["br_netfilter"] = true
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if !res.Matches {
		t.Errorf("loaded+persisted should match; diff = %q", res.Diff)
	}
}

func TestCheck_PresentLoadedButNotPersisted(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if res.Matches {
		t.Error("loaded but no persist entry should drift")
	}
	if !strings.Contains(res.Diff, "persist entry missing") {
		t.Errorf("diff should mention persist; got %q", res.Diff)
	}
}

func TestCheck_PresentPersistFalse_IgnoresEntry(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StatePresent, map[string]any{"persist": false}))
	if !res.Matches {
		t.Errorf("persist:false should match on loaded alone; diff = %q", res.Diff)
	}
}

func TestCheck_AbsentNotLoaded(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StateAbsent, nil))
	if !res.Matches {
		t.Error("absent + not-loaded should match")
	}
}

func TestCheck_AbsentLoaded(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StateAbsent, nil))
	if res.Matches {
		t.Error("absent + loaded should drift")
	}
}

func TestCheck_AbsentPersistEntryPresent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.persist["br_netfilter"] = true // not loaded but auto-load entry exists
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("br_netfilter", StateAbsent, nil))
	if res.Matches {
		t.Error("absent + persist entry present should drift (would reload at boot)")
	}
	if !strings.Contains(res.Diff, "would reload at boot") {
		t.Errorf("diff should explain the persist concern; got %q", res.Diff)
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_PresentNotLoaded_CallsLoadAndAddPersist(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed should be true")
	}
	if len(f.loadCalls) != 1 || len(f.addCalls) != 1 {
		t.Errorf("expected Load+AddPersist; got load=%v add=%v", f.loadCalls, f.addCalls)
	}
}

func TestApply_PresentLoadedNoPersist_OnlyAddPersist(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.loadCalls) != 0 {
		t.Errorf("Load should not fire when already loaded; got %v", f.loadCalls)
	}
	if len(f.addCalls) != 1 {
		t.Errorf("AddPersist should fire; got %v", f.addCalls)
	}
}

func TestApply_PersistFalse_NoPersistCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, map[string]any{"persist": false})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.addCalls) != 0 {
		t.Errorf("AddPersist should not fire with persist:false; got %v", f.addCalls)
	}
	if len(f.loadCalls) != 1 {
		t.Errorf("Load should still fire; got %v", f.loadCalls)
	}
}

func TestApply_AlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	f.persist["br_netfilter"] = true
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if res.Changed {
		t.Error("converged should be Changed=false")
	}
	if len(f.loadCalls)+len(f.addCalls)+len(f.unloadCalls)+len(f.removeCalls) != 0 {
		t.Error("no provider calls on converged")
	}
}

func TestApply_AbsentLoaded_CallsUnloadAndRemovePersist(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	f.persist["br_netfilter"] = true
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("br_netfilter", StateAbsent, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.unloadCalls) != 1 || len(f.removeCalls) != 1 {
		t.Errorf("expected Unload+RemovePersist; got unload=%v remove=%v", f.unloadCalls, f.removeCalls)
	}
}

func TestApply_AbsentNotLoadedButPersistEntry_OnlyRemove(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.persist["br_netfilter"] = true
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("br_netfilter", StateAbsent, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.unloadCalls) != 0 {
		t.Errorf("Unload should not fire when not loaded; got %v", f.unloadCalls)
	}
	if len(f.removeCalls) != 1 {
		t.Errorf("RemovePersist should fire; got %v", f.removeCalls)
	}
}

func TestApply_LoadError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loadErr = errors.New("modprobe: module not found")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if err == nil || !strings.Contains(err.Error(), "module not found") {
		t.Errorf("err = %v, want provider error", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
}

func TestApply_AddPersistError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	f.addErr = errors.New("write /etc/modules-load.d: read-only fs")
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("br_netfilter", StatePresent, nil))
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("err = %v, want add error", err)
	}
}

// ---- end-to-end --------------------------------------------------

func TestModule_EndToEnd_PresentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	decl := declFor("br_netfilter", StatePresent, nil)
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

func TestModule_EndToEnd_AbsentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.loaded["br_netfilter"] = true
	f.persist["br_netfilter"] = true
	m := newModuleWith(f)
	decl := declFor("br_netfilter", StateAbsent, nil)
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Apply")
	}
}

func TestSentinels(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("sentinel mismatch")
	}
	if IsUnsupportedOS(errors.New("x")) {
		t.Error("unrelated error matched")
	}
}
