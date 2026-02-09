# Runbook: Disaster Recovery

## Overview

This runbook covers disaster recovery procedures for complete or partial Keystone Core cluster loss.

## Prerequisites

- [ ] Access to backup storage (S3, GCS, Azure, local)
- [ ] Backup decryption keys/identities
- [ ] New infrastructure provisioned (or recovery site)
- [ ] Network connectivity to backup location
- [ ] DNS update access (if needed)

## Trigger Conditions

- Complete cluster loss (all control plane nodes down)
- Datacenter outage
- Unrecoverable data corruption
- Complete infrastructure failure

## Severity Assessment

| Scenario | RTO Target | RPO Target | Priority |
|----------|------------|------------|----------|
| Single node failure | 15 min | 0 | P2 |
| Multi-node failure | 30 min | 0 | P1 |
| Complete cluster loss | 4 hours | Last backup | P0 |
| Datacenter outage | 4 hours | Last backup | P0 |

## Procedure

### Phase 1: Assessment (15 minutes)

#### Step 1.1: Confirm Disaster

```bash
# Attempt to reach all control plane nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ping -c 3 $node || echo "$node unreachable"
  curl -sk https://$node:8080/health/live || echo "$node API unreachable"
done

# Check if any node is responding
kscorectl cluster health || echo "Cluster unreachable"
```

#### Step 1.2: Identify Available Backups

```bash
# List backups from primary location
kscore-cluster-backup list --dest s3://keystone-backups/

# List backups from DR location
kscore-cluster-backup list --dest s3://keystone-backups-dr/

# Identify most recent valid backup
# Note: timestamp, size, components
```

#### Step 1.3: Document Current State

```
Disaster Assessment:
- Date/Time: ____________
- Affected nodes: ____________
- Last known good state: ____________
- Latest backup: ____________
- Recovery location: ____________
```

### Phase 2: Infrastructure Preparation (30-60 minutes)

#### Step 2.1: Provision Recovery Infrastructure

```bash
# Option A: Use pre-provisioned DR site
# Verify DR infrastructure is ready
ssh dr-server-1 "hostname && systemctl status"

# Option B: Provision new infrastructure
# Use Terraform/CloudFormation/etc.
cd infrastructure/
terraform apply -var="environment=recovery"
```

#### Step 2.2: Verify Network Connectivity

```bash
# Test connectivity between recovery nodes
for node in dr-server-1 dr-server-2 dr-server-3; do
  ssh $node "ping -c 1 dr-server-1 && ping -c 1 dr-server-2 && ping -c 1 dr-server-3"
done

# Test connectivity to backup storage
aws s3 ls s3://keystone-backups-dr/ --region us-west-2
```

#### Step 2.3: Prepare Recovery Environment

```bash
# Copy bootstrap binary to recovery nodes
for node in dr-server-1 dr-server-2 dr-server-3; do
  scp kscore-bootstrap $node:/usr/local/bin/
done

# Copy decryption identity
scp /secure/backup-identity.txt dr-server-1:/tmp/
```

### Phase 3: Restore from Backup (60-120 minutes)

#### Step 3.1: Download and Verify Backup

```bash
# SSH to first recovery node
ssh dr-server-1

# Download latest backup
aws s3 cp s3://keystone-backups-dr/backup-2024-01-15T02-00-00.tar.gz /tmp/

# Verify backup integrity
kscore-cluster-backup verify /tmp/backup-2024-01-15T02-00-00.tar.gz

# Expected: "Backup verification passed"
```

#### Step 3.2: Restore First Node

```bash
# Restore from backup
kscore-bootstrap restore \
  --backup /tmp/backup-2024-01-15T02-00-00.tar.gz \
  --decrypt-identity /tmp/backup-identity.txt \
  --node-ip $(hostname -I | awk '{print $1}')

# Monitor restore progress
# This will restore: database, config, certificates, JetStream data
```

#### Step 3.3: Verify First Node

```bash
# Check server status
systemctl status kscore-server

# Check API health
curl -k https://localhost:8080/health/ready

# Check cluster status (single node)
kscorectl cluster health
```

#### Step 3.4: Join Additional Nodes

```bash
# Get join token from cluster config or bootstrap output
JOIN_TOKEN="$CLUSTER_JOIN_TOKEN"  # Set from config file

# On each additional node
ssh dr-server-2
kscore-bootstrap import \
  --join https://dr-server-1:8080 \
  --token $JOIN_TOKEN

ssh dr-server-3
kscore-bootstrap import \
  --join https://dr-server-1:8080 \
  --token $JOIN_TOKEN
```

