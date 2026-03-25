---
title: "Configuration Reference"
weight: 3
description: >
  Complete configuration reference for control plane, agents, and all subsystems
---

## Overview

Keystone Core components are configured using YAML files. This reference documents all configuration options.

**Configuration Files**:

- Control Plane: `/etc/keystone-core/server.yaml`
- Agent: `/etc/keystone-core/agent.yaml`
- CLI: `~/.keystone-core/config.yaml`

**Note**: When running without `--config`, binaries search for `keystone-core.yaml` in `/etc/keystone-core/`, `~/.keystone-core/`, and the current directory. The package-installed systemd services explicitly pass the config file path.

**Why `/etc/keystone-core` instead of `/etc/keystone-core`?**

- Clearer and less ambiguous for operators and in multi-product environments.
- Aligns with the full product/package name and systemd unit naming.
- Reduces support friction by keeping docs and paths consistent.

## Control Plane Configuration

Complete configuration reference for `kscore-server`.

### Basic Configuration

```yaml
# /etc/keystone-core/server.yaml

# API Server
# Note: api.listen and api.grpc_listen are convenience aliases for server.httplisten
# and server.grpclisten. Similarly, api.cors.* and api.rate_limit.* are aliases for
# the top-level cors.* and ratelimit.* settings.
api:
  listen: "0.0.0.0:8080"           # HTTP API listen address
  grpc_listen: "0.0.0.0:9090"       # gRPC API listen address
  cors:
    enabled: true                   # Enable CORS
    allowedorigins: ["*"]           # Allowed origins
    allowedmethods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowedheaders: ["Content-Type", "Authorization", "X-API-Key"]
    allowcredentials: false
    maxage: 86400                   # Preflight cache max age (seconds)
  rate_limit:
    enabled: true                   # Enable rate limiting (HTTP and gRPC)
    requestsperminute: 100          # Requests per minute per key
    burst: 20                       # Burst capacity
    keyextractor: "ip"              # ip, apikey, header (HTTP only; gRPC always uses peer IP)
    headername: "X-API-Key"         # Header to use when keyextractor: header

# Server settings (advanced)
# These settings are NOT available under api.* and must use the server.* prefix
server:
  listenaddrs: []                   # Optional multi-address binding
                                    # Example: ["[::]:8080", "0.0.0.0:8080"]
  addressfamily: "prefer_ipv4"      # prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
  allowinsecurenonloopback: false   # Allow non-loopback listen without TLS (dev only)

# TLS Configuration (applies to API server)
tls:
  enabled: false                    # Enable TLS
  cert_file: ""                     # TLS certificate file
  key_file: ""                      # TLS key file
  ca_file: ""                       # CA certificate for client auth
  min_version: "1.3"                # Minimum TLS version (1.2 or 1.3)

# NATS Configuration
nats:
  mode: "embedded"                  # embedded, external, leaf
  # Listen format: "host:port" for embedded mode
  # Examples: "0.0.0.0:4222" (all interfaces), "127.0.0.1:4222" (localhost only)
  #           "[::]:4222" (IPv6 all interfaces), "[::1]:4222" (IPv6 localhost)
  listen: "0.0.0.0:4222"           # NATS listen address (embedded mode)
  url: "nats://nats:4222"           # NATS URL(s) (external/leaf; comma-separated allowed)
  token: ""                         # NATS auth token (mutually exclusive with credential)
  credential: ""                    # NATS credentials file (NKey/JWT)
  max_reconnects: -1                # -1 = unlimited, 0 = disabled
  reconnect_wait: "2s"
  jetstream:
    enabled: true                   # Enable JetStream
    storedir: "/var/lib/keystone-core/nats"  # Default: ./data/nats (relative to working directory)
    max_storage: "10GB"             # Max total storage (bytes)
  embedded:
    listen: "0.0.0.0:4222"           # Combined host:port (overrides host/port)
    host: "127.0.0.1"               # Host for embedded NATS (default: 127.0.0.1)
    port: 4222                       # Port for embedded NATS
    enable_jetstream: true           # Enable JetStream in embedded mode
    storedir: "./data/nats"          # JetStream storage directory
    max_memory: 1073741824           # Max memory in bytes (default: 1GB)
    max_connections: 1000            # Max concurrent connections
    addressfamily: "prefer_ipv4"    # prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
    leaf_node_urls: []               # Parent leaf URLs when mode: leaf

# State Storage
storage:
  backend: "sqlite"                 # sqlite, postgresql
  sqlite:
    path: "/var/lib/keystone-core/keystone-core.db"  # Default: ./data/keystone-core.db
    wal: true                        # Enable WAL mode (default: true)
    busy_timeout: 5000               # Busy timeout in milliseconds (default: 5000)
  postgresql:
    dsn: ""                          # Connection string (e.g., "postgres://user:pass@host:5432/db?sslmode=verify-full")
    max_open_conns: 25               # Maximum open connections (default: 25)
    max_idle_conns: 5                # Maximum idle connections (default: 5)
    conn_max_lifetime: "5m"          # Connection max lifetime (default: 5m)

# Logging
# Note: File output is not supported - use journald, container log drivers, or syslog
logging:
  level: "info"                     # debug, info, warn, error
  format: "json"                    # json, logfmt, text
  output: "stdout"                  # stdout, syslog
  include_caller: false             # Include caller file:line
  include_stacktrace: false         # Include stack traces for errors
  syslog:                           # Syslog settings (when output: syslog)
    network: "unix"                 # unix, udp, tcp, tcp+tls
    address: "/dev/log"             # /dev/log (unix) or host:port
    facility: "daemon"              # local0-local7, daemon, user
    app_name: "kscore-server"       # Application name
    tls:                            # TLS settings for tcp+tls
      enabled: false
      cacert: ""
      cert: ""
      key: ""
      skipverify: false
      minversion: "1.3"             # Minimum TLS version (1.2 or 1.3)

# Metrics
metrics:
  enabled: true
  listen: ":8080"                   # Metrics endpoint (usually same as API)
  path: "/metrics"                  # Metrics path
  includegometrics: true
  includeprocessmetrics: true

# Tracing
tracing:
  enabled: false
  exporter: "otlp"                  # otlp, jaeger, zipkin
  endpoint: "http://jaeger:4318"
  sampling:
    strategy: "ratio"               # always, never, ratio
    ratio: 0.1                      # Sample rate (0.0-1.0)
  resource:
    service.name: "kscore-server"
    service.version: "1.0.0"
    deployment.environment: "production"

# Health Checks
health:
  enabled: true
  startupgraceperiod: "30s"        # Grace period on startup
  checkinterval: "10s"              # Health check interval
  checks:
    nats:
      enabled: true
      timeout: "5s"
    database:
      enabled: true
      timeout: "5s"
    agents:
      enabled: true
      minhealthy: 0.8               # 80% of agents must be healthy
```

### Agent Management

Server-side settings for monitoring and managing connected agents.

```yaml
agentmanagement:
  heartbeatinterval: "30s"           # Interval agents should send heartbeats
  heartbeattimeout: "60s"            # Time before marking an agent stale
  stalethreshold: 3                  # Missed heartbeats before marking offline
  monitorinterval: "10s"             # Control plane agent health check interval
  metadatarefresh: "5m"              # Agent system metadata refresh interval
  maxconcurrentcommands: 0           # Max concurrent commands per agent (0 = unlimited)
```

