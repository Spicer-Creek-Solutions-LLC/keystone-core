---
title: "Proxy Agent Operations"
linkTitle: "Proxy Agents"
weight: 23
description: >
  Operating Keystone Core proxy agents for managing unmanaged devices via SSH, SNMP, REST, WinRM, NETCONF, RESTCONF, and gNMI.
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
kscore-agent --proxy --config /etc/keystone-core/proxy-agent.yaml
```

**Configuration (`/etc/keystone-core/proxy-agent.yaml`):**

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
    ca_file: /etc/keystone-core/certs/ca.crt
    cert_file: /etc/keystone-core/certs/agent.crt
    key_file: /etc/keystone-core/certs/agent.key

proxy:
  # Device registry
  devices:
    config_file: /etc/keystone-core/devices.yaml
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
          args: ["--proxy", "--config", "/etc/keystone-core/config.yaml"]
          env:
            - name: KSCORE_AGENT_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
          volumeMounts:
            - name: config
              mountPath: /etc/keystone-core
            - name: certs
              mountPath: /etc/keystone-core/certs
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
# /etc/keystone-core/devices.yaml
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
    path: /etc/keystone-core/credentials.yaml
    encryption_key_file: /etc/keystone-core/creds.key
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
curl -k -H "Authorization: Bearer $API_KEY" https://192.168.1.254/api/status
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

## Vendor-Specific Configuration

This section provides detailed configuration guides for specific vendor platforms.

### Cisco IOS/IOS-XE

**Supported Models:** ISR routers, Catalyst switches, ASR routers

**Profile Configuration:**

```yaml
profiles:
  cisco-ios:
    vendor: cisco
    os: ios
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
      session_timeout: 300s
    parser:
      type: textfsm
      templates_dir: /etc/keystone-core/templates/cisco-ios
    commands:
      show_config: show running-config
      show_version: show version
      save_config: write memory
      config_mode: configure terminal
      exit_config: end
    prompts:
      login: "Username:"
      password: "Password:"
      exec: ">"
      privileged: "#"
      config: "(config)#"
    enable:
      required: true
      command: enable
      password_prompt: "Password:"
```

**Device Preparation:**

```bash
! Enable SSH on the device
configure terminal
hostname core-router-01
ip domain-name example.com
crypto key generate rsa modulus 2048
ip ssh version 2
ip ssh time-out 60
ip ssh authentication-retries 3

! Create management user
username kscore privilege 15 secret $STRONG_PASSWORD

! Configure VTY lines
line vty 0 15
 transport input ssh
 login local
 exec-timeout 15 0

! Enable SCP server (for file transfers)
ip scp server enable

! Configure logging
logging host 10.0.0.100
logging trap informational
logging source-interface Loopback0
```

**Example State Application:**

```yaml
cisco_ntp_config:
  module: ios_config
  state: present
  target: "proxy:vendor=cisco and os=ios"
  lines:
    - ntp server 10.0.0.1 prefer
    - ntp server 10.0.0.2
    - ntp update-calendar
  parents: []
  save_when: modified

cisco_acl_config:
  module: ios_config
  state: present
  target: "proxy:core-router-01"
  lines:
    - permit tcp any any eq 22
    - permit tcp any any eq 443
    - deny ip any any log
  parents:
    - ip access-list extended MANAGEMENT

cisco_interface_config:
  module: ios_config
  state: present
  target: "proxy:core-router-01"
  lines:
    - description Uplink to ISP
    - ip address 203.0.113.1 255.255.255.252
    - no shutdown
  parents:
    - interface GigabitEthernet0/0/0
```

**Common Issues:**

| Issue | Cause | Solution |
|-------|-------|----------|
| SSH connection refused | SSH not enabled | Generate RSA keys and enable SSH |
| Authentication failed | Wrong privilege level | Verify user has privilege 15 |
| Command timeout | Slow device response | Increase timeout in profile |
| Config not saved | `save_when` not set | Add `save_when: modified` |

---

### Cisco NX-OS

**Supported Models:** Nexus 3000, 5000, 7000, 9000 series

**Profile Configuration:**

```yaml
profiles:
  cisco-nxos:
    vendor: cisco
    os: nxos
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: nxapi
      port: 443
      ssl: true
    commands:
      show_config: show running-config
      show_version: show version
      save_config: copy running-config startup-config
      config_mode: configure terminal
    features:
      - nxapi
      - scp-server
```

**Device Preparation:**

```bash
! Enable NX-API for REST access
configure terminal
feature nxapi
nxapi http port 80
nxapi https port 443
nxapi sandbox

! Enable SSH
feature ssh
ssh key rsa 2048

! Create management user
username kscore password $STRONG_PASSWORD role network-admin

