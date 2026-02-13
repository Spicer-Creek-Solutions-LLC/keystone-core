package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// mockTransitBackend implements secrets.TransitBackend for testing.
type mockTransitBackend struct{}

func (m *mockTransitBackend) Encrypt(_ context.Context, req *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{
		Ciphertext: "vault:v1:encrypted-" + string(req.Plaintext),
		KeyVersion: 1,
	}, nil
}

func (m *mockTransitBackend) Decrypt(_ context.Context, req *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	plain := strings.TrimPrefix(req.Ciphertext, "vault:v1:encrypted-")
	return &secrets.TransitResponse{
		Plaintext:  []byte(plain),
		KeyVersion: 1,
	}, nil
}

func (m *mockTransitBackend) Rewrap(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Ciphertext: "vault:v2:rewrapped"}, nil
}

func (m *mockTransitBackend) Sign(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{
		Signature:  "vault:v1:sig-abc123",
		KeyVersion: 1,
	}, nil
}

func (m *mockTransitBackend) Verify(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Valid: true}, nil
}

func (m *mockTransitBackend) HMAC(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{HMAC: "vault:v1:hmac-abc123"}, nil
}

func (m *mockTransitBackend) Hash(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Hash: "sha256:abc123"}, nil
}

func (m *mockTransitBackend) BatchEncrypt(_ context.Context, reqs []*secrets.TransitRequest) ([]*secrets.TransitResponse, error) {
	results := make([]*secrets.TransitResponse, len(reqs))
	for i, req := range reqs {
		results[i] = &secrets.TransitResponse{
			Ciphertext: "vault:v1:batch-" + string(req.Plaintext),
			KeyVersion: 1,
		}
	}
	return results, nil
}

func (m *mockTransitBackend) BatchDecrypt(_ context.Context, reqs []*secrets.TransitRequest) ([]*secrets.TransitResponse, error) {
	results := make([]*secrets.TransitResponse, len(reqs))
	for i, req := range reqs {
		plain := strings.TrimPrefix(req.Ciphertext, "vault:v1:batch-")
		results[i] = &secrets.TransitResponse{
			Plaintext:  []byte(plain),
			KeyVersion: 1,
		}
	}
	return results, nil
}

func setupTransitHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	h := NewHandler(nil, nil, nil, &mockTransitBackend{}, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandleTransitEncrypt(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"plaintext":"aGVsbG8="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/encrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitEncryptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ciphertext == "" {
		t.Error("expected non-empty ciphertext")
	}
}

func TestHandleTransitDecrypt(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"ciphertext":"vault:v1:encrypted-hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/decrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitDecryptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Plaintext) != "hello" {
		t.Errorf("expected plaintext hello, got %s", string(resp.Plaintext))
	}
}

func TestHandleTransitBatchEncrypt(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"items":[{"plaintext":"aGVsbG8="},{"plaintext":"d29ybGQ="}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/batch-encrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitBatchEncryptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}
}

func TestHandleTransitBatchDecrypt(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"items":[{"ciphertext":"vault:v1:batch-hello"},{"ciphertext":"vault:v1:batch-world"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/batch-decrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitBatchDecryptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}
}

func TestHandleTransitSign(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"input":"aGVsbG8="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/sign/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitSignResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Signature == "" {
		t.Error("expected non-empty signature")
	}
}

func TestHandleTransitVerify(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"input":"aGVsbG8=","signature":"vault:v1:sig-abc123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/verify/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitVerifyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Error("expected valid to be true")
	}
}

func TestHandleTransitHMAC(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"input":"aGVsbG8="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/hmac/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TransitHMACResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.HMAC == "" {
		t.Error("expected non-empty HMAC")
	}
}

func TestHandleTransitNilBackend(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"plaintext":"aGVsbG8="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/encrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleTransitUnknownOp(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/unknown/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTransitMissingKey(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"plaintext":"aGVsbG8="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/encrypt/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTransitMethodNotAllowed(t *testing.T) {
	mux := setupTransitHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transit/encrypt/mykey", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleTransitDatakey(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/datakey/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

func TestHandleTransitBatchEncryptEmptyItems(t *testing.T) {
	mux := setupTransitHandler(t)

	body := `{"items":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit/batch-encrypt/mykey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
