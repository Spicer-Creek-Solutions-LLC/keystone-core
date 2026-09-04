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
for cert in /etc/kscore/certs/*.crt; do
  echo "=== $(basename $cert) ==="
  openssl x509 -in "$cert" -noout -subject -enddate
  DAYS=$(( ($(date -d "$(openssl x509 -in "$cert" -noout -enddate | cut -d= -f2)" +%s) - $(date +%s)) / 86400 ))
  echo "Days remaining: $DAYS"
done

# Check specific certificate
openssl x509 -in /etc/kscore/certs/server.crt -noout -dates
```

### Step 2: Backup Current Certificates

```bash
# Back up the cert directory. v0.1 has no selective component backup;
# the certs are plain files, so copy the directory aside.
sudo cp -a /etc/kscore/certs /backup/certs-$(date +%Y%m%d)

# Verify backup
ls -la /backup/certs-*/
```

### Step 3: Generate New Certificates

#### Option A: Using Keystone Core Identity System

```bash
# Rotate the signing CA using the identity plugin
kscore-identity ca rotate-signing
```

> **v0.x scope note**: Per-agent certificate reissue/regeneration is not available
> in v0.1 (no agent-side cert-reload path or revocation yet); tracked in
> [`ROADMAP.md`](../project/ROADMAP.md). For now, rotating the signing CA
> (`kscore-identity ca rotate-signing`) plus re-bootstrapping affected agents is
> the available path.

#### Option B: Using External PKI

```bash
# Generate CSR with openssl
openssl req -new -newkey rsa:4096 -nodes \
  -keyout /tmp/server.key -out /tmp/server.csr \
  -subj "/CN=kscore-server"

# Submit CSR to external CA
# ... (external process)

# Install signed certificate
cp /path/to/new-server.crt /etc/kscore/certs/server.crt
cp /path/to/server.key /etc/kscore/certs/server.key
chmod 600 /etc/kscore/certs/server.key
```

#### Option C: Full CA Rotation

```bash
# Rotate the signing CA (careful - affects all certificates)
kscore-identity ca rotate-signing

# This will:
# 1. Generate a new signing CA
# 2. Update the trust bundle distributed to servers and agents
# 3. Require a rolling restart of all services
```

> **v0.x scope note**: Per-agent certificate reissue/regeneration is not available
> in v0.1 (no agent-side cert-reload path or revocation yet); tracked in
> [`ROADMAP.md`](../project/ROADMAP.md). After rotating the signing CA, affected
> agents must be re-bootstrapped to obtain certificates from the new CA.

### Step 4: Distribute New Certificates

```bash
# Certificates are automatically distributed via state management
# Apply certificate state
kscorectl state apply --target localhost /etc/kscore/states/certificates.yaml

# For manual distribution:
for node in ks-server-1 ks-server-2 ks-server-3; do
  scp /etc/kscore/certs/*.crt $node:/etc/kscore/certs/
  scp /etc/kscore/certs/*.key $node:/etc/kscore/certs/
done
```

### Step 5: Rolling Restart

> **v0.x scope note**: Per-node drain/undrain (cordoning) is not available in v0.1.
> The available cluster operation is evicting a member with
> `kscore-cluster remove <member-id>` (or simply stopping the node). The rolling
> restart below restarts one server at a time and waits for the cluster to report
> healthy before moving on, without draining.

```bash
# Restart services to pick up new certificates, one node at a time
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "Restarting $node..."

  # Restart server
  ssh $node "sudo systemctl restart kscore-server"

  # Wait for healthy before moving to the next node
  sleep 30
  until kscore-cluster status | grep -q "healthy"; do
    sleep 5
  done
done
```

### Step 6: Update Agents

```bash
# Agents need trust bundle update if CA changed
# This is handled automatically via NATS

# Verify agents have updated certificates
kscorectl agent list

# For manual agent update:
# On each agent node:
scp /etc/kscore/certs/ca.crt agent-node:/etc/kscore/certs/
ssh agent-node "sudo systemctl restart kscore-agent"
```

### Step 7: Verification

```bash
# Verify new certificates
openssl verify -CAfile /etc/kscore/certs/ca.crt /etc/kscore/certs/server.crt

# Test TLS connections
for node in ks-server-1 ks-server-2 ks-server-3; do
  echo "=== $node ==="
  openssl s_client -connect $node:8080 </dev/null 2>/dev/null | \
    openssl x509 -noout -dates
done

# Verify agent connectivity and certificates against the current trust bundle.
# Re-checks each agent's stored cert (chain + expiry + SPIFFE identity) and exits
# non-zero if any agent with a stored cert fails — ideal after a CA rotation.
kscorectl agent verify --all

# To verify a single agent:
# kscorectl agent verify <agent-id>
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
# Restore previous certificates: copy the saved directory back.
sudo cp -a /backup/certs-<timestamp>/. /etc/kscore/certs/

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
