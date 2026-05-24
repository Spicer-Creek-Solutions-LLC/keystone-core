// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

func sampleEventRecord(id, typ, source string) *EventStoreRecord {
	return &EventStoreRecord{
		ID:            id,
		Type:          typ,
		Source:        source,
		Time:          time.Now().UTC().Truncate(time.Millisecond),
		Severity:      "info",
		CorrelationID: "req-" + id,
		Tags:          map[string]string{"role": "web", "env": "prod"},
		Data:          map[string]any{"latency_ms": 12.5, "host": "h-1"},
		Subject:       "kscore.default.events." + typ,
	}
}

func TestSQLite_CreateEvent_RoundTrip(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleEventRecord("e-1", "agent.connect", "agent-1")
	if err := s.CreateEvent(ctx, in); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	got, err := s.GetEvent(ctx, "e-1")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Type != "agent.connect" || got.Source != "agent-1" {
		t.Errorf("round-trip mismatch: %#v", got)
	}
	if got.Severity != "info" {
		t.Errorf("severity: %q", got.Severity)
	}
	if got.Tags["role"] != "web" || got.Tags["env"] != "prod" {
		t.Errorf("tags lost: %#v", got.Tags)
	}
	if got.Data["host"] != "h-1" {
		t.Errorf("data host lost: %#v", got.Data)
	}
	// JSON number round-trips as float64.
	if v, ok := got.Data["latency_ms"].(float64); !ok || v != 12.5 {
		t.Errorf("data latency: %v (%T)", got.Data["latency_ms"], got.Data["latency_ms"])
	}
	if got.Subject != in.Subject {
		t.Errorf("subject: %q", got.Subject)
	}
	if got.CorrelationID != in.CorrelationID {
		t.Errorf("correlation: %q", got.CorrelationID)
	}
	if got.Time.Unix() != in.Time.Unix() {
		t.Errorf("time: got %v want %v", got.Time, in.Time)
	}
}

func TestSQLite_CreateEvent_Duplicate(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	in := sampleEventRecord("dup", "agent.connect", "agent-1")
	if err := s.CreateEvent(ctx, in); err != nil {
		t.Fatalf("first CreateEvent: %v", err)
	}
	err := s.CreateEvent(ctx, in)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("second CreateEvent err = %v, want ErrDuplicate", err)
	}
}

func TestSQLite_CreateEvent_Validation(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*EventStoreRecord)
		wantIn string
	}{
		{"nil", nil, "nil record"},
		{"empty id", func(r *EventStoreRecord) { r.ID = "" }, "ID is required"},
		{"empty type", func(r *EventStoreRecord) { r.Type = "" }, "Type is required"},
		{"empty source", func(r *EventStoreRecord) { r.Source = "" }, "Source is required"},
		{"zero time", func(r *EventStoreRecord) { r.Time = time.Time{} }, "Time is required"},
		{"empty severity", func(r *EventStoreRecord) { r.Severity = "" }, "Severity is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rec *EventStoreRecord
			if c.mutate != nil {
				rec = sampleEventRecord("v", "agent.connect", "agent-1")
				c.mutate(rec)
			}
			err := s.CreateEvent(ctx, rec)
			if err == nil {
				t.Fatalf("CreateEvent succeeded; want error %q", c.wantIn)
			}
		})
	}
}

