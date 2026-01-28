---
title: "Maintenance Guide"
weight: 3
description: >
  Operational maintenance procedures including backup, restore, upgrades, and migrations
---

## Overview

Regular maintenance is essential for reliable Keystone Core operations. This guide covers backup procedures, disaster recovery, version upgrades, database migrations, and capacity planning.

**Maintenance Tasks:**
- **Daily**: Automated backups, health checks
- **Weekly**: Backup verification, log rotation
- **Monthly**: Capacity review, upgrade planning
- **Quarterly**: Disaster recovery testing, security audits

## Backup Procedures

Keystone Core state should be backed up regularly to prevent data loss.

### What to Back Up

**Critical Data:**
1. **State Database** (SQLite or PostgreSQL)
   - Agent registrations
   - State definitions and history
   - Policy definitions
   - Job execution history

2. **Configuration Files**
   - `/etc/kscore/server.yaml`
   - `/etc/kscore/agent.yaml`
   - Reactor definitions
   - Policy files

3. **JetStream Data** (event history)
   - Optional - events can be reconstructed
   - Useful for audit compliance

**Non-Critical Data:**
- Prometheus metrics (ephemeral, can be lost)
- Log files (archived separately)
- Temporary caches

### Backup Strategies

**Full Backups:**
- Complete database snapshot
- Run daily during low-traffic window (e.g., 2 AM)
- Retain for 30 days minimum

**Incremental Backups:**
- Transaction logs (PostgreSQL WAL)
- Run hourly for point-in-time recovery
- Retain for 7 days

**Configuration Backups:**
- Git repository (recommended)
- Version controlled
- Automatic on every change

### SQLite Backup

**Online Backup (Hot Backup):**
```bash
#!/bin/bash
# /usr/local/bin/backup-sqlite.sh

BACKUP_DIR="/var/backups/kscore"
DB_PATH="/var/lib/kscore/state.db"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup database
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/state-$TIMESTAMP.db'"

# Compress backup
gzip "$BACKUP_DIR/state-$TIMESTAMP.db"

# Delete backups older than 30 days
find "$BACKUP_DIR" -name "state-*.db.gz" -mtime +30 -delete

echo "Backup completed: state-$TIMESTAMP.db.gz"
```

**Schedule with Cron:**
```bash
# /etc/cron.d/kscore-cluster-backup
0 2 * * * keystonecore /usr/local/bin/backup-sqlite.sh >> /var/log/kscore/backup.log 2>&1
```

**Verify Backup:**
```bash
# Test backup integrity
gunzip -c /var/backups/kscore/state-20240115-020000.db.gz | sqlite3 :memory: "PRAGMA integrity_check;"

# Expected output: ok
```

### PostgreSQL Backup

**pg_dump (Logical Backup):**
```bash
#!/bin/bash
# /usr/local/bin/backup-postgres.sh

BACKUP_DIR="/var/backups/kscore"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

# Full database dump
pg_dump -U kscore -h localhost -Fc keystonecore > "$BACKUP_DIR/postgres-$TIMESTAMP.dump"

# Schema-only backup
pg_dump -U kscore -h localhost -s > "$BACKUP_DIR/schema-$TIMESTAMP.sql"

# Cleanup old backups
find "$BACKUP_DIR" -name "postgres-*.dump" -mtime +30 -delete

echo "Backup completed: postgres-$TIMESTAMP.dump"
```

**pg_basebackup (Physical Backup):**
```bash
# Full cluster backup (includes all databases)
pg_basebackup -h localhost -U replicator -D /var/backups/postgres-base -Ft -z -P

# Creates compressed tarball of entire data directory
```

**Continuous Archiving (WAL Archiving):**
```ini
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'cp %p /var/lib/postgresql/wal_archive/%f'
archive_timeout = 300  # Force WAL archival every 5 minutes
```

**Automated Backup Script:**
```bash
#!/bin/bash
# Full backup + WAL archiving

# Daily full backup
pg_basebackup -h localhost -U replicator -D "/var/backups/postgres-$(date +%Y%m%d)" -Ft -z

# WAL files archived continuously via archive_command
```

### JetStream Backup

