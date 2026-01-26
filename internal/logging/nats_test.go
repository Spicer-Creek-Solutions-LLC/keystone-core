package logging

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultNATSOutputConfig(t *testing.T) {
	config := DefaultNATSOutputConfig()

	if config.URL != "nats://localhost:4222" {
		t.Errorf("Expected URL nats://localhost:4222, got %s", config.URL)
	}

	if config.Subject != "kscore.logs" {
		t.Errorf("Expected Subject kscore.logs, got %s", config.Subject)
	}

	if config.BufferSize != 1000 {
		t.Errorf("Expected BufferSize 1000, got %d", config.BufferSize)
	}

	if config.FlushInterval != 100*time.Millisecond {
		t.Errorf("Expected FlushInterval 100ms, got %v", config.FlushInterval)
	}

	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("Expected ConnectTimeout 5s, got %v", config.ConnectTimeout)
	}

	if config.ReconnectWait != 1*time.Second {
		t.Errorf("Expected ReconnectWait 1s, got %v", config.ReconnectWait)
	}

	if config.MaxReconnects != -1 {
		t.Errorf("Expected MaxReconnects -1 (unlimited), got %d", config.MaxReconnects)
	}

	if !config.Async {
		t.Error("Expected Async to be true by default")
	}

	if config.SubjectPerLevel {
		t.Error("Expected SubjectPerLevel to be false by default")
	}

	if config.SubjectPerService {
		t.Error("Expected SubjectPerService to be false by default")
	}
}

func TestNATSLogMessage(t *testing.T) {
	msg := NATSLogMessage{
		Timestamp:     "2024-01-15T10:30:00Z",
		Level:         "info",
		Logger:        "test-logger",
		Message:       "Test message",
		CorrelationID: "corr-123",
		TraceID:       "trace-456",
		SpanID:        "span-789",
		Caller:        "test.go:42",
		Host:          "localhost",
		Service:       "kscore-server",
		Version:       "1.0.0",
		PID:           12345,
		Fields: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NATSLogMessage: %v", err)
	}

	var decoded NATSLogMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSLogMessage: %v", err)
	}

	if decoded.Timestamp != msg.Timestamp {
		t.Errorf("Expected Timestamp %s, got %s", msg.Timestamp, decoded.Timestamp)
	}

	if decoded.Level != msg.Level {
		t.Errorf("Expected Level %s, got %s", msg.Level, decoded.Level)
	}

	if decoded.Logger != msg.Logger {
		t.Errorf("Expected Logger %s, got %s", msg.Logger, decoded.Logger)
	}

	if decoded.Message != msg.Message {
		t.Errorf("Expected Message %s, got %s", msg.Message, decoded.Message)
	}

	if decoded.CorrelationID != msg.CorrelationID {
		t.Errorf("Expected CorrelationID %s, got %s", msg.CorrelationID, decoded.CorrelationID)
	}

	if decoded.Service != msg.Service {
		t.Errorf("Expected Service %s, got %s", msg.Service, decoded.Service)
	}

	if decoded.Host != msg.Host {
		t.Errorf("Expected Host %s, got %s", msg.Host, decoded.Host)
	}

	if decoded.PID != msg.PID {
		t.Errorf("Expected PID %d, got %d", msg.PID, decoded.PID)
	}
}

func TestNATSLogMessageOmitEmpty(t *testing.T) {
	msg := NATSLogMessage{
		Timestamp: "2024-01-15T10:30:00Z",
		Level:     "info",
		Logger:    "test",
		Message:   "Test",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NATSLogMessage: %v", err)
	}

	// Check that empty optional fields are omitted
	dataStr := string(data)
	if contains(dataStr, "correlation_id") {
		t.Error("Expected correlation_id to be omitted when empty")
	}
	if contains(dataStr, "trace_id") {
		t.Error("Expected trace_id to be omitted when empty")
	}
	if contains(dataStr, "span_id") {
		t.Error("Expected span_id to be omitted when empty")
	}
	if contains(dataStr, "fields") {
		t.Error("Expected fields to be omitted when nil")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNATSFormatterFormat(t *testing.T) {
	formatter := &NATSFormatter{
		ServiceName: "test-service",
		IncludeRaw:  false,
	}

	now := time.Now()
	entry := &Entry{
		Timestamp:     now,
		Level:         LevelInfo,
		Logger:        "test-logger",
		Message:       "Test message",
		CorrelationID: "corr-123",
		Fields: map[string]interface{}{
			"key": "value",
		},
		Metadata: &EntryMetadata{
			Host:    "localhost",
			PID:     12345,
			Version: "1.0.0",
			Service: "other-service",
			Caller:  "test.go:42",
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var msg NATSLogMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal formatted message: %v", err)
	}

	// Check fields
	if msg.Level != "info" {
		t.Errorf("Expected Level 'info', got %s", msg.Level)
	}

	if msg.Logger != "test-logger" {
		t.Errorf("Expected Logger 'test-logger', got %s", msg.Logger)
	}

	if msg.Message != "Test message" {
		t.Errorf("Expected Message 'Test message', got %s", msg.Message)
	}

	if msg.CorrelationID != "corr-123" {
		t.Errorf("Expected CorrelationID 'corr-123', got %s", msg.CorrelationID)
	}

	// Service should come from formatter, not metadata
	if msg.Service != "test-service" {
		t.Errorf("Expected Service 'test-service', got %s", msg.Service)
	}

	if msg.Host != "localhost" {
		t.Errorf("Expected Host 'localhost', got %s", msg.Host)
	}

	if msg.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", msg.PID)
	}

	if msg.Caller != "test.go:42" {
		t.Errorf("Expected Caller 'test.go:42', got %s", msg.Caller)
	}

	if msg.Fields["key"] != "value" {
		t.Errorf("Expected Fields['key'] = 'value', got %v", msg.Fields["key"])
	}

	if msg.Raw != "" {
		t.Error("Expected Raw to be empty when IncludeRaw is false")
	}
}

func TestNATSFormatterWithRaw(t *testing.T) {
	formatter := &NATSFormatter{
		ServiceName: "test-service",
		IncludeRaw:  true,
	}

	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelError,
		Logger:    "test",
		Message:   "Error occurred",
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var msg NATSLogMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal formatted message: %v", err)
	}

	if msg.Raw == "" {
		t.Error("Expected Raw to be populated when IncludeRaw is true")
	}

	// Raw should be valid JSON
	var rawEntry map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Raw), &rawEntry); err != nil {
		t.Errorf("Raw field should be valid JSON: %v", err)
	}
}

