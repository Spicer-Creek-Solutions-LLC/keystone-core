# Runbook: Troubleshooting Guide

## Overview

This runbook provides troubleshooting procedures for common Keystone Core issues.

## Quick Diagnostics

```bash
# Collect full diagnostics package
kscorectl diagnostics collect --output /tmp/diagnostics-$(date +%Y%m%d).tar.gz

# Quick health check
kscorectl cluster health
kscorectl agent list --status | head -20
```

## Issue Categories

1. [Cluster Issues](#cluster-issues)
2. [Agent Issues](#agent-issues)
3. [NATS Issues](#nats-issues)
4. [Database Issues](#database-issues)
5. [State Application Issues](#state-application-issues)
6. [Performance Issues](#performance-issues)

---

## Cluster Issues

### Cluster Has No Leader

**Symptoms:**

- `kscorectl cluster leader` returns empty
- API requests fail or timeout
- State applications don't execute

**Diagnosis:**

```bash
# Check cluster members
kscorectl cluster members

# Check etcd health
etcdctl endpoint health --endpoints=https://localhost:2379

# Check server logs
journalctl -u kscore-server -n 100 | grep -i "election\|leader"
```

**Resolution:**

```bash
# If quorum lost, check if majority of nodes are reachable
for node in ks-server-1 ks-server-2 ks-server-3; do
  ping -c 1 $node && echo "$node reachable" || echo "$node unreachable"
done

# If nodes are up but no leader:
# 1. Check network partitions
# 2. Restart election
kscorectl cluster election restart

# If quorum truly lost, see disaster recovery runbook
```

### Node Won't Join Cluster

**Symptoms:**

- New node fails to join
- Node shows as "unhealthy" after join

**Diagnosis:**

```bash
# Verify cluster connectivity (token validation happens during join)
curl -k https://existing-node:8080/health/ready

# Check network connectivity
nc -zv existing-node 8080
nc -zv existing-node 6222
nc -zv existing-node 2379

# Check TLS certificates
openssl s_client -connect existing-node:8080 </dev/null
```

**Resolution:**

```bash
# Get join token from cluster configuration
# Token regeneration requires updating cluster config and restarting control plane
cat /etc/keystone-core/server.yaml | grep -A5 cluster

# Check firewall
sudo iptables -L -n | grep -E "8080|6222|2379"

# Force rejoin
kscore-bootstrap import --join https://leader:8080 --token $TOKEN --force
```

### Split-Brain Detected

**Symptoms:**

- Multiple nodes claim leadership
- Inconsistent data between partitions

**Diagnosis:**

```bash
# Check for multiple leaders
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "kscorectl cluster leader"
done
```

**Resolution:**

```bash
# 1. Identify authoritative partition (most nodes, or most recent data)
# 2. Stop servers in non-authoritative partition
ssh ks-server-3 "sudo systemctl stop kscore-server"

# 3. Wait for authoritative partition to stabilize
sleep 30
kscorectl cluster health

# 4. Rejoin nodes from non-authoritative partition
ssh ks-server-3 "kscore-bootstrap import --join https://ks-server-1:8080 --force-rejoin"
```

---

## Agent Issues

### Agent Not Connecting

**Symptoms:**

- Agent shows offline
- No heartbeats received

**Diagnosis:**

```bash
# On agent node:
# Check agent status
systemctl status kscore-agent

# Check agent logs
journalctl -u kscore-agent -n 100

# Test connectivity to server
curl -k https://server:8080/health/live
nats-cli -s nats://server:4222 ping
```

**Resolution:**

```bash
# Check agent configuration
cat /etc/keystone-core/agent.yaml

# Verify server URL is correct
# Verify credentials are valid

# Restart agent
systemctl restart kscore-agent

# If TLS issues:
# Verify CA certificate is correct
openssl verify -CAfile /etc/keystone-core/certs/ca.crt /etc/keystone-core/certs/agent.crt
```

### Agent Heartbeat Timeout

**Symptoms:**

- Agent intermittently shows offline
- "Heartbeat timeout" in server logs

**Diagnosis:**

```bash
# Check network latency
ping -c 10 agent-node

# Check for packet loss
mtr agent-node

# Check agent resource usage
ssh agent-node "top -bn1 | head -20"
```

**Resolution:**

```bash
# Increase heartbeat timeout
# In server config:
agent:
  heartbeat_timeout: 60s  # Increase from default 30s

# Restart server
systemctl restart kscore-server

# If high CPU on agent:
# Check for runaway processes
ssh agent-node "ps aux --sort=-%cpu | head -10"
```

### Agent Version Mismatch

**Symptoms:**

- Agents on different versions
- Feature incompatibility

**Diagnosis:**

```bash
# Check agent versions
kscorectl agent list --show-version

# Group by version
kscorectl agent list --show-version | awk '{print $NF}' | sort | uniq -c
```

**Resolution:**

```bash
# Upgrade outdated agents
kscorectl upgrade agents --target 1.6.0

# Or upgrade specific agents
kscorectl upgrade agents --target 1.6.0 --filter "version!=1.6.0"
```

---

## NATS Issues

### NATS Connection Failed

**Symptoms:**

- "NATS connection refused"
- Events not being delivered

**Diagnosis:**

```bash
# Check NATS status
systemctl status nats-server

# Check NATS logs
journalctl -u nats-server -n 100

# Test NATS connectivity
nats-cli -s nats://localhost:4222 server info
```

**Resolution:**

```bash
# Restart NATS
systemctl restart nats-server

# If persistent issues, check config
cat /etc/nats/nats.conf

# Verify ports are open
netstat -tlnp | grep 4222
```

### JetStream Not Working

**Symptoms:**

- Events not being stored
- Stream creation fails

**Diagnosis:**

```bash
# Check JetStream status
nats-cli stream list

# Check JetStream storage
df -h /var/lib/nats/jetstream

# Check JetStream logs
journalctl -u nats-server | grep -i jetstream
```

**Resolution:**

```bash
# If storage full:
# 1. Clean old messages
nats-cli stream purge EVENTS --force

# 2. Increase storage quota
# Edit /etc/nats/nats.conf
jetstream {
  max_file_store: 50GB  # Increase limit
}

# Restart NATS
systemctl restart nats-server
```

---

## Database Issues

### Database Connection Failed

**Symptoms:**

- "Database connection refused"
- State queries fail

**Diagnosis:**

```bash
# For PostgreSQL:
psql -h localhost -U keystone -d keystone -c "SELECT 1"

# Check PostgreSQL logs
sudo tail -100 /var/log/postgresql/postgresql-*.log

# For SQLite:
sqlite3 /var/lib/keystone-core/state.db "SELECT count(*) FROM agents"
```

**Resolution:**

```bash
# PostgreSQL not running:
systemctl restart postgresql

# Connection limit reached:
psql -c "SELECT count(*) FROM pg_stat_activity"
# Increase max_connections in postgresql.conf

# SQLite locked:
# Check for long-running transactions
lsof /var/lib/keystone-core/state.db
```

### Database Corruption

**Symptoms:**

- Query errors
- Inconsistent data

**Diagnosis:**

```bash
# PostgreSQL:
psql -c "VACUUM ANALYZE"

# SQLite:
sqlite3 /var/lib/keystone-core/state.db "PRAGMA integrity_check"
```

**Resolution:**

```bash
# Restore from backup
kscore-bootstrap restore \
  --backup /backup/latest.tar.gz \
  --components database

# Or repair if possible:
# SQLite:
sqlite3 /var/lib/keystone-core/state.db ".recover" | sqlite3 /var/lib/keystone-core/state-new.db
mv /var/lib/keystone-core/state-new.db /var/lib/keystone-core/state.db
```

---

## State Application Issues

### State Application Fails

**Symptoms:**

- State shows "failed"
- Changes not applied

**Diagnosis:**

```bash
# Check state with dry-run to see current status
kscorectl state apply my-state.yaml --dry-run

# Check agent logs on target
ssh agent-node "journalctl -u kscore-agent -n 100"

# Check state definition
kscorectl state show my-state
```

**Resolution:**

```bash
# Retry state application
kscorectl state apply my-state.yaml --force

# If module error:
# Check module availability
kscorectl module list

# Check module logs
kscorectl state debug my-state
```

### State Drift Detected

**Symptoms:**

- Drift alerts
- State shows "drifted"

**Diagnosis:**

```bash
# Check drift status
kscorectl state drift my-state

# Compare desired vs actual
kscorectl state diff my-state
```

**Resolution:**

```bash
# Re-apply state to fix drift
kscorectl state apply my-state.yaml

# Or update state to match reality
kscorectl state update my-state --from-actual
```

---

## Performance Issues

### High CPU Usage

**Diagnosis:**

```bash
# Check process CPU
top -bn1 | grep kscore

# Check goroutine count
curl -s http://localhost:8080/debug/pprof/goroutine?debug=1 | head -20

# Collect CPU profile
curl -s "http://localhost:8080/debug/pprof/profile?seconds=30" > cpu.pprof
go tool pprof cpu.pprof
```

**Resolution:**

```bash
# If too many goroutines:
# Check for connection leaks
netstat -an | grep 8080 | wc -l

# Restart server to reset
systemctl restart kscore-server
```

### High Memory Usage

**Diagnosis:**

```bash
# Check memory usage
free -h
ps aux | grep kscore

# Collect heap profile
curl -s http://localhost:8080/debug/pprof/heap > heap.pprof
go tool pprof heap.pprof
```

**Resolution:**

```bash
# If memory leak suspected:
# Enable GC debugging
GODEBUG=gctrace=1 kscore-server

# Restart to free memory
systemctl restart kscore-server

# Consider increasing memory limits
# Edit systemd unit:
# MemoryMax=4G
```

### Slow API Responses

**Diagnosis:**

```bash
# Check API latency
curl -w "@curl-format.txt" -k https://localhost:8080/api/v1/agents

# Check database query times
psql -c "SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10"

# Check for slow operations
journalctl -u kscore-server | grep -i "slow\|timeout"
```

**Resolution:**

```bash
# If database slow:
psql -c "VACUUM ANALYZE"

# If too many agents:
# Consider scaling horizontally

# If network latency:
# Check with traceroute/mtr
```

---

## Collecting Diagnostics

When opening a support case, collect:

```bash
# Full diagnostic bundle
kscorectl diagnostics collect \
  --include-logs \
  --include-config \
  --include-metrics \
  --output /tmp/diagnostics.tar.gz

# This includes:
# - Server/agent logs (last 24h)
# - Configuration files (redacted)
# - Prometheus metrics snapshot
# - Cluster state dump
# - System info (uname, memory, disk)
```

## Getting Help

- Documentation: <https://docs.keystone.io>
- GitHub Issues: <https://github.com/shawnbutts/keystone-core/issues>
- Community Slack: <https://keystone-community.slack.com>
- Enterprise Support: <support@keystone.io>
