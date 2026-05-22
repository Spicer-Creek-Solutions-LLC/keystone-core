# Security Policy

This document outlines security practices, assumptions, and procedures for Keystone Core.

## Supported Versions

| Version | Supported |
|---------|-----------|
| `v1.x` (latest minor) | yes |
| `v0.x` (pre-1.0) | best-effort during reconstruction; no SemVer stability guarantee |

`v0.x` is the active reconstruction line — see [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) for the v0.1 → v0.5 → v1.0 ladder. Once `v1.0` ships, the previous minor (`v1.<N-1>`) also receives security fixes until the next minor + 30 days.

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

- **Operator API**: API-key (`Authorization: Bearer …`) or mTLS (`identity.enabled: true`). RBAC roles: admin / operator / readonly (`pkg/api/auth/authorizer.go`).
- **Agent bootstrap**: PSK (`nats.bootstrap.psks`) or identity-issued join token + SVID (`identity.enabled: true` — see [`docs/project/SECURITY-DESIGN.md`](docs/project/SECURITY-DESIGN.md)). Identity is the v1.0 production path; PSK is documented for v0.x trials.
- **Command signing**: HMAC-SHA-256 between server and agent (`security.hmacsecret`). Operators must rotate via an out-of-band secret manager; the `ProductionWarnings()` machinery surfaces a warning when a static HMAC ships to production.
- **Credentials at rest**: API keys hashed via SHA-256 in the store; secret values encrypted via the encrypted-file backend (`internal/secrets/file`) or delegated to Vault (`internal/secrets/vault`).

### HTTP Server Security

- **Timeouts**: All HTTP servers configured with read, write, and idle timeouts to prevent Slowloris attacks
- **CORS**: Origins must be explicitly configured; wildcard (*) not used in production
- **Headers**: Security headers recommended for proxy deployments

### Input Validation

- **Path traversal**: Use `pkg/security.ValidatePath()` for user-supplied paths
- **Filename validation**: Use `pkg/security.ValidateFilename()` for user-supplied filenames
- **SQL injection**: Parameterized queries used throughout
- **Command injection**: User input never passed directly to shell commands

## Supply Chain Security & Release Verification

Keystone Core releases are produced through a formal offline multi-party
signing ceremony. No release artifacts are built or signed in CI/CD. This
is a deliberate choice to minimize the attack surface of the release process.

Every release ships with:

- **Signed checksums** — `checksums.txt` signed by multiple release signers (GPG detached signatures)
- **SBOMs** — CycloneDX and SPDX format, signed
- **Release record** — A full audit log of the ceremony, signed by all participants
- **Container image signatures** — Cosign key-based signatures (not keyless)

### Verifying Release Artifacts

```bash
# Import the release public key
gpg --import keys/release-pubkey.asc

# Verify checksums signature (check all signers)
gpg --verify checksums.txt.sig.A checksums.txt

# Verify artifact integrity
sha256sum -c checksums.txt

# Verify container images
cosign verify --key release-cosign.pub ghcr.io/kscore/kscore-server:<VERSION>
```

The complete release process, including key hierarchy, quorum rules, and
threat model, is documented in [RELEASE-PLAYBOOK.md](RELEASE-PLAYBOOK.md).

## Security Scanning

The v1.0 baseline pipeline runs four scans on every PR via CI's
`security` job. Full policy + annotation conventions live in
[`docs/project/SECURITY-GOVERNANCE.md`](docs/project/SECURITY-GOVERNANCE.md)
"Security Baseline Pipeline."

| Scan | Tool | Local |
|------|------|-------|
| Secrets in git history | gitleaks | `make security-secrets` |
| Known CVEs in deps | govulncheck | `make security-vulns` |
| Static analysis (SAST) | gosec, HIGH-only, G115 excluded | `make security-sast` |
| Dependency licenses | go-licenses (strict) | `make security-licenses` |

v1.x expansion (semgrep, trivy, syft SBOM, hadolint, gosec MEDIUM
gate, G115 re-enablement) is tracked under the ROADMAP entry
*"Security baseline expansion."*

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
