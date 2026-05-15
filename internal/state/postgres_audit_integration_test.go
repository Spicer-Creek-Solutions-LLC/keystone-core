//go:build integration

package state

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func samplePgAuditRecord(id, action string) *AuditEntryStoreRecord {
	return &AuditEntryStoreRecord{
		ID:              id,
		Timestamp:       time.Now().UTC().Truncate(time.Millisecond),
		PolicyID:        "require-labels",
		PolicyName:      "Require Labels",
		PolicyType:      "builtin",
		ResourceType:    "secret",
		Allowed:         true,
		DurationNS:      12_500_000,
		Violations:      []byte("[]"),
		EnforcementMode: "audit",
		Severity:        "low",
		User:            "spiffe://kscore.local/agent/agent-1",
		Action:          action,
		Metadata:        map[string]string{"region": "us-east"},
	}
}

func TestPg_CreateAuditEntry_RoundTrip(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	in := samplePgAuditRecord("pg-a-1", "policy.evaluate")
	if err := s.CreateAuditEntry(ctx, in); err != nil {
		t.Fatalf("CreateAuditEntry: %v", err)
	}
	got, err := s.GetAuditEntry(ctx, "pg-a-1")
	if err != nil {
		t.Fatalf("GetAuditEntry: %v", err)
	}
	if got.PolicyID != "require-labels" {
		t.Errorf("policy_id: %q", got.PolicyID)
	}
	if got.Metadata["region"] != "us-east" {
		t.Errorf("metadata lost: %#v", got.Metadata)
	}
}

func TestPg_CreateAuditEntry_Duplicate(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	in := samplePgAuditRecord("pg-dup", "x")
	if err := s.CreateAuditEntry(ctx, in); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.CreateAuditEntry(ctx, in); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestPg_CreateAuditEntriesBatch(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	batch := []*AuditEntryStoreRecord{
		samplePgAuditRecord("pg-b-1", "x"),
		samplePgAuditRecord("pg-b-2", "y"),
		samplePgAuditRecord("pg-b-3", "z"),
	}
	if err := s.CreateAuditEntriesBatch(ctx, batch); err != nil {
		t.Fatalf("batch: %v", err)
	}
	for _, r := range batch {
		if _, err := s.GetAuditEntry(ctx, r.ID); err != nil {
			t.Errorf("GetAuditEntry(%s): %v", r.ID, err)
		}
	}

	dupBatch := []*AuditEntryStoreRecord{
		samplePgAuditRecord("pg-c-1", "x"),
		samplePgAuditRecord("pg-b-2", "y"), // dup
	}
	if err := s.CreateAuditEntriesBatch(ctx, dupBatch); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
	if _, err := s.GetAuditEntry(ctx, "pg-c-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pg-c-1 persisted after rollback")
	}
}

func TestPg_ListAuditEntries_Filters(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	now := time.Now().UTC()

	seed := []*AuditEntryStoreRecord{
		{ID: "pg-l-01", Timestamp: now.Add(-3 * time.Hour), PolicyID: "p-a", User: "alice", ResourceType: "secret", Allowed: true, Severity: "low", EnforcementMode: "audit", Action: "get", Violations: []byte("[]")},
		{ID: "pg-l-02", Timestamp: now.Add(-2 * time.Hour), PolicyID: "p-b", User: "bob", ResourceType: "lease", Allowed: false, Severity: "high", EnforcementMode: "enforce", Action: "renew", Violations: []byte("[]")},
		{ID: "pg-l-03", Timestamp: now.Add(-1 * time.Hour), PolicyID: "p-b", User: "alice", ResourceType: "policy", Allowed: true, Severity: "critical", EnforcementMode: "audit", Action: "evaluate", Violations: []byte("[]")},
	}
	for _, r := range seed {
		if err := s.CreateAuditEntry(ctx, r); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{User: "alice"})
	if len(got) != 2 {
		t.Errorf("user=alice: %d, want 2", len(got))
	}

	denied := false
	got, _ = s.ListAuditEntries(ctx, AuditEntryFilter{Allowed: &denied})
	if len(got) != 1 || got[0].ID != "pg-l-02" {
		t.Errorf("denied: %v", got)
	}

	got, _ = s.ListAuditEntries(ctx, AuditEntryFilter{Severities: []string{"high", "critical"}})
	if len(got) != 2 {
		t.Errorf("sev IN (high, critical): %d, want 2", len(got))
	}
}

func TestPg_ApplyAuditRetention_MinSeverityExempt(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		rec := samplePgAuditRecord(fmt.Sprintf("pg-low-%d", i), "x")
		rec.Timestamp = now.Add(-25 * time.Hour)
		rec.Severity = "low"
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		rec := samplePgAuditRecord(fmt.Sprintf("pg-crit-%d", i), "x")
		rec.Timestamp = now.Add(-25 * time.Hour)
		rec.Severity = "critical"
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	deleted, _ := s.ApplyAuditRetention(ctx, AuditRetentionPolicy{
		MaxAge:      24 * time.Hour,
		MinSeverity: "high",
	})
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3 (low only; critical exempt)", deleted)
	}
}

func TestPg_DeleteAuditEntry(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	_ = s.CreateAuditEntry(ctx, samplePgAuditRecord("pg-d-1", "x"))
	if err := s.DeleteAuditEntry(ctx, "pg-d-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetAuditEntry(ctx, "pg-d-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v", err)
	}
}
