package query

import (
	"context"
	"testing"
	"time"
)

func TestNewInMemoryTracesQuerier(t *testing.T) {
	querier := NewInMemoryTracesQuerier()
	if querier == nil {
		t.Fatal("NewInMemoryTracesQuerier returned nil")
	}
}

func TestTracesQuerierAddTrace(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	trace := &TraceResult{
		TraceID: "trace-1",
		Spans: []Span{
			{
				TraceID:       "trace-1",
				SpanID:        "span-1",
				OperationName: "test-op",
				StartTime:     time.Now(),
				Duration:      100 * time.Millisecond,
			},
		},
	}

	querier.AddTrace(trace)

	querier.mu.RLock()
	defer querier.mu.RUnlock()

	if len(querier.traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(querier.traces))
	}
}

func TestTracesQuerierGetTrace(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	trace := &TraceResult{
		TraceID: "trace-123",
		Spans: []Span{
			{
				TraceID:       "trace-123",
				SpanID:        "span-1",
				OperationName: "test-op",
				StartTime:     time.Now(),
				Duration:      100 * time.Millisecond,
			},
		},
	}

	querier.AddTrace(trace)

	ctx := context.Background()
	result, err := querier.GetTrace(ctx, "trace-123")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}

	if result.TraceID != "trace-123" {
		t.Errorf("Expected trace ID trace-123, got %s", result.TraceID)
	}
}

func TestTracesQuerierGetTraceNotFound(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	ctx := context.Background()
	_, err := querier.GetTrace(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent trace")
	}
}

func TestTracesQuerierQueryByService(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	// Add traces for different services
	querier.AddTrace(&TraceResult{
		TraceID: "trace-1",
		Spans: []Span{
			{
				TraceID:       "trace-1",
				SpanID:        "span-1",
				OperationName: "op-1",
				StartTime:     now,
				Duration:      100 * time.Millisecond,
			},
		},
		Processes: map[string]Process{
			"p1": {ServiceName: "service-a"},
		},
	})

	querier.AddTrace(&TraceResult{
		TraceID: "trace-2",
		Spans: []Span{
			{
				TraceID:       "trace-2",
				SpanID:        "span-2",
				OperationName: "op-2",
				StartTime:     now,
				Duration:      200 * time.Millisecond,
			},
		},
		Processes: map[string]Process{
			"p1": {ServiceName: "service-b"},
		},
	})

	ctx := context.Background()
	query := &TracesQuery{
		Service: "service-a",
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-1" {
		t.Errorf("Expected trace-1, got %s", result.Traces[0].TraceID)
	}
}

func TestTracesQuerierQueryByOperation(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	querier.AddTrace(&TraceResult{
		TraceID: "trace-1",
		Spans: []Span{
			{
				TraceID:       "trace-1",
				SpanID:        "span-1",
				OperationName: "GET /api/users",
				StartTime:     now,
				Duration:      100 * time.Millisecond,
			},
		},
	})

	querier.AddTrace(&TraceResult{
		TraceID: "trace-2",
		Spans: []Span{
			{
				TraceID:       "trace-2",
				SpanID:        "span-2",
				OperationName: "POST /api/users",
				StartTime:     now,
				Duration:      200 * time.Millisecond,
			},
		},
	})

	ctx := context.Background()
	query := &TracesQuery{
		Operation: "GET /api/users",
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-1" {
		t.Errorf("Expected trace-1, got %s", result.Traces[0].TraceID)
	}
}

