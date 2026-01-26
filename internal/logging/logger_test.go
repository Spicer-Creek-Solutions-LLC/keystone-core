package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// mockOutput captures log output for testing
type mockOutput struct {
	data [][]byte
}

func (m *mockOutput) Write(data []byte) error {
	// Make a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.data = append(m.data, dataCopy)
	return nil
}

func (m *mockOutput) Close() error {
	return nil
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{Level(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
		ok       bool
	}{
		{"debug", LevelDebug, true},
		{"info", LevelInfo, true},
		{"warn", LevelWarn, true},
		{"error", LevelError, true},
		{"unknown", LevelInfo, false},
	}

	for _, tt := range tests {
		level, ok := ParseLevel(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseLevel(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && level != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, level, tt.expected)
		}
	}
}

func TestJSONFormatter(t *testing.T) {
	formatter := &JSONFormatter{}

	entry := &Entry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "test message",
		Fields: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// Parse JSON
	var output map[string]interface{}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fields
	if output["level"] != "info" {
		t.Errorf("level = %v, want info", output["level"])
	}
	if output["logger"] != "test" {
		t.Errorf("logger = %v, want test", output["logger"])
	}
	if output["message"] != "test message" {
		t.Errorf("message = %v, want test message", output["message"])
	}
	if output["key1"] != "value1" {
		t.Errorf("key1 = %v, want value1", output["key1"])
	}
}

func TestJSONFormatterWithCorrelationID(t *testing.T) {
	formatter := &JSONFormatter{}

	entry := &Entry{
		Timestamp:     time.Now(),
		Level:         LevelInfo,
		Logger:        "test",
		Message:       "test",
		CorrelationID: "test-123",
		Fields:        make(map[string]interface{}),
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if output["correlation_id"] != "test-123" {
		t.Errorf("correlation_id = %v, want test-123", output["correlation_id"])
	}
}

func TestLogfmtFormatter(t *testing.T) {
	formatter := &LogfmtFormatter{}

	entry := &Entry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "test message",
		Fields: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "level=info") {
		t.Errorf("output missing level=info: %s", output)
	}
	if !strings.Contains(output, "logger=test") {
		t.Errorf("output missing logger=test: %s", output)
	}
	if !strings.Contains(output, "message=\"test message\"") {
		t.Errorf("output missing message: %s", output)
	}
}

func TestTextFormatter(t *testing.T) {
	formatter := &TextFormatter{DisableColors: true}

	entry := &Entry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "test message",
		Fields:    make(map[string]interface{}),
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "INFO") {
		t.Errorf("output missing INFO: %s", output)
	}
	if !strings.Contains(output, "[test]") {
		t.Errorf("output missing [test]: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("output missing message: %s", output)
	}
}

func TestCorrelationID(t *testing.T) {
	// Test generation
	id1 := GenerateCorrelationID()
	id2 := GenerateCorrelationID()

	if id1 == "" {
		t.Error("GenerateCorrelationID returned empty string")
	}
	if id1 == id2 {
		t.Error("GenerateCorrelationID returned duplicate IDs")
	}

	// Test context
	ctx := context.Background()
	ctx = ContextWithCorrelationID(ctx, "test-id")

	id, ok := CorrelationIDFromContext(ctx)
	if !ok {
		t.Error("Failed to get correlation ID from context")
	}
	if id != "test-id" {
		t.Errorf("correlation ID = %v, want test-id", id)
	}
}

func TestEnsureCorrelationID(t *testing.T) {
	// Test with existing ID
	ctx := ContextWithCorrelationID(context.Background(), "existing-id")
	newCtx, id := EnsureCorrelationID(ctx)

	if id != "existing-id" {
		t.Errorf("EnsureCorrelationID changed existing ID: got %v, want existing-id", id)
	}
	if newCtx != ctx {
		t.Error("EnsureCorrelationID created new context when ID existed")
	}

	// Test without ID
	ctx = context.Background()
	newCtx, id = EnsureCorrelationID(ctx)

	if id == "" {
		t.Error("EnsureCorrelationID did not generate ID")
	}

	retrievedID, ok := CorrelationIDFromContext(newCtx)
	if !ok || retrievedID != id {
		t.Error("EnsureCorrelationID did not add ID to context")
	}
}

func TestStructuredLogger(t *testing.T) {
	output := &mockOutput{}

	logger := NewLogger(Config{
		Name:      "test",
		Level:     LevelDebug,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{output},
	})

	logger.Info("test message", String("key", "value"))

	if len(output.data) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(output.data))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(output.data[0], &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry["message"] != "test message" {
		t.Errorf("message = %v, want test message", entry["message"])
	}
	if entry["key"] != "value" {
		t.Errorf("key = %v, want value", entry["key"])
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	output := &mockOutput{}

	logger := NewLogger(Config{
		Name:      "test",
		Level:     LevelWarn,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{output},
	})

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	if len(output.data) != 2 {
		t.Fatalf("Expected 2 log entries (warn, error), got %d", len(output.data))
	}
}

func TestLoggerWithFields(t *testing.T) {
	output := &mockOutput{}

	logger := NewLogger(Config{
		Name:      "test",
		Level:     LevelInfo,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{output},
	})

	childLogger := logger.WithFields(String("base", "field"))
	childLogger.Info("test", String("extra", "field"))

	if len(output.data) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(output.data))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(output.data[0], &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry["base"] != "field" {
		t.Errorf("base = %v, want field", entry["base"])
	}
	if entry["extra"] != "field" {
		t.Errorf("extra = %v, want field", entry["extra"])
	}
}

func TestLoggerWithCorrelationID(t *testing.T) {
	output := &mockOutput{}

	logger := NewLogger(Config{
		Name:      "test",
		Level:     LevelInfo,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{output},
	})

	childLogger := logger.WithCorrelationID("test-123")
	childLogger.Info("test")

	if len(output.data) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(output.data))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(output.data[0], &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry["correlation_id"] != "test-123" {
		t.Errorf("correlation_id = %v, want test-123", entry["correlation_id"])
	}
}

func TestLoggerWithContext(t *testing.T) {
	output := &mockOutput{}

	logger := NewLogger(Config{
		Name:      "test",
		Level:     LevelInfo,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{output},
	})

	ctx := ContextWithCorrelationID(context.Background(), "ctx-123")
	childLogger := logger.WithContext(ctx)
	childLogger.Info("test")

	if len(output.data) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(output.data))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(output.data[0], &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry["correlation_id"] != "ctx-123" {
		t.Errorf("correlation_id = %v, want ctx-123", entry["correlation_id"])
	}
}

func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name  string
		field Field
	}{
		{"String", String("key", "value")},
		{"Int", Int("key", 42)},
		{"Int64", Int64("key", int64(42))},
		{"Float64", Float64("key", 3.14)},
		{"Bool", Bool("key", true)},
		{"Duration", Duration("key", time.Second)},
		{"Time", Time("key", time.Now())},
		{"Any", Any("key", map[string]string{"nested": "value"})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Key != "key" {
				t.Errorf("Field.Key = %v, want key", tt.field.Key)
			}
			if tt.field.Value == nil {
				t.Error("Field.Value is nil")
			}
		})
	}
}

