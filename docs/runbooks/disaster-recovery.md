# Runbook: Disaster Recovery

## Overview

This runbook covers disaster recovery procedures for complete or partial Keystone Core cluster loss.

## Prerequisites

- [ ] Access to backup storage (S3, GCS, Azure, local)
- [ ] Backup decryption keys/identities
- [ ] New infrastructure provisioned (or recovery site)
- [ ] Network connectivity to backup location
- [ ] DNS update access (if needed)

## Trigger Conditions

- Complete cluster loss (all control plane nodes down)
- Datacenter outage
- Unrecoverable data corruption
- Complete infrastructure failure

## Severity Assessment

| Scenario | RTO Target | RPO Target | Priority |
|----------|------------|------------|----------|
| Single node failure | 15 min | 0 | P2 |
| Multi-node failure | 30 min | 0 | P1 |
| Complete cluster loss | 4 hours | Last backup | P0 |
| Datacenter outage | 4 hours | Last backup | P0 |

## Procedure

### Phase 1: Assessment (15 minutes)

#### Step 1.1: Confirm Disaster

```bash
# Attempt to reach all control plane nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ping -c 3 $node || echo "$node unreachable"
  curl -sk https://$node:8080/health/live || echo "$node API unreachable"
done

# Check if any node is responding
kscorectl cluster health || echo "Cluster unreachable"
```

#### Step 1.2: Identify Available Backups

```bash
# List backups from primary location
kscore-backup list --dest s3://keystone-backups/

# List backups from DR location
kscore-backup list --dest s3://keystone-backups-dr/

# Identify most recent valid backup
# Note: timestamp, size, components
```

#### Step 1.3: Document Current State

```
Disaster Assessment:
- Date/Time: ____________
- Affected nodes: ____________
- Last known good state: ____________
- Latest backup: ____________
- Recovery location: ____________
```

### Phase 2: Infrastructure Preparation (30-60 minutes)

#### Step 2.1: Provision Recovery Infrastructure

```bash
# Option A: Use pre-provisioned DR site
# Verify DR infrastructure is ready
ssh dr-server-1 "hostname && systemctl status"

# Option B: Provision new infrastructure
# Use Terraform/CloudFormation/etc.
cd infrastructure/
terraform apply -var="environment=recovery"
```

#### Step 2.2: Verify Network Connectivity

```bash
# Test connectivity between recovery nodes
for node in dr-server-1 dr-server-2 dr-server-3; do
  ssh $node "ping -c 1 dr-server-1 && ping -c 1 dr-server-2 && ping -c 1 dr-server-3"
done

# Test connectivity to backup storage
aws s3 ls s3://keystone-backups-dr/ --region us-west-2
```

#### Step 2.3: Prepare Recovery Environment

```bash
# Copy bootstrap binary to recovery nodes
for node in dr-server-1 dr-server-2 dr-server-3; do
  scp kscore-bootstrap $node:/usr/local/bin/
done

# Copy decryption identity
scp /secure/backup-identity.txt dr-server-1:/tmp/
```

### Phase 3: Restore from Backup (60-120 minutes)

#### Step 3.1: Download and Verify Backup

```bash
# SSH to first recovery node
ssh dr-server-1

# Download latest backup
aws s3 cp s3://keystone-backups-dr/backup-2024-01-15T02-00-00.tar.gz /tmp/

# Verify backup integrity
kscore-backup verify /tmp/backup-2024-01-15T02-00-00.tar.gz

# Expected: "Backup verification passed"
```

#### Step 3.2: Restore First Node

```bash
# Restore from backup
kscore-bootstrap restore \
  --backup /tmp/backup-2024-01-15T02-00-00.tar.gz \
  --decrypt-identity /tmp/backup-identity.txt \
  --node-ip $(hostname -I | awk '{print $1}')

# Monitor restore progress
# This will restore: database, config, certificates, JetStream data
```

#### Step 3.3: Verify First Node

```bash
# Check server status
systemctl status kscore-server

# Check API health
curl -k https://localhost:8080/health/ready

# Check cluster status (single node)
kscorectl cluster health
```

#### Step 3.4: Join Additional Nodes

