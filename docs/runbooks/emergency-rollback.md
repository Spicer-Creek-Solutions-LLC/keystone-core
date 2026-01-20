# Runbook: Emergency Rollback

## Overview

This runbook covers emergency rollback procedures when an upgrade causes critical issues.

## Prerequisites

- [ ] Access to Keystone Core control plane
- [ ] Knowledge of previous stable version
- [ ] Backup from before upgrade (recommended)

## Trigger Conditions

Execute this runbook when:

- Error rate exceeds 5% after upgrade
- P99 latency exceeds 2x baseline
- Agent disconnection rate exceeds 10%
- Critical functionality is broken
- Security vulnerability discovered in new version

## Severity Assessment

| Indicator | Warning | Critical |
|-----------|---------|----------|
| Error Rate | > 2% | > 5% |
| P99 Latency | > 1.5x baseline | > 2x baseline |
| Agent Disconnections | > 5% | > 10% |
| API Availability | < 99.5% | < 99% |

## Procedure

### Step 1: Assess Situation (2 minutes)

```bash
# Check current upgrade status
kscorectl upgrade status

# Check cluster health
kscorectl cluster health

# Check error rates
curl -s http://localhost:9090/api/v1/query?query=rate(kscore_api_errors_total[5m])

# Check agent status
kscorectl agent list --status | grep -c offline
```

### Step 2: Decision Point

**If automatic rollback is in progress:**
```bash
# Monitor rollback progress
kscorectl upgrade status --watch

# Skip to Step 5 (Verification)
```

**If manual rollback is needed:**
```bash
# Continue to Step 3
```

### Step 3: Initiate Rollback (5 minutes)

```bash
# Initiate rollback
kscorectl upgrade rollback

# Or rollback to specific version
kscorectl upgrade rollback --target 1.5.0

# Monitor rollback progress
kscorectl upgrade status --watch
```

### Step 4: Monitor Rollback (10-30 minutes)

```bash
# Watch rollback progress
watch -n 5 'kscorectl upgrade status'

# Monitor error rates
watch -n 10 'curl -s http://localhost:9090/api/v1/query?query=rate(kscore_api_errors_total[1m]) | jq'

# Monitor agent reconnections
watch -n 10 'kscorectl agent list --status | grep -c online'
```

### Step 5: Verification (5 minutes)

```bash
# Verify rollback completed
kscorectl upgrade status
# Expected: Status: rolled_back

# Verify cluster health
kscorectl cluster health
# Expected: Status: healthy

# Verify all agents reconnected
kscorectl agent list --status | grep -c offline
# Expected: 0

# Verify API functionality
curl -k https://localhost:8080/health/ready
# Expected: {"status":"ready"}

# Run smoke tests
kscore-test smoke
```

### Step 6: Stabilization (15 minutes)

```bash
# Monitor for 15 minutes after rollback
# Watch error rates
# Watch latency
# Watch agent status

# If issues persist, escalate to disaster recovery
```

## Verification Checklist

- [ ] Rollback status shows completed
- [ ] Cluster health is healthy
- [ ] All agents are online
- [ ] Error rate < 0.1%
- [ ] P99 latency at baseline
- [ ] API responds correctly
- [ ] Smoke tests pass

## Rollback of Rollback

If rollback causes additional issues:

```bash
# Check if previous version backup exists
kscore-cluster-backup list --dest /backup/

# Restore from pre-upgrade backup
kscore-bootstrap restore --backup /backup/pre-upgrade.tar.gz

# Or attempt forward fix
# Contact engineering for assistance
```

## Post-Procedure

### Immediate (within 1 hour)

1. [ ] Update status page/incident ticket
2. [ ] Notify stakeholders of rollback completion
3. [ ] Collect diagnostic information:
   ```bash
   kscorectl diagnostics collect --output /tmp/diagnostics-$(date +%Y%m%d).tar.gz
   ```

### Within 24 hours

4. [ ] Create incident report
5. [ ] Schedule post-mortem
6. [ ] File bug report with diagnostics
7. [ ] Update runbook if needed

### Before Next Upgrade Attempt

8. [ ] Root cause identified and fixed
9. [ ] Fix validated in staging
10. [ ] Rollback procedure reviewed
11. [ ] Stakeholders notified of retry plan

## Appendix: Quick Reference

### Rollback Commands

```bash
# Check if rollback is possible
kscorectl upgrade rollback --dry-run

# Initiate rollback
kscorectl upgrade rollback

# Rollback specific components
kscorectl upgrade rollback --components server

# Force rollback (skip health checks)
kscorectl upgrade rollback --force

# Cancel in-progress rollback
kscorectl upgrade rollback --cancel
```

### Key Metrics to Monitor

```promql
# Error rate
rate(kscore_api_errors_total[5m])

# Request latency P99
histogram_quantile(0.99, rate(kscore_api_request_duration_seconds_bucket[5m]))

# Agent connectivity
kscore_agents_connected

# Rollback status
kscore_upgrade_rollback_status
```

### Emergency Contacts

| Role | Contact |
|------|---------|
| On-Call Engineer | PagerDuty |
| Platform Team Lead | [Contact Info] |
| Keystone Support | support@keystone.io |
