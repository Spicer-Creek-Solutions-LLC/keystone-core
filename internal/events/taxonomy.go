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

// canonicalByCategory groups the 22 canonical constants by their
// owning [Category] in §4.9 documentation order. Built once at
// package init via the loop in [canonicalByCategoryInit]; queried
// by [EventTypesForCategory] / [CountForCategory] /
// [AllCategoriesWithCounts].
var canonicalByCategory = canonicalByCategoryInit()

// canonicalByCategoryInit walks [CanonicalEventTypes] in order,
// grouping by [EventType.Category]. The package-init pattern keeps
// the data the single source of truth — adding a constant to the
// list automatically lands it in the right category bucket.
func canonicalByCategoryInit() map[Category][]EventType {
	out := make(map[Category][]EventType, len(knownCategorySet))
	// Initialise every known category to an empty slice so
	// EventTypesForCategory returns a non-nil empty list (rather
	// than nil) when the category is known but has zero canonical
	// types in v1.0 — currently impossible but defensive against
	// future taxonomy churn.
	for c := range knownCategorySet {
		out[c] = nil
	}
	for _, typ := range CanonicalEventTypes() {
		c := typ.Category()
		out[c] = append(out[c], typ)
	}
	return out
}

// EventTypesForCategory returns the canonical event types belonging
// to c in §4.9 documentation order. Unknown / empty category
// returns nil — callers can treat `len(...)==0` as "no canonical
// types for this category."
//
// Returned slice is a fresh copy per call; callers may mutate
// without affecting subsequent calls or other consumers.
//
// Note on multi-segment subtypes: `state.apply.start` /
// `state.apply.done` / `state.apply.fail` all belong to
// [CategoryState] — their first-dot-split puts `state` in the
// category slot and `apply.<verb>` in the subtype slot. The
// returned list reflects that grouping.
func EventTypesForCategory(c Category) []EventType {
	src, ok := canonicalByCategory[c]
	if !ok || len(src) == 0 {
		return nil
	}
	out := make([]EventType, len(src))
	copy(out, src)
	return out
}

// CountForCategory returns the number of canonical event types in c.
// Sugar around `len(EventTypesForCategory(c))` for callers sizing
// a buffer without copying — the gRPC handler's GetEventStats uses
// it to size the per-category counts map up-front.
//
// Unknown category returns 0.
func CountForCategory(c Category) int {
	return len(canonicalByCategory[c])
}

// AllCategoriesWithCounts returns a fresh map of every known
// category → its canonical event count. Sums to 22 across all 6
// v1.0 categories. Useful for CLI/operator-facing taxonomy
// summary surfaces (the deferred `kscore-events types --by-category`
// flag).
func AllCategoriesWithCounts() map[Category]int {
	out := make(map[Category]int, len(canonicalByCategory))
	for c, types := range canonicalByCategory {
		out[c] = len(types)
	}
	return out
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
