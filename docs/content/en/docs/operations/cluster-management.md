---
title: "Cluster Management"
weight: 2
description: >
  Operating and managing Keystone Core high-availability clusters
---

## Overview

This guide covers day-to-day operations for Keystone Core HA clusters, including cluster health monitoring, member management, failover procedures, and recovery operations.

**Prerequisites:**
- Keystone Core cluster deployed with etcd
- Understanding of [Clustering concepts](/docs/concepts/control-plane/#high-availability)
- Access to `kscore-cluster` CLI

## Cluster Architecture

A production Keystone Core cluster consists of:

```mermaid
flowchart TB
    subgraph LB["Load Balancer"]
    end

    subgraph K1["kscore-1 (leader)"]
        E1["etcd"]
    end

    subgraph K2["kscore-2 (follower)"]
        E2["etcd"]
    end

    subgraph K3["kscore-3 (follower)"]
        E3["etcd"]
    end

    LB --> K1
    LB --> K2
    LB --> K3

    E1 <--> E2
    E2 <--> E3
    E1 <--> E3
```

**Quorum Requirements:**
- 3 nodes: Tolerates 1 failure
- 5 nodes: Tolerates 2 failures
- 7 nodes: Tolerates 3 failures

## Cluster Status

### Check Cluster Health

```bash
# Overall cluster status
kscorectl cluster status

# Output:
# Cluster Status: healthy
# Quorum: 3/3 members healthy
# Leader: kscore-1
#
# Members:
#   kscore-1 (leader)  - healthy - 192.168.1.10:2380
#   kscore-2           - healthy - 192.168.1.11:2380
#   kscore-3           - healthy - 192.168.1.12:2380
```

### List Cluster Members

```bash
# List all members
kscorectl cluster members

# Detailed member info
kscorectl cluster members --output yaml
```

### Check Current Leader

```bash
kscorectl cluster leader

# Output:
# Current Leader: kscore-1
# Leader Since: 2026-01-10T08:30:15Z
# Term: 42
```

### Health Check Details

```bash
# Detailed health check
kscorectl cluster health

# Output:
# Cluster Health Check
# ====================
# etcd: healthy
# NATS: healthy
# API: healthy
# State DB: healthy
#
# Member Health:
#   kscore-1: all checks passed
#   kscore-2: all checks passed
#   kscore-3: all checks passed
```

## Member Management

### Add a New Member

When scaling up the cluster:

```bash
# 1. Prepare the new node with kscore-server installed

# 2. Add member to cluster (run on existing member)
kscorectl cluster add-member \
  --name kscore-4 \
  --peer-urls https://192.168.1.13:2380

# 3. Start kscore-server on new node with join config
kscore-server --config /etc/kscore/server.yaml --join
```

### Remove a Member

```bash
# Remove unhealthy or decommissioned member
kscorectl cluster remove <member-id>

# Force remove (use with caution)
kscorectl cluster remove <member-id> --force
```

### Trigger Rebalance

After adding/removing members, rebalance agents:

```bash
# Redistribute agents across members
kscorectl cluster rebalance

# Output:
# Rebalancing agents across 4 members...
# Moved 125 agents from kscore-1 to kscore-4
# Moved 118 agents from kscore-2 to kscore-4
# Moved 122 agents from kscore-3 to kscore-4
# Rebalance complete: 500 agents distributed evenly
```

## Failover Procedures

### Automatic Failover

Keystone Core automatically handles leader failover:

1. **Detection**: Followers detect leader failure via heartbeat timeout
2. **Election**: New leader elected via Raft consensus
3. **Promotion**: New leader assumes control
4. **Notification**: Agents notified of new leader

Failover typically completes in 5-15 seconds.

### Manual Failover

For planned maintenance:

```bash
# Step down current leader (triggers election)
kscorectl cluster stepdown

# Verify new leader
kscorectl cluster leader
```

### Monitoring Failovers

```bash
# View leader history
kscorectl cluster leader --history

# Output:
# Leader History:
#   2026-01-10T08:30:15Z  kscore-1  (current)
#   2026-01-09T22:15:30Z  kscore-2  (stepped down)
#   2026-01-09T14:00:00Z  kscore-1  (failover from kscore-3)
```

## Recovery Procedures

### Single Member Failure

If one member fails:

1. **Assess**: Check if member can recover
   ```bash
   kscorectl cluster status
   ```

2. **If recoverable**: Restart the member
   ```bash
   systemctl restart kscore-server
   ```

3. **If unrecoverable**: Remove and replace
   ```bash
   kscorectl cluster remove <member-id>
   kscorectl cluster add-member --name new-member --peer-urls <urls>
   ```

### Quorum Loss Recovery

If quorum is lost (majority of members down):

**WARNING**: This procedure may result in data loss.

```bash
# 1. Stop all remaining members
systemctl stop kscore-server

# 2. On the member with most recent data, force new cluster
kscore-server --force-new-cluster

# 3. Verify single-node cluster is healthy
kscorectl cluster status

# 4. Add new members to restore HA
kscorectl cluster add-member --name kscore-2 --peer-urls ...
```

### Data Recovery from Backup

```bash
# 1. Stop all cluster members
# 2. Restore etcd data on each member
etcdctl snapshot restore /backup/etcd-snapshot.db \
  --name kscore-1 \
  --initial-cluster kscore-1=https://192.168.1.10:2380,...

# 3. Start cluster
systemctl start kscore-server
```

## Performance Tuning

### etcd Tuning

For large clusters (>1000 agents):

```yaml
# server.yaml
cluster:
  etcd:
    # Increase snapshot count for fewer snapshots
    snapshot_count: 10000

    # Increase election timeout for slower networks
    election_timeout: 5000

    # Increase heartbeat interval
    heartbeat_interval: 500

    # Quota for etcd storage (default: 2GB)
    quota_backend_bytes: 8589934592
```

### Work Distribution Tuning

```yaml
cluster:
  # How often to rebalance (default: 5m)
  rebalance_interval: 10m

  # Minimum imbalance to trigger rebalance (percentage)
  rebalance_threshold: 15

  # Maximum agents per member
  max_agents_per_member: 2000
```

## Monitoring

### Key Metrics

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `kscore_cluster_members_healthy` | < expected | Healthy member count |
| `kscore_cluster_has_quorum` | == 0 | Quorum lost |
| `kscore_cluster_leader_changes_total` | Rate > 5/hour | Frequent elections |
| `kscore_cluster_etcd_operation_duration_seconds` | P95 > 100ms | Slow etcd ops |

### Grafana Dashboard

Import the Cluster Health dashboard from `deploy/grafana/dashboards/cluster-health.json`:

- Cluster overview: quorum, members, leader status
- Member health timeline
- Leader election frequency
- etcd operation latency

### Alert Rules

Critical alerts to configure:

```yaml
# Prometheus alert rules
- alert: KscoreClusterNoQuorum
  expr: kscore_cluster_has_quorum == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Keystone Core cluster lost quorum"

- alert: KscoreClusterMemberUnhealthy
  expr: kscore_cluster_member_status != 1
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "Cluster member {{ $labels.member }} is unhealthy"
```

## Maintenance Operations

### Rolling Restart

For upgrades or configuration changes:

```bash
# 1. Restart followers first, one at a time
for member in kscore-2 kscore-3; do
  ssh $member "systemctl restart kscore-server"
  sleep 30
  kscorectl cluster health
done

# 2. Failover from leader
kscorectl cluster stepdown

# 3. Restart old leader
ssh kscore-1 "systemctl restart kscore-server"

# 4. Verify cluster health
kscorectl cluster status
```

### etcd Compaction

Periodically compact etcd history:

```bash
# Get current revision
ETCDCTL_API=3 etcdctl endpoint status

# Compact history (keep last 1000 revisions)
ETCDCTL_API=3 etcdctl compact <revision-1000>

# Defragment to reclaim space
ETCDCTL_API=3 etcdctl defrag
```

### Backup Cluster State

```bash
# Backup etcd
etcdctl snapshot save /backup/etcd-$(date +%Y%m%d).db

# Backup Keystone Core config
tar -czf /backup/kscore-config-$(date +%Y%m%d).tar.gz /etc/kscore/
```

## Troubleshooting

### Member Won't Join

1. Check network connectivity:
   ```bash
   nc -zv <peer-ip> 2380
   ```

2. Verify TLS certificates match cluster CA

3. Check etcd logs:
   ```bash
   journalctl -u kscore-server -f | grep etcd
   ```

### Frequent Leader Elections

1. Check network latency between members
2. Increase election timeout
3. Check for resource contention (CPU, disk I/O)

### Split Brain Prevention

Keystone Core uses Raft consensus to prevent split-brain:

- Requires majority for writes
- Minority partition becomes read-only
- Automatic healing when partition resolves

## Failover Drills

Regular failover drills validate your cluster's ability to recover from failures. Running drills builds confidence in your HA configuration and helps identify issues before they cause outages.

### Why Run Failover Drills

- **Validate recovery procedures** before you need them in an emergency
- **Identify configuration issues** that may prevent successful failover
- **Train operations staff** on recovery procedures
- **Verify monitoring and alerting** detects failures correctly
- **Measure recovery time** to set realistic RTO expectations

### HA Configuration Recommendations

Before running drills, verify your HA configuration meets best practices:

```bash
# Check HA recommendations
kscorectl cluster ha-check

# Output:
# HA Configuration Check
# ======================
# ✓ Cluster has odd number of members (3) - optimal for quorum
# ✓ Election timeout (15s) is >= 3x heartbeat interval (5s)
# ✓ TLS enabled for cluster communication
# ✓ Peer certificate authentication enabled
#
# Recommendations:
#   (none)
```

**Critical HA recommendations:**
- Use **odd cluster sizes** (3, 5, 7) for clear quorum decisions
- Minimum **3 nodes** for any fault tolerance
- **Election timeout ≥ 3× heartbeat interval** to prevent election storms
- **TLS and peer certificate authentication** for multi-node clusters

### Running a Controlled Failover Drill

**Schedule the drill** during a maintenance window with the team prepared.

#### Step 1: Verify Pre-Drill Health

```bash
# Confirm cluster is fully healthy
kscorectl cluster status
kscorectl cluster health

# Record current state
kscorectl cluster leader
kscorectl cluster members --output yaml > /tmp/pre-drill-members.yaml
```

#### Step 2: Simulate Leader Failure

**Option A: Graceful stepdown (safest)**
```bash
# Step down current leader, triggering election
kscorectl cluster stepdown

# Wait for new leader election
sleep 10
kscorectl cluster leader
```

**Option B: Process kill (tests detection)**
```bash
# On leader node
kill -9 $(pgrep kscore-server)

# On another node, watch for failover
watch -n1 'kscorectl cluster status'
```

**Option C: Network partition (tests split-brain prevention)**
```bash
# On leader node, block cluster traffic
iptables -A INPUT -p tcp --dport 2380 -j DROP
iptables -A OUTPUT -p tcp --dport 2380 -j DROP

# Monitor from other nodes
kscorectl cluster status

# After drill, restore network
iptables -D INPUT -p tcp --dport 2380 -j DROP
iptables -D OUTPUT -p tcp --dport 2380 -j DROP
```

#### Step 3: Monitor During Drill

During failover, observe:

```bash
# Watch cluster status (updates every second)
watch -n1 'kscorectl cluster status'

# Monitor metrics
curl -s localhost:9090/metrics | grep kscore_cluster

# Check logs on all nodes
journalctl -u kscore-server -f
```

**Key metrics to watch:**
- `kscore_cluster_leader_changes_total` - should increment by 1
- `kscore_cluster_has_quorum` - should remain 1
- `kscore_cluster_leader_election_duration_seconds` - measure recovery time

#### Step 4: Verify Recovery

```bash
# Confirm new leader elected
kscorectl cluster leader

# Verify all operations work
kscorectl cluster status
kscorectl exec run --target '*' 'hostname'

# Restart failed member
systemctl start kscore-server

# Confirm member rejoins
kscorectl cluster members
```

#### Step 5: Document Results

Record the drill results:
- **Failover time**: How long until new leader was elected?
- **Service impact**: Did any agent operations fail?
- **Alerting**: Did monitoring detect the failure?
- **Recovery**: Did the failed member rejoin cleanly?

### Success Criteria

A successful failover drill should show:

| Metric | Target | Critical |
|--------|--------|----------|
| Leader election time | < 15 seconds | < 30 seconds |
| Quorum maintained | Always | Always |
| Agent reconnection | < 30 seconds | < 60 seconds |
| No data loss | Yes | Yes |
| Alerts fired | Yes | Yes |

### Recommended Drill Schedule

| Environment | Frequency | Type |
|-------------|-----------|------|
| Development | Weekly | Graceful stepdown |
| Staging | Bi-weekly | Process kill |
| Production | Monthly | Graceful stepdown |
| Production | Quarterly | Network partition |

### Common Issues During Drills

**Election takes too long:**
- Check network latency between nodes
- Verify election timeout is at least 3× heartbeat interval
- Check for resource contention

**Quorum lost during drill:**
- Verify you have enough members (need majority)
- Check if multiple members failed simultaneously
- Review network configuration

**Member won't rejoin:**
- Check for data directory corruption
- Verify TLS certificates are valid
- Review etcd logs for errors

## See Also

- [Clustering Concepts](/docs/concepts/control-plane/#high-availability)
- [Maintenance Guide]({{< ref "maintenance" >}})
- [Monitoring Guide]({{< ref "monitoring" >}})
- [Security Guide]({{< ref "security" >}})
