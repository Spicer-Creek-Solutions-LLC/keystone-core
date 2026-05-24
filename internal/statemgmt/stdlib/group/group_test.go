// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider is the test injection point. Each call appends to
// the relevant slice so tests can assert which Provider method
// fired. Lookup returns a pointer copy of the seeded GroupInfo or
// nil when missing.
type fakeProvider struct {
	groups   map[string]GroupInfo
	addErr   error
	modErr   error
	delErr   error
	addCalls []addCall
	modCalls []modCall
	delCalls []string
}

type addCall struct {
	Name   string
	GID    *int
	System bool
}
type modCall struct {
	Name string
	GID  int
}

func newFake(seed ...GroupInfo) *fakeProvider {
	f := &fakeProvider{groups: map[string]GroupInfo{}}
	for _, g := range seed {
		f.groups[g.Name] = g
	}
	return f
}

func (f *fakeProvider) Lookup(name string) (*GroupInfo, error) {
	g, ok := f.groups[name]
	if !ok {
		return nil, nil
	}
	return &g, nil
}

func (f *fakeProvider) Add(_ context.Context, name string, gid *int, system bool) error {
	f.addCalls = append(f.addCalls, addCall{Name: name, GID: gid, System: system})
	if f.addErr != nil {
		return f.addErr
	}
	// Simulate the real groupadd's behaviour: pick a GID if one
	// wasn't given. 5000 is an arbitrary "system picked one for you"
	// marker the tests can recognise.
	resolved := 5000
	if gid != nil {
		resolved = *gid
	}
	f.groups[name] = GroupInfo{Name: name, GID: resolved}
	return nil
}

func (f *fakeProvider) Mod(_ context.Context, name string, gid int) error {
	f.modCalls = append(f.modCalls, modCall{Name: name, GID: gid})
	if f.modErr != nil {
		return f.modErr
	}
	if g, ok := f.groups[name]; ok {
		g.GID = gid
		f.groups[name] = g
	}
	return nil
}

func (f *fakeProvider) Del(_ context.Context, name string) error {
	f.delCalls = append(f.delCalls, name)
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.groups, name)
	return nil
}

func declFor(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "group:" + name,
		Module: "group",
		Name:   name,
		State:  state,
		Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- parseParams / validate ---------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("dev", StatePresent, map[string]any{"gid_": 1000}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param error", err)
	}
}

func TestValidate_InvalidGID(t *testing.T) {
	t.Parallel()
	for _, gid := range []int{-1, -100} {
		p, _ := parseParams(declFor("dev", StatePresent, map[string]any{"gid": gid}))
		if err := p.validate(); err == nil || !strings.Contains(err.Error(), "non-negative") {
			t.Errorf("gid=%d should be rejected; got %v", gid, err)
		}
	}
}

func TestValidate_AbsentRejectsAttrs(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("dev", StateAbsent, map[string]any{"gid": 1000}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("expected absent-rejects-attrs error, got %v", err)
	}
}

