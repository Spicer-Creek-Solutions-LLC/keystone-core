# Runbook: Backup and Restore

## Overview

This runbook covers backup creation, verification, and restoration procedures for Keystone Core.

## Prerequisites

- [ ] Backup destination configured and accessible
- [ ] Encryption keys available (if using encryption)
- [ ] Sufficient storage space at destination
- [ ] Access to control plane nodes

## Backup Procedures

### Create Full Backup

```bash
# Create full backup to local storage
kscore-backup create \
  --type full \
  --dest /backup/keystone

# Create full backup to S3
kscore-backup create \
  --type full \
  --dest s3://keystone-backups/$(date +%Y/%m/%d)/ \
  --encrypt \
  --encrypt-recipient age1...

# Create backup with specific label
kscore-backup create \
  --type full \
  --dest /backup/keystone \
  --label "pre-upgrade-1.6.0"
```

### Create Incremental Backup

```bash
# Incremental backup (database changes only)
kscore-backup create \
  --type incremental \
  --dest /backup/keystone \
  --base-backup /backup/keystone/full-2024-01-15.tar.gz
```

### Create Component-Specific Backup

```bash
# Database only
kscore-backup create \
  --components database \
  --dest /backup/db-only

# Configuration only
kscore-backup create \
  --components config,certificates \
  --dest /backup/config-only

# JetStream only
kscore-backup create \
  --components jetstream \
  --dest /backup/jetstream-only
```

### Verify Backup

```bash
# Verify backup integrity
kscore-backup verify /backup/keystone/backup-2024-01-15.tar.gz

# Expected output:
# Verifying backup...
# - Manifest: OK
# - Checksum: OK
# - Components: 5/5 OK
# - Decryptable: OK (if encrypted)
# Backup verification passed
```

### List Backups

```bash
# List local backups
kscore-backup list --dest /backup/keystone

# List S3 backups
kscore-backup list --dest s3://keystone-backups/

# List with details
kscore-backup list --dest /backup/keystone --verbose

# Output:
# Backup                           Size      Date                 Components
# backup-2024-01-15T02-00-00.tar.gz  1.2GB    2024-01-15 02:00:00  full
# backup-2024-01-14T02-00-00.tar.gz  1.1GB    2024-01-14 02:00:00  full
# backup-2024-01-13T02-00-00.tar.gz  1.1GB    2024-01-13 02:00:00  full
```

## Restore Procedures

### Pre-Restore Checklist

- [ ] Identify correct backup to restore
- [ ] Verify backup integrity
- [ ] Ensure decryption keys available
- [ ] Plan for service interruption
- [ ] Notify stakeholders

### Full Restore

```bash
# Stop services on all nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "sudo systemctl stop kscore-server"
done

# Restore on first node
kscore-bootstrap restore \
  --backup /backup/keystone/backup-2024-01-15.tar.gz \
  --decrypt-identity /secure/backup-key.txt

# Start services
sudo systemctl start kscore-server

# Rejoin other nodes
# (existing data will be replaced with restored data via cluster sync)
```

### Partial Restore

```bash
# Restore configuration only
kscore-bootstrap restore \
  --backup /backup/keystone/backup.tar.gz \
  --components config \
  --no-restart

# Apply restored config
sudo systemctl restart kscore-server

# Restore certificates only
kscore-bootstrap restore \
  --backup /backup/keystone/backup.tar.gz \
  --components certificates \
  --no-restart
```

### Restore to Different Cluster

```bash
# Extract backup
mkdir /tmp/restore
tar -xzf /backup/keystone/backup.tar.gz -C /tmp/restore

# Modify configuration for new environment
vim /tmp/restore/config/server.yaml

# Import into new cluster
kscore-bootstrap import \
  --from-backup /tmp/restore \
  --new-cluster-name new-keystone
```

### Point-in-Time Recovery (PostgreSQL)

```bash
# Requires WAL archiving configured
# Restore to specific timestamp
kscore-bootstrap restore \
  --backup /backup/base-2024-01-15.tar.gz \
  --target-time "2024-01-15 14:30:00 UTC" \
  --wal-archive s3://keystone-wal/
```

## Verification Checklist

After restore:

- [ ] Server starts successfully
- [ ] Cluster health is healthy
- [ ] Database queries work
- [ ] All expected data present
- [ ] Agents reconnect
- [ ] Scheduled jobs resume

