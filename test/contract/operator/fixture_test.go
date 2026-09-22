//go:build contract

package operatorcontract

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The fixture is NOT part of the frozen surface. C04-I may correct it without a
// contract amendment, as C03-I corrected C03's broker fixture; what it may not
// do is weaken what a case asserts.

const (
	image          = "keystone-c04-operator-contract:local"
	containerLabel = "io.keystone-core.test=c04-operator"
	serverBin      = "/usr/local/bin/keystone-server"
	probeBin       = "/usr/local/bin/operator-probe"
	configPath     = "/etc/keystone/keystone-server.toml"
	storeDir       = "/var/lib/keystone"
	storePath      = storeDir + "/server.db"
	serverLog      = "/tmp/server.log"
	serverExit     = "/tmp/server.exit"

	// The principals. Real accounts in the container's /etc/passwd and
	// /etc/group; ADR-0010 § 1 is why that is feedback rather than acceptance.
	adminGroup  = "ksadmin"
	adminGID    = 1500
	alice       = "alice" // a member: the authorized control in every case
	aliceUID    = 1501
	stale       = "stale" // a member whose membership is revoked under a running session
	staleUID    = 1502
	outsider    = "outsider" // never a member
	outsiderUID = 1503

	// controlOp names no operation. At C04 none exists, so an authorized peer
	// receives unknown_operation -- the one response a refused peer cannot get.
	controlOp = "c04-contract-control"
)

// The build context is only what the image compiles, sent as a tar on stdin.
// A directory context would upload the whole repository, .git included, and the
// CI job's client has no buildx to honour a per-Dockerfile ignore file.
var contextPaths = []string{
	"go.mod", "go.sum", "cmd", "internal",
	"test/contract/operator/Dockerfile",
	"test/contract/operator/probe",
	"test/contract/operator/sockprobe",
}

var (
	buildOnce sync.Once
	buildErr  error
)

func repoRoot() string { return filepath.Join("..", "..", "..") }

func buildImage(t *testing.T) {
	t.Helper()
	buildOnce.Do(func() {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		root := repoRoot()
		for _, p := range contextPaths {
			err := filepath.WalkDir(filepath.Join(root, p), func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(b))}); err != nil {
					return err
				}
				_, err = tw.Write(b)
				return err
			})
			if err != nil {
				buildErr = fmt.Errorf("build context %s: %w", p, err)
				return
			}
		}
		if err := tw.Close(); err != nil {
			buildErr = err
			return
		}
		cmd := exec.Command("docker", "build", "-q", "-t", image, "-f", "test/contract/operator/Dockerfile", "-")
		cmd.Stdin = &buf
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("docker build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
}

func sweep() {
	out, err := exec.Command("docker", "ps", "-aq", "--filter", "label="+containerLabel).Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.Command("docker", "rm", "--force", id).Run()
	}
}

// box is one container: one host, its accounts, and at most one server.
type box struct {
	t    *testing.T
	name string
}

// newBox starts a container with no network, a size-limited store filesystem,
// and the four principals provisioned.
func newBox(t *testing.T) *box {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("the operator contract requires Docker: %v", err)
	}
	buildImage(t)
	b := &box{t: t, name: fmt.Sprintf("keystone-c04-%d", time.Now().UnixNano())}
	if out, err := exec.Command("docker", "run", "-d", "--name", b.name, "--label", containerLabel,
		"--network", "none", "--tmpfs", storeDir+":rw,size=16m", image).CombinedOutput(); err != nil {
		t.Fatalf("start container: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := b.exec("", "cat", serverLog)
			t.Logf("server log:\n%s", log)
		}
		exec.Command("docker", "rm", "--force", b.name).Run()
	})
	b.sh(fmt.Sprintf(`set -e
addgroup -g %d %s
adduser -D -u %d %s
adduser -D -u %d %s
adduser -D -u %d %s
adduser %s %s
adduser %s %s
mkdir -p /etc/keystone`,
		adminGID, adminGroup, aliceUID, alice, staleUID, stale, outsiderUID, outsider,
		alice, adminGroup, stale, adminGroup))
	return b
}

