package server

import (
	"context"
	"log/slog"
	"time"

	"go.keystone-core.io/keystone-core/internal/health"
	"go.keystone-core.io/keystone-core/internal/state"
)

// componentStatus is the per-check JSON shape rendered into
// /health/ready, /health/status, and /api/status.
//
// Status is the coarse "ok" | "fail" string (no error detail) — error
// strings on the public surface would leak topology info. Detailed
// errors land in logs at warn level.
type componentStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

// healthSnapshot is the response body for /health/ready (with a 503
// status code when Ready=false) and /health/status (always 200).
//
// PROJECT-DETAILS §4.4: ready=true only when ALL components ok AND
// the startup grace period has elapsed.
type healthSnapshot struct {
	Ready         bool                       `json:"ready"`
	InGracePeriod bool                       `json:"in_grace_period"`
	StartedAt     string                     `json:"started_at"`
	UptimeSeconds float64                    `json:"uptime_seconds"`
	Components    map[string]componentStatus `json:"components"`
}

// healthChecker is the server-layer wire-format adapter over the
// Epic-17 task-6 internal/health.Registry. Epic 04 shipped the inline
// nats+db version of this; the registry moved into internal/health so
// JetStream + operator-supplied custom checks can plug in without
// churning every test fixture.
//
// The "ok" / "fail" wire values are intentional — the JSON surface
// predates the richer health.Status enum and changing it would break
// existing dashboards / probes. Internal callers wanting the richer
// enum go through health.Registry directly.
type healthChecker struct {
	reg *health.Registry
}

func newHealthChecker(
	nats NATSManager,
	store state.HealthStore,
	startedAt time.Time,
	grace, checkTimeout time.Duration,
	now func() time.Time,
	logger *slog.Logger,
	extras ...health.Checker,
) *healthChecker {
	reg := health.NewRegistry(health.Options{
		CheckTimeout:       checkTimeout,
		StartupGracePeriod: grace,
		StartedAt:          startedAt,
		Now:                now,
		Logger:             logger,
	})
	// NATS + DB are the §4.4 baseline. JetStream (epic 17 task 6) and
	// any operator-supplied custom checks come through extras.
	reg.Register(
		health.NewNATSChecker(nats, 0),
		health.NewDBChecker(store, 0),
	)
	reg.Register(extras...)
	return &healthChecker{reg: reg}
}

// Snapshot runs every registered check in parallel under the configured
// per-check timeout and renders the result in the long-standing public
// JSON shape. Map entries are ordered insertion-free on the wire (Go
// JSON encoder sorts map keys alphabetically); existing dashboards rely
// on the component-key names, not their position.
func (h *healthChecker) Snapshot(ctx context.Context) healthSnapshot {
	snap := h.reg.Snapshot(ctx)
	components := make(map[string]componentStatus, len(snap.Results))
	for _, r := range snap.Results {
		components[r.Name] = componentStatus{
			Status:    statusWire(r.Status),
			LatencyMS: r.Latency.Milliseconds(),
		}
	}
	return healthSnapshot{
		Ready:         snap.Ready,
		InGracePeriod: snap.InGracePeriod,
		StartedAt:     snap.StartedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: snap.Uptime.Seconds(),
		Components:    components,
	}
}

// statusWire maps the rich health.Status to the legacy "ok"/"fail"
// strings the public API serves. Anything not StatusHealthy is "fail" —
// degraded / unknown collapse to fail so probes don't accidentally pass
// on partial state.
func statusWire(s health.Status) string {
	if s == health.StatusHealthy {
		return "ok"
	}
	return "fail"
}
