package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToCloudEvent(t *testing.T) {
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		CorrelationID("test-correlation").
		Tag("env", "production").
		Tag("region", "us-west-2").
		Data("key1", "value1").
		Data("key2", 123).
		Build()

	ce := ToCloudEvent(event)

	if ce.ID != event.ID {
		t.Errorf("Expected ID=%s, got %s", event.ID, ce.ID)
	}

	if ce.Source != event.Source {
		t.Errorf("Expected Source=%s, got %s", event.Source, ce.Source)
	}

	if ce.Type != string(event.Type) {
		t.Errorf("Expected Type=%s, got %s", event.Type, ce.Type)
	}

	if ce.SpecVersion != "1.0" {
		t.Errorf("Expected SpecVersion=1.0, got %s", ce.SpecVersion)
	}

	if ce.DataContentType != "application/json" {
		t.Errorf("Expected DataContentType=application/json, got %s", ce.DataContentType)
	}

	// Check extensions
	if ce.Extensions["severity"] != string(event.Severity) {
		t.Errorf("Expected severity extension=%s, got %v", event.Severity, ce.Extensions["severity"])
	}

	if ce.Extensions["correlationid"] != event.CorrelationID {
		t.Errorf("Expected correlationid extension=%s, got %v", event.CorrelationID, ce.Extensions["correlationid"])
	}

	if ce.Extensions["tag_env"] != "production" {
		t.Errorf("Expected tag_env=production, got %v", ce.Extensions["tag_env"])
	}

	if ce.Extensions["tag_region"] != "us-west-2" {
		t.Errorf("Expected tag_region=us-west-2, got %v", ce.Extensions["tag_region"])
	}

	// Check data
	dataMap, ok := ce.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be map[string]interface{}")
	}

	if dataMap["key1"] != "value1" {
		t.Errorf("Expected data.key1=value1, got %v", dataMap["key1"])
	}
}

func TestFromCloudEvent(t *testing.T) {
	now := time.Now()
	ce := &CloudEvent{
		ID:              "test-id",
		Source:          "/test-source",
		SpecVersion:     "1.0",
		Type:            "test.event",
		DataContentType: "application/json",
		Time:            &now,
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": float64(123),
		},
		Extensions: map[string]interface{}{
			"severity":      "warning",
			"correlationid": "test-correlation",
			"tag_env":       "production",
			"tag_region":    "us-west-2",
		},
	}

	event, err := FromCloudEvent(ce)
	if err != nil {
		t.Fatalf("FromCloudEvent failed: %v", err)
	}

	if event.ID != ce.ID {
		t.Errorf("Expected ID=%s, got %s", ce.ID, event.ID)
	}

	if event.Source != ce.Source {
		t.Errorf("Expected Source=%s, got %s", ce.Source, event.Source)
	}

	if string(event.Type) != ce.Type {
		t.Errorf("Expected Type=%s, got %s", ce.Type, event.Type)
	}

	if event.Severity != SeverityWarning {
		t.Errorf("Expected Severity=warning, got %s", event.Severity)
	}

	if event.CorrelationID != "test-correlation" {
		t.Errorf("Expected CorrelationID=test-correlation, got %s", event.CorrelationID)
	}

	if event.Tags["env"] != "production" {
		t.Errorf("Expected tag env=production, got %s", event.Tags["env"])
	}

	if event.Tags["region"] != "us-west-2" {
		t.Errorf("Expected tag region=us-west-2, got %s", event.Tags["region"])
	}

	if event.Data["key1"] != "value1" {
		t.Errorf("Expected data.key1=value1, got %v", event.Data["key1"])
	}
}

func TestFromCloudEvent_UnsupportedVersion(t *testing.T) {
	ce := &CloudEvent{
		ID:          "test-id",
		Source:      "/test",
		SpecVersion: "2.0",
		Type:        "test.event",
	}

	_, err := FromCloudEvent(ce)
	if err == nil {
		t.Error("Expected error for unsupported version")
	}
}

