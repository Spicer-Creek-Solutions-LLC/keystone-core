package access

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestInMemoryAuditLogger_Log(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)

	ctx := context.Background()
	event := &AuditEvent{
		ID:        "test-1",
		Type:      AuditEventAccess,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:   "test-user",
			Type: "user",
		},
		Request: &AuditRequest{
			Namespace: "packages",
			Path:      "/nginx.deb",
			Action:    ActionGet,
		},
	}

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	if events[0].ID != "test-1" {
		t.Errorf("expected event ID 'test-1', got '%s'", events[0].ID)
	}
}

func TestInMemoryAuditLogger_MaxSize(t *testing.T) {
	logger := NewInMemoryAuditLogger(3)

	ctx := context.Background()

	// Add 4 events (exceeds max size of 3)
	for i := 1; i <= 4; i++ {
		logger.Log(ctx, &AuditEvent{
			ID:        "test-" + string(rune('0'+i)),
			Type:      AuditEventAccess,
			Timestamp: time.Now(),
		})
	}

	events := logger.GetEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events (max size), got %d", len(events))
	}

	// First event should have been removed
	if events[0].ID != "test-2" {
		t.Errorf("expected oldest event 'test-2', got '%s'", events[0].ID)
	}
}

func TestInMemoryAuditLogger_Query(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)

	ctx := context.Background()
	now := time.Now()

	// Add events
	logger.Log(ctx, &AuditEvent{
		ID:        "event-1",
		Type:      AuditEventDownload,
		Timestamp: now.Add(-time.Hour),
		Identity: &AuditIdentity{
			ID:   "user-1",
			Type: "user",
		},
		Request: &AuditRequest{
			Namespace: "packages",
			Path:      "/nginx.deb",
			Action:    ActionGet,
		},
		Response: &AuditResponse{
			Allowed: true,
		},
	})

	logger.Log(ctx, &AuditEvent{
		ID:        "event-2",
		Type:      AuditEventDenied,
		Timestamp: now.Add(-30 * time.Minute),
		Identity: &AuditIdentity{
			ID:   "user-2",
			Type: "agent",
		},
		Request: &AuditRequest{
			Namespace: "configs",
			Path:      "/secret.yaml",
			Action:    ActionGet,
		},
		Response: &AuditResponse{
			Allowed: false,
		},
	})

	logger.Log(ctx, &AuditEvent{
		ID:        "event-3",
		Type:      AuditEventUpload,
		Timestamp: now,
		Identity: &AuditIdentity{
			ID:   "user-1",
			Type: "user",
		},
		Request: &AuditRequest{
			Namespace: "packages",
			Path:      "/nginx-new.deb",
			Action:    ActionPut,
		},
		Response: &AuditResponse{
			Allowed: true,
		},
	})

	tests := []struct {
		name     string
		filter   *AuditFilter
		expected int
	}{
		{
			name:     "no filter",
			filter:   nil,
			expected: 3,
		},
		{
			name: "filter by identity",
			filter: &AuditFilter{
				IdentityID: "user-1",
			},
			expected: 2,
		},
		{
			name: "filter by identity type",
			filter: &AuditFilter{
				IdentityType: "agent",
			},
			expected: 1,
		},
		{
			name: "filter by namespace",
			filter: &AuditFilter{
				Namespace: "packages",
			},
			expected: 2,
		},
		{
			name: "filter by event type",
			filter: &AuditFilter{
				EventType: AuditEventDenied,
			},
			expected: 1,
		},
		{
			name: "filter by allowed",
			filter: &AuditFilter{
				Allowed: boolPtr(true),
			},
			expected: 2,
		},
		{
			name: "filter by time range",
			filter: &AuditFilter{
				StartTime: now.Add(-45 * time.Minute),
				EndTime:   now.Add(time.Minute),
			},
			expected: 2,
		},
		{
			name: "filter with limit",
			filter: &AuditFilter{
				Limit: 2,
			},
			expected: 2,
		},
		{
			name: "filter with offset",
			filter: &AuditFilter{
				Offset: 1,
			},
			expected: 2,
		},
		{
			name: "filter with offset and limit",
			filter: &AuditFilter{
				Offset: 1,
				Limit:  1,
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := logger.Query(ctx, tt.filter)
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			if len(results) != tt.expected {
				t.Errorf("Query() returned %d events, expected %d", len(results), tt.expected)
			}
		})
	}
}

func TestInMemoryAuditLogger_Clear(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)

	ctx := context.Background()
	logger.Log(ctx, &AuditEvent{ID: "test-1"})
	logger.Log(ctx, &AuditEvent{ID: "test-2"})

	logger.Clear()

	events := logger.GetEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events after clear, got %d", len(events))
	}
}

func TestJSONFileAuditLogger(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	logger, err := NewJSONFileAuditLogger(tmpPath)
	if err != nil {
		t.Fatalf("NewJSONFileAuditLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Log event
	event := &AuditEvent{
		ID:        "test-1",
		Type:      AuditEventAccess,
		Timestamp: time.Now(),
		Identity: &AuditIdentity{
			ID:   "test-user",
			Type: "user",
		},
	}

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Close and read file
	logger.Close()

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var logged AuditEvent
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Fatalf("failed to parse logged event: %v", err)
	}

	if logged.ID != "test-1" {
		t.Errorf("expected event ID 'test-1', got '%s'", logged.ID)
	}

	// Query should not be supported
	_, err = logger.Query(ctx, nil)
	if err == nil {
		t.Error("expected Query() to return error for file logger")
	}
}

func TestWriterAuditLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterAuditLogger(&buf)

	ctx := context.Background()

	event := &AuditEvent{
		ID:        "test-1",
		Type:      AuditEventAccess,
		Timestamp: time.Now(),
	}

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	var logged AuditEvent
	if err := json.Unmarshal(buf.Bytes(), &logged); err != nil {
		t.Fatalf("failed to parse logged event: %v", err)
	}

	if logged.ID != "test-1" {
		t.Errorf("expected event ID 'test-1', got '%s'", logged.ID)
	}
}

