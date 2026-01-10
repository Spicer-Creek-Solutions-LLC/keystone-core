---
title: "File Distribution Operations"
linkTitle: "File Distribution"
weight: 22
description: >
  Operating the Keystone Core file distribution system including backend configuration, mirror groups, and troubleshooting.
---

## Overview

The file distribution system (`kscore-files`) provides centralized file management and distribution across your infrastructure. This guide covers operational procedures for:

- **Backend Configuration**: Setting up storage backends (local, S3, GCS, Azure, Git, NATS)
- **Mirror Groups**: Geographic routing and high availability
- **Namespace Management**: Access control and organization
- **Monitoring**: Metrics and health checks
- **Troubleshooting**: Common issues and solutions

## Deployment

### Single-Node Deployment

For development and small deployments, use the local filesystem backend:

```bash
# Start file server with local backend
kscore-files serve --config /etc/kscore/files.yaml
```

**Configuration (`/etc/kscore/files.yaml`):**

```yaml
server:
  listen: :8081
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/server.crt
    key_file: /etc/kscore/certs/server.key

nats:
  url: nats://localhost:4222

backend:
  type: local
  local:
    root: /var/lib/kscore/files
    temp_dir: /var/lib/kscore/tmp

access_control:
  default_policy: deny
  acls:
    - namespace: "*"
      principals: ["role:admin"]
      permissions: ["read", "write", "delete", "admin"]
```

### Production Deployment with S3

For production, use a cloud storage backend with mirror groups:

```yaml
server:
  listen: :8081
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/server.crt
    key_file: /etc/kscore/certs/server.key

nats:
  urls:
    - nats://nats1:4222
    - nats://nats2:4222
    - nats://nats3:4222

backend:
  type: s3
  s3:
    bucket: kscore-files-prod
    region: us-west-2
    prefix: files/
    # Use IAM roles in production (no explicit credentials)
    server_side_encryption: aws:kms
    kms_key_id: alias/kscore-files

access_control:
  default_policy: deny
  acls:
    - namespace: "prod/*"
      principals: ["role:ops", "role:sre"]
      permissions: ["read", "write"]
    - namespace: "dev/*"
      principals: ["role:developer"]
      permissions: ["read", "write", "delete"]

rate_limit:
  requests_per_second: 1000
  burst: 100
```

### Kubernetes Deployment

Deploy as a StatefulSet for persistence:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kscore-files
spec:
  serviceName: kscore-files
  replicas: 3
  selector:
    matchLabels:
      app: kscore-files
  template:
    metadata:
      labels:
        app: kscore-files
    spec:
      serviceAccountName: kscore-files
      containers:
        - name: kscore-files
          image: kscore/kscore-files:latest
          ports:
            - containerPort: 8081
              name: https
            - containerPort: 9090
              name: metrics
          env:
            - name: KSCORE_FILES_BACKEND_TYPE
              value: s3
            - name: KSCORE_FILES_S3_BUCKET
              value: kscore-files-prod
            - name: KSCORE_FILES_S3_REGION
              value: us-west-2
          volumeMounts:
            - name: config
              mountPath: /etc/kscore
            - name: tls
              mountPath: /etc/kscore/certs
      volumes:
        - name: config
          configMap:
            name: kscore-files-config
        - name: tls
          secret:
            secretName: kscore-files-tls
---
apiVersion: v1
kind: Service
metadata:
  name: kscore-files
spec:
  selector:
    app: kscore-files
  ports:
    - port: 8081
      name: https
    - port: 9090
      name: metrics
```

## Backend Operations

### Switching Backends

To migrate from one backend to another:

```bash
# 1. Export all files from current backend
kscore-files export --output /tmp/files-backup

# 2. Update configuration with new backend
vim /etc/kscore/files.yaml

# 3. Import files to new backend
kscore-files import --input /tmp/files-backup

# 4. Restart file server
systemctl restart kscore-files

# 5. Verify files
kscore-files ls --recursive /
```

### Backend Health Check

```bash
# Check backend status
kscore-files backend status

# Output:
# Backend: s3
# Status: healthy
# Bucket: kscore-files-prod
# Region: us-west-2
# Objects: 15,432
# Total Size: 2.3 GB
# Last Check: 2024-01-15T10:23:45Z
```

### Garbage Collection

Remove orphaned content-addressed files:

```bash
# Dry run - show what would be deleted
kscore-files backend gc --dry-run

# Execute garbage collection
kscore-files backend gc

