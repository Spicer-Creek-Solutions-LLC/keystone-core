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

// C05-I's fourth stage: the frozen holds, and the orders only a hold makes
// observable. crash-harness.json fixes each mechanism; a hold's reached record
// says only WHERE a process is, and every case below makes its ordering claim
// from an independent observation -- the broker, or the server's store read
// on a connection of its own.

const holdMS = 4000

// harnessAs connects the harness as one of the deployment's service
// principals.
func (tp *topology) harnessAs(t *testing.T, p natsauth.Principal) *nats.Conn {
	t.Helper()
	creds, err := natsauth.Credentials(tp.dep.nats.Services[p])
	if err != nil {
		t.Fatal(err)
	}
	return tp.harness(t, string(creds))
}

// activeAndSpent reads, on the probe's own read-only connection, whether S4's
// single write has happened.
func (b *box) activeAndSpent(tok issued) bool {
	b.t.Helper()
	rows := b.query(fmt.Sprintf(`SELECT e.state AS token, a.state AS agent FROM enrollment e
		LEFT JOIN agents a ON a.agent_id = e.agent_id WHERE e.token_id = '%s'`, tok.bundle.TokenID)).Rows
	return len(rows) == 1 && rows[0]["token"] == "spent" && rows[0]["agent"] == "active"
}

func (b *box) waitActiveAndSpent(tok issued, timeout time.Duration) {
	b.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b.activeAndSpent(tok) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.t.Fatal("active-and-spent never became readable")
}

// recordedSigning is the signing half S1 recorded for a token.
func (b *box) recordedSigning(tok issued) *protocol.VerifyingKey {
	b.t.Helper()
	rows := b.query(fmt.Sprintf("SELECT hex(signing_public_key) AS k FROM enrollment WHERE token_id = '%s'", tok.bundle.TokenID)).Rows
	if len(rows) != 1 {
		b.t.Fatal("no S1 record")
	}
	raw := make([]byte, len(rows[0]["k"].(string))/2)
	fmt.Sscanf(rows[0]["k"].(string), "%X", &raw)
	v, err := protocol.ParseVerifyingKey(raw)
	if err != nil {
		b.t.Fatalf("recorded signing half: %v", err)
	}
	return v
}

