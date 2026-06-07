// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "network:" + name,
		Module: "network",
		State:  StatePresent,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	state    *InterfaceState
	getErr   error
	addErr   error
	delErr   error
	mtuErr   error
	upErr    error
	addCalls []addrCall
	delCalls []addrCall
	mtuCalls []int
	upCalls  []bool
}

type addrCall struct {
	Iface string
	CIDR  string
}

func (f *fakeProvider) GetInterface(_ context.Context, _ string) (*InterfaceState, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.state, nil
}
func (f *fakeProvider) AddAddress(_ context.Context, name, cidr string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.addCalls = append(f.addCalls, addrCall{Iface: name, CIDR: cidr})
	f.state.Addresses = append(f.state.Addresses, cidr)
	return nil
}
func (f *fakeProvider) DelAddress(_ context.Context, name, cidr string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.delCalls = append(f.delCalls, addrCall{Iface: name, CIDR: cidr})
	for i, a := range f.state.Addresses {
		if a == cidr {
			f.state.Addresses = append(f.state.Addresses[:i], f.state.Addresses[i+1:]...)
			break
		}
	}
	return nil
}
func (f *fakeProvider) SetMTU(_ context.Context, _ string, mtu int) error {
	if f.mtuErr != nil {
		return f.mtuErr
	}
	f.mtuCalls = append(f.mtuCalls, mtu)
	f.state.MTU = mtu
	return nil
}
func (f *fakeProvider) SetLinkUp(_ context.Context, _ string, up bool) error {
	if f.upErr != nil {
		return f.upErr
	}
	f.upCalls = append(f.upCalls, up)
	f.state.Up = up
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", map[string]any{"ifname": "eth0"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypesAndDefaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", map[string]any{"interface": "eth0", "mtu": 1500}))
	if err != nil || p.Interface != "eth0" || !p.HasMTU || p.MTU != 1500 || p.HasAddresses || p.HasUp {
		t.Errorf("parse: %+v %v", p, err)
	}
	// addresses canonicalised
	p, err = parseParams(decl("l", map[string]any{"interface": "eth0", "addresses": []any{"192.168.1.10/24", "FE80::1/64"}}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.1.10/24", "fe80::1/64"}
	if !reflect.DeepEqual(p.Addresses, want) {
		t.Errorf("canonicalised: %+v want %+v", p.Addresses, want)
	}
	// MTU coercion forms
	for _, v := range []any{1500, int64(1500), float64(1500), "1500"} {
		p, err := parseParams(decl("l", map[string]any{"interface": "eth0", "mtu": v}))
		if err != nil || p.MTU != 1500 {
			t.Errorf("mtu=%v: %+v %v", v, p, err)
		}
	}
	// type errors
	for _, bad := range []map[string]any{
		{"interface": 1, "mtu": 1500},
		{"interface": "eth0", "addresses": "not-a-list"},
		{"interface": "eth0", "addresses": []any{1}},
		{"interface": "eth0", "addresses": []any{"not-a-cidr"}},
		{"interface": "eth0", "mtu": "abc"},
		{"interface": "eth0", "mtu": 1.5},
		{"interface": "eth0", "up": "true"},
	} {
		if _, err := parseParams(decl("l", bad)); err == nil {
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
		{"mtu only", decl("l", map[string]any{"interface": "eth0", "mtu": 1500}), false},
		{"addresses only", decl("l", map[string]any{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}}), false},
		{"up only", decl("l", map[string]any{"interface": "eth0", "up": true}), false},
		{"all three", decl("l", map[string]any{"interface": "eth0", "mtu": 9000, "addresses": []any{"10.0.0.1/24"}, "up": true}), false},
		{"empty addresses ok", decl("l", map[string]any{"interface": "eth0", "addresses": []any{}}), false}, // explicit "no addresses"
		{"bare interface rejected", decl("l", map[string]any{"interface": "eth0"}), true},
		{"missing interface", decl("l", map[string]any{"mtu": 1500}), true},
		{"bad interface charset", decl("l", map[string]any{"interface": "eth 0", "mtu": 1500}), true},
		{"interface too long", decl("l", map[string]any{"interface": "0123456789abcdef", "mtu": 1500}), true},
		{"mtu too small", decl("l", map[string]any{"interface": "eth0", "mtu": 67}), true},
		{"mtu too large", decl("l", map[string]any{"interface": "eth0", "mtu": 70000}), true},
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
	d := decl("l", map[string]any{"interface": "eth0", "up": true})
	d.State = "absent"
	p, err := parseParams(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.validate(); err == nil {
		t.Error("state=absent should be rejected")
	}
}

func TestIsLinkLocal(t *testing.T) {
	t.Parallel()
	for cidr, want := range map[string]bool{
		"fe80::1/64":     true,
		"169.254.5.5/16": true,
		"192.168.1.1/24": false,
		"fd00::1/64":     false, // ULA, not link-local
		"::1/128":        false,
		"":               false,
	} {
		// Canonicalise first for fair comparison
		var canon string
		if cidr == "" {
			canon = ""
		} else {
			c, err := canonicalCIDR(cidr)
			if err != nil {
				canon = "" // unparseable → false
			} else {
				canon = c
			}
		}
		if got := isLinkLocal(canon); got != want {
			t.Errorf("isLinkLocal(%q[canon=%q]) = %v want %v", cidr, canon, got, want)
		}
	}
}

// --- module logic ----------------------------------------------------

func TestAddressDelta_LinkLocalNotRemoved(t *testing.T) {
	t.Parallel()
	state := &InterfaceState{
		Addresses: []string{"192.168.1.10/24", "fe80::1/64", "10.0.0.1/24"},
	}
	p := &params{Addresses: []string{"192.168.1.10/24"}, HasAddresses: true}
	mod := &Module{provider: &fakeProvider{state: state}}
	add, remove := mod.addressDelta(p, state)
	if len(add) != 0 {
		t.Errorf("add: %v", add)
	}
	// fe80::1/64 must NOT be in remove
	wantRemove := []string{"10.0.0.1/24"}
	if !reflect.DeepEqual(remove, wantRemove) {
		t.Errorf("remove = %v, want %v (link-local fe80::1/64 must be preserved)", remove, wantRemove)
	}
}

func TestCheckApply_AlreadyConverged(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: true, MTU: 1500, Addresses: []string{"192.168.1.10/24"}}}
	m := NewWithProvider(f)
	d := decl("e", map[string]any{"interface": "eth0", "mtu": 1500, "addresses": []any{"192.168.1.10/24"}, "up": true})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply: %+v", sr)
	}
}

