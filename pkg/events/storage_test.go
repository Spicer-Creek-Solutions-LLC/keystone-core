package events

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestNewSQLiteEventStore(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, err := NewSQLiteEventStore(config)
	if err != nil {
		t.Fatalf("NewSQLiteEventStore failed: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Error("Expected database to be initialized")
	}
}

func TestEventStore_Store(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "test").
		Data("key", "value").
		Build()

	err := store.Store(ctx, event)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.Get(ctx, event.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != event.ID {
		t.Errorf("Expected ID=%s, got %s", event.ID, retrieved.ID)
	}

	if retrieved.Type != event.Type {
		t.Errorf("Expected Type=%s, got %s", event.Type, retrieved.Type)
	}

	if retrieved.Tags["env"] != "test" {
		t.Error("Tags not preserved")
	}

	if retrieved.Data["key"] != "value" {
		t.Error("Data not preserved")
	}
}

func TestEventStore_StoreBatch(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	err := store.StoreBatch(ctx, events)
	if err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	// Verify count
	query := NewEventQuery()
	count, err := store.Count(ctx, query)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

func TestEventStore_Query_ByType(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store different event types
	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeAgentConnect).Source("/test2").Build(),
		NewEvent(EventTypeJobStart).Source("/test3").Build(),
	}

	store.StoreBatch(ctx, events)

	// Query for AgentConnect events
	query := NewEventQuery().WithTypes(EventTypeAgentConnect)
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(result.Events))
	}

	if result.TotalCount != 2 {
		t.Errorf("Expected TotalCount=2, got %d", result.TotalCount)
	}
}

func TestEventStore_Query_BySource(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/agent1").Build(),
		NewEvent(EventTypeAgentConnect).Source("/agent2").Build(),
		NewEvent(EventTypeAgentConnect).Source("/agent1").Build(),
	}

	store.StoreBatch(ctx, events)

	query := NewEventQuery().WithSources("/agent1")
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Expected 2 events from /agent1, got %d", len(result.Events))
	}
}

func TestEventStore_Query_BySeverity(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityInfo).Build(),
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityError).Build(),
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityCritical).Build(),
	}

	store.StoreBatch(ctx, events)

	query := NewEventQuery().WithSeverities(SeverityError, SeverityCritical)
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Expected 2 high-severity events, got %d", len(result.Events))
	}
}

func TestEventStore_Query_ByTimeRange(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	now := time.Now()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").Build(),
	}
	events[0].Time = past

	event2 := NewEvent(EventTypeJobStart).Source("/test").Build()
	event2.Time = now
	events = append(events, event2)

	event3 := NewEvent(EventTypeStateChange).Source("/test").Build()
	event3.Time = future
	events = append(events, event3)

	store.StoreBatch(ctx, events)

	// Query for events in the last hour
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	query := NewEventQuery().WithTimeRange(startTime, endTime)
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Expected 1 event in time range, got %d", len(result.Events))
	}
}

func TestEventStore_Query_ByCorrelationID(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").CorrelationID("corr-123").Build(),
		NewEvent(EventTypeJobStart).Source("/test").CorrelationID("corr-123").Build(),
		NewEvent(EventTypeStateChange).Source("/test").CorrelationID("corr-456").Build(),
	}

	store.StoreBatch(ctx, events)

	query := NewEventQuery().WithCorrelationID("corr-123")
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Expected 2 events with correlation ID, got %d", len(result.Events))
	}
}

func TestEventStore_Query_Pagination(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store 10 events
	var events []*Event
	for i := 0; i < 10; i++ {
		events = append(events, NewEvent(EventTypeAgentConnect).Source("/test").Build())
	}

	if err := store.StoreBatch(ctx, events); err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	// Get first page (5 events)
	query := NewEventQuery().WithPagination(5, 0)
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 5 {
		t.Errorf("Expected 5 events in first page, got %d", len(result.Events))
	}

	if result.TotalCount != 10 {
		t.Errorf("Expected TotalCount=10, got %d", result.TotalCount)
	}

	// Get second page
	query = NewEventQuery().WithPagination(5, 5)
	result, err = store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 5 {
		t.Errorf("Expected 5 events in second page, got %d", len(result.Events))
	}
}

