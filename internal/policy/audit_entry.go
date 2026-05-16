package policy

import "go.keystone-core.io/keystone-core/internal/audit"

// EvaluationAuditAction is the audit.AuditEntry.Action stamped on
// every policy-evaluation audit record — the §4.12 "every sensitive
// op MUST emit" hook for policy eval.
const EvaluationAuditAction = "policy.evaluate"

// EvaluationAuditEntry builds the canonical audit record for one
// policy evaluation. Both the gRPC PolicyService server and the
// REST handler call this so the audit shape has a single source of
// truth (a drifting shape is exactly what the "every sensitive op
// emits" contract guards against).
//
// Severity is max(res.Violations[].Severity) falling back to
// SeverityLow when the evaluation produced no violations (an
// allow). policyType is the evaluated policy's type (the caller
// resolves it from the registry; zero value is fine — it just
// records "" for a non-resolvable policy). user comes from the
// evaluation input.
//
// Returns the construction error from audit.NewAuditEntry; callers
// treat emission as fire-and-forget (an error means drop the
// record, never fail the in-flight op — matches the Epic 12 task-4
// hooks).
func EvaluationAuditEntry(policyType audit.PolicyType, res EvaluationResult, user string) (audit.AuditEntry, error) {
	sev := audit.SeverityLow
	for _, v := range res.Violations {
		if v.Severity.AtLeast(sev) {
			sev = v.Severity
		}
	}
	return audit.NewAuditEntry(audit.AuditEntryInput{
		PolicyID:   res.PolicyID,
		PolicyName: res.PolicyName,
		PolicyType: policyType,
		Allowed:    res.Allowed,
		Duration:   res.Duration,
		Violations: res.Violations,
		Severity:   sev,
		User:       user,
		Action:     EvaluationAuditAction,
	})
}