func TestTracesQuerierQueryByTags(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	querier.AddTrace(&TraceResult{
		TraceID: "trace-1",
		Spans: []Span{
			{
				TraceID:       "trace-1",
				SpanID:        "span-1",
				OperationName: "op-1",
				StartTime:     now,
				Duration:      100 * time.Millisecond,
				Tags: map[string]interface{}{
					"http.status_code": "200",
					"http.method":      "GET",
				},
			},
		},
	})

	querier.AddTrace(&TraceResult{
		TraceID: "trace-2",
		Spans: []Span{
			{
				TraceID:       "trace-2",
				SpanID:        "span-2",
				OperationName: "op-2",
				StartTime:     now,
				Duration:      200 * time.Millisecond,
				Tags: map[string]interface{}{
					"http.status_code": "500",
					"http.method":      "POST",
				},
			},
		},
	})

	ctx := context.Background()
	query := &TracesQuery{
		Tags: map[string]string{
			"http.status_code": "500",
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-2" {
		t.Errorf("Expected trace-2, got %s", result.Traces[0].TraceID)
	}
}

func TestTracesQuerierQueryByDuration(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	querier.AddTrace(&TraceResult{
		TraceID: "trace-fast",
		Spans: []Span{
			{
				TraceID:       "trace-fast",
				SpanID:        "span-1",
				OperationName: "fast-op",
				StartTime:     now,
				Duration:      50 * time.Millisecond,
			},
		},
	})

	querier.AddTrace(&TraceResult{
		TraceID: "trace-slow",
		Spans: []Span{
			{
				TraceID:       "trace-slow",
				SpanID:        "span-2",
				OperationName: "slow-op",
				StartTime:     now,
				Duration:      500 * time.Millisecond,
			},
		},
	})

	ctx := context.Background()

	// Query for slow traces (> 100ms)
	query := &TracesQuery{
		MinDuration: 100 * time.Millisecond,
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 slow trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-slow" {
		t.Errorf("Expected trace-slow, got %s", result.Traces[0].TraceID)
	}

	// Query for fast traces (< 200ms)
	query = &TracesQuery{
		MaxDuration: 200 * time.Millisecond,
	}

	result, err = querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 fast trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-fast" {
		t.Errorf("Expected trace-fast, got %s", result.Traces[0].TraceID)
	}
}

func TestTracesQuerierQueryByTimeRange(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	querier.AddTrace(&TraceResult{
		TraceID: "trace-old",
		Spans: []Span{
			{
				TraceID:       "trace-old",
				SpanID:        "span-1",
				OperationName: "op-1",
				StartTime:     now.Add(-2 * time.Hour),
				Duration:      100 * time.Millisecond,
			},
		},
	})

	querier.AddTrace(&TraceResult{
		TraceID: "trace-recent",
		Spans: []Span{
			{
				TraceID:       "trace-recent",
				SpanID:        "span-2",
				OperationName: "op-2",
				StartTime:     now.Add(-10 * time.Minute),
				Duration:      100 * time.Millisecond,
			},
		},
	})

	ctx := context.Background()
	query := &TracesQuery{
		Range: &TimeRange{
			Start: now.Add(-30 * time.Minute),
			End:   now,
		},
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 recent trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-recent" {
		t.Errorf("Expected trace-recent, got %s", result.Traces[0].TraceID)
	}
}

func TestTracesQuerierQueryLimit(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	// Add 10 traces
	for i := 0; i < 10; i++ {
		querier.AddTrace(&TraceResult{
			TraceID: string(rune('a' + i)),
			Spans: []Span{
				{
					TraceID:       string(rune('a' + i)),
					SpanID:        "span-1",
					OperationName: "op-1",
					StartTime:     now.Add(time.Duration(-i) * time.Minute),
					Duration:      100 * time.Millisecond,
				},
			},
		})
	}

	ctx := context.Background()
	query := &TracesQuery{
		Limit: 5,
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 5 {
		t.Errorf("Expected 5 traces (limited), got %d", len(result.Traces))
	}

	if result.Total != 10 {
		t.Errorf("Expected total 10, got %d", result.Total)
	}
}

func TestTracesQuerierDefaultLimit(t *testing.T) {
	querier := NewInMemoryTracesQuerier()

	now := time.Now()

	// Add 30 traces (more than default limit of 20)
	for i := 0; i < 30; i++ {
		querier.AddTrace(&TraceResult{
			TraceID: string(rune('a' + i)),
			Spans: []Span{
				{
					TraceID:       string(rune('a' + i)),
					SpanID:        "span-1",
					OperationName: "op-1",
					StartTime:     now.Add(time.Duration(-i) * time.Minute),
					Duration:      100 * time.Millisecond,
				},
			},
		})
	}

	ctx := context.Background()
	query := &TracesQuery{
		// No limit specified, should use default of 20
	}

	result, err := querier.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Traces) != 20 {
		t.Errorf("Expected 20 traces (default limit), got %d", len(result.Traces))
	}
}
