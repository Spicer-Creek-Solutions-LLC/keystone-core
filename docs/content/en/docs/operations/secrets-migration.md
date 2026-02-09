---
title: "Secrets Migration Guide"
weight: 44
description: "Migrate from legacy credential systems to Keystone Core's unified secrets management."
---

## Overview

This guide helps you migrate from existing credential management approaches to Keystone Core's unified secrets management system. Whether you're moving from environment variables, configuration files, or other secret managers, this guide provides step-by-step instructions.

> **Implementation Note**: Keystone Core's secrets management focuses on **rotation orchestration** and
> **workload secret injection**, not direct secret storage. The `kscorectl secrets put/get` commands
> shown in this guide represent the conceptual workflow. In practice:
>
> - **Store secrets** using your backend directly (Vault CLI, AWS CLI, etc.)
> - **Reference secrets** in workload definitions using `{{ secret "path" }}` syntax
> - **Orchestrate rotation** using `kscorectl secrets rotate` commands
>
> See [Secrets Backends](/docs/operations/secrets-backends/) for backend-specific instructions.

## Migration Paths

### From Environment Variables

Environment variables are a common but insecure method for managing secrets.

**Before (environment variables):**

```bash
export DB_PASSWORD="secretpassword"
export API_KEY="sk_live_abc123"
```

```yaml
# workload.yaml
workload:
  env:
    - name: DB_PASSWORD
      value_from:
        env: DB_PASSWORD
```

**After (Keystone Secrets):**

```yaml
# Store secrets first
# kscorectl secrets put database/app/password --value "secretpassword"
# kscorectl secrets put api/stripe --value "sk_live_abc123"

# workload.yaml
workload:
  secrets:
    - path: database/app/password
      env:
        DB_PASSWORD:
          key: value
    - path: api/stripe
      env:
        API_KEY:
          key: value
```

**Migration steps:**

1. Inventory all environment variables containing secrets
2. Store each secret in your backend (e.g., Vault):

   ```bash
   # Using Vault CLI
   vault kv put secret/database/app/password value="$DB_PASSWORD"
   vault kv put secret/api/stripe value="$API_KEY"
   ```

3. Update workload definitions to reference secrets
4. Remove environment variables from deployment scripts
5. Verify workloads receive secrets correctly

### From Configuration Files

**Before (plaintext config files):**

```yaml
# config.yaml
database:
  host: db.example.com
  username: app_user
  password: secretpassword

api:
  key: sk_live_abc123
```

**After (Keystone Secrets):**

```yaml
# config.yaml (secrets removed)
database:
  host: db.example.com
  username: app_user
  password: "${secrets:database/credentials/password}"

api:
  key: "${secrets:api/stripe/key}"
```

```yaml
# workload.yaml
workload:
  secrets:
    - path: database/credentials
      template:
        destination: /etc/app/secrets.yaml
        content: |
          database:
            password: {{ .password }}
          api:
            key: {{ .api_key }}
```

### From HashiCorp Vault (Direct)

If you're already using Vault but want to leverage Keystone's unified interface:

**Before (direct Vault access):**

```go
client, _ := vault.NewClient(vault.DefaultConfig())
secret, _ := client.Logical().Read("secret/data/myapp")
password := secret.Data["data"].(map[string]interface{})["password"]
```

**After (Keystone Secrets):**

```yaml
# Configure Vault as backend
secrets:
  backends:
    vault:
      address: https://vault.example.com:8200
      auth:
        method: kubernetes
        role: keystone-secrets
```

```yaml
# workload.yaml
workload:
  secrets:
    - backend: vault
      path: secret/data/myapp
      env:
        PASSWORD:
          key: password
```

**Benefits of migration:**

- Unified API across multiple backends
- Automatic lease management
- Coordinated rotation
- Consistent audit logging

### From AWS Secrets Manager (Direct)

**Before (direct AWS SDK):**

```go
client := secretsmanager.NewFromConfig(cfg)
result, _ := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
    SecretId: aws.String("myapp/database"),
})
password := result.SecretString
```

**After (Keystone Secrets):**

```yaml
# Configure AWS as backend
secrets:
  backends:
    aws:
      region: us-west-2
      auth:
        method: iam_role
```

```yaml
# workload.yaml
workload:
  secrets:
    - backend: aws
      path: myapp/database
      env:
        DB_PASSWORD:
          key: password
```

### From Kubernetes Secrets

