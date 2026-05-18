// Package examples_test drives every module in modules/examples
// through the author UX: manifest validation, the real kscore-module
// CLI (validate / build / test), unit tests via pkg/module/testing,
// and entrypoint execution through the task-11 runtime with the
// task-12 SDK (injecting fake hosts for the side-effecting
// capabilities). The opsbundle case additionally exercises
// dependency resolution against an in-process registry.
//
// Scope: the sign/publish/install registry round-trip is covered by
// the task-14 e2e and the task-17 generic fake-registry e2e; this
// test proves each shipped example is valid, test-passing, and
// executable, plus the resolve/lockfile UX for opsbundle.
package examples_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/module"
	"go.keystone-core.io/keystone-core/internal/registry/storage"
	starlarksdk "go.keystone-core.io/keystone-core/modules/sdk/starlark"
	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
	moduletest "go.keystone-core.io/keystone-core/pkg/module/testing"
)

var allExamples = []string{
	"hello", "kvcache", "fsreport", "httpfetch",
	"cmdrun", "secretsync", "opsbundle",
}

func exampleDir(name string) string { return name }

func loadExample(t *testing.T, name string) (*manifest.Manifest, []byte) {
	t.Helper()
	dir := exampleDir(name)
	my, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	if err != nil {
		t.Fatalf("%s: read manifest: %v", name, err)
	}
	m, err := manifest.UnmarshalManifest(my)
	if err != nil {
		t.Fatalf("%s: unmarshal manifest: %v", name, err)
	}
	src, err := os.ReadFile(filepath.Join(dir, m.Entrypoint))
	if err != nil {
		t.Fatalf("%s: read entrypoint: %v", name, err)
	}
	return m, src
}

// --- fake capability hosts ------------------------------------------------

type osFS struct{}

func (osFS) ReadFile(p string) ([]byte, error) {
	return os.ReadFile(p) //nolint:gosec // G304: path is capability-scoped by the manifest
}

func (osFS) WriteFile(p string, d []byte, perm uint32) error {
	return os.WriteFile(p, d, os.FileMode(perm)) //nolint:gosec // G306: perm from the scoped capability
}

func (osFS) Stat(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

type discardLog struct{}

func (discardLog) Log(_, _ string, _ map[string]string) {}

type fakeHTTP struct{}

func (fakeHTTP) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     make(http.Header),
	}, nil
}

type fakeExec struct{}

func (fakeExec) Run(_ context.Context, _, name string, args []string) ([]byte, []byte, error) {
	return []byte(name + " " + strings.Join(args, " ")), nil, nil
}

type fakeSecrets struct {
	mu   sync.Mutex
	data map[string]map[string]any
}

func (f *fakeSecrets) Get(_ context.Context, path string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make(map[string]any, len(v))
	for k, val := range v {
		out[k] = val
	}
	return out, nil
}

func (f *fakeSecrets) Set(_ context.Context, path string, data map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data == nil {
		f.data = map[string]map[string]any{}
	}
	f.data[path] = data
	return nil
}

type capAudit struct {
	mu      sync.Mutex
	entries []maudit.Entry
}

func (c *capAudit) Emit(_ context.Context, e maudit.Entry) {
	c.mu.Lock()
	c.entries = append(c.entries, e)
	c.mu.Unlock()
}

func (c *capAudit) successes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.entries {
		if e.Success {
			n++
		}
	}
	return n
}

// exec builds caps + invoker with hosts, runs the entrypoint.
func execModule(
	t *testing.T, m *manifest.Manifest, src []byte,
	hosts capability.Hosts, input map[string]any,
) (map[string]any, *capAudit, error) {
	t.Helper()
	ctx := context.Background()
	reg, err := capability.NewRegistryFromManifest(m)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	aud := &capAudit{}
	inv := capability.NewInvoker(reg, aud)
	caps, err := capability.BuildCapabilities(m, hosts)
	if err != nil {
		t.Fatalf("build capabilities: %v", err)
	}
	rt := srt.New(srt.Config{Builtins: starlarksdk.Provider(inv)})
	inst, err := rt.Init(ctx, m, src, caps)
	if err != nil {
		t.Fatalf("runtime init: %v", err)
	}
	defer func() { _ = inst.Close() }()
	res, err := inst.Execute(ctx, input)
	if err != nil {
		return nil, aud, err
	}
	return res.Output, aud, nil
}

