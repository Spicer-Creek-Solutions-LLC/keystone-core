//go:build contract

package enrollmentcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// C05-I's fifth stage: crash-harness.json, driven. Each case enables one
// boundary's hold, waits for the durable reached record AND the arrival
// observation the harness freezes, SIGKILLs the named process, restarts it
// without the hold, and checks the recovery the harness freezes. A reached
// record only says where the kill lands; every claim about recovery is read
// from the server's store, the broker, or the agent's artifacts.

// boundary is one row of crash-harness.json, as loaded.
func boundary(t *testing.T, id string) (process, fault string) {
	t.Helper()
	for _, b := range loadCrashHarnessSurface(t).Boundaries {
		if b.Case == id {
			return b.Process, b.Fault
		}
	}
	t.Fatalf("%s is not in crash-harness.json", id)
	return "", ""
}

// converged is the recovery every case shares: exactly one active identity,
// its token spent, one credential issued and one activation -- whatever the
// kill interrupted.
func (b *box) converged(tok issued) {
	b.t.Helper()
	b.enrolled(tok)
	if n := len(b.query("SELECT agent_id FROM agents").Rows); n != 1 {
		b.t.Fatalf("%d agent identities after recovery, want 1", n)
	}
	a := b.query(auditAll)
	if n := len(a.withAction(enrollment.ActionCredentialIssued)); n != 1 {
		b.t.Fatalf("%d credentials minted across the crash, want 1", n)
	}
	if n := len(a.withAction(enrollment.ActionActivated)); n != 1 {
		b.t.Fatalf("%d activations across the crash, want 1", n)
	}
}

// crashAgent runs the agent to one boundary and kills it there, after arrival
// has held; it returns once the process is gone.
func crashAgent(t *testing.T, id string, tp *topology, b *box, tok issued, arrival func(a *agentBox)) *agentBox {
	t.Helper()
	process, fault := boundary(t, id)
	if process != "agent" {
		t.Fatalf("%s kills %s, not the agent", id, process)
	}
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.configure(brokerAlias, faultHold{fault, 120000})
	a.startEnroll()
	a.reached(fault, 60*time.Second)
	arrival(a)
	if !a.running() {
		t.Fatalf("fixture: the agent exited before the kill at %s:\n%s", fault, a.log())
	}
	a.kill()
	a.configure(brokerAlias)
	return a
}

func (a *agentBox) mode(path string) string {
	a.t.Helper()
	return strings.TrimSpace(a.mustExec("", "stat", "-c", "%a", path))
}

func TestCRASH1RecoverS0ToS1(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	var captured enrollment.AgentKeys
	a := crashAgent(t, "CRASH-1", tp, b, tok, func(a *agentBox) {
		if a.mode(agentStateDir+"/keys.json") != "600" {
			t.Fatal("arrival: the key artifact is not mode 0600")
		}
		captured = agentKeys(t, a)
		if b.tokenRow(tok.bundle.TokenID)["nats_public_key"] != nil {
			t.Fatal("arrival: S1 was already recorded; the kill would not be at S0-S1")
		}
	})
	a.mustEnroll()
	b.converged(tok)
	if b.tokenRow(tok.bundle.TokenID)["nats_public_key"] != captured.NATSPublicKey() {
		t.Fatal("the restart presented halves other than the artifact written before the kill")
	}
}

func TestCRASH2RecoverS1ToS2(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	var recordedJWT any
	a := crashAgent(t, "CRASH-2", tp, b, tok, func(a *agentBox) {
		recordedJWT = b.tokenRow(tok.bundle.TokenID)["permanent_jwt"]
		if recordedJWT == nil {
			t.Fatal("arrival: no credential reply was issued before the hold")
		}
		if a.exists(agentStateDir + "/identity.json") {
			t.Fatal("arrival: the identity artifact exists; the kill would not be at S1-S2")
		}
	})
	a.mustEnroll()
	b.converged(tok)
	var id identityArtifactV1
	json.Unmarshal(a.file(agentStateDir+"/identity.json"), &id)
	if !strings.Contains(id.PermanentCredentials, fmt.Sprint(recordedJWT)) {
		t.Fatal("the restart holds a JWT other than the one S1 recorded before the kill")
	}
}

