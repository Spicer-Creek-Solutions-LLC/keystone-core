# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Post-reconstruction launch-prep work. Frozen into v0.1.0 at the tag cut.
Anchored on the reconstruction baseline (commit `14be1109`).

### Security

- Webhook handler error leakage, gRPC bypass on misrouted methods, and
  HMAC hex-decode timing — three high-severity audit findings closed
  (Phase B5 H1–H3, commit `7b46b28e`).
- Join-token base62-prefix bias documented + exec capability allowlist
  semantics tightened (Phase B5 M1+M2, commit `1f0164e0`).
- Empty `security.hmacsecret` now emits a loud production-mode warning
  (Phase B5 C1, commit `03d511e0`) — operators must set this explicitly
  in production; dev-mode bootstrap UX remains unchanged.
- Upgraded `golang.org/x/net` to v0.55.0 to close GO-2026-5026 (Phase B1,
  commit `43c5590a`).

### Added

- **SPDX license headers on every hand-written source file**
  (`// SPDX-License-Identifier: Apache-2.0`): 1,332 `.go` files + 6
  `.sh` files. Generated `.pb.go` excluded (already `linters: all` in
  `.golangci.yml`). Enforced going forward by enabling the `goheader`
  linter — new files without the header fail lint. Ecosystem-standard
  posture (Kubernetes / etcd / NATS / CoreDNS / Prometheus all do this).

### Changed

- **Default gRPC server port moved from `9090` → `5397`** to avoid the
  Cockpit collision on Rocky 10 / RHEL 10 (commit `3d482fa1`). Cockpit's
  default `9090` made `apt install kscore-server` fail to start out-of-box
  on Rocky 10; 5397 has no known popular collision. Operators with old
  `kscorectl` defaults pointing at `9090` get `connection refused` and
  pass `--server localhost:5397` to recover.
- Debian/RPM packaging: postinst hooks create the `kscore` system user,
  `/etc/kscore`, `/var/lib/kscore`, `/var/log/kscore`, `/run/kscore`;
  auto-generate the HMAC secret; ship a default config so
  `systemctl start kscore-server` works out-of-box (commit `4be8d19d`).
- Binary install path moved from `/usr/local/bin/` to FHS-canonical
  `/usr/bin/` for distro packages (commit `4be8d19d`).

### Fixed

- State integration tests: TRUNCATE + boundary + JSONB regressions
  uncovered during clean-tree Phase C run (commit `dd7f03b4`).
- Three timing-sensitive test flakes exposed by the Forgejo runner —
  queue-group `≥1` assumption, observer-vs-state race, NATS
  subscription-flush race (commit `928874b5`).
- CI: pinned `protoc-gen-go@v1.36.11` + `protoc-gen-go-grpc@v1.6.1` so
  generated stubs don't drift from `@latest` (commit `cbc78351`).
- CI release-smoke: native-execution fallback when Docker is absent on
  the Forgejo runner image (commit `032219cc`).
- CI: musl `lychee` variant for the Forgejo runner's older glibc
  (commit `af46bb52`); `install-tools` now pulls a pinned lychee binary
  (commit `af4cb9e7`).

### Docs

- **First-impression doc pass** for the v0.1.x soft launch:
  - `README.md` Quickstart rewritten around the `apt install` /
    `systemctl` / `kscorectl` operator path (was `git clone`,
    `make e2e-up`, and grpcurl). Mirrors
    [`docs/runbooks/bootstrap-new-cluster.md`](docs/runbooks/bootstrap-new-cluster.md).
  - `docs/project/GETTING-STARTED.md` rewritten as a guided ~30-minute
    fresh-VM operator tutorial: package install, smoke checks, agent
    online, run a command via `kscorectl exec`, apply state via
    `kscorectl state apply`, browse audit via `kscorectl audit log`.
    Closes the matching v0.x ROADMAP entry.
  - Docker-compose dev topology + grpcurl walkthrough relocated to
    `docs/project/DEVELOPMENT.md` § Local Dev Topology, where it
    belongs as contributor onboarding.
- **F1 soft-launch decision** for v0.1.x recorded durably in
  [`docs/project/GOVERNANCE.md`](docs/project/GOVERNANCE.md) § Launch
  Posture; F1 ticked in the public-launch checklist; F2
  (announcement draft) marked not-applicable for v0.1.x.