func TestValidate_BadGroupName(t *testing.T) {
	t.Parallel()
	bad := []string{"UPPER", "1leadingdigit", "spaces inhere", "weird@chars", ""}
	for _, name := range bad {
		decl := &statemgmt.Declaration{
			ID: "group:" + name, Module: "group", Name: name, State: StatePresent,
		}
		p, err := parseParams(decl)
		if err != nil {
			continue // unknown-key path won't fire here
		}
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestParseParams_NonStringValues(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("dev", StatePresent, map[string]any{"gid": "not-an-int"}))
	if err == nil {
		t.Error("gid: non-int should be rejected")
	}
	_, err = parseParams(declFor("dev", StatePresent, map[string]any{"system": "not-a-bool"}))
	if err == nil {
		t.Error("system: non-bool should be rejected")
	}
}

func TestParseParams_GIDCoercion(t *testing.T) {
	t.Parallel()
	cases := map[string]any{
		"int":   1000,
		"int64": int64(1000),
		"float": float64(1000),
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := parseParams(declFor("dev", StatePresent, map[string]any{"gid": v}))
			if err != nil {
				t.Errorf("err = %v", err)
				return
			}
			if p.GID == nil || *p.GID != 1000 {
				t.Errorf("GID = %v, want 1000", p.GID)
			}
		})
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "group" {
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
	if got := m.DriftSeverity(declFor("x", StateAbsent, nil), nil); got != statemgmt.DriftSeverityHigh {
		t.Errorf("absent severity = %v, want high", got)
	}
	if got := m.DriftSeverity(declFor("x", StatePresent, nil), nil); got != statemgmt.DriftSeverityMedium {
		t.Errorf("present severity = %v, want medium", got)
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_PresentMissing(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, err := m.Check(context.Background(), declFor("dev", StatePresent, nil))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("missing group should report drift")
	}
	if !strings.Contains(res.Diff, "missing") {
		t.Errorf("diff should mention missing; got %q", res.Diff)
	}
}

func TestCheck_PresentExactGID(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(GroupInfo{Name: "dev", GID: 1500}))
	res, _ := m.Check(context.Background(), declFor("dev", StatePresent, map[string]any{"gid": 1500}))
	if !res.Matches {
		t.Errorf("matching gid should be no drift; diff = %q", res.Diff)
	}
}

func TestCheck_PresentWrongGID(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(GroupInfo{Name: "dev", GID: 1500}))
	res, _ := m.Check(context.Background(), declFor("dev", StatePresent, map[string]any{"gid": 1600}))
	if res.Matches {
		t.Error("wrong gid should be drift")
	}
	if !strings.Contains(res.Diff, "1500") || !strings.Contains(res.Diff, "1600") {
		t.Errorf("diff should mention both gids; got %q", res.Diff)
	}
}

func TestCheck_PresentNoGIDDeclared(t *testing.T) {
	t.Parallel()
	// Group exists with some gid; no gid declared → no drift.
	m := newModuleWith(newFake(GroupInfo{Name: "dev", GID: 1500}))
	res, _ := m.Check(context.Background(), declFor("dev", StatePresent, nil))
	if !res.Matches {
		t.Errorf("present + no-gid-declared should match; diff = %q", res.Diff)
	}
}

func TestCheck_AbsentMissing(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("dev", StateAbsent, nil))
	if !res.Matches {
		t.Error("absent + missing should be no drift")
	}
}

func TestCheck_AbsentPresent(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(GroupInfo{Name: "dev", GID: 1500}))
	res, _ := m.Check(context.Background(), declFor("dev", StateAbsent, nil))
	if res.Matches {
		t.Error("absent + present should report drift")
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_PresentMissing_CallsAdd(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("dev", StatePresent, map[string]any{"gid": 1500}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Success || !res.Changed {
		t.Errorf("Success=%v Changed=%v, want true/true", res.Success, res.Changed)
	}
	if len(f.addCalls) != 1 {
		t.Fatalf("Add calls = %d, want 1", len(f.addCalls))
	}
	if f.addCalls[0].Name != "dev" || f.addCalls[0].GID == nil || *f.addCalls[0].GID != 1500 {
		t.Errorf("Add args lost: %+v", f.addCalls[0])
	}
}

func TestApply_PresentWrongGID_CallsMod(t *testing.T) {
	t.Parallel()
	f := newFake(GroupInfo{Name: "dev", GID: 1500})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("dev", StatePresent, map[string]any{"gid": 1600})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.modCalls) != 1 || f.modCalls[0].Name != "dev" || f.modCalls[0].GID != 1600 {
		t.Errorf("Mod calls = %+v, want [{dev 1600}]", f.modCalls)
	}
	if len(f.addCalls) != 0 || len(f.delCalls) != 0 {
		t.Errorf("expected only Mod; got Add=%d Del=%d", len(f.addCalls), len(f.delCalls))
	}
}

func TestApply_PresentAlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake(GroupInfo{Name: "dev", GID: 1500})
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("dev", StatePresent, map[string]any{"gid": 1500}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Error("converged Apply should be Changed=false")
	}
	if len(f.addCalls)+len(f.modCalls)+len(f.delCalls) != 0 {
		t.Errorf("expected zero provider calls; got add=%d mod=%d del=%d",
			len(f.addCalls), len(f.modCalls), len(f.delCalls))
	}
}

