# Security Design Principles

This document defines the security design principles, architectural guardrails, and cryptographic standards for Keystone Core. These principles are inspired by NIST 800-53 and industry best practices, providing a foundation for security-conscious development.

## Core Design Principles

### 1. Least Privilege

**Principle**: Every component, process, and user should operate with the minimum privileges necessary to perform its function.

**Implementation Guidelines**:

- Agents run as non-root users by default; root access requires explicit configuration
- API endpoints enforce role-based access control (RBAC) with deny-by-default
- Module capabilities are explicitly declared and enforced; no implicit permissions
- Database connections use role-specific credentials (read-only, read-write, admin)
- NATS subjects use fine-grained publish/subscribe permissions per component

**Code Review Checklist**:

- [ ] Does this component request only necessary permissions?
- [ ] Are elevated privileges scoped to the minimum required duration?
- [ ] Is there a clear justification for any root/admin access?

### 2. Defense in Depth

**Principle**: Security controls should be layered so that compromise of one layer does not immediately compromise the entire system.

**Implementation Guidelines**:

- Multiple authentication factors: network (mTLS), identity (SPIFFE/JWT), authorization (OPA/CEL)
- Input validation at every trust boundary, not just the perimeter
- Secrets are encrypted at rest AND in transit, with separate key hierarchies
- Audit logging is independent of application logging with separate retention
- Network segmentation between control plane, agents, and external services

**Code Review Checklist**:

- [ ] Are there multiple independent controls protecting sensitive operations?
- [ ] Does failure of one control still leave other protections in place?
- [ ] Is data validated at each trust boundary crossing?

### 3. Fail Secure

**Principle**: When a component fails or encounters an error, it should fail to a secure state that denies access rather than granting it.

**Implementation Guidelines**:

- Authentication failures result in denial, never default access
- Policy evaluation errors deny the action (fail closed)
- Missing or invalid certificates reject the connection
- Database connection failures prevent operations rather than using cached/stale data
- Rate limit exhaustion returns 429/503, not success

**Code Review Checklist**:

- [ ] What happens if this operation times out? Is access denied?
- [ ] If the policy engine is unavailable, are requests denied or queued?
- [ ] Do error handlers maintain security invariants?

### 4. Explicit Over Implicit

**Principle**: Security-relevant configuration should be explicit. Avoid hidden defaults that might create security gaps.

**Implementation Guidelines**:

- TLS is required by default; insecure mode requires `KSCORE_ALLOW_INSECURE_TLS=1`
- CORS origins must be explicitly configured; no wildcard (`*`) in production
- Agent registration requires explicit attestation or join token
- Module trust requires explicit policy configuration
- Webhook secrets must be explicitly configured per source

**Code Review Checklist**:

- [ ] Are all security-relevant settings explicitly configured?
- [ ] Is there documentation for every security default?
- [ ] Would a user be surprised by the default behavior?

### 5. Auditability

**Principle**: All security-relevant actions must be logged with sufficient detail for forensic analysis and compliance.

**Implementation Guidelines**:

- Every API request logs: user, action, resource, result, timestamp, correlation ID
- Command executions log: initiator, target, command hash, success/failure
- State changes log: before/after values (with sensitive data redacted)
- Authentication events log: method, result, source IP, user agent
- Audit logs are append-only with integrity protection

**Code Review Checklist**:

- [ ] Would this action be visible in audit logs?
- [ ] Is there enough context to understand what happened?
- [ ] Are sensitive values redacted but action still traceable?

### 6. Cryptographic Agility

**Principle**: Cryptographic algorithms should be configurable to allow migration when algorithms are deprecated or compromised.

**Implementation Guidelines**:

- TLS cipher suites are configurable (with secure defaults)
- Hash algorithms for integrity checks are versioned in data format
- Key derivation functions are abstracted behind interfaces
- Module signatures support multiple algorithms (RSA, ECDSA, Ed25519)
- Deprecated algorithms (MD5, SHA1, DES, 3DES) emit warnings when used

