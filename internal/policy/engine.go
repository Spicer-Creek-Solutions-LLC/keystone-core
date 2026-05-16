package policy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// ErrNoEvaluator is returned by the Engine.Evaluate* methods when no
// Evaluator is registered for a policy's type (the OPA / CEL /
// Builtin evaluators are wired via WithEvaluator at boot).
var ErrNoEvaluator = errors.New("policy: no evaluator registered for policy type")

// ErrEngineMisconfigured is returned by NewEngine when required
// wiring is missing (nil registry).
var ErrEngineMisconfigured = errors.New("policy: engine misconfigured")

// ErrPolicyDisabled is returned by Engine.Evaluate when the named
// policy is registered but Enabled=false. The caller asked for this
// policy by ID, so the disabled state is surfaced distinctly rather
// than masked as a clean allow. Bulk evaluation
// (EvaluatePolicySet / EvaluateForResource) silently skips disabled
// members instead — a disabled member just doesn't contribute.
var ErrPolicyDisabled = errors.New("policy: policy is disabled")

// Engine is the §4.12 policy coordinator. It owns the Registry and
// routes each policy to the Evaluator matching its
// audit.PolicyType. v1.0 ships the engine in audit-mode-only: the
// Enforcer (task 10) ignores the verdict; evaluation + audit still
// happen.
//
// Evaluators are supplied via WithEvaluator at construction (OPA /
// CEL / Builtin from Epic 12 tasks 6-8). The Evaluate* dispatch +
// aggregation is the task-9 implementation below.
type Engine struct {
	registry   *Registry
	evaluators map[audit.PolicyType]Evaluator
}

// EngineOption configures an Engine at construction.
type EngineOption func(*Engine)

// WithEvaluator registers eval for policies of type pt. Later
// options for the same type overwrite earlier ones (last-wins) so
// boot wiring can layer a default then override in tests. A nil
// evaluator or unknown type is ignored — the slot stays empty and
// Evaluate* returns ErrNoEvaluator for that type.
func WithEvaluator(pt audit.PolicyType, eval Evaluator) EngineOption {
	return func(e *Engine) {
		if eval == nil || !pt.IsKnown() {
			return
		}
		e.evaluators[pt] = eval
	}
}

// NewEngine returns an Engine backed by reg. Returns
// ErrEngineMisconfigured when reg is nil — the engine has no
// degraded "no registry" mode.
func NewEngine(reg *Registry, opts ...EngineOption) (*Engine, error) {
	if reg == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrEngineMisconfigured)
	}
	e := &Engine{
		registry:   reg,
		evaluators: make(map[audit.PolicyType]Evaluator, 3),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e, nil
}

// Registry returns the engine's policy registry so callers (gRPC /
// REST handlers in tasks 12-13) can register + list without holding
// a separate reference.
func (e *Engine) Registry() *Registry { return e.registry }

// Evaluator returns the registered evaluator for pt and whether one
// is present. Used by task 9's dispatch + by tests asserting the
// WithEvaluator wiring.
func (e *Engine) Evaluator(pt audit.PolicyType) (Evaluator, bool) {
	ev, ok := e.evaluators[pt]
	return ev, ok
}

// stampTime sets input.Timestamp to now (UTC) only when the caller
// left it zero, so every member of a set / resource fan-out sees
// the same instant (time-window builtin consistency). A
// caller-supplied timestamp — e.g. historical replay — is
// respected.
func stampTime(input EvaluationInput) EvaluationInput {
	if input.Timestamp.IsZero() {
		input.Timestamp = time.Now().UTC()
	}
	return input
}

// dispatch routes an already-resolved, enabled policy to its
// evaluator. Timestamp is assumed already stamped by the caller.
func (e *Engine) dispatch(ctx context.Context, p *Policy, input EvaluationInput) (EvaluationResult, error) {
	ev, ok := e.evaluators[p.Type]
	if !ok {
		return EvaluationResult{}, fmt.Errorf("%w: %q (policy %q)", ErrNoEvaluator, p.Type, p.ID)
	}
	return ev.Evaluate(ctx, p, input)
}