// --- CLI author flow ------------------------------------------------------

type cliRunner struct{}

func (cliRunner) RunTests(ctx context.Context, dir string, a module.AuditOptions) (int, int, error) {
	rep, err := moduletest.Run(ctx, dir, moduletest.Options{
		Audit: moduletest.AuditOptions{Level: a.Level, Output: a.Output},
	})
	if err != nil {
		return 0, 0, err
	}
	return rep.Passed, rep.Failed, nil
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := module.NewCommand(module.Deps{TestRunner: cliRunner{}})
	var out bytes.Buffer
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

// --- table-driven baseline: validate + CLI + unit tests -------------------

func TestExamples_AuthorUX(t *testing.T) {
	for _, name := range allExamples {
		t.Run(name, func(t *testing.T) {
			dir := exampleDir(name)
			m, _ := loadExample(t, name)
			if err := m.Validate(); err != nil {
				t.Fatalf("manifest.Validate: %v", err)
			}

			if out, err := runCLI(t, "validate", dir); err != nil {
				t.Fatalf("cli validate: %v (%s)", err, out)
			} else if !strings.Contains(out, "ok: "+m.Name) {
				t.Fatalf("cli validate output = %q", out)
			}

			zip := filepath.Join(t.TempDir(), "m.zip")
			if out, err := runCLI(t, "build", dir, "-o", zip); err != nil {
				t.Fatalf("cli build: %v (%s)", err, out)
			}
			if fi, err := os.Stat(zip); err != nil || fi.Size() == 0 {
				t.Fatalf("build produced no zip: %v", err)
			}

			if out, err := runCLI(t, "test", dir); err != nil {
				t.Fatalf("cli test: %v (%s)", err, out)
			} else if !strings.Contains(out, "0 failed") {
				t.Fatalf("cli test output = %q", out)
			}

			rep, err := moduletest.Run(context.Background(), dir, moduletest.Options{})
			if err != nil {
				t.Fatalf("moduletest.Run: %v", err)
			}
			if rep.Failed != 0 || rep.Passed == 0 {
				t.Fatalf("%s tests: %d passed, %d failed: %+v",
					name, rep.Passed, rep.Failed, rep.Results)
			}
		})
	}
}

// --- per-capability execution --------------------------------------------

func TestExample_Hello_Execute(t *testing.T) {
	m, src := loadExample(t, "hello")
	out, _, err := execModule(t, m, src, capability.Hosts{}, map[string]any{"name": "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if out["message"] != "hello, ops!" {
		t.Fatalf("output = %#v", out)
	}
}

func TestExample_KVCache_Execute(t *testing.T) {
	m, src := loadExample(t, "kvcache")
	hosts := capability.Hosts{Logger: discardLog{}}
	out, aud, err := execModule(t, m, src, hosts,
		map[string]any{"op": "incr", "namespace": "n", "key": "h"})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["value"]; got != int64(1) {
		t.Fatalf("value = %#v (%T), want 1", got, got)
	}
	if aud.successes() == 0 {
		t.Fatal("expected audited successful kv/log invocations")
	}
}

func TestExample_FSReport_Execute(t *testing.T) {
	m, src := loadExample(t, "fsreport")
	tmp := t.TempDir()
	sb := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(sb, 0o750); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(sb, "in.txt")
	dst := filepath.Join(sb, "out.txt")
	if err := os.WriteFile(in, []byte("a\n\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := execModule(t, m, src, capability.Hosts{FS: osFS{}},
		map[string]any{"src": in, "dst": dst})
	if err != nil {
		t.Fatalf("scoped read/write should succeed: %v", err)
	}
	if out["lines"] == nil {
		t.Fatalf("summary = %#v", out)
	}
	b, err := os.ReadFile(dst) //nolint:gosec // G304: test-controlled temp path
	if err != nil || !strings.HasPrefix(string(b), "lines=") {
		t.Fatalf("report not written: %q (%v)", b, err)
	}

	// Out-of-scope write is denied even with an os FS host wired.
	if _, _, err := execModule(t, m, src, capability.Hosts{FS: osFS{}},
		map[string]any{"src": in, "dst": "/etc/keystone-denied.txt"}); err == nil {
		t.Fatal("write outside the manifest scope must fail")
	}
}

func TestExample_HTTPFetch_Execute(t *testing.T) {
	m, src := loadExample(t, "httpfetch")
	hosts := capability.Hosts{HTTP: fakeHTTP{}}
	out, _, err := execModule(t, m, src, hosts,
		map[string]any{"url": "https://api.example.com/health"})
	if err != nil {
		t.Fatalf("allowed-domain fetch failed: %v", err)
	}
	if out["status"] != int64(200) {
		t.Fatalf("status = %#v", out["status"])
	}
	// Denied domain is rejected before the host is reached.
	if _, _, err := execModule(t, m, src, hosts,
		map[string]any{"url": "https://evil.example.org/x"}); err == nil {
		t.Fatal("non-allowlisted domain must fail")
	}
}

func TestExample_CmdRun_Execute(t *testing.T) {
	m, src := loadExample(t, "cmdrun")
	hosts := capability.Hosts{Exec: fakeExec{}}
	out, _, err := execModule(t, m, src, hosts,
		map[string]any{"cmd": "echo", "args": []any{"hi"}})
	if err != nil {
		t.Fatalf("allowlisted command failed: %v", err)
	}
	if !strings.Contains(toString(out["stdout"]), "echo hi") {
		t.Fatalf("stdout = %#v", out["stdout"])
	}
	if _, _, err := execModule(t, m, src, hosts,
		map[string]any{"cmd": "rm", "args": []any{"-rf", "/"}}); err == nil {
		t.Fatal("non-allowlisted command must fail")
	}
}

func TestExample_SecretSync_Execute(t *testing.T) {
	m, src := loadExample(t, "secretsync")
	fs := &fakeSecrets{data: map[string]map[string]any{
		"app/source/db": {"user": "db", "pass": "s3cr3t"},
	}}
	hosts := capability.Hosts{Secrets: fs, Logger: discardLog{}}
	_, _, err := execModule(t, m, src, hosts,
		map[string]any{"src": "app/source/db", "dst": "app/dest/db"})
	if err != nil {
		t.Fatalf("scoped sync failed: %v", err)
	}
	if got := fs.data["app/dest/db"]; got == nil || got["rotated"] != "true" {
		t.Fatalf("dest not written with rotation marker: %#v", fs.data["app/dest/db"])
	}
	// Reading the write-scope path is denied (read scope is app/source/*).
	if _, _, err := execModule(t, m, src, hosts,
		map[string]any{"src": "app/dest/db", "dst": "app/dest/x"}); err == nil {
		t.Fatal("reading outside the read scope must fail")
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// zipDir packages a module dir's manifest + entrypoint into a ZIP
// (the registry only requires a non-empty archive; the manifest is
// passed to Publish separately).
func zipDir(t *testing.T, dir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"manifest.yaml", "main.star"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("zip read %s: %v", name, err)
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// --- opsbundle: dependency resolution ------------------------------------

func TestExample_OpsBundle_Resolve(t *testing.T) {
	ctx := context.Background()
	st, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)

	publish := func(name string) {
		dir := exampleDir(name)
		my, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := reg.Publish(ctx, my, zipDir(t, dir)); err != nil {
			t.Fatalf("publish %s: %v", name, err)
		}
	}
	publish("httpfetch")
	publish("cmdrun")

	ops, _ := loadExample(t, "opsbundle")
	res, err := resolver.New(reg, resolver.Config{}).Resolve(ctx, ops)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	lf, err := res.LockFile()
	if err != nil {
		t.Fatalf("lockfile: %v", err)
	}
	for _, want := range []string{"keystone/httpfetch", "keystone/cmdrun"} {
		lm, ok := lf.Modules[want]
		if !ok {
			t.Fatalf("lockfile missing %s: %+v", want, lf.Modules)
		}
		if lm.Version != "1.0.0" {
			t.Fatalf("%s pinned to %s, want 1.0.0", want, lm.Version)
		}
	}

	// Reproducible: re-resolving yields the identical lockfile bytes.
	res2, _ := resolver.New(reg, resolver.Config{}).Resolve(ctx, ops)
	lf2, _ := res2.LockFile()
	b1, _ := manifest.MarshalLockFile(lf)
	b2, _ := manifest.MarshalLockFile(lf2)
	if !bytes.Equal(b1, b2) {
		t.Fatal("lockfile is not reproducible")
	}
}
