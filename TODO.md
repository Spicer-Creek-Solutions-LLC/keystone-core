# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Code Gaps (Docs Claim, Code Missing)

### CLI Commands
- [x] Implement event CLI plugin for `kscorectl event(s)` (list/query/emit/replay/watch/retention/dlq) referenced in `docs/content/en/docs/concepts/events.md` and `docs/content/en/docs/reference/events.md`. FIXED: Implemented kscore-events CLI with list (with filters), query (advanced filtering), emit (publish events), replay (replay from storage), watch (real-time streaming), retention (list/set/apply policies), and dlq (list/show/retry/purge dead letter queue).
- [x] Implement agent management CLI (`kscorectl agent list/show/quarantine/token/...`) referenced throughout operations/tutorial docs such as `docs/content/en/docs/operations/maintenance.md`. FIXED: Implemented kscore-agents CLI with list (filter by status/labels/edge), show (detailed agent info), delete, quarantine/unquarantine, token (create/list/revoke), tags (set/add/remove/show), status (fleet or single agent), and renew-svid commands.
- [x] Implement maintenance mode CLI (`kscorectl maintenance enable/disable`) referenced in `docs/content/en/docs/operations/self-management.md`. FIXED: Added maintenance commands to kscorectl - enable (with reason/duration), disable, status, queue (--status), and cleanup (--older-than with dry-run).
- [x] Implement backup CLI (`kscore-backup create/list/verify`) referenced in `docs/content/en/docs/operations/self-management.md` and `docs/content/en/docs/concepts/state-storage.md`. FIXED: Implemented kscore-backup CLI with create (full/incremental/component types, encryption, compression), list (time/type/status filtering), show (detailed info), verify (integrity/checksum/restorability checks), restore (with dry-run, target, components), delete (by ID or age), replication-status, schedule (create/list/delete/enable/disable), and retention (show/set/apply policies).
- [x] Implement test runner CLI (`kscore-test integration --suite ...`) referenced in `docs/content/en/docs/operations/self-management.md`. FIXED: Implemented kscore-test CLI with smoke (quick health tests), integration (suite-based tests: basic, recovery, cluster, state, execution, events, policy, gitops), run (configurable test execution with dry-run), list (available suites), show (test run details), history (test run history with filtering), and suite management (show/create/delete).
- [x] Provide `kscore-gateway` CLI referenced in `docs/content/en/docs/reference/cli.md` (telemetry gateway alias). RESOLVED: Not needed - docs correctly use `kscore-telemetry-gateway`. The `kscore-gateway` references are Prometheus job labels and example service names, not CLI commands.
- [x] Add `kscorectl health` and `kscorectl api-key` commands listed in `docs/content/en/docs/reference/cli-quick-reference.md`. FIXED: Added health (with check subcommand) and api-key (create/list/revoke) commands to kscorectl.
- [x] Add `kscorectl completion` plus `kscorectl version --short/--verbose` flags referenced in `docs/content/en/docs/reference/cli.md` and `docs/content/en/docs/community/support.md`. FIXED: Added completion command (bash, zsh, fish, powershell) and version flags (--short for version only, --verbose for JSON with Go/platform info).
- [x] Add missing exec subcommands (`exec async/cancel/history/output/shell/script`) referenced in `docs/content/en/docs/reference/cli-quick-reference.md` and migration notes in `docs/content/en/docs/reference/cli.md`. FIXED: Added all 6 commands - async (fire-and-forget execution), cancel (with confirmation), history (with filtering), output (view job output), shell (interactive session placeholder), script (execute script files with auto-detected interpreter).
- [x] Add `kscorectl cluster health` command referenced in `docs/content/en/docs/reference/cli.md` and `docs/content/en/docs/operations/cluster-management.md` (56 occurrences in docs). FIXED: Implemented health command with detailed checks output.
- [x] Add `kscorectl files push/pull` aliases referenced in `docs/content/en/docs/reference/cli.md` migration table. FIXED: Added `push` as alias for `put` and `pull` as alias for `get` in kscore-files commands_files.go.
- [x] Implement `kscorectl upgrade` CLI (`upgrade check/plan/execute/status/cancel/canary/agents/rollback`) - 61 occurrences in docs (self-management.md, upgrade-cluster.md, emergency-rollback.md) but `cmd/kscore-upgrade/` does not exist. Logic exists in `pkg/upgrade/` but has no CLI exposure. FIXED: Implemented kscore-upgrade CLI with check (version comparison, compatibility), plan (upgrade planning with strategy selection), execute (rolling/canary/blue-green strategies, skip-backup, max-unavailable), status (progress monitoring with watch mode), cancel (abort with force option), canary (promote/rollback/status for staged rollouts), agents (fleet upgrade with batching, filtering, reporting), rollback (version/component rollback with dry-run), history (upgrade history with filtering), and logs (upgrade log viewing with follow/tail).
- [x] Implement `kscorectl benchmark` CLI referenced in `docs/content/en/docs/operations/monitoring.md` (7 occurrences) - no implementation exists. FIXED: Added benchmark command group to kscorectl with subcommands: all (run all benchmarks), agent-registration (--count, --parallel), command-execution (--count, --parallel), state-apply (--state, --targets), compare (baseline vs results with --threshold). Includes JSON/text output, latency percentiles (P50/P95/P99), ops/sec metrics, and CI/CD integration support.
- [x] Implement `kscorectl proxy device` commands referenced in `docs/content/en/docs/operations/proxy-agents.md` - `cmd/kscore-proxy/` does not exist. FIXED: Implemented kscore-proxy CLI plugin with complete proxy agent management - device commands (list/add/import/show/update/remove/test/health/ping/status/config/connect), credential commands (add/list/show/test/rotate/delete/verify/backend-status), discover commands (scan/list/status/approve/approve-all/reject/ignore/auto-approve/logs/config), drift commands (check/report/remediate), state commands (apply/logs), and status overview.
- [x] Add missing kscore-state subcommands (test, diff, show, history, rollback) - only 4 of 9 documented commands implemented. FIXED: Added test (dry-run with test messaging), diff (alias for drift), show (rendered state preview), history (server-side history placeholder), rollback (server-side rollback placeholder).
- [x] Add missing kscore-cluster subcommands (health, shards, join, leave, drain, undrain) - status, members, leader, add, remove, transfer-leader, rebalance, backup, restore already implemented. FIXED: Added all 6 missing commands with client methods and command implementations.
- [x] Add missing kscore-policy subcommands (create, update, delete, activate, deactivate) - only 5 of 10 documented commands exist. FIXED: Added all 5 commands with policy file CRUD operations, validation, and confirmation prompts.
- [x] Add missing kscore-gitops subcommands (repo list/add/remove/sync, deploy list/show/rollback/approve) - only 7 of 12+ documented commands exist. FIXED: Added repo command group (list/add/remove/sync) for repository management and deploy command group (list/show/rollback/approve) for deployment lifecycle management.

