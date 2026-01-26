package traces

import (
	"testing"
	"time"
)

func TestTracesStore_Store(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewTracesStore(config)

	spans := []Span{
		{
			TraceID:       "trace-1",
			SpanID:        "span-1",
			OperationName: "test-op",
			ServiceName:   "test-service",
			StartTime:     time.Now().Add(-100 * time.Millisecond),
			EndTime:       time.Now(),
			Status:        SpanStatusOK,
		},
	}

	stored := store.Store(spans)
	if stored != 1 {
		t.Errorf("Store() returned %d, want 1", stored)
	}

	stats := store.Stats()
	if stats.TraceCount != 1 {
		t.Errorf("TraceCount = %d, want 1", stats.TraceCount)
	}
	if stats.SpansReceived != 1 {
		t.Errorf("SpansReceived = %d, want 1", stats.SpansReceived)
	}
}

func TestTracesStore_Get(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewTracesStore(config)

	now := time.Now()
	spans := []Span{
		{
			TraceID:       "trace-1",
			SpanID:        "span-1",
			OperationName: "root",
			ServiceName:   "frontend",
			StartTime:     now.Add(-200 * time.Millisecond),
			EndTime:       now,
		},
		{
			TraceID:       "trace-1",
			SpanID:        "span-2",
			ParentSpanID:  "span-1",
			OperationName: "child",
			ServiceName:   "backend",
			StartTime:     now.Add(-150 * time.Millisecond),
			EndTime:       now.Add(-50 * time.Millisecond),
		},
	}

	store.Store(spans)

	trace, exists := store.Get("trace-1")
	if !exists {
		t.Fatal("Get() returned false, want true")
	}

	if len(trace.Spans) != 2 {
		t.Errorf("len(trace.Spans) = %d, want 2", len(trace.Spans))
	}

	if trace.ServiceName != "frontend" {
		t.Errorf("trace.ServiceName = %s, want frontend", trace.ServiceName)
	}

	if trace.OperationName != "root" {
		t.Errorf("trace.OperationName = %s, want root", trace.OperationName)
	}
}

func TestTracesStore_Sampling(t *testing.T) {
	config := StoreConfig{
		MaxTraces:    1000,
		MaxAge:       1 * time.Hour,
		SamplingRate: 0.0, // Drop everything
		SampleErrors: true,
	}
	store := NewTracesStore(config)

	// Normal span should be dropped
	store.Store([]Span{{
		TraceID: "trace-1",
		SpanID:  "span-1",
		Status:  SpanStatusOK,
	}})

	// Error span should be kept (SampleErrors = true)
	store.Store([]Span{{
		TraceID: "trace-2",
		SpanID:  "span-2",
		Status:  SpanStatusError,
	}})

	stats := store.Stats()
	if stats.TraceCount != 1 {
		t.Errorf("TraceCount = %d, want 1 (only error trace)", stats.TraceCount)
	}
}

func TestTracesStore_SlowThreshold(t *testing.T) {
	config := StoreConfig{
		MaxTraces:     1000,
		MaxAge:        1 * time.Hour,
		SamplingRate:  0.0, // Drop everything
		SlowThreshold: 100 * time.Millisecond,
	}
	store := NewTracesStore(config)

	now := time.Now()

	// Fast span should be dropped
	store.Store([]Span{{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		StartTime: now.Add(-50 * time.Millisecond),
		EndTime:   now,
		Duration:  50 * time.Millisecond,
	}})

	// Slow span should be kept
	store.Store([]Span{{
		TraceID:   "trace-2",
		SpanID:    "span-2",
		StartTime: now.Add(-200 * time.Millisecond),
		EndTime:   now,
		Duration:  200 * time.Millisecond,
	}})

	stats := store.Stats()
	if stats.TraceCount != 1 {
		t.Errorf("TraceCount = %d, want 1 (only slow trace)", stats.TraceCount)
	}
}

