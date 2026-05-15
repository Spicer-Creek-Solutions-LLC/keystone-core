package state

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

func sampleAuditRecord(id, action string) *AuditEntryStoreRecord {
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

func TestSQLite_CreateAuditEntry_RoundTrip(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleAuditRecord("a-1", "policy.evaluate")
	if err := s.CreateAuditEntry(ctx, in); err != nil {
		t.Fatalf("CreateAuditEntry: %v", err)
	}
	got, err := s.GetAuditEntry(ctx, "a-1")
	if err != nil {
		t.Fatalf("GetAuditEntry: %v", err)
	}
	if got.PolicyID != "require-labels" || got.Severity != "low" {
		t.Errorf("round-trip mismatch: %#v", got)
	}
	if got.Metadata["region"] != "us-east" {
		t.Errorf("metadata lost: %#v", got.Metadata)
	}
	if got.Timestamp.Unix() != in.Timestamp.Unix() {
		t.Errorf("timestamp: got %v want %v", got.Timestamp, in.Timestamp)
	}
	if !got.Allowed {
		t.Errorf("Allowed lost: %#v", got)
	}
}

func TestSQLite_CreateAuditEntry_Duplicate(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	in := sampleAuditRecord("dup", "x")
	if err := s.CreateAuditEntry(ctx, in); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := s.CreateAuditEntry(ctx, in)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("second err = %v, want ErrDuplicate", err)
	}
}

