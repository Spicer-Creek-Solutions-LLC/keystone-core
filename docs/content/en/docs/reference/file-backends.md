---
title: "File Backends Reference"
description: "Configuration reference for file distribution storage backends"
weight: 80
---

This page provides detailed configuration options for each file distribution storage backend.

## Backend Configuration

All backends are configured in the kscore-files configuration file as an array:

```yaml
# /etc/keystone-core/files.yaml
backends:
  - name: <backend-name>
    type: <backend-type>
    root_path: <path>
    paths: []        # Optional: restrict to specific paths
    read_only: false # Optional: make backend read-only
```

> **Note**: The server expects a `backends` array, not a single `backend` object. Each backend requires a unique `name` and `type`.
>
> **Current Support**: Only the `filesystem` backend type is fully wired in the kscore-files server. Cloud backends (S3, GCS, Azure) are implemented as library code but not yet exposed through the server configuration. See individual backend sections for future configuration reference.

## Compression System

The file distribution service includes an optional compression library for stored files and transfer payloads. Compression is MIME-aware, can skip already compressed formats, and automatically avoids compression when it would increase size. Integration is available for file distribution components but is not yet exposed through the public configuration.

**Supported Algorithms**:

- `none` (disabled)
- `gzip`
- `zstd`
- `lz4`
- `snappy`
- `auto` (size-based selection, currently falls back to gzip when other libraries are not available)

> **Note**: The current implementation uses gzip as a fallback for zstd/lz4/snappy until native libraries are wired in.

**Compression Configuration Fields** (internal API; not yet exposed in `files.yaml`):

| Field | Type | Description |
|-------|------|-------------|
| `algorithm` | string | Compression algorithm (`none`, `gzip`, `zstd`, `lz4`, `snappy`, `auto`) |
| `level` | int | Compression level (`0` default, 1 fastest, 5 balanced, 7 better, 9 best) |
| `min_size` | int64 | Minimum size (bytes) to consider compression |
| `max_size` | int64 | Maximum size (bytes) to attempt compression |
| `skip_compressed` | bool | Skip already compressed content types |
| `compressible_types` | []string | MIME types to always compress |
| `incompressible_types` | []string | MIME types to never compress |

**Behavior Notes**:

- If `compressible_types` is set, only those types are compressed.
- If compression yields larger data, the original payload is preserved.
- MIME checks use both content type and file extension hints.

## Storage Failover

Keystone Core includes a storage failover manager that can monitor backend health and switch to alternate backends. Health checks track latency, consecutive failures, and recovery thresholds, and the failover manager can queue operations while a backend is unavailable. This is currently a library component for file distribution and not yet exposed in the public configuration.

**Failover Configuration Fields** (internal API; not yet exposed in `files.yaml`):

| Field | Type | Description |
|-------|------|-------------|
| `health_check_interval` | duration | Interval between health checks |
| `health_check_timeout` | duration | Timeout for a single health check |
| `max_consecutive_failures` | int | Failures before marking unhealthy |
| `recovery_threshold` | int | Successes required to mark healthy |
| `queue_size` | int | Maximum queued operations |
| `queue_timeout` | duration | Time to keep operations in queue |
| `retry_attempts` | int | Retry attempts for failed ops |
| `retry_delay` | duration | Delay between retries |
| `enable_queue` | bool | Enable queueing during failover |

**Failover Behavior**:

- Health checks run continuously and update backend status.
- When a backend becomes unhealthy, operations can be queued or retried.
- Backends recover automatically after reaching the recovery threshold.

## Mirror Sync Strategies

Mirror groups support incremental sync based on metadata comparison (checksum, size, and modification time). Sync plans classify actions per file: `copy`, `delete`, `conflict`, or `skip`.

**Conflict Strategies**:

- `newest-wins`: Choose the file with the most recent modification time.
- `largest-wins`: Choose the file with the largest size.
- `primary-wins`: Always select the primary mirror's version.
- `manual`: Flag conflicts for manual resolution.

**Sync Configuration Fields** (internal engine config; not yet exposed in `files.yaml`):

