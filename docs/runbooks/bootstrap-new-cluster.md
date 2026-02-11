# Runbook: Bootstrap New Cluster

## Overview

This runbook covers bootstrapping a new Keystone Core cluster from scratch using seed configurations, blueprints, or manual setup. It includes detailed procedures for single-node, HA cluster, and multi-region deployments.

## Prerequisites

### Infrastructure Requirements

| Component | Minimum (Demo) | Recommended (Production) | Enterprise |
|-----------|---------------|-------------------------|------------|
| Control Plane Nodes | 1 | 3 | 5+ per region |
| CPU per Node | 2 cores | 4 cores | 8+ cores |
| Memory per Node | 4GB | 8GB | 16GB+ |
| Storage per Node | 50GB SSD | 200GB SSD | 500GB+ NVMe |
| Network | 1 Gbps | 10 Gbps | 10 Gbps+ |

### Software Requirements

- [ ] Linux OS (Ubuntu 20.04+, RHEL 8+, or Debian 11+)
- [ ] Root or sudo access
- [ ] curl, tar, openssl installed
- [ ] systemd for service management

### Network Requirements

| Port | Protocol | Purpose | Required |
|------|----------|---------|----------|
| 8080 | TCP | API server | Yes |
| 8443 | TCP | API server (TLS) | Yes |
| 4222 | TCP | NATS client | Yes |
| 6222 | TCP | NATS cluster | HA only |
| 8222 | TCP | NATS monitoring | Optional |
| 2379 | TCP | etcd client | HA only |
| 2380 | TCP | etcd peer | HA only |
| 5432 | TCP | PostgreSQL | External DB |
| 9090 | TCP | Metrics | Optional |

### Checklist

- [ ] Infrastructure provisioned (servers, networking, storage)
- [ ] SSH access to all nodes
- [ ] Seed configuration file prepared (`seed.yaml`) OR blueprint selected
- [ ] DNS entries configured (if applicable)
- [ ] Firewall rules configured
- [ ] Required credentials available (database, cloud storage)
- [ ] TLS certificates prepared (or use auto-generation)
- [ ] Backup storage configured (S3, NFS, etc.)

## Trigger Conditions

- Initial Keystone Core deployment
- Creating a new isolated cluster
- DR cluster provisioning
- Multi-region expansion
- Development/testing environment setup

## Deployment Scenarios

### Scenario 1: Single-Node Demo Deployment

Best for: Evaluation, development, CI/CD testing.

### Scenario 2: HA Cluster Deployment

Best for: Production workloads with high availability requirements.

### Scenario 3: Multi-Region Enterprise Deployment

Best for: Global enterprises with geo-distributed infrastructure.

---

## Procedure: Single-Node Demo Deployment

### Step 1: Download and Install

```bash
# Download the installer
curl -sSL https://install.keystone-core.io | sudo bash

# Or download manually
curl -LO https://releases.keystone-core.io/latest/kscore-bootstrap-linux-amd64.tar.gz
tar -xzf kscore-bootstrap-linux-amd64.tar.gz
sudo mv kscore-bootstrap /usr/local/bin/
sudo chmod +x /usr/local/bin/kscore-bootstrap

# Verify installation
kscore-bootstrap version
```

### Step 2: Create Demo Seed Configuration

```bash
# Create minimal seed configuration
cat > /tmp/seed.yaml << 'EOF'
apiVersion: bootstrap.keystone-core.io/v1
kind: SeedConfiguration

metadata:
  name: demo-cluster
  description: Single-node demo deployment

cluster:
  name: demo
  mode: single  # single, ha, enterprise

server:
  bind_address: 0.0.0.0
  http_port: 8080
  grpc_port: 9090

  tls:
    enabled: true
    auto_generate: true
    cert_validity: 8760h  # 1 year

database:
  type: sqlite
  sqlite:
    path: /var/lib/keystone-core/keystone.db
    cache_size: -64000  # 64MB

nats:
  embedded: true
  jetstream:
    enabled: true
    store_dir: /var/lib/keystone-core/jetstream
    max_memory: 1GB
    max_file: 10GB

admin:
  username: admin
  password_hash: ""  # Will prompt during bootstrap
  # Or specify: password_hash: "$argon2id$v=19$m=65536,t=3,p=2$..."

observability:
  metrics:
    enabled: true
    port: 9091
  logging:
    level: info
    format: json
EOF
```