func TestEventStore_Query_Sorting(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store events with different times
	now := time.Now()
	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").Build(),
	}
	events[0].Time = now.Add(-2 * time.Hour)

	event2 := NewEvent(EventTypeJobStart).Source("/test").Build()
	event2.Time = now.Add(-1 * time.Hour)
	events = append(events, event2)

	event3 := NewEvent(EventTypeStateChange).Source("/test").Build()
	event3.Time = now
	events = append(events, event3)

	store.StoreBatch(ctx, events)

	// Query sorted by time ascending
	query := NewEventQuery().WithSort("time", "asc")
	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(result.Events))
	}

	// Verify ascending order
	if result.Events[0].Type != EventTypeAgentConnect {
		t.Error("Expected oldest event first (ascending)")
	}

	if result.Events[2].Type != EventTypeStateChange {
		t.Error("Expected newest event last (ascending)")
	}

	// Query sorted by time descending (default)
	query = NewEventQuery().WithSort("time", "desc")
	result, err = store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify descending order
	if result.Events[0].Type != EventTypeStateChange {
		t.Error("Expected newest event first (descending)")
	}
}

func TestEventStore_Delete(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	store.Store(ctx, event)

	// Delete event
	err := store.Delete(ctx, event.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, event.ID)
	if err == nil {
		t.Error("Expected error getting deleted event")
	}
}

func TestEventStore_DeleteBatch(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	store.StoreBatch(ctx, events)

	// Delete first two events
	ids := []string{events[0].ID, events[1].ID}
	err := store.DeleteBatch(ctx, ids)
	if err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}

	// Verify count
	query := NewEventQuery()
	count, err := store.Count(ctx, query)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 remaining event, got %d", count)
	}
}

func TestEventStore_ApplyRetention_MaxAge(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store old and new events
	now := time.Now()
	oldEvent := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	oldEvent.Time = now.Add(-48 * time.Hour) // 2 days old

	newEvent := NewEvent(EventTypeJobStart).Source("/test").Build()
	newEvent.Time = now

	store.StoreBatch(ctx, []*Event{oldEvent, newEvent})

	// Apply retention: keep events less than 24 hours old
	policy := &RetentionPolicy{
		MaxAge: 24 * time.Hour,
	}

	deleted, err := store.ApplyRetention(ctx, policy)
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted event, got %d", deleted)
	}

	// Verify only new event remains
	query := NewEventQuery()
	count, _ := store.Count(ctx, query)

	if count != 1 {
		t.Errorf("Expected 1 remaining event, got %d", count)
	}
}

func TestEventStore_ApplyRetention_MaxCount(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store 10 events individually with delays to ensure different created_at
	for i := 0; i < 10; i++ {
		event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
		store.Store(ctx, event)
	}

	// Apply retention: keep only 5 events
	policy := &RetentionPolicy{
		MaxCount: 5,
	}

	deleted, err := store.ApplyRetention(ctx, policy)
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if deleted != 5 {
		t.Errorf("Expected 5 deleted events, got %d", deleted)
	}

	// Verify only 5 events remain
	query := NewEventQuery()
	count, _ := store.Count(ctx, query)

	if count != 5 {
		t.Errorf("Expected 5 remaining events, got %d", count)
	}
}

func TestEventStore_ApplyRetention_MinSeverity(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityDebug).Build(),
		NewEvent(EventTypeJobStart).Source("/test").Severity(SeverityInfo).Build(),
		NewEvent(EventTypeStateChange).Source("/test").Severity(SeverityWarning).Build(),
		NewEvent(EventTypeJobComplete).Source("/test").Severity(SeverityError).Build(),
	}

	store.StoreBatch(ctx, events)

	// Apply retention: keep only warnings and above
	policy := &RetentionPolicy{
		MinSeverity: SeverityWarning,
	}

	deleted, err := store.ApplyRetention(ctx, policy)
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 deleted events (debug, info), got %d", deleted)
	}

	// Verify only warning and error remain
	query := NewEventQuery()
	count, _ := store.Count(ctx, query)

	if count != 2 {
		t.Errorf("Expected 2 remaining events, got %d", count)
	}
}

