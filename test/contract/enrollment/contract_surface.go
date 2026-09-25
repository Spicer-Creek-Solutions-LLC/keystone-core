//go:build contract

package enrollmentcontract

import "testing"

type acceptanceCase struct {
	TestName    string
	Requirement string
}

// This is C05-A's immutable acceptance surface. C05-I may replace pending test
// bodies and correct the fixture, but it may not change these names,
// requirements, or the interface below without an approved contract amendment.
var acceptanceContractSurface = map[string]acceptanceCase{
	"ISS-1":    {"TestISS1TokenIssuedOnceAndAudited", "enroll create must durably record issuance and print exactly one bundle to stdout and nowhere else"},
	"CLI-1":    {"TestCLI1RefusalPathsAreIndistinguishable", "kernel and in-band operator authorization refusals must be indistinguishable to the CLI caller"},
	"CLI-2":    {"TestCLI2AuthorizationRefusalsExitTen", "both operator authorization refusal paths must exit 10"},
	"CLI-3":    {"TestCLI3MissingSocketExitsOne", "a missing operator socket must exit 1"},
	"LIM-5":    {"TestLIM5BoundsInflightRequests", "one operator connection must process exactly one request at a time: it must not read a pipelined second frame until the first reply is written, and replies must preserve request order"},
	"FLT-2":    {"TestFLT2OperatorResponseHoldIsAudited", "before serving a connection, a server with operator_hold_before_response_ms enabled must record fault.enabled:operator_hold_before_response_ms, and a server without it must record no such action"},
	"JWT-1":    {"TestJWT1BootstrapCredentialIsExactAndBounded", "the bootstrap JWT must have only its token request and reply grants, expire with the token, and match C03's frozen matrix"},
	"BND-1":    {"TestBND1BundleCarriesBothServiceTrustHalves", "the bundle must carry the public halves of the fixture-provisioned service signing private key and result-encryption public key"},
	"SKEY-1":   {"TestSKEY1ServiceSigningPrivateKeyIsOwnerOnly", "the server must refuse to start when the configured service signing private-key file has any group or other permission bit; owner-only modes including 0400 and 0600 are permitted"},
	"ARG-1":    {"TestARG1BundleNeverAppearsInArgv", "the token bundle and its bootstrap seed must never appear in a process argv"},
	"FILE-1":   {"TestFILE1RefusesReadableBundle", "a bundle file readable by group or world must be refused as local error 1"},
	"STDIN-1":  {"TestSTDIN1EnrollsFromStandardInput", "--token-file - must consume the bundle from standard input and enroll"},
	"FILE-2":   {"TestFILE2SuccessfulEnrollmentRemovesBundle", "the bundle file must not survive enrollment exit 0"},
	"DENY-1":   {"TestDENY1TokenFailuresAreIndistinguishableAndAudited", "expired, spent, and invalid tokens must be denied with the same caller-visible result 10 and each observed denial must be audited with its internal reason"},
	"AUTH-1":   {"TestAUTH1RequestIdentityAndSignatureMustMatch", "a request from another token's bootstrap identity or without a valid signature by its presented signing key must be denied"},
	"IDEM-1":   {"TestIDEM1DifferentKeysAreNotARetry", "re-presenting an issued token with different public halves must be denied"},
	"IDEM-2":   {"TestIDEM2SameKeysReturnSameIdentity", "re-presenting the same public halves while issued must return the same JWT and one token must yield exactly one identity"},
	"ID-1":     {"TestID1AgentIdentifierIsServerAssigned", "the permanent identifier must be a server-generated 128-bit lowercase hexadecimal identifier, never the operator label or an enrolling-party value"},
	"KEY-1":    {"TestKEY1AgentPrivateKeysNeverLeaveAgent", "no agent-generated private key may occur in server storage, server logs, or broker-visible bytes"},
	"KEY-2":    {"TestKEY2PersistedKeysSurvivePreRequestCrash", "the mode-0600 agent-key artifact must atomically persist the NKey seed and AST-4 and AST-5 private keys before S1, and restart after that write must present the same public halves"},
	"CRED-1":   {"TestCRED1IdentityArtifactIsAtomicAndPrivate", "the permanent credential and service trust halves must become visible together in one mode-0600 identity artifact, never partially"},
	"LEDGER-1": {"TestLEDGER1LedgerExistsBeforeActivation", "the durable agent ledger must exist before the server can reach S4"},
	"ORD-1":    {"TestORD1PermanentConnectionPrecedesSpentWrite", "the broker must observe the permanent identity connect and publish its signed presence proof before the server atomically records active and spent"},
	"SPENT-1":  {"TestSPENT1SpentTokenOnlyAnswersConfirmation", "after S4 every request on the spent token must be refused and audited except its idempotent S6 confirmation"},
	"EXP-1":    {"TestEXP1BrokerRefusesBootstrapAfterExpiry", "a bootstrap credential proven usable before expiry must be refused by the broker after expiry whether or not enrollment reached S4"},
	"OBS-1":    {"TestOBS1EndOfBootstrapAccessIsObserved", "no component may claim bootstrap access ended from a claims read-back; the contract must observe server refusal and broker expiry where each takes effect"},
	"S6-1":     {"TestS61SuccessRequiresConfirmation", "the agent must not exit 0 without an authenticated S6 active-and-spent confirmation, including after expiry between S4 and S6"},
	"CRASH-1":  {"TestCRASH1RecoverS0ToS1", "after a confirmed kill at S0-S1, restart must converge on exactly one active identity and one spent token"},
	"CRASH-2":  {"TestCRASH2RecoverS1ToS2", "after a confirmed kill at S1-S2, restart must converge using the recorded JWT, without minting another identity"},
	"CRASH-3":  {"TestCRASH3RecoverS2ToS3", "after a confirmed kill at S2-S3, restart must use the complete identity artifact and converge without re-enrollment"},
	"CRASH-4":  {"TestCRASH4RecoverS3ToS4", "after a confirmed kill at S3-S4, restart must repeat the signed permanent proof idempotently and converge"},
	"CRASH-5":  {"TestCRASH5RecoverAgentS4ToS6", "after a confirmed agent kill at S4-S6, restart must request confirmation and converge without changing the identity"},
	"CRASH-6":  {"TestCRASH6RecoverServerS4ToS6", "after a confirmed server kill after the active-and-spent commit and before S6 reply, restart must confirm the same identity and spent token"},
	"RECON-1":  {"TestRECON1EnrolledAgentReconnectsWithoutEnrollment", "an enrolled agent must reconnect with its permanent identity and never re-enroll"},
	"RECON-2":  {"TestRECON2MissingIdentityFailsLocally", "an agent without a readable identity artifact must fail with exit 1 and must not attempt enrollment"},
	"NON-1":    {"TestNON1SecondEnrollmentDoesNotChangeFirst", "enrolling a second agent must not change the first agent's identity or active state"},
	"TLS-1":    {"TestTLS1EveryProductionConnectionVerifiesBroker", "every server and agent NATS connection must use TLS-first TLS 1.3 and verify the configured broker name against the configured CA"},
	"ROLE-1":   {"TestROLE1WrongServerCredentialRoleIsRefused", "the server must refuse startup when a configured credential does not match its enrollment-service or presence-consumer slot and must read no unconfigured credential"},
	"ISO-1":    {"TestISO1JourneyUsesOnlyBrokerBetweenServerAndAgents", "the enrollment journey must work across isolated networks with no server-to-agent or agent-to-agent route and no agent private key on the server"},
}

