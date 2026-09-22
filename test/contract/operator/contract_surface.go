//go:build contract

package operatorcontract

import "testing"

type acceptanceCase struct {
	TestName    string
	Requirement string
}

// This file is C04-A's immutable acceptance surface. C04-I may replace the
// pending test bodies and correct the fixture, but it may not change these
// names, requirements, or the operator interface frozen below.
//
// Every case runs against the production keystone-server binary, started with
// no arguments inside a container whose principals are real accounts. Per
// ADR-0010 § 1 that is PULL-REQUEST FEEDBACK and not acceptance: ADR-0009's
// release gate is C13's VM harness.
var acceptanceContractSurface = map[string]acceptanceCase{
	"SOCK-1": {"TestSOCK1SocketOwnershipAndModes", "the server must create /run/keystone as root:<admin group> 0750 and the socket as root:<admin group> 0660"},
	"SOCK-2": {"TestSOCK2RefusesToStartWithoutAdminGroup", "a server whose configuration names no admin group must exit non-zero without creating the socket, and the same configuration with the group must start"},
	"SOCK-3": {"TestSOCK3OutsiderCannotStatTheSocket", "a principal outside the admin group must be refused a stat of the socket while a member is not"},
	"SOCK-4": {"TestSOCK4KernelRefusesOutsiderConnect", "a principal outside the admin group must have connect() refused by the kernel with EACCES while a member's connection is answered"},
	"ORD-1":  {"TestORD1NoRequestByteReadBeforeDecision", "while the server is held after its authorization decision, the count of request bytes it has not consumed must not fall, for an authorized peer and for a denied one"},
	"ORD-2":  {"TestORD2DenialRecordDurableBeforeResponse", "a server killed with SIGKILL the moment its denial response arrives must already have written that denial's record, on every repetition"},
	"ORD-3":  {"TestORD3NoDenialAnsweredWithoutDurableRecord", "when the denial record cannot be written the server must close the connection without writing a response, and must answer and record the next denial once it can"},
	"DENY-1": {"TestDENY1StaleMemberDeniedInBand", "a session that still carries the admin group after its account was removed from it must connect and then be denied in-band"},
	"DENY-2": {"TestDENY2RootOutsideGroupDeniedInBand", "root, which is not a member of the admin group, must connect and then be denied in-band"},
	"DENY-3": {"TestDENY3DenialSaysOnlyAuthorizationDenied", "every in-band denial must be exactly one frame whose body is the object {\"error\":\"authorization_denied\"}, byte-identical for a stale member and for root, followed by close"},
	"DENY-4": {"TestDENY4InBandDenialIsRecorded", "each in-band denial must add exactly one denial record, and an authorized connection must add none"},
	"DENY-5": {"TestDENY5KernelRefusalIsNotRecorded", "a connect() the kernel refuses must add no audit record of any kind"},
	"DENY-6": {"TestDENY6RecordActorIsKernelDerived", "a denial record must carry the peer's SO_PEERCRED uid and its username as a snapshot, no job and no target, and the audit table must have no pid column"},
	"DENY-7": {"TestDENY7RequestCannotSetActor", "request fields naming another actor, uid or user must not change the actor a denial record names"},
	"REC-1":  {"TestREC1NonSocketIsNotRemoved", "a non-socket at the socket path must be left in place and the server must exit non-zero"},
	"REC-2":  {"TestREC2ForeignOwnedSocketIsNotRemoved", "a socket at the path owned by a user other than root must be left in place and the server must exit non-zero"},
	"REC-3":  {"TestREC3LiveSocketIsNotRemoved", "a socket at the path that accepts connections must be left in place and still accepting, and the second server must exit non-zero"},
	"REC-4":  {"TestREC4StaleSocketIsRecovered", "a root-owned socket that refuses with ECONNREFUSED in a root-only-writable directory must be replaced and the server must serve"},
	"REC-5":  {"TestREC5WritableDirectoryRefusesStart", "a stale socket in a directory writable by a non-root principal must be left in place and the server must exit non-zero"},
	"LIM-1":  {"TestLIM1UnauthorizedConnectionLimit", "connections awaiting authorization beyond the configured limit must be closed without a response while those within it are served"},
	"LIM-2":  {"TestLIM2AuthorizationTimeoutFreesSlot", "a connection whose authorization outlives the configured timeout must be closed without a response and must free its slot for the next"},
	"LIM-3":  {"TestLIM3AuthorizedConnectionLimit", "an authorized connection beyond the configured limit must receive {\"error\":\"connection_limit\"} and be closed, and a slot must be usable again once released"},
	"LIM-4":  {"TestLIM4OversizedFrameRefusedBeforeAllocation", "a length prefix above the configured maximum must receive {\"error\":\"frame_too_large\"} without the server's peak virtual size growing by the prefix, and the server must keep serving"},
	"ISO-1":  {"TestISO1NoNetworkSocket", "the server must hold no IP socket of any kind; at C04 no broker is configured, so no network destination exists"},
	"FLT-1":  {"TestFLT1EnabledFaultPointIsAudited", "a server started with a fault point enabled must have an audit record naming it before it accepts a connection, and one started with none must have no such record"},
}

// ---------------------------------------------------------------------------
// The operator interface C04-I implements. Frozen with the cases, because the
// cases cannot be written without it; C04 decides it (C04.md "What C04
// decides") and C04-A is where it is decided.
// ---------------------------------------------------------------------------

// The socket. ADR-0009 § 1 fixes both paths; neither is configurable.
const (
	socketDir  = "/run/keystone"
	socketPath = "/run/keystone/operator.sock"
)

// The server is started with NO ARGUMENTS and reads its configuration from the
// file internal/config's KEYSTONE_SERVER_CONFIG names. It serves until
// signalled. A server that cannot serve exits non-zero.
const serverConfigEnv = "KEYSTONE_SERVER_CONFIG"

