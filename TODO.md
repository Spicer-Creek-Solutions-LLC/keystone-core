# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Resolution Tags

Each TODO item includes a `Resolution:` line to indicate how it should be addressed:

- `doc` — update documentation to match current code behavior.
- `code` — update code to add the documented behavior and update documents to new behavior.
- `both` — update both docs and code.
- `decide` — needs triage to choose a direction.

## High Priority (Documentation Gaps)

### API Documentation Gaps

- [x] **Secrets API reference docs have no matching REST/gRPC implementation** *(Moved to Epic 43)*
  - See `epics/43-secrets-api-implementation.md` for full implementation plan
  - Covers REST handlers, gRPC service, public client package, CLI wiring

- [x] **Agent metrics REST endpoint documented but not implemented** ✓ FIXED
  - Resolution: code
  - Added `GET /api/v1/agents/{id}/metrics` endpoint to `pkg/api/agents/handlers.go`
  - Returns `LastMetrics` (cpu, memory, disk, load, connections) from agent heartbeats
  - Added `AgentMetricsResponse` type and tests

- [x] **Maintenance and API key REST endpoints referenced by CLI are not implemented** *(DONE)*
  - Added `pkg/api/maintenance/` package with `Store` interface, etcd and memory backends, 5 REST endpoints (enable, disable, status, queue, cleanup)
  - Added `pkg/api/apikeys/` package with `Store` interface, memory backend, 3 REST endpoints (create, list, revoke)
  - 34 tests across both packages, all passing with race detection

---

## Medium Priority (Consistency Issues)

### Configuration Field Naming

- [x] **NATS config docs include unsupported nested settings** *(DONE)*
  - Removed unsupported `nats.tls`, `nats.auth`, `embedded.server_name`, `embedded.max_payload`, `leaf_remotes`, `ping_interval`, `max_ping_out`
  - Removed `jetstream.max_memory`/`max_file`; only `max_storage` exists
  - Added actual embedded fields: host, port, enable_jetstream, storedir, max_memory, max_connections

- [x] **Configuration docs list control-plane sections not in config** *(DONE)*
  - Removed unsupported `agents`, `execution`, `state`, `events`, `gitops`, and `security` blocks from configuration.md
  - These are not wired in `internal/config.Config`

- [x] **Storage config docs include unsupported PostgreSQL fields** *(DONE)*
  - Removed unsupported host/port/database/username/password/sslmode and `max_connections`/`idle_connections`/`connection_lifetime`
  - Removed SQLite `max_connections`; only `path`, `wal`, and `busy_timeout` exist
  - Kept `dsn`, `max_open_conns`, `max_idle_conns`, `conn_max_lifetime` for PostgreSQL

- [x] **Policy config docs include unsupported cache/OPA/CEL settings** *(DONE)*
  - Removed unsupported `cache_ttl`, `evaluation_timeout`, `opa.*`, and `cel.*` blocks
  - Only `enabled`, `engine`, `enforcement_mode`, and `builtin_policies` exist in PolicyConfig

- [x] **Update telemetry gateway deploy configs to match gateway schema** *(DONE)*
  - Updated all 3 deploy configs to match `internal/gateway/types.go` struct field names
  - Fixed: `nats.cluster`, `stale_timeout`, `loki.batch_wait`, `otlp.endpoint/compression`, `ha.queue_group/leader_election`
  - Removed: `logging.*`, `max_entries`, `max_age`, `max_traces`, flat sampling fields, `instance_id/shard_count/cleanup_interval`

- [x] **Document default path differences** *(DONE)*
  - Added inline comments to configuration.md noting code defaults (./data/nats, ./data/keystone-core.db)
  - Docs examples show production paths; comments clarify that defaults are relative

- [x] **Agent config docs include fields not in config structs** *(DONE)*
  - Removed unsupported `datacenter`, `environment`, `role`, `tags`, `metadata`, `heartbeat.*`, `execution.*`, `state.*` sections
  - Added actual fields: `heartbeat_interval`, `command_timeout`, `metadata_interval`
  - Note: datacenter/environment/role should be set as labels

- [x] **Logging file output is documented but not supported** *(DONE)*
  - Removed unsupported `file:` field from agent config example
  - Changed production example from `output: file` to `output: syslog` with syslog config
  - Deploy scripts and systemd services correctly use syslog/journald

---

## Additional Documentation Drift (In Progress)

### Compatibility API Reference Drift

- [x] **Compatibility Go package referenced in docs does not exist** *(DONE)*
  - Updated compatibility.md code example to use `pkg/semver` (Compare, Diff, IsCompatible)

### File Backends Reference Drift

- [x] **File backends config schema and supported types don't match implementation** *(DONE)*
  - Resolution: both
  - Wired all 6 backend types in `createBackend()`: `filesystem`/`local`, `s3`, `gcs`, `azure`, `git`, `nats`/`nats-object-store`
  - Extended `BackendConfig` with cloud-specific fields matching internal backend configs
  - Fixed docs to use `backends:` array format with flat fields matching code struct
  - Fixed field name mismatches (`project_id`→`project`, `account`→`account_name`, `access_key`→`account_key`, `bucket`→`bucket_name` for NATS)
  - Added 10 tests for `createBackend()` covering all types and aliases

### Concepts Documentation CLI Drift

- [x] **Events concept documents subcommands not implemented** *(DONE)*
  - Added `analyze`, `subscribers`, `storage-stats`, `prune`, and `archive` subcommands to `cmd/kscore-events/main.go`

- [x] **GitOps concept references `git-sync` CLI that does not exist** *(DONE)*
  - Resolution: code
  - Added `git-sync` subcommand group to `cmd/kscore-gitops/main.go` with 13 commands:
    status, trigger, force, conflicts (list/show/diff/resolve/resolve-all), lock, unlock, locks, history, audit
  - Fixed docs to match CLI syntax (`locks list` → `locks`, removed unsupported `--push` flag)
  - Added 26 tests covering all subcommands, flags, error cases, and output formats

- [x] **Edge concept references `kscorectl cache`, `kscorectl sync`, and `kscorectl connection`** *(DONE - cache commands)*
  - Added `cache invalidate/verify/show/refresh/set-ttl/history` subcommands to `cmd/kscore-files`
  - Note: `sync status/force` and `connection test` commands still need implementation

- [x] **Remote execution concept references unsupported exec subcommands/flags** *(DONE)*
  - Added `archive`, `export`, and `cleanup` subcommands to `cmd/kscore-exec/main.go`
  - Export supports JSON and CSV formats; cleanup requires --older-than with dry-run support

- [x] **Blueprints concept references unsupported blueprint commands** *(DONE)*
  - Added `blueprint bundle create/install`, `blueprint mirror add/remove/list/sync/status`, and `blueprint applied list/usage/history` subcommands to `cmd/kscore-blueprint`

- [x] **Identity concept references `kscore-agent identity`** *(DONE)*
  - Added `identity show/renew/status` subcommands to `cmd/kscore-agent`
  - Shows SVID details, trust bundle, renewal, and expiry status

- [x] **NATS mesh concept references missing CLI helpers** *(DONE - agent nats subcommands)*
  - Added `nats ping/status/test` subcommands to `cmd/kscore-agent`
  - Note: `kscorectl debug nats` still needs implementation

- [x] **Agents concept/CLI naming mismatch** *(DONE)*
  - Fixed all 30+ help text examples in `cmd/kscore-agents/main.go` from `kscorectl agent` to `kscorectl agents`
  - Previously fixed: doc references in agents.md and service-mesh.md

### Operations Documentation CLI Drift

- [x] **NATS mesh operations docs reference missing CLI helpers** *(DONE - agent nats subcommands)*
  - Added `kscore-agent nats ping/status/test` subcommands
  - Note: `kscorectl debug nats` and `kscorectl agent update-certs` still need implementation

- [x] **Windows ops docs use unsupported exec/state flags** *(DONE)*
  - Added `--shell` flag to `exec run` for specifying shell (powershell, cmd, bash)
  - Added `--check-only` alias for `--dry-run` on `state apply`
  - Added `--validate-config` flag to `kscore-agent` for config validation and exit

- [x] **Migrations ops docs reference missing commands/flags** *(DONE)*
  - Added `migrate verify --source-system` command supporting salt, ansible, puppet, chef
  - Verifies source data readable, state definitions migrated, agent mappings, variables, event subscriptions

- [x] **Secrets backends ops docs reference missing secrets commands** *(DONE)*
  - Added `secrets get`, `secrets list`, `secrets backends`, and `secrets audit` subcommands to `cmd/kscore-secrets`

### Runbooks and Community Documentation CLI Drift

- [x] **Runbooks reference missing auth/debug/config/db/diagnostics commands** *(Done: added `auth` (login, revoke-all, sessions, rotate-signing-key, key), `config set`/`config show`, `db` (compact, rotate-credentials), `diagnostics` (collect) command groups to kscorectl with 30+ tests)*
  - Resolution: code
  - `docs/runbooks/bootstrap-new-cluster.md`, `docs/runbooks/performance-degradation.md`, `docs/runbooks/security-incident.md`, `docs/runbooks/disaster-recovery.md` use `kscorectl auth ...`, `kscorectl debug ...`, `kscorectl config set ...`, `kscorectl db compact/rotate-credentials`, `kscorectl diagnostics collect`
  - None of these command groups exist in `cmd/kscorectl/main.go`

- [x] **Runbooks reference unsupported cluster/federation subcommands** *(Done: added `cluster member add/remove`, `cluster election restart`, `--token`/`--advertise-addr` on `cluster join`, `federation status`, `federation trust list`, `federation ping --region`; `cluster token generate/list/revoke` deferred to Epic 44)*
  - Resolution: code (6 commands real implementations; `cluster token` deferred to Epic 44)
  - `docs/runbooks/bootstrap-new-cluster.md`, `docs/runbooks/disaster-recovery.md`, `docs/runbooks/capacity-scaling.md` use `cluster token`, `cluster quorum`, `cluster join-config`, `cluster member add/remove`
  - `docs/runbooks/bootstrap-new-cluster.md` references `federation status`, `federation trust list`, `federation ping`
  - `cmd/kscore-cluster` and `cmd/kscore-federation` do not implement these subcommands
  - `cluster token generate/list/revoke` requires server-side token store → Epic 44 (`epics/44-cluster-join-tokens.md`)

