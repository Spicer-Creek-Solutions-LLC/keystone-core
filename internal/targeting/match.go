package targeting

import (
	"fmt"
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

// matchValue reports whether value matches pattern. Patterns without
// glob metacharacters use string equality; patterns with metachars are
// compiled once via gobwas/glob and cached. A pattern that fails to
// compile evaluates to false (logged at the call site by the matcher
// in task 3 — for task 1 we simply do not panic).
func matchValue(value any, pattern string) bool {
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
