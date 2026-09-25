//go:build contract

package enrollmentcontract

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

	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// The fixture is NOT part of the frozen surface. C05-I may correct it without
// an amendment, as C03-I and C04-I corrected theirs; what it may not do is
// weaken what a case asserts.

const (
	image               = "keystone-c05-enrollment-contract:local"
	containerLabel      = "io.keystone-core.test=c05-enrollment"
	serverBin           = "/usr/local/bin/keystone-server"
	cliBin              = "/usr/local/bin/keystone"
	probeBin            = "/usr/local/bin/enrollment-probe"
	serverConfig        = "/etc/keystone/keystone-server.toml"
	serverConfigEnvName = "KEYSTONE_SERVER_CONFIG"
	natsDir             = "/etc/keystone/nats"
	signingKeyPath      = "/etc/keystone/service-signing.key"
	resultKeyPath       = "/etc/keystone/result-encryption.pub"
	storeDir            = "/var/lib/keystone"
	storePath           = storeDir + "/server.db"
	serverLog           = "/tmp/server.log"
	serverExit          = "/tmp/server.exit"
	socketPath          = "/run/keystone/operator.sock"

	adminGroup  = "ksadmin"
	adminGID    = 1500
	alice       = "alice" // a member: the authorized control
	aliceUID    = 1501
	outsider    = "outsider" // never a member: the kernel refuses connect()
	outsiderUID = 1503
	// root is not a member either: it connects, and the in-band check denies it.
)

