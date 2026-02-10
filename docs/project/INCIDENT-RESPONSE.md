# Security Incident Response Plan

This document defines the procedures for detecting, responding to, and recovering from security incidents affecting Keystone Core deployments.

## Incident Classification

### Severity Levels

| Level | Name | Description | Response Time |
|-------|------|-------------|---------------|
| **P1** | Critical | Active exploitation, data breach, complete system compromise | Immediate (<15 min) |
| **P2** | High | Attempted exploitation, privilege escalation, partial compromise | <1 hour |
| **P3** | Medium | Suspicious activity, policy violations, vulnerability discovered | <4 hours |
| **P4** | Low | Minor security issues, failed attacks, informational | <24 hours |

### Incident Types

| Type | Examples | Typical Severity |
|------|----------|------------------|
| **Unauthorized Access** | Credential theft, session hijacking, bypass | P1-P2 |
| **Data Breach** | Exfiltration, exposure, unauthorized disclosure | P1 |
| **Malware/Compromise** | Agent compromise, control plane infection | P1 |
| **Denial of Service** | Resource exhaustion, flood attacks | P2-P3 |
| **Insider Threat** | Malicious admin, policy violation | P1-P2 |
| **Vulnerability** | Zero-day, CVE affecting components | P2-P3 |
| **Supply Chain** | Compromised dependency, malicious module | P1 |

## Roles and Responsibilities

### Incident Response Team

| Role | Responsibilities |
|------|------------------|
| **Incident Commander (IC)** | Overall incident coordination, decision-making, external communication |
| **Security Lead** | Technical investigation, forensics, containment strategies |
| **Operations Lead** | System access, containment actions, recovery execution |
| **Communications Lead** | Internal/external messaging, customer notification |
| **Scribe** | Timeline documentation, evidence logging |

### Contact List

Maintain an up-to-date contact list including:

- Incident response team members (primary and backup)
- Security team leadership
- Legal counsel
- PR/Communications
- Customer success (for customer-facing incidents)
- External security consultants/IR firms

## Response Phases

### Phase 1: Detection and Triage (0-15 minutes)

**Objective**: Confirm incident and assess severity.

1. **Initial Alert**
   - Receive alert from monitoring, user report, or discovery
   - Document initial information in incident ticket
   - Timestamp all observations

2. **Triage**
   - Confirm this is a security incident (not false positive)
   - Assess scope: What systems/data are affected?
   - Classify severity level (P1-P4)
   - Identify incident type

3. **Activation**
   - For P1/P2: Activate incident response team immediately
   - For P3/P4: Assign to on-call security engineer
   - Create incident channel (Slack/Teams) for coordination
   - Begin incident timeline

**Triage Checklist**:

```
[ ] Alert validated as security incident
[ ] Severity level assigned: P__
[ ] Incident type identified
[ ] Affected systems documented
[ ] Incident Commander assigned
[ ] Response team notified
[ ] Incident channel created
```

### Phase 2: Containment (15 min - 2 hours)

**Objective**: Stop the bleeding, prevent further damage.

#### Short-Term Containment

1. **Isolate Affected Systems**

   ```bash
   # Quarantine compromised agent
   kscorectl agents quarantine <agent-id>

   # Block network access if needed
   kscorectl exec run "iptables -A INPUT -j DROP" --target "<agent-id>"
   ```

2. **Preserve Evidence**

   ```bash
   # Capture forensic data before changes
   kscorectl exec run "tar -czf /tmp/forensics-$(date +%s).tar.gz /var/log /etc" \
     --target "<agent-id>"

   # Retrieve forensic archive to secure location
   kscorectl exec run "scp /tmp/forensics-*.tar.gz evidence@secure-host:/evidence/" \
     --target "<agent-id>"
   ```

3. **Revoke Compromised Credentials**

   ```bash
   # Revoke specific API keys
   kscorectl api-key revoke <key-id>

   # Revoke all API keys
   kscorectl auth revoke-all --force

   # Rotate agent SVID certificate
   kscorectl agents renew-svid <agent-id> --force
   ```

4. **Block Attack Vector**
   - Update firewall rules
   - Disable compromised accounts
   - Block malicious IPs
   - Disable vulnerable features

#### Long-Term Containment

1. **System Hardening**
   - Apply emergency patches
   - Update security policies
   - Enhance monitoring

2. **Scope Assessment**
   - Identify all affected systems
   - Check for lateral movement
   - Review audit logs for timeline

**Containment Checklist**:

```
[ ] Affected systems isolated
[ ] Evidence preserved
[ ] Compromised credentials revoked
[ ] Attack vector blocked
[ ] Scope fully identified
[ ] No ongoing active threat
```

### Phase 3: Eradication (2-24 hours)

**Objective**: Remove the threat completely.

