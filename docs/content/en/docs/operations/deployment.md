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

```mermaid
flowchart TB
    subgraph Server["Single Server"]
        subgraph KS["kscore-server"]
            API[API Server]
            SM[State Manager]
            EE[Event Engine]
            NATS[Embedded NATS]
            SQL[(SQLite Database)]
        end
    end

    Agents[Agents] <-->|NATS| KS
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

High-availability deployment with automatic failover.

### Architecture

```mermaid
flowchart TB
    subgraph LB["Load Balancer"]
        HAProxy[HAProxy / nginx / Cloud LB]
    end

    subgraph ControlPlane["Control Plane Cluster"]
        S1["Server 1<br/>(Leader)"]
        S2["Server 2<br/>(Follower)"]
        S3["Server 3<br/>(Follower)"]
    end

    subgraph NATS["NATS Cluster"]
        N1[nats-1]
        N2[nats-2]
        N3[nats-3]
    end

    subgraph DB["PostgreSQL"]
        PG1[(Primary)]
        PG2[(Replica)]
        PG3[(Replica)]
    end

    subgraph etcd["etcd Cluster"]
        E1[etcd-1]
        E2[etcd-2]
        E3[etcd-3]
    end

    Agents[Agents] <-->|NATS| NATS
    LB --> S1 & S2 & S3
    S1 & S2 & S3 <-->|NATS| NATS
    S1 & S2 & S3 -->|SQL| DB
    S1 & S2 & S3 <-->|Leader Election| etcd

    PG1 --> PG2 & PG3
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
    mode: external  # Options: embedded, external
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

### Embedded etcd Mode

For smaller HA deployments (3-5 control plane nodes), you can use embedded etcd mode which runs an etcd server in-process with each control plane node. This eliminates the need for a separate etcd cluster.

**Embedded Mode Configuration:**
```yaml
# /etc/kscore/server.yaml
clustering:
  enabled: true
  etcd:
    mode: embedded
    embedded:
      # Unique name for this node in the cluster
      name: server1
      # Directory for etcd data
      data_dir: /var/lib/kscore/etcd
      # Client listen address
      listen_client_urls: "http://0.0.0.0:2379"
      # Peer listen address
      listen_peer_urls: "http://0.0.0.0:2380"
      # Advertise client URL (reachable by other components)
      advertise_client_urls: "http://server1:2379"
      # Initial advertise peer URL (reachable by other nodes)
      initial_advertise_peer_urls: "http://server1:2380"
      # Initial cluster configuration
      initial_cluster: "server1=http://server1:2380,server2=http://server2:2380,server3=http://server3:2380"
  election:
    lease_ttl: 10s
```

**When to Use Embedded Mode:**
- 3-5 control plane nodes
- Simpler deployment without separate etcd cluster
- Development and staging environments
- Smaller production deployments

**When to Use External Mode:**
- 5+ control plane nodes
- Existing etcd infrastructure
- Need for independent etcd scaling
- Strict separation of concerns

**Mixed Deployment:**
You can start with embedded mode and migrate to external mode later by:
1. Setting up external etcd cluster
2. Exporting data from embedded etcd
3. Importing to external cluster
4. Updating configuration to `mode: external`

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

Keystone Core provides two options for Kubernetes deployment:
1. **Helm Charts** - Full-featured charts with HA support and Bitnami dependencies
2. **Raw Manifests** - Kustomize-based manifests for simple deployments

### Prerequisites

- Kubernetes 1.23+
- Helm 3.8+ (for Helm deployment)
- kubectl configured
- Persistent storage provisioner (for PostgreSQL and NATS JetStream)

### Architecture