**Stream Backup:**
```bash
# Backup specific stream
nats stream backup kscore-events /var/backups/nats/events-$(date +%Y%m%d)

# Backup all streams
for stream in $(nats stream list -n); do
    nats stream backup "$stream" "/var/backups/nats/${stream}-$(date +%Y%m%d)"
done
```

### Cluster State Backup (HA Only)

For high-availability deployments, you can backup the cluster state via the API. This includes membership, shard assignments (which agents are managed by which control plane), and cluster configuration.

**Create Cluster Backup:**
```bash
# Backup cluster state to file
curl -H "Authorization: Bearer $API_KEY" \
  http://control-plane:8080/api/v1/cluster/backup > cluster-backup-$(date +%Y%m%d).json
```

**kscore-cluster-backup CLI (Recommended):**
```bash
# Create a backup
kscorectl cluster-backup backup --file /var/backups/kscore/cluster-backup.bin

# Verify a backup before restore
kscorectl cluster-backup verify --input /var/backups/kscore/cluster-backup.bin

# Restore from backup (use --dry-run to preview)
kscorectl cluster-backup restore --input /var/backups/kscore/cluster-backup.bin --dry-run
```

**Backup Contents:**
- Cluster membership and health status
- Shard assignments (agent-to-member mappings)
- Cluster configuration settings

**Automated Cluster Backup Script:**
```bash
#!/bin/bash
# /usr/local/bin/backup-cluster.sh

BACKUP_DIR="/var/backups/kscore/cluster"
API_URL="http://localhost:8080/api/v1"
API_KEY="${KSCORE_API_KEY}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

# Create cluster backup
curl -s -H "Authorization: Bearer $API_KEY" \
  "$API_URL/cluster/backup" > "$BACKUP_DIR/cluster-$TIMESTAMP.json"

# Verify backup is valid JSON
if jq empty "$BACKUP_DIR/cluster-$TIMESTAMP.json" 2>/dev/null; then
    echo "Cluster backup completed: cluster-$TIMESTAMP.json"
    # Cleanup old backups (keep 30 days)
    find "$BACKUP_DIR" -name "cluster-*.json" -mtime +30 -delete
else
    echo "ERROR: Cluster backup failed - invalid JSON"
    rm -f "$BACKUP_DIR/cluster-$TIMESTAMP.json"
    exit 1
fi
```

**Schedule with Cron:**
```bash
# /etc/cron.d/kscore-cluster-backup
0 * * * * keystonecore /usr/local/bin/backup-cluster.sh >> /var/log/kscore/cluster-backup.log 2>&1
```

### Configuration Backup

**Git Repository (Recommended):**
```bash
# Initialize git repo
cd /etc/kscore
git init
git add .
git commit -m "Initial configuration"

# Add remote
git remote add origin git@github.com:yourorg/kscore-config.git
git push -u origin main

# Automatic backup on changes
cat > /etc/kscore/.git/hooks/post-commit <<'EOF'
#!/bin/bash
git push origin main
EOF
chmod +x /etc/kscore/.git/hooks/post-commit
```

**Tarball Backup:**
```bash
# Backup all configs
tar -czf /var/backups/kscore/config-$(date +%Y%m%d).tar.gz /etc/kscore
```

### Backup Verification

**Weekly Verification Test:**
```bash
#!/bin/bash
# /usr/local/bin/verify-backup.sh

LATEST_BACKUP=$(ls -t /var/backups/kscore/postgres-*.dump | head -1)

# Restore to test database
createdb kscore_test
pg_restore -U kscore -d kscore_test "$LATEST_BACKUP"

# Run integrity checks
psql -U kscore -d kscore_test -c "SELECT COUNT(*) FROM agents;"
psql -U kscore -d kscore_test -c "SELECT COUNT(*) FROM state_resources;"

# Cleanup
dropdb kscore_test

echo "Backup verification passed: $LATEST_BACKUP"
```

### Off-Site Backup

**S3 Backup:**
```bash
# Upload to S3
aws s3 sync /var/backups/kscore/ s3://my-backups/kscore/ \
  --storage-class STANDARD_IA \
  --exclude "*" --include "*.dump" --include "*.db.gz"

# Verify upload
aws s3 ls s3://my-backups/kscore/
```

**rsync to Remote Server:**
```bash
# Daily rsync to backup server
rsync -avz --delete /var/backups/kscore/ backup-server:/backups/kscore/
```

## Restore Procedures

### SQLite Restore