// Evaluate runs a single registered policy against input. Resolves
// policyID (ErrNotFound if absent), rejects a disabled policy with
// ErrPolicyDisabled (the caller asked for it by name), routes by
// Policy.Type (ErrNoEvaluator if no evaluator is wired for that
// type), and returns the evaluator's result/error unchanged.
func (e *Engine) Evaluate(ctx context.Context, policyID string, input EvaluationInput) (EvaluationResult, error) {
	p, err := e.registry.GetPolicy(policyID)
	if err != nil {
		return EvaluationResult{}, err
	}
	if !p.Enabled {
		return EvaluationResult{}, fmt.Errorf("%w: %q", ErrPolicyDisabled, policyID)
	}
	return e.dispatch(ctx, p, stampTime(input))
}

// EvaluatePolicySet runs every enabled member policy of setID
// against input and returns the per-member results in member order.
//
// A disabled set, or any disabled member, is silently skipped
// (bulk evaluation — a disabled member just doesn't contribute; a
// disabled set contributes nothing). Set verdict is the §4.12
// "all-or-nothing AND" — use AllowedAll on the returned slice.
//
// Fail-fast: the first member whose evaluator returns an error
// (malformed Rego/CEL/JSON config) aborts the fan-out and the error
// is returned wrapped with the offending policy ID — an evaluator
// error is a misconfiguration that must be loud, and the set
// verdict is genuinely unknown so no result is fabricated.
//
// EnforcementOverride (PolicySet.EffectiveMode) has no observable
// v1.0 effect — EvaluationResult carries no mode and the v1.0
// Enforcer (task 10) ignores mode — so it is deliberately not
// applied here; the Enforcer re-derives the effective mode from
// policy + set where it matters.
func (e *Engine) EvaluatePolicySet(ctx context.Context, setID string, input EvaluationInput) ([]EvaluationResult, error) {
	set, err := e.registry.GetPolicySet(setID)
	if err != nil {
		return nil, err
	}
	if !set.Enabled {
		return nil, nil
	}
	input = stampTime(input)
	var results []EvaluationResult
	for _, pid := range set.PolicyIDs {
		p, err := e.registry.GetPolicy(pid)
		if err != nil {
			// A set member that no longer resolves is a registry
			// integrity failure (RegisterPolicySet rejected dangling
			// refs, so this only happens if registration invariants
			// were bypassed). Surface it loudly.
			return nil, fmt.Errorf("policy set %q: member %q: %w", setID, pid, err)
		}
		if !p.Enabled {
			continue
		}
		res, err := e.dispatch(ctx, p, input)
		if err != nil {
			return nil, fmt.Errorf("policy set %q: member %q: %w", setID, pid, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// EvaluateForResource resolves every enabled binding matching
// resourceType (+ action + labels) and evaluates the bound policy
// or policy-set, flattening all results into one slice.
//
// Bindings are evaluated in the registry's deterministic order; a
// policy referenced by multiple bindings (or by a binding plus a
// set member) is evaluated once per occurrence — no dedup, since
// bindings are intentional attachments and the caller should see
// exactly which binding produced which result.
//
// No matched bindings → empty slice, nil error: a resource with no
// policy attached is allow-by-default, not an error. Fail-fast on
// any evaluator-internal error (same rationale as
// EvaluatePolicySet).
func (e *Engine) EvaluateForResource(ctx context.Context, resourceType, action string, labels map[string]string, input EvaluationInput) ([]EvaluationResult, error) {
	bindings := e.registry.BindingsForResource(resourceType, action, labels)
	if len(bindings) == 0 {
		return nil, nil
	}
	input = stampTime(input)
	var results []EvaluationResult
	for _, b := range bindings {
		if b.TargetsSet() {
			setRes, err := e.EvaluatePolicySet(ctx, b.PolicySetID, input)
			if err != nil {
				return nil, fmt.Errorf("binding %q: %w", b.ID, err)
			}
			results = append(results, setRes...)
			continue
		}
		p, err := e.registry.GetPolicy(b.PolicyID)
		if err != nil {
			return nil, fmt.Errorf("binding %q: %w", b.ID, err)
		}
		if !p.Enabled {
			continue
		}
		res, err := e.dispatch(ctx, p, input)
		if err != nil {
			return nil, fmt.Errorf("binding %q: policy %q: %w", b.ID, b.PolicyID, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// AllowedAll reports the §4.12 "all-or-nothing AND" policy-set
// verdict: true iff every result allowed. An empty slice is
// vacuously allowed (no policy attached → allow-by-default). Task
// 10's Enforcer + the gRPC/REST handlers use this for the combined
// decision without re-implementing the fold.
func AllowedAll(results []EvaluationResult) bool {
	for _, r := range results {
		if !r.Allowed {
			return false
		}
	}
	return true
}