func TestCRASH3RecoverS2ToS3(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	var before []byte
	a := crashAgent(t, "CRASH-3", tp, b, tok, func(a *agentBox) {
		if a.mode(agentStateDir+"/identity.json") != "600" {
			t.Fatal("arrival: the identity artifact is not mode 0600")
		}
		before = a.file(agentStateDir + "/identity.json")
		var id identityArtifactV1
		json.Unmarshal(before, &id)
		if id.AgentKeySHA256 == "" || id.PermanentCredentials == "" {
			t.Fatal("arrival: the identity artifact is incomplete")
		}
	})

	// The restart proves presence with the restored key, and never re-enrolls:
	// the identity artifact is not written again.
	c := tp.harnessAs(t, natsauth.PresenceConsumer)
	presence := make(chan *nats.Msg, 16)
	c.ChanSubscribe(enrollment.PresenceSubject(tok.bundle.AgentID), presence)
	c.Flush()
	w := a.watch(agentStateDir)
	a.mustEnroll()
	b.converged(tok)
	for _, e := range w.stop() {
		if strings.HasSuffix(e, " identity.json") && !strings.HasPrefix(e, "OPEN") {
			t.Fatalf("the restart wrote the identity artifact again (%s); that is re-enrollment", e)
		}
	}
	if string(a.file(agentStateDir+"/identity.json")) != string(before) {
		t.Fatal("the identity artifact changed across the restart")
	}
	verifiedPresence(t, b, tok, presence)
}

// verifiedPresence requires a presence the S1-recorded AST-4 half verifies.
func verifiedPresence(t *testing.T, b *box, tok issued, presence chan *nats.Msg) {
	t.Helper()
	select {
	case m := <-presence:
		e, err := enrollment.OpenEnvelope(m.Data, protocol.ClassPresence, tok.bundle.AgentID)
		if err != nil || enrollment.Verify(b.recordedSigning(tok), e) != nil {
			t.Fatalf("the presence does not verify against the recorded AST-4 half: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no presence from the agent")
	}
}

func TestCRASH4RecoverS3ToS4(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	c := tp.harnessAs(t, natsauth.PresenceConsumer)
	presence := make(chan *nats.Msg, 16)
	c.ChanSubscribe(enrollment.PresenceSubject(tok.bundle.AgentID), presence)
	c.Flush()
	var row map[string]any
	a := crashAgent(t, "CRASH-4", tp, b, tok, func(a *agentBox) {
		verifiedPresence(t, b, tok, presence)
		b.waitActiveAndSpent(tok, 30*time.Second)
		row = b.agentRow(tok.bundle.AgentID)
	})
	a.mustEnroll()
	b.converged(tok)
	if fmt.Sprint(b.agentRow(tok.bundle.AgentID)) != fmt.Sprint(row) {
		t.Fatal("the repeated proof changed the identity")
	}
}

func TestCRASH5RecoverAgentS4ToS6(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	var row map[string]any
	var identity []byte
	a := crashAgent(t, "CRASH-5", tp, b, tok, func(a *agentBox) {
		b.waitActiveAndSpent(tok, 30*time.Second)
		row = b.agentRow(tok.bundle.AgentID)
		identity = a.file(agentStateDir + "/identity.json")
	})
	s := tp.session(t, b, tok)
	a.mustEnroll()
	b.converged(tok)
	if s.confirmations() == 0 {
		t.Fatal("the restart exited 0 without an S6 confirmation being delivered")
	}
	if fmt.Sprint(b.agentRow(tok.bundle.AgentID)) != fmt.Sprint(row) || string(a.file(agentStateDir+"/identity.json")) != string(identity) {
		t.Fatal("asking again for S6 changed the identity")
	}
}

func TestCRASH6RecoverServerS4ToS6(t *testing.T) {
	process, fault := boundary(t, "CRASH-6")
	if process != "server" {
		t.Fatalf("CRASH-6 kills %s, not the server", process)
	}
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{HoldAfterActivationMS: 120000})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.startEnroll()
	b.waitReached(fault, 60*time.Second)
	if !b.activeAndSpent(tok) {
		t.Fatal("arrival: the reached record without the single active-and-spent commit")
	}
	row := b.agentRow(tok.bundle.AgentID)
	b.sh("pkill -KILL keystone-server; true")
	waitGone(b)
	if !a.running() {
		t.Fatalf("the agent exited while the server was held and then killed:\n%s", a.log())
	}
	b.start(serverConf{})
	if code := a.wait(90 * time.Second); code != 0 {
		t.Fatalf("after the server restarted the agent exited %d:\n%s", code, a.log())
	}
	b.converged(tok)
	if fmt.Sprint(b.agentRow(tok.bundle.AgentID)) != fmt.Sprint(row) {
		t.Fatal("the restarted server changed the identity it had activated")
	}
}

