# Runbook: Capacity Scaling

## Overview

This runbook covers scaling Keystone Core infrastructure to handle increased load or agent count.

## Prerequisites

- [ ] Current capacity metrics documented
- [ ] New infrastructure provisioned (if horizontal scaling)
- [ ] Maintenance window scheduled
- [ ] Backup completed
- [ ] Monitoring dashboards ready

## Trigger Conditions

- Agent count approaching capacity limits
- Resource utilization consistently > 70%
- Performance degradation due to scale
- Planned growth (new datacenter, acquisition)

## Scaling Decision Matrix

| Current Agents | Recommended Architecture | Control Plane | Database |
|----------------|-------------------------|---------------|----------|
| < 100 | Single node | 1 server | Embedded SQLite |
| 100-500 | Small cluster | 3 servers | Embedded SQLite |
| 500-2,000 | Medium cluster | 3-5 servers | PostgreSQL |
| 2,000-10,000 | Large cluster | 5+ servers | PostgreSQL HA |
| > 10,000 | Enterprise | Regional clusters | PostgreSQL HA + read replicas |

## Procedure

### Vertical Scaling (Single Node)

#### Step 1: Assess Current Usage

```bash
# Check current resource usage
top -bn1 | head -20
free -h
df -h

# Check connection counts
ss -s
nats server report connections 2>/dev/null || echo "NATS CLI not available"

# Check agent count
kscorectl agent list -o json | jq length

# Check database size
du -sh /var/lib/kscore/
```

#### Step 2: Resize Instance

```bash
# For cloud instances (example: AWS)
# 1. Stop instance
aws ec2 stop-instances --instance-ids i-xxx

# 2. Resize
aws ec2 modify-instance-attribute \
  --instance-id i-xxx \
  --instance-type "{\"Value\": \"m5.2xlarge\"}"

# 3. Start instance
aws ec2 start-instances --instance-ids i-xxx

# Verify
kscore-cluster status
```

#### Step 3: Update Configuration

```yaml
# /etc/kscore/server.yaml
server:
  workers: 16  # Increase with CPU cores
  max_connections: 10000

storage:
  driver: sqlite
  dsn: /var/lib/kscore/keystone.db

nats:
  max_payload: 8MB
```

```bash
systemctl restart kscore-server
```

### Horizontal Scaling (Add Nodes)

#### Step 1: Provision New Node

```bash
# Provision infrastructure (example: Terraform)
cd infrastructure/
terraform apply -var="node_count=4"

# Or manually provision and configure
# - Install OS
# - Configure networking
# - Install prerequisites
```

#### Step 2: Install Keystone Core

```bash
# On new node
curl -sSL https://install.keystone-core.io | sudo bash

# Copy configuration from existing node
scp ks-server-1:/etc/kscore/server.yaml /etc/kscore/
```

#### Step 3: Join Cluster

> **v0.x scope note**: There is no explicit join command. Cluster
> membership is automatic — a `kscore-server` joins by starting with its
> cluster config pointing at the existing etcd/peer endpoints, then
> self-registers. To add a node, configure its `cluster` section to
> reference the existing members and start the service.

```yaml
# /etc/kscore/server.yaml on the new node
cluster:
  endpoints:
    - https://ks-server-1:8080
    - https://ks-server-2:8080
    - https://ks-server-3:8080
  advertise_addr: 10.0.0.4:8080  # this node's reachable address
```

```bash
# Start the new server; it self-registers with the cluster
systemctl start kscore-server

# Verify cluster membership
kscore-cluster members
```

#### Step 4: Rebalance Workload

```bash
# Agents will automatically rebalance over time
# To force immediate rebalancing:
kscore-cluster rebalance

# Monitor rebalancing progress
watch -n 5 'kscorectl agent list -o json | jq "group_by(.control_plane_node) | .[] | {node: .[0].control_plane_node, count: length}"'
```

### Database Scaling

#### SQLite to PostgreSQL Migration

```bash
# 1. Provision PostgreSQL
# (Use managed service or deploy cluster)

# 2. Create database and user
psql -h postgres.example.com -U admin << EOF
CREATE DATABASE kscore;
CREATE USER kscore WITH ENCRYPTED PASSWORD 'secure-password';
GRANT ALL PRIVILEGES ON DATABASE kscore TO kscore;
EOF

# 3. Run migration
kscore-migrate run \
  --source sqlite:///var/lib/kscore/keystone.db \
  --target postgres://kscore:password@postgres.example.com/kscore \
  --dry-run

# If dry-run succeeds:
kscore-migrate run \
  --source sqlite:///var/lib/kscore/keystone.db \
  --target postgres://kscore:password@postgres.example.com/kscore

# 4. Update configuration
cat >> /etc/kscore/server.yaml << EOF
storage:
  driver: postgres
  dsn: postgres://kscore:secure-password@postgres.example.com:5432/kscore?sslmode=require
EOF

# 5. Restart and verify
systemctl restart kscore-server
kscore-cluster status
```

#### PostgreSQL Read Replicas

```bash
# Configure primary for replication
psql -h postgres-primary -U kscore << EOF
ALTER SYSTEM SET wal_level = replica;
ALTER SYSTEM SET max_wal_senders = 5;
SELECT pg_reload_conf();
EOF

# Point Keystone at the primary via its DSN
cat >> /etc/kscore/server.yaml << EOF
storage:
  driver: postgres
  dsn: postgres://kscore:secure-password@postgres-primary.example.com:5432/kscore?sslmode=require
EOF

systemctl restart kscore-server
```

