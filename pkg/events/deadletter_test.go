package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSQLiteDeadLetterQueue(t *testing.T) {
	config := DefaultDeadLetterConfig()
	config.Path = ":memory:"
	config.AutoRetry = false // Disable for testing

	dlq, err := NewSQLiteDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("Failed to create dead letter queue: %v", err)
	}
	defer dlq.Close()

	if dlq.db == nil {
		t.Error("Expected database to be initialized")
	}
}

func TestDeadLetterEnqueue(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	entry := &DeadLetterEntry{
		ReactorID:   "test-reactor",
		ReactorName: "Test Reactor",
		Event:       createTestEvent(t),
		ActionIndex: 0,
		ActionName:  "test-action",
		Error:       "test error",
	}

	err := dlq.Enqueue(ctx, entry)
	if err != nil {
		t.Fatalf("Failed to enqueue entry: %v", err)
	}

	// Entry should have been assigned an ID
	if entry.ID == "" {
		t.Error("Expected entry to have an ID")
	}

	// Verify it was stored
	retrieved, err := dlq.Get(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}

	if retrieved.ReactorID != entry.ReactorID {
		t.Errorf("Expected reactor ID %s, got %s", entry.ReactorID, retrieved.ReactorID)
	}
	if retrieved.Error != entry.Error {
		t.Errorf("Expected error %s, got %s", entry.Error, retrieved.Error)
	}
	if retrieved.Status != DeadLetterStatusPending {
		t.Errorf("Expected status pending, got %s", retrieved.Status)
	}
	if retrieved.Event == nil {
		t.Error("Expected event to be preserved")
	}
}

func TestDeadLetterQuery(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create multiple entries
	for i := 0; i < 5; i++ {
		entry := &DeadLetterEntry{
			ReactorID:   "reactor-1",
			ReactorName: "Reactor 1",
			Event:       createTestEvent(t),
			Error:       "test error",
		}
		dlq.Enqueue(ctx, entry)
	}
	for i := 0; i < 3; i++ {
		entry := &DeadLetterEntry{
			ReactorID:   "reactor-2",
			ReactorName: "Reactor 2",
			Event:       createTestEvent(t),
			Error:       "test error",
		}
		dlq.Enqueue(ctx, entry)
	}

	// Query all
	result, err := dlq.Query(ctx, &DeadLetterQuery{})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 8 {
		t.Errorf("Expected 8 entries, got %d", result.TotalCount)
	}

	// Query by reactor ID
	result, err = dlq.Query(ctx, &DeadLetterQuery{
		ReactorIDs: []string{"reactor-1"},
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 5 {
		t.Errorf("Expected 5 entries for reactor-1, got %d", result.TotalCount)
	}

	// Query with pagination
	result, err = dlq.Query(ctx, &DeadLetterQuery{
		Limit:  3,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(result.Entries) != 3 {
		t.Errorf("Expected 3 entries in page, got %d", len(result.Entries))
	}
}

func TestDeadLetterUpdateStatus(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	entry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "test error",
	}
	dlq.Enqueue(ctx, entry)

	// Update to resolved
	err := dlq.UpdateStatus(ctx, entry.ID, DeadLetterStatusResolved)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, _ := dlq.Get(ctx, entry.ID)
	if retrieved.Status != DeadLetterStatusResolved {
		t.Errorf("Expected status resolved, got %s", retrieved.Status)
	}
}

func TestDeadLetterIncrementRetry(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	entry := &DeadLetterEntry{
		ReactorID:  "test-reactor",
		Event:      createTestEvent(t),
		Error:      "test error",
		MaxRetries: 3,
	}
	dlq.Enqueue(ctx, entry)

	// Increment retry
	err := dlq.IncrementRetry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Failed to increment retry: %v", err)
	}

	retrieved, _ := dlq.Get(ctx, entry.ID)
	if retrieved.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", retrieved.RetryCount)
	}
	if retrieved.LastRetryAt == nil {
		t.Error("Expected LastRetryAt to be set")
	}
	if retrieved.Status != DeadLetterStatusPending {
		t.Errorf("Expected status pending, got %s", retrieved.Status)
	}

	// Retry until max
	dlq.IncrementRetry(ctx, entry.ID)
	dlq.IncrementRetry(ctx, entry.ID)

	retrieved, _ = dlq.Get(ctx, entry.ID)
	if retrieved.Status != DeadLetterStatusFailed {
		t.Errorf("Expected status failed after max retries, got %s", retrieved.Status)
	}
}

func TestDeadLetterDelete(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	entry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "test error",
	}
	dlq.Enqueue(ctx, entry)

	// Delete
	err := dlq.Delete(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Failed to delete entry: %v", err)
	}

	// Verify deleted
	_, err = dlq.Get(ctx, entry.ID)
	if err == nil {
		t.Error("Expected error getting deleted entry")
	}
}