| Setting | Default | Env Override |
|---------|---------|-------------|
| `heartbeatinterval` | `30s` | `KSCORE_AGENT_MGMT_HEARTBEAT_INTERVAL` |
| `heartbeattimeout` | `60s` | `KSCORE_AGENT_MGMT_HEARTBEAT_TIMEOUT` |
| `stalethreshold` | `3` | `KSCORE_AGENT_MGMT_STALE_THRESHOLD` |
| `monitorinterval` | `10s` | `KSCORE_AGENT_MGMT_MONITOR_INTERVAL` |
| `metadatarefresh` | `5m` | `KSCORE_AGENT_MGMT_METADATA_REFRESH` |
| `maxconcurrentcommands` | `0` | `KSCORE_AGENT_MGMT_MAX_CONCURRENT_COMMANDS` |

Validation rules:

- `heartbeattimeout` must be >= `heartbeatinterval`
- All duration and integer values must be non-negative

### Command Execution

Settings for remote command execution targeting agents.

```yaml
execution:
  defaulttimeout: "5m"               # Default command timeout
  maxtimeout: "1h"                   # Maximum allowed timeout (requests clamped)
  batchsize: 10                      # Agents per batch in batch operations
  batchdelay: "0s"                   # Delay between batch groups
  streamingbuffer: 100               # Buffer size for streaming responses
  resultretention: "24h"             # How long to keep execution results
```

| Setting | Default | Env Override |
|---------|---------|-------------|
| `defaulttimeout` | `5m` | `KSCORE_EXEC_DEFAULT_TIMEOUT` |
| `maxtimeout` | `1h` | `KSCORE_EXEC_MAX_TIMEOUT` |
| `batchsize` | `10` | `KSCORE_EXEC_BATCH_SIZE` |
| `batchdelay` | `0s` | `KSCORE_EXEC_BATCH_DELAY` |
| `streamingbuffer` | `100` | `KSCORE_EXEC_STREAMING_BUFFER` |
| `resultretention` | `24h` | `KSCORE_EXEC_RESULT_RETENTION` |

Validation rules:

- `defaulttimeout` cannot exceed `maxtimeout`
- All values must be non-negative

### State Management

Settings for declarative state management and drift detection.

```yaml
statemanagement:
  defaulttimeout: "10m"              # Default state apply timeout
  maxconcurrent: 5                   # Max concurrent state apply operations
  driftcheckinterval: "0s"           # Automatic drift check interval (0 = disabled)
  resultretention: "168h"            # How long to keep state apply results (7 days)
```

| Setting | Default | Env Override |
|---------|---------|-------------|
| `defaulttimeout` | `10m` | `KSCORE_STATE_DEFAULT_TIMEOUT` |
| `maxconcurrent` | `5` | `KSCORE_STATE_MAX_CONCURRENT` |
| `driftcheckinterval` | `0s` | `KSCORE_STATE_DRIFT_CHECK_INTERVAL` |
| `resultretention` | `168h` | `KSCORE_STATE_RESULT_RETENTION` |

Validation rules:

- All values must be non-negative
- Set `driftcheckinterval` to `0` to disable automatic drift detection

### Event System

JetStream-backed event bus settings for the reactor system.

```yaml
events:
  enabled: true                      # Enable the event system
  retention: "168h"                  # Max event age (7 days)
  maxbytes: 10737418240              # Max event storage (10GB)
  maxmessages: 1000000               # Max stored events (1M)
  publisherbuffersize: 256           # Publisher channel buffer
  subscriberbuffersize: 256          # Subscriber channel buffer
  subscriberackwait: "30s"           # Subscriber acknowledgment timeout
```

| Setting | Default | Env Override |
|---------|---------|-------------|
| `enabled` | `true` | `KSCORE_EVENTS_ENABLED` |
| `retention` | `168h` | `KSCORE_EVENTS_RETENTION` |
| `maxbytes` | `10737418240` | `KSCORE_EVENTS_MAX_BYTES` |
| `maxmessages` | `1000000` | `KSCORE_EVENTS_MAX_MESSAGES` |

Validation rules:

- All values must be non-negative
- When `enabled: false`, no JetStream publisher is created

### Policy Enforcement

```yaml
# Policy enforcement settings
policy:
  enabled: true
  engine: "both"                    # opa, cel, both
  enforcement_mode: "enforce"       # enforce, audit, warn
  policies:                         # Built-in policy definitions
    - id: "deny-ssh-root"
      name: "Deny SSH Root Login"
      description: "Block SSH root login"
      type: "opa"                   # opa, cel
      category: "security"          # security, compliance, operational
      severity: "high"              # low, medium, high, critical
      enabled: true
      code: |
        package kscore.security
        deny[msg] { input.resource.type == "ssh" }
```

### GitOps Integration

Automatic git repository synchronization for GitOps workflows.

```yaml
gitops:
  gitsync:
    enabled: false                   # Enable git-sync
    repositories:
      - url: "https://github.com/myorg/infra.git"
        branch: "main"              # Branch to track (default: main)
        interval: "5m"              # Poll interval (default: 5m)
        path: ""                    # Subdirectory to sync (empty = root)
```

Validation rules:

- When `gitsync.enabled: true`, each repository must have a non-empty `url`
- `interval` must be non-negative

### Webhook Receiver

```yaml
webhook:
  enabled: false
  port: 8082
  path: "/webhooks"
  authtype: "none"                  # none, hmac, bearer
  hmacsecret: ""
  bearertoken: ""
  handlers: ["argocd", "flux", "github", "gitlab"]
```

### Outbound Webhooks

Outbound webhook subscriptions deliver internal events to external HTTP endpoints.

```yaml
webhook:
  outbound:
    enabled: false                     # Enable outbound webhook system
    max_retries: 3                     # Default max retries per delivery
    retry_backoff: 1s                  # Initial retry backoff duration
    timeout: 10s                       # HTTP request timeout
    max_payload_size: 1048576          # Max payload size in bytes (1MB)
    delivery_retention: 168h           # Delivery history retention (7 days)
    circuit_breaker:
      failure_threshold: 5             # Consecutive 5xx/errors before opening circuit
      success_threshold: 2             # Consecutive successes in half-open to reclose
      open_duration: 30s               # How long circuit stays open before probing
```

| Setting | Default | Env Override | Description |
|---------|---------|-------------|-------------|
| `webhook.outbound.enabled` | `false` | `KSCORE_WEBHOOK_OUTBOUND_ENABLED` | Enable outbound webhooks |
| `webhook.outbound.max_retries` | `3` | `KSCORE_WEBHOOK_OUTBOUND_MAX_RETRIES` | Max delivery retry attempts |
| `webhook.outbound.retry_backoff` | `1s` | `KSCORE_WEBHOOK_OUTBOUND_RETRY_BACKOFF` | Initial backoff between retries (exponential) |
| `webhook.outbound.timeout` | `10s` | `KSCORE_WEBHOOK_OUTBOUND_TIMEOUT` | HTTP timeout for deliveries |
| `webhook.outbound.max_payload_size` | `1048576` | `KSCORE_WEBHOOK_OUTBOUND_MAX_PAYLOAD_SIZE` | Maximum payload size in bytes |
| `webhook.outbound.delivery_retention` | `168h` | `KSCORE_WEBHOOK_OUTBOUND_DELIVERY_RETENTION` | How long to keep delivery history |
| `webhook.outbound.circuit_breaker.failure_threshold` | `5` | — | Consecutive failures before opening circuit |
| `webhook.outbound.circuit_breaker.success_threshold` | `2` | — | Successes in half-open to reclose |
| `webhook.outbound.circuit_breaker.open_duration` | `30s` | — | Duration circuit stays open before probe |

**Validation** (when `enabled: true`):

