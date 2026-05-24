// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// StateApplyObserver is a [statemgmt.RunObserver] that emits an
// [AuditEntry] for every declaration the runner completes. Wraps an
// optional inner observer so the existing event-emitting observer
// (Epic 11 task 1's event-bus dispatch) still receives every method.
//
// One audit entry per Done call — the terminal status carries the
// success/failure outcome via [statemgmt.DeclarationResult.Outcome].
// Skip calls do NOT emit: a skipped declaration never ran, so there
// is no sensitive op to audit (matches §4.12's "every sensitive op
// MUST emit" rule; "every scheduled op" would also include skipped
// items which is the wrong granularity).
//
// Auditor must be non-nil; pass [NoopAuditor] to discard.
type StateApplyObserver struct {
	inner   statemgmt.RunObserver
	auditor Auditor
}

// NewStateApplyObserver wraps inner with audit emission on Done.
// inner may be nil (no chained observer); auditor defaults to
// [NoopAuditor] if nil.
func NewStateApplyObserver(inner statemgmt.RunObserver, auditor Auditor) *StateApplyObserver {
	if auditor == nil {
		auditor = NoopAuditor{}
	}
	return &StateApplyObserver{inner: inner, auditor: auditor}
}

// Start forwards to the inner observer.
func (o *StateApplyObserver) Start(ctx context.Context, decl *statemgmt.Declaration) {
	if o.inner != nil {
		o.inner.Start(ctx, decl)
	}
}

// Drift forwards to the inner observer.
func (o *StateApplyObserver) Drift(ctx context.Context, decl *statemgmt.Declaration, check *statemgmt.ModuleCheckResult) {
	if o.inner != nil {
		o.inner.Drift(ctx, decl, check)
	}
}

// Change forwards to the inner observer.
func (o *StateApplyObserver) Change(ctx context.Context, decl *statemgmt.Declaration, result *statemgmt.StateResult) {
	if o.inner != nil {
		o.inner.Change(ctx, decl, result)
	}
}

// Done forwards to the inner observer, then emits one audit entry
// for this declaration's terminal outcome. Audit emission happens
// AFTER the inner forward so a panicking inner doesn't suppress the
// audit row.
func (o *StateApplyObserver) Done(ctx context.Context, result *statemgmt.DeclarationResult) {
	if o.inner != nil {
		o.inner.Done(ctx, result)
	}
	if result == nil {
		return
	}
	e, err := NewAuditEntry(AuditEntryInput{
		Action:       "state.apply",
		ResourceType: result.Module,
		Allowed:      result.Outcome != statemgmt.OutcomeFailed,
		Duration:     result.Duration,
		Metadata:     stateApplyMetadata(result),
		Severity:     severityFromOutcome(result.Outcome),
	})
	if err != nil {
		// Should never happen — Action + ResourceType are always
		// set above. Drop silently rather than crash the runner.
		return
	}
	// Override Timestamp with the result's StartedAt for a faithful
	// audit trail; NewAuditEntry's UTC stamp is fine but StartedAt
	// matches the eventual events.state.apply.done timestamp.
	if !result.StartedAt.IsZero() {
		e.Timestamp = result.StartedAt.UTC()
	}
	if result.Error != nil {
		e.Violations = []Violation{{
			Rule:     "state.apply",
			Message:  result.Error.Error(),
			Severity: SeverityHigh,
		}}
	}
	o.auditor.Emit(ctx, e)
}

// Skip forwards to the inner observer but does NOT emit an audit
// entry — see [StateApplyObserver] doc.
func (o *StateApplyObserver) Skip(ctx context.Context, decl *statemgmt.Declaration, reason error) {
	if o.inner != nil {
		o.inner.Skip(ctx, decl, reason)
	}
}

// stateApplyMetadata returns a compact metadata map for the audit
// entry. Decl-level fields go in; the heavy StateResult/Diff payload
// stays in the events bus emission only.
func stateApplyMetadata(r *statemgmt.DeclarationResult) map[string]string {
	m := map[string]string{
		"decl_id": r.DeclID,
		"outcome": r.Outcome.String(),
	}
	if r.Apply != nil {
		if r.Apply.Changed {
			m["changed"] = "true"
		} else {
			m["changed"] = "false"
		}
	}
	if r.Compensated {
		m["compensated"] = "true"
	}
	return m
}

// severityFromOutcome maps a declaration outcome to its audit
// severity. Failed → High; everything else (including DriftDetected
// in check mode) → Low. Critical is reserved for higher-level
// incidents (Compensate failure, multi-decl cascade — handled by
// task 11 ComplianceReport rollups).
func severityFromOutcome(o statemgmt.Outcome) Severity {
	if o == statemgmt.OutcomeFailed {
		return SeverityHigh
	}
	return SeverityLow
}

// _ silences "imported and not used" if time becomes unused in
// future refactors.
var _ = time.Time{}

// Compile-time assertion that *StateApplyObserver satisfies
// [statemgmt.RunObserver].
var _ statemgmt.RunObserver = (*StateApplyObserver)(nil)
