---
title: "Self-Management Operations"
linkTitle: "Self-Management"
weight: 25
description: >
  Operating Keystone Core's self-management capabilities including bootstrap, backup/restore, upgrades, and disaster recovery.
---

## Overview

Keystone Core can manage its own infrastructure, enabling true "infrastructure as code" for the control plane itself. This guide covers operational procedures for:

- **Bootstrap**: Initial cluster deployment from seed configuration
- **Backup & Restore**: Data protection and recovery
- **Upgrades**: Rolling upgrades with automatic rollback
- **State Management**: Self-management state modules
- **Disaster Recovery**: Recovery procedures and runbooks

## Bootstrap Operations

### Initial Cluster Bootstrap

Bootstrap a new Keystone Core cluster from a seed configuration:

```bash
# Validate seed configuration first
kscore-bootstrap validate --config seed.yaml

# Bootstrap the cluster
kscore-bootstrap seed --config seed.yaml

# Check bootstrap status
kscore-bootstrap status
```

#### Seed Configuration Example

```yaml
# seed.yaml - Minimal single-node deployment
cluster:
  name: keystone-prod
  domain: keystone.example.com

nodes:
  - hostname: ks-server-1
    role: server
    ip: 10.0.1.10

nats:
  mode: embedded
  jetstream:
    enabled: true
    store_dir: /var/lib/kscore/jetstream

database:
  type: sqlite
  path: /var/lib/kscore/state.db

certificates:
  generate: true
  ca:
    validity_days: 3650
  server:
    validity_days: 365
```

#### Multi-Node HA Bootstrap

```yaml
# seed-ha.yaml - Three-node HA cluster
cluster:
  name: keystone-prod
  domain: keystone.example.com

nodes:
  - hostname: ks-server-1
    role: server
    ip: 10.0.1.10
  - hostname: ks-server-2
    role: server
    ip: 10.0.1.11
  - hostname: ks-server-3
    role: server
    ip: 10.0.1.12

nats:
  mode: cluster
  cluster:
    name: keystone-nats
    routes:
      - nats://10.0.1.10:6222
      - nats://10.0.1.11:6222
      - nats://10.0.1.12:6222
  jetstream:
    enabled: true
    store_dir: /var/lib/kscore/jetstream
    replicas: 3

database:
  type: postgresql
  host: postgres.example.com
  port: 5432
  name: keystone
  user: keystone
  password: ${POSTGRES_PASSWORD}

etcd:
  endpoints:
    - https://10.0.1.10:2379
    - https://10.0.1.11:2379
    - https://10.0.1.12:2379

certificates:
  generate: true
```

### Bootstrap Modes

| Mode | Command | Use Case |
|------|---------|----------|
| Seed | `kscore-bootstrap seed` | New cluster from scratch |
| Restore | `kscore-bootstrap restore` | Restore from backup |
| Import | `kscore-bootstrap import` | Import existing installation |

### Bootstrap Troubleshooting

```bash
# View bootstrap logs
journalctl -u kscore-server -f

# Clean up failed bootstrap
kscore-bootstrap cleanup --force

# Retry bootstrap with debug logging
kscore-bootstrap seed --config seed.yaml --debug
```

## Backup Operations

### Configuring Backups

Create a backup configuration in your state file:

```yaml
# /etc/kscore/states/backup.yaml
kscore_backup:
  daily_backup:
    state: configured
    schedule: "0 2 * * *"  # 2 AM daily
    destination:
      type: s3
      bucket: keystone-backups
      prefix: daily/
      region: us-west-2
    retention:
      max_backups: 30
      max_age: 30d
    encryption:
      enabled: true
      provider: age
      recipient: age1...
    components:
      - database
      - config
      - certificates
      - jetstream
```

### Manual Backup

```bash
# Full backup to local directory
kscore-cluster-backup create --dest /backup/keystone

# Backup to S3
kscore-cluster-backup create \
  --dest s3://keystone-backups/manual/ \
  --encrypt --encrypt-recipient age1...

# Database-only backup
kscore-cluster-backup create \
  --components database \
  --dest /backup/db-only

# List available backups
kscore-cluster-backup list --dest s3://keystone-backups/

# Verify backup integrity
kscore-cluster-backup verify /backup/keystone/backup-2024-01-15.tar.gz
```

