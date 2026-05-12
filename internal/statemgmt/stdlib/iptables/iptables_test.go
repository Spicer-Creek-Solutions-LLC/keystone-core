package iptables

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "iptables:" + name,
		Module: "iptables",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type rec struct {
	op            string // "add" / "del" / "save"
	family, table string
	chain         string
	position      int
	rule          []string
	path          string
}

type fakeProvider struct {
	present   bool // what HasRule reports (toggled by Add/Delete)
	calls     []rec
	lookups   int
	lookupErr error
	addErr    error
	delErr    error
	saveErr   error
}

func (f *fakeProvider) HasRule(_ context.Context, _, _, _ string, _ []string) (bool, error) {
	f.lookups++
	if f.lookupErr != nil {
		return false, f.lookupErr
	}
	return f.present, nil
}
func (f *fakeProvider) AddRule(_ context.Context, fam, table, chain string, pos int, rule []string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.calls = append(f.calls, rec{op: "add", family: fam, table: table, chain: chain, position: pos, rule: rule})
	f.present = true
	return nil
}
func (f *fakeProvider) DeleteRule(_ context.Context, fam, table, chain string, rule []string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.calls = append(f.calls, rec{op: "del", family: fam, table: table, chain: chain, rule: rule})
	f.present = false
	return nil
}
func (f *fakeProvider) Save(_ context.Context, fam, path string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.calls = append(f.calls, rec{op: "save", family: fam, path: path})
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"chains": "INPUT"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParseRule(t *testing.T) {
	t.Parallel()
	got, err := parseRule("-p tcp --dport 22 -j ACCEPT")
	if err != nil || !reflect.DeepEqual(got, []string{"-p", "tcp", "--dport", "22", "-j", "ACCEPT"}) {
		t.Fatalf("string rule: %v %v", got, err)
	}
	got, err = parseRule([]any{"-p", "tcp", "-j", "DROP"})
	if err != nil || !reflect.DeepEqual(got, []string{"-p", "tcp", "-j", "DROP"}) {
		t.Errorf("list rule: %v %v", got, err)
	}
	for _, bad := range []any{"", "   ", []any{}, []any{"a", ""}, []any{1}, []any{"a\nb"}, 7} {
		if _, err := parseRule(bad); err == nil {
			t.Errorf("parseRule(%v) should error", bad)
		}
	}
}

func TestParsePosition(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in any
		ok bool
		n  int
	}{{0, true, 0}, {int64(3), true, 3}, {float64(2), true, 2}, {"5", true, 5}, {-1, false, 0}, {float64(1.5), false, 0}, {"x", false, 0}, {true, false, 0}} {
		got, err := parsePosition(c.in)
		if c.ok {
			if err != nil || got != c.n {
				t.Errorf("parsePosition(%v) = %d,%v want %d", c.in, got, err, c.n)
			}
		} else if err == nil {
			t.Errorf("parsePosition(%v): expected error", c.in)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	r := func() any { return "-p tcp --dport 22 -j ACCEPT" }
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok", decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": r()}), false},
		{"present ok ipv6 nat insert", decl("l", StatePresent, map[string]any{"chain": "POSTROUTING", "table": "nat", "family": "ipv6", "rule": "-o eth0 -j MASQUERADE", "position": 1}), false},
		{"needs chain", decl("l", StatePresent, map[string]any{"rule": r()}), true},
		{"bad chain", decl("l", StatePresent, map[string]any{"chain": "IN PUT", "rule": r()}), true},
		{"needs rule", decl("l", StatePresent, map[string]any{"chain": "INPUT"}), true},
		{"bad table", decl("l", StatePresent, map[string]any{"chain": "INPUT", "table": "bogus", "rule": r()}), true},
		{"bad family", decl("l", StatePresent, map[string]any{"chain": "INPUT", "family": "ipv5", "rule": r()}), true},
		{"banned -t in rule", decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-t nat -j ACCEPT"}), true},
		{"banned -A in rule", decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-A INPUT -j ACCEPT"}), true},
		{"relative save", decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": r(), "save": "rules.v4"}), true},
		{"negative position", decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": r(), "position": -1}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"chain": "INPUT", "rule": r()}), false},
		{"absent allows save", decl("l", StateAbsent, map[string]any{"chain": "INPUT", "rule": r(), "save": "/etc/iptables/rules.v4"}), false},
		{"absent rejects position", decl("l", StateAbsent, map[string]any{"chain": "INPUT", "rule": r(), "position": 1}), true},
		{"bad state", decl("l", "frob", map[string]any{"chain": "INPUT", "rule": r()}), true},
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

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Present(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: false}
	m := NewWithProvider(f)
	d := decl("allow-ssh", StatePresent, map[string]any{"chain": "INPUT", "rule": "-p tcp --dport 22 -j ACCEPT"})

	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("rule absent → should drift")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if len(f.calls) != 1 || f.calls[0].op != "add" || f.calls[0].table != "filter" || f.calls[0].chain != "INPUT" || f.calls[0].position != 0 {
		t.Fatalf("Add call wrong: %+v", f.calls)
	}
	if !reflect.DeepEqual(f.calls[0].rule, []string{"-p", "tcp", "--dport", "22", "-j", "ACCEPT"}) {
		t.Errorf("Add rule = %v", f.calls[0].rule)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("should match after apply, diff=%q", r.Diff)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}
}

func TestApply_Present_InsertAndSave(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: false}
	m := NewWithProvider(f)
	d := decl("masq", StatePresent, map[string]any{
		"chain": "POSTROUTING", "table": "nat", "family": "ipv6",
		"rule": []any{"-o", "eth0", "-j", "MASQUERADE"}, "position": 2,
		"save": "/etc/iptables/rules.v6",
	})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("expected add + save, got %+v", f.calls)
	}
	add := f.calls[0]
	if add.op != "add" || add.family != "ipv6" || add.table != "nat" || add.chain != "POSTROUTING" || add.position != 2 {
		t.Errorf("Add call wrong: %+v", add)
	}
	sv := f.calls[1]
	if sv.op != "save" || sv.family != "ipv6" || sv.path != "/etc/iptables/rules.v6" {
		t.Errorf("Save call wrong: %+v", sv)
	}
	// converged → no save on a no-op
	f.calls = nil
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Error("a no-op apply should not Save")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{present: true}
	m := NewWithProvider(f)
	d := decl("drop-it", StateAbsent, map[string]any{"chain": "INPUT", "rule": "-s 10.0.0.5 -j DROP", "save": "/etc/iptables/rules.v4"})

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
	// one delete + one save
	dels, saves := 0, 0
	for _, c := range f.calls {
		switch c.op {
		case "del":
			dels++
		case "save":
			saves++
		}
	}
	if dels != 1 || saves != 1 {
		t.Errorf("expected 1 del + 1 save, got %+v", f.calls)
	}
	// already absent → no-op
	f.calls = nil
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || len(f.calls) != 0 {
		t.Errorf("absent on a missing rule should be a no-op: changed=%v calls=%+v", sr.Changed, f.calls)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("absent should match once the rule is gone")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	r := map[string]any{"chain": "INPUT", "rule": "-j ACCEPT"}
	// lookup error
	if _, err := NewWithProvider(&fakeProvider{lookupErr: errors.New("bad chain")}).Check(context.Background(), decl("l", StatePresent, r)); err == nil {
		t.Error("lookup error should propagate from Check")
	}
	// add error
	if _, err := NewWithProvider(&fakeProvider{present: false, addErr: errors.New("nope")}).Apply(context.Background(), decl("l", StatePresent, r)); err == nil {
		t.Error("add error should propagate")
	}
	// delete error
	if _, err := NewWithProvider(&fakeProvider{present: true, delErr: errors.New("nope")}).Apply(context.Background(), decl("l", StateAbsent, r)); err == nil {
		t.Error("delete error should propagate")
	}
	// save error after a successful add
	if _, err := NewWithProvider(&fakeProvider{present: false, saveErr: errors.New("disk full")}).Apply(context.Background(),
		decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-j ACCEPT", "save": "/etc/iptables/rules.v4"})); err == nil {
		t.Error("save error should propagate")
	}
}

func TestApply_Absent_DeleteLoopGivesUp(t *testing.T) {
	t.Parallel()
	// a pathological provider whose DeleteRule never actually
	// removes the rule — Apply must give up, not spin.
	f := &fakeProvider{present: true}
	// override Delete to be a no-op (don't flip present)
	bad := &stuckProvider{fakeProvider: f}
	if _, err := NewWithProvider(bad).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"chain": "INPUT", "rule": "-j DROP"})); err == nil {
		t.Error("a never-removing delete should make Apply give up with an error")
	}
}

type stuckProvider struct{ *fakeProvider }

func (s *stuckProvider) DeleteRule(context.Context, string, string, string, []string) error {
	return nil // pretend success but don't change `present`
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "iptables" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("iptables should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-j ACCEPT"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("l", StateAbsent, map[string]any{"chain": "INPUT", "rule": "-j ACCEPT"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-j ACCEPT"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"chain": "INPUT"})); err == nil {
		t.Error("present without rule should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{present: false})
	d := decl("l", StatePresent, map[string]any{"chain": "INPUT", "rule": "-j ACCEPT"})
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
	if !IsNoIptables(ErrNoIptables) || IsNoIptables(errors.New("x")) {
		t.Error("IsNoIptables")
	}
}
