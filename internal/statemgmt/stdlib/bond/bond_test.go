package bond

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "bond:" + name,
		Module: "bond",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	links map[string]*LinkInfo

	getErr, createErr, deleteErr, masterErr error

	createCalls []BondSpec
	deleteCalls []string
	masterCalls []masterCall
}

type masterCall struct {
	Child, Master string
}

func newFake() *fakeProvider { return &fakeProvider{links: map[string]*LinkInfo{}} }

func (f *fakeProvider) GetLink(_ context.Context, name string) (*LinkInfo, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.links[name], nil
}
func (f *fakeProvider) CreateBond(_ context.Context, s BondSpec) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, s)
	f.links[s.Name] = &LinkInfo{Name: s.Name, Kind: "bond"}
	return nil
}
func (f *fakeProvider) DeleteLink(_ context.Context, name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleteCalls = append(f.deleteCalls, name)
	delete(f.links, name)
	return nil
}
func (f *fakeProvider) SetMaster(_ context.Context, child, master string) error {
	if f.masterErr != nil {
		return f.masterErr
	}
	f.masterCalls = append(f.masterCalls, masterCall{Child: child, Master: master})
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"slaves": "eth0"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_ModeAndMembers(t *testing.T) {
	t.Parallel()
	// numeric mode canonicalised
	p, err := parseParams(decl("l", StatePresent, map[string]any{"name": "bond0", "mode": "1"}))
	if err != nil || p.Mode != "active-backup" {
		t.Errorf("numeric: %+v %v", p, err)
	}
	// named mode passthrough
	p, err = parseParams(decl("l", StatePresent, map[string]any{"name": "bond0", "mode": "802.3ad"}))
	if err != nil || p.Mode != "802.3ad" {
		t.Errorf("named: %+v %v", p, err)
	}
	// default
	p, err = parseParams(decl("l", StatePresent, map[string]any{"name": "bond0"}))
	if err != nil || p.Mode != "balance-rr" {
		t.Errorf("default mode: %+v %v", p, err)
	}
	// members + miimon
	p, err = parseParams(decl("l", StatePresent, map[string]any{"name": "bond0", "members": []any{"eth0", "eth1"}, "miimon": 100}))
	if err != nil || !reflect.DeepEqual(p.Members, []string{"eth0", "eth1"}) || !p.HasMiimon || p.Miimon != 100 {
		t.Errorf("members+miimon: %+v %v", p, err)
	}
	// bad mode
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"name": "bond0", "mode": "round-robin"})); err == nil {
		t.Error("bad mode should error")
	}
	// type errors
	for _, bad := range []map[string]any{
		{"name": 1},
		{"name": "bond0", "mode": 1},
		{"name": "bond0", "members": "not-a-list"},
		{"name": "bond0", "members": []any{1}},
		{"name": "bond0", "members": []any{""}},
		{"name": "bond0", "miimon": "abc"},
		{"name": "bond0", "miimon": 1.5},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestKnownModes_Sorted(t *testing.T) {
	t.Parallel()
	got := KnownModes()
	if len(got) != 7 {
		t.Fatalf("want 7 modes, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("not sorted: %q >= %q", got[i-1], got[i])
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
		{"present ok", decl("l", StatePresent, map[string]any{"name": "bond0"}), false},
		{"with members + miimon", decl("l", StatePresent, map[string]any{"name": "bond0", "members": []any{"eth0", "eth1"}, "miimon": 100}), false},
		{"absent ok", decl("l", StateAbsent, map[string]any{"name": "bond0"}), false},
		{"absent rejects members", decl("l", StateAbsent, map[string]any{"name": "bond0", "members": []any{"eth0"}}), true},
		{"name required", decl("l", StatePresent, map[string]any{}), true},
		{"bad name charset", decl("l", StatePresent, map[string]any{"name": "bond 0"}), true},
		{"name too long", decl("l", StatePresent, map[string]any{"name": "0123456789abcdef"}), true},
		{"bad member name", decl("l", StatePresent, map[string]any{"name": "bond0", "members": []any{"eth 0"}}), true},
		{"negative miimon", decl("l", StatePresent, map[string]any{"name": "bond0", "miimon": -1}), true},
		{"bad state", decl("l", "frob", map[string]any{"name": "bond0"}), true},
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

// --- module logic ----------------------------------------------------

func TestPresent_Create(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("b", StatePresent, map[string]any{"name": "bond0", "mode": "active-backup", "members": []any{"eth0", "eth1"}, "miimon": 100})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("absent → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.createCalls) != 1 || f.createCalls[0].Mode != "active-backup" || !f.createCalls[0].HasMiimon || f.createCalls[0].Miimon != 100 {
		t.Errorf("create: %+v", f.createCalls)
	}
	if len(f.masterCalls) != 2 {
		t.Errorf("master calls: %+v", f.masterCalls)
	}
	for i, mc := range f.masterCalls {
		if mc.Master != "bond0" {
			t.Errorf("master[%d]: %+v", i, mc)
		}
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply: %+v", sr)
	}
}

func TestPresent_RefusesToClobberOtherKind(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["bond0"] = &LinkInfo{Name: "bond0", Kind: "veth"}
	m := NewWithProvider(f)
	r, _ := m.Check(context.Background(), decl("b", StatePresent, map[string]any{"name": "bond0"}))
	if r.Matches || !contains(r.Diff, "veth") {
		t.Errorf("check: %+v", r)
	}
	if _, err := m.Apply(context.Background(), decl("b", StatePresent, map[string]any{"name": "bond0"})); err == nil {
		t.Error("clobber should be refused")
	}
}

func TestAbsent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["bond0"] = &LinkInfo{Name: "bond0", Kind: "bond"}
	m := NewWithProvider(f)
	d := decl("b", StateAbsent, map[string]any{"name": "bond0"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("present → drift: %+v", r)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.deleteCalls) != 1 || f.deleteCalls[0] != "bond0" {
		t.Errorf("delete: %+v", f.deleteCalls)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
}

func TestPresent_AlreadyBond_NoReconcile(t *testing.T) {
	t.Parallel()
	// v1.0: if interface exists with right type, no-op (don't reconcile mode/members)
	f := newFake()
	f.links["bond0"] = &LinkInfo{Name: "bond0", Kind: "bond"}
	m := NewWithProvider(f)
	d := decl("b", StatePresent, map[string]any{"name": "bond0", "mode": "active-backup"})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("existing bond → match (v1.0 no in-place reconcile): %+v", r)
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{getErr: errors.New("get")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"name": "bond0"})); err == nil {
		t.Error("Get error should propagate")
	}
	f := newFake()
	f.createErr = errors.New("create")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "bond0"})); err == nil {
		t.Error("create error should propagate")
	}
	f = newFake()
	f.masterErr = errors.New("master")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "bond0", "members": []any{"eth0"}})); err == nil {
		t.Error("master error should propagate")
	}
	f = newFake()
	f.links["bond0"] = &LinkInfo{Name: "bond0", Kind: "bond"}
	f.deleteErr = errors.New("delete")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"name": "bond0"})); err == nil {
		t.Error("delete error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{}) // no name
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject")
	}
}

// --- module surface --------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "bond" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("bond should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"name": "bond0"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"name": "bond0"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing name should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"name": "bond0"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoIP(ErrNoIP) || IsNoIP(errors.New("x")) {
		t.Error("IsNoIP")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
