// Package traces provides the traces gateway for aggregating agent traces.
package traces

import (
	"sync"
	"time"
)

// StoreConfig holds configuration for the traces store.
type StoreConfig struct {
	// MaxTraces is the maximum number of traces to store
	MaxTraces int

	// MaxAge is the maximum age for traces
	MaxAge time.Duration

	// SamplingRate is the probability of keeping a trace (0.0 to 1.0)
	SamplingRate float64

	// SampleErrors always keeps traces with errors
	SampleErrors bool

	// SlowThreshold always keeps traces slower than this
	SlowThreshold time.Duration
}

// DefaultStoreConfig returns a configuration with sensible defaults.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxTraces:     10000,
		MaxAge:        1 * time.Hour,
		SamplingRate:  1.0,
		SampleErrors:  true,
		SlowThreshold: 1 * time.Second,
	}
}

// Trace represents a distributed trace.
type Trace struct {
	// TraceID is the unique trace identifier
	TraceID string

	// Spans are the spans in this trace
	Spans []Span

	// StartTime is when the trace started
	StartTime time.Time

	// EndTime is when the trace ended
	EndTime time.Time

	// Duration is the total trace duration
	Duration time.Duration

	// HasError indicates if any span has an error
	HasError bool

	// ServiceName is the root service name
	ServiceName string

	// OperationName is the root operation name
	OperationName string
}

// Span represents a span in a trace.
type Span struct {
	// TraceID is the trace this span belongs to
	TraceID string

	// SpanID is the unique span identifier
	SpanID string

	// ParentSpanID is the parent span (empty for root)
	ParentSpanID string

	// OperationName is the operation name
	OperationName string

	// ServiceName is the service name
	ServiceName string

	// StartTime is when the span started
	StartTime time.Time

	// EndTime is when the span ended
	EndTime time.Time

	// Duration is the span duration
	Duration time.Duration

	// Status is the span status
	Status SpanStatus

	// Attributes are span attributes
	Attributes map[string]interface{}

	// Events are span events
	Events []SpanEvent

	// AgentID is the agent that produced this span
	AgentID string
}

// SpanStatus represents the span status.
type SpanStatus int

// SpanStatusUnset constants define the possible statuses.
const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

// SpanEvent represents an event in a span.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]interface{}
}

// Store stores traces from multiple agents.
type Store struct {
	traces map[string]*Trace
	mu     sync.RWMutex
	config StoreConfig

	// Sampling state
	sampleCounter uint64

	// Callbacks
	onTraceAdded func(trace *Trace)

	// Stats
	tracesReceived int64
	tracesDropped  int64
	spansReceived  int64
}

// NewTracesStore creates a new traces store.
func NewTracesStore(config StoreConfig) *Store {
	return &Store{
		traces: make(map[string]*Trace),
		config: config,
	}
}

// SetOnTraceAdded sets the callback for when a trace is added.
func (s *Store) SetOnTraceAdded(fn func(trace *Trace)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTraceAdded = fn
}

// Store stores spans, grouping them into traces.
func (s *Store) Store(spans []Span) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored := 0

	for i := range spans {
		span := &spans[i]
		s.spansReceived++

		// Get or create trace
		trace, exists := s.traces[span.TraceID]
		if !exists {
			// Check if we should sample this trace
			if !s.shouldSample(span) {
				s.tracesDropped++
				continue
			}

			s.tracesReceived++

			// Evict oldest if at capacity
			if len(s.traces) >= s.config.MaxTraces {
				s.evictOldest()
			}

			trace = &Trace{
				TraceID:   span.TraceID,
				Spans:     make([]Span, 0),
				StartTime: span.StartTime,
			}
			s.traces[span.TraceID] = trace
		}

		// Add span to trace
		trace.Spans = append(trace.Spans, *span)

		// Update trace metadata
		if span.StartTime.Before(trace.StartTime) || trace.StartTime.IsZero() {
			trace.StartTime = span.StartTime
		}
		if span.EndTime.After(trace.EndTime) {
			trace.EndTime = span.EndTime
		}
		if span.Status == SpanStatusError {
			trace.HasError = true
		}

		// Set root span info if this is the root
		if span.ParentSpanID == "" {
			trace.ServiceName = span.ServiceName
			trace.OperationName = span.OperationName
		}

		// Recalculate duration
		if !trace.StartTime.IsZero() && !trace.EndTime.IsZero() {
			trace.Duration = trace.EndTime.Sub(trace.StartTime)
		}

		stored++
	}

	return stored
}

