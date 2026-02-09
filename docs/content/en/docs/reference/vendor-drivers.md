---
title: "Vendor Drivers Reference"
weight: 25
description: >
  Configuration and usage reference for all supported network device vendor drivers
---

## Overview

Vendor drivers provide device-specific adapters that implement the `VendorAdapter` interface, extending the generic protocol adapters (SSH, REST) with vendor-specific CLI commands, output parsing, and configuration management. Each driver auto-registers with the `DefaultRegistry` at startup.

**Supported vendors** (25 total):

| Vendor | Driver | VendorType | Protocol | State Module |
|--------|--------|------------|----------|--------------|
| Cisco IOS | `cisco/ios` | `cisco_ios` | SSH | `ios_config` |
| Cisco NX-OS | `cisco/nxos` | `cisco_nxos` | SSH | `nxos_config` |
| Juniper JUNOS | `juniper/junos` | `juniper_junos` | SSH | `junos_config` |
| Arista EOS | `arista/eos` | `arista_eos` | SSH | `eos_config` |
| VyOS | `vyos/vyos` | `vyos` | SSH | `vyos_config` |
| pfSense | `pfsense/pfsense` | `pfsense` | REST | `pfsense_config` |
| OPNsense | `opnsense/opnsense` | `opnsense` | REST | `opnsense_config` |
| HP ProCurve | `hp/procurve` | `hp_procurve` | SSH | `hp_procurve_config` |
| HP ArubaOS | `hp/arubaos` | `hp_arubaos` | SSH | `hp_arubaos_config` |
| Aruba AOS-CX | `hp/aoscx` | `hp_aoscx` | SSH | `hp_aoscx_config` |
| Dell OS10 | `dell/os10` | `dell_os10` | SSH | `dell_os10_config` |
| Dell OS9 / FTOS | `dell/os9` | `dell_os9` | SSH | `dell_os9_config` |
| Dell PowerSwitch | `dell/powerswitch` | `dell_powerswitch` | SSH | `dell_powerswitch_config` |
| Fortinet FortiOS | `fortinet/fortios` | `fortinet_fortios` | SSH | `fortios_config` |
| Palo Alto PAN-OS | `paloalto/panos` | `paloalto_panos` | SSH | `panos_config` |
| F5 BIG-IP | `f5/bigip` | `f5_bigip` | SSH (tmsh) | `bigip_config` |
| Check Point Gaia | `checkpoint/gaia` | `checkpoint_gaia` | SSH (clish) | `checkpoint_gaia_config` |
| MikroTik RouterOS | `mikrotik/routeros` | `mikrotik_routeros` | SSH | `mikrotik_routeros_config` |
| Ubiquiti EdgeOS | `ubiquiti/edgeos` | `ubiquiti_edgeos` | SSH | `ubiquiti_edgeos_config` |
| Extreme EXOS | `extreme/exos` | `extreme_exos` | SSH | `extreme_exos_config` |
| Nokia SR OS | `nokia/sros` | `nokia_sros` | SSH | `nokia_sros_config` |
| Huawei VRP | `huawei/vrp` | `huawei_vrp` | SSH | `huawei_vrp_config` |
| Mellanox/NVIDIA Onyx | `mellanox/onyx` | `mellanox_onyx` | SSH | `mellanox_onyx_config` |
| Allied Telesis AWPlus | `alliedtelesis/awplus` | `alliedtelesis_awplus` | SSH | `alliedtelesis_awplus_config` |
| Ciena SAOS | `ciena/saos` | `ciena_saos` | SSH | `ciena_saos_config` |

## VendorAdapter Interface

All vendor drivers implement:

```go
type VendorAdapter interface {
    protocols.ProtocolAdapter

    Vendor() VendorType
    GetConfig(ctx context.Context, section string) (string, error)
    SetConfig(ctx context.Context, commands []string) error
    GetFacts(ctx context.Context) (*DeviceFacts, error)
    SaveConfig(ctx context.Context) error
}
```

