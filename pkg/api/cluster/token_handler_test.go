package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/audit"
	"github.com/shawnbutts/keystone-core/internal/cluster/token"
)

// mockTokenStore implements token.Store for testing.
type mockTokenStore struct {
	mu     sync.Mutex
	tokens map[string]*token.JoinToken

	// lookupFunc allows overriding Lookup behavior for specific tests.
	lookupFunc func(ctx context.Context, rawToken string) (*token.JoinToken, error)
	// incrementErr overrides IncrementUses to return an error.
	incrementErr error
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{tokens: make(map[string]*token.JoinToken)}
}

func (m *mockTokenStore) Create(_ context.Context, t *token.JoinToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[t.ID]; ok {
		return token.ErrTokenDuplicate
	}
	m.tokens[t.ID] = t
	return nil
}

func (m *mockTokenStore) GetByID(_ context.Context, id string) (*token.JoinToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	if !ok {
		return nil, token.ErrTokenNotFound
	}
	return t, nil
}

func (m *mockTokenStore) Lookup(ctx context.Context, rawToken string) (*token.JoinToken, error) {
	if m.lookupFunc != nil {
		return m.lookupFunc(ctx, rawToken)
	}
	return nil, token.ErrTokenNotFound
}

func (m *mockTokenStore) List(_ context.Context) ([]*token.JoinToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*token.JoinToken, 0, len(m.tokens))
	for _, t := range m.tokens {
		result = append(result, t)
	}
	return result, nil
}

func (m *mockTokenStore) Revoke(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	if !ok {
		return token.ErrTokenNotFound
	}
	t.Revoked = true
	return nil
}

func (m *mockTokenStore) IncrementUses(_ context.Context, id string) error {
	if m.incrementErr != nil {
		return m.incrementErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	if !ok {
		return token.ErrTokenNotFound
	}
	if t.Revoked {
		return token.ErrTokenRevoked
	}
	if t.IsExpired() {
		return token.ErrTokenExpired
	}
	if t.MaxUses > 0 && t.UsedCount >= t.MaxUses {
		return token.ErrTokenExhausted
	}
	t.UsedCount++
	return nil
}

func (m *mockTokenStore) DeleteExpired(_ context.Context) (int, error) {
	return 0, nil
}

// helper to add a token directly to the mock store.
func (m *mockTokenStore) add(t *token.JoinToken) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[t.ID] = t
}

func TestCreateToken(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(GenerateTokenRequest{
		Label:   "test-token",
		TTL:     "1h",
		MaxUses: 5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/tokens", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleTokens(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp GenerateTokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected raw token in response")
	}
	if resp.ID == "" {
		t.Error("expected token ID in response")
	}
	if resp.Label != "test-token" {
		t.Errorf("expected label 'test-token', got %q", resp.Label)
	}
	if resp.MaxUses != 5 {
		t.Errorf("expected max_uses 5, got %d", resp.MaxUses)
	}
}

