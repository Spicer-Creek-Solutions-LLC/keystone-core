package debug

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelOff, "off"},
		{LevelBasic, "basic"},
		{LevelVerbose, "verbose"},
		{LevelTrace, "trace"},
		{Level(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level(%d).String() = %s, want %s", tt.level, got, tt.expected)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"off", LevelOff},
		{"none", LevelOff},
		{"0", LevelOff},
		{"basic", LevelBasic},
		{"info", LevelBasic},
		{"1", LevelBasic},
		{"verbose", LevelVerbose},
		{"debug", LevelVerbose},
		{"2", LevelVerbose},
		{"trace", LevelTrace},
		{"all", LevelTrace},
		{"3", LevelTrace},
		{"VERBOSE", LevelVerbose}, // case insensitive
		{"invalid", LevelOff},
	}

	for _, tt := range tests {
		if got := ParseLevel(tt.input); got != tt.expected {
			t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestNewLogger(t *testing.T) {
	l := NewLogger()

	if l.Level() != LevelOff {
		t.Errorf("Default level = %d, want LevelOff", l.Level())
	}
	if l.format != FormatText {
		t.Errorf("Default format = %s, want text", l.format)
	}
}

func TestNewLogger_WithOptions(t *testing.T) {
	var buf bytes.Buffer

	l := NewLogger(
		WithLevel(LevelVerbose),
		WithOutput(&buf),
		WithFormat(FormatJSON),
		WithProtocol(ProtocolSSH),
		WithDeviceID("device-1"),
		WithSessionID("session-1"),
		WithMaxEvents(100),
	)

	if l.Level() != LevelVerbose {
		t.Errorf("Level = %d, want LevelVerbose", l.Level())
	}
	if l.format != FormatJSON {
		t.Errorf("Format = %s, want json", l.format)
	}
	if l.protocol != ProtocolSSH {
		t.Errorf("Protocol = %s, want ssh", l.protocol)
	}
	if l.deviceID != "device-1" {
		t.Errorf("DeviceID = %s, want device-1", l.deviceID)
	}
	if l.sessionID != "session-1" {
		t.Errorf("SessionID = %s, want session-1", l.sessionID)
	}
	if l.maxEvents != 100 {
		t.Errorf("MaxEvents = %d, want 100", l.maxEvents)
	}
}

func TestLogger_SetLevel(t *testing.T) {
	l := NewLogger()

	l.SetLevel(LevelTrace)
	if l.Level() != LevelTrace {
		t.Errorf("Level = %d, want LevelTrace", l.Level())
	}
}

func TestLogger_IsEnabled(t *testing.T) {
	l := NewLogger(WithLevel(LevelVerbose))

	if !l.IsEnabled(LevelBasic) {
		t.Error("LevelBasic should be enabled when level is LevelVerbose")
	}
	if !l.IsEnabled(LevelVerbose) {
		t.Error("LevelVerbose should be enabled when level is LevelVerbose")
	}
	if l.IsEnabled(LevelTrace) {
		t.Error("LevelTrace should not be enabled when level is LevelVerbose")
	}
}

func TestLogger_Log_Filtering(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(
		WithLevel(LevelBasic),
		WithOutput(&buf),
	)

	// This should be logged (LevelBasic)
	l.Log(&Event{
		Type:    EventTypeConnect,
		Message: "Connected",
		Level:   LevelBasic,
	})

	// This should not be logged (LevelVerbose)
	l.Log(&Event{
		Type:    EventTypeCommand,
		Message: "Command",
		Level:   LevelVerbose,
	})

	output := buf.String()
	if !strings.Contains(output, "Connected") {
		t.Error("Basic level event should be logged")
	}
	if strings.Contains(output, "Command") {
		t.Error("Verbose level event should not be logged at basic level")
	}
}

func TestLogger_FormatText(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(
		WithLevel(LevelTrace),
		WithOutput(&buf),
		WithFormat(FormatText),
		WithProtocol(ProtocolSSH),
		WithDeviceID("router-1"),
	)

	l.Log(&Event{
		Type:      EventTypeSend,
		Direction: DirectionSend,
		Message:   "Sending command",
		Data:      []byte("ls -la"),
		Duration:  100 * time.Millisecond,
		Level:     LevelVerbose,
	})

	output := buf.String()

	expectedParts := []string{
		"[DEBUG]",
		"[ssh:router-1]",
		">>>",
		"[send]",
		"Sending command",
		"100.000ms",
		"ls -la",
	}

	for _, part := range expectedParts {
		if !strings.Contains(output, part) {
			t.Errorf("Output missing: %s\nGot: %s", part, output)
		}
	}
}

func TestLogger_FormatJSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(
		WithLevel(LevelTrace),
		WithOutput(&buf),
		WithFormat(FormatJSON),
		WithProtocol(ProtocolREST),
		WithDeviceID("api-server"),
	)

	l.Log(&Event{
		Type:    EventTypeCommand,
		Message: "GET /api/status",
		Level:   LevelVerbose,
	})

	output := buf.String()

	// Should be valid JSON
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(output), &event); err != nil {
		t.Errorf("Output is not valid JSON: %v\n%s", err, output)
	}

	if event["protocol"] != "rest" {
		t.Errorf("Protocol = %v, want rest", event["protocol"])
	}
	if event["device_id"] != "api-server" {
		t.Errorf("DeviceID = %v, want api-server", event["device_id"])
	}
	if event["message"] != "GET /api/status" {
		t.Errorf("Message = %v", event["message"])
	}
}

func TestLogger_FormatHex(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(
		WithLevel(LevelTrace),
		WithOutput(&buf),
		WithFormat(FormatHex),
	)

	// Binary data
	l.Log(&Event{
		Type:    EventTypeSend,
		Message: "Binary packet",
		Data:    []byte{0x00, 0x01, 0x02, 0x03, 0xFF},
		Level:   LevelVerbose,
	})

	output := buf.String()

	// Should contain hex dump
	if !strings.Contains(output, "00000000") {
		t.Error("Should contain hex offset")
	}
	if !strings.Contains(output, "00 01 02 03 ff") {
		t.Error("Should contain hex bytes")
	}
}

func TestLogger_Connect(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelBasic), WithOutput(&buf))

	l.Connect("Connected to 192.168.1.1:22")

	output := buf.String()
	if !strings.Contains(output, "Connected") {
		t.Error("Connect message not logged")
	}
	if !strings.Contains(output, "[connect]") {
		t.Error("Event type not shown")
	}
}