// The C05-owned public interface. Strings are frozen wire/configuration tokens,
// not suggestions to the implementation.
const (
	operatorOperation      = "enroll.create"
	bundleVersion          = 1
	identityVersion        = 1
	defaultTokenTTLSeconds = 15 * 60
	minimumTokenTTLSeconds = 60
	maximumTokenTTLSeconds = 59 * 60

	requestKindCredential = "credential"
	requestKindConfirm    = "confirmation"
	replyStateActive      = "active"
	replyTokenSpent       = "spent"

	serverTableNATS             = "nats"
	serverKeyBrokerURL          = "url"
	serverKeyBrokerCA           = "ca_file"
	serverKeyBrokerName         = "server_name"
	serverKeyEnrollmentCreds    = "enrollment_credentials"
	serverKeyPresenceCreds      = "presence_credentials"
	serverKeyAccountSigningSeed = "account_signing_seed"
	serverTableService          = "service"
	serverKeySigningPrivateKey  = "signing_private_key_file"
	serverKeyResultPublicKey    = "result_encryption_public_key_file"
	serverTableFaults           = "faults"
	agentTableNATS              = "nats"
	agentKeyBrokerURL           = "url"
	agentKeyBrokerCA            = "ca_file"
	agentKeyBrokerName          = "server_name"
	agentTableAgent             = "agent"
	agentKeyIdentityPath        = "identity_path"
	agentKeyPrivateKeysPath     = "private_keys_path"
	agentKeyLedgerPath          = "ledger_path"
	agentTableFaults            = "faults"
	agentKeyFaultRecordPath     = "record_path"
	agentKeyArtifactVersion     = 1
	agentFaultRecordVersion     = 1
	privateArtifactMode         = 0o600

	faultOperatorBeforeResponse  = "operator_hold_before_response_ms"
	faultAgentBeforeRequest      = "enrollment_agent_hold_before_request_ms"
	faultAgentAfterReply         = "enrollment_agent_hold_after_credential_reply_ms"
	faultAgentAfterIdentityWrite = "enrollment_agent_hold_after_identity_write_ms"
	faultAgentAfterProof         = "enrollment_agent_hold_after_permanent_proof_ms"
	faultAgentBeforeConfirmation = "enrollment_agent_hold_before_confirmation_ms"
	faultServerAfterActivation   = "enrollment_server_hold_after_activation_commit_ms"
)

