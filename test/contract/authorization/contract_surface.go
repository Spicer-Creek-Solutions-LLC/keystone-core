//go:build contract

package authorizationcontract

import "testing"

type acceptanceCase struct {
	TestName    string
	Requirement string
}

// This file is C03-A's immutable acceptance surface. C03-I may replace the
// pending test bodies, but it may not change these names, requirements, or the
// independently recorded permission matrix.
var acceptanceContractSurface = map[string]acceptanceCase{
	"POS-1":  {"TestPOS1CommandPublisherPublishesCommand", "the command publisher must publish commands for any agent"},
	"POS-2":  {"TestPOS2CommandPublisherPublishesCancellation", "the command publisher must publish cancellations for any agent"},
	"POS-3":  {"TestPOS3AgentSubscribesOwnCommand", "an agent must subscribe to its own command subject"},
	"POS-4":  {"TestPOS4AgentSubscribesOwnCancellation", "an agent must subscribe to its own cancellation subject"},
	"POS-5":  {"TestPOS5AgentPublishesOwnResult", "an agent must publish its own result"},
	"POS-6":  {"TestPOS6AgentPublishesOwnPresence", "an agent must publish its own presence"},
	"POS-7":  {"TestPOS7AgentPullsOwnCommandConsumer", "an agent must reach its exact KS_CMD consumer-next API and the test must distinguish authorization from a missing consumer"},
	"POS-8":  {"TestPOS8AgentAcknowledgesOwnCommand", "an agent must publish acknowledgements for its exact KS_CMD consumer"},
	"POS-9":  {"TestPOS9PresenceConsumerSubscribesFleetPresence", "the presence consumer must subscribe to fleet presence"},
	"POS-10": {"TestPOS10ResultConsumerPullsOwnConsumer", "the result consumer must reach its exact KS_RES consumer-next API and the test must distinguish authorization from a missing consumer"},
	"POS-11": {"TestPOS11BootstrapPublishesOwnRequest", "a bootstrap identity must publish its own enrollment request"},
	"POS-12": {"TestPOS12BootstrapSubscribesOwnReply", "a bootstrap identity must subscribe to its own enrollment reply"},
	"POS-13": {"TestPOS13EnrollmentServiceSubscribesRequests", "the enrollment service must subscribe to enrollment requests"},
	"POS-14": {"TestPOS14MonitoringSubscribesMaxDeliveriesAdvisory", "the monitoring role must subscribe to the KS_CMD max-deliveries advisory and the test must distinguish authorization from an absent advisory source"},
	"POS-15": {"TestPOS15CommandPublisherSubscribesOwnInbox", "the command publisher must subscribe to its own reply inbox prefix"},
	"POS-16": {"TestPOS16EnrollmentServicePublishesReplies", "the enrollment service must publish enrollment replies"},
	"POS-17": {"TestPOS17AgentPublishesOwnEvent", "an agent must publish its own lifecycle event"},
	"POS-18": {"TestPOS18AgentSubscribesOwnInbox", "an agent must subscribe to its own reply inbox prefix"},
	"POS-19": {"TestPOS19MonitoringSubscribesFleetEvents", "the monitoring role must subscribe to fleet lifecycle events"},
	"POS-20": {"TestPOS20ResultConsumerAcknowledgesResult", "the result consumer must publish acknowledgements for its exact KS_RES consumer"},
	"POS-21": {"TestPOS21ResultConsumerSubscribesOwnInbox", "the result consumer must subscribe to its own reply inbox prefix"},
	"POS-22": {"TestPOS22MonitoringSubscribesTerminatedAdvisory", "the monitoring role must subscribe to the KS_CMD message-terminated advisory and the test must distinguish authorization from an absent advisory source"},
	"POS-23": {"TestPOS23MonitoringSubscribesLimitAdvisory", "the monitoring role must subscribe to this account's API limit advisory"},
	"NEG-1":  {"TestNEG1AgentCannotPublishCommand", "an agent must be denied publishing a command for any agent"},
	"NEG-2":  {"TestNEG2AgentCannotPublishCancellation", "an agent must be denied publishing a cancellation for any agent"},
	"NEG-3":  {"TestNEG3AgentCannotSubscribeOtherCommand", "an agent must be denied subscribing to another agent's command"},
	"NEG-4":  {"TestNEG4AgentCannotSubscribeOtherJobPrefix", "an agent must be denied subscribing to another agent's job prefix"},
	"NEG-5":  {"TestNEG5AgentCannotPublishOtherResult", "an agent must be denied publishing another agent's result"},
	"NEG-6":  {"TestNEG6AgentCannotSubscribeOtherResult", "an agent must be denied subscribing to another agent's result"},
	"NEG-7":  {"TestNEG7RevokedBootstrapCannotConnect", "a revoked bootstrap identity must be refused at connection establishment, not after connecting"},
	"NEG-8":  {"TestNEG8PermanentAgentCannotPublishEnrollment", "a permanent agent must be denied publishing to the enrollment plane"},
	"NEG-9":  {"TestNEG9PermanentAgentCannotSubscribeEnrollment", "a permanent agent must be denied subscribing to the enrollment plane"},
	"NEG-10": {"TestNEG10AgentCannotSubscribeSystemSubjects", "an agent must be denied subscribing to system subjects"},
	"NEG-11": {"TestNEG11AgentCannotDeleteCommandStream", "an agent must receive a permissions violation for the KS_CMD stream-delete API, distinguished from every other refusal"},
	"NEG-12": {"TestNEG12AgentCannotPullOtherConsumer", "an agent must receive a permissions violation for another agent's consumer-next API, distinguished from a missing consumer"},
	"NEG-13": {"TestNEG13AgentCannotPublishBareAcknowledgement", "an agent must receive a permissions violation for the bare acknowledgement wildcard, distinguished from every other refusal"},
	"NEG-14": {"TestNEG14CommandPublisherCannotSubscribeResults", "the command publisher must be denied subscribing to results"},
	"NEG-15": {"TestNEG15PresenceConsumerCannotPublishCommands", "the presence consumer must be denied publishing commands"},
	"GEN-1":  {"TestGEN1TwoAccountsAndNoSystemPrincipals", "exactly one Keystone account and one system account must exist, with no Keystone principal in the system account"},
	"GEN-2":  {"TestGEN2EveryPrincipalHasDistinctIdentity", "every service principal and agent must have a distinct identity"},
	"GEN-3":  {"TestGEN3GeneratedPermissionsContainNoExtraGrant", "no generated JWT may contain a permission absent from the frozen matrix"},
	"GEN-4":  {"TestGEN4GeneratedPermissionsOmitNoGrant", "no permission in the frozen matrix may be absent from a generated JWT"},
	"GEN-5":  {"TestGEN5AccountLimitsAreFinite", "every ADR-0002 account limit must be present and finite, and stream and consumer limit values must be rendered for C06"},
	"GEN-6":  {"TestGEN6IdentifiersAreValidated", "the generator must refuse invalid and reserved identifiers"},
	"GEN-7":  {"TestGEN7OperatorSeedIsOutsideDeploymentProcesses", "the operator seed must be unreachable from every process the deployment runs"},
}

