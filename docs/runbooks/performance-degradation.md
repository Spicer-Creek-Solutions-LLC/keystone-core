# Runbook: Performance Degradation

## Overview

This runbook covers diagnosis and remediation of performance degradation in Keystone Core clusters.

## Prerequisites

- [ ] Access to control plane nodes (SSH)
- [ ] Access to monitoring dashboards (Grafana)
- [ ] Access to log aggregation (Loki/ELK)
- [ ] Understanding of baseline performance metrics
- [ ] Ability to make configuration changes

## Trigger Conditions

- API response times exceed SLA (P95 > 500ms)
- State application taking longer than expected
- Agent heartbeat latency increasing
- Command execution timeouts increasing
- User reports of slowness

## Severity Assessment

| Symptom | Severity | Response Time |
|---------|----------|---------------|
| P95 latency 2x baseline | Low | 4 hours |
| P95 latency 5x baseline | Medium | 1 hour |
| P95 latency 10x+ baseline | High | 15 minutes |
| Operations timing out | Critical | Immediate |

## Procedure

### Phase 1: Quick Assessment (5 minutes)

#### Step 1.1: Check System Health

```bash
# Overall health check
kscore-cluster status

# Check control plane resource usage
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "top -bn1 | head -5; free -h; df -h /"
done

# Check agent health summary
kscorectl agent list -o json | jq -r '.[].status' | sort | uniq -c
```

#### Step 1.2: Review Key Metrics

```bash
# Query Prometheus for key metrics
# API latency
curl -g 'http://prometheus:9090/api/v1/query?query=histogram_quantile(0.95,rate(kscore_api_request_duration_seconds_bucket[5m]))'

# Database query latency
curl -g 'http://prometheus:9090/api/v1/query?query=histogram_quantile(0.95,rate(kscore_db_query_duration_seconds_bucket[5m]))'

# NATS message latency
curl -g 'http://prometheus:9090/api/v1/query?query=histogram_quantile(0.95,rate(kscore_nats_message_latency_seconds_bucket[5m]))'
```

### Phase 2: Identify Bottleneck (15 minutes)

#### Step 2.1: Control Plane Analysis

**CPU Saturation**:

```bash
# Check CPU usage per component
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node CPU ==="
  ssh $node "pidstat -u 1 3 | grep -E 'kscore|etcd|nats'"
done

# Check for CPU throttling (if containerized)
kubectl top pods -n kscore
```

**Memory Pressure**:

```bash
# Check memory usage
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node Memory ==="
  ssh $node "ps aux --sort=-%mem | head -10"
  ssh $node "cat /proc/meminfo | grep -E 'MemAvailable|Buffers|Cached'"
done

# Check for OOM events
dmesg | grep -i "out of memory"
journalctl -k | grep -i oom
```

**Disk I/O**:

```bash
# Check disk I/O
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node Disk I/O ==="
  ssh $node "iostat -xz 1 3"
done

# Check disk latency
curl -g 'http://prometheus:9090/api/v1/query?query=rate(node_disk_io_time_seconds_total[5m])'
```

#### Step 2.2: Database Analysis

**PostgreSQL**:

```bash
# Check active connections
psql -h postgres -U kscore -c "SELECT count(*) FROM pg_stat_activity WHERE state = 'active';"

# Check slow queries
psql -h postgres -U kscore -c "
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE (now() - pg_stat_activity.query_start) > interval '5 seconds'
ORDER BY duration DESC;
"

# Check table bloat
psql -h postgres -U kscore -c "
SELECT schemaname, relname, n_dead_tup, n_live_tup,
       round(n_dead_tup::numeric / nullif(n_live_tup,0) * 100, 2) as dead_pct
FROM pg_stat_user_tables
WHERE n_dead_tup > 1000
ORDER BY n_dead_tup DESC
LIMIT 10;
"

# Check lock contention
psql -h postgres -U kscore -c "
SELECT blocked_locks.pid AS blocked_pid,
       blocked_activity.usename AS blocked_user,
       blocking_locks.pid AS blocking_pid,
       blocking_activity.usename AS blocking_user,
       blocked_activity.query AS blocked_statement
FROM pg_catalog.pg_locks blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks blocking_locks ON blocking_locks.locktype = blocked_locks.locktype
JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted;
"
```

**SQLite**:

```bash
# Check database size and fragmentation
sqlite3 /var/lib/kscore/keystone.db "
SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size();
SELECT freelist_count * page_size as fragmented FROM pragma_freelist_count(), pragma_page_size();
"

# Check for long-running queries
# (requires query logging enabled)
grep "duration" /var/log/kscore/query.log | awk '$NF > 1000' | tail -20
```

#### Step 2.3: NATS Analysis

```bash
# Check NATS server status
nats server report

# Check JetStream status
nats stream report
nats consumer report

# Check for slow consumers
nats server report consumers --sort pending

# Check connection count
curl http://nats:8222/varz | jq '.connections'

# Check for message backlog
nats stream info KSCORE_COMMANDS --json | jq '.state.messages'
```

#### Step 2.4: Network Analysis

```bash
# Check network latency between nodes
for src in ks-server-1 ks-server-2 ks-server-3; do
  for dst in ks-server-1 ks-server-2 ks-server-3; do
    [ "$src" != "$dst" ] && echo "$src -> $dst: $(ssh $src "ping -c 3 $dst | tail -1 | awk -F'/' '{print \$5}')"
  done
done

# Check for packet loss
netstat -s | grep -E "retransmit|timeout"

# Check connection states
ss -s
```

### Phase 3: Remediation

#### Remediation: High CPU

