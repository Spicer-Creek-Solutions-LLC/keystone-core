# Runbook: Scheduled Maintenance

## Overview

This runbook covers procedures for performing scheduled maintenance on a Keystone Core cluster.

## Prerequisites

- [ ] Maintenance window approved
- [ ] Stakeholders notified (24+ hours in advance)
- [ ] Backup completed within last 24 hours
- [ ] Rollback plan reviewed
- [ ] Change ticket created

## Trigger Conditions

- Scheduled OS patching
- Hardware maintenance
- Network changes
- Storage maintenance
- Routine health checks

## Procedure

### Pre-Maintenance (T-1 hour)

#### Step 1: Final Notifications

```bash
# Send final notification
# Update status page to "Scheduled Maintenance"
```

#### Step 2: Create Fresh Backup

```bash
# Create pre-maintenance backup
kscore-cluster-backup create \
  --dest /backup/pre-maintenance-$(date +%Y%m%d) \
  --label "pre-maintenance"

# Verify backup
kscore-cluster-backup verify /backup/pre-maintenance-*/latest.tar.gz
```

#### Step 3: Record Current State

```bash
# Record cluster state
kscorectl cluster health > /tmp/pre-maintenance-health.txt
kscorectl cluster members > /tmp/pre-maintenance-members.txt
kscorectl agents list > /tmp/pre-maintenance-agents.txt

# Record version info
kscorectl version > /tmp/pre-maintenance-version.txt
```

### During Maintenance

#### Step 4: Enable Maintenance Mode

```bash
# Enable maintenance mode
kscorectl maintenance enable

# Verify maintenance mode
kscorectl maintenance status
# Expected: "Maintenance mode: enabled"

# Note: In maintenance mode:
# - New state applications are queued
# - Scheduled jobs are paused
# - Monitoring alerts are suppressed
```

#### Step 5: Perform Maintenance Tasks

```bash
# Example: OS patching
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Patching $node..."
  ssh $node "sudo apt update && sudo apt upgrade -y"
done

# Example: Restart services
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Restarting services on $node..."
  ssh $node "sudo systemctl restart kscore-server"
  # Wait for node to become healthy
  sleep 30
  kscorectl cluster health
done
```

#### Step 6: Verify After Each Change

```bash
# After each significant change:
kscorectl cluster health
kscorectl agents list --status online -o json | jq length
```

### Post-Maintenance

#### Step 7: Disable Maintenance Mode

```bash
# Disable maintenance mode
kscorectl maintenance disable

# Verify maintenance mode disabled
kscorectl maintenance status
# Expected: "Maintenance mode: disabled"
```

#### Step 8: Verification

```bash
# Compare cluster state
diff /tmp/pre-maintenance-health.txt <(kscorectl cluster health)
diff /tmp/pre-maintenance-members.txt <(kscorectl cluster members)

# Verify all agents reconnected
BEFORE=$(cat /tmp/pre-maintenance-agents.txt | grep -c online)
AFTER=$(kscorectl agents list --status online -o json | jq length)
echo "Agents before: $BEFORE, after: $AFTER"

# Run smoke tests
kscore-test smoke
```

#### Step 9: Final Checks

```bash
# Full health check
kscorectl cluster health --verbose

# Check queued operations processed
kscorectl maintenance queue --status

# Verify scheduled operations resumed (check schedule plugin)
kscorectl schedule list
```

## Verification Checklist

- [ ] Maintenance mode disabled
- [ ] All cluster nodes healthy
- [ ] All agents reconnected
- [ ] Queued operations processed
- [ ] Scheduled jobs running
- [ ] Smoke tests passing
- [ ] No new alerts

## Rollback

If issues occur during maintenance:

```bash
# Restore from pre-maintenance backup
kscore-bootstrap restore \
  --backup /backup/pre-maintenance-*/latest.tar.gz

# Or revert specific changes
# (depends on what was changed)
```

## Post-Procedure

1. [ ] Update status page to "Operational"
2. [ ] Send completion notification
3. [ ] Close change ticket
4. [ ] Document any issues encountered
5. [ ] Update runbook if needed

## Appendix: Maintenance Mode Behavior

| Feature | Behavior in Maintenance Mode |
|---------|------------------------------|
| API | Available (read-only for most operations) |
| Agent Heartbeats | Accepted |
| State Applications | Queued |
| Scheduled Jobs | Paused |
| Events | Collected but not processed |
| Alerts | Suppressed |
| Backups | Can be triggered manually |

## Appendix: Common Maintenance Tasks

### OS Patching

```bash
# Rolling OS update
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Updating $node..."

  # Drain node
  kscorectl cluster drain $node

  # Update OS
  ssh $node "sudo apt update && sudo apt upgrade -y"

  # Reboot if needed
  ssh $node "sudo reboot" || true

  # Wait for node to come back
  sleep 60
  until ssh $node "echo 'ready'"; do sleep 10; done

  # Uncordon node
  kscorectl cluster undrain $node

  # Wait for healthy
  sleep 30
  kscorectl cluster health
done
```

### Certificate Renewal

See [certificate-rotation.md](certificate-rotation.md)

### Storage Maintenance

```bash
# Check disk usage
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "df -h /var/lib/keystone-core"
done

# Clean old data if needed
kscorectl maintenance cleanup --older-than 30d
```
