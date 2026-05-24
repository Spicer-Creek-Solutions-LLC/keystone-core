// SPDX-License-Identifier: Apache-2.0

package hostname

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type fakeProvider struct {
	cur      string
	set      bool
	curErr   error
	setErr   error
	setCalls []string
}

func (f *fakeProvider) Current() (string, bool, error) {
	if f.curErr != nil {
		return "", false, f.curErr
	}
	return f.cur, f.set, nil
}
func (f *fakeProvider) Set(_ context.Context, h string) error {
	f.setCalls = append(f.setCalls, h)
	if f.setErr != nil {
		return f.setErr
	}
	f.cur = h
	f.set = true
	return nil
}

func declFor(name string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{ID: "hostname:" + name, Module: "hostname", Name: name, State: StatePresent, Params: params}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- params / validate -------------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("web-1", map[string]any{"foo": "bar"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestValidate_AcceptsCommon(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"web", "web-1", "web1.example.com", "a", "host-01.us-east.internal"} {
		p, err := parseParams(declFor(name, nil))
		if err != nil {
			t.Errorf("parse %q: %v", name, err)
			continue
		}
		if err := p.validate(); err != nil {
			t.Errorf("good name %q rejected: %v", name, err)
		}
	}
}

func TestValidate_RejectsBad(t *testing.T) {
	t.Parallel()
	bad := []string{"", "-leading-hyphen", "with space", "evil;rm", "trailing.dot.", "double..dot"}
	for _, name := range bad {
		p, err := parseParams(declFor(name, nil))
		if err != nil {
			continue
		}
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestValidate_LabelTooLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 64)
	p, _ := parseParams(declFor(long, nil))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "63") {
		t.Errorf("64-char label should be rejected; got %v", err)
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "hostname" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 1 || m.ValidStates()[0] != StatePresent {
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
	if (&Module{}).DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("want medium")
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	if New() == nil {
		t.Fatal("nil")
	}
}

func TestNewWithProvider_Wires(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{cur: "web-1", set: true})
	if m == nil {
		t.Fatal("nil")
	}
	ok, err := m.Test(context.Background(), declFor("web-1", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !ok {
		t.Error("Test on matching fake should be true")
	}
}

func TestModule_Validate(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("web-1", nil)); err != nil {
		t.Errorf("good hostname rejected: %v", err)
	}
	if err := m.Validate(declFor("with space", nil)); err == nil {
		t.Error("bad hostname accepted")
	}
	if err := m.Validate(declFor("web-1", map[string]any{"unknown": "x"})); err == nil {
		t.Error("unknown param accepted")
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_NotSet(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{set: false})
	res, _ := m.Check(context.Background(), declFor("web-1", nil))
	if res.Matches {
		t.Error("unset hostname should drift")
	}
	if !strings.Contains(res.Diff, "no static hostname") {
		t.Errorf("diff should explain; got %q", res.Diff)
	}
}

func TestCheck_Match(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "web-1", set: true})
	res, _ := m.Check(context.Background(), declFor("web-1", nil))
	if !res.Matches {
		t.Errorf("matching hostname should not drift; diff = %q", res.Diff)
	}
}

func TestCheck_Mismatch(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "old", set: true})
	res, _ := m.Check(context.Background(), declFor("new", nil))
	if res.Matches {
		t.Error("mismatched hostname should drift")
	}
	if !strings.Contains(res.Diff, "old") || !strings.Contains(res.Diff, "new") {
		t.Errorf("diff should cite both; got %q", res.Diff)
	}
}

func TestCheck_ProviderError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{curErr: errors.New("read /etc/hostname: permission denied")})
	_, err := m.Check(context.Background(), declFor("web-1", nil))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want provider error", err)
	}
}

func TestCheck_ParseError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "web-1", set: true})
	_, err := m.Check(context.Background(), declFor("web-1", map[string]any{"unknown": "x"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestCheck_ValidateError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "web-1", set: true})
	_, err := m.Check(context.Background(), declFor("with space", nil))
	if err == nil {
		t.Error("invalid hostname should fail Check")
	}
}

func TestTest_PropagatesError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{curErr: errors.New("boom")})
	ok, err := m.Test(context.Background(), declFor("web-1", nil))
	if err == nil {
		t.Error("Test should bubble up Check error")
	}
	if ok {
		t.Error("Test on error should return false")
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_SetsWhenDrifted(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "old", set: true}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("new", nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed should be true")
	}
	if len(f.setCalls) != 1 || f.setCalls[0] != "new" {
		t.Errorf("Set calls = %v, want [new]", f.setCalls)
	}
}

func TestApply_Converged_NoCall(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "web-1", set: true}
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("web-1", nil))
	if res.Changed {
		t.Error("converged should be Changed=false")
	}
	if len(f.setCalls) != 0 {
		t.Errorf("Set should not fire on converged; got %v", f.setCalls)
	}
}

func TestApply_SetError_Propagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "old", set: true, setErr: errors.New("hostnamectl: permission denied")}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("new", nil))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want provider error", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
}

func TestApply_PreCheckError(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{curErr: errors.New("read failed")}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("web-1", nil))
	if err == nil {
		t.Fatal("Apply should propagate pre-check error")
	}
	if res == nil || res.Success {
		t.Errorf("res = %+v, want non-nil Success=false", res)
	}
}

func TestApply_ParamError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "web-1", set: true})
	_, err := m.Apply(context.Background(), declFor("web-1", map[string]any{"bad": 1}))
	if err == nil {
		t.Error("Apply should fail on unknown param")
	}
}

func TestApply_ValidateError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "web-1", set: true})
	_, err := m.Apply(context.Background(), declFor("with space", nil))
	if err == nil {
		t.Error("Apply should fail on invalid hostname")
	}
}

// ---- end-to-end --------------------------------------------------

func TestModule_EndToEnd(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "old", set: true}
	m := newModuleWith(f)
	decl := declFor("new", nil)
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

func TestSentinel(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("sentinel mismatch")
	}
	if IsUnsupportedOS(errors.New("x")) {
		t.Error("unrelated error matched")
	}
}
