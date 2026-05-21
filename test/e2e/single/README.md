# Single-topology E2E harness

Epic 19 task 1/2 — `test/e2e/single/`. The baseline v1.0 E2E topology:
**1× kscore-server + 2× kscore-agent + Postgres + NATS** via
docker-compose, plus per-task scenario coverage on top.

## Scope by sub-task

Epic 19 task 2 ships in three slices (2a, 2b, 2c). What's landed
here right now:

| Slice | Scenarios | Status |
|-------|-----------|--------|
| **Task 1** | Infrastructure health (server, postgres, nats, schema) | landed |
| **Task 2a** | 1. agent registration · 2. command exec · 3. state apply | landed |
| **Task 2b** | 4. blueprint apply · 5. module stdlib execute · 6. secrets KV round-trip | landed (this PR) |
| **Task 2c** | 7. audit query · 8. outbound webhook · 9. GitOps webhook+rollback · CI required | pending |

### Task 2b scenario notes

- **Scenario 4 (BlueprintApply)** exercises the BlueprintService gRPC
  surface (`ListBlueprints` + `ApplyBlueprint`) against a
  **distroless-compatible test catalog** at `blueprints/e2e-noop/`.
  The production `modules/examples/blueprints/demo` blueprint installs
  packages + manages services and cannot run inside the nonroot
  distroless kscore-server container. Server-side apply only — remote-
  agent dispatch is the gate-v1.0 ROADMAP item *"Remote / distributed
  blueprint apply wiring"*.

- **Scenario 5 (ModuleStdlibExecution)** is the *Option A* fallback
  per epic-19 task-2b. It uses a multi-stdlib state YAML
  (file + cmd with a requires dependency) over `ApplyState` to prove
  the loader → runner → resolver chain. **Target form**: the full
  *Option B* registry path — stand up `kscore-registry`, publish a
  signed Starlark module, `kscore-module install`, then ApplyState
  against the installed module. **Blocked on**: the gate-v1.0
  ROADMAP item *"Module system boot wiring (loader PolicyChecker /
  Hosts / trust-policy + runtime registration)"* — until that lands,
  `cmd/kscore-{server,agent}` cannot load + execute a published
  Starlark module end-to-end. Once it does, scenario 5 graduates
  to Option B and this note can be removed.

- **Scenario 6 (SecretsRoundTrip)** covers KV operations only
  (`WriteSecret` → `GetSecret` → `ListSecrets` → `DeleteSecret`)
  against the encrypted-file backend. **Deferred**: lease lifecycle
  + transit (Encrypt/Decrypt/Sign/Verify) — both are Vault-only in
  v1.0 (`internal/secrets/file/backend.go:370+` and
  `internal/secrets/transit.go:13`). A Vault-backed compose service
  + scenario extension lands in v1.x when the Vault-backed deployment
  story is exercised.

What the harness asserts at infrastructure level (Task 1):

1. `kscore-server` HTTP `/health/live` returns 200 (process alive).
2. `kscore-server` HTTP `/health/ready` returns 200 within budget
   (NATS + Postgres reachable from inside the server container).
3. NATS monitoring `/healthz` returns 200 (broker is healthy).
4. Postgres has `state.applySchema` results — the `agents` table
   exists — proving the server's `state.NewStore` succeeded.

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
│   ├── server.yaml       # kscore-server config (postgres + external nats + secrets + blueprints)
│   ├── agent-1.yaml      # kscore-agent config (id=agent-1, PSK bootstrap)
│   └── agent-2.yaml      # kscore-agent config (id=agent-2, PSK bootstrap)
├── blueprints/
│   └── e2e-noop/         # distroless-compatible blueprint fixture (task 2b)
├── scaffold_test.go      # //go:build e2e — TestMain + helpers
└── scenarios_test.go     # //go:build e2e — TestE2E_* scenarios
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
