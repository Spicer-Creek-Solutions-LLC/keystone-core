# Epic 22: File Distribution

## Overview

Implement a dedicated file distribution system that enables agents to retrieve packages, configurations, binaries, and other files over NATS without requiring additional network connections. The system uses a dedicated file server (`kscore-files`) that supports multiple storage backends and integrates with proxy agents for edge caching.

**Goal**: Provide a secure, efficient, and scalable file distribution mechanism that works entirely over the existing NATS infrastructure, supporting everything from small config files to large binary packages.

## Success Criteria

- [ ] Agents can request and receive files over NATS (no HTTP/S3 access required)
- [ ] Support files up to 10GB with chunked transfer
- [ ] Multiple storage backends: Local filesystem, S3, GCS, Azure Blob, NATS Object Store, HTTP, Git
- [ ] SHA-256 integrity verification for all transfers
- [ ] Proxy agent caching reduces bandwidth by 80%+ for repeated file requests
- [ ] File metadata and versioning support
- [ ] Access control integrated with agent identity
- [ ] Transfer resume on connection interruption
- [ ] <100ms latency for cached file metadata lookups
- [ ] Support 1000+ concurrent file transfers across fleet

## Architecture

### High-Level Architecture

```mermaid
flowchart TD
    subgraph Agents["Agent Fleet"]
        A1[Agent 1]
        A2[Agent 2]
        A3[Agent 3]
    end

    subgraph ProxyLayer["Proxy Agents (Optional)"]
        PA1[Proxy Agent<br/>+ File Cache]
        PA2[Proxy Agent<br/>+ File Cache]
    end

    subgraph FileServer["File Server Cluster"]
        FS1[kscore-files 1]
        FS2[kscore-files 2]
    end

    subgraph Storage["Storage Backends"]
        OBJ[(NATS Object Store)]
        S3[(S3 / GCS / Azure)]
        LOCAL[(Local Filesystem)]
        GIT[(Git Repository)]
        HTTP[HTTP Remote]
    end

    NATS{NATS Cluster}

    A1 & A2 & A3 <--> NATS
    PA1 & PA2 <--> NATS
    FS1 & FS2 <--> NATS

    FS1 & FS2 --> OBJ & S3 & LOCAL & GIT & HTTP
```

### Request Flow

```mermaid
sequenceDiagram
    participant Agent
    participant NATS
    participant Proxy as Proxy Agent (Cache)
    participant FS as kscore-files
    participant Backend as Storage Backend

    Agent->>NATS: 1. FileRequest (path, checksum?)

    alt Proxy Agent Available
        NATS->>Proxy: 2a. Forward request
        alt Cache Hit
            Proxy->>NATS: 3a. Return cached file (chunked)
            NATS->>Agent: 4a. File chunks
        else Cache Miss
            Proxy->>NATS: 3b. Forward to file server
            NATS->>FS: 4b. FileRequest
            FS->>Backend: 5. Fetch file
            Backend->>FS: 6. File data
            FS->>NATS: 7. File chunks
            NATS->>Proxy: 8. Cache + forward
            Proxy->>NATS: 9. Forward to agent
            NATS->>Agent: 10. File chunks
        end
    else Direct to File Server
        NATS->>FS: 2b. FileRequest
        FS->>Backend: 3. Fetch file
        Backend->>FS: 4. File data
        FS->>NATS: 5. File chunks (streamed)
        NATS->>Agent: 6. File chunks
    end

    Agent->>Agent: Verify SHA-256 checksum
```

### Chunked Transfer Protocol

```mermaid
sequenceDiagram
    participant Agent
    participant FS as kscore-files

    Agent->>FS: FileRequest{path: "/packages/nginx-1.24.deb"}
    FS->>Agent: FileMetadata{size: 52428800, chunks: 50, sha256: "abc..."}

    loop For each chunk (0-49)
        FS->>Agent: FileChunk{index: N, data: [1MB], sha256: "..."}
        Agent->>Agent: Verify chunk, write to disk
    end

    Agent->>Agent: Verify complete file SHA-256
    Agent->>FS: FileAck{status: "complete"}
```

