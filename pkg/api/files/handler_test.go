package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	internalfiles "go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/acl"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// newServer wires a Handler with a fresh in-memory store + optional
// ACL into an httptest server. Returns the server, the store (so
// tests can seed/inspect state) and a shutdown closure.
func newServer(t *testing.T, a acl.ACL) (*httptest.Server, backend.Store) {
	t.Helper()
	store := backend.NewMemoryStore(nil)
	mux := http.NewServeMux()
	NewHandler(store, a, nil).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store
}

func newServerWithStore(t *testing.T, store backend.Store, a acl.ACL) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(store, a, nil).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- happy paths -------------------------------------------------------------

func TestPutGetDelete_RoundTrip(t *testing.T) {
	srv, _ := newServer(t, nil)

	// PUT
	body := []byte("hello rest")
	req := mustReq(t, http.MethodPut, srv.URL+"/api/v1/files/configs/app.yaml", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/yaml")
	q := req.URL.Query()
	q.Add("tag", "env=prod")
	q.Add("tag", "owner=ops")
	req.URL.RawQuery = q.Encode()
	resp := doReq(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d (%s)", resp.StatusCode, readErr(resp))
	}
	var put fileMetadataDTO
	mustDecode(t, resp, &put)
	if put.Hash != internalfiles.HashOf(body) {
		t.Errorf("PUT hash mismatch")
	}
	if put.Tags["env"] != "prod" || put.Tags["owner"] != "ops" {
		t.Errorf("PUT tags = %+v", put.Tags)
	}

	// GET body
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d (%s)", resp.StatusCode, readErr(resp))
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("GET body mismatch")
	}
	if got := resp.Header.Get("X-Kscore-File-Hash"); got != put.Hash {
		t.Errorf("X-Kscore-File-Hash = %q, want %q", got, put.Hash)
	}
	if resp.Header.Get("Content-Type") != "application/yaml" {
		t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}

	// GET metadata
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/metadata/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET metadata status = %d", resp.StatusCode)
	}
	var stat fileMetadataDTO
	mustDecode(t, resp, &stat)
	if stat.Hash != put.Hash || stat.Path != "configs/app.yaml" {
		t.Errorf("metadata mismatch: %+v", stat)
	}

	// DELETE
	resp = doReq(t, mustReq(t, http.MethodDelete, srv.URL+"/api/v1/files/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE status = %d", resp.StatusCode)
	}

	// GET after delete → 404
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete status = %d, want 404", resp.StatusCode)
	}
}

func TestList(t *testing.T) {
	srv, store := newServer(t, nil)
	ctx := context.Background()
	for _, p := range []string{"a/1", "a/2", "b/1"} {
		if _, err := store.Put(ctx, internalfiles.FileMetadata{Path: p}, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatal(err)
		}
	}

	// All.
	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST status = %d (%s)", resp.StatusCode, readErr(resp))
	}
	var all listResponseDTO
	mustDecode(t, resp, &all)
	if len(all.Files) != 3 {
		t.Errorf("all = %d, want 3", len(all.Files))
	}

	// Prefix.
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files?prefix=a/", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST prefix status = %d", resp.StatusCode)
	}
	var subset listResponseDTO
	mustDecode(t, resp, &subset)
	if len(subset.Files) != 2 {
		t.Errorf("prefix = %d, want 2", len(subset.Files))
	}
}

func TestGet_MetadataRouteReturnsJSON(t *testing.T) {
	// The metadata route returns a JSON FileMetadata (not body
	// bytes). Confirms the more-specific
	// /api/v1/files/metadata/{path...} pattern wins over the
	// catch-all /api/v1/files/{path...} body route.
	srv, store := newServer(t, nil)
	ctx := context.Background()
	if _, err := store.Put(ctx, internalfiles.FileMetadata{Path: "configs/app.yaml"}, bytes.NewReader([]byte("body"))); err != nil {
		t.Fatal(err)
	}

	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/metadata/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var stat fileMetadataDTO
	mustDecode(t, resp, &stat)
	if stat.Path != "configs/app.yaml" {
		t.Errorf("stat.Path = %q, want configs/app.yaml", stat.Path)
	}
	if stat.Hash != internalfiles.HashOf([]byte("body")) {
		t.Errorf("stat.Hash mismatch")
	}
}

// --- error paths -------------------------------------------------------------

func TestGet_NotFound(t *testing.T) {
	srv, _ := newServer(t, nil)
	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/never/there", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	var e errorDTO
	mustDecode(t, resp, &e)
	if !strings.Contains(e.Error, "not found") {
		t.Errorf("error = %q", e.Error)
	}
}