# Schedule regular GC (cron)
0 3 * * * /usr/bin/kscore-files backend gc --quiet
```

## Mirror Group Operations

### Creating Mirror Groups

Configure mirror groups for geographic distribution:

```yaml
# /etc/kscore/files.yaml
mirror_groups:
  - id: us-mirrors
    name: US Region Mirrors
    mirrors:
      - id: us-west
        cluster_id: cluster-west
        location: "37.7749,-122.4194"  # San Francisco
        is_primary: true
      - id: us-east
        cluster_id: cluster-east
        location: "40.7128,-74.0060"   # New York
    read_strategy: nearest
    write_policy: all

  - id: eu-mirrors
    name: EU Region Mirrors
    mirrors:
      - id: eu-west
        cluster_id: cluster-ireland
        location: "53.3498,-6.2603"    # Dublin
        is_primary: true
      - id: eu-central
        cluster_id: cluster-frankfurt
        location: "50.1109,8.6821"     # Frankfurt
    read_strategy: nearest
    write_policy: quorum
```

### Mirror Status

```bash
# List all mirror groups
kscore-files mirrors list

# Show mirror group details
kscore-files mirrors show us-mirrors

# Check mirror health
kscore-files mirrors health --group us-mirrors

# Output:
# Mirror Group: us-mirrors
# Status: healthy
#
# Mirror     | Status  | Latency | Objects | Last Sync
# -----------|---------|---------|---------|------------------
# us-west    | healthy | 5ms     | 15,432  | 2024-01-15T10:20:00Z
# us-east    | healthy | 45ms    | 15,432  | 2024-01-15T10:20:00Z
```

### Sync Operations

```bash
# Check sync status
kscore-files mirrors sync-status --group us-mirrors

# Trigger manual sync
kscore-files mirrors sync --group us-mirrors

# Force full resync
kscore-files mirrors sync --group us-mirrors --force

# View sync history
kscore-files mirrors history --group us-mirrors --limit 10
```

### Conflict Resolution

```bash
# List conflicts
kscore-files mirrors conflicts --group us-mirrors

# View conflict details
kscore-files mirrors conflicts --id conflict-123

# Resolve using source version
kscore-files mirrors resolve-conflict conflict-123 --strategy source

# Resolve using target version
kscore-files mirrors resolve-conflict conflict-123 --strategy target
```

### Failover Operations

```bash
# Trigger failover from unhealthy mirror
kscore-files mirrors failover us-mirrors --from us-west

# Promote secondary to primary
kscore-files mirrors promote us-mirrors --mirror us-east
```

## Namespace Management

### Creating Namespaces

```bash
# Create namespace
kscore-files namespace create prod-configs \
  --description "Production configuration files"

# Create with quota
kscore-files namespace create dev-configs \
  --description "Development configuration files" \
  --quota 10GB \
  --max-files 10000
```

### Managing ACLs

```bash
# Add ACL
kscore-files namespace acl add prod-configs \
  --principal role:ops \
  --permission read,write

# List ACLs
kscore-files namespace acl list prod-configs

# Remove ACL
kscore-files namespace acl remove prod-configs \
  --principal role:developer
```

### Namespace Operations

```bash
# List namespaces
kscore-files namespace list

# Show namespace details
kscore-files namespace show prod-configs

# Delete namespace (with confirmation)
kscore-files namespace delete dev-old --force

# Export namespace
kscore-files namespace export prod-configs --output /backup/prod-configs.tar.gz
```

## File Operations

### Upload and Download

```bash
# Upload a file
kscore-files put local-file.txt prod-configs/path/file.txt

# Upload directory recursively
kscore-files put -r ./configs/ prod-configs/app/

# Download a file
kscore-files get prod-configs/path/file.txt local-file.txt

# Download to stdout
kscore-files cat prod-configs/path/file.txt
```

### Listing and Metadata

```bash
# List files
kscore-files ls prod-configs/

# List recursively
kscore-files ls -r prod-configs/

# Get file metadata
kscore-files stat prod-configs/path/file.txt

# Get file hash
kscore-files hash prod-configs/path/file.txt
```

### Delete Operations

```bash
# Delete single file
kscore-files rm prod-configs/path/old-file.txt

# Delete recursively
kscore-files rm -r prod-configs/old-directory/

# Delete with confirmation
kscore-files rm -i prod-configs/important-file.txt
```

## Monitoring

### Prometheus Metrics

Key metrics to monitor:

| Metric | Type | Description |
|--------|------|-------------|
| `kscore_files_requests_total` | Counter | Total file requests by operation |
| `kscore_files_request_duration_seconds` | Histogram | Request latency |
| `kscore_files_bytes_transferred_total` | Counter | Bytes transferred |
| `kscore_files_backend_operations_total` | Counter | Backend operations |
| `kscore_files_backend_errors_total` | Counter | Backend errors |
| `kscore_mirror_health` | Gauge | Mirror health (1=healthy) |
| `kscore_mirror_sync_lag_seconds` | Gauge | Sync lag between mirrors |
| `kscore_mirror_conflicts_total` | Counter | Sync conflicts |

### Health Endpoints

```bash
# Liveness check
curl http://localhost:8081/health/live

