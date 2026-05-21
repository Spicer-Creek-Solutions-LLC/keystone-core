package files

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	internalfiles "go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	"go.keystone-core.io/keystone-core/internal/files/transport"
	natspkg "go.keystone-core.io/keystone-core/internal/nats"
)

// --- URI + flag parsing ----------------------------------------------------

func TestParseRemotePath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"configs/app.yaml", "configs/app.yaml", false},
		{"kv://configs/app.yaml", "configs/app.yaml", false},
		{"kv://single", "single", false},
		{"", "", true},
		{"kv://", "", true},
		{"/leading", "", true},
	}
	for _, tc := range cases {
		got, err := parseRemotePath(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseRemotePath(%q): want error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRemotePath(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseRemotePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseListPrefix(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"a/", "a/", false},
		{"kv://a/", "a/", false},
		{"kv://", "", false},
		{"/bad", "", true},
	}
	for _, tc := range cases {
		got, err := parseListPrefix(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseListPrefix(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseListPrefix(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseListPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseTagFlags(t *testing.T) {
	got, err := parseTagFlags([]string{"env=prod", "owner=ops"})
	if err != nil {
		t.Fatal(err)
	}
	if got["env"] != "prod" || got["owner"] != "ops" {
		t.Errorf("tags = %+v", got)
	}

	if _, err := parseTagFlags([]string{"badnoequals"}); err == nil {
		t.Error("malformed tag should error")
	}
	if got, _ := parseTagFlags(nil); got != nil {
		t.Errorf("nil input should return nil, got %+v", got)
	}
}

func TestApplyEnvDefaults_NoEnvSet_DefaultsApplied(t *testing.T) {
	// Save + clear potentially-set env vars so the defaults branch runs.
	for _, k := range []string{"KSCORE_NATS_URL", "KSCORE_CLUSTER", "KSCORE_PRINCIPAL_ID", "KSCORE_PRINCIPAL_ROLE"} {
		t.Setenv(k, "")
	}
	g := &globals{}
	applyEnvDefaults(g)
	if g.natsURL != "nats://localhost:4222" {
		t.Errorf("natsURL = %q", g.natsURL)
	}
	if g.cluster != "default" {
		t.Errorf("cluster = %q", g.cluster)
	}
}

func TestApplyEnvDefaults_EnvOverridesDefault(t *testing.T) {
	t.Setenv("KSCORE_NATS_URL", "nats://example:4222")
	t.Setenv("KSCORE_CLUSTER", "dc1")
	t.Setenv("KSCORE_PRINCIPAL_ID", "op-7")
	t.Setenv("KSCORE_PRINCIPAL_ROLE", "operator")
	g := &globals{}
	applyEnvDefaults(g)
	if g.natsURL != "nats://example:4222" {
		t.Errorf("natsURL = %q", g.natsURL)
	}
	if g.cluster != "dc1" {
		t.Errorf("cluster = %q", g.cluster)
	}
	if g.principalID != "op-7" {
		t.Errorf("principalID = %q", g.principalID)
	}
	if g.principalRole != "operator" {
		t.Errorf("principalRole = %q", g.principalRole)
	}
}

func TestGlobals_Principal_NilWhenUnset(t *testing.T) {
	g := &globals{}
	p, err := g.principal()
	if err != nil || p != nil {
		t.Errorf("want nil/nil, got %v/%v", p, err)
	}
}

func TestGlobals_Principal_ParseError(t *testing.T) {
	g := &globals{principalID: "abc", principalRole: "wizard"}
	if _, err := g.principal(); err == nil {
		t.Error("unknown role should error")
	}
}

// --- list/stat rendering ----------------------------------------------------

func TestRenderList_Table(t *testing.T) {
	var buf bytes.Buffer
	list := []internalfiles.FileMetadata{
		{Path: "a", Size: 10, Version: 1, Hash: strings.Repeat("a", 64)},
		{Path: "b", Size: 20, Version: 2, Hash: strings.Repeat("b", 64)},
	}
	if err := renderList(&buf, list, "table"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "PATH") || !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("table output:\n%s", out)
	}
}

func TestRenderList_JSON(t *testing.T) {
	var buf bytes.Buffer
	list := []internalfiles.FileMetadata{{Path: "a", Size: 1}}
	if err := renderList(&buf, list, "json"); err != nil {
		t.Fatal(err)
	}
	var got []internalfiles.FileMetadata
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Path != "a" {
		t.Errorf("got = %+v", got)
	}
}

func TestRenderList_Unknown(t *testing.T) {
	if err := renderList(&bytes.Buffer{}, nil, "yaml"); err == nil {
		t.Error("unknown format should error")
	}
}

func TestRenderStat_Table(t *testing.T) {
	var buf bytes.Buffer
	m := internalfiles.FileMetadata{
		Path: "p", Size: 5, Hash: "abc", Version: 1,
		ContentType: "text/plain",
		CreatedAt:   time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		Tags:        map[string]string{"env": "prod"},
	}
	if err := renderStat(&buf, m, "table"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"PATH:", "p", "SIZE:", "HASH:", "CONTENT-TYPE:", "TAG[env]:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderStat_JSON(t *testing.T) {
	var buf bytes.Buffer
	m := internalfiles.FileMetadata{Path: "p"}
	if err := renderStat(&buf, m, "json"); err != nil {
		t.Fatal(err)
	}
	var got internalfiles.FileMetadata
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "p" {
		t.Errorf("got = %+v", got)
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("abcdefghijklmnop"); got != "abcdefghijkl" {
		t.Errorf("got %q", got)
	}
	if got := shortHash("short"); got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestIsResumableErr(t *testing.T) {
	cases := map[string]bool{
		"chunk timeout after 60s":    true,
		"response timeout after 60s": true,
		"some other error":           false,
		"":                           false,
	}
	for in, want := range cases {
		var err error
		if in != "" {
			err = errString(in)
		}
		if got := isResumableErr(err); got != want {
			t.Errorf("isResumableErr(%q) = %v, want %v", in, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestNewLogger_LevelParsing(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error", "weird", ""} {
		if l := newLogger(lvl); l == nil {
			t.Errorf("newLogger(%q) = nil", lvl)
		}
	}
}

// --- end-to-end via embedded NATS -------------------------------------------

// e2eRig spins an embedded NATS + transport.Service against a
// MemoryStore so the cobra subcommands can be driven end-to-end
// without external dependencies.
type e2eRig struct {
	srv  *natsserver.Server
	conn *nats.Conn
	svc  *transport.Service
	url  string
}

func newE2ERig(t *testing.T) *e2eRig {
	t.Helper()
	opts := &natsserver.Options{
		Host:       "127.0.0.1",
		Port:       freePort(t),
		NoSigs:     true,
		NoLog:      true,
		MaxPayload: 4 * 1024 * 1024,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("embedded NATS not ready")
	}
	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("connect: %v", err)
	}
	subjects, err := natspkg.NewSubjectBuilder("default")
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}
	svc, err := transport.NewService(conn, subjects, backend.NewMemoryStore(nil), nil)
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}
	rig := &e2eRig{srv: srv, conn: conn, svc: svc, url: srv.ClientURL()}
	t.Cleanup(rig.close)
	return rig
}

func (r *e2eRig) close() {
	_ = r.svc.Stop()
	if r.conn != nil {
		r.conn.Close()
	}
	if r.srv != nil {
		r.srv.Shutdown()
		r.srv.WaitForShutdown()
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// run executes the kscore-files cobra command with the given args
// and returns stdout + the result error.
func run(t *testing.T, rigURL string, args ...string) (string, error) {
	t.Helper()
	cmd := NewFilesCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	all := append([]string{"--nats-url", rigURL, "--cluster", "default"}, args...)
	cmd.SetArgs(all)
	err := cmd.Execute()
	return out.String(), err
}

func TestCLI_PutGetRoundTrip_OverNATS(t *testing.T) {
	rig := newE2ERig(t)
	dir := t.TempDir()
	localSrc := filepath.Join(dir, "src.txt")
	body := []byte("hello cli")
	if err := os.WriteFile(localSrc, body, 0o600); err != nil {
		t.Fatal(err)
	}

	// PUT
	out, err := run(t, rig.url,
		"put", localSrc, "kv://configs/app.yaml",
		"--content-type", "application/yaml",
		"--tag", "env=prod",
	)
	if err != nil {
		t.Fatalf("put: %v\n%s", err, out)
	}
	if !strings.Contains(out, "uploaded") || !strings.Contains(out, "configs/app.yaml") {
		t.Errorf("unexpected put output:\n%s", out)
	}

	// GET
	localDest := filepath.Join(dir, "dst.txt")
	out, err = run(t, rig.url, "get", "kv://configs/app.yaml", localDest)
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	got, err := os.ReadFile(localDest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("downloaded body mismatch")
	}
	if !strings.Contains(out, "downloaded") {
		t.Errorf("unexpected get output:\n%s", out)
	}
}

func TestCLI_List_OverNATS(t *testing.T) {
	rig := newE2ERig(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	if err := os.WriteFile(src, []byte("z"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a/1", "a/2", "b/1"} {
		if _, err := run(t, rig.url, "put", src, "kv://"+p); err != nil {
			t.Fatalf("seed put %s: %v", p, err)
		}
	}

	out, err := run(t, rig.url, "list", "--output", "json")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	var got []internalfiles.FileMetadata
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode list json: %v\n%s", err, out)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}

	// Prefix list.
	out, err = run(t, rig.url, "list", "kv://a/", "--output", "json")
	if err != nil {
		t.Fatalf("list prefix: %v\n%s", err, out)
	}
	var pref []internalfiles.FileMetadata
	if err := json.Unmarshal([]byte(out), &pref); err != nil {
		t.Fatal(err)
	}
	if len(pref) != 2 {
		t.Errorf("prefix len = %d, want 2", len(pref))
	}
}

func TestCLI_Stat_OverNATS(t *testing.T) {
	rig := newE2ERig(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	if err := os.WriteFile(src, []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, rig.url, "put", src, "kv://configs/y"); err != nil {
		t.Fatalf("put: %v", err)
	}
	out, err := run(t, rig.url, "stat", "kv://configs/y", "--output", "json")
	if err != nil {
		t.Fatalf("stat: %v\n%s", err, out)
	}
	var m internalfiles.FileMetadata
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if m.Path != "configs/y" || m.Size != 1 {
		t.Errorf("stat = %+v", m)
	}
}

func TestCLI_Delete_OverNATS(t *testing.T) {
	rig := newE2ERig(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	if err := os.WriteFile(src, []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, rig.url, "put", src, "kv://tmp/file"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, rig.url, "delete", "kv://tmp/file")
	if err != nil {
		t.Fatalf("delete: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("unexpected delete output:\n%s", out)
	}

	// stat after delete should error
	if _, err := run(t, rig.url, "stat", "kv://tmp/file"); err == nil {
		t.Error("stat after delete should fail")
	}
}

func TestCLI_NoNATSURL_Errors(t *testing.T) {
	// applyEnvDefaults supplies nats://localhost:4222 as the
	// default when neither flag nor env var is set, so this test
	// exercises the connect path which then fails to reach a real
	// NATS server. The behaviour is "command fails cleanly", not
	// "panics".
	t.Setenv("KSCORE_NATS_URL", "")
	cmd := NewFilesCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--timeout", "100ms"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("want error when NATS unreachable:\n%s", out.String())
	}
}

func TestCLI_BadRoleFlag_Errors(t *testing.T) {
	rig := newE2ERig(t)
	out, err := run(t, rig.url, "--principal-role", "wizard", "list")
	if err == nil {
		t.Errorf("want error for unknown role:\n%s", out)
	}
}

func TestCLI_BadOutputFormat_Errors(t *testing.T) {
	rig := newE2ERig(t)
	if _, err := run(t, rig.url, "list", "--output", "yaml"); err == nil {
		t.Error("want error for unknown output format")
	}
}
