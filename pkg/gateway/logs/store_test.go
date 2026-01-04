package logs

import (
	"testing"
	"time"
)

func TestLogsStore_Store(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewLogsStore(config)

	entry := LogEntry{
		ID:        "1",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		Level:     LevelInfo,
		Source:    "test",
		Message:   "Hello, World!",
		Labels:    map[string]string{"key": "value"},
	}

	stored := store.Store(entry)
	if !stored {
		t.Error("Store() returned false, want true")
	}

	stats := store.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("EntryCount = %d, want 1", stats.EntryCount)
	}
}

func TestLogsStore_LevelFilter(t *testing.T) {
	config := StoreConfig{
		MaxEntries: 1000,
		MinLevel:   "warn",
	}
	store := NewLogsStore(config)

	// Debug entry should be dropped
	store.Store(LogEntry{
		ID:        "1",
		Timestamp: time.Now(),
		Level:     LevelDebug,
		Message:   "Debug message",
	})

	// Info entry should be dropped
	store.Store(LogEntry{
		ID:        "2",
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   "Info message",
	})

	// Warn entry should be stored
	store.Store(LogEntry{
		ID:        "3",
		Timestamp: time.Now(),
		Level:     LevelWarn,
		Message:   "Warn message",
	})

	stats := store.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("EntryCount = %d, want 1", stats.EntryCount)
	}
	if stats.EntriesDropped != 2 {
		t.Errorf("EntriesDropped = %d, want 2", stats.EntriesDropped)
	}
}

func TestLogsStore_SourceFilter(t *testing.T) {
	config := StoreConfig{
		MaxEntries:     1000,
		IncludeSources: []string{"app", "system"},
	}
	store := NewLogsStore(config)

	// Should be stored (in include list)
	store.Store(LogEntry{
		ID:     "1",
		Source: "app",
		Level:  LevelInfo,
	})

	// Should be dropped (not in include list)
	store.Store(LogEntry{
		ID:     "2",
		Source: "network",
		Level:  LevelInfo,
	})

	stats := store.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("EntryCount = %d, want 1", stats.EntryCount)
	}
}

func TestLogsStore_Query(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewLogsStore(config)

	now := time.Now()

	store.Store(LogEntry{ID: "1", Timestamp: now.Add(-2 * time.Minute), AgentID: "agent-1", Level: LevelInfo, Source: "app"})
	store.Store(LogEntry{ID: "2", Timestamp: now.Add(-1 * time.Minute), AgentID: "agent-1", Level: LevelError, Source: "app"})
	store.Store(LogEntry{ID: "3", Timestamp: now, AgentID: "agent-2", Level: LevelInfo, Source: "system"})

	// Query by agent
	results := store.Query(LogQuery{AgentID: "agent-1"})
	if len(results) != 2 {
		t.Errorf("Query by agent: got %d, want 2", len(results))
	}

	// Query by source
	results = store.Query(LogQuery{Source: "app"})
	if len(results) != 2 {
		t.Errorf("Query by source: got %d, want 2", len(results))
	}

	// Query by level
	results = store.Query(LogQuery{Level: LevelError})
	if len(results) != 1 {
		t.Errorf("Query by level: got %d, want 1", len(results))
	}

	// Query with limit
	results = store.Query(LogQuery{Limit: 2})
	if len(results) != 2 {
		t.Errorf("Query with limit: got %d, want 2", len(results))
	}
}

func TestLogsStore_GetRecent(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewLogsStore(config)

	for i := 0; i < 10; i++ {
		store.Store(LogEntry{
			ID:        string(rune('0' + i)),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Level:     LevelInfo,
		})
	}

	recent := store.GetRecent(5)
	if len(recent) != 5 {
		t.Errorf("GetRecent(5) returned %d entries, want 5", len(recent))
	}

	// Should be in reverse order (newest first)
	if recent[0].ID != "9" {
		t.Errorf("GetRecent[0].ID = %s, want 9", recent[0].ID)
	}
}

func TestLogsStore_Cleanup(t *testing.T) {
	config := StoreConfig{
		MaxEntries: 1000,
		MaxAge:     100 * time.Millisecond,
	}
	store := NewLogsStore(config)

	store.Store(LogEntry{
		ID:        "1",
		Timestamp: time.Now().Add(-200 * time.Millisecond),
		Level:     LevelInfo,
	})
	store.Store(LogEntry{
		ID:        "2",
		Timestamp: time.Now(),
		Level:     LevelInfo,
	})

	removed := store.Cleanup()
	if removed != 1 {
		t.Errorf("Cleanup() removed %d, want 1", removed)
	}

	stats := store.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("EntryCount after Cleanup = %d, want 1", stats.EntryCount)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo}, // default
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%s) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
	}

	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("LogLevel(%d).String() = %s, want %s", tt.level, got, tt.want)
		}
	}
}