func TestLogger_Disconnect(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelBasic), WithOutput(&buf))

	l.Disconnect("Connection closed")

	if !strings.Contains(buf.String(), "Connection closed") {
		t.Error("Disconnect message not logged")
	}
}

func TestLogger_Authenticate(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelBasic), WithOutput(&buf))

	l.Authenticate("publickey", true)
	output := buf.String()
	if !strings.Contains(output, "publickey") || !strings.Contains(output, "success") {
		t.Errorf("Success authentication not shown correctly: %s", output)
	}

	buf.Reset()
	l.Authenticate("keyboard-interactive", false)
	output = buf.String()
	if !strings.Contains(output, "keyboard-interactive") || !strings.Contains(output, "failed") {
		t.Errorf("Failed authentication not shown correctly: %s", output)
	}
}

func TestLogger_Command(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelVerbose), WithOutput(&buf))

	l.Command("show running-config")

	output := buf.String()
	if !strings.Contains(output, "show running-config") {
		t.Error("Command not logged")
	}
	if !strings.Contains(output, "[command]") {
		t.Error("Command type not shown")
	}
}

func TestLogger_Response(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelVerbose), WithOutput(&buf))

	l.Response("OK", 0, 50*time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "exit 0") {
		t.Error("Exit code not shown")
	}
	if !strings.Contains(output, "50.000ms") {
		t.Error("Duration not shown")
	}
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelBasic), WithOutput(&buf))

	l.Error("Connection failed", errors.New("timeout"))

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Error("Error indicator not shown")
	}
	if !strings.Contains(output, "Connection failed") {
		t.Error("Error message not shown")
	}
	if !strings.Contains(output, "timeout") {
		t.Error("Error details not shown")
	}
}

func TestLogger_Warning(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelBasic), WithOutput(&buf))

	l.Warning("Connection unstable")

	output := buf.String()
	if !strings.Contains(output, "[WARN]") {
		t.Error("Warning indicator not shown")
	}
}