**Full Restore:**
```bash
# Stop Keystone Core
sudo systemctl stop kscore-server

# Restore database
gunzip -c /var/backups/kscore/state-20240115-020000.db.gz > /var/lib/kscore/state.db

# Fix permissions
sudo chown kscore:kscore /var/lib/kscore/state.db

# Start Keystone Core
sudo systemctl start kscore-server

# Verify
kscorectl agent list
```

**Point-in-Time Recovery:**
Not supported with SQLite. Use PostgreSQL for PITR.

### PostgreSQL Restore

**Full Database Restore:**
```bash
# Stop Keystone Core
sudo systemctl stop kscore-server

# Drop and recreate database
dropdb kscore
createdb kscore

# Restore from dump
pg_restore -U kscore -d keystonecore /var/backups/kscore/postgres-20240115-020000.dump

# Start Keystone Core
sudo systemctl start kscore-server
```

**Point-in-Time Recovery (PITR):**
```bash
# 1. Restore base backup
tar -xzf /var/backups/postgres-20240115.tar.gz -C /var/lib/postgresql/14/main

# 2. Create recovery.conf
cat > /var/lib/postgresql/14/main/recovery.conf <<EOF
restore_command = 'cp /var/lib/postgresql/wal_archive/%f %p'
recovery_target_time = '2024-01-15 14:30:00'
EOF

# 3. Start PostgreSQL (will replay WAL to target time)
sudo systemctl start postgresql

# 4. Promote to primary when ready
psql -c "SELECT pg_wal_replay_resume();"
```

### JetStream Restore

**Stream Restore:**
```bash
# Restore stream from backup
nats stream restore kscore-events /var/backups/nats/events-20240115

# Verify messages
nats stream info kscore-events
```

### Configuration Restore

**From Git:**
```bash
cd /etc/kscore
git pull origin main
sudo systemctl restart kscore-server
```

**From Tarball:**
```bash
tar -xzf /var/backups/kscore/config-20240115.tar.gz -C /
sudo systemctl restart kscore-server
```

### Cluster State Restore (HA Only)

Restore cluster state from a backup file. This is useful when:
- Recovering from cluster failure
- Migrating to new hardware
- Restoring after accidental configuration changes

**Basic Restore:**
```bash
# Restore cluster state from backup
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup-20240115.json \
  http://control-plane:8080/api/v1/cluster/restore
```

**Restore Options:**

| Option | Default | Description |
|--------|---------|-------------|
| `force` | false | Override safety checks (cluster health, name match) |
| `restore_shards` | true | Restore agent-to-member assignments |
| `restore_config` | true | Restore cluster configuration settings |

**Force Restore (Healthy Cluster):**

By default, restore is blocked on healthy clusters to prevent accidental overwrites. Use `force=true` to override:
```bash
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  "http://control-plane:8080/api/v1/cluster/restore?force=true"
```

**Selective Restore:**
```bash
# Restore only configuration (not shard assignments)
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  "http://control-plane:8080/api/v1/cluster/restore?restore_shards=false"

# Restore only shard assignments (not configuration)
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  "http://control-plane:8080/api/v1/cluster/restore?restore_config=false"
```

**Restore Response:**
```json
{
  "success": true,
  "message": "Cluster restored successfully",
  "shards_restored": 150,
  "config_restored": 5,
  "warnings": [
    "Agent web-05 assigned to unavailable member server-3, reassigned to server-1"
  ]
}
```

