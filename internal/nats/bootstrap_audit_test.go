package nats

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// mockEventPublisher implements events.EventPublisher for testing
type mockEventPublisher struct {
	publishedEvents []*events.Event
	publishErr      error
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	return m.Publish(event)
}

func (m *mockEventPublisher) Close() error {
	return nil
}

func TestNewEventBasedBootstrapAuditLogger(t *testing.T) {
	publisher := &mockEventPublisher{}

	tests := []struct {
		name    string
		source  string
		wantSrc string
	}{
		{
			name:    "with custom source",
			source:  "my-handler",
			wantSrc: "my-handler",
		},
		{
			name:    "with empty source uses default",
			source:  "",
			wantSrc: "bootstrap-handler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewEventBasedBootstrapAuditLogger(publisher, tt.source)
			if logger == nil {
				t.Fatal("NewEventBasedBootstrapAuditLogger returned nil")
			}
			if logger.source != tt.wantSrc {
				t.Errorf("source = %s, want %s", logger.source, tt.wantSrc)
			}
		})
	}
}

func TestEventBasedBootstrapAuditLogger_Log(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		event    BootstrapAuditEvent
		wantType events.EventType
		wantTags []string
	}{
		{
			name: "successful registration",
			event: BootstrapAuditEvent{
				Timestamp:    time.Now(),
				EventType:    BootstrapAuditEventRegister,
				CredentialID: "cred-123",
				AgentID:      "agent-123",
				Cluster:      "test-cluster",
				Success:      true,
			},
			wantType: EventTypeBootstrapRegister,
			wantTags: []string{"bootstrap", "test-cluster", "success"},
		},
		{
			name: "failed validation",
			event: BootstrapAuditEvent{
				Timestamp:    time.Now(),
				EventType:    BootstrapAuditEventValidate,
				CredentialID: "cred-123",
				Cluster:      "test-cluster",
				Success:      false,
				Error:        "credential expired",
			},
			wantType: EventTypeBootstrapValidate,
			wantTags: []string{"bootstrap", "test-cluster", "failure"},
		},
		{
			name: "credential generation",
			event: BootstrapAuditEvent{
				Timestamp:    time.Now(),
				EventType:    BootstrapAuditEventGenerate,
				CredentialID: "cred-123",
				Cluster:      "test-cluster",
				Success:      true,
			},
			wantType: EventTypeBootstrapGenerate,
			wantTags: []string{"bootstrap", "test-cluster", "success"},
		},
		{
			name: "credential revocation",
			event: BootstrapAuditEvent{
				Timestamp:    time.Now(),
				EventType:    BootstrapAuditEventRevoke,
				CredentialID: "cred-123",
				Cluster:      "test-cluster",
				Success:      true,
				Details: map[string]interface{}{
					"reason": "security concern",
				},
			},
			wantType: EventTypeBootstrapRevoke,
			wantTags: []string{"bootstrap", "test-cluster", "success"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &mockEventPublisher{}
			logger := NewEventBasedBootstrapAuditLogger(publisher, "test-source")

			err := logger.Log(ctx, tt.event)
			if err != nil {
				t.Fatalf("Log() error = %v", err)
			}

			if len(publisher.publishedEvents) != 1 {
				t.Fatalf("expected 1 published event, got %d", len(publisher.publishedEvents))
			}

			publishedEvent := publisher.publishedEvents[0]
			if publishedEvent.Type != tt.wantType {
				t.Errorf("event type = %s, want %s", publishedEvent.Type, tt.wantType)
			}

			// Check tags
			for _, wantTag := range tt.wantTags {
				found := false
				for _, tag := range publishedEvent.Tags {
					if tag == wantTag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing expected tag: %s", wantTag)
				}
			}
		})
	}
}

func TestEventBasedBootstrapAuditLogger_NilPublisher(t *testing.T) {
	ctx := context.Background()
	logger := NewEventBasedBootstrapAuditLogger(nil, "test")

	err := logger.Log(ctx, BootstrapAuditEvent{
		EventType: BootstrapAuditEventRegister,
		Success:   true,
	})

	// Should not error with nil publisher
	if err != nil {
		t.Errorf("Log() with nil publisher should not error, got %v", err)
	}
}

func TestMapBootstrapEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected events.EventType
	}{
		{BootstrapAuditEventGenerate, EventTypeBootstrapGenerate},
		{BootstrapAuditEventValidate, EventTypeBootstrapValidate},
		{BootstrapAuditEventRevoke, EventTypeBootstrapRevoke},
		{BootstrapAuditEventUse, EventTypeBootstrapUse},
		{BootstrapAuditEventRegister, EventTypeBootstrapRegister},
		{BootstrapAuditEventExpire, EventTypeBootstrapExpire},
		{BootstrapAuditEventCleanup, EventTypeBootstrapCleanup},
		{"custom", events.EventType("bootstrap.custom")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapBootstrapEventType(tt.input)
			if result != tt.expected {
				t.Errorf("mapBootstrapEventType(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetermineSeverity(t *testing.T) {
	tests := []struct {
		name     string
		event    BootstrapAuditEvent
		expected events.Severity
	}{
		{
			name: "failed registration is warning",
			event: BootstrapAuditEvent{
				EventType: BootstrapAuditEventRegister,
				Success:   false,
			},
			expected: events.SeverityWarning,
		},
		{
			name: "successful registration is info",
			event: BootstrapAuditEvent{
				EventType: BootstrapAuditEventRegister,
				Success:   true,
			},
			expected: events.SeverityInfo,
		},
		{
			name: "cleanup is debug",
			event: BootstrapAuditEvent{
				EventType: BootstrapAuditEventCleanup,
				Success:   true,
			},
			expected: events.SeverityDebug,
		},
		{
			name: "revocation is info",
			event: BootstrapAuditEvent{
				EventType: BootstrapAuditEventRevoke,
				Success:   true,
			},
			expected: events.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineSeverity(tt.event)
			if result != tt.expected {
				t.Errorf("determineSeverity() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestBuildEventData(t *testing.T) {
	event := BootstrapAuditEvent{
		Timestamp:    time.Now(),
		EventType:    BootstrapAuditEventRegister,
		CredentialID: "cred-123",
		AgentID:      "agent-123",
		Cluster:      "test-cluster",
		Success:      true,
		SourceIP:     "192.168.1.1",
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	data := buildEventData(event)

	// Check required fields
	if data["event_type"] != BootstrapAuditEventRegister {
		t.Errorf("event_type = %v, want %s", data["event_type"], BootstrapAuditEventRegister)
	}
	if data["cluster"] != "test-cluster" {
		t.Errorf("cluster = %v, want test-cluster", data["cluster"])
	}
	if data["success"] != true {
		t.Errorf("success = %v, want true", data["success"])
	}
	if data["credential_id"] != "cred-123" {
		t.Errorf("credential_id = %v, want cred-123", data["credential_id"])
	}
	if data["agent_id"] != "agent-123" {
		t.Errorf("agent_id = %v, want agent-123", data["agent_id"])
	}
	if data["source_ip"] != "192.168.1.1" {
		t.Errorf("source_ip = %v, want 192.168.1.1", data["source_ip"])
	}
	if data["details"] == nil {
		t.Error("details should not be nil")
	}
}

func TestNewInMemoryBootstrapAuditLogger(t *testing.T) {
	tests := []struct {
		name      string
		maxEvents int
		wantMax   int
	}{
		{
			name:      "custom max",
			maxEvents: 100,
			wantMax:   100,
		},
		{
			name:      "zero max uses default",
			maxEvents: 0,
			wantMax:   1000,
		},
		{
			name:      "negative max uses default",
			maxEvents: -1,
			wantMax:   1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewInMemoryBootstrapAuditLogger(tt.maxEvents)
			if logger == nil {
				t.Fatal("NewInMemoryBootstrapAuditLogger returned nil")
			}
			if logger.maxEvents != tt.wantMax {
				t.Errorf("maxEvents = %d, want %d", logger.maxEvents, tt.wantMax)
			}
		})
	}
}

func TestInMemoryBootstrapAuditLogger_Log(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(10)

	event := BootstrapAuditEvent{
		Timestamp:    time.Now(),
		EventType:    BootstrapAuditEventRegister,
		CredentialID: "cred-123",
		Success:      true,
	}

	err := logger.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	if logger.Count() != 1 {
		t.Errorf("Count() = %d, want 1", logger.Count())
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Errorf("GetEvents() returned %d events, want 1", len(events))
	}
}

func TestInMemoryBootstrapAuditLogger_MaxEvents(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(3)

	// Log 5 events
	for i := 0; i < 5; i++ {
		_ = logger.Log(ctx, BootstrapAuditEvent{
			Timestamp:    time.Now(),
			EventType:    BootstrapAuditEventRegister,
			CredentialID: "cred-" + string(rune('0'+i)),
			Success:      true,
		})
	}

	// Should only have 3 events
	if logger.Count() != 3 {
		t.Errorf("Count() = %d, want 3", logger.Count())
	}
}

func TestInMemoryBootstrapAuditLogger_GetEventsByType(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(100)

	_ = logger.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRegister})
	_ = logger.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventValidate})
	_ = logger.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRegister})

	registerEvents := logger.GetEventsByType(BootstrapAuditEventRegister)
	if len(registerEvents) != 2 {
		t.Errorf("GetEventsByType() returned %d events, want 2", len(registerEvents))
	}
}

func TestInMemoryBootstrapAuditLogger_GetEventsByCredentialID(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(100)

	_ = logger.Log(ctx, BootstrapAuditEvent{CredentialID: "cred-1"})
	_ = logger.Log(ctx, BootstrapAuditEvent{CredentialID: "cred-2"})
	_ = logger.Log(ctx, BootstrapAuditEvent{CredentialID: "cred-1"})

	events := logger.GetEventsByCredentialID("cred-1")
	if len(events) != 2 {
		t.Errorf("GetEventsByCredentialID() returned %d events, want 2", len(events))
	}
}

func TestInMemoryBootstrapAuditLogger_GetEventsByAgentID(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(100)

	_ = logger.Log(ctx, BootstrapAuditEvent{AgentID: "agent-1"})
	_ = logger.Log(ctx, BootstrapAuditEvent{AgentID: "agent-2"})
	_ = logger.Log(ctx, BootstrapAuditEvent{AgentID: "agent-1"})

	events := logger.GetEventsByAgentID("agent-1")
	if len(events) != 2 {
		t.Errorf("GetEventsByAgentID() returned %d events, want 2", len(events))
	}
}

func TestInMemoryBootstrapAuditLogger_GetFailedEvents(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(100)

	_ = logger.Log(ctx, BootstrapAuditEvent{Success: true})
	_ = logger.Log(ctx, BootstrapAuditEvent{Success: false})
	_ = logger.Log(ctx, BootstrapAuditEvent{Success: false})

	events := logger.GetFailedEvents()
	if len(events) != 2 {
		t.Errorf("GetFailedEvents() returned %d events, want 2", len(events))
	}
}

func TestInMemoryBootstrapAuditLogger_Clear(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryBootstrapAuditLogger(100)

	_ = logger.Log(ctx, BootstrapAuditEvent{})
	_ = logger.Log(ctx, BootstrapAuditEvent{})

	logger.Clear()

	if logger.Count() != 0 {
		t.Errorf("Count() after Clear() = %d, want 0", logger.Count())
	}
}

func TestCompositeBootstrapAuditLogger(t *testing.T) {
	ctx := context.Background()

	logger1 := NewInMemoryBootstrapAuditLogger(100)
	logger2 := NewInMemoryBootstrapAuditLogger(100)

	composite := NewCompositeBootstrapAuditLogger(logger1, logger2)

	event := BootstrapAuditEvent{
		Timestamp: time.Now(),
		EventType: BootstrapAuditEventRegister,
		Success:   true,
	}

	err := composite.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Both loggers should have the event
	if logger1.Count() != 1 {
		t.Errorf("logger1.Count() = %d, want 1", logger1.Count())
	}
	if logger2.Count() != 1 {
		t.Errorf("logger2.Count() = %d, want 1", logger2.Count())
	}
}

func TestCompositeBootstrapAuditLogger_AddLogger(t *testing.T) {
	ctx := context.Background()

	logger1 := NewInMemoryBootstrapAuditLogger(100)
	composite := NewCompositeBootstrapAuditLogger(logger1)

	logger2 := NewInMemoryBootstrapAuditLogger(100)
	composite.AddLogger(logger2)

	_ = composite.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRegister})

	if logger1.Count() != 1 || logger2.Count() != 1 {
		t.Error("both loggers should have 1 event")
	}
}