**Code Review Checklist**:

- [ ] Is the cryptographic algorithm configurable or hardcoded?
- [ ] Is there a migration path if this algorithm is deprecated?
- [ ] Are deprecated algorithms logged and warned about?

### 7. Reproducible Builds

**Principle**: Builds should be deterministic and verifiable to detect supply chain attacks.

**Implementation Guidelines**:

- Go modules use checksummed dependencies (`go.sum`)
- Module signatures are verified against SumDB transparency log
- Docker images use content-addressable digests, not floating tags
- Build metadata includes Git commit, builder identity, timestamp
- SBOM (Software Bill of Materials) generated for releases

**Code Review Checklist**:

- [ ] Are all dependencies pinned to specific versions?
- [ ] Can this build be reproduced from source?
- [ ] Is the build provenance verifiable?

### 8. Trust Boundary Enforcement

**Principle**: Data crossing trust boundaries must be validated, authenticated, and authorized.

**Implementation Guidelines**:

- External API inputs are validated against schemas before processing
- Agent commands are authorized by policy engine before execution
- File paths are validated to prevent directory traversal
- User-supplied content is never executed or interpreted directly
- Cross-boundary data carries integrity proofs (signatures, MACs)

**Code Review Checklist**:

- [ ] What trust boundaries does this data cross?
- [ ] Is the data validated at each boundary?
- [ ] Can an attacker influence data after validation?

---

## Cryptographic Standards

### Approved Algorithms

| Purpose | Recommended | Acceptable | Deprecated |
|---------|-------------|------------|------------|
| **Symmetric Encryption** | AES-256-GCM | AES-128-GCM, ChaCha20-Poly1305 | DES, 3DES, AES-ECB |
| **Asymmetric Encryption** | RSA-4096, X25519 | RSA-2048, P-256 | RSA-1024 |
| **Digital Signatures** | Ed25519, ECDSA P-384 | RSA-PSS (2048+), ECDSA P-256 | RSA PKCS#1 v1.5, DSA |
| **Hash Functions** | SHA-256, SHA-384, SHA-512 | SHA-3, BLAKE2b | MD5, SHA-1 |
| **Key Derivation** | Argon2id, scrypt | PBKDF2-SHA256 (100k+ iterations) | PBKDF2-SHA1, bcrypt (low cost) |
| **Message Authentication** | HMAC-SHA256, Poly1305 | HMAC-SHA384/512 | HMAC-MD5, HMAC-SHA1 |

### TLS Configuration

**Minimum Version**: TLS 1.2 (TLS 1.3 preferred)

**Recommended Cipher Suites (in priority order)**:

```
TLS_AES_256_GCM_SHA384 (TLS 1.3)
TLS_CHACHA20_POLY1305_SHA256 (TLS 1.3)
TLS_AES_128_GCM_SHA256 (TLS 1.3)
TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 (TLS 1.2)
TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 (TLS 1.2)
TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 (TLS 1.2)
TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 (TLS 1.2)
```

**Disabled Cipher Suites**:

- All CBC mode ciphers
- All RC4 ciphers
- All export-grade ciphers
- All NULL ciphers
- All anonymous ciphers

**Certificate Requirements**:

- Minimum RSA key size: 2048 bits (4096 recommended)
- Minimum ECDSA key size: P-256 (P-384 recommended)
- Maximum certificate validity: 398 days (production), 825 days (development CA)
- Subject Alternative Name (SAN) required; CN deprecated for hostname matching

### Key Sizes

| Key Type | Minimum | Recommended | Notes |
|----------|---------|-------------|-------|
| RSA (encryption) | 2048 bits | 4096 bits | For long-term CA keys |
| RSA (signing) | 2048 bits | 3072 bits | For short-lived certificates |
| ECDSA | P-256 | P-384 | For certificates |
| Ed25519 | 256 bits | 256 bits | Fixed size |
| AES | 128 bits | 256 bits | For data encryption |
| HMAC | 256 bits | 256 bits | For message authentication |