**Before (K8s Secrets):**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: myapp-secrets
type: Opaque
data:
  password: c2VjcmV0cGFzc3dvcmQ=  # base64 encoded
```

```yaml
# deployment.yaml
containers:
  - name: app
    envFrom:
      - secretRef:
          name: myapp-secrets
```

**After (Keystone Secrets):**

```yaml
# Store in Keystone
# kscorectl secrets put myapp/database --value "secretpassword"

# workload.yaml
workload:
  secrets:
    - path: myapp/database
      env:
        PASSWORD:
          key: value
```

**Benefits:**

- No base64 encoding required
- Automatic rotation support
- Cross-cluster secret sharing
- Enhanced audit logging

## Bulk Migration Tools

### Export Existing Secrets

> **Note**: Use your secrets backend CLI directly for bulk operations.

**From environment files to Vault:**

```bash
# Export from .env file to Vault
while IFS='=' read -r key value; do
  if [[ ! "$key" =~ ^# && -n "$key" ]]; then
    vault kv put "secret/env/$key" value="$value"
    echo "Migrated: $key"
  fi
done < .env
```

**From Kubernetes to Vault:**

```bash
# Export K8s secrets to Vault
for secret in $(kubectl get secrets -o name); do
  name=$(echo $secret | cut -d'/' -f2)
  kubectl get $secret -o jsonpath='{.data}' | jq -r 'to_entries[] | "\(.key)=\(.value)"' | while read line; do
    key=$(echo $line | cut -d'=' -f1)
    value=$(echo $line | cut -d'=' -f2 | base64 -d)
    vault kv put "secret/k8s/$name/$key" value="$value"
    echo "Migrated: $name/$key"
  done
done
```

**Reorganize within Vault:**

```bash
# Copy secrets within Vault to new paths
vault kv list -format=json secret/ | jq -r '.[]' | while read path; do
  vault kv get -format=json "secret/$path" | jq -r '.data.data' | \
    vault kv put "secret/migrated/$path" -
  echo "Copied: $path"
done
```

### Migration Validation

Validate secrets were migrated correctly:

```bash
# Compare original and migrated values
original=$(cat .env | grep "^DB_PASSWORD=" | cut -d'=' -f2)
migrated=$(vault kv get -field=value secret/env/DB_PASSWORD)

if [ "$original" = "$migrated" ]; then
  echo "DB_PASSWORD: MATCH"
else
  echo "DB_PASSWORD: MISMATCH"
fi
```

## Step-by-Step Migration Process

### Phase 1: Assessment

1. **Inventory secrets:**

   ```bash
   # Manually inventory secrets from various sources
   # Environment variables
   env | grep -E '(PASSWORD|SECRET|KEY|TOKEN)' > inventory-env.txt

   # Vault secrets
   vault kv list -format=json secret/ > inventory-vault.json

   # Kubernetes secrets
   kubectl get secrets -A -o json | jq '.items[].metadata.name' > inventory-k8s.txt
   ```

2. **Identify secret types:**
   - Static credentials (passwords, API keys)
   - Dynamic credentials (database users, certificates)
   - Rotating credentials (OAuth tokens, certificates)

3. **Document dependencies:**
   - Which workloads use each secret
   - Rotation requirements
   - Compliance requirements

### Phase 2: Backend Configuration

1. **Configure backends:**

   ```yaml
   secrets:
     backends:
       vault:
         address: https://vault.example.com:8200
         auth:
           method: kubernetes
           role: keystone-secrets
       aws:
         region: us-west-2
         auth:
           method: iam_role
   ```

2. **Test connectivity:**

   ```bash
   # Test Vault connectivity
   vault status
   vault token lookup

   # Test AWS connectivity
   aws sts get-caller-identity
   aws secretsmanager list-secrets --max-results 1
   ```

3. **Configure routing:**

   ```yaml
   secrets:
     routing:
       rules:
         - pattern: "database/*"
           backend: vault
         - pattern: "aws/*"
           backend: aws
   ```

### Phase 3: Secret Migration

1. **Migrate secrets in batches:**

   ```bash
   # Copy secrets within Vault to new paths
   # First, read the source secret
   vault kv get -format=json secret/data/database > /tmp/db-secret.json

   # Then write to new location
   cat /tmp/db-secret.json | jq '.data.data' | \
     vault kv put secret/database/production -

   # Clean up temp file
   rm /tmp/db-secret.json
   ```

2. **Verify migration:**

   ```bash
   # Compare source and destination values
   vault kv get secret/data/database
   vault kv get secret/database/production
   ```

### Phase 4: Workload Updates

1. **Update workload definitions:**

   ```yaml
   # Before
   workload:
     env:
       - name: DB_PASSWORD
         value_from:
           vault_path: secret/data/database
           key: password

   # After
   workload:
     secrets:
       - path: database/production
         env:
           DB_PASSWORD:
             key: password
   ```

2. **Deploy updates gradually:**

   ```bash
   # Update canary first
   kscorectl state apply workload.yaml --target "role:canary"

   # Verify functionality by checking application logs and health
   kscorectl exec run "role:canary" -- curl -s localhost:8080/health

   # Roll out to all
   kscorectl state apply workload.yaml
   ```

### Phase 5: Cleanup

1. **Remove legacy access:**

   ```bash
   # Revoke old Vault tokens
   vault token revoke <old-token>

   # Remove old K8s secrets
   kubectl delete secret myapp-secrets
   ```

2. **Update documentation:**
   - Remove references to old secret paths
   - Document new secret paths
   - Update runbooks

## Rollback Procedures

### Quick Rollback

If issues occur during migration:

```bash
# Restore previous workload configuration using git
git checkout HEAD~1 -- workload.yaml
kscorectl state apply workload.yaml

# Or manually update workload to use old secret paths
# Edit workload.yaml to reference old paths, then re-apply
```

### Gradual Rollback

For partial rollback:

```yaml
# Route specific paths back to legacy
secrets:
  routing:
    rules:
      - pattern: "database/problematic/*"
        backend: legacy
      - pattern: "database/*"
        backend: vault  # Other database secrets stay migrated
```

## Testing Migration

### Integration Tests

```bash
# Test secret retrieval directly from backend
vault kv get secret/database/production

# Test secret injection by deploying a test workload
kscorectl state apply test-workload.yaml --dry-run

# Test rotation workflow
kscorectl secrets rotate start --path database/production --dry-run
```

### Smoke Tests

```bash
# Verify application can connect
curl -f http://app.example.com/health/database

# Verify secrets are injected (use agent ID, not pod name)
kscorectl exec run "name:app-server" -- env | grep DB_PASSWORD
```

## Common Migration Issues

### Issue: Secret Path Conflicts

**Problem:** Existing paths conflict with new naming scheme.

**Solution:**

```yaml
secrets:
  routing:
    rules:
      # Alias old paths to new paths
      - pattern: "old/database/*"
        rewrite: "database/production/*"
        backend: vault
```

### Issue: Rotation Schedule Mismatch

**Problem:** Legacy system has different rotation schedules.

**Solution:**

```yaml
secrets:
  rotation:
    policies:
      - paths:
          - database/*
        schedule: "0 0 * * 0"  # Weekly, matching legacy
        transition_period: 7d  # Allow gradual transition
```

### Issue: Permission Denied

**Problem:** New system lacks permissions from old system.

**Solution:**

```bash
# Check current Vault policy
vault policy read keystone-secrets

# Update Vault policy to grant access
vault policy write keystone-secrets - <<EOF
path "secret/data/database/*" {
  capabilities = ["read", "list"]
}
path "database/creds/*" {
  capabilities = ["read"]
}
EOF
```

### Issue: Encoding Differences

**Problem:** Secrets have different encodings (base64, URL encoding).

**Solution:**

```yaml
# Specify encoding during migration
secrets:
  migration:
    encoding:
      - pattern: "k8s/*"
        source_encoding: base64
        target_encoding: utf8
```

## Post-Migration Checklist

- [ ] All secrets migrated and verified
- [ ] All workloads updated and tested
- [ ] Rotation schedules configured
- [ ] Access policies configured
- [ ] Monitoring and alerting set up
- [ ] Documentation updated
- [ ] Old credentials revoked
- [ ] Rollback procedure tested
- [ ] Compliance requirements verified
- [ ] Backup and recovery tested

## Next Steps

- [Backend Configuration](/docs/operations/secrets-backends/) - Configure backends
- [Rotation Strategies](/docs/operations/secrets-rotation/) - Set up rotation
- [Security Guide](/docs/operations/secrets-security/) - Security best practices
- [Troubleshooting](/docs/operations/secrets-troubleshooting/) - Common issues
