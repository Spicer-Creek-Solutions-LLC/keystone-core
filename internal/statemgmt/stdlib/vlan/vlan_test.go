// SPDX-License-Identifier: Apache-2.0

package vlan

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "vlan:" + name,
		Module: "vlan",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	links map[string]*LinkInfo

	getErr, createErr, deleteErr error

	createCalls []VLANSpec
	deleteCalls []string
}

func newFake() *fakeProvider { return &fakeProvider{links: map[string]*LinkInfo{}} }

func (f *fakeProvider) GetLink(_ context.Context, name string) (*LinkInfo, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.links[name], nil
}
func (f *fakeProvider) CreateVLAN(_ context.Context, s VLANSpec) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, s)
	f.links[s.Name] = &LinkInfo{Name: s.Name, Kind: "vlan"}
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

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"vid": 10})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypesAndCoercion(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10}))
	if err != nil || p.Name != "eth0.10" || p.Parent != "eth0" || !p.HasID || p.ID != 10 {
		t.Errorf("parse: %+v %v", p, err)
	}
	// id coercion
	for _, v := range []any{10, int64(10), float64(10), "10"} {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": v}))
		if err != nil || p.ID != 10 {
			t.Errorf("id=%v: %+v %v", v, p, err)
		}
	}
	// type errors
	for _, bad := range []map[string]any{
		{"name": 1, "parent": "eth0", "id": 10},
		{"name": "eth0.10", "parent": 1, "id": 10},
		{"name": "eth0.10", "parent": "eth0", "id": "abc"},
		{"name": "eth0.10", "parent": "eth0", "id": 1.5},
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
		{"present ok", decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10}), false},
		{"present id 1", decl("l", StatePresent, map[string]any{"name": "eth0.1", "parent": "eth0", "id": 1}), false},
		{"present id 4094", decl("l", StatePresent, map[string]any{"name": "eth0.4094", "parent": "eth0", "id": 4094}), false},
		{"id 0 rejected", decl("l", StatePresent, map[string]any{"name": "x", "parent": "eth0", "id": 0}), true},
		{"id 4095 rejected", decl("l", StatePresent, map[string]any{"name": "x", "parent": "eth0", "id": 4095}), true},
		{"name required", decl("l", StatePresent, map[string]any{"parent": "eth0", "id": 10}), true},
		{"parent required", decl("l", StatePresent, map[string]any{"name": "eth0.10", "id": 10}), true},
		{"id required", decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0"}), true},
		{"absent ok (name only)", decl("l", StateAbsent, map[string]any{"name": "eth0.10"}), false},
		{"absent rejects parent", decl("l", StateAbsent, map[string]any{"name": "eth0.10", "parent": "eth0"}), true},
		{"absent rejects id", decl("l", StateAbsent, map[string]any{"name": "eth0.10", "id": 10}), true},
		{"bad name charset", decl("l", StatePresent, map[string]any{"name": "eth 0.10", "parent": "eth0", "id": 10}), true},
		{"bad parent charset", decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth 0", "id": 10}), true},
		{"bad state", decl("l", "frob", map[string]any{"name": "eth0.10"}), true},
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
	d := decl("v", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.createCalls) != 1 || f.createCalls[0].Parent != "eth0" || f.createCalls[0].ID != 10 {
		t.Errorf("create: %+v", f.createCalls)
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
	f.links["eth0.10"] = &LinkInfo{Name: "eth0.10", Kind: "bond"}
	m := NewWithProvider(f)
	r, _ := m.Check(context.Background(), decl("v", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10}))
	if r.Matches || !contains(r.Diff, "bond") {
		t.Errorf("check: %+v", r)
	}
	if _, err := m.Apply(context.Background(), decl("v", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})); err == nil {
		t.Error("clobber should be refused")
	}
}

func TestAbsent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["eth0.10"] = &LinkInfo{Name: "eth0.10", Kind: "vlan"}
	m := NewWithProvider(f)
	d := decl("v", StateAbsent, map[string]any{"name": "eth0.10"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.deleteCalls) != 1 || f.deleteCalls[0] != "eth0.10" {
		t.Errorf("delete: %+v", f.deleteCalls)
	}
}

func TestPresent_AlreadyVLAN_NoReconcile(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.links["eth0.10"] = &LinkInfo{Name: "eth0.10", Kind: "vlan"}
	m := NewWithProvider(f)
	d := decl("v", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 99})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("existing vlan → match (v1.0 no reconcile of id/parent): %+v", r)
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{getErr: errors.New("get")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})); err == nil {
		t.Error("get")
	}
	f := newFake()
	f.createErr = errors.New("create")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})); err == nil {
		t.Error("create")
	}
	f = newFake()
	f.links["eth0.10"] = &LinkInfo{Name: "eth0.10", Kind: "vlan"}
	f.deleteErr = errors.New("delete")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"name": "eth0.10"})); err == nil {
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
	if m.Name() != "vlan" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})); err != nil {
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
	d := decl("l", StatePresent, map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10})
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
