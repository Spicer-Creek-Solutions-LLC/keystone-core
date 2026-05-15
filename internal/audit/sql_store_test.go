package audit_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/state"
)

func newTestStore(t *testing.T) (audit.AuditStore, state.Store) {
	t.Helper()
	cfg := &state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "audit.db")},
	}
	store, err := state.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return audit.NewSQLAuditStore(store), store
}

func TestSQLAuditStore_Store_RoundTrip(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()

	in := audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID:        "require-labels",
		PolicyName:      "Require Labels",
		PolicyType:      audit.PolicyTypeBuiltin,
		ResourceType:    "secret",
		Allowed:         false,
		Duration:        25 * time.Millisecond,
		Violations:      []audit.Violation{{Rule: "missing-owner", Severity: audit.SeverityHigh, Message: "no owner set"}},
		EnforcementMode: audit.EnforcementModeAudit,
		Severity:        audit.SeverityHigh,
		User:            "spiffe://kscore.local/agent/agent-1",
		Action:          "policy.evaluate",
		Metadata:        map[string]string{"region": "us-east"},
	})
	if err := as.Store(ctx, in); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := as.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PolicyID != in.PolicyID || got.Severity != audit.SeverityHigh {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, got)
	}
	if got.EnforcementMode != audit.EnforcementModeAudit {
		t.Errorf("enforcement_mode lost: %s", got.EnforcementMode)
	}
	if got.PolicyType != audit.PolicyTypeBuiltin {
		t.Errorf("policy_type lost: %s", got.PolicyType)
	}
	if len(got.Violations) != 1 || got.Violations[0].Rule != "missing-owner" {
		t.Errorf("violations lost: %+v", got.Violations)
	}
	if got.Metadata["region"] != "us-east" {
		t.Errorf("metadata lost: %+v", got.Metadata)
	}
}

func TestSQLAuditStore_Store_RejectsInvalid(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	err := as.Store(context.Background(), audit.AuditEntry{})
	if err == nil {
		t.Fatalf("zero entry validated; want error")
	}
	if !errors.Is(err, audit.ErrInvalidAuditEntry) {
		t.Errorf("err = %v; want ErrInvalidAuditEntry", err)
	}
}

func TestSQLAuditStore_StoreBatch_AllOrNothing(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()

	if err := as.StoreBatch(ctx, nil); err != nil {
		t.Errorf("empty batch err = %v", err)
	}

	good := []audit.AuditEntry{
		audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "a"}),
		audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "b"}),
		audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "c"}),
	}
	if err := as.StoreBatch(ctx, good); err != nil {
		t.Fatalf("good batch: %v", err)
	}
	for _, e := range good {
		if _, err := as.Get(ctx, e.ID); err != nil {
			t.Errorf("Get(%s): %v", e.ID, err)
		}
	}

	// Mixed batch with invalid entry: validation fails before any DB call.
	mixed := []audit.AuditEntry{
		audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "a-2"}),
		{}, // zero-value — fails Validate
	}
	if err := as.StoreBatch(ctx, mixed); err == nil {
		t.Errorf("mixed batch succeeded; want validation error")
	}
	count, _ := as.Count(ctx, audit.AuditQuery{Action: "a-2"})
	if count != 0 {
		t.Errorf("action a-2 leaked through pre-tx validation: %d", count)
	}
}

func TestSQLAuditStore_Query_ByMinSeverity(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for _, sev := range []audit.Severity{audit.SeverityLow, audit.SeverityMedium, audit.SeverityHigh, audit.SeverityCritical} {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{
			Action:   "x",
			Severity: sev,
		})
		if err := as.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}
	page, _ := as.Query(ctx, audit.AuditQuery{MinSeverity: audit.SeverityHigh})
	if len(page.Entries) != 2 {
		t.Errorf("high+critical count = %d, want 2", len(page.Entries))
	}
	for _, e := range page.Entries {
		if !e.Severity.AtLeast(audit.SeverityHigh) {
			t.Errorf("below-threshold leaked: %s", e.Severity)
		}
	}
}

func TestSQLAuditStore_Query_DefaultLimit(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		_ = as.Store(ctx, e)
	}
	page, _ := as.Query(ctx, audit.AuditQuery{})
	if len(page.Entries) != 5 {
		t.Errorf("default-limit got %d, want 5", len(page.Entries))
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor non-empty on short page: %q", page.NextCursor)
	}
}

func TestSQLAuditStore_Query_Pagination(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	stored := make([]audit.AuditEntry, 7)
	for i := range stored {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		stored[i] = e
		if err := as.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
		time.Sleep(2 * time.Millisecond) // ensure distinct UUIDv7
	}
	page1, _ := as.Query(ctx, audit.AuditQuery{Limit: 3})
	if len(page1.Entries) != 3 || page1.NextCursor == "" {
		t.Errorf("page1: len=%d cursor=%q", len(page1.Entries), page1.NextCursor)
	}
	page2, _ := as.Query(ctx, audit.AuditQuery{Limit: 3, Cursor: page1.NextCursor})
	if len(page2.Entries) != 3 {
		t.Errorf("page2: %d", len(page2.Entries))
	}
	page3, _ := as.Query(ctx, audit.AuditQuery{Limit: 3, Cursor: page2.NextCursor})
	if len(page3.Entries) != 1 || page3.NextCursor != "" {
		t.Errorf("page3: len=%d cursor=%q", len(page3.Entries), page3.NextCursor)
	}
}

func TestSQLAuditStore_Query_RejectsInvalid(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	_, err := as.Query(context.Background(), audit.AuditQuery{Limit: -1})
	if !errors.Is(err, audit.ErrInvalidAuditEntry) {
		t.Errorf("err = %v; want ErrInvalidAuditEntry", err)
	}
}

func TestSQLAuditStore_Count(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		_ = as.Store(ctx, e)
	}
	all, err := as.Count(ctx, audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if all != 4 {
		t.Errorf("Count = %d, want 4", all)
	}
}

func TestSQLAuditStore_Delete(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
	_ = as.Store(ctx, e)
	if err := as.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := as.Get(ctx, e.ID); err == nil {
		t.Errorf("Get after Delete: nil err")
	}
}

func TestSQLAuditStore_ApplyRetention(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		e.Timestamp = time.Now().UTC().Add(-25 * time.Hour) // override for retention test
		_ = as.Store(ctx, e)
	}
	deleted, err := as.ApplyRetention(ctx, audit.RetentionPolicy{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted != 5 {
		t.Errorf("deleted = %d, want 5", deleted)
	}
}

func TestSQLAuditStore_CloseIsNoop(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	if err := as.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := as.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