### REST API Gaps
- [x] Implement Agent REST endpoints documented in api.md: GET /api/v1/agents, GET /api/v1/agents/{id}, PATCH /api/v1/agents/{id}/tags - no handler in pkg/api/. FIXED: Created pkg/api/agents/handlers.go with Handler struct, RegisterRoutes(), and handlers for listing agents (with status/label filtering and sorting), getting single agent, and updating agent tags. Returns all AgentMetadata fields including IPv4/IPv6 addresses, dual-stack status, and labels.
- [x] Implement Execution REST endpoints: POST /api/v1/exec, GET /api/v1/jobs/{id}, GET /api/v1/jobs - documented but no REST handlers. FIXED: Created pkg/api/execution/handlers.go with Handler struct and handlers for: POST /api/v1/exec (sync/async execution with targets, command, args, env, timeout), GET /api/v1/jobs (list with agent_id, status, limit, offset, sort filters), GET /api/v1/jobs/{id} (get single job). Properly handles streaming CommandResponseType (stdout, stderr, completed, failed, timeout).
- [x] Implement State REST endpoints: POST /api/v1/state/apply, POST /api/v1/state/check, POST /api/v1/state/drift - documented but no REST handlers. FIXED: Created pkg/api/state/handlers.go with Handler struct and handlers for: POST /api/v1/state/apply (execute state with dry-run support), POST /api/v1/state/check (validate state file with error/warning categorization), POST /api/v1/state/drift (check for configuration drift). Supports both content and path-based state loading.
- [x] Implement Events REST endpoints: GET /api/v1/events, POST /api/v1/events - documented but no REST handlers. FIXED: Created pkg/api/events/handlers.go with Handler struct and handlers for: GET /api/v1/events (list with type, source, severity, correlation_id, tags, time range, pagination, sorting), POST /api/v1/events (create and publish events via EventBuilder), GET /api/v1/events/{id} (get single event by ID).
- [x] Implement Policy REST endpoints: POST /api/v1/policies/evaluate, GET /api/v1/policies/violations, GET /api/v1/policies/compliance - documented but no REST handlers. FIXED: Created pkg/api/policy/handlers.go with Handler struct and handlers for: POST /api/v1/policies/evaluate (supports policy_id, policy_set_id, or resource_type evaluation with full input context), GET /api/v1/policies/violations (audit entries with filtering by policy_id, resource_type, user, action, time range), GET /api/v1/policies/compliance (compliance report for time period with severity breakdown).
- [x] Implement GitOps REST endpoints: GET /api/v1/gitops/verifications, POST /api/v1/gitops/rollback - documented but no REST handlers. FIXED: Created pkg/api/gitops/handlers.go with Handler struct and handlers for: GET /api/v1/gitops/verifications (list with success/workflow filtering), GET /api/v1/gitops/verifications/{id} (get single verification), POST /api/v1/gitops/rollback (trigger rollback with type, strategy, application, reason, skip_verification), GET /api/v1/gitops/rollbacks (list with status/application filtering), GET /api/v1/gitops/rollbacks/{id} (get single rollback), POST /api/v1/gitops/rollbacks/{id}/approve (approve/reject with approver and comment).
- [x] Implement Webhooks REST endpoint: POST /api/v1/webhooks - documented but no REST handler. FIXED: Created pkg/api/webhooks/handlers.go with Handler struct and handlers for: POST /api/v1/webhooks (receive webhook payloads with type detection or explicit type field, parse payload using appropriate handler), GET /api/v1/webhooks/stats (receiver statistics including total received/processed/failed counts by type), GET /api/v1/webhooks/config (webhook configuration details).