## HP/Aruba Drivers

### HP ProCurve

SSH-based adapter for HP ProCurve switches.

**CLI pattern**: IOS-like with `enable` / `configure terminal` / `exit`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Show startup | `show config` |
| Enter config | `configure terminal` |
| Save config | `write memory` |
| Disable paging | `no page` |
| Facts | `show system-information`, `show version`, `show interfaces brief` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: procurve-switch-01
      vendor: hp_procurve
      protocol: ssh
      address: 10.0.1.10
      credential_ref: switch-creds
```

**State module** (`hp_procurve_config`):

```yaml
states:
  - name: configure-vlan
    module: hp_procurve_config
    parameters:
      lines:
        - "vlan 100"
        - "name Management"
      save: true
```

### HP ArubaOS

SSH-based adapter for ArubaOS wireless controllers.

**CLI pattern**: IOS-like with `enable` / `configure terminal` / `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure terminal` |
| Save config | `write memory` |
| Disable paging | `no paging` |
| Facts | `show version`, `show switches` |

Additional method: `GetAPDatabase()` retrieves the access point database.

**State module** (`hp_arubaos_config`): Same parameters as `hp_procurve_config`.

### Aruba AOS-CX

SSH-based adapter for modern Aruba AOS-CX switches. Role-based access (no traditional enable mode).

**CLI pattern**: IOS-like with `configure terminal` / `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure terminal` |
| Save config | `copy running-config startup-config` |
| Disable paging | `no page` |
| Facts | `show version`, `show system`, `show interface brief` |

**State module** (`hp_aoscx_config`): Same parameters as `hp_procurve_config`, but saves with `copy running-config startup-config`.

## Dell Drivers

### Dell OS10

SSH-based adapter for Dell Networking OS10 (modern Linux-based platform).

**CLI pattern**: IOS-like with `enable` / `configure terminal` / `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-configuration` |
| Enter config | `configure terminal` |
| Save config | `write memory` |
| Disable paging | `terminal length 0` |
| Facts | `show version`, `show system`, `show interface status` |

**State module** (`dell_os10_config`):

```yaml
states:
  - name: configure-interface
    module: dell_os10_config
    parameters:
      lines:
        - "interface ethernet1/1/1"
        - "no shutdown"
      save: true
```

### Dell OS9 / FTOS

SSH-based adapter for Dell OS9 (legacy Force10 FTOS).

**CLI pattern**: `enable` / `configure` / `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure` |
| Save config | `write` |
| Disable paging | `terminal length 0` |
| Facts | `show version`, `show system brief`, `show interfaces status` |

**State module** (`dell_os9_config`): Uses `configure` to enter config mode and `write` to save.

### Dell PowerSwitch

SSH-based adapter for Dell PowerSwitch N-series.

**CLI pattern**: `enable` / `configure` / `exit`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure` |
| Save config | `copy running-config startup-config` |
| Disable paging | `terminal length 0` |
| Facts | `show version`, `show interfaces status` |

**State module** (`dell_powerswitch_config`): Uses `configure` and saves with `copy running-config startup-config`.

## Fortinet FortiOS

SSH-based adapter for FortiGate firewalls.

**CLI pattern**: FortiOS uses a unique hierarchical pattern with `config` / `edit` / `set` / `next` / `end`. There is no traditional `enable` or `configure terminal` mode. Configuration is auto-saved on `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show full-configuration` |
| Show section | `show system interface` |
| Firewall rules | `show firewall policy` |
| Disable paging | `config system console` / `set output standard` / `end` |
| Facts | `get system status`, `get system performance status` |

**VDOM support**: Configure `vdom` in the adapter config to operate within a specific virtual domain.

**Configuration**:

```yaml
proxy:
  devices:
    - id: fortigate-01
      vendor: fortinet_fortios
      protocol: ssh
      address: 10.0.1.1
      credential_ref: forti-creds
```