func (b *box) exec(user string, args ...string) (string, error) {
	a := []string{"exec"}
	if user != "" {
		a = append(a, "-u", user)
	}
	a = append(a, b.name)
	out, err := exec.Command("docker", append(a, args...)...).CombinedOutput()
	return string(out), err
}

func (b *box) mustExec(user string, args ...string) string {
	b.t.Helper()
	out, err := b.exec(user, args...)
	if err != nil {
		b.t.Fatalf("exec %v as %q: %v\n%s", args, user, err, out)
	}
	return out
}

// detach starts args in the background and returns once it has started.
func (b *box) detach(user string, args ...string) {
	b.t.Helper()
	a := []string{"exec", "-d"}
	if user != "" {
		a = append(a, "-u", user)
	}
	a = append(a, b.name)
	if out, err := exec.Command("docker", append(a, args...)...).CombinedOutput(); err != nil {
		b.t.Fatalf("detach %v: %v\n%s", args, err, out)
	}
}

func (b *box) sh(script string) string {
	b.t.Helper()
	return b.mustExec("", "sh", "-c", script)
}

func (b *box) writeFile(path, content string) {
	b.t.Helper()
	cmd := exec.Command("docker", "exec", "-i", b.name, "sh", "-c", "cat > "+path)
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		b.t.Fatalf("write %s: %v\n%s", path, err, out)
	}
}

func (b *box) probe(user string, v any, args ...string) {
	b.t.Helper()
	out := b.mustExec(user, append([]string{probeBin}, args...)...)
	if err := json.Unmarshal([]byte(out), v); err != nil {
		b.t.Fatalf("probe %v: %v\n%s", args, err, out)
	}
}

// ---------------------------------------------------------------------------
// The server
// ---------------------------------------------------------------------------

type serverConfig struct {
	AdminGroup      string // "" omits the key
	MaxUnauthorized int    // 0 omits the key, here and below
	AuthTimeoutMS   int
	MaxAuthorized   int
	MaxFrameBytes   int
	HoldBeforeMS    int
	HoldAfterMS     int
}

func defaultConfig() serverConfig { return serverConfig{AdminGroup: adminGroup} }

func (c serverConfig) render() string {
	var s strings.Builder
	fmt.Fprintf(&s, "[%s]\n", tableOperator)
	if c.AdminGroup != "" {
		fmt.Fprintf(&s, "%s = %q\n", keyAdminGroup, c.AdminGroup)
	}
	for _, kv := range []struct {
		k string
		v int
	}{{keyMaxUnauthorized, c.MaxUnauthorized}, {keyAuthTimeoutMS, c.AuthTimeoutMS}, {keyMaxAuthorized, c.MaxAuthorized}, {keyMaxFrameBytes, c.MaxFrameBytes}} {
		if kv.v != 0 {
			fmt.Fprintf(&s, "%s = %d\n", kv.k, kv.v)
		}
	}
	fmt.Fprintf(&s, "\n[%s]\n%s = %q\n", tableStore, keyStore, storePath)
	if c.HoldBeforeMS != 0 || c.HoldAfterMS != 0 {
		fmt.Fprintf(&s, "\n[%s]\n", tableFaults)
		if c.HoldBeforeMS != 0 {
			fmt.Fprintf(&s, "%s = %d\n", keyHoldBeforeDecision, c.HoldBeforeMS)
		}
		if c.HoldAfterMS != 0 {
			fmt.Fprintf(&s, "%s = %d\n", keyHoldAfterDecision, c.HoldAfterMS)
		}
	}
	return s.String()
}

// launch writes the configuration and starts the server with no arguments.
func (b *box) launch(c serverConfig) {
	b.t.Helper()
	b.writeFile(configPath, c.render())
	b.sh("rm -f " + serverExit)
	b.detach("", "sh", "-c", fmt.Sprintf("%s=%s %s >%s 2>&1; echo $? >%s",
		serverConfigEnv, configPath, serverBin, serverLog, serverExit))
}