### Step 3: Run Bootstrap

```bash
# Validate configuration
kscore-bootstrap validate --config /tmp/seed.yaml

# Run bootstrap (will prompt for admin password)
sudo kscore-bootstrap seed --config /tmp/seed.yaml

# Or specify password via environment
export KSCORE_ADMIN_PASSWORD="your-secure-password"
sudo kscore-bootstrap seed --config /tmp/seed.yaml
```

### Step 4: Verify Installation

```bash
# Check service status
systemctl status kscore-server

# Check logs
journalctl -u kscore-server -f

# Test API
curl -k https://localhost:8080/health/ready

# Verify CLI connectivity
kscorectl cluster health
```

---

## Procedure: HA Cluster Deployment

### Step 1: Prepare All Nodes

On each node (ks-server-1, ks-server-2, ks-server-3):

```bash
# Install kscore-bootstrap
curl -sSL https://install.keystone-core.io | sudo bash

# Verify installation
kscore-bootstrap version

# Set hostname (if not already set)
sudo hostnamectl set-hostname ks-server-1  # Adjust for each node

# Update /etc/hosts (on all nodes)
cat >> /etc/hosts << 'EOF'
10.0.1.10 ks-server-1
10.0.1.11 ks-server-2
10.0.1.12 ks-server-3
EOF

# Verify network connectivity
for node in ks-server-1 ks-server-2 ks-server-3; do
  ping -c 3 $node
done
```

### Step 2: Prepare External PostgreSQL (Recommended)

```bash
# On PostgreSQL server (or use managed service)
psql -U postgres << 'EOF'
-- Create database
CREATE DATABASE kscore;

-- Create user
CREATE USER kscore WITH ENCRYPTED PASSWORD 'secure-password';

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE kscore TO kscore;

-- Create replication user (for HA)
CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'repl-password';

-- Enable required extensions
\c kscore
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
EOF
```

### Step 3: Create HA Seed Configuration

```bash
cat > /tmp/seed-ha.yaml << 'EOF'
apiVersion: bootstrap.keystone-core.io/v1
kind: SeedConfiguration

metadata:
  name: production-cluster
  description: HA production deployment

cluster:
  name: prod
  mode: ha

  nodes:
    - name: ks-server-1
      address: 10.0.1.10
      roles: [control-plane, etcd]
    - name: ks-server-2
      address: 10.0.1.11
      roles: [control-plane, etcd]
    - name: ks-server-3
      address: 10.0.1.12
      roles: [control-plane, etcd]

  etcd:
    initial_cluster_token: kscore-prod-cluster
    auto_compaction_mode: periodic
    auto_compaction_retention: "24h"
    snapshot_count: 10000

server:
  bind_address: 0.0.0.0
  http_port: 8080
  grpc_port: 9090

  tls:
    enabled: true
    auto_generate: true
    ca_validity: 87600h      # 10 years
    cert_validity: 8760h     # 1 year
    auto_rotate: true
    rotate_before: 720h      # Rotate 30 days before expiry

  load_balancer:
    enabled: true
    virtual_ip: 10.0.1.100   # If using keepalived
    # Or use external load balancer
    # external: lb.example.com

database:
  type: postgres
  postgres:
    host: postgres.internal
    port: 5432
    database: kscore
    user: kscore
    password: "${POSTGRES_PASSWORD}"  # From environment
    sslmode: require
    pool:
      max_connections: 50
      min_connections: 5
      max_lifetime: 1h

nats:
  embedded: true
  cluster:
    enabled: true
    name: kscore-nats
  jetstream:
    enabled: true
    store_dir: /var/lib/keystone-core/jetstream
    max_memory: 4GB
    max_file: 100GB
    replicas: 3

admin:
  username: admin
  password_hash: ""  # Will prompt

observability:
  metrics:
    enabled: true
    port: 9091
  logging:
    level: info
    format: json
  tracing:
    enabled: true
    endpoint: http://jaeger:14268/api/traces
    sampling_rate: 0.1

backup:
  enabled: true
  schedule: "0 2 * * *"  # 2 AM daily
  retention_days: 30
  destination:
    type: s3
    s3:
      bucket: kscore-backups
      region: us-east-1
      prefix: prod/
EOF
```

