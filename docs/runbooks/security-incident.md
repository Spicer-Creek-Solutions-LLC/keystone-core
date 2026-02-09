# Runbook: Security Incident Response

## Overview

This runbook covers the response to security incidents affecting the Keystone Core infrastructure.

## Prerequisites

- [ ] Incident response team contact list
- [ ] Access to all control plane nodes
- [ ] Access to audit logs
- [ ] Backup decryption keys (for forensics)
- [ ] Isolated analysis environment

## Trigger Conditions

- Unauthorized access detected
- Credential compromise suspected
- Unusual agent activity
- Policy violations indicating attack
- External security notification
- Anomalous API activity

## Severity Classification

| Severity | Description | Response Time | Examples |
|----------|-------------|---------------|----------|
| P1 - Critical | Active breach, data exfiltration | Immediate | Unauthorized admin access, mass agent compromise |
| P2 - High | Confirmed compromise, no active threat | 1 hour | Leaked credentials, unauthorized agent registration |
| P3 - Medium | Suspicious activity, unconfirmed | 4 hours | Failed auth spike, policy violations |
| P4 - Low | Security improvement needed | 24 hours | Audit finding, vulnerability scan result |

## Procedure

### Phase 1: Initial Response (0-15 minutes)

#### Step 1.1: Assess and Document

```bash
# Document initial observations
cat > /tmp/incident-$(date +%Y%m%d-%H%M%S).txt << EOF
Incident Start: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
Reporter: [Name]
Initial Observations:
- [Description]
Detection Method: [How was it detected]
Affected Systems: [List]
EOF
```

#### Step 1.2: Preserve Evidence

```bash
# Capture current state before any changes
mkdir -p /secure/incident-$(date +%Y%m%d)/evidence

# Capture running processes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "ps auxf" > /secure/incident-$(date +%Y%m%d)/evidence/${node}-processes.txt
done

# Capture network connections
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "ss -tuanp" > /secure/incident-$(date +%Y%m%d)/evidence/${node}-connections.txt
done

# Capture recent logs
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "journalctl -u kscore-server --since '24 hours ago'" > /secure/incident-$(date +%Y%m%d)/evidence/${node}-logs.txt
done

# Export audit logs
kscorectl audit export \
  --since "$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)" \
  --output /secure/incident-$(date +%Y%m%d)/evidence/audit-logs.json
```

#### Step 1.3: Notify Incident Team

```bash
# Use your organization's notification system
# Example: PagerDuty
curl -X POST https://events.pagerduty.com/v2/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "routing_key": "YOUR_ROUTING_KEY",
    "event_action": "trigger",
    "payload": {
      "summary": "Security incident - Keystone Core",
      "severity": "critical",
      "source": "kscore-security"
    }
  }'
```

### Phase 2: Containment (15-60 minutes)

#### Step 2.1: Isolate Compromised Components

**If credential compromise suspected**:

```bash
# Revoke all API keys
kscorectl auth revoke-all --type api-key --confirm

# Force re-authentication for all sessions
kscorectl auth sessions invalidate --all

# Rotate control plane JWT signing key
kscorectl auth rotate-signing-key --immediate
```

**If agent compromise suspected**:

```bash
# Identify suspicious agents
kscorectl agent list --suspicious

# Quarantine specific agents
kscorectl agent quarantine agent-123 --reason "Security incident investigation"

# Block agent network access (if needed)
for agent_ip in $(kscorectl agent show agent-123 --format json | jq -r '.ip'); do
  iptables -A INPUT -s $agent_ip -j DROP
done
```

**If control plane compromise suspected**:

```bash
# Isolate affected node from cluster
ssh ks-server-compromised "systemctl stop kscore-server"

# Remove from cluster
kscorectl cluster member remove ks-server-compromised

# Block network access
ssh ks-server-compromised "iptables -P INPUT DROP; iptables -P OUTPUT DROP; iptables -A INPUT -p tcp --dport 22 -j ACCEPT"
```

#### Step 2.2: Preserve Forensic Data

