// SPDX-License-Identifier: Apache-2.0

package apikeys_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
)

// httpDo executes req against srv and returns (status, body bytes).
// Helper to keep tests succinct.
func httpDo(t *testing.T, srv *httptest.Server, method, path string, body any) (int, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, out
}

func newHandlerServer(t *testing.T) (*httptest.Server, *fakeAPIKeyStore) {
	t.Helper()
	store := &fakeAPIKeyStore{}
	h := apikeys.NewHandler(store)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store
}

func TestHandler_Create_ReturnsCleartextOnce(t *testing.T) {
	srv, _ := newHandlerServer(t)

	status, body := httpDo(t, srv, "POST", "/api/v1/apikeys",
		map[string]any{"name": "ops-key", "role": "operator"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", status, body)
	}

	var resp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID == "" || resp.Name != "ops-key" || resp.Role != "operator" {
		t.Errorf("response: %+v", resp)
	}
	if len(resp.Key) < apikeys.MinCleartextLength {
		t.Errorf("key cleartext too short: %q", resp.Key)
	}

	// Subsequent GET must NOT include the cleartext.
	status, body = httpDo(t, srv, "GET", "/api/v1/apikeys/"+resp.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("get status = %d", status)
	}
	if strings.Contains(string(body), `"key":`) {
		t.Errorf("GET response leaked cleartext: %s", body)
	}
	if strings.Contains(string(body), `"key_hash":`) {
		t.Errorf("GET response leaked key_hash: %s", body)
	}
}

func TestHandler_Create_RejectsInvalidJSON(t *testing.T) {
	srv, _ := newHandlerServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/apikeys",
		bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_Create_RejectsMissingFields(t *testing.T) {
	srv, _ := newHandlerServer(t)
	for _, body := range []map[string]any{
		{"role": "operator"},               // missing name
		{"name": "k"},                      // missing role
		{"name": "k", "role": "superuser"}, // bad role
	} {
		status, _ := httpDo(t, srv, "POST", "/api/v1/apikeys", body)
		if status != http.StatusBadRequest {
			t.Errorf("body=%v: status = %d, want 400", body, status)
		}
	}
}

func TestHandler_List(t *testing.T) {
	srv, _ := newHandlerServer(t)

	for _, name := range []string{"a", "b", "c"} {
		status, _ := httpDo(t, srv, "POST", "/api/v1/apikeys",
			map[string]any{"name": name, "role": "operator"})
		if status != http.StatusCreated {
			t.Fatalf("seed %q: status = %d", name, status)
		}
	}

	status, body := httpDo(t, srv, "GET", "/api/v1/apikeys", nil)
	if status != http.StatusOK {
		t.Fatalf("list status = %d", status)
	}

	var listed struct {
		APIKeys []map[string]any `json:"apikeys"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.APIKeys) != 3 {
		t.Errorf("listed %d, want 3", len(listed.APIKeys))
	}
	// No cleartext or hash in any list entry.
	for _, k := range listed.APIKeys {
		if _, ok := k["key"]; ok {
			t.Errorf("list leaked cleartext: %v", k)
		}
		if _, ok := k["key_hash"]; ok {
			t.Errorf("list leaked key_hash: %v", k)
		}
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	srv, _ := newHandlerServer(t)
	status, _ := httpDo(t, srv, "GET", "/api/v1/apikeys/missing", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestHandler_Delete(t *testing.T) {
	srv, _ := newHandlerServer(t)
	_, body := httpDo(t, srv, "POST", "/api/v1/apikeys",
		map[string]any{"name": "k", "role": "operator"})
	var resp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &resp)

	status, _ := httpDo(t, srv, "DELETE", "/api/v1/apikeys/"+resp.ID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", status)
	}

	// Now GET should 404.
	status, _ = httpDo(t, srv, "GET", "/api/v1/apikeys/"+resp.ID, nil)
	if status != http.StatusNotFound {
		t.Errorf("post-delete get status = %d, want 404", status)
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	srv, _ := newHandlerServer(t)
	status, _ := httpDo(t, srv, "DELETE", "/api/v1/apikeys/missing", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestHandler_StoredOnlyHash(t *testing.T) {
	// Critical acceptance criterion: storage holds hash only,
	// cleartext is never persisted.
	srv, store := newHandlerServer(t)
	status, body := httpDo(t, srv, "POST", "/api/v1/apikeys",
		map[string]any{"name": "k", "role": "admin"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d", status)
	}
	var resp struct {
		Key string `json:"key"`
	}
	json.Unmarshal(body, &resp)

	// Walk every stored record. Cleartext must not appear in any
	// field of any record.
	for _, r := range store.byHash {
		// KeyHash should be present and non-empty.
		if r.KeyHash == "" {
			t.Error("stored record has empty KeyHash")
		}
		// Cleartext must not equal any field.
		fields := []string{r.ID, r.Name, r.KeyHash, r.Role}
		for _, f := range fields {
			if f == resp.Key {
				t.Errorf("stored field %q == cleartext key %q", f, resp.Key)
			}
		}
	}
}

func TestHandler_WrongMethod(t *testing.T) {
	srv, _ := newHandlerServer(t)
	// PUT /api/v1/apikeys is not registered; net/http's method-aware
	// patterns return 405 Method Not Allowed.
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/apikeys", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

// Ensure clock injection works for deterministic CreatedAt in tests.
func TestHandler_SetClock(t *testing.T) {
	store := &fakeAPIKeyStore{}
	h := apikeys.NewHandler(store)
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	h.SetClock(func() time.Time { return now })

	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	status, body := httpDo(t, srv, "POST", "/api/v1/apikeys",
		map[string]any{"name": "k", "role": "operator"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d", status)
	}
	var resp struct {
		CreatedAt time.Time `json:"created_at"`
	}
	json.Unmarshal(body, &resp)
	if !resp.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", resp.CreatedAt, now)
	}
}
