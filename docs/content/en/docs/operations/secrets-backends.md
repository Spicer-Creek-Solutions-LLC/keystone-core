---
title: "Secrets Backend Setup"
weight: 40
description: "Configuration guides for HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, and GCP Secret Manager."
---

## Overview

This guide covers the setup and configuration of each supported secret backend. Choose the backend that best fits your infrastructure and security requirements.

> **Implementation Note**: The `backends:` YAML configuration blocks shown in this document
> represent conceptual integration patterns for reference. Currently, backend authentication
> is handled through:
> - **Environment variables** (e.g., `VAULT_ADDR`, `VAULT_TOKEN`, `AWS_REGION`)
> - **Backend-native configuration** (Vault agent, AWS IAM roles, Azure managed identity, GCP workload identity)
> - **State file secret references** using `{{ secret "path" }}` syntax
>
> Keystone Core's secrets CLI (`kscore-secrets`) focuses on rotation orchestration, scheduling,
> and policies. Direct secret retrieval uses the backend's native tools or environment injection.

## HashiCorp Vault

HashiCorp Vault is a full-featured secret management platform supporting dynamic secrets, encryption-as-a-service, and extensive audit capabilities.

### Prerequisites

- Vault server 1.12+ installed and unsealed
- Network connectivity from Keystone Core to Vault
- Appropriate Vault policies configured

### Authentication Methods

#### Token Authentication

Simplest method, suitable for development:

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    auth:
      method: token
      token: "{{ env.VAULT_TOKEN }}"
```

#### AppRole Authentication

Recommended for production automated systems:

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    auth:
      method: approle
      role_id: "db02de05-fa39-4855-059b-67221c5c2f63"
      secret_id: "{{ env.VAULT_SECRET_ID }}"
      # Optional: wrapped secret ID
      # secret_id_wrapped: true
```

**Vault Setup:**
```bash
# Enable AppRole auth
vault auth enable approle

# Create policy
vault policy write keystone-secrets - <<EOF
path "secret/data/*" {
  capabilities = ["read", "list"]
}
path "database/creds/*" {
  capabilities = ["read"]
}
path "pki/issue/*" {
  capabilities = ["create", "update"]
}
path "transit/encrypt/*" {
  capabilities = ["create", "update"]
}
path "transit/decrypt/*" {
  capabilities = ["create", "update"]
}
EOF

# Create AppRole
vault write auth/approle/role/keystone \
  token_policies="keystone-secrets" \
  token_ttl=1h \
  token_max_ttl=4h \
  secret_id_ttl=10m

# Get role ID
vault read auth/approle/role/keystone/role-id

# Generate secret ID
vault write -f auth/approle/role/keystone/secret-id
```

#### Kubernetes Authentication

Recommended for Kubernetes deployments:

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    auth:
      method: kubernetes
      role: "keystone-secrets"
      # Optional: custom mount path
      # mount_path: "kubernetes-prod"
      # Optional: custom service account token path
      # token_path: "/var/run/secrets/kubernetes.io/serviceaccount/token"
```

**Vault Setup:**
```bash
# Enable Kubernetes auth
vault auth enable kubernetes

# Configure Kubernetes auth
vault write auth/kubernetes/config \
  kubernetes_host="https://kubernetes.default.svc" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

# Create role
vault write auth/kubernetes/role/keystone-secrets \
  bound_service_account_names=keystone-server \
  bound_service_account_namespaces=keystone-system \
  policies=keystone-secrets \
  ttl=1h
```

### Secret Engines

#### KV v2 (Key-Value)

```yaml
backends:
  vault:
    engines:
      kv:
        mount_path: "secret"
        version: 2
```

**Usage:**
```bash
# Write secret
vault kv put secret/myapp/config username="app" password="secret"

# Read secret (use Vault CLI directly)
vault kv get secret/myapp/config
```

> **Note**: Keystone Core's secrets CLI (`kscore-secrets`) focuses on rotation orchestration,
> scheduling, and policies. Direct secret retrieval is handled by the backend (e.g., Vault CLI)
> or through environment variable injection in state files.

#### Database Secrets Engine

Dynamic database credentials:

```yaml
backends:
  vault:
    engines:
      database:
        mount_path: "database"
        # Connection configuration done in Vault
