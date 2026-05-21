# Security Release Process

This document defines the process for handling security vulnerabilities and releasing security patches for Keystone Core.

## Vulnerability Handling Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  1. RECEIVE          2. TRIAGE           3. FIX                     │
│  ───────────         ─────────           ────────                   │
│  Report received     Assess severity     Develop patch              │
│  Acknowledge         Assign CVE          Review and test            │
│  within 24 hours     Set timeline        Prepare advisory           │
│                                                                     │
│  4. RELEASE          5. DISCLOSE         6. POST-RELEASE            │
│  ─────────           ──────────          ─────────────              │
│  Coordinate date     Publish advisory    Monitor adoption           │
│  Release patches     Notify users        Track any issues           │
│  Update docs         Update databases    Retrospective              │
└─────────────────────────────────────────────────────────────────────┘
```

## Receiving Vulnerability Reports

### Reporting Channels

| Channel | Use Case | Response Time |
|---------|----------|---------------|
| <security@keystone-core.io> | General security reports | 24 hours |
| GitHub Security Advisories | Public disclosure ready | 24 hours |
| HackerOne (if applicable) | Bug bounty reports | 24 hours |

### Initial Response

Within 24 hours of receiving a report:

1. **Acknowledge Receipt**

   ```
   Subject: Re: Security Report - [Brief Description]

   Thank you for reporting this security issue to the Keystone Core
   security team. We have received your report and will investigate.

   We aim to provide an initial assessment within 5 business days.
   We will keep you informed of our progress.

   Please do not disclose this issue publicly until we have had an
   opportunity to address it.

   Regards,
   Keystone Core Security Team
   ```

2. **Create Internal Tracking**
   - Create private security issue in tracking system
   - Assign to security team member
   - Set initial priority based on description

3. **Verify Legitimacy**
   - Confirm the report describes a real vulnerability
   - Rule out false positives and misunderstandings
   - Request additional information if needed

## Triage and Assessment

### Severity Classification

Use CVSS 3.1 to assess severity:

| CVSS Score | Severity | Typical Response Time |
|------------|----------|----------------------|
| 9.0 - 10.0 | Critical | 24-48 hours |
| 7.0 - 8.9 | High | 7 days |
| 4.0 - 6.9 | Medium | 30 days |
| 0.1 - 3.9 | Low | Next regular release |

### CVSS Scoring Factors

Consider these factors when scoring:

- **Attack Vector**: Network, Adjacent, Local, Physical
- **Attack Complexity**: Low, High
- **Privileges Required**: None, Low, High
- **User Interaction**: None, Required
- **Scope**: Changed, Unchanged
- **Impact**: Confidentiality, Integrity, Availability

### CVE Assignment

For vulnerabilities that will be publicly disclosed:

1. Request CVE ID from MITRE CNA or GitHub Security Advisories
2. Reserve CVE before fix is developed
3. Include CVE in all communications

### Assessment Report

Document the following:

```markdown
## Vulnerability Assessment

**Report ID**: SEC-YYYY-NNN
**CVE ID**: CVE-YYYY-NNNNN (if assigned)
**Reported By**: [Name/Alias]
**Report Date**: YYYY-MM-DD
**Assessment Date**: YYYY-MM-DD

### Summary
[Brief description of the vulnerability]

### Affected Versions
- v1.2.0 through v1.4.2
- All versions prior to v1.2.0 NOT affected

### CVSS Score
Score: X.X (Severity)
Vector: CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H

### Technical Details
[Detailed technical description]

### Impact
[What an attacker could achieve]

### Exploitation
[Is this being actively exploited?]
[Is there a public exploit?]

### Recommended Fix
[Approach for fixing the vulnerability]

