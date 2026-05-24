# Single-topology E2E harness

Epic 19 task 1/2 — `test/e2e/single/`. The baseline v1.0 E2E topology:
**1× kscore-server + 2× kscore-agent + Postgres + NATS**, plus
per-task scenario coverage on top.

The topology runs in one of two modes (selected via env var by
`TestMain`):

| Mode | Trigger | What it exercises | When to use |
|---|---|---|---|
| **Native** (default) | nothing — it's the default | Host subprocesses for server + agents, embedded NATS, postgres pointed to by `KSCORE_TEST_POSTGRES_DSN`. No docker required. | CI, most local development. |
| **Docker** | `KSCORE_E2E_USE_DOCKER=1` | `docker-compose.yml` + `Dockerfile.kscore` — the production-shaped distroless image stack with every service in its own container. | Locally when you want to validate that the actual container image is correct (entrypoint, distroless, nonroot UID, mounted blueprints volume, etc.). |

Both modes run the same 11 `TestE2E_*` scenarios; the only
mode-dependent values are the command path used by `TestE2E_CommandExec`
(host `/bin/echo` vs. distroless `/usr/local/bin/kscore`) and the
webhook receiver host (`127.0.0.1` vs. `host.docker.internal`).

## Scope by sub-task

Epic 19 task 2 ships in three slices (2a, 2b, 2c). What's landed
here right now:

| Slice | Scenarios | Status |
|-------|-----------|--------|
| **Task 1** | Infrastructure health (server, postgres, nats, schema) | landed |
| **Task 2a** | 1. agent registration · 2. command exec · 3. state apply | landed |
| **Task 2b** | 4. blueprint apply · 5. module stdlib execute · 6. secrets KV round-trip | landed |
| **Task 2c** | 7. audit query · 8. outbound webhook · 9. GitOps webhook+rollback · CI required | landed (this PR) |

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

### Task 2c scenario notes

- **Scenario 7 (AuditLogQuery)** exercises `PolicyService.GetAuditLog`
  + `GetComplianceReport`. Earlier scenarios (registration, command
  exec, blueprint apply, secrets write) populate the audit store
  asynchronously; the test polls until entries land.

- **Scenario 8 (OutboundWebhook)** stands up a host-side
  `httptest.Server` bound on `0.0.0.0`, registers a subscription via
  the kscore-server REST surface (`POST /api/v1/webhooks/subscriptions`),
  fires the synthetic test ping (`POST .../test`), and asserts the
  receiver got the POST. The server reaches the host via
  `host.docker.internal` (Linux: enabled by the compose's
  `extra_hosts: host-gateway` directive).

- **Scenario 9 (GitOpsWebhookIngest)** POSTs an HMAC-signed GitHub
  push payload to the GitOps inbound receiver on `:8081/webhooks`
  with the configured secret. Asserts a 202 acceptance.

- **Scenario 9 (GitOpsRollback)** exercises the rollback engine FSM
  via REST: `POST /api/v1/gitops/rollback` with `require_approval:
  true` → `POST .../reject` → `GET .../{id}` and asserts the final
  state is `Rejected`. Proves the engine + SQLite store + REST
  handler + FSM transitions. **Real Git executor coverage** (clone
  → revert → push against a working git server) is deferred to v1.x
  — requires an in-compose git server (e.g. gitea or alpine/git
  sidecar) which isn't worth the infrastructure for a v1.0 baseline.
  ArgoCD + K8s rollout-undo executors stay gate-v1.0 ROADMAP items
  (ArgoCD needs a real server; K8s pulls `k8s.io/client-go`).

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
                 │   :5397 grpc │
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

- `127.0.0.1:8080` → server HTTP (`/health/*`, `/metrics`, REST APIs)
- `127.0.0.1:5397` → server gRPC
- `127.0.0.1:8081` → server GitOps inbound webhook receiver
- `127.0.0.1:5432` → Postgres
- `127.0.0.1:8222` → NATS monitoring

## Running

From the repo root:

### Native mode (default — CI entry point)

```bash
# Provide a reachable postgres (the kscore-server's tables are dropped
# and recreated at startup). The integration job's service is reused
# for free in CI; locally set the DSN to your own postgres.
export KSCORE_TEST_POSTGRES_DSN="postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"
make e2e-test
```

This builds the `kscore-server` + `kscore-agent` binaries, embeds
`nats-server` in-process, and runs the binaries as host subprocesses.
No docker required.

### Docker mode (production-image coverage)

```bash
make e2e-build         # Build images (first run: 60–90s; cached after)
make e2e-up            # Bring up the topology, wait for healthchecks
make e2e-test-docker   # Full cycle: build, up, run tests, down (cleanup on failure)
make e2e-down          # Tear down + remove volumes
make e2e-logs          # Follow logs from all services
```

Use this when you want to exercise the `Dockerfile.kscore` image —
distroless runtime, nonroot UID, volume mounts, entrypoint, etc.
Not run in CI.

### Direct `go test`

```bash
# Native (needs KSCORE_TEST_POSTGRES_DSN):
CGO_ENABLED=1 go test -tags=e2e -count=1 -timeout=300s ./test/e2e/single/...

# Docker (TestMain manages compose lifecycle):
KSCORE_E2E_USE_DOCKER=1 CGO_ENABLED=1 \
  go test -tags=e2e -count=1 -timeout=300s ./test/e2e/single/...
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
├── scaffold_test.go         # //go:build e2e — TestMain + shared helpers
├── scaffold_native_test.go  # //go:build e2e — native-mode subprocess lifecycle
└── scenarios_test.go        # //go:build e2e — TestE2E_* scenarios
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
