// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// stubEvaluator is a controllable Evaluator for dispatch tests.
// allow sets the verdict; err makes it an evaluator-internal
// failure; seenTimestamp captures the input timestamp the engine
// passed (to assert stamp-when-zero / respect-provided).
type stubEvaluator struct {
	allow         bool
	err           error
	seenTimestamp *time.Time
}

func (s *stubEvaluator) Evaluate(_ context.Context, p *policy.Policy, in policy.EvaluationInput) (policy.EvaluationResult, error) {
	if s.seenTimestamp != nil {
		*s.seenTimestamp = in.Timestamp
	}
	if s.err != nil {
		return policy.EvaluationResult{}, s.err
	}
	res := policy.EvaluationResult{
		PolicyID:   p.ID,
		PolicyName: p.Name,
		Allowed:    s.allow,
	}
	if !s.allow {
		res.Violations = []audit.Violation{{Rule: "stub", Message: "denied", Severity: p.Severity}}
	}
	return res, nil
}

func enginePolicy(id string, enabled bool) *policy.Policy {
	return &policy.Policy{
		ID: id, Name: id, Type: audit.PolicyTypeBuiltin,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Code: "{}", Enabled: enabled,
	}
}

// ---- construction / wiring (carried from task 5) ------------------

func TestNewEngine_RequiresRegistry(t *testing.T) {
	t.Parallel()
	if _, err := policy.NewEngine(nil); !errors.Is(err, policy.ErrEngineMisconfigured) {
		t.Errorf("err = %v, want ErrEngineMisconfigured", err)
	}
}

func TestNewEngine_OK(t *testing.T) {
	t.Parallel()
	e, err := policy.NewEngine(policy.NewRegistry())
	if err != nil || e.Registry() == nil {
		t.Errorf("NewEngine: e=%v err=%v", e, err)
	}
}

func TestWithEvaluator_WiringAndIgnores(t *testing.T) {
	t.Parallel()
	first := &stubEvaluator{allow: false}
	second := &stubEvaluator{allow: true}
	e, _ := policy.NewEngine(policy.NewRegistry(),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, first),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, second), // last-wins
		policy.WithEvaluator(audit.PolicyTypeOPA, nil),        // nil ignored
		policy.WithEvaluator(audit.PolicyType("ldap"), second), // unknown ignored
	)
	got, ok := e.Evaluator(audit.PolicyTypeBuiltin)
	if !ok || got != policy.Evaluator(second) {
		t.Errorf("builtin slot: ok=%v got=%v (want last-wins=second)", ok, got)
	}
	if _, ok := e.Evaluator(audit.PolicyTypeOPA); ok {
		t.Errorf("nil evaluator stored")
	}
	if _, ok := e.Evaluator(audit.PolicyType("ldap")); ok {
		t.Errorf("unknown-type evaluator stored")
	}
}

// ---- Evaluate -----------------------------------------------------

func TestEngine_Evaluate_Allow(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: true}))

	res, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Allowed || res.PolicyID != "p1" {
		t.Errorf("res = %+v", res)
	}
}

func TestEngine_Evaluate_DenyPropagatesViolations(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: false}))

	res, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if res.Allowed || len(res.Violations) != 1 {
		t.Errorf("res = %+v", res)
	}
}

func TestEngine_Evaluate_NotFound(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry())
	_, err := e.Evaluate(context.Background(), "missing", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEngine_Evaluate_DisabledIsError(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", false))
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: true}))

	_, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrPolicyDisabled) {
		t.Errorf("err = %v, want ErrPolicyDisabled", err)
	}
}

func TestEngine_Evaluate_NoEvaluatorForType(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true)) // builtin type
	e, _ := policy.NewEngine(r)                     // none wired
	_, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{})
	if !errors.Is(err, policy.ErrNoEvaluator) {
		t.Errorf("err = %v, want ErrNoEvaluator", err)
	}
}

func TestEngine_Evaluate_StampsTimestampWhenZero(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	var seen time.Time
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin,
		&stubEvaluator{allow: true, seenTimestamp: &seen}))

	if _, err := e.Evaluate(context.Background(), "p1", policy.EvaluationInput{}); err != nil {
		t.Fatalf("%v", err)
	}
	if seen.IsZero() {
		t.Errorf("engine did not stamp a zero timestamp")
	}
}

func TestEngine_Evaluate_RespectsProvidedTimestamp(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	var seen time.Time
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin,
		&stubEvaluator{allow: true, seenTimestamp: &seen}))

	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := e.Evaluate(context.Background(), "p1",
		policy.EvaluationInput{Timestamp: want}); err != nil {
		t.Fatalf("%v", err)
	}
	if !seen.Equal(want) {
		t.Errorf("seen = %v, want caller-provided %v", seen, want)
	}
}

// ---- EvaluatePolicySet --------------------------------------------