### Random Number Generation

- Use `crypto/rand` for all cryptographic operations
- Never use `math/rand` for security-sensitive purposes
- Seed generation must use OS entropy sources

---

## Audit Event Taxonomy

### Event Categories

| Category | Prefix | Description |
|----------|--------|-------------|
| Authentication | `auth.*` | Login, logout, token operations |
| Authorization | `authz.*` | Permission checks, policy evaluations |
| Agent | `agent.*` | Registration, heartbeat, status changes |
| Command | `cmd.*` | Remote command execution |
| State | `state.*` | Configuration state operations |
| Policy | `policy.*` | Policy CRUD and enforcement |
| System | `system.*` | System-level events (startup, shutdown) |
| Admin | `admin.*` | Administrative operations |

### Required Fields

Every audit event MUST include:

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | RFC 3339 | Event occurrence time (UTC) |
| `event_type` | string | Category-prefixed event type |
| `correlation_id` | UUID | Request correlation identifier |
| `actor` | object | Who performed the action |
| `actor.type` | string | `user`, `agent`, `system`, `service` |
| `actor.id` | string | Unique identifier |
| `resource` | object | What was affected |
| `resource.type` | string | Resource type (agent, policy, state) |
| `resource.id` | string | Resource identifier |
| `action` | string | Action performed (create, read, update, delete, execute) |
| `outcome` | string | `success`, `failure`, `error` |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `source_ip` | string | Client IP address |
| `user_agent` | string | Client user agent |
| `duration_ms` | integer | Operation duration |
| `error_code` | string | Error code if failed |
| `error_message` | string | Error description (sanitized) |
| `metadata` | object | Event-specific additional data |
| `previous_value` | any | Value before change (redacted) |
| `new_value` | any | Value after change (redacted) |

### Event Types

#### Authentication Events

- `auth.login.success` - Successful authentication
- `auth.login.failure` - Failed authentication attempt
- `auth.logout` - User logout
- `auth.token.issued` - New token issued
- `auth.token.revoked` - Token revoked
- `auth.token.expired` - Token expired
- `auth.mfa.challenge` - MFA challenge issued
- `auth.mfa.success` - MFA verification succeeded
- `auth.mfa.failure` - MFA verification failed

#### Authorization Events

- `authz.check.allowed` - Permission check allowed
- `authz.check.denied` - Permission check denied
- `authz.policy.evaluated` - Policy evaluation completed

#### Agent Events

- `agent.registered` - New agent registered
- `agent.deregistered` - Agent removed
- `agent.heartbeat.received` - Heartbeat received
- `agent.heartbeat.missed` - Heartbeat timeout
- `agent.status.changed` - Agent status changed
- `agent.certificate.issued` - Certificate issued to agent
- `agent.certificate.rotated` - Certificate rotated

#### Command Events

- `cmd.submitted` - Command submitted for execution
- `cmd.started` - Command execution started
- `cmd.completed` - Command execution completed
- `cmd.failed` - Command execution failed
- `cmd.cancelled` - Command cancelled
- `cmd.timeout` - Command timed out

#### State Events

- `state.applied` - State configuration applied
- `state.checked` - State drift check performed
- `state.drift.detected` - Configuration drift detected
- `state.remediated` - Drift remediated

#### Policy Events

- `policy.created` - Policy created
- `policy.updated` - Policy updated
- `policy.deleted` - Policy deleted
- `policy.activated` - Policy activated
- `policy.deactivated` - Policy deactivated
- `policy.violation` - Policy violation detected

#### System Events

- `system.startup` - System started
- `system.shutdown` - System shutdown initiated
- `system.config.changed` - Configuration changed
- `system.backup.created` - Backup created
- `system.backup.restored` - Backup restored