func TestDelete_NotFound(t *testing.T) {
	srv, _ := newServer(t, nil)
	resp := doReq(t, mustReq(t, http.MethodDelete, srv.URL+"/api/v1/files/never/there", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestStat_NotFound(t *testing.T) {
	srv, _ := newServer(t, nil)
	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/metadata/never/there", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestPut_PathTraversal_NormalizedByGo documents that Go's HTTP
// stack normalizes ".." segments out of URLs before the router
// sees them. A request to /api/v1/files/foo/../bar lands at the
// route as path="bar" and succeeds. The path-validation 400
// branch is exercised by TestWriteBackendErr_PathValidation_Is400
// directly because crafting a malformed-after-normalization URL
// from net/http is not possible.
func TestPut_PathTraversal_NormalizedByGo(t *testing.T) {
	srv, _ := newServer(t, nil)
	req := mustReq(t, http.MethodPut, srv.URL+"/api/v1/files/foo/../bar", bytes.NewReader([]byte("x")))
	resp := doReq(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (%s); Go should have normalized the URL", resp.StatusCode, readErr(resp))
	}
	var put fileMetadataDTO
	mustDecode(t, resp, &put)
	if put.Path != "bar" {
		t.Errorf("path = %q, want bar (URL was normalized)", put.Path)
	}
}

func TestDisabled_Returns503(t *testing.T) {
	srv := newServerWithStore(t, nil, nil)
	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files", nil))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/x", nil))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("GET body status = %d, want 503", resp.StatusCode)
	}
	resp = doReq(t, mustReq(t, http.MethodPut, srv.URL+"/api/v1/files/x", bytes.NewReader([]byte("z"))))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("PUT status = %d, want 503", resp.StatusCode)
	}
	resp = doReq(t, mustReq(t, http.MethodDelete, srv.URL+"/api/v1/files/x", nil))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("DELETE status = %d, want 503", resp.StatusCode)
	}
	resp = doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/metadata/x", nil))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("STAT status = %d, want 503", resp.StatusCode)
	}
}

// --- ACL gating --------------------------------------------------------------

func TestACL_Deny(t *testing.T) {
	// Closed-by-default ACL with no rules → everything denied.
	a := acl.NewRoleACL()
	srv, _ := newServer(t, a)

	// We have not set a principal on the request — auth middleware
	// is upstream and we're talking direct to the handler. The
	// handler reads PrincipalFromContext which returns nil; the
	// closed-by-default RoleACL denies a nil principal.
	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/x", nil))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (%s)", resp.StatusCode, readErr(resp))
	}
}

func TestACL_Allow_WithPrincipalInContext(t *testing.T) {
	a := acl.NewRoleACL(acl.WithRule("configs", internalfiles.FileOpGet, auth.RoleReadonly))
	store := backend.NewMemoryStore(nil)
	if _, err := store.Put(context.Background(),
		internalfiles.FileMetadata{Path: "configs/app.yaml"},
		bytes.NewReader([]byte("v"))); err != nil {
		t.Fatal(err)
	}

	// Build a server that injects a Readonly principal into every
	// request context — simulating what auth middleware does.
	mux := http.NewServeMux()
	NewHandler(store, a, nil).Register(mux)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithPrincipal(r.Context(), &auth.Principal{ID: "u-1", Role: auth.RoleReadonly})
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	srv := httptest.NewServer(wrapped)
	defer srv.Close()

	resp := doReq(t, mustReq(t, http.MethodGet, srv.URL+"/api/v1/files/configs/app.yaml", nil))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("readonly+rule should allow, got %d (%s)", resp.StatusCode, readErr(resp))
	}

	// Readonly cannot put.
	resp = doReq(t, mustReq(t, http.MethodPut, srv.URL+"/api/v1/files/configs/other.yaml", bytes.NewReader([]byte("x"))))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("readonly put should deny, got %d", resp.StatusCode)
	}
}

func TestACL_NilACL_AllowsAll(t *testing.T) {
	srv, _ := newServer(t, nil)
	resp := doReq(t, mustReq(t, http.MethodPut, srv.URL+"/api/v1/files/whatever", bytes.NewReader([]byte("x"))))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("nil ACL should allow, got %d", resp.StatusCode)
	}
}

// --- misc --------------------------------------------------------------------

func TestNewHandler_NilLogger(t *testing.T) {
	h := NewHandler(backend.NewMemoryStore(nil), nil, nil)
	if h.logger == nil {
		t.Error("nil logger should map to slog.Default")
	}
}

func TestWriteBackendErr_GenericErrorIs500(t *testing.T) {
	h := NewHandler(backend.NewMemoryStore(nil), nil, nil)
	w := httptest.NewRecorder()
	h.writeBackendErr(w, errors.New("disk full"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestWriteBackendErr_PathValidation_Is400(t *testing.T) {
	h := NewHandler(backend.NewMemoryStore(nil), nil, nil)
	w := httptest.NewRecorder()
	h.writeBackendErr(w, errors.New("backend: path must not contain '..' segments"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestParseTagParams_MalformedDropped(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/x?tag=ok=yes&tag=bad-no-equals", nil)
	tags := parseTagParams(req)
	if tags["ok"] != "yes" {
		t.Errorf("good tag missing: %+v", tags)
	}
	if _, ok := tags["bad-no-equals"]; ok {
		t.Errorf("malformed tag should drop: %+v", tags)
	}
}

func TestParseTagParams_NoTags(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/x", nil)
	if tags := parseTagParams(req); tags != nil {
		t.Errorf("want nil, got %+v", tags)
	}
}

// --- helpers -----------------------------------------------------------------

func mustReq(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}

// fakeResp carries the parts of an *http.Response tests need
// (status, headers, buffered body) so doReq can fully close the
// real http.Response inside the helper and the bodyclose linter
// is satisfied at every call site.
type fakeResp struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// doReq sends req, drains + closes the response body, and returns
// the buffered parts tests need.
func doReq(t *testing.T, req *http.Request) *fakeResp {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return &fakeResp{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}
}

func mustDecode(t *testing.T, resp *fakeResp, into any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body, into); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, resp.Body)
	}
}

func readErr(resp *fakeResp) string {
	return fmt.Sprintf("body=%s", resp.Body)
}