## Troubleshooting

### Backup Fails

```bash
# Check disk space
df -h /backup

# Check destination permissions
ls -la /backup/

# Check cloud credentials
aws s3 ls s3://keystone-backups/

# Run with debug logging
kscore-backup create --dest /backup --debug
```

### Restore Fails

```bash
# Verify backup integrity
kscore-backup verify /backup/backup.tar.gz

# Check decryption key
kscore-backup verify /backup/backup.tar.gz --decrypt-identity /path/to/key

# Extract and inspect manually
tar -tzf /backup/backup.tar.gz

# Check logs
journalctl -u kscore-server -n 100
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "Backup destination not accessible" | Network/permissions | Check connectivity and credentials |
| "Decryption failed" | Wrong key | Verify correct identity file |
| "Checksum mismatch" | Corrupt backup | Use different backup |
| "Incompatible version" | Version mismatch | Use compatible restore method |

## Post-Procedure

1. [ ] Verify restored data integrity
2. [ ] Test critical functionality
3. [ ] Update monitoring
4. [ ] Document restore details
5. [ ] Notify stakeholders

## Appendix: Backup Schedule Recommendations

| Environment | Full Backup | Incremental | Retention |
|-------------|-------------|-------------|-----------|
| Production | Daily 2AM | Every 6 hours | 30 days |
| Staging | Daily | N/A | 7 days |
| Development | Weekly | N/A | 3 days |

## Appendix: Backup Storage Options

### Local Storage

Best for small deployments or as a staging area before cloud upload.

```yaml
# Configuration for local storage
backup:
  storage:
    type: local
    path: /backup/keystone
    # Recommended: Use dedicated partition or volume
    # - ext4/xfs with journaling
    # - RAID-1 for redundancy
    # - NVMe SSD for performance

  # Retention
  retention:
    full_backups: 7
    incremental_backups: 30
    cleanup_schedule: "0 3 * * *"  # 3AM daily

  # Permissions (critical)
  permissions:
    directory_mode: "0700"
    file_mode: "0600"
    owner: kscore
    group: kscore
```

**Local Storage Recommendations**:

| Requirement | Recommendation |
|-------------|----------------|
| Filesystem | ext4 or XFS with journaling |
| RAID | RAID-1 or RAID-10 for redundancy |
| Storage | 3x expected backup size minimum |
| Backup of backups | Replicate to remote location |

### S3-Compatible Storage

Supports AWS S3, MinIO, Ceph, DigitalOcean Spaces, Backblaze B2, and others.

```yaml
# AWS S3 configuration
backup:
  storage:
    type: s3
    bucket: myorg-keystone-backups
    region: us-east-1
    prefix: keystone/production/

    # Authentication options
    auth:
      # Option 1: IAM role (recommended for AWS)
      use_iam_role: true

      # Option 2: Static credentials
      access_key_id: ${AWS_ACCESS_KEY_ID}
      secret_access_key: ${AWS_SECRET_ACCESS_KEY}

      # Option 3: Profile-based
      profile: keystone-backup

    # Storage class for cost optimization
    storage_class: STANDARD_IA  # Infrequent Access

    # Enable server-side encryption
    server_side_encryption: AES256

    # Optional: Use customer-managed key
    kms_key_id: arn:aws:kms:us-east-1:123456789012:key/abc123

    # Transfer settings
    transfer:
      multipart_threshold: 100MB
      multipart_chunksize: 25MB
      max_concurrency: 10
```

**S3 Lifecycle Policy** (recommended):

```json
{
  "Rules": [
    {
      "ID": "keystone-backup-lifecycle",
      "Status": "Enabled",
      "Filter": { "Prefix": "keystone/production/" },
      "Transitions": [
        { "Days": 30, "StorageClass": "STANDARD_IA" },
        { "Days": 90, "StorageClass": "GLACIER" }
      ],
      "Expiration": { "Days": 365 }
    }
  ]
}
```

### Azure Blob Storage

```yaml
# Azure Blob configuration
backup:
  storage:
    type: azure
    account_name: mystorageaccount
    container: keystone-backups
    prefix: production/

    # Authentication options
    auth:
      # Option 1: Managed Identity (recommended)
      use_managed_identity: true

      # Option 2: Connection string
      connection_string: ${AZURE_STORAGE_CONNECTION_STRING}

      # Option 3: Account key
      account_key: ${AZURE_STORAGE_ACCOUNT_KEY}

    # Storage tier
    tier: Cool  # Hot, Cool, or Archive

    # Server-side encryption with customer key
    encryption:
      enabled: true
      key_vault_uri: https://mykeyvault.vault.azure.net/
      key_name: keystone-backup-key