```bash
# Get join token
JOIN_TOKEN=$(kscorectl cluster token)

# On each additional node
ssh dr-server-2
kscore-bootstrap import \
  --join https://dr-server-1:8080 \
  --token $JOIN_TOKEN

ssh dr-server-3
kscore-bootstrap import \
  --join https://dr-server-1:8080 \
  --token $JOIN_TOKEN
```

#### Step 3.5: Verify Cluster Formation

```bash
# Verify all nodes are healthy
kscorectl cluster members

# Verify quorum
kscorectl cluster health

# Verify leader election
kscorectl cluster leader
```

### Phase 4: Agent Recovery (30-60 minutes)

#### Step 4.1: Update DNS (if needed)

```bash
# Update DNS to point to new control plane
# This depends on your DNS provider

# Example: AWS Route53
aws route53 change-resource-record-sets \
  --hosted-zone-id ZXXXXX \
  --change-batch file://dns-update.json
```

#### Step 4.2: Wait for Agent Reconnection

```bash
# Agents should automatically reconnect if DNS is updated
# Monitor agent reconnections
watch -n 10 'kscorectl agent list --status | wc -l'

# Check for agents that haven't reconnected
kscorectl agent list --status | grep offline
```

#### Step 4.3: Manual Agent Update (if needed)

```bash
# If agents can't reach new control plane via DNS,
# update agent configurations
# On each agent node:
sed -i 's/old-server/new-server/g' /etc/kscore/agent.yaml
systemctl restart kscore-agent
```

### Phase 5: Validation (30 minutes)

#### Step 5.1: Verify Cluster Health

```bash
# Full health check
kscorectl cluster health --verbose

# Verify all agents connected
kscorectl agent list --status

# Verify state data
kscorectl state list
```

#### Step 5.2: Run Integration Tests

```bash
# Run smoke tests
kscore-test smoke

# Run integration tests
kscore-test integration --suite recovery
```

#### Step 5.3: Verify Critical Functionality

```bash
# Test remote execution
kscorectl exec "hostname" --target "role=webserver" --limit 1

# Test state application
kscorectl state check /etc/kscore/states/test.yaml

# Test event system
kscorectl events list --limit 10
```

## Verification Checklist

- [ ] All control plane nodes healthy
- [ ] Cluster quorum established
- [ ] Leader elected
- [ ] Database restored and accessible
- [ ] NATS cluster formed
- [ ] All agents reconnected
- [ ] API responding correctly
- [ ] State data intact
- [ ] Remote execution working
- [ ] Events flowing

## Rollback

If recovery fails:

```bash
# Try older backup
kscore-bootstrap restore --backup /tmp/backup-older.tar.gz

# Or restore to previous recovery point
# Contact support for assistance
```

## Post-Procedure

### Immediate (within 1 hour)

1. [ ] Update status page
2. [ ] Notify stakeholders of recovery
3. [ ] Document recovery details:
   - Recovery start time
   - Recovery completion time
   - Data loss (if any)
   - Issues encountered

### Within 24 hours

4. [ ] Root cause analysis
5. [ ] Update monitoring for new infrastructure
6. [ ] Reconfigure backup jobs
7. [ ] Test backup/restore cycle

### Within 1 week

8. [ ] Post-mortem meeting
9. [ ] Update DR procedures based on learnings
10. [ ] Plan infrastructure improvements
11. [ ] Update runbook if needed

## Appendix: Recovery Time Estimates

| Component | Estimated Time |
|-----------|----------------|
| Infrastructure provisioning | 15-60 min |
| Backup download | 5-30 min |
| Database restore | 10-60 min |
| Certificate restore | 2 min |
| NATS data restore | 10-30 min |
| Cluster formation | 10 min |
| Agent reconnection | 5-30 min |
| Validation | 30 min |
| **Total** | **1.5-4 hours** |

## Appendix: Backup Locations

| Location | Purpose | Retention |
|----------|---------|-----------|
| s3://keystone-backups/ | Primary backups | 30 days |
| s3://keystone-backups-dr/ | DR backups (cross-region) | 30 days |
| /backup/local/ | Local fast restore | 7 days |
