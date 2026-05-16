package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/cel-go/cel"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// CELEvaluator implements [Evaluator] for audit.PolicyTypeCEL by
// wrapping google/cel-go (already a direct dependency from Epic 11
// task 5's event filter). A CEL policy's Code is a single boolean
// expression: true = allow, false = deny + one synthetic
// audit.Violation — the same fail-closed-but-visible philosophy as
// OPAEvaluator's undefined-allow path.
//
// Unlike Rego, CEL has no "undefined" concept: every successful
// evaluation yields a typed value. So a CEL runtime error (missing
// key, type mismatch) is a genuine policy-authoring bug and is
// returned as an evaluator error (ErrInvalidPolicy family) per the
// Evaluator contract — not silently coerced to a deny.
//
// Compilation is cached the same way as OPAEvaluator: a cel.Program
// keyed policyID + sha256(Code) under a mutex. Code is immutable per
// registration (no Deregister in v1.0); a v1.8 re-register with
// changed Code gets a fresh key naturally, so no invalidation.
type CELEvaluator struct {
	mu    sync.Mutex
	cache map[string]cel.Program
}

// NewCELEvaluator returns a ready CELEvaluator.
func NewCELEvaluator() *CELEvaluator {
	return &CELEvaluator{cache: make(map[string]cel.Program)}
}

// celEnv builds the §4.12 CEL environment: the five documented vars.
// `input` is the composite document; resource/action/user/context
// are convenience top-level bindings of the same data
// (input.action == 'x' and action == 'x' are equivalent). cel-go is
// sandboxed by default (no I/O, no network) so no capability
// restriction is needed (cf. OPAEvaluator). Standard macros (has,
// all, exists, map, filter) + timestamp/duration types are
// available out of the box.
func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("input", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("action", cel.StringType),
		cel.Variable("user", cel.StringType),
		cel.Variable("context", cel.MapType(cel.StringType, cel.DynType)),
	)
}

// Evaluate compiles (cached) and runs policy.Code against input.
//
//   - compile failure / non-bool expression → evaluator error.
//   - eval runtime error (missing key, type mismatch) → evaluator
//     error (CEL has no "undefined"; this is an authoring bug).
//   - clean true  → Allowed=true.
//   - clean false → Allowed=false + one synthetic audit.Violation
//     (severity = policy's declared severity).
func (e *CELEvaluator) Evaluate(ctx context.Context, policy *Policy, input EvaluationInput) (result EvaluationResult, err error) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: cel evaluate panic: %v", ErrInvalidPolicy, r)
		}
	}()

	prog, err := e.program(policy)
	if err != nil {
		return EvaluationResult{}, err
	}

	out, _, evalErr := prog.ContextEval(ctx, celActivation(input))
	if evalErr != nil {
		return EvaluationResult{}, fmt.Errorf("%w: cel eval %q: %w", ErrInvalidPolicy, policy.ID, evalErr)
	}

	allowed, ok := out.Value().(bool)
	if !ok {
		// The AST output-type check in program() guarantees bool, so
		// this is unreachable in practice — guard rather than panic.
		return EvaluationResult{}, fmt.Errorf("%w: cel %q returned %T, want bool", ErrInvalidPolicy, policy.ID, out.Value())
	}

	res := EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		Allowed:     allowed,
		EvaluatedAt: start.UTC(),
		Duration:    time.Since(start),
	}
	if !allowed {
		res.Violations = []audit.Violation{{
			Rule:     "cel.denied",
			Message:  fmt.Sprintf("policy %q CEL expression evaluated false", policy.ID),
			Severity: policy.Severity,
		}}
	}
	return res, nil
}

// program returns the cached cel.Program for policy, compiling +
// caching on first use. Key = policyID + sha256(Code).
func (e *CELEvaluator) program(policy *Policy) (cel.Program, error) {
	sum := sha256.Sum256([]byte(policy.Code))
	key := policy.ID + ":" + hex.EncodeToString(sum[:])

	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.cache[key]; ok {
		return p, nil
	}

	env, err := celEnv()
	if err != nil {
		return nil, fmt.Errorf("%w: cel env: %v", ErrInvalidPolicy, err)
	}
	ast, issues := env.Compile(policy.Code)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("%w: cel compile %q: %v", ErrInvalidPolicy, policy.ID, issues.Err())
	}
	// A policy expression must yield a bool — a string/int result
	// would silently mis-decide at runtime.
	if !ast.OutputType().IsExactType(cel.BoolType) {
		return nil, fmt.Errorf("%w: cel policy %q must return bool, got %s",
			ErrInvalidPolicy, policy.ID, ast.OutputType().String())
	}
	prog, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("%w: cel plan %q: %v", ErrInvalidPolicy, policy.ID, err)
	}
	e.cache[key] = prog
	return prog, nil
}

// celActivation maps EvaluationInput into the CEL variable bindings.
// `input` is the composite document (timestamp as RFC3339Nano
// string for exact parity with the OPA input doc); the other four
// vars are convenience bindings of the same data. Caller maps are
// referenced, not copied — the Evaluator contract forbids mutation
// and CEL treats the activation as read-only. nil maps are coerced
// to empty so `has(resource.x)` works on a resource-less input.
func celActivation(in EvaluationInput) map[string]any {
	resource := in.Resource
	if resource == nil {
		resource = map[string]any{}
	}
	cctx := in.Context
	if cctx == nil {
		cctx = map[string]any{}
	}
	ts := in.Timestamp.UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"input": map[string]any{
			"resource":  resource,
			"action":    in.Action,
			"user":      in.User,
			"context":   cctx,
			"timestamp": ts,
		},
		"resource": resource,
		"action":   in.Action,
		"user":     in.User,
		"context":  cctx,
	}
}

// Compile-time assertion that *CELEvaluator satisfies [Evaluator].
var _ Evaluator = (*CELEvaluator)(nil)
