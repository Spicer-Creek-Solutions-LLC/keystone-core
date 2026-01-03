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

// Tests for QueryType constants
func TestQueryTypeValues(t *testing.T) {
	tests := []struct {
		queryType QueryType
		expected  string
	}{
		{QueryTypeMetrics, "metrics"},
		{QueryTypeLogs, "logs"},
		{QueryTypeTraces, "traces"},
	}

	for _, tt := range tests {
		t.Run(string(tt.queryType), func(t *testing.T) {
			if string(tt.queryType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.queryType)
			}
		})
	}
}

// Tests for TimeRange struct
func TestTimeRange(t *testing.T) {
	now := time.Now()
	start := now.Add(-1 * time.Hour)
	end := now

	tr := TimeRange{
		Start: start,
		End:   end,
	}

	if !tr.Start.Equal(start) {
		t.Errorf("Expected Start to be %v, got %v", start, tr.Start)
	}

	if !tr.End.Equal(end) {
		t.Errorf("Expected End to be %v, got %v", end, tr.End)
	}
}

// Tests for MetricsQuery struct
func TestMetricsQuery(t *testing.T) {
	now := time.Now()
	queryTime := now.Add(-5 * time.Minute)
	timeRange := &TimeRange{
		Start: now.Add(-1 * time.Hour),
		End:   now,
	}

	mq := &MetricsQuery{
		Query:   "up{job=\"prometheus\"}",
		Time:    &queryTime,
		Range:   timeRange,
		Step:    15 * time.Second,
		Timeout: 30 * time.Second,
	}

	if mq.Query != "up{job=\"prometheus\"}" {
		t.Errorf("Expected query 'up{job=\"prometheus\"}', got '%s'", mq.Query)
	}

	if !mq.Time.Equal(queryTime) {
		t.Errorf("Expected time %v, got %v", queryTime, mq.Time)
	}

	if mq.Range != timeRange {
		t.Error("Expected range to match")
	}

	if mq.Step != 15*time.Second {
		t.Errorf("Expected step 15s, got %v", mq.Step)
	}

	if mq.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", mq.Timeout)
	}
}

// Tests for MetricsResult struct
func TestMetricsResult(t *testing.T) {
	mr := &MetricsResult{
		ResultType: "vector",
		Result:     []map[string]interface{}{{"value": 1.0}},
		Warnings:   []string{"warning1", "warning2"},
	}

	if mr.ResultType != "vector" {
		t.Errorf("Expected ResultType 'vector', got '%s'", mr.ResultType)
	}

	if mr.Result == nil {
		t.Error("Expected Result to be non-nil")
	}

	if len(mr.Warnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(mr.Warnings))
	}
}

// Tests for LogsQuery struct
func TestLogsQuery(t *testing.T) {
	now := time.Now()
	lq := &LogsQuery{
		Query: `{app="test"} |= "error"`,
		Range: TimeRange{
			Start: now.Add(-1 * time.Hour),
			End:   now,
		},
		Limit:     100,
		Direction: "backward",
		Start:     "cursor123",
	}

	if lq.Query != `{app="test"} |= "error"` {
		t.Errorf("Expected query, got '%s'", lq.Query)
	}

	if lq.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", lq.Limit)
	}

	if lq.Direction != "backward" {
		t.Errorf("Expected direction 'backward', got '%s'", lq.Direction)
	}

	if lq.Start != "cursor123" {
		t.Errorf("Expected start 'cursor123', got '%s'", lq.Start)
	}
}

// Tests for LogEntry struct
func TestLogEntry(t *testing.T) {
	now := time.Now()
	le := LogEntry{
		Timestamp: now,
		Line:      "Error: something went wrong",
		Labels:    map[string]string{"app": "myapp", "env": "production"},
	}

	if !le.Timestamp.Equal(now) {
		t.Errorf("Expected timestamp %v, got %v", now, le.Timestamp)
	}

	if le.Line != "Error: something went wrong" {
		t.Errorf("Expected line, got '%s'", le.Line)
	}

	if le.Labels["app"] != "myapp" {
		t.Errorf("Expected app label 'myapp', got '%s'", le.Labels["app"])
	}

	if le.Labels["env"] != "production" {
		t.Errorf("Expected env label 'production', got '%s'", le.Labels["env"])
	}
}

// Tests for LogsStats and LogsSummary structs
func TestLogsStats(t *testing.T) {
	stats := &LogsStats{
		Summary: LogsSummary{
			BytesProcessed:      1024,
			LinesProcessed:      100,
			TotalBytesProcessed: 2048,
			ExecTime:            0.5,
		},
	}

	if stats.Summary.BytesProcessed != 1024 {
		t.Errorf("Expected BytesProcessed 1024, got %d", stats.Summary.BytesProcessed)
	}

	if stats.Summary.LinesProcessed != 100 {
		t.Errorf("Expected LinesProcessed 100, got %d", stats.Summary.LinesProcessed)
	}

	if stats.Summary.TotalBytesProcessed != 2048 {
		t.Errorf("Expected TotalBytesProcessed 2048, got %d", stats.Summary.TotalBytesProcessed)
	}

	if stats.Summary.ExecTime != 0.5 {
		t.Errorf("Expected ExecTime 0.5, got %f", stats.Summary.ExecTime)
	}
}