```mermaid
flowchart TB
    subgraph K8s["Kubernetes Cluster"]
        subgraph NS["Namespace: kscore-system"]
            subgraph Server["Control Plane"]
                D1[kscore-server<br/>Deployment/StatefulSet]
                SVC1[kscore-server<br/>Service]
            end

            subgraph Agent["Agents"]
                DS[kscore-agent<br/>DaemonSet]
            end

            subgraph Deps["Dependencies (HA Mode)"]
                NATS[NATS StatefulSet<br/>3 replicas]
                PG[(PostgreSQL<br/>Primary + Replicas)]
                ETCD[etcd StatefulSet<br/>3 replicas]
            end

            CM[ConfigMaps]
            SEC[Secrets]
            PVC[(PersistentVolumeClaims)]
        end

        ING[Ingress Controller]
    end

    Users -->|HTTPS| ING --> SVC1
    D1 <--> NATS
    D1 --> PG
    D1 <--> ETCD
    DS <-->|NATS| NATS
```

### Option 1: Helm Charts (Recommended)

Keystone Core provides two Helm charts in `deploy/helm/`:
- **kscore-server** - Control plane (Deployment or StatefulSet for HA)
- **kscore-agent** - Agent DaemonSet

#### Quick Start (Single-Node)

```bash
# Install control plane with embedded NATS and SQLite
helm install kscore ./deploy/helm/kscore-server \
  --namespace kscore-system \
  --create-namespace

# Install agents
helm install kscore-agent ./deploy/helm/kscore-agent \
  --namespace kscore-system
```

#### HA Deployment with External Dependencies

```bash
# Install with HA configuration
helm install kscore ./deploy/helm/kscore-server \
  --namespace kscore-system \
  --create-namespace \
  --values ./deploy/helm/kscore-server/values-ha.yaml

# Or customize HA settings inline
helm install kscore ./deploy/helm/kscore-server \
  --namespace kscore-system \
  --create-namespace \
  --set deploymentMode=ha \
  --set replicaCount=3 \
  --set cluster.enabled=true \
  --set nats.enabled=true \
  --set postgresql.enabled=true \
  --set etcd.enabled=true
```

#### Server Chart Values

The server chart (`deploy/helm/kscore-server/values.yaml`) provides extensive configuration:

```yaml
# Deployment mode: single or ha
deploymentMode: single
replicaCount: 1

# Container image
image:
  repository: ghcr.io/keystone-core/kscore-server
  tag: "latest"
  pullPolicy: IfNotPresent

# Server configuration
server:
  grpc:
    port: 9090
  http:
    port: 8080
  metrics:
    port: 9100

# NATS configuration
nats:
  mode: embedded  # embedded or external
  embedded:
    port: 4222
    jetstream:
      enabled: true
  # For external NATS:
  # external:
  #   urls:
  #     - nats://nats-0.nats:4222
  #     - nats://nats-1.nats:4222
  #     - nats://nats-2.nats:4222

# Storage backend
storage:
  type: sqlite  # sqlite or postgresql
  sqlite:
    path: /data/state.db
  # postgresql:
  #   host: postgresql
  #   port: 5432
  #   database: kscore
  #   username: kscore

# HA Clustering (requires etcd)
cluster:
  enabled: false
  etcd:
    mode: external
    endpoints:
      - http://etcd-0.etcd:2379
      - http://etcd-1.etcd:2379
      - http://etcd-2.etcd:2379

# Resource limits
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi

# Observability
serviceMonitor:
  enabled: false
  interval: 30s

prometheusRule:
  enabled: false

grafanaDashboards:
  enabled: false

# Ingress
ingress:
  enabled: false
  className: nginx
  hosts:
    - host: kscore.example.com
      paths:
        - path: /
          pathType: Prefix
```

#### Agent Chart Values

The agent chart (`deploy/helm/kscore-agent/values.yaml`):

```yaml
image:
  repository: ghcr.io/keystone-core/kscore-agent
  tag: "latest"

# Agent configuration
agent:
  serverAddress: kscore-server.kscore-system.svc:9090
  labels:
    role: kubernetes

# Host access
hostNetwork: false
hostPID: false

# Security context
securityContext:
  privileged: false
  capabilities:
    add:
      - SYS_PTRACE

# Resources
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi

# Node selection
nodeSelector: {}
tolerations: []
affinity: {}
```

### Option 2: Raw Kubernetes Manifests

