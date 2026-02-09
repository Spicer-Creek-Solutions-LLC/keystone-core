---
title: "Vendor Configuration Guide"
weight: 24
description: >
  Step-by-step guides for configuring proxy agents to manage vendor-specific network devices
---

## Prerequisites

Before using vendor-specific modules, ensure:

1. A proxy agent is deployed and connected to the control plane
2. Device credentials are configured in the credential store
3. The device is reachable from the proxy agent
4. API access is enabled on the target device (for REST/XML API modules)

## FortiGate Configuration

### Initial Setup

Enable the REST API on your FortiGate device:

```
config system api-user
  edit "keystone"
    set accprofile "super_admin"
    set vdom "root"
    config trusthost
      edit 1
        set ipv4-trusthost 10.0.0.0/8
      next
    end
  next
end
```

### Managing Firewall Policies

Use the `fortios_policy` module to manage firewall policies:

```yaml
proxy_state:
  - module: fortios_policy
    parameters:
      policy_id: 1
      name: allow-web
      srcintf: [port1]
      dstintf: [wan1]
      srcaddr: [all]
      dstaddr: [WebServers]
      action: accept
      service: [HTTP, HTTPS]
      schedule: always
      nat: true
      logtraffic: all
```

### Removing a Policy

```yaml
proxy_state:
  - module: fortios_policy
    parameters:
      policy_id: 99
      state: absent
```

## Palo Alto PAN-OS Configuration

### Initial Setup

Create an API key on your Palo Alto device:

```
request api keygen username=admin password=<password>
```

Store the API key as a REST API credential in Keystone Core.

### Managing Security Rules

```yaml
proxy_state:
  - module: panos_rule
    parameters:
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

### Managing NAT Rules

```yaml
proxy_state:
  - module: panos_rule
    parameters:
      name: outbound-nat
      rule_type: nat
      source_zone: [trust]
      dest_zone: [untrust]
      source: [internal-nets]
      destination: [any]
      commit: true
```

## F5 BIG-IP Configuration

### Initial Setup

Ensure the iControl REST API is accessible (default on port 443). Use REST Basic authentication with an admin account.

### Managing Pools

```yaml
proxy_state:
  - module: bigip_pool
    parameters:
      name: /Common/web-pool
      lb_method: least-connections-member
      monitors: [/Common/http]
      members:
        - address: 10.0.0.1
          port: 80
        - address: 10.0.0.2
          port: 80
      description: "Web application pool"
```

### Managing Virtual Servers

```yaml
proxy_state:
  - module: bigip_virtual
    parameters:
      name: /Common/web-vs
      destination: 10.0.0.100
      port: 443
      pool: /Common/web-pool
      profiles: [http, clientssl]
      snat: automap
      description: "HTTPS virtual server"
```

### Disabling a Virtual Server

```yaml
proxy_state:
  - module: bigip_virtual
    parameters:
      name: /Common/web-vs
      state: disabled
```

## Check Point Configuration

### Initial Setup

Enable the Web Services API on your Check Point Management Server:

```bash
mgmt_cli login user admin password <password> --format json
mgmt_cli set api-settings accepted-api-calls-from "All IP addresses" --format json
mgmt_cli publish --format json
```

### Managing Access Rules

```yaml
proxy_state:
  - module: checkpoint_rule
    parameters:
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

### Removing a Rule

```yaml
proxy_state:
  - module: checkpoint_rule
    parameters:
      name: old-rule
      state: absent
```

## NETCONF Device Configuration

### Supported Devices

Any device supporting NETCONF with OpenConfig YANG models can use the NETCONF state modules. Common vendors include Cisco IOS-XE, Juniper JUNOS, Arista EOS, and Nokia SR OS.

### Interface Management

```yaml
proxy_state:
  - module: netconf_interface
    parameters:
      name: GigabitEthernet0/0/0
      description: "Uplink to core"
      enabled: true
      mtu: 9000
      ip_address: 10.0.0.1
      ip_prefix_length: 30
```

### VLAN Management

```yaml
proxy_state:
  - module: netconf_vlan
    parameters:
      vlan_id: 100
      name: production
      vlan_state: active
```

### Static Routing

```yaml
proxy_state:
  - module: netconf_routing
    parameters:
      prefix: 10.0.0.0/8
      next_hop: 192.168.1.1
      metric: 100
      vrf: mgmt
```

### Access Control Lists

```yaml
proxy_state:
  - module: netconf_acl
    parameters:
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
        - sequence: 30
          action: deny
          source: 192.168.0.0/16
          destination: 0.0.0.0/0
        - sequence: 100
          action: permit
          source: 0.0.0.0/0
          destination: 0.0.0.0/0
```
