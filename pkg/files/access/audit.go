// Package access provides access control for the file distribution system.
package access

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"
)

// AuditEventType represents the type of audit event.
type AuditEventType string

const (
	AuditEventAccess   AuditEventType = "file.access"
	AuditEventDownload AuditEventType = "file.download"
	AuditEventUpload   AuditEventType = "file.upload"
	AuditEventDelete   AuditEventType = "file.delete"
	AuditEventList     AuditEventType = "file.list"
	AuditEventDenied   AuditEventType = "file.denied"
	AuditEventError    AuditEventType = "file.error"
)

// AuditEvent represents a file access audit event.
type AuditEvent struct {
	// ID is the unique event identifier.
	ID string `json:"id"`

	// Type is the event type.
	Type AuditEventType `json:"type"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Identity is the identity that performed the action.
	Identity *AuditIdentity `json:"identity"`

	// Request contains request details.
	Request *AuditRequest `json:"request"`

	// Response contains response details.
	Response *AuditResponse `json:"response,omitempty"`

	// Duration is how long the operation took.
	Duration time.Duration `json:"duration_ns"`

	// Metadata contains additional event metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AuditIdentity represents the identity in an audit event.
type AuditIdentity struct {
	// ID is the identity identifier.
	ID string `json:"id"`

	// Type is the identity type.
	Type string `json:"type"`

	// Roles are the identity roles.
	Roles []string `json:"roles,omitempty"`

	// SourceIP is the source IP address.
	SourceIP string `json:"source_ip,omitempty"`

	// UserAgent is the client user agent.
	UserAgent string `json:"user_agent,omitempty"`
}

// AuditRequest represents the request in an audit event.
type AuditRequest struct {
	// Namespace is the file namespace.
	Namespace string `json:"namespace"`

	// Path is the file path.
	Path string `json:"path"`

	// Action is the requested action.
	Action Action `json:"action"`

	// Backend is the storage backend used.
	Backend string `json:"backend,omitempty"`

	// Checksum is the expected checksum (if provided).
	Checksum string `json:"checksum,omitempty"`

	// Range is the byte range requested (if any).
	Range string `json:"range,omitempty"`
}

// AuditResponse represents the response in an audit event.
type AuditResponse struct {
	// Allowed indicates if access was granted.
	Allowed bool `json:"allowed"`

	// Reason provides the access decision reason.
	Reason string `json:"reason,omitempty"`

	// MatchedRule is the ACL rule that matched.
	MatchedRule string `json:"matched_rule,omitempty"`

	// StatusCode is the response status code.
	StatusCode int `json:"status_code,omitempty"`

	// BytesTransferred is the number of bytes transferred.
	BytesTransferred int64 `json:"bytes_transferred,omitempty"`

	// FileSize is the total file size.
	FileSize int64 `json:"file_size,omitempty"`

	// Error is the error message (if any).
	Error string `json:"error,omitempty"`
}

// AuditLogger logs audit events.
type AuditLogger interface {
	// Log logs an audit event.
	Log(ctx context.Context, event *AuditEvent) error

	// Query queries audit events.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error)

	// Close closes the logger.
	Close() error
}

// AuditFilter filters audit events.
type AuditFilter struct {
	// StartTime filters events after this time.
	StartTime time.Time

	// EndTime filters events before this time.
	EndTime time.Time

	// IdentityID filters by identity ID.
	IdentityID string

	// IdentityType filters by identity type.
	IdentityType string

	// Namespace filters by namespace.
	Namespace string

	// Path filters by path (supports glob).
	Path string

	// Action filters by action.
	Action Action

	// EventType filters by event type.
	EventType AuditEventType

	// Allowed filters by access decision.
	Allowed *bool

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int
}

// InMemoryAuditLogger is an in-memory audit logger for testing.
type InMemoryAuditLogger struct {
	events  []*AuditEvent
	maxSize int
	mu      sync.RWMutex
}

// NewInMemoryAuditLogger creates a new in-memory audit logger.
func NewInMemoryAuditLogger(maxSize int) *InMemoryAuditLogger {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &InMemoryAuditLogger{
		events:  make([]*AuditEvent, 0, maxSize),
		maxSize: maxSize,
	}
}

// Log logs an audit event.
func (l *InMemoryAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Remove oldest if at capacity
	if len(l.events) >= l.maxSize {
		l.events = l.events[1:]
	}

	l.events = append(l.events, event)
	return nil
}

// Query queries audit events.
func (l *InMemoryAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []*AuditEvent

	for _, event := range l.events {
		if l.matches(event, filter) {
			results = append(results, event)
		}
	}

	// Apply offset and limit (only if filter is not nil)
	if filter != nil {
		if filter.Offset > 0 {
			if filter.Offset >= len(results) {
				return nil, nil
			}
			results = results[filter.Offset:]
		}

		if filter.Limit > 0 && len(results) > filter.Limit {
			results = results[:filter.Limit]
		}
	}

	return results, nil
}

// matches checks if an event matches the filter.
func (l *InMemoryAuditLogger) matches(event *AuditEvent, filter *AuditFilter) bool {
	if filter == nil {
		return true
	}

	if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
		return false
	}

	if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
		return false
	}

	if filter.IdentityID != "" && (event.Identity == nil || event.Identity.ID != filter.IdentityID) {
		return false
	}

	if filter.IdentityType != "" && (event.Identity == nil || event.Identity.Type != filter.IdentityType) {
		return false
	}

	if filter.Namespace != "" && (event.Request == nil || event.Request.Namespace != filter.Namespace) {
		return false
	}

	if filter.Path != "" && (event.Request == nil || !matchGlob(filter.Path, event.Request.Path)) {
		return false
	}

	if filter.Action != "" && (event.Request == nil || event.Request.Action != filter.Action) {
		return false
	}

	if filter.EventType != "" && event.Type != filter.EventType {
		return false
	}

	if filter.Allowed != nil && (event.Response == nil || event.Response.Allowed != *filter.Allowed) {
		return false
	}

	return true
}

// GetEvents returns all events (for testing).
func (l *InMemoryAuditLogger) GetEvents() []*AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]*AuditEvent{}, l.events...)
}

// Clear clears all events.
func (l *InMemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = l.events[:0]
}

// Close closes the logger.
func (l *InMemoryAuditLogger) Close() error {
	return nil
}

// JSONFileAuditLogger logs audit events to a JSON file.
type JSONFileAuditLogger struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewJSONFileAuditLogger creates a new JSON file audit logger.
func NewJSONFileAuditLogger(path string) (*JSONFileAuditLogger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &JSONFileAuditLogger{
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

// Log logs an audit event.
func (l *JSONFileAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to encode audit event: %w", err)
	}

	return nil
}

// Query queries audit events (not supported for file logger).
func (l *JSONFileAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	return nil, fmt.Errorf("query not supported for file logger")
}

// Close closes the logger.
func (l *JSONFileAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// WriterAuditLogger logs audit events to any io.Writer.
type WriterAuditLogger struct {
	writer  io.Writer
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewWriterAuditLogger creates a new writer audit logger.
func NewWriterAuditLogger(writer io.Writer) *WriterAuditLogger {
	return &WriterAuditLogger{
		writer:  writer,
		encoder: json.NewEncoder(writer),
	}
}

// Log logs an audit event.
func (l *WriterAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to encode audit event: %w", err)
	}

	return nil
}

// Query queries audit events (not supported for writer logger).
func (l *WriterAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	return nil, fmt.Errorf("query not supported for writer logger")
}

// Close closes the logger.
func (l *WriterAuditLogger) Close() error {
	if closer, ok := l.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// MultiAuditLogger logs to multiple audit loggers.
type MultiAuditLogger struct {
	loggers []AuditLogger
}

// NewMultiAuditLogger creates a new multi audit logger.
func NewMultiAuditLogger(loggers ...AuditLogger) *MultiAuditLogger {
	return &MultiAuditLogger{
		loggers: loggers,
	}
}

// Log logs an audit event to all loggers.
func (l *MultiAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	var lastErr error
	for _, logger := range l.loggers {
		if err := logger.Log(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Query queries the first logger that supports querying.
func (l *MultiAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error) {
	for _, logger := range l.loggers {
		result, err := logger.Query(ctx, filter)
		if err == nil {
			return result, nil
		}
	}
	return nil, fmt.Errorf("no logger supports query")
}

// Close closes all loggers.
func (l *MultiAuditLogger) Close() error {
	var lastErr error
	for _, logger := range l.loggers {
		if err := logger.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// AuditRecorder provides helpers for recording audit events.
type AuditRecorder struct {
	logger    AuditLogger
	idCounter int64
	mu        sync.Mutex
}

// NewAuditRecorder creates a new audit recorder.
func NewAuditRecorder(logger AuditLogger) *AuditRecorder {
	return &AuditRecorder{
		logger: logger,
	}
}

// generateID generates a unique event ID.
func (r *AuditRecorder) generateID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idCounter++
	return fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), r.idCounter)
}

// RecordAccess records a file access event.
func (r *AuditRecorder) RecordAccess(ctx context.Context, identity *Identity, req *AccessRequest, result *AccessResult) error {
	eventType := AuditEventAccess
	if !result.Allowed {
		eventType = AuditEventDenied
	}

	event := &AuditEvent{
		ID:        r.generateID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:    identity.ID,
			Type:  identity.Type,
			Roles: identity.Roles,
		},
		Request: &AuditRequest{
			Namespace: req.Namespace,
			Path:      req.Path,
			Action:    req.Action,
		},
		Response: &AuditResponse{
			Allowed:     result.Allowed,
			Reason:      result.Reason,
			MatchedRule: result.MatchedRule,
		},
		Duration: result.Duration,
		Metadata: req.Metadata,
	}

	return r.logger.Log(ctx, event)
}

// RecordDownload records a file download event.
func (r *AuditRecorder) RecordDownload(ctx context.Context, identity *Identity, namespace, path, backend string, size int64, duration time.Duration, err error) error {
	event := &AuditEvent{
		ID:        r.generateID(),
		Type:      AuditEventDownload,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:    identity.ID,
			Type:  identity.Type,
			Roles: identity.Roles,
		},
		Request: &AuditRequest{
			Namespace: namespace,
			Path:      path,
			Action:    ActionGet,
			Backend:   backend,
		},
		Response: &AuditResponse{
			Allowed:          err == nil,
			BytesTransferred: size,
			FileSize:         size,
			StatusCode:       200,
		},
		Duration: duration,
	}

	if err != nil {
		event.Type = AuditEventError
		event.Response.Error = err.Error()
		event.Response.StatusCode = 500
	}

	return r.logger.Log(ctx, event)
}

// RecordUpload records a file upload event.
func (r *AuditRecorder) RecordUpload(ctx context.Context, identity *Identity, namespace, path, backend string, size int64, duration time.Duration, err error) error {
	event := &AuditEvent{
		ID:        r.generateID(),
		Type:      AuditEventUpload,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:    identity.ID,
			Type:  identity.Type,
			Roles: identity.Roles,
		},
		Request: &AuditRequest{
			Namespace: namespace,
			Path:      path,
			Action:    ActionPut,
			Backend:   backend,
		},
		Response: &AuditResponse{
			Allowed:          err == nil,
			BytesTransferred: size,
			FileSize:         size,
			StatusCode:       201,
		},
		Duration: duration,
	}

	if err != nil {
		event.Type = AuditEventError
		event.Response.Error = err.Error()
		event.Response.StatusCode = 500
	}

	return r.logger.Log(ctx, event)
}

// RecordDelete records a file delete event.
func (r *AuditRecorder) RecordDelete(ctx context.Context, identity *Identity, namespace, path, backend string, duration time.Duration, err error) error {
	event := &AuditEvent{
		ID:        r.generateID(),
		Type:      AuditEventDelete,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:    identity.ID,
			Type:  identity.Type,
			Roles: identity.Roles,
		},
		Request: &AuditRequest{
			Namespace: namespace,
			Path:      path,
			Action:    ActionDelete,
			Backend:   backend,
		},
		Response: &AuditResponse{
			Allowed:    err == nil,
			StatusCode: 204,
		},
		Duration: duration,
	}

	if err != nil {
		event.Type = AuditEventError
		event.Response.Error = err.Error()
		event.Response.StatusCode = 500
	}

	return r.logger.Log(ctx, event)
}

// RecordList records a file list event.
func (r *AuditRecorder) RecordList(ctx context.Context, identity *Identity, namespace, path, backend string, count int, duration time.Duration, err error) error {
	event := &AuditEvent{
		ID:        r.generateID(),
		Type:      AuditEventList,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:    identity.ID,
			Type:  identity.Type,
			Roles: identity.Roles,
		},
		Request: &AuditRequest{
			Namespace: namespace,
			Path:      path,
			Action:    ActionList,
			Backend:   backend,
		},
		Response: &AuditResponse{
			Allowed:    err == nil,
			StatusCode: 200,
		},
		Duration: duration,
		Metadata: map[string]string{
			"count": fmt.Sprintf("%d", count),
		},
	}

	if err != nil {
		event.Type = AuditEventError
		event.Response.Error = err.Error()
		event.Response.StatusCode = 500
	}

	return r.logger.Log(ctx, event)
}

// matchGlob performs glob matching using the same semantics as ACL patterns.
// * matches any character except /
// ** matches any character including /
// ? matches any single character except /
func matchGlob(pattern, value string) bool {
	if pattern == "" {
		return true
	}

	// Use globToRegex for proper glob semantics
	regexPattern := globToRegex(pattern)
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}
