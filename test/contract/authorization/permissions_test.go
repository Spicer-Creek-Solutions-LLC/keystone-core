//go:build contract

package authorizationcontract

import "testing"

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
func TestGEN3GeneratedPermissionsContainNoExtraGrant(t *testing.T)    { pending(t, "GEN-3") }
func TestGEN4GeneratedPermissionsOmitNoGrant(t *testing.T)            { pending(t, "GEN-4") }