func TestEventStore_GetMetrics(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityInfo).Build(),
		NewEvent(EventTypeAgentConnect).Source("/test").Severity(SeverityInfo).Build(),
		NewEvent(EventTypeJobStart).Source("/test").Severity(SeverityWarning).Build(),
	}

	store.StoreBatch(ctx, events)

	metrics, err := store.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.TotalEvents != 3 {
		t.Errorf("Expected TotalEvents=3, got %d", metrics.TotalEvents)
	}

	if metrics.EventsByType[EventTypeAgentConnect] != 2 {
		t.Errorf("Expected 2 AgentConnect events, got %d", metrics.EventsByType[EventTypeAgentConnect])
	}

	if metrics.EventsBySeverity[SeverityInfo] != 2 {
		t.Errorf("Expected 2 Info events, got %d", metrics.EventsBySeverity[SeverityInfo])
	}

	if metrics.OldestEvent.IsZero() {
		t.Error("Expected OldestEvent to be set")
	}

	if metrics.NewestEvent.IsZero() {
		t.Error("Expected NewestEvent to be set")
	}
}

func TestEventStore_Replay(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	store.StoreBatch(ctx, events)

	// Replay events
	var replayed []*Event
	handler := func(event *Event) error {
		replayed = append(replayed, event)
		return nil
	}

	query := NewEventQuery().WithTypes(EventTypeAgentConnect, EventTypeJobStart)
	err := store.Replay(ctx, query, handler)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(replayed) != 2 {
		t.Errorf("Expected 2 replayed events, got %d", len(replayed))
	}
}

func TestEventStore_ReplayFrom(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Create events at different times
	now := time.Now()
	oldEvent := NewEvent(EventTypeAgentConnect).Source("/old").Build()
	oldEvent.Time = now.Add(-2 * time.Hour)

	recentEvent := NewEvent(EventTypeAgentConnect).Source("/recent").Build()
	recentEvent.Time = now.Add(-30 * time.Minute)

	store.Store(ctx, oldEvent)
	store.Store(ctx, recentEvent)

	// Replay from 1 hour ago
	var replayed []*Event
	handler := func(event *Event) error {
		replayed = append(replayed, event)
		return nil
	}

	err := store.ReplayFrom(ctx, now.Add(-1*time.Hour), handler)
	if err != nil {
		t.Fatalf("ReplayFrom failed: %v", err)
	}

	// Should only get the recent event
	if len(replayed) != 1 {
		t.Errorf("Expected 1 replayed event, got %d", len(replayed))
	}
}

func TestEventStore_ReplayRange(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Create events with specific times
	now := time.Now()

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/e1").Build(),
		NewEvent(EventTypeJobStart).Source("/e2").Build(),
		NewEvent(EventTypeStateChange).Source("/e3").Build(),
	}

	// Set times to spread across 2 hours
	events[0].Time = now.Add(-2 * time.Hour)
	events[1].Time = now.Add(-1 * time.Hour)
	events[2].Time = now

	for _, e := range events {
		store.Store(ctx, e)
	}

	// Replay only middle time range
	var replayed []*Event
	handler := func(event *Event) error {
		replayed = append(replayed, event)
		return nil
	}

	startTime := now.Add(-90 * time.Minute)
	endTime := now.Add(-30 * time.Minute)

	err := store.ReplayRange(ctx, startTime, endTime, handler)
	if err != nil {
		t.Fatalf("ReplayRange failed: %v", err)
	}

	// Should only get the middle event
	if len(replayed) != 1 {
		t.Errorf("Expected 1 replayed event, got %d", len(replayed))
	}
}

func TestEventStore_ReplayRange_InvalidRange(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	now := time.Now()
	handler := func(event *Event) error { return nil }

	// Start time after end time should fail
	err := store.ReplayRange(ctx, now, now.Add(-1*time.Hour), handler)
	if err == nil {
		t.Error("Expected error for invalid time range")
	}
}

func TestEventStore_ReplayWithProgress(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Create multiple events
	for i := 0; i < 10; i++ {
		event := NewEvent(EventTypeAgentConnect).Source(fmt.Sprintf("/agent%d", i)).Build()
		store.Store(ctx, event)
	}

	var replayed []*Event
	var progressUpdates []*ReplayProgress

	handler := func(event *Event) error {
		replayed = append(replayed, event)
		return nil
	}

	progressFn := func(progress *ReplayProgress) {
		progressUpdates = append(progressUpdates, progress)
	}

	query := NewEventQuery()
	err := store.ReplayWithProgress(ctx, query, handler, progressFn)
	if err != nil {
		t.Fatalf("ReplayWithProgress failed: %v", err)
	}

	if len(replayed) != 10 {
		t.Errorf("Expected 10 replayed events, got %d", len(replayed))
	}

	if len(progressUpdates) != 10 {
		t.Errorf("Expected 10 progress updates, got %d", len(progressUpdates))
	}

	// Check last progress update
	if len(progressUpdates) > 0 {
		last := progressUpdates[len(progressUpdates)-1]
		if last.Percentage != 100.0 {
			t.Errorf("Expected final progress to be 100%%, got %.2f%%", last.Percentage)
		}
	}
}

