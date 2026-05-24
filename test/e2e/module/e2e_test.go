// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Package modulee2e is the Epic 14 task-17 end-to-end module-flow
// integration suite. It drives the real kscore-module CLI (wired
// exactly as cmd/kscore-module: registry HTTP client + the task-15
// moduletest runner) against a fake registry server — the task-9
// HTTP handler over a filesystem-backed storage served by
// httptest — exercising the full author lifecycle, dependency
// graphs over HTTP, end-to-end signature/hash failure, registry
// Go-mod protocol conformance, and post-install execution through
// the task-11 runtime.
//
// Build-tagged `integration` (run via `make test-integration`), so
// it is excluded from the default `go test ./...`.
package modulee2e

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	climod "go.keystone-core.io/keystone-core/internal/cli/module"
	"go.keystone-core.io/keystone-core/internal/registry/storage"
	starlarksdk "go.keystone-core.io/keystone-core/modules/sdk/starlark"
	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
	moduletest "go.keystone-core.io/keystone-core/pkg/module/testing"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

// --- harness --------------------------------------------------------------

func keypair(t *testing.T, dir, base string) (priv, pub string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk8, _ := x509.MarshalPKCS8PrivateKey(k)
	priv = filepath.Join(dir, base+".key")
	if err := os.WriteFile(priv, pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: pk8}), 0o600); err != nil {
		t.Fatal(err)
	}
	pkix, _ := x509.MarshalPKIXPublicKey(k.Public())
	pub = filepath.Join(dir, base+".pem")
	if err := os.WriteFile(pub, pem.EncodeToMemory(
		&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}), 0o600); err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func regServer(t *testing.T) (*httptest.Server, storage.Storage) {
	t.Helper()
	st, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registry.NewHandler(registry.New(st)).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

type e2eRunner struct{}

func (e2eRunner) RunTests(ctx context.Context, dir string, a climod.AuditOptions) (int, int, error) {
	rep, err := moduletest.Run(ctx, dir, moduletest.Options{
		Audit: moduletest.AuditOptions{Level: a.Level, Output: a.Output},
	})
	if err != nil {
		return 0, 0, err
	}
	return rep.Passed, rep.Failed, nil
}

func newDeps(srv *httptest.Server) climod.Deps {
	return climod.Deps{
		NewClient: func(string) climod.RegistryClient {
			return registry.NewClient(srv.URL, srv.Client())
		},
		TestRunner: e2eRunner{},
	}
}

func cli(t *testing.T, d climod.Deps, args ...string) (string, error) {
	t.Helper()
	cmd := climod.NewCommand(d)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	return func() (string, error) { e := cmd.Execute(); return out.String(), e }()
}

func mustCLI(t *testing.T, d climod.Deps, args ...string) string {
	t.Helper()
	out, err := cli(t, d, args...)
	if err != nil {
		t.Fatalf("kscore-module %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

type modSpec struct {
	name    string
	version string
	main    string
	test    string
	deps    map[string]string
}

func writeMod(t *testing.T, dir string, s modSpec) string {
	t.Helper()
	md := filepath.Join(dir, filepath.Base(s.name))
	if err := os.MkdirAll(md, 0o750); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Name: s.name, Version: s.version, Type: manifest.TypeStarlark,
		Entrypoint: "main.star", Description: "e2e fixture",
		License: "Apache-2.0", Dependencies: s.deps,
	}
	my, err := manifest.MarshalManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(md, "manifest.yaml"), my)
	write(t, filepath.Join(md, "main.star"), []byte(s.main))
	if s.test != "" {
		write(t, filepath.Join(md, "fix_test.star"), []byte(s.test))
	}
	return md
}

func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// buildSignPublish builds modDir → ZIP, optionally signs it with
// priv, and publishes it to the fake registry. Returns the ZIP path.
func buildSignPublish(t *testing.T, d climod.Deps, srv *httptest.Server, work, modDir, priv string, sign bool) string {
	t.Helper()
	zipPath := filepath.Join(work, filepath.Base(modDir)+".zip")
	mustCLI(t, d, "build", modDir, "-o", zipPath)
	if sign {
		mustCLI(t, d, "sign", zipPath, "--key", priv)
	}
	mustCLI(t, d, "publish", zipPath, "--registry", srv.URL)
	return zipPath
}

// execZip unzips a fetched module artifact and runs its entrypoint
// through the real Starlark runtime — proving the *distributed*
// artifact executes end-to-end.
func execZip(t *testing.T, zb []byte, input map[string]any) map[string]any {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = b
	}
	m, err := manifest.UnmarshalManifest(files["manifest.yaml"])
	if err != nil {
		t.Fatalf("artifact manifest: %v", err)
	}
	reg, err := capability.NewRegistryFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	inv := capability.NewInvoker(reg, maudit.NoopAuditor{})
	caps, err := capability.BuildCapabilities(m, capability.Hosts{})
	if err != nil {
		t.Fatal(err)
	}
	rt := srt.New(srt.Config{Builtins: starlarksdk.Provider(inv)})
	inst, err := rt.Init(context.Background(), m, files[m.Entrypoint], caps)
	if err != nil {
		t.Fatalf("runtime init of distributed artifact: %v", err)
	}
	defer func() { _ = inst.Close() }()
	res, err := inst.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute distributed artifact: %v", err)
	}
	return res.Output
}

