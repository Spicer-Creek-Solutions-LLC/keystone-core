---
title: "Security Guide"
weight: 5
description: >
  Security hardening, authentication, TLS configuration, and compliance
---

## Overview

Security is critical for production Keystone Core deployments. This guide covers authentication methods, TLS configuration, RBAC policies, security hardening, and audit logging.

**Security Layers:**
- **Authentication**: Token-based and certificate-based auth
- **Encryption**: TLS for all communications (NATS, API, database)
- **Authorization**: RBAC with policy-based access control
- **Audit**: Comprehensive audit logging for compliance
- **Hardening**: OS and application-level security

## Authentication

Keystone Core supports multiple authentication methods.

### API Key Authentication

**Generate API Key:**
```bash
# Create API key
kscorectl api-key create --name "monitoring-system" --role "read-only"

# Output:
# Key ID: ak_1234567890
# Secret: sk_abcdef1234567890 (save this - it won't be shown again)
```

**Use API Key:**
```bash
# With CLI
export KSCORE_API_KEY="sk_abcdef1234567890"
kscorectl agent list

# With HTTP API
curl -H "Authorization: Bearer sk_abcdef1234567890" \
  http://control-plane:8080/api/v1/agents
```

**Rotate API Keys:**
```bash
# List keys
kscorectl api-key list

# Revoke old key
kscorectl api-key revoke ak_1234567890

# Create new key
kscorectl api-key create --name "monitoring-system" --role "read-only"
```

### Token-Based Authentication (JWT)

Keystone Core supports JWT (JSON Web Token) authentication with multiple signing algorithms.

**Supported Algorithms:**
| Algorithm | Type | Key Size | Use Case |
|-----------|------|----------|----------|
| HS256/384/512 | HMAC | 256+ bits | Shared secret, simple deployments |
| RS256/384/512 | RSA | 2048+ bits | Public/private key, distributed systems |
| ES256/384/512 | ECDSA | P-256/384/521 | Compact tokens, mobile clients |

**HMAC Configuration (Symmetric Key):**
```yaml
# server.yaml
auth:
  type: jwt
  jwt:
    secret: "$JWT_SECRET"        # 256-bit minimum (32+ characters)
    issuer: "kscore"             # Token issuer (validated if set)
    audience: "kscore-api"       # Token audience (validated if set)
    role_claim: "role"           # Claim containing user role
```

**RSA/ECDSA Configuration (Asymmetric Key):**
```yaml
# server.yaml
auth:
  type: jwt
  jwt:
    public_key_file: /etc/kscore/jwt-public.pem  # RSA or ECDSA public key
    issuer: "https://auth.example.com"
    audience: "kscore-api"
    role_claim: "role"
```

**JWT Claims Structure:**
```json
{
  "sub": "user-123",           // Subject (user ID)
  "name": "Alice Admin",       // Display name (optional)
  "email": "alice@example.com",// Email (optional)
  "role": "admin",             // Role: admin, operator, readonly
  "iss": "kscore",             // Issuer
  "aud": "kscore-api",         // Audience
  "exp": 1705401600,           // Expiration time
  "iat": 1705315200,           // Issued at
  "jti": "unique-token-id"     // JWT ID (optional)
}
```

**Generate Token:**
```bash
# Create user token
kscorectl auth login --username admin --password $ADMIN_PASSWORD

# Token stored in ~/.kscore/token
```

**Token Refresh:**
```bash
# Tokens auto-refresh when within 1 hour of expiry
# Manual refresh:
kscorectl auth refresh
```

**Using JWT with External Identity Providers:**
```yaml
# Integration with Auth0, Okta, Keycloak, etc.
auth:
  type: jwt
  jwt:
    public_key_file: /etc/kscore/idp-public.pem  # From your IdP
    issuer: "https://your-tenant.auth0.com/"
    audience: "kscore-api"
    role_claim: "https://kscore.io/role"  # Custom claim namespace
```

### Certificate-Based Authentication (mTLS)

**Recommended for production** - Strongest authentication method with certificate-based identity.

Keystone Core's mTLS authenticator extracts identity from client certificates and maps certificate attributes to roles using flexible pattern matching.

**Identity Extraction (in priority order):**
1. Common Name (CN) from certificate subject
2. DNS Subject Alternative Names (SANs)
3. Email Subject Alternative Names
4. URI Subject Alternative Names (including SPIFFE IDs)

**Generate Certificates:**
```bash
# Create CA
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 365 -key ca-key.pem -out ca.pem \
  -subj "/CN=Keystone Core CA"

# Create server certificate
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server.csr \
  -subj "/CN=kscore-server"
openssl x509 -req -days 365 -in server.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem

# Create admin client certificate
openssl genrsa -out admin-key.pem 4096
openssl req -new -key admin-key.pem -out admin.csr \
  -subj "/CN=admin.kscore.example.com"
openssl x509 -req -days 365 -in admin.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out admin-cert.pem

# Create operator client certificate
openssl genrsa -out operator-key.pem 4096
openssl req -new -key operator-key.pem -out operator.csr \
  -subj "/CN=operator.ops.example.com"
openssl x509 -req -days 365 -in operator.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out operator-cert.pem
```

**Server Configuration with Role Mapping:**
```yaml
# server.yaml
auth:
  type: mtls
  mtls:
    require_client_cert: true
    cert_roles:
      # Exact matches
      "admin.kscore.example.com": admin

      # Wildcard patterns (* matches one level, no dots)
      "*.admin.example.com": admin
      "*.ops.example.com": operator

      # Double wildcard (** matches anything including dots)
      "**.readonly.example.com": readonly

      # Email SANs
      "admin@example.com": admin
      "*@ops.example.com": operator

      # SPIFFE IDs (URI SANs)
      "spiffe://cluster.local/ns/admin/**": admin
      "spiffe://cluster.local/ns/ops/**": operator

      # Catch-all fallback (lowest priority)
      "**": readonly

api:
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/server-cert.pem
    key_file: /etc/kscore/certs/server-key.pem
    ca_file: /etc/kscore/certs/ca.pem
    client_auth: require
```

