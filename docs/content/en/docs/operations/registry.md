---
title: "Module Registry"
weight: 7
description: >
  Deploy and operate a scalable, redundant module registry for Keystone Core
---

## Overview

The Keystone Core Module Registry (`kscore-registry`) provides module distribution for your organization. This guide covers production deployment patterns from single-node to highly available multi-region setups.

## Architecture

```mermaid
flowchart TB
    LB["Load Balancer<br/>(HAProxy / Nginx / Cloud LB)"]
    Primary["kscore-registry<br/>(Primary)<br/>Read/Write"]
    Replica["kscore-registry<br/>(Replica)<br/>Read-Only"]
    Storage[("Shared Storage<br/>(NFS / EFS / GCS / S3 / Azure Files)")]

    LB --> Primary
    LB --> Replica
    Primary --> Storage
    Replica --> Storage
```

## Deployment Options

### Single-Node Deployment

Suitable for development, testing, and small organizations (<100 users).

**Docker**:

```bash
docker run -d \
  --name kscore-registry \
  -p 8090:8090 \
  -v /var/lib/keystone-core/modules:/data \
  -e KSCORE_REGISTRY_API_KEY="${REGISTRY_API_KEY}" \
  keystonecore/kscore-registry:latest
```

**Binary**:

```bash
kscore-registry \
  --data /var/lib/keystone-core/modules \
  --listen :8090 \
  --api-key "${REGISTRY_API_KEY}"
```

**systemd Service**:

```ini
# /etc/systemd/system/kscore-registry.service
[Unit]
Description=Keystone Core Module Registry
After=network.target

[Service]
Type=simple
User=kscore
Group=kscore
ExecStart=/usr/local/bin/kscore-registry \
  --data /var/lib/keystone-core/modules \
  --listen :8090
Environment=KSCORE_REGISTRY_API_KEY=your-secret-key
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### High Availability Deployment

For production environments requiring redundancy and zero-downtime updates.

#### Architecture Components

1. **Load Balancer**: Distributes traffic across registry instances
2. **Primary Registry**: Handles read and write operations
3. **Replica Registries**: Read-only mirrors for high availability
4. **Shared Storage**: Network filesystem accessible by all instances

#### Docker Compose (Development HA)

```yaml
# docker-compose.yml
version: '3.8'

services:
  lb:
    image: haproxy:2.8
    ports:
      - "8090:8090"
    volumes:
      - ./haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro
    depends_on:
      - registry-primary
      - registry-replica-1
      - registry-replica-2

  registry-primary:
    image: keystonecore/kscore-registry:latest
    environment:
      - KSCORE_REGISTRY_API_KEY=${REGISTRY_API_KEY}
    volumes:
      - registry-data:/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  registry-replica-1:
    image: keystonecore/kscore-registry:latest
    command: ["--readonly"]
    volumes:
      - registry-data:/data:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  registry-replica-2:
    image: keystonecore/kscore-registry:latest
    command: ["--readonly"]
    volumes:
      - registry-data:/data:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
      interval: 10s
      timeout: 5s
      retries: 3

volumes:
  registry-data:
```

**HAProxy Configuration**:

```
# haproxy.cfg
global
    daemon
    maxconn 4096

defaults
    mode http
    timeout connect 5s
    timeout client 30s
    timeout server 30s
    option httpchk GET /health

frontend registry_frontend
    bind *:8090
    default_backend registry_backend

backend registry_backend
    balance roundrobin
    option httpchk GET /health

    # Primary handles writes, all handle reads
    acl is_write method POST DELETE

    # Write requests go only to primary
    use-server primary if is_write

    # Read requests distributed across all
    server primary registry-primary:8090 check
    server replica1 registry-replica-1:8090 check backup
    server replica2 registry-replica-2:8090 check backup
```

### Kubernetes Deployment

For cloud-native production environments.

#### StatefulSet (Primary)

```yaml
# registry-primary.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kscore-registry-primary
  namespace: kscore-system
spec:
  serviceName: kscore-registry-primary
  replicas: 1
  selector:
    matchLabels:
      app: kscore-registry
      role: primary
  template:
    metadata:
      labels:
        app: kscore-registry
        role: primary
    spec:
      containers:
      - name: registry
        image: keystonecore/kscore-registry:latest
        ports:
        - containerPort: 8090
          name: http
        env:
        - name: KSCORE_REGISTRY_API_KEY
          valueFrom:
            secretKeyRef:
              name: kscore-registry-secrets
              key: api-key
        args:
        - --data=/data
        - --listen=:8090
        volumeMounts:
        - name: data
          mountPath: /data
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast-ssd
      resources:
        requests:
          storage: 50Gi