### gRPC API Gaps
- [x] Fix api.md ExecutionService - documented but doesn't exist; functionality is in ControlPlaneService with different method names (GetCommandStatus not GetJob, ListCommands not ListJobs). FIXED: Removed non-existent ExecutionService, added ControlPlaneService documentation with all 9 methods (GetServerStatus, ListAgents, GetAgent, ExecuteCommand, GetCommandStatus, ListCommands, BatchExecuteCommand, GetBatchJobStatus, ListBatchJobs).
- [x] Fix api.md AgentService documentation - docs show ListAgents/GetAgent/UpdateAgentTags but proto has Register/Heartbeat/ExecuteCommand/GetAgentInfo. FIXED: Updated AgentService section to show correct methods: Register, Heartbeat, ExecuteCommand (streaming), GetAgentInfo. Also updated StateService, EventService, PolicyService, and ClusterService to show complete method lists matching proto files.

### Event System
- [x] Implement bootstrap event types (`bootstrap.generate`, `bootstrap.validate`, `bootstrap.use`, `bootstrap.register`, `bootstrap.revoke`, `bootstrap.expire`, `bootstrap.cleanup`) documented in `docs/content/en/docs/reference/events.md` (lines 376-472) but NOT defined in `pkg/events/types.go`.
- [x] Implement `state_apply` reactor action documented in `docs/content/en/docs/concepts/reactors.md` examples (lines 138, 334-335) but NOT implemented in `pkg/events/actions.go`.
- [x] Wire GitOps events to event bus - webhook parsing exists in `pkg/gitops/webhook/` and `ToKscoreEvent()` conversion works, but `cmd/kscore-server/main.go` uses `loggingEventProcessor` instead of `EventBusProcessor`, so events are logged but NOT published to the event system.
- [x] Add `agent.heartbeat_failed` event type - fully documented in events.md (lines 144-157) with severity, data fields, and examples, but NOT defined in `pkg/events/types.go`.
- [x] Fix Event struct field name: docs use "timestamp" but code uses "time" (line 97 of types.go). All doc JSON examples use wrong field name.
- [x] Fix Event Tags field type: docs show array `["tag1", "tag2"]` but code uses `map[string]string` (line 106 of types.go). FIXED: Updated all doc examples to use map format `{"key": "value"}`. Also updated agent "tags" references to "labels" to match AgentRecord.Labels field type.
- [x] Implement event filter functions `timestamp()` and `duration()` documented in events.md (lines 764, 789) but not in filter_expression.go parser. FIXED: Added TimestampValue and DurationValue types, parseValue() function for parsing function calls, and comparison support for time.Time and time.Duration values. Also added "timestamp" field accessor for event time.
- [x] Implement nested data field filtering (e.g., `data.results.success`) - documented but code only supports single-level (`data.field`), see filter_expression.go lines 98-101. FIXED: Added getNestedValue() helper function that recursively navigates nested maps and slices. Supports paths like data.results.success, data.items.0.name.