- **F3 triage SLO commitment** documented in
  [`docs/project/MAINTAINERS.md`](docs/project/MAINTAINERS.md) §
  Triage and Response. v0.1.x posture: best-effort, no formal SLO;
  rough cadences stated for issues / PRs / security reports;
  cadences reassessed at v0.5 (formal SLO possible from v0.5+).
- **F4 release-incident response plan** lands at new
  [`docs/project/RELEASE-INCIDENT.md`](docs/project/RELEASE-INCIDENT.md).
  Covers the post-publication decision tree (yank vs patch vs
  communicate-only), yank procedure, fast follow-up release
  numbering + communication, and post-incident process (CHANGELOG,
  post-mortem, process change). Kept distinct from
  INCIDENT-RESPONSE.md (production security incidents) and
  RELEASE-PLAYBOOK § 14 (expedited release ceremony) which it
  cross-references. v0.1.x-specific: operator-distributed-package
  reality shapes the "yank" mechanics (no public APT/DNF repo to
  withdraw from yet).
- **Hugo docs site pulled forward from v1.x to gate-v0.5**: updates
  AGENTS.md §5, FEATURES.md §1, VERSIONING.md (resolves the prior
  v1.0-gate-7-vs-v1.x-FEATURES contradiction — Hugo is now a v0.5
  gate; v1.0 gates renumbered 8/9/10 → 7/8/9), ROADMAP.md (Hugo
  entry moved from v1.x to gate-v0.5; dependent v1.x entries
  "Expanded getting-started guides" + "Error-message docs URLs"
  re-framed against Hugo's new position). Rationale: a polished,
  searchable doc experience benefits the v0.5 external-tester
  audience; pre-v0.5 Markdown + subtree READMEs remain sufficient
  for the v0.1.x invited-installer audience. PDF export stays
  v1.x (FEATURES.md §1).
- Public-launch checklist Phases A–D ticked across 4 commits
  (`524757a0` → `9622ed36`): code-vs-docs sync, link health, epic
  acceptance audit, security baseline + dummy-report-flow doc, threat-
  model refresh, clean-tree CI gates green, six-VM cross-distro
  environment validation (debian12 / ubuntu22 / ubuntu24 / rocky8 /
  rocky9 / rocky10).
- NOTICE accuracy audit: dropped `wazero`, added 8 notable deps
  (HashiCorp Vault, gRPC, OpenTelemetry, Prometheus client,
  SPIFFE go-spiffe, minio-go, modernc.org/sqlite, go-git), reorganized
  by domain, documented the `modernc.org/mathutil` "Unknown" license
  exception inline (commit `33f19178`).

## [v1.0.0] — Planned

Pending all 19 epics complete + the v1.0 gate checklist in
[`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). The full v1.0.0
entry will land with the v1.0 cut; the in-progress feature inventory tracks
under [`FEATURES.md`](FEATURES.md). Until then, v0.x is the active release
line per the v0.1 → v0.5 → v1.0 ladder.

## [v0.1.0] — Unreleased

First public release of the post-reset codebase. **Linux-only,
`v0.x`-quality.** The reconstruction baseline established on 2026-05-05
closed all 19 epics; v0.1.0 is the first release on the v0.x line —
the "genuinely try-able" cut shipped to curious operators and early
adopters per [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md).
**Expect breaking changes between minor versions** (minimised, always with
a migration note). The formal external-tester milestone is the v0.5
checklist; the SemVer stability commitment begins at v1.0.

Implementation tracked in [`epics/`](epics/); ranked backlog in
[`docs/project/ROADMAP.md`](docs/project/ROADMAP.md); the prior
implementation is preserved on the `archive/v0` branch / `archive/v0-final`
tag.

### Notable behavior — the policy engine is AUDIT-MODE-ONLY

> **Read this before relying on policy.** The policy engine evaluates,
> audits, and reports — but **it does not block anything**. Even when a
> policy returns `Allowed=false`, the operation **still proceeds**.
> `policy.enforcement_enabled` is hardcoded `false` and is not
> operator-settable on the `v0.x → v1.0` line.

This is intentional: full enforcement carries breaking-change risk (a
misconfigured policy could block the fleet), so v1.0 ships policy as
*observability* — run real policies against real workloads, inspect what
*would* have been blocked via the audit trail / `WouldDeny`, and build
confidence first. **A future post-v1.0 release flips enforcement on
and is a behavior-changing release** — policies left at
`EnforcementMode=enforce` will start blocking at that point. See
[`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md)
for the full audit-mode contract and the v1.0 → enabling-enforcement
migration steps.

The audit log itself is fully live: every sensitive op (auth, secret
access, command exec, state apply, policy eval) writes an `AuditEntry`.

### Added

Grouped by epic. Each entry names the deliverable; the linked epic file
carries the per-task acceptance details. Some entries include a v0.1.0
caveat (this release line allows breaking changes); the v0.5 + v1.0 gates
in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) are the
graduation targets.

