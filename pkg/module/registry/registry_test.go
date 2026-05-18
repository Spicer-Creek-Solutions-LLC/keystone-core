package registry_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/registry/storage"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
)

func newReg(t *testing.T) *registry.Registry {
	t.Helper()
	st, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return registry.New(st)
}

func manYAML(name, ver string, deps map[string]string) []byte {
	m := &manifest.Manifest{
		Name: name, Version: ver, Type: manifest.TypeStarlark,
		Entrypoint: "main.star", Dependencies: deps,
	}
	b, _ := manifest.MarshalManifest(m)
	return b
}

func publish(t *testing.T, r *registry.Registry, name, ver string, deps map[string]string) {
	t.Helper()
	if err := r.Publish(context.Background(), manYAML(name, ver, deps), []byte("ZIP:"+name+"@"+ver)); err != nil {
		t.Fatalf("Publish %s@%s: %v", name, ver, err)
	}
}

func TestPublishAndImmutability(t *testing.T) {
	r := newReg(t)
	publish(t, r, "acme/lib", "1.0.0", nil)

	// Re-publish same version → rejected.
	if err := r.Publish(context.Background(), manYAML("acme/lib", "1.0.0", nil), []byte("z")); !errors.Is(err, registry.ErrVersionExists) {
		t.Fatalf("re-publish = %v, want ErrVersionExists", err)
	}
	// Non-namespaced name → invalid (squatting guard via Validate).
	if err := r.Publish(context.Background(), manYAML("bare", "1.0.0", nil), []byte("z")); !errors.Is(err, registry.ErrInvalidModule) {
		t.Fatalf("non-namespaced = %v, want ErrInvalidModule", err)
	}
	// Garbage manifest / empty zip.
	if err := r.Publish(context.Background(), []byte("not: [yaml"), []byte("z")); !errors.Is(err, registry.ErrInvalidModule) {
		t.Fatalf("bad manifest = %v, want ErrInvalidModule", err)
	}
	if err := r.Publish(context.Background(), manYAML("acme/lib", "2.0.0", nil), nil); !errors.Is(err, registry.ErrInvalidModule) {
		t.Fatalf("empty zip = %v, want ErrInvalidModule", err)
	}
}

func TestRegistry_AsResolverSource(t *testing.T) {
	r := newReg(t)
	// app → lib (>=1.0.0); lib has 1.0.0 and 1.2.0 → MVS picks 1.0.0.
	publish(t, r, "acme/lib", "1.0.0", nil)
	publish(t, r, "acme/lib", "1.2.0", nil)

	var src resolver.Source = r // must satisfy the interface
	root := &manifest.Manifest{
		Name: "acme/app", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star", Dependencies: map[string]string{"acme/lib": ">=1.0.0"},
	}
	res, err := resolver.New(src, resolver.Config{}).Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve against registry: %v", err)
	}
	sel := res.Selected["acme/lib"]
	if sel.Version.String() != "1.0.0" {
		t.Fatalf("resolved acme/lib = %s, want 1.0.0 (MVS)", sel.Version)
	}
	lf, err := res.LockFile()
	if err != nil {
		t.Fatalf("LockFile: %v", err) // proves the registry's Hash is sha256:<hex>
	}
	if lf.Modules["acme/lib"].Version != "1.0.0" {
		t.Fatalf("lockfile = %+v", lf.Modules)
	}

	// Unknown module via Source → ErrNotFound.
	if _, err := r.ListVersions(context.Background(), "acme/nope"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("ListVersions(unknown) = %v, want ErrNotFound", err)
	}
	if !manifest.ValidModuleName("acme/lib") || manifest.ValidModuleName("bare") {
		t.Fatal("ValidModuleName sanity")
	}
}

func TestHTTPEndpoints(t *testing.T) {
	r := newReg(t)
	publish(t, r, "acme/lib", "1.0.0", nil)
	publish(t, r, "acme/lib", "1.2.0", map[string]string{"acme/dep": "^1.0.0"})

	mux := http.NewServeMux()
	registry.NewHandler(r).Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Returns (status, body); the body is read+closed internally so
	// no *http.Response escapes (bodyclose-clean, the kscore-cluster
	// handler-test precedent).
	get := func(p string) (int, string) {
		resp, err := srv.Client().Get(srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(b)
	}

	code, body := get("/acme/lib/@v/list")
	if code != 200 || !strings.Contains(body, "1.0.0") || !strings.Contains(body, "1.2.0") {
		t.Fatalf("list = %d %q", code, body)
	}

	code, body = get("/acme/lib/@v/1.2.0.info")
	if code != 200 {
		t.Fatalf("info = %d", code)
	}
	var info map[string]any
	_ = json.Unmarshal([]byte(body), &info)
	if info["Version"] != "1.2.0" || info["Time"] == nil {
		t.Fatalf("info body = %q", body)
	}
	if _, leaked := info["hash"]; leaked { // §4.18: only {Version,Time}
		t.Fatalf("info leaked internal hash: %q", body)
	}

	code, body = get("/acme/lib/@v/1.2.0.mod")
	if code != 200 || !strings.Contains(body, "acme/dep") {
		t.Fatalf("mod = %d %q", code, body)
	}

	code, body = get("/acme/lib/@v/1.0.0.zip")
	if code != 200 || body != "ZIP:acme/lib@1.0.0" {
		t.Fatalf("zip = %d %q", code, body)
	}

	// Errors.
	if code, _ := get("/acme/nope/@v/list"); code != http.StatusNotFound {
		t.Fatalf("unknown module = %d, want 404", code)
	}
	if code, _ := get("/acme/lib/@v/9.9.9.info"); code != http.StatusNotFound {
		t.Fatalf("unknown version = %d, want 404", code)
	}
	if code, _ := get("/noatsign"); code != http.StatusBadRequest {
		t.Fatalf("no /@v/ = %d, want 400", code)
	}
	if code, _ := get("/bare/@v/list"); code != http.StatusBadRequest {
		t.Fatalf("non-namespaced = %d, want 400", code)
	}
	if code, _ := get("/acme/lib/@v/1.0.0.tar"); code != http.StatusBadRequest {
		t.Fatalf("unknown action = %d, want 400", code)
	}
	// Traversal in the module segment is rejected by name validation.
	if code, _ := get("/../../etc/@v/list"); code != http.StatusBadRequest {
		t.Fatalf("traversal module = %d, want 400", code)
	}
}
