package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCorrelationIDGenerator(t *testing.T) {
	gen := NewCorrelationIDGenerator(nil)
	if gen == nil {
		t.Fatal("Expected generator to be created")
	}
}

func TestCorrelationIDGenerator_Generate(t *testing.T) {
	gen := NewCorrelationIDGenerator(nil)

	id1 := gen.Generate()
	id2 := gen.Generate()

	if id1 == "" {
		t.Error("Expected non-empty ID")
	}
	if id1 == id2 {
		t.Error("Expected unique IDs")
	}
	// Default 16 bytes = 32 hex chars
	if len(id1) != 32 {
		t.Errorf("Expected ID length 32, got %d", len(id1))
	}
}

func TestCorrelationIDGenerator_CustomLength(t *testing.T) {
	config := &CorrelationConfig{
		IDLength: 8,
	}
	gen := NewCorrelationIDGenerator(config)

	id := gen.Generate()
	// 8 bytes = 16 hex chars
	if len(id) != 16 {
		t.Errorf("Expected ID length 16, got %d", len(id))
	}
}

func TestCorrelationPropagator_Extract(t *testing.T) {
	config := DefaultCorrelationConfig()
	propagator := NewCorrelationPropagator(config)

	// Helper to create headers properly (http.Header uses canonical keys)
	makeHeaders := func(pairs ...string) http.Header {
		h := http.Header{}
		for i := 0; i < len(pairs); i += 2 {
			h.Set(pairs[i], pairs[i+1])
		}
		return h
	}

	tests := []struct {
		name     string
		headers  http.Header
		expected string
	}{
		{
			name:     "correlation_id_header",
			headers:  makeHeaders(HeaderCorrelationID, "test-correlation-123"),
			expected: "test-correlation-123",
		},
		{
			name:     "request_id_header",
			headers:  makeHeaders(HeaderRequestID, "test-request-456"),
			expected: "test-request-456",
		},
		{
			name:     "trace_id_header",
			headers:  makeHeaders(HeaderTraceID, "test-trace-789"),
			expected: "test-trace-789",
		},
		{
			name: "precedence",
			headers: makeHeaders(
				HeaderCorrelationID, "correlation",
				HeaderRequestID, "request",
				HeaderTraceID, "trace",
			),
			expected: "correlation", // First in precedence
		},
		{
			name:     "empty_headers",
			headers:  http.Header{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := propagator.Extract(tt.headers)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestCorrelationPropagator_ExtractFromMap(t *testing.T) {
	config := DefaultCorrelationConfig()
	propagator := NewCorrelationPropagator(config)

	headers := map[string]string{
		HeaderCorrelationID: "map-correlation-123",
	}

	result := propagator.ExtractFromMap(headers)
	if result != "map-correlation-123" {
		t.Errorf("Expected 'map-correlation-123', got '%s'", result)
	}
}

func TestCorrelationPropagator_Inject(t *testing.T) {
	config := DefaultCorrelationConfig()
	propagator := NewCorrelationPropagator(config)

	headers := http.Header{}
	propagator.Inject(headers, "injected-id")

	if headers.Get(HeaderCorrelationID) != "injected-id" {
		t.Errorf("Expected 'injected-id', got '%s'", headers.Get(HeaderCorrelationID))
	}
}

func TestCorrelationPropagator_InjectToMap(t *testing.T) {
	config := DefaultCorrelationConfig()
	propagator := NewCorrelationPropagator(config)

	headers := make(map[string]string)
	propagator.InjectToMap(headers, "map-injected-id")

	if headers[HeaderCorrelationID] != "map-injected-id" {
		t.Errorf("Expected 'map-injected-id', got '%s'", headers[HeaderCorrelationID])
	}
}

func TestCorrelationPropagator_GetOrGenerate(t *testing.T) {
	config := DefaultCorrelationConfig()
	propagator := NewCorrelationPropagator(config)

	// With existing header
	headers := http.Header{}
	headers.Set(HeaderCorrelationID, "existing-id")
	result := propagator.GetOrGenerate(headers)
	if result != "existing-id" {
		t.Errorf("Expected 'existing-id', got '%s'", result)
	}

	// Without header (should generate)
	emptyHeaders := http.Header{}
	result = propagator.GetOrGenerate(emptyHeaders)
	if result == "" {
		t.Error("Expected generated ID")
	}
}

func TestCorrelationPropagator_GetOrGenerate_NoGenerate(t *testing.T) {
	config := &CorrelationConfig{
		GenerateIfMissing: false,
		HeaderNames:       []string{HeaderCorrelationID},
	}
	propagator := NewCorrelationPropagator(config)

	emptyHeaders := http.Header{}
	result := propagator.GetOrGenerate(emptyHeaders)
	if result != "" {
		t.Error("Expected empty ID when GenerateIfMissing is false")
	}
}

func TestCorrelationContext(t *testing.T) {
	cc := NewCorrelationContext("test-correlation")

	if cc.CorrelationID != "test-correlation" {
		t.Error("Expected correlation ID to be set")
	}
	if cc.StartTime.IsZero() {
		t.Error("Expected start time to be set")
	}

	cc.WithRequestID("request-123").
		WithSource("test-source").
		WithMetadata("key", "value")

	if cc.RequestID != "request-123" {
		t.Error("Expected request ID to be set")
	}
	if cc.Source != "test-source" {
		t.Error("Expected source to be set")
	}
	if cc.Metadata["key"] != "value" {
		t.Error("Expected metadata to be set")
	}
}

func TestWithCorrelationID_Context(t *testing.T) {
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "ctx-correlation-123")

	result := CorrelationIDFromContext(ctx)
	if result != "ctx-correlation-123" {
		t.Errorf("Expected 'ctx-correlation-123', got '%s'", result)
	}
}

func TestWithRequestID_Context(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "ctx-request-456")

	result := RequestIDFromContext(ctx)
	if result != "ctx-request-456" {
		t.Errorf("Expected 'ctx-request-456', got '%s'", result)
	}
}

func TestCorrelationIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	result := CorrelationIDFromContext(ctx)
	if result != "" {
		t.Error("Expected empty string for missing correlation ID")
	}
}

func TestWithCorrelationContext(t *testing.T) {
	cc := NewCorrelationContext("full-context-id")
	cc.WithRequestID("full-request-id")

	ctx := context.Background()
	ctx = WithCorrelationContext(ctx, cc)

	if CorrelationIDFromContext(ctx) != "full-context-id" {
		t.Error("Expected correlation ID to be set")
	}
	if RequestIDFromContext(ctx) != "full-request-id" {
		t.Error("Expected request ID to be set")
	}
}

func TestCorrelationContextFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "extracted-correlation")
	ctx = WithRequestID(ctx, "extracted-request")

	cc := CorrelationContextFromContext(ctx)

	if cc.CorrelationID != "extracted-correlation" {
		t.Error("Expected correlation ID to be extracted")
	}
	if cc.RequestID != "extracted-request" {
		t.Error("Expected request ID to be extracted")
	}
}