```

### Google Cloud Storage

```yaml
# GCS configuration
backup:
  storage:
    type: gcs
    bucket: myorg-keystone-backups
    prefix: production/

    # Authentication options
    auth:
      # Option 1: Service account (file)
      credentials_file: /etc/kscore/gcs-credentials.json

      # Option 2: Workload Identity (GKE)
      use_workload_identity: true

      # Option 3: Default application credentials
      use_default_credentials: true

    # Storage class
    storage_class: NEARLINE  # STANDARD, NEARLINE, COLDLINE, ARCHIVE

    # Customer-managed encryption key
    kms_key: projects/myproject/locations/us/keyRings/kr/cryptoKeys/key
```

### SFTP Storage

```yaml
# SFTP configuration
backup:
  storage:
    type: sftp
    host: backup.example.com
    port: 22
    path: /backups/keystone

    # Authentication
    auth:
      username: backup-user
      # Option 1: SSH key (recommended)
      private_key_file: /etc/kscore/backup-ssh-key
      # Option 2: Password (not recommended)
      password: ${SFTP_PASSWORD}

    # Known hosts verification
    known_hosts_file: /etc/kscore/known_hosts
    # Or disable verification (not recommended for production)
    # skip_host_verification: true

    # Transfer settings
    concurrent_transfers: 4
```

### NFS Storage

```yaml
# NFS configuration
backup:
  storage:
    type: nfs
    server: nas.example.com
    share: /exports/keystone-backups
    mount_point: /mnt/backup

    # Mount options
    mount_options:
      - "vers=4.1"
      - "hard"
      - "intr"
      - "rsize=1048576"
      - "wsize=1048576"
      - "timeo=600"

    # Path within mount
    path: /production
```

## Appendix: Encryption Configuration

### Age Encryption (Recommended)

Age is a modern, secure encryption tool with simple key management.

```bash
# Generate new encryption key pair
age-keygen -o /secure/backup-key.txt

# Output:
# Public key: age1xyz...

# The public key is used for encryption (safe to store in config)
# The private key is for decryption (keep secure!)
```

```yaml
# Encryption configuration with Age
backup:
  encryption:
    enabled: true
    method: age

    # Multiple recipients for disaster recovery
    recipients:
      - age1primary...   # Primary operations key
      - age1dr...        # Disaster recovery key (stored separately)
      - age1security...  # Security team key

    # Optional: Identity file for verification
    identity_file: /secure/backup-key.txt
```

**Backup with Age encryption**:

```bash
# Encrypt backup for multiple recipients
kscore-backup create \
  --dest s3://keystone-backups/ \
  --encrypt \
  --encrypt-recipient age1primary... \
  --encrypt-recipient age1dr...

# Restore with decryption
kscore-backup restore \
  --backup s3://keystone-backups/backup.tar.gz.age \
  --decrypt-identity /secure/backup-key.txt
```

### GPG Encryption

```yaml
# GPG encryption configuration
backup:
  encryption:
    enabled: true
    method: gpg

    # GPG key ID or email
    recipients:
      - backup@example.com
      - security@example.com

    # GPG home directory (if non-default)
    gnupg_home: /etc/kscore/gnupg

    # Signing key (optional)
    sign: true
    signing_key: signing@example.com
```

**GPG key setup**:

```bash
# Generate GPG key for backups
gpg --full-generate-key
# Select: RSA and RSA, 4096 bits, never expires

# Export public key (for encryption servers)
gpg --armor --export backup@example.com > backup-pubkey.asc

# Import on backup servers
gpg --import backup-pubkey.asc

# Trust the key
gpg --edit-key backup@example.com trust
```

### AES-256 Encryption (Passphrase)

For simpler deployments without key infrastructure.

```yaml
# Passphrase-based encryption
backup:
  encryption:
    enabled: true
    method: aes256

    # Passphrase from environment variable
    passphrase: ${BACKUP_ENCRYPTION_PASSPHRASE}

    # Or from file
    passphrase_file: /secure/backup-passphrase

    # Key derivation
    kdf: argon2id
    kdf_iterations: 10
    kdf_memory: 1GB
