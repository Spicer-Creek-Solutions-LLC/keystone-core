---
title: "Secrets Troubleshooting"
weight: 43
description: "Diagnose and resolve common issues with secrets management, backends, rotation, and leases."
---

## Overview

This guide helps diagnose and resolve common issues with Keystone Core's secrets management system.

> **Implementation Note**: Many diagnostic commands shown in this guide represent planned CLI features.
> Currently, troubleshooting primarily uses:
>
> - **Backend-native tools** (Vault CLI, AWS CLI, etc.) for direct diagnostics
> - **Log files** at `/var/log/keystone/` for server and agent issues
> - **`kscorectl secrets rotate`** for rotation orchestration
> - **Standard system tools** (curl, systemctl, journalctl) for service health

## Diagnostic Commands

### Health Check

```bash
# Check Keystone server health
curl -s http://localhost:8080/health | jq

# Check Vault backend health
vault status
curl -s https://vault.example.com:8200/v1/sys/health | jq

# Check AWS backend connectivity
aws sts get-caller-identity
```

### Debug Mode

```bash
# Enable debug logging for kscore-server
export KSCORE_LOG_LEVEL=debug
systemctl restart kscore-server

# View debug logs
journalctl -u kscore-server -f

# Test secret retrieval directly from backend
vault kv get -format=json secret/database/creds/app
```

### Status Commands

```bash
# Check rotation status
kscorectl secrets rotate list

# Check Vault lease status directly
vault list sys/leases/lookup/database/creds/

# Check server metrics
curl -s http://localhost:8080/metrics | grep secrets
```

## Connection Issues

### Cannot Connect to Backend

**Symptoms:**

- "connection refused" errors
- Timeout errors
- TLS handshake failures

**Diagnosis:**

```bash
# Test network connectivity to Vault
curl -k -s https://vault.example.com:8200/v1/sys/health | jq

# Check TLS configuration
openssl s_client -connect vault.example.com:8200 -showcerts </dev/null 2>/dev/null | openssl x509 -noout -dates

# Test Vault authentication
vault token lookup
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

# Test authentication with a simple read
vault kv get secret/test
```

**Solutions:**

```bash
# Renew token
vault token renew

# Generate new token
vault token create -policy=keystone-secrets

# Update token in environment and restart server
export VAULT_TOKEN="new-token"
systemctl restart kscore-server
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

# Update secret ID in environment and restart
export VAULT_SECRET_ID="new-secret-id"
systemctl restart kscore-server
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
# List leases in Vault
vault list sys/leases/lookup/database/creds/

# Check specific lease
vault lease lookup <lease-id>
```

**Solutions:**

```bash
# Request new credentials directly from Vault
vault read database/creds/app

# Or trigger a rotation to get fresh credentials
kscorectl secrets rotate start --path database/creds/app

# Configure eager renewal in config
```

```yaml
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
# Check server logs for renewal failures
journalctl -u kscore-server | grep -i "lease"

# View lease details in Vault
vault lease lookup <lease-id>
```

**Solutions:**

```bash
# Manually renew lease using Vault CLI
vault lease renew <lease-id>

# Or request new credentials
vault read database/creds/app
```

### Too Many Leases

**Symptoms:**

- "lease limit exceeded"
- Backend performance degradation
- Memory pressure

**Diagnosis:**

```bash
# Count leases in Vault
vault list -format=json sys/leases/lookup/database/creds/ | jq 'length'

# View lease quotas
vault read sys/quotas/lease-count/global
```

**Solutions:**

```bash
# Revoke all leases for a path (use with caution)
vault lease revoke -prefix database/creds/app

# Or revoke specific leases
vault lease revoke <lease-id>
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
kscorectl secrets rotate list

# View server logs for rotation details
journalctl -u kscore-server | grep -i "rotation"

# Check agent connectivity
kscorectl agents list
```

**Solutions:**

