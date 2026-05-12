# v1.x Backlog — Implementation-Time Deferrals

Single source of truth for scope **narrowed during implementation** of v1.0. Distinct from `FEATURES.md` (up-front product scope) and `PROJECT-DETAILS.md §6.2` (release roadmap) — those capture what was *planned*. This file captures what was *cut* once contact with the code revealed friction, so v1.x epics can pick the work back up without archaeology through `(landed: …)` annotations.

## How to use this file

- **When deferring scope mid-task**: add an entry under the target version. Cite the epic + task that produced the deferral and the source-of-truth file/line where the placeholder was committed.
- **When picking up a v1.x epic**: grep this file for the version tag — every implementation-time narrowing for that release is here.
- **When the deferred work lands**: move the entry to a `### Done` block under the version with a one-line "landed in commit/PR" note, or delete it if cleanup is obvious from the diff.

Format: each entry is a `####` heading; body has **What / Why deferred / Acceptance / References** lines.

---

## v1.1

#### Agent-side cancel propagation (SIGTERM to in-flight commands)
- **What**: When `CancelBatchJob` fires server-side, also signal the affected agents over NATS so any in-flight `os/exec` process is SIGTERM'd. v1.0 cancel persists CANCELLED status server-side; agent-side in-flight processes keep running until they exit naturally.
- **Why deferred**: Needs a new NATS cancel-command message type (subject scheme, envelope shape, signing) and an agent-side handler that maps inbound cancels to per-command context.CancelFuncs in the executor. Each piece is small but together they're a meaningful surface. v1.0 trial scope tolerates the gap — long-running commands time out via agent.ExecutorConfig.DefaultTimeout.
- **Acceptance**: `kscorectl exec cancel <id>` mid-batch results in agent-side processes receiving SIGTERM (then SIGKILL after KillGrace); per-agent batch_agent_result rows record the cancelled state.
- **References**: Epic 07 task 12 acceptance bullet ("agent receives SIGTERM"); `internal/agent/executor.go` for the SIGTERM-then-SIGKILL kill protocol that already exists.

#### Unified single + batch dispatch persistence
- **What**: Today every batch agent dispatch creates both a `commands` row (via `CommandDispatcher.Dispatch`) and a `batch_agent_results` row. At fleet scale that's a 2× write cost per command.
- **Why deferred**: Reusing `CommandDispatcher` gives us free retention + consistent signing for v1.0, but past a certain batch volume the duplication will show up in disk pressure / query cost.
- **Acceptance**: Batch dispatches write to a single row family (TBD: extend batch_agent_results, OR keep commands and drop batch_agent_results, OR linked via a `commands.batch_id` FK). Pick depends on observed query shape.
- **References**: Epic 07 task 12 layering note; `internal/controlplane/nats_batch_executor.go`.

#### Server-side target expression compile (proto extension)
- **What**: Add `string target_expression` to `v1.BatchExecuteCommandRequest` (or a new `Target.expression` field) plus a server-side resolver that runs the full `internal/targeting` shorthand — `os:linux`, `arch:amd64`, `status:online`, `ip:10.0.0.0/8`, `id:web-*` (glob), `OR`, `NOT`, parens — and returns the matching agent set. kscorectl `exec` subcommands switch from client-side `ParseTarget` (limited to AND-of-labels-plus-hostname-glob) to sending the raw expression string.
- **Why deferred**: v1.0 proto `Target` only carries `agent_ids`, `labels`, `hostname_pattern` — three AND'd dimensions. kscorectl `exec` 11a parses the shorthand client-side and rejects (`ErrTargetUnsupported`) anything that doesn't fit. Adding a string field is a proto change + regenerated stubs + GRPCServer wiring; we held it out of 11a to keep the CLI commit narrow.
- **Acceptance**: `kscorectl exec run "uptime" --target "os:linux AND (role:web OR role:cache)"` resolves correctly against a stretched fleet; the AND-of-labels-plus-hostname-glob path remains a valid (degenerate) subset.
- **References**: Epic 07 task 11a; `internal/cli/exec/target.go` `ParseTarget` + `ErrTargetUnsupported`; `internal/controlplane/agent_resolver.go` (the existing server-side filter is the foundation).

#### Batch job retention (DeleteBatchJobsBefore + cascading FK)
- **What**: `BatchJobStore.DeleteBatchJobsBefore(t)` analogous to `CommandStore.DeleteCommandsBefore`, plus `ON DELETE CASCADE` on the `batch_agent_results.batch_job_id` FK so batch cleanup wipes the per-agent rows in lockstep. A retention loop on `BatchDispatcher` (`runRetention`-shaped, mirroring CommandDispatcher) drops batches older than the configured TTL.
- **Why deferred**: Pre-1.0 deployments don't accumulate enough batches to need retention before trial-readiness. The store surface and the retention loop are independent additions; either can land first.
- **Acceptance**: `BatchJobStore.DeleteBatchJobsBefore(t)` deletes both `batch_jobs` rows and dependent `batch_agent_results` rows in a single tx (or cascade); `BatchDispatcher` has a configurable retention TTL and a periodic sweeper; CLI shows expected behavior after one TTL elapses.
- **References**: Epic 07 task 10; `internal/state/store.go` `BatchJobStore`; `internal/controlplane/command_dispatcher.go` `runRetention` as the pattern.