## Concepts

### File Server (`kscore-files`)

A dedicated service that:
- Listens for file requests over NATS
- Fetches files from configured storage backends
- Streams files in chunks to requesting agents
- Handles concurrent requests with connection pooling
- Provides file metadata (size, checksum, modified time, version)
- Supports multiple instances for horizontal scaling

### Storage Backends

| Backend | Use Case | Features |
|---------|----------|----------|
| **NATS Object Store** | Small-medium files, built-in replication | Native NATS, automatic chunking, versioning |
| **Local Filesystem** | Air-gapped, simple deployments | Fast, no external deps, manual replication |
| **S3 / GCS / Azure** | Large scale, cloud-native | Virtually unlimited, durable, expensive egress |
| **Git Repository** | Config files, versioned content | Version history, branch support, GitOps friendly |
| **HTTP Remote** | Existing file servers, mirrors | Proxy existing infrastructure |

### File Namespaces

Files are organized into namespaces for access control and organization:

```
/packages/          # OS packages (deb, rpm, msi)
/configs/           # Configuration files
/scripts/           # Automation scripts
/binaries/          # Executable binaries
/certificates/      # TLS certificates, CA bundles
/modules/           # Keystone Core modules
/custom/            # User-defined content
```

### Proxy Agent Caching

Proxy agents (from Epic 21) can act as file caches:

- **Cache Policy**: LRU with configurable max size
- **TTL**: Per-file or namespace-level TTL
- **Invalidation**: Explicit invalidation via NATS message
- **Warm-up**: Pre-populate cache with expected files
- **Bandwidth Saving**: Significant reduction for repeated requests in same location

### File Versioning

Files support versioning for safe updates:

```yaml
# Request specific version
path: /packages/nginx
version: "1.24.0-2"

# Request latest
path: /packages/nginx
version: "latest"

# Request by tag
path: /configs/app-config.yaml
tag: "production"
```

## User Stories

### US22.1: Basic File Retrieval
**As an** agent
**I want to** retrieve files over NATS
**So that** I don't need additional network access

**Acceptance Criteria**:
- Agent can request file by path
- File server streams file in chunks
- Agent verifies SHA-256 checksum
- Failed transfers can be resumed
- Timeout and retry handling

### US22.2: Storage Backend Configuration
**As a** platform operator
**I want to** configure multiple storage backends
**So that** I can use existing infrastructure

**Acceptance Criteria**:
- Support local filesystem backend
- Support S3-compatible storage (AWS, MinIO, GCS, Azure)
- Support NATS JetStream Object Store
- Support Git repositories (clone/pull)
- Support HTTP/HTTPS remote URLs
- Backend selection based on file path patterns
- Fallback backends for redundancy

### US22.3: Large File Transfer
**As an** agent
**I want to** download large files (>1GB) reliably
**So that** I can install large packages or binaries

**Acceptance Criteria**:
- Chunked transfer with configurable chunk size (default 1MB)
- Per-chunk checksum verification
- Resume interrupted transfers from last successful chunk
- Progress reporting via events
- Memory-efficient streaming (no full file buffering)
- Support files up to 10GB

### US22.4: File Metadata and Discovery
**As an** agent or operator
**I want to** query file metadata without downloading
**So that** I can check versions and make conditional requests

**Acceptance Criteria**:
- Query file size, checksum, modified time, version
- List files in namespace/directory
- Conditional GET (if-none-match with checksum)
- Search files by metadata/tags
- File existence check

### US22.5: Proxy Agent File Caching
**As a** platform operator
**I want** proxy agents to cache files
**So that** bandwidth is reduced for agents in the same location

**Acceptance Criteria**:
- Proxy agents can enable file caching
- Configurable cache size (default 10GB)
- LRU eviction when cache full
- TTL-based expiration (configurable per namespace)
- Cache hit/miss metrics
- Manual cache invalidation
- Cache warm-up from file list
- Agents automatically use nearest proxy with cache

### US22.6: Access Control
**As a** security engineer
**I want** file access controlled by agent identity
**So that** agents can only access authorized files

