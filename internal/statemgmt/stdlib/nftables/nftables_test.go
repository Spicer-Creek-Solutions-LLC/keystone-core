// SPDX-License-Identifier: Apache-2.0

package nftables

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "nftables:" + name,
		Module: "nftables",
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
	index         int
	handle        int
	rule          []string
	path          string
}

type fakeProvider struct {
	// rules is the modelled chain contents: text → count of copies.
	// matchingHandles compares against strings.Join(rule, " ").
	rules    map[string]int
	wantText string // the text the module will look for; set by tests
	nextH    int
	calls    []rec
	lists    int
	listErr  error
	addErr   error
	delErr   error
	saveErr  error
}

func newFake(present int, wantText string) *fakeProvider {
	f := &fakeProvider{rules: map[string]int{}, wantText: wantText, nextH: 100}
	if present > 0 {
		f.rules[wantText] = present
	}
	return f
}

func (f *fakeProvider) ListRuleHandles(_ context.Context, _, _, _ string) ([]RuleHandle, error) {
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []RuleHandle
	for text, n := range f.rules {
		for i := 0; i < n; i++ {
			f.nextH++
			out = append(out, RuleHandle{Text: text, Handle: f.nextH})
		}
	}
	return out, nil
}

func (f *fakeProvider) AddRule(_ context.Context, fam, table, chain string, index int, rule []string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.calls = append(f.calls, rec{op: "add", family: fam, table: table, chain: chain, index: index, rule: rule})
	f.rules[f.wantText]++
	return nil
}

func (f *fakeProvider) DeleteRule(_ context.Context, fam, table, chain string, handle int) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.calls = append(f.calls, rec{op: "del", family: fam, table: table, chain: chain, handle: handle})
	if f.rules[f.wantText] > 0 {
		f.rules[f.wantText]--
		if f.rules[f.wantText] == 0 {
			delete(f.rules, f.wantText)
		}
	}
	return nil
}

func (f *fakeProvider) SaveRuleset(_ context.Context, path string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.calls = append(f.calls, rec{op: "save", path: path})
	return nil
}

