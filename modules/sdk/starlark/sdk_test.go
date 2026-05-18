package starlarksdk_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	starlarksdk "go.keystone-core.io/keystone-core/modules/sdk/starlark"
	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
)

// ---- fake hosts ---------------------------------------------------------

type memFS struct {
	mu sync.Mutex
	f  map[string][]byte
}

func (m *memFS) ReadFile(p string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.f[p]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}
func (m *memFS) WriteFile(p string, d []byte, _ uint32) error {
	m.mu.Lock()
	m.f[p] = d
	m.mu.Unlock()
	return nil
}
func (m *memFS) Stat(p string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.f[p]; ok {
		return int64(len(b)), nil
	}
	return 0, errors.New("not found")
}

type httpDoer struct{}

func (httpDoer) Do(r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) }

type fakeExec struct{}

func (fakeExec) Run(_ context.Context, _, name string, _ []string) ([]byte, []byte, error) {
	return []byte("ran:" + name), nil, nil
}

type memSecrets struct{ s map[string]map[string]any }

func (m *memSecrets) Get(_ context.Context, p string) (map[string]any, error) {
	v, ok := m.s[p]
	if !ok {
		return nil, errors.New("no secret")
	}
	return v, nil
}
func (m *memSecrets) Set(_ context.Context, p string, d map[string]any) error {
	m.s[p] = d
	return nil
}

type capLogger struct {
	mu sync.Mutex
	n  int
}

func (c *capLogger) Log(string, string, map[string]string) { c.mu.Lock(); c.n++; c.mu.Unlock() }

type auditCap struct {
	mu sync.Mutex
	es []maudit.Entry
}

func (a *auditCap) Emit(_ context.Context, e maudit.Entry) {
	a.mu.Lock()
	a.es = append(a.es, e)
	a.mu.Unlock()
}
func (a *auditCap) caps() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.es))
	for i, e := range a.es {
		out[i] = e.Capability
	}
	return out
}

// ---- harness ------------------------------------------------------------

func fullManifest(httpHost string) *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapFSRead:       {Paths: []string{"/data/**"}},
			manifest.CapFSWrite:      {Paths: []string{"/data/**"}},
			manifest.CapHTTPGet:      {Domains: []string{httpHost}},
			manifest.CapExec:         {Commands: []string{"echo"}},
			manifest.CapSecretsRead:  {SecretPaths: []string{"app/**"}},
			manifest.CapSecretsWrite: {SecretPaths: []string{"app/**"}},
			manifest.CapKV:           {},
			manifest.CapLog:          {RateLimit: "1000/s"},
		},
	}
}

func run(t *testing.T, m *manifest.Manifest, hosts capability.Hosts, src string) (*loader.ExecuteResult, *auditCap, error) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
	caps, err := capability.BuildCapabilities(m, hosts)
	if err != nil {
		t.Fatalf("BuildCapabilities: %v", err)
	}
	reg, err := capability.NewRegistryFromManifest(m)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ac := &auditCap{}
	inv := capability.NewInvoker(reg, ac)

	rt := srt.New(srt.Config{Builtins: starlarksdk.Provider(inv)})
	inst, err := rt.Init(context.Background(), m, []byte(src), caps)
	if err != nil {
		return nil, ac, err
	}
	out, err := inst.Execute(context.Background(), map[string]any{})
	return out, ac, err
}

func TestSDK_AllNamespacesHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("PONG"))
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	fs := &memFS{f: map[string][]byte{}}
	sec := &memSecrets{s: map[string]map[string]any{"app/db": {"pw": "s3cr3t"}}}
	lg := &capLogger{}
	hosts := capability.Hosts{FS: fs, HTTP: httpDoer{}, Exec: fakeExec{}, Secrets: sec, Logger: lg}

	src := `
def main(input):
    kv.set("k", "v")
    log.info("hello", run="1")
    fs.write("/data/out.txt", "written")
    body = fs.read("/data/out.txt")
    s = secrets.read("app/db")
    secrets.write("app/new", {"x": "y"})
    r = http.get("` + srv.URL + `")
    e = exec.run("echo", ["hi"])
    return {
        "kv": kv.get("k"),
        "fs": body,
        "sec": s["pw"],
        "http_status": r["status"],
        "http_body": r["body"],
        "exec": e["stdout"],
        "missing": kv.get("nope"),
    }
`
	out, ac, err := run(t, fullManifest(host), hosts, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	o := out.Output
	if o["kv"] != "v" || o["fs"] != "written" || o["sec"] != "s3cr3t" ||
		o["http_status"].(int64) != 200 || o["http_body"] != "PONG" ||
		o["exec"] != "ran:echo" || o["missing"] != nil {
		t.Fatalf("output = %+v", o)
	}
	if lg.n != 1 {
		t.Fatalf("logger called %d times, want 1", lg.n)
	}
	if sec.s["app/new"]["x"] != "y" {
		t.Fatalf("secrets.write didn't persist: %+v", sec.s)
	}
	// Every capability invocation was audited.
	got := ac.caps()
	for _, want := range []string{manifest.CapKV, manifest.CapLog, manifest.CapFSWrite,
		manifest.CapFSRead, manifest.CapSecretsRead, manifest.CapSecretsWrite,
		manifest.CapHTTPGet, manifest.CapExec} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("capability %q not audited (audited: %v)", want, got)
		}
	}
}