func TestDeadLetterDeleteByReactor(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entries for two reactors
	for i := 0; i < 3; i++ {
		dlq.Enqueue(ctx, &DeadLetterEntry{
			ReactorID: "reactor-1",
			Event:     createTestEvent(t),
			Error:     "test error",
		})
	}
	for i := 0; i < 2; i++ {
		dlq.Enqueue(ctx, &DeadLetterEntry{
			ReactorID: "reactor-2",
			Event:     createTestEvent(t),
			Error:     "test error",
		})
	}

	// Delete reactor-1 entries
	err := dlq.DeleteByReactor(ctx, "reactor-1")
	if err != nil {
		t.Fatalf("Failed to delete by reactor: %v", err)
	}

	// Verify only reactor-2 entries remain
	result, _ := dlq.Query(ctx, &DeadLetterQuery{})
	if result.TotalCount != 2 {
		t.Errorf("Expected 2 entries remaining, got %d", result.TotalCount)
	}
}

func TestDeadLetterPurge(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entry with old timestamp
	oldEntry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "old error",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	dlq.Enqueue(ctx, oldEntry)

	// Create recent entry
	newEntry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "new error",
	}
	dlq.Enqueue(ctx, newEntry)

	// Purge entries older than 24 hours
	deleted, err := dlq.Purge(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to purge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify only new entry remains
	result, _ := dlq.Query(ctx, &DeadLetterQuery{})
	if result.TotalCount != 1 {
		t.Errorf("Expected 1 entry remaining, got %d", result.TotalCount)
	}
}

func TestDeadLetterCount(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entries with different statuses
	for i := 0; i < 5; i++ {
		dlq.Enqueue(ctx, &DeadLetterEntry{
			ReactorID: "test-reactor",
			Event:     createTestEvent(t),
			Error:     "test error",
		})
	}

	// Mark some as resolved
	result, _ := dlq.Query(ctx, &DeadLetterQuery{Limit: 2})
	for _, entry := range result.Entries {
		dlq.UpdateStatus(ctx, entry.ID, DeadLetterStatusResolved)
	}

	// Count pending
	pendingCount, err := dlq.Count(ctx, &DeadLetterQuery{
		Statuses: []DeadLetterStatus{DeadLetterStatusPending},
	})
	if err != nil {
		t.Fatalf("Failed to count: %v", err)
	}
	if pendingCount != 3 {
		t.Errorf("Expected 3 pending, got %d", pendingCount)
	}

	// Count resolved
	resolvedCount, _ := dlq.Count(ctx, &DeadLetterQuery{
		Statuses: []DeadLetterStatus{DeadLetterStatusResolved},
	})
	if resolvedCount != 2 {
		t.Errorf("Expected 2 resolved, got %d", resolvedCount)
	}
}

