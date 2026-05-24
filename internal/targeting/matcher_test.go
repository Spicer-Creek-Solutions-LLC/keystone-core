// SPDX-License-Identifier: Apache-2.0

package targeting

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"

	"go.keystone-core.io/keystone-core/internal/state"
)

func TestMatcher_Match(t *testing.T) {
	t.Parallel()

	web := state.AgentRecord{
		ID:           "web-01",
		Hostname:     "web-prod-01",
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{"10.0.1.5", "fe80::1"},
		Labels:       map[string]string{"role": "web", "env": "prod"},
		Status:       state.AgentStatusConnected,
	}
	dbStale := state.AgentRecord{
		ID:           "db-02",
		Hostname:     "db-prod-02",
		OS:           "linux",
		Architecture: "arm64",
		IPAddresses:  []string{"192.168.10.4"},
		Labels:       map[string]string{"role": "db", "env": "prod"},
		Status:       state.AgentStatusStale,
	}
	cacheDev := state.AgentRecord{
		ID:           "cache-07",
		Hostname:     "cache-dev-07",
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{"10.0.2.9"},
		Labels:       map[string]string{"role": "cache", "env": "dev"},
		Status:       state.AgentStatusConnected,
	}
	bare := state.AgentRecord{ID: "bare", Status: state.AgentStatusPending}

	cases := []struct {
		name string
		expr string
		rec  state.AgentRecord
		want bool
	}{
		// Builtin direct matches.
		{name: "id glob hit", expr: "id:web-*", rec: web, want: true},
		{name: "id glob miss", expr: "id:db-*", rec: web, want: false},
		{name: "hostname literal hit", expr: `hostname:"web-prod-01"`, rec: web, want: true},
		{name: "hostname glob", expr: "hostname:db-prod-*", rec: dbStale, want: true},
		{name: "os hit", expr: "os:linux", rec: web, want: true},
		{name: "os miss", expr: "os:darwin", rec: web, want: false},
		{name: "arch hit", expr: "arch:amd64", rec: web, want: true},
		{name: "arch miss", expr: "arch:arm64", rec: web, want: false},

		// Status normalization (connected → online).
		{name: "status online hit", expr: "status:online", rec: web, want: true},
		{name: "status online miss for stale", expr: "status:online", rec: dbStale, want: false},
		{name: "status stale hit", expr: "status:stale", rec: dbStale, want: true},
		{name: "status pending hit", expr: "status:pending", rec: bare, want: true},

		// IP / CIDR.
		{name: "ip cidr ipv4 hit", expr: "ip:10.0.0.0/8", rec: web, want: true},
		{name: "ip cidr ipv4 miss", expr: "ip:172.16.0.0/12", rec: web, want: false},
		{name: "ip cidr narrow hit", expr: "ip:10.0.1.0/24", rec: web, want: true},
		{name: "ip cidr narrow miss db", expr: "ip:10.0.0.0/8", rec: dbStale, want: false},
		{name: "ip cidr ipv6 hit", expr: "ip:fe80::/10", rec: web, want: true},
		{name: "ip literal single element", expr: "ip:10.0.1.5", rec: web, want: true},

		// Label sugar.
		{name: "label sugar role hit", expr: "role:web", rec: web, want: true},
		{name: "label sugar role miss", expr: "role:web", rec: dbStale, want: false},
		{name: "labels prefix hit", expr: "labels.env:prod", rec: web, want: true},
		{name: "labels prefix miss", expr: "labels.env:dev", rec: web, want: false},
		{name: "missing label", expr: "role:web", rec: bare, want: false},

		// Compound expressions.
		{name: "AND both true", expr: "role:web AND env:prod", rec: web, want: true},
		{name: "AND one false", expr: "role:web AND env:dev", rec: web, want: false},
		{name: "OR first true", expr: "role:db OR role:cache", rec: dbStale, want: true},
		{name: "OR second true", expr: "role:db OR role:cache", rec: cacheDev, want: true},
		{name: "OR none true", expr: "role:db OR role:cache", rec: web, want: false},
		{name: "NOT hit", expr: "NOT role:legacy", rec: web, want: true},
		{name: "NOT miss", expr: "NOT role:web", rec: web, want: false},
		{name: "NOT AND mix", expr: "NOT role:legacy AND env:prod", rec: web, want: true},
		{name: "parens precedence", expr: "(role:db OR role:cache) AND status:online", rec: dbStale, want: false},
		{name: "parens precedence hit", expr: "(role:web OR role:cache) AND env:prod", rec: web, want: true},
		{name: "three-way AND", expr: "id:web-* AND status:online AND ip:10.0.0.0/8", rec: web, want: true},
		{name: "three-way AND miss on status", expr: "id:db-* AND status:online AND ip:192.168.0.0/16", rec: dbStale, want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			te, err := Compile(tc.expr)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tc.expr, err)
			}
			m := NewMatcher(te)
			got, err := m.Match(tc.rec)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			if got != tc.want {
				t.Errorf("Match(%q, %s) = %v, want %v", tc.expr, tc.rec.ID, got, tc.want)
			}
		})
	}
}

func TestMatcher_Nil(t *testing.T) {
	t.Parallel()

	var nilMatcher *Matcher
	if _, err := nilMatcher.Match(state.AgentRecord{}); err == nil {
		t.Error("nil matcher: expected error, got nil")
	}

	emptyMatcher := NewMatcher(nil)
	if _, err := emptyMatcher.Match(state.AgentRecord{}); err == nil {
		t.Error("matcher with nil expression: expected error, got nil")
	}

	noProgram := NewMatcher(&TargetExpression{Raw: "x", Translated: "x"})
	if _, err := noProgram.Match(state.AgentRecord{}); err == nil {
		t.Error("matcher with nil program: expected error, got nil")
	}
}

func TestMatcher_RunError(t *testing.T) {
	t.Parallel()

	// Compile a program that triggers a runtime error in the expr VM
	// (integer division by zero) so the Matcher's expr.Run wrapper
	// path is exercised. The error is wrapped with a "run" prefix.
	prog, err := expr.Compile("[1,2,3][10]")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m := NewMatcher(&TargetExpression{Raw: "oob", Translated: "[1,2,3][10]", Program: prog})
	_, runErr := m.Match(state.AgentRecord{ID: "x"})
	if runErr == nil {
		t.Fatal("expected run error, got nil")
	}
	if !strings.Contains(runErr.Error(), "run ") {
		t.Errorf("error %q should reference the run failure", runErr.Error())
	}
}

func TestMatcher_NonBoolResult(t *testing.T) {
	t.Parallel()

	// Compile a program whose result is a string, then bypass AsBool by
	// hand-building the TargetExpression. Match must reject the
	// non-bool result rather than coerce it.
	prog, err := expr.Compile(`"hello"`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m := NewMatcher(&TargetExpression{Raw: `"hello"`, Translated: `"hello"`, Program: prog})
	_, err = m.Match(state.AgentRecord{})
	if err == nil {
		t.Fatal("expected non-bool error, got nil")
	}
	if !strings.Contains(err.Error(), "want bool") {
		t.Errorf("error %q should mention bool requirement", err.Error())
	}
}

func TestMatcher_Expression(t *testing.T) {
	t.Parallel()

	te, err := Compile("os:linux")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	m := NewMatcher(te)
	if got := m.Expression(); got != te {
		t.Errorf("Expression() = %v, want %v", got, te)
	}

	var nilM *Matcher
	if got := nilM.Expression(); got != nil {
		t.Errorf("nil matcher Expression() = %v, want nil", got)
	}
}