! Enable SCP
feature scp-server
```

**Example State Application:**

```yaml
nxos_vlan_config:
  module: nxos_config
  state: present
  target: "proxy:vendor=cisco and os=nxos"
  lines:
    - vlan 100
    - name Production
    - vlan 200
    - name Development

nxos_interface_config:
  module: nxos_config
  state: present
  target: "proxy:nexus-9k-01"
  lines:
    - description Server Port
    - switchport mode access
    - switchport access vlan 100
    - spanning-tree port type edge
    - no shutdown
  parents:
    - interface Ethernet1/1
```

---

### Arista EOS

**Supported Models:** All Arista switches (7000, 7100, 7200, 7300, 7500 series)

**Profile Configuration:**

```yaml
profiles:
  arista-eos:
    vendor: arista
    os: eos
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: eapi
      port: 443
      ssl: true
      transport: https
    commands:
      show_config: show running-config
      show_version: show version
      save_config: copy running-config startup-config
      config_mode: configure
      exit_config: end
    prompts:
      exec: ">"
      privileged: "#"
      config: "(config)#"
```

**Device Preparation:**

```bash
! Enable eAPI
configure
management api http-commands
   protocol https
   no shutdown

! Create management user
username kscore privilege 15 role network-admin secret $STRONG_PASSWORD

! Enable SSH
management ssh
   idle-timeout 15
   authentication mode password

! Configure management interface
interface Management1
   ip address 10.0.0.10/24
   no shutdown
```

**Example State Application:**

```yaml
arista_bgp_config:
  module: eos_config
  state: present
  target: "proxy:vendor=arista"
  lines:
    - neighbor 10.0.0.1 remote-as 65001
    - neighbor 10.0.0.1 description ISP-Peer
    - neighbor 10.0.0.1 maximum-routes 10000
    - network 192.168.0.0/16
  parents:
    - router bgp 65000

arista_evpn_config:
  module: eos_config
  state: present
  target: "proxy:spine-01"
  lines:
    - vlan 100
    - rd auto
    - route-target both 65000:100
    - redistribute learned
  parents:
    - router bgp 65000
    - vlan-aware-bundle TENANT-A
```

**eAPI Access Example:**

```yaml
# Using eAPI directly
arista_api_call:
  module: http_resource
  state: present
  target: "proxy:arista-switch-01"
  method: POST
  url: /command-api
  body:
    jsonrpc: "2.0"
    method: runCmds
    params:
      version: 1
      cmds:
        - show vlan
```

---

### Juniper JUNOS

**Supported Models:** MX, EX, QFX, SRX series

**Profile Configuration:**

```yaml
profiles:
  juniper-junos:
    vendor: juniper
    os: junos
    connection:
      protocol: ssh
      port: 22
      timeout: 60s
    api:
      enabled: true
      type: netconf
      port: 830
    commands:
      show_config: show configuration | display set
      show_version: show version
      commit: commit
      config_mode: configure
      exit_config: exit
    config_format: set  # or 'text', 'xml'
    prompts:
      exec: ">"
      config: "#"
```

**Device Preparation:**

```bash
# Enable SSH and NETCONF
set system services ssh
set system services netconf ssh

# Create management user
set system login user kscore class super-user authentication plain-text-password
# (enter password when prompted)

# Configure management interface
set interfaces fxp0 unit 0 family inet address 10.0.0.20/24

# Commit changes
commit
```

**Example State Application:**

```yaml
junos_firewall_policy:
  module: junos_config
  state: present
  target: "proxy:vendor=juniper and os=junos"
  lines:
    - set security zones security-zone trust interfaces ge-0/0/0
    - set security zones security-zone untrust interfaces ge-0/0/1
    - set security policies from-zone trust to-zone untrust policy allow-outbound match source-address any
    - set security policies from-zone trust to-zone untrust policy allow-outbound match destination-address any
    - set security policies from-zone trust to-zone untrust policy allow-outbound then permit

junos_routing_config:
  module: junos_config
  state: present
  target: "proxy:mx-router-01"
  config_format: text
  config: |
    routing-options {
        static {
            route 0.0.0.0/0 next-hop 10.0.0.1;
        }
        autonomous-system 65000;
    }
```

**NETCONF Access Example:**

```yaml
# Direct NETCONF operation
junos_netconf:
  module: netconf_config
  state: present
  target: "proxy:juniper-srx-01"
  config: |
    <configuration>
      <interfaces>
        <interface>
          <name>ge-0/0/0</name>
          <unit>
            <name>0</name>
            <family>
              <inet>
                <address>
                  <name>192.168.1.1/24</name>
                </address>
              </inet>
            </family>
          </unit>
        </interface>
      </interfaces>
    </configuration>
