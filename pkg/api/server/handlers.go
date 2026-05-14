package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/agents"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
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
	cluster.NewHandler().Register(mux)
	events.NewHandler().Register(mux)
	execution.NewHandler().Register(mux)
	gitops.NewHandler().Register(mux)
	maintenance.NewHandler().Register(mux)
	policy.NewHandler().Register(mux)
	runbook.NewHandler().Register(mux)
	schedule.NewHandler().Register(mux)
	secrets.NewHandler(s.secretsBroker, s.secretsTransit, s.secretsLeases).Register(mux)
	stateapi.NewHandler().Register(mux)
	webhooks.NewHandler().Register(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
