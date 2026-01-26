package events

import (
	"context"
	"time"
)

// EventStore defines the interface for event persistence
type EventStore interface {
	// Store stores an event
	Store(ctx context.Context, event *Event) error

	// StoreBatch stores multiple events
	StoreBatch(ctx context.Context, events []*Event) error

	// Get retrieves an event by ID
	Get(ctx context.Context, id string) (*Event, error)

	// Query queries events with filters
	Query(ctx context.Context, query *EventQuery) (*EventQueryResult, error)

	// Delete deletes an event by ID
	Delete(ctx context.Context, id string) error

	// DeleteBatch deletes multiple events
	DeleteBatch(ctx context.Context, ids []string) error

	// Count returns the total number of events matching the query
	Count(ctx context.Context, query *EventQuery) (int64, error)

	// ApplyRetention applies retention policy and returns number of deleted events
	ApplyRetention(ctx context.Context, policy *RetentionPolicy) (int64, error)

	// Close closes the store
	Close() error
}

// EventQuery defines query parameters for searching events
type EventQuery struct {
	// Filter criteria
	Types         []EventType
	Sources       []string
	Severities    []Severity
	Tags          map[string]string
	CorrelationID string

	// Time range
	StartTime *time.Time
	EndTime   *time.Time

	// Sorting
	SortBy    string // "time", "type", "severity"
	SortOrder string // "asc", "desc"

	// Pagination
	Limit  int
	Offset int
}

// EventQueryResult holds query results
type EventQueryResult struct {
	Events     []*Event
	TotalCount int64
	Offset     int
	Limit      int
}

// RetentionPolicy defines event retention rules
type RetentionPolicy struct {
	// MaxAge is the maximum age of events to keep
	MaxAge time.Duration

	// MaxCount is the maximum number of events to keep
	MaxCount int64

	// MinSeverity is the minimum severity to keep (events below this are deleted)
	MinSeverity Severity

	// TypeFilters defines per-type retention
	TypeFilters map[EventType]*TypeRetention
}

// TypeRetention defines retention for a specific event type
type TypeRetention struct {
	MaxAge   time.Duration
	MaxCount int64
}

// EventStoreConfig holds configuration for event storage
type EventStoreConfig struct {
	// Database path (for SQLite)
	Path string

	// Connection pool settings
	MaxOpenConns int
	MaxIdleConns int

	// Batch settings
	BatchSize     int
	BatchInterval time.Duration

	// Indexing
	EnableFullTextSearch bool
	IndexTags            bool
	IndexSeverity        bool

	// Auto-retention
	AutoRetention        bool
	RetentionCheckInterval time.Duration
	DefaultRetentionPolicy *RetentionPolicy
}

// DefaultEventStoreConfig returns default configuration
func DefaultEventStoreConfig() *EventStoreConfig {
	return &EventStoreConfig{
		Path:                   "events.db",
		MaxOpenConns:           10,
		MaxIdleConns:           5,
		BatchSize:              100,
		BatchInterval:          1 * time.Second,
		EnableFullTextSearch:   false,
		IndexTags:              true,
		IndexSeverity:          true,
		AutoRetention:          true,
		RetentionCheckInterval: 1 * time.Hour,
		DefaultRetentionPolicy: &RetentionPolicy{
			MaxAge:      30 * 24 * time.Hour, // 30 days
			MaxCount:    1000000,              // 1 million events
			MinSeverity: SeverityDebug,        // Keep all severities
		},
	}
}

// EventReplay provides event replay capabilities
type EventReplay interface {
	// Replay replays events matching the query
	Replay(ctx context.Context, query *EventQuery, handler EventHandler) error

	// ReplayFrom replays events from a specific time
	ReplayFrom(ctx context.Context, startTime time.Time, handler EventHandler) error

	// ReplayRange replays events within a time range
	ReplayRange(ctx context.Context, startTime, endTime time.Time, handler EventHandler) error
}

// ReplayProgress tracks replay progress
type ReplayProgress struct {
	// Current is the number of events processed so far
	Current int64
	// Total is the total number of events to replay
	Total int64
	// LastEvent is the most recently processed event
	LastEvent *Event
	// Percentage is the completion percentage (0-100)
	Percentage float64
}