```

---

### HP/HPE ProCurve/Aruba

**Supported Models:** ProCurve, Aruba CX, Aruba OS-Switch

**Profile Configuration (ProCurve/Aruba OS-Switch):**

```yaml
profiles:
  hp-procurve:
    vendor: hp
    os: procurve
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    commands:
      show_config: show running-config
      show_version: show version
      save_config: write memory
      config_mode: configure terminal
      exit_config: exit
    prompts:
      exec: ">"
      privileged: "#"
      config: "(config)#"
```

**Profile Configuration (Aruba CX):**

```yaml
profiles:
  aruba-cx:
    vendor: aruba
    os: aoscx
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
      version: v10.09
    commands:
      show_config: show running-config
      save_config: copy running-config startup-config
```

**Device Preparation (ProCurve):**

```bash
! Enable SSH
configure terminal
crypto key generate ssh rsa
ip ssh

! Create management user
password manager user-name kscore
# Enter password when prompted

! Configure management VLAN
vlan 1
 ip address 10.0.0.30 255.255.255.0
 no shutdown
```

**Device Preparation (Aruba CX):**

```bash
! Enable REST API
configure terminal
https-server vrf mgmt
https-server rest access-mode read-write

! Create management user
user kscore group administrators password plaintext $STRONG_PASSWORD
```

**Example State Application:**

```yaml
hp_vlan_config:
  module: procurve_config
  state: present
  target: "proxy:vendor=hp and os=procurve"
  lines:
    - vlan 100
    - name "Production"
    - untagged 1-24
    - tagged 25-26

aruba_cx_interface:
  module: aoscx_config
  state: present
  target: "proxy:vendor=aruba and os=aoscx"
  lines:
    - interface 1/1/1
    - no shutdown
    - description "Server Connection"
    - no routing
    - vlan access 100
```

---

### Dell EMC Networking

**Supported Models:** PowerSwitch (S-Series), Dell Networking OS10, OS9

**Profile Configuration (OS10):**

```yaml
profiles:
  dell-os10:
    vendor: dell
    os: os10
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
    commands:
      show_config: show running-configuration
      show_version: show version
      save_config: write memory
      config_mode: configure terminal
    prompts:
      exec: ">"
      privileged: "#"
      config: "(config)#"
```

**Profile Configuration (OS9):**

```yaml
profiles:
  dell-os9:
    vendor: dell
    os: os9
    connection:
      protocol: ssh
      port: 22
    commands:
      show_config: show running-config
      save_config: copy running-config startup-config
      config_mode: configure terminal
```

**Device Preparation (OS10):**

```bash
! Enable SSH
configure terminal
ip ssh server enable

! Create management user
username kscore password $STRONG_PASSWORD role sysadmin

! Enable REST API
system
 rest port 443
 rest enable

! Configure management interface
interface mgmt1/1/1
 ip address 10.0.0.40/24
 no shutdown
```

**Example State Application:**

```yaml
dell_vlt_config:
  module: os10_config
  state: present
  target: "proxy:vendor=dell and os=os10"
  lines:
    - vlt-domain 1
    - peer-link port-channel 100
    - back-up destination 10.0.0.41
    - discovery-interface ethernet1/1/1-1/1/2

dell_interface_config:
  module: os10_config
  state: present
  target: "proxy:dell-s5248-01"
  lines:
    - interface ethernet1/1/1
    - no shutdown
    - description "Uplink"
    - switchport mode trunk
    - switchport trunk allowed vlan 100-200
```

---

### Fortinet FortiGate

**Supported Models:** All FortiGate models

**Profile Configuration:**

```yaml
profiles:
  fortinet-fortigate:
    vendor: fortinet
    os: fortios
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
      api_key: true
    commands:
      show_config: show full-configuration
      config_mode: config
      exit_config: end
    prompts:
      exec: "#"
      config: "edit:"
```

**Device Preparation:**

```bash
# Create API admin user
config system api-user
    edit "kscore"
        set accprofile "super_admin"
        set vdom "root"
        config trusthost
            edit 1
                set ipv4-trusthost 10.0.0.0 255.255.255.0
            next
        end
    next
end

# Generate API key
execute api-user generate-key kscore
# Save the generated key

# Enable SSH
config system global
    set admin-ssh-port 22
end

# Create admin user for SSH
config system admin
    edit "kscore"
        set accprofile "super_admin"
        set password $STRONG_PASSWORD
        set trusthost1 10.0.0.0 255.255.255.0
    next
end
```

**Example State Application:**

```yaml
fortigate_firewall_policy:
  module: fortios_config
  state: present
  target: "proxy:vendor=fortinet"
  config:
    firewall_policy:
      - policyid: 100
        name: "Allow-Outbound"
        srcintf:
          - name: "internal"
        dstintf:
          - name: "wan1"
        srcaddr:
          - name: "all"
        dstaddr:
          - name: "all"
        action: accept
        schedule: "always"
        service:
          - name: "ALL"
        nat: enable
        logtraffic: all

