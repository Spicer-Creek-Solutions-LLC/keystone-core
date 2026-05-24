// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/secrets"
)

// secretsAuditSource is the [events.AuditEventInput.Source] every
// audit event the bridge produces carries. Distinguishes
// secrets-domain audit emissions from future Epic 12 user-action /
// policy-evaluation emitters that ride the same bus + types.
const secretsAuditSource = "secrets-broker"

// secretsAuditResourceType is the [events.AuditTagResourceType]
// canonical value for every secrets-domain audit event. Audit
// query consumers (Epic 12, the deferred kscore-events audit /
// kscore-secrets audit subcommands) filter on
// `tags.resource_type == "secret"` to scope to this domain.
const secretsAuditResourceType = "secret"

// secretsAuditEventBridge implements [secrets.Auditor] by
// translating each [secrets.SecretAccessEvent] into an
// [events.AuditEventInput] and emitting it through the configured
// [events.AuditEmitter].
//
// Sync publish per the task 10 user-confirmed plan: the broker
// blocks on the NATS ack before its op returns. Note: the
// [secrets.Auditor.Emit] signature has no error return — the
// broker's contract is "fire and forget; never error back to the
// caller." So a publish failure here can't actually fail the
// in-flight secrets op. The bridge logs the failure at ERROR (the
// AuditEmitter does this internally via slog) + bumps the
// emitter's [events.AuditEmitter.FailedPublishes] counter. A v1.x
// ROADMAP entry tracks stronger consistency via an Auditor.Emit
// signature change.
type secretsAuditEventBridge struct {
	emitter *events.AuditEmitter
	logger  *slog.Logger
}

// newSecretsAuditEventBridge constructs the bridge. emitter MUST be
// non-nil — the bridge has no degraded mode (callers checking for
// "events disabled" should skip constructing the bridge entirely,
// not pass a nil emitter).
func newSecretsAuditEventBridge(emitter *events.AuditEmitter, logger *slog.Logger) *secretsAuditEventBridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &secretsAuditEventBridge{emitter: emitter, logger: logger}
}

// Emit translates the SecretAccessEvent into an audit event and
// publishes synchronously. The broker calls Emit on every secret
// op (per the §4.11 "failure to log = bug" invariant); the bridge
// fans it out to the events bus alongside the existing
// LogAuditor / BufferedAuditor / SamplingAuditor entries in the
// secrets MultiAuditor.
//
// Failures from the AuditEmitter (validation or publish) are logged
// at WARN and the broker continues — see the type-level comment for
// the rationale.
func (b *secretsAuditEventBridge) Emit(ctx context.Context, event secrets.SecretAccessEvent) {
	in := events.AuditEventInput{
		Source:        secretsAuditSource,
		Actor:         actorFromPrincipal(event.Principal),
		Action:        event.Action,
		ResourceType:  secretsAuditResourceType,
		Resource:      resourceFromEvent(event),
		Outcome:       outcomeFromAllowed(event.Allowed),
		Reason:        event.ErrorReason,
		CorrelationID: "", // §4.11 SecretAccessEvent doesn't carry a correlation id; Epic 12 adds plumbing
		ExtraTags:     bridgeExtraTags(event),
		Data:          bridgeData(event),
	}
	if err := b.emitter.Emit(ctx, in); err != nil {
		b.logger.LogAttrs(ctx, slog.LevelWarn,
			"events: secrets audit bridge publish failed (compliance gap)",
			slog.String("audit_action", event.Action),
			slog.String("audit_path", event.Path),
			slog.String("audit_lease_id", event.LeaseID),
			slog.Bool("audit_allowed", event.Allowed),
			slog.Any("error", err),
		)
	}
}

// Compile-time interface compliance check.
var _ secrets.Auditor = (*secretsAuditEventBridge)(nil)

// actorFromPrincipal extracts the most-specific identifier the
// principal carries. SPIFFE ID wins (mTLS-authenticated callers,
// Epic 09 task 13); AgentID is the in-cluster fallback; User is
// the human operator (API-key / JWT path). Returns empty when none
// is set — operator audit consumers expect "" to mean "system /
// no upstream principal."
func actorFromPrincipal(p secrets.Principal) string {
	if p.SPIFFEID != "" {
		return p.SPIFFEID
	}
	if p.AgentID != "" {
		return p.AgentID
	}
	return p.User
}

// resourceFromEvent prefers Path over LeaseID — SecretAccessEvent
// is "Path empty when LeaseID is set" per the type docstring.
// Returns empty when both are absent (system-driven events that
// don't have a target resource).
func resourceFromEvent(e secrets.SecretAccessEvent) string {
	if e.Path != "" {
		return e.Path
	}
	return e.LeaseID
}

// outcomeFromAllowed maps the boolean to the canonical AuditOutcome.
func outcomeFromAllowed(allowed bool) events.AuditOutcome {
	if allowed {
		return events.AuditOutcomeAllowed
	}
	return events.AuditOutcomeDenied
}

// bridgeExtraTags surfaces SecretAccessEvent fields the canonical
// audit-tag set doesn't cover: backend name (which Vault / file
// store served the op) and lease ID (when Path is the primary
// resource).
func bridgeExtraTags(e secrets.SecretAccessEvent) map[string]string {
	tags := make(map[string]string, 2)
	if e.Backend != "" {
		tags["backend"] = e.Backend
	}
	if e.LeaseID != "" && e.Path != "" {
		// When Path is the primary resource, lease_id is metadata —
		// surface it as a tag so audit queries can pivot to lease ops.
		tags["lease_id"] = e.LeaseID
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// bridgeData stuffs the SecretAccessEvent's duration + masked
// payload into [Event.Data] for callers wanting the full record.
// Tags carry the fields audit queries pivot on; Data carries the
// detail.
func bridgeData(e secrets.SecretAccessEvent) map[string]any {
	data := make(map[string]any, 3)
	data["duration_ns"] = e.Duration.Nanoseconds()
	if !e.Timestamp.IsZero() {
		// SecretAccessEvent has its own timestamp; preserve it in
		// data even though the wrapping events.Event stamps its own
		// Time (the broker call site's timestamp may differ slightly
		// from the bridge's NewEvent time).
		data["secret_op_timestamp"] = e.Timestamp.Format("2006-01-02T15:04:05.000000000Z07:00")
	}
	if e.MaskedPayload != nil {
		data["masked_payload"] = e.MaskedPayload
	}
	return data
}
