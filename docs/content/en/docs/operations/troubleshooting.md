---
title: "Troubleshooting Guide"
weight: 4
description: >
  Diagnostic procedures and solutions for common Keystone Core issues
---

## Overview

This guide provides systematic troubleshooting procedures for common Keystone Core issues. Each section includes diagnostic steps, common causes, and proven solutions.

**Troubleshooting Methodology:**
1. **Identify symptoms** - What is broken?
2. **Check logs** - What do the logs say?
3. **Verify configuration** - Is config correct?
4. **Test connectivity** - Can components reach each other?
5. **Check resources** - Are resources exhausted?
6. **Apply fix** - Implement solution
7. **Verify resolution** - Confirm issue resolved

## Agent Connectivity Issues

### Symptoms
- Agents show as "offline" in `kscorectl agent list`
- Agents cannot connect to control plane
- Heartbeat failures

### Diagnostic Steps

**1. Check Agent Status:**
```bash
# On agent node
sudo systemctl status kscore-agent

# Check agent logs
sudo journalctl -u kscore-agent -f
```

**2. Test NATS Connectivity:**
```bash
# From agent node, test NATS connection
nc -zv nats-server 4222

# Or use telnet
telnet nats-server 4222
```

**3. Verify Credentials:**
```bash
# Check agent configuration
cat /etc/kscore/agent.yaml | grep -A5 nats

# Test authentication
nats-sub -s nats://username:password@nats-server:4222 test
```

### Common Causes and Solutions

**Firewall Blocking NATS Port (4222):**
```bash
# Check firewall rules
sudo iptables -L -n | grep 4222

# Allow NATS port
sudo iptables -A INPUT -p tcp --dport 4222 -j ACCEPT
sudo iptables-save
```

**DNS Resolution Failure:**
```bash
# Test DNS resolution
nslookup nats-server

# Add to /etc/hosts if DNS failing
echo "10.0.1.10 nats-server" | sudo tee -a /etc/hosts
```

**TLS Certificate Mismatch:**
```bash
# Check certificate validity
openssl s_client -connect nats-server:4222 -showcerts

# Verify certificate CN matches hostname
openssl x509 -in /etc/kscore/certs/ca.crt -text -noout | grep Subject
```

**Agent Credential Mismatch:**
```yaml
# Fix credentials in agent.yaml
nats:
  url: "nats://nats-server:4222"
  credentials:
    username: "kscore"
    password: "correct-password"  # Update here
```

**NATS Server Down:**
```bash
# Check NATS server status
sudo systemctl status nats-server

# Start if stopped
sudo systemctl start nats-server

# Check cluster health
nats server check connection
```

**Network Partition:**
```bash
# Test network latency
ping nats-server

# Traceroute to identify network issues
traceroute nats-server

# Check for packet loss
mtr -r nats-server
```

## NATS Connection Problems

### Symptoms
- Control plane cannot connect to NATS
- "connection refused" errors
- "authentication failed" errors
- JetStream unavailable

### Diagnostic Steps

**1. Check NATS Server:**
```bash
# NATS server status
sudo systemctl status nats-server

# NATS server logs
sudo journalctl -u nats-server -f

# Check listening ports
sudo netstat -tlnp | grep nats
```

**2. Test NATS CLI:**
```bash
# Publish test message
nats pub test "hello"

# Subscribe to test subject
nats sub test
```

**3. Check JetStream:**
```bash
# JetStream account info
nats account info

# List streams
nats stream list
```

### Common Causes and Solutions

**NATS Server Not Running:**
```bash
# Start NATS
sudo systemctl start nats-server

# Enable auto-start
sudo systemctl enable nats-server
```

**JetStream Not Enabled:**
```conf
# nats-server.conf
jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 8GB
  max_file_store: 100GB
}
```

```bash
# Restart NATS to apply
sudo systemctl restart nats-server
```

**Cluster Split-Brain:**
```bash
# Check cluster status
nats server check jetstream

# Expected: All nodes connected
# If split, restart minority partition nodes
```

**Out of Disk Space (JetStream):**
```bash
# Check disk usage
df -h /var/lib/nats/jetstream

# Clean old streams if needed
nats stream purge old-stream --force
nats stream delete unused-stream
```

**Memory Exhaustion:**
```bash
# Check NATS memory usage
nats server check connection

# Increase memory limits in nats-server.conf
max_payload: 8MB
max_pending_size: 512MB
```

## State Application Failures

### Symptoms
- `kscorectl state apply` fails
- State resources show as "failed"
- Error: "state application timeout"
- Drift detection not working

### Diagnostic Steps

**1. Check State File Syntax:**
```bash
# Validate YAML syntax
kscore-state check web-server.yaml

# Look for syntax errors
yamllint web-server.yaml
```