- `max_retries` must be >= 0
- `timeout` must be > 0
- `max_payload_size` must be > 0

### Authorization

RBAC authorization settings that control access to gRPC and REST API methods.

```yaml
auth:
  authorization:
    enabled: true                    # Enable RBAC checks (default: true)
    defaultdeny: true                # Deny unmapped methods (default: true)
```

| Setting | Default | Env Override |
|---------|---------|-------------|
| `auth.authorization.enabled` | `true` | `KSCORE_AUTHZ_ENABLED` |
| `auth.authorization.defaultdeny` | `true` | `KSCORE_AUTHZ_DEFAULT_DENY` |

When `enabled: false`, all authenticated requests are allowed regardless of role.
When `defaultdeny: false`, unmapped methods are allowed for any authenticated user.
When `defaultdeny: true`, unmapped methods require admin role.

### API Authentication (Control Plane)

```yaml
auth:
  enabled: true
  type: "apikey"                    # apikey, jwt, mtls, multi
  bypass_methods:
    - "/keystone.core.v1.ControlPlaneService/GetServerStatus"

  apikey:
    headername: "X-API-Key"
    metadatakey: "x-api-key"
    keys:
      "<your-api-key>":
        name: "admin"
        role: "admin"               # admin, operator, readonly
        enabled: true
        expires_at: ""              # RFC3339 timestamp

  jwt:
    secret: ""                      # HS256 secret (or use publickeyfile)
    publickeyfile: ""               # RS256/ES256 public key
    issuer: ""
    audience: ""
    roleclaim: "role"

  mtls:
    requireclientcert: true
    certroles:
      "*.admin.example.com": "admin"
```

### TLS Client Settings

```yaml
tls:
  enabled: false
  cert_file: ""
  key_file: ""
  ca_file: ""
  insecure_skip_verify: false       # Dev/test only
  min_version: "1.3"                # Minimum TLS version (1.2 or 1.3)
```

### Cluster Configuration

```yaml
# High availability cluster settings
cluster:
  enabled: false                        # Enable clustering (default: false)
  member_id: ""                         # Unique member ID (auto-generated if empty)
  cluster_name: "kscore"                # Cluster name (all members must match)
  advertise_address: ""                 # Address for other members (auto-detected if empty)
  etcd_endpoints:                       # External etcd endpoints
    - "etcd1.example.com:2379"
    - "etcd2.example.com:2379"
    - "etcd3.example.com:2379"
  etcd_prefix: "/kscore"               # Key prefix in etcd

  # Full etcd configuration (advanced)
  etcd:
    mode: "embedded"                  # embedded, external
    endpoints:                        # External etcd endpoints
      - "etcd1.example.com:2379"
      - "etcd2.example.com:2379"
      - "etcd3.example.com:2379"
    dial_timeout: "5s"
    request_timeout: "10s"
    tls:
      enabled: false
      cert_file: ""
      key_file: ""
      ca_file: ""
      min_version: "1.3"            # Minimum TLS version (1.2 or 1.3)

  # Membership settings
  membership:
    heartbeat_interval: "5s"          # Inter-member heartbeat
    heartbeat_timeout: "15s"          # Timeout for member failure
    min_quorum: 2                     # Minimum members for quorum

  # Leader election
  election:
    enabled: true
    lease_duration: "15s"
    renew_deadline: "10s"
    retry_period: "2s"

  # Agent sharding (work distribution)
  sharding:
    enabled: true
    virtual_nodes: 100                # Virtual nodes for consistent hashing
    rebalance_delay: "10s"            # Delay before rebalancing
```

**Clustering defaults:**

| Field | Default | Env Var |
|-------|---------|---------|
| `cluster.enabled` | `false` | `KSCORE_CLUSTER_ENABLED` |
| `cluster.member_id` | `""` (auto-generated) | `KSCORE_CLUSTER_MEMBER_ID` |
| `cluster.cluster_name` | `"kscore"` | `KSCORE_CLUSTER_NAME` |
| `cluster.advertise_address` | `""` (auto-detected) | `KSCORE_CLUSTER_ADVERTISE_ADDRESS` |
| `cluster.etcd_endpoints` | `[]` | `KSCORE_CLUSTER_ETCD_ENDPOINTS` |
| `cluster.etcd_prefix` | `"/kscore"` | `KSCORE_CLUSTER_ETCD_PREFIX` |

**Validation rules** (when `cluster.enabled=true`):

- `cluster_name` is required
- `etcd_endpoints` must have at least one entry

When clustering is enabled, the cluster REST API handler (`/api/v1/cluster/*`) is wired with real etcd, membership, leader election, and health monitoring dependencies. When disabled, the handler returns `503 Service Unavailable` for all endpoints.

The Schedule REST API (`/api/v1/schedules/*` and `/api/v1/maintenance/windows/*`) also requires cluster mode. The schedule and maintenance window managers use the etcd-backed schedule store. When `cluster.enabled=false`, schedule endpoints return `503 Service Unavailable`.

### Secrets

```yaml
# Secrets management settings
secrets:
  enabled: false                         # Enable secrets service (default: false)
  default_backend: "vault"               # Default secrets backend name
  cache_enabled: false                   # Enable in-memory secret caching
  cache_ttl: "5m"                        # Cache entry TTL (default: 5m)
```

**Secrets defaults:**

| Field | Default | Env Var |
|-------|---------|---------|
| `secrets.enabled` | `false` | `KSCORE_SECRETS_ENABLED` |
| `secrets.default_backend` | `""` | `KSCORE_SECRETS_DEFAULT_BACKEND` |
| `secrets.cache_enabled` | `false` | `KSCORE_SECRETS_CACHE_ENABLED` |
| `secrets.cache_ttl` | `5m` | `KSCORE_SECRETS_CACHE_TTL` |

**Validation rules** (when `secrets.enabled=true`):

- `default_backend` is required
- `cache_ttl` cannot be negative

When secrets is enabled, the gRPC SecretsService is wired with a real `SecretBroker` built via `BrokerBuilder`. When disabled, SecretsService RPCs return `codes.Unavailable`.

### Kubernetes Operator

```yaml
# Kubernetes operator settings (watches CRDs, runs reconciliation)
operator:
  enabled: false                        # Enable the Kubernetes operator
  namespace: ""                         # Restrict to a single namespace (empty = all)
  leader_election: true                 # Enable leader election for HA
  leader_election_id: "kscore-operator" # Lease name for leader election
  reconcile_interval: "1m"              # Periodic reconciliation interval
  max_concurrent_reconciles: 3          # Maximum parallel reconciliations
```

| Setting | Default | Env Var | Description |
|---------|---------|---------|-------------|
| `operator.enabled` | `false` | `KSCORE_OPERATOR_ENABLED` | Enable the Kubernetes operator |
| `operator.namespace` | `""` | `KSCORE_OPERATOR_NAMESPACE` | Namespace restriction (empty = all) |
| `operator.leader_election` | `true` | `KSCORE_OPERATOR_LEADER_ELECTION` | Enable leader election |
| `operator.leader_election_id` | `kscore-operator` | `KSCORE_OPERATOR_LEADER_ELECTION_ID` | Lease name for leader election |
| `operator.reconcile_interval` | `1m` | `KSCORE_OPERATOR_RECONCILE_INTERVAL` | Periodic reconciliation interval (min 10s) |
| `operator.max_concurrent_reconciles` | `3` | `KSCORE_OPERATOR_MAX_CONCURRENT_RECONCILES` | Max parallel reconciliations (min 1) |

### Identity Configuration