```

**Vault Setup:**
```bash
# Enable database engine
vault secrets enable database

# Configure PostgreSQL connection
vault write database/config/postgres \
  plugin_name=postgresql-database-plugin \
  allowed_roles="readonly,readwrite" \
  connection_url="postgresql://{{username}}:{{password}}@postgres:5432/mydb" \
  username="vault_admin" \
  password="admin_password"

# Create readonly role
vault write database/roles/readonly \
  db_name=postgres \
  creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
  default_ttl="1h" \
  max_ttl="24h"
```

#### PKI Secrets Engine

Certificate management:

```yaml
backends:
  vault:
    engines:
      pki:
        mount_path: "pki"
```

**Vault Setup:**
```bash
# Enable PKI
vault secrets enable pki

# Configure CA
vault write pki/root/generate/internal \
  common_name="Example CA" \
  ttl=87600h

# Create role
vault write pki/roles/web-server \
  allowed_domains="example.com" \
  allow_subdomains=true \
  max_ttl="720h"
```

#### Transit Secrets Engine

Encryption-as-a-service:

```yaml
backends:
  vault:
    engines:
      transit:
        mount_path: "transit"
```

**Vault Setup:**
```bash
# Enable transit
vault secrets enable transit

# Create encryption key
vault write -f transit/keys/my-key

# Create signing key
vault write transit/keys/signing-key type=ecdsa-p256
```

### TLS Configuration

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    tls:
      ca_cert: "/etc/ssl/certs/vault-ca.pem"
      client_cert: "/etc/ssl/certs/client.pem"
      client_key: "/etc/ssl/private/client-key.pem"
      # Skip verification (not recommended for production)
      # insecure_skip_verify: true
```

### Enterprise Features

#### Namespaces

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    namespace: "production/team-a"
```

#### Performance Replication

```yaml
backends:
  vault:
    address: "https://vault.example.com:8200"
    # Prefer local performance replica
    prefer_local_replica: true
```

---

## AWS Secrets Manager

AWS Secrets Manager provides native secret storage with automatic rotation for AWS services.

### Prerequisites

- AWS account with Secrets Manager access
- IAM permissions for Keystone Core
- Network connectivity (VPC endpoints recommended)

### Authentication Methods

#### IAM Role (Recommended)

For EC2 instances or ECS tasks:

```yaml
backends:
  aws:
    region: "us-west-2"
    auth:
      method: iam_role
```

**IAM Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret",
        "secretsmanager:ListSecrets"
      ],
      "Resource": "arn:aws:secretsmanager:us-west-2:123456789:secret:production/*"
    }
  ]
}
```

#### Assume Role

For cross-account access:

```yaml
backends:
  aws:
    region: "us-west-2"
    auth:
      method: assume_role
      role_arn: "arn:aws:iam::987654321:role/SecretsReader"
      # Optional: external ID for security
      external_id: "keystone-production"
      # Optional: session duration
      session_duration: 1h
```

#### Web Identity (EKS)

For EKS with IRSA:

```yaml
backends:
  aws:
    region: "us-west-2"
    auth:
      method: web_identity
      role_arn: "arn:aws:iam::123456789:role/KeystoneSecretsRole"
      # Token file path (default for EKS)
      web_identity_token_file: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
```

**EKS Setup:**
```bash
# Create IAM OIDC provider
eksctl utils associate-iam-oidc-provider --cluster my-cluster --approve

# Create service account with IAM role
eksctl create iamserviceaccount \
  --name keystone-server \
  --namespace keystone-system \
  --cluster my-cluster \
  --attach-policy-arn arn:aws:iam::123456789:policy/SecretsReadPolicy \
  --approve
```

### Secret Versioning

Access specific versions:

```yaml
# Current version (default)
path: "production/database"
version: "AWSCURRENT"

# Previous version
path: "production/database"
version: "AWSPREVIOUS"

# Specific version ID
path: "production/database"
version_id: "a1b2c3d4-5678-90ab-cdef-EXAMPLE11111"
```

### Automatic Rotation

AWS Secrets Manager can automatically rotate secrets:

```yaml
backends:
  aws:
    rotation:
      # Detect rotation status
      detect_rotation: true
      # Refresh cache on rotation
      refresh_on_rotation: true
```