var contextPaths = []string{
	"go.mod", "go.sum", "cmd", "internal",
	"test/contract/enrollment/Dockerfile",
	"test/contract/enrollment/probe",
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
		cmd := exec.Command("docker", "build", "-q", "-t", image, "-f", "test/contract/enrollment/Dockerfile", "-")
		cmd.Stdin = &buf
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("docker build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
}

// deployment is what an administrator provisions for the server: the generated
// NATS material and the two service keys. The fixture generates the keys; G51
// owns production provisioning.
type deployment struct {
	nats    *natsauth.Deployment
	signing *protocol.SigningKey
	result  *protocol.DecryptionKey
}

func newDeployment(t *testing.T) *deployment {
	t.Helper()
	d, err := natsauth.Generate(natsauth.Config{FleetSize: 4, BrokerNames: []string{brokerAlias}, RuntimeDir: brokerRuntimeDir})
	if err != nil {
		t.Fatal(err)
	}
	signing, err := protocol.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	result, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	return &deployment{nats: d, signing: signing, result: result}
}

// box is one host: its accounts, the deployment's files, and at most one
// server.
type box struct {
	t    *testing.T
	name string
	dep  *deployment
	tp   *topology
}

// newBox is a server host in a topology of its own.
func newBox(t *testing.T) *box {
	t.Helper()
	return newTopology(t).server()
}

// server starts the server host on the server network. The operator CLI runs
// here too: ADR-0009's socket is local to the server.
func (tp *topology) server() *box {
	t := tp.t
	t.Helper()
	b := &box{t: t, name: tp.prefix + "-server", dep: tp.dep, tp: tp}
	if out, err := exec.Command("docker", "run", "-d", "--name", b.name, "--label", containerLabel,
		"--network", tp.serverNet, "--tmpfs", storeDir+":rw,size=16m", image).CombinedOutput(); err != nil {
		t.Fatalf("start container: %v\n%s", err, out)
	}
	tp.cleanup(func() {
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
adduser %s %s
mkdir -p %s`,
		adminGID, adminGroup, aliceUID, alice, outsiderUID, outsider, alice, adminGroup, natsDir))
	b.provision()
	return b
}

// provision writes every credential the deployment has into natsDir -- the
// ones the server is configured with and the ones it is not -- and the two
// service keys.
func (b *box) provision() {
	b.t.Helper()
	for _, p := range natsauth.Services() {
		creds, err := natsauth.Credentials(b.dep.nats.Services[p])
		if err != nil {
			b.t.Fatal(err)
		}
		b.writeFile(credsPath(p), string(creds), 0o600)
	}
	b.writeFile(natsDir+"/account-signing.nk", string(b.dep.nats.Keystone.SigningSeed), 0o600)
	b.writeFile(natsDir+"/ca.pem", string(b.dep.nats.TLS.CACertPEM), 0o644)
	b.writeFile(signingKeyPath, enrollment.EncodeKey(protocol.MarshalSigningKey(b.dep.signing))+"\n", 0o600)
	b.writeFile(resultKeyPath, enrollment.EncodeKey(b.dep.result.Recipient().Bytes())+"\n", 0o644)
}

func credsPath(p natsauth.Principal) string { return natsDir + "/" + string(p) + ".creds" }

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

func (b *box) writeFile(path, content string, mode os.FileMode) {
	b.t.Helper()
	cmd := exec.Command("docker", "exec", "-i", b.name, "sh", "-c",
		fmt.Sprintf("cat > %s && chmod %04o %s", path, mode, path))
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

// serverConfig is the server's configuration file, in the frozen keys.
type serverConf struct {
	EnrollmentCreds string // default: the enrollment service's own credential
	PresenceCreds   string // default: the presence consumer's own credential
	HoldResponseMS  int
	// ServerName is the broker name the server verifies; default the one its
	// certificate carries.
	ServerName string
}

func (c serverConf) serverName() string {
	if c.ServerName == "" {
		return brokerAlias
	}
	return c.ServerName
}

func (c serverConf) render() string {
	enroll, presence := c.EnrollmentCreds, c.PresenceCreds
	if enroll == "" {
		enroll = credsPath(natsauth.EnrollmentService)
	}
	if presence == "" {
		presence = credsPath(natsauth.PresenceConsumer)
	}
	var s strings.Builder
	fmt.Fprintf(&s, "[operator]\nadmin_group = %q\n\n[store]\npath = %q\n\n", adminGroup, storePath)
	fmt.Fprintf(&s, "[%s]\n", serverTableNATS)
	for _, kv := range [][2]string{
		{serverKeyBrokerURL, "tls://" + brokerAlias + ":4222"},
		{serverKeyBrokerCA, natsDir + "/ca.pem"},
		{serverKeyBrokerName, c.serverName()},
		{serverKeyEnrollmentCreds, enroll},
		{serverKeyPresenceCreds, presence},
		{serverKeyAccountSigningSeed, natsDir + "/account-signing.nk"},
	} {
		fmt.Fprintf(&s, "%s = %q\n", kv[0], kv[1])
	}
	fmt.Fprintf(&s, "\n[%s]\n%s = %q\n%s = %q\n", serverTableService,
		serverKeySigningPrivateKey, signingKeyPath, serverKeyResultPublicKey, resultKeyPath)
	if c.HoldResponseMS != 0 {
		fmt.Fprintf(&s, "\n[%s]\n%s = %d\n", serverTableFaults, faultOperatorBeforeResponse, c.HoldResponseMS)
	}
	return s.String()
}

func (b *box) launch(c serverConf) {
	b.t.Helper()
	b.writeFile(serverConfig, c.render(), 0o600)
	b.sh("rm -f " + serverExit)
	b.detach("", "sh", "-c", fmt.Sprintf("%s=%s %s >%s 2>&1; echo $? >%s",
		serverConfigEnvName, serverConfig, serverBin, serverLog, serverExit))
}

func (b *box) exitCode() (int, bool) {
	out, err := b.exec("", "cat", serverExit)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	return n, err == nil
}

func (b *box) socketExists() bool {
	_, err := b.exec("", "test", "-S", socketPath)
	return err == nil
}

// start launches a server that must come up, and returns its pid once the
// socket exists.
func (b *box) start(c serverConf) int {
	b.t.Helper()
	b.launch(c)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if code, done := b.exitCode(); done {
			log, _ := b.exec("", "cat", serverLog)
			b.t.Fatalf("the server exited %d instead of serving:\n%s", code, log)
		}
		if b.socketExists() {
			if pid := b.serverPID(); pid > 0 {
				return pid
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.t.Fatal("the server did not create its socket within 20s")
	return 0
}

// refuse launches a server that must refuse to start, and returns its exit
// status and log.
func (b *box) refuse(c serverConf) (int, string) {
	b.t.Helper()
	b.launch(c)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if code, done := b.exitCode(); done {
			log, _ := b.exec("", "cat", serverLog)
			if code == 0 {
				b.t.Fatal("the server exited 0; a refusal to start must exit non-zero")
			}
			if b.socketExists() {
				b.t.Fatal("the server created its socket before refusing to start")
			}
			return code, log
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.stop()
	b.t.Fatal("the server was still running after 20s; it was required to refuse to start")
	return 0, ""
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
// The CLI and observations
// ---------------------------------------------------------------------------

type cliResult struct {
	Exit   int
	Stdout string
	Stderr string
}

// cli runs the production keystone CLI as user, capturing its streams
// separately.
func (b *box) cli(user string, args ...string) cliResult {
	b.t.Helper()
	a := []string{"exec", "-u", user, b.name, cliBin}
	cmd := exec.Command("docker", append(a, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := cliResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.Exit = ee.ExitCode()
	} else if err != nil {
		b.t.Fatalf("running the CLI: %v", err)
	}
	return r
}

// bundle decodes a successful create's standard output, which must be exactly
// one JSON object.
func (b *box) bundle(r cliResult) tokenBundleV1 {
	b.t.Helper()
	if r.Exit != 0 {
		b.t.Fatalf("enroll create exited %d: %s", r.Exit, r.Stderr)
	}
	d := json.NewDecoder(strings.NewReader(r.Stdout))
	d.DisallowUnknownFields()
	var bundle tokenBundleV1
	if err := d.Decode(&bundle); err != nil {
		b.t.Fatalf("standard output is not a bundle: %v\n%s", err, r.Stdout)
	}
	var extra json.RawMessage
	if err := d.Decode(&extra); err == nil {
		b.t.Fatalf("standard output holds more than one value:\n%s", r.Stdout)
	}
	return bundle
}

func (b *box) create(user, name string, extra ...string) cliResult {
	b.t.Helper()
	return b.cli(user, append([]string{"enroll", "create", "--agent-name", name}, extra...)...)
}

type records struct {
	Error   string           `json:"error"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

func (b *box) query(q string) records {
	b.t.Helper()
	var r records
	b.probe("", &r, "read-records", "-db", storePath, "-query", q)
	if r.Error != "" {
		b.t.Fatalf("query %q: %s", q, r.Error)
	}
	return r
}

func (r records) withAction(action string) []map[string]any {
	var out []map[string]any
	for _, row := range r.Rows {
		if row["action"] == action {
			out = append(out, row)
		}
	}
	return out
}

func operatorRequest(name string, ttl int64) string {
	req := operatorEnrollRequest{Operation: operatorOperation, AgentName: name, TTLSeconds: ttl}
	b, _ := json.Marshal(req)
	return string(b)
}
