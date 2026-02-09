# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Resolution Tags

Each TODO item includes a `Resolution:` line to indicate how it should be addressed:

- `doc` — update documentation to match current code behavior.
- `code` — update code to add the documented behavior and update documents to new behavior.
- `both` — update both docs and code.
- `decide` — needs triage to choose a direction.

## High Priority (Documentation Gaps)


### CLI Reference Mismatches

- [x] **kscorectl global flags documented but not implemented** ✓ FIXED
  - Resolution: code
  - Added `--config`, `--format`, `--verbose`, `--quiet`, `--timeout` global flags to `cmd/kscorectl/main.go`

- [x] **kscorectl version has undocumented `--verbose` flag** ✓ FIXED
  - Resolution: doc
  - Added `--verbose` flag documentation to `docs/content/en/docs/reference/cli.md`

- [x] **CLI reference uses non-existent policy subcommands** ✓ FIXED
  - Resolution: code
  - Added `compliance` and `violations` commands to `cmd/kscore-policy/main.go`

- [x] **Test suite list is documented but not implemented** ✓ FIXED
  - Resolution: code
  - Added `suite list` subcommand to `cmd/kscore-test/main.go`

- [x] **Deprecated/compatibility CLI mappings documented but not implemented** ✓ FIXED
  - Resolution: doc
  - Removed references to `compatibility_mode` config and `kscorectl --show-deprecated` from docs
  - Updated migration guide to note that legacy commands are no longer supported

- [x] **Upgrade CLI flags/subcommands in docs do not match implementation** ✓ FIXED
  - Resolution: code
  - Added to `cmd/kscore-upgrade/main.go`:
    - `check --include-prerelease`, `--channel`
    - `plan --batch-size`, `--save`
    - `execute --plan`, `--async`, `--confirm`
    - `cancel --rollback`

- [x] **Events CLI reference uses `--until` instead of implemented `--before`** ✓ FIXED
  - Resolution: code
  - Added `--until` as alias for `--before` in `cmd/kscore-events/main.go`

- [x] **Cluster CLI reference does not match implementation** ✓ FIXED
  - Resolution: code
  - Added to `cmd/kscore-cluster/commands.go`:
    - `status --watch`, `--filter`, `--format`
    - `backup --shards-only`, `--config-only`
  - Note: `add`, `join`, `leave`, `drain`, `undrain`, `transfer-leader`, `shards` commands exist but need docs update

- [x] **Bootstrap CLI docs list flags not implemented in kscore-bootstrap** ✓ FIXED
  - Resolution: code
  - Added to `cmd/kscore-bootstrap/main.go`:
    - `seed --cluster-name`, `--trust-domain`, `--nats-mode`
    - `restore --transform`

- [x] **Identity token flags in docs do not match implementation** ✓ FIXED
  - Resolution: code
  - Added `--path` and `--uses` flags to `token create` in `cmd/kscore-identity/main.go`

- [x] **Monitor CLI docs list flags not implemented** ✓ FIXED
  - Resolution: code
  - Added `--view` flag (1-8) to set initial view and `--server` as alias for `--control-plane`
  - Updated config to support `initial_view` setting
  - Reference: `cmd/kscore-monitor/main.go`, `cmd/kscore-monitor/config/config.go`, `cmd/kscore-monitor/ui/program.go`

### API Documentation Gaps

- [x] **Document Runbook/Approval API endpoints** ✓ FIXED
  - Resolution: code
  - `/api/v1/runbook/approvals`
  - `/api/v1/runbook/interventions`
  - Location: `pkg/api/runbook/handlers.go`
  - Add to: `docs/content/en/docs/reference/api.md`
  - Added comprehensive documentation for approval and intervention endpoints

- [ ] **Secrets API reference docs have no matching REST/gRPC implementation**
  - Resolution: code
  - Docs describe `/api/v1/secrets/*` REST and secrets gRPC services
  - No secrets proto files under `api/proto/` and no REST handlers under `pkg/api/`
  - Docs reference `github.com/shawnbutts/keystone-core/pkg/secrets` and `pkg/secrets/transit`, but no public `pkg/secrets` exists
  - Reference: `docs/content/en/docs/reference/secrets-api.md`

- [x] **Document PolicyService RPCs** (8 of 11 undocumented) ✓ FIXED
  - Resolution: doc
  - `EvaluatePolicySet`, `ListPolicies`, `GetPolicy`
  - `CreatePolicy`, `UpdatePolicy`, `DeletePolicy`
  - `GetAuditLog`, `ListPolicySets`, `GetPolicySet`
  - Location: `api/proto/policy.proto`
  - All 12 RPCs are already documented in api.md PolicyService section

- [x] **Document GitOps additional endpoints** ✓ FIXED
  - Resolution: doc
  - `GET /api/v1/gitops/verifications/{id}`
  - `GET /api/v1/gitops/rollbacks`
  - `GET /api/v1/gitops/rollbacks/{id}`
  - Location: `pkg/api/gitops/handlers.go`
  - Added documentation for all three endpoints in api.md

- [x] **Document Events API advanced parameters** ✓ FIXED
  - Resolution: doc
  - `filter` (CEL expression)
  - `min_severity`
  - `sort_order`
  - `correlation_id` filtering
  - `tags` filtering
  - Location: `api/proto/event.proto`
  - Added all parameters to List Events query parameters in api.md

- [x] **Reconcile API v2 documentation with code** ✓ FIXED
  - Resolution: doc
  - `docs/content/en/docs/reference/api.md` documents `/api/v2/*` preview endpoints and migration guidance
  - Codebase only implements `/api/v1` handlers; no `/api/v2` routes found
  - Decide whether to implement v2 or remove/flag docs as future work - should only have v1
  - Updated docs to clarify only v1 is implemented, made v2 references hypothetical

- [ ] **Agent metrics REST endpoint documented but not implemented**
  - Resolution: code
  - `docs/content/en/docs/reference/api.md` lists `GET /api/v1/agents/{id}/metrics`
  - `pkg/api/agents/handlers.go` only wires `/api/v1/agents` and `/api/v1/agents/{id}` (+ tags)
  - Implement endpoint or update docs

- [ ] **Maintenance and API key REST endpoints referenced by CLI are not implemented**
  - Resolution:code
  - `cmd/kscorectl` calls `/api/v1/maintenance/*` and `/api/v1/api-keys/*`
  - No handlers found under `pkg/api` for maintenance or api-keys
  - Implement endpoints or remove CLI commands

---

## Medium Priority (Consistency Issues)

### Configuration Field Naming

- [x] **Clarify storage.type vs storage.backend** ✓ FIXED
  - Resolution: doc
  - Docs use `storage.type`, code uses `storage.backend`
  - Viper alias exists but should be documented
  - Location: `internal/config/config.go:671`
  - Updated docs to use `storage.backend` consistently

- [x] **Fix NATS store_dir config key mismatch** ✓ FIXED
  - Resolution: doc
  - Docs use `nats.embedded.store_dir` and `nats.jetstream.store_dir`
  - Code expects `nats.embedded.storedir` and `nats.jetstream.storedir` (no alias)
  - Update docs or add config aliases
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`
  - Updated docs to use `storedir` (no underscore)

- [ ] **NATS config docs include unsupported nested settings**
  - Resolution: code
  - Docs show `nats.tls`, `nats.auth`, `embedded.server_name`, `embedded.max_payload`, `leaf_remotes`, `ping_interval`, and `max_ping_out`
  - Docs include `jetstream.max_memory` and `jetstream.max_file`, but code only supports `jetstream.max_storage`
  - `internal/config.NATSConfig` only supports `mode`, `url`, `embedded.{listen,host,port,enable_jetstream,storedir,max_memory,max_connections,leaf_node_urls,address_family}`, `jetstream`, and reconnect settings
  - Update docs or expand config structs and parsing
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [ ] **Configuration docs list control-plane sections not in config**
  - Resolution: code
  - Docs include `agents`, `execution`, `state`, `events`, `gitops`, and `security` blocks
  - `internal/config.Config` has no corresponding fields for these sections
  - Add configuration wiring for these useful operational settings
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [x] **Configuration docs use api.* keys without aliases** ✓ FIXED
  - Resolution: doc
  - Docs use `api.listen_addrs`, `api.address_family`, `api.allow_insecure_non_loopback`, and `api.tls.*`
  - Code only aliases `api.listen`/`api.grpc_listen` (and api.cors/rate_limit); TLS is top-level `tls.*` and server settings live under `server.*`
  - Restructured Basic Configuration to use correct paths: kept aliased keys under `api:`, moved unaliased to `server:` and `tls:` blocks
  - Also fixed Production Setup example to use top-level `tls:` instead of `api.tls`

- [ ] **Storage config docs include unsupported PostgreSQL fields**
  - Resolution: code
  - Docs list host/port/database/username/password/sslmode and `max_connections`/`idle_connections`
  - Code only supports `storage.postgresql.dsn`, `max_open_conns`, `max_idle_conns`, and `conn_max_lifetime`
  - SQLite docs include `max_connections` but code only supports `path`, `wal`, and `busy_timeout`
  - Update docs or extend config parsing
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [ ] **Policy config docs include unsupported cache/OPA/CEL settings**
  - Resolution: code
  - Docs include `cache_ttl`, `evaluation_timeout`, `opa.*`, and `cel.*`
  - `internal/config.PolicyConfig` only supports enabled/engine/enforcement_mode and built-in policies
  - Update docs or implement policy config options
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [x] **Reconcile telemetry gateway config schema with docs** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/operations/gateway.md` to match `internal/gateway/types.go`:
    - NATS: `url` → `urls[]`, `ca_cert` → `tls.ca_file`
    - Metrics: `max_age` → `stale_timeout`, `max_series` → `cardinality.max_series`, removed `high_cardinality_threshold`
    - Logs: Removed unsupported `max_entries`/`max_age`, `include_sources`/`exclude_sources` → `sources.include`/`sources.exclude`
    - Traces: `sampling_rate` → `sampling.rate`, `sample_errors` → `sampling.priority_sample.errors`, `otlp.url` → `otlp.endpoint`, `use_gzip` → `compression: "gzip"`
    - Removed reference to unsupported `NATS_URL` env var

- [ ] **Update telemetry gateway deploy configs to match gateway schema**
  - Resolution: code
  - `deploy/gateway/config.yaml`, `deploy/gateway/config.yaml.example`, and `deploy/config/telemetry-gateway.yaml` use legacy keys (`nats.url`, `metrics.max_age`, `logs.max_entries`, `traces.sampling_rate`, `otlp.url/use_gzip`, etc.)
  - `deploy/config/telemetry-gateway.yaml` also includes `logging.*` not supported by gateway config
  - Update configs or implement aliases
  - Reference: `deploy/gateway/config.yaml`, `deploy/gateway/config.yaml.example`, `deploy/config/telemetry-gateway.yaml`, `internal/gateway/types.go`

- [ ] **Document default path differences**
  - Resolution: code
  - Docs show: `/var/lib/keystone-core/nats`, `/var/lib/keystone-core/keystone-core.db`
  - Code defaults: `./data/nats`, `./data/keystone-core.db`
  - Clarify dev vs production defaults