#### Admin Events

- `admin.user.created` - User account created
- `admin.user.updated` - User account updated
- `admin.user.deleted` - User account deleted
- `admin.role.assigned` - Role assigned to user
- `admin.role.revoked` - Role revoked from user

### Retention Recommendations

| Event Category | Minimum Retention | Recommended Retention | Notes |
|----------------|-------------------|----------------------|-------|
| Authentication | 90 days | 1 year | Compliance requirement |
| Authorization | 90 days | 1 year | For access reviews |
| Command | 1 year | 3 years | Forensics requirement |
| State | 1 year | 3 years | Change tracking |
| Policy | 1 year | 3 years | Compliance evidence |
| System | 90 days | 1 year | Operational needs |
| Admin | 1 year | 7 years | Regulatory compliance |

### Sensitive Data Handling

The following data MUST be redacted in audit logs:

- Passwords and secrets (replace with `[REDACTED]`)
- Private keys and certificates
- API tokens and bearer tokens
- Personal identifiable information (PII) where not essential
- Credit card numbers and financial data

The following MAY be logged with appropriate masking:

- Email addresses (partial: `u***@example.com`)
- IP addresses (may be full for security investigations)
- Resource identifiers (full, for traceability)

---

## Contributor Security Guidelines

### Before Writing Code

1. **Understand the threat model**: Review [docs/concepts/threat-model.md](docs/content/en/docs/concepts/threat-model.md)
2. **Identify trust boundaries**: Know which boundaries your code crosses
3. **Plan for failures**: Design for graceful degradation

### During Development

1. **Validate all input**: Use `internal/security.Validate*` helpers for user input
2. **Handle errors securely**: Never expose internal errors to users
3. **Use parameterized queries**: Never concatenate SQL strings
4. **Avoid shell injection**: Never pass user input to `exec.Command` without validation
5. **Log securely**: Use structured logging with automatic redaction

### Code Patterns to Avoid

```go
// BAD: SQL injection vulnerability
query := "SELECT * FROM users WHERE id = '" + userID + "'"

// GOOD: Parameterized query
query := "SELECT * FROM users WHERE id = ?"
db.Query(query, userID)
```

```go
// BAD: Path traversal vulnerability
filepath := "/data/" + userInput

// GOOD: Validated path
if err := security.ValidatePath(userInput); err != nil {
    return err
}
filepath := filepath.Join("/data", filepath.Clean(userInput))
```

```go
// BAD: Command injection vulnerability
cmd := exec.Command("sh", "-c", "ls " + userDir)

// GOOD: Direct command with arguments
cmd := exec.Command("ls", userDir)
```

```go
// BAD: Exposing internal errors
return fmt.Errorf("database error: %v", err)

// GOOD: Generic error to user, detailed log
log.Error("database query failed", "error", err, "query", query)
return ErrInternalServer
```

### Security Review Requirements

PRs that touch these areas require explicit security review:

- Authentication or authorization logic
- Cryptographic operations
- Database queries with user input
- File operations with user-supplied paths
- Network protocol handling
- Credential or secret management
- Audit logging
- Trust boundary crossings

### Reporting Security Issues

See [SECURITY.md](SECURITY.md) for vulnerability reporting procedures.

---

## References

- [NIST SP 800-53 Rev. 5](https://csrc.nist.gov/publications/detail/sp/800-53/rev-5/final) - Security and Privacy Controls
- [OWASP Secure Coding Practices](https://owasp.org/www-project-secure-coding-practices-quick-reference-guide/)
- [CWE/SANS Top 25](https://cwe.mitre.org/top25/archive/2023/2023_top25_list.html) - Most Dangerous Software Weaknesses
- [NIST Cryptographic Standards](https://csrc.nist.gov/projects/cryptographic-standards-and-guidelines)
- [SPIFFE Specification](https://spiffe.io/docs/latest/spiffe-about/overview/)