**Pattern Matching Rules:**

| Pattern | Matches | Does Not Match |
|---------|---------|----------------|
| `*.example.com` | `api.example.com` | `api.sub.example.com` |
| `**.example.com` | `api.sub.example.com` | `other.com` |
| `svc-*.ops.example.com` | `svc-api.ops.example.com` | `api.ops.example.com` |
| `admin@*` | `admin@internal` | `admin@external.com` (has dot) |
| `admin@**` | `admin@external.com` | `user@external.com` |
| `spiffe://cluster.local/**` | Any SPIFFE ID in cluster | Other trust domains |

**Pattern Priority:**
Patterns are matched in specificity order (most specific first):
1. Exact matches (no wildcards)
2. Single wildcards (`*`)
3. Double wildcards (`**`)
4. Longer patterns before shorter

**Client Configuration:**
```yaml
# ~/.kscore/config.yaml
api:
  url: "https://control-plane:8080"
  tls:
    cert_file: ~/.kscore/certs/client-cert.pem
    key_file: ~/.kscore/certs/client-key.pem
    ca_file: ~/.kscore/certs/ca.pem
```

**Certificate Metadata:**
The authenticator extracts and logs certificate metadata for auditing:
- `cn`: Common Name
- `serial`: Certificate serial number
- `issuer`: Issuer CN
- `org`: Organization(s)
- `dns_names`: DNS SANs (comma-separated)
- `emails`: Email SANs (comma-separated)
- `not_before`, `not_after`: Validity period

**Multi-Method Authentication:**
Combine mTLS with other methods for defense in depth:
```yaml
auth:
  type: multi  # Try methods in order
  mtls:
    require_client_cert: false  # Optional client cert
    cert_roles:
      "*.admin.example.com": admin
  apikey:
    keys:
      - id: "backup-key"
        secret_hash: "$2a$..."
        role: operator
  jwt:
    secret: "$JWT_SECRET"
```

### NATS Authentication

**Credentials File (Recommended):**
```bash
# Create NATS credentials
nats-server --genkey --user keystonecore > /etc/nats/kscore.creds
```

**NATS Configuration:**
```conf
# nats-server.conf
accounts {
  KSCORE: {
    users: [
      {
        user: "kscore"
        password: "$2a$11$..." # bcrypt hash
      }
    ]
  }
}
```

**Agent Configuration:**
```yaml
# agent.yaml
nats:
  url: "nats://nats-server:4222"
  credentials:
    file: /etc/kscore/nats.creds
```

## TLS Configuration

Encrypt all communications in production.

### TLS Security Enforcement

Keystone Core enforces TLS certificate verification by default. The `InsecureSkipVerify` option is blocked to prevent man-in-the-middle attacks.

**Development/Testing Override:**

For development or testing environments where self-signed certificates are used, you can temporarily disable verification:

```bash
# WARNING: Only use in development/testing environments
# This allows InsecureSkipVerify in TLS configurations
export KSCORE_ALLOW_INSECURE_TLS=1

# Start server/agent with insecure TLS allowed
kscore-server --config server.yaml
```

**Affected Components:**
- NATS connections (Direct, TLS, WebSocket, LeafNode strategies)
- NATS gateway connections
- Module registry clients (OCI and HTTP)
- Syslog transport (TLS mode)

**Security Warning:**
When `KSCORE_ALLOW_INSECURE_TLS=1` is set, a warning is logged:
```
WARNING: InsecureSkipVerify is enabled - this should only be used for development/testing
```

**Best Practice:** Never use `KSCORE_ALLOW_INSECURE_TLS=1` in production. Instead, configure proper CA certificates for all TLS connections.

### Control Plane TLS

**Server Configuration:**
```yaml
# server.yaml
api:
  listen: "0.0.0.0:8443"  # HTTPS port
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/server.crt
    key_file: /etc/kscore/certs/server.key
    min_version: "TLS1.2"
    cipher_suites:
      - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
      - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
      - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
```

**Let's Encrypt Certificates:**
```bash
# Install certbot
sudo apt-get install certbot

# Get certificate
sudo certbot certonly --standalone -d kscore.example.com

# Certificates in /etc/letsencrypt/live/kscore.example.com/
# - fullchain.pem (cert + chain)
# - privkey.pem (private key)

# Auto-renewal
sudo crontab -e
0 0 * * * certbot renew --quiet --post-hook "systemctl reload kscore-server"
```

### NATS TLS

**NATS Server Configuration:**
```conf
# nats-server.conf
tls {
  cert_file: "/etc/nats/certs/server.crt"
  key_file: "/etc/nats/certs/server.key"
  ca_file: "/etc/nats/certs/ca.crt"
  verify: true
  timeout: 5
}
```

**Agent Configuration:**
```yaml
# agent.yaml
nats:
  url: "tls://nats-server:4222"
  tls:
    ca_file: /etc/kscore/certs/nats-ca.crt
    cert_file: /etc/kscore/certs/agent-cert.crt
    key_file: /etc/kscore/certs/agent-key.key
    verify_server: true
```

### PostgreSQL TLS

**PostgreSQL Configuration:**
```ini
# postgresql.conf
ssl = on
ssl_cert_file = '/etc/postgresql/certs/server.crt'
ssl_key_file = '/etc/postgresql/certs/server.key'
ssl_ca_file = '/etc/postgresql/certs/ca.crt'
ssl_min_protocol_version = 'TLSv1.2'
```

**pg_hba.conf:**
```
# Require SSL for all connections
hostssl    kscore      kscore      10.0.0.0/8              md5
```

**Client Configuration:**
```yaml
# server.yaml
storage:
  postgresql:
    host: "postgres-server"
    sslmode: "require"  # or "verify-ca" or "verify-full"
    sslcert: "/etc/kscore/certs/client.crt"
    sslkey: "/etc/kscore/certs/client.key"
    sslrootcert: "/etc/kscore/certs/ca.crt"
```

## Webhook Security

