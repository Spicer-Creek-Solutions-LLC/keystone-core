package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

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

// healthChecker pings the configured backends and renders snapshots.
// It runs the registered checks in parallel, bounded by checkTimeout
// per-check, and reports each result into Components.
type healthChecker struct {
	nats         NATSManager
	store        state.HealthStore
	startedAt    time.Time
	grace        time.Duration
	checkTimeout time.Duration
	now          func() time.Time
	logger       *slog.Logger
}

func newHealthChecker(
	nats NATSManager,
	store state.HealthStore,
	startedAt time.Time,
	grace, checkTimeout time.Duration,
	now func() time.Time,
	logger *slog.Logger,
) *healthChecker {
	if checkTimeout == 0 {
		checkTimeout = 2 * time.Second
	}
	if grace == 0 {
		grace = 30 * time.Second
	}
	return &healthChecker{
		nats:         nats,
		store:        store,
		startedAt:    startedAt,
		grace:        grace,
		checkTimeout: checkTimeout,
		now:          now,
		logger:       logger,
	}
}

// Snapshot runs the configured checks in parallel and renders the
// result. Total latency is bounded by checkTimeout, not by the sum of
// per-check latencies.
func (h *healthChecker) Snapshot(ctx context.Context) healthSnapshot {
	now := h.now()
	uptime := now.Sub(h.startedAt)
	inGrace := uptime < h.grace

	components := h.runChecks(ctx)
	allOK := true
	for _, c := range components {
		if c.Status != "ok" {
			allOK = false
			break
		}
	}

	return healthSnapshot{
		Ready:         allOK && !inGrace,
		InGracePeriod: inGrace,
		StartedAt:     h.startedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: uptime.Seconds(),
		Components:    components,
	}
}

// runChecks fires every check in its own goroutine, bounded by the
// configured per-check timeout, and aggregates the results into a
// single map. Each check is independent — a slow NATS check does not
// delay the DB result.
func (h *healthChecker) runChecks(ctx context.Context) map[string]componentStatus {
	type result struct {
		name   string
		status componentStatus
	}
	out := make(map[string]componentStatus, 2)
	results := make(chan result, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- result{name: "nats", status: h.timed(ctx, "nats", h.nats.Health)}
	}()
	go func() {
		defer wg.Done()
		results <- result{name: "db", status: h.timed(ctx, "db", h.store.Ping)}
	}()
	wg.Wait()
	close(results)

	for r := range results {
		out[r.name] = r.status
	}
	return out
}

// timed runs fn under a per-check timeout context and records the
// latency. On error, status is "fail" and the error is logged at warn
// level — never embedded in the response payload.
func (h *healthChecker) timed(parent context.Context, name string, fn func(context.Context) error) componentStatus {
	ctx, cancel := context.WithTimeout(parent, h.checkTimeout)
	defer cancel()

	start := h.now()
	err := fn(ctx)
	elapsed := h.now().Sub(start)

	cs := componentStatus{LatencyMS: elapsed.Milliseconds()}
	if err != nil {
		cs.Status = "fail"
		h.logger.Warn("server: health check failed",
			"component", name, "err", err, "latency_ms", cs.LatencyMS)
	} else {
		cs.Status = "ok"
	}
	return cs
}
