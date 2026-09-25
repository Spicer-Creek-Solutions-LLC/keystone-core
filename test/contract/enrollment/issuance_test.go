//go:build contract

package enrollmentcontract

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"

	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// C05-I's first stage: issuance over the operator socket, and the server's
// startup checks. No broker is involved; the container has no network.

const auditAll = "SELECT * FROM audit"

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// seedOf is the bootstrap credential's secret line.
func seedOf(t *testing.T, creds string) string {
	t.Helper()
	kp, err := jwt.ParseDecoratedNKey([]byte(creds))
	if err != nil {
		t.Fatalf("the bundle's bootstrap credential has no seed: %v", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}
	return string(seed)
}

func TestISS1TokenIssuedOnceAndAudited(t *testing.T) {
	b := newBox(t)
	pid := b.start(serverConf{})

	r := b.create(alice, "web-1")
	bundle := b.bundle(r)
	if r.Stderr != "" {
		t.Fatalf("a successful create wrote to standard error: %q", r.Stderr)
	}
	rows := b.query("SELECT token_id, agent_id, state FROM enrollment").Rows
	if len(rows) != 1 || rows[0]["token_id"] != bundle.TokenID || rows[0]["agent_id"] != bundle.AgentID || rows[0]["state"] != "issued" {
		t.Fatalf("one create must record exactly its token: %v", rows)
	}
	issued := b.query(auditAll).withAction("enrollment.token.issued")
	if len(issued) != 1 || issued[0]["actor_uid"] != float64(aliceUID) || issued[0]["target"] != bundle.AgentID {
		t.Fatalf("one create must add exactly one issuance record naming its actor and agent: %v", issued)
	}

	// Nowhere else: not in the server's log, its store, or any regular file on
	// the host. The search names each file that holds the seed, and any file
	// it could not search; clean output is exactly the marker. Sockets are not
	// files a seed could be written to, and grep fails on them, which is why
	// the search is not a bare recursive grep: that exits 2 on the operator
	// socket whatever it finds, and a case reading exit codes was blind.
	seed := seedOf(t, bundle.BootstrapCredentials)
	search := func(dirs string) string {
		out, _ := b.exec("", "sh", "-c", "find "+dirs+" -type f -exec grep -lF '"+seed+"' {} \\; 2>&1; echo search-done")
		return strings.TrimSpace(out)
	}
	b.sh("printf '%s' '" + seed + "' > /tmp/control-seed")
	if out := search("/tmp"); !strings.Contains(out, "/tmp/control-seed") {
		t.Fatalf("control: the search did not find a planted copy of the seed:\n%s", out)
	}
	b.sh("rm /tmp/control-seed")
	if out := search("/var/lib/keystone /tmp /etc /run /root /home"); out != "search-done" {
		t.Fatalf("the bootstrap seed was written outside standard output, or the search failed:\n%s", out)
	}
	if strings.Contains(r.Stderr, seed) {
		t.Fatal("the bootstrap seed reached standard error")
	}

	// Durably: the server is SIGKILLed the instant each response is complete,
	// and every token it answered with is on record after the kill.
	for i := 0; i < 5; i++ {
		b.sh("rm -f /tmp/fifo /tmp/killed.json && mkfifo /tmp/fifo && chmod 666 /tmp/fifo")
		b.detach("", probeBin, "kill-on-fifo", "-fifo", "/tmp/fifo", "-pid", fmt.Sprint(pid), "-out", "/tmp/killed.json")
		var n struct {
			Error string `json:"error"`
			Frame string `json:"frame"`
		}
		b.probe(alice, &n, "request-and-notify", "-path", socketPath, "-frame", operatorRequest(fmt.Sprintf("kill-%d", i), 0), "-fifo", "/tmp/fifo")
		if n.Error != "" {
			t.Fatalf("round %d: %s", i, n.Error)
		}
		var resp operatorEnrollResponse
		if err := json.Unmarshal([]byte(n.Frame), &resp); err != nil || resp.Bundle.TokenID == "" {
			t.Fatalf("round %d: not a bundle: %s", i, n.Frame)
		}
		waitGone(b)
		got := b.query(fmt.Sprintf("SELECT token_id FROM enrollment WHERE token_id = '%s'", resp.Bundle.TokenID)).Rows
		if len(got) != 1 {
			t.Fatalf("round %d: the server answered with token %s and was killed; the token is not on record", i, resp.Bundle.TokenID)
		}
		pid = b.start(serverConf{})
	}
}

// waitGone waits for a killed server to be gone, so the next start is not
// refused by its live socket.
func waitGone(b *box) {
	b.t.Helper()
	for i := 0; i < 100; i++ {
		if b.serverPID() == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.t.Fatal("the killed server is still running")
}

// refusedPair is the same create by a principal the kernel refuses and by one
// the in-band check denies, after proving each layer is the one refusing.
func refusedPair(t *testing.T) (*box, cliResult, cliResult) {
	t.Helper()
	b := newBox(t)
	b.start(serverConf{})
	b.bundle(b.create(alice, "control")) // the authorized control

	var kernel, inband struct {
		ConnectErrno string   `json:"connect_errno"`
		Frames       []string `json:"frames"`
	}
	b.probe(outsider, &kernel, "connect", "-path", socketPath, "-frame", operatorRequest("x", 0))
	if kernel.ConnectErrno != "EACCES" {
		t.Fatalf("fixture: the outsider was not refused by the kernel: %+v", kernel)
	}
	b.probe("root", &inband, "connect", "-path", socketPath, "-frame", operatorRequest("x", 0))
	if inband.ConnectErrno != "" || len(inband.Frames) != 1 || inband.Frames[0] != `{"error":"authorization_denied"}` {
		t.Fatalf("fixture: root was not connected and denied in-band: %+v", inband)
	}
	k := b.create(outsider, "refused")
	i := b.create("root", "refused")
	if n := len(b.query("SELECT token_id FROM enrollment").Rows); n != 1 {
		t.Fatalf("refused creates changed the record: %d tokens", n)
	}
	return b, k, i
}

func TestCLI1RefusalPathsAreIndistinguishable(t *testing.T) {
	_, kernel, inband := refusedPair(t)
	if kernel != inband {
		t.Fatalf("the CLI distinguishes the two refusals:\nkernel: %+v\nin-band: %+v", kernel, inband)
	}
}

func TestCLI2AuthorizationRefusalsExitTen(t *testing.T) {
	_, kernel, inband := refusedPair(t)
	if kernel.Exit != 10 || inband.Exit != 10 {
		t.Fatalf("authorization refusals exited %d (kernel) and %d (in-band), want 10", kernel.Exit, inband.Exit)
	}
	if kernel.Stdout != "" || inband.Stdout != "" {
		t.Fatal("a refused create wrote to standard output")
	}
}

func TestCLI3MissingSocketExitsOne(t *testing.T) {
	b := newBox(t)
	if r := b.create(alice, "web-1"); r.Exit != 1 || r.Stdout != "" {
		t.Fatalf("with no server: exit %d, stdout %q", r.Exit, r.Stdout)
	}
	// The same command succeeds once the socket exists, so the exit above was
	// the missing socket and not the command.
	b.start(serverConf{})
	b.bundle(b.create(alice, "web-1"))
	b.stop()
	if b.socketExists() {
		t.Fatal("fixture: the stopped server left its socket")
	}
	if r := b.create(alice, "web-1"); r.Exit != 1 {
		t.Fatalf("after the server stopped: exit %d", r.Exit)
	}
}

type pipelineResult struct {
	Error          string   `json:"error"`
	FirstConsumed  bool     `json:"first_consumed"`
	Pending        int      `json:"pending"`
	Samples        int      `json:"samples"`
	ReadEarly      bool     `json:"read_early"`
	Frames         []string `json:"frames"`
	FrameMS        []int    `json:"frame_ms"`
	SecondConsumed bool     `json:"second_consumed"`
}

func TestLIM5BoundsInflightRequests(t *testing.T) {
	b := newBox(t)
	const hold = 2000
	b.start(serverConf{HoldResponseMS: hold})
	var p pipelineResult
	b.probe(alice, &p, "pipeline", "-path", socketPath,
		"-first", operatorRequest("first", 0), "-second", operatorRequest("second", 0), "-watch-ms", "20000")
	if p.Error != "" {
		t.Fatalf("instrument: %s", p.Error)
	}
	if p.Samples < 100 {
		t.Fatalf("instrument: only %d observations while the first request was held", p.Samples)
	}
	if p.ReadEarly {
		t.Fatal("the server read the pipelined second request before writing the first reply")
	}
	if !p.SecondConsumed || len(p.Frames) != 2 {
		t.Fatalf("control: the second request was never read or answered: %+v", p)
	}
	if p.FrameMS[0] < hold/2 {
		t.Fatalf("fixture: the first reply arrived at %dms; the hold of %dms was not in effect", p.FrameMS[0], hold)
	}
	for i, want := range []string{"first", "second"} {
		var resp operatorEnrollResponse
		if err := json.Unmarshal([]byte(p.Frames[i]), &resp); err != nil {
			t.Fatalf("reply %d is not a bundle: %s", i, p.Frames[i])
		}
		rows := b.query(fmt.Sprintf("SELECT agent_name FROM enrollment WHERE token_id = '%s'", resp.Bundle.TokenID)).Rows
		if len(rows) != 1 || rows[0]["agent_name"] != want {
			t.Fatalf("reply %d answers %v, want the %q request: replies are not in request order", i, rows, want)
		}
	}
}

func TestFLT2OperatorResponseHoldIsAudited(t *testing.T) {
	b := newBox(t)
	action := "fault.enabled:" + faultOperatorBeforeResponse

	b.start(serverConf{HoldResponseMS: 500})
	// Read before any connection: the record must precede serving.
	if got := b.query(auditAll).withAction(action); len(got) != 1 {
		t.Fatalf("with the hold enabled, %d %q records before the first connection", len(got), action)
	}
	b.stop()

	b.sh("rm -f " + storePath + "*")
	b.start(serverConf{})
	if got := b.query(auditAll).withAction(action); len(got) != 0 {
		t.Fatalf("with the hold disabled, %d %q records", len(got), action)
	}
	// The control: the store read above is the server's, and it is written.
	b.bundle(b.create(alice, "control"))
	if len(b.query(auditAll).withAction("enrollment.token.issued")) != 1 {
		t.Fatal("control: the store the case read is not the one the server writes")
	}
}

// c03BootstrapRows reads C03-A's frozen matrix for the bootstrap principal.
func c03BootstrapRows(t *testing.T, token string) (pub, sub []string) {
	t.Helper()
	src, err := os.ReadFile("../authorization/contract_surface.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\{"bootstrap", "(publish|subscribe)", "([^"]+)"\}`)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		subject := strings.ReplaceAll(m[2], "<own-token>", token)
		if m[1] == "publish" {
			pub = append(pub, subject)
		} else {
			sub = append(sub, subject)
		}
	}
	if len(pub) == 0 || len(sub) == 0 {
		t.Fatalf("C03's matrix has no bootstrap rows to compare against: pub %v sub %v", pub, sub)
	}
	return pub, sub
}

func sameStrings(a, b []string) bool {
	x, y := append([]string{}, a...), append([]string{}, b...)
	if len(x) != len(y) {
		return false
	}
	seen := map[string]int{}
	for _, s := range x {
		seen[s]++
	}
	for _, s := range y {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func bootstrapClaims(t *testing.T, bundle tokenBundleV1) *jwt.UserClaims {
	t.Helper()
	token, err := jwt.ParseDecoratedJWT([]byte(bundle.BootstrapCredentials))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestJWT1BootstrapCredentialIsExactAndBounded(t *testing.T) {
	b := newBox(t)
	b.start(serverConf{})

	before := time.Now().Unix()
	bundle := b.bundle(b.create(alice, "web-1", "--ttl-seconds", "120"))
	after := time.Now().Unix()
	c := bootstrapClaims(t, bundle)
	pub, sub := c03BootstrapRows(t, bundle.TokenID)
	if !sameStrings(c.Permissions.Pub.Allow, pub) || !sameStrings(c.Permissions.Sub.Allow, sub) ||
		len(c.Permissions.Pub.Deny) != 0 || len(c.Permissions.Sub.Deny) != 0 || c.Permissions.Resp != nil {
		t.Fatalf("grants %+v, want exactly C03's bootstrap rows pub %v sub %v", c.Permissions, pub, sub)
	}
	if c.Issuer != b.dep.nats.Keystone.SigningKey || c.IssuerAccount != b.dep.nats.Keystone.PublicKey {
		t.Fatalf("issued by %s for account %s", c.Issuer, c.IssuerAccount)
	}
	if c.Expires < before+120-1 || c.Expires > after+120 {
		t.Fatalf("expires %d, want 120s after issuance (%d..%d)", c.Expires, before, after)
	}
	row := b.query(fmt.Sprintf("SELECT expires_at FROM enrollment WHERE token_id = '%s'", bundle.TokenID)).Rows
	if len(row) != 1 || row[0]["expires_at"] != float64(c.Expires*1000) {
		t.Fatalf("the credential does not expire with the token: JWT %d, record %v", c.Expires, row)
	}

	// The default lifetime.
	before = time.Now().Unix()
	d := bootstrapClaims(t, b.bundle(b.create(alice, "web-2")))
	if d.Expires < before+int64(defaultTokenTTLSeconds)-1 || d.Expires > time.Now().Unix()+int64(defaultTokenTTLSeconds) {
		t.Fatalf("default expiry %d is not %ds after issuance", d.Expires, defaultTokenTTLSeconds)
	}

	// The bounds, at the CLI and at the socket.
	for _, ttl := range []int{minimumTokenTTLSeconds - 1, maximumTokenTTLSeconds + 1} {
		if r := b.create(alice, "bounds", "--ttl-seconds", fmt.Sprint(ttl)); r.Exit != 1 || r.Stdout != "" {
			t.Errorf("ttl %d at the CLI: exit %d", ttl, r.Exit)
		}
		var raw struct {
			Frames []string `json:"frames"`
		}
		b.probe(alice, &raw, "connect", "-path", socketPath, "-frame", operatorRequest("bounds", int64(ttl)))
		if len(raw.Frames) != 1 || strings.Contains(raw.Frames[0], "bundle") {
			t.Errorf("ttl %d at the socket was answered with %v", ttl, raw.Frames)
		}
	}
	for _, ttl := range []int{minimumTokenTTLSeconds, maximumTokenTTLSeconds} {
		b.bundle(b.create(alice, "bounds", "--ttl-seconds", fmt.Sprint(ttl)))
	}
	if n := len(b.query("SELECT token_id FROM enrollment WHERE agent_name = 'bounds'").Rows); n != 2 {
		t.Fatalf("%d tokens for the bounds checks, want the 2 in range", n)
	}
}

func TestBND1BundleCarriesBothServiceTrustHalves(t *testing.T) {
	b := newBox(t)
	b.start(serverConf{})
	bundle := b.bundle(b.create(alice, "web-1"))
	if bundle.Version != bundleVersion || !hex64.MatchString(bundle.TokenID) || !hex32.MatchString(bundle.AgentID) {
		t.Fatalf("bundle header %d %q %q", bundle.Version, bundle.TokenID, bundle.AgentID)
	}
	signing, err := base64.StdEncoding.DecodeString(bundle.ServiceSigningPublicKey)
	if err != nil || !bytes.Equal(signing, b.dep.signing.Verifying().Bytes()) {
		t.Fatal("the bundle's signing half is not the provisioned service signing key's")
	}
	if _, err := protocol.ParseVerifyingKey(signing); err != nil {
		t.Fatal(err)
	}
	result, err := base64.StdEncoding.DecodeString(bundle.ResultEncryptionPublicKey)
	if err != nil || !bytes.Equal(result, b.dep.result.Recipient().Bytes()) {
		t.Fatal("the bundle's result half is not the provisioned result-encryption key")
	}
}

func TestSKEY1ServiceSigningPrivateKeyIsOwnerOnly(t *testing.T) {
	b := newBox(t)
	for _, mode := range []string{"0640", "0604", "0660", "0644", "0610"} {
		b.sh("chmod " + mode + " " + signingKeyPath)
		if _, log := b.refuse(serverConf{}); !strings.Contains(log, signingKeyPath) {
			t.Errorf("mode %s: refused without naming the key file:\n%s", mode, log)
		}
	}
	for _, mode := range []string{"0400", "0600", "0700"} {
		b.sh("chmod " + mode + " " + signingKeyPath)
		b.start(serverConf{})
		b.bundle(b.create(alice, "mode-"+mode))
		b.stop()
	}
}

func TestROLE1WrongServerCredentialRoleIsRefused(t *testing.T) {
	b := newBox(t)
	enroll, presence := credsPath(natsauth.EnrollmentService), credsPath(natsauth.PresenceConsumer)
	for _, c := range []serverConf{
		{EnrollmentCreds: presence},
		{PresenceCreds: enroll},
		{EnrollmentCreds: credsPath(natsauth.CommandPublisher)},
		{PresenceCreds: credsPath(natsauth.MonitoringRole)},
		{EnrollmentCreds: credsPath(natsauth.ResultConsumer), PresenceCreds: enroll},
	} {
		b.refuse(c)
	}

	// Correctly configured, it starts -- and opens no credential it was not
	// given, though every other one is in the same directory.
	b.sh("rm -f /tmp/ready /tmp/stop /tmp/opens.json")
	b.detach("", probeBin, "watch-opens", "-dir", natsDir, "-ready", "/tmp/ready", "-until", "/tmp/stop", "-out", "/tmp/opens.json")
	for i := 0; ; i++ {
		if _, err := b.exec("", "test", "-e", "/tmp/ready"); err == nil {
			break
		}
		if i > 100 {
			t.Fatal("fixture: the open watch never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.start(serverConf{})
	b.bundle(b.create(alice, "web-1"))
	b.sh("touch /tmp/stop")
	var opens struct {
		Error  string   `json:"error"`
		Opened []string `json:"opened"`
	}
	for i := 0; ; i++ {
		out, err := b.exec("", "cat", "/tmp/opens.json")
		if err == nil {
			if err := json.Unmarshal([]byte(out), &opens); err != nil {
				t.Fatal(err)
			}
			break
		}
		if i > 100 {
			t.Fatal("fixture: the open watch never reported")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if opens.Error != "" {
		t.Fatalf("instrument: %s", opens.Error)
	}
	opened := map[string]bool{}
	for _, n := range opens.Opened {
		opened[n] = true
	}
	for _, want := range []string{"enrollment-service.creds", "presence-consumer.creds", "account-signing.nk"} {
		if !opened[want] {
			t.Fatalf("control: the watch did not see the server open %s (%v)", want, opens.Opened)
		}
	}
	for _, p := range []natsauth.Principal{natsauth.CommandPublisher, natsauth.ResultConsumer, natsauth.MonitoringRole} {
		if opened[string(p)+".creds"] {
			t.Errorf("the server opened %s.creds, which it was not configured with", p)
		}
	}
}