func TestLogger_Trace(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(WithLevel(LevelTrace), WithOutput(&buf))

	l.Trace(DirectionSend, []byte{0x01, 0x02, 0x03})

	output := buf.String()
	if !strings.Contains(output, "raw data") {
		t.Error("Trace message not shown")
	}
}

func TestLogger_Events(t *testing.T) {
	l := NewLogger(WithLevel(LevelVerbose))

	l.Connect("Connected")
	l.Command("ls")
	l.Disconnect("Disconnected")

	events := l.Events()
	if len(events) != 3 {
		t.Errorf("Events count = %d, want 3", len(events))
	}
}

func TestLogger_Clear(t *testing.T) {
	l := NewLogger(WithLevel(LevelVerbose))

	l.Connect("Connected")
	l.Command("ls")

	l.Clear()

	events := l.Events()
	if len(events) != 0 {
		t.Errorf("Events should be empty after Clear, got %d", len(events))
	}
}

func TestLogger_MaxEvents(t *testing.T) {
	l := NewLogger(WithLevel(LevelVerbose), WithMaxEvents(3))

	for i := 0; i < 10; i++ {
		l.Info("Event")
	}

	events := l.Events()
	if len(events) != 3 {
		t.Errorf("Events count = %d, want 3 (max)", len(events))
	}
}

func TestLogger_Callback(t *testing.T) {
	callbackCalled := false
	var capturedEvent *Event

	l := NewLogger(
		WithLevel(LevelBasic),
		WithCallback(func(e *Event) {
			callbackCalled = true
			capturedEvent = e
		}),
	)

	l.Connect("Test connection")

	if !callbackCalled {
		t.Error("Callback was not called")
	}
	if capturedEvent == nil || capturedEvent.Type != EventTypeConnect {
		t.Error("Callback did not receive correct event")
	}
}

func TestNewRedactor(t *testing.T) {
	r := NewRedactor()

	if len(r.patterns) == 0 {
		t.Error("Redactor should have default patterns")
	}
}

func TestRedactor_Redact(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		input    string
		contains string
		excludes string
	}{
		{"password=secret123", "***REDACTED***", "secret123"},
		{"token=abc123", "***REDACTED***", "abc123"},
		{"Authorization: Bearer xyz", "***REDACTED***", "Bearer xyz"},
		{"community=public", "***REDACTED***", "public"},
		{"safe data", "safe data", ""},
	}

	for _, tt := range tests {
		result := r.RedactString(tt.input)

		if tt.contains != "" && !strings.Contains(result, tt.contains) {
			t.Errorf("Redacted result should contain %q: %s", tt.contains, result)
		}
		if tt.excludes != "" && strings.Contains(result, tt.excludes) {
			t.Errorf("Redacted result should not contain %q: %s", tt.excludes, result)
		}
	}
}

func TestRedactor_AddPattern(t *testing.T) {
	r := NewRedactor()

	err := r.AddPattern(`secret_\d+`, "***SECRET***", "custom secrets")
	if err != nil {
		t.Errorf("AddPattern failed: %v", err)
	}

	result := r.RedactString("Found secret_12345 in log")
	if strings.Contains(result, "secret_12345") {
		t.Error("Custom pattern not redacted")
	}
	if !strings.Contains(result, "***SECRET***") {
		t.Error("Replacement not applied")
	}
}

func TestRedactor_AddPattern_Invalid(t *testing.T) {
	r := NewRedactor()

	err := r.AddPattern(`[invalid`, "replacement", "test")
	if err == nil {
		t.Error("Should return error for invalid regex")
	}
}

func TestRedactor_Redact_Bytes(t *testing.T) {
	r := NewRedactor()

	input := []byte("password=secret123")
	result := r.Redact(input)

	if strings.Contains(string(result), "secret123") {
		t.Error("Bytes not redacted")
	}
}

func TestIsPrintable(t *testing.T) {
	tests := []struct {
		data     []byte
		expected bool
	}{
		{[]byte("hello world"), true},
		{[]byte("line1\nline2\r\n"), true},
		{[]byte{0x00, 0x01, 0x02}, false},
		{[]byte{}, true},
	}

	for _, tt := range tests {
		if got := isPrintable(tt.data); got != tt.expected {
			t.Errorf("isPrintable(%v) = %v, want %v", tt.data, got, tt.expected)
		}
	}
}

