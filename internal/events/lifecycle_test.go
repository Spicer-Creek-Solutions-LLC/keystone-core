package events

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewSQLiteLifecycleTracker(t *testing.T) {
	config := DefaultLifecycleTrackerConfig()
	config.Path = ":memory:"
	config.EnableAutoCleanup = false

	tracker, err := NewSQLiteLifecycleTracker(config)
	if err != nil {
		t.Fatalf("Failed to create lifecycle tracker: %v", err)
	}
	defer tracker.Close()

	if tracker.db == nil {
		t.Error("Expected database to be initialized")
	}
}

func TestLifecycleTracker_Track(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Track creation
	transition := &LifecycleTransition{
		EventID:   "event-1",
		ToState:   LifecycleStateCreated,
		Timestamp: time.Now(),
		Component: "test-publisher",
		Details: map[string]interface{}{
			"event_type": "test.event",
		},
	}

	err := tracker.Track(ctx, transition)
	if err != nil {
		t.Fatalf("Failed to track transition: %v", err)
	}

	// Verify lifecycle was created
	lifecycle, err := tracker.Get(ctx, "event-1")
	if err != nil {
		t.Fatalf("Failed to get lifecycle: %v", err)
	}

	if lifecycle.EventID != "event-1" {
		t.Errorf("Expected event ID 'event-1', got '%s'", lifecycle.EventID)
	}
	if lifecycle.CurrentState != LifecycleStateCreated {
		t.Errorf("Expected state 'created', got '%s'", lifecycle.CurrentState)
	}
}

func TestLifecycleTracker_MultipleTransitions(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()
	eventID := "event-multi"

	// Track multiple transitions
	transitions := []LifecycleState{
		LifecycleStateCreated,
		LifecycleStatePublished,
		LifecycleStateRouted,
		LifecycleStateProcessing,
		LifecycleStateProcessed,
	}

	baseTime := time.Now()
	for i, state := range transitions {
		err := tracker.Track(ctx, &LifecycleTransition{
			EventID:   eventID,
			ToState:   state,
			Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
			Component: "test",
		})
		if err != nil {
			t.Fatalf("Failed to track %s: %v", state, err)
		}
	}

	// Get lifecycle
	lifecycle, err := tracker.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("Failed to get lifecycle: %v", err)
	}

	if lifecycle.CurrentState != LifecycleStateProcessed {
		t.Errorf("Expected final state 'processed', got '%s'", lifecycle.CurrentState)
	}

	if len(lifecycle.Transitions) != len(transitions) {
		t.Errorf("Expected %d transitions, got %d", len(transitions), len(lifecycle.Transitions))
	}
}

func TestLifecycleTracker_FailedState(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()
	eventID := "event-failed"

	// Create event
	tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateCreated,
		Component: "test",
	})

	// Track failure
	err := tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateFailed,
		Component: "test",
		Details: map[string]interface{}{
			"error": "test error message",
		},
	})
	if err != nil {
		t.Fatalf("Failed to track failure: %v", err)
	}

	lifecycle, _ := tracker.Get(ctx, eventID)
	if lifecycle.CurrentState != LifecycleStateFailed {
		t.Errorf("Expected state 'failed', got '%s'", lifecycle.CurrentState)
	}
}

func TestLifecycleTracker_Get_NotFound(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	_, err := tracker.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent lifecycle")
	}
}

