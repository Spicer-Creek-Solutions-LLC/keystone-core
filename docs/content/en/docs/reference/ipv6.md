---
title: "IPv6 Reference"
weight: 25
description: >
  Complete reference for IPv6 configuration options, address formats, and APIs
---

## Configuration Reference

### Address Family Preference

Controls how Keystone Core selects between IPv4 and IPv6 addresses.

| Value | Description |
|-------|-------------|
| `prefer_ipv4` | Use IPv4 when available, fall back to IPv6 |
| `prefer_ipv6` | Use IPv6 when available, fall back to IPv4 (default) |
| `ipv4_only` | Only use IPv4 addresses, fail if unavailable |
| `ipv6_only` | Only use IPv6 addresses, fail if unavailable |

**Control Plane:**

```yaml
cluster:
  address_family: prefer_ipv6
```

**Agent:**

```yaml
agent:
  address_family: prefer_ipv6
```

### Control Plane IPv6 Configuration

#### API Server

```yaml
api:
  grpc:
    # Single IPv6 address
    listen: "[::]:8080"

    # Or dual-stack (array)
    listen:
      - "[::]:8080"
      - "0.0.0.0:8080"

    # TLS with IPv6
    tls:
      enabled: true
      cert_file: /etc/keystone-core/tls/server.crt
      key_file: /etc/keystone-core/tls/server.key

  rest:
    listen: "[::]:8081"
```

#### Metrics Endpoint

```yaml
metrics:
  enabled: true
  listen: "[::]:9090"
```

#### Health Endpoint

```yaml
health:
  enabled: true
  listen: "[::]:9091"
```

### NATS IPv6 Configuration

#### Embedded NATS

```yaml
nats:
  mode: embedded

  # Client connections
  listen: "[::]:4222"

  # Cluster routing
  cluster:
    enabled: true
    listen: "[::]:6222"
    routes:
      - "nats://[2001:db8::2]:6222"
      - "nats://[2001:db8::3]:6222"

  # WebSocket
  websocket:
    enabled: true
    listen: "[::]:8443"
```

#### External NATS

```yaml
nats:
  mode: external
  urls:
    - "nats://[2001:db8::1]:4222"
    - "nats://[2001:db8::2]:4222"

  # Dual-stack with failover
  urls:
    - "nats://[2001:db8::1]:4222"   # IPv6 primary
    - "nats://10.0.1.1:4222"         # IPv4 fallback
```

#### Leaf Node

```yaml
nats:
  mode: leaf
  leaf:
    remotes:
      - url: "nats://[2001:db8::1]:7422"
        tls:
          enabled: true
```

### etcd IPv6 Configuration

#### Embedded etcd

```yaml
cluster:
  etcd:
    mode: embedded
    embedded:
      # Data directory
      data_dir: /var/lib/keystone-core/etcd

      # Listen on all IPv6 interfaces
      listen_address: "::"

      # Advertise specific IPv6 address
      advertise_address: "2001:db8::1"

      # Ports
      client_port: 2379
      peer_port: 2380

      # Cluster formation (brackets required)
      initial_cluster: "n1=http://[2001:db8::1]:2380,n2=http://[2001:db8::2]:2380"
      initial_cluster_token: "kscore-cluster"
      initial_cluster_state: new
```

#### External etcd

```yaml
cluster:
  etcd:
    mode: external
    endpoints:
      - "https://[2001:db8::1]:2379"
      - "https://[2001:db8::2]:2379"
      - "https://[2001:db8::3]:2379"

    tls:
      enabled: true
      cert_file: /etc/keystone-core/tls/etcd-client.crt
      key_file: /etc/keystone-core/tls/etcd-client.key
      ca_file: /etc/keystone-core/tls/etcd-ca.crt
```

### PostgreSQL IPv6 Configuration

#### Structured Configuration (Recommended)

```yaml
state:
  backend: postgresql
  postgresql:
    # IPv6 address (brackets added automatically in DSN)
    host: "2001:db8::10"
    port: 5432
    database: kscore
    user: kscore
    password: "${KSCORE_DB_PASSWORD}"

    # Connection pool
    max_open: 25
    max_idle: 5
    conn_max_life: 5m

    # SSL/TLS
    sslmode: require           # disable, allow, prefer, require, verify-ca, verify-full
    sslrootcert: /etc/keystone-core/tls/pg-ca.crt
    sslcert: /etc/keystone-core/tls/pg-client.crt
    sslkey: /etc/keystone-core/tls/pg-client.key

    # Timeouts
    connect_timeout: 10

    # Application identifier
    application_name: kscore-server
```

