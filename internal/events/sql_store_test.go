// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// newTestStore opens a fresh SQLite-backed events store keyed on a
// per-test temp file. Each test gets isolated data.
func newTestStore(t *testing.T) (EventStore, state.Store) {
	t.Helper()
	cfg := &state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "events.db")},
	}
	store, err := state.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewSQLEventStore(store), store
}

func TestSQLEventStore_Store_RoundTrip(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	in := MustNewEvent(EventTypeAgentConnect, "agent-1")
	in.Tags = map[string]string{"role": "web"}
	in.Data = map[string]any{"latency_ms": 12.5}
	if _, err := in.StampSubject("default"); err != nil {
		t.Fatalf("StampSubject: %v", err)
	}

	if err := es.Store(ctx, in); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := es.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Type != in.Type || got.Source != in.Source {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, in)
	}
	if got.Severity != SeverityInfo {
		t.Errorf("severity round-trip: %s", got.Severity)
	}
	if got.Subject != in.Subject {
		t.Errorf("subject: %q vs %q", got.Subject, in.Subject)
	}
	if got.Tags["role"] != "web" {
		t.Errorf("tags lost: %+v", got.Tags)
	}
	if v, ok := got.Data["latency_ms"].(float64); !ok || v != 12.5 {
		t.Errorf("data lost: %v", got.Data["latency_ms"])
	}
}

func TestSQLEventStore_Store_RejectsInvalid(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)

	// Manually-constructed invalid event (Validate would reject).
	bad := Event{} // empty everywhere
	err := es.Store(context.Background(), bad)
	if err == nil {
		t.Fatalf("Store(zero) succeeded; want validation error")
	}
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
	}
}

func TestSQLEventStore_StoreBatch_AllOrNothing(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	// Empty batch → no-op.
	if err := es.StoreBatch(ctx, nil); err != nil {
		t.Errorf("empty batch err = %v", err)
	}

	// Happy: 3 events all persist.
	good := []Event{
		MustNewEvent(EventTypeAgentConnect, "a-1"),
		MustNewEvent(EventTypeAgentHeartbeat, "a-1"),
		MustNewEvent(EventTypeAgentDisconnect, "a-1"),
	}
	if err := es.StoreBatch(ctx, good); err != nil {
		t.Fatalf("good batch: %v", err)
	}
	for _, e := range good {
		if _, err := es.Get(ctx, e.ID); err != nil {
			t.Errorf("Get(%s): %v", e.ID, err)
		}
	}

	// Mid-batch validation failure aborts BEFORE any DB call.
	mixed := []Event{
		MustNewEvent(EventTypeAgentConnect, "a-2"),
		{}, // zero-value — fails Validate
	}
	if err := es.StoreBatch(ctx, mixed); err == nil {
		t.Errorf("mixed batch succeeded; want validation error")
	}
	// The valid first event must not have leaked through.
	count, _ := es.Count(ctx, EventQuery{Source: "a-2"})
	if count != 0 {
		t.Errorf("source a-2 count = %d, want 0 (pre-tx validation)", count)
	}
}

