package main

import (
	"context"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// authAuditResourceType is the [audit.AuditEntry.ResourceType] every
// auth-decision emission carries. Audit consumers (kscore-audit task
// 14, ComplianceReport task 11) filter on `resource_type == "rpc"`
// to scope to the auth-decision channel.
const authAuditResourceType = "rpc"

// newAuthDecisionEmitter builds an [auth.AuthDecisionFunc] that
// emits an [audit.AuditEntry] through auditor on every Authorize
// result — both allow and deny per the §4.12 "every sensitive op
// MUST emit" rule.
//
// Mapping:
//
//   - Action       ← gRPC FullMethod ("/svc/Method")
//   - ResourceType ← "rpc"
//   - User         ← principal.ID (SPIFFE / AgentID / API key holder); empty for bypass paths
//   - Allowed      ← interceptor-supplied allowed bit
//   - Severity     ← Low on allow; Medium on deny (auth denials are common; reserve High for compliance violations)
//   - Violations   ← single-element from reason on deny
func newAuthDecisionEmitter(auditor audit.Auditor) auth.AuthDecisionFunc {
	return func(ctx context.Context, method string, principal *auth.Principal, allowed bool, reason error) {
		if auditor == nil {
			return
		}
		severity := audit.SeverityLow
		var violations []audit.Violation
		if !allowed {
			severity = audit.SeverityMedium
			msg := "auth: denied"
			if reason != nil {
				msg = reason.Error()
			}
			violations = []audit.Violation{{
				Rule:     "auth.authorize",
				Message:  msg,
				Severity: severity,
			}}
		}
		var user string
		if principal != nil {
			user = principal.ID
		}
		entry, err := audit.NewAuditEntry(audit.AuditEntryInput{
			Action:       method,
			ResourceType: authAuditResourceType,
			Allowed:      allowed,
			Severity:     severity,
			Violations:   violations,
			User:         user,
		})
		if err != nil {
			// NewAuditEntry validates Action/Severity/EnforcementMode;
			// all three are stamped above. Silently drop on the
			// off-chance method is empty.
			return
		}
		auditor.Emit(ctx, entry)
	}
}
