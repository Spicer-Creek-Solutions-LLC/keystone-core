package observability

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestNewProxyEventBus(t *testing.T) {
	bus := NewProxyEventBus(100)

	if bus == nil {
		t.Fatal("NewProxyEventBus returned nil")
	}

	if bus.handlers == nil {
		t.Error("handlers slice is nil")
	}

	if bus.buffer == nil {
		t.Error("buffer channel is nil")
	}

	bus.Stop()
}

func TestNewProxyEventBus_DefaultBufferSize(t *testing.T) {
	bus := NewProxyEventBus(0)

	if bus == nil {
		t.Fatal("NewProxyEventBus returned nil")
	}

	// Buffer should be created with default size (1000)
	if cap(bus.buffer) != 1000 {
		t.Errorf("expected buffer capacity 1000, got %d", cap(bus.buffer))
	}

	bus.Stop()
}

func TestProxyEventBus_Subscribe(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	bus.Subscribe(func(event *ProxyEvent) {
		// Handler registered
	})

	if len(bus.handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(bus.handlers))
	}
}

func TestProxyEventBus_Emit(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	var received *ProxyEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(func(event *ProxyEvent) {
		received = event
		wg.Done()
	})

	event := NewProxyEvent(EventDeviceConnected).
		WithSource("test").
		WithMessage("Test message").
		Build()

	bus.Emit(event)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	if received == nil {
		t.Fatal("event not received")
	}

	if received.Type != EventDeviceConnected {
		t.Errorf("expected type %s, got %s", EventDeviceConnected, received.Type)
	}

	if received.Message != "Test message" {
		t.Errorf("expected message 'Test message', got '%s'", received.Message)
	}
}

func TestProxyEventBus_EmitMultipleHandlers(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	count := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		bus.Subscribe(func(event *ProxyEvent) {
			mu.Lock()
			count++
			mu.Unlock()
			wg.Done()
		})
	}

	event := NewProxyEvent(EventDeviceConnected).Build()
	bus.Emit(event)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for handlers")
	}

	mu.Lock()
	if count != 3 {
		t.Errorf("expected 3 handler calls, got %d", count)
	}
	mu.Unlock()
}

func TestNewProxyEvent(t *testing.T) {
	builder := NewProxyEvent(EventCommandStarted)

	if builder == nil {
		t.Fatal("NewProxyEvent returned nil")
	}

	event := builder.Build()

	if event.Type != EventCommandStarted {
		t.Errorf("expected type %s, got %s", EventCommandStarted, event.Type)
	}

	if event.ID == "" {
		t.Error("expected non-empty ID")
	}

	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	if event.Severity != SeverityInfo {
		t.Errorf("expected default severity info, got %s", event.Severity)
	}
}

func TestProxyEventBuilder_WithSeverity(t *testing.T) {
	event := NewProxyEvent(EventDeviceDisconnected).
		WithSeverity(SeverityWarning).
		Build()

	if event.Severity != SeverityWarning {
		t.Errorf("expected severity warning, got %s", event.Severity)
	}
}

func TestProxyEventBuilder_WithSource(t *testing.T) {
	event := NewProxyEvent(EventDeviceConnected).
		WithSource("proxy-agent").
		Build()

	if event.Source != "proxy-agent" {
		t.Errorf("expected source 'proxy-agent', got '%s'", event.Source)
	}
}

func TestProxyEventBuilder_WithDeviceID(t *testing.T) {
	event := NewProxyEvent(EventDeviceConnected).
		WithDeviceID("device-123").
		Build()

	if event.DeviceID != "device-123" {
		t.Errorf("expected device ID 'device-123', got '%s'", event.DeviceID)
	}
}

func TestProxyEventBuilder_WithProtocol(t *testing.T) {
	event := NewProxyEvent(EventDeviceConnected).
		WithProtocol("ssh").
		Build()

	if event.Protocol != "ssh" {
		t.Errorf("expected protocol 'ssh', got '%s'", event.Protocol)
	}
}

func TestProxyEventBuilder_WithCorrelationID(t *testing.T) {
	event := NewProxyEvent(EventCommandStarted).
		WithCorrelationID("corr-456").
		Build()

	if event.CorrelationID != "corr-456" {
		t.Errorf("expected correlation ID 'corr-456', got '%s'", event.CorrelationID)
	}
}

func TestProxyEventBuilder_WithMessage(t *testing.T) {
	event := NewProxyEvent(EventDeviceConnected).
		WithMessage("Device connected successfully").
		Build()

	if event.Message != "Device connected successfully" {
		t.Errorf("expected message 'Device connected successfully', got '%s'", event.Message)
	}
}

