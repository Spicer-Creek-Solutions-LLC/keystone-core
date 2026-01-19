// Package audit provides comprehensive audit logging capabilities.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Errors returned by the audit logger.
var (
	ErrLoggerClosed = errors.New("logger closed")
	ErrInvalidEvent = errors.New("invalid event")
)

// Level represents audit log severity.
type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

// Category represents audit event category.
type Category string

const (
	CategoryAuth       Category = "authentication"
	CategoryAuthz      Category = "authorization"
	CategoryData       Category = "data_access"
	CategoryConfig     Category = "configuration"
	CategoryAdmin      Category = "administration"
	CategorySecurity   Category = "security"
	CategoryCompliance Category = "compliance"
	CategorySystem     Category = "system"
)

// Action represents the type of action performed.
type Action string

const (
	ActionCreate  Action = "create"
	ActionRead    Action = "read"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionExecute Action = "execute"
	ActionLogin   Action = "login"
	ActionLogout  Action = "logout"
	ActionGrant   Action = "grant"
	ActionRevoke  Action = "revoke"
	ActionExport  Action = "export"
	ActionImport  Action = "import"
)

// Outcome represents the result of an action.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
	OutcomeError   Outcome = "error"
)

// Event represents an audit log event.
type Event struct {
	// ID is the unique event identifier.
	ID string `json:"id"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Level is the event severity.
	Level Level `json:"level"`
	// Category is the event category.
	Category Category `json:"category"`
	// Action is the action performed.
	Action Action `json:"action"`
	// Outcome is the result of the action.
	Outcome Outcome `json:"outcome"`
	// Actor contains information about who performed the action.
	Actor *Actor `json:"actor,omitempty"`
	// Resource contains information about the affected resource.
	Resource *Resource `json:"resource,omitempty"`
	// Request contains request details.
	Request *Request `json:"request,omitempty"`
	// Response contains response details.
	Response *Response `json:"response,omitempty"`
	// Context contains additional context.
	Context map[string]interface{} `json:"context,omitempty"`
	// Tags are searchable labels.
	Tags []string `json:"tags,omitempty"`
	// Duration is how long the action took.
	Duration time.Duration `json:"duration,omitempty"`
	// Error contains error details if applicable.
	Error *ErrorInfo `json:"error,omitempty"`
}

// Actor represents who performed an action.
type Actor struct {
	// ID is the actor's unique identifier.
	ID string `json:"id"`
	// Type is the actor type (user, service, system).
	Type string `json:"type"`
	// Name is the actor's display name.
	Name string `json:"name,omitempty"`
	// Email is the actor's email.
	Email string `json:"email,omitempty"`
	// IP is the actor's IP address.
	IP string `json:"ip,omitempty"`
	// SessionID is the session identifier.
	SessionID string `json:"session_id,omitempty"`
	// Roles are the actor's current roles.
	Roles []string `json:"roles,omitempty"`
}

// Resource represents the affected resource.
type Resource struct {
	// ID is the resource identifier.
	ID string `json:"id"`
	// Type is the resource type.
	Type string `json:"type"`
	// Name is the resource name.
	Name string `json:"name,omitempty"`
	// Path is the resource path/location.
	Path string `json:"path,omitempty"`
	// Owner is the resource owner.
	Owner string `json:"owner,omitempty"`
	// Attributes are additional resource attributes.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Request contains request details.
type Request struct {
	// ID is the request identifier.
	ID string `json:"id,omitempty"`
	// Method is the request method.
	Method string `json:"method,omitempty"`
	// Path is the request path.
	Path string `json:"path,omitempty"`
	// Headers are selected request headers.
	Headers map[string]string `json:"headers,omitempty"`
	// Parameters are request parameters.
	Parameters map[string]string `json:"parameters,omitempty"`
	// Size is the request body size.
	Size int64 `json:"size,omitempty"`
}

// Response contains response details.
type Response struct {
	// StatusCode is the response status code.
	StatusCode int `json:"status_code,omitempty"`
	// Size is the response body size.
	Size int64 `json:"size,omitempty"`
	// Headers are selected response headers.
	Headers map[string]string `json:"headers,omitempty"`
}

// ErrorInfo contains error details.
type ErrorInfo struct {
	// Code is the error code.
	Code string `json:"code,omitempty"`
	// Message is the error message.
	Message string `json:"message"`
	// Stack is the stack trace.
	Stack string `json:"stack,omitempty"`
}

// Config holds audit logger configuration.
type Config struct {
	// Enabled controls whether logging is active.
	Enabled bool
	// MinLevel is the minimum level to log.
	MinLevel Level
	// Categories are categories to log (empty = all).
	Categories []Category
	// BufferSize is the event buffer size.
	BufferSize int
	// FlushInterval is how often to flush the buffer.
	FlushInterval time.Duration
	// IncludeRequest includes request details.
	IncludeRequest bool
	// IncludeResponse includes response details.
	IncludeResponse bool
	// RedactFields are field names to redact.
	RedactFields []string
	// OutputFormat is the output format (json, text).
	OutputFormat string
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		MinLevel:        LevelInfo,
		BufferSize:      1000,
		FlushInterval:   time.Second * 5,
		IncludeRequest:  true,
		IncludeResponse: true,
		RedactFields:    []string{"password", "token", "secret", "key", "authorization"},
		OutputFormat:    "json",
	}
}

// Writer is the interface for audit log destinations.
type Writer interface {
	Write(event *Event) error
	Flush() error
	Close() error
}

// Logger is the main audit logger.
type Logger struct {
	config    *Config
	writers   []Writer
	buffer    chan *Event
	idCounter uint64
	closed    bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
}

// NewLogger creates a new audit logger.
func NewLogger(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig()
	}

	l := &Logger{
		config: config,
		buffer: make(chan *Event, config.BufferSize),
		stopCh: make(chan struct{}),
	}

	// Start background flusher
	l.wg.Add(1)
	go l.flushLoop()

	return l
}

// AddWriter adds an output writer.
func (l *Logger) AddWriter(w Writer) {
	l.mu.Lock()
	l.writers = append(l.writers, w)
	l.mu.Unlock()
}

// Log logs an audit event.
func (l *Logger) Log(ctx context.Context, event *Event) error {
	if !l.config.Enabled {
		return nil
	}

	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return ErrLoggerClosed
	}
	l.mu.RUnlock()

	// Check level
	if !l.shouldLog(event) {
		return nil
	}

	// Set defaults
	if event.ID == "" {
		event.ID = l.generateID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}

	// Redact sensitive fields
	l.redact(event)

	// Buffer the event
	select {
	case l.buffer <- event:
		return nil
	default:
		// Buffer full, write directly
		return l.writeEvent(event)
	}
}

// LogAuth logs an authentication event.
func (l *Logger) LogAuth(ctx context.Context, action Action, outcome Outcome, actor *Actor, details map[string]interface{}) error {
	event := &Event{
		Category: CategoryAuth,
		Action:   action,
		Outcome:  outcome,
		Actor:    actor,
		Context:  details,
	}

	if outcome == OutcomeFailure || outcome == OutcomeDenied {
		event.Level = LevelWarning
	}

	return l.Log(ctx, event)
}

// LogAuthz logs an authorization event.
func (l *Logger) LogAuthz(ctx context.Context, action Action, outcome Outcome, actor *Actor, resource *Resource, details map[string]interface{}) error {
	event := &Event{
		Category: CategoryAuthz,
		Action:   action,
		Outcome:  outcome,
		Actor:    actor,
		Resource: resource,
		Context:  details,
	}

	if outcome == OutcomeDenied {
		event.Level = LevelWarning
	}

	return l.Log(ctx, event)
}

// LogData logs a data access event.
func (l *Logger) LogData(ctx context.Context, action Action, outcome Outcome, actor *Actor, resource *Resource) error {
	event := &Event{
		Category: CategoryData,
		Action:   action,
		Outcome:  outcome,
		Actor:    actor,
		Resource: resource,
	}

	return l.Log(ctx, event)
}

// LogConfig logs a configuration change event.
func (l *Logger) LogConfig(ctx context.Context, action Action, outcome Outcome, actor *Actor, resource *Resource, before, after interface{}) error {
	event := &Event{
		Category: CategoryConfig,
		Action:   action,
		Outcome:  outcome,
		Actor:    actor,
		Resource: resource,
		Level:    LevelWarning,
		Context: map[string]interface{}{
			"before": before,
			"after":  after,
		},
	}

	return l.Log(ctx, event)
}

// LogAdmin logs an administrative action.
func (l *Logger) LogAdmin(ctx context.Context, action Action, outcome Outcome, actor *Actor, resource *Resource, details map[string]interface{}) error {
	event := &Event{
		Category: CategoryAdmin,
		Action:   action,
		Outcome:  outcome,
		Actor:    actor,
		Resource: resource,
		Context:  details,
		Level:    LevelWarning,
	}

	return l.Log(ctx, event)
}

// LogSecurity logs a security event.
func (l *Logger) LogSecurity(ctx context.Context, message string, actor *Actor, details map[string]interface{}) error {
	event := &Event{
		Category: CategorySecurity,
		Level:    LevelCritical,
		Outcome:  OutcomeFailure,
		Actor:    actor,
		Context:  details,
		Tags:     []string{"security", "alert"},
	}

	if details == nil {
		event.Context = make(map[string]interface{})
	}
	event.Context["message"] = message

	return l.Log(ctx, event)
}

func (l *Logger) shouldLog(event *Event) bool {
	// Check level
	levels := map[Level]int{
		LevelInfo:     0,
		LevelWarning:  1,
		LevelCritical: 2,
	}

	if levels[event.Level] < levels[l.config.MinLevel] {
		return false
	}

	// Check category filter
	if len(l.config.Categories) > 0 {
		found := false
		for _, c := range l.config.Categories {
			if c == event.Category {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (l *Logger) redact(event *Event) {
	if len(l.config.RedactFields) == 0 {
		return
	}

	// Redact in context
	if event.Context != nil {
		l.redactMap(event.Context)
	}

	// Redact in request
	if event.Request != nil && event.Request.Headers != nil {
		l.redactStringMap(event.Request.Headers)
	}
	if event.Request != nil && event.Request.Parameters != nil {
		l.redactStringMap(event.Request.Parameters)
	}

	// Redact in response
	if event.Response != nil && event.Response.Headers != nil {
		l.redactStringMap(event.Response.Headers)
	}
}

func (l *Logger) redactMap(m map[string]interface{}) {
	for key := range m {
		for _, field := range l.config.RedactFields {
			if containsIgnoreCase(key, field) {
				m[key] = "[REDACTED]"
				break
			}
		}
	}
}

func (l *Logger) redactStringMap(m map[string]string) {
	for key := range m {
		for _, field := range l.config.RedactFields {
			if containsIgnoreCase(key, field) {
				m[key] = "[REDACTED]"
				break
			}
		}
	}
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr)))
}

func (l *Logger) generateID() string {
	l.mu.Lock()
	l.idCounter++
	id := l.idCounter
	l.mu.Unlock()
	return fmt.Sprintf("aud-%d-%d", time.Now().UnixNano(), id)
}

func (l *Logger) flushLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-l.buffer:
			l.writeEvent(event)
		case <-ticker.C:
			l.Flush()
		case <-l.stopCh:
			// Drain buffer
			for {
				select {
				case event := <-l.buffer:
					l.writeEvent(event)
				default:
					return
				}
			}
		}
	}
}

func (l *Logger) writeEvent(event *Event) error {
	l.mu.RLock()
	writers := l.writers
	l.mu.RUnlock()

	var lastErr error
	for _, w := range writers {
		if err := w.Write(event); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Flush flushes all buffered events.
func (l *Logger) Flush() error {
	l.mu.RLock()
	writers := l.writers
	l.mu.RUnlock()

	var lastErr error
	for _, w := range writers {
		if err := w.Flush(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Close closes the logger.
func (l *Logger) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return ErrLoggerClosed
	}
	l.closed = true
	l.mu.Unlock()

	close(l.stopCh)
	l.wg.Wait()

	// Close all writers
	var lastErr error
	for _, w := range l.writers {
		if err := w.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// JSONWriter writes events as JSON.
type JSONWriter struct {
	w       io.Writer
	mu      sync.Mutex
	pretty  bool
	newline bool
}

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter(w io.Writer) *JSONWriter {
	return &JSONWriter{
		w:       w,
		newline: true,
	}
}

// Write writes an event as JSON.
func (w *JSONWriter) Write(event *Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var data []byte
	var err error

	if w.pretty {
		data, err = json.MarshalIndent(event, "", "  ")
	} else {
		data, err = json.Marshal(event)
	}

	if err != nil {
		return err
	}

	if w.newline {
		data = append(data, '\n')
	}

	_, err = w.w.Write(data)
	return err
}

// Flush flushes the writer.
func (w *JSONWriter) Flush() error {
	if f, ok := w.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// Close closes the writer.
func (w *JSONWriter) Close() error {
	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// FileWriter writes events to a file.
type FileWriter struct {
	*JSONWriter
	path string
	file *os.File
}

// NewFileWriter creates a new file writer.
func NewFileWriter(path string) (*FileWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &FileWriter{
		JSONWriter: NewJSONWriter(file),
		path:       path,
		file:       file,
	}, nil
}

// Close closes the file.
func (w *FileWriter) Close() error {
	return w.file.Close()
}

// MemoryWriter stores events in memory (for testing).
type MemoryWriter struct {
	events []*Event
	mu     sync.Mutex
	cond   *sync.Cond
}

// NewMemoryWriter creates a new memory writer.
func NewMemoryWriter() *MemoryWriter {
	w := &MemoryWriter{}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// Write stores an event.
func (w *MemoryWriter) Write(event *Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	w.cond.Broadcast()
	return nil
}

// Flush is a no-op for memory writer.
func (w *MemoryWriter) Flush() error {
	return nil
}

// Close is a no-op for memory writer.
func (w *MemoryWriter) Close() error {
	return nil
}

// Events returns all stored events.
func (w *MemoryWriter) Events() []*Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]*Event, len(w.events))
	copy(result, w.events)
	return result
}

// Clear clears all events.
func (w *MemoryWriter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = nil
}

// Count returns the number of events.
func (w *MemoryWriter) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.events)
}

// WaitForCount waits until the writer has at least n events.
func (w *MemoryWriter) WaitForCount(n int, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		w.mu.Lock()
		for len(w.events) < n {
			w.cond.Wait()
		}
		w.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		w.cond.Broadcast() // Wake up waiting goroutine
		return fmt.Errorf("timeout waiting for %d events (have %d)", n, w.Count())
	}
}
