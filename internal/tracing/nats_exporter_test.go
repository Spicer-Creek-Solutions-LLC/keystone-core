package tracing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultNATSExporterConfig(t *testing.T) {
	config := DefaultNATSExporterConfig()

	if config.URL != "nats://localhost:4222" {
		t.Errorf("Expected URL nats://localhost:4222, got %s", config.URL)
	}

	if config.Subject != "kscore.traces" {
		t.Errorf("Expected Subject kscore.traces, got %s", config.Subject)
	}

	if config.BatchSize != 100 {
		t.Errorf("Expected BatchSize 100, got %d", config.BatchSize)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("Expected FlushInterval 5s, got %v", config.FlushInterval)
	}

	if config.BufferSize != 10000 {
		t.Errorf("Expected BufferSize 10000, got %d", config.BufferSize)
	}

	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("Expected ConnectTimeout 5s, got %v", config.ConnectTimeout)
	}

	if config.MaxReconnects != -1 {
		t.Errorf("Expected MaxReconnects -1, got %d", config.MaxReconnects)
	}

	if config.SubjectPerService {
		t.Error("Expected SubjectPerService to be false by default")
	}
}

func TestNATSSpan(t *testing.T) {
	span := NATSSpan{
		TraceID:      "trace123",
		SpanID:       "span456",
		ParentSpanID: "parent789",
		Name:         "test-operation",
		Kind:         "server",
		StartTime:    "2024-01-15T10:30:00Z",
		EndTime:      "2024-01-15T10:30:01Z",
		Duration:     1000000000,
		Status:       "ok",
		StatusMsg:    "",
		Attributes: map[string]interface{}{
			"http.method": "GET",
			"http.url":    "/api/test",
		},
		Service: "test-service",
		Host:    "localhost",
		Version: "1.0.0",
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpan: %v", err)
	}

	var decoded NATSSpan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpan: %v", err)
	}

	if decoded.TraceID != span.TraceID {
		t.Errorf("Expected TraceID %s, got %s", span.TraceID, decoded.TraceID)
	}

	if decoded.SpanID != span.SpanID {
		t.Errorf("Expected SpanID %s, got %s", span.SpanID, decoded.SpanID)
	}

	if decoded.ParentSpanID != span.ParentSpanID {
		t.Errorf("Expected ParentSpanID %s, got %s", span.ParentSpanID, decoded.ParentSpanID)
	}

	if decoded.Name != span.Name {
		t.Errorf("Expected Name %s, got %s", span.Name, decoded.Name)
	}

	if decoded.Kind != span.Kind {
		t.Errorf("Expected Kind %s, got %s", span.Kind, decoded.Kind)
	}

	if decoded.Duration != span.Duration {
		t.Errorf("Expected Duration %d, got %d", span.Duration, decoded.Duration)
	}

	if decoded.Status != span.Status {
		t.Errorf("Expected Status %s, got %s", span.Status, decoded.Status)
	}

	if decoded.Attributes["http.method"] != "GET" {
		t.Errorf("Expected Attributes[http.method] = 'GET', got %v", decoded.Attributes["http.method"])
	}
}

func TestNATSSpanEvent(t *testing.T) {
	event := NATSSpanEvent{
		Name:      "exception",
		Timestamp: "2024-01-15T10:30:00.5Z",
		Attributes: map[string]interface{}{
			"exception.type":    "RuntimeError",
			"exception.message": "Something went wrong",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpanEvent: %v", err)
	}

	var decoded NATSSpanEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpanEvent: %v", err)
	}

	if decoded.Name != event.Name {
		t.Errorf("Expected Name %s, got %s", event.Name, decoded.Name)
	}

	if decoded.Timestamp != event.Timestamp {
		t.Errorf("Expected Timestamp %s, got %s", event.Timestamp, decoded.Timestamp)
	}
}

