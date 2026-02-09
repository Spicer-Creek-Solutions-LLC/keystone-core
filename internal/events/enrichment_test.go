package events

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestEnrichmentPipeline_Basic(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	// Add tag enricher
	pipeline.AddEnricher(NewTagEnricher("tags", map[string]string{
		"env": "test",
	}))

	// Add data enricher
	pipeline.AddEnricher(NewDataEnricher("data", map[string]interface{}{
		"version": "1.0",
	}))

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := pipeline.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	// Verify tags
	if event.Tags["env"] != "test" {
		t.Errorf("Expected tag env=test, got %s", event.Tags["env"])
	}

	// Verify data
	if event.Data["version"] != "1.0" {
		t.Errorf("Expected data version=1.0, got %v", event.Data["version"])
	}
}

func TestTagEnricher(t *testing.T) {
	enricher := NewTagEnricher("test-tags", map[string]string{
		"env":    "production",
		"region": "us-west-2",
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if event.Tags["env"] != "production" {
		t.Errorf("Expected tag env=production, got %s", event.Tags["env"])
	}
	if event.Tags["region"] != "us-west-2" {
		t.Errorf("Expected tag region=us-west-2, got %s", event.Tags["region"])
	}
}

func TestDataEnricher(t *testing.T) {
	enricher := NewDataEnricher("test-data", map[string]interface{}{
		"version":  "1.0.0",
		"build_id": 12345,
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if event.Data["version"] != "1.0.0" {
		t.Errorf("Expected data version=1.0.0, got %v", event.Data["version"])
	}
	if event.Data["build_id"] != 12345 {
		t.Errorf("Expected data build_id=12345, got %v", event.Data["build_id"])
	}
}

func TestFunctionEnricher(t *testing.T) {
	called := false
	enricher := NewFunctionEnricher("test-fn", func(ctx context.Context, event *Event) error {
		called = true
		event.Data["custom"] = "value"
		return nil
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if !called {
		t.Error("Function enricher was not called")
	}
	if event.Data["custom"] != "value" {
		t.Errorf("Expected data custom=value, got %v", event.Data["custom"])
	}
}

func TestFunctionEnricher_Error(t *testing.T) {
	expectedErr := fmt.Errorf("enrichment error")
	enricher := NewFunctionEnricher("test-fn", func(ctx context.Context, event *Event) error {
		return expectedErr
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestConditionalEnricher(t *testing.T) {
	filter, _ := ParseFilterExpression(`type == "agent.connect"`)
	enricher := NewConditionalEnricher(
		"conditional",
		filter,
		NewDataEnricher("data", map[string]interface{}{
			"matched": true,
		}),
	)

	// Matching event
	event1 := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	enricher.Enrich(context.Background(), event1)

	if event1.Data["matched"] != true {
		t.Error("Expected data to be enriched for matching event")
	}

	// Non-matching event
	event2 := NewEvent(EventTypeJobStart).Source("/test").Build()
	enricher.Enrich(context.Background(), event2)

	if _, exists := event2.Data["matched"]; exists {
		t.Error("Expected data not to be enriched for non-matching event")
	}
}

func TestTimestampEnricher(t *testing.T) {
	enricher := NewTimestampEnricher("timestamps", "enriched_at", "processed_at")

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if _, exists := event.Data["enriched_at"]; !exists {
		t.Error("Expected enriched_at field")
	}
	if _, exists := event.Data["processed_at"]; !exists {
		t.Error("Expected processed_at field")
	}
}

func TestHostnameEnricher(t *testing.T) {
	enricher := NewHostnameEnricher("hostname", "test-host-01")

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if event.Data["enriched_by_host"] != "test-host-01" {
		t.Errorf("Expected hostname test-host-01, got %v", event.Data["enriched_by_host"])
	}

	// Test custom field name
	enricher2 := NewHostnameEnricher("hostname", "test-host-02").SetField("host")
	event2 := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	enricher2.Enrich(context.Background(), event2)

	if event2.Data["host"] != "test-host-02" {
		t.Errorf("Expected host test-host-02, got %v", event2.Data["host"])
	}
}

func TestSequenceNumberEnricher(t *testing.T) {
	enricher := NewSequenceNumberEnricher("sequence")

	// Enrich multiple events
	for i := 1; i <= 5; i++ {
		event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
		enricher.Enrich(context.Background(), event)

		seq, ok := event.Data["sequence"].(uint64)
		if !ok {
			t.Fatal("Expected sequence to be uint64")
		}
		if seq != uint64(i) {
			t.Errorf("Expected sequence=%d, got %d", i, seq)
		}
	}
}

func TestSequenceNumberEnricher_CustomField(t *testing.T) {
	enricher := NewSequenceNumberEnricher("sequence").SetField("event_number")

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	enricher.Enrich(context.Background(), event)

	if _, exists := event.Data["event_number"]; !exists {
		t.Error("Expected event_number field")
	}
}

func TestChainEnrichers(t *testing.T) {
	chained := ChainEnrichers(
		"chained",
		NewTagEnricher("tags", map[string]string{"env": "test"}),
		NewDataEnricher("data", map[string]interface{}{"version": "1.0"}),
		NewSequenceNumberEnricher("seq"),
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := chained.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if event.Tags["env"] != "test" {
		t.Error("Expected tag enrichment")
	}
	if event.Data["version"] != "1.0" {
		t.Error("Expected data enrichment")
	}
	if _, exists := event.Data["sequence"]; !exists {
		t.Error("Expected sequence enrichment")
	}
}

func TestEnrichmentPipeline_Order(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	order := []string{}

	pipeline.AddEnricher(NewFunctionEnricher("first", func(ctx context.Context, event *Event) error {
		order = append(order, "first")
		return nil
	}))

	pipeline.AddEnricher(NewFunctionEnricher("second", func(ctx context.Context, event *Event) error {
		order = append(order, "second")
		return nil
	}))

	pipeline.AddEnricher(NewFunctionEnricher("third", func(ctx context.Context, event *Event) error {
		order = append(order, "third")
		return nil
	}))

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	pipeline.Enrich(context.Background(), event)

	expected := []string{"first", "second", "third"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d enrichers, got %d", len(expected), len(order))
	}

	for i, name := range expected {
		if order[i] != name {
			t.Errorf("Expected order[%d]=%s, got %s", i, name, order[i])
		}
	}
}

func TestEnrichmentPipeline_RemoveEnricher(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	pipeline.AddEnricher(NewTagEnricher("tags", map[string]string{"env": "test"}))
	pipeline.AddEnricher(NewDataEnricher("data", map[string]interface{}{"version": "1.0"}))

	// Remove tags enricher
	removed := pipeline.RemoveEnricher("tags")
	if !removed {
		t.Error("Expected enricher to be removed")
	}

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	pipeline.Enrich(context.Background(), event)

	// Tags should not be enriched
	if _, exists := event.Tags["env"]; exists {
		t.Error("Expected tags not to be enriched after removal")
	}

	// Data should still be enriched
	if event.Data["version"] != "1.0" {
		t.Error("Expected data to still be enriched")
	}

	// Try to remove non-existent enricher
	removed = pipeline.RemoveEnricher("nonexistent")
	if removed {
		t.Error("Expected false when removing non-existent enricher")
	}
}

func TestEnrichmentPipeline_ErrorHandler(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	var erroredEnricher string
	var erroredEvent *Event
	var errorReceived error

	pipeline.SetErrorHandler(func(enricherName string, event *Event, err error) {
		erroredEnricher = enricherName
		erroredEvent = event
		errorReceived = err
	})

	expectedErr := fmt.Errorf("enrichment error")
	pipeline.AddEnricher(NewFunctionEnricher("failing", func(ctx context.Context, event *Event) error {
		return expectedErr
	}))

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	pipeline.Enrich(context.Background(), event)

	if erroredEnricher != "failing" {
		t.Errorf("Expected error from 'failing' enricher, got %s", erroredEnricher)
	}
	if erroredEvent != event {
		t.Error("Expected errored event to match")
	}
	if !errors.Is(errorReceived, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, errorReceived)
	}
}

func TestEnrichmentPipeline_ContinueOnError(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	// First enricher fails
	pipeline.AddEnricher(NewFunctionEnricher("failing", func(ctx context.Context, event *Event) error {
		return fmt.Errorf("error")
	}))

	// Second enricher should still run
	called := false
	pipeline.AddEnricher(NewFunctionEnricher("succeeding", func(ctx context.Context, event *Event) error {
		called = true
		return nil
	}))

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	pipeline.Enrich(context.Background(), event)

	if !called {
		t.Error("Expected second enricher to be called despite first failing")
	}
}

func TestContextEnricher(t *testing.T) {
	type contextKey string
	const userKey contextKey = "user_id"
	const reqKey contextKey = "request_id"

	// Create enricher with key->fieldName pairs
	enricher := NewContextEnricher("context", userKey, "user_id", reqKey, "request_id")

	ctx := context.Background()
	ctx = context.WithValue(ctx, userKey, "user-123")
	ctx = context.WithValue(ctx, reqKey, "req-456")

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(ctx, event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	if event.Data["user_id"] != "user-123" {
		t.Errorf("Expected user_id=user-123, got %v", event.Data["user_id"])
	}
	if event.Data["request_id"] != "req-456" {
		t.Errorf("Expected request_id=req-456, got %v", event.Data["request_id"])
	}
}

func TestContextEnricher_MissingKeys(t *testing.T) {
	type contextKey string
	const missingKey contextKey = "missing"

	enricher := NewContextEnricher("context", missingKey, "missing_key")

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := enricher.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("Enrichment failed: %v", err)
	}

	// Missing keys should not be added
	if _, exists := event.Data["missing_key"]; exists {
		t.Error("Expected missing key not to be added")
	}
}

func TestEnrichedPublisher(t *testing.T) {
	// Create mock publisher
	var published *Event
	mockPublisher := &MockPublisher{
		PublishFunc: func(event *Event) error {
			published = event
			return nil
		},
	}

	// Create pipeline
	pipeline := NewEnrichmentPipeline(
		NewTagEnricher("tags", map[string]string{"env": "test"}),
		NewDataEnricher("data", map[string]interface{}{"version": "1.0"}),
	)

	// Create enriched publisher
	enrichedPub := NewEnrichedPublisher(mockPublisher, pipeline)

	// Publish event
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := enrichedPub.Publish(event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify enrichment happened
	if published.Tags["env"] != "test" {
		t.Error("Expected event to be enriched before publishing")
	}
	if published.Data["version"] != "1.0" {
		t.Error("Expected event data to be enriched before publishing")
	}
}

func TestEnrichedPublisher_Async(t *testing.T) {
	// Create mock publisher
	var published *Event
	mockPublisher := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			published = event
			return nil
		},
	}

	// Create pipeline
	pipeline := NewEnrichmentPipeline(
		NewTagEnricher("tags", map[string]string{"env": "test"}),
	)

	// Create enriched publisher
	enrichedPub := NewEnrichedPublisher(mockPublisher, pipeline)

	// Publish async
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := enrichedPub.PublishAsync(event)
	if err != nil {
		t.Fatalf("PublishAsync failed: %v", err)
	}

	// Verify enrichment
	if published.Tags["env"] != "test" {
		t.Error("Expected event to be enriched before async publishing")
	}
}

func TestEnrichmentPipeline_GetEnrichers(t *testing.T) {
	pipeline := NewEnrichmentPipeline()

	pipeline.AddEnricher(NewTagEnricher("tags", nil))
	pipeline.AddEnricher(NewDataEnricher("data", nil))

	enrichers := pipeline.GetEnrichers()

	if len(enrichers) != 2 {
		t.Errorf("Expected 2 enrichers, got %d", len(enrichers))
	}

	if enrichers[0].Name() != "tags" {
		t.Errorf("Expected first enricher name=tags, got %s", enrichers[0].Name())
	}
	if enrichers[1].Name() != "data" {
		t.Errorf("Expected second enricher name=data, got %s", enrichers[1].Name())
	}
}

// Mock publisher for testing
type MockPublisher struct {
	PublishFunc      func(event *Event) error
	PublishAsyncFunc func(event *Event) error
	CloseFunc        func() error
}

func (m *MockPublisher) Publish(event *Event) error {
	if m.PublishFunc != nil {
		return m.PublishFunc(event)
	}
	return nil
}

func (m *MockPublisher) PublishAsync(event *Event) error {
	if m.PublishAsyncFunc != nil {
		return m.PublishAsyncFunc(event)
	}
	return nil
}

func (m *MockPublisher) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Benchmark enrichment
func BenchmarkEnrichmentPipeline(b *testing.B) {
	pipeline := NewEnrichmentPipeline(
		NewTagEnricher("tags", map[string]string{"env": "test"}),
		NewDataEnricher("data", map[string]interface{}{"version": "1.0"}),
		NewSequenceNumberEnricher("seq"),
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipeline.Enrich(context.Background(), event)
	}
}

func BenchmarkSequenceNumberEnricher(b *testing.B) {
	enricher := NewSequenceNumberEnricher("seq")
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enricher.Enrich(context.Background(), event)
	}
}
