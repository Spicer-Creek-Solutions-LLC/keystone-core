---
title: "File Backends Reference"
description: "Configuration reference for file distribution storage backends"
weight: 80
---

# File Backends Reference

This page provides detailed configuration options for each file distribution storage backend.

## Backend Configuration

All backends are configured in the kscore-files configuration file:

```yaml
# /etc/kscore/files.yaml
backend:
  type: <backend-type>
  <backend-type>:
    # Backend-specific options
```

## Local Filesystem Backend

Stores files on the local filesystem.

```yaml
backend:
  type: local
  local:
    root: /var/lib/kscore/files
    temp_dir: /var/lib/kscore/tmp
    create_dirs: true
    dir_mode: "0755"
    file_mode: "0644"
```

### Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `root` | string | Required | Root directory for file storage |
| `temp_dir` | string | `<root>/.tmp` | Directory for temporary files |
| `create_dirs` | bool | `true` | Create directories if they don't exist |
| `dir_mode` | string | `"0755"` | Permission mode for directories |
| `file_mode` | string | `"0644"` | Permission mode for files |

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
    local_path: /var/lib/kscore/git-files
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

## Content-Addressed Storage

All backends use content-addressed storage internally:

1. Files are hashed with SHA-256
2. Stored at path: `<prefix>/<hash[0:2]>/<hash>`
3. Metadata stored separately with original path mapping
4. Deduplication automatic for identical content

```
/var/lib/kscore/files/
├── ab/
│   ├── ab3def4567890...  # Content file
│   └── ab9876543210...   # Another content file
└── cd/
    └── cdef0123456...    # Content file
```

## See Also

- [File Distribution Concepts](/docs/concepts/file-distribution)
