package capability_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// ---- fake hosts ---------------------------------------------------------

type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemFS() *memFS { return &memFS{files: map[string][]byte{}} }

func (m *memFS) ReadFile(p string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[p]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}
func (m *memFS) WriteFile(p string, d []byte, _ uint32) error {
	m.mu.Lock()
	m.files[p] = d
	m.mu.Unlock()
	return nil
}
func (m *memFS) Stat(p string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[p]
	if !ok {
		return 0, errors.New("not found")
	}
	return int64(len(b)), nil
}

type fakeExec struct{ ran string }

func (f *fakeExec) Run(_ context.Context, _, name string, _ []string) ([]byte, []byte, error) {
	f.ran = name
	return []byte("ok"), nil, nil
}

type fakeSecrets struct{ store map[string]map[string]any }

func (f *fakeSecrets) Get(_ context.Context, p string) (map[string]any, error) {
	v, ok := f.store[p]
	if !ok {
		return nil, errors.New("no secret")
	}
	return v, nil
}
func (f *fakeSecrets) Set(_ context.Context, p string, d map[string]any) error {
	f.store[p] = d
	return nil
}

// ---- fs.read / fs.write -------------------------------------------------

func TestFS_Scope(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/apt/sources.list"] = []byte("deb ...")
	fs.files["/etc/shadow"] = []byte("root:...")

	rd, err := capability.NewFSRead(manifest.CapabilityConfig{
		Paths: []string{"/etc/apt/**"}, DeniedPaths: []string{"/etc/apt/secret/**"},
	}, fs)
	if err != nil {
		t.Fatalf("NewFSRead: %v", err)
	}
	if _, err := rd.Read("/etc/apt/sources.list"); err != nil {
		t.Fatalf("in-scope read: %v", err)
	}
	if _, err := rd.Read("/etc/shadow"); !errors.Is(err, capability.ErrPathDenied) {
		t.Fatalf("out-of-scope read = %v, want ErrPathDenied", err)
	}
	if _, err := rd.Read("/etc/apt/../shadow"); !errors.Is(err, capability.ErrPathDenied) {
		t.Fatalf("traversal escape not blocked: %v", err)
	}

	wr, err := capability.NewFSWrite(manifest.CapabilityConfig{
		Paths: []string{"/etc/apt/sources.list.d/**"}, MaxFileSize: "16",
	}, fs)
	if err != nil {
		t.Fatalf("NewFSWrite: %v", err)
	}
	if err := wr.Write("/etc/apt/sources.list.d/x.list", []byte("deb x"), 0o644); err != nil {
		t.Fatalf("in-scope write: %v", err)
	}
	if err := wr.Write("/etc/apt/sources.list.d/x.list", []byte(strings.Repeat("z", 99)), 0o644); !errors.Is(err, capability.ErrSizeExceeded) {
		t.Fatalf("oversize write = %v, want ErrSizeExceeded", err)
	}
	if err := wr.Write("/tmp/evil", []byte("x"), 0o644); !errors.Is(err, capability.ErrPathDenied) {
		t.Fatalf("out-of-scope write = %v, want ErrPathDenied", err)
	}
}

func TestFS_HostUnavailableAndBadGlob(t *testing.T) {
	rd, _ := capability.NewFSRead(manifest.CapabilityConfig{Paths: []string{"/x/**"}}, nil)
	if _, err := rd.Read("/x/y"); !errors.Is(err, capability.ErrHostUnavailable) {
		t.Fatalf("nil host = %v, want ErrHostUnavailable", err)
	}
	if _, err := capability.NewFSRead(manifest.CapabilityConfig{Paths: []string{"[bad"}}, newMemFS()); err == nil {
		t.Fatal("bad glob: want construction error")
	}
	if _, err := capability.NewFSRead(manifest.CapabilityConfig{MaxFileSize: "nope"}, newMemFS()); err == nil {
		t.Fatal("bad max_file_size: want construction error")
	}
}

// ---- http.get / http.post ----------------------------------------------