func TestMultiAuditLogger(t *testing.T) {
	logger1 := NewInMemoryAuditLogger(100)
	logger2 := NewInMemoryAuditLogger(100)

	multi := NewMultiAuditLogger(logger1, logger2)

	ctx := context.Background()

	event := &AuditEvent{
		ID:        "test-1",
		Type:      AuditEventAccess,
		Timestamp: time.Now(),
	}

	if err := multi.Log(ctx, event); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Both loggers should have the event
	if len(logger1.GetEvents()) != 1 {
		t.Error("expected logger1 to have 1 event")
	}

	if len(logger2.GetEvents()) != 1 {
		t.Error("expected logger2 to have 1 event")
	}

	// Query should work (uses first logger that supports it)
	results, err := multi.Query(ctx, nil)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestAuditRecorder_RecordAccess(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:    "test-user",
		Type:  "user",
		Roles: []string{"reader"},
	}

	req := &AccessRequest{
		Identity:  identity,
		Namespace: "packages",
		Path:      "/nginx.deb",
		Action:    ActionGet,
	}

	result := &AccessResult{
		Allowed:     true,
		Reason:      "allowed by ACL",
		MatchedRule: "allow-readers",
		Duration:    time.Millisecond,
	}

	if err := recorder.RecordAccess(ctx, identity, req, result); err != nil {
		t.Fatalf("RecordAccess() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventAccess {
		t.Errorf("expected event type %s, got %s", AuditEventAccess, event.Type)
	}

	if event.Identity.ID != "test-user" {
		t.Errorf("expected identity ID 'test-user', got '%s'", event.Identity.ID)
	}

	if event.Response.Allowed != true {
		t.Error("expected Response.Allowed to be true")
	}
}

func TestAuditRecorder_RecordDownload(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:   "test-user",
		Type: "user",
	}

	if err := recorder.RecordDownload(ctx, identity, "packages", "/nginx.deb", "local", 1024, time.Millisecond, nil); err != nil {
		t.Fatalf("RecordDownload() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventDownload {
		t.Errorf("expected event type %s, got %s", AuditEventDownload, event.Type)
	}

	if event.Response.BytesTransferred != 1024 {
		t.Errorf("expected BytesTransferred 1024, got %d", event.Response.BytesTransferred)
	}
}

func TestAuditRecorder_RecordUpload(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:   "test-user",
		Type: "user",
	}

	if err := recorder.RecordUpload(ctx, identity, "packages", "/nginx.deb", "local", 2048, time.Millisecond, nil); err != nil {
		t.Fatalf("RecordUpload() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventUpload {
		t.Errorf("expected event type %s, got %s", AuditEventUpload, event.Type)
	}

	if event.Response.StatusCode != 201 {
		t.Errorf("expected StatusCode 201, got %d", event.Response.StatusCode)
	}
}

func TestAuditRecorder_RecordDelete(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:   "test-user",
		Type: "user",
	}

	if err := recorder.RecordDelete(ctx, identity, "packages", "/nginx.deb", "local", time.Millisecond, nil); err != nil {
		t.Fatalf("RecordDelete() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventDelete {
		t.Errorf("expected event type %s, got %s", AuditEventDelete, event.Type)
	}

	if event.Response.StatusCode != 204 {
		t.Errorf("expected StatusCode 204, got %d", event.Response.StatusCode)
	}
}

func TestAuditRecorder_RecordList(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:   "test-user",
		Type: "user",
	}

	if err := recorder.RecordList(ctx, identity, "packages", "/", "local", 10, time.Millisecond, nil); err != nil {
		t.Fatalf("RecordList() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventList {
		t.Errorf("expected event type %s, got %s", AuditEventList, event.Type)
	}

	if event.Metadata["count"] != "10" {
		t.Errorf("expected count '10' in metadata, got '%s'", event.Metadata["count"])
	}
}

func TestAuditRecorder_RecordError(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	recorder := NewAuditRecorder(logger)

	ctx := context.Background()

	identity := &Identity{
		ID:   "test-user",
		Type: "user",
	}

	testErr := os.ErrNotExist

	if err := recorder.RecordDownload(ctx, identity, "packages", "/missing.deb", "local", 0, time.Millisecond, testErr); err != nil {
		t.Fatalf("RecordDownload() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != AuditEventError {
		t.Errorf("expected event type %s, got %s", AuditEventError, event.Type)
	}

	if event.Response.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", event.Response.StatusCode)
	}

	if event.Response.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"", "anything", true},
		{"*", "anything", true},
		{"/path/*", "/path/file", true},
		{"/path/*", "/path/sub/file", false},
		{"/path/file.txt", "/path/file.txt", true},
		{"/path/file.txt", "/path/other.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			if got := matchGlob(tt.pattern, tt.value); got != tt.expected {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.expected)
			}
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
