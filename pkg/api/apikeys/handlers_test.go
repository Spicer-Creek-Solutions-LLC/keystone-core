package apikeys

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	// Verify routes respond
	req := httptest.NewRequest(http.MethodPut, "/api/v1/api-keys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCreate(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"name":"test-key","role":"admin","expires_in":"30d"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp CreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Name != "test-key" {
		t.Errorf("name = %q, want %q", resp.Name, "test-key")
	}
	if resp.Role != "admin" {
		t.Errorf("role = %q, want %q", resp.Role, "admin")
	}
	if resp.ID == "" {
		t.Error("id should not be empty")
	}
	if !strings.HasPrefix(resp.Key, "ks_") {
		t.Errorf("key should start with ks_, got %q", resp.Key)
	}
}

func TestHandleCreateDefaults(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"name":"default-key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp CreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Role != "readonly" {
		t.Errorf("default role = %q, want %q", resp.Role, "readonly")
	}
}

func TestHandleCreateMissingName(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"role":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateInvalidRole(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"name":"test","role":"superadmin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateInvalidBody(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateInvalidExpiresIn(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"name":"test","expires_in":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleList(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// List empty
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var entries []ListEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries count = %d, want 0", len(entries))
	}

	// Create a key then list again
	body := `{"name":"list-test","role":"operator","expires_in":"7d"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}
	if entries[0].Name != "list-test" {
		t.Errorf("name = %q, want %q", entries[0].Name, "list-test")
	}
	if entries[0].Role != "operator" {
		t.Errorf("role = %q, want %q", entries[0].Role, "operator")
	}
}

func TestHandleRevoke(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Create a key
	body := `{"name":"revoke-test","role":"readonly","expires_in":"1d"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var created CreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Revoke it
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+created.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["revoked"] != true {
		t.Errorf("revoked = %v, want true", resp["revoked"])
	}

	// Verify it no longer appears in list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var entries []ListEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries count = %d after revoke, want 0", len(entries))
	}
}

func TestHandleRevokeNotFound(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRevokeMissingID(t *testing.T) {
	store := NewMemoryStore()
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()

	// Create
	key := &APIKey{
		ID:        "test-id",
		Name:      "test",
		Role:      "admin",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := store.Create(key); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Get
	got, err := store.Get("test-id")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}

	// List
	keys, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("List count = %d, want 1", len(keys))
	}

	// Delete (revoke)
	if err := store.Delete("test-id"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// List should be empty (revoked keys hidden)
	keys, err = store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List count after delete = %d, want 0", len(keys))
	}
}

func TestMemoryStoreGetNotFound(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestMemoryStoreDeleteNotFound(t *testing.T) {
	store := NewMemoryStore()
	err := store.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},
		{"", 0, true},
		{"x", 0, true},
		{"30m", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
	if len(id1) != 16 {
		t.Errorf("ID length = %d, want 16", len(id1))
	}
}

func TestGenerateKey(t *testing.T) {
	key1 := GenerateKey()
	key2 := GenerateKey()
	if key1 == key2 {
		t.Error("generated keys should be unique")
	}
	if !strings.HasPrefix(key1, "ks_") {
		t.Errorf("key should start with ks_, got %q", key1)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

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
