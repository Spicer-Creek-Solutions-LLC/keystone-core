package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookRequestStructure(t *testing.T) {
	req := WebhookRequest{
		Type: "github",
		Payload: map[string]interface{}{
			"action":     "push",
			"repository": "myrepo",
			"ref":        "refs/heads/main",
		},
	}

	if req.Type != "github" {
		t.Errorf("Type = %v", req.Type)
	}
	if req.Payload["action"] != "push" {
		t.Errorf("Payload[action] = %v", req.Payload["action"])
	}
	if req.Payload["repository"] != "myrepo" {
		t.Errorf("Payload[repository] = %v", req.Payload["repository"])
	}
}

func TestWebhookResponseStructure(t *testing.T) {
	resp := WebhookResponse{
		ID:        "webhook-123",
		Status:    "accepted",
		Type:      "github",
		EventType: "push",
		Timestamp: time.Now(),
	}

	if resp.ID != "webhook-123" {
		t.Errorf("ID = %v", resp.ID)
	}
	if resp.Status != "accepted" {
		t.Errorf("Status = %v", resp.Status)
	}
	if resp.Type != "github" {
		t.Errorf("Type = %v", resp.Type)
	}
	if resp.EventType != "push" {
		t.Errorf("EventType = %v", resp.EventType)
	}
}

func TestStatsResponseStructure(t *testing.T) {
	now := time.Now()
	resp := StatsResponse{
		TotalReceived:  100,
		TotalProcessed: 95,
		TotalFailed:    5,
		ByType: map[string]int64{
			"github":  50,
			"gitlab":  30,
			"argocd":  15,
			"flux":    5,
		},
		LastReceivedTime:  &now,
		LastProcessedTime: &now,
		RetrievedAt:       now,
	}

	if resp.TotalReceived != 100 {
		t.Errorf("TotalReceived = %d", resp.TotalReceived)
	}
	if resp.TotalProcessed != 95 {
		t.Errorf("TotalProcessed = %d", resp.TotalProcessed)
	}
	if resp.TotalFailed != 5 {
		t.Errorf("TotalFailed = %d", resp.TotalFailed)
	}
	if len(resp.ByType) != 4 {
		t.Errorf("ByType count = %d", len(resp.ByType))
	}
	if resp.ByType["github"] != 50 {
		t.Errorf("ByType[github] = %d", resp.ByType["github"])
	}
}

func TestConfigResponseStructure(t *testing.T) {
	resp := ConfigResponse{
		Enabled:    true,
		Addr:       ":8080",
		Path:       "/webhooks",
		AuthType:   "hmac",
		Handlers:   []string{"argocd", "flux", "github", "gitlab"},
		WebhookURL: "https://example.com/webhooks",
	}

	if !resp.Enabled {
		t.Error("Enabled should be true")
	}
	if resp.Addr != ":8080" {
		t.Errorf("Addr = %v", resp.Addr)
	}
	if resp.AuthType != "hmac" {
		t.Errorf("AuthType = %v", resp.AuthType)
	}
	if len(resp.Handlers) != 4 {
		t.Errorf("Handlers count = %d", len(resp.Handlers))
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"id":     "webhook-123",
		"status": "accepted",
	}

	writeJSON(w, http.StatusAccepted, data)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %v", w.Header().Get("Content-Type"))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["id"] != "webhook-123" {
		t.Errorf("result[id] = %v", result["id"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "invalid webhook type")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "invalid webhook type" {
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
	t.Run("webhooks endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// GET should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET /api/v1/webhooks status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("stats endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/webhooks/stats status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("config endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/config", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/webhooks/config status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestWebhookRequestJSONMarshal(t *testing.T) {
	req := WebhookRequest{
		Type: "argocd",
		Payload: map[string]interface{}{
			"app":    "myapp",
			"status": "Synced",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled WebhookRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != req.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, req.Type)
	}
	if unmarshaled.Payload["app"] != "myapp" {
		t.Errorf("Payload[app] = %v", unmarshaled.Payload["app"])
	}
}

func TestWebhookResponseJSONMarshal(t *testing.T) {
	resp := WebhookResponse{
		ID:        "webhook-456",
		Status:    "accepted",
		Type:      "flux",
		EventType: "ReconciliationSucceeded",
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled WebhookResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID = %v, want %v", unmarshaled.ID, resp.ID)
	}
	if unmarshaled.Status != resp.Status {
		t.Errorf("Status = %v, want %v", unmarshaled.Status, resp.Status)
	}
	if unmarshaled.Type != resp.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, resp.Type)
	}
}

func TestStatsResponseJSONMarshal(t *testing.T) {
	now := time.Now().UTC()
	resp := StatsResponse{
		TotalReceived:     50,
		TotalProcessed:    48,
		TotalFailed:       2,
		ByType:            map[string]int64{"github": 50},
		LastReceivedTime:  &now,
		LastProcessedTime: &now,
		RetrievedAt:       now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled StatsResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.TotalReceived != resp.TotalReceived {
		t.Errorf("TotalReceived = %d, want %d", unmarshaled.TotalReceived, resp.TotalReceived)
	}
	if unmarshaled.TotalFailed != resp.TotalFailed {
		t.Errorf("TotalFailed = %d, want %d", unmarshaled.TotalFailed, resp.TotalFailed)
	}
}

func TestConfigResponseJSONMarshal(t *testing.T) {
	resp := ConfigResponse{
		Enabled:    true,
		Addr:       ":9090",
		Path:       "/hooks",
		AuthType:   "token",
		Handlers:   []string{"github"},
		WebhookURL: "https://hooks.example.com",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ConfigResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !unmarshaled.Enabled {
		t.Error("Enabled should be true")
	}
	if unmarshaled.AuthType != resp.AuthType {
		t.Errorf("AuthType = %v, want %v", unmarshaled.AuthType, resp.AuthType)
	}
	if len(unmarshaled.Handlers) != 1 {
		t.Errorf("Handlers count = %d", len(unmarshaled.Handlers))
	}
}

func TestStatsResponseOptionalTimes(t *testing.T) {
	resp := StatsResponse{
		TotalReceived:  0,
		TotalProcessed: 0,
		TotalFailed:    0,
		ByType:         map[string]int64{},
		RetrievedAt:    time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled StatsResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.LastReceivedTime != nil {
		t.Error("LastReceivedTime should be nil")
	}
	if unmarshaled.LastProcessedTime != nil {
		t.Error("LastProcessedTime should be nil")
	}
}
