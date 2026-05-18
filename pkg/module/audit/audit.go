// Package audit defines the module-domain capability-invocation
// audit record and the bridge that forwards it to the §4.12 audit
// log (Epic 14 task 2).
//
// Every capability a module invokes produces an [Entry]. The
// [Auditor] sink is fire-and-forget (no error back to the caller —
// the §4.11 "failure to log is a bug, not a request failure"
// contract, mirroring internal/audit.Auditor). [StoreBridge] adapts
// an [Auditor] onto an internal/audit.Auditor so module activity
// lands in the same audit store as policy/secrets/state.
package audit

import (
	"context"
	"log/slog"
	"time"

	iaudit "go.keystone-core.io/keystone-core/internal/audit"
)

// Entry is one capability invocation by a loaded module
// (PROJECT-DETAILS §4.18 shape).
type Entry struct {
	Timestamp  time.Time
	Module     string // namespaced vendor/pkg
	Version    string
	Capability string // e.g. "fs.write"
	Operation  string // capability-specific verb, e.g. "write" / "denied"
	Success    bool
	Duration   time.Duration
	Details    map[string]string
}

// Auditor records module capability invocations. Implementations
// must not block or error back to the caller.
type Auditor interface {
	Emit(ctx context.Context, e Entry)
}

// NoopAuditor discards every entry — the default when none is
// wired. A nil *NoopAuditor is safe.
type NoopAuditor struct{}

// Emit implements [Auditor].
func (NoopAuditor) Emit(context.Context, Entry) {}

// StoreBridge adapts the module [Auditor] onto the §4.12
// internal/audit.Auditor, so module capability activity is
// persisted + queryable alongside every other audited domain.
type StoreBridge struct {
	sink iaudit.Auditor
	log  *slog.Logger
}

// NewStoreBridge wires the bridge. A nil sink falls back to
// internal/audit.NoopAuditor; a nil logger to slog.Default.
func NewStoreBridge(sink iaudit.Auditor, logger *slog.Logger) *StoreBridge {
	if sink == nil {
		sink = iaudit.NoopAuditor{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &StoreBridge{sink: sink, log: logger}
}

// Emit maps a module [Entry] onto an internal/audit.AuditEntry and
// forwards it. A construction error is logged and dropped — never
// propagated (audit emission must not fail the module call).
func (b *StoreBridge) Emit(ctx context.Context, e Entry) {
	meta := map[string]string{
		"module":     e.Module,
		"version":    e.Version,
		"capability": e.Capability,
		"operation":  e.Operation,
	}
	for k, v := range e.Details {
		// Don't let a Details key shadow the canonical fields.
		if _, reserved := meta[k]; !reserved {
			meta[k] = v
		}
	}
	sev := iaudit.SeverityLow
	if !e.Success {
		sev = iaudit.SeverityMedium // denials / failed invocations are noteworthy
	}
	entry, err := iaudit.NewAuditEntry(iaudit.AuditEntryInput{
		ResourceType: "module",
		Action:       "module." + e.Capability,
		Allowed:      e.Success,
		Duration:     e.Duration,
		Severity:     sev,
		User:         e.Module + "@" + e.Version,
		Metadata:     meta,
	})
	if err != nil {
		b.log.Warn("module audit entry dropped (construction failed)",
			"module", e.Module, "capability", e.Capability, "err", err)
		return
	}
	b.sink.Emit(ctx, entry)
}
