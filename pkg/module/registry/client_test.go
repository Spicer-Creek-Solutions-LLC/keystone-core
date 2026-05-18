package registry_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.keystone-core.io/keystone-core/internal/registry/storage"
	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

func clientServer(t *testing.T) (*registry.Client, *registry.Registry) {
	t.Helper()
	st, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	mux := http.NewServeMux()
	registry.NewHandler(reg).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return registry.NewClient(srv.URL, srv.Client()), reg
}

func TestClient_PublishFetchResolveAndSignature(t *testing.T) {
	c, _ := clientServer(t)
	ctx := context.Background()
	man := manYAML("acme/lib", "1.0.0", nil)
	zip := []byte("ZIPBYTES")

	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sig, _ := verify.Sign(zip, k)
	sigBytes, _ := verify.MarshalSignature(sig)

	if err := c.Publish(ctx, man, zip, sigBytes); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// Re-publish → conflict.
	if err := c.Publish(ctx, man, zip, sigBytes); err == nil {
		t.Fatal("re-publish should conflict")
	}

	// Client satisfies resolver.Source end-to-end.
	var src resolver.Source = c
	vs, err := src.ListVersions(ctx, "acme/lib")
	if err != nil || len(vs) != 1 || vs[0].Version.String() != "1.0.0" {
		t.Fatalf("ListVersions = %+v, %v", vs, err)
	}
	if vs[0].Hash != cas.HashBytes(zip) {
		t.Fatalf("hash = %q, want %q", vs[0].Hash, cas.HashBytes(zip))
	}
	gm, err := src.GetManifest(ctx, "acme/lib", semver.MustParse("1.0.0"))
	if err != nil || gm.Name != "acme/lib" {
		t.Fatalf("GetManifest = %+v, %v", gm, err)
	}

	gz, err := c.FetchZip(ctx, "acme/lib", semver.MustParse("1.0.0"))
	if err != nil || !bytes.Equal(gz, zip) {
		t.Fatalf("FetchZip = %q, %v", gz, err)
	}
	gs, ok, err := c.FetchSignature(ctx, "acme/lib", semver.MustParse("1.0.0"))
	if err != nil || !ok || gs.KeyID != sig.KeyID {
		t.Fatalf("FetchSignature = %+v ok=%v err=%v", gs, ok, err)
	}

	// Unknown module + unsigned-version signature absence.
	if _, err := c.ListVersions(ctx, "acme/missing"); err == nil {
		t.Fatal("ListVersions(missing): want error")
	}
	if err := c.Publish(ctx, manYAML("acme/unsigned", "1.0.0", nil), []byte("Z"), nil); err != nil {
		t.Fatalf("publish unsigned: %v", err)
	}
	if _, ok, err := c.FetchSignature(ctx, "acme/unsigned", semver.MustParse("1.0.0")); ok || err != nil {
		t.Fatalf("unsigned FetchSignature: ok=%v err=%v, want false,nil", ok, err)
	}
}