func TestLifecycleTracker_Query(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Create multiple lifecycles
	for i := 0; i < 5; i++ {
		tracker.Track(ctx, &LifecycleTransition{
			EventID:   fmt.Sprintf("event-%d", i),
			ToState:   LifecycleStateCreated,
			Component: "test",
			Details: map[string]interface{}{
				"event_type": "test.event",
			},
		})
	}

	// Mark some as processed
	for i := 0; i < 3; i++ {
		tracker.Track(ctx, &LifecycleTransition{
			EventID:   fmt.Sprintf("event-%d", i),
			ToState:   LifecycleStateProcessed,
			Component: "test",
		})
	}

	// Query all
	result, err := tracker.Query(ctx, &LifecycleQuery{})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 5 {
		t.Errorf("Expected 5 total, got %d", result.TotalCount)
	}

	// Query by state
	result, err = tracker.Query(ctx, &LifecycleQuery{
		States: []LifecycleState{LifecycleStateProcessed},
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 3 {
		t.Errorf("Expected 3 processed, got %d", result.TotalCount)
	}
}

func TestLifecycleTracker_Query_TerminalOnly(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Create events in different states
	states := map[string]LifecycleState{
		"e1": LifecycleStateCreated,
		"e2": LifecycleStateProcessing,
		"e3": LifecycleStateProcessed,
		"e4": LifecycleStateFailed,
		"e5": LifecycleStateArchived,
	}

	for eventID, state := range states {
		tracker.Track(ctx, &LifecycleTransition{
			EventID:   eventID,
			ToState:   state,
			Component: "test",
		})
	}

	// Query terminal only
	result, err := tracker.Query(ctx, &LifecycleQuery{
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 3 { // processed, failed, archived
		t.Errorf("Expected 3 terminal, got %d", result.TotalCount)
	}

	// Query active only
	result, err = tracker.Query(ctx, &LifecycleQuery{
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 2 { // created, processing
		t.Errorf("Expected 2 active, got %d", result.TotalCount)
	}
}

func TestLifecycleTracker_Query_Pagination(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Create 10 lifecycles
	for i := 0; i < 10; i++ {
		tracker.Track(ctx, &LifecycleTransition{
			EventID:   fmt.Sprintf("event-%02d", i),
			ToState:   LifecycleStateCreated,
			Component: "test",
		})
	}

	// Query with pagination
	result, err := tracker.Query(ctx, &LifecycleQuery{
		Limit:  3,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(result.Lifecycles) != 3 {
		t.Errorf("Expected 3 in first page, got %d", len(result.Lifecycles))
	}
	if result.TotalCount != 10 {
		t.Errorf("Expected total count 10, got %d", result.TotalCount)
	}
}

func TestLifecycleTracker_GetMetrics(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Create lifecycles in different states
	for i := 0; i < 5; i++ {
		eventID := fmt.Sprintf("event-%d", i)
		tracker.Track(ctx, &LifecycleTransition{
			EventID:   eventID,
			ToState:   LifecycleStateCreated,
			Component: "test",
		})
		if i < 3 {
			tracker.Track(ctx, &LifecycleTransition{
				EventID:   eventID,
				ToState:   LifecycleStateProcessed,
				Component: "test",
			})
		} else {
			tracker.Track(ctx, &LifecycleTransition{
				EventID:   eventID,
				ToState:   LifecycleStateFailed,
				Component: "test",
			})
		}
	}

	metrics, err := tracker.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}

	if metrics.TotalTracked != 5 {
		t.Errorf("Expected 5 total tracked, got %d", metrics.TotalTracked)
	}

	if metrics.ByState[LifecycleStateProcessed] != 3 {
		t.Errorf("Expected 3 processed, got %d", metrics.ByState[LifecycleStateProcessed])
	}

	if metrics.ByState[LifecycleStateFailed] != 2 {
		t.Errorf("Expected 2 failed, got %d", metrics.ByState[LifecycleStateFailed])
	}

	// Success rate should be 3/5 = 0.6
	expectedRate := 0.6
	if metrics.SuccessRate < expectedRate-0.01 || metrics.SuccessRate > expectedRate+0.01 {
		t.Errorf("Expected success rate ~%.2f, got %.2f", expectedRate, metrics.SuccessRate)
	}
}

func TestLifecycleTracker_Cleanup(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Create old lifecycle
	oldTime := time.Now().Add(-48 * time.Hour)
	tracker.Track(ctx, &LifecycleTransition{
		EventID:   "old-event",
		ToState:   LifecycleStateProcessed,
		Timestamp: oldTime,
		Component: "test",
	})

	// Create recent lifecycle
	tracker.Track(ctx, &LifecycleTransition{
		EventID:   "new-event",
		ToState:   LifecycleStateProcessed,
		Component: "test",
	})

	// Cleanup events older than 24 hours
	deleted, err := tracker.Cleanup(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify old event is gone
	_, err = tracker.Get(ctx, "old-event")
	if err == nil {
		t.Error("Expected old event to be deleted")
	}

	// Verify new event still exists
	_, err = tracker.Get(ctx, "new-event")
	if err != nil {
		t.Error("Expected new event to still exist")
	}
}

func TestEventLifecycle_TotalDuration(t *testing.T) {
	createdAt := time.Now().Add(-1 * time.Hour)
	updatedAt := time.Now()

	lifecycle := &EventLifecycle{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	duration := lifecycle.TotalDuration()
	expected := 1 * time.Hour

	if duration < expected-time.Minute || duration > expected+time.Minute {
		t.Errorf("Expected duration ~%v, got %v", expected, duration)
	}
}

func TestEventLifecycle_IsTerminal(t *testing.T) {
	tests := []struct {
		state    LifecycleState
		terminal bool
	}{
		{LifecycleStateCreated, false},
		{LifecycleStatePublished, false},
		{LifecycleStateRouted, false},
		{LifecycleStateProcessing, false},
		{LifecycleStateProcessed, true},
		{LifecycleStateFailed, true},
		{LifecycleStateRetrying, false},
		{LifecycleStateArchived, true},
		{LifecycleStateExpired, true},
	}

	for _, test := range tests {
		lifecycle := &EventLifecycle{CurrentState: test.state}
		if lifecycle.IsTerminal() != test.terminal {
			t.Errorf("Expected IsTerminal() = %v for state %s", test.terminal, test.state)
		}
	}
}

func TestLifecycleMiddleware(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()
	middleware := NewLifecycleMiddleware(tracker, "test-component")

	// Create event
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	// Track lifecycle
	middleware.TrackCreated(ctx, event)
	middleware.TrackPublished(ctx, event.ID)
	middleware.TrackRouted(ctx, event.ID, 3)
	middleware.TrackProcessing(ctx, event.ID, "handler-1")
	middleware.TrackProcessed(ctx, event.ID)

	// Verify
	lifecycle, err := tracker.Get(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to get lifecycle: %v", err)
	}

	if lifecycle.CurrentState != LifecycleStateProcessed {
		t.Errorf("Expected state 'processed', got '%s'", lifecycle.CurrentState)
	}

	if len(lifecycle.Transitions) != 5 {
		t.Errorf("Expected 5 transitions, got %d", len(lifecycle.Transitions))
	}
}

func TestLifecycleMiddleware_TrackFailed(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()
	middleware := NewLifecycleMiddleware(tracker, "test-component")

	eventID := "failed-event"

	// Track creation and failure
	middleware.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateCreated,
		Component: "test",
	})

	testErr := fmt.Errorf("test processing error")
	middleware.TrackFailed(ctx, eventID, testErr)

	lifecycle, _ := tracker.Get(ctx, eventID)
	if lifecycle.CurrentState != LifecycleStateFailed {
		t.Errorf("Expected state 'failed', got '%s'", lifecycle.CurrentState)
	}
}

func TestLifecycleMiddleware_TrackArchived(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()
	middleware := NewLifecycleMiddleware(tracker, "test-component")

	eventID := "archived-event"

	// Track creation and archival
	middleware.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateCreated,
		Component: "test",
	})

	middleware.TrackArchived(ctx, eventID, "s3://bucket/archive")

	lifecycle, _ := tracker.Get(ctx, eventID)
	if lifecycle.CurrentState != LifecycleStateArchived {
		t.Errorf("Expected state 'archived', got '%s'", lifecycle.CurrentState)
	}
}

func TestLifecycleTracker_ValidationErrors(t *testing.T) {
	tracker := createTestLifecycleTracker(t)
	defer tracker.Close()

	ctx := context.Background()

	// Nil transition
	err := tracker.Track(ctx, nil)
	if err == nil {
		t.Error("Expected error for nil transition")
	}

	// Empty event ID
	err = tracker.Track(ctx, &LifecycleTransition{
		ToState: LifecycleStateCreated,
	})
	if err == nil {
		t.Error("Expected error for empty event ID")
	}

	// Empty state
	err = tracker.Track(ctx, &LifecycleTransition{
		EventID: "test",
	})
	if err == nil {
		t.Error("Expected error for empty state")
	}
}

func TestDefaultLifecycleTrackerConfig(t *testing.T) {
	config := DefaultLifecycleTrackerConfig()

	if config.RetentionPeriod <= 0 {
		t.Error("Expected positive retention period")
	}
	if config.CleanupInterval <= 0 {
		t.Error("Expected positive cleanup interval")
	}
	if config.MaxBatchSize <= 0 {
		t.Error("Expected positive max batch size")
	}
}

// Helper for tests
func createTestLifecycleTracker(t *testing.T) *SQLiteLifecycleTracker {
	t.Helper()

	config := DefaultLifecycleTrackerConfig()
	config.Path = ":memory:"
	config.EnableAutoCleanup = false

	tracker, err := NewSQLiteLifecycleTracker(config)
	if err != nil {
		t.Fatalf("Failed to create lifecycle tracker: %v", err)
	}

	return tracker
}
