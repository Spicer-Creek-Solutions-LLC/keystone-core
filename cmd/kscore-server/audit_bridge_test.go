// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/secrets"
)

// bridgeTestPublisher is the EventPublisher impl backing the
// AuditEmitter the bridge tests against. Mutex-guarded; lets tests
// inject a publish error.
type bridgeTestPublisher struct {
	mu        sync.Mutex
	published []events.Event
	emitErr   error
}

func (p *bridgeTestPublisher) Start(context.Context) error { return nil }
func (p *bridgeTestPublisher) Publish(_ context.Context, e events.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.emitErr != nil {
		return p.emitErr
	}
	p.published = append(p.published, e)
	return nil
}
func (p *bridgeTestPublisher) PublishAsync(ctx context.Context, e events.Event) error {
	return p.Publish(ctx, e)
}
func (p *bridgeTestPublisher) Stop(context.Context) error { return nil }

func (p *bridgeTestPublisher) snapshot() []events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]events.Event, len(p.published))
	copy(out, p.published)
	return out
}

func newBridge(t *testing.T) (*secretsAuditEventBridge, *bridgeTestPublisher) {
	t.Helper()
	pub := &bridgeTestPublisher{}
	emitter, err := events.NewAuditEmitter(pub, nil)
	if err != nil {
		t.Fatalf("NewAuditEmitter: %v", err)
	}
	return newSecretsAuditEventBridge(emitter, nil), pub
}

// ---- allowed / denied → policy.pass / policy.violation -----------------

func TestBridge_AllowedSecretAccess_PublishesPolicyPass(t *testing.T) {
	t.Parallel()
	bridge, pub := newBridge(t)
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Timestamp: time.Now().UTC(),
		Action:    secrets.ActionGetSecret,
		Path:      "kv/app/db",
		Backend:   "file",
		Principal: secrets.Principal{SPIFFEID: "spiffe://kscore.local/agent/agent-1"},
		Allowed:   true,
		Duration:  12 * time.Millisecond,
	})
	got := pub.snapshot()
	if len(got) != 1 {
		t.Fatalf("publisher saw %d events, want 1", len(got))
	}
	if got[0].Type != events.EventTypePolicyPass {
		t.Errorf("type = %s, want policy.pass", got[0].Type)
	}
	if got[0].Severity != events.SeverityInfo {
		t.Errorf("severity = %s, want info", got[0].Severity)
	}
	if got[0].Tags[events.AuditTagOutcome] != "allowed" {
		t.Errorf("outcome tag = %q", got[0].Tags[events.AuditTagOutcome])
	}
	if got[0].Tags[events.AuditTagResourceType] != "secret" {
		t.Errorf("resource_type tag = %q", got[0].Tags[events.AuditTagResourceType])
	}
	if got[0].Tags[events.AuditTagAction] != "get_secret" {
		t.Errorf("action tag = %q", got[0].Tags[events.AuditTagAction])
	}
	if got[0].Tags[events.AuditTagResource] != "kv/app/db" {
		t.Errorf("resource tag = %q", got[0].Tags[events.AuditTagResource])
	}
	if got[0].Tags["backend"] != "file" {
		t.Errorf("backend extra-tag = %q", got[0].Tags["backend"])
	}
	if got[0].Source != "secrets-broker" {
		t.Errorf("source = %q", got[0].Source)
	}
}

func TestBridge_DeniedSecretAccess_PublishesPolicyViolation(t *testing.T) {
	t.Parallel()
	bridge, pub := newBridge(t)
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Timestamp:   time.Now().UTC(),
		Action:      secrets.ActionGetSecret,
		Path:        "kv/forbidden",
		Backend:     "file",
		Principal:   secrets.Principal{User: "alice"},
		Allowed:     false,
		ErrorReason: "capability refused: missing kv",
	})
	got := pub.snapshot()
	if len(got) != 1 {
		t.Fatalf("publisher saw %d, want 1", len(got))
	}
	if got[0].Type != events.EventTypePolicyViolation {
		t.Errorf("type = %s, want policy.violation", got[0].Type)
	}
	if got[0].Severity != events.SeverityWarn {
		t.Errorf("severity = %s, want warn", got[0].Severity)
	}
	if got[0].Tags[events.AuditTagOutcome] != "denied" {
		t.Errorf("outcome tag = %q", got[0].Tags[events.AuditTagOutcome])
	}
	if got[0].Tags[events.AuditTagReason] != "capability refused: missing kv" {
		t.Errorf("reason tag = %q", got[0].Tags[events.AuditTagReason])
	}
}

// ---- actor extraction precedence ---------------------------------------

func TestActorFromPrincipal_SPIFFEIDWins(t *testing.T) {
	t.Parallel()
	p := secrets.Principal{
		SPIFFEID: "spiffe://kscore.local/agent/agent-1",
		AgentID:  "agent-1",
		User:     "alice",
	}
	if got := actorFromPrincipal(p); got != p.SPIFFEID {
		t.Errorf("actor = %q, want SPIFFEID", got)
	}
}

func TestActorFromPrincipal_AgentIDFallback(t *testing.T) {
	t.Parallel()
	p := secrets.Principal{AgentID: "agent-1", User: "alice"}
	if got := actorFromPrincipal(p); got != "agent-1" {
		t.Errorf("actor = %q, want agent-1", got)
	}
}

