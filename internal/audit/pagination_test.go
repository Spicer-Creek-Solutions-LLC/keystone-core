package audit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

func TestPagination_CursorPastEndReturnsEmpty(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	stored := seedN(t, as, 3)

	page, err := as.Query(ctx, audit.AuditQuery{
		Limit:  10,
		Cursor: stored[len(stored)-1].ID, // past the last
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(page.Entries) != 0 {
		t.Errorf("cursor past end: %d entries, want 0", len(page.Entries))
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor non-empty: %q", page.NextCursor)
	}
}

func TestPagination_EmptySetNoCursor(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	page, err := as.Query(context.Background(), audit.AuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(page.Entries) != 0 {
		t.Errorf("empty set: %d entries", len(page.Entries))
	}
	if page.NextCursor != "" {
		t.Errorf("empty set: NextCursor = %q", page.NextCursor)
	}
}

func TestPagination_LimitEqualsTotal(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 5)
	page, err := as.Query(context.Background(), audit.AuditQuery{Limit: 5})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(page.Entries) != 5 {
		t.Errorf("Limit=total: %d, want 5", len(page.Entries))
	}
	// NOTE: NextCursor is set when len==Limit (full page) — the
	// caller MUST hit Query again to discover the result set is
	// exhausted. This is the standard behavior for cursor APIs
	// where "next page exists" is undecidable without another call.
	if page.NextCursor == "" {
		t.Errorf("full-page Query should set NextCursor; got empty")
	}
	// Following the cursor returns empty.
	page2, _ := as.Query(context.Background(), audit.AuditQuery{Limit: 5, Cursor: page.NextCursor})
	if len(page2.Entries) != 0 || page2.NextCursor != "" {
		t.Errorf("post-final follow-up: %d entries cursor=%q",
			len(page2.Entries), page2.NextCursor)
	}
}

func TestPagination_LimitGreaterThanTotal(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 3)
	page, err := as.Query(context.Background(), audit.AuditQuery{Limit: 100})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(page.Entries) != 3 {
		t.Errorf("got %d, want 3", len(page.Entries))
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor set on short page: %q", page.NextCursor)
	}
}

func TestPagination_DescendingFullWalk(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	stored := seedN(t, as, 7)

	page1, _ := as.Query(context.Background(), audit.AuditQuery{Limit: 3, Descending: true})
	page2, _ := as.Query(context.Background(), audit.AuditQuery{Limit: 3, Descending: true, Cursor: page1.NextCursor})
	page3, _ := as.Query(context.Background(), audit.AuditQuery{Limit: 3, Descending: true, Cursor: page2.NextCursor})

	all := append(append(page1.Entries, page2.Entries...), page3.Entries...)
	if len(all) != 7 {
		t.Fatalf("walk got %d, want 7", len(all))
	}
	// Descending: first entry is the LAST stored.
	if all[0].ID != stored[6].ID {
		t.Errorf("first descending = %s, want %s", all[0].ID, stored[6].ID)
	}
	if all[6].ID != stored[0].ID {
		t.Errorf("last descending = %s, want %s", all[6].ID, stored[0].ID)
	}
	if page3.NextCursor != "" {
		t.Errorf("page3 NextCursor non-empty: %q", page3.NextCursor)
	}
}

func TestPagination_CombinedFilterWithCursor(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	// Seed 8 matching (PolicyID=p-a, User=alice, denied, high) +
	// 4 non-matching (mixed).
	for i := 0; i < 8; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-a", User: "alice", Action: "x",
			Allowed: false, Severity: audit.SeverityHigh,
		})
		_ = as.Store(ctx, e)
		time.Sleep(1 * time.Millisecond)
	}
	// Non-matching: same policy but different user.
	for i := 0; i < 2; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-a", User: "bob", Action: "x",
			Allowed: false, Severity: audit.SeverityHigh,
		}))
	}
	// Non-matching: allowed (so wrong outcome).
	for i := 0; i < 2; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-a", User: "alice", Action: "x",
			Allowed: true, Severity: audit.SeverityHigh,
		}))
	}

	denied := false
	q := audit.AuditQuery{
		PolicyID:    "p-a",
		User:        "alice",
		Allowed:     &denied,
		MinSeverity: audit.SeverityHigh,
		Limit:       3,
	}
	page1, _ := as.Query(ctx, q)
	q.Cursor = page1.NextCursor
	page2, _ := as.Query(ctx, q)
	q.Cursor = page2.NextCursor
	page3, _ := as.Query(ctx, q)

	got := append(append(page1.Entries, page2.Entries...), page3.Entries...)
	if len(got) != 8 {
		t.Fatalf("combined-filter walk got %d, want 8", len(got))
	}
	for _, e := range got {
		if e.PolicyID != "p-a" || e.User != "alice" || e.Allowed || !e.Severity.AtLeast(audit.SeverityHigh) {
			t.Errorf("filter leaked: %+v", e)
		}
	}
	if page3.NextCursor != "" {
		t.Errorf("final page NextCursor: %q", page3.NextCursor)
	}
}

