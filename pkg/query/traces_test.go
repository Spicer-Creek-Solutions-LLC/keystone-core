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

// Tests for JaegerConfig
func TestJaegerConfig(t *testing.T) {
	config := &JaegerConfig{
		Address:  "http://jaeger-query:16686",
		Username: "admin",
		Password: "secret",
		Timeout:  60 * time.Second,
	}

	if config.Address != "http://jaeger-query:16686" {
		t.Errorf("Expected Address 'http://jaeger-query:16686', got '%s'", config.Address)
	}

	if config.Username != "admin" {
		t.Errorf("Expected Username 'admin', got '%s'", config.Username)
	}

	if config.Password != "secret" {
		t.Errorf("Expected Password 'secret', got '%s'", config.Password)
	}

	if config.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", config.Timeout)
	}
}

// Tests for NewJaegerQuerier
func TestNewJaegerQuerier(t *testing.T) {
	querier := NewJaegerQuerier("http://jaeger-query:16686")

	if querier == nil {
		t.Fatal("NewJaegerQuerier returned nil")
	}

	if querier.config.Address != "http://jaeger-query:16686" {
		t.Errorf("Expected Address 'http://jaeger-query:16686', got '%s'", querier.config.Address)
	}

	if querier.config.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", querier.config.Timeout)
	}
}

// Tests for NewJaegerQuerierWithConfig
func TestNewJaegerQuerierWithConfig(t *testing.T) {
	config := &JaegerConfig{
		Address:  "http://jaeger-query:16686",
		Username: "admin",
		Password: "secret",
		Timeout:  60 * time.Second,
	}

	querier := NewJaegerQuerierWithConfig(config)

	if querier == nil {
		t.Fatal("NewJaegerQuerierWithConfig returned nil")
	}

	if querier.config.Address != "http://jaeger-query:16686" {
		t.Errorf("Expected Address 'http://jaeger-query:16686', got '%s'", querier.config.Address)
	}

	if querier.config.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", querier.config.Timeout)
	}
}

// Tests for NewJaegerQuerierWithConfig with zero timeout (uses default)
func TestNewJaegerQuerierWithConfig_DefaultTimeout(t *testing.T) {
	config := &JaegerConfig{
		Address: "http://jaeger-query:16686",
		// Timeout not set
	}

	querier := NewJaegerQuerierWithConfig(config)

	if querier.config.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", querier.config.Timeout)
	}
}