func TestNATSSpanLink(t *testing.T) {
	link := NATSSpanLink{
		TraceID: "linked-trace",
		SpanID:  "linked-span",
		Attributes: map[string]interface{}{
			"link.reason": "parent",
		},
	}

	data, err := json.Marshal(link)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpanLink: %v", err)
	}

	var decoded NATSSpanLink
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpanLink: %v", err)
	}

	if decoded.TraceID != link.TraceID {
		t.Errorf("Expected TraceID %s, got %s", link.TraceID, decoded.TraceID)
	}

	if decoded.SpanID != link.SpanID {
		t.Errorf("Expected SpanID %s, got %s", link.SpanID, decoded.SpanID)
	}
}

func TestNATSSpanBatch(t *testing.T) {
	batch := NATSSpanBatch{
		Spans: []NATSSpan{
			{TraceID: "trace1", SpanID: "span1", Name: "op1"},
			{TraceID: "trace1", SpanID: "span2", Name: "op2"},
		},
		Timestamp: "2024-01-15T10:30:00Z",
		Service:   "test-service",
		Host:      "localhost",
	}

	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpanBatch: %v", err)
	}

	var decoded NATSSpanBatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpanBatch: %v", err)
	}

	if len(decoded.Spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(decoded.Spans))
	}

	if decoded.Service != "test-service" {
		t.Errorf("Expected Service 'test-service', got %s", decoded.Service)
	}
}