fortigate_interface_config:
  module: fortios_config
  state: present
  target: "proxy:fg-firewall-01"
  config:
    system_interface:
      - name: "port1"
        mode: static
        ip: "192.168.1.1 255.255.255.0"
        allowaccess: "ping https ssh"
        type: physical
```

**REST API Example:**

```yaml
fortigate_api_call:
  module: http_resource
  state: present
  target: "proxy:fg-firewall-01"
  method: POST
  url: /api/v2/cmdb/firewall/address
  headers:
    Authorization: "Bearer {{ .vars.fortigate_api_key }}"
  body:
    name: "WebServers"
    type: ipmask
    subnet: "10.0.100.0 255.255.255.0"
    comment: "Web Server Network"
```

---

### Check Point

**Supported Models:** All Check Point Gaia-based appliances

**Profile Configuration:**

```yaml
profiles:
  checkpoint-gaia:
    vendor: checkpoint
    os: gaia
    connection:
      protocol: ssh
      port: 22
      timeout: 60s
    api:
      enabled: true
      type: web-api
      port: 443
      ssl: true
    commands:
      show_config: show configuration
      expert_mode: expert
      clish_mode: clish
    prompts:
      clish: ">"
      expert: "#"
```

**Device Preparation:**

```bash
# In Clish mode
set user kscore shell /bin/bash
set user kscore password
# Enter password

# Grant API access
set api-settings accepted-api-calls-from "All IP Addresses That Can Be Used For GUI Clients"
api status  # Verify API is running

# In SmartConsole, create an API key for the user
```

**Example State Application:**

```yaml
checkpoint_host_object:
  module: checkpoint_config
  state: present
  target: "proxy:vendor=checkpoint"
  api_version: 1.8
  objects:
    - type: host
      name: web-server-1
      ip-address: 10.0.100.10
      color: blue
      comments: "Production Web Server"

checkpoint_network_object:
  module: checkpoint_config
  state: present
  target: "proxy:cp-mgmt-01"
  objects:
    - type: network
      name: production-network
      subnet: 10.0.100.0
      mask-length: 24
      color: green

checkpoint_access_rule:
  module: checkpoint_config
  state: present
  target: "proxy:cp-mgmt-01"
  rules:
    - type: access-rule
      layer: "Network"
      position: top
      name: "Allow-Web-Traffic"
      source:
        - "Any"
      destination:
        - "web-server-1"
      service:
        - "http"
        - "https"
      action: Accept
      track:
        type: Log
```

---

### Palo Alto Networks

**Supported Models:** All PAN-OS devices (PA-Series, VM-Series)

**Profile Configuration:**

```yaml
profiles:
  paloalto-panos:
    vendor: paloalto
    os: panos
    connection:
      protocol: ssh
      port: 22
      timeout: 60s
    api:
      enabled: true
      type: xml-api
      port: 443
      ssl: true
    commands:
      show_config: show config running
      commit: commit
      config_mode: configure
    prompts:
      exec: ">"
      config: "#"
```

**Device Preparation:**

```bash
# Create admin user
configure
set mgt-config users kscore permissions role-based superuser yes
set mgt-config users kscore password
# Enter password

# Generate API key (via API call)
curl -k -X GET "https://10.0.0.50/api/?type=keygen&user=kscore&password=PASSWORD"

# Enable API access
set deviceconfig system permitted-ip 10.0.0.0/24
commit
```

**Example State Application:**

```yaml
panos_address_object:
  module: panos_config
  state: present
  target: "proxy:vendor=paloalto"
  objects:
    address:
      - name: web-server
        ip-netmask: 10.0.100.10/32
        description: "Production Web Server"
        tag:
          - production

panos_security_rule:
  module: panos_config
  state: present
  target: "proxy:pa-firewall-01"
  rules:
    security:
      - name: "Allow-Web"
        from: ["trust"]
        to: ["untrust"]
        source: ["any"]
        destination: ["web-server"]
        application: ["web-browsing", "ssl"]
        service: ["application-default"]
        action: allow
        log-start: true
        log-end: true
        rule-type: universal

panos_nat_rule:
  module: panos_config
  state: present
  target: "proxy:pa-firewall-01"
  rules:
    nat:
      - name: "NAT-Web-Server"
        from: ["untrust"]
        to: ["untrust"]
        source: ["any"]
        destination: ["203.0.113.10"]
        service: "service-http"
        destination-translation:
          translated-address: web-server
