// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// Filter is a compiled CEL expression that evaluates against an [Event]
// per PROJECT-DETAILS §4.9. Constructed via [CompileFilter]; safe for
// concurrent use across any number of [Filter.Match] calls.
//
// Method-value compatibility: `filter.Match` is a `func(Event) bool`
// suitable for [WithFilter] (task 4) without an adapter. Typical use:
//
//	f, err := CompileFilter("tags.role == 'web' && severity.at_least('warn')")
//	if err != nil { return err }
//	sub.Subscribe(ctx, pattern, handler, WithFilter(f.Match))
type Filter struct {
	expr    string
	program cel.Program
	logger  *slog.Logger
}

// CompileFilter parses and type-checks the given CEL expression
// against the events-domain environment, then plans it into an
// executable program. The expression has access to these variables:
//
//   - `type` (string)            — Event.Type
//   - `source` (string)          — Event.Source
//   - `severity` (string)        — canonical lowercase name
//   - `time` (Timestamp)         — Event.Time; supports CEL's
//     timestamp comparison + getDayOfWeek / getHours / etc.
//   - `correlation_id` (string)  — Event.CorrelationID
//   - `subject` (string)         — Event.Subject
//   - `tags` (map<string,string>)
//   - `data` (map<string,dyn>)   — nested access works
//
// Plus one custom method:
//
//   - `string.at_least(string) bool` — severity-ordinal comparison.
//     Used as `severity.at_least('warn')` to express §4.9's
//     `severity >= 'warn'` semantic. Lexicographic string ordering
//     doesn't match severity ordering, so the built-in `>=` operator
//     on the severity string is NOT what most operators want.
//
// Empty expression returns [ErrInvalidFilter] — callers wanting
// no-filter should pass `nil` to [WithFilter] instead of an empty
// string, keeping intent unambiguous at the call site.
//
// CEL compile / parse / check errors wrap [ErrInvalidFilter] so call
// sites match with [errors.Is]. The wrapped error preserves the CEL
// diagnostic (line + column + reason) for operator-facing reporting.
func CompileFilter(expr string) (*Filter, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, fmt.Errorf("%w: expression is empty (pass nil to WithFilter for no-filter)", ErrInvalidFilter)
	}
	env, err := newFilterEnv()
	if err != nil {
		return nil, fmt.Errorf("%w: env: %v", ErrInvalidFilter, err)
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilter, issues.Err())
	}
	// Result type must be bool — a filter that returns string or int
	// makes no sense and would silently match-or-not at runtime.
	if !ast.OutputType().IsExactType(cel.BoolType) {
		return nil, fmt.Errorf("%w: expression must return bool, got %s", ErrInvalidFilter, ast.OutputType().String())
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("%w: plan: %v", ErrInvalidFilter, err)
	}
	return &Filter{
		expr:    expr,
		program: program,
		logger:  slog.Default(),
	}, nil
}

// Match evaluates the compiled program against the given event.
// Non-bool eval results and eval errors (missing tag/data keys,
// type mismatches, etc.) fall back to `false` — the filter excludes
// the event rather than crashing the dispatcher. The fallback is
// logged at WARN once per failed evaluation; persistent CEL errors
// surface in operator logs without breaking the live subscription.
func (f *Filter) Match(e Event) bool {
	activation := buildActivation(e)
	result, _, err := f.program.Eval(activation)
	if err != nil {
		f.logger.LogAttrs(context.TODO(), slog.LevelWarn,
			"events: filter eval failed (excluding event)",
			slog.String("event_id", e.ID),
			slog.String("expression", f.expr),
			slog.Any("error", err),
		)
		return false
	}
	b, ok := result.Value().(bool)
	if !ok {
		f.logger.LogAttrs(context.TODO(), slog.LevelWarn,
			"events: filter returned non-bool (excluding event)",
			slog.String("event_id", e.ID),
			slog.String("expression", f.expr),
			slog.Any("got_type", fmt.Sprintf("%T", result.Value())),
		)
		return false
	}
	return b
}

// Expression returns the original CEL source text the filter was
// compiled from. Used by diagnostics (the gRPC handler's subscription
// metadata, task 6) and by tests that lock the filter's source-form
// stability.
func (f *Filter) Expression() string {
	return f.expr
}

// newFilterEnv builds the events-domain CEL environment. Built fresh
// on every CompileFilter call — environments are cheap to construct
// and operators tuning expressions interactively benefit from fresh
// state per attempt. If profiling shows env construction is hot, a
// package-level sync.Once + cached env is a safe drop-in optimisation.
func newFilterEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("type", cel.StringType),
		cel.Variable("source", cel.StringType),
		cel.Variable("severity", cel.StringType),
		cel.Variable("time", cel.TimestampType),
		cel.Variable("correlation_id", cel.StringType),
		cel.Variable("subject", cel.StringType),
		cel.Variable("tags", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("data", cel.MapType(cel.StringType, cel.DynType)),

		// at_least is a MEMBER overload on string — CEL's method-call
		// syntax `x.at_least(y)` desugars to `at_least(x, y)`. We
		// register it as a string-string-bool method so the
		// idiomatic operator-facing form is `severity.at_least('warn')`.
		// Plain `>=` on string severities would do lexicographic
		// compare ("critical" < "debug" < "error" < "info" < "warn")
		// — exactly wrong for §4.9's severity-threshold idiom.
		cel.Function("at_least",
			cel.MemberOverload(
				"string_at_least_string",
				[]*cel.Type{cel.StringType, cel.StringType},
				cel.BoolType,
				cel.BinaryBinding(severityAtLeastFn),
			),
		),
	)
}

// severityAtLeastFn implements `severity.at_least(threshold)`. Both
// arguments are parsed via [ParseSeverity] — unknown names return a
// CEL error so the surrounding [Filter.Match] falls back to false
// rather than silently passing every event.
func severityAtLeastFn(self, threshold ref.Val) ref.Val {
	selfStr, ok := self.Value().(string)
	if !ok {
		return types.NewErr("at_least: receiver is not a string")
	}
	threshStr, ok := threshold.Value().(string)
	if !ok {
		return types.NewErr("at_least: threshold is not a string")
	}
	selfSev, err := ParseSeverity(selfStr)
	if err != nil {
		return types.NewErr("at_least: unknown severity %q on receiver", selfStr)
	}
	threshSev, err := ParseSeverity(threshStr)
	if err != nil {
		return types.NewErr("at_least: unknown threshold %q", threshStr)
	}
	return types.Bool(selfSev.AtLeast(threshSev))
}

// buildActivation converts an [Event] into the variable-map CEL
// evaluates against. Nil maps are coerced to empty so cel-go's map
// access doesn't fail on `tags.role` for events with no tags
// (CEL still errors on missing keys; Match catches that and returns
// false, which is the right default — better to exclude than match).
func buildActivation(e Event) map[string]any {
	tags := e.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	data := e.Data
	if data == nil {
		data = map[string]any{}
	}
	return map[string]any{
		"type":           string(e.Type),
		"source":         e.Source,
		"severity":       e.Severity.String(),
		"time":           e.Time,
		"correlation_id": e.CorrelationID,
		"subject":        e.Subject,
		"tags":           tags,
		"data":           data,
	}
}