**2. Check Agent Logs:**
```bash
# On target agent
sudo journalctl -u kscore-agent | grep state
```

**3. Test Individual States:**
```bash
# Apply single state for debugging
kscore-state apply nginx-package.yaml --target "web-01"
```

### Common Causes and Solutions

**Invalid YAML Syntax:**
```yaml
# Bad (tabs instead of spaces)
nginx_config:
	module: file  # ERROR: tab character

# Good (spaces only)
nginx_config:
  module: file
```

**Module Not Available on Agent:**
```bash
# Check available modules on agent
kscorectl exec run "kscore-agent --list-modules" --target "web-01"

# Install missing module package manager
sudo apt-get install python3-apt  # For apt module
```

**File Path Doesn't Exist:**
```yaml
# Bad
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf  # Parent dir doesn't exist

# Good - create parent first
nginx_conf_dir:
  module: file
  state: directory
  path: /etc/nginx
  makedirs: true

nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  require:
    - nginx_conf_dir
```

**Circular Dependency:**
```yaml
# Bad
service_a:
  module: service
  require:
    - service_b

service_b:
  module: service
  require:
    - service_a  # Circular!
```

**Timeout on Slow Operations:**
```yaml
# Increase timeout for slow operations
large_download:
  module: cmd
  state: run
  command: wget https://example.com/large-file.iso
  timeout: "30m"  # Default is 5m
```

**Permission Denied:**
```bash
# Agent needs elevated permissions
# Run agent as root or with sudo capabilities

# Or fix file permissions
sudo chown kscore:kscore /etc/nginx/nginx.conf
```

## Performance Issues

### Symptoms
- High API latency
- Slow command execution
- Database queries slow
- High CPU/memory usage

### Diagnostic Steps

**1. Check Resource Usage:**
```bash
# CPU and memory
top -p $(pgrep kscore-server)

# Disk I/O
iostat -x 1

# Network
iftop -i eth0
```

**2. Check Database Performance:**
```bash
# PostgreSQL slow queries
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state != 'idle' AND now() - query_start > interval '5 seconds'
ORDER BY duration DESC;

# Table statistics
SELECT relname, seq_scan, idx_scan, n_tup_ins, n_tup_upd, n_tup_del
FROM pg_stat_user_tables
ORDER BY seq_scan DESC;
```

**3. Check NATS Performance:**
```bash
# NATS message rates
nats server check jetstream

# Check for slow consumers
nats consumer report
```

**4. Profile Control Plane:**
```bash
# Get pprof CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze with go tool
go tool pprof cpu.prof
```

### Common Causes and Solutions

**Insufficient CPU:**
```bash
# Check CPU usage
mpstat 1 5

# Solution: Scale vertically (more CPUs) or horizontally (more nodes)
```

**Memory Swapping:**
```bash
# Check swap usage
free -h
vmstat 1

# Solution: Increase RAM or reduce memory usage
# Disable swap for performance-critical services
sudo swapoff -a
```

**Disk I/O Bottleneck:**
```bash
# Check disk latency
iostat -x 1

# Look for high await (>10ms indicates saturation)

# Solutions:
# 1. Use SSD/NVMe instead of HDD
# 2. Add read cache
# 3. Use RAID 10 for better I/O
```

**Database Connection Pool Exhausted:**
```yaml
# Increase connection pool
storage:
  postgresql:
    pool:
      max_connections: 100  # Increase from 50
      max_idle: 20
```

**Slow Database Queries:**
```sql
-- Add missing index
CREATE INDEX idx_agents_datacenter ON agents(datacenter);
CREATE INDEX idx_events_timestamp ON events(timestamp);

-- Vacuum and analyze
VACUUM ANALYZE;
```

**Too Many Concurrent State Applications:**
```yaml
# Limit concurrency
state:
  execution:
    max_concurrent: 10  # Process 10 states at a time
```

**Large Event Backlog:**
```bash
# Check JetStream pending messages
nats stream info kscore-events

# Increase consumer processing
# Add more reactor workers
```

## Common Error Messages

### "Connection Refused"

**Meaning:** Cannot connect to specified host:port

**Check:**
```bash
# Verify service is listening
sudo netstat -tlnp | grep 8080

# Check firewall
sudo iptables -L -n | grep 8080
```

**Fix:**
```bash
# Start service
sudo systemctl start kscore-server

# Allow through firewall
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
```

### "Authentication Failed"

**Meaning:** Invalid username/password or token

**Check:**
```bash
# Verify credentials in config
cat /etc/kscore/agent.yaml | grep -A3 credentials
```

**Fix:**
```yaml
# Update credentials
nats:
  credentials:
    username: "kscore"
    password: "correct-password"
```

### "Database Connection Failed"

**Meaning:** Cannot connect to PostgreSQL

**Check:**
```bash
# Test database connection
psql -U kscore -h localhost -d titananvil -c "SELECT 1;"
```

