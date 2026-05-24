// SPDX-License-Identifier: Apache-2.0

package firewalld

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "firewalld:" + name,
		Module: "firewalld",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type rec struct {
	op   string // "add" / "remove" / "reload"
	zone string
	item Item
}

type fakeProvider struct {
	present   bool
	calls     []rec
	lookups   int
	lookupErr error
	addErr    error
	removeErr error
	reloadErr error
}

func (f *fakeProvider) Has(_ context.Context, _ string, _ Item) (bool, error) {
	f.lookups++
	if f.lookupErr != nil {
		return false, f.lookupErr
	}
	return f.present, nil
}
func (f *fakeProvider) Add(_ context.Context, zone string, it Item) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.calls = append(f.calls, rec{op: "add", zone: zone, item: it})
	f.present = true
	return nil
}
func (f *fakeProvider) Remove(_ context.Context, zone string, it Item) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.calls = append(f.calls, rec{op: "remove", zone: zone, item: it})
	f.present = false
	return nil
}
func (f *fakeProvider) Reload(_ context.Context) error {
	if f.reloadErr != nil {
		return f.reloadErr
	}
	f.calls = append(f.calls, rec{op: "reload"})
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"services": "ssh"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_DefaultsAndTypes(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"service": "ssh"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Zone != "public" || !p.Reload || p.Item.Kind != KindService || p.Item.Value != "ssh" {
		t.Errorf("defaults wrong: %+v", p)
	}
	// reload=false survives
	p, err = parseParams(decl("l", StatePresent, map[string]any{"service": "ssh", "reload": false}))
	if err != nil || p.Reload {
		t.Errorf("reload=false: %+v %v", p, err)
	}
	// type errors
	for _, bad := range []map[string]any{
		{"zone": 1, "service": "ssh"},
		{"service": 1},
		{"port": 1},
		{"rich_rule": 1},
		{"service": "ssh", "reload": "no"},
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
		{"present service ok", decl("l", StatePresent, map[string]any{"service": "ssh"}), false},
		{"present port ok", decl("l", StatePresent, map[string]any{"port": "8080/tcp", "zone": "trusted"}), false},
		{"present port range ok", decl("l", StatePresent, map[string]any{"port": "1000-2000/udp"}), false},
		{"present rich_rule ok", decl("l", StatePresent, map[string]any{"rich_rule": `rule family="ipv4" source address="10.0.0.0/8" drop`}), false},
		{"needs one item", decl("l", StatePresent, map[string]any{}), true},
		{"two items rejected", decl("l", StatePresent, map[string]any{"service": "ssh", "port": "80/tcp"}), true},
		{"all three items rejected", decl("l", StatePresent, map[string]any{"service": "ssh", "port": "80/tcp", "rich_rule": "rule drop"}), true},
		{"bad zone charset", decl("l", StatePresent, map[string]any{"zone": "pu blic", "service": "ssh"}), true},
		{"empty zone", decl("l", StatePresent, map[string]any{"zone": "  ", "service": "ssh"}), true},
		{"empty service", decl("l", StatePresent, map[string]any{"service": "   "}), true},
		{"bad service charset", decl("l", StatePresent, map[string]any{"service": "ss h"}), true},
		{"bad port — missing proto", decl("l", StatePresent, map[string]any{"port": "80"}), true},
		{"bad port — bad proto", decl("l", StatePresent, map[string]any{"port": "80/icmp"}), true},
		{"bad port — non-numeric", decl("l", StatePresent, map[string]any{"port": "ssh/tcp"}), true},
		{"newline in rich_rule", decl("l", StatePresent, map[string]any{"rich_rule": "rule drop\nrule accept"}), true},
		{"NUL in rich_rule", decl("l", StatePresent, map[string]any{"rich_rule": "rule\x00accept"}), true},
		{"absent service ok", decl("l", StateAbsent, map[string]any{"service": "ssh"}), false},
		{"bad state", decl("l", "frob", map[string]any{"service": "ssh"}), true},
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

func TestParamKeyFor(t *testing.T) {
	t.Parallel()
	if paramKeyFor(KindService) != "service" || paramKeyFor(KindPort) != "port" || paramKeyFor(KindRichRule) != "rich_rule" {
		t.Error("paramKeyFor returned wrong key")
	}
	if paramKeyFor("frob") != "frob" {
		t.Error("paramKeyFor should pass through unknown kinds")
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Present_Service(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: false}
	m := NewWithProvider(f)
	d := decl("allow-ssh", StatePresent, map[string]any{"service": "ssh"})

	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("service absent → should drift")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	// expect add + reload
	if len(f.calls) != 2 || f.calls[0].op != "add" || f.calls[1].op != "reload" {
		t.Fatalf("expected add + reload, got %+v", f.calls)
	}
	a := f.calls[0]
	if a.zone != "public" || a.item.Kind != KindService || a.item.Value != "ssh" {
		t.Errorf("Add call wrong: %+v", a)
	}
	// converged → no-op
	f.calls = nil
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" || len(f.calls) != 0 {
		t.Errorf("second apply should be a no-op: changed=%v comment=%q calls=%+v", sr.Changed, sr.Comment, f.calls)
	}
}

func TestApply_Present_Port_CustomZone_NoReload(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: false}
	m := NewWithProvider(f)
	d := decl("allow-8080", StatePresent, map[string]any{"port": "8080/tcp", "zone": "dmz", "reload": false})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0].op != "add" {
		t.Fatalf("reload=false should skip reload, got %+v", f.calls)
	}
	a := f.calls[0]
	if a.zone != "dmz" || a.item.Kind != KindPort || a.item.Value != "8080/tcp" {
		t.Errorf("Add call wrong: %+v", a)
	}
}

func TestCheckApply_Absent_RichRule(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: true}
	m := NewWithProvider(f)
	d := decl("no-bad-host", StateAbsent, map[string]any{"rich_rule": `rule family="ipv4" source address="10.0.0.5" drop`})

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("rule present → should drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	// expect remove + reload
	if len(f.calls) != 2 || f.calls[0].op != "remove" || f.calls[1].op != "reload" {
		t.Fatalf("expected remove + reload, got %+v", f.calls)
	}
	if f.calls[0].item.Kind != KindRichRule {
		t.Errorf("Remove call wrong: %+v", f.calls[0])
	}
	// already absent → no-op
	f.calls = nil
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || len(f.calls) != 0 {
		t.Errorf("absent on a missing item should be a no-op: changed=%v calls=%+v", sr.Changed, f.calls)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("absent should match once the item is gone")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	base := map[string]any{"service": "ssh"}
	mk := func(f *fakeProvider) statemgmt.Module { return NewWithProvider(f) }

	if _, err := mk(&fakeProvider{lookupErr: errors.New("boom")}).Check(context.Background(), decl("l", StatePresent, base)); err == nil {
		t.Error("lookup error should propagate from Check")
	}
	if _, err := mk(&fakeProvider{present: false, addErr: errors.New("nope")}).Apply(context.Background(), decl("l", StatePresent, base)); err == nil {
		t.Error("add error should propagate")
	}
	if _, err := mk(&fakeProvider{present: true, removeErr: errors.New("nope")}).Apply(context.Background(), decl("l", StateAbsent, base)); err == nil {
		t.Error("remove error should propagate")
	}
	if _, err := mk(&fakeProvider{present: false, reloadErr: errors.New("dbus down")}).Apply(context.Background(), decl("l", StatePresent, base)); err == nil {
		t.Error("reload error after a successful add should propagate")
	}
	if _, err := mk(&fakeProvider{present: true, reloadErr: errors.New("dbus down")}).Apply(context.Background(), decl("l", StateAbsent, base)); err == nil {
		t.Error("reload error after a successful remove should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{})
	bad := decl("l", StatePresent, map[string]any{}) // no item
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "firewalld" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("firewalld should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	good := decl("l", StatePresent, map[string]any{"service": "ssh"})
	if dsm.DriftSeverity(good, nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("l", StateAbsent, map[string]any{"service": "ssh"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(good); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing item should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: false}
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"service": "ssh"})
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
	if !IsNoFirewallCmd(ErrNoFirewallCmd) || IsNoFirewallCmd(errors.New("x")) {
		t.Error("IsNoFirewallCmd")
	}
}
