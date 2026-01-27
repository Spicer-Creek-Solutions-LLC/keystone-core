---
title: "Secrets Troubleshooting"
weight: 43
description: "Diagnose and resolve common issues with secrets management, backends, rotation, and leases."
---

## Overview

This guide helps diagnose and resolve common issues with Keystone Core's secrets management system.

## Diagnostic Commands

### Health Check

```bash
# Overall secrets system health
kscorectl secrets health

# Backend-specific health
kscorectl secrets health --backend vault
kscorectl secrets health --backend aws
```

### Debug Mode

```bash
# Enable debug logging
kscorectl secrets --debug get database/creds/app

# Verbose output
kscorectl secrets -v get database/creds/app
```

### Status Commands

```bash
# Lease status
kscorectl secrets lease list --status active

# Rotation status
kscorectl secrets rotation list --status in_progress

# Backend status
kscorectl secrets backend status --all
```

## Connection Issues

### Cannot Connect to Backend

**Symptoms:**
- "connection refused" errors
- Timeout errors
- TLS handshake failures

**Diagnosis:**
```bash
# Test network connectivity
kscorectl secrets backend test vault

# Check TLS configuration
kscorectl secrets backend test vault --show-tls

# Verbose connection test
kscorectl secrets backend test vault -v
```

**Common Causes and Solutions:**

| Cause | Solution |
|-------|----------|
| Firewall blocking | Open ports (Vault: 8200, AWS: 443) |
| DNS resolution failure | Check DNS settings, use IP address |
| TLS certificate mismatch | Update CA certificate |
| Backend not running | Start backend service |

**Vault-Specific:**
```bash
# Check Vault status
vault status

# Verify Vault is unsealed
vault status | grep Sealed

# Test Vault connectivity
curl -k https://vault.example.com:8200/v1/sys/health
```

**AWS-Specific:**
```bash
# Test AWS credentials
aws sts get-caller-identity

# Check Secrets Manager endpoint
aws secretsmanager list-secrets --region us-west-2
```

### TLS Certificate Errors

**Symptoms:**
- "x509: certificate signed by unknown authority"
- "certificate has expired"
- "certificate is not valid for hostname"

**Solutions:**

```yaml
# Option 1: Specify CA certificate
backends:
  vault:
    tls:
      ca_cert: "/etc/ssl/certs/vault-ca.pem"

# Option 2: Use system CA store
backends:
  vault:
    tls:
      use_system_ca: true

# Option 3: Skip verification (NOT for production)
backends:
  vault:
    tls:
      insecure_skip_verify: true
```

**Verify certificate:**
```bash
# Check certificate details
openssl s_client -connect vault.example.com:8200 -showcerts

# Verify CA chain
openssl verify -CAfile /etc/ssl/certs/vault-ca.pem /path/to/cert.pem
```

## Authentication Issues

### Token Authentication Failures

**Symptoms:**
- "permission denied"
- "invalid token"
- "token expired"

**Diagnosis:**
```bash
# Check token validity (Vault)
vault token lookup

# Test authentication
kscorectl secrets auth test --backend vault
```

**Solutions:**

```bash
# Renew token
vault token renew

# Generate new token
vault token create -policy=keystone-secrets

# Update configuration
kscorectl secrets backend update vault --token "new-token"
```

### AppRole Authentication Failures

**Symptoms:**
- "invalid role_id"
- "invalid secret_id"
- "secret_id expired"

**Diagnosis:**
```bash
# Verify role exists
vault read auth/approle/role/keystone

# Check secret ID
vault write auth/approle/role/keystone/secret-id-accessor/lookup \
  secret_id_accessor="accessor-id"
```

**Solutions:**
```bash
# Generate new secret ID
vault write -f auth/approle/role/keystone/secret-id

# Update configuration
kscorectl secrets backend update vault \
  --secret-id "new-secret-id"
```

### Kubernetes Authentication Failures

**Symptoms:**
- "service account token invalid"
- "namespace not allowed"
- "role not found"

**Diagnosis:**
```bash
# Check service account
kubectl get serviceaccount keystone-server -n keystone-system

# Verify token mount
kubectl exec -it keystone-server-xxx -n keystone-system -- \
  cat /var/run/secrets/kubernetes.io/serviceaccount/token

# Test Vault auth
vault write auth/kubernetes/login \
  role=keystone-secrets \
  jwt="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"
```

**Solutions:**
```bash
# Update Vault Kubernetes config
vault write auth/kubernetes/config \
  kubernetes_host="https://kubernetes.default.svc" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

# Fix role binding
vault write auth/kubernetes/role/keystone-secrets \
  bound_service_account_names=keystone-server \
  bound_service_account_namespaces=keystone-system \
  policies=keystone-secrets
```

### Cloud Provider Authentication