#### DSN Configuration

```yaml
state:
  backend: postgresql
  # IPv6 addresses must be bracketed in DSN
  postgresql_dsn: "host=[2001:db8::10] port=5432 dbname=kscore user=kscore sslmode=require"
```

### Cluster Coordination IPv6

```yaml
cluster:
  enabled: true

  # Member identification
  member_id: "node-1"
  member_name: "Control Plane Node 1"

  # IPv6 advertise address
  advertise_address: "2001:db8::1"

  # gRPC port for inter-member communication
  grpc_port: 9090

  # Address family preference
  address_family: prefer_ipv6

  # Heartbeat settings
  heartbeat_interval: 5s
  election_timeout: 10s
```

### Agent IPv6 Configuration

```yaml
agent:
  # Agent identity
  id: "${HOSTNAME}"

  # Address family preference
  address_family: prefer_ipv6

  # Metadata
  labels:
    environment: production
    datacenter: dc1

nats:
  urls:
    - "nats://[2001:db8::1]:4222"
    - "nats://[2001:db8::2]:4222"

  # Connection settings
  connection:
    timeout: 30s
    reconnect_wait: 2s
    max_reconnect: -1  # Unlimited
```

## Address Format Reference

### Valid IPv6 Formats

| Format | Example | Notes |
|--------|---------|-------|
| Full | `2001:0db8:85a3:0000:0000:8a2e:0370:7334` | All 8 groups |
| Compressed | `2001:db8:85a3::8a2e:370:7334` | `::` replaces consecutive zeros |
| Loopback | `::1` | Equivalent to IPv4 `127.0.0.1` |
| Unspecified | `::` | Bind to all interfaces |
| IPv4-mapped | `::ffff:192.168.1.1` | IPv4 address in IPv6 format |
| Link-local | `fe80::1` | Requires zone ID for routing |
| With zone ID | `fe80::1%eth0` | Interface-specific (link-local) |

### URL Format with Port

When specifying IPv6 addresses with ports in URLs, brackets are required:

```
# Format: [ipv6-address]:port
[2001:db8::1]:8080
[::1]:4222
[::]:8080

# In URL context
nats://[2001:db8::1]:4222
grpc://[::1]:8080
https://[2001:db8:85a3::8a2e:370:7334]:443
postgresql://user:pass@[2001:db8::10]:5432/dbname
```

### PostgreSQL DSN Format

PostgreSQL connection strings require brackets around IPv6 addresses:

```
# Key-value format
host=[2001:db8::10] port=5432 dbname=kscore user=kscore

# URI format
postgresql://kscore:password@[2001:db8::10]:5432/kscore?sslmode=require
```

## Agent Metadata Reference

### IPv6 Fields

Agents report the following IPv6-related metadata:

| Field | Type | Description |
|-------|------|-------------|
| `ipv4_addresses` | `[]string` | List of IPv4 addresses |
| `ipv6_addresses` | `[]string` | List of IPv6 addresses |
| `address_family` | `string` | Agent's address family mode |
| `primary_ipv4` | `string` | Primary IPv4 address |
| `primary_ipv6` | `string` | Primary IPv6 address |

Example agent metadata:

```json
{
  "id": "agent-001",
  "hostname": "server1.example.com",
  "ipv4_addresses": ["10.0.1.5", "172.17.0.1"],
  "ipv6_addresses": ["2001:db8::5", "fe80::1"],
  "primary_ipv4": "10.0.1.5",
  "primary_ipv6": "2001:db8::5",
  "address_family": "dual_stack"
}
```

## Targeting Expression Reference

### IP Address Selector

The `ip:` selector matches any of an agent's IP addresses (both IPv4 and IPv6):

| Selector | Example | Description |
|----------|---------|-------------|
| `ip:` | `ip:2001:db8::5` | Exact IP match (IPv4 or IPv6) |
| `ip:*` | `ip:2001:db8::*` | Glob pattern match |
| `ip:*` | `ip:10.0.1.*` | IPv4 glob pattern |

> **Note**: CIDR notation is not directly supported. Use glob patterns for prefix matching.

### Examples