**State module** (`fortios_config`):

```yaml
states:
  - name: configure-interface
    module: fortios_config
    parameters:
      section: "system interface"
      name: "port1"
      settings:
        ip: "10.0.0.1/24"
        allowaccess: "ping https ssh"
      backup: true
```

**SaveConfig**: No-op (FortiOS auto-saves on `end`).

## Palo Alto PAN-OS

SSH-based adapter for Palo Alto Networks firewalls. Uses a transactional commit model with candidate configuration.

**CLI pattern**: `configure` enters config mode. Changes are applied to the candidate config with `set` commands and activated via `commit`.

| Operation | Command |
|-----------|---------|
| Enter config | `configure` |
| Set config | `set <path> <value>` |
| Show candidate | `show` (in config mode) |
| Commit | `commit` |
| Disable paging | `set cli pager off` |
| Facts | `show system info` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: pa-fw-01
      vendor: paloalto_panos
      protocol: ssh
      address: 10.0.1.1
      credential_ref: panos-creds
```

**State module** (`panos_config`):

```yaml
states:
  - name: configure-security-zone
    module: panos_config
    parameters:
      lines:
        - "set network zone trust network layer3 ethernet1/1"
      commit: true
```

Additional methods: `Commit()`, `CommitPartial(admin)`.

## F5 BIG-IP

SSH-based adapter using the `tmsh` (Traffic Management Shell). Commands use `list`, `create`, `modify`, and `delete` verbs.

**CLI pattern**: Enters `tmsh` shell on connect. No separate config mode.

| Operation | Command |
|-----------|---------|
| List virtuals | `list ltm virtual` |
| List pools | `list ltm pool` |
| List interfaces | `list net interface` |
| Save config | `save sys config` |
| Disable paging | `modify cli preference pager disabled` |
| Facts | `show sys version`, `show sys hardware`, `list sys global-settings hostname` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: bigip-01
      vendor: f5_bigip
      protocol: ssh
      address: 10.0.1.1
      credential_ref: f5-creds
```

**State module** (`bigip_config`):

```yaml
states:
  - name: create-pool
    module: bigip_config
    parameters:
      commands:
        - "create ltm pool web-pool members add { 10.0.1.10:80 10.0.1.11:80 }"
        - "create ltm virtual web-vs destination 10.0.0.100:80 pool web-pool"
      save: true
```

Additional methods: `ListVirtuals()`, `ListPools()`.

## Check Point Gaia

SSH-based adapter using the Gaia clish (command-line interface shell). Uses `set` commands directly without an explicit config mode.

**CLI pattern**: Direct `set` commands in clish. Expert mode available via `expert` command.

| Operation | Command |
|-----------|---------|
| Show config | `show configuration` |
| Save config | `save config` |
| Disable paging | `set clienv rows 0` |
| Facts | `show version all`, `show asset all`, `show hostname` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: cp-gw-01
      vendor: checkpoint_gaia
      protocol: ssh
      address: 10.0.1.1
      credential_ref: gaia-creds
```

**State module** (`checkpoint_gaia_config`):

```yaml
states:
  - name: configure-interface
    module: checkpoint_gaia_config
    parameters:
      commands:
        - "set interface eth0 ipv4-address 10.0.1.1 mask-length 24"
        - "set interface eth0 state on"
      save: true
```

## MikroTik RouterOS

SSH-based adapter using RouterOS path-based CLI syntax. Configuration auto-saves after each command.

**CLI pattern**: Path-based commands (`/ip address add`, `/interface print`). No explicit config mode.

| Operation | Command |
|-----------|---------|
| Show config | `/export` |
| Save config | No-op (auto-saves) |
| Disable paging | `terminal width 512` |
| Facts | `/system resource print`, `/system identity print`, `/system routerboard print` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: mikrotik-01
      vendor: mikrotik_routeros
      protocol: ssh
      address: 10.0.1.1
      credential_ref: mikrotik-creds
```

