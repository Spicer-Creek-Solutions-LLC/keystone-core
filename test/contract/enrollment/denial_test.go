//go:build contract

package enrollmentcontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// C05-I's third stage: what the service refuses, what it repeats, and how
// bootstrap access ends. The harness speaks the protocol itself, with the
// token's own bootstrap credential, so each case sends exactly the request it
// names.

// session is the harness holding one token's bootstrap connection.
type session struct {
	t       *testing.T
	b       *box
	tok     issued
	c       *nats.Conn
	replies chan *nats.Msg
}

func (tp *topology) session(t *testing.T, b *box, tok issued) *session {
	t.Helper()
	c := tp.harness(t, tok.bundle.BootstrapCredentials)
	s := &session{t: t, b: b, tok: tok, c: c, replies: make(chan *nats.Msg, 16)}
	if _, err := c.ChanSubscribe(enrollment.ReplySubject(tok.bundle.TokenID), s.replies); err != nil {
		t.Fatal(err)
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	return s
}

// request sends one request and returns the verified reply, or nil if none
// arrives. signer signs; keys supplies the halves presented; sender and
// subject default to the token's own.
func (s *session) request(keys enrollment.AgentKeys, kind string, opt ...func(*reqOpts)) *enrollmentReplyV1 {
	s.t.Helper()
	o := reqOpts{signer: keys.Signing, sender: s.tok.bundle.AgentID, token: s.tok.bundle.TokenID, subjectToken: s.tok.bundle.TokenID}
	for _, f := range opt {
		f(&o)
	}
	payload, _ := json.Marshal(enrollment.Request{
		NATSPublicKey: keys.NATSPublicKey(), Kind: kind,
		SigningPublicKey:    enrollment.EncodeKey(keys.Signing.Verifying().Bytes()),
		EncryptionPublicKey: enrollment.EncodeKey(keys.Decryption.Recipient().Bytes()),
	})
	env, err := enrollment.Seal(o.signer, protocol.ClassEnrollmentRequest, o.sender, o.token, payload, time.Now())
	if err != nil {
		s.t.Fatal(err)
	}
	if err := s.c.Publish(enrollment.RequestSubject(o.subjectToken), env); err != nil {
		s.t.Fatal(err)
	}
	s.c.Flush()
	select {
	case m := <-s.replies:
		r := verifiedReply(s.t, s.b.dep, m.Data)
		return &r
	case <-time.After(5 * time.Second):
		return nil
	}
}

type reqOpts struct {
	signer       *protocol.SigningKey
	sender       string
	token        string
	subjectToken string
}

func freshKeys(t *testing.T) enrollment.AgentKeys {
	t.Helper()
	k, err := enrollment.GenerateAgentKeys()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// denials is every denial record the service wrote for a token.
func (b *box) denials(token string) []string {
	b.t.Helper()
	var out []string
	for _, r := range b.query(auditAll).withAction(enrollment.ActionDenied) {
		if r["correlation_id"] == token {
			out = append(out, fmt.Sprint(r["result"]))
		}
	}
	return out
}

func denied(t *testing.T, what string, r *enrollmentReplyV1) {
	t.Helper()
	if r == nil || r.Kind != enrollment.KindDenied || r.PermanentJWT != "" || r.AgentID != "" {
		t.Fatalf("%s: want a bare denial, got %+v", what, r)
	}
}

func TestIDEM2SameKeysReturnSameIdentity(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	s := tp.session(t, b, tok)
	keys := freshKeys(t)
	first := s.request(keys, requestKindCredential)
	if first == nil || first.Kind != requestKindCredential || first.PermanentJWT == "" {
		t.Fatalf("first request: %+v", first)
	}
	for i := 0; i < 3; i++ {
		again := s.request(keys, requestKindCredential)
		if again == nil || again.PermanentJWT != first.PermanentJWT {
			t.Fatalf("retry %d with the same halves returned another identity: %+v", i, again)
		}
	}
	if n := len(b.query("SELECT agent_id FROM enrollment WHERE permanent_jwt IS NOT NULL").Rows); n != 1 {
		t.Fatalf("%d minted identities for one token", n)
	}
	if n := len(b.query(auditAll).withAction(enrollment.ActionCredentialIssued)); n != 1 {
		t.Fatalf("%d credential issuances for one token", n)
	}
}

func TestIDEM1DifferentKeysAreNotARetry(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok := b.issue("web-1")
	s := tp.session(t, b, tok)
	first := freshKeys(t)
	if r := s.request(first, requestKindCredential); r == nil || r.Kind != requestKindCredential {
		t.Fatalf("control: the first request was not answered with a credential: %+v", r)
	}
	recorded := b.tokenRow(tok.bundle.TokenID)["nats_public_key"]
	denied(t, "a re-presentation with other halves", s.request(freshKeys(t), requestKindCredential))
	if got := b.tokenRow(tok.bundle.TokenID)["nats_public_key"]; got != recorded {
		t.Fatalf("other halves replaced the recorded ones: %v -> %v", recorded, got)
	}
	if d := b.denials(tok.bundle.TokenID); len(d) != 1 {
		t.Fatalf("denial records %v, want exactly one", d)
	}
	// The first halves are still the token's.
	if r := s.request(first, requestKindCredential); r == nil || r.Kind != requestKindCredential {
		t.Fatalf("after the refusal, the recorded halves were refused too: %+v", r)
	}
}

func TestAUTH1RequestIdentityAndSignatureMustMatch(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	own, other := b.issue("web-1"), b.issue("web-2")
	s := tp.session(t, b, own)

	// Another token's bootstrap identity: the broker refuses the publish on
	// the other token's subject, so the service never sees it.
	errs := make(chan error, 4)
	s.c.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { errs <- err })
	s.request(freshKeys(t), requestKindCredential, func(o *reqOpts) {
		o.subjectToken, o.token, o.sender = other.bundle.TokenID, other.bundle.TokenID, other.bundle.AgentID
	})
	select {
	case err := <-errs:
		if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
			t.Fatalf("publishing as another token failed for another reason: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the broker let one token's bootstrap identity publish on another's subject")
	}
	if b.tokenRow(other.bundle.TokenID)["nats_public_key"] != nil {
		t.Fatal("a request from another token's identity was recorded")
	}

	// On its own subject, naming the other token in the signed envelope.
	denied(t, "a request naming another token", s.request(freshKeys(t), requestKindCredential, func(o *reqOpts) {
		o.token = other.bundle.TokenID
	}))

	// Halves presented with a signature by a different key.
	keys := freshKeys(t)
	denied(t, "a request not signed by its presented key", s.request(keys, requestKindCredential, func(o *reqOpts) {
		o.signer = freshKeys(t).Signing
	}))
	if b.tokenRow(own.bundle.TokenID)["nats_public_key"] != nil {
		t.Fatal("a refused request was recorded")
	}
	if d := b.denials(own.bundle.TokenID); len(d) != 2 {
		t.Fatalf("denial records %v, want the two refusals", d)
	}
	// The control: the same token, correctly signed, is answered.
	if r := s.request(keys, requestKindCredential); r == nil || r.Kind != requestKindCredential {
		t.Fatalf("control: a correct request was not answered: %+v", r)
	}
}

// enrolledAgent runs a complete enrollment and keeps a copy of the bundle, as an
// attacker who read it before it was removed would.
func enrolledAgent(t *testing.T, tp *topology, b *box, name string, ttl ...string) (issued, *agentBox) {
	t.Helper()
	r := b.create(alice, name, ttl...)
	tok := issued{raw: r.Stdout, bundle: b.bundle(r)}
	a := tp.agent(name)
	a.deliver(tok.raw, 0o600)
	a.mustEnroll()
	b.enrolled(tok)
	return tok, a
}

// agentKeys restores an enrolled agent's keys from its artifact.
func agentKeys(t *testing.T, a *agentBox) enrollment.AgentKeys {
	t.Helper()
	path := t.TempDir() + "/keys.json"
	if err := writePrivate(path, a.file(agentStateDir+"/keys.json")); err != nil {
		t.Fatal(err)
	}
	var k agentKeyArtifactV1
	json.Unmarshal(a.file(agentStateDir+"/keys.json"), &k)
	keys, err := enrollment.ReadKeyArtifact(path, k.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestSPENT1SpentTokenOnlyAnswersConfirmation(t *testing.T) {
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok, a := enrolledAgent(t, tp, b, "web-1")
	keys := agentKeys(t, a)
	s := tp.session(t, b, tok)

	denied(t, "a credential request on the spent token, with the enrolled halves", s.request(keys, requestKindCredential))
	denied(t, "a credential request on the spent token, with other halves", s.request(freshKeys(t), requestKindCredential))
	denied(t, "a confirmation with other halves", s.request(freshKeys(t), requestKindConfirm))
	if d := b.denials(tok.bundle.TokenID); len(d) != 3 {
		t.Fatalf("denial records %v, want one per refusal", d)
	}
	for i := 0; i < 2; i++ {
		r := s.request(keys, requestKindConfirm)
		if r == nil || r.Kind != requestKindConfirm || r.Identity != replyStateActive || r.Token != replyTokenSpent || r.AgentID != tok.bundle.AgentID {
			t.Fatalf("the enrolled agent's confirmation %d was not answered active and spent: %+v", i, r)
		}
	}
	if d := b.denials(tok.bundle.TokenID); len(d) != 3 {
		t.Fatalf("a confirmation was recorded as a denial: %v", d)
	}
	// A second enrollment with the kept bundle is the charter's spent token.
	again := tp.agent("thief")
	again.deliver(tok.raw, 0o600)
	if r := again.enroll(); r.Exit != 10 {
		t.Fatalf("enrolling again with a spent bundle: exit %d, want 10:\n%s", r.Exit, again.log())
	}
	if n := len(b.query("SELECT agent_id FROM agents").Rows); n != 1 {
		t.Fatalf("a spent token yielded a second identity: %d agents", n)
	}
}

func TestDENY1TokenFailuresAreIndistinguishableAndAudited(t *testing.T) {
	t.Parallel()
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})

	// Spent: a complete enrollment, then its bundle presented again.
	spent, _ := enrolledAgent(t, tp, b, "spent")
	// Expired: a token that outlives nothing, presented after its lifetime.
	expired := b.issue2("expired", "--ttl-seconds", fmt.Sprint(minimumTokenTTLSeconds))
	// Invalid: a token the service has no record of. Its bundle is genuine and
	// the broker accepts it; the server's record is gone.
	invalid := b.issue("invalid")
	b.stop()
	b.forget(invalid.bundle.TokenID)
	b.start(serverConf{})

	// What the caller sees: the exit code, standard output, and what the agent
	// wrote to standard error, which the fixture keeps in the agent's log.
	type seen struct {
		exit           int
		stdout, stderr string
	}
	results := map[string]seen{}
	run := func(name string, tok issued) {
		a := tp.agent("agent-" + name)
		a.deliver(tok.raw, 0o600)
		r := a.enroll()
		results[name] = seen{r.Exit, r.Stdout, a.log()}
	}
	run("spent", spent)
	run("invalid", invalid)
	waitPast(t, expired)
	run("expired", expired)

	for name, r := range results {
		if r.exit != 10 {
			t.Errorf("%s token: exit %d, want 10:\n%s", name, r.exit, r.stderr)
		}
		if r.stderr == "" {
			t.Fatalf("control: the %s refusal reported nothing, so comparing reports proves nothing", name)
		}
	}
	if results["spent"] != results["invalid"] || results["spent"] != results["expired"] {
		t.Fatalf("the three refusals are distinguishable to the caller: %+v", results)
	}
	// Every denial the service observed is recorded with its reason. An expired
	// credential is refused by the broker, so the service observes nothing.
	for name, tok := range map[string]issued{"spent": spent, "invalid": invalid} {
		d := b.denials(tok.bundle.TokenID)
		if len(d) == 0 {
			t.Errorf("the %s token's denial was not recorded", name)
		}
		for _, reason := range d {
			if !strings.Contains(reason, map[string]string{"spent": "spent", "invalid": "unknown"}[name]) {
				t.Errorf("the %s token's denial is recorded as %q", name, reason)
			}
		}
	}
}

// issue2 is issue with extra CLI arguments.
func (b *box) issue2(name string, args ...string) issued {
	b.t.Helper()
	r := b.create(alice, name, args...)
	return issued{raw: r.Stdout, bundle: b.bundle(r)}
}

// forget removes a token's record from the stopped server's store, so the
// token is one the service has never issued.
func (b *box) forget(token string) {
	b.t.Helper()
	var r struct {
		Error string `json:"error"`
	}
	b.probe("", &r, "write-records", "-db", storePath, "-statement",
		fmt.Sprintf("DELETE FROM enrollment WHERE token_id = '%s'", token))
	if r.Error != "" {
		b.t.Fatalf("forget %s: %s", token, r.Error)
	}
}

// waitPast waits until a token's bootstrap credential has expired.
func waitPast(t *testing.T, tok issued) {
	t.Helper()
	exp := time.Unix(bootstrapClaims(t, tok.bundle).Expires, 0)
	time.Sleep(time.Until(exp) + 2*time.Second)
}

func TestEXP1BrokerRefusesBootstrapAfterExpiry(t *testing.T) {
	t.Parallel()
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	ttl := fmt.Sprint(minimumTokenTTLSeconds)
	untouched := b.issue2("untouched", "--ttl-seconds", ttl)
	enrolled, _ := enrolledAgent(t, tp, b, "enrolled", "--ttl-seconds", ttl)

	// Proven usable before expiry: each credential connects and completes a
	// request -- a denial, for the spent one, is still a completed round trip.
	s := tp.session(t, b, untouched)
	if r := s.request(freshKeys(t), requestKindCredential); r == nil || r.Kind != requestKindCredential {
		t.Fatalf("control: the untouched credential completed no request before expiry: %+v", r)
	}
	s.c.Close()
	s = tp.session(t, b, enrolled)
	denied(t, "control: the spent credential before expiry", s.request(freshKeys(t), requestKindConfirm))
	s.c.Close()
	waitPast(t, untouched)
	waitPast(t, enrolled)
	for name, tok := range map[string]issued{"untouched": untouched, "enrolled": enrolled} {
		_, err := tp.connect(t, tok.bundle.BootstrapCredentials)
		if err == nil || !errors.Is(err, nats.ErrAuthorization) && !strings.Contains(strings.ToLower(err.Error()), "authorization") {
			t.Errorf("the %s bootstrap credential after expiry: %v, want the broker's authorization violation", name, err)
		}
	}
}

func TestOBS1EndOfBootstrapAccessIsObserved(t *testing.T) {
	t.Parallel()
	tp := newTopology(t)
	b := tp.server()
	b.start(serverConf{})
	tok, a := enrolledAgent(t, tp, b, "web-1", "--ttl-seconds", fmt.Sprint(minimumTokenTTLSeconds))
	keys := agentKeys(t, a)

	// After S4, the still-live credential is refused by the application, and
	// the refusal is recorded -- observed where it takes effect.
	s := tp.session(t, b, tok)
	before := len(b.denials(tok.bundle.TokenID))
	denied(t, "a fresh request on the spent token", s.request(keys, requestKindCredential))
	if after := len(b.denials(tok.bundle.TokenID)); after != before+1 {
		t.Fatalf("the refusal added %d denial records, want 1", after-before)
	}
	// Nothing claims the end of access from a claims read-back: no component
	// asked the broker for account claims at any point.
	if trace := tp.brokerTrace(); strings.Contains(trace, "$SYS.REQ.CLAIMS") {
		t.Fatal("something published to $SYS.REQ.CLAIMS; bootstrap access must not be judged from a claims read-back")
	} else if !strings.Contains(trace, enrollment.RequestSubject(tok.bundle.TokenID)) {
		t.Fatal("control: the broker trace does not show the token's requests; it proves nothing")
	}
	s.c.Close()

	// After expiry, the broker refuses the credential at connection.
	waitPast(t, tok)
	if _, err := tp.connect(t, tok.bundle.BootstrapCredentials); err == nil || !strings.Contains(strings.ToLower(err.Error()), "authorization") {
		t.Fatalf("after expiry: %v, want the broker's authorization violation", err)
	}
}

func writePrivate(path string, b []byte) error { return os.WriteFile(path, b, 0o600) }