> **v0.x scope note**: The storage config exposes only `storage.driver`
> and `storage.dsn` (single connection string) — there is no built-in
> read-replica routing. To offload reads to replicas, front PostgreSQL
> with an external pooler/load balancer (e.g. PgBouncer or HAProxy) and
> point `storage.dsn` at it.

### NATS Scaling

#### Add NATS Cluster Nodes

```bash
# 1. Install NATS on new node
curl -sSL https://nats.io/install | sh

# 2. Configure cluster membership
cat > /etc/nats/nats.conf << EOF
server_name: nats-4
listen: 0.0.0.0:4222
cluster {
  name: kscore
  listen: 0.0.0.0:6222
  routes: [
    nats://nats-1:6222,
    nats://nats-2:6222,
    nats://nats-3:6222
  ]
}
jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 4GB
  max_file_store: 100GB
}
EOF

# 3. Start NATS
systemctl start nats

# 4. Verify cluster
nats server report
```

#### Scale JetStream

```bash
# Increase stream replicas
nats stream edit KSCORE_COMMANDS --replicas 5

# Increase stream limits
nats stream edit KSCORE_COMMANDS \
  --max-bytes 100GB \
  --max-msgs 10000000

# Add more consumers for parallel processing
# Edit /etc/kscore/server.yaml:
#   nats:
#     consumer_count: 10
# Then restart: systemctl restart kscore-server
```

### Agent Capacity Planning

#### Estimate Agent Capacity

```
Control Plane Capacity =
  (Available_Memory - OS_Overhead) / (Memory_Per_Agent)

Where:
  Memory_Per_Agent ≈ 50KB (metadata) + 10KB (connection state)

Example:
  8GB server - 2GB OS = 6GB available
  6GB / 60KB = ~100,000 agents theoretical max

Practical limit: 50% of theoretical = ~50,000 agents
```

#### Configure for Scale

```yaml
# High-scale configuration
# /etc/kscore/server.yaml

server:
  workers: 32
  max_connections: 100000
  read_timeout: 60s
  write_timeout: 60s

agent:
  heartbeat_interval: 60s  # Reduce frequency for scale
  heartbeat_timeout: 180s
  batch_size: 1000  # Batch operations

state:
  parallel_apply: 100  # Concurrent state applications
  batch_size: 500

nats:
  pool_size: 10
  pending_limit: 256MB
```

### Regional/Multi-Cluster Scaling

#### Deploy Regional Cluster

```bash
# Bootstrap a new regional cluster from a seed file
kscore-bootstrap --seed us-west-seed.yaml
```

> **v0.x scope note**: Multi-cluster federation is not in v0.1 (planned
> post-v1.0/v2.x); tracked in [`ROADMAP.md`](../project/ROADMAP.md). In
> v0.x each regional cluster is independent — there is no
> `kscorectl federation` command or `federation` config block, and no
> cross-cluster agent routing.

## Verification Checklist

- [ ] New nodes healthy and in cluster
- [ ] Agents balanced across nodes
- [ ] Database accessible from all nodes
- [ ] NATS cluster healthy with correct replica count
- [ ] Performance metrics within targets
- [ ] No errors in logs
- [ ] Monitoring updated for new infrastructure

## Rollback

### Remove Added Node

> **v0.x scope note**: Graceful drain (controlled shard migration before
> removal) is post-v1.0. In v0.x the available operation is eviction:
> `kscore-cluster remove <member-id>` removes the member, and its shards
> are reassigned by the cluster afterward.

```bash
# Identify the member ID
kscore-cluster members

# Remove (evict) the member from the cluster
kscore-cluster remove ks-server-4

# Decommission infrastructure
```

### Revert Database Migration

```bash
# If PostgreSQL migration fails, revert to SQLite
# (requires recent backup)

# 1. Stop all control plane nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "systemctl stop kscore-server"
done

# 2. Restore SQLite backup
cp /backup/keystone.db /var/lib/kscore/keystone.db

# 3. Revert configuration: set storage back to the sqlite driver + dsn
#    storage:
#      driver: sqlite
#      dsn: /var/lib/kscore/keystone.db
$EDITOR /etc/kscore/server.yaml

# 4. Start services
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "systemctl start kscore-server"
done
```

## Post-Procedure

1. [ ] Update capacity documentation
2. [ ] Update monitoring thresholds
3. [ ] Update runbooks for new scale
4. [ ] Conduct load testing
5. [ ] Update disaster recovery procedures
6. [ ] Schedule follow-up capacity review

## Appendix: Capacity Metrics

| Metric | Current | Target | Max |
|--------|---------|--------|-----|
| Agent Count | | | |
| Control Plane CPU | | < 70% | 100% |
| Control Plane Memory | | < 70% | 100% |
| Database Size | | | |
| NATS Messages/sec | | | |
| API P95 Latency | | < 100ms | 500ms |

## Appendix: Cost Estimation

| Scale | Agents | Control Plane | Database | Est. Monthly Cost |
|-------|--------|---------------|----------|-------------------|
| Small | 100 | 1x m5.large | SQLite | $70 |
| Medium | 1,000 | 3x m5.xlarge | SQLite | $450 |
| Large | 5,000 | 5x m5.2xlarge | RDS r5.large | $2,000 |
| Enterprise | 50,000 | Regional clusters | RDS r5.2xlarge | $10,000+ |

*Costs are approximate and vary by cloud provider and region.*