func TestCloudEvent_RoundTrip(t *testing.T) {
	original := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityError).
		CorrelationID("test-123").
		Tag("env", "test").
		Data("foo", "bar").
		Build()

	// Convert to CloudEvent
	ce := ToCloudEvent(original)

	// Convert back to Event
	converted, err := FromCloudEvent(ce)
	if err != nil {
		t.Fatalf("Round trip failed: %v", err)
	}

	// Verify key fields
	if converted.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, converted.ID)
	}

	if converted.Source != original.Source {
		t.Errorf("Source mismatch: expected %s, got %s", original.Source, converted.Source)
	}

	if converted.Type != original.Type {
		t.Errorf("Type mismatch: expected %s, got %s", original.Type, converted.Type)
	}

	if converted.Severity != original.Severity {
		t.Errorf("Severity mismatch: expected %s, got %s", original.Severity, converted.Severity)
	}

	if converted.CorrelationID != original.CorrelationID {
		t.Errorf("CorrelationID mismatch: expected %s, got %s", original.CorrelationID, converted.CorrelationID)
	}
}

func TestCloudEvent_JSON_Marshal(t *testing.T) {
	ce := &CloudEvent{
		ID:              "test-id",
		Source:          "/test",
		SpecVersion:     "1.0",
		Type:            "test.event",
		DataContentType: "application/json",
		Data: map[string]interface{}{
			"key": "value",
		},
		Extensions: map[string]interface{}{
			"custom_field": "custom_value",
			"severity":     "info",
		},
	}

	data, err := json.Marshal(ce)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Unmarshal to verify extensions are included
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if raw["custom_field"] != "custom_value" {
		t.Error("Expected custom_field extension in JSON")
	}

	if raw["severity"] != "info" {
		t.Error("Expected severity extension in JSON")
	}
}

func TestCloudEvent_JSON_Unmarshal(t *testing.T) {
	jsonData := `{
		"id": "test-id",
		"source": "/test",
		"specversion": "1.0",
		"type": "test.event",
		"datacontenttype": "application/json",
		"data": {"key": "value"},
		"custom_field": "custom_value",
		"severity": "info"
	}`

	var ce CloudEvent
	if err := json.Unmarshal([]byte(jsonData), &ce); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if ce.ID != "test-id" {
		t.Errorf("Expected ID=test-id, got %s", ce.ID)
	}

	if ce.Extensions["custom_field"] != "custom_value" {
		t.Error("Expected custom_field extension")
	}

	if ce.Extensions["severity"] != "info" {
		t.Error("Expected severity extension")
	}
}