func TestHexDump(t *testing.T) {
	data := []byte("Hello, World!")
	dump := hexDump(data)

	// Should contain offset
	if !strings.Contains(dump, "00000000") {
		t.Error("Missing offset")
	}

	// Should contain hex representation
	if !strings.Contains(dump, "48 65 6c 6c 6f") { // "Hello"
		t.Error("Missing hex bytes")
	}

	// Should contain ASCII
	if !strings.Contains(dump, "|Hello") {
		t.Error("Missing ASCII representation")
	}
}

func TestNewSessionLogger(t *testing.T) {
	base := NewLogger(WithLevel(LevelVerbose))
	session := NewSessionLogger(base, "session-1", "device-1", ProtocolSSH)

	if session.sessionID != "session-1" {
		t.Errorf("SessionID = %s, want session-1", session.sessionID)
	}
	if session.deviceID != "device-1" {
		t.Errorf("DeviceID = %s, want device-1", session.deviceID)
	}
	if session.protocol != ProtocolSSH {
		t.Errorf("Protocol = %s, want ssh", session.protocol)
	}
}

func TestSessionLogger_Log(t *testing.T) {
	base := NewLogger(WithLevel(LevelVerbose))
	session := NewSessionLogger(base, "session-1", "device-1", ProtocolSSH)

	session.Log(&Event{
		Type:    EventTypeCommand,
		Message: "test",
		Level:   LevelVerbose,
	})

	events := base.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.SessionID != "session-1" {
		t.Errorf("Event SessionID = %s, want session-1", event.SessionID)
	}
	if event.DeviceID != "device-1" {
		t.Errorf("Event DeviceID = %s, want device-1", event.DeviceID)
	}
	if event.Protocol != ProtocolSSH {
		t.Errorf("Event Protocol = %s, want ssh", event.Protocol)
	}
}

func TestSessionLogger_Duration(t *testing.T) {
	base := NewLogger(WithLevel(LevelVerbose))
	session := NewSessionLogger(base, "session-1", "device-1", ProtocolSSH)

	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return session.Duration() >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expected duration to advance: %v", err)
	}

	duration := session.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("Duration = %v, should be at least 10ms", duration)
	}
}

func TestSessionLogger_Summary(t *testing.T) {
	base := NewLogger(WithLevel(LevelVerbose))
	session := NewSessionLogger(base, "session-1", "device-1", ProtocolSSH)

	session.Log(&Event{Type: EventTypeCommand, Message: "cmd1", Level: LevelVerbose})
	session.Log(&Event{Type: EventTypeCommand, Message: "cmd2", Level: LevelVerbose})
	session.Log(&Event{Type: EventTypeError, Message: "error", Level: LevelBasic})
	session.Log(&Event{Type: EventTypeSend, DataLen: 100, Level: LevelVerbose})
	session.Log(&Event{Type: EventTypeReceive, DataLen: 200, Level: LevelVerbose})

	summary := session.Summary()

	if summary.SessionID != "session-1" {
		t.Errorf("SessionID = %s", summary.SessionID)
	}
	if summary.EventCount != 5 {
		t.Errorf("EventCount = %d, want 5", summary.EventCount)
	}
	if summary.CommandCount != 2 {
		t.Errorf("CommandCount = %d, want 2", summary.CommandCount)
	}
	if summary.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", summary.ErrorCount)
	}
	if summary.BytesSent != 100 {
		t.Errorf("BytesSent = %d, want 100", summary.BytesSent)
	}
	if summary.BytesReceived != 200 {
		t.Errorf("BytesReceived = %d, want 200", summary.BytesReceived)
	}
}

func TestLogger_Redaction_InEvents(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(
		WithLevel(LevelVerbose),
		WithOutput(&buf),
	)

	l.Send("Sending credentials", []byte("password=secret123"))

	events := l.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	// Data should be redacted
	if strings.Contains(string(events[0].Data), "secret123") {
		t.Error("Sensitive data should be redacted")
	}
}
