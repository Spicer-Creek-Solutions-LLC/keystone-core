# Runbook: Upgrade Cluster

## Overview

This runbook covers upgrading a Keystone Core cluster to a new version.

## Prerequisites

- [ ] Target version tested in staging
- [ ] Upgrade path validated (version compatibility)
- [ ] Backup created within last 24 hours
- [ ] Maintenance window scheduled
- [ ] Rollback plan reviewed
- [ ] Release notes reviewed

## Trigger Conditions

- New version available with required features
- Security patch required
- Bug fix needed
- End of support approaching

## Pre-Upgrade Checklist

```bash
# 1. Check current version
kscorectl version

# 2. Check available upgrades
kscorectl upgrade check

# 3. Verify compatibility
kscorectl upgrade check --target 0.2.0

# 4. Review release notes
# Visit: https://docs.keystone.io/releases/0.2.0

# 5. Verify cluster health
kscorectl cluster health

# 6. Create backup
kscore-cluster-backup create \
  --dest /backup/pre-upgrade-0.2.0 \
  --label "pre-upgrade-0.2.0"
```

## Procedure

### Step 1: Pre-Flight Checks

```bash
# Verify all nodes healthy
kscorectl cluster health --verbose

# Verify all agents connected
TOTAL_AGENTS=$(kscorectl agents list --status online -o json | jq length)
echo "Total agents online: $TOTAL_AGENTS"

# Verify no recent state failures (check logs)
journalctl -u kscore-server --since "1 hour ago" | grep -i "state apply\|failed" | tail -10

# Record baseline metrics
curl -s http://localhost:9090/api/v1/query?query=rate(kscore_api_errors_total[5m]) > /tmp/baseline-errors.txt
curl -s http://localhost:9090/api/v1/query?query=histogram_quantile(0.99,rate(kscore_api_request_duration_seconds_bucket[5m])) > /tmp/baseline-latency.txt
```

### Step 2: Generate Upgrade Plan

```bash
# Generate and review upgrade plan
kscorectl upgrade plan --target 0.2.0

# Example output:
# Upgrade Plan: 1.5.0 -> 0.2.0
# Strategy: rolling
# Estimated duration: 45 minutes
#
# Pre-upgrade:
#   - Validate prerequisites
#   - Create backup checkpoint
#
# Server upgrade (rolling):
#   1. ks-server-1: drain, upgrade, verify (15 min)
#   2. ks-server-2: drain, upgrade, verify (15 min)
#   3. ks-server-3: drain, upgrade, verify (15 min)
#
# Agent upgrade (batched):
#   - 15 batches of 10 agents (30 min)
#
# Post-upgrade:
#   - Verify cluster health
#   - Run smoke tests
#
# Rollback: Automatic on failure

# Save plan for reference
kscorectl upgrade plan --target 0.2.0 --output /tmp/upgrade-plan.yaml
```

### Step 3: Execute Upgrade

```bash
# Start upgrade
kscorectl upgrade execute --target 0.2.0

# Or with specific options
kscorectl upgrade execute --target 0.2.0 \
  --strategy rolling \
  --max-unavailable 1 \
  --backup-before \
  --auto-rollback
```

### Step 4: Monitor Progress

```bash
# Watch upgrade progress
kscorectl upgrade status --watch

# In another terminal, monitor metrics
watch -n 10 'curl -s http://localhost:9090/api/v1/query?query=rate(kscore_api_errors_total[1m]) | jq ".data.result[0].value[1]"'

# Monitor cluster health
watch -n 10 'kscorectl cluster health'

# Monitor agent count
watch -n 10 'kscorectl agents list --status online -o json | jq length'
```

### Step 5: Verification

