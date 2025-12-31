---
title: "Deployment Guide"
weight: 1
description: >
  Production deployment patterns and strategies for Keystone Core across different environments
---

## Overview

Keystone Core supports multiple deployment patterns to match your infrastructure needs. This guide covers deployment options from simple single-node setups to highly-available production clusters across Kubernetes, VMs, and bare metal.

**Deployment Spectrum:**
- **Development** → Single-node with embedded NATS and SQLite
- **Small Production** → Multi-node with external NATS and SQLite (<100 nodes)
- **Large Production** → HA cluster with external NATS and PostgreSQL (100+ nodes)

## Single-Node Deployment

Perfect for development, testing, and small deployments (<50 managed nodes).

### Architecture

```
┌─────────────────────────────────────┐
│  Single Server                      │
│                                     │
│  ┌──────────────────────────────┐  │
│  │  kscore-server           │  │
│  │  - API Server                │  │
│  │  - State Manager             │  │
│  │  - Event Engine              │  │
│  │  - Embedded NATS             │  │
│  │  - SQLite Database           │  │
│  └──────────────────────────────┘  │
│                                     │
└─────────────────────────────────────┘
           ↑
           │ (NATS)
           ↓
    ┌─────────────┐
    │   Agents    │
    └─────────────┘
```

### Installation

**Prerequisites:**
- Linux server (Ubuntu 22.04, RHEL 8+, Debian 11+)
- 2+ CPU cores
- 4GB+ RAM
- 20GB+ disk space

**Install Binary:**
```bash
# Download latest release
wget https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-server-linux-amd64
chmod +x kscore-server-linux-amd64
sudo mv kscore-server-linux-amd64 /usr/local/bin/kscore-server

# Verify installation
kscore-server version
```

**Configuration:**
```yaml
# /etc/kscore/server.yaml
api:
  listen: "0.0.0.0:8080"

nats:
  mode: embedded  # In-process NATS
  embedded:
    port: 4222
    jetstream:
      enabled: true
      store_dir: /var/lib/kscore/jetstream

storage:
  type: sqlite
  sqlite:
    path: /var/lib/kscore/state.db

logging:
  level: info
  format: json
```

**Systemd Service:**
```ini
# /etc/systemd/system/kscore-server.service
[Unit]
Description=Keystone Core Control Plane
After=network.target

[Service]
Type=simple
User=kscore
Group=kscore
ExecStart=/usr/local/bin/kscore-server --config /etc/kscore/server.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

**Start Service:**
```bash
# Create user and directories
sudo useradd --system --no-create-home kscore
sudo mkdir -p /var/lib/kscore /etc/kscore
sudo chown kscore:kscore /var/lib/kscore

# Start service
sudo systemctl daemon-reload
sudo systemctl enable kscore-server
sudo systemctl start kscore-server

# Check status
sudo systemctl status kscore-server
```

### Limitations

- No high availability (single point of failure)
- Limited to ~50-100 managed nodes
- Not suitable for production workloads requiring 99.9%+ uptime
- SQLite performance limits state operations at scale

**Migration Path:** When you outgrow single-node, migrate to [High-Availability Setup](#high-availability).

## High-Availability Deployment

Production-ready deployment with automatic failover (99.9%+ uptime).

### Architecture

```
                    ┌─────────────────┐
                    │  Load Balancer  │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼────┐         ┌────▼────┐         ┌───▼─────┐
    │ Server1 │         │ Server2 │         │ Server3 │
    │ (Active)│         │(Standby)│         │(Standby)│
    └────┬────┘         └────┬────┘         └────┬────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                    ┌────────▼────────┐
                    │  NATS Cluster   │
                    │  (3+ nodes)     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  PostgreSQL     │
                    │  (Primary +     │
                    │   Replicas)     │
                    └─────────────────┘