func TestORD1PermanentConnectionPrecedesSpentWrite(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.configure(brokerAlias, faultHold{faultAgentAfterIdentityWrite, holdMS})

	// Subscribed before the agent starts, as the presence consumer, so "no
	// presence" means none since the subscription began.
	c := tp.harnessAs(t, natsauth.PresenceConsumer)
	presence := make(chan *nats.Msg, 16)
	if _, err := c.ChanSubscribe(enrollment.PresenceSubject(tok.bundle.AgentID), presence); err != nil {
		t.Fatal(err)
	}
	c.Flush()

	a.startEnroll()
	a.reached(faultAgentAfterIdentityWrite, 30*time.Second)
	held := time.Now()
	if len(presence) != 0 {
		t.Fatal("the broker delivered presence while the agent was held before its proof")
	}
	if b.activeAndSpent(tok) {
		t.Fatal("active-and-spent was committed while the agent was held before its proof")
	}
	if !a.running() || time.Since(held) > holdMS*time.Millisecond/2 {
		t.Fatal("fixture: the observations above were not made inside the hold")
	}

	select {
	case m := <-presence:
		e, err := enrollment.OpenEnvelope(m.Data, protocol.ClassPresence, tok.bundle.AgentID)
		if err != nil {
			t.Fatalf("the proof is not a presence envelope from the agent: %v", err)
		}
		if err := enrollment.Verify(b.recordedSigning(tok), e); err != nil {
			t.Fatal("the proof's signature does not verify against the AST-4 half S1 recorded")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no presence after the hold")
	}
	b.waitActiveAndSpent(tok, 30*time.Second)
	if code := a.wait(60 * time.Second); code != 0 {
		t.Fatalf("the agent exited %d:\n%s", code, a.log())
	}
}

func TestS61SuccessRequiresConfirmation(t *testing.T) {
	t.Run("the server is held after activation", func(t *testing.T) {
		tp := newTopology(t)
		b := tp.server()
		b.start(serverConf{HoldAfterActivationMS: holdMS})
		tok := b.issue("web-1")
		a := tp.agent("agent-1")
		a.deliver(tok.raw, 0o600)

		s := tp.session(t, b, tok)
		a.startEnroll()
		b.waitReached(faultServerAfterActivation, 30*time.Second)
		held := time.Now()
		if !b.activeAndSpent(tok) {
			t.Fatal("the server recorded reaching its after-commit hold without the commit")
		}
		if n := s.confirmations(); n != 0 {
			t.Fatalf("%d S6 confirmations were delivered while the server was held after the commit", n)
		}
		if !a.running() {
			t.Fatalf("the agent exited while the server was held, before any S6:\n%s", a.log())
		}
		if time.Since(held) > holdMS*time.Millisecond/2 {
			t.Fatal("fixture: the observations above were not made inside the hold")
		}
		if code := a.wait(60 * time.Second); code != 0 {
			t.Fatalf("after the hold the agent exited %d:\n%s", code, a.log())
		}
		if n := s.confirmations(); n == 0 {
			t.Fatal("the agent exited 0 and no service-signed S6 confirmation was delivered")
		}
		if !b.activeAndSpent(tok) {
			t.Fatal("the store does not show active-and-spent")
		}
	})

	// D-C05-3: the bootstrap credential expires between S4 and S6. The agent
	// must not exit 0, and its working identity must survive.
	t.Run("the bootstrap credential expires between S4 and S6", func(t *testing.T) {
		tp := newTopology(t)
		b := tp.server()
		b.start(serverConf{HoldAfterActivationMS: (minimumTokenTTLSeconds + 30) * 1000})
		tok := b.issue2("web-1", "--ttl-seconds", fmt.Sprint(minimumTokenTTLSeconds))
		a := tp.agent("agent-1")
		a.deliver(tok.raw, 0o600)
		a.startEnroll()
		b.waitReached(faultServerAfterActivation, 30*time.Second)
		if code := a.wait(2 * time.Minute); code != 10 {
			t.Fatalf("with S6 unreachable before expiry the agent exited %d, want 10:\n%s", code, a.log())
		}
		if !b.activeAndSpent(tok) || !a.exists(agentStateDir+"/identity.json") {
			t.Fatal("the permanent identity did not survive an expiry between S4 and S6")
		}
	})
}

// waitReached waits for the server's durable fault.reached record.
func (b *box) waitReached(key string, timeout time.Duration) {
	b.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(b.query(auditAll).withAction("fault.reached:"+key)) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.t.Fatalf("the server never recorded reaching %s", key)
}

// confirmations counts the service-signed S6 confirmations the session has
// received on the token's reply subject since it subscribed.
func (s *session) confirmations() int {
	s.t.Helper()
	n := 0
	for {
		select {
		case m := <-s.replies:
			r := verifiedReply(s.t, s.b.dep, m.Data)
			if r.Kind == requestKindConfirm && r.Identity == replyStateActive && r.Token == replyTokenSpent {
				s.seen++
			}
			continue
		default:
		}
		break
	}
	n = s.seen
	return n
}

func TestKEY2PersistedKeysSurvivePreRequestCrash(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.configure(brokerAlias, faultHold{faultAgentBeforeRequest, 60000})

	a.sh("rm -f /tmp/ev-ready /tmp/ev-stop /tmp/ev.json")
	a.detach("", probeBin, "watch-events", "-dir", agentStateDir, "-ready", "/tmp/ev-ready", "-until", "/tmp/ev-stop", "-out", "/tmp/ev.json")
	a.waitFor("/tmp/ev-ready")
	a.startEnroll()
	a.reached(faultAgentBeforeRequest, 30*time.Second)

	// At the boundary: the artifact is complete, private, and was not sent.
	if perm := strings.TrimSpace(a.mustExec("", "stat", "-c", "%a", agentStateDir+"/keys.json")); perm != "600" {
		t.Fatalf("key artifact mode %s, want 600", perm)
	}
	captured := agentKeys(t, a)
	if b.tokenRow(tok.bundle.TokenID)["nats_public_key"] != nil {
		t.Fatal("the agent sent S1 before its hold; the boundary is not before the request")
	}
	a.kill()
	a.sh("touch /tmp/ev-stop")
	a.waitFor("/tmp/ev.json")
	var w eventsWatch
	json.Unmarshal(a.file("/tmp/ev.json"), &w)
	arrived := false
	for _, e := range w.Events {
		switch e {
		case "MOVED_TO keys.json":
			arrived = true
		case "CREATE keys.json", "MODIFY keys.json", "CLOSE_WRITE keys.json":
			t.Fatalf("the key artifact was written in place (%s): %v", e, w.Events)
		}
	}
	if !arrived {
		t.Fatalf("control: the watch never saw the key artifact arrive: %v", w.Events)
	}

	// The restart presents the same halves, and they are the ones recorded.
	a.configure(brokerAlias)
	a.mustEnroll()
	b.enrolled(tok)
	rows := b.query(fmt.Sprintf("SELECT nats_public_key, hex(signing_public_key) AS s, hex(encryption_public_key) AS e FROM enrollment WHERE token_id = '%s'", tok.bundle.TokenID)).Rows
	hexOf := func(b []byte) string { return strings.ToUpper(fmt.Sprintf("%x", b)) }
	if rows[0]["nats_public_key"] != captured.NATSPublicKey() ||
		rows[0]["s"] != hexOf(captured.Signing.Verifying().Bytes()) ||
		rows[0]["e"] != hexOf(captured.Decryption.Recipient().Bytes()) {
		t.Fatal("after the crash the agent presented other public halves than the artifact it wrote before S1")
	}
}

func TestLEDGER1LedgerExistsBeforeActivation(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	a := tp.agent("agent-1")
	a.deliver(tok.raw, 0o600)
	a.configure(brokerAlias, faultHold{faultAgentAfterIdentityWrite, holdMS})
	a.startEnroll()
	a.reached(faultAgentAfterIdentityWrite, 30*time.Second)

	// At the last boundary before S3's proof -- so before the server can reach
	// S4 -- the ledger exists and is a ledger.
	if b.activeAndSpent(tok) {
		t.Fatal("control: the server already reached S4, so this is not a pre-S4 observation")
	}
	var r records
	a.probe("", &r, "read-records", "-db", agentStateDir+"/ledger.db", "-query", "SELECT version FROM schema_migrations")
	if r.Error != "" || len(r.Rows) == 0 {
		t.Fatalf("no durable ledger when the agent was held before its proof: %s %v", r.Error, r.Rows)
	}
	if code := a.wait(60 * time.Second); code != 0 {
		t.Fatalf("the agent exited %d:\n%s", code, a.log())
	}
	b.enrolled(tok)
}
