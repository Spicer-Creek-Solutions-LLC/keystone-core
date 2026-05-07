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
)

// registerHealthEndpoints wires the §4.4 unauthenticated /health/*
// endpoints. Task 4 ships placeholders that always return 200 (or
// trivial payloads); task 7 replaces these with real readiness checks.
//
// /api/status is registered separately on the auth'd mux by
// buildHTTPHandler — it's an operator-only endpoint with goroutine /
// memory metrics and shouldn't ride the public health surface.
func (s *Server) registerHealthEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		// task 7: actual NATS + DB checks + grace period
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /health/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"status":     "ok",
			"checked_at": s.now().UTC().Format(time.RFC3339),
		})
	})
}

// handleAPIStatus serves /api/status. Task 9 extends with production
// warnings; task 7 fleshes out per-component latencies.
func (s *Server) handleAPIStatus(w http.ResponseWriter, _ *http.Request) {
	counts := s.connMgr.Counts()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeJSON(w, map[string]any{
		"version":  s.version,
		"uptime":   s.now().Sub(s.startedAt).String(),
		"agents": map[string]int{
			"total":     counts.Total,
			"connected": counts.Connected,
			"stale":     counts.Stale,
			"disabled":  counts.Disabled,
		},
		"runtime": map[string]any{
			"goroutines":   runtime.NumGoroutine(),
			"alloc_bytes":  mem.Alloc,
			"sys_bytes":    mem.Sys,
			"num_gc":       mem.NumGC,
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
	secrets.NewHandler().Register(mux)
	stateapi.NewHandler().Register(mux)
	webhooks.NewHandler().Register(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