### Step 4: Bootstrap First Node

```bash
# SSH to first node
ssh ks-server-1

# Set required environment variables
export POSTGRES_PASSWORD="secure-password"
export KSCORE_ADMIN_PASSWORD="admin-password"

# Copy seed configuration
scp seed-ha.yaml ks-server-1:/tmp/

# Validate configuration
kscore-bootstrap validate --config /tmp/seed-ha.yaml

# Dry run first
kscore-bootstrap seed \
  --config /tmp/seed-ha.yaml \
  --node ks-server-1 \
  --dry-run

# Execute bootstrap
sudo kscore-bootstrap seed \
  --config /tmp/seed-ha.yaml \
  --node ks-server-1

# Get join token from bootstrap output or cluster configuration
# The token is generated during initial cluster seed and stored in cluster config
JOIN_TOKEN="$CLUSTER_JOIN_TOKEN"  # Set from bootstrap output or config file
echo "Join token: $JOIN_TOKEN"
```

### Step 5: Join Additional Nodes

On ks-server-2:

```bash
# Join cluster
sudo kscore-bootstrap join \
  --server https://ks-server-1:8080 \
  --token "$JOIN_TOKEN" \
  --node ks-server-2 \
  --advertise-address 10.0.1.11

# Verify join
kscorectl cluster members
```

On ks-server-3:

```bash
# Join cluster
sudo kscore-bootstrap join \
  --server https://ks-server-1:8080 \
  --token "$JOIN_TOKEN" \
  --node ks-server-3 \
  --advertise-address 10.0.1.12

# Verify join
kscorectl cluster members
```

### Step 6: Verify HA Cluster

```bash
# From any node
kscorectl cluster health

# Expected output:
# Cluster: prod
# Status: healthy
# Leader: ks-server-1
# Members:
#   ks-server-1: healthy (leader)
#   ks-server-2: healthy (follower)
#   ks-server-3: healthy (follower)
# Quorum: 3/3
# NATS: healthy (3/3 nodes)
# Database: healthy

# Test leader election
# Stop leader and verify failover
ssh ks-server-1 "sudo systemctl stop kscore-server"

# Check new leader
kscorectl cluster health

# Restart original leader
ssh ks-server-1 "sudo systemctl start kscore-server"
```

---

## Procedure: Blueprint-Based Deployment

### Using Production Cluster Blueprint

```bash
# Create parameters file
cat > params.yaml << 'EOF'
cluster_name: prod-east
node_count: 3
postgres_host: postgres.internal
postgres_database: kscore
postgres_user: kscore
postgres_password: !secret databases/postgres/kscore
nats_urls:
  - nats://nats-1:4222
  - nats://nats-2:4222
  - nats://nats-3:4222
EOF

# Bootstrap with blueprint
kscore-bootstrap seed \
  --apply-blueprint kscore/production-cluster \
  --params params.yaml

# Or with inline parameters
kscore-bootstrap seed \
  --apply-blueprint kscore/production-cluster \
  --param cluster_name=prod-east \
  --param postgres_host=postgres.internal \
  --param postgres_password="${POSTGRES_PASSWORD}"
```

