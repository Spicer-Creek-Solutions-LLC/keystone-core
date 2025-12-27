---
title: "Configuration Reference"
weight: 3
description: >
  Complete configuration reference for control plane, agents, and all subsystems
---

## Overview

TitanAnvil components are configured using YAML files. This reference documents all configuration options.

**Configuration Files**:
- Control Plane: `/etc/titananvil/server.yaml`
- Agent: `/etc/titananvil/agent.yaml`
- CLI: `~/.titananvil/config.yaml`

## Control Plane Configuration

Complete configuration reference for `titananvil-server`.

### Basic Configuration

```yaml
# /etc/titananvil/server.yaml

# API Server
api:
  listen: "0.0.0.0:8080"           # HTTP API listen address
  grpc_listen: "0.0.0.0:9090"       # gRPC API listen address
  tls:
    enabled: false                  # Enable TLS
    cert_file: ""                   # TLS certificate file
    key_file: ""                    # TLS key file
    ca_file: ""                     # CA certificate for client auth
  cors:
    enabled: true                   # Enable CORS
    allowed_origins: ["*"]          # Allowed origins
  rate_limit:
    enabled: true                   # Enable rate limiting
    requests_per_minute: 100        # Requests per minute per key
    burst: 20                       # Burst capacity

# NATS Configuration
nats:
  mode: "embedded"                  # embedded, external, leaf
  listen: "0.0.0.0:4222"           # NATS listen address (embedded mode)
  urls: []                          # NATS server URLs (external mode)
  credentials: ""                   # NATS credentials file
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""
  jetstream:
    enabled: true                   # Enable JetStream
    store_dir: "/var/lib/titananvil/nats"
    max_memory: "1GB"               # Max memory for streams
    max_file: "10GB"                # Max file storage

# State Storage
storage:
  type: "sqlite"                    # sqlite, postgresql
  sqlite:
    path: "/var/lib/titananvil/titan.db"
    max_connections: 10
    busy_timeout: "5s"
  postgresql:
    host: "localhost"
    port: 5432
    database: "titananvil"
    username: "titananvil"
    password: ""
    sslmode: "disable"              # disable, require, verify-ca, verify-full
    max_connections: 25
    idle_connections: 5
    connection_lifetime: "1h"

# Logging
logging:
  level: "info"                     # debug, info, warn, error
  format: "json"                    # json, logfmt, text
  output: "stdout"                  # stdout, file
  file: "/var/log/titananvil/server.log"
  max_size: "100MB"                 # Max log file size
  max_backups: 3                    # Max backup files
  max_age: 30                       # Max age in days
  compress: true                    # Compress rotated logs

# Metrics
metrics:
  enabled: true
  listen: ":8080"                   # Metrics endpoint (usually same as API)
  path: "/metrics"                  # Metrics path

# Tracing
tracing:
  enabled: false
  exporter: "otlp"                  # otlp, jaeger, zipkin
  endpoint: "http://jaeger:4318"
  sampling:
    strategy: "ratio"               # always, never, ratio
    ratio: 0.1                      # Sample rate (0.0-1.0)
  resource:
    service.name: "titananvil-server"
    service.version: "1.0.0"
    deployment.environment: "production"

# Health Checks
health:
  enabled: true
  startup_grace_period: "30s"      # Grace period on startup
  check_interval: "10s"             # Health check interval
  checks:
    nats:
      enabled: true
      timeout: "5s"
    database:
      enabled: true
      timeout: "5s"
    agents:
      enabled: true
      min_healthy: 0.8              # 80% of agents must be healthy
```

### Agent Management

```yaml
# Agent management settings
agents:
  heartbeat_interval: "30s"         # Expected heartbeat interval
  heartbeat_timeout: "90s"          # Timeout before marking offline
  metadata_refresh: "5m"            # Metadata refresh interval
  max_concurrent_commands: 100      # Max concurrent command dispatches
```

### Command Execution

```yaml
# Remote execution settings
execution:
  default_timeout: "5m"             # Default command timeout
  max_timeout: "1h"                 # Maximum allowed timeout
  batch_size: 10                    # Default batch size
  batch_delay: "5s"                 # Default batch delay
  streaming_buffer: 1024            # Stream buffer size (lines)
  result_retention: "7d"            # How long to keep results
```

### State Management

```yaml
# State management settings
state:
  default_timeout: "10m"            # Default state apply timeout
  max_concurrent: 50                # Max concurrent state applies
  drift_check_interval: "1h"        # Automatic drift check interval
  result_retention: "30d"           # How long to keep results
```

### Event System

```yaml
# Event system settings
events:
  storage:
    enabled: true
    retention:
      max_age: "30d"                # Delete events older than
      max_count: 1000000            # Keep max events
      min_severity: "info"          # Delete debug events after 7d
    type_retention:                 # Per-type retention
      "agent.heartbeat": "1d"
      "state.drift": "90d"
  publisher:
    buffer_size: 1000               # Event buffer size
    batch_size: 100                 # Batch publish size
  subscriber:
    buffer_size: 1000
    ack_wait: "30s"                 # Ack timeout
```

### Policy Enforcement

```yaml
# Policy enforcement settings
policy:
  enabled: true
  enforcement_mode: "enforce"       # enforce, audit, warn
  cache_ttl: "5m"                   # Policy cache TTL
  evaluation_timeout: "10s"         # Policy evaluation timeout
  opa:
    enabled: true
    memory_limit: "512MB"
  cel:
    enabled: true
```

### GitOps Integration