func TestActorFromPrincipal_UserFallback(t *testing.T) {
	t.Parallel()
	p := secrets.Principal{User: "alice"}
	if got := actorFromPrincipal(p); got != "alice" {
		t.Errorf("actor = %q, want alice", got)
	}
}

func TestActorFromPrincipal_AllEmpty(t *testing.T) {
	t.Parallel()
	if got := actorFromPrincipal(secrets.Principal{}); got != "" {
		t.Errorf("empty principal → %q, want empty string", got)
	}
}

// ---- resource extraction ----------------------------------------------

func TestResourceFromEvent_PathWins(t *testing.T) {
	t.Parallel()
	e := secrets.SecretAccessEvent{Path: "kv/app/db", LeaseID: "lease-1"}
	if got := resourceFromEvent(e); got != "kv/app/db" {
		t.Errorf("resource = %q, want path", got)
	}
}

func TestResourceFromEvent_LeaseIDFallback(t *testing.T) {
	t.Parallel()
	e := secrets.SecretAccessEvent{LeaseID: "lease-1"}
	if got := resourceFromEvent(e); got != "lease-1" {
		t.Errorf("resource = %q, want lease-1", got)
	}
}

// ---- extra tag merging --------------------------------------------------

func TestBridge_PathPrimary_LeaseIDAsExtraTag(t *testing.T) {
	t.Parallel()
	bridge, pub := newBridge(t)
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Action:    secrets.ActionGetSecret,
		Path:      "kv/dynamic/role-a",
		LeaseID:   "lease-42",
		Backend:   "vault",
		Principal: secrets.Principal{AgentID: "agent-1"},
		Allowed:   true,
	})
	got := pub.snapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot len = %d", len(got))
	}
	if got[0].Tags["lease_id"] != "lease-42" {
		t.Errorf("lease_id extra-tag = %q", got[0].Tags["lease_id"])
	}
	if got[0].Tags[events.AuditTagResource] != "kv/dynamic/role-a" {
		t.Errorf("resource = %q, want path (LeaseID belongs in extra-tag when Path is set)", got[0].Tags[events.AuditTagResource])
	}
}

func TestBridge_LeaseOp_NoExtraLeaseIDTag(t *testing.T) {
	t.Parallel()
	// Lease op: Path empty, LeaseID set → LeaseID is the primary
	// resource. Extra-tag lease_id would be redundant.
	bridge, pub := newBridge(t)
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Action:    secrets.ActionRenewLease,
		LeaseID:   "lease-42",
		Backend:   "vault",
		Principal: secrets.Principal{AgentID: "agent-1"},
		Allowed:   true,
	})
	got := pub.snapshot()[0]
	if got.Tags[events.AuditTagResource] != "lease-42" {
		t.Errorf("resource = %q, want lease-42", got.Tags[events.AuditTagResource])
	}
	if _, has := got.Tags["lease_id"]; has {
		t.Errorf("extra-tag lease_id present when LeaseID is the primary resource: %q", got.Tags["lease_id"])
	}
}

// ---- publish failure handling ------------------------------------------

func TestBridge_PublishFailure_LogsButContinues(t *testing.T) {
	t.Parallel()
	pub := &bridgeTestPublisher{emitErr: errors.New("nats down")}
	emitter, err := events.NewAuditEmitter(pub, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	bridge := newSecretsAuditEventBridge(emitter, nil)
	// Emit has no error return — the broker continues regardless.
	// We assert via behaviour: no panic, snapshot stays empty, and
	// emitter's FailedPublishes counter ticks.
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Action:    secrets.ActionGetSecret,
		Path:      "kv/app/db",
		Principal: secrets.Principal{AgentID: "agent-1"},
		Allowed:   true,
	})
	if got := emitter.FailedPublishes(); got != 1 {
		t.Errorf("FailedPublishes = %d, want 1", got)
	}
	if len(pub.snapshot()) != 0 {
		t.Errorf("publisher recorded event despite error")
	}
}

// ---- data round-trip ---------------------------------------------------

func TestBridge_DataIncludesDurationAndMaskedPayload(t *testing.T) {
	t.Parallel()
	bridge, pub := newBridge(t)
	masked := map[string]any{"password": "***"}
	ts := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Timestamp:     ts,
		Action:        secrets.ActionWriteSecret,
		Path:          "kv/app/db",
		Principal:     secrets.Principal{User: "alice"},
		Allowed:       true,
		Duration:      25 * time.Millisecond,
		MaskedPayload: masked,
	})
	got := pub.snapshot()[0]
	if got.Data["duration_ns"] != int64(25*time.Millisecond) {
		t.Errorf("duration_ns = %v, want %d", got.Data["duration_ns"], int64(25*time.Millisecond))
	}
	if got.Data["masked_payload"] == nil {
		t.Errorf("masked_payload missing")
	}
	if got.Data["secret_op_timestamp"] == nil {
		t.Errorf("secret_op_timestamp missing")
	}
}

// ---- interface conformance ---------------------------------------------

func TestBridge_ImplementsSecretsAuditor(t *testing.T) {
	t.Parallel()
	pub := &bridgeTestPublisher{}
	emitter, _ := events.NewAuditEmitter(pub, nil)
	var a secrets.Auditor = newSecretsAuditEventBridge(emitter, nil)
	_ = a // compile-time check only
}
