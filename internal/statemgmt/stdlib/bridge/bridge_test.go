package bridge

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "bridge:" + name,
		Module: "bridge",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	links map[string]*LinkInfo

	getErr, createErr, deleteErr, masterErr error

	createCalls []BridgeSpec
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
func (f *fakeProvider) CreateBridge(_ context.Context, s BridgeSpec) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, s)
	f.links[s.Name] = &LinkInfo{Name: s.Name, Kind: "bridge"}
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
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"ports": "eth0"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypesAndDefaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"name": "br0"}))
	if err != nil || p.Name != "br0" || p.STP {
		t.Errorf("defaults: %+v %v", p, err)
	}
	p, err = parseParams(decl("l", StatePresent, map[string]any{"name": "br0", "stp": true, "members": []any{"eth0", "eth1"}}))
	if err != nil || !p.STP || len(p.Members) != 2 {
		t.Errorf("with members+stp: %+v %v", p, err)
	}
	// type errors
	for _, bad := range []map[string]any{
		{"name": 1},
		{"name": "br0", "members": "not-a-list"},
		{"name": "br0", "members": []any{1}},
		{"name": "br0", "members": []any{""}},
		{"name": "br0", "stp": "true"},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
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
		{"present ok", decl("l", StatePresent, map[string]any{"name": "br0"}), false},
		{"stp + members", decl("l", StatePresent, map[string]any{"name": "br0", "stp": true, "members": []any{"eth0"}}), false},
		{"absent ok", decl("l", StateAbsent, map[string]any{"name": "br0"}), false},
		{"absent rejects members", decl("l", StateAbsent, map[string]any{"name": "br0", "members": []any{"eth0"}}), true},
		{"name required", decl("l", StatePresent, map[string]any{}), true},
		{"bad name", decl("l", StatePresent, map[string]any{"name": "br 0"}), true},
		{"bad member", decl("l", StatePresent, map[string]any{"name": "br0", "members": []any{"eth 0"}}), true},
		{"bad state", decl("l", "frob", map[string]any{"name": "br0"}), true},
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
	d := decl("b", StatePresent, map[string]any{"name": "br0", "stp": true, "members": []any{"eth0", "eth1"}})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.createCalls) != 1 || !f.createCalls[0].STP {
		t.Errorf("create: %+v", f.createCalls)
	}
	if len(f.masterCalls) != 2 {
		t.Errorf("master: %+v", f.masterCalls)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
}

func TestPresent_RefusesToClobberOtherKind(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["br0"] = &LinkInfo{Name: "br0", Kind: "vlan"}
	m := NewWithProvider(f)
	r, _ := m.Check(context.Background(), decl("b", StatePresent, map[string]any{"name": "br0"}))
	if r.Matches || !contains(r.Diff, "vlan") {
		t.Errorf("check: %+v", r)
	}
	if _, err := m.Apply(context.Background(), decl("b", StatePresent, map[string]any{"name": "br0"})); err == nil {
		t.Error("clobber should be refused")
	}
}

func TestAbsent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["br0"] = &LinkInfo{Name: "br0", Kind: "bridge"}
	m := NewWithProvider(f)
	d := decl("b", StateAbsent, map[string]any{"name": "br0"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.deleteCalls) != 1 || f.deleteCalls[0] != "br0" {
		t.Errorf("delete: %+v", f.deleteCalls)
	}
}

func TestPresent_AlreadyBridge_NoReconcile(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["br0"] = &LinkInfo{Name: "br0", Kind: "bridge"}
	m := NewWithProvider(f)
	d := decl("b", StatePresent, map[string]any{"name": "br0", "stp": true})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("existing bridge → match (v1.0): %+v", r)
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{getErr: errors.New("get")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"name": "br0"})); err == nil {
		t.Error("get")
	}
	f := newFake()
	f.createErr = errors.New("create")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "br0"})); err == nil {
		t.Error("create")
	}
	f = newFake()
	f.masterErr = errors.New("master")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "br0", "members": []any{"eth0"}})); err == nil {
		t.Error("master")
	}
	f = newFake()
	f.links["br0"] = &LinkInfo{Name: "br0", Kind: "bridge"}
	f.deleteErr = errors.New("delete")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"name": "br0"})); err == nil {
		t.Error("delete")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{})
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
	if m.Name() != "bridge" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"name": "br0"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"name": "br0"})); err != nil {
		t.Errorf("valid: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing name")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"name": "br0"})
	if ok, _ := m.Test(context.Background(), d); ok {
		t.Error("before")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("after: %v %v", ok, err)
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
