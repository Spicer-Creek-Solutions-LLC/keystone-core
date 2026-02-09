---
title: "Proxy State Modules Reference"
weight: 26
description: >
  Complete reference for proxy state modules including SSH, SNMP, REST, WinRM, NETCONF, and vendor-specific modules
---

## Overview

Proxy state modules provide idempotent configuration management for network devices and servers managed through proxy agents. Each module implements the `ProxyModule` interface with `Execute()` and `Check()` methods.

All modules support:
- **Dry-run mode**: Check what changes would be made without applying them
- **Idempotent execution**: Only make changes when the current state differs from desired state
- **Result reporting**: Return whether changes were made and detailed information

## SSH Modules

### ssh_file

Manages files on remote hosts via SSH.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `content` | string | no | File content |
| `source` | string | no | Source file to copy |
| `mode` | string | no | File permissions |
| `owner` | string | no | File owner |
| `state` | string | no | `present` (default), `absent`, `directory` |

### ssh_cmd

Executes commands on remote hosts via SSH.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cmd` | string | yes | Command to execute |
| `creates` | string | no | Skip if this path exists |
| `unless` | string | no | Skip if this command succeeds |

### ssh_service

Manages system services via SSH.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Service name |
| `state` | string | no | `running`, `stopped`, `restarted`, `reloaded` |
| `enabled` | bool | no | Enable/disable on boot |

### ssh_package

Manages packages via SSH (auto-detects apt, dnf, yum, pacman, apk).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Package name |
| `state` | string | no | `installed` (default), `removed`, `absent` |

### ssh_user

Manages user accounts via SSH.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Username |
| `shell` | string | no | Login shell |
| `home` | string | no | Home directory |
| `groups` | []string | no | Group memberships |
| `state` | string | no | `present` (default), `absent` |

### ssh_group

Manages groups via SSH.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Group name |
| `gid` | int | no | Group ID |
| `state` | string | no | `present` (default), `absent` |

## SNMP Modules

### snmp_value

Gets or sets SNMP OID values.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `oid` | string | yes | SNMP OID |
| `value` | string | no | Value to set |
| `type` | string | no | Value type (`s` for string, default) |

### snmp_table

Retrieves SNMP table data.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `oid` | string | yes | Table root OID |

## REST/HTTP Modules

### http_config

Manages configuration via REST API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | API endpoint path |
| `config` | map | no | Configuration to apply |
| `method` | string | no | HTTP method (`PUT` default) |

### http_resource

Manages REST API resources.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Resource path |
| `data` | map | no | Resource data |
| `state` | string | no | `present` (default), `absent` |

## Network Device Configuration Modules

These modules manage device configuration using vendor-specific CLI commands. All support `lines`, `parents`, `save`, and `backup` parameters.

| Module | Vendor | Config Mode | Save Command |
|--------|--------|-------------|--------------|
| `ios_config` | Cisco IOS | `configure terminal` | `write memory` |
| `nxos_config` | Cisco NX-OS | `configure terminal` | `copy running-config startup-config` |
| `junos_config` | Juniper JUNOS | `configure` | `commit` |
| `eos_config` | Arista EOS | `configure session` | `write memory` |
| `vyos_config` | VyOS | `configure` | `save` |
| `pfsense_config` | pfSense | REST API | Automatic |
| `opnsense_config` | OPNsense | REST API | Automatic |
| `hp_procurve_config` | HP ProCurve | `configure terminal` | `write memory` |
| `hp_arubaos_config` | HP ArubaOS | `configure terminal` | `write memory` |
| `hp_aoscx_config` | HP AOS-CX | `configure terminal` | `write memory` |
| `dell_os10_config` | Dell OS10 | `configure terminal` | `write memory` |
| `dell_os9_config` | Dell OS9 | `configure terminal` | `write memory` |
| `dell_powerswitch_config` | Dell PowerSwitch | `configure terminal` | `write memory` |
| `fortios_config` | Fortinet FortiOS | CLI | Automatic |
| `panos_config` | Palo Alto PAN-OS | CLI | `commit` |
| `bigip_config` | F5 BIG-IP | tmsh | `save sys config` |
| `checkpoint_gaia_config` | Check Point Gaia | `clish` | `save config` |
| `mikrotik_routeros_config` | MikroTik | CLI | Automatic |
| `ubiquiti_edgeos_config` | Ubiquiti EdgeOS | `configure` | `save` |
| `extreme_exos_config` | Extreme EXOS | CLI | `save configuration` |
| `nokia_sros_config` | Nokia SR OS | `configure` | `admin save` |
| `huawei_vrp_config` | Huawei VRP | `system-view` | `save` |
| `mellanox_onyx_config` | Mellanox Onyx | `configure terminal` | `configuration write` |
| `alliedtelesis_awplus_config` | Allied Telesis | `configure terminal` | `write` |
| `ciena_saos_config` | Ciena SAOS | CLI | `configuration save` |

## NETCONF State Modules

These modules manage device configuration using NETCONF (RFC 6241) with OpenConfig YANG models. They use the candidate datastore with a lock/edit/validate/commit/unlock workflow.

### netconf_interface

Manages network interfaces via NETCONF using the OpenConfig `openconfig-interfaces` model.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Interface name (e.g., `GigabitEthernet0/0/0`) |
| `description` | string | no | Interface description |
| `enabled` | bool | no | Administrative state |
| `mtu` | int | no | Maximum transmission unit |
| `ip_address` | string | no | IPv4 address |
| `ip_prefix_length` | int | no | IPv4 prefix length |
| `state` | string | no | `present` (default), `absent`, `up`, `down` |

```yaml
netconf_interface:
  name: GigabitEthernet0/0/0
  description: "Uplink to core"
  enabled: true
  mtu: 9000
  ip_address: 10.0.0.1
  ip_prefix_length: 30