```yaml
# SPIFFE identity settings
identity:
  enabled: true
  trust_domain: "kscore.local"        # SPIFFE trust domain

  # Identity provider configuration
  provider:
    type: "embedded"                  # embedded, spire, aws, gcp, azure, istio, consul, linkerd

    # SPIRE Workload API (when type: spire)
    spire:
      agent_socket_path: "/run/spire/agent/sockets/agent.sock"
      server_address: ""              # SPIRE server address for admin operations
      trust_domain: ""                # Must match SPIRE server trust domain

    # AWS identity provider (when type: aws)
    aws:
      region: ""                      # AWS region (auto-detected if empty)
      role_arn: ""                    # IAM role ARN for IRSA
      use_imdsv2: true                # Require IMDSv2 for instance identity

    # GCP identity provider (when type: gcp)
    gcp:
      project_id: ""                  # GCP project ID (auto-detected if empty)
      service_account_email: ""       # Service account to impersonate

    # Azure identity provider (when type: azure)
    azure:
      tenant_id: ""                   # Azure AD tenant ID
      client_id: ""                   # Managed identity client ID

  # SVID (X.509 certificate) configuration
  svid:
    default_ttl: "1h"                 # Default SVID TTL
    max_ttl: "24h"                    # Maximum allowed SVID TTL
    rotation_interval: "30s"          # How often to check for rotation
    grace_period: "10m"               # Keep serving with expired SVID

  # Attestation configuration
  attestation:
    allowed_attestors:                # Enabled attestors
      - "join_token"
    allow_none: false                 # Allow "none" attestor (dev only)

    # Join token attestor
    join_token:
      enabled: true
      default_ttl: "5m"               # Default token TTL
      max_ttl: "24h"                  # Maximum token TTL
      token_length: 32                # Length of generated tokens
      one_time_use: true              # Single-use tokens
      cleanup_interval: "1h"          # Cleanup expired tokens interval

    # AWS attestor (for embedded provider)
    aws:
      enabled: false
      allowed_account_ids: []         # Restrict by AWS account IDs
      allowed_regions: []             # Restrict by regions
      allowed_instance_tags: {}       # Restrict by instance tags

    # GCP attestor (for embedded provider)
    gcp:
      enabled: false
      allowed_project_ids: []         # Restrict by project IDs
      allowed_zones: []               # Restrict by zones
      allowed_service_accounts: []    # Restrict by service accounts

    # Azure attestor (for embedded provider)
    azure:
      enabled: false
      allowed_subscription_ids: []    # Restrict by subscription IDs
      allowed_resource_groups: []     # Restrict by resource groups

  # CA configuration (for embedded provider)
  ca:
    key_type: "ecdsa-p256"            # ecdsa-p256, ecdsa-p384, rsa-2048, rsa-4096
    root_ca_ttl: "87600h"             # Root CA TTL (10 years)
    signing_ca_ttl: "8760h"           # Signing CA TTL (1 year)
    rotate_signing_ca_before: "720h"  # Rotate signing CA 30 days before expiry
    storage_path: "data/identity/ca"  # CA key storage path
    encryption_key: ""                # Encrypt CA keys at rest (recommended)

  # Trust federation
  federation:
    enabled: false
    bundle_endpoint: ""               # Expose trust bundle at this endpoint
    refresh_interval: "1h"            # Refresh external bundles interval
    trusted_domains:                  # External trust domains
      - domain: "partner.example.com"
        bundle_endpoint: "https://partner.example.com/.well-known/spiffe/bundle"
        policy:
          allowed_paths: ["/ns/*"]    # Allowed SPIFFE paths
          denied_paths: []            # Denied SPIFFE paths
```

## Telemetry Gateway Configuration

Configuration reference for `kscore-telemetry-gateway`.

```yaml
# /etc/keystone-core/gateway.yaml

# NATS connection
nats:
  urls:
    - "nats://localhost:4222"
  cluster: "default"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""
    min_version: "1.3"              # Minimum TLS version (1.2 or 1.3)
  credentials_file: ""
  max_reconnects: -1
  reconnect_wait: "2s"
  reconnect_jitter: "1s"

# HTTP server
server:
  listen: "0.0.0.0:9091"
  metrics_path: "/metrics"
  health_path: "/health"
  ready_path: "/ready"
  federate_path: "/federate"
  read_timeout: "30s"
  write_timeout: "30s"

# Metrics gateway
metrics:
  enabled: true
  subject: "kscore.metrics.>"
  stale_timeout: "5m"
  labels:
    add:
      environment: "production"
    drop:
      - "__tmp_*"
    rewrite:
      - source: "old_label"
        target: "new_label"
  cardinality:
    max_series: 100000
    max_labels_per_series: 50
    drop_high_cardinality: true
  remote_write:
    enabled: false
    url: "http://prometheus:9090/api/v1/write"
    batch_size: 1000
    flush_interval: "10s"
    auth:
      type: "none"                    # none, basic, bearer
      username: ""
      password: ""
      token: ""
    retry:
      enabled: true
      max_retries: 3
      initial_interval: "1s"
      max_interval: "30s"
  federation:
    enabled: true
    match: []

# Logs gateway
logs:
  enabled: true
  subject: "kscore.logs.>"
  levels:
    - debug
    - info
    - warn
    - error
  sources:
    include: []
    exclude: []
  loki:
    enabled: false
    url: "http://loki:3100/loki/api/v1/push"
    batch_size: 100
    flush_interval: "5s"
    tenant_id: ""
    auth:
      type: "none"
      username: ""
      password: ""

# Traces gateway
traces:
  enabled: true
  subject: "kscore.traces.>"
  sampling:
    enabled: true
    rate: 0.1                         # 10% sampling
    error_always: true                # Always sample errors
    slow_always: true                 # Always sample slow traces
    slow_threshold: "5s"
  otlp:
    enabled: false
    endpoint: "http://tempo:4318/v1/traces"
    compression: "gzip"
    timeout: "10s"
    auth:
      type: "none"

# High availability
ha:
  enabled: false
  instance_id: ""                     # Auto-generated if empty
  shards: 1                           # Number of shards
```

## MCP Server Configuration

Configuration reference for `kscore-mcp`, the MCP server for AI-assisted operations.

`kscore-mcp` is configured via a standalone YAML file passed with `--config`. It does not share the control plane's `server.yaml`.