**AWS Setup:**
```bash
# Enable rotation with Lambda
aws secretsmanager rotate-secret \
  --secret-id production/database \
  --rotation-lambda-arn arn:aws:lambda:us-west-2:123456789:function:SecretsRotation \
  --rotation-rules AutomaticallyAfterDays=30
```

### VPC Endpoints

For private connectivity:

```yaml
backends:
  aws:
    region: "us-west-2"
    endpoint_url: "https://vpce-xxx.secretsmanager.us-west-2.vpce.amazonaws.com"
```

---

## Azure Key Vault

Azure Key Vault provides secure storage for secrets, keys, and certificates in Azure.

### Prerequisites

- Azure subscription with Key Vault
- Appropriate RBAC permissions
- Network connectivity (private endpoints recommended)

### Authentication Methods

#### Managed Identity (Recommended)

For Azure VMs or AKS:

```yaml
backends:
  azure:
    vault_url: "https://myvault.vault.azure.net"
    auth:
      method: managed_identity
      # Optional: user-assigned identity
      # client_id: "12345678-1234-1234-1234-123456789abc"
```

**Azure Setup:**
```bash
# Enable system-assigned identity on VM
az vm identity assign -g myResourceGroup -n myVM

# Grant access to Key Vault
az keyvault set-policy -n myvault \
  --object-id $(az vm show -g myResourceGroup -n myVM --query identity.principalId -o tsv) \
  --secret-permissions get list
```

#### Service Principal

For non-Azure environments:

```yaml
backends:
  azure:
    vault_url: "https://myvault.vault.azure.net"
    auth:
      method: service_principal
      tenant_id: "{{ env.AZURE_TENANT_ID }}"
      client_id: "{{ env.AZURE_CLIENT_ID }}"
      client_secret: "{{ env.AZURE_CLIENT_SECRET }}"
```

#### Workload Identity (AKS)

For AKS with workload identity:

```yaml
backends:
  azure:
    vault_url: "https://myvault.vault.azure.net"
    auth:
      method: workload_identity
      # Uses AZURE_CLIENT_ID and AZURE_FEDERATED_TOKEN_FILE from environment
```

**AKS Setup:**
```bash
# Enable workload identity on AKS
az aks update -g myResourceGroup -n myAKS --enable-oidc-issuer --enable-workload-identity

# Create managed identity
az identity create -g myResourceGroup -n keystone-identity

# Create federated credential
az identity federated-credential create \
  --name keystone-federated \
  --identity-name keystone-identity \
  --resource-group myResourceGroup \
  --issuer $(az aks show -g myResourceGroup -n myAKS --query "oidcIssuerProfile.issuerUrl" -o tsv) \
  --subject system:serviceaccount:keystone-system:keystone-server

# Grant Key Vault access
az keyvault set-policy -n myvault \
  --object-id $(az identity show -g myResourceGroup -n keystone-identity --query principalId -o tsv) \
  --secret-permissions get list
```

### Key Operations

Azure Key Vault supports cryptographic operations:

```yaml
backends:
  azure:
    vault_url: "https://myvault.vault.azure.net"
    key_operations:
      enabled: true
      # Operations: encrypt, decrypt, sign, verify, wrap, unwrap
```

### Soft Delete

Handle soft-deleted secrets:

```yaml
backends:
  azure:
    vault_url: "https://myvault.vault.azure.net"
    soft_delete:
      # List deleted secrets
      list_deleted: true
      # Recover deleted secrets
      allow_recover: true
      # Purge deleted secrets (requires purge permission)
      allow_purge: false
```

### Private Endpoints

For secure connectivity:

```yaml
backends:
  azure:
    vault_url: "https://myvault.privatelink.vaultcore.azure.net"
    # Disable public access
    private_link: true
```

---

## GCP Secret Manager

GCP Secret Manager provides secret storage with fine-grained IAM and audit logging.

### Prerequisites

- GCP project with Secret Manager API enabled
- Appropriate IAM permissions
- Network connectivity (VPC Service Controls supported)

### Authentication Methods

#### Default Credentials (Recommended)

For GCE, GKE, or Cloud Run:

```yaml
backends:
  gcp:
    project: "my-project-id"
    auth:
      method: default
```

#### Service Account Key

For non-GCP environments:

```yaml
backends:
  gcp:
    project: "my-project-id"
    auth:
      method: service_account
      credentials_file: "/etc/gcp/service-account.json"
```

#### Workload Identity (GKE)

For GKE with workload identity:

```yaml
backends:
  gcp:
    project: "my-project-id"
    auth:
      method: workload_identity
      # Uses GKE workload identity automatically
```

**GKE Setup:**
```bash
# Enable workload identity on GKE
gcloud container clusters update my-cluster \
  --workload-pool=my-project.svc.id.goog

# Create service account
gcloud iam service-accounts create keystone-secrets \
  --display-name="Keystone Secrets Reader"

# Grant Secret Manager access
gcloud projects add-iam-policy-binding my-project \
  --member="serviceAccount:keystone-secrets@my-project.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

# Link Kubernetes SA to GCP SA
gcloud iam service-accounts add-iam-policy-binding \
  keystone-secrets@my-project.iam.gserviceaccount.com \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:my-project.svc.id.goog[keystone-system/keystone-server]"

# Annotate Kubernetes SA
kubectl annotate serviceaccount keystone-server \
  -n keystone-system \
  iam.gke.io/gcp-service-account=keystone-secrets@my-project.iam.gserviceaccount.com
```

#### Service Account Impersonation

For cross-project access:

```yaml
backends:
  gcp:
    project: "target-project-id"
    auth:
      method: impersonation
      target_service_account: "secrets-reader@target-project.iam.gserviceaccount.com"
```

### Secret Versioning

```yaml
# Latest version (default)
path: "projects/my-project/secrets/database-password/versions/latest"

# Specific version
path: "projects/my-project/secrets/database-password/versions/5"
```

### Version Management

```yaml
backends:
  gcp:
    project: "my-project-id"
    version_management:
      # Automatically disable old versions after rotation
      auto_disable_old: true
      # Keep last N versions enabled
      keep_versions: 2
```

### Customer-Managed Encryption Keys (CMEK)

```yaml
backends:
  gcp:
    project: "my-project-id"
    cmek:
      enabled: true
      key_name: "projects/my-project/locations/global/keyRings/my-keyring/cryptoKeys/secrets-key"
```

### VPC Service Controls

For enhanced security:

```yaml
backends:
  gcp:
    project: "my-project-id"
    vpc_service_controls:
      enabled: true
      # Access only through VPC
      perimeter: "accessPolicies/123456/servicePerimeters/my-perimeter"
```

### Pub/Sub Notifications

Receive rotation notifications:

```yaml
backends:
  gcp:
    project: "my-project-id"
    notifications:
      topic: "projects/my-project/topics/secret-rotations"
      # Subscribe to rotation events
      subscribe_rotations: true
```

---

## Multi-Backend Configuration

Configure multiple backends with routing:

```yaml
secrets:
  backends:
    vault:
      address: "https://vault.example.com:8200"
      auth:
        method: kubernetes
        role: "keystone"

    aws:
      region: "us-west-2"
      auth:
        method: iam_role

    azure:
      vault_url: "https://myvault.vault.azure.net"
      auth:
        method: managed_identity

    gcp:
      project: "my-project"
      auth:
        method: workload_identity

  # Route secrets to appropriate backends
  routing:
    "vault/*": vault
    "database/*": vault
    "aws/*": aws
    "azure/*": azure
    "gcp/*": gcp

  # Failover configuration
  failover:
    enabled: true
    # Try alternative backend on failure
    groups:
      production:
        primary: vault
        secondary: aws
```

## Health Monitoring

Monitor backend health:

```yaml
secrets:
  health:
    # Check interval
    interval: 30s
    # Timeout for health checks
    timeout: 5s
    # Circuit breaker settings
    circuit_breaker:
      failure_threshold: 5
      recovery_timeout: 60s
```

## Next Steps

- [Rotation Strategies](/docs/operations/secrets-rotation/) - Configure secret rotation
- [Security Guide](/docs/operations/secrets-security/) - Security best practices
- [Troubleshooting](/docs/operations/secrets-troubleshooting/) - Common issues