func TestHTTP_Scope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello-body")
	}))
	defer srv.Close()

	// Allow the test server's own host so the request reaches it.
	host := strings.TrimPrefix(srv.URL, "http://")
	hostOnly := strings.Split(host, ":")[0] // 127.0.0.1

	get, err := capability.NewHTTPGet(manifest.CapabilityConfig{
		Domains: []string{hostOnly}, MaxResponseSize: "1KB", Timeout: "5s",
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewHTTPGet: %v", err)
	}
	body, code, err := get.Call(context.Background(), srv.URL, nil)
	if err != nil || code != 200 || string(body) != "hello-body" {
		t.Fatalf("in-scope GET = %q,%d,%v", body, code, err)
	}

	// Domain not allowed.
	deny, _ := capability.NewHTTPGet(manifest.CapabilityConfig{Domains: []string{"example.com"}}, srv.Client())
	if _, _, err := deny.Call(context.Background(), srv.URL, nil); !errors.Is(err, capability.ErrDomainDenied) {
		t.Fatalf("disallowed domain = %v, want ErrDomainDenied", err)
	}

	// Response over limit.
	tiny, _ := capability.NewHTTPGet(manifest.CapabilityConfig{Domains: []string{hostOnly}, MaxResponseSize: "3"}, srv.Client())
	if _, _, err := tiny.Call(context.Background(), srv.URL, nil); !errors.Is(err, capability.ErrSizeExceeded) {
		t.Fatalf("oversize resp = %v, want ErrSizeExceeded", err)
	}

	// Request body over limit (POST).
	post, _ := capability.NewHTTPPost(manifest.CapabilityConfig{Domains: []string{hostOnly}, MaxRequestSize: "2"}, srv.Client())
	if _, _, err := post.Call(context.Background(), srv.URL, []byte("toolong")); !errors.Is(err, capability.ErrSizeExceeded) {
		t.Fatalf("oversize req = %v, want ErrSizeExceeded", err)
	}

	// Rate limit.
	rl, _ := capability.NewHTTPGet(manifest.CapabilityConfig{Domains: []string{hostOnly}, RateLimit: "1/h"}, srv.Client())
	if _, _, err := rl.Call(context.Background(), srv.URL, nil); err != nil {
		t.Fatalf("first rate-limited call: %v", err)
	}
	if _, _, err := rl.Call(context.Background(), srv.URL, nil); !errors.Is(err, capability.ErrRateLimited) {
		t.Fatalf("second call = %v, want ErrRateLimited", err)
	}
}

// ---- exec ---------------------------------------------------------------

func TestExec_Scope(t *testing.T) {
	fe := &fakeExec{}
	e, err := capability.NewExec(manifest.CapabilityConfig{
		Commands: []string{"apt-get"}, WorkingDir: "/tmp", Timeout: "30s",
	}, fe)
	if err != nil {
		t.Fatalf("NewExec: %v", err)
	}
	if _, _, err := e.Run(context.Background(), "apt-get", []string{"update"}); err != nil {
		t.Fatalf("allowed cmd: %v", err)
	}
	if _, _, err := e.Run(context.Background(), "/usr/bin/apt-get", nil); err != nil {
		t.Fatalf("allowed cmd by base: %v", err)
	}
	if _, _, err := e.Run(context.Background(), "rm", []string{"-rf", "/"}); !errors.Is(err, capability.ErrCommandDenied) {
		t.Fatalf("disallowed cmd = %v, want ErrCommandDenied", err)
	}
	if _, err := capability.NewExec(manifest.CapabilityConfig{Timeout: "soon"}, fe); err == nil {
		t.Fatal("bad timeout: want construction error")
	}
}

