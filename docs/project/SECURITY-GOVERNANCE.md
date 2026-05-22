# Security Governance

This document establishes the security governance structure for Keystone Core, defining roles, responsibilities, decision-making processes, and security policies.

## Governance Overview

Security governance ensures that security considerations are systematically integrated into all aspects of the project. This framework establishes clear accountability, consistent processes, and continuous improvement for security.

### Governance Objectives

1. **Risk Management**: Identify, assess, and mitigate security risks
2. **Compliance**: Meet security requirements of relevant frameworks (SOC 2, PCI-DSS, HIPAA)
3. **Incident Response**: Respond effectively to security incidents
4. **Continuous Improvement**: Learn from incidents and evolve security practices
5. **Transparency**: Maintain open communication about security matters

---

## Security Organization Structure

```
┌─────────────────────────────────────────────────────────────────┐
│                   Project Leadership (BDFL)                      │
│                   - Final security decisions                     │
│                   - Breaking change approval                     │
│                   - Governance updates                           │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Security Working Group                        │
│                    - Security strategy                           │
│                    - Threat modeling                             │
│                    - Incident coordination                       │
│                    - Vulnerability management                    │
└──────────────────────────────┬──────────────────────────────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
          ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐
│ Security        │  │ Security        │  │ Security Response   │
│ Maintainers     │  │ Champions       │  │ Team                │
│                 │  │                 │  │                     │
│ - Code review   │  │ - Team liaison  │  │ - Incident handling │
│ - Architecture  │  │ - Training      │  │ - Forensics         │
│ - Standards     │  │ - Awareness     │  │ - Communication     │
└─────────────────┘  └─────────────────┘  └─────────────────────┘
```

---

## Roles and Responsibilities

### Project Leadership (BDFL)

**Security Responsibilities**:

- Approve security governance policies and updates
- Make final decisions on critical security matters
- Approve breaking changes with security implications
- Authorize security-related resource allocation
- Review and approve security incident communications

**Decision Authority**:

- Security policy changes
- Vulnerability disclosure timing for critical issues
- Security tool and process investments
- Security-related breaking changes

### Security Working Group

**Composition**: 3-5 members including at least one maintainer with security expertise

**Responsibilities**:

- Develop and maintain security strategy
- Review threat models for new features
- Coordinate vulnerability response
- Oversee security testing and audits
- Review security metrics and trends
- Recommend security investments

**Meeting Cadence**: Bi-weekly (more frequent during incidents)

**Decisions Made**:

- Vulnerability severity classification
- Patch timeline prioritization
- Security architecture recommendations
- Security tool selection

### Security Maintainers

**Qualifications**:

- Maintainer status in the project
- Demonstrated security expertise
- Completed security training curriculum

**Responsibilities**:

- Review security-sensitive pull requests
- Maintain security documentation
- Implement security controls and features
- Respond to security vulnerability reports
- Conduct security architecture reviews

**Authority**:

- Approve/reject PRs affecting security
- Request security reviews on any PR
- Escalate security concerns to working group

### Security Champions

**Qualifications**:

- Active contributor to the project
- Completed security training (Modules 1-3)
- Nominated by maintainers

**Responsibilities**:

- Advocate for security within development teams
- Perform initial security review of PRs
- Identify potential security issues early
- Promote security awareness and training
- Escalate security concerns appropriately

**One Per Area**:

- Core infrastructure
- Agent system
- API and authentication
- Module system
- State management
- Observability

### Security Response Team

**Composition**: On-call rotation of security maintainers and champions

**Responsibilities**:

- Initial triage of vulnerability reports
- Incident response coordination
- Forensic analysis when needed
- Communication with reporters
- Patch development and review

**Response SLAs**:

| Severity | Initial Response | Assessment | Patch |
|----------|------------------|------------|-------|
| Critical | 4 hours | 24 hours | 72 hours |
| High | 24 hours | 48 hours | 30 days |
| Medium | 48 hours | 7 days | 60 days |
| Low | 7 days | 30 days | Next release |

---

## Decision-Making Processes

### Security Policy Decisions