- [x] **Fix agent env var documentation** ✓ FIXED
  - Resolution: doc
  - `KSCORE_AGENT_DATACENTER`, `KSCORE_AGENT_ENVIRONMENT`, `KSCORE_AGENT_ROLE`
  - Documented in CLI section but don't exist in config struct
  - Either implement or remove from docs
  - Removed non-existent env vars from documentation

- [ ] **Agent config docs include fields not in config structs**
  - Resolution: both
  - Docs show `logging.file`, `execution.*`, `state.*`, `security.*`, and `agent.{datacenter,environment,role}` under agent config
  - Docs also include `heartbeat.*`, `agent.tags`, and `agent.metadata` sections
  - `internal/config.AgentConfig` only includes ID/heartbeat interval/command timeout/metadata interval/address family/labels/advertise_addrs
  - Update docs or implement settings
  - Reference: `docs/content/en/docs/reference/configuration.md`, `internal/config/config.go`

- [ ] **Logging file output is documented but not supported**
  - Resolution: code
  - Docs include `logging.file` and file output examples
  - LoggingConfig explicitly notes file output is not supported
  - Monitoring guide tails `/var/log/keystone-core/*.log` files

---

## Additional Documentation Drift (In Progress)

### Blueprint Reference Drift

- [x] **Blueprint manifest schema mismatch in reference** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/reference/blueprints.md` to match `internal/blueprint/manifest.go`:
    - `compatibility.modules`: Changed from map to array of strings (`"module@version"` format)
    - `dependencies.requires`/`requires_before`: Changed from objects to strings (`"vendor/name@version"` format)
    - `platforms.arch`: Changed from array to single string (add separate entries for multi-arch)
  - Updated Compatibility Fields table and Dependency Types section examples

- [x] **Blueprint catalog uses unsupported bootstrap flag** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/reference/blueprints-catalog.md`:
    - Changed `--param KEY=VALUE` to `--blueprint-param BLUEPRINT:KEY=VALUE`
  - Note: Similar issues exist in runbooks (covered by separate TODO)

### Query API Reference Drift

- [x] **Query API docs reference a public package that does not exist** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/reference/query-api.md`:
    - Fixed import path to `github.com/shawnbutts/keystone-core/internal/query`
    - Added note that this is an internal API, not part of public Go SDK
    - Recommended using gRPC/REST endpoints for external integrations

### Visualization API Reference Drift

- [x] **Visualization API docs reference a public package that does not exist** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/reference/visualization.md`:
    - Fixed import path to `github.com/shawnbutts/keystone-core/internal/visualization`
    - Added note that this is an internal API, not part of public Go SDK
    - Recommended using HTTP/WebSocket endpoints for external integrations

### Compatibility API Reference Drift

- [ ] **Compatibility Go package referenced in docs does not exist**
  - Resolution: code - use pkg/semver determineChangeType etc
  - `docs/content/en/docs/community/compatibility.md` imports `github.com/shawnbutts/keystone-core/compatibility`
  - No `compatibility` package exists in the repo
  - Update docs or publish the compatibility package

### File Backends Reference Drift

- [ ] **File backends config schema and supported types don’t match implementation**
  - Resolution: code
  - `docs/content/en/docs/reference/file-backends.md` documents a single `backend:` block and types `local`, `s3`, `gcs`, `azure`, `git`, `nats`
  - `cmd/kscore-files/main.go` expects `backends:` list and only wires `filesystem` backend in `createBackend`
  - Update docs or wire remaining backend types and schema aliases (`local` → `filesystem`, `nats` → `nats-object-store`)

### Modules Reference Drift

- [x] **Module reference count/name drift** ✓ FIXED
  - Resolution: doc
  - Updated `docs/content/en/docs/reference/modules.md`: Changed "94 built-in modules" to "93"
  - Code defines 93 unique module names (includes `dns_records`, `mock`, `test`)

- [x] **Auto-generated module docs use absolute import paths** ✓ FIXED
  - Resolution: doc
  - Fixed import paths in both module docs:
    - `resources.md`: Changed to `github.com/shawnbutts/keystone-core/pkg/plugin/resources`
    - `stdlib.md`: Changed to `github.com/shawnbutts/keystone-core/pkg/plugin/stdlib`
  - Note: `tools/moddoc` should also be updated to prevent regression (separate code task)

### Concepts Documentation CLI Drift

- [x] **Events concept uses wrong command name `kscorectl event`** ✓ FIXED
  - Resolution: doc
  - Fixed `kscorectl event` → `kscorectl events` (plural) in `docs/content/en/docs/concepts/events.md`

- [ ] **Events concept documents subcommands not implemented**
  - Resolution: code
  - `docs/content/en/docs/concepts/events.md` documents subcommands not in `cmd/kscore-events/main.go`: `analyze`, `subscribers`, `storage-stats`, `prune`, `archive`
  - Implement these subcommands or remove from docs

- [x] **GitOps concept references missing top-level CLI commands** ✓ FIXED
  - Resolution: doc
  - `docs/content/en/docs/concepts/gitops.md` uses `kscorectl rollback`, `kscorectl promote`, `kscorectl approvals`, `kscorectl verify run/logs`, `kscorectl logs`
  - Fixed: Changed to `kscorectl gitops rollback`, `kscorectl gitops promote`, `kscorectl gitops verify`, `kscorectl runbook approvals`, and journalctl for logs

- [ ] **GitOps concept references `git-sync` CLI that does not exist**
  - Resolution: code - substantial functionality gap
  - `docs/content/en/docs/concepts/gitops.md` documents extensive `git-sync` CLI: conflicts (list/show/diff/resolve/resolve-all), locks (lock/unlock/list), history, audit, status, trigger
  - Only `kscorectl gitops repo sync <name>` exists; conflict resolution, locking, and audit features need implementation
  - Note: Cannot simply remove docs - conflict resolution is core GitOps workflow content

- [x] **Message bus concept uses `kscorectl bench`** ✓ FIXED
  - Resolution: doc
  - `docs/content/en/docs/concepts/message-bus.md` uses `kscorectl bench ...`
  - Actual command is `kscorectl benchmark ...` in `cmd/kscorectl/main.go`
  - Updated to use correct `kscorectl benchmark` command with actual subcommands

- [ ] **Edge concept references `kscorectl cache`, `kscorectl sync`, and `kscorectl connection`**
  - Resolution: code - substantial functionality gap
  - `docs/content/en/docs/concepts/edge.md` uses cache/sync/connection commands
  - Cache commands are under `kscorectl files cache`, not `kscorectl cache`
  - Documented but missing: `cache invalidate/verify/show/refresh/set-ttl/history`
  - Available: `cache status/clear/warm/list/evict/stats`
  - `sync status/force` and `connection test` commands don't exist

- [x] **Proxy agents concept uses outdated credential subcommand** ✓ FIXED
  - Resolution: doc
  - `docs/content/en/docs/concepts/proxy-agents.md` uses `kscorectl proxy credential create`
  - `cmd/kscore-proxy/main.go` provides `proxy credential add`
  - Updated all instances to use `credential add`

- [ ] **Remote execution concept references unsupported exec subcommands/flags**
  - Resolution: code - missing subcommands
  - `docs/content/en/docs/concepts/remote-execution.md` documents `exec archive`, `exec export`, `exec cleanup` subcommands
  - Available: `run/status/list/async/cancel/history/output/shell/script`
  - Not available: `archive`, `export`, `cleanup` (need implementation)

- [ ] **Blueprints concept references unsupported blueprint commands**
  - Resolution: code
  - `docs/content/en/docs/concepts/blueprints.md` uses `blueprint bundle`, `blueprint mirror`, and `blueprint applied usage/history`
  - These commands are not implemented in `cmd/kscore-blueprint` or `cmd/kscore-blueprint-state`

- [ ] **Identity concept references `kscore-agent identity`**
  - Resolution: code
  - `docs/content/en/docs/concepts/identity.md` uses `kscore-agent identity renew`
  - `cmd/kscore-agent` has no `identity` subcommands

- [ ] **NATS mesh concept references missing CLI helpers**
  - Resolution: code
  - `docs/content/en/docs/concepts/nats-mesh.md` uses `kscore-agent nats ping/status/test` and `kscorectl debug nats ...`
  - No `nats` subcommand in `cmd/kscore-agent` and no `debug` command in `kscorectl`

- [ ] **Agents concept/CLI naming mismatch**
  - Resolution: code (remaining)
  - Plugin is `kscore-agents` → exposes `kscorectl agents` (plural is correct)
  - Code help examples use `kscorectl agent` (singular) - need to change to `agents`
  - ✓ Fixed doc: Changed `agents get` → `agents show` in agents.md, service-mesh.md
  - ✓ Fixed doc: Changed `--format json` → `-o json` in agents.md

### Operations Documentation CLI Drift

- [ ] **NATS mesh operations docs reference missing CLI helpers**
  - Resolution: code
  - `docs/content/en/docs/operations/nats-mesh-operations.md` uses `kscore-agent nats ...` and `kscorectl debug nats ...`
  - No `nats` subcommand in `cmd/kscore-agent` and no `debug` command in `kscorectl`
  - Also uses `kscorectl agent update-certs`, which does not exist in `cmd/kscore-agents`

- [ ] **Windows ops docs use unsupported exec/state flags**
  - Resolution: code
  - `docs/content/en/docs/operations/windows.md` uses `kscorectl exec run --shell` and `kscorectl state apply --check-only`
  - `cmd/kscore-exec` has no `--shell` on `exec run` and `cmd/kscore-state` uses `--dry-run` / `state check` instead of `--check-only`
  - Also references `kscore-agent --validate-config` which is not implemented

- [ ] **Migrations ops docs reference missing commands/flags**
  - Resolution: both (partial doc fix applied)
  - Fixed: `state validate` → `state check`, removed `--rendered` from `state show`
  - Remaining code: `migrate verify --source-system salt` doesn't exist (existing `migrate validate` is for SQLite→PostgreSQL only)

