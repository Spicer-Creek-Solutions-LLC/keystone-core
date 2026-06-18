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
# Create pre-maintenance cluster snapshot
kscore-cluster-backup backup \
  --output /backup/pre-maintenance-$(date +%Y%m%d).tar \
  --description "pre-maintenance"

# Verify backup
kscore-cluster-backup verify --input /backup/pre-maintenance-*/latest.tar.gz
```

#### Step 3: Record Current State

```bash
# Record cluster state
kscore-cluster status > /tmp/pre-maintenance-health.txt
kscore-cluster members > /tmp/pre-maintenance-members.txt
kscorectl agent list > /tmp/pre-maintenance-agents.txt

# Record version info
kscorectl --version > /tmp/pre-maintenance-version.txt
```

### During Maintenance

#### Step 4: Quiesce Operator-Driven Work

> **v0.x scope note**: A built-in maintenance mode (`kscorectl maintenance
> enable|disable|status`) that queues state applications, pauses scheduled
> jobs, and suppresses alerts is not available in v0.1 — the maintenance
> API is a 410 stub and the capability is tracked for post-v1.0 in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). In the meantime,
> achieve an equivalent quiet window manually:
>
> - Pause operator-driven applies (stop your GitOps reconcile loop / hold
>   off running `kscorectl state apply`) so no new desired state lands
>   mid-maintenance.
> - Silence or mute the relevant alerts in your external monitoring stack
>   for the maintenance window.
>
> Agent heartbeats and the API remain live throughout; there is no server
> flag to flip.

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
  kscore-cluster status
done
```

#### Step 6: Verify After Each Change

```bash
# After each significant change:
kscore-cluster status
kscorectl agent list --status connected -o json | jq length
```

### Post-Maintenance

#### Step 7: Resume Operator-Driven Work

> **v0.x scope note**: There is no maintenance mode to disable in v0.1
> (see Step 4). Reverse the manual quiesce instead:
>
> - Re-enable operator-driven applies (resume your GitOps reconcile loop).
> - Un-silence the alerts you muted for the window.

#### Step 8: Verification

```bash
# Compare cluster state
diff /tmp/pre-maintenance-health.txt <(kscore-cluster status)
diff /tmp/pre-maintenance-members.txt <(kscore-cluster members)

# Verify all agents reconnected
BEFORE=$(cat /tmp/pre-maintenance-agents.txt | grep -c online)
AFTER=$(kscorectl agent list --status connected -o json | jq length)
echo "Agents before: $BEFORE, after: $AFTER"

# Run smoke tests
kscore-test smoke
```

#### Step 9: Final Checks

```bash
# Full health check
kscore-cluster status
```

> **v0.x scope note**: A maintenance operation queue
> (`kscorectl maintenance queue`) and a scheduled-jobs engine
> (`kscorectl schedule list`) are not available in v0.1 — both are
> post-v1.0, tracked in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). Because nothing is
> queued or paused on the server, there is no queue to drain or schedule
> to resume; confirm operator-driven applies are flowing again by checking
> that your GitOps reconcile loop has run cleanly post-maintenance.

## Verification Checklist

- [ ] Operator-driven applies resumed (GitOps reconcile loop re-enabled)
- [ ] Muted alerts un-silenced
- [ ] All cluster nodes healthy
- [ ] All agents reconnected
- [ ] Smoke tests passing
- [ ] No new alerts

## Rollback

If issues occur during maintenance:

```bash
# Restore the cluster snapshot from the pre-maintenance backup
kscore-cluster-backup restore \
  --input /backup/pre-maintenance-*/latest.tar.gz

# (Config-only restore from a kscore-backup archive uses:)
#   kscore-backup restore --src /backup/pre-maintenance-*/latest.tar.gz

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

> **v0.x scope note**: A built-in maintenance mode is not available in
> v0.1 — the maintenance API is a 410 stub and the capability is tracked
> for post-v1.0 in [`docs/project/ROADMAP.md`](../project/ROADMAP.md). The
> table below describes the *intended* post-v1.0 behavior; in v0.1 the
> server has no such mode and every component below stays fully live
> unless you intervene manually (see Step 4).

| Feature | Intended behavior (post-v1.0) | v0.1 reality |
|---------|-------------------------------|--------------|
| API | Available (read-only for most operations) | Fully available |
| Agent Heartbeats | Accepted | Accepted |
| State Applications | Queued | Applied immediately; pause your applier manually |
| Scheduled Jobs | Paused | No scheduled-jobs engine exists |
| Events | Collected but not processed | Collected and processed |
| Alerts | Suppressed | Not suppressed; mute externally |
| Backups | Can be triggered manually | Can be triggered manually |

## Appendix: Common Maintenance Tasks

### OS Patching

> **v0.x scope note**: A node cordon/drain command (`kscorectl cluster
> drain|undrain`) is not available in v0.1 — membership is self-managed
> (members self-register via etcd on start), so there is no scheduler to
> cordon. The closest real operations are to stop the node's `kscore-server`
> for the patch window — it rejoins on restart — or, to permanently evict a
> member, `kscore-cluster remove <member-id>`. A first-class drain workflow
> is tracked in [`docs/project/ROADMAP.md`](../project/ROADMAP.md).

```bash
# Rolling OS update
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Updating $node..."

  # Take the node out of service for the patch window by stopping its
  # server; it re-registers via etcd on restart.
  ssh $node "sudo systemctl stop kscore-server"

  # Update OS
  ssh $node "sudo apt update && sudo apt upgrade -y"

  # Reboot if needed
  ssh $node "sudo reboot" || true

  # Wait for node to come back
  sleep 60
  until ssh $node "echo 'ready'"; do sleep 10; done

  # Bring the node back into service.
  ssh $node "sudo systemctl start kscore-server"

  # Wait for healthy
  sleep 30
  kscore-cluster status
done
```

### Certificate Renewal

See [certificate-rotation.md](certificate-rotation.md)

### Storage Maintenance

```bash
# Check disk usage
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "df -h /var/lib/kscore"
done
```

> **v0.x scope note**: A built-in data-retention sweep
> (`kscorectl maintenance cleanup --older-than`) is not available in v0.1
> — it is post-v1.0, tracked in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). In the meantime,
> prune old data manually (e.g. expire stale backup artifacts under your
> backup destination, or rotate logs) appropriate to your retention policy.