```yaml
# GitOps integration settings
gitops:
  webhooks:
    enabled: true
    listen: "0.0.0.0:8090"
    path: "/webhooks"
    auth:
      type: "hmac"                  # none, hmac, bearer
      secret: "webhook-secret"
    async: true
    queue_size: 1000

  git_sync:
    enabled: true
    repositories:
      - name: "infrastructure-config"
        url: "https://github.com/myorg/infrastructure-config"
        branch: "main"
        auth:
          type: "ssh"
          ssh_key_path: "/etc/titananvil/id_rsa"
        paths:
          states: "states/"
          reactors: "reactors/"
          policies: "policies/"
        sync_interval: "5m"
```

### Security

```yaml
# Security settings
security:
  authentication:
    type: "api_key"                 # api_key, mtls, oauth2
    api_keys:
      - name: "admin"
        key: "ta_live_abc123"
        permissions: ["*"]
    mtls:
      ca_file: "/etc/titananvil/ca.crt"
      verify_client: true

  authorization:
    enabled: true
    default_deny: true
    rbac:
      enabled: true
      policy_file: "/etc/titananvil/rbac.yaml"
```

## Agent Configuration

Complete configuration reference for `titananvil-agent`.

### Basic Configuration

```yaml
# /etc/titananvil/agent.yaml

# Control Plane Connection
control_plane:
  url: "nats://control-plane.example.com:4222"
  credentials: "/etc/titananvil/agent.creds"
  tls:
    enabled: false
    ca_file: "/etc/titananvil/ca.crt"
    cert_file: "/etc/titananvil/agent.crt"
    key_file: "/etc/titananvil/agent.key"

  # Connection settings
  max_reconnects: -1                # Unlimited reconnects
  reconnect_delay: "2s"
  reconnect_jitter: "1s"
  ping_interval: "2m"
  max_ping_out: 2

# Agent Identity
agent:
  id: ""                            # Auto-generated if empty
  datacenter: "us-east-1"
  environment: "production"
  role: "web"
  tags:
    - "nginx"
    - "frontend"

  # Custom metadata
  metadata:
    team: "platform"
    cost_center: "engineering"

# Heartbeat
heartbeat:
  interval: "30s"
  timeout: "10s"
  include_stats: true               # Include resource stats

# Logging
logging:
  level: "info"
  format: "json"
  output: "stdout"
  file: "/var/log/titananvil/agent.log"

# Execution Settings
execution:
  timeout: "5m"                     # Default command timeout
  max_concurrent: 10                # Max concurrent commands
  shell: "bash"                     # bash, sh, zsh, powershell, cmd
  working_dir: "/tmp"

  # Resource limits
  limits:
    max_memory: "512MB"
    max_cpu_percent: 80
    max_processes: 100

# State Management
state:
  modules_dir: "/var/lib/titananvil/modules"
  cache_enabled: true
  cache_dir: "/var/cache/titananvil"
  dry_run: false

# Security
security:
  sandbox: true                     # Sandbox command execution
  allowed_commands: []              # Command whitelist (empty = all)
  blocked_commands:                 # Command blacklist
    - "rm -rf /"
    - "mkfs"
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
  directory: "/var/lib/titananvil/cache"
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

Client configuration for `titanctl`.

```yaml
# ~/.titananvil/config.yaml

# Control plane connection
server: "http://control-plane.example.com:8080"
api_key: "ta_live_abc123xyz789"

# TLS configuration
tls:
  enabled: false
  ca_cert: "/etc/titananvil/ca.crt"
  client_cert: "/etc/titananvil/client.crt"
  client_key: "/etc/titananvil/client.key"
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
    package titananvil.security

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
  type: sqlite
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
    cert_file: "/etc/titananvil/server.crt"
    key_file: "/etc/titananvil/server.key"
nats:
  mode: external
  urls:
    - "nats://nats1:4222"
    - "nats://nats2:4222"
    - "nats://nats3:4222"
  credentials: "/etc/titananvil/nats.creds"
storage:
  type: postgresql
  postgresql:
    host: "postgres-cluster"
    port: 5432
    sslmode: verify-full
logging:
  level: info
  format: json
  output: file
  file: "/var/log/titananvil/server.log"
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
    ca_file: "/etc/titananvil/nats-ca.crt"
storage:
  type: postgresql
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
TITAN_API_LISTEN="0.0.0.0:8080"
TITAN_NATS_MODE="embedded"
TITAN_NATS_URL="nats://nats:4222"
TITAN_STORAGE_TYPE="postgresql"
TITAN_STORAGE_POSTGRES_HOST="postgres"
TITAN_LOG_LEVEL="info"
TITAN_LOG_FORMAT="json"
```

### Agent

```bash
TITAN_CONTROL_PLANE_URL="nats://control-plane:4222"
TITAN_AGENT_ID="custom-agent-01"
TITAN_AGENT_DATACENTER="us-east-1"
TITAN_AGENT_ENVIRONMENT="production"
TITAN_AGENT_ROLE="web"
TITAN_LOG_LEVEL="info"
```

### CLI

```bash
TITAN_SERVER="http://control-plane:8080"
TITAN_API_KEY="ta_live_abc123"
TITAN_OUTPUT_FORMAT="json"
TITAN_NO_COLOR="true"
```

## Configuration Validation

Validate configuration files:

```bash
# Validate control plane config
titananvil-server --config server.yaml --validate

# Validate agent config
titananvil-agent --config agent.yaml --validate

# Test configuration
titananvil-server --config server.yaml --test
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