| Field | Type | Description |
|-------|------|-------------|
| `interval` | duration | Automatic sync interval (0 disables) |
| `batch_size` | int | Number of files per sync batch |
| `bandwidth_limit` | int64 | Bytes/sec limit (0 = unlimited) |
| `conflict_strategy` | string | Conflict resolution strategy |
| `retry_attempts` | int | Retries for failed operations |
| `retry_delay` | duration | Delay between retries |
| `exclude_patterns` | []string | Glob/regex patterns to skip |
| `prioritize_small_files` | bool | Sync small files first |
| `small_file_size_threshold` | int64 | Threshold for "small" files |

## Local Filesystem Backend

Stores files on the local filesystem.

```yaml
backends:
  - name: local-files
    type: filesystem
    root_path: /var/lib/keystone-core/files
    paths: []
    read_only: false
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `filesystem` |
| `root_path` | string | Required | Root directory for file storage |
| `paths` | []string | `[]` | Restrict access to specific paths (empty = all) |
| `read_only` | bool | `false` | Make backend read-only |

> **Note**: The backend type is `filesystem`, not `local`. Directories are created automatically.

## Amazon S3 Backend

Stores files in Amazon S3 or S3-compatible storage.

```yaml
backend:
  type: s3
  s3:
    bucket: my-kscore-files
    region: us-west-2
    prefix: files/
    endpoint: ""  # Custom endpoint for S3-compatible storage
    access_key_id: ${AWS_ACCESS_KEY_ID}
    secret_access_key: ${AWS_SECRET_ACCESS_KEY}
    session_token: ""  # Optional session token
    use_path_style: false  # Use path-style URLs
    storage_class: STANDARD
    server_side_encryption: ""  # AES256 or aws:kms
    kms_key_id: ""  # KMS key for encryption
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `bucket` | string | Required | S3 bucket name |
| `region` | string | Required | AWS region |
| `prefix` | string | `""` | Key prefix for all objects |
| `endpoint` | string | `""` | Custom endpoint URL (for MinIO, etc.) |
| `access_key_id` | string | `""` | AWS access key (or use IAM role) |
| `secret_access_key` | string | `""` | AWS secret key |
| `session_token` | string | `""` | AWS session token |
| `use_path_style` | bool | `false` | Use path-style URLs instead of virtual-hosted |
| `storage_class` | string | `STANDARD` | S3 storage class |
| `server_side_encryption` | string | `""` | Server-side encryption type |
| `kms_key_id` | string | `""` | KMS key ID for encryption |

### S3-Compatible Storage

For MinIO, Ceph, or other S3-compatible storage:

```yaml
backend:
  type: s3
  s3:
    bucket: my-bucket
    region: us-east-1
    endpoint: https://minio.example.com
    use_path_style: true
    access_key_id: minioadmin
    secret_access_key: minioadmin
```

## Google Cloud Storage Backend

Stores files in Google Cloud Storage.

```yaml
backend:
  type: gcs
  gcs:
    bucket: my-kscore-files
    prefix: files/
    credentials_file: /path/to/credentials.json
    project_id: my-project
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `bucket` | string | Required | GCS bucket name |
| `prefix` | string | `""` | Object prefix |
| `credentials_file` | string | `""` | Path to service account JSON file |
| `project_id` | string | `""` | GCP project ID |

### Authentication

GCS supports multiple authentication methods:

1. **Service account file**: Set `credentials_file`
2. **Environment variable**: Set `GOOGLE_APPLICATION_CREDENTIALS`
3. **Workload Identity**: For GKE deployments
4. **Metadata server**: For Compute Engine instances

## Azure Blob Storage Backend

Stores files in Azure Blob Storage.

```yaml
backend:
  type: azure
  azure:
    container: kscore-files
    account: mystorageaccount
    access_key: ${AZURE_STORAGE_KEY}
    prefix: files/
    endpoint: ""  # Custom endpoint
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `container` | string | Required | Blob container name |
| `account` | string | Required | Storage account name |
| `access_key` | string | `""` | Storage account access key |
| `prefix` | string | `""` | Blob name prefix |
| `endpoint` | string | `""` | Custom endpoint URL |