// exitCode reports the server's exit status once it has exited.
func (b *box) exitCode() (int, bool) {
	out, err := b.exec("", "cat", serverExit)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	return n, err == nil
}

// start launches a server that must come up, and returns its pid once the
// socket exists.
func (b *box) start(c serverConfig) int {
	b.t.Helper()
	b.launch(c)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if code, done := b.exitCode(); done {
			b.t.Fatalf("the server exited %d instead of serving", code)
		}
		var st statResult
		b.probe("", &st, "stat", "-path", socketPath)
		if st.Type == "socket" {
			if pid := b.serverPID(); pid > 0 {
				return pid
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.t.Fatal("the server did not create its socket within 15s")
	return 0
}

// refuse launches a server that must refuse to start, and returns its exit
// status.
func (b *box) refuse(c serverConfig) int {
	b.t.Helper()
	b.launch(c)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if code, done := b.exitCode(); done {
			if code == 0 {
				b.t.Fatal("the server exited 0; a refusal to start must exit non-zero")
			}
			return code
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.stop()
	b.t.Fatal("the server was still running after 15s; it was required to refuse to start")
	return 0
}

func (b *box) serverPID() int {
	out, err := b.exec("", "pidof", "keystone-server")
	if err != nil {
		return 0
	}
	f := strings.Fields(out)
	if len(f) != 1 {
		return 0
	}
	n, _ := strconv.Atoi(f[0])
	return n
}

func (b *box) stop() {
	b.t.Helper()
	b.exec("", "sh", "-c", "pkill -TERM keystone-server")
	for i := 0; i < 50; i++ {
		if b.serverPID() == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.exec("", "sh", "-c", "pkill -KILL keystone-server")
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

type statResult struct {
	Errno string `json:"errno"`
	UID   int    `json:"uid"`
	GID   int    `json:"gid"`
	Perm  string `json:"perm"`
	Type  string `json:"type"`
	Inode uint64 `json:"inode"`
}

type sample struct {
	MS   int `json:"ms"`
	OutQ int `json:"outq"`
}

type session struct {
	UID          int      `json:"uid"`
	Groups       []int    `json:"groups"`
	ConnectErrno string   `json:"connect_errno"`
	Sent         int      `json:"sent"`
	InitialOutQ  int      `json:"initial_outq"`
	Samples      []sample `json:"samples"`
	FirstDropMS  int      `json:"first_drop_ms"`
	Frames       []string `json:"frames"`
	FrameMS      []int    `json:"frame_ms"`
	Trailing     int      `json:"trailing_bytes"`
	EOFMS        int      `json:"eof_ms"`
	ProbeError   string   `json:"probe_error"`
}

type auditResult struct {
	Error   string           `json:"error"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

func (b *box) stat(user, path string) statResult {
	b.t.Helper()
	var st statResult
	b.probe(user, &st, "stat", "-path", path)
	return st
}

func request(op string, extra map[string]any) string {
	m := map[string]any{requestOp: op}
	for k, v := range extra {
		m[k] = v
	}
	j, _ := json.Marshal(m)
	return string(j)
}

func (b *box) session(user string, args ...string) session {
	b.t.Helper()
	var s session
	b.probe(user, &s, append([]string{"session", "-path", socketPath}, args...)...)
	return s
}

func (b *box) multi(user string, args ...string) []session {
	b.t.Helper()
	var s []session
	b.probe(user, &s, append([]string{"multi", "-path", socketPath}, args...)...)
	return s
}

// authorizedControl is the positive half of every refusal case: a member's
// connection passes authorization and is answered. Without it, a server that
// refused everyone would satisfy every denial.
func (b *box) authorizedControl() {
	b.t.Helper()
	s := b.session(alice, "-frame", request(controlOp, nil), "-watch-ms", "10000", "-until-frames", "1")
	if s.ConnectErrno != "" {
		b.t.Fatalf("control: a member's connect() failed with %s", s.ConnectErrno)
	}
	if len(s.Frames) != 1 || errorCode(b.t, s.Frames[0]) != errUnknownOperation {
		b.t.Fatalf("control: a member was not answered with %s: %+v", errUnknownOperation, s)
	}
}

// errorCode decodes a server error frame, which must be an object with exactly
// one member.
func errorCode(t *testing.T, frame string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(frame), &m); err != nil {
		t.Fatalf("frame %q is not a JSON object: %v", frame, err)
	}
	code, ok := m[errorKey].(string)
	if !ok || len(m) != 1 {
		t.Fatalf("frame %q is not an object whose only member is %q", frame, errorKey)
	}
	return code
}

// armStale starts a probe session as stale WHILE stale is still a member, so
// the process carries the admin group in its credentials, and holds it before
// connecting. The returned function releases it and collects its result. The
// caller revokes the membership in between; that is the stale session ADR-0009
// § 3 says only the in-band check can catch.
func (b *box) armStale(tag string, args ...string) func() session {
	b.t.Helper()
	gate, out := "/tmp/go-"+tag, "/tmp/out-"+tag+".json"
	b.detach(stale, append([]string{probeBin, "session", "-path", socketPath, "-wait-for", gate, "-out", out}, args...)...)
	return func() session {
		b.t.Helper()
		b.sh("touch " + gate)
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if raw, err := b.exec("", "cat", out); err == nil {
				var s session
				if err := json.Unmarshal([]byte(raw), &s); err != nil {
					b.t.Fatalf("stale session result: %v\n%s", err, raw)
				}
				if !hasGroup(s.Groups, adminGID) {
					b.t.Fatalf("fixture: the stale session does not carry group %d (%v); the case would test nothing", adminGID, s.Groups)
				}
				return s
			}
			time.Sleep(50 * time.Millisecond)
		}
		b.t.Fatal("the stale session produced no result within 60s")
		return session{}
	}
}

func (b *box) revoke(user string) {
	b.t.Helper()
	b.sh(fmt.Sprintf("delgroup %s %s", user, adminGroup))
}
func (b *box) grant(user string) { b.t.Helper(); b.sh(fmt.Sprintf("adduser %s %s", user, adminGroup)) }

func hasGroup(groups []int, gid int) bool {
	for _, g := range groups {
		if g == gid {
			return true
		}
	}
	return false
}

func (b *box) audit() auditResult {
	b.t.Helper()
	var a auditResult
	b.probe("", &a, "read-records", "-db", storePath, "-query", auditQuery)
	if a.Error != "" {
		b.t.Fatalf("reading the audit table: %s", a.Error)
	}
	return a
}

func (a auditResult) withAction(action string) []map[string]any {
	var out []map[string]any
	for _, r := range a.Rows {
		if r[columnAction] == action {
			out = append(out, r)
		}
	}
	return out
}

func (a auditResult) withActionPrefix(prefix string) []map[string]any {
	var out []map[string]any
	for _, r := range a.Rows {
		if s, ok := r[columnAction].(string); ok && strings.HasPrefix(s, prefix) {
			out = append(out, r)
		}
	}
	return out
}

// denied asserts a session was answered with exactly one authorization_denied
// frame and then closed.
func denied(t *testing.T, who string, s session) {
	t.Helper()
	if s.ConnectErrno != "" {
		t.Fatalf("%s: connect() failed with %s; the kernel admitted this peer, so the in-band check must be what refuses it", who, s.ConnectErrno)
	}
	if len(s.Frames) != 1 || errorCode(t, s.Frames[0]) != errAuthorizationDenied {
		t.Fatalf("%s: expected exactly one %s frame, got %+v", who, errAuthorizationDenied, s)
	}
	if s.EOFMS < 0 {
		t.Fatalf("%s: the connection was not closed after the denial: %+v", who, s)
	}
}

// actorUID reads a numeric column from a probe row, which JSON delivers as a
// float64.
func actorUID(t *testing.T, row map[string]any) int {
	t.Helper()
	v, ok := row[columnActorUID].(float64)
	if !ok {
		t.Fatalf("record %v has no numeric %s", row, columnActorUID)
	}
	return int(v)
}
