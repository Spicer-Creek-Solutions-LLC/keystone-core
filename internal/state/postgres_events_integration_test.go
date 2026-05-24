// SPDX-License-Identifier: Apache-2.0

//go:build integration

package state

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func samplePgEventRecord(id, typ, source string) *EventStoreRecord {
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

func TestPg_CreateEvent_RoundTrip(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	in := samplePgEventRecord("pg-e-1", "agent.connect", "agent-1")
	if err := s.CreateEvent(ctx, in); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	got, err := s.GetEvent(ctx, "pg-e-1")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Type != "agent.connect" {
		t.Errorf("type: %q", got.Type)
	}
	if got.Tags["role"] != "web" {
		t.Errorf("tags lost: %#v", got.Tags)
	}
	if got.Data["host"] != "h-1" {
		t.Errorf("data lost: %#v", got.Data)
	}
	if v, ok := got.Data["latency_ms"].(float64); !ok || v != 12.5 {
		t.Errorf("latency: %v", got.Data["latency_ms"])
	}
}

func TestPg_CreateEvent_Duplicate(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	in := samplePgEventRecord("pg-dup", "agent.connect", "agent-1")
	if err := s.CreateEvent(ctx, in); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.CreateEvent(ctx, in); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestPg_CreateEventsBatch(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	batch := []*EventStoreRecord{
		samplePgEventRecord("pg-b-1", "agent.connect", "agent-1"),
		samplePgEventRecord("pg-b-2", "agent.heartbeat", "agent-1"),
		samplePgEventRecord("pg-b-3", "agent.disconnect", "agent-1"),
	}
	if err := s.CreateEventsBatch(ctx, batch); err != nil {
		t.Fatalf("batch: %v", err)
	}
	for _, r := range batch {
		if _, err := s.GetEvent(ctx, r.ID); err != nil {
			t.Errorf("GetEvent(%s): %v", r.ID, err)
		}
	}

	// Duplicate-mid-batch → atomic rollback.
	dupBatch := []*EventStoreRecord{
		samplePgEventRecord("pg-c-1", "agent.connect", "agent-1"),
		samplePgEventRecord("pg-b-2", "agent.connect", "agent-1"), // duplicate
	}
	if err := s.CreateEventsBatch(ctx, dupBatch); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
	if _, err := s.GetEvent(ctx, "pg-c-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pg-c-1 persisted after rollback")
	}
}

func TestPg_ListEvents_Filters(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	now := time.Now().UTC()
	seed := []*EventStoreRecord{
		{ID: "pg-l-01", Type: "agent.connect", Source: "agent-1", Time: now.Add(-3 * time.Hour), Severity: "info", Tags: map[string]string{"role": "web"}},
		{ID: "pg-l-02", Type: "agent.heartbeat", Source: "agent-1", Time: now.Add(-2 * time.Hour), Severity: "warn", Tags: map[string]string{"role": "db"}},
		{ID: "pg-l-03", Type: "job.fail", Source: "scheduler", Time: now.Add(-1 * time.Hour), Severity: "error", CorrelationID: "req-pg-42"},
	}
	for _, r := range seed {
		if err := s.CreateEvent(ctx, r); err != nil {
			t.Fatalf("seed(%s): %v", r.ID, err)
		}
	}

	got, _ := s.ListEvents(ctx, EventFilter{TypePrefix: "agent."})
	if len(got) != 2 {
		t.Errorf("typeprefix agent.: got %d, want 2", len(got))
	}

	got, _ = s.ListEvents(ctx, EventFilter{Severities: []string{"warn", "error", "critical"}})
	if len(got) != 2 {
		t.Errorf("severities >= warn: got %d, want 2", len(got))
	}

	got, _ = s.ListEvents(ctx, EventFilter{Tags: map[string]string{"role": "web"}})
	if len(got) != 1 || got[0].ID != "pg-l-01" {
		t.Errorf("tag role=web: %v", got)
	}

	got, _ = s.ListEvents(ctx, EventFilter{CorrelationID: "req-pg-42"})
	if len(got) != 1 || got[0].ID != "pg-l-03" {
		t.Errorf("correlation: %v", got)
	}

	// Half-open time range: [now-2h30m, now-30m) → pg-l-02 only.
	got, _ = s.ListEvents(ctx, EventFilter{
		Since: now.Add(-2*time.Hour - 30*time.Minute),
		Until: now.Add(-30 * time.Minute),
	})
	if len(got) != 2 { // pg-l-02 + pg-l-03 (both within window)
		t.Errorf("time range: got %d, want 2", len(got))
	}
}

func TestPg_ListEvents_CursorAndOrder(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	for i := 1; i <= 5; i++ {
		rec := samplePgEventRecord(fmt.Sprintf("pg-p-%02d", i), "agent.connect", "agent-1")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	page1, _ := s.ListEvents(ctx, EventFilter{Limit: 2})
	if len(page1) != 2 || page1[0].ID != "pg-p-01" || page1[1].ID != "pg-p-02" {
		t.Errorf("page1: %v", page1)
	}
	page2, _ := s.ListEvents(ctx, EventFilter{Cursor: "pg-p-02", Limit: 2})
	if len(page2) != 2 || page2[0].ID != "pg-p-03" {
		t.Errorf("page2: %v", page2)
	}

	// Descending order.
	desc, _ := s.ListEvents(ctx, EventFilter{Limit: 2, Descending: true})
	if len(desc) != 2 || desc[0].ID != "pg-p-05" {
		t.Errorf("desc: %v", desc)
	}
}

func TestPg_DeleteEvent(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateEvent(ctx, samplePgEventRecord("pg-d-1", "agent.connect", "agent-1")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.DeleteEvent(ctx, "pg-d-1"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, err := s.GetEvent(ctx, "pg-d-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v", err)
	}
	if err := s.DeleteEvent(ctx, "pg-ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete ghost err = %v", err)
	}
}

func TestPg_ApplyRetention_MaxAge(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	now := time.Now().UTC()
	// Phase C1 finding: the original loop placed an event at exactly
	// `now - MaxAge`, which races against the boundary -- by the time
	// ApplyEventsRetention reads its own time.Now(), the cutoff has
	// drifted forward by milliseconds and the boundary event lands
	// strictly older than the cutoff, getting deleted. Use clearly
	// older / younger offsets so the assertion is timing-stable.
	ages := []time.Duration{
		-4 * time.Hour,    // delete: older than 2h
		-3 * time.Hour,    // delete: older than 2h
		-1 * time.Hour,    // keep:   younger than 2h
		-30 * time.Minute, // keep:   younger than 2h
	}
	for i, age := range ages {
		rec := samplePgEventRecord(fmt.Sprintf("pg-a-%d", i), "agent.connect", "agent-1")
		rec.Time = now.Add(age)
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	deleted, err := s.ApplyEventsRetention(ctx, []EventsRetentionPolicy{{MaxAge: 2 * time.Hour}})
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
}

func TestPg_ApplyRetention_MaxCount(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	for i := 0; i < 5; i++ {
		rec := samplePgEventRecord(fmt.Sprintf("pg-mc-%d", i), "agent.connect", "agent-1")
		if err := s.CreateEvent(ctx, rec); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	deleted, _ := s.ApplyEventsRetention(ctx, []EventsRetentionPolicy{{Type: "agent.connect", MaxCount: 2}})
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
}
