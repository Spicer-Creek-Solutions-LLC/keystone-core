//go:build contract

package authorizationcontract

import (
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

func TestPOS1CommandPublisherPublishesCommand(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.CommandPublisher).publishPermitted(t, "ks.job."+agentOne+".cmd")
}
func TestPOS2CommandPublisherPublishesCancellation(t *testing.T) {
	d := runningBroker(t)
	// agentTwo rather than agentOne: the grant is `ks.job.*.cancel`, and a case
	// that only ever named one agent would pass against a generator that had
	// scoped the command publisher to that agent alone.
	connectAsService(t, d, natsauth.CommandPublisher).publishPermitted(t, "ks.job."+agentTwo+".cancel")
}
func TestPOS3AgentSubscribesOwnCommand(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).subscribePermitted(t, "ks.job."+agentOne+".cmd")
}
func TestPOS4AgentSubscribesOwnCancellation(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).subscribePermitted(t, "ks.job."+agentOne+".cancel")
}
func TestPOS5AgentPublishesOwnResult(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).publishPermitted(t, "ks.out."+agentOne+".result")
}
func TestPOS6AgentPublishesOwnPresence(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).publishPermitted(t, "ks.out."+agentOne+".presence")
}
func TestPOS7AgentPullsOwnCommandConsumer(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	connectAsAgent(t, d, agentOne).requestPermitted(t,
		"$JS.API.CONSUMER.MSG.NEXT."+natsauth.CommandStream+"."+natsauth.CommandConsumer(agentOne),
		[]byte(`{"batch":1,"expires":1000000000}`))
}
func TestPOS8AgentAcknowledgesOwnCommand(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	// An acknowledgement carries per-message tokens the grant cannot enumerate,
	// which is why ADR-0004 section 6 justifies the trailing wildcard. It is
	// bounded to one stream and one consumer, and NEG-13 is the case that keeps
	// the bare wildcard out.
	connectAsAgent(t, d, agentOne).publishPermitted(t,
		"$JS.ACK."+natsauth.CommandStream+"."+natsauth.CommandConsumer(agentOne)+".1.1.1.0.0")
}
func TestPOS9PresenceConsumerSubscribesFleetPresence(t *testing.T) {
	d := runningBroker(t)
	// The wildcard is the grant: presence is a fleet view, and ADR-0004 § 4
	// documents this as one of the four permitted exceptions.
	connectAsService(t, d, natsauth.PresenceConsumer).subscribePermitted(t, "ks.out.*.presence")
}
func TestPOS10ResultConsumerPullsOwnConsumer(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	connectAsService(t, d, natsauth.ResultConsumer).requestPermitted(t,
		"$JS.API.CONSUMER.MSG.NEXT."+natsauth.ResultStream+"."+natsauth.ResultConsumerName,
		[]byte(`{"batch":1,"expires":1000000000}`))
}
func TestPOS11BootstrapPublishesOwnRequest(t *testing.T) {
	d := runningBroker(t)
	connectAsBootstrap(t, d, tokenOne).publishPermitted(t, "ks.enroll."+tokenOne+".request")
}
func TestPOS12BootstrapSubscribesOwnReply(t *testing.T) {
	d := runningBroker(t)
	connectAsBootstrap(t, d, tokenOne).subscribePermitted(t, "ks.enroll."+tokenOne+".reply")
}
func TestPOS13EnrollmentServiceSubscribesRequests(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.EnrollmentService).subscribePermitted(t, "ks.enroll.*.request")
}
func TestPOS14MonitoringSubscribesMaxDeliveriesAdvisory(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	// The assertion is the PERMISSION, not the arrival of an advisory. An
	// advisory that never fires is indistinguishable from one that was refused
	// if the test waits for a message, which is what this case's frozen
	// requirement means by distinguishing authorization from an absent advisory
	// source.
	connectAsService(t, d, natsauth.MonitoringRole).subscribePermitted(t,
		"$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES."+natsauth.CommandStream+".*")
}
func TestPOS15CommandPublisherSubscribesOwnInbox(t *testing.T) {
	d := runningBroker(t)
	// Without this the command publisher receives no PubAck for anything it
	// publishes to KS_CMD -- ADR-0002 § 6 is why the grant exists at all.
	connectAsService(t, d, natsauth.CommandPublisher).subscribeToOwnInboxPermitted(t)
}
func TestPOS16EnrollmentServicePublishesReplies(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.EnrollmentService).publishPermitted(t, "ks.enroll."+tokenOne+".reply")
}
func TestPOS17AgentPublishesOwnEvent(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).publishPermitted(t, "ks.out."+agentOne+".event")
}
func TestPOS18AgentSubscribesOwnInbox(t *testing.T) {
	d := runningBroker(t)
	connectAsAgent(t, d, agentOne).subscribeToOwnInboxPermitted(t)
}
func TestPOS19MonitoringSubscribesFleetEvents(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.MonitoringRole).subscribePermitted(t, "ks.out.*.event")
}
func TestPOS20ResultConsumerAcknowledgesResult(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	connectAsService(t, d, natsauth.ResultConsumer).publishPermitted(t,
		"$JS.ACK."+natsauth.ResultStream+"."+natsauth.ResultConsumerName+".1.1.1.0.0")
}
func TestPOS21ResultConsumerSubscribesOwnInbox(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.ResultConsumer).subscribeToOwnInboxPermitted(t)
}
func TestPOS22MonitoringSubscribesTerminatedAdvisory(t *testing.T) {
	d := runningBroker(t)
	provisionJetStream(t, d)
	connectAsService(t, d, natsauth.MonitoringRole).subscribePermitted(t,
		"$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED."+natsauth.CommandStream+".*")
}
func TestPOS23MonitoringSubscribesLimitAdvisory(t *testing.T) {
	d := runningBroker(t)
	connectAsService(t, d, natsauth.MonitoringRole).subscribePermitted(t,
		"$JS.EVENT.ADVISORY.API.LIMIT_REACHED")
}