```

### netconf_vlan

Manages VLANs via NETCONF using the OpenConfig `openconfig-vlan` model.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `vlan_id` | int | yes | VLAN ID (1-4094) |
| `name` | string | no | VLAN name |
| `vlan_state` | string | no | `active` or `suspend` |
| `state` | string | no | `present` (default), `absent` |

```yaml
netconf_vlan:
  vlan_id: 100
  name: production
  vlan_state: active
```

### netconf_routing

Manages static routes via NETCONF using the OpenConfig `openconfig-network-instance` model.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prefix` | string | yes | Route prefix (e.g., `10.0.0.0/8`) |
| `next_hop` | string | yes | Next hop address |
| `metric` | int | no | Route metric |
| `vrf` | string | no | VRF name (`default` if omitted) |
| `state` | string | no | `present` (default), `absent` |

```yaml
netconf_routing:
  prefix: 10.0.0.0/8
  next_hop: 192.168.1.1
  metric: 100
  vrf: mgmt
```

### netconf_acl

Manages access control lists via NETCONF using the OpenConfig `openconfig-acl` model.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | ACL name |
| `type` | string | no | `ipv4` (default), `ipv6` |
| `entries` | list | no | ACL entries (see below) |
| `state` | string | no | `present` (default), `absent` |

**ACL entry fields**: `sequence` (int), `action` (`permit`/`deny`), `protocol` (string), `source` (CIDR), `destination` (CIDR)

```yaml
netconf_acl:
  name: BLOCK-RFC1918
  type: ipv4
  entries:
    - sequence: 10
      action: deny
      source: 10.0.0.0/8
      destination: 0.0.0.0/0
    - sequence: 20
      action: deny
      source: 172.16.0.0/12
      destination: 0.0.0.0/0
    - sequence: 100
      action: permit
      source: 0.0.0.0/0
      destination: 0.0.0.0/0
```

## Vendor State Modules

These modules provide higher-level abstractions for vendor-specific features like firewall policies, load balancer pools, and security rules.

### fortios_policy

Manages FortiGate firewall policies via the FortiOS REST API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `policy_id` | int | yes | Policy ID |
| `name` | string | no | Policy name |
| `srcintf` | []string | no | Source interfaces |
| `dstintf` | []string | no | Destination interfaces |
| `srcaddr` | []string | no | Source addresses |
| `dstaddr` | []string | no | Destination addresses |
| `action` | string | no | `accept` or `deny` |
| `service` | []string | no | Services |
| `schedule` | string | no | Schedule name |
| `nat` | bool | no | Enable NAT |
| `logtraffic` | string | no | `all`, `utm`, `disable` |
| `comment` | string | no | Policy comment |
| `state` | string | no | `present` (default), `absent` |

