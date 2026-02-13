package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func setupComplianceHandler(t *testing.T) (*http.ServeMux, *secrets.InMemorySecretAuditLogger, *secrets.SecretBroker) {
	t.Helper()
	audit := secrets.NewInMemorySecretAuditLogger(nil)
	broker := secrets.NewSecretBroker(nil)
	backend := newMockBackend()
	if err := broker.RegisterBackend("mock", backend); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(broker, nil, nil, nil, audit)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, audit, broker
}

func TestHandleComplianceReports(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/reports", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ComplianceReportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestHandleComplianceReportsMethodNotAllowed(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/reports", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleAuditLogs(t *testing.T) {
	mux, audit, _ := setupComplianceHandler(t)

	_ = audit.LogSecretAccess(context.Background(), &secrets.SecretAccessEvent{
		SecretPath: "vault/kv/app",
		Action:     "read",
		Timestamp:  time.Now(),
		Success:    true,
		AgentID:    "agent-1",
	})
	_ = audit.LogSecretAccess(context.Background(), &secrets.SecretAccessEvent{
		SecretPath: "vault/kv/other",
		Action:     "read",
		Timestamp:  time.Now(),
		Success:    true,
		AgentID:    "agent-2",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuditLogResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestHandleAuditLogsWithFilters(t *testing.T) {
	mux, audit, _ := setupComplianceHandler(t)

	now := time.Now()
	_ = audit.LogSecretAccess(context.Background(), &secrets.SecretAccessEvent{
		SecretPath: "vault/kv/app",
		Action:     "read",
		Timestamp:  now,
		Success:    true,
		AgentID:    "agent-1",
	})
	_ = audit.LogSecretAccess(context.Background(), &secrets.SecretAccessEvent{
		SecretPath: "vault/kv/other",
		Action:     "read_failed",
		Timestamp:  now.Add(time.Minute),
		Success:    false,
		AgentID:    "agent-2",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs?path=vault/kv/app&limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuditLogResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
}

func TestHandleAuditLogsWithTimeRange(t *testing.T) {
	mux, audit, _ := setupComplianceHandler(t)

	now := time.Now()
	_ = audit.LogSecretAccess(context.Background(), &secrets.SecretAccessEvent{
		SecretPath: "vault/kv/app",
		Action:     "read",
		Timestamp:  now,
		Success:    true,
	})

	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Add(time.Hour).Format(time.RFC3339)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs?start="+start+"&end="+end, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuditLogResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
}

func TestHandleAuditLogsInvalidStartTime(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs?start=not-a-time", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleAuditLogsMethodNotAllowed(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleAuditLogsNilLogger(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSecretsHealthHealthy(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Healthy {
		t.Error("expected healthy to be true")
	}
	if resp.BackendCount != 1 {
		t.Errorf("expected backend_count 1, got %d", resp.BackendCount)
	}
}

func TestHandleSecretsHealthDegraded(t *testing.T) {
	audit := secrets.NewInMemorySecretAuditLogger(nil)
	broker := secrets.NewSecretBroker(nil)
	backend := newMockBackend()
	backend.healthy = false
	if err := broker.RegisterBackend("mock", backend); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(broker, nil, nil, nil, audit)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Healthy {
		t.Error("expected healthy to be false")
	}
}

func TestHandleSecretsHealthNilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSecretsHealthMethodNotAllowed(t *testing.T) {
	mux, _, _ := setupComplianceHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/health/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