- **Foundations** ([epic 01](epics/01-foundations.md)) — `pkg/{version,semver,wait,dbutil}`, `internal/{config,logging,cli}`, `pkg/api/{apierror,v1}` (proto codegen), `Makefile`-driven workflow (`make build`, `test`, `lint`, `proto`, `release-snapshot`), cross-compile matrix (linux/{amd64,arm64} + darwin/{amd64,arm64} + windows/{amd64,arm64}; pure Go, no CGO), CI on Forgejo Actions + Codeberg Woodpecker.

- **Storage layer** ([epic 02](epics/02-storage-layer.md)) — `internal/state` with SQLite + PostgreSQL backends. `Store` interface composes per-domain sub-interfaces (Agent, Command, BatchJob, APIKey, Health, State, Saga, Secrets, Events, Audit, Cluster, etc.). Migrator runs on `Open` and supports forward-only migrations + rollback transactions. SQLite store ships with sane PRAGMAs (`journal_mode=WAL`, `foreign_keys=ON`).

- **API surface** ([epic 03](epics/03-api-surface.md)) — gRPC + REST proto schemas covering 13 services. Auth chain (`pkg/api/auth`: APIKey / JWT / mTLS authenticators, RBAC authorizer, sliding-window rate limiter). Per-domain REST handlers, gateway via grpc-gateway. OpenAPI 3.0 spec lints in CI.

- **Control plane core** ([epic 04](epics/04-control-plane-core.md)) — `kscore-server` is a real daemon. `internal/controlplane`: `ConnectionManager`, `CommandDispatcher`, `BatchDispatcher`. `pkg/api/server` runs a 21-step deterministic init, dual-stack listeners, auth middleware chain, `/health/{live,ready,status}` + `/api/status` endpoints. Dev mode auto-generates an admin API key once at boot (printed once; not recoverable).

- **NATS messaging** ([epic 05](epics/05-nats-messaging.md)) — `internal/nats.Manager` (external client + embedded server modes), `SubjectBuilder` with `kscore.{cluster}.…` prefix enforced on both sides, `Envelope` wire format with length-prefixed dedup, per-endpoint circuit breakers, JetStream stream provisioning, server-side bootstrap registration handler with PSK validator + API-key issuer.

- **Agent runtime + bootstrap UX** ([epic 06](epics/06-agent-runtime.md)) — `kscore-agent` is real. Subscribes to its command subject; runs `Executor` (os/exec wrap with SIGTERM-grace-then-SIGKILL, hard-cap output truncation, optional uid switch), `MetadataCollector` (gopsutil-backed; distro / kernel / NIC / virt / CPU / memory / disk), `SecurityEnforcer` (HMAC-SHA-256 + principal/command allowlists + env filter). Drains in-flight commands on SIGTERM. systemd unit + non-interactive bootstrap flags.

- **Remote execution & targeting** ([epic 07](epics/07-remote-execution.md)) — operator-facing dispatch end-to-end. `internal/targeting`: shorthand expression compiler (`expr-lang/expr` + `gobwas/glob`) → `Matcher.Match(AgentRecord)` against flattened metadata; AND-of-labels-plus-hostname-glob today. `internal/execution`: `Executor` interface + `ManagedExecution` (PENDING / RUNNING / COMPLETED / FAILED / TIMEOUT / CANCELLED / RETRYING with `Callbacks` + `RetryPolicy`), `Pipeline` (sequential stages with stdout-piping), `Shell` selectors (bash / sh / powershell / cmd), `CommandPolicy` (`Validate` / `ValidateNoShell` modes). `internal/controlplane.BatchDispatcher.ExecuteBatch` (semaphore concurrency, 500 ms progress ticker, async orchestration detached from request ctx). `kscorectl exec` with `run` / `async` / `script` / `status` / `list` / `cancel` / `output` subcommands + `--dry-run`; table / json / yaml formatters; `--raw` pipe-friendly single-agent output.

