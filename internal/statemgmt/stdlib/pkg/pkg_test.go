// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider drives Module tests without touching apt-get.
type fakeProvider struct {
	pkgs          map[string]PkgInfo
	installErr    error
	removeErr     error
	installCalls  []installCall
	removeCalls   []string
}

type installCall struct {
	Name    string
	Version string
}

func newFake(seed ...PkgInfo) *fakeProvider {
	f := &fakeProvider{pkgs: map[string]PkgInfo{}}
	for _, p := range seed {
		f.pkgs[p.Name] = p
	}
	return f
}

func (f *fakeProvider) Lookup(name string) (*PkgInfo, error) {
	p, ok := f.pkgs[name]
	if !ok {
		return &PkgInfo{Name: name, Installed: false}, nil
	}
	return &p, nil
}

func (f *fakeProvider) Install(_ context.Context, name, version string) error {
	f.installCalls = append(f.installCalls, installCall{Name: name, Version: version})
	if f.installErr != nil {
		return f.installErr
	}
	v := version
	if v == "" {
		v = "1.0.0-latest" // simulate apt picking a version
	}
	f.pkgs[name] = PkgInfo{Name: name, Installed: true, Version: v}
	return nil
}

func (f *fakeProvider) Remove(_ context.Context, name string) error {
	f.removeCalls = append(f.removeCalls, name)
	if f.removeErr != nil {
		return f.removeErr
	}
	delete(f.pkgs, name)
	return nil
}

func declFor(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "package:" + name,
		Module: "package",
		Name:   name,
		State:  state,
		Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- parseParams / validate ---------------------------------------

func TestParseParams_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("nginx", StateInstalled, map[string]any{"badkey": "x"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param error", err)
	}
}

func TestParseParams_NonStringVersion(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("nginx", StateInstalled, map[string]any{"version": 1.20}))
	if err == nil {
		t.Error("non-string version should be rejected")
	}
}

func TestValidate_AbsentRejectsVersion(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("nginx", StateAbsent, map[string]any{"version": "1.20"}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("want absent-rejects-version error, got %v", err)
	}
}

func TestValidate_BadPackageName(t *testing.T) {
	t.Parallel()
	bad := []string{"", "nginx pipe|cat", "nginx;rm -rf", "<bad>", "with spaces"}
	for _, name := range bad {
		decl := &statemgmt.Declaration{
			ID: "package:" + name, Module: "package", Name: name, State: StateInstalled,
		}
		p, err := parseParams(decl)
		if err != nil {
			continue
		}
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestValidate_AcceptsCommonPackageNames(t *testing.T) {
	t.Parallel()
	good := []string{"nginx", "libssl1.1", "python3.11", "lib32-glibc", "g++", "0install"}
	for _, name := range good {
		decl := &statemgmt.Declaration{
			ID: "package:" + name, Module: "package", Name: name, State: StateInstalled,
		}
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
	if m.Name() != "package" {
		t.Errorf("Name = %q, want package", m.Name())
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
	if got := m.DriftSeverity(declFor("nginx", StateAbsent, nil), nil); got != statemgmt.DriftSeverityHigh {
		t.Errorf("absent severity = %v, want high", got)
	}
	if got := m.DriftSeverity(declFor("nginx", StateInstalled, nil), nil); got != statemgmt.DriftSeverityMedium {
		t.Errorf("installed severity = %v, want medium", got)
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	m := New()
	if m == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewWithProvider_UsesInjected(t *testing.T) {
	t.Parallel()
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	m := NewWithProvider(f)
	res, _ := m.Check(context.Background(), declFor("nginx", StateInstalled, nil))
	if !res.Matches {
		t.Errorf("seeded package should match; diff = %q", res.Diff)
	}
}

// ---- Check (diffCheck) -------------------------------------------

func TestCheck_AbsentNotInstalled(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("nginx", StateAbsent, nil))
	if !res.Matches {
		t.Error("absent + not-installed should match")
	}
}

func TestCheck_AbsentInstalled(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateAbsent, nil))
	if res.Matches {
		t.Error("absent + installed should drift")
	}
	if !strings.Contains(res.Diff, "1.20") {
		t.Errorf("diff should mention version; got %q", res.Diff)
	}
}

func TestCheck_InstalledMissing(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("nginx", StateInstalled, nil))
	if res.Matches {
		t.Error("installed + missing should drift")
	}
}

func TestCheck_InstalledPresentNoVersionPin(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateInstalled, nil))
	if !res.Matches {
		t.Errorf("installed + present (no pin) should match; diff = %q", res.Diff)
	}
}