### Timeline
- Target Fix Date: YYYY-MM-DD
- Target Disclosure Date: YYYY-MM-DD
```

## Developing the Fix

### Security Patch Requirements

1. **Private Development**
   - Develop fix in private repository or branch
   - Limit access to security team and essential reviewers
   - Do not reference CVE or vulnerability details in commits

2. **Code Review**
   - Security-focused review using [SECURITY-REVIEW.md](SECURITY-REVIEW.md)
   - Verify fix addresses root cause, not just symptoms
   - Check for regression in related code

3. **Testing**
   - Unit tests for the specific vulnerability
   - Regression tests for affected functionality
   - Security testing (fuzzing, static analysis)
   - Test on all supported platforms

4. **Backporting**
   - Prepare patches for all supported versions
   - Test backports independently
   - Document any version-specific differences

### Commit Message Format

```
security: fix [brief description]

[Detailed description without revealing exploitation details]

This addresses a security vulnerability in [component].

Fixes: SEC-YYYY-NNN
CVE: CVE-YYYY-NNNNN
```

## Release Preparation

### Pre-Release Checklist

```
[ ] Fix reviewed and approved by security team
[ ] Fix tested on all supported versions
[ ] Backports prepared and tested
[ ] Security advisory drafted
[ ] Release notes prepared
[ ] Documentation updated
[ ] Disclosure date coordinated with reporter
[ ] Downstream projects notified (if applicable)
```

### Security Advisory Format

```markdown
# Security Advisory: [Title]

**CVE ID**: CVE-YYYY-NNNNN
**Severity**: [Critical/High/Medium/Low] (CVSS X.X)
**Affected Versions**: v1.2.0 - v1.4.2
**Fixed Versions**: v1.4.3, v1.3.5, v1.2.8

## Summary

[One paragraph summary suitable for public consumption]

## Impact

[What can an attacker achieve? Be specific but don't provide
exploitation instructions]

## Affected Users

[Who is affected? What configurations are vulnerable?]

## Mitigation

### Upgrade (Recommended)

Upgrade to one of the fixed versions:
- v1.4.3 or later (current release line)
- v1.3.5 or later (if on v1.3.x)
- v1.2.8 or later (if on v1.2.x)

### Workaround

[If a workaround exists, describe it here]

## Detection

[How can users detect if they were affected?]

## Timeline

- YYYY-MM-DD: Vulnerability reported
- YYYY-MM-DD: Fix developed
- YYYY-MM-DD: Fix released
- YYYY-MM-DD: Public disclosure

## Credit

This vulnerability was responsibly disclosed by [Reporter Name/Alias].

## References

- [Link to fixed versions]
- [Link to detailed documentation]
```

### Release Notes Format

```markdown
## Security Fixes

### CVE-YYYY-NNNNN: [Brief Title]

**Severity**: High (CVSS 7.5)

[Brief description]. Users should upgrade immediately.