### Backup Components

| Component | Contents | Typical Size |
|-----------|----------|--------------|
| database | SQLite/PostgreSQL state | 10MB - 10GB |
| config | Server/agent YAML configs | < 1MB |
| certificates | CA and server certs | < 1MB |
| jetstream | NATS JetStream data | 1GB - 100GB |
| etcd | etcd snapshot | 100MB - 10GB |
| secrets | Encrypted secrets | < 10MB |

### Backup Destinations

```yaml
# Local filesystem
destination:
  type: local
  path: /backup/keystone

# AWS S3
destination:
  type: s3
  bucket: keystone-backups
  prefix: prod/
  region: us-west-2
  # Uses AWS credentials from environment or instance role

# Google Cloud Storage
destination:
  type: gcs
  bucket: keystone-backups
  prefix: prod/
  # Uses GOOGLE_APPLICATION_CREDENTIALS

# Azure Blob Storage
destination:
  type: azure
  container: keystone-backups
  prefix: prod/
  account: mystorageaccount
  # Uses AZURE_STORAGE_KEY or managed identity

# SFTP
destination:
  type: sftp
  host: backup.example.com
  port: 22
  user: backup
  key_file: /etc/kscore/backup-key
  path: /backups/keystone

# Multiple destinations (replication)
destination:
  type: multi
  destinations:
    - type: local
      path: /backup/keystone
    - type: s3
      bucket: keystone-backups-primary
    - type: s3
      bucket: keystone-backups-dr
      region: eu-west-1
```

### Retention Policies

```yaml
retention:
  max_backups: 30        # Keep at most 30 backups
  max_age: 90d           # Delete backups older than 90 days
  keep_daily: 7          # Keep 7 daily backups
  keep_weekly: 4         # Keep 4 weekly backups
  keep_monthly: 12       # Keep 12 monthly backups
  keep_yearly: 3         # Keep 3 yearly backups
```

## Restore Operations

### Restore from Backup

```bash
# List available backups
kscore-cluster-backup list --dest s3://keystone-backups/

# Restore full backup
kscore-bootstrap restore \
  --backup s3://keystone-backups/backup-2024-01-15.tar.gz \
  --decrypt-identity /etc/kscore/backup-key.txt

# Restore specific components only
kscore-bootstrap restore \
  --backup /backup/backup-2024-01-15.tar.gz \
  --components database,config

# Dry-run restore (validate only)
kscore-bootstrap restore \
  --backup /backup/backup-2024-01-15.tar.gz \
  --dry-run
```

### Point-in-Time Recovery (PostgreSQL)

```bash
# PostgreSQL PITR requires WAL archiving configured
# Restore to specific timestamp
kscore-bootstrap restore \
  --backup /backup/base-backup.tar.gz \
  --target-time "2024-01-15 14:30:00 UTC" \
  --wal-archive s3://keystone-wal/
```

### Partial Restore

```bash
# Restore only configuration (keep existing data)
kscore-bootstrap restore \
  --backup /backup/backup.tar.gz \
  --components config \
  --no-restart

# Restore certificates only
kscore-bootstrap restore \
  --backup /backup/backup.tar.gz \
  --components certificates \
  --no-restart

# Apply restored config
systemctl restart kscore-server
```

## Upgrade Operations

### Checking for Upgrades

```bash
# Check if upgrade is available
kscorectl upgrade check

# Output:
# Current version: 1.5.0
# Latest version: 1.6.0
# Upgrade available: yes
# Compatible: yes
# Upgrade path: 1.5.0 -> 1.6.0

# Check upgrade compatibility
kscorectl upgrade check --target 2.0.0
# Warning: Direct upgrade from 1.5.0 to 2.0.0 not supported
# Required path: 1.5.0 -> 1.8.0 -> 2.0.0
```

### Planning an Upgrade