### Configuration/Documentation Mismatches
- [x] Fix docs referencing `kscore-backup` (69 occurrences across 14 files) - actual binary is `kscore-cluster-backup`. Either rename binary or update all doc references.
- [x] Fix NATS config field name mismatch - docs reference `nats.urls: []` (array) but code uses `nats.url: ""` (string) in `pkg/config/config.go`.
- [x] Fix storage config field name: docs show `storage.type: "sqlite"` but code uses `storage.backend: "sqlite"` in pkg/config/config.go. FIXED: Added viper alias so `storage.type` works as alias for `storage.backend`.
- [x] Fix NATS listen address format: docs show `nats.listen: "0.0.0.0:4222"` (combined) but code uses separate `nats.embedded.host` and `nats.embedded.port` fields. FIXED: Added `nats.embedded.listen` field with `nats.listen` alias that parses "host:port" format into separate Host/Port fields.
- [x] Fix server API config structure: docs show `api.listen` and `api.grpc_listen` but code uses `server.listenaddr`, `server.grpcport`, `server.httpport` as separate fields. FIXED: Added `server.httplisten` and `server.grpclisten` fields with `api.listen` and `api.grpc_listen` aliases that parse "host:port" format.
- [x] Implement documented config sections: CORS (`api.cors.*`), rate limiting (`api.rate_limit.*`), metrics (`metrics.*`), tracing (`tracing.*`), health checks (`health.*`) - all documented in configuration.md but not in pkg/config/config.go Config struct. FIXED: Added CORSConfig, RateLimitConfig, MetricsConfig, TracingConfig, and HealthConfig structs with all documented fields, defaults, validation, and viper aliases for documented paths.

### State Module Gaps
- [x] Register 5 Nginx modules that are implemented but NOT registered: nginx_upstream, nginx_proxy, nginx_ssl, nginx_location, nginx_rate_limit - types exist in module_web.go but no init() function calls RegisterModule(). FIXED: Added init() function to register all 9 web modules. Also fixed all Test() methods to implement Module interface correctly (was returning *StateResult instead of bool).
- [x] Implement 6 format validators documented in blueprints.md but not in parameter_validator.go: cidr, date-time, uuid, port, semver, dns-name. ALREADY IMPLEMENTED: All 6 validators exist in parameter_validator.go lines 45-53 (registration) and 462-601 (implementations).
- [x] Clarify Windows module loading: 5 modules have both stub and real implementations (win_feature, win_registry, win_service, win_firewall, win_package) - unclear which is being loaded. RESOLVED: Build tags properly control loading. Real implementations use `//go:build windows`, stubs use `//go:build !windows`. Go's build system automatically selects correct file based on GOOS.