func TestValidateCloudEvent(t *testing.T) {
	tests := []struct {
		name        string
		ce          *CloudEvent
		shouldError bool
	}{
		{
			name: "valid event",
			ce: &CloudEvent{
				ID:          "test-id",
				Source:      "/test",
				SpecVersion: "1.0",
				Type:        "test.event",
			},
			shouldError: false,
		},
		{
			name: "missing ID",
			ce: &CloudEvent{
				Source:      "/test",
				SpecVersion: "1.0",
				Type:        "test.event",
			},
			shouldError: true,
		},
		{
			name: "missing source",
			ce: &CloudEvent{
				ID:          "test-id",
				SpecVersion: "1.0",
				Type:        "test.event",
			},
			shouldError: true,
		},
		{
			name: "missing specversion",
			ce: &CloudEvent{
				ID:     "test-id",
				Source: "/test",
				Type:   "test.event",
			},
			shouldError: true,
		},
		{
			name: "missing type",
			ce: &CloudEvent{
				ID:          "test-id",
				Source:      "/test",
				SpecVersion: "1.0",
			},
			shouldError: true,
		},
		{
			name: "unsupported version",
			ce: &CloudEvent{
				ID:          "test-id",
				Source:      "/test",
				SpecVersion: "0.3",
				Type:        "test.event",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCloudEvent(tt.ce)
			if tt.shouldError && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestCloudEventBatch(t *testing.T) {
	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	batch := ToCloudEventBatch(events)

	if len(batch.Events) != 3 {
		t.Errorf("Expected 3 events in batch, got %d", len(batch.Events))
	}

	// Convert back
	converted, err := FromCloudEventBatch(batch)
	if err != nil {
		t.Fatalf("Batch conversion failed: %v", err)
	}

	if len(converted) != 3 {
		t.Errorf("Expected 3 converted events, got %d", len(converted))
	}

	for i := range events {
		if converted[i].Type != events[i].Type {
			t.Errorf("Event %d type mismatch: expected %s, got %s", i, events[i].Type, converted[i].Type)
		}
	}
}

func TestCloudEventBatch_JSON(t *testing.T) {
	batch := &CloudEventBatch{
		Events: []*CloudEvent{
			{
				ID:          "1",
				Source:      "/test1",
				SpecVersion: "1.0",
				Type:        "test.event1",
			},
			{
				ID:          "2",
				Source:      "/test2",
				SpecVersion: "1.0",
				Type:        "test.event2",
			},
		},
	}

	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Batch marshal failed: %v", err)
	}

	var decoded CloudEventBatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Batch unmarshal failed: %v", err)
	}

	if len(decoded.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(decoded.Events))
	}
}

func TestHTTPCloudEventHandler(t *testing.T) {
	handled := false
	var handledEvent *Event

	handler := NewHTTPCloudEventHandler(func(event *Event) error {
		handled = true
		handledEvent = event
		return nil
	})

	ce := &CloudEvent{
		ID:          "test-id",
		Source:      "/test",
		SpecVersion: "1.0",
		Type:        "test.event",
		Data: map[string]interface{}{
			"key": "value",
		},
	}

	err := handler.HandleCloudEvent(ce)
	if err != nil {
		t.Fatalf("HandleCloudEvent failed: %v", err)
	}

	if !handled {
		t.Error("Expected handler to be called")
	}

	if handledEvent.ID != ce.ID {
		t.Errorf("Expected event ID=%s, got %s", ce.ID, handledEvent.ID)
	}
}

func TestCloudEventPublisher(t *testing.T) {
	var published *Event
	mockPublisher := &MockPublisher{
		PublishFunc: func(event *Event) error {
			published = event
			return nil
		},
	}

	cePublisher := NewCloudEventPublisher(mockPublisher)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := cePublisher.Publish(event)
	if err != nil {
		t.Fatalf("CloudEventPublisher.Publish failed: %v", err)
	}

	if published == nil {
		t.Fatal("Expected event to be published")
	}
}

func TestCloudEventSubscriber(t *testing.T) {
	mockSubscriber := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			// Test the wrapped handler
			event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
			handler(event)
			return &Subscription{}, nil
		},
	}

	ceSubscriber := NewCloudEventSubscriber(mockSubscriber)

	handled := false
	_, err := ceSubscriber.Subscribe("test.>", func(event *Event) error {
		handled = true
		return nil
	})

	if err != nil {
		t.Fatalf("CloudEventSubscriber.Subscribe failed: %v", err)
	}

	if !handled {
		t.Error("Expected handler to be called")
	}
}

// Benchmark CloudEvent conversion
func BenchmarkToCloudEvent(b *testing.B) {
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "test").
		Data("key", "value").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ToCloudEvent(event)
	}
}

func BenchmarkFromCloudEvent(b *testing.B) {
	now := time.Now()
	ce := &CloudEvent{
		ID:          "test-id",
		Source:      "/test",
		SpecVersion: "1.0",
		Type:        "test.event",
		Time:        &now,
		Data: map[string]interface{}{
			"key": "value",
		},
		Extensions: map[string]interface{}{
			"severity": "info",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromCloudEvent(ce)
	}
}

func BenchmarkCloudEvent_JSON_Marshal(b *testing.B) {
	ce := &CloudEvent{
		ID:          "test-id",
		Source:      "/test",
		SpecVersion: "1.0",
		Type:        "test.event",
		Data: map[string]interface{}{
			"key": "value",
		},
		Extensions: map[string]interface{}{
			"custom": "value",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(ce)
	}
}

// Mock subscriber for testing
type MockSubscriber struct {
	SubscribeFunc      func(subject string, handler EventHandler) (*Subscription, error)
	SubscribeQueueFunc func(subject string, queue string, handler EventHandler) (*Subscription, error)
	CloseFunc          func() error
}

func (m *MockSubscriber) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(subject, handler)
	}
	return &Subscription{}, nil
}

func (m *MockSubscriber) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	if m.SubscribeQueueFunc != nil {
		return m.SubscribeQueueFunc(subject, queue, handler)
	}
	return &Subscription{}, nil
}

func (m *MockSubscriber) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
