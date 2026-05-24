// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
)

// StoreAuditor satisfies [Auditor] by writing every entry through an
// [AuditStore]. Failures are logged via slog and increment
// [FailedStores]; never returned to the producer (Epic 10/11
// "fire and forget" precedent applies).
//
// Task 4 wires this in front of an [AuditStore] backed by the SQL
// state composite (Epic 12 task 2's [NewSQLAuditStore]) and uses it
// as one branch of a [MultiAuditor] fan-out alongside an in-memory
// [BufferedAuditor] for kscore-audit's live tail.
type StoreAuditor struct {
	store        AuditStore
	logger       *slog.Logger
	failedStores atomic.Int64
}

// NewStoreAuditor wraps store. logger is used to record dropped
// entries when [AuditStore.Store] returns an error; passing nil
// falls back to a discard logger so callers don't have to import
// slog just to suppress noise in tests.
func NewStoreAuditor(store AuditStore, logger *slog.Logger) *StoreAuditor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &StoreAuditor{store: store, logger: logger}
}

// Emit forwards the entry to the backing store. Errors are logged
// + counted; never propagated.
//
// A nil receiver or nil backing store silently no-ops: keeps boot
// wiring tolerant of partial assembly during start-up where the
// store isn't ready yet but a downstream subsystem already calls
// Emit.
func (a *StoreAuditor) Emit(ctx context.Context, entry AuditEntry) {
	if a == nil || a.store == nil {
		return
	}
	if err := a.store.Store(ctx, entry); err != nil {
		a.failedStores.Add(1)
		a.logger.LogAttrs(ctx, slog.LevelWarn,
			"audit: StoreAuditor.Emit dropped entry",
			slog.String("entry_id", entry.ID),
			slog.String("action", entry.Action),
			slog.Any("error", err),
		)
	}
}

// FailedStores returns the count of entries dropped because the
// backing AuditStore.Store call returned an error. Operator
// telemetry / health check reads this to detect a stuck audit log.
func (a *StoreAuditor) FailedStores() int64 {
	if a == nil {
		return 0
	}
	return a.failedStores.Load()
}

// Compile-time assertion that *StoreAuditor satisfies [Auditor].
var _ Auditor = (*StoreAuditor)(nil)
