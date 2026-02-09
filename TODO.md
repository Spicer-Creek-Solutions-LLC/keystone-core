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

- [ ] **File backends config schema and supported types don’t match implementation**
  - Resolution: code
  - `docs/content/en/docs/reference/file-backends.md` documents a single `backend:` block and types `local`, `s3`, `gcs`, `azure`, `git`, `nats`
  - `cmd/kscore-files/main.go` expects `backends:` list and only wires `filesystem` backend in `createBackend`
  - Update docs or wire remaining backend types and schema aliases (`local` → `filesystem`, `nats` → `nats-object-store`)

### Concepts Documentation CLI Drift

- [x] **Events concept documents subcommands not implemented** *(DONE)*
  - Added `analyze`, `subscribers`, `storage-stats`, `prune`, and `archive` subcommands to `cmd/kscore-events/main.go`

- [ ] **GitOps concept references `git-sync` CLI that does not exist**
  - Resolution: code - substantial functionality gap
  - `docs/content/en/docs/concepts/gitops.md` documents extensive `git-sync` CLI: conflicts (list/show/diff/resolve/resolve-all), locks (lock/unlock/list), history, audit, status, trigger
  - Only `kscorectl gitops repo sync <name>` exists; conflict resolution, locking, and audit features need implementation
  - Note: Cannot simply remove docs - conflict resolution is core GitOps workflow content

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

- [ ] **Runbooks reference missing auth/debug/config/db/diagnostics commands**
  - Resolution: code
  - `docs/runbooks/bootstrap-new-cluster.md`, `docs/runbooks/performance-degradation.md`, `docs/runbooks/security-incident.md`, `docs/runbooks/disaster-recovery.md` use `kscorectl auth ...`, `kscorectl debug ...`, `kscorectl config set ...`, `kscorectl db compact/rotate-credentials`, `kscorectl diagnostics collect`
  - None of these command groups exist in `cmd/kscorectl/main.go`

- [ ] **Runbooks reference unsupported cluster/federation subcommands**
  - Resolution: code
  - `docs/runbooks/bootstrap-new-cluster.md`, `docs/runbooks/disaster-recovery.md`, `docs/runbooks/capacity-scaling.md` use `cluster token`, `cluster quorum`, `cluster join-config`, `cluster member add/remove`
  - `docs/runbooks/bootstrap-new-cluster.md` references `federation status`, `federation trust list`, `federation ping`
  - `cmd/kscore-cluster` and `cmd/kscore-federation` do not implement these subcommands

- [ ] **Runbooks reference unsupported bootstrap and upgrade commands**
  - Resolution: code
  - `docs/runbooks/bootstrap-new-cluster.md` uses `kscore-bootstrap join`, `prereq-check`, `cert-gen`
  - `cmd/kscore-bootstrap` only implements `seed`, `validate`, `restore`, `import`, `status`, `cleanup`, `version`
  - `docs/runbooks/upgrade-cluster.md` uses `upgrade resume`, `upgrade path`, `upgrade check --from/--to`, `upgrade agents --status`
  - `cmd/kscore-upgrade` does not implement these flags/subcommands

- [ ] **Runbooks reference unsupported agent/security commands**
  - Resolution: code - extensive security incident commands need implementation
  - `docs/runbooks/security-incident.md` uses `agent verify`, `agent invalidate-sessions`, `agent certificates regenerate`, `security scan`
  - Also references `identity ca rotate`, `db rotate-credentials`, `nats rotate-credentials`, `secrets rotate-keys`
  - No alternatives exist - these security response features need implementation

### Project Docs Drift

- [ ] **Incident response project doc references unsupported commands**
  - Resolution: code - extensive incident response commands need implementation
  - `docs/project/INCIDENT-RESPONSE.md` uses `kscorectl files download`, `api-key revoke-all`, `agent certificate rotate/regenerate`, `user reset-password`, `audit query`, `policy audit list`, `auth session invalidate-all`
  - These are security/incident response features that need implementation - no alternatives exist

### Runbooks and Operations Docs Drift

- [ ] **Runbooks/operations docs reference missing `kscorectl debug` commands**
  - Resolution: code
  - `docs/runbooks/*.md`, `docs/content/en/docs/operations/nats-mesh-operations.md`, and `docs/content/en/docs/concepts/nats-mesh.md` reference `kscorectl debug ...` (db-status, nats status/events/trace/export, conflict resolution, etc.)
  - No `debug` command or plugin exists in `cmd/`

### Epics Documentation Drift

- [x] **Epic 36 secrets management doc lists many CLI commands not implemented** *(DONE - dynamic/leases/transit/template/cache)*
  - Added `dynamic list/get/revoke`, `leases list/revoke/renew`, `encrypt`, `decrypt`, `rewrap`, `template`, `cache status/clear/list` subcommands
  - Note: `secrets backends` and `secrets audit` were added in a previous commit

- [ ] **Epic 09 plugin system doc references missing `kscorectl plugin` commands and module features**
  - Resolution: code
  - `epics/09-plugin-system.md` references `kscorectl plugin list/install/publish/verify` and module commands like `module mirror`, `module clean`, `module update`
  - No `kscorectl plugin` command group and several module subcommands are not implemented

- [x] **Epic 03 state management doc references missing CLI commands** *(DONE)*
  - Added `compile` (with --vars, --vars-file, --output), `vars get/list`, `export`, `restore`, and `drift --fix` to kscore-state

- [x] **Epic 37 runbooks doc references `kscorectl runbook` subcommands not implemented** *(DONE - core commands)*
  - Added `list`, `execute`, `status`, `list-executions`, `audit`, `test` subcommands
  - Note: `show`, `delete`, `versions`, `rollback` still need implementation

- [ ] **Epic 02 remote execution doc uses missing top-level exec/file/job commands and flags**
  - Resolution: code
  - `epics/02-remote-execution.md` references `kscorectl script`, `kscorectl file upload`, and `kscorectl job` commands
  - `cmd/kscore-exec` only provides `exec run/async/script/status/list/output/cancel/history` and has no `--script`, `--script-file`, or `--upload` flags

- [x] **Epic 04 event system doc uses missing events subcommands** *(DONE)*
  - Added `subscribe` (with --filter-type, --filter-severity, --output) and `export` (with --format json/csv, --start, --end, --type, --limit) subcommands

- [x] **Epic 06 policy enforcement doc references policy/compliance commands not implemented** *(DONE)*
  - Added `eval`, `test`, `schedule create/list/delete`, `remediate`, `monitor`, and `compliance report` subcommands

- [ ] **Epic 07 observability doc references missing debug profiling command**
  - Resolution: code
  - `epics/07-observability.md` documents `kscorectl debug profile`
  - No `kscore-debug` plugin or `debug` command in `kscorectl`

- [ ] **Epic 08 multi-environment doc references missing inventory/discover/import/container/istio commands**
  - Resolution: code
  - `epics/08-multi-environment.md` documents `kscorectl inventory`, `discover`, `import`, `container`, and `istio` commands
  - No corresponding plugins exist under `cmd/`

- [ ] **Epic 22 file distribution doc references missing CLI operations**
  - Resolution: code
  - `epics/22-file-distribution.md` documents `kscorectl files list/get/put/delete` and `kscorectl files mirrors ...`
  - `cmd/kscore-files` only implements `serve` and `version`

- [ ] **Epic 23 self-management doc references missing self/backup/upgrade commands**
  - Resolution: code
  - `epics/23-self-management.md` documents `kscorectl self status/health/drift/apply/export` and `kscorectl backup download`
  - `kscorectl self` plugin does not exist and `cmd/kscore-backup` has no `download` subcommand
  - Doc examples also use `kscorectl upgrade apply/rollback` but `cmd/kscore-upgrade` uses `execute` and has no rollback command

- [ ] **Epic 25 blueprints doc references missing bundle/mirror/migrate/status commands**
  - Resolution: code
  - `epics/25-blueprints.md` documents `kscorectl blueprint bundle/bundle-install/bundle-verify/migrate/status` and `kscorectl mirror ...`
  - No bundle or mirror subcommands exist in `cmd/kscore-blueprint*`, and no `kscore-mirror` plugin exists

- [ ] **Epic 38 air-gapped deployments doc references missing airgap CLI**
  - Resolution: code
  - `epics/38-air-gapped-deployments.md` documents `kscorectl airgap ...`
  - No `kscore-airgap` plugin or subcommands exist