#### Step 3.5: Verify Cluster Formation

```bash
# Verify all nodes are healthy
kscorectl cluster members

# Verify quorum
kscorectl cluster health

# Verify leader election
kscorectl cluster leader
```

### Phase 4: Agent Recovery (30-60 minutes)

#### Step 4.1: Update DNS (if needed)

```bash
# Update DNS to point to new control plane
# This depends on your DNS provider

# Example: AWS Route53
aws route53 change-resource-record-sets \
  --hosted-zone-id ZXXXXX \
  --change-batch file://dns-update.json
```

#### Step 4.2: Wait for Agent Reconnection

```bash
# Agents should automatically reconnect if DNS is updated
# Monitor agent reconnections
watch -n 10 'kscorectl agent list --status | wc -l'

# Check for agents that haven't reconnected
kscorectl agent list --status | grep offline
```

#### Step 4.3: Manual Agent Update (if needed)

```bash
# If agents can't reach new control plane via DNS,
# update agent configurations
# On each agent node:
sed -i 's/old-server/new-server/g' /etc/keystone-core/agent.yaml
systemctl restart kscore-agent
```

### Phase 5: Validation (30 minutes)

#### Step 5.1: Verify Cluster Health

```bash
# Full health check
kscorectl cluster health --verbose

# Verify all agents connected
kscorectl agent list --status

# Verify state data by checking a known state file
kscorectl state check /path/to/known-state.yaml
```

#### Step 5.2: Run Integration Tests

```bash
# Run smoke tests
kscore-test smoke

# Run integration tests
kscore-test integration --suite recovery
```

#### Step 5.3: Verify Critical Functionality

```bash
# Test remote execution (on a single agent)
kscorectl exec run "role:webserver" -- hostname

# Test state application
kscorectl state check /etc/keystone-core/states/test.yaml

# Test event system
kscorectl events list --limit 10
```

## Verification Checklist

- [ ] All control plane nodes healthy
- [ ] Cluster quorum established
- [ ] Leader elected
- [ ] Database restored and accessible
- [ ] NATS cluster formed
- [ ] All agents reconnected
- [ ] API responding correctly
- [ ] State data intact
- [ ] Remote execution working
- [ ] Events flowing

## Rollback

If recovery fails:

```bash
# Try older backup
kscore-bootstrap restore --backup /tmp/backup-older.tar.gz

# Or restore to previous recovery point
# Contact support for assistance
```

## Post-Procedure

### Immediate (within 1 hour)

1. [ ] Update status page
2. [ ] Notify stakeholders of recovery
3. [ ] Document recovery details:
   - Recovery start time
   - Recovery completion time
   - Data loss (if any)
   - Issues encountered

### Within 24 hours

1. [ ] Root cause analysis
2. [ ] Update monitoring for new infrastructure
3. [ ] Reconfigure backup jobs
4. [ ] Test backup/restore cycle

### Within 1 week

1. [ ] Post-mortem meeting
2. [ ] Update DR procedures based on learnings
3. [ ] Plan infrastructure improvements
4. [ ] Update runbook if needed

## Appendix: Recovery Time Estimates

| Component | Estimated Time |
|-----------|----------------|
| Infrastructure provisioning | 15-60 min |
| Backup download | 5-30 min |
| Database restore | 10-60 min |
| Certificate restore | 2 min |
| NATS data restore | 10-30 min |
| Cluster formation | 10 min |
| Agent reconnection | 5-30 min |
| Validation | 30 min |
| **Total** | **1.5-4 hours** |

## Split-Brain Recovery Playbook

Split-brain occurs when a network partition divides the cluster into multiple segments, each believing it's the authoritative cluster. This is one of the most dangerous distributed systems failure modes.

### Understanding Split-Brain

#### What Causes Split-Brain

```
Normal State:
┌─────────────────────────────────────────────────────┐
│                    Cluster                           │
│   [Node A] ←──→ [Node B] ←──→ [Node C]              │
│     (L)           (F)           (F)                  │
│   L=Leader, F=Follower                               │
└─────────────────────────────────────────────────────┘

Split-Brain State:
┌───────────────────────┐     ┌───────────────────────┐
│    Partition A        │  X  │    Partition B        │
│   [Node A] ←→ [Node B]│     │      [Node C]         │
│     (L)        (F)    │     │        (L?)           │
└───────────────────────┘     └───────────────────────┘
                  Network partition
```

