// Package observability provides metrics, logging, and tracing for proxy agents.
package observability

import (
	"encoding/json"
	"fmt"
	"time"
)

// ProxyEventType is the type of proxy event.
type ProxyEventType string

// EventDeviceDisconnected constants define the events.
const (
	// Device events
	EventDeviceConnected     ProxyEventType = "proxy.device.connected"
	EventDeviceDisconnected  ProxyEventType = "proxy.device.disconnected"
	EventDeviceHealthChanged ProxyEventType = "proxy.device.health_changed"
	EventDeviceConfigured    ProxyEventType = "proxy.device.configured"

	// Command events
	EventCommandStarted   ProxyEventType = "proxy.command.started"
	EventCommandCompleted ProxyEventType = "proxy.command.completed"
	EventCommandFailed    ProxyEventType = "proxy.command.failed"
	EventCommandTimeout   ProxyEventType = "proxy.command.timeout"

	// State events
	EventStateApplyStarted   ProxyEventType = "proxy.state.apply_started"
	EventStateApplyCompleted ProxyEventType = "proxy.state.apply_completed"
	EventStateApplyFailed    ProxyEventType = "proxy.state.apply_failed"
	EventStateChanged        ProxyEventType = "proxy.state.changed"

	// Drift events
	EventDriftCheckStarted   ProxyEventType = "proxy.drift.check_started"
	EventDriftCheckCompleted ProxyEventType = "proxy.drift.check_completed"
	EventDriftDetected       ProxyEventType = "proxy.drift.detected"
	EventDriftResolved       ProxyEventType = "proxy.drift.resolved"

	// Discovery events
	EventDiscoveryScanStarted    ProxyEventType = "proxy.discovery.scan_started"
	EventDiscoveryScanCompleted  ProxyEventType = "proxy.discovery.scan_completed"
	EventDiscoveryDeviceFound    ProxyEventType = "proxy.discovery.device_found"
	EventDiscoveryDeviceApproved ProxyEventType = "proxy.discovery.device_approved"
	EventDiscoveryDeviceRejected ProxyEventType = "proxy.discovery.device_rejected"

	// Connection events
	EventConnectionEstablished ProxyEventType = "proxy.connection.established"
	EventConnectionLost        ProxyEventType = "proxy.connection.lost"
	EventConnectionRetrying    ProxyEventType = "proxy.connection.retrying"

	// Error events
	EventError ProxyEventType = "proxy.error"
)

// ProxyEventSeverity is the severity of a proxy event.
type ProxyEventSeverity string

// SeverityDebug constants define the severity levels.
const (
	SeverityDebug    ProxyEventSeverity = "debug"
	SeverityInfo     ProxyEventSeverity = "info"
	SeverityWarning  ProxyEventSeverity = "warning"
	SeverityError    ProxyEventSeverity = "error"
	SeverityCritical ProxyEventSeverity = "critical"
)

// ProxyEvent represents an event from the proxy system.
type ProxyEvent struct {
	ID            string                 `json:"id"`
	Type          ProxyEventType         `json:"type"`
	Severity      ProxyEventSeverity     `json:"severity"`
	Timestamp     time.Time              `json:"timestamp"`
	Source        string                 `json:"source"`
	DeviceID      string                 `json:"device_id,omitempty"`
	Protocol      string                 `json:"protocol,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Message       string                 `json:"message"`
	Data          map[string]interface{} `json:"data,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Duration      time.Duration          `json:"duration_ms,omitempty"`
}

// ProxyEventEmitter emits proxy events.
type ProxyEventEmitter interface {
	Emit(event *ProxyEvent)
}

// ProxyEventHandler handles proxy events.
type ProxyEventHandler func(event *ProxyEvent)

// ProxyEventBus is a simple event bus for proxy events.
type ProxyEventBus struct {
	handlers []ProxyEventHandler
	buffer   chan *ProxyEvent
	stop     chan struct{}
}

// NewProxyEventBus creates a new event bus.
func NewProxyEventBus(bufferSize int) *ProxyEventBus {
	if bufferSize == 0 {
		bufferSize = 1000
	}

	bus := &ProxyEventBus{
		handlers: make([]ProxyEventHandler, 0),
		buffer:   make(chan *ProxyEvent, bufferSize),
		stop:     make(chan struct{}),
	}

	// Start processing goroutine
	go bus.process()

	return bus
}

// Subscribe adds an event handler.
func (b *ProxyEventBus) Subscribe(handler ProxyEventHandler) {
	b.handlers = append(b.handlers, handler)
}