```yaml
# mcp.yaml

# Upstream kscore-server connection
server:
  address: "localhost:50051"              # gRPC address of kscore-server (required)
  rest_base_url: "http://localhost:8080"  # REST base URL for HTTP-based tools

# Authentication (operator's own credentials, passed through to kscore-server)
auth:
  method: "apikey"                        # apikey, jwt, or mtls (required)

  # API key authentication (when method: apikey)
  api_key: ""                             # API key value (required when method=apikey)

  # JWT authentication (when method: jwt)
  jwt_token: ""                           # JWT token (required when method=jwt)

  # mTLS authentication (when method: mtls)
  tls_cert: ""                            # Client certificate path (required when method=mtls)
  tls_key: ""                             # Client key path (required when method=mtls)
  tls_ca_cert: ""                         # CA certificate for server verification

# Feature controls
features:
  default_profile: "ops_safe"             # Capability profile: read_only, ops_safe, ops_admin
  max_target_count: 50                    # Max agents per exec_run call (0 = unlimited)
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `server.address` | Yes | — | gRPC address of the upstream `kscore-server` |
| `server.rest_base_url` | No | `""` | REST base URL for HTTP-based tool calls |
| `auth.method` | Yes | — | Authentication method: `apikey`, `jwt`, or `mtls` |
| `auth.api_key` | When `apikey` | `""` | API key for authentication |
| `auth.jwt_token` | When `jwt` | `""` | JWT token for authentication |
| `auth.tls_cert` | When `mtls` | `""` | Client certificate file path |
| `auth.tls_key` | When `mtls` | `""` | Client private key file path |
| `auth.tls_ca_cert` | No | `""` | CA certificate for server TLS verification |
| `features.default_profile` | No | `ops_safe` | Capability profile controlling tool visibility |
| `features.max_target_count` | No | `0` | Maximum agents targeted per `exec_run` (0 = unlimited) |

**Capability profiles** control which MCP tools are registered. Server-side RBAC remains authoritative regardless of profile.

| Profile | Tools Available |
|---------|----------------|
| `read_only` | Agent list/show/health, exec status/history, state check/drift/history, event query/stats, runbook list/status, cluster status |
| `ops_safe` | All `read_only` tools plus exec run, runbook execute |
| `ops_admin` | All tools including state apply |

**Validation rules:**

- `server.address` is required
- `auth.method` is required and must be `apikey`, `jwt`, or `mtls`
- Credentials matching the auth method must be provided
- `features.max_target_count` must be non-negative
- `features.default_profile`, if set, must be `read_only`, `ops_safe`, or `ops_admin`

TLS is enforced at minimum version 1.3 when `tls_ca_cert` or mTLS credentials are provided.

See also: [MCP Setup Guide](../../guides/mcp-setup/), [MCP Security](../../guides/mcp-security/)

## Module Registry Configuration

Configuration reference for `kscore-registry`.

`kscore-registry` is configured via CLI flags and environment variables. The following YAML mirrors `deploy/config/registry.yaml` used by deployment tooling (the server does not read this file directly).

```yaml
# /etc/keystone-core/registry.yaml

server:
  listen_address: "0.0.0.0:8081"

storage:
  data_dir: "/var/lib/keystone-core/registry"
  max_upload_size: 104857600          # 100MB

auth:
  enabled: false
  api_key: ""                         # Use with X-API-Key or Authorization: Bearer

security:
  read_only: false
  cors_enabled: true
  cors_origins: []                    # Comma-separated list, empty = deny

logging:
  level: "info"
  format: "json"
  output: "stdout"

telemetry:
  enabled: true
  prometheus:
    enabled: true
    path: "/metrics"
```

Flag mapping:

| YAML field | CLI flag/env |
|-----------|--------------|
| `server.listen_address` | `--listen` |
| `storage.data_dir` | `--data` |
| `storage.max_upload_size` | `--max-upload-size` |
| `auth.api_key` | `--api-key` or `KSCORE_REGISTRY_API_KEY` |
| `security.read_only` | `--readonly` |
| `security.cors_enabled` | `--cors` |
| `security.cors_origins` | `--cors-origins` |

Notes:

- `auth.enabled` is deployment metadata only; the server enables write auth when `api_key` is provided.
- `auth.api_key_file` is not supported by `kscore-registry` flags (use `KSCORE_REGISTRY_API_KEY`).
- `logging.*` and `telemetry.*` are deployment reference only; `kscore-registry` does not expose these as CLI flags.

Environment override:

```
KSCORE_REGISTRY_API_KEY="your-secret-api-key"
```

## File Server Configuration

Configuration reference for `kscore-files`.

```yaml
# /etc/keystone-core/files.yaml

# Server settings
server:
  cluster_id: "default"
  instance_id: ""                     # Auto-generated if empty
  workers: 10                         # Concurrent transfer handlers
  max_chunk_size: 65536               # 64KB chunks
  max_file_size: 1073741824           # 1GB max file size
  request_timeout: "5m"

# NATS connection
nats:
  url: "nats://localhost:4222"
  token: ""                           # Auth token
  username: ""                        # Username/password auth
  password: ""
  tls_cert_file: ""                   # mTLS client certificate
  tls_key_file: ""                    # mTLS client key
  tls_ca_file: ""                     # CA certificate

# Storage backends (array — multiple backends can be configured)
backends:
  # Local filesystem backend
  - name: "local-files"
    type: "local"
    root_path: "/var/lib/keystone-core/files"
    paths: ["/configs", "/packages"]  # Optional: restrict to specific paths
    read_only: false

  # S3 backend
  - name: "s3-packages"
    type: "s3"
    bucket: "my-kscore-files"
    region: "us-east-1"
    prefix: "files/"
    endpoint: ""                      # For MinIO/compatible
    access_key_id: ""
    secret_access_key: ""
    profile: ""                       # AWS profile name
    use_path_style: false
    read_only: false

  # GCS backend
  - name: "gcs-assets"
    type: "gcs"
    bucket: "my-kscore-files"
    prefix: "files/"
    project: ""
    credentials_file: ""              # Path to service account JSON

  # Azure Blob backend
  - name: "azure-configs"
    type: "azure"
    container: "kscore-files"
    account_name: ""
    account_key: ""
    connection_string: ""             # Alternative to account_name/account_key
    prefix: ""

  # Git backend
  - name: "git-configs"
    type: "git"
    url: "https://github.com/myorg/configs.git"
    branch: "main"
    local_path: "/var/lib/keystone-core/git-files"
    username: ""                      # HTTPS auth
    password: ""
    ssh_key_file: ""                  # SSH auth
    auto_pull: true
    pull_interval: "5m"

  # NATS object store backend
  - name: "nats-store"
    type: "nats"
    bucket_name: "kscore-files"       # JetStream object store bucket
```

## Proxy Agent Configuration

Configuration reference for proxy agents that manage devices via SSH, SNMP, REST, or WinRM.

```yaml
# /etc/keystone-core/proxy-agent.yaml

# Proxy agent identity
agent:
  id: "proxy-01"                      # Unique identifier
  cluster_name: "default"
  labels:
    role: "network-proxy"
    datacenter: "us-east-1"

# NATS connection
nats:
  urls:
    - "nats://control-plane:4222"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""
  credentials_file: ""

# Health monitoring
health:
  interval: "30s"                     # Check interval
  timeout: "10s"                      # Check timeout
  max_concurrent: 10                  # Concurrent checks
  stale_threshold: "5m"               # Stale device threshold

# Managed devices
devices:
  - id: "router-01"
    name: "Core Router"
    type: "router"
    vendor: "cisco"
    model: "ISR4431"
    protocol: "ssh"                   # ssh, snmp, rest, winrm
    address: "10.0.1.1"
    port: 22                          # 0 = default for protocol
    profile_id: "cisco-ios"
    credential_ref: "router-creds"
    metadata:
      site: "datacenter-1"
    labels:
      tier: "core"
      critical: "true"

  - id: "switch-01"
    name: "Access Switch"
    type: "switch"
    vendor: "arista"
    protocol: "rest"
    address: "10.0.2.1"
    port: 443
    profile_id: "arista-eos"
    credential_ref: "switch-creds"

  - id: "windows-legacy"
    name: "Legacy Windows Server"
    type: "server"
    vendor: "microsoft"
    protocol: "winrm"
    address: "10.0.3.1"
    port: 5985
    profile_id: "windows-server"
    credential_ref: "windows-creds"

# Credential providers
credentials:
  provider: "vault"                   # vault, kubernetes, file, env

  vault:
    address: "https://vault:8200"
    token: "${VAULT_TOKEN}"
    path: "secret/kscore/devices"
    tls:
      enabled: true
      ca_file: "/etc/keystone-core/vault-ca.crt"

  kubernetes:
    namespace: "kscore"
    label_selector: "app=kscore-creds"

  file:
    path: "/etc/keystone-core/credentials.enc"
    encryption_key: "${CREDS_KEY}"

  env:
    prefix: "KSCORE_CRED"            # Default prefix for env vars