```

#### Deployment (Read Replicas)

```yaml
# registry-replicas.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kscore-registry-replicas
  namespace: kscore-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: kscore-registry
      role: replica
  template:
    metadata:
      labels:
        app: kscore-registry
        role: replica
    spec:
      containers:
      - name: registry
        image: keystonecore/kscore-registry:latest
        ports:
        - containerPort: 8090
          name: http
        args:
        - --data=/data
        - --listen=:8090
        - --readonly
        volumeMounts:
        - name: data
          mountPath: /data
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 256Mi
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: kscore-registry-data
          readOnly: true
```

#### Services

```yaml
# registry-services.yaml
apiVersion: v1
kind: Service
metadata:
  name: kscore-registry
  namespace: kscore-system
spec:
  type: ClusterIP
  ports:
  - port: 8090
    targetPort: http
    name: http
  selector:
    app: kscore-registry
---
apiVersion: v1
kind: Service
metadata:
  name: kscore-registry-primary
  namespace: kscore-system
spec:
  type: ClusterIP
  ports:
  - port: 8090
    targetPort: http
    name: http
  selector:
    app: kscore-registry
    role: primary
```

#### Ingress

```yaml
# registry-ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kscore-registry
  namespace: kscore-system
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - registry.example.com
    secretName: kscore-registry-tls
  rules:
  - host: registry.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: kscore-registry
            port:
              number: 8090
```

### Multi-Region Deployment

For global organizations requiring low-latency access across regions.

```mermaid
flowchart TB
    DNS["Global DNS (GeoDNS)<br/>registry.example.com"]

    subgraph US ["US-East Region"]
        USReg["kscore-registry<br/>(Primary Write)"]
    end

    subgraph EU ["EU-West Region"]
        EUReg["kscore-registry<br/>(Read Replica)"]
    end

    subgraph APAC ["APAC Region"]
        APACReg["kscore-registry<br/>(Read Replica)"]
    end

    Replication["Replication<br/>(rsync / S3 Cross-Region)"]

    DNS --> USReg
    DNS --> EUReg
    DNS --> APACReg

    USReg --> Replication
    Replication --> EUReg
    Replication --> APACReg
```

#### Replication Script

```bash
#!/bin/bash
# sync-registry.sh - Run on primary, syncs to replicas

REGIONS=(
  "us-east-1:s3://kscore-registry-us-east-1"
  "eu-west-1:s3://kscore-registry-eu-west-1"
  "ap-southeast-1:s3://kscore-registry-ap-southeast-1"
)

SOURCE_DIR="/var/lib/keystone-core/modules"

for region_bucket in "${REGIONS[@]}"; do
  region="${region_bucket%%:*}"
  bucket="${region_bucket#*:}"

  echo "Syncing to ${region}..."
  aws s3 sync "${SOURCE_DIR}" "${bucket}" \
    --region "${region}" \
    --delete \
    --exclude ".tmp/*"
done
```

#### AWS S3 Cross-Region Replication

```hcl
# terraform/s3-replication.tf
resource "aws_s3_bucket" "registry_primary" {
  bucket = "kscore-registry-us-east-1"

  versioning {
    enabled = true
  }
}

resource "aws_s3_bucket_replication_configuration" "registry" {
  bucket = aws_s3_bucket.registry_primary.id
  role   = aws_iam_role.replication.arn

  rule {
    id     = "replicate-to-eu"
    status = "Enabled"

    destination {
      bucket        = aws_s3_bucket.registry_eu.arn
      storage_class = "STANDARD"
    }
  }

  rule {
    id     = "replicate-to-apac"
    status = "Enabled"

    destination {
      bucket        = aws_s3_bucket.registry_apac.arn
      storage_class = "STANDARD"
    }
  }
}
```

## Storage Backends

The registry supports pluggable storage backends. Use `--storage-backend` to select a backend. The default is `filesystem` (local disk).

| Backend | Flag Value | Use Case |
|---------|-----------|----------|
| Filesystem | `filesystem` | Single-node, development, small deployments |
| AWS S3 | `s3` | Production on AWS, multi-region replication |
| Google Cloud Storage | `gcs` | Production on GCP |
| Azure Blob Storage | `azure` | Production on Azure |
| NATS Object Store | `nats` | NATS-native deployments |

### Filesystem (Default)

Stores modules on local disk. Suitable for single-node deployments or when using a shared filesystem (NFS/EFS).

```bash
kscore-registry --storage-backend filesystem --data /var/lib/keystone-core/modules
```

### AWS S3

Stores modules in an S3 bucket. Supports S3-compatible services (MinIO, DigitalOcean Spaces, etc.) via `--s3-endpoint`.

```bash
kscore-registry --storage-backend s3 \
  --s3-bucket kscore-registry \
  --s3-region us-east-1 \
  --s3-prefix modules/

