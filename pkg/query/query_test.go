package query

import (
	"context"
	"testing"
	"time"
)

func TestNewAPI(t *testing.T) {
	logsQuerier := NewInMemoryLogsQuerier()
	tracesQuerier := NewInMemoryTracesQuerier()

	api := NewAPI(nil, logsQuerier, tracesQuerier)
	if api == nil {
		t.Fatal("NewAPI returned nil")
	}
}

func TestAPIQueryMetricsNotConfigured(t *testing.T) {
	api := NewAPI(nil, nil, nil)

	ctx := context.Background()
	query := &MetricsQuery{
		Query: "up",
	}

	_, err := api.QueryMetrics(ctx, query)
	if err == nil {
		t.Error("Expected error when metrics querier not configured")
	}

	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Error("Expected ErrorResponse")
	}

	if errResp.Code != "METRICS_UNAVAILABLE" {
		t.Errorf("Expected code METRICS_UNAVAILABLE, got %s", errResp.Code)
	}
}

func TestAPIQueryLogsNotConfigured(t *testing.T) {
	api := NewAPI(nil, nil, nil)

	ctx := context.Background()
	query := &LogsQuery{
		Query: `{app="test"}`,
		Range: TimeRange{
			Start: time.Now().Add(-1 * time.Hour),
			End:   time.Now(),
		},
	}

	_, err := api.QueryLogs(ctx, query)
	if err == nil {
		t.Error("Expected error when logs querier not configured")
	}

	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Error("Expected ErrorResponse")
	}

	if errResp.Code != "LOGS_UNAVAILABLE" {
		t.Errorf("Expected code LOGS_UNAVAILABLE, got %s", errResp.Code)
	}
}

func TestAPIQueryTracesNotConfigured(t *testing.T) {
	api := NewAPI(nil, nil, nil)

	ctx := context.Background()
	query := &TracesQuery{
		Service: "test-service",
	}

	_, err := api.QueryTraces(ctx, query)
	if err == nil {
		t.Error("Expected error when traces querier not configured")
	}

	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Error("Expected ErrorResponse")
	}

	if errResp.Code != "TRACES_UNAVAILABLE" {
		t.Errorf("Expected code TRACES_UNAVAILABLE, got %s", errResp.Code)
	}
}

func TestAPIGetTraceNotConfigured(t *testing.T) {
	api := NewAPI(nil, nil, nil)

	ctx := context.Background()

	_, err := api.GetTrace(ctx, "trace-123")
	if err == nil {
		t.Error("Expected error when traces querier not configured")
	}

	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Error("Expected ErrorResponse")
	}

	if errResp.Code != "TRACES_UNAVAILABLE" {
		t.Errorf("Expected code TRACES_UNAVAILABLE, got %s", errResp.Code)
	}
}

func TestAPIQueryLogs(t *testing.T) {
	logsQuerier := NewInMemoryLogsQuerier()

	// Add some test entries
	now := time.Now()
	logsQuerier.AddEntry(LogEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Line:      "Test log 1",
		Labels:    map[string]string{"app": "test"},
	})
	logsQuerier.AddEntry(LogEntry{
		Timestamp: now.Add(-3 * time.Minute),
		Line:      "Test log 2",
		Labels:    map[string]string{"app": "test"},
	})

	api := NewAPI(nil, logsQuerier, nil)

	ctx := context.Background()
	query := &LogsQuery{
		Query: "Test",
		Range: TimeRange{
			Start: now.Add(-10 * time.Minute),
			End:   now,
		},
		Limit: 10,
	}

	result, err := api.QueryLogs(ctx, query)
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result.Entries))
	}

	if result.Stats == nil {
		t.Error("Expected stats to be present")
	}
}

func TestAPIQueryTraces(t *testing.T) {
	tracesQuerier := NewInMemoryTracesQuerier()

	// Add a test trace
	now := time.Now()
	trace := &TraceResult{
		TraceID: "trace-123",
		Spans: []Span{
			{
				TraceID:       "trace-123",
				SpanID:        "span-1",
				OperationName: "test-operation",
				StartTime:     now.Add(-1 * time.Minute),
				Duration:      100 * time.Millisecond,
			},
		},
		Processes: map[string]Process{
			"p1": {
				ServiceName: "test-service",
			},
		},
	}
	tracesQuerier.AddTrace(trace)

	api := NewAPI(nil, nil, tracesQuerier)

	ctx := context.Background()
	query := &TracesQuery{
		Service: "test-service",
		Limit:   10,
	}

	result, err := api.QueryTraces(ctx, query)
	if err != nil {
		t.Fatalf("QueryTraces failed: %v", err)
	}

	if len(result.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(result.Traces))
	}

	if result.Traces[0].TraceID != "trace-123" {
		t.Errorf("Expected trace ID trace-123, got %s", result.Traces[0].TraceID)
	}
}

func TestAPIGetTrace(t *testing.T) {
	tracesQuerier := NewInMemoryTracesQuerier()

	// Add a test trace
	now := time.Now()
	trace := &TraceResult{
		TraceID: "trace-456",
		Spans: []Span{
			{
				TraceID:       "trace-456",
				SpanID:        "span-1",
				OperationName: "test-operation",
				StartTime:     now.Add(-1 * time.Minute),
				Duration:      100 * time.Millisecond,
			},
		},
	}
	tracesQuerier.AddTrace(trace)

	api := NewAPI(nil, nil, tracesQuerier)

	ctx := context.Background()

	result, err := api.GetTrace(ctx, "trace-456")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}

	if result.TraceID != "trace-456" {
		t.Errorf("Expected trace ID trace-456, got %s", result.TraceID)
	}
}

func TestAPIGetTraceNotFound(t *testing.T) {
	tracesQuerier := NewInMemoryTracesQuerier()
	api := NewAPI(nil, nil, tracesQuerier)

	ctx := context.Background()

	_, err := api.GetTrace(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent trace")
	}
}
