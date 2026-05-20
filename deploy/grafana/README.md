# Keystone Core — Grafana dashboards

Pre-built Grafana dashboards for the v1.0 Keystone Core metric surface.
Twelve dashboards under `dashboards/`, one per operator domain. Each is
self-contained JSON; import via the Grafana UI or HTTP API.

## v1.0 dashboard set

| File | Title | Domain |
|---|---|---|
| `control-plane-health.json` | Control Plane Health | Quorum, members, HTTP/gRPC latency, runtime |
| `agent-fleet.json` | Agent Fleet | Agent counts by status, by cluster |
| `state-management.json` | State Management | Apply rate, drift detections |
| `policy-compliance.json` | Policy Compliance (audit-mode) | Allow/deny ratios, top policies |
| `gitops-operations.json` | GitOps Operations | Event-derived GitOps activity |
| `nats-mesh.json` | NATS Mesh | Throughput, quorum, failovers, cardinality |
| `audit-log.json` | Audit Log | Entry throughput, deny ratio, top policies |
| `module-system.json` | Module System | System events + module-policy audits |
| `event-system.json` | Event System | Events by type and severity |
| `secrets-management.json` | Secrets Management | Ops by backend / op / result |
| `remote-execution.json` | Remote Execution | Command rate, p50/p99, top agents |
| `multi-environment.json` | Multi-Environment | Cross-env / cross-DC fleet view |

## Import — Grafana UI

1. **Dashboards → New → Import**.
2. Upload the JSON file (or paste its contents).
3. Pick the Prometheus datasource scraping your kscore-server's
   `/metrics` endpoint.
4. Save.

Re-importing the same JSON updates an existing dashboard in place — the
`uid` field on each dashboard (`kscore-<domain>`) is stable across
versions.

## Import — Grafana HTTP API

```bash
for f in deploy/grafana/dashboards/*.json; do
  curl -sf -H "Authorization: Bearer ${GRAFANA_TOKEN}" \
       -H 'Content-Type: application/json' \
       -d "{\"dashboard\": $(cat "$f"), \"overwrite\": true}" \
       "${GRAFANA_URL}/api/dashboards/db" | jq .uid
done
```

## Prometheus scrape config — required for the `env` / `datacenter` filters

The dashboards expose `$env` and `$datacenter` template variables.
Neither is a kscore metric label — operators inject them at scrape
time via Prometheus `external_labels` so the *same* metric series can
be partitioned per deployment.

Example `prometheus.yml` snippet:

```yaml
global:
  external_labels:
    env: production
    datacenter: us-east-1

scrape_configs:
  - job_name: kscore-server
    static_configs:
      - targets: ['kscore-server.internal:8080']
    metrics_path: /metrics
```

With no `external_labels`, the template-var dropdowns are empty — that
is acceptable; the dashboards default to `.+` and render every series.

## Validation

`deploy/grafana/dashboards_test.go` runs as part of `go test ./...`:

- Every dashboard parses as JSON.
- Required schema fields (`title`, `uid`, `schemaVersion`, `panels`,
  `templating`) are present.
- Every `kscore_*` metric a dashboard references is listed in
  `expected_metrics.txt`.
- Every metric defined in `internal/metrics/metricdefs.go` is listed in
  `expected_metrics.txt`.
- Dashboard `uid`s are unique and start with `kscore-`.

Renaming a metric in `metricdefs.go` MUST update `expected_metrics.txt`
in the same commit; otherwise the gate fails.

## Live-import verification

The Go test gates static correctness. To verify dashboards render
non-empty panels against real data:

1. Run `kscore-server` (`make run` or any boot wiring).
2. Run Prometheus pointed at the server's `/metrics`.
3. Run Grafana with the Prometheus datasource configured.
4. Import each dashboard JSON.
5. Confirm panels populate within ~30 s of activity (longer for the
   24h-windowed tables).

This is a manual pre-release check; the Epic 17 task-8 acceptance line
covers static validation in CI.

## v1.x followups

- Per-domain richer metrics (GitOps sync duration, Module load time,
  NATS-server-internal stats) — out of scope for v1.0; the dashboards
  here repurpose `events_emitted_total` filters and audit-entries where
  no dedicated metric exists yet.
- TUI dashboard mode (`kscore-monitor`) — v1.x.
- Telemetry gateway dashboards (Loki logs, traces) — v1.x.

See `docs/project/ROADMAP.md` for the deferral list.
