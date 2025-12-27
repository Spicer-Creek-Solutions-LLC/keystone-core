---
title: "Maintenance Guide"
weight: 3
description: >
  Operational maintenance procedures including backup, restore, upgrades, and migrations
---

## Overview

Regular maintenance is essential for reliable TitanAnvil operations. This guide covers backup procedures, disaster recovery, version upgrades, database migrations, and capacity planning.

**Maintenance Tasks:**
- **Daily**: Automated backups, health checks
- **Weekly**: Backup verification, log rotation
- **Monthly**: Capacity review, upgrade planning
- **Quarterly**: Disaster recovery testing, security audits

## Backup Procedures

TitanAnvil state should be backed up regularly to prevent data loss.

### What to Back Up

**Critical Data:**
1. **State Database** (SQLite or PostgreSQL)
   - Agent registrations
   - State definitions and history
   - Policy definitions
   - Job execution history

2. **Configuration Files**
   - `/etc/titan anvil/server.yaml`
   - `/etc/titananvil/agent.yaml`
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

BACKUP_DIR="/var/backups/titananvil"
DB_PATH="/var/lib/titananvil/state.db"
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
# /etc/cron.d/titananvil-backup
0 2 * * * titananvil /usr/local/bin/backup-sqlite.sh >> /var/log/titananvil/backup.log 2>&1
```

**Verify Backup:**
```bash
# Test backup integrity
gunzip -c /var/backups/titananvil/state-20240115-020000.db.gz | sqlite3 :memory: "PRAGMA integrity_check;"

# Expected output: ok
```

### PostgreSQL Backup

**pg_dump (Logical Backup):**
```bash
#!/bin/bash
# /usr/local/bin/backup-postgres.sh

BACKUP_DIR="/var/backups/titananvil"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

# Full database dump
pg_dump -U titananvil -h localhost -Fc titananvil > "$BACKUP_DIR/postgres-$TIMESTAMP.dump"

# Schema-only backup
pg_dump -U titananvil -h localhost -s > "$BACKUP_DIR/schema-$TIMESTAMP.sql"

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
nats stream backup titananvil-events /var/backups/nats/events-$(date +%Y%m%d)

# Backup all streams
for stream in $(nats stream list -n); do
    nats stream backup "$stream" "/var/backups/nats/${stream}-$(date +%Y%m%d)"
done
```

### Configuration Backup

**Git Repository (Recommended):**
```bash
# Initialize git repo
cd /etc/titananvil
git init
git add .
git commit -m "Initial configuration"

# Add remote
git remote add origin git@github.com:yourorg/titananvil-config.git
git push -u origin main

# Automatic backup on changes
cat > /etc/titananvil/.git/hooks/post-commit <<'EOF'
#!/bin/bash
git push origin main
EOF
chmod +x /etc/titananvil/.git/hooks/post-commit
```

**Tarball Backup:**
```bash
# Backup all configs
tar -czf /var/backups/titananvil/config-$(date +%Y%m%d).tar.gz /etc/titananvil
```

### Backup Verification

**Weekly Verification Test:**
```bash
#!/bin/bash
# /usr/local/bin/verify-backup.sh

LATEST_BACKUP=$(ls -t /var/backups/titananvil/postgres-*.dump | head -1)

# Restore to test database
createdb titananvil_test
pg_restore -U titananvil -d titananvil_test "$LATEST_BACKUP"

# Run integrity checks
psql -U titananvil -d titananvil_test -c "SELECT COUNT(*) FROM agents;"
psql -U titananvil -d titananvil_test -c "SELECT COUNT(*) FROM state_resources;"

# Cleanup
dropdb titananvil_test

echo "Backup verification passed: $LATEST_BACKUP"
```

### Off-Site Backup

**S3 Backup:**
```bash
# Upload to S3
aws s3 sync /var/backups/titananvil/ s3://my-backups/titananvil/ \
  --storage-class STANDARD_IA \
  --exclude "*" --include "*.dump" --include "*.db.gz"

# Verify upload
aws s3 ls s3://my-backups/titananvil/
```

**rsync to Remote Server:**
```bash
# Daily rsync to backup server
rsync -avz --delete /var/backups/titananvil/ backup-server:/backups/titananvil/
```

## Restore Procedures

### SQLite Restore

**Full Restore:**
```bash
# Stop TitanAnvil
sudo systemctl stop titananvil-server

