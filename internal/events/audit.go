// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
)

// Audit tag keys — the canonical shape every Audit-flavored Event
// carries on its [Event.Tags] map. Consumers (Epic 12 audit query,
// the CLI's deferred `kscore-events audit` subcommand, operator
// dashboards) read against these keys; producers should populate
// via [AuditEventInput] rather than stamping tags by hand so the
// names stay in lockstep.
const (
	// AuditTagActor identifies the principal — SPIFFE ID > AgentID >
	// User, in that order of specificity. Empty when the caller is
	// the in-process system (e.g., the retention scheduler firing
	// a maintenance event with no upstream principal).
	AuditTagActor = "actor"

	// AuditTagAction is the canonical action verb — `get_secret`,
	// `write_secret`, `policy.evaluate`, etc. Producers should reuse
	// their domain's existing action constant (e.g., Epic 10's
	// `secrets.ActionGetSecret`) so audit queries can match across
	// domains.
	AuditTagAction = "action"

	// AuditTagResource is the resource identifier the actor touched
	// — a secret path, a lease ID, a policy ID, an agent ID, etc.
	AuditTagResource = "resource"

	// AuditTagResourceType disambiguates resources across domains
	// — `secret`, `lease`, `policy`, `agent`, `identity`, etc.
	// Empty when not applicable.
	AuditTagResourceType = "resource_type"

	// AuditTagOutcome is the canonical string form of [AuditOutcome]
	// — `allowed` or `denied`. Redundant with [Event.Type]
	// (`policy.pass` vs `policy.violation`) for the default mapping
	// but explicitly tagged so producers using custom types (e.g.,
	// `user.command`) still expose the outcome.
	AuditTagOutcome = "outcome"

	// AuditTagReason carries the redacted denial reason when
	// Outcome=denied. Empty on allowed events. Mirrors
	// [secrets.SecretAccessEvent.ErrorReason] in shape — the
	// caller is responsible for keeping it free of cleartext.
	AuditTagReason = "reason"
)

// AuditOutcome is the binary allowed/denied decision every audit
// event carries. Tagged into [Event.Tags] under [AuditTagOutcome]
// AND drives the default [Event.Type] selection (allowed →
// [EventTypePolicyPass]; denied → [EventTypePolicyViolation]) when
// the caller leaves [AuditEventInput.Type] empty.
type AuditOutcome string

const (
	// AuditOutcomeAllowed is the success outcome — the operation
	// completed without policy / capability / validation rejection.
	AuditOutcomeAllowed AuditOutcome = "allowed"

	// AuditOutcomeDenied is the failure outcome — the operation
	// was rejected (policy, capability, validation, backend error,
	// ctx cancellation, etc.). [AuditEventInput.Reason] should
	// carry the redacted summary.
	AuditOutcomeDenied AuditOutcome = "denied"
)

// IsValid reports whether the receiver is one of the two known
// outcome values. Used by [NewAuditEvent] to gate the default
// type selection — unknown outcomes return [ErrInvalidEvent].
func (o AuditOutcome) IsValid() bool {
	return o == AuditOutcomeAllowed || o == AuditOutcomeDenied
}

// AuditEventInput is the producer-facing audit-event shape. Every
// audit emitter constructs one of these and hands it to
// [NewAuditEvent] / [AuditEmitter] — keeps the tag-key + severity
// + type defaulting in one place so producers across domains
// (secrets via the cmd/kscore-server bridge, Epic 12 policy
// engine, future modules) all surface the same shape.
//
// Required fields: Source, Action, Outcome. Actor + Resource are
// strongly recommended but technically optional (system-driven
// audits without a principal, or actions without a target).
//
// Type override: when empty, defaults to [EventTypePolicyPass] /
// [EventTypePolicyViolation] per §4.9's "audit-mode only in v1.0"
// note. Producers with a more specific story (e.g.,
// [EventTypeUserCommand] for human-initiated ops) set Type
// explicitly.
//
// Severity override: when zero ([SeverityUnknown]), defaults to
// [SeverityInfo] (allowed) / [SeverityWarn] (denied). Producers
// emitting a severe denial (e.g., a critical-policy violation)
// should set Severity=[SeverityError] or higher explicitly.
//
// ExtraTags merge: canonical tag keys ([AuditTagActor], etc.)
// always win — if ExtraTags contains a colliding key, the
// canonical value (from the dedicated field) is what reaches the
// stamped Event. ExtraTags fills the rest.
type AuditEventInput struct {
	Type          EventType
	Source        string
	Actor         string
	Action        string
	ResourceType  string
	Resource      string
	Outcome       AuditOutcome
	Reason        string
	Severity      Severity
	CorrelationID string
	ExtraTags     map[string]string
	Data          map[string]any
}

// canonicalAuditTagKeys is the set of tag keys [NewAuditEvent]
// reserves for the dedicated [AuditEventInput] fields. ExtraTags
// entries matching any of these are dropped (canonical wins).
var canonicalAuditTagKeys = map[string]struct{}{
	AuditTagActor:        {},
	AuditTagAction:       {},
	AuditTagResource:     {},
	AuditTagResourceType: {},
	AuditTagOutcome:      {},
	AuditTagReason:       {},
}