func TestApply_AddsRemovesAddresses(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: true, MTU: 1500, Addresses: []string{"192.168.1.10/24", "10.0.0.1/24"}}}
	m := NewWithProvider(f)
	d := decl("e", map[string]any{"interface": "eth0", "addresses": []any{"192.168.1.10/24", "192.168.2.5/24"}})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("drift expected: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.addCalls) != 1 || f.addCalls[0].CIDR != "192.168.2.5/24" {
		t.Errorf("add: %+v", f.addCalls)
	}
	if len(f.delCalls) != 1 || f.delCalls[0].CIDR != "10.0.0.1/24" {
		t.Errorf("del: %+v", f.delCalls)
	}
}

func TestApply_SetsMTU(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", MTU: 1500}}
	m := NewWithProvider(f)
	d := decl("e", map[string]any{"interface": "eth0", "mtu": 9000})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.mtuCalls) != 1 || f.mtuCalls[0] != 9000 {
		t.Errorf("mtu: %+v", f.mtuCalls)
	}
}

func TestApply_SetsLinkUp(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: false, MTU: 1500}}
	m := NewWithProvider(f)
	d := decl("e", map[string]any{"interface": "eth0", "up": true})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.upCalls) != 1 || !f.upCalls[0] {
		t.Errorf("up: %+v", f.upCalls)
	}
}

func TestApply_OrderMTUBeforeAddrBeforeUp(t *testing.T) {
	t.Parallel()
	// Check that all three reconciliations fire when all three drift,
	// and that the final state matches.
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: false, MTU: 1500, Addresses: nil}}
	m := NewWithProvider(f)
	d := decl("e", map[string]any{"interface": "eth0", "mtu": 9000, "addresses": []any{"10.0.0.1/24"}, "up": true})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if f.mtuCalls[0] != 9000 || f.addCalls[0].CIDR != "10.0.0.1/24" || !f.upCalls[0] {
		t.Errorf("all three: mtu=%v add=%+v up=%v", f.mtuCalls, f.addCalls, f.upCalls)
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	// Get error
	m := NewWithProvider(&fakeProvider{getErr: errors.New("no link")})
	if _, err := m.Check(context.Background(), decl("l", map[string]any{"interface": "eth0", "up": true})); err == nil {
		t.Error("get error should propagate")
	}
	// AddAddress error
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", MTU: 1500}, addErr: errors.New("EEXIST")}
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", map[string]any{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}})); err == nil {
		t.Error("add error should propagate")
	}
	// DelAddress error
	f = &fakeProvider{state: &InterfaceState{Name: "eth0", MTU: 1500, Addresses: []string{"10.0.0.1/24"}}, delErr: errors.New("EBUSY")}
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", map[string]any{"interface": "eth0", "addresses": []any{}})); err == nil {
		t.Error("del error should propagate")
	}
	// SetMTU error
	f = &fakeProvider{state: &InterfaceState{Name: "eth0", MTU: 1500}, mtuErr: errors.New("EINVAL")}
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", map[string]any{"interface": "eth0", "mtu": 9000})); err == nil {
		t.Error("mtu error should propagate")
	}
	// SetLinkUp error
	f = &fakeProvider{state: &InterfaceState{Name: "eth0", MTU: 1500, Up: false}, upErr: errors.New("EBUSY")}
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", map[string]any{"interface": "eth0", "up": true})); err == nil {
		t.Error("up error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{})
	bad := decl("l", map[string]any{}) // missing interface + no reconcile target
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

// --- module surface --------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "network" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 1 || got[0] != StatePresent {
		t.Errorf("ValidStates=%v (should be present-only)", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("network should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", map[string]any{"interface": "eth0", "up": true}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", map[string]any{"interface": "eth0", "up": true})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", map[string]any{})); err == nil {
		t.Error("missing interface should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: false, MTU: 1500}}
	m := NewWithProvider(f)
	d := decl("l", map[string]any{"interface": "eth0", "up": true})
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
	if !IsInterfaceNotFound(ErrInterfaceNotFound) || IsInterfaceNotFound(errors.New("x")) {
		t.Error("IsInterfaceNotFound")
	}
}