Webhooks are an external ingress point for GitOps events (ArgoCD, Flux, GitHub, GitLab). Securing webhook endpoints is critical to prevent unauthorized event injection and denial of service attacks.

### Trust Boundaries

Webhooks cross the **TB1: External Network → Control Plane API** trust boundary defined in the [Threat Model](/docs/concepts/threat-model/). Key security considerations:

| Threat | Risk | Mitigation |
|--------|------|------------|
| **Spoofing** | Attacker sends fake deployment events | HMAC signature verification |
| **Tampering** | Attacker modifies webhook payload in transit | TLS encryption, signature verification |
| **Information Disclosure** | Webhook payloads contain sensitive data | TLS encryption, audit logging |
| **Denial of Service** | Flood of webhook requests | Rate limiting, IP allowlisting |
| **Injection** | Malicious payloads trigger unintended actions | Input validation, event sanitization |

### Webhook Authentication

Keystone Core supports three authentication methods for webhooks:

#### HMAC Signature Verification (Recommended)

HMAC-SHA256 signature verification ensures webhooks originate from trusted sources:

```yaml
# server.yaml
webhooks:
  enabled: true
  addr: ":8080"
  path: "/webhooks"
  auth:
    type: hmac
    secret: "${WEBHOOK_SECRET}"  # Shared secret with webhook source
```

**How it works:**
1. Webhook source computes HMAC-SHA256 of request body using shared secret
2. Signature sent in `X-Hub-Signature-256` header (GitHub format)
3. Keystone Core verifies signature before processing

**Configuring webhook sources:**

| Source | Configuration |
|--------|---------------|
| **GitHub** | Repository Settings → Webhooks → Secret |
| **GitLab** | Project Settings → Webhooks → Secret Token |
| **ArgoCD** | ArgoCD notifications → Webhook secret |
| **Flux** | Notification Controller → Receiver secret |

**Generate a secure secret:**
```bash
# Generate 256-bit secret
openssl rand -base64 32

# Store in environment (never commit to git)
export WEBHOOK_SECRET="$(openssl rand -base64 32)"
```

#### Bearer Token Authentication

Simple token-based authentication for internal services:

```yaml
# server.yaml
webhooks:
  auth:
    type: bearer
    token: "${WEBHOOK_TOKEN}"
```

**Usage:**
```bash
# Webhook source includes Authorization header
curl -X POST \
  -H "Authorization: Bearer ${WEBHOOK_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"event": "deployment"}' \
  https://kscore.example.com/webhooks
```

#### No Authentication (Development Only)

**⚠️ WARNING:** Never use in production.

```yaml
# server.yaml (development only)
webhooks:
  auth:
    type: none
```

### Rate Limiting

Protect webhook endpoints from denial of service attacks:

```yaml
# server.yaml
webhooks:
  rate_limiting:
    enabled: true
    requests_per_minute: 60    # Max 60 webhooks per minute per source
    burst: 10                  # Allow burst of 10 requests
```

**Per-source rate limiting:**
```yaml
webhooks:
  rate_limiting:
    enabled: true
    by_source: true            # Rate limit per source IP
    requests_per_minute: 100
    burst: 20
```

**Response when rate limited:**
- HTTP 429 Too Many Requests
- `Retry-After` header indicates when to retry

### IP Allowlisting

Restrict webhook sources to known IP ranges:

```yaml
# server.yaml
webhooks:
  allowed_ips:
    # GitHub webhook IPs (check GitHub docs for current ranges)
    - "192.30.252.0/22"
    - "185.199.108.0/22"
    - "140.82.112.0/20"
    - "143.55.64.0/20"
    # GitLab.com webhook IPs
    - "35.231.145.151/32"
    - "34.74.90.64/28"
    # Internal services
    - "10.0.0.0/8"
```

**Dynamic IP lookup for cloud services:**
```bash
# GitHub webhook IPs (updated regularly)
curl -s https://api.github.com/meta | jq '.hooks[]'

# Store in config management, update weekly
```

### TLS Configuration

**Always use HTTPS for webhook endpoints:**

```yaml
# server.yaml
webhooks:
  enabled: true
  addr: ":8443"                # HTTPS port
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/webhook.crt
    key_file: /etc/kscore/certs/webhook.key
    min_version: "TLS1.2"
```

**Let's Encrypt with automatic renewal:**
```bash
certbot certonly --standalone -d webhooks.kscore.example.com
```

### Event Ingress Security

Control which event types are processed:

```yaml
# server.yaml
webhooks:
  handlers:
    - argocd    # Accept ArgoCD webhooks
    - flux      # Accept Flux webhooks
    - github    # Accept GitHub webhooks
    # - gitlab  # Disabled: not needed
```

**Event validation:**
- All webhook payloads validated against expected schema
- Unknown event types logged but not processed
- Payload size limited (default: 1MB)

```yaml
webhooks:
  max_payload_size: "1MB"      # Reject oversized payloads
  validate_schema: true        # Reject malformed payloads
```

### Audit Logging

Track all webhook activity for security monitoring:

```yaml
# server.yaml
webhooks:
  audit:
    enabled: true
    log_payloads: false        # Don't log payload contents (may contain secrets)
    log_headers: true          # Log request headers
```

**Audit log entries include:**
- Timestamp
- Source IP
- Webhook type (argocd, flux, github, gitlab)
- Event type
- Authentication result
- Processing result

**Query webhook audit logs:**
```bash
kscorectl audit query \
  --type "gitops.webhook.*" \
  --since 24h \
  --result failed
```

### Monitoring and Alerting

**Prometheus metrics:**
```yaml
# Alert on webhook authentication failures
- alert: WebhookAuthFailures
  expr: rate(kscore_webhook_auth_failures_total[5m]) > 1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High rate of webhook authentication failures"
```

**Key metrics:**
| Metric | Description |
|--------|-------------|
| `kscore_webhook_requests_total` | Total webhook requests by type and status |
| `kscore_webhook_auth_failures_total` | Authentication failures by type |
| `kscore_webhook_processing_duration_seconds` | Webhook processing latency |
| `kscore_webhook_rate_limited_total` | Requests rejected due to rate limiting |