```

When using the `env` provider, credentials are read from environment variables derived from
the device's `credential_ref`. For a ref of `router-creds` with the default prefix:

| Variable | Field |
|----------|-------|
| `KSCORE_CRED_ROUTER_CREDS_USERNAME` | Username |
| `KSCORE_CRED_ROUTER_CREDS_PASSWORD` | Password |
| `KSCORE_CRED_ROUTER_CREDS_TOKEN` | API/bearer token |
| `KSCORE_CRED_ROUTER_CREDS_COMMUNITY` | SNMP community string |
| `KSCORE_CRED_ROUTER_CREDS_TYPE` | Credential type override |

The credential type is inferred from which variables are set (password, token, or snmp_community)
unless explicitly overridden with `_TYPE`.

Device discovery and device profiles are managed through the CLI (`kscorectl proxy discover`)
and the vendor driver system respectively, not through the proxy agent configuration file.

## Air-Gap Configuration

Configuration reference for air-gapped deployment features. These settings are used by the `kscore-transfer sync` and `kscore-transfer diode` commands, not via `server.yaml`.

### Sync Windows

Sync windows define scheduled connectivity periods for environments with intermittent network access. Configure via `kscore-transfer sync` CLI flags or a sync config file.

```yaml
# sync-windows.yaml

windows:
  - name: "nightly-module-sync"
    cron_schedule: "0 2 * * *"            # Daily at 2 AM
    duration: "2h"                        # Max window duration
    timezone: "America/New_York"          # Optional timezone (default: UTC)
    bandwidth_limit: 10485760             # Bytes/sec (10 MB/s, 0 = unlimited)
    enabled: true
    operations:
      - type: "pull_modules"              # pull_modules, pull_blueprints, push_audit_logs, push_metrics, full_sync
        priority: 1                       # Lower = higher priority
        endpoint: "https://registry.internal:8081"
      - type: "push_audit_logs"
        priority: 2
        endpoint: "https://siem.internal:9200"

  - name: "weekly-full-sync"
    cron_schedule: "0 0 * * 0"            # Sundays at midnight
    duration: "8h"
    bandwidth_limit: 0                    # Unlimited
    enabled: true
    operations:
      - type: "full_sync"
        priority: 1
        endpoint: "https://hub.internal:8080"
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `name` | Yes | — | Unique window identifier |
| `cron_schedule` | Yes | — | Standard cron expression (5-field) |
| `duration` | Yes | — | Maximum window duration |
| `timezone` | No | UTC | IANA timezone name |
| `bandwidth_limit` | No | `0` | Rate limit in bytes/sec (0 = unlimited) |
| `enabled` | No | `false` | Whether the window is active |
| `operations[].type` | Yes | — | `pull_modules`, `pull_blueprints`, `push_audit_logs`, `push_metrics`, `full_sync` |
| `operations[].priority` | No | `0` | Execution priority (lower = first) |
| `operations[].endpoint` | Yes | — | Target endpoint URL |

**Validation rules:**

- `name` and `cron_schedule` are required
- `duration` must be positive
- At least one operation is required per window
- `cron_schedule` must be a valid cron expression
- `timezone`, if set, must be a valid IANA timezone

### Data Diode

Data diode configuration for one-way UDP transfers in classified environments. Configure via `kscore-transfer diode send` / `diode receive` CLI flags.

```yaml
# diode.yaml

address: "10.0.0.1:9999"                 # UDP address (sender: destination, receiver: listen)
packet_size: 1400                         # UDP payload size in bytes
rate_limit: 1048576                       # Bytes/sec (1 MB/s, 0 = unlimited)
fec_enabled: true                         # Enable forward error correction
fec_group_size: 5                         # Data packets per FEC parity group
timeout: "30s"                            # Session timeout
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `address` | Yes | — | UDP address (`host:port`) |
| `packet_size` | No | `1400` | UDP payload size in bytes |
| `rate_limit` | No | `0` | Transmission rate limit in bytes/sec |
| `fec_enabled` | No | `false` | Enable XOR parity forward error correction |
| `fec_group_size` | No | `5` | Number of data packets per FEC group |
| `timeout` | No | `30s` | Session timeout for sender/receiver |

**Validation rules:**

- `address` is required
- `packet_size` must be non-negative
- `fec_group_size` must be non-negative

See also: [Air-Gapped Deployments](../../operations/air-gapped-deployments/)

## Agent Configuration

Complete configuration reference for `kscore-agent`.

### Basic Configuration

```yaml
# /etc/keystone-core/agent.yaml

# NATS Configuration
# The agent requires explicit NATS mode configuration for security
nats:
  mode: "external"                  # external, embedded, leaf (REQUIRED)
  url: "nats://control-plane:4222"  # External NATS URL (when mode: external)
  credentials: "/etc/keystone-core/agent.creds"
  tls:
    enabled: false
    ca_file: "/etc/keystone-core/ca.crt"
    cert_file: "/etc/keystone-core/agent.crt"
    key_file: "/etc/keystone-core/agent.key"

  # Embedded NATS settings (when mode: embedded or leaf)
  embedded:
    listen: "0.0.0.0:4222"         # Combined host:port
    host: "0.0.0.0"                 # Bind address
    port: 4222                      # NATS port
    enable_jetstream: true           # Enable JetStream
    storedir: "./data/nats"          # JetStream storage directory
    max_memory: 1073741824           # Max memory (bytes, default: 1GB)
    max_connections: 100             # Max client connections
    addressfamily: "prefer_ipv4"    # prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
    leaf_node_urls: []               # Parent leaf URLs (when mode: leaf)

  # Connection settings
  max_reconnects: -1                # Unlimited reconnects
  reconnect_wait: "2s"

# Agent Identity
agent:
  id: ""                            # Auto-generated if empty
  heartbeat_interval: "30s"         # Heartbeat interval (default: 30s)
  command_timeout: "5m"             # Command execution timeout (default: 5m)
  metadata_interval: "5m"           # System metadata collection interval (default: 5m)
  addressfamily: "prefer_ipv4"     # prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
  advertise_addrs: []               # Optional static advertise addresses
  labels:                           # Key-value pairs for targeting
    tier: "frontend"
    env: "production"
    role: "web"

# Logging
logging:
  level: "info"
  format: "json"
  output: "stdout"                  # stdout, syslog (file output not supported)

# Security
security:
  # Authorization settings
  authorization:
    enabled: false                  # Enable authorization checks on commands
    shared_secret: ""               # HMAC secret for command signing
    allowed_principals: []          # Principals allowed to execute commands
    require_signature: false        # Require cryptographic command signatures

  # Command filtering
  command_filter:
    mode: "blocklist"               # allowlist (more secure) or blocklist
    allowlist: []                   # Permitted commands (when mode: allowlist)
    blocklist: []                   # Blocked commands (when mode: blocklist)
    allow_builtins: true            # Allow shell builtins
    max_arg_length: 65536           # 64KB max per argument
    block_env_overrides: true       # Block dangerous env vars
    blocked_env_vars:               # Environment variables that cannot be set
      - "LD_PRELOAD"
      - "LD_LIBRARY_PATH"
      - "DYLD_INSERT_LIBRARIES"
      - "PYTHONPATH"
      - "RUBYLIB"
      - "PERL5LIB"
      - "NODE_PATH"
    blocked_patterns:               # Regex patterns that block commands
      - ';\s*rm\s+-rf\s+/'          # Dangerous rm commands
      - '>\s*/dev/sd[a-z]'          # Writing to block devices
      - 'mkfs\.'                    # Filesystem creation
      - 'dd\s+.*of=/dev/'           # dd to devices
    exempt_commands: []             # Commands that bypass blocked_patterns
                                    # Use glob patterns, e.g., "mkfs.*", "/sbin/mkfs*"

  # Legacy options (deprecated - use command_filter instead)
  sandbox: true                     # Sandbox command execution
  allowed_commands: []              # Command whitelist (empty = all)
  blocked_commands: []              # Command blacklist
  run_as_user: ""                   # Run as specific user

  # Sandboxing (Linux)
  seccomp_enabled: true
  cgroups_enabled: true