```

### Prerequisites

- 3+ control plane servers (5 recommended for production)
- 3-node NATS cluster
- PostgreSQL with replication (primary + 2+ replicas)
- Load balancer (HAProxy, nginx, or cloud LB)

### NATS Cluster Setup

**Install NATS Server:**
```bash
# On each NATS node
wget https://github.com/nats-io/nats-server/releases/latest/download/nats-server-linux-amd64
chmod +x nats-server-linux-amd64
sudo mv nats-server-linux-amd64 /usr/local/bin/nats-server
```

**NATS Configuration (Node 1):**
```conf
# /etc/nats/nats-server.conf
port: 4222
server_name: nats1

cluster {
  name: kscore
  listen: 0.0.0.0:6222
  routes: [
    nats://nats1:6222
    nats://nats2:6222
    nats://nats3:6222
  ]
}

jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 8GB
  max_file_store: 100GB
}

accounts {
  KSCORE: {
    jetstream: enabled
    users: [
      {user: "kscore", password: "$NATS_PASSWORD"}
    ]
  }
}
```

**Systemd Service:**
```ini
# /etc/systemd/system/nats-server.service
[Unit]
Description=NATS Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nats-server -c /etc/nats/nats-server.conf
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Repeat for nodes 2 and 3 (change `server_name` accordingly).

### PostgreSQL Setup

**Install PostgreSQL 14+:**
```bash
sudo apt-get install postgresql-14 postgresql-14-contrib
```

**Configure Primary:**
```sql
-- Create database and user
CREATE USER kscore WITH PASSWORD 'secure_password';
CREATE DATABASE kscore OWNER keystonecore;

-- Configure replication user
CREATE USER replicator WITH REPLICATION PASSWORD 'repl_password';
```

**postgresql.conf:**
```ini
# /etc/postgresql/14/main/postgresql.conf
listen_addresses = '*'
wal_level = replica
max_wal_senders = 10
max_replication_slots = 10
hot_standby = on
```

**pg_hba.conf:**
```
# /etc/postgresql/14/main/pg_hba.conf
host    replication     replicator      10.0.0.0/8              md5
host    kscore      kscore      10.0.0.0/8              md5
```

**Set up Replicas:**
```bash
# On replica servers
pg_basebackup -h primary_host -D /var/lib/postgresql/14/main -U replicator -P --wal-method=stream
```

### Control Plane Configuration

```yaml
# /etc/kscore/server.yaml
api:
  listen: "0.0.0.0:8080"

nats:
  mode: external
  external:
    urls:
      - "nats://nats1:4222"
      - "nats://nats2:4222"
      - "nats://nats3:4222"
    credentials:
      username: "kscore"
      password: "$NATS_PASSWORD"

storage:
  type: postgresql
  postgresql:
    host: "postgres-primary"
    port: 5432
    database: "kscore"
    username: "kscore"
    password: "$POSTGRES_PASSWORD"
    pool:
      max_connections: 50
      max_idle: 10

clustering:
  enabled: true
  etcd:
    endpoints:
      - "http://etcd1:2379"
      - "http://etcd2:2379"
      - "http://etcd3:2379"
  election:
    lease_ttl: 10s

logging:
  level: info
  format: json
```

### Agent Persistence and Handoff

In HA deployments, agents may connect to different control plane servers over time (due to load balancing or failover). Keystone Core handles this automatically:

**How It Works:**
1. **Startup Loading**: Each control plane server loads all registered agents from the shared PostgreSQL database on startup
2. **Dynamic Discovery**: When a heartbeat arrives from an agent not in memory, the server checks the database before rejecting it
3. **Seamless Handoff**: If the agent exists in the database, it's loaded and heartbeat processing continues normally

**Benefits:**
- Zero agent re-registration during failover
- Agents can connect to any control plane server
- Load balancer can route agents freely without session affinity
- New control plane servers immediately recognize all existing agents

**Requirements:**
- Shared PostgreSQL database (required for HA)
- All control plane servers must use the same database

This ensures true high-availability with no agent disruption when control plane servers fail or during rolling updates.

### Load Balancer Setup

**HAProxy Configuration:**
```
# /etc/haproxy/haproxy.cfg
frontend kscore_api
    bind *:8080
    mode http
    default_backend kscore_servers

backend kscore_servers
    mode http
    balance roundrobin
    option httpchk GET /health/ready

    server server1 10.0.1.10:8080 check
    server server2 10.0.1.11:8080 check
    server server3 10.0.1.12:8080 check
```