- **State management engine + 35 stdlib modules** ([epic 08](epics/08-state-management.md)) — `internal/statemgmt` ships the engine (parse → render → validate → resolve → check / apply / test) plus **35 stdlib modules** across nine categories: system & core / scheduled tasks / storage / network base / firewall / SSH & security / system config / files & VCS / certificates. `pkg/saga` provides aggregate-and-continue compensation that `Runner.RunSaga` wires into the state runner against the `StateHistoryStore`. `StateGRPCServer` implements `ApplyState` (streaming), `CheckState`, `DetectDrift`, `GetStateHistory`, `RollbackState`, `GetStateStatus`. Integration test covers five paths (apply / idempotency / drift / rollback / saga compensation / requisite-cycle error).

- **Identity & auth** ([epic 09](epics/09-identity-auth.md)) — embedded SPIFFE-shaped CA, mTLS-ready join tokens, `kscore-identity` operator CLI, mTLS-on-gRPC default-on. `internal/identity` ships the full surface — `SPIFFEID` / `X509SVID` / `JWTSVID` / `TrustBundle`, two-tier root + signing CA with auto-rotation, `JoinTokenStore` (in-memory + state-backed) with constant-time hash + atomic MaxUses, `JoinTokenAttestor`, `EmbeddedProvider` composing everything behind the `Provider` interface. `IdentityService` gRPC exposes `token {create,list,revoke,cleanup}` + `ca {info,rotate-signing,export}` + `status`. Server-side mTLS derives from the running provider with auto-refresh on signing-CA rotation; `defaultConfig().Server.TLS.Enabled = true`. NATS bootstrap upgrades the epic-05 PSK path to a SPIFFE-aware `JoinTokenBootstrapValidator` + `SVIDBootstrapIssuer`.

- **Secrets management** ([epic 10](epics/10-secrets.md)) — `internal/secrets`: encrypted-file backend (XChaCha20-Poly1305 envelope + Argon2id KDF) and HashiCorp Vault backend (AppRole / Kubernetes / LDAP auth modes). Per-secret KV2 versioning; capability-based access via `pkg/api/secrets` REST + gRPC. `kscore-secrets` CLI: `get` / `put` / `list` / `delete` / `versions` / `rotate`. `SecretsBackend` interface lets new backends slot in without server changes. Audit-mode-only policy can evaluate "what would be denied" on every read.

- **Event system** ([epic 11](epics/11-events.md)) — `internal/events.JetStreamPublisher` (in-process embedded + external JetStream backed) with envelope + length-prefixed dedup + at-least-once delivery. Per-event-type retention policies. `internal/events.Subscriber` ships the pull-consumer side with at-most-once ACK + dead-letter on persistent failure. `EventService` REST + gRPC with `Publish` / `Subscribe` (streaming) / `Replay` / `Tail`.

- **Audit log + policy engine** ([epic 12](epics/12-audit-policy.md)) — `internal/audit.Store` (Postgres + SQLite) records every sensitive op as an immutable `AuditEntry` chained to the prior via SHA-256. `PolicyService` evaluates OPA-style Rego or CEL policies in **audit-mode only** for v1.0; `WouldDeny` enriches the audit trail without blocking. `kscore-audit` + `kscore-policy` CLIs cover query + bundle / load / test workflows. See [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md) for the v1.0 → v1.8 enforcement migration path.

- **Clustering & HA** ([epic 13](epics/13-clustering-ha.md)) — the v1.0 differentiator. `internal/cluster` ships embedded etcd v3 (`Manager.Mode = embedded`) + external etcd modes. `Membership`, `Leader`, `Shard`, `Routing`, `Health`, `Recovery` subsystems. Server-side `ClusterService` gRPC: `GetClusterStatus`, `ListMembers`, `AddMember`, `RemoveMember`, `GetLeader`, `TransferLeader`, `Rebalance`, `CreateBackup`, `RestoreBackup`, `WatchMembership` + `WatchLeadership` (streaming). `kscore-cluster` + `kscore-cluster-backup` CLIs. Wall-clock SLOs gate the build: first leader <3 s, failover <5 s/10 s, minority-block <1 s, recovery <15 s.