**State module** (`mikrotik_routeros_config`):

```yaml
states:
  - name: configure-ip
    module: mikrotik_routeros_config
    parameters:
      commands:
        - "/ip address add address=10.0.1.1/24 interface=ether1"
        - "/ip dns set servers=8.8.8.8"
```

**SaveConfig**: No-op (RouterOS auto-saves all changes).

## Ubiquiti EdgeOS

SSH-based adapter for Ubiquiti EdgeRouter devices. Vyatta-based with transactional commit model.

**CLI pattern**: `configure` / `set` / `commit` / `save` (Vyatta-style).

| Operation | Command |
|-----------|---------|
| Show config | `show configuration` or `show configuration commands` |
| Enter config | `configure` |
| Commit | `commit` |
| Save config | `save` |
| Disable paging | `terminal length 0` |
| Facts | `show version`, `show hardware` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: edgerouter-01
      vendor: ubiquiti_edgeos
      protocol: ssh
      address: 10.0.1.1
      credential_ref: edge-creds
```

**State module** (`ubiquiti_edgeos_config`):

```yaml
states:
  - name: configure-interface
    module: ubiquiti_edgeos_config
    parameters:
      commands:
        - "interfaces ethernet eth0 address 10.0.1.1/24"
        - "interfaces ethernet eth0 description WAN"
      commit: true
      save: true
```

## Extreme EXOS

SSH-based adapter for Extreme Networks EXOS switches. No explicit config mode; all commands run at the privileged level.

**CLI pattern**: Direct commands using `create`, `configure`, `enable`, `disable` verbs. Slot-based port naming.

| Operation | Command |
|-----------|---------|
| Show config | `show configuration` |
| Save config | `save configuration primary` |
| Disable paging | `disable clipaging` |
| Facts | `show version`, `show switch`, `show ports` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: exos-switch-01
      vendor: extreme_exos
      protocol: ssh
      address: 10.0.1.10
      credential_ref: exos-creds
```

**State module** (`extreme_exos_config`):

```yaml
states:
  - name: configure-vlan
    module: extreme_exos_config
    parameters:
      commands:
        - "create vlan Management tag 100"
        - "configure vlan Management add ports 1:1-1:24 tagged"
      save: true
```

## Nokia SR OS

SSH-based adapter for Nokia (formerly Alcatel-Lucent) SR OS routers. Uses classic CLI mode.

**CLI pattern**: `configure` / commands / `exit all`. Uses `admin` prefix for administrative operations.

| Operation | Command |
|-----------|---------|
| Show config | `admin display-config` |
| Enter config | `configure` |
| Exit config | `exit all` |
| Save config | `admin save` |
| Disable paging | `environment no more` |
| Facts | `show system information`, `show version`, `show chassis` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: nokia-pe-01
      vendor: nokia_sros
      protocol: ssh
      address: 10.0.1.1
      credential_ref: nokia-creds
```

**State module** (`nokia_sros_config`):

```yaml
states:
  - name: configure-interface
    module: nokia_sros_config
    parameters:
      commands:
        - "router interface system address 10.0.0.1/32"
      save: true
```

## Huawei VRP

SSH-based adapter for Huawei VRP (Versatile Routing Platform) devices. Uses `display` instead of `show` and `system-view` for configuration mode.

**CLI pattern**: `system-view` to enter config (like `configure terminal`). Uses `undo` to negate commands.

| Operation | Command |
|-----------|---------|
| Show config | `display current-configuration` |
| Enter config | `system-view` |
| Exit config | `return` |
| Save config | `save` (prompts for `Y` confirmation) |
| Disable paging | `screen-length 0 temporary` |
| Facts | `display version`, `display device`, `display interface brief` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: huawei-sw-01
      vendor: huawei_vrp
      protocol: ssh
      address: 10.0.1.10
      credential_ref: huawei-creds
```