func TestPagination_StableUnderIntermediateDeletes(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	stored := seedN(t, as, 9)

	page1, _ := as.Query(ctx, audit.AuditQuery{Limit: 3})
	if len(page1.Entries) != 3 {
		t.Fatalf("page1: %d", len(page1.Entries))
	}
	// Delete entries from page1 — they're already returned. Pagination
	// keys on UUIDv7 ID, so this should not double-skip page2.
	for _, e := range page1.Entries {
		_ = as.Delete(ctx, e.ID)
	}
	page2, _ := as.Query(ctx, audit.AuditQuery{Limit: 3, Cursor: page1.NextCursor})
	if len(page2.Entries) != 3 {
		t.Errorf("page2 after deletes: %d", len(page2.Entries))
	}
	// Verify page2 entries are exactly stored[3:6] — no skipping.
	for i, e := range page2.Entries {
		if e.ID != stored[3+i].ID {
			t.Errorf("page2[%d] = %s, want %s", i, e.ID, stored[3+i].ID)
		}
	}
}

func TestPagination_TimeRangeFilterWithCursor(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	// Seed 6 entries across two time bands.
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		e.Timestamp = now.Add(-2 * time.Hour).Add(time.Duration(i) * time.Millisecond)
		_ = as.Store(ctx, e)
	}
	for i := 0; i < 3; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		e.Timestamp = now.Add(-30 * time.Minute).Add(time.Duration(i) * time.Millisecond)
		_ = as.Store(ctx, e)
	}

	q := audit.AuditQuery{
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(time.Hour),
		Limit: 2,
	}
	page1, _ := as.Query(ctx, q)
	if len(page1.Entries) != 2 {
		t.Fatalf("page1: %d, want 2", len(page1.Entries))
	}
	q.Cursor = page1.NextCursor
	page2, _ := as.Query(ctx, q)
	if len(page2.Entries) != 1 {
		t.Errorf("page2: %d, want 1", len(page2.Entries))
	}
	if page2.NextCursor != "" {
		t.Errorf("page2 NextCursor non-empty: %q", page2.NextCursor)
	}
	// All returned should have timestamps in the [Since, Until] window.
	all := append(page1.Entries, page2.Entries...)
	for _, e := range all {
		if e.Timestamp.Before(q.Since) || !e.Timestamp.Before(q.Until) {
			t.Errorf("entry outside window: %v", e.Timestamp)
		}
	}
}

func TestPagination_InvalidCursorHandledGracefully(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 3)

	// "not-a-uuid" lex-sorts before any UUIDv7 (which start with
	// a hex digit). So either the page is empty (treats as past-end)
	// or returns full set (treats as before-first). Both are
	// acceptable — the test asserts no panic and no error.
	_, err := as.Query(context.Background(), audit.AuditQuery{
		Limit:  10,
		Cursor: "not-a-uuid",
	})
	if err != nil {
		t.Fatalf("invalid cursor errored: %v", err)
	}
}

func TestPagination_DefaultLimitOnEmptyLimit(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	// Default is 100; seed less than that.
	seedN(t, as, 7)
	page, _ := as.Query(context.Background(), audit.AuditQuery{})
	if len(page.Entries) != 7 {
		t.Errorf("default limit: %d, want 7", len(page.Entries))
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor on short page: %q", page.NextCursor)
	}
}

func TestPagination_ExactlyAtDefaultLimitSetsCursor(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("default-limit sweep is slow")
	}
	as, _ := newTestStore(t)
	// Seed exactly default limit. NextCursor MUST be set because
	// from the store's perspective there might be more.
	seedN(t, as, audit.DefaultQueryLimit)
	page, _ := as.Query(context.Background(), audit.AuditQuery{})
	if len(page.Entries) != audit.DefaultQueryLimit {
		t.Errorf("got %d, want %d", len(page.Entries), audit.DefaultQueryLimit)
	}
	if page.NextCursor == "" {
		t.Errorf("at-limit page must set NextCursor")
	}
}

// _ silences "imported and not used" if a test elsewhere already
// imports strings.
var _ = strings.HasPrefix
