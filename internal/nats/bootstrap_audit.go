package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// Bootstrap event types for the events system
const (
	// EventTypeBootstrapGenerate is emitted when a bootstrap credential is generated
	EventTypeBootstrapGenerate events.EventType = "bootstrap.generate"
	// EventTypeBootstrapValidate is emitted when a bootstrap credential is validated
	EventTypeBootstrapValidate events.EventType = "bootstrap.validate"
	// EventTypeBootstrapRevoke is emitted when a bootstrap credential is revoked
	EventTypeBootstrapRevoke events.EventType = "bootstrap.revoke"
	// EventTypeBootstrapUse is emitted when a bootstrap credential is used
	EventTypeBootstrapUse events.EventType = "bootstrap.use"
	// EventTypeBootstrapRegister is emitted when an agent registers via bootstrap
	EventTypeBootstrapRegister events.EventType = "bootstrap.register"
	// EventTypeBootstrapExpire is emitted when a bootstrap credential expires
	EventTypeBootstrapExpire events.EventType = "bootstrap.expire"
	// EventTypeBootstrapCleanup is emitted when expired credentials are cleaned up
	EventTypeBootstrapCleanup events.EventType = "bootstrap.cleanup"
)

// EventBasedBootstrapAuditLogger implements BootstrapAuditLogger using the events system
type EventBasedBootstrapAuditLogger struct {
	publisher events.EventPublisher
	source    string
}

// NewEventBasedBootstrapAuditLogger creates a new event-based audit logger
func NewEventBasedBootstrapAuditLogger(publisher events.EventPublisher, source string) *EventBasedBootstrapAuditLogger {
	if source == "" {
		source = "bootstrap-handler"
	}
	return &EventBasedBootstrapAuditLogger{
		publisher: publisher,
		source:    source,
	}
}

// Log logs a bootstrap audit event by publishing it to the events system
func (l *EventBasedBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	if l.publisher == nil {
		return nil
	}

	// Map bootstrap audit event type to events.EventType
	eventType := mapBootstrapEventType(event.EventType)

	// Determine severity based on success and event type
	severity := determineSeverity(event)

	// Build event data
	data := buildEventData(event)

	// Create the event
	e := events.NewEvent(eventType).
		Source(l.source).
		Severity(severity).
		DataMap(data)

	// Add correlation ID if we have an agent ID
	if event.AgentID != "" {
		e.CorrelationID(fmt.Sprintf("agent-%s", event.AgentID))
	} else if event.CredentialID != "" {
		e.CorrelationID(fmt.Sprintf("bootstrap-%s", event.CredentialID))
	}

	// Add tags
	e.Tag("category", "bootstrap")
	e.Tag("cluster", event.Cluster)
	if event.Success {
		e.Tag("status", "success")
	} else {
		e.Tag("status", "failure")
	}

	// Publish the event
	return l.publisher.Publish(e.Build())
}

// mapBootstrapEventType maps bootstrap audit event types to events.EventType
func mapBootstrapEventType(eventType string) events.EventType {
	switch eventType {
	case BootstrapAuditEventGenerate:
		return EventTypeBootstrapGenerate
	case BootstrapAuditEventValidate:
		return EventTypeBootstrapValidate
	case BootstrapAuditEventRevoke:
		return EventTypeBootstrapRevoke
	case BootstrapAuditEventUse:
		return EventTypeBootstrapUse
	case BootstrapAuditEventRegister:
		return EventTypeBootstrapRegister
	case BootstrapAuditEventExpire:
		return EventTypeBootstrapExpire
	case BootstrapAuditEventCleanup:
		return EventTypeBootstrapCleanup
	default:
		return events.EventType("bootstrap." + eventType)
	}
}

// determineSeverity determines the event severity based on the audit event
func determineSeverity(event BootstrapAuditEvent) events.Severity {
	if !event.Success {
		// Failed operations are warnings
		switch event.EventType {
		case BootstrapAuditEventRegister, BootstrapAuditEventValidate:
			// Failed registration or validation attempts are more concerning
			return events.SeverityWarning
		default:
			return events.SeverityWarning
		}
	}

	// Successful operations
	switch event.EventType {
	case BootstrapAuditEventGenerate:
		// Generating new credentials is informational
		return events.SeverityInfo
	case BootstrapAuditEventRevoke:
		// Revocation is informational but notable
		return events.SeverityInfo
	case BootstrapAuditEventRegister:
		// Successful registration is important
		return events.SeverityInfo
	case BootstrapAuditEventCleanup:
		// Cleanup is debug-level
		return events.SeverityDebug
	default:
		return events.SeverityInfo
	}
}