### Using Enterprise Blueprint

```bash
# Create enterprise configuration
cat > enterprise-params.yaml << 'EOF'
cluster_name: global-platform
regions:
  - name: us-east
    primary: true
    nodes:
      - ks-east-1.example.com
      - ks-east-2.example.com
      - ks-east-3.example.com
    postgres_host: postgres-us-east.internal
    nats_urls:
      - nats://nats-east-1:4222
      - nats://nats-east-2:4222
  - name: eu-west
    primary: false
    nodes:
      - ks-eu-1.example.com
      - ks-eu-2.example.com
      - ks-eu-3.example.com
    postgres_host: postgres-eu-west.internal
    nats_urls:
      - nats://nats-eu-1:4222
      - nats://nats-eu-2:4222
federation_enabled: true
identity_provider: oidc
oidc_issuer: https://auth.example.com
oidc_client_id: kscore-platform
oidc_client_secret: !secret auth/oidc/secret
EOF

# Bootstrap enterprise platform
kscore-bootstrap seed \
  --apply-blueprint kscore/enterprise-platform \
  --params enterprise-params.yaml
```

---

## Procedure: Kubernetes Deployment

### Deploy via Helm

```bash
# Add Keystone Core Helm repo
helm repo add kscore https://charts.keystone-core.io
helm repo update

# Create namespace
kubectl create namespace kscore-system

# Create secrets
kubectl create secret generic kscore-secrets \
  --namespace kscore-system \
  --from-literal=admin-password="${ADMIN_PASSWORD}" \
  --from-literal=postgres-password="${POSTGRES_PASSWORD}"

# Install with custom values
cat > values.yaml << 'EOF'
controlPlane:
  replicas: 3

  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 2000m
      memory: 2Gi

database:
  type: postgres
  postgres:
    host: postgres.database.svc
    database: kscore
    existingSecret: kscore-secrets
    secretKey: postgres-password

nats:
  enabled: true
  replicas: 3
  jetstream:
    enabled: true
    fileStorage:
      enabled: true
      size: 50Gi
      storageClassName: fast-ssd

ingress:
  enabled: true
  className: nginx
  hosts:
    - kscore.example.com
  tls:
    - secretName: kscore-tls
      hosts:
        - kscore.example.com

monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
EOF

# Install
helm install kscore kscore/kscore \
  --namespace kscore-system \
  --values values.yaml \
  --wait

# Verify installation
kubectl get pods -n kscore-system
kubectl get svc -n kscore-system
```

### Deploy via Operator

```bash
# Install operator
kubectl apply -f https://releases.keystone-core.io/operator/latest/install.yaml

# Create KSCoreCluster resource
cat << 'EOF' | kubectl apply -f -
apiVersion: keystone-core.io/v1
kind: KSCoreCluster
metadata:
  name: production
  namespace: kscore-system
spec:
  controlPlane:
    replicas: 3
  database:
    type: postgres
    postgres:
      host: postgres.database.svc
      database: kscore
      secretRef:
        name: postgres-credentials
  nats:
    replicas: 3
    jetstream:
      enabled: true
  tls:
    mode: auto
EOF

# Watch deployment progress
kubectl get kscorecluster production -n kscore-system -w
```

---

## Seed Configuration Reference

### Complete Seed Configuration Schema