**Fix:**
```bash
# Check PostgreSQL is running
sudo systemctl status postgresql

# Verify pg_hba.conf allows connection
# /etc/postgresql/14/main/pg_hba.conf
host    kscore      kscore      10.0.0.0/8              md5
```

### "State Application Timeout"

**Meaning:** State execution exceeded timeout

**Check:**
```bash
# Check agent logs for what's slow
sudo journalctl -u kscore-agent | grep timeout
```

**Fix:**
```yaml
# Increase timeout
slow_operation:
  module: cmd
  command: /usr/local/bin/slow-script.sh
  timeout: "30m"  # Increase from 5m default
```

### "Policy Violation: Operation Denied"

**Meaning:** Policy engine blocked the operation

**Check:**
```bash
# Check policy evaluation logs
kscorectl policy audit --resource "state/nginx-config"
```

**Fix:**
```yaml
# Update policy to allow operation
# Or request policy exception
```

### "Drift Detected: Critical"

**Meaning:** Significant configuration drift from desired state

**Check:**
```bash
# View drift details
kscorectl state drift web-server.yaml --target "web-01"
```

**Fix:**
```bash
# Reapply state to fix drift
kscorectl state apply web-server.yaml --target "web-01"
```

## Debug Logging

### Enable Debug Logs

**Control Plane:**
```yaml
# /etc/kscore/server.yaml
logging:
  level: debug  # Change from info
```

```bash
# Restart to apply
sudo systemctl restart kscore-server

# Tail debug logs
sudo journalctl -u kscore-server -f | grep DEBUG
```

**Agent:**
```yaml
# /etc/kscore/agent.yaml
logging:
  level: debug
```

```bash
sudo systemctl restart kscore-agent
sudo journalctl -u kscore-agent -f
```

**Temporary Debug (Runtime):**
```bash
# Enable debug for single command
TITAN_LOG_LEVEL=debug kscorectl state apply web-server.yaml
```

### Structured Logging

**Query Logs by Correlation ID:**
```bash
# Follow specific request
sudo journalctl -u kscore-server | grep "correlation_id=abc-123"

# Or with Loki
logcli query '{job="kscore-server"} | json | correlation_id="abc-123"'
```

**Query Logs by Component:**
```bash
# All state management logs
sudo journalctl -u kscore-server | grep '"logger":"statemgmt"'

# All event system logs
sudo journalctl -u kscore-server | grep '"logger":"events"'
```

## Network Diagnostics

### Tools

**Test Connectivity:**
```bash
# TCP connection test
nc -zv server 8080

# HTTP endpoint test
curl -v http://server:8080/health/ready

# NATS connection test
nats-sub -s nats://server:4222 test
```

**Measure Latency:**
```bash
# ICMP ping
ping server

# TCP ping
tcpping server 8080

# HTTP latency
time curl -so /dev/null http://server:8080/health/live
```

**Packet Capture:**
```bash
# Capture NATS traffic
sudo tcpdump -i eth0 -w nats.pcap port 4222

# Analyze with wireshark
wireshark nats.pcap
```

**Bandwidth Test:**
```bash
# Install iperf3
sudo apt-get install iperf3

# Server
iperf3 -s

# Client
iperf3 -c server
```

### Network Issues

**Packet Loss:**
```bash
# Detect packet loss
mtr -r server

# Check network interfaces
ifconfig eth0

# Check for errors
ethtool -S eth0 | grep error
```

**High Latency:**
```bash
# Identify slow hop
traceroute server

# Check for network congestion
iftop -i eth0
```

**MTU Issues:**
```bash
# Test MTU
ping -M do -s 1472 server  # 1500 - 28 (headers)

# If fails, lower MTU
sudo ip link set dev eth0 mtu 1400
```

## Performance Tuning

### Operating System

**File Descriptor Limits:**
```bash
# Check current limit
ulimit -n

# Increase limit
echo "* soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# Verify after reboot
ulimit -n
```

**TCP Tuning:**
```bash
# /etc/sysctl.conf
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_max_syn_backlog = 8192
net.core.somaxconn = 4096

# Apply
sudo sysctl -p
```

**Transparent Huge Pages (Disable for Databases):**
```bash
# Check status
cat /sys/kernel/mm/transparent_hugepage/enabled

# Disable
echo never | sudo tee /sys/kernel/mm/transparent_hugepage/enabled
```

### PostgreSQL Tuning

```ini
# /etc/postgresql/14/main/postgresql.conf

# Memory
shared_buffers = 4GB  # 25% of RAM
effective_cache_size = 12GB  # 75% of RAM
work_mem = 16MB  # Per operation
maintenance_work_mem = 512MB

# Connections
max_connections = 200

# WAL
wal_buffers = 16MB
checkpoint_completion_target = 0.9

# Query Planner
random_page_cost = 1.1  # For SSD
effective_io_concurrency = 200  # For SSD
```

