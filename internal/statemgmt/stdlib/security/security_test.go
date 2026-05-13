package security

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "security:" + name,
		Module: "security",
		State:  StatePresent,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type fakeProvider struct {
	persistentMode   string
	persistentErr    error
	persistentSetErr error
	runtimeMode      string
	runtimeErr       error
	runtimeSetErr    error
	booleans         map[string]bool
	getBoolErr       error
	setBoolErr       error
	persistentSets   []string
	runtimeSets      []string
	booleanSets      []boolSetCall
}

type boolSetCall struct {
	Name  string
	Value bool
}

func (f *fakeProvider) GetPersistentMode(_ context.Context) (string, error) {
	return f.persistentMode, f.persistentErr
}
func (f *fakeProvider) GetRuntimeMode(_ context.Context) (string, error) {
	return f.runtimeMode, f.runtimeErr
}
func (f *fakeProvider) SetPersistentMode(_ context.Context, mode string) error {
	if f.persistentSetErr != nil {
		return f.persistentSetErr
	}
	f.persistentSets = append(f.persistentSets, mode)
	f.persistentMode = mode
	return nil
}
func (f *fakeProvider) SetRuntimeMode(_ context.Context, mode string) error {
	if f.runtimeSetErr != nil {
		return f.runtimeSetErr
	}
	f.runtimeSets = append(f.runtimeSets, mode)
	f.runtimeMode = mode
	return nil
}
func (f *fakeProvider) GetBoolean(_ context.Context, name string) (bool, error) {
	if f.getBoolErr != nil {
		return false, f.getBoolErr
	}
	v, ok := f.booleans[name]
	if !ok {
		return false, nil
	}
	return v, nil
}
func (f *fakeProvider) SetBoolean(_ context.Context, name string, value bool) error {
	if f.setBoolErr != nil {
		return f.setBoolErr
	}
	if f.booleans == nil {
		f.booleans = map[string]bool{}
	}
	f.booleans[name] = value
	f.booleanSets = append(f.booleanSets, boolSetCall{Name: name, Value: value})
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", map[string]any{"modes": "enforcing"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_ModeAndBooleanExclusion(t *testing.T) {
	t.Parallel()
	// neither
	if _, err := parseParams(decl("l", map[string]any{})); err == nil {
		t.Error("missing mode/boolean should error")
	}
	// both
	if _, err := parseParams(decl("l", map[string]any{"mode": "enforcing", "boolean": "x", "value": "on"})); err == nil {
		t.Error("mode + boolean should error")
	}
	// value without boolean
	if _, err := parseParams(decl("l", map[string]any{"mode": "enforcing", "value": "on"})); err == nil {
		t.Error("value with mode should error")
	}
	// boolean without value
	if _, err := parseParams(decl("l", map[string]any{"boolean": "x"})); err == nil {
		t.Error("boolean without value should error")
	}
}

func TestParse_ModeOp(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"enforcing", "permissive", "disabled", "ENFORCING", "Permissive"} {
		p, err := parseParams(decl("l", map[string]any{"mode": m}))
		if err != nil {
			t.Fatalf("mode=%q: %v", m, err)
		}
		if p.Op != OpMode || p.Mode == "" {
			t.Errorf("mode=%q parsed wrong: %+v", m, p)
		}
	}
	if _, err := parseParams(decl("l", map[string]any{"mode": 1})); err == nil {
		t.Error("non-string mode should error")
	}
}