**State module** (`huawei_vrp_config`):

```yaml
states:
  - name: configure-interface
    module: huawei_vrp_config
    parameters:
      lines:
        - "interface GE0/0/1"
        - "ip address 10.0.1.1 255.255.255.0"
      save: true
```

## Mellanox/NVIDIA Onyx

SSH-based adapter for Mellanox/NVIDIA Onyx switches (formerly MLNX-OS). IOS-like CLI.

**CLI pattern**: `enable` / `configure terminal` / `end`. Saves with `configuration write`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure terminal` |
| Save config | `configuration write` |
| Disable paging | `no cli session paging enable` |
| Facts | `show version`, `show system`, `show interfaces status` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: mlnx-switch-01
      vendor: mellanox_onyx
      protocol: ssh
      address: 10.0.1.10
      credential_ref: mlnx-creds
```

**State module** (`mellanox_onyx_config`):

```yaml
states:
  - name: configure-interface
    module: mellanox_onyx_config
    parameters:
      lines:
        - "interface ethernet 1/1"
        - "no shutdown"
      save: true
```

## Allied Telesis AlliedWare Plus

SSH-based adapter for Allied Telesis switches running AlliedWare Plus. IOS-like CLI.

**CLI pattern**: `enable` / `configure terminal` / `end`.

| Operation | Command |
|-----------|---------|
| Show config | `show running-config` |
| Enter config | `configure terminal` |
| Save config | `write` |
| Disable paging | `terminal length 0` |
| Facts | `show version`, `show system` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: at-switch-01
      vendor: alliedtelesis_awplus
      protocol: ssh
      address: 10.0.1.10
      credential_ref: at-creds
```

**State module** (`alliedtelesis_awplus_config`):

```yaml
states:
  - name: configure-vlan
    module: alliedtelesis_awplus_config
    parameters:
      lines:
        - "vlan database"
        - "vlan 100 name Management"
      save: true
```

## Ciena SAOS

SSH-based adapter for Ciena SAOS switches and packet-optical platforms. Uses noun-first command ordering.

**CLI pattern**: Direct commands with noun-first ordering (`configuration show`, `port set`). No explicit config mode.

| Operation | Command |
|-----------|---------|
| Show config | `configuration show` |
| Save config | `configuration save` |
| Disable paging | `system shell set terminal rows infinite` |
| Facts | `software show`, `chassis show`, `port show` |

**Configuration**:

```yaml
proxy:
  devices:
    - id: ciena-01
      vendor: ciena_saos
      protocol: ssh
      address: 10.0.1.10
      credential_ref: ciena-creds
```

**State module** (`ciena_saos_config`):

```yaml
states:
  - name: configure-port
    module: ciena_saos_config
    parameters:
      commands:
        - "port set port 1/1 admin-state enabled"
        - "port set port 1/1 description Uplink"
      save: true