func TestCreateToken_InvalidTTL(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(GenerateTokenRequest{TTL: "not-a-duration"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/tokens", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleTokens(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateToken_NilStore(t *testing.T) {
	h := &Handler{}

	body, _ := json.Marshal(GenerateTokenRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/tokens", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleTokens(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListTokens(t *testing.T) {
	store := newMockTokenStore()
	store.add(&token.JoinToken{
		ID:        "tok-1",
		Label:     "first",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	store.add(&token.JoinToken{
		ID:        "tok-2",
		Label:     "second",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens", nil)
	w := httptest.NewRecorder()

	h.handleTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp []TokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(resp))
	}
}

func TestListTokens_Empty(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens", nil)
	w := httptest.NewRecorder()

	h.handleTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp []TokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(resp))
	}
}

func TestGetToken(t *testing.T) {
	store := newMockTokenStore()
	store.add(&token.JoinToken{
		ID:        "tok-abc",
		Label:     "my-token",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens/tok-abc", nil)
	w := httptest.NewRecorder()

	h.handleTokenResource(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.ID != "tok-abc" {
		t.Errorf("expected id 'tok-abc', got %q", resp.ID)
	}
}

func TestGetToken_NotFound(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens/nonexistent", nil)
	w := httptest.NewRecorder()

	h.handleTokenResource(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRevokeToken(t *testing.T) {
	store := newMockTokenStore()
	store.add(&token.JoinToken{
		ID:        "tok-revoke",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster/tokens/tok-revoke", nil)
	w := httptest.NewRecorder()

	h.handleTokenResource(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRevokeToken_NotFound(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster/tokens/nonexistent", nil)
	w := httptest.NewRecorder()

	h.handleTokenResource(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleJoin_ValidToken(t *testing.T) {
	store := newMockTokenStore()
	validToken := &token.JoinToken{
		ID:        "tok-valid",
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   10,
	}
	store.add(validToken)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return validToken, nil
	}

	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{
		ClusterAddress: "10.0.0.1:9300",
		Token:          "kscore-join-sometoken",
		AdvertiseAddr:  "10.0.0.2:9300",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestHandleJoin_NoToken_Required(t *testing.T) {
	store := newMockTokenStore()
	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{
		ClusterAddress: "10.0.0.1:9300",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_NoToken_NoStore(t *testing.T) {
	h := &Handler{}

	body, _ := json.Marshal(JoinRequest{
		ClusterAddress: "10.0.0.1:9300",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (backward compat), got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_ExpiredToken(t *testing.T) {
	store := newMockTokenStore()
	expired := &token.JoinToken{
		ID:        "tok-expired",
		ExpiresAt: time.Now().Add(-time.Hour),
		MaxUses:   10,
	}
	store.add(expired)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return expired, nil
	}

	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{Token: "some-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_RevokedToken(t *testing.T) {
	store := newMockTokenStore()
	revoked := &token.JoinToken{
		ID:        "tok-revoked",
		ExpiresAt: time.Now().Add(time.Hour),
		Revoked:   true,
	}
	store.add(revoked)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return revoked, nil
	}

	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{Token: "some-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_ExhaustedToken(t *testing.T) {
	store := newMockTokenStore()
	exhausted := &token.JoinToken{
		ID:        "tok-exhausted",
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   1,
		UsedCount: 1,
	}
	store.add(exhausted)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return exhausted, nil
	}

	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{Token: "some-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_InvalidToken(t *testing.T) {
	store := newMockTokenStore()
	// lookupFunc returns not found (default behavior)
	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{Token: "kscore-join-doesnotexist"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJoin_IncrementUsesError(t *testing.T) {
	store := newMockTokenStore()
	valid := &token.JoinToken{
		ID:        "tok-inc-err",
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   10,
	}
	store.add(valid)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return valid, nil
	}
	store.incrementErr = token.ErrTokenExhausted

	h := &Handler{tokenStore: store}

	body, _ := json.Marshal(JoinRequest{Token: "some-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleJoin(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLogging(t *testing.T) {
	store := newMockTokenStore()
	memWriter := audit.NewMemoryWriter()
	logger := audit.NewLogger(audit.DefaultConfig())
	logger.AddWriter(memWriter)
	defer logger.Close()

	validToken := &token.JoinToken{
		ID:        "tok-audit",
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   10,
	}
	store.add(validToken)
	store.lookupFunc = func(_ context.Context, _ string) (*token.JoinToken, error) {
		return validToken, nil
	}

	h := &Handler{tokenStore: store, auditLogger: logger}

	// Create token
	createBody, _ := json.Marshal(GenerateTokenRequest{Label: "audit-test", TTL: "1h"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/tokens", bytes.NewReader(createBody))
	createW := httptest.NewRecorder()
	h.handleTokens(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", createW.Code)
	}

	// Decode to get the ID for revoke
	var createResp GenerateTokenResponse
	json.NewDecoder(createW.Body).Decode(&createResp)

	// Revoke the created token
	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster/tokens/"+createResp.ID, nil)
	revokeW := httptest.NewRecorder()
	h.handleTokenResource(revokeW, revokeReq)
	if revokeW.Code != http.StatusNoContent {
		t.Fatalf("revoke: expected 204, got %d", revokeW.Code)
	}

	// Join with valid token
	joinBody, _ := json.Marshal(JoinRequest{Token: "kscore-join-test"})
	joinReq := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader(joinBody))
	joinW := httptest.NewRecorder()
	h.handleJoin(joinW, joinReq)
	if joinW.Code != http.StatusOK {
		t.Fatalf("join: expected 200, got %d: %s", joinW.Code, joinW.Body.String())
	}

	// Flush and check events (async buffer needs a moment)
	logger.Flush()
	if err := memWriter.WaitForCount(3, 5*time.Second); err != nil {
		t.Fatalf("expected 3 audit events: %v (got %d)", err, memWriter.Count())
	}

	events := memWriter.Events()
	// Verify event categories/actions
	foundCreate := false
	foundRevoke := false
	foundJoin := false
	for _, e := range events {
		switch {
		case e.Category == audit.CategoryAdmin && e.Action == audit.ActionCreate:
			foundCreate = true
		case e.Category == audit.CategoryAdmin && e.Action == audit.ActionRevoke:
			foundRevoke = true
		case e.Category == audit.CategoryAuth && e.Action == audit.ActionLogin:
			foundJoin = true
		}
	}
	if !foundCreate {
		t.Error("missing audit event for token creation")
	}
	if !foundRevoke {
		t.Error("missing audit event for token revocation")
	}
	if !foundJoin {
		t.Error("missing audit event for cluster join")
	}
}

func TestTokenErrorStatus(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{token.ErrTokenNotFound, http.StatusNotFound},
		{token.ErrTokenExpired, http.StatusGone},
		{token.ErrTokenRevoked, http.StatusForbidden},
		{token.ErrTokenExhausted, http.StatusForbidden},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		got := tokenErrorStatus(tt.err)
		if got != tt.status {
			t.Errorf("tokenErrorStatus(%v) = %d, want %d", tt.err, got, tt.status)
		}
	}
}

func TestHandleTokens_MethodNotAllowed(t *testing.T) {
	h := &Handler{tokenStore: newMockTokenStore()}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cluster/tokens", nil)
	w := httptest.NewRecorder()
	h.handleTokens(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleTokenResource_MethodNotAllowed(t *testing.T) {
	h := &Handler{tokenStore: newMockTokenStore()}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/tokens/some-id", nil)
	w := httptest.NewRecorder()
	h.handleTokenResource(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleTokenResource_MissingID(t *testing.T) {
	h := &Handler{tokenStore: newMockTokenStore()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens/", nil)
	w := httptest.NewRecorder()
	h.handleTokenResource(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleJoin_MethodNotAllowed(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/join", nil)
	w := httptest.NewRecorder()
	h.handleJoin(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleJoin_InvalidBody(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/join", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	h.handleJoin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListTokens_NilStore(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens", nil)
	w := httptest.NewRecorder()
	h.handleTokens(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestGetToken_NilStore(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/tokens/some-id", nil)
	w := httptest.NewRecorder()
	h.handleTokenResource(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestRevokeToken_NilStore(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster/tokens/some-id", nil)
	w := httptest.NewRecorder()
	h.handleTokenResource(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
