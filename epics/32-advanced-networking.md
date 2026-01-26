# Epic 32: Advanced Networking Features

## Overview

This epic covers advanced networking features that extend beyond the basic network, VLAN, bond, and bridge modules implemented in Epic 31 (Network Features Enhancement).

## Status

**COMPLETE** ✅

### Completed
- [x] WiFi module implementation (Linux nmcli/wpa_supplicant, macOS networksetup, Windows netsh wlan)
- [x] WiFi module tests (config parsing, validation, wpa_supplicant block generation, Windows XML generation)
- [x] WiFi module documentation
- [x] 802.1X module implementation (Linux NetworkManager/wpa_supplicant, macOS profiles, Windows dot3svc)
- [x] 802.1X module tests (config parsing, validation, wpa_supplicant config generation, Windows XML, macOS profile)
- [x] 802.1X module documentation
- [x] Link settings module implementation (Linux ethtool, macOS ifconfig, Windows netsh/PowerShell)
- [x] Link settings module tests (config parsing, validation, platform helpers)
- [x] Link settings module documentation
- [x] Promiscuous mode module implementation (Linux ip link, macOS/BSD ifconfig, Windows PowerShell)
- [x] Promiscuous mode module tests (config parsing, validation, flag parsing)
- [x] Promiscuous mode module documentation

## Scope

### In Scope

1. **Wireless (WiFi) Configuration**
   - SSID and password management
   - Security modes (WPA2, WPA3, WEP, Open)
   - Hidden network support
   - Multiple network profiles
   - Priority/roaming configuration

2. **802.1X Authentication**
   - EAP-TLS, EAP-TTLS, PEAP support
   - Certificate management
   - RADIUS integration
   - Wired and wireless 802.1X

3. **Link Speed/Duplex Settings**
   - Force specific speed (10/100/1000/10000 Mbps)
   - Force duplex mode (half/full)
   - Auto-negotiation control
   - Interface diagnostics

4. **Promiscuous Mode**
   - Enable/disable promiscuous mode
   - Use cases: bridging, packet capture, IDS
   - Security considerations

### Out of Scope

- Network namespaces (container/VM specific)
- Traffic shaping/QoS (separate epic)
- Firewall rules (handled by existing firewall module)
- VPN configuration (separate epic)

## Dependencies

- Epic 3 (State Management) - Base module system
- Network module (current)
- VLAN, Bond, Bridge modules (current)

## Technical Approach

### WiFi Module

```yaml
wifi:
  office_network:
    state: connected
    ssid: "Office WiFi"
    security: wpa2-psk
    password: "{{ vault.wifi_password }}"
    interface: wlan0
    priority: 100

  backup_network:
    state: configured  # Known but not active
    ssid: "Backup WiFi"
    security: wpa3
    password: "{{ vault.backup_wifi }}"
    priority: 50
```

**Backend support:**
- Linux: NetworkManager (nmcli), wpa_supplicant
- macOS: networksetup, airport
- Windows: netsh wlan

### 802.1X Module

```yaml
dot1x:
  wired_auth:
    state: enabled
    interface: eth0
    eap_method: tls
    identity: "user@example.com"
    client_cert: /etc/pki/client.crt
    client_key: /etc/pki/client.key
    ca_cert: /etc/pki/ca.crt
```

### Link Settings Module

```yaml
link:
  eth0:
    state: configured
    speed: 1000  # Mbps
    duplex: full
    autoneg: false
```

**Implementation:** ethtool on Linux, platform-specific on macOS/Windows

## User Stories

### WiFi Configuration

1. As an operator, I want to configure WiFi networks declaratively so that managed devices can connect automatically.
2. As an operator, I want to prioritize WiFi networks so devices prefer faster/more reliable networks.
3. As an operator, I want to manage WiFi passwords securely using the secrets system.

### 802.1X Authentication

4. As a security engineer, I want to require 802.1X authentication on wired ports for network access control.
5. As an operator, I want to deploy certificates for 802.1X authentication.

### Link Settings

6. As an operator, I want to force specific link speeds for compatibility with legacy equipment.
7. As an operator, I want to diagnose link issues by checking negotiated speed/duplex.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| WiFi varies significantly by driver/hardware | High | Support major network managers only |
| 802.1X requires complex PKI | Medium | Integrate with existing certificate management |
| Forcing link settings can cause connectivity loss | High | Implement rollback on failure |

## Estimated Effort

- WiFi module: 2-3 weeks
- 802.1X module: 2-3 weeks
- Link settings module: 1 week
- Promiscuous mode: 1 week
- Testing and documentation: 2 weeks

**Total: 8-10 weeks**

## Success Criteria

- [x] WiFi module supports WPA2/WPA3 on Linux, macOS, Windows
- [x] 802.1X module supports EAP-TLS, EAP-TTLS, EAP-PEAP on Linux, macOS, Windows
- [x] Link settings module works with ethtool on Linux, ifconfig on macOS, netsh on Windows
- [x] Promiscuous mode module works on Linux, macOS, Windows
- [x] All modules have comprehensive test coverage
- [x] WiFi module documentation complete with examples
- [x] 802.1X module documentation complete with examples
- [x] Link settings module documentation complete with examples
- [x] Promiscuous mode module documentation complete with examples

## References

- wpa_supplicant documentation
- NetworkManager WiFi configuration
- IEEE 802.1X specification
- ethtool man page