**Acceptance Criteria**:
- Namespace-level permissions (read, write, list)
- Agent identity (SPIFFE) integration
- Policy-based access control (OPA/CEL)
- Audit logging of file access
- Support for file-level ACLs
- Deny by default for undefined permissions

### US22.7: File Upload from Agents
**As an** agent
**I want to** upload files to the file server
**So that** I can store logs, artifacts, or backups

**Acceptance Criteria**:
- Chunked upload protocol
- Server-side checksum verification
- Configurable upload size limits per namespace
- Quota enforcement per agent/namespace
- Temporary upload staging before commit
- Upload progress events

### US22.8: Git Repository Backend
**As a** platform operator
**I want to** serve files from Git repositories
**So that** configuration is versioned and GitOps-friendly

**Acceptance Criteria**:
- Clone Git repository on startup
- Periodic pull for updates (configurable interval)
- Webhook trigger for immediate refresh
- Branch/tag selection per namespace mapping
- Sparse checkout for large repos
- SSH and HTTPS authentication

### US22.9: File Change Notifications
**As an** agent
**I want to** be notified when files change
**So that** I can automatically update configurations

**Acceptance Criteria**:
- Subscribe to file/namespace changes
- Change events include: path, version, checksum, action (created/updated/deleted)
- Debounce rapid changes
- Event filtering by pattern
- Integration with reactor system (Epic 4)

### US22.10: Bandwidth Management
**As a** platform operator
**I want to** control file transfer bandwidth
**So that** file distribution doesn't impact other operations

**Acceptance Criteria**:
- Rate limiting per agent
- Rate limiting per file server instance
- Priority queues (critical files first)
- Concurrent transfer limits
- Bandwidth metrics and monitoring
- Time-based bandwidth policies (full speed during maintenance windows)

### US22.11: File Server High Availability
**As a** platform operator
**I want** file servers to be highly available
**So that** file distribution continues during failures

**Acceptance Criteria**:
- Multiple file server instances
- Automatic request routing to healthy instances
- Shared backend storage (S3, NATS Object Store)
- No single point of failure
- Graceful degradation when backends unavailable
- Health check endpoints

### US22.12: Integration with State Modules
**As a** state module author
**I want** state modules to reference distributed files
**So that** configurations can include managed file content

**Acceptance Criteria**:
- `file` module supports `source: kscore://path/to/file`
- Automatic checksum verification
- Version pinning in state files
- Cached locally after first retrieval
- Template files supported

## NATS Subject Namespace

```
kscore.{cluster}.files.request.{namespace}     # File requests
kscore.{cluster}.files.metadata.{namespace}    # Metadata queries
kscore.{cluster}.files.upload.{namespace}      # File uploads
kscore.{cluster}.files.notify.{namespace}      # Change notifications
kscore.{cluster}.files.cache.invalidate        # Cache invalidation
kscore.{cluster}.files.admin.{operation}       # Admin operations
```

## File Request Protocol

### FileRequest Message

```go
type FileRequest struct {
    RequestID     string            `json:"request_id"`
    Path          string            `json:"path"`
    Version       string            `json:"version,omitempty"`       // "latest", semver, or tag
    Checksum      string            `json:"checksum,omitempty"`      // Skip if matches (conditional)
    Range         *ByteRange        `json:"range,omitempty"`         // For resume
    ChunkSize     int               `json:"chunk_size,omitempty"`    // Override default
    Priority      int               `json:"priority,omitempty"`      // 0=normal, 1=high, 2=critical
    AgentID       string            `json:"agent_id"`
    Metadata      map[string]string `json:"metadata,omitempty"`
}

type ByteRange struct {
    Start int64 `json:"start"`
    End   int64 `json:"end,omitempty"`  // 0 = to end of file
}
```

### FileMetadata Response

```go
type FileMetadata struct {
    RequestID    string            `json:"request_id"`
    Path         string            `json:"path"`
    Version      string            `json:"version"`
    Size         int64             `json:"size"`
    Checksum     string            `json:"checksum"`           // SHA-256
    ContentType  string            `json:"content_type"`
    ModifiedTime time.Time         `json:"modified_time"`
    ChunkCount   int               `json:"chunk_count"`
    ChunkSize    int               `json:"chunk_size"`
    Tags         map[string]string `json:"tags,omitempty"`
    NotModified  bool              `json:"not_modified"`       // True if checksum matched
}
```