// GEN-3 and GEN-4 are the pair ADR-0004 section 8 is explicit about: "a positive
// list shorter than the permission list lets C03 generate a JWT missing a
// granted permission and still pass every case". One direction catches a
// permission too wide, the other a permission too narrow, and a suite with only
// the first cannot see an agent quietly losing the ability to publish its own
// results.
//
// Both read the DECODED JWT, not the generator's Go record of what it intended
// to put there, and compare against frozenPermissionMatrix -- which is C03-A's
// independent transcription of ADR-0004 sections 4 and 6, not a list derived
// from generator code.

func TestGEN3GeneratedPermissionsContainNoExtraGrant(t *testing.T) {
	d := generated(t)
	frozen := frozenSet()
	got := generatedSet(t, d)

	if len(got) == 0 {
		t.Fatal("no generated grant was read; the comparison below would pass by checking nothing")
	}
	for key := range got {
		if !frozen[key] {
			principal, direction, subject := splitKey(key)
			t.Errorf("generated %s grant for %s is absent from the frozen matrix: %s",
				direction, principal, subject)
		}
	}
}

func TestGEN4GeneratedPermissionsOmitNoGrant(t *testing.T) {
	d := generated(t)
	frozen := frozenSet()
	got := generatedSet(t, d)

	if len(frozen) == 0 {
		t.Fatal("the frozen matrix is empty; the comparison below would pass by checking nothing")
	}
	for key := range frozen {
		if !got[key] {
			principal, direction, subject := splitKey(key)
			t.Errorf("frozen %s grant for %s is absent from every generated JWT: %s",
				direction, principal, subject)
		}
	}
}

func splitKey(key string) (principal, direction, subject string) {
	parts := strings.SplitN(key, "\x00", 3)
	return parts[0], parts[1], parts[2]
}
