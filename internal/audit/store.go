package audit

import (
	"context"
	"fmt"
	"time"
)

// DefaultQueryLimit is the page size [AuditStore.Query] applies
// when the caller leaves [AuditQuery.Limit] at zero. Matches the
// §4.12 CLI default for `kscore-audit log --limit 50`.
const DefaultQueryLimit = 100

// Default retention policy per PROJECT-DETAILS §4.12.
const (
	DefaultRetentionMaxAge       = 90 * 24 * time.Hour
	DefaultRetentionMaxCount     = 100_000
	DefaultRetentionInterval     = time.Hour
	DefaultRetentionJitter       = 0.1
)

// AuditStore is the consumer-facing persistence interface for the
// v1.0 audit log. Epic 12 task 4's emission hooks (auth, secrets,
// state-apply, command-exec, policy-eval) write through Store;
// the gRPC handler (task 12) reads via Get / Query / Count; the
// retention enforcer (this task) drives ApplyRetention; the export
// surface (task 15) reads via Query + applies redaction (this
// task's RedactionConfig).
//
// Implementations layer above an `internal/state.AuditStore` (see
// [NewSQLAuditStore]) and translate typed values to DB-shape.
type AuditStore interface {
	// Store validates the entry and persists it. Returns
	// [ErrInvalidAuditEntry] for any structural rejection.
	// PRIMARY-KEY collision wraps `state.ErrDuplicate`.
	Store(ctx context.Context, e AuditEntry) error

	// StoreBatch persists every entry atomically — all rows land
	// or none do. Every entry is validated before any DB call.
	StoreBatch(ctx context.Context, entries []AuditEntry) error

	// Get returns the entry with the given ID. Returns
	// `state.ErrNotFound` when absent.
	Get(ctx context.Context, id string) (AuditEntry, error)

	// Query returns entries matching the filter, paginated by
	// k-sortable [AuditEntry.ID]. [AuditPage.NextCursor] is empty
	// when no more entries match.
	Query(ctx context.Context, q AuditQuery) (AuditPage, error)

	// Count returns the total number of entries matching the
	// filter (ignoring Cursor / Limit / Descending).
	Count(ctx context.Context, q AuditQuery) (int, error)

	// Delete removes a single entry by ID.
	Delete(ctx context.Context, id string) error

	// ApplyRetention applies the policy and returns the total
	// number of rows removed.
	ApplyRetention(ctx context.Context, policy RetentionPolicy) (int, error)

	// Summarize aggregates the filter set into totals + denied
	// breakdowns + time range. Cursor / Limit / Descending fields
	// of the query are ignored.
	Summarize(ctx context.Context, q AuditQuery) (AuditSummary, error)

	// Close releases resources held by the store implementation.
	// Idempotent.
	Close() error
}

// AuditQuery is the typed filter the gRPC handler (task 12) and
// CLI (task 14) build before handing off to [AuditStore.Query].
// Zero-value fields are ignored.
//
// Cursor pagination uses [AuditEntry.ID] (UUIDv7 — k-sortable by
// stamping time). First page passes Cursor=""; subsequent pages
// pass the previous page's [AuditPage.NextCursor].
//
// MinSeverity is the threshold; the wrapper translates it to an
// IN-clause over the closed severity set ahead of the SQL call.
type AuditQuery struct {
	PolicyID     string
	User         string
	ResourceType string
	Action       string
	MinSeverity  Severity
	Allowed      *bool
	Since        time.Time
	Until        time.Time
	Cursor       string
	Limit        int
	Descending   bool
}

// Validate enforces the structural invariants.
func (q AuditQuery) Validate() error {
	if q.Limit < 0 {
		return fmt.Errorf("%w: Limit must be non-negative, got %d", ErrInvalidAuditEntry, q.Limit)
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && !q.Since.Before(q.Until) {
		return fmt.Errorf("%w: Since (%s) must be before Until (%s)", ErrInvalidAuditEntry, q.Since, q.Until)
	}
	return nil
}

// AuditPage is the result of [AuditStore.Query].
type AuditPage struct {
	Entries    []AuditEntry
	NextCursor string
}

// TimeRange is the [time.Time] half-open interval reported by
// [AuditSummary]. Start is the minimum timestamp of the filtered
// set, End is the maximum. Both are zero when no rows match.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// AuditSummary is the §4.12 aggregation of the filtered audit set.
// Counts are over the full filtered set; ViolationsByPolicy and
// ViolationsBySeverity count DENIED entries only (allowed=false) —
// they answer "what is failing policy" not "what was evaluated".
// Range reports min/max timestamp of the matched rows.
type AuditSummary struct {
	TotalEvaluations     int
	AllowedCount         int
	DeniedCount          int
	ViolationsByPolicy   map[string]int
	ViolationsBySeverity map[Severity]int
	Range                TimeRange
}

// RetentionPolicy is the §4.12 retention rule applied by the
// [RetentionEnforcer]. MinSeverity exempts at-or-above entries
// from deletion (operators keep `critical` audit forever).
//
// RetentionInterval is the scheduler's tick cadence; the
// enforcer's [WithRetentionInterval] option exposes it.
type RetentionPolicy struct {
	MaxAge            time.Duration
	MaxCount          int
	MinSeverity       Severity
	RetentionInterval time.Duration
}

// DefaultRetentionPolicy returns the §4.12 defaults: 90d / 100k /
// no MinSeverity exemption / 1h interval.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxAge:            DefaultRetentionMaxAge,
		MaxCount:          DefaultRetentionMaxCount,
		MinSeverity:       SeverityUnknown,
		RetentionInterval: DefaultRetentionInterval,
	}
}

// severitiesAtLeast returns the canonical lowercase names of every
// severity at or above the given threshold, in ordering order.
// Returns nil for an invalid threshold (no severity filter).
// Mirrors the events-package helper.
func severitiesAtLeast(threshold Severity) []string {
	if !threshold.IsValid() {
		return nil
	}
	out := make([]string, 0, 4)
	for _, lvl := range AllSeverities() {
		if lvl.AtLeast(threshold) {
			out = append(out, lvl.String())
		}
	}
	return out
}