```

---

### pfSense/OPNsense

**Supported Models:** All pfSense/OPNsense installations

**Profile Configuration (pfSense):**

```yaml
profiles:
  pfsense:
    vendor: netgate
    os: pfsense
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
      package: pfSense-pkg-API
    commands:
      shell: /bin/sh
```

**Profile Configuration (OPNsense):**

```yaml
profiles:
  opnsense:
    vendor: deciso
    os: opnsense
    connection:
      protocol: ssh
      port: 22
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
      auth: key+secret
```

**Device Preparation (pfSense):**

```bash
# Install API package via SSH
pkg install -y pfSense-pkg-API

# Or via GUI: System > Package Manager > Available Packages > API

# Create API user via GUI or API
# System > API > Users > Add
```

**Device Preparation (OPNsense):**

```bash
# Enable API access via GUI
# System > Access > Users > [user] > API keys > Add

# Note the API key and secret
```

**Example State Application (pfSense):**

```yaml
pfsense_firewall_rule:
  module: pfsense_config
  state: present
  target: "proxy:vendor=netgate"
  config:
    firewall/rule:
      - interface: wan
        type: pass
        ipprotocol: inet
        protocol: tcp
        src: any
        dst: (self)
        dstport: 443
        descr: "Allow HTTPS to firewall"

pfsense_alias:
  module: pfsense_config
  state: present
  target: "proxy:pfsense-01"
  config:
    firewall/alias:
      - name: RFC1918
        type: network
        address: "10.0.0.0/8 172.16.0.0/12 192.168.0.0/16"
        descr: "Private Networks"
```

**Example State Application (OPNsense):**

```yaml
opnsense_firewall_rule:
  module: opnsense_config
  state: present
  target: "proxy:vendor=deciso"
  config:
    firewall/filter/rule:
      - enabled: true
        action: pass
        quick: true
        interface: wan
        direction: in
        ipprotocol: inet
        protocol: TCP
        source_net: any
        destination_port: 443
        description: "Allow HTTPS"
```

---

### MikroTik RouterOS

**Supported Models:** All MikroTik devices running RouterOS

**Profile Configuration:**

```yaml
profiles:
  mikrotik-routeros:
    vendor: mikrotik
    os: routeros
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    api:
      enabled: true
      type: api
      port: 8728
      ssl_port: 8729
      ssl: true
    commands:
      show_config: /export
      print: print
    prompts:
      exec: ">"
    path_separator: "/"
```

**Device Preparation:**

```bash
# Enable API access
/ip service set api address=10.0.0.0/24
/ip service set api-ssl address=10.0.0.0/24 certificate=server-cert

# Create management user
/user add name=kscore password=$STRONG_PASSWORD group=full

# Enable SSH
/ip service set ssh port=22

# Generate SSH keys (optional)
/user ssh-keys import public-key-file=kscore.pub user=kscore
```

**Example State Application:**

```yaml
mikrotik_firewall_rules:
  module: routeros_config
  state: present
  target: "proxy:vendor=mikrotik"
  config:
    /ip/firewall/filter:
      - chain: input
        action: accept
        protocol: tcp
        dst-port: 22
        comment: "Allow SSH"
      - chain: input
        action: accept
        protocol: icmp
        comment: "Allow ICMP"
      - chain: input
        action: drop
        comment: "Drop all other input"

mikrotik_interface_config:
  module: routeros_config
  state: present
  target: "proxy:mikrotik-01"
  config:
    /interface/ethernet:
      - name: ether1
        comment: "WAN Interface"
    /ip/address:
      - address: 192.168.1.1/24
        interface: ether2
        comment: "LAN Interface"
```

---

### Ubiquiti EdgeOS/EdgeRouter

**Supported Models:** EdgeRouter series, EdgeSwitch (EdgeOS)

**Profile Configuration:**

```yaml
profiles:
  ubiquiti-edgeos:
    vendor: ubiquiti
    os: edgeos
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    commands:
      show_config: show configuration
      config_mode: configure
      commit: commit
      save: save
      exit_config: exit
    prompts:
      exec: "$"
      config: "#"
    config_format: vyatta
```

**Device Preparation:**

```bash
# Enable SSH (usually enabled by default)
configure
set service ssh port 22

# Create management user
set system login user kscore authentication plaintext-password $STRONG_PASSWORD
set system login user kscore level admin

commit
save
```

**Example State Application:**

```yaml
edgeos_firewall_config:
  module: vyos_config
  state: present
  target: "proxy:vendor=ubiquiti and os=edgeos"
  lines:
    - set firewall name WAN_IN default-action drop
    - set firewall name WAN_IN rule 10 action accept
    - set firewall name WAN_IN rule 10 state established enable
    - set firewall name WAN_IN rule 10 state related enable
    - set firewall name WAN_IN rule 20 action accept
    - set firewall name WAN_IN rule 20 protocol icmp