```bash
# Create disk image of compromised node
ssh ks-server-compromised "dd if=/dev/sda | gzip" | \
  cat > /secure/incident-$(date +%Y%m%d)/disk-image.gz

# Export memory dump (if possible)
ssh ks-server-compromised "
  cat /proc/kcore > /tmp/memory.dump
  gzip /tmp/memory.dump
"
scp ks-server-compromised:/tmp/memory.dump.gz /secure/incident-$(date +%Y%m%d)/

# Capture container state (if containerized)
kubectl exec -n kscore kscore-server-0 -- tar czf - /var/lib/keystone-core > /secure/incident-$(date +%Y%m%d)/container-data.tar.gz
```

### Phase 3: Investigation (1-4 hours)

#### Step 3.1: Analyze Audit Logs

```bash
# Search for unauthorized actions
kscorectl audit search \
  --type "auth.*" \
  --status "failed" \
  --since "7d" \
  --output /tmp/failed-auth.json

# Search for privilege escalation
kscorectl audit search \
  --type "admin.*" \
  --since "7d" \
  --output /tmp/admin-actions.json

# Search for unusual agent activity
kscorectl audit search \
  --type "agent.*" \
  --agent "agent-123" \
  --since "7d" \
  --output /tmp/agent-activity.json

# Identify anomalies
kscorectl audit analyze \
  --input /tmp/*.json \
  --baseline "30d" \
  --output anomalies.json
```

#### Step 3.2: Analyze Authentication Events

```bash
# Find authentication anomalies
kscorectl audit search --type "auth.login" --since "7d" | \
  jq -r '[.timestamp, .user, .ip, .status] | @tsv' | \
  sort | uniq -c | sort -rn | head -20

# Check for brute force attempts
kscorectl audit search --type "auth.login" --status "failed" --since "24h" | \
  jq -r '.ip' | sort | uniq -c | sort -rn | head -10

# Check for impossible travel
kscorectl audit search --type "auth.login" --user "admin" --since "24h" | \
  jq -r '[.timestamp, .ip, .location] | @tsv'
```

#### Step 3.3: Analyze Command Execution

```bash
# List all commands executed on compromised agent
kscorectl audit search \
  --type "exec.*" \
  --agent "agent-123" \
  --since "7d" \
  --output commands.json

# Check for suspicious commands
cat commands.json | jq -r '.command' | grep -E "(curl|wget|nc|bash -i|python -c|perl -e)"

# Check for data exfiltration patterns
cat commands.json | jq -r '.command' | grep -E "(tar|zip|scp|rsync|curl.*POST)"
```

#### Step 3.4: Network Analysis

```bash
# Analyze connection patterns
cat /secure/incident-*/evidence/*-connections.txt | \
  grep ESTABLISHED | \
  awk '{print $5}' | \
  sort | uniq -c | sort -rn

# Check for C2 indicators
cat /secure/incident-*/evidence/*-connections.txt | \
  grep -E ":443|:8443|:4444|:1337"

# DNS query analysis (if logged)
grep -h "query" /var/log/dns/*.log | \
  awk '{print $NF}' | sort | uniq -c | sort -rn | head -50
```

### Phase 4: Eradication (30-120 minutes)

#### Step 4.1: Remove Malicious Access

```bash
# Remove compromised API keys
kscorectl auth key revoke compromised-key-id

# Remove unauthorized users
kscorectl user delete malicious-user --force

# Remove unauthorized agents
kscorectl agent delete compromised-agent --force

# Remove unauthorized policies
kscorectl policy delete malicious-policy
```

#### Step 4.2: Rotate All Credentials

```bash
# Rotate CA certificates
kscorectl identity ca rotate --force

# Regenerate all agent certificates
kscorectl agent certificates regenerate --all

# Rotate database credentials
kscorectl db rotate-credentials

# Rotate NATS credentials
kscorectl nats rotate-credentials

# Rotate encryption keys
kscorectl secrets rotate-keys
```

#### Step 4.3: Rebuild Compromised Systems

```bash
# For compromised control plane node
# 1. Provision new server
# 2. Join to cluster
kscorectl cluster join \
  --server https://ks-server-1:8080 \
  --token "$CLUSTER_JOIN_TOKEN"  # From cluster config or bootstrap

# For compromised agents
# 1. Reinstall agent from trusted source
curl -sSL https://install.keystone-core.io | sudo bash

# 2. Re-register with new identity
kscore-agent register \
  --server https://control-plane:8080 \
  --token $(kscorectl agent token)
```