func TestSQLite_GetEvent_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	_, err := s.GetEvent(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_CreateEventsBatch_AllOrNothing(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	// Empty slice → no-op.
	if err := s.CreateEventsBatch(ctx, nil); err != nil {
		t.Errorf("empty batch err = %v", err)
	}

	// Happy path: 3 events.
	batch := []*EventStoreRecord{
		sampleEventRecord("b-1", "agent.connect", "agent-1"),
		sampleEventRecord("b-2", "agent.heartbeat", "agent-1"),
		sampleEventRecord("b-3", "agent.disconnect", "agent-1"),
	}
	if err := s.CreateEventsBatch(ctx, batch); err != nil {
		t.Fatalf("batch: %v", err)
	}
	for _, r := range batch {
		if _, err := s.GetEvent(ctx, r.ID); err != nil {
			t.Errorf("GetEvent(%s): %v", r.ID, err)
		}
	}

	// Mid-batch failure (duplicate ID) rolls back the whole batch.
	mixed := []*EventStoreRecord{
		sampleEventRecord("c-1", "agent.connect", "agent-1"),
		sampleEventRecord("b-2", "agent.connect", "agent-1"), // duplicate
		sampleEventRecord("c-3", "agent.connect", "agent-1"),
	}
	err := s.CreateEventsBatch(ctx, mixed)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("mixed batch err = %v, want ErrDuplicate", err)
	}
	// c-1 and c-3 must NOT be in the DB (rollback).
	for _, id := range []string{"c-1", "c-3"} {
		_, err := s.GetEvent(ctx, id)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("after rollback GetEvent(%s) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestSQLite_CreateEventsBatch_ValidatesAllUpFront(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	// First record valid, second invalid (empty source).
	good := sampleEventRecord("v-1", "agent.connect", "agent-1")
	bad := sampleEventRecord("v-2", "agent.connect", "")
	err := s.CreateEventsBatch(ctx, []*EventStoreRecord{good, bad})
	if err == nil {
		t.Fatalf("CreateEventsBatch with invalid succeeded; want error")
	}
	// The valid first record must NOT have been inserted (pre-tx validation).
	_, getErr := s.GetEvent(ctx, "v-1")
	if !errors.Is(getErr, ErrNotFound) {
		t.Errorf("valid record persisted despite batch validation failure: %v", getErr)
	}
}

func TestSQLite_ListEvents_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	// Seed a deterministic spread of events: vary type, severity,
	// source, time, and tags.
	now := time.Now().UTC()
	seed := []*EventStoreRecord{
		{ID: "01", Type: "agent.connect", Source: "agent-1", Time: now.Add(-5 * time.Hour), Severity: "info", Tags: map[string]string{"role": "web"}},
		{ID: "02", Type: "agent.disconnect", Source: "agent-1", Time: now.Add(-4 * time.Hour), Severity: "warn", Tags: map[string]string{"role": "web"}},
		{ID: "03", Type: "agent.heartbeat", Source: "agent-2", Time: now.Add(-3 * time.Hour), Severity: "debug", Tags: map[string]string{"role": "db"}},
		{ID: "04", Type: "job.start", Source: "scheduler", Time: now.Add(-2 * time.Hour), Severity: "info"},
		{ID: "05", Type: "job.fail", Source: "scheduler", Time: now.Add(-1 * time.Hour), Severity: "error", CorrelationID: "req-42"},
		{ID: "06", Type: "system.startup", Source: "server-1", Time: now.Add(-30 * time.Minute), Severity: "critical"},
	}
	for _, r := range seed {
		if err := s.CreateEvent(ctx, r); err != nil {
			t.Fatalf("seed CreateEvent(%s): %v", r.ID, err)
		}
	}

	t.Run("by Type exact", func(t *testing.T) {
		got, err := s.ListEvents(ctx, EventFilter{Type: "agent.connect"})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if len(got) != 1 || got[0].ID != "01" {
			t.Errorf("got %v", eventIDs(got))
		}
	})

	t.Run("by TypePrefix agent", func(t *testing.T) {
		got, err := s.ListEvents(ctx, EventFilter{TypePrefix: "agent."})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if want := []string{"01", "02", "03"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})

	t.Run("by Source", func(t *testing.T) {
		got, _ := s.ListEvents(ctx, EventFilter{Source: "scheduler"})
		if want := []string{"04", "05"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})

	t.Run("by Severities IN", func(t *testing.T) {
		// MinSeverity warn → IN ('warn','error','critical')
		got, _ := s.ListEvents(ctx, EventFilter{
			Severities: []string{"warn", "error", "critical"},
		})
		if want := []string{"02", "05", "06"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})

	t.Run("by Tags exact AND", func(t *testing.T) {
		got, _ := s.ListEvents(ctx, EventFilter{
			Tags: map[string]string{"role": "web"},
		})
		if want := []string{"01", "02"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})

	t.Run("by CorrelationID", func(t *testing.T) {
		got, _ := s.ListEvents(ctx, EventFilter{CorrelationID: "req-42"})
		if len(got) != 1 || got[0].ID != "05" {
			t.Errorf("got %v", eventIDs(got))
		}
	})

	t.Run("by Since (half-open)", func(t *testing.T) {
		got, _ := s.ListEvents(ctx, EventFilter{Since: now.Add(-90 * time.Minute)})
		if want := []string{"05", "06"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})

	t.Run("by Until (half-open)", func(t *testing.T) {
		got, _ := s.ListEvents(ctx, EventFilter{Until: now.Add(-3*time.Hour - time.Minute)})
		if want := []string{"01", "02"}; !equalIDs(got, want) {
			t.Errorf("got %v, want %v", eventIDs(got), want)
		}
	})
}

func TestSQLite_ListEvents_CursorPagination(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i := 1; i <= 7; i++ {
		rec := sampleEventRecord(fmt.Sprintf("p-%02d", i), "agent.connect", "agent-1")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// Page 1: ascending, limit 3.
	page1, err := s.ListEvents(ctx, EventFilter{Limit: 3})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if want := []string{"p-01", "p-02", "p-03"}; !equalIDs(page1, want) {
		t.Errorf("page1: %v, want %v", eventIDs(page1), want)
	}

	// Page 2.
	page2, _ := s.ListEvents(ctx, EventFilter{Cursor: "p-03", Limit: 3})
	if want := []string{"p-04", "p-05", "p-06"}; !equalIDs(page2, want) {
		t.Errorf("page2: %v, want %v", eventIDs(page2), want)
	}

	// Page 3 — last partial page.
	page3, _ := s.ListEvents(ctx, EventFilter{Cursor: "p-06", Limit: 3})
	if want := []string{"p-07"}; !equalIDs(page3, want) {
		t.Errorf("page3: %v, want %v", eventIDs(page3), want)
	}

	// Descending: cursor flips to "id < ?".
	desc1, _ := s.ListEvents(ctx, EventFilter{Limit: 3, Descending: true})
	if want := []string{"p-07", "p-06", "p-05"}; !equalIDs(desc1, want) {
		t.Errorf("desc1: %v, want %v", eventIDs(desc1), want)
	}
	desc2, _ := s.ListEvents(ctx, EventFilter{Cursor: "p-05", Limit: 3, Descending: true})
	if want := []string{"p-04", "p-03", "p-02"}; !equalIDs(desc2, want) {
		t.Errorf("desc2: %v, want %v", eventIDs(desc2), want)
	}
}

func TestSQLite_CountEvents(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i, typ := range []string{"agent.connect", "agent.connect", "job.start"} {
		rec := sampleEventRecord(fmt.Sprintf("c-%d", i), typ, "src")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	all, err := s.CountEvents(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("CountEvents: %v", err)
	}
	if all != 3 {
		t.Errorf("all = %d, want 3", all)
	}

	agents, _ := s.CountEvents(ctx, EventFilter{TypePrefix: "agent."})
	if agents != 2 {
		t.Errorf("agents = %d, want 2", agents)
	}

	// Cursor/Limit are ignored by Count.
	withCursor, _ := s.CountEvents(ctx, EventFilter{Cursor: "c-0", Limit: 1})
	if withCursor != 3 {
		t.Errorf("with cursor = %d, want 3 (Count ignores cursor/limit)", withCursor)
	}
}

func TestSQLite_DeleteEvent(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	if err := s.CreateEvent(ctx, sampleEventRecord("d-1", "agent.connect", "agent-1")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.DeleteEvent(ctx, "d-1"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	_, err := s.GetEvent(ctx, "d-1")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v, want ErrNotFound", err)
	}

	// Delete non-existent → ErrNotFound (idempotent guard).
	if err := s.DeleteEvent(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete ghost err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_ApplyEventsRetention_MaxAge(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	// 5 events spaced 1h apart, oldest at -5h.
	for i := 0; i < 5; i++ {
		rec := sampleEventRecord(fmt.Sprintf("a-%d", i), "agent.connect", "agent-1")
		rec.Time = now.Add(time.Duration(-(5 - i)) * time.Hour)
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// MaxAge 2h30m → events older than that go. Removes a-0 (5h), a-1 (4h), a-2 (3h).
	deleted, err := s.ApplyEventsRetention(ctx, []EventsRetentionPolicy{
		{MaxAge: 2*time.Hour + 30*time.Minute},
	})
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	left, _ := s.CountEvents(ctx, EventFilter{})
	if left != 2 {
		t.Errorf("left = %d, want 2", left)
	}
}

func TestSQLite_ApplyEventsRetention_MaxCount_PerType(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	// 5 agent.connect events + 3 job.start events.
	for i := 0; i < 5; i++ {
		rec := sampleEventRecord(fmt.Sprintf("ac-%d", i), "agent.connect", "agent-1")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		rec := sampleEventRecord(fmt.Sprintf("js-%d", i), "job.start", "scheduler")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// Keep only 2 of each.
	deleted, err := s.ApplyEventsRetention(ctx, []EventsRetentionPolicy{
		{Type: "agent.connect", MaxCount: 2},
		{Type: "job.start", MaxCount: 2},
	})
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	// 5 → 2 (deletes 3) + 3 → 2 (deletes 1) = 4 total.
	if deleted != 4 {
		t.Errorf("deleted = %d, want 4", deleted)
	}

	agents, _ := s.CountEvents(ctx, EventFilter{Type: "agent.connect"})
	if agents != 2 {
		t.Errorf("agent.connect remaining = %d, want 2", agents)
	}
	jobs, _ := s.CountEvents(ctx, EventFilter{Type: "job.start"})
	if jobs != 2 {
		t.Errorf("job.start remaining = %d, want 2", jobs)
	}
}

func TestSQLite_ApplyEventsRetention_MaxCount_Global(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		rec := sampleEventRecord(fmt.Sprintf("g-%d", i), "agent.connect", "agent-1")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// Global policy (Type="") with MaxCount=3 → newest 3 survive.
	deleted, _ := s.ApplyEventsRetention(ctx, []EventsRetentionPolicy{{MaxCount: 3}})
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	left, _ := s.CountEvents(ctx, EventFilter{})
	if left != 3 {
		t.Errorf("left = %d, want 3", left)
	}
}

func TestSQLite_ApplyEventsRetention_Empty(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	// Empty policies + zero-zero policies are no-ops.
	if d, err := s.ApplyEventsRetention(context.Background(), nil); err != nil || d != 0 {
		t.Errorf("empty: deleted=%d err=%v", d, err)
	}
	if d, err := s.ApplyEventsRetention(context.Background(), []EventsRetentionPolicy{{}}); err != nil || d != 0 {
		t.Errorf("zero-zero policy: deleted=%d err=%v", d, err)
	}
}

// eventIDs returns the IDs of the records in order. Named with the
// "event" prefix to avoid colliding with the sibling `ids` helpers in
// other state-package tests.
func eventIDs(recs []*EventStoreRecord) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.ID
	}
	return out
}

// equalIDs checks set-equality between recs' IDs and want; order is
// not asserted (id-order tests assert directly via eventIDs).
func equalIDs(recs []*EventStoreRecord, want []string) bool {
	if len(recs) != len(want) {
		return false
	}
	got := eventIDs(recs)
	// Sort both so we don't accidentally lock in arbitrary ordering;
	// id-order tests use their own assertions where order matters.
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
