// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"context"
	"sync"
	"time"
)

// AuditLogger logs credential access events.
type AuditLogger interface {
	// LogCredentialAccess logs a credential access event.
	LogCredentialAccess(ctx context.Context, event *CredentialAccessEvent) error
}

// CredentialAccessEvent represents a credential access audit event.
type CredentialAccessEvent struct {
	// CredentialRef is the credential reference that was accessed.
	CredentialRef string `json:"credential_ref"`
	// CredentialID is the resolved credential ID.
	CredentialID string `json:"credential_id,omitempty"`
	// CredentialType is the type of credential accessed.
	CredentialType CredentialType `json:"credential_type,omitempty"`
	// ProxyAgentID is the ID of the proxy agent that accessed the credential.
	ProxyAgentID string `json:"proxy_agent_id"`
	// DeviceID is the ID of the device the credential was used for.
	DeviceID string `json:"device_id,omitempty"`
	// RequestID is the unique request ID.
	RequestID string `json:"request_id"`
	// Action is the action performed.
	Action string `json:"action"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Duration is how long the operation took.
	Duration time.Duration `json:"duration,omitempty"`
	// Success indicates if the operation was successful.
	Success bool `json:"success"`
	// Error is the error message if the operation failed.
	Error string `json:"error,omitempty"`
	// SourceIP is the source IP of the request.
	SourceIP string `json:"source_ip,omitempty"`
	// Extra contains additional event-specific data.
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// AuditAction constants for credential operations.
const (
	AuditActionFetch         = "fetch"
	AuditActionFetchSuccess  = "fetch_success"
	AuditActionFetchFailed   = "fetch_failed"
	AuditActionStore         = "store"
	AuditActionDelete        = "delete"
	AuditActionRotate        = "rotate"
	AuditActionCacheHit      = "cache_hit"
	AuditActionCacheMiss     = "cache_miss"
	AuditActionCacheEvict    = "cache_evict"
	AuditActionValidate      = "validate"
	AuditActionDecrypt       = "decrypt"
	AuditActionDecryptFailed = "decrypt_failed"
)

// InMemoryAuditLogger stores audit events in memory.
type InMemoryAuditLogger struct {
	mu       sync.RWMutex
	events   []*CredentialAccessEvent
	maxSize  int
	callback func(*CredentialAccessEvent)
}

// InMemoryAuditLoggerConfig configures the in-memory audit logger.
type InMemoryAuditLoggerConfig struct {
	// MaxSize is the maximum number of events to store.
	MaxSize int
	// Callback is called for each event (optional).
	Callback func(*CredentialAccessEvent)
}

// NewInMemoryAuditLogger creates a new in-memory audit logger.
func NewInMemoryAuditLogger(config *InMemoryAuditLoggerConfig) *InMemoryAuditLogger {
	if config == nil {
		config = &InMemoryAuditLoggerConfig{}
	}
	maxSize := config.MaxSize
	if maxSize == 0 {
		maxSize = 10000
	}
	return &InMemoryAuditLogger{
		events:   make([]*CredentialAccessEvent, 0),
		maxSize:  maxSize,
		callback: config.Callback,
	}
}

// LogCredentialAccess logs a credential access event.
func (l *InMemoryAuditLogger) LogCredentialAccess(ctx context.Context, event *CredentialAccessEvent) error {
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
func (l *InMemoryAuditLogger) GetEvents() []*CredentialAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*CredentialAccessEvent, len(l.events))
	copy(result, l.events)
	return result
}

// GetEventsByCredential returns events for a specific credential.
func (l *InMemoryAuditLogger) GetEventsByCredential(credentialRef string) []*CredentialAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*CredentialAccessEvent
	for _, event := range l.events {
		if event.CredentialRef == credentialRef {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByProxyAgent returns events for a specific proxy agent.
func (l *InMemoryAuditLogger) GetEventsByProxyAgent(proxyAgentID string) []*CredentialAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*CredentialAccessEvent
	for _, event := range l.events {
		if event.ProxyAgentID == proxyAgentID {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByTimeRange returns events within a time range.
func (l *InMemoryAuditLogger) GetEventsByTimeRange(start, end time.Time) []*CredentialAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*CredentialAccessEvent
	for _, event := range l.events {
		if (event.Timestamp.Equal(start) || event.Timestamp.After(start)) &&
			(event.Timestamp.Equal(end) || event.Timestamp.Before(end)) {
			result = append(result, event)
		}
	}
	return result
}

// GetEventCount returns the number of stored events.
func (l *InMemoryAuditLogger) GetEventCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}

// Clear removes all stored events.
func (l *InMemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = make([]*CredentialAccessEvent, 0)
}

// Ensure InMemoryAuditLogger implements AuditLogger.
var _ AuditLogger = (*InMemoryAuditLogger)(nil)

// NoopAuditLogger is an audit logger that does nothing.
type NoopAuditLogger struct{}

// LogCredentialAccess does nothing.
func (l *NoopAuditLogger) LogCredentialAccess(ctx context.Context, event *CredentialAccessEvent) error {
	return nil
}

// Ensure NoopAuditLogger implements AuditLogger.
var _ AuditLogger = (*NoopAuditLogger)(nil)

// MultiAuditLogger logs to multiple loggers.
type MultiAuditLogger struct {
	loggers []AuditLogger
}

// NewMultiAuditLogger creates a new multi audit logger.
func NewMultiAuditLogger(loggers ...AuditLogger) *MultiAuditLogger {
	return &MultiAuditLogger{loggers: loggers}
}

// LogCredentialAccess logs to all configured loggers.
func (l *MultiAuditLogger) LogCredentialAccess(ctx context.Context, event *CredentialAccessEvent) error {
	var lastErr error
	for _, logger := range l.loggers {
		if err := logger.LogCredentialAccess(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Ensure MultiAuditLogger implements AuditLogger.
var _ AuditLogger = (*MultiAuditLogger)(nil)

// AuditSummary contains aggregate statistics for audit events.
type AuditSummary struct {
	// TotalEvents is the total number of events.
	TotalEvents int64
	// SuccessCount is the number of successful operations.
	SuccessCount int64
	// FailureCount is the number of failed operations.
	FailureCount int64
	// EventsByAction is the count of events by action.
	EventsByAction map[string]int64
	// EventsByCredentialType is the count of events by credential type.
	EventsByCredentialType map[CredentialType]int64
	// EventsByProxyAgent is the count of events by proxy agent.
	EventsByProxyAgent map[string]int64
	// AverageDuration is the average operation duration.
	AverageDuration time.Duration
	// OldestEvent is the timestamp of the oldest event.
	OldestEvent time.Time
	// NewestEvent is the timestamp of the newest event.
	NewestEvent time.Time
}

// GetSummary returns aggregate statistics for the stored events.
func (l *InMemoryAuditLogger) GetSummary() *AuditSummary {
	l.mu.RLock()
	defer l.mu.RUnlock()

	summary := &AuditSummary{
		EventsByAction:         make(map[string]int64),
		EventsByCredentialType: make(map[CredentialType]int64),
		EventsByProxyAgent:     make(map[string]int64),
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

		summary.EventsByAction[event.Action]++

		if event.CredentialType != "" {
			summary.EventsByCredentialType[event.CredentialType]++
		}

		if event.ProxyAgentID != "" {
			summary.EventsByProxyAgent[event.ProxyAgentID]++
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

// AuditFilter filters audit events.
type AuditFilter struct {
	// CredentialRef filters by credential reference.
	CredentialRef string
	// ProxyAgentID filters by proxy agent ID.
	ProxyAgentID string
	// DeviceID filters by device ID.
	DeviceID string
	// Action filters by action.
	Action string
	// SuccessOnly filters to only successful events.
	SuccessOnly bool
	// FailureOnly filters to only failed events.
	FailureOnly bool
	// StartTime filters events after this time.
	StartTime time.Time
	// EndTime filters events before this time.
	EndTime time.Time
	// Limit limits the number of results.
	Limit int
}

// GetEventsFiltered returns events matching the filter.
func (l *InMemoryAuditLogger) GetEventsFiltered(filter *AuditFilter) []*CredentialAccessEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*CredentialAccessEvent

	for _, event := range l.events {
		// Apply filters
		if filter.CredentialRef != "" && event.CredentialRef != filter.CredentialRef {
			continue
		}
		if filter.ProxyAgentID != "" && event.ProxyAgentID != filter.ProxyAgentID {
			continue
		}
		if filter.DeviceID != "" && event.DeviceID != filter.DeviceID {
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
		if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
			continue
		}

		result = append(result, event)

		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result
}