// Emit emits an event to all handlers.
func (b *ProxyEventBus) Emit(event *ProxyEvent) {
	select {
	case b.buffer <- event:
	default:
		// Buffer full, drop event
	}
}

// process processes events from the buffer.
func (b *ProxyEventBus) process() {
	for {
		select {
		case <-b.stop:
			return
		case event := <-b.buffer:
			for _, handler := range b.handlers {
				handler(event)
			}
		}
	}
}

// Stop stops the event bus.
func (b *ProxyEventBus) Stop() {
	close(b.stop)
}

// ProxyEventBuilder builds proxy events.
type ProxyEventBuilder struct {
	event *ProxyEvent
}

// NewProxyEvent creates a new event builder.
func NewProxyEvent(eventType ProxyEventType) *ProxyEventBuilder {
	return &ProxyEventBuilder{
		event: &ProxyEvent{
			ID:        generateEventID(),
			Type:      eventType,
			Severity:  SeverityInfo,
			Timestamp: time.Now(),
			Data:      make(map[string]interface{}),
		},
	}
}

// WithSeverity sets the event severity.
func (b *ProxyEventBuilder) WithSeverity(severity ProxyEventSeverity) *ProxyEventBuilder {
	b.event.Severity = severity
	return b
}

// WithSource sets the event source.
func (b *ProxyEventBuilder) WithSource(source string) *ProxyEventBuilder {
	b.event.Source = source
	return b
}

// WithDeviceID sets the device ID.
func (b *ProxyEventBuilder) WithDeviceID(deviceID string) *ProxyEventBuilder {
	b.event.DeviceID = deviceID
	return b
}

// WithProtocol sets the protocol.
func (b *ProxyEventBuilder) WithProtocol(protocol string) *ProxyEventBuilder {
	b.event.Protocol = protocol
	return b
}

// WithCorrelationID sets the correlation ID.
func (b *ProxyEventBuilder) WithCorrelationID(correlationID string) *ProxyEventBuilder {
	b.event.CorrelationID = correlationID
	return b
}

// WithMessage sets the event message.
func (b *ProxyEventBuilder) WithMessage(message string) *ProxyEventBuilder {
	b.event.Message = message
	return b
}

// WithError sets the error.
func (b *ProxyEventBuilder) WithError(err error) *ProxyEventBuilder {
	if err != nil {
		b.event.Error = err.Error()
		if b.event.Severity == SeverityInfo {
			b.event.Severity = SeverityError
		}
	}
	return b
}

// WithDuration sets the duration.
func (b *ProxyEventBuilder) WithDuration(d time.Duration) *ProxyEventBuilder {
	b.event.Duration = d
	return b
}

// WithData adds data to the event.
func (b *ProxyEventBuilder) WithData(key string, value interface{}) *ProxyEventBuilder {
	b.event.Data[key] = value
	return b
}

// Build returns the built event.
func (b *ProxyEventBuilder) Build() *ProxyEvent {
	return b.event
}

// generateEventID generates a unique event ID.
var eventIDCounter int64

func generateEventID() string {
	eventIDCounter++
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), eventIDCounter)
}

// ProxyLogger provides structured logging for proxy operations.
type ProxyLogger struct {
	emitter ProxyEventEmitter
	source  string
}

// NewProxyLogger creates a new proxy logger.
func NewProxyLogger(emitter ProxyEventEmitter, source string) *ProxyLogger {
	return &ProxyLogger{
		emitter: emitter,
		source:  source,
	}
}

// Debug logs a debug message.
func (l *ProxyLogger) Debug(message string, data map[string]interface{}) {
	event := NewProxyEvent(EventError).
		WithSeverity(SeverityDebug).
		WithSource(l.source).
		WithMessage(message).
		Build()
	for k, v := range data {
		event.Data[k] = v
	}
	l.emitter.Emit(event)
}

// Info logs an info message.
func (l *ProxyLogger) Info(message string, data map[string]interface{}) {
	event := NewProxyEvent(EventError).
		WithSeverity(SeverityInfo).
		WithSource(l.source).
		WithMessage(message).
		Build()
	for k, v := range data {
		event.Data[k] = v
	}
	l.emitter.Emit(event)
}

// Warn logs a warning message.
func (l *ProxyLogger) Warn(message string, data map[string]interface{}) {
	event := NewProxyEvent(EventError).
		WithSeverity(SeverityWarning).
		WithSource(l.source).
		WithMessage(message).
		Build()
	for k, v := range data {
		event.Data[k] = v
	}
	l.emitter.Emit(event)
}