// buildEventData builds the event data map from a bootstrap audit event
func buildEventData(event BootstrapAuditEvent) map[string]interface{} {
	data := map[string]interface{}{
		"event_type": event.EventType,
		"cluster":    event.Cluster,
		"success":    event.Success,
		"timestamp":  event.Timestamp.Format(time.RFC3339),
	}

	if event.CredentialID != "" {
		data["credential_id"] = event.CredentialID
	}
	if event.AgentID != "" {
		data["agent_id"] = event.AgentID
	}
	if event.Error != "" {
		data["error"] = event.Error
	}
	if event.SourceIP != "" {
		data["source_ip"] = event.SourceIP
	}
	if len(event.Details) > 0 {
		data["details"] = event.Details
	}

	return data
}

// InMemoryBootstrapAuditLogger stores audit events in memory for testing/debugging
type InMemoryBootstrapAuditLogger struct {
	events    []BootstrapAuditEvent
	maxEvents int
}

// NewInMemoryBootstrapAuditLogger creates an in-memory audit logger
func NewInMemoryBootstrapAuditLogger(maxEvents int) *InMemoryBootstrapAuditLogger {
	if maxEvents <= 0 {
		maxEvents = 1000
	}
	return &InMemoryBootstrapAuditLogger{
		events:    make([]BootstrapAuditEvent, 0),
		maxEvents: maxEvents,
	}
}

// Log stores an audit event in memory
func (l *InMemoryBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	// Remove oldest events if at capacity
	if len(l.events) >= l.maxEvents {
		l.events = l.events[1:]
	}
	l.events = append(l.events, event)
	return nil
}

// GetEvents returns all stored events
func (l *InMemoryBootstrapAuditLogger) GetEvents() []BootstrapAuditEvent {
	result := make([]BootstrapAuditEvent, len(l.events))
	copy(result, l.events)
	return result
}

// GetEventsByType returns events filtered by type
func (l *InMemoryBootstrapAuditLogger) GetEventsByType(eventType string) []BootstrapAuditEvent {
	var result []BootstrapAuditEvent
	for _, event := range l.events {
		if event.EventType == eventType {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByCredentialID returns events for a specific credential
func (l *InMemoryBootstrapAuditLogger) GetEventsByCredentialID(credentialID string) []BootstrapAuditEvent {
	var result []BootstrapAuditEvent
	for _, event := range l.events {
		if event.CredentialID == credentialID {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByAgentID returns events for a specific agent
func (l *InMemoryBootstrapAuditLogger) GetEventsByAgentID(agentID string) []BootstrapAuditEvent {
	var result []BootstrapAuditEvent
	for _, event := range l.events {
		if event.AgentID == agentID {
			result = append(result, event)
		}
	}
	return result
}

// GetFailedEvents returns all failed events
func (l *InMemoryBootstrapAuditLogger) GetFailedEvents() []BootstrapAuditEvent {
	var result []BootstrapAuditEvent
	for _, event := range l.events {
		if !event.Success {
			result = append(result, event)
		}
	}
	return result
}

// Clear removes all stored events
func (l *InMemoryBootstrapAuditLogger) Clear() {
	l.events = l.events[:0]
}

// Count returns the number of stored events
func (l *InMemoryBootstrapAuditLogger) Count() int {
	return len(l.events)
}

// CompositeBootstrapAuditLogger logs to multiple loggers
type CompositeBootstrapAuditLogger struct {
	loggers []BootstrapAuditLogger
}

// NewCompositeBootstrapAuditLogger creates a composite logger
func NewCompositeBootstrapAuditLogger(loggers ...BootstrapAuditLogger) *CompositeBootstrapAuditLogger {
	return &CompositeBootstrapAuditLogger{
		loggers: loggers,
	}
}

// AddLogger adds a logger to the composite
func (l *CompositeBootstrapAuditLogger) AddLogger(logger BootstrapAuditLogger) {
	l.loggers = append(l.loggers, logger)
}

// Log logs to all configured loggers
func (l *CompositeBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	var lastErr error
	for _, logger := range l.loggers {
		if err := logger.Log(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// FilteredBootstrapAuditLogger filters events before logging
type FilteredBootstrapAuditLogger struct {
	logger BootstrapAuditLogger
	filter func(BootstrapAuditEvent) bool
}

// NewFilteredBootstrapAuditLogger creates a filtered logger
func NewFilteredBootstrapAuditLogger(logger BootstrapAuditLogger, filter func(BootstrapAuditEvent) bool) *FilteredBootstrapAuditLogger {
	return &FilteredBootstrapAuditLogger{
		logger: logger,
		filter: filter,
	}
}

// Log logs the event if it passes the filter
func (l *FilteredBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	if l.filter != nil && !l.filter(event) {
		return nil
	}
	return l.logger.Log(ctx, event)
}

// FailuresOnlyFilter returns a filter that only passes failed events
func FailuresOnlyFilter(event BootstrapAuditEvent) bool {
	return !event.Success
}

// EventTypesFilter returns a filter that only passes specified event types
func EventTypesFilter(types ...string) func(BootstrapAuditEvent) bool {
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	return func(event BootstrapAuditEvent) bool {
		return typeSet[event.EventType]
	}
}