# S3-compatible (MinIO)
kscore-registry --storage-backend s3 \
  --s3-bucket kscore-registry \
  --s3-endpoint http://minio:9000 \
  --s3-path-style
```

**S3 Flags**:

| Flag | Description |
|------|-------------|
| `--s3-bucket` | S3 bucket name (required) |
| `--s3-region` | AWS region (required unless using `--s3-endpoint`) |
| `--s3-endpoint` | Custom S3 endpoint URL |
| `--s3-prefix` | Key prefix within the bucket |
| `--s3-path-style` | Use path-style addressing (required for MinIO) |

### Google Cloud Storage

```bash
kscore-registry --storage-backend gcs \
  --gcs-bucket kscore-registry \
  --gcs-project my-project \
  --gcs-credentials-file /path/to/credentials.json
```

**GCS Flags**:

| Flag | Description |
|------|-------------|
| `--gcs-bucket` | GCS bucket name (required) |
| `--gcs-project` | GCP project ID |
| `--gcs-credentials-file` | Path to service account JSON key |
| `--gcs-prefix` | Object prefix within the bucket |

### Azure Blob Storage

```bash
kscore-registry --storage-backend azure \
  --azure-container kscore-registry \
  --azure-account-name mystorageaccount

# Or with connection string
kscore-registry --storage-backend azure \
  --azure-container kscore-registry \
  --azure-connection-string "DefaultEndpointsProtocol=https;..."
```

**Azure Flags**:

| Flag | Description |
|------|-------------|
| `--azure-container` | Blob container name (required) |
| `--azure-account-name` | Storage account name |
| `--azure-connection-string` | Full connection string (alternative to account name) |
| `--azure-prefix` | Blob prefix within the container |

### NATS Object Store

For deployments already running NATS JetStream.

```bash
kscore-registry --storage-backend nats \
  --nats-url nats://nats:4222 \
  --nats-bucket kscore-registry \
  --nats-credentials /path/to/nats.creds
```

**NATS Flags**:

| Flag | Description |
|------|-------------|
| `--nats-url` | NATS server URL (required) |
| `--nats-bucket` | Object store bucket name (required) |
| `--nats-credentials` | Path to NATS credentials file |
| `--nats-prefix` | Key prefix within the bucket |

### Migrating Between Backends

Use the `migrate-storage` subcommand to copy all modules from one backend to another:

```bash
# Migrate from filesystem to S3
kscore-registry migrate-storage \
  --data /var/lib/keystone-core/modules \
  --dest-backend s3 \
  --dest-s3-bucket kscore-registry \
  --dest-s3-region us-east-1

# Dry run (preview what would be migrated)
kscore-registry migrate-storage \
  --data /var/lib/keystone-core/modules \
  --dest-backend s3 \
  --dest-s3-bucket kscore-registry \
  --dest-s3-region us-east-1 \
  --dry-run
```

The migration verifies hash integrity for each module version after copying. The source backend is not modified.

### Kubernetes with Cloud Backends

With native cloud storage backends, you no longer need shared filesystems (NFS/EFS) for multi-replica deployments:

```yaml
# registry-deployment.yaml — multiple replicas with S3 backend
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kscore-registry
  namespace: kscore-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: kscore-registry
  template:
    metadata:
      labels:
        app: kscore-registry
    spec:
      containers:
      - name: registry
        image: keystonecore/kscore-registry:latest
        ports:
        - containerPort: 8090
          name: http
        args:
        - --storage-backend=s3
        - --s3-bucket=$(S3_BUCKET)
        - --s3-region=$(AWS_REGION)
        - --listen=:8090
        env:
        - name: KSCORE_REGISTRY_API_KEY
          valueFrom:
            secretKeyRef:
              name: kscore-registry-secrets
              key: api-key
        - name: S3_BUCKET
          value: kscore-registry
        - name: AWS_REGION
          value: us-east-1
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

## Security

### TLS Termination

Always use TLS in production. Configure at the load balancer or ingress:

**Nginx**:

```nginx
server {
    listen 443 ssl http2;
    server_name registry.example.com;

    ssl_certificate /etc/ssl/registry.crt;
    ssl_certificate_key /etc/ssl/registry.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;

    location / {
        proxy_pass http://registry-backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### API Key Rotation

Rotate API keys regularly without downtime:

1. Generate new API key
2. Update registry with both old and new keys (comma-separated)
3. Update all clients to use new key
4. Remove old key from registry

```bash
# Support multiple API keys (future enhancement)
export KSCORE_REGISTRY_API_KEY="new-key,old-key"
```

### Network Policies (Kubernetes)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kscore-registry
  namespace: kscore-system
spec:
  podSelector:
    matchLabels:
      app: kscore-registry
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: kscore-system
    - namespaceSelector:
        matchLabels:
          kscore-registry-access: "true"
    ports:
    - protocol: TCP
      port: 8090
  egress:
  - to:
    - ipBlock:
        cidr: 10.0.0.0/8  # Internal network only
```

## Monitoring

> **Note:** `kscore-registry` does not currently expose a `/metrics` endpoint or Prometheus-format metrics. Monitor the registry using its `/health` endpoint and external tools.

### Health Check Monitoring

Use [blackbox_exporter](https://github.com/prometheus/blackbox_exporter) to probe the `/health` endpoint:

```yaml
scrape_configs:
  - job_name: 'kscore-registry-health'
    metrics_path: /probe
    params:
      module: [http_2xx]
    static_configs:
      - targets:
          - http://registry.example.com:8090/health
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: blackbox-exporter:9115
```

### Alerts

```yaml
groups:
- name: kscore-registry
  rules:
  - alert: RegistryDown
    expr: probe_success{job="kscore-registry-health"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Module registry is down"
      description: "{{ $labels.instance }} has been unreachable for 1 minute."

  - alert: RegistryDiskSpaceLow
    expr: node_filesystem_avail_bytes{mountpoint="/data"} / node_filesystem_size_bytes{mountpoint="/data"} < 0.1
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Registry disk space low"
      description: "Less than 10% disk space remaining."
```

## Backup and Recovery

### Backup Strategy

```bash
#!/bin/bash
# backup-registry.sh

BACKUP_DIR="/backups/kscore-registry"
DATA_DIR="/var/lib/keystone-core/modules"
DATE=$(date +%Y%m%d-%H%M%S)

# Create backup
tar -czf "${BACKUP_DIR}/registry-${DATE}.tar.gz" -C "${DATA_DIR}" .

# Upload to S3
aws s3 cp "${BACKUP_DIR}/registry-${DATE}.tar.gz" \
  s3://kscore-backups/registry/

# Retain last 30 days
find "${BACKUP_DIR}" -name "registry-*.tar.gz" -mtime +30 -delete
```

### Recovery

```bash
#!/bin/bash
# restore-registry.sh

BACKUP_FILE="$1"
DATA_DIR="/var/lib/keystone-core/modules"

# Stop registry
systemctl stop kscore-registry

# Restore
tar -xzf "${BACKUP_FILE}" -C "${DATA_DIR}"

# Fix permissions
chown -R kscore:kscore "${DATA_DIR}"

# Start registry
systemctl start kscore-registry
```

## Scaling Guidelines

| Users | Modules | Recommended Setup |
|-------|---------|-------------------|
| < 50 | < 100 | Single node |
| 50-200 | 100-500 | 1 primary + 2 replicas |
| 200-1000 | 500-2000 | 1 primary + 3-5 replicas with LB |
| > 1000 | > 2000 | Multi-region with CDN |

### Resource Sizing

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| Primary | 500m-1 core | 512MB-1GB | 50-500GB SSD |
| Replica | 200m-500m | 256MB-512MB | Shared storage |
| Load Balancer | 100m | 128MB | - |

## Troubleshooting

### Common Issues

**Registry won't start**:

```bash
# Check data directory permissions
ls -la /var/lib/keystone-core/modules

# Fix permissions
chown -R kscore:kscore /var/lib/keystone-core/modules
chmod 755 /var/lib/keystone-core/modules
```

**Publish fails with 403**:

- Verify API key is set correctly
- Check if registry is in read-only mode

**Module download slow**:

- Check network between client and registry
- Consider deploying regional replicas
- Enable CDN caching for static assets

**Disk space issues**:

```bash
# Find large modules
du -sh /var/lib/keystone-core/modules/*/*/* | sort -rh | head -20

# Clean old versions (keep last 3)
find /var/lib/keystone-core/modules -type d -name "[0-9]*" | \
  while read dir; do
    parent=$(dirname "$dir")
    ls -1v "$parent" | head -n -3 | \
      xargs -I {} rm -rf "$parent/{}"
  done
```

## See Also

- [CLI Reference: kscore-registry](/docs/reference/cli/#kscore-registry-module-registry-server)
- [CLI Reference: kscore-module](/docs/reference/cli/#kscore-module-module-management)
- [Deployment Guide](/docs/operations/deployment/)
- [Security Guide](/docs/operations/security/)
