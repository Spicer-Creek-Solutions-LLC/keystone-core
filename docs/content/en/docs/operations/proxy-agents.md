---
title: "Proxy Agent Operations"
linkTitle: "Proxy Agents"
weight: 23
description: >
  Operating Keystone Core proxy agents for managing unmanaged devices via SSH, SNMP, REST, and WinRM.
---

## Overview

Proxy agents enable Keystone Core to manage devices that cannot run the native agent software. This guide covers operational procedures for:

- **Deployment**: Setting up proxy agents
- **Device Management**: Adding, configuring, and removing proxied devices
- **Credential Management**: Secure credential storage and rotation
- **Discovery**: Auto-discovering network devices
- **State Management**: Applying configurations to proxied devices
- **Monitoring**: Metrics and health checks
- **Troubleshooting**: Common issues and solutions

## Deployment

### Single Proxy Agent

For managing devices in a single network segment:

```bash
# Start proxy agent
kscore-agent --proxy --config /etc/kscore/proxy-agent.yaml
```

**Configuration (`/etc/kscore/proxy-agent.yaml`):**

```yaml
agent:
  id: proxy-datacenter-1
  mode: proxy
  labels:
    location: datacenter-1
    type: proxy

nats:
  url: nats://control-plane:4222
  tls:
    enabled: true
    ca_file: /etc/kscore/certs/ca.crt
    cert_file: /etc/kscore/certs/agent.crt
    key_file: /etc/kscore/certs/agent.key

proxy:
  # Device registry
  devices:
    config_file: /etc/kscore/devices.yaml
    sync_interval: 5m

  # Credential storage
  credentials:
    type: vault
    vault:
      address: https://vault.example.com:8200
      auth_method: kubernetes
      secret_path: secret/kscore/devices

  # Discovery settings
  discovery:
    enabled: true
    scan_interval: 1h
    networks:
      - 192.168.1.0/24
      - 192.168.2.0/24
    exclude_networks:
      - 192.168.1.0/30  # Management network
    auto_approve: false

  # Health monitoring
  health:
    check_interval: 1m
    timeout: 30s
    retries: 3
```

### Multiple Proxy Agents

For large deployments, deploy proxy agents per network segment:

```yaml
# proxy-agent-dc1.yaml
agent:
  id: proxy-dc1
  mode: proxy
  labels:
    datacenter: dc1
    type: proxy

proxy:
  discovery:
    networks:
      - 10.1.0.0/16

---
# proxy-agent-dc2.yaml
agent:
  id: proxy-dc2
  mode: proxy
  labels:
    datacenter: dc2
    type: proxy

proxy:
  discovery:
    networks:
      - 10.2.0.0/16
```

### Kubernetes Deployment

Deploy as a DaemonSet or Deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kscore-proxy-agent
spec:
  replicas: 2
  selector:
    matchLabels:
      app: kscore-proxy-agent
  template:
    metadata:
      labels:
        app: kscore-proxy-agent
    spec:
      serviceAccountName: kscore-proxy-agent
      containers:
        - name: proxy-agent
          image: kscore/agent:latest
          args: ["--proxy", "--config", "/etc/kscore/config.yaml"]
          env:
            - name: KSCORE_AGENT_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
          volumeMounts:
            - name: config
              mountPath: /etc/kscore
            - name: certs
              mountPath: /etc/kscore/certs
      volumes:
        - name: config
          configMap:
            name: kscore-proxy-config
        - name: certs
          secret:
            secretName: kscore-proxy-certs
```

## Device Management

### Adding Devices

**Via Configuration File:**

```yaml
# /etc/kscore/devices.yaml
devices:
  - id: core-router-01
    name: Core Router 1
    type: router
    vendor: cisco
    model: ISR 4451
    protocol: ssh
    address: 192.168.1.1
    port: 22
    credential_ref: cisco-ssh
    labels:
      site: headquarters
      role: core
    profile: cisco-ios

  - id: edge-switch-01
    name: Edge Switch 1
    type: switch
    vendor: arista
    protocol: ssh
    address: 192.168.1.10
    port: 22
    credential_ref: arista-ssh
    labels:
      site: headquarters
      role: edge
    profile: arista-eos