1. **Root Cause Analysis**
   - How did the attacker gain access?
   - What vulnerability was exploited?
   - What tools/techniques were used?

2. **Malware Removal**

   ```bash
   # Scan for known malware
   kscorectl exec run "clamscan -r /" --target "<affected-agents>"

   # Remove malicious files
   kscorectl state apply malware-remediation.yaml --target "<affected-agents>"
   ```

3. **Vulnerability Remediation**
   - Patch vulnerable software
   - Update configurations
   - Strengthen access controls

4. **Credential Reset**

   ```bash
   # Regenerate all agent certificates
   kscorectl agents certificates regenerate --all --force

   # Rotate secrets encryption keys
   kscorectl secrets rotate-keys --force

   # Rotate secrets in Vault
   vault kv put secret/kscore/credentials password="$(openssl rand -base64 32)"

   # Force password resets via your identity provider (IdP)
   # Keystone Core delegates user credential management to the external IdP
   ```

5. **Policy Updates**
   - Update OPA/CEL policies to prevent recurrence
   - Strengthen authentication requirements
   - Add new detection rules

**Eradication Checklist**:

```
[ ] Root cause identified
[ ] All malware/backdoors removed
[ ] Vulnerability patched
[ ] All affected credentials rotated
[ ] Policies updated to prevent recurrence
[ ] Systems verified clean
```

### Phase 4: Recovery (24-72 hours)

**Objective**: Restore normal operations securely.

1. **System Restoration**

   ```bash
   # Restore from known-good backup
   kscorectl backup restore --backup-id <id> --target <agent-id>

   # Verify system integrity
   kscorectl state check --target "<restored-agents>"
   ```

2. **Monitoring Enhancement**
   - Deploy additional monitoring
   - Lower alert thresholds temporarily
   - Add specific IoC detection

3. **Staged Reconnection**
   - Reconnect systems in phases
   - Monitor for signs of reinfection
   - Validate each system before full restoration

4. **Verification**

   ```bash
   # Verify agent integrity
   kscorectl agents verify --all

   # Check for drift
   kscorectl state check baseline.yaml --target "<restored-agents>"

   # Review recent audit logs
   kscorectl audit search --since "7d" --agent "<restored-agent-id>"
   ```

**Recovery Checklist**:

```
[ ] Systems restored from clean state
[ ] Enhanced monitoring in place
[ ] Systems verified healthy
[ ] No signs of reinfection
[ ] Normal operations resumed
[ ] Stakeholders notified of resolution
```

### Phase 5: Post-Incident (1-2 weeks)

**Objective**: Learn from the incident and improve.

1. **Documentation**
   - Complete incident timeline
   - Document all actions taken
   - Compile evidence inventory
   - Calculate impact metrics

2. **Post-Mortem Meeting**
   - Schedule within 5 business days
   - Include all responders
   - Focus on process improvement
   - No blame assignment

3. **Post-Mortem Report**
   Include:
   - Executive summary
   - Timeline of events
   - Root cause analysis
   - Impact assessment
   - Response effectiveness
   - Lessons learned
   - Action items with owners/dates

4. **Process Improvements**
   - Update runbooks based on lessons
   - Improve detection capabilities
   - Enhance automation
   - Update training materials

5. **Follow-Up Actions**
   - Track all action items to completion
   - Verify improvements are effective
   - Update incident response plan

## Specific Incident Playbooks

### Playbook: Compromised Agent

```
1. DETECT
   - Alert: Unusual agent behavior, unauthorized commands
   - Verify: Check agent audit logs, compare baseline

2. CONTAIN
   kscorectl agents quarantine <agent-id>
   kscorectl exec run "iptables -A OUTPUT -j DROP" --target "<agent-id>"

3. INVESTIGATE
   kscorectl audit search --agent <agent-id> --since 7d
   kscorectl exec run "last -a" --target "<agent-id>"
   kscorectl exec run "netstat -tlnp" --target "<agent-id>"

4. ERADICATE
   # Option A: Reimage
   # Option B: Remediate
   kscorectl agents renew-svid <agent-id> --force
   kscorectl state apply hardening.yaml --target "<agent-id>"

5. RECOVER
   kscorectl agents unquarantine <agent-id>
   # Enhanced monitoring for 7 days
```

### Playbook: Compromised Control Plane

```
1. DETECT
   - Alert: Unauthorized API access, config changes
   - Verify: Review audit logs, check for unauthorized users

2. CONTAIN
   # Isolate affected instance
   # Revoke all sessions
   kscorectl auth revoke-all --force
   kscorectl auth sessions invalidate

3. INVESTIGATE
   # Review all recent changes
   kscorectl audit search --type "admin.*" --since "24h"
   kscorectl audit search --type "policy.*" --since "24h"

4. ERADICATE
   # Rotate all credentials
   # Restore from backup if needed
   kscorectl backup restore --backup-id <known-good>

5. RECOVER
   # Re-issue all API keys
   # Re-enroll agents
   # Verify all policies
```

