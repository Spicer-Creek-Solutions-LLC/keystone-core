//go:build contract

package authorizationcontract

import (
	"strings"
	"testing"
)

func TestPOS1CommandPublisherPublishesCommand(t *testing.T)           { pending(t, "POS-1") }
func TestPOS2CommandPublisherPublishesCancellation(t *testing.T)      { pending(t, "POS-2") }
func TestPOS3AgentSubscribesOwnCommand(t *testing.T)                  { pending(t, "POS-3") }
func TestPOS4AgentSubscribesOwnCancellation(t *testing.T)             { pending(t, "POS-4") }
func TestPOS5AgentPublishesOwnResult(t *testing.T)                    { pending(t, "POS-5") }
func TestPOS6AgentPublishesOwnPresence(t *testing.T)                  { pending(t, "POS-6") }
func TestPOS7AgentPullsOwnCommandConsumer(t *testing.T)               { pending(t, "POS-7") }
func TestPOS8AgentAcknowledgesOwnCommand(t *testing.T)                { pending(t, "POS-8") }
func TestPOS9PresenceConsumerSubscribesFleetPresence(t *testing.T)    { pending(t, "POS-9") }
func TestPOS10ResultConsumerPullsOwnConsumer(t *testing.T)            { pending(t, "POS-10") }
func TestPOS11BootstrapPublishesOwnRequest(t *testing.T)              { pending(t, "POS-11") }
func TestPOS12BootstrapSubscribesOwnReply(t *testing.T)               { pending(t, "POS-12") }
func TestPOS13EnrollmentServiceSubscribesRequests(t *testing.T)       { pending(t, "POS-13") }
func TestPOS14MonitoringSubscribesMaxDeliveriesAdvisory(t *testing.T) { pending(t, "POS-14") }
func TestPOS15CommandPublisherSubscribesOwnInbox(t *testing.T)        { pending(t, "POS-15") }
func TestPOS16EnrollmentServicePublishesReplies(t *testing.T)         { pending(t, "POS-16") }
func TestPOS17AgentPublishesOwnEvent(t *testing.T)                    { pending(t, "POS-17") }
func TestPOS18AgentSubscribesOwnInbox(t *testing.T)                   { pending(t, "POS-18") }
func TestPOS19MonitoringSubscribesFleetEvents(t *testing.T)           { pending(t, "POS-19") }
func TestPOS20ResultConsumerAcknowledgesResult(t *testing.T)          { pending(t, "POS-20") }
func TestPOS21ResultConsumerSubscribesOwnInbox(t *testing.T)          { pending(t, "POS-21") }
func TestPOS22MonitoringSubscribesTerminatedAdvisory(t *testing.T)    { pending(t, "POS-22") }
func TestPOS23MonitoringSubscribesLimitAdvisory(t *testing.T)         { pending(t, "POS-23") }

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
