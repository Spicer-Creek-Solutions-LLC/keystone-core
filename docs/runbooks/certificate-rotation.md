# Runbook: Certificate Rotation

## Overview

This runbook covers TLS certificate rotation for Keystone Core components.

## Prerequisites

- [ ] Access to CA key or new certificates
- [ ] Maintenance window scheduled
- [ ] Backup of current certificates
- [ ] All nodes accessible

## Trigger Conditions

- Certificate expiry approaching (< 30 days)
- Certificate compromise suspected
- CA rotation required
- Compliance requirement

## Certificate Inventory

| Certificate | Location | Purpose | Typical Validity |
|-------------|----------|---------|------------------|
| CA | `/etc/keystone-core/certs/ca.crt` | Root CA | 10 years |
| Server | `/etc/keystone-core/certs/server.crt` | API/NATS | 1 year |
| Agent | `/etc/keystone-core/certs/agent.crt` | Agent auth | 1 year |
| etcd | `/etc/etcd/certs/etcd.crt` | etcd cluster | 1 year |

## Procedure

### Step 1: Check Current Certificate Status

```bash
# Check all certificate expiry dates
for cert in /etc/keystone-core/certs/*.crt; do
  echo "=== $(basename $cert) ==="
  openssl x509 -in "$cert" -noout -subject -enddate
  DAYS=$(( ($(date -d "$(openssl x509 -in "$cert" -noout -enddate | cut -d= -f2)" +%s) - $(date +%s)) / 86400 ))
  echo "Days remaining: $DAYS"
done

# Check specific certificate
openssl x509 -in /etc/keystone-core/certs/server.crt -noout -dates
```

### Step 2: Backup Current Certificates

```bash
# Backup all certificates
kscore-cluster-backup create \
  --components certificates \
  --dest /backup/certs-$(date +%Y%m%d)

# Verify backup
ls -la /backup/certs-*/
```

### Step 3: Generate New Certificates

#### Option A: Using Keystone Core Identity System

```bash
# Rotate CA using the identity plugin
kscorectl identity ca rotate --force

# Regenerate all agent certificates
kscorectl agents certificates regenerate --all --force
```

#### Option B: Using External PKI

```bash
# Generate CSR with openssl
openssl req -new -newkey rsa:4096 -nodes \
  -keyout /tmp/server.key -out /tmp/server.csr \
  -subj "/CN=kscore-server"

# Submit CSR to external CA
# ... (external process)

# Install signed certificate
cp /path/to/new-server.crt /etc/keystone-core/certs/server.crt
cp /path/to/server.key /etc/keystone-core/certs/server.key
chmod 600 /etc/keystone-core/certs/server.key
```

#### Option C: Full CA Rotation

```bash
# Rotate CA (careful - affects all certificates)
kscorectl identity ca rotate --force

# This will:
# 1. Generate new CA
# 2. Trigger re-issuance of agent certificates
# 3. Require rolling restart of all services

# Regenerate all agent certificates with new CA
kscorectl agents certificates regenerate --all --force
```

### Step 4: Distribute New Certificates

```bash
# Certificates are automatically distributed via state management
# Apply certificate state
kscorectl state apply /etc/keystone-core/states/certificates.yaml

# For manual distribution:
for node in ks-server-1 ks-server-2 ks-server-3; do
  scp /etc/keystone-core/certs/*.crt $node:/etc/keystone-core/certs/
  scp /etc/keystone-core/certs/*.key $node:/etc/keystone-core/certs/
done
```

### Step 5: Rolling Restart

```bash
# Restart services to pick up new certificates
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Restarting $node..."

  # Drain node
  kscorectl cluster drain $node

  # Restart server
  ssh $node "sudo systemctl restart kscore-server"

  # Wait for healthy
  sleep 30
  until kscorectl cluster health | grep -q "healthy"; do
    sleep 5
  done

  # Uncordon
  kscorectl cluster undrain $node
done
```

### Step 6: Update Agents

```bash
# Agents need trust bundle update if CA changed
# This is handled automatically via NATS

# Verify agents have updated certificates
kscorectl agents list -o wide

# For manual agent update:
# On each agent node:
scp /etc/keystone-core/certs/ca.crt agent-node:/etc/keystone-core/certs/
ssh agent-node "sudo systemctl restart kscore-agent"
```

### Step 7: Verification

```bash
# Verify new certificates
openssl verify -CAfile /etc/keystone-core/certs/ca.crt /etc/keystone-core/certs/server.crt

# Test TLS connections
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  openssl s_client -connect $node:8080 </dev/null 2>/dev/null | \
    openssl x509 -noout -dates
done

# Verify agent connectivity
kscorectl agents verify --all
```

## Verification Checklist

- [ ] New certificates installed on all servers
- [ ] All servers restarted successfully
- [ ] Cluster health is healthy
- [ ] All agents reconnected
- [ ] TLS connections use new certificates
- [ ] Certificate expiry > 300 days

## Rollback

If certificate rotation fails:

```bash
# Restore previous certificates
kscore-cluster-backup restore \
  --backup /backup/certs-*/latest.tar.gz \
  --components certificates

# Restart services
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "sudo systemctl restart kscore-server"
done
```

## Post-Procedure

1. [ ] Update certificate monitoring
2. [ ] Update documentation with new expiry dates
3. [ ] Schedule next rotation (before expiry)
4. [ ] Close change ticket
5. [ ] Archive old certificates (for audit)

## Appendix: Certificate Commands

```bash
# View certificate details
openssl x509 -in /etc/keystone-core/certs/server.crt -noout -text

# Check certificate chain
openssl verify -CAfile /etc/keystone-core/certs/ca.crt /etc/keystone-core/certs/server.crt

# Check key matches certificate
diff <(openssl x509 -in cert.crt -noout -modulus) \
     <(openssl rsa -in key.key -noout -modulus)

# Generate self-signed for testing
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Check expiry days
echo $(( ($(date -d "$(openssl x509 -in cert.crt -noout -enddate | cut -d= -f2)" +%s) - $(date +%s)) / 86400 ))
```

## Appendix: Automation

```yaml
# Automatic certificate monitoring state
# /etc/keystone-core/states/cert-monitoring.yaml
certificate_monitor:
  check_expiry:
    state: configured
    warning_days: 30
    critical_days: 7
    action: alert

  auto_rotate:
    state: enabled
    before_expiry_days: 30
    components:
      - server
      - agent
```
