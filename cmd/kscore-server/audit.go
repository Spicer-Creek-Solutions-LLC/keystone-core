package main

import (
	"context"
	"log/slog"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/state"
)

// DefaultAuditBufferSize is the in-memory ring buffer capacity for
// the live-tail [audit.BufferedAuditor]. Sized so kscore-audit
// (task 14) can show ~10 seconds of audit events at a busy server's
// rate without runaway memory.
const DefaultAuditBufferSize = 1000

// auditRuntime carries the long-lived audit infrastructure built at
// boot. Subsystem startups (secrets, auth, exec, state-apply) receive
// FanOut and emit through it; the SQL store + buffer underneath
// persist + expose the entries.
type auditRuntime struct {
	// Store is the SQL-backed [audit.AuditStore] wrapping the state
	// composite. nil when state is nil (test wiring).
	Store audit.AuditStore

	// StoreAuditor wraps Store as a [audit.Auditor]. Boot wiring
	// reads this for telemetry (FailedStores counter) but emission
	// goes through FanOut.
	StoreAuditor *audit.StoreAuditor

	// Buffer is the in-memory ring buffer for kscore-audit's live
	// tail (task 14). Operators read the most-recent N entries
	// without hitting SQL.
	Buffer *audit.BufferedAuditor

	// FanOut is the [audit.MultiAuditor] every subsystem emits
	// through. Includes StoreAuditor + Buffer when both configured.
	FanOut audit.Auditor
}

// startAudit assembles the v1.0 audit pipeline and returns the
// runtime. Returns a nil runtime + nil error when state is nil
// (degraded test paths) — callers treat nil as "no audit; subsystems
// fall back to NoopAuditor."
//
// The pipeline is:
//
//	MultiAuditor → StoreAuditor → AuditStore → state.AuditStore → SQL
//	            → BufferedAuditor → ring (live tail)
//
// Both branches share the same [audit.AuditEntry]; the events bus
// emission lives separately on each subsystem (Epic 11 task 10's
// bridge + secrets-side path).
func startAudit(ctx context.Context, st state.Store, am *audit.Metrics, log *slog.Logger) (*auditRuntime, error) {
	if st == nil {
		log.LogAttrs(ctx, slog.LevelInfo, "audit: state store is nil; skipping")
		return nil, nil
	}
	store := audit.NewSQLAuditStore(st)
	storeAud := audit.NewStoreAuditor(store, log)

	buf, err := audit.NewBufferedAuditor(DefaultAuditBufferSize)
	if err != nil {
		return nil, err
	}

	// MeasuringAuditor wraps the fan-out so every emission across
	// every subsystem records kscore_audit_entries_total{policy,allowed}.
	// nil-metrics is the no-op pass-through.
	var fanOut audit.Auditor = audit.NewMultiAuditor(storeAud, buf)
	fanOut = audit.NewMeasuringAuditor(fanOut, am)
	return &auditRuntime{
		Store:        store,
		StoreAuditor: storeAud,
		Buffer:       buf,
		FanOut:       fanOut,
	}, nil
}

// stop releases the audit store. Idempotent: nil receiver or nil
// store is a no-op.
func (r *auditRuntime) stop(_ context.Context, _ *slog.Logger) {
	if r == nil || r.Store == nil {
		return
	}
	_ = r.Store.Close()
}