- [x] **Cluster management ops docs reference missing cluster subcommands** ✓ FIXED
  - Resolution: doc
  - Fixed: `add-member` → `add`, `stepdown` → `transfer-leader`, `ha-check` → `health`
  - Removed: `compact --all-members` (command doesn't exist)

- [ ] **Secrets backends ops docs reference missing secrets commands**
  - Resolution: code
  - `docs/content/en/docs/operations/secrets-backends.md` uses `kscorectl secrets get`
  - `cmd/kscore-secrets` does not implement `get` (only rotate/schedule/policy subcommands)

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

- [x] **Community docs reference unsupported CLI flags** ✓ FIXED
  - Resolution: doc
  - Fixed: Changed `--log-level=debug` to `KSCORE_LOG_LEVEL=debug` environment variable
  - Fixed: Changed `--server-url` to config file approach with `--config` flag

### Examples and Deploy Docs Drift

- [x] **Blueprint example uses unsupported `--target` flag** ✓ FIXED
  - Resolution: doc
  - Fixed: Removed `--target` flag from `blueprint test` command in lamp-stack README

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

- [ ] **Epic 36 secrets management doc lists many CLI commands not implemented**
  - Resolution: code
  - `epics/36-deep-secrets-management.md` documents `secrets backends`, `get`, `dynamic`, `leases`, `encrypt/decrypt/rewrap`, `template`, `audit`, `cache`
  - `cmd/kscore-secrets` currently only exposes rotate/schedule/policy subcommands

- [ ] **Epic 09 plugin system doc references missing `kscorectl plugin` commands and module features**
  - Resolution: code
  - `epics/09-plugin-system.md` references `kscorectl plugin list/install/publish/verify` and module commands like `module mirror`, `module clean`, `module update`
  - No `kscorectl plugin` command group and several module subcommands are not implemented

- [ ] **Epic 03 state management doc references missing CLI commands**
  - Resolution: code
  - `epics/03-state-management.md` mentions `state compile`, `vars get`, and `state drift --fix`
  - `cmd/kscore-state` does not implement these commands/options

- [ ] **Epic 37 runbooks doc references `kscorectl runbook` subcommands not implemented**
  - Resolution: code
  - `epics/37-enhanced-runbooks.md` documents runbook list/show/apply/delete/versions/rollback and execution controls
  - `cmd/kscore-runbook` only implements approvals/approve/reject/delegate/interventions/respond

- [ ] **Epic 02 remote execution doc uses missing top-level exec/file/job commands and flags**
  - Resolution: code
  - `epics/02-remote-execution.md` references `kscorectl script`, `kscorectl file upload`, and `kscorectl job` commands
  - `cmd/kscore-exec` only provides `exec run/async/script/status/list/output/cancel/history` and has no `--script`, `--script-file`, or `--upload` flags

- [ ] **Epic 04 event system doc uses missing events subcommands**
  - Resolution: code
  - `epics/04-event-system.md` documents `kscorectl events subscribe` and `kscorectl events export`
  - `cmd/kscore-events` has `watch` and no `subscribe` or `export` subcommands

- [ ] **Epic 06 policy enforcement doc references policy/compliance commands not implemented**
  - Resolution: code
  - `epics/06-policy-enforcement.md` documents `kscorectl policy eval/test/schedule/remediate/monitor` and `kscorectl compliance report`
  - `cmd/kscore-policy` only supports list/validate/check/show/create/update/delete/activate/deactivate/audit/report

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

- [x] **Future web UI management console doc references missing user/group/role CLI** ✓ FIXED
  - Resolution: doc
  - Added note to CLI Commands section indicating these are part of the planned future implementation
  - Commands are documented for planning purposes, not as available features
  - Reference: `epics/future-web-ui-management-console.md`

### Reference Docs Drift

- [x] **Events reference docs use `kscorectl event` instead of `events` and include missing exports** ✓ FIXED
  - Resolution: doc
  - Fixed: Changed `kscorectl event` to `kscorectl events` in events.md and cli-quick-reference.md

- [x] **Events reference docs use unsupported flags and values** ✓ FIXED
  - Resolution: doc
  - Fixed: Changed `--until now` to `--before 30m` (though `--until` is aliased)
  - Fixed: Changed `--severity warning,error,critical` to `--severity warning` (minimum severity)

- [x] **API reference uses non-existent `kscorectl auth` commands** ✓ FIXED
  - Resolution: doc
  - Fixed: Changed `kscorectl auth create-key --ttl` to `kscorectl api-key create --expires-in`

- [ ] **Runbook automation reference lists unsupported runbook commands**
  - Resolution: code
  - `docs/content/en/docs/reference/runbook-automation.md` uses `kscorectl runbook execute`, `status`, `list-executions`, `audit`, and `test`
  - `cmd/kscore-runbook` only implements approvals/approve/reject/delegate/interventions/respond
  - These are core runbook execution commands that need implementation

- [ ] **NATS mesh reference lists missing debug commands**
  - Resolution: code
  - `docs/content/en/docs/reference/nats-mesh.md` documents `kscorectl debug nats ...`
  - No `debug` subcommand or plugin exists in `cmd/`
  - Significant diagnostic tooling needs implementation

- [ ] **Blueprint reference docs include bundle/mirror/registry commands not implemented**
  - Resolution: code
  - `docs/content/en/docs/reference/blueprints.md` documents `blueprint bundle` and `blueprint mirror`
  - `docs/content/en/docs/reference/blueprints-catalog.md` documents `blueprint registry add`
  - No bundle/mirror/registry subcommands exist in `cmd/kscore-blueprint*`
  - Core blueprint distribution commands need implementation
### Tutorials and Scenarios Documentation CLI Drift

- [x] **Tutorials/scenarios use legacy exec syntax and flags** ✓ FIXED
  - Resolution: doc
  - Fixed: Changed `exec "target" --cmd "cmd"` to `exec run "target" -- cmd` in scenario docs
  - Fixed: Changed `exec -t "target" -s shell` to `exec run "target" --` in scenario docs
  - Files: multi-tier-webapp.md, database-ha.md, hybrid-infrastructure.md, windows-infrastructure.md,
    microservices-platform.md, gitops-workflow.md, edge-deployment.md

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

- [x] **Auth bypass method example references non-existent RPC** ✓ FIXED
  - Resolution: doc
  - Changed `/kscore.v1.ControlPlaneService/HealthCheck` to `/keystone.core.v1.ControlPlaneService/GetServerStatus`
  - Fixed package name (`kscore.v1` → `keystone.core.v1`) and RPC method (`HealthCheck` → `GetServerStatus`)
  - Reference: `docs/content/en/docs/reference/configuration.md`

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

- [x] **API server config keys in docs do not match config** ✓ FIXED
  - Resolution: doc
  - Fixed server config keys to use correct viper format (no underscores):
    - `listen_addrs` → `listenaddrs`
    - `allow_insecure_non_loopback` → `allowinsecurenonloopback`
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [x] **API listen_addrs/address_family keys in docs lack config aliases** ✓ FIXED
  - Resolution: doc
  - Fixed `address_family` → `addressfamily` in server, nats.embedded, and agent sections
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [x] **API metrics/tracing/health config keys in docs lack aliases** ✓ RESOLVED
  - Resolution: doc
  - Docs already use top-level `metrics`, `tracing`, `health` (not `api.*`)
  - No changes needed - docs already match the config structure
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [x] **Config docs use snake_case keys that don't match config fields** ✓ FIXED
  - Resolution: doc
  - Fixed metrics: `include_go_metrics` → `includegometrics`, `include_process_metrics` → `includeprocessmetrics`
  - Fixed health: `startup_grace_period` → `startupgraceperiod`, `check_interval` → `checkinterval`, `min_healthy` → `minhealthy`
  - Fixed rate limit: `key_extractor` → `keyextractor`, `header_name` → `headername`, `requests_per_minute` → `requestsperminute`
  - Fixed auth: `header_name` → `headername`, `metadata_key` → `metadatakey`, `public_key_file` → `publickeyfile`, `role_claim` → `roleclaim`, `require_client_cert` → `requireclientcert`, `cert_roles` → `certroles`
  - Fixed webhook: `auth_type` → `authtype`, `hmac_secret` → `hmacsecret`, `bearer_token` → `bearertoken`
  - Fixed syslog TLS: `ca_cert` → `cacert`, `skip_verify` → `skipverify`, `min_version` → `minversion`
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [x] **CORS aliases are incomplete** ✓ FIXED
  - Resolution: doc
  - Updated CORS config keys to use viper format (no underscores):
    - `allowed_origins` → `allowedorigins`
    - `allowed_methods` → `allowedmethods`
    - `allowed_headers` → `allowedheaders`
    - `allow_credentials` → `allowcredentials`
    - `max_age` → `maxage`
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [x] **CLI configuration file is documented but not implemented** ✓ FIXED
  - Resolution: doc
  - Added note that CLI configuration file support is planned but not yet implemented
  - Docs now clarify to use command-line flags or environment variables
  - Reference: `docs/content/en/docs/reference/configuration.md`

- [ ] **`kscorectl config set` is referenced in docs but not implemented**
  - Resolution: code
  - `kscorectl config` only supports `validate`
  - Docs use `kscorectl config set ...` in runbooks, security training, and blueprints (15+ references)
  - Implementing `config set` would be more appropriate than removing operational examples
  - Reference: `docs/runbooks/*.md`, `docs/content/en/docs/operations/security-training.md`, `docs/content/en/docs/concepts/blueprints.md`, `cmd/kscorectl/main.go`

### SDK Documentation vs Implementation

- [x] **Align client SDK docs with actual SDK availability** ✓ FIXED
  - Resolution: doc
  - Added disclaimers that Python/TypeScript/Java client SDKs are planned, not published
  - Fixed proto path from `proto/` to `api/proto/`
  - Updated Reference Implementations section to note `pkg/client`, `pkg/models`, `pkg/errors` are planned
  - Added note to SDK Distribution section that package configs are examples
  - Reference: `docs/content/en/docs/reference/sdk.md`
  - Note: Compatibility docs still reference non-existent `compatibility` package (separate issue)

- [x] **SDK docs reference non-existent client packages and proto layout** ✓ FIXED
  - Resolution: doc
  - Fixed proto path from `proto/` to `api/proto/*.proto`
  - Updated Reference Resources table to show actual paths
  - Added note that `pkg/client`, `pkg/models`, `pkg/errors` are planned
  - Reference: `docs/content/en/docs/reference/sdk.md`

- [x] **SDK docs publish package metadata for SDKs that don't exist** ✓ FIXED
  - Resolution: doc
  - Added disclaimer to SDK Distribution section noting these are example configs
  - Clarified that `kscore` (PyPI), `@kscore/client` (npm), etc. are planned, not published
  - Reference: `docs/content/en/docs/reference/sdk.md`

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

- [ ] **REST API handlers are not wired into kscore-server**
  - Resolution: code
  - `cmd/kscore-server/main.go` only registers `/health/*` and `/api/status`
  - REST handlers exist under `pkg/api/*` but are never registered on the HTTP mux
  - Docs describe `/api/v1/*` endpoints as available

- [ ] **OpenAPI spec paths do not match implemented API**
  - Resolution: both
  - OpenAPI spec lists `/api/agents` and omits `/api/v1/agents`; code only implements `/api/v1/agents`
  - Spec includes `/api/v1/mirrors` and `/api/v1/discovery` endpoints that are not wired into `kscore-server`
  - Update spec to reflect actual endpoints or wire the handlers
  - Reference: `api/openapi/openapi-spec.yaml`, `pkg/api/agents/handlers.go`, `internal/files/mirror/api.go`, `internal/proxy/discovery/api.go`, `cmd/kscore-server/main.go`

- [x] **Fix Agent API tags vs labels terminology** ✓ FIXED
  - Resolution: doc
  - Changed `PATCH /api/v1/agents/{agent_id}/labels` to `PATCH /api/v1/agents/{id}/tags`
  - Updated request/response format to match implementation
  - Location: `docs/content/en/docs/reference/api.md`

- [x] **Fix Events API pagination parameters** ✓ FIXED
  - Resolution: doc
  - Changed `since`/`until` to `start`/`end` to match REST handler
  - Updated response to include `retrieved_at` field
  - Note: REST handler uses `limit`/`offset`; gRPC uses `page_size`/`page_token`
  - Location: `docs/content/en/docs/reference/api.md`

- [x] **Fix timeout parameter format** ✓ FIXED
  - Resolution: doc
  - Changed `timeout: "30s"` to `timeout: 30` (integer seconds)
  - Updated exec request body and parameters in API docs
  - Location: `docs/content/en/docs/reference/api.md`

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

- [x] **Agent tags update request body does not match docs/SDK** ✓ FIXED
  - Resolution: doc
  - Fixed SDK docs to use `tags: dict[str, str]` format matching handler
  - Added docstring explaining empty string deletes tag
  - Added `updated` field to API docs response
  - Reference: `docs/content/en/docs/reference/api.md`, `docs/content/en/docs/reference/sdk.md`, `pkg/api/agents/handlers.go`

- [x] **Execution REST API docs do not match handler request/response** ✓ FIXED
  - Resolution: doc
  - Fixed POST /api/v1/exec: Updated request params, changed results from array to map, replaced summary with created_at
  - Fixed GET /api/v1/jobs/{id}: Changed job_id→id, target→agent_id, added actual response fields
  - Fixed GET /api/v1/jobs: Updated query params (agent_id, status, sort, order), added retrieved_at to response
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/execution/handlers.go`

- [x] **State REST API docs do not match handler** ✓ FIXED
  - Resolution: doc
  - Fixed apply endpoint: state→content, check_only→dry_run, updated response to single-run format
  - Fixed check endpoint: Added proper validation response with valid/errors/warnings/states/modules
  - Fixed drift endpoint: Updated to has_drift, states array with differences
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/state/handlers.go`

- [x] **Events REST API docs do not match handler query params and response** ✓ FIXED
  - Resolution: doc
  - Query params already correct (start/end, correlation_id, tags, sort, order)
  - Fixed POST response to show full event object instead of just id/type/timestamp
  - Added missing GET /api/v1/events/{id} endpoint documentation
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/events/handlers.go`

- [x] **Policy REST API docs do not match handler inputs/filters** ✓ FIXED
  - Resolution: doc
  - Fixed evaluate: Changed input→resource, added action (required), policy_set_id, resource_type options
  - Fixed violations: Changed policy→policy_id, since→start/end, added resource_type/user/action/limit params
  - Fixed compliance: Changed environment/period→start/end, updated response to report object
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/policy/handlers.go`

- [x] **GitOps REST API docs do not match handler fields/endpoints** ✓ FIXED
  - Resolution: doc
  - Fixed List Verifications: Added query params, updated response with workflow_name, step stats, pagination
  - Fixed Trigger Rollback: Added required type/reason params, updated response fields
  - Fixed List Rollbacks: Removed unsupported namespace filter
  - Added missing POST /api/v1/gitops/rollbacks/{id}/approve endpoint
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/gitops/handlers.go`

- [x] **Cluster REST API docs do not match response formats** ✓ FIXED
  - Resolution: doc
  - Fixed status: Added healthy, members[], updated_at; removed cluster_name, healthy_count
  - Fixed members: Changed from wrapped object to raw array, updated field names
  - Fixed leader: Changed to full member status response
  - Fixed rebalance: Updated to show moved_agents, reason, trigger_member_id, timing fields
  - Fixed backup: Added note that endpoint returns file download
  - Reference: `docs/content/en/docs/reference/api.md`, `pkg/api/cluster/handlers.go`

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

- [x] **`/api/status` endpoint is implemented but not documented** ✓ FIXED
  - Resolution: doc
  - Added "Server Status" section to API reference documenting `/api/status` endpoint
  - Documents version, uptime, agent counts, runtime metrics, and health status
  - Reference: `docs/content/en/docs/reference/api.md`

### Blueprint Documentation

- [x] **Document `kscore/registry` blueprint** ✓ RESOLVED
  - Resolution: doc
  - Note: `examples/blueprints/kscore/registry/` is NOT a blueprint - it contains the registry metadata (catalog.json and blueprint manifests)
  - Already referenced in blueprints-catalog.md line 1045
  - No additional documentation needed

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

- [x] **Update kscore/monitoring-stack features** ✓ FIXED
  - Resolution: doc
  - Added `default_dashboards` and `default_alerts` to Features table
  - Note: `blackbox_exporter` was already documented
  - Reference: `docs/content/en/docs/reference/blueprints-catalog.md`

### Operations Documentation Gaps

- [ ] **Registry ops docs imply object storage backends not supported by code**
  - Resolution: code
  - Docs describe shared storage options including GCS/S3/Azure Files and S3 cross-region replication
  - `kscore-registry` only reads/writes from a local filesystem data directory (no object storage integration or replication support)
  - Update docs to clarify external replication only, or implement storage backend support
  - Reference: `docs/content/en/docs/operations/registry.md`, `cmd/kscore-registry/main.go`

- [x] **Security guide references non-existent auth CLI commands** ✅ FIXED
  - Resolution: doc
  - Replaced `kscorectl auth login/refresh` with `kscorectl api-key create` in security.md
  - Replaced `kscorectl auth revoke-session` and `kscorectl user disable` with api-key commands in security-training.md
  - Added notes that user/session management is handled by external identity providers
  - Reference: `docs/content/en/docs/operations/security.md`, `docs/content/en/docs/operations/security-training.md`

- [ ] **API key CLI points to REST endpoints that are not implemented**
  - Resolution: code
  - `kscorectl api-key` uses `/api/v1/api-keys` endpoints
  - No REST handlers for API key CRUD are wired into `kscore-server`
  - Update docs/CLI or implement API key REST handlers
  - Reference: `cmd/kscorectl/main.go`, `cmd/kscore-server/main.go`, `docs/content/en/docs/operations/security.md`

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

- [x] **Troubleshooting docs use command names/flags not in CLI** ✅ PARTIALLY FIXED
  - Resolution: both (doc fixes applied, remaining items need code)
  - Fixed in docs:
    - `kscorectl event` → `kscorectl events` (list, dlq show/list, replay)
    - `kscorectl reactor` → replaced with NATS consumer info and journalctl
    - `kscorectl approval` → `kscorectl runbook approvals`
    - `kscorectl file` → `kscorectl files-storage` backend commands
    - `kscorectl gitops sync` → `kscorectl gitops repo sync`
    - `kscorectl webhook` → `kscorectl gitops webhook`
    - `kscorectl policy evaluate` → `kscorectl policy check`
  - Remaining items need code implementation:
    - `kscorectl cloud status`, `k8s status`, `env list` (multi-env CLI)
    - `kscorectl module list/status/capabilities/signing-key/debug/wasm`
    - `kscorectl policy precedence/conflicts/profile/eval-cel`
    - `kscorectl blueprint status/logs/resources/can-rollback/deps`
  - Reference: `docs/content/en/docs/operations/troubleshooting.md`

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

- [x] **GitOps concept docs use config validation commands that target the wrong config** ✅ FIXED
  - Resolution: doc
  - Changed `kscorectl config validate states/` to use `kscorectl state check` with shell loop
  - Removed vars/ validation (vars are data files, not state definitions)
  - Reference: `docs/content/en/docs/concepts/gitops.md`

- [ ] **Compliance scenario references a CLI that does not exist**
  - Resolution: code
  - Docs use `kscorectl compliance ...` commands - should be `kscorectl policy ...`
  - Commands exist (`policy report`, `policy compliance`, `policy check`) but with different flags
  - Doc flags: `--framework`, `--target`, `--from`, `--to`, `--format html/pdf`
  - Code flags: `--days`, `--output (text/json/yaml/table)`
  - Also uses `kscorectl logs --reactor` which doesn't exist
  - Requires implementing doc flags or rewriting entire scenario
  - Reference: `docs/content/en/docs/scenarios/compliance-automation.md`, `cmd/kscore-policy/main.go`

- [x] **Drift detection tutorial uses non-existent `kscorectl apply` command** ✓ FIXED
  - Resolution: doc
  - Changed `kscorectl apply` to `kscorectl state apply`
  - Changed `--last 24h` to `--since 24h`
  - Reference: `docs/content/en/docs/tutorials/drift-detection.md`

- [x] **Scenarios use invalid exec CLI syntax** ✓ FIXED
  - Resolution: doc
  - Verified all scenario docs already use correct syntax: `kscorectl exec run <target> -- <command>`
  - No instances of incorrect `--cmd` or `-t` flags found
  - Reference: `docs/content/en/docs/scenarios/multi-tier-webapp.md`, `docs/content/en/docs/scenarios/microservices-platform.md`

- [x] **File distribution concept docs use command names not in kscore-files** ✅ FIXED
  - Resolution: doc
  - Fixed `files ls` → `files list`, `files rm` → `files delete`, `files stat/hash` → `files info`
  - Fixed `namespace acl add` → `namespace access` with correct flags
  - Fixed `backend gc` → `backend health`
  - Fixed `mirrors sync-status/sync --group` → positional group-id argument
  - Fixed `mirrors failover --from` → `--to`, `conflicts --id` → `resolve-conflict`
  - Reference: `docs/content/en/docs/concepts/file-distribution.md`

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

- [x] **Events reference doc uses wrong CLI name and flags** ✓ FIXED
  - Resolution: doc
  - Verified docs already use `kscorectl events` (plural) correctly
  - No instances of incorrect `--until` flag or multi-value `--severity` found
  - Reference: `docs/content/en/docs/reference/events.md`

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

- [x] **CLI quick reference uses event/policy flags that do not exist** ✅ FIXED
  - Resolution: doc
  - Fixed `--agent` to `--source` in events list command
  - Fixed `policy test` to `policy check`
  - Reference: `docs/content/en/docs/reference/cli-quick-reference.md`, `cmd/kscore-events/main.go`, `cmd/kscore-policy/main.go`

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

- [x] **Community support docs reference missing CLI commands/flags** ✅ FIXED
  - Resolution: doc
  - Fixed `state list-modules` → `module tree`
  - Fixed `events retention --max-age=7d --apply` → `events retention set --max-age 7d` + `events retention apply`
  - `--log-level=debug` not found in support.md (may have been fixed previously)
  - Reference: `docs/content/en/docs/community/support.md`

- [ ] **FAQ uses unsupported state apply verbosity flag**
  - Resolution: c ode
  - Docs use `kscorectl state apply myconfig.yaml -v`
  - `kscore-state` has no `-v/--verbose` flag
  - Update docs or implement flag
  - Reference: `docs/content/en/docs/community/faq.md`, `cmd/kscore-state/main.go`

- [x] **Compatibility docs reference non-existent CLI** ✅ FIXED
  - Resolution: doc
  - Changed `kscorectl version --all` → `kscorectl version --verbose`
  - Changed `kscorectl agent list` → `kscorectl agents list` (plural)
  - Removed non-existent `kscorectl compatibility check` command section
  - Reference: `docs/content/en/docs/community/compatibility.md`

- [x] **Community announcement uses non-existent bootstrap/blueprint commands** ✅ FIXED
  - Resolution: doc
  - Changed `bootstrap init` → `bootstrap seed` in announcement and release notes
  - Changed `blueprint apply` → `blueprint install` in announcement
  - Reference: `docs/content/en/docs/community/announcement-0.1.0.md`, `docs/content/en/docs/community/release-notes.md`

- [x] **Release notes reference unsupported state apply flags** ✓ FIXED
  - Resolution: doc
  - Changed `state apply -f my-state.yaml` to `state apply my-state.yaml`
  - Reference: `docs/content/en/docs/community/release-notes.md`

- [x] **IPv6 targeting docs reference unsupported selectors** ✓ FIXED
  - Resolution: doc
  - Updated docs to use generic `ip:` selector with glob patterns
  - Removed unsupported `ipv6:`, `ipv6_cidr:`, `ipv4:`, and `has_ipv6:` selectors
  - Added note that CIDR notation is not directly supported
  - Reference: `docs/content/en/docs/reference/ipv6.md`

- [x] **Targeting docs use `agent_id` selector that is not supported** ✓ FIXED
  - Resolution: doc
  - Changed `agent_id:` to `id:` in targeting examples
  - Updated: hello-world.md, quick-start.md, cli.md
  - Note: YAML fields like `.event.data.agent_id` are not targeting selectors, left unchanged

- [x] **Docs use --log-level flags that binaries do not support** ✓ FIXED
  - Resolution: doc
  - Changed `--log-level=debug` to `KSCORE_LOG_LEVEL=debug` environment variable
  - Removed unsupported `--server-url` and `--agent-id` flags from examples
  - Updated: development.md, agents.md
  - Note: support.md may still have issues (separate review needed)

- [x] **Self-management docs reference cluster token CLI that does not exist** ✅ FIXED
  - Resolution: doc
  - Replaced `$(kscorectl cluster token)` with `$CLUSTER_JOIN_TOKEN` env var
  - Added comment explaining token comes from initial bootstrap or cluster config
  - Reference: `docs/content/en/docs/operations/self-management.md`

### Makefile Target Documentation

- [x] **Fix build target names in development.md** ✓ FIXED
  - Resolution: doc
  - Changed `make build-server/build-agent/build-cli` to `make server/agent/cli`
  - Removed non-existent `make build-plugins` from installation.md
  - Updated both development.md and installation.md

- [x] **Fix missing build/docker targets in development.md** ✓ FIXED
  - Resolution: doc
  - Changed `make build-all` to `make build-all-platforms`
  - Removed non-existent `make docker-build`, kept manual docker build commands
  - Reference: `docs/content/en/docs/community/development.md`, `Makefile`

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

Consider implementing these documented but missing targets:

- [ ] `make fmt` - Run gofmt
- [ ] `make lint-fix` - Run golangci-lint --fix
- [ ] `make check` - Pre-commit checks (fmt, vet, lint, tidy, test)
- [ ] `make dev` - Hot reload development with air
- [ ] `make test-verbose` - Verbose test output
- [ ] `make test-coverage` - Generate coverage reports
- [ ] `make test-integration` - Run integration tests
- [ ] `make benchmark` - Run benchmarks

### CLI Flag Documentation

- [x] **Document TLS flags for kscore-exec** ✓ FIXED
  - Resolution: doc
  - Added `--tls-ca-cert`, `--tls-cert`, `--tls-key`, `--tls-server-name` to cli.md
  - Reference: `docs/content/en/docs/reference/cli.md`

### CLI Command Coverage

- [x] **Reconcile policy compliance/violations commands** ✅ FIXED
  - Resolution: doc
  - Commands `policy compliance` and `policy violations` DO exist in code
  - Fixed incorrect flags in docs: `--environment` → `--days`, `--severity` → `--limit`
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document kscorectl health command** ✅ FIXED
  - Resolution: doc
  - Added `kscorectl health` with `--full` flag and `health check` subcommand to CLI docs
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document kscorectl api-key commands** ✅ FIXED
  - Resolution: doc
  - Added `api-key create/list/revoke` with all flags to CLI docs
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document kscorectl benchmark commands** ✅ FIXED
  - Resolution: doc
  - Added benchmark command with all subcommands and flags to CLI docs
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Remove or implement HTML output in policy report examples** ✅ FIXED
  - Resolution: doc
  - Changed `--output html` to `--output json` in migration example
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Fix agent command naming mismatch** ✅ FIXED
  - Resolution: doc
  - Changed `kscorectl agent` to `kscorectl agents` (plural) throughout CLI docs
  - Updated kscore-agents section and migration table
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Align kscore-monitor CLI docs with implementation** ✅ FIXED
  - Resolution: doc
  - Added `--control-plane`, `--nats-url`, `--theme`, `--no-color` flags
  - Fixed `--refresh` from duration to int seconds
  - Updated `--server` to note it's an alias for `--control-plane`
  - Note: `--view` was already in the code (TODO description was incorrect)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-monitor/main.go`

- [ ] **Implement --format flag for events watch command**
  - Resolution: code
  - Docs claim `kscorectl events watch --format jsonl`, but command has no `--format` flag
  - Implement `--format` flag with text, json, jsonl options
  - Also add missing `--filter` and `--tag` flags to docs (these exist in code)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [x] **Align events list flags with docs** ✅ FIXED
  - Resolution: doc
  - Fixed `--limit` default from 100 to 50, removed `-n` alias
  - Added `--before` flag, updated `--until` as alias
  - Added `--tag` flag
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [ ] **Add missing events query flags**
  - Resolution: code
  - Command exists at `cmd/kscore-events/main.go:newQueryCmd` (line 202)
  - Code only has `--limit` flag; docs also show `--since` and `--until`
  - Implement `--since` and `--until` time range flags
  - Also: docs show `-n` alias for --limit which doesn't exist (doc issue)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`

- [x] **Fix events retention command name in docs** ✓ FIXED
  - Resolution: doc
  - Docs use `events retention show`; code uses `events retention list`
  - Update docs or add alias
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-events/main.go`
  - Updated cli.md and cli-quick-reference.md to use `events retention list`

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

- [x] **Fix upgrade check flags in docs** ✅ FIXED
  - Resolution: doc
  - Original description was incorrect - code has all three flags
  - Docs were missing `--target, -t` flag; also fixed channel values (nightly not edge)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-upgrade/main.go`

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

- [x] **Document proxy discover subcommands** ✅ FIXED
  - Resolution: doc
  - Added documentation for: `status`, `approve-all`, `ignore`, `auto-approve`, `logs`, `config`
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [x] **Fix proxy drift/state CLI docs** ✅ FIXED
  - Resolution: doc
  - Renamed `drift show` to `drift report`
  - Fixed `drift check` to use `[device-id]` positional arg with `--all`
  - Added `drift remediate` subcommand
  - Added `state logs` subcommand
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [ ] **Implement proxy drift/state missing features**
  - Resolution: code
  - Implement `drift check --severity` filter (docs show it, useful feature)
  - Implement `state check` command (docs show it, useful for compliance checking)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [x] **Align proxy credential CLI docs with implementation** ✅ FIXED
  - Resolution: doc
  - Renamed `remove` to `delete`, removed non-existent `update`
  - Added: `show`, `test`, `rotate`, `verify`, `backend-status`
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [x] **Align proxy device CLI docs with implementation** ✅ FIXED
  - Resolution: doc
  - Added missing subcommands: import, update, health, ping, status, config show, connect
  - Fixed list flags: --proxy, --vendor, --type, --status
  - Fixed add flags: --type (not --device-type), --labels (not --label), removed --port
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-proxy/main.go`

- [x] **Resolve maintenance command name clash** ✅ FIXED
  - Resolution: doc
  - Changed all `kscorectl maintenance` to `kscorectl schedule maintenance` in docs
  - Updated section headers to use `schedule maintenance` prefix
  - Note: Built-in maintenance mode commands (enable/disable/status/queue/cleanup) are separate
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Align kscore-bootstrap CLI docs with implemented flags** ✅ FIXED
  - Resolution: doc
  - Removed non-existent flags from validate (--config-dir, --check-connectivity, --strict)
  - Fixed validate syntax to use positional arg instead of --config flag
  - Removed --verbose from status, added Status Flags section
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-bootstrap/main.go`

- [x] **Fix CLI migration table commands that do not exist** ✓ FIXED
  - Resolution: doc
  - Fixed migration table entries:
    - Removed `exec batch` row (command doesn't exist)
    - Changed `policy evaluate` to `policy check`
    - Changed `gitops sync` to `gitops repo sync`
    - Changed `state apply --all` to `state apply <statefile>`
    - Updated `vars get` and `facts list` to note actual alternatives
  - Note: `files push/pull` are valid aliases for `put/get`, left as-is
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document gitops repo/deploy commands or remove from code** ✓ FIXED
  - Resolution: doc
  - Added documentation for `gitops repo` commands (list, add, remove, sync)
  - Added documentation for `gitops deploy` commands (list, show, rollback, approve)
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Align gitops output format flags in docs** ✓ FIXED
  - Resolution: doc
  - Updated all gitops command output format flags to show `text, json, yaml, table`
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document kscore-agent bootstrap flags not listed in docs** ✓ FIXED
  - Resolution: doc
  - Reorganized bootstrap flags into logical sections (Basic, Cluster, Storage, NATS, TLS, Package, Migration, Blueprint)
  - Added all missing flags: --dry-run, --verbose, --json, --config, --config-file, --skip-repo-setup, --postgres-*, --tls-*, --nats-*, --package-*, --migrate-*, --blueprint-*, --export-states-dir
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Fix kscore-server default config path and filename in docs** ✓ FIXED
  - Resolution: doc
  - Updated docs to show CLI default `./keystone-core.yaml` for development
  - Added note explaining production path `/etc/keystone-core/server.yaml`
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Align kscore-agent config filename/path in docs and CLI help** ✓ FIXED
  - Resolution: doc
  - Updated docs to show CLI default `./keystone-core-agent.yaml` for development
  - Added note explaining production paths for Linux/macOS and Windows
  - Reference: `docs/content/en/docs/reference/cli.md`

### File Distribution Docs vs CLI

- [ ] **Align file distribution docs with actual files CLI**
  - Resolution: code
  - Docs use `namespace/path` syntax for files, but CLI requires `--namespace <name> <path>` separately
  - Some commands fixed (list/delete/info), but namespace path syntax is a structural difference
  - Requires implementing namespace path syntax support (e.g., `namespace:/path`) or extensive doc rewrites
  - Reference: `docs/content/en/docs/concepts/file-distribution.md`, `cmd/kscore-files/*`

- [x] **Align file backends configuration docs with kscore-files config** ✅ FIXED
  - Resolution: doc
  - Docs show a single `backend:` block with nested type config; server config expects `backends:` array with `name`, `type`, `root_path`, `paths`, and `read_only`
  - Fixed: Updated general config example and filesystem backend to use correct `backends:` array format
  - Added note about current backend support (only `filesystem` is wired)
  - Reference: `docs/content/en/docs/reference/file-backends.md`

- [x] **Document or implement non-filesystem file backends** ✅ FIXED
  - Resolution: doc
  - `kscore-files` server config only wires `filesystem` in `createBackend`; docs list S3/GCS/Azure/Git/NATS backends as supported
  - Fixed: Added note that cloud backends are implemented as library code but not yet exposed in server config
  - Reference: `docs/content/en/docs/reference/file-backends.md`

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

- [x] **Registry configuration docs include unsupported fields** ✅ FIXED
  - Resolution: doc
  - Docs include `auth.enabled`, `security.cors_*`, `logging.*`, and `telemetry.*` in registry YAML
  - `kscore-registry` only supports `--listen`, `--data`, `--api-key`, `--readonly`, `--max-upload-size`, and CORS toggles/origins
  - Added note clarifying that `logging.*` and `telemetry.*` are deployment reference only
  - Reference: `docs/content/en/docs/reference/configuration.md`, `cmd/kscore-registry/main.go`

- [ ] **Fix file backend option names in docs**
  - Resolution: both
  - Docs use `gcs.project_id` but code expects `gcs.project`
  - Docs use `azure.account` and `azure.access_key`; code expects `account_name` and `account_key`
  - Docs use `nats.bucket` and `nats.storage`; code expects `bucket_name` and has no `storage` option
  - Docs use `git.auth` block with `sync_interval/auto_commit/commit_author`; code expects `username/password`, `auto_pull`, `pull_interval`, and `ssh_key_file`
  - Docs describe backend interface methods `Read/Write/Hash`, but code uses `Get/Put/Stat` etc.
  - Reference: `docs/content/en/docs/reference/file-backends.md`, `internal/files/backend/*.go`

- [x] **File backend docs claim content-addressed storage but backends use direct paths** ✅ FIXED
  - Resolution: doc
  - Docs say all backends store files by SHA-256 path prefixes
  - Filesystem backend uses `root + path` and preserves caller paths; no content-addressed layout
  - Replaced "Content-Addressed Storage" section with accurate "Storage Layout" description
  - Reference: `docs/content/en/docs/reference/file-backends.md`, `internal/files/backend/filesystem.go`

- [ ] **HTTP file backend is declared but not implemented or documented**
  - Resolution: code
  - `BackendTypeHTTP` is defined but there is no `http` backend implementation
  - Docs do not mention an HTTP backend
  - Implement backend or remove type from API surface
  - Reference: `internal/files/backend/backend.go`, `docs/content/en/docs/reference/file-backends.md`

### Loadtest/Test CLI Docs vs Code

- [x] **Document missing kscore-loadtest flags** ✅ FIXED
  - Resolution: doc
  - Added flags table for `loadtest run` with all 9 flags
  - Added flags table for `loadtest report` with `--file` flag
  - Added brief descriptions for each subcommand
  - Reference: `docs/content/en/docs/reference/cli.md`

- [ ] **Align kscore-test flags and suite commands**
  - Resolution: code
  - Docs omit many flags (`--tags`, `--parallel`, `--fail-fast`, `--type` for list, `--status` for history)
  - Docs list `test suite list` but code only has `suite show/create/delete`
  - Update docs or add missing commands/flags
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-test/main.go`

### Module CLI Docs vs Code

- [x] **Fix module build flags** ✅ FIXED
  - Resolution: doc
  - Removed non-existent `--exclude` flag
  - Changed `--no-validate` to `--no-verify` (matches code)
  - Added `-o` short form for `--output`
  - Updated example to use `--no-verify`
  - Reference: `docs/content/en/docs/reference/cli.md`

### Exec CLI Docs vs Code

- [ ] **Document missing kscore-exec commands and flags**
  - Resolution: both
  - Docs cover run/status/list/shell/script only
  - Code also includes `async`, `cancel`, `history`, and `output` subcommands
  - Docs omit run flag `--dry-run` and list/history filters (`--since`, `--before`, etc.)
  - TLS flags in code (`--tls`, `--tls-ca-cert`, `--tls-cert`, `--tls-key`, `--tls-server-name`) are not documented
  - Update docs or remove/alias commands
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-exec/main.go`

### State CLI Docs vs Code

- [x] **Document state apply preview flag** ✅ FIXED
  - Resolution: doc
  - Added `--preview` flag to state apply flags list
  - Added example showing preview with variables
  - Reference: `docs/content/en/docs/reference/cli.md`

### Events CLI Docs vs Code

- [x] **Document events retention min-severity flag** ✅ FIXED
  - Resolution: doc
  - Added `--min-severity` flag to events retention set flags list
  - Added example showing min-severity filter for debug events
  - Reference: `docs/content/en/docs/reference/cli.md`

### Cluster CLI Docs vs Code

- [x] **Document missing kscore-cluster commands and flags** ✅ FIXED
  - Resolution: doc
  - ~~Docs omit `shards`, `add`, `join`, `leave`, `drain`, `undrain`, `transfer-leader`~~ Added all 7 commands
  - ~~Docs show `cluster status --watch` but code has no watch flag~~ Code does have --watch, no fix needed
  - ~~Docs show `cluster members --filter`; code uses `--details`~~ Fixed to use --details
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-cluster/commands.go`

- [x] **Fix remaining cluster command flag discrepancies** ✅ FIXED
  - Resolution: doc
  - ~~Docs show `cluster backup --format`~~ Removed, added --shards-only/--config-only
  - ~~Docs show `cluster restore` positional file with `--shards-only/--config-only`~~ Fixed to use --input/-f
  - ~~Docs show `cluster rebalance --target`~~ Fixed to use --reason
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-cluster/commands.go`

### Policy CLI Docs vs Code

- [x] **Align policy output format options** ✅ FIXED
  - Resolution: doc
  - Updated `policy check` output formats to: text, json, yaml, table
  - Updated `policy report` output formats to: text, json, yaml, table
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Align policy show/audit output formats** ✅ FIXED
  - Resolution: doc
  - Updated `policy show` output formats to: text, json, yaml, table
  - Updated `policy audit` output formats to: text, json, yaml, table
  - Reference: `docs/content/en/docs/reference/cli.md`

### Files Server CLI Docs vs Code

- [x] **Align kscore-files server flags and global output docs** ✅ FIXED
  - Resolution: doc
  - ~~Docs show global `-o, --output` flag~~ Removed (doesn't exist)
  - ~~Docs show `serve --listen`~~ Removed (doesn't exist)
  - Added missing `--audit-level` and `--audit-output` flags to Global Flags
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-files/main.go`

- [x] **CLI reference documents kscore-files subcommands that do not exist** ✅ RESOLVED
  - Resolution: doc
  - TODO description was incorrect - code DOES implement all documented commands:
    - `serve`, `files`, `cache`, `namespace`, `version` (active)
    - `backend`, `mirrors` (deprecated, moving to kscore-files-storage)
  - No changes needed; docs correctly reflect implementation
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscore-files/main.go`

### Agents CLI Docs vs Code

- [x] **Document agent token list flags** ✅ FIXED
  - Resolution: doc
  - Added proper subcommand sections for agents token (create/list/revoke)
  - Added `--show-expired` flag to token list
  - Added `--ttl` and `--max-uses` flags to token create
  - Reference: `docs/content/en/docs/reference/cli.md`

- [x] **Document agent list filter flag** ✅ FIXED
  - Resolution: doc
  - Added complete flags table for agents list command
  - Documented all 6 flags: --status, --filter, --label, --edge, --limit, --show-compatibility
  - Added example using --filter flag
  - Reference: `docs/content/en/docs/reference/cli.md`

### kscorectl Core CLI Docs vs Code

- [x] **Align kscorectl global flags** ✅ FIXED
  - Resolution: doc
  - TODO description was incorrect - code has more flags than stated
  - Fixed docs to match actual flags: --server/-s, --config/-c, --format/-o, --verbose/-v, --quiet/-q, --timeout
  - Removed non-existent --api-key and --no-color flags
  - Changed --output to --format (correct flag name)
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscorectl/main.go`

- [x] **Document built-in kscorectl commands** ✅ FIXED
  - Resolution: doc
  - Code provides `health`, `api-key`, `maintenance`, and `benchmark` subcommands
  - health, api-key, and benchmark were already documented; added maintenance command docs
  - Reference: `docs/content/en/docs/reference/cli.md`, `cmd/kscorectl/main.go`

### Non-CLI Docs Referencing Missing Commands

- [x] **NATS mesh docs reference missing debug/agent NATS commands** ✅ FIXED
  - Resolution: doc
  - Replaced non-existent commands with working alternatives:
    - `kscore-agent nats *` → journalctl, kscorectl agents show, metrics endpoints
    - `kscorectl debug nats *` → nats CLI commands, journalctl, metrics
    - `kscore-agent restart` → systemctl restart instructions
    - `agent update-certs` → manual cert deployment with ansible/scp
    - `--group-by` → jq grouping of JSON output
  - Reference: `docs/content/en/docs/operations/nats-mesh-operations.md`, `docs/content/en/docs/concepts/nats-mesh.md`

- [x] **Runbooks/ops docs reference missing debug/db/conflict commands** ✅ FIXED
  - Resolution: doc
  - Replaced non-existent debug commands with working alternatives:
    - `debug db-status` → health endpoint, journalctl
    - `debug db last-write` → stat on database file
    - `debug db diff` → manual comparison with jq
    - `debug conflict *` → manual log review and state reapplication
    - `debug replay-commands` → manual review and replay procedure
    - `debug connections` → ss, nats server report, agents list
  - Reference: `docs/runbooks/bootstrap-new-cluster.md`, `docs/runbooks/disaster-recovery.md`, `docs/runbooks/capacity-scaling.md`

- [ ] **Runbooks reference many missing auth/cluster/config/diagnostics/certs/agent commands**
  - Resolution: doc
  - Examples: `kscorectl auth login`, `cluster token/quorum/health/uncordon`, `config set`, `diagnostics collect`, `certs *`, `agent quarantine/unquarantine/verify/ping`
  - Several runbooks rely on `audit search/export/analyze/timeline` and `security scan` commands not in CLI
  - Runbooks reference `backup status` and agent list flags like `--show-version`, `--show-cert-expiry`, `--count`, `--suspicious`
  - Update runbooks or implement commands
  - Reference: `docs/runbooks/*`

- [x] **Runbooks use exec syntax not supported by CLI** ✅ FIXED
  - Resolution: doc
  - Fixed `kscorectl exec "hostname" --target ... --limit 1` to `kscorectl exec run "role:webserver" -- hostname`
  - Reference: `docs/runbooks/disaster-recovery.md`

- [x] **Runbooks reference missing jobs/state commands** ✅ FIXED
  - Resolution: doc
  - Replaced non-existent commands with working alternatives:
    - `jobs list --scheduled` → `schedule list`
    - `state list --pending` → journalctl grep for state failures
    - `state status` → `state apply --dry-run`
    - `state list` → `state check` with known state file
  - Reference: `docs/runbooks/scheduled-maintenance.md`, `docs/runbooks/upgrade-cluster.md`, `docs/runbooks/troubleshooting.md`, `docs/runbooks/disaster-recovery.md`

- [x] **Runbooks use unsupported cluster flags** ✅ FIXED
  - Resolution: doc
  - Fixed unsupported flags:
    - `cluster health --node` → SSH to specific node and run `cluster health`
    - `cluster health --json` → `cluster status -o json`
    - `cluster leader --local` → `cluster leader` (removed --local flag)
  - Reference: `docs/runbooks/disaster-recovery.md`, `docs/runbooks/upgrade-cluster.md`

- [x] **Runbooks use cluster subcommands that don't exist** ✅ FIXED
  - Resolution: doc
  - Fixed non-existent subcommands across multiple runbooks:
    - `cluster token` → env var or config file reference
    - `cluster quorum` → `cluster status` (check has_quorum)
    - `cluster join-config` → removed, use cluster join
    - `cluster member add` → `cluster add`
    - `cluster uncordon` → `cluster undrain`
  - Reference: `docs/runbooks/*`

- [x] **Security training docs reference unimplemented admin commands** ✅ FIXED
  - Resolution: doc
  - Missing commands/flags: `config show/set --include-defaults`, `rbac list-roles/export`, `audit query/watch` with filters, `auth revoke-session`, `user disable`, `api-key rotate`, `agent re-enroll`, `agent revoke-credentials`, `backup restore --verify`, `exec run --immediate`, compliance CLI (`compliance report/scan/status/check`)
  - Update docs or add implementations
  - Reference: `docs/content/en/docs/operations/security-training.md`
  - Fixed: Replaced non-existent commands with actual equivalents (kscore-audit log with jq, kscore-backup verify/restore, kscore-policy compliance, config files). Removed commands handled by external IdP (auth revoke-session, user disable). Added useful missing commands to Future Work section in TODO.md.

- [x] **Runbook automation docs reference missing runbook commands** ✅ FIXED
  - Resolution: doc
  - Docs use `runbook execute`, `runbook status`, `runbook list-executions`, `runbook test`, `runbook audit *`
  - `kscore-runbook` only exposes approvals/interventions commands
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/reference/runbook-automation.md`
  - Fixed: Added notes explaining commands are planned, documented currently available commands (approvals, interventions), added missing commands to Future Work section in TODO.md

- [x] **Migration docs reference missing migrate/state commands** ✅ FIXED
  - Resolution: doc
  - Docs use `migrate convert-salt`, `migrate convert-pillar`, and `state validate`
  - Code only has `kscore-migrate run/validate/version` and `kscore-state` lacks `validate`
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/migrations.md`
  - Fixed: Added notes explaining that automated conversion tools are planned but not yet implemented, replaced conversion scripts with manual workflow guidance

- [x] **Multi-environment scenario references missing environment/promote/approve/metrics commands** ✅ FIXED
  - Resolution: doc
  - Commands like `kscorectl environment status/compare/sync`, `kscorectl promote ...`, `kscorectl approve ...`, `kscorectl metrics query`, `kscorectl rollback` do not exist
  - Scenario uses `kscorectl state export` and `kscorectl state restore` (no such commands)
  - Update docs or implement plugins
  - Reference: `docs/content/en/docs/scenarios/multi-environment.md`
  - Fixed: Added note clarifying this is a conceptual scenario with planned commands, directing users to GitOps workflow for current promotion capabilities

- [x] **Event-driven automation scenario references missing reactor/compliance/event subcommands** ✅ FIXED
  - Resolution: doc
  - Commands like `kscorectl compliance scan/report`, `kscorectl event emit` (singular), `events watch --filter`, `events trace`, `events queue-status`, `events dlq replay`, `reactor *`, `policy test` are not implemented
  - Scenario uses `agent drain` and `agent decommission` commands that do not exist
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/scenarios/event-driven-automation.md`
  - Fixed: Added note clarifying this is a conceptual scenario with planned CLI commands, noting basic event operations are available via events emit/list/show

- [x] **GitOps workflow scenario references missing gitops flags/commands** ✅ FIXED
  - Resolution: doc
  - Docs use `gitops status --deployment/--environment`, `gitops verify --deployment/--dry-run/--verbose`, `gitops history`, `gitops drift`, `gitops sync`, `gitops webhook-stats`, `gitops rollback-history`, `logs --component`
  - Code only supports `verify <workflow-file>`, `rollback`, `promote`, `status`, `repo`, `deploy`
  - Scenario uses `state restore --id` (no such command)
  - Update docs or implement commands/flags
  - Reference: `docs/content/en/docs/scenarios/gitops-workflow.md`
  - Fixed: Added note listing actual available gitops commands and recommending --help for current syntax

- [x] **Scenarios reference missing ping/exec flags** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl ping` and `kscorectl exec -t/-s` shorthand flags not supported by exec CLI
  - Update docs to use `kscorectl exec run --target` and remove ping, or implement
  - Reference: `docs/content/en/docs/scenarios/edge-deployment.md`, `docs/content/en/docs/scenarios/windows-infrastructure.md`, `docs/content/en/docs/scenarios/microservices-platform.md`
  - Fixed: Scenarios already use correct `exec run "target"` syntax; no ping or -t/-s shorthand flags found in referenced files

- [x] **Windows ops docs reference unsupported flags/commands** ✅ FIXED
  - Resolution: doc
  - `kscorectl exec run --shell` (no such flag), `state apply --check-only/--verbose` (unsupported), `kscore-agent --validate-config` (unsupported)
  - Update docs or implement flags/commands
  - Reference: `docs/content/en/docs/operations/windows.md`
  - Fixed: Changed `exec run --shell` to `exec run "target" -- powershell -Command`, replaced `--check-only` with `--dry-run`, replaced `--verbose` with `--preview`, replaced `--validate-config` with yamllint suggestion

- [x] **Windows installation docs reference non-existent agent subcommands** ✅ FIXED
  - Resolution: doc
  - Docs use `kscore-agent.exe install`/`uninstall` instead of `service-install`/`service-uninstall`
  - Update docs or add aliases
  - Reference: `docs/content/en/docs/operations/windows-installation.md`
  - Fixed: Changed `install` to `service-install` and `uninstall` to `service-uninstall` in windows-installation.md and windows-development.md

- [x] **Windows installation docs reference unsupported agent flag** ✅ FIXED
  - Resolution: doc
  - Docs use `kscore-agent.exe --console` (flag not defined in agent CLI)
  - Update docs or implement flag
  - Reference: `docs/content/en/docs/operations/windows-installation.md`
  - Fixed: Removed `--console` flag; running the binary directly already runs in foreground mode

- [x] **Secrets ops docs reference unsupported secrets commands** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl secrets verify` and `kscorectl secrets pentest run` (not implemented)
  - Docs use `kscorectl secrets get` for backends usage (no get command)
  - Update docs or add commands
  - Reference: `docs/content/en/docs/operations/secrets-security.md`, `docs/content/en/docs/operations/secrets-backends.md`
  - Fixed: Replaced non-existent secrets CLI commands with Vault CLI equivalents and kscore-secrets rotation commands. Added notes explaining Keystone focuses on rotation orchestration, not direct secret retrieval.

- [x] **Secrets backend setup docs reference unsupported config** ✅ FIXED
  - Resolution: doc
  - Docs define `backends.vault/*` and other secret backend config blocks
  - `internal/config.Config` has no secrets backend configuration section
  - Update docs or implement configuration support
  - Reference: `docs/content/en/docs/operations/secrets-backends.md`, `internal/config/config.go`
  - Fixed: Added implementation note explaining config blocks are conceptual patterns; actual auth uses environment variables and backend-native configuration

- [x] **Security ops docs reference missing auth/rbac/user/audit commands** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl auth login/refresh`, `kscorectl audit query`, `kscorectl rbac assign/list-roles/list-assignments`, `kscorectl user delete/reset-password`, `kscorectl export --user`
  - Docs use `kscorectl api-key revoke-all`, `kscorectl secrets reload`, `kscorectl agent quarantine`
  - Docs use `kscorectl module lock/unlock`, `module list --show-capabilities`, and `module capabilities show/export`
  - Docs use `kscorectl policy audit list/summary` (policy audit has no subcommands; uses flags)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/security.md`
  - Fixed: Replaced non-existent CLI commands with actual alternatives:
    - `audit query` → `kscore-audit log` with jq filtering
    - `rbac assign/list-roles/list-assignments` → Note about policy file configuration
    - `user delete/reset-password` → Note about external IdP handling
    - `api-key revoke-all` → Individual `api-key revoke` commands
    - `secrets reload` → Service restart or rolling restart
    - `agent quarantine` → `agents quarantine` (correct command)
    - `module lock/unlock/capabilities` → Capability policy YAML configuration
    - `policy audit list/summary` → `policy audit` with appropriate flags

- [x] **Secrets migration/troubleshooting docs reference missing secrets/agent/logs commands** ✅ FIXED
  - Resolution: doc
  - Secrets commands not implemented: `secrets put/get`, `secrets inventory`, `secrets migrate`, `secrets verify`, `secrets backend test/status/update/latency`, `secrets health`, `secrets lease *`, `secrets cache *`, `secrets metrics`, `secrets injection *`, `secrets history`, `secrets restore`, `secrets route set`, `secrets test *`
  - Rotation commands in docs use `secrets rotation ...` with `verify/complete/agents/logs/rollback-status` that don't exist; CLI uses `secrets rotate ...`
  - Agent/log commands not implemented: `agents refresh/ping/logs/config/secrets/restart`, `logs search/context/export`, `support bundle`
  - Docs also use `kscorectl exec <target> -- env` (no `exec` shorthand; should use `exec run`)
  - Update docs or implement commands
  - Reference: `docs/content/en/docs/operations/secrets-migration.md`, `docs/content/en/docs/operations/secrets-rotation.md`, `docs/content/en/docs/operations/secrets-troubleshooting.md`
  - Fixed: Added implementation notes and replaced non-existent commands:
    - secrets-migration.md: Added note that Keystone focuses on rotation, not direct storage; replaced `secrets put/get/inventory/migrate/test` with Vault CLI equivalents; fixed `exec` command syntax
    - secrets-troubleshooting.md: Added implementation note; replaced health/lease/cache/metrics commands with backend-native tools and journalctl; fixed `secrets rotation` to `secrets rotate`; replaced agent refresh/ping/logs/config with ssh/journalctl; replaced support bundle with manual collection

- [x] **Troubleshooting docs reference many missing commands/aliases** ✅ FIXED
  - Resolution: doc
  - Missing/incorrect commands: `kscorectl reactor *`, `kscorectl webhook status/events/logs`, `kscorectl gitops sync *`, `kscorectl approval *`
  - Missing policy subcommands: `policy evaluate`, `policy eval-cel`, `policy precedence`, `policy conflicts`, `policy profile`
  - Missing env/platform commands: `kscorectl cloud status`, `kscorectl k8s status`, `kscorectl env list`
  - Missing module commands: `module status`, `module capabilities`, `module signing-key show`, `module debug`, `module wasm status`
  - Missing cluster/agent/bootstrap commands: `cluster elect`, `cluster leadership-history`, `agent token validate`, `bootstrap regenerate-certs`, `bootstrap init-db`
  - Missing proxy/files/blueprint commands: `proxy status/credentials update`, `file status/transfers/verify/upload`, `blueprint status/logs/resources/can-rollback/deps`
  - Docs use singular `kscorectl event ...` but CLI uses `kscorectl events ...`
  - Update docs or implement commands/aliases
  - Reference: `docs/content/en/docs/operations/troubleshooting.md`
  - Fixed: Replaced non-existent commands throughout troubleshooting.md:
    - `kscorectl agent` → `kscorectl agents` (plural)
    - `policy check/eval-cel/precedence/conflicts/profile` → OPA CLI and policy file review
    - `cloud/k8s/env status` → cloud metadata endpoints and kubectl
    - `module status/capabilities/signing-key/debug/wasm` → journalctl and module show/verify
    - `cluster elect/leadership-history` → cluster leader and etcd commands
    - `proxy test/credentials` → manual testing and device show/update
    - `blueprint status/logs/resources/deps` → validate/lint/snapshot commands
    - `bootstrap regenerate-certs/init-db` → manual cert generation and journalctl

- [x] **Cluster management docs reference missing subcommands/flags** ✅ FIXED
  - Resolution: doc
  - Fixed: `leader --history` → use `cluster leader` and etcd CLI for history
  - Fixed: `members --role` → filter with jq: `.[] | select(.role=="learner")`
  - Fixed: `members --output wide` → `members --output yaml` (valid formats: table, text, json, yaml)
  - Fixed: `rebalance --max-concurrent/--delay` → `rebalance --reason` (rate limiting via config)
  - Fixed: `agent list` → `agents list`
  - Reference: `docs/content/en/docs/operations/cluster-management.md`

- [x] **Events docs reference missing event commands** ✅ FIXED
  - Resolution: doc
  - Fixed: Added spaces in CLI commands (e.g., `kscorectl eventsemit` → `kscorectl events emit`)
  - Fixed: `events list --show-sequence` → use NATS CLI for sequence info
  - Fixed: `events analyze --check-order` → use NATS CLI
  - Fixed: `events subscribers/storage-stats/prune/archive` → use NATS CLI and metrics endpoints
  - Reference: `docs/content/en/docs/concepts/events.md` (reference/events.md was already correct)

- [x] **Concepts events docs import non-existent events package** ✅ FIXED
  - Resolution: doc
  - Fixed: Removed external import statement, added note that package is internal
  - Added guidance to use CLI/HTTP API for external integrations
  - Reference: `docs/content/en/docs/concepts/events.md`

- [x] **Concepts agents docs reference missing agent/config commands** ✅ FIXED
  - Resolution: doc
  - Fixed: `config validate` → use Python yaml.safe_load or journalctl for validation
  - Note: `agents show` was already correct in docs (verified in code)
  - Reference: `docs/content/en/docs/concepts/agents.md`

- [x] **Concepts blueprints docs reference missing blueprint/config commands** ✅ FIXED
  - Resolution: doc
  - Fixed: Added implementation note listing available commands at top of document
  - Fixed: `blueprint build` → `blueprint publish`
  - Fixed: `blueprint list` → `blueprint search`
  - Fixed: `blueprint show` → `blueprint info`
  - Note: `applied`, `bundle`, `mirror`, `tree` commands remain as planned features in docs
  - Reference: `docs/content/en/docs/concepts/blueprints.md`

- [x] **Concepts state management docs reference missing state subcommands** ✅ FIXED
  - Resolution: doc
  - Added implementation note listing available commands
  - Fixed: `state render` → use `state check` or `state show`
  - Fixed: `state graph` → use `state show` and review manually
  - Removed: `state precompile`, `generate-test`, `benchmark*`, `baseline*` sections
  - Reference: `docs/content/en/docs/concepts/state-management.md`

- [x] **Docs use a state file schema not supported by parser** ✓ FIXED
  - Resolution: doc
  - Many docs/scenarios use `apiVersion/kind: state` with `resources:` and `type:` or top-level `states:`
  - `internal/statemgmt.Parser` expects module names at the top level (no `resources`/`states` wrapper)
  - Fixed state file examples in:
    - `tutorials/first-state.md` - all 4 examples
    - `tutorials/drift-detection.md` - all 2 examples
    - `scenarios/gitops-workflow.md` - webapp.yaml example
    - `scenarios/multi-environment.md` - webapp.yaml example
    - `scenarios/hybrid-infrastructure.md` - nginx and dns examples
    - `scenarios/database-ha.md` - postgresql, mysql, backup, monitoring examples
    - `reference/modules/dns.md` - all 5 DNS record examples
    - `concepts/cloud-platforms.md` - app config example
    - `community/announcement-0.1.0.md` - package/service example
    - `reference/modules.md` - file, package, service, user, group, cmd, cron module examples (partial - large file)
  - Changed format from `states: { state_id: { module: X } }` or `module: X` at top to `module_name: { state_id: { state: ... } }`
  - Changed `path:` to `name:` for file module states

- [x] **Docs use Salt-style module names not supported by statemgmt** ✓ FIXED
  - Resolution: doc
  - Examples used `file.managed`, `pkg.installed`, `service.running`, `cmd.run`, and `windows.*` resource types
  - Statemgmt modules are named `file`, `package`, `service`, `cmd`, etc., with states `present/absent/running/installed`
  - Fixed in:
    - `scenarios/edge-deployment.md` - 3 state file blocks with `file.managed`, `pkg.installed`, `service.running`
    - `concepts/blueprints.md` - 6 state file blocks with Salt-style module names
    - `tutorials/video-series-outlines.md` - 1 example with `file.managed`, `pkg.installed`
    - `operations/migrations.md` - all Keystone Core examples (Salt examples left as-is for comparison)
  - Also fixed schema issues: removed `apiVersion/kind/resources` wrappers, `module:` fields, `properties:` wrappers
  - Note: `windows.*` modules in windows-infrastructure.md covered by separate TODO item

- [x] **Windows scenario uses modules that do not exist** ✓ FIXED
  - Resolution: doc
  - Scenario used `windows.ad_user`, `windows.gpo`, `windows.dsc`, `windows.service`, `windows.registry`, `windows.firewall_rule`
  - Fixed in `docs/content/en/docs/scenarios/windows-infrastructure.md`:
    - Added implementation note at top listing implemented vs planned modules
    - Fixed `windows.service` → `win_service` with correct state file format
    - Fixed `windows.registry` → `win_registry` with correct format
    - Fixed `windows.firewall_rule` → `win_firewall` with correct format
    - Marked `ad_user`, `gpo`, `dsc` as planned features with conceptual API designs
    - Added PowerShell workarounds for planned features
    - Fixed state file schema throughout (removed apiVersion/kind/resources wrappers)
  - Added planned modules to `epics/20-windows-support.md` Future Enhancements section

- [x] **Concepts remote execution docs reference missing exec subcommands/flags** ✓ FIXED
  - Resolution: doc
  - Docs used `exec archive`, `exec export`, `exec cleanup`, and flags `--from-archive`, `--full`
  - CLI only supports `run/shell/script/async/cancel/history/output/status/list`
  - Fixed in `docs/content/en/docs/concepts/remote-execution.md`:
    - Removed non-existent `exec archive`, `exec export`, `exec cleanup` command sections
    - Removed `--from-archive` and `--full` flag examples
    - Updated output command examples to show actual flags (`--agent`, `--tail`, `--follow`, `-o`)
    - Added note that archival/export/cleanup commands are planned but not implemented

- [x] **Concepts modules docs reference missing module subcommands** ✓ FIXED
  - Resolution: doc
  - Docs used `module lock/unlock`, `module lock show`, `module list --show-capabilities`, `module capabilities export`, `module policy audit`
  - CLI only supports: init, build, validate, verify, sign, publish, install, resolve, tree, test
  - Fixed in `docs/content/en/docs/concepts/modules.md`:
    - Removed `module lock` and `module lock show` CLI examples
    - Removed `module unlock` CLI example, replaced with description
    - Removed `module list --show-capabilities`, `module policy audit`, `module capabilities export` examples
    - Updated compliance reporting section to reference file-based methods

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

- [x] **Additional concept docs reference missing commands** ✅ FIXED
  - Resolution: doc
  - Kubernetes concept doc uses `cluster status --cluster` (unsupported flag) - removed invalid flag
  - Message bus concept doc uses `kscorectl bench ...` - already fixed in previous session
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `docs/content/en/docs/concepts/message-bus.md`

- [x] **Kubernetes concept docs reference non-existent public package** ✅ FIXED
  - Resolution: doc
  - Docs refer to `pkg/k8s/client.go` and public APIs under `pkg/k8s`
  - Code lives under `internal/k8s` and is not exported
  - Fixed path to `internal/k8s/client.go` and added note about internal API
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/*`

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

- [x] **Kubernetes concept docs use wrong client method names** ✅ FIXED
  - Resolution: doc
  - Docs call `client.PodExec(...)`; client implements `ExecInPod`/`ExecInPods`/`StreamExecOutput`
  - Updated to use `client.ExecInPod(k8s.PodExecOptions{...})` with correct signature
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/client.go`

- [ ] **Kubernetes concept docs CRD schemas do not match code**
  - Resolution: doc
  - RemoteExecution example uses `target.labelSelector.matchLabels`, `namespaces`, `command.shell/script`, `retries`; code expects `Target` as `PodSelector` and `Command` as `[]string` with no retries
  - StateConfig example uses `id`, `state`, `driftDetection`; code expects `StateDeclaration{Name, Module, Parameters, Requisites}` and has no drift fields in CRD
  - Update docs or adjust CRD types/controllers
  - Reference: `docs/content/en/docs/concepts/kubernetes.md`, `internal/k8s/types.go`

- [x] **Operations IPv6 docs reference unsupported output format** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl agent list -o wide`; CLI does support `wide` format
  - Actual issue: command name was singular; fixed to `kscorectl agents list -o wide`
  - Reference: `docs/content/en/docs/operations/ipv6.md`

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

- [x] **Installation docs reference missing config validation command** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl config validate --config /etc/keystone-core/server.yaml`
  - Command EXISTS in `cmd/kscorectl/main.go` - no changes needed
  - Reference: `docs/content/en/docs/getting-started/installation.md`

- [x] **API reference docs reference missing auth command** ✅ FIXED
  - Resolution: doc
  - Docs use `kscorectl auth create-key` (no auth CLI; API keys via `kscorectl api-key create`)
  - Already uses correct `kscorectl api-key create` command - no changes needed
  - Reference: `docs/content/en/docs/reference/api.md`

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

- [x] **Docs reference non-existent GitHub org/module paths** ✅ FIXED
  - Resolution: doc
  - Multiple docs link to `github.com/keystone-core/...` and import paths under that org
  - Actual module path is `github.com/shawnbutts/keystone-core`
  - Fixed kubernetes.md and announcement-0.1.0.md to use correct org
  - References: `docs/content/en/docs/concepts/kubernetes.md`, `docs/content/en/docs/community/announcement-0.1.0.md`

- [x] **Event docs reference non-existent gitops webhook package path** ✅ FIXED
  - Resolution: doc
  - Docs mention `pkg/gitops/webhook` for event emission
  - Code lives under `internal/gitops/webhook`
  - Fixed path to `internal/gitops/webhook/`
  - Reference: `docs/content/en/docs/reference/events.md`

- [x] **Development docs reference non-existent logging package** ✅ FIXED
  - Resolution: doc
  - Docs import `github.com/shawnbutts/keystone-core/pkg/logging`
  - Logging package is under `internal/logging`
  - Fixed path to `internal/logging` with note about internal package
  - Reference: `docs/project/DEVELOPMENT.md`

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

- [x] **Fix Homebrew tap name mismatch** ✓ FIXED
  - Resolution: doc
  - Docs say `brew tap kscore/tap`
  - Repo generation docs/scripts use `keystonecore/tap`
  - References: `docs/content/en/docs/getting-started/installation.md`, `internal/repogen/generator.go`, `build/repos/macos/README.md`
  - Updated installation.md to use `keystonecore/tap` and `keystone-core`

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

| Category | Open | Fixed |
|----------|------|-------|
| High Priority | 8 | 11 |
| Medium Priority | 13 | 0 |
| Additional Documentation Drift | 151 | 0 |
| Low Priority | 169 | 0 |
| **Total** | **341** | **11** |
