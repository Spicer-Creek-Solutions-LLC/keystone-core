package policy_test

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// stubEvaluator is a no-op Evaluator for wiring tests; task 9 adds
// real dispatch tests against the actual OPA/CEL/Builtin evaluators.
type stubEvaluator struct{}

func (stubEvaluator) Evaluate(context.Context, *policy.Policy, policy.EvaluationInput) (policy.EvaluationResult, error) {
	return policy.EvaluationResult{Allowed: true}, nil
}

func TestNewEngine_RequiresRegistry(t *testing.T) {
	t.Parallel()
	_, err := policy.NewEngine(nil)
	if !errors.Is(err, policy.ErrEngineMisconfigured) {
		t.Errorf("err = %v, want ErrEngineMisconfigured", err)
	}
}

func TestNewEngine_OK(t *testing.T) {
	t.Parallel()
	e, err := policy.NewEngine(policy.NewRegistry())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Registry() == nil {
		t.Errorf("Registry() nil")
	}
}

func TestWithEvaluator_Wiring(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry(),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, stubEvaluator{}),
	)
	if _, ok := e.Evaluator(audit.PolicyTypeBuiltin); !ok {
		t.Errorf("builtin evaluator not wired")
	}
	if _, ok := e.Evaluator(audit.PolicyTypeOPA); ok {
		t.Errorf("opa evaluator present but never registered")
	}
}

func TestWithEvaluator_IgnoresNilAndUnknown(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry(),
		policy.WithEvaluator(audit.PolicyTypeOPA, nil),
		policy.WithEvaluator(audit.PolicyType("ldap"), stubEvaluator{}),
	)
	if _, ok := e.Evaluator(audit.PolicyTypeOPA); ok {
		t.Errorf("nil evaluator stored")
	}
	if _, ok := e.Evaluator(audit.PolicyType("ldap")); ok {
		t.Errorf("unknown-type evaluator stored")
	}
}

func TestWithEvaluator_LastWins(t *testing.T) {
	t.Parallel()
	first := stubEvaluator{}
	second := stubEvaluator{}
	e, _ := policy.NewEngine(policy.NewRegistry(),
		policy.WithEvaluator(audit.PolicyTypeCEL, first),
		policy.WithEvaluator(audit.PolicyTypeCEL, second),
	)
	got, ok := e.Evaluator(audit.PolicyTypeCEL)
	if !ok {
		t.Fatalf("cel evaluator missing")
	}
	if got != policy.Evaluator(second) {
		t.Errorf("last-wins not honored")
	}
}

func TestEngine_Evaluate_NoEvaluatorUntilTask9(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(regPolicy("p1"))
	e, _ := policy.NewEngine(r) // no evaluators wired

	_, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNoEvaluator) {
		t.Errorf("Evaluate err = %v, want ErrNoEvaluator", err)
	}

	// Missing policy surfaces ErrNotFound before the evaluator check.
	_, err = e.Evaluate(context.Background(), "missing", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("Evaluate(missing) err = %v, want ErrNotFound", err)
	}
}

func TestEngine_Evaluate_EvaluatorWiredStillNoDispatch(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(regPolicy("p1")) // builtin type
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, stubEvaluator{}))

	// Even with an evaluator wired, task-5 dispatch is not
	// implemented — still ErrNoEvaluator (the "dispatch lands in
	// task 9" sentinel wrap).
	_, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNoEvaluator) {
		t.Errorf("Evaluate err = %v, want ErrNoEvaluator (dispatch deferred to task 9)", err)
	}
}

func TestEngine_EvaluatePolicySet_NotFoundThenNoEvaluator(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	e, _ := policy.NewEngine(r)
	if _, err := e.EvaluatePolicySet(context.Background(), "missing", policy.EvaluationInput{}); !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("missing set err = %v, want ErrNotFound", err)
	}

	_ = r.RegisterPolicy(regPolicy("p1"))
	_ = r.RegisterPolicySet(&policy.PolicySet{ID: "s1", Name: "s", PolicyIDs: []string{"p1"}, Enabled: true})
	if _, err := e.EvaluatePolicySet(context.Background(), "s1", policy.EvaluationInput{}); !errors.Is(err, policy.ErrNoEvaluator) {
		t.Errorf("present set err = %v, want ErrNoEvaluator", err)
	}
}

func TestEngine_EvaluateForResource_NoEvaluator(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry())
	_, err := e.EvaluateForResource(context.Background(), "secret", "write", nil, policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNoEvaluator) {
		t.Errorf("err = %v, want ErrNoEvaluator", err)
	}
}