func TestFilteredBootstrapAuditLogger(t *testing.T) {
	ctx := context.Background()
	inner := NewInMemoryBootstrapAuditLogger(100)

	// Create a filtered logger that only logs failures
	filtered := NewFilteredBootstrapAuditLogger(inner, FailuresOnlyFilter)

	_ = filtered.Log(ctx, BootstrapAuditEvent{Success: true})
	_ = filtered.Log(ctx, BootstrapAuditEvent{Success: false})

	// Only the failed event should be logged
	if inner.Count() != 1 {
		t.Errorf("Count() = %d, want 1", inner.Count())
	}
}

func TestEventTypesFilter(t *testing.T) {
	ctx := context.Background()
	inner := NewInMemoryBootstrapAuditLogger(100)

	// Create a filter for only register and validate events
	filter := EventTypesFilter(BootstrapAuditEventRegister, BootstrapAuditEventValidate)
	filtered := NewFilteredBootstrapAuditLogger(inner, filter)

	_ = filtered.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRegister})
	_ = filtered.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventValidate})
	_ = filtered.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRevoke})

	// Only register and validate events should be logged
	if inner.Count() != 2 {
		t.Errorf("Count() = %d, want 2", inner.Count())
	}
}

func TestFilteredBootstrapAuditLogger_NilFilter(t *testing.T) {
	ctx := context.Background()
	inner := NewInMemoryBootstrapAuditLogger(100)

	// Create a filtered logger with nil filter (should pass all events)
	filtered := NewFilteredBootstrapAuditLogger(inner, nil)

	_ = filtered.Log(ctx, BootstrapAuditEvent{EventType: BootstrapAuditEventRegister})

	if inner.Count() != 1 {
		t.Errorf("Count() = %d, want 1", inner.Count())
	}
}
