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

func opaPolicy(id, code string) *policy.Policy {
	return &policy.Policy{
		ID:              id,
		Name:            id,
		Type:            audit.PolicyTypeOPA,
		Category:        policy.CategorySecurity,
		Severity:        audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit,
		Code:            code,
		Enabled:         true,
	}
}

const allowPolicy = `package keystone.policy

allow := true
`

const denyWithViolationsPolicy = `package keystone.policy

allow := false

violations contains v if {
	input.action == "delete"
	v := {
		"rule":     "no-delete",
		"message":  "delete is not permitted",
		"severity": "critical",
		"path":     "action",
		"expected": "read|write",
		"actual":   input.action,
	}
}
`

const stringViolationsPolicy = `package keystone.policy

allow := false

violations contains "first reason"
violations contains "second reason"
`

const warningsPolicy = `package keystone.policy

allow := true

warnings contains "deprecated field used"
`

const noAllowRulePolicy = `package keystone.policy

# deliberately no allow rule
other := 1
`

const nonBoolAllowPolicy = `package keystone.policy

allow := "yes"
`

const inputDrivenPolicy = `package keystone.policy

allow if {
	input.user == "admin"
	input.resource.env == "prod"
}
`

const httpSendPolicy = `package keystone.policy

allow if {
	resp := http.send({"method": "GET", "url": "http://example.com"})
	resp.status_code == 200
}
`

