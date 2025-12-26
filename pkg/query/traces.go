package query

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// InMemoryTracesQuerier is a simple in-memory traces querier for testing
// In production, this would be replaced with a Jaeger or Tempo client
type InMemoryTracesQuerier struct {
	traces map[string]*TraceResult
	mu     sync.RWMutex
}

// NewInMemoryTracesQuerier creates a new in-memory traces querier
func NewInMemoryTracesQuerier() *InMemoryTracesQuerier {
	return &InMemoryTracesQuerier{
		traces: make(map[string]*TraceResult),
	}
}

// AddTrace adds a trace (for testing)
func (t *InMemoryTracesQuerier) AddTrace(trace *TraceResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.traces[trace.TraceID] = trace
}

// Query executes a traces query
func (t *InMemoryTracesQuerier) Query(ctx context.Context, query *TracesQuery) (*TracesResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Filter traces
	filtered := make([]*TraceResult, 0)
	for _, trace := range t.traces {
		if matchesQuery(trace, query) {
			filtered = append(filtered, trace)
		}
	}

	// Sort by start time (most recent first)
	sort.Slice(filtered, func(i, j int) bool {
		return getTraceStartTime(filtered[i]).After(getTraceStartTime(filtered[j]))
	})

	// Apply limit
	limit := query.Limit
	if limit == 0 {
		limit = 20 // Default limit
	}

	total := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	// Convert to result
	results := make([]TraceResult, len(filtered))
	for i, trace := range filtered {
		results[i] = *trace
	}

	return &TracesResult{
		Traces: results,
		Total:  total,
	}, nil
}

// GetTrace retrieves a specific trace by ID
func (t *InMemoryTracesQuerier) GetTrace(ctx context.Context, traceID string) (*TraceResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	trace, exists := t.traces[traceID]
	if !exists {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	return trace, nil
}

// matchesQuery checks if a trace matches the query criteria
func matchesQuery(trace *TraceResult, query *TracesQuery) bool {
	// Check service filter
	if query.Service != "" {
		serviceMatches := false
		for _, process := range trace.Processes {
			if process.ServiceName == query.Service {
				serviceMatches = true
				break
			}
		}
		if !serviceMatches {
			return false
		}
	}

	// Check operation filter
	if query.Operation != "" {
		operationMatches := false
		for _, span := range trace.Spans {
			if span.OperationName == query.Operation {
				operationMatches = true
				break
			}
		}
		if !operationMatches {
			return false
		}
	}

	// Check tags filter
	if len(query.Tags) > 0 {
		tagsMatch := false
		for _, span := range trace.Spans {
			if matchesTags(span.Tags, query.Tags) {
				tagsMatch = true
				break
			}
		}
		if !tagsMatch {
			return false
		}
	}

	// Check time range
	if query.Range != nil {
		traceStart := getTraceStartTime(trace)
		if traceStart.Before(query.Range.Start) || traceStart.After(query.Range.End) {
			return false
		}
	}

	// Check duration filters
	if query.MinDuration > 0 || query.MaxDuration > 0 {
		traceDuration := getTraceDuration(trace)
		if query.MinDuration > 0 && traceDuration < query.MinDuration {
			return false
		}
		if query.MaxDuration > 0 && traceDuration > query.MaxDuration {
			return false
		}
	}

	return true
}

// matchesTags checks if span tags match the query tags
func matchesTags(spanTags map[string]interface{}, queryTags map[string]string) bool {
	for key, value := range queryTags {
		spanValue, exists := spanTags[key]
		if !exists {
			return false
		}
		if fmt.Sprintf("%v", spanValue) != value {
			return false
		}
	}
	return true
}

// getTraceStartTime returns the earliest span start time in a trace
func getTraceStartTime(trace *TraceResult) time.Time {
	if len(trace.Spans) == 0 {
		return time.Time{}
	}

	earliest := trace.Spans[0].StartTime
	for _, span := range trace.Spans[1:] {
		if span.StartTime.Before(earliest) {
			earliest = span.StartTime
		}
	}
	return earliest
}

// getTraceDuration returns the total duration of a trace
func getTraceDuration(trace *TraceResult) time.Duration {
	if len(trace.Spans) == 0 {
		return 0
	}

	start := getTraceStartTime(trace)
	var end time.Time

	for _, span := range trace.Spans {
		spanEnd := span.StartTime.Add(span.Duration)
		if spanEnd.After(end) {
			end = spanEnd
		}
	}

	return end.Sub(start)
}

// JaegerQuerier queries traces from Jaeger
type JaegerQuerier struct {
	address string
	// In a real implementation, this would have a Jaeger client
}

// NewJaegerQuerier creates a new Jaeger traces querier
func NewJaegerQuerier(address string) *JaegerQuerier {
	return &JaegerQuerier{
		address: address,
	}
}

// Query executes a traces query against Jaeger
func (j *JaegerQuerier) Query(ctx context.Context, query *TracesQuery) (*TracesResult, error) {
	// This is a placeholder - a real implementation would use the Jaeger API
	return nil, fmt.Errorf("Jaeger querier not implemented - use in-memory querier for testing")
}

// GetTrace retrieves a specific trace from Jaeger
func (j *JaegerQuerier) GetTrace(ctx context.Context, traceID string) (*TraceResult, error) {
	// This is a placeholder - a real implementation would use the Jaeger API
	return nil, fmt.Errorf("Jaeger querier not implemented - use in-memory querier for testing")
}