func TestSQLite_CreateAuditEntry_Validation(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*AuditEntryStoreRecord)
	}{
		{"nil", nil},
		{"empty id", func(r *AuditEntryStoreRecord) { r.ID = "" }},
		{"zero timestamp", func(r *AuditEntryStoreRecord) { r.Timestamp = time.Time{} }},
		{"empty action", func(r *AuditEntryStoreRecord) { r.Action = "" }},
		{"empty severity", func(r *AuditEntryStoreRecord) { r.Severity = "" }},
		{"empty enforcement", func(r *AuditEntryStoreRecord) { r.EnforcementMode = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rec *AuditEntryStoreRecord
			if c.mutate != nil {
				rec = sampleAuditRecord("v", "x")
				c.mutate(rec)
			}
			if err := s.CreateAuditEntry(ctx, rec); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestSQLite_GetAuditEntry_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	_, err := s.GetAuditEntry(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_CreateAuditEntriesBatch_AllOrNothing(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	if err := s.CreateAuditEntriesBatch(ctx, nil); err != nil {
		t.Errorf("empty batch err = %v", err)
	}

	good := []*AuditEntryStoreRecord{
		sampleAuditRecord("b-1", "a"),
		sampleAuditRecord("b-2", "b"),
		sampleAuditRecord("b-3", "c"),
	}
	if err := s.CreateAuditEntriesBatch(ctx, good); err != nil {
		t.Fatalf("good batch: %v", err)
	}
	for _, r := range good {
		if _, err := s.GetAuditEntry(ctx, r.ID); err != nil {
			t.Errorf("GetAuditEntry(%s): %v", r.ID, err)
		}
	}

	// Mid-batch duplicate → rollback.
	mixed := []*AuditEntryStoreRecord{
		sampleAuditRecord("c-1", "x"),
		sampleAuditRecord("b-2", "y"), // duplicate
		sampleAuditRecord("c-3", "z"),
	}
	if err := s.CreateAuditEntriesBatch(ctx, mixed); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
	for _, id := range []string{"c-1", "c-3"} {
		_, err := s.GetAuditEntry(ctx, id)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("after rollback GetAuditEntry(%s) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestSQLite_CreateAuditEntriesBatch_PreTxValidation(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	good := sampleAuditRecord("v-1", "x")
	bad := sampleAuditRecord("v-2", "")
	if err := s.CreateAuditEntriesBatch(ctx, []*AuditEntryStoreRecord{good, bad}); err == nil {
		t.Fatalf("invalid batch succeeded")
	}
	if _, err := s.GetAuditEntry(ctx, "v-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("valid record persisted despite pre-tx validation failure")
	}
}

func TestSQLite_ListAuditEntries_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seed := []*AuditEntryStoreRecord{
		{ID: "01", Timestamp: now.Add(-5 * time.Hour), PolicyID: "pol-a", User: "alice", ResourceType: "secret", Allowed: true, Severity: "low", EnforcementMode: "audit", Action: "get_secret", Violations: []byte("[]")},
		{ID: "02", Timestamp: now.Add(-4 * time.Hour), PolicyID: "pol-a", User: "alice", ResourceType: "secret", Allowed: false, Severity: "high", EnforcementMode: "audit", Action: "get_secret", Violations: []byte("[]")},
		{ID: "03", Timestamp: now.Add(-3 * time.Hour), PolicyID: "pol-b", User: "bob", ResourceType: "lease", Allowed: true, Severity: "medium", EnforcementMode: "audit", Action: "renew", Violations: []byte("[]")},
		{ID: "04", Timestamp: now.Add(-2 * time.Hour), PolicyID: "pol-b", User: "bob", ResourceType: "policy", Allowed: false, Severity: "critical", EnforcementMode: "enforce", Action: "evaluate", Violations: []byte("[]")},
		{ID: "05", Timestamp: now.Add(-1 * time.Hour), PolicyID: "", User: "alice", ResourceType: "secret", Allowed: true, Severity: "low", EnforcementMode: "audit", Action: "write_secret", Violations: []byte("[]")},
	}
	for _, r := range seed {
		if err := s.CreateAuditEntry(ctx, r); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	t.Run("by PolicyID", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{PolicyID: "pol-a"})
		if want := []string{"01", "02"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by User", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{User: "alice"})
		if want := []string{"01", "02", "05"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by ResourceType", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{ResourceType: "secret"})
		if want := []string{"01", "02", "05"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Action", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Action: "get_secret"})
		if want := []string{"01", "02"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Severities IN (high+critical)", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Severities: []string{"high", "critical"}})
		if want := []string{"02", "04"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Allowed=false", func(t *testing.T) {
		denied := false
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Allowed: &denied})
		if want := []string{"02", "04"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Allowed=true", func(t *testing.T) {
		allowed := true
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Allowed: &allowed})
		if want := []string{"01", "03", "05"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Since (half-open)", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Since: now.Add(-150 * time.Minute)})
		if want := []string{"04", "05"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
	t.Run("by Until (half-open)", func(t *testing.T) {
		got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Until: now.Add(-3*time.Hour - time.Minute)})
		if want := []string{"01", "02"}; !equalAuditIDs(got, want) {
			t.Errorf("got %v, want %v", auditIDsOf(got), want)
		}
	})
}

func TestSQLite_ListAuditEntries_CursorPagination(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i := 1; i <= 7; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("p-%02d", i), "x")
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	page1, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Limit: 3})
	if want := []string{"p-01", "p-02", "p-03"}; !equalAuditIDs(page1, want) {
		t.Errorf("page1: %v", auditIDsOf(page1))
	}
	page2, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Cursor: "p-03", Limit: 3})
	if want := []string{"p-04", "p-05", "p-06"}; !equalAuditIDs(page2, want) {
		t.Errorf("page2: %v", auditIDsOf(page2))
	}
	page3, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Cursor: "p-06", Limit: 3})
	if want := []string{"p-07"}; !equalAuditIDs(page3, want) {
		t.Errorf("page3: %v", auditIDsOf(page3))
	}

	desc, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Limit: 3, Descending: true})
	if want := []string{"p-07", "p-06", "p-05"}; !equalAuditIDs(desc, want) {
		t.Errorf("desc: %v", auditIDsOf(desc))
	}
}

func TestSQLite_CountAuditEntries(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("c-%d", i), "x")
		rec.User = "alice"
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := sampleAuditRecord("c-bob", "x")
	rec.User = "bob"
	_ = s.CreateAuditEntry(ctx, rec)

	all, _ := s.CountAuditEntries(ctx, AuditEntryFilter{})
	if all != 4 {
		t.Errorf("all = %d, want 4", all)
	}
	alice, _ := s.CountAuditEntries(ctx, AuditEntryFilter{User: "alice"})
	if alice != 3 {
		t.Errorf("alice = %d, want 3", alice)
	}
	// Cursor / Limit ignored.
	withCursor, _ := s.CountAuditEntries(ctx, AuditEntryFilter{Cursor: "c-0", Limit: 1})
	if withCursor != 4 {
		t.Errorf("with cursor = %d, want 4", withCursor)
	}
}

