// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strconv"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/secrets"
)

// secretsAuditStoreBridge implements [secrets.Auditor] by translating
// each [secrets.SecretAccessEvent] into an [audit.AuditEntry] and
// emitting it through the configured [audit.Auditor]. Runs in
// parallel with [secretsAuditEventBridge] (which writes to the
// events bus): the SQL [audit.AuditStore] is the long-term forensic
// channel; the events bus is the realtime/7d channel. Both fan-outs
// live behind the secrets [secrets.MultiAuditor] in buildAuditor.
//
// Fire-and-forget per the §4.12 / Epic 10/11 precedent — the wrapped
// auditor's Emit signature has no error return, and the [audit.
// StoreAuditor] internally logs + counts store failures.
type secretsAuditStoreBridge struct {
	auditor audit.Auditor
}

// newSecretsAuditStoreBridge constructs the bridge. auditor MUST be
// non-nil; callers in degraded mode (no audit store) should skip
// constructing the bridge entirely rather than wire in
// [audit.NoopAuditor] — keeps the buildAuditor fan-out lean and
// avoids the "is this branch even alive?" question downstream.
func newSecretsAuditStoreBridge(auditor audit.Auditor) *secretsAuditStoreBridge {
	return &secretsAuditStoreBridge{auditor: auditor}
}

// Emit translates the SecretAccessEvent into an AuditEntry and
// emits it. The broker invokes this on every secret op
// (§4.11 "failure to log = bug").
//
// Mapping:
//
//   - Action       ← event.Action (e.g. "secret.get")
//   - ResourceType ← "secret"
//   - User         ← actorFromPrincipal(event.Principal) — SPIFFE > AgentID > User
//   - Allowed      ← event.Allowed
//   - Duration     ← event.Duration
//   - Severity     ← Low on allowed; High on denied
//   - Violations   ← single-element from event.ErrorReason on denied
//   - Metadata     ← {backend, path, lease_id, duration_ns} where present;
//                    masked_payload is intentionally NOT copied — the events
//                    bus emission already carries the masked payload and
//                    audit rows favor compact rolls.
func (b *secretsAuditStoreBridge) Emit(ctx context.Context, event secrets.SecretAccessEvent) {
	severity := audit.SeverityLow
	var violations []audit.Violation
	if !event.Allowed {
		severity = audit.SeverityHigh
		if event.ErrorReason != "" {
			violations = []audit.Violation{{
				Rule:     "secret.denied",
				Message:  event.ErrorReason,
				Severity: severity,
			}}
		}
	}
	entry, err := audit.NewAuditEntry(audit.AuditEntryInput{
		Action:       event.Action,
		ResourceType: secretsAuditResourceType,
		Allowed:      event.Allowed,
		Duration:     event.Duration,
		User:         actorFromPrincipal(event.Principal),
		Severity:     severity,
		Violations:   violations,
		Metadata:     secretsAuditMetadata(event),
	})
	if err != nil {
		// NewAuditEntry's failure modes are bad Severity / bad
		// EnforcementMode / empty Action — Action is always set by
		// the broker. Silently drop rather than crash the broker.
		return
	}
	if !event.Timestamp.IsZero() {
		entry.Timestamp = event.Timestamp.UTC()
	}
	b.auditor.Emit(ctx, entry)
}

// Compile-time interface compliance check.
var _ secrets.Auditor = (*secretsAuditStoreBridge)(nil)

// secretsAuditMetadata builds the audit-entry metadata map from the
// SecretAccessEvent. Returns nil when no fields are populated so the
// AuditEntry's Metadata stays sparse (cheaper to scan in SQL).
func secretsAuditMetadata(e secrets.SecretAccessEvent) map[string]string {
	m := make(map[string]string, 4)
	if e.Backend != "" {
		m["backend"] = e.Backend
	}
	if e.Path != "" {
		m["path"] = e.Path
	}
	if e.LeaseID != "" {
		m["lease_id"] = e.LeaseID
	}
	if e.Duration > 0 {
		m["duration_ns"] = strconv.FormatInt(e.Duration.Nanoseconds(), 10)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
