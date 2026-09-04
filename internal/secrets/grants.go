// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"fmt"
	"sort"
	"strings"
)

// AgentGrant allows a set of agents to read a set of secret paths.
//
// Grants live in server configuration, not on the agent and not in a
// field the agent supplies. That is the whole point: an agent that
// declares its own entitlements is granting itself, and no amount of
// authentication downstream repairs that. The agent proves who it is;
// the server decides what that identity may read.
//
// An empty AgentIDs and an empty Labels means the rule matches no
// agent. "Matches nothing" is the safe reading of an under-specified
// rule -- a typo that widens a grant to the whole fleet is exactly the
// failure a secret store exists to prevent. Operators who really want
// every agent say so with `agent_ids: ["*"]`.
type AgentGrant struct {
	// AgentIDs are exact agent identities, or the single entry "*"
	// for every agent.
	AgentIDs []string `koanf:"agent_ids"`
	// Labels are label key/value pairs an agent must carry ALL of.
	// Combined with AgentIDs by OR: an agent matches the rule if its
	// id is listed or its labels match.
	Labels map[string]string `koanf:"labels"`
	// Paths are secret-path prefixes this rule allows. A prefix
	// ending in "/" allows a subtree; anything else must match the
	// path exactly. Requiring the trailing slash to be explicit stops
	// `app` from silently granting `application/…`.
	Paths []string `koanf:"paths"`
}

// AgentGrants evaluates a rule set. The zero value denies everything,
// which is the correct behaviour for a server with no grants
// configured: secrets are not readable by agents until an operator
// says which ones.
type AgentGrants struct {
	rules []AgentGrant
}

// NewAgentGrants validates and compiles a rule set.
func NewAgentGrants(rules []AgentGrant) (*AgentGrants, error) {
	compiled := make([]AgentGrant, 0, len(rules))
	for i, r := range rules {
		if len(r.Paths) == 0 {
			return nil, fmt.Errorf("secrets: agent grant %d: paths is required", i)
		}
		for _, p := range r.Paths {
			if p == "" {
				return nil, fmt.Errorf("secrets: agent grant %d: empty path", i)
			}
			// A bare "*" or "/" would grant the entire store through
			// what looks like a narrow rule. If that is genuinely
			// wanted it can be written as an explicit list.
			if p == "*" || p == "/" {
				return nil, fmt.Errorf("secrets: agent grant %d: path %q grants the entire secret store; list prefixes explicitly", i, p)
			}
		}
		if len(r.AgentIDs) == 0 && len(r.Labels) == 0 {
			return nil, fmt.Errorf("secrets: agent grant %d: needs agent_ids or labels; a rule matching no agent is almost certainly a mistake", i)
		}
		compiled = append(compiled, r)
	}
	return &AgentGrants{rules: compiled}, nil
}

// AgentIdentity is what a grant decision is made about: the verified
// agent id and the labels the control plane holds for it.
//
// Labels come from the agent record in the control plane's store, not
// from the request. An agent that could supply its own labels could
// label its way into any grant.
type AgentIdentity struct {
	AgentID string
	Labels  map[string]string
}

// Allows reports whether identity may read path.
//
// Deny by default: no matching rule means no. There is no "default
// allow" setting, deliberately -- `security.defaultpolicy: allow`
// exists for command execution, where an operator may reasonably want
// a permissive lab setup, and extending that idea to secrets would let
// one config line expose every credential in the store.
func (g *AgentGrants) Allows(identity AgentIdentity, path string) bool {
	if g == nil || identity.AgentID == "" || path == "" {
		return false
	}
	for i := range g.rules {
		r := &g.rules[i]
		if !matchesAgent(r, identity) {
			continue
		}
		for _, p := range r.Paths {
			if matchesPath(p, path) {
				return true
			}
		}
	}
	return false
}

// GrantedPaths returns the path prefixes identity may read, sorted.
// For operator-facing diagnostics -- "why can this agent not read X" --
// not for making decisions.
func (g *AgentGrants) GrantedPaths(identity AgentIdentity) []string {
	if g == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for i := range g.rules {
		r := &g.rules[i]
		if !matchesAgent(r, identity) {
			continue
		}
		for _, p := range r.Paths {
			seen[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func matchesAgent(r *AgentGrant, identity AgentIdentity) bool {
	for _, id := range r.AgentIDs {
		if id == "*" || id == identity.AgentID {
			return true
		}
	}
	if len(r.Labels) == 0 {
		return false
	}
	for k, want := range r.Labels {
		got, ok := identity.Labels[k]
		if !ok || got != want {
			return false
		}
	}
	return true
}

// matchesPath applies the trailing-slash rule: "app/" is a subtree,
// "app/db" is exact.
func matchesPath(grant, path string) bool {
	if strings.HasSuffix(grant, "/") {
		return strings.HasPrefix(path, grant)
	}
	return path == grant
}