### Reference Docs Drift

- [x] **Runbook automation reference lists unsupported runbook commands** *(DONE)*
  - Added `runbook list`, `execute`, `status`, `list-executions`, `audit`, and `test` subcommands to `cmd/kscore-runbook`
  - Updated runbook-automation.md reference docs

- [ ] **NATS mesh reference lists missing debug commands**
  - Resolution: code
  - `docs/content/en/docs/reference/nats-mesh.md` documents `kscorectl debug nats ...`
  - No `debug` subcommand or plugin exists in `cmd/`
  - Significant diagnostic tooling needs implementation

- [x] **Blueprint reference docs include bundle/mirror/registry commands not implemented** *(DONE - bundle/mirror)*
  - Added `blueprint bundle create/install` and `blueprint mirror add/remove/list/sync/status` subcommands
  - Note: `blueprint registry add` still needs implementation
### Tutorials and Scenarios Documentation CLI Drift

- [ ] **Event-driven automation scenario references missing reactor/trace commands**
  - Resolution: code
  - `docs/content/en/docs/scenarios/event-driven-automation.md` uses `kscorectl reactors`, `kscorectl reactor ...`, `kscorectl events trace`, `events queue-status`
  - No reactor CLI exists and `kscorectl events` does not implement `trace` or `queue-status`

- [ ] **Scenario docs reference missing environment/promote/approve/rollback commands**
  - Resolution: both
  - `docs/content/en/docs/scenarios/multi-environment.md` uses `kscorectl environment ...`, `promote ...`, `approve ...`, `rollback ...`, `metrics query`
  - promote/rollback exist under `kscorectl gitops` but with different flags (--pipeline vs --state)
  - approve exists under `kscorectl runbook` but with different flags
  - environment and metrics commands don't exist at all
  - Requires both CLI changes and doc restructuring

- [ ] **Scenario docs reference missing ping/connectivity commands**
  - Resolution: code
  - `docs/content/en/docs/scenarios/windows-infrastructure.md`, `docs/content/en/docs/scenarios/edge-deployment.md`, `docs/content/en/docs/scenarios/hybrid-infrastructure.md` use `kscorectl ping` and `kscorectl connectivity test`
  - No `ping` or `connectivity` commands exist in `kscorectl`

- [ ] **Scenario docs reference missing state export/restore commands**
  - Resolution: code
  - `docs/content/en/docs/scenarios/multi-environment.md` uses `kscorectl state export` and `kscorectl state restore`
  - `cmd/kscore-state` does not implement these commands
  - Implement export/restore commands
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [ ] **Control plane config docs include settings not in config structs**
  - Resolution: code
  - Sections for agent management, execution, state, events, gitops, security, cluster, identity are documented
  - `internal/config.Config` has no corresponding fields for most of these
  - Implement config struct fields for documented settings
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [ ] **GitOps and security config sections are not implemented**
  - Resolution: code
  - Docs show `gitops.*` and `security.*` blocks in server config
  - No corresponding fields exist in `internal/config.Config`
  - Implement gitops and security config struct fields
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [ ] **`kscorectl config set` is referenced in docs but not implemented**
  - Resolution: code
  - `kscorectl config` only supports `validate`
  - Docs use `kscorectl config set ...` in runbooks, security training, and blueprints (15+ references)
  - Implementing `config set` would be more appropriate than removing operational examples
  - Reference: `docs/runbooks/*.md`, `docs/content/en/docs/operations/security-training.md`, `docs/content/en/docs/concepts/blueprints.md`, `cmd/kscorectl/main.go`

### SDK Documentation vs Implementation

- [ ] **SDK docs reference gRPC client code that is not generated**
  - Resolution: code
  - Examples import `event_service_pb2`/`event_service_pb2_grpc` and other `*_service.proto` artifacts
  - Only agent/controlplane/coordination stubs exist in `pkg/api/v1`
  - Generate missing gRPC stubs for event, policy, state, and gitops services
  - Reference: `docs/content/en/docs/reference/sdk.md`, `pkg/api/v1/*`, `api/proto/*.proto`

- [ ] **SDK examples target REST endpoints that don't exist**
  - Resolution: code
  - Examples use `/api/v1/events/stream`, webhook creation, and agent tag update payloads not supported by handlers
  - Implement missing endpoints: events streaming, webhook configuration
  - Reference: `docs/content/en/docs/reference/sdk.md`, `pkg/api/*/handlers.go`

### API Field/Endpoint Mismatches

- [x] **REST API handlers are not wired into kscore-server** *(DONE)*
  - Wired agents, execution, state, maintenance, and apikeys handlers into kscore-server HTTP mux
  - Remaining handlers (events, policy, gitops, cluster, webhooks, runbook) need their dependencies instantiated first

- [ ] **OpenAPI spec paths do not match implemented API**
  - Resolution: both
  - OpenAPI spec lists `/api/agents` and omits `/api/v1/agents`; code only implements `/api/v1/agents`
  - Spec includes `/api/v1/mirrors` and `/api/v1/discovery` endpoints that are not wired into `kscore-server`
  - Update spec to reflect actual endpoints or wire the handlers
  - Reference: `api/openapi/openapi-spec.yaml`, `pkg/api/agents/handlers.go`, `internal/files/mirror/api.go`, `internal/proxy/discovery/api.go`, `cmd/kscore-server/main.go`

- [ ] **Add missing agent proto fields** (or update docs)
  - Resolution: both
  - Documented but not in proto: `datacenter`, `environment`, `role`, `connected_at`
  - Location: `api/proto/agent.proto`

- [ ] **Agents REST API docs do not match handler filtering/response fields**
  - Resolution: both
  - Docs list `datacenter/environment/role` filters and return `labels` + `connected_at`
  - Handler only supports `status` + `labels` filter and returns `hostname/os/arch/labels/ip_addresses` + `registered_at/last_seen`
  - Docs show pagination (`limit/offset`), but handler returns all agents and uses `sort`
  - Update docs or implement filters/pagination/fields
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/agents/handlers.go`

- [ ] **Webhook REST API docs do not match implementation**
  - Resolution: code
  - Docs describe outbound webhooks (sending events TO external systems via url/events/secret config)
  - Handler only implements inbound webhooks (receiving FROM ArgoCD, GitHub, etc.) with `/webhooks/stats` + `/webhooks/config`
  - Need to implement outbound webhook support:
    - POST /api/v1/webhooks - create outbound webhook subscription (url, events, secret)
    - GET /api/v1/webhooks - list configured outbound webhooks
    - GET /api/v1/webhooks/{id} - get outbound webhook details
    - DELETE /api/v1/webhooks/{id} - remove outbound webhook
    - Webhook dispatcher to send events to configured endpoints
    - HMAC signature generation for outbound payloads
  - Keep existing inbound webhook receiver functionality
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/webhooks/handlers.go`

- [ ] **Webhook endpoint/path in docs does not match runtime server**
  - Resolution: both
  - Docs use `/api/v1/webhooks` on the main API port
  - Server runs a separate webhook receiver on `cfg.Webhook.Port` and `cfg.Webhook.Path`
  - REST webhooks handler is not wired into `kscore-server`
  - Reference: `docs/content/en/docs/reference/api.md`, `cmd/kscore-server/main.go`, `internal/gitops/webhook/receiver.go`

- [ ] **Rate limiting documentation does not match code**
  - Resolution: code
  - Docs: per-API-key request limits and `X-RateLimit-*` headers
  - Code: auth failure lockout for gRPC only (no HTTP rate limit headers)
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/auth/ratelimit.go`

- [ ] **API rate_limit config is documented but not wired**
  - Resolution: code
  - Config docs define `api.rate_limit` settings
  - No server middleware uses `internal/ratelimit` or these settings
  - Update docs or implement HTTP/gRPC rate limiting
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/ratelimit/*`, `cmd/kscore-server/main.go`

- [ ] **API CORS config is documented but not applied**
  - Resolution: code
  - Config docs define `api.cors` settings
  - No CORS handling in `kscore-server` HTTP mux
  - Update docs or add CORS middleware
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`, `cmd/kscore-server/main.go`

- [ ] **API TLS settings are documented but not wired into server**
  - Resolution: code
  - Config docs show API TLS fields (`cert_file`, `key_file`, `ca_file`, `min_version`)
  - gRPC/HTTP servers start without TLS configuration
  - Update docs or implement TLS for API listeners
  - Reference: `docs/content/en/docs/reference/configuration.md`, `cmd/kscore-server/main.go`

- [ ] **Metrics config is documented but not wired into control plane**
  - Resolution: code
  - Config docs define `metrics` section for server
  - `kscore-server` does not read/apply metrics config or expose `/metrics`
  - Update docs or implement metrics configuration wiring
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`, `cmd/kscore-server/main.go`

