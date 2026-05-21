package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.keystone-core.io/keystone-core/pkg/api/agents"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	blueprintapi "go.keystone-core.io/keystone-core/pkg/api/blueprint"
	"go.keystone-core.io/keystone-core/pkg/api/cluster"
	"go.keystone-core.io/keystone-core/pkg/api/events"
	"go.keystone-core.io/keystone-core/pkg/api/execution"
	"go.keystone-core.io/keystone-core/pkg/api/gitops"
	"go.keystone-core.io/keystone-core/pkg/api/maintenance"
	"go.keystone-core.io/keystone-core/pkg/api/policy"
	"go.keystone-core.io/keystone-core/pkg/api/runbook"
	"go.keystone-core.io/keystone-core/pkg/api/schedule"
	"go.keystone-core.io/keystone-core/pkg/api/secrets"
	stateapi "go.keystone-core.io/keystone-core/pkg/api/state"
	"go.keystone-core.io/keystone-core/pkg/api/webhooks"
	"go.keystone-core.io/keystone-core/pkg/natsstatus"
)

// registerHealthEndpoints wires the §4.4 unauthenticated /health/*
// endpoints. /health/live is trivial; /health/ready and
// /health/status delegate to healthChecker (task 7) which pings NATS
// + DB and applies the startup grace period.
//
// /api/status is registered separately on the auth'd mux by
// buildHTTPHandler — it's an operator-only endpoint with goroutine /
// memory metrics and shouldn't ride the public health surface.
func (s *Server) registerHealthEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /health/ready", s.handleHealthReady)
	mux.HandleFunc("GET /health/status", s.handleHealthStatus)
}

// registerMetricsEndpoint mounts the Prom exposition handler on mux at
// cfg.Metrics.Path. Same auth posture as /health/* (none) per Prom
// convention; same listener as the rest of HTTP so no new port is
// required. Skipped when metrics are disabled or no registry was
// supplied to Options.
func (s *Server) registerMetricsEndpoint(mux *http.ServeMux) {
	if !s.cfg.Metrics.Enabled || s.metricsRegistry == nil {
		return
	}
	h := promhttp.HandlerFor(s.metricsRegistry.Gatherer(), promhttp.HandlerOpts{
		ErrorLog:      promErrorLog{logger: s.logger},
		ErrorHandling: promhttp.ContinueOnError,
	})
	mux.Handle("GET "+s.cfg.Metrics.Path, h)
}

// promErrorLog adapts *slog.Logger to promhttp.Logger so scrape-time
// gather errors land in the structured log stream instead of stderr.
type promErrorLog struct{ logger *slog.Logger }

// Println implements promhttp.Logger.
func (p promErrorLog) Println(v ...any) {
	if p.logger == nil {
		return
	}
	msg := ""
	for i, a := range v {
		if i > 0 {
			msg += " "
		}
		msg += fmtAny(a)
	}
	p.logger.Warn("server: /metrics gather error", "err", msg)
}

func fmtAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return ""
	}
}

// handleHealthReady runs the configured checks and returns 200 only
// when every component is OK and the startup grace period has
// elapsed. Otherwise returns 503 with the same JSON body so ops
// tooling can inspect why.
func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	snap := s.healthChecker.Snapshot(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if !snap.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(snap)
}

// handleHealthStatus returns 200 with the snapshot regardless of
// readiness — it's a diagnostic endpoint, not a probe.
func (s *Server) handleHealthStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.healthChecker.Snapshot(r.Context())
	writeJSON(w, snap)
}

// handleAPIStatus serves /api/status. Includes the same per-component
// latency snapshot as /health/status for operators inspecting the
// auth'd surface, plus the production-warning list, auth mode, and
// per-NATS-endpoint state (Epic 05 task 11) for ops dashboards.
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	counts := s.connMgr.Counts()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snap := s.healthChecker.Snapshot(r.Context())

	// Always emit production_warnings + nats_endpoints as arrays
	// (never null) so dashboards can render conditionally without a
	// nil-check. Embedded mode and a not-yet-started Manager both
	// surface as an empty array.
	warnings := s.ProductionWarnings()
	if warnings == nil {
		warnings = []string{}
	}
	endpoints := s.nats.EndpointSnapshots()
	if endpoints == nil {
		endpoints = []natsstatus.EndpointSnapshot{}
	}

	writeJSON(w, map[string]any{
		"version":             s.version,
		"uptime":              s.now().Sub(s.startedAt).String(),
		"started_at":          s.startedAt.UTC().Format(time.RFC3339),
		"ready":               snap.Ready,
		"auth_mode":           s.authMode(),
		"production_warnings": warnings,
		"components":          snap.Components,
		"nats_endpoints":      endpoints,
		"agents": map[string]int{
			"total":     counts.Total,
			"connected": counts.Connected,
			"stale":     counts.Stale,
			"disabled":  counts.Disabled,
		},
		"runtime": map[string]any{
			"goroutines":  runtime.NumGoroutine(),
			"alloc_bytes": mem.Alloc,
			"sys_bytes":   mem.Sys,
			"num_gc":      mem.NumGC,
		},
	})
}

// registerDomainHandlers calls Register on every per-domain handler
// scaffolded by Epic 03 task 7. Each one currently returns 501 until
// the owning epic ships its concrete impl. apikeys is the only handler
// with a real impl already (Epic 03 task 5b) and so takes the store.
func (s *Server) registerDomainHandlers(mux *http.ServeMux) {
	agents.NewHandler().Register(mux)
	apikeys.NewHandler(s.store).Register(mux)
	blueprintapi.NewHandler(blueprintapi.Providers{}).Register(mux)
	// Real providers are wired when clustering is constructed at
	// boot (deferred — see the "Cluster gRPC services boot
	// registration" ROADMAP entry); until then routes 503.
	cluster.NewHandler(cluster.ClusterProviders{}).Register(mux)
	events.NewHandler(s.eventStore, s.eventPublisher).Register(mux)
	execution.NewHandler().Register(mux)
	gitops.NewHandler(s.gitopsProviders).Register(mux)
	maintenance.NewHandler().Register(mux)
	policy.NewHandler(s.policyEngine, s.policyReports, s.policyAuditLog, s.policyAuditor).Register(mux)
	runbook.NewHandler(runbook.Providers{}).Register(mux)
	schedule.NewHandler().Register(mux)
	secrets.NewHandler(s.secretsBroker, s.secretsTransit, s.secretsLeases).Register(mux)
	stateapi.NewHandler().Register(mux)
	webhooks.NewHandler(s.webhookProviders).Register(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
