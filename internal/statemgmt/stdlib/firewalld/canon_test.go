// SPDX-License-Identifier: Apache-2.0

package firewalld

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func TestCanonicalizeRichRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"collapse whitespace",
			`rule   family="ipv4"   service name="ssh"   accept`,
			`rule family="ipv4" service name="ssh" accept`,
		},
		{
			"add quotes",
			`rule family=ipv4 service name=ssh accept`,
			`rule family="ipv4" service name="ssh" accept`,
		},
		{
			"sort attributes within an element",
			`rule family="ipv4" port protocol="tcp" port="80" accept`,
			`rule family="ipv4" port port="80" protocol="tcp" accept`,
		},
		{
			"forward-port attribute order",
			`rule forward-port to-port="8080" protocol="tcp" port="80"`,
			`rule forward-port port="80" protocol="tcp" to-port="8080"`,
		},
		{
			"source not address",
			`rule  family="ipv4"  source  not  address="10.0.0.0/8"  drop`,
			`rule family="ipv4" source not address="10.0.0.0/8" drop`,
		},
		{
			"log prefix with internal spaces is preserved",
			`rule family="ipv4" log prefix="drop: bad host" level="info" drop`,
			`rule family="ipv4" log level="info" prefix="drop: bad host" drop`,
		},
		{
			"bare actions only",
			`rule masquerade`,
			`rule masquerade`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canonicalizeRichRule(tc.in)
			if got != tc.want {
				t.Errorf("canon = %q, want %q", got, tc.want)
			}
			// idempotent: canon(canon(x)) == canon(x)
			if again := canonicalizeRichRule(got); again != got {
				t.Errorf("not idempotent: %q → %q", got, again)
			}
		})
	}
}

func TestCanonicalizeRichRule_EquivalentForms(t *testing.T) {
	t.Parallel()
	// Two rules differing only in whitespace, quoting, and attribute
	// order must canonicalise to the same string.
	a := `rule family="ipv4" source address="10.0.0.0/8" port port="22" protocol="tcp" accept`
	b := `rule  family=ipv4  source address=10.0.0.0/8  port protocol=tcp port=22  accept`
	if canonicalizeRichRule(a) != canonicalizeRichRule(b) {
		t.Errorf("equivalent rules differ:\n  %q\n  %q", canonicalizeRichRule(a), canonicalizeRichRule(b))
	}
}

func TestSplitAttr(t *testing.T) {
	t.Parallel()
	if k, v, ok := splitAttr(`port="80"`); !ok || k != "port" || v != "80" {
		t.Errorf(`splitAttr(port="80") = %q %q %v`, k, v, ok)
	}
	if k, v, ok := splitAttr(`family=ipv4`); !ok || k != "family" || v != "ipv4" {
		t.Errorf("splitAttr(family=ipv4) = %q %q %v", k, v, ok)
	}
	// bare keywords are not attributes
	for _, bare := range []string{"rule", "accept", "source", "not", "masquerade"} {
		if _, _, ok := splitAttr(bare); ok {
			t.Errorf("splitAttr(%q) should not be an attribute", bare)
		}
	}
	// a leading '=' is not a valid attribute
	if _, _, ok := splitAttr(`="x"`); ok {
		t.Error("leading '=' should not be an attribute")
	}
}

// --- module rich-rule idempotency via canonical compare ----------------

func richDecl(state, rule string) *statemgmt.Declaration {
	return decl("r", state, map[string]any{"rich_rule": rule, "zone": "public"})
}

func TestRichRule_ReformattedMatchesStored(t *testing.T) {
	t.Parallel()
	// firewalld stores a canonical form; the operator declares the same
	// rule with different whitespace / quoting / attribute order. It
	// must read as present (converged), not drift.
	f := &fakeProvider{richRules: []string{`rule family="ipv4" port port="22" protocol="tcp" accept`}}
	m := NewWithProvider(f)
	d := richDecl(StatePresent, `rule  family=ipv4  port protocol=tcp port=22  accept`)

	res, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matches {
		t.Error("a re-formatted rich rule should match the stored canonical form")
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed || len(f.calls) != 0 {
		t.Errorf("converged rich rule must be a no-op; got %+v calls=%+v", sr, f.calls)
	}
}

func TestRichRule_AbsentAddsThenIdempotent(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{} // no stored rules
	m := NewWithProvider(f)
	d := richDecl(StatePresent, `rule family="ipv4" service name="ssh" accept`)

	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.calls) != 2 || f.calls[0].op != "add" || f.calls[1].op != "reload" {
		t.Fatalf("first apply should add + reload; got %+v", f.calls)
	}
	// the fake recorded the added rule; a second apply is a no-op
	f.calls = nil
	sr2, _ := m.Apply(context.Background(), d)
	if sr2.Changed || len(f.calls) != 0 {
		t.Errorf("second apply should be a no-op; got %+v calls=%+v", sr2, f.calls)
	}
}

func TestRichRule_ListErrorPropagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{listErr: context.Canceled}
	m := NewWithProvider(f)
	if _, err := m.Check(context.Background(), richDecl(StatePresent, `rule masquerade`)); err == nil {
		t.Error("ListRichRules error should propagate")
	}
}