// Tests for helper functions
func TestMatchesTags(t *testing.T) {
	tests := []struct {
		name      string
		spanTags  map[string]interface{}
		queryTags map[string]string
		expected  bool
	}{
		{
			name:      "matching single tag",
			spanTags:  map[string]interface{}{"http.status_code": "200"},
			queryTags: map[string]string{"http.status_code": "200"},
			expected:  true,
		},
		{
			name:      "matching multiple tags",
			spanTags:  map[string]interface{}{"http.status_code": "200", "http.method": "GET"},
			queryTags: map[string]string{"http.status_code": "200", "http.method": "GET"},
			expected:  true,
		},
		{
			name:      "non-matching value",
			spanTags:  map[string]interface{}{"http.status_code": "200"},
			queryTags: map[string]string{"http.status_code": "500"},
			expected:  false,
		},
		{
			name:      "missing tag",
			spanTags:  map[string]interface{}{"http.status_code": "200"},
			queryTags: map[string]string{"http.method": "GET"},
			expected:  false,
		},
		{
			name:      "empty query tags",
			spanTags:  map[string]interface{}{"http.status_code": "200"},
			queryTags: map[string]string{},
			expected:  true, // empty query matches everything
		},
		{
			name:      "integer tag value",
			spanTags:  map[string]interface{}{"http.status_code": 200},
			queryTags: map[string]string{"http.status_code": "200"},
			expected:  true, // should convert int to string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesTags(tt.spanTags, tt.queryTags)
			if result != tt.expected {
				t.Errorf("matchesTags() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTraceStartTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		trace    *TraceResult
		expected time.Time
	}{
		{
			name: "single span",
			trace: &TraceResult{
				Spans: []Span{
					{StartTime: now},
				},
			},
			expected: now,
		},
		{
			name: "multiple spans - find earliest",
			trace: &TraceResult{
				Spans: []Span{
					{StartTime: now.Add(10 * time.Second)},
					{StartTime: now.Add(-10 * time.Second)},
					{StartTime: now},
				},
			},
			expected: now.Add(-10 * time.Second),
		},
		{
			name: "empty spans",
			trace: &TraceResult{
				Spans: []Span{},
			},
			expected: time.Time{}, // zero time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTraceStartTime(tt.trace)
			if !result.Equal(tt.expected) {
				t.Errorf("getTraceStartTime() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTraceDuration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		trace    *TraceResult
		expected time.Duration
	}{
		{
			name: "single span",
			trace: &TraceResult{
				Spans: []Span{
					{StartTime: now, Duration: 100 * time.Millisecond},
				},
			},
			expected: 100 * time.Millisecond,
		},
		{
			name: "multiple spans - parallel",
			trace: &TraceResult{
				Spans: []Span{
					{StartTime: now, Duration: 50 * time.Millisecond},
					{StartTime: now.Add(10 * time.Millisecond), Duration: 60 * time.Millisecond},
				},
			},
			expected: 70 * time.Millisecond, // max(50ms, 10ms+60ms)
		},
		{
			name: "empty spans",
			trace: &TraceResult{
				Spans: []Span{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTraceDuration(tt.trace)
			if result != tt.expected {
				t.Errorf("getTraceDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Tests for convertJaegerRefs
func TestConvertJaegerRefs(t *testing.T) {
	refs := []jaegerSpanRef{
		{RefType: "CHILD_OF", TraceID: "trace-123", SpanID: "span-parent"},
		{RefType: "FOLLOWS_FROM", TraceID: "trace-123", SpanID: "span-sibling"},
	}

	result := convertJaegerRefs(refs)

	if len(result) != 2 {
		t.Fatalf("Expected 2 references, got %d", len(result))
	}

	if result[0].RefType != "CHILD_OF" {
		t.Errorf("Expected RefType 'CHILD_OF', got '%s'", result[0].RefType)
	}

	if result[0].TraceID != "trace-123" {
		t.Errorf("Expected TraceID 'trace-123', got '%s'", result[0].TraceID)
	}

	if result[1].RefType != "FOLLOWS_FROM" {
		t.Errorf("Expected RefType 'FOLLOWS_FROM', got '%s'", result[1].RefType)
	}
}

// Tests for convertJaegerTags
func TestConvertJaegerTags(t *testing.T) {
	tags := []jaegerKeyValue{
		{Key: "http.method", Type: "string", Value: "GET"},
		{Key: "http.status_code", Type: "int64", Value: float64(200)},
		{Key: "error", Type: "bool", Value: false},
	}

	result := convertJaegerTags(tags)

	if len(result) != 3 {
		t.Fatalf("Expected 3 tags, got %d", len(result))
	}

	if result["http.method"] != "GET" {
		t.Errorf("Expected http.method 'GET', got '%v'", result["http.method"])
	}

	if result["http.status_code"] != float64(200) {
		t.Errorf("Expected http.status_code 200, got '%v'", result["http.status_code"])
	}

	if result["error"] != false {
		t.Errorf("Expected error false, got '%v'", result["error"])
	}
}

// Tests for convertJaegerLogs
func TestConvertJaegerLogs(t *testing.T) {
	logs := []jaegerLog{
		{
			Timestamp: 1609459200000000, // 2021-01-01 00:00:00 UTC in microseconds
			Fields: []jaegerKeyValue{
				{Key: "message", Value: "test log message"},
				{Key: "level", Value: "info"},
			},
		},
	}

	result := convertJaegerLogs(logs)

	if len(result) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(result))
	}

	expectedTime := time.UnixMicro(1609459200000000)
	if !result[0].Timestamp.Equal(expectedTime) {
		t.Errorf("Expected timestamp %v, got %v", expectedTime, result[0].Timestamp)
	}

	if result[0].Fields["message"] != "test log message" {
		t.Errorf("Expected message 'test log message', got '%v'", result[0].Fields["message"])
	}
}

// Tests for convertJaegerTrace
func TestConvertJaegerTrace(t *testing.T) {
	jt := &jaegerTrace{
		TraceID: "trace-123",
		Spans: []jaegerSpan{
			{
				TraceID:       "trace-123",
				SpanID:        "span-456",
				OperationName: "HTTP GET /api/users",
				StartTime:     1609459200000000, // microseconds
				Duration:      100000,           // 100ms in microseconds
				ProcessID:     "p1",
				Tags: []jaegerKeyValue{
					{Key: "http.method", Value: "GET"},
				},
			},
		},
		Processes: map[string]jaegerProcess{
			"p1": {
				ServiceName: "my-service",
				Tags: []jaegerKeyValue{
					{Key: "hostname", Value: "host1"},
				},
			},
		},
		Warnings: []string{"test warning"},
	}

	result := convertJaegerTrace(jt)

	if result.TraceID != "trace-123" {
		t.Errorf("Expected TraceID 'trace-123', got '%s'", result.TraceID)
	}

	if len(result.Spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(result.Spans))
	}

	if result.Spans[0].SpanID != "span-456" {
		t.Errorf("Expected SpanID 'span-456', got '%s'", result.Spans[0].SpanID)
	}

	if result.Spans[0].OperationName != "HTTP GET /api/users" {
		t.Errorf("Expected OperationName 'HTTP GET /api/users', got '%s'", result.Spans[0].OperationName)
	}

	expectedDuration := 100 * time.Millisecond
	if result.Spans[0].Duration != expectedDuration {
		t.Errorf("Expected Duration %v, got %v", expectedDuration, result.Spans[0].Duration)
	}

	if len(result.Processes) != 1 {
		t.Fatalf("Expected 1 process, got %d", len(result.Processes))
	}

	if result.Processes["p1"].ServiceName != "my-service" {
		t.Errorf("Expected ServiceName 'my-service', got '%s'", result.Processes["p1"].ServiceName)
	}

	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}

	if result.Warnings[0] != "test warning" {
		t.Errorf("Expected warning 'test warning', got '%s'", result.Warnings[0])
	}
}

// Test TraceResult with Warnings
func TestTraceResultWithWarnings(t *testing.T) {
	tr := &TraceResult{
		TraceID:  "trace-123",
		Spans:    []Span{},
		Warnings: []string{"warning1", "warning2"},
	}

	if len(tr.Warnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(tr.Warnings))
	}

	if tr.Warnings[0] != "warning1" {
		t.Errorf("Expected warning 'warning1', got '%s'", tr.Warnings[0])
	}
}

// Test SpanRef struct
func TestSpanRef(t *testing.T) {
	ref := SpanRef{
		RefType: "CHILD_OF",
		TraceID: "trace-123",
		SpanID:  "span-parent",
	}

	if ref.RefType != "CHILD_OF" {
		t.Errorf("Expected RefType 'CHILD_OF', got '%s'", ref.RefType)
	}

	if ref.TraceID != "trace-123" {
		t.Errorf("Expected TraceID 'trace-123', got '%s'", ref.TraceID)
	}

	if ref.SpanID != "span-parent" {
		t.Errorf("Expected SpanID 'span-parent', got '%s'", ref.SpanID)
	}
}

// Test SpanLog struct
func TestSpanLog(t *testing.T) {
	now := time.Now()
	log := SpanLog{
		Timestamp: now,
		Fields: map[string]interface{}{
			"message": "test message",
			"level":   "info",
		},
	}

	if !log.Timestamp.Equal(now) {
		t.Errorf("Expected Timestamp %v, got %v", now, log.Timestamp)
	}

	if log.Fields["message"] != "test message" {
		t.Errorf("Expected message 'test message', got '%v'", log.Fields["message"])
	}
}

// Test matchesQuery function
func TestMatchesQuery(t *testing.T) {
	now := time.Now()

	baseTrace := &TraceResult{
		TraceID: "trace-123",
		Spans: []Span{
			{
				TraceID:       "trace-123",
				SpanID:        "span-1",
				OperationName: "GET /api/users",
				StartTime:     now,
				Duration:      100 * time.Millisecond,
				Tags: map[string]interface{}{
					"http.method": "GET",
				},
			},
		},
		Processes: map[string]Process{
			"p1": {ServiceName: "my-service"},
		},
	}

	tests := []struct {
		name     string
		trace    *TraceResult
		query    *TracesQuery
		expected bool
	}{
		{
			name:     "empty query matches all",
			trace:    baseTrace,
			query:    &TracesQuery{},
			expected: true,
		},
		{
			name:  "matching service",
			trace: baseTrace,
			query: &TracesQuery{
				Service: "my-service",
			},
			expected: true,
		},
		{
			name:  "non-matching service",
			trace: baseTrace,
			query: &TracesQuery{
				Service: "other-service",
			},
			expected: false,
		},
		{
			name:  "matching operation",
			trace: baseTrace,
			query: &TracesQuery{
				Operation: "GET /api/users",
			},
			expected: true,
		},
		{
			name:  "non-matching operation",
			trace: baseTrace,
			query: &TracesQuery{
				Operation: "POST /api/users",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesQuery(tt.trace, tt.query)
			if result != tt.expected {
				t.Errorf("matchesQuery() = %v, want %v", result, tt.expected)
			}
		})
	}
}