### Verification

```bash
# Check cluster health
kscorectl cluster status

# Expected output:
# NODE      STATUS    ROLE      UPTIME
# server1   healthy   leader    5d2h
# server2   healthy   follower  5d2h
# server3   healthy   follower  5d2h

# Check NATS cluster
nats-server --routes_check

# Check PostgreSQL replication
psql -U kscore -c "SELECT * FROM pg_stat_replication;"
```

## Kubernetes Deployment

Native Kubernetes deployment with Helm charts.

### Prerequisites

- Kubernetes 1.23+
- Helm 3.8+
- kubectl configured
- Persistent storage provisioner (for PostgreSQL and NATS JetStream)

### Architecture

```
Kubernetes Cluster
├── Namespace: kscore
├── Deployment: kscore-server (3 replicas)
├── StatefulSet: nats (3 replicas)
├── StatefulSet: postgresql (1 primary + 2 replicas)
├── Service: kscore-api (LoadBalancer)
├── Service: nats (ClusterIP)
├── Service: postgresql (ClusterIP)
├── ConfigMap: server-config
└── Secret: credentials
```

### Installation with Helm

**Add Helm Repository:**
```bash
helm repo add keystonecore https://charts.kscore.io
helm repo update
```

**Create Namespace:**
```bash
kubectl create namespace kscore
```

**Install Chart:**
```bash
helm install keystonecore keystonecore/kscore \
  --namespace kscore \
  --set server.replicas=3 \
  --set nats.cluster.enabled=true \
  --set nats.cluster.replicas=3 \
  --set postgresql.enabled=true \
  --set postgresql.replication.enabled=true \
  --set postgresql.replication.readReplicas=2
```

**Custom Values (values.yaml):**
```yaml
server:
  replicas: 3
  image:
    repository: kscore/server
    tag: "v1.0.0"

  resources:
    requests:
      cpu: "1000m"
      memory: "2Gi"
    limits:
      cpu: "2000m"
      memory: "4Gi"

  persistence:
    enabled: false  # Using external PostgreSQL

nats:
  cluster:
    enabled: true
    replicas: 3

  jetstream:
    enabled: true
    storage:
      size: 10Gi
      storageClass: "fast-ssd"

postgresql:
  enabled: true
  auth:
    username: kscore
    password: "" # Set via secret
    database: kscore

  primary:
    resources:
      requests:
        cpu: "1000m"
        memory: "4Gi"
    persistence:
      size: 50Gi
      storageClass: "fast-ssd"

  replication:
    enabled: true
    readReplicas: 2

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: kscore.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: kscore-tls
      hosts:
        - kscore.example.com
```

**Install with Custom Values:**
```bash
helm install keystonecore keystonecore/kscore \
  --namespace kscore \
  --values values.yaml
```

### Deploy Agents as DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kscore-agent
  namespace: kscore
spec:
  selector:
    matchLabels:
      app: kscore-agent
  template:
    metadata:
      labels:
        app: kscore-agent
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: agent
        image: kscore/agent:v1.0.0
        env:
        - name: KSCORE_SERVER_URL
          value: "nats://kscore-nats:4222"
        - name: KSCORE_AGENT_ID
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: host-root
          mountPath: /host
          readOnly: true
        securityContext:
          privileged: true
      volumes:
      - name: host-root
        hostPath:
          path: /
```

### Verification

```bash
# Check pods
kubectl get pods -n kscore

# Expected output:
# NAME                                 READY   STATUS    RESTARTS
# kscore-server-0                  1/1     Running   0
# kscore-server-1                  1/1     Running   0
# kscore-server-2                  1/1     Running   0
# nats-0                               1/1     Running   0
# nats-1                               1/1     Running   0
# nats-2                               1/1     Running   0
# postgresql-0                         1/1     Running   0
# postgresql-read-0                    1/1     Running   0
# postgresql-read-1                    1/1     Running   0
# kscore-agent-xxxxx               1/1     Running   0

# Check services
kubectl get svc -n kscore