// Tests for TracesQuery struct
func TestTracesQuery(t *testing.T) {
	now := time.Now()
	tq := &TracesQuery{
		Service:     "my-service",
		Operation:   "GET /api/users",
		Tags:        map[string]string{"http.status_code": "200"},
		Range:       &TimeRange{Start: now.Add(-1 * time.Hour), End: now},
		MinDuration: 100 * time.Millisecond,
		MaxDuration: 1 * time.Second,
		Limit:       50,
	}

	if tq.Service != "my-service" {
		t.Errorf("Expected service 'my-service', got '%s'", tq.Service)
	}

	if tq.Operation != "GET /api/users" {
		t.Errorf("Expected operation 'GET /api/users', got '%s'", tq.Operation)
	}

	if tq.Tags["http.status_code"] != "200" {
		t.Errorf("Expected tag value '200', got '%s'", tq.Tags["http.status_code"])
	}

	if tq.MinDuration != 100*time.Millisecond {
		t.Errorf("Expected MinDuration 100ms, got %v", tq.MinDuration)
	}

	if tq.MaxDuration != 1*time.Second {
		t.Errorf("Expected MaxDuration 1s, got %v", tq.MaxDuration)
	}

	if tq.Limit != 50 {
		t.Errorf("Expected limit 50, got %d", tq.Limit)
	}
}

// Tests for Span struct
func TestSpan(t *testing.T) {
	now := time.Now()
	span := Span{
		TraceID:       "trace-123",
		SpanID:        "span-456",
		OperationName: "HTTP GET",
		References: []SpanRef{
			{RefType: "CHILD_OF", TraceID: "trace-123", SpanID: "span-parent"},
		},
		StartTime: now,
		Duration:  100 * time.Millisecond,
		Tags: map[string]interface{}{
			"http.method": "GET",
			"http.status": 200,
		},
		Logs: []SpanLog{
			{Timestamp: now, Fields: map[string]interface{}{"message": "test"}},
		},
		ProcessID: "p1",
	}

	if span.TraceID != "trace-123" {
		t.Errorf("Expected TraceID 'trace-123', got '%s'", span.TraceID)
	}

	if span.SpanID != "span-456" {
		t.Errorf("Expected SpanID 'span-456', got '%s'", span.SpanID)
	}

	if span.OperationName != "HTTP GET" {
		t.Errorf("Expected OperationName 'HTTP GET', got '%s'", span.OperationName)
	}

	if len(span.References) != 1 {
		t.Errorf("Expected 1 reference, got %d", len(span.References))
	}

	if span.References[0].RefType != "CHILD_OF" {
		t.Errorf("Expected RefType 'CHILD_OF', got '%s'", span.References[0].RefType)
	}

	if span.Duration != 100*time.Millisecond {
		t.Errorf("Expected Duration 100ms, got %v", span.Duration)
	}

	if span.ProcessID != "p1" {
		t.Errorf("Expected ProcessID 'p1', got '%s'", span.ProcessID)
	}
}

// Tests for Process struct
func TestProcess(t *testing.T) {
	process := Process{
		ServiceName: "my-service",
		Tags: map[string]interface{}{
			"hostname": "host1",
			"version":  "1.0.0",
		},
	}

	if process.ServiceName != "my-service" {
		t.Errorf("Expected ServiceName 'my-service', got '%s'", process.ServiceName)
	}

	if process.Tags["hostname"] != "host1" {
		t.Errorf("Expected hostname 'host1', got '%v'", process.Tags["hostname"])
	}
}

// Tests for TracesResult struct
func TestTracesResult(t *testing.T) {
	now := time.Now()
	tr := &TracesResult{
		Traces: []TraceResult{
			{
				TraceID: "trace-1",
				Spans: []Span{
					{TraceID: "trace-1", SpanID: "span-1", StartTime: now, Duration: 100 * time.Millisecond},
				},
			},
		},
		Total: 1,
	}

	if len(tr.Traces) != 1 {
		t.Errorf("Expected 1 trace, got %d", len(tr.Traces))
	}

	if tr.Total != 1 {
		t.Errorf("Expected Total 1, got %d", tr.Total)
	}
}

// Tests for ErrorResponse struct
func TestErrorResponse(t *testing.T) {
	er := &ErrorResponse{
		Message: "something went wrong",
		Code:    "INTERNAL_ERROR",
	}

	if er.Message != "something went wrong" {
		t.Errorf("Expected Message, got '%s'", er.Message)
	}

	if er.Code != "INTERNAL_ERROR" {
		t.Errorf("Expected Code 'INTERNAL_ERROR', got '%s'", er.Code)
	}

	// Test Error() method
	if er.Error() != "something went wrong" {
		t.Errorf("Expected Error() to return message, got '%s'", er.Error())
	}
}

// Test API with range query
func TestAPIQueryMetricsWithRange(t *testing.T) {
	// API without metrics querier
	api := NewAPI(nil, nil, nil)

	ctx := context.Background()
	now := time.Now()
	query := &MetricsQuery{
		Query: "up",
		Range: &TimeRange{
			Start: now.Add(-1 * time.Hour),
			End:   now,
		},
		Step: 15 * time.Second,
	}

	_, err := api.QueryMetrics(ctx, query)
	if err == nil {
		t.Error("Expected error when metrics querier not configured")
	}
}