func setupSet(t *testing.T, members []*policy.Policy, ev policy.Evaluator) *policy.Engine {
	t.Helper()
	r := policy.NewRegistry()
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if err := r.RegisterPolicy(m); err != nil {
			t.Fatalf("register %s: %v", m.ID, err)
		}
		ids = append(ids, m.ID)
	}
	if err := r.RegisterPolicySet(&policy.PolicySet{
		ID: "set1", Name: "set1", PolicyIDs: ids, Enabled: true,
	}); err != nil {
		t.Fatalf("register set: %v", err)
	}
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, ev))
	return e
}

func TestEngine_EvaluatePolicySet_AllMembersInOrder(t *testing.T) {
	t.Parallel()
	e := setupSet(t, []*policy.Policy{
		enginePolicy("a", true), enginePolicy("b", true), enginePolicy("c", true),
	}, &stubEvaluator{allow: true})

	res, err := e.EvaluatePolicySet(context.Background(), "set1", policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res) != 3 {
		t.Fatalf("results = %d, want 3", len(res))
	}
	for i, id := range []string{"a", "b", "c"} {
		if res[i].PolicyID != id {
			t.Errorf("result[%d] = %s, want %s (member order)", i, res[i].PolicyID, id)
		}
	}
	if !policy.AllowedAll(res) {
		t.Errorf("AllowedAll = false, want true (all members allow)")
	}
}

func TestEngine_EvaluatePolicySet_SkipsDisabledMembers(t *testing.T) {
	t.Parallel()
	e := setupSet(t, []*policy.Policy{
		enginePolicy("a", true), enginePolicy("b", false), enginePolicy("c", true),
	}, &stubEvaluator{allow: true})

	res, err := e.EvaluatePolicySet(context.Background(), "set1", policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res) != 2 || res[0].PolicyID != "a" || res[1].PolicyID != "c" {
		t.Errorf("results = %+v, want [a c] (b disabled, skipped)", res)
	}
}

func TestEngine_EvaluatePolicySet_DisabledSetReturnsNil(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("a", true))
	_ = r.RegisterPolicySet(&policy.PolicySet{ID: "s", Name: "s", PolicyIDs: []string{"a"}, Enabled: false})
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: true}))

	res, err := e.EvaluatePolicySet(context.Background(), "s", policy.EvaluationInput{})
	if err != nil || res != nil {
		t.Errorf("disabled set: res=%+v err=%v, want nil/nil", res, err)
	}
}

func TestEngine_EvaluatePolicySet_NotFound(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry())
	if _, err := e.EvaluatePolicySet(context.Background(), "missing", policy.EvaluationInput{}); !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEngine_EvaluatePolicySet_FailFastOnEvaluatorError(t *testing.T) {
	t.Parallel()
	boom := errors.New("bad rego")
	e := setupSet(t, []*policy.Policy{
		enginePolicy("a", true), enginePolicy("b", true),
	}, &stubEvaluator{err: boom})

	_, err := e.EvaluatePolicySet(context.Background(), "set1", policy.EvaluationInput{})
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped %v", err, boom)
	}
}

func TestEngine_EvaluatePolicySet_StampsOnceSharedAcrossMembers(t *testing.T) {
	t.Parallel()
	var seen time.Time
	e := setupSet(t, []*policy.Policy{
		enginePolicy("a", true), enginePolicy("b", true),
	}, &stubEvaluator{allow: true, seenTimestamp: &seen})

	if _, err := e.EvaluatePolicySet(context.Background(), "set1", policy.EvaluationInput{}); err != nil {
		t.Fatalf("%v", err)
	}
	if seen.IsZero() {
		t.Errorf("set fan-out did not stamp the zero timestamp")
	}
}

// ---- EvaluateForResource ------------------------------------------

func TestEngine_EvaluateForResource_NoBindingsAllowByDefault(t *testing.T) {
	t.Parallel()
	e, _ := policy.NewEngine(policy.NewRegistry())
	res, err := e.EvaluateForResource(context.Background(), "secret", "read", nil, policy.EvaluationInput{})
	if err != nil || res != nil {
		t.Errorf("no bindings: res=%+v err=%v, want nil/nil", res, err)
	}
	if !policy.AllowedAll(res) {
		t.Errorf("AllowedAll on empty = false, want vacuously true")
	}
}

func TestEngine_EvaluateForResource_PerBindingNoDedup(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	// Two distinct bindings, both pointing at p1, both matching.
	_ = r.RegisterBinding(&policy.Binding{ID: "b1", PolicyID: "p1", ResourceType: "secret", Enabled: true})
	_ = r.RegisterBinding(&policy.Binding{ID: "b2", PolicyID: "p1", ResourceType: "secret", Enabled: true})
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: true}))

	res, err := e.EvaluateForResource(context.Background(), "secret", "write", nil, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res) != 2 {
		t.Errorf("results = %d, want 2 (p1 evaluated once per binding, no dedup)", len(res))
	}
}

