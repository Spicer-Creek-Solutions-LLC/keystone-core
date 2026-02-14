# Epic 53: gRPC Service Completion

## Overview

Several gRPC services were registered in `kscore-server` with nil dependencies or had RPCs that returned `codes.Unimplemented`. This epic wired real backends into the services and implemented the remaining RPCs.

## Goal

All registered gRPC services should be fully functional when the required infrastructure is available. Services that depend on optional infrastructure (cluster mode, secrets backends) are conditionally wired with real deps when configured.

## Success Criteria

- [x] `ClusterServer` registered with real membership, leader, and shard providers when cluster mode is enabled
- [x] `CoordinationService` registered when cluster mode is enabled
- [x] `StateServer.GetStateHistory` implemented with persistent SQLite state history store
- [x] `StateServer.GetStateStatus` implemented with persistent per-agent state store
- [x] `SecretsServer` wired with real broker when `secrets.enabled` is true
- [x] `ClusterServer.RestoreBackup` has improved error message referencing `kscorectl cluster-backup restore`
- [x] Integration tests for state history wiring

## Dependencies

- **Epic 36** (Secrets Management): Secrets broker
- **Epic 11** (Clustering): Membership manager, leader election, shard store
- **Epic 3** (State Management): State execution history

## Technical Tasks

### Phase 1: Cluster Service Wiring - COMPLETE

- Hoisted cluster infrastructure (etcdClient, membership, leader, health) above both gRPC and REST registration blocks
- Wire `ClusterServer` with real `MembershipManager`, `LeaderElector`, and `ShardManager` (via `shardManagerAdapter`) when cluster mode enabled
- Simplified REST cluster handler to reuse hoisted vars
- Added `shardManagerAdapter` type to bridge `ShardManager.Rebalance` to `ClusterShardProvider.TriggerRebalance`

### Phase 2: CoordinationService Registration - COMPLETE

- Register `CoordinationService` when cluster mode is enabled with real deps
- Added `natsStatusAdapter` bridging `natsmgr.Manager` to `cluster.NATSStatusProvider`
- Removed "available but not registered" log message

### Phase 3: State History Store - COMPLETE

- Created `internal/statemgmt/history/` package:
  - `store.go`: `Store` interface, `StateRunRecord`, `StateStatusRecord`, `ListFilter` types
  - `sqlite.go`: SQLite implementation with WAL mode, `state_runs` and `state_status` tables, pagination, upsert
  - `sqlite_test.go`: 9 tests (CRUD, filters, pagination, upsert, edge cases)
- Modified `pkg/api/server/state_server.go`:
  - Added `StateHistoryStore` interface, `StateHistoryRun`, `StateHistoryFilter`, `StateStatusEntry` types
  - Added `SetHistoryStore()` setter
  - Implemented `GetStateHistory` with filter conversion, pagination, proto mapping
  - Implemented `GetStateStatus` with status queries and last-checked tracking
- Wired in `cmd/kscore-server/main.go` via `historyStoreAdapter`
- Updated state_server_test.go: replaced `codes.Unimplemented` tests with `codes.Unavailable` + mock store tests

### Phase 4: Secrets Service Wiring - COMPLETE

- Added `SecretsConfig` to `internal/config/config.go`: Enabled, DefaultBackend, CacheEnabled, CacheTTL
- Viper env bindings: `KSCORE_SECRETS_ENABLED`, `KSCORE_SECRETS_DEFAULT_BACKEND`, `KSCORE_SECRETS_CACHE_ENABLED`, `KSCORE_SECRETS_CACHE_TTL`
- Validation: require `default_backend` when enabled, reject negative `cache_ttl`
- Wire `SecretsServer` with real `BrokerBuilder.Build()` when `secrets.enabled` is true
- LeaseManager and TransitBackend remain nil (require vault/KMS backends not yet configurable)
- Config validation tests added

### Phase 5: Cleanup + Docs - COMPLETE

- Improved `RestoreBackup` error message to reference `kscorectl cluster-backup restore` CLI
- Integration tests: `TestIntegration_StateHistory_WithStore`, `TestIntegration_StateStatus_WithStore`
- Documentation updates: epic doc, AGENTS.md, API reference, configuration reference

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| State history store adds new SQLite DB file | Uses `deriveDataPath` pattern, same directory as main DB |
| Secrets broker creation may fail if no backends configured | Graceful fallback: log error, keep nil deps, RPCs return Unavailable |
| CoordinationService requires MembershipManager (non-nil guard in constructor) | Only registered when `clusterMembership != nil` |

## Definition of Done

- [x] ClusterServer wired with real deps when cluster mode enabled
- [x] CoordinationService registered when cluster mode enabled
- [x] GetStateHistory and GetStateStatus implemented with SQLite store
- [x] SecretsServer wired with real broker when configured
- [x] Integration tests pass
- [x] `make test` and `make lint` pass