- **Plugin / module system** ([epic 14](epics/14-plugin-module-system.md)) — Starlark module runtime with capability-based sandboxing. Cosign-signed `.kscore-module` bundles; filesystem registry (`KSCORE_MODULE_PATH`). `kscore-module` CLI: `scaffold` / `pack` / `sign` / `verify` / `publish` (filesystem) / `inspect` / `lint` / `test`. `pkg/module/testing` runner harness for offline module tests. `internal/statemgmt/stdlib` modules graduate from in-tree to module-loadable in v1.x.

- **Blueprints + runbooks + saga + state-machine library** ([epic 15](epics/15-blueprints-runbooks.md)) — `internal/blueprint` composes states into reusable bundles (`Blueprint`, `BlueprintLibrary`, `BlueprintBundle` for sign / publish / import). `internal/runbook` runs imperative procedures with checkpoint / resume / rollback via `pkg/saga`. `internal/statemachine` shared FSM library used by GitOps rollback (epic 16) + self-management (epic 18). `kscore-blueprint` + `kscore-runbook` CLIs.

- **GitOps integration + outbound webhooks** ([epic 16](epics/16-gitops-webhooks.md)) — inbound webhook receiver (`webhook` HTTP server on `:8081`) with per-source HMAC / Bearer / `none` auth methods for Argo CD / Flux / GitHub / GitLab. `internal/gitops.Verifier` runs state + drift checks against the post-deploy fleet. `internal/gitops.Rollback` is an FSM-driven recovery loop wired to epic 15's state-machine library. Outbound webhook fan-out (`internal/webhook`) for downstream notifications. `kscore-gitops` + `kscore-webhook` CLIs.

- **Observability** ([epic 17](epics/17-observability.md)) — slog handler with masking layer (regex-based for tokens / API keys / passwords). Prometheus metrics for every domain (`kscore_*_total` / `_seconds` / `_bytes` counters + histograms). OpenTelemetry tracing wired through gRPC + REST handlers; OTLP exporter. Grafana dashboard JSON ships under `deploy/grafana/`. `/health/{live,ready,status}` for orchestrator probes.

- **Self-management + file distribution + rate limiting** ([epic 18](epics/18-self-mgmt-files-ratelimit.md)) — `internal/selfmgmt` covers backup (`kscore-backup` CLI; SQLite + Postgres + JetStream snapshots), restore, upgrade-staging, and seed-bootstrap. `internal/files` is the chunked file-distribution transport with resumable GETs, ACL-gated PUTs, content-addressed cache, `kscore-files` CLI. Token-bucket rate-limit middleware on both HTTP and gRPC with `Retry-After` (delta-seconds) responses; per-key extractors (`api_key`, `principal`, `client_ip`).

- **Test, harden, & release infrastructure** ([epic 19](epics/19-test-harden-release.md)) — the v1.0 release-quality gates: docker-compose E2E (`make e2e-test`; 11 scenarios), in-process module + secrets + blueprint + self-mgmt + webhook integration suites, in-process HA SLOs (cluster forms <10 s, leader <3 s, failover <10 s), perf SLOs (command latency <100 ms, event throughput >10k/s, batch-10 fan-out <2 s), per-package coverage gates (`make coverage-gate` — critical ≥70%, CLI ≥40%), race detector on every `go test` (`make race-policy`), `goleak` in every integration package (`make goleak-policy`), four-scan security baseline (`make security-{secrets,vulns,sast,licenses}` — gitleaks / govulncheck / gosec / go-licenses; CI-gated), hardening pass (audit tables in [`docs/project/HARDENING-BASELINE.md`](docs/project/HARDENING-BASELINE.md), pprof baseline in [`docs/project/PROFILING-BASELINE.md`](docs/project/PROFILING-BASELINE.md)), auto-generated reference docs (`make docs-sync` + `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md`), single-signer release ceremony (`RELEASE-PLAYBOOK.md` covers v0.x / v1.0 / v1.1 with the v1.2 multi-party graduation checklist).

### Known limitations for v0.1.0

The v0.5 + v1.0 gates in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md)
are the graduation targets. Items shipping in v0.1.0 with reduced
coverage or behind a v0.x flag:

- **Cross-distro CI matrix is in-progress.** State stdlib runs against Debian 12 / Ubuntu 22.04+24.04 / Rocky 9 / Alpine 3.19 locally; the v0.5 checklist expands the matrix.
- **`package` module ships apt + dnf** as the v0.1 backends; **apk + zypper** graduate at v0.5.
- **Network base: firewalld rich-rule canonicalisation deferred** to v0.5 per [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md).
- **Persistence renderers** for networking (NetworkManager, netplan, systemd-networkd) — limited coverage in v0.1; full set graduates at v0.5.
- **Policy enforcement** is locked at audit-mode for the entire v0.x → v1.0 line per [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md); enforcement flips at v1.8.
- **Hugo docs site** — planned for v0.5 (pulled forward from a former v1.x position; see [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) § v0.5 gate § Documentation). v0.1.x ships Markdown references under `docs/project/` with curated subtree-index `README.md` files for navigation.
- **Multi-party release signing** — v1.2 graduation per [`RELEASE-PLAYBOOK.md`](RELEASE-PLAYBOOK.md). v0.x / v1.0 / v1.1 ship under the single-signer ceremony.
- **Windows + macOS agents, WASM modules, full SPIRE, Kubernetes operator, federation, web UI, blueprint marketplace** — explicitly post-v1.0 (epic 19 §Scope out).
- **gRPC server reflection disabled in dev** — the workaround (pass `-import-path api/proto -proto api/proto/keystone/core/v1/controlplane.proto` to grpcurl, or use the REST surface) is documented in [`docs/project/DEVELOPMENT.md`](docs/project/DEVELOPMENT.md) § Local Dev Topology.
- **gosec G115 (integer overflow conversion) excluded project-wide** per [`docs/project/SECURITY-GOVERNANCE.md`](docs/project/SECURITY-GOVERNANCE.md) "Security Baseline Pipeline." v1.x ROADMAP entry "Security baseline expansion" tracks the per-site re-audit.
- **Per-domain sustained-load profiling, 1-hour fd-leak soak, docs-URL injection in errors, context-aware threading of 122 deep-helper log sites** — tracked as v1.x ROADMAP entries; the v1.0 baseline is in [`docs/project/PROFILING-BASELINE.md`](docs/project/PROFILING-BASELINE.md) + [`docs/project/HARDENING-BASELINE.md`](docs/project/HARDENING-BASELINE.md).

### Breaking changes

This is the v0.x line — breaking changes between minor versions are
expected. Each future release will list its breaking changes under a
`### Breaking changes` section here, with a migration note in the
release announcement and (when possible) a one-cycle deprecation
period. **For v0.1.0 specifically: no breaking changes — this is the
first release.**

### Migration

**First release.** No upgrade path applies. Operators evaluating
v0.1.0 should follow [`docs/project/GETTING-STARTED.md`](docs/project/GETTING-STARTED.md)
end-to-end on a fresh VM; the `archive/v0` branch carries the
pre-reset codebase for reference but is not a migration source — the
reset was intentional.

### Verification

v0.1.0 ships under the single-signer release ceremony documented in
[`RELEASE-PLAYBOOK.md`](RELEASE-PLAYBOOK.md) (release lines: `v0.x`
row of the matrix). End-user verification per
[Appendix A "v0.x / v1.0 / v1.1 (single-signer)"](RELEASE-PLAYBOOK.md#v0x--v10--v11-single-signer):

```bash
# Import the Keystone Core release public key
curl -sSL https://keys.keystone-core.io/release-pubkey.asc | gpg --import

# Verify the checksum signature
gpg --verify checksums.txt.sig checksums.txt

# Verify artifact integrity
sha256sum -c checksums.txt

# Verify container images
cosign verify --key release-cosign.pub ghcr.io/kscore/kscore-server:v0.1.0
```

Multi-party verification (`.sig.A` / `.sig.B` / `.sig.C` suffixes)
applies from v1.2.0 onward — see the playbook's v1.2 graduation
checklist for the onboarding path.

### Acknowledgments

Project sponsor: **Spicer Creek Solutions LLC** ([`OWNERSHIP.md`](OWNERSHIP.md)).

Contribution policy: [`CONTRIBUTING.md`](CONTRIBUTING.md) (DCO
sign-off required on every commit). AI-assisted contributions are
disclosed per [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md);
the reconstruction was done substantially with AI assistance and the
practice is documented openly.

Public hosting (primary): [`codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core).
GitHub mirror for discoverability: [`github.com/Spicer-Creek-Solutions-LLC/keystone-core`](https://github.com/Spicer-Creek-Solutions-LLC/keystone-core).

Issues, RFCs, and discussion live on Codeberg.

[Unreleased]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/compare/14be1109...HEAD
[v0.1.0]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/releases/tag/v0.1.0
[v1.0.0]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/releases/tag/v1.0.0