edgeos_interface_config:
  module: vyos_config
  state: present
  target: "proxy:edge-router-01"
  lines:
    - set interfaces ethernet eth0 address 192.168.1.1/24
    - set interfaces ethernet eth0 description "LAN"
    - set interfaces ethernet eth1 address dhcp
    - set interfaces ethernet eth1 description "WAN"
    - set interfaces ethernet eth1 firewall in name WAN_IN
```

---

### VMware ESXi/vSphere

**Supported Models:** ESXi hosts, vCenter Server

**Profile Configuration (ESXi):**

```yaml
profiles:
  vmware-esxi:
    vendor: vmware
    os: esxi
    connection:
      protocol: ssh
      port: 22
      timeout: 60s
    api:
      enabled: true
      type: vim
      port: 443
      ssl: true
```

**Profile Configuration (vCenter):**

```yaml
profiles:
  vmware-vcenter:
    vendor: vmware
    os: vcenter
    api:
      enabled: true
      type: rest
      port: 443
      ssl: true
      base_path: /api
```

**Device Preparation (ESXi):**

```bash
# Enable SSH
vim-cmd hostsvc/enable_ssh
vim-cmd hostsvc/start_ssh

# Create local user (via vSphere Client or API)
# Host > Manage > Security & Users > Users > Add User
```

**Example State Application:**

```yaml
esxi_network_config:
  module: esxi_config
  state: present
  target: "proxy:vendor=vmware and os=esxi"
  config:
    vswitch:
      - name: vSwitch0
        uplinks:
          - vmnic0
          - vmnic1
        mtu: 9000
    portgroup:
      - name: Production
        vswitch: vSwitch0
        vlan: 100

vcenter_cluster_config:
  module: vcenter_config
  state: present
  target: "proxy:vcenter-01"
  config:
    cluster:
      - name: Production-Cluster
        drs:
          enabled: true
          automation_level: fullyAutomated
        ha:
          enabled: true
          admission_control: true
```

---

### Linux Servers (via SSH)

**Supported:** Any Linux distribution accessible via SSH

**Profile Configuration:**

```yaml
profiles:
  linux-ssh:
    vendor: generic
    os: linux
    connection:
      protocol: ssh
      port: 22
      timeout: 30s
    commands:
      shell: /bin/bash
      sudo: sudo
    become:
      method: sudo
      password_required: false  # If NOPASSWD is configured
```

**Example State Application:**

```yaml
linux_packages:
  module: ssh_cmd
  state: run
  target: "proxy:os=linux"
  command: apt-get update && apt-get install -y nginx
  become: true

linux_file_config:
  module: ssh_file
  state: present
  target: "proxy:linux-server-01"
  path: /etc/nginx/sites-available/default
  content: |
    server {
        listen 80;
        server_name _;
        root /var/www/html;
    }
  owner: root
  group: root
  mode: "0644"
  become: true

linux_service:
  module: ssh_service
  state: running
  target: "proxy:linux-server-01"
  name: nginx
  enabled: true
  become: true
```

---

### Windows Servers (via WinRM)

**Supported:** Windows Server 2012+, Windows 10/11

**Profile Configuration:**

```yaml
profiles:
  windows-winrm:
    vendor: microsoft
    os: windows
    connection:
      protocol: winrm
      port: 5986
      transport: ntlm  # or kerberos, certificate
      ssl: true
    commands:
      shell: powershell
```

**Device Preparation:**

```powershell
# Enable WinRM with HTTPS
winrm quickconfig -transport:https

# Or manually configure
Enable-PSRemoting -Force
New-NetFirewallRule -Name "WinRM HTTPS" -DisplayName "WinRM HTTPS" -Enabled True -Direction Inbound -Protocol TCP -LocalPort 5986

# Create self-signed certificate (for testing)
$cert = New-SelfSignedCertificate -DnsName $env:COMPUTERNAME -CertStoreLocation Cert:\LocalMachine\My
winrm create winrm/config/Listener?Address=*+Transport=HTTPS "@{Hostname=`"$($env:COMPUTERNAME)`";CertificateThumbprint=`"$($cert.Thumbprint)`"}"

# Create service account
$password = ConvertTo-SecureString "StrongPassword" -AsPlainText -Force
New-LocalUser -Name "kscore" -Password $password
Add-LocalGroupMember -Group "Administrators" -Member "kscore"
```

**Example State Application:**

```yaml
windows_feature:
  module: winrm_package
  state: present
  target: "proxy:os=windows"
  name: Web-Server
  include_management_tools: true

windows_service:
  module: winrm_service
  state: running
  target: "proxy:windows-server-01"
  name: W3SVC
  start_mode: Automatic

