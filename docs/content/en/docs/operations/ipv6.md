---
title: "IPv6 Deployment"
weight: 15
description: >
  Deploy Keystone Core in IPv6-only, IPv4-only, or dual-stack network environments
---

## Overview

Keystone Core provides full support for IPv6 networking, enabling deployments in:

- **IPv6-only environments** - Data centers and cloud regions with no IPv4
- **Dual-stack environments** - Networks supporting both IPv4 and IPv6
- **Transitional environments** - Migrating from IPv4 to IPv6

All components (control plane, agents, NATS, etcd, PostgreSQL) support IPv6 addressing with proper configuration and validation.

## IPv6 Address Formats

### Supported Formats

Keystone Core accepts IPv6 addresses in all standard notations:

| Format | Example | Description |
|--------|---------|-------------|
| Full | `2001:0db8:85a3:0000:0000:8a2e:0370:7334` | All 8 groups |
| Compressed | `2001:db8:85a3::8a2e:370:7334` | Consecutive zeros compressed |
| Loopback | `::1` | Local loopback |
| All interfaces | `::` | Bind to all interfaces |
| IPv4-mapped | `::ffff:192.168.1.1` | IPv4 in IPv6 notation |
| Link-local | `fe80::1` | Link-local addresses |

### URL Format (Brackets Required)

When specifying IPv6 addresses in URLs or with ports, brackets are required:

```
# Correct
nats://[2001:db8::1]:4222
grpc://[::1]:8080
https://[2001:db8:85a3::8a2e:370:7334]:443

# Incorrect (will fail)
nats://2001:db8::1:4222
grpc://::1:8080
```

## Address Family Preference

Configure how Keystone Core selects addresses when both IPv4 and IPv6 are available:

```yaml
# Control Plane
server:
  cluster:
    address_family: prefer_ipv6  # Options: prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only

# Agent
agent:
  address_family: prefer_ipv6
```

| Option | Behavior |
|--------|----------|
| `prefer_ipv4` | Use IPv4 when available, fall back to IPv6 |
| `prefer_ipv6` | Use IPv6 when available, fall back to IPv4 |
| `ipv4_only` | Only use IPv4, fail if unavailable |
| `ipv6_only` | Only use IPv6, fail if unavailable |

## Deployment Patterns

### IPv6-Only Control Plane

Deploy the control plane exclusively on IPv6:

```yaml
# /etc/keystone-core/server.yaml
api:
  grpc:
    listen: "[::]:8080"
  rest:
    listen: "[::]:8081"

metrics:
  listen: "[::]:9090"

health:
  listen: "[::]:9091"

nats:
  mode: embedded
  listen: "[::]:4222"
  cluster:
    listen: "[::]:6222"

cluster:
  enabled: true
  address_family: ipv6_only
  advertise_address: "2001:db8::1"
  etcd:
    mode: embedded
    embedded:
      listen_address: "::"
      advertise_address: "2001:db8::1"
      client_port: 2379
      peer_port: 2380

state:
  backend: postgresql
  postgresql:
    host: "2001:db8::10"
    port: 5432
    database: kscore
    user: kscore
    sslmode: require
```

### Dual-Stack Control Plane

Bind to both IPv4 and IPv6 for maximum compatibility:

```yaml
# /etc/keystone-core/server.yaml
api:
  grpc:
    listen:
      - "[::]:8080"      # IPv6
      - "0.0.0.0:8080"   # IPv4
  rest:
    listen:
      - "[::]:8081"
      - "0.0.0.0:8081"

metrics:
  listen:
    - "[::]:9090"
    - "0.0.0.0:9090"

nats:
  mode: external
  urls:
    - "nats://[2001:db8::1]:4222"   # IPv6 primary
    - "nats://10.0.1.1:4222"         # IPv4 fallback

cluster:
  enabled: true
  address_family: prefer_ipv6
  advertise_address: "2001:db8::1"
  etcd:
    mode: external
    endpoints:
      - "http://[2001:db8::1]:2379"
      - "http://[2001:db8::2]:2379"
      - "http://10.0.1.1:2379"       # IPv4 fallback
```