```bash
# If CPU bound on API processing, increase worker threads
# Edit /etc/kscore/server.yaml:
#   server:
#     workers: 16
systemctl restart kscore-server

# If CPU bound on state rendering, enable template caching
# Edit /etc/kscore/server.yaml:
#   state:
#     template_cache:
#       enabled: true
systemctl restart kscore-server

# If CPU bound on policy evaluation, add caching
# Edit /etc/kscore/server.yaml:
#   policy:
#     cache:
#       enabled: true
#       ttl: 1m
```

#### Remediation: Memory Pressure

```bash
# Increase memory limits (if containerized)
kubectl patch deployment kscore-server -n kscore -p '{"spec":{"template":{"spec":{"containers":[{"name":"kscore-server","resources":{"limits":{"memory":"4Gi"}}}]}}}}'

# Reduce in-memory caches
# Edit /etc/kscore/server.yaml:
#   cache:
#     max_size: 256MB
systemctl restart kscore-server

# Force garbage collection
curl -X POST http://localhost:8080/debug/gc

# If using embedded SQLite, reduce cache
# Edit /etc/kscore/server.yaml:
#   database:
#     sqlite:
#       cache_size: -32000
```

#### Remediation: Disk I/O

```bash
# Move to faster storage (if possible)
# Or optimize write patterns

# Enable write batching for database
# Edit /etc/kscore/server.yaml:
#   database:
#     write_batch_size: 100
#     sync_interval: 1s
systemctl restart kscore-server

# Move JetStream to faster storage
nats-server --signal reload
# Update jetstream.store_dir in config

# Compact SQLite database (if applicable)
sqlite3 /var/lib/kscore/keystone.db "VACUUM;"
```

#### Remediation: Database Slow Queries

```bash
# PostgreSQL: Run VACUUM ANALYZE
psql -h postgres -U kscore -c "VACUUM ANALYZE;"

# PostgreSQL: Reindex
psql -h postgres -U kscore -c "REINDEX DATABASE kscore;"

# PostgreSQL: Kill long-running queries
psql -h postgres -U kscore -c "
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE (now() - pg_stat_activity.query_start) > interval '10 minutes'
AND state = 'active';
"

# SQLite: Vacuum
sqlite3 /var/lib/kscore/keystone.db "VACUUM;"

# SQLite: Optimize
sqlite3 /var/lib/kscore/keystone.db "PRAGMA optimize;"
```

#### Remediation: NATS Backlog

```bash
# Increase consumer workers
# Edit /etc/kscore/server.yaml:
#   nats:
#     consumer_workers: 10
systemctl restart kscore-server

# If persistent backlog, consider purging old messages
nats stream purge KSCORE_EVENTS --keep 10000

# Scale out (add more servers)
# Update cluster configuration

# Temporary: Reduce message retention
nats stream edit KSCORE_EVENTS --max-age 1h
```

#### Remediation: Network Latency

```bash
# Check for MTU issues
ping -M do -s 1472 ks-server-2

# Increase TCP buffers
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216

# Enable TCP BBR congestion control
sysctl -w net.ipv4.tcp_congestion_control=bbr

# Check for DNS resolution delays
time nslookup ks-server-2
# If slow, add to /etc/hosts
```

### Phase 4: Verification (10 minutes)

```bash
# Verify latency improved
watch -n 5 'curl -s -w "Total: %{time_total}s\n" -o /dev/null http://localhost:8080/api/v1/agents'

# Verify through metrics
curl -g 'http://prometheus:9090/api/v1/query?query=histogram_quantile(0.95,rate(kscore_api_request_duration_seconds_bucket[5m]))'

# Run smoke tests as a basic validation
kscore-test smoke

# Verify no new errors
journalctl -u kscore-server --since "10 minutes ago" | grep -i error
```

## Verification Checklist

- [ ] P95 latency back within SLA
- [ ] No timeout errors in logs
- [ ] Database query times normal
- [ ] NATS message backlog cleared
- [ ] CPU/Memory/Disk utilization normal
- [ ] All agents reporting normally

## Rollback

If remediation causes issues:

```bash
# Restore previous configuration
cp /etc/kscore/server.yaml.bak /etc/kscore/server.yaml
systemctl restart kscore-server

# If database changes caused issues
# Restore from backup (see backup-restore.md)
```

## Post-Procedure

1. [ ] Document root cause
2. [ ] Update monitoring thresholds if needed
3. [ ] Schedule capacity review if resources constrained
4. [ ] Update runbook with lessons learned
5. [ ] Create ticket for permanent fix if workaround applied
6. [ ] Notify stakeholders of resolution

## Appendix: Performance Baselines

| Metric | Good | Warning | Critical |
|--------|------|---------|----------|
| API P95 latency | < 100ms | 100-500ms | > 500ms |
| DB query P95 | < 50ms | 50-200ms | > 200ms |
| NATS message latency | < 10ms | 10-100ms | > 100ms |
| State apply (10 resources) | < 5s | 5-15s | > 15s |
| Agent heartbeat | < 5s | 5-30s | > 30s |

## Appendix: Quick Commands

```bash
# One-liner health check
kscore-cluster status && echo "Healthy" || echo "UNHEALTHY"

# Quick latency test
time kscorectl agent list --limit 1 > /dev/null

# Check recent errors
journalctl -u kscore-server --since "1 hour ago" -p err

# Resource snapshot
echo "CPU: $(top -bn1 | grep 'Cpu(s)' | awk '{print $2}')% | Mem: $(free | grep Mem | awk '{print int($3/$2*100)}')% | Disk: $(df / | tail -1 | awk '{print $5}')"
```