### Production Checklist

Before exposing webhooks to the internet:

- [ ] **HMAC authentication enabled** with strong secret (256+ bits)
- [ ] **TLS enabled** with valid certificate (not self-signed)
- [ ] **Rate limiting enabled** with appropriate limits
- [ ] **IP allowlisting configured** for known webhook sources
- [ ] **Audit logging enabled** for security monitoring
- [ ] **Alerting configured** for authentication failures
- [ ] **Firewall rules** restrict webhook port to necessary sources
- [ ] **Secret rotation plan** documented and tested
- [ ] **Separate endpoint** for webhooks (not on admin API port)

### Example: Secure GitHub Webhook Configuration

**1. Generate secret:**
```bash
export GITHUB_WEBHOOK_SECRET="$(openssl rand -base64 32)"
```

**2. Configure Keystone Core:**
```yaml
# server.yaml
webhooks:
  enabled: true
  addr: ":8443"
  path: "/github/webhooks"
  tls:
    enabled: true
    cert_file: /etc/kscore/certs/webhook.crt
    key_file: /etc/kscore/certs/webhook.key
  auth:
    type: hmac
    secret: "${GITHUB_WEBHOOK_SECRET}"
  handlers:
    - github
  allowed_ips:
    - "192.30.252.0/22"
    - "185.199.108.0/22"
    - "140.82.112.0/20"
    - "143.55.64.0/20"
  rate_limiting:
    enabled: true
    requests_per_minute: 60
    burst: 10
  audit:
    enabled: true
```

**3. Configure GitHub repository:**
- Repository Settings → Webhooks → Add webhook
- Payload URL: `https://webhooks.kscore.example.com/github/webhooks`
- Content type: `application/json`
- Secret: `${GITHUB_WEBHOOK_SECRET}` value
- Events: Select specific events (deployment, push)

**4. Verify webhook delivery:**
```bash
# Check webhook stats
curl -s https://kscore.example.com/stats | jq '.webhooks'

# Check recent webhook events
kscorectl audit query --type "gitops.github.*" --since 1h
```

## RBAC (Role-Based Access Control)

Define fine-grained access control policies.

### Built-in Roles

**admin:**
- Full system access
- Create/modify policies
- Manage users and roles
- Execute any command

**operator:**
- Deploy applications
- Execute commands
- Apply state configurations
- View all resources

**read-only:**
- View agents and resources
- Query metrics and logs
- No write permissions

**agent:**
- Agent registration only
- Heartbeat and telemetry
- No user-facing permissions

### Custom Roles

**Define Custom Role:**
```yaml
# roles.yaml
- name: deployment-manager
  description: "Can deploy applications but not manage infrastructure"
  permissions:
    - resource: "state/*"
      actions: ["apply", "check"]
    - resource: "job/*"
      actions: ["create", "read", "cancel"]
    - resource: "agent/*"
      actions: ["read"]
    - resource: "event/*"
      actions: ["read"]
```

**Assign Role to User:**
```bash
kscorectl rbac assign --user alice --role deployment-manager
```

### Policy-Based Access Control

**OPA Policy Example:**
```rego
# rbac.rego
package kscore.rbac

import data.users
import data.roles

# Allow admins everything
allow {
    input.user.role == "admin"
}

# Allow operators to execute commands
allow {
    input.user.role == "operator"
    input.action == "exec.run"
}

# Restrict state apply to specific datacenters
allow {
    input.user.role == "deployment-manager"
    input.action == "state.apply"
    input.resource.datacenter == input.user.datacenter
}

# Deny destructive operations in production
deny {
    input.resource.environment == "production"
    input.action in ["agent.delete", "state.delete"]
    not input.user.role == "admin"
}
```

**Apply Policy:**
```bash
kscorectl policy create rbac --file rbac.rego --enforce
```

### Audit RBAC Changes

```bash
# List all roles
kscorectl rbac list-roles

# List role assignments
kscorectl rbac list-assignments

# View audit log
kscorectl policy audit --category rbac --since 24h
```

## Security Hardening

### Operating System Hardening

**Firewall Configuration:**
```bash
# Allow only necessary ports
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Control plane
sudo ufw allow 8080/tcp  # API (or 8443 for HTTPS)

# NATS
sudo ufw allow from 10.0.0.0/8 to any port 4222 proto tcp

# PostgreSQL
sudo ufw allow from 10.0.0.0/8 to any port 5432 proto tcp

# SSH (restrict to management network)
sudo ufw allow from 10.0.1.0/24 to any port 22 proto tcp

sudo ufw enable
```

**Disable Unnecessary Services:**
```bash
# List running services
systemctl list-units --type=service --state=running

# Disable unneeded services
sudo systemctl disable bluetooth
sudo systemctl disable cups
```

**File System Permissions:**
```bash
# Restrict config files
sudo chmod 600 /etc/kscore/server.yaml
sudo chown kscore:kscore /etc/kscore/server.yaml

# Restrict private keys
sudo chmod 400 /etc/kscore/certs/*.key
sudo chown kscore:kscore /etc/kscore/certs/*.key

# Restrict database files
sudo chmod 700 /var/lib/kscore
sudo chown kscore:kscore /var/lib/kscore
```

**SELinux/AppArmor:**
```bash
# Enable SELinux
sudo setenforce 1

# Create custom SELinux policy for Keystone Core
# (beyond scope - consult SELinux documentation)
```

### Application Hardening

**Principle of Least Privilege:**
```bash
# Run as non-root user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin kscore

# Systemd service restrictions
[Service]
User=kscore
Group=kscore
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/kscore
```

**API Rate Limiting:**
```yaml
# server.yaml
api:
  rate_limiting:
    enabled: true
    requests_per_minute: 100
    burst: 20
```

**Authentication Rate Limiting:**

Keystone Core automatically rate-limits failed authentication attempts to prevent brute-force attacks:

```yaml
# server.yaml
auth:
  rate_limiting:
    enabled: true           # Enable auth rate limiting (default: true)
    max_failures: 5         # Lock out after 5 failed attempts (default: 5)
    lockout_duration: 15m   # Lockout duration (default: 15 minutes)
    cleanup_interval: 5m    # Cleanup expired entries (default: 5 minutes)
```

When a client exceeds the failure threshold:
- Further authentication attempts return gRPC `ResourceExhausted` status
- The lockout includes time remaining in the error message
- Successful authentication resets the failure counter
- Client identification uses peer IP or X-Forwarded-For/X-Real-IP headers

**Monitor rate limiting:**
```bash
# Check rate limiter stats via metrics
curl -s http://control-plane:8080/metrics | grep auth_rate
```

**Input Validation:**
- All API inputs validated
- YAML parsing with size limits
- Command injection prevention
- SQL injection prevention (parameterized queries with allowlisted ORDER BY columns)

**Command Execution Policy:**

Keystone Core enforces command execution policies to prevent shell injection and dangerous commands:

```yaml
# server.yaml
execution:
  policy:
    mode: normal  # strict, normal, or permissive (deprecated)
```

| Mode | Description |
|------|-------------|
| `strict` | Only explicitly allowlisted commands permitted |
| `normal` | Blocks dangerous patterns but allows most commands (recommended) |
| `permissive` | **DEPRECATED** - Minimal restrictions, will be removed |

**Deprecation Warning:**
`ExecutionModePermissive` is deprecated and will be removed in a future release. If you're using permissive mode, migrate to `normal` mode:

```yaml
# Before (deprecated)
execution:
  policy:
    mode: permissive

# After (recommended)
execution:
  policy:
    mode: normal
```

The deprecation warning is logged once at startup when permissive mode is detected.

**Secrets Management:**
```yaml
# Use external secrets manager
secrets:
  backend: vault
  vault:
    address: https://vault.example.com
    token_file: /etc/kscore/vault-token

# Never store secrets in config files
database:
  password: "{{ vault.secret('database/postgres/password') }}"
```

### Network Segmentation

**Recommended Network Layout:**

```mermaid
flowchart TB
    subgraph M["Management Network (10.0.1.0/24)"]
        M1[Bastion/Jump host]
        M2[Admin workstations]
    end

    subgraph C["Control Plane Network (10.0.2.0/24)"]
        C1[Keystone Core servers]
        C2[NATS cluster]
        C3[PostgreSQL]
    end

    subgraph A["Agent Network (10.0.10.0/16)"]
        A1[Agents on managed nodes]
    end

    M --> C
    C --> A
```

**Firewall Rules:**
- Management → Control Plane: SSH, API (8080/8443)
- Control Plane → Control Plane: NATS (4222, 6222), PostgreSQL (5432)
- Agents → Control Plane: NATS (4222) only
- Control Plane → Agents: Blocked (agents initiate connections)

## Audit Logging

Track all security-relevant events.

### Enable Audit Logging

**Configuration:**
```yaml
# server.yaml
audit:
  enabled: true
  log_file: /var/log/kscore/audit.log
  log_level: info
  events:
    - authentication
    - authorization
    - state.apply
    - policy.evaluation
    - user.create
    - user.delete
    - role.assign
```

### Audit Log Format

```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "event_type": "state.apply",
  "user": "alice",
  "source_ip": "10.0.1.50",
  "action": "apply",
  "resource": "web-server.yaml",
  "target": "role:web",
  "result": "success",
  "correlation_id": "abc-123"
}
```

### Query Audit Logs

**With jq:**
```bash
# All failed operations
cat /var/log/kscore/audit.log | jq 'select(.result == "failed")'

# All actions by user
cat /var/log/kscore/audit.log | jq 'select(.user == "alice")'

# State applications in last hour
cat /var/log/kscore/audit.log | \
  jq 'select(.event_type == "state.apply" and .timestamp > "2024-01-15T09:00:00Z")'
```

**With Elasticsearch:**
```json
{
  "query": {
    "bool": {
      "must": [
        { "match": { "event_type": "state.apply" }},
        { "range": { "timestamp": { "gte": "now-24h" }}}
      ]
    }
  }
}
```

### Audit Log Retention

```yaml
# server.yaml
audit:
  retention:
    max_age: "365d"  # Compliance requirement: 1 year
    max_size: "10GB"
```

**Off-site Archival:**
```bash
# Daily export to S3
aws s3 cp /var/log/kscore/audit.log \
  s3://compliance-logs/kscore/audit-$(date +%Y-%m-%d).log

# Compress and encrypt
gpg --encrypt --recipient compliance@example.com audit.log
aws s3 cp audit.log.gpg s3://compliance-logs/kscore/
```

### Policy Audit (Persistent Store)

Policy evaluations are audited separately with support for persistent storage, configurable retention, and automatic sensitive data redaction.

**Persistent SQLite Audit Store:**
```yaml
# server.yaml
policy:
  audit:
    # Use persistent SQLite storage (recommended for production)
    store: sqlite
    path: /var/lib/kscore/policy-audit.db

    # Retention policy
    retention:
      max_age: 90d      # Keep entries for 90 days
      max_count: 100000 # Maximum 100k entries
      interval: 1h      # Run retention cleanup hourly

    # Sensitive data redaction
    redaction:
      # Redact metadata keys containing these strings (case-insensitive)
      metadata_keys:
        - password
        - secret
        - token
        - key
        - credential
        - api_key
        - authorization

      # Regex patterns to redact anywhere in values
      patterns:
        - 'AKIA[0-9A-Z]{16}'                    # AWS access keys
        - 'eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' # JWT tokens

      # Partially redact user identifiers (shows first 2 chars)
      redact_user: true
```