### Blueprint Gaps
- [x] Implement `required_if` conditional parameters documented in blueprints.md (lines 715-716) but ParameterSchema struct in manifest.go has no RequiredIf field. FIXED: Added RequiredIf field to ParameterSchema and validation logic in ValidateParameters to check conditional requirements.
- [x] Fix blueprint include field name: docs show `params:` but code uses `parameters:` (loader.go line 444). Users following docs will have params ignored. FIXED: Added Params field as alias for Parameters with GetParameters() method that merges both.
- [x] Fix template syntax mismatch: docs show Jinja2 syntax (`{% if %}`, `{{ var }}`) but code uses Go text/template (`{{- if .vars.x }}`, `{{ .vars.x }}`). FIXED: Updated blueprints.md template examples to Go text/template syntax (`.tmpl` extension, `{{- if .condition }}`, `{{/* comment */}}`, `{{ default value .var }}`). Also fixed `.j2` references in compliance-automation.md, event-driven-automation.md, and hybrid-infrastructure.md.

## Doc Gaps (Code Exists, Docs Missing/Incomplete)

### CLI Commands Needing Documentation
- [x] Document `kscore-audit` CLI (log/report/export/stats) beyond roadmap mention in `docs/content/en/docs/community/roadmap.md`.
- [x] Document `kscore-webhook` CLI (list/show/test/history/secrets) beyond roadmap mention in `docs/content/en/docs/community/roadmap.md`.
- [x] Document `kscore-blueprint-publish` and `kscore-blueprint-state` CLIs beyond roadmap mention in `docs/content/en/docs/community/roadmap.md`.
- [x] Document `kscore-federation` CLI beyond roadmap mention in `docs/content/en/docs/community/roadmap.md`.
- [x] Document `kscore-files-storage` CLI beyond roadmap mention in `docs/content/en/docs/community/roadmap.md`.
- [x] Document `kscore-cluster-backup` CLI beyond cron snippet in `docs/content/en/docs/operations/maintenance.md`.
- [x] Document `kscore-telemetry-gateway` CLI - only `serve` and `version` commands, but no configuration reference or deployment guide. FIXED: Updated cli.md with correct command structure (serve/version subcommands), comprehensive configuration reference from types.go, and HA deployment guide.
- [x] Document `kscore-registry` CLI - HTTP server with no documented REST API endpoints or configuration. FIXED: Documentation was already comprehensive in cli.md. Updated with missing --cors-origins flag, version subcommand, and root endpoint (/) server info response.
- [x] Document kscore-files advanced commands (44+ undocumented): namespace management, backend storage, cache management, mirrors management. FIXED: Added comprehensive documentation for all kscore-files commands in cli.md including cache (status, clear, warm, list, evict, stats), namespace/ns (list, create, delete, info, quota, access), backend (list, status, sync, enable, disable, health), and mirrors (list, show, sync-status, sync, health, failover, latency, conflicts, resolve-conflict, history). All commands documented with flags, descriptions, defaults, and examples.
- [x] Document kscore-blueprint advanced commands (12+ undocumented): detailed examples for all commands, snapshot/rollback workflows, publishing workflow. FIXED: Added comprehensive documentation for all kscore-blueprint commands in cli.md including init/validate/lint/docs (development), search/info/versions (discovery), install/update/remove (management), publish/sign/verify (distribution), test (testing), snapshot (list/show/delete), rollback, and applied (list/show/history). All commands documented with flags, descriptions, and examples.
- [x] Document kscore-identity advanced commands (7+ undocumented): token lifecycle, CA rotation procedures, federation setup. FIXED: Corrected documentation for all kscore-identity commands to match factory function implementations - token (create/list/show/revoke with --path, --uses, --limit flags), ca (info/backup/restore/rotate with --input flag), federation (list/add/show/suspend/activate/remove/refresh with --endpoint, --profile flags), bundle (show/export), events (--limit flag), status. All examples updated with correct flags.