```

## State Module Parameters

### IOS-style Modules

Used by: `ios_config`, `nxos_config`, `eos_config`, `hp_procurve_config`, `hp_arubaos_config`, `hp_aoscx_config`, `dell_os10_config`, `dell_os9_config`, `dell_powerswitch_config`, `huawei_vrp_config`, `mellanox_onyx_config`, `alliedtelesis_awplus_config`.

| Parameter | Type | Description |
|-----------|------|-------------|
| `lines` | []string | Configuration lines to apply (required) |
| `parents` | []string | Parent context commands (e.g., `interface GigabitEthernet0/1`) |
| `save` | bool | Save configuration after applying |
| `before` | []string | Commands to execute before lines (IOS only) |
| `after` | []string | Commands to execute after lines (IOS only) |
| `replace` | string | Replace mode: `block` replaces entire section (IOS only) |
| `backup` | bool | Backup current config before changes (IOS only) |

### FortiOS Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `section` | string | Config section (required, e.g., `system interface`) |
| `name` | string | Object name to edit (e.g., `port1`) |
| `settings` | map | Key-value settings to apply |
| `backup` | bool | Backup current config before changes |

### PAN-OS Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `lines` | []string | Set/delete commands (required) |
| `commit` | bool | Commit after applying changes |

### BIG-IP Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `commands` | []string | tmsh commands to execute (required) |
| `save` | bool | Save config after applying |

### Direct-Command Modules

Used by: `checkpoint_gaia_config`, `extreme_exos_config`, `ciena_saos_config`.

| Parameter | Type | Description |
|-----------|------|-------------|
| `commands` | []string | Commands to execute (required) |
| `save` | bool | Save config after applying |

### MikroTik RouterOS Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `commands` | []string | Path-based commands to execute (required) |

Note: RouterOS auto-saves; no explicit save parameter needed.

### Ubiquiti EdgeOS Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `commands` | []string | Set/delete commands (required) |
| `commit` | bool | Commit changes after applying |
| `save` | bool | Save configuration after commit |

### Nokia SR OS Module

| Parameter | Type | Description |
|-----------|------|-------------|
| `commands` | []string | Commands to execute in configure mode (required) |
| `save` | bool | Run `admin save` after applying |

## DeviceFacts

All drivers populate a common `DeviceFacts` structure:

| Field | Description |
|-------|-------------|
| `hostname` | Device hostname |
| `fqdn` | Fully qualified domain name |
| `model` | Hardware model |
| `serial_number` | Serial number |
| `vendor` | Vendor name |
| `os_type` | Operating system type |
| `os_version` | Software version |
| `uptime` | System uptime |
| `interfaces` | Network interface list |
| `memory_total` | Total memory (bytes) |
| `memory_free` | Free memory (bytes) |
| `cpu_usage` | CPU usage percentage |

## Credential Rotation

The credential rotation framework automatically rotates device credentials based on configurable policies. It supports all credential types used by vendor drivers.

### Rotation Providers

| Provider | Credential Types | Generation | Verification |
|----------|-----------------|------------|--------------|
| SSH | `ssh_password`, `ssh_key` | Random password, ed25519 keypair | SSH connection test |
| SNMP | `snmpv2c`, `snmpv3` | Random community/passwords | SNMP GET test |
| REST | `rest_basic`, `rest_bearer`, `rest_apikey`, `rest_oauth2` | API-driven rotation | Authenticated API call |
| Certificate | `gnmi` | ECDSA P-256 certificate | TLS handshake |

### Rotation Workflow

Each rotation follows a state machine workflow with automatic rollback on failure:

```
Pending → ValidatingOld → Generating → Applying → VerifyingNew → Storing → Cleanup → Completed
```

If any stage fails, the engine transitions to `Failed` and optionally rolls back to the previous credential.

### Rotation Policy Example

```yaml
credential_rotation:
  policies:
    - id: ssh-90-day
      credential_types: [ssh_password, ssh_key]
      max_age: 2160h      # 90 days
      warning_age: 1800h   # 75 days
      schedule: "0 2 * * 0"  # Weekly at 2 AM Sunday
      auto_rotate: true
      rollback_on_fail: true

    - id: snmp-quarterly
      credential_types: [snmpv2c, snmpv3]
      max_age: 2160h
      schedule: "0 3 1 */3 *"  # 3 AM on 1st of every 3rd month
      auto_rotate: true
```

## See Also

- [Proxy Agents]({{< relref "../concepts/proxy-agents.md" >}})
- [Proxy Agent Operations]({{< relref "../operations/proxy-agents.md" >}})
- [NETCONF Protocol Reference]({{< relref "netconf.md" >}})
- [RESTCONF Protocol Reference]({{< relref "restconf.md" >}})
- [gNMI Protocol Reference]({{< relref "gnmi.md" >}})
- [Telnet Protocol Reference]({{< relref "telnet.md" >}})