func TestSQLite_DeleteAuditEntry(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	_ = s.CreateAuditEntry(ctx, sampleAuditRecord("d-1", "x"))
	if err := s.DeleteAuditEntry(ctx, "d-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetAuditEntry(ctx, "d-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v", err)
	}
	if err := s.DeleteAuditEntry(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete ghost err = %v", err)
	}
}

func TestSQLite_ApplyAuditRetention_MaxAge(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("a-%d", i), "x")
		rec.Timestamp = now.Add(time.Duration(-(5 - i)) * time.Hour)
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// MaxAge 2h30m → deletes a-0 (5h), a-1 (4h), a-2 (3h).
	deleted, err := s.ApplyAuditRetention(ctx, AuditRetentionPolicy{MaxAge: 2*time.Hour + 30*time.Minute})
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
}

func TestSQLite_ApplyAuditRetention_MaxCount(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("g-%d", i), "x")
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	deleted, _ := s.ApplyAuditRetention(ctx, AuditRetentionPolicy{MaxCount: 3})
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	left, _ := s.CountAuditEntries(ctx, AuditEntryFilter{})
	if left != 3 {
		t.Errorf("left = %d, want 3", left)
	}
}

func TestSQLite_ApplyAuditRetention_MinSeverityExempt(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	// 5 old low-severity + 5 old critical-severity entries.
	for i := 0; i < 5; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("low-%d", i), "x")
		rec.Timestamp = now.Add(-25 * time.Hour)
		rec.Severity = "low"
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed low: %v", err)
		}
	}
	for i := 0; i < 5; i++ {
		rec := sampleAuditRecord(fmt.Sprintf("crit-%d", i), "x")
		rec.Timestamp = now.Add(-25 * time.Hour)
		rec.Severity = "critical"
		if err := s.CreateAuditEntry(ctx, rec); err != nil {
			t.Fatalf("seed crit: %v", err)
		}
	}

	// MaxAge=24h with MinSeverity=critical → only critical entries exempt.
	// Wait — MinSeverity=high means high+critical exempt. Let me use
	// MinSeverity=high to exempt the 5 critical (and any high).
	deleted, _ := s.ApplyAuditRetention(ctx, AuditRetentionPolicy{
		MaxAge:      24 * time.Hour,
		MinSeverity: "high",
	})
	if deleted != 5 {
		t.Errorf("deleted = %d, want 5 (low entries only; critical exempt)", deleted)
	}

	left, _ := s.CountAuditEntries(ctx, AuditEntryFilter{})
	if left != 5 {
		t.Errorf("left = %d, want 5 (only critical)", left)
	}
	got, _ := s.ListAuditEntries(ctx, AuditEntryFilter{Severities: []string{"critical"}})
	if len(got) != 5 {
		t.Errorf("expected 5 critical survivors, got %d", len(got))
	}
}

func TestSQLite_ApplyAuditRetention_ZeroZeroNoOp(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if d, err := s.ApplyAuditRetention(context.Background(), AuditRetentionPolicy{}); err != nil || d != 0 {
		t.Errorf("zero-zero: deleted=%d err=%v", d, err)
	}
}

// auditIDsOf + equalAuditIDs are like the events tests' eventIDs +
// equalIDs (separate names to avoid the existing-helper collision
// in the state package).
func auditIDsOf(recs []*AuditEntryStoreRecord) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.ID
	}
	return out
}

func equalAuditIDs(recs []*AuditEntryStoreRecord, want []string) bool {
	if len(recs) != len(want) {
		return false
	}
	got := auditIDsOf(recs)
	sort.Strings(got)
	w := append([]string{}, want...)
	sort.Strings(w)
	for i := range got {
		if got[i] != w[i] {
			return false
		}
	}
	return true
}
