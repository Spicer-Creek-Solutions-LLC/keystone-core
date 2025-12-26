package query

import (
	"context"
	"testing"
	"time"
)

func TestNewInMemoryLogsQuerier(t *testing.T) {
	querier := NewInMemoryLogsQuerier()
	if querier == nil {
		t.Fatal("NewInMemoryLogsQuerier returned nil")
	}
}

func TestLogsQuerierAddEntry(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	entry := LogEntry{
		Timestamp: time.Now(),
		Line:      "Test log",
		Labels:    map[string]string{"app": "test"},
	}

	querier.AddEntry(entry)

	querier.mu.RLock()
	defer querier.mu.RUnlock()

	if len(querier.entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(querier.entries))
	}
}

func TestLogsQuerierQueryTimeRange(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	// Add entries at different times
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-10 * time.Minute),
		Line:      "Old log",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Recent log",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-1 * time.Minute),
		Line:      "New log",
	})

	ctx := context.Background()
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-6 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Should return 2 entries (recent and new)
	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result.Entries))
	}
}

func TestLogsQuerierQueryWithFilter(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Error in system",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-4 * time.Minute),
		Line:      "Info message",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-3 * time.Minute),
		Line:      "Another error occurred",
	})

	ctx := context.Background()
	query := &LogsQuery{
		Query: "error",
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Should return 2 entries with "error"
	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result.Entries))
	}
}

func TestLogsQuerierQueryLimit(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	// Add 10 entries
	for i := 0; i < 10; i++ {
		querier.AddEntry(LogEntry{
			Timestamp: now.Add(time.Duration(-i) * time.Minute),
			Line:      "Test log",
		})
	}

	ctx := context.Background()
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-20 * time.Minute),
			End:   now,
		},
		Limit: 5,
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Entries) != 5 {
		t.Errorf("Expected 5 entries (limited), got %d", len(result.Entries))
	}
}

func TestLogsQuerierQueryDirection(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-3 * time.Minute),
		Line:      "First",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-2 * time.Minute),
		Line:      "Second",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-1 * time.Minute),
		Line:      "Third",
	})

	ctx := context.Background()

	// Test backward (most recent first)
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
		Direction: "backward",
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(result.Entries))
	}

	if result.Entries[0].Line != "Third" {
		t.Errorf("Expected first entry to be 'Third', got '%s'", result.Entries[0].Line)
	}

	// Test forward (oldest first)
	query.Direction = "forward"

	result, err = querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Entries[0].Line != "First" {
		t.Errorf("Expected first entry to be 'First', got '%s'", result.Entries[0].Line)
	}
}

func TestLogsQuerierStats(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Test log 1",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-3 * time.Minute),
		Line:      "Test log 2",
	})

	ctx := context.Background()
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Stats == nil {
		t.Fatal("Expected stats to be present")
	}

	if result.Stats.Summary.LinesProcessed != 2 {
		t.Errorf("Expected 2 lines processed, got %d", result.Stats.Summary.LinesProcessed)
	}

	if result.Stats.Summary.BytesProcessed <= 0 {
		t.Error("Expected bytes processed > 0")
	}

	if result.Stats.Summary.ExecTime <= 0 {
		t.Error("Expected exec time > 0")
	}
}

func TestLogsQuerierQueryWithLabels(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Log without match",
		Labels:    map[string]string{"app": "other"},
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-4 * time.Minute),
		Line:      "Log with match",
		Labels:    map[string]string{"app": "myapp"},
	})

	ctx := context.Background()
	query := &LogsQuery{
		Query: "myapp",
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(result.Entries))
	}

	if result.Entries[0].Line != "Log with match" {
		t.Errorf("Expected 'Log with match', got '%s'", result.Entries[0].Line)
	}
}
