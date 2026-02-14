package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	isecrets "github.com/shawnbutts/keystone-core/internal/secrets"
)

// backendsTestBackend implements isecrets.SecretBackend for testing.
type backendsTestBackend struct {
	name    string
	typ     isecrets.BackendType
	healthy bool
}

func (b *backendsTestBackend) Type() isecrets.BackendType                                     { return b.typ }
func (b *backendsTestBackend) Name() string                                                    { return b.name }
func (b *backendsTestBackend) Healthy(context.Context) bool                                    { return b.healthy }
func (b *backendsTestBackend) Read(context.Context, *isecrets.SecretRequest) (*isecrets.Secret, error) {
	return nil, nil
}
func (b *backendsTestBackend) ReadDynamic(context.Context, *isecrets.SecretRequest) (*isecrets.Secret, error) {
	return nil, nil
}
func (b *backendsTestBackend) List(context.Context, string) ([]string, error) { return nil, nil }
func (b *backendsTestBackend) RenewLease(context.Context, string, time.Duration) (*isecrets.Lease, error) {
	return nil, nil
}
func (b *backendsTestBackend) RevokeLease(context.Context, string) error { return nil }
func (b *backendsTestBackend) Close() error                              { return nil }

func TestHandleBackendsList_NilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends", nil)
	w := httptest.NewRecorder()
	h.handleBackendsList(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleBackendsList_Success(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	if err := broker.RegisterBackend("vault", &backendsTestBackend{name: "vault", typ: isecrets.BackendTypeVault, healthy: true}); err != nil {
		t.Fatal(err)
	}
	if err := broker.RegisterBackend("aws", &backendsTestBackend{name: "aws", typ: isecrets.BackendTypeAWS, healthy: false}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends", nil)
	w := httptest.NewRecorder()
	h.handleBackendsList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp BackendListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("expected 2 backends, got %d", resp.Total)
	}
}

func TestHandleBackendsList_MethodNotAllowed(t *testing.T) {
	h := NewHandler(isecrets.NewSecretBroker(nil), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/backends", nil)
	w := httptest.NewRecorder()
	h.handleBackendsList(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleBackendRoute_NilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends/vault", nil)
	w := httptest.NewRecorder()
	h.handleBackendRoute(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleBackendRoute_NotFound(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends/nonexistent", nil)
	w := httptest.NewRecorder()
	h.handleBackendRoute(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleBackendRoute_Success(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	if err := broker.RegisterBackend("vault", &backendsTestBackend{name: "vault", typ: isecrets.BackendTypeVault, healthy: true}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends/vault", nil)
	w := httptest.NewRecorder()
	h.handleBackendRoute(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp BackendInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "vault" {
		t.Errorf("expected name 'vault', got %q", resp.Name)
	}
	if resp.Type != "vault" {
		t.Errorf("expected type 'vault', got %q", resp.Type)
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
}

func TestHandleBackendRoute_EmptyName(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	if err := broker.RegisterBackend("vault", &backendsTestBackend{name: "vault", typ: isecrets.BackendTypeVault, healthy: true}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/backends/", nil)
	w := httptest.NewRecorder()
	h.handleBackendRoute(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (delegates to list), got %d", w.Code)
	}
}

func TestHandleCacheStats_NilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/cache/stats", nil)
	w := httptest.NewRecorder()
	h.handleCacheStats(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCacheStats_NoCache(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/cache/stats", nil)
	w := httptest.NewRecorder()
	h.handleCacheStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp CacheStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Entries != 0 {
		t.Errorf("expected 0 entries, got %d", resp.Entries)
	}
}

func TestHandleCacheStats_WithCache(t *testing.T) {
	cfg := &isecrets.CacheConfig{
		Enabled:    true,
		MaxEntries: 100,
	}
	broker := isecrets.NewSecretBroker(&isecrets.BrokerConfig{
		Cache: cfg,
	})
	cache := isecrets.NewInMemorySecretCache(cfg)
	defer cache.Close()
	broker.SetCache(cache)

	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/cache/stats", nil)
	w := httptest.NewRecorder()
	h.handleCacheStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp CacheStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.MaxEntries != 100 {
		t.Errorf("expected max_entries=100, got %d", resp.MaxEntries)
	}
}

func TestHandleCacheStats_MethodNotAllowed(t *testing.T) {
	h := NewHandler(isecrets.NewSecretBroker(nil), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/cache/stats", nil)
	w := httptest.NewRecorder()
	h.handleCacheStats(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCacheRoute_NilBroker(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/cache", nil)
	w := httptest.NewRecorder()
	h.handleCacheRoute(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCacheRoute_Clear(t *testing.T) {
	broker := isecrets.NewSecretBroker(nil)
	cache := isecrets.NewInMemorySecretCache(nil)
	defer cache.Close()
	broker.SetCache(cache)

	h := NewHandler(broker, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/cache", nil)
	w := httptest.NewRecorder()
	h.handleCacheRoute(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp CacheClearResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Message != "cache cleared" {
		t.Errorf("expected message 'cache cleared', got %q", resp.Message)
	}
}

func TestHandleCacheRoute_MethodNotAllowed(t *testing.T) {
	h := NewHandler(isecrets.NewSecretBroker(nil), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/cache", nil)
	w := httptest.NewRecorder()
	h.handleCacheRoute(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestBackendsRoute_Registration(t *testing.T) {
	mux := http.NewServeMux()
	broker := isecrets.NewSecretBroker(nil)
	h := NewHandler(broker, nil, nil, nil, nil)
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/api/v1/secrets/backends", http.StatusOK},
		{http.MethodGet, "/api/v1/secrets/backends/vault", http.StatusNotFound},
		{http.MethodGet, "/api/v1/secrets/cache/stats", http.StatusOK},
		{http.MethodDelete, "/api/v1/secrets/cache", http.StatusOK},
	}

	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 404 && r.want != 404 {
			t.Errorf("route %s %s returned 404 (not registered)", r.method, r.path)
		}
	}
}