For simpler deployments without Helm, use the Kustomize-based manifests in `deploy/kubernetes/`:

```
deploy/kubernetes/
├── kustomization.yaml      # Deploy everything
├── kscore-server/          # Control plane
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── serviceaccount.yaml
│   ├── pvc.yaml
│   ├── deployment.yaml
│   └── service.yaml
└── kscore-agent/           # Agent DaemonSet
    ├── kustomization.yaml
    ├── configmap.yaml
    ├── serviceaccount.yaml
    ├── daemonset.yaml
    └── service.yaml
```

#### Deploy with Kustomize

```bash
# Deploy server and agents
kubectl apply -k deploy/kubernetes/

# Or deploy components separately
kubectl apply -k deploy/kubernetes/kscore-server/
kubectl apply -k deploy/kubernetes/kscore-agent/

# Check status
kubectl -n kscore-system get pods
```

#### Customize with Overlays

Create overlays for environment-specific configuration:

```bash
mkdir -p deploy/kubernetes/overlays/production
```

```yaml
# deploy/kubernetes/overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../

# Change image tags
images:
  - name: ghcr.io/keystone-core/kscore-server
    newTag: v0.10.0
  - name: ghcr.io/keystone-core/kscore-agent
    newTag: v0.10.0

# Change namespace
namespace: kscore-production

# Add resource limits patch
patches:
  - patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/resources/limits/memory
        value: 2Gi
    target:
      kind: Deployment
      name: kscore-server
```

```bash
# Deploy production overlay
kubectl apply -k deploy/kubernetes/overlays/production/
```

### Verification

```bash
# Check pods
kubectl get pods -n kscore-system

# Expected output (single-node):
# NAME                             READY   STATUS    RESTARTS
# kscore-server-xxxxxxxxx-xxxxx   1/1     Running   0
# kscore-agent-xxxxx              1/1     Running   0
# kscore-agent-yyyyy              1/1     Running   0

# Expected output (HA mode with dependencies):
# NAME                             READY   STATUS    RESTARTS
# kscore-server-0                  1/1     Running   0
# kscore-server-1                  1/1     Running   0
# kscore-server-2                  1/1     Running   0
# nats-0                           1/1     Running   0
# nats-1                           1/1     Running   0
# nats-2                           1/1     Running   0
# etcd-0                           1/1     Running   0
# etcd-1                           1/1     Running   0
# etcd-2                           1/1     Running   0
# postgresql-0                     1/1     Running   0
# kscore-agent-xxxxx               1/1     Running   0

# Check services
kubectl get svc -n kscore-system

# Access API via port-forward
kubectl port-forward -n kscore-system svc/kscore-server 8080:8080 9090:9090

# Check cluster status (HA mode)
kscorectl cluster status
```

### Accessing the Server

**Port Forward (Development):**
```bash
# gRPC API
kubectl -n kscore-system port-forward svc/kscore-server 9090:9090

# HTTP API
kubectl -n kscore-system port-forward svc/kscore-server 8080:8080
```

**Ingress (Production):**
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kscore-server
  namespace: kscore-system
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - kscore.example.com
      secretName: kscore-tls
  rules:
    - host: kscore.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: kscore-server
                port:
                  name: http
```

### Monitoring with Prometheus

Enable ServiceMonitor for Prometheus Operator:

```bash
# With Helm
helm upgrade kscore ./deploy/helm/kscore-server \
  --namespace kscore-system \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboards.enabled=true
```

Or apply ServiceMonitor manually:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kscore-server
  namespace: kscore-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: kscore-server
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

### Uninstall

```bash
# Helm
helm uninstall kscore -n kscore-system
helm uninstall kscore-agent -n kscore-system

# Kustomize
kubectl delete -k deploy/kubernetes/

# Delete namespace (removes all resources)
kubectl delete namespace kscore-system
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

## Sizing Guidelines for Embedded Deployments

Embedded deployments (embedded NATS + SQLite) are simpler to operate but have different sizing considerations than external clusters. This section provides detailed guidance.

### When to Use Embedded Mode