const serviceSigningKeyForbiddenMode = 0o077

type operatorEnrollRequest struct {
	Operation  string `json:"op"`
	AgentName  string `json:"agent_name"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

type operatorEnrollResponse struct {
	Bundle tokenBundleV1 `json:"bundle"`
}

type tokenBundleV1 struct {
	Version                   int    `json:"version"`
	TokenID                   string `json:"token_id"`
	AgentID                   string `json:"agent_id"`
	BootstrapCredentials      string `json:"bootstrap_credentials"`
	ServiceSigningPublicKey   string `json:"service_signing_public_key"`
	ResultEncryptionPublicKey string `json:"result_encryption_public_key"`
}

type enrollmentRequestV1 struct {
	NATSPublicKey       string `json:"nats_public_key"`
	SigningPublicKey    string `json:"signing_public_key"`
	EncryptionPublicKey string `json:"encryption_public_key"`
	Kind                string `json:"kind"`
}

type enrollmentReplyV1 struct {
	Kind         string `json:"kind"`
	AgentID      string `json:"agent_id"`
	PermanentJWT string `json:"permanent_jwt,omitempty"`
	Identity     string `json:"identity_state,omitempty"`
	Token        string `json:"token_state,omitempty"`
}

type identityArtifactV1 struct {
	Version                   int    `json:"version"`
	AgentID                   string `json:"agent_id"`
	PermanentCredentials      string `json:"permanent_credentials"`
	AgentKeySHA256            string `json:"agent_key_sha256"`
	ServiceSigningPublicKey   string `json:"service_signing_public_key"`
	ResultEncryptionPublicKey string `json:"result_encryption_public_key"`
}

// agentKeyArtifactV1 is written by atomic rename, chmodded to
// privateArtifactMode and fsynced before the first S1 request. SigningPrivateKey
// is the base64 encoding of protocol.MarshalSigningKey; DecryptionPrivateKey is
// the base64 encoding of protocol.MarshalDecryptionKey. Identity artifacts bind
// to the SHA-256 digest of the exact bytes stored here, so startup cannot pair
// permanent credentials with a different private-key set.
type agentKeyArtifactV1 struct {
	Version              int    `json:"version"`
	AgentID              string `json:"agent_id"`
	NATSSeed             string `json:"nats_seed"`
	SigningPrivateKey    string `json:"signing_private_key"`
	DecryptionPrivateKey string `json:"decryption_private_key"`
}

// agentFaultRecordV1 is one JSON line appended and fsynced at record_path
// before an enabled agent hold begins. The harness waits for this durable line
// before SIGKILL. It proves only that the configured boundary was reached; the
// independent observations in crash-harness.json prove ordering and recovery.
type agentFaultRecordV1 struct {
	Version int    `json:"version"`
	Event   string `json:"event"`
	Fault   string `json:"fault"`
	Process string `json:"process"`
}

const (
	faultEventEnabled = "enabled"
	faultEventReached = "reached"
)

var _ = map[string]func(*testing.T){
	"ISS-1": TestISS1TokenIssuedOnceAndAudited, "CLI-1": TestCLI1RefusalPathsAreIndistinguishable,
	"CLI-2": TestCLI2AuthorizationRefusalsExitTen, "CLI-3": TestCLI3MissingSocketExitsOne,
	"LIM-5": TestLIM5BoundsInflightRequests, "FLT-2": TestFLT2OperatorResponseHoldIsAudited,
	"JWT-1": TestJWT1BootstrapCredentialIsExactAndBounded, "BND-1": TestBND1BundleCarriesBothServiceTrustHalves,
	"SKEY-1": TestSKEY1ServiceSigningPrivateKeyIsOwnerOnly, "ARG-1": TestARG1BundleNeverAppearsInArgv,
	"FILE-1": TestFILE1RefusesReadableBundle, "STDIN-1": TestSTDIN1EnrollsFromStandardInput,
	"FILE-2": TestFILE2SuccessfulEnrollmentRemovesBundle, "DENY-1": TestDENY1TokenFailuresAreIndistinguishableAndAudited,
	"AUTH-1": TestAUTH1RequestIdentityAndSignatureMustMatch, "IDEM-1": TestIDEM1DifferentKeysAreNotARetry,
	"IDEM-2": TestIDEM2SameKeysReturnSameIdentity, "ID-1": TestID1AgentIdentifierIsServerAssigned,
	"KEY-1": TestKEY1AgentPrivateKeysNeverLeaveAgent, "KEY-2": TestKEY2PersistedKeysSurvivePreRequestCrash,
	"CRED-1":   TestCRED1IdentityArtifactIsAtomicAndPrivate,
	"LEDGER-1": TestLEDGER1LedgerExistsBeforeActivation, "ORD-1": TestORD1PermanentConnectionPrecedesSpentWrite,
	"SPENT-1": TestSPENT1SpentTokenOnlyAnswersConfirmation, "EXP-1": TestEXP1BrokerRefusesBootstrapAfterExpiry,
	"OBS-1": TestOBS1EndOfBootstrapAccessIsObserved, "S6-1": TestS61SuccessRequiresConfirmation,
	"CRASH-1": TestCRASH1RecoverS0ToS1, "CRASH-2": TestCRASH2RecoverS1ToS2,
	"CRASH-3": TestCRASH3RecoverS2ToS3, "CRASH-4": TestCRASH4RecoverS3ToS4,
	"CRASH-5": TestCRASH5RecoverAgentS4ToS6, "CRASH-6": TestCRASH6RecoverServerS4ToS6,
	"RECON-1": TestRECON1EnrolledAgentReconnectsWithoutEnrollment, "RECON-2": TestRECON2MissingIdentityFailsLocally,
	"NON-1": TestNON1SecondEnrollmentDoesNotChangeFirst, "TLS-1": TestTLS1EveryProductionConnectionVerifiesBroker,
	"ROLE-1": TestROLE1WrongServerCredentialRoleIsRefused, "ISO-1": TestISO1JourneyUsesOnlyBrokerBetweenServerAndAgents,
}
