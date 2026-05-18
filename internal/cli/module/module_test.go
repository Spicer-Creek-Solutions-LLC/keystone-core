package module_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	climod "go.keystone-core.io/keystone-core/internal/cli/module"
	"go.keystone-core.io/keystone-core/internal/registry/storage"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
)

// keypair writes a PKCS8 private + PKIX public PEM and returns paths.
func keypair(t *testing.T, dir string) (priv, pub string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk8, _ := x509.MarshalPKCS8PrivateKey(k)
	priv = filepath.Join(dir, "local.key")
	if err := os.WriteFile(priv, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8}), 0o600); err != nil {
		t.Fatal(err)
	}
	pkix, _ := x509.MarshalPKIXPublicKey(k.Public())
	pub = filepath.Join(dir, "pub.pem")
	if err := os.WriteFile(pub, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}), 0o600); err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func regServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registry.NewHandler(registry.New(st)).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, d climod.Deps, args ...string) (string, error) {
	t.Helper()
	cmd := climod.NewCommand(d)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return out.String(), err
}

func TestAuthorFlow_EndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the cas cache root

	work := t.TempDir()
	priv, pub := keypair(t, work)
	srv := regServer(t)
	deps := climod.Deps{
		NewClient: func(string) climod.RegistryClient {
			return registry.NewClient(srv.URL, srv.Client())
		},
	}

	modDir := filepath.Join(work, "widget")
	zipPath := filepath.Join(work, "widget.zip")

	// init → validate → build → sign → verify
	if _, err := run(t, deps, "init", "acme/widget", "--dir", modDir); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modDir, "manifest.yaml")); err != nil {
		t.Fatalf("init scaffold missing manifest: %v", err)
	}
	if out, err := run(t, deps, "validate", modDir); err != nil {
		t.Fatalf("validate: %v (%s)", err, out)
	}
	if _, err := run(t, deps, "build", modDir, "-o", zipPath); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := run(t, deps, "sign", zipPath, "--key", priv); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := os.Stat(zipPath + ".sig"); err != nil {
		t.Fatalf("sign produced no .sig: %v", err)
	}
	if out, err := run(t, deps, "verify", zipPath, "--key", pub); err != nil {
		t.Fatalf("verify: %v (%s)", err, out)
	}

	// publish → install (resolve + fetch + hash + signature + lock)
	if out, err := run(t, deps, "publish", zipPath, "--registry", srv.URL); err != nil {
		t.Fatalf("publish: %v (%s)", err, out)
	}
	instDir := filepath.Join(work, "inst")
	if err := os.MkdirAll(instDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, deps, "install", "acme/widget@0.1.0",
		"--registry", srv.URL, "--key", pub, "--dir", instDir); err != nil {
		t.Fatalf("install: %v (%s)", err, out)
	}
	lock1, err := os.ReadFile(filepath.Join(instDir, "module.lock"))
	if err != nil {
		t.Fatalf("module.lock not written: %v", err)
	}
	// Reproducible: a second install yields a byte-identical lock.
	if _, err := run(t, deps, "install", "acme/widget@0.1.0",
		"--registry", srv.URL, "--key", pub, "--dir", instDir); err != nil {
		t.Fatalf("install #2: %v", err)
	}
	lock2, _ := os.ReadFile(filepath.Join(instDir, "module.lock"))
	if !bytes.Equal(lock1, lock2) {
		t.Fatalf("install not reproducible:\n%s\n---\n%s", lock1, lock2)
	}

	// resolve + tree against the published module.
	if out, err := run(t, deps, "tree", modDir, "--registry", srv.URL); err != nil {
		t.Fatalf("tree: %v (%s)", err, out)
	}

	// Re-publish the same version → conflict.
	if _, err := run(t, deps, "publish", zipPath, "--registry", srv.URL); err == nil {
		t.Fatal("re-publish should fail (version exists)")
	}
}

func TestVerify_TamperFails(t *testing.T) {
	work := t.TempDir()
	priv, pub := keypair(t, work)
	deps := climod.Deps{}

	modDir := filepath.Join(work, "m")
	zp := filepath.Join(work, "m.zip")
	mustRun(t, deps, "init", "acme/m", "--dir", modDir)
	mustRun(t, deps, "build", modDir, "-o", zp)
	mustRun(t, deps, "sign", zp, "--key", priv)

	// Tamper the zip after signing → verify must fail (the
	// "signature mismatch causes load to fail" acceptance line).
	if err := os.WriteFile(zp, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, deps, "verify", zp, "--key", pub); err == nil {
		t.Fatal("verify of a tampered zip must fail")
	}
}

func TestInit_Guards(t *testing.T) {
	deps := climod.Deps{}
	if _, err := run(t, deps, "init", "notnamespaced"); err == nil {
		t.Fatal("non-namespaced init must fail")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, deps, "init", "acme/x", "--dir", dir); err == nil {
		t.Fatal("init into a non-empty dir must fail")
	}
}

type fakeRunner struct {
	passed, failed int
	err            error
}

func (f fakeRunner) RunTests(context.Context, string, climod.AuditOptions) (int, int, error) {
	return f.passed, f.failed, f.err
}

func TestTest_SeamToTask15(t *testing.T) {
	// Default runner → the pending-framework error.
	if _, err := run(t, climod.Deps{}, "test", t.TempDir()); !errors.Is(err, climod.ErrTestFrameworkPending) {
		t.Fatalf("default test = %v, want ErrTestFrameworkPending", err)
	}
	// Injected runner → success.
	out, err := run(t, climod.Deps{TestRunner: fakeRunner{passed: 3}},
		"test", t.TempDir())
	if err != nil || !contains(out, "3 passed") {
		t.Fatalf("seam runner = %q, %v", out, err)
	}
	// Failing tests → non-zero.
	if _, err := run(t, climod.Deps{TestRunner: fakeRunner{failed: 2}},
		"test", t.TempDir()); err == nil {
		t.Fatal("failing tests must return an error")
	}
}

func mustRun(t *testing.T, d climod.Deps, args ...string) {
	t.Helper()
	if out, err := run(t, d, args...); err != nil {
		t.Fatalf("%v: %v (%s)", args, err, out)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