// NewAuditEvent constructs an audit-shaped [Event] from input.
// Errors wrap [ErrInvalidEvent]:
//
//   - Source / Action empty.
//   - Outcome not one of [AuditOutcomeAllowed] / [AuditOutcomeDenied].
//   - Type, when explicitly set, fails [ParseEventType] (caller-
//     supplied custom types still go through the same shape +
//     category-allowlist validation as any other [EventType]).
func NewAuditEvent(in AuditEventInput) (Event, error) {
	if in.Source == "" {
		return Event{}, fmt.Errorf("%w: audit event source is required", ErrInvalidEvent)
	}
	if in.Action == "" {
		return Event{}, fmt.Errorf("%w: audit event action is required", ErrInvalidEvent)
	}
	if !in.Outcome.IsValid() {
		return Event{}, fmt.Errorf("%w: audit event outcome %q is not %q or %q", ErrInvalidEvent, in.Outcome, AuditOutcomeAllowed, AuditOutcomeDenied)
	}

	typ := in.Type
	if typ == "" {
		switch in.Outcome {
		case AuditOutcomeAllowed:
			typ = EventTypePolicyPass
		case AuditOutcomeDenied:
			typ = EventTypePolicyViolation
		}
	}
	// Validate explicit Type (default branch's output is already
	// canonical).
	if _, err := ParseEventType(string(typ)); err != nil {
		return Event{}, err
	}

	e, err := NewEvent(typ, in.Source)
	if err != nil {
		return Event{}, err
	}

	// Severity default: info on allowed, warn on denied. Caller
	// override wins when non-zero.
	if in.Severity.IsValid() {
		e.Severity = in.Severity
	} else if in.Outcome == AuditOutcomeDenied {
		e.Severity = SeverityWarn
	}
	// allowed + no override → keep NewEvent's SeverityInfo default.

	e.CorrelationID = in.CorrelationID
	e.Data = in.Data

	// Tags: ExtraTags first (so canonical fields written below
	// overwrite collisions — canonical-wins semantic).
	tags := make(map[string]string, len(in.ExtraTags)+6)
	for k, v := range in.ExtraTags {
		if _, reserved := canonicalAuditTagKeys[k]; reserved {
			continue
		}
		tags[k] = v
	}
	if in.Actor != "" {
		tags[AuditTagActor] = in.Actor
	}
	tags[AuditTagAction] = in.Action
	if in.Resource != "" {
		tags[AuditTagResource] = in.Resource
	}
	if in.ResourceType != "" {
		tags[AuditTagResourceType] = in.ResourceType
	}
	tags[AuditTagOutcome] = string(in.Outcome)
	if in.Reason != "" {
		tags[AuditTagReason] = in.Reason
	}
	e.Tags = tags

	return e, nil
}

// AuditEmitter wraps an [EventPublisher] with audit-event-shaped
// construction. The canonical entry point for any audit producer
// (secrets bridge, Epic 12 policy engine, future modules) so the
// tag-key + type + severity defaulting stays in this package.
//
// Failed publishes are logged at ERROR (matching §4.11's "failure
// to log = bug" framing) AND counted via [FailedPublishes] so
// observability surfaces the gap without losing the in-flight
// operation. Note: the v1.0 [secrets.Auditor.Emit] interface has
// no error return, so the secrets-side bridge can't actually fail
// the in-flight op on a publish error; a v1.x ROADMAP entry
// tracks stronger consistency via an Auditor.Emit signature
// change.
type AuditEmitter struct {
	publisher EventPublisher
	logger    *slog.Logger

	failedPublishes atomic.Int64
}

// NewAuditEmitter builds an emitter around publisher. Returns
// an error when publisher is nil — audit emission with no bus
// destination is a programming bug, not a degraded mode. Nil
// logger falls back to [slog.Default].
func NewAuditEmitter(publisher EventPublisher, logger *slog.Logger) (*AuditEmitter, error) {
	if publisher == nil {
		return nil, errors.New("events: audit emitter requires a non-nil publisher")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditEmitter{publisher: publisher, logger: logger}, nil
}

// Emit constructs an audit event from input and synchronously
// publishes it through the wrapped [EventPublisher]. Returns
// any error from [NewAuditEvent] (validation rejection) or the
// publisher (transport / store failure).
//
// Sync semantics per user-confirmed plan: the caller blocks on
// the NATS ack so an audit event reaches the bus before the
// emitter call returns. Failures are also logged at ERROR with
// every audit-relevant field so log-only operators can recover
// the audit trail even when NATS is down.
func (a *AuditEmitter) Emit(ctx context.Context, in AuditEventInput) error {
	e, err := NewAuditEvent(in)
	if err != nil {
		return err
	}
	if err := a.publisher.Publish(ctx, e); err != nil {
		a.failedPublishes.Add(1)
		a.logger.LogAttrs(ctx, slog.LevelError,
			"events: audit publish failed (compliance gap)",
			slog.String("event_id", e.ID),
			slog.String("event_type", string(e.Type)),
			slog.String("subject", e.Subject),
			slog.String("audit_actor", in.Actor),
			slog.String("audit_action", in.Action),
			slog.String("audit_resource", in.Resource),
			slog.String("audit_outcome", string(in.Outcome)),
			slog.Any("error", err),
		)
		return err
	}
	return nil
}

// EmitAsync constructs an audit event and enqueues it via the
// publisher's async pipeline. Returns immediately on enqueue
// success; failures during the background flush surface via the
// publisher's [JetStreamPublisher.FailedPublishes] counter and
// (when configured) [WithAsyncErrorCallback].
//
// Use sparingly — audit events are usually safer to publish
// synchronously per §4.11. EmitAsync exists for non-critical
// audit-adjacent emissions where the producer cannot tolerate
// the NATS RTT.
func (a *AuditEmitter) EmitAsync(ctx context.Context, in AuditEventInput) error {
	e, err := NewAuditEvent(in)
	if err != nil {
		return err
	}
	return a.publisher.PublishAsync(ctx, e)
}

// FailedPublishes returns the count of sync [Emit] failures since
// process start. Failures during async-emit propagate to the
// publisher's counter instead.
func (a *AuditEmitter) FailedPublishes() int64 {
	return a.failedPublishes.Load()
}