func TestErrorField(t *testing.T) {
	// Test with error
	field := Error(bytes.ErrTooLarge)
	if field.Key != "error" {
		t.Errorf("Field.Key = %v, want error", field.Key)
	}
	if field.Value != "bytes.Buffer: too large" {
		t.Errorf("Field.Value = %v, want error message", field.Value)
	}

	// Test with nil
	field = Error(nil)
	if field.Value != nil {
		t.Errorf("Field.Value = %v, want nil", field.Value)
	}
}

func TestFields(t *testing.T) {
	fields := Fields("key1", "value1", "key2", 42)

	if len(fields) != 2 {
		t.Fatalf("Expected 2 fields, got %d", len(fields))
	}

	if fields[0].Key != "key1" || fields[0].Value != "value1" {
		t.Errorf("Field 0 = %v:%v, want key1:value1", fields[0].Key, fields[0].Value)
	}
	if fields[1].Key != "key2" || fields[1].Value != 42 {
		t.Errorf("Field 1 = %v:%v, want key2:42", fields[1].Key, fields[1].Value)
	}
}

func TestFieldsOddNumber(t *testing.T) {
	// Odd number of arguments should append nil
	fields := Fields("key1", "value1", "key2")

	if len(fields) != 2 {
		t.Fatalf("Expected 2 fields, got %d", len(fields))
	}

	if fields[1].Value != nil {
		t.Errorf("Field 1 value = %v, want nil", fields[1].Value)
	}
}

func TestWriterOutput(t *testing.T) {
	var buf bytes.Buffer
	output := NewWriterOutput(&buf)

	data := []byte("test data\n")
	if err := output.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.String() != string(data) {
		t.Errorf("output = %q, want %q", buf.String(), string(data))
	}

	if err := output.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestDefaultLogger(t *testing.T) {
	// Test default logger operations don't panic
	Default().Info("test")
	Debug("test")
	Info("test")
	Warn("test")
	ErrorLog("test")
	WithFields(String("key", "value")).Info("test")
	WithCorrelationID("test").Info("test")
	WithContext(context.Background()).Info("test")
}

func TestSetGetLevel(t *testing.T) {
	logger := NewDefaultLogger("test")

	logger.SetLevel(LevelWarn)
	if logger.GetLevel() != LevelWarn {
		t.Errorf("GetLevel() = %v, want LevelWarn", logger.GetLevel())
	}
}