### IPv6-Only Agent

Configure agents to connect via IPv6:

```yaml
# /etc/keystone-core/agent.yaml
agent:
  address_family: ipv6_only

nats:
  urls:
    - "nats://[2001:db8::1]:4222"
    - "nats://[2001:db8::2]:4222"
```

### Dual-Stack Agent

Agents with fallback between address families:

```yaml
# /etc/keystone-core/agent.yaml
agent:
  address_family: prefer_ipv6

nats:
  urls:
    - "nats://[2001:db8::1]:4222"   # IPv6 primary
    - "nats://10.0.1.1:4222"         # IPv4 fallback
  connection:
    failover_enabled: true
```

## Component-Specific Configuration

### NATS

#### Embedded NATS with IPv6

```yaml
nats:
  mode: embedded
  listen: "[::]:4222"
  cluster:
    enabled: true
    listen: "[::]:6222"
    routes:
      - "nats://[2001:db8::2]:6222"
      - "nats://[2001:db8::3]:6222"
```

#### External NATS Cluster

```yaml
nats:
  mode: external
  urls:
    - "nats://[2001:db8::1]:4222"
    - "nats://[2001:db8::2]:4222"
    - "nats://[2001:db8::3]:4222"
```

#### WebSocket over IPv6

```yaml
nats:
  websocket:
    enabled: true
    listen: "[::]:8443"
    tls:
      enabled: true
      cert_file: /etc/keystone-core/tls/server.crt
      key_file: /etc/keystone-core/tls/server.key
```

### etcd

#### Embedded etcd with IPv6

```yaml
cluster:
  etcd:
    mode: embedded
    embedded:
      data_dir: /var/lib/keystone-core/etcd
      listen_address: "::"
      advertise_address: "2001:db8::1"
      client_port: 2379
      peer_port: 2380
      initial_cluster: "node1=http://[2001:db8::1]:2380,node2=http://[2001:db8::2]:2380,node3=http://[2001:db8::3]:2380"
```

#### External etcd Cluster

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

### PostgreSQL

#### Using Structured Config (Recommended)

```yaml
state:
  backend: postgresql
  postgresql:
    host: "2001:db8::10"      # IPv6 address (brackets added automatically)
    port: 5432
    database: kscore
    user: kscore
    password: "${KSCORE_DB_PASSWORD}"
    sslmode: require
    sslrootcert: /etc/keystone-core/tls/pg-ca.crt
```

#### Using DSN

```yaml
state:
  backend: postgresql
  postgresql_dsn: "host=[2001:db8::10] port=5432 dbname=kscore user=kscore sslmode=require"
```

**Note:** When using DSN format, IPv6 addresses must be enclosed in brackets.

### gRPC Cluster Communication

```yaml
cluster:
  enabled: true
  member_id: "node-1"
  advertise_address: "2001:db8::1"
  grpc_port: 9090
  address_family: prefer_ipv6
```

## Agent Targeting with IPv6

### Targeting by IPv6 Address

Target agents using their IPv6 addresses:

```bash
# Target specific IPv6 address
kscorectl exec run --target 'ipv6:2001:db8::5' -- hostname

# Target IPv6 prefix (CIDR)
kscorectl exec run --target 'ipv6_cidr:2001:db8::/32' -- uptime

# Combine with other selectors
kscorectl exec run --target 'ipv6:2001:db8::* AND role:webserver' -- systemctl status nginx
```

### Agent Metadata

Agents report both IPv4 and IPv6 addresses in their metadata:

```yaml
agent:
  id: "agent-001"
  hostname: "server1.example.com"
  ipv4_addresses:
    - "10.0.1.5"
  ipv6_addresses:
    - "2001:db8::5"
    - "fe80::1"
  address_family: dual_stack
```