```bash
# Generate upgrade plan
kscorectl upgrade plan --target 1.6.0

# Output:
# Upgrade Plan: 1.5.0 -> 1.6.0
# Strategy: rolling
#
# Steps:
# 1. Validate prerequisites
# 2. Backup current state
# 3. Upgrade server nodes (rolling)
#    - ks-server-1: drain, upgrade, verify
#    - ks-server-2: drain, upgrade, verify
#    - ks-server-3: drain, upgrade, verify
# 4. Upgrade agent nodes (batched)
#    - Batch 1: 10 agents
#    - Batch 2: 10 agents
#    - ...
# 5. Verify cluster health
#
# Estimated duration: 45 minutes
# Rollback: automatic on failure
```

### Executing an Upgrade

```bash
# Execute upgrade with default strategy (rolling)
kscorectl upgrade execute --target 1.6.0

# Execute with canary strategy
kscorectl upgrade execute --target 1.6.0 --strategy canary

# Execute with custom parameters
kscorectl upgrade execute --target 1.6.0 \
  --strategy rolling \
  --max-unavailable 1 \
  --drain-timeout 5m \
  --health-check-interval 30s

# Monitor upgrade progress
kscorectl upgrade status

# Cancel in-progress upgrade
kscorectl upgrade cancel
```

### Upgrade Strategies

#### Rolling Upgrade

Zero-downtime upgrade that processes nodes one at a time:

```yaml
# Rolling upgrade configuration
upgrade:
  strategy: rolling
  rolling:
    max_unavailable: 1      # Max nodes down at once
    drain_timeout: 5m       # Time to wait for drain
    node_delay: 30s         # Delay between nodes
    order: leader_last      # Upgrade order
```

```bash
# Execute rolling upgrade
kscorectl upgrade execute --target 1.6.0 --strategy rolling
```

#### Canary Upgrade

Gradual rollout with automatic promotion or rollback:

```yaml
# Canary upgrade configuration
upgrade:
  strategy: canary
  canary:
    initial_percentage: 10  # Start with 10%
    increment: 20           # Increase by 20% each step
    interval: 10m           # Wait between steps
    success_threshold: 3    # Required successful checks
    metrics:
      - name: error_rate
        threshold: 0.01     # Max 1% error rate
      - name: latency_p99
        threshold: 500ms    # Max 500ms p99 latency
```

```bash
# Execute canary upgrade
kscorectl upgrade execute --target 1.6.0 --strategy canary

# Manually promote canary
kscorectl upgrade canary promote

# Manually rollback canary
kscorectl upgrade canary rollback
```

#### Blue-Green Upgrade

Deploy new version alongside old, then switch traffic:

```yaml
# Blue-green upgrade configuration
upgrade:
  strategy: blue_green
  blue_green:
    switch_timeout: 5m      # Time to switch traffic
    keep_old_version: true  # Keep old version for rollback
    traffic_switch: instant # instant or gradual
```

### Agent Upgrades

Agents are upgraded in batches after server upgrades complete:

```yaml
# Agent batch upgrade configuration
agent_upgrade:
  batch_size: 10           # Agents per batch
  batch_delay: 1m          # Delay between batches
  max_failures: 5          # Abort after N failures
  selectors:               # Target specific agents
    environment: production
  exclude_selectors:       # Exclude agents
    role: canary
  priority:                # Upgrade order
    - datacenter
    - role
```

```bash
# Upgrade agents only
kscorectl upgrade agents --target 1.6.0

# Upgrade agents matching selector
kscorectl upgrade agents --target 1.6.0 \
  --selector "environment=staging"

# Check agent version report
kscorectl upgrade agents --report
```

### Rollback Operations

```bash
# Automatic rollback on failure
# (enabled by default)

# Manual rollback
kscorectl upgrade rollback

# Rollback to specific version
kscorectl upgrade rollback --target 1.5.0

# Check rollback status
kscorectl upgrade rollback --status
```

### Upgrade Monitoring

```bash
# Watch upgrade progress
kscorectl upgrade status --watch

# View upgrade history
kscorectl upgrade history

# View upgrade logs
kscorectl upgrade logs --upgrade-id abc123
```

## Self-Management State Modules

Keystone Core provides state modules for managing its own components:

### Server Module