func TestParse_BooleanOp_AndValueCoercion(t *testing.T) {
	t.Parallel()
	for _, v := range []any{true, "true", "on", "yes", "1", 1, int64(1)} {
		p, err := parseParams(decl("l", map[string]any{"boolean": "httpd_can_network_connect", "value": v}))
		if err != nil || p.Op != OpBoolean || !p.BooleanValue || p.BooleanName != "httpd_can_network_connect" {
			t.Errorf("value=%v: parsed %+v err=%v", v, p, err)
		}
	}
	for _, v := range []any{false, "false", "off", "no", "0", 0, int64(0)} {
		p, err := parseParams(decl("l", map[string]any{"boolean": "x", "value": v}))
		if err != nil || p.Op != OpBoolean || p.BooleanValue {
			t.Errorf("value=%v: parsed %+v err=%v", v, p, err)
		}
	}
	for _, v := range []any{"frob", 7, int64(2), 1.5, []any{}} {
		if _, err := parseParams(decl("l", map[string]any{"boolean": "x", "value": v})); err == nil {
			t.Errorf("value=%v should error", v)
		}
	}
	// non-string boolean name
	if _, err := parseParams(decl("l", map[string]any{"boolean": 1, "value": true})); err == nil {
		t.Error("non-string boolean should error")
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"mode enforcing", decl("l", map[string]any{"mode": "enforcing"}), false},
		{"mode permissive", decl("l", map[string]any{"mode": "permissive"}), false},
		{"mode disabled", decl("l", map[string]any{"mode": "disabled"}), false},
		{"bad mode", decl("l", map[string]any{"mode": "strict"}), true},
		{"boolean ok", decl("l", map[string]any{"boolean": "httpd_can_network_connect", "value": true}), false},
		{"empty boolean name", decl("l", map[string]any{"boolean": "   ", "value": true}), true},
		{"bad boolean charset", decl("l", map[string]any{"boolean": "no-dash", "value": true}), true},
		{"boolean starts with digit", decl("l", map[string]any{"boolean": "9foo", "value": true}), true},
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

func TestValidate_StateMustBePresent(t *testing.T) {
	t.Parallel()
	d := decl("l", map[string]any{"mode": "enforcing"})
	d.State = "absent"
	p, err := parseParams(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.validate(); err == nil {
		t.Error("state=absent should be rejected")
	}
}

func TestOpString(t *testing.T) {
	t.Parallel()
	if OpMode.String() != "mode" || OpBoolean.String() != "boolean" || OpUnknown.String() != "unknown" {
		t.Error("Op.String mapping wrong")
	}
}

// --- mode op ----------------------------------------------------------

func TestMode_AlreadyConverged(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{persistentMode: ModeEnforcing, runtimeMode: ModeEnforcing}
	m := NewWithProvider(f)
	d := decl("set-mode", map[string]any{"mode": "enforcing"})
	r, err := m.Check(context.Background(), d)
	if err != nil || !r.Matches {
		t.Errorf("converged check: %+v %v", r, err)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || sr.Changed {
		t.Errorf("converged apply: %+v %v", sr, err)
	}
	if len(f.persistentSets) != 0 || len(f.runtimeSets) != 0 {
		t.Errorf("converged should make no calls: %+v %+v", f.persistentSets, f.runtimeSets)
	}
}

func TestMode_PersistentDriftOnly(t *testing.T) {
	t.Parallel()
	// persistent says permissive but runtime is enforcing — needPersist
	f := &fakeProvider{persistentMode: ModePermissive, runtimeMode: ModeEnforcing}
	m := NewWithProvider(f)
	d := decl("set-mode", map[string]any{"mode": "enforcing"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches || !contains(r.Diff, "persistent") {
		t.Errorf("check: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("apply: %+v %v", sr, err)
	}
	if len(f.persistentSets) != 1 || f.persistentSets[0] != ModeEnforcing {
		t.Errorf("persistentSets: %+v", f.persistentSets)
	}
	if len(f.runtimeSets) != 0 {
		t.Errorf("runtime should not have been set: %+v", f.runtimeSets)
	}
	if sr.Comment != "applied" {
		t.Errorf("comment: %q", sr.Comment)
	}
}

func TestMode_RuntimeDriftOnly(t *testing.T) {
	t.Parallel()
	// persistent already matches; runtime drifted
	f := &fakeProvider{persistentMode: ModeEnforcing, runtimeMode: ModePermissive}
	m := NewWithProvider(f)
	d := decl("set-mode", map[string]any{"mode": "enforcing"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches || !contains(r.Diff, "runtime") {
		t.Errorf("check: %+v", r)
	}
	sr, _ := m.Apply(context.Background(), d)
	if !sr.Changed || len(f.runtimeSets) != 1 || f.runtimeSets[0] != ModeEnforcing {
		t.Errorf("runtime drift: %+v calls=%+v", sr, f.runtimeSets)
	}
	if len(f.persistentSets) != 0 {
		t.Error("persistent should not have been touched")
	}
}

func TestMode_BothDrift(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{persistentMode: ModePermissive, runtimeMode: ModePermissive}
	m := NewWithProvider(f)
	d := decl("set-mode", map[string]any{"mode": "enforcing"})
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.persistentSets) != 1 || len(f.runtimeSets) != 1 {
		t.Errorf("expected both sets: persistent=%+v runtime=%+v", f.persistentSets, f.runtimeSets)
	}
}

func TestMode_TargetDisabled_RebootComment(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{persistentMode: ModeEnforcing, runtimeMode: ModeEnforcing}
	m := NewWithProvider(f)
	d := decl("disable", map[string]any{"mode": "disabled"})
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("persistent should change")
	}
	if len(f.runtimeSets) != 0 {
		t.Errorf("runtime should not be set when target=disabled: %+v", f.runtimeSets)
	}
	if !contains(sr.Comment, "reboot") {
		t.Errorf("comment should mention reboot, got %q", sr.Comment)
	}
}

func TestMode_KernelDisabled_PersistentOnly(t *testing.T) {
	t.Parallel()
	// SELinux disabled at boot; operator wants enforcing
	f := &fakeProvider{persistentMode: ModePermissive, runtimeMode: ModeDisabled}
	m := NewWithProvider(f)
	d := decl("set-mode", map[string]any{"mode": "enforcing"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("persistent drift should show")
	}
	if contains(r.Diff, "runtime") {
		t.Errorf("runtime drift should not be reported when kernel-disabled, got %q", r.Diff)
	}
	sr, _ := m.Apply(context.Background(), d)
	if len(f.runtimeSets) != 0 {
		t.Error("runtime must not be touched when kernel-disabled")
	}
	if !contains(sr.Comment, "reboot") {
		t.Errorf("kernel-disabled comment: %q", sr.Comment)
	}
}

func TestMode_KernelDisabled_Converged(t *testing.T) {
	t.Parallel()
	// already disabled persistently and at runtime
	f := &fakeProvider{persistentMode: ModeDisabled, runtimeMode: ModeDisabled}
	m := NewWithProvider(f)
	d := decl("d", map[string]any{"mode": "disabled"})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("disabled+disabled should converge: %+v", r)
	}
}

func TestMode_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	// Get persistent error
	m := NewWithProvider(&fakeProvider{persistentErr: errors.New("read")})
	if _, err := m.Check(context.Background(), decl("l", map[string]any{"mode": "enforcing"})); err == nil {
		t.Error("persistent read error should propagate")
	}
	// Get runtime error
	m = NewWithProvider(&fakeProvider{persistentMode: ModeEnforcing, runtimeErr: errors.New("getenforce")})
	if _, err := m.Check(context.Background(), decl("l", map[string]any{"mode": "enforcing"})); err == nil {
		t.Error("runtime read error should propagate")
	}
	// SetPersistent error
	m = NewWithProvider(&fakeProvider{persistentMode: ModePermissive, runtimeMode: ModeEnforcing, persistentSetErr: errors.New("write")})
	if _, err := m.Apply(context.Background(), decl("l", map[string]any{"mode": "enforcing"})); err == nil {
		t.Error("persistent set error should propagate")
	}
	// SetRuntime error
	m = NewWithProvider(&fakeProvider{persistentMode: ModeEnforcing, runtimeMode: ModePermissive, runtimeSetErr: errors.New("setenforce")})
	if _, err := m.Apply(context.Background(), decl("l", map[string]any{"mode": "enforcing"})); err == nil {
		t.Error("runtime set error should propagate")
	}
}

// --- boolean op ------------------------------------------------------

func TestBoolean_Toggle(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{booleans: map[string]bool{"httpd_can_network_connect": false}}
	m := NewWithProvider(f)
	d := decl("allow-net", map[string]any{"boolean": "httpd_can_network_connect", "value": "on"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches || !contains(r.Diff, "off → on") {
		t.Errorf("check: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.booleanSets) != 1 || f.booleanSets[0].Name != "httpd_can_network_connect" || !f.booleanSets[0].Value {
		t.Errorf("booleanSets: %+v", f.booleanSets)
	}
	// converged → no-op
	f.booleanSets = nil
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || len(f.booleanSets) != 0 {
		t.Errorf("converged apply: %+v calls=%+v", sr, f.booleanSets)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
}

func TestBoolean_TurnOff(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{booleans: map[string]bool{"frob": true}}
	m := NewWithProvider(f)
	d := decl("off", map[string]any{"boolean": "frob", "value": false})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if f.booleans["frob"] != false {
		t.Error("boolean should be off")
	}
}

func TestBoolean_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{getBoolErr: errors.New("get")})
	if _, err := m.Check(context.Background(), decl("l", map[string]any{"boolean": "x", "value": true})); err == nil {
		t.Error("get error should propagate")
	}
	m = NewWithProvider(&fakeProvider{booleans: map[string]bool{"x": false}, setBoolErr: errors.New("set")})
	if _, err := m.Apply(context.Background(), decl("l", map[string]any{"boolean": "x", "value": true})); err == nil {
		t.Error("set error should propagate")
	}
}

// --- module surface ---------------------------------------------------

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{})
	bad := decl("l", map[string]any{}) // no op
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "security" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 1 || got[0] != StatePresent {
		t.Errorf("ValidStates=%v (should be present-only)", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("security should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", map[string]any{"mode": "enforcing"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", map[string]any{"mode": "enforcing"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", map[string]any{})); err == nil {
		t.Error("missing op should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{booleans: map[string]bool{"x": false}}
	m := NewWithProvider(f)
	d := decl("l", map[string]any{"boolean": "x", "value": true})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply should be false: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply should be true: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsSELinuxUnavailable(ErrSELinuxUnavailable) || IsSELinuxUnavailable(errors.New("x")) {
		t.Error("IsSELinuxUnavailable")
	}
}

// --- helper ----------------------------------------------------------

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