func TestCorrelationMiddleware(t *testing.T) {
	config := DefaultCorrelationConfig()
	middleware := CorrelationMiddleware(config)

	var capturedCorrelationID string
	var capturedRequestID string

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorrelationID = CorrelationIDFromContext(r.Context())
		capturedRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Test with existing correlation ID
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(HeaderCorrelationID, "middleware-correlation")
	req.Header.Set(HeaderRequestID, "middleware-request")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedCorrelationID != "middleware-correlation" {
		t.Errorf("Expected 'middleware-correlation', got '%s'", capturedCorrelationID)
	}
	if capturedRequestID != "middleware-request" {
		t.Errorf("Expected 'middleware-request', got '%s'", capturedRequestID)
	}

	// Check response headers
	if rr.Header().Get(HeaderCorrelationID) != "middleware-correlation" {
		t.Error("Expected correlation ID in response header")
	}
	if rr.Header().Get(HeaderRequestID) != "middleware-request" {
		t.Error("Expected request ID in response header")
	}
}

func TestCorrelationMiddleware_Generate(t *testing.T) {
	config := DefaultCorrelationConfig()
	middleware := CorrelationMiddleware(config)

	var capturedCorrelationID string

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorrelationID = CorrelationIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Test without correlation ID (should generate)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedCorrelationID == "" {
		t.Error("Expected generated correlation ID")
	}

	// Check it's also in response
	if rr.Header().Get(HeaderCorrelationID) == "" {
		t.Error("Expected correlation ID in response header")
	}
}

