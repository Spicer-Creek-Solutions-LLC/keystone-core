package events

// The 22 v1.0 event type constants per PROJECT-DETAILS §4.9. These
// are the documented spellings — [IsCanonical] reports whether a
// given [EventType] matches one of them. Operators and v1.4 plugins
// (Epic 14) MAY emit other subtypes within a known [Category]
// without modifying this file; the canonical set is the
// recommendation, not the enforced limit.
//
// Three `state.apply.*` constants intentionally have multi-segment
// subtypes — they parse as category `state` + subtype `apply.start`
// (etc.) per [ParseEventType]'s first-dot split.
const (
	// agent x5 — agent lifecycle and health
	EventTypeAgentConnect         EventType = "agent.connect"
	EventTypeAgentDisconnect      EventType = "agent.disconnect"
	EventTypeAgentHeartbeat       EventType = "agent.heartbeat"
	EventTypeAgentHeartbeatFailed EventType = "agent.heartbeat_failed"
	EventTypeAgentError           EventType = "agent.error"

	// job x4 — remote execution job lifecycle
	EventTypeJobStart    EventType = "job.start"
	EventTypeJobComplete EventType = "job.complete"
	EventTypeJobFail     EventType = "job.fail"
	EventTypeJobOutput   EventType = "job.output"

	// state x5 — declared-state apply operations and drift
	EventTypeStateApplyStart EventType = "state.apply.start"
	EventTypeStateApplyDone  EventType = "state.apply.done"
	EventTypeStateApplyFail  EventType = "state.apply.fail"
	EventTypeStateChange     EventType = "state.change"
	EventTypeStateDrift      EventType = "state.drift"

	// system x3 — control-plane process lifecycle
	EventTypeSystemStartup  EventType = "system.startup"
	EventTypeSystemShutdown EventType = "system.shutdown"
	EventTypeSystemError    EventType = "system.error"

	// user x3 — user-initiated actions captured for audit
	EventTypeUserLogin   EventType = "user.login"
	EventTypeUserCommand EventType = "user.command"
	EventTypeUserError   EventType = "user.error"

	// policy x2 — policy evaluation outcomes (audit-mode only in v1.0)
	EventTypePolicyPass      EventType = "policy.pass"
	EventTypePolicyViolation EventType = "policy.violation"
)

// canonicalEventTypeSet is the membership oracle for [IsCanonical].
// Built from the 22 constants at init time; lookup is O(1).
var canonicalEventTypeSet = map[EventType]struct{}{
	EventTypeAgentConnect:         {},
	EventTypeAgentDisconnect:      {},
	EventTypeAgentHeartbeat:       {},
	EventTypeAgentHeartbeatFailed: {},
	EventTypeAgentError:           {},
	EventTypeJobStart:             {},
	EventTypeJobComplete:          {},
	EventTypeJobFail:              {},
	EventTypeJobOutput:            {},
	EventTypeStateApplyStart:      {},
	EventTypeStateApplyDone:       {},
	EventTypeStateApplyFail:       {},
	EventTypeStateChange:          {},
	EventTypeStateDrift:           {},
	EventTypeSystemStartup:        {},
	EventTypeSystemShutdown:       {},
	EventTypeSystemError:          {},
	EventTypeUserLogin:            {},
	EventTypeUserCommand:          {},
	EventTypeUserError:            {},
	EventTypePolicyPass:           {},
	EventTypePolicyViolation:      {},
}

// IsCanonical reports whether the receiver is one of the 22 v1.0
// documented event types from §4.9. Operators MAY use other
// subtypes within a known category (the parser accepts them); this
// helper exists for documentation, CLI completion, and a future
// `make lint` rule that warns when project code emits a non-canonical
// type without an explicit opt-out.
func IsCanonical(t EventType) bool {
	_, ok := canonicalEventTypeSet[t]
	return ok
}

// CanonicalEventTypes returns the 22 v1.0 constants in the
// documentation order from §4.9. Consumed by
// `EventService.GetEventTypes` (task 9) and by the CLI's `list`
// command for completion. Returned slice is a fresh copy; callers
// may mutate without affecting subsequent calls.
func CanonicalEventTypes() []EventType {
	return []EventType{
		EventTypeAgentConnect,
		EventTypeAgentDisconnect,
		EventTypeAgentHeartbeat,
		EventTypeAgentHeartbeatFailed,
		EventTypeAgentError,
		EventTypeJobStart,
		EventTypeJobComplete,
		EventTypeJobFail,
		EventTypeJobOutput,
		EventTypeStateApplyStart,
		EventTypeStateApplyDone,
		EventTypeStateApplyFail,
		EventTypeStateChange,
		EventTypeStateDrift,
		EventTypeSystemStartup,
		EventTypeSystemShutdown,
		EventTypeSystemError,
		EventTypeUserLogin,
		EventTypeUserCommand,
		EventTypeUserError,
		EventTypePolicyPass,
		EventTypePolicyViolation,
	}
}