# Access API
kubectl port-forward -n kscore svc/kscore-api 8080:8080
```

## Docker Compose Deployment

Quick containerized setup for development and testing.

### Docker Compose File

```yaml
# docker-compose.yml
version: '3.8'

services:
  nats:
    image: nats:2.10-alpine
    command:
      - "--cluster_name=kscore"
      - "--jetstream"
      - "--store_dir=/data/jetstream"
    ports:
      - "4222:4222"
      - "6222:6222"
      - "8222:8222"
    volumes:
      - nats-data:/data
    healthcheck:
      test: ["CMD", "nats-server", "--signal", "check"]
      interval: 10s
      timeout: 5s
      retries: 3

  postgres:
    image: postgres:14-alpine
    environment:
      POSTGRES_DB: kscore
      POSTGRES_USER: kscore
      POSTGRES_PASSWORD: password
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kscore"]
      interval: 10s
      timeout: 5s
      retries: 3

  server:
    image: kscore/server:latest
    depends_on:
      nats:
        condition: service_healthy
      postgres:
        condition: service_healthy
    ports:
      - "8080:8080"
    environment:
      KSCORE_NATS_URL: "nats://nats:4222"
      KSCORE_STORAGE_TYPE: postgresql
      KSCORE_POSTGRES_HOST: postgres
      KSCORE_POSTGRES_USER: kscore
      KSCORE_POSTGRES_PASSWORD: password
    volumes:
      - ./config:/etc/kscore
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - "--config.file=/etc/prometheus/prometheus.yml"
      - "--storage.tsdb.path=/prometheus"

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./grafana/datasources:/etc/grafana/provisioning/datasources
    depends_on:
      - prometheus

volumes:
  nats-data:
  postgres-data:
  prometheus-data:
  grafana-data:
```

### Start Services

```bash
# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f server

# Stop services
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

## Scaling Strategies

### Horizontal Scaling (Adding Nodes)

**Control Plane Scaling:**
```bash
# Add new control plane node
# 1. Install kscore-server
# 2. Use same configuration (NATS, PostgreSQL endpoints)
# 3. Enable clustering with etcd
# 4. Add to load balancer pool

# Nodes will automatically elect leader and distribute work
```

**Agent Scaling:**
- Agents are stateless - add as many as needed
- No configuration changes required
- Agents automatically discover control plane via NATS

**NATS Scaling:**
- Always use odd numbers (3, 5, 7) for cluster consensus
- 3 nodes sufficient for most workloads
- 5+ nodes for geo-distributed deployments

**Database Scaling:**
- Add read replicas for query load
- Shard by datacenter/region for geo-distribution
- Connection pooling at application layer

### Vertical Scaling (Resource Increases)

**CPU Scaling:**
- Control plane: 2-4 cores typical, 8+ for high throughput
- NATS: 4+ cores for message routing
- PostgreSQL: 4-8 cores for query processing

**Memory Scaling:**
- Control plane: 4GB minimum, 8-16GB typical, 32GB+ for large deployments
- NATS JetStream: 8GB+ for message buffering
- PostgreSQL: 8-32GB+ depending on state size

**Disk Scaling:**
- JetStream: 100GB+ for event storage (depends on retention)
- PostgreSQL: 50GB+ (grows with managed nodes and state history)
- Use SSD/NVMe for best performance

### Scaling Thresholds

| Metric | Single-Node | Multi-Node | HA Cluster |
|--------|-------------|------------|------------|
| Managed Nodes | <50 | 50-500 | 500+ |
| Commands/sec | <10 | 10-100 | 100+ |
| State Resources | <1,000 | 1K-10K | 10K+ |
| Events/sec | <100 | 100-1,000 | 1,000+ |
| API Requests/sec | <50 | 50-500 | 500+ |

## Migration Paths

### Embedded NATS → External NATS Cluster