### Playbook: Data Breach

```
1. DETECT
   - Alert: Unusual data access, exfiltration indicators
   - Verify: Review data access logs, network flows

2. CONTAIN
   # Block exfiltration
   # Revoke access
   # Preserve evidence

3. INVESTIGATE
   # Identify scope of exposure
   # Determine data types affected
   # Timeline of access

4. LEGAL/COMPLIANCE
   # Engage legal counsel
   # Determine notification requirements
   # Document for regulators

5. NOTIFY
   # Per legal guidance
   # Customer notification
   # Regulatory notification
```

### Playbook: Supply Chain Attack

```
1. DETECT
   - Alert: Malicious module behavior, signature mismatch
   - Verify: Check module signatures, SumDB transparency log

2. CONTAIN
   # Remove affected module from registry and agents
   kscorectl module verify <module-name>@<version>
   # Manually remove the module version from the registry if compromised

   # Search audit logs for module activity
   kscorectl audit search --type "module.*" --since "30d"

3. INVESTIGATE
   # Analyze module contents
   # Determine scope of compromise
   # Review module source

4. ERADICATE
   # Remove module from all agents
   kscorectl state apply remove-module.yaml --target "module:affected"

   # Audit affected systems
   kscorectl exec run "audit-script.sh" --target "module:affected"

5. RECOVER
   # Deploy patched/alternative module
   # Verify system integrity
```

## Communication Templates

### Internal Notification (P1/P2)

```
SECURITY INCIDENT - [SEVERITY]

Incident ID: INC-XXXX
Time Detected: YYYY-MM-DD HH:MM UTC
Incident Commander: [Name]
Status: [Active/Contained/Resolved]

Summary:
[Brief description of incident]

Impact:
[Systems/data affected]

Current Actions:
[What is being done]

Next Update: [Time]

Incident Channel: #incident-XXXX
```

### Customer Notification

```
Subject: Security Notice - [Brief Description]

Dear [Customer],

We are writing to inform you of a security incident that may affect
your Keystone Core deployment.

What Happened:
[Clear, factual description]

What We're Doing:
[Actions taken and planned]

What You Should Do:
[Customer action items]

Timeline:
[When discovered, when contained, when resolved]

For questions, please contact: security@example.com

Sincerely,
[Security Team]
```

## Evidence Handling

### Evidence Collection

1. **Digital Evidence**
   - System logs (preserve with timestamps)
   - Network captures (pcap files)
   - Memory dumps
   - Disk images
   - Configuration files

2. **Collection Commands**

   ```bash
   # Collect logs
   kscorectl exec run "tar -czf /tmp/logs.tar.gz /var/log" --target "<agent>"

   # Memory dump (if needed)
   kscorectl exec run "dd if=/dev/mem of=/tmp/mem.dump bs=1M" --target "<agent>"

   # Network state
   kscorectl exec run "netstat -tlnp > /tmp/netstat.txt" --target "<agent>"
   ```

3. **Chain of Custody**
   - Document who collected evidence
   - Timestamp all evidence
   - Calculate and record hashes
   - Store in secure, access-controlled location

### Evidence Storage

- Use encrypted storage
- Maintain access logs
- Preserve original evidence (work on copies)
- Retain per legal hold requirements

## Metrics and Reporting

### Incident Metrics

Track and report:

- Mean Time to Detect (MTTD)
- Mean Time to Contain (MTTC)
- Mean Time to Resolve (MTTR)
- Number of incidents by type/severity
- Root cause categories
- Recurrence rate

### Monthly Security Report

Include:

- Incident summary
- Trend analysis
- Key metrics
- Notable events
- Improvement actions
- Upcoming concerns

## Training and Exercises

### Training Requirements

- All engineers: Annual security awareness
- IR team: Quarterly incident response training
- New team members: IR onboarding within 30 days

### Tabletop Exercises

Conduct quarterly tabletop exercises covering:

- Different incident types
- Various severity levels
- Communication scenarios
- Decision-making under pressure

### Live Drills

Conduct semi-annual live drills:

- Simulated incidents in test environment
- Full response activation
- Post-drill review

## Plan Maintenance

- Review this plan quarterly
- Update after each significant incident
- Test procedures during exercises
- Keep contact lists current
- Archive old versions

## References

- [NIST SP 800-61](https://csrc.nist.gov/publications/detail/sp/800-61/rev-2/final) - Computer Security Incident Handling Guide
- [SANS Incident Handler's Handbook](https://www.sans.org/white-papers/33901/)
- [Threat Model](../docs/content/en/docs/concepts/threat-model.md) - System threat model
- [Security Guide](../docs/content/en/docs/operations/security.md) - Security operations
