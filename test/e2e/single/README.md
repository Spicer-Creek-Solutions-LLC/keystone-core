# Single-topology E2E harness

Epic 19 task 1 — `test/e2e/single/`. The baseline v1.0 E2E topology:
**1× kscore-server + 2× kscore-agent + Postgres + NATS** via
docker-compose.

## Scope: task 1 vs task 2

Epic 19 splits the v1.0 E2E surface into two tasks:

| Task | Scope |
|------|-------|
| **Task 1 (this file)** | Topology, build pipeline, harness validation — *infrastructure* only. |
| **Task 2** | Wires the **9 feature scenarios** (agent registration, command exec, state apply, blueprint apply, module install/exec, secrets, audit, outbound webhook, GitOps webhook+rollback) on top of this topology. |

This task 1 harness deliberately does **not** assert agent
registration, command dispatch, or any user-visible behavior. Those
are scenario 1+ of task 2.

What task 1 *does* assert (see `harness_test.go`):

1. `kscore-server` HTTP `/health/live` returns 200 (process alive).
2. `kscore-server` HTTP `/health/ready` returns 200 within budget
   (NATS + Postgres reachable from inside the server container).
3. NATS monitoring `/healthz` returns 200 (broker is healthy).
4. Postgres has `state.applySchema` results — the `agents` table
   exists — proving the server's `state.NewStore` succeeded.

If those four pass, the harness is sound and task 2 can build
scenarios on top.

## Topology

```
                 ┌──────────────┐
                 │   postgres   │  16-alpine, kscore/kscore@kscore
                 └──────┬───────┘
                        │
                 ┌──────▼───────┐
                 │ kscore-server│  --config /etc/kscore/server.yaml
                 │   :8080 http │
                 │   :9090 grpc │
                 └──────┬───────┘
                        │ (NATS subjects)
                 ┌──────▼───────┐
                 │     nats     │  2.10-alpine, JetStream on
                 │   :4222 nats │
                 │   :8222 mon  │
                 └──┬───────┬───┘
                    │       │
        ┌───────────▼─┐   ┌─▼────────────┐
        │  agent-1    │   │  agent-2     │   --config /etc/kscore/agent.yaml
        └─────────────┘   └──────────────┘
```

External port bindings (loopback only):

- `127.0.0.1:8080` → server HTTP (`/health/*`, `/metrics`)
- `127.0.0.1:9090` → server gRPC
- `127.0.0.1:5432` → Postgres
- `127.0.0.1:8222` → NATS monitoring

## Running

From the repo root:

```bash
make e2e-build    # Build images (first run: 60–90s; cached after)
make e2e-up       # Bring up the topology, wait for healthchecks
make e2e-test     # Full cycle: build, up, run smoke test, down (cleanup on failure)
make e2e-down     # Tear down + remove volumes
make e2e-logs     # Follow logs from all services
```

`make e2e-test` is the CI entry point. It sets
`KSCORE_E2E_NO_COMPOSE=1` so the Go test doesn't double-manage
compose lifecycle.

The Go test can also be run *without* the make wrapper — it will
manage compose itself if `KSCORE_E2E_NO_COMPOSE` is unset:

```bash
CGO_ENABLED=1 go test -tags=e2e -count=1 -timeout=300s ./test/e2e/single/...
```

## Files

```
test/e2e/single/
├── README.md             # this file
├── docker-compose.yml    # the 5-service topology
├── Dockerfile.kscore     # multi-stage; BIN build-arg selects server|agent
├── config/
│   ├── server.yaml       # kscore-server config (postgres + external nats)
│   ├── agent-1.yaml      # kscore-agent config (id=agent-1)
│   └── agent-2.yaml      # kscore-agent config (id=agent-2)
└── harness_test.go       # //go:build e2e — task 1 smoke test
```

## Build-tag

The test file is gated by `//go:build e2e`. It will not compile
into `make test`, `make test-integration`, or `make slo`. The only
way to run it is `make e2e-test` or an explicit `-tags=e2e`
invocation.

This matches the gating used by `test/e2e/state/` (`run.sh` /
docker-compose only) and `test/e2e/ha/` (`-tags=integration` and
`-tags=slo`).

## Design notes

- **No identity / TLS / bootstrap**: task 1 disables `identity` and
  `nats.bootstrap` so the harness can prove infrastructure
  reachability without dragging in mTLS material. Task 2 will add a
  PSK or join-token mode so agents actually register.
- **Distroless runtime, nonroot UID**: the kscore image uses
  `gcr.io/distroless/static-debian12:nonroot`. The Dockerfile
  materializes `/data` owned by UID 65532 in the builder stage so
  the agent can write its SQLite file at runtime — distroless has
  no shell to `mkdir` after the fact.
- **No docker healthcheck on kscore-server**: distroless ships no
  curl/wget/sh, so docker-level healthchecks have nothing to probe
  with. The Go harness test polls `http://127.0.0.1:8080/health/ready`
  through the published port instead.
- **External NATS**: agents require `nats.mode: external` per
  `cmd/kscore-agent/main.go:67`. The server matches.
- **Postgres schema auto-applies**: `state.NewStore` with the
  postgres backend invokes `applySchema` (see
  `internal/state/postgres.go:52`) — no separate migrate step is
  needed.

## Limits / known shortcomings

- No CI wiring yet. `.github/workflows/` integration is task 2's
  responsibility (acceptance criterion: "All v1.0 E2E scenarios pass
  in CI.").
- No performance assertions. SLO scripts land in
  `test/e2e/perf/` per task 3.
- No goleak integration. Task 6.
- No backup/restore round-trip. That's in epic 19's acceptance
  criteria but is delivered by a different domain.
