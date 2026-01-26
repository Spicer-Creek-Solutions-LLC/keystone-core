package nats

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestNewConnectionDebugger(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	if debugger == nil {
		t.Fatal("expected non-nil debugger")
	}
	if debugger.config != config {
		t.Error("expected config to be set")
	}
}

func TestConnectionDebugger_RecordEvent(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	event := ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "nats://localhost:4222",
		Strategy:  "primary",
	}

	debugger.RecordEvent(event)

	events := debugger.GetEvents(nil)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventTypeConnect {
		t.Errorf("expected connect event, got %v", events[0].Type)
	}
}

func TestConnectionDebugger_EventBufferLimit(t *testing.T) {
	config := DefaultDebugConfig()
	config.MaxEvents = 10
	debugger := NewConnectionDebugger(config)

	// Record more events than buffer size
	for i := 0; i < 15; i++ {
		debugger.RecordEvent(ConnectionEvent{
			Timestamp: time.Now(),
			Type:      EventTypeConnect,
			Endpoint:  "endpoint",
		})
	}

	events := debugger.GetEvents(nil)
	if len(events) != 10 {
		t.Errorf("expected 10 events (buffer limit), got %d", len(events))
	}
}

func TestConnectionDebugger_StartTrace(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	traceID := "trace-123"
	debugger.StartTrace(traceID, "msg-456", "cmd.execute", "agent-1", 1024)

	trace := debugger.GetTrace(traceID)
	if trace == nil {
		t.Fatal("expected trace to exist")
	}
	if trace.TraceID != traceID {
		t.Errorf("expected trace ID %s, got %s", traceID, trace.TraceID)
	}
	if trace.MessageID != "msg-456" {
		t.Errorf("expected message ID msg-456, got %s", trace.MessageID)
	}
	if trace.Subject != "cmd.execute" {
		t.Errorf("expected subject cmd.execute, got %s", trace.Subject)
	}
}

func TestConnectionDebugger_AddHop(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	traceID := "trace-123"
	debugger.StartTrace(traceID, "msg-456", "cmd.execute", "agent-1", 1024)
	debugger.AddHop(traceID, "server-1", "nats://localhost:4222", "received")
	debugger.CompleteHop(traceID, nil)
	debugger.AddHop(traceID, "server-2", "nats://localhost:4223", "forwarded")
	debugger.CompleteHop(traceID, nil)

	trace := debugger.GetTrace(traceID)
	if len(trace.Hops) != 2 {
		t.Errorf("expected 2 hops, got %d", len(trace.Hops))
	}

	if trace.Hops[0].Component != "server-1" {
		t.Errorf("expected first hop component server-1, got %s", trace.Hops[0].Component)
	}
	if trace.Hops[1].Action != "forwarded" {
		t.Errorf("expected second hop action forwarded, got %s", trace.Hops[1].Action)
	}
}

func TestConnectionDebugger_EndTrace(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	traceID := "trace-123"
	debugger.StartTrace(traceID, "msg-456", "cmd.execute", "agent-1", 1024)
	debugger.AddHop(traceID, "server-1", "nats://localhost:4222", "received")
	debugger.CompleteHop(traceID, nil)

	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 5*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("trace delay did not elapse: %v", err)
	}
	debugger.EndTrace(traceID, "agent-2", "delivered", nil)

	trace := debugger.GetTrace(traceID)
	if trace.Status != "delivered" {
		t.Errorf("expected status 'delivered', got %s", trace.Status)
	}
	if trace.TotalLatency == 0 {
		t.Error("expected non-zero total latency")
	}
}

func TestConnectionDebugger_EndTraceWithError(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	traceID := "trace-123"
	debugger.StartTrace(traceID, "msg-456", "cmd.execute", "agent-1", 1024)
	debugger.EndTrace(traceID, "", "failed", fmt.Errorf("timeout"))

	trace := debugger.GetTrace(traceID)
	if trace.Status != "failed" {
		t.Errorf("expected status 'failed', got %s", trace.Status)
	}
	if trace.Error != "timeout" {
		t.Errorf("expected error 'timeout', got %s", trace.Error)
	}
}

func TestConnectionDebugger_Timeline(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	endpoint := "nats://localhost:4222"

	// Record events for timeline
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  endpoint,
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now().Add(1 * time.Second),
		Type:      EventTypeDisconnect,
		Endpoint:  endpoint,
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now().Add(2 * time.Second),
		Type:      EventTypeReconnect,
		Endpoint:  endpoint,
	})

	timeline := debugger.GetTimeline(endpoint)
	if timeline == nil {
		t.Fatal("expected timeline")
	}
	if len(timeline.Events) != 3 {
		t.Errorf("expected 3 events in timeline, got %d", len(timeline.Events))
	}
}