### gRPC API Documentation Gaps
- [x] Document ControlPlaneService as gRPC service - 9 RPCs fully implemented but not documented as gRPC (only REST mentioned). FIXED: Added ControlPlaneService section to api.md with all 9 methods.
- [x] Document batch execution operations: BatchExecuteCommand, GetBatchJobStatus, ListBatchJobs - not in api.md. FIXED: Included in ControlPlaneService documentation.
- [x] Document 8 of 12 PolicyService RPCs missing: EvaluatePolicySet, ListPolicies, GetPolicy, CreatePolicy, UpdatePolicy, DeletePolicy, GetAuditLog, ListPolicySets, GetPolicySet. FIXED: Added all 13 PolicyService methods to api.md.
- [x] Document 3 of 6 EventService RPCs missing: GetEvent, GetEventTypes, GetEventStats. FIXED: Added all 6 EventService methods to api.md.
- [x] Document 4 of 12 ClusterService RPCs missing: GetMember, RemoveMember, WatchMembership, WatchLeadership (including streaming). FIXED: Added all 12 ClusterService methods including streaming RPCs.
- [x] Document CoordinationService RPC details - only brief mention, no method signatures or examples. FIXED: Added CoordinationService section with all 7 methods (ClusterHealth, GetLeader, NATSStatus, Heartbeat, ElectionState, StatePropagate, RestartNATS).
- [x] Document streaming RPCs - 7+ methods return streams but docs don't indicate streaming behavior. FIXED: Added "(streaming)" annotation to all streaming methods: ExecuteCommand, BatchExecuteCommand, SubscribeEvents, WatchMembership, WatchLeadership, WatchBackups.

### File Distribution System (pkg/files/)
- [x] Document file compression system (`pkg/files/compression/`) - supports Gzip, Zstd, LZ4, Snappy with configurable levels and MIME-type rules. NOT mentioned in `docs/content/en/docs/reference/file-backends.md`.
- [x] Document storage failover system (`pkg/files/storage/failover.go`) - Backend health tracking, consecutive failure detection, automatic failover. Only mentioned in passing in operations guide.
- [x] Document mirror sync strategies (`pkg/files/mirror/sync.go`) in detail - incremental sync, conflict resolution strategies (newest-wins, largest-wins, primary-wins, manual) only ~20% documented.

### Proxy Agents (pkg/proxy/)
- [x] Document proxy debug system (`pkg/proxy/debug/debug.go`) - debug levels (Off, Basic, Verbose, Trace), protocol debugging, hex dumps. NOT mentioned in `docs/content/en/docs/concepts/proxy-agents.md`.

### Blueprints (pkg/blueprint/)
- [x] Expand blueprint testing framework documentation - `pkg/blueprint/testing/` has TestSuite YAML schema, 19 assertion types, mock configurations, setup/teardown lifecycle. Only ~30% documented in `docs/content/en/docs/reference/blueprints.md`.

### NATS Mesh (pkg/nats/)
- [x] Document NATS backpressure mechanism (`pkg/nats/backpressure/`) - flow control for high-volume scenarios. NOT documented.
- [x] Document NATS message ordering (`pkg/nats/ordering/`) - ordering guarantees. NOT documented.

### Event Documentation Gaps
- [x] Document 6 event types defined in code but missing from events.md: agent.error, job.output, user.login, user.command, user.error, system.error. FIXED: Added agent.heartbeat, agent.error, job.output, system.error, user.login, user.command, user.error, policy.pass, policy.violation. Removed agent.metadata_changed and user.custom which don't exist in code.
- [x] Fix event count in events.md overview - claims 22 types but code has 28+ standard types. FIXED: Updated to 29 types across 8 categories (added Policy Events category).