```yaml
apiVersion: bootstrap.keystone-core.io/v1
kind: SeedConfiguration

metadata:
  name: string                    # Cluster name (required)
  description: string             # Human-readable description
  labels:                         # Custom labels
    environment: production
    region: us-east

cluster:
  name: string                    # Short cluster name
  mode: string                    # single, ha, enterprise

  nodes:                          # For HA/enterprise mode
    - name: string                # Node hostname
      address: string             # Node IP address
      roles: [string]             # control-plane, etcd, nats
      labels: {}                  # Node labels

  etcd:                           # etcd configuration (HA mode)
    initial_cluster_token: string
    auto_compaction_mode: string  # periodic, revision
    auto_compaction_retention: string
    snapshot_count: integer
    quota_backend_bytes: integer

  federation:                     # Enterprise multi-cluster
    enabled: boolean
    trust_domain: string
    remote_clusters:
      - name: string
        endpoint: string
        bundle_endpoint: string

server:
  bind_address: string            # 0.0.0.0 for all interfaces
  http_port: integer              # API HTTP port (default: 8080)
  grpc_port: integer              # gRPC port (default: 9090)
  workers: integer                # Worker threads (default: auto)
  max_connections: integer        # Max concurrent connections

  tls:
    enabled: boolean
    auto_generate: boolean        # Auto-generate certificates
    ca_cert: string               # Path to CA certificate
    cert: string                  # Path to server certificate
    key: string                   # Path to private key
    ca_validity: string           # CA validity duration
    cert_validity: string         # Certificate validity duration
    auto_rotate: boolean          # Auto-rotate before expiry
    rotate_before: string         # Rotate this duration before expiry

  load_balancer:
    enabled: boolean
    type: string                  # keepalived, external
    virtual_ip: string            # VIP for keepalived
    external: string              # External LB hostname

database:
  type: string                    # sqlite, postgres

  sqlite:                         # SQLite configuration
    path: string
    cache_size: integer           # Negative = KB, positive = pages
    journal_mode: string          # wal, delete, truncate
    synchronous: string           # off, normal, full

  postgres:                       # PostgreSQL configuration
    host: string
    port: integer
    database: string
    user: string
    password: string              # Or ${ENV_VAR}
    sslmode: string               # disable, require, verify-full
    pool:
      max_connections: integer
      min_connections: integer
      max_lifetime: string
      health_check_interval: string

nats:
  embedded: boolean               # Use embedded NATS
  urls: [string]                  # External NATS URLs
  credentials_file: string        # NATS credentials file

  cluster:
    enabled: boolean
    name: string
    routes: [string]              # Cluster routes

  jetstream:
    enabled: boolean
    store_dir: string
    max_memory: string
    max_file: string
    replicas: integer             # Stream replicas

  tls:
    enabled: boolean
    cert: string
    key: string
    ca: string

admin:
  username: string                # Admin username
  password: string                # Plaintext password (not recommended)
  password_hash: string           # Argon2id hash
  email: string                   # Admin email

observability:
  metrics:
    enabled: boolean
    port: integer
    path: string                  # Default: /metrics

  logging:
    level: string                 # debug, info, warn, error
    format: string                # json, text
    output: string                # stdout, file path

  tracing:
    enabled: boolean
    endpoint: string              # Jaeger/OTLP endpoint
    sampling_rate: number         # 0.0 - 1.0

  audit:
    enabled: boolean
    destination: string           # file, syslog, webhook
    retention_days: integer

backup:
  enabled: boolean
  schedule: string                # Cron expression
  retention_days: integer
  destination:
    type: string                  # s3, gcs, azure, local, nfs
    s3:
      bucket: string
      region: string
      endpoint: string            # For S3-compatible storage
      prefix: string
      access_key: string
      secret_key: string
    gcs:
      bucket: string
      project: string
      credentials_file: string
    azure:
      container: string
      account: string
      key: string
    local:
      path: string
    nfs:
      server: string
      path: string

security:
  policy:
    default_deny: boolean         # Deny by default
    audit_all: boolean            # Audit all operations

  secrets:
    backend: string               # vault, kubernetes, file
    vault:
      address: string
      auth_method: string
      role: string
      path: string
```

---

## Verification Checklist

### Single-Node Deployment