windows_registry:
  module: winrm_registry
  state: present
  target: "proxy:windows-server-01"
  path: HKLM:\SOFTWARE\Company
  name: Setting
  data: Value
  type: String

windows_file:
  module: winrm_file
  state: present
  target: "proxy:windows-server-01"
  path: C:\inetpub\wwwroot\web.config
  content: |
    <?xml version="1.0" encoding="UTF-8"?>
    <configuration>
      <system.webServer>
        <defaultDocument>
          <files>
            <add value="index.html" />
          </files>
        </defaultDocument>
      </system.webServer>
    </configuration>
```

## Vendor API Compatibility Matrix

This matrix shows the supported protocols, API versions, and features for each vendor platform.

### Network Device Vendors

| Vendor | Platform | SSH | NETCONF | REST API | SNMP | Min Version | Tested Versions |
|--------|----------|-----|---------|----------|------|-------------|-----------------|
| Cisco | IOS | Yes | No | No | v2c/v3 | 12.4 | 12.4, 15.x, 16.x, 17.x |
| Cisco | IOS-XE | Yes | Yes | RESTCONF | v2c/v3 | 16.3 | 16.3+, 17.x |
| Cisco | NX-OS | Yes | Yes | NX-API | v2c/v3 | 7.0 | 7.x, 9.x, 10.x |
| Cisco | ASA | Yes | No | REST API | v2c/v3 | 9.1 | 9.x |
| Arista | EOS | Yes | No | eAPI | v2c/v3 | 4.20 | 4.20+, 4.25+, 4.28+ |
| Juniper | JUNOS | Yes | Yes | REST API | v2c/v3 | 14.1 | 14.x, 18.x, 20.x, 21.x |
| HP/Aruba | ProCurve | Yes | No | No | v2c/v3 | K.15 | K.15+, WC.16+ |
| HP/Aruba | AOS-CX | Yes | No | REST API | v2c/v3 | 10.04 | 10.04+, 10.09+ |
| Dell | OS10 | Yes | No | REST API | v2c/v3 | 10.4 | 10.4+, 10.5+ |
| Dell | OS9 | Yes | No | No | v2c/v3 | 9.10 | 9.10+, 9.14+ |

### Firewall Vendors

| Vendor | Platform | SSH | REST API | API Version | SNMP | Min Version | Tested Versions |
|--------|----------|-----|----------|-------------|------|-------------|-----------------|
| Fortinet | FortiOS | Yes | REST | v2 | v2c/v3 | 6.0 | 6.0, 6.2, 6.4, 7.0, 7.2 |
| Palo Alto | PAN-OS | Yes | XML API | v9.1+ | v2c/v3 | 8.1 | 8.1, 9.x, 10.x, 11.x |
| Check Point | Gaia | Yes | Web API | v1.8+ | v2c/v3 | R80 | R80, R81, R81.10 |
| pfSense | CE/Plus | Yes | REST (pkg) | v1 | v2c | 2.4 | 2.4, 2.5, 2.6, 2.7 |
| OPNsense | OPNsense | Yes | REST | v1 | v2c | 21.1 | 21.x, 22.x, 23.x |
| Sophos | XG/XGS | Yes | REST | v1 | v2c/v3 | SFOS 17 | 17.x, 18.x, 19.x |

### Routing/Switching Vendors

| Vendor | Platform | SSH | API Type | SNMP | Min Version | Tested Versions |
|--------|----------|-----|----------|------|-------------|-----------------|
| MikroTik | RouterOS | Yes | RouterOS API | v2c/v3 | 6.40 | 6.x, 7.x |
| Ubiquiti | EdgeOS | Yes | No | v2c | 1.9 | 1.9, 1.10, 2.x |
| VyOS | VyOS | Yes | HTTP API | v2c | 1.2 | 1.2, 1.3, 1.4 |
| Cumulus | Linux | Yes | NCLU | v2c/v3 | 3.7 | 3.7, 4.x, 5.x |
| SONiC | SONiC | Yes | REST/gNMI | v2c/v3 | 202012 | 202012+, 202205+ |

### Virtualization Platforms

| Vendor | Platform | SSH | API Type | Min Version | Tested Versions |
|--------|----------|-----|----------|-------------|-----------------|
| VMware | ESXi | Yes | VIM/SOAP | 6.5 | 6.5, 6.7, 7.0, 8.0 |
| VMware | vCenter | No | REST API | 6.5 | 6.5, 6.7, 7.0, 8.0 |
| Proxmox | VE | Yes | REST API | 6.0 | 6.x, 7.x, 8.x |
| Microsoft | Hyper-V | WinRM | PowerShell | 2016 | 2016, 2019, 2022 |

### Cloud/SDN Platforms

| Vendor | Platform | API Type | Auth Method | Min Version | Tested Versions |
|--------|----------|----------|-------------|-------------|-----------------|
| Cisco | ACI | REST | Token | 4.0 | 4.x, 5.x |
| VMware | NSX-T | REST | Token | 3.0 | 3.x, 4.x |
| Arista | CloudVision | REST | Token/OAuth | 2021.1 | 2021.x, 2022.x |

### Operating Systems

| Platform | Protocol | Become Method | Min Version | Tested Versions |
|----------|----------|---------------|-------------|-----------------|
| Linux (Debian) | SSH | sudo | 9 | 9, 10, 11, 12 |
| Linux (Ubuntu) | SSH | sudo | 18.04 | 18.04, 20.04, 22.04, 24.04 |
| Linux (RHEL/Rocky) | SSH | sudo | 7 | 7, 8, 9 |
| Linux (SUSE) | SSH | sudo | 12 SP5 | 12 SP5, 15 SP3+ |
| FreeBSD | SSH | sudo/doas | 12 | 12, 13, 14 |
| Windows Server | WinRM | RunAs | 2012 R2 | 2012 R2, 2016, 2019, 2022 |
| Windows | WinRM | RunAs | 10 | 10, 11 |

### Protocol Feature Support

| Feature | SSH | NETCONF | REST | SNMP v2c | SNMP v3 | WinRM |
|---------|-----|---------|------|----------|---------|-------|
| Config Read | Yes | Yes | Yes | Limited | Limited | Yes |
| Config Write | Yes | Yes | Yes | No | No | Yes |
| Command Exec | Yes | No | Varies | No | No | Yes |
| File Transfer | SCP/SFTP | No | Varies | No | No | Yes |
| Bulk Operations | Limited | Yes | Yes | Yes | Yes | Yes |
| Transaction | No | Yes | Varies | No | No | No |
| Streaming | No | Subscription | Webhook | Traps | Traps | Events |

### API Authentication Methods

| Vendor | Basic Auth | Token | API Key | OAuth 2.0 | Certificate | SAML |
|--------|------------|-------|---------|-----------|-------------|------|
| Cisco NX-API | Yes | No | No | No | Yes | No |
| Cisco RESTCONF | Yes | No | No | No | Yes | No |
| Arista eAPI | Yes | Session | No | No | Yes | No |
| Juniper REST | Yes | No | No | No | Yes | No |
| Fortinet | Yes | No | Yes | No | Yes | No |
| Palo Alto | No | No | Yes | No | Yes | No |
| Check Point | Yes | Token | Yes | No | Yes | No |
| pfSense | Yes | No | Yes | No | No | No |
| OPNsense | No | No | Key+Secret | No | No | No |
| VMware vCenter | Yes | Session | No | Yes | Yes | Yes |

### Rate Limits and Recommendations

| Vendor | Rate Limit | Concurrent Connections | Recommended Polling Interval |
|--------|------------|----------------------|------------------------------|
| Cisco IOS | None | 5 SSH sessions | 5 minutes |
| Cisco NX-OS | 100 req/min | 10 sessions | 1 minute |
| Arista EOS | 1000 req/min | 32 sessions | 30 seconds |
| Juniper JUNOS | None | 10 NETCONF sessions | 1 minute |
| Fortinet | 500 req/10s | 32 sessions | 1 minute |
| Palo Alto | 4000 req/min | Unlimited | 30 seconds |
| Check Point | 500 req/min | 30 sessions | 1 minute |
| pfSense | None | Limited by PHP | 5 minutes |
| VMware vCenter | 100 req/min | 10 sessions | 5 minutes |

### Known Limitations

| Vendor/Platform | Limitation | Workaround |
|-----------------|------------|------------|
| Cisco IOS | No API, SSH-only | Use SSH with expect-style parsing |
| HP ProCurve | Limited SNMP write | Use SSH for configuration |
| MikroTik | API has different syntax than CLI | Use dedicated RouterOS module |
| pfSense | API requires additional package | Install pfSense-pkg-API |
| Fortinet | Some features not in API | Use CLI via SSH for unsupported features |
| Check Point | Changes require "publish" | Include publish step in automation |
| Palo Alto | Changes require "commit" | Include commit step in automation |
| VyOS | HTTP API is optional | Enable API or use SSH |
| Windows | WinRM requires setup | Use setup script or GPO |

## See Also

- [Proxy Agents Concepts](/docs/concepts/proxy-agents/)
- [Configuration Reference - Proxy Agent](/docs/reference/configuration/#proxy-agent-configuration)
- [State Modules Reference](/docs/reference/modules/)
- [CLI Reference](/docs/reference/cli/)