# Offline Mode
offline:
  enabled: false
  buffer_size: 1000                 # Commands buffered while offline
  reconnect_interval: "1m"
  max_offline_duration: "24h"

# Cache
cache:
  enabled: false
  directory: "/var/lib/keystone-core/cache"
  max_size: "1GB"
```

### Resource Monitoring

```yaml
# Resource monitoring settings
monitoring:
  enabled: true
  interval: "30s"                   # Monitoring interval

  cpu:
    enabled: true
    threshold_warning: 80           # Warning threshold (%)
    threshold_critical: 95          # Critical threshold (%)

  memory:
    enabled: true
    threshold_warning: 80
    threshold_critical: 95

  disk:
    enabled: true
    threshold_warning: 80
    threshold_critical: 95
    mount_points:                   # Monitor specific mounts
      - "/"
      - "/var"
```

## CLI Configuration

> **Note**: CLI configuration file support is planned but not yet implemented. Currently, use command-line flags or environment variables to configure `kscorectl`. The configuration format below shows the planned structure.

Client configuration for `kscorectl`.

```yaml
# ~/.keystone-core/config.yaml

# Control plane connection
server: "http://control-plane.example.com:8080"
api_key: "<your-api-key>"

# TLS configuration
tls:
  enabled: false
  ca_cert: "/etc/keystone-core/ca.crt"
  client_cert: "/etc/keystone-core/client.crt"
  client_key: "/etc/keystone-core/client.key"
  skip_verify: false

# Output preferences
output:
  format: "text"                    # text, json, yaml
  color: true
  timestamps: false
  verbose: false

# Defaults
defaults:
  timeout: "5m"
  batch_size: 10
  batch_delay: "5s"

# Profiles (switch with --profile flag)
profiles:
  production:
    server: "https://prod.example.com"
    api_key: "${PROD_API_KEY}"
  staging:
    server: "https://staging.example.com"
    api_key: "${STAGING_API_KEY}"
```

## State File Configuration

State file syntax and options.

### State Declaration

```yaml
# State ID
state_id:
  # Module type
  module: package                   # file, package, service, user, group, cmd

  # Desired state
  state: installed                  # Module-specific states

  # Module parameters (vary by module)
  name: nginx
  version: "1.24.*"

  # Requisites (dependencies)
  require:                          # Execute after
    - other_state_id
  require_in:                       # Make other state depend on this
    - dependent_state
  watch:                            # Execute when dependency changes
    - watched_state
  watch_in:                         # Notify when this changes
    - notified_state
  prereq:                           # Must succeed before
    - following_state
  onchanges:                        # Execute only if dependency changes
    - changed_state
```

### Template Variables

```yaml
# Use vars in state files
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  contents: |
    worker_processes {{ .vars.worker_processes }};
    server_name {{ .vars.server_name }};
```

### Facts

```yaml
# Use facts in state files
platform_package:
  module: package
  state: installed
  name: |
    {{- if eq .facts.os "ubuntu" }}
    nginx
    {{- else if eq .facts.os "centos" }}
    nginx
    {{- end }}
```

## Reactor Configuration

Reactor definitions.

```yaml
# Reactor ID
reactor_id:
  name: "Reactor name"
  description: "Reactor description"

  # Event filter (CEL expression)
  filter: "type == 'agent.disconnect' and severity >= 'warning'"

  # Priority (lower = higher priority)
  priority: 1

  # Actions
  actions:
    - type: command
      command: "systemctl restart nginx"
      target: "agent_id == {{ event.source }}"

    - type: webhook
      url: "https://slack.com/hooks/xxx"
      method: POST
      body: |
        {"text": "Alert: {{ event.type }}"}

  # Conditions
  conditions:
    throttle: "5m"                  # Max once per 5 minutes
    debounce: "30s"                 # Wait for quiet period
    max_executions: 10              # Max executions
    time_window: "1h"               # Per time window
    only_if: "event.data.critical == true"
    unless: "event.data.ignore == true"

  # Execution settings
  max_concurrent: 3                 # Max concurrent executions
  timeout: "10m"                    # Action timeout
  error_strategy: "continue"        # continue, stop, retry

  # Enable/disable
  enabled: true
```

## Policy Configuration

Policy definitions.

```yaml
# Policy ID
policy_id:
  name: "Policy name"
  description: "Policy description"

  # Policy type
  type: opa                         # opa, cel, builtin

  # Category
  category: security                # security, compliance, operational, cost

  # Severity
  severity: high                    # low, medium, high, critical

  # Enforcement mode
  enforcement: enforce              # enforce, audit, warn

  # Policy code
  code: |
    package kscore.security

    deny[msg] {
        input.resource.type == "file"
        input.resource.path == "/etc/ssh/sshd_config"
        contains(input.resource.contents, "Port 22")
        msg := "SSH must not use default port 22"
    }

  # Enforcement points
  enforce_at:
    - pre_execution
    - on_change
    - on_drift

  # Target resources
  targets:
    - resource_type: file
      path: "/etc/ssh/sshd_config"
```

## Common Configurations

### Development Setup

```yaml
# server.yaml (development)
api:
  listen: "localhost:8080"
nats:
  mode: embedded
storage:
  backend: sqlite
  sqlite:
    path: "./dev.db"
logging:
  level: debug
  format: text
```

### Production Setup

```yaml
# server.yaml (production)
api:
  listen: "0.0.0.0:8080"
tls:
  enabled: true
  cert_file: "/etc/keystone-core/server.crt"
  key_file: "/etc/keystone-core/server.key"
nats:
  mode: external
  urls:
    - "nats://nats1:4222"
    - "nats://nats2:4222"
    - "nats://nats3:4222"
  credentials: "/etc/keystone-core/nats.creds"
storage:
  backend: postgresql
  postgresql:
    host: "postgres-cluster"
    port: 5432
    sslmode: verify-full
logging:
  level: info
  format: json
  output: syslog                    # Use syslog for production (file output not supported)
  syslog:
    network: unix
    address: "/dev/log"
```

### High Availability

```yaml
# Multiple control planes with external NATS
nats:
  mode: external
  urls:
    - "nats://nats1.dc1:4222"
    - "nats://nats2.dc1:4222"
    - "nats://nats3.dc1:4222"
    - "nats://nats1.dc2:4222"
    - "nats://nats2.dc2:4222"
  tls:
    enabled: true
    ca_file: "/etc/keystone-core/nats-ca.crt"
storage:
  backend: postgresql
  postgresql:
    host: "postgres-ha.cluster.local"
    port: 5432
    sslmode: verify-full
    max_connections: 50
```

## Environment Variables

Override configuration with environment variables:

### Control Plane

```bash
KSCORE_API_LISTEN="0.0.0.0:8080"
KSCORE_NATS_MODE="embedded"
KSCORE_NATS_URL="nats://nats:4222"
KSCORE_STORAGE_TYPE="postgresql"
KSCORE_STORAGE_POSTGRES_HOST="postgres"
KSCORE_LOG_LEVEL="info"
KSCORE_LOG_FORMAT="json"