- [ ] Service running: `systemctl status kscore-server`
- [ ] API healthy: `curl -k https://localhost:8080/health/ready`
- [ ] CLI connected: `kscorectl cluster health`
- [ ] Metrics available: `curl http://localhost:9091/metrics`
- [ ] NATS healthy: `nats server check connection`

### HA Cluster Deployment

- [ ] All nodes running: `kscorectl cluster members`
- [ ] Cluster healthy: `kscorectl cluster health`
- [ ] Leader elected: `kscorectl cluster leader`
- [ ] NATS cluster formed: `nats server report`
- [ ] Database connected: `curl -s http://localhost:8080/health/ready` (checks DB connectivity)
- [ ] Quorum established: `kscorectl cluster status` (check has_quorum field)
- [ ] Failover works: Stop leader, verify new leader elected
- [ ] API accessible via LB: `curl -k https://lb.example.com:8080/health/ready`

### Enterprise Deployment

- [ ] All regions online: `kscorectl federation status`
- [ ] Federation trust established: `kscorectl federation trust list`
- [ ] Cross-region communication: `kscorectl federation ping --region eu-west`
- [ ] OIDC authentication works: Test login via SSO
- [ ] Audit logging active: `kscorectl audit search --limit 10`
- [ ] Backups scheduled: `kscore-cluster-backup list`

---

## Rollback Procedures

### Failed Bootstrap Cleanup

```bash
# Stop services
sudo systemctl stop kscore-server || true
sudo systemctl stop nats-server || true

# Clean up data directories
sudo rm -rf /var/lib/keystone-core/*
sudo rm -rf /etc/keystone-core/certs/*

# Remove systemd units
sudo rm -f /etc/systemd/system/kscore-server.service
sudo rm -f /etc/systemd/system/nats-server.service
sudo systemctl daemon-reload

# Clean up any remaining processes
pkill -f kscore-server || true
pkill -f nats-server || true

# Review logs for root cause
sudo journalctl -u kscore-server -n 200 --no-pager
```

### Rollback to Previous Version

```bash
# Stop service
sudo systemctl stop kscore-server

# Backup current state
sudo cp -r /var/lib/keystone-core /var/lib/keystone-core.bak
sudo cp -r /etc/keystone-core /etc/keystone-core.bak

# Download previous version
curl -LO https://releases.keystone-core.io/v1.4.0/kscore-server-linux-amd64
sudo mv kscore-server-linux-amd64 /usr/local/bin/kscore-server
sudo chmod +x /usr/local/bin/kscore-server

# Restore previous configuration if needed
# sudo cp -r /backup/kscore-config/* /etc/keystone-core/

# Start service
sudo systemctl start kscore-server

# Verify
kscorectl cluster health
```

---

## Post-Bootstrap Tasks

1. **Documentation**
   - [ ] Document cluster details in CMDB
   - [ ] Record admin credentials securely
   - [ ] Document network topology
   - [ ] Update runbooks with specific cluster details

2. **Security**
   - [ ] Change default admin password
   - [ ] Configure RBAC roles
   - [ ] Set up API key rotation
   - [ ] Enable audit logging

3. **Monitoring**
   - [ ] Configure Prometheus scraping
   - [ ] Import Grafana dashboards
   - [ ] Set up alert rules
   - [ ] Configure on-call notifications

4. **Backup**
   - [ ] Verify backup job runs successfully
   - [ ] Test backup restoration
   - [ ] Document backup location and encryption keys

5. **Agents**
   - [ ] Generate agent registration tokens
   - [ ] Deploy first agents
   - [ ] Verify agent connectivity
   - [ ] Set up agent auto-update policy

6. **Integration**
   - [ ] Configure GitOps repository
   - [ ] Set up webhook integrations
   - [ ] Configure secret backend
   - [ ] Test end-to-end workflow

---

## Troubleshooting

### Bootstrap Fails to Start

