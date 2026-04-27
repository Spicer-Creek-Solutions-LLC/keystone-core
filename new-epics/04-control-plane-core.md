# Epic 04: Control Plane Core

**Phase**: B • **Estimate**: 2 weeks • **Depends on**: 01, 02, 03 • **Blocks**: 06, 13, 16, 17

## Goal

`kscore-server` daemon that wires together NATS, storage, services, gRPC + REST listeners with a deterministic startup sequence and a clean graceful-shutdown sequence. Health endpoints, middleware chain, dual-stack listeners, signals.

## Scope (in)

- `cmd/kscore-server/main.go` — Cobra root + subcommands (`run`, `version`).
- `internal/controlplane/` — orchestration:
  - Strict 21-step startup sequence (see PROJECT-DETAILS §4.4).
  - `ConnectionManager` (registration, heartbeat-monitor loop default 10s, stale detection at 3 missed).
  - `CommandDispatcher` (route, timeout, retention loop).
  - `BatchDispatcher` (batch state machine).
  - Adapters (`stateStoreAdapter`, `shardManagerAdapter`) to avoid cyclic imports.
  - 30s status-ticker loop logging agent counts + health.
- `pkg/api/server/` — `Server` orchestrator:
  - Listener creation with dual-stack (IPv4 + IPv6) — `[::]:9090` + `0.0.0.0:9090` for gRPC; same for HTTP.
  - gRPC server with chained interceptors (CORS no-op → rate-limit → auth) per `pkg/api/auth.InterceptorConfig`.
  - HTTP mux with health endpoints (`/health/live`, `/health/ready`, `/health/status`, `/api/status`) + middleware (CORS outermost).
  - Reverse-of-init graceful shutdown: gRPC GracefulStop → ConnMgr Stop → Store Close → NATS Shutdown → HTTP Shutdown(ctx 30s) → tracing/profiling cleanup (5s timeouts).
- Optional components gated by config: webhook receiver port (8081), profiling port (6060, default off), tracing exporters, policy engine, cluster mode (etcd; covered by Epic 13).
- Production warnings on startup (embedded NATS in production, SQLite in production, TLS off, JetStream defaults).
- Default zero-config startup: embedded NATS + SQLite + dev API key with loud warning.

## Scope (out / non-goals)

- Cluster wiring — Epic 13.
- Operator embedding — v1.3.
- gRPC reflection / channelz — v1.0.x dot release.

## Design summary

See `PROJECT-DETAILS.md §4.4`.

## Tasks

1. **`internal/controlplane/connection_manager.go`** — agent registration, in-memory + DB-persisted state, heartbeat monitor goroutine, stale eviction.
2. **`internal/controlplane/command_dispatcher.go`** — dispatch via NATS pub/sub (NATS lands in Epic 05; stub `NATSPublisher` interface here for tests).
3. **`internal/controlplane/batch_dispatcher.go`** — see Epic 07 for the targeting + execution side; this epic provides the orchestrator + persistence.
4. **`pkg/api/server/server.go`** — Server struct with Start/Stop methods. 21-step init.
5. **Listener creation** with `ensureIPv6Brackets()` helper + dual-stack helper. Tests for both IPv4-only, IPv6-only, dual-stack configurations.
6. **Middleware chain** wired with auth from Epic 03.
7. **Health endpoints**: `/health/live` (200 trivial), `/health/ready` (NATS + DB checks; respect `health.startup_grace_period` default 30s), `/health/status` (component latencies), `/api/status` (uptime, version, agent counts, memory, goroutines).
8. **Graceful shutdown** sequence with deferred Close and 30s context timeout per HTTP listener.
9. **Production warnings** — `Config.ProductionWarnings()` returns the list; logged at startup; exposed via `/api/status` for ops dashboards.
10. **Integration test**: spawn full server with test config (SQLite, embedded NATS via stub, auth disabled), gRPC client calls GetServerStatus, REST client calls /health/ready, trigger SIGTERM, verify clean exit and no leaks.

## Acceptance criteria

- [ ] `kscore-server run --config dev.yaml` starts in <2s on a laptop.
- [ ] First-run banner includes versions, ports, auth mode, production warnings.
- [ ] All 7 v1.0 gRPC services register (with nil-guarded conditional services for cluster/policy/etc.).
- [ ] All v1.0 REST handler routes registered (with nil-guard for not-yet-implemented domains).
- [ ] `/health/live` → 200 always.
- [ ] `/health/ready` → 503 during grace period, 200 after when deps healthy, 503 if NATS or DB unreachable.
- [ ] CORS preflight does not consume rate-limit budget.
- [ ] Auth interceptor logs denials and includes principal info on accepted requests.
- [ ] SIGTERM produces ordered shutdown logs; integration test verifies no goroutine leaks (`goleak` package).
- [ ] Coverage >75% on `internal/controlplane`, `pkg/api/server`.

## Risks

- **Init order regressions**: tests must catch nil-deref panics introduced by future additions. Add a "registration ordering" test that fails if any service is registered before its dep.
- **Listener leaks on partial-init failure**: defer Close in setup function; verified by goleak.
- **Slow CI**: integration test should boot in <5s; if slower, profile and trim.

## References

- PROJECT-DETAILS §4.4.
