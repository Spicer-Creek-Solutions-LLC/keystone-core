//go:build contract

package enrollmentcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// C05-I's second stage: the journey across the broker.

// issued is one token, as the operator received it.
type issued struct {
	raw    string
	bundle tokenBundleV1
}

func (b *box) issue(name string) issued {
	b.t.Helper()
	r := b.create(alice, name)
	return issued{raw: r.Stdout, bundle: b.bundle(r)}
}

// agentRow is the server's record of an active agent.
func (b *box) agentRow(id string) map[string]any {
	b.t.Helper()
	rows := b.query(fmt.Sprintf("SELECT agent_id, state, nats_public_key, hex(signing_public_key) AS signing, hex(encryption_public_key) AS encryption FROM agents WHERE agent_id = '%s'", id)).Rows
	if len(rows) != 1 {
		return nil
	}
	return rows[0]
}

func (b *box) tokenRow(token string) map[string]any {
	b.t.Helper()
	rows := b.query(fmt.Sprintf("SELECT state, nats_public_key, permanent_jwt FROM enrollment WHERE token_id = '%s'", token)).Rows
	if len(rows) != 1 {
		b.t.Fatalf("token %s has %d records", token, len(rows))
	}
	return rows[0]
}

// enrolled asserts the server's view of a completed enrollment.
func (b *box) enrolled(tok issued) {
	b.t.Helper()
	row := b.agentRow(tok.bundle.AgentID)
	if row == nil || row["state"] != "active" {
		b.t.Fatalf("agent %s is not recorded active: %v", tok.bundle.AgentID, row)
	}
	if st := b.tokenRow(tok.bundle.TokenID)["state"]; st != "spent" {
		b.t.Fatalf("token state %v after enrollment, want spent", st)
	}
}

func (a *agentBox) mustEnroll() {
	a.t.Helper()
	if r := a.enroll(); r.Exit != 0 {
		a.t.Fatalf("enrollment exited %d:\n%s", r.Exit, a.log())
	}
}

func (a *agentBox) exists(path string) bool {
	_, err := a.exec("", "test", "-e", path)
	return err == nil
}

// file reads a file on the host as root.
func (b *box) file(path string) []byte {
	b.t.Helper()
	out, err := exec.Command("docker", "exec", b.name, "cat", path).Output()
	if err != nil {
		b.t.Fatalf("read %s: %v", path, err)
	}
	return out
}

// searchFiles names every regular file under dirs holding any needle, and any
// file the search could not read. Clean output is exactly the marker; a
// planted copy proves the search sees what it looks for.
func (b *box) searchFiles(dirs string, needles ...string) string {
	b.t.Helper()
	b.sh("printf '%s' '" + needles[0] + "' > /tmp/search-control")
	var cmd strings.Builder
	cmd.WriteString("find " + dirs + " -type f")
	for i, n := range needles {
		if i == 0 {
			cmd.WriteString(" \\( ")
		} else {
			cmd.WriteString(" -o ")
		}
		cmd.WriteString("-exec grep -qF '" + n + "' {} \\; -print")
	}
	cmd.WriteString(" \\) 2>&1; echo search-done")
	control, _ := b.exec("", "sh", "-c", strings.Replace(cmd.String(), "find "+dirs, "find /tmp", 1))
	if !strings.Contains(control, "/tmp/search-control") {
		b.t.Fatalf("control: the search did not find a planted needle:\n%s", control)
	}
	b.sh("rm /tmp/search-control")
	out, _ := b.exec("", "sh", "-c", cmd.String())
	return strings.TrimSpace(out)
}

func TestSTDIN1EnrollsFromStandardInput(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	if r := a.enrollStdin(tok.raw); r.Exit != 0 {
		t.Fatalf("enroll --token-file - exited %d:\n%s", r.Exit, a.log())
	}
	b.enrolled(tok)
	// The bundle came through a pipe and never touched the agent's disk.
	if out := a.searchFiles("/root /tmp /etc /var "+agentStateDir, seedOf(t, tok.bundle.BootstrapCredentials)); out != "search-done" {
		t.Fatalf("a bundle read from standard input was written to disk:\n%s", out)
	}
}

