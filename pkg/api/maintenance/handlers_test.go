package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHandler(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterRoutes(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify routes respond (wrong method returns 405)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/enable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /enable status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleEnable(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"reason":"scheduled upgrade","duration":"2h","force":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["enabled"] != true {
		t.Errorf("enabled = %v, want true", resp["enabled"])
	}
	if resp["reason"] != "scheduled upgrade" {
		t.Errorf("reason = %v", resp["reason"])
	}
}

func TestHandleEnableConflict(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Enable first
	body := `{"reason":"first"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first enable status = %d", w.Code)
	}

	// Try to enable again without force
	body = `{"reason":"second"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("second enable status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestHandleEnableForce(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Enable first
	body := `{"reason":"first"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Force enable
	body = `{"reason":"forced","force":true}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("force enable status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleEnableInvalidBody(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDisable(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Enable first
	body := `{"reason":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Disable
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/disable", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["enabled"] != false {
		t.Errorf("enabled = %v, want false", resp["enabled"])
	}
}

func TestHandleDisableNotEnabled(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/disable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestHandleStatus(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check status when disabled
	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Enabled {
		t.Error("should not be enabled initially")
	}

	// Enable and check again
	body := `{"reason":"upgrade"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/status", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !resp.Enabled {
		t.Error("should be enabled after enable call")
	}
	if resp.Reason != "upgrade" {
		t.Errorf("reason = %q, want %q", resp.Reason, "upgrade")
	}
}

func TestHandleQueue(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/queue", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp QueueResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.QueuedCommands != 0 {
		t.Errorf("queued_commands = %d, want 0", resp.QueuedCommands)
	}
}

func TestHandleCleanup(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/cleanup?older_than=24h&dry_run=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", resp["dry_run"])
	}
	if resp["older_than"] != "24h" {
		t.Errorf("older_than = %v, want 24h", resp["older_than"])
	}
}

func TestHandleMethodNotAllowed(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"enable GET", http.MethodGet, "/api/v1/maintenance/enable"},
		{"disable GET", http.MethodGet, "/api/v1/maintenance/disable"},
		{"status POST", http.MethodPost, "/api/v1/maintenance/status"},
		{"queue POST", http.MethodPost, "/api/v1/maintenance/queue"},
		{"cleanup GET", http.MethodGet, "/api/v1/maintenance/cleanup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()

	// Initial state should be empty
	state, err := store.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if state.Enabled {
		t.Error("initial state should not be enabled")
	}

	// Set state
	newState := &State{
		Enabled: true,
		Reason:  "testing",
	}
	if err := store.SetState(context.Background(), newState); err != nil {
		t.Fatalf("SetState error: %v", err)
	}

	// Verify state
	state, err = store.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if !state.Enabled {
		t.Error("state should be enabled")
	}
	if state.Reason != "testing" {
		t.Errorf("reason = %q, want %q", state.Reason, "testing")
	}

	// Clear state
	if err := store.ClearState(context.Background()); err != nil {
		t.Fatalf("ClearState error: %v", err)
	}

	state, err = store.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if state.Enabled {
		t.Error("state should not be enabled after clear")
	}
}

func TestMemoryStoreCopySemantics(t *testing.T) {
	store := NewMemoryStore()

	original := &State{Enabled: true, Reason: "original"}
	if err := store.SetState(context.Background(), original); err != nil {
		t.Fatalf("SetState error: %v", err)
	}

	// Modify original after setting - should not affect stored state
	original.Reason = "modified"

	state, err := store.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if state.Reason != "original" {
		t.Errorf("reason = %q, want %q (store should copy values)", state.Reason, "original")
	}

	// Modify retrieved state - should not affect stored state
	state.Reason = "also modified"
	state2, err := store.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if state2.Reason != "original" {
		t.Errorf("reason = %q, want %q (store should return copies)", state2.Reason, "original")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}
	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %v", w.Header().Get("Content-Type"))
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result["error"] != "test error" {
		t.Errorf("error = %v", result["error"])
	}
}