# Restore database
gunzip -c /var/backups/titananvil/state-20240115-020000.db.gz > /var/lib/titananvil/state.db

# Fix permissions
sudo chown titananvil:titananvil /var/lib/titananvil/state.db

# Start TitanAnvil
sudo systemctl start titananvil-server

# Verify
titanctl agent list
```

**Point-in-Time Recovery:**
Not supported with SQLite. Use PostgreSQL for PITR.

### PostgreSQL Restore

**Full Database Restore:**
```bash
# Stop TitanAnvil
sudo systemctl stop titananvil-server

# Drop and recreate database
dropdb titananvil
createdb titananvil

# Restore from dump
pg_restore -U titananvil -d titananvil /var/backups/titananvil/postgres-20240115-020000.dump

# Start TitanAnvil
sudo systemctl start titananvil-server
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
nats stream restore titananvil-events /var/backups/nats/events-20240115

# Verify messages
nats stream info titananvil-events
```

### Configuration Restore

**From Git:**
```bash
cd /etc/titananvil
git pull origin main
sudo systemctl restart titananvil-server
```

**From Tarball:**
```bash
tar -xzf /var/backups/titananvil/config-20240115.tar.gz -C /
sudo systemctl restart titananvil-server
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
wget https://github.com/titananvil/titananvil/releases/download/v1.1.0/titananvil-server-linux-amd64
chmod +x titananvil-server-linux-amd64
```

**3. Stop Service:**
```bash
sudo systemctl stop titananvil-server
```

**4. Replace Binary:**
```bash
sudo mv titananvil-server-linux-amd64 /usr/local/bin/titananvil-server
```

**5. Run Migrations (if needed):**
```bash
titananvil-server migrate --config /etc/titananvil/server.yaml
```

**6. Start Service:**
```bash
sudo systemctl start titananvil-server
```

**7. Verify:**
```bash
# Check version
titanctl version

# Check health
curl http://localhost:8080/health/ready

# Check agents
titanctl agent list
```

**Downtime:** 2-5 minutes

### High-Availability Rolling Upgrade

**Zero-downtime upgrade for HA clusters:**

**1. Upgrade Secondary Nodes First:**
```bash
# On server2 (secondary)
sudo systemctl stop titananvil-server
sudo mv /tmp/titananvil-server-new /usr/local/bin/titananvil-server
sudo systemctl start titananvil-server

# Wait for health check
curl http://server2:8080/health/ready

# Repeat for server3
```

**2. Upgrade Primary (Leader):**
```bash
# Trigger leader election to another node
titanctl cluster step-down

# Wait for new leader election (30 seconds)
titanctl cluster status

# Upgrade old leader
sudo systemctl stop titananvil-server
sudo mv /tmp/titananvil-server-new /usr/local/bin/titananvil-server
sudo systemctl start titananvil-server
```

**3. Verify Cluster:**
```bash
titanctl cluster status

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
kubectl set image deployment/titananvil-server \
  titananvil-server=titananvil/server:v1.1.0 \
  -n titananvil

# Watch rollout
kubectl rollout status deployment/titananvil-server -n titananvil

# Verify
kubectl get pods -n titananvil
```

**With Helm:**
```bash
# Update chart values
helm upgrade titananvil titananvil/titananvil \
  --namespace titananvil \
  --set server.image.tag=v1.1.0 \
  --reuse-values

# Rollback if needed
helm rollback titananvil -n titananvil
```

### Agent Upgrades

**Canary Upgrade (Recommended):**
```bash
# 1. Upgrade 10% of agents
titanctl exec run "wget https://.../titananvil-agent-new && systemctl restart titananvil-agent" \
  --target "datacenter:us-east-1" --limit 10%

# 2. Monitor for 1 hour
titanctl agent list --filter "version:1.1.0"

# 3. If successful, upgrade remaining
titanctl exec run "wget https://.../titananvil-agent-new && systemctl restart titananvil-agent" \
  --target "version:1.0.0"
```

**Automated with State Management:**
```yaml
# agent-upgrade.yaml
agent_binary:
  module: file
  state: present
  path: /usr/local/bin/titananvil-agent
  source: https://releases.titananvil.io/v1.1.0/titananvil-agent-linux-amd64
  mode: "0755"

agent_restart:
  module: service
  state: running
  name: titananvil-agent
  watch:
    - agent_binary
