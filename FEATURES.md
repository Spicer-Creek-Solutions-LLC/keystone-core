# FEATURES.md

Complete feature inventory for Keystone Core, organized by domain. Each feature carries a **priority-bucket** tag (`v1.0`, `v1.x`, `v2.x+`, `v3.x+/future`) and a one-line reasoning note.

> **Versioning note.** Tags here are **priority buckets, not pinned minor releases.** [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) is canonical: there is deliberately *no* pre-allocated table of v0.2/v0.3/v1.1/v1.8 contents. Bucket meanings:
>
> - **`v1.0`** — in the v0.x → v1.0 MVP scope; lands incrementally across the v0.x line, frozen at the v1.0 SemVer commitment.
> - **`v1.x`** — post-v1.0 additive features on the stable line. Not scheduled to a specific minor.
> - **`v2.x+`** — architectural post-v1.0 (federation, multi-region, supercluster, cloud KMS).
> - **`v3.x+/future`** — marketplace, web UI, multi-cloud.
>
> When precise sequencing matters, defer to the ranked entries in [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) (`gate-v0.5` | `gate-v1.0` | `v0.x` | `v1.x` | `v2.x+`). See PROJECT-DETAILS.md §6 for the version-strategy summary.
>
> **MVP target.** v1.0 must be **clusterable** and **commercial-trial-ready** for sysadmin/IT-admin users — i.e. ~90% of what they need for day-to-day work. This drives several capabilities (plugin/module system, base stdlib, real auth, audit, HA, observability) into v1.0 that a leaner control-plane MVP would defer. The work lands incrementally across the v0.x line; v1.0 is the SemVer stability commitment at the end.
>
> **Source paths** are relative to the existing Keystone Core repo. They identify where the *current* implementation lives, not where the new build will put it.

---

## Domain Index

> **Status markers** below reflect v0.1.x reconstruction state. `landed` = the v1.0-scope items in that section work; `landed, with gaps` = code is in place but specific gate-v1.0 boot-wiring or durable-store items remain (tracked in `docs/project/ROADMAP.md`). See each section's body for landed-item annotations.