func TestSDK_ScopingDenialIsStarlarkErrorAndAudited(t *testing.T) {
	fs := &memFS{f: map[string][]byte{}}
	hosts := capability.Hosts{FS: fs}
	m := &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapFSWrite: {Paths: []string{"/data/**"}},
		},
	}
	// Write outside the allowed path → ErrPathDenied → Starlark error
	// → runtime ErrExec; an audited failure entry is still emitted.
	_, ac, err := run(t, m, hosts, "def main(input):\n    fs.write(\"/etc/passwd\", \"x\")\n    return {}\n")
	if !errors.Is(err, srt.ErrExec) {
		t.Fatalf("scoping denial = %v, want srt.ErrExec", err)
	}
	a := ac.es
	if len(a) != 1 || a[0].Capability != manifest.CapFSWrite || a[0].Success {
		t.Fatalf("expected one audited fs.write failure, got %+v", a)
	}
}

func TestSDK_NonGrantedNamespaceAbsent(t *testing.T) {
	// Only kv granted → `log` is not predeclared → NameError → ErrExec.
	m := &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint:   "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{manifest.CapKV: {}},
	}
	_, _, err := run(t, m, capability.Hosts{}, "def main(input):\n    log.info(\"x\")\n    return {}\n")
	if !errors.Is(err, srt.ErrCompile) && !errors.Is(err, srt.ErrExec) {
		t.Fatalf("non-granted namespace = %v, want compile/exec resolve error", err)
	}
}

func TestSDK_PostExecArgsKVDeleteAndDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		_, _ = w.Write([]byte("echo:" + string(b)))
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	m := &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapHTTPPost: {Domains: []string{host}},
			manifest.CapExec:     {Commands: []string{"echo"}},
			manifest.CapKV:       {},
		},
	}
	hosts := capability.Hosts{HTTP: httpDoer{}, Exec: fakeExec{}}
	src := `
def main(input):
    r = http.post("` + srv.URL + `", "PAYLOAD")
    e = exec.run("echo", ["a", "b"])
    kv.set("x", "1")
    before = kv.get("x")
    kv.delete("x")
    return {"post": r["body"], "exec": e["stdout"], "before": before, "after": kv.get("x"), "names": dir(kv)}
`
	out, _, err := run(t, m, hosts, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	o := out.Output
	if o["post"] != "echo:PAYLOAD" || o["exec"] != "ran:echo" ||
		o["before"] != "1" || o["after"] != nil {
		t.Fatalf("output = %+v", o)
	}
	names := o["names"].([]any)
	if len(names) != 3 { // get, set, delete (sorted)
		t.Fatalf("dir(kv) = %v", names)
	}
}

func TestSDK_BuiltinErrorPaths(t *testing.T) {
	hosts := capability.Hosts{Exec: fakeExec{}}
	m := &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapExec:         {Commands: []string{"echo"}},
			manifest.CapSecretsWrite: {SecretPaths: []string{"app/**"}},
			manifest.CapKV:           {},
		},
	}
	cases := map[string]string{
		"exec non-string arg":    `def main(input):` + "\n" + `    exec.run("echo", [1])` + "\n" + `    return {}`,
		"secrets.write non-dict": `def main(input):` + "\n" + `    secrets.write("app/x", "notadict")` + "\n" + `    return {}`,
		"attr miss":              `def main(input):` + "\n" + `    return {"v": kv.nope}`,
		"bad args":               `def main(input):` + "\n" + `    kv.set("only-one-arg")` + "\n" + `    return {}`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := run(t, m, hosts, src); err == nil {
				t.Fatalf("%s: want error", name)
			}
		})
	}
}

func TestBuildStringDict_Errors(t *testing.T) {
	reg, _ := capability.NewRegistryFromManifest(&manifest.Manifest{
		Name: "a/b", Version: "1.0.0", Type: manifest.TypeStarlark, Entrypoint: "m.star",
		Capabilities: map[string]manifest.CapabilityConfig{manifest.CapKV: {}},
	})
	inv := capability.NewInvoker(reg, maudit.NoopAuditor{})

	if _, err := starlarksdk.BuildStringDict(map[string]any{"kv": "not a backend"}, inv); err == nil {
		t.Fatal("unknown caps value: want error")
	}
	if _, err := starlarksdk.BuildStringDict(map[string]any{}, nil); err == nil {
		t.Fatal("nil invoker: want error")
	}
	// Empty caps → empty dict, no error.
	d, err := starlarksdk.BuildStringDict(map[string]any{}, inv)
	if err != nil || len(d) != 0 {
		t.Fatalf("empty = %v, %v", d, err)
	}
}
