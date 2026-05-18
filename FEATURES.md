# FEATURES.md

Complete feature inventory for Keystone Core, organized by domain. Each feature carries a **priority-bucket** tag (`v1.0`, `v1.x`, `v2.x+`, `v3.x+/future`) and a one-line reasoning note.

> **Versioning note.** Tags here are **priority buckets, not pinned minor releases.** [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) is canonical: there is deliberately *no* pre-allocated table of v0.2/v0.3/v1.1/v1.8 contents. Bucket meanings:
> - **`v1.0`** — in the v0.x → v1.0 MVP scope; lands incrementally across the v0.x line, frozen at the v1.0 SemVer commitment.
> - **`v1.x`** — post-v1.0 additive features on the stable line. Not scheduled to a specific minor.
> - **`v2.x+`** — architectural post-v1.0 (federation, multi-region, supercluster, cloud KMS).
> - **`v3.x+/future`** — marketplace, web UI, multi-cloud.
>
> When precise sequencing matters, defer to the ranked entries in [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) (`gate-v0.5` | `gate-v1.0` | `v0.x` | `v1.x` | `v2.x+`). See PROJECT-DETAILS.md §6 for the version-strategy summary.

> **MVP target.** v1.0 must be **clusterable** and **commercial-trial-ready** for sysadmin/IT-admin users — i.e. ~90% of what they need for day-to-day work. This drives several capabilities (plugin/module system, base stdlib, real auth, audit, HA, observability) into v1.0 that a leaner control-plane MVP would defer. The work lands incrementally across the v0.x line; v1.0 is the SemVer stability commitment at the end.

> **Source paths** are relative to the existing Keystone Core repo. They identify where the *current* implementation lives, not where the new build will put it.

---

## Domain Index