// shouldSample determines if a span/trace should be sampled.
func (s *Store) shouldSample(span *Span) bool {
	// Always sample errors if configured
	if s.config.SampleErrors && span.Status == SpanStatusError {
		return true
	}

	// Always sample slow spans if configured
	if s.config.SlowThreshold > 0 && span.Duration >= s.config.SlowThreshold {
		return true
	}

	// Apply sampling rate
	if s.config.SamplingRate >= 1.0 {
		return true
	}
	if s.config.SamplingRate <= 0.0 {
		return false
	}

	// Simple deterministic sampling based on trace ID hash
	s.sampleCounter++
	return float64(s.sampleCounter%1000)/1000.0 < s.config.SamplingRate
}

// evictOldest removes the oldest trace.
func (s *Store) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, trace := range s.traces {
		if oldestID == "" || trace.StartTime.Before(oldestTime) {
			oldestID = id
			oldestTime = trace.StartTime
		}
	}

	if oldestID != "" {
		delete(s.traces, oldestID)
	}
}

// Get returns a trace by ID.
func (s *Store) Get(traceID string) (*Trace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trace, exists := s.traces[traceID]
	return trace, exists
}

// GetAll returns all traces.
func (s *Store) GetAll() []*Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Trace, 0, len(s.traces))
	for _, trace := range s.traces {
		result = append(result, trace)
	}
	return result
}

// TraceQuery defines query parameters for traces.
type TraceQuery struct {
	// ServiceName filters by service
	ServiceName string

	// OperationName filters by operation
	OperationName string

	// MinDuration filters traces with duration >= this
	MinDuration time.Duration

	// MaxDuration filters traces with duration <= this
	MaxDuration time.Duration

	// Start is the start time
	Start time.Time

	// End is the end time
	End time.Time

	// HasError filters traces with errors
	HasError *bool

	// Limit is the maximum number of traces to return
	Limit int
}

// Query returns traces matching the query.
func (s *Store) Query(q TraceQuery) []*Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Trace

	for _, trace := range s.traces {
		// Apply filters
		if q.ServiceName != "" && trace.ServiceName != q.ServiceName {
			continue
		}
		if q.OperationName != "" && trace.OperationName != q.OperationName {
			continue
		}
		if q.MinDuration > 0 && trace.Duration < q.MinDuration {
			continue
		}
		if q.MaxDuration > 0 && trace.Duration > q.MaxDuration {
			continue
		}
		if !q.Start.IsZero() && trace.StartTime.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && trace.EndTime.After(q.End) {
			continue
		}
		if q.HasError != nil && trace.HasError != *q.HasError {
			continue
		}

		result = append(result, trace)

		if q.Limit > 0 && len(result) >= q.Limit {
			break
		}
	}

	return result
}

// GetPending returns traces that haven't been exported yet.
func (s *Store) GetPending(limit int) []*Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Trace, 0, limit)
	for _, trace := range s.traces {
		result = append(result, trace)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// Remove removes a trace.
func (s *Store) Remove(traceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.traces, traceID)
}

// Cleanup removes old traces.
func (s *Store) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.MaxAge <= 0 {
		return 0
	}

	cutoff := time.Now().Add(-s.config.MaxAge)
	removed := 0

	for id, trace := range s.traces {
		if trace.EndTime.Before(cutoff) {
			delete(s.traces, id)
			removed++
		}
	}

	return removed
}

// Clear removes all traces.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces = make(map[string]*Trace)
}

// StoreStats holds store statistics.
type StoreStats struct {
	TraceCount     int
	TracesReceived int64
	TracesDropped  int64
	SpansReceived  int64
}

// Stats returns store statistics.
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StoreStats{
		TraceCount:     len(s.traces),
		TracesReceived: s.tracesReceived,
		TracesDropped:  s.tracesDropped,
		SpansReceived:  s.spansReceived,
	}
}