```
┌─────────────────────────────────────────────────────────────────┐
│                    Policy Change Proposed                        │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│              Security Working Group Review                       │
│              - Impact assessment                                 │
│              - Stakeholder identification                        │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Community Comment Period                      │
│                    (7-14 days for non-urgent)                    │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│              Working Group Recommendation                        │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    BDFL Approval                                 │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│               Implementation & Documentation                     │
└─────────────────────────────────────────────────────────────────┘
```

### Security Baseline Pipeline (Epic 19 task 7)

Four scans gate every PR via CI's `security` job. The same scans are
runnable locally as `make security-secrets`, `make security-vulns`,
`make security-sast`, `make security-licenses`.

| Scan | Tool | Threshold | Notes |
|------|------|-----------|-------|
| Secrets in git history | gitleaks | zero findings | scans full history (`fetch-depth: 0` in CI) |
| Known CVEs in deps | govulncheck | zero **called** vulnerabilities | Go toolchain pinned to `go 1.26.3` in `go.mod`; `golang.org/x/crypto` ≥ v0.52.0 |
| Static analysis (SAST) | gosec | zero **HIGH+** findings | G115 globally excluded (see below); `-severity=high` |
| Dependency licenses | go-licenses | no `forbidden`, `restricted`, or `unknown` | Apache-2.0 / MIT / BSD-compatible per epic 19 §Scope |

#### G115 (integer overflow) is globally excluded

`gosec`'s G115 rule flags every `int → int32` / `uint64 → int64`
conversion as a potential overflow. In this codebase those
conversions sit at well-validated boundaries — proto ↔ Go field
widths (where the protobuf int32/int64 width is the contract),
parser-checked archive headers, range-validated config. Per-site
`//nosec` annotation noise outweighs the signal.

This matches the standard Go-project posture (kubernetes, prometheus,
etcd, etc. all exclude G115 globally). Re-enabling G115 + a per-site
audit is tracked under the v1.x ROADMAP entry *"Security baseline
expansion"* as a sub-bullet.

#### gosec annotation convention

For findings that are real false positives (or where the security
posture is intentional), annotate with a `// #nosec` comment on
or just before the offending line:

```go
// #nosec G404 -- jitter is anti-thundering-herd timing, not security-sensitive
offset := (rand.Float64()*2 - 1) * jitterRange
```

Every `// #nosec` annotation MUST carry a short rationale after the
`--`. `make security-sast` reads `// #nosec` in two forms: trailing
the statement (`stmt() // #nosec G404`) or on the line above.

`// #nosec` is the gosec syntax — `//nolint:gosec` only suppresses
golangci-lint's gosec linter, not standalone gosec. Most existing
annotations carry **both** so each tool sees its own marker.

#### License-check ignore convention

`make security-licenses` carries one ignore today:

- `modernc.org/mathutil` — BSD-3-Clause (confirmed in its LICENSE
  file), but go-licenses can't auto-classify because the file lacks
  an SPDX header. The `--ignore` is documented inline in the Makefile.

Any new ignore needs the same shape — a comment naming the actual
license + why go-licenses can't classify it.

#### Deferred to v1.x

Tracked as sub-bullets under the ROADMAP entry *"Security baseline
expansion"*:

- Re-enable G115 + audit every site (currently ~84 findings,
  almost all at bounded conversions).
- Add semgrep for cross-language SAST.
- Add trivy / grype for container-image and lockfile scanning.
- Add syft for SBOM generation.
- Add hadolint for Dockerfile linting.

### Security PR Review Process

All PRs are automatically labeled by CI based on files changed. PRs with security-relevant changes require additional review.

**Security-Sensitive Areas** (require security review):

- `internal/security/*` - Security utilities
- `internal/audit/*` - Audit logging
- `pkg/api/auth/*` - API authentication
- `cmd/*/exec*` - Command execution
- `internal/policy/*` - Policy enforcement
- Any file containing `password`, `secret`, `token`, `credential`

**Review Requirements**:

| Change Type | Required Reviewers | Approval Count |
|-------------|-------------------|----------------|
| Security-sensitive code | Security maintainer | 2 |
| Authentication/authorization | Security maintainer | 2 |
| Cryptographic changes | Security maintainer + crypto expert | 2 |
| Security configuration | Security maintainer | 1 |
| Documentation only | Any maintainer | 1 |

### Vulnerability Response Process