### FileChunk Message

```go
type FileChunk struct {
    RequestID  string `json:"request_id"`
    Index      int    `json:"index"`
    TotalCount int    `json:"total_count"`
    Data       []byte `json:"data"`            // Base64 encoded in JSON
    Checksum   string `json:"checksum"`        // Chunk checksum
    Final      bool   `json:"final"`
}
```

## Configuration

### File Server Configuration

```yaml
# kscore-files configuration
server:
  nats:
    urls: ["nats://nats:4222"]
    credentials_file: /etc/kscore/nats.creds

  cluster_id: "production"
  instance_id: "files-1"

  # Worker settings
  workers: 10                    # Concurrent transfer handlers
  max_chunk_size: 1048576        # 1MB default
  max_file_size: 10737418240     # 10GB max

  # Rate limiting
  rate_limit:
    per_agent: "100MB/s"
    global: "1GB/s"
    concurrent_transfers: 100

# Storage backends
backends:
  # NATS Object Store (recommended for small-medium files)
  - name: nats-objects
    type: nats-object-store
    bucket: kscore-files
    priority: 1
    paths:
      - /configs/**
      - /certificates/**
      - /scripts/**
    max_file_size: 104857600     # 100MB limit for this backend

  # S3-compatible storage (for large files)
  - name: s3-packages
    type: s3
    bucket: kscore-packages
    region: us-east-1
    endpoint: ""                 # Empty for AWS, set for MinIO/GCS
    priority: 2
    paths:
      - /packages/**
      - /binaries/**
    credentials:
      access_key_env: AWS_ACCESS_KEY_ID
      secret_key_env: AWS_SECRET_ACCESS_KEY

  # Git repository (for versioned configs)
  - name: git-configs
    type: git
    url: git@github.com:org/configs.git
    branch: main
    ssh_key_file: /etc/kscore/git-deploy-key
    poll_interval: 60s
    priority: 3
    paths:
      - /gitops/**

  # Local filesystem (fallback)
  - name: local
    type: filesystem
    root: /var/lib/kscore/files
    priority: 10
    paths:
      - /**                      # Catch-all fallback

# Namespace configuration
namespaces:
  packages:
    path: /packages
    permissions:
      - agents: ["*"]
        actions: [read, list]
      - agents: ["admin-*"]
        actions: [read, write, list, delete]
    cache_ttl: 24h

  configs:
    path: /configs
    permissions:
      - agents: ["*"]
        actions: [read]
    cache_ttl: 1h
    notify_on_change: true

  uploads:
    path: /uploads
    permissions:
      - agents: ["*"]
        actions: [read, write, list]
    max_file_size: 1073741824    # 1GB
    quota_per_agent: 10737418240 # 10GB
    retention: 7d
```

### Agent File Client Configuration

```yaml
# Agent configuration for file retrieval
files:
  enabled: true
  cache_dir: /var/cache/kscore/files
  cache_size: 1073741824         # 1GB local cache
  chunk_size: 1048576            # 1MB
  retry_attempts: 3
  retry_delay: 5s
  verify_checksums: true

  # Prefer proxy agents for caching
  prefer_proxy: true
  proxy_timeout: 5s              # Fallback to file server if proxy slow
```

### Proxy Agent Cache Configuration

```yaml
# Proxy agent file cache configuration
file_cache:
  enabled: true
  cache_dir: /var/cache/kscore/file-cache
  max_size: 10737418240          # 10GB

  # Eviction policy
  eviction: lru
  min_free_space: 1073741824     # 1GB reserved

  # TTL settings
  default_ttl: 24h
  namespace_ttl:
    /packages: 168h              # 7 days
    /configs: 1h
    /scripts: 4h

  # Pre-warming
  warm_on_start:
    - /packages/common/**
    - /configs/base/**

  # Metrics
  metrics:
    enabled: true
    report_interval: 60s
```

