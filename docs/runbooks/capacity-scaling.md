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
kscorectl agents list -o json | jq length

# Check database size
du -sh /var/lib/keystone-core/
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
kscorectl cluster health
```

#### Step 3: Update Configuration

```yaml
# /etc/keystone-core/server.yaml
server:
  workers: 16  # Increase with CPU cores
  max_connections: 10000

database:
  sqlite:
    cache_size: -128000  # 128MB for more memory

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
scp ks-server-1:/etc/keystone-core/server.yaml /etc/keystone-core/
```

#### Step 3: Join Cluster

```bash
# Get join token from cluster config or bootstrap output
JOIN_TOKEN="$CLUSTER_JOIN_TOKEN"  # From /etc/keystone-core/server.yaml

# Join new node to cluster
kscorectl cluster join \
  --server https://ks-server-1:8080 \
  --token $JOIN_TOKEN \
  --advertise-addr $(hostname -I | awk '{print $1}')

# Verify cluster membership
kscorectl cluster members
```

#### Step 4: Rebalance Workload

```bash
# Agents will automatically rebalance over time
# To force immediate rebalancing:
kscorectl cluster rebalance

# Monitor rebalancing progress
watch -n 5 'kscorectl agent list --group-by control-plane-node | head -20'
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
  --source sqlite:///var/lib/keystone-core/keystone.db \
  --target postgres://kscore:password@postgres.example.com/kscore \
  --dry-run

# If dry-run succeeds:
kscore-migrate run \
  --source sqlite:///var/lib/keystone-core/keystone.db \
  --target postgres://kscore:password@postgres.example.com/kscore

# 4. Update configuration
cat >> /etc/keystone-core/server.yaml << EOF
database:
  type: postgres
  postgres:
    host: postgres.example.com
    port: 5432
    database: kscore
    user: kscore
    password: secure-password
    sslmode: require
    pool:
      max_connections: 50
EOF

# 5. Restart and verify
systemctl restart kscore-server
kscorectl cluster health
```

#### PostgreSQL Read Replicas

```bash
# Configure primary for replication
psql -h postgres-primary -U kscore << EOF
ALTER SYSTEM SET wal_level = replica;
ALTER SYSTEM SET max_wal_senders = 5;
SELECT pg_reload_conf();
EOF

# Update Keystone to use read replicas
cat >> /etc/keystone-core/server.yaml << EOF
database:
  postgres:
    primary: postgres-primary.example.com
    replicas:
      - postgres-replica-1.example.com
      - postgres-replica-2.example.com
    read_preference: replica  # Route reads to replicas
EOF

systemctl restart kscore-server
```

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
kscorectl config set nats.consumer_count 10
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
# /etc/keystone-core/server.yaml

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
# 1. Bootstrap new regional cluster
kscore-bootstrap init \
  --cluster-name us-west \
  --region us-west-2

# 2. Configure federation
kscorectl federation add \
  --cluster us-west \
  --endpoint https://us-west.kscore.example.com:8080 \
  --token "$US_WEST_FEDERATION_TOKEN"  # From us-west cluster config

# 3. Configure agent routing
cat >> /etc/keystone-core/server.yaml << EOF
federation:
  enabled: true
  clusters:
    us-east:
      endpoint: https://us-east.kscore.example.com:8080
    us-west:
      endpoint: https://us-west.kscore.example.com:8080
  routing:
    strategy: geographic  # Route agents to nearest cluster
EOF
```

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

```bash
# Gracefully drain node
kscorectl cluster drain ks-server-4

# Wait for agents to migrate
sleep 300

# Remove from cluster
kscorectl cluster member remove ks-server-4

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
cp /backup/keystone.db /var/lib/keystone-core/keystone.db

# 3. Revert configuration
sed -i 's/type: postgres/type: sqlite/' /etc/keystone-core/server.yaml

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
