package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// mockBackend implements secrets.SecretBackend for testing.
type mockBackend struct {
	secrets map[string]*secrets.Secret
	lists   map[string][]string
	healthy bool
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		secrets: make(map[string]*secrets.Secret),
		lists:   make(map[string][]string),
		healthy: true,
	}
}

func (m *mockBackend) Type() secrets.BackendType    { return secrets.BackendTypeVault }
func (m *mockBackend) Name() string                 { return "mock" }
func (m *mockBackend) Healthy(_ context.Context) bool { return m.healthy }
func (m *mockBackend) Close() error                 { return nil }

func (m *mockBackend) Read(_ context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	s, ok := m.secrets[req.Path]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}
	return s, nil
}

func (m *mockBackend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	return m.Read(ctx, req)
}

func (m *mockBackend) List(_ context.Context, prefix string) ([]string, error) {
	keys, ok := m.lists[prefix]
	if !ok {
		return []string{}, nil
	}
	return keys, nil
}

func (m *mockBackend) RenewLease(_ context.Context, _ string, _ time.Duration) (*secrets.Lease, error) {
	return nil, nil
}

func (m *mockBackend) RevokeLease(_ context.Context, _ string) error {
	return nil
}

func setupSecretsHandler(t *testing.T) (*Handler, *http.ServeMux, *mockBackend) {
	t.Helper()
	backend := newMockBackend()
	broker := secrets.NewSecretBroker(nil)
	if err := broker.RegisterBackend("mock", backend); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(broker, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux, backend
}

func TestHandleSecretRead(t *testing.T) {
	_, mux, backend := setupSecretsHandler(t)

	backend.secrets["kv/myapp"] = &secrets.Secret{
		Path:    "mock/kv/myapp",
		Backend: secrets.BackendTypeVault,
		Type:    secrets.SecretTypeStatic,
		Data:    map[string]interface{}{"username": "admin", "password": "secret"},
		Version: 1,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/kv/myapp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SecretResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Path != "mock/kv/myapp" {
		t.Errorf("expected path mock/kv/myapp, got %s", resp.Path)
	}
	if resp.Version != 1 {
		t.Errorf("expected version 1, got %d", resp.Version)
	}
}

func TestHandleSecretReadNotFound(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSecretReadWithVersion(t *testing.T) {
	_, mux, backend := setupSecretsHandler(t)

	backend.secrets["kv/myapp"] = &secrets.Secret{
		Path:    "mock/kv/myapp",
		Backend: secrets.BackendTypeVault,
		Type:    secrets.SecretTypeStatic,
		Data:    map[string]interface{}{"key": "value"},
		Version: 3,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/kv/myapp?version=3", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSecretReadInvalidVersion(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/kv/myapp?version=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSecretList(t *testing.T) {
	_, mux, backend := setupSecretsHandler(t)

	backend.lists["kv/"] = []string{"app1", "app2", "app3"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/kv/?list=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SecretListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(resp.Keys))
	}
}

func TestHandleSecretListEmpty(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/kv/empty?list=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SecretListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Keys == nil {
		t.Error("expected non-nil keys array")
	}
	if len(resp.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(resp.Keys))
	}
}

func TestHandleSecretWriteNotWritable(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	body := `{"data":{"key":"value"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/mock/kv/myapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSecretWriteInvalidBody(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/mock/kv/myapp", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSecretWriteMissingData(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	body := `{"metadata":{"env":"prod"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/mock/kv/myapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSecretDeleteNotDeletable(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/mock/kv/myapp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSecretMethodNotAllowed(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/secrets/mock/kv/myapp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSecretEmptyPath(t *testing.T) {
	_, mux, _ := setupSecretsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSecretNilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/vault/kv/app", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSecretReadWithLease(t *testing.T) {
	_, mux, backend := setupSecretsHandler(t)

	backend.secrets["db/creds"] = &secrets.Secret{
		Path:    "mock/db/creds",
		Backend: secrets.BackendTypeVault,
		Type:    secrets.SecretTypeDynamic,
		Data:    map[string]interface{}{"username": "dynamic-user"},
		Lease: &secrets.Lease{
			ID:        "lease-123",
			Renewable: true,
		},
		Renewable: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/mock/db/creds", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SecretResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.LeaseID != "lease-123" {
		t.Errorf("expected lease_id lease-123, got %s", resp.LeaseID)
	}
	if !resp.Renewable {
		t.Error("expected renewable to be true")
	}
}
