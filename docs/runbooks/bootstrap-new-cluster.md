# Runbook: Bootstrap New Cluster

## Overview

This runbook covers bootstrapping a new Keystone Core cluster from scratch using a seed configuration.

## Prerequisites

- [ ] Infrastructure provisioned (servers, networking, storage)
- [ ] SSH access to all nodes
- [ ] Seed configuration file prepared (`seed.yaml`)
- [ ] DNS entries configured (if applicable)
- [ ] Firewall rules configured (ports 8080, 4222, 6222, 2379)
- [ ] Required credentials available (database, cloud storage)

## Trigger Conditions

- Initial Keystone Core deployment
- Creating a new isolated cluster
- DR cluster provisioning

## Procedure

### Step 1: Validate Prerequisites

```bash
# Verify SSH access to all nodes
for node in ks-server-1 ks-server-2 ks-server-3; do
  ssh $node "hostname && uname -a"
done

# Verify network connectivity between nodes
# From each node, test connectivity to others
ping -c 3 10.0.1.10
ping -c 3 10.0.1.11
ping -c 3 10.0.1.12
```

### Step 2: Prepare Seed Configuration

```bash
# Review seed configuration
cat seed.yaml

# Validate configuration
kscore-bootstrap validate --config seed.yaml

# Expected output: "Configuration valid"
```

### Step 3: Copy Bootstrap Binary

```bash
# Copy kscore-bootstrap to first node
scp kscore-bootstrap ks-server-1:/usr/local/bin/
scp seed.yaml ks-server-1:/tmp/
```

### Step 4: Run Bootstrap (Dry-Run)

```bash
# SSH to first node
ssh ks-server-1

# Run bootstrap in dry-run mode first
kscore-bootstrap seed --config /tmp/seed.yaml --dry-run

# Review planned actions
# Verify no errors or warnings
```

### Step 5: Execute Bootstrap

```bash
# Execute actual bootstrap
kscore-bootstrap seed --config /tmp/seed.yaml

# Monitor progress
# Bootstrap will display progress for each step

# Expected completion message:
# "Bootstrap completed successfully"
```

### Step 6: Verify Bootstrap

```bash
# Check bootstrap status
kscore-bootstrap status

# Verify server is running
systemctl status kscore-server

# Verify NATS is healthy
nats server info

# Check cluster health
kscorectl cluster health
```

### Step 7: Register Additional Nodes (HA)

```bash
# For each additional server node
# Get join token from first node
JOIN_TOKEN=$(kscorectl cluster token)

# On additional nodes
kscore-bootstrap import --join https://ks-server-1:8080 --token $JOIN_TOKEN
```

### Step 8: Verify HA Cluster

```bash
# Verify all nodes are members
kscorectl cluster members

# Verify quorum
kscorectl cluster health

# Expected: All nodes healthy, quorum established
```

## Verification

- [ ] All server nodes are running: `systemctl status kscore-server`
- [ ] Cluster health is healthy: `kscorectl cluster health`
- [ ] NATS cluster is formed: `nats server info`
- [ ] Leader elected: `kscorectl cluster leader`
- [ ] API is accessible: `curl -k https://ks-server-1:8080/health/ready`

## Rollback

If bootstrap fails:

```bash
# Cleanup failed bootstrap
kscore-bootstrap cleanup --force

# Review logs for errors
journalctl -u kscore-server -n 100

# Fix issues and retry
```

## Post-Procedure

1. [ ] Document cluster details in CMDB
2. [ ] Configure monitoring and alerting
3. [ ] Schedule first backup
4. [ ] Configure agent deployment
5. [ ] Update DNS if needed
6. [ ] Notify stakeholders of completion

## Appendix: Common Issues

### Issue: Certificate Generation Fails

```bash
# Check for existing certificates
ls -la /etc/kscore/certs/

# Remove stale certificates
rm -rf /etc/kscore/certs/*

# Retry bootstrap
```

### Issue: NATS Cluster Won't Form

```bash
# Verify network connectivity on port 6222
nc -zv 10.0.1.11 6222

# Check NATS logs
journalctl -u nats-server -n 100

# Verify routes in NATS config
cat /etc/nats/nats.conf
```

### Issue: Database Connection Fails

```bash
# Test database connectivity
psql -h postgres.example.com -U keystone -d keystone -c "SELECT 1"

# Verify credentials
echo $POSTGRES_PASSWORD

# Check firewall rules
nc -zv postgres.example.com 5432
```
