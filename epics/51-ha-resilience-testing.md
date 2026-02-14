# Epic 51: HA Resilience Testing

## Overview

Implement infrastructure failure and network partition E2E tests for the HA cluster topology. These tests were originally scoped as T4.4 of Epic 45 (Control Plane Config Wiring) but deferred due to their dependency on additional container infrastructure (dedicated NATS nodes, etcd nodes, PostgreSQL replicas, and network partition tooling).

## Goal

Validate that Keystone Core's HA cluster handles infrastructure failures gracefully: NATS node loss, etcd node loss, PostgreSQL failover, network partitions, and split-brain prevention.

## Success Criteria

- [x] HA harness supports `StopService(ctx, name)` / `StartService(ctx, name)` for any docker-compose service
- [x] `TestHACluster_NATSFailure`: kill NATS node, verify message delivery continues, restart and verify rejoin
- [x] `TestHACluster_EtcdFailure`: kill etcd node, verify leader election, restart and verify rejoin
- [x] `TestHACluster_DatabaseFailover`: PostgreSQL stop/restart with graceful degradation and recovery
- [x] `TestHACluster_NetworkPartition`: iptables-based network partition, verify majority operates
- [x] `TestHACluster_SplitBrain`: symmetric network partition with split-brain prevention verification

## Dependencies

- **Epic 11** (Clustering): HA cluster coordination, leader election, etcd integration
- **Epic 45** (Control Plane Config Wiring): E2E test infrastructure, config sections

## Technical Tasks

### Phase 1: HA Harness Extensions (Week 1)

**T1.1: Service Control Methods**
- Add `StopService(ctx context.Context, name string) error` to `HAClusterEnvironment`
- Add `StartService(ctx context.Context, name string) error` to `HAClusterEnvironment`
- Implement via `docker compose stop <service>` / `docker compose start <service>`
- Add `WaitForServiceHealthy(ctx, name, timeout)` to verify service recovery

**T1.2: HA Compose Topology Updates**
- Add dedicated NATS containers (nats-1, nats-2, nats-3) to HA compose file
- Add dedicated etcd containers (etcd-1, etcd-2, etcd-3) to HA compose file
- Configure NATS cluster routes and etcd peer URLs
- Ensure services have health checks for startup coordination

### Phase 2: Infrastructure Failure Tests (Week 2)

**T2.1: NATS Node Failure**
- Implement `TestHACluster_NATSFailure`:
  1. Stop nats-1 container
  2. Execute commands via control plane → verify delivery via nats-2/nats-3
  3. Verify JetStream operations still work (event persistence)
  4. Restart nats-1 → verify it rejoins the NATS cluster
  5. Verify message delivery works through all nodes again

**T2.2: etcd Node Failure**
- Implement `TestHACluster_EtcdFailure`:
  1. Stop etcd-1 container
  2. Verify leader election completes on remaining nodes
  3. Verify cluster state operations (read/write) continue
  4. Restart etcd-1 → verify it rejoins the etcd cluster
  5. Verify state consistency across all etcd nodes

**T2.3: PostgreSQL Failover**
- Add PostgreSQL replica to HA compose topology
- Implement `TestHACluster_DatabaseFailover`:
  1. Verify primary is handling writes
  2. Stop primary PostgreSQL container
  3. Verify replica promotes to primary (or connection failover)
  4. Verify state operations continue
  5. Restart original primary → verify it rejoins as replica

### Phase 3: Network Partition Tests (Week 3)

**T3.1: Container Network Tooling**
- Add iptables and tc (traffic control) to container images
- Create helper functions for network manipulation:
  - `PartitionNetwork(ctx, fromService, toService)` — block traffic between services
  - `HealPartition(ctx, fromService, toService)` — restore traffic
  - `AddLatency(ctx, service, latencyMs)` — inject network latency

**T3.2: Network Partition Test**
- Implement `TestHACluster_NetworkPartition`:
  1. Create network partition between server-1 and server-2/server-3
  2. Verify that the majority partition continues operating
  3. Verify the minority partition detects isolation
  4. Heal partition → verify cluster reunification
  5. Verify state consistency after partition heals

**T3.3: Split-Brain Prevention**
- Implement `TestHACluster_SplitBrain`:
  1. Create symmetric network partition (two groups of servers)
  2. Verify only one partition accepts writes (majority wins)
  3. Verify minority partition rejects or queues operations
  4. Heal partition → verify automatic reconciliation
  5. Verify no data loss or corruption

## Testing Strategy

- All tests require `KSCORE_E2E_TESTS=1` environment variable
- Tests use the HA compose topology (`test/e2e/containers/docker-compose.ha.yml`)
- Each test starts from a healthy cluster state
- Network partition tests require `NET_ADMIN` capability in containers
- Tests should be idempotent — clean up partitions/stopped services on failure

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Network partition tooling requires privileged containers | Use `cap_add: NET_ADMIN` in compose, document security implications |
| PostgreSQL failover is complex to set up | Start with connection-level failover, add streaming replication later |
| Tests may be flaky due to timing | Use generous timeouts, retry-with-backoff for health checks |
| CI environment may not support privileged containers | Gate network partition tests behind `KSCORE_E2E_PRIVILEGED=1` |

## Definition of Done

- [x] HA harness has StopService/StartService/WaitForServiceHealthy/ExecInService methods
- [x] All 5 HA resilience tests implemented and un-skipped
- [x] HA compose topology includes dedicated NATS, etcd, and PostgreSQL containers with NET_ADMIN
- [x] Network partition tests work with iptables tooling (gated by KSCORE_E2E_PRIVILEGED=1)
- [x] Tests compile cleanly with race detector, lint passes
- [x] Network partition helpers in test/e2e/harness/network.go with PartitionService/HealPartition/HealAllPartitions