#### Live mid-execution stdout / stderr streaming
- **What**: Wire `BatchAgentOutput` chunks into the `BatchExecuteCommand` (and `CommandOutputChunk`s into the `ExecuteCommand`) gRPC streams as the agent produces them, instead of one buffered chunk at completion.
- **Why deferred**: v1.0 agents return stdout / stderr buffered inside `CommandResponse` — no live mid-execution publish. The gRPC streaming protocol's shape allows interleaved output chunks (per PROJECT-DETAILS §4.7), so callers won't see kscorectl `--follow`-style live output until the agent grows an incremental publish path and the server-side response correlator forwards chunks before the terminal `CommandResponse`.
- **Acceptance**: A long-running command (e.g., `tail -F`) streams partial stdout to a connected `kscorectl exec` over gRPC; agent disconnect mid-stream surfaces as `AGENT_FAILED` with the partial bytes already flushed.
- **References**: Epic 07 task 9; `internal/controlplane/grpc_server.go`; `internal/agent/agent.go` command handler.

#### Eval-time observability for malformed targeting patterns
- **What**: Slog hooks (or counters) for `match()` calls that swallow a malformed glob or an unparseable IP-vs-CIDR comparison. Today both fall through to `false` silently.
- **Why deferred**: The match function lives in a hot path (one call per agent per term per batch). Per-Matcher logger threading needs either a closure-built program per dispatch or a context plumbed through `expr.Run`; both add overhead the v1.0 trial doesn't need. Operators get a clear *parse* error from `Compile`; the gap is only at runtime.
- **Acceptance**: A target expression with a literal-asterisk pattern (`role:*`-with-meta-on-empty-value) or a bad CIDR logs once per dispatch with the offending pattern, evaluation count, and target-expression raw form; no per-agent log spam.
- **References**: Epic 07 task 3; `internal/targeting/match.go` `matchValue`.

#### Replay protection on agent commands
- **What**: Timestamp window + nonce dedup in `internal/agent.SecurityEnforcer.Validate`. Today HMAC alone gates command execution.
- **Why deferred**: HMAC covers v1.0 trial scope. Nonce store needs a persistence layer + TTL eviction; not worth the complexity for the v1.0 ship date.
- **Acceptance**: Replayed `CommandRequest` (same nonce within window) is rejected with a typed error and audit log; legitimate commands inside the window pass.
- **References**: Epic 06 task 4 `_(landed)_` annotation; PROJECT-DETAILS §4.10; `internal/agent/security.go`.

#### Schema versioning via `golang-migrate`
- **What**: Replace auto-DDL on startup with versioned migrations.
- **Why deferred**: v1.0 schema is new and stable; auto-DDL is fine until the first breaking change.
- **Acceptance**: `kscore-migrate up/down` cycles cleanly across at least one breaking schema change; CI runs migrations against PostgreSQL + SQLite.
- **References**: PROJECT-DETAILS §4.3 (line 301); Epic 02 scope-out.

#### Reactor engine + event lifecycle tracking
- **What**: Filter→action chains with throttle/debounce/DLQ + lifecycle states (created/published/routed/processing/processed/failed/expired).
- **Why deferred**: v1.0 ships passive event system (emit/subscribe/query). Reactors need runbook + policy boundaries to be settled.
- **Acceptance**: `LogAction` / `EventAction` / `WebhookAction` reactors fire on event match with throttle + DLQ semantics.
- **References**: PROJECT-DETAILS §4.9 (lines 679-681).

#### Trust federation (cross-domain bundle endpoint)
- **What**: SPIFFE federation — fetch + verify trust bundles from peer domains.
- **Why deferred**: v1.0 ships single-domain embedded CA; federation needs bundle distribution protocol.
- **Acceptance**: `kscore-identity federation add-domain/list/fetch-bundle` works against a second cluster.
- **References**: Epic 09 scope-out (line 23); PROJECT-DETAILS §4.10.

#### Maintenance + Schedule gRPC
- **What**: gRPC handlers for `MaintenanceService` and `ScheduleService` (REST is in v1.0).
- **Why deferred**: gRPC + REST land together at v1.1 per design.
- **Acceptance**: gRPC clients call `MaintenanceService.{Plan,Apply,Status}` and `ScheduleService.{Create,List,Run}`.
- **References**: Epic 03 scope-out (line 33); `pkg/api/maintenance/handler.go:5`.