**1. Set up NATS cluster** (see [NATS Cluster Setup](#nats-cluster-setup))

**2. Update control plane configuration:**
```yaml
nats:
  mode: external
  external:
    urls:
      - "nats://nats1:4222"
      - "nats://nats2:4222"
      - "nats://nats3:4222"
```

**3. Migrate JetStream data** (if needed):
```bash
# Backup embedded JetStream
nats-server --signal backup --jetstream /var/lib/kscore/jetstream

# Restore to cluster
nats stream restore --dir /backup/jetstream
```

**4. Restart control plane:**
```bash
sudo systemctl restart kscore-server
```

**5. Verify agents reconnect:**
```bash
kscorectl agent list
```

### SQLite → PostgreSQL Migration

**1. Set up PostgreSQL** (see [PostgreSQL Setup](#postgresql-setup))

**2. Export SQLite data:**
```bash
kscore-migrate export \
  --source sqlite:///var/lib/kscore/state.db \
  --output /tmp/state-export.sql
```

**3. Import to PostgreSQL:**
```bash
kscore-migrate import \
  --input /tmp/state-export.sql \
  --target postgres://kscore:password@localhost/kscore
```

**4. Update configuration:**
```yaml
storage:
  type: postgresql
  postgresql:
    host: "localhost"
    database: "kscore"
    username: "kscore"
    password: "$POSTGRES_PASSWORD"
```

**5. Restart and verify:**
```bash
sudo systemctl restart kscore-server
kscorectl state list  # Verify state is accessible
```

## Best Practices

### Deployment
- **Start small, scale up** - Begin with single-node, migrate to HA as you grow
- **Use external NATS** for production (>100 managed nodes)
- **Use PostgreSQL** for production (>100 managed nodes or high state change rate)
- **Enable clustering** for high availability (3+ control plane nodes)
- **Deploy agents near control plane** - Same datacenter for <10ms latency

### Networking
- **Use private networks** - Control plane and NATS should not be public
- **Enable TLS** - Encrypt all traffic (NATS, API, PostgreSQL)
- **Configure firewalls** - Restrict access to necessary ports only
- **Use load balancers** - Distribute API load across control plane nodes

### Storage
- **SSD/NVMe required** for PostgreSQL and JetStream
- **RAID 10 recommended** for redundancy and performance
- **Monitor disk usage** - Set up alerts for 80%+ utilization
- **Plan for growth** - 20% annual growth in state and events

### Capacity Planning
- **Control plane**: 1 CPU core per 100 agents, 1GB RAM per 100 agents
- **NATS**: 1 CPU core per 1,000 msgs/sec, 1GB RAM per 10,000 buffered messages
- **PostgreSQL**: 1GB RAM per 10,000 state resources
- **Network**: 1Mbps per 100 agents (typical, spiky during state application)

## Troubleshooting Deployments

### Control Plane Won't Start

**Check logs:**
```bash
sudo journalctl -u kscore-server -f
```

**Common issues:**
- Configuration syntax errors - validate YAML
- Cannot connect to NATS - verify NATS is running and credentials correct
- Cannot connect to database - verify PostgreSQL is running and accessible
- Port already in use - check if another instance is running

### Agents Won't Connect

**Check agent logs:**
```bash
sudo journalctl -u kscore-agent -f
```

**Common issues:**
- Cannot resolve NATS host - verify DNS or use IP address
- Authentication failed - check NATS credentials
- Network unreachable - verify firewall rules and routing
- TLS certificate errors - verify certificates are valid

### Cluster Election Fails

**Check etcd health:**
```bash
etcdctl endpoint health
```

**Common issues:**
- Less than 3 etcd nodes - need quorum for elections
- Network partitions - verify connectivity between nodes
- Clock skew - synchronize clocks with NTP

### Performance Issues

**Check resource utilization:**
```bash
# CPU and memory
top

# Disk I/O
iostat -x 1

# Network
iftop
```

**Common bottlenecks:**
- Insufficient CPU - scale vertically or horizontally
- Memory swapping - increase RAM or reduce memory usage
- Disk I/O saturation - use faster disks or add caching
- Network congestion - increase bandwidth or optimize traffic

## See Also

- [Monitoring Guide](monitoring/) - Set up observability
- [Maintenance Guide](maintenance/) - Backup and upgrade procedures
- [Security Guide](security/) - Harden your deployment
- [Configuration Reference](/docs/reference/configuration/) - All configuration options
