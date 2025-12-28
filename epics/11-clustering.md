# Epic 11: High Availability Clustering

## Overview

Implement high availability clustering for `kscore-server` using etcd for distributed coordination. Transform Keystone Core from a single-server architecture to a distributed, fault-tolerant cluster that can handle server failures, automatically redistribute work, and provide zero-downtime operations.

**Goal**: Enable production-grade high availability with automatic failover, intelligent work distribution across cluster members, and no single point of failure for critical Keystone Core operations.

## Success Criteria

- [ ] etcd integration (embedded and external modes)
- [ ] Automatic cluster formation and member discovery
- [ ] Leader election for singleton tasks
- [ ] Intelligent work distribution (agent connections, jobs, events)
- [ ] Automatic failover with <5 second detection
- [ ] Zero-downtime rolling updates
- [ ] Split-brain prevention with quorum
- [ ] Cluster health monitoring and metrics
- [ ] Backup and restore for cluster state
- [ ] Complete cluster operations CLI
- [ ] Documentation updated (Epic 10)
- [ ] Chaos testing passed (failure scenarios)

## Problem Statement

**Current State:**
- Single `kscore-server` instance handles all operations
- Agent connections, job execution, state management all on one server
- Server failure = complete system outage
- No horizontal scalability for high load
- Manual failover required
- Downtime during upgrades

**Target State:**
- 3+ `kscore-server` instances in active-active cluster
- Distributed agent connections across cluster members
- Automatic failover in seconds, not minutes
- Horizontal scaling for capacity
- Zero-downtime rolling updates
- Quorum-based consistency guarantees

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Keystone Core Cluster                       │
│                                                             │
│  ┌────────────┐      ┌────────────┐      ┌────────────┐   │
│  │  Server 1  │      │  Server 2  │      │  Server 3  │   │
│  │  (Leader)  │      │  (Follower)│      │  (Follower)│   │
│  │            │      │            │      │            │   │
│  │ - API      │      │ - API      │      │ - API      │   │
│  │ - Agents   │      │ - Agents   │      │ - Agents   │   │
│  │ - Jobs     │      │ - Jobs     │      │ - Jobs     │   │
│  │ - Events   │      │ - Events   │      │ - Events   │   │
│  └─────┬──────┘      └─────┬──────┘      └─────┬──────┘   │
│        │                   │                   │           │
│        └───────────────────┴───────────────────┘           │
│                            │                                │
│                    ┌───────┴────────┐                      │
│                    │  etcd Cluster  │                      │
│                    │  (3+ members)  │                      │
│                    │                │                      │
│                    │ - Membership   │                      │
│                    │ - Leader Elect │                      │
│                    │ - Config       │                      │
│                    │ - Coordination │                      │
│                    └────────────────┘                      │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
    ┌─────────┐          ┌─────────┐         ┌─────────┐
    │ Agents  │          │ Agents  │         │ Agents  │
    │ Shard 1 │          │ Shard 2 │         │ Shard 3 │
    └─────────┘          └─────────┘         └─────────┘
