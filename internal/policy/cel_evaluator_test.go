// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func celPolicy(id, code string) *policy.Policy {
	return &policy.Policy{
		ID:              id,
		Name:            id,
		Type:            audit.PolicyTypeCEL,
		Category:        policy.CategorySecurity,
		Severity:        audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit,
		Code:            code,
		Enabled:         true,
	}
}

func TestCELEvaluator_AllowTrue(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	res, err := e.Evaluate(context.Background(), celPolicy("c-allow", "true"), policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Allowed {
		t.Errorf("Allowed = false, want true")
	}
	if len(res.Violations) != 0 {
		t.Errorf("violations on allow: %+v", res.Violations)
	}
	if res.PolicyID != "c-allow" || res.PolicyName != "c-allow" {
		t.Errorf("identity not propagated: %+v", res)
	}
	if res.EvaluatedAt.IsZero() {
		t.Errorf("EvaluatedAt zero")
	}
}

func TestCELEvaluator_DenyFalseSyntheticViolation(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	p := celPolicy("c-deny", "false")
	p.Severity = audit.SeverityCritical
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Allowed {
		t.Errorf("Allowed = true, want false")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("violations = %d, want 1 synthetic", len(res.Violations))
	}
	v := res.Violations[0]
	if v.Rule != "cel.denied" {
		t.Errorf("synthetic rule = %q", v.Rule)
	}
	if v.Severity != audit.SeverityCritical {
		t.Errorf("synthetic severity = %v, want policy severity Critical", v.Severity)
	}
}

func TestCELEvaluator_ConvenienceVars(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	p := celPolicy("c-vars", `action == "delete" && user == "admin"`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		Action: "delete",
		User:   "admin",
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !res.Allowed {
		t.Errorf("convenience vars not bound: action/user")
	}
}

func TestCELEvaluator_InputCompositeVar(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	// input composite carries the same data as the convenience vars.
	p := celPolicy("c-input", `input.action == "write" && input.resource.env == "prod"`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		Action:   "write",
		Resource: map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !res.Allowed {
		t.Errorf("input composite not bound correctly")
	}
}

func TestCELEvaluator_TimestampRFC3339String(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	// input.timestamp is an RFC3339 string (OPA parity); a
	// time-window policy wraps it with timestamp().
	p := celPolicy("c-ts",
		`timestamp(input.timestamp) < timestamp("2030-01-01T00:00:00Z")`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		Timestamp: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !res.Allowed {
		t.Errorf("timestamp string not parseable by CEL timestamp()")
	}
}

func TestCELEvaluator_ContextVar(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	p := celPolicy("c-ctx", `context.region == "us-east"`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		Context: map[string]any{"region": "us-east"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !res.Allowed {
		t.Errorf("context var not bound")
	}
}

func TestCELEvaluator_NilMapsCoercedForHas(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	// resource is nil in the input; has() must not error.
	p := celPolicy("c-has", `!has(resource.env)`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("nil resource should be coerced to empty map: %v", err)
	}
	if !res.Allowed {
		t.Errorf("has() on coerced-empty resource: want true")
	}
}

func TestCELEvaluator_NonBoolExpressionRejected(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	_, err := e.Evaluate(context.Background(), celPolicy("c-str", `"hello"`), policy.EvaluationInput{})
	if err == nil {
		t.Fatalf("non-bool expression accepted")
	}
	if !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("err not ErrInvalidPolicy family: %v", err)
	}
	if !strings.Contains(err.Error(), "must return bool") {
		t.Errorf("error should explain the bool requirement: %v", err)
	}
}

func TestCELEvaluator_CompileErrorIsEvaluatorError(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	_, err := e.Evaluate(context.Background(), celPolicy("c-bad", "this is not (((valid cel"),
		policy.EvaluationInput{})
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("err not ErrInvalidPolicy family: %v", err)
	}
}

func TestCELEvaluator_EvalRuntimeErrorIsEvaluatorError(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	// resource.missing on an empty resource is a CEL no_such_key
	// runtime error — a policy-authoring bug, surfaced as an
	// evaluator error (CEL has no "undefined" like Rego).
	p := celPolicy("c-runtime", `resource.missing == "x"`)
	_, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		Resource: map[string]any{"present": "y"},
	})
	if err == nil {
		t.Fatalf("expected eval runtime error")
	}
	if !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("err not ErrInvalidPolicy family: %v", err)
	}
}

func TestCELEvaluator_CacheReuse(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	p := celPolicy("c-cache", `action == "read"`)
	for i := 0; i < 5; i++ {
		res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{Action: "read"})
		if err != nil || !res.Allowed {
			t.Fatalf("iter %d: res=%+v err=%v", i, res, err)
		}
	}
}

func TestCELEvaluator_CodeChangeRecompiles(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	p := celPolicy("c-mut", "true")
	r1, _ := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if !r1.Allowed {
		t.Fatalf("first eval should allow")
	}
	p.Code = "false" // same ID, different Code → different cache key
	r2, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if r2.Allowed {
		t.Errorf("changed code not recompiled (still allowing)")
	}
}

func TestCELEvaluator_StandardMacros(t *testing.T) {
	t.Parallel()
	e := policy.NewCELEvaluator()
	// exercise a standard CEL macro to confirm the env exposes them.
	p := celPolicy("c-macro",
		`["a","b","c"].exists(x, x == action)`)
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{Action: "b"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !res.Allowed {
		t.Errorf("exists() macro did not evaluate")
	}
}

func TestCELEvaluator_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ policy.Evaluator = policy.NewCELEvaluator()
}
