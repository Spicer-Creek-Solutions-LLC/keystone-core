package policy

import (
	"context"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// EvaluationInput is the §4.12 evaluator input. Resource + Context
// are free-form maps so each evaluator binds them into its own
// model (OPA: `input`; CEL: `resource` / `context` vars; Builtin:
// typed lookups). Timestamp is the evaluation wall-clock — set by
// the Engine, not the caller, so time-window builtin rules see a
// consistent value across a policy-set evaluation.
type EvaluationInput struct {
	Resource  map[string]any
	Action    string
	User      string
	Context   map[string]any
	Timestamp time.Time
}

// EvaluationResult is the §4.12 evaluator output. Violations reuses
// audit.Violation so the result flows into the audit log with no
// translation (Epic 12 task 4's emission consumes audit.Violation
// directly). Allowed is the evaluator's verdict; the v1.0 Enforcer
// (task 10) ignores it and always permits, but the result still
// records what *would* have been blocked.
type EvaluationResult struct {
	PolicyID    string
	PolicyName  string
	Allowed     bool
	Violations  []audit.Violation
	Warnings    []string
	Message     string
	Duration    time.Duration
	EvaluatedAt time.Time
}

// Evaluator is the seam tasks 6-8 implement (OPA / CEL / Builtin).
// The Engine selects an evaluator by the policy's audit.PolicyType
// and delegates. Implementations MUST NOT mutate the input maps —
// the Engine may reuse one EvaluationInput across every policy in
// a set.
//
// An Evaluator returns a non-nil error only for evaluator-internal
// failures (malformed policy Code, evaluator runtime panic recovered,
// etc.). A policy that legitimately denies is NOT an error: that is
// `EvaluationResult{Allowed:false, Violations:[...]}` with err==nil.
type Evaluator interface {
	// Evaluate runs policy against input. policy.Type is guaranteed
	// to match the evaluator's type (the Engine routes by type), and
	// policy has already passed Validate.
	Evaluate(ctx context.Context, policy *Policy, input EvaluationInput) (EvaluationResult, error)
}
