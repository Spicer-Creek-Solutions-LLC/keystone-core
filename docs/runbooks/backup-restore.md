# Runbook: Backup and Restore

## Overview

This runbook covers backup creation, verification, and restoration procedures for Keystone Core.

## Prerequisites

- [ ] Backup destination configured and accessible
- [ ] Encryption keys available (if using encryption)
- [ ] Sufficient storage space at destination
- [ ] Access to control plane nodes

## Backup Procedures

### Create Full Backup

```bash
# Create full backup to local storage
kscore-backup create \
  --type full \
  --dest /backup/keystone

# Create full backup to S3
kscore-backup create \
  --type full \
  --dest s3://keystone-backups/$(date +%Y/%m/%d)/ \
  --encrypt \
  --encrypt-recipient age1...

# Create backup with specific label
kscore-backup create \
  --type full \
  --dest /backup/keystone \
  --label "pre-upgrade-1.6.0"
```

### Create Incremental Backup

```bash
# Incremental backup (database changes only)
kscore-backup create \
  --type incremental \
  --dest /backup/keystone \
  --base-backup /backup/keystone/full-2024-01-15.tar.gz
```

### Create Component-Specific Backup

```bash
# Database only
kscore-backup create \
  --components database \
  --dest /backup/db-only

# Configuration only
kscore-backup create \
  --components config,certificates \
  --dest /backup/config-only

# JetStream only
kscore-backup create \
  --components jetstream \
  --dest /backup/jetstream-only
```

### Verify Backup

```bash
# Verify backup integrity
kscore-backup verify /backup/keystone/backup-2024-01-15.tar.gz

# Expected output:
# Verifying backup...
# - Manifest: OK
# - Checksum: OK
# - Components: 5/5 OK
# - Decryptable: OK (if encrypted)
# Backup verification passed
```

### List Backups

```bash
# List local backups
kscore-backup list --dest /backup/keystone

# List S3 backups
kscore-backup list --dest s3://keystone-backups/

# List with details
kscore-backup list --dest /backup/keystone --verbose

# Output:
# Backup                           Size      Date                 Components
# backup-2024-01-15T02-00-00.tar.gz  1.2GB    2024-01-15 02:00:00  full
# backup-2024-01-14T02-00-00.tar.gz  1.1GB    2024-01-14 02:00:00  full
# backup-2024-01-13T02-00-00.tar.gz  1.1GB    2024-01-13 02:00:00  full
```

## Restore Procedures

### Pre-Restore Checklist

- [ ] Identify correct backup to restore
- [ ] Verify backup integrity
- [ ] Ensure decryption keys available
- [ ] Plan for service interruption
- [ ] Notify stakeholders

### Full Restore

```bash
# Stop services on all nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "sudo systemctl stop kscore-server"
done

# Restore on first node
kscore-bootstrap restore \
  --backup /backup/keystone/backup-2024-01-15.tar.gz \
  --decrypt-identity /secure/backup-key.txt

# Start services
sudo systemctl start kscore-server

# Rejoin other nodes
# (existing data will be replaced with restored data via cluster sync)
```

### Partial Restore

```bash
# Restore configuration only
kscore-bootstrap restore \
  --backup /backup/keystone/backup.tar.gz \
  --components config \
  --no-restart

# Apply restored config
sudo systemctl restart kscore-server

# Restore certificates only
kscore-bootstrap restore \
  --backup /backup/keystone/backup.tar.gz \
  --components certificates \
  --no-restart
```

### Restore to Different Cluster

```bash
# Extract backup
mkdir /tmp/restore
tar -xzf /backup/keystone/backup.tar.gz -C /tmp/restore

# Modify configuration for new environment
vim /tmp/restore/config/server.yaml

# Import into new cluster
kscore-bootstrap import \
  --from-backup /tmp/restore \
  --new-cluster-name new-keystone
```

### Point-in-Time Recovery (PostgreSQL)

```bash
# Requires WAL archiving configured
# Restore to specific timestamp
kscore-bootstrap restore \
  --backup /backup/base-2024-01-15.tar.gz \
  --target-time "2024-01-15 14:30:00 UTC" \
  --wal-archive s3://keystone-wal/
```

## Verification Checklist

After restore:

- [ ] Server starts successfully
- [ ] Cluster health is healthy
- [ ] Database queries work
- [ ] All expected data present
- [ ] Agents reconnect
- [ ] Scheduled jobs resume

## Troubleshooting

### Backup Fails

```bash
# Check disk space
df -h /backup

# Check destination permissions
ls -la /backup/

# Check cloud credentials
aws s3 ls s3://keystone-backups/

# Run with debug logging
kscore-backup create --dest /backup --debug
```

### Restore Fails

```bash
# Verify backup integrity
kscore-backup verify /backup/backup.tar.gz

# Check decryption key
kscore-backup verify /backup/backup.tar.gz --decrypt-identity /path/to/key

# Extract and inspect manually
tar -tzf /backup/backup.tar.gz

# Check logs
journalctl -u kscore-server -n 100
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "Backup destination not accessible" | Network/permissions | Check connectivity and credentials |
| "Decryption failed" | Wrong key | Verify correct identity file |
| "Checksum mismatch" | Corrupt backup | Use different backup |
| "Incompatible version" | Version mismatch | Use compatible restore method |

## Post-Procedure

1. [ ] Verify restored data integrity
2. [ ] Test critical functionality
3. [ ] Update monitoring
4. [ ] Document restore details
5. [ ] Notify stakeholders

## Appendix: Backup Schedule Recommendations

| Environment | Full Backup | Incremental | Retention |
|-------------|-------------|-------------|-----------|
| Production | Daily 2AM | Every 6 hours | 30 days |
| Staging | Daily | N/A | 7 days |
| Development | Weekly | N/A | 3 days |

## Appendix: Backup Storage Sizing

```
Estimate backup size:
- Database: ~1-10GB (depends on state count)
- Configuration: <1MB
- Certificates: <1MB
- JetStream: 1-100GB (depends on event retention)
- etcd: 100MB-10GB (depends on cluster data)

Monthly storage = (daily_size × 7) + (weekly_size × 4) + (monthly_size × 12)
```