```bash
# Verify upgrade completed
kscorectl upgrade status
# Expected: Status: completed

# Verify version
kscorectl version
# Expected: Version: 0.2.0

# Verify cluster health
kscorectl cluster health
# Expected: Status: healthy

# Verify all agents upgraded
kscorectl upgrade agents --report | grep -c "0.2.0"

# Compare metrics to baseline
curl -s http://localhost:9090/api/v1/query?query=rate(kscore_api_errors_total[5m])
# Should be similar to baseline

# Run smoke tests
kscore-test smoke
```

### Step 6: Agent Upgrade (if separate)

```bash
# If agents are upgraded separately
kscorectl upgrade agents --target 0.2.0

# Monitor agent upgrade
kscorectl upgrade agents --status

# Verify all agents upgraded
kscorectl agents list --show-compatibility | grep -v "0.2.0"
# Should be empty
```

## Verification Checklist

- [ ] Server version is 0.2.0
- [ ] All cluster nodes healthy
- [ ] All agents on 0.2.0
- [ ] Error rate at baseline
- [ ] Latency at baseline
- [ ] All smoke tests pass
- [ ] No new alerts

## Rollback Procedure

If issues are detected:

```bash
# Automatic rollback is triggered if:
# - Error rate > 5%
# - Node health check fails
# - Cluster loses quorum

# Manual rollback
kscorectl upgrade rollback

# Or rollback to specific version
kscorectl upgrade rollback --target 1.5.0

# Monitor rollback
kscorectl upgrade status --watch
```

## Post-Procedure

### Immediate

1. [ ] Update status page
2. [ ] Notify stakeholders
3. [ ] Close change ticket

### Within 24 hours

1. [ ] Review post-upgrade metrics
2. [ ] Address any new warnings
3. [ ] Update documentation
4. [ ] Clean up old version artifacts

### If Rollback Occurred

1. [ ] Collect diagnostics
2. [ ] File issue with details
3. [ ] Schedule post-mortem
4. [ ] Plan retry after fix

## Appendix: Upgrade Strategies

### Rolling Upgrade (Default)

```bash
kscorectl upgrade execute --target 0.2.0 --strategy rolling

# Options:
# --max-unavailable 1     # Max nodes down at once
# --drain-timeout 5m      # Time to drain workloads
# --node-delay 30s        # Delay between nodes
```

Best for: Most upgrades, minimal disruption

### Canary Upgrade

```bash
kscorectl upgrade execute --target 0.2.0 --strategy canary

# Options:
# --canary-percentage 10  # Initial percentage
# --canary-increment 20   # Step increase
# --canary-interval 10m   # Time between steps
```

Best for: Major version upgrades, new features

### Blue-Green Upgrade

```bash
kscorectl upgrade execute --target 0.2.0 --strategy blue-green

# Options:
# --keep-old              # Keep old version for quick rollback
```

Best for: When instant rollback is critical

## Appendix: Version Compatibility

Check upgrade path compatibility:

```bash
# Direct upgrade possible?
kscorectl upgrade check --from 1.4.0 --to 0.2.0

# If not, find required path
kscorectl upgrade path --from 1.4.0 --to 0.2.0
# Output: 1.4.0 -> 1.5.0 -> 0.2.0
```

## Appendix: Troubleshooting

### Upgrade Stuck

```bash
# Check upgrade status
kscorectl upgrade status --verbose

# Check node status
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  ssh $node "sudo systemctl status kscore-server"
done

# Force proceed (use with caution)
kscorectl upgrade resume --force
```

### Health Check Fails

```bash
# Check what's failing
kscorectl cluster health --verbose

# Check specific node via SSH
ssh ks-server-1 "kscorectl cluster health"

# Override health check (emergency only)
kscorectl upgrade resume --skip-health-check
```

### Agent Upgrade Fails

```bash
# Check agent logs
ssh agent-node "journalctl -u kscore-agent -n 100"

# Retry specific agent
kscorectl upgrade agents --retry agent-web-1

# Skip problematic agent
kscorectl upgrade agents --skip agent-web-1
```
