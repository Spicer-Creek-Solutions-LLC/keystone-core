# Epic 17: Observability — Logs, Metrics, Traces, Health, Grafana

**Phase**: L • **Estimate**: 1.5 weeks • **Depends on**: 01, 04 • **Blocks**: nothing

## Goal

Production-grade observability baseline: structured logs (already in Epic 01), Prometheus metrics, OTel traces, health endpoints (already wired in Epic 04 — this epic completes the registry + dashboards), pre-built Grafana dashboards. **TUI monitor is v1.2; telemetry gateway is v1.4** — neither blocks v1.0.

## Scope (in)

### Metrics (`internal/metrics/`)

- Custom Prom registry; `Collector` interface (counter, gauge, histogram, summary); `MetricRegistry` with metric definitions; `Timer` utility; `cardinality.Limiter` enforces hard label-cardinality limits with drop/aggregate fallback.
- `/metrics` HTTP endpoint on the main HTTP server (configurable; gated by `metrics.enabled=true`, default true).
- v1.0 metric set:
  - `kscore_agents_total{cluster, status}` — gauge.
  - `kscore_commands_executed_total{status, agent}` — counter.
  - `kscore_command_duration_seconds{type}` — histogram.
  - `kscore_state_apply_total{result}` — counter.
  - `kscore_state_drift_detected_total{severity}` — counter.
  - `kscore_events_emitted_total{type, severity}` — counter.
  - `kscore_secrets_access_total{backend, op, result}` — counter.
  - `kscore_audit_entries_total{policy, allowed}` — counter.
  - `kscore_cluster_members_total{state}` — gauge.
  - `kscore_cluster_quorum` — gauge (0 = lost, 1 = healthy).
  - `kscore_cluster_failover_total{outcome}` — counter.
  - `kscore_grpc_request_duration_seconds{method, code}` — histogram.
  - `kscore_http_request_duration_seconds{method, code, route}` — histogram.
  - Plus pkg/wait, NATS connection, etcd, DB connection-pool metrics.

### Tracing (`internal/tracing/`)

- OTel SDK; `TracerProvider`; samplers: `always_on/off`, `probabilistic`, `parent_based`, `rate_limiting`, `adaptive`.
- Exporters: OTLP (gRPC + HTTP), Zipkin, stdout.
- Helper attribute functions: `AgentAttrs`, `JobAttrs`, `StateAttrs`, `EventAttrs`, `PolicyAttrs`.
- Batch processor with configurable batch size + flush interval.
- Default v1.0: probabilistic 0.1 sample rate.

### Health (`internal/health/`) — extends Epic 04

- `Checker` interface (`Check`, `Name`, `Interval`).
- v1.0 checkers: NATS, DB, JetStream, custom.
- `Status` enum (healthy / degraded / unhealthy / unknown).
- `/health/live` (always 200), `/health/ready` (NATS + DB), `/health/status` (component latencies + agent pool ratio), `/api/status` (uptime, agent counts, version, memory, goroutines).
- Startup grace period default 30s.

### Profiling (`internal/profiling/`)

- pprof endpoints (CPU, memory, goroutine, mutex contention).
- Default off; opt-in via `profiling.enabled=true`. Listen port default 6060.

### Grafana dashboards (`deploy/grafana/dashboards/`)

- v1.0 set (12 dashboards as JSON):
  - Control Plane Health
  - Agent Fleet
  - State Management
  - Policy Compliance (audit-mode)
  - GitOps Operations
  - NATS Mesh
  - Audit Log
  - Module System
  - Event System
  - Secrets Management
  - Remote Execution
  - Multi-Environment
- Datasource templating (`env`, `datacenter` variables).
- Documented in `deploy/grafana/README.md` with import instructions.

### Correlation IDs

- Generated at request entry points; flow via context, gRPC metadata, NATS message headers, span attributes, log entries.

## Scope (out / non-goals)

