# Epic 13: Clustering & HA — The v1.0 Differentiator

**Phase**: H • **Estimate**: 4 weeks • **Depends on**: 02, 03, 04, 05, 09 • **Blocks**: v1.0 release

## Goal

Run `kscore-server` as a 3-node cluster with leader election, automatic failover, consistent-hash agent assignment, NATS-fallback recovery, and split-brain prevention. **This is the commercial-trial-ready differentiator. Do not defer.**

## Scope (in)

### Topology (v1.0)

- 3 × `kscore-server`, each with embedded etcd (single binary deploy).
- 1 × Postgres (shared state).
- 1 × NATS cluster (or embedded NATS in each server with leaf links — v2.0).
- Production scaling path: 5 × CP + external 3-node etcd + Postgres replica + external NATS cluster.

### Core components (`internal/cluster/`)

- `EtcdClient` — wraps `etcd v3 client`; embedded or external mode; lease + watch; auto-sync.
- `MembershipManager` — ephemeral keys (lease TTL 15s); 5s heartbeat; observers; member metadata; load/watch members.
- `LeaderElector` — `concurrency.Election` primitives; campaign loop; resignation; transfer; observers.
- `ShardManager` — consistent hash ring (default 150 virtual nodes); agent → member by `hash(agentID)`; rebalance on topology change.
- `ShardStore` — etcd-backed `agentID → memberID` mapping, versioned for optimistic locking.
- `HealthMonitor` — consecutive-failure threshold (default 3); P50/P99 latency; partition detection via quorum loss.
- `FailoverManager` — detect failed members → reassign agents (batch 100) → reassign jobs (batch 50, idempotency keys); cooldown 10s.
- `SingletonTaskManager` — leader-only tasks: reactor coordinator (v1.1), scheduled jobs (v1.1), cleanup, metric aggregation, agent rebalance, audit retention enforcer.
- `RecoveryManager` — phases STARTING → CONNECTING → SYNCING → VERIFYING → REJOINING → RECLAIMING → COMPLETED.
- `FencingManager` — lease + epoch fencing; modes STRICT, READ_ONLY, GRACEFUL.
- `CoordinationServer/Client` — mTLS gRPC `CoordinationService` for server↔server; NATS-down recovery channel.
- `StateStore`, `ConfigStore`, `ShardStore` — etcd-backed namespaced stores (general state, hot-reload config, shard map).

### Lifecycle state machines

- Member: `HEALTHY → DEGRADED → UNHEALTHY → LEAVING → removed` (with `recover` arrow back to HEALTHY).
- Leader: `no leader → CAMPAIGNING → ELECTED → (TRANSFERRED|LOST)`.
- Failover: `IDLE → DETECTING → INITIATED → IN_PROGRESS → COMPLETED` (with `FAILED` and `ROLLED_BACK` terminal states).
- Recovery: 7 phases per above.
- Graceful shutdown: `RUNNING → INITIATED → DRAINING → TRANSFERRING → DEREGISTERING → COMPLETED`.

### APIs

- gRPC `ClusterService` (Epic 03 protos): `GetClusterStatus`, `ListMembers`, `GetMember`, `AddMember`, `RemoveMember`, `GetLeader`, `TransferLeader`, `Rebalance`, `CreateBackup`, `RestoreBackup`, `WatchMembership`, `WatchLeadership`.
- gRPC `CoordinationService` (mTLS-only): `ClusterHealth`, `GetLeader`, `NATSStatus`, `RecoveryCoordinate`, `Heartbeat`, `PropagateState`.
- REST: `/api/v1/cluster/{status,members,members/{id},leader,leader/transfer,rebalance,backup,restore}`.
- `kscore-cluster` CLI: `status`, `members`, `leader`, `add`, `remove`, `transfer-leader`, `rebalance`, `backup`, `restore`.
- `kscore-cluster-backup` CLI: `backup`, `restore`, `list`, `verify`, `schedule` (v1.1).

### Cluster backup

- Binary + JSON snapshot; cluster metadata, shard assignments, config; leader-initiated for ordering.

### HA resilience tests (CI on every PR)

- NATS-failure, etcd-failure, network-partition, split-brain.
- Performance SLOs verified: cluster forms <10s; first leader <3s; failover detection <5s; agent reassign <10s; minority blocks writes <1s; recovery <15s.

## Scope (out / non-goals)