func TestFILE1RefusesReadableBundle(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	for _, mode := range []os.FileMode{0o640, 0o604, 0o644} {
		a.deliver(tok.raw, mode)
		if r := a.enroll(); r.Exit != 1 {
			t.Fatalf("a mode-%04o bundle: exit %d, want 1:\n%s", mode, r.Exit, a.log())
		}
		if row := b.tokenRow(tok.bundle.TokenID); row["nats_public_key"] != nil {
			t.Fatalf("a mode-%04o bundle was refused only after its request reached the server", mode)
		}
		if a.exists(agentStateDir + "/keys.json") {
			t.Fatalf("a mode-%04o bundle was refused only after the agent generated keys", mode)
		}
	}
	// The same bundle at 0600 enrolls, so the refusals were the mode.
	a.deliver(tok.raw, 0o600)
	a.mustEnroll()
	b.enrolled(tok)
}

func TestFILE2SuccessfulEnrollmentRemovesBundle(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	if !a.exists(agentBundle) {
		t.Fatal("fixture: the bundle was not delivered")
	}
	a.mustEnroll()
	if a.exists(agentBundle) {
		t.Fatal("the bundle survived an enrollment that exited 0")
	}
	b.enrolled(tok)
}

type argvWatch struct {
	Error   string   `json:"error"`
	Samples int      `json:"samples"`
	Argv    []string `json:"argv"`
}

// watchArgv samples every command line on a host while run executes.
func (b *box) watchArgv(run func()) argvWatch {
	b.t.Helper()
	b.sh("rm -f /tmp/argv-ready /tmp/argv-stop /tmp/argv.json")
	b.detach("", probeBin, "watch-argv", "-ready", "/tmp/argv-ready", "-until", "/tmp/argv-stop", "-out", "/tmp/argv.json")
	b.waitFor("/tmp/argv-ready")
	run()
	b.sh("touch /tmp/argv-stop")
	b.waitFor("/tmp/argv.json")
	var w argvWatch
	if err := json.Unmarshal(b.file("/tmp/argv.json"), &w); err != nil || w.Error != "" {
		b.t.Fatalf("instrument: %v %s", err, w.Error)
	}
	return w
}

