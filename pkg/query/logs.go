package query

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// InMemoryLogsQuerier is a simple in-memory logs querier for testing
// In production, this would be replaced with a Loki client or similar
type InMemoryLogsQuerier struct {
	entries []LogEntry
	mu      sync.RWMutex
}

// NewInMemoryLogsQuerier creates a new in-memory logs querier
func NewInMemoryLogsQuerier() *InMemoryLogsQuerier {
	return &InMemoryLogsQuerier{
		entries: make([]LogEntry, 0),
	}
}

// AddEntry adds a log entry (for testing)
func (l *InMemoryLogsQuerier) AddEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

// Query executes a logs query
func (l *InMemoryLogsQuerier) Query(ctx context.Context, query *LogsQuery) (*LogsResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	startTime := time.Now()

	// Filter entries by time range
	filtered := make([]LogEntry, 0)
	for _, entry := range l.entries {
		if entry.Timestamp.After(query.Range.Start) && entry.Timestamp.Before(query.Range.End) {
			// Simple query matching - check if query string is in the line
			if query.Query == "" || containsQuery(entry, query.Query) {
				filtered = append(filtered, entry)
			}
		}
	}

	// Sort by direction
	direction := query.Direction
	if direction == "" {
		direction = "backward" // Default to most recent first
	}

	if direction == "backward" {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.After(filtered[j].Timestamp)
		})
	} else {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.Before(filtered[j].Timestamp)
		})
	}

	// Apply limit
	limit := query.Limit
	if limit == 0 {
		limit = 100 // Default limit
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	execTime := time.Since(startTime).Seconds()

	return &LogsResult{
		Entries: filtered,
		Stats: &LogsStats{
			Summary: LogsSummary{
				BytesProcessed:      calculateBytes(filtered),
				LinesProcessed:      int64(len(filtered)),
				TotalBytesProcessed: calculateBytes(l.entries),
				ExecTime:            execTime,
			},
		},
	}, nil
}

// containsQuery checks if a log entry matches the query
// This is a simple implementation - a real implementation would use LogQL
func containsQuery(entry LogEntry, query string) bool {
	// Check if query is in the line
	if len(entry.Line) > 0 && contains(entry.Line, query) {
		return true
	}

	// Check labels
	for k, v := range entry.Labels {
		if contains(k, query) || contains(v, query) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	// Simple case-insensitive substring match
	return findSubstring(toLowerCase(s), toLowerCase(substr))
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLowerCase(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

func calculateBytes(entries []LogEntry) int64 {
	var total int64
	for _, entry := range entries {
		total += int64(len(entry.Line))
	}
	return total
}

// LokiQuerier queries logs from Grafana Loki
type LokiQuerier struct {
	address string
	// In a real implementation, this would have a Loki client
}

// NewLokiQuerier creates a new Loki logs querier
func NewLokiQuerier(address string) *LokiQuerier {
	return &LokiQuerier{
		address: address,
	}
}

// Query executes a logs query against Loki
func (l *LokiQuerier) Query(ctx context.Context, query *LogsQuery) (*LogsResult, error) {
	// This is a placeholder - a real implementation would use the Loki HTTP API
	return nil, fmt.Errorf("Loki querier not implemented - use in-memory querier for testing")
}