### Authentication

Azure supports:

1. **Access key**: Set `access_key`
2. **Managed Identity**: Omit `access_key` for Azure VMs
3. **Service Principal**: Use environment variables

## Git Repository Backend

Stores files in a Git repository with version history.

```yaml
backend:
  type: git
  git:
    url: https://github.com/myorg/kscore-files.git
    branch: main
    local_path: /var/lib/keystone-core/git-files
    sync_interval: 5m
    auto_commit: true
    commit_author: "Keystone Core <kscore@example.com>"
    auth:
      type: token
      token: ${GIT_TOKEN}
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `url` | string | Required | Git repository URL |
| `branch` | string | `main` | Branch to use |
| `local_path` | string | Required | Local clone directory |
| `sync_interval` | duration | `5m` | Pull interval |
| `auto_commit` | bool | `true` | Auto-commit on write |
| `commit_author` | string | `Keystone Core` | Git commit author |

### Authentication

```yaml
# Token authentication (GitHub, GitLab)
auth:
  type: token
  token: ghp_xxxxxxxxxxxx

# SSH key authentication
auth:
  type: ssh-key
  ssh_key_path: /path/to/id_rsa
  ssh_key_password: ""  # Optional passphrase

# SSH agent
auth:
  type: ssh-agent
```

## NATS Object Store Backend

Stores files in NATS JetStream Object Store.

```yaml
backend:
  type: nats
  nats:
    bucket: kscore-files
    description: "Keystone Core file storage"
    replicas: 3
    ttl: 0  # 0 = no expiration
    max_bytes: 0  # 0 = unlimited
    storage: file  # file or memory
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `bucket` | string | Required | Object store bucket name |
| `description` | string | `""` | Bucket description |
| `replicas` | int | `1` | Replica count for HA |
| `ttl` | duration | `0` | Object TTL (0 = never expire) |
| `max_bytes` | int64 | `0` | Max bucket size (0 = unlimited) |
| `storage` | string | `file` | Storage type: `file` or `memory` |

### Connection

The NATS backend uses the existing NATS connection from kscore-files. Configure the NATS connection in the server config:

```yaml
nats:
  url: nats://localhost:4222
  # Or for clusters:
  urls:
    - nats://nats1:4222
    - nats://nats2:4222
    - nats://nats3:4222
```

## Backend Selection Guide

| Use Case | Recommended Backend |
|----------|---------------------|
| Development/testing | Local |
| AWS deployment | S3 |
| GCP deployment | GCS |
| Azure deployment | Azure |
| GitOps workflows | Git |
| NATS-centric architecture | NATS Object Store |
| Hybrid/multi-cloud | S3 (with compatible storage) |

## Backend Interface

All backends implement the `Backend` interface:

```go
type Backend interface {
    // Read reads a file from the backend
    Read(ctx context.Context, path string) (io.ReadCloser, error)

    // Write writes a file to the backend
    Write(ctx context.Context, path string, r io.Reader) error

    // Delete removes a file from the backend
    Delete(ctx context.Context, path string) error

    // Exists checks if a file exists
    Exists(ctx context.Context, path string) (bool, error)

    // Stat returns file metadata
    Stat(ctx context.Context, path string) (*FileInfo, error)

    // List lists files with a prefix
    List(ctx context.Context, prefix string) ([]FileInfo, error)

    // Hash returns the content hash of a file
    Hash(ctx context.Context, path string) (string, error)
}
```

## Storage Layout

The filesystem backend stores files at their original paths relative to the root directory:

```
/var/lib/keystone-core/files/
├── configs/
│   └── app.yaml
├── scripts/
│   └── deploy.sh
└── artifacts/
    └── v1.2.3/
        └── binary
```

Files are stored with their original names and directory structure. SHA-256 checksums are calculated for integrity verification but are not used for storage paths.

## See Also

- [File Distribution Concepts](/docs/concepts/file-distribution)
