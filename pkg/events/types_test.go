package events

import (
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/web-01").
		Severity(SeverityInfo).
		Tag("datacenter", "us-east-1").
		Tag("role", "web").
		Data("ip", "10.0.1.5").
		Data("hostname", "web-01").
		Build()

	if event.Type != EventTypeAgentConnect {
		t.Errorf("Expected type %s, got %s", EventTypeAgentConnect, event.Type)
	}

	if event.Source != "/agents/web-01" {
		t.Errorf("Expected source /agents/web-01, got %s", event.Source)
	}

	if event.Severity != SeverityInfo {
		t.Errorf("Expected severity %s, got %s", SeverityInfo, event.Severity)
	}

	if event.Tags["datacenter"] != "us-east-1" {
		t.Error("Expected datacenter tag to be us-east-1")
	}

	if event.Data["ip"] != "10.0.1.5" {
		t.Error("Expected ip data field to be 10.0.1.5")
	}

	if event.ID == "" {
		t.Error("Expected event ID to be generated")
	}

	if event.Time.IsZero() {
		t.Error("Expected event time to be set")
	}
}

func TestEventFilter_MatchesType(t *testing.T) {
	event := NewEvent(EventTypeStateChange).Build()

	filter := &EventFilter{
		Types: []EventType{EventTypeStateChange, EventTypeStateDrift},
	}

	if !filter.Matches(event) {
		t.Error("Expected filter to match event type")
	}

	filter2 := &EventFilter{
		Types: []EventType{EventTypeAgentConnect},
	}

	if filter2.Matches(event) {
		t.Error("Expected filter to not match different event type")
	}
}

func TestEventFilter_MatchesSource(t *testing.T) {
	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/web-01").
		Build()

	filter := &EventFilter{
		Sources: []string{"/agents/web-01", "/agents/web-02"},
	}

	if !filter.Matches(event) {
		t.Error("Expected filter to match event source")
	}

	filter2 := &EventFilter{
		Sources: []string{"/agents/db-01"},
	}

	if filter2.Matches(event) {
		t.Error("Expected filter to not match different source")
	}
}

func TestEventFilter_MatchesTags(t *testing.T) {
	event := NewEvent(EventTypeStateChange).
		Tag("environment", "production").
		Tag("datacenter", "us-east-1").
		Build()

	filter := &EventFilter{
		Tags: map[string]string{
			"environment": "production",
		},
	}

	if !filter.Matches(event) {
		t.Error("Expected filter to match event tags")
	}

	filter2 := &EventFilter{
		Tags: map[string]string{
			"environment": "production",
			"datacenter":  "us-east-1",
		},
	}

	if !filter2.Matches(event) {
		t.Error("Expected filter to match multiple tags")
	}

	filter3 := &EventFilter{
		Tags: map[string]string{
			"environment": "staging",
		},
	}

	if filter3.Matches(event) {
		t.Error("Expected filter to not match different tag value")
	}
}

func TestEventFilter_MatchesSeverity(t *testing.T) {
	infoEvent := NewEvent(EventTypeAgentConnect).
		Severity(SeverityInfo).
		Build()

	errorEvent := NewEvent(EventTypeAgentError).
		Severity(SeverityError).
		Build()

	// Filter for warning or higher
	filter := &EventFilter{
		Severity: SeverityWarning,
	}

	if filter.Matches(infoEvent) {
		t.Error("Expected filter to not match info event (below warning)")
	}

	if !filter.Matches(errorEvent) {
		t.Error("Expected filter to match error event (above warning)")
	}
}

func TestEventFilter_MatchesTimeRange(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	event := NewEvent(EventTypeStateChange).Build()
	event.Time = now

	// Filter for events after 1 hour ago
	filter := &EventFilter{
		Since: &past,
	}

	if !filter.Matches(event) {
		t.Error("Expected filter to match event after since time")
	}

	// Filter for events before 1 hour from now
	filter2 := &EventFilter{
		Until: &future,
	}

	if !filter2.Matches(event) {
		t.Error("Expected filter to match event before until time")
	}

	// Filter for events in the future
	filter3 := &EventFilter{
		Since: &future,
	}

	if filter3.Matches(event) {
		t.Error("Expected filter to not match event before since time")
	}
}

func TestEventFilter_MatchesMultipleCriteria(t *testing.T) {
	event := NewEvent(EventTypeStateChange).
		Source("/agents/web-01").
		Severity(SeverityWarning).
		Tag("environment", "production").
		Build()

	filter := &EventFilter{
		Types:    []EventType{EventTypeStateChange, EventTypeStateDrift},
		Sources:  []string{"/agents/web-01"},
		Severity: SeverityInfo,
		Tags: map[string]string{
			"environment": "production",
		},
	}

	if !filter.Matches(event) {
		t.Error("Expected filter to match event with all criteria")
	}

	// Change one criteria
	filter2 := &EventFilter{
		Types:   []EventType{EventTypeStateChange},
		Sources: []string{"/agents/web-01"},
		Tags: map[string]string{
			"environment": "staging", // Different environment
		},
	}

	if filter2.Matches(event) {
		t.Error("Expected filter to not match when one criteria doesn't match")
	}
}

func TestSeverityLevels(t *testing.T) {
	tests := []struct {
		actual   Severity
		minimum  Severity
		expected bool
	}{
		{SeverityDebug, SeverityDebug, true},
		{SeverityInfo, SeverityDebug, true},
		{SeverityWarning, SeverityInfo, true},
		{SeverityError, SeverityWarning, true},
		{SeverityCritical, SeverityError, true},
		{SeverityDebug, SeverityInfo, false},
		{SeverityInfo, SeverityWarning, false},
		{SeverityWarning, SeverityError, false},
		{SeverityError, SeverityCritical, false},
	}

	for _, tt := range tests {
		result := severityAtLeast(tt.actual, tt.minimum)
		if result != tt.expected {
			t.Errorf("severityAtLeast(%s, %s) = %v, want %v",
				tt.actual, tt.minimum, result, tt.expected)
		}
	}
}

func TestEventBuilder_DataMap(t *testing.T) {
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	event := NewEvent(EventTypeJobComplete).
		DataMap(data).
		Build()

	if event.Data["key1"] != "value1" {
		t.Error("Expected key1 to be value1")
	}

	if event.Data["key2"] != 42 {
		t.Error("Expected key2 to be 42")
	}

	if event.Data["key3"] != true {
		t.Error("Expected key3 to be true")
	}
}

func TestEventBuilder_CorrelationID(t *testing.T) {
	correlationID := "deploy-abc123"

	event := NewEvent(EventTypeStateApplyStart).
		CorrelationID(correlationID).
		Build()

	if event.CorrelationID != correlationID {
		t.Errorf("Expected correlation ID %s, got %s", correlationID, event.CorrelationID)
	}
}

func TestEventSubject(t *testing.T) {
	event := NewEvent(EventTypeAgentConnect).Build()

	if event.Subject == "" {
		t.Error("Expected subject to be set automatically")
	}

	if event.Subject != string(EventTypeAgentConnect) {
		t.Errorf("Expected subject to be %s, got %s", EventTypeAgentConnect, event.Subject)
	}
}
