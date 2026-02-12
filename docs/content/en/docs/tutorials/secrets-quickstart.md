---
title: "Secrets Management Quickstart"
weight: 20
description: "Get started with Keystone Core's secrets management in under 10 minutes."
---

## Overview

This tutorial walks you through setting up secrets management, storing your first secrets, and injecting them into workloads.

## Prerequisites

- Keystone Core server running
- `kscorectl` CLI installed
- At least one agent connected

## Step 1: Check Secrets System Health

Verify the secrets system is ready:

```bash
kscorectl secrets backends
```

Expected output:

```
BACKEND    STATUS    LATENCY
vault      healthy   12ms
```

## Step 2: Store Your First Secret

Store a database password using your backend's native CLI:

```bash
# Example using HashiCorp Vault
vault kv put secret/database/production/password value="super-secret-password-123"
```

Verify it's accessible through Keystone Core:

```bash
kscorectl secrets get database/production/password
```

## Step 3: Retrieve the Secret

Verify the secret was stored:

```bash
kscorectl secrets get database/production/password
```

Output:

```
Key      : database/production/password
Value    : super-secret-password-123
Version  : 1
Created  : 2024-01-15T10:30:00Z
```

## Step 4: Store Multiple Fields

Store a secret with multiple fields using your backend's native CLI:

```bash
# Example using HashiCorp Vault
vault kv put secret/database/production/credentials \
  username="app_user" password="secure-password" host="db.example.com" port="5432"
```

Retrieve specific fields:

```bash
# Get just the password
kscorectl secrets get database/production/credentials --field password

# Get all fields as JSON
kscorectl secrets get database/production/credentials -o json
```

## Step 5: Create a Workload with Secrets

Create a workload that uses your secrets.

**workload.yaml:**

```yaml
workload:
  name: myapp

  secrets:
    # Inject as environment variables
    - path: database/production/credentials
      env:
        DB_HOST:
          key: host
        DB_PORT:
          key: port
        DB_USER:
          key: username
        DB_PASSWORD:
          key: password

    # Also write to a file
    - path: database/production/credentials
      files:
        - destination: /etc/myapp/db-config.yaml
          template: |
            database:
              host: {{ .host }}
              port: {{ .port }}
              username: {{ .username }}
              password: {{ .password }}
          mode: 0600
```

Apply the workload:

```bash
kscorectl state apply workload.yaml
```

## Step 6: Verify Secret Injection

Check that the agent received the secrets:

```bash
# Check environment variables
kscorectl exec run --target myapp -- env | grep DB_

# Check the config file
kscorectl exec run --target myapp -- cat /etc/myapp/db-config.yaml
```

Expected output for environment:

```
DB_HOST=db.example.com
DB_PORT=5432
DB_USER=app_user
DB_PASSWORD=secure-password
```

## Step 7: Set Up Secret Rotation

Configure automatic rotation for your database credentials:

```yaml
# rotation.yaml
secrets:
  rotation:
    policies:
      - paths:
          - database/production/credentials
        schedule: "0 0 * * 0"  # Weekly on Sunday midnight
        strategy: blue_green
        verification:
          timeout: 30s
          retries: 3
```

Apply the rotation policy:

```bash
kscorectl state apply rotation.yaml
```

Trigger a manual rotation:

```bash
kscorectl secrets rotate start \
  --secret database/production/credentials \
  --strategy blue_green
```

## Step 8: Monitor Secret Access

View secret access logs:

```bash
# Recent accesses
kscorectl secrets audit "database/*"

# Check cache hit rates
kscorectl secrets cache status
```

## Complete Example: Full Application Setup

Here's a complete example deploying a web application with secrets:

**1. Store application secrets:**

```bash
# Store secrets using your backend's native CLI (example: HashiCorp Vault)
vault kv put secret/myapp/database \
  url="postgresql://db.example.com:5432/myapp" username="myapp" password="db-secret-123"

vault kv put secret/myapp/api-keys \
  stripe="sk_live_xxx" sendgrid="SG.xxx"

vault kv put secret/myapp/tls \
  cert=@/path/to/cert.pem key=@/path/to/key.pem
```

**2. Create workload definition:**

```yaml
# myapp-workload.yaml
workload:
  name: myapp-web

  secrets:
    # Database connection
    - path: myapp/database
      env:
        DATABASE_URL:
          template: "postgresql://{{ .username }}:{{ .password }}@db.example.com:5432/myapp"

    # API keys
    - path: myapp/api-keys
      env:
        STRIPE_API_KEY:
          key: stripe
        SENDGRID_API_KEY:
          key: sendgrid

    # TLS certificates
    - path: myapp/tls
      files:
        - destination: /etc/ssl/certs/myapp.crt
          key: cert
          mode: 0644
        - destination: /etc/ssl/private/myapp.key
          key: key
          mode: 0600
```

**3. Deploy:**

```bash
kscorectl state apply myapp-workload.yaml
```

**4. Verify:**

```bash
# Verify secrets are accessible
kscorectl secrets get myapp/database --field username

# Test application
curl https://myapp.example.com/health
```

## Troubleshooting

### Secret not found

```bash
# Check if secret exists
kscorectl secrets list "database/*"

# Check path spelling
kscorectl secrets get database/production/password -v
```

### Permission denied

```bash
# Check access policy
kscorectl secrets policy show database/production/password

# Create an access policy
kscorectl secrets policy create \
  --principal "workload:myapp" \
  --path "database/production/*" \
  --operations read
```

### Environment variable not set

```bash
# Check if secret is accessible
kscorectl secrets get database/production/credentials --field password

# Clear cache to force re-fetch
kscorectl secrets cache clear
```

## Next Steps

- [Backend Configuration](/docs/operations/secrets-backends/) - Connect to Vault, AWS, etc.
- [Rotation Strategies](/docs/operations/secrets-rotation/) - Set up automated rotation
- [Security Guide](/docs/operations/secrets-security/) - Security best practices
- [API Reference](/docs/reference/secrets-api/) - Complete API documentation
