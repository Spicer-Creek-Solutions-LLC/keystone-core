// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	internalsecrets "go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	pkgsecrets "go.keystone-core.io/keystone-core/pkg/api/secrets"
)

// testRig builds a Handler backed by a real broker + lease manager
// + fake test backend + fake transit. The broker is fully wired —
// the REST layer exercises the full chain.
type testRig struct {
	srv     *httptest.Server
	backend *fakeBackend
	transit *fakeTransit
}

func newRig(t *testing.T) *testRig {
	t.Helper()

	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	be := &fakeBackend{
		name: "test",
		caps: []internalsecrets.BackendCapability{
			internalsecrets.CapKV, internalsecrets.CapList,
			internalsecrets.CapDynamic, internalsecrets.CapLeaseRenew, internalsecrets.CapLeaseRevoke,
		},
		entries: make(map[string]*internalsecrets.Secret),
		leases:  make(map[string]struct{}),
	}

	router, err := internalsecrets.NewRouter([]internalsecrets.Route{
		{Prefix: "kv/", Backend: "test"},
		{Prefix: "database/", Backend: "test"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	lm, err := internalsecrets.NewLeaseManager(internalsecrets.LeaseManagerConfig{Store: store})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}
	broker, err := internalsecrets.NewBroker(internalsecrets.BrokerConfig{
		Router:         router,
		Backends:       []internalsecrets.SecretBackend{be},
		DefaultBackend: "test",
		LeaseDirectory: lm,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	lm.SetRenewer(broker.RenewLease)

	transit := &fakeTransit{}
	h := pkgsecrets.NewHandler(broker, transit, lm)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testRig{srv: srv, backend: be, transit: transit}
}

func (r *testRig) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequest(method, r.srv.URL+path, reader)
	} else {
		req, err = http.NewRequest(method, r.srv.URL+path, nil)
	}
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// ---- KV ----------------------------------------------------------

func TestHandler_WriteGet_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newRig(t)

	// Write.
	resp := r.do(t, "PUT", "/api/v1/secrets/kv/app/db", map[string]any{
		"data":   map[string]any{"password": "hunter2"},
		"labels": map[string]any{"env": "prod"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	// Get.
	resp2 := r.do(t, "GET", "/api/v1/secrets/kv/app/db", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d", resp2.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&got)
	data := got["data"].(map[string]any)
	if data["password"] != "hunter2" {
		t.Errorf("password = %v", data["password"])
	}
}

func TestHandler_GetSecret_NotFound(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "GET", "/api/v1/secrets/kv/missing", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandler_GetSecret_InvalidVersion(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "GET", "/api/v1/secrets/kv/x?version=abc", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_WriteSecret_BadJSON(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	req, _ := http.NewRequest("PUT", r.srv.URL+"/api/v1/secrets/kv/x", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_WriteSecret_TTLSecondsToMetadata(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "PUT", "/api/v1/secrets/kv/ttl", map[string]any{
		"data":        map[string]any{"k": "v"},
		"ttl_seconds": 60,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	r.backend.mu.Lock()
	stored := r.backend.entries["kv/ttl"]
	r.backend.mu.Unlock()
	if stored.Metadata["ttl_seconds"] != "60" {
		t.Errorf("ttl_seconds metadata = %q, want 60", stored.Metadata["ttl_seconds"])
	}
}

func TestHandler_ListSecrets(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for _, p := range []string{"a", "b", "c"} {
		resp := r.do(t, "PUT", "/api/v1/secrets/kv/"+p, map[string]any{"data": map[string]any{"k": "v"}})
		resp.Body.Close()
	}

	resp := r.do(t, "GET", "/api/v1/secrets?prefix=kv/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Entries) != 3 {
		t.Errorf("entries = %d, want 3", len(body.Entries))
	}
}

func TestHandler_ListSecrets_BadPageSize(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "GET", "/api/v1/secrets?page_size=abc", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_DeleteSecret(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "PUT", "/api/v1/secrets/kv/x", map[string]any{"data": map[string]any{"k": "v"}})
	resp.Body.Close()

	resp2 := r.do(t, "DELETE", "/api/v1/secrets/kv/x", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp2.StatusCode)
	}

	// Subsequent Get returns 404.
	resp3 := r.do(t, "GET", "/api/v1/secrets/kv/x", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("after delete: status = %d, want 404", resp3.StatusCode)
	}
}

// ---- Leases ------------------------------------------------------

func TestHandler_LeaseFlow(t *testing.T) {
	t.Parallel()
	r := newRig(t)

	// Use the test backend directly to issue a dynamic secret (no
	// REST route for IssueDynamicSecret in v1.0).
	r.backend.mu.Lock()
	r.backend.leases["lease-1"] = struct{}{}
	r.backend.mu.Unlock()
	// Lease manager doesn't know about it yet — record via PUT to a
	// dynamic path which will route through the broker.

	// List leases (empty).
	resp := r.do(t, "GET", "/api/v1/leases", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ListLeases status = %d", resp.StatusCode)
	}

	// GET on unknown lease returns 404.
	resp2 := r.do(t, "GET", "/api/v1/leases/ghost", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("GetLease(ghost) status = %d, want 404", resp2.StatusCode)
	}

	// Revoke unknown lease → ErrLeaseNotFound → 404.
	resp3 := r.do(t, "POST", "/api/v1/leases/ghost/revoke", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("RevokeLease(ghost) status = %d, want 404", resp3.StatusCode)
	}

	// Renew unknown lease → 404.
	resp4 := r.do(t, "POST", "/api/v1/leases/ghost/renew", nil)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Errorf("RenewLease(ghost) status = %d, want 404", resp4.StatusCode)
	}
}

func TestHandler_ListLeases_BadPageSize(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "GET", "/api/v1/leases?page_size=abc", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---- Transit -----------------------------------------------------

func TestHandler_Transit_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newRig(t)

	// Encrypt.
	resp := r.do(t, "POST", "/api/v1/transit/encrypt", map[string]any{
		"key":       "k",
		"plaintext": []byte("hello"),
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Encrypt status = %d", resp.StatusCode)
	}
	var enc struct {
		Ciphertext string `json:"ciphertext"`
		KeyVersion int    `json:"key_version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&enc)

	// Decrypt.
	resp2 := r.do(t, "POST", "/api/v1/transit/decrypt", map[string]any{
		"key":        "k",
		"ciphertext": enc.Ciphertext,
	})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Decrypt status = %d", resp2.StatusCode)
	}
	var dec struct {
		Plaintext []byte `json:"plaintext"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&dec)
	if string(dec.Plaintext) != "hello" {
		t.Errorf("decrypt mismatch: %q", dec.Plaintext)
	}
}

func TestHandler_Transit_SignVerify(t *testing.T) {
	t.Parallel()
	r := newRig(t)

	resp := r.do(t, "POST", "/api/v1/transit/sign", map[string]any{
		"key":     "k",
		"message": []byte("p"),
	})
	defer resp.Body.Close()
	var sig struct {
		Signature string `json:"signature"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sig)

	resp2 := r.do(t, "POST", "/api/v1/transit/verify", map[string]any{
		"key":       "k",
		"message":   []byte("p"),
		"signature": sig.Signature,
	})
	defer resp2.Body.Close()
	var v struct {
		Valid bool `json:"valid"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&v)
	if !v.Valid {
		t.Errorf("Verify of freshly-signed payload = false")
	}
}

func TestHandler_Transit_BadJSON(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	req, _ := http.NewRequest("POST", r.srv.URL+"/api/v1/transit/encrypt", strings.NewReader("garbage"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_Transit_MissingKey(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := r.do(t, "POST", "/api/v1/transit/encrypt", map[string]any{"plaintext": []byte("x")})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---- Disabled ----------------------------------------------------

func TestHandler_Disabled_Returns503(t *testing.T) {
	t.Parallel()
	h := pkgsecrets.NewHandler(nil, nil, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/v1/secrets/x"},
		{"PUT", "/api/v1/secrets/x"},
		{"DELETE", "/api/v1/secrets/x"},
		{"GET", "/api/v1/secrets"},
		{"GET", "/api/v1/leases"},
		{"GET", "/api/v1/leases/x"},
		{"POST", "/api/v1/leases/x/renew"},
		{"POST", "/api/v1/leases/x/revoke"},
		{"POST", "/api/v1/transit/encrypt"},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var reader *strings.Reader
			if tc.method == "PUT" || tc.method == "POST" {
				reader = strings.NewReader("{}")
			}
			var req *http.Request
			if reader != nil {
				req, _ = http.NewRequest(tc.method, srv.URL+tc.path, reader)
			} else {
				req, _ = http.NewRequest(tc.method, srv.URL+tc.path, nil)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", resp.StatusCode)
			}
		})
	}
}

// ---- test backend + transit fakes -------------------------------

type fakeBackend struct {
	mu      sync.Mutex
	name    string
	caps    []internalsecrets.BackendCapability
	entries map[string]*internalsecrets.Secret
	leases  map[string]struct{}
}

func (b *fakeBackend) Name() string                                      { return b.name }
func (b *fakeBackend) Capabilities() []internalsecrets.BackendCapability { return b.caps }
func (b *fakeBackend) Start(context.Context) error                       { return nil }
func (b *fakeBackend) Stop(context.Context) error                        { return nil }
func (b *fakeBackend) Health(context.Context) error                      { return nil }

func (b *fakeBackend) GetSecret(_ context.Context, req internalsecrets.GetSecretRequest) (*internalsecrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.entries[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", internalsecrets.ErrSecretNotFound, req.Path)
	}
	return s, nil
}

func (b *fakeBackend) WriteSecret(_ context.Context, req internalsecrets.WriteSecretRequest) (*internalsecrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &internalsecrets.Secret{Path: req.Path, Data: req.Data, Metadata: req.Metadata, Version: 1}
	b.entries[req.Path] = s
	return s, nil
}

func (b *fakeBackend) ListSecrets(_ context.Context, req internalsecrets.ListSecretsRequest) (*internalsecrets.ListSecretsResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &internalsecrets.ListSecretsResponse{}
	for path := range b.entries {
		if req.Prefix == "" || strings.HasPrefix(path, req.Prefix) {
			out.Entries = append(out.Entries, internalsecrets.ListEntry{Path: path})
		}
	}
	return out, nil
}

func (b *fakeBackend) DeleteSecret(_ context.Context, req internalsecrets.DeleteSecretRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, req.Path)
	return nil
}

func (b *fakeBackend) IssueDynamicSecret(context.Context, internalsecrets.IssueDynamicSecretRequest) (*internalsecrets.Secret, error) {
	return nil, internalsecrets.ErrNotImplementedYet
}

func (b *fakeBackend) RenewLease(_ context.Context, req internalsecrets.RenewLeaseRequest) (*internalsecrets.LeaseInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.leases[req.LeaseID]; !ok {
		return nil, fmt.Errorf("%w: %q", internalsecrets.ErrLeaseNotFound, req.LeaseID)
	}
	return &internalsecrets.LeaseInfo{ID: req.LeaseID}, nil
}

func (b *fakeBackend) RevokeLease(_ context.Context, req internalsecrets.RevokeLeaseRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.leases, req.LeaseID)
	return nil
}

type fakeTransit struct{}

func (fakeTransit) Encrypt(_ context.Context, req internalsecrets.EncryptRequest) (*internalsecrets.EncryptResponse, error) {
	out := &internalsecrets.EncryptResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, internalsecrets.EncryptResult{
			Ciphertext: "vault:v1:" + string(it.Plaintext),
			KeyVersion: 1,
		})
	}
	return out, nil
}

func (fakeTransit) Decrypt(_ context.Context, req internalsecrets.DecryptRequest) (*internalsecrets.DecryptResponse, error) {
	out := &internalsecrets.DecryptResponse{}
	for _, it := range req.Items {
		const prefix = "vault:v1:"
		if !strings.HasPrefix(it.Ciphertext, prefix) {
			out.Results = append(out.Results, internalsecrets.DecryptResult{Err: "bad ciphertext"})
			continue
		}
		out.Results = append(out.Results, internalsecrets.DecryptResult{Plaintext: []byte(it.Ciphertext[len(prefix):])})
	}
	return out, nil
}

func (fakeTransit) Sign(_ context.Context, req internalsecrets.SignRequest) (*internalsecrets.SignResponse, error) {
	out := &internalsecrets.SignResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, internalsecrets.SignResult{
			Signature: "vault:v1:sig(" + string(it.Input) + ")",
		})
	}
	return out, nil
}

func (fakeTransit) Verify(_ context.Context, req internalsecrets.VerifyRequest) (*internalsecrets.VerifyResponse, error) {
	out := &internalsecrets.VerifyResponse{}
	for _, it := range req.Items {
		want := "vault:v1:sig(" + string(it.Input) + ")"
		out.Results = append(out.Results, internalsecrets.VerifyResult{Valid: it.Signature == want})
	}
	return out, nil
}

func (fakeTransit) HMAC(context.Context, internalsecrets.HMACRequest) (*internalsecrets.HMACResponse, error) {
	return &internalsecrets.HMACResponse{}, nil
}
func (fakeTransit) VerifyHMAC(context.Context, internalsecrets.VerifyHMACRequest) (*internalsecrets.VerifyResponse, error) {
	return &internalsecrets.VerifyResponse{}, nil
}
func (fakeTransit) Rewrap(context.Context, internalsecrets.RewrapRequest) (*internalsecrets.RewrapResponse, error) {
	return &internalsecrets.RewrapResponse{}, nil
}
func (fakeTransit) GenerateDataKey(context.Context, internalsecrets.GenerateDataKeyRequest) (*internalsecrets.GenerateDataKeyResponse, error) {
	return &internalsecrets.GenerateDataKeyResponse{}, nil
}

var _ internalsecrets.TransitBackend = fakeTransit{}