func TestDeadLetterReadyForRetry(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entry with past next_retry_at
	entry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "test error",
	}
	dlq.Enqueue(ctx, entry)

	// Update next_retry_at to past
	dlq.db.Exec("UPDATE dead_letter_entries SET next_retry_at = ? WHERE id = ?",
		time.Now().Add(-1*time.Minute), entry.ID)

	// Query ready for retry
	result, err := dlq.Query(ctx, &DeadLetterQuery{
		ReadyForRetry: true,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("Expected 1 entry ready for retry, got %d", len(result.Entries))
	}
}

func TestDeadLetterExponentialBackoff(t *testing.T) {
	config := DefaultDeadLetterConfig()
	config.Path = ":memory:"
	config.AutoRetry = false
	config.RetryBackoff = 1 * time.Second
	config.RetryBackoffMultiplier = 2.0
	config.MaxBackoff = 1 * time.Minute

	dlq, _ := NewSQLiteDeadLetterQueue(config)
	defer dlq.Close()

	// Test backoff calculation
	// retryCount=0 means first attempt, backoff is base (1s)
	// retryCount=1 means after 1 failed retry, backoff is 1s * 2.0 = 2s
	// retryCount=2 means after 2 failed retries, backoff is 1s * 2.0 * 2.0 = 4s
	retry0 := dlq.calculateNextRetry(0)
	retry1 := dlq.calculateNextRetry(1)
	retry2 := dlq.calculateNextRetry(2)

	now := time.Now()
	diff0 := retry0.Sub(now)
	diff1 := retry1.Sub(now)
	diff2 := retry2.Sub(now)

	// Retry 0 should be ~1 second (base backoff)
	if diff0 < 900*time.Millisecond || diff0 > 1100*time.Millisecond {
		t.Errorf("Expected retry 0 backoff ~1s, got %v", diff0)
	}

	// Retry 1 should be ~2 seconds (1s * 2.0)
	if diff1 < 1900*time.Millisecond || diff1 > 2100*time.Millisecond {
		t.Errorf("Expected retry 1 backoff ~2s, got %v", diff1)
	}

	// Retry 2 should be ~4 seconds (1s * 2.0 * 2.0)
	if diff2 < 3900*time.Millisecond || diff2 > 4100*time.Millisecond {
		t.Errorf("Expected retry 2 backoff ~4s, got %v", diff2)
	}

	// Verify backoff is capped at max
	retryHigh := dlq.calculateNextRetry(100)
	diffHigh := retryHigh.Sub(time.Now())
	if diffHigh > config.MaxBackoff+time.Second {
		t.Errorf("Expected backoff capped at %v, got %v", config.MaxBackoff, diffHigh)
	}
}

func TestDeadLetterAlert(t *testing.T) {
	var alertReceived atomic.Bool
	var receivedAlert *DeadLetterAlert

	config := DefaultDeadLetterConfig()
	config.Path = ":memory:"
	config.AutoRetry = false
	config.AlertThreshold = 5
	config.AlertCallback = func(alert *DeadLetterAlert) {
		alertReceived.Store(true)
		receivedAlert = alert
	}

	dlq, _ := NewSQLiteDeadLetterQueue(config)
	defer dlq.Close()

	ctx := context.Background()

	// Add entries to trigger threshold
	for i := 0; i < 6; i++ {
		dlq.Enqueue(ctx, &DeadLetterEntry{
			ReactorID:   "test-reactor",
			ReactorName: "Test Reactor",
			Event:       createTestEvent(t),
			Error:       "test error",
		})
	}

	// Wait for alert to be processed
	time.Sleep(100 * time.Millisecond)

	if !alertReceived.Load() {
		t.Error("Expected alert to be triggered")
	}

	if receivedAlert != nil {
		if receivedAlert.PendingCount < 5 {
			t.Errorf("Expected pending count >= 5, got %d", receivedAlert.PendingCount)
		}
		if receivedAlert.Threshold != 5 {
			t.Errorf("Expected threshold 5, got %d", receivedAlert.Threshold)
		}
	}
}

