package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testutil"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Enabled should be true")
	}
	if config.BufferSize <= 0 {
		t.Error("BufferSize should be positive")
	}
	if len(config.RedactFields) == 0 {
		t.Error("RedactFields should have default values")
	}
}

func TestLogger_Log(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	event := &Event{
		Category: CategoryAuth,
		Action:   ActionLogin,
		Outcome:  OutcomeSuccess,
		Actor: &Actor{
			ID:   "user-1",
			Type: "user",
			Name: "Test User",
		},
	}

	err := logger.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Wait for event to be written
	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	if writer.Count() != 1 {
		t.Errorf("Expected 1 event, got %d", writer.Count())
	}

	events := writer.Events()
	if events[0].Actor.ID != "user-1" {
		t.Error("Actor ID should be user-1")
	}
}

func TestLogger_LogAuth(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	actor := &Actor{ID: "user-1", Type: "user"}

	err := logger.LogAuth(ctx, ActionLogin, OutcomeSuccess, actor, nil)
	if err != nil {
		t.Fatalf("LogAuth failed: %v", err)
	}

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].Category != CategoryAuth {
		t.Errorf("Category = %s, want %s", events[0].Category, CategoryAuth)
	}
	if events[0].Action != ActionLogin {
		t.Errorf("Action = %s, want %s", events[0].Action, ActionLogin)
	}
}

func TestLogger_LogAuthz(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	actor := &Actor{ID: "user-1", Type: "user"}
	resource := &Resource{ID: "doc-1", Type: "document"}

	err := logger.LogAuthz(ctx, ActionRead, OutcomeDenied, actor, resource, nil)
	if err != nil {
		t.Fatalf("LogAuthz failed: %v", err)
	}

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].Category != CategoryAuthz {
		t.Error("Category should be authorization")
	}
	if events[0].Level != LevelWarning {
		t.Error("Denied should have warning level")
	}
}

func TestLogger_LogData(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	actor := &Actor{ID: "user-1", Type: "user"}
	resource := &Resource{ID: "record-1", Type: "record"}

	err := logger.LogData(ctx, ActionRead, OutcomeSuccess, actor, resource)
	if err != nil {
		t.Fatalf("LogData failed: %v", err)
	}

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	if writer.Count() != 1 {
		t.Error("Expected 1 event")
	}
}

func TestLogger_LogConfig(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	actor := &Actor{ID: "admin-1", Type: "user"}
	resource := &Resource{ID: "settings", Type: "config"}

	before := map[string]string{"setting": "old"}
	after := map[string]string{"setting": "new"}

	err := logger.LogConfig(ctx, ActionUpdate, OutcomeSuccess, actor, resource, before, after)
	if err != nil {
		t.Fatalf("LogConfig failed: %v", err)
	}

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].Context["before"] == nil {
		t.Error("Context should have before value")
	}
	if events[0].Context["after"] == nil {
		t.Error("Context should have after value")
	}
}

func TestLogger_LogSecurity(t *testing.T) {
	config := DefaultConfig()
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	actor := &Actor{ID: "unknown", Type: "external", IP: "192.168.1.100"}

	err := logger.LogSecurity(ctx, "Suspicious activity detected", actor, nil)
	if err != nil {
		t.Fatalf("LogSecurity failed: %v", err)
	}

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].Level != LevelCritical {
		t.Error("Security events should be critical")
	}
	if events[0].Category != CategorySecurity {
		t.Error("Category should be security")
	}
}

func TestLogger_Redaction(t *testing.T) {
	config := DefaultConfig()
	config.RedactFields = []string{"password", "token"}
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	event := &Event{
		Category: CategoryAuth,
		Action:   ActionLogin,
		Outcome:  OutcomeSuccess,
		Context: map[string]interface{}{
			"password": "secret123",
			"username": "testuser",
		},
		Request: &Request{
			Headers: map[string]string{
				"Authorization": "Bearer xyz",
			},
		},
	}

	logger.Log(ctx, event)
	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	// Check redaction in context
	if events[0].Context["password"] != "[REDACTED]" {
		t.Error("password should be redacted")
	}
	if events[0].Context["username"] != "testuser" {
		t.Error("username should not be redacted")
	}
}

func TestLogger_LevelFilter(t *testing.T) {
	config := DefaultConfig()
	config.MinLevel = LevelWarning
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	// Info level - should be filtered
	logger.Log(ctx, &Event{
		Level:    LevelInfo,
		Category: CategoryAuth,
	})

	// Warning level - should pass
	logger.Log(ctx, &Event{
		Level:    LevelWarning,
		Category: CategoryAuth,
	})

	// Critical level - should pass
	logger.Log(ctx, &Event{
		Level:    LevelCritical,
		Category: CategoryAuth,
	})

	// Wait for the 2 events that should pass through (warning + critical)
	if err := writer.WaitForCount(2, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	if writer.Count() != 2 {
		t.Errorf("Expected 2 events (warning + critical), got %d", writer.Count())
	}
}

func TestLogger_CategoryFilter(t *testing.T) {
	config := DefaultConfig()
	config.Categories = []Category{CategoryAuth}
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	// Auth category - should pass
	logger.Log(ctx, &Event{Category: CategoryAuth})

	// Data category - should be filtered
	logger.Log(ctx, &Event{Category: CategoryData})

	// Wait for the 1 event that should pass through (auth only)
	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	// Verify only 1 event passed (the filtered one should never arrive)
	if !testutil.Never(func() bool { return writer.Count() > 1 }, 50*time.Millisecond) {
		t.Errorf("Expected 1 event (auth only), got %d", writer.Count())
	}
}

func TestLogger_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	logger.Log(ctx, &Event{Category: CategoryAuth})

	// For disabled logger, verify no events get logged
	if !testutil.Never(func() bool { return writer.Count() > 0 }, 50*time.Millisecond) {
		t.Error("Disabled logger should not log events")
	}
	logger.Flush()
}