func TestConnectionDebugger_GetTraces(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	// Start some traces
	debugger.StartTrace("trace-1", "msg-1", "subject-1", "source-1", 100)
	debugger.StartTrace("trace-2", "msg-2", "subject-2", "source-2", 200)
	debugger.StartTrace("trace-3", "msg-3", "subject-3", "source-3", 300)

	// End all traces (GetTraces only returns completed traces)
	debugger.EndTrace("trace-1", "dest1", "delivered", nil)
	debugger.EndTrace("trace-2", "dest2", "delivered", nil)
	debugger.EndTrace("trace-3", "dest3", "delivered", nil)

	// Get all completed traces
	allTraces := debugger.GetTraces(nil)
	if len(allTraces) != 3 {
		t.Errorf("expected 3 traces, got %d", len(allTraces))
	}
}

func TestConnectionDebugger_DiagnosticReport(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	// Record some events
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "nats://localhost:4222",
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeError,
		Endpoint:  "nats://localhost:4222",
		Error:     "connection timeout",
	})

	// Start a trace
	debugger.StartTrace("trace-1", "msg-1", "subject", "source", 100)

	report := debugger.GenerateDiagnosticReport()
	if report == nil {
		t.Fatal("expected diagnostic report")
	}
	if report.ActiveTraces != 1 {
		t.Errorf("expected 1 active trace, got %d", report.ActiveTraces)
	}
}

func TestConnectionDebugger_Clear(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	// Record events
	for i := 0; i < 5; i++ {
		debugger.RecordEvent(ConnectionEvent{
			Timestamp: time.Now(),
			Type:      EventTypeConnect,
		})
	}

	debugger.Clear()
	events := debugger.GetEvents(nil)
	if len(events) != 0 {
		t.Errorf("expected 0 events after clear, got %d", len(events))
	}
}

func TestDiagnosticCLI_StatusCommand(t *testing.T) {
	debugConfig := DefaultDebugConfig()
	debugger := NewConnectionDebugger(debugConfig)

	obsConfig := DefaultObservabilityConfig()
	collector := NewNATSMetricsCollector(obsConfig)

	// Record some events
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "nats://localhost:4222",
	})

	cli := NewDiagnosticCLI(debugger, collector)
	output := cli.StatusCommand()

	if !strings.Contains(output, "Connection Status") {
		t.Error("expected status output to contain 'Connection Status'")
	}
	if !strings.Contains(output, "nats://localhost:4222") {
		t.Error("expected status to show endpoint")
	}
}

func TestDiagnosticCLI_DiagnoseCommand(t *testing.T) {
	debugConfig := DefaultDebugConfig()
	debugger := NewConnectionDebugger(debugConfig)

	obsConfig := DefaultObservabilityConfig()
	collector := NewNATSMetricsCollector(obsConfig)

	cli := NewDiagnosticCLI(debugger, collector)
	output := cli.DiagnoseCommand()

	if !strings.Contains(output, "Diagnostic Report") {
		t.Error("expected diagnose output to contain 'Diagnostic Report'")
	}
}

func TestDiagnosticCLI_TraceCommand(t *testing.T) {
	debugConfig := DefaultDebugConfig()
	debugConfig.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(debugConfig)

	obsConfig := DefaultObservabilityConfig()
	collector := NewNATSMetricsCollector(obsConfig)

	// Create a trace
	debugger.StartTrace("trace-123", "msg-456", "cmd.execute", "agent-1", 1024)
	debugger.AddHop("trace-123", "server-1", "nats://localhost:4222", "received")
	debugger.CompleteHop("trace-123", nil)
	debugger.EndTrace("trace-123", "dest", "delivered", nil)

	cli := NewDiagnosticCLI(debugger, collector)
	output := cli.TraceCommand("trace-123")

	if !strings.Contains(output, "trace-123") {
		t.Error("expected trace output to contain trace ID")
	}
	if !strings.Contains(output, "server-1") {
		t.Error("expected trace output to contain hop component")
	}
}

func TestDiagnosticCLI_EventsCommand(t *testing.T) {
	debugConfig := DefaultDebugConfig()
	debugger := NewConnectionDebugger(debugConfig)

	obsConfig := DefaultObservabilityConfig()
	collector := NewNATSMetricsCollector(obsConfig)

	endpoint := "nats://localhost:4222"
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  endpoint,
	})

	cli := NewDiagnosticCLI(debugger, collector)
	output := cli.EventsCommand(10)

	if !strings.Contains(output, "Recent Connection Events") {
		t.Error("expected events output to contain 'Recent Connection Events'")
	}
}

func TestDebugConfig_Defaults(t *testing.T) {
	config := DefaultDebugConfig()

	if config.MaxEvents == 0 {
		t.Error("expected non-zero max events")
	}
	if config.Level != DebugLevelInfo {
		t.Error("expected default level to be info")
	}
}

func TestConnectionEventTypes(t *testing.T) {
	types := []ConnectionEventType{
		EventTypeConnect,
		EventTypeDisconnect,
		EventTypeReconnect,
		EventTypeError,
		EventTypeFailover,
		EventTypeStateChange,
		EventTypeLatencySpike,
		EventTypeBufferWarning,
		EventTypeCircuitOpen,
		EventTypeCircuitClose,
	}

	// Just verify these constants exist and are distinct
	seen := make(map[ConnectionEventType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type: %v", et)
		}
		seen[et] = true
	}
}