```

**Via CLI:**

```bash
# Add a device
kscorectl proxy device add \
  --id core-router-01 \
  --type router \
  --vendor cisco \
  --protocol ssh \
  --address 192.168.1.1 \
  --port 22 \
  --credential cisco-ssh \
  --profile cisco-ios

# Import from file
kscorectl proxy device import --file devices.yaml

# Bulk import from CSV
kscorectl proxy device import --file devices.csv --format csv
```

### Managing Devices

```bash
# List all proxied devices
kscorectl proxy device list

# List with filters
kscorectl proxy device list --vendor cisco --type router

# Show device details
kscorectl proxy device show core-router-01

# Update device
kscorectl proxy device update core-router-01 --labels role=distribution

# Remove device
kscorectl proxy device remove core-router-01

# Test connectivity
kscorectl proxy device test core-router-01
```

### Device Health Status

```bash
# Check device health
kscorectl proxy device health core-router-01

# Output:
# Device: core-router-01
# Status: healthy
# Last Check: 2024-01-15T10:23:45Z
# Protocol: SSH
# Connection: established
# Response Time: 45ms
# Uptime: 45d 12h 30m

# List unhealthy devices
kscorectl proxy device list --status unhealthy

# Check all devices
kscorectl proxy device health --all
```

## Credential Management

### Credential Backends

**File Backend (Development):**

```yaml
credentials:
  type: file
  file:
    path: /etc/kscore/credentials.yaml
    encryption_key_file: /etc/kscore/creds.key
```

**HashiCorp Vault (Production):**

```yaml
credentials:
  type: vault
  vault:
    address: https://vault.example.com:8200
    auth_method: kubernetes  # or token, approle
    secret_path: secret/data/kscore/devices
    # For token auth
    # token: ${VAULT_TOKEN}
    # For AppRole auth
    # role_id: ${VAULT_ROLE_ID}
    # secret_id: ${VAULT_SECRET_ID}
```

**Kubernetes Secrets:**

```yaml
credentials:
  type: kubernetes
  kubernetes:
    namespace: kscore
    secret_prefix: kscore-cred-
```

### Managing Credentials

```bash
# Add SSH credential with password
kscorectl proxy credential add cisco-ssh \
  --type ssh \
  --username admin \
  --password-prompt

# Add SSH credential with key
kscorectl proxy credential add cisco-ssh-key \
  --type ssh \
  --username admin \
  --private-key-file ~/.ssh/network_key

# Add SNMP v2c community
kscorectl proxy credential add snmp-community \
  --type snmp \
  --version v2c \
  --community public

# Add SNMP v3 credential
kscorectl proxy credential add snmpv3-auth \
  --type snmp \
  --version v3 \
  --username snmpuser \
  --auth-protocol sha256 \
  --auth-password-prompt \
  --priv-protocol aes256 \
  --priv-password-prompt

# Add REST API key
kscorectl proxy credential add pfsense-api \
  --type rest \
  --api-key-prompt

# List credentials
kscorectl proxy credential list

# Rotate credential
kscorectl proxy credential rotate cisco-ssh --password-prompt

# Delete credential
kscorectl proxy credential delete old-credential
```

### Credential Rotation

Set up automatic credential rotation:

```yaml
credentials:
  rotation:
    enabled: true
    interval: 90d
    notify_before: 14d
    notification_channel: slack
```

## Discovery Operations

### Network Scanning

```bash
# Start a discovery scan
kscorectl proxy discover scan \
  --networks 192.168.1.0/24,192.168.2.0/24 \
  --exclude 192.168.1.1

# View scan status
kscorectl proxy discover status

# List discovered devices
kscorectl proxy discover list

# Output:
# ID              | Address       | Vendor  | Type    | Status
# ----------------|---------------|---------|---------|--------
# discovered-001  | 192.168.1.10  | Cisco   | router  | pending
# discovered-002  | 192.168.1.20  | Arista  | switch  | pending
# discovered-003  | 192.168.1.30  | Unknown | unknown | pending
```

### Approval Workflow

```bash
# Approve a discovered device
kscorectl proxy discover approve discovered-001 \
  --id core-router-01 \
  --credential cisco-ssh \
  --profile cisco-ios