- [x] **Runbooks reference unsupported bootstrap and upgrade commands** ✅ Done
  - Bootstrap: added `join` (HTTP-based cluster join), `prereq-check` (system validation), `cert-gen` (ECDSA TLS cert generation)
  - Upgrade: added `path` (version path analysis), `resume` (interrupted upgrade recovery)
  - Upgrade flags: `--from` on check, `--backup-before`/`--auto-rollback` on execute, `--verbose` on status, `--status`/`--retry`/`--skip` on agents

- [x] **Runbooks reference unsupported agent/security commands** ✅ Done
  - Added `agent list --suspicious`, `agent verify` (--all, --sample), `agent certificates regenerate` (--all, --force) to kscore-agents
  - Added `security scan` (--full, --output, --targets) and `nats rotate-credentials`/`nats status` to kscorectl
  - Added `audit search` (--type, --status, --agent, --user, --since, --count-by), `audit analyze` (--input, --baseline), `audit timeline` (--from, --to, HTML output) to kscore-audit
  - Added `secrets rotate-keys` (--force) to kscore-secrets

### Project Docs Drift

- [x] **Incident response project doc references unsupported commands** ✅ Done
  - Updated all CLI commands in INCIDENT-RESPONSE.md to match actual implementations
  - `agent` → `agents` (plural), `files download` → `exec run "scp ..."`, `api-key revoke-all` → `auth revoke-all --force`
  - `agent certificate rotate` → `agents renew-svid --force`, `agent certificate regenerate --scope` → `agents certificates regenerate --all`
  - `audit query` → `audit search`, `policy audit list` → `audit search --type "policy.*"`
  - `auth session invalidate-all` → `auth sessions invalidate`, `module block/audit` → `module verify` + `audit search`
  - Removed `user reset-password` (delegated to external IdP), added `secrets rotate-keys`

### Runbooks and Operations Docs Drift

- [x] **Runbooks/operations docs reference missing `kscorectl debug` commands** ✅ Done
  - Only `docs/content/en/docs/reference/nats-mesh.md` had `kscorectl debug` references (runbooks/operations/concepts had none)
  - Replaced 6 debug commands with existing equivalents: `nats status`, `events list/export`, `audit timeline`, `diagnostics collect`

### Epics Documentation Drift

- [x] **Epic 36 secrets management doc lists many CLI commands not implemented** *(DONE - dynamic/leases/transit/template/cache)*
  - Added `dynamic list/get/revoke`, `leases list/revoke/renew`, `encrypt`, `decrypt`, `rewrap`, `template`, `cache status/clear/list` subcommands
  - Note: `secrets backends` and `secrets audit` were added in a previous commit

- [x] **Epic 09 plugin system doc references missing `kscorectl plugin` commands and module features** ✅ Done
  - Replaced `kscorectl plugin publish/install/verify/list` with existing `module publish/install/verify` and `--help` auto-discovery
  - Added `module update`, `module mirror`, and `module clean` subcommands to kscore-module

- [x] **Epic 03 state management doc references missing CLI commands** *(DONE)*
  - Added `compile` (with --vars, --vars-file, --output), `vars get/list`, `export`, `restore`, and `drift --fix` to kscore-state

- [x] **Epic 37 runbooks doc references `kscorectl runbook` subcommands not implemented** *(DONE - core commands)*
  - Added `list`, `execute`, `status`, `list-executions`, `audit`, `test` subcommands
  - Note: `show`, `delete`, `versions`, `rollback` still need implementation

- [x] **Epic 02 remote execution doc uses missing top-level exec/file/job commands and flags** *(DONE - doc fix)*
  - Updated US2.3 script execution examples to use `exec run`/`exec script` subcommands
  - Updated US2.4 file transfer examples to use `exec script` and `files put`
  - Updated US2.6 job management to use `exec list/status/cancel/output`
  - Updated T5.1 CLI commands list to match actual kscore-exec subcommands

- [x] **Epic 04 event system doc uses missing events subcommands** *(DONE)*
  - Added `subscribe` (with --filter-type, --filter-severity, --output) and `export` (with --format json/csv, --start, --end, --type, --limit) subcommands

- [x] **Epic 06 policy enforcement doc references policy/compliance commands not implemented** *(DONE)*
  - Added `eval`, `test`, `schedule create/list/delete`, `remediate`, `monitor`, and `compliance report` subcommands

- [x] **Epic 07 observability doc references missing debug profiling command** *(DONE - doc fix)*
  - Replaced `kscorectl debug profile` with curl-based pprof access, consistent with all other profiling docs

- [x] **Epic 08 multi-environment doc references missing inventory/discover/import/container/istio commands** *(DONE - doc fix)*
  - `inventory` → `agents list --labels`; `discover` → `proxy discover scan`; `import` → agent self-registration
  - `container restart/logs/pull/prune` → `exec run "docker ..."` on targets
  - `istio inject/traffic-shift` → `exec run "istioctl ..."` on targets

- [x] **Epic 22 file distribution doc references missing CLI operations** *(DONE - already implemented)*
  - All referenced commands already exist: `files list/get/put/delete/info/sync`, `mirrors list/show/sync-status/sync/health/failover/latency/conflicts/resolve-conflict/history`, plus `cache` and `namespace` groups
  - TODO description was outdated; `cmd/kscore-files` has full implementation

- [x] **Epic 23 self-management doc references missing self/backup/upgrade commands** *(DONE - doc fix)*
  - `backup download` → `backup show` (shows details/path)
  - `upgrade apply` → `upgrade execute --target`
  - `self status/health/drift/apply/export` → `diagnostics collect`, `state check/apply`, `backup create`

- [x] **Epic 25 blueprints doc references missing bundle/mirror/migrate/status commands** *(DONE - doc fix)*
  - `blueprint list` → `blueprint applied list`; `blueprint status` → `blueprint applied show`
  - `blueprint bundle <path>` → `bundle create`; `bundle-install` → `bundle install`; `bundle-verify` → `verify`
  - `mirror init/sync --with-deps` → removed; `kscorectl mirror` → `blueprint mirror` subcommands
  - Removed `blueprint migrate` (no equivalent; use `blueprint install <name>@<version>`)

- [x] **Epic 38 air-gapped deployments doc references missing airgap CLI** *(DONE - doc fix)*
  - Replaced 32 `kscorectl airgap` references with existing commands
  - Package creation → `blueprint bundle create`, `module mirror`, `upgrade plan`
  - Installation → `blueprint verify/bundle install`, `bootstrap seed`
  - Registry → `blueprint mirror import/serve`, `module mirror --import`
  - Upgrade → `upgrade check/execute/rollback`
  - Export/import → `audit export`, `backup create/restore`
  - Removed validation/sync sections (not needed)

### Reference Docs Drift

- [x] **Runbook automation reference lists unsupported runbook commands** *(DONE)*
  - Added `runbook list`, `execute`, `status`, `list-executions`, `audit`, and `test` subcommands to `cmd/kscore-runbook`
  - Updated runbook-automation.md reference docs

- [x] **NATS mesh reference lists missing debug commands** *(DONE - already fixed by TODO line 185)*
  - All `kscorectl debug nats` references replaced with `nats status`, `events list/export`, `audit timeline`, `diagnostics collect`

- [x] **Blueprint reference docs include bundle/mirror/registry commands not implemented** *(DONE - bundle/mirror)*
  - Added `blueprint bundle create/install` and `blueprint mirror add/remove/list/sync/status` subcommands
  - Note: `blueprint registry add` still needs implementation
### Tutorials and Scenarios Documentation CLI Drift

- [x] **Event-driven automation scenario references missing reactor/trace commands** *(DONE - doc fix)*
  - `event emit` → `events emit` (plural); `events watch --filter` → `--type`
  - `reactors list/status/disable/history` → `events list --type "reactor.*"`
  - `reactor test/debug` → `events emit` + `events watch`; removed rate-limit-status
  - `events trace` → `events list --correlation-id`; `queue-status` → `storage-stats`
  - `dlq replay` → `dlq retry`

- [x] **Scenario docs reference missing environment/promote/approve/rollback commands** *(DONE - doc fix)*
  - `environment status/compare/sync` → `agents list --labels`, `state diff/check/apply`
  - `promote --state --version` → `gitops promote --pipeline --from --to --revision`
  - `promote list/status` → `gitops status`
  - `approve list/status/remind` → `runbook approvals/approve`
  - `rollback --deployment --target` → `gitops rollback --app --strategy`
  - `metrics query` → `curl` against Prometheus API

- [x] **Scenario docs reference missing ping/connectivity commands** *(DONE - doc fix)*
  - Only hybrid-infrastructure.md had a reference (other 2 files had none)
  - `connectivity test --from --to` → `agents verify --all`

- [x] **Scenario docs reference missing state export/restore commands** *(DONE - already implemented)*
  - `state export` and `state restore` both exist in `cmd/kscore-state/main.go`
  - TODO description was outdated

- [x] **Control plane config docs include settings not in config structs** *(DONE - moved to Epic 45)*
  - Removed unsupported `agents`, `execution`, `state`, `events` config sections from configuration.md
  - Implementation deferred to `epics/45-control-plane-config-wiring.md`

- [x] **GitOps and security config sections are not implemented** *(DONE - moved to Epic 45)*
  - Removed unsupported `gitops` and `security` config sections from configuration.md
  - Implementation deferred to `epics/45-control-plane-config-wiring.md`

- [x] **`kscorectl config set` is referenced in docs but not implemented** *(DONE - already implemented)*
  - `config set` and `config show` both exist in `cmd/kscorectl/main.go`
  - Implemented as part of earlier runbook CLI work (TODO line 150)

