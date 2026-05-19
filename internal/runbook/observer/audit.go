package observer

import (
	"context"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/runbook"
)

// AuditObserver emits one audit entry per terminal transition:
// execution succeeded/failed and step succeeded/failed. In-flight
// (→running) and skipped transitions do not audit — a skipped step
// never ran, mirroring internal/audit.StateApplyObserver.
type AuditObserver struct {
	Auditor audit.Auditor
}

// NewAuditObserver returns an AuditObserver. A nil auditor defaults
// to audit.NoopAuditor.
func NewAuditObserver(a audit.Auditor) *AuditObserver {
	if a == nil {
		a = audit.NoopAuditor{}
	}
	return &AuditObserver{Auditor: a}
}

// OnTransition implements [runbook.Observer].
func (o *AuditObserver) OnTransition(ctx context.Context, ev runbook.ObserverEvent) {
	if o == nil || o.Auditor == nil {
		return
	}
	if ev.To != runbook.StatusSucceeded && ev.To != runbook.StatusFailed {
		return // only terminal outcomes are audited
	}

	action := "runbook.execute"
	if ev.Step != "" {
		action = "runbook.step"
	}
	allowed := ev.To == runbook.StatusSucceeded
	sev := audit.SeverityLow
	if !allowed {
		sev = audit.SeverityHigh
	}
	meta := map[string]string{
		"execution_id": ev.ExecutionID,
		"runbook":      ev.Runbook,
		"status":       string(ev.To),
	}
	if ev.Step != "" {
		meta["step"] = ev.Step
	}
	if ev.Note != "" {
		meta["note"] = ev.Note
	}

	entry, err := audit.NewAuditEntry(audit.AuditEntryInput{
		Action:       action,
		ResourceType: "runbook",
		Allowed:      allowed,
		Severity:     sev,
		Metadata:     meta,
	})
	if err != nil {
		return // Action is always set; defensive only
	}
	o.Auditor.Emit(ctx, entry)
}