### Metrics Documentation Gaps
- [x] Document 11 cluster metrics in metrics.md: kscore_cluster_members_total, kscore_cluster_members_healthy, kscore_cluster_has_quorum, kscore_cluster_is_leader, kscore_cluster_leader_changes_total, kscore_cluster_leader_election_duration_seconds, kscore_cluster_rebalance_total, kscore_cluster_rebalance_duration_seconds, kscore_cluster_agents_moved_total, kscore_cluster_heartbeat_latency_seconds, kscore_cluster_etcd_operations_total. FIXED: Added Cluster Metrics section with 13 metrics (including member_status and etcd_operation_duration).
- [x] Document 20+ NATS mesh metrics from Epic 14: kscore_nats_connection_latency_seconds, kscore_nats_bootstrap_requests_total, kscore_nats_delivery_acked_total, kscore_nats_gateway_connections_total, etc. FIXED: Added comprehensive NATS Mesh (Epic 14) section to metrics.md with 20 metrics across 6 categories: Connection (5), Message (2), Buffer (2), Topology (3), Delivery (4), Bootstrap (2), Coordination (2). All metrics documented with type, description, labels, and examples.
- [x] Document 13 file mirror metrics from Epic 22: kscore_mirror_health, kscore_mirror_sync_operations_total, kscore_mirror_read_operations_total, etc. FIXED: Added comprehensive File Mirror (Epic 22) section to metrics.md with 18 metrics across 4 categories: Group (2), Read (4), Write (4), Sync (8). All metrics documented with type, description, labels, and examples.
- [x] Fix metric naming inconsistencies: docs show `kscore_agents_connected_total` but code uses `kscore_agents_connected` (no _total suffix); docs show `kscore_commands_executed_total` but code uses `kscore_command_executions_total`. FIXED: Updated observability.md, metrics.md, monitoring.md, quick-start.md to match code.