- **`kscore-monitor` TUI** (8 base views; 13 with enhancements) — v1.2.
- **NATS telemetry transport** (logs/metrics/traces over NATS subjects) — v1.4.
- **Telemetry gateway** (`kscore-telemetry-gateway` standalone service) — v1.4.
- HA gateway with queue groups + leader election — v1.4.
- CLI audit logging to syslog/journald — v1.2 (audit infra in v0.1 already).
- Adaptive sampling tied to error metrics — v2.0.
- pprof visualization UI — v2.x.
- SIEM export (CEF/LEEF) — v2.0.
- Real-time alerting from TUI — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.16`.

## Tasks

1. **Custom Prom registry** + `Collector` + `Timer` + `cardinality.Limiter`.
2. **All v1.0 metrics defined** in their owning packages (`internal/agent`, `internal/execution`, `internal/statemgmt`, etc.) — each emit at appropriate points.
3. **`/metrics` HTTP handler** wired in Epic 04 server.
4. **OTel `TracerProvider`** + samplers + exporters + batch processor.
5. **Span helper attribute functions**.
6. **Health checker registry** + concrete checkers (NATS, DB, JetStream, custom).
7. **Profiling endpoint** (opt-in).
8. **12 Grafana dashboards** as JSON in `deploy/grafana/dashboards/`. Use Promtool or equivalent to validate queries against running CP.
9. **`deploy/grafana/README.md`** with import + datasource setup instructions.
10. **Correlation ID middleware** wired in HTTP + gRPC layers; propagated via context + NATS headers.
11. **Integration test**: hit `/metrics`, verify expected metric names; OTel exporter emits spans to local collector; correlation ID flows through a request.

## Acceptance criteria

- [x] `/metrics` exposes all v1.0 metrics in Prometheus exposition format. _(Task 3: `promhttp.HandlerFor(metricsRegistry.Gatherer(), …)` mounted on `publicMux` at `cfg.Metrics.Path` (default `/metrics`); same HTTP listener as `/health/*`, no auth, no new port. Disabled via `metrics.enabled=false`; 404s when Registry not supplied.)_
- [x] Cardinality limiter drops metrics above threshold; logs warning with metric name. _(Task 1: `internal/metrics/cardinality.Limiter` enforces a per-metric cap with Drop (default) or Aggregate (`"_overflow"` sentinel) mode; drops emit `kscore_metrics_cardinality_total{metric,outcome}`; first-drop warn log is throttled per metric.)_
- [x] OTel traces export to OTLP receiver (test with otelcol or stdout exporter). _(Task 4: `internal/tracing.New` builds a `*sdktrace.TracerProvider` with the configured Exporter (stdout / OTLP gRPC / OTLP HTTP / Zipkin), Sampler (`always_on/off` / `probabilistic` / `parent_based` / `rate_limiting` — `adaptive` deferred to v2.x+ ROADMAP), and a `BatchSpanProcessor` honoring `BatchSize` / `QueueSize` / `FlushInterval`. `Provider.Shutdown` flush is verified via a stub exporter test.)_
- [ ] `/health/live` always 200; `/health/ready` returns 503 during startup grace period.
- [ ] `/api/status` returns version, uptime, agent counts, memory, goroutines.
- [ ] All 12 Grafana dashboards import successfully against a running CP and display non-empty panels.
- [ ] Correlation ID present in JSON log lines + gRPC metadata + span attributes for end-to-end requests.
- [ ] pprof endpoint returns valid profile when enabled.
- [ ] Coverage >75% on `internal/metrics`, `internal/tracing`.

## Risks

- **Cardinality explosion** is real — limiter is mandatory; monitor `kscore_metrics_cardinality_total` and alert.
- **Sampling at 100%** adds 5-10% latency at scale. Default to `probabilistic 0.1` with per-span rules upgrading on errors.
- **Health check timeouts** must be tight; circular dependencies (a check that calls into the component being checked) deadlock.
- **Dashboard maintenance** — every metric rename breaks dashboards; CI should detect via metric-name diff against `deploy/grafana/expected_metrics.txt`.

## References

- PROJECT-DETAILS §4.16.
