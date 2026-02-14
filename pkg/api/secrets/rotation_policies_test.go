package secrets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/credentials/rotation"
)

// mockPolicyManager implements RotationPolicyManager for testing.
type mockPolicyManager struct {
	policies map[string]*rotation.Policy
}

func newMockPolicyManager() *mockPolicyManager {
	return &mockPolicyManager{policies: make(map[string]*rotation.Policy)}
}

func (m *mockPolicyManager) AddPolicy(p *rotation.Policy) error {
	if _, ok := m.policies[p.ID]; ok {
		return fmt.Errorf("policy already exists")
	}
	m.policies[p.ID] = p
	return nil
}

func (m *mockPolicyManager) GetPolicy(id string) (*rotation.Policy, bool) {
	p, ok := m.policies[id]
	return p, ok
}

func (m *mockPolicyManager) ListPolicies() []*rotation.Policy {
	result := make([]*rotation.Policy, 0, len(m.policies))
	for _, p := range m.policies {
		result = append(result, p)
	}
	return result
}

func (m *mockPolicyManager) RemovePolicy(id string) error {
	if _, ok := m.policies[id]; !ok {
		return fmt.Errorf("policy not found")
	}
	delete(m.policies, id)
	return nil
}

func (m *mockPolicyManager) UpdatePolicy(p *rotation.Policy) error {
	if _, ok := m.policies[p.ID]; !ok {
		return fmt.Errorf("policy not found")
	}
	m.policies[p.ID] = p
	return nil
}

func setupPolicyHandler(t *testing.T) (*http.ServeMux, *mockPolicyManager) {
	t.Helper()
	mgr := newMockPolicyManager()
	h := NewHandler(nil, nil, nil, nil, nil)
	h.SetRotationPolicyEngine(mgr)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, mgr
}

func TestHandleListRotationPolicies(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-1"] = &rotation.Policy{
		ID: "pol-1", Name: "test-policy", Enabled: true,
		CredentialTypes: []credentials.CredentialType{"*"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationPolicyListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 policy, got %d", resp.Total)
	}
}

func TestHandleListRotationPoliciesEmpty(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp RotationPolicyListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 policies, got %d", resp.Total)
	}
}

func TestHandleGetRotationPolicy(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-1"] = &rotation.Policy{
		ID: "pol-1", Name: "test-policy",
		CredentialTypes: []credentials.CredentialType{"password"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies/pol-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationPolicyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "pol-1" {
		t.Errorf("expected ID pol-1, got %s", resp.ID)
	}
	if resp.Name != "test-policy" {
		t.Errorf("expected name test-policy, got %s", resp.Name)
	}
}

func TestHandleGetRotationPolicyNotFound(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCreateRotationPolicy(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	body := `{"id":"pol-new","name":"new-policy","max_age":"90d","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationPolicyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "pol-new" {
		t.Errorf("expected ID pol-new, got %s", resp.ID)
	}
}

func TestHandleCreateRotationPolicyInvalidBody(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateRotationPolicyMissingFields(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	body := `{"id":"pol-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateRotationPolicyDuplicate(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-1"] = &rotation.Policy{
		ID: "pol-1", Name: "existing",
		CredentialTypes: []credentials.CredentialType{"*"},
	}

	body := `{"id":"pol-1","name":"duplicate","max_age":"30d"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleDeleteRotationPolicy(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-del"] = &rotation.Policy{
		ID: "pol-del", Name: "to-delete",
		CredentialTypes: []credentials.CredentialType{"*"},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/rotation/policies/pol-del", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationPolicyActionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.PolicyID != "pol-del" {
		t.Errorf("expected policy_id pol-del, got %s", resp.PolicyID)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHandleDeleteRotationPolicyNotFound(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/rotation/policies/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleEnableRotationPolicy(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-en"] = &rotation.Policy{
		ID: "pol-en", Name: "to-enable", Enabled: false,
		CredentialTypes: []credentials.CredentialType{"*"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies/pol-en/enable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if !mgr.policies["pol-en"].Enabled {
		t.Error("expected policy to be enabled")
	}
}

func TestHandleEnableRotationPolicyNotFound(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies/nonexistent/enable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDisableRotationPolicy(t *testing.T) {
	mux, mgr := setupPolicyHandler(t)
	mgr.policies["pol-dis"] = &rotation.Policy{
		ID: "pol-dis", Name: "to-disable", Enabled: true,
		CredentialTypes: []credentials.CredentialType{"*"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies/pol-dis/disable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if mgr.policies["pol-dis"].Enabled {
		t.Error("expected policy to be disabled")
	}
}

func TestHandleDisableRotationPolicyNotFound(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies/nonexistent/disable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleRotationPoliciesNilEngine(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list", http.MethodGet, "/api/v1/secrets/rotation/policies"},
		{"create", http.MethodPost, "/api/v1/secrets/rotation/policies"},
		{"get", http.MethodGet, "/api/v1/secrets/rotation/policies/test-id"},
		{"delete", http.MethodDelete, "/api/v1/secrets/rotation/policies/test-id"},
		{"enable", http.MethodPost, "/api/v1/secrets/rotation/policies/test-id/enable"},
		{"disable", http.MethodPost, "/api/v1/secrets/rotation/policies/test-id/disable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.method == http.MethodPost && tt.path == "/api/v1/secrets/rotation/policies" {
				body = strings.NewReader(`{"id":"p","name":"n","max_age":"30d"}`)
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

func TestHandleRotationPoliciesMethodNotAllowed(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/rotation/policies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleEnableRotationPolicyMethodNotAllowed(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies/test-id/enable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleDisableRotationPolicyMethodNotAllowed(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/rotation/policies/test-id/disable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCreateRotationPolicyInvalidMaxAge(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	body := `{"id":"pol-bad","name":"bad-policy","max_age":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateRotationPolicyWithWarningAge(t *testing.T) {
	mux, _ := setupPolicyHandler(t)

	body := `{"id":"pol-warn","name":"warn-policy","max_age":"90d","warning_age":"7d","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/rotation/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp RotationPolicyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.WarningAge == "" {
		t.Error("expected non-empty warning_age")
	}
}

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"90d", false},
		{"30d", false},
		{"24h", false},
		{"1h30m", false},
		{"invalid", true},
		{"d", true},
	}

	for _, tt := range tests {
		_, err := parseDurationString(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDurationString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}