**Policy Audit Entry Format:**
```json
{
  "id": "audit-1705312345678",
  "timestamp": "2024-01-15T10:30:45Z",
  "policy_id": "require-labels",
  "policy_name": "Require Labels",
  "policy_type": "opa",
  "resource_type": "deployment",
  "allowed": false,
  "duration_ns": 1500000,
  "enforcement_mode": "enforce",
  "user": "al***",
  "action": "create",
  "violations": [
    {
      "rule": "require-owner-label",
      "message": "Deployment missing required label: owner",
      "severity": "high",
      "path": "metadata.labels"
    }
  ],
  "metadata": {
    "namespace": "production",
    "api_key": "[REDACTED]"
  }
}
```

**Query Policy Audit:**
```bash
# List policy evaluations
kscorectl policy audit list --limit 100

# Filter by policy
kscorectl policy audit list --policy-id require-labels

# Filter by result
kscorectl policy audit list --denied --since 24h

# Get summary
kscorectl policy audit summary --since 7d
```

**Retention Policy Options:**

| Setting | Default | Description |
|---------|---------|-------------|
| `max_age` | 90d | Delete entries older than this |
| `max_count` | 100000 | Keep only this many entries (newest) |
| `interval` | 1h | How often to run retention cleanup |
| `min_severity` | - | Keep only entries with violations at or above this severity |

**Redaction Configuration:**

Redaction automatically sanitizes sensitive data before storing audit entries:

1. **Metadata Key Redaction**: Any metadata key containing configured strings (e.g., `password`, `token`) has its value replaced with `[REDACTED]`.

2. **Pattern Redaction**: Regex patterns match sensitive data anywhere in string values (e.g., AWS keys, JWT tokens).

3. **User Redaction**: When enabled, user identifiers are partially masked (e.g., `administrator` → `ad***`).

**Example Redaction:**
```yaml
# Original audit entry metadata
metadata:
  db_password: "supersecret123"
  aws_access_key: "AKIAIOSFODNN7EXAMPLE"
  api_token: "sk_live_abc123"
  environment: "production"

# After redaction
metadata:
  db_password: "[REDACTED]"
  aws_access_key: "[REDACTED]"
  api_token: "[REDACTED]"
  environment: "production"
```

**Best Practices:**
- Use persistent storage (`sqlite`) for production deployments
- Set retention to meet compliance requirements (SOC 2: 1 year, HIPAA: 6 years)
- Enable `redact_user` if user identifiers are considered sensitive
- Add organization-specific patterns to redact internal credentials

## Compliance

### SOC 2 Compliance

**Access Control:**
- [x] Role-based access control implemented
- [x] Audit logging of all access
- [x] MFA for administrative access
- [x] Regular access reviews

**Data Security:**
- [x] Encryption in transit (TLS)
- [x] Encryption at rest (database)
- [x] Secrets management (Vault)
- [x] Data retention policies

**Change Management:**
- [x] All changes tracked in audit log
- [x] State configurations version controlled
- [x] Approval workflow for production changes

### HIPAA Compliance

**Required:**
- Encryption: TLS 1.2+ for all communications
- Access Controls: RBAC with audit logging
- Audit Trails: 6-year retention minimum
- Backup/Recovery: Daily backups, quarterly DR tests

**Configuration:**
```yaml
# server.yaml - HIPAA compliance settings
audit:
  enabled: true
  retention:
    max_age: "2190d"  # 6 years

api:
  tls:
    min_version: "TLS1.2"

storage:
  encryption:
    enabled: true
    algorithm: "AES-256-GCM"
```

### GDPR Compliance

**Data Subject Rights:**
```bash
# Right to access
kscorectl audit query --user alice --output json > alice-data.json

# Right to erasure ("right to be forgotten")
kscorectl user delete alice --purge-data

# Data portability
kscorectl export --user alice --format json
```

**Data Retention:**
```yaml
# server.yaml
data_retention:
  events: "30d"  # Minimize retention
  audit_logs: "365d"  # Compliance requirement
  metrics: "90d"
```

## Secret Management

### HashiCorp Vault Integration

**Configuration:**
```yaml
# server.yaml
secrets:
  backend: vault
  vault:
    address: "https://vault.example.com:8200"
    auth_method: "approle"
    role_id: "$VAULT_ROLE_ID"
    secret_id: "$VAULT_SECRET_ID"
    paths:
      database: "secret/data/kscore/database"
      nats: "secret/data/kscore/nats"
```

**Store Secrets:**
```bash
# Database password
vault kv put secret/kscore/database password="secure-db-password"

# NATS credentials
vault kv put secret/kscore/nats username="kscore" password="secure-nats-password"

# API keys
vault kv put secret/kscore/api-keys monitoring="sk_monitoring_key"
```

**Rotate Secrets:**
```bash
# Update secret in Vault
vault kv put secret/kscore/database password="new-secure-password"

# Keystone Core auto-reloads secrets every 5 minutes
# Or trigger immediate reload
kscorectl secrets reload
```

### Kubernetes Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kscore-secrets
  namespace: kscore
type: Opaque
data:
  postgres-password: <base64-encoded>
  nats-password: <base64-encoded>
  api-key: <base64-encoded>
```

```yaml
# Use in deployment
spec:
  containers:
  - name: kscore-server
    env:
    - name: KSCORE_POSTGRES_PASSWORD
      valueFrom:
        secretKeyRef:
          name: kscore-secrets
          key: postgres-password
```

## Agent Command Security

Agents enforce security policies on command execution to prevent unauthorized or dangerous operations.

### Command Authorization

Configure authorization to control who can execute commands on agents:

```yaml
# agent.yaml
security:
  authorization:
    enabled: true                     # Enable authorization checks
    shared_secret: "${COMMAND_SECRET}" # HMAC secret for signing
    require_signature: true           # Require signed commands
    allowed_principals:               # Principals allowed to execute
      - "admin"
      - "operator"
      - "ci-system"