1. [Foundations & Build System](#1-foundations--build-system) *(landed)*
2. [NATS Messaging](#2-nats-messaging) *(landed)*
3. [Storage Layer](#3-storage-layer) *(landed)*
4. [Control Plane Core](#4-control-plane-core) *(landed)*
5. [API Surface (gRPC + REST)](#5-api-surface-grpc--rest) *(landed)*
6. [Agent Runtime](#6-agent-runtime) *(landed)*
7. [Remote Execution & Targeting](#7-remote-execution--targeting) *(landed)*
8. [State Management & Stdlib Modules](#8-state-management--stdlib-modules) *(landed)*
9. [Event System](#9-event-system) *(landed)*
10. [Identity & Auth](#10-identity--auth) *(landed)*
11. [Secrets Management](#11-secrets-management) *(landed)*
12. [Audit & Policy](#12-audit--policy) *(landed)*
13. [GitOps Integration](#13-gitops-integration) *(landed, with gaps)*
14. [Outbound Webhooks](#14-outbound-webhooks) *(landed, with gaps)*
15. [Clustering & HA](#15-clustering--ha) *(landed, with gaps)*
16. [Observability](#16-observability) *(landed)*
17. [Blueprints & Runbooks](#17-blueprints--runbooks) *(landed, with gaps)*
18. [Plugin / Module System](#18-plugin--module-system) *(landed, with gaps)*
19. [Multi-Environment](#19-multi-environment) *(landed)*
20. [Specialized & Extension Domains](#20-specialized--extension-domains) *(landed)*

---

## 1. Foundations & Build System

### v1.0 (in scope)

- **Cross-platform builds** (linux/darwin/windows × amd64/arm64) via Makefile. Source: `Makefile`. *Reasoning: required for any commercial trial.*
- **Pure-Go build (CGO_ENABLED=0)**. Source: `Makefile`, `pkg/dbutil/`. *Reasoning: enables Alpine/scratch images, simple cross-compile; non-negotiable for distribution.*
- **Buf-based proto codegen** (Go + gRPC plugins). Source: `buf.yaml`, `buf.gen.yaml`. *Reasoning: API surface depends on it.*
- **Build-time version injection** (`pkg/version`: Version, GitCommit, BuildDate). Source: `pkg/version/`, `Makefile` LDFLAGS. *Reasoning: support window + bug reports require it.*
- **Semver library** (`pkg/semver`: Parse, constraints, Diff). Source: `pkg/semver/`. *Reasoning: needed by module system and upgrade logic in v1.*
- **Cancelable wait/poll utilities** (`pkg/wait`). Source: `pkg/wait/`. *Reasoning: tiny, used everywhere.*
- **koanf-based config** (YAML + env vars, `KSCORE_` prefix). Source: `internal/config/`. *Reasoning: clean strict-unmarshal semantics, lighter deps than Viper. Cobra remains for CLI parsing.*
- **Structured logging** (`log/slog`, JSON/logfmt/text, correlation IDs). Source: `internal/logging/`. *Reasoning: zero-config observability baseline; stdlib eliminates a third-party dep.*
- **Standard error model** (`pkg/api/apierror`: code, message, details). Source: `pkg/api/apierror/`. *Reasoning: REST + gRPC error consistency.*
- **Make targets**: `build`, `test`, `lint`, `proto`, `dev` (hot-reload), `e2e-up/down`, `release-snapshot`. Source: `Makefile`. *Reasoning: minimum dev loop.*
- **Goreleaser snapshot** (multi-arch tarballs). Source: `.goreleaser.yaml`. *Reasoning: distribution baseline.*
- **Pre-commit hooks** (gofmt, golangci-lint, smoke). Source: `.pre-commit-config.yaml`. *Reasoning: quality gate.*
- **Baseline lint set** (errcheck, govet, staticcheck, gosec, bodyclose). Source: `.golangci.yml`. *Reasoning: catches real bugs without slowing dev.*
- **Single-topology E2E** (all-in-one docker-compose). Source: `test/e2e/single/`, Makefile, required CI gate. *Reasoning: smoke test for releases. **Landed via epic-19 task 2 (2a/2b/2c)**: 11 `TestE2E_*` scenarios over server + 2× agent + Postgres + NATS — infrastructure health, agent registration via PSK bootstrap + heartbeats, command exec, state apply, blueprint apply (server-local Applier), multi-stdlib module execution, secrets KV round-trip, audit log query + compliance report, outbound webhook delivery, GitOps inbound webhook ingest, GitOps rollback FSM. Several gate-v1.0 ROADMAP items remain open (remote-fleet blueprint apply, module system boot wiring, K8s/ArgoCD rollback executors, Vault transit + leases) but the v1.0 baseline is in place.*

### v0.5 (pulled forward from v1.x)

- **Documentation site** (Hugo + Docsy). Source: `docs/`. **[v0.5]**. *Reasoning: pulled forward from v1.x because the v0.5 external-tester audience benefits from a polished, searchable doc experience; pre-v0.5 README + reference docs remain sufficient for v0.1.x invitees. PDF export stays v1.x (see below).*

### v1.x (deferred)

- **Syslog logging output** [v1.x]. *Reasoning: ops nice-to-have; stdout suffices for trial users on systemd/k8s.*
- **PDF export of the docs site** (Hugo print pipeline). Source: `docs/`. **[v1.x]**. *Reasoning: web docs at v0.5 (above) satisfy the tester audience; PDF generation is a separate ceremony — toolchain, page-break tuning, archive-class freezing — not justified for the v0.5 milestone.*
- **HA / IPv6 / HA+IPv6 E2E topologies** [v1.x]. *Reasoning: clustering ships in v0.1 but full topology matrix can land iteratively.*
- **Hot-reload dev server (`air`)** [v1.0.x]. *Reasoning: dev-only, ship with v1.0 dot release.*
- **Repository generation (DNF/APT/Windows MSI)** [v1.x]. *Reasoning: tarballs cover trial users; package repos require infra commitment.*
- **VM bootstrap test harness** [v1.x]. *Reasoning: container E2E sufficient for v1.0.*
- **Full security scanning suite** (semgrep, trivy, syft, grype, hadolint) [v1.x]. *Reasoning: gitleaks + govulncheck + gosec sufficient for v1.0.*
- **Goreleaser signing ceremony / multi-party release** [v1.x]. *Reasoning: heavy ceremony documented in current `RELEASE-PLAYBOOK.md`; single-signer release acceptable for v1.0.*
- **Benchmark suite** [v1.x]. *Reasoning: not commercial-trial-blocking.*
- **Air-gapped repo packaging (`kscore-bootstrap`)** [v1.x]. *Reasoning: see Specialized domain.*

---

## 2. NATS Messaging

### v1.0 (in scope)

- **Embedded NATS server** (in-process, zero-dependency dev path). Source: `internal/nats/`. *Reasoning: zero-config startup is a key UX promise.*
- **External NATS cluster** (production HA path). Source: `internal/nats/`, config `nats.mode=external`. *Reasoning: required for cluster mode v1.0.*
- **Subject hierarchy** (`kscore.{cluster}.{category}.{...}`). Source: `internal/nats/`. *Reasoning: even single-cluster v1 must use cluster-prefixed subjects so v2 supercluster doesn't require refactor.*
- **Direct TCP + TLS connection strategies**. *Reasoning: covers ~95% of trial deployments.*
- **Multi-endpoint failover with health checks**. *Reasoning: clusterable means agents survive a CP node going down.*
- **Per-endpoint circuit breaker**. *Reasoning: prevents thundering herd on partial outages.*
- **Message envelope** (MessageID, CorrelationID, priority, TTL). *Reasoning: dedup + req/resp correlation.*
- **JetStream enablement** (embedded + external). *Reasoning: events + command persistence rely on it.*
- **Bootstrap registration with minimal-permission credentials**. *Reasoning: security baseline; documented in Epic 17.*
- **Connection state machine** (Disconnected → Connecting → Connected → Reconnecting). *Reasoning: deterministic agent reconnect behavior.*
- **Static endpoint configuration**. *Reasoning: simple ops path; auto-discovery deferred.*
- **Health check hooks** (Manager.Health, ConnectionManager.Health). *Reasoning: drives `/health/ready`.*

### v1.x (deferred)

- **Leaf node mode** (edge / hierarchical). **[v2.x+]**. *Reasoning: edge use case isn't a trial-day-1 feature; deployment matrix grows considerably.*
- **Supercluster / gateway** (multi-region). **[v2.x+]**. *Reasoning: same; multi-region is post-trial.*
- **WebSocket / WSS transport** (firewall traversal). **[v2.x+]**. *Reasoning: NAT traversal via TLS-on-443 mostly suffices for v1; WSS is for strict-firewall enterprises.*
- **Auto-discovery** (DNS-SRV, mDNS, K8s, Consul, etcd). **[v1.x]**. *Reasoning: static config workable at trial scale; K8s discovery lands when K8s operator does (v1.x).*
- **NAT traversal via reverse leaf** [v2.x+]. *Reasoning: niche.*
- **Exactly-once delivery** [v2.x+]. *Reasoning: at-least-once + dedup window covers vast majority of cases.*

---

## 3. Storage Layer

### v1.0 (in scope)

- **SQLite backend** (single-node dev/small). Source: `internal/state/`, `pkg/dbutil/`. *Reasoning: zero-config promise.*
- **PostgreSQL backend** (multi-node HA). Source: `internal/state/`. *Reasoning: clusterable v1 requires shared storage; SQLite cannot serve multiple servers.*
- **Pure-Go drivers** (`modernc.org/sqlite`, `lib/pq`). *Reasoning: CGO-free build constraint.*
- **Auto-schema initialization** (CREATE TABLE IF NOT EXISTS on first start). Source: `internal/state/`. *Reasoning: trial UX — no migration step on first run.*
- **Repository pattern** (Store, AgentStore, CommandStore, BatchJobStore, HealthStore interfaces). *Reasoning: decouples business logic from backend.*
- **Direct parametrized SQL** (no ORM). *Reasoning: matches existing project style; predictable performance; simpler to reason about.*
- **Connection pooling tuned per-backend** (SQLite: 1 writer; Postgres: configurable). *Reasoning: SQLite single-writer constraint is real.*
- **JSON-encoded complex columns** (labels, env vars, IPs). *Reasoning: pragmatic; avoids schema bloat for sparse fields.*
- **SQLite → PostgreSQL migration tool** (`kscore-migrate`). Source: `cmd/kscore-migrate/`, `internal/state/Migrator`. *Reasoning: trial users start on SQLite; need a no-data-loss path to scale up.*
- **Migration features**: dry-run, batch-size, txlog, progress reporter, validation. *Reasoning: production-grade migration is table stakes.*
- **IPv6-safe DSN building** (Postgres). *Reasoning: dual-stack v1 requirement.*

### v1.x (deferred)

- **Schema versioning / golang-migrate** **[v1.x]**. *Reasoning: v1 schema is new and stable; auto-DDL fine; add when first breaking change is needed.*
- **Encryption at rest** (KeyProvider, AES-GCM/CBC/ChaCha20) **[v1.x]**. *Reasoning: KeyProvider scaffolding may exist sooner, but full data-at-rest encryption with key rotation is a real project; commercial buyers expect it but not on day 1.*
- **Multi-table transaction wrapper (`Tx`)** **[v1.x]**. *Reasoning: current per-method transactions cover the only multi-step op (CompleteBatchJob); add when consistency bugs surface.*
- **Backup/restore as Store API methods** **[v1.x]**. *Reasoning: external `kscore-backup` CLI is fine for v1; integration is convenience.*
- **Query backends — Loki/Prometheus/Jaeger integration** **[v1.x]** (lands with Telemetry Gateway). *Reasoning: in-memory stubs sufficient for unit testing v1.*
- **Cloud KMS for storage encryption keys** **[v2.x+]**. *Reasoning: depends on cloud KMS work in secrets domain.*

---

## 4. Control Plane Core

### v1.0 (in scope)

- **kscore-server daemon** with strict init order (NATS → State → ConnMgr → Dispatcher → gRPC/HTTP → optional). Source: `cmd/kscore-server/`, `internal/controlplane/`, `pkg/api/server/`. *Reasoning: deterministic startup is critical for ops.*
- **Connection Manager** (agent registration, heartbeat tracking, stale detection). Source: `internal/controlplane/`. *Reasoning: core agent lifecycle.*
- **Command Dispatcher** (route, timeout, result retention). *Reasoning: core remote-execution backbone.*
- **Batch Dispatcher** (group-targeted ops, batch progress state machine). *Reasoning: required for fleet operations at any scale.*
- **Listen ports**: gRPC 5397, HTTP 8080, optional metrics, optional pprof. *Reasoning: standard split.*
- **Dual-stack (IPv4 + IPv6) listeners**. *Reasoning: minimal cost, broad applicability.*
- **Health endpoints** (`/health/live`, `/health/ready`, `/health/status`, `/api/status`). *Reasoning: K8s probes + ops dashboards.*
- **Middleware chain** (CORS → rate-limit → auth → handler). *Reasoning: standard ordering; auth as innermost so audit can log denials before handler.*
- **Graceful shutdown sequence** (gRPC → ConnMgr → State → NATS, with HTTP context timeout). *Reasoning: data-loss on shutdown destroys trial user trust.*
- **30s status ticker logging** (agent counts, health). *Reasoning: zero-config visibility into running server.*
- **Production warnings on startup** (embedded NATS, SQLite, TLS off in production). *Reasoning: makes operational risk explicit.*
- **Default zero-config startup** (embedded NATS + SQLite + dev API key + warning). *Reasoning: trial UX promise.*

### v1.x (deferred)

- **gRPC reflection / channelz** **[v1.0.x]**. *Reasoning: ship with first dot release; trivial to add.*
- **Webhook receiver port (8081)** **[v1.x]**. *Reasoning: see Webhooks domain — outbound is v1.0, inbound non-GitOps webhooks v1.x.*
- **K8s operator wiring** **[v1.x]**. *Reasoning: gates on operator domain; not v1.0.*
- **Profiling endpoint defaults-on** **[never]**. *Reasoning: leave opt-in; security default.*

---

## 5. API Surface (gRPC + REST)

### v1.0 (in scope — services)

- **AgentService**: Register, Heartbeat, ExecuteCommand (stream), GetAgentInfo. Source: `api/proto/agent.proto`. *Reasoning: agent lifecycle.*
- **ControlPlaneService**: GetServerStatus, ListAgents, GetAgent, ExecuteCommand (stream), BatchExecuteCommand (stream), command status/history. Source: `api/proto/controlplane.proto`. *Reasoning: orchestration.*
- **StateService**: ApplyState (stream), CheckState, DetectDrift, GetStateHistory. *Reasoning: core IaC capability.*
- **EventService**: ListEvents, EmitEvent, SubscribeEvents (stream), GetEventStats. *Reasoning: observability + audit trail.*
- **PolicyService** (audit-only in v1.0): EvaluatePolicy, ListViolations, GetAuditLog, GetComplianceReport. *Reasoning: audit alone is valuable for compliance-curious trial users; full enforcement deferred.*
- **SecretsService**: GetSecret, ListSecrets, WriteSecret, DeleteSecret, lease ops, transit encrypt/decrypt/sign/verify. *Reasoning: any sysadmin trial requires real secrets handling.*
- **ClusterService**: GetClusterStatus, ListMembers, GetLeader, TransferLeader, WatchMembership (stream), WatchLeadership (stream). *Reasoning: clusterable v1.*
- **CoordinationService** (mTLS-only server-to-server): ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, PropagateState. *Reasoning: cluster recovery during NATS partition.*
- **AuthN**: API key (Bearer), JWT, mTLS — Principal model, Authorizer interface, role hierarchy (admin > operator > readonly). Source: `pkg/api/auth/`. *Reasoning: minimum auth set for commercial trial.*
- **REST endpoints (v1.0 wired)**: `/health/*`, `/api/status`, `/api/v1/agents`, `/api/v1/commands`, `/api/v1/state/*`, `/api/v1/secrets/*`, `/api/v1/policies` (audit), `/api/v1/cluster/*`, `/api/v1/apikeys`, `/api/v1/audit`, `/api/v1/runbooks`. *Reasoning: REST is the on-ramp for ad-hoc tooling, dashboards, scripts.*
- **Streaming patterns**: server-stream for command output, batch progress, state apply progress, event subscription, membership/leadership watch. *Reasoning: real-time UX matters.*
- **Standardized pagination** (page_size + page_token + total_count). *Reasoning: large fleets.*
- **Standard error model** (`pkg/api/apierror`). *Reasoning: ecosystem consistency.*
- **Versioning registry** (`pkg/api/versioning`: current/supported/deprecated/retired/beta/alpha + dates). *Reasoning: support window enforcement.*
- **gRPC ↔ REST mapping**: hand-coded REST handlers (no grpc-gateway annotations in protos). *Reasoning: matches existing structure; gateway adoption can be evaluated v2.*

### v1.x (deferred)

- **MaintenanceService** (maintenance windows API) **[v1.x]**. *Reasoning: scheduling domain — see v1.x.*
- **ScheduleService** (job scheduling API) **[v1.x]**. *Reasoning: same.*
- **RunbookService gRPC** **[v1.0]** (REST wired in v1.0). *Reasoning: keep both for v1.*
- **WebhookService** (REST handler exists, not wired) **[v1.0 outbound]** + **[v1.x inbound non-GitOps]**. *Reasoning: outbound is in v1.0; non-GitOps inbound subscriptions slide to v1.x.*
- **GitOps webhook REST handlers wiring** **[v1.0]**. *Reasoning: GitOps domain — included in v0.1.*
- **MirrorService / DiscoveryService** (proxy-related) **[v2.x+]**. *Reasoning: see Specialized.*
- **Full RBAC (`pkg/api/rbac`)** **[v1.x]**. *Reasoning: simple admin/operator/readonly hierarchy ships v1.0; full RBAC is a separate epic.*
- **gRPC-gateway adoption** (auto-generated REST) **[v2.x+]**. *Reasoning: hand-coded handlers work for v1; reduces churn during initial implementation.*
- **OpenAPI auto-generation from protos** **[v2.x+]**. *Reasoning: hand-maintained YAML acceptable for v1.*

---

## 6. Agent Runtime

### v1.0 (in scope)

- **`kscore-agent` daemon** (Linux amd64/arm64). Source: `cmd/kscore-agent/`, `internal/agent/`. *Reasoning: a Linux agent is the foundational sysadmin trial UX.*
- **Agent registration** (with hardware/OS metadata; auto-gen or explicit ID). *Reasoning: lifecycle entry point.*
- **Heartbeat loop with system metrics** (CPU, memory, disk %; default 30s). *Reasoning: liveness + drives stale detection on server.*
- **Continuous metadata collection** (distro, kernel, CPU, memory, NIC, dual-stack, container/VM detection). Source: `internal/agent/metadata.go`. *Reasoning: powers targeting + drift.*
- **Command execution engine** (timeout, working dir, env, user-switch, SIGTERM→SIGKILL grace). Source: `internal/agent/executor.go`. *Reasoning: core remote-exec endpoint.*
- **Security enforcement** (HMAC-signed commands, allowlist/blocklist, blocked patterns, env restrictions). Source: `internal/agent/security.go`. *Reasoning: trial users need explicit safety; defaults must be strict.*
- **TUI-guided bootstrap** (Bubble Tea, demo/production/enterprise modes). Source: `cmd/kscore-bootstrap/`, `cmd/kscore-agent/.../bootstrap`. *Reasoning: per Epic 27 — single-binary "answer 5 questions" install is a Salt-parity table-stake.*
- **Non-interactive bootstrap** (CLI flags + env vars). *Reasoning: automation/Ansible-driven installs need this.*
- **Bootstrap phases** (detect → configure → validate → install → verify with rollback on failure). *Reasoning: safe re-runs.*
- **Systemd service install + management**. *Reasoning: standard Linux daemon UX.*
- **Self-signed CA bootstrap path** (demo) + CSR path (production). *Reasoning: zero-config dev + real-prod story.*
- **Agent config on disk** (`/etc/keystone-core/keystone-core-agent.yaml`, certs in `certs/`). *Reasoning: standard FHS layout.*
- **Graceful shutdown** (SIGTERM → unsubscribe, drain in-flight, exit). *Reasoning: clean rolling restarts.*
- **Reconnect with exponential backoff**. *Reasoning: handles flaky control plane links.*
- **Plugin host integration** (loads from module system — see Domain 18). *Reasoning: required for state apply + extensibility.*
- **State runner integration** (apply/check/drift modules locally). *Reasoning: see Domain 8.*

### v1.x (deferred)

- **Embedded NATS / hybrid mode (agent as host or leaf)** **[v2.x+]**. Source: `internal/agent/nats_server.go`, hybrid_mode_state_machine. *Reasoning: edge / disconnected scenarios; v1 assumes control plane is reachable. Carries significant FSM complexity.*
- **Endpoint advertiser + reverse-leaf NAT traversal** **[v2.x+]**. *Reasoning: same — niche.*
- **Windows agent** (native service) **[v1.x]**. *Reasoning: deferred per state-mgmt categorization; Windows stdlib lands together in v1.x.*
- **macOS agent** **[v1.x]**. *Reasoning: low trial-population; defer behind Windows.*
- **Interactive shell sessions** **[v1.x]**. *Reasoning: nice-to-have, not core.*
- **VM-based bootstrap test harness** **[v1.x]**. *Reasoning: Docker CI sufficient for v1.0.*
- **Auto-rotation of NATS creds in memory** **[v1.x]**. *Reasoning: gates on SPIRE/identity rotation.*

---

## 7. Remote Execution & Targeting

### v1.0 (in scope)

- **Single-agent execution** (gRPC `ExecuteCommand` stream). Source: `internal/execution/`, `pkg/api/v1`. *Reasoning: most basic op.*
- **Batch execution across target expressions** (concurrency-limited semaphore, configurable). Source: `internal/targeting/batch.go`, `internal/controlplane/batch_dispatcher.go`. *Reasoning: fleet ops table-stake.*
- **Streaming output protocol** (BATCH_START → AGENT_START → AGENT_OUTPUT → AGENT_COMPLETE → BATCH_COMPLETE + summary). *Reasoning: real-time UX.*
- **Targeting by hostname glob** (`web-*`, `db-prod-*`). Source: `internal/targeting/`. *Reasoning: classic Salt UX.*
- **Targeting by labels** (`role:web`, `env:prod`). *Reasoning: modern label-based selectors.*
- **Targeting by built-in fields** (os, arch, status, ip, hostname). *Reasoning: expected sysadmin UX.*
- **Compound expressions** (AND/OR/NOT, parens). Source: uses `expr-lang/expr`. *Reasoning: code is already there; cost negligible to enable in v1.0; users expect it from a Salt-parity tool.*
- **Dry-run mode** (show matched agents without dispatching). *Reasoning: safety.*
- **Job tracking** (UUID, persistence, status, history). *Reasoning: audit + retrospect.*
- **Cancellation** (SIGTERM 5s grace → SIGKILL). *Reasoning: stuck job recovery.*
- **Cross-platform shell abstraction** (bash, sh, PowerShell, cmd; auto-detect). Source: `internal/execution/shell.go`. *Reasoning: matches multi-OS state stdlib.*
- **Continue-on-failure flag** (default on for batch). *Reasoning: matches Salt expectations.*
- **Command policy** (Strict/Normal/Permissive; allow/block lists; block shell metacharacters). Source: `internal/execution/policy.go`. *Reasoning: defense in depth.*
- **`kscorectl exec run|async|status|list|cancel|output|script` CLI**. Source: `cmd/kscore-exec/`. *Reasoning: primary trial UX surface.*

### v1.x (deferred)

- **Fact-based selectors** (`facts.memory > 16Gi`) **[v1.x]**. *Reasoning: requires stable agent fact schema first; expression engine already supports it once facts land in metadata.*
- **Percentage-based / rolling batches** (`--batch 10%`) **[v1.x]**. *Reasoning: nice-to-have; concurrency limit covers most use cases.*
- **Output archival to object storage (S3/GCS) cold-tier** **[v1.x]**. *Reasoning: cost optimization; v1 stores in DB.*
- **Interactive shell over stream** **[v1.x]**. *Reasoning: see Agent Runtime.*

---

## 8. State Management & Stdlib Modules

### v1.0 (in scope — declarative engine)

- **Declarative state DSL** (YAML; metadata, includes, variables, declarations). Source: `internal/statemgmt/`. *Reasoning: core IaC UX.*
- **Module interface** (Name, ValidStates, Check, Apply, Test). *Reasoning: extension point.*
- **Requisite system** (require, require_in, watch, watch_in, prereq, prereq_in, onchanges, onchanges_in). *Reasoning: dependency management is what separates "config push" from "state engine".*
- **DAG resolver with cycle detection**. *Reasoning: deterministic apply order; clear error reporting.*
- **Go template rendering** (vars, facts, custom filters: upper/lower/title/trim/join/split/default). *Reasoning: parameterization.*
- **Drift detection with severity** (none/low/medium/high/critical). *Reasoning: continuous compliance promise.*
- **Cross-platform dispatch** (runtime.GOOS-based provider selection). *Reasoning: stdlib breadth.*
- **Dry-run / check mode**. *Reasoning: safety.*
- **Audit + event emission per state apply**. *Reasoning: compliance trail.*
- **History store** (SQLite-backed; query past runs). *Reasoning: rollback + audit.*
- **State runner pipeline** (parse → validate → resolve → check → apply → test → report). *Reasoning: deterministic, debuggable engine.*
- **Saga / checkpoint integration** (Epic 58 — minimal v1; advanced v1.x). *Reasoning: long-running multi-step state needs ordered compensation.*

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

*Reasoning: this set covers ~90% of universal Linux sysadmin daily tasks. Sysadmin trial users can replace Salt formulas / Ansible playbooks for their core workflow.*

### v1.x (deferred — Linux extended + Windows + containers + DBs)

- **Windows-native modules**: `win_feature`, `win_firewall`, `win_registry`, `win_service`, `win_package`. *Reasoning: ships with Windows agent in v1.x.*
- **Container modules**: `docker_container`, `docker_image`, `docker_network`, `docker_volume`. *Reasoning: ops feature; not blocking sysadmin trial.*
- **Web servers**: `web` (nginx/Apache abstraction). *Reasoning: same.*
- **Database admin**: `postgres_database`, `mysql_database`, `redis`. *Reasoning: same.*
- **macOS-specific scheduling**: `launchd`. *Reasoning: macOS agent is v1.x.*

### v2.x+ (deferred)

- **Kubernetes modules** (`k8s_deployment`, `k8s_statefulset`, `k8s_daemonset`, `k8s_job`, `k8s_cronjob`, `k8s_service`, `k8s_ingress`, `k8s_configmap`, `k8s_secret`, `k8s_namespace`, `k8s_pvc`, `k8s_hpa`). **[v2.x+]**. *Reasoning: cloud-native; depends on K8s operator (v1.x) maturing first.*
- **DNS provider modules** (Route53, CloudFlare, Hetzner, etc.). **[v2.x+]**. *Reasoning: see DNS provider domain.*
- **Niche networking** (`promisc`, `wifi`, `dot1x`, `scheduled_task` Windows). **[v2.x+]**. *Reasoning: niche.*
- **Vendor-specific modules** (Cisco IOS, Juniper, etc.). **[v2.x+]**. *Reasoning: proxy-agent territory.*

---

## 9. Event System

### v1.0 (in scope)

- **Event bus on NATS JetStream** (KSCORE_EVENTS stream, `kscore.events.>` subjects). Source: `internal/events/`. *Reasoning: fan-out + replay foundation.*
- **Event types**: 29 across 7 categories (agent×5, job×4, state×5, system×3, user×3, policy×2, runbook×7). *Reasoning: covers v1 emit needs from all domains; runbook×7 added in Epic 15 task 9 for runbook lifecycle/step observability.*
- **Event struct** (ID, Type, Source, Time, Severity, CorrelationID, Tags, Data, Subject). *Reasoning: standard event envelope.*
- **EventStore** (SQL-backed; query, retention, batching). *Reasoning: long-term query + compliance.*
- **EventPublisher / EventSubscriber interfaces**. *Reasoning: pluggable transport.*
- **Filter expressions** (homegrown CEL-like parser; field comparison, AND/OR/NOT, regex, glob). *Reasoning: routing, subscription, queries depend on it. Note: rebuild can adopt `google/cel-go` directly instead of homegrown — see PROJECT-DETAILS §4.9.*
- **gRPC EventService** (List, Get, Emit, Subscribe stream w/ optional historical replay, GetEventTypes, GetEventStats). *Reasoning: client API surface.*
- **CLI** (`kscore-events list|query|emit|subscribe|watch|storage-stats`). *Reasoning: ops visibility.*
- **Audit emission integration** (events for auth/policy/user actions). *Reasoning: compliance trail.*
- **Retention policies** (per-type, age + count limits). *Reasoning: cost control.*
- **Correlation IDs** (group related events; passed through gRPC contexts). *Reasoning: tracing across multi-step ops.*
- **Severity levels** (debug/info/warn/error/critical). *Reasoning: filtering + alerting.*

### v1.x (deferred — automation layer)

- **Reactor engine** (filter → action; LogAction/EventAction/WebhookAction; throttle/debounce). **[v1.x]**. *Reasoning: valuable but overlaps with runbooks/policy; hold one release for clean separation of concerns.*
- **Lifecycle tracking** (created → published → routed → processing → processed/failed/expired). **[v1.x]**. *Reasoning: debugging tool; ships with reactors.*
- **Enrichment pipeline** (tag/data/conditional enrichers). **[v1.x]**. *Reasoning: paired with reactors.*
- **Dead-letter queue** (failed reactor exec retry). **[v1.x]**. *Reasoning: paired with reactors.*

### v2.x+ (deferred)

- **Kafka integration** (sarama producer; CloudEvents serialization). **[v2.x+]**. *Reasoning: enterprise integration; not blocking.*
- **CloudEvents 1.0 marshaling**. **[v2.x+]**. *Reasoning: standard adoption.*
- **Inbound webhook receiver for events** (HMAC, signature). **[v1.x]**. *Reasoning: minor scope.*
- **Object-storage archival (S3/GCS)**. **[v2.x+]**. *Reasoning: cost optimization.*
- **Multi-region replication**. **[v3.x+]**. *Reasoning: post-supercluster.*

---

## 10. Identity & Auth

### v1.0 (in scope)

- **API keys** (random base62, SHA-256 hashed at rest, expiry, role assignment). Source: `pkg/api/apikeys/`, `pkg/api/auth/`. *Reasoning: simplest auth path; trial UX.*
- **API key rotation + revocation**. *Reasoning: real auth lifecycle.*
- **mTLS** (X.509 v3 with SPIFFE URI SANs, TLS 1.3 default min). Source: `internal/security/`, `pkg/api/auth/`. *Reasoning: server-to-server + agent identity.*
- **Embedded CA** (root CA 10y default + signing CA 1y, auto-rotate at 30d before expiry). Source: `internal/identity/ca.go`, `internal/pki/`. *Reasoning: zero-config UX; defers SPIRE complexity.*
- **SPIFFE-shaped identities from day 1** (trust domain default `kscore.local`; agent/server/service paths). Source: `internal/identity/`. *Reasoning: cheap to do at start; expensive to retrofit. v1.x SPIRE swap-in is then near-trivial.*
- **Embedded identity provider** (CA + SVID issuer + attestation engine + token store). *Reasoning: trial-day-1 zero-deps.*
- **JWT** (HS/RS/ES family; configurable role claim). *Reasoning: integration with external IdPs.*
- **Cluster join tokens** (SHA-256 stored, TTL default 5m, max-uses, leader-issued). Source: `internal/identity/`, Epic 44, `cmd/kscore-identity/token`. *Reasoning: secure cluster bootstrap.*
- **RBAC** (admin/operator/readonly hierarchy; method→role map; bypass list). Source: `pkg/api/rbac/`. *Reasoning: minimum viable RBAC; full role/permission CRUD ships v1.x.*
- **Auth interceptor chain** (gRPC unary + stream; rate-limit on failure with exp backoff; audit logging). *Reasoning: enforcement entry point.*
- **`kscore-identity` CLI** (token CRUD, CA info/rotate/export, status). Source: `cmd/kscore-identity/`. *Reasoning: ops admin needs visibility.*
- **Cert auto-rotation at ~50% lifetime** (agent-driven). *Reasoning: zero-touch cert lifecycle.*
- **TLS 1.3 default** (1.2 opt-in for legacy). *Reasoning: secure default.*

### v1.x (deferred)

- **Full RBAC role/permission CRUD with per-resource permissions** **[v1.x]**. *Reasoning: trio (admin/operator/readonly) covers ~80% of trials; full RBAC is a real epic.*
- **Trust federation** (bundle endpoint for cross-domain trust) **[v1.x]**. *Reasoning: multi-site adoption follows trial.*
- **SPIRE integration** (external SPIRE server, agent-socket attestation, K8s SAT/AWS IID/etc.) **[v1.x]**. *Reasoning: SPIRE adds 3+ ops setup steps; trial users use embedded provider.*

### v2.x+ (deferred)

- **Cloud workload identity** (AWS IAM/IRSA, GCP WI, Azure MI). **[v2.x+]**. *Reasoning: multi-cloud follows mature single-domain.*
- **Service mesh integration** (Istio, Linkerd, Consul Connect identity extraction). **[v2.x+]**. *Reasoning: adds CNI/injector deps; post-trial.*
- **Multi-party CA / certificate issuance** **[v2.x+]**. *Reasoning: governance overhead; single-signer is fine for v1.*

---

## 11. Secrets Management

### v1.0 (in scope)

- **Encrypted-file backend** (AES-GCM, JSON serialization, zero external deps). Source: `internal/secrets/file/`. *Reasoning: trial-day-1 zero-deps option.*
- **HashiCorp Vault backend** (KV v1/v2, dynamic secrets, transit, namespace support). Source: `internal/secrets/vault/`. *Reasoning: highest-ROI; most common backend in trial environments.*
- **SecretBroker with path-prefix routing** (longest-prefix-first). Source: `internal/secrets/broker.go`. *Reasoning: multi-backend coordination from day 1.*
- **CRUD via REST + gRPC + CLI**. Source: `pkg/api/secrets/`, `pkg/api/v1/secrets.pb.go`, `cmd/kscore-secrets/`. *Reasoning: fully implemented; gating costs more than shipping.*
- **Lease management** (persistent SQLite, eager/lazy/on-demand renewal strategies, scheduler, callbacks). Source: `internal/secrets/lease_manager.go`. *Reasoning: dynamic secrets without lease management is broken.*
- **Transit operations** (encrypt/decrypt/sign/verify/HMAC, batch ops, key versioning, convergent encryption). Source: `internal/secrets/vault/transit.go`. *Reasoning: encryption-as-a-service for app-layer integration; gRPC API exists; cost to ship is near zero.*
- **Encrypted in-memory cache** (AES-GCM, TTL eviction, path-prefix invalidation, stats). Source: `internal/secrets/cache.go`. *Reasoning: latency reduction is critical at scale.*
- **Audit emission integration** (every secret access event with agent ID, SPIFFE ID, action, timestamp). Source: `internal/secrets/audit.go`. *Reasoning: compliance trail.*
- **CLI** (`kscore-secrets get|list|backends|audit|leases|cache|encrypt|decrypt|template`). *Reasoning: ops + integration UX.*
- **Secret masking in API responses + logs**. *Reasoning: leak prevention default.*

### v1.x (deferred)

- **Rotation orchestration with strategies** (blue-green / rolling / canary / immediate; health checks; auto-rollback) **[v1.x]**. Source: `internal/secrets/rotation.go`. *Reasoning: powerful but complex; ship after v1.0 stabilizes; depends on healthy verification framework (GitOps domain v1.x features).*
- **Cron-based rotation scheduling + Slack/PagerDuty notifications** **[v1.x]**. *Reasoning: ships with rotation orchestrator.*
- **Compliance reports + anomaly detection** **[v1.x]**. *Reasoning: paired with rotation; same release.*

### v2.x+ (deferred)

- **AWS Secrets Manager backend** **[v2.x+]**. *Reasoning: cloud-vendor backend; ships with cloud KMS work.*
- **Azure Key Vault backend** **[v2.x+]**. *Reasoning: same.*
- **GCP Secret Manager backend** **[v2.x+]**. *Reasoning: same.*
- **Cloud KMS for master keys** (AWS KMS / Azure HSM / GCP KMS, envelope encryption) **[v2.x+]**. *Reasoning: depends on cloud SDKs; v1 master keys are file-based or Vault-transit-derived.*
- **Hardware HSM support** (PKCS#11, Thales Luna, AWS CloudHSM) **[v2.x+]**. *Reasoning: enterprise compliance; niche.*
- **L2 KMS-backed cache** **[v2.x+]**. *Reasoning: ships with cloud KMS.*

---

## 12. Audit & Policy

### v1.0 (in scope — audit + policy infrastructure, audit-mode-only enforcement)

- **Audit logger** (`internal/audit/`) — structured events for all sensitive ops (auth decisions, secret access, state apply, command exec, policy evaluations). *Reasoning: compliance-curious trial users will run an audit query in week one.*
- **Audit storage** (in-memory circular buffer + SQLite persistent backend, retention policy, redaction config). Source: `internal/policy/audit.go`, `SQLitePolicyAuditStore`. *Reasoning: query + compliance reports require persistence.*
- **Audit query API** (`AuditFilter` — actor, resource, time range, severity; pagination). *Reasoning: ops investigation UX.*
- **Audit export** (JSON / JSONL / CSV). *Reasoning: hand-off to SIEM tools.*
- **`kscore-audit` CLI** (`log`, `report`, `export`, `stats`, `search`, `analyze`, `timeline`, `watch`). *Reasoning: ops surface.*
- **Policy engine infrastructure** (Engine + Registry + 3 evaluators: OPA, CEL, Builtin). Source: `internal/policy/`. *Reasoning: shipping the engine in audit-mode is cheap; v1.x just flips the enforce flag.*
- **OPA Rego evaluator** (`open-policy-agent/opa` v1.16.2, embedded `v1/rego`; Rego v1 syntax; fixed `package keystone.policy`; `http.send`/`net.*`/`opa.runtime` builtins denied; compiled-query cache). *Reasoning: standard policy language for compliance shops; restricted builtins keep operator-supplied policies pure decision logic.*
- **CEL evaluator** (`google/cel-go`; `input`/`resource`/`action`/`user`/`context` variables). *Reasoning: lighter-weight inline policies.*
- **Builtin policies** (require-labels, require-owner, allowed-environments, allowed-actions, deny-privileged, time-window, no-root-execution, etc.). *Reasoning: ship with sensible defaults.*
- **`Policy{ID, Name, Type, Category, Severity, EnforcementMode, Code, Enabled, Tags}`** model. *Reasoning: full schema in v0.1 even if enforcement is gated.*
- **`PolicySet`** (groups; set-level enforcement override). *Reasoning: same.*
- **`Bindings`** (attach policies to resource types, optional action/selector). *Reasoning: same.*
- **Policy evaluation API** (single + set + by-resource-type). *Reasoning: callers can evaluate; v1.0 just doesn't *block* on results.*
- **Compliance reports** (period, compliance rate, per-policy stats, top violations, severity distribution, trend points). *Reasoning: this is the audit user's payoff.*
- **Compliance framework mappings** (CIS, SOC2, NIST-800-53, HIPAA, PCI-DSS, GDPR, ISO-27001, Custom). *Reasoning: turnkey compliance value.*
- **`PolicyService` gRPC** (Evaluate, EvaluatePolicySet, ListViolations, GetComplianceReport, GetAuditLog). *Reasoning: full RPC surface; CRUD methods present but server returns Unimplemented.*
- **`kscore-policy` CLI v1.0 subset** (`list`, `validate`, `check`, `show`, `eval`, `test`, `compliance`, `violations`). *Reasoning: ops trial UX.*

### v1.x (deferred — full enforcement)

- **Enforcement modes: Enforce + Warn (active blocking)** **[v1.x per user direction]**. *Reasoning: misconfigured policy can break a fleet — needs proven audit-mode track record + simulation tooling first.*
- **Enforcement actions** (Block, Warn, Audit, Remediate). **[v1.x]**. *Reasoning: blocking semantics belong with approval workflow infra.*
- **Pre/post-execution hooks** (state apply + command dispatch interception). **[v1.x]**. *Reasoning: depends on enforcement.*
- **Approval workflows for policy violations**. **[v1.x]**. *Reasoning: human-in-the-loop.*
- **`kscore-policy create|update|delete|activate|deactivate|remediate|monitor`**. **[v1.x]**. *Reasoning: full CRUD ships with enforcement.*
- **Policy persistence** (etcd or Postgres; dynamic reload). **[v1.x]**. *Reasoning: in-memory registry is fine for v1.0 audit-only.*

### v1.x other

- **Continuous compliance scan scheduler** **[v1.x]**. *Reasoning: cron-based eval; depends on schedule infrastructure.*
- **CEL custom function library** **[v1.x]**. *Reasoning: power-user feature.*
- **Anomaly detection (audit log analysis)** **[v1.x]**. *Reasoning: ships with rotation/security work.*

---

## 13. GitOps Integration

### v1.0 (in scope — basics for sysadmin trial)

- **Webhook receiver** (HTTP server, configurable addr/path, source auto-detection). Source: `internal/gitops/webhook/`. *Reasoning: GitOps integration is a key trial differentiator.*
- **ArgoCD webhook handler** (application sync/health/deployment events). *Reasoning: most common GitOps tool.*
- **Flux webhook handler** (Kustomization/HelmRelease events). *Reasoning: same.*
- **GitHub webhook handler** (deployment, deployment_status, workflow_run, push). *Reasoning: trial users use GitHub.*
- **GitLab webhook handler** (push, deployment, pipeline). *Reasoning: GitLab is common in EU.*
- **Webhook authentication** (HMAC-SHA256, Bearer token). *Reasoning: secure-by-default.*
- **Event normalization** (provider events → unified `webhook.Event` → emit on Keystone event bus as `gitops.{argocd|flux|github|gitlab}.*`). *Reasoning: consumed by reactors (v1.x) and audit.*
- **Verification engine** (HTTP, gRPC, command verifiers; sequential or parallel; retries + timeout per step). Source: `internal/gitops/verification/`. *Reasoning: deployment verification is the Keystone runtime-control-plane value prop.*
- **Verification workflow execution** (`Verifier` interface + plugin registration). *Reasoning: extension point.*
- **Manual rollback API + CLI** (REST `/api/v1/gitops/rollback`, `kscore-gitops rollback`). *Reasoning: must-have for "verify then rollback" trial story.*
- **Rollback executors** (Git revert, ArgoCD sync to revision, K8s rollout undo). *Reasoning: all common patterns.*
- **Approval workflow for rollback** (Pending → Approved/Rejected → InProgress → Completed/Failed → Verifying). *Reasoning: human-in-the-loop is non-negotiable for prod.*
- **Verification result storage + REST list/get** (`/api/v1/gitops/verifications`). *Reasoning: history + audit.*
- **`kscore-gitops` CLI** (`verify`, `rollback`). *Reasoning: trial UX.*

### v1.x (deferred — automation)

- **Multi-env promotion pipelines** (sequential dev → staging → prod with approvals). **[v1.x]**. *Reasoning: powerful; needs design time. Foundational rollback works in v0.1.*
- **Promotion state machine + REST API**. **[v1.x]**. *Reasoning: same.*
- **Basic remediation strategies** (rollback action). **[v1.x]**. *Reasoning: paired with promotion.*

### v1.x (deferred — progressive delivery)

- **Canary deployments** (weight-based progression: 5/25/50/100, dwell time, threshold eval). **[v1.x]**. *Reasoning: depends on observability metrics + healthy verification framework.*
- **Threshold evaluation per canary step**. **[v1.x]**. *Reasoning: same.*
- **Advanced remediation** (scale-down, traffic shift, custom workflows). **[v1.x]**. *Reasoning: cluster integration heavy.*
- **Diagnostic collection on remediation**. **[v1.x]**. *Reasoning: same.*
- **Git sync orchestration + multi-repo coordination** **[v1.x]**. *Reasoning: separate from webhook ingest; specialized.*
- **Helm/Kustomize-native integration** **[v1.x]**. *Reasoning: extension.*
- **Deployment dependency graph** **[v1.x]**. *Reasoning: complex to design correctly.*
- **Webhook timestamp validation + nonce dedup** **[v1.x]**. *Reasoning: defense-in-depth; current HMAC-only is acceptable for v1.0.*

---

## 14. Outbound Webhooks

### v1.0 (in scope)

- **Persistent webhook subscriptions** (SQLite-backed; CRUD via REST + CLI). Source: `internal/webhook/outbound/`, `pkg/api/webhooks/`. *Reasoning: commercial trial users want Slack/PagerDuty/custom hooks on day 1.*
- **Event filter on subscriptions** (glob patterns: `agent.*`, `state.drift`, `policy.*`, `*`). *Reasoning: targeted notifications.*
- **HMAC-SHA256 signing** (GitHub-compatible `sha256=<hex>` format; per-subscription secret). *Reasoning: secure delivery.*
- **Custom HTTP headers per subscription**. *Reasoning: integrate with auth-required receivers (Slack tokens, etc.).*
- **Exponential backoff retry** (jittered, configurable max retries default 3). *Reasoning: handles transient receiver errors.*
- **Delivery history with audit trail** (status, status code, attempt #, error, timestamp). *Reasoning: ops visibility.*
- **Per-endpoint circuit breaker** (closed → open after N failures → half-open recovery). *Reasoning: protects against repeated failures.*
- **Per-subscription delivery timeout** (default 10s). *Reasoning: prevents stuck deliveries.*
- **Secret masking in API responses** (`***`). *Reasoning: leak prevention.*
- **REST API** (`/api/v1/webhooks/subscriptions` CRUD + `{id}/test` + `{id}/deliveries`). *Reasoning: standard surface.*
- **`kscore-webhook outbound` CLI** (`list`, `create`, `show`, `delete`, `history`, `test`). *Reasoning: ops UX.*
- **NATS event-bus consumer** (`>` subject; pattern-matches; async fan-out per subscription). *Reasoning: integration with Domain 9.*
- **Manager-driven async delivery** (WaitGroup, bounded goroutines for back-pressure). *Reasoning: scales.*

### v1.x (deferred)

- **Inbound webhooks for non-GitOps event sources** (custom payload ingestion + event emission). **[v1.x]**. *Reasoning: complementary to outbound; not blocking trial.*
- **Webhook body templating** (Handlebars/Jinja2-style). **[v1.x]**. *Reasoning: nice-to-have; hardcoded JSON serialization works for v1.*
- **Per-destination rate limiting**. **[v1.x]**. *Reasoning: most receivers handle bursts; explicit RL is power-user feature.*
- **Auto-cleanup of old delivery history** (currently manual). **[v1.0.x]**. *Reasoning: trivial; ship with first dot release.*

---

## 15. Clustering & HA

### v1.0 (in scope — minimum viable cluster, the v1 differentiator)

- **3-node cluster formation** (3 × `kscore-server` with embedded etcd). Source: `internal/cluster/`. *Reasoning: the v1 commercial-trial-ready bar.*
- **Embedded etcd mode** (single-binary, etcd in-process). *Reasoning: zero-deps + clusterable.*
- **External etcd mode** (3-node external etcd cluster for medium production). *Reasoning: production scaling path.*
- **etcd-based membership** (ephemeral leases, join, leave, fail detection). Source: `internal/cluster/`. *Reasoning: foundational.*
- **Heartbeat mechanism** (5s interval, 30s timeout — must be ≥3× interval). *Reasoning: avoids leader flapping.*
- **Member status state machine** (HEALTHY → DEGRADED → UNHEALTHY → LEAVING → removed). *Reasoning: clear lifecycle.*
- **etcd-based leader election** (concurrency.Election; <3s on failure). *Reasoning: standard pattern.*
- **Voluntary leadership resignation + transfer**. *Reasoning: graceful upgrades.*
- **Automatic failover** (heartbeat-loss detection <5s, batched agent reassignment, batched job reassignment, idempotency keys for dedup). *Reasoning: HA promise.*
- **Consistent hashing for agent assignment** (configurable virtual nodes default 150; minimal rebalancing on topology changes). *Reasoning: even distribution + stability.*
- **Rebalancing on member join/leave** (cooldown 5s minimum). *Reasoning: prevents rebalance storms.*
- **Singleton-task manager (leader-only)** (reactor coordinator, scheduled jobs, cleanup, metric aggregation, agent rebalance). *Reasoning: avoids duplicate work.*
- **Recovery workflow** (STARTING → CONNECTING → SYNCING → VERIFYING → REJOINING → RECLAIMING → COMPLETED). *Reasoning: deterministic restart.*
- **Graceful shutdown** (drain → transfer leadership → deregister). *Reasoning: zero-downtime upgrades.*
- **Split-brain prevention via quorum** (N/2+1; minority blocks writes). *Reasoning: data integrity.*
- **Lease + epoch fencing** (operations require valid lease; epoch increments on election; stale epochs rejected). *Reasoning: defense-in-depth against split-brain.*
- **Server-to-server coordination** (mTLS gRPC `CoordinationService`: ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, Heartbeat, PropagateState). *Reasoning: NATS-fallback recovery.*
- **Health monitor with consecutive-failure threshold** (3 failures → unhealthy). *Reasoning: avoids transient false positives.*
- **Cluster backup/restore** (binary + JSON snapshot; cluster metadata, shard assignments, config). Source: `cmd/kscore-cluster-backup/`. *Reasoning: DR baseline.*
- **`ClusterService` gRPC + REST** (status, members CRUD, leader transfer, rebalance, backup/restore, watch streams). *Reasoning: full ops surface.*
- **`kscore-cluster` CLI** (`status`, `members`, `leader`, `add`, `remove`, `transfer-leader`, `rebalance`, `backup`, `restore`). Source: `cmd/kscore-cluster/` (shared `internal/cli/cluster`). *Reasoning: ops UX.*
- **HA resilience tests in CI** (NATS failure, etcd failure, network partition, split-brain). Source: `test/e2e/ha/` (`//go:build integration`). *Reasoning: prove the promise.*
- **Performance targets**: cluster forms <10s; first leader <3s; failover detection <5s; agent reassign <10s; minority blocks writes <1s. Source: `test/e2e/ha/slo_test.go` (`//go:build slo`, `make slo`, every-PR CI gate). *Reasoning: documented SLOs.*

### v1.x (deferred)

- **Backup automation/scheduling** **[v1.x]**. *Reasoning: manual backup works; scheduling adds infra.*
- **Comprehensive HA dashboard** **[v1.x]**. *Reasoning: basic status sufficient; rich dashboard is observability work.*
- **Read-only replicas** **[v1.x]**. *Reasoning: niche.*
- **Auto-scaling** (auto-add/remove members based on metrics) **[v1.x]**. *Reasoning: complex.*

### v2.x+ (deferred)

- **Multi-region clustering / federation** (cross-DC, gateway routing) **[v2.x+]**. *Reasoning: ships with NATS supercluster.*
- **Dynamic shard splitting under load** **[v2.x+]**. *Reasoning: speculative.*
- **Advanced topology (gateway / proxy members)** **[v2.x+]**. *Reasoning: same.*

---

## 16. Observability

### v1.0 (in scope)

- **Structured logging** (`log/slog`; JSON / logfmt / text; stdout default; correlation IDs; log levels). Source: `internal/logging/`. *Reasoning: zero-config baseline.*
- **Prometheus metrics registry** (custom; counters, gauges, histograms, summaries; cardinality limiter). Source: `internal/metrics/`. *Reasoning: standard ops integration.*
- **`/metrics` HTTP endpoint** (Prometheus exposition). *Reasoning: scrape target.*
- **OpenTelemetry tracing** (OTLP, Zipkin, stdout exporters; configurable sampling: probabilistic/parent-based/rate-limiting/adaptive). Source: `internal/tracing/`. *Reasoning: distributed-debug baseline.*
- **Health endpoints** (`/health/live`, `/health/ready` with NATS+DB checks, `/health/status` with component latencies). Source: `internal/health/`. *Reasoning: K8s probes + ops dashboards.*
- **Pre-built Grafana dashboards** (Control Plane Health, Agent Fleet, State Mgmt, Policy Compliance, NATS, Audit, Module System, Event System, Secrets, Remote Execution). Source: `deploy/grafana/dashboards/`. *Reasoning: turnkey monitoring.*
- **pprof profiling endpoints** (CPU, memory, goroutine, mutex; opt-in via config). Source: `internal/profiling/`. *Reasoning: standard Go ops tool.*
- **Correlation ID propagation** (gRPC metadata, NATS headers, log entries, span attributes). *Reasoning: traceable cross-service flows.*

### v1.x (deferred — TUI + NATS telemetry transport)

- **`kscore-monitor` TUI** (Bubble Tea; 8 base views: dashboard, agents, events, state, policy, jobs, logs, metrics). **[v1.x]**. *Reasoning: powerful ops UX but not blocking trial; needs gRPC-multiplex client + NATS subscriber. v1.0 ships READable web dashboards (Grafana) + CLI.*
- **TUI extras** (cluster, secrets/leases, schedules, runbooks, webhooks views — 13 total) **[v1.x]**.
- **Drill-downs, vim navigation, alert bar, connection health indicators, themes, search filters** **[v1.x]**.
- **NATS telemetry transport** (logs/metrics/traces over NATS subjects so isolated agents don't need outbound HTTP). **[v1.x]**. *Reasoning: paired with telemetry gateway.*
- **CLI audit logging to syslog/journald** **[v1.x]**. *Reasoning: ships with TUI work; audit infra in v0.1 already.*

### v1.x (deferred — telemetry gateway)

- **`kscore-telemetry-gateway` standalone service** (subscribes to NATS telemetry subjects; aggregates; exposes Prom scrape, Loki push, OTLP traces). Source: `internal/gateway/`, `cmd/kscore-telemetry-gateway/`. **[v1.x]**. *Reasoning: enables agents-behind-NAT topology; not needed for v1.0 connected trial.*
- **HA gateway** (queue groups + leader election) **[v1.x]**.
- **Helm chart for gateway** **[v1.x]**.

### v2.x+ (deferred)

- **Adaptive sampling tied to error metrics** **[v2.x+]**. *Reasoning: refinement.*
- **pprof visualization UI** **[v2.x+]**. *Reasoning: niche.*
- **SIEM export (CEF/LEEF)** **[v2.x+]**. *Reasoning: enterprise integration.*
- **Real-time alerting from TUI** **[v2.x+]**. *Reasoning: niche.*

---

## 17. Blueprints & Runbooks

### v1.0 (in scope — basic but real)

- **Blueprint manifest format** (`blueprint.yaml`: metadata, requires/requires_before, parameters JSON-Schema, entrypoints, hooks, features, outputs). Source: `internal/blueprint/`. *Reasoning: Salt-formula-shaped UX.*
- **Blueprint apply** (parse → param validate → resolve dependencies → render templates → execute states). *Reasoning: core functionality.*
- **Blueprint feature flags + multi-instance namespacing (`as:`)**. *Reasoning: composability for sysadmin reuse.*
- **Standard blueprint catalog (~6 v1.0)**: `demo` (single-node demo), `production-cluster` (3-node HA), `monitoring-stack` (Prom+Grafana+Loki), `security-baseline` (CIS-aligned hardening), `postgres-ha` (Postgres + WAL replication), `nats-cluster`. *Reasoning: 6 turnkey blueprints prove the model and give sysadmins immediate value.*
- **`kscore-blueprint` CLI** (`init`, `validate`, `lint`, `info`, `install`, `apply`, `update`, `remove`, `applied`, `rollback`, `bundle`). *Reasoning: full lifecycle.*
- **Blueprint storage**: filesystem (v1.0). *Reasoning: simple.*
- **Runbook YAML model** (metadata + spec with inputs, steps, onSuccess/onFailure, timeout, maxRetries). Source: `internal/runbook/`. *Reasoning: workflow automation table-stake.*
- **Runbook step types (v1.0 subset)**: `command`, `api`, `state`, `notification`, `wait`, `noop`, `fail`, `script`, `query`. *Reasoning: covers ~80% of common ops workflows; conditionals + approvals deferred.*
- **Runbook step dependencies** (`depends_on`). *Reasoning: order matters.*
- **Runbook variable templating** (`{{ steps.X.outputs.Y }}`). *Reasoning: chaining steps.*
- **Runbook execution + status** (`kscore-runbook list|execute|status|list-executions|audit|test`). *Reasoning: ops UX.*
- **Saga coordinator (minimal)** (`pkg/saga`: forward execution + compensating transactions on failure; in-memory or SQLite log; no checkpoint resume). *Reasoning: rollback safety for multi-step ops.*
- **StateMachine library** (`pkg/statemachine`: generic FSM with guards, callbacks, history, optional checkpoint). *Reasoning: used by runbook/promotion/rollback engines.*
- **Runbook + blueprint storage** (SQLite). *Reasoning: persistence + history.*

### v1.x (deferred — scheduling)

- **`kscore-schedule` CLI + ScheduleService** (cron + interval; time windows; agent/role/tag targeting; retry policy). **[v1.x]**. *Reasoning: high-value but separate concern; ships with Maintenance Service.*
- **Maintenance windows + change-window awareness** **[v1.x]**.
- **Schedule + maintenance gRPC + REST APIs** **[v1.x]**.

### v1.x (deferred — runbook power features)

- **Runbook conditional steps** (`if`, `switch`, `loop`, `parallel`, `sub-runbook`). **[v1.x]**. *Reasoning: real workflow power; deferred so the v1.0 step engine ships solid.*
- **Per-step approvals + delegations** (with timeout, Slack/email/PagerDuty notifications). **[v1.x]**. *Reasoning: ITSM integration weight.*
- **Manual interventions** (`prompt`, `wait-manual`, `confirm` step types). **[v1.x]**. *Reasoning: paired with approvals.*
- **Runbook dry-run mode** **[v1.x]**. *Reasoning: ships with conditionals.*
- **Rollback step type with auto-compensation** **[v1.x]**. *Reasoning: deepens saga integration.*

### v1.x (deferred — full catalog + saga advanced)

- **Standard catalog expansion** (full 14: + `enterprise-platform`, `kubernetes-operator`, `identity-federation`, `gitops-integration`, `proxy-agents`, `file-distribution`, `edge-deployment`, `metrics-only`). **[v1.x]**. *Reasoning: rich catalog drives adoption.*
- **Saga checkpoint resume** (`pkg/saga/log_sqlite` advanced; resume from interruption). **[v1.x]**. *Reasoning: production resilience.*
- **Blueprint signature + signed bundles** (`kscore-blueprint sign|verify|publish`). **[v1.x]**. *Reasoning: supply chain hardening.*
- **Blueprint mirror for air-gap** **[v1.x]**.

---

## 18. Plugin / Module System

### v1.0 (in scope — Starlark + Cosign + filesystem registry)

> **Decision**: Per user direction, plugin/module system ships in v1.0. To make that achievable, v1.0 is **Starlark-only** with **filesystem-backed registry** and **Cosign-only verification** (no SumDB transparency log, no WASM, no cloud-backed registry yet). This delivers Salt-like extensibility on day 1 without doubling v0.1 scope.

- **Starlark runtime** (Python-like sandboxed scripting via `go.starlark.net`; deterministic mode with random/time disabled by default). Source: `pkg/module/runtime/starlark/`. *Reasoning: pure Go, simpler than WASM, sufficient for trial extensibility.*
- **Module manifest format** (`module.yaml`: name namespaced `vendor/pkg`, version semver, type, entrypoint, capabilities, limits, dependencies). Source: `pkg/module/manifest/`. *Reasoning: schema is shared with v1.x WASM modules.*
- **Capability-based security (9 core capabilities)**: `fs.read`, `fs.write`, `http.get`, `http.post`, `exec`, `secrets.read`, `secrets.write`, `kv`, `log`. Source: `pkg/module/capability/`. *Reasoning: covers ~90% of use cases. Per-syscall granularity is v1.x.*
- **Capability scoping** (path globs, domain allowlists, command allowlists, secret-path scoping, rate limits, timeouts). Source: `pkg/module/capability/`. *Reasoning: not just on/off — actually safe defaults.*
- **Cosign signature verification** (RSA, ECDSA, Ed25519; KeyID-based key management). Source: `pkg/module/verify/`. *Reasoning: cryptographic supply chain baseline.*
- **SHA-256 content addressing** (CAS storage in `~/.kscore/modules/<hash>/`). *Reasoning: dedup + integrity.*
- **Module resolver** (semver constraints; DAG with cycle detection; minimum-version-selection conflict resolution). Source: `pkg/module/resolver/`. *Reasoning: reproducible builds.*
- **Module lockfile** (`module.lock`: pinned versions + hashes; reproducible across team). Source: `pkg/module/manifest/`. *Reasoning: avoid "works on my machine".*
- **Filesystem-backed registry** (Go-mod-style HTTP endpoints: `/@v/list`, `/@v/{ver}.info`, `/@v/{ver}.mod`, `/@v/{ver}.zip`). Source: `pkg/module/registry/`, `cmd/kscore-registry/`. *Reasoning: simple, proven pattern (matches Go modules); cloud backends slip to v1.x.*
- **Module loader pipeline** (parse manifest → verify → policy check → capability validation → runtime init → register granted capabilities). Source: `pkg/module/loader/`. *Reasoning: clear lifecycle.*
- **Module audit logging** (every capability invocation: module, version, capability, op, success/failure, duration). Source: `pkg/module/audit/`. *Reasoning: compliance.*
- **Plugin discovery** (`kscore-*` binaries in `$PATH`; git-style dispatch via `kscorectl`). Source: `pkg/plugin/`. *Reasoning: extension model from day 1.*
- **Starlark SDK** (host capability bindings; Go shims). Source: `modules/sdk/starlark/`. *Reasoning: enables module authors.*
- **`kscore-module` CLI** (`init`, `build`, `validate`, `resolve`, `verify`, `sign`, `test`, `publish`, `install`, `update`, `clean`, `tree`). *Reasoning: full author UX.*
- **Module test framework** (Starlark unit-test harness). *Reasoning: authors need test/run loop.*
- **Module policy hooks** (audit-mode in v1.0; OPA/CEL evaluation gated on `module load`). *Reasoning: enforces capability policy at load time even though policy enforcement is v1.x.*

### v1.x (deferred — WASM + cloud backends)

- **WASM runtime** (wazero; WASI; instruction metering; memory bounds). **[v1.x]**. *Reasoning: enables Rust/Go/C++ modules; pure Go runtime so no CGO regression.*
- **Rust SDK** (Cargo crate with WASI bindings). **[v1.x]**. *Reasoning: top-tier perf-language for module authors.*
- **Go (TinyGo) SDK** **[v1.x]**. *Reasoning: same.*
- **OCI registry backend** (Harbor, ECR, GCR, ACR; modules as OCI artifacts). **[v1.x]**. *Reasoning: enterprise registry compatibility.*
- **S3/GCS/Azure storage backends** for filesystem registry **[v1.x]**. *Reasoning: cloud-native scaling.*
- **`kscore-module mirror`** (export/import for air-gap) **[v1.x]**.
- **`kscore-module update`** with auto-upgrade-compatible-versions **[v1.x]**.

### v1.x (deferred — fine-grained security + supply chain)

- **SumDB transparency log** (Merkle proofs, append-only log, tamper detection). **[v1.x]**. *Reasoning: regulatory/audit value above and beyond signature; not blocking.*
- **Fine-grained capability model** (per-syscall grants via seccomp/eBPF on Linux). **[v1.x]**. *Reasoning: defense-in-depth refinement.*
- **Module vulnerability scanning + SBOM generation** **[v1.x]**. *Reasoning: supply-chain hardening.*

### v2.x+ (deferred)

- **C++ SDK** (Emscripten/WASI SDK). **[v2.x+]**. *Reasoning: niche audience; Rust/Go/Starlark cover most.*
- **Federated module registries** (cross-trust-domain) **[v2.x+]**.

---

## 19. Multi-Environment

### v1.0 (in scope — Linux + universal foundations)

- **Platform detection** (Linux distro via `/etc/os-release`; arch; package manager: apt/dnf/yum/pacman/zypper/apk; init system: systemd/sysv/openrc; virtualization: KVM/VMware/VBox/Xen/Hyper-V). Source: `internal/platform/`. *Reasoning: drives stdlib module dispatch.*
- **Hardware introspection** (CPU, memory, disks, NICs, system/BIOS info). Source: `internal/hardware/`. *Reasoning: targeting + drift.*
- **Network interface detection** (IPv4/IPv6, MTU, speed). Source: `internal/netutil/`. *Reasoning: v1 must be IPv6-clean.*
- **IPv6 dual-stack on all listeners** (control plane gRPC/HTTP, NATS, etcd, Postgres). *Reasoning: per Epic 18; non-negotiable for modern infra.*
- **Address family preference** (`prefer_ipv4`, `prefer_ipv6`, `ipv4_only`, `ipv6_only`). *Reasoning: deterministic selection.*
- **IPv6 bracketing helpers** (`[::]:8080`, `[::1]:5397`, IPv6-aware DSN building for Postgres). *Reasoning: real-world bug source.*
- **Cloud metadata stub** (AWS IMDSv2 token, GCP metadata header, Azure MSI — minimal probe; full metadata extraction v1.x). *Reasoning: detect "running in cloud" vs not is useful day 1.*

### v1.x (deferred)

- **Windows agent** (native service via SCM, Event Log, PowerShell exec, registry, Chocolatey/winget). **[v1.x]**. *Reasoning: paired with Windows stdlib in state mgmt; meaningful subset of trial users.*
- **macOS agent** **[v1.x]**. *Reasoning: low population; defer behind Windows.*
- **Container runtime detection** (Docker/containerd/Podman/CRI-O via socket + cgroup parsing). **[v1.x]**. *Reasoning: paired with v1.x container stdlib modules.*

### v1.x (deferred)

- **Kubernetes operator** (RemoteExecution + StateConfig CRDs; informer-based reconciliation; drift detection; pod exec). Source: `internal/k8s/`, Epic 48. **[v1.x]**. *Reasoning: complex; depends on mature core; CRDs + reconciliation framework is its own epic.*
- **`k8s_*` stdlib modules** (12 modules — see State Management §8). **[v1.x]**.

### v1.x (deferred)

- **Full cloud metadata extraction** (AWS region, AZ, instance ID, type, VPC, subnet, IAM; GCP project, zone, instance, K8s SA; Azure RG, VM, MI). **[v1.x]**. *Reasoning: deeper integration; enables labels-from-metadata targeting.*
- **Container metadata extraction** (image, labels, env, volumes, network). **[v1.x]**. *Reasoning: pairs with cloud metadata.*

### v2.x+ (deferred)

- **Service mesh integration** (Istio, Linkerd, Consul; SPIFFE extraction; proxy config; mTLS). **[v2.x+]**. *Reasoning: post-trial enterprise feature.*
- **Edge agent mode** (local NATS leaf; offline buffer; resource-constrained ARM; low-power telemetry). **[v2.x+]**. *Reasoning: niche; ships with NATS leaf v2.x+.*
- **DNS provider management** (libdns: Route53/CloudFlare/Hetzner/Azure DNS/etc.; declarative records). **[v2.x+]**. *Reasoning: see Specialized §20.*
- **Advanced networking** (WiFi, 802.1X, link speed control, promiscuous mode, BMC/IPMI). **[v2.x+]**. *Reasoning: niche.*

---

## 20. Specialized & Extension Domains

### v1.0 (in scope — minimum)

- **File distribution (basic)**: NATS-based file server with chunked transfer (1 MB chunks, SHA-256 per chunk, resume); local filesystem + S3-compatible backends; proxy caching with LRU+TTL. Source: `internal/files/`, `cmd/kscore-files/`. *Reasoning: agents need to fetch packages, configs, blueprints — this is operational table-stakes. Mirror groups + advanced sync defer to v1.x.*
- **Self-management (basic)**: bootstrap from seed YAML (`kscore-bootstrap --seed`); full system backup (Postgres dump + JetStream + etcd + config + secrets, age-encrypted); restore from backup; `kscore-backup` CLI. Source: `internal/selfmgmt/`, `internal/backup/`, `cmd/kscore-backup/`. *Reasoning: ops-day-1 capability — disaster recovery has to work, period. Automated scheduling + rolling upgrades are v1.x.*
- **Basic rate limiting** (token bucket per IP / API key / header; per-route configurable). Source: `internal/ratelimit/`. *Reasoning: protects against agent storms; trivial cost.*

### v1.x (deferred)

- **File distribution: NATS Object Store + Git backend + mirror groups (geographic redundancy with read strategies + write policies)** **[v1.x]**. *Reasoning: operational scaling.*
- **Self-management: automated scheduled backups + rolling upgrades + drift detection on self-config** **[v1.x]**. *Reasoning: ops automation.*
- **Quota system** (per-namespace resource quotas: agents, secrets, configs, etc.) **[v1.x]**. *Reasoning: multi-tenant safety.*
- **`kscore-loadtest` benchmarking** (registration / heartbeat / exec / state-apply scenarios; metric reports). **[v1.x]**. *Reasoning: capacity planning + regression testing.*

### v2.x+ (deferred)

- **Proxy agents** (Epic 21, 42 — manage unmanaged devices over SSH, SNMP v2c/v3, REST/HTTP, WinRM, NETCONF, RESTCONF, gNMI, Telnet). Source: `internal/proxy/`, `internal/protocols/`, `internal/vendors/`. **[v2.x+]**. *Reasoning: large surface (8 protocols × 20 vendors); SSH+SNMP+REST core first, more protocols/vendors in later waves; not blocking commercial trial of "manage Linux/K8s fleet".*
  - **v2.x+**: SSH, SNMP, REST/HTTP adapters; vendor adapters for Cisco IOS, Juniper JUNOS, Arista EOS, pfSense, OPNsense, VyOS.
  - **v2.x+**: WinRM, NETCONF, RESTCONF, gNMI, Telnet; remaining 14 vendor drivers.
- **Air-gapped deployments** (offline registry, bootstrap packages, upgrade archives, transfer tooling, UDP data diode). Source: `internal/airgap/`, `cmd/kscore-bootstrap`, `cmd/kscore-transfer/`. **[v1.x baseline; v3.x+ data diode]**. *Reasoning: regulated/government use case; depends on registry + upgrade maturity.*
- **Federation** (SPIFFE trust-domain federation, multi-cluster trust bundles). **[v2.x+]**. *Reasoning: multi-cluster.*
- **MCP server** (`kscore-mcp` for Claude Desktop / Claude Code / Cursor; 16 tools across capability profiles; audit attribution). Source: `internal/mcp/`, `cmd/kscore-mcp/`. **[v2.x+]**. *Reasoning: AI-assisted ops is great differentiator but not commercial-trial-blocking; depends on stable APIs.*
- **Saga checkpoint resume** **[v1.x]** — already noted in Blueprints/Runbooks domain.

### v3.x+ / future (deferred)

- **Web UI / Management Console** (browser-based). **[v3.x+]**. *Reasoning: TUI + Grafana cover ops UX in v1; web UI is post-product-market-fit work.*
- **Blueprint marketplace** (community sharing). **[v3.x+]**. *Reasoning: depends on adoption.*
- **Multi-cloud test matrix** (real-AWS / real-GCP / real-Azure CI). **[v3.x+]**. *Reasoning: cost-heavy; mocks suffice for v1.*
- **Cross-platform expanded test matrix** (BSDs, Solaris, etc.). **[v3.x+]**. *Reasoning: niche.*
- **UDP data diode** (one-way transfer with FEC). **[v3.x+]**. *Reasoning: military / classified-network use case.*

---