| Use Case | Embedded Mode | External Cluster |
|----------|---------------|------------------|
| Development/testing | Recommended | Overkill |
| Home lab / POC | Recommended | Optional |
| Small production (<100 agents) | Suitable | Optional |
| Medium production (100-500 agents) | Possible with tuning | Recommended |
| Large production (>500 agents) | Not recommended | Required |
| High availability required | Not suitable | Required |

### Embedded NATS Sizing

Embedded NATS runs in-process with the control plane. Memory is the primary constraint.

**Memory Components:**

| Component | Base | Per Agent | Per 1K msgs/sec |
|-----------|------|-----------|-----------------|
| Connection buffers | 10 MB | 64 KB | - |
| Subscription state | 5 MB | 1 KB | - |
| Message routing | 50 MB | - | 10 MB |
| JetStream memory | Configurable | - | - |
| JetStream file cache | - | - | ~15% of file store |

**Sizing Examples:**

| Agents | Messages/sec | JetStream Memory | JetStream File | Total Memory |
|--------|--------------|------------------|----------------|--------------|
| 25 | 50 | 256 MB | 1 GB | ~500 MB |
| 50 | 100 | 512 MB | 2 GB | ~800 MB |
| 100 | 200 | 1 GB | 5 GB | ~1.5 GB |
| 200 | 500 | 2 GB | 10 GB | ~3 GB |
| 500 | 1000 | 4 GB | 20 GB | ~6 GB |

**Configuration for Different Scales:**

**Small (up to 50 agents):**
```yaml
nats:
  mode: embedded
  jetstream:
    max_memory: 512MB
    max_file: 2GB
  max_connections: 100
  max_payload: 1MB
```

**Medium (50-200 agents):**
```yaml
nats:
  mode: embedded
  jetstream:
    max_memory: 2GB
    max_file: 10GB
  max_connections: 500
  max_payload: 1MB
  flow_control:
    enabled: true
```

**Large (200-500 agents - at the limit):**
```yaml
nats:
  mode: embedded
  jetstream:
    max_memory: 4GB
    max_file: 50GB
  max_connections: 1000
  max_payload: 1MB
  flow_control:
    enabled: true
    max_pending: 5000
```

### SQLite Sizing

SQLite is single-writer and file-based. Performance depends on disk speed and database size.

**Performance Characteristics:**

| Operation | Typical Performance | Notes |
|-----------|--------------------| ------|
| Read | 10,000+ ops/sec | Limited by disk I/O |
| Write | 100-1,000 ops/sec | Single writer, WAL mode helps |
| State apply | 10-50 resources/sec | Depends on resource complexity |
| Query | 1,000+ queries/sec | Index-dependent |

**Database Size Estimates:**

| Data Type | Size per Item | Growth Rate |
|-----------|---------------|-------------|
| Agent record | ~2 KB | Per agent |
| State resource | ~1 KB | Per resource |
| Job record | ~5 KB | Per command execution |
| Event | ~1 KB | Per event (if stored) |
| Policy | ~2 KB | Per policy |

**Sizing Examples:**

| Agents | Resources | Jobs/day | Events/day | Est. DB Size |
|--------|-----------|----------|------------|--------------|
| 25 | 500 | 100 | 5,000 | ~50 MB |
| 50 | 1,000 | 500 | 25,000 | ~200 MB |
| 100 | 2,500 | 1,000 | 100,000 | ~500 MB |
| 200 | 5,000 | 2,500 | 250,000 | ~1 GB |
| 500 | 10,000 | 5,000 | 500,000 | ~2.5 GB |

*After 30 days with retention*

**SQLite Configuration:**

```yaml
storage:
  type: sqlite
  sqlite:
    path: /var/lib/kscore/state.db
    # Performance tuning
    journal_mode: WAL          # Write-Ahead Logging (faster writes)
    busy_timeout: 5000ms       # Wait for locks instead of failing
    cache_size: 100MB          # In-memory page cache
    synchronous: NORMAL        # Balance safety and speed
```