```yaml
fortios_policy:
  policy_id: 1
  name: allow-web
  srcintf: [port1]
  dstintf: [port2]
  srcaddr: [all]
  dstaddr: [WebServers]
  action: accept
  service: [HTTP, HTTPS]
  schedule: always
  nat: true
  logtraffic: all
```

### panos_rule

Manages Palo Alto PAN-OS security and NAT rules via the PAN-OS XML API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Rule name |
| `rule_type` | string | no | `security` (default), `nat` |
| `source_zone` | []string | no | Source zones |
| `dest_zone` | []string | no | Destination zones |
| `source` | []string | no | Source addresses |
| `destination` | []string | no | Destination addresses |
| `application` | []string | no | Applications |
| `service` | []string | no | Services |
| `action` | string | no | `allow`, `deny`, `drop` |
| `log_setting` | string | no | Log forwarding profile |
| `commit` | bool | no | Auto-commit changes |
| `state` | string | no | `present` (default), `absent` |

```yaml
panos_rule:
  name: allow-web-traffic
  source_zone: [trust]
  dest_zone: [untrust]
  source: [any]
  destination: [any]
  application: [web-browsing, ssl]
  service: [application-default]
  action: allow
  commit: true
```

### bigip_pool

Manages F5 BIG-IP LTM pools via iControl REST.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Pool name (e.g., `/Common/web-pool`) |
| `lb_method` | string | no | Load balancing method |
| `monitors` | []string | no | Health monitors |
| `members` | list | no | Pool members (`address`, `port`) |
| `description` | string | no | Description |
| `state` | string | no | `present` (default), `absent` |

```yaml
bigip_pool:
  name: /Common/web-pool
  lb_method: least-connections-member
  monitors: [/Common/http]
  members:
    - address: 10.0.0.1
      port: 80
    - address: 10.0.0.2
      port: 80
  description: Web application pool
```

### bigip_virtual

Manages F5 BIG-IP LTM virtual servers via iControl REST.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Virtual server name |
| `destination` | string | no | Destination IP |
| `port` | int | no | Destination port |
| `pool` | string | no | Default pool |
| `profiles` | []string | no | Applied profiles |
| `snat` | string | no | SNAT setting (`automap`, `none`) |
| `description` | string | no | Description |
| `state` | string | no | `present`, `absent`, `enabled`, `disabled` |

```yaml
bigip_virtual:
  name: /Common/web-vs
  destination: 10.0.0.100
  port: 443
  pool: /Common/web-pool
  profiles: [http, clientssl]
  snat: automap
  description: HTTPS virtual server
```

### checkpoint_rule

Manages Check Point access rules via the Web Services API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Rule name |
| `layer` | string | no | Policy layer (`Network` default) |
| `position` | string | no | Rule position (`top`, `bottom`, number) |
| `source` | []string | no | Source objects |
| `destination` | []string | no | Destination objects |
| `service` | []string | no | Service objects |
| `action` | string | no | `Accept`, `Drop`, `Reject` |
| `track` | string | no | `Log`, `None`, `Alert` |
| `install_on` | []string | no | Installation targets |
| `comment` | string | no | Rule comment |
| `state` | string | no | `present` (default), `absent` |

```yaml
checkpoint_rule:
  name: allow-web-traffic
  layer: Network
  position: top
  source: [WebServers]
  destination: [Internet]
  service: [HTTP, HTTPS]
  action: Accept
  track: Log
  comment: "Allow web server outbound traffic"
```

## WinRM Modules

### winrm_file

Manages files on Windows hosts via WinRM.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `content` | string | no | File content |
| `state` | string | no | `present` (default), `absent` |

### winrm_service

Manages Windows services via WinRM.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Service name |
| `state` | string | no | `running`, `stopped` |
| `startup_type` | string | no | `automatic`, `manual`, `disabled` |

### winrm_registry

Manages Windows registry keys via WinRM.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Registry path |
| `name` | string | no | Value name |
| `data` | string | no | Value data |
| `type` | string | no | Value type |
| `state` | string | no | `present` (default), `absent` |

### winrm_package

Manages packages on Windows via WinRM (MSI, Chocolatey).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Package name or MSI path |
| `provider` | string | no | `msi` or `chocolatey` |
| `state` | string | no | `installed` (default), `removed` |