### SDK Documentation vs Implementation

- [x] **SDK docs reference gRPC client code that is not generated** *(DONE - docs fixed)*
  - Updated gRPC example to use `controlplane_pb2_grpc.ControlPlaneServiceStub` instead of `event_service_pb2_grpc.EventServiceStub`
  - Fixed proto file listing to show actual filenames (`agent.proto`, `controlplane.proto`, `coordination.proto`) with notes on generation status

- [x] **SDK examples target REST endpoints that don't exist** *(DONE - docs fixed)*
  - Replaced SSE `EventStream` class (using non-existent `/api/v1/events/stream`) with REST `EventPoller` class using `GET /api/v1/events`
  - Updated Python and Go examples to use polling pattern matching actual API

### API Field/Endpoint Mismatches

- [x] **REST API handlers are not wired into kscore-server** *(DONE)*
  - Wired agents, execution, state, maintenance, and apikeys handlers into kscore-server HTTP mux
  - Remaining handlers (events, policy, gitops, cluster, webhooks, runbook) need their dependencies instantiated first

- [x] **OpenAPI spec paths do not match implemented API** *(DONE - spec fixed)*
  - Fixed `/api/agents` → `/api/v1/agents`, `/events` → `/api/v1/events`, `/webhooks` → `/api/v1/webhooks`
  - Removed non-existent endpoints (`/api/topology`, `/api/graph`, `/events/batch`)
  - Added missing spec entries for wired handlers: execution, state, maintenance, apikeys
  - Updated Agent/AgentList schemas to match actual handler response fields
  - Added notes on not-yet-wired handlers (cluster, events, webhooks, mirrors, discovery)
  - Added `/api/v1/webhooks/config` endpoint to spec

- [x] **Add missing agent proto fields** (or update docs) *(DONE - docs fixed)*
  - Updated api.md to match actual proto/handler fields (no `datacenter/environment/role/connected_at`)
  - Proto uses `labels` map for flexible metadata; handler returns flat fields

- [x] **Agents REST API docs do not match handler filtering/response fields** *(DONE - docs fixed)*
  - Updated List Agents: query params now show `status`, `labels`, `sort`; response shows actual fields with `total/online/offline/retrieved_at`
  - Updated Get Agent: response shows flat fields matching `AgentResponse` struct
  - Fixed filtering examples, Go/Python/TypeScript client examples, and migration examples

- [x] **Webhook REST API docs do not match implementation** *(DONE - docs fixed)*
  - Replaced outbound webhook docs with actual inbound webhook receiver docs
  - Documented POST /api/v1/webhooks (receive), GET /stats, GET /config with actual response schemas
  - Added note that handler is not yet wired into kscore-server

- [x] **Webhook endpoint/path in docs does not match runtime server** *(DONE - docs fixed)*
  - Documented that webhooks handler exists but is not yet wired into kscore-server
  - Config response shows separate addr/path fields for the webhook receiver

- [x] **Rate limiting documentation does not match code**
  - Resolution: both
  - Added `X-RateLimit-Reset` header (Unix timestamp) to rate limit middleware
  - Changed 429 response from plain text to JSON via `apierror.Write` (`{"error":"rate_limit_exceeded","message":"Rate limit exceeded"}`)
  - Updated docs: configurable limits (not hardcoded), key extraction strategies, `Retry-After` header, response header table
  - Reference: `cmd/kscore-server/main.go`, `docs/content/en/docs/reference/api.md`

- [x] **API rate_limit config is documented but not wired**
  - Resolution: code
  - Wired `internal/ratelimit.TokenBucket` into `kscore-server` HTTP handler via `rateLimitMiddleware`
  - Extracts client key by ip/apikey/header per `cfg.RateLimit.KeyExtractor`
  - Returns 429 with Retry-After header; sets X-RateLimit-Limit/Remaining headers
  - Reference: `cmd/kscore-server/main.go`

- [x] **API CORS config is documented but not applied**
  - Resolution: code
  - Added `corsMiddleware` to `kscore-server` HTTP handler chain
  - Handles preflight OPTIONS (Allow-Methods, Allow-Headers, Max-Age) and actual request headers (Allow-Origin, Allow-Credentials)
  - Supports wildcard and specific origin allowlists, Vary header for non-wildcard
  - CORS wraps outermost so preflight responses bypass rate limiting
  - Reference: `cmd/kscore-server/main.go`

- [x] **API TLS settings are documented but not wired into server**
  - Resolution: code
  - Added `buildTLSConfig` that loads cert/key, optional CA for mTLS client auth, sets min version
  - gRPC server uses `grpc.Creds(credentials.NewTLS(...))` when TLS enabled
  - HTTP server uses `tls.NewListener` wrapping existing listeners when TLS enabled
  - Reference: `cmd/kscore-server/main.go`

- [x] **Metrics config is documented but not wired into control plane**
  - Resolution: code
  - `kscore-server` initializes `PrometheusCollector`, registers Go/process collectors per config
  - Serves Prometheus metrics at `cfg.Metrics.Path` (default `/metrics`) on main HTTP mux
  - Reference: `cmd/kscore-server/main.go`

- [x] **Tracing config is documented but not wired into control plane**
  - Resolution: code
  - `kscore-server` initializes `tracing.NewProvider()` when `tracing.enabled=true`
  - Maps `config.TracingConfig` to `tracing.Config` (exporter type, endpoint, sampling, resource attrs)
  - Supports OTLP, Zipkin, stdout exporters; always/never/ratio/parent_based sampling
  - Provider shutdown deferred with 5s timeout
  - Reference: `cmd/kscore-server/main.go`

- [x] **Health config is documented but not wired into control plane**
  - Resolution: code
  - `kscore-server` health endpoints now use `cfg.Health` settings
  - `/health/ready` checks NATS and database per `Checks.NATS.Enabled`/`Checks.Database.Enabled`
  - `StartupGracePeriod` reports healthy during grace window even if checks fail
  - Per-check `Timeout` applied via context; disabled checks are skipped
  - Reference: `cmd/kscore-server/main.go`

- [x] **Health endpoint responses do not match docs**
  - Resolution: code
  - `/health/ready` now checks NATS + database (both configurable)
  - `/health/status` returns component-level health matching docs: `{status, components: {nats: {status, latency_ms}, database: {status, latency_ms}, agent_pool: {status, agents}}, uptime_seconds}`
  - Agent pool health uses `MinHealthy` threshold from config
  - 11 new tests covering all check pass/fail combinations, grace period, and disabled checks
  - Reference: `cmd/kscore-server/main.go`

- [x] **Pagination section in API docs conflicts with implementation**
  - Resolution: docs
  - Docs incorrectly said "cursor-based" but showed offset examples; REST handlers use `limit`/`offset`
  - Split pagination docs into REST (offset-based) and gRPC (cursor-based) sections
  - Updated response example to match actual handler shape: `total`, `limit`, `offset`, `retrieved_at`
  - Reference: `docs/content/en/docs/reference/api.md`

- [x] **REST API error response format differs from docs**
  - Resolution: code
  - Created shared `pkg/api/apierror/` package with `Response` struct (`error`, `message`, `details`) and `Write()` helper
  - `StatusCode()` maps HTTP status to error codes: 400→`invalid_request`, 404→`not_found`, 409→`conflict`, etc.
  - Updated `writeError` in all 11 handler packages to delegate to `apierror.Write`
  - Updated test assertions in 9 test files to check error code and message separately
  - Reference: `pkg/api/apierror/error.go`, `pkg/api/*/handlers.go`

- [x] **REST auth model in docs does not match implementation**
  - Resolution: both
  - Added `httpAuthMiddleware` that reuses existing `auth.InterceptorConfig` authenticators for HTTP requests
  - Extracts credentials from `Authorization: Bearer <token>` or `X-API-Key` header (configurable)
  - Health/status endpoints bypass auth; mTLS client certs supported via `r.TLS.PeerCertificates`
  - Returns 401 JSON via `apierror.Write` on failure; stores `Principal` in request context
  - Updated docs: listed unauthenticated endpoints, documented both header formats
  - 7 new tests covering valid/invalid/missing credentials, health bypass, custom header, principal context
  - Reference: `cmd/kscore-server/main.go`, `docs/content/en/docs/reference/api.md`

- [x] **gRPC package names in API docs do not match proto**
  - Resolution: already fixed
  - Docs already show correct `package keystone.core.v1;` matching all 7 proto files
  - No `kscore.api.v1` or `kscore.api.v2` references remain in docs
  - Reference: `docs/content/en/docs/reference/api.md` line 2300

- [x] **gRPC services documented but not generated/implemented**
  - Resolution: deferred to Epic 46 (`epics/46-grpc-service-implementation.md`)
  - Created epic covering proto generation, server implementation, and registration for all 7 services
  - Added status annotations to each gRPC service section in API docs

- [x] **gRPC services exist but are not registered in server**
  - Resolution: deferred to Epic 46 (`epics/46-grpc-service-implementation.md`)
  - Epic covers registering AgentService and CoordinationService in kscore-server (Phase 1)

- [x] **Docs reference REST endpoints that do not exist**
  - Resolution: docs
  - Fixed `/api/v1/status` → `/api/status` in proxy-agents.md
  - Fixed `/api/v1/events/stats` → `/api/status` in support.md (events handler not wired)
  - Fixed `/api/v1/agents/{id}/execute` → `/api/v1/exec` in api.md versioning example
  - Fixed changelog deprecation note to reference `/api/status` instead of `/api/v1/health`
  - Fixed `/api/v1/health` → `/health/ready` in secrets-rotation.md
  - Removed `/api/v1/health/secrets` section from secrets-api.md (no handler exists)
  - `/api/v1/exclusions` in windows.md is Trend Micro Apex One API, not Keystone (no change needed)
  - `/api/v1/agents/{id}/metrics` already exists (no change needed)

### Blueprint Documentation