func TestDeadLetterMetrics(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Enqueue some entries
	for i := 0; i < 3; i++ {
		dlq.Enqueue(ctx, &DeadLetterEntry{
			ReactorID: "test-reactor",
			Event:     createTestEvent(t),
			Error:     "test error",
		})
	}

	// Update status of some
	result, _ := dlq.Query(ctx, &DeadLetterQuery{Limit: 1})
	dlq.UpdateStatus(ctx, result.Entries[0].ID, DeadLetterStatusResolved)

	// Retry one
	dlq.IncrementRetry(ctx, result.Entries[0].ID)

	metrics := dlq.GetMetrics()
	if metrics.EnqueuedCount != 3 {
		t.Errorf("Expected 3 enqueued, got %d", metrics.EnqueuedCount)
	}
	if metrics.RetriedCount != 1 {
		t.Errorf("Expected 1 retried, got %d", metrics.RetriedCount)
	}
}

func TestDeadLetterQuerySorting(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entries with different retry counts
	for i := 0; i < 3; i++ {
		entry := &DeadLetterEntry{
			ReactorID: "test-reactor",
			Event:     createTestEvent(t),
			Error:     "test error",
		}
		dlq.Enqueue(ctx, entry)

		// Increment retry count differently for each
		for j := 0; j < i; j++ {
			dlq.IncrementRetry(ctx, entry.ID)
		}
	}

	// Query sorted by retry count descending
	result, err := dlq.Query(ctx, &DeadLetterQuery{
		SortBy:    "retry_count",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	// Verify sorting
	for i := 1; i < len(result.Entries); i++ {
		if result.Entries[i].RetryCount > result.Entries[i-1].RetryCount {
			t.Error("Results not sorted by retry count descending")
		}
	}
}

func TestDeadLetterWithMetadata(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	entry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "test error",
		Metadata: map[string]interface{}{
			"custom_field": "custom_value",
			"count":        42,
		},
	}
	dlq.Enqueue(ctx, entry)

	retrieved, err := dlq.Get(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}

	if retrieved.Metadata == nil {
		t.Fatal("Expected metadata to be preserved")
	}
	if retrieved.Metadata["custom_field"] != "custom_value" {
		t.Errorf("Expected custom_field to be preserved")
	}
}

func TestDeadLetterQueryByTimeRange(t *testing.T) {
	dlq := createTestDeadLetterQueue(t)
	defer dlq.Close()

	ctx := context.Background()

	// Create entries at different times
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()

	oldEntry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "old error",
		CreatedAt: oldTime,
	}
	dlq.Enqueue(ctx, oldEntry)

	newEntry := &DeadLetterEntry{
		ReactorID: "test-reactor",
		Event:     createTestEvent(t),
		Error:     "new error",
		CreatedAt: newTime,
	}
	dlq.Enqueue(ctx, newEntry)

	// Query for recent entries only
	cutoff := time.Now().Add(-1 * time.Hour)
	result, err := dlq.Query(ctx, &DeadLetterQuery{
		StartTime: &cutoff,
	})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if result.TotalCount != 1 {
		t.Errorf("Expected 1 entry in time range, got %d", result.TotalCount)
	}
}

// Helper functions

func createTestDeadLetterQueue(t *testing.T) *SQLiteDeadLetterQueue {
	t.Helper()

	config := DefaultDeadLetterConfig()
	config.Path = ":memory:"
	config.AutoRetry = false

	dlq, err := NewSQLiteDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("Failed to create dead letter queue: %v", err)
	}

	return dlq
}

func createTestEvent(t *testing.T) *Event {
	t.Helper()

	return NewEvent(EventType("test.event")).
		Source("/test").
		Severity(SeverityInfo).
		DataMap(map[string]interface{}{
			"test_key": "test_value",
		}).
		Build()
}