```bash
# Target specific IPv6 address
kscorectl exec run --target 'ip:2001:db8::5' -- hostname

# Target IPv6 with glob pattern (prefix matching)
kscorectl exec run --target 'ip:2001:db8:85a3::*' -- date

# Combine IP with other selectors
kscorectl exec run --target 'ip:2001:db8::* AND role:webserver' -- nginx -t

# Target by both IPv4 and IPv6 patterns
kscorectl exec run --target 'ip:10.0.1.* OR ip:2001:db8::*' -- hostname

# Target specific IPv4 address
kscorectl exec run --target 'ip:192.168.1.100' -- uptime
```

## Metrics Reference

### Connection Metrics by Family

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_connections_total` | Counter | `family` | Total connections established |
| `kscore_connection_failures_total` | Counter | `family` | Connection failures |
| `kscore_connection_duration_seconds` | Histogram | `family` | Connection duration |

**Labels:**

- `family`: `ipv4`, `ipv6`

### Agent Address Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_agents_by_address_family` | Gauge | `family` | Agent count by family |
| `kscore_agent_addresses_total` | Gauge | `family` | Total addresses reported |

**Labels:**

- `family`: `ipv4`, `ipv6`, `dual_stack`

### Example PromQL Queries

```promql
# Total connections by family
sum(kscore_connections_total) by (family)

# Connection failure rate by family (5m window)
sum(rate(kscore_connection_failures_total[5m])) by (family)

# Agents with only IPv6
kscore_agents_by_address_family{family="ipv6"}

# Dual-stack agents
kscore_agents_by_address_family{family="dual_stack"}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `KSCORE_ADDRESS_FAMILY` | Default address family preference | `prefer_ipv6` |
| `KSCORE_LISTEN_ADDRESS` | Control plane listen address | `[::]:8080` |
| `KSCORE_ADVERTISE_ADDRESS` | Cluster advertise address | (auto-detect) |
| `KSCORE_NATS_URL` | NATS server URL | `nats://[::1]:4222` |
| `KSCORE_ETCD_ENDPOINTS` | etcd endpoints (comma-separated) | `http://[::1]:2379` |
| `KSCORE_POSTGRES_HOST` | PostgreSQL host | `::1` |

## API Reference

### Health Check Endpoints

All health endpoints support IPv6:

```
GET http://[2001:db8::1]:9091/health/live
GET http://[2001:db8::1]:9091/health/ready
GET http://[2001:db8::1]:9091/health/status
```

### Metrics Endpoint

```
GET http://[2001:db8::1]:9090/metrics
```

### gRPC API

Connect to gRPC API via IPv6:

```go
conn, err := grpc.Dial("[2001:db8::1]:8080", grpc.WithTransportCredentials(creds))
```

### REST API

```bash
curl -X GET "https://[2001:db8::1]:8081/api/v1/agents"
```

## Validation Rules

### Address Validation

Keystone Core validates IPv6 addresses during configuration loading:

1. **Format validation**: Must be valid IPv6 notation
2. **Zone ID handling**: Zone IDs (`%eth0`) are stripped for comparison
3. **Bracket validation**: URLs must use brackets for IPv6
4. **Port validation**: Port numbers must be 1-65535

### Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| `invalid IPv6 address` | Malformed address | Check address format |
| `missing brackets in URL` | IPv6 in URL without brackets | Use `[addr]:port` format |
| `address family mismatch` | Config requires unavailable family | Check network interfaces |
| `cannot bind to address` | Address not available on host | Verify network configuration |

## Compatibility Matrix

| Component | IPv4 | IPv6 | Dual-Stack |
|-----------|------|------|------------|
| Control Plane API | ✅ | ✅ | ✅ |
| NATS (Embedded) | ✅ | ✅ | ✅ |
| NATS (External) | ✅ | ✅ | ✅ |
| NATS WebSocket | ✅ | ✅ | ✅ |
| etcd (Embedded) | ✅ | ✅ | ✅ |
| etcd (External) | ✅ | ✅ | ✅ |
| PostgreSQL | ✅ | ✅ | ✅ |
| SQLite | ✅ | ✅ | N/A |
| Agent Connectivity | ✅ | ✅ | ✅ |
| Metrics Endpoint | ✅ | ✅ | ✅ |
| Health Checks | ✅ | ✅ | ✅ |
| Agent Targeting | ✅ | ✅ | ✅ |