func TestNATSFormatterWithNoMetadata(t *testing.T) {
	formatter := &NATSFormatter{
		ServiceName: "test-service",
	}

	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "Test",
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var msg NATSLogMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal formatted message: %v", err)
	}

	// Service should still be set from formatter
	if msg.Service != "test-service" {
		t.Errorf("Expected Service 'test-service', got %s", msg.Service)
	}

	// Host, PID, Caller should be empty
	if msg.Host != "" {
		t.Errorf("Expected Host to be empty, got %s", msg.Host)
	}

	if msg.Caller != "" {
		t.Errorf("Expected Caller to be empty, got %s", msg.Caller)
	}
}

func TestNATSFormatterServiceFromMetadata(t *testing.T) {
	formatter := &NATSFormatter{
		ServiceName: "", // Empty service name
	}

	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "Test",
		Metadata: &EntryMetadata{
			Service: "metadata-service",
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var msg NATSLogMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal formatted message: %v", err)
	}

	// Service should come from metadata when formatter has empty service
	if msg.Service != "metadata-service" {
		t.Errorf("Expected Service 'metadata-service', got %s", msg.Service)
	}
}

func TestNATSOutputBuildSubject(t *testing.T) {
	tests := []struct {
		name     string
		config   *NATSOutputConfig
		entry    map[string]interface{}
		expected string
	}{
		{
			name: "base subject only",
			config: &NATSOutputConfig{
				Subject:           "kscore.logs",
				SubjectPerLevel:   false,
				SubjectPerService: false,
			},
			entry:    map[string]interface{}{},
			expected: "kscore.logs",
		},
		{
			name: "with service",
			config: &NATSOutputConfig{
				Subject:           "kscore.logs",
				SubjectPerLevel:   false,
				SubjectPerService: true,
				ServiceName:       "kscore-server",
			},
			entry:    map[string]interface{}{},
			expected: "kscore.logs.kscore-server",
		},
		{
			name: "with level",
			config: &NATSOutputConfig{
				Subject:           "kscore.logs",
				SubjectPerLevel:   true,
				SubjectPerService: false,
			},
			entry: map[string]interface{}{
				"level": "error",
			},
			expected: "kscore.logs.error",
		},
		{
			name: "with service and level",
			config: &NATSOutputConfig{
				Subject:           "kscore.logs",
				SubjectPerLevel:   true,
				SubjectPerService: true,
				ServiceName:       "kscore-agent",
			},
			entry: map[string]interface{}{
				"level": "warn",
			},
			expected: "kscore.logs.kscore-agent.warn",
		},
		{
			name: "empty service name",
			config: &NATSOutputConfig{
				Subject:           "kscore.logs",
				SubjectPerLevel:   false,
				SubjectPerService: true,
				ServiceName:       "",
			},
			entry:    map[string]interface{}{},
			expected: "kscore.logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &NATSOutput{
				config: tt.config,
			}
			result := output.buildSubject(tt.entry)
			if result != tt.expected {
				t.Errorf("buildSubject() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestNATSOutputStats(t *testing.T) {
	output := &NATSOutput{
		messagesSent:    100,
		messagesDropped: 5,
		lastError:       nil,
		lastErrorTime:   time.Time{},
	}

	sent, dropped, lastErr, lastErrTime := output.Stats()

	if sent != 100 {
		t.Errorf("Expected sent 100, got %d", sent)
	}

	if dropped != 5 {
		t.Errorf("Expected dropped 5, got %d", dropped)
	}

	if lastErr != nil {
		t.Errorf("Expected lastErr nil, got %v", lastErr)
	}

	if !lastErrTime.IsZero() {
		t.Errorf("Expected lastErrTime to be zero, got %v", lastErrTime)
	}
}

func TestNATSOutputIsConnectedNil(t *testing.T) {
	output := &NATSOutput{
		conn: nil,
	}

	if output.IsConnected() {
		t.Error("Expected IsConnected() to return false when conn is nil")
	}
}

func TestNATSOutputWriteClosed(t *testing.T) {
	output := &NATSOutput{
		closed: true,
		buffer: make(chan []byte, 10),
	}

	err := output.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error when writing to closed output")
	}
}

func TestNATSOutputWriteAsyncBufferFull(t *testing.T) {
	output := &NATSOutput{
		config: &NATSOutputConfig{
			Async: true,
		},
		closed: false,
		buffer: make(chan []byte, 1), // Small buffer
	}

	// Fill the buffer
	output.buffer <- []byte("first")

	// Try to write when buffer is full
	err := output.Write([]byte("second"))
	if err == nil {
		t.Error("Expected error when buffer is full")
	}

	if output.messagesDropped != 1 {
		t.Errorf("Expected messagesDropped 1, got %d", output.messagesDropped)
	}
}

func TestNATSOutputWriteAsync(t *testing.T) {
	output := &NATSOutput{
		config: &NATSOutputConfig{
			Async: true,
		},
		closed: false,
		buffer: make(chan []byte, 10),
	}

	err := output.Write([]byte("test message"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// Verify message was buffered
	select {
	case msg := <-output.buffer:
		if string(msg) != "test message" {
			t.Errorf("Expected 'test message', got '%s'", string(msg))
		}
	default:
		t.Error("Expected message in buffer")
	}
}

func TestNATSOutputPublishNotConnected(t *testing.T) {
	output := &NATSOutput{
		config: &NATSOutputConfig{
			Subject: "test",
		},
		conn: nil,
	}

	err := output.publish([]byte("test"))
	if err == nil {
		t.Error("Expected error when not connected")
	}
}

func TestNATSOutputCloseAlreadyClosed(t *testing.T) {
	output := &NATSOutput{
		closed: true,
	}

	err := output.Close()
	if err != nil {
		t.Errorf("Close() on already closed output should not error, got %v", err)
	}
}

// TestNATSSubscriberUnsubscribeNil tests unsubscribing when subscription is nil
func TestNATSSubscriberUnsubscribeNil(t *testing.T) {
	subscriber := &NATSSubscriber{
		sub: nil,
	}

	err := subscriber.Unsubscribe()
	if err != nil {
		t.Errorf("Unsubscribe() with nil subscription should not error, got %v", err)
	}
}

// TestNATSSubscriberClose tests the Close method
func TestNATSSubscriberClose(t *testing.T) {
	subscriber := &NATSSubscriber{
		sub: nil,
	}

	err := subscriber.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// TestNATSOutputConfigValidation tests configuration validation scenarios
func TestNATSOutputConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *NATSOutputConfig
		valid  bool
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			valid:  true,
		},
		{
			name: "valid config",
			config: &NATSOutputConfig{
				URL:       "nats://localhost:4222",
				Subject:   "logs",
				Async:     true,
			},
			valid: true,
		},
		{
			name: "with all auth options",
			config: &NATSOutputConfig{
				URL:       "nats://localhost:4222",
				Subject:   "logs",
				Token:     "secret-token",
				User:      "admin",
				Password:  "password",
				NKeyFile:  "/path/to/nkey",
				CredFile:  "/path/to/creds",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test without a NATS server, but we can verify
			// that config defaults are applied correctly
			if tt.config == nil {
				config := DefaultNATSOutputConfig()
				if config.URL == "" || config.Subject == "" {
					t.Error("Default config should have URL and Subject set")
				}
			}
		})
	}
}

// TestAllLogLevels tests formatting for all log levels
func TestNATSFormatterAllLogLevels(t *testing.T) {
	formatter := &NATSFormatter{
		ServiceName: "test",
	}

	levels := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	levelStrings := []string{"debug", "info", "warn", "error"}

	for i, level := range levels {
		entry := &Entry{
			Timestamp: time.Now(),
			Level:     level,
			Logger:    "test",
			Message:   "Test",
		}

		data, err := formatter.Format(entry)
		if err != nil {
			t.Errorf("Format() error for level %s: %v", levelStrings[i], err)
			continue
		}

		var msg NATSLogMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Errorf("Failed to unmarshal for level %s: %v", levelStrings[i], err)
			continue
		}

		if msg.Level != levelStrings[i] {
			t.Errorf("Expected level %s, got %s", levelStrings[i], msg.Level)
		}
	}
}