## Technical Tasks

### Phase 1: Core Protocol & Local Backend (Weeks 1-2)

**T1.1: File Protocol Types**
- Define protobuf messages for file operations
- FileRequest, FileMetadata, FileChunk, FileAck
- Error types and status codes

**T1.2: File Server Core**
- Create `cmd/kscore-files/` binary
- NATS connection and subscription handling
- Request routing and worker pool
- Chunked streaming implementation

**T1.3: Local Filesystem Backend**
- Implement filesystem storage backend
- File reading with streaming
- Directory listing
- Checksum calculation (SHA-256)

**T1.4: Agent File Client**
- File request API in agent
- Chunk reassembly and verification
- Local file caching
- Resume support for interrupted transfers

### Phase 2: Cloud Storage Backends (Weeks 3-4)

**T2.1: S3-Compatible Backend**
- AWS S3 client integration
- Support for MinIO, GCS (S3-compatible mode)
- Streaming download (no full buffering)
- Multipart upload support

**T2.2: Azure Blob Backend**
- Azure Blob Storage client
- Block blob streaming
- SAS token support

**T2.3: Google Cloud Storage Backend**
- GCS native client
- Streaming operations
- Service account authentication

**T2.4: Backend Selection Logic**
- Path-based backend routing
- Priority-based fallback
- Health checking for backends

### Phase 3: NATS Object Store Backend (Weeks 5-6)

**T3.1: NATS Object Store Integration**
- JetStream Object Store client
- Put/Get/Delete operations
- Chunking handled by NATS
- Watch for changes

**T3.2: Object Store Optimization**
- Connection pooling
- Batch metadata queries
- Efficient listing

**T3.3: Object Store Replication**
- Multi-replica configuration
- Read from nearest replica
- Write consistency settings

### Phase 4: Git Repository Backend (Weeks 7-8)

**T4.1: Git Clone and Fetch**
- go-git integration
- SSH and HTTPS authentication
- Sparse checkout support

**T4.2: Branch and Tag Support**
- Map namespaces to branches
- Tag-based versioning
- Ref resolution

**T4.3: Webhook Integration**
- HTTP endpoint for webhooks
- GitHub/GitLab/Bitbucket support
- Immediate refresh on push

### Phase 5: Proxy Agent Caching (Weeks 9-10)

**T5.1: Cache Storage**
- File-based cache with index
- LRU eviction implementation
- TTL expiration handling

**T5.2: Cache Protocol**
- Request interception at proxy
- Cache hit/miss handling
- Passthrough for uncacheable

**T5.3: Cache Management**
- Invalidation messages
- Cache warm-up API
- Cache statistics

**T5.4: Proxy Discovery**
- Agents discover proxy with cache
- Nearest proxy selection
- Automatic fallback to file server

### Phase 6: Access Control & Security (Weeks 11-12)

**T6.1: Authentication**
- Agent identity verification (SPIFFE)
- Request signing
- Credential validation

**T6.2: Authorization**
- Namespace permissions
- Policy engine integration (OPA/CEL)
- ACL evaluation

**T6.3: Audit Logging**
- File access logging
- Download/upload tracking
- Integration with audit system

### Phase 7: High Availability & Scaling (Weeks 13-14)

**T7.1: Multi-Instance Support**
- Stateless file server design
- Load distribution via NATS queue groups
- Health check endpoints

**T7.2: Bandwidth Management**
- Per-agent rate limiting
- Global rate limiting
- Priority queues

**T7.3: Metrics & Monitoring**
- Transfer metrics (count, size, duration)
- Cache metrics (hit rate, size)
- Backend metrics (latency, errors)
- Prometheus exporter

### Phase 8: State Module Integration (Weeks 15-16)

**T8.1: File Module Enhancement**
- Support `source: kscore://` URLs
- Automatic checksum verification
- Version pinning

**T8.2: Package Module Enhancement**
- Package files from file server
- Repository mirroring support

**T8.3: Template Integration**
- Template files from file server
- Variable substitution

### Phase 9: CLI & Administration (Weeks 17-18)