**AWS IAM Role Issues:**
```bash
# Check instance metadata
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/

# Verify role
aws sts get-caller-identity

# Check permissions
aws secretsmanager get-secret-value --secret-id test-secret
```

**Azure Managed Identity Issues:**
```bash
# Check identity
curl -H "Metadata: true" \
  "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://vault.azure.net"

# Verify Key Vault access
az keyvault secret list --vault-name myvault
```

**GCP Workload Identity Issues:**
```bash
# Check service account
gcloud auth list

# Verify Secret Manager access
gcloud secrets list --project my-project
```

## Lease Issues

### Lease Expiration

**Symptoms:**
- "lease not found"
- "lease expired"
- Credentials stop working

**Diagnosis:**
```bash
# List expired leases
kscorectl secrets lease list --status expired

# Check specific lease
kscorectl secrets lease show lease-abc123
```

**Solutions:**
```bash
# Request new credentials
kscorectl secrets get database/creds/app --force-refresh

# Configure eager renewal
secrets:
  lease_management:
    strategy: eager
    renewal_threshold: 0.5  # Renew at 50% TTL
```

### Lease Renewal Failures

**Symptoms:**
- "lease renewal failed"
- "max TTL exceeded"
- Credentials expire unexpectedly

**Diagnosis:**
```bash
# Check renewal logs
kscorectl secrets lease logs lease-abc123

# View lease details
kscorectl secrets lease show lease-abc123 --verbose
```

**Solutions:**
```bash
# Manually renew lease
kscorectl secrets lease renew lease-abc123

# Configure retry settings
secrets:
  lease_management:
    renewal_retries: 3
    renewal_retry_delay: 5s
    grace_period: 5m
```

### Too Many Leases

**Symptoms:**
- "lease limit exceeded"
- Backend performance degradation
- Memory pressure

**Diagnosis:**
```bash
# Count leases by backend
kscorectl secrets lease count --group-by backend

# Find stale leases
kscorectl secrets lease list --idle-since 24h
```

**Solutions:**
```bash
# Revoke stale leases
kscorectl secrets lease revoke --idle-since 24h

# Configure cleanup
secrets:
  lease_management:
    cleanup:
      enabled: true
      interval: 1h
      max_idle: 24h
```

## Rotation Issues

### Rotation Stuck in Progress

**Symptoms:**
- Rotation never completes
- "rotation timeout" errors
- Agents not receiving new credentials

**Diagnosis:**
```bash
# Check rotation status
kscorectl secrets rotation status rotation-abc123

# View rotation logs
kscorectl secrets rotation logs rotation-abc123

# Check agent status
kscorectl secrets rotation agents rotation-abc123
```

**Solutions:**
```bash
# Resume stuck rotation
kscorectl secrets rotation resume rotation-abc123

# Cancel and restart
kscorectl secrets rotation cancel rotation-abc123
kscorectl secrets rotation start --path database/creds/app

# Force completion (use with caution)
kscorectl secrets rotation complete rotation-abc123 --force
```

### Verification Failures

**Symptoms:**
- "verification failed"
- Health checks failing
- Rotation rolls back

**Diagnosis:**
```bash
# Check verification logs
kscorectl secrets rotation logs rotation-abc123 --phase verification

# Test verification manually
kscorectl secrets rotation verify-test \
  --path database/creds/app \
  --agent agent-1
```

**Solutions:**
```bash
# Increase verification timeout
rotation:
  verification:
    timeout: 60s
    retries: 5

# Skip verification (not recommended)
kscorectl secrets rotation start \
  --path database/creds/app \
  --skip-verification
```

### Rollback Issues

**Symptoms:**
- Rollback doesn't restore old credentials
- "rollback failed" errors
- Agents stuck with bad credentials

**Diagnosis:**
```bash
# Check rollback status
kscorectl secrets rotation rollback-status rotation-abc123

# View credential history
kscorectl secrets history --path database/creds/app
```

**Solutions:**
```bash
# Manual rollback
kscorectl secrets restore \
  --path database/creds/app \
  --version 1

# Force agent refresh
kscorectl agents refresh \
  --target "environment=production" \
  --secrets database/creds/app
```

## Cache Issues

### Stale Cache

**Symptoms:**
- Old secrets returned after rotation
- Inconsistent credentials across agents
- Cache hit rate anomalies

**Diagnosis:**
```bash
# Check cache statistics
kscorectl secrets cache stats

# View cached entries
kscorectl secrets cache list --path "database/*"
```

**Solutions:**
```bash
# Clear specific cache entry
kscorectl secrets cache clear --path database/creds/app

# Clear all cache
kscorectl secrets cache clear --all

# Disable cache temporarily
kscorectl secrets get database/creds/app --no-cache
```

### Cache Memory Issues

