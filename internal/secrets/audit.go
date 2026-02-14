package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// AuditAction constants for secret operations.
const (
	AuditActionRead             = "read"
	AuditActionReadFailed       = "read_failed"
	AuditActionReadDynamic      = "read_dynamic"
	AuditActionList             = "list"
	AuditActionCacheHit         = "cache_hit"
	AuditActionCacheMiss        = "cache_miss"
	AuditActionCacheEvict       = "cache_evict"
	AuditActionLeaseCreate      = "lease_create"
	AuditActionLeaseRenew       = "lease_renew"
	AuditActionLeaseRenewFailed = "lease_renew_failed"
	AuditActionLeaseRevoke      = "lease_revoke"
	AuditActionLeaseExpire      = "lease_expire"
	AuditActionRotationStart    = "rotation_start"
	AuditActionRotationComplete = "rotation_complete"
	AuditActionRotationFailed   = "rotation_failed"
	AuditActionRotationRollback = "rotation_rollback"
	AuditActionTransitEncrypt   = "transit_encrypt"
	AuditActionTransitDecrypt   = "transit_decrypt"
	AuditActionTransitSign      = "transit_sign"
	AuditActionTransitVerify    = "transit_verify"
	AuditActionAccessDenied     = "access_denied"
)

// SecretAccessEvent represents a secret access audit event.
type SecretAccessEvent struct {
	// SecretPath is the path of the secret accessed.
	SecretPath string `json:"secret_path"`

	// Backend is the backend that served the request.
	Backend string `json:"backend,omitempty"`

	// SecretType is the type of secret accessed.
	SecretType string `json:"secret_type,omitempty"`

	// AgentID is the ID of the agent that accessed the secret.
	AgentID string `json:"agent_id,omitempty"`

	// SPIFFEID is the SPIFFE identity of the requester.
	SPIFFEID string `json:"spiffe_id,omitempty"`

	// RequestID is the unique request ID.
	RequestID string `json:"request_id,omitempty"`

	// LeaseID is the lease ID for dynamic secrets.
	LeaseID string `json:"lease_id,omitempty"`

	// Action is the action performed.
	Action string `json:"action"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Duration is how long the operation took.
	Duration time.Duration `json:"duration,omitempty"`

	// CacheHit indicates if the request was served from cache.
	CacheHit bool `json:"cache_hit,omitempty"`

	// Success indicates if the operation was successful.
	Success bool `json:"success"`

	// Error is the error message if the operation failed.
	Error string `json:"error,omitempty"`

	// SourceIP is the source IP of the request.
	SourceIP string `json:"source_ip,omitempty"`

	// Extra contains additional event-specific data.
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// MarshalJSON implements json.Marshaler for SecretAccessEvent.
func (e *SecretAccessEvent) MarshalJSON() ([]byte, error) {
	type Alias SecretAccessEvent
	return json.Marshal(&struct {
		*Alias
		DurationMS int64 `json:"duration_ms,omitempty"`
	}{
		Alias:      (*Alias)(e),
		DurationMS: e.Duration.Milliseconds(),
	})
}

// SecretAuditLogger logs secret access events.
type SecretAuditLogger interface {
	// LogSecretAccess logs a secret access event.
	LogSecretAccess(ctx context.Context, event *SecretAccessEvent) error
}

// InMemorySecretAuditLogger stores audit events in memory.
type InMemorySecretAuditLogger struct {
	mu       sync.RWMutex
	events   []*SecretAccessEvent
	maxSize  int
	callback func(*SecretAccessEvent)
}

// InMemoryAuditLoggerConfig configures the in-memory audit logger.
type InMemoryAuditLoggerConfig struct {
	// MaxSize is the maximum number of events to store.
	MaxSize int

	// Callback is called for each event (optional).
	Callback func(*SecretAccessEvent)
}

// NewInMemorySecretAuditLogger creates a new in-memory audit logger.
func NewInMemorySecretAuditLogger(config *InMemoryAuditLoggerConfig) *InMemorySecretAuditLogger {
	if config == nil {
		config = &InMemoryAuditLoggerConfig{}
	}

	maxSize := config.MaxSize
	if maxSize == 0 {
		maxSize = 10000
	}

	return &InMemorySecretAuditLogger{
		events:   make([]*SecretAccessEvent, 0),
		maxSize:  maxSize,
		callback: config.Callback,
	}
}

// LogSecretAccess logs a secret access event.
func (l *InMemorySecretAuditLogger) LogSecretAccess(ctx context.Context, event *SecretAccessEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Enforce max size (ring buffer behavior)
	if len(l.events) >= l.maxSize {
		l.events = l.events[1:]
	}

	l.events = append(l.events, event)

	// Call callback if set
	if l.callback != nil {
		l.callback(event)
	}

	return nil
}

// GetEvents returns all stored events.
func (l *InMemorySecretAuditLogger) GetEvents() []*SecretAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*SecretAccessEvent, len(l.events))
	copy(result, l.events)
	return result
}

// GetEventsByPath returns events for a specific secret path.
func (l *InMemorySecretAuditLogger) GetEventsByPath(path string) []*SecretAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*SecretAccessEvent
	for _, event := range l.events {
		if event.SecretPath == path {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByAgent returns events for a specific agent.
func (l *InMemorySecretAuditLogger) GetEventsByAgent(agentID string) []*SecretAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*SecretAccessEvent
	for _, event := range l.events {
		if event.AgentID == agentID {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByTimeRange returns events within a time range.
func (l *InMemorySecretAuditLogger) GetEventsByTimeRange(start, end time.Time) []*SecretAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*SecretAccessEvent
	for _, event := range l.events {
		if (event.Timestamp.Equal(start) || event.Timestamp.After(start)) &&
			(event.Timestamp.Equal(end) || event.Timestamp.Before(end)) {
			result = append(result, event)
		}
	}
	return result
}

// GetEventCount returns the number of stored events.
func (l *InMemorySecretAuditLogger) GetEventCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}

// Clear removes all stored events.
func (l *InMemorySecretAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = make([]*SecretAccessEvent, 0)
}

// Ensure InMemorySecretAuditLogger implements SecretAuditLogger.
var _ SecretAuditLogger = (*InMemorySecretAuditLogger)(nil)

// NoopSecretAuditLogger is an audit logger that does nothing.
type NoopSecretAuditLogger struct{}

// LogSecretAccess does nothing.
func (l *NoopSecretAuditLogger) LogSecretAccess(ctx context.Context, event *SecretAccessEvent) error {
	return nil
}

// Ensure NoopSecretAuditLogger implements SecretAuditLogger.
var _ SecretAuditLogger = (*NoopSecretAuditLogger)(nil)

// MultiSecretAuditLogger logs to multiple loggers.
type MultiSecretAuditLogger struct {
	loggers []SecretAuditLogger
}

// NewMultiSecretAuditLogger creates a new multi audit logger.
func NewMultiSecretAuditLogger(loggers ...SecretAuditLogger) *MultiSecretAuditLogger {
	return &MultiSecretAuditLogger{loggers: loggers}
}

// LogSecretAccess logs to all configured loggers.
func (l *MultiSecretAuditLogger) LogSecretAccess(ctx context.Context, event *SecretAccessEvent) error {
	var lastErr error
	for _, logger := range l.loggers {
		if err := logger.LogSecretAccess(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Ensure MultiSecretAuditLogger implements SecretAuditLogger.
var _ SecretAuditLogger = (*MultiSecretAuditLogger)(nil)

// SecretAuditSummary contains aggregate statistics for audit events.
type SecretAuditSummary struct {
	// TotalEvents is the total number of events.
	TotalEvents int64 `json:"total_events"`

	// SuccessCount is the number of successful operations.
	SuccessCount int64 `json:"success_count"`

	// FailureCount is the number of failed operations.
	FailureCount int64 `json:"failure_count"`

	// CacheHits is the number of cache hits.
	CacheHits int64 `json:"cache_hits"`

	// CacheMisses is the number of cache misses.
	CacheMisses int64 `json:"cache_misses"`

	// EventsByAction is the count of events by action.
	EventsByAction map[string]int64 `json:"events_by_action"`

	// EventsByBackend is the count of events by backend.
	EventsByBackend map[string]int64 `json:"events_by_backend"`

	// EventsByAgent is the count of events by agent.
	EventsByAgent map[string]int64 `json:"events_by_agent"`

	// TopSecrets is the most accessed secrets.
	TopSecrets map[string]int64 `json:"top_secrets"`

	// AverageDuration is the average operation duration.
	AverageDuration time.Duration `json:"average_duration"`

	// OldestEvent is the timestamp of the oldest event.
	OldestEvent time.Time `json:"oldest_event"`

	// NewestEvent is the timestamp of the newest event.
	NewestEvent time.Time `json:"newest_event"`
}

// GetSummary returns aggregate statistics for the stored events.
func (l *InMemorySecretAuditLogger) GetSummary() *SecretAuditSummary {
	l.mu.RLock()
	defer l.mu.RUnlock()

	summary := &SecretAuditSummary{
		EventsByAction:  make(map[string]int64),
		EventsByBackend: make(map[string]int64),
		EventsByAgent:   make(map[string]int64),
		TopSecrets:      make(map[string]int64),
	}

	if len(l.events) == 0 {
		return summary
	}

	var totalDuration time.Duration
	var durationCount int64

	for i, event := range l.events {
		summary.TotalEvents++

		if event.Success {
			summary.SuccessCount++
		} else {
			summary.FailureCount++
		}

		if event.CacheHit {
			summary.CacheHits++
		} else if event.Action == AuditActionRead || event.Action == AuditActionReadDynamic {
			summary.CacheMisses++
		}

		summary.EventsByAction[event.Action]++

		if event.Backend != "" {
			summary.EventsByBackend[event.Backend]++
		}

		if event.AgentID != "" {
			summary.EventsByAgent[event.AgentID]++
		}

		if event.SecretPath != "" {
			summary.TopSecrets[event.SecretPath]++
		}

		if event.Duration > 0 {
			totalDuration += event.Duration
			durationCount++
		}

		if i == 0 {
			summary.OldestEvent = event.Timestamp
			summary.NewestEvent = event.Timestamp
		} else {
			if event.Timestamp.Before(summary.OldestEvent) {
				summary.OldestEvent = event.Timestamp
			}
			if event.Timestamp.After(summary.NewestEvent) {
				summary.NewestEvent = event.Timestamp
			}
		}
	}

	if durationCount > 0 {
		summary.AverageDuration = totalDuration / time.Duration(durationCount)
	}

	return summary
}

// SecretAuditFilter filters audit events.
type SecretAuditFilter struct {
	// SecretPath filters by secret path.
	SecretPath string

	// Backend filters by backend.
	Backend string

	// AgentID filters by agent ID.
	AgentID string

	// SPIFFEID filters by SPIFFE ID.
	SPIFFEID string

	// Action filters by action.
	Action string

	// SuccessOnly filters to only successful events.
	SuccessOnly bool

	// FailureOnly filters to only failed events.
	FailureOnly bool

	// CacheHitOnly filters to only cache hits.
	CacheHitOnly bool

	// CacheMissOnly filters to only cache misses.
	CacheMissOnly bool

	// StartTime filters events after this time.
	StartTime time.Time

	// EndTime filters events before this time.
	EndTime time.Time

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int
}

// GetEventsFiltered returns events matching the filter.
func (l *InMemorySecretAuditLogger) GetEventsFiltered(filter *SecretAuditFilter) []*SecretAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*SecretAccessEvent
	skipped := 0

	for _, event := range l.events {
		// Apply filters
		if filter.SecretPath != "" && event.SecretPath != filter.SecretPath {
			continue
		}
		if filter.Backend != "" && event.Backend != filter.Backend {
			continue
		}
		if filter.AgentID != "" && event.AgentID != filter.AgentID {
			continue
		}
		if filter.SPIFFEID != "" && event.SPIFFEID != filter.SPIFFEID {
			continue
		}
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.SuccessOnly && !event.Success {
			continue
		}
		if filter.FailureOnly && event.Success {
			continue
		}
		if filter.CacheHitOnly && !event.CacheHit {
			continue
		}
		if filter.CacheMissOnly && event.CacheHit {
			continue
		}
		if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
			continue
		}

		// Handle offset
		if filter.Offset > 0 && skipped < filter.Offset {
			skipped++
			continue
		}

		result = append(result, event)

		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result
}

// ChannelSecretAuditLogger sends audit events to a channel.
type ChannelSecretAuditLogger struct {
	ch            chan *SecretAccessEvent
	eventsDropped atomic.Int64
}

// NewChannelSecretAuditLogger creates a new channel-based audit logger.
func NewChannelSecretAuditLogger(bufferSize int) *ChannelSecretAuditLogger {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &ChannelSecretAuditLogger{
		ch: make(chan *SecretAccessEvent, bufferSize),
	}
}

// LogSecretAccess sends an event to the channel.
func (l *ChannelSecretAuditLogger) LogSecretAccess(ctx context.Context, event *SecretAccessEvent) error {
	select {
	case l.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		l.eventsDropped.Add(1)
		return fmt.Errorf("audit event dropped: channel full")
	}
}

// EventsDropped returns the number of events dropped due to a full channel.
func (l *ChannelSecretAuditLogger) EventsDropped() int64 {
	return l.eventsDropped.Load()
}

// Events returns the event channel for consumption.
func (l *ChannelSecretAuditLogger) Events() <-chan *SecretAccessEvent {
	return l.ch
}

// Close closes the channel.
func (l *ChannelSecretAuditLogger) Close() {
	close(l.ch)
}

// Ensure ChannelSecretAuditLogger implements SecretAuditLogger.
var _ SecretAuditLogger = (*ChannelSecretAuditLogger)(nil)
