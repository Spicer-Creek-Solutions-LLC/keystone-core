# PROJECT-DETAILS.md

Implementation reconstruction guide for Keystone Core. Goal: enable an LLM with this document plus FEATURES.md plus the `epics/` series to rebuild the project from scratch on the same Go stack.

> **Scope**. This document captures *what to build and how to structure it* — types, interfaces, protocols, patterns, and rationale. It does **not** reproduce the source code; the existing repo is the canonical reference for syntax-level details.

> **Convention**. v1.0 = the commercial-trial-ready MVP (clusterable, sysadmin-friendly). Later versions (v1.x → v2.x → v3+) are tracked in §6.

---

## Table of Contents

1. [Vision & Positioning](#1-vision--positioning)
2. [High-Level Architecture](#2-high-level-architecture)
3. [Tech Stack & Build System](#3-tech-stack--build-system)
4. [Domain-by-Domain Implementation](#4-domain-by-domain-implementation)
   - 4.1 [Foundations](#41-foundations)
   - 4.2 [NATS Messaging](#42-nats-messaging)
   - 4.3 [Storage Layer](#43-storage-layer)
   - 4.4 [Control Plane Core](#44-control-plane-core)
   - 4.5 [API Surface](#45-api-surface)
   - 4.6+ *(Agent runtime → Specialized; populated as briefs land)*
5. [Cross-Cutting Concerns](#5-cross-cutting-concerns) *(Phase 3)*
6. [Versioning Strategy & MVP Rationale](#6-versioning-strategy--mvp-rationale) *(Phase 4)*
7. [Reconstruction Order](#7-reconstruction-order) *(Phase 5)*

---

## 1. Vision & Positioning

**Tagline**: *GitOps deploys it. We keep it running.*

Keystone Core is the **runtime operations control plane** between deployment tooling (GitOps/IaC) and day-2 operations. It complements ArgoCD/Flux/Terraform — it does **not** replace them. Where they answer "what should be deployed", Keystone Core answers "what is happening right now, is it drifting, and what should we do about it?"

**Target users (v1.0)**: sysadmins and IT admins running heterogeneous infrastructure (Linux hosts, K8s clusters, mixed cloud). The v1.0 bar is "feature-rich enough that a sysadmin can run a small production stack on it for a real client."

**Anti-positioning**:
- Not a config management tool that scales to 10 nodes (we target thousands).
- Not a Salt/Ansible clone (we are agent-pull-with-push-cap, GitOps-native, real-time-event-driven).
- Not a Kubernetes-only operator (multi-environment is core).

**Competitive frame**:
- vs Ansible: 10× faster at scale; real-time vs sequential; event-driven.
- vs Salt: modern Go single-binary; cloud-native first; clear OSS license.
- vs K8s operators alone: works across infrastructure types, not just K8s.

---

## 2. High-Level Architecture

```
┌─────────────────────────── Control Plane ───────────────────────────┐
│  kscore-server                                                        │
│  ├─ gRPC server (:9090)        REST gateway (:8080)                  │
│  ├─ ConnectionMgr  ───── tracks agents, heartbeats                    │
│  ├─ CommandDispatcher  ── routes commands, retains results           │
│  ├─ BatchDispatcher  ──── group ops with batch state machine         │
│  ├─ StateService  ───── apply/check/drift                            │
│  ├─ EventService  ───── pub/sub + persistence                        │
│  ├─ PolicyService  ──── audit-mode evaluation (v1.0)                 │
│  ├─ SecretsService  ─── CRUD + leases + transit                      │
│  ├─ ClusterService  ─── etcd-backed membership + leader election     │
│  └─ CoordinationSvc ─── server↔server mTLS (NATS-fallback recovery)  │
└──────────────┬──────────────────────────────┬───────────────────────┘
               │                              │
               ▼                              ▼
        ┌──────────────┐             ┌────────────────────┐
        │     NATS     │             │  SQLite / Postgres │
        │  (embedded   │             │  (state, events,   │
        │   external,  │             │   commands,        │
        │   leaf v2)   │             │   secrets, etc.)   │
        └──────┬───────┘             └────────────────────┘
               │
   ┌───────────┼───────────────┬───────────────┐
   ▼           ▼               ▼               ▼
┌─────────┐ ┌─────────┐  ┌─────────────┐  ┌──────────────┐
│Agent K8s│ │Agent VM │  │Agent Edge   │  │Proxy Agent   │
│         │ │         │  │(leaf v2)    │  │(devices, v2) │
└─────────┘ └─────────┘  └─────────────┘  └──────────────┘
```

**Subject hierarchy** (NATS, even single-cluster v1):
```
kscore.{cluster}.agent.register
kscore.{cluster}.agent.heartbeat
kscore.{cluster}.agent.{id}.command|response|state|events
kscore.{cluster}.server.announce|control
kscore.{cluster}.bootstrap.{id}.register|response
kscore.{cluster}.discovery
```

**Storage of record**: SQLite (single-server) or PostgreSQL (cluster). Events are mirrored: NATS JetStream for realtime delivery, SQL for query.

**Key architectural principle**: *Start simple, scale up by configuration.* A single binary with embedded NATS + SQLite + a generated dev API key must work on first run with `./kscore-server`. The same binary runs in a 3-node HA cluster with external NATS + Postgres + mTLS by config change alone.

---

## 3. Tech Stack & Build System

### 3.1 Languages & Toolchain

- **Go 1.25+** (single language for all server and agent code).
- **CGO_ENABLED=0** for all builds. Pure-Go SQLite (`modernc.org/sqlite`); no `mattn/go-sqlite3`.
- **Buf** for proto management (`buf.yaml` STANDARD lint rules; `buf.gen.yaml` with `buf.build/protocolbuffers/go` and `buf.build/grpc/go` plugins).

### 3.2 Critical Dependencies (direct)

| Concern | Module | Notes |
|---|---|---|
| SemVer | `github.com/Masterminds/semver/v3` | Wrapped by `pkg/semver` facade; provides parsing, comparisons, and constraint grammar. |
| Messaging | `github.com/nats-io/nats.go`, `github.com/nats-io/nats-server/v2` | Both client and embedded server. NATS 2.10+ for leaf/gateway later. |
| gRPC | `google.golang.org/grpc`, `google.golang.org/protobuf` | TLS, streaming. |
| CLI | `spf13/cobra` | Subcommand CLI for all binaries. |
| Config | `knadh/koanf/v2` (+ parsers/yaml, providers/{file,env,structs}) | YAML + KSCORE_-env loader. Strict unmarshal. Chosen over Viper in epic 01 task 7 review for cleaner semantics and lighter deps; Cobra→koanf flag bridge added in task 13. |
| Logging | `log/slog` (stdlib) | Structured. JSON, logfmt, and "text" (compact-time TextHandler) formats. Correlation IDs injected from context via a wrapping `slog.Handler`. |
| Tracing | `go.opentelemetry.io/otel/*` | OTLP/Zipkin/stdout exporters. |
| Metrics | `prometheus/client_golang` | Standard. |
| Storage | `modernc.org/sqlite`, `lib/pq` | Pure-Go drivers. |
| Cluster | `go.etcd.io/etcd/{client/v3,server/v3,api/v3}` (v3.6.11) | Embedded etcd for HA. Added Epic 13 task 1: `internal/cluster.EtcdClient` wraps etcd v3 in **embedded** (in-process server via `server/v3/embed`, client wired straight to it with `etcdserver/api/v3client` — no network hop) or **external** mode; owns lifecycle + lease (grant/keepalive/revoke) + thin KV/Watch/Txn passthrough; `Client()` exposes `*clientv3.Client` so the Task 3 LeaderElector layers `concurrency.Election` without re-dialing. Only credible embeddable strongly-consistent (Raft quorum) coordination backend in Go — gossip (serf/memberlist) can't satisfy the split-brain/fencing acceptance criteria. Clustering is opt-in (`cluster.enabled: false` default). |
| GitOps | `go-git/go-git/v5`, `argoproj/argo-cd/v3` | Webhook + reconcile. |
| Policy | `open-policy-agent/opa` (v1.16.2), `google/cel-go` | Dual engine, audit-mode v1. OPA added Epic 12 task 6: embedded `opa/v1/rego` SDK (in-process; no subprocess/sidecar — only credible embeddable Rego engine in Go). Rego v1 syntax; fixed package `keystone.policy` (query `data.keystone.policy.{allow,violations,warnings}`); restricted capability set denies `http.send`/`net.*`/`opa.runtime` (operator-supplied policies must be pure decision logic — SSRF/exfil guard); compiled queries cached by `policyID+sha256(Code)`. |
| WASM | `tetratelabs/wazero` | Pure-Go WASM runtime. |
| K8s | `k8s.io/client-go`, `apimachinery`, `api` | For operator (post-v1.0) and K8s exec. |
| Cloud | AWS SDK v2, GCP, Azure SDKs | Initially used by secrets+identity; broader v2.x+. |
| TUI | `charmbracelet/bubbletea`, `lipgloss` | Monitor binary (post-v1.0). |
| Targeting | `github.com/expr-lang/expr`, `github.com/gobwas/glob` | Compiled-VM expressions for `--target` selectors with a `match()` glob function. Chosen in Epic 07 task 1 over CEL (heavy, proto-centric) and a custom RD parser. |

### 3.3 Build Outputs

- `build/bin/$GOOS/$GOARCH/kscore-{name}` for native and cross-compile.
- `dist/` for goreleaser snapshot (multi-arch tarballs).
- `build/dev/` for hot-reload (air).

### 3.4 Make Targets (v1.0 set)

| Group | Targets |
|---|---|
| Build | `proto`, `build`, `build-all-platforms`, `clean`, `deps`, `install-tools` |
| Test | `test`, `test-verbose`, `test-coverage`, `test-integration`, `check` |
| Lint | `fmt`, `lint`, `lint-fix`, `proto-lint`, `proto-breaking` |
| Dev | `dev` (configurable via `DEV_BIN`), per-binary `dev-server`, `dev-agent` |
| E2E (v1.0 minimal) | `e2e-build`, `e2e-test`, `e2e-up`, `e2e-down`, `e2e-logs` (single-topology only) |
| Security (v1.0 minimal) | `security-secrets` (gitleaks), `security-vulns` (govulncheck), `security-sast` (gosec) |
| Release | `release-snapshot`, `release-dry-run` |

### 3.5 Linting Baseline (.golangci.yml)

Enabled: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `bodyclose`, `gosec`. Defer to post-v1.0: `revive`, `exhaustive`, `gocritic`, `contextcheck`, `durationcheck`, `rowserrcheck`, `sqlclosecheck`. Test files exempt from `gosec`/`gocritic`/`errcheck`. `.pb.go` linters all disabled.

### 3.6 Project Layout (target for rebuild)

```
keystone-core/
├── api/proto/                # 8 .proto files (agent, controlplane, state, event, policy, secrets, cluster, coordination)
├── api/openapi/              # hand-maintained openapi-spec.yaml (v1.0); generated v2.x+
├── pkg/api/                  # gRPC stubs (v1) + per-domain REST handlers + clients + auth/rbac/apierror/versioning
├── pkg/version/              # build-time version info
├── pkg/semver/               # semver parsing, constraints, diff
├── pkg/wait/                 # cancelable wait/poll
├── pkg/dbutil/               # SQLite WAL setup
├── pkg/saga/                 # saga coordinator (v1.0 minimal; post-v1.0 advanced)
├── pkg/statemachine/         # generic FSM library
├── pkg/secrets/              # secrets client lib
├── pkg/policy/               # policy client lib
├── pkg/module/               # plugin/module system (v1.0 — see §4.x)
├── pkg/plugin/               # plugin discovery + executor
├── cmd/                      # binaries — v1.0 set: kscore-server, kscore-agent, kscorectl, kscore-{exec,state,agents,events,policy,audit,gitops,webhook,cluster,blueprint,runbook,secrets,module,backup,migrate,monitor,bootstrap}
├── internal/                 # private packages — see §4
├── modules/                  # plugin SDKs + stdlib + examples
├── deploy/                   # Helm + K8s manifests + Grafana dashboards
├── docs/                     # Hugo + Docsy (post-v1.0)
├── test/e2e/                 # docker-compose topologies
├── epics/                # this rebuild's planning docs
├── Makefile / .goreleaser.yaml / buf.{yaml,gen.yaml} / .golangci.yml / .pre-commit-config.yaml
└── FEATURES.md / PROJECT-DETAILS.md / README.md / RELEASE-PLAYBOOK.md
```

---

## 4. Domain-by-Domain Implementation

### 4.1 Foundations

**Purpose**: Establish the build, config, logging, error, version, and time/wait primitives that every other domain depends on.

**Key types & responsibilities**:

- `pkg/version.Info{Version, GitCommit, BuildDate}` — populated via `-ldflags -X` at build time.
- `pkg/semver.Version` — full SemVer 2.0.0 parsing; `Parse`, `MustParse`, `NextMajor/Minor/Patch`, comparisons. Implemented as a thin facade over `github.com/Masterminds/semver/v3` so callers don't import Masterminds directly.
- `pkg/semver.Constraint` (interface) — caret, tilde, wildcard, compound, OR; used by module resolver. Backed by Masterminds' `Constraints`.
- `pkg/semver.Diff{Kind, Direction}` — `DiffSame/Patch/Minor/Major/Prerelease` × `DirectionSame/Upgrade/Downgrade`, with `IsBreaking()`/`IsFeature()`/`IsBugFix()` predicates. Drives plugin compatibility checks. Project-specific (no Masterminds equivalent).
- `pkg/wait.ForCondition(ctx, interval, fn)` — cancellable polling; replaces `for { time.Sleep() }` patterns.
- `pkg/dbutil.OpenSQLite(path, opts...)` — WAL mode, busy-timeout, FK on, single writer.
- `pkg/api/apierror.Response{Error, Message, Details map}` — standard JSON error body; `StatusCode()` maps codes to HTTP.
- `internal/config.Config` — root struct loaded via koanf-based `Load(path)` from YAML + env (`KSCORE_` prefix). Foundations ships 3 sub-configs (`Server`, `Logging`, `Storage`) plus a top-level `Mode`; later epics extend `Config` with their own sub-config struct + `Validate()` + production-warning entries. Single-word koanf keys (e.g., `grpcport`, `httpport`, `certfile`) keep env-var mapping unambiguous with a single-underscore separator (`KSCORE_SERVER_GRPCPORT`).
- `internal/logging` — `slog`-backed logger via `New(Options{Level, Format, Output})` returning `*slog.Logger`. Three formats (json, logfmt, text — last is `TextHandler` with RFC3339 timestamps for terminals). Correlation IDs via `WithCorrelationID(ctx, id)` are auto-injected by a wrapping `slog.Handler` on every record whose ctx carries one. v1.0 outputs to stdout only (syslog post-v1.0).

**Config validation rule**: validate **after** unmarshal. Each sub-config implements `Validate() error`; the root `Config.Validate()` calls them all. `Config.ProductionWarnings() []string` reports risky combinations (TLS disabled in production, SQLite in production, embedded NATS in production once Epic 05 lands).

**Error model rule**: REST → `apierror.Response` JSON; gRPC → `status.Error(codes.X, msg)`. The two must map (`apierror.StatusCode()` provides translation).

**Build-time metadata**:
```
-X go.keystone-core.io/keystone-core/pkg/version.Version=$(VERSION)
-X go.keystone-core.io/keystone-core/pkg/version.GitCommit=$(git rev-parse --short HEAD)
-X go.keystone-core.io/keystone-core/pkg/version.BuildDate=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
```

**Gotchas**:
- Add a binary → drop the directory under `cmd/`. The Makefile auto-detects it (`BINARIES := $(notdir $(wildcard cmd/*))`). `.goreleaser.yaml` (Task 15) mirrors the same convention.
- koanf env-var mapping is case-insensitive (we lowercase before lookup) but our convention is uppercase: `KSCORE_LOGGING_LEVEL`.
- Duration parsing is strict: `5m`, not `5min`.

### 4.2 NATS Messaging

**Purpose**: All control-plane↔agent and intra-cluster server↔server data plane traffic. Embedded for trial / single-server, external for HA cluster.

**Modes (v1.0)**:
- `embedded`: `nats-server/v2` runs in-process. Default for `kscore-server` zero-config startup. JetStream on; storage in `./data/jetstream/`.
- `external`: connect to a NATS cluster. Required when running cluster mode (>1 control-plane node).

**Subject convention**: every subject prefixed with `kscore.{cluster}.` from day one. Cluster name defaults to `default` for trial. This is the *non-negotiable* design rule that lets v2 supercluster slide in without refactoring all subscriptions.

**Connection layer**:
- `Manager` — owns the connection (or embedded server) lifecycle; `Start()`, `Shutdown()`, `Health()`.
- `ConnectionManager` — manages multiple endpoints; selects by health, failover, circuit-breaker.
- `ConnectionStrategy` (interface) — `Direct`, `TLS` for v1.0; `WebSocket`, `LeafNode` deferred.
- `StrategySelector` — picks strategy from URL scheme + endpoint capability.
- `Endpoint{URL, Scheme, Auth, Priority, Weight, Tags}`, `EndpointState{state, latency, failure_count, circuit_status}`.

**Reliability**:
- `Envelope{MessageID, CorrelationID, Priority, TTL, ClusterPrefix}` — wraps every message.
- `DedupConfig{WindowDuration, MaxEntries, PerSubjectOverrides}` — SHA256-based dedup.
- `CircuitBreakerConfig{FailureThreshold, SuccessThreshold, OpenDuration, HalfOpenMaxAttempts}` — closed→open→half-open→closed.
- Delivery modes: `at-most-once`, `at-least-once` (JetStream); `exactly-once` deferred to v2.x.

**Bootstrap registration flow** (security baseline):
1. Agent ships with bootstrap credential (PSK or one-time token, short TTL — default 5 min).
2. Agent connects to NATS with permissions limited to `kscore.{cluster}.bootstrap.{id}.register|response`.
3. Agent publishes registration request; server validates identity proof.
4. Server publishes full agent credentials on bootstrap response subject.
5. Agent reconnects with full credentials (agent-specific subjects).
6. Bootstrap credential expires.

**Health/readiness**: `Manager.Health()` checks connection state + embedded server status. Wired to `/health/ready`.

**Config keys** (v1.0): `nats.mode`, `nats.url[s]`, `nats.token`, `nats.credential`, `nats.max_reconnects`, `nats.reconnect_wait`, `nats.jetstream.{enabled,store_dir,max_storage}`, `nats.embedded.{port,max_connections,enable_jetstream,max_memory}`.

**Gotchas**:
- Connection leaks: subscriptions not unsubscribed on disconnect leak memory. Wrap subscriptions in connection-scoped manager.
- Bootstrap credential exchange is **not atomic** at NATS level (agent briefly has old creds during switch). Live with it; document.
- Dedup window must exceed max network RTT; too short = false duplicates.
- IPv6 address formatting: `[::]:4222`, not `:4222` or `::4222`.

**v1.0 build & test**: embedded NATS for unit tests (no testcontainers). Cluster tests use a real external NATS in docker-compose E2E.

### 4.3 Storage Layer

**Purpose**: Persistent state of record. Agents, commands, batch jobs, events, secrets metadata, audit log, policy violations, cluster snapshot.

**Backends (v1.0)**: SQLite (single-server, dev) and PostgreSQL (cluster, production). Both implement the same `Store` interface composed from sub-interfaces.

**Repository design**:
```
type Store interface {
    AgentStore
    CommandStore
    BatchJobStore
    HealthStore
    // additional sub-interfaces added by other domains:
    EventStore        // §4.x
    SecretsStore      // §4.x
    AuditStore        // §4.x
    PolicyStore       // §4.x (audit-mode v1.0)
    ClusterStore      // §4.x
    Close() error
    Ping(ctx) error
}
```

**Backend implementations**: `SQLiteStore`, `PostgreSQLStore`. Direct parametrized SQL — no ORM (intentional). Sort columns are allowlisted (SQL injection guard).

**Connection pool defaults**:
- SQLite: `MaxOpenConns=1`, `MaxIdleConns=1`, WAL mode on, busy-timeout 5s.
- PostgreSQL: `MaxOpen=25`, `MaxIdle=5`, `ConnMaxLife=30m` (configurable).

**Schema (v1.0 — auto-DDL)**:
- `agents(id, hostname, os, architecture, ip_addresses jsonb, platform_version, agent_version, labels jsonb, status, registered_at, last_heartbeat_at, metrics jsonb)`
- `commands(id, agent_id FK, command, args jsonb, env jsonb, working_dir, user, timeout_seconds, status, exit_code, stdout, stderr, started_at, completed_at)`
- `batch_jobs(id, target jsonb, command, args jsonb, status, concurrency, total_agents, completed_agents, successful_agents, failed_agents, created_at, started_at, completed_at)`
- `batch_agent_results(batch_job_id, agent_id, success, exit_code, error, started_at, completed_at, PRIMARY KEY(batch_job_id, agent_id))`
- *(events, secrets_metadata, audit, policies, violations, cluster_snapshots — added by their domains, same auto-DDL pattern.)*

**Timestamps**: SQLite stores TEXT (RFC3339); Postgres stores `TIMESTAMPTZ`. Conversion handled in `*Record` structs via `time.Time`.

**JSON columns**: SQLite TEXT; Postgres JSONB. Don't silently swallow `json.Unmarshal` errors — log and surface (this is a real bug in the existing code; fix in rebuild).

**Migration tooling (`cmd/kscore-migrate`)**: SQLite → PostgreSQL with `Migrator{Migrate, ValidateMigration}`, `MigrationOptions{DryRun, BatchSize, ContinueOnError, SkipExisting, ProgressCallback}`, `MigrationStats`, `TransactionLog` (audit trail of migration ops with checkpoints), `ProgressReporter` (per-table rate + ETA).

**Migration order**: agents → commands → batch_jobs → batch_agent_results (FK order). Use INSERT ... ON CONFLICT DO NOTHING when `SkipExisting`.

**Schema versioning**: deferred to post-v1.0. Auto-DDL is fine for v1.0 since the schema is new and stable. Add `golang-migrate` when first breaking schema change is needed.

**Encryption at rest**: `KeyProvider` interface scaffolding may exist in v0.1 but real implementation deferred to post-v1.0 (with cloud KMS support gating on §4.11 secrets v2 work).

**Gotchas**:
- Don't swallow `json.Unmarshal` errors silently (fix in rebuild).
- IPv6 in Postgres DSN: brackets only for IPv6 literals, not hostnames or v4. Provide `BuildDSN()` helper.
- SQLite's single-writer constraint is real — even with WAL.
- Test harness: temp SQLite files in `t.TempDir()`, not shared dirs.

### 4.4 Control Plane Core

**Purpose**: The `kscore-server` binary. Wires everything together with a strict, deterministic startup sequence and a clean shutdown sequence.

**Startup sequence** (literal order — deviating breaks invariants):
1. Parse + validate config (YAML + env via koanf).
2. Initialize logger (level, format).
3. Start NATS (embedded or external connect).
4. Verify NATS + JetStream availability.
5. Initialize Store (SQLite or Postgres) and run `Ping`.
6. Construct ConnectionManager; start heartbeat-monitor loop (10s interval, 3-missed → stale).
7. Construct CommandDispatcher; start result-retention loop.
8. Construct BatchDispatcher (no separate goroutine; lazy).
9. Build TLS config if enabled.
10. Build auth interceptor + middleware chain (CORS → rate-limit → auth → handler).
11. Construct gRPC server with chained interceptors.
12. Register gRPC services (nil guards for optional ones).
13. Bind gRPC listeners (dual-stack IPv4 + IPv6).
14. Start one goroutine per gRPC listener.
15. Build HTTP mux; register health endpoints.
16. Register REST handlers (conditional on config — e.g., cluster-related only if cluster enabled).
17. Wrap HTTP with middleware (CORS outermost).
18. Bind HTTP listeners; start one HTTP server per listener.
19. Optional: start policy engine, webhook receiver, profiling, tracing exporters.
20. Log startup banner (versions, ports, auth mode, warnings).
21. Enter status-ticker loop (30s); wait for SIGTERM/SIGINT.

**Shutdown sequence** (reverse-of-init, with timeouts):
1. Receive signal; log shutdown begin.
2. `grpcServer.GracefulStop()` (waits for in-flight RPCs).
3. `connMgr.Stop()` (no new commands).
4. `store.Close()` (flush pending writes).
5. `natsManager.Shutdown()` (close client, stop embedded server).
6. `httpServer.Shutdown(ctx 30s)` per listener.
7. Cleanup tracing/profiling (5s timeouts).
8. Log shutdown complete; exit 0.

**Listener creation**: dual-stack helper builds both IPv4 and IPv6 listeners on configured ports. `[::]:8080` is correct; bare `:8080` is wrong for IPv6. Helper handles bracketing.

**Middleware order (gRPC and HTTP)**:
```
CORS  →  Rate-Limit  →  Auth  →  Handler
```
- CORS outermost so OPTIONS preflight bypasses rate-limit.
- Rate-limit per IP, per API key, or per arbitrary header (configurable key-extractor).
- Auth innermost so audit log includes auth result.

**Optional components (gated by config)**:
- Cluster mode (etcd) — gated by `cluster.enabled`. If enabled but etcd init fails, refuse to start (fail fast, do not silently disable). v1.0: cluster is opt-in but supported.
- Policy engine — gated by `policy.enabled`. v1.0: audit mode only.
- Webhook receiver (port 8081) — gated by `webhook.enabled`.
- Profiling (port 6060) — gated by `profiling.enabled`. Default off.
- Tracing exporters — gated by `tracing.enabled`.

**Adapters to avoid cyclic imports**: `stateStoreAdapter`, `shardManagerAdapter`. The pattern: internal services define interfaces; the API server implements/wraps them. Server depends on internal; internal does not depend on server.

**Production warnings on startup** (visible in logs and via `Config.ProductionWarnings()`):
1. Embedded NATS while `mode=production` and >100 agents expected.
2. SQLite while `mode=production`.
3. TLS disabled while talking to external NATS / Postgres.
4. JetStream at default storage limits.

**Health endpoints**:
- `/health/live` — trivial 200.
- `/health/ready` — NATS healthy AND DB ping OK; respects `health.startup_grace_period` (default 30s).
- `/health/status` — component latencies + agent pool ratio.
- `/api/status` — uptime, version, agent counts, memory, goroutines, ClusterInfo if cluster mode.

**Gotchas**:
- Service init order is critical. ConnectionManager must exist before gRPC services that reference it. State store must exist before any service that reads agents in HA mode.
- Listener leaks on partial-init failure — defer Close() in setup function.
- Nil-dependency panics — `if cluster != nil` guards before service registration.
- Unmarshal does not validate — call `Config.Validate()` after.

### 4.5 API Surface

**Purpose**: A single source of truth (proto) for all server↔client and client↔agent contracts, with a compatible REST surface for ad-hoc tooling, scripts, and dashboards.

**Proto layout** (`api/proto/`, all in `package keystone.core.v1`, Go package `pkg/api/v1`):
- `agent.proto` — `AgentService`: Register, Heartbeat, ExecuteCommand (stream), GetAgentInfo.
- `controlplane.proto` — `ControlPlaneService`: ServerStatus, ListAgents, GetAgent, ExecuteCommand (stream), BatchExecuteCommand (stream), command-status/history.
- `state.proto` — `StateService`: ApplyState (stream), CheckState, DetectDrift, GetStateHistory, GetStateStatus.
- `event.proto` — `EventService`: ListEvents, GetEvent, EmitEvent, SubscribeEvents (stream), GetEventTypes, GetEventStats.
- `policy.proto` — `PolicyService`: EvaluatePolicy, EvaluatePolicySet, CRUD, ListViolations, GetComplianceReport, GetAuditLog.
- `secrets.proto` — `SecretsService`: GetSecret, ListSecrets, WriteSecret, DeleteSecret, lease ops, transit (Encrypt/Decrypt/Sign/Verify).
- `cluster.proto` — `ClusterService`: GetClusterStatus, ListMembers, CRUD, GetLeader, TransferLeader, Rebalance, Backup/Restore, WatchMembership (stream), WatchLeadership (stream).
- `coordination.proto` — `CoordinationService` (mTLS-only, server↔server): ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, Heartbeat, PropagateState.

**Rebuild rule**: services are stable in v1; minor additions OK. Breaking changes require a v2 package and a deprecation entry in `pkg/api/versioning`.

**Auth model**:
- API key (`Authorization: Bearer <key>`) — primary for v1.0; first-run generates a dev key with a loud warning.
- mTLS — supported in v1.0 (CoordinationService **requires** mTLS).
- JWT — supported in v1.0.
- `Principal{ID, Name, Role, AuthMethod, Metadata, AuthenticatedAt}`; roles: `admin > operator > readonly`.
- `Authorizer` interface checks principal vs. method.

**Streaming patterns**:
- Command output: chunked stdout/stderr → completion message.
- Batch execution: per-agent `AGENT_START → AGENT_OUTPUT → AGENT_COMPLETE/FAILED → BATCH_COMPLETE/FAILED`.
- State application: `RUN_START → (AGENT_START → STATE_RESULT... → AGENT_COMPLETE/FAILED)* → RUN_COMPLETE/FAILED`.
- Event subscription: optional historical replay (last N seconds) followed by realtime; supports queue groups for load-balanced consumers.
- Membership/Leadership: event-based streams, optional filters.

**Pagination convention**: `page_size: int32`, `page_token: string`; response `next_page_token: string`, `total_count: int32`. Server enforces max page_size.

**REST handlers** (hand-coded in `pkg/api/<domain>/handlers.go` — NOT grpc-gateway in v1.0):
- v1.0 wired: `agents`, `execution`, `state`, `secrets`, `policy` (audit), `cluster`, `apikeys`, `auth`, `runbook`, `gitops`, `webhooks` (outbound).
- post-v1.0 wired: `events` (REST is in v0.1 for /events; the *gRPC* endpoint is in v0.1 too — both shipped), `maintenance`, `schedule`.
- v2.x+ wired: `mirror`, `discovery`.

**Versioning registry** (`pkg/api/versioning`): tracks `Status{current, supported, deprecated, retired, beta, alpha}` plus `ReleasedAt`, `DeprecatedAt`, `SunsetAt`. Used to emit deprecation headers and refuse retired endpoints.

**Codegen flow**: `make proto` → `buf generate` → outputs to `pkg/api/v1/{domain}.pb.go` and `{domain}_grpc.pb.go`. `buf lint` enforces STANDARD rules (with documented exclusions). `buf breaking` checks against `main`.

**OpenAPI**: hand-maintained `api/openapi/openapi-spec.yaml` in v1.0. Auto-gen from protos deferred to v2.x+.

**Gotchas**:
- gRPC ↔ REST drift: REST handlers can lag gRPC. Discipline: every new RPC ships its REST handler in the same PR.
- Streaming reconnect: clients implement exponential backoff; context cancellation terminates streams.
- Error-code mapping: gRPC `status.Code` ↔ HTTP status via `apierror.StatusCode()` — keep in sync.
- mTLS only path is **CoordinationService**; do not expose this RPC to external clients.

---

### 4.6 Agent Runtime

**Purpose**: The `kscore-agent` daemon. Connects to control plane via NATS, registers with metadata, heartbeats, executes commands, applies state, hosts plugins.

**Core subsystems**:
- **Core Agent** (`internal/agent/agent.go`) — owns lifecycle; spawns heartbeat + metadata loops; subscribes to command topic.
- **NATS Client Manager** — connection lifecycle; v1.0 client mode only (embedded/leaf modes deferred to v2.x+).
- **Executor** (`internal/agent/executor.go`) — wraps `os/exec`; timeout via context; user switching; SIGTERM 5s grace then SIGKILL.
- **Metadata Collector** (`internal/agent/metadata.go`) — `gopsutil`-backed; periodic refresh (default 60s) of distro, kernel, IPv4/IPv6 (separated, dual-stack flagged), CPU/memory/disk, virtualization detection.
- **Security Enforcer** (`internal/agent/security.go`) — HMAC signature on commands, principal allowlist, command allowlist/blocklist (glob + regex), env-var filter, max arg length.
- **State Runner** (delegates to `internal/statemgmt`) — runs modules locally.
- **Plugin Host** — loads plugins via `pkg/module` (see §4.18).

**Concurrency**: heartbeat loop, metadata loop, command-handler-per-request — all goroutines under a `WaitGroup`. State protected by `sync.RWMutex`.

**Bootstrap flow** (Epic 27 — TUI-guided, `cmd/kscore-bootstrap` and agent subcommand):
1. **Detect** — probe OS, distro, init system, package manager, existing install, resources.
2. **Configure** — Bubble Tea wizard (or CLI flags / env vars) → mode (demo/production/enterprise), cluster name, node role, storage backend, NATS mode, TLS strategy, blueprint selection.
3. **Validate** — config consistency, DNS/network, storage backend reachability, cert readiness.
4. **Install** — package repo setup (apt/dnf), binary + systemd unit install, certs, schema migration, service enable.
5. **Blueprints** — post-install discovery + parameter substitution + apply.
6. **Verify** — health checks (services up, NATS reachable, DB reachable, cluster joined).

Bootstrap is **transactional with rollback**. Re-runs must be idempotent.

**On-disk layout**:
```
/etc/keystone-core/keystone-core-agent.yaml      # config
/etc/keystone-core/certs/{ca.crt,agent.crt,agent.key}
/etc/keystone-core/nats/creds.txt                # NATS creds
/var/lib/keystone-core/blueprint-tracker.json    # bootstrap state
/var/lib/keystone-core/snapshots/                # rollback snapshots
/var/lib/keystone-core/blueprints/               # downloaded blueprints
```

**Agent config (top-level keys)**: `agent.{id, cluster, heartbeat_interval, metadata_interval, command_timeout, labels}`, `nats.{mode, urls, embedded.*, tls.*}`, `security.{authorization, command_filter}`.

**Agent CLI** (`kscore-agent`): root daemon mode + subcommands `config enable-embedded-nats|disable-embedded-nats`, `bootstrap`, `identity`, `nats` (diagnostics), `service install|uninstall|start|stop|status` (Windows post-v1.0).

**Signals**: `SIGTERM`, `os.Interrupt` → unsubscribe, cancel contexts, drain pending, exit cleanly.

**No HTTP health endpoint on the agent** — health is signaled via heartbeat absence. (Endpoint advertiser publishes health via NATS in v2.x+ with embedded mode.)

**Gotchas**:
- Reconnect storms: exponential backoff is non-negotiable on connection loss.
- Clock skew: heartbeats are timestamped; recommend NTP sync in install docs; trust window in CP must exceed expected skew.
- Cert rotation: agent does not auto-rotate in-memory creds in v1.0 (rotation = restart). Auto-rotation in post-v1.0 with SPIRE.
- Fact collection cost: full metadata collection (DMI, NIC enumeration) is expensive — heartbeats carry only lightweight metrics; full metadata refresh on slower interval.
- Bootstrap idempotency: every phase has a checkpoint. Re-running must not break existing state. Tests must hammer this.
- File-descriptor leaks in long-lived agents — every command-execution goroutine must defer-cleanup.

**v1.0 platform target**: Linux amd64 + arm64. Windows agent post-v1.0, macOS post-v1.0.

### 4.7 Remote Execution & Targeting

**Purpose**: Server-side orchestration for "run this command on these agents." Targeting expressions resolve to agent sets; commands stream output back.

**Targeting** (`internal/targeting/`):
- Glob patterns: `id:web-*`, `hostname:db-prod-*`.
- Direct field match: `os:linux`, `arch:amd64`, `status:online`.
- Label match: `role:web`, `labels.env:prod`.
- Compound: AND/OR/NOT with parens.
- **Engine**: `expr-lang/expr` (compiled VM program) + `gobwas/glob` for glob patterns. `Matcher.Match(agent)` evaluates compiled expression against flattened agent metadata.
- **Server-side only** — agents never see expressions; CP filters then dispatches.

**Execution layer** (`internal/execution/`):
- The `os/exec` wrapper itself lives at `internal/agent.Executor` (§4.6); `internal/execution` defines an `Executor` interface that the agent type satisfies, so the lifecycle primitives below are decoupled from agent specifics.
- `Executor` interface: `Execute(ctx, ExecuteRequest) ExecuteResult` — system-level errors live in `Result.Error` rather than a Go error return.
- `ManagedExecution` — wraps Execute with the state machine (PENDING → RUNNING → RETRYING/COMPLETED/FAILED/TIMEOUT/CANCELLED).
- `Callbacks{OnStarted, OnCompleted, OnFailed, OnTimeout, OnCancelled, OnRetrying, OnRetry}` — observable transitions.
- `Pipeline` (sequential stages with output piping, optional fail-fast, optional Transform per stage). v1.0 includes; rarely used externally but underlies blueprint apply.
- **Shell abstraction**: `Shell` interface (`Bash`, `Sh`, `Powershell`, `Cmd`, `Default`) — `GetDefaultShell()` from `runtime.GOOS`; `IsAvailable()` via `exec.LookPath`.
- **Command policy** (`Strict`/`Normal`/`Permissive` modes; `AllowedCommands`, `AllowedPatterns`, `BlockedCommands`, `BlockedPatterns`, `AllowShellExecution`, `MaxCommandLength` 64KB default). Default mode: `Normal`. `ValidateNoShell()` (renamed in Epic 07 task 7 from `ValidateForShell()` — the original name read backwards) is stricter than `Validate()`: it blocks shell metacharacters `;`, `&`, `|`, backtick. Use `ValidateNoShell` for direct-exec (no shell), `Validate` for shell-mode.

**Batch dispatch** (`internal/controlplane/batch_dispatcher.go`, `internal/targeting/batch.go`):
- `BatchDispatcher.ExecuteBatch(req) → batchID, error` — creates job (UUID or supplied), spawns async goroutine.
- `BatchExecutor` runs commands in parallel using a semaphore (default concurrency 10).
- Progress messages on a 500ms ticker; `BatchSummary{total, successful, failed, success_rate}`.
- States: PENDING → RUNNING → BATCH_COMPLETE / BATCH_FAILED.
- Persistence: `state.BatchJobStore` (job, target, command, status, counts, timestamps); `batch_agent_results` (per-agent success/exit/error/timing).

**Streaming protocol** (gRPC):
```
BATCH_START → [PROGRESS, AGENT_START, AGENT_OUTPUT, AGENT_COMPLETE]* → BATCH_COMPLETE/BATCH_FAILED
```
Single-command stream is similar minus batch wrapping. Output channel buffer = 100 (back-pressure boundary; slow consumers will drop progress, not output).

**CLI** (`kscorectl exec` via `cmd/kscore-exec`):
```
kscorectl exec run "uptime" --target "role:web" [--concurrency N] [--command-timeout 5m] [--continue-on-failure] [--shell bash] [--working-dir /tmp] [--user root] [--env KEY=VAL]
kscorectl exec async <target> -- <command>
kscorectl exec status <job-id>
kscorectl exec list
kscorectl exec output <job-id> [--tail N] [--follow] [--agent X]
kscorectl exec cancel <job-id>
kscorectl exec script <target> <file>
```

**Gotchas**:
- Targeting eval is O(N × E) — narrow expressions help; no agent-set indexing in v1.0.
- Output buffering: large outputs go entirely into memory. Truncate at storage layer (stdout 1 MB, stderr 256 KB, combined 2 MB defaults).
- Stream reconnection: clients re-call `output <job-id>` to resume; no in-stream resume.
- Race on output handlers in pipelines — sync primitives in user-callback aggregators.

**v1.0 deferred**: facts-based selectors (need agent fact schema first), percentage batching, output-tier archival, interactive shell.

### 4.8 State Management & Stdlib

**Purpose**: Declarative configuration management — apply YAML state files to agents, detect drift, remediate. Salt-Project-shaped UX.

**State runner pipeline** (`internal/statemgmt/`):
1. **Parse** — YAML → `StateFile` (variables, includes, declarations).
2. **Template render** — `text/template` with custom filters (`upper`, `lower`, `title`, `trim`, `join`, `split`, `default`); vars + facts as render context.
3. **Validate** — syntax, module existence, parameter validation.
4. **Resolve dependencies** — DAG from requisites (`require`, `require_in`, `watch`, `watch_in`, `prereq`, `prereq_in`, `onchanges`, `onchanges_in`); cycle detection with cycle-path error.
5. **Topological sort** — execution order; independent states run in parallel (limited; sequential default in v0.1 for stability — parallel exec is post-v1.0).
6. **Check phase** — `Module.Check(ctx, decl) → ModuleCheckResult{Matches, Diff}`.
7. **Apply phase** — for `Matches=false`: `Module.Apply(ctx, decl) → StateResult{Success, Changed, Diff, Comment, Duration}`. Idempotent.
8. **Test phase** — `Module.Test(ctx, decl)` verifies post-apply.
9. **Report** — emit `StateResult` per state; emit events (`state.apply.start`, `state.change`, `state.apply.done|fail`, `state.drift`).

**Module interface** (minimal):
```go
type Module interface {
    Name() string
    ValidStates() []string
    Check(ctx, decl) (*ModuleCheckResult, error)
    Apply(ctx, decl) (*StateResult, error)
    Test(ctx, decl) (bool, error)
}
```

**Module registration**: package-level `init()` calls `RegisterModule(name, factory)`. Global `DefaultRegistry`.

**Cross-platform dispatch**: build tags (`//go:build linux`/`//go:build windows`) for OS-specific code OR runtime `runtime.GOOS` switch inside module — dispatch at call-time to `systemd_provider`, `launchd_provider`, `windows_service_provider`, etc.

**Drift detection** (`internal/statemgmt/drift/`):
- Compile desired state from declarations.
- For each state, call `Module.Check()` → diff.
- `DriftSeverity` calculation: `critical` (security/stability), `high` (service impact), `medium` (config mismatch), `low` (cosmetic).
- `DriftReport{DriftStatus[], aggregate_severity}`.
- Optional auto-remediation: re-apply changed states.

**State file DSL example**:
```yaml
metadata:
  name: webserver-setup
  version: "1.0"
variables:
  nginx_user: www-data
packages:
  nginx:
    state: installed
    version: ">=1.20"
files:
  /etc/nginx/nginx.conf:
    state: present
    source: /path/to/nginx.conf
    user: root
    mode: "0644"
    require: [package: nginx]
    watch:   [package: nginx]
services:
  nginx:
    state: running
    enable: true
    require: [file: /etc/nginx/nginx.conf]
    watch:   [file: /etc/nginx/nginx.conf]
```

**v1.0 stdlib (~40 modules)**:

| Category | Modules |
|---|---|
| System & core | `file`, `package`, `service`, `user`, `group`, `cmd`, `system` |
| Scheduled tasks | `cron`, `systemd_timer`, `at` |
| Storage | `mount`, `swap`, `lvm`, `disk`, `link` |
| Network (base) | `network`, `route`, `bond`, `bridge`, `vlan` |
| Firewall (base) | `firewall` (abstraction), `iptables`, `nftables`, `firewalld` |
| SSH & security | `ssh`, `security` (SELinux/AppArmor) |
| System config | `hostname`, `timezone`, `sysctl`, `kernel_module` |
| Files & VCS | `git`, `config`, `archive`, `langpkg` (pip/npm/gem) |
| Certificates | `x509` |

**post-v1.0 stdlib additions** (Windows agent + extended Linux): `win_feature`, `win_firewall`, `win_registry`, `win_service`, `win_package`, `docker_container`, `docker_image`, `docker_network`, `docker_volume`, `web` (nginx/Apache abstraction), `postgres_database`, `mysql_database`, `redis`, `launchd` (macOS).

**v2.x+ stdlib additions**: `k8s_*` family (12 modules); DNS provider modules; niche networking (`promisc`, `wifi`, `dot1x`); vendor-specific.

**CLI** (`kscorectl state`): `apply`, `check`, `drift [--fix]`, `compile`, `show`, `test`, `history`, `rollback`, `export`, `restore`; `kscorectl vars get`.

**Saga/checkpoint integration** (`pkg/saga`, `pkg/statemachine`): minimal v1.0 — saga coordinator scaffolding for multi-step state with compensating transactions; advanced features (resume from checkpoint, cross-state compensation graphs) ship in post-v1.0.

**Gotchas**:
- Idempotency — modules MUST diff before applying; `Check`-`Apply`-`Test` pattern is mandatory.
- Requisite cycles — detect at resolve time; report with full cycle path.
- Template injection — vars/facts from agents may contain template syntax; document as untrusted.
- Drift false positives — exclude transient attributes (mtime, SELinux contexts where not relevant); compare content hash for files.
- Two state files managing the same resource in v0.1 will conflict — no resource locking. Document; consider state namespacing for post-v1.0.

### 4.9 Event System

**Purpose**: Pub/sub event bus + persistence + filtering. Foundation for audit, observability, automation.

**Bus topology**:
- NATS JetStream stream `KSCORE_EVENTS`, subject prefix `kscore.events.>` (always with cluster prefix: `kscore.{cluster}.events.>`).
- Subjects by type: `kscore.{cluster}.events.{category}.{subtype}` — e.g., `kscore.default.events.agent.connect`, `kscore.default.events.state.drift`.
- Stream retention defaults: 7 days, 10 GB, 1 M messages, `DiscardNew` policy (back-pressure on full).
- Durable consumer groups for load-balanced subscribers; manual ack; 30s ack timeout; 3 max redeliveries.

**Persistence**: SQL-backed `EventStore` (separate from JetStream, for long-term query).
- Indexed on type, source, timestamp, severity, correlation_id.
- Retention enforcer (per-type age + count limits) runs hourly on cluster leader.

**Event struct**:
```go
Event{
  ID, Type EventType, Source string, Time time.Time,
  Severity Severity, CorrelationID string,
  Tags map[string]string, Data map[string]any, Subject string,
}
```

**Event taxonomy (v1.0 — 22 types, 6 categories)**:
- `agent.{connect, disconnect, heartbeat, heartbeat_failed, error}` (5)
- `job.{start, complete, fail, output}` (4)
- `state.{apply.start, apply.done, apply.fail, change, drift}` (5)
- `system.{startup, shutdown, error}` (3)
- `user.{login, command, error}` (3)
- `policy.{pass, violation}` (2) — audit-mode only in v1.0

**Filter expressions**: in v0.1 we **adopt `google/cel-go`** (existing project rolls a homegrown parser; rebuild should use cel-go from the start — saves ongoing maintenance and gives a richer feature set for free). Filter on: type, source, severity, time range, tags.*, data.* (nested JSON path), regex, glob.

**EventPublisher** (sync `Publish`, async `PublishAsync`, `Close`); **EventSubscriber** (`Subscribe(pattern)`, `SubscribeQueue(pattern, group)`, `SubscribeWithFilter(pattern, filter)`).

**gRPC EventService**:
- `ListEvents` (filter, pagination, sort)
- `GetEvent`
- `EmitEvent`
- `SubscribeEvents` (stream; optional `replay_seconds` for historical-then-realtime)
- `GetEventTypes`
- `GetEventStats` (counts/breakdown)

**CLI** (`kscore-events`): `list`, `query`, `emit`, `subscribe`, `watch`, `replay`, `retention`, `dlq` (post-v1.0), `storage-stats`, `analyze`.

**Reactor engine** — **post-v1.0 not v1.0**. Filter→action chains; throttle/debounce; bounded concurrency; DLQ on failure; retry-with-exp-backoff. Actions: `LogAction`, `EventAction`, `WebhookAction` initially. _Reasoning: v1.0 ships a passive event system (emit + subscribe + query); reactors land in post-v1.0 once core is proven and runbook/policy boundaries are clear._

**Lifecycle tracking** — **post-v1.0**. Events transition through: created → published → routed → processing → processed/failed/expired. Ships with reactors.

**Gotchas**:
- Reactor loops (post-v1.0) — filters matching own emitted events. No automatic detection in post-v1.0; throttle/debounce mitigate.
- Slow consumers — handler blocks; messages redelivered after 30s. RouteAsync bounded to 100 goroutines.
- Replay window — JetStream retention is the floor for live replay; older events come from EventStore SQL query (slower).
- Clock skew between sources — ordering is per-source, not global.

### 4.10 Identity & Auth

**Purpose**: Establish *who* every actor is (server, agent, user, automation) and *what* they may do.

**Two-mode design**:
1. **Embedded provider (v1.0)** — built-in CA, SVID issuer, attestation engine, token store. Zero external deps.
2. **SPIRE (post-v1.0)** — external SPIRE server, agent socket, federation. Pluggable via `Provider` interface.

**Identity package** (`internal/identity/`):
- `Provider` interface — `Start/Stop`, `Health`, `TrustDomain`, `GetTrustBundle`, `WatchTrustBundle`, `Attest`, `IssueX509SVID`, `IssueJWTSVID`, `CreateJoinToken`, `ListJoinTokens`, `DeleteJoinToken`.
- `SPIFFEID` — `trustDomain + path`. Defaults: `spiffe://kscore.local/server/control-plane`, `spiffe://kscore.local/agent/{id}`, `spiffe://kscore.local/service/{name}`.
- `X509SVID{cert chain, private key, expiry, IssuedAt, Hint}`. Predicates: `Expired()`, `ShouldRotate()` (~50% of lifetime).
- `JWTSVID{token, claims, audience, expiry}`.
- `TrustBundle{X509Authorities, JWTAuthorities, RefreshHint, SequenceNumber}`.
- `JoinToken{Token, Hash, Salt, Prefix, AgentID, TTL, UsedAt, Metadata}`.

**CA Manager** (`internal/identity/ca.go`):
- Root CA: ECDSA-P256 default, 10-year TTL.
- Signing CA: 1-year TTL, auto-rotates 30 days before expiry.
- TLS 1.3 default minimum.
- Key types: `ecdsa-p256` (default), `ecdsa-p384`, `rsa-2048`, `rsa-4096`.
- Methods: `Initialize`, `GetTrustChain`, `ShouldRotateSigningCA`, `RotateSigningCA`, `IssueCertificate(template, ttl)`.

**Auth package** (`pkg/api/auth/`):
- `Principal{ID, Name, Role, AuthMethod, Metadata, AuthenticatedAt}` — `HasRole(Role) bool`.
- `Authenticator` interface — `Authenticate(ctx, credentials) → Principal`.
- Concretes: `APIKeyAuthenticator` (constant-time hash compare), `JWTAuthenticator` (HS/RS/ES family), `MTLSAuthenticator` (peer cert; SAN/CN pattern matching with glob → regex).
- `Authorizer` interface — `Authorize(ctx, principal, method) error`.
- `RBACAuthorizer` — method→role map, bypass list, default-deny-or-allow configurable.
- `RateLimiter` — exponential backoff, per-client lockout, configurable max-failures + window.
- `InterceptorConfig` wires it all into gRPC unary + stream interceptors. CoordinationService **requires mTLS**; bypass list includes health endpoints and agent registration.

**API keys** (`pkg/api/apikeys/`):
- `APIKey{ID, Name, KeyHash, Role, CreatedAt, ExpiresAt, LastUsed}`.
- `Store` (in-memory or DB); `Handler` (REST CRUD).
- Generation: random base62 ≥ 32 chars; hashed (SHA-256) before persistence; cleartext returned only once on creation.
- Rotation: new key issued with overlapping validity; old key revoked after operator confirms switchover.

**Cluster join tokens** (`internal/identity/`, Epic 44):
- Format: `kscore-join-<base62-random>`.
- Stored as SHA-256 hash + salt; never plaintext after generation.
- Defaults: TTL 5m, max-uses 1 (one-time), 24h max TTL.
- Lifecycle: leader generates → operator presents token to new server via `kscorectl cluster join --token X` → server validates (not expired/used/over-max) → store increments use count → revoke on max-use.
- Background cleanup: hourly on leader.

**RBAC (v1.0 minimum, full post-v1.0)**:
- v1.0: three roles — `admin > operator > readonly` — with hardcoded method→required-role map.
- v1.0: bypass methods list (health, registration, coordination).
- post-v1.0: full Role/Permission CRUD, principal bindings, dynamic policy.

**Config keys**: `identity.enabled`, `identity.provider.type` (`embedded|spire|aws|gcp|azure`), `identity.trust_domain`, `identity.ca.{key_type, root_ca_ttl, signing_ca_ttl, rotate_signing_ca_before}`, `identity.svid.{default_ttl, max_ttl}`, `identity.attestation.allowed_attestors`, `identity.nats.require_mtls`, `security.min_tls_version`.

**CLI** (`kscore-identity`): `token {create, list, revoke, cleanup}`, `ca {info, rotate-signing, export}`, `federation {add-domain, list, fetch-bundle}` (post-v1.0), `status`.

**Gotchas**:
- Cert rotation under clock skew — grace period must exceed expected skew across fleet.
- API key timing attacks — use constant-time comparison.
- mTLS with NATS leaf nodes (v2.x+) — leaf certs are separate from agent identity certs; rotate independently.
- SPIRE socket availability — no auto-fallback; missing socket = attestation failure (must surface clearly).
- JWT role claim — missing → readonly fallback (with warning); invalid string → reject (don't default).

**Why SPIFFE-shaped from day 1**: even v1.0 with embedded CA uses SPIFFE IDs in URI SANs. post-v1.0 SPIRE swap-in is a provider implementation change; nothing else moves.

---

### 4.11 Secrets Management

**Purpose**: Unified broker for secret retrieval, lease management, and encryption-as-a-service. Used by agents (NATS credentials, app passwords) and operators (CLI/REST).

**Architecture**:
- `SecretBroker` — entry point; routes by **path-prefix** (longest-match-first); delegates to backends; applies cache; emits audit.
- `SecretBackend` interface — `GetSecret`, `WriteSecret`, `ListSecrets`, `DeleteSecret`, `IssueDynamicSecret`, `RenewLease`, `RevokeLease`.
- `LeaseManager` — persistent SQLite store; in-memory tracking; background scheduler with strategy:
  - `eager` — renew at 50% of TTL.
  - `lazy` — renew at 90% of TTL.
  - `on_demand` — renew only when client asks.
- `RotationOrchestrator` (post-v1.0) — strategies (blue-green, rolling, canary, immediate); health checks; auto-rollback.
- `TransitBackend` — encryption-as-a-service (Vault transit engine); encrypt/decrypt/sign/verify/HMAC; batch ops; key versioning; convergent option.
- `SecretCache` — in-memory L1, AES-GCM at-rest, TTL eviction (default 5m), prefix-deletion on revoke; bounded-LRU eviction.
- `SecretAuditLogger` — every access emits an event with agent ID, SPIFFE ID, action, path, timestamp, duration; sensitive data is masked via `LogMasker` regex set.

**v1.0 backends**:
1. **Encrypted-file** (`internal/secrets/file/`) — AES-GCM, JSON, no external deps. Ideal for dev, air-gap demo.
2. **HashiCorp Vault** (`internal/secrets/vault/`) — KV v1/v2, dynamic secrets (DB, IAM, PKI, SSH), transit, namespace support, all auth methods.

**v2.x+ backends**: AWS Secrets Manager, Azure Key Vault, GCP Secret Manager (all already implemented; just gated to v2 for SDK weight).

**Lease lifecycle**:
```
Pending → Active → Renewing → Active (loop)
                 ↘ Expired ↘
                  Revoked   → Cleanup → removed
```

**Secret retrieval flow**:
1. Agent (SPIFFE-authenticated) → REST/gRPC `GetSecret(path)`.
2. Broker → policy check (OPA/CEL — audit-mode in v1.0).
3. Cache lookup → on hit, AES-GCM decrypt and return.
4. Cache miss → backend `GetSecret` → cache encrypted → return.
5. If dynamic, register lease with `LeaseManager`.
6. Audit logger emits `secret.access` event.

**Public API** (`pkg/api/v1/secrets.proto`):
- CRUD: `GetSecret`, `ListSecrets`, `WriteSecret`, `DeleteSecret`.
- Leases: `GetLease`, `ListLeases`, `RenewLease`, `RevokeLease`.
- Transit: `Encrypt`, `Decrypt`, `Sign`, `Verify` (and unspecified-but-likely batch variants in `pkg/api/secrets/`).

**REST**: `/api/v1/secrets/{path}` (CRUD), `/api/v1/leases/{id}/renew|revoke`, `/api/v1/transit/{op}/{key}` (encrypt/decrypt/sign/verify/datakey/batch-*).

**CLI** (`kscore-secrets`): `get`, `list`, `backends`, `audit`, `dynamic`, `leases`, `cache`, `encrypt`, `decrypt`, `template`. Rotation subcommands ship in post-v1.0.

**Config**:
```yaml
secrets:
  default_backend: file        # or vault
  backends:
    - name: file
      type: encrypted_file
      file: { path: /var/lib/keystone/secrets.bin, master_key: env:KSCORE_SECRETS_MASTER }
    - name: vault
      type: vault
      vault: { address: https://vault.internal, auth_method: approle, ... }
  routing:
    - prefix: secret/             # longest-prefix-first
      backend: vault
    - prefix: kv/                 # default
      backend: file
  cache: { enabled: true, max_entries: 10000, default_ttl: 5m }
```

**Gotchas**:
- Cache invalidation on backend update is **explicit** (refresh param or operator clear); document.
- Master key rotation breaks cache; mitigate via dual-key window.
- Transit backend round-trips (Vault) add 50–200ms; batch ops materially help.
- Lease renewal storms — add jitter (default scheduler does).
- Audit log disk-full silently breaks: circuit-break + alert is mandatory.

### 4.12 Audit & Policy

**Purpose**: Two related concerns shipped together — **audit log of all sensitive ops** (full v1.0) and **policy engine** (audit-mode-only v1.0; full enforcement post-v1.0).

**Why split this way**: Audit is non-negotiable for compliance-curious users. Full policy enforcement carries breaking-change risk (a misconfigured policy blocks the fleet). v1.0 ships the engine in audit-mode so users can run real policies against real workloads, see what *would* have been blocked, and build confidence. post-v1.0 flips `policy.enforcement_enabled=true` and turns the audit-mode auditor into a true gate.

**Audit log infrastructure (full v1.0)**:
- `Auditor` (in-memory circular buffer; configurable size).
- `AuditStore` interface; `SQLitePolicyAuditStore` (WAL, indexed on `policy_id`, `actor`, `resource_type`, `timestamp`, `severity`, `allowed`).
- `AuditEntry{ID, Timestamp, PolicyID, PolicyName, PolicyType, ResourceType, Allowed, Duration, Violations[], EnforcementMode, User, Action, Metadata}`.
- `AuditFilter{PolicyID, Allowed, Severity, ResourceType, User, Action, StartTime, EndTime, Limit}`; pagination.
- `AuditSummary{TotalEvaluations, AllowedCount, DeniedCount, ViolationsByPolicy, ViolationsBySeverity, TimeRange}`.
- `AuditRetentionPolicy{MaxAge default 90d, MaxCount default 100k, MinSeverity, RetentionInterval default 1h}`.
- `AuditRedactionConfig{RedactMetadataKeys, RedactPatterns regex[], RedactUser bool}`. Applied on export.
- Audit export: JSON, JSONL, CSV via CLI (`kscore-audit export`).
- **Hard rule for the rebuild**: every sensitive op (auth decision, secret access, command exec, state apply, policy eval) emits an audit entry. *Failure to log = bug.*

**Policy engine infrastructure (full v1.0; enforcement gated)**:
- `Engine{registry, opaEvaluator, celEvaluator, builtinEvaluator}` — coordinator.
- `Policy{ID, Name, Type (OPA|CEL|Builtin), Category (Security|Compliance|Operational|Cost|Custom), Severity (Low|Medium|High|Critical), EnforcementMode (Audit|Warn|Enforce), Code, Enabled, Tags, Metadata, CreatedAt, UpdatedAt}`.
- `PolicySet` — group of policies; set-level enforcement override.
- `Bindings` — attach policies/sets to resource types (with optional action/selector).
- **Evaluators**:
  - `OPAEvaluator` — wraps `open-policy-agent/opa/v1/rego` (v1.16.2). **v1.0 conventions (Epic 12 task 6)**: Rego v1 syntax; **fixed package `keystone.policy`** (queries the whole `data.keystone.policy` object and reads `allow` / `violations` / `warnings` keys — predictable, no module parsing); undefined/non-bool `allow` → `Allowed=false` + a synthetic `audit.Violation` (fail-closed but surfaces the misconfig in the audit trail; not an engine error); `violations` best-effort (string → message, object → rule/message/severity/path/expected/actual/remediation); restricted capability set strips `http.send`/`net.*`/`opa.runtime` builtins (a policy referencing them fails to compile); compiled `PreparedEvalQuery` cached by `policyID+sha256(Code)`; per-eval timeout (default 5s) guards pathological policies.
  - `CELEvaluator` — wraps `google/cel-go`; vars `input`, `resource`, `action`, `user`, `context`.
  - `BuiltinEvaluator` — hardcoded rules: `require-labels`, `require-owner`, `allowed-environments`, `allowed-actions`, `deny-privileged`, `allowed-users`, `time-window`, `no-root-execution`, `require-approval`, `max-concurrent`, `resource-quota`, `pattern-allow`, `pattern-deny`. Config via JSON in policy `Code`.
- `EvaluationInput{Resource, Action, User, Context, Timestamp}`.
- `EvaluationResult{PolicyID, PolicyName, Allowed, Violations[], Warnings[], Message, Duration, EvaluatedAt}`.
- `Violation{Rule, Message, Severity, Path, Expected, Actual, Remediation}`.
- `Engine.Evaluate(ctx, policyID, input)` / `EvaluatePolicySet(ctx, setID, input)` / `EvaluateForResource(ctx, resourceType, input)`.

**v1.0 enforcement gate**: `Enforcer` exists but **always returns `Allowed=true` regardless of evaluation result**. Policies still evaluate, audit, and report — they just don't block. Config `policy.enforcement_enabled=false` (hardcoded false in v1.0).

**Enforcement changes (post-v1.0)**: `policy.enforcement_enabled=true` becomes available; `Enforcer` honors `EnforcementMode` per policy:
- `Audit` — log only (v1.0 default behavior).
- `Warn` — log + emit warn event; allow operation.
- `Enforce` — log + invoke violation handlers; deny operation.

**Compliance reporting (v1.0)**:
- `ComplianceReport{Period, ComplianceRate, TotalEvaluations, CompliantEvaluations, NonCompliantEvaluations, PolicyStats[], TopViolations[], ViolationsBySeverity, Trend[]}`.
- `ComplianceControl{ID, Framework (CIS|SOC2|NIST-800-53|HIPAA|PCI-DSS|GDPR|ISO-27001|Custom), Title, Severity, PolicyIDs[]}`.
- `ControlMapping` — 2-way framework↔policies lookup.
- `ResourceAuditTrail` — all evaluations for a single resource over time.

**APIs**:
- `PolicyService` gRPC: `EvaluatePolicy`, `EvaluatePolicySet`, `ListPolicies`, `GetPolicy`, `CreatePolicy` *(post-v1.0)*, `UpdatePolicy` *(post-v1.0)*, `DeletePolicy` *(post-v1.0)*, `ListViolations`, `GetComplianceReport`, `GetAuditLog`, `ListPolicySets`, `GetPolicySet`. v1.0 server returns `Unimplemented` for post-v1.0-gated CRUD methods.
- REST: `/api/v1/policies` (list, evaluate, violations, compliance, audit-log).
- CLI v1.0 subset: `kscore-policy list|validate|check|show|eval|test|compliance|violations`. (`create|update|delete|activate|deactivate|remediate|monitor` are post-v1.0.)
- CLI v1.0: `kscore-audit log|report|export|stats|search|analyze|timeline|watch`.

**Gotchas**:
- OPA policies MUST declare `package keystone.policy` and an `allow` rule (undefined `allow` is fail-closed-with-violation, not an error). `http.send`/`net.*`/`opa.runtime` are unavailable by default — policies are pure decision logic. Document templates + the restricted-builtin list.
- CEL is dynamically typed — type errors surface at eval, not compile.
- Policy-set semantics are **all-or-nothing AND**. Document; consider adding OR/threshold semantics for compliance-style sets in post-v1.0.
- The enforcement flip is a behavior-changing release; release notes must call it out loudly.
- SQLite audit table can grow fast — retention policy MUST be set in v0.1 defaults.
- Redaction regex must be reviewed before prod (overly broad patterns = false positives in audit data).

### 4.13 GitOps Integration

**Purpose**: Bridge GitOps deployers (ArgoCD/Flux) and Git providers (GitHub/GitLab) to Keystone Core's runtime control plane. v1.0 ships the basics: ingest, verify, manual rollback. Promotion + canary slip to post-v1.0/post-v1.0.

**Webhook ingest** (`internal/gitops/webhook/`):
- HTTP server (default `:8080/webhooks`).
- `Handler` interface — `Type()`, `Parse(request, body) → Event`. Concrete: ArgoCD, Flux, GitHub, GitLab.
- `Authenticator` interface — `HMACAuthenticator` (SHA-256 HMAC; secret per source), `BearerAuthenticator`, `NoneAuthenticator`.
- Source auto-detection by header (`X-GitHub-Event`, `X-Gitlab-Event`, `X-Argo-CD-Webhook`, `X-Flux-Event`).
- Event normalization → unified `webhook.Event{webhookID, provider, application, namespace, revision, status, raw}`. `ToKscoreEvent()` emits on Keystone event bus as `gitops.{argocd|flux|github|gitlab}.*`.
- Replay protection in v1.0: HMAC-only. Timestamp-window + nonce dedup land post-v1.0.

**Verification engine** (`internal/gitops/verification/`):
- `Verifier` interface — `Type()`, `Verify(step) → Result`.
- v1.0 verifiers: HTTP, gRPC, command/script. post-v1.0 adds Kubernetes (resource ready), Prometheus (metric query), log analysis.
- `Workflow{Steps, Parallel, Timeout, OnFailure}` — sequential default; parallel via goroutines.
- `Result{Success, Message, Data, Duration, Error, Retries}`. Per-step retries + timeout.
- Optional steps don't fail the workflow on individual failure.

**Rollback engine** (`internal/gitops/rollback/`):
- `Executor` interface — `Type()`, `Execute(ctx, config, req) → Result`, `GetPreviousRevision()`, `GetLastKnownGood()`.
- v1.0 executors: Git revert, ArgoCD sync-to-revision, K8s rollout undo. post-v1.0 adds Flux suspend.
- `Engine` — `RegisterExecutor`, `Execute`, `ApproveRollback`, `GetRollback`, `ListRollbacks`. Optional approval gates.
- State machine: `Pending → (Approved|Rejected) → InProgress → (Completed|Failed) → (Verifying → Verified|VerificationFailed)`.

**Promotion engine (post-v1.0)**:
- `Pipeline{Name, Application, Environments[], Strategy (blue-green|canary|rolling|immediate), CanarySteps, Thresholds, Remediation}`.
- `Environment{Name, AutoPromote, RequireApproval, VerificationWorkflow, Thresholds override}`.
- `CanaryStep{Weight (5/25/50/100), Duration, VerificationWorkflow, Thresholds override}`.
- State machine: `Pending → (WaitingApproval | InProgress) → (Verifying | RollingOut) → (Completed | RollingBack) → (RolledBack | Completed)`.
- Remediation strategies (post-v1.0): rollback, scale-down, traffic-shift, custom.

**APIs (v1.0)**:
- REST: `/api/v1/gitops/verifications` (GET list, get), `/api/v1/gitops/rollback` (POST), `/api/v1/gitops/rollbacks` (GET list, get), `/api/v1/gitops/rollbacks/{id}/approve` (POST).
- CLI: `kscore-gitops verify <workflow-file>` (flags: --parallel, --timeout, --output), `kscore-gitops rollback --app X --strategy previous|specific|last-known-good --revision Y --reason Z`. post-v1.0 adds `promote`, `status`, `repo`, `deploy`, `git-sync`.

**External clients**:
- `argocd.Client` — gRPC; sync, rollback, get app status.
- `github.Client` — REST (`google/go-github`); deployment status updates.
- `gitlab.Client` — REST (`xanzy/go-gitlab`); commit status, MR ops.
- `gitsync.Syncer` — go-git; clone, pull, commit, branch, MR/PR creation.

**Gotchas**:
- Webhook signed-replay: HMAC only — capture+replay is possible. post-v1.0 adds timestamp window + nonce dedup.
- Secret rotation: single secret per auth method in v1.0; rotation requires restart.
- Rollback storms: no cooldown in v1.0; rely on approval gates and operator judgment.
- Canary-fail mid-weight (post-v1.0): traffic shift to 0% must be atomic with rollback.
- Approval timeout: not enforced in post-v1.0; operator must intervene. post-v1.0 adds default expiry.
- Verification timeout: per-step config; default conservative.

### 4.14 Outbound Webhooks

**Purpose**: Push event notifications to external systems (Slack, PagerDuty, custom HTTP receivers). Closes the loop on integration.

**Architecture** (`internal/webhook/outbound/`):
- `SubscriptionStore` — SQLite-backed CRUD; survives restart. Schema: `subscriptions(id, name, url, secret, events_json, enabled, headers_json, max_retries, timeout_sec, created_at, updated_at)`; `deliveries(id, subscription_id, event_type, event_id, status, status_code, attempt, error, delivered_at)`.
- `Manager` — subscribes to NATS event bus on `>` (cluster-prefixed in v1.0); pattern-matches each event against each enabled subscription's filter list (glob); fans out async.
- `Dispatcher` — `Deliver(ctx, sub, payload, deliveryID) → (statusCode, error)`. HTTP POST with custom headers, HMAC signature header (`X-Keystone-Signature: sha256=<hex>`), per-subscription timeout.
- `RetryQueue` — exponential backoff with jitter; max retries default 3; on exhaustion → delivery `failed` (history retains).
- Per-endpoint **circuit breaker** (concurrent-safe `sync.Map`): `closed → open` after 5 failures; `→ half-open` after 30s; `→ closed` after 2 successes.
- Concurrency: `sync.WaitGroup` tracks in-flight; bounded goroutines for back-pressure; `Stop()` waits for drain.

**Subscription model**:
```go
Subscription{
  ID, Name, URL, Secret, Events []string,
  Enabled bool, Headers map[string]string,
  MaxRetries int, TimeoutSec int,
  CreatedAt, UpdatedAt time.Time,
}
```

**Delivery audit**:
```go
DeliveryRecord{
  ID, SubscriptionID, EventType, EventID,
  Status DeliveryStatus,  // pending|success|failed|retrying
  StatusCode, Attempt int, Error string,
  DeliveredAt time.Time,
}
```

**Signing** — `Sign(secret, payload) → "sha256=<hex>"`; `Verify()` for receiver-side validation. GitHub-compatible.

**APIs**:
- REST: `GET/POST /api/v1/webhooks/subscriptions`, `GET/PATCH/DELETE /api/v1/webhooks/subscriptions/{id}`, `POST {id}/test`, `GET {id}/deliveries`.
- CLI: `kscore-webhook outbound list|create|show|delete|history|test`.

**Config**:
```yaml
webhook:
  outbound:
    enabled: true
    max_retries: 3
    retry_backoff: 1s
    timeout: 10s
    max_payload_size: 1048576
    delivery_retention: 168h
```

**Gotchas**:
- Secret in API responses **always masked** (`***`); cleartext returned only on creation.
- Slow receivers — circuit-breaker mitigates; delivery timeout caps.
- Filter perf at high event volume — glob patterns are evaluated per event per subscription (no precompilation in v1.0). Cache compiled glob in post-v1.0 if profiling shows hot spot.
- Delivery history growth — `DeleteOldDeliveries(retention)` exists but auto-invocation in v0.1.x (trivial fix).
- Circuit-breaker false positives — network timeouts count as failures; transient issues can flip a healthy receiver to `open` briefly. Acceptable.

### 4.15 Clustering & HA

**Purpose**: The v1.0 differentiator. Runs `kscore-server` as a 3-node cluster with leader election, automatic failover, consistent-hash agent assignment, NATS-fallback recovery, and split-brain prevention.

**Topology (v1.0)**:
- 3 × `kscore-server`, each with embedded etcd (single binary deploy).
- 1 × Postgres (shared state).
- 1 × NATS cluster (or embedded NATS in each server with leaf links — v2.x+ enhancement).
- Production scaling path: 5 × `kscore-server` + external 3-node etcd + Postgres replica + external NATS cluster.

**Core components** (`internal/cluster/`):

| Component | Purpose |
|---|---|
| `EtcdClient` | Wraps `etcd v3 client`; embedded or external; lease + watch; auto-sync. |
| `MembershipManager` | Ephemeral keys (lease TTL 15s); 5s heartbeat; member metadata; observers. **Landed Epic 13 task 2** (`internal/cluster/membership.go`): ephemeral-lease registration + keepalive, heartbeat loop, `LoadMembers`/`GetMember`, single shared `WithPrevKV` watch → `MemberEvent{Joined,Updated,Left}` observer fan-out + `WatchMembers` channel adapter, validated `MemberStatus` state machine (HEALTHY/LEAVING owned here; DEGRADED/UNHEALTHY are HealthMonitor/task 7), stable UUIDv7 member ID persisted across restarts (RecoveryManager/task 10 shard reclaim). Anti-flap guard enforced in `config.ClusterConfig.Validate`: `lease_ttl_seconds ≥ 3× membership.heartbeat_interval` (default 15s/5s). |
| `LeaderElector` | `concurrency.Election` primitives; campaign loop; resignation; transfer; observers. **Landed Epic 13 task 3** (`internal/cluster/election.go`): single worker-ctx campaign goroutine owns the `*Session`/`*Election`; `LeaderState` SM (Unknown→Campaigning→Elected→{Resigned\|Transferred\|Lost}→Campaigning); session-loss recreates + re-campaigns; `Resign`/`TransferLeadership` (+`ReCampaignDelay`) are no-ops when not leader; `IsLeader`/`State`/`LeaderID` (`ErrNoLeader`); `LeadershipObserver` snapshot fan-out. `config.ClusterElectionConfig.session_ttl_seconds` (default 3, the "<3s leader" SLO target — separate from the membership anti-flap lease; SLO tuning is task 18). `IsLeader` is the leader-check the audit/events `WithRetentionLeaderCheck` seam consumes — wired by SingletonTaskManager (task 9), not here. |
| `ShardManager` (consistent hash ring) | Configurable virtual nodes (default 150); agent → member by `hash(agentID)`; rebalance on topology change. **Ring primitive landed Epic 13 task 4** (`internal/cluster/hashring.go`): pure `HashRing` (no etcd/dep; stdlib FNV-1a-64), `Add`/`Remove`/`Get`/`Members`/`Has`/`Len`, RWMutex; **deterministic rebuild from the sorted member set** so every node agrees on key→owner regardless of mutation order (collisions → smallest member). `config.ClusterShardConfig.virtual_nodes` (default 150). ShardStore persistence is task 5. **ShardManager landed task 6** (`internal/cluster/shardmanager.go`): composes HashRing+ShardStore+MembershipManager-observer; eligible-member ring sync on every node; `Owner` (sticky-else-ring), `Rebalance` (CAS-reconcile, minimal), `AssignAgent` (create-only), debounced ≥`rebalance_cooldown` (default 5s) on membership flaps; **leader-only write seam `LeaderCheck`** (ring synced everywhere, only leader persists — wired to LeaderElector by task 9's SingletonTaskManager); `RebalanceObserver` → `[]ShardMove` for FailoverManager (task 8). |
| `ShardStore` | etcd-backed `agentID → memberID` mapping, versioned for optimistic locking. **Landed Epic 13 task 5** (`internal/cluster/shardstore.go`): stateless wrapper over a started `EtcdClient`; JSON record at `<prefix>/shards/<agentID>`; `Version` = etcd `ModRevision`. `Assign`/`AssignIf(expected)` (CAS via Txn; `expected==0` = create-only) / `Get` (`ErrShardNotFound`) / `List` (sorted) / `Delete` (idempotent) / `DeleteIf(expected)` (CAS; nil if absent) / `Watch` (`ShardEvent{Set,Deleted}` via prev-kv). `ErrVersionConflict` on stale CAS. Rebalance + ShardManager composition is task 6. |
| `HealthMonitor` | Consecutive-failure threshold (default 3); P50/P99 latency; partition detection via quorum loss. **Landed Epic 13 task 7** (`internal/cluster/health.go`): pluggable `HealthChecker` (built-in etcd/heartbeat; DB/NATS as injected ping funcs — keeps internal/cluster dep-clean), per-checker consecutive-failure + `latencyRing` P50/P99, quorum-minority on `LoadMembers` error / failing critical etcd checker (enforcement is FencingManager/task 11), and drives `MembershipManager.SetStatus` through the Task 2 SM via **valid edges only** (stepwise HEALTHY→DEGRADED→UNHEALTHY; recover direct) — the consumer closing the loop with task 6's `eligible()`. `config.ClusterHealthConfig` (check_interval 5s / failure_threshold 3 / latency_window 100). |
| `FailoverManager` | Detect → reassign agents (batch 100) → reassign jobs (batch 50, idempotency keys); cooldown 10s. **Landed Epic 13 task 8** (`internal/cluster/failover.go`): `FailoverState` SM (IDLE→DETECTING→INITIATED→IN_PROGRESS→COMPLETED / FAILED / ROLLED_BACK); detection = membership UNHEALTHY/Left correlated with ShardManager moves whose `From` is the failed member (join-driven moves ignored); per-member signal dedup; pluggable `AgentReassigner`/`JobReassigner` (boot-wired) batched 100/50 with deterministic idempotency key `failover/<member>/<episode>/<batch>`; reassigner error → FAILED (+optional Rollback → ROLLED_BACK); 10s cooldown; leader-gated `LeaderCheck` seam (wired by task 9). `config.ClusterFailoverConfig` (cooldown 10s / agent_batch 100 / job_batch 50). |
| `SingletonTaskManager` | Leader-only tasks: reactor coordinator, scheduled jobs, cleanup, metric aggregation, agent rebalance. **Landed Epic 13 task 9** (`internal/cluster/singleton.go`): `SingletonTask` iface + `StartStopTask`/`LoopTask` adapters; observes `*LeaderElector`, Start/Stop all registered tasks on leadership gain/lose (idempotent, txMu-serialised); `LeaderCheck() func() bool` is the canonical leader accessor for the ShardManager/FailoverManager/RetentionEnforcer seams. Reactor-coordinator + scheduled-jobs tasks are v1.1. **Boot wiring of LeaderCheck into those seams (incl. audit/events RetentionEnforcer `AlwaysLeader`) is a deferred `gate-v1.0` ROADMAP item** — depends on cluster→kscore-server boot integration. |
| `RecoveryManager` | Phases: STARTING → CONNECTING → SYNCING → VERIFYING → REJOINING → RECLAIMING → COMPLETED. **Landed Epic 13 task 10** (`internal/cluster/recovery.go`): one-shot `Recover(ctx)`; CONNECTING = retried etcd probe; SYNCING = `ShardStore.List` (+ best-effort `LoadMembers`, `ErrNotRegistered` tolerated pre-rejoin); REJOINING = `Membership.Register` under the stable Task-2 member ID; RECLAIMING filters the shard map by `Self().ID` → pluggable `AgentReclaimer` (boot-wired); terminal `FAILED` wraps the failing phase; single-use; `RecoveryObserver`. `config.ClusterRecoveryConfig` (connect_timeout 5s / connect_retries 3). |
| `FencingManager` | Lease + epoch fencing; modes: STRICT, READ_ONLY, GRACEFUL. **Landed Epic 13 task 11** (`internal/cluster/fencing.go`): split-brain enforcement (HealthMonitor only detects). Quorum-loss ⇒ fenced (lease fencing); leader election bumps an etcd epoch (`<prefix>/fence/epoch`, Txn CAS) watched for staleness ⇒ a deposed leader self-fences (epoch fencing). `Guard(OpType)` enforces per mode (default `read_only` per §4.15 acceptance) + in-flight `Drain` (GRACEFUL); `ValidEpoch(e)` for op-commit checks. `config.ClusterFencingConfig.mode`. Wiring `Guard` around server write paths is a deferred `gate-v1.0` ROADMAP item (server-boot integration). |
| `CoordinationServer/Client` | mTLS gRPC `CoordinationService` for server↔server; NATS-down recovery channel. **Server landed Epic 13 task 12** (`internal/controlplane/grpc_coordination_server.go`; `coordination.proto` realigned additively, `buf generate`): all 6 RPCs (ClusterHealth/LookupLeader/NATSStatus/RecoveryCoordinate/NodeHeartbeat/PropagateState) delegating to nilable cluster providers; **mTLS-only `requireMTLS` guard** (no verified client cert ⇒ codes.Unauthenticated — the acceptance criterion). Boot registration on a dedicated mTLS listener is a deferred `gate-v1.0` ROADMAP item. **Client landed Epic 13 task 13** (`internal/controlplane/coordination_client.go`): per-peer pooled `v1.CoordinationServiceClient` (lazy conns; AddPeer/RemovePeer/SetPeers), all 6 RPC wrappers with retry + capped exponential backoff (transient codes only), NodeHeartbeat liveness tracking (FailureThreshold → unreachable + `PeerObserver`), mTLS via injected DialOptions; `config.ClusterCoordinationConfig`. Driving the pool from MembershipManager + real mTLS creds is the same deferred boot-wiring ROADMAP item. |
| `StateStore`, `ConfigStore`, `ShardStore` | etcd-backed namespaced stores (general state, hot-reload config, shard map). |

**Member lifecycle**:
```
HEALTHY → DEGRADED → UNHEALTHY → LEAVING → removed
  ↑__________|        (heartbeat threshold)
   recover
```

**Leader transitions**:
```
no leader → CAMPAIGNING → ELECTED → (TRANSFERRED|LOST) → CAMPAIGNING
```

**Failover workflow**:
1. HealthMonitor: heartbeat-loss → consecutive-failure threshold → DETECTING.
2. FailoverManager: → INITIATED.
3. Reassign agents in batches (100).
4. Reassign jobs in batches (50) with idempotency keys (no double-execution).
5. Verify consistency → IN_PROGRESS → COMPLETED.
6. Cooldown 10s before next failover.

**Recovery (after restart)**:
1. STARTING — load state.
2. CONNECTING — establish etcd.
3. SYNCING — load member info, shard assignments.
4. VERIFYING — validate.
5. REJOINING — register with new lease.
6. RECLAIMING — claim agents per shard map.
7. COMPLETED.

**Graceful shutdown**:
1. RUNNING → INITIATED on SIGTERM.
2. DRAINING — stop accepting agents.
3. TRANSFERRING — move leadership.
4. DEREGISTERING — drain in-flight, remove member key (timeout 30s).
5. COMPLETED.

**Landed Epic 13 task 14** (`internal/cluster/shutdown.go`): one-shot `GracefulShutdown.Shutdown(ctx)` driving the 5-phase machine (terminal FAILED only if Deregister errors — the member key must come out). DRAINING = `StopAccepting` hook then `Membership.SetStatus(LEAVING)` (ShardManager rebalances this node's agents to peers before exit → "no agent disconnections"); TRANSFERRING = `Leadership.TransferLeadership` if leader; DEREGISTERING = `Drainer.Drain` (FencingManager GRACEFUL, best-effort within `cluster.shutdown.timeout` 30s — a slow drain never blocks key removal) then `Membership.Deregister`. Narrow nilable collaborator interfaces; single-use; `ShutdownObserver`. The cmd/kscore-server SIGTERM→Shutdown hookup + real StopAccepting is the deferred boot-integration ROADMAP item.

**Split-brain prevention**:
- Quorum size = N/2 + 1 (3-node → 2; 5-node → 3).
- Minority partition: writes blocked within 1s; reads continue.
- **Lease fencing**: every write requires a current etcd lease.
- **Epoch fencing**: leader election bumps epoch; stale-epoch operations rejected.
- Modes: `STRICT` (block all), `READ_ONLY` (allow reads), `GRACEFUL` (finish in-flight then block).

**APIs**:
- gRPC `ClusterService`: GetClusterStatus, ListMembers, GetMember, AddMember, RemoveMember, GetLeader, TransferLeader, Rebalance, CreateBackup, RestoreBackup, WatchMembership (stream), WatchLeadership (stream).
- gRPC `CoordinationService` (mTLS-only): ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, Heartbeat, PropagateState.
- REST: `/api/v1/cluster/{status,members,members/{id},leader,leader/transfer,rebalance,backup,restore}`.
- CLI: `kscore-cluster status|members|leader|add|remove|transfer-leader|rebalance|backup|restore`. `kscore-cluster-backup backup|restore|list|verify|schedule`.

**Config**:
```yaml
cluster:
  enabled: true               # default false; opt-in
  member_id: ""               # auto-UUID
  cluster_name: keystone-core
  heartbeat_interval: 5s
  heartbeat_timeout: 30s      # ≥ 3 × interval
  election_timeout: 15s
  quorum_size: 0              # auto N/2+1
  address_family: prefer_ipv4
  etcd:
    mode: embedded            # or external
    endpoints: []
    leases_ttl: 15s
    embedded:
      client_port: 2379
      peer_port: 2380
      data_dir: ./etcd-data
    tls: { enabled: false, cert_file, key_file, ca_file }
```

**v1.0 SLO targets** (must meet in CI):
- Cluster forms: < 10s.
- First leader: < 3s.
- Failover detection: < 5s.
- Agent reassignment: < 10s.
- Minority partition blocks writes: < 1s.
- Recovery (restart): < 15s.
- Zero job loss/duplication during failover (idempotency keys).

**HA resilience tests** (`test/e2e/ha_*_test.go`): NATS-failure, etcd-failure, network-partition, split-brain. CI must run these on every release.

**Gotchas**:
- Embedded etcd → ≤3 members; use external etcd for 5+.
- Heartbeat-timeout < 3× interval = leader flapping.
- Slow NATS → false failover; CoordinationService is the safety net.
- etcd disk full → read-only → cluster freezes. Monitor.
- Connection storms after failover — rate limit + stagger reconnect.
- Clock skew across servers → unexpected lease expiry. NTP is mandatory.
- Backup at point-in-time may catch mid-commit state — leader-initiated backup ensures ordering.

---

### 4.16 Observability

**Purpose**: Logs, metrics, traces, health, and (eventually) a TUI single-pane-of-glass.

**v1.0 layers**:
- **Logging** (`internal/logging/`) — `log/slog`-backed; structured JSON default; logfmt and text formatters available. Outputs: stdout in v1.0 (syslog/journald/Windows Event Log/NATS in post-v1.0). Correlation IDs flow via context, request middleware, NATS message headers, span attributes.
- **Metrics** (`internal/metrics/`) — custom Prom registry; `Collector` interface (counter, gauge, histogram, summary); `MetricRegistry` with metric definitions; `Timer` utility; `cardinality.Limiter` enforces hard label-cardinality limits with drop/aggregate fallback. Endpoint `/metrics` Prometheus-exposition format.
- **Tracing** (`internal/tracing/`) — OTel SDK; `TracerProvider`; samplers: `always_on/off`, `probabilistic`, `parent_based`, `rate_limiting`, `adaptive` (rebalances on observed error rate). Exporters: OTLP (gRPC + HTTP), Zipkin, stdout. Helper attribute functions: `AgentAttrs`, `JobAttrs`, `StateAttrs`, `EventAttrs`, `PolicyAttrs`. Batch processor.
- **Health** (`internal/health/`) — `Checker` interface; concrete: NATS, DB, JetStream, custom. `Status` enum (healthy/degraded/unhealthy/unknown). Liveness, readiness, status endpoints. Startup grace period to avoid false-not-ready during boot.
- **Profiling** (`internal/profiling/`) — pprof endpoints (CPU, memory, goroutine, mutex). Default off; opt-in.
- **Grafana dashboards** (`deploy/grafana/dashboards/`) — JSON exports for: Control Plane Health, Agent Fleet, State Mgmt, Policy Compliance, GitOps Ops, NATS, Audit, Module System, Event System, Secrets, Remote Execution, Multi-Env. Datasource templating (env, datacenter).

**post-v1.0 — TUI monitor** (`cmd/kscore-monitor/`):
- Bubble Tea / lipgloss / bubbles.
- Views (8 base in post-v1.0 baseline; 13 once enhancements ship): dashboard, agents, events, state-drift, policy-violations, jobs, logs, metrics. post-v1.0 enhancements: cluster, secrets/leases, schedules, runbooks, webhooks.
- Drill-downs (agent detail, job output streaming, event correlation).
- Vim navigation (`j`/`k`/`gg`/`G`/`Ctrl-d`/`Ctrl-u`); Tab/Shift-Tab between views; `?` help; `/` search; themes (dark/light/solarized/monokai).
- Persistent alert bar at top with critical counts.
- Connection-health indicators (gRPC + NATS).
- gRPC client multiplexed across 6 services; NATS subscriber for realtime; REST for runbooks/schedules/webhooks. Per-view refresh rate.
- Optional `--export` mode for capture-and-exit pipelines.

**post-v1.0 — Telemetry gateway** (`cmd/kscore-telemetry-gateway/`, `internal/gateway/`):
- Subscribes to NATS subjects: `kscore.{cluster}.telemetry.metrics.>`, `.logs.>`, `.traces.>`, `.audit.>`.
- Aggregates into in-memory store; exposes Prom `/metrics` + remote-write + Loki push + OTLP traces.
- HA via JetStream queue groups + leader election; deduplicates on remote-write.
- Helm chart for K8s deployment.
- Rationale: enables agents-behind-NAT or air-gapped fleets to feed observability backends without inbound HTTP.

**Subject hierarchy (post-v1.0 NATS telemetry)**:
```
kscore.{cluster}.telemetry.logs.{source}.{level}
kscore.{cluster}.telemetry.metrics.{source}
kscore.{cluster}.telemetry.traces.{source}
kscore.{cluster}.telemetry.audit.{source}.{action_type}
```

**Gotchas**:
- Cardinality explosion is real — limiter is mandatory, monitor `kscore_metrics_cardinality_total`.
- Sampling at 100% adds 5-10% latency at scale. Default to `probabilistic 0.1` with per-span rules upgrading on errors.
- Health check timeouts must be tight; circular dependencies (a check that calls into the component being checked) deadlock.
- TUI rendering at 10k+ agents needs pagination/lazy load (in post-v1.0 design).
- NATS telemetry buffer overflow policy: default `drop_oldest` — surface buffer-depth metric.

### 4.17 Blueprints & Runbooks

**Purpose**: Two related composability layers — pre-packaged state collections (blueprints, Salt-formula-shaped) and trigger-based workflow automation (runbooks).

**Blueprints** (`internal/blueprint/`):
- `Manifest{Metadata, Compatibility, Dependencies (requires, requires_before), Features, Entrypoints (default, rollback, named), Parameters (JSON-Schema), Outputs, Hooks (pre_apply, post_apply, pre_rollback, post_rollback), SourcePath}`.
- Loader → parses `blueprint.yaml`; validates manifest.
- Parameter validation against JSON-Schema; coercion (string→int/bool); `sensitive: true, source: secret` triggers credential lookup via Secrets Broker.
- Dependency resolver builds DAG; cycle detection; soft (`requires_before` — apply-before-me) vs hard (`requires` — must-be-installed) edges.
- Feature flag evaluation (`features:` block; conditional state inclusion).
- Multi-instance: `as: <namespace>` deploys same blueprint twice with namespaced state names.
- Template rendering (Go templates) with parameter context.
- Apply: invokes State Runner with the rendered state collection.
- Hooks run as runbooks (pre_apply / post_apply / pre_rollback / post_rollback).

**v1.0 standard catalog** (6 blueprints):
- `demo` — single-node demo deployment.
- `production-cluster` — 3-node HA control plane with embedded etcd, Postgres, NATS cluster.
- `monitoring-stack` — Prometheus + Grafana + Loki.
- `security-baseline` — CIS-aligned host hardening.
- `postgres-ha` — Postgres + WAL replication + monitoring.
- `nats-cluster` — NATS cluster with JetStream.

**post-v1.0 catalog expansion**: + `enterprise-platform`, `kubernetes-operator`, `identity-federation`, `gitops-integration`, `proxy-agents`, `file-distribution`, `edge-deployment`, `metrics-only` → 14 total.

**Runbooks** (`internal/runbook/`):
- `Runbook{Metadata{Name, Namespace, Version, Labels, Annotations}, Spec{Inputs, Steps, OnSuccess, OnFailure, Timeout, MaxRetries}}`.
- `Step{Type, Name, Description, DependsOn, Condition, Timeout, Retries, Config}`.
- **v1.0 step types** (subset, ~9): `command`, `api`, `state`, `notification`, `wait`, `noop`, `fail`, `script`, `query`.
- **post-v1.0 step types**: `if`, `switch`, `loop`, `parallel`, `sub-runbook`, `dryrun`.
- **post-v1.0 step types**: `approval`, `prompt`, `wait-manual`, `confirm`, `rollback`.
- Variable templating between steps: `{{ steps.<name>.outputs.<field> }}`.
- Step dependency DAG; cycle detection; conditional pre-execution.
- Retries with exponential backoff per step.
- Audit trail per execution.

**Saga coordinator** (`pkg/saga/`):
- `Step{Name, Action func(ctx, data) → (data, error), Compensate func(ctx, data) → error}`.
- `Execution{ID, Name, Status, Data, Steps[], StartedAt, EndedAt, Error}`.
- v1.0: forward-execute steps; on first error, walk completed steps in **reverse** invoking `Compensate`. In-memory or SQLite log.
- post-v1.0 advanced: checkpoint-resume (`pkg/saga/log_sqlite` persists between steps); compensation aggregation on multi-step failure.

**StateMachine library** (`pkg/statemachine/`):
- `Machine[S, E]{Builder, States, Transitions, Guards, Callbacks (OnEnter, OnExit, OnTransition), History, Metrics, Checkpointer (optional)}`.
- Used internally by: rollback engine, promotion engine, schedule executor, runbook executor.

**Schedules** (`internal/schedule/`, **post-v1.0**):
- `Schedule{Type (command|state|blueprint|reactor|custom), Cron|Interval, Timezone, TimeWindow{days, hours, excludes}, Target{agents, roles, tags, regions}, Payload, Status, Priority, MaxConcurrent, Timeout, RetryPolicy, MaintenanceWindowID, RequireApproval, NotifyBefore, Channels, StartDate, EndDate, Labels}`.
- `Execution{ID, ScheduleID, Status, TriggerType, ScheduledTime, StartTime, EndTime, AgentResults, Approvals}`.
- Two state machines: `ScheduleStateMachine` (active/paused/disabled/expired); `ExecutionStateMachine` (pending → approved → running → completed/failed/cancelled/timeout).
- Cron parsing via `robfig/cron/v3`. TZ via `time/tzdata`.

**APIs (v1.0)**:
- `kscore-blueprint` CLI: `init`, `validate`, `lint`, `info`, `install`, `apply`, `update`, `remove`, `applied`, `rollback`, `bundle`.
- `kscore-runbook` CLI: `list`, `execute`, `status`, `list-executions`, `audit`, `test`. (post-v1.0 adds `approvals`, `approve`, `reject`, `delegate`, `interventions`, `respond`.)
- REST (v1.0): `GET/POST /api/v1/runbooks`, `GET /api/v1/executions/{id}`. (post-v1.0 adds approvals/interventions.)
- Schedule APIs ship in post-v1.0.

**Gotchas**:
- Blueprint dependency cycles — detect at resolve time.
- Param coercion — invalid input must surface clearly, not silently coerce to zero value.
- Sensitive params — never log; mask in audit.
- Runbook variable scope — explicit reference required (`{{ steps.X.outputs.Y }}`); silent variables don't cross steps.
- Approval timeouts — must enforce or executions hang (post-v1.0 adds default expiry).
- DST/timezone math in schedules — `time/tzdata` or fail at config-validate time.
- Multi-instance namespacing — collision detection between namespaced and unnamespaced state names.
- Saga compensation ordering — reverse of completion order; failure of compensation = aggregate-and-continue (don't abort).

### 4.18 Plugin / Module System

**Purpose**: Salt-like extensibility on day 1. Lets sysadmins author safe, sandboxed modules in Starlark and ship them through a verified, reproducible distribution pipeline.

**v0.1 scope decision**: Starlark-only + Cosign verification + filesystem registry. WASM, OCI, SumDB, S3 deferred. This delivers the *experience* of a real module system without the long tail.

**Manifest format** (`pkg/module/manifest/`):
```yaml
name: vendor/pkg_apt          # namespaced
version: 1.2.3                # semver
type: starlark                # v1.0; wasm post-v1.0
entrypoint: main.star
description: APT package management
author: vendor
license: Apache-2.0
capabilities:
  fs.read:
    paths: [/etc/apt/**, /var/lib/apt/**]
    max_file_size: 10MB
  fs.write:
    paths: [/etc/apt/sources.list.d/**]
  exec:
    commands: [apt-get, dpkg]
    timeout: 60s
  log: {rate_limit: 100/s}
limits:
  timeout: 5m
  memory: 64MB
  cpu: 0.5
dependencies:
  vendor/pkg_common: ^1.0.0
```

**Capabilities (v1.0 — 9 core)**: `fs.read`, `fs.write`, `http.get`, `http.post`, `exec`, `secrets.read`, `secrets.write`, `kv`, `log`. Each scoped (paths, domains, commands, secret-paths, rate limits, timeouts). post-v1.0 adds per-syscall granularity via seccomp/eBPF on Linux.

**Verification pipeline** (`pkg/module/verify/`):
- Hash: SHA-256 content addressing; CAS storage `~/.kscore/modules/<hash>/`.
- Cosign signature: `cosign_test.go`-shaped; RSA, ECDSA, Ed25519; `KeyID` for rotation.
- Trust policy (v1.0 baseline): TLS-trusted registry + Cosign signature. post-v1.0 adds SumDB transparency log.

**Resolver** (`pkg/module/resolver/`):
- Recursive dependency resolution against semver constraints (`>=1.0 <2.0`, `^1.5.0`, `~1.2.3`).
- DAG construction with cycle detection (`HasCycle()`).
- Conflict resolution via **MVS** (Minimum Version Selection — Go modules pattern).
- Generates `module.lock` (sorted by name; deterministic).

**Lockfile**:
```yaml
schema_version: 1
modules:
  vendor/pkg_apt:
    version: 1.2.3
    hash: sha256:abc...
  vendor/pkg_common:
    version: 1.0.5
    hash: sha256:def...
```

**Registry (v1.0 — filesystem-backed)** (`pkg/module/registry/`, `cmd/kscore-registry/`):
- Go-mod-style HTTP endpoints:
  - `GET /<module>/@v/list` — list versions.
  - `GET /<module>/@v/<ver>.info` — version metadata (`{Version, Time}`).
  - `GET /<module>/@v/<ver>.mod` — manifest YAML.
  - `GET /<module>/@v/<ver>.zip` — module ZIP.
- Storage backend interface (filesystem in v1.0; S3, OCI, NATS Object Store in post-v1.0).
- post-v1.0 adds `GET /sumdb/lookup/<module>@<ver>` for transparency log.

**Loader pipeline** (`pkg/module/loader/`):
1. Parse manifest.
2. Verify (hash, signature) — unless `SkipVerification`.
3. Policy check (manifest vs policy) — unless `SkipPolicyValidation`.
4. Capability policy evaluation (per-capability OPA/CEL eval).
5. Capability lock check — if `PreviousCapabilities` set, reject if a capability was previously granted but is now denied (prevents capability removal mid-run).
6. Initialize runtime (Starlark/WASM).
7. Register **only granted** capabilities.

**Audit** (`pkg/module/audit/`):
- Every capability invocation: `AuditEntry{Timestamp, Module, Version, Capability, Operation, Success, Duration, Details}`.
- `CapabilityInvoker` wraps the call; emit entry; integrate with §4.12 audit log.

**SDK (v1.0)**:
- Starlark — `modules/sdk/starlark/`. Host capability bindings exposed as Starlark builtins. Examples included.

**SDK (post-v1.0)**:
- Rust — `modules/sdk/rust/` Cargo crate (WASI bindings).
- Go (TinyGo) — `modules/sdk/go/`.
- C++ — v2.x+ (`modules/sdk/cpp/`).

**CLI** (`cmd/kscore-module`): `init`, `build`, `validate`, `resolve`, `verify`, `sign`, `test`, `publish`, `install`, `update`, `clean`, `tree`. post-v1.0 adds `mirror` for air-gap.

**Plugin discovery** (`pkg/plugin/`):
- `Discovery.Discover()` scans `$PATH` for `kscore-*` binaries.
- `Executor.Execute()` runs them via `exec.Command`; stdin/stdout/stderr piping; context cancellation.
- This is what makes `kscorectl <foo>` dispatch to `kscore-foo` (Git-style plugin pattern).

**Gotchas**:
- Sandbox escape — defense in depth: runtime limits + capability scoping + policy + audit.
- Capability creep — defaults are permissive for ergonomics. Production policies tighten.
- Signature key rotation — multiple trusted keys + transparency log (post-v1.0) for tamper detection.
- Determinism violations — Starlark `random()` and `time.now()` disabled by default (capability gate).
- Lock-file drift — validate on every load; CI/CD must check.
- WASM (post-v1.0) — Go-WASM modules share Go heap; consider OOM blast radius.
- Namespace squatting — registry enforces namespaced names; validation on publish.

### 4.19 Multi-Environment

**Purpose**: Universal agent capability (Linux + IPv6 dual-stack v1.0); incremental expansion to Windows, K8s, container runtimes, cloud metadata, service mesh, edge.

**v1.0 baseline (Linux + foundations)**:
- `internal/platform/` — `Detector{Detect(), DetectOS, DetectDistro, DetectArch, DetectPackageManager, DetectInitSystem}`. `Info{OSType (linux|windows|darwin|bsd), DistroType, ArchType, PackageManager, InitSystem, Virtualization}`. `/etc/os-release`-driven Linux distro detection with fallback heuristics.
- `internal/hardware/` — `Detector{Detect}`. `CPUInfo`, `MemoryInfo`, `DiskInfo`, `NetworkInfo`, `SystemInfo`, `BMCInfo`. `gopsutil`-backed.
- `internal/netutil/` — `ParseAddress(s)`, `ParseURL(s)`, `AddressFamily` enum (`IPv4`, `IPv6`, `DualStack`), `AddressFamilyPreference` (`prefer_ipv4|prefer_ipv6|ipv4_only|ipv6_only`).
- IPv6 bracketing — server / agent / NATS / etcd / Postgres all use `[::]:<port>` or `[::1]:<port>` consistently. Helper functions in `netutil` ensure correctness.
- `internal/cloud/` (stub in v1.0) — minimal "are we in cloud?" probe via AWS IMDSv2 / GCP `Metadata-Flavor: Google` / Azure MSI; full metadata extraction in post-v1.0.

**post-v1.0 (Windows agent)**:
- Native service via SCM; auto-start; recovery options.
- Windows Event Log integration (`golang.org/x/sys/windows/svc/eventlog`).
- PowerShell 5.1+ and 7+ execution; script bypass policy where allowed.
- Registry management (read/write keys with type coercion).
- Windows-specific stdlib modules (see §4.8): `win_service`, `win_feature`, `win_firewall`, `win_registry`, `win_package`.

**post-v1.0 (container runtime detection)**:
- `internal/container/` — `Detector` identifies Docker/containerd/CRI-O/Podman via socket query + cgroup parsing.
- `Metadata{ContainerID, Image, Labels, Env, Volumes, NetworkConfig}`, `ResourceLimits`, `HealthStatus`.

**post-v1.0 (Kubernetes operator)** (`internal/k8s/`, Epic 48):
- CRDs: `RemoteExecution{Spec{Targets, Command, Schedule (cron)}, Status{Phase, AgentResults}}`, `StateConfig{Spec{State YAML, ReconcileInterval, AutoRemediate}, Status{DriftDetected, LastReconcile}}`.
- Controllers: `RemoteExecutionController`, `StateConfigController` — work-queue + informers + periodic safety-net reconcile.
- `ClientInterface` wraps `k8s.io/client-go`; pod exec with streaming output; pod selectors (labels/fields/names).
- ClusterRole + ClusterRoleBinding for `keystonecore.io` resources.
- Embedded into `kscore-server` via config `operator.enabled=true` OR standalone operator binary.
- Runs alongside cluster-mode CP; one-leader-runs-operator pattern.

**post-v1.0 (full cloud metadata)** (`internal/cloud/`):
- AWS: IMDSv2 token; instance ID/type, region, AZ, VPC, subnet, IAM, tags. Watch out: 21-byte max PUT body.
- GCP: `Metadata-Flavor: Google` header required; project, zone, instance, K8s SA.
- Azure: MSI token endpoint; RG, VM, MI.
- `K8sMetadata` (pod, namespace, SA from downward API + env), `ContainerMetadata` (image, ECS task def), `ServerlessMetadata` (Lambda function, version, runtime).
- Drives label-from-metadata targeting (`labels.cloud.region:us-east-1`).

**v2.x+ (service mesh, edge, DNS, advanced networking)**:
- `internal/servicemesh/` — `Detector` identifies Istio/Linkerd/Consul Connect/Kuma/OSM via well-known label/env patterns; SPIFFE extraction; proxy admin/health/stats; mTLS cert paths + min-version + provider.
- Edge agent mode (NATS leaf node + offline buffer + low-power telemetry).
- `internal/dns/` — `Record{Type, Name, Value, TTL, Priority, Proxied}`, `Metadata{Zone, Provider, Desired, Detected}`, `Diff{Create, Update, Delete, NoOp}`. Backed by `libdns/*` (Cloudflare, Route53, GCP DNS, Azure DNS, DigitalOcean, Hetzner, etc.).
- Advanced networking — WiFi (SSID/security/priority), 802.1X (EAP-TLS/TTLS/PEAP, RADIUS), link speed/duplex control, promiscuous mode, BMC/IPMI.

**Gotchas**:
- Distro detection on minimal containers — `/etc/os-release` may be missing.
- IMDSv2 token strict body size.
- Cgroup v1/v2 hybrid parsing for container detection.
- Service mesh detection heuristics (env vars, labels) can false-positive in custom setups.
- IPv6 zone IDs (`fe80::1%eth0`) — limited support in stdlib URL parsers; document.
- DNS provider rate limits vary widely (Cloudflare 1200/min vs Route53 5/sec).

### 4.20 Specialized & Extension Domains

**v0.1 in scope**:

**File distribution (basic)** (`internal/files/`, `cmd/kscore-files/`):
- `FileRequest`, `FileMetadata{Path, Size, Hash, ContentType, CreatedAt, Version, Tags}`, `FileChunk{ID, FileID, Index, Total, Data, Hash}`.
- Chunked streaming (1 MB default); SHA-256 per chunk; resume on interrupt.
- v1.0 backends: local filesystem, S3-compatible (AWS / MinIO).
- Proxy caching (LRU + TTL on agents).
- post-v1.0: NATS Object Store, Git, Azure Blob, GCS; mirror groups; sync engine.

**Self-management (basic)** (`internal/selfmgmt/`, `internal/backup/`, `cmd/kscore-backup/`, `cmd/kscore-bootstrap/`):
- `SeedConfig` — YAML for bootstrap-from-scratch (`kscore-bootstrap --seed config.yaml`). Installs binaries, forms cluster, hands off to self-management state.
- `BackupManager` — full snapshot: Postgres dump + JetStream + etcd + config + secrets, age-encrypted (master key from env or KMS).
- `RestoreManager` — portable artifact verification + partial restore + schema-compatibility check.
- `kscore-backup create|list|restore|verify` CLI.
- post-v1.0: scheduled backup, rolling upgrades, drift detection on self-config, self-healing.

**Basic rate limiting** (`internal/ratelimit/`):
- Token bucket; per-IP / per-API-key / per-header configurable.
- Rate-limit response headers (`Retry-After`).
- Wired into middleware chain (§4.4).

**v2.x+ deferred**:

**Proxy agents** (`internal/proxy/`, `internal/protocols/`, `internal/vendors/`, `cmd/kscore-proxy/`):
- `ProtocolAdapter` interface — `Connect`, `Execute`, `Disconnect`, `Health`.
- Protocols (v2.x+): SSH, SNMP v2c/v3, REST/HTTP. (v2.x: WinRM, NETCONF, RESTCONF, gNMI, Telnet.)
- Vendor adapters (v2.x+): Cisco IOS/NX-OS, Juniper JUNOS, Arista EOS, pfSense, OPNsense, VyOS. (v2.x: 14 more.)
- Device discovery (SNMP/SSH/LLDP/CDP).
- 100+ devices per proxy with sub-500ms command latency target.
- Credential proxy via NATS with X25519-encrypted exchange.

**Air-gapped deployments** (`internal/airgap/`, `cmd/kscore-transfer/`):
- post-v1.0 baseline: offline registry, bootstrap packages, upgrade archives.
- v3+: UDP data diode (one-way + FEC) for classified networks.

**Federation** (`internal/sync/`, `cmd/kscore-federation/`):
- v2.x+: SPIFFE trust-domain federation; trust bundle exchange; cross-domain identity validation.

**MCP server** (`internal/mcp/`, `cmd/kscore-mcp/`):
- v2.x+: 16 tools (agent_list, exec_run, state_check, runbook_execute, etc.); 3 resources (agents, cluster, events).
- Capability profiles: `read_only` (13), `ops_safe` (+exec/runbook), `ops_admin` (+state apply).
- Credential pass-through; MCP metadata in audit log (client_type, tool, session, ai_client) for accountability.

---

## 5. Cross-Cutting Concerns

### 5.1 Configuration

- Single `Config` struct grown by epic. Foundations (Epic 01) ships `Mode`, `Server`, `Logging`, `Storage`. Each later epic adds its own sub-config (NATS, Auth, Policy, GitOps, Secrets, Events, Execution, StateManagement, Webhook, CORS, RateLimit, Metrics, Tracing, Health, Profiling, Cluster, Operator) with its own `Validate()` and production-warning entries.
- Loaded via koanf from YAML + env (`KSCORE_` prefix). Single-word koanf keys (e.g., `grpcport`, `httpport`, `certfile`) keep env mapping unambiguous: `server.grpcport` ↔ `KSCORE_SERVER_GRPCPORT`.
- Validation is **post-unmarshal**. Each sub-config implements `Validate() error`; the root `Config.Validate()` calls them all.
- Production warnings emitted on startup for: embedded NATS in production mode, SQLite in production mode, TLS off in production mode, JetStream at default storage limits.

### 5.2 Logging, Errors, Correlation

- `internal/logging` produces `log/slog`-backed structured logs; correlation IDs flow via context, gRPC metadata, NATS headers, span attributes.
- Errors use `pkg/api/apierror.Response` for REST and `status.Error(codes.X, msg)` for gRPC. `apierror.StatusCode()` translates between them; the two must stay in sync.
- Sensitive data is masked at the log layer via a `LogMasker` regex set (secrets, tokens, API keys, passwords).
- Audit log (§4.12) is the structured trail for sensitive ops; logs are for engineering ops.

### 5.3 Testing Strategy

- **Unit tests** — every package; `t.TempDir()` for files; in-memory implementations for stores; `httptest` for HTTP.
- **Integration tests** — opt-in via `-tags integration`; embedded NATS, real Postgres in docker-compose.
- **E2E (v1.0 baseline — single topology)** — `test/e2e/`: docker-compose with kscore-server + kscore-agent + Postgres + NATS. Tests: agent registration, command exec, state apply, drift detection.
- **HA E2E (v1.0 must-pass)** — `test/e2e/ha_*`: 3-node cluster formation, failover, network partition, split-brain prevention. Performance SLOs verified (cluster <10s, leader <3s, failover <5s).
- **Smoke** — `scripts/smoke-test.sh quick` (pre-commit); SQLite + embedded NATS only.
- Coverage targets: critical packages >70%, CLI packages >40%.

### 5.4 Build, Release, & Supply Chain

- `Makefile` is the orchestrator. CI never runs raw `go test`/`go build` — always Make targets.
- All builds `CGO_ENABLED=0`. Pure-Go SQLite (`modernc.org/sqlite`) and Postgres (`lib/pq`).
- Version metadata injected via `-ldflags -X`: `pkg/version.Version`, `.GitCommit`, `.BuildDate`.
- Goreleaser snapshot for v1.0 (multi-arch tarballs). post-v1.0: signing ceremony + multi-party process per `RELEASE-PLAYBOOK.md`. post-v1.0: DNF/APT/MSI repos.
- Buf: `buf.yaml` STANDARD lint with documented exclusions; `buf breaking` against `main`; `buf generate` outputs to `pkg/api/v1/`.
- Pre-commit: gofmt, golangci-lint, smoke test.
- Security baseline (v1.0): gitleaks (secrets), govulncheck (CVE), gosec (SAST). post-v1.0 adds semgrep, trivy, syft (SBOM), grype, hadolint.

### 5.5 Documentation

- v1.0: README + reference docs (CLI, API, configuration) + RELEASE-PLAYBOOK + SECURITY + AGENTS.
- post-v1.0: Hugo + Docsy site (`docs/` build to `docs/public/`).
- Glossary, governance, RFC process, AI-contributions policy, DCO — port forward as-is.

### 5.6 Governance Carry-Overs

- BDFL model with technical-meritocracy maintainers. RFCs for major changes.
- Apache 2.0 license; DCO sign-off mandatory.
- AI-assisted contribution policy (disclosure, no copyright on purely AI code, accountable maintainer review).

---

## 6. Versioning Strategy & MVP Rationale

### 6.1 Strategy

- **SemVer 2.0.0**. Breaking change → major. Feature → minor. Bug fix → patch.
- 6-month release cadence (per `COMPATIBILITY.md`); 2-year support window per major.
- v1.0 is the **commercial-trial-ready MVP** (clusterable + sysadmin-rich).
- v1.x cadence is granular (~1 release every 6-8 weeks) to ship value continuously without forcing breaking changes.
- **No breaking changes within v1.x**. Anything that breaks compatibility waits for v2.0.
- Deprecation: `pkg/api/versioning` registry tracks `Status{current, supported, deprecated, retired}` + `DeprecatedAt`, `SunsetAt`. Min one minor-release of warning before retirement.

### 6.2 Release Roadmap (the headline)

**The versioning scheme + per-milestone gate definitions live in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md)** — that's the canonical reference. This section summarises the headline progression.

| Line | Identity | What's in it |
|---|---|---|
| **`v0.1.x`** | First public release. **Linux-only, internal-quality.** | Epics 01–08 complete (control plane + agent + exec + state engine + 35-module base stdlib + saga + integration test). Breaking changes between patches permitted. |
| **`v0.2.x` – `v0.4.x`** | Incremental, release-as-ready. | Closes one or more epics each. Identity/auth, secrets, events, audit/policy, clustering/HA — order driven by ranked [ROADMAP.md](docs/project/ROADMAP.md), not pre-allocation. |
| **`v0.5.x`** | **External-tester milestone.** | All major Linux distro families pass the cross-distro CI matrix. Persistence renderers (netplan + networkd minimum). All four firewall backends solid on their native distros. apt + dnf + apk in `package`. AppArmor in `security`. Cross-distro reboot detection. Filesystem-resize + LV resize. Epics 09 + 11 minimum. Full v0.5 checklist in VERSIONING.md. |
| **`v0.6.x` – `v0.9.x`** | Polish + remaining epic work toward v1.0. | Epics 14–19 land here in some order. Breaking changes possible but rare; each documented. Renderers expand (NetworkManager, RHEL ifcfg). zypper added. SUSE support solidifies. |
| **`v1.0.0`** | **SemVer stability commitment.** | All 19 epics complete and acceptance-tested. API + CLI + state-file YAML + gRPC frozen for one full v0.x cycle prior. Cross-distro CI green on the full applicable-module set. Performance + security baselines. Hugo docs site live. Migration tooling from v0.x. Full v1.0 checklist in VERSIONING.md. |
| **`v1.x`** | Feature additions on the stable v1.0 line. | Windows agent, WASM runtime, TUI, K8s operator + CRDs, SPIRE identity, runbook approvals, telemetry gateway + cloud metadata + saga checkpoint-resume, catalog expansion, compliance scans + DB stdlib, air-gap + supply chain, policy enforcement modes, macOS / BMC / edge baseline. Order driven by post-v1.0 priorities. |
| **`v2.0` / `v2.x`** | **Federation, multi-region, supercluster, cloud KMS.** | NATS supercluster + leaf nodes + WebSocket + auto-discovery, federation, cloud KMS for secrets, AWS Secrets Manager + Azure Key Vault + GCP Secret Manager, DNS provider mgmt, advanced networking, proxy agents core (SSH+SNMP+REST + 6 vendors), MCP server. Remaining proxy protocols + 14 more vendors, service mesh, edge agent mode in v2.x. |
| **`v3.0+`** | **Marketplace, Web UI, multi-cloud.** | Blueprint marketplace, browser-based management console, multi-cloud test matrix, UDP data diode. |

**Why this shape:** the original roadmap pre-allocated every minor release of v1.x (v1.1 = Windows+WASM, v1.2 = TUI, ...). That commitment hardens decisions that should still be revocable. The post-v0.5, pre-v1.0 work is now a single ranked backlog; v1.x post-v1.0 reorders as priorities shift. v2.x+ identity stays anchored because federation + marketplace are architectural commitments.

### 6.3 Why v0.1 / v0.5 / v1.0 Are What They Are — MVP Reasoning Recap

**v0.1 — internal-quality first release** is what's landing now and through the next several v0.x cuts. The shape it has:

1. **Sysadmin/IT-admin trial users**, not platform engineers. They need to feel at home: `kscorectl exec 'uptime' --target role:web` works; state files apply with familiar primitives (file/package/service/user); blueprints look like Salt formulas; modules can be authored without a build chain.
2. **~90% of daily sysadmin work covered**: 35 stdlib modules across system/scheduling/storage/network/firewall/SSH/system-config/files/certs. Many ship in their *minimum useful* form (runtime-only network, apt-only package, SELinux-only security, etc.); the remaining backends + persistence land en route to v0.5.
3. **Clusterable, not single-server**. 3-node HA from day 1 (Epic 13); embedded etcd; consistent-hash shards; sub-5s failover. *This is the commercial-trial differentiator.*
4. **Audit + secrets + identity are real, not stubs** — Epics 09 (identity/auth), 10 (secrets), 12 (audit/policy). Compliance-curious users running v0.5+ can audit-query in week one. Embedded CA + API keys + mTLS + JWT cover real deployments.
5. **Plugin/module system** (Epic 14, Starlark-only). Salt's extensibility on day 1 is the explicit ask. Starlark-only keeps scope sane (~8 weeks engineering vs 14+ for full stack); WASM/OCI/SumDB defer to v1.x.
6. **Policy enforcement is post-v1.0** (per direction). The v0.x → v1.0 line ships audit-mode-only. The engine, policies, evaluators, audit log, compliance reports all work — the engine just doesn't *block*. Enforcement modes flip the switch in v1.x with workflow infra ready.
7. **Observability minimum** (Epic 17) = Prom metrics + OTel traces + structured logs + Grafana JSONs. TUI + telemetry gateway are post-v1.0.
8. **Defer platform breadth, not depth**. Linux + IPv6 universal in v0.1 → v1.0; Windows / K8s operator / macOS all land post-v1.0 in v1.x. The state engine is universal; the platforms come in waves.

**v0.5 narrows the gap** between "Linux works at all" (v0.1) and "Linux works for external testers" (v0.5): cross-distro CI matrix green, persistence renderers in place, all firewall backends solid, cross-distro reboot detection. See [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) for the explicit gate.

**v1.0 is the SemVer stability commitment** — all 19 epics complete, contracts frozen, full test matrix, security audit, migration tooling. Once cut, breaking changes require a v2.0.

---

## 7. Reconstruction Order

Build sequence for the rebuild. Each step has a clear "definition of done." Track in `epics/`.

### Phase A — Foundations (weeks 1-2)
1. **Project scaffold**: repo layout, `go.mod`, Makefile, Buf, golangci-lint, pre-commit. Hello-world for `kscore-server`, `kscore-agent`, `kscorectl`.
2. **`pkg/version`, `pkg/semver`, `pkg/wait`, `pkg/dbutil`, `pkg/api/apierror`** — utility packages.
3. **`internal/config`** — koanf-based loader; per-sub-config `Validate()`; `ProductionWarnings()`. Foundations ships `Mode`/`Server`/`Logging`/`Storage`; later epics extend.
4. **`internal/logging`** — `log/slog`-backed; correlation IDs via wrapping handler.
5. **Storage layer** — `internal/state`, SQLite + Postgres backends; auto-DDL; basic CRUD for agents/commands.

### Phase B — API & wire format (weeks 3-4)
6. **Protos** — agent, controlplane, state, event, policy, secrets, cluster, coordination protos. Buf generate.
7. **`pkg/api/auth`, `pkg/api/apikeys`, `pkg/api/rbac`** — auth interceptors, Principal model, role hierarchy.
8. **`pkg/api/server`** — HTTP + gRPC server bootstrap; dual-stack listeners; middleware chain.

### Phase C — Messaging (weeks 4-5, parallel with B)
9. **NATS embedded mode** — Manager, Connection state machine, subject builder with cluster prefix.
10. **NATS external mode** — multi-endpoint, circuit breaker, JetStream stream definitions.
11. **Bootstrap registration flow** — agent ↔ server credential exchange.

### Phase D — Agent runtime (weeks 5-6)
12. **`kscore-agent` daemon** — registration, heartbeat, metadata, command exec.
13. **Agent bootstrap** — TUI + non-interactive; phases (detect/configure/validate/install/verify/blueprints).

### Phase E — Control plane services (weeks 7-9)
14. **Connection Manager + Command Dispatcher + Batch Dispatcher**.
15. **AgentService, ControlPlaneService** wiring.
16. **State management engine** — Module interface, runner pipeline, requisite resolver, drift detector.
17. **40 stdlib modules** (incremental) — file/package/service/user/group → cron/mount/firewall → network/ssh → x509/git/archive.

### Phase F — Identity & secrets (weeks 9-11)
18. **Embedded CA + identity Provider** — SVID issuer, attestation, token store.
19. **Cluster join tokens**.
20. **Secrets broker + encrypted-file backend + Vault backend + lease manager + transit ops**.

### Phase G — Events, audit, policy infra (weeks 11-12)
21. **Event bus + EventStore + EventService**.
22. **Audit log infrastructure**.
23. **Policy engine (audit-mode)** — OPA + CEL + Builtin evaluators; PolicyService.

### Phase H — Clustering & HA (weeks 13-15) **[the v1 differentiator]**
24. **etcd integration (embedded + external)**.
25. **MembershipManager + LeaderElector + ShardManager + HealthMonitor**.
26. **FailoverManager + RecoveryManager + FencingManager + SingletonTaskManager**.
27. **CoordinationService (mTLS)**.
28. **Cluster backup/restore**.
29. **HA E2E tests** (failover, partition, split-brain).

### Phase I — Plugin / module system (weeks 16-18)
30. **Starlark runtime + capability invoker + audit**.
31. **Manifest + lockfile + resolver + filesystem registry + Cosign verifier**.
32. **`kscore-module` CLI + Starlark SDK + module test framework**.

### Phase J — Blueprints & runbooks (weeks 19-20)
33. **Blueprint engine + 6-blueprint standard catalog**.
34. **Runbook engine (v1.0 step subset)**.
35. **Saga coordinator (minimal) + StateMachine library**.

### Phase K — GitOps & webhooks (weeks 20-21)
36. **GitOps webhook receiver + auth + 4 source handlers**.
37. **Verification engine (HTTP + gRPC + command)**.
38. **Rollback engine (Git revert + ArgoCD sync + K8s undo) + approval gates**.
39. **Outbound webhook subscriptions + dispatcher + circuit breaker**.

### Phase L — Observability & ops (weeks 22-23)
40. **Prom metrics registry + cardinality limiter + `/metrics` endpoint**.
41. **OTel tracing + sampling + exporters**.
42. **Health endpoints + Grafana dashboards**.
43. **`kscore-backup` (basic) + `kscore-bootstrap` from seed**.
44. **Rate limiter + middleware**.

### Phase M — File distribution & misc (weeks 23-24)
45. **File distribution (NATS chunked + filesystem + S3 backends + proxy cache)**.

### Phase N — Test, harden, release (weeks 25-26)
46. **HA resilience tests pass on every PR**.
47. **Performance SLOs verified (cluster <10s, leader <3s, failover <5s)**.
48. **Security baseline scans clean (gitleaks + govulncheck + gosec)**.
49. **Documentation pass — README, reference docs, RELEASE-PLAYBOOK, SECURITY**.
50. **v1.0.0 release via goreleaser snapshot**.

**Total: ~38 engineering-weeks for v1.0** (parallelizable after Phase A; with a 4-engineer team and good parallelism, ~10-12 calendar weeks). The detailed per-epic estimates live in `epics/00-meta-reconstruction-plan.md`.

---