## High Availability with IPv6

### Three-Node HA Cluster

```yaml
# Node 1: /etc/keystone-core/server.yaml
cluster:
  enabled: true
  member_id: "node-1"
  member_name: "Control Plane 1"
  advertise_address: "2001:db8::1"
  grpc_port: 9090
  address_family: ipv6_only
  etcd:
    mode: embedded
    embedded:
      listen_address: "::"
      advertise_address: "2001:db8::1"
      initial_cluster: "node-1=http://[2001:db8::1]:2380,node-2=http://[2001:db8::2]:2380,node-3=http://[2001:db8::3]:2380"
      initial_cluster_state: new

nats:
  mode: embedded
  listen: "[::]:4222"
  cluster:
    enabled: true
    listen: "[::]:6222"
    routes:
      - "nats://[2001:db8::2]:6222"
      - "nats://[2001:db8::3]:6222"

state:
  backend: postgresql
  postgresql:
    host: "2001:db8::10"
    port: 5432
    database: kscore
```

## Monitoring IPv6 Deployments

### Prometheus Scraping

Configure Prometheus to scrape IPv6 endpoints:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'kscore-control-plane'
    static_configs:
      - targets:
          - '[2001:db8::1]:9090'
          - '[2001:db8::2]:9090'
          - '[2001:db8::3]:9090'

  - job_name: 'kscore-agents'
    static_configs:
      - targets:
          - '[2001:db8::10]:9100'
          - '[2001:db8::11]:9100'
```

### IPv6 Connection Metrics

Monitor connection health by address family:

```promql
# Connections by address family
sum(kscore_connections_total) by (family)

# Connection failures by family
sum(rate(kscore_connection_failures_total[5m])) by (family)

# Agents by address family
kscore_agents_by_address_family
```

## Troubleshooting

### Common Issues

#### Connection Refused on IPv6

**Symptom:** Agent cannot connect to control plane on IPv6.

**Check:**
```bash
# Verify control plane is listening on IPv6
ss -tlnp | grep 8080

# Should show [::]:8080 for IPv6
tcp   LISTEN  0  128  [::]:8080  [::]:*  users:(("kscore-server"...))

# Test connectivity
nc -6 -zv 2001:db8::1 8080
```

**Solution:** Ensure the control plane config uses IPv6 listen address:
```yaml
api:
  grpc:
    listen: "[::]:8080"
```

#### Invalid IPv6 Address Error

**Symptom:** Configuration fails with "invalid IPv6 address" error.

**Check:** Ensure brackets are used in URLs:
```yaml
# Wrong
nats:
  urls:
    - "nats://2001:db8::1:4222"

# Correct
nats:
  urls:
    - "nats://[2001:db8::1]:4222"
```

#### etcd Cluster Formation Fails

**Symptom:** etcd nodes cannot form cluster over IPv6.

**Check:**
```bash
# Verify etcd is listening
ss -tlnp | grep 2379
ss -tlnp | grep 2380

# Check etcd member list
etcdctl --endpoints=http://[2001:db8::1]:2379 member list
```

**Solution:** Ensure initial_cluster uses bracketed IPv6:
```yaml
initial_cluster: "n1=http://[2001:db8::1]:2380,n2=http://[2001:db8::2]:2380"
```

#### PostgreSQL Connection Fails

**Symptom:** Cannot connect to PostgreSQL on IPv6.

**Check:**
```bash
# Verify PostgreSQL accepts IPv6 connections
psql -h 2001:db8::10 -U kscore -d kscore

# Check pg_hba.conf allows IPv6
# host  kscore  kscore  2001:db8::/32  scram-sha-256
```

**Solution:** Use structured config or bracketed DSN:
```yaml
# Structured (recommended)
postgresql:
  host: "2001:db8::10"

