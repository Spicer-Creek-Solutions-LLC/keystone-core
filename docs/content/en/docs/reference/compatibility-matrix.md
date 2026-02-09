---
title: "Protocol Compatibility Matrix"
weight: 27
description: >
  Compatibility matrix showing protocol support across vendor platforms
---

## Protocol Support Matrix

| Vendor / Platform | SSH | NETCONF | RESTCONF | gNMI | SNMP | REST API | Telnet | WinRM |
|-------------------|-----|---------|----------|------|------|----------|--------|-------|
| Cisco IOS-XE | Yes | Yes | Yes | Yes | Yes | - | Yes | - |
| Cisco IOS (classic) | Yes | - | - | - | Yes | - | Yes | - |
| Cisco NX-OS | Yes | Yes | Yes | - | Yes | NX-API | - | - |
| Juniper JUNOS | Yes | Yes | - | Yes | Yes | - | - | - |
| Arista EOS | Yes | Yes | Yes | Yes | Yes | eAPI | - | - |
| VyOS | Yes | - | - | - | Yes | - | - | - |
| pfSense | Yes | - | - | - | - | REST | - | - |
| OPNsense | Yes | - | - | - | - | REST | - | - |
| HP ProCurve | Yes | - | - | - | Yes | - | Yes | - |
| HP ArubaOS | Yes | - | - | - | Yes | REST | - | - |
| HP AOS-CX | Yes | Yes | Yes | - | Yes | REST | - | - |
| Dell OS10 | Yes | Yes | - | Yes | Yes | REST | - | - |
| Dell OS9 | Yes | - | - | - | Yes | - | Yes | - |
| Dell PowerSwitch | Yes | - | - | - | Yes | - | - | - |
| Fortinet FortiOS | Yes | - | - | - | Yes | REST | - | - |
| Palo Alto PAN-OS | Yes | - | - | - | Yes | XML API | - | - |
| F5 BIG-IP | Yes | - | - | - | Yes | iControl REST | - | - |
| Check Point Gaia | Yes | - | - | - | Yes | Web API | - | - |
| MikroTik RouterOS | Yes | - | - | - | Yes | REST | - | - |
| Ubiquiti EdgeOS | Yes | - | - | - | Yes | - | - | - |
| Extreme EXOS | Yes | Yes | - | - | Yes | - | - | - |
| Nokia SR OS | Yes | Yes | - | Yes | Yes | - | - | - |
| Huawei VRP | Yes | Yes | - | - | Yes | - | Yes | - |
| Mellanox Onyx | Yes | - | - | - | Yes | - | - | - |
| Allied Telesis AWP | Yes | - | - | - | Yes | REST | - | - |
| Ciena SAOS | Yes | Yes | - | - | Yes | - | - | - |
| Windows Server | - | - | - | - | - | - | - | Yes |

## State Module Compatibility

| Module | Compatible Vendors |
|--------|--------------------|
| `netconf_interface` | All NETCONF-capable vendors (Cisco IOS-XE, NX-OS, Juniper, Arista, HP AOS-CX, Dell OS10, Nokia, Huawei, Ciena) |
| `netconf_vlan` | All NETCONF-capable vendors with OpenConfig VLAN support |
| `netconf_routing` | All NETCONF-capable vendors with OpenConfig network-instance support |
| `netconf_acl` | All NETCONF-capable vendors with OpenConfig ACL support |
| `fortios_policy` | Fortinet FortiOS 6.0+ |
| `panos_rule` | Palo Alto PAN-OS 8.0+ |
| `bigip_pool` | F5 BIG-IP 12.0+ |
| `bigip_virtual` | F5 BIG-IP 12.0+ |
| `checkpoint_rule` | Check Point R80+ |

## NETCONF Capability Matrix

| Capability | IOS-XE | NX-OS | JUNOS | EOS | AOS-CX | OS10 | SR OS | VRP |
|------------|--------|-------|-------|-----|--------|------|-------|-----|
| base:1.0 | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| base:1.1 | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| writable-running | Yes | Yes | - | Yes | Yes | Yes | - | Yes |
| candidate | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| confirmed-commit | Yes | - | Yes | - | Yes | Yes | Yes | - |
| rollback-on-error | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| validate | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| startup | Yes | - | - | - | - | - | Yes | - |
| xpath | Yes | - | Yes | - | - | Yes | Yes | - |

## Credential Rotation Support

| Protocol | Password Rotation | Key Rotation | Token Rotation | Certificate Rotation |
|----------|-------------------|--------------|----------------|----------------------|
| SSH | Yes | Yes | - | - |
| SNMPv2c | Yes (community) | - | - | - |
| SNMPv3 | Yes (auth/priv) | - | - | - |
| REST Basic | Yes | - | - | - |
| REST Bearer | - | - | Yes | - |
| REST API Key | - | - | Yes | - |
| REST OAuth2 | - | - | Yes (secret) | - |
| gNMI | - | - | - | Yes |

## Minimum Firmware Versions

| Vendor | Platform | Minimum Version | Notes |
|--------|----------|-----------------|-------|
| Cisco | IOS-XE | 16.6 | NETCONF/RESTCONF support |
| Cisco | NX-OS | 7.0(3) | NETCONF support |
| Juniper | JUNOS | 14.1 | NETCONF 1.1 |
| Arista | EOS | 4.20 | gNMI/OpenConfig support |
| Fortinet | FortiOS | 6.0 | REST API v2 |
| Palo Alto | PAN-OS | 8.0 | XML API stable |
| F5 | BIG-IP | 12.0 | iControl REST |
| Check Point | Gaia | R80 | Web Services API |
| Nokia | SR OS | 16.0 | OpenConfig/gNMI |
| Huawei | VRP | V800R011 | NETCONF support |
