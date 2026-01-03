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

// Tests for helper functions
func TestContainsQuery(t *testing.T) {
	tests := []struct {
		entry    LogEntry
		query    string
		expected bool
	}{
		{
			entry:    LogEntry{Line: "Error: connection refused"},
			query:    "error",
			expected: true, // case-insensitive
		},
		{
			entry:    LogEntry{Line: "Error: connection refused"},
			query:    "ERROR",
			expected: true, // case-insensitive
		},
		{
			entry:    LogEntry{Line: "Info: all good"},
			query:    "error",
			expected: false,
		},
		{
			entry:    LogEntry{Line: "", Labels: map[string]string{"app": "myapp"}},
			query:    "myapp",
			expected: true, // matches label value
		},
		{
			entry:    LogEntry{Line: "", Labels: map[string]string{"app": "myapp"}},
			query:    "app",
			expected: true, // matches label key
		},
		{
			entry:    LogEntry{Line: "test", Labels: map[string]string{}},
			query:    "",
			expected: true, // empty query matches everything
		},
	}

	for _, tt := range tests {
		result := containsQuery(tt.entry, tt.query)
		if result != tt.expected {
			t.Errorf("containsQuery(%v, %s) = %v, want %v", tt.entry, tt.query, result, tt.expected)
		}
	}
}

func TestFindSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "lo wo", true},
		{"hello world", "foo", false},
		{"hello", "hello world", false}, // substr longer than s
		{"hello", "", true},             // empty substr always matches
		{"", "", true},                  // empty string matches empty substr
		{"", "a", false},                // empty string doesn't contain "a"
	}

	for _, tt := range tests {
		result := findSubstring(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("findSubstring(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
		}
	}
}

func TestToLowerCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HELLO", "hello"},
		{"Hello World", "hello world"},
		{"hello", "hello"},
		{"123ABC", "123abc"},
		{"", ""},
		{"ABC123xyz", "abc123xyz"},
	}

	for _, tt := range tests {
		result := toLowerCase(tt.input)
		if result != tt.expected {
			t.Errorf("toLowerCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCalculateBytes(t *testing.T) {
	tests := []struct {
		entries  []LogEntry
		expected int64
	}{
		{
			entries:  []LogEntry{},
			expected: 0,
		},
		{
			entries: []LogEntry{
				{Line: "hello"},
			},
			expected: 5,
		},
		{
			entries: []LogEntry{
				{Line: "hello"},
				{Line: "world"},
			},
			expected: 10,
		},
		{
			entries: []LogEntry{
				{Line: "test message one"},
				{Line: "test message two"},
			},
			expected: 32,
		},
	}

	for _, tt := range tests {
		result := calculateBytes(tt.entries)
		if result != tt.expected {
			t.Errorf("calculateBytes() = %d, want %d", result, tt.expected)
		}
	}
}

// Tests for LokiConfig
func TestLokiConfig(t *testing.T) {
	config := &LokiConfig{
		Address:  "http://loki:3100",
		Username: "admin",
		Password: "secret",
		TenantID: "tenant-1",
		Timeout:  60 * time.Second,
	}

	if config.Address != "http://loki:3100" {
		t.Errorf("Expected Address 'http://loki:3100', got '%s'", config.Address)
	}

	if config.Username != "admin" {
		t.Errorf("Expected Username 'admin', got '%s'", config.Username)
	}

	if config.Password != "secret" {
		t.Errorf("Expected Password 'secret', got '%s'", config.Password)
	}

	if config.TenantID != "tenant-1" {
		t.Errorf("Expected TenantID 'tenant-1', got '%s'", config.TenantID)
	}

	if config.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", config.Timeout)
	}
}

// Tests for NewLokiQuerier
func TestNewLokiQuerier(t *testing.T) {
	querier := NewLokiQuerier("http://loki:3100")

	if querier == nil {
		t.Fatal("NewLokiQuerier returned nil")
	}

	if querier.config.Address != "http://loki:3100" {
		t.Errorf("Expected Address 'http://loki:3100', got '%s'", querier.config.Address)
	}

	if querier.config.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", querier.config.Timeout)
	}
}

// Tests for NewLokiQuerierWithConfig
func TestNewLokiQuerierWithConfig(t *testing.T) {
	config := &LokiConfig{
		Address:  "http://loki:3100",
		Username: "admin",
		Password: "secret",
		Timeout:  60 * time.Second,
	}

	querier := NewLokiQuerierWithConfig(config)

	if querier == nil {
		t.Fatal("NewLokiQuerierWithConfig returned nil")
	}

	if querier.config.Address != "http://loki:3100" {
		t.Errorf("Expected Address 'http://loki:3100', got '%s'", querier.config.Address)
	}

	if querier.config.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", querier.config.Timeout)
	}
}

// Tests for NewLokiQuerierWithConfig with zero timeout (uses default)
func TestNewLokiQuerierWithConfig_DefaultTimeout(t *testing.T) {
	config := &LokiConfig{
		Address: "http://loki:3100",
		// Timeout not set
	}

	querier := NewLokiQuerierWithConfig(config)

	if querier.config.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", querier.config.Timeout)
	}
}

// Tests for LogsResult struct
func TestLogsResult(t *testing.T) {
	now := time.Now()
	lr := &LogsResult{
		Entries: []LogEntry{
			{Timestamp: now, Line: "test log 1", Labels: map[string]string{"app": "test"}},
			{Timestamp: now.Add(-1 * time.Minute), Line: "test log 2"},
		},
		Stats: &LogsStats{
			Summary: LogsSummary{
				BytesProcessed:      100,
				LinesProcessed:      2,
				TotalBytesProcessed: 200,
				ExecTime:            0.01,
			},
		},
	}

	if len(lr.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(lr.Entries))
	}

	if lr.Stats == nil {
		t.Fatal("Expected Stats to be non-nil")
	}

	if lr.Stats.Summary.LinesProcessed != 2 {
		t.Errorf("Expected LinesProcessed 2, got %d", lr.Stats.Summary.LinesProcessed)
	}
}

// Test querier with empty query (returns all in range)
func TestLogsQuerierQueryEmptyQuery(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Log 1",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-3 * time.Minute),
		Line:      "Log 2",
	})

	ctx := context.Background()
	query := &LogsQuery{
		Query: "", // empty query should match all
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries with empty query, got %d", len(result.Entries))
	}
}

// Test default limit behavior
func TestLogsQuerierQueryDefaultLimit(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	// Add 150 entries (more than default limit of 100)
	for i := 0; i < 150; i++ {
		querier.AddEntry(LogEntry{
			Timestamp: now.Add(time.Duration(-i) * time.Second),
			Line:      "Test log",
		})
	}

	ctx := context.Background()
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-1 * time.Hour),
			End:   now,
		},
		// Limit not set, should default to 100
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Entries) != 100 {
		t.Errorf("Expected 100 entries (default limit), got %d", len(result.Entries))
	}
}

// Test default direction (backward)
func TestLogsQuerierQueryDefaultDirection(t *testing.T) {
	querier := NewInMemoryLogsQuerier()

	now := time.Now()

	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Older",
	})
	querier.AddEntry(LogEntry{
		Timestamp: now.Add(-1 * time.Minute),
		Line:      "Newer",
	})

	ctx := context.Background()
	query := &LogsQuery{
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
		// Direction not set, should default to "backward"
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// With backward direction, newest should be first
	if result.Entries[0].Line != "Newer" {
		t.Errorf("Expected first entry to be 'Newer' (backward direction), got '%s'", result.Entries[0].Line)
	}
}