func TestProxyEventBuilder_WithError(t *testing.T) {
	err := context.DeadlineExceeded
	event := NewProxyEvent(EventCommandFailed).
		WithError(err).
		Build()

	if event.Error != err.Error() {
		t.Errorf("expected error '%s', got '%s'", err.Error(), event.Error)
	}

	// Should auto-upgrade severity
	if event.Severity != SeverityError {
		t.Errorf("expected severity to be upgraded to error, got %s", event.Severity)
	}
}

func TestProxyEventBuilder_WithDuration(t *testing.T) {
	event := NewProxyEvent(EventCommandCompleted).
		WithDuration(150 * time.Millisecond).
		Build()

	if event.Duration != 150*time.Millisecond {
		t.Errorf("expected duration 150ms, got %v", event.Duration)
	}
}

func TestProxyEventBuilder_WithData(t *testing.T) {
	event := NewProxyEvent(EventCommandStarted).
		WithData("command", "show version").
		WithData("retries", 3).
		Build()

	if event.Data["command"] != "show version" {
		t.Errorf("expected data command 'show version', got '%v'", event.Data["command"])
	}

	if event.Data["retries"] != 3 {
		t.Errorf("expected data retries 3, got '%v'", event.Data["retries"])
	}
}

func TestProxyEventBuilder_Chaining(t *testing.T) {
	event := NewProxyEvent(EventCommandCompleted).
		WithSeverity(SeverityInfo).
		WithSource("proxy").
		WithDeviceID("device-1").
		WithProtocol("ssh").
		WithCorrelationID("corr-1").
		WithMessage("Command completed").
		WithDuration(100 * time.Millisecond).
		WithData("exit_code", 0).
		Build()

	if event.Type != EventCommandCompleted {
		t.Error("type mismatch")
	}
	if event.Severity != SeverityInfo {
		t.Error("severity mismatch")
	}
	if event.Source != "proxy" {
		t.Error("source mismatch")
	}
	if event.DeviceID != "device-1" {
		t.Error("deviceID mismatch")
	}
	if event.Protocol != "ssh" {
		t.Error("protocol mismatch")
	}
	if event.CorrelationID != "corr-1" {
		t.Error("correlationID mismatch")
	}
	if event.Message != "Command completed" {
		t.Error("message mismatch")
	}
	if event.Duration != 100*time.Millisecond {
		t.Error("duration mismatch")
	}
	if event.Data["exit_code"] != 0 {
		t.Error("data mismatch")
	}
}

func TestProxyEventTypeConstants(t *testing.T) {
	tests := []struct {
		eventType ProxyEventType
		expected  string
	}{
		{EventDeviceConnected, "proxy.device.connected"},
		{EventDeviceDisconnected, "proxy.device.disconnected"},
		{EventDeviceHealthChanged, "proxy.device.health_changed"},
		{EventDeviceConfigured, "proxy.device.configured"},
		{EventCommandStarted, "proxy.command.started"},
		{EventCommandCompleted, "proxy.command.completed"},
		{EventCommandFailed, "proxy.command.failed"},
		{EventCommandTimeout, "proxy.command.timeout"},
		{EventStateApplyStarted, "proxy.state.apply_started"},
		{EventStateApplyCompleted, "proxy.state.apply_completed"},
		{EventStateApplyFailed, "proxy.state.apply_failed"},
		{EventStateChanged, "proxy.state.changed"},
		{EventDriftCheckStarted, "proxy.drift.check_started"},
		{EventDriftCheckCompleted, "proxy.drift.check_completed"},
		{EventDriftDetected, "proxy.drift.detected"},
		{EventDriftResolved, "proxy.drift.resolved"},
		{EventDiscoveryScanStarted, "proxy.discovery.scan_started"},
		{EventDiscoveryScanCompleted, "proxy.discovery.scan_completed"},
		{EventDiscoveryDeviceFound, "proxy.discovery.device_found"},
		{EventDiscoveryDeviceApproved, "proxy.discovery.device_approved"},
		{EventDiscoveryDeviceRejected, "proxy.discovery.device_rejected"},
		{EventConnectionEstablished, "proxy.connection.established"},
		{EventConnectionLost, "proxy.connection.lost"},
		{EventConnectionRetrying, "proxy.connection.retrying"},
		{EventError, "proxy.error"},
	}

	for _, tt := range tests {
		if string(tt.eventType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.eventType)
		}
	}
}

func TestProxyEventSeverityConstants(t *testing.T) {
	tests := []struct {
		severity ProxyEventSeverity
		expected string
	}{
		{SeverityDebug, "debug"},
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
	}

	for _, tt := range tests {
		if string(tt.severity) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.severity)
		}
	}
}

