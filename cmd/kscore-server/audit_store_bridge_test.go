package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/secrets"
)

type recordingAuditor struct {
	mu      sync.Mutex
	entries []audit.AuditEntry
}

func (r *recordingAuditor) Emit(_ context.Context, e audit.AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *recordingAuditor) get() []audit.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.AuditEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

func TestSecretsAuditStoreBridge_AllowedEmitsLow(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	b := newSecretsAuditStoreBridge(rec)
	b.Emit(context.Background(), secrets.SecretAccessEvent{
		Timestamp: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Action:    "secret.get",
		Path:      "kv/data/app/db",
		Backend:   "vault-default",
		Principal: secrets.Principal{SPIFFEID: "spiffe://k.local/agent/a-1"},
		Allowed:   true,
		Duration:  25 * time.Millisecond,
	})

	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[0]
	if e.Action != "secret.get" {
		t.Errorf("Action = %q", e.Action)
	}
	if e.ResourceType != "secret" {
		t.Errorf("ResourceType = %q", e.ResourceType)
	}
	if !e.Allowed {
		t.Errorf("Allowed false")
	}
	if e.User != "spiffe://k.local/agent/a-1" {
		t.Errorf("User = %q", e.User)
	}
	if e.Severity != audit.SeverityLow {
		t.Errorf("Severity = %v", e.Severity)
	}
	if len(e.Violations) != 0 {
		t.Errorf("Violations non-empty on allowed: %+v", e.Violations)
	}
	if e.Metadata["backend"] != "vault-default" || e.Metadata["path"] != "kv/data/app/db" {
		t.Errorf("metadata = %+v", e.Metadata)
	}
}

func TestSecretsAuditStoreBridge_DeniedEmitsHighWithViolation(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	b := newSecretsAuditStoreBridge(rec)
	b.Emit(context.Background(), secrets.SecretAccessEvent{
		Action:      "secret.write",
		Path:        "kv/data/locked",
		Backend:     "file-default",
		Principal:   secrets.Principal{User: "alice"},
		Allowed:     false,
		ErrorReason: "permission denied",
		Duration:    5 * time.Millisecond,
	})
	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[0]
	if e.Allowed {
		t.Errorf("Allowed true on denied")
	}
	if e.Severity != audit.SeverityHigh {
		t.Errorf("Severity = %v, want High", e.Severity)
	}
	if len(e.Violations) != 1 {
		t.Fatalf("Violations = %d", len(e.Violations))
	}
	if e.Violations[0].Message != "permission denied" {
		t.Errorf("Violation message = %q", e.Violations[0].Message)
	}
	if e.User != "alice" {
		t.Errorf("User = %q", e.User)
	}
}

func TestSecretsAuditStoreBridge_ActorPrecedenceSPIFFE(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action: "secret.get",
		Principal: secrets.Principal{
			SPIFFEID: "spiffe://k.local/agent/a-1",
			AgentID:  "a-1",
			User:     "alice",
		},
		Allowed: true,
	})
	entries := rec.get()
	if len(entries) != 1 || entries[0].User != "spiffe://k.local/agent/a-1" {
		t.Errorf("SPIFFE precedence broken: %+v", entries)
	}
}

func TestSecretsAuditStoreBridge_ActorPrecedenceAgentIDFallback(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action: "secret.get",
		Principal: secrets.Principal{
			AgentID: "a-1",
			User:    "alice",
		},
		Allowed: true,
	})
	entries := rec.get()
	if len(entries) != 1 || entries[0].User != "a-1" {
		t.Errorf("AgentID fallback broken: %+v", entries)
	}
}

func TestSecretsAuditStoreBridge_DeniedWithNoErrorReasonNoViolation(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action:  "secret.get",
		Allowed: false,
		// No ErrorReason set.
	})
	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	if len(entries[0].Violations) != 0 {
		t.Errorf("Violations created without ErrorReason: %+v", entries[0].Violations)
	}
	if entries[0].Severity != audit.SeverityHigh {
		t.Errorf("Severity = %v, want High", entries[0].Severity)
	}
}

func TestSecretsAuditStoreBridge_MetadataDurationOnly(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action:   "secret.get",
		Allowed:  true,
		Duration: 1500 * time.Millisecond,
	})
	e := rec.get()[0]
	if e.Metadata["duration_ns"] != "1500000000" {
		t.Errorf("duration_ns = %q", e.Metadata["duration_ns"])
	}
}

func TestSecretsAuditStoreBridge_NilMetadataOnZeroEvent(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action:  "secret.x",
		Allowed: true,
	})
	e := rec.get()[0]
	if e.Metadata != nil {
		t.Errorf("Metadata non-nil on zero event: %+v", e.Metadata)
	}
}

func TestSecretsAuditStoreBridge_TimestampPreservedFromEvent(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	want := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	newSecretsAuditStoreBridge(rec).Emit(context.Background(), secrets.SecretAccessEvent{
		Action:    "secret.get",
		Timestamp: want,
		Allowed:   true,
	})
	got := rec.get()[0].Timestamp
	if !got.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", got, want)
	}
}