1. [Foundations & Build System](#1-foundations--build-system)
2. [NATS Messaging](#2-nats-messaging)
3. [Storage Layer](#3-storage-layer)
4. [Control Plane Core](#4-control-plane-core)
5. [API Surface (gRPC + REST)](#5-api-surface-grpc--rest)
6. [Agent Runtime](#6-agent-runtime) *(pending)*
7. [Remote Execution & Targeting](#7-remote-execution--targeting) *(pending)*
8. [State Management & Stdlib](#8-state-management--stdlib) *(pending)*
9. [Event System](#9-event-system) *(pending)*
10. [Identity & Auth](#10-identity--auth) *(pending)*
11. [Secrets Management](#11-secrets-management) *(pending)*
12. [Audit & Policy](#12-audit--policy) *(pending)*
13. [GitOps Integration](#13-gitops-integration) *(pending)*
14. [Outbound Webhooks](#14-outbound-webhooks) *(pending)*
15. [Clustering & HA](#15-clustering--ha) *(pending)*
16. [Observability](#16-observability) *(pending)*
17. [Blueprints & Runbooks](#17-blueprints--runbooks) *(pending)*
18. [Plugin / Module System](#18-plugin--module-system) *(pending)*
19. [Multi-Environment](#19-multi-environment) *(pending)*
20. [Specialized & Extension](#20-specialized--extension) *(pending)*

---

## 1. Foundations & Build System

### v1.0 (in scope)

- **Cross-platform builds** (linux/darwin/windows × amd64/arm64) via Makefile. Source: `Makefile`. _Reasoning: required for any commercial trial._
- **Pure-Go build (CGO_ENABLED=0)**. Source: `Makefile`, `pkg/dbutil/`. _Reasoning: enables Alpine/scratch images, simple cross-compile; non-negotiable for distribution._
- **Buf-based proto codegen** (Go + gRPC plugins). Source: `buf.yaml`, `buf.gen.yaml`. _Reasoning: API surface depends on it._
- **Build-time version injection** (`pkg/version`: Version, GitCommit, BuildDate). Source: `pkg/version/`, `Makefile` LDFLAGS. _Reasoning: support window + bug reports require it._
- **Semver library** (`pkg/semver`: Parse, constraints, Diff). Source: `pkg/semver/`. _Reasoning: needed by module system and upgrade logic in v1._
- **Cancelable wait/poll utilities** (`pkg/wait`). Source: `pkg/wait/`. _Reasoning: tiny, used everywhere._
- **koanf-based config** (YAML + env vars, `KSCORE_` prefix). Source: `internal/config/`. _Reasoning: clean strict-unmarshal semantics, lighter deps than Viper. Cobra remains for CLI parsing._
- **Structured logging** (`log/slog`, JSON/logfmt/text, correlation IDs). Source: `internal/logging/`. _Reasoning: zero-config observability baseline; stdlib eliminates a third-party dep._
- **Standard error model** (`pkg/api/apierror`: code, message, details). Source: `pkg/api/apierror/`. _Reasoning: REST + gRPC error consistency._
- **Make targets**: `build`, `test`, `lint`, `proto`, `dev` (hot-reload), `e2e-up/down`, `release-snapshot`. Source: `Makefile`. _Reasoning: minimum dev loop._
- **Goreleaser snapshot** (multi-arch tarballs). Source: `.goreleaser.yaml`. _Reasoning: distribution baseline._
- **Pre-commit hooks** (gofmt, golangci-lint, smoke). Source: `.pre-commit-config.yaml`. _Reasoning: quality gate._
- **Baseline lint set** (errcheck, govet, staticcheck, gosec, bodyclose). Source: `.golangci.yml`. _Reasoning: catches real bugs without slowing dev._
- **Single-topology E2E** (all-in-one docker-compose). Source: `test/e2e/`, Makefile. _Reasoning: smoke test for releases._

### v1.x (deferred)

- **Syslog logging output** [v1.x]. _Reasoning: ops nice-to-have; stdout suffices for trial users on systemd/k8s._
- **Documentation site** (Hugo + Docsy + PDF). Source: `docs/`. **[v1.x]**. _Reasoning: README + reference docs sufficient for v1.0; Hugo site adds tooling burden._
- **HA / IPv6 / HA+IPv6 E2E topologies** [v1.x]. _Reasoning: clustering ships in v0.1 but full topology matrix can land iteratively._
- **Hot-reload dev server (`air`)** [v1.0.x]. _Reasoning: dev-only, ship with v1.0 dot release._
- **Repository generation (DNF/APT/Windows MSI)** [v1.x]. _Reasoning: tarballs cover trial users; package repos require infra commitment._
- **VM bootstrap test harness** [v1.x]. _Reasoning: container E2E sufficient for v1.0._
- **Full security scanning suite** (semgrep, trivy, syft, grype, hadolint) [v1.x]. _Reasoning: gitleaks + govulncheck + gosec sufficient for v1.0._
- **Goreleaser signing ceremony / multi-party release** [v1.x]. _Reasoning: heavy ceremony documented in current `RELEASE-PLAYBOOK.md`; single-signer release acceptable for v1.0._
- **Benchmark suite** [v1.x]. _Reasoning: not commercial-trial-blocking._
- **Air-gapped repo packaging (`kscore-bootstrap`)** [v1.x]. _Reasoning: see Specialized domain._

---

## 2. NATS Messaging

### v1.0 (in scope)

- **Embedded NATS server** (in-process, zero-dependency dev path). Source: `internal/nats/`. _Reasoning: zero-config startup is a key UX promise._
- **External NATS cluster** (production HA path). Source: `internal/nats/`, config `nats.mode=external`. _Reasoning: required for cluster mode v1.0._
- **Subject hierarchy** (`kscore.{cluster}.{category}.{...}`). Source: `internal/nats/`. _Reasoning: even single-cluster v1 must use cluster-prefixed subjects so v2 supercluster doesn't require refactor._
- **Direct TCP + TLS connection strategies**. _Reasoning: covers ~95% of trial deployments._
- **Multi-endpoint failover with health checks**. _Reasoning: clusterable means agents survive a CP node going down._
- **Per-endpoint circuit breaker**. _Reasoning: prevents thundering herd on partial outages._
- **Message envelope** (MessageID, CorrelationID, priority, TTL). _Reasoning: dedup + req/resp correlation._
- **JetStream enablement** (embedded + external). _Reasoning: events + command persistence rely on it._
- **Bootstrap registration with minimal-permission credentials**. _Reasoning: security baseline; documented in Epic 17._
- **Connection state machine** (Disconnected → Connecting → Connected → Reconnecting). _Reasoning: deterministic agent reconnect behavior._
- **Static endpoint configuration**. _Reasoning: simple ops path; auto-discovery deferred._
- **Health check hooks** (Manager.Health, ConnectionManager.Health). _Reasoning: drives `/health/ready`._

### v1.x (deferred)

- **Leaf node mode** (edge / hierarchical). **[v2.x+]**. _Reasoning: edge use case isn't a trial-day-1 feature; deployment matrix grows considerably._
- **Supercluster / gateway** (multi-region). **[v2.x+]**. _Reasoning: same; multi-region is post-trial._
- **WebSocket / WSS transport** (firewall traversal). **[v2.x+]**. _Reasoning: NAT traversal via TLS-on-443 mostly suffices for v1; WSS is for strict-firewall enterprises._
- **Auto-discovery** (DNS-SRV, mDNS, K8s, Consul, etcd). **[v1.x]**. _Reasoning: static config workable at trial scale; K8s discovery lands when K8s operator does (v1.x)._
- **NAT traversal via reverse leaf** [v2.x+]. _Reasoning: niche._
- **Exactly-once delivery** [v2.x+]. _Reasoning: at-least-once + dedup window covers vast majority of cases._

---

## 3. Storage Layer

### v1.0 (in scope)

- **SQLite backend** (single-node dev/small). Source: `internal/state/`, `pkg/dbutil/`. _Reasoning: zero-config promise._
- **PostgreSQL backend** (multi-node HA). Source: `internal/state/`. _Reasoning: clusterable v1 requires shared storage; SQLite cannot serve multiple servers._
- **Pure-Go drivers** (`modernc.org/sqlite`, `lib/pq`). _Reasoning: CGO-free build constraint._
- **Auto-schema initialization** (CREATE TABLE IF NOT EXISTS on first start). Source: `internal/state/`. _Reasoning: trial UX — no migration step on first run._
- **Repository pattern** (Store, AgentStore, CommandStore, BatchJobStore, HealthStore interfaces). _Reasoning: decouples business logic from backend._
- **Direct parametrized SQL** (no ORM). _Reasoning: matches existing project style; predictable performance; simpler to reason about._
- **Connection pooling tuned per-backend** (SQLite: 1 writer; Postgres: configurable). _Reasoning: SQLite single-writer constraint is real._
- **JSON-encoded complex columns** (labels, env vars, IPs). _Reasoning: pragmatic; avoids schema bloat for sparse fields._
- **SQLite → PostgreSQL migration tool** (`kscore-migrate`). Source: `cmd/kscore-migrate/`, `internal/state/Migrator`. _Reasoning: trial users start on SQLite; need a no-data-loss path to scale up._
- **Migration features**: dry-run, batch-size, txlog, progress reporter, validation. _Reasoning: production-grade migration is table stakes._
- **IPv6-safe DSN building** (Postgres). _Reasoning: dual-stack v1 requirement._

### v1.x (deferred)

- **Schema versioning / golang-migrate** **[v1.x]**. _Reasoning: v1 schema is new and stable; auto-DDL fine; add when first breaking change is needed._
- **Encryption at rest** (KeyProvider, AES-GCM/CBC/ChaCha20) **[v1.x]**. _Reasoning: KeyProvider scaffolding may exist sooner, but full data-at-rest encryption with key rotation is a real project; commercial buyers expect it but not on day 1._
- **Multi-table transaction wrapper (`Tx`)** **[v1.x]**. _Reasoning: current per-method transactions cover the only multi-step op (CompleteBatchJob); add when consistency bugs surface._
- **Backup/restore as Store API methods** **[v1.x]**. _Reasoning: external `kscore-backup` CLI is fine for v1; integration is convenience._
- **Query backends — Loki/Prometheus/Jaeger integration** **[v1.x]** (lands with Telemetry Gateway). _Reasoning: in-memory stubs sufficient for unit testing v1._
- **Cloud KMS for storage encryption keys** **[v2.x+]**. _Reasoning: depends on cloud KMS work in secrets domain._

---

## 4. Control Plane Core

### v1.0 (in scope)

- **kscore-server daemon** with strict init order (NATS → State → ConnMgr → Dispatcher → gRPC/HTTP → optional). Source: `cmd/kscore-server/`, `internal/controlplane/`, `pkg/api/server/`. _Reasoning: deterministic startup is critical for ops._
- **Connection Manager** (agent registration, heartbeat tracking, stale detection). Source: `internal/controlplane/`. _Reasoning: core agent lifecycle._
- **Command Dispatcher** (route, timeout, result retention). _Reasoning: core remote-execution backbone._
- **Batch Dispatcher** (group-targeted ops, batch progress state machine). _Reasoning: required for fleet operations at any scale._
- **Listen ports**: gRPC 9090, HTTP 8080, optional metrics, optional pprof. _Reasoning: standard split._
- **Dual-stack (IPv4 + IPv6) listeners**. _Reasoning: minimal cost, broad applicability._
- **Health endpoints** (`/health/live`, `/health/ready`, `/health/status`, `/api/status`). _Reasoning: K8s probes + ops dashboards._
- **Middleware chain** (CORS → rate-limit → auth → handler). _Reasoning: standard ordering; auth as innermost so audit can log denials before handler._
- **Graceful shutdown sequence** (gRPC → ConnMgr → State → NATS, with HTTP context timeout). _Reasoning: data-loss on shutdown destroys trial user trust._
- **30s status ticker logging** (agent counts, health). _Reasoning: zero-config visibility into running server._
- **Production warnings on startup** (embedded NATS, SQLite, TLS off in production). _Reasoning: makes operational risk explicit._
- **Default zero-config startup** (embedded NATS + SQLite + dev API key + warning). _Reasoning: trial UX promise._

### v1.x (deferred)

- **gRPC reflection / channelz** **[v1.0.x]**. _Reasoning: ship with first dot release; trivial to add._
- **Webhook receiver port (8081)** **[v1.x]**. _Reasoning: see Webhooks domain — outbound is v1.0, inbound non-GitOps webhooks v1.x._
- **K8s operator wiring** **[v1.x]**. _Reasoning: gates on operator domain; not v1.0._
- **Profiling endpoint defaults-on** **[never]**. _Reasoning: leave opt-in; security default._

---

## 5. API Surface (gRPC + REST)

### v1.0 (in scope — services)

- **AgentService**: Register, Heartbeat, ExecuteCommand (stream), GetAgentInfo. Source: `api/proto/agent.proto`. _Reasoning: agent lifecycle._
- **ControlPlaneService**: GetServerStatus, ListAgents, GetAgent, ExecuteCommand (stream), BatchExecuteCommand (stream), command status/history. Source: `api/proto/controlplane.proto`. _Reasoning: orchestration._
- **StateService**: ApplyState (stream), CheckState, DetectDrift, GetStateHistory. _Reasoning: core IaC capability._
- **EventService**: ListEvents, EmitEvent, SubscribeEvents (stream), GetEventStats. _Reasoning: observability + audit trail._
- **PolicyService** (audit-only in v1.0): EvaluatePolicy, ListViolations, GetAuditLog, GetComplianceReport. _Reasoning: audit alone is valuable for compliance-curious trial users; full enforcement deferred._
- **SecretsService**: GetSecret, ListSecrets, WriteSecret, DeleteSecret, lease ops, transit encrypt/decrypt/sign/verify. _Reasoning: any sysadmin trial requires real secrets handling._
- **ClusterService**: GetClusterStatus, ListMembers, GetLeader, TransferLeader, WatchMembership (stream), WatchLeadership (stream). _Reasoning: clusterable v1._
- **CoordinationService** (mTLS-only server-to-server): ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, PropagateState. _Reasoning: cluster recovery during NATS partition._
- **AuthN**: API key (Bearer), JWT, mTLS — Principal model, Authorizer interface, role hierarchy (admin > operator > readonly). Source: `pkg/api/auth/`. _Reasoning: minimum auth set for commercial trial._
- **REST endpoints (v1.0 wired)**: `/health/*`, `/api/status`, `/api/v1/agents`, `/api/v1/commands`, `/api/v1/state/*`, `/api/v1/secrets/*`, `/api/v1/policies` (audit), `/api/v1/cluster/*`, `/api/v1/apikeys`, `/api/v1/audit`, `/api/v1/runbooks`. _Reasoning: REST is the on-ramp for ad-hoc tooling, dashboards, scripts._
- **Streaming patterns**: server-stream for command output, batch progress, state apply progress, event subscription, membership/leadership watch. _Reasoning: real-time UX matters._
- **Standardized pagination** (page_size + page_token + total_count). _Reasoning: large fleets._
- **Standard error model** (`pkg/api/apierror`). _Reasoning: ecosystem consistency._
- **Versioning registry** (`pkg/api/versioning`: current/supported/deprecated/retired/beta/alpha + dates). _Reasoning: support window enforcement._
- **gRPC ↔ REST mapping**: hand-coded REST handlers (no grpc-gateway annotations in protos). _Reasoning: matches existing structure; gateway adoption can be evaluated v2._

### v1.x (deferred)

- **MaintenanceService** (maintenance windows API) **[v1.x]**. _Reasoning: scheduling domain — see v1.x._
- **ScheduleService** (job scheduling API) **[v1.x]**. _Reasoning: same._
- **RunbookService gRPC** **[v1.0]** (REST wired in v1.0). _Reasoning: keep both for v1._
- **WebhookService** (REST handler exists, not wired) **[v1.0 outbound]** + **[v1.x inbound non-GitOps]**. _Reasoning: outbound is in v1.0; non-GitOps inbound subscriptions slide to v1.x._
- **GitOps webhook REST handlers wiring** **[v1.0]**. _Reasoning: GitOps domain — included in v0.1._
- **MirrorService / DiscoveryService** (proxy-related) **[v2.x+]**. _Reasoning: see Specialized._
- **Full RBAC (`pkg/api/rbac`)** **[v1.x]**. _Reasoning: simple admin/operator/readonly hierarchy ships v1.0; full RBAC is a separate epic._
- **gRPC-gateway adoption** (auto-generated REST) **[v2.x+]**. _Reasoning: hand-coded handlers work for v1; reduces churn during initial implementation._
- **OpenAPI auto-generation from protos** **[v2.x+]**. _Reasoning: hand-maintained YAML acceptable for v1._

---

## 6. Agent Runtime

### v1.0 (in scope)

- **`kscore-agent` daemon** (Linux amd64/arm64). Source: `cmd/kscore-agent/`, `internal/agent/`. _Reasoning: a Linux agent is the foundational sysadmin trial UX._
- **Agent registration** (with hardware/OS metadata; auto-gen or explicit ID). _Reasoning: lifecycle entry point._
- **Heartbeat loop with system metrics** (CPU, memory, disk %; default 30s). _Reasoning: liveness + drives stale detection on server._
- **Continuous metadata collection** (distro, kernel, CPU, memory, NIC, dual-stack, container/VM detection). Source: `internal/agent/metadata.go`. _Reasoning: powers targeting + drift._
- **Command execution engine** (timeout, working dir, env, user-switch, SIGTERM→SIGKILL grace). Source: `internal/agent/executor.go`. _Reasoning: core remote-exec endpoint._
- **Security enforcement** (HMAC-signed commands, allowlist/blocklist, blocked patterns, env restrictions). Source: `internal/agent/security.go`. _Reasoning: trial users need explicit safety; defaults must be strict._
- **TUI-guided bootstrap** (Bubble Tea, demo/production/enterprise modes). Source: `cmd/kscore-bootstrap/`, `cmd/kscore-agent/.../bootstrap`. _Reasoning: per Epic 27 — single-binary "answer 5 questions" install is a Salt-parity table-stake._
- **Non-interactive bootstrap** (CLI flags + env vars). _Reasoning: automation/Ansible-driven installs need this._
- **Bootstrap phases** (detect → configure → validate → install → verify with rollback on failure). _Reasoning: safe re-runs._
- **Systemd service install + management**. _Reasoning: standard Linux daemon UX._
- **Self-signed CA bootstrap path** (demo) + CSR path (production). _Reasoning: zero-config dev + real-prod story._
- **Agent config on disk** (`/etc/keystone-core/keystone-core-agent.yaml`, certs in `certs/`). _Reasoning: standard FHS layout._
- **Graceful shutdown** (SIGTERM → unsubscribe, drain in-flight, exit). _Reasoning: clean rolling restarts._
- **Reconnect with exponential backoff**. _Reasoning: handles flaky control plane links._
- **Plugin host integration** (loads from module system — see Domain 18). _Reasoning: required for state apply + extensibility._
- **State runner integration** (apply/check/drift modules locally). _Reasoning: see Domain 8._

### v1.x (deferred)

- **Embedded NATS / hybrid mode (agent as host or leaf)** **[v2.x+]**. Source: `internal/agent/nats_server.go`, hybrid_mode_state_machine. _Reasoning: edge / disconnected scenarios; v1 assumes control plane is reachable. Carries significant FSM complexity._
- **Endpoint advertiser + reverse-leaf NAT traversal** **[v2.x+]**. _Reasoning: same — niche._
- **Windows agent** (native service) **[v1.x]**. _Reasoning: deferred per state-mgmt categorization; Windows stdlib lands together in v1.x._
- **macOS agent** **[v1.x]**. _Reasoning: low trial-population; defer behind Windows._
- **Interactive shell sessions** **[v1.x]**. _Reasoning: nice-to-have, not core._
- **VM-based bootstrap test harness** **[v1.x]**. _Reasoning: Docker CI sufficient for v1.0._
- **Auto-rotation of NATS creds in memory** **[v1.x]**. _Reasoning: gates on SPIRE/identity rotation._

---

## 7. Remote Execution & Targeting

### v1.0 (in scope)

- **Single-agent execution** (gRPC `ExecuteCommand` stream). Source: `internal/execution/`, `pkg/api/v1`. _Reasoning: most basic op._
- **Batch execution across target expressions** (concurrency-limited semaphore, configurable). Source: `internal/targeting/batch.go`, `internal/controlplane/batch_dispatcher.go`. _Reasoning: fleet ops table-stake._
- **Streaming output protocol** (BATCH_START → AGENT_START → AGENT_OUTPUT → AGENT_COMPLETE → BATCH_COMPLETE + summary). _Reasoning: real-time UX._
- **Targeting by hostname glob** (`web-*`, `db-prod-*`). Source: `internal/targeting/`. _Reasoning: classic Salt UX._
- **Targeting by labels** (`role:web`, `env:prod`). _Reasoning: modern label-based selectors._
- **Targeting by built-in fields** (os, arch, status, ip, hostname). _Reasoning: expected sysadmin UX._
- **Compound expressions** (AND/OR/NOT, parens). Source: uses `expr-lang/expr`. _Reasoning: code is already there; cost negligible to enable in v1.0; users expect it from a Salt-parity tool._
- **Dry-run mode** (show matched agents without dispatching). _Reasoning: safety._
- **Job tracking** (UUID, persistence, status, history). _Reasoning: audit + retrospect._
- **Cancellation** (SIGTERM 5s grace → SIGKILL). _Reasoning: stuck job recovery._
- **Cross-platform shell abstraction** (bash, sh, PowerShell, cmd; auto-detect). Source: `internal/execution/shell.go`. _Reasoning: matches multi-OS state stdlib._
- **Continue-on-failure flag** (default on for batch). _Reasoning: matches Salt expectations._
- **Command policy** (Strict/Normal/Permissive; allow/block lists; block shell metacharacters). Source: `internal/execution/policy.go`. _Reasoning: defense in depth._
- **`kscorectl exec run|async|status|list|cancel|output|script` CLI**. Source: `cmd/kscore-exec/`. _Reasoning: primary trial UX surface._

### v1.x (deferred)

- **Fact-based selectors** (`facts.memory > 16Gi`) **[v1.x]**. _Reasoning: requires stable agent fact schema first; expression engine already supports it once facts land in metadata._
- **Percentage-based / rolling batches** (`--batch 10%`) **[v1.x]**. _Reasoning: nice-to-have; concurrency limit covers most use cases._
- **Output archival to object storage (S3/GCS) cold-tier** **[v1.x]**. _Reasoning: cost optimization; v1 stores in DB._
- **Interactive shell over stream** **[v1.x]**. _Reasoning: see Agent Runtime._

---

## 8. State Management & Stdlib Modules

### v1.0 (in scope — declarative engine)

- **Declarative state DSL** (YAML; metadata, includes, variables, declarations). Source: `internal/statemgmt/`. _Reasoning: core IaC UX._
- **Module interface** (Name, ValidStates, Check, Apply, Test). _Reasoning: extension point._
- **Requisite system** (require, require_in, watch, watch_in, prereq, prereq_in, onchanges, onchanges_in). _Reasoning: dependency management is what separates "config push" from "state engine"._
- **DAG resolver with cycle detection**. _Reasoning: deterministic apply order; clear error reporting._
- **Go template rendering** (vars, facts, custom filters: upper/lower/title/trim/join/split/default). _Reasoning: parameterization._
- **Drift detection with severity** (none/low/medium/high/critical). _Reasoning: continuous compliance promise._
- **Cross-platform dispatch** (runtime.GOOS-based provider selection). _Reasoning: stdlib breadth._
- **Dry-run / check mode**. _Reasoning: safety._
- **Audit + event emission per state apply**. _Reasoning: compliance trail._
- **History store** (SQLite-backed; query past runs). _Reasoning: rollback + audit._
- **State runner pipeline** (parse → validate → resolve → check → apply → test → report). _Reasoning: deterministic, debuggable engine._
- **Saga / checkpoint integration** (Epic 58 — minimal v1; advanced v1.x). _Reasoning: long-running multi-step state needs ordered compensation._

### v1.0 (in scope — base stdlib, ~40 modules)

> **System & core**: `file`, `package`, `service`, `user`, `group`, `cmd`, `system`
> **Scheduled tasks**: `cron`, `systemd_timer`, `at`
> **Storage**: `mount`, `swap`, `lvm`, `disk`, `link`
> **Network (base)**: `network`, `route`, `bond`, `bridge`, `vlan`
> **Firewall (base)**: `firewall` (abstraction), `iptables`, `nftables`, `firewalld`
> **SSH & security**: `ssh`, `security` (SELinux/AppArmor)
> **System config**: `hostname`, `timezone`, `sysctl`, `kernel_module`
> **Files & VCS**: `git`, `config`, `archive`, `langpkg` (pip/npm/gem/etc.)
> **Certificates**: `x509`

_Reasoning: this set covers ~90% of universal Linux sysadmin daily tasks. Sysadmin trial users can replace Salt formulas / Ansible playbooks for their core workflow._

### v1.x (deferred — Linux extended + Windows + containers + DBs)

- **Windows-native modules**: `win_feature`, `win_firewall`, `win_registry`, `win_service`, `win_package`. _Reasoning: ships with Windows agent in v1.x._
- **Container modules**: `docker_container`, `docker_image`, `docker_network`, `docker_volume`. _Reasoning: ops feature; not blocking sysadmin trial._
- **Web servers**: `web` (nginx/Apache abstraction). _Reasoning: same._
- **Database admin**: `postgres_database`, `mysql_database`, `redis`. _Reasoning: same._
- **macOS-specific scheduling**: `launchd`. _Reasoning: macOS agent is v1.x._

### v2.x+ (deferred)

- **Kubernetes modules** (`k8s_deployment`, `k8s_statefulset`, `k8s_daemonset`, `k8s_job`, `k8s_cronjob`, `k8s_service`, `k8s_ingress`, `k8s_configmap`, `k8s_secret`, `k8s_namespace`, `k8s_pvc`, `k8s_hpa`). **[v2.x+]**. _Reasoning: cloud-native; depends on K8s operator (v1.x) maturing first._
- **DNS provider modules** (Route53, CloudFlare, Hetzner, etc.). **[v2.x+]**. _Reasoning: see DNS provider domain._
- **Niche networking** (`promisc`, `wifi`, `dot1x`, `scheduled_task` Windows). **[v2.x+]**. _Reasoning: niche._
- **Vendor-specific modules** (Cisco IOS, Juniper, etc.). **[v2.x+]**. _Reasoning: proxy-agent territory._

---

## 9. Event System

### v1.0 (in scope)

- **Event bus on NATS JetStream** (KSCORE_EVENTS stream, `kscore.events.>` subjects). Source: `internal/events/`. _Reasoning: fan-out + replay foundation._
- **Event types**: 22 across 6 categories (agent×5, job×4, state×5, system×3, user×3, policy×2). _Reasoning: covers v1 emit needs from all domains._
- **Event struct** (ID, Type, Source, Time, Severity, CorrelationID, Tags, Data, Subject). _Reasoning: standard event envelope._
- **EventStore** (SQL-backed; query, retention, batching). _Reasoning: long-term query + compliance._
- **EventPublisher / EventSubscriber interfaces**. _Reasoning: pluggable transport._
- **Filter expressions** (homegrown CEL-like parser; field comparison, AND/OR/NOT, regex, glob). _Reasoning: routing, subscription, queries depend on it. Note: rebuild can adopt `google/cel-go` directly instead of homegrown — see PROJECT-DETAILS §4.9._
- **gRPC EventService** (List, Get, Emit, Subscribe stream w/ optional historical replay, GetEventTypes, GetEventStats). _Reasoning: client API surface._
- **CLI** (`kscore-events list|query|emit|subscribe|watch|storage-stats`). _Reasoning: ops visibility._
- **Audit emission integration** (events for auth/policy/user actions). _Reasoning: compliance trail._
- **Retention policies** (per-type, age + count limits). _Reasoning: cost control._
- **Correlation IDs** (group related events; passed through gRPC contexts). _Reasoning: tracing across multi-step ops._
- **Severity levels** (debug/info/warn/error/critical). _Reasoning: filtering + alerting._

### v1.x (deferred — automation layer)

- **Reactor engine** (filter → action; LogAction/EventAction/WebhookAction; throttle/debounce). **[v1.x]**. _Reasoning: valuable but overlaps with runbooks/policy; hold one release for clean separation of concerns._
- **Lifecycle tracking** (created → published → routed → processing → processed/failed/expired). **[v1.x]**. _Reasoning: debugging tool; ships with reactors._
- **Enrichment pipeline** (tag/data/conditional enrichers). **[v1.x]**. _Reasoning: paired with reactors._
- **Dead-letter queue** (failed reactor exec retry). **[v1.x]**. _Reasoning: paired with reactors._

### v2.x+ (deferred)

- **Kafka integration** (sarama producer; CloudEvents serialization). **[v2.x+]**. _Reasoning: enterprise integration; not blocking._
- **CloudEvents 1.0 marshaling**. **[v2.x+]**. _Reasoning: standard adoption._
- **Inbound webhook receiver for events** (HMAC, signature). **[v1.x]**. _Reasoning: minor scope._
- **Object-storage archival (S3/GCS)**. **[v2.x+]**. _Reasoning: cost optimization._
- **Multi-region replication**. **[v3.x+]**. _Reasoning: post-supercluster._

---

## 10. Identity & Auth

### v1.0 (in scope)

- **API keys** (random base62, SHA-256 hashed at rest, expiry, role assignment). Source: `pkg/api/apikeys/`, `pkg/api/auth/`. _Reasoning: simplest auth path; trial UX._
- **API key rotation + revocation**. _Reasoning: real auth lifecycle._
- **mTLS** (X.509 v3 with SPIFFE URI SANs, TLS 1.3 default min). Source: `internal/security/`, `pkg/api/auth/`. _Reasoning: server-to-server + agent identity._
- **Embedded CA** (root CA 10y default + signing CA 1y, auto-rotate at 30d before expiry). Source: `internal/identity/ca.go`, `internal/pki/`. _Reasoning: zero-config UX; defers SPIRE complexity._
- **SPIFFE-shaped identities from day 1** (trust domain default `kscore.local`; agent/server/service paths). Source: `internal/identity/`. _Reasoning: cheap to do at start; expensive to retrofit. v1.x SPIRE swap-in is then near-trivial._
- **Embedded identity provider** (CA + SVID issuer + attestation engine + token store). _Reasoning: trial-day-1 zero-deps._
- **JWT** (HS/RS/ES family; configurable role claim). _Reasoning: integration with external IdPs._
- **Cluster join tokens** (SHA-256 stored, TTL default 5m, max-uses, leader-issued). Source: `internal/identity/`, Epic 44, `cmd/kscore-identity/token`. _Reasoning: secure cluster bootstrap._
- **RBAC** (admin/operator/readonly hierarchy; method→role map; bypass list). Source: `pkg/api/rbac/`. _Reasoning: minimum viable RBAC; full role/permission CRUD ships v1.x._
- **Auth interceptor chain** (gRPC unary + stream; rate-limit on failure with exp backoff; audit logging). _Reasoning: enforcement entry point._
- **`kscore-identity` CLI** (token CRUD, CA info/rotate/export, status). Source: `cmd/kscore-identity/`. _Reasoning: ops admin needs visibility._
- **Cert auto-rotation at ~50% lifetime** (agent-driven). _Reasoning: zero-touch cert lifecycle._
- **TLS 1.3 default** (1.2 opt-in for legacy). _Reasoning: secure default._

### v1.x (deferred)

- **Full RBAC role/permission CRUD with per-resource permissions** **[v1.x]**. _Reasoning: trio (admin/operator/readonly) covers ~80% of trials; full RBAC is a real epic._
- **Trust federation** (bundle endpoint for cross-domain trust) **[v1.x]**. _Reasoning: multi-site adoption follows trial._
- **SPIRE integration** (external SPIRE server, agent-socket attestation, K8s SAT/AWS IID/etc.) **[v1.x]**. _Reasoning: SPIRE adds 3+ ops setup steps; trial users use embedded provider._

### v2.x+ (deferred)

- **Cloud workload identity** (AWS IAM/IRSA, GCP WI, Azure MI). **[v2.x+]**. _Reasoning: multi-cloud follows mature single-domain._
- **Service mesh integration** (Istio, Linkerd, Consul Connect identity extraction). **[v2.x+]**. _Reasoning: adds CNI/injector deps; post-trial._
- **Multi-party CA / certificate issuance** **[v2.x+]**. _Reasoning: governance overhead; single-signer is fine for v1._

---

## 11. Secrets Management

### v1.0 (in scope)

- **Encrypted-file backend** (AES-GCM, JSON serialization, zero external deps). Source: `internal/secrets/file/`. _Reasoning: trial-day-1 zero-deps option._
- **HashiCorp Vault backend** (KV v1/v2, dynamic secrets, transit, namespace support). Source: `internal/secrets/vault/`. _Reasoning: highest-ROI; most common backend in trial environments._
- **SecretBroker with path-prefix routing** (longest-prefix-first). Source: `internal/secrets/broker.go`. _Reasoning: multi-backend coordination from day 1._
- **CRUD via REST + gRPC + CLI**. Source: `pkg/api/secrets/`, `pkg/api/v1/secrets.pb.go`, `cmd/kscore-secrets/`. _Reasoning: fully implemented; gating costs more than shipping._
- **Lease management** (persistent SQLite, eager/lazy/on-demand renewal strategies, scheduler, callbacks). Source: `internal/secrets/lease_manager.go`. _Reasoning: dynamic secrets without lease management is broken._
- **Transit operations** (encrypt/decrypt/sign/verify/HMAC, batch ops, key versioning, convergent encryption). Source: `internal/secrets/vault/transit.go`. _Reasoning: encryption-as-a-service for app-layer integration; gRPC API exists; cost to ship is near zero._
- **Encrypted in-memory cache** (AES-GCM, TTL eviction, path-prefix invalidation, stats). Source: `internal/secrets/cache.go`. _Reasoning: latency reduction is critical at scale._
- **Audit emission integration** (every secret access event with agent ID, SPIFFE ID, action, timestamp). Source: `internal/secrets/audit.go`. _Reasoning: compliance trail._
- **CLI** (`kscore-secrets get|list|backends|audit|leases|cache|encrypt|decrypt|template`). _Reasoning: ops + integration UX._
- **Secret masking in API responses + logs**. _Reasoning: leak prevention default._

### v1.x (deferred)

- **Rotation orchestration with strategies** (blue-green / rolling / canary / immediate; health checks; auto-rollback) **[v1.x]**. Source: `internal/secrets/rotation.go`. _Reasoning: powerful but complex; ship after v1.0 stabilizes; depends on healthy verification framework (GitOps domain v1.x features)._
- **Cron-based rotation scheduling + Slack/PagerDuty notifications** **[v1.x]**. _Reasoning: ships with rotation orchestrator._
- **Compliance reports + anomaly detection** **[v1.x]**. _Reasoning: paired with rotation; same release._

### v2.x+ (deferred)

- **AWS Secrets Manager backend** **[v2.x+]**. _Reasoning: cloud-vendor backend; ships with cloud KMS work._
- **Azure Key Vault backend** **[v2.x+]**. _Reasoning: same._
- **GCP Secret Manager backend** **[v2.x+]**. _Reasoning: same._
- **Cloud KMS for master keys** (AWS KMS / Azure HSM / GCP KMS, envelope encryption) **[v2.x+]**. _Reasoning: depends on cloud SDKs; v1 master keys are file-based or Vault-transit-derived._
- **Hardware HSM support** (PKCS#11, Thales Luna, AWS CloudHSM) **[v2.x+]**. _Reasoning: enterprise compliance; niche._
- **L2 KMS-backed cache** **[v2.x+]**. _Reasoning: ships with cloud KMS._

---

## 12. Audit & Policy

### v1.0 (in scope — audit + policy infrastructure, audit-mode-only enforcement)

- **Audit logger** (`internal/audit/`) — structured events for all sensitive ops (auth decisions, secret access, state apply, command exec, policy evaluations). _Reasoning: compliance-curious trial users will run an audit query in week one._
- **Audit storage** (in-memory circular buffer + SQLite persistent backend, retention policy, redaction config). Source: `internal/policy/audit.go`, `SQLitePolicyAuditStore`. _Reasoning: query + compliance reports require persistence._
- **Audit query API** (`AuditFilter` — actor, resource, time range, severity; pagination). _Reasoning: ops investigation UX._
- **Audit export** (JSON / JSONL / CSV). _Reasoning: hand-off to SIEM tools._
- **`kscore-audit` CLI** (`log`, `report`, `export`, `stats`, `search`, `analyze`, `timeline`, `watch`). _Reasoning: ops surface._
- **Policy engine infrastructure** (Engine + Registry + 3 evaluators: OPA, CEL, Builtin). Source: `internal/policy/`. _Reasoning: shipping the engine in audit-mode is cheap; v1.x just flips the enforce flag._
- **OPA Rego evaluator** (`open-policy-agent/opa` v1.16.2, embedded `v1/rego`; Rego v1 syntax; fixed `package keystone.policy`; `http.send`/`net.*`/`opa.runtime` builtins denied; compiled-query cache). _Reasoning: standard policy language for compliance shops; restricted builtins keep operator-supplied policies pure decision logic._
- **CEL evaluator** (`google/cel-go`; `input`/`resource`/`action`/`user`/`context` variables). _Reasoning: lighter-weight inline policies._
- **Builtin policies** (require-labels, require-owner, allowed-environments, allowed-actions, deny-privileged, time-window, no-root-execution, etc.). _Reasoning: ship with sensible defaults._
- **`Policy{ID, Name, Type, Category, Severity, EnforcementMode, Code, Enabled, Tags}`** model. _Reasoning: full schema in v0.1 even if enforcement is gated._
- **`PolicySet`** (groups; set-level enforcement override). _Reasoning: same._
- **`Bindings`** (attach policies to resource types, optional action/selector). _Reasoning: same._
- **Policy evaluation API** (single + set + by-resource-type). _Reasoning: callers can evaluate; v1.0 just doesn't *block* on results._
- **Compliance reports** (period, compliance rate, per-policy stats, top violations, severity distribution, trend points). _Reasoning: this is the audit user's payoff._
- **Compliance framework mappings** (CIS, SOC2, NIST-800-53, HIPAA, PCI-DSS, GDPR, ISO-27001, Custom). _Reasoning: turnkey compliance value._
- **`PolicyService` gRPC** (Evaluate, EvaluatePolicySet, ListViolations, GetComplianceReport, GetAuditLog). _Reasoning: full RPC surface; CRUD methods present but server returns Unimplemented._
- **`kscore-policy` CLI v1.0 subset** (`list`, `validate`, `check`, `show`, `eval`, `test`, `compliance`, `violations`). _Reasoning: ops trial UX._

### v1.x (deferred — full enforcement)

- **Enforcement modes: Enforce + Warn (active blocking)** **[v1.x per user direction]**. _Reasoning: misconfigured policy can break a fleet — needs proven audit-mode track record + simulation tooling first._
- **Enforcement actions** (Block, Warn, Audit, Remediate). **[v1.x]**. _Reasoning: blocking semantics belong with approval workflow infra._
- **Pre/post-execution hooks** (state apply + command dispatch interception). **[v1.x]**. _Reasoning: depends on enforcement._
- **Approval workflows for policy violations**. **[v1.x]**. _Reasoning: human-in-the-loop._
- **`kscore-policy create|update|delete|activate|deactivate|remediate|monitor`**. **[v1.x]**. _Reasoning: full CRUD ships with enforcement._
- **Policy persistence** (etcd or Postgres; dynamic reload). **[v1.x]**. _Reasoning: in-memory registry is fine for v1.0 audit-only._

### v1.x other

- **Continuous compliance scan scheduler** **[v1.x]**. _Reasoning: cron-based eval; depends on schedule infrastructure._
- **CEL custom function library** **[v1.x]**. _Reasoning: power-user feature._
- **Anomaly detection (audit log analysis)** **[v1.x]**. _Reasoning: ships with rotation/security work._

---

## 13. GitOps Integration

### v1.0 (in scope — basics for sysadmin trial)

- **Webhook receiver** (HTTP server, configurable addr/path, source auto-detection). Source: `internal/gitops/webhook/`. _Reasoning: GitOps integration is a key trial differentiator._
- **ArgoCD webhook handler** (application sync/health/deployment events). _Reasoning: most common GitOps tool._
- **Flux webhook handler** (Kustomization/HelmRelease events). _Reasoning: same._
- **GitHub webhook handler** (deployment, deployment_status, workflow_run, push). _Reasoning: trial users use GitHub._
- **GitLab webhook handler** (push, deployment, pipeline). _Reasoning: GitLab is common in EU._
- **Webhook authentication** (HMAC-SHA256, Bearer token). _Reasoning: secure-by-default._
- **Event normalization** (provider events → unified `webhook.Event` → emit on Keystone event bus as `gitops.{argocd|flux|github|gitlab}.*`). _Reasoning: consumed by reactors (v1.x) and audit._
- **Verification engine** (HTTP, gRPC, command verifiers; sequential or parallel; retries + timeout per step). Source: `internal/gitops/verification/`. _Reasoning: deployment verification is the Keystone runtime-control-plane value prop._
- **Verification workflow execution** (`Verifier` interface + plugin registration). _Reasoning: extension point._
- **Manual rollback API + CLI** (REST `/api/v1/gitops/rollback`, `kscore-gitops rollback`). _Reasoning: must-have for "verify then rollback" trial story._
- **Rollback executors** (Git revert, ArgoCD sync to revision, K8s rollout undo). _Reasoning: all common patterns._
- **Approval workflow for rollback** (Pending → Approved/Rejected → InProgress → Completed/Failed → Verifying). _Reasoning: human-in-the-loop is non-negotiable for prod._
- **Verification result storage + REST list/get** (`/api/v1/gitops/verifications`). _Reasoning: history + audit._
- **`kscore-gitops` CLI** (`verify`, `rollback`). _Reasoning: trial UX._

### v1.x (deferred — automation)

- **Multi-env promotion pipelines** (sequential dev → staging → prod with approvals). **[v1.x]**. _Reasoning: powerful; needs design time. Foundational rollback works in v0.1._
- **Promotion state machine + REST API**. **[v1.x]**. _Reasoning: same._
- **Basic remediation strategies** (rollback action). **[v1.x]**. _Reasoning: paired with promotion._

### v1.x (deferred — progressive delivery)

- **Canary deployments** (weight-based progression: 5/25/50/100, dwell time, threshold eval). **[v1.x]**. _Reasoning: depends on observability metrics + healthy verification framework._
- **Threshold evaluation per canary step**. **[v1.x]**. _Reasoning: same._
- **Advanced remediation** (scale-down, traffic shift, custom workflows). **[v1.x]**. _Reasoning: cluster integration heavy._
- **Diagnostic collection on remediation**. **[v1.x]**. _Reasoning: same._
- **Git sync orchestration + multi-repo coordination** **[v1.x]**. _Reasoning: separate from webhook ingest; specialized._
- **Helm/Kustomize-native integration** **[v1.x]**. _Reasoning: extension._
- **Deployment dependency graph** **[v1.x]**. _Reasoning: complex to design correctly._
- **Webhook timestamp validation + nonce dedup** **[v1.x]**. _Reasoning: defense-in-depth; current HMAC-only is acceptable for v1.0._

---

## 14. Outbound Webhooks

### v1.0 (in scope)

- **Persistent webhook subscriptions** (SQLite-backed; CRUD via REST + CLI). Source: `internal/webhook/outbound/`, `pkg/api/webhooks/`. _Reasoning: commercial trial users want Slack/PagerDuty/custom hooks on day 1._
- **Event filter on subscriptions** (glob patterns: `agent.*`, `state.drift`, `policy.*`, `*`). _Reasoning: targeted notifications._
- **HMAC-SHA256 signing** (GitHub-compatible `sha256=<hex>` format; per-subscription secret). _Reasoning: secure delivery._
- **Custom HTTP headers per subscription**. _Reasoning: integrate with auth-required receivers (Slack tokens, etc.)._
- **Exponential backoff retry** (jittered, configurable max retries default 3). _Reasoning: handles transient receiver errors._
- **Delivery history with audit trail** (status, status code, attempt #, error, timestamp). _Reasoning: ops visibility._
- **Per-endpoint circuit breaker** (closed → open after N failures → half-open recovery). _Reasoning: protects against repeated failures._
- **Per-subscription delivery timeout** (default 10s). _Reasoning: prevents stuck deliveries._
- **Secret masking in API responses** (`***`). _Reasoning: leak prevention._
- **REST API** (`/api/v1/webhooks/subscriptions` CRUD + `{id}/test` + `{id}/deliveries`). _Reasoning: standard surface._
- **`kscore-webhook outbound` CLI** (`list`, `create`, `show`, `delete`, `history`, `test`). _Reasoning: ops UX._
- **NATS event-bus consumer** (`>` subject; pattern-matches; async fan-out per subscription). _Reasoning: integration with Domain 9._
- **Manager-driven async delivery** (WaitGroup, bounded goroutines for back-pressure). _Reasoning: scales._

### v1.x (deferred)

- **Inbound webhooks for non-GitOps event sources** (custom payload ingestion + event emission). **[v1.x]**. _Reasoning: complementary to outbound; not blocking trial._
- **Webhook body templating** (Handlebars/Jinja2-style). **[v1.x]**. _Reasoning: nice-to-have; hardcoded JSON serialization works for v1._
- **Per-destination rate limiting**. **[v1.x]**. _Reasoning: most receivers handle bursts; explicit RL is power-user feature._
- **Auto-cleanup of old delivery history** (currently manual). **[v1.0.x]**. _Reasoning: trivial; ship with first dot release._

---

## 15. Clustering & HA

### v1.0 (in scope — minimum viable cluster, the v1 differentiator)

- **3-node cluster formation** (3 × `kscore-server` with embedded etcd). Source: `internal/cluster/`. _Reasoning: the v1 commercial-trial-ready bar._
- **Embedded etcd mode** (single-binary, etcd in-process). _Reasoning: zero-deps + clusterable._
- **External etcd mode** (3-node external etcd cluster for medium production). _Reasoning: production scaling path._
- **etcd-based membership** (ephemeral leases, join, leave, fail detection). Source: `internal/cluster/`. _Reasoning: foundational._
- **Heartbeat mechanism** (5s interval, 30s timeout — must be ≥3× interval). _Reasoning: avoids leader flapping._
- **Member status state machine** (HEALTHY → DEGRADED → UNHEALTHY → LEAVING → removed). _Reasoning: clear lifecycle._
- **etcd-based leader election** (concurrency.Election; <3s on failure). _Reasoning: standard pattern._
- **Voluntary leadership resignation + transfer**. _Reasoning: graceful upgrades._
- **Automatic failover** (heartbeat-loss detection <5s, batched agent reassignment, batched job reassignment, idempotency keys for dedup). _Reasoning: HA promise._
- **Consistent hashing for agent assignment** (configurable virtual nodes default 150; minimal rebalancing on topology changes). _Reasoning: even distribution + stability._
- **Rebalancing on member join/leave** (cooldown 5s minimum). _Reasoning: prevents rebalance storms._
- **Singleton-task manager (leader-only)** (reactor coordinator, scheduled jobs, cleanup, metric aggregation, agent rebalance). _Reasoning: avoids duplicate work._
- **Recovery workflow** (STARTING → CONNECTING → SYNCING → VERIFYING → REJOINING → RECLAIMING → COMPLETED). _Reasoning: deterministic restart._
- **Graceful shutdown** (drain → transfer leadership → deregister). _Reasoning: zero-downtime upgrades._
- **Split-brain prevention via quorum** (N/2+1; minority blocks writes). _Reasoning: data integrity._
- **Lease + epoch fencing** (operations require valid lease; epoch increments on election; stale epochs rejected). _Reasoning: defense-in-depth against split-brain._
- **Server-to-server coordination** (mTLS gRPC `CoordinationService`: ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, Heartbeat, PropagateState). _Reasoning: NATS-fallback recovery._
- **Health monitor with consecutive-failure threshold** (3 failures → unhealthy). _Reasoning: avoids transient false positives._
- **Cluster backup/restore** (binary + JSON snapshot; cluster metadata, shard assignments, config). Source: `cmd/kscore-cluster-backup/`. _Reasoning: DR baseline._
- **`ClusterService` gRPC + REST** (status, members CRUD, leader transfer, rebalance, backup/restore, watch streams). _Reasoning: full ops surface._
- **`kscore-cluster` CLI** (`status`, `members`, `leader`, `add`, `remove`, `transfer-leader`, `rebalance`, `backup`, `restore`). Source: `cmd/kscore-cluster/` (shared `internal/cli/cluster`). _Reasoning: ops UX._
- **HA resilience tests in CI** (NATS failure, etcd failure, network partition, split-brain). Source: `test/e2e/ha/` (`//go:build integration`). _Reasoning: prove the promise._
- **Performance targets**: cluster forms <10s; first leader <3s; failover detection <5s; agent reassign <10s; minority blocks writes <1s. Source: `test/e2e/ha/slo_test.go` (`//go:build slo`, `make slo`, every-PR CI gate). _Reasoning: documented SLOs._

### v1.x (deferred)

- **Backup automation/scheduling** **[v1.x]**. _Reasoning: manual backup works; scheduling adds infra._
- **Comprehensive HA dashboard** **[v1.x]**. _Reasoning: basic status sufficient; rich dashboard is observability work._
- **Read-only replicas** **[v1.x]**. _Reasoning: niche._
- **Auto-scaling** (auto-add/remove members based on metrics) **[v1.x]**. _Reasoning: complex._

### v2.x+ (deferred)

- **Multi-region clustering / federation** (cross-DC, gateway routing) **[v2.x+]**. _Reasoning: ships with NATS supercluster._
- **Dynamic shard splitting under load** **[v2.x+]**. _Reasoning: speculative._
- **Advanced topology (gateway / proxy members)** **[v2.x+]**. _Reasoning: same._

---

## 16. Observability

### v1.0 (in scope)

- **Structured logging** (`log/slog`; JSON / logfmt / text; stdout default; correlation IDs; log levels). Source: `internal/logging/`. _Reasoning: zero-config baseline._
- **Prometheus metrics registry** (custom; counters, gauges, histograms, summaries; cardinality limiter). Source: `internal/metrics/`. _Reasoning: standard ops integration._
- **`/metrics` HTTP endpoint** (Prometheus exposition). _Reasoning: scrape target._
- **OpenTelemetry tracing** (OTLP, Zipkin, stdout exporters; configurable sampling: probabilistic/parent-based/rate-limiting/adaptive). Source: `internal/tracing/`. _Reasoning: distributed-debug baseline._
- **Health endpoints** (`/health/live`, `/health/ready` with NATS+DB checks, `/health/status` with component latencies). Source: `internal/health/`. _Reasoning: K8s probes + ops dashboards._
- **Pre-built Grafana dashboards** (Control Plane Health, Agent Fleet, State Mgmt, Policy Compliance, NATS, Audit, Module System, Event System, Secrets, Remote Execution). Source: `deploy/grafana/dashboards/`. _Reasoning: turnkey monitoring._
- **pprof profiling endpoints** (CPU, memory, goroutine, mutex; opt-in via config). Source: `internal/profiling/`. _Reasoning: standard Go ops tool._
- **Correlation ID propagation** (gRPC metadata, NATS headers, log entries, span attributes). _Reasoning: traceable cross-service flows._

### v1.x (deferred — TUI + NATS telemetry transport)

- **`kscore-monitor` TUI** (Bubble Tea; 8 base views: dashboard, agents, events, state, policy, jobs, logs, metrics). **[v1.x]**. _Reasoning: powerful ops UX but not blocking trial; needs gRPC-multiplex client + NATS subscriber. v1.0 ships READable web dashboards (Grafana) + CLI._
- **TUI extras** (cluster, secrets/leases, schedules, runbooks, webhooks views — 13 total) **[v1.x]**.
- **Drill-downs, vim navigation, alert bar, connection health indicators, themes, search filters** **[v1.x]**.
- **NATS telemetry transport** (logs/metrics/traces over NATS subjects so isolated agents don't need outbound HTTP). **[v1.x]**. _Reasoning: paired with telemetry gateway._
- **CLI audit logging to syslog/journald** **[v1.x]**. _Reasoning: ships with TUI work; audit infra in v0.1 already._

### v1.x (deferred — telemetry gateway)

- **`kscore-telemetry-gateway` standalone service** (subscribes to NATS telemetry subjects; aggregates; exposes Prom scrape, Loki push, OTLP traces). Source: `internal/gateway/`, `cmd/kscore-telemetry-gateway/`. **[v1.x]**. _Reasoning: enables agents-behind-NAT topology; not needed for v1.0 connected trial._
- **HA gateway** (queue groups + leader election) **[v1.x]**.
- **Helm chart for gateway** **[v1.x]**.

### v2.x+ (deferred)

- **Adaptive sampling tied to error metrics** **[v2.x+]**. _Reasoning: refinement._
- **pprof visualization UI** **[v2.x+]**. _Reasoning: niche._
- **SIEM export (CEF/LEEF)** **[v2.x+]**. _Reasoning: enterprise integration._
- **Real-time alerting from TUI** **[v2.x+]**. _Reasoning: niche._

---

## 17. Blueprints & Runbooks

### v1.0 (in scope — basic but real)

- **Blueprint manifest format** (`blueprint.yaml`: metadata, requires/requires_before, parameters JSON-Schema, entrypoints, hooks, features, outputs). Source: `internal/blueprint/`. _Reasoning: Salt-formula-shaped UX._
- **Blueprint apply** (parse → param validate → resolve dependencies → render templates → execute states). _Reasoning: core functionality._
- **Blueprint feature flags + multi-instance namespacing (`as:`)**. _Reasoning: composability for sysadmin reuse._
- **Standard blueprint catalog (~6 v1.0)**: `demo` (single-node demo), `production-cluster` (3-node HA), `monitoring-stack` (Prom+Grafana+Loki), `security-baseline` (CIS-aligned hardening), `postgres-ha` (Postgres + WAL replication), `nats-cluster`. _Reasoning: 6 turnkey blueprints prove the model and give sysadmins immediate value._
- **`kscore-blueprint` CLI** (`init`, `validate`, `lint`, `info`, `install`, `apply`, `update`, `remove`, `applied`, `rollback`, `bundle`). _Reasoning: full lifecycle._
- **Blueprint storage**: filesystem (v1.0). _Reasoning: simple._
- **Runbook YAML model** (metadata + spec with inputs, steps, onSuccess/onFailure, timeout, maxRetries). Source: `internal/runbook/`. _Reasoning: workflow automation table-stake._
- **Runbook step types (v1.0 subset)**: `command`, `api`, `state`, `notification`, `wait`, `noop`, `fail`, `script`, `query`. _Reasoning: covers ~80% of common ops workflows; conditionals + approvals deferred._
- **Runbook step dependencies** (`depends_on`). _Reasoning: order matters._
- **Runbook variable templating** (`{{ steps.X.outputs.Y }}`). _Reasoning: chaining steps._
- **Runbook execution + status** (`kscore-runbook list|execute|status|list-executions|audit|test`). _Reasoning: ops UX._
- **Saga coordinator (minimal)** (`pkg/saga`: forward execution + compensating transactions on failure; in-memory or SQLite log; no checkpoint resume). _Reasoning: rollback safety for multi-step ops._
- **StateMachine library** (`pkg/statemachine`: generic FSM with guards, callbacks, history, optional checkpoint). _Reasoning: used by runbook/promotion/rollback engines._
- **Runbook + blueprint storage** (SQLite). _Reasoning: persistence + history._

### v1.x (deferred — scheduling)

- **`kscore-schedule` CLI + ScheduleService** (cron + interval; time windows; agent/role/tag targeting; retry policy). **[v1.x]**. _Reasoning: high-value but separate concern; ships with Maintenance Service._
- **Maintenance windows + change-window awareness** **[v1.x]**.
- **Schedule + maintenance gRPC + REST APIs** **[v1.x]**.

### v1.x (deferred — runbook power features)

- **Runbook conditional steps** (`if`, `switch`, `loop`, `parallel`, `sub-runbook`). **[v1.x]**. _Reasoning: real workflow power; deferred so the v1.0 step engine ships solid._
- **Per-step approvals + delegations** (with timeout, Slack/email/PagerDuty notifications). **[v1.x]**. _Reasoning: ITSM integration weight._
- **Manual interventions** (`prompt`, `wait-manual`, `confirm` step types). **[v1.x]**. _Reasoning: paired with approvals._
- **Runbook dry-run mode** **[v1.x]**. _Reasoning: ships with conditionals._
- **Rollback step type with auto-compensation** **[v1.x]**. _Reasoning: deepens saga integration._

### v1.x (deferred — full catalog + saga advanced)

- **Standard catalog expansion** (full 14: + `enterprise-platform`, `kubernetes-operator`, `identity-federation`, `gitops-integration`, `proxy-agents`, `file-distribution`, `edge-deployment`, `metrics-only`). **[v1.x]**. _Reasoning: rich catalog drives adoption._
- **Saga checkpoint resume** (`pkg/saga/log_sqlite` advanced; resume from interruption). **[v1.x]**. _Reasoning: production resilience._
- **Blueprint signature + signed bundles** (`kscore-blueprint sign|verify|publish`). **[v1.x]**. _Reasoning: supply chain hardening._
- **Blueprint mirror for air-gap** **[v1.x]**.

---

## 18. Plugin / Module System

### v1.0 (in scope — Starlark + Cosign + filesystem registry)

> **Decision**: Per user direction, plugin/module system ships in v1.0. To make that achievable, v1.0 is **Starlark-only** with **filesystem-backed registry** and **Cosign-only verification** (no SumDB transparency log, no WASM, no cloud-backed registry yet). This delivers Salt-like extensibility on day 1 without doubling v0.1 scope.

- **Starlark runtime** (Python-like sandboxed scripting via `go.starlark.net`; deterministic mode with random/time disabled by default). Source: `pkg/module/runtime/starlark/`. _Reasoning: pure Go, simpler than WASM, sufficient for trial extensibility._
- **Module manifest format** (`module.yaml`: name namespaced `vendor/pkg`, version semver, type, entrypoint, capabilities, limits, dependencies). Source: `pkg/module/manifest/`. _Reasoning: schema is shared with v1.x WASM modules._
- **Capability-based security (7 core capabilities)**: `fs.read`, `fs.write`, `http.get`, `http.post`, `exec`, `secrets.read`, `secrets.write`, `kv`, `log`. _Reasoning: covers ~90% of use cases. Per-syscall granularity is v1.x._
- **Capability scoping** (path globs, domain allowlists, command allowlists, secret-path scoping, rate limits, timeouts). _Reasoning: not just on/off — actually safe defaults._
- **Cosign signature verification** (RSA, ECDSA, Ed25519; KeyID-based key management). Source: `pkg/module/verify/`. _Reasoning: cryptographic supply chain baseline._
- **SHA-256 content addressing** (CAS storage in `~/.kscore/modules/<hash>/`). _Reasoning: dedup + integrity._
- **Module resolver** (semver constraints; DAG with cycle detection; minimum-version-selection conflict resolution). Source: `pkg/module/resolver/`. _Reasoning: reproducible builds._
- **Module lockfile** (`module.lock`: pinned versions + hashes; reproducible across team). Source: `pkg/module/manifest/`. _Reasoning: avoid "works on my machine"._
- **Filesystem-backed registry** (Go-mod-style HTTP endpoints: `/@v/list`, `/@v/{ver}.info`, `/@v/{ver}.mod`, `/@v/{ver}.zip`). Source: `pkg/module/registry/`, `cmd/kscore-registry/`. _Reasoning: simple, proven pattern (matches Go modules); cloud backends slip to v1.x._
- **Module loader pipeline** (parse manifest → verify → policy check → capability validation → runtime init → register granted capabilities). Source: `pkg/module/loader/`. _Reasoning: clear lifecycle._
- **Module audit logging** (every capability invocation: module, version, capability, op, success/failure, duration). Source: `pkg/module/audit/`. _Reasoning: compliance._
- **Plugin discovery** (`kscore-*` binaries in `$PATH`; git-style dispatch via `kscorectl`). Source: `pkg/plugin/`. _Reasoning: extension model from day 1._
- **Starlark SDK** (host capability bindings; Go shims). Source: `modules/sdk/starlark/`. _Reasoning: enables module authors._
- **`kscore-module` CLI** (`init`, `build`, `validate`, `resolve`, `verify`, `sign`, `test`, `publish`, `install`, `update`, `clean`, `tree`). _Reasoning: full author UX._
- **Module test framework** (Starlark unit-test harness). _Reasoning: authors need test/run loop._
- **Module policy hooks** (audit-mode in v1.0; OPA/CEL evaluation gated on `module load`). _Reasoning: enforces capability policy at load time even though policy enforcement is v1.x._

### v1.x (deferred — WASM + cloud backends)

- **WASM runtime** (wazero; WASI; instruction metering; memory bounds). **[v1.x]**. _Reasoning: enables Rust/Go/C++ modules; pure Go runtime so no CGO regression._
- **Rust SDK** (Cargo crate with WASI bindings). **[v1.x]**. _Reasoning: top-tier perf-language for module authors._
- **Go (TinyGo) SDK** **[v1.x]**. _Reasoning: same._
- **OCI registry backend** (Harbor, ECR, GCR, ACR; modules as OCI artifacts). **[v1.x]**. _Reasoning: enterprise registry compatibility._
- **S3/GCS/Azure storage backends** for filesystem registry **[v1.x]**. _Reasoning: cloud-native scaling._
- **`kscore-module mirror`** (export/import for air-gap) **[v1.x]**.
- **`kscore-module update`** with auto-upgrade-compatible-versions **[v1.x]**.

### v1.x (deferred — fine-grained security + supply chain)

- **SumDB transparency log** (Merkle proofs, append-only log, tamper detection). **[v1.x]**. _Reasoning: regulatory/audit value above and beyond signature; not blocking._
- **Fine-grained capability model** (per-syscall grants via seccomp/eBPF on Linux). **[v1.x]**. _Reasoning: defense-in-depth refinement._
- **Module vulnerability scanning + SBOM generation** **[v1.x]**. _Reasoning: supply-chain hardening._

### v2.x+ (deferred)

- **C++ SDK** (Emscripten/WASI SDK). **[v2.x+]**. _Reasoning: niche audience; Rust/Go/Starlark cover most._
- **Federated module registries** (cross-trust-domain) **[v2.x+]**.

---

## 19. Multi-Environment

### v1.0 (in scope — Linux + universal foundations)

- **Platform detection** (Linux distro via `/etc/os-release`; arch; package manager: apt/dnf/yum/pacman/zypper/apk; init system: systemd/sysv/openrc; virtualization: KVM/VMware/VBox/Xen/Hyper-V). Source: `internal/platform/`. _Reasoning: drives stdlib module dispatch._
- **Hardware introspection** (CPU, memory, disks, NICs, system/BIOS info). Source: `internal/hardware/`. _Reasoning: targeting + drift._
- **Network interface detection** (IPv4/IPv6, MTU, speed). Source: `internal/netutil/`. _Reasoning: v1 must be IPv6-clean._
- **IPv6 dual-stack on all listeners** (control plane gRPC/HTTP, NATS, etcd, Postgres). _Reasoning: per Epic 18; non-negotiable for modern infra._
- **Address family preference** (`prefer_ipv4`, `prefer_ipv6`, `ipv4_only`, `ipv6_only`). _Reasoning: deterministic selection._
- **IPv6 bracketing helpers** (`[::]:8080`, `[::1]:9090`, IPv6-aware DSN building for Postgres). _Reasoning: real-world bug source._
- **Cloud metadata stub** (AWS IMDSv2 token, GCP metadata header, Azure MSI — minimal probe; full metadata extraction v1.x). _Reasoning: detect "running in cloud" vs not is useful day 1._

### v1.x (deferred)

- **Windows agent** (native service via SCM, Event Log, PowerShell exec, registry, Chocolatey/winget). **[v1.x]**. _Reasoning: paired with Windows stdlib in state mgmt; meaningful subset of trial users._
- **macOS agent** **[v1.x]**. _Reasoning: low population; defer behind Windows._
- **Container runtime detection** (Docker/containerd/Podman/CRI-O via socket + cgroup parsing). **[v1.x]**. _Reasoning: paired with v1.x container stdlib modules._

### v1.x (deferred)

- **Kubernetes operator** (RemoteExecution + StateConfig CRDs; informer-based reconciliation; drift detection; pod exec). Source: `internal/k8s/`, Epic 48. **[v1.x]**. _Reasoning: complex; depends on mature core; CRDs + reconciliation framework is its own epic._
- **`k8s_*` stdlib modules** (12 modules — see State Management §8). **[v1.x]**.

### v1.x (deferred)

- **Full cloud metadata extraction** (AWS region, AZ, instance ID, type, VPC, subnet, IAM; GCP project, zone, instance, K8s SA; Azure RG, VM, MI). **[v1.x]**. _Reasoning: deeper integration; enables labels-from-metadata targeting._
- **Container metadata extraction** (image, labels, env, volumes, network). **[v1.x]**. _Reasoning: pairs with cloud metadata._

### v2.x+ (deferred)

- **Service mesh integration** (Istio, Linkerd, Consul; SPIFFE extraction; proxy config; mTLS). **[v2.x+]**. _Reasoning: post-trial enterprise feature._
- **Edge agent mode** (local NATS leaf; offline buffer; resource-constrained ARM; low-power telemetry). **[v2.x+]**. _Reasoning: niche; ships with NATS leaf v2.x+._
- **DNS provider management** (libdns: Route53/CloudFlare/Hetzner/Azure DNS/etc.; declarative records). **[v2.x+]**. _Reasoning: see Specialized §20._
- **Advanced networking** (WiFi, 802.1X, link speed control, promiscuous mode, BMC/IPMI). **[v2.x+]**. _Reasoning: niche._

---

## 20. Specialized & Extension Domains

### v1.0 (in scope — minimum)

- **File distribution (basic)**: NATS-based file server with chunked transfer (1 MB chunks, SHA-256 per chunk, resume); local filesystem + S3-compatible backends; proxy caching with LRU+TTL. Source: `internal/files/`, `cmd/kscore-files/`. _Reasoning: agents need to fetch packages, configs, blueprints — this is operational table-stakes. Mirror groups + advanced sync defer to v1.x._
- **Self-management (basic)**: bootstrap from seed YAML (`kscore-bootstrap --seed`); full system backup (Postgres dump + JetStream + etcd + config + secrets, age-encrypted); restore from backup; `kscore-backup` CLI. Source: `internal/selfmgmt/`, `internal/backup/`, `cmd/kscore-backup/`. _Reasoning: ops-day-1 capability — disaster recovery has to work, period. Automated scheduling + rolling upgrades are v1.x._
- **Basic rate limiting** (token bucket per IP / API key / header; per-route configurable). Source: `internal/ratelimit/`. _Reasoning: protects against agent storms; trivial cost._

### v1.x (deferred)

- **File distribution: NATS Object Store + Git backend + mirror groups (geographic redundancy with read strategies + write policies)** **[v1.x]**. _Reasoning: operational scaling._
- **Self-management: automated scheduled backups + rolling upgrades + drift detection on self-config** **[v1.x]**. _Reasoning: ops automation._
- **Quota system** (per-namespace resource quotas: agents, secrets, configs, etc.) **[v1.x]**. _Reasoning: multi-tenant safety._
- **`kscore-loadtest` benchmarking** (registration / heartbeat / exec / state-apply scenarios; metric reports). **[v1.x]**. _Reasoning: capacity planning + regression testing._

### v2.x+ (deferred)

- **Proxy agents** (Epic 21, 42 — manage unmanaged devices over SSH, SNMP v2c/v3, REST/HTTP, WinRM, NETCONF, RESTCONF, gNMI, Telnet). Source: `internal/proxy/`, `internal/protocols/`, `internal/vendors/`. **[v2.x+]**. _Reasoning: large surface (8 protocols × 20 vendors); SSH+SNMP+REST core first, more protocols/vendors in later waves; not blocking commercial trial of "manage Linux/K8s fleet"._
  - **v2.x+**: SSH, SNMP, REST/HTTP adapters; vendor adapters for Cisco IOS, Juniper JUNOS, Arista EOS, pfSense, OPNsense, VyOS.
  - **v2.x+**: WinRM, NETCONF, RESTCONF, gNMI, Telnet; remaining 14 vendor drivers.
- **Air-gapped deployments** (offline registry, bootstrap packages, upgrade archives, transfer tooling, UDP data diode). Source: `internal/airgap/`, `cmd/kscore-bootstrap`, `cmd/kscore-transfer/`. **[v1.x baseline; v3.x+ data diode]**. _Reasoning: regulated/government use case; depends on registry + upgrade maturity._
- **Federation** (SPIFFE trust-domain federation, multi-cluster trust bundles). **[v2.x+]**. _Reasoning: multi-cluster._
- **MCP server** (`kscore-mcp` for Claude Desktop / Claude Code / Cursor; 16 tools across capability profiles; audit attribution). Source: `internal/mcp/`, `cmd/kscore-mcp/`. **[v2.x+]**. _Reasoning: AI-assisted ops is great differentiator but not commercial-trial-blocking; depends on stable APIs._
- **Saga checkpoint resume** **[v1.x]** — already noted in Blueprints/Runbooks domain.

### v3.x+ / future (deferred)

- **Web UI / Management Console** (browser-based). **[v3.x+]**. _Reasoning: TUI + Grafana cover ops UX in v1; web UI is post-product-market-fit work._
- **Blueprint marketplace** (community sharing). **[v3.x+]**. _Reasoning: depends on adoption._
- **Multi-cloud test matrix** (real-AWS / real-GCP / real-Azure CI). **[v3.x+]**. _Reasoning: cost-heavy; mocks suffice for v1._
- **Cross-platform expanded test matrix** (BSDs, Solaris, etc.). **[v3.x+]**. _Reasoning: niche._
- **UDP data diode** (one-way transfer with FEC). **[v3.x+]**. _Reasoning: military / classified-network use case._

---