func TestOPAEvaluator_Allow(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	res, err := e.Evaluate(context.Background(), opaPolicy("p-allow", allowPolicy), policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Allowed {
		t.Errorf("Allowed = false, want true")
	}
	if len(res.Violations) != 0 {
		t.Errorf("violations on allow: %+v", res.Violations)
	}
	if res.PolicyID != "p-allow" || res.PolicyName != "p-allow" {
		t.Errorf("identity not propagated: %+v", res)
	}
	if res.EvaluatedAt.IsZero() {
		t.Errorf("EvaluatedAt zero")
	}
}

func TestOPAEvaluator_DenyWithObjectViolations(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	res, err := e.Evaluate(context.Background(), opaPolicy("p-deny", denyWithViolationsPolicy),
		policy.EvaluationInput{Action: "delete"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Allowed {
		t.Errorf("Allowed = true, want false")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("violations = %d, want 1", len(res.Violations))
	}
	v := res.Violations[0]
	if v.Rule != "no-delete" || v.Message != "delete is not permitted" {
		t.Errorf("violation fields: %+v", v)
	}
	if v.Severity != audit.SeverityCritical {
		t.Errorf("severity = %v, want Critical (from rego)", v.Severity)
	}
	if v.Path != "action" || v.Expected != "read|write" || v.Actual != "delete" {
		t.Errorf("violation detail fields: %+v", v)
	}
}

func TestOPAEvaluator_StringViolationsFallbackSeverity(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	p := opaPolicy("p-strvio", stringViolationsPolicy)
	p.Severity = audit.SeverityMedium
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(res.Violations) != 2 {
		t.Fatalf("violations = %d, want 2", len(res.Violations))
	}
	for _, v := range res.Violations {
		if v.Severity != audit.SeverityMedium {
			t.Errorf("string violation severity = %v, want policy fallback Medium", v.Severity)
		}
		if v.Message == "" {
			t.Errorf("empty message: %+v", v)
		}
	}
}

func TestOPAEvaluator_Warnings(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	res, err := e.Evaluate(context.Background(), opaPolicy("p-warn", warningsPolicy), policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Allowed {
		t.Errorf("Allowed = false")
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "deprecated field used" {
		t.Errorf("warnings = %+v", res.Warnings)
	}
}

func TestOPAEvaluator_UndefinedAllowSyntheticViolation(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	res, err := e.Evaluate(context.Background(), opaPolicy("p-noallow", noAllowRulePolicy), policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("undefined allow should not error: %v", err)
	}
	if res.Allowed {
		t.Errorf("Allowed = true, want false (fail-closed)")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("violations = %d, want 1 synthetic", len(res.Violations))
	}
	if res.Violations[0].Rule != "opa.no-decision" {
		t.Errorf("synthetic rule = %q", res.Violations[0].Rule)
	}
	if res.Violations[0].Severity != audit.SeverityHigh {
		t.Errorf("synthetic severity = %v, want policy severity High", res.Violations[0].Severity)
	}
}

func TestOPAEvaluator_NonBoolAllow(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	res, err := e.Evaluate(context.Background(), opaPolicy("p-nonbool", nonBoolAllowPolicy), policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("non-bool allow should not error: %v", err)
	}
	if res.Allowed {
		t.Errorf("Allowed = true, want false")
	}
	if len(res.Violations) != 1 || res.Violations[0].Rule != "opa.non-bool-allow" {
		t.Errorf("expected non-bool synthetic violation, got %+v", res.Violations)
	}
}

func TestOPAEvaluator_InputBinding(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	p := opaPolicy("p-input", inputDrivenPolicy)

	deny, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		User:     "guest",
		Resource: map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if deny.Allowed {
		t.Errorf("guest should be denied")
	}

	allow, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{
		User:     "admin",
		Resource: map[string]any{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !allow.Allowed {
		t.Errorf("admin+prod should be allowed")
	}
}

func TestOPAEvaluator_CompileErrorIsEvaluatorError(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	_, err := e.Evaluate(context.Background(), opaPolicy("p-bad", "this is not valid rego {{{"),
		policy.EvaluationInput{})
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("err not ErrInvalidPolicy family: %v", err)
	}
}

func TestOPAEvaluator_HTTPSendDeniedByCapabilities(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	_, err := e.Evaluate(context.Background(), opaPolicy("p-http", httpSendPolicy), policy.EvaluationInput{})
	if err == nil {
		t.Fatalf("http.send policy should fail to compile under restricted caps")
	}
	if !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("err not ErrInvalidPolicy family: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "http.send") {
		t.Errorf("error should mention the disallowed builtin: %v", err)
	}
}

func TestOPAEvaluator_CacheReusesPreparedQuery(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	p := opaPolicy("p-cache", allowPolicy)

	// First eval compiles; subsequent evals must reuse. We can't see
	// the cache directly, but a second eval must succeed identically
	// and far faster than a fresh compile — assert correctness +
	// that repeated evals are stable.
	for i := 0; i < 5; i++ {
		res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
		if err != nil || !res.Allowed {
			t.Fatalf("iter %d: res=%+v err=%v", i, res, err)
		}
	}
}

func TestOPAEvaluator_CodeChangeRecompiles(t *testing.T) {
	t.Parallel()
	e := policy.NewOPAEvaluator()
	p := opaPolicy("p-mut", allowPolicy)
	r1, _ := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if !r1.Allowed {
		t.Fatalf("first eval should allow")
	}
	// Same ID, different Code → different cache key → recompiled.
	p.Code = `package keystone.policy
allow := false
`
	r2, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if r2.Allowed {
		t.Errorf("changed code not recompiled (still allowing)")
	}
}

// computeHeavyPolicy does enough work that the topdown evaluator
// checks ctx.Err() mid-eval — a trivial `allow := true` returns
// before the cancellation is observed.
const computeHeavyPolicy = `package keystone.policy

allow if {
	sum([x | some i; x := numbers.range(1, 200000)[i]]) > 0
}
`

func TestOPAEvaluator_EvalTimeout(t *testing.T) {
	t.Parallel()
	// A 1ns eval timeout on a compute-heavy policy must surface as
	// an evaluator error, not a hang. Compile happens first (it has
	// no timeout); the timeout wraps only Eval.
	e := policy.NewOPAEvaluator(policy.WithOPAEvalTimeout(time.Nanosecond))
	_, err := e.Evaluate(context.Background(),
		opaPolicy("p-timeout", computeHeavyPolicy), policy.EvaluationInput{})
	if err == nil {
		t.Errorf("expected error from timed-out eval")
	}
	if err != nil && !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("timeout err not ErrInvalidPolicy family: %v", err)
	}
}

func TestOPAEvaluator_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ policy.Evaluator = policy.NewOPAEvaluator()
}

func TestOPAEvaluator_WithOPACapabilitiesNilIgnored(t *testing.T) {
	t.Parallel()
	// Nil caps option must not override the safe default — http.send
	// still denied.
	e := policy.NewOPAEvaluator(policy.WithOPACapabilities(nil))
	_, err := e.Evaluate(context.Background(), opaPolicy("p-h", httpSendPolicy), policy.EvaluationInput{})
	if err == nil {
		t.Errorf("nil caps override should keep restricted default; http.send must still fail")
	}
}
