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
   _(landed: new **`internal/cluster/`** package + first dep of the epic — `go.etcd.io/etcd/{client/v3,server/v3,api/v3}` v3.6.11 (comparison in the plan: gossip/raw-raft/Postgres-lock alternatives can't meet the quorum/fencing acceptance; PROJECT-DETAILS §3.2 updated). `EtcdClient`: embedded mode runs an in-process `embed.Etcd` (single-member bootstrap — multi-member join is Task 2's MembershipManager), waits `Server.ReadyNotify()` within `StartTimeout`, wires the client via `etcdserver/api/v3client` (no network hop); external mode `clientv3.New` + fail-fast `Status` probe → `ErrEtcdUnavailable`. Single-use lifecycle (created→started→stopped; double-Start→`ErrAlreadyStarted`, Start-after-Stop→`ErrStopped`, Stop idempotent, ops pre-start→`ErrNotStarted`), mutex-guarded. Lease grant/keepalive(worker-ctx, gosec-G118-clean)/revoke (revoke idempotent on unknown lease); thin Put/Get/Delete/Watch/Txn passthrough; `Client()` exposes `*clientv3.Client` for Task 3's `concurrency.Election`. New **`config.ClusterConfig`** (`cluster.enabled` **false by default — opt-in**; embedded|external validation; lease-ttl floor — the heartbeat≥3× rule lands with Task 2). `internal/cluster` 85.7% cov (>80% gate), race-clean; full `make lint`/test/docs-lint green. Tasks 2-18 NOT started — one-task-one-PR.)_
2. **`MembershipManager`** — register on Start with ephemeral lease; heartbeat; LoadMembers; WatchMembers; AddObserver/RemoveObserver.
   _(landed: `internal/cluster/membership.go` on Task 1's `EtcdClient` — **no new dep**. `Member` + `MemberStatus` state machine (`HEALTHY→DEGRADED→UNHEALTHY→LEAVING`, recover edge; validated `canTransition`; Task 2 drives only HEALTHY/LEAVING + a `SetStatus` seam, the DEGRADED/UNHEALTHY edges are HealthMonitor's in Task 7). `Register` grants an ephemeral lease + keepalive + writes `<prefix>/members/<id>` `WithLease`; heartbeat loop (worker-ctx, gosec-G118-clean) refreshes `LastHeartbeat`; `LoadMembers`/`GetMember` (malformed-skip, `ErrMemberNotFound`); single shared watch (`WithPrefix`+`WithPrevKV`) → `MemberEvent{Joined|Updated|Left}` fan-out to `MembershipObserver`s (`AddObserver`/`RemoveObserver`, snapshot dispatch mirroring the RunObserver convention); `WatchMembers(ctx)` = channel adapter over one shared watch, auto-removed + closed on ctx cancel (drops on slow consumer). **Stable member ID** persisted to `MemberIDFile` (UUIDv7; survives restart so RecoveryManager/Task 10 reclaims shards). `Deregister`/`Stop` announces LEAVING then revokes the lease; single-use lifecycle (`ErrNotRegistered`/`ErrAlreadyStarted`/`ErrStopped`, idempotent Stop). **Anti-flap guard (the Task 1 deferral, now closed):** `config.ClusterConfig` gains `membership.{heartbeat_interval,key_prefix}` and `Validate` rejects `lease_ttl_seconds < 3× heartbeat` (risk-list "CI must not allow tighter"); defaults 5s/15s sit exactly at 3×; the `MembershipConfig` runtime mirror enforces it too. `internal/cluster` 84.0% cov (>80% gate), race+lint+docs-lint green. Lease-expiry crash detection deferred to the HA E2E suite (Task 17). Tasks 3-18 NOT started.)_
3. **`LeaderElector`** — `concurrency.Election`; Campaign in goroutine; Resign; TransferLeadership; observers fire on transitions.
   _(landed: `internal/cluster/election.go` on Task 1's `EtcdClient.Client()` — **no new dep** (`go.etcd.io/etcd/client/v3/concurrency` is a client/v3 subpkg). `LeaderState` SM (`Unknown→Campaigning→Elected→{Resigned|Transferred|Lost}→Campaigning`); single campaign-loop goroutine (worker-ctx) owns the `*Session`/`*Election` (no cross-goroutine races): `Campaign` → `Elected`; in-leader `select` on `workerCtx.Done()` (resign+exit) / `session.Done()` (Lost → recreate session → re-campaign) / resign-or-transfer cmd. `Resign` re-enters the queue; `TransferLeadership` = resign + `ReCampaignDelay` so a peer wins; both are no-ops if not leader (no blocking). `IsLeader()`/`State()`/`LeaderID(ctx)` (→`ErrNoLeader`); `LeadershipObserver` fan-out (snapshot dispatch, AddObserver/RemoveObserver) per the MembershipObserver convention. Single-use lifecycle (`ErrNotStarted`/`ErrAlreadyStarted`/`ErrStopped`, idempotent Stop). New `config.ClusterElectionConfig` (`session_ttl_seconds` default 3 = the "<3s leader" SLO target; `recampaign_delay` default 1s; separate from the membership anti-flap lease — SLO tuning is Task 18). **`IsLeader()` is the `func() bool` the `WithRetentionLeaderCheck` seam wants — wiring it (replacing `AlwaysLeader` in internal/audit + internal/events) is Task 9's job, NOT done here.** `internal/cluster` 82.3% cov (>80% gate), race+lint+docs-lint green. Hard-crash session-lease-expiry failover (vs graceful Resign/Stop, both tested) deferred to HA E2E (Task 17) + SLO verification (Task 18). Tasks 4-18 NOT started.)_
4. **Consistent hash ring** + virtual nodes; `Add(member)`, `Remove(member)`, `Get(agentID) → member`.
   _(landed: `internal/cluster/hashring.go` — **pure data structure, no etcd, no new dep** (stdlib `hash/fnv` FNV-1a-64; rejected xxhash/blake as unjustified deps, crc32 as weaker). `HashRing`: `Add`(idempotent)/`Remove`(no-op-if-absent)/`Get`(binary search + wrap)/`Members`/`Has`/`Len`, RWMutex (RLock lookups, Lock topology). **Deterministic rebuild from the sorted member set** → identical key→member mapping on every node regardless of Add/Remove order (required for cluster-wide ownership agreement + Task 6 minimal-migration); vnode-hash collisions resolve to the lexicographically-smallest member, order-independent. New `config.ClusterShardConfig.virtual_nodes` (default 150 per §4.15, `Validate` rejects <1); `NewHashRing(≤0→DefaultVirtualNodes)`. Tests assert stability, order-independence, distribution sanity, and the acceptance criterion — adding a 4th member moves only ~1/4 of keys and **only onto the newcomer** (never reshuffles survivors); remove redistributes only the departed member's keys. `internal/cluster` 84.4% cov (hashring fully covered), race+lint+docs-lint green (pre-existing unrelated `internal/statemgmt/stdlib/service` -race flake is ROADMAP-logged — not in this diff). ShardStore (T5), rebalance+cooldown+ShardManager composition (T6), FailoverManager (T8) NOT started.)_
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