```

**How Authorization Works:**
1. Control plane sends commands with `X-Keystone-Principal` header
2. Agent validates principal against `allowed_principals` list
3. If `require_signature` is true, validates HMAC signature in `X-Keystone-Signature` header
4. Commands from unauthorized principals are rejected

### Command Filtering

Filter commands to prevent dangerous operations:

```yaml
# agent.yaml
security:
  command_filter:
    mode: "blocklist"               # allowlist (strict) or blocklist (permissive)

    # Blocklist mode: block specific commands
    blocklist:
      - "rm"
      - "shutdown"
      - "reboot"

    # Blocked patterns (always applied regardless of mode)
    blocked_patterns:
      - ';\s*rm\s+-rf\s+/'          # Prevent shell injection: ; rm -rf /
      - '>\s*/dev/sd[a-z]'          # Prevent writing to block devices
      - 'mkfs\.'                    # Prevent filesystem creation
      - 'dd\s+.*of=/dev/'           # Prevent dd to devices

    # Exempt specific commands from blocked_patterns
    # Use when legitimate operations need blocked commands
    exempt_commands:
      - "mkfs.*"                    # Allow mkfs for disk provisioning
      - "/sbin/mkfs*"               # Allow full path mkfs
      - "dd"                        # Allow dd for specific use cases
```

**Allowlist Mode (Most Secure):**
```yaml
security:
  command_filter:
    mode: "allowlist"
    allowlist:
      - "systemctl"
      - "apt-get"
      - "yum"
      - "docker"
      - "/usr/local/bin/*"          # Allow custom scripts
```

### Blocked Environment Variables

Prevent library injection attacks by blocking dangerous environment variables:

```yaml
security:
  command_filter:
    block_env_overrides: true
    blocked_env_vars:
      - "LD_PRELOAD"                # Linux library preload
      - "LD_LIBRARY_PATH"           # Linux library path
      - "DYLD_INSERT_LIBRARIES"     # macOS library injection
      - "PYTHONPATH"                # Python module injection
      - "RUBYLIB"                   # Ruby module injection
      - "PERL5LIB"                  # Perl module injection
      - "NODE_PATH"                 # Node.js module injection
```

### Embedded NATS Security

When running embedded NATS on agents, security is enforced:

**Default Behavior:**
- Embedded NATS is **disabled by default** (requires explicit configuration)
- When enabled, binds to `0.0.0.0` (all interfaces)
- TLS and authentication are **required** for non-localhost binding

**Enable Embedded NATS via CLI:**
```bash
# Enable with TLS (required for external access)
kscore-agent config enable-embedded-nats \
  --host 0.0.0.0 \
  --port 4222 \
  --restart

# Show current configuration
kscore-agent config show

# Disable and connect to external NATS
kscore-agent config disable-embedded-nats \
  --nats-url "nats://external-cluster:4222" \
  --restart
```

**Secure Embedded NATS Configuration:**
```yaml
# agent.yaml
nats:
  mode: "embedded"
  embedded:
    host: "0.0.0.0"                 # External access
    port: 4222
    tls:                            # REQUIRED for non-localhost
      cert_file: "/etc/kscore/nats.crt"
      key_file: "/etc/kscore/nats.key"
      ca_file: "/etc/kscore/ca.crt"
      verify: true
    auth:                           # REQUIRED for non-localhost
      token: "${NATS_TOKEN}"
```

**Localhost-only Mode (Development):**
```yaml
# agent.yaml - no TLS/auth required for localhost
nats:
  mode: "embedded"
  embedded:
    host: "127.0.0.1"               # Localhost only - no TLS required
    port: 4222
```

### Agent Security Checklist

- [ ] **NATS mode explicitly configured** (no auto-default to embedded)
- [ ] **TLS enabled** for embedded NATS with external access
- [ ] **Authentication configured** for embedded NATS
- [ ] **Command authorization enabled** in production
- [ ] **Blocked patterns configured** for dangerous commands
- [ ] **Environment variable blocking enabled**
- [ ] **Exempt commands reviewed** for legitimate use cases

## Module Security

Keystone Core's module system uses capability-based security to ensure modules cannot access system resources without explicit authorization.

### Capability-Based Access Control

Modules operate under a **no ambient authority** model:

- **No default permissions**: Modules cannot access files, network, or execute commands by default
- **Explicit capability grants**: Each capability must be declared in the module manifest
- **Operator override**: Operators can restrict capabilities beyond what modules declare
- **Capability locking**: Lock module capabilities to prevent escalation on updates

### Sandboxed Execution

Modules run in isolated execution environments:

| Runtime | Isolation Mechanism | Resource Limits |
|---------|---------------------|-----------------|
| Starlark | Deterministic execution, no I/O by default | Step limit, timeout |
| WASM | Memory isolation, no syscalls | Memory limit, fuel metering |

### Deploying Capability Policy

Create a capability policy to restrict module permissions:

```yaml
# /etc/kscore/capability-policy.yaml
schema_version: 1