**Common causes:**

- Network equipment failure (switch, router)
- Misconfigured firewalls
- Cloud provider network issues
- DNS failures
- Certificate expiration blocking communication

### Detection

#### Symptoms of Split-Brain

1. **Conflicting leaders**: Multiple nodes claiming leadership
2. **Data divergence**: Different state on different nodes
3. **Agent confusion**: Agents connected to different "clusters"
4. **Duplicate operations**: Commands executed multiple times

#### Detection Commands

```bash
# Check for multiple leaders
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "kscorectl cluster leader 2>/dev/null || echo 'unreachable'"
done

# Check etcd cluster state
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "etcdctl endpoint status --cluster 2>/dev/null" || echo "unreachable"
done

# Check agent distribution
kscorectl agent list --group-by control-plane-node

# Check for data divergence
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node agent count ==="
  ssh $node "kscorectl agent list --count 2>/dev/null" || echo "unreachable"
done
```

#### Alert Rules for Split-Brain

```yaml
groups:
  - name: split-brain-detection
    rules:
      - alert: ClusterMultipleLeaders
        expr: count(kscore_cluster_is_leader == 1) > 1
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "SPLIT-BRAIN: Multiple leaders detected"
          runbook_url: "https://docs.example.com/runbooks/split-brain"

      - alert: ClusterPartitioned
        expr: |
          count(kscore_cluster_member_reachable == 0) by (from_node) > 0
          and count(kscore_cluster_member_reachable == 1) by (from_node) > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Network partition detected in cluster"

      - alert: EtcdClusterDegraded
        expr: etcd_server_has_leader == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "etcd node has no leader - possible split-brain"
```

### Recovery Procedure

#### Phase 1: Isolate and Assess (5-10 minutes)

##### Step 1.1: Prevent Further Damage

```bash
# CRITICAL: Stop all write operations
# On ALL nodes that might be accepting writes:
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "kscorectl config set server.read_only true" 2>/dev/null || true
  ssh $node "systemctl restart kscore-server" 2>/dev/null || true
done

# Block agent connections temporarily (if needed)
# This prevents agents from making changes during recovery
iptables -A INPUT -p tcp --dport 4222 -j REJECT --reject-with tcp-reset
```

##### Step 1.2: Identify Partitions

```bash
# From each node, check connectivity to others
echo "=== Connectivity Matrix ==="
for src in ks-server-1 ks-server-2 ks-server-3; do
  echo -n "$src: "
  for dst in ks-server-1 ks-server-2 ks-server-3; do
    ssh $src "nc -zw1 $dst 2379 && echo -n '$dst:OK ' || echo -n '$dst:FAIL '" 2>/dev/null || echo -n "$dst:UNREACHABLE "
  done
  echo
done

# Document partition topology
# Example output:
# ks-server-1: ks-server-1:OK ks-server-2:OK ks-server-3:FAIL
# ks-server-2: ks-server-1:OK ks-server-2:OK ks-server-3:FAIL
# ks-server-3: ks-server-1:FAIL ks-server-2:FAIL ks-server-3:OK
#
# This shows: [Node1, Node2] | [Node3] partition
```

##### Step 1.3: Determine Authoritative Partition

Criteria for selecting authoritative partition (in order):

1. **Quorum**: Partition with majority of nodes
2. **Data freshness**: Partition with most recent data
3. **Agent count**: Partition serving more agents
4. **Manual selection**: If no clear winner, make explicit choice

```bash
# Check which partition has quorum
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node quorum status ==="
  ssh $node "kscorectl cluster status -o json | jq '.has_quorum'" 2>/dev/null || echo "unreachable"
done

# Check data timestamps (last write time via database file mtime)
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node last write ==="
  ssh $node "stat /var/lib/keystone-core/keystone-core.db 2>/dev/null | grep Modify" || echo "unreachable"
done

# Check agent counts
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node agents ==="
  ssh $node "kscorectl agent list --count 2>/dev/null" || echo "unreachable"
done
```

#### Phase 2: Resolve Network Partition (5-30 minutes)

##### Step 2.1: Diagnose Network Issue

```bash
# Check for network issues
traceroute ks-server-3
mtr --report ks-server-3

# Check firewall rules
iptables -L -n | grep -E "2379|4222|8080"

# Check cloud security groups (AWS)
aws ec2 describe-security-groups --group-ids sg-xxx

# Check DNS resolution
dig ks-server-3

# Check certificates
openssl s_client -connect ks-server-3:8080 -servername ks-server-3
```

