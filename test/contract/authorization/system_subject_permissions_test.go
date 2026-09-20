//go:build contract

package authorizationcontract

import "testing"

func TestNEG1AgentCannotPublishCommand(t *testing.T)               { pending(t, "NEG-1") }
func TestNEG2AgentCannotPublishCancellation(t *testing.T)          { pending(t, "NEG-2") }
func TestNEG3AgentCannotSubscribeOtherCommand(t *testing.T)        { pending(t, "NEG-3") }
func TestNEG4AgentCannotSubscribeOtherJobPrefix(t *testing.T)      { pending(t, "NEG-4") }
func TestNEG5AgentCannotPublishOtherResult(t *testing.T)           { pending(t, "NEG-5") }
func TestNEG6AgentCannotSubscribeOtherResult(t *testing.T)         { pending(t, "NEG-6") }
func TestNEG7RevokedBootstrapCannotConnect(t *testing.T)           { pending(t, "NEG-7") }
func TestNEG8PermanentAgentCannotPublishEnrollment(t *testing.T)   { pending(t, "NEG-8") }
func TestNEG9PermanentAgentCannotSubscribeEnrollment(t *testing.T) { pending(t, "NEG-9") }
func TestNEG10AgentCannotSubscribeSystemSubjects(t *testing.T)     { pending(t, "NEG-10") }
func TestNEG11AgentCannotDeleteCommandStream(t *testing.T)         { pending(t, "NEG-11") }
func TestNEG12AgentCannotPullOtherConsumer(t *testing.T)           { pending(t, "NEG-12") }
func TestNEG13AgentCannotPublishBareAcknowledgement(t *testing.T)  { pending(t, "NEG-13") }
func TestNEG14CommandPublisherCannotSubscribeResults(t *testing.T) { pending(t, "NEG-14") }
func TestNEG15PresenceConsumerCannotPublishCommands(t *testing.T)  { pending(t, "NEG-15") }