defaults:
  trust: none
  denied_capabilities:
    - exec           # Deny command execution by default
    - secrets.write  # Deny secret modification

  capabilities:
    fs.write:
      denied_paths:
        - /etc/**
        - /root/**
        - /usr/**
        - /bin/**

modules:
  # Trust internal modules
  internal/approved-deployer:
    trust: full

  # Heavily restrict third-party modules
  community/external-reporter:
    trust: none
    denied_capabilities:
      - exec
      - fs.write
      - secrets.*
    capabilities:
      http.get:
        allowed_domains:
          - api.approved-service.com
        rate_limit: 10
```

### Locking Module Capabilities

Prevent capability escalation across module updates:

```bash
# Lock a module's capabilities
kscorectl module lock my-org/production-module \
  --reason "Production deployment - capabilities frozen"

# View lock status
kscorectl module lock show my-org/production-module

# Unlock (requires admin role)
kscorectl module unlock my-org/production-module
```

When a locked module is updated:
- New capabilities are **blocked**
- Removed capabilities are **allowed**
- More restrictive configurations are **allowed**

### Module Security Audit

```bash
# List all modules and their capabilities
kscorectl module list --show-capabilities

# Show capability policy evaluation for a module
kscorectl module capabilities show my-org/web-deployer

# Export capability grants for compliance review
kscorectl module capabilities export --format csv > capabilities-report.csv
```

### Module Security Checklist

- [ ] Capability policy deployed (`/etc/kscore/capability-policy.yaml`)
- [ ] `exec` capability denied by default
- [ ] Third-party modules have explicit policy entries
- [ ] Production modules are capability-locked
- [ ] Allowed paths/domains scoped to necessary resources
- [ ] Rate limits configured for HTTP capabilities

For complete module security documentation, see [Module System & Security](/docs/concepts/modules/).

## Security Checklist

### Pre-Production

- [ ] TLS enabled for all connections (API, NATS, database)
- [ ] Certificate-based authentication configured
- [ ] RBAC policies defined and tested
- [ ] Firewall rules restrict access to necessary ports only
- [ ] Secrets stored in external secrets manager (Vault)
- [ ] Audit logging enabled with off-site archival
- [ ] File permissions restricted (600 for configs, 400 for keys)
- [ ] Services run as non-root user
- [ ] SELinux/AppArmor policies applied
- [ ] Vulnerability scanning completed (container images, OS packages)
- [ ] Penetration testing performed
- [ ] Disaster recovery plan tested

### Ongoing Operations

- [ ] Regular security updates applied (monthly)
- [ ] Access reviews conducted (quarterly)
- [ ] Audit logs reviewed (weekly)
- [ ] Secrets rotated (quarterly)
- [ ] Backup verification (weekly)
- [ ] Vulnerability scans (monthly)
- [ ] Security training for team (annually)

### Incident Response

1. **Detect**: Monitor audit logs and alerts
2. **Contain**: Isolate affected systems
3. **Eradicate**: Remove threat and patch vulnerability
4. **Recover**: Restore from backups if needed
5. **Review**: Post-incident review and remediation

**Incident Response Plan:**
```bash
# 1. Isolate compromised node
kscorectl agent quarantine web-05

# 2. Collect forensic data
kscorectl exec run "tar -czf /tmp/forensics.tar.gz /var/log /etc" --target "web-05"

# 3. Revoke credentials
kscorectl api-key revoke-all --user compromised-user

# 4. Force password reset
kscorectl user reset-password --all

# 5. Review audit logs
kscorectl audit query --since "incident-start-time" --severity critical
```

## Security Tools

**Vulnerability Scanning:**
```bash
# Scan Docker images
trivy image kscore/server:latest

# Scan OS packages
lynis audit system

# Scan network services
nmap -sV control-plane
```

**Intrusion Detection:**
```bash
# Install OSSEC
sudo apt-get install ossec-hids

# Monitor logs for suspicious activity
tail -f /var/ossec/logs/alerts/alerts.log
```

**Secrets Detection:**
```bash
# Scan git repo for accidentally committed secrets
trufflehog filesystem /etc/kscore
```

## Vulnerability Management

### CI Security Scanning

Keystone Core uses automated security scanning in CI that **blocks** merges when vulnerabilities are detected:

| Tool | Purpose | Configuration |
|------|---------|---------------|
| **govulncheck** | Go vulnerability database | Update dependencies to fix |
| **gosec** | Static security analysis | `.gosec.yaml` for waivers |

**Security scans are blocking by default.** Failed scans prevent CI from passing.

### Handling Vulnerability Findings

**Option 1: Fix the Vulnerability (Preferred)**
```bash
# Update vulnerable dependency
go get -u github.com/vulnerable/package@latest
go mod tidy

# Verify vulnerability is fixed
govulncheck ./...
```

**Option 2: Inline Waiver with Nosec**

For gosec findings where the code is safe despite the warning:
```go
// #nosec G104 -- error intentionally ignored for cleanup operations
// Justification: File removal errors cannot be meaningfully handled during shutdown
_ = os.Remove(tempFile)
```

**Nosec annotation requirements:**
- Include the rule ID (e.g., `G104`)
- Provide justification comment
- Document why the code is safe

**Option 3: Global Waiver in .gosec.yaml**

For patterns that are safe across the codebase:
```yaml
# .gosec.yaml
# G104: Errors unhandled - excluded for logging cleanup
# Justification: Logger Close() errors cannot be recovered
# Tracked: KSCORE-1234
# Review: 2025-06-01
rules:
  - G104
```

**Waiver requirements:**
- Justification for why the code is safe
- Tracking reference (issue/ticket)
- Review date for periodic reassessment

### Accepted Vulnerability Process

1. **Document the risk** in this security guide
2. **Create a tracking issue** for remediation
3. **Set a review date** (max 90 days)
4. **Get security team approval** before merging waiver

### Common Gosec Rules

| Rule | Description | Common Fix |
|------|-------------|------------|
| G101 | Hardcoded credentials | Use secrets manager |
| G104 | Errors not checked | Handle or explicitly ignore |
| G107 | URL from taint input | Validate/sanitize URLs |
| G201-G203 | SQL injection | Use parameterized queries |
| G204 | Command injection | Avoid shell, use exec.Command |
| G301-G307 | File permissions | Use restrictive permissions |
| G401-G406 | Weak crypto | Use modern algorithms |

### Dependency Vulnerability Response

When govulncheck reports a vulnerability:

1. **Check if the vulnerable code path is used:**
   ```bash
   govulncheck -show verbose ./...
   ```

2. **Update the dependency if available:**
   ```bash
   go get -u github.com/package@latest
   ```

3. **If no fix available, document in security.md:**
   ```markdown
   ### Accepted Vulnerabilities

   | CVE | Package | Severity | Justification | Review Date |
   |-----|---------|----------|---------------|-------------|
   | CVE-2024-XXXX | example/pkg | Medium | Vulnerable code path not used | 2025-06-01 |
   ```

4. **Monitor for upstream fixes** via GitHub security advisories

## See Also

- [Threat Model](/docs/concepts/threat-model/) - Security threat model and STRIDE analysis
- [Deployment Guide](deployment/) - Secure deployment patterns
- [Monitoring Guide](monitoring/) - Security monitoring
- [Maintenance Guide](maintenance/) - Secure backup procedures
- [Troubleshooting Guide](troubleshooting/) - Security issue resolution
- [Policy Concepts](/docs/concepts/policy/) - Policy enforcement