See [security advisory](link) for details.
```

## Release Process

{{% alert title="Offline Signing Ceremony Required" color="warning" %}}
All releases — including security patch releases — must follow the offline
multi-party signing ceremony defined in
[RELEASE-PLAYBOOK.md](../../RELEASE-PLAYBOOK.md). Security patches use the
expedited process (Playbook Phase 14) which allows a reduced quorum of 2
participants but does not relax signing or verification requirements.
{{% /alert %}}

### Coordinated Disclosure Timeline

| Day | Action |
|-----|--------|
| D-7 | Notify downstream projects and major users (under embargo) |
| D-3 | Final testing of release artifacts |
| D-1 | Prepare announcement, stage release |
| D-0 | Release patches, publish advisory, notify mailing lists |
| D+1 | Monitor for issues, respond to questions |
| D+7 | Publish detailed technical write-up (optional) |

### Release Steps

Security patch releases follow `RELEASE-PLAYBOOK.md` Phase 14 (Emergency
and Patch Releases). The process is identical to a standard release with
these modifications:

- **Reduced quorum**: Minimum 2 participants (one signer, one witness).
  Requires unanimous approval among those present.
- **Scoped dependency audit**: May audit only changed dependencies, but
  `go mod verify` must still pass against the full tree.
- **Reproducibility**: Still required (2+ independent builds must match).

Steps:

1. **Convene ceremony** with at least 2 authorized participants
2. **Verify source** per Playbook Phase 2
3. **Build on air-gapped machine** per Playbook Phase 4
4. **Sign artifacts** per Playbook Phase 6 (majority of present signers)
5. **Publish** per Playbook Phase 9 (majority vote per channel)
6. **Post-release verification** within 24 hours per Playbook Phase 10
7. **Announce** via channels below

### Announcement Channels

| Channel | Timing | Content |
|---------|--------|---------|
| GitHub Security Advisory | Release time | Full advisory |
| Security mailing list | Release time | Advisory + upgrade instructions |
| Blog | Release time | Summary + links |
| Twitter/Social | Release time | Brief announcement + links |
| Slack/Discord | Release time | Announcement + support |

## Post-Release

### Monitoring

After release, monitor for:

- Upgrade adoption rate
- Issues with the fix
- Exploitation attempts
- Questions from users

### Metrics to Track

- Time from report to acknowledgment
- Time from report to fix available
- Time from fix to public disclosure
- Number of affected users
- Upgrade completion rate

### Retrospective

Within 2 weeks of release, conduct a retrospective:

- What went well?
- What could be improved?
- Update processes based on learnings
- Document any process changes

## Supported Versions

### Version Support Policy

| Version | Support Status | Security Patches Until |
|---------|----------------|----------------------|
| v2.x | Current | Active development |
| v1.4.x | Maintained | 12 months after v2.0 |
| v1.3.x | Security only | 6 months after v1.4 EOL |
| v1.2.x | End of life | No longer supported |

### Backport Policy

- **Critical (CVSS 9.0+)**: Backport to all maintained versions
- **High (CVSS 7.0-8.9)**: Backport to current and previous major
- **Medium (CVSS 4.0-6.9)**: Current version only
- **Low (CVSS < 4.0)**: Current version, next regular release

## Embargo Policy

### Embargo Duration

| Severity | Standard Embargo | Maximum Embargo |
|----------|-----------------|-----------------|
| Critical | 7 days | 14 days |
| High | 14 days | 30 days |
| Medium | 30 days | 60 days |
| Low | 60 days | 90 days |

### Embargo Rules

- Vulnerability details are confidential until disclosure date
- Only essential personnel have access to details
- Communication about the vulnerability is restricted
- No public commits, issues, or discussions referencing the vulnerability

### Breaking Embargo

Embargo may be shortened if:

- Active exploitation is detected in the wild
- Vulnerability is independently discovered and published
- Reporter requests early disclosure

## Third-Party Dependencies

### Handling Dependency Vulnerabilities

1. **Monitor** for vulnerabilities in dependencies (Dependabot, govulncheck)
2. **Assess** impact on Keystone Core
3. **Update** dependency to patched version
4. **Release** security update if exploitation is possible through Keystone Core

### Coordinating with Upstream

When a vulnerability is in a dependency:

1. Check if upstream has a fix
2. If not, report to upstream privately
3. Coordinate disclosure timing
4. Consider temporary mitigations while waiting for upstream fix

## Security Contacts

| Role | Contact | Backup |
|------|---------|--------|
| Security Lead | <security-lead@keystone-core.io> | <security@keystone-core.io> |
| Release Manager | <releases@keystone-core.io> | <engineering-leads@keystone-core.io> |
| Communications | <comms@keystone-core.io> | <marketing@keystone-core.io> |

## References

- [RELEASE-PLAYBOOK.md](../../RELEASE-PLAYBOOK.md) - Authoritative release process (multi-party signing ceremony)
- [SECURITY.md](../../SECURITY.md) - Reporting vulnerabilities
- [INCIDENT-RESPONSE.md](INCIDENT-RESPONSE.md) - Incident handling
- [CVSS Calculator](https://www.first.org/cvss/calculator/3.1)
- [CVE Program](https://www.cve.org/)