func TestNATSSpanExporterBuildSubject(t *testing.T) {
	tests := []struct {
		name     string
		config   *NATSExporterConfig
		expected string
	}{
		{
			name: "base subject only",
			config: &NATSExporterConfig{
				Subject:           "kscore.traces",
				SubjectPerService: false,
			},
			expected: "kscore.traces",
		},
		{
			name: "with service",
			config: &NATSExporterConfig{
				Subject:           "kscore.traces",
				SubjectPerService: true,
				ServiceName:       "kscore-server",
			},
			expected: "kscore.traces.kscore-server",
		},
		{
			name: "empty service name",
			config: &NATSExporterConfig{
				Subject:           "kscore.traces",
				SubjectPerService: true,
				ServiceName:       "",
			},
			expected: "kscore.traces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := &NATSSpanExporter{
				config: tt.config,
			}
			result := exporter.buildSubject()
			if result != tt.expected {
				t.Errorf("buildSubject() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestNATSSpanExporterStats(t *testing.T) {
	exporter := &NATSSpanExporter{
		spansExported: 100,
		spansDropped:  5,
		batchesSent:   10,
		lastError:     nil,
		lastErrorTime: time.Time{},
	}

	exported, dropped, batches, lastErrTime, lastErr := exporter.Stats()

	if exported != 100 {
		t.Errorf("Expected exported 100, got %d", exported)
	}

	if dropped != 5 {
		t.Errorf("Expected dropped 5, got %d", dropped)
	}

	if batches != 10 {
		t.Errorf("Expected batches 10, got %d", batches)
	}

	if lastErr != nil {
		t.Errorf("Expected lastErr nil, got %v", lastErr)
	}

	if !lastErrTime.IsZero() {
		t.Errorf("Expected lastErrTime to be zero, got %v", lastErrTime)
	}
}

func TestNATSSpanExporterIsConnectedNil(t *testing.T) {
	exporter := &NATSSpanExporter{
		conn: nil,
	}

	if exporter.IsConnected() {
		t.Error("Expected IsConnected() to return false when conn is nil")
	}
}

func TestNATSSpanExporterExportSpanClosed(t *testing.T) {
	exporter := &NATSSpanExporter{
		closed: true,
		spans:  make(chan *NATSSpan, 10),
	}

	err := exporter.ExportSpan(&NATSSpan{TraceID: "test"})
	if err == nil {
		t.Error("Expected error when exporting to closed exporter")
	}
}

func TestNATSSpanExporterExportSpanBufferFull(t *testing.T) {
	exporter := &NATSSpanExporter{
		closed: false,
		spans:  make(chan *NATSSpan, 1), // Small buffer
	}

	// Fill the buffer
	exporter.spans <- &NATSSpan{TraceID: "first"}

	// Try to export when buffer is full
	err := exporter.ExportSpan(&NATSSpan{TraceID: "second"})
	if err == nil {
		t.Error("Expected error when buffer is full")
	}

	if exporter.spansDropped != 1 {
		t.Errorf("Expected spansDropped 1, got %d", exporter.spansDropped)
	}
}

func TestNATSSpanExporterExportSpan(t *testing.T) {
	exporter := &NATSSpanExporter{
		closed: false,
		spans:  make(chan *NATSSpan, 10),
	}

	span := &NATSSpan{
		TraceID: "test-trace",
		SpanID:  "test-span",
		Name:    "test-op",
	}

	err := exporter.ExportSpan(span)
	if err != nil {
		t.Errorf("ExportSpan() error = %v", err)
	}

	// Verify span was buffered
	select {
	case s := <-exporter.spans:
		if s.TraceID != "test-trace" {
			t.Errorf("Expected TraceID 'test-trace', got %s", s.TraceID)
		}
	default:
		t.Error("Expected span in buffer")
	}
}

func TestNATSSpanSubscriberUnsubscribeNil(t *testing.T) {
	subscriber := &NATSSpanSubscriber{
		sub: nil,
	}

	err := subscriber.Unsubscribe()
	if err != nil {
		t.Errorf("Unsubscribe() with nil subscription should not error, got %v", err)
	}
}

func TestNATSSpanSubscriberClose(t *testing.T) {
	subscriber := &NATSSpanSubscriber{
		sub: nil,
	}

	err := subscriber.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestSpanBuilder(t *testing.T) {
	now := time.Now()
	later := now.Add(100 * time.Millisecond)

	span := NewSpanBuilder("trace123", "span456", "test-operation").
		WithParentSpanID("parent789").
		WithKind("server").
		WithStartTime(now).
		WithEndTime(later).
		WithStatus("ok", "").
		WithAttribute("http.method", "GET").
		WithAttribute("http.status_code", 200).
		WithService("test-service").
		WithHost("localhost").
		WithVersion("1.0.0").
		Build()

	if span.TraceID != "trace123" {
		t.Errorf("Expected TraceID 'trace123', got %s", span.TraceID)
	}

	if span.SpanID != "span456" {
		t.Errorf("Expected SpanID 'span456', got %s", span.SpanID)
	}

	if span.ParentSpanID != "parent789" {
		t.Errorf("Expected ParentSpanID 'parent789', got %s", span.ParentSpanID)
	}

	if span.Name != "test-operation" {
		t.Errorf("Expected Name 'test-operation', got %s", span.Name)
	}

	if span.Kind != "server" {
		t.Errorf("Expected Kind 'server', got %s", span.Kind)
	}

	if span.Status != "ok" {
		t.Errorf("Expected Status 'ok', got %s", span.Status)
	}

	if span.Service != "test-service" {
		t.Errorf("Expected Service 'test-service', got %s", span.Service)
	}

	if span.Attributes["http.method"] != "GET" {
		t.Errorf("Expected Attributes[http.method] = 'GET', got %v", span.Attributes["http.method"])
	}

	if span.Attributes["http.status_code"] != 200 {
		t.Errorf("Expected Attributes[http.status_code] = 200, got %v", span.Attributes["http.status_code"])
	}

	// Check duration is positive
	if span.Duration <= 0 {
		t.Errorf("Expected positive Duration, got %d", span.Duration)
	}
}

func TestSpanBuilderWithEvent(t *testing.T) {
	now := time.Now()

	span := NewSpanBuilder("trace123", "span456", "test-operation").
		WithEvent("exception", now, map[string]interface{}{
			"exception.type": "RuntimeError",
		}).
		Build()

	if len(span.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(span.Events))
	}

	if span.Events[0].Name != "exception" {
		t.Errorf("Expected event name 'exception', got %s", span.Events[0].Name)
	}

	if span.Events[0].Attributes["exception.type"] != "RuntimeError" {
		t.Errorf("Expected exception.type 'RuntimeError', got %v", span.Events[0].Attributes["exception.type"])
	}
}

func TestSpanBuilderWithLink(t *testing.T) {
	span := NewSpanBuilder("trace123", "span456", "test-operation").
		WithLink("linked-trace", "linked-span", map[string]interface{}{
			"link.reason": "follows_from",
		}).
		Build()

	if len(span.Links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(span.Links))
	}

	if span.Links[0].TraceID != "linked-trace" {
		t.Errorf("Expected link TraceID 'linked-trace', got %s", span.Links[0].TraceID)
	}

	if span.Links[0].SpanID != "linked-span" {
		t.Errorf("Expected link SpanID 'linked-span', got %s", span.Links[0].SpanID)
	}
}

func TestSpanBuilderWithDuration(t *testing.T) {
	span := NewSpanBuilder("trace123", "span456", "test-operation").
		WithDuration(100 * time.Millisecond).
		Build()

	expected := int64(100 * time.Millisecond)
	if span.Duration != expected {
		t.Errorf("Expected Duration %d, got %d", expected, span.Duration)
	}
}

func TestSpanOmitEmpty(t *testing.T) {
	span := NATSSpan{
		TraceID: "trace",
		SpanID:  "span",
		Name:    "op",
		Kind:    "internal",
		Status:  "unset",
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpan: %v", err)
	}

	dataStr := string(data)

	// Check that empty optional fields are omitted
	if containsJSON(dataStr, "parent_span_id") {
		t.Error("Expected parent_span_id to be omitted when empty")
	}
	if containsJSON(dataStr, "status_message") {
		t.Error("Expected status_message to be omitted when empty")
	}
	if containsJSON(dataStr, "attributes") {
		t.Error("Expected attributes to be omitted when nil")
	}
	if containsJSON(dataStr, "events") {
		t.Error("Expected events to be omitted when nil")
	}
	if containsJSON(dataStr, "links") {
		t.Error("Expected links to be omitted when nil")
	}
}

func containsJSON(s, field string) bool {
	// Simple check for field in JSON
	for i := 0; i <= len(s)-len(field)-3; i++ {
		if s[i:i+len(field)+3] == "\""+field+"\":" {
			return true
		}
	}
	return false
}

func TestSpanKinds(t *testing.T) {
	kinds := []string{"internal", "server", "client", "producer", "consumer"}

	for _, kind := range kinds {
		span := NewSpanBuilder("trace", "span", "op").
			WithKind(kind).
			Build()

		if span.Kind != kind {
			t.Errorf("Expected Kind %s, got %s", kind, span.Kind)
		}
	}
}

func TestSpanStatuses(t *testing.T) {
	statuses := []struct {
		status  string
		message string
	}{
		{"unset", ""},
		{"ok", ""},
		{"error", "Something went wrong"},
	}

	for _, s := range statuses {
		span := NewSpanBuilder("trace", "span", "op").
			WithStatus(s.status, s.message).
			Build()

		if span.Status != s.status {
			t.Errorf("Expected Status %s, got %s", s.status, span.Status)
		}

		if span.StatusMsg != s.message {
			t.Errorf("Expected StatusMsg %s, got %s", s.message, span.StatusMsg)
		}
	}
}

func TestNATSSpanWithEvents(t *testing.T) {
	span := NATSSpan{
		TraceID: "trace",
		SpanID:  "span",
		Name:    "op",
		Kind:    "server",
		Status:  "ok",
		Events: []NATSSpanEvent{
			{Name: "event1", Timestamp: "2024-01-15T10:30:00Z"},
			{Name: "event2", Timestamp: "2024-01-15T10:30:01Z"},
		},
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpan with events: %v", err)
	}

	var decoded NATSSpan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpan with events: %v", err)
	}

	if len(decoded.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(decoded.Events))
	}
}

func TestNATSSpanWithLinks(t *testing.T) {
	span := NATSSpan{
		TraceID: "trace",
		SpanID:  "span",
		Name:    "op",
		Kind:    "server",
		Status:  "ok",
		Links: []NATSSpanLink{
			{TraceID: "link1", SpanID: "span1"},
			{TraceID: "link2", SpanID: "span2"},
		},
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("Failed to marshal NATSSpan with links: %v", err)
	}

	var decoded NATSSpan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSSpan with links: %v", err)
	}

	if len(decoded.Links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(decoded.Links))
	}
}