```bash
# Check prerequisites
kscore-bootstrap prereq-check

# Common issues:
# - Insufficient disk space
# - Port already in use
# - SELinux/AppArmor blocking
# - Missing dependencies

# Check port availability
ss -tlnp | grep -E '(8080|4222|6222|2379)'

# Check SELinux
getenforce
# If enforcing, check for denials:
ausearch -m avc -ts recent

# Check system resources
free -h
df -h
```

### Certificate Generation Fails

```bash
# Check for existing certificates
ls -la /etc/keystone-core/certs/

# Remove stale certificates
sudo rm -rf /etc/keystone-core/certs/*

# Check openssl is available
openssl version

# Try manual certificate generation
sudo kscore-bootstrap cert-gen \
  --ca-cn "Keystone Core CA" \
  --server-cn "$(hostname -f)" \
  --output /etc/keystone-core/certs/
```

### NATS Cluster Formation Issues

```bash
# Check NATS logs
journalctl -u nats-server -n 100 --no-pager

# Verify network connectivity
for port in 4222 6222 8222; do
  for node in ks-server-1 ks-server-2 ks-server-3; do
    nc -zv $node $port && echo "$node:$port OK" || echo "$node:$port FAILED"
  done
done

# Check NATS configuration
cat /etc/nats/nats.conf

# Verify cluster routes
nats server info --server nats://localhost:4222

# Force cluster rejoin
sudo systemctl restart nats-server
```

### etcd Cluster Issues

```bash
# Check etcd health
etcdctl endpoint health

# Check cluster membership
etcdctl member list

# Check leader
etcdctl endpoint status

# If member is unhealthy, try removing and re-adding
MEMBER_ID=$(etcdctl member list | grep unhealthy-node | cut -d, -f1)
etcdctl member remove $MEMBER_ID
etcdctl member add unhealthy-node --peer-urls=https://10.0.1.12:2380
```

### Database Connection Issues

```bash
# Test PostgreSQL connectivity
psql -h postgres.internal -U kscore -d kscore -c "SELECT 1"

# Check health endpoint (includes database status)
curl -s http://localhost:8080/health/ready

# Check server logs for database connection info
journalctl -u kscore-server | grep -i "database\|postgres\|sqlite"

# Check for connection limits
psql -h postgres.internal -U postgres -c "SELECT * FROM pg_stat_activity WHERE datname = 'kscore'"

# Verify SSL mode
psql "host=postgres.internal dbname=kscore user=kscore sslmode=require" -c "SELECT 1"

# Check DNS resolution
nslookup postgres.internal
dig postgres.internal
```

### Node Join Failures

```bash
# Verify cluster is accessible (token validation happens during join)
curl -k https://ks-server-1:8080/health/ready

# Check network connectivity to existing nodes
curl -k https://ks-server-1:8080/health/ready

# Check TLS certificate chain
openssl s_client -connect ks-server-1:8080 -showcerts

# Check time synchronization
timedatectl status
chronyc tracking

# Retry with verbose logging
sudo kscore-bootstrap join \
  --server https://ks-server-1:8080 \
  --token "$JOIN_TOKEN" \
  --debug
```

---

## Performance Tuning

### Control Plane Optimization

```yaml
# /etc/keystone-core/server.yaml additions for high load
server:
  workers: 32                    # Match CPU cores
  max_connections: 100000
  read_timeout: 60s
  write_timeout: 60s

  rate_limit:
    enabled: true
    requests_per_second: 1000
    burst: 2000

database:
  postgres:
    pool:
      max_connections: 100
      min_connections: 20

nats:
  pool_size: 20
  pending_limit: 512MB
```

### System Tuning

```bash
# Increase file descriptors
cat >> /etc/security/limits.conf << 'EOF'
kscore soft nofile 65536
kscore hard nofile 65536
EOF

# Network tuning
cat >> /etc/sysctl.conf << 'EOF'
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_tw_reuse = 1
EOF

sysctl -p
```