# Readiness check (includes backend)
curl http://localhost:8081/health/ready

# Detailed status
curl http://localhost:8081/health/status
```

### Grafana Dashboard

Import the pre-built dashboard from `deploy/grafana/dashboards/file-distribution.json`:

- File operation rates and latency
- Backend health and performance
- Mirror group status
- Namespace usage
- Error rates

### Alert Rules

Key alerts to configure:

```yaml
groups:
  - name: file-distribution
    rules:
      - alert: FileBackendUnhealthy
        expr: kscore_files_backend_health == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "File distribution backend unhealthy"

      - alert: MirrorSyncLag
        expr: kscore_mirror_sync_lag_seconds > 300
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Mirror sync lag exceeds 5 minutes"

      - alert: HighFileErrorRate
        expr: rate(kscore_files_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High file operation error rate"
```

## Troubleshooting

### Backend Connection Issues

```bash
# Test backend connectivity
kscore-files backend test

# Check backend logs
journalctl -u kscore-files -f | grep -i backend

# Verify credentials (S3)
aws s3 ls s3://kscore-files-prod/

# Verify credentials (GCS)
gsutil ls gs://kscore-files-prod/
```

### Mirror Sync Issues

```bash
# Check sync status
kscore-files mirrors sync-status --group us-mirrors --verbose

# View sync errors
kscore-files mirrors errors --group us-mirrors

# Force resync
kscore-files mirrors sync --group us-mirrors --force

# Check network connectivity between mirrors
kscore-files mirrors ping --group us-mirrors
```

### Performance Issues

```bash
# Check backend latency
kscore-files backend benchmark

# Profile file operations
kscore-files --debug get prod-configs/large-file.bin /dev/null

# Check rate limiting
curl http://localhost:8081/metrics | grep rate_limit
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `backend unavailable` | Backend connection failed | Check credentials and network |
| `namespace not found` | Invalid namespace | Create namespace or check spelling |
| `permission denied` | ACL violation | Update ACLs or use correct principal |
| `quota exceeded` | Namespace quota full | Increase quota or delete files |
| `sync conflict` | Concurrent modifications | Resolve conflict manually |
| `hash mismatch` | Corruption detected | Re-upload file or check backend |

## Backup and Recovery

### Backup Procedures

```bash
# Export all files and metadata
kscore-files export --output /backup/files-$(date +%Y%m%d).tar.gz

# Export specific namespace
kscore-files namespace export prod-configs \
  --output /backup/prod-configs-$(date +%Y%m%d).tar.gz

# Export metadata only (for disaster recovery)
kscore-files export --metadata-only --output /backup/metadata-$(date +%Y%m%d).json
```

### Recovery Procedures

```bash
# Restore from backup
kscore-files import --input /backup/files-20240115.tar.gz

# Restore specific namespace
kscore-files namespace import prod-configs \
  --input /backup/prod-configs-20240115.tar.gz

# Verify restoration
kscore-files verify --namespace prod-configs
```

### Disaster Recovery

1. **Backend failure**: Failover to secondary mirror or restore from backup
2. **Data corruption**: Use content-addressed hashes to identify and restore corrupted files
3. **Complete loss**: Restore from backup to new backend, resync mirrors

## Best Practices

### Storage Backend Selection

| Use Case | Recommended Backend |
|----------|---------------------|
| Development/testing | Local |
| AWS deployment | S3 |
| GCP deployment | GCS |
| Azure deployment | Azure Blob |
| GitOps workflows | Git |
| NATS-centric | NATS Object Store |
| Multi-cloud | S3-compatible (MinIO) |

### High Availability

- Configure at least 2 mirrors per region
- Use `write_policy: quorum` for consistency
- Monitor sync lag and alert on >5 minutes
- Test failover procedures regularly

### Security

- Enable TLS for all connections
- Use IAM roles instead of static credentials
- Enable server-side encryption for cloud backends
- Use namespace ACLs for least-privilege access
- Enable audit logging for compliance

### Performance

- Use content-addressed storage for deduplication
- Enable agent-side caching for frequently accessed files
- Place mirrors close to agent clusters
- Monitor and tune rate limits based on load

## See Also

- [File Distribution Concepts](/docs/concepts/file-distribution/)
- [File Backends Reference](/docs/reference/file-backends/)
- [CLI Reference - kscore-files](/docs/reference/cli/#kscore-files)
- [Configuration Reference](/docs/reference/configuration/#file-server-configuration)