func TestConnectionDebugger_ExportJSON(t *testing.T) {
	config := DefaultDebugConfig()
	config.EnableMessageTracing = true // Enable tracing
	debugger := NewConnectionDebugger(config)

	// Record some data
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "nats://localhost:4222",
	})
	debugger.StartTrace("trace-1", "msg-1", "subject", "source", 100)
	debugger.EndTrace("trace-1", "dest", "delivered", nil) // Complete the trace

	jsonData, err := debugger.ExportJSON()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	if !strings.Contains(jsonData, "events") {
		t.Error("expected JSON to contain events")
	}
	if !strings.Contains(jsonData, "trace-1") {
		t.Error("expected JSON to contain trace ID")
	}
}

func TestMessageTrace_Hops(t *testing.T) {
	trace := &MessageTrace{
		TraceID:   "test-trace",
		MessageID: "test-msg",
		Subject:   "test.subject",
		Source:    "test-source",
		Size:      1024,
		StartTime: time.Now(),
		Hops:      []MessageHop{},
	}

	// Add hops
	trace.Hops = append(trace.Hops, MessageHop{
		HopNumber: 1,
		Component: "node-1",
		Action:    "received",
		Latency:   5 * time.Millisecond,
	})
	trace.Hops = append(trace.Hops, MessageHop{
		HopNumber: 2,
		Component: "node-2",
		Action:    "delivered",
		Latency:   10 * time.Millisecond,
	})

	if len(trace.Hops) != 2 {
		t.Errorf("expected 2 hops, got %d", len(trace.Hops))
	}

	totalLatency := trace.Hops[0].Latency + trace.Hops[1].Latency
	if totalLatency != 15*time.Millisecond {
		t.Errorf("expected total latency 15ms, got %v", totalLatency)
	}
}

func TestConnectionTimeline_Statistics(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	endpoint := "nats://localhost:4222"
	now := time.Now()

	// Record connection and disconnection events
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now,
		Type:      EventTypeConnect,
		Endpoint:  endpoint,
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now.Add(10 * time.Second),
		Type:      EventTypeDisconnect,
		Endpoint:  endpoint,
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now.Add(12 * time.Second),
		Type:      EventTypeConnect,
		Endpoint:  endpoint,
	})

	timeline := debugger.GetTimeline(endpoint)
	if timeline == nil {
		t.Fatal("expected timeline")
	}

	// Should track connect and disconnect counts
	if timeline.Summary.TotalConnections < 2 {
		t.Errorf("expected at least 2 connects, got %d", timeline.Summary.TotalConnections)
	}
}

func TestGetAllTimelines(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	// Record events for multiple endpoints
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "endpoint-1",
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "endpoint-2",
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: time.Now(),
		Type:      EventTypeConnect,
		Endpoint:  "endpoint-3",
	})

	timelines := debugger.GetAllTimelines()
	if len(timelines) != 3 {
		t.Errorf("expected 3 timelines, got %d", len(timelines))
	}
}

func TestDebugLevel_String(t *testing.T) {
	tests := []struct {
		level    DebugLevel
		expected string
	}{
		{DebugLevelOff, "off"},
		{DebugLevelError, "error"},
		{DebugLevelWarn, "warn"},
		{DebugLevelInfo, "info"},
		{DebugLevelDebug, "debug"},
		{DebugLevelTrace, "trace"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("DebugLevel(%d).String() = %s, want %s", tt.level, got, tt.expected)
		}
	}
}

func TestParseDebugLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected DebugLevel
	}{
		{"error", DebugLevelError},
		{"warn", DebugLevelWarn},
		{"warning", DebugLevelWarn},
		{"info", DebugLevelInfo},
		{"debug", DebugLevelDebug},
		{"trace", DebugLevelTrace},
		{"invalid", DebugLevelOff},
		{"", DebugLevelOff},
	}

	for _, tt := range tests {
		if got := ParseDebugLevel(tt.input); got != tt.expected {
			t.Errorf("ParseDebugLevel(%s) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestEventFilter(t *testing.T) {
	config := DefaultDebugConfig()
	debugger := NewConnectionDebugger(config)

	now := time.Now()
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now,
		Type:      EventTypeConnect,
		Endpoint:  "endpoint-1",
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now.Add(1 * time.Second),
		Type:      EventTypeError,
		Endpoint:  "endpoint-2",
	})
	debugger.RecordEvent(ConnectionEvent{
		Timestamp: now.Add(2 * time.Second),
		Type:      EventTypeConnect,
		Endpoint:  "endpoint-1",
	})

	// Filter by type
	filter := &EventFilter{Types: []ConnectionEventType{EventTypeConnect}}
	events := debugger.GetEvents(filter)
	if len(events) != 2 {
		t.Errorf("expected 2 connect events, got %d", len(events))
	}

	// Filter by endpoint
	filter = &EventFilter{Endpoints: []string{"endpoint-1"}}
	events = debugger.GetEvents(filter)
	if len(events) != 2 {
		t.Errorf("expected 2 events for endpoint-1, got %d", len(events))
	}
}
