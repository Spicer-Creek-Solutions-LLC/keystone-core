// SPDX-License-Identifier: Apache-2.0

package targeting

import (
	"fmt"

	"github.com/expr-lang/expr"

	"go.keystone-core.io/keystone-core/internal/state"
)

// Matcher evaluates a compiled TargetExpression against agent records.
//
// One Matcher per dispatch is the expected pattern: BatchDispatcher
// (task 8) compiles the operator's --target string once, wraps it in a
// Matcher, and asks Match for every candidate agent. Match itself is
// safe for concurrent use because expr.Run does not mutate the program.
type Matcher struct {
	expr *TargetExpression
}

// NewMatcher returns a Matcher backed by the given TargetExpression.
// The expression must be the result of a successful Compile; passing
// nil produces a Matcher whose Match always errors.
func NewMatcher(te *TargetExpression) *Matcher {
	return &Matcher{expr: te}
}

// Match reports whether rec satisfies the compiled expression. The
// error return is reserved for runtime failures that callers should
// treat as observability events: nil program, expr.Run error, or a
// program that produced a non-bool result. A clean miss is always
// (false, nil).
func (m *Matcher) Match(rec state.AgentRecord) (bool, error) {
	if m == nil || m.expr == nil || m.expr.Program == nil {
		return false, fmt.Errorf("targeting: matcher has no compiled expression")
	}
	env := Flatten(rec)
	out, err := expr.Run(m.expr.Program, env)
	if err != nil {
		return false, fmt.Errorf("targeting: run %q: %w", m.expr.Raw, err)
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("targeting: expression %q produced %T, want bool", m.expr.Raw, out)
	}
	return b, nil
}

// Expression returns the wrapped TargetExpression for diagnostics.
func (m *Matcher) Expression() *TargetExpression {
	if m == nil {
		return nil
	}
	return m.expr
}