**Safety Checks:**
- Backup version must be compatible (version "1.0" supported)
- Backup must have valid timestamp
- Cluster name must match (prevents restoring wrong cluster's backup)
- Cluster should not be healthy (use `force=true` to override)

**Intelligent Shard Reassignment:**

If agents were assigned to members that no longer exist, the restore process automatically reassigns them to healthy members.

**Post-Restore Verification:**
```bash
# Check cluster status
kscorectl cluster status

# Verify agent assignments
kscorectl cluster members

# Check agent health
kscorectl agent list --filter "status:online"
```

### Disaster Recovery Drill

**Quarterly DR Test:**
1. Spin up new infrastructure (test environment)
2. Restore all backups (database, configs, JetStream)
3. Verify agent connections
4. Execute test commands
5. Check state application
6. Verify policy enforcement
7. Document recovery time (RTO) and data loss (RPO)
8. Tear down test environment

**Expected Recovery Times:**
- RTO (Recovery Time Objective): <1 hour
- RPO (Recovery Point Objective): <1 hour (with hourly incremental backups)

## Upgrade Procedures

### Version Compatibility

**Semantic Versioning:**
- **Major** (1.0.0 → 2.0.0): Breaking changes, migration required
- **Minor** (1.0.0 → 1.1.0): New features, backward compatible
- **Patch** (1.0.0 → 1.0.1): Bug fixes, always safe to upgrade

**Compatibility Matrix:**
| Server Version | Agent Version | Notes |
|----------------|---------------|-------|
| 1.0.x | 1.0.x | Exact match recommended |
| 1.1.x | 1.0.x - 1.1.x | Minor version skew allowed |
| 2.0.x | 2.0.x | Major version must match |

### Pre-Upgrade Checklist

- [ ] Review release notes for breaking changes
- [ ] Backup database and configurations
- [ ] Test upgrade in staging environment
- [ ] Schedule maintenance window
- [ ] Notify users of downtime
- [ ] Verify rollback procedure
- [ ] Check disk space (2x current database size)

### Single-Node Upgrade

**1. Backup:**
```bash
# Full backup
/usr/local/bin/backup-sqlite.sh
```

**2. Download New Version:**
```bash
wget https://github.com/shawnbutts/keystone-core/releases/download/v0.2.0/kscore-server-linux-amd64
chmod +x kscore-server-linux-amd64
```

**3. Stop Service:**
```bash
sudo systemctl stop kscore-server
```

**4. Replace Binary:**
```bash
sudo mv kscore-server-linux-amd64 /usr/local/bin/kscore-server
```

**5. Run Migrations (if needed):**
```bash
kscore-server migrate --config /etc/kscore/server.yaml
```

**6. Start Service:**
```bash
sudo systemctl start kscore-server
```

**7. Verify:**
```bash
# Check version
kscorectl version

# Check health
curl http://localhost:8080/health/ready

# Check agents
kscorectl agent list
```

**Downtime:** 2-5 minutes

### High-Availability Rolling Upgrade

**Zero-downtime upgrade for HA clusters:**

**1. Upgrade Secondary Nodes First:**
```bash
# On server2 (secondary)
sudo systemctl stop kscore-server
sudo mv /tmp/kscore-server-new /usr/local/bin/kscore-server
sudo systemctl start kscore-server

# Wait for health check
curl http://server2:8080/health/ready

# Repeat for server3
```

**2. Upgrade Primary (Leader):**
```bash
# Trigger leader election to another node
kscorectl cluster step-down

# Wait for new leader election (30 seconds)
kscorectl cluster status

# Upgrade old leader
sudo systemctl stop kscore-server
sudo mv /tmp/kscore-server-new /usr/local/bin/kscore-server
sudo systemctl start kscore-server
```

**3. Verify Cluster:**
```bash
kscorectl cluster status

# All nodes should be healthy
# NODE      STATUS    VERSION   ROLE
# server1   healthy   1.1.0     follower
# server2   healthy   1.1.0     leader
# server3   healthy   1.1.0     follower
```

**Downtime:** 0 minutes (rolling upgrade)

### Kubernetes Upgrade

**Rolling Update:**
```bash
# Update image tag
kubectl set image deployment/kscore-server \
  kscore-server=keystonecore/server:v0.2.0 \
  -n kscore

# Watch rollout
kubectl rollout status deployment/kscore-server -n kscore

# Verify
kubectl get pods -n kscore
```

**With Helm:**
```bash
# Update chart values
helm upgrade keystonecore keystonecore/kscore \
  --namespace kscore \
  --set server.image.tag=v0.2.0 \
  --reuse-values

# Rollback if needed
helm rollback kscore -n kscore
```

### Agent Upgrades

**Canary Upgrade (Recommended):**
```bash
# 1. Upgrade 10% of agents
kscorectl exec run "wget https://.../kscore-agent-new && systemctl restart kscore-agent" \
  --target "datacenter:us-east-1" --limit 10%

# 2. Monitor for 1 hour
kscorectl agent list --filter "version:1.1.0"

# 3. If successful, upgrade remaining
kscorectl exec run "wget https://.../kscore-agent-new && systemctl restart kscore-agent" \
  --target "version:1.0.0"
```

**Automated with State Management:**
```yaml
# agent-upgrade.yaml
agent_binary:
  module: file
  state: present
  path: /usr/local/bin/kscore-agent
  source: https://releases.keystonecore.io/v0.2.0/kscore-agent-linux-amd64
  mode: "0755"

agent_restart:
  module: service
  state: running
  name: kscore-agent
  watch:
    - agent_binary
```

```bash
kscorectl state apply agent-upgrade.yaml
```

Run this on each agent host that you want to upgrade.

### Rollback Procedure

**If upgrade fails:**

**1. Stop New Version:**
```bash
sudo systemctl stop kscore-server
```

**2. Restore Old Binary:**
```bash
sudo mv /usr/local/bin/kscore-server.backup /usr/local/bin/kscore-server
```

**3. Restore Database (if migrations ran):**
```bash
pg_restore -U kscore -d keystonecore /var/backups/kscore/pre-upgrade-backup.dump
```

**4. Start Old Version:**
```bash
sudo systemctl start kscore-server
```

**5. Verify:**
```bash
kscorectl version
kscorectl agent list
```

## NATS Recovery (HA Only)

When NATS connectivity issues occur in an HA cluster, the coordination service provides recovery actions that can be triggered via the gRPC CoordinationService.

### Recovery Actions

**Restart Embedded NATS (Embedded Mode Only):**
```bash
# Using grpcurl (requires mTLS)
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-1", "initiator_id": "admin", "action": "RECOVERY_ACTION_RESTART_EMBEDDED"}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate
```

**Force Reconnection:**
```bash
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-2", "initiator_id": "admin", "action": "RECOVERY_ACTION_RECONNECT"}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate
```

**Failover to Backup NATS Servers:**
```bash
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-3", "initiator_id": "admin", "action": "RECOVERY_ACTION_FAILOVER", "parameters": {"target_urls": "nats://backup1:4222,nats://backup2:4222"}}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate
```

**Drain Connections (Before Maintenance):**
```bash
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "recovery-4", "initiator_id": "admin", "action": "RECOVERY_ACTION_DRAIN"}' \
  server1:9443 keystone.core.v1.CoordinationService/RecoveryCoordinate
```

### State Propagation During NATS Outage

When NATS is unavailable, critical state changes are propagated via the CoordinationService:

**State Update Types:**
- `AGENT_REGISTER` - New agent registrations
- `AGENT_HEARTBEAT` - Agent heartbeat updates
- `AGENT_DISCONNECT` - Agent disconnect notifications
- `COMMAND_RESULT` - Command execution results
- `MEMBERSHIP_CHANGE` - Cluster membership changes

State propagation includes version tracking to prevent stale updates from being applied.

### Checking NATS Status

**Query NATS status on a specific server:**
```bash
grpcurl -cacert ca.crt -cert client.crt -key client.key \
  -d '{"request_id": "status-1", "requester_id": "admin"}' \
  server1:9443 keystone.core.v1.CoordinationService/NATSStatus
```

**Response includes:**
- Connection status (connected, connecting, reconnecting, disconnected)
- Connected NATS URLs
- JetStream availability
- Last successful publish/subscribe timestamps

### Recovery Workflow

1. **Detect**: Monitor cluster health for NATS connectivity issues
2. **Assess**: Check NATS status on all servers via CoordinationService
3. **Coordinate**: Use RecoveryCoordinate to trigger recovery action
4. **Verify**: Confirm NATS connectivity restored
5. **Resume**: Use RESUME action if PAUSE was used

## Database Maintenance

### SQLite Maintenance

**Vacuum (Defragment):**
```bash
# Reclaim space from deleted records
sqlite3 /var/lib/kscore/state.db "VACUUM;"

# Analyze for query optimization
sqlite3 /var/lib/kscore/state.db "ANALYZE;"
```

**Integrity Check:**
```bash
sqlite3 /var/lib/kscore/state.db "PRAGMA integrity_check;"
```

**Size Monitoring:**
```bash
# Check database size
du -h /var/lib/kscore/state.db

# Alert if >10GB (approaching SQLite limits)
```

### PostgreSQL Maintenance

**Vacuum:**
```bash
# Manual vacuum
vacuumdb -U kscore -d keystonecore -v

# Analyze statistics
vacuumdb -U kscore -d keystonecore -z

# Full vacuum (reclaims disk space, requires table lock)
vacuumdb -U kscore -d keystonecore -f
```

**Autovacuum Configuration:**
```ini
# postgresql.conf
autovacuum = on
autovacuum_max_workers = 3
autovacuum_naptime = 1min
autovacuum_vacuum_threshold = 50
autovacuum_analyze_threshold = 50
```

**Reindex:**
```bash
# Rebuild indexes
reindexdb -U kscore -d keystonecore
```

**Statistics Update:**
```sql
ANALYZE VERBOSE;
```

### Migration: SQLite → PostgreSQL

**When to Migrate:**
- Approaching 100 managed agents
- Database size >5GB
- Need for high availability
- Require point-in-time recovery

**Migration Steps:**

**1. Set Up PostgreSQL:**
```bash
# See Deployment Guide for PostgreSQL setup
sudo apt-get install postgresql-14
sudo -u postgres createuser kscore
sudo -u postgres createdb -O kscore keystonecore
```

**2. Plan the Migration (Dry Run):**
```bash
# Stop control plane
sudo systemctl stop kscore-server

# Dry run to see what will be migrated
kscorectl migrate run \
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore" \
  --dry-run --verbose
```

**3. Run the Migration:**
```bash
# Migrate all data from SQLite to PostgreSQL
kscorectl migrate run \
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"

# Output shows progress:
#   agents: 0/150
#   agents: 100/150
#   agents: 150/150
#   commands: 0/1234
#   ...
#   Migration completed!
#   Duration: 2.5s
#   Agents migrated: 150
#   Commands migrated: 1234
#   Batch jobs migrated: 45
#   Batch agent results migrated: 890
```

**4. Validate the Migration:**
```bash
# Verify all data was migrated correctly
kscorectl migrate validate \
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"

# Output:
#   Record counts:
#     Agents:             Source=150  Target=150
#     Commands:           Source=1234 Target=1234
#     Batch jobs:         Source=45   Target=45
#     Batch agent results: Source=890  Target=890
#
#   Validation PASSED - all record counts match
```

**5. Update Configuration:**
```yaml
# /etc/kscore/server.yaml
storage:
  type: postgresql
  postgresql:
    host: localhost
    port: 5432
    database: keystonecore
    username: kscore
    password: $POSTGRES_PASSWORD
```

**6. Start and Verify:**
```bash
sudo systemctl start kscore-server

# Verify agent count
kscorectl agent list | wc -l

# Verify all agents are healthy
kscorectl agent list --filter "status:online"
```

**7. Backup SQLite (Archive):**
```bash
gzip /var/lib/kscore/state.db
mv /var/lib/kscore/state.db.gz /var/backups/kscore/sqlite-archive-$(date +%Y%m%d).db.gz
```

**Migration Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | Validate without writing to target |
| `--batch-size` | 100 | Records per progress update |
| `--continue-on-error` | false | Continue even if some records fail |
| `--skip-existing` | true | Skip records already in target |
| `--verbose` | false | Show detailed progress |

**Incremental Migration:**

If you need to migrate while the system is running (not recommended for production):
```bash
# First migration
kscorectl migrate run --sqlite ... --postgres ... --skip-existing

# Later, migrate any new records
kscorectl migrate run --sqlite ... --postgres ... --skip-existing
```

**Rollback:**

If migration fails, the source SQLite database is unchanged. Simply:
1. Fix the PostgreSQL issue
2. Re-run the migration
3. Or continue using SQLite

## Data Retention

### Event Retention

**Configure Retention:**
```yaml
# server.yaml
events:
  storage:
    retention:
      max_age: "30d"        # Delete events older than 30 days
      max_count: 1000000    # Keep max 1M events
      min_severity: "info"  # Delete debug events
    type_retention:
      "agent.heartbeat": "1d"    # Keep heartbeats for 1 day
      "state.drift": "90d"        # Keep drift events for 90 days
      "user.custom": "365d"       # Keep custom events for 1 year
```

**Manual Cleanup:**
```sql
-- Delete old events
DELETE FROM events
WHERE timestamp < NOW() - INTERVAL '30 days'
  AND severity NOT IN ('error', 'critical');
```

### Log Retention

**Logrotate Configuration:**
```
# /etc/logrotate.d/kscore
/var/log/kscore/*.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    create 0644 kscore kscore
    sharedscripts
    postrotate
        systemctl reload kscore-server > /dev/null 2>&1 || true
    endscript
}
```

### Metric Retention

**Prometheus Retention:**
```yaml
# prometheus.yml
storage:
  tsdb:
    retention.time: 30d
    retention.size: 100GB
```

**Recording Rules (Aggregate Historical Data):**
```yaml
# Downsample to 5-minute averages for long-term storage
groups:
  - name: downsampling
    interval: 5m
    rules:
      - record: kscore:api:request_rate:5m
        expr: rate(kscore_api_requests_total[5m])

      - record: kscore:api:latency:p95:5m
        expr: histogram_quantile(0.95, rate(kscore_api_request_duration_seconds_bucket[5m]))
```

## Capacity Planning

### Growth Estimation

**Monitor Growth Trends:**
```promql
# Agent growth rate (agents/month)
deriv(kscore_agents_total[30d]) * 86400 * 30

# State resource growth
deriv(kscore_state_resources_total[30d]) * 86400 * 30

# Event rate growth
deriv(rate(kscore_events_published_total[5m])[30d]) * 86400 * 30
```

### Resource Planning

**Control Plane Sizing:**
| Agents | CPU | Memory | Disk | Network |
|--------|-----|--------|------|---------|
| 100 | 2 cores | 4GB | 50GB | 10Mbps |
| 500 | 4 cores | 8GB | 100GB | 50Mbps |
| 1000 | 8 cores | 16GB | 200GB | 100Mbps |
| 5000 | 16 cores | 32GB | 500GB | 500Mbps |

**Database Sizing:**
| State Resources | Disk Space | IOPS |
|-----------------|------------|------|
| 10,000 | 10GB | 1,000 |
| 100,000 | 50GB | 5,000 |
| 1,000,000 | 200GB | 10,000 |

**JetStream Sizing:**
| Events/Day | Retention | Disk Space |
|------------|-----------|------------|
| 100K | 30d | 10GB |
| 1M | 30d | 50GB |
| 10M | 30d | 200GB |

### Scaling Triggers

**Add Control Plane Capacity When:**
- CPU usage >70% sustained
- Memory usage >80%
- API latency P95 >500ms
- Disk space <20% free

**Scale Database When:**
- Query latency P95 >100ms
- Connection pool exhaustion
- Disk I/O saturation >80%

## Best Practices

### Backup
- **3-2-1 Rule**: 3 copies, 2 different media, 1 off-site
- **Test Restores**: Verify backups monthly
- **Automate Everything**: No manual backup processes
- **Monitor Backup Jobs**: Alert on failures

### Upgrades
- **Test in Staging**: Always test upgrades before production
- **Rolling Upgrades**: Use for zero downtime (HA only)
- **Canary Agents**: Upgrade 10% of agents first
- **Keep Old Binaries**: For quick rollback

### Maintenance Windows
- **Schedule Off-Peak**: 2-4 AM typical
- **Notify Users**: 48 hours advance notice
- **Document Changes**: Changelog for every upgrade
- **Post-Maintenance Checks**: Verify all systems healthy

## Troubleshooting Maintenance

### Backup Failures

**Disk Full:**
```bash
# Check disk space
df -h /var/backups

# Clean old backups
find /var/backups -mtime +30 -delete
```

**Database Lock:**
```bash
# Check for long-running queries
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state != 'idle'
ORDER BY duration DESC;

# Kill blocking query if needed
SELECT pg_terminate_backend(pid);
```

### Upgrade Failures

**Migration Failed:**
```bash
# Check migration log
journalctl -u kscore-server | grep migration

# Rollback to previous version
/usr/local/bin/kscore-server.backup --version
```

**Version Mismatch:**
```bash
# Check all component versions
kscorectl version        # CLI
kscorectl cluster status # Server
kscorectl agent list     # Agents

# Upgrade mismatched components
```

## See Also

- [Deployment Guide](deployment/) - Initial deployment
- [Monitoring Guide](monitoring/) - Track system health
- [Troubleshooting Guide](troubleshooting/) - Resolve issues
- [Configuration Reference](/docs/reference/configuration/) - All config options