func (b *box) waitFor(path string) {
	b.t.Helper()
	for i := 0; i < 200; i++ {
		if _, err := b.exec("", "test", "-e", path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.t.Fatalf("fixture: %s never appeared", path)
}

func TestARG1BundleNeverAppearsInArgv(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	var tok issued
	serverSide := b.watchArgv(func() { tok = b.issue("web-1") })
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	agentSide := a.watchArgv(func() { a.mustEnroll() })

	secrets := []string{seedOf(t, tok.bundle.BootstrapCredentials), bootstrapJWT(t, tok.bundle)}
	for host, w := range map[string]argvWatch{"server": serverSide, "agent": agentSide} {
		// The control: the sampler saw the command it was watching for.
		want := map[string]string{"server": "enroll create", "agent": "keystone-agent enroll"}[host]
		saw := false
		for _, line := range w.Argv {
			saw = saw || strings.Contains(line, want)
			for _, s := range secrets {
				if strings.Contains(line, s) {
					t.Errorf("the bundle's credential appeared in a %s process's argv: %q", host, line)
				}
			}
		}
		if !saw {
			t.Fatalf("control: the %s sampler never saw %q in %d samples; it proves nothing", host, want, w.Samples)
		}
	}
}

func bootstrapJWT(t *testing.T, bundle tokenBundleV1) string {
	t.Helper()
	token, err := jwt.ParseDecoratedJWT([]byte(bundle.BootstrapCredentials))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// agentPrivate is every representation of an agent's private keys the
// contract looks for: the NKey seed, and each Keystone key in the base64 form
// the artifact stores and as raw bytes.
func agentPrivate(t *testing.T, a *agentBox) (text []string, raw [][]byte) {
	t.Helper()
	var k keyArtifact
	if err := json.Unmarshal(a.file(agentStateDir+"/keys.json"), &k); err != nil {
		t.Fatal(err)
	}
	text = []string{k.NATSSeed, k.SigningPrivateKey, k.DecryptionPrivateKey}
	for _, s := range []string{k.SigningPrivateKey, k.DecryptionPrivateKey} {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, b)
	}
	kp, err := nkeys.FromSeed([]byte(k.NATSSeed))
	if err != nil {
		t.Fatal(err)
	}
	rawSeed, err := kp.PrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return append(text, string(rawSeed)), raw
}

type keyArtifact = agentKeyArtifactV1

func TestKEY1AgentPrivateKeysNeverLeaveAgent(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.mustEnroll()
	b.enrolled(tok)

	text, raw := agentPrivate(t, a)
	if out := b.searchFiles(storeDir+" /tmp /etc /run /root", text[:3]...); out != "search-done" {
		t.Fatalf("an agent private key is on the server host:\n%s", out)
	}
	for _, name := range []string{"server.db", "server.db-wal"} {
		content, _ := exec.Command("docker", "exec", b.name, "cat", storeDir+"/"+name).Output()
		for _, r := range raw {
			if bytes.Contains(content, r) {
				t.Fatalf("an agent private key's raw bytes are in the server's %s", name)
			}
		}
	}
	// Broker-visible bytes: the broker's protocol trace, which carries every
	// payload it relayed. The control is that the trace does carry the agent's
	// public signing half, so an absent private one is absence, not blindness.
	trace := tp.brokerTrace()
	var k keyArtifact
	json.Unmarshal(a.file(agentStateDir+"/keys.json"), &k)
	signing, _ := base64.StdEncoding.DecodeString(k.SigningPrivateKey)
	sk, err := protocol.ParseSigningKey(signing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trace, enrollment.EncodeKey(sk.Verifying().Bytes())[:64]) {
		t.Fatal("control: the broker trace does not show the request's public signing half; it proves nothing")
	}
	for _, s := range text {
		if strings.Contains(trace, s) {
			t.Fatal("an agent private key crossed the broker")
		}
	}
	for _, r := range raw {
		if strings.Contains(trace, string(r)) {
			t.Fatal("an agent private key's raw bytes crossed the broker")
		}
	}
}

type eventsWatch struct {
	Error  string   `json:"error"`
	Events []string `json:"events"`
}

func TestCRED1IdentityArtifactIsAtomicAndPrivate(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)

	a.sh("rm -f /tmp/ev-ready /tmp/ev-stop /tmp/ev.json")
	a.detach("", probeBin, "watch-events", "-dir", agentStateDir, "-ready", "/tmp/ev-ready", "-until", "/tmp/ev-stop", "-out", "/tmp/ev.json")
	a.waitFor("/tmp/ev-ready")
	a.mustEnroll()
	a.sh("touch /tmp/ev-stop")
	a.waitFor("/tmp/ev.json")
	var w eventsWatch
	if err := json.Unmarshal(a.file("/tmp/ev.json"), &w); err != nil || w.Error != "" {
		t.Fatalf("instrument: %v %s", err, w.Error)
	}
	// Never partially: the identity artifact's name appears only by rename.
	// Written in place, it would be created and modified under its own name,
	// visible part-written in between.
	arrived := false
	for _, e := range w.Events {
		switch e {
		case "MOVED_TO identity.json":
			arrived = true
		case "CREATE identity.json", "MODIFY identity.json", "CLOSE_WRITE identity.json":
			t.Fatalf("the identity artifact was written in place (%s); it must appear whole, by rename: %v", e, w.Events)
		}
	}
	if !arrived {
		t.Fatalf("control: the watch never saw the identity artifact arrive: %v", w.Events)
	}

	if perm := strings.TrimSpace(a.mustExec("", "stat", "-c", "%a", agentStateDir+"/identity.json")); perm != "600" {
		t.Fatalf("identity artifact mode %s, want 600", perm)
	}
	var id identityArtifactV1
	d := json.NewDecoder(bytes.NewReader(a.file(agentStateDir + "/identity.json")))
	d.DisallowUnknownFields()
	if err := d.Decode(&id); err != nil {
		t.Fatal(err)
	}
	keysBytes := a.file(agentStateDir + "/keys.json")
	sum := sha256.Sum256(keysBytes)
	var k keyArtifact
	json.Unmarshal(keysBytes, &k)
	kp, _ := nkeys.FromSeed([]byte(k.NATSSeed))
	pub, _ := kp.PublicKey()
	token, err := jwt.ParseDecoratedJWT([]byte(id.PermanentCredentials))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if id.Version != identityVersion || id.AgentID != tok.bundle.AgentID || id.AgentKeySHA256 != hex.EncodeToString(sum[:]) ||
		id.ServiceSigningPublicKey != tok.bundle.ServiceSigningPublicKey ||
		id.ResultEncryptionPublicKey != tok.bundle.ResultEncryptionPublicKey ||
		claims.Subject != pub || claims.Name != tok.bundle.AgentID {
		t.Fatalf("the identity artifact does not hold the credential and both trust halves for these keys: %+v", id)
	}
}

func TestTLS1EveryProductionConnectionVerifiesBroker(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()

	// The server: a name its broker's certificate does not carry, and a CA
	// that did not issue it, each refuse startup.
	if _, log := b.refuse(serverConf{ServerName: "not-the-broker"}); !strings.Contains(log, "certificate") {
		t.Fatalf("server with the wrong name refused for another reason:\n%s", log)
	}
	other := newDeployment(t)
	b.writeFile(natsDir+"/ca.pem", string(other.nats.TLS.CACertPEM), 0o644)
	if _, log := b.refuse(serverConf{}); !strings.Contains(log, "certificate") {
		t.Fatalf("server trusting another CA refused for another reason:\n%s", log)
	}
	b.writeFile(natsDir+"/ca.pem", string(tp.dep.nats.TLS.CACertPEM), 0o644)
	b.start(serverConf{})
	tok := b.issue("web-1")

	// The agent: the same two, and neither sends a request.
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.configure("not-the-broker")
	if r := a.enroll(); r.Exit == 0 || !strings.Contains(a.log(), "certificate") {
		t.Fatalf("agent with the wrong name: exit %d:\n%s", r.Exit, a.log())
	}
	a.configure(brokerAlias)
	a.writeFile("/etc/keystone/ca.pem", string(other.nats.TLS.CACertPEM), 0o644)
	if r := a.enroll(); r.Exit == 0 || !strings.Contains(a.log(), "certificate") {
		t.Fatalf("agent trusting another CA: exit %d:\n%s", r.Exit, a.log())
	}
	if row := b.tokenRow(tok.bundle.TokenID); row["nats_public_key"] != nil {
		t.Fatal("an agent that could not verify the broker still reached the server")
	}

	// Verified, both work. The broker accepts only TLS-first TLS 1.3 (G49), so
	// every connection that succeeds here is one.
	a.writeFile("/etc/keystone/ca.pem", string(tp.dep.nats.TLS.CACertPEM), 0o644)
	a.mustEnroll()
	b.enrolled(tok)
}

func TestNON1SecondEnrollmentDoesNotChangeFirst(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	first, second := b.issue("web-1"), b.issue("web-2")
	a1, a2 := tp.agent("agent-1"), tp.agent("agent-2")
	a1.deliver(first.raw, 0o600)
	a1.mustEnroll()
	b.enrolled(first)
	beforeRow := b.agentRow(first.bundle.AgentID)
	beforeToken := b.tokenRow(first.bundle.TokenID)
	beforeIdentity := a1.file(agentStateDir + "/identity.json")

	a2.deliver(second.raw, 0o600)
	a2.mustEnroll()
	b.enrolled(second)
	if first.bundle.AgentID == second.bundle.AgentID {
		t.Fatal("two tokens assigned one identifier")
	}
	if after := b.agentRow(first.bundle.AgentID); fmt.Sprint(after) != fmt.Sprint(beforeRow) {
		t.Fatalf("enrolling agent 2 changed agent 1's record:\nbefore %v\nafter  %v", beforeRow, after)
	}
	if after := b.tokenRow(first.bundle.TokenID); fmt.Sprint(after) != fmt.Sprint(beforeToken) {
		t.Fatal("enrolling agent 2 changed agent 1's token record")
	}
	if after := a1.file(agentStateDir + "/identity.json"); !bytes.Equal(after, beforeIdentity) {
		t.Fatal("enrolling agent 2 changed agent 1's identity artifact")
	}
}

func TestID1AgentIdentifierIsServerAssigned(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	// A label that is itself a valid identifier, so a server that used the
	// label would pass every format check.
	label := strings.Repeat("ab", 16)
	tok := b.issue(label)
	if !hex32.MatchString(tok.bundle.AgentID) || tok.bundle.AgentID == label || !protocol.ValidIdentifier(tok.bundle.AgentID) {
		t.Fatalf("assigned identifier %q", tok.bundle.AgentID)
	}
	again := b.issue(label)
	if again.bundle.AgentID == tok.bundle.AgentID {
		t.Fatal("two tokens for one label were assigned the same identifier")
	}

	// The enrolling party cannot choose another: a request naming itself as a
	// different agent is refused, and nothing is recorded for it.
	c := tp.harness(t, tok.bundle.BootstrapCredentials)
	replies := make(chan *nats.Msg, 4)
	if _, err := c.ChanSubscribe(enrollment.ReplySubject(tok.bundle.TokenID), replies); err != nil {
		t.Fatal(err)
	}
	c.Flush()
	keys, err := enrollment.GenerateAgentKeys()
	if err != nil {
		t.Fatal(err)
	}
	chosen := strings.Repeat("cd", 16)
	payload, _ := json.Marshal(enrollment.Request{
		NATSPublicKey: keys.NATSPublicKey(), Kind: requestKindCredential,
		SigningPublicKey:    enrollment.EncodeKey(keys.Signing.Verifying().Bytes()),
		EncryptionPublicKey: enrollment.EncodeKey(keys.Decryption.Recipient().Bytes()),
	})
	env, err := enrollment.Seal(keys.Signing, protocol.ClassEnrollmentRequest, chosen, tok.bundle.TokenID, payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(enrollment.RequestSubject(tok.bundle.TokenID), env); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-replies:
		reply := verifiedReply(t, b.dep, m.Data)
		if reply.Kind == requestKindCredential {
			t.Fatalf("a request naming itself %s was issued a credential: %+v", chosen, reply)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("control: the service did not answer the harness's request at all")
	}
	if row := b.tokenRow(tok.bundle.TokenID); row["nats_public_key"] != nil {
		t.Fatal("a request naming another agent was recorded")
	}
	c.Close()

	// The real agent enrolls as the assigned identifier.
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.mustEnroll()
	b.enrolled(tok)
	var id identityArtifactV1
	json.Unmarshal(a.file(agentStateDir+"/identity.json"), &id)
	token, _ := jwt.ParseDecoratedJWT([]byte(id.PermanentCredentials))
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil || claims.Name != tok.bundle.AgentID || id.AgentID != tok.bundle.AgentID {
		t.Fatalf("the permanent identity is not the assigned one: %v %+v", err, claims)
	}
}

// verifiedReply opens an enrollment reply the service signed with the
// deployment's key.
func verifiedReply(t *testing.T, dep *deployment, data []byte) enrollmentReplyV1 {
	t.Helper()
	e, err := enrollment.OpenEnvelope(data, protocol.ClassEnrollmentReply, protocol.SenderEnrollmentService)
	if err != nil {
		t.Fatalf("reply is not an enrollment-reply envelope: %v", err)
	}
	if err := enrollment.Verify(dep.signing.Verifying(), e); err != nil {
		t.Fatalf("reply is not signed by the service key: %v", err)
	}
	var r enrollmentReplyV1
	if err := json.Unmarshal(e.Payload, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// harness connects the test process to the topology's broker with a
// credential, TLS-first and verifying the broker, the way production clients
// do. The broker is reached on the server network; a job container joins it
// for the duration, and a developer host routes to the bridge directly.
func (tp *topology) harness(t *testing.T, creds string) *nats.Conn {
	t.Helper()
	if host, err := os.Hostname(); err == nil && exec.Command("docker", "inspect", host).Run() == nil {
		if out, err := exec.Command("docker", "network", "connect", tp.serverNet, host).CombinedOutput(); err != nil &&
			!strings.Contains(string(out), "already exists") {
			t.Fatalf("join the server network: %v\n%s", err, out)
		}
		tp.cleanup(func() { exec.Command("docker", "network", "disconnect", "--force", tp.serverNet, host).Run() })
	}
	ip, err := exec.Command("docker", "inspect", "--format",
		fmt.Sprintf("{{ (index .NetworkSettings.Networks %q).IPAddress }}", tp.serverNet), tp.broker).Output()
	if err != nil || strings.TrimSpace(string(ip)) == "" {
		t.Fatalf("the broker has no address on %s: %v", tp.serverNet, err)
	}
	opts, err := enrollment.ClientOptions(tp.dep.nats.TLS.CACertPEM, brokerAlias, []byte(creds))
	if err != nil {
		t.Fatal(err)
	}
	c, err := nats.Connect("tls://"+strings.TrimSpace(string(ip))+":4222", append(opts, nats.Timeout(10*time.Second))...)
	if err != nil {
		t.Fatalf("harness connect: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}
