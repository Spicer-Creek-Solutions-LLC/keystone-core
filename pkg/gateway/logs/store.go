// Package logs provides the logs gateway for aggregating agent logs.
package logs

import (
	"strings"
	"sync"
	"time"
)

// LogLevel represents a log level.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of a log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel parses a log level string.
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// LogEntry represents a log entry.
type LogEntry struct {
	// ID is a unique identifier for this entry
	ID string

	// Timestamp is when the log was generated
	Timestamp time.Time

	// AgentID is the source agent
	AgentID string

	// Level is the log level
	Level LogLevel

	// Source is the log source (e.g., "agent", "state", "command")
	Source string

	// Message is the log message
	Message string

	// Labels are additional labels
	Labels map[string]string

	// Fields are structured fields
	Fields map[string]interface{}
}

// StoreConfig holds configuration for the logs store.
type StoreConfig struct {
	// MaxEntries is the maximum number of log entries to store
	MaxEntries int

	// MaxAge is the maximum age for log entries
	MaxAge time.Duration

	// MinLevel is the minimum log level to store
	MinLevel string

	// IncludeSources filters to only these sources
	IncludeSources []string

	// ExcludeSources filters out these sources
	ExcludeSources []string
}

// DefaultStoreConfig returns a configuration with sensible defaults.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxEntries: 100000,
		MaxAge:     1 * time.Hour,
		MinLevel:   "info",
	}
}

// LogsStore stores log entries from multiple agents.
type LogsStore struct {
	entries []LogEntry
	mu      sync.RWMutex
	config  StoreConfig

	// Parsed config
	minLevel       LogLevel
	includeSources map[string]bool
	excludeSources map[string]bool

	// Callbacks
	onEntryAdded func(entry *LogEntry)

	// Stats
	entriesReceived int64
	entriesDropped  int64
}

// NewLogsStore creates a new logs store.
func NewLogsStore(config StoreConfig) *LogsStore {
	s := &LogsStore{
		entries: make([]LogEntry, 0, config.MaxEntries),
		config:  config,
	}

	s.minLevel = ParseLevel(config.MinLevel)

	if len(config.IncludeSources) > 0 {
		s.includeSources = make(map[string]bool)
		for _, src := range config.IncludeSources {
			s.includeSources[src] = true
		}
	}

	if len(config.ExcludeSources) > 0 {
		s.excludeSources = make(map[string]bool)
		for _, src := range config.ExcludeSources {
			s.excludeSources[src] = true
		}
	}

	return s
}

// SetOnEntryAdded sets the callback for when an entry is added.
func (s *LogsStore) SetOnEntryAdded(fn func(entry *LogEntry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEntryAdded = fn
}

// Store stores a log entry.
func (s *LogsStore) Store(entry LogEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entriesReceived++

	// Check level
	if entry.Level < s.minLevel {
		s.entriesDropped++
		return false
	}

	// Check source filters
	if s.includeSources != nil && !s.includeSources[entry.Source] {
		s.entriesDropped++
		return false
	}
	if s.excludeSources != nil && s.excludeSources[entry.Source] {
		s.entriesDropped++
		return false
	}

	// Evict oldest if at capacity
	if len(s.entries) >= s.config.MaxEntries {
		s.entries = s.entries[1:]
	}

	s.entries = append(s.entries, entry)

	// Notify callback
	if s.onEntryAdded != nil {
		go s.onEntryAdded(&entry)
	}

	return true
}

// StoreBatch stores multiple log entries.
func (s *LogsStore) StoreBatch(entries []LogEntry) int {
	stored := 0
	for _, entry := range entries {
		if s.Store(entry) {
			stored++
		}
	}
	return stored
}

// Query queries log entries.
type LogQuery struct {
	// AgentID filters by agent
	AgentID string

	// Source filters by source
	Source string

	// Level filters by minimum level
	Level LogLevel

	// Start is the start time
	Start time.Time

	// End is the end time
	End time.Time

	// Limit is the maximum number of entries to return
	Limit int

	// Labels filters by label values
	Labels map[string]string

	// Search is a text search in the message
	Search string
}

// Query returns log entries matching the query.
func (s *LogsStore) Query(q LogQuery) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []LogEntry

	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]

		// Apply filters
		if q.AgentID != "" && entry.AgentID != q.AgentID {
			continue
		}
		if q.Source != "" && entry.Source != q.Source {
			continue
		}
		if entry.Level < q.Level {
			continue
		}
		if !q.Start.IsZero() && entry.Timestamp.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && entry.Timestamp.After(q.End) {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(q.Search)) {
			continue
		}

		// Check labels
		if len(q.Labels) > 0 {
			match := true
			for k, v := range q.Labels {
				if entry.Labels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		result = append(result, entry)

		if q.Limit > 0 && len(result) >= q.Limit {
			break
		}
	}

	return result
}

// GetRecent returns the most recent entries.
func (s *LogsStore) GetRecent(limit int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := 0
	if len(s.entries) > limit {
		start = len(s.entries) - limit
	}

	result := make([]LogEntry, len(s.entries)-start)
	copy(result, s.entries[start:])

	// Reverse to get newest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// GetPending returns entries that haven't been pushed yet.
// Used by Loki pusher to get entries for pushing.
func (s *LogsStore) GetPending(lastID string, limit int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []LogEntry
	found := lastID == ""

	for _, entry := range s.entries {
		if !found {
			if entry.ID == lastID {
				found = true
			}
			continue
		}

		result = append(result, entry)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// Cleanup removes old entries.
func (s *LogsStore) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.MaxAge <= 0 {
		return 0
	}

	cutoff := time.Now().Add(-s.config.MaxAge)
	removed := 0

	// Find first entry that's not expired
	firstValid := 0
	for i, entry := range s.entries {
		if entry.Timestamp.After(cutoff) {
			firstValid = i
			break
		}
		removed++
	}

	if removed > 0 {
		s.entries = s.entries[firstValid:]
	}

	return removed
}

// Clear removes all entries.
func (s *LogsStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = s.entries[:0]
}

// LogsStoreStats holds store statistics.
type LogsStoreStats struct {
	EntryCount      int
	EntriesReceived int64
	EntriesDropped  int64
}

// Stats returns store statistics.
func (s *LogsStore) Stats() LogsStoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return LogsStoreStats{
		EntryCount:      len(s.entries),
		EntriesReceived: s.entriesReceived,
		EntriesDropped:  s.entriesDropped,
	}
}