func TestTracesStore_Query(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewTracesStore(config)

	now := time.Now()

	// Add traces with different characteristics
	store.Store([]Span{{
		TraceID:       "trace-1",
		SpanID:        "span-1",
		ServiceName:   "frontend",
		OperationName: "handle-request",
		StartTime:     now.Add(-200 * time.Millisecond),
		EndTime:       now,
		Duration:      200 * time.Millisecond,
		Status:        SpanStatusOK,
	}})

	store.Store([]Span{{
		TraceID:       "trace-2",
		SpanID:        "span-2",
		ServiceName:   "backend",
		OperationName: "query-db",
		StartTime:     now.Add(-100 * time.Millisecond),
		EndTime:       now,
		Duration:      100 * time.Millisecond,
		Status:        SpanStatusError,
	}})

	// Query by service
	results := store.Query(TraceQuery{ServiceName: "frontend"})
	if len(results) != 1 {
		t.Errorf("Query by service: got %d, want 1", len(results))
	}

	// Query by min duration
	results = store.Query(TraceQuery{MinDuration: 150 * time.Millisecond})
	if len(results) != 1 {
		t.Errorf("Query by min duration: got %d, want 1", len(results))
	}

	// Query with errors
	hasError := true
	results = store.Query(TraceQuery{HasError: &hasError})
	if len(results) != 1 {
		t.Errorf("Query with errors: got %d, want 1", len(results))
	}
}

func TestTracesStore_Cleanup(t *testing.T) {
	config := StoreConfig{
		MaxTraces:    1000,
		MaxAge:       100 * time.Millisecond,
		SamplingRate: 1.0, // Keep all traces
	}
	store := NewTracesStore(config)

	now := time.Now()

	// Old trace
	store.Store([]Span{{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		StartTime: now.Add(-200 * time.Millisecond),
		EndTime:   now.Add(-150 * time.Millisecond),
	}})

	// New trace
	store.Store([]Span{{
		TraceID:   "trace-2",
		SpanID:    "span-2",
		StartTime: now.Add(-50 * time.Millisecond),
		EndTime:   now,
	}})

	removed := store.Cleanup()
	if removed != 1 {
		t.Errorf("Cleanup() removed %d, want 1", removed)
	}

	stats := store.Stats()
	if stats.TraceCount != 1 {
		t.Errorf("TraceCount after Cleanup = %d, want 1", stats.TraceCount)
	}
}

func TestTracesStore_Remove(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewTracesStore(config)

	store.Store([]Span{{
		TraceID: "trace-1",
		SpanID:  "span-1",
	}})

	if stats := store.Stats(); stats.TraceCount != 1 {
		t.Fatalf("TraceCount = %d, want 1", stats.TraceCount)
	}

	store.Remove("trace-1")

	if stats := store.Stats(); stats.TraceCount != 0 {
		t.Errorf("TraceCount after Remove = %d, want 0", stats.TraceCount)
	}
}

func TestTracesStore_GetAll(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewTracesStore(config)

	store.Store([]Span{{TraceID: "trace-1", SpanID: "span-1"}})
	store.Store([]Span{{TraceID: "trace-2", SpanID: "span-2"}})
	store.Store([]Span{{TraceID: "trace-3", SpanID: "span-3"}})

	all := store.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll() returned %d traces, want 3", len(all))
	}
}

func TestDefaultStoreConfig(t *testing.T) {
	config := DefaultStoreConfig()

	if config.MaxTraces != 10000 {
		t.Errorf("MaxTraces = %d, want 10000", config.MaxTraces)
	}
	if config.MaxAge != 1*time.Hour {
		t.Errorf("MaxAge = %v, want 1h", config.MaxAge)
	}
	if config.SamplingRate != 1.0 {
		t.Errorf("SamplingRate = %f, want 1.0", config.SamplingRate)
	}
	if !config.SampleErrors {
		t.Error("SampleErrors = false, want true")
	}
	if config.SlowThreshold != 1*time.Second {
		t.Errorf("SlowThreshold = %v, want 1s", config.SlowThreshold)
	}
}
