// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Route is one entry in the path-prefix routing table — the
// operator's `secrets.routing[]` config (PROJECT-DETAILS §4.11)
// deserialises to a slice of these.
//
// Prefix is matched against [Secret.Path] with `strings.HasPrefix`;
// trailing `/` is conventional but not required (`Vault`-style mount
// paths use it). A prefix of `secret` (no slash) matches both
// `secret/foo` AND `secretstore/foo` — operators avoid that footgun
// by terminating prefixes at segment boundaries with `/`.
//
// Backend names the [SecretBackend] the matching paths route to;
// must match a `secrets.backends[].name` entry in the operator
// config. The router does NOT resolve names to backend instances —
// that join lives in the broker (task 3).
type Route struct {
	Prefix  string `json:"prefix"`
	Backend string `json:"backend"`
}

// Router resolves a [Secret.Path] to the [Route] that should serve
// it, longest-prefix-first per PROJECT-DETAILS §4.11. The implementation
// is a pre-sorted slice + linear `HasPrefix` scan — a trie would shave
// microseconds in pathological configs, but realistic deployments hold
// ≤ 20 routes and the slice form is meaningfully simpler to read.
//
// Construction validates the routing table; [Router.Lookup] is
// allocation-free against a non-empty set.
type Router struct {
	routes []Route
}

// NewRouter validates the routes and returns the engine. All error
// branches wrap [ErrInvalidBackend] so call sites can match the
// family root with [errors.Is].
//
// Validation rules:
//
//   - Prefix MUST be non-empty. PROJECT-DETAILS §4.11's
//     `default_backend` config knob covers the "no prefix matched"
//     fallback; allowing an empty prefix here would conflict with it.
//   - Prefix MUST NOT start with `/`. [Secret.Path] is unrooted by the
//     task-1 convention, so a rooted prefix can never match.
//   - Prefix MUST NOT contain whitespace or non-printable bytes.
//     Defensive: an attacker-supplied prefix from a misconfigured
//     loader can otherwise smuggle a route the operator can't see in
//     a `kscore-secrets backends` listing.
//   - Backend MUST be non-empty.
//   - Prefix MUST be unique across the table — duplicates are an
//     ambiguous config and the error message lists every offender so
//     operators fix it in one pass.
//
// On success, routes are sorted longest-prefix-first; the secondary
// sort is lexicographic-ascending so two same-length prefixes with
// different strings have a deterministic order (pinned in tests).
func NewRouter(routes []Route) (*Router, error) {
	cleaned := make([]Route, 0, len(routes))
	seen := make(map[string][]int, len(routes))

	for i, r := range routes {
		if err := validateRoute(i, r); err != nil {
			return nil, err
		}
		seen[r.Prefix] = append(seen[r.Prefix], i)
		cleaned = append(cleaned, r)
	}

	if dups := collectDuplicates(seen); len(dups) > 0 {
		return nil, fmt.Errorf("%w: routing: duplicate prefix(es): %s",
			ErrInvalidBackend, strings.Join(dups, ", "))
	}

	sort.SliceStable(cleaned, func(i, j int) bool {
		if len(cleaned[i].Prefix) != len(cleaned[j].Prefix) {
			return len(cleaned[i].Prefix) > len(cleaned[j].Prefix)
		}
		return cleaned[i].Prefix < cleaned[j].Prefix
	})

	return &Router{routes: cleaned}, nil
}

func validateRoute(idx int, r Route) error {
	if r.Prefix == "" {
		return fmt.Errorf("%w: routing[%d]: prefix is required", ErrInvalidBackend, idx)
	}
	if strings.HasPrefix(r.Prefix, "/") {
		return fmt.Errorf("%w: routing[%d]: prefix %q must not start with %q", ErrInvalidBackend, idx, r.Prefix, "/")
	}
	for _, ch := range r.Prefix {
		if unicode.IsSpace(ch) || !unicode.IsPrint(ch) {
			return fmt.Errorf("%w: routing[%d]: prefix %q contains whitespace or non-printable character", ErrInvalidBackend, idx, r.Prefix)
		}
	}
	if r.Backend == "" {
		return fmt.Errorf("%w: routing[%d]: backend is required", ErrInvalidBackend, idx)
	}
	return nil
}

func collectDuplicates(seen map[string][]int) []string {
	var dups []string
	for prefix, idxs := range seen {
		if len(idxs) > 1 {
			dups = append(dups, fmt.Sprintf("%q at indices %v", prefix, idxs))
		}
	}
	sort.Strings(dups)
	return dups
}

// Lookup returns the [Route] whose prefix is the longest match against
// path. Returns the zero-value [Route] + false when no route matches —
// the broker (task 3) then falls back to its `default_backend` config.
//
// Path is matched verbatim with `strings.HasPrefix`; no normalisation
// (trim, lowercase, slash-fold) — backends and the broker treat paths
// as opaque keys, so the router does too.
func (r *Router) Lookup(path string) (Route, bool) {
	for _, route := range r.routes {
		if strings.HasPrefix(path, route.Prefix) {
			return route, true
		}
	}
	return Route{}, false
}

// Routes returns a defensive copy of the routing table in the order
// [Router] uses for lookup (longest-prefix-first, lexicographic
// tie-break). Callers may mutate the result without disturbing the
// engine.
func (r *Router) Routes() []Route {
	out := make([]Route, len(r.routes))
	copy(out, r.routes)
	return out
}

// Len returns the number of routes the engine holds. Zero is operator
// misconfiguration (the broker's `Health` check surfaces it); the
// router itself is happy to exist empty so unit tests can build one
// without ceremony.
func (r *Router) Len() int {
	return len(r.routes)
}