### Phase 5: Recovery (1-4 hours)

#### Step 5.1: Restore Normal Operations

```bash
# Remove quarantine from verified-clean agents
kscorectl agent unquarantine agent-456

# Re-enable blocked IPs (after verification)
iptables -D INPUT -s $agent_ip -j DROP

# Restore API access
kscorectl config set server.maintenance_mode false
systemctl restart kscore-server

# Issue new API keys to legitimate users
kscorectl auth key create --name "restored-key" --ttl 30d
```

#### Step 5.2: Verify System Integrity

```bash
# Run integrity checks
kscorectl verify --all

# Check cluster health
kscorectl cluster health --verbose

# Verify all agents
kscorectl agent verify --all

# Run security scan
kscorectl security scan --full
```

### Phase 6: Post-Incident (24-72 hours)

#### Step 6.1: Documentation

```bash
# Generate incident timeline
kscorectl audit timeline \
  --from "incident-start-time" \
  --to "incident-end-time" \
  --output incident-timeline.html

# Export all evidence
tar czf incident-evidence-$(date +%Y%m%d).tar.gz /secure/incident-*/
```

#### Step 6.2: Notification

Based on incident severity and regulatory requirements:

- [ ] Notify affected users
- [ ] Notify management
- [ ] Notify legal/compliance (if data breach)
- [ ] Notify regulators (if required)
- [ ] Update status page

## Verification Checklist

- [ ] All compromised credentials rotated
- [ ] Malicious access removed
- [ ] Compromised systems rebuilt or verified clean
- [ ] Normal operations restored
- [ ] No ongoing malicious activity
- [ ] Evidence preserved for analysis

## Rollback

If containment measures cause operational issues:

```bash
# Restore from known-good backup (see backup-restore.md)
# Only after confirming backup is not compromised

kscorectl backup restore \
  --backup /backup/pre-incident.tar.gz \
  --verify-before-restore
```

## Post-Procedure

1. [ ] Complete incident report
2. [ ] Conduct post-mortem meeting
3. [ ] Update detection rules
4. [ ] Implement preventive measures
5. [ ] Update runbook with lessons learned
6. [ ] Security awareness training if needed

## Appendix: Indicators of Compromise (IOC)

### Suspicious Audit Log Patterns

```bash
# Mass agent deletion
kscorectl audit search --type "agent.delete" --count-by hour

# Bulk credential access
kscorectl audit search --type "secret.read" --count-by hour

# Policy changes at unusual times
kscorectl audit search --type "policy.*" --hour "0-6"

# Commands from unexpected IPs
kscorectl audit search --type "exec.*" | jq 'select(.ip | test("^10\\.") | not)'
```

### File System IOCs

```bash
# Check for unauthorized binaries
find /usr/local/bin -mtime -1 -type f

# Check for unauthorized cron jobs
cat /etc/crontab /etc/cron.d/* /var/spool/cron/*

# Check for SSH backdoors
cat /root/.ssh/authorized_keys
grep -r "ssh-" /home/*/.ssh/

# Check for modified system files
rpm -Va 2>/dev/null || dpkg --verify
```

### Network IOCs

```bash
# Unusual outbound connections
ss -tuanp | grep ESTABLISHED | grep -v -E ":(22|80|443|4222|8080|9090) "

# DNS tunneling indicators
tcpdump -i any -n port 53 | grep -E "TXT|NULL"

# Encrypted traffic to unusual ports
ss -tuanp | grep ESTABLISHED | grep -E ":(4444|1337|31337)"
```

## Appendix: Emergency Contacts

| Role | Contact | When to Notify |
|------|---------|----------------|
| Security Lead | <security@example.com> | All incidents |
| On-call Engineer | <pager@example.com> | P1/P2 incidents |
| Legal | <legal@example.com> | Data breaches |
| Management | <ciso@example.com> | P1 incidents |

## Appendix: Evidence Retention

| Evidence Type | Retention Period | Storage Location |
|---------------|------------------|------------------|
| Audit logs | 7 years | S3/archival storage |
| Disk images | 90 days | Secure offline storage |
| Memory dumps | 90 days | Secure offline storage |
| Network captures | 30 days | Secure storage |
| Incident reports | Indefinite | Document management |
