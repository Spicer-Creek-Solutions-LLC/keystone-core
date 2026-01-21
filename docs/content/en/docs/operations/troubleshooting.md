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

**HA Cluster Recovery (via CoordinationService):**

When NATS issues occur in an HA cluster, use the server-to-server coordination channel:

```bash
# Check NATS status on a server
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "check-1", "requester_id": "admin"}' \
  server1:9443 keystone.core.v1.CoordinationService/NATSStatus

# Force reconnection
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-1", "initiator_id": "admin", "action": "RECOVERY_ACTION_RECONNECT"}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate

# Failover to backup NATS servers
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-2", "initiator_id": "admin", "action": "RECOVERY_ACTION_FAILOVER", "parameters": {"target_urls": "nats://backup1:4222"}}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate
```

See [Maintenance - NATS Recovery](/docs/operations/maintenance/#nats-recovery-ha-only) for detailed procedures.

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
kscorectl state check web-server.yaml

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
kscorectl state apply nginx-package.yaml
```

### Common Causes and Solutions

**Invalid YAML Syntax:**
```text
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
psql -U kscore -h localhost -d keystonecore -c "SELECT 1;"
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
kscorectl state drift web-server.yaml
```

**Fix:**
```bash
# Reapply state to fix drift
kscorectl state apply web-server.yaml
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
KSCORE_LOG_LEVEL=debug kscorectl state apply web-server.yaml
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
2. Check documentation: https://docs.keystonecore.io
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

---

## Epic-Specific Troubleshooting

The following sections provide troubleshooting guidance for specific feature areas (epics) of Keystone Core.

### Remote Execution Issues (Epic 2)

**Symptoms:**
- Commands not executing on agents
- Command timeout errors
- "execution denied" errors
- Output truncation

**Diagnostic Steps:**

```bash
# Check command history
kscorectl exec history --limit 10

# View specific job details
kscorectl exec show JOB_ID

# Check agent execution capability
kscorectl agent show AGENT_ID | grep -A5 capabilities
```

**Common Issues:**

**Command Blocked by Policy:**
```bash
# Check policy evaluation
kscorectl policy evaluate --action exec.run --user $USER --target $AGENT

# View policy audit
kscorectl policy audit --action exec.run --since 1h
```

**Agent Execution Mode Restrictions:**
```yaml
# agent.yaml - Check execution policy
execution:
  policy:
    mode: normal  # 'strict' blocks most commands
    allowed_commands:  # Whitelist in strict mode
      - ls
      - cat
      - systemctl
```

**Output Buffer Overflow:**
```yaml
# Increase output buffer for large outputs
execution:
  max_output_size: "10MB"  # Default 1MB
```

**Shell Environment Issues:**
```bash
# Commands run in clean environment
# Set environment explicitly
kscorectl exec run "export PATH=/usr/local/bin:\$PATH && my-command" --target $AGENT

# Or use env parameter
kscorectl exec run "my-command" --env "PATH=/usr/local/bin:/usr/bin" --target $AGENT
```

---

### Event System Issues (Epic 4)

**Symptoms:**
- Events not being processed
- Reactor not triggering
- Event backlog growing
- Duplicate event processing

**Diagnostic Steps:**

```bash
# Check event stream health
nats stream info kscore-events

# View consumer lag
nats consumer info kscore-events kscore-reactor

# Check reactor status
kscorectl reactor status

# View recent events
kscorectl event list --since 1h
```

**Common Issues:**

**Event Consumer Lag:**
```bash
# Check consumer pending count
nats consumer info kscore-events kscore-reactor | grep "Pending Messages"

# If high, increase reactor workers
# server.yaml
events:
  reactor:
    worker_count: 16  # Default 4
```

**Event Schema Validation Failures:**
```bash
# Check for rejected events
kscorectl event list --status rejected --since 24h

# View event details
kscorectl event show EVENT_ID
```

**Reactor Execution Failures:**
```bash
# Check reactor error logs
kscorectl reactor errors --since 1h

# View specific reactor execution
kscorectl reactor history REACTOR_NAME --limit 10
```

**Event Ordering Issues:**
```yaml
# Ensure event ordering for dependent events
events:
  ordering:
    enabled: true
    key: "$.agent_id"  # Order by agent
```

**Dead Letter Queue Processing:**
```bash
# View dead letter events
nats stream info kscore-events-dlq

# Replay dead letter events
kscorectl event replay --stream kscore-events-dlq --limit 100
```

---

### GitOps Integration Issues (Epic 5)

**Symptoms:**
- Webhooks not being received
- Sync failures
- Drift detection not working
- Approval workflows stuck

**Diagnostic Steps:**

```bash
# Check webhook status
kscorectl webhook status

# View recent webhook events
kscorectl webhook events --since 1h

# Check sync status
kscorectl gitops sync status
```

**Common Issues:**

**Webhook Signature Verification Failure:**
```bash
# Check webhook logs
kscorectl webhook logs --filter "verification failed"

# Verify HMAC secret matches source
# GitHub: Settings > Webhooks > Secret
# GitLab: Settings > Webhooks > Secret Token
```

**Webhook Not Reachable:**
```bash
# Test from external source
curl -v https://kscore.example.com/webhooks/github

# Check firewall allows webhook source IPs
# GitHub webhook IPs
curl -s https://api.github.com/meta | jq '.hooks[]'
```

**Git Authentication Failures:**
```bash
# Test git credentials
git ls-remote https://github.com/org/repo.git

# Check credential configuration
kscorectl gitops config show | grep -A5 git
```

**Sync Conflicts:**
```bash
# View sync conflict details
kscorectl gitops sync conflicts

# Force resync from git (use carefully)
kscorectl gitops sync --force
```

**Approval Workflow Issues:**
```bash
# Check pending approvals
kscorectl approval list --status pending

# View approval history
kscorectl approval history DEPLOYMENT_ID
```

---

### Policy Enforcement Issues (Epic 6)

**Symptoms:**
- Unexpected policy denials
- Policy evaluation errors
- Compliance report failures
- Policy conflicts

**Diagnostic Steps:**

```bash
# Test policy evaluation
kscorectl policy evaluate --input '{"action":"exec.run","target":"agent-1"}'

# View policy decisions
kscorectl policy audit --since 1h

# List active policies
kscorectl policy list --status active
```

**Common Issues:**

**Policy Syntax Errors (OPA):**
```bash
# Validate policy syntax
opa check my-policy.rego

# Test policy locally
opa eval -i input.json -d my-policy.rego "data.kscore.allow"
```

**Policy Conflict Resolution:**
```bash
# Check policy precedence
kscorectl policy precedence --action state.apply

# View conflicting policies
kscorectl policy conflicts
```

**Policy Evaluation Performance:**
```bash
# Profile policy evaluation
kscorectl policy profile --action state.apply --iterations 100

# Check for expensive rules
# Avoid unbounded iterations in policies
```

**CEL Policy Issues:**
```bash
# Test CEL expression
kscorectl policy eval-cel 'request.user.role == "admin"' --input '{"user":{"role":"admin"}}'
```

**Policy Not Applied:**
```yaml
# Check policy is enabled and has correct targets
# policy.yaml
metadata:
  name: require-labels
  enabled: true  # Must be true
spec:
  targets:
    - resource: deployment/*  # Check target pattern
```

---

### Observability Issues (Epic 7)

**Symptoms:**
- Missing metrics
- Traces not appearing
- Log aggregation failures
- Dashboard errors

**Diagnostic Steps:**

```bash
# Check metrics endpoint
curl http://localhost:8080/metrics | head -50

# Verify Prometheus scraping
curl http://prometheus:9090/api/v1/targets | jq '.data.activeTargets'

# Check trace export
curl http://localhost:8080/debug/tracez
```

**Common Issues:**

**Metrics Not Scraped:**
```yaml
# Check Prometheus scrape config
# prometheus.yml
scrape_configs:
  - job_name: 'kscore'
    static_configs:
      - targets: ['control-plane:8080']
    scrape_interval: 15s
    metrics_path: /metrics
```

**High Cardinality Metrics:**
```bash
# Check metric cardinality
curl -s http://localhost:8080/metrics | grep -c "kscore_"

# If too high, check for unbounded labels
# Avoid dynamic labels like agent_id on high-frequency metrics
```

**Traces Not Appearing:**
```yaml
# Check OTLP exporter configuration
# server.yaml
observability:
  tracing:
    enabled: true
    exporter: otlp
    endpoint: "otel-collector:4317"
    sampling_rate: 0.1  # 10% sampling
```

**Log Forwarding Issues:**
```yaml
# Check log output configuration
# server.yaml
logging:
  format: json  # Required for log aggregation
  output:
    - type: file
      path: /var/log/kscore/server.log
    - type: syslog
      address: "udp://logserver:514"
```

**Dashboard Query Errors:**
```bash
# Test PromQL query
curl "http://prometheus:9090/api/v1/query?query=kscore_api_requests_total"

# Check for missing metrics
curl http://prometheus:9090/api/v1/label/__name__/values | jq '.data | map(select(startswith("kscore")))'
```

---

### Multi-Environment Issues (Epic 8)

**Symptoms:**
- Cloud discovery not working
- Kubernetes integration failures
- Environment promotion stuck
- Cross-environment communication issues

**Diagnostic Steps:**

```bash
# Check cloud provider integration
kscorectl cloud status

# Verify K8s connection
kscorectl k8s status

# View environment configuration
kscorectl env list
```

**Common Issues:**

**Cloud Metadata Discovery Failures:**
```bash
# AWS
curl http://169.254.169.254/latest/meta-data/instance-id

# GCP
curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/id

# Azure
curl -H "Metadata: true" "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
```

**IAM Permission Issues:**
```bash
# AWS - Check IAM role
aws sts get-caller-identity

# Required permissions:
# - ec2:DescribeInstances
# - ec2:DescribeTags
```

**Kubernetes Service Account Issues:**
```bash
# Check service account token
kubectl get serviceaccount kscore-agent -o yaml

# Verify RBAC permissions
kubectl auth can-i list pods --as=system:serviceaccount:kscore:kscore-agent
```

**Cross-Environment NATS Gateway Issues:**
```bash
# Check gateway status
nats server check gateway

# Verify gateway configuration
nats server info --gateway
```

---

### Module System Issues (Epic 9)

**Symptoms:**
- Module loading failures
- Sandbox escape errors
- Resource limit exceeded
- Module signature verification failed

**Diagnostic Steps:**

```bash
# List loaded modules
kscorectl module list

# Check module status
kscorectl module status MODULE_NAME

# View module capabilities
kscorectl module capabilities MODULE_NAME
```

**Common Issues:**

**Module Signature Verification Failed:**
```bash
# Check module signature
kscorectl module verify MODULE_NAME

# View signing key
kscorectl module signing-key show

# If signature invalid, module may be tampered
```

**Module Resource Limits Exceeded:**
```yaml
# Increase module resource limits
# server.yaml
modules:
  resources:
    max_memory: "256MB"  # Default 128MB
    max_cpu_time: "30s"  # Default 10s
    max_network_calls: 100
```

**Module Capability Denied:**
```bash
# Check capability policy
kscorectl module capabilities show MODULE_NAME

# Grant required capability
kscorectl module capabilities grant MODULE_NAME fs.read
```

**Starlark Execution Errors:**
```bash
# Debug Starlark module
kscorectl module debug MODULE_NAME --input '{"key":"value"}'

# Common issues:
# - Undefined variables
# - Type mismatches
# - Infinite loops (hit CPU limit)
```

**WASM Module Issues:**
```bash
# Check WASM runtime status
kscorectl module wasm status

# Verify WASM file is valid
wasm-validate module.wasm
```

---

### HA Clustering Issues (Epic 11)

**Symptoms:**
- Leader election failures
- Split-brain scenarios
- Etcd cluster unhealthy
- Agent rebalancing issues

**Diagnostic Steps:**

```bash
# Check cluster status
kscorectl cluster status

# View etcd cluster health
etcdctl endpoint health --cluster

# Check leader
kscorectl cluster leader
```

**Common Issues:**

**Etcd Cluster Unhealthy:**
```bash
# Check etcd member status
etcdctl member list

# View etcd logs
journalctl -u etcd -f

# If member is unhealthy, remove and re-add
etcdctl member remove MEMBER_ID
etcdctl member add node3 --peer-urls=https://node3:2380
```

**Split-Brain Recovery:**
```bash
# Identify which partition has majority
etcdctl endpoint status --cluster

# Restart minority partition nodes
# They will rejoin automatically

# If majority lost, manual recovery required
# See: docs/runbooks/split-brain-recovery.md
```

**Leader Election Issues:**
```bash
# Force leader election
# WARNING: Use only if cluster is stuck
kscorectl cluster elect --force

# Check leadership history
kscorectl cluster leadership-history
```

**Agent Rebalancing Slow:**
```yaml
# Tune rebalancing parameters
# server.yaml
cluster:
  rebalancing:
    interval: "30s"  # How often to check balance
    max_concurrent: 10  # Agents to move at once
    threshold: 0.2  # Imbalance threshold (20%)
```

---

### NATS Mesh Issues (Epic 14)

**Symptoms:**
- Supercluster connectivity issues
- Gateway authentication failures
- Message routing problems
- Leafnode connection failures

**Diagnostic Steps:**

```bash
# Check cluster status
nats server check cluster

# View gateway status
nats server check gateway

# Check leafnode connections
nats server info --leafnodes
```

**Common Issues:**

**Gateway Connection Failures:**
```bash
# Test gateway connectivity
nc -zv gateway-remote 7222

# Check gateway credentials
nats server info --gateway | grep -A5 "Gateways"
```

**Leafnode Authentication:**
```conf
# nats-server.conf
leafnodes {
  remotes [
    {
      url: "nats://hub:7422"
      credentials: "/etc/nats/leaf.creds"  # Check file exists
    }
  ]
}
```

**Message Not Routing:**
```bash
# Trace message routing
nats sub ">" --trace

# Check subject mappings
nats server info --subjects
```

**Supercluster Recovery:**
```bash
# Restart gateways in order
# 1. Stop all gateways
# 2. Start primary gateway
# 3. Start secondary gateways one by one
```

---

### SPIFFE Identity Issues (Epic 17)

**Symptoms:**
- SVID issuance failures
- Trust bundle errors
- Attestation failures
- Certificate rotation problems

**Diagnostic Steps:**

```bash
# Check SPIRE agent status
spire-agent healthcheck

# View current SVID
spire-agent api fetch x509 -write /tmp/

# Check trust bundle
spire-agent api fetch bundle
```

**Common Issues:**

**Attestation Failure:**
```bash
# Check attestation method
# Agent logs show attestation type
journalctl -u spire-agent | grep attestation

# Common issues:
# - K8s: Service account token expired
# - AWS: Instance profile not attached
# - Unix: GID/UID mismatch
```

**SVID Not Rotating:**
```bash
# Check SVID expiry
openssl x509 -in /tmp/svid.pem -noout -dates

# Force rotation
spire-agent api fetch x509 -write /tmp/ -force
```

**Trust Bundle Sync Issues:**
```bash
# Check federation status
spire-server federation list

# Refresh trust bundle
spire-server bundle show -format spiffe > trust-bundle.json
```

**Workload Registration Missing:**
```bash
# List registrations
spire-server entry show

# Create missing registration
spire-server entry create \
  -spiffeID spiffe://cluster.local/ns/kscore/sa/kscore-agent \
  -parentID spiffe://cluster.local/spire/agent/k8s_psat/... \
  -selector k8s:ns:kscore \
  -selector k8s:sa:kscore-agent
```

---

### IPv6 Issues (Epic 18)

**Symptoms:**
- IPv6 connectivity failures
- Dual-stack issues
- IPv6 address detection problems
- Firewall blocking IPv6

**Diagnostic Steps:**

```bash
# Check IPv6 connectivity
ping6 -c 3 ipv6.google.com

# View IPv6 addresses
ip -6 addr show

# Test IPv6 to control plane
nc -zv -6 control-plane 8443
```

**Common Issues:**

**IPv6 Disabled on System:**
```bash
# Check if IPv6 is enabled
sysctl net.ipv6.conf.all.disable_ipv6

# Enable IPv6
sudo sysctl -w net.ipv6.conf.all.disable_ipv6=0
```

**Firewall Blocking IPv6:**
```bash
# Check ip6tables rules
sudo ip6tables -L -n

# Allow Keystone Core ports
sudo ip6tables -A INPUT -p tcp --dport 8443 -j ACCEPT
sudo ip6tables -A INPUT -p tcp --dport 4222 -j ACCEPT
```

**Dual-Stack Address Selection:**
```yaml
# Prefer IPv4 or IPv6
# server.yaml
network:
  prefer_ipv6: true  # Set based on your network
  dual_stack: true
```

**IPv6 Address Detection:**
```bash
# Agent may pick wrong IPv6 address
# Configure explicitly
# agent.yaml
network:
  advertise_address: "2001:db8::1"  # Set correct IPv6
```

---

### Windows Agent Issues (Epic 20)

**Symptoms:**
- Windows agent not starting
- PowerShell execution failures
- Service management issues
- Path encoding problems

**Diagnostic Steps:**

```powershell
# Check service status
Get-Service kscore-agent

# View event logs
Get-EventLog -LogName Application -Source kscore-agent -Newest 20

# Check agent configuration
Get-Content C:\ProgramData\keystone\agent.yaml
```

**Common Issues:**

**Service Not Starting:**
```powershell
# Check service logs
Get-WinEvent -FilterHashtable @{LogName='Application';ProviderName='kscore-agent'} -MaxEvents 10

# Verify service account permissions
# Should be LocalSystem or dedicated service account with proper permissions
```

**PowerShell Execution Policy:**
```powershell
# Check current policy
Get-ExecutionPolicy

# Set for scripts to run
Set-ExecutionPolicy RemoteSigned -Scope LocalMachine
```

**Path Encoding Issues:**
```yaml
# Use forward slashes or escape backslashes
# Good
path: "C:/ProgramData/keystone/config.yaml"
# Good
path: "C:\\ProgramData\\keystone\\config.yaml"
# Bad (unescaped)
path: "C:\ProgramData\keystone\config.yaml"
```

**UAC Elevation Required:**
```yaml
# State requiring elevation
install_software:
  module: windows.package
  state: present
  name: "Visual Studio Code"
  elevated: true  # Run with elevation
```

**Antivirus Blocking Agent:**
```powershell
# Add exclusions
Add-MpPreference -ExclusionPath "C:\Program Files\keystone"
Add-MpPreference -ExclusionProcess "kscore-agent.exe"
```

---

### Proxy Agent Issues (Epic 21)

**Symptoms:**
- Cannot connect to network devices
- SSH/SNMP authentication failures
- Command execution timeout
- Credential issues

**Diagnostic Steps:**

```bash
# Check proxy agent status
kscorectl proxy status

# Test device connectivity
kscorectl proxy test DEVICE_ID

# View proxy agent logs
journalctl -u kscore-proxy-agent -f
```

**Common Issues:**

**SSH Connection Failures:**
```bash
# Test SSH manually
ssh -v admin@device-ip

# Common issues:
# - Key format (convert if needed)
ssh-keygen -p -m PEM -f ~/.ssh/id_rsa
# - Host key verification
# Add to known_hosts or disable strict checking
```

**SNMP Authentication Failures:**
```bash
# Test SNMPv3 manually
snmpwalk -v3 -u admin -l authPriv -a SHA -A authpass -x AES -X privpass device-ip sysDescr

# Check credentials in proxy config
kscorectl proxy credentials show DEVICE_ID
```

**REST API Issues:**
```bash
# Test REST endpoint
curl -v -u admin:password https://device-ip/restconf/data

# Check TLS certificate
openssl s_client -connect device-ip:443 -showcerts
```

**Credential Rotation:**
```bash
# Update device credentials
kscorectl proxy credentials update DEVICE_ID \
  --username admin \
  --password new-password

# Verify connection
kscorectl proxy test DEVICE_ID
```

---

### File Distribution Issues (Epic 22)

**Symptoms:**
- File upload failures
- Download timeouts
- Checksum mismatches
- Storage backend errors

**Diagnostic Steps:**

```bash
# Check file server status
kscorectl file status

# View recent transfers
kscorectl file transfers --since 1h

# Check storage backend
kscorectl file storage status
```

**Common Issues:**

**Upload Failures:**
```bash
# Check file size limits
# server.yaml
file_distribution:
  max_file_size: "1GB"  # Increase if needed

# Check storage quota
kscorectl file storage quota
```

**Download Timeouts:**
```yaml
# Increase timeout for large files
# agent.yaml
file_distribution:
  download_timeout: "30m"  # Default 5m
  retry_count: 3
```

**Checksum Mismatch:**
```bash
# Verify file checksum
kscorectl file verify FILE_ID

# Re-upload if corrupted
kscorectl file upload --replace /path/to/file
```

**S3 Backend Issues:**
```bash
# Test S3 access
aws s3 ls s3://kscore-files/

# Check IAM permissions:
# - s3:GetObject
# - s3:PutObject
# - s3:DeleteObject
# - s3:ListBucket
```

---

### Blueprint Issues (Epic 25)

**Symptoms:**
- Blueprint deployment failures
- Parameter validation errors
- Resource creation stuck
- Rollback not working

**Diagnostic Steps:**

```bash
# Check blueprint status
kscorectl blueprint status DEPLOYMENT_ID

# View deployment logs
kscorectl blueprint logs DEPLOYMENT_ID

# List blueprint resources
kscorectl blueprint resources DEPLOYMENT_ID
```

**Common Issues:**

**Parameter Validation Errors:**
```bash
# Validate blueprint parameters
kscorectl blueprint validate my-blueprint \
  --param cluster_size=3 \
  --param region=us-west-2

# Check parameter schema
kscorectl blueprint show my-blueprint --show-params
```

**Resource Creation Failures:**
```bash
# Check individual resource status
kscorectl blueprint resource status DEPLOYMENT_ID RESOURCE_NAME

# View resource error
kscorectl blueprint resource logs DEPLOYMENT_ID RESOURCE_NAME
```

**Rollback Issues:**
```bash
# Check rollback capability
kscorectl blueprint can-rollback DEPLOYMENT_ID

# View rollback plan
kscorectl blueprint rollback --dry-run DEPLOYMENT_ID

# Execute rollback
kscorectl blueprint rollback DEPLOYMENT_ID
```

**Dependency Resolution:**
```bash
# View dependency graph
kscorectl blueprint deps DEPLOYMENT_ID

# Check for circular dependencies
kscorectl blueprint validate my-blueprint --check-cycles
```

---

### Bootstrap Issues (Epic 27)

**Symptoms:**
- Bootstrap process fails
- Agent enrollment stuck
- Control plane not initializing
- Certificate generation errors

**Diagnostic Steps:**

```bash
# Check bootstrap status
kscorectl bootstrap status

# View bootstrap logs
journalctl -u kscore-bootstrap -f

# Verify seed configuration
kscorectl bootstrap validate seed.yaml
```

**Common Issues:**

**Seed Configuration Errors:**
```bash
# Validate seed file
kscorectl bootstrap validate seed.yaml

# Check required fields
# - control_plane.address
# - identity.mode
# - storage configuration
```

**Certificate Generation Failures:**
```bash
# Check CA availability
openssl verify -CAfile /etc/kscore/certs/ca.crt /etc/kscore/certs/server.crt

# Regenerate certificates
kscorectl bootstrap regenerate-certs
```

**Database Initialization Errors:**
```bash
# Check database connectivity
psql -U kscore -h localhost -d keystonecore -c "SELECT 1;"

# Reinitialize database (WARNING: destructive)
kscorectl bootstrap init-db --force
```

**Agent Enrollment Stuck:**
```bash
# Check join token validity
kscorectl agent token validate TOKEN

# Generate new token
kscorectl agent token create --ttl 1h
```

---

## Emergency Procedures

### Complete System Recovery

When the entire Keystone Core deployment is down:

```bash
#!/bin/bash
# emergency-recovery.sh

echo "=== Keystone Core Emergency Recovery ==="

# 1. Start infrastructure
echo "Starting PostgreSQL..."
sudo systemctl start postgresql
sleep 5

echo "Starting NATS..."
sudo systemctl start nats-server
sleep 5

echo "Starting etcd..."
sudo systemctl start etcd
sleep 5

# 2. Verify infrastructure
echo "Checking infrastructure..."
psql -U kscore -c "SELECT 1;" || exit 1
nats server check connection || exit 1
etcdctl endpoint health || exit 1

# 3. Start control plane
echo "Starting control plane..."
sudo systemctl start kscore-server
sleep 10

# 4. Verify control plane
echo "Checking control plane..."
curl -f http://localhost:8080/health/ready || exit 1

# 5. Restart agents
echo "Control plane ready. Agents will reconnect automatically."
echo "Monitor with: kscorectl agent list --watch"
```

### Database Recovery from Backup

```bash
# Stop control plane
sudo systemctl stop kscore-server

# Restore database
pg_restore -U kscore -d keystonecore -c /backups/kscore-cluster-backup.dump

# Verify restore
psql -U kscore -d keystonecore -c "SELECT COUNT(*) FROM agents;"

# Restart control plane
sudo systemctl start kscore-server
```

### Force Agent Reconnection

```bash
# Restart all agents (use with caution)
kscorectl exec run "systemctl restart kscore-agent" --target "all"

# Or target specific agents
kscorectl exec run "systemctl restart kscore-agent" --target "role:web"
```

---

## See Also

- [Monitoring Guide](monitoring/) - Set up observability to catch issues early
- [Maintenance Guide](maintenance/) - Regular maintenance prevents issues
- [Security Guide](security/) - Secure your deployment
- [Deployment Guide](deployment/) - Proper deployment reduces issues
- [Runbooks](https://github.com/keystone-core/keystone-core/tree/main/docs/runbooks) - Step-by-step operational procedures