- [x] **Fix blueprint version references**
  - Resolution: docs
  - Fixed all 14 blueprint version references in blueprints-catalog.md to `@0.1.0`
  - Corrected: demo, production-cluster, enterprise-platform, nats-cluster, postgres-ha, monitoring-stack, metrics-only, security-baseline, identity-federation, gitops-integration, proxy-agents, file-distribution, kubernetes-operator, edge-deployment

- [x] **Update kscore/demo parameters**
  - Resolution: both
  - Added `api_port`, `metrics_port`, `data_dir`, `log_level` parameters to blueprint.yaml
  - Added `hostname`, `enable_examples`, `enable_dashboards` to docs parameter table
  - Removed non-existent features table (`sample_agents`, `sample_states`, `web_ui`)
  - Updated usage example to use actual parameter names

- [x] **Update kscore/production-cluster parameters**
  - Resolution: docs
  - Rewrote parameter table to match all 17 actual blueprint.yaml parameters
  - Fixed defaults: `cluster_name` (keystone), `postgres_database`/`postgres_user` (keystone), `nats_urls` (optional)
  - Replaced `node_count` with actual `node_role` + `control_plane_nodes` parameters
  - Fixed `tls_mode` enum: generate/provided/letsencrypt (not auto/manual/disabled)
  - Replaced features: `etcd_clustering`/`auto_scaling` → `nats_cluster`/`postgres_ha`/`monitoring`/`security`/`gitops`
  - Updated usage example with correct parameter and feature names

### Operations Documentation Gaps

- [x] **Registry ops docs imply object storage backends not supported by code**
  - Resolution: deferred to Epic 47 (`epics/47-registry-storage-backends.md`)
  - Created epic to implement pluggable storage backends (S3, GCS, Azure, NATS) for kscore-registry
  - Reuses existing backend infrastructure from `internal/files/backend/`

- [x] **API key CLI points to REST endpoints that are not implemented** *(DONE)*
  - Implemented `pkg/api/apikeys/` package with create/list/revoke handlers
  - Wired into kscore-server HTTP mux

- [x] **NATS mesh operations doc references missing commands/flags**
  - Resolution: already fixed
  - `nats-mesh-operations.md` no longer contains the missing command references
  - Remaining references in `nats-mesh.md` and runbooks covered by separate TODO items (lines 607+)

- [x] **Proxy agents docs reference a proxy agent binary/flag that doesn't exist**
  - Resolution: docs
  - Removed non-existent `--proxy` flag from `kscore-agent` startup and K8s deployment args
  - Renamed `kscore-proxy-agent` to `kscore-agent-proxy` in K8s deployment/labels/serviceAccount
  - Fixed kubectl exec pod name references

- [x] **File distribution ops docs list backend types not supported by kscore-files**
  - Resolution: already fixed
  - `createBackend` in `cmd/kscore-files/main.go` supports all 6 types: filesystem, s3, gcs, azure, git, nats
  - Fixed as part of TODO line 87 (file backends config schema)

- [x] **File distribution ops docs reference missing backend maintenance commands**
  - Resolution: docs
  - Replaced `backend gc` section with `backend health` + `backend status`
  - Replaced `backend test` with `backend health`
  - Replaced `backend benchmark` with `backend health`