type permissionGrant struct {
	Principal string
	Direction string
	Subject   string
}

// frozenPermissionMatrix is an independent, machine-readable transcription of
// ADR-0004 sections 4 and 6. Generated JWTs are inputs to this matrix, never
// expectations used to construct it.
var frozenPermissionMatrix = []permissionGrant{
	{"command-publisher", "publish", "ks.job.*.cmd"},
	{"command-publisher", "publish", "ks.job.*.cancel"},
	{"command-publisher", "subscribe", "<own-inbox>.>"},
	{"enrollment-service", "publish", "ks.enroll.*.reply"},
	{"enrollment-service", "subscribe", "ks.enroll.*.request"},
	{"result-consumer", "publish", "$JS.API.CONSUMER.MSG.NEXT.KS_RES.<own-consumer>"},
	{"result-consumer", "publish", "$JS.ACK.KS_RES.<own-consumer>.>"},
	{"result-consumer", "subscribe", "<own-inbox>.>"},
	{"presence-consumer", "subscribe", "ks.out.*.presence"},
	{"monitoring-role", "subscribe", "ks.out.*.event"},
	{"monitoring-role", "subscribe", "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.KS_CMD.<consumer>"},
	{"monitoring-role", "subscribe", "$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.KS_CMD.<consumer>"},
	{"monitoring-role", "subscribe", "$JS.EVENT.ADVISORY.API.LIMIT_REACHED"},
	{"agent", "publish", "ks.out.<own-id>.result"},
	{"agent", "publish", "ks.out.<own-id>.event"},
	{"agent", "publish", "ks.out.<own-id>.presence"},
	{"agent", "publish", "$JS.API.CONSUMER.MSG.NEXT.KS_CMD.<own-consumer>"},
	{"agent", "publish", "$JS.ACK.KS_CMD.<own-consumer>.>"},
	{"agent", "subscribe", "ks.job.<own-id>.>"},
	{"agent", "subscribe", "<own-inbox>.>"},
	{"bootstrap", "publish", "ks.enroll.<own-token>.request"},
	{"bootstrap", "subscribe", "ks.enroll.<own-token>.reply"},
}