```yaml
# Manage Keystone Core server
kscore_server:
  production_server:
    state: running
    version: 1.6.0
    install_method: package  # package, binary, docker
    config:
      api:
        listen: 0.0.0.0:8080
        tls:
          enabled: true
          cert_file: /etc/kscore/server.crt
          key_file: /etc/kscore/server.key
      nats:
        url: nats://localhost:4222
      database:
        type: postgresql
        host: localhost
        name: keystone
```

### Agent Module

```yaml
# Manage Keystone Core agent
kscore_agent:
  local_agent:
    state: running
    version: 1.6.0
    install_method: package
    config:
      server_urls:
        - https://ks-server-1:8080
        - https://ks-server-2:8080
      tags:
        environment: production
        role: webserver
```

### NATS Module

```yaml
# Manage NATS configuration
kscore_nats:
  embedded_nats:
    state: running
    mode: embedded
    jetstream:
      enabled: true
      store_dir: /var/lib/kscore/jetstream
      max_memory: 1GB
      max_file: 10GB
    cluster:
      name: keystone-nats
      routes:
        - nats://10.0.1.10:6222
        - nats://10.0.1.11:6222
```

### Database Module

```yaml
# Manage database configuration
kscore_database:
  primary_db:
    state: configured
    type: postgresql
    host: localhost
    port: 5432
    name: keystone
    user: keystone
    password: ${POSTGRES_PASSWORD}
    ssl_mode: require
```

### Backup Module

```yaml
# Manage backup configuration
kscore_backup:
  scheduled_backup:
    state: configured
    schedule: "0 */6 * * *"  # Every 6 hours
    destination:
      type: s3
      bucket: keystone-backups
    retention:
      max_age: 7d
    encryption:
      enabled: true
      provider: age
```

## Health Monitoring

### Cluster Health Check

```bash
# Check overall cluster health
kscorectl cluster health

# Output:
# Cluster: keystone-prod
# Status: healthy
#
# Servers:
#   ks-server-1: healthy (leader)
#   ks-server-2: healthy
#   ks-server-3: healthy
#
# NATS:
#   Status: healthy
#   Streams: 5
#   Consumers: 12
#
# Database:
#   Status: healthy
#   Connections: 15/100
#
# Agents:
#   Total: 150
#   Healthy: 148
#   Degraded: 2
#   Offline: 0
```

### Component Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/health/live` | Liveness probe (process running) |
| `/health/ready` | Readiness probe (accepting traffic) |
| `/health/status` | Detailed component status |

```bash
# Check liveness
curl -s http://localhost:8080/health/live

# Check readiness
curl -s http://localhost:8080/health/ready

# Get detailed status
curl -s http://localhost:8080/health/status | jq
```

## Disaster Recovery

### DR Checklist

Before a disaster occurs:

- [ ] Regular backups configured and tested
- [ ] Backup encryption keys stored securely (offline)
- [ ] DR runbooks documented and practiced
- [ ] Cross-region backup replication enabled
- [ ] Recovery time objectives (RTO) defined
- [ ] Recovery point objectives (RPO) defined

### Recovery Scenarios

#### Single Node Failure

```bash
# 1. Remove failed node from cluster
kscorectl cluster remove ks-server-2

# 2. Provision replacement node

# 3. Join new node to cluster
kscore-bootstrap import \
  --join https://ks-server-1:8080 \
  --token $(kscorectl cluster token)
```

#### Complete Cluster Loss

```bash
# 1. Provision new infrastructure

# 2. Restore from latest backup
kscore-bootstrap restore \
  --backup s3://keystone-backups-dr/latest.tar.gz \
  --decrypt-identity /secure/backup-key.txt

# 3. Verify restoration
kscorectl cluster health

# 4. Re-register agents
# Agents will auto-reconnect if servers are accessible
# For network changes, update agent configs
```

#### Split-Brain Recovery

```bash
# 1. Identify the authoritative partition
# (usually the one with quorum)

# 2. Isolate the non-authoritative partition
# Stop servers in non-authoritative partition

# 3. Restore network connectivity

# 4. Re-join nodes from non-authoritative partition
kscore-bootstrap import \
  --join https://authoritative-leader:8080 \
  --force-rejoin
```

### Recovery Validation

