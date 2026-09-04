// SPDX-License-Identifier: Apache-2.0

package target

import (
	"errors"
	"fmt"
	"strings"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ErrTargetUnsupported is returned for shorthand that the v1.0 proto
// Target shape can't express (os: / arch: / status: / ip:, OR, NOT,
// parens, glob-on-id). v1.x will add a server-side compile via a
// proto string field; the ROADMAP entry tracks the gap.
var ErrTargetUnsupported = errors.New("exec: target expression not yet supported in v0.1 proto (use AND of id / hostname / label clauses)")

// ParseTarget translates the v1.0-subset of the targeting shorthand
// into a v1.Target proto.
//
// Supported syntax:
//
//	role:web                    // label
//	role:web AND env:prod       // AND of labels (any number)
//	id:web-01                   // exact agent ID (comma-separated for multiple)
//	id:web-01,web-02            // multiple exact IDs in one clause
//	hostname:web-prod-*         // glob on hostname (single clause)
//	role:web AND hostname:web-* // combination
//
// Unsupported (returns ErrTargetUnsupported):
//
//	OR, NOT, parens
//	id:web-*               (glob on id — proto AgentIDs is exact-match)
//	os: / arch: / status: / ip:  (built-in fields not in v0.1 proto Target)
//
// Empty input returns (nil, nil) so callers can treat "no target" as
// the server-side "match nothing" contract.
func ParseTarget(raw string) (*v1.Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if containsAny(raw, "()") {
		return nil, fmt.Errorf("%w: parens", ErrTargetUnsupported)
	}
	lower := strings.ToLower(raw)
	if matchesWord(lower, "or") || matchesWord(lower, "not") {
		return nil, fmt.Errorf("%w: OR / NOT", ErrTargetUnsupported)
	}

	clauses, err := splitAnd(raw)
	if err != nil {
		return nil, err
	}
	t := &v1.Target{Labels: map[string]string{}}
	hostnameSet := false
	for _, c := range clauses {
		field, value, ok := strings.Cut(c, ":")
		if !ok {
			return nil, fmt.Errorf("exec: target clause %q: missing ':'", c)
		}
		field = strings.TrimSpace(field)
		value = strings.TrimSpace(value)
		if field == "" || value == "" {
			return nil, fmt.Errorf("exec: target clause %q: empty field or value", c)
		}
		switch field {
		case "id":
			if strings.ContainsAny(value, "*?[") {
				return nil, fmt.Errorf("%w: id:<glob>", ErrTargetUnsupported)
			}
			for _, id := range strings.Split(value, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					t.AgentIds = append(t.AgentIds, id)
				}
			}
		case "hostname":
			if hostnameSet {
				return nil, fmt.Errorf("exec: target hostname clause appears twice")
			}
			t.HostnamePattern = value
			hostnameSet = true
		case "os", "arch", "status", "ip":
			return nil, fmt.Errorf("%w: %s: (built-in field not in v0.1 proto Target)",
				ErrTargetUnsupported, field)
		default:
			// Label clause. Reject duplicate keys — server semantic
			// would be AND with same key = impossible match.
			labelKey := strings.TrimPrefix(field, "labels.")
			if _, exists := t.Labels[labelKey]; exists {
				return nil, fmt.Errorf("exec: label %q appears twice in target", labelKey)
			}
			t.Labels[labelKey] = value
		}
	}
	if len(t.Labels) == 0 {
		t.Labels = nil // proto-clean: empty map vs nil
	}
	return t, nil
}

// splitAnd tokenizes the input on whitespace-bracketed "AND"
// (case-insensitive). Rejects leading / trailing / consecutive ANDs
// so dangling boolean operators surface as parse errors.
func splitAnd(raw string) ([]string, error) {
	tokens := strings.Fields(raw)
	var out []string
	var cur strings.Builder
	pendingAnd := false
	for i, tok := range tokens {
		if strings.EqualFold(tok, "AND") {
			if cur.Len() == 0 {
				if i == 0 {
					return nil, fmt.Errorf("exec: target starts with AND")
				}
				return nil, fmt.Errorf("exec: consecutive AND in target")
			}
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			pendingAnd = true
			continue
		}
		pendingAnd = false
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(tok)
	}
	if pendingAnd {
		return nil, fmt.Errorf("exec: target ends with AND")
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out, nil
}

// matchesWord reports whether haystack contains needle as a
// whitespace-delimited word. Used to detect OR / NOT.
func matchesWord(haystack, needle string) bool {
	for _, tok := range strings.Fields(haystack) {
		if tok == needle {
			return true
		}
	}
	return false
}

func containsAny(s string, chars string) bool {
	return strings.ContainsAny(s, chars)
}

// Localhost is the reserved target naming the machine the control
// plane runs on.
//
// Every command that acts on hosts requires a target — omitting one
// used to mean "the control plane", which is a surprising default for
// a fleet tool and the reason a state apply could look fleet-wide
// while converging exactly one host. Making it explicit costs six
// characters and removes the ambiguity.
//
// It resolves to no proto Target, which the server reads as "run here".
// When the control plane is itself managed by an agent, `localhost`
// becomes an ordinary agent id and this special case retires.
const Localhost = "localhost"

// ErrTargetRequired is returned when a host-acting command is invoked
// without a target.
var ErrTargetRequired = errors.New(
	"--target is required (use 'localhost' for the control-plane host, " +
		"or id:<agent> / <label>:<value> / hostname:<glob> for agents)")

// ParseRequired parses a mandatory target.
//
// Returns (nil, nil) for Localhost — the absence of a proto Target is
// how "run on the control plane" is expressed on the wire, so callers
// pass the result straight through.
func ParseRequired(raw string) (*v1.Target, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, ErrTargetRequired
	}
	if strings.EqualFold(strings.TrimSpace(raw), Localhost) {
		return nil, nil
	}
	t, err := ParseTarget(raw)
	if err != nil {
		return nil, err
	}
	if t == nil {
		// ParseTarget only returns nil for empty input, which the
		// guard above already rejected; treat it as required anyway
		// rather than silently converging the control plane.
		return nil, ErrTargetRequired
	}
	return t, nil
}

// IsLocalhost reports whether raw names the control-plane host.
func IsLocalhost(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), Localhost)
}