// watch starts an inotify watch on dir; stop returns what it saw.
type dirWatch struct {
	a *agentBox
}

func (a *agentBox) watch(dir string) dirWatch {
	a.t.Helper()
	a.sh("rm -f /tmp/w-ready /tmp/w-stop /tmp/w.json")
	a.detach("", probeBin, "watch-events", "-dir", dir, "-ready", "/tmp/w-ready", "-until", "/tmp/w-stop", "-out", "/tmp/w.json")
	a.waitFor("/tmp/w-ready")
	return dirWatch{a}
}

func (w dirWatch) stop() []string {
	w.a.t.Helper()
	w.a.sh("touch /tmp/w-stop")
	w.a.waitFor("/tmp/w.json")
	var e eventsWatch
	if err := json.Unmarshal(w.a.file("/tmp/w.json"), &e); err != nil || e.Error != "" {
		w.a.t.Fatalf("instrument: %v %s", err, e.Error)
	}
	return e.Events
}

// serve starts the enrolled agent -- keystone-agent with no command -- in the
// background.
func (a *agentBox) serve() {
	a.t.Helper()
	a.sh("rm -f /tmp/serve.exit")
	a.detach("", "sh", "-c", "KEYSTONE_AGENT_CONFIG="+agentConfig+" keystone-agent 2>/tmp/serve.log; echo $? > /tmp/serve.exit")
}

func TestRECON1EnrolledAgentReconnectsWithoutEnrollment(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok, a := enrolledAgent(t, tp, b, "web-1")
	c := tp.harnessAs(t, natsauth.PresenceConsumer)
	presence := make(chan *nats.Msg, 64)
	c.ChanSubscribe(enrollment.PresenceSubject(tok.bundle.AgentID), presence)
	c.Flush()
	requests := strings.Count(tp.brokerTrace(), enrollment.RequestSubject(tok.bundle.TokenID))
	if requests == 0 {
		t.Fatal("control: the trace shows none of the enrollment's own requests, so counting them proves nothing")
	}

	// Twice: the second start is a restart of an enrolled agent.
	for i := 0; i < 2; i++ {
		for len(presence) > 0 {
			<-presence
		}
		a.serve()
		verifiedPresence(t, b, tok, presence)
		a.sh("pkill -TERM keystone-agent; true")
		a.waitFor("/tmp/serve.exit")
	}
	// The broker saw no enrollment request from the restarted agent, and
	// nothing was issued or activated again.
	if after := strings.Count(tp.brokerTrace(), enrollment.RequestSubject(tok.bundle.TokenID)); after != requests {
		t.Fatalf("the enrolled agent published on its enrollment subject again (%d -> %d trace lines)", requests, after)
	}
	b.converged(tok)
}

func TestRECON2MissingIdentityFailsLocally(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	// The bundle is on the host, so an agent that went looking for an identity
	// would find one to use.
	a.deliver(tok.raw, 0o600)
	connects := strings.Count(tp.brokerTrace(), "CONNECT")
	if connects == 0 {
		t.Fatal("control: the trace shows no CONNECT at all -- not even the server's -- so counting them proves nothing")
	}
	for name, setup := range map[string]string{
		"missing":    "rm -f " + agentStateDir + "/identity.json",
		"unreadable": "printf '{' > " + agentStateDir + "/identity.json && chmod 600 " + agentStateDir + "/identity.json",
		"exposed":    "chmod 644 " + agentStateDir + "/identity.json",
	} {
		a.sh(setup)
		a.serve()
		a.waitFor("/tmp/serve.exit")
		if code := strings.TrimSpace(string(a.file("/tmp/serve.exit"))); code != "1" {
			t.Fatalf("%s identity: exit %s, want 1", name, code)
		}
	}
	if after := strings.Count(tp.brokerTrace(), "CONNECT"); after != connects {
		t.Fatal("an agent without a usable identity connected to the broker")
	}
	if b.tokenRow(tok.bundle.TokenID)["nats_public_key"] != nil {
		t.Fatal("an agent without an identity attempted enrollment")
	}
	// The control: with the bundle, enrollment itself still works here.
	a.sh("rm -f " + agentStateDir + "/identity.json")
	a.mustEnroll()
	b.enrolled(tok)
}
