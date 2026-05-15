package events

import (
	"context"
	"fmt"
	"time"
)

// DefaultQueryLimit is the page size [EventStore.Query] applies when
// the caller leaves [EventQuery.Limit] at zero. Matches the §4.9
// CLI default (`kscore-events list ... --limit 50`) scaled to the
// gRPC handler's expectation that it can fan-out a reasonable page
// without operator-supplied tuning.
const DefaultQueryLimit = 100

// EventStore is the consumer-facing persistence interface for the v1.0
// event system. Task 3's [EventPublisher] writes through Store /
// StoreBatch; the gRPC handler (task 6) and CLI (task 7) read through
// Get / Query / Count; the retention enforcer (task 8) drives
// ApplyRetention.
//
// Implementations layer above an `internal/state.EventStore` (see
// [NewSQLEventStore]) and translate typed [Event] / [EventQuery] /
// [RetentionPolicy] values into the DB-shape representation. Backends
// MUST be safe for concurrent use; the SQL backend inherits its
// safety from `database/sql` connection pooling.
type EventStore interface {
	// Store validates the event and persists it. Returns
	// [ErrInvalidEvent] for any structural rejection (before any DB
	// call). PRIMARY-KEY collision (re-emitting an event with an
	// already-stored ID) wraps `state.ErrDuplicate` from the
	// underlying store — callers branch with [errors.Is].
	Store(ctx context.Context, e Event) error

	// StoreBatch persists every event atomically — all rows land or
	// none do. Every event is validated before any DB call.
	StoreBatch(ctx context.Context, events []Event) error

	// Get returns the event with the given ID. Returns
	// `state.ErrNotFound` when absent.
	Get(ctx context.Context, id string) (Event, error)

	// Query returns events matching the filter, paginated by
	// k-sortable [Event.ID]. [EventPage.NextCursor] is empty when no
	// more events match.
	Query(ctx context.Context, q EventQuery) (EventPage, error)

	// Count returns the total number of events matching the filter
	// (ignoring Cursor/Limit/Descending).
	Count(ctx context.Context, q EventQuery) (int, error)

	// Delete removes a single event by ID. Returns
	// `state.ErrNotFound` when absent.
	Delete(ctx context.Context, id string) error

	// ApplyRetention applies every policy in order and returns the
	// total number of rows removed. Empty slice is a no-op.
	ApplyRetention(ctx context.Context, policies []RetentionPolicy) (int, error)

	// Close releases resources held by the store implementation.
	// Idempotent — safe to call on a closed or never-started store.
	Close() error
}

// EventQuery is the typed filter the gRPC handler and CLI build before
// handing off to [EventStore.Query]. Zero-value fields are ignored so
// `Query(ctx, EventQuery{})` returns the most recent
// [DefaultQueryLimit] events.
//
// Type and Category are mutually exclusive:
//
//   - Type matches the literal `<category>.<subtype>` exactly.
//   - Category fans-in every event whose [EventType.Category] equals
//     the given value (translates to SQL `type LIKE 'agent.%'`).
//
// MinSeverity is the threshold from PROJECT-DETAILS §4.9's
// `severity >= 'warn'` idiom; the wrapper translates it to an
// IN-clause over the closed severity set ahead of the SQL call.
//
// Tags is ANDed exact-match on the stored JSON tag map. The
// underlying store walks tag predicates via JSON-extraction (no GIN
// index in v1.0 — see the gate-v1.0 ROADMAP entry if measurements
// show we need one).
//
// Cursor pagination uses [Event.ID] (UUIDv7 — k-sortable by stamping
// time). First page passes Cursor=""; subsequent pages pass the
// previous page's [EventPage.NextCursor].
type EventQuery struct {
	Type          EventType
	Category      Category
	Source        string
	MinSeverity   Severity
	Tags          map[string]string
	CorrelationID string
	Since         time.Time
	Until         time.Time
	Cursor        string
	Limit         int
	Descending    bool
}

// Validate enforces the structural invariants the store and the CEL
// filter compiler (task 5) both depend on:
//
//   - Type and Category are mutually exclusive.
//   - Limit is non-negative.
//   - When both Since and Until are non-zero, Since must be before
//     Until (the half-open `[Since, Until)` semantics require it).
//
// Wraps [ErrInvalidFilter] so callers branch with [errors.Is].
func (q EventQuery) Validate() error {
	if q.Type != "" && q.Category != "" {
		return fmt.Errorf("%w: Type and Category are mutually exclusive", ErrInvalidFilter)
	}
	if q.Limit < 0 {
		return fmt.Errorf("%w: Limit must be non-negative, got %d", ErrInvalidFilter, q.Limit)
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && !q.Since.Before(q.Until) {
		return fmt.Errorf("%w: Since (%s) must be before Until (%s)", ErrInvalidFilter, q.Since, q.Until)
	}
	return nil
}

// EventPage is the result of [EventStore.Query]: the events in the
// page plus a cursor for the next page. NextCursor is empty when the
// store returned fewer than Limit events — there is definitely no
// next page.
//
// Callers MUST NOT assume a non-empty NextCursor guarantees more
// events; the cursor is only an upper bound. A second Query that
// returns zero events is the canonical "we're caught up" signal.
type EventPage struct {
	Events     []Event
	NextCursor string
}

// RetentionPolicy is one row in the retention enforcer's per-type
// table. Type is the canonical `<category>.<subtype>` string (or
// empty for the catch-all "applies to every type" rule); MaxAge is
// the maximum age past which an event is removed; MaxCount caps the
// number of events of the matching type at the newest N.
//
// A policy with both MaxAge and MaxCount zero is a no-op (and is
// skipped by the store).
type RetentionPolicy struct {
	Type     EventType
	MaxAge   time.Duration
	MaxCount int
}

// severitiesAtLeast returns the canonical lowercase names of every
// severity at or above the given threshold, in ordering order. The
// closed enum makes the result deterministic and bounded (≤ 5
// entries); callers translate this into a SQL IN clause.
//
// Returns nil for an invalid threshold so the wrapper code can detect
// "no threshold filter" via len(...)==0 without a separate flag.
func severitiesAtLeast(threshold Severity) []string {
	if !threshold.IsValid() {
		return nil
	}
	out := make([]string, 0, 5)
	for _, lvl := range AllSeverities() {
		if lvl.AtLeast(threshold) {
			out = append(out, lvl.String())
		}
	}
	return out
}
