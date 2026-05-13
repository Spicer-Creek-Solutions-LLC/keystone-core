package timezone

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
func (f *fakeProvider) Set(_ context.Context, z string) error {
	f.setCalls = append(f.setCalls, z)
	if f.setErr != nil {
		return f.setErr
	}
	f.cur = z
	f.set = true
	return nil
}

func declFor(name string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{ID: "timezone:" + name, Module: "timezone", Name: name, State: StatePresent, Params: params}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- params / validate -------------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("UTC", map[string]any{"foo": "bar"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestValidate_AcceptsCommon(t *testing.T) {
	t.Parallel()
	for _, z := range []string{"UTC", "America/New_York", "Europe/London", "Etc/GMT+5", "Etc/UTC", "Asia/Kolkata"} {
		p, err := parseParams(declFor(z, nil))
		if err != nil {
			t.Errorf("parse %q: %v", z, err)
			continue
		}
		if err := p.validate(); err != nil {
			t.Errorf("good zone %q rejected: %v", z, err)
		}
	}
}

func TestValidate_RejectsBad(t *testing.T) {
	t.Parallel()
	for _, z := range []string{"", "../etc/passwd", "/UTC", "UTC/", "evil zone", "with;semicolon"} {
		p, err := parseParams(declFor(z, nil))
		if err != nil {
			continue
		}
		if err := p.validate(); err == nil {
			t.Errorf("zone %q should be rejected", z)
		}
	}
}

func TestValidate_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	// "America/.." contains a dot, which the zone-name charset
	// excludes — traversal is impossible by construction; the
	// rejection comes from the charset gate.
	p, _ := parseParams(declFor("America/..", nil))
	if err := p.validate(); err == nil {
		t.Error("dot-containing zone should be rejected")
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "timezone" {
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
	m := NewWithProvider(&fakeProvider{cur: "UTC", set: true})
	if m == nil {
		t.Fatal("nil")
	}
	// And it must talk to the injected provider, not the OS default.
	ok, err := m.Test(context.Background(), declFor("UTC", nil))
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
	if err := m.Validate(declFor("UTC", nil)); err != nil {
		t.Errorf("good zone rejected: %v", err)
	}
	if err := m.Validate(declFor("evil zone", nil)); err == nil {
		t.Error("bad zone accepted")
	}
	// parseParams error path (unknown key) also routes through
	// Validate, exercising the early return.
	if err := m.Validate(declFor("UTC", map[string]any{"unknown": "x"})); err == nil {
		t.Error("unknown param accepted")
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_NotSet(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{set: false})
	res, _ := m.Check(context.Background(), declFor("UTC", nil))
	if res.Matches {
		t.Error("unset localtime should drift")
	}
	if !strings.Contains(res.Diff, "not a zoneinfo symlink") {
		t.Errorf("diff should explain; got %q", res.Diff)
	}
}

func TestCheck_Match(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "America/New_York", set: true})
	res, _ := m.Check(context.Background(), declFor("America/New_York", nil))
	if !res.Matches {
		t.Errorf("matching zone should not drift; diff = %q", res.Diff)
	}
}

func TestCheck_Mismatch(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "UTC", set: true})
	res, _ := m.Check(context.Background(), declFor("America/New_York", nil))
	if res.Matches {
		t.Error("mismatched zone should drift")
	}
	if !strings.Contains(res.Diff, "UTC") || !strings.Contains(res.Diff, "America/New_York") {
		t.Errorf("diff should cite both; got %q", res.Diff)
	}
}

func TestCheck_ProviderError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{curErr: errors.New("readlink: permission denied")})
	_, err := m.Check(context.Background(), declFor("UTC", nil))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want provider error", err)
	}
}

func TestCheck_ParseError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "UTC", set: true})
	_, err := m.Check(context.Background(), declFor("UTC", map[string]any{"unknown": "x"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestCheck_ValidateError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "UTC", set: true})
	_, err := m.Check(context.Background(), declFor("evil zone", nil))
	if err == nil {
		t.Error("invalid zone should fail Check")
	}
}

func TestTest_PropagatesError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{curErr: errors.New("boom")})
	ok, err := m.Test(context.Background(), declFor("UTC", nil))
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
	f := &fakeProvider{cur: "UTC", set: true}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("America/New_York", nil))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed should be true")
	}
	if len(f.setCalls) != 1 || f.setCalls[0] != "America/New_York" {
		t.Errorf("Set calls = %v", f.setCalls)
	}
}

func TestApply_Converged_NoCall(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "UTC", set: true}
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("UTC", nil))
	if res.Changed {
		t.Error("converged should be Changed=false")
	}
	if len(f.setCalls) != 0 {
		t.Errorf("Set should not fire; got %v", f.setCalls)
	}
}

func TestApply_SetError_Propagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "UTC", set: true, setErr: errors.New("timedatectl: invalid time zone")}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("Bogus/Zone", nil))
	if err == nil || !strings.Contains(err.Error(), "invalid time zone") {
		t.Errorf("err = %v, want provider error", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
}

func TestApply_ZoneNotFound_Propagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "UTC", set: true, setErr: ErrZoneNotFound}
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("Made/Up", nil))
	if !errors.Is(err, ErrZoneNotFound) {
		t.Errorf("err = %v, want ErrZoneNotFound", err)
	}
}

func TestApply_PreCheckError(t *testing.T) {
	t.Parallel()
	// provider.Current errors → check() bubbles → Apply returns the
	// error wrapped in a non-success StateResult.
	f := &fakeProvider{curErr: errors.New("readlink failed")}
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("UTC", nil))
	if err == nil {
		t.Fatal("Apply should propagate pre-check error")
	}
	if res == nil || res.Success {
		t.Errorf("res = %+v, want non-nil Success=false", res)
	}
}

func TestApply_ParamError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "UTC", set: true})
	_, err := m.Apply(context.Background(), declFor("UTC", map[string]any{"bad": 1}))
	if err == nil {
		t.Error("Apply should fail on unknown param")
	}
}

func TestApply_ValidateError(t *testing.T) {
	t.Parallel()
	m := newModuleWith(&fakeProvider{cur: "UTC", set: true})
	_, err := m.Apply(context.Background(), declFor("../etc/passwd", nil))
	if err == nil {
		t.Error("Apply should fail on path-traversal zone")
	}
}

// ---- end-to-end --------------------------------------------------

func TestModule_EndToEnd(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{cur: "UTC", set: true}
	m := newModuleWith(f)
	decl := declFor("America/New_York", nil)
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

func TestSentinels(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || !IsZoneNotFound(ErrZoneNotFound) {
		t.Error("sentinel mismatch")
	}
	if IsZoneNotFound(errors.New("x")) {
		t.Error("unrelated error matched")
	}
}