# Agent management
KSCORE_AGENT_MGMT_HEARTBEAT_INTERVAL="30s"
KSCORE_AGENT_MGMT_HEARTBEAT_TIMEOUT="60s"
KSCORE_AGENT_MGMT_STALE_THRESHOLD="3"
KSCORE_AGENT_MGMT_MONITOR_INTERVAL="10s"
KSCORE_AGENT_MGMT_METADATA_REFRESH="5m"
KSCORE_AGENT_MGMT_MAX_CONCURRENT_COMMANDS="0"

# Command execution
KSCORE_EXEC_DEFAULT_TIMEOUT="5m"
KSCORE_EXEC_MAX_TIMEOUT="1h"
KSCORE_EXEC_BATCH_SIZE="10"
KSCORE_EXEC_BATCH_DELAY="0s"
KSCORE_EXEC_STREAMING_BUFFER="100"
KSCORE_EXEC_RESULT_RETENTION="24h"

# State management
KSCORE_STATE_DEFAULT_TIMEOUT="10m"
KSCORE_STATE_MAX_CONCURRENT="5"
KSCORE_STATE_DRIFT_CHECK_INTERVAL="0s"
KSCORE_STATE_RESULT_RETENTION="168h"

# Event system
KSCORE_EVENTS_ENABLED="true"
KSCORE_EVENTS_RETENTION="168h"
KSCORE_EVENTS_MAX_BYTES="10737418240"
KSCORE_EVENTS_MAX_MESSAGES="1000000"

# Authorization
KSCORE_AUTHZ_ENABLED="true"
KSCORE_AUTHZ_DEFAULT_DENY="true"

# Kubernetes operator
KSCORE_OPERATOR_ENABLED="false"
KSCORE_OPERATOR_NAMESPACE=""
KSCORE_OPERATOR_LEADER_ELECTION="true"
KSCORE_OPERATOR_LEADER_ELECTION_ID="kscore-operator"
KSCORE_OPERATOR_RECONCILE_INTERVAL="1m"
KSCORE_OPERATOR_MAX_CONCURRENT_RECONCILES="3"
```

### Agent

```bash
KSCORE_CONTROL_PLANE_URL="nats://control-plane:4222"
KSCORE_AGENT_ID="custom-agent-01"
KSCORE_LOG_LEVEL="info"
```

### CLI

```bash
KSCORE_SERVER="http://control-plane:8080"
KSCORE_API_KEY="<your-api-key>"
KSCORE_OUTPUT_FORMAT="json"
KSCORE_NO_COLOR="true"
```

### Bootstrap Environment Variables

The agent bootstrap process supports extensive environment variable configuration for automated deployments:

#### Core Bootstrap Settings

```bash
# Bootstrap mode: standalone, cluster, join, migrate
KSCORE_BOOTSTRAP_MODE="cluster"

# Cluster identification
KSCORE_CLUSTER_NAME="production-cluster"
KSCORE_NODE_NAME="node-01"
KSCORE_NODE_ROLE="control"  # control, agent, or both

# Network configuration
KSCORE_BIND_ADDRESS="0.0.0.0"
KSCORE_ADVERTISE_ADDRESS="10.0.0.1"
```

#### Cluster Join Settings

```bash
# Join an existing cluster
KSCORE_JOIN_ENDPOINT="https://control-plane:8080"
KSCORE_JOIN_TOKEN="<bootstrap-token>"
```

#### Storage Backend

```bash
# Storage backend: sqlite, postgresql
KSCORE_STORAGE_BACKEND="postgresql"

# PostgreSQL connection
KSCORE_POSTGRES_HOST="postgres.example.com"
KSCORE_POSTGRES_PORT="5432"
KSCORE_POSTGRES_USER="keystone"
KSCORE_POSTGRES_PASSWORD="<password>"
KSCORE_POSTGRES_DATABASE="keystone_core"
KSCORE_POSTGRES_SSL_MODE="require"
```

#### NATS Configuration

```bash
# NATS mode: embedded, external, leaf
KSCORE_NATS_MODE="external"
KSCORE_NATS_URLS="nats://nats-1:4222,nats://nats-2:4222"
KSCORE_NATS_CREDS_FILE="/etc/keystone-core/nats.creds"
KSCORE_NATS_USER="keystone"
KSCORE_NATS_PASSWORD="<password>"
```

#### TLS Configuration

```bash
# Generate self-signed certificates (development only)
KSCORE_GENERATE_CERTS="true"

# Use existing certificates
KSCORE_TLS_CERT="/etc/keystone-core/tls/cert.pem"
KSCORE_TLS_KEY="/etc/keystone-core/tls/key.pem"
KSCORE_TLS_CA="/etc/keystone-core/tls/ca.pem"
KSCORE_TLS_CLIENT_CERT="/etc/keystone-core/tls/client-cert.pem"
KSCORE_TLS_CLIENT_KEY="/etc/keystone-core/tls/client-key.pem"
KSCORE_TLS_MIN_VERSION="1.3"
```

#### Node Labels

```bash
# Labels for targeting (key=value pairs, comma-separated)
KSCORE_NODE_LABELS="environment=production,datacenter=us-east-1,role=webserver"
```

#### Package Installation

```bash
# Package channel: stable, beta, nightly
KSCORE_PACKAGE_CHANNEL="stable"

# Specific version to install
KSCORE_PACKAGE_VERSION="0.1.0"
```

#### Migration Settings

```bash
# Migrate from another system
KSCORE_MIGRATE_FROM="salt"
KSCORE_MIGRATE_CONFIG="/etc/salt/minion"
KSCORE_MIGRATE_STATE_DIR="/srv/salt"
KSCORE_MIGRATE_PILLAR_DIR="/srv/pillar"
```

#### Blueprint Application

```bash
# Blueprints directory to apply on bootstrap
KSCORE_BLUEPRINTS_DIR="/etc/keystone-core/blueprints"

# Apply specific blueprints (comma-separated)
KSCORE_APPLY_BLUEPRINTS="kscore/production-cluster,kscore/monitoring-stack"

# Blueprint parameters (JSON format)
KSCORE_BLUEPRINT_PARAMS='{"cluster_name":"prod","node_count":3}'

# Blueprint features to enable (comma-separated)
KSCORE_BLUEPRINT_FEATURES="tls,monitoring,backup"

# Blueprint entrypoints (comma-separated)
KSCORE_BLUEPRINT_ENTRYPOINTS="infra,services"
```

#### State Export

```bash
# Export generated states to directory
KSCORE_EXPORT_STATES_DIR="/var/lib/keystone-core/generated-states"
```

#### Non-Interactive Mode

```bash
# Run bootstrap without prompts (for automation)
KSCORE_BOOTSTRAP_NON_INTERACTIVE="true"
```

## Configuration Validation

Validate configuration files:

```bash
# Validate control plane config
kscorectl config validate --config server.yaml

# Validate agent config
kscorectl config validate --config agent.yaml
```

## Configuration Examples

Complete example configurations are available in the repository:

```
deploy/
├── examples/
│   ├── development/
│   │   ├── server.yaml
│   │   └── agent.yaml
│   ├── production/
│   │   ├── server.yaml
│   │   ├── agent.yaml
│   │   └── nats-cluster.conf
│   └── kubernetes/
│       ├── configmaps/
│       └── secrets/
```

## See Also

- [API Reference](../api/) - API endpoints
- [CLI Reference](../cli/) - Command-line tools
- [Getting Started](../../getting-started/installation/) - Installation guide