func TestProxyEvent_JSON(t *testing.T) {
	event := NewProxyEvent(EventDeviceConnected).
		WithSource("test").
		WithDeviceID("device-1").
		WithMessage("Connected").
		Build()

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var unmarshaled ProxyEvent
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if unmarshaled.Type != event.Type {
		t.Errorf("type mismatch: expected %s, got %s", event.Type, unmarshaled.Type)
	}

	if unmarshaled.DeviceID != event.DeviceID {
		t.Errorf("deviceID mismatch: expected %s, got %s", event.DeviceID, unmarshaled.DeviceID)
	}
}

func TestNewProxyLogger(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	logger := NewProxyLogger(bus, "test-source")

	if logger == nil {
		t.Fatal("NewProxyLogger returned nil")
	}

	if logger.source != "test-source" {
		t.Errorf("expected source 'test-source', got '%s'", logger.source)
	}
}

func TestProxyLogger_Debug(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	var received *ProxyEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(func(event *ProxyEvent) {
		received = event
		wg.Done()
	})

	logger := NewProxyLogger(bus, "test")
	logger.Debug("Debug message", map[string]interface{}{"key": "value"})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}

	if received.Severity != SeverityDebug {
		t.Errorf("expected severity debug, got %s", received.Severity)
	}
}

func TestProxyLogger_DeviceConnected(t *testing.T) {
	bus := NewProxyEventBus(100)
	defer bus.Stop()

	var received *ProxyEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(func(event *ProxyEvent) {
		received = event
		wg.Done()
	})

	logger := NewProxyLogger(bus, "test")
	logger.DeviceConnected("device-1", "ssh", 100*time.Millisecond)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}

	if received.Type != EventDeviceConnected {
		t.Errorf("expected type %s, got %s", EventDeviceConnected, received.Type)
	}

	if received.DeviceID != "device-1" {
		t.Errorf("expected device ID 'device-1', got '%s'", received.DeviceID)
	}

	if received.Protocol != "ssh" {
		t.Errorf("expected protocol 'ssh', got '%s'", received.Protocol)
	}
}

func TestJSONEventHandler(t *testing.T) {
	var output []byte

	handler := JSONEventHandler(func(data []byte) {
		output = data
	})

	event := NewProxyEvent(EventDeviceConnected).
		WithDeviceID("device-1").
		Build()

	handler(event)

	if len(output) == 0 {
		t.Error("expected non-empty output")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal(output, &parsed)
	if err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestFilterEventHandler(t *testing.T) {
	var received []*ProxyEvent

	handler := FilterEventHandler(
		[]ProxyEventType{EventDeviceConnected, EventDeviceDisconnected},
		func(event *ProxyEvent) {
			received = append(received, event)
		},
	)

	// Should be handled
	handler(NewProxyEvent(EventDeviceConnected).Build())
	handler(NewProxyEvent(EventDeviceDisconnected).Build())

	// Should be filtered out
	handler(NewProxyEvent(EventCommandStarted).Build())
	handler(NewProxyEvent(EventStateChanged).Build())

	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
}

func TestSeverityFilterHandler(t *testing.T) {
	var received []*ProxyEvent

	handler := SeverityFilterHandler(
		SeverityWarning,
		func(event *ProxyEvent) {
			received = append(received, event)
		},
	)

	// Should be handled (severity >= warning)
	handler(NewProxyEvent(EventError).WithSeverity(SeverityWarning).Build())
	handler(NewProxyEvent(EventError).WithSeverity(SeverityError).Build())
	handler(NewProxyEvent(EventError).WithSeverity(SeverityCritical).Build())

	// Should be filtered out (severity < warning)
	handler(NewProxyEvent(EventError).WithSeverity(SeverityDebug).Build())
	handler(NewProxyEvent(EventError).WithSeverity(SeverityInfo).Build())

	if len(received) != 3 {
		t.Errorf("expected 3 events, got %d", len(received))
	}
}

func TestGetSeverityFromDrift(t *testing.T) {
	tests := []struct {
		driftSeverity string
		expected      ProxyEventSeverity
	}{
		{"critical", SeverityCritical},
		{"high", SeverityError},
		{"medium", SeverityWarning},
		{"low", SeverityInfo},
		{"unknown", SeverityInfo},
	}

	for _, tt := range tests {
		result := getSeverityFromDrift(tt.driftSeverity)
		if result != tt.expected {
			t.Errorf("getSeverityFromDrift(%s): expected %s, got %s", tt.driftSeverity, tt.expected, result)
		}
	}
}