var _ = map[string]func(*testing.T){
	"POS-1": TestPOS1CommandPublisherPublishesCommand, "POS-2": TestPOS2CommandPublisherPublishesCancellation,
	"POS-3": TestPOS3AgentSubscribesOwnCommand, "POS-4": TestPOS4AgentSubscribesOwnCancellation,
	"POS-5": TestPOS5AgentPublishesOwnResult, "POS-6": TestPOS6AgentPublishesOwnPresence,
	"POS-7": TestPOS7AgentPullsOwnCommandConsumer, "POS-8": TestPOS8AgentAcknowledgesOwnCommand,
	"POS-9": TestPOS9PresenceConsumerSubscribesFleetPresence, "POS-10": TestPOS10ResultConsumerPullsOwnConsumer,
	"POS-11": TestPOS11BootstrapPublishesOwnRequest, "POS-12": TestPOS12BootstrapSubscribesOwnReply,
	"POS-13": TestPOS13EnrollmentServiceSubscribesRequests, "POS-14": TestPOS14MonitoringSubscribesMaxDeliveriesAdvisory,
	"POS-15": TestPOS15CommandPublisherSubscribesOwnInbox, "POS-16": TestPOS16EnrollmentServicePublishesReplies,
	"POS-17": TestPOS17AgentPublishesOwnEvent, "POS-18": TestPOS18AgentSubscribesOwnInbox,
	"POS-19": TestPOS19MonitoringSubscribesFleetEvents, "POS-20": TestPOS20ResultConsumerAcknowledgesResult,
	"POS-21": TestPOS21ResultConsumerSubscribesOwnInbox, "POS-22": TestPOS22MonitoringSubscribesTerminatedAdvisory,
	"POS-23": TestPOS23MonitoringSubscribesLimitAdvisory,
	"NEG-1":  TestNEG1AgentCannotPublishCommand, "NEG-2": TestNEG2AgentCannotPublishCancellation,
	"NEG-3": TestNEG3AgentCannotSubscribeOtherCommand, "NEG-4": TestNEG4AgentCannotSubscribeOtherJobPrefix,
	"NEG-5": TestNEG5AgentCannotPublishOtherResult, "NEG-6": TestNEG6AgentCannotSubscribeOtherResult,
	"NEG-7": TestNEG7RevokedBootstrapCannotConnect, "NEG-8": TestNEG8PermanentAgentCannotPublishEnrollment,
	"NEG-9": TestNEG9PermanentAgentCannotSubscribeEnrollment, "NEG-10": TestNEG10AgentCannotSubscribeSystemSubjects,
	"NEG-11": TestNEG11AgentCannotDeleteCommandStream, "NEG-12": TestNEG12AgentCannotPullOtherConsumer,
	"NEG-13": TestNEG13AgentCannotPublishBareAcknowledgement, "NEG-14": TestNEG14CommandPublisherCannotSubscribeResults,
	"NEG-15": TestNEG15PresenceConsumerCannotPublishCommands,
	"GEN-1":  TestGEN1TwoAccountsAndNoSystemPrincipals, "GEN-2": TestGEN2EveryPrincipalHasDistinctIdentity,
	"GEN-3": TestGEN3GeneratedPermissionsContainNoExtraGrant, "GEN-4": TestGEN4GeneratedPermissionsOmitNoGrant,
	"GEN-5": TestGEN5AccountLimitsAreFinite, "GEN-6": TestGEN6IdentifiersAreValidated,
	"GEN-7": TestGEN7OperatorSeedIsOutsideDeploymentProcesses,
}