#### Windows agent (native service)
- **What**: kscore-agent on Windows with `service install/uninstall/start/stop/status` subcommands and SCM integration.
- **Why deferred**: v1.0 platform target = Linux amd64+arm64. Windows needs separate stdlib (`win_feature`, `win_firewall`, etc.) and user-switching — `internal/agent/exec_user_windows.go:15` is a stub returning `not supported in v1.0`.
- **Acceptance**: kscore-agent runs as a Windows service; SIGTERM equivalent triggers clean shutdown; user switching works via `LogonUser` / `CreateProcessAsUser`.
- **References**: PROJECT-DETAILS §4.6 (line 487), §4.8 (line 619); `internal/agent/exec_user_windows.go`.

#### WASM module runtime
- **What**: Wazero-based module execution alongside the v1.0 Starlark runtime.
- **Why deferred**: Starlark covers v1.0 module needs; WASM adds a second runtime to maintain.
- **Acceptance**: `pkg/plugin/runtime/wasm` loads + runs a signed wasm module via the same `Runtime` interface as Starlark.
- **References**: Epic 00 deferred list (line 69).

#### Server-side heartbeat / metadata NATS subscriber → agent registry
- **What**: kscore-server has `internal/controlplane.ConnectionManager.Heartbeat(ctx, id)` and `UpdateAgent` plumbing, but nothing in v1.0 subscribes to `kscore.{cluster}.agent.heartbeat` / `kscore.{cluster}.agent.{id}.state` and feeds those payloads into the registry. Agents publish heartbeats and metadata into the void; the server's agent registry stays empty.
- **Why deferred**: Epic 06 owns the agent side (publishing) — it shipped. The consumer side is a server-runtime concern that fits naturally with Epic 07 (remote execution targeting reads from the registry) or Epic 13 (clustering / agent registry HA). Either of those epics will land the bridge.
- **Acceptance**: Three Epic 06 acceptance bullets that gate on this consumer flip to ✓ once the bridge lands —
    1. "Agent registers with control plane on startup (visible in `kscorectl agents list`)"
    2. "Heartbeat every 30s; control plane marks stale after 3 missed"
    3. "Metadata published on startup + every 60s; visible via `kscorectl agents show <id>`"
  Plus: a server-side integration test driving an agent → server registry visible round-trip.
- **References**: Epic 06 task 12 `_(landed)_` surfaced the gap; `internal/controlplane/connection_manager.go:218` (Heartbeat method waiting for a caller); agent publishes at `internal/agent/agent.go` (`runHeartbeatLoop`, `runMetadataLoop`).