See [SECURITY-RELEASE.md](SECURITY-RELEASE.md) for detailed procedures.

**Summary**:

1. **Report Received**: Acknowledge within 48 hours
2. **Triage**: Assess severity and impact within 24-48 hours
3. **Develop Fix**: Create and review patch privately
4. **Disclosure Coordination**: Work with reporter on timeline
5. **Release**: Publish fix and advisory
6. **Post-Mortem**: Document lessons learned

---

## Security Policies

### Vulnerability Disclosure Policy

**Coordinated Disclosure**:

- Keystone Core follows responsible disclosure practices
- We work with reporters to set reasonable timelines
- Standard embargo period: 90 days (negotiable based on severity)
- Reporters credited unless they prefer anonymity

**Disclosure Channels**:

- Security reports: <security@keystone-core.io>
- PGP key available on website for encrypted reports
- GitHub Security Advisories for managed disclosure

### Dependency Management Policy

**Requirements**:

- All dependencies must be vetted before addition
- Dependencies must have active maintenance (commit within 12 months)
- Security vulnerabilities must be addressed within SLA
- Transitive dependencies are subject to same requirements

**Monitoring**:

- Dependabot alerts enabled for all repositories
- Weekly dependency review by security maintainer
- Quarterly deep review of all dependencies

**Response**:

- Critical vulnerabilities: Immediate update or removal
- High vulnerabilities: Update within 30 days
- Medium/Low: Update in next regular release

### Cryptographic Standards Policy

See [SECURITY-DESIGN.md](SECURITY-DESIGN.md) for approved algorithms.

**Requirements**:

- Use only approved cryptographic algorithms
- Cryptographic code must be reviewed by security maintainer
- No custom cryptographic implementations
- Deprecation warnings for aging algorithms
- Migration path for algorithm transitions

### Access Control Policy

**Repository Access**:

| Role | Permissions |
|------|-------------|
| Contributor | Read, fork, submit PRs |
| Triage | Above + label, assign issues |
| Maintainer | Above + merge, manage branches |
| Security Maintainer | Above + security advisory access |
| Admin | Above + repository settings |

**Secrets Management**:

- CI/CD secrets managed through GitHub Secrets
- Rotation required every 90 days
- Access logged and reviewed quarterly
- Separation of duties for production secrets

### Incident Classification Policy

**Severity Levels**:

| Level | Description | Examples |
|-------|-------------|----------|
| **Critical** | Active exploitation, data breach, complete system compromise | RCE in production, credential theft |
| **High** | Significant vulnerability, potential for exploitation | Auth bypass, privilege escalation |
| **Medium** | Vulnerability with mitigating factors | CSRF, limited data exposure |
| **Low** | Minor issue, difficult to exploit | Information disclosure, DoS with auth |

**Escalation Matrix**:

| Severity | Notification | Response Team | Communication |
|----------|--------------|---------------|---------------|
| Critical | Immediate (phone) | Full security team + BDFL | Public advisory within 72h |
| High | Same day (email) | Security maintainers | Advisory with patch |
| Medium | 48 hours | Assigned maintainer | Release notes |
| Low | Weekly review | Regular PR process | Changelog |

---

## Security Metrics and Reporting

### Key Performance Indicators

| Metric | Target | Measurement |
|--------|--------|-------------|
| Vulnerability Response Time | < SLA | Time from report to acknowledgment |
| Patch Deployment Time | < SLA | Time from report to fix release |
| Open Critical Vulnerabilities | 0 | Count at any point |
| Open High Vulnerabilities | < 3 | Count at any point |
| Security Review Coverage | 100% | Security-sensitive PRs reviewed |
| Dependency Vulnerability Backlog | < 5 high | Unresolved dependency CVEs |
| Security Training Completion | 100% | Active maintainers trained |

### Reporting Cadence

| Report | Audience | Frequency |
|--------|----------|-----------|
| Security Metrics Dashboard | Public | Real-time |
| Vulnerability Status | Security team | Weekly |
| Security Posture Summary | BDFL | Monthly |
| Annual Security Review | Community | Yearly |

### Security Dashboard

Public security metrics available at `/security-dashboard`:

- Days since last security incident
- Open vulnerability count by severity
- Security patch coverage
- Dependency health status