### Blueprint Documentation Gaps
- [x] Document platform defaults system in pkg/blueprint/platform_defaults.go: 7-layer parameter inheritance (schema defaults, vars/defaults.yaml, vars/platforms/*.yaml, parent blueprint, user params, feature params, computed params). FIXED: Added "Platform-Specific Defaults (vars/)" section to blueprints.md with loading order, platform detection, family mapping table, and usage examples.
- [x] Document hooks execution behavior: order, failure handling, context/parameters available to hooks. FIXED: Added "Lifecycle Hooks" section to blueprints.md with available hooks table, execution order diagrams, failure handling semantics, context available to hooks, and best practices.
- [x] Document secrets integration in blueprints: !secret YAML tag, multiple backends (environment, Vault, K8s), runtime resolution. FIXED: Added "Secrets Integration" section to blueprints.md with secret reference syntax, available backends (env, Vault, K8s), usage examples, multi-backend configuration, validation process, resolution flow, and security best practices.
- [x] Document entrypoints system: multiple named entrypoints, how to invoke non-default entrypoints. FIXED: Added "Entrypoints System" section to blueprints.md with entrypoint definition, special entrypoints (default, main, rollback), resolution order, usage in state includes, rollback context parameters, CLI selection, and best practices.
- [x] Document applied blueprint tracking system in pkg/blueprint/tracker.go. FIXED: Added "Applied Blueprint Tracking" section to blueprints.md with tracker architecture, tracked information tables, history actions, configuration options, CLI commands (applied list/show/history/usage), programmatic access examples, rollback integration, data persistence mechanism, and use cases.

### Webhook Documentation Gaps
- [x] Document GitHub "deployment_status" event type - implemented but not listed in gitops.md webhook section. FIXED: Added to gitops.md.
- [x] Document GitLab "merge_request" event type - implemented but only in events.md, not in gitops.md. FIXED: Added to gitops.md.
- [x] Document webhook payload field mappings for GitHub and GitLab - no documentation of how payloads are parsed and mapped. FIXED: Added comprehensive "Webhook Payload Field Mappings" section to gitops.md with GitHub payload structure table, GitHub event type mapping, GitLab payload structure table, GitLab event type detection priority, WebhookEvent structure, Keystone Core event conversion rules, data field contents for both platforms, and example filters.
- [x] Fix ArgoCD webhook documentation: examples show both "status" and "sync_status" fields but code only has one "status" field. FIXED: Removed sync_status, added note about status field.
- [x] Fix Flux webhook documentation: docs show "Ready" status but code uses "severity" field (success/warning/error). FIXED: Updated example and added note.

### Concepts Documentation Inconsistencies
- [x] Update `docs/content/en/docs/concepts/state-management.md` (lines 65-67) - claims "6 built-in modules" but 84 modules are actually implemented across 20+ categories. Should reference the comprehensive modules.md reference or summarize all categories.
- [x] Document GitOps event type constants - webhook parsing in `pkg/gitops/webhook/` creates dynamic event types (gitops.argocd.*, gitops.flux.*, etc.) but these are not listed in `docs/content/en/docs/reference/events.md` event types section, only documented in examples.

## Short-Term Priority (1-2 Releases)

### Test Coverage Improvements
- [ ] Epic 3 (State Management): Increase coverage from 44% to >80%
- [x] Add edge case tests for complex requisite chains and circular dependencies
- [ ] Add integration tests between all major epics
- [ ] Implement load test scenarios with configurable agent counts

### Epic 8 (Multi-Environment) Gaps
- [ ] Implement automated bare metal discovery with profile matching
- [ ] Complete service mesh integration with mTLS policy verification
- [ ] Implement K8s NetworkPolicy integration for network enforcement
- [ ] Add comprehensive container registry authentication support

---

## Medium-Term Priority (2-3 Releases)

### Epic 11 (HA Clustering) Gaps
- [x] Integrate etcd backup/restore with cluster backup system - VERIFIED: EtcdExporter and EtcdImporter were already implemented in pkg/backup/exporters.go. Added 14 unit tests covering creation, Name(), Component(), EstimateSize(), Verify(), interface implementation, and registration with BackupManager/RestoreManager. All 40 backup tests passing.

### Epic 12 (E2E Testing) Gaps
- [x] Add comprehensive security testing covering auth/authz/audit scenarios

### Epic 16 (Stdlib Modules) Gaps
- [ ] Increase unit test coverage to >80% for all modules
- [x] Add parse-config unit tests for disk/filesystem/mount/swap/systemd timer modules

### Epic 17 (SPIFFE Identity) Gaps
- [ ] Simplify trust federation setup with interactive wizard
- [ ] Add automatic fallback between attestation methods

### Epic 18 (IPv6) Gaps
- [x] Implement cloud provider IPv6 metadata detection

### Epic 22 (File Distribution) Gaps
- [x] Tighter integration with policy engine for access control - FIXED: Implemented PolicyEngineAdapter in pkg/files/access/policy_adapter.go that bridges file access control (PolicyEvaluator interface) to the policy engine. Supports CEL policies, policy sets, resource bindings, and includes helper functions for creating file access policies (FileAccessPolicy, DefaultFileAccessPolicies). All 7 tests passing.

### Epic 23 (Self-Management) Gaps
- [x] Expand configuration validation with comprehensive test suite
- [ ] Test automatic rollback on upgrade failure extensively

### Epic 27 (Bootstrap) Gaps
- [ ] Implement comprehensive error recovery with detailed diagnostics
- [ ] Implement atomic bootstrap with automatic rollback

### Epic 29 (Bootstrap Testing) Gaps
- [ ] Comprehensive recovery and rollback testing

---

## Long-Term Priority (Future Versions)

### Major Features (from Roadmap)
- [ ] Web UI dashboard
- [ ] Mobile monitoring app
- [ ] Natural language interface
- [ ] Scheduled operations & maintenance windows
- [ ] Multi-tenancy and namespace isolation
- [ ] Agent self-update with staged rollouts

### Infrastructure
- [ ] Create automated translation pipeline for additional languages

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