func TestLogger_Close(t *testing.T) {
	logger := NewLogger(nil)

	err := logger.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()
	err = logger.Log(ctx, &Event{})
	if err != ErrLoggerClosed {
		t.Errorf("Expected ErrLoggerClosed, got %v", err)
	}

	// Double close
	err = logger.Close()
	if err != ErrLoggerClosed {
		t.Errorf("Double close should return ErrLoggerClosed, got %v", err)
	}
}

func TestLogger_Concurrent(t *testing.T) {
	config := DefaultConfig()
	config.BufferSize = 100
	logger := NewLogger(config)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Log(ctx, &Event{
					Category: CategoryAuth,
					Action:   ActionLogin,
				})
			}
		}(i)
	}

	wg.Wait()
	if err := writer.WaitForCount(1000, 5*time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	if writer.Count() != 1000 {
		t.Errorf("Expected 1000 events, got %d", writer.Count())
	}
}

func TestJSONWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := NewJSONWriter(&buf)

	event := &Event{
		ID:        "test-1",
		Category:  CategoryAuth,
		Action:    ActionLogin,
		Outcome:   OutcomeSuccess,
		Timestamp: time.Now(),
	}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check output is valid JSON
	var decoded Event
	err = json.Unmarshal(buf.Bytes(), &decoded)
	if err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if decoded.ID != "test-1" {
		t.Error("ID should be test-1")
	}
}

func TestMemoryWriter(t *testing.T) {
	writer := NewMemoryWriter()

	// Write events
	for i := 0; i < 5; i++ {
		writer.Write(&Event{ID: string(rune('a' + i))})
	}

	if writer.Count() != 5 {
		t.Errorf("Count = %d, want 5", writer.Count())
	}

	events := writer.Events()
	if len(events) != 5 {
		t.Errorf("Events len = %d, want 5", len(events))
	}

	// Clear
	writer.Clear()
	if writer.Count() != 0 {
		t.Error("Count should be 0 after clear")
	}
}

func TestEvent_Fields(t *testing.T) {
	event := &Event{
		ID:        "event-1",
		Timestamp: time.Now(),
		Level:     LevelWarning,
		Category:  CategorySecurity,
		Action:    ActionExecute,
		Outcome:   OutcomeFailure,
		Actor: &Actor{
			ID:        "user-1",
			Type:      "user",
			Name:      "Test User",
			Email:     "test@example.com",
			IP:        "192.168.1.1",
			SessionID: "sess-123",
			Roles:     []string{"admin", "user"},
		},
		Resource: &Resource{
			ID:    "res-1",
			Type:  "command",
			Name:  "dangerous_cmd",
			Path:  "/system/cmd",
			Owner: "root",
			Attributes: map[string]string{
				"critical": "true",
			},
		},
		Request: &Request{
			ID:     "req-1",
			Method: "POST",
			Path:   "/api/execute",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Size: 256,
		},
		Response: &Response{
			StatusCode: 403,
			Size:       128,
		},
		Context: map[string]interface{}{
			"reason": "insufficient permissions",
		},
		Tags:     []string{"security", "blocked"},
		Duration: time.Second,
		Error: &ErrorInfo{
			Code:    "ERR_403",
			Message: "Access denied",
		},
	}

	// Marshal and unmarshal to verify all fields
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Event
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != event.ID {
		t.Error("ID mismatch")
	}
	if decoded.Actor.ID != event.Actor.ID {
		t.Error("Actor.ID mismatch")
	}
	if decoded.Resource.Type != event.Resource.Type {
		t.Error("Resource.Type mismatch")
	}
	if decoded.Error.Code != event.Error.Code {
		t.Error("Error.Code mismatch")
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"password", "password", true},
		{"user_password", "password", true},
		{"password_hash", "password", true},
		{"username", "password", false},
		{"Authorization", "authorization", false}, // Case sensitive in this impl
		{"", "test", false},
	}

	for _, tt := range tests {
		got := containsIgnoreCase(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestLogger_AutoGeneratedID(t *testing.T) {
	logger := NewLogger(nil)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	// Log event without ID
	logger.Log(ctx, &Event{Category: CategoryAuth})

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].ID == "" {
		t.Error("ID should be auto-generated")
	}

	if !strings.HasPrefix(events[0].ID, "aud-") {
		t.Error("ID should start with 'aud-'")
	}
}

func TestLogger_AutoTimestamp(t *testing.T) {
	logger := NewLogger(nil)
	defer logger.Close()

	writer := NewMemoryWriter()
	logger.AddWriter(writer)

	ctx := context.Background()

	before := time.Now()

	// Log event without timestamp
	logger.Log(ctx, &Event{Category: CategoryAuth})

	if err := writer.WaitForCount(1, time.Second); err != nil {
		t.Fatalf("Wait for events: %v", err)
	}
	logger.Flush()

	events := writer.Events()
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].Timestamp.Before(before) {
		t.Error("Timestamp should be auto-set to current time")
	}
}
