# Security Review Checklist for Pull Requests

This document provides comprehensive checklists for reviewing security-related changes in pull requests. Use these checklists to ensure consistent and thorough security reviews.

## When Security Review is Required

A security review is **required** for PRs that modify:

- Authentication or authorization logic
- Cryptographic operations or key management
- Database queries with user input
- File operations with user-supplied paths
- Network protocol handling
- Credential or secret management
- Audit logging
- Trust boundary crossings
- Command execution logic
- API endpoint handlers
- TLS/certificate handling

## General Security Checklist

### Input Validation

- [ ] All external inputs are validated before use
- [ ] Input validation happens at trust boundaries, not just the perimeter
- [ ] Validation uses allowlists rather than denylists where possible
- [ ] Invalid input results in rejection, not modification
- [ ] Error messages do not reveal internal implementation details

### Output Encoding

- [ ] Output is properly encoded for its destination context
- [ ] User-controlled data is never directly included in logs without sanitization
- [ ] Sensitive data is redacted before logging
- [ ] Error responses do not leak internal system information

### Authentication

- [ ] Authentication failures return generic error messages
- [ ] Authentication state is checked on every request requiring auth
- [ ] Session/token handling follows secure practices
- [ ] Credentials are never logged or exposed in error messages

### Authorization

- [ ] Authorization checks are performed for all protected operations
- [ ] Authorization uses deny-by-default
- [ ] RBAC roles are correctly assigned and checked
- [ ] Privilege escalation paths are not introduced

### Cryptography

- [ ] Only approved algorithms are used (see SECURITY-DESIGN.md)
- [ ] Key sizes meet minimum requirements
- [ ] Random numbers use `crypto/rand`, not `math/rand`
- [ ] Cryptographic secrets are properly zeroized after use
- [ ] No hardcoded keys, IVs, or seeds

## Code Pattern Checklists

### SQL/Database Operations

- [ ] All queries use parameterized statements or prepared queries
- [ ] No string concatenation for query construction
- [ ] ORDER BY columns are allowlisted, not user-controlled
- [ ] LIMIT/OFFSET values are validated as positive integers
- [ ] Connection strings do not contain credentials in logs

**Bad pattern**:

```go
query := "SELECT * FROM users WHERE name = '" + userName + "'"
```

**Good pattern**:

```go
query := "SELECT * FROM users WHERE name = ?"
db.Query(query, userName)
```

### File Operations

- [ ] File paths are validated using `security.ValidatePath()`
- [ ] Directory traversal is prevented (`../` sequences blocked)
- [ ] Symbolic links are handled safely or rejected
- [ ] File permissions are restrictive (600 for sensitive files)
- [ ] Temporary files are created securely and cleaned up

**Bad pattern**:

```go
path := filepath.Join(baseDir, userInput)
```

**Good pattern**:

```go
if err := security.ValidatePath(userInput); err != nil {
    return err
}
path := filepath.Join(baseDir, filepath.Clean(userInput))
if !strings.HasPrefix(path, baseDir) {
    return ErrPathTraversal
}
```

### Command Execution

- [ ] User input is never passed to shell interpreters
- [ ] `exec.Command()` is used with explicit arguments, not shell strings
- [ ] Command allowlists are enforced where applicable
- [ ] Environment variables are explicitly set, not inherited
- [ ] Working directory is explicitly set

**Bad pattern**:

```go
cmd := exec.Command("sh", "-c", "ls " + userDir)
```

**Good pattern**:

```go
cmd := exec.Command("ls", userDir)
cmd.Dir = "/safe/base/path"
cmd.Env = []string{"PATH=/usr/bin"}
```

### Network Operations

- [ ] TLS is enforced for external connections
- [ ] Certificate verification is not disabled (no `InsecureSkipVerify`)
- [ ] Timeouts are set on all network operations
- [ ] IP addresses/hosts from user input are validated
- [ ] SSRF protections are in place for user-supplied URLs

### Error Handling

- [ ] Errors are handled at every level
- [ ] Internal errors are logged with details
- [ ] External error messages are generic
- [ ] Panics are recovered in request handlers
- [ ] Resource cleanup happens even on error (defer/finally)

**Bad pattern**:

```go
return fmt.Errorf("failed to query database: %v", err)
```

**Good pattern**:

```go
log.Error("database query failed", "error", err, "query_type", "user_lookup")
return ErrInternalError
```

### Secrets Management

- [ ] Secrets are never logged
- [ ] Secrets are never committed to version control
- [ ] Secrets use environment variables or secret managers
- [ ] In-memory secrets are zeroized when no longer needed
- [ ] Secret comparison uses constant-time functions

### Concurrency

- [ ] Shared state is properly synchronized
- [ ] Race conditions are not introduced
- [ ] Locks are released on all code paths (defer)
- [ ] Timeouts prevent indefinite blocking
- [ ] Context cancellation is respected

## API Endpoint Checklist

- [ ] Endpoint requires appropriate authentication
- [ ] Authorization is checked before processing
- [ ] Input is validated against expected schema
- [ ] Rate limiting is applied where appropriate
- [ ] Response does not leak sensitive information
- [ ] CORS configuration is appropriate
- [ ] Request size limits are enforced

## NATS/Message Bus Checklist

- [ ] Message publishers have appropriate permissions
- [ ] Message subscribers validate incoming data
- [ ] Subject names do not contain user-controlled data
- [ ] Acknowledgment handling is correct
- [ ] Dead letter handling is implemented

## Module/Plugin Checklist

- [ ] Module capabilities are declared and enforced
- [ ] Sandbox restrictions are not bypassed
- [ ] Resource limits are configured
- [ ] Module signatures are verified
- [ ] Untrusted code cannot access sensitive APIs

## Audit Logging Checklist

- [ ] Security-relevant actions are logged
- [ ] Audit events include required fields (see SECURITY-DESIGN.md)
- [ ] Sensitive data is redacted
- [ ] Audit logs are written to independent sink
- [ ] Log integrity is maintained

## Pre-Merge Verification

Before approving a security-related PR:

1. [ ] All automated security checks pass (gosec, govulncheck)
2. [ ] Unit tests cover security-critical paths
3. [ ] Integration tests verify security controls
4. [ ] Documentation is updated if behavior changes
5. [ ] Threat model impact is assessed

## Severity Classification

When identifying issues, classify by severity:

| Severity | Description | Action |
|----------|-------------|--------|
| **Critical** | Remote code execution, auth bypass, data breach | Block merge, immediate fix required |
| **High** | Privilege escalation, sensitive data exposure | Block merge, fix before release |
| **Medium** | DoS, information disclosure (non-sensitive) | Should fix before release |
| **Low** | Minor issues, defense-in-depth improvements | Fix when convenient |

## Requesting Security Review

To request a security review:

1. Add the `security-review` label to the PR
2. Assign a reviewer from the security team
3. Include a brief description of security-relevant changes
4. Note any areas of concern or uncertainty

## Post-Merge Monitoring

After merging security-related changes:

1. Monitor error rates and audit logs for anomalies
2. Watch for security scanner findings in CI
3. Be prepared to revert if issues are discovered

## References

- [SECURITY-DESIGN.md](SECURITY-DESIGN.md) - Security design + system threat model
- [OWASP Code Review Guide](https://owasp.org/www-project-code-review-guide/)
- [Go Security Best Practices](https://go.dev/doc/security/best-practices)