#### `service` stdlib module — OpenRC / sysvinit / launchd backends
- **What**: Epic 08 task 11f ships the `service` module with systemd-only. Hosts with a different init system (Alpine's default OpenRC, Gentoo OpenRC/sysvinit, older RHEL/CentOS sysvinit, macOS launchd) get `service.ErrNoBackend` from mutating ops. Add provider implementations:
    - `openrc` (Alpine / Gentoo) — task 11f2
    - `sysvinit` — post-v1.0
    - `launchd` (macOS) — post-v1.0
- **Why deferred**: systemd covers the whole Epic 08 cross-distro Docker matrix (Debian 12, Ubuntu 22.04/24.04, RHEL 9, Rocky 9 — Alpine 3.19 defaults to OpenRC but a systemd variant exists). One backend at a time keeps PRs reviewable; the Provider interface + detection skeleton is in place.
- **Acceptance**: openrc backend added (rc-service / rc-update wrappers); auto-detect picks the right init system per host; the `service` module's idempotency tests pass on Alpine 3.19 (OpenRC) in addition to the systemd distros.
- **References**: Epic 08 task 11f; `internal/statemgmt/stdlib/service/detect_linux.go` `defaultProvider`; `internal/statemgmt/stdlib/service/systemd.go` as the template.

#### `package` stdlib module — dnf, apk, zypper, pacman backends
- **What**: Epic 08 task 11e ships the `package` module with apt-only (Debian / Ubuntu). On hosts where no supported package manager is detected the module returns `pkg.ErrNoBackend` ("no supported package manager detected on this host") rather than silently doing nothing. Add provider implementations for the other Linux package managers:
    - `dnf` (RHEL 8+ / Rocky / Fedora) — task 11e2
    - `apk` (Alpine) — task 11e3
    - `zypper` (openSUSE / SLES) — post-v1.0
    - `pacman` (Arch) — post-v1.0
- **Why deferred**: One backend at a time keeps PRs reviewable; the Provider interface + detection skeleton is in place, so adding a backend is a new file + a branch in `detect_linux.go` + per-backend tests.
- **Acceptance**: dnf + apk backends added (with appropriate command-line parsers); auto-detect picks the right backend per host; the Epic 08 cross-distro Docker matrix (Debian 12, Ubuntu 22.04/24.04, RHEL 9, Rocky 9, Alpine 3.19) passes the `package` module's idempotency tests.
- **References**: Epic 08 task 11e; `internal/statemgmt/stdlib/pkg/detect_linux.go` `defaultProvider`; `internal/statemgmt/stdlib/pkg/apt.go` as the template.

#### `state.apply.skip` event taxonomy + wiring
- **What**: Epic 08 task 6 ships a `RunObserver.Skip` callback for cascade-skipped declarations (an earlier failure aborted the run; subsequent decls don't execute but are surfaced via the observer so external subscribers — alerting, audit, dashboards — see them). The statemgmt runner does not own event-subject naming; that's Epic 11. The corresponding event type `state.apply.skip` is therefore NOT yet listed in PROJECT-DETAILS §4.9's event taxonomy ("agent 5 / job 4 / state 5 / system 3 / user 3 / policy 2 = 22 types"). It needs to be added when Epic 11 wires the runner observer to the NATS event bus.
- **Why deferred**: The statemgmt task adds the observer hook; the event system itself lives in Epic 11. Updating §4.9's taxonomy now would predict an event Epic 11 hasn't shipped.
- **Acceptance**: PROJECT-DETAILS §4.9 lists `state.apply.skip` alongside the existing 5 state.* types (state.* becomes 6 types, total 23); Epic 11's event publisher emits `kscore.{cluster}.events.state.apply.skip` for each cascade-skipped decl; an integration test asserts the subject lands in JetStream.
- **References**: Epic 08 task 6; `internal/statemgmt/runner.go` `RunObserver.Skip`; PROJECT-DETAILS §4.9 event taxonomy; Epic 11 (event system).

#### Salt-faithful `prereq` direction in statemgmt resolver
- **What**: The Epic 08 dependency resolver applies a **uniform direction rule** to all eight requisite keys: `<key>: [B]` on A puts B before A; `<key>_in: [B]` puts A before B. Salt's actual `prereq` semantic is the opposite — Salt reads `prereq: [B]` on A as "A is a prerequisite for B" (A first). Keystone's rule deviates so all eight keys teach the same way.
- **Why deferred**: One rule is much easier to teach + remember than a per-key direction table. v1.0 trial scope hasn't surfaced a real workflow that needs Salt-faithful prereq; if it does, we add a per-key direction policy in the resolver and surface it on the DSL.
- **Acceptance**: Resolver applies a per-key direction policy where `prereq` and `prereq_in` use the Salt-faithful convention while keeping `require` / `watch` / `onchanges` (and their `_in` variants) on the existing uniform rule; docs in PROJECT-DETAILS §4.8 reflect the per-key directions explicitly.
- **References**: Epic 08 task 5; `internal/statemgmt/resolve.go` package comment; PROJECT-DETAILS §4.8.

#### PROJECT-DETAILS §4.8 DSL example uses plural module keys
- **What**: The state-file DSL example in `PROJECT-DETAILS.md §4.8` uses plural top-level keys (`packages:`, `files:`, `services:`) while its requisite references use singular module names (`[package: nginx]`, `[file: ...]`). Epic 08 task 2 ships the parser with **singular** top-level keys so one rule covers both surfaces. The doc example needs to be updated to match.
- **Why deferred**: Pure doc cleanup; not blocking. Touching `PROJECT-DETAILS.md` mid-implementation muddies the diff for Epic 08 task 2 (the parser PR), which is the natural place to confirm the DSL shape. Easier to land the parser, get DSL examples committed to `internal/statemgmt/testdata/`, then sync the spec doc in a small follow-up.
- **Acceptance**: §4.8's YAML example uses `package:`, `file:`, `service:` (singular) consistent with the parser and the requisite-reference syntax; testdata fixtures are referenced as the canonical examples.
- **References**: Epic 08 task 2; `internal/statemgmt/parse.go`; `internal/statemgmt/testdata/webserver.yaml`; `PROJECT-DETAILS.md` §4.8 lines 580–605.

## v1.2

#### TUI monitor (`kscore-monitor`)
- **What**: Bubble Tea single-pane-of-glass — 8 base views (dashboard/agents/events/state/policy/jobs/logs/metrics), 13 with enhancements (cluster/secrets/leases/schedules/runbooks/webhooks).
- **Why deferred**: v1.0 ships Grafana dashboards + CLI. TUI needs gRPC-multiplex client + NATS subscriber. Not blocking trial.
- **Acceptance**: `kscore-monitor` opens, navigates between views, refreshes live.
- **References**: PROJECT-DETAILS §4.16 (line 1123); Epic 17.

#### Full RBAC role/permission CRUD
- **What**: `Role`/`Permission` CRUD with per-resource permissions + dynamic policy. v1.0 ships fixed admin/operator/readonly.
- **Why deferred**: 3-role model covers trial scope; full CRUD needs UI affordances we don't have yet.
- **Acceptance**: Operators define custom roles via `kscorectl roles create`; bindings to principals enforce on API calls.
- **References**: Epic 09 scope-out (line 22); PROJECT-DETAILS §4.10 (line 737).

#### Multi-table transaction wrapper
- **What**: A `state.Tx` type that batches mutations across tables atomically.
- **Why deferred**: v1.0 stores can serialize through caller-side coordination; full transaction wrapper needs careful error-handling design.
- **Acceptance**: Cross-table writes (agent + apikey + audit) commit/rollback atomically; tests verify rollback on mid-batch failure.
- **References**: Epic 02 scope-out (line 24); `internal/controlplane/bootstrap.go:270` (API key issuance currently non-transactional).

#### macOS agent
- **What**: kscore-agent on macOS with launchd integration.
- **Why deferred**: Linux is v1.0 target; macOS adds `launchd` stdlib + Keychain integration.
- **Acceptance**: kscore-agent runs under launchd; agent identity stored in Keychain.
- **References**: PROJECT-DETAILS §4.6 (line 487), §4.8 (line 619).

#### Percentage-based / rolling batch execution
- **What**: `kscorectl exec run --rolling 25%` for staged rollouts.
- **Why deferred**: v1.0 ships full-fanout + concurrency-cap; rolling needs progress-pause/resume semantics.
- **Acceptance**: A 100-target batch with `--rolling 10%` runs in 10 waves; failure-rate threshold halts the rollout.
- **References**: Epic 07 scope-out (line 29).

## v1.3

#### Auto-rotation of in-memory NATS creds
- **What**: Agent rotates NATS credentials without restart.
- **Why deferred**: Gates on SPIRE provider (v1.3). v1.0 rotation = restart.
- **Acceptance**: Agent re-issues NATS creds on cert rotation event without dropping the connection.
- **References**: Epic 06 scope-out (line 33); PROJECT-DETAILS §4.6 (line 482).

#### SPIRE-backed identity provider
- **What**: Swap embedded CA for external SPIRE server + agent socket.
- **Why deferred**: Embedded CA covers v1.0; SPIRE needs operational tooling.
- **Acceptance**: `IdentityProvider` interface backed by SPIRE socket; SVID rotation is automatic.
- **References**: Epic 09 scope-out (line 24); PROJECT-DETAILS §4.10 (line 695).

#### K8s operator
- **What**: CRDs + reconciler for declarative Keystone Core management.
- **Why deferred**: K8s is one deployment target; v1.0 ships standalone-binary baseline.
- **Acceptance**: `Cluster`, `Agent`, `Blueprint` CRDs reconcile against a live cluster.
- **References**: Epic 00 deferred list (line 73).

#### Weighted endpoint load distribution + K8s endpoint discovery
- **What**: `cfg.NATS.Endpoints[].Weight` actually distributes load; K8s service-name endpoint discovery.
- **Why deferred**: v1.0 uses priority-only endpoint selection; weight is reserved in the schema. K8s discovery is part of the operator work.
- **Acceptance**: Load measurably distributes proportionally to weights; `nats.urls = ["k8s://service-name"]` resolves through discovery.
- **References**: `internal/config/nats.go:113`; `internal/nats/subject.go:135`.

#### AWS decorrelated jitter for fleet-scale reconnect storms
- **What**: Replace symmetric exp-jitter (`reconnectDelay` in `internal/nats/backoff.go`) with AWS decorrelated jitter — `delay = min(max, random(base, prev_delay * 3))`. Better herd-avoidance properties at >500-agent scale.
- **Why deferred**: Symmetric jitter is adequate for v1.0 trial fleets (≤500 agents per design). Decorrelated needs careful per-call state tracking and is harder to test deterministically.
- **Acceptance**: `reconnectDelay` (or a sibling) implements decorrelated jitter; benchmark/sim shows tighter reconnect-time distribution at 1000+ agent scale; opt-in via a config knob (`reconnectjitterstrategy: symmetric|decorrelated`).
- **References**: Epic 06 task 10 `_(landed)_`; `internal/nats/backoff.go`.

## v1.4

#### Rotation orchestrator
- **What**: Strategies (blue-green, rolling, canary, immediate) with health checks + auto-rollback.
- **Why deferred**: v1.0 secrets ship with manual rotation; orchestration is its own domain.
- **Acceptance**: `kscore-secrets rotate` invokes a strategy, runs health checks, rolls back on failure.
- **References**: Epic 00 deferred list (line 77); PROJECT-DETAILS §4.11 (line 765).

#### Telemetry gateway
- **What**: `kscore-telemetry-gateway` — standalone collector for logs/metrics/traces over NATS subjects.
- **Why deferred**: v1.0 emits OTel/Prom directly to operator-supplied backends.
- **Acceptance**: Gateway aggregates from N agents; forwards to Loki/Prom/Jaeger.
- **References**: PROJECT-DETAILS §4.16 (line 1152); Epic 17.

#### Output archival to object storage cold-tier
- **What**: Long-running batch results overflow to S3/GCS/Azure cold tier.
- **Why deferred**: v1.0 keeps results in PostgreSQL with size cap.
- **Acceptance**: Outputs > N MiB archive to operator-configured bucket; `kscorectl exec show` fetches from cold tier.
- **References**: Epic 07 scope-out (line 30).

## v1.5

#### Encryption at rest (`KeyProvider`)
- **What**: Pluggable `KeyProvider` (file/age/Vault/Cloud KMS) encrypts secrets + audit logs at rest.
- **Why deferred**: PostgreSQL TDE + filesystem encryption cover v1.0 trial. Full implementation gates on cloud KMS work.
- **Acceptance**: `cfg.storage.encryption.provider = age|vault|aws-kms` round-trips encrypted blobs; key rotation re-wraps.
- **References**: Epic 02 scope-out (line 23); PROJECT-DETAILS §4.3 (line 303); §4.11.

#### Saga/checkpoint advanced features
- **What**: Resume from checkpoint, cross-state compensation graphs.
- **Why deferred**: v1.0 ships saga coordinator scaffolding only.
- **Acceptance**: A 10-step saga survives a crash mid-step 5, resumes from checkpoint, completes the remainder.
- **References**: PROJECT-DETAILS §4.8 (line 625).

#### File distribution: NATS Object Store + Git backends, mirror groups, conflict resolution
- **What**: Multi-backend file dist with mirror groups, sync engine, conflict resolution strategies.
- **Why deferred**: v1.0 ships local FS + S3.
- **Acceptance**: Mirror group spans 3 backends; conflict resolution (newest-wins/largest-wins/primary-wins/manual) is operator-selectable.
- **References**: Epic 18 scope-out (lines 69-73).

#### Backup orchestration features
- **What**: Automated scheduling, rolling upgrades, drift detection on self-config, self-healing, DR test harness.
- **Why deferred**: v1.0 ships manual `kscore-backup` only.
- **Acceptance**: `kscore-backup schedule add` registers a cron; rolling upgrade verifies health between waves.
- **References**: Epic 18 scope-out (lines 61-65).

## v1.7

#### Air-gap baseline
- **What**: Offline registry, bootstrap packages, upgrade archives, full security scanning suite, signed module bundles, signing ceremony.
- **Why deferred**: v1.0 assumes online package repos.
- **Acceptance**: Operator installs Keystone Core in a fully air-gapped network; module updates flow through offline registry.
- **References**: PROJECT-DETAILS §6.2 (line 1496).

## v1.8

#### Policy enforcement (Enforce + Warn modes)
- **What**: Flip `policy.enforcement_enabled=true`. v1.0 ships policy engine in audit-mode-only.
- **Why deferred**: A misconfigured policy blocks the fleet. Audit-mode lets operators build confidence first.
- **Acceptance**: Policy in Enforce mode blocks command exec; Warn mode logs and allows.
- **References**: Epic 03 task 6; Epic 12 (audit/policy); PROJECT-DETAILS §4.12 (line 859).

#### Policy CRUD via gRPC (`CreatePolicy`/`UpdatePolicy`/`DeletePolicy`/`activate`/`deactivate`/`remediate`/`monitor`)
- **What**: Server-side mutating endpoints for policies (today returns Unimplemented).
- **Why deferred**: Mutations gate on enforcement going GA so operators can author policies that actually run.
- **Acceptance**: gRPC clients author policies via the API; CLI subcommands `create|update|delete|activate|deactivate|remediate|monitor` wire to the new endpoints.
- **References**: Epic 03 task spec (line 16); `pkg/api/policy/handler.go:5`; `pkg/api/v1/policy.pb.go:3`.

## v2.0

#### Embedded NATS / hybrid mode / leaf node / endpoint advertiser / supercluster / WebSocket
- **What**: Agents run embedded nats-servers; cluster forms via leaf-node mesh; reverse-leaf NAT traversal for agents behind firewalls; WebSocket transport for browser-side ops.
- **Why deferred**: v1.0 ships agent-as-NATS-client only; hybrid topology + WebSocket multiply test surface.
- **Acceptance**: Agent runs embedded NATS; reverse-leaf publishes its endpoint; supercluster gateway federates two clusters.
- **References**: Epic 05 scope-out (lines 38-42); Epic 06 scope-out (lines 27-28).

#### Active dial-time circuit breaker eviction
- **What**: Skip OPEN endpoints when nats.go picks the next reconnect target.
- **Why deferred**: Requires replacing nats.go's native multi-URL failover with a per-endpoint dial loop — substantial refactor for marginal v1.0 benefit.
- **Acceptance**: An endpoint with breaker OPEN is skipped during reconnect attempts until the breaker half-opens.
- **References**: Epic 05 task 7 `_(landed)_`; `internal/nats/breaker.go:20`.

#### Cloud workload identity (AWS IRSA, GCP WI, Azure MI)
- **What**: Cloud-native identity binding without static credentials.
- **Why deferred**: v1.0 ships embedded CA + JWT/PSK; cloud bindings need per-provider integration.
- **Acceptance**: Agent on EC2 with IRSA receives identity from instance metadata.
- **References**: Epic 09 scope-out (line 25).

#### gRPC-gateway annotation-driven REST + OpenAPI auto-gen
- **What**: REST + OpenAPI generated from proto annotations.
- **Why deferred**: v1.0 hand-codes both for control and simplicity (the v0 reset traced its complexity in part to grpc-gateway tooling friction).
- **Acceptance**: A new gRPC method automatically gets REST + OpenAPI without hand-edits.
- **References**: Epic 03 scope-out (line 30); PROJECT-DETAILS §4.5 (line 426).

---

## Implementation-time narrowings inside delivered v1.0 features

These don't move to a future version — they document where v1.0 *is* shipping but with reduced scope. Useful for release notes + "what's new in v1.x" planning.

#### Cluster-wide HMAC secret (vs per-agent)
- **What**: All agents share one HMAC secret in v1.0. Per-agent keys derived from bootstrap exchange land in v1.x (no specific version targeted yet).
- **Why now**: Per-agent keys need a key-distribution mechanism that's still being designed.
- **References**: Epic 06 task 6 `_(landed)_`; `internal/agent/security.go:71`; `internal/config/security.go:15`.

#### Bootstrap: demo mode only (TUI + non-interactive)
- **What**: Both `kscore-agent bootstrap` paths (TUI wizard from task 7 and `--non-interactive` flags from task 8) accept all three modes structurally but `bootstrap.ValidateForV10` rejects production / enterprise with a v1.x deferral message before the Engine reaches Validate.
- **Why now**: Production mode needs TLS cert collection (gates on Epic 11 — Identity & Auth); enterprise mode needs blueprint selection (gates on Epic 14 + Epic 17). Both are post-v1.0.
- **Acceptance for unblock**: TUI screens for cert paths + cert-generation toggle (production); blueprint picker (enterprise); equivalent `--generate-certs` / `--apply-blueprint` CLI flags wired to the non-interactive path. Drop or no-op `bootstrap.ValidateForV10`.
- **References**: Epic 06 tasks 7 + 8 `_(landed)_`; `internal/agent/bootstrap/configure.go` (search `ValidateForV10`); `cmd/kscore-agent/main.go` (search `buildConfigurer`).

#### Bootstrap CLI flags dropped from v1.0 surface
- **What**: The original Epic 06 task 8 spec listed `--postgres-*`, `--nats-*` beyond `--join`/`--join-token`, `--generate-certs`, and `--apply-blueprint`. v1.0 ships without them.
- **Why now**: `--postgres-*` is server-only and belongs in the future unified `kscore-bootstrap` binary (the agent doesn't run a database). Extra `--nats-*` flags are unnecessary because v1.0 agents are external-mode only — embedded NATS is v2.0. `--generate-certs` gates on Epic 11; `--apply-blueprint` gates on Epic 14 + 17.
- **Acceptance for unblock**: Per-flag — when its blocking epic lands, add the flag with appropriate plumbing. The `--state-path` flag added in task 8 stays.
- **References**: Epic 06 task 8 `_(landed)_`; `cmd/kscore-agent/main.go` (search `registerBootstrapFlags`).

#### Bootstrap wizard: storage backend + blueprint selection screens
- **What**: PROJECT-DETAILS §4.6 + Epic 06 task 7 originally envisioned the agent wizard collecting storage backend and applying blueprints. Both were dropped from the v1.0 agent surface — storage is server-only (the future unified `kscore-bootstrap` binary's concern); blueprint apply gates on Epic 14 + 17.
- **Why now**: Agents don't run a database, so the storage prompts had no destination. Blueprint apply needs the plugin/module system + blueprint runtime, neither of which ship in v1.0.
- **Acceptance for unblock**: For storage — `cmd/kscore-bootstrap` (unified server+agent binary) gains the storage screens. For blueprints — Epic 14 + 17 land, then a "select blueprints to apply" screen feeds an installer-side blueprint apply step.
- **References**: Epic 06 task 7 `_(landed)_`; PROJECT-DETAILS §4.6 (line 451).

#### Bootstrap auto-installs systemd unit (production mode)
- **What**: When demo-only mode-gate lifts, the bootstrap Engine's Install phase should call `systemd.Install` automatically — operator runs `kscore-agent bootstrap` once, gets both config and unit installed/enabled.
- **Why now**: Production mode is itself deferred (see "Bootstrap: demo mode only"), and v1.0's two-step flow (`bootstrap` then `service install`) is a clean explicit invocation that maps to the demo-mode-only world.
- **Acceptance for unblock**: When Epic 11 + 14 + 17 land and `bootstrap.ValidateForV10` lifts, `bootstrap.NewDefaultInstaller` (or a production-mode wrapper) chains `systemd.Install` after the YAML render.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/bootstrap/install.go`; `internal/agent/systemd/install.go`.

#### `kscore-agent service start|stop` subcommands
- **What**: PROJECT-DETAILS §4.6 lists `service install|uninstall|start|stop|status`. v1.0 ships `install|uninstall|status` only.
- **Why now**: `systemctl start kscore-agent` / `systemctl stop kscore-agent` are universally known by Linux operators; wrapping them adds maintenance for zero ergonomic value. Punted unless a specific operator workflow surfaces a need.
- **Acceptance for unblock**: A documented use case where `kscore-agent service start` is genuinely better than `systemctl start kscore-agent` (e.g. cross-platform script that needs the same command shape on Windows v1.1's SCM).
- **References**: Epic 06 task 9 `_(landed)_`; PROJECT-DETAILS §4.6 (line 473).

#### Dedicated `keystone-core` system user auto-creation
- **What**: v1.0 systemd unit defaults to root; `--user/--group` flags let operators run as a dedicated user, but the user must already exist (no auto-create).
- **Why now**: User creation belongs in package-mgmt territory (Epic 18 — rpm/deb post-install scripts via `useradd --system`). Doing it in `service install` would duplicate the package-mgmt path and split responsibility.
- **Acceptance for unblock**: Epic 18 packaging work creates the `keystone-core` system user as part of `apt install kscore-agent` / `dnf install kscore-agent`. After that, `service install` can default `--user keystone-core --group keystone-core` and update the rendered ReadWritePaths to match.
- **References**: Epic 06 task 9 `_(landed)_`; Epic 18 (file dist + package mgmt).

#### Type=notify systemd integration (sd_notify)
- **What**: v1.0 unit uses `Type=exec`. `Type=notify` would let systemd track agent readiness via `sd_notify("READY=1")` calls — useful for `Wants=kscore-agent.service` ordering and reliable health checks.
- **Why now**: Requires the daemon to call into `coreos/go-systemd/v22/daemon`'s `SdNotify`, which adds the dep we explicitly skipped for v1.0. Re-evaluate when v1.4 telemetry-gateway work surfaces a real consumer.
- **Acceptance for unblock**: Daemon calls `SdNotify("READY=1")` after NATS connect + initial heartbeat publishes; unit flips to `Type=notify`; `systemctl is-active` reports `activating` until ready.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/systemd/unit.go` (search `Type=exec`).

#### Bootstrap: no rollback / transactional revert
- **What**: Bootstrap engine resumes from checkpoint but doesn't revert side effects (config files, systemd units) on failure.
- **Why now**: Rollback semantics need install-step inversion + snapshot tracking.
- **Acceptance**: Failed bootstrap re-runs cleanly; for true rollback, operator runs `kscore-agent bootstrap --rollback` (v1.x).
- **References**: Epic 06 task 6; `internal/agent/bootstrap/doc.go:19`.

#### Glob matching: no `**` (double-star)
- **What**: `internal/agent.SecurityEnforcer` uses `path.Match` (single-star only). gobwas/glob with double-star semantics is reserved for v1.x.
- **Why now**: stdlib `path.Match` covers the v1.0 command-allowlist use cases.
- **References**: `internal/agent/security.go:59`.

#### API key issuance: non-transactional
- **What**: `internal/controlplane.BootstrapHandler` issues credentials via separate write paths (not a single transaction).
- **Why now**: v1.0 doesn't have multi-table tx wrapper (see v1.2 entry above).
- **References**: `internal/controlplane/bootstrap.go:270`.

#### Migration journal: no per-table checkpoint resume
- **What**: `kscore-migrate` records per-table checkpoints in the txlog but recovery from a partial migration restarts from the last full-table boundary, not the row-level checkpoint.
- **Why now**: Row-level resume needs a transactional checkpoint protocol that v1.0's `state.Tx` (deferred to v1.2) would unlock.
- **References**: `internal/state/migrate_txlog.go:25`.

#### Batch dispatcher: no orphan-job recovery
- **What**: Jobs in RUNNING state at process start are not auto-recovered; operators must inspect + retry.
- **Why now**: Orphan recovery needs ownership semantics (which CP node owns the job?) clarified by Epic 13 (clustering).
- **References**: `internal/controlplane/batch_dispatcher.go:72`.

#### Config: no per-endpoint TLS overrides
- **What**: `cfg.NATS.Endpoints[]` use the cluster-wide TLS config; per-endpoint overrides are reserved schema fields.
- **Why now**: v1.0 has one TLS strategy per cluster; mixed-TLS topologies are post-v1.0.
- **References**: `internal/config/nats.go:126`.

#### Boostrap PSK consumption: in-memory tracking only
- **What**: Used PSKs are tracked in-memory. Server restart re-permits a previously-used PSK.
- **Why now**: Persistence path lands in v1.x with the bootstrap-state-store work.
- **References**: `internal/config/nats.go:53`; Epic 05 task 9 `_(landed)_`.