func TestSQLEventStore_Query_ByCategory(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	// Mix of agent + job events.
	for _, typ := range []EventType{
		EventTypeAgentConnect, EventTypeAgentHeartbeat, EventTypeAgentError,
		EventTypeJobStart, EventTypeJobComplete,
	} {
		e := MustNewEvent(typ, "src")
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	page, err := es.Query(ctx, EventQuery{Category: CategoryAgent})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 3 {
		t.Errorf("agent count = %d, want 3", len(page.Events))
	}
	for _, e := range page.Events {
		if e.Type.Category() != CategoryAgent {
			t.Errorf("non-agent event in agent fan-in: %s", e.Type)
		}
	}
}

func TestSQLEventStore_Query_ByMinSeverity(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	severities := []Severity{SeverityDebug, SeverityInfo, SeverityWarn, SeverityError, SeverityCritical}
	for _, sev := range severities {
		e := MustNewEvent(EventTypeAgentConnect, "src")
		e.Severity = sev
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	page, _ := es.Query(ctx, EventQuery{MinSeverity: SeverityWarn})
	if len(page.Events) != 3 {
		t.Errorf("warn-and-above count = %d, want 3", len(page.Events))
	}
	for _, e := range page.Events {
		if !e.Severity.AtLeast(SeverityWarn) {
			t.Errorf("event below threshold leaked: %s", e.Severity)
		}
	}
}

func TestSQLEventStore_Query_Pagination(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	// 7 events.
	stored := make([]Event, 7)
	for i := range stored {
		e := MustNewEvent(EventTypeAgentConnect, fmt.Sprintf("src-%d", i))
		stored[i] = e
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
		// Stagger so UUIDv7 stamps distinct IDs.
		time.Sleep(2 * time.Millisecond)
	}

	page1, err := es.Query(ctx, EventQuery{Limit: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page1.Events) != 3 {
		t.Errorf("page1 len = %d", len(page1.Events))
	}
	if page1.NextCursor == "" {
		t.Errorf("page1 NextCursor empty; expected continuation")
	}

	page2, _ := es.Query(ctx, EventQuery{Limit: 3, Cursor: page1.NextCursor})
	if len(page2.Events) != 3 {
		t.Errorf("page2 len = %d", len(page2.Events))
	}

	page3, _ := es.Query(ctx, EventQuery{Limit: 3, Cursor: page2.NextCursor})
	if len(page3.Events) != 1 {
		t.Errorf("page3 len = %d, want 1", len(page3.Events))
	}
	if page3.NextCursor != "" {
		t.Errorf("page3 NextCursor = %q, want empty (short page)", page3.NextCursor)
	}
}

func TestSQLEventStore_Query_DefaultLimit(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	// Stamp ≤ DefaultQueryLimit so we don't have to insert 100+ rows
	// to verify the default is applied. The pagination test above
	// covers the limit behaviour itself; here we just confirm
	// Limit=0 → no error and returns everything (because total <
	// DefaultQueryLimit).
	for i := 0; i < 5; i++ {
		e := MustNewEvent(EventTypeAgentConnect, fmt.Sprintf("d-%d", i))
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}
	page, _ := es.Query(ctx, EventQuery{})
	if len(page.Events) != 5 {
		t.Errorf("default-limit got %d, want 5", len(page.Events))
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor non-empty on short page: %q", page.NextCursor)
	}
}

func TestSQLEventStore_Query_RejectsInvalidFilter(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	_, err := es.Query(context.Background(), EventQuery{Type: EventTypeAgentConnect, Category: CategoryAgent})
	if !errors.Is(err, ErrInvalidFilter) {
		t.Errorf("err = %v; want ErrInvalidFilter", err)
	}
}

func TestSQLEventStore_Count(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		typ := EventTypeAgentConnect
		if i%2 == 0 {
			typ = EventTypeJobStart
		}
		e := MustNewEvent(typ, "src")
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}
	all, err := es.Count(ctx, EventQuery{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if all != 4 {
		t.Errorf("Count(all) = %d, want 4", all)
	}
	jobs, _ := es.Count(ctx, EventQuery{Category: CategoryJob})
	if jobs != 2 {
		t.Errorf("Count(job category) = %d, want 2", jobs)
	}

	if _, err := es.Count(ctx, EventQuery{Limit: -1}); !errors.Is(err, ErrInvalidFilter) {
		t.Errorf("Count rejects invalid: %v", err)
	}
}

func TestSQLEventStore_Delete(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()
	e := MustNewEvent(EventTypeAgentConnect, "src")
	if err := es.Store(ctx, e); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := es.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// state.ErrNotFound flows through.
	if _, err := es.Get(ctx, e.ID); err == nil {
		t.Errorf("Get after Delete: nil err")
	}
}

func TestSQLEventStore_ApplyRetention(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	ctx := context.Background()

	// 4 events, 2 agent + 2 job. Keep newest 1 of each type.
	for _, typ := range []EventType{
		EventTypeAgentConnect, EventTypeAgentConnect,
		EventTypeJobStart, EventTypeJobStart,
	} {
		e := MustNewEvent(typ, "src")
		if err := es.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Empty + zero-zero policies are no-ops.
	if n, err := es.ApplyRetention(ctx, nil); err != nil || n != 0 {
		t.Errorf("empty: n=%d err=%v", n, err)
	}

	deleted, err := es.ApplyRetention(ctx, []RetentionPolicy{
		{Type: EventTypeAgentConnect, MaxCount: 1},
		{Type: EventTypeJobStart, MaxCount: 1},
	})
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	left, _ := es.Count(ctx, EventQuery{})
	if left != 2 {
		t.Errorf("left = %d, want 2", left)
	}
}

func TestSQLEventStore_CloseIsNoop(t *testing.T) {
	t.Parallel()
	es, _ := newTestStore(t)
	if err := es.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Second close still a no-op.
	if err := es.Close(); err != nil {
		t.Errorf("Close second: %v", err)
	}
}
