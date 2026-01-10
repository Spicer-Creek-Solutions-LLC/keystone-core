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
| CA | `/etc/kscore/certs/ca.crt` | Root CA | 10 years |
| Server | `/etc/kscore/certs/server.crt` | API/NATS | 1 year |
| Agent | `/etc/kscore/certs/agent.crt` | Agent auth | 1 year |
| etcd | `/etc/etcd/certs/etcd.crt` | etcd cluster | 1 year |

## Procedure

### Step 1: Check Current Certificate Status

```bash
# Check all certificate expiry dates
kscorectl certs status

# Output example:
# Certificate          Expires             Days Left  Status
# CA                   2034-01-15          3650       OK
# Server               2025-01-15          30         WARNING
# Agent Template       2025-01-15          30         WARNING
# etcd                 2025-02-15          61         OK

# Check specific certificate
openssl x509 -in /etc/kscore/certs/server.crt -noout -dates
```

### Step 2: Backup Current Certificates

```bash
# Backup all certificates
kscore-backup create \
  --components certificates \
  --dest /backup/certs-$(date +%Y%m%d)

# Verify backup
ls -la /backup/certs-*/
```

### Step 3: Generate New Certificates

#### Option A: Using Keystone Core CA

```bash
# Rotate server certificate (uses existing CA)
kscorectl certs rotate --component server

# Rotate agent certificates
kscorectl certs rotate --component agent
```

#### Option B: Using External PKI

```bash
# Generate CSR
kscorectl certs csr --component server --output /tmp/server.csr

# Submit CSR to external CA
# ... (external process)

# Import signed certificate
kscorectl certs import \
  --component server \
  --cert /path/to/new-server.crt \
  --key /path/to/server.key
```

#### Option C: Full CA Rotation

```bash
# Generate new CA (careful - affects all certificates)
kscorectl certs rotate-ca

# This will:
# 1. Generate new CA
# 2. Re-issue all server certificates
# 3. Re-issue all agent certificates
# 4. Trigger rolling restart
```

### Step 4: Distribute New Certificates

```bash
# Certificates are automatically distributed via state management
# Apply certificate state
kscorectl state apply /etc/kscore/states/certificates.yaml

# For manual distribution:
for node in ks-server-1 ks-server-2 ks-server-3; do
  scp /etc/kscore/certs/*.crt $node:/etc/kscore/certs/
  scp /etc/kscore/certs/*.key $node:/etc/kscore/certs/
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
  kscorectl cluster uncordon $node
done
```

### Step 6: Update Agents

```bash
# Agents need trust bundle update if CA changed
# This is handled automatically via NATS

# Verify agents have new trust bundle
kscorectl agent list --show-cert-expiry

# For manual agent update:
# On each agent node:
scp /etc/kscore/certs/ca.crt agent-node:/etc/kscore/certs/
ssh agent-node "sudo systemctl restart kscore-agent"
```

### Step 7: Verification

```bash
# Verify new certificates
kscorectl certs verify

# Test TLS connections
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  openssl s_client -connect $node:8080 </dev/null 2>/dev/null | \
    openssl x509 -noout -dates
done

# Verify agent connectivity
kscorectl agent ping --all
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
kscore-backup restore \
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
openssl x509 -in /etc/kscore/certs/server.crt -noout -text

# Check certificate chain
openssl verify -CAfile /etc/kscore/certs/ca.crt /etc/kscore/certs/server.crt

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
# /etc/kscore/states/cert-monitoring.yaml
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
