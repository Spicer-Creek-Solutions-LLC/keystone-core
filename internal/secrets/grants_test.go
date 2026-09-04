// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"reflect"
	"testing"
)

func mustGrants(t *testing.T, rules []AgentGrant) *AgentGrants {
	t.Helper()
	g, err := NewAgentGrants(rules)
	if err != nil {
		t.Fatalf("NewAgentGrants: %v", err)
	}
	return g
}

// The zero value and an empty rule set must both deny. A server with
// no grants configured has not opted agents into reading anything.
func TestAgentGrants_DenyByDefault(t *testing.T) {
	identity := AgentIdentity{AgentID: "agent-1", Labels: map[string]string{"role": "web"}}

	t.Run("nil receiver", func(t *testing.T) {
		var g *AgentGrants
		if g.Allows(identity, "app/db") {
			t.Error("nil AgentGrants allowed a read")
		}
	})

	t.Run("zero value", func(t *testing.T) {
		if (&AgentGrants{}).Allows(identity, "app/db") {
			t.Error("zero AgentGrants allowed a read")
		}
	})

	t.Run("empty rule set", func(t *testing.T) {
		if mustGrants(t, nil).Allows(identity, "app/db") {
			t.Error("empty rule set allowed a read")
		}
	})

	t.Run("no matching rule", func(t *testing.T) {
		g := mustGrants(t, []AgentGrant{{AgentIDs: []string{"agent-2"}, Paths: []string{"app/"}}})
		if g.Allows(identity, "app/db") {
			t.Error("a rule for another agent allowed this one")
		}
	})
}

func TestAgentGrants_MatchesByAgentID(t *testing.T) {
	g := mustGrants(t, []AgentGrant{{AgentIDs: []string{"agent-1", "agent-3"}, Paths: []string{"app/"}}})

	if !g.Allows(AgentIdentity{AgentID: "agent-1"}, "app/db") {
		t.Error("listed agent was denied")
	}
	if !g.Allows(AgentIdentity{AgentID: "agent-3"}, "app/db") {
		t.Error("second listed agent was denied")
	}
	if g.Allows(AgentIdentity{AgentID: "agent-2"}, "app/db") {
		t.Error("unlisted agent was allowed")
	}
	if g.Allows(AgentIdentity{}, "app/db") {
		t.Error("empty agent id was allowed")
	}
}

func TestAgentGrants_MatchesByLabels(t *testing.T) {
	g := mustGrants(t, []AgentGrant{{
		Labels: map[string]string{"role": "web", "env": "prod"},
		Paths:  []string{"app/"},
	}})

	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"all labels match", map[string]string{"role": "web", "env": "prod"}, true},
		{"extra labels are fine", map[string]string{"role": "web", "env": "prod", "dc": "a"}, true},
		{"one label missing", map[string]string{"role": "web"}, false},
		{"one label differs", map[string]string{"role": "web", "env": "staging"}, false},
		{"no labels", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.Allows(AgentIdentity{AgentID: "agent-1", Labels: tt.labels}, "app/db")
			if got != tt.want {
				t.Errorf("Allows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentGrants_Wildcard(t *testing.T) {
	g := mustGrants(t, []AgentGrant{{AgentIDs: []string{"*"}, Paths: []string{"shared/"}}})
	if !g.Allows(AgentIdentity{AgentID: "any-agent"}, "shared/ca-bundle") {
		t.Error("wildcard rule denied an agent")
	}
	// The wildcard widens WHO, never WHAT.
	if g.Allows(AgentIdentity{AgentID: "any-agent"}, "app/db") {
		t.Error("wildcard agent rule allowed a path outside its prefixes")
	}
}

// The trailing slash is the difference between a subtree and an exact
// path. Without the rule, "app" would silently grant "application/…".
func TestAgentGrants_PathMatching(t *testing.T) {
	g := mustGrants(t, []AgentGrant{{
		AgentIDs: []string{"agent-1"},
		Paths:    []string{"app/", "shared/ca-bundle"},
	}})
	identity := AgentIdentity{AgentID: "agent-1"}

	tests := []struct {
		path string
		want bool
	}{
		{"app/db", true},
		{"app/db/password", true},
		{"app/", true},
		{"application/db", false},
		{"app", false},
		{"shared/ca-bundle", true},
		{"shared/ca-bundle/extra", false},
		{"shared/other", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := g.Allows(identity, tt.path); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNewAgentGrants_Rejects(t *testing.T) {
	tests := []struct {
		name  string
		rules []AgentGrant
	}{
		{"no paths", []AgentGrant{{AgentIDs: []string{"agent-1"}}}},
		{"empty path", []AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{""}}}},
		{"no subject", []AgentGrant{{Paths: []string{"app/"}}}},
		// A rule that looks narrow but grants everything is the
		// dangerous shape; make the operator spell it out.
		{"star path", []AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"*"}}}},
		{"root path", []AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"/"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAgentGrants(tt.rules); err == nil {
				t.Error("NewAgentGrants() error = nil, want a validation error")
			}
		})
	}
}

func TestAgentGrants_GrantedPaths(t *testing.T) {
	g := mustGrants(t, []AgentGrant{
		{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}},
		{Labels: map[string]string{"role": "web"}, Paths: []string{"web/", "app/"}},
		{AgentIDs: []string{"agent-2"}, Paths: []string{"other/"}},
	})

	got := g.GrantedPaths(AgentIdentity{AgentID: "agent-1", Labels: map[string]string{"role": "web"}})
	want := []string{"app/", "web/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GrantedPaths() = %v, want %v (deduplicated and sorted)", got, want)
	}

	if got := g.GrantedPaths(AgentIdentity{AgentID: "agent-9"}); len(got) != 0 {
		t.Errorf("GrantedPaths() = %v, want none for an unmatched agent", got)
	}

	var nilGrants *AgentGrants
	if got := nilGrants.GrantedPaths(AgentIdentity{AgentID: "agent-1"}); got != nil {
		t.Errorf("GrantedPaths() = %v, want nil", got)
	}
}

// Rules are OR'd: matching any one is enough.
func TestAgentGrants_MultipleRulesCombine(t *testing.T) {
	g := mustGrants(t, []AgentGrant{
		{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}},
		{Labels: map[string]string{"role": "db"}, Paths: []string{"db/"}},
	})
	identity := AgentIdentity{AgentID: "agent-1", Labels: map[string]string{"role": "db"}}

	if !g.Allows(identity, "app/x") {
		t.Error("id rule did not apply")
	}
	if !g.Allows(identity, "db/x") {
		t.Error("label rule did not apply")
	}
	if g.Allows(identity, "web/x") {
		t.Error("a path in neither rule was allowed")
	}
}
