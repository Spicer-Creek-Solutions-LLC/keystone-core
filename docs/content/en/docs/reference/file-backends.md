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
> **Supported Types**: `filesystem` (alias: `local`), `s3`, `gcs`, `azure`, `git`, `nats` (alias: `nats-object-store`). Backend-specific options are set as flat fields alongside `name` and `type`.

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

> **Note**: The backend type is `filesystem` (alias: `local`). Directories are created automatically.

## Amazon S3 Backend

Stores files in Amazon S3 or S3-compatible storage.

```yaml
backends:
  - name: s3-files
    type: s3
    bucket: my-kscore-files
    region: us-west-2
    prefix: files/
    endpoint: ""           # Custom endpoint for S3-compatible storage
    access_key_id: ${AWS_ACCESS_KEY_ID}
    secret_access_key: ${AWS_SECRET_ACCESS_KEY}
    profile: ""            # AWS profile (or use explicit keys)
    use_path_style: false  # Use path-style URLs
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `s3` |
| `bucket` | string | Required | S3 bucket name |
| `region` | string | Required | AWS region |
| `prefix` | string | `""` | Key prefix for all objects |
| `endpoint` | string | `""` | Custom endpoint URL (for MinIO, etc.) |
| `access_key_id` | string | `""` | AWS access key (or use IAM role) |
| `secret_access_key` | string | `""` | AWS secret key |
| `profile` | string | `""` | AWS credentials profile |
| `use_path_style` | bool | `false` | Use path-style URLs instead of virtual-hosted |
| `paths` | []string | `[]` | Restrict access to specific paths |
| `read_only` | bool | `false` | Make backend read-only |

### S3-Compatible Storage

For MinIO, Ceph, or other S3-compatible storage:

```yaml
backends:
  - name: minio-files
    type: s3
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
backends:
  - name: gcs-files
    type: gcs
    bucket: my-kscore-files
    prefix: files/
    credentials_file: /path/to/credentials.json
    project: my-project
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `gcs` |
| `bucket` | string | Required | GCS bucket name |
| `prefix` | string | `""` | Object prefix |
| `credentials_file` | string | `""` | Path to service account JSON file |
| `project` | string | `""` | GCP project ID |
| `paths` | []string | `[]` | Restrict access to specific paths |
| `read_only` | bool | `false` | Make backend read-only |

### Authentication

GCS supports multiple authentication methods:

1. **Service account file**: Set `credentials_file`
2. **Environment variable**: Set `GOOGLE_APPLICATION_CREDENTIALS`
3. **Workload Identity**: For GKE deployments
4. **Metadata server**: For Compute Engine instances

## Azure Blob Storage Backend

Stores files in Azure Blob Storage.

```yaml
backends:
  - name: azure-files
    type: azure
    container: kscore-files
    account_name: mystorageaccount
    account_key: ${AZURE_STORAGE_KEY}
    prefix: files/
    connection_string: ""  # Alternative: full connection string
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `azure` |
| `container` | string | Required | Blob container name |
| `account_name` | string | Required | Storage account name |
| `account_key` | string | `""` | Storage account access key |
| `connection_string` | string | `""` | Full connection string (alternative to account_name/account_key) |
| `prefix` | string | `""` | Blob name prefix |
| `paths` | []string | `[]` | Restrict access to specific paths |
| `read_only` | bool | `false` | Make backend read-only |

### Authentication

Azure supports:

1. **Access key**: Set `account_key`
2. **Connection string**: Set `connection_string` (includes account and key)
3. **Managed Identity**: Omit `account_key` for Azure VMs

## Git Repository Backend

Stores files in a Git repository with version history.

```yaml
backends:
  - name: git-files
    type: git
    url: https://github.com/myorg/kscore-files.git
    branch: main
    local_path: /var/lib/keystone-core/git-files
    pull_interval: 5m
    auto_pull: true
    username: git-user
    password: ${GIT_TOKEN}
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `git` |
| `url` | string | Required | Git repository URL |
| `branch` | string | `main` | Branch to use |
| `local_path` | string | Required | Local clone directory |
| `pull_interval` | duration | `0` | Automatic pull interval (0 = disabled) |
| `auto_pull` | bool | `false` | Enable automatic pulling |
| `ssh_key_file` | string | `""` | Path to SSH private key |
| `username` | string | `""` | Git username (for HTTPS auth) |
| `password` | string | `""` | Git password or token (for HTTPS auth) |
| `paths` | []string | `[]` | Restrict access to specific paths |
| `read_only` | bool | `false` | Make backend read-only |

### Authentication

```yaml
# HTTPS token authentication (GitHub, GitLab)
backends:
  - name: git-files
    type: git
    url: https://github.com/myorg/kscore-files.git
    username: oauth2
    password: ghp_xxxxxxxxxxxx

# SSH key authentication
backends:
  - name: git-files
    type: git
    url: git@github.com:myorg/kscore-files.git
    ssh_key_file: /path/to/id_rsa
```

## NATS Object Store Backend

Stores files in NATS JetStream Object Store.

```yaml
backends:
  - name: nats-files
    type: nats              # alias: nats-object-store
    bucket_name: kscore-files
    endpoint: nats://localhost:4222  # Optional: override NATS URL
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | Required | Unique name for this backend |
| `type` | string | Required | Must be `nats` or `nats-object-store` |
| `bucket_name` | string | Required | Object store bucket name |
| `endpoint` | string | `""` | NATS server URL (overrides server config) |
| `paths` | []string | `[]` | Restrict access to specific paths |
| `read_only` | bool | `false` | Make backend read-only |

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
