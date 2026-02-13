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

// mockLeaseManager implements secrets.LeaseManager for testing.
type mockLeaseManager struct {
	leases map[string]*secrets.Lease
}

func newMockLeaseManager() *mockLeaseManager {
	return &mockLeaseManager{
		leases: make(map[string]*secrets.Lease),
	}
}

func (m *mockLeaseManager) Track(_ context.Context, lease *secrets.Lease) error {
	m.leases[lease.ID] = lease
	return nil
}

func (m *mockLeaseManager) Get(_ context.Context, leaseID string) (*secrets.Lease, error) {
	l, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrLeaseNotFound
	}
	return l, nil
}

func (m *mockLeaseManager) List(_ context.Context) ([]*secrets.Lease, error) {
	result := make([]*secrets.Lease, 0, len(m.leases))
	for _, l := range m.leases {
		result = append(result, l)
	}
	return result, nil
}

func (m *mockLeaseManager) ListByPath(_ context.Context, path string) ([]*secrets.Lease, error) {
	var result []*secrets.Lease
	for _, l := range m.leases {
		if strings.HasPrefix(l.SecretPath, path) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockLeaseManager) ListByBackend(_ context.Context, backend secrets.BackendType) ([]*secrets.Lease, error) {
	var result []*secrets.Lease
	for _, l := range m.leases {
		if l.Backend == backend {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockLeaseManager) ListExpiring(_ context.Context, _ time.Duration) ([]*secrets.Lease, error) {
	return nil, nil
}

func (m *mockLeaseManager) Renew(_ context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	l, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrLeaseNotFound
	}
	if !l.Renewable {
		return nil, secrets.ErrLeaseNotRenewable
	}
	l.ExpiresAt = time.Now().Add(increment)
	l.RenewalCount++
	l.LastRenewal = time.Now()
	return l, nil
}

func (m *mockLeaseManager) Revoke(_ context.Context, leaseID string) error {
	if _, ok := m.leases[leaseID]; !ok {
		return secrets.ErrLeaseNotFound
	}
	delete(m.leases, leaseID)
	return nil
}

func (m *mockLeaseManager) RevokeByPath(_ context.Context, pathPrefix string) (int, error) {
	count := 0
	for id, l := range m.leases {
		if strings.HasPrefix(l.SecretPath, pathPrefix) {
			delete(m.leases, id)
			count++
		}
	}
	return count, nil
}

func (m *mockLeaseManager) Remove(_ context.Context, leaseID string) error {
	delete(m.leases, leaseID)
	return nil
}

func (m *mockLeaseManager) Stats(_ context.Context) (*secrets.LeaseStats, error) {
	return &secrets.LeaseStats{ActiveLeases: len(m.leases)}, nil
}

func (m *mockLeaseManager) Start(_ context.Context) error { return nil }
func (m *mockLeaseManager) Stop() error                   { return nil }

func setupLeasesHandler(t *testing.T) (*http.ServeMux, *mockLeaseManager) {
	t.Helper()
	lm := newMockLeaseManager()
	h := NewHandler(nil, lm, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, lm
}

func TestHandleLeaseGet(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "vault/db/creds",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        30 * time.Minute,
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(30 * time.Minute),
		Renewable:  true,
		Revocable:  true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases/lease-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LeaseResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "lease-1" {
		t.Errorf("expected ID lease-1, got %s", resp.ID)
	}
	if resp.State != "active" {
		t.Errorf("expected state active, got %s", resp.State)
	}
}

func TestHandleLeaseGetNotFound(t *testing.T) {
	mux, _ := setupLeasesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleLeasesList(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "vault/db/creds",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        30 * time.Minute,
	}
	lm.leases["lease-2"] = &secrets.Lease{
		ID:         "lease-2",
		SecretPath: "vault/db/other",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        60 * time.Minute,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LeaseListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestHandleLeasesListWithPathFilter(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "vault/db/creds",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        30 * time.Minute,
	}
	lm.leases["lease-2"] = &secrets.Lease{
		ID:         "lease-2",
		SecretPath: "aws/iam/role",
		Backend:    secrets.BackendTypeAWS,
		State:      secrets.LeaseStateActive,
		TTL:        60 * time.Minute,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases?path=vault", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LeaseListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
}

func TestHandleLeasesListWithLimitOffset(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	for i := 0; i < 5; i++ {
		id := "lease-" + string(rune('a'+i))
		lm.leases[id] = &secrets.Lease{
			ID:         id,
			SecretPath: "vault/db/" + id,
			Backend:    secrets.BackendTypeVault,
			State:      secrets.LeaseStateActive,
			TTL:        30 * time.Minute,
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LeaseListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Leases) != 2 {
		t.Errorf("expected 2 leases, got %d", len(resp.Leases))
	}
	if resp.Total != 5 {
		t.Errorf("expected total 5, got %d", resp.Total)
	}
}

func TestHandleLeaseRenew(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{
		ID:        "lease-1",
		Renewable: true,
		TTL:       30 * time.Minute,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	body := `{"increment_seconds": 3600}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/lease-1/renew", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LeaseResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.RenewalCount != 1 {
		t.Errorf("expected renewal count 1, got %d", resp.RenewalCount)
	}
}

func TestHandleLeaseRenewNotRenewable(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{
		ID:        "lease-1",
		Renewable: false,
	}

	body := `{"increment_seconds": 3600}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/lease-1/renew", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLeaseRevoke(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{ID: "lease-1"}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/lease-1/revoke", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, ok := lm.leases["lease-1"]; ok {
		t.Error("expected lease to be revoked")
	}
}

func TestHandleLeaseRevokeNotFound(t *testing.T) {
	mux, _ := setupLeasesHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/nonexistent/revoke", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleBulkRevokeByPath(t *testing.T) {
	mux, lm := setupLeasesHandler(t)

	lm.leases["lease-1"] = &secrets.Lease{ID: "lease-1", SecretPath: "vault/db/creds"}
	lm.leases["lease-2"] = &secrets.Lease{ID: "lease-2", SecretPath: "vault/db/other"}
	lm.leases["lease-3"] = &secrets.Lease{ID: "lease-3", SecretPath: "aws/iam/role"}

	body := `{"path":"vault/db"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/bulk-revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkRevokeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Revoked != 2 {
		t.Errorf("expected 2 revoked, got %d", resp.Revoked)
	}
}

func TestHandleBulkRevokeNoFilter(t *testing.T) {
	mux, _ := setupLeasesHandler(t)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leases/bulk-revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLeasesNilLeaseManager(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleLeasesMethodNotAllowed(t *testing.T) {
	mux, _ := setupLeasesHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/leases", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleLeasesListInvalidBackend(t *testing.T) {
	mux, _ := setupLeasesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/leases?backend=invalid_type", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
