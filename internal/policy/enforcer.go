package policy

import (
	"context"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// EnforcementDecision is the result of [Enforcer.Enforce]. Allowed
// is the gate decision the caller acts on. WouldDeny is the real
// policy verdict — in v1.0 (enforcement disabled) Allowed is ALWAYS
// true, so WouldDeny is how compliance reporting (task 11) and the
// audit trail surface "this operation would have been blocked if
// enforcement were on" per PROJECT-DETAILS §4.12.
//
// Mode is the effective enforcement mode the caller supplied
// (recorded, and acted on only when enforcement is enabled —
// v1.8). Results is the evaluated set passed through so callers /
// audit emission don't have to re-thread it.
type EnforcementDecision struct {
	Allowed   bool
	WouldDeny bool
	Mode      audit.EnforcementMode
	Results   []EvaluationResult
}

// Enforcer is the §4.12 enforcement gate. v1.0 ships it in
// audit-mode-only: policies evaluate, audit, and report, but the
// Enforcer NEVER blocks (Allowed is always true). enforcement is
// hardcoded off; the WithEnforcementEnabled seam exists so the
// v1.8 gate path is tested + wiring-ready (mirrors Epic 11's
// RetentionEnforcer leader-check seam: real mechanism, v1.0-safe
// default).
//
// v1.8 flips enforcement on and honors EnforcementMode per policy.
// The allow/deny GATE for all three modes is implemented now (small
// + testable); the Warn-event emission and Enforce violation-handler
// SIDE-EFFECTS are deferred to a v1.x ROADMAP entry — they need
// infrastructure that does not exist in v1.0.
type Enforcer struct {
	enforcementEnabled bool
}

// EnforcerOption configures an Enforcer.
type EnforcerOption func(*Enforcer)

// WithEnforcementEnabled toggles real enforcement. v1.0 leaves this
// false (the §4.12 "hardcoded false in v1.0"); operator config
// plumbing for it is v1.8. Tests use it to exercise the v1.8 gate.
func WithEnforcementEnabled(enabled bool) EnforcerOption {
	return func(e *Enforcer) { e.enforcementEnabled = enabled }
}

// NewEnforcer returns an Enforcer. Default: enforcement disabled
// (audit-mode-only) — the v1.0 contract.
func NewEnforcer(opts ...EnforcerOption) *Enforcer {
	e := &Enforcer{enforcementEnabled: false}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Enabled reports whether real enforcement is on. Always false in
// v1.0 wiring; true only when a caller (tests, v1.8) opted in via
// WithEnforcementEnabled.
func (e *Enforcer) Enabled() bool { return e.enforcementEnabled }

// Enforce applies the enforcement gate to an evaluated result set.
// mode is the caller-derived effective enforcement mode (from the
// policy, or PolicySet.EffectiveMode for set/binding evaluations).
// results is one [Engine.Evaluate] result (wrap in a 1-element
// slice) or the slice from EvaluatePolicySet / EvaluateForResource.
//
// WouldDeny is always the real verdict: !AllowedAll(results).
//
//   - Enforcement disabled (v1.0): Allowed is ALWAYS true regardless
//     of WouldDeny or mode — audit-mode-only.
//   - Enforcement enabled (v1.8 path): Audit / Warn → allow (Warn's
//     warn-event is a deferred side-effect); Enforce → block on a
//     denying verdict (Allowed = !WouldDeny), violation-handler
//     invocation is a deferred side-effect.
func (e *Enforcer) Enforce(ctx context.Context, mode audit.EnforcementMode, results []EvaluationResult) EnforcementDecision {
	wouldDeny := !AllowedAll(results)
	d := EnforcementDecision{
		WouldDeny: wouldDeny,
		Mode:      mode,
		Results:   results,
	}
	if !e.enforcementEnabled {
		// v1.0: audit-mode-only — never block.
		d.Allowed = true
		return d
	}
	// v1.8 gate (seam): block only in Enforce mode on a denying
	// verdict. Side-effects (warn events, violation handlers) are
	// deferred — see the v1.x ROADMAP entry.
	switch mode {
	case audit.EnforcementModeEnforce:
		d.Allowed = !wouldDeny
	default: // Audit, Warn, Unknown → allow
		d.Allowed = true
	}
	return d
}