- Backup automation/scheduling — v1.1.
- Comprehensive HA dashboard — v1.2 (basic status sufficient for v1.0).
- Read-only replicas — v1.4.
- Auto-scaling — v1.5.
- Multi-region clustering / federation — v2.0.
- Dynamic shard splitting under load — v2.x.
- Advanced topology (gateway / proxy members) — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.15`.

## Tasks

1. **`EtcdClient` wrapper** — embedded mode with `etcd server/v3` library; external mode connects to existing cluster; auto-sync; lease management.
2. **`MembershipManager`** — register on Start with ephemeral lease; heartbeat; LoadMembers; WatchMembers; AddObserver/RemoveObserver.
3. **`LeaderElector`** — `concurrency.Election`; Campaign in goroutine; Resign; TransferLeadership; observers fire on transitions.
4. **Consistent hash ring** + virtual nodes; `Add(member)`, `Remove(member)`, `Get(agentID) → member`.
5. **`ShardStore`** — etcd-backed agentID→memberID with version (optimistic locking).
6. **Rebalance algorithm** — minimal migration on member join/leave; cooldown 5s.
7. **`HealthMonitor`** — pluggable checkers (heartbeat, etcd, DB, NATS); consecutive-failure tracking; observers.
8. **`FailoverManager`** — detection → reassignment with idempotency keys; cooldown.
9. **`SingletonTaskManager`** — leader-only tasks; transition on leadership change.
10. **`RecoveryManager`** with 7 phases.
11. **`FencingManager`** — lease + epoch fencing; modes.
12. **`CoordinationServer`** (mTLS gRPC) — full RPC set.
13. **`CoordinationClient`** with connection pool + heartbeat tracking + retry with exp backoff.
14. **Graceful shutdown** sequence (5 phases) wired into Epic 04 server lifecycle.
15. **`ClusterService` gRPC + REST handlers**.
16. **`kscore-cluster` CLI** + **`kscore-cluster-backup` CLI**.
17. **HA E2E tests** — `test/e2e/ha_*_test.go`:
    - **NATSFailure**: stop NATS node, verify delivery via CoordinationService, restart, verify resumption.
    - **EtcdFailure**: stop etcd node, verify quorum maintained, restart.
    - **DatabaseFailover**: stop/restart Postgres; verify reconnect + zero data loss.
    - **NetworkPartition**: iptables partition; majority operates; minority blocks writes within 1s.
    - **SplitBrain**: symmetric partition; quorum enforcement; automatic healing.
18. **Performance verification** — every SLO metric checked in CI; failures block PR merge.

## Acceptance criteria

- [ ] 3-node cluster forms via embedded etcd; first leader elected in <3s.
- [ ] Adding a 4th member triggers minimal rebalancing (consistent hash); existing assignments mostly preserved.
- [ ] Removing a member triggers reassignment in <10s; agents reconnect to new shard owner with no command loss.
- [ ] Killing the leader: new leader elected in <3s; failover begins, completes in <10s; idempotency keys prevent double-execution.
- [ ] Network partition: minority partition blocks writes within 1s; majority continues serving.
- [ ] Restart of failed member: rejoins cluster in <15s; reclaims its shards from shard map.
- [ ] Graceful shutdown drains in-flight commands and transfers leadership; no agent disconnections seen by clients.
- [ ] `kscore-cluster status` shows member list, leader, health, quorum.
- [ ] `kscore-cluster-backup backup --output /tmp/backup` produces valid snapshot; `restore --input /tmp/backup --force` restores cleanly.
- [ ] CoordinationService rejects non-mTLS callers.
- [ ] All HA E2E tests pass on every PR; performance SLOs verified.
- [ ] Coverage >80% on `internal/cluster`.

## Risks

- **Heartbeat-timeout < 3× interval** = leader flapping. Default 5s/30s; CI must not allow tighter.
- **Slow NATS** → false failover. CoordinationService is the safety net.
- **etcd disk full** → cluster freezes. Document monitoring; emit `kscore_etcd_disk_used_bytes` metric.
- **Connection storms after failover** — rate limit + stagger reconnect at agent side.
- **Clock skew** → unexpected lease expiry. Document NTP requirement.
- **Backup at point-in-time** may catch mid-commit state — leader-initiated backup ensures ordering.
- **etcd embedded mode** scales to ≤3 members; external etcd required for 5+. Document.
- **Split-brain via clock skew or partition** — fencing is mandatory; tests must assert no double-leader scenarios.

## References

- PROJECT-DETAILS §4.15.
