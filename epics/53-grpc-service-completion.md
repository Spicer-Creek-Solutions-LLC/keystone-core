# Epic 53: gRPC Service Completion

## Overview

Several gRPC services are registered in `kscore-server` with nil dependencies or have RPCs that return `codes.Unimplemented`. This epic wires real backends into the services and implements the remaining RPCs.

## Goal

All registered gRPC services should be fully functional when the required infrastructure is available. Services that depend on optional infrastructure (cluster mode, secrets backends) should be conditionally registered only when their dependencies are configured.

## Success Criteria

- [ ] `SecretsServer` registered with real broker, lease manager, and transit backend when secrets config is present
- [ ] `ClusterServer` registered with real membership, leader, and shard providers when cluster mode is enabled
- [ ] `CoordinationService` registered when cluster mode is enabled
- [ ] `StateServer.GetStateHistory` implemented with persistent state history store
- [ ] `StateServer.GetStateStatus` implemented with persistent per-agent state store
- [ ] `ClusterServer.RestoreBackup` implemented with coordinated etcd/shard restore
- [ ] Conditional registration: services not registered when their deps are unavailable (instead of registering with nil and returning Unavailable)

## Dependencies

- **Epic 36** (Secrets Management): Secrets broker, lease manager, transit backend
- **Epic 11** (Clustering): Membership manager, leader election, shard store
- **Epic 3** (State Management): State execution history

## Technical Tasks

### Phase 1: Secrets Service Wiring (Week 1)

**T1.1: Wire SecretsServer with Real Dependencies**
- File: `cmd/kscore-server/main.go` line 366
- Currently: `NewSecretsServer(nil, nil, nil)`
- Fix: When secrets config is present, create a real `SecretBroker`, `LeaseManager`, and `TransitBackend` from the config and pass them to `NewSecretsServer`.
- When secrets config is absent, skip registration entirely instead of registering with nil deps.

**T1.2: Wire ClusterServer with Real Dependencies**
- File: `cmd/kscore-server/main.go` line 419
- Currently: `NewClusterServer(nil, nil, nil)`
- Fix: When cluster mode is enabled and etcd is configured, create real providers from the cluster coordinator and pass them to `NewClusterServer`.
- When not in cluster mode, skip registration.

**T1.3: Register CoordinationService in Cluster Mode**
- File: `cmd/kscore-server/main.go` line 424 (comment only)
- Currently: "available but not registered (requires cluster mode)"
- Fix: Register `CoordinationService` when cluster mode is enabled, backed by real `MembershipManager` and etcd client.

### Phase 2: State History Store (Week 2)

**T2.1: Create State History Store**
- New interface and SQLite/PostgreSQL implementation for persisting state execution history.
- Schema: `state_runs` table with run_id, state_path, agent_id, status, started_at, completed_at, result_json.
- Query support: by agent_id, by state_path, by time range, with pagination.

**T2.2: Wire GetStateHistory**
- File: `pkg/api/server/state_server.go` line 271
- Replace `codes.Unimplemented` with query against state history store.
- Pagination via cursor-based tokens (matching existing `ListEvents` pattern).

**T2.3: Wire GetStateStatus**
- File: `pkg/api/server/state_server.go` line 278
- Replace `codes.Unimplemented` with per-agent state status lookup.
- Return last run time, last result, drift status for the requested agent.

### Phase 3: Cluster Restore (Week 3)

**T3.1: Implement RestoreBackup RPC**
- File: `pkg/api/server/cluster_server.go` line 313
- Design: Accept backup JSON, coordinate with `ClusterMembershipProvider` to pause operations, restore etcd state, restore shard assignments, resume.
- Safety: Require explicit confirmation field in request. Log audit event before and after restore.
- This is the most complex task — may need to coordinate across cluster members via NATS.

### Phase 4: Conditional Registration Refactor (Week 4)

**T4.1: Refactor Service Registration**
- Replace nil-dep registrations with conditional blocks.
- Pattern: `if secretsCfg.Enabled { registerSecretsService(grpcServer, broker, leaseMgr, transit) }`
- Services not registered should not appear in gRPC reflection.
- Add health check entries for each conditionally registered service.

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| State history store adds schema migration | Use existing migration framework; add migration version |
| Cluster restore is a dangerous operation | Require confirmation field; gate behind admin role; comprehensive audit logging |
| Conditional registration changes gRPC reflection output | Document which services are available per deployment mode |

## Definition of Done

- [ ] All gRPC services either fully functional or not registered
- [ ] No `codes.Unimplemented` returns for RPCs that should work
- [ ] No nil-dep registrations in `kscore-server`
- [ ] State history store with SQLite and PostgreSQL backends
- [ ] Integration tests for service registration in each deployment mode
- [ ] `make test` and `make lint` pass