```bash
# Cancel stuck rotation and restart
kscorectl secrets rotate cancel <rotation-id>
kscorectl secrets rotate start --path database/creds/app

# Check Vault for credential state
vault read database/creds/app
```

### Verification Failures

**Symptoms:**

- "verification failed"
- Health checks failing
- Rotation rolls back

**Diagnosis:**

```bash
# Check server logs for verification failures
journalctl -u kscore-server | grep -i "verification"

# Test database connectivity manually
psql -h db.example.com -U app -c "SELECT 1"
```

**Solutions:**

```bash
# Check that health check endpoints work
curl -s http://app.example.com/health/database

# Skip verification during rotation (not recommended for production)
kscorectl secrets rotate start --path database/creds/app --skip-verification
```

### Rollback Issues

**Symptoms:**

- Rollback doesn't restore old credentials
- "rollback failed" errors
- Agents stuck with bad credentials

**Diagnosis:**

```bash
# Check rotation logs
journalctl -u kscore-server | grep -i "rollback"

# Verify current credential state in Vault
vault read database/creds/app
```

**Solutions:**

```bash
# Request new credentials from Vault
vault read database/creds/app

# Restart agents to pick up new credentials
systemctl restart kscore-agent

# Or restart specific agent via exec
kscorectl exec run "name:web-server" -- systemctl restart myapp
```

## Cache Issues

### Stale Cache

**Symptoms:**

- Old secrets returned after rotation
- Inconsistent credentials across agents
- Cache hit rate anomalies

**Diagnosis:**

```bash
# Check server metrics for cache stats
curl -s http://localhost:8080/metrics | grep cache

# Check agent logs for cache behavior
journalctl -u kscore-agent | grep -i "cache"
```

**Solutions:**

```bash
# Restart the agent to clear in-memory cache
systemctl restart kscore-agent

# Read directly from Vault bypassing any cache
vault read database/creds/app

# Trigger rotation to refresh credentials
kscorectl secrets rotate start --path database/creds/app
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
# Measure Vault latency directly
time vault read database/creds/app

# Check Vault server status
vault status

# View Keystone metrics
curl -s http://localhost:8080/metrics | grep secrets
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
# Check agent status
kscorectl agents show <agent-id>

# View agent logs on the agent host
journalctl -u kscore-agent | grep -i "secret"

# Test agent connectivity
kscorectl exec run "name:<agent-name>" -- echo "connected"
```

**Solutions:**

```bash
# Restart agent to refresh secrets
ssh <agent-host> "systemctl restart kscore-agent"

# Check agent configuration on the host
ssh <agent-host> "cat /etc/keystone-core/agent.yaml"

# Verify environment variables are set
kscorectl exec run "name:<agent-name>" -- env | grep -E "(DB_|API_)"
```

### Secret Injection Failures

**Symptoms:**

- Environment variables missing
- Secret files not created
- Application startup failures

**Diagnosis:**

```bash
# Check agent logs for injection errors
journalctl -u kscore-agent | grep -i "inject"

# Verify file permissions on secret destination
ls -la /etc/secrets/
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
# Search for specific errors in server logs
journalctl -u kscore-server | grep "SECRETS_001"

# Search with context
journalctl -u kscore-server | grep -B5 -A5 "SECRETS_001"

# Export logs for analysis
journalctl -u kscore-server --since "24 hours ago" -o json > secrets-logs.json
```

## Getting Help

### Debug Information

Collect debug information for support:

```bash
# Generate support bundle manually
mkdir -p /tmp/secrets-debug
journalctl -u kscore-server --since "24 hours ago" > /tmp/secrets-debug/server.log
journalctl -u kscore-agent --since "24 hours ago" > /tmp/secrets-debug/agent.log
cp /etc/keystone-core/server.yaml /tmp/secrets-debug/  # Review for secrets before sharing
curl -s http://localhost:8080/metrics > /tmp/secrets-debug/metrics.txt
tar -czf secrets-debug.tar.gz -C /tmp secrets-debug

# IMPORTANT: Review the bundle for sensitive data before sharing
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
