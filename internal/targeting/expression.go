package targeting

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// TargetExpression is a compiled target expression. Raw is the original
// user input; Translated is the expr-lang source produced by the
// shorthand translator (kept for diagnostics); Program is the compiled
// VM program ready for the Matcher in task 3.
type TargetExpression struct {
	Raw        string
	Translated string
	Program    *vm.Program
}

// envSchema describes the shape Matcher will pass to expr at runtime.
// Field names match the shorthand identifiers exactly. Unknown labels
// surface as the zero string at evaluation time.
var envSchema = map[string]any{
	"id":       "",
	"hostname": "",
	"os":       "",
	"arch":     "",
	"status":   "",
	"ip":       "",
	"labels":   map[string]string{},
}

// Compile parses the shorthand and produces a compiled expression.
// Empty / whitespace-only input is rejected — the empty selector would
// otherwise evaluate to true and silently match the entire fleet.
func Compile(raw string) (*TargetExpression, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("targeting: empty expression")
	}
	src, err := translate(raw)
	if err != nil {
		return nil, fmt.Errorf("targeting: parse %q: %w", raw, err)
	}
	prog, err := expr.Compile(
		src,
		expr.AsBool(),
		expr.Env(envSchema),
		expr.Function(
			"match",
			matchAny,
			new(func(string, string) bool),
			new(func(any, string) bool),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("targeting: compile %q (translated %q): %w", raw, src, err)
	}
	return &TargetExpression{Raw: raw, Translated: src, Program: prog}, nil
}
