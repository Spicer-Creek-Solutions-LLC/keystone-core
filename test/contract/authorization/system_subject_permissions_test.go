//go:build contract

package authorizationcontract

import (
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

func TestNEG1AgentCannotPublishCommand(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedPublish(t, "ks.job."+agentOne+".cmd")
}
func TestNEG2AgentCannotPublishCancellation(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedPublish(t, "ks.job."+agentOne+".cancel")
}
func TestNEG3AgentCannotSubscribeOtherCommand(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedSubscribe(t, "ks.job."+agentTwo+".cmd")
}
func TestNEG4AgentCannotSubscribeOtherJobPrefix(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedSubscribe(t, "ks.job."+agentTwo+".>")
}
func TestNEG5AgentCannotPublishOtherResult(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedPublish(t, "ks.out."+agentTwo+".result")
}
func TestNEG6AgentCannotSubscribeOtherResult(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedSubscribe(t, "ks.out."+agentTwo+".result")
}
func TestNEG7RevokedBootstrapCannotConnect(t *testing.T) {
	d := runningBroker(t)

	// The control, and it is what stops this case passing for the wrong reason.
	// If no bootstrap identity could connect -- a broken resolver, an account
	// that never loaded -- the refusal below would look identical. ADR-0003
	// section 6 revokes ONE token at S5; the others keep working.
	if _, err := tryConnectAs(t, d, tokenOne, d.set.Bootstraps[tokenOne]); err != nil {
		t.Fatalf("an unrevoked bootstrap identity could not connect, so a refusal "+
			"below would prove nothing about revocation: %v", err)
	}

	revoked, ok := d.set.Bootstraps[tokenTwo]
	if !ok {
		t.Fatal("no bootstrap identity was generated for the revoked token")
	}
	if len(d.set.Revoked) == 0 || d.set.Revoked[0] != revoked.PublicKey {
		t.Fatalf("the deployment does not record %s as revoked; nothing is under test", tokenTwo)
	}

	// ADR-0004 section 9 marks this case as different in kind from the rest:
	// revocation removes the user from the account, so the refusal happens
	// while the connection is being established. "A test that connects
	// successfully and is refused on publish has proved something weaker than
	// revocation" -- so a nil error here is the failure, whatever happens after.
	conn, err := tryConnectAs(t, d, tokenTwo, revoked)
	if err == nil {
		conn.Close()
		t.Fatal("a revoked bootstrap identity established a connection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "authorization") {
		t.Fatalf("the connection failed, but not as an authorization refusal, so this "+
			"does not show revocation was the reason: %v", err)
	}
}
func TestNEG8PermanentAgentCannotPublishEnrollment(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedPublish(t, "ks.enroll."+tokenOne+".request")
}
func TestNEG9PermanentAgentCannotSubscribeEnrollment(t *testing.T) {
	d := runningBroker(t)
	// The whole plane in one line, which is why ADR-0004 section 2 made enroll a
	// plane rather than a class: spread across classes this would be several
	// rules, and several rules are several chances to miss one.
	agent(t, d, agentOne).deniedSubscribe(t, natsauth.EnrollmentPlane)
}
func TestNEG10AgentCannotSubscribeSystemSubjects(t *testing.T) {
	d := runningBroker(t)
	agent(t, d, agentOne).deniedSubscribe(t, "$SYS.>")
}
func TestNEG11AgentCannotDeleteCommandStream(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	// The stream EXISTS, so a refusal here cannot be a missing target.
	agent(t, d, agentOne).deniedPublish(t, "$JS.API.STREAM.DELETE."+natsauth.CommandStream)
}
func TestNEG12AgentCannotPullOtherConsumer(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	// agentTwo's consumer exists -- that is why the fixture creates one per
	// agent. Without it this case could not tell a permissions violation from a
	// consumer that is simply not there.
	agent(t, d, agentOne).deniedPublish(t,
		"$JS.API.CONSUMER.MSG.NEXT."+natsauth.CommandStream+"."+natsauth.CommandConsumer(agentTwo))
}
func TestNEG13AgentCannotPublishBareAcknowledgement(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	// POS-8 shows the agent may acknowledge on its own consumer. This shows the
	// bare wildcard is permitted to nobody, which is the other half of ADR-0004
	// section 6's justification for the trailing wildcard being bounded.
	agent(t, d, agentOne).deniedPublish(t, "$JS.ACK.>")
}
func TestNEG14CommandPublisherCannotSubscribeResults(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.CommandPublisher).
		allow(permitted{publish: "ks.job." + agentOne + ".cmd", inbox: true}).
		deniedSubscribe(t, "ks.out.*.result")
}
func TestNEG15PresenceConsumerCannotPublishCommands(t *testing.T) {
	d := runningBroker(t)
	// The presence consumer has NO publish grant at all, so its permitted
	// operation is a subscribe. ADR-0004 section 4 gives it "Publish: none", and
	// before G46 that rendered as an empty allow list -- which NATS reads as
	// unrestricted, so this case failed in a generated deployment.
	connectAsService(t, d, natsauth.PresenceConsumer).
		allow(permitted{subscribe: "ks.out.*.presence"}).
		deniedPublish(t, "ks.job."+agentOne+".cmd")
}

// agent connects as one agent with its own permitted operations declared. Every
// agent denial below uses it, so none of them can pass against a generator that
// denied the agent its own subjects too.
func agent(t *testing.T, d deployment, id string) *client {
	t.Helper()
	return connectAsAgent(t, d, id).allow(permitted{
		publish:   "ks.out." + id + ".result",
		subscribe: "ks.job." + id + ".>",
	})
}