# DSN
postgresql_dsn: "host=[2001:db8::10] port=5432 ..."
```

### Diagnostic Commands

```bash
# Check IPv6 connectivity
ping6 2001:db8::1

# Test NATS connection
nats-cli -s nats://[2001:db8::1]:4222 pub test "hello"

# Check agent IPv6 addresses
kscorectl agents list -o wide

# View control plane IPv6 bindings
kscorectl cluster status
```

### Firewall Rules

#### iptables (IPv6)

```bash
# Allow NATS
ip6tables -A INPUT -p tcp --dport 4222 -j ACCEPT
ip6tables -A INPUT -p tcp --dport 6222 -j ACCEPT

# Allow gRPC API
ip6tables -A INPUT -p tcp --dport 8080 -j ACCEPT

# Allow metrics
ip6tables -A INPUT -p tcp --dport 9090 -j ACCEPT

# Allow etcd
ip6tables -A INPUT -p tcp --dport 2379 -j ACCEPT
ip6tables -A INPUT -p tcp --dport 2380 -j ACCEPT
```

#### nftables

```bash
# /etc/nftables.conf
table ip6 filter {
    chain input {
        type filter hook input priority 0; policy drop;

        # Allow established
        ct state established,related accept

        # Allow loopback
        iif lo accept

        # Keystone Core ports
        tcp dport { 4222, 6222, 8080, 8081, 9090, 2379, 2380 } accept
    }
}
```

#### firewalld (RHEL/CentOS/Fedora)

```bash
# Add services
firewall-cmd --permanent --add-port=4222/tcp  # NATS client
firewall-cmd --permanent --add-port=6222/tcp  # NATS cluster
firewall-cmd --permanent --add-port=8080/tcp  # gRPC API
firewall-cmd --permanent --add-port=8081/tcp  # REST API
firewall-cmd --permanent --add-port=9090/tcp  # Metrics
firewall-cmd --permanent --add-port=2379/tcp  # etcd client
firewall-cmd --permanent --add-port=2380/tcp  # etcd peer
firewall-cmd --reload

# Verify
firewall-cmd --list-all
```

#### ufw (Ubuntu/Debian)

```bash
# Allow Keystone Core ports for IPv6
ufw allow proto tcp from any to any port 4222 comment 'NATS client'
ufw allow proto tcp from any to any port 6222 comment 'NATS cluster'
ufw allow proto tcp from any to any port 8080 comment 'gRPC API'
ufw allow proto tcp from any to any port 8081 comment 'REST API'
ufw allow proto tcp from any to any port 9090 comment 'Metrics'
ufw allow proto tcp from any to any port 2379 comment 'etcd client'
ufw allow proto tcp from any to any port 2380 comment 'etcd peer'

# Enable IPv6 in UFW
# Ensure IPV6=yes in /etc/default/ufw
ufw enable
```

#### Windows Firewall

```powershell
# Create firewall rules for IPv6
New-NetFirewallRule -DisplayName "Keystone NATS Client" -Direction Inbound -Protocol TCP -LocalPort 4222 -Action Allow
New-NetFirewallRule -DisplayName "Keystone NATS Cluster" -Direction Inbound -Protocol TCP -LocalPort 6222 -Action Allow
New-NetFirewallRule -DisplayName "Keystone gRPC API" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
New-NetFirewallRule -DisplayName "Keystone REST API" -Direction Inbound -Protocol TCP -LocalPort 8081 -Action Allow
New-NetFirewallRule -DisplayName "Keystone Metrics" -Direction Inbound -Protocol TCP -LocalPort 9090 -Action Allow
New-NetFirewallRule -DisplayName "Keystone etcd Client" -Direction Inbound -Protocol TCP -LocalPort 2379 -Action Allow
New-NetFirewallRule -DisplayName "Keystone etcd Peer" -Direction Inbound -Protocol TCP -LocalPort 2380 -Action Allow
```

### Cloud Provider Firewall Configuration

#### AWS Security Groups

```hcl
# Terraform example for IPv6 security group
resource "aws_security_group" "kscore" {
  name        = "kscore-control-plane"
  description = "Keystone Core control plane IPv6"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port        = 4222
    to_port          = 4222
    protocol         = "tcp"
    ipv6_cidr_blocks = ["::/0"]
    description      = "NATS client"
  }

  ingress {
    from_port        = 8080
    to_port          = 8080
    protocol         = "tcp"
    ipv6_cidr_blocks = ["::/0"]
    description      = "gRPC API"
  }

  # Add other ports as needed
}
```

#### GCP Firewall Rules

```bash
# Create IPv6 firewall rule
gcloud compute firewall-rules create kscore-ipv6 \
  --network=default \
  --allow=tcp:4222,tcp:6222,tcp:8080,tcp:8081,tcp:9090 \
  --source-ranges=::/0 \
  --description="Keystone Core IPv6 access"