// ReplayProgressFunc is a callback for reporting replay progress
type ReplayProgressFunc func(progress *ReplayProgress)

// BatchEventHandler processes a batch of events
type BatchEventHandler func(events []*Event) error

// EventReplayExtended provides extended replay capabilities
type EventReplayExtended interface {
	EventReplay

	// ReplayWithProgress replays events with progress tracking
	ReplayWithProgress(ctx context.Context, query *EventQuery, handler EventHandler, progressFn ReplayProgressFunc) error

	// ReplayBatched replays events in batches for memory efficiency
	ReplayBatched(ctx context.Context, query *EventQuery, batchSize int, handler BatchEventHandler) error

	// ReplayRangeWithTypes replays specific event types within a time range
	ReplayRangeWithTypes(ctx context.Context, startTime, endTime time.Time, types []EventType, handler EventHandler) error
}

// EventArchive provides event archiving capabilities
type EventArchive interface {
	// Archive archives events to external storage
	Archive(ctx context.Context, query *EventQuery, destination string) error

	// Restore restores events from archive
	Restore(ctx context.Context, source string) (int64, error)
}

// StorageMetrics tracks storage statistics
type StorageMetrics struct {
	TotalEvents      int64
	EventsByType     map[EventType]int64
	EventsBySeverity map[Severity]int64
	OldestEvent      time.Time
	NewestEvent      time.Time
	DatabaseSize     int64
}

// NewEventQuery creates a new event query with defaults
func NewEventQuery() *EventQuery {
	return &EventQuery{
		SortBy:    "time",
		SortOrder: "desc",
		Limit:     100,
		Offset:    0,
	}
}

// WithTypes adds type filter to query
func (q *EventQuery) WithTypes(types ...EventType) *EventQuery {
	q.Types = types
	return q
}

// WithSources adds source filter to query
func (q *EventQuery) WithSources(sources ...string) *EventQuery {
	q.Sources = sources
	return q
}

// WithSeverities adds severity filter to query
func (q *EventQuery) WithSeverities(severities ...Severity) *EventQuery {
	q.Severities = severities
	return q
}

// WithTimeRange adds time range filter to query
func (q *EventQuery) WithTimeRange(start, end time.Time) *EventQuery {
	q.StartTime = &start
	q.EndTime = &end
	return q
}

// WithTag adds tag filter to query
func (q *EventQuery) WithTag(key, value string) *EventQuery {
	if q.Tags == nil {
		q.Tags = make(map[string]string)
	}
	q.Tags[key] = value
	return q
}

// WithCorrelationID adds correlation ID filter to query
func (q *EventQuery) WithCorrelationID(id string) *EventQuery {
	q.CorrelationID = id
	return q
}

// WithPagination sets pagination parameters
func (q *EventQuery) WithPagination(limit, offset int) *EventQuery {
	q.Limit = limit
	q.Offset = offset
	return q
}

// WithSort sets sorting parameters
func (q *EventQuery) WithSort(sortBy, sortOrder string) *EventQuery {
	q.SortBy = sortBy
	q.SortOrder = sortOrder
	return q
}

// DefaultRetentionPolicy returns a default retention policy
func DefaultRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{
		MaxAge:      30 * 24 * time.Hour, // 30 days
		MaxCount:    1000000,              // 1 million events
		MinSeverity: SeverityDebug,
	}
}

// WithMaxAge sets maximum age for retention
func (p *RetentionPolicy) WithMaxAge(age time.Duration) *RetentionPolicy {
	p.MaxAge = age
	return p
}

// WithMaxCount sets maximum count for retention
func (p *RetentionPolicy) WithMaxCount(count int64) *RetentionPolicy {
	p.MaxCount = count
	return p
}

// WithMinSeverity sets minimum severity for retention
func (p *RetentionPolicy) WithMinSeverity(severity Severity) *RetentionPolicy {
	p.MinSeverity = severity
	return p
}

// WithTypeRetention adds per-type retention
func (p *RetentionPolicy) WithTypeRetention(eventType EventType, retention *TypeRetention) *RetentionPolicy {
	if p.TypeFilters == nil {
		p.TypeFilters = make(map[EventType]*TypeRetention)
	}
	p.TypeFilters[eventType] = retention
	return p
}