##### Step 2.2: Fix Network Issue

```bash
# If firewall issue:
iptables -D INPUT -p tcp --dport 2379 -j DROP  # Remove bad rule
systemctl restart kscore-server

# If DNS issue:
# Update /etc/hosts or fix DNS records
echo "10.0.1.3 ks-server-3" >> /etc/hosts

# If cloud network issue:
# Update security groups, VPC peering, etc.

# If certificate issue:
# Regenerate or distribute certificates
```

##### Step 2.3: Verify Connectivity Restored

```bash
# Test connectivity from all nodes
for src in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== From $src ==="
  ssh $src "for dst in ks-server-1 ks-server-2 ks-server-3; do nc -zw1 \$dst 2379 && echo \"\$dst OK\" || echo \"\$dst FAIL\"; done"
done
```

#### Phase 3: Cluster Reconciliation (10-30 minutes)

##### Step 3.1: Stop Non-Authoritative Partition

```bash
# Identify non-authoritative partition (example: Node 3)
# Stop services on non-authoritative nodes
ssh ks-server-3 "systemctl stop kscore-server"
ssh ks-server-3 "systemctl stop etcd"
```

##### Step 3.2: Preserve Split Data (Optional)

```bash
# If data on non-authoritative partition might be needed
ssh ks-server-3 "
  # Backup local data before reset
  tar czf /backup/split-brain-data-$(date +%Y%m%d-%H%M%S).tar.gz \
    /var/lib/keystone-core \
    /var/lib/etcd
"
```

##### Step 3.3: Reset Non-Authoritative Nodes

```bash
# Remove cluster membership and data from non-authoritative nodes
ssh ks-server-3 "
  # Remove from etcd cluster first
  etcdctl member remove \$(etcdctl member list | grep ks-server-3 | cut -d',' -f1)

  # Clear local data
  rm -rf /var/lib/etcd/*
  rm -rf /var/lib/keystone-core/state/*
"
```

##### Step 3.4: Rejoin Nodes to Authoritative Cluster

```bash
# On authoritative cluster, add member back
kscorectl cluster add ks-server-3

# Prepare join configuration on the rejoining node
# The node will use cluster join command with appropriate token

# On rejoining node
ssh ks-server-3 "
  # Write join configuration
  echo '$JOIN_CONFIG' > /etc/keystone-core/join.yaml

  # Start etcd in join mode
  systemctl start etcd

  # Wait for etcd to sync
  sleep 30

  # Start kscore-server
  systemctl start kscore-server
"
```

##### Step 3.5: Verify Cluster Unified

```bash
# Check cluster membership
kscorectl cluster members

# Verify all nodes see same leader
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "$node leader: $(ssh $node 'kscorectl cluster leader')"
done

# Verify quorum
kscorectl cluster health

# Check etcd cluster
etcdctl endpoint status --cluster -w table
```

#### Phase 4: Data Reconciliation (15-60 minutes)

##### Step 4.1: Identify Data Conflicts

```bash
# Export agent list from authoritative partition
kscorectl agents list -o json > authoritative-agents.json

# Compare with backup from non-authoritative partition (if available)
# Check for agents that exist in backup but not in authoritative
jq -r '.[].id' authoritative-agents.json | sort > auth-ids.txt
jq -r '.[].id' backup-agents.json | sort > backup-ids.txt
comm -13 auth-ids.txt backup-ids.txt > missing-agents.txt
```

##### Step 4.2: Manual Conflict Resolution

```bash
# For split-brain scenarios, data reconciliation must be done manually:
# 1. Identify the authoritative partition (usually the one with more recent data
#    or the one that was accessible to more agents)
# 2. Restore from the authoritative partition's backup
# 3. Review logs from the non-authoritative partition for any critical operations
#    that need to be replayed manually

# Check server logs for operations during split-brain window
journalctl -u kscore-server --since "2024-01-15 10:00" --until "2024-01-15 12:00" \
  | grep -E "state apply|exec run|agent register"
```

##### Step 4.3: Replay Lost Operations

If the non-authoritative partition accepted commands that need to be preserved:

```bash
# Extract backup from non-authoritative partition
tar xzf /backup/split-brain-data-xxx.tar.gz -C /tmp/split-data

# Review logs from non-authoritative partition for operations to replay
# State applies during the split-brain window:
grep "state apply" /tmp/split-data/var/log/keystone-core/*.log

# Manual replay: re-apply any critical state files
# Review each state file and apply if appropriate
for state_file in /tmp/split-data/states/*.yaml; do
  echo "Review: $state_file"
  cat "$state_file"
  # After review, apply with:
  # kscorectl state apply "$state_file" --dry-run
done

# For exec commands, review and re-run manually as needed
grep "exec run" /tmp/split-data/var/log/keystone-core/*.log
```