```

#### Azure Network Security Groups

```bash
# Create NSG rules for IPv6
az network nsg rule create \
  --nsg-name kscore-nsg \
  --resource-group kscore-rg \
  --name Allow-NATS-IPv6 \
  --priority 100 \
  --source-address-prefixes '::/0' \
  --destination-port-ranges 4222 \
  --access Allow \
  --protocol Tcp
```

## Migration Guide

### IPv4 to Dual-Stack

1. **Update control plane config** to bind to both families:
   ```yaml
   api:
     grpc:
       listen:
         - "[::]:8080"
         - "0.0.0.0:8080"
   ```

2. **Add IPv6 endpoints** to existing IPv4:
   ```yaml
   nats:
     urls:
       - "nats://[2001:db8::1]:4222"
       - "nats://10.0.1.1:4222"
   ```

3. **Update agents** with new endpoints and preference:
   ```yaml
   agent:
     address_family: prefer_ipv6
   ```

4. **Verify connectivity** on both families:
   ```bash
   kscorectl agents list -o wide
   ```

### Dual-Stack to IPv6-Only

1. **Verify all agents support IPv6:**
   ```bash
   kscorectl exec run --target 'ipv6:*' -- echo "IPv6 OK"
   ```

2. **Update address_family** to ipv6_only:
   ```yaml
   cluster:
     address_family: ipv6_only
   ```

3. **Remove IPv4 endpoints** from configuration.

4. **Update firewall rules** to close IPv4 ports.

## Extended Troubleshooting

### Connectivity Testing Matrix

When troubleshooting IPv6 issues, work through this matrix:

| Test | Command | Expected Result |
|------|---------|-----------------|
| Local IPv6 | `ping6 ::1` | Response from localhost |
| Interface IPv6 | `ip -6 addr show` | Shows configured addresses |
| Gateway reachability | `ping6 <gateway>` | Response from gateway |
| DNS resolution | `dig AAAA kscore.example.com` | Returns IPv6 address |
| Port listening | `ss -6 -tlnp` | Shows services on IPv6 |
| Remote connectivity | `nc -6 -zv <addr> <port>` | Connection established |

### Common Failure Modes

#### Failure: Agent Cannot Connect Over IPv6

**Symptoms:**
- Agent logs show connection timeouts
- Control plane doesn't see agent registration

**Diagnostic Steps:**
```bash
# 1. Check agent can reach control plane
ping6 2001:db8::1

# 2. Check port is reachable
nc -6 -zv 2001:db8::1 4222

# 3. Check agent configuration
grep -r "address_family\|nats" /etc/keystone-core/agent.yaml

# 4. Check local firewall
ip6tables -L -n
```

**Solutions:**
- Enable IPv6 routing: `sysctl net.ipv6.conf.all.forwarding=1`
- Add firewall rule: `ip6tables -A INPUT -p tcp --dport 4222 -j ACCEPT`
- Verify NATS URL has brackets: `nats://[2001:db8::1]:4222`

#### Failure: etcd Cluster Won't Form

