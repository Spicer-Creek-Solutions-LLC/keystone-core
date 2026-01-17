# Security Policy

This document outlines security practices, assumptions, and procedures for Keystone Core.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in Keystone Core, please report it responsibly:

1. **Do not** open a public issue
2. Email security concerns to the maintainers with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Any suggested fixes

We aim to respond within 48 hours and will work with you to understand and address the issue.

## Security Architecture

### TLS/SSL

- **Production deployments**: All external communications use TLS by default
- **Certificate validation**: Enabled by default; insecure mode requires explicit environment variable (`KSCORE_ALLOW_INSECURE_TLS=1`)
- **CLI clients**: Default to HTTPS for non-localhost connections; HTTP only for localhost/127.0.0.1/::1

### Authentication & Authorization

- **SNMPv3**: Supports USM (User-based Security Model) with authentication and privacy
  - Recommended: SHA-256+ for authentication, AES-128+ for encryption
  - Deprecated: MD5 and DES (warnings logged when used)
- **API access**: Token-based authentication with configurable expiration
- **Credentials**: Stored encrypted at rest with proper key management

### HTTP Server Security

- **Timeouts**: All HTTP servers configured with read, write, and idle timeouts to prevent Slowloris attacks
- **CORS**: Origins must be explicitly configured; wildcard (*) not used in production
- **Headers**: Security headers recommended for proxy deployments

### Input Validation

- **Path traversal**: Use `pkg/security.ValidatePath()` for user-supplied paths
- **Filename validation**: Use `pkg/security.ValidateFilename()` for user-supplied filenames
- **SQL injection**: Parameterized queries used throughout
- **Command injection**: User input never passed directly to shell commands

## Security Scanning

### CI Pipeline

The following security tools run on every PR:

1. **gosec**: Static analysis for Go security issues
   - Configuration: `.gosec.yaml`
   - All findings are blocking by default

2. **govulncheck**: Dependency vulnerability scanning
   - Checks for known vulnerabilities in dependencies

### Running Locally

```bash
# Run gosec
gosec -conf .gosec.yaml -exclude-dir=test -exclude-dir=modules ./...

# Run govulncheck
govulncheck ./...
```

## Security Assumptions

### Network

1. Internal cluster communication is assumed to be on a trusted network
2. External API endpoints should be behind a reverse proxy with TLS termination
3. Agent-to-control-plane communication uses mutual TLS when available

### Host

1. The host operating system is properly hardened
2. File system permissions are correctly configured
3. Keystone Core runs with minimal required privileges

### Dependencies

1. Dependencies are regularly updated for security patches
2. Vulnerability alerts are monitored and addressed promptly
3. Only well-maintained dependencies are used

## Best Practices for Operators

### Deployment

- Run Keystone Core as a non-root user
- Use read-only file systems where possible
- Implement network segmentation for control plane
- Enable audit logging for compliance requirements

### Configuration

- Never commit credentials to version control
- Use environment variables or secret management for sensitive values
- Rotate credentials and API tokens regularly
- Configure TLS certificates with appropriate validity periods

### Monitoring

- Enable audit logging for security-relevant events
- Monitor for authentication failures
- Set up alerts for unusual API access patterns
- Review logs regularly for security anomalies

## Known Vulnerabilities

The following vulnerabilities have been identified and are being tracked:

| ID | Module | Status | Notes |
|----|--------|--------|-------|
| GO-2025-3547 | k8s.io/kubernetes | No fix available | Race condition in kube-apiserver. Indirect dependency via client-go. Monitoring for upstream fix. |
| GO-2025-3521 | k8s.io/kubernetes | No fix available | GitRepo Volume inadvertent local repository access. Indirect dependency via client-go. Monitoring for upstream fix. |

These are reviewed periodically and updated as fixes become available.

## Changelog

### 2026-01

- Added HTTP server timeouts to prevent Slowloris attacks
- Standardized TLS verification with environment variable gating
- Added CORS origin configuration (replaced wildcard)
- Added path validation helpers in `pkg/security`
- Added deprecation warnings for weak SNMP algorithms (MD5/DES)
- Defaulted CLI clients to HTTPS for production endpoints