# Approve multiple devices
kscorectl proxy discover approve-all --vendor cisco \
  --credential cisco-ssh \
  --profile cisco-ios

# Reject a device
kscorectl proxy discover reject discovered-003

# Ignore a device (won't appear in future scans)
kscorectl proxy discover ignore 192.168.1.100
```

### Auto-Approval Rules

Configure rules for automatic approval:

```yaml
discovery:
  auto_approve_rules:
    - name: cisco-routers
      match:
        vendor: cisco
        type: router
      apply:
        credential_ref: cisco-ssh
        profile: cisco-ios
        labels:
          managed: "true"

    - name: arista-switches
      match:
        vendor: arista
        type: switch
      apply:
        credential_ref: arista-ssh
        profile: arista-eos
```

## State Management

### Applying States to Proxied Devices

Proxied devices support state modules:

```yaml
# Apply configuration to a Cisco router
configure_core_router:
  module: ios_config
  state: present
  target: "proxy:core-router-01"
  lines:
    - hostname core-router-01
    - ip domain-name example.com
    - ntp server 10.0.0.1

# Apply to multiple devices with targeting
configure_all_routers:
  module: ios_config
  state: present
  target: "proxy:vendor=cisco and type=router"
  lines:
    - logging host 10.0.0.100
    - logging trap informational
```

### Protocol-Specific Modules

**SSH Modules:**
- `ssh_file` - Manage files via SCP/SFTP
- `ssh_cmd` - Execute commands
- `ssh_service` - Manage services (Linux via SSH)

**SNMP Modules:**
- `snmp_value` - Set SNMP values
- `snmp_table` - Manage SNMP table entries

**REST Modules:**
- `http_config` - Configure via REST API
- `http_resource` - Manage REST resources

**Network Device Modules:**
- `ios_config` - Cisco IOS configuration
- `nxos_config` - Cisco NX-OS configuration
- `junos_config` - Juniper JUNOS configuration
- `eos_config` - Arista EOS configuration
- `vyos_config` - VyOS/EdgeOS configuration
- `pfsense_config` - pfSense configuration
- `opnsense_config` - OPNsense configuration

**WinRM Modules:**
- `winrm_file` - Manage Windows files
- `winrm_service` - Manage Windows services
- `winrm_registry` - Manage Windows registry
- `winrm_package` - Manage Windows packages

### Drift Detection

```bash
# Check for drift on a device
kscorectl proxy drift check core-router-01

# Check all devices
kscorectl proxy drift check --all

# View drift report
kscorectl proxy drift report core-router-01

# Auto-remediate drift
kscorectl proxy drift remediate core-router-01
```

## Monitoring

### Prometheus Metrics

Key metrics to monitor:

| Metric | Type | Description |
|--------|------|-------------|
| `kscore_proxy_devices_total` | Gauge | Total proxied devices by status |
| `kscore_proxy_device_health` | Gauge | Device health (1=healthy, 0=unhealthy) |
| `kscore_proxy_connections_total` | Counter | Connection attempts by protocol |
| `kscore_proxy_connection_errors_total` | Counter | Connection errors by device |
| `kscore_proxy_command_duration_seconds` | Histogram | Command execution latency |
| `kscore_proxy_commands_total` | Counter | Commands executed by protocol |
| `kscore_proxy_discovery_devices_found` | Counter | Devices found by discovery |
| `kscore_proxy_state_applications_total` | Counter | State applications by result |
| `kscore_proxy_drift_detected_total` | Counter | Drift detection events |

### Grafana Dashboard

Import the pre-built dashboard from `deploy/grafana/dashboards/proxy-agents.json`:

- Device health overview
- Protocol distribution
- Command success rates
- Connection latency
- Discovery statistics
- Drift detection events

### Health Endpoints

```bash
# Proxy agent health
curl http://localhost:8080/health/live

# Device status endpoint
curl http://localhost:8080/health/devices