**When SQLite Becomes a Bottleneck:**

Signs you need PostgreSQL:
- Write latency >100ms consistently
- State apply operations queuing
- Database file >10GB
- Need concurrent write operations
- Backup windows becoming too long

### Combined Resource Requirements

**System Memory Formula:**
```
Total RAM = OS Base (512 MB)
          + Control Plane Base (500 MB)
          + NATS Memory (see table)
          + SQLite Cache (100-500 MB)
          + Headroom (20%)
```

**Recommended Minimum Specifications:**

| Deployment Size | Agents | CPU | RAM | Disk (SSD) |
|-----------------|--------|-----|-----|------------|
| Minimal | 10-25 | 2 cores | 2 GB | 10 GB |
| Small | 25-50 | 2 cores | 4 GB | 20 GB |
| Medium | 50-100 | 4 cores | 8 GB | 50 GB |
| Large | 100-200 | 4 cores | 16 GB | 100 GB |
| Maximum (embedded) | 200-500 | 8 cores | 32 GB | 200 GB |

### Disk I/O Considerations

SQLite and JetStream both require fast disk I/O.

**Disk Type Recommendations:**

| Disk Type | Suitability | Notes |
|-----------|-------------|-------|
| NVMe SSD | Excellent | Best for all workloads |
| SATA SSD | Good | Suitable up to 200 agents |
| HDD | Poor | Only for dev/test |
| Network storage (NFS/EBS) | Variable | Test latency carefully |

**I/O Monitoring:**
```bash
# Check disk I/O wait
iostat -x 1

# Alert threshold: I/O wait >20% indicates disk bottleneck
```

### Memory Pressure Handling

When memory is constrained, embedded NATS and SQLite compete for resources.

**Priority Order:**
1. OS and control plane base functions
2. Active connections and subscriptions
3. JetStream memory cache
4. SQLite page cache

**Under Memory Pressure:**
- JetStream falls back to file storage (slower)
- SQLite flushes cache more frequently
- Connection buffers may be reduced

**Configuration for Memory-Constrained Environments:**
```yaml
nats:
  jetstream:
    max_memory: 256MB    # Minimal memory allocation
    max_file: 10GB       # More reliance on disk

storage:
  sqlite:
    cache_size: 50MB     # Reduced cache
    synchronous: NORMAL  # Not FULL
```

### Monitoring Embedded Deployments

**Key Metrics:**

```promql
# Memory usage
process_resident_memory_bytes{job="kscore-server"}

# SQLite operations
kscore_sqlite_operations_total{operation="write"}
kscore_sqlite_busy_timeout_total

# JetStream usage
kscore_nats_jetstream_memory_used_bytes
kscore_nats_jetstream_file_used_bytes

# Connection count
kscore_nats_connections_current
```

**Alert Thresholds:**

```yaml
alerts:
  # Memory approaching limit
  - alert: EmbeddedMemoryHigh
    expr: process_resident_memory_bytes / node_memory_MemTotal_bytes > 0.8
    for: 5m
    severity: warning

  # SQLite write latency
  - alert: SQLiteWriteSlow
    expr: histogram_quantile(0.95, kscore_sqlite_operation_duration_seconds_bucket{operation="write"}) > 0.1
    for: 5m
    severity: warning

  # JetStream at capacity
  - alert: JetStreamMemoryFull
    expr: kscore_nats_jetstream_memory_used_bytes / kscore_nats_jetstream_memory_max_bytes > 0.9
    for: 5m
    severity: warning
```

### Migration from Embedded to External

When you outgrow embedded mode, migrate to external NATS cluster and PostgreSQL.

**Migration Triggers:**
- Consistent memory pressure (>80% usage)
- SQLite write latency >100ms (p95)
- Need for high availability
- Agent count approaching 500

**Migration Path:**
1. Deploy external NATS cluster
2. Deploy PostgreSQL
3. Migrate using `kscore-migrate` tool
4. Update configuration
5. Test thoroughly
6. Switch production

See [Migration Paths](#migration-paths) for detailed procedures.

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