func TestApply_AbsentPresent_CallsDel(t *testing.T) {
	t.Parallel()
	f := newFake(GroupInfo{Name: "dev", GID: 1500})
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("dev", StateAbsent, nil)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.delCalls) != 1 || f.delCalls[0] != "dev" {
		t.Errorf("Del calls = %v, want [dev]", f.delCalls)
	}
}

func TestApply_AbsentMissing_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("dev", StateAbsent, nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Error("absent+missing should be Changed=false")
	}
	if len(f.delCalls) != 0 {
		t.Errorf("Del should not be called; got %v", f.delCalls)
	}
}

func TestApply_ProviderError_PropagatesAndReportsDiff(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.addErr = errors.New("groupadd exit 9: name in use")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("dev", StatePresent, nil))
	if err == nil || !strings.Contains(err.Error(), "name in use") {
		t.Errorf("err = %v, want underlying provider error", err)
	}
	if res.Success || res.Changed {
		t.Errorf("Success=%v Changed=%v, want false/false on provider error", res.Success, res.Changed)
	}
	if !strings.Contains(res.Diff, "missing") {
		t.Errorf("Diff should describe pre-apply gap; got %q", res.Diff)
	}
}

// ---- End-to-end via fakeProvider (idempotency) -------------------

func TestModule_EndToEnd_PresentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	decl := declFor("dev", StatePresent, map[string]any{"gid": 1500})

	// Initial Check: drift.
	check, _ := m.Check(context.Background(), decl)
	if check.Matches {
		t.Fatal("expected initial drift")
	}

	// Apply.
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Test re-checks.
	ok, err := m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Errorf("Test = %v err = %v, want true/nil", ok, err)
	}

	// Re-Apply is idempotent.
	res, _ := m.Apply(context.Background(), decl)
	if res.Changed {
		t.Errorf("re-Apply should be Changed=false; comment = %q", res.Comment)
	}
}

func TestModule_EndToEnd_AbsentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake(GroupInfo{Name: "dev", GID: 1500})
	m := newModuleWith(f)
	decl := declFor("dev", StateAbsent, nil)

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Del")
	}
}

// ---- New() factory + IsUnsupportedOS surface ---------------------

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	m := New()
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.Name() != "group" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestNewWithProvider_UsesInjected(t *testing.T) {
	t.Parallel()
	f := newFake(GroupInfo{Name: "dev", GID: 1500})
	m := NewWithProvider(f)
	res, err := m.Check(context.Background(), declFor("dev", StatePresent, nil))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.Matches {
		t.Errorf("seeded group should match; diff = %q", res.Diff)
	}
}

func TestModule_Validate_PassesGoodDeclaration(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("dev", StatePresent, map[string]any{"gid": 1500})); err != nil {
		t.Errorf("Validate should pass; got %v", err)
	}
}

func TestModule_Validate_RejectsBadParams(t *testing.T) {
	t.Parallel()
	m := &Module{}
	err := m.Validate(declFor("UPPER", StatePresent, nil))
	if err == nil {
		t.Error("uppercase name should be rejected")
	}
}

func TestOSLookup_NoSuchGroup(t *testing.T) {
	t.Parallel()
	// osLookup over a deterministically-missing name. Confirms the
	// UnknownGroupError → (nil, nil) path.
	l := osLookup{}
	info, err := l.Lookup("zzz-no-such-group-zzz")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for missing group; got %+v", info)
	}
}

func TestIsUnsupportedOS(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("ErrUnsupportedOS sentinel didn't match itself")
	}
	if IsUnsupportedOS(errors.New("something else")) {
		t.Error("unrelated error matched IsUnsupportedOS")
	}
}