```bash
# Verify cluster state after recovery
kscorectl cluster health --verbose

# Verify agent connectivity
kscorectl agent list --status

# Verify state integrity
kscorectl state check /etc/kscore/states/*.yaml

# Run integration tests
kscore-test integration --suite recovery
```

## Operational Runbooks

### Runbook: Scheduled Maintenance

```markdown
## Pre-Maintenance
1. Notify stakeholders
2. Create backup: `kscore-cluster-backup create --dest /backup/pre-maintenance`
3. Verify backup: `kscore-cluster-backup verify /backup/pre-maintenance/...`

## During Maintenance
4. Enable maintenance mode: `kscorectl maintenance enable`
5. Perform maintenance tasks
6. Disable maintenance mode: `kscorectl maintenance disable`

## Post-Maintenance
7. Verify cluster health: `kscorectl cluster health`
8. Verify agent connectivity: `kscorectl agent list --status`
9. Run smoke tests
10. Notify stakeholders of completion
```

### Runbook: Emergency Rollback

```markdown
## Trigger Conditions
- Error rate > 5% after upgrade
- P99 latency > 2x baseline
- Agent disconnection rate > 10%

## Rollback Procedure
1. Initiate rollback: `kscorectl upgrade rollback`
2. Monitor rollback: `kscorectl upgrade status --watch`
3. Verify rollback: `kscorectl cluster health`

## Post-Rollback
4. Collect diagnostics: `kscorectl diagnostics collect`
5. Create incident report
6. Schedule post-mortem
```

### Runbook: Certificate Rotation

```markdown
## Pre-Rotation
1. Verify current cert expiry: `kscorectl certs status`
2. Create backup: `kscore-cluster-backup create --components certificates`

## Rotation
3. Generate new certificates:
   ```bash
   kscorectl certs rotate --component server
   kscorectl certs rotate --component agent
   ```

4. Distribute new certificates:
   ```bash
   kscorectl state apply /etc/kscore/states/certificates.yaml
   ```

## Verification
5. Verify new certificates: `kscorectl certs verify`
6. Test connectivity: `kscorectl agent ping --all`
```

## Metrics and Alerting

### Key Metrics to Monitor

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `kscore_cluster_health` | Overall cluster health | != healthy |
| `kscore_backup_age_seconds` | Age of last backup | > 86400 (24h) |
| `kscore_backup_size_bytes` | Backup size | Deviation > 50% |
| `kscore_upgrade_status` | Current upgrade status | failed |
| `kscore_agent_version_mismatch` | Agents on wrong version | > 0 after upgrade |

### Prometheus Alert Rules

```yaml
groups:
  - name: keystone-self-management
    rules:
      - alert: BackupTooOld
        expr: time() - kscore_backup_last_success_timestamp > 86400
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Keystone backup is more than 24 hours old"

      - alert: BackupFailed
        expr: kscore_backup_last_status != 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Keystone backup failed"

      - alert: UpgradeFailed
        expr: kscore_upgrade_status == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Keystone upgrade failed"

      - alert: AgentVersionMismatch
        expr: kscore_agent_version_mismatch_count > 0
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Some agents are on different versions"
```

## Best Practices

### Backup Best Practices

1. **Test restores regularly** - Run restore drills monthly
2. **Encrypt all backups** - Use age or cloud KMS
3. **Replicate to multiple regions** - Protect against regional outages
4. **Monitor backup health** - Alert on failures or age
5. **Document recovery procedures** - Keep runbooks updated

### Upgrade Best Practices

1. **Test in staging first** - Never upgrade production directly
2. **Use canary deployments** - For major version upgrades
3. **Monitor metrics during upgrade** - Watch error rates and latency
4. **Keep rollback capability** - Don't delete previous version immediately
5. **Schedule during low-traffic periods** - Minimize impact

### Security Best Practices

1. **Rotate credentials regularly** - Certificates, tokens, passwords
2. **Store backup keys securely** - Use HSM or secure vault
3. **Audit access to backups** - Log all backup/restore operations
4. **Encrypt data at rest** - Both backups and operational data
5. **Use mTLS everywhere** - Server-to-server and agent-to-server