---

## Compliance and Audit

### Compliance Framework Alignment

Keystone Core security controls are designed to support:

| Framework | Applicability | Evidence |
|-----------|---------------|----------|
| SOC 2 Type II | Cloud deployments | Security controls documentation |
| PCI-DSS | Financial sector | Encryption, access control, logging |
| HIPAA | Healthcare | Audit trails, access controls |
| FedRAMP | Government | Security baseline alignment |
| NIST 800-53 | General | Control mapping documentation |

### Audit Support

**Audit Readiness**:

- Control documentation maintained in `/docs/compliance/`
- Evidence collection automated where possible
- Audit trails preserved per retention policy
- Change history in version control

**Audit Request Process**:

1. Request received through official channels
2. Security working group assigns liaison
3. Evidence package prepared
4. Findings tracked and addressed

---

## Training and Awareness

### Required Training

| Role | Required Training | Renewal |
|------|-------------------|---------|
| All Maintainers | Module 1: Security Fundamentals | Annual |
| Code Contributors | Module 2: Secure Development | Annual |
| Security Maintainers | Full curriculum (all modules) | Annual |
| Security Champions | Modules 1-5 | Annual |

### Security Awareness Program

**Components**:

- Quarterly security newsletters
- Monthly threat briefings
- Security office hours (bi-weekly)
- Annual security workshop

**Topics Covered**:

- Emerging threats and vulnerabilities
- Security best practices updates
- Incident lessons learned
- Tool and process changes

---

## Exception and Waiver Process

### When Exceptions Apply

Exceptions may be requested when:

- Technical constraints prevent compliance
- Temporary workaround while proper fix is developed
- Legacy system transition period
- Research or testing purposes

### Exception Request Process

1. **Submit Request**: Document exception with justification
2. **Risk Assessment**: Security team evaluates risk
3. **Mitigations**: Identify compensating controls
4. **Approval**: Security working group + BDFL for high-risk
5. **Documentation**: Record exception in security log
6. **Review Date**: Set expiration (max 90 days)

### Exception Documentation

```yaml
exception_id: SEC-EXC-2025-001
policy: Cryptographic Standards Policy
requested_by: contributor@keystone-core.io
approved_by: security-lead@keystone-core.io
justification: |
  Legacy system integration requires TLS 1.0 support during
  transition period. Customer cannot upgrade until Q2.
risk_level: medium
mitigations:
  - Network isolation
  - Enhanced monitoring
  - Short-lived certificates
expiration: 2025-03-31
review_date: 2025-02-15
```

---

## Policy Review and Updates

### Review Schedule

| Document | Review Frequency | Last Review | Next Review |
|----------|------------------|-------------|-------------|
| Security Governance | Annual | 2025-01 | 2026-01 |
| Security Policy | Semi-annual | 2025-01 | 2025-07 |
| Incident Response Plan | Annual | 2025-01 | 2026-01 |
| Threat Model | Quarterly | 2025-01 | 2025-04 |
| Security Training | Annual | 2025-01 | 2026-01 |

### Change Process

1. **Proposed Change**: Submit as PR with rationale
2. **Review Period**: 14 days for comments (7 days urgent)
3. **Working Group Review**: Assess impact and feasibility
4. **Community Input**: Consider feedback
5. **BDFL Approval**: Final decision
6. **Implementation**: Update docs, communicate changes

---

## Contact Information

**Security Working Group**: <security-wg@keystone-core.io>
**Vulnerability Reports**: <security@keystone-core.io>
**Security Questions**: #security channel in community chat

**PGP Key Fingerprint**: (Published on website)

---

## Related Documents

- [SECURITY.md](../../SECURITY.md) - Vulnerability reporting and security assumptions
- [SECURITY-DESIGN.md](SECURITY-DESIGN.md) - Security design principles
- [SECURITY-REVIEW.md](SECURITY-REVIEW.md) - Security review process
- [SECURITY-RELEASE.md](SECURITY-RELEASE.md) - Security release procedures
- [Threat Model](/docs/concepts/threat-model/) - Security threats and mitigations
- [Security Training](/docs/operations/security-training/) - Training curriculum

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-01 | Security Team | Initial governance framework |