func TestCorrelationRoundTripper(t *testing.T) {
	var capturedCorrelationID string
	var capturedRequestID string

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorrelationID = r.Header.Get(HeaderCorrelationID)
		capturedRequestID = r.Header.Get(HeaderRequestID)
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	config := DefaultCorrelationConfig()
	transport := NewCorrelationRoundTripper(http.DefaultTransport, config)
	client := &http.Client{Transport: transport}

	// Create request with context
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "rt-correlation")
	ctx = WithRequestID(ctx, "rt-request")

	req, _ := http.NewRequestWithContext(ctx, "GET", testServer.URL, nil)
	_, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if capturedCorrelationID != "rt-correlation" {
		t.Errorf("Expected 'rt-correlation', got '%s'", capturedCorrelationID)
	}
	if capturedRequestID != "rt-request" {
		t.Errorf("Expected 'rt-request', got '%s'", capturedRequestID)
	}
}

func TestCorrelationRoundTripper_Generate(t *testing.T) {
	var capturedCorrelationID string

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCorrelationID = r.Header.Get(HeaderCorrelationID)
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	config := DefaultCorrelationConfig()
	transport := NewCorrelationRoundTripper(http.DefaultTransport, config)
	client := &http.Client{Transport: transport}

	// Create request without correlation ID
	req, _ := http.NewRequest("GET", testServer.URL, nil)
	_, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if capturedCorrelationID == "" {
		t.Error("Expected generated correlation ID")
	}
}

func TestNATSCorrelationHeaders(t *testing.T) {
	headers := &NATSCorrelationHeaders{
		CorrelationID: "nats-correlation",
		RequestID:     "nats-request",
		TraceID:       "nats-trace",
	}

	m := headers.ToMap()
	if m[HeaderCorrelationID] != "nats-correlation" {
		t.Error("Expected correlation ID in map")
	}
	if m[HeaderRequestID] != "nats-request" {
		t.Error("Expected request ID in map")
	}
	if m[HeaderTraceID] != "nats-trace" {
		t.Error("Expected trace ID in map")
	}

	// Test FromMap
	newHeaders := &NATSCorrelationHeaders{}
	newHeaders.FromMap(m)
	if newHeaders.CorrelationID != "nats-correlation" {
		t.Error("Expected correlation ID from map")
	}
}

func TestExtractNATSCorrelation(t *testing.T) {
	config := DefaultCorrelationConfig()
	headers := map[string]string{
		HeaderCorrelationID: "nats-extracted-correlation",
		HeaderRequestID:     "nats-extracted-request",
		HeaderTraceID:       "nats-extracted-trace",
	}

	cc := ExtractNATSCorrelation(headers, config)

	if cc.CorrelationID != "nats-extracted-correlation" {
		t.Error("Expected correlation ID to be extracted")
	}
	if cc.RequestID != "nats-extracted-request" {
		t.Error("Expected request ID to be extracted")
	}
	if cc.TraceID != "nats-extracted-trace" {
		t.Error("Expected trace ID to be extracted")
	}
}

func TestInjectNATSCorrelation(t *testing.T) {
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "inject-nats-correlation")
	ctx = WithRequestID(ctx, "inject-nats-request")

	headers := make(map[string]string)
	InjectNATSCorrelation(ctx, headers)

	if headers[HeaderCorrelationID] != "inject-nats-correlation" {
		t.Error("Expected correlation ID to be injected")
	}
	if headers[HeaderRequestID] != "inject-nats-request" {
		t.Error("Expected request ID to be injected")
	}
}

func TestCorrelationLogFields(t *testing.T) {
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "log-correlation")
	ctx = WithRequestID(ctx, "log-request")

	fields := CorrelationLogFields(ctx)

	if fields["correlation_id"] != "log-correlation" {
		t.Error("Expected correlation_id in log fields")
	}
	if fields["request_id"] != "log-request" {
		t.Error("Expected request_id in log fields")
	}
}

func TestDefaultCorrelationConfig(t *testing.T) {
	config := DefaultCorrelationConfig()

	if !config.GenerateIfMissing {
		t.Error("Expected GenerateIfMissing to be true")
	}
	if !config.PropagateToChildren {
		t.Error("Expected PropagateToChildren to be true")
	}
	if !config.IncludeInLogs {
		t.Error("Expected IncludeInLogs to be true")
	}
	if config.IncludeInMetrics {
		t.Error("Expected IncludeInMetrics to be false (high cardinality)")
	}
	if len(config.HeaderNames) != 3 {
		t.Errorf("Expected 3 header names, got %d", len(config.HeaderNames))
	}
	if config.IDLength != 16 {
		t.Errorf("Expected IDLength 16, got %d", config.IDLength)
	}
}