**Symptoms:**
- etcd logs show connection refused or timeout
- Cluster status shows unhealthy members

**Diagnostic Steps:**
```bash
# 1. Check etcd is listening on IPv6
ss -tlnp | grep 2379
ss -tlnp | grep 2380

# 2. Test peer connectivity
nc -6 -zv 2001:db8::2 2380

# 3. Check etcd member list
ETCDCTL_API=3 etcdctl --endpoints=http://[::1]:2379 member list

# 4. Check initial cluster configuration
grep initial_cluster /etc/keystone-core/server.yaml
```

**Solutions:**
- Ensure initial_cluster uses bracketed addresses
- Verify peer ports are open in firewall
- Check advertise_address is correct and routable

#### Failure: PostgreSQL Connection Fails

**Symptoms:**
- Server logs show "could not connect to database"
- Authentication succeeds but connection drops

**Diagnostic Steps:**
```bash
# 1. Test direct PostgreSQL connection
psql -h 2001:db8::10 -U kscore -d kscore

# 2. Check PostgreSQL is listening on IPv6
sudo -u postgres psql -c "SHOW listen_addresses;"

# 3. Check pg_hba.conf has IPv6 entries
sudo cat /etc/postgresql/*/main/pg_hba.conf | grep -v "^#" | grep ":"

# 4. Check SSL mode
psql "host=2001:db8::10 sslmode=require" -c "SELECT 1;"
```

**Solutions:**
- Set `listen_addresses = '*'` in postgresql.conf
- Add IPv6 entry to pg_hba.conf: `host kscore kscore ::/0 scram-sha-256`
- Use structured config instead of DSN to avoid bracket issues

### Performance Debugging

#### Measure IPv6 vs IPv4 Latency

```bash
# Compare latency
ping -c 100 10.0.1.1 | tail -1
ping6 -c 100 2001:db8::1 | tail -1

# Check for MTU issues (IPv6 minimum is 1280)
ping6 -M do -s 1452 2001:db8::1

# Check path MTU
tracepath6 2001:db8::1
```

#### Monitor Connection Distribution

```promql
# Connection distribution by address family
sum(kscore_connections_total) by (family)

# Alert if IPv6 connections drop unexpectedly
ALERT IPv6ConnectionsDegraded
  IF sum(rate(kscore_connection_failures_total{family="ipv6"}[5m]))
     / sum(rate(kscore_connections_total{family="ipv6"}[5m])) > 0.1
  FOR 5m
  LABELS { severity = "warning" }
```

### Kernel Parameters

Ensure these sysctl settings for reliable IPv6:

```bash
# /etc/sysctl.d/99-kscore-ipv6.conf

# Enable IPv6
net.ipv6.conf.all.disable_ipv6 = 0
net.ipv6.conf.default.disable_ipv6 = 0

# Enable forwarding (control plane nodes)
net.ipv6.conf.all.forwarding = 1

# Increase neighbor cache (large deployments)
net.ipv6.neigh.default.gc_thresh1 = 1024
net.ipv6.neigh.default.gc_thresh2 = 2048
net.ipv6.neigh.default.gc_thresh3 = 4096

# Prefer IPv6 temporary addresses (privacy)
net.ipv6.conf.all.use_tempaddr = 0

# Apply changes
sysctl -p /etc/sysctl.d/99-kscore-ipv6.conf
```

## Best Practices

1. **Use structured config** for PostgreSQL to avoid DSN bracket issues
2. **Set explicit address_family** preference in production
3. **Include both IPv4 and IPv6** endpoints for transitional periods
4. **Monitor connections by family** to detect issues early
5. **Test failover** between address families before production
6. **Document IP allocations** for both families in your infrastructure
7. **Use DNS names** where possible to abstract address family selection
8. **Configure kernel parameters** for IPv6 on all nodes
9. **Test MTU path** to avoid fragmentation issues
10. **Use consistent firewall rules** across all control plane nodes