```

## User Stories

### US11.1: Cluster Formation
**As an** operator
**I want to** form a multi-server cluster
**So that** I have high availability and fault tolerance

**Acceptance Criteria**:
- Deploy multiple `kscore-server` instances
- Automatic discovery and cluster formation
- etcd cluster integrated (embedded or external)
- Cluster membership viewable via CLI
- Health status of all members visible
- Join/leave operations supported

### US11.2: Automatic Failover
**As an** operator
**I want to** automatic failover on server failure
**So that** the system remains available during failures

**Acceptance Criteria**:
- Server failure detected within 5 seconds
- Agent connections redistributed automatically
- In-flight jobs completed on remaining servers
- Leader re-election completes in <3 seconds
- No user-visible disruption for most operations
- Alerts sent on failover events

### US11.3: Work Distribution
**As an** operator
**I want to** work distributed across cluster members
**So that** I can scale horizontally and balance load

**Acceptance Criteria**:
- Agent connections sharded across servers
- Job execution distributed
- Event processing distributed
- Policy evaluation distributed
- Configurable sharding strategies
- Load balancing metrics visible

### US11.4: Zero-Downtime Updates
**As an** operator
**I want to** upgrade servers without downtime
**So that** I can apply updates without service interruption

**Acceptance Criteria**:
- Rolling update procedure documented
- One server updated at a time
- Agent connections drained before shutdown
- Jobs completed before shutdown
- No connection drops during update
- Automated update orchestration

### US11.5: Split-Brain Prevention
**As an** operator
**I want to** prevent split-brain scenarios
**So that** data consistency is maintained

**Acceptance Criteria**:
- Quorum required for operations (N/2 + 1)
- Network partitions detected
- Minority partition stops serving requests
- Automatic healing when partition resolves
- Metrics for split-brain detection

### US11.6: Cluster Operations
**As an** operator
**I want to** manage the cluster via CLI
**So that** I can perform operational tasks easily

**Acceptance Criteria**:
- `kscorectl cluster status` - view cluster health
- `kscorectl cluster members` - list all members
- `kscorectl cluster add` - add new member
- `kscorectl cluster remove` - remove member
- `kscorectl cluster leader` - show current leader
- `kscorectl cluster backup` - backup cluster state
- `kscorectl cluster restore` - restore from backup

### US11.7: Observability
**As an** operator
**I want to** cluster metrics and health dashboards
**So that** I can monitor cluster health

**Acceptance Criteria**:
- Cluster member count metric
- Leader election count metric
- Failover count metric
- Work distribution metrics
- etcd health metrics
- Grafana dashboard for cluster health

## Technical Tasks

### Phase 1: etcd Integration & Cluster Formation (Week 1-2)

**T1.1: etcd Client Integration**
- Add etcd v3 client library
- Create etcd client wrapper (pkg/cluster/etcd.go)
- Support embedded etcd mode (single-server development)
- Support external etcd cluster (production)
- Connection retry logic with exponential backoff
- Health checking for etcd

**T1.2: Cluster Membership**
- Member registration on startup (pkg/cluster/membership.go)
- Heartbeat mechanism (5-second intervals)
- Member discovery (read from etcd)
- Member health monitoring
- Automatic member removal on timeout (30 seconds)
- Member metadata (ID, address, version, capabilities)

**T1.3: Cluster Configuration**
- Server configuration for cluster mode
- etcd endpoints configuration
- Cluster name and member ID
- Quorum size configuration
- Election timeout configuration
- Heartbeat interval configuration

**T1.4: Cluster State Storage**
- Store cluster state in etcd
- Member list persistence
- Leader information
- Cluster configuration
- Work distribution state
- Migration from single-server mode

### Phase 2: Leader Election & Work Distribution (Week 3-4)

**T2.1: Leader Election**
- Implement leader election using etcd (pkg/cluster/leader.go)
- Campaign for leadership on startup
- Leader session with TTL (15 seconds)
- Automatic resignation on server shutdown
- Observer pattern for leadership changes
- Leadership transfer support

**T2.2: Singleton Task Management**
- Identify singleton tasks (tasks only leader runs):
  - Reactor coordinator
  - Scheduled job coordinator
  - Cleanup tasks (log rotation, etc.)
  - Metric aggregation
  - Report generation
- Leader executes singleton tasks
- Follower idles singleton tasks
- Transfer state on leadership change

**T2.3: Agent Connection Sharding**
- Hash-based agent assignment (pkg/cluster/sharding.go)
- Consistent hashing for agent IDs
- Rebalance on member join/leave
- Agent connection migration (graceful handoff)
- Shard state tracking in etcd
- Connection metrics per shard

**T2.4: Job Distribution**
- Job queue sharding by target agent
- Job assignment to server owning agent connection
- Job redistribution on server failure
- Job completion tracking across cluster
- Prevent duplicate job execution

**T2.5: Event Processing Distribution**
- Event stream partitioning
- Each server processes subset of events
- Event processing guarantees (at-least-once)
- Reactor execution distribution
- Event replay on server failure

### Phase 3: Failover & Recovery (Week 5-6)

**T3.1: Failure Detection**
- Heartbeat monitoring (pkg/cluster/health.go)
- Member failure detection (<5 seconds)
- Network partition detection
- Slow member detection (performance degradation)
- Cascading failure prevention

**T3.2: Automatic Failover**
- Agent connection failover (pkg/cluster/failover.go)
- Agent reconnection to different server
- Job reassignment on failure
- Event processing continuation
- State transfer to takeover server
- Failover metrics and alerts

**T3.3: Graceful Shutdown**
- Drain agent connections on shutdown
- Complete in-flight jobs before shutdown
- Transfer leadership before shutdown
- Notify cluster of planned departure
- Configurable drain timeout (default 30s)

**T3.4: Recovery Procedures**
- Rejoin cluster after restart
- State synchronization from etcd
- Agent reconnection after recovery
- Job recovery from persistent queue
- Data consistency verification

**T3.5: Fencing**
- Prevent zombie servers from operating
- etcd lease-based fencing
- Automatic server isolation on partition
- Health check before serving requests
- Split-brain detection and mitigation

### Phase 4: Data Consistency & Replication (Week 7-8)

**T4.1: Distributed State**
- Use PostgreSQL for shared state (existing)
- etcd for coordination state only
- Optimistic locking for concurrent updates
- Transaction support for atomic operations
- Conflict resolution strategies

**T4.2: Agent State Synchronization**
- Agent metadata in shared database
- Agent heartbeat in etcd (ephemeral)
- Connection state distributed tracking
- Fact cache synchronization
- Last-seen timestamp updates

**T4.3: Job State Consistency**
- Job queue in shared database
- Job assignment in etcd (ephemeral)
- Job result persistence
- Prevent duplicate job execution (idempotency keys)
- Job timeout and retry logic

**T4.4: Event Stream Consistency**
- Event storage in shared database (existing)
- Event processing offsets in etcd
- At-least-once delivery guarantee
- Event deduplication (event IDs)
- Event replay from offset

**T4.5: Configuration Distribution**
- Centralized configuration in etcd
- Watch for configuration changes
- Hot reload on configuration update
- Configuration versioning
- Rollback capability

### Phase 5: Cluster Operations & Management (Week 9-10)

**T5.1: Cluster CLI (kscorectl cluster)**
- `status` - cluster health and member status
- `members` - list all members with details
- `leader` - show current leader
- `add <address>` - add new member
- `remove <member-id>` - remove member
- `transfer-leader <member-id>` - transfer leadership
- `rebalance` - trigger manual rebalancing

**T5.2: Cluster API**
- REST API for cluster operations (pkg/api/cluster.go)
- GET /api/v1/cluster/status
- GET /api/v1/cluster/members
- GET /api/v1/cluster/leader
- POST /api/v1/cluster/members (add member)
- DELETE /api/v1/cluster/members/:id (remove member)
- POST /api/v1/cluster/leader/transfer

**T5.3: Backup & Restore**
- Backup cluster state (etcd snapshot)
- Backup shared database (PostgreSQL dump)
- Automated backup schedule
- Point-in-time recovery
- Restore procedures (disaster recovery)
- Backup verification

**T5.4: Cluster Scaling**
- Add server to cluster (join operation)
- Remove server from cluster (leave operation)
- Automatic rebalancing on scale up/down
- Minimum cluster size enforcement (3 members)
- Maximum cluster size recommendations (7 members)

**T5.5: Upgrade Orchestration**
- Rolling update automation
- Version compatibility checking
- Backward compatibility guarantees
- Upgrade status tracking
- Rollback on upgrade failure

### Phase 6: Observability & Monitoring (Week 11-12)

**T6.1: Cluster Metrics**
- Cluster member count (gauge)
- Leader election count (counter)
- Leadership duration (histogram)
- Failover count (counter)
- Failover duration (histogram)
- Member health status (gauge per member)
- etcd operation latency (histogram)
- etcd operation errors (counter)

**T6.2: Work Distribution Metrics**
- Agents per server (gauge)
- Jobs per server (gauge)
- Events processed per server (counter)
- Rebalance count (counter)
- Rebalance duration (histogram)
- Shard distribution skew (gauge)

**T6.3: Cluster Health Checks**
- Individual member health
- Quorum health (have majority?)
- Leader election health
- etcd connectivity health
- Database connectivity health (all members)
- Network partition detection

**T6.4: Grafana Dashboard**
- Cluster topology visualization
- Member status matrix
- Leader election history
- Failover events timeline
- Work distribution charts
- etcd health metrics
- Database connection pool metrics

**T6.5: Alerting Rules**
- Member down alert
- Leader election failure alert
- Quorum loss alert (CRITICAL)
- Split-brain detected alert (CRITICAL)
- High failover rate alert
- Rebalancing failures alert
- etcd unhealthy alert

### Phase 7: Testing & Validation (Week 13-14)

**T7.1: Unit Tests**
- Leader election tests
- Membership management tests
- Sharding algorithm tests
- Failover logic tests
- Health check tests
- Backup/restore tests

**T7.2: Integration Tests**
- Multi-server cluster formation
- Leader election under load
- Agent connection failover
- Job distribution and failover
- Configuration propagation
- etcd failure scenarios

**T7.3: Chaos Testing**
- Random server kill (chaos monkey)
- Network partition simulation
- Clock skew simulation
- Slow member simulation
- Cascading failure simulation
- Split-brain scenarios

**T7.4: Performance Testing**
- Cluster formation time (3-5 members)
- Leader election time (<3 seconds)
- Failover detection time (<5 seconds)
- Agent reconnection time (<10 seconds)
- Job redistribution time
- Throughput with 3 vs 5 vs 7 members

**T7.5: Disaster Recovery Testing**
- Complete cluster failure
- Restore from backup
- Quorum loss recovery
- Data corruption recovery
- Time to full recovery (<5 minutes)

### Phase 8: Documentation Update (Week 15-16)

**T8.1: Update Epic 10 Documentation**
- Concepts: Add clustering architecture
- Concepts: Explain leader election
- Concepts: Document work distribution
- Guides: Cluster deployment guide
- Guides: Cluster operations guide
- Reference: Cluster CLI commands
- Reference: Cluster API endpoints
- Reference: Cluster configuration options

**T8.2: Operations Documentation**
- Deployment: HA cluster setup (3/5/7 nodes)
- Deployment: etcd deployment (embedded vs external)
- Deployment: Load balancer configuration
- Maintenance: Rolling update procedures
- Maintenance: Backup and restore
- Troubleshooting: Cluster issues
- Troubleshooting: Split-brain recovery
- Troubleshooting: Quorum loss

**T8.3: Runbooks**
- Add new cluster member
- Remove cluster member
- Handle server failure
- Recover from split-brain
- Restore from backup
- Upgrade cluster version
- Scale cluster up/down

**T8.4: Architecture Diagrams**
- Cluster topology diagram
- Leader election flow
- Failover sequence diagram
- Agent connection sharding
- Work distribution architecture
- Data flow in clustered mode

## Cluster Sizing Recommendations

### Development
- **1 server**: Single server with embedded etcd (no HA)
- **Use case**: Local development, testing

### Small Production
- **3 servers**: Minimum HA setup
- **etcd**: Embedded etcd on each server (3-node etcd cluster)
- **Capacity**: Up to 5,000 agents, 100 jobs/sec
- **Failover**: Tolerates 1 server failure

### Medium Production
- **5 servers**: Recommended production setup
- **etcd**: External 3-node etcd cluster (dedicated)
- **Capacity**: Up to 20,000 agents, 500 jobs/sec
- **Failover**: Tolerates 2 server failures

### Large Production
- **7 servers**: High-scale deployment
- **etcd**: External 5-node etcd cluster (dedicated)
- **Capacity**: Up to 50,000 agents, 1,000 jobs/sec
- **Failover**: Tolerates 3 server failures

**Why odd numbers?** Quorum requires N/2 + 1, so 3 servers need 2 for quorum (tolerates 1 failure), 5 servers need 3 for quorum (tolerates 2 failures).

## Work Distribution Strategies

### Agent Connection Sharding
**Strategy**: Consistent hashing on agent ID
- Each agent assigned to server based on hash(agent_id)
- Rebalance minimally on member join/leave (consistent hashing)
- Agent reconnects to assigned server
- Connection state tracked in etcd

### Job Queue Distribution
**Strategy**: Route jobs to server owning target agent
- Job for agent-123 goes to server owning agent-123's connection
- Direct execution, no cross-server coordination
- Fallback: Round-robin if agent offline

### Event Processing
**Strategy**: Topic-based partitioning
- Event types hashed to processing servers
- Each server processes subset of event types
- Reactor execution on assigned server
- Rebalance on member changes

### Policy Evaluation
**Strategy**: Stateless, any server can evaluate
- Policy evaluation is stateless
- Route to least-loaded server
- Load balancing by API gateway or DNS

## Split-Brain Prevention

### Quorum-Based Operations
- Require N/2 + 1 members for write operations
- Read operations allowed from any member (with staleness)
- Minority partition stops accepting writes
- Network partition detected via etcd

### Fencing Mechanisms
1. **etcd leases**: Members hold lease, writes only with valid lease
2. **Epoch numbers**: Increment on leader election, reject old epoch
3. **Database fencing**: Use PostgreSQL advisory locks

### Recovery from Split-Brain
1. Detect partition via etcd quorum loss
2. Minority partition enters read-only mode
3. Wait for partition heal
4. Resync state from majority partition
5. Resume normal operation

## Dependencies

- **etcd v3**: Distributed coordination
- **PostgreSQL**: Shared state storage (already present)
- **NATS**: Agent messaging (already present)
- **Completed Epics**: Epic 1, 7 (metrics/logging for observability)
- **Epic 10**: Documentation (will be updated)

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Split-brain data corruption | Critical | Low | Quorum enforcement, fencing, testing |
| Network partition handling | High | Medium | Partition detection, minority shutdown |
| etcd cluster failure | Critical | Low | Regular backups, external etcd cluster |
| Cascading failures | High | Medium | Circuit breakers, rate limiting |
| Agent connection storms | Medium | Medium | Connection rate limiting, gradual reconnect |
| Rebalancing disruption | Medium | High | Gradual rebalancing, connection draining |
| Complex operations | Medium | High | Comprehensive docs, testing, runbooks |

## Metrics & Monitoring

### Cluster Health Metrics
```
kscore_cluster_members{status="healthy|unhealthy"} - Member count by status
kscore_cluster_leader_elections_total - Total leader elections
kscore_cluster_leader_duration_seconds - Leadership duration
kscore_cluster_failovers_total - Total failover events
kscore_cluster_failover_duration_seconds - Failover detection to recovery time
kscore_cluster_quorum_status{status="ok|lost"} - Quorum status
```

### Work Distribution Metrics
```
kscore_cluster_agents_per_member{member_id} - Agent count per member
kscore_cluster_jobs_per_member{member_id} - Job count per member
kscore_cluster_events_per_member{member_id} - Events processed per member
kscore_cluster_rebalances_total - Total rebalancing events
kscore_cluster_rebalance_duration_seconds - Rebalancing duration
```

### etcd Metrics
```
kscore_etcd_request_duration_seconds{operation} - etcd request latency
kscore_etcd_request_errors_total{operation} - etcd request errors
kscore_etcd_lease_renewals_total - etcd lease renewals
kscore_etcd_watch_events_total - etcd watch events
```

## Testing Strategy

### Unit Tests
- Leader election logic
- Consistent hashing algorithm
- Member health checking
- Failover decision making
- Quorum calculations

### Integration Tests
- 3-node cluster formation
- Leader election and re-election
- Agent failover between servers
- Job redistribution on failure
- Configuration synchronization

### Chaos Tests
- Kill random server (1/3, 2/3)
- Network partition (minority/majority)
- Slow network (latency injection)
- Clock skew between servers
- Resource exhaustion (CPU, memory)

### Load Tests
- 10,000 agents across 3 servers
- 100 jobs/sec distributed execution
- 1,000 events/sec processing
- Failover under load
- Rebalancing under load

## Documentation Requirements

### User Documentation (Epic 10 Update)
- **Concepts**: Clustering architecture and behavior
- **Getting Started**: Deploy HA cluster
- **Guides**: Cluster operations guide
- **Reference**: Cluster CLI and API
- **Operations**: HA deployment, disaster recovery
- **Troubleshooting**: Cluster-specific issues

### Operator Documentation
- **Runbooks**: Step-by-step operational procedures
- **Architecture**: Detailed cluster internals
- **Decision Matrix**: When to use 3/5/7 nodes
- **Capacity Planning**: Sizing recommendations
- **Disaster Recovery**: Complete recovery procedures

## Definition of Done

- [ ] All 8 phases completed
- [ ] etcd integration working (embedded and external)
- [ ] 3-node cluster operational
- [ ] Leader election tested under failure
- [ ] Agent failover working (<10 sec)
- [ ] Job distribution working correctly
- [ ] Quorum enforcement verified
- [ ] Split-brain prevention tested
- [ ] Backup/restore working
- [ ] Cluster CLI completed
- [ ] Metrics and dashboards created
- [ ] Chaos tests passing
- [ ] Performance benchmarks met
- [ ] Documentation updated (Epic 10)
- [ ] Runbooks completed
- [ ] Production deployment guide ready

## Timeline

Total: **16 weeks** (4 months)

- **Weeks 1-2**: etcd integration, cluster formation
- **Weeks 3-4**: Leader election, work distribution
- **Weeks 5-6**: Failover and recovery
- **Weeks 7-8**: Data consistency and replication
- **Weeks 9-10**: Cluster operations and management
- **Weeks 11-12**: Observability and monitoring
- **Weeks 13-14**: Testing and validation
- **Weeks 15-16**: Documentation update (Epic 10)

## Success Metrics

- **Availability**: 99.95% uptime with 3-node cluster
- **Failover Time**: <5 seconds from failure to recovery
- **Leader Election**: <3 seconds
- **Agent Reconnection**: <10 seconds after failover
- **Zero Data Loss**: During normal failover scenarios
- **Scalability**: Linear scale up to 7 nodes
- **Upgrade Downtime**: Zero downtime for rolling updates

## Future Enhancements (Post-Epic)

- **Multi-region clustering**: Cross-datacenter deployments
- **Dynamic sharding**: Automatic shard splitting under load
- **Read replicas**: Read-only cluster members for scale
- **Automatic scaling**: Auto-add/remove members based on load
- **Advanced placement**: Datacenter/rack-aware placement
- **Federation**: Multiple independent clusters