# Detailed status
curl http://localhost:8080/health/status
```

### Alert Rules

```yaml
groups:
  - name: proxy-agents
    rules:
      - alert: ProxyDeviceUnhealthy
        expr: kscore_proxy_device_health == 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Proxied device {{ $labels.device_id }} unhealthy"

      - alert: ProxyConnectionFailure
        expr: rate(kscore_proxy_connection_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High connection failure rate for proxy agent"

      - alert: ProxyDeviceDrift
        expr: increase(kscore_proxy_drift_detected_total[1h]) > 0
        labels:
          severity: info
        annotations:
          summary: "Configuration drift detected on proxied devices"
```

## Troubleshooting

### Connection Issues

```bash
# Test device connectivity
kscorectl proxy device test core-router-01 --verbose

# Debug SSH connection
kscorectl proxy device test core-router-01 --protocol ssh --debug

# Test with specific credential
kscorectl proxy device test core-router-01 --credential cisco-ssh-alt

# Check from proxy agent logs
journalctl -u kscore-agent -f | grep "core-router-01"
```

### Common Connection Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `connection refused` | Port closed or firewall | Check device accessibility and firewall rules |
| `authentication failed` | Wrong credentials | Verify credential configuration |
| `timeout` | Network latency or device slow | Increase timeout or check network |
| `host key verification failed` | SSH host key changed | Update known hosts or disable strict checking |
| `SNMP timeout` | Wrong community/credentials | Verify SNMP configuration on device |

### Protocol-Specific Debugging

**SSH:**
```bash
# Test SSH directly
ssh -v admin@192.168.1.1

# Check SSH connectivity from proxy agent container
kubectl exec -it kscore-proxy-agent-xxx -- ssh -v admin@192.168.1.1
```

**SNMP:**
```bash
# Test SNMP v2c
snmpwalk -v2c -c public 192.168.1.10 system

# Test SNMP v3
snmpwalk -v3 -u snmpuser -l authPriv -a SHA -A authpass -x AES -X privpass 192.168.1.10 system
```

**REST:**
```bash
# Test REST API
curl -k -H "Authorization: Bearer $API_KEY" https://192.168.1.254/api/v1/status
```

**WinRM:**
```bash
# Test WinRM connectivity
winrm identify -r:https://192.168.1.100:5986 -u:Administrator -p:password
```

### Credential Issues

```bash
# Verify credential exists
kscorectl proxy credential show cisco-ssh

# Test credential against device
kscorectl proxy credential test cisco-ssh --device core-router-01

# Check credential backend connectivity
kscorectl proxy credential backend-status
```

### Discovery Issues

```bash
# Debug discovery scan
kscorectl proxy discover scan --networks 192.168.1.0/24 --debug

# Check scanner logs
journalctl -u kscore-agent | grep discovery

# Verify network access from proxy agent
kubectl exec -it kscore-proxy-agent-xxx -- ping 192.168.1.1
```

## Best Practices

### Deployment

- Deploy proxy agents close to managed devices (same network segment)
- Use dedicated service accounts for proxy agents
- Enable TLS for all proxy agent communication
- Use HashiCorp Vault or Kubernetes Secrets for production credentials

### Device Management

- Use consistent naming conventions for device IDs
- Apply labels for organization and targeting
- Use device profiles for vendor-specific settings
- Test connectivity before adding devices to production

### Security

- Use SNMP v3 with authPriv security level
- Enable SSH key-based authentication where possible
- Rotate credentials regularly
- Audit all proxy agent operations
- Use least-privilege accounts on managed devices

### Monitoring

- Monitor device health status
- Alert on connection failures
- Track drift detection events
- Review command execution patterns

### High Availability

- Deploy multiple proxy agents per network segment
- Use agent labels for redundancy
- Configure health check intervals appropriately
- Test failover procedures regularly

## See Also

- [Proxy Agents Concepts](/docs/concepts/proxy-agents/)
- [Configuration Reference - Proxy Agent](/docs/reference/configuration/#proxy-agent-configuration)
- [State Modules Reference](/docs/reference/modules/)
- [CLI Reference](/docs/reference/cli/)