func TestEventStore_ReplayBatched(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Create 25 events
	for i := 0; i < 25; i++ {
		event := NewEvent(EventTypeAgentConnect).Source(fmt.Sprintf("/agent%d", i)).Build()
		store.Store(ctx, event)
	}

	var batchCount int
	var totalEvents int

	handler := func(events []*Event) error {
		batchCount++
		totalEvents += len(events)
		return nil
	}

	query := NewEventQuery()
	err := store.ReplayBatched(ctx, query, 10, handler) // Batch size of 10
	if err != nil {
		t.Fatalf("ReplayBatched failed: %v", err)
	}

	// Should have 3 batches (10 + 10 + 5)
	if batchCount != 3 {
		t.Errorf("Expected 3 batches, got %d", batchCount)
	}

	if totalEvents != 25 {
		t.Errorf("Expected 25 total events, got %d", totalEvents)
	}
}

func TestEventStore_ReplayRangeWithTypes(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	now := time.Now()

	// Create mixed event types
	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/a1").Build(),
		NewEvent(EventTypeJobStart).Source("/j1").Build(),
		NewEvent(EventTypeStateChange).Source("/s1").Build(),
		NewEvent(EventTypeAgentConnect).Source("/a2").Build(),
	}

	for i, e := range events {
		e.Time = now.Add(time.Duration(i) * time.Minute)
		store.Store(ctx, e)
	}

	// Replay only AgentConnect events
	var replayed []*Event
	handler := func(event *Event) error {
		replayed = append(replayed, event)
		return nil
	}

	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	err := store.ReplayRangeWithTypes(ctx, startTime, endTime, []EventType{EventTypeAgentConnect}, handler)
	if err != nil {
		t.Fatalf("ReplayRangeWithTypes failed: %v", err)
	}

	if len(replayed) != 2 {
		t.Errorf("Expected 2 AgentConnect events, got %d", len(replayed))
	}

	for _, e := range replayed {
		if e.Type != EventTypeAgentConnect {
			t.Errorf("Expected AgentConnect event, got %v", e.Type)
		}
	}
}

func TestEventStore_Close(t *testing.T) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)

	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestEventStore_AutoRetention(t *testing.T) {
	// This test verifies that auto-retention background goroutine works
	// We test the retention logic itself in the ApplyRetention tests above

	tmpfile := "/tmp/test-events-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpfile)

	config := DefaultEventStoreConfig()
	config.Path = tmpfile
	config.AutoRetention = true
	config.RetentionCheckInterval = 100 * time.Millisecond
	config.DefaultRetentionPolicy = &RetentionPolicy{
		MaxCount: 5,
	}

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Store events individually to ensure different created_at timestamps
	for i := 0; i < 10; i++ {
		event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
		store.Store(ctx, event)
	}

	if err := helpers.WaitForTimeout(5*time.Second, 10*time.Millisecond, func() (bool, error) {
		query := NewEventQuery()
		count, _ := store.Count(ctx, query)
		return count <= 5, nil
	}); err != nil {
		t.Fatalf("Expected auto-retention to reduce events: %v", err)
	}

	// Verify retention was applied (should keep only 5 events)
	query := NewEventQuery()
	count, _ := store.Count(ctx, query)

	// The count should be 5 or less (if retention ran multiple times)
	if count > 5 {
		t.Errorf("Expected auto-retention to reduce events to 5 or less, got %d", count)
	}
}

// Benchmark event storage
func BenchmarkEventStore_Store(b *testing.B) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newEvent := NewEvent(EventTypeAgentConnect).
			Source("/test").
			Severity(SeverityInfo).
			Tag("env", "test").
			Data("key", "value").
			Build()
		store.Store(ctx, newEvent)
	}
}

func BenchmarkEventStore_Query(b *testing.B) {
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, _ := NewSQLiteEventStore(config)
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with events
	var events []*Event
	for i := 0; i < 1000; i++ {
		events = append(events, NewEvent(EventTypeAgentConnect).Source("/test").Build())
	}
	store.StoreBatch(ctx, events)

	query := NewEventQuery().WithTypes(EventTypeAgentConnect).WithPagination(100, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Query(ctx, query)
	}
}