// Error logs an error message.
func (l *ProxyLogger) Error(message string, err error, data map[string]interface{}) {
	event := NewProxyEvent(EventError).
		WithSeverity(SeverityError).
		WithSource(l.source).
		WithMessage(message).
		WithError(err).
		Build()
	for k, v := range data {
		event.Data[k] = v
	}
	l.emitter.Emit(event)
}

// DeviceConnected logs a device connection event.
func (l *ProxyLogger) DeviceConnected(deviceID, protocol string, latency time.Duration) {
	l.emitter.Emit(NewProxyEvent(EventDeviceConnected).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithProtocol(protocol).
		WithDuration(latency).
		WithMessage(fmt.Sprintf("Device %s connected via %s", deviceID, protocol)).
		Build())
}

// DeviceDisconnected logs a device disconnection event.
func (l *ProxyLogger) DeviceDisconnected(deviceID, protocol, reason string) {
	l.emitter.Emit(NewProxyEvent(EventDeviceDisconnected).
		WithSeverity(SeverityWarning).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithProtocol(protocol).
		WithMessage(fmt.Sprintf("Device %s disconnected: %s", deviceID, reason)).
		Build())
}

// CommandStarted logs a command start event.
func (l *ProxyLogger) CommandStarted(deviceID, protocol, command, correlationID string) {
	l.emitter.Emit(NewProxyEvent(EventCommandStarted).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithProtocol(protocol).
		WithCorrelationID(correlationID).
		WithMessage(fmt.Sprintf("Executing command on %s", deviceID)).
		WithData("command", command).
		Build())
}

// CommandCompleted logs a command completion event.
func (l *ProxyLogger) CommandCompleted(deviceID, protocol, correlationID string, duration time.Duration, exitCode int) {
	l.emitter.Emit(NewProxyEvent(EventCommandCompleted).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithProtocol(protocol).
		WithCorrelationID(correlationID).
		WithDuration(duration).
		WithMessage(fmt.Sprintf("Command completed on %s", deviceID)).
		WithData("exit_code", exitCode).
		Build())
}

// CommandFailed logs a command failure event.
func (l *ProxyLogger) CommandFailed(deviceID, protocol, correlationID string, err error, duration time.Duration) {
	l.emitter.Emit(NewProxyEvent(EventCommandFailed).
		WithSeverity(SeverityError).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithProtocol(protocol).
		WithCorrelationID(correlationID).
		WithDuration(duration).
		WithMessage(fmt.Sprintf("Command failed on %s", deviceID)).
		WithError(err).
		Build())
}

// DriftDetected logs a drift detection event.
func (l *ProxyLogger) DriftDetected(deviceID, severity string, differences int) {
	l.emitter.Emit(NewProxyEvent(EventDriftDetected).
		WithSeverity(getSeverityFromDrift(severity)).
		WithSource(l.source).
		WithDeviceID(deviceID).
		WithMessage(fmt.Sprintf("Configuration drift detected on %s", deviceID)).
		WithData("severity", severity).
		WithData("differences", differences).
		Build())
}

// getSeverityFromDrift maps drift severity to event severity.
func getSeverityFromDrift(driftSeverity string) ProxyEventSeverity {
	switch driftSeverity {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityError
	case "medium":
		return SeverityWarning
	case "low":
		return SeverityInfo
	default:
		return SeverityInfo
	}
}

// JSONEventHandler creates a handler that outputs events as JSON.
func JSONEventHandler(output func([]byte)) ProxyEventHandler {
	return func(event *ProxyEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		output(data)
	}
}

// FilterEventHandler creates a handler that filters events by type.
func FilterEventHandler(types []ProxyEventType, handler ProxyEventHandler) ProxyEventHandler {
	typeSet := make(map[ProxyEventType]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	return func(event *ProxyEvent) {
		if typeSet[event.Type] {
			handler(event)
		}
	}
}

// SeverityFilterHandler creates a handler that filters events by minimum severity.
func SeverityFilterHandler(minSeverity ProxyEventSeverity, handler ProxyEventHandler) ProxyEventHandler {
	severityOrder := map[ProxyEventSeverity]int{
		SeverityDebug:    0,
		SeverityInfo:     1,
		SeverityWarning:  2,
		SeverityError:    3,
		SeverityCritical: 4,
	}

	minLevel := severityOrder[minSeverity]

	return func(event *ProxyEvent) {
		if severityOrder[event.Severity] >= minLevel {
			handler(event)
		}
	}
}
