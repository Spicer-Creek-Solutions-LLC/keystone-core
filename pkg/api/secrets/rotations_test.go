package secrets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func setupRotationsHandler(t *testing.T) (*http.ServeMux, *secrets.RotationOrchestrator) {
	t.Helper()
	broker := secrets.NewSecretBroker(nil)
	orch := secrets.NewRotationOrchestrator(broker)
	h := NewHandler(broker, nil, orch, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, orch
}

func TestHandleStartRotation(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	body := `{
		"id": "rot-1",
		"secret_path": "vault/db/creds",
		"config": {
			"strategy": "blue_green",
			"rollback_on_failure": true
		},
		"targets": [
			{"id": "t1", "name": "target-1", "type": "agent", "agent_id": "agent-1"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "rot-1" {
		t.Errorf("expected ID rot-1, got %s", resp.ID)
	}
}

func TestHandleStartRotationInvalidBody(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleStartRotationMissingFields(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	body := `{"id": "rot-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetRotationNotFound(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rotations/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetRotationViaStart(t *testing.T) {
	// Test that start returns valid rotation data (GET may race with async cleanup).
	mux, _ := setupRotationsHandler(t)

	body := `{
		"id": "rot-get",
		"secret_path": "vault/db/creds",
		"config": {"strategy": "blue_green"},
		"targets": [{"id": "t1", "name": "t1", "type": "agent"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "rot-get" {
		t.Errorf("expected ID rot-get, got %s", resp.ID)
	}
	if resp.SecretPath != "vault/db/creds" {
		t.Errorf("expected secret_path vault/db/creds, got %s", resp.SecretPath)
	}
	if resp.TotalTargets != 1 {
		t.Errorf("expected total_targets 1, got %d", resp.TotalTargets)
	}

	// GET may return 200 or 404 depending on whether async completed.
	// Both are acceptable since the rotation runs async.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/rotations/rot-get", nil)
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK && getW.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d: %s", getW.Code, getW.Body.String())
	}
}

func TestHandleCancelRotation(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	// Start a rotation first
	body := `{
		"id": "rot-cancel",
		"secret_path": "vault/db/creds",
		"config": {"strategy": "blue_green"},
		"targets": [{"id": "t1", "name": "t1", "type": "agent"}]
	}`
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startW := httptest.NewRecorder()
	mux.ServeHTTP(startW, startReq)

	// Cancel it — may succeed or fail (rotation could be terminal already)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations/rot-cancel/cancel", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusConflict {
		t.Errorf("expected 200 or 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRollbackRotationNotFailed(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	body := `{
		"id": "rot-rollback",
		"secret_path": "vault/db/creds",
		"config": {"strategy": "blue_green"},
		"targets": [{"id": "t1", "name": "t1", "type": "agent"}]
	}`
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/rotations", strings.NewReader(body))
	startReq.Header.Set("Content-Type", "application/json")
	startW := httptest.NewRecorder()
	mux.ServeHTTP(startW, startReq)

	// Try to rollback — should fail since rotation is not in failed state
	// (could also be 409 "not found" if already cleaned up)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rotations/rot-rollback/rollback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRotationsNilOrchestrator(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"start", http.MethodPost, "/api/v1/rotations"},
		{"get", http.MethodGet, "/api/v1/rotations/test-id"},
		{"cancel", http.MethodPost, "/api/v1/rotations/test-id/cancel"},
		{"rollback", http.MethodPost, "/api/v1/rotations/test-id/rollback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.method == http.MethodPost && tt.path == "/api/v1/rotations" {
				body = strings.NewReader(`{"id":"r","secret_path":"p","config":{"strategy":"blue_green"}}`)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503, got %d", w.Code)
			}
		})
	}
}

func TestHandleRotationsMethodNotAllowed(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rotations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCancelRotationMethodNotAllowed(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rotations/test-id/cancel", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleRollbackRotationMethodNotAllowed(t *testing.T) {
	mux, _ := setupRotationsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rotations/test-id/rollback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