const okMain = `def main(input):
    return {"echo": input.get("v", "")}
`

// --- scenarios ------------------------------------------------------------

func TestModuleE2E(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the cas cache root

	t.Run("single-module lifecycle + execute", func(t *testing.T) {
		work := t.TempDir()
		srv, _ := regServer(t)
		d := newDeps(srv)
		priv, pub := keypair(t, work, "k")

		// init scaffolds; then add a unit test.
		modDir := filepath.Join(work, "widget")
		mustCLI(t, d, "init", "e2e/widget", "--dir", modDir)
		write(t, filepath.Join(modDir, "widget_test.star"),
			[]byte("def test_ok():\n    assert.true(main({})[\"ok\"])\n"))

		mustCLI(t, d, "validate", modDir)
		if out := mustCLI(t, d, "test", modDir); !strings.Contains(out, "0 failed") {
			t.Fatalf("test: %s", out)
		}
		zipPath := buildSignPublish(t, d, srv, work, modDir, priv, true)
		mustCLI(t, d, "verify", zipPath, "--key", pub)

		inst := filepath.Join(work, "inst")
		if err := os.MkdirAll(inst, 0o750); err != nil {
			t.Fatal(err)
		}
		mustCLI(t, d, "install", "e2e/widget@0.1.0",
			"--registry", srv.URL, "--key", pub, "--dir", inst)
		lock1, err := os.ReadFile(filepath.Join(inst, "module.lock"))
		if err != nil {
			t.Fatalf("no module.lock: %v", err)
		}
		mustCLI(t, d, "install", "e2e/widget@0.1.0",
			"--registry", srv.URL, "--key", pub, "--dir", inst)
		lock2, _ := os.ReadFile(filepath.Join(inst, "module.lock"))
		if !bytes.Equal(lock1, lock2) {
			t.Fatal("install not reproducible")
		}

		// Execute the distributed artifact pulled from the registry.
		c := registry.NewClient(srv.URL, srv.Client())
		zb, err := c.FetchZip(context.Background(), "e2e/widget", semver.MustParse("0.1.0"))
		if err != nil {
			t.Fatal(err)
		}
		if out := execZip(t, zb, map[string]any{}); out["ok"] != true {
			t.Fatalf("distributed artifact output = %#v", out)
		}
	})

	t.Run("dependency graph over HTTP", func(t *testing.T) {
		work := t.TempDir()
		srv, _ := regServer(t)
		d := newDeps(srv)
		priv, pub := keypair(t, work, "k")

		la := writeMod(t, work, modSpec{name: "e2e/leafa", version: "1.0.0", main: okMain})
		lb := writeMod(t, work, modSpec{name: "e2e/leafb", version: "1.0.0", main: okMain})
		buildSignPublish(t, d, srv, work, la, priv, true)
		buildSignPublish(t, d, srv, work, lb, priv, true)

		root := writeMod(t, work, modSpec{
			name: "e2e/root", version: "1.0.0", main: okMain,
			deps: map[string]string{"e2e/leafa": ">=1.0.0", "e2e/leafb": ">=1.0.0"},
		})

		lockPath := filepath.Join(root, "module.lock")
		mustCLI(t, d, "resolve", root, "-o", lockPath, "--registry", srv.URL)
		tree := mustCLI(t, d, "tree", root, "--registry", srv.URL)
		for _, want := range []string{"e2e/leafa", "e2e/leafb"} {
			if !strings.Contains(tree, want) {
				t.Fatalf("tree missing %s:\n%s", want, tree)
			}
		}
		mustCLI(t, d, "install", root, "--registry", srv.URL, "--key", pub)
		lf, err := manifest.UnmarshalLockFile(readFile(t, lockPath))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"e2e/leafa", "e2e/leafb"} {
			if _, ok := lf.Modules[want]; !ok {
				t.Fatalf("lockfile missing %s: %+v", want, lf.Modules)
			}
		}
		// Reproducible re-resolve.
		mustCLI(t, d, "resolve", root, "-o", lockPath+".2", "--registry", srv.URL)
		if !bytes.Equal(readFile(t, lockPath), readFile(t, lockPath+".2")) {
			t.Fatal("resolve not reproducible")
		}
	})

	t.Run("signature + hash mismatch fail end-to-end", func(t *testing.T) {
		work := t.TempDir()
		srv, st := regServer(t)
		d := newDeps(srv)
		privA, _ := keypair(t, work, "a")
		_, pubB := keypair(t, work, "b")
		privA2, pubA2 := keypair(t, work, "a2")

		// (a) signed with A, install --key B → verification fails.
		ma := writeMod(t, work, modSpec{name: "e2e/siga", version: "1.0.0", main: okMain})
		buildSignPublish(t, d, srv, work, ma, privA, true)
		instA := filepath.Join(work, "ia")
		_ = os.MkdirAll(instA, 0o750)
		if _, err := cli(t, d, "install", "e2e/siga@1.0.0",
			"--registry", srv.URL, "--key", pubB, "--dir", instA); err == nil {
			t.Fatal("install with wrong key must fail")
		}
		if _, err := os.Stat(filepath.Join(instA, "module.lock")); err == nil {
			t.Fatal("no module.lock should be written on signature failure")
		}

		// (b) unsigned, install --key → fails (missing signature).
		mb := writeMod(t, work, modSpec{name: "e2e/sigb", version: "1.0.0", main: okMain})
		buildSignPublish(t, d, srv, work, mb, "", false)
		instB := filepath.Join(work, "ib")
		_ = os.MkdirAll(instB, 0o750)
		out, err := cli(t, d, "install", "e2e/sigb@1.0.0",
			"--registry", srv.URL, "--key", pubA2, "--dir", instB)
		if err == nil || !strings.Contains(out+errStr(err), "unsigned") {
			t.Fatalf("unsigned + --key must fail with 'unsigned': %v\n%s", err, out)
		}

		// (c) signed module whose stored ZIP is tampered after
		// publish → the signature was computed over the original
		// bytes, so `install --key` rejects the tampered artifact
		// (the real integrity guarantee: an unsigned registry that
		// serves self-consistent bytes+hash cannot be caught by the
		// hash gate alone — which is precisely why signatures
		// exist).
		mc := writeMod(t, work, modSpec{name: "e2e/sigc", version: "1.0.0", main: okMain})
		buildSignPublish(t, d, srv, work, mc, privA2, true)
		if err := st.Put(context.Background(),
			path.Join("e2e/sigc", "1.0.0", "module.zip"),
			bytes.NewReader([]byte("TAMPERED"))); err != nil {
			t.Fatal(err)
		}
		instC := filepath.Join(work, "ic")
		_ = os.MkdirAll(instC, 0o750)
		if _, err := cli(t, d, "install", "e2e/sigc@1.0.0",
			"--registry", srv.URL, "--key", pubA2, "--dir", instC); err == nil {
			t.Fatal("tampered signed artifact must fail signature verification")
		}
		if _, err := os.Stat(filepath.Join(instC, "module.lock")); err == nil {
			t.Fatal("no module.lock on tampered artifact")
		}
	})

	t.Run("registry Go-mod protocol conformance", func(t *testing.T) {
		work := t.TempDir()
		srv, _ := regServer(t)
		d := newDeps(srv)
		priv, _ := keypair(t, work, "k")

		mp := writeMod(t, work, modSpec{name: "e2e/proto", version: "1.0.0", main: okMain})
		zp := buildSignPublish(t, d, srv, work, mp, priv, true)

		// .info exposes only {Version, Time} (Hash stays internal).
		info := httpGet(t, srv, "/e2e/proto/@v/1.0.0.info")
		var meta map[string]any
		if err := json.Unmarshal(info, &meta); err != nil {
			t.Fatalf("info not JSON: %v (%s)", err, info)
		}
		if _, ok := meta["Version"]; !ok {
			t.Fatalf(".info missing Version: %s", info)
		}
		if _, leak := meta["Hash"]; leak {
			t.Fatalf(".info leaked Hash: %s", info)
		}

		// .sig present for a signed module.
		if code, _ := httpStatus(t, srv, "/e2e/proto/@v/1.0.0.sig"); code != http.StatusOK {
			t.Fatalf(".sig status = %d, want 200", code)
		}
		// .sig 404 for an unsigned module.
		mu := writeMod(t, work, modSpec{name: "e2e/unsig", version: "1.0.0", main: okMain})
		buildSignPublish(t, d, srv, work, mu, "", false)
		if code, _ := httpStatus(t, srv, "/e2e/unsig/@v/1.0.0.sig"); code != http.StatusNotFound {
			t.Fatalf("unsigned .sig status = %d, want 404", code)
		}

		// Re-publish the same version → conflict.
		if _, err := cli(t, d, "publish", zp, "--registry", srv.URL); err == nil {
			t.Fatal("re-publish of an existing version must conflict")
		}
	})
}

// --- small helpers --------------------------------------------------------

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func httpGet(t *testing.T, srv *httptest.Server, p string) []byte {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return b
}

func httpStatus(t *testing.T, srv *httptest.Server, p string) (int, []byte) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