// Phase B5 finding M2: codify the allowlist semantics explicitly
// (per the doc-comment on Exec.Run).
//
//   - bare allowlist entry ("apt-get") → matches anywhere-apt-get
//   - absolute-path allowlist entry ("/usr/bin/apt-get") → matches
//     only that exact path; DOES NOT match "/tmp/apt-get"
//
// Operators wanting strict path containment must use absolute
// paths in the manifest.
func TestExec_AllowlistSemantics(t *testing.T) {
	t.Run("bare entry matches anywhere", func(t *testing.T) {
		e, err := capability.NewExec(manifest.CapabilityConfig{
			Commands: []string{"apt-get"},
		}, &fakeExec{})
		if err != nil {
			t.Fatalf("NewExec: %v", err)
		}
		for _, name := range []string{"apt-get", "/usr/bin/apt-get", "/tmp/apt-get", "/home/anywhere/apt-get"} {
			if _, _, err := e.Run(context.Background(), name, nil); err != nil {
				t.Errorf("bare allowlist + name=%q: unexpected deny %v", name, err)
			}
		}
	})
	t.Run("absolute-path entry matches only exact path", func(t *testing.T) {
		e, err := capability.NewExec(manifest.CapabilityConfig{
			Commands: []string{"/usr/bin/apt-get"},
		}, &fakeExec{})
		if err != nil {
			t.Fatalf("NewExec: %v", err)
		}
		if _, _, err := e.Run(context.Background(), "/usr/bin/apt-get", nil); err != nil {
			t.Errorf("abs-path allowlist + exact-match: unexpected deny %v", err)
		}
		for _, name := range []string{"apt-get", "/tmp/apt-get", "/home/anywhere/apt-get", "/usr/local/bin/apt-get"} {
			if _, _, err := e.Run(context.Background(), name, nil); !errors.Is(err, capability.ErrCommandDenied) {
				t.Errorf("abs-path allowlist + name=%q: want ErrCommandDenied, got %v", name, err)
			}
		}
	})
}

// ---- secrets.read / secrets.write --------------------------------------

func TestSecrets_Scope(t *testing.T) {
	fs := &fakeSecrets{store: map[string]map[string]any{
		"kv/data/app/db": {"password": "s3cr3t"},
	}}
	rd, err := capability.NewSecretsRead(manifest.CapabilityConfig{
		SecretPaths: []string{"kv/data/app/**"},
	}, fs)
	if err != nil {
		t.Fatalf("NewSecretsRead: %v", err)
	}
	if _, err := rd.Get(context.Background(), "kv/data/app/db"); err != nil {
		t.Fatalf("in-scope secret get: %v", err)
	}
	if _, err := rd.Get(context.Background(), "kv/data/other/x"); !errors.Is(err, capability.ErrSecretPathDenied) {
		t.Fatalf("out-of-scope get = %v, want ErrSecretPathDenied", err)
	}

	wr, _ := capability.NewSecretsWrite(manifest.CapabilityConfig{SecretPaths: []string{"kv/data/app/**"}}, fs)
	if err := wr.Set(context.Background(), "kv/data/app/new", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("in-scope set: %v", err)
	}
	if err := wr.Set(context.Background(), "kv/data/root", map[string]any{"k": "v"}); !errors.Is(err, capability.ErrSecretPathDenied) {
		t.Fatalf("out-of-scope set = %v, want ErrSecretPathDenied", err)
	}
}

// ---- kv -----------------------------------------------------------------

func TestKV(t *testing.T) {
	kv, err := capability.NewKV(manifest.CapabilityConfig{MaxFileSize: "2"})
	if err != nil {
		t.Fatalf("NewKV: %v", err)
	}
	if err := kv.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	_ = kv.Set("b", "2")
	if err := kv.Set("c", "3"); !errors.Is(err, capability.ErrSizeExceeded) {
		t.Fatalf("over key cap = %v, want ErrSizeExceeded", err)
	}
	if err := kv.Set("a", "updated"); err != nil { // updating existing is fine
		t.Fatalf("update existing over-cap: %v", err)
	}
	if v, ok := kv.Get("a"); !ok || v != "updated" {
		t.Fatalf("get = %q,%v", v, ok)
	}
	kv.Delete("a")
	if _, ok := kv.Get("a"); ok {
		t.Fatal("delete failed")
	}
	if kv.Len() != 1 {
		t.Fatalf("len = %d, want 1", kv.Len())
	}
}

// ---- log ----------------------------------------------------------------

type capLogger struct {
	mu  sync.Mutex
	cnt int
}

func (c *capLogger) Log(string, string, map[string]string) {
	c.mu.Lock()
	c.cnt++
	c.mu.Unlock()
}