func TestCheck_InstalledVersionMatches(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20.1"}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateInstalled, map[string]any{"version": "1.20.1"}))
	if !res.Matches {
		t.Error("matching version pin should not drift")
	}
}

func TestCheck_InstalledVersionMismatch(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20.1"}))
	res, _ := m.Check(context.Background(), declFor("nginx", StateInstalled, map[string]any{"version": "1.22.0"}))
	if res.Matches {
		t.Error("version mismatch should drift")
	}
	if !strings.Contains(res.Diff, "1.20.1") || !strings.Contains(res.Diff, "1.22.0") {
		t.Errorf("diff should cite both versions; got %q", res.Diff)
	}
}

// ---- Apply via fakeProvider --------------------------------------

func TestApply_InstalledMissing_CallsInstall(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("nginx", StateInstalled, nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Success || !res.Changed {
		t.Errorf("Success=%v Changed=%v, want true/true", res.Success, res.Changed)
	}
	if len(f.installCalls) != 1 || f.installCalls[0].Name != "nginx" || f.installCalls[0].Version != "" {
		t.Errorf("Install calls wrong: %+v", f.installCalls)
	}
}

func TestApply_InstalledWithVersion_PassesVersionThrough(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateInstalled, map[string]any{"version": "1.20"})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.installCalls) != 1 || f.installCalls[0].Version != "1.20" {
		t.Errorf("version not propagated to Install: %+v", f.installCalls)
	}
}

func TestApply_AlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("nginx", StateInstalled, nil))
	if res.Changed {
		t.Error("already-installed should be Changed=false")
	}
	if len(f.installCalls) != 0 {
		t.Errorf("Install should not fire on converged; got %+v", f.installCalls)
	}
}

func TestApply_VersionMismatch_RunsInstallToConverge(t *testing.T) {
	t.Parallel()
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("nginx", StateInstalled, map[string]any{"version": "1.22"}))
	if !res.Changed {
		t.Error("version-mismatch Apply should be Changed=true")
	}
	if len(f.installCalls) != 1 || f.installCalls[0].Version != "1.22" {
		t.Errorf("Install should fire with pinned version; got %+v", f.installCalls)
	}
}

func TestApply_AbsentInstalled_CallsRemove(t *testing.T) {
	t.Parallel()
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("nginx", StateAbsent, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0] != "nginx" {
		t.Errorf("Remove calls = %v, want [nginx]", f.removeCalls)
	}
}

func TestApply_AbsentNotInstalled_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("nginx", StateAbsent, nil))
	if res.Changed {
		t.Error("absent + not-installed should be Changed=false")
	}
	if len(f.removeCalls) != 0 {
		t.Error("Remove should not fire on absent+missing")
	}
}

func TestApply_InstallError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.installErr = errors.New("apt-get: dependency conflict")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("nginx", StateInstalled, nil))
	if err == nil || !strings.Contains(err.Error(), "dependency conflict") {
		t.Errorf("err = %v, want underlying provider error", err)
	}
	if res.Success || res.Changed {
		t.Errorf("Success=%v Changed=%v, want false/false", res.Success, res.Changed)
	}
}

func TestApply_RemoveError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	f.removeErr = errors.New("apt-get: held back")
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("nginx", StateAbsent, nil))
	if err == nil || !strings.Contains(err.Error(), "held back") {
		t.Errorf("err = %v, want underlying provider error", err)
	}
}

// ---- End-to-end --------------------------------------------------

func TestModule_EndToEnd_InstalledLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	decl := declFor("nginx", StateInstalled, nil)

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
	f := newFake(PkgInfo{Name: "nginx", Installed: true, Version: "1.20"})
	m := newModuleWith(f)
	decl := declFor("nginx", StateAbsent, nil)
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Remove")
	}
}

// ---- Validate wrapper + sentinels --------------------------------

func TestModule_Validate_PassesGood(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("nginx", StateInstalled, nil)); err != nil {
		t.Errorf("Validate should pass; got %v", err)
	}
}

func TestModule_Validate_RejectsBad(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("with spaces", StateInstalled, nil)); err == nil {
		t.Error("space in name should be rejected")
	}
}

func TestIsUnsupportedOS(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("sentinel should match itself")
	}
	if IsUnsupportedOS(errors.New("other")) {
		t.Error("unrelated error matched")
	}
}

func TestIsNoBackend(t *testing.T) {
	t.Parallel()
	if !IsNoBackend(ErrNoBackend) {
		t.Error("sentinel should match itself")
	}
	if IsNoBackend(errors.New("other")) {
		t.Error("unrelated error matched")
	}
}