**Symptoms:**
- High memory usage
- Cache eviction errors
- Slow secret retrieval

**Solutions:**
```yaml
secrets:
  cache:
    l1:
      # Reduce max entries
      max_entries: 5000
      # Shorter TTL
      ttl: 1m
    l2:
      # Enable compression
      compression: true
```

## Performance Issues

### Slow Secret Retrieval

**Symptoms:**
- High latency for secret operations
- Timeout errors
- Backend throttling

**Diagnosis:**
```bash
# Measure latency
kscorectl secrets get database/creds/app --timing

# Check backend latency
kscorectl secrets backend latency --backend vault

# View metrics
kscorectl secrets metrics --period 1h
```

**Solutions:**
```yaml
# Enable connection pooling
secrets:
  backends:
    vault:
      pool:
        min_connections: 5
        max_connections: 20
        idle_timeout: 5m

# Configure caching
secrets:
  cache:
    l1:
      enabled: true
      max_entries: 10000
      ttl: 5m

# Enable request batching
secrets:
  batching:
    enabled: true
    max_batch_size: 100
    batch_timeout: 10ms
```

### Rate Limiting

**Symptoms:**
- "rate limit exceeded" errors
- 429 responses from backend
- Throttling warnings

**Solutions:**
```yaml
secrets:
  rate_limiting:
    # Client-side rate limiting
    requests_per_second: 50
    burst: 100

    # Per-backend limits
    backends:
      vault:
        requests_per_second: 100
      aws:
        requests_per_second: 50
```

## Agent Issues

### Agent Not Receiving Secrets

**Symptoms:**
- Agent reports "secret not found"
- Environment variables not set
- Files not created

**Diagnosis:**
```bash
# Check agent secret status
kscorectl agents secrets status agent-1

# View agent logs
kscorectl agents logs agent-1 --filter secrets

# Test agent connectivity
kscorectl agents ping agent-1
```

**Solutions:**
```bash
# Force secret refresh
kscorectl agents refresh agent-1 --secrets

# Check agent configuration
kscorectl agents config show agent-1

# Restart agent secret sync
kscorectl agents restart agent-1 --component secrets
```

### Secret Injection Failures

**Symptoms:**
- Environment variables missing
- Secret files not created
- Application startup failures

**Diagnosis:**
```bash
# Check injection status
kscorectl secrets injection status --agent agent-1

# View injection logs
kscorectl secrets injection logs --agent agent-1
```

**Solutions:**
```yaml
# Fix file permissions
workload:
  secrets:
    - path: database/creds/app
      files:
        - destination: /etc/secrets/db-password
          mode: 0600
          owner: app
          group: app

# Fix environment injection
workload:
  secrets:
    - path: database/creds/app
      env:
        DB_PASSWORD:
          key: password
          # Ensure variable name is valid
          sanitize: true
```

## Error Reference

### Common Error Codes

| Error Code | Description | Solution |
|------------|-------------|----------|
| `SECRETS_001` | Backend connection failed | Check network connectivity |
| `SECRETS_002` | Authentication failed | Verify credentials |
| `SECRETS_003` | Permission denied | Check access policies |
| `SECRETS_004` | Secret not found | Verify secret path |
| `SECRETS_005` | Lease expired | Request new credentials |
| `SECRETS_006` | Rate limit exceeded | Reduce request rate |
| `SECRETS_007` | Rotation failed | Check verification settings |
| `SECRETS_008` | Encryption failed | Verify KMS configuration |
| `SECRETS_009` | Cache error | Clear cache, check memory |
| `SECRETS_010` | Validation error | Check secret format |

### Error Log Analysis

```bash
# Search for specific errors
kscorectl logs search --component secrets --error "SECRETS_001"

# Get error context
kscorectl logs context --error-id err_abc123

# Export errors for analysis
kscorectl logs export --component secrets --since 24h --format json
```

## Getting Help

### Debug Information

Collect debug information for support:

```bash
# Generate support bundle
kscorectl support bundle \
  --component secrets \
  --include-logs \
  --include-config \
  --output secrets-debug.tar.gz
```

### Log Locations

| Component | Log Location |
|-----------|-------------|
| Server | `/var/log/keystone/server.log` |
| Agent | `/var/log/keystone/agent.log` |
| Secrets | `/var/log/keystone/secrets.log` |
| Audit | `/var/log/keystone/secrets-audit.log` |

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `/health/secrets` | Overall secrets health |
| `/health/secrets/backends` | Backend health |
| `/health/secrets/leases` | Lease manager health |
| `/health/secrets/cache` | Cache health |

## Next Steps

- [Backend Setup](/docs/operations/secrets-backends/) - Configure backends
- [Rotation Strategies](/docs/operations/secrets-rotation/) - Configure rotation
- [Security Guide](/docs/operations/secrets-security/) - Security best practices