> **Note**: Automated command replay is not currently implemented. Operations
> from the non-authoritative partition must be reviewed and replayed manually.

#### Phase 5: Agent Recovery (10-30 minutes)

##### Step 5.1: Enable Write Operations

```bash
# Re-enable writes on all nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "kscorectl config set server.read_only false"
  ssh $node "systemctl restart kscore-server"
done

# Re-enable agent connections (if blocked)
iptables -D INPUT -p tcp --dport 4222 -j REJECT --reject-with tcp-reset
```

##### Step 5.2: Force Agent Re-registration

```bash
# Agents connected to non-authoritative partition need to re-register
kscorectl agent invalidate-sessions --stale-since "2024-01-15T10:00:00Z"

# Agents will automatically reconnect and re-register

# Monitor agent reconnection
watch -n 5 'kscorectl agent list --status | grep -c connected'
```

##### Step 5.3: Verify Agent State

```bash
# Check all agents are connected
kscorectl agent list --status disconnected

# If agents stuck, force reconnect
kscorectl agent reconnect --target "status=disconnected"

# Verify agent data is consistent
kscorectl agent verify --sample 10
```

### Post-Recovery Checklist

- [ ] All cluster nodes healthy and communicating
- [ ] Single leader elected
- [ ] Quorum established
- [ ] All agents reconnected
- [ ] Data consistency verified
- [ ] No conflicting operations pending
- [ ] Monitoring alerts cleared
- [ ] Write operations re-enabled

### Prevention Measures

#### Network Redundancy

```yaml
# Use multiple network paths
cluster:
  peers:
    - name: ks-server-1
      peer_urls:
        - https://10.0.1.1:2380      # Primary network
        - https://172.16.1.1:2380    # Secondary network
```

#### Quorum Configuration

```yaml
# Ensure odd number of nodes for clear majority
cluster:
  min_quorum: 2  # For 3 nodes
  election_timeout: 5s
  heartbeat_interval: 1s
```

#### Split-Brain Prevention

```yaml
# Configure pre-vote to prevent disruption
etcd:
  pre_vote: true

  # Strict quorum checking
  strict_reconfig_check: true

  # Auto-compaction to limit divergence
  auto_compaction_retention: "1h"
```

#### Monitoring

```yaml
# Set up split-brain alerting
alerting:
  rules:
    - alert: PotentialSplitBrain
      expr: |
        (sum(kscore_cluster_members_connected) by (node) /
         count(kscore_cluster_members_total)) < 0.5
      for: 30s
      severity: warning
```

### Recovery Time Estimates

| Phase | Estimated Time |
|-------|----------------|
| Detection and assessment | 5-10 min |
| Network diagnosis | 5-15 min |
| Network repair | 5-30 min |
| Cluster reconciliation | 10-30 min |
| Data reconciliation | 15-60 min |
| Agent recovery | 10-30 min |
| Validation | 15 min |
| **Total** | **1-3 hours** |

### Decision Tree

```
Split-brain detected
        │
        ▼
┌───────────────────────┐
│ Network partition     │
│ currently active?     │
└───────────┬───────────┘
            │
     ┌──────┴──────┐
     │             │
    YES           NO
     │             │
     ▼             ▼
Fix network    Partitions may
first          have rejoined
     │         automatically
     │             │
     ▼             ▼
┌───────────────────────┐
│ Multiple leaders      │
│ detected?             │
└───────────┬───────────┘
            │
     ┌──────┴──────┐
     │             │
    YES           NO
     │             │
     ▼             ▼
Select         Check for
authoritative  data divergence
partition      only
     │             │
     ▼             ▼
Reset non-     Reconcile
authoritative  data
nodes              │
     │             │
     └──────┬──────┘
            │
            ▼
    Verify cluster
    unified
            │
            ▼
    Reconcile
    agent state
            │
            ▼
    Resume normal
    operations
```

## Appendix: Backup Locations

| Location | Purpose | Retention |
|----------|---------|-----------|
| s3://keystone-backups/ | Primary backups | 30 days |
| s3://keystone-backups-dr/ | DR backups (cross-region) | 30 days |
| /backup/local/ | Local fast restore | 7 days |