**T9.1: File Server CLI**
- `kscore-files` admin commands
- Backend status
- Cache management

**T9.2: kscorectl Integration**
- `kscorectl files list`
- `kscorectl files get`
- `kscorectl files put`
- `kscorectl files delete`

**T9.3: File Upload Tool**
- Bulk upload utility
- Directory sync
- Checksum verification

### Phase 10: Documentation & Testing (Weeks 19-20)

**T10.1: Documentation**
- Architecture documentation
- Configuration reference
- Backend setup guides
- Troubleshooting guide

**T10.2: Unit Tests**
- Protocol tests
- Backend tests
- Cache tests

**T10.3: Integration Tests**
- End-to-end transfer tests
- Multi-backend tests
- Proxy cache tests

**T10.4: Performance Tests**
- Large file transfer benchmarks
- Concurrent transfer tests
- Cache performance tests

## Dependencies

- **Epic 1** (Core Infrastructure) - NATS, agent communication
- **Epic 4** (Event System) - File change notifications
- **Epic 6** (Policy Enforcement) - Access control
- **Epic 14** (NATS Mesh) - NATS communication patterns
- **Epic 17** (SPIFFE Identity) - Agent authentication
- **Epic 21** (Proxy Agents) - Proxy caching infrastructure

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Large file transfers overwhelm NATS | High | Medium | Chunked streaming, rate limiting, separate NATS connections |
| Storage backend failures | High | Medium | Multiple backends with fallback, health checks |
| Cache inconsistency | Medium | Medium | TTL-based expiration, explicit invalidation, checksums |
| Network partition during transfer | Medium | Medium | Resume support, chunk-level checkpoints |
| Storage costs for large deployments | Medium | Low | Tiered storage, compression, deduplication |
| Proxy cache fills disk | Medium | Medium | LRU eviction, reserved space, monitoring |

## Security Considerations

### Data in Transit
- All transfers over NATS (already mTLS encrypted)
- Per-chunk checksums prevent tampering
- Request signing prevents replay attacks

### Data at Rest
- Backend-specific encryption (S3 SSE, etc.)
- NATS Object Store encryption
- Local cache encryption (optional)

### Access Control
- Namespace-based permissions
- Agent identity verification
- Audit logging for compliance

### Sensitive Files
- Certificates and keys in restricted namespace
- Automatic expiration for uploaded credentials
- No file content in logs

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `kscore_files_requests_total` | Counter | Total file requests by namespace, status |
| `kscore_files_bytes_transferred_total` | Counter | Total bytes transferred by direction |
| `kscore_files_transfer_duration_seconds` | Histogram | Transfer duration by size bucket |
| `kscore_files_active_transfers` | Gauge | Currently active transfers |
| `kscore_files_backend_requests_total` | Counter | Backend requests by backend, status |
| `kscore_files_backend_latency_seconds` | Histogram | Backend operation latency |
| `kscore_files_cache_hits_total` | Counter | Proxy cache hits |
| `kscore_files_cache_misses_total` | Counter | Proxy cache misses |
| `kscore_files_cache_size_bytes` | Gauge | Current cache size |
| `kscore_files_cache_evictions_total` | Counter | Cache evictions |

## Testing Strategy

### Unit Tests
- Protocol message serialization
- Backend implementations (mocked storage)
- Cache eviction logic
- Checksum verification

### Integration Tests
- Full transfer flow with real NATS
- Multi-backend routing
- Proxy cache behavior
- Resume interrupted transfers

### Performance Tests
- 1GB file transfer latency
- 100 concurrent transfers
- Cache hit rate under load
- Backend failover time

### Chaos Tests
- Backend unavailability
- NATS partition during transfer
- Proxy agent failure mid-transfer
- Disk full scenarios

## Definition of Done

- [ ] All user stories completed and accepted
- [ ] Unit test coverage >80%
- [ ] Integration tests passing
- [ ] Performance benchmarks met
- [ ] Documentation complete
- [ ] Security review completed
- [ ] Grafana dashboard for file distribution
- [ ] Alert rules for transfer failures and cache issues