// The configuration file is TOML. These are the tables and keys the contract
// writes; C04-I may accept more, and must accept these with these meanings.
// Every duration is an integer number of milliseconds.
const (
	tableOperator = "operator"
	// keyAdminGroup names the group that owns the directory and the socket.
	// Absent is a configuration error (ADR-0009 § 1); there is no default.
	keyAdminGroup = "admin_group"
	// The four ADR-0009 § 9 limits C04 can make observable. The fifth -- in
	// flight requests per connection -- is registered to C05. Absent keys take
	// C04-I's defaults, which C04-I states and C15 revisits.
	keyMaxUnauthorized = "max_unauthorized_connections"
	keyAuthTimeoutMS   = "authorization_timeout_ms"
	keyMaxAuthorized   = "max_authorized_connections"
	keyMaxFrameBytes   = "max_frame_bytes"

	tableStore = "store"
	keyStore   = "path"

	// Fault points (ADR-0010 § 5): configuration in the shipped binary, inert
	// when absent or zero, and recorded in the audit when enabled (§ 12).
	tableFaults = "faults"
	// keyHoldBeforeDecision holds a connection after accept() and before the
	// authorization decision. It elapses INSIDE the window the authorization
	// timeout bounds and counts against the unauthorized-connection limit.
	keyHoldBeforeDecision = "operator_hold_before_decision_ms"
	// keyHoldAfterDecision holds a connection after the authorization decision
	// and before anything else is done with it: before any read, any record,
	// and any response.
	keyHoldAfterDecision = "operator_hold_after_decision_ms"
)

// The framing: every frame, in both directions, is a uint32 big-endian length
// followed by exactly that many bytes of a JSON object. A request carries its
// operation in the "op" member. A server error is an object whose only member
// is "error".
//
// No byte of a request is read before the authorization decision (ADR-0009
// §§ 3, 10). No request member may name the actor (§ 7).
const (
	requestOp = "op"
	errorKey  = "error"

	// Written to a peer the in-band check refused, then the connection is
	// closed. It says nothing about which check refused or why (§ 5).
	errAuthorizationDenied = "authorization_denied"
	// Written to an authorized peer whose operation does not exist. The
	// connection stays open. At C04 no operation exists, so this is the one
	// thing an authorized connection reaches and a refused one does not.
	errUnknownOperation = "unknown_operation"
	// Written when a length prefix exceeds the configured maximum, before any
	// allocation for the body; then the connection is closed.
	errFrameTooLarge = "frame_too_large"
	// Written to an authorized peer beyond the authorized-connection limit;
	// then the connection is closed.
	errConnectionLimit = "connection_limit"
)

// The audit record. C04-I extends ADR-0008 § 6's table with one migration, and
// the contract reads the whole of it with auditQuery.
const (
	auditQuery = "SELECT * FROM audit"
	// actionDenied is the action of an in-band authorization denial.
	actionDenied = "operator.authorization.denied"
	// actionFaultPrefix + a key name is the action of an enabled fault point,
	// e.g. "fault.enabled:operator_hold_after_decision_ms".
	actionFaultPrefix = "fault.enabled:"

	columnAction = "action"
	// columnActorUID is the authoritative actor: SO_PEERCRED's uid (§ 7).
	columnActorUID = "actor_uid"
	// columnActorUsernameSnapshot is the name the uid resolved to at the time
	// of the request, and its column name is what marks it as a snapshot.
	columnActorUsernameSnapshot = "actor_username_snapshot"
	// A denial concerns no job and no agent: both are NULL.
	columnJobID  = "job_id"
	columnTarget = "target"
	// No column of the audit table may contain this, case-insensitively (§ 7).
	forbiddenColumnFragment = "pid"
)

var _ = map[string]func(*testing.T){
	"SOCK-1": TestSOCK1SocketOwnershipAndModes, "SOCK-2": TestSOCK2RefusesToStartWithoutAdminGroup,
	"SOCK-3": TestSOCK3OutsiderCannotStatTheSocket, "SOCK-4": TestSOCK4KernelRefusesOutsiderConnect,
	"ORD-1": TestORD1NoRequestByteReadBeforeDecision, "ORD-2": TestORD2DenialRecordDurableBeforeResponse,
	"ORD-3":  TestORD3NoDenialAnsweredWithoutDurableRecord,
	"DENY-1": TestDENY1StaleMemberDeniedInBand, "DENY-2": TestDENY2RootOutsideGroupDeniedInBand,
	"DENY-3": TestDENY3DenialSaysOnlyAuthorizationDenied, "DENY-4": TestDENY4InBandDenialIsRecorded,
	"DENY-5": TestDENY5KernelRefusalIsNotRecorded, "DENY-6": TestDENY6RecordActorIsKernelDerived,
	"DENY-7": TestDENY7RequestCannotSetActor,
	"REC-1":  TestREC1NonSocketIsNotRemoved, "REC-2": TestREC2ForeignOwnedSocketIsNotRemoved,
	"REC-3": TestREC3LiveSocketIsNotRemoved, "REC-4": TestREC4StaleSocketIsRecovered,
	"REC-5": TestREC5WritableDirectoryRefusesStart,
	"LIM-1": TestLIM1UnauthorizedConnectionLimit, "LIM-2": TestLIM2AuthorizationTimeoutFreesSlot,
	"LIM-3": TestLIM3AuthorizedConnectionLimit, "LIM-4": TestLIM4OversizedFrameRefusedBeforeAllocation,
	"ISO-1": TestISO1NoNetworkSocket,
	"FLT-1": TestFLT1EnabledFaultPointIsAudited,
}