func TestLog_RateLimited(t *testing.T) {
	cl := &capLogger{}
	lg, err := capability.NewLog(manifest.CapabilityConfig{RateLimit: "1/h"}, cl)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if err := lg.Emit("info", "first", nil); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	if err := lg.Emit("info", "second", nil); !errors.Is(err, capability.ErrRateLimited) {
		t.Fatalf("second emit = %v, want ErrRateLimited", err)
	}
	if cl.cnt != 1 {
		t.Fatalf("logger called %d times, want 1 (drop over rate)", cl.cnt)
	}
	// nil host defaults to slog (must not panic).
	def, _ := capability.NewLog(manifest.CapabilityConfig{}, nil)
	if err := def.Emit("warn", "via slog", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("default slog emit: %v", err)
	}
}

// ---- BuildCapabilities + Invoker integration (acceptance lines) ---------

func validManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0",
		Type: manifest.TypeStarlark, Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapFSWrite: {Paths: []string{"/data/**"}},
			manifest.CapKV:      {},
			manifest.CapLog:     {RateLimit: "100/s"},
		},
	}
}

func TestBuildCapabilities(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
	caps, err := capability.BuildCapabilities(m, capability.Hosts{FS: newMemFS()})
	if err != nil {
		t.Fatalf("BuildCapabilities: %v", err)
	}
	if len(caps) != 3 {
		t.Fatalf("built %d caps, want 3", len(caps))
	}
	if _, ok := caps[manifest.CapFSWrite].(*capability.FSWrite); !ok {
		t.Fatalf("fs.write type = %T", caps[manifest.CapFSWrite])
	}

	// A malformed capability scope fails the whole build.
	bad := validManifest()
	bad.Capabilities[manifest.CapFSRead] = manifest.CapabilityConfig{Paths: []string{"[bad"}}
	if _, err := capability.BuildCapabilities(bad, capability.Hosts{FS: newMemFS()}); err == nil {
		t.Fatal("malformed glob: want build error")
	}
	if _, err := capability.BuildCapabilities(nil, capability.Hosts{}); err == nil {
		t.Fatal("nil manifest: want error")
	}
}

// TestAcceptance_UnauthorizedAndOutOfScopeAreAuditedFailures composes
// task-3 scoping with the task-2 Invoker to prove the epic lines:
// "Module attempting fs.write outside allowed paths fails with clear
// error + audit entry" and "Module attempting unauthorized exec
// fails with audit entry".
func TestAcceptance_UnauthorizedAndOutOfScopeAreAuditedFailures(t *testing.T) {
	m := validManifest() // grants fs.write (scoped to /data/**), kv, log — NOT exec
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	reg, err := capability.NewRegistryFromManifest(m)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	fa := &auditCapture{}
	inv := capability.NewInvoker(reg, fa)
	caps, _ := capability.BuildCapabilities(m, capability.Hosts{FS: newMemFS()})
	fw := caps[manifest.CapFSWrite].(*capability.FSWrite)

	// fs.write OUTSIDE allowed paths → scoping error, audited failure.
	err = inv.Invoke(context.Background(), manifest.CapFSWrite, "write", func(context.Context) error {
		return fw.Write("/etc/passwd", []byte("x"), 0o644)
	})
	if !errors.Is(err, capability.ErrPathDenied) {
		t.Fatalf("fs.write escape = %v, want ErrPathDenied", err)
	}
	last := fa.last()
	if last.Capability != "fs.write" || last.Success {
		t.Fatalf("expected audited fs.write failure, got %+v", last)
	}

	// exec NOT granted → Invoker refuses + audits denial (fn not run).
	ran := false
	err = inv.Invoke(context.Background(), manifest.CapExec, "run", func(context.Context) error {
		ran = true
		return nil
	})
	if ran || !errors.Is(err, capability.ErrCapabilityNotGranted) {
		t.Fatalf("unauthorized exec: ran=%v err=%v", ran, err)
	}
	last = fa.last()
	if last.Capability != "exec" || last.Success || last.Operation != "denied" {
		t.Fatalf("expected audited exec denial, got %+v", last)
	}
}

type auditCapture struct {
	mu sync.Mutex
	es []maudit.Entry
}

func (a *auditCapture) Emit(_ context.Context, e maudit.Entry) {
	a.mu.Lock()
	a.es = append(a.es, e)
	a.mu.Unlock()
}

func (a *auditCapture) last() maudit.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.es[len(a.es)-1]
}
