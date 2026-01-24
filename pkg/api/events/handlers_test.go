package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEventResponseStructure(t *testing.T) {
	resp := EventResponse{
		ID:            "event-123",
		Type:          "agent.registered",
		Source:        "agent/server1",
		Timestamp:     time.Now(),
		Severity:      "info",
		CorrelationID: "corr-456",
		Tags: map[string]string{
			"env":  "prod",
			"host": "server1",
		},
		Data: map[string]interface{}{
			"hostname":   "server1",
			"ip_address": "192.168.1.100",
		},
	}

	if resp.ID != "event-123" {
		t.Errorf("ID = %v", resp.ID)
	}
	if resp.Type != "agent.registered" {
		t.Errorf("Type = %v", resp.Type)
	}
	if resp.Severity != "info" {
		t.Errorf("Severity = %v", resp.Severity)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("Tags count = %d", len(resp.Tags))
	}
	if resp.Data["hostname"] != "server1" {
		t.Errorf("Data[hostname] = %v", resp.Data["hostname"])
	}
}

func TestEventListResponseStructure(t *testing.T) {
	resp := EventListResponse{
		Events: []EventResponse{
			{ID: "event-1", Type: "agent.registered"},
			{ID: "event-2", Type: "agent.heartbeat"},
		},
		Total:       100,
		Limit:       50,
		Offset:      0,
		RetrievedAt: time.Now(),
	}

	if resp.Total != 100 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.Limit != 50 {
		t.Errorf("Limit = %d", resp.Limit)
	}
	if len(resp.Events) != 2 {
		t.Errorf("Events count = %d", len(resp.Events))
	}
}

func TestCreateEventRequestStructure(t *testing.T) {
	req := CreateEventRequest{
		Type:          "custom.event",
		Source:        "api/test",
		Severity:      "warning",
		CorrelationID: "corr-123",
		Tags: map[string]string{
			"category": "test",
		},
		Data: map[string]interface{}{
			"message": "test event",
			"count":   42,
		},
	}

	if req.Type != "custom.event" {
		t.Errorf("Type = %v", req.Type)
	}
	if req.Source != "api/test" {
		t.Errorf("Source = %v", req.Source)
	}
	if req.Severity != "warning" {
		t.Errorf("Severity = %v", req.Severity)
	}
	if req.Data["count"] != 42 {
		t.Errorf("Data[count] = %v", req.Data["count"])
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "single tag",
			input: "env=prod",
			expected: map[string]string{
				"env": "prod",
			},
		},
		{
			name:  "multiple tags",
			input: "env=prod,region=us-east,tier=frontend",
			expected: map[string]string{
				"env":    "prod",
				"region": "us-east",
				"tier":   "frontend",
			},
		},
		{
			name:  "with spaces",
			input: " env = prod , region = us-east ",
			expected: map[string]string{
				"env":    "prod",
				"region": "us-east",
			},
		},
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "malformed entry",
			input:    "invalid,env=prod",
			expected: map[string]string{
				"env": "prod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTags(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseTags(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("parseTags(%q)[%q] = %q, want %q", tt.input, k, result[k], v)
				}
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"status": "ok",
		"count":  42,
	}

	writeJSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %v", w.Header().Get("Content-Type"))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result[status] = %v", result["status"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusNotFound, "event not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "event not found" {
		t.Errorf("result[error] = %v", result["error"])
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(nil, nil)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := NewHandler(nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	t.Run("events endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// DELETE should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE /api/v1/events status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("event by id endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/123", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/events/123 status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestEventResponseJSONMarshal(t *testing.T) {
	now := time.Now().UTC()
	resp := EventResponse{
		ID:        "test-event",
		Type:      "test.type",
		Source:    "test/source",
		Timestamp: now,
		Severity:  "info",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled EventResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID = %v, want %v", unmarshaled.ID, resp.ID)
	}
	if unmarshaled.Type != resp.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, resp.Type)
	}
}

func TestEventListResponseJSONMarshal(t *testing.T) {
	resp := EventListResponse{
		Events: []EventResponse{
			{ID: "event-1", Type: "type1"},
			{ID: "event-2", Type: "type2"},
		},
		Total:       2,
		Limit:       50,
		Offset:      0,
		RetrievedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled EventListResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Events) != 2 {
		t.Errorf("Events count = %d", len(unmarshaled.Events))
	}
	if unmarshaled.Total != 2 {
		t.Errorf("Total = %d", unmarshaled.Total)
	}
}

func TestCreateEventRequestJSONMarshal(t *testing.T) {
	req := CreateEventRequest{
		Type:     "custom.event",
		Source:   "test",
		Severity: "warning",
		Tags: map[string]string{
			"key": "value",
		},
		Data: map[string]interface{}{
			"field": "data",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled CreateEventRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != req.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, req.Type)
	}
	if unmarshaled.Severity != req.Severity {
		t.Errorf("Severity = %v, want %v", unmarshaled.Severity, req.Severity)
	}
}