```

```bash
titanctl state apply agent-upgrade.yaml --target "all"
```

### Rollback Procedure

**If upgrade fails:**

**1. Stop New Version:**
```bash
sudo systemctl stop titananvil-server
```

**2. Restore Old Binary:**
```bash
sudo mv /usr/local/bin/titananvil-server.backup /usr/local/bin/titananvil-server
```

**3. Restore Database (if migrations ran):**
```bash
pg_restore -U titananvil -d titananvil /var/backups/titananvil/pre-upgrade-backup.dump
```

**4. Start Old Version:**
```bash
sudo systemctl start titananvil-server
```

**5. Verify:**
```bash
titanctl version
titanctl agent list
```

## Database Maintenance

### SQLite Maintenance

**Vacuum (Defragment):**
```bash
# Reclaim space from deleted records
sqlite3 /var/lib/titananvil/state.db "VACUUM;"

# Analyze for query optimization
sqlite3 /var/lib/titananvil/state.db "ANALYZE;"
```

**Integrity Check:**
```bash
sqlite3 /var/lib/titananvil/state.db "PRAGMA integrity_check;"
```

**Size Monitoring:**
```bash
# Check database size
du -h /var/lib/titananvil/state.db

# Alert if >10GB (approaching SQLite limits)
```

### PostgreSQL Maintenance

**Vacuum:**
```bash
# Manual vacuum
vacuumdb -U titananvil -d titananvil -v

# Analyze statistics
vacuumdb -U titananvil -d titananvil -z

# Full vacuum (reclaims disk space, requires table lock)
vacuumdb -U titananvil -d titananvil -f
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
reindexdb -U titananvil -d titananvil
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
sudo -u postgres createuser titananvil
sudo -u postgres createdb -O titananvil titananvil
```

**2. Export SQLite Data:**
```bash
# Stop control plane
sudo systemctl stop titananvil-server

# Export to SQL
titananvil-migrate export \
  --source sqlite:///var/lib/titananvil/state.db \
  --format sql \
  --output /tmp/export.sql
```

**3. Import to PostgreSQL:**
```bash
titananvil-migrate import \
  --input /tmp/export.sql \
  --target postgres://titananvil:password@localhost/titananvil \
  --validate
```

**4. Update Configuration:**
```yaml
# /etc/titananvil/server.yaml
storage:
  type: postgresql
  postgresql:
    host: localhost
    port: 5432
    database: titananvil
    username: titananvil
    password: $POSTGRES_PASSWORD
```

**5. Start and Verify:**
```bash
sudo systemctl start titananvil-server

# Verify agent count
titanctl agent list | wc -l

# Verify state resources
titanctl state list | wc -l
```

**6. Backup SQLite (Archive):**
```bash
gzip /var/lib/titananvil/state.db
mv /var/lib/titananvil/state.db.gz /var/backups/titananvil/sqlite-archive-$(date +%Y%m%d).db.gz
```

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
# /etc/logrotate.d/titananvil
/var/log/titananvil/*.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    create 0644 titananvil titananvil
    sharedscripts
    postrotate
        systemctl reload titananvil-server > /dev/null 2>&1 || true
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
      - record: titananvil:api:request_rate:5m
        expr: rate(titananvil_api_requests_total[5m])

      - record: titananvil:api:latency:p95:5m
        expr: histogram_quantile(0.95, rate(titananvil_api_request_duration_seconds_bucket[5m]))
```

## Capacity Planning

### Growth Estimation

**Monitor Growth Trends:**
```promql
# Agent growth rate (agents/month)
deriv(titananvil_agents_total[30d]) * 86400 * 30

# State resource growth
deriv(titananvil_state_resources_total[30d]) * 86400 * 30

# Event rate growth
deriv(rate(titananvil_events_published_total[5m])[30d]) * 86400 * 30
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
journalctl -u titananvil-server | grep migration

# Rollback to previous version
/usr/local/bin/titananvil-server.backup --version
```

**Version Mismatch:**
```bash
# Check all component versions
titanctl version        # CLI
titanctl cluster status # Server
titanctl agent list     # Agents

# Upgrade mismatched components
```

## See Also

- [Deployment Guide](deployment/) - Initial deployment
- [Monitoring Guide](monitoring/) - Track system health
- [Troubleshooting Guide](troubleshooting/) - Resolve issues
- [Configuration Reference](/docs/reference/configuration/) - All config options
