// SPDX-License-Identifier: Apache-2.0

package targeting

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

// globCache memoizes compiled globs keyed by raw pattern. Compile cost
// is non-trivial and the same target expression is typically run
// against many agents in a single dispatch.
var globCache sync.Map // map[string]*globEntry

type globEntry struct {
	g   glob.Glob
	err error
}

// matchValue reports whether value matches pattern. Three modes,
// chosen in this order:
//
//  1. Slice value — return true if any element matches the pattern
//     (recursive call). Used for AgentRecord.IPAddresses, where a
//     multi-homed agent should match if any address satisfies the
//     pattern.
//  2. CIDR pattern (`net.ParseCIDR` succeeds) — parse the stringified
//     value as an IP and test containment. Non-IP values evaluate to
//     false rather than panic.
//  3. Otherwise — literal equality, or `gobwas/glob` match if the
//     pattern contains a glob metacharacter. Compiled globs are
//     cached package-wide; a single batch dispatch reuses them across
//     every agent.
func matchValue(value any, pattern string) bool {
	switch v := value.(type) {
	case []string:
		for _, s := range v {
			if matchValue(s, pattern) {
				return true
			}
		}
		return false
	case []any:
		for _, e := range v {
			if matchValue(e, pattern) {
				return true
			}
		}
		return false
	}
	if _, ipNet, err := net.ParseCIDR(pattern); err == nil {
		ip := net.ParseIP(stringify(value))
		if ip == nil {
			return false
		}
		return ipNet.Contains(ip)
	}
	s := stringify(value)
	if !hasGlobMeta(pattern) {
		return s == pattern
	}
	g, err := getOrCompileGlob(pattern)
	if err != nil {
		return false
	}
	return g.Match(s)
}

// hasGlobMeta reports whether pattern contains any glob metacharacter.
// `\*` and friends would be flagged as meta here too — acceptable for
// v1.0 since literal-asterisk targeting is not part of the shorthand.
func hasGlobMeta(p string) bool {
	return strings.ContainsAny(p, "*?[{")
}

func getOrCompileGlob(pattern string) (glob.Glob, error) {
	if v, ok := globCache.Load(pattern); ok {
		e := v.(*globEntry)
		return e.g, e.err
	}
	g, err := glob.Compile(pattern)
	e := &globEntry{g: g, err: err}
	globCache.Store(pattern, e)
	return g, err
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}

// matchAny is the variadic adapter expected by expr.Function.
func matchAny(params ...any) (any, error) {
	if len(params) != 2 {
		return false, fmt.Errorf("match: expected 2 arguments, got %d", len(params))
	}
	pattern, ok := params[1].(string)
	if !ok {
		return false, fmt.Errorf("match: pattern must be a string, got %T", params[1])
	}
	return matchValue(params[0], pattern), nil
}
