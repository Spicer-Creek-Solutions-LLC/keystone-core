package policy

import (
	"context"
	"errors"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// ErrNoEvaluator is returned by the Engine.Evaluate* methods when no
// Evaluator is registered for a policy's type. In task 5 this is the
// expected result for every call — the OPA / CEL / Builtin
// evaluators land in tasks 6-8 and the dispatch logic in task 9.
// Defining the methods now keeps the public surface stable for the
// gRPC / REST / CLI wiring (tasks 12-14) to compile against.
var ErrNoEvaluator = errors.New("policy: no evaluator registered for policy type")

// ErrEngineMisconfigured is returned by NewEngine when required
// wiring is missing (nil registry).
var ErrEngineMisconfigured = errors.New("policy: engine misconfigured")

// Engine is the §4.12 policy coordinator. It owns the Registry and
// routes each policy to the Evaluator matching its
// audit.PolicyType. v1.0 ships the engine in audit-mode-only: the
// Enforcer (task 10) ignores the verdict; evaluation + audit still
// happen.
//
// Task 5 builds the shell: Registry + evaluator slots + the
// Evaluate* method signatures. Tasks 6-8 supply the evaluators via
// WithEvaluator; task 9 fills the Evaluate* dispatch + result
// aggregation logic. Until then Evaluate* return ErrNoEvaluator.
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

// Evaluate runs a single registered policy against input.
//
// Task 5: returns ErrNoEvaluator (no evaluators wired yet). Task 9
// implements: resolve policyID → Policy, route by Policy.Type to
// the matching Evaluator, stamp input.Timestamp, return its result.
func (e *Engine) Evaluate(ctx context.Context, policyID string, input EvaluationInput) (EvaluationResult, error) {
	p, err := e.registry.GetPolicy(policyID)
	if err != nil {
		return EvaluationResult{}, err
	}
	_, ok := e.evaluators[p.Type]
	if !ok {
		return EvaluationResult{}, fmt.Errorf("%w: %q (policy %q)", ErrNoEvaluator, p.Type, policyID)
	}
	// Dispatch lands in task 9.
	return EvaluationResult{}, fmt.Errorf("%w: Engine.Evaluate dispatch lands in Epic 12 task 9", ErrNoEvaluator)
}

// EvaluatePolicySet runs every member policy of setID against input
// and aggregates the results.
//
// Task 5: returns ErrNoEvaluator. Task 9 implements the per-member
// fan-out + aggregation (EnforcementOverride applied via
// PolicySet.EffectiveMode).
func (e *Engine) EvaluatePolicySet(ctx context.Context, setID string, input EvaluationInput) ([]EvaluationResult, error) {
	if _, err := e.registry.GetPolicySet(setID); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: Engine.EvaluatePolicySet dispatch lands in Epic 12 task 9", ErrNoEvaluator)
}

// EvaluateForResource resolves every binding matching resourceType
// (+ action + labels) and evaluates the bound policies/sets.
//
// Task 5: returns ErrNoEvaluator. Task 9 implements binding
// resolution → per-policy/set evaluation → aggregation.
func (e *Engine) EvaluateForResource(ctx context.Context, resourceType, action string, labels map[string]string, input EvaluationInput) ([]EvaluationResult, error) {
	_ = e.registry.BindingsForResource(resourceType, action, labels)
	return nil, fmt.Errorf("%w: Engine.EvaluateForResource dispatch lands in Epic 12 task 9", ErrNoEvaluator)
}