### NATS Tuning

```conf
# nats-server.conf

# Connection limits
max_connections = 10000
max_subscriptions = 10000

# Message size
max_payload = 8MB

# Performance
write_deadline = "10s"
max_pending_size = 512MB
```

### Control Plane Tuning

```yaml
# server.yaml

# API server
api:
  workers: 16  # Number of CPU cores
  read_timeout: "30s"
  write_timeout: "30s"

# Connection pools
storage:
  postgresql:
    pool:
      max_connections: 100
      max_idle: 20
      max_lifetime: "1h"

# State execution
state:
  execution:
    max_concurrent: 20
    timeout: "10m"

# Event processing
events:
  processing:
    worker_count: 8
    batch_size: 100
```

## Diagnostic Commands

### Quick Health Check

```bash
#!/bin/bash
# health-check.sh - Quick system health check

echo "=== Keystone Core Health Check ==="

# Control plane
echo -n "Control Plane: "
curl -s http://localhost:8080/health/ready && echo "OK" || echo "FAIL"

# Database
echo -n "Database: "
psql -U kscore -c "SELECT 1;" > /dev/null 2>&1 && echo "OK" || echo "FAIL"

# NATS
echo -n "NATS: "
nats server check connection > /dev/null 2>&1 && echo "OK" || echo "FAIL"

# Agents
TOTAL=$(kscorectl agent list | wc -l)
CONNECTED=$(kscorectl agent list --filter "status:healthy" | wc -l)
echo "Agents: $CONNECTED/$TOTAL connected"

# Disk space
echo "Disk Space:"
df -h / /var/lib/kscore /var/lib/postgresql | grep -v Filesystem
```

### Collect Diagnostic Bundle

```bash
#!/bin/bash
# collect-diagnostics.sh

BUNDLE_DIR="/tmp/kscore-diagnostics-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BUNDLE_DIR"

# System info
uname -a > "$BUNDLE_DIR/system-info.txt"
free -h >> "$BUNDLE_DIR/system-info.txt"
df -h >> "$BUNDLE_DIR/system-info.txt"

# Logs
journalctl -u kscore-server --since "1 hour ago" > "$BUNDLE_DIR/server-logs.txt"
journalctl -u kscore-agent --since "1 hour ago" > "$BUNDLE_DIR/agent-logs.txt"

# Configuration (redact sensitive data)
sed 's/password: .*/password: [REDACTED]/' /etc/kscore/server.yaml > "$BUNDLE_DIR/server-config.yaml"

# Status
kscorectl cluster status > "$BUNDLE_DIR/cluster-status.txt"
kscorectl agent list > "$BUNDLE_DIR/agents.txt"

# Database
psql -U kscore -c "\dt" > "$BUNDLE_DIR/db-tables.txt"
psql -U kscore -c "SELECT * FROM pg_stat_database;" > "$BUNDLE_DIR/db-stats.txt"

# Create tarball
tar -czf "${BUNDLE_DIR}.tar.gz" -C /tmp "$(basename $BUNDLE_DIR)"
echo "Diagnostic bundle: ${BUNDLE_DIR}.tar.gz"
```

## Getting Help

### Before Opening an Issue

1. Search existing issues: https://github.com/shawnbutts/keystone-core/issues
2. Check documentation: https://docs.kscore.io
3. Review logs with debug logging enabled
4. Collect diagnostic bundle
5. Try the solution in a test environment first

### Issue Template

```markdown
**Environment:**
- Keystone Core Version: v1.0.0
- OS: Ubuntu 22.04
- Deployment: Kubernetes / VM / Bare Metal
- Agent Count: 100

**Problem Description:**
[Clear description of the issue]

**Steps to Reproduce:**
1. Step one
2. Step two
3. Step three

**Expected Behavior:**
[What should happen]

**Actual Behavior:**
[What actually happens]

**Logs:**
```
[Relevant log excerpts with debug logging]
```

**Configuration:**
```yaml
[Relevant config (redact secrets)]
```
```

### Community Support

- **GitHub Issues**: Bug reports and feature requests
- **Discussions**: https://github.com/shawnbutts/keystone-core/discussions
- **Slack**: https://keystonecore.slack.com
- **Email**: support@keystonecore.io

### Commercial Support

Enterprise support contracts available:
- 24/7 support with SLA
- Dedicated Slack channel
- Regular architecture reviews
- Priority bug fixes

## See Also

- [Monitoring Guide](monitoring/) - Set up observability to catch issues early
- [Maintenance Guide](maintenance/) - Regular maintenance prevents issues
- [Security Guide](security/) - Secure your deployment
- [Deployment Guide](deployment/) - Proper deployment reduces issues