- [x] **File distribution ops docs use unsupported mirrors flags**
  - Resolution: docs
  - Fixed `sync-status --group <id>` → `sync-status <id>` (positional arg)
  - Fixed `sync --group <id> --force` → `sync <id> --dry-run` (positional arg, no --force)
  - Fixed `conflicts --id <id>` → `resolve-conflict <id> --dry-run`
  - Replaced non-existent `errors`/`ping` commands with `history`/`health`
  - Removed `--verbose` flag (doesn't exist)

- [x] **Cluster management docs use non-existent exec limit flag**
  - Resolution: docs
  - Removed non-existent `--limit 3` flag from `exec run` command in cluster-management.md

- [x] **Deployment docs reference a missing state list command**
  - Resolution: docs
  - Replaced `kscorectl state list` with `kscorectl state history` in deployment.md

- [x] **File backends reference does not match kscore-files config schema** *(DONE — see line 87)*

- [x] **Secrets operations docs reference non-existent kscore-secrets subcommands**
  - Resolution: already fixed
  - Docs correctly note kscore-secrets focuses on rotation orchestration
  - All commands referenced in docs (rotate, get, list, backends, audit) now exist
  - Added in earlier TODOs (lines 145, 191): get, list, backends, audit, dynamic, leases, encrypt, decrypt, rewrap, template, cache, rotate-keys

### Tutorials/Concepts/Scenarios Documentation Gaps

- [x] **Secrets quickstart tutorial uses commands not implemented in kscore-secrets/exec**
  - Resolution: both
  - Uses `kscorectl secrets get/put/list/audit/stats/injection/refresh` and `secrets rotation` subcommands
  - `kscore-secrets` only implements `rotate`, `schedule`, and `policy` (no get/put/list/audit/stats/injection/refresh; no `rotation` command alias)
  - Uses `kscorectl exec myapp -- ...` (no `exec` subcommand; should be `exec run`)
  - Update tutorial or implement missing commands
  - Reference: `docs/content/en/docs/tutorials/secrets-quickstart.md`, `cmd/kscore-secrets/main.go`, `cmd/kscore-exec/main.go`
  - DONE: Updated tutorial to use implemented commands: secrets backends/get/list/audit/rotate/cache/policy, exec run --target, and vault kv put for secret storage

- [x] **Events concept docs reference non-existent command names/subcommands**
  - Resolution: both
  - Docs use `kscorectl event ...` (plugin is `kscore-events`, invoked as `kscorectl events`)
  - Commands like `event analyze`, `event subscribers`, `event storage-stats`, `event prune`, `event archive` are not implemented
  - Flags like `--until` and `--show-sequence`, and multi-value `--severity` lists are not supported (CLI uses `--before` and a single min severity)
  - Update docs or add commands/aliases
  - Reference: `docs/content/en/docs/concepts/events.md`, `cmd/kscore-events/main.go`
  - DONE: Fixed --until now → --since/--before, --severity list → single min severity, removed --target flag from replay, replaced with --dry-run example. Commands (analyze, subscribers, storage-stats, prune, archive) already exist in CLI.

- [x] **GitOps concept docs reference commands not in kscore-gitops**
  - Resolution: code
  - Docs use `kscorectl git-sync ...` (22+ references), `kscorectl verify ...`, `kscorectl logs ...`, top-level `rollback`/`promote`, and `approvals` commands
  - `kscore-gitops` does not implement `git-sync` or a top-level `verify`/`logs` command; rollbacks/promotions live under `kscorectl gitops ...`
  - Approvals are under `kscorectl runbook approvals`
  - Requires implementing git-sync subcommands or removing ~500 lines of conflict resolution documentation
  - Reference: `docs/content/en/docs/concepts/gitops.md`, `cmd/kscore-gitops/main.go`, `cmd/kscore-runbook/main.go`
  - DONE: Fixed 22 git-sync refs (added gitops prefix), fixed concatenated subcommands (rollbacklist/approve/reject, promotemyapp, verify run/logs, rollbackpolicy/triggers/execute) to use actual CLI flags

- [x] **Compliance scenario references a CLI that does not exist** *(DONE - doc fix)*
  - Resolution: doc
  - Replaced all `kscorectl compliance ...` commands with existing `kscorectl policy ...` equivalents
  - `compliance report` → `policy compliance report`, `compliance scan` → `policy eval`, `compliance status` → `policy compliance`
  - `compliance check --control` → `policy check <file> --policy`, `compliance remediation-history` → `policy violations`
  - `kscorectl logs --reactor` → `kscorectl events list --type "reactor.*"`
  - `policy evaluate --framework --agent` → `policy eval <name> <target>`
  - Reference: `docs/content/en/docs/scenarios/compliance-automation.md`

- [x] **Blueprint apply commands are documented but not implemented**
  - Resolution: code (changed from doc - requires implementation of apply command with --var and --target support)
  - Docs use `kscorectl blueprint apply ...` with variables and targeting in scenarios and concepts
  - `kscore-blueprint` has no `apply` subcommand (install only downloads locally)
  - Need to implement apply command that deploys blueprint to targets with variable substitution
  - Reference: `docs/content/en/docs/scenarios/_index.md`, `docs/content/en/docs/scenarios/database-ha.md`, `docs/content/en/docs/concepts/blueprints.md`, `cmd/kscore-blueprint/main.go`

### Runbook Documentation Gaps

- [x] **Runbooks reference many CLI commands that don't exist** *(DONE)*
  - Resolution: doc (most commands were implemented in earlier TODOs; remaining refs fixed in docs)
  - Most listed commands now exist: auth login/revoke-all/sessions/rotate-signing-key, cluster election restart/member remove/undrain, federation status/ping/trust list, audit search/analyze/timeline, diagnostics collect, config set, db compact/rotate-credentials, nats rotate-credentials, secrets rotate-keys, upgrade resume/path with --retry/--skip
  - Fixed `user delete` → IdP delegation note + `auth key revoke` (security-incident.md)
  - Fixed `backup status` → `backup list` (bootstrap-new-cluster.md)
  - Fixed `kscore-bootstrap init` → `kscore-bootstrap seed` (capacity-scaling.md)
  - Fixed `--show-version` → `--show-compatibility` (troubleshooting.md, upgrade-cluster.md)
  - Fixed `state update --from-actual` → `state check` (troubleshooting.md)
  - Fixed `agent` → `agents` (plural) for modified lines

### Reference/Community Documentation Gaps

- [x] **NATS mesh reference doc calls missing debug commands** *(DONE - already fixed)*
  - Docs already use correct commands: `nats status`, `events list/export`, `audit timeline`, `diagnostics collect`
  - No `kscorectl debug nats` references remain in nats-mesh.md reference or concept docs

- [x] **NATS mesh reference config does not match config structs** *(DONE)*
  - Resolution: doc
  - Replaced `server.nats.*`/`agent.nats.*` config blocks with actual `nats:` top-level config matching NATSConfig/NATSEmbeddedConfig/JetStreamConfig structs
  - Removed unsupported: gateway, websocket, routing, endpoints, buffer, discovery, circuit_breaker, delivery, dedup, degradation sections
  - Added undocumented fields: token, credential
  - Fixed env vars to match bootstrap code (KSCORE_NATS_MODE/URLS/CREDS_FILE/USER/PASSWORD)
  - Fixed CLI section to match actual kscore-agent nats subcommands (removed leaf/buffer commands)
  - Fixed metrics labels to match observability.go (error_type→error, type→subject_prefix, etc.)

- [x] **CLI quick reference lists module commands that do not exist** *(DONE)*
  - Resolution: doc
  - Removed `module list`, `module show`, `module uninstall` from quick reference (not implemented)
  - `module update` already exists; `test suite list` already exists (TODO description was partially outdated)

- [x] **Runbook automation reference doc lists commands not in kscore-runbook** *(DONE - already implemented)*
  - All referenced commands exist: list, execute, status, list-executions, audit, test, approvals, interventions

- [x] **FAQ uses unsupported state apply verbosity flag** *(DONE)*
  - Resolution: doc
  - Removed `-v` flag from `state apply` example in faq.md

### Makefile Target Documentation

- [x] **Document cross-platform build targets** *(DONE)*
  - Added `build-linux`, `build-darwin`, `build-windows`, `build-all-platforms` to development.md

- [x] **Document repository generation targets** *(DONE)*
  - Added `repos`, `repo-gen`, `repos-dnf`, `repos-apt`, `repos-windows`, `repos-blueprints`, `repos-modules` to development.md

---

## Low Priority (Enhancements)

### Missing Makefile Convenience Targets

- [x] `make fmt` - Run gofmt *(DONE)*
- [x] `make lint-fix` - Run golangci-lint --fix *(DONE)*
- [x] `make check` - Pre-commit checks (fmt, lint, test) *(DONE)*
- [x] `make dev` - Hot reload development with air *(DONE)*
- [x] `make test-verbose` - Verbose test output (alias for test) *(DONE)*
- [x] `make test-coverage` - Generate coverage reports *(DONE)*
- [x] `make test-integration` - Run integration tests *(DONE)*
- [x] `make benchmark` - Run benchmarks *(DONE)*

### CLI Command Coverage

- [x] **Implement --format flag for events watch command** *(DONE)*
  - Added `--format` flag (text/json/jsonl) to watch command
  - Added `--filter` and `--tag` flags to cli.md docs (already existed in code)
  - 4 new tests covering all formats and invalid format error

- [x] **Add missing events query flags** *(DONE)*
  - Added `--since` and `--until` time range flags to query command
  - Added `-n` shorthand alias for `--limit`

- [x] **Implement missing events list filters** *(DONE)*
  - Added severity filtering with level ordering (debug < info < warning < error < critical)
  - Added time range filtering (--since, --before) with RFC3339 and duration parsing
  - Tag filtering skipped — EventDisplay has no Tags field

- [x] **Implement missing events DLQ flags** *(DONE)*
  - Added `--reason` filter and `-n` shorthand for `--limit` on `dlq list`
  - Added `--type` flag on `dlq retry` to retry events by type
  - Added `--older-than` and `--reason` flags on `dlq purge`
  - 11 new tests covering all flags and error paths

- [x] **Implement missing upgrade command features** *(DONE)*
  - Added `[upgrade-id]` positional arg to `cancel` and `logs` (docs don't show one for `status`)
  - Added `--confirm` flag to `canary promote` and `canary rollback`
  - Added `--status` filter to `history` command
  - `agents list`/`agents status` subcommands not needed — docs use `--report`/`--status` flags (already exist)
  - 12 new tests

- [x] **Implement cluster members --filter flag** *(NOT NEEDED)*
  - Docs do not show `--filter` on `cluster members` — only `--details` and `--output`, both already exist

- [x] **Implement cluster rebalance --target flag** *(NOT NEEDED)*
  - Docs only show `--dry-run` and `--reason`, both already exist in code; no `--target` in docs

- [x] **Implement proxy discover scan flags** *(DONE)*
  - Added `--subnet` (alias for `--network`), `--protocols`, `--ports`, `--timeout`, `--workers`
  - Updated docs to include `--network`, `--networks`, `--debug` alongside existing flags
  - 8 new tests covering all flags, mutual exclusion, and error paths

- [x] **Implement proxy drift/state missing features** *(DONE)*
  - Added `state check <state-file>` command with `--device` and `--target` flags
  - `drift check --severity` skipped — docs don't show it (TODO was inaccurate)
  - 5 new tests

### File Distribution Docs vs CLI

- [x] **Align file distribution docs with actual files CLI** *(DONE)*
  - Rewrote CLI examples in file-distribution.md to use `--namespace` flag instead of `namespace/path` syntax
  - Updated namespace creation to match actual `--backend`/`--path` flags
  - Added `sync` and `--recursive` examples matching actual CLI

- [x] **File server configuration docs include unsupported sections** *(DONE)*
  - Removed unsupported `rate_limit`, `access`, `mirror_groups`, `cache` sections
  - Added missing `nats` connection settings (url, token, username, password, TLS)
  - Rewrote `backend` to `backends` array format matching actual `BackendConfig` struct

- [x] **Proxy agent configuration docs include unsupported discovery/profiles** *(DONE)*
  - Implemented `EnvCredentialProvider` for env-based credentials (useful for dev/CI)
  - Added `Env` config field to `CredentialsConfig`
  - Removed aspirational `discovery` and `profiles` config sections from docs
  - Added env provider documentation with variable naming convention
  - 9 new tests

- [x] **Fix file backend option names in docs** *(DONE — see line 87)*

- [x] **HTTP file backend is declared but not implemented or documented**
  - Resolution: code
  - Implemented `HTTPBackend` in `internal/files/backend/http.go` (read-only, HTTP GET/HEAD)
  - Added `HTTPConfig` with base_url, timeout, headers, auth (basic/bearer), TLS skip-verify
  - Supports conditional GET (ETag), range requests, health checks
  - 22 new tests in `internal/files/backend/http_test.go`
  - Documented in `docs/content/en/docs/reference/file-backends.md`

### Loadtest/Test CLI Docs vs Code

- [x] **Align kscore-test flags and suite commands** *(DONE — doc fix)*
  - Resolution: doc (TODO description was inaccurate — `suite list` already existed in code)
  - Added flag tables for all `test` subcommands documenting `--tags`, `--parallel`, `--fail-fast`, `--type`, `--status`
  - Added `suite create`, `suite delete`, `history` to docs (already in code but undocumented)
  - Updated quick reference with `history`, `suite show`, `suite create`, `suite delete`

### Exec CLI Docs vs Code

- [x] **Document missing kscore-exec commands and flags**
  - Resolution: both
  - Docs cover run/status/list/shell/script only
  - Code also includes `async`, `cancel`, `history`, and `output` subcommands
  - Docs omit run flag `--dry-run` and list/history filters (`--since`, `--before`, etc.)
  - TLS flags in code (`--tls`, `--tls-ca-cert`, `--tls-cert`, `--tls-key`, `--tls-server-name`) are not documented
  - Update docs or remove/alias commands
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-exec/main.go`

### Non-CLI Docs Referencing Missing Commands

- [x] **Runbooks reference many missing auth/cluster/config/diagnostics/certs/agent commands**
  - Resolution: doc
  - Examples: `kscorectl auth login`, `cluster token/quorum/health/uncordon`, `config set`, `diagnostics collect`, `certs *`, `agent quarantine/unquarantine/verify/ping`
  - Several runbooks rely on `audit search/export/analyze/timeline` and `security scan` commands not in CLI
  - Runbooks reference `backup status` and agent list flags like `--show-version`, `--show-cert-expiry`, `--count`, `--suspicious`
  - Update runbooks or implement commands
  - Reference: `docs/runbooks/*`

- [x] **Concepts policy docs reference missing policy subcommands**
  - Resolution: doc
  - Docs use `policy compliance/violations/import/evidence/export/bindings/test/evaluate`
  - CLI only supports list/validate/check/show/create/update/delete/activate/deactivate/audit/report
  - Note: `compliance` and `violations` ARE now implemented - remove from this list
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/concepts/policy.md`
  - **PLANNED FIX** (awaiting approval):
    - Lines 1782-1791: Replace `policy import <url>` with `policy create <file>` (download first)
    - Lines 1846-1850: Remove `policy evidence` command (no equivalent)
    - Lines 1853-1856: Replace `policy export` with `policy report --format csv`
    - Line 1871: Remove `policy bindings` (bindings defined in YAML, not CLI)
    - Lines 1874, 1884: Replace `policy test` with `policy check <file>`
    - Line 1900: Replace `policy evaluate --all` with `policy compliance` to refresh data

- [x] **Concepts reactors docs reference missing reactor/logs commands**
  - Resolution: doc
  - Docs use `kscorectl reactor list/status/stats/history` and `kscorectl logs --filter`
  - No reactor or logs CLI exists
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/concepts/reactors.md`

- [x] **Concepts GitOps docs reference missing git-sync/rollback/verify/approvals commands**
  - Resolution: doc
  - Docs use `kscorectl rollback *`, `kscorectl promote`, `kscorectl verify *`, `kscorectl approvals *`
  - Docs use `kscorectl git-sync *` subcommands and `kscorectl logs --filter`
  - Docs use `kscorectl config validate` (no config CLI)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/concepts/gitops.md`

- [x] **Concepts identity docs reference missing identity/agent commands**
  - Resolution: doc
  - Docs use `identity ca backups list`, `identity metrics`, `identity cache stats`, and `identity events --follow` (not implemented)
  - Docs use `agent renew-svid` (not implemented)
  - Update docs or implement commands/flags
  - Reference: `docs/content/en/docs/concepts/identity.md`

- [x] **Concepts secrets management docs reference missing secrets compliance command**
  - Resolution: doc
  - Docs use `kscorectl secrets compliance report` (not implemented)
  - Update docs or implement command
  - Reference: `docs/content/en/docs/concepts/secrets-management.md`

- [x] **Concepts service mesh docs reference missing agent/identity commands**
  - Resolution: doc
  - Docs use `kscorectl agents get` (CLI uses `agents show`)
  - Docs use `kscorectl identity federation list` (federation is a separate plugin: `kscorectl federation list`)
  - Update docs or implement aliases
  - Reference: `docs/content/en/docs/concepts/service-mesh.md`

- [x] **Concepts observability docs reference missing logs command**
  - Resolution: doc
  - Docs use `kscorectl logs ...` for correlation/tail queries
  - No logs CLI exists
  - Update docs or implement logs plugin
  - Reference: `docs/content/en/docs/concepts/observability.md`

- [x] **Concepts observability docs import non-existent query package**
  - Resolution: doc
  - Docs import `github.com/shawnbutts/keystone-core/pkg/query`
  - Code is `internal/query` only
  - Update docs or promote package
  - Reference: `docs/content/en/docs/concepts/observability.md`

- [x] **Concepts state storage docs reference missing migrate/backup flags**
  - Resolution: doc
  - Docs use `kscorectl cluster-backup create --type database` (CLI uses `cluster-backup backup` and no `--type` flag)
  - Docs use `migrate run --resume` and `--skip-errors/--log-errors` (CLI uses `--continue-on-error` and no log/errors flags)
  - Update docs or implement flags
  - Reference: `docs/content/en/docs/concepts/state-storage.md`

- [x] **Community docs reference missing commands**
  - Resolution: none needed — TODO claims were inaccurate
  - Docs correctly use `bootstrap seed`, `blueprint install`, `state apply`, `module tree`, etc.
  - None of the claimed missing commands (`state list-modules`, `agent status`, `bootstrap init`, `blueprint apply`) appear in these files
  - Reference: `docs/content/en/docs/community/support.md`, `docs/content/en/docs/community/release-notes.md`, `docs/content/en/docs/community/announcement-0.1.0.md`

- [x] **Community development docs reference removed pkg paths**
  - Resolution: doc
  - Docs recommend tests/debugging under `./pkg/state`, `./pkg/events`, `./pkg/statemgmt`, and `./pkg/agent`
  - These packages moved under `internal/*` (no public pkg equivalents)
  - Update docs or re-export packages
  - Reference: `docs/content/en/docs/community/development.md`, `docs/content/en/docs/community/windows-development.md`

- [x] **Blueprint reference docs reference missing blueprint subcommands**
  - Resolution: doc
  - Docs use `blueprint bundle *`, `blueprint mirror *`, and `blueprint snapshot show`
  - CLI lacks bundle/mirror commands and snapshot show
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/reference/blueprints.md`

- [x] **Module reference doc counts do not match code**
  - Resolution: code + doc
  - Doc said "93" (TODO claimed "94"), TOC listed 95 modules
  - Updated count from 93 to 95
  - Reference: `docs/content/en/docs/reference/modules.md`, `internal/statemgmt/*`

- [x] **Module reference lists modules that are not registered**
  - Resolution: code + doc
  - Registered 44 previously-unregistered modules by adding init()+RegisterModule() to 16 files
  - Fixed Module interface mismatches: database Apply (extra check param), docker/podman/git/x509/database Test (wrong return type)
  - K8s modules (12) remain unregistered — they require a client dependency; added doc note
  - Reference: `docs/content/en/docs/reference/modules.md`, `internal/statemgmt/*`

- [x] **Kubernetes concept docs overstate operator/CRD support**
  - Resolution: doc + epic
  - Added implementation status callouts throughout kubernetes.md marking controllers, drift detection, and deployment as planned/scaffolded
  - Created Epic 48 (`epics/48-kubernetes-operator.md`) for full operator implementation: informers, reconciliation, drift detection, leader election, Helm/Kustomize deployment
  - Updated AGENTS.md with Epic 48 in repo structure and planned epics list
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `epics/48-kubernetes-operator.md`, `AGENTS.md`

- [x] **Kubernetes concept docs claim context switching that is not implemented**
  - Resolution: code fix
  - `NewClient` now uses `clientcmd.NewNonInteractiveDeferredLoadingClientConfig` with `ConfigOverrides.CurrentContext` to respect `ClusterConfig.Context`
  - Added tests: context selection, invalid kubeconfig, invalid context
  - Reference: `internal/k8s/client.go`, `internal/k8s/client_test.go`

- [x] **Kubernetes concept docs CRD schemas do not match code**
  - Resolution: doc
  - Rewrote RemoteExecution example: `keystonecore.io/v1` API group, flat `labelSelector` string, `command` as `[]string`, `mode` field, removed `retries`, fixed status field names
  - Rewrote StateConfig example: `keystonecore.io/v1`, `name` instead of `id`, removed `state` field, removed `driftDetection` from spec, added `target`/`requisites`, flat `driftDetected` bool in status
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`

- [x] **Windows installation docs reference missing agent commands/flags**
  - Resolution: doc
  - `windows-installation.md` already uses correct `service-install`/`service-uninstall` commands
  - Removed nonexistent `--console` flag from `windows-development.md` (3 occurrences); agent runs in foreground by default when not detected as a Windows service
  - Reference: `docs/content/en/docs/community/windows-development.md`

- [x] **Windows operations docs use unsupported exec flags**
  - Resolution: no change needed (invalid TODO)
  - Docs use `-- powershell -Command` (command after `--` separator), not `--shell`
  - `exec run` does have a `--shell` flag (line 282 of `cmd/kscore-exec/main.go`)
  - Reference: `docs/content/en/docs/operations/windows.md`, `cmd/kscore-exec/main.go`

- [x] **Registry ops docs reference metrics not exposed by registry**
  - Resolution: doc
  - Removed nonexistent Prometheus scrape config, latency alert, and Grafana metrics table
  - Replaced with blackbox_exporter health probe config and note that `/metrics` is not yet implemented
  - Kept disk space alert (uses node_exporter, not registry metrics)
  - Reference: `docs/content/en/docs/operations/registry.md`

- [x] **CLI reference docs include non-existent flags/commands**
  - Resolution: no change needed (invalid TODO)
  - `config validate` exists in `cmd/kscorectl/main.go` with tests
  - `--show-deprecated` and `exec batch` are not referenced in cli.md
  - Salt-style aliases appear only in migration table's "Old Command" column (correctly documenting removal)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscorectl/main.go`

- [x] **Reference NATS mesh docs reference missing debug plugin**
  - Resolution: no change needed (invalid TODO)
  - No `kscorectl debug` references exist in nats-mesh.md or anywhere in docs/
  - Reference: `docs/content/en/docs/reference/nats-mesh.md`

- [x] **NATS mesh deployment ops docs use unsupported config/commands**
  - Resolution: doc
  - Changed 7 occurrences of `nats.urls:` (YAML list) to `nats.url:` (comma-separated string) matching `NATSConfig.URL string`
  - Replaced nonexistent `kscorectl agent update-config --nats-urls` with config file edit + service restart
  - Left `hub.urls` and `gateway.remotes[].urls` unchanged (those are `[]string` slices)
  - Reference: `docs/content/en/docs/operations/nats-mesh-deployment.md`

- [x] **Query API reference docs mismatch with code package/export**
  - Resolution: no change needed (invalid TODO)
  - Doc already notes it's an internal package and uses correct `internal/query` import path
  - No `pkg/query` references exist in the doc
  - Reference: `docs/content/en/docs/reference/query-api.md`

- [x] **Query API docs describe HTTP API that is not implemented**
  - Resolution: doc
  - Removed misleading "use the gRPC or REST API endpoints" from note on line 10
  - Replaced with "External query API endpoints (REST/gRPC) are not yet available"
  - Reference: `docs/content/en/docs/reference/query-api.md`

- [x] **Query API docs mention log pagination cursor not supported**
  - Resolution: code
  - Implemented pagination cursor support in both LokiQuerier and InMemoryLogsQuerier
  - LokiQuerier: `LogsQuery.Start` overrides `start` query param when set
  - InMemoryLogsQuerier: `LogsQuery.Start` parsed as RFC3339Nano, filters entries after cursor
  - Updated docs pagination example with correct cursor format per backend
  - Added 4 tests: InMemory pagination, invalid cursor, Loki cursor passthrough, Loki no-cursor
  - Reference: `docs/content/en/docs/reference/query-api.md`, `internal/query/logs.go`

- [x] **Metrics reference docs claim /metrics on control plane**
  - Resolution: invalid
  - `kscore-server` DOES expose `/metrics` when `metrics.enabled: true` (lines 358-378 of main.go)
  - Configurable path via `metrics.path` (default `/metrics`), includes optional Go/process metrics
  - Reference: `docs/content/en/docs/reference/metrics.md`, `cmd/kscore-server/main.go`

- [x] **Profiling docs reference config/pprof endpoints not wired**
  - Resolution: code
  - Added `ProfilingConfig` to `internal/config` with `profiling.enabled`, `profiling.listen`, `profiling.blockprofilerate`, `profiling.mutexprofilefraction`
  - Wired `internal/profiling.Server` start/stop into `kscore-server` (after tracing init) and `kscore-agent` (after NATS health check)
  - Fixed observability concept doc: profiling port 8080 → 6060 (separate server)
  - Added 3 config validation tests
  - Reference: `internal/config/config.go`, `cmd/kscore-server/main.go`, `cmd/kscore-agent/main.go`

- [x] **Metrics catalog does not match implemented metrics**
  - Resolution: doc
  - Rewrote `docs/content/en/docs/reference/metrics.md` to match all code-defined metrics
  - Removed nonexistent metrics: DB (kscore_db_*), batch (kscore_batch_*), kscore_api_active_connections, kscore_agent_heartbeat_received/missed, kscore_agent_memory_total_bytes, kscore_agent_disk_total_bytes, kscore_event_lag_seconds, kscore_events_stored/count, kscore_policy_compliance_score (→ kscore_compliance_score), kscore_policy_compliant_agents, kscore_policy_violations_by_agent, kscore_gitops_verifications_total (→ kscore_gitops_deployments_verified_total), kscore_gitops_rollbacks_total (→ kscore_gitops_rollbacks_triggered_total), kscore_gitops_webhooks_failed/verification_duration/rollback_duration/promotions/sync
  - Added undocumented metrics: event subsystem (received, failed, severity, publisher/subscriber errors, active_subscribers, action_failures, storage_operations/failures, uptime, event_rate, last_event_timestamp), network/IPv6 (listeners_active, connections_total/active, agents_by_ip_version), agent (heartbeat_seconds, commands_executed, states_applied), proxy (all kscore_proxy_*), file distribution (all kscore_files_*)
  - Reference: `docs/content/en/docs/reference/metrics.md`

- [x] **Metrics reference types/labels don't match implementation**
  - Resolution: doc (fixed as part of full metrics.md rewrite above)
  - Fixed `kscore_api_request_duration_seconds`: Summary → Histogram, label `path` → `endpoint`
  - Fixed `kscore_api_requests_total`: label `path` → `endpoint`
  - Fixed `kscore_state_application_duration_seconds`: Summary → Histogram
  - Fixed `kscore_cluster_heartbeat_latency_seconds`: Histogram → Summary, label `target` → `member_id`
  - Fixed `kscore_agents_connected`: removed nonexistent `environment` label
  - Fixed `kscore_agents_disconnected_total`: removed nonexistent `reason` label
  - Fixed `kscore_command_executions_total`: removed nonexistent `datacenter` label
  - Fixed `kscore_state_resources_total`: labels `module` → `type`, `status`
  - Fixed `kscore_state_drift_detected_total`: label `severity` → `resource`
  - Fixed `kscore_cluster_member_status`: removed nonexistent `address` label
  - Fixed `kscore_agent_disk_usage_bytes`: removed nonexistent `mount` label
  - Fixed command metric name: `kscore_command_duration_seconds` → `kscore_command_execution_duration_seconds`
  - Fixed state changes name: `kscore_state_changes_total` → `kscore_state_changes_applied_total`
  - Fixed compliance metric name: `kscore_policy_compliance_score` → `kscore_compliance_score`
  - Fixed all query/alert examples to use correct metric names and histogram_quantile() syntax
  - Reference: `docs/content/en/docs/reference/metrics.md`, `internal/metrics/collectors.go`

- [x] **Visualization API docs reference server not wired into control plane**
  - Resolution: doc
  - Added implementation status callout noting the server is not yet integrated into `kscore-server`
  - Endpoints will become available in a future release; package can be used as library in custom builds
  - Reference: `docs/content/en/docs/reference/visualization.md`

- [x] **Visualization docs reference non-existent Go package**
  - Resolution: doc (already fixed)
  - Docs already use correct import path `github.com/shawnbutts/keystone-core/internal/visualization`
  - The earlier TODO description was inaccurate — the import was already correct in the doc
  - Reference: `docs/content/en/docs/reference/visualization.md`

- [x] **Visualization docs include fields not present in API structs**
  - Resolution: doc
  - Replaced `"labels": {"tier": "web", ...}` with `"tags": ["tier:web", ...]` in both agent response examples
  - Fixed config type name from `VisualizationConfig` → `Config` to match `internal/visualization/types.go`
  - Reference: `docs/content/en/docs/reference/visualization.md`, `internal/visualization/types.go`

- [x] **CLI quick reference docs reference missing commands**
  - Resolution: doc
  - Docs include `kscorectl config validate`, `agent status/quarantine/unquarantine/renew-svid`, `health check`, `event list --agent`, and `policy test`
  - CLI does not implement these commands/flags
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/reference/cli-quick-reference.md`

- [x] **Additional scenarios reference missing exec/compliance/cluster flags**
  - Resolution: doc
  - Compliance scenario uses `kscorectl compliance *` and `policy test/evaluate` plus `logs --reactor` (not implemented)
  - Database HA and multi-tier/hybrid scenarios use `kscorectl exec ... --cmd` shorthand (not implemented) and `blueprint apply` (missing)
  - Hybrid scenario uses `agents list --group-by` and `connectivity test` (not implemented)
  - Disaster recovery uses `event emit` (singular) and `cluster status --endpoint` (unsupported flag)
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/scenarios/compliance-automation.md`, `docs/content/en/docs/scenarios/database-ha.md`, `docs/content/en/docs/scenarios/multi-tier-webapp.md`, `docs/content/en/docs/scenarios/hybrid-infrastructure.md`, `docs/content/en/docs/scenarios/disaster-recovery.md`

- [x] **Edge deployment scenario uses unsupported agent config keys**
  - Resolution: doc
  - Scenario `agent.yaml` uses `server.urls`, `reconnect.*`, `cache.*`, `offline.*`, and `proxy.*`
  - `internal/config.AgentConfig` has no such fields (agent config only includes ID, heartbeat, timeouts, metadata interval, address family, labels, advertise_addrs)
  - Update scenario docs or implement config support
  - Reference: `docs/content/en/docs/scenarios/edge-deployment.md`, `internal/config/config.go`

- [x] **GitOps/event scenarios use config files not supported by control plane**
  - Resolution: doc
  - GitOps scenario uses `gitops.webhook/sync/verification` config blocks
  - Event-driven automation scenario defines `events:` schemas under `config/events/*.yaml`
  - `internal/config.Config` has no gitops or events config sections
  - Update scenarios or implement config support
  - Reference: `docs/content/en/docs/scenarios/gitops-workflow.md`, `docs/content/en/docs/scenarios/event-driven-automation.md`, `internal/config/config.go`

- [x] **Edge and proxy concept docs reference missing cache/sync/proxy commands**
  - Resolution: doc
  - Edge docs use `kscorectl cache *`, `kscorectl sync *`, `kscorectl connection test`, and `agent status --edge` (not implemented)
  - Proxy concept docs use `proxy credential create/verify`, `proxy discovery *` (CLI uses `proxy discover *`), `proxy device ping/status/connect/config show`, `proxy discovery logs/config show`, and `proxy state apply --device`
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/concepts/edge.md`, `docs/content/en/docs/concepts/proxy-agents.md`

- [x] **Tutorial drift detection docs reference missing commands/flags**
  - Resolution: doc
  - Docs use `kscorectl apply` (no top-level apply; should be `state apply`)
  - Docs use `events list --last 24h` (CLI uses `--since`)
  - Update docs or implement aliases
  - Reference: `docs/content/en/docs/tutorials/drift-detection.md`

- [x] **Tutorial secrets quickstart docs reference missing secrets/exec commands**
  - Resolution: doc
  - Docs use `secrets health/put/get/list/audit/stats/policy/refresh/injection` and `secrets rotation ...` (not implemented; CLI uses `secrets rotate`)
  - Docs use `kscorectl exec <target> -- ...` (no exec shorthand; should be `exec run`)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/tutorials/secrets-quickstart.md`

- [x] **Self-management docs reference missing diagnostics/certs commands**
  - Resolution: doc
  - Docs use `kscorectl diagnostics collect` and `kscorectl certs status/rotate/verify`
  - No diagnostics/certs plugin found in codebase
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [x] **Self-management docs reference missing cluster token command**
  - Resolution: doc
  - Docs use `kscorectl cluster token` during bootstrap import
  - `kscore-cluster` has no `token` subcommand
  - Update docs or implement token command
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [x] **Self-management docs reference missing agent ping command**
  - Resolution: doc
  - Docs use `kscorectl agent ping --all`
  - `kscore-agents` has no `ping` subcommand
  - Update docs or implement ping
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [x] **Deployment docs reference missing migrate export/import**
  - Resolution: doc
  - Docs use `kscorectl migrate export` and `kscorectl migrate import`
  - Code only implements `kscorectl migrate run` and `kscorectl migrate validate`
  - Update docs or implement subcommands
  - Reference: `docs/content/en/docs/operations/deployment.md`

- [x] **Deployment docs reference missing state list command**
  - Resolution: doc
  - Docs use `kscorectl state list`
  - `kscore-state` has no `list` subcommand
  - Update docs or implement list
  - Reference: `docs/content/en/docs/operations/deployment.md`

- [x] **Deployment docs use unsupported NATS config keys**
  - Resolution: doc
  - Example config uses `nats.embedded.jetstream.enabled` and `nats.embedded.jetstream.store_dir`
  - Code expects `nats.embedded.enable_jetstream` and separate `nats.jetstream.*` config
  - Update docs or add config alias support
  - Reference: `docs/content/en/docs/operations/deployment.md`, `internal/config/config.go`

- [x] **Incident response doc references missing or mismatched CLI commands**
  - Resolution: doc
  - Commands not implemented: `api-key revoke-all`, `agent certificate rotate/regenerate`, `agent health`, `user reset-password`, `auth session invalidate-all`, `module block`, `module audit`
  - Commands/flags mismatched: `policy audit list --since` (no list subcommand or since filter), `audit query` (no query command), `files download` (CLI uses `files get`)
  - Update docs or implement commands/flags
  - Reference: `docs/project/INCIDENT-RESPONSE.md`, `cmd/kscorectl/main.go`, `cmd/kscore-agents/main.go`, `cmd/kscore-audit/main.go`, `cmd/kscore-policy/main.go`, `cmd/kscore-module/main.go`, `cmd/kscore-files/commands_files.go`

- [x] **Scenarios index references missing blueprint apply command**
  - Resolution: doc
  - `kscorectl blueprint apply` is used but no apply subcommand exists
  - Update docs or implement apply
  - Reference: `docs/content/en/docs/scenarios/_index.md`

- [x] **Module reference docs reference unsupported init template flag**
  - Resolution: doc
  - Docs use `kscorectl module init --template rust` but CLI only supports `--type` (starlark/wasm)
  - Update docs or implement template flag
  - Reference: `docs/content/en/docs/reference/modules.md`

- [x] **DNS module docs reference secret_ref/env credentials not supported**
  - Resolution: doc
  - Docs show `credentials.secret_ref` and provider env vars for DNS records
  - DNS module only parses inline `api_key`/`api_token`/`account_id` and `credentials.extra`; no secret_ref/env resolution
  - Update docs or implement secret/env credential resolution
  - Reference: `docs/content/en/docs/reference/modules/dns.md`, `internal/statemgmt/module_dns.go`, `internal/dns/providers/*`

### Packaging Documentation

- [x] **Update kscore-cli package contents**
  - Resolution: doc
  - Docs list only 4 tools in the `kscore-cli` package
  - Actual package includes many plugin binaries (agents, audit, backup, cluster, gitops, identity, etc.)
  - Update `docs/content/en/docs/getting-started/installation.md` and release guide as needed

### API Documentation Improvements

- [x] **Document streaming gRPC methods**
  - Resolution: doc
  - `SubscribeEvents` (event streaming)
  - `WatchMembership` (cluster membership changes)
  - `WatchLeadership` (leadership changes)

- [x] **Document CoordinationService**
  - Resolution: doc
  - Server-to-server coordination when NATS unavailable
  - 6 RPCs: `ClusterHealth`, `GetLeader`, `NATSStatus`, etc.

### Identity CLI Docs vs Code

- [x] **Align kscore-identity token create flags and required args**
  - Resolution: doc
  - Docs show `token create` flags: `--path`, `--ttl`, `--uses`
  - Code requires `--agent-id`, supports `--ttl` and `--label`, and does not implement `--path`/`--uses`
  - Update docs or implement missing flags
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [x] **Fix kscore-identity CA output/fields mismatch**
  - Resolution: doc
  - Docs list CA info fields like `SVIDs Issued`, `Last Rotation`, `Next Rotation`, `Auto-Rotation`
  - Code only prints Trust Domain + Root/Signing CA details
  - Update docs or implement additional fields
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [x] **Align kscore-identity federation add flags**
  - Resolution: doc
  - Docs include `--type` (bidirectional/unidirectional) and mark `--endpoint` as required
  - Code supports `--endpoint`, `--profile`, `--refresh-interval` and does not implement `--type` or enforce required endpoint
  - Update docs or add flag/validation
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [x] **Align kscore-identity bundle export formats**
  - Resolution: doc
  - Docs list `pem`, `jwks`, and `spiffe` formats
  - Code only supports `pem` and `jwks`
  - Update docs or implement `spiffe`
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [x] **Fix kscore-identity global defaults**
  - Resolution: doc
  - Docs say `--audit-level` default is `errors` and `--audit-output` is system-dependent
  - Code defaults to `audit-level=all` and `audit-output=auto`
  - Update docs or change defaults
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

### Environment Variable Documentation

- [x] **Verify documented but not found env vars**
  - Resolution: doc
  - `KSCORE_NATS_TLS_CERT`, `KSCORE_NATS_TLS_KEY`, `KSCORE_NATS_TLS_CA`
  - `KSCORE_NATS_DEBUG`, `KSCORE_NATS_CLUSTER`
  - `KSCORE_NATS_URL`, `KSCORE_NATS_URLS`, `KSCORE_NATS_MODE`
  - `KSCORE_NATS_CREDENTIALS`, `KSCORE_NATS_CREDS_FILE`
  - `KSCORE_NATS_USER`, `KSCORE_NATS_PASSWORD`
  - `KSCORE_NATS_BUFFER_SIZE`, `KSCORE_NATS_BUFFER_DIR`
  - `KSCORE_OUTPUT_FORMAT`

- [x] **Document test/development env vars**
  - Resolution: doc
  - `KSCORE_LOAD_TEST`, `KSCORE_AGENT_COUNT`, `KSCORE_TEST_DURATION`
  - `KSCORE_TEST_POSTGRES_DSN`, `KSCORE_PERF_TESTS`
  - Mark as development/testing only

### Project Documentation

- [x] **Project security docs reference missing public packages**
  - Resolution: doc
  - SECURITY-DESIGN and SECURITY-GOVERNANCE reference `pkg/security/*`, `pkg/auth/*`, `pkg/authz/*`, `pkg/crypto/*`, and `pkg/audit/*`
  - Code uses `internal/security` and `internal/audit`; auth/authz/crypto packages are not present
  - Update docs or export packages
  - Reference: `docs/project/SECURITY-DESIGN.md`, `docs/project/SECURITY-GOVERNANCE.md`, `internal/security/*`, `internal/audit/*`

- [x] **Design docs reference internal paths as pkg**
  - Resolution: doc
  - DESIGN.md references `pkg/security/tls.go` but file lives in `internal/security/tls.go`
  - Update docs or move code
  - Reference: `docs/project/DESIGN.md`, `internal/security/tls.go`

- [x] **Development docs use stale package paths**
  - Resolution: doc
  - Docs recommend `go test ./pkg/state/...` but `pkg/state` no longer exists (state code moved to `internal/state`)
  - Update docs or re-export packages
  - Reference: `docs/project/DEVELOPMENT.md`

- [x] **Roadmap docs reference non-existent deprecation package path**
  - Resolution: doc
  - Roadmap claims `pkg/cli/deprecation/` exists
  - Deprecation framework lives under `internal/cli/deprecation`
  - Update docs or export package
  - Reference: `docs/content/en/docs/community/roadmap.md`, `internal/cli/deprecation/*`

- [x] **Incident response doc references many non-existent CLI commands**
  - Resolution: doc
  - `kscorectl api-key revoke-all`, `kscorectl auth session invalidate-all`
  - `kscorectl agent certificate rotate/regenerate`, `kscorectl agent health`
  - `kscorectl user reset-password`, `kscorectl module block/audit`
  - `kscorectl audit query`, `kscorectl policy audit list`
  - `kscorectl state check --comprehensive`
  - `kscorectl files download` (no such CLI)
  - Update incident response guide or implement commands
  - Reference: `docs/project/INCIDENT-RESPONSE.md`, `cmd/kscorectl/main.go`, `cmd/kscore-agents/main.go`

---

## Future Work (Planned CLI Enhancements)

These are CLI commands identified as worth implementing in a future epic:

### Admin & Security CLI Commands

- [x] **kscorectl config show** - Display current configuration with optional `--include-defaults` flag *(Done: added server-side GET /api/v1/config endpoint with secret redaction, improved CLI fallback to load and display local config when server unreachable)*
- [x] **kscorectl rbac list-roles** - List RBAC roles with optional `--show-permissions` flag *(Done: added GET /api/v1/rbac/roles endpoint, kscorectl rbac list-roles command with table output and local fallback)*
- [x] **kscorectl rbac export** - Export RBAC configuration for backup/audit *(Done: added GET /api/v1/rbac/export endpoint, kscorectl rbac export command with --format json/yaml and --output flags)*
- [x] **kscore-audit query** - Query audit logs with filters (`--type`, `--since`, `--api-key`, `--agent`) — added `query` alias for `search` command and `--api-key` filter flag
- [x] **kscore-audit watch** - Real-time audit log monitoring with filters — added watch subcommand with --type, --status, --agent, --user, --api-key, --interval flags and context-based cancellation
- [x] **kscore-agents re-enroll** - Re-enroll agent with new credentials (for security incidents) — added re-enroll subcommand with --force, --reason flags; invalidates credentials and issues one-time enrollment token
- [x] **kscore-agents revoke-credentials** - Revoke agent credentials without deletion — added revoke-credentials subcommand with --force, --reason flags; locks out agent with no new token, directs to re-enroll for recovery

### Runbook CLI Commands

- [x] **kscore-runbook execute** - Execute a runbook with inputs (`--input key=value`, `--dry-run`) — command already existed with --var/--dry-run/--wait/--timeout; added --input as alias for --var
- [x] **kscore-runbook status** - Check execution status of a running/completed runbook — command already existed with execution-id arg, state/progress/step display, json/yaml/table output; added docs
- [x] **kscore-runbook list-executions** - List runbook execution history (`--runbook`, `--since`) — command already existed with --runbook/--state/--limit; added --since flag with duration and date parsing
- [x] **kscore-runbook test** - Test runbook with mock handlers (`--mock-file`) — added --mock-file flag with JSON validation, mock handler count reporting, and PASS/FAIL results
- [x] **kscore-runbook audit list** - List runbook audit events (`--runbook`, `--start`, `--end`) — restructured audit as parent command with show/list subcommands; list supports --runbook, --start, --end, --limit filters
- [x] **kscore-runbook audit report** - Generate compliance report (`--format`, `--start`, `--end`) — added audit report subcommand with summary/detailed/csv formats, --start/--end/--runbook filters, aggregation by action/runbook/user

### CLI Plugin Command Routing

- [x] **Review and fix "double command" routing in kscorectl plugin dispatch** — audited all 15 plugins; only kscore-files was affected (had `files` subcommand group under `kscore-files` root). Flattened file subcommands (list/get/put/delete/info/sync) directly onto root. Updated tests, docs, and help examples.

- [x] **Wire remaining REST API handlers into kscore-server** — moved to Epic 49 (`epics/49-rest-api-handler-wiring.md`). Requires constructing real dependencies for 8 handler packages, with conditional registration for infrastructure-dependent handlers.

- [ ] **Implement outbound webhook subscriptions**
  - Resolution: code
  - Currently only inbound webhooks (receiving from ArgoCD/GitHub/etc.) are supported
  - Implement outbound webhooks to send events to external systems:
    - CRUD endpoints for webhook subscriptions (url, events filter, secret)
    - Webhook dispatcher to deliver events to configured endpoints
    - HMAC signature generation for outbound payloads
    - Delivery tracking and retry logic
  - Reference: `pkg/api/webhooks/handlers.go`, `internal/gitops/webhook/`

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
- Documentation should be updated alongside code changes

---

## Summary Statistics

| Category | Open |
|----------|------|
| High Priority | 3 |
| Medium Priority | 8 |
| Additional Documentation Drift | 85 |
| Low Priority | 77 |
| Future Work | 13 |
| **Total** | **186** |