func (f *fakeProvider) opCounts() (add, del, save int) {
	for _, c := range f.calls {
		switch c.op {
		case "add":
			add++
		case "del":
			del++
		case "save":
			save++
		}
	}
	return
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"chains": "input"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_DefaultsAndTypes(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "tcp dport 22 accept"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Family != "inet" || p.Index != -1 || p.indexSet {
		t.Errorf("defaults wrong: %+v", p)
	}
	for _, bad := range []map[string]any{
		{"family": 1, "table": "t", "chain": "c", "rule": "accept"},
		{"table": 1, "chain": "c", "rule": "accept"},
		{"table": "t", "chain": 1, "rule": "accept"},
		{"table": "t", "chain": "c", "rule": "accept", "save": 1},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestParseRule(t *testing.T) {
	t.Parallel()
	got, err := parseRule("tcp dport 22 accept")
	if err != nil || !reflect.DeepEqual(got, []string{"tcp", "dport", "22", "accept"}) {
		t.Fatalf("string rule: %v %v", got, err)
	}
	got, err = parseRule([]any{"ct", "state", "established,related", "accept"})
	if err != nil || !reflect.DeepEqual(got, []string{"ct", "state", "established,related", "accept"}) {
		t.Errorf("list rule: %v %v", got, err)
	}
	for _, bad := range []any{"", "   ", []any{}, []any{"a", ""}, []any{1}, []any{"a\nb"}, []any{"accept;flush"}, "a;b", 7} {
		if _, err := parseRule(bad); err == nil {
			t.Errorf("parseRule(%v) should error", bad)
		}
	}
}

func TestParseIndex(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in any
		ok bool
		n  int
	}{{0, true, 0}, {int64(3), true, 3}, {float64(2), true, 2}, {"5", true, 5}, {-1, false, 0}, {float64(1.5), false, 0}, {"x", false, 0}, {true, false, 0}} {
		got, err := parseIndex(c.in)
		if c.ok {
			if err != nil || got != c.n {
				t.Errorf("parseIndex(%v) = %d,%v want %d", c.in, got, err, c.n)
			}
		} else if err == nil {
			t.Errorf("parseIndex(%v): expected error", c.in)
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
		{"present ok", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "tcp dport 22 accept"}), false},
		{"present ok ip6 index", decl("l", StatePresent, map[string]any{"family": "ip6", "table": "filter", "chain": "forward", "rule": "drop", "index": 0}), false},
		{"needs table", decl("l", StatePresent, map[string]any{"chain": "input", "rule": "accept"}), true},
		{"needs chain", decl("l", StatePresent, map[string]any{"table": "filter", "rule": "accept"}), true},
		{"needs rule", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input"}), true},
		{"bad family", decl("l", StatePresent, map[string]any{"family": "ipv4", "table": "filter", "chain": "input", "rule": "accept"}), true},
		{"bad table name", decl("l", StatePresent, map[string]any{"table": "fi lter", "chain": "input", "rule": "accept"}), true},
		{"bad chain name", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "in put", "rule": "accept"}), true},
		{"banned rule head 'add'", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "add rule inet filter input accept"}), true},
		{"banned rule head 'flush'", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "flush ruleset"}), true},
		{"relative save", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "accept", "save": "ruleset.nft"}), true},
		{"negative index", decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "accept", "index": -1}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "tcp dport 22 accept"}), false},
		{"absent allows save", decl("l", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "accept", "save": "/etc/nftables.conf"}), false},
		{"absent rejects index", decl("l", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "accept", "index": 0}), true},
		{"bad state", decl("l", "frob", map[string]any{"table": "filter", "chain": "input", "rule": "accept"}), true},
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
	f := newFake(0, "tcp dport 22 accept")
	m := NewWithProvider(f)
	d := decl("allow-ssh", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "tcp dport 22 accept"})

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
	add, del, save := f.opCounts()
	if add != 1 || del != 0 || save != 0 {
		t.Fatalf("op counts add=%d del=%d save=%d, calls=%+v", add, del, save, f.calls)
	}
	a := f.calls[0]
	if a.family != "inet" || a.table != "filter" || a.chain != "input" || a.index != -1 {
		t.Errorf("Add call wrong: %+v", a)
	}
	if !reflect.DeepEqual(a.rule, []string{"tcp", "dport", "22", "accept"}) {
		t.Errorf("Add rule = %v", a.rule)
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
	f := newFake(0, "tcp dport 80 accept")
	m := NewWithProvider(f)
	d := decl("allow-http", StatePresent, map[string]any{
		"family": "ip", "table": "filter", "chain": "input",
		"rule": []any{"tcp", "dport", "80", "accept"}, "index": 2,
		"save": "/etc/nftables.conf",
	})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	add, del, save := f.opCounts()
	if add != 1 || del != 0 || save != 1 {
		t.Fatalf("expected add + save, got %+v", f.calls)
	}
	a := f.calls[0]
	if a.op != "add" || a.family != "ip" || a.index != 2 {
		t.Errorf("Add call wrong: %+v", a)
	}
	sv := f.calls[1]
	if sv.op != "save" || sv.path != "/etc/nftables.conf" {
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

func TestCheckApply_Absent_RemovesDuplicates(t *testing.T) {
	t.Parallel()
	f := newFake(3, "ip saddr 10.0.0.5 drop") // three copies of the rule
	m := NewWithProvider(f)
	d := decl("no-bad-host", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "ip saddr 10.0.0.5 drop", "save": "/etc/nftables.conf"})

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
	add, del, save := f.opCounts()
	if add != 0 || del != 3 || save != 1 {
		t.Errorf("expected 3 del + 1 save, got %+v", f.calls)
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
	base := map[string]any{"table": "filter", "chain": "input", "rule": "accept"}
	mk := func(f *fakeProvider) statemgmt.Module { return NewWithProvider(f) }

	if _, err := mk(&fakeProvider{rules: map[string]int{}, listErr: errors.New("boom")}).Check(context.Background(), decl("l", StatePresent, base)); err == nil {
		t.Error("list error should propagate from Check")
	}
	if _, err := mk(&fakeProvider{rules: map[string]int{}, wantText: "accept", addErr: errors.New("nope")}).Apply(context.Background(), decl("l", StatePresent, base)); err == nil {
		t.Error("add error should propagate")
	}
	if _, err := mk(&fakeProvider{rules: map[string]int{"accept": 1}, wantText: "accept", delErr: errors.New("nope")}).Apply(context.Background(), decl("l", StateAbsent, base)); err == nil {
		t.Error("delete error should propagate")
	}
	if _, err := mk(&fakeProvider{rules: map[string]int{}, wantText: "accept", saveErr: errors.New("disk full")}).Apply(context.Background(),
		decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "accept", "save": "/etc/nftables.conf"})); err == nil {
		t.Error("save error should propagate")
	}
}

func TestApply_Absent_DeleteLoopGivesUp(t *testing.T) {
	t.Parallel()
	// a pathological provider whose DeleteRule never actually removes
	// the rule — Apply must give up, not spin.
	f := newFake(1, "drop")
	bad := &stuckProvider{fakeProvider: f}
	if _, err := NewWithProvider(bad).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "drop"})); err == nil {
		t.Error("a never-removing delete should make Apply give up with an error")
	}
}

type stuckProvider struct{ *fakeProvider }

func (s *stuckProvider) DeleteRule(context.Context, string, string, string, int) error {
	return nil // pretend success but don't change the modelled chain
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake(0, ""))
	bad := decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input"}) // missing rule
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
	if m.Name() != "nftables" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("nftables should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	good := decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "accept"})
	if dsm.DriftSeverity(good, nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("l", StateAbsent, map[string]any{"table": "filter", "chain": "input", "rule": "accept"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(good); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input"})); err == nil {
		t.Error("present without rule should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake(0, "accept")
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"table": "filter", "chain": "input", "rule": "accept"})
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
	if !IsNoNft(ErrNoNft) || IsNoNft(errors.New("x")) {
		t.Error("IsNoNft")
	}
}
