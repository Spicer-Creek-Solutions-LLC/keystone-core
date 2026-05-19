package events

import (
	"fmt"
	"strings"
)

// Category is the closed enum of v1.0 event categories per
// PROJECT-DETAILS §4.9. The set is closed by design: downstream
// routing (the audit pipeline in epic 12, the post-v1.0 reactor engine,
// retention policy in task 8) switches exhaustively on category, so
// new categories require a deliberate code change rather than
// runtime extension. Subtypes within a category remain free-form so
// operators and post-v1.0 plugins (Epic 14) can introduce new signals
// without core changes.
type Category string

const (
	// CategoryAgent groups events about agent lifecycle and health:
	// connect, disconnect, heartbeat, heartbeat_failed, error.
	CategoryAgent Category = "agent"

	// CategoryJob groups events about remote-execution job lifecycle:
	// start, complete, fail, output.
	CategoryJob Category = "job"

	// CategoryState groups events about declared-state apply
	// operations and drift: apply.start, apply.done, apply.fail,
	// change, drift.
	CategoryState Category = "state"

	// CategorySystem groups events about the control-plane process
	// itself: startup, shutdown, error.
	CategorySystem Category = "system"

	// CategoryUser groups events about user-initiated actions
	// captured for audit: login, command, error.
	CategoryUser Category = "user"

	// CategoryPolicy groups events about policy evaluation outcomes:
	// pass, violation. Audit-mode only in v1.0.
	CategoryPolicy Category = "policy"

	// CategoryRunbook groups events about runbook workflow execution:
	// execute.start/done/fail, step.start/done/fail/skip
	// (Epic 15 task 9).
	CategoryRunbook Category = "runbook"
)

// knownCategorySet is the membership oracle for [Category.IsKnown]
// and [ParseCategory]. Order is irrelevant for lookup; the canonical
// emission order lives in [KnownCategories].
var knownCategorySet = map[Category]struct{}{
	CategoryAgent:   {},
	CategoryJob:     {},
	CategoryState:   {},
	CategorySystem:  {},
	CategoryUser:    {},
	CategoryPolicy:  {},
	CategoryRunbook: {},
}

// KnownCategories returns the seven v1.0 categories in the
// documentation order from §4.9 (agent, job, state, system, user,
// policy, runbook). The returned slice is a fresh copy; callers may
// mutate without affecting subsequent calls.
func KnownCategories() []Category {
	return []Category{
		CategoryAgent,
		CategoryJob,
		CategoryState,
		CategorySystem,
		CategoryUser,
		CategoryPolicy,
		CategoryRunbook,
	}
}

// IsKnown reports whether the receiver is one of the closed v1.0
// categories. Used by [ParseEventType] to reject unknown category
// prefixes and by [Event.Validate] as a structural check.
func (c Category) IsKnown() bool {
	_, ok := knownCategorySet[c]
	return ok
}

// String returns the underlying string form. Distinct method (rather
// than relying on the Category-to-string conversion at use sites) so
// log / format calls render the category symbolically.
func (c Category) String() string {
	return string(c)
}

// ParseCategory accepts the canonical lowercase names. Whitespace is
// trimmed and case is normalised. Errors wrap [ErrInvalidEvent] —
// the error message names the input so config-file typos are easy
// to spot.
func ParseCategory(s string) (Category, error) {
	normalised := Category(strings.ToLower(strings.TrimSpace(s)))
	if normalised == "" {
		return "", fmt.Errorf("%w: category is empty", ErrInvalidEvent)
	}
	if !normalised.IsKnown() {
		return "", fmt.Errorf("%w: unknown category %q (known: %v)", ErrInvalidEvent, s, KnownCategories())
	}
	return normalised, nil
}