func TestEngine_EvaluateForResource_SetTargetingBindingFlattens(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("a", true))
	_ = r.RegisterPolicy(enginePolicy("b", true))
	_ = r.RegisterPolicySet(&policy.PolicySet{ID: "s1", Name: "s1", PolicyIDs: []string{"a", "b"}, Enabled: true})
	_ = r.RegisterBinding(&policy.Binding{ID: "bind", PolicySetID: "s1", ResourceType: "lease", Enabled: true})
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: true}))

	res, err := e.EvaluateForResource(context.Background(), "lease", "renew", nil, policy.EvaluationInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res) != 2 {
		t.Errorf("results = %d, want 2 (set flattened)", len(res))
	}
}

func TestEngine_EvaluateForResource_DenyShowsInAllowedAll(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	_ = r.RegisterBinding(&policy.Binding{ID: "b1", PolicyID: "p1", ResourceType: "secret", Enabled: true})
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{allow: false}))

	res, _ := e.EvaluateForResource(context.Background(), "secret", "delete", nil, policy.EvaluationInput{})
	if policy.AllowedAll(res) {
		t.Errorf("AllowedAll = true, want false (p1 denied)")
	}
}

func TestEngine_EvaluateForResource_FailFast(t *testing.T) {
	t.Parallel()
	boom := errors.New("bad config")
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(enginePolicy("p1", true))
	_ = r.RegisterBinding(&policy.Binding{ID: "b1", PolicyID: "p1", ResourceType: "secret", Enabled: true})
	e, _ := policy.NewEngine(r, policy.WithEvaluator(audit.PolicyTypeBuiltin, &stubEvaluator{err: boom}))

	_, err := e.EvaluateForResource(context.Background(), "secret", "x", nil, policy.EvaluationInput{})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped %v", err, boom)
	}
}

// ---- cross-evaluator dispatch (real OPA / CEL / Builtin) ----------

func TestEngine_Dispatch_RoutesByPolicyType(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	// OPA policy (allow), CEL policy (deny), Builtin policy (allow).
	opa := &policy.Policy{ID: "opa1", Name: "opa1", Type: audit.PolicyTypeOPA,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: "package keystone.policy\n\nallow := true\n"}
	cel := &policy.Policy{ID: "cel1", Name: "cel1", Type: audit.PolicyTypeCEL,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: `action == "read"`}
	bi := &policy.Policy{ID: "bi1", Name: "bi1", Type: audit.PolicyTypeBuiltin,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: `{"rule":"allowed-actions","allowed":["read"]}`}
	for _, p := range []*policy.Policy{opa, cel, bi} {
		if err := r.RegisterPolicy(p); err != nil {
			t.Fatalf("register %s: %v", p.ID, err)
		}
	}
	e, _ := policy.NewEngine(r,
		policy.WithEvaluator(audit.PolicyTypeOPA, policy.NewOPAEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeCEL, policy.NewCELEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, policy.NewBuiltinEvaluator()),
	)

	ro, err := e.Evaluate(context.Background(), "opa1", policy.EvaluationInput{})
	if err != nil || !ro.Allowed {
		t.Errorf("OPA route: res=%+v err=%v", ro, err)
	}
	rc, err := e.Evaluate(context.Background(), "cel1", policy.EvaluationInput{Action: "read"})
	if err != nil || !rc.Allowed {
		t.Errorf("CEL route (read): res=%+v err=%v", rc, err)
	}
	rcDeny, err := e.Evaluate(context.Background(), "cel1", policy.EvaluationInput{Action: "delete"})
	if err != nil || rcDeny.Allowed {
		t.Errorf("CEL route (delete) should deny: res=%+v err=%v", rcDeny, err)
	}
	rb, err := e.Evaluate(context.Background(), "bi1", policy.EvaluationInput{Action: "read"})
	if err != nil || !rb.Allowed {
		t.Errorf("Builtin route: res=%+v err=%v", rb, err)
	}
}

func TestEngine_PolicySet_MixedEvaluators(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	opaAllow := &policy.Policy{ID: "o", Name: "o", Type: audit.PolicyTypeOPA,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: "package keystone.policy\n\nallow := true\n"}
	biDeny := &policy.Policy{ID: "d", Name: "d", Type: audit.PolicyTypeBuiltin,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: `{"rule":"allowed-actions","allowed":["read"]}`}
	_ = r.RegisterPolicy(opaAllow)
	_ = r.RegisterPolicy(biDeny)
	_ = r.RegisterPolicySet(&policy.PolicySet{ID: "mix", Name: "mix", PolicyIDs: []string{"o", "d"}, Enabled: true})
	e, _ := policy.NewEngine(r,
		policy.WithEvaluator(audit.PolicyTypeOPA, policy.NewOPAEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, policy.NewBuiltinEvaluator()),
	)

	// action=delete → builtin denies, OPA allows → AND = deny.
	res, err := e.EvaluatePolicySet(context.Background(), "mix", policy.EvaluationInput{Action: "delete"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res) != 2 {
		t.Fatalf("results = %d, want 2", len(res))
	}
	if policy.AllowedAll(res) {
		t.Errorf("AllowedAll = true, want false (builtin member denies delete)")
	}
}