- [ ] **Tracing config is documented but not wired into control plane**
  - Resolution: code
  - Config docs define `tracing` section
  - No tracing initialization in `kscore-server`
  - Update docs or implement tracing wiring
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`, `cmd/kscore-server/main.go`

- [ ] **Health config is documented but not wired into control plane**
  - Resolution: code
  - Config docs define `health` settings
  - `kscore-server` health endpoints do not use config
  - Update docs or wire health config
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`, `cmd/kscore-server/main.go`

- [ ] **Health endpoint responses do not match docs**
  - Resolution: code
  - `/health/ready` checks NATS only; docs say NATS + database
  - `/health/status` returns agent counts; docs show component health and uptime
  - Update docs or expand health handlers
  - Reference: `docs/content/en/docs/operations/monitoring.md`, `cmd/kscore-server/main.go`

- [ ] **Pagination section in API docs conflicts with implementation**
  - Resolution: code
  - Docs claim cursor-based pagination for list endpoints
  - REST handlers use `limit`/`offset`; gRPC uses `page_token`/`page_size`
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/*/handlers.go`, `pkg/api/server/controlplane_server.go`

- [ ] **REST API error response format differs from docs**
  - Resolution: code
  - Docs show `{error, message, details}` schema
  - REST handlers return `{"error": "<message>"}` only
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/*/handlers.go`

- [ ] **REST auth model in docs does not match implementation**
  - Resolution: code
  - Docs show API key/JWT auth for REST endpoints
  - HTTP server has no REST auth middleware; only gRPC interceptors are wired
  - Reference: `docs/content/en/docs/reference/api.md`, `cmd/kscore-server/main.go`, `pkg/api/auth/*`

- [ ] **gRPC package names in API docs do not match proto**
  - Resolution: everything is v1
  - Docs use `package kscore.api.v1`/`v2`
  - Protos use `package keystone.core.v1`
  - Reference: `docs/content/en/docs/reference/api.md`, `api/proto/*.proto`

- [ ] **gRPC services documented but not generated/implemented**
  - Resolution: code
  - Docs list StateService, EventService, PolicyService, ClusterService RPCs
  - Only agent/controlplane/coordination gRPC stubs are generated in `pkg/api/v1`
  - No gRPC server implementations for state/event/policy/cluster
  - Reference: `docs/content/en/docs/reference/api.md`, `api/proto/*.proto`, `pkg/api/v1/*`

- [ ] **gRPC services exist but are not registered in server**
  - Resolution: code
  - `kscore-server` only registers ControlPlaneService
  - AgentService and CoordinationService are not registered on the gRPC server
  - Update docs or wire services
  - Reference: `cmd/kscore-server/main.go`, `pkg/api/v1/*`

- [ ] **Docs reference REST endpoints that do not exist**
  - Resolution: both
  - `/api/v1/status` is referenced but server exposes `/api/status`
  - `/api/v1/events/stats`, `/api/v1/events/stream` are documented but no handlers exist
  - `/api/v1/agents/{id}/metrics` and `/api/v1/agents/{id}/execute` appear in API versioning examples
  - `/api/v1/health` and `/api/v1/health/secrets` are referenced but only `/health/*` exists
  - `/api/v1/exclusions` appears in Windows ops docs but no API handler exists
  - References: `docs/content/en/docs/operations/proxy-agents.md`, `docs/content/en/docs/community/support.md`, `docs/content/en/docs/reference/sdk.md`, `docs/content/en/docs/operations/windows.md`, `docs/content/en/docs/operations/secrets-rotation.md`, `docs/content/en/docs/reference/secrets-api.md`

### Blueprint Documentation

- [ ] **Fix blueprint version references**
  - Resolution: code
  - Docs use: `kscore/demo@1.0.0`, `kscore/production-cluster@2.0.0`
  - Actual: All blueprints are version `0.1.0`

- [ ] **Update kscore/demo parameters**
  - Resolution: both
  - Documented but not implemented: `api_port`, `metrics_port`, `data_dir`, `log_level`
  - Implemented but not documented: `hostname`, `enable_examples`, `enable_dashboards`

- [ ] **Update kscore/production-cluster parameters**
  - Resolution: both
  - Multiple default value mismatches
  - Features don't match (docs: `etcd_clustering`, code: `nats_cluster`)
  - `tls_mode` enum values differ

### Operations Documentation Gaps

- [ ] **Registry ops docs imply object storage backends not supported by code**
  - Resolution: code
  - Docs describe shared storage options including GCS/S3/Azure Files and S3 cross-region replication
  - `kscore-registry` only reads/writes from a local filesystem data directory (no object storage integration or replication support)
  - Update docs to clarify external replication only, or implement storage backend support
  - Reference: `docs/content/en/docs/operations/registry.md`, `cmd/kscore-registry/main.go`

- [x] **API key CLI points to REST endpoints that are not implemented** *(DONE)*
  - Implemented `pkg/api/apikeys/` package with create/list/revoke handlers
  - Wired into kscore-server HTTP mux

- [ ] **NATS mesh operations doc references missing commands/flags**
  - Resolution: code
  - `kscore-agent nats status/buffer/leaf`, `kscore-agent restart`
  - `kscorectl debug nats events/timeline/trace/diagnose`
  - `kscorectl agent update-certs` and `kscorectl agent list --group-by`
  - None of these subcommands/flags exist in the CLI
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/nats-mesh-operations.md`, `cmd/kscore-agent/main.go`, `cmd/kscore-agents/main.go`

- [ ] **Proxy agents docs reference a proxy agent binary/flag that doesn't exist**
  - Resolution: code
  - Docs use `kscore-agent --proxy` and deploy `kscore-proxy-agent`
  - No `--proxy` flag or `kscore-proxy-agent` binary exists; only `kscore-agent` and `kscore-proxy` CLI are present
  - Update docs or implement proxy agent binary/flag
  - Reference: `docs/content/en/docs/operations/proxy-agents.md`, `cmd/kscore-agent/main.go`, `cmd/kscore-proxy/main.go`

- [ ] **File distribution ops docs list backend types not supported by kscore-files**
  - Resolution: code
  - Docs show `s3`, `gcs`, `azure`, `git`, and `nats` backends
  - `kscore-files` only allows `filesystem` in `createBackend`
  - Update docs or wire in additional backend types
  - Reference: `docs/content/en/docs/operations/file-distribution.md`, `cmd/kscore-files/main.go`, `internal/files/backend/*`

- [ ] **File distribution ops docs reference missing backend maintenance commands**
  - Resolution: code
  - Docs use `kscore-files backend gc`, `backend test`, and `backend benchmark`
  - CLI only supports backend list/status/sync/enable/disable/health
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/file-distribution.md`, `cmd/kscore-files/commands_backend.go`, `cmd/kscore-files-storage/main.go`

- [ ] **File distribution ops docs use unsupported mirrors flags**
  - Resolution: code
  - Docs use `mirrors sync-status --group`, `mirrors sync --group`, `mirrors conflicts --id`, and `--verbose`
  - CLI expects positional group ID and has no `--id`/`--verbose` flags for these commands
  - Update docs or add flags
  - Reference: `docs/content/en/docs/operations/file-distribution.md`, `cmd/kscore-files/commands_mirrors.go`

- [ ] **Cluster management docs use non-existent exec limit flag**
  - Resolution: code
  - Docs use `kscorectl exec run --limit 3 ...`
  - `kscore-exec` has no `--limit` flag
  - Update docs or implement flag
  - Reference: `docs/content/en/docs/operations/cluster-management.md`, `cmd/kscore-exec/main.go`

- [ ] **Deployment docs reference a missing state list command**
  - Resolution:code
  - Docs use `kscorectl state list` to verify state availability
  - `kscore-state` has no `list` subcommand
  - Update docs or implement command
  - Reference: `docs/content/en/docs/operations/deployment.md`, `cmd/kscore-state/main.go`

- [ ] **File backends reference does not match kscore-files config schema**
  - Resolution: both
  - Docs use a single `backend:` block with `type: local|s3|gcs|azure|git|nats`, and backend-specific keys like `root`, `temp_dir`
  - `kscore-files` expects `backends: []` with `name/type/paths/root_path/read_only` and only supports `filesystem` in `createBackend`
  - Update reference docs or align config parsing
  - Reference: `docs/content/en/docs/reference/file-backends.md`, `cmd/kscore-files/main.go`, `internal/files/backend/*`

- [ ] **Secrets operations docs reference non-existent kscore-secrets subcommands**
  - Resolution: code
  - Docs use `kscorectl secrets get/health/backend/auth/cache/lease` and backend updates/tests
  - `kscore-secrets` plugin only implements rotation/schedule/policy commands
  - Update docs or implement missing commands
  - Reference: `docs/content/en/docs/operations/secrets-backends.md`, `docs/content/en/docs/operations/secrets-troubleshooting.md`, `cmd/kscore-secrets/main.go`

### Tutorials/Concepts/Scenarios Documentation Gaps

- [ ] **Secrets quickstart tutorial uses commands not implemented in kscore-secrets/exec**
  - Resolution: both
  - Uses `kscorectl secrets get/put/list/audit/stats/injection/refresh` and `secrets rotation` subcommands
  - `kscore-secrets` only implements `rotate`, `schedule`, and `policy` (no get/put/list/audit/stats/injection/refresh; no `rotation` command alias)
  - Uses `kscorectl exec myapp -- ...` (no `exec` subcommand; should be `exec run`)
  - Update tutorial or implement missing commands
  - Reference: `docs/content/en/docs/tutorials/secrets-quickstart.md`, `cmd/kscore-secrets/main.go`, `cmd/kscore-exec/main.go`

- [ ] **Events concept docs reference non-existent command names/subcommands**
  - Resolution: both
  - Docs use `kscorectl event ...` (plugin is `kscore-events`, invoked as `kscorectl events`)
  - Commands like `event analyze`, `event subscribers`, `event storage-stats`, `event prune`, `event archive` are not implemented
  - Flags like `--until` and `--show-sequence`, and multi-value `--severity` lists are not supported (CLI uses `--before` and a single min severity)
  - Update docs or add commands/aliases
  - Reference: `docs/content/en/docs/concepts/events.md`, `cmd/kscore-events/main.go`

- [ ] **GitOps concept docs reference commands not in kscore-gitops**
  - Resolution: code
  - Docs use `kscorectl git-sync ...` (22+ references), `kscorectl verify ...`, `kscorectl logs ...`, top-level `rollback`/`promote`, and `approvals` commands
  - `kscore-gitops` does not implement `git-sync` or a top-level `verify`/`logs` command; rollbacks/promotions live under `kscorectl gitops ...`
  - Approvals are under `kscorectl runbook approvals`
  - Requires implementing git-sync subcommands or removing ~500 lines of conflict resolution documentation
  - Reference: `docs/content/en/docs/concepts/gitops.md`, `cmd/kscore-gitops/main.go`, `cmd/kscore-runbook/main.go`

- [ ] **Compliance scenario references a CLI that does not exist**
  - Resolution: code
  - Docs use `kscorectl compliance ...` commands - should be `kscorectl policy ...`
  - Commands exist (`policy report`, `policy compliance`, `policy check`) but with different flags
  - Doc flags: `--framework`, `--target`, `--from`, `--to`, `--format html/pdf`
  - Code flags: `--days`, `--output (text/json/yaml/table)`
  - Also uses `kscorectl logs --reactor` which doesn't exist
  - Requires implementing doc flags or rewriting entire scenario
  - Reference: `docs/content/en/docs/scenarios/compliance-automation.md`, `cmd/kscore-policy/main.go`

- [ ] **Blueprint apply commands are documented but not implemented**
  - Resolution: code (changed from doc - requires implementation of apply command with --var and --target support)
  - Docs use `kscorectl blueprint apply ...` with variables and targeting in scenarios and concepts
  - `kscore-blueprint` has no `apply` subcommand (install only downloads locally)
  - Need to implement apply command that deploys blueprint to targets with variable substitution
  - Reference: `docs/content/en/docs/scenarios/_index.md`, `docs/content/en/docs/scenarios/database-ha.md`, `docs/content/en/docs/concepts/blueprints.md`, `cmd/kscore-blueprint/main.go`

### Runbook Documentation Gaps

- [ ] **Runbooks reference many CLI commands that don't exist**
  - Resolution: code
  - Over 20 non-existent commands across multiple runbook files
  - `kscorectl auth login/revoke-all/sessions/rotate-signing-key`
  - `kscorectl debug db-status`/`debug connections`
  - `kscorectl cluster token/quorum/election restart`
  - `kscorectl cluster uncordon` (command is `undrain`), `cluster member remove`, `cluster health --node`
  - `kscorectl federation status/ping/trust list`
  - `kscorectl audit search/analyze/timeline`
  - `kscorectl backup status`
  - `kscorectl diagnostics collect`
  - `kscorectl config set`, `kscorectl db compact`, `kscorectl db rotate-credentials`, `kscorectl nats rotate-credentials`
  - `kscorectl secrets rotate-keys`, `kscorectl user delete`, `kscorectl security scan`
  - `kscorectl upgrade resume/path` and agent-level `--retry/--skip` options
  - `kscorectl agent list --show-version` (no such flag; only `--show-compatibility`)
  - `kscore-bootstrap init` (bootstrap CLI uses `seed/restore/import/...`)
  - `kscorectl state list/status/update` (no such state subcommands)
  - Requires implementing many CLI commands or extensive runbook rewrites
  - Reference: `docs/runbooks/*.md`, `cmd/kscorectl/main.go`, `cmd/kscore-cluster/main.go`, `cmd/kscore-audit/main.go`, `cmd/kscore-backup/main.go`, `cmd/kscore-federation/main.go`, `cmd/kscore-upgrade/main.go`

### Reference/Community Documentation Gaps

- [ ] **NATS mesh reference doc calls missing debug commands**
  - Resolution: code
  - Docs use `kscorectl debug nats status/events/timeline/trace/diagnose/export`
  - No `debug` command exists
  - Update docs or implement debug CLI
  - Reference: `docs/content/en/docs/reference/nats-mesh.md`, `docs/content/en/docs/concepts/nats-mesh.md`, `cmd/kscorectl/main.go`

- [ ] **NATS mesh reference config does not match config structs**
  - Resolution: code
  - Docs use `server.nats.*` and `agent.nats.*` with gateway/websocket/routing/endpoints blocks
  - Code only supports top-level `nats.*` with limited embedded/jetstream settings
  - Update reference docs or expand config schema
  - Reference: `docs/content/en/docs/reference/nats-mesh.md`, `internal/config/config.go`

- [ ] **CLI quick reference lists module commands that do not exist**
  - Resolution: both
  - Docs list `module list/show/update/uninstall`
  - `kscore-module` only supports init/validate/build/resolve/tree/verify/sign/test/publish/install
  - Docs list `test suite list`; `kscore-test` uses `test list` for suites (no `suite list`)
  - Update quick reference or implement commands
  - Reference: `docs/content/en/docs/reference/cli-quick-reference.md`, `docs/content/en/docs/reference/cli.md`, `cmd/kscore-module/main.go`, `cmd/kscore-test/main.go`

- [ ] **Runbook automation reference doc lists commands not in kscore-runbook**
  - Resolution: both
  - Docs use `kscorectl runbook execute/status/list-executions/test/audit`
  - `kscore-runbook` only supports approvals/interventions flows
  - Update docs or implement runbook execution CLI
  - Reference: `docs/content/en/docs/reference/runbook-automation.md`, `cmd/kscore-runbook/main.go`

- [ ] **FAQ uses unsupported state apply verbosity flag**
  - Resolution: c ode
  - Docs use `kscorectl state apply myconfig.yaml -v`
  - `kscore-state` has no `-v/--verbose` flag
  - Update docs or implement flag
  - Reference: `docs/content/en/docs/community/faq.md`, `cmd/kscore-state/main.go`

### Makefile Target Documentation

- [ ] **Document cross-platform build targets**
  - Resolution: code
  - `make build-linux` - Linux amd64/arm64
  - `make build-darwin` - macOS amd64/arm64
  - `make build-windows` - Windows amd64

- [ ] **Document repository generation targets**
  - Resolution: code
  - `make repos`, `make repo-gen`
  - `make repos-dnf`, `make repos-apt`, `make repos-windows`
  - `make repos-blueprints`, `make repos-modules`

---

## Low Priority (Enhancements)

### Missing Makefile Convenience Targets

- [x] `make fmt` - Run gofmt *(DONE)*
- [x] `make lint-fix` - Run golangci-lint --fix *(DONE)*
- [x] `make check` - Pre-commit checks (fmt, lint, test) *(DONE)*
- [ ] `make dev` - Hot reload development with air
- [x] `make test-verbose` - Verbose test output (alias for test) *(DONE)*
- [x] `make test-coverage` - Generate coverage reports *(DONE)*
- [x] `make test-integration` - Run integration tests *(DONE)*
- [x] `make benchmark` - Run benchmarks *(DONE)*

### CLI Command Coverage

- [ ] **Implement --format flag for events watch command**
  - Resolution: code
  - Docs claim `kscorectl events watch --format jsonl`, but command has no `--format` flag
  - Implement `--format` flag with text, json, jsonl options
  - Also add missing `--filter` and `--tag` flags to docs (these exist in code)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [ ] **Add missing events query flags**
  - Resolution: code
  - Command exists at `cmd/kscore-events/main.go:newQueryCmd` (line 202)
  - Code only has `--limit` flag; docs also show `--since` and `--until`
  - Implement `--since` and `--until` time range flags
  - Also: docs show `-n` alias for --limit which doesn't exist (doc issue)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [ ] **Implement missing events list filters**
  - Resolution: code
  - Flags exist for --severity, --since, --before, --tag but filterEvents() ignores them
  - filterEvents() only uses type, source, correlation-id (lines 1013-1041)
  - Implement severity, time range, and tag filtering in filterEvents()
  - Reference: `cmd/kscore-events/main.go:filterEvents`

- [ ] **Implement missing events DLQ flags**
  - Resolution: code
  - Implement `dlq list --reason` filter and `-n` alias for --limit
  - Implement `dlq retry --type` to retry by event type
  - Implement `dlq purge --older-than` and `--reason` filters
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [ ] **Implement missing upgrade command features**
  - Resolution: code
  - Add `[upgrade-id]` positional arg to `status`, `cancel`, `logs` commands
  - Add `--confirm` flag to `canary promote` and `canary rollback`
  - Add `agents list` and `agents status` subcommands (currently single command with --report)
  - Add `--status` filter to `history` command
  - Note: `plan --batch-size/--save` and `execute --plan/--confirm/--async` already exist
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-upgrade/main.go`

- [ ] **Implement cluster members --filter flag**
  - Resolution: code
  - Docs show `--filter` to filter by status (healthy, degraded, unhealthy)
  - Code only has `--details` flag
  - Implement `--filter` flag in addition to existing `--details`
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-cluster/commands.go`

- [ ] **Implement cluster rebalance --target flag**
  - Resolution: code
  - Docs show `--target` to specify member to rebalance from
  - Code only has `--reason` and `--dry-run`
  - Implement `--target` flag; also add `--reason` to docs
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-cluster/commands.go`

- [ ] **Implement proxy discover scan flags**
  - Resolution: code
  - Docs show `--subnet/--ports/--protocols/--timeout/--workers`
  - Code only has `--network/--networks/--debug`
  - Implement the documented flags (rename --subnet to --network for consistency)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [ ] **Implement proxy drift/state missing features**
  - Resolution: code
  - Implement `drift check --severity` filter (docs show it, useful feature)
  - Implement `state check` command (docs show it, useful for compliance checking)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

### File Distribution Docs vs CLI

- [ ] **Align file distribution docs with actual files CLI**
  - Resolution: code
  - Docs use `namespace/path` syntax for files, but CLI requires `--namespace <name> <path>` separately
  - Some commands fixed (list/delete/info), but namespace path syntax is a structural difference
  - Requires implementing namespace path syntax support (e.g., `namespace:/path`) or extensive doc rewrites
  - Reference: `docs/content/en/docs/concepts/file-distribution.md`, `cmd/kscore-files/*`

- [ ] **File server configuration docs include unsupported sections**
  - Resolution: both
  - `server.rate_limit`, `access`, `mirror_groups`, and `cache` are documented but not parsed by `kscore-files`
  - `nats` connection settings are missing from files server docs (only NATS backend bucket config is shown)
  - Update docs or wire config into server
  - Reference: `docs/content/en/docs/reference/configuration.md`, `cmd/kscore-files/main.go`

- [ ] **Proxy agent configuration docs include unsupported discovery/profiles**
  - Resolution: both
  - Docs include `discovery.*` and `profiles` sections for proxy agents
  - `internal/proxy.Config` has no discovery or profile fields (discovery config exists separately and is not wired)
  - Docs list `credentials.provider: env`, but only vault/kubernetes/file are implemented
  - Update docs or wire configuration into proxy agent
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/proxy/config.go`, `internal/proxy/discovery/*`

- [ ] **Fix file backend option names in docs**
  - Resolution: both
  - Docs use `gcs.project_id` but code expects `gcs.project`
  - Docs use `azure.account` and `azure.access_key`; code expects `account_name` and `account_key`
  - Docs use `nats.bucket` and `nats.storage`; code expects `bucket_name` and has no `storage` option
  - Docs use `git.auth` block with `sync_interval/auto_commit/commit_author`; code expects `username/password`, `auto_pull`, `pull_interval`, and `ssh_key_file`
  - Docs describe backend interface methods `Read/Write/Hash`, but code uses `Get/Put/Stat` etc.
  - Reference: `docs/content/en/docs/reference/file-backends.md`, `internal/files/backend/*.go`

- [ ] **HTTP file backend is declared but not implemented or documented**
  - Resolution: code
  - `BackendTypeHTTP` is defined but there is no `http` backend implementation
  - Docs do not mention an HTTP backend
  - Implement backend or remove type from API surface
  - Reference: `internal/files/backend/backend.go`, `docs/content/en/docs/reference/file-backends.md`

### Loadtest/Test CLI Docs vs Code

- [ ] **Align kscore-test flags and suite commands**
  - Resolution: code
  - Docs omit many flags (`--tags`, `--parallel`, `--fail-fast`, `--type` for list, `--status` for history)
  - Docs list `test suite list` but code only has `suite show/create/delete`
  - Update docs or add missing commands/flags
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-test/main.go`

### Exec CLI Docs vs Code

- [ ] **Document missing kscore-exec commands and flags**
  - Resolution: both
  - Docs cover run/status/list/shell/script only
  - Code also includes `async`, `cancel`, `history`, and `output` subcommands
  - Docs omit run flag `--dry-run` and list/history filters (`--since`, `--before`, etc.)
  - TLS flags in code (`--tls`, `--tls-ca-cert`, `--tls-cert`, `--tls-key`, `--tls-server-name`) are not documented
  - Update docs or remove/alias commands
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-exec/main.go`

### Non-CLI Docs Referencing Missing Commands

- [ ] **Runbooks reference many missing auth/cluster/config/diagnostics/certs/agent commands**
  - Resolution: doc
  - Examples: `kscorectl auth login`, `cluster token/quorum/health/uncordon`, `config set`, `diagnostics collect`, `certs *`, `agent quarantine/unquarantine/verify/ping`
  - Several runbooks rely on `audit search/export/analyze/timeline` and `security scan` commands not in CLI
  - Runbooks reference `backup status` and agent list flags like `--show-version`, `--show-cert-expiry`, `--count`, `--suspicious`
  - Update runbooks or implement commands
  - Reference: `docs/runbooks/*`

- [ ] **Concepts policy docs reference missing policy subcommands**
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

- [ ] **Concepts reactors docs reference missing reactor/logs commands**
  - Resolution: doc
  - Docs use `kscorectl reactor list/status/stats/history` and `kscorectl logs --filter`
  - No reactor or logs CLI exists
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/concepts/reactors.md`

- [ ] **Concepts GitOps docs reference missing git-sync/rollback/verify/approvals commands**
  - Resolution: doc
  - Docs use `kscorectl rollback *`, `kscorectl promote`, `kscorectl verify *`, `kscorectl approvals *`
  - Docs use `kscorectl git-sync *` subcommands and `kscorectl logs --filter`
  - Docs use `kscorectl config validate` (no config CLI)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/concepts/gitops.md`

- [ ] **Concepts identity docs reference missing identity/agent commands**
  - Resolution: doc
  - Docs use `identity ca backups list`, `identity metrics`, `identity cache stats`, and `identity events --follow` (not implemented)
  - Docs use `agent renew-svid` (not implemented)
  - Update docs or implement commands/flags
  - Reference: `docs/content/en/docs/concepts/identity.md`

- [ ] **Concepts secrets management docs reference missing secrets compliance command**
  - Resolution: doc
  - Docs use `kscorectl secrets compliance report` (not implemented)
  - Update docs or implement command
  - Reference: `docs/content/en/docs/concepts/secrets-management.md`

- [ ] **Concepts service mesh docs reference missing agent/identity commands**
  - Resolution: doc
  - Docs use `kscorectl agents get` (CLI uses `agents show`)
  - Docs use `kscorectl identity federation list` (federation is a separate plugin: `kscorectl federation list`)
  - Update docs or implement aliases
  - Reference: `docs/content/en/docs/concepts/service-mesh.md`

- [ ] **Concepts observability docs reference missing logs command**
  - Resolution: doc
  - Docs use `kscorectl logs ...` for correlation/tail queries
  - No logs CLI exists
  - Update docs or implement logs plugin
  - Reference: `docs/content/en/docs/concepts/observability.md`

- [ ] **Concepts observability docs import non-existent query package**
  - Resolution: doc
  - Docs import `github.com/shawnbutts/keystone-core/pkg/query`
  - Code is `internal/query` only
  - Update docs or promote package
  - Reference: `docs/content/en/docs/concepts/observability.md`

- [ ] **Concepts state storage docs reference missing migrate/backup flags**
  - Resolution: doc
  - Docs use `kscorectl cluster-backup create --type database` (CLI uses `cluster-backup backup` and no `--type` flag)
  - Docs use `migrate run --resume` and `--skip-errors/--log-errors` (CLI uses `--continue-on-error` and no log/errors flags)
  - Update docs or implement flags
  - Reference: `docs/content/en/docs/concepts/state-storage.md`

- [ ] **Community docs reference missing commands**
  - Resolution: doc
  - Support docs use `state list-modules` and `agent status` (not implemented)
  - Release notes/announcement use `bootstrap init --mode embedded` and `blueprint apply` (not implemented)
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/community/support.md`, `docs/content/en/docs/community/release-notes.md`, `docs/content/en/docs/community/announcement-0.1.0.md`

- [ ] **Community development docs reference removed pkg paths**
  - Resolution: doc
  - Docs recommend tests/debugging under `./pkg/state`, `./pkg/events`, `./pkg/statemgmt`, and `./pkg/agent`
  - These packages moved under `internal/*` (no public pkg equivalents)
  - Update docs or re-export packages
  - Reference: `docs/content/en/docs/community/development.md`, `docs/content/en/docs/community/windows-development.md`

- [ ] **Blueprint reference docs reference missing blueprint subcommands**
  - Resolution: doc
  - Docs use `blueprint bundle *`, `blueprint mirror *`, and `blueprint snapshot show`
  - CLI lacks bundle/mirror commands and snapshot show
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/reference/blueprints.md`

- [ ] **Module reference doc counts do not match code**
  - Resolution: doc
  - Docs claim "94 built-in state modules"
  - Code registers 101 modules (`NewBaseModule(...)` occurrences in `internal/statemgmt`)
  - Update docs or reconcile module list/count
  - Reference: `docs/content/en/docs/reference/modules.md`, `internal/statemgmt/*`

- [ ] **Module reference lists modules that are not registered**
  - Resolution: doc
  - Docs list docker/podman, database (postgres/mysql/redis), system (timezone/locale/hostname/hosts/sysctl/kernel_module), storage (mount/swap/lvm/disk/filesystem), git, SSH, Kubernetes, and cert (x509/ca/acme) modules
  - These modules exist in code but have no `RegisterModule(...)` calls, so they are not available in the default registry
  - Update docs or register modules
  - Reference: `docs/content/en/docs/reference/modules.md`, `internal/statemgmt/*`

- [ ] **Kubernetes concept docs overstate operator/CRD support**
  - Resolution: doc
  - Docs describe controllers that watch CRDs and reconcile RemoteExecution/StateConfig
  - `internal/k8s/controller.go` is stubbed (no informers, reconcile is no-op) and there is no operator binary/deployment wiring
  - Update docs or implement operator runtime and wiring
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/controller.go`

- [ ] **Kubernetes concept docs claim context switching that is not implemented**
  - Resolution: doc
  - Docs mention kubeconfig context switching; `ClusterConfig.Context` is never used in `NewClient`
  - Update docs or implement context selection
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/client.go`, `internal/k8s/types.go`

- [ ] **Kubernetes concept docs CRD schemas do not match code**
  - Resolution: doc
  - RemoteExecution example uses `target.labelSelector.matchLabels`, `namespaces`, `command.shell/script`, `retries`; code expects `Target` as `PodSelector` and `Command` as `[]string` with no retries
  - StateConfig example uses `id`, `state`, `driftDetection`; code expects `StateDeclaration{Name, Module, Parameters, Requisites}` and has no drift fields in CRD
  - Update docs or adjust CRD types/controllers
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/types.go`

- [ ] **Windows installation docs reference missing agent commands/flags**
  - Resolution: doc
  - Docs use `kscore-agent.exe install`, `kscore-agent.exe uninstall`, and `--console`
  - Agent CLI only supports `service-install`, `service-uninstall`, `service-start`, `service-stop`, `service-status`; no `install/uninstall` or `--console`
  - Update docs or add aliases/flags
  - Reference: `docs/content/en/docs/operations/windows-installation.md`, `cmd/kscore-agent/main.go`

- [ ] **Windows operations docs use unsupported exec flags**
  - Resolution: doc
  - Docs use `kscorectl exec run --shell powershell/pwsh`
  - `exec run` has no `--shell` flag (only `exec shell` has `--shell`)
  - Update docs or add flag
  - Reference: `docs/content/en/docs/operations/windows.md`, `cmd/kscore-exec/main.go`

- [ ] **Registry ops docs reference metrics not exposed by registry**
  - Resolution: doc
  - Docs include Prometheus scrape config and `registry_storage_bytes`/HTTP latency metrics
  - `kscore-registry` does not expose `/metrics` or registry-specific metrics
  - Update docs or implement metrics endpoint
  - Reference: `docs/content/en/docs/operations/registry.md`, `cmd/kscore-registry/main.go`

- [ ] **CLI reference docs include non-existent flags/commands**
  - Resolution: doc
  - Docs mention `kscorectl config validate` and `kscorectl --show-deprecated` (not implemented)
  - Docs suggest `exec batch` and Salt-style aliases that don't exist
  - Update docs or implement commands/flags
  - Reference: `docs/content/en/docs/reference/cli.md`

- [ ] **Reference NATS mesh docs reference missing debug plugin**
  - Resolution: doc
  - Docs use `kscorectl debug nats ...` (no debug CLI)
  - Update docs or implement debug plugin
  - Reference: `docs/content/en/docs/reference/nats-mesh.md`

- [ ] **NATS mesh deployment ops docs use unsupported config/commands**
  - Resolution: doc
  - Docs use `nats.urls` list in kscore config; code supports a single `nats.url`
  - Docs reference `kscorectl agent update-config --nats-urls` (no such command)
  - Update docs or implement config/CLI support
  - Reference: `docs/content/en/docs/operations/nats-mesh-deployment.md`, `internal/config/config.go`, `cmd/kscore-agents/main.go`

- [ ] **Query API reference docs mismatch with code package/export**
  - Resolution: doc
  - Docs reference `github.com/keystone-core/pkg/query` and public `PrometheusQuerier/LokiQuerier` APIs
  - Code lives under `internal/query` (not exported as public SDK) and has no `pkg/query`
  - Update docs or promote package to public API
  - Reference: `docs/content/en/docs/reference/query-api.md`, `internal/query/*`

- [ ] **Query API docs describe HTTP API that is not implemented**
  - Resolution: doc
  - No query endpoints in `kscore-server` or API handlers for metrics/logs/traces queries
  - Update docs or add server/API routes
  - Reference: `docs/content/en/docs/reference/query-api.md`, `cmd/kscore-server/main.go`

- [ ] **Query API docs mention log pagination cursor not supported**
  - Resolution: doc
  - Docs describe `LogsQuery.start` cursor; Loki querier ignores it
  - Update docs or implement pagination
  - Reference: `docs/content/en/docs/reference/query-api.md`, `internal/query/logs.go`

- [ ] **Metrics reference docs claim /metrics on control plane**
  - Resolution: code
  - Control plane HTTP mux only exposes health and `/api/status` (no `/metrics`)
  - Metrics collectors exist but are not wired to an HTTP endpoint in `kscore-server`
  - Update docs or expose `/metrics`
  - Reference: `docs/content/en/docs/reference/metrics.md`, `cmd/kscore-server/main.go`, `internal/metrics/*`

- [ ] **Profiling docs reference config/pprof endpoints not wired**
  - Resolution: doc
  - Docs describe `profiling:` config blocks and `/debug/pprof` endpoints for server/agent/gateway
  - `internal/profiling` exists but is not integrated into configs or started by binaries
  - Update docs or wire profiling server into binaries/config
  - Reference: `docs/content/en/docs/operations/monitoring.md`, `docs/content/en/docs/concepts/observability.md`, `internal/profiling/*`, `internal/config/*`, `cmd/kscore-server/main.go`

- [ ] **Metrics catalog does not match implemented metrics**
  - Resolution: doc
  - Docs list many metrics not present in code (DB, batch, detailed NATS mesh, policy compliance, command target count, etc.)
  - Code exports metrics not documented (e.g., `kscore_action_failures_total`, `kscore_active_subscribers`, `kscore_events_received_total`, `kscore_event_rate`, `kscore_proxy`, `kscore_files`)
  - Update docs to match emitted metrics or implement missing metrics
  - Reference: `docs/content/en/docs/reference/metrics.md`, `internal/metrics/*`, `internal/events/prometheus.go`, `internal/nats/observability.go`, `internal/proxy/observability/metrics.go`, `internal/files/*`

- [ ] **Metrics reference types/labels don't match implementation**
  - Resolution: doc
  - Example: `kscore_api_request_duration_seconds` documented as Summary with `path` label; code defines Histogram with `endpoint`
  - Review other metric type/label differences and align docs or code
  - Reference: `docs/content/en/docs/reference/metrics.md`, `internal/metrics/collectors.go`

- [ ] **Visualization API docs reference server not wired into control plane**
  - Resolution: doc
  - Visualization server exists under `internal/visualization` but is not started by any binary
  - Docs imply `/api/topology` and `/ws/topology` are available
  - Update docs or wire visualization server into `kscore-server`
  - Reference: `docs/content/en/docs/reference/visualization.md`, `internal/visualization/*`

- [ ] **Visualization docs reference non-existent Go package**
  - Resolution: doc
  - Docs import `github.com/keystone-core/pkg/visualization`
  - Code is under `internal/visualization` (not exported)
  - Update docs or promote package to public API
  - Reference: `docs/content/en/docs/reference/visualization.md`, `internal/visualization/*`

- [ ] **Visualization docs include fields not present in API structs**
  - Resolution: doc
  - Example responses include `labels` on agents, but `internal/visualization.Agent` has no `Labels` field
  - Update docs or extend visualization agent model
  - Reference: `docs/content/en/docs/reference/visualization.md`, `internal/visualization/types.go`

- [ ] **CLI quick reference docs reference missing commands**
  - Resolution: doc
  - Docs include `kscorectl config validate`, `agent status/quarantine/unquarantine/renew-svid`, `health check`, `event list --agent`, and `policy test`
  - CLI does not implement these commands/flags
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/reference/cli-quick-reference.md`

- [ ] **Additional scenarios reference missing exec/compliance/cluster flags**
  - Resolution: doc
  - Compliance scenario uses `kscorectl compliance *` and `policy test/evaluate` plus `logs --reactor` (not implemented)
  - Database HA and multi-tier/hybrid scenarios use `kscorectl exec ... --cmd` shorthand (not implemented) and `blueprint apply` (missing)
  - Hybrid scenario uses `agents list --group-by` and `connectivity test` (not implemented)
  - Disaster recovery uses `event emit` (singular) and `cluster status --endpoint` (unsupported flag)
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/scenarios/compliance-automation.md`, `docs/content/en/docs/scenarios/database-ha.md`, `docs/content/en/docs/scenarios/multi-tier-webapp.md`, `docs/content/en/docs/scenarios/hybrid-infrastructure.md`, `docs/content/en/docs/scenarios/disaster-recovery.md`

- [ ] **Edge deployment scenario uses unsupported agent config keys**
  - Resolution: doc
  - Scenario `agent.yaml` uses `server.urls`, `reconnect.*`, `cache.*`, `offline.*`, and `proxy.*`
  - `internal/config.AgentConfig` has no such fields (agent config only includes ID, heartbeat, timeouts, metadata interval, address family, labels, advertise_addrs)
  - Update scenario docs or implement config support
  - Reference: `docs/content/en/docs/scenarios/edge-deployment.md`, `internal/config/config.go`

- [ ] **GitOps/event scenarios use config files not supported by control plane**
  - Resolution: doc
  - GitOps scenario uses `gitops.webhook/sync/verification` config blocks
  - Event-driven automation scenario defines `events:` schemas under `config/events/*.yaml`
  - `internal/config.Config` has no gitops or events config sections
  - Update scenarios or implement config support
  - Reference: `docs/content/en/docs/scenarios/gitops-workflow.md`, `docs/content/en/docs/scenarios/event-driven-automation.md`, `internal/config/config.go`

- [ ] **Edge and proxy concept docs reference missing cache/sync/proxy commands**
  - Resolution: doc
  - Edge docs use `kscorectl cache *`, `kscorectl sync *`, `kscorectl connection test`, and `agent status --edge` (not implemented)
  - Proxy concept docs use `proxy credential create/verify`, `proxy discovery *` (CLI uses `proxy discover *`), `proxy device ping/status/connect/config show`, `proxy discovery logs/config show`, and `proxy state apply --device`
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/concepts/edge.md`, `docs/content/en/docs/concepts/proxy-agents.md`

- [ ] **Tutorial drift detection docs reference missing commands/flags**
  - Resolution: doc
  - Docs use `kscorectl apply` (no top-level apply; should be `state apply`)
  - Docs use `events list --last 24h` (CLI uses `--since`)
  - Update docs or implement aliases
  - Reference: `docs/content/en/docs/tutorials/drift-detection.md`

- [ ] **Tutorial secrets quickstart docs reference missing secrets/exec commands**
  - Resolution: doc
  - Docs use `secrets health/put/get/list/audit/stats/policy/refresh/injection` and `secrets rotation ...` (not implemented; CLI uses `secrets rotate`)
  - Docs use `kscorectl exec <target> -- ...` (no exec shorthand; should be `exec run`)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/tutorials/secrets-quickstart.md`

- [ ] **Self-management docs reference missing diagnostics/certs commands**
  - Resolution: doc
  - Docs use `kscorectl diagnostics collect` and `kscorectl certs status/rotate/verify`
  - No diagnostics/certs plugin found in codebase
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [ ] **Self-management docs reference missing cluster token command**
  - Resolution: doc
  - Docs use `kscorectl cluster token` during bootstrap import
  - `kscore-cluster` has no `token` subcommand
  - Update docs or implement token command
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [ ] **Self-management docs reference missing agent ping command**
  - Resolution: doc
  - Docs use `kscorectl agent ping --all`
  - `kscore-agents` has no `ping` subcommand
  - Update docs or implement ping
  - Reference: `docs/content/en/docs/operations/self-management.md`

- [ ] **Deployment docs reference missing migrate export/import**
  - Resolution: doc
  - Docs use `kscorectl migrate export` and `kscorectl migrate import`
  - Code only implements `kscorectl migrate run` and `kscorectl migrate validate`
  - Update docs or implement subcommands
  - Reference: `docs/content/en/docs/operations/deployment.md`

- [ ] **Deployment docs reference missing state list command**
  - Resolution: doc
  - Docs use `kscorectl state list`
  - `kscore-state` has no `list` subcommand
  - Update docs or implement list
  - Reference: `docs/content/en/docs/operations/deployment.md`

- [ ] **Deployment docs use unsupported NATS config keys**
  - Resolution: doc
  - Example config uses `nats.embedded.jetstream.enabled` and `nats.embedded.jetstream.store_dir`
  - Code expects `nats.embedded.enable_jetstream` and separate `nats.jetstream.*` config
  - Update docs or add config alias support
  - Reference: `docs/content/en/docs/operations/deployment.md`, `internal/config/config.go`

- [ ] **Incident response doc references missing or mismatched CLI commands**
  - Resolution: doc
  - Commands not implemented: `api-key revoke-all`, `agent certificate rotate/regenerate`, `agent health`, `user reset-password`, `auth session invalidate-all`, `module block`, `module audit`
  - Commands/flags mismatched: `policy audit list --since` (no list subcommand or since filter), `audit query` (no query command), `files download` (CLI uses `files get`)
  - Update docs or implement commands/flags
  - Reference: `docs/project/INCIDENT-RESPONSE.md`, `cmd/kscorectl/main.go`, `cmd/kscore-agents/main.go`, `cmd/kscore-audit/main.go`, `cmd/kscore-policy/main.go`, `cmd/kscore-module/main.go`, `cmd/kscore-files/commands_files.go`

- [ ] **Scenarios index references missing blueprint apply command**
  - Resolution: doc
  - `kscorectl blueprint apply` is used but no apply subcommand exists
  - Update docs or implement apply
  - Reference: `docs/content/en/docs/scenarios/_index.md`

- [ ] **Module reference docs reference unsupported init template flag**
  - Resolution: doc
  - Docs use `kscorectl module init --template rust` but CLI only supports `--type` (starlark/wasm)
  - Update docs or implement template flag
  - Reference: `docs/content/en/docs/reference/modules.md`

- [ ] **DNS module docs reference secret_ref/env credentials not supported**
  - Resolution: doc
  - Docs show `credentials.secret_ref` and provider env vars for DNS records
  - DNS module only parses inline `api_key`/`api_token`/`account_id` and `credentials.extra`; no secret_ref/env resolution
  - Update docs or implement secret/env credential resolution
  - Reference: `docs/content/en/docs/reference/modules/dns.md`, `internal/statemgmt/module_dns.go`, `internal/dns/providers/*`

### Packaging Documentation

- [ ] **Update kscore-cli package contents**
  - Resolution: doc
  - Docs list only 4 tools in the `kscore-cli` package
  - Actual package includes many plugin binaries (agents, audit, backup, cluster, gitops, identity, etc.)
  - Update `docs/content/en/docs/getting-started/installation.md` and release guide as needed

### API Documentation Improvements

- [ ] **Document streaming gRPC methods**
  - Resolution: doc
  - `SubscribeEvents` (event streaming)
  - `WatchMembership` (cluster membership changes)
  - `WatchLeadership` (leadership changes)

- [ ] **Document CoordinationService**
  - Resolution: doc
  - Server-to-server coordination when NATS unavailable
  - 6 RPCs: `ClusterHealth`, `GetLeader`, `NATSStatus`, etc.

### Identity CLI Docs vs Code

- [ ] **Align kscore-identity token create flags and required args**
  - Resolution: doc
  - Docs show `token create` flags: `--path`, `--ttl`, `--uses`
  - Code requires `--agent-id`, supports `--ttl` and `--label`, and does not implement `--path`/`--uses`
  - Update docs or implement missing flags
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [ ] **Fix kscore-identity CA output/fields mismatch**
  - Resolution: doc
  - Docs list CA info fields like `SVIDs Issued`, `Last Rotation`, `Next Rotation`, `Auto-Rotation`
  - Code only prints Trust Domain + Root/Signing CA details
  - Update docs or implement additional fields
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [ ] **Align kscore-identity federation add flags**
  - Resolution: doc
  - Docs include `--type` (bidirectional/unidirectional) and mark `--endpoint` as required
  - Code supports `--endpoint`, `--profile`, `--refresh-interval` and does not implement `--type` or enforce required endpoint
  - Update docs or add flag/validation
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [ ] **Align kscore-identity bundle export formats**
  - Resolution: doc
  - Docs list `pem`, `jwks`, and `spiffe` formats
  - Code only supports `pem` and `jwks`
  - Update docs or implement `spiffe`
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

- [ ] **Fix kscore-identity global defaults**
  - Resolution: doc
  - Docs say `--audit-level` default is `errors` and `--audit-output` is system-dependent
  - Code defaults to `audit-level=all` and `audit-output=auto`
  - Update docs or change defaults
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-identity/main.go`

### Environment Variable Documentation

- [ ] **Verify documented but not found env vars**
  - Resolution: doc
  - `KSCORE_NATS_TLS_CERT`, `KSCORE_NATS_TLS_KEY`, `KSCORE_NATS_TLS_CA`
  - `KSCORE_NATS_DEBUG`, `KSCORE_NATS_CLUSTER`
  - `KSCORE_NATS_URL`, `KSCORE_NATS_URLS`, `KSCORE_NATS_MODE`
  - `KSCORE_NATS_CREDENTIALS`, `KSCORE_NATS_CREDS_FILE`
  - `KSCORE_NATS_USER`, `KSCORE_NATS_PASSWORD`
  - `KSCORE_NATS_BUFFER_SIZE`, `KSCORE_NATS_BUFFER_DIR`
  - `KSCORE_OUTPUT_FORMAT`

- [ ] **Document test/development env vars**
  - Resolution: doc
  - `KSCORE_LOAD_TEST`, `KSCORE_AGENT_COUNT`, `KSCORE_TEST_DURATION`
  - `KSCORE_TEST_POSTGRES_DSN`, `KSCORE_PERF_TESTS`
  - Mark as development/testing only

### Project Documentation

- [ ] **Project security docs reference missing public packages**
  - Resolution: doc
  - SECURITY-DESIGN and SECURITY-GOVERNANCE reference `pkg/security/*`, `pkg/auth/*`, `pkg/authz/*`, `pkg/crypto/*`, and `pkg/audit/*`
  - Code uses `internal/security` and `internal/audit`; auth/authz/crypto packages are not present
  - Update docs or export packages
  - Reference: `docs/project/SECURITY-DESIGN.md`, `docs/project/SECURITY-GOVERNANCE.md`, `internal/security/*`, `internal/audit/*`

- [ ] **Design docs reference internal paths as pkg**
  - Resolution: doc
  - DESIGN.md references `pkg/security/tls.go` but file lives in `internal/security/tls.go`
  - Update docs or move code
  - Reference: `docs/project/DESIGN.md`, `internal/security/tls.go`

- [ ] **Development docs use stale package paths**
  - Resolution: doc
  - Docs recommend `go test ./pkg/state/...` but `pkg/state` no longer exists (state code moved to `internal/state`)
  - Update docs or re-export packages
  - Reference: `docs/project/DEVELOPMENT.md`

- [ ] **Roadmap docs reference non-existent deprecation package path**
  - Resolution: doc
  - Roadmap claims `pkg/cli/deprecation/` exists
  - Deprecation framework lives under `internal/cli/deprecation`
  - Update docs or export package
  - Reference: `docs/content/en/docs/community/roadmap.md`, `internal/cli/deprecation/*`

- [ ] **Incident response doc references many non-existent CLI commands**
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

- [ ] **kscorectl config show** - Display current configuration with optional `--include-defaults` flag
- [ ] **kscorectl rbac list-roles** - List RBAC roles with optional `--show-permissions` flag
- [ ] **kscorectl rbac export** - Export RBAC configuration for backup/audit
- [ ] **kscore-audit query** - Query audit logs with filters (`--type`, `--since`, `--api-key`, `--agent`)
- [ ] **kscore-audit watch** - Real-time audit log monitoring with filters
- [ ] **kscore-agents re-enroll** - Re-enroll agent with new credentials (for security incidents)
- [ ] **kscore-agents revoke-credentials** - Revoke agent credentials without deletion

### Runbook CLI Commands

- [ ] **kscore-runbook execute** - Execute a runbook with inputs (`--input key=value`, `--dry-run`)
- [ ] **kscore-runbook status** - Check execution status of a running/completed runbook
- [ ] **kscore-runbook list-executions** - List runbook execution history (`--runbook`, `--since`)
- [ ] **kscore-runbook test** - Test runbook with mock handlers (`--mock-file`)
- [ ] **kscore-runbook audit list** - List runbook audit events (`--runbook`, `--start`, `--end`)
- [ ] **kscore-runbook audit report** - Generate compliance report (`--format`, `--start`, `--end`)

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
| Additional Documentation Drift | 93 |
| Low Priority | 90 |
| Future Work | 13 |
| **Total** | **207** |