```

**Passphrase management**:

```bash
# Generate strong passphrase
openssl rand -base64 32 > /secure/backup-passphrase
chmod 600 /secure/backup-passphrase

# Store in HashiCorp Vault
vault kv put secret/keystone/backup passphrase="$(cat /secure/backup-passphrase)"

# Retrieve during backup
export BACKUP_ENCRYPTION_PASSPHRASE=$(vault kv get -field=passphrase secret/keystone/backup)
kscore-backup create --dest /backup --encrypt
```

### Hardware Security Modules (HSM)

For high-security environments requiring FIPS 140-2/3 compliance.

```yaml
# HSM encryption configuration
backup:
  encryption:
    enabled: true
    method: hsm

    # PKCS#11 provider
    pkcs11:
      library: /usr/lib/softhsm/libsofthsm2.so
      slot: 0
      pin: ${HSM_PIN}
      key_label: keystone-backup-key

    # Or AWS CloudHSM
    cloudhsm:
      cluster_id: cluster-abc123
      key_label: keystone-backup-key
      credentials_file: /etc/kscore/cloudhsm-credentials.json
```

### Encryption Key Management Best Practices

| Practice | Recommendation |
|----------|----------------|
| Key storage | Never store with backups; use separate secure storage |
| Key rotation | Rotate encryption keys annually |
| Key escrow | Store DR copy with trusted third party |
| Access control | Limit key access to authorized personnel |
| Audit logging | Log all key access and usage |
| Testing | Monthly test decryption with DR key |

### Multi-Layer Encryption

For defense in depth:

```yaml
# Client-side + server-side encryption
backup:
  encryption:
    # Layer 1: Client-side encryption (before upload)
    client_side:
      enabled: true
      method: age
      recipients: [age1xyz...]

    # Layer 2: Server-side encryption (at rest)
    server_side:
      enabled: true
      # S3 SSE-KMS
      method: aws-kms
      kms_key_id: arn:aws:kms:us-east-1:123456789012:key/abc123

  # Layer 3: TLS in transit (always enabled)
  transport:
    tls: true
    min_version: "1.3"
```

### Encryption Verification

```bash
# Verify backup is encrypted
file backup.tar.gz.age
# Output: backup.tar.gz.age: data

# Verify with specific identity
kscore-backup verify \
  --backup backup.tar.gz.age \
  --decrypt-identity /secure/backup-key.txt

# Test decryption without full restore
kscore-backup test-decrypt \
  --backup backup.tar.gz.age \
  --identity /secure/backup-key.txt

# List available decryption identities
kscore-backup info \
  --backup backup.tar.gz.age
# Output:
# Encryption: age
# Recipients: 3
# - age1primary... (Primary)
# - age1dr... (DR)
# - age1security... (Security)
```

### Disaster Recovery Key Procedures

**Key Escrow Setup**:

```bash
# Split DR key using Shamir's Secret Sharing
ssss-split -t 3 -n 5 < /secure/backup-key.txt > shares.txt

# Distribute shares to 5 trustees
# Any 3 shares can reconstruct the key
```

**Key Recovery**:

```bash
# Collect 3+ shares from trustees
ssss-combine -t 3 > /tmp/recovered-key.txt
# Enter 3 shares...

# Use recovered key for restore
kscore-backup restore \
  --backup backup.tar.gz.age \
  --decrypt-identity /tmp/recovered-key.txt

# Securely delete temporary key
shred -u /tmp/recovered-key.txt
```

### Compliance Mappings

| Requirement | Configuration |
|-------------|---------------|
| HIPAA | AES-256 + key management + audit logs |
| PCI-DSS | HSM or KMS + key rotation + access control |
| SOC 2 | Encryption at rest + in transit + key escrow |
| GDPR | Encryption + key access audit + deletion capability |
| FedRAMP | FIPS 140-2 validated encryption (HSM) |

## Appendix: Backup Storage Sizing

```
Estimate backup size:
- Database: ~1-10GB (depends on state count)
- Configuration: <1MB
- Certificates: <1MB
- JetStream: 1-100GB (depends on event retention)
- etcd: 100MB-10GB (depends on cluster data)

Monthly storage = (daily_size × 7) + (weekly_size × 4) + (monthly_size × 12)
```
