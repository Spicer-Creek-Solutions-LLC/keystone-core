# Epic 42: Network Protocol Expansion

## Overview

This epic extends the proxy agent system (Epic 21) with additional network management protocols and expanded vendor support. While Epic 21 established the foundation with SSH, SNMP, REST, and WinRM, this epic adds enterprise-grade protocols (NETCONF, RESTCONF, Telnet) and expands the vendor compatibility matrix.

**Epic Type**: Feature, Integration

**Scope**:
- NETCONF protocol adapter (RFC 6241)
- RESTCONF protocol adapter (RFC 8040)
- Telnet protocol adapter (legacy device support)
- gNMI/gNOI protocol adapter (modern streaming telemetry)
- Expanded vendor support (15+ vendors)
- Universal credential rotation for all backends
- Protocol-specific state modules
- Comprehensive vendor compatibility matrix

**Out of Scope**:
- Proxy agent architecture changes (Epic 21)
- New discovery mechanisms (existing infrastructure)
- Network topology mapping (future epic)
- Traffic analysis or packet capture

## Rationale

### Problem Statement

Epic 21 (Proxy Agents) established core protocols but explicitly deferred:

| TODO Item | Description |
|-----------|-------------|
| Add support for additional protocols (Telnet, NETCONF, RESTCONF) | Enterprise protocols missing |
| Expand vendor support (HP, Dell, Checkpoint, etc.) | Limited vendor matrix |
| Implement automatic credential rotation for all backends | Incomplete rotation |
| Benchmark large numbers of proxy agents | Scale testing needed |

Current limitations:
1. **Protocol Gaps**: Modern network automation relies heavily on NETCONF/RESTCONF
2. **Vendor Gaps**: Major vendors (HP/Aruba, Dell, Fortinet, F5) not supported
3. **Legacy Devices**: Telnet-only devices cannot be managed
4. **Streaming Telemetry**: No gNMI support for modern observability
5. **Credential Security**: Rotation not universal across protocols

### Benefits

1. **Enterprise Ready**: NETCONF/RESTCONF for modern network automation
2. **Legacy Support**: Telnet for older devices that can't be upgraded
3. **Vendor Coverage**: Support for 90%+ of enterprise network devices
4. **Modern Telemetry**: gNMI for streaming metrics and state
5. **Security**: Universal credential rotation across all protocols
6. **Completeness**: Full-featured network automation platform

## Objectives

1. **O1**: Implement NETCONF protocol adapter with full RFC 6241 compliance
2. **O2**: Implement RESTCONF protocol adapter with RFC 8040 compliance
3. **O3**: Implement Telnet protocol adapter with security controls
4. **O4**: Implement gNMI/gNOI protocol adapter for streaming telemetry
5. **O5**: Expand vendor support to 15+ network device vendors
6. **O6**: Implement universal credential rotation across all protocols
7. **O7**: Create protocol-specific state modules for configuration management
8. **O8**: Achieve >80% compatibility with enterprise network device inventory

## Protocol Support Matrix

### Current Protocols (Epic 21)

| Protocol | Status | Use Case |
|----------|--------|----------|
| SSH | ✅ Complete | CLI-based device management |
| SNMP v2c/v3 | ✅ Complete | Monitoring, basic config |
| REST/HTTP | ✅ Complete | Modern API-driven devices |
| WinRM | ✅ Complete | Windows-based appliances |

### New Protocols (This Epic)

| Protocol | Priority | Use Case |
|----------|----------|----------|
| NETCONF | P0 | Configuration management, transactions |
| RESTCONF | P0 | RESTful YANG data access |
| Telnet | P1 | Legacy device support |
| gNMI | P1 | Streaming telemetry |
| gNOI | P2 | gRPC network operations |

## Vendor Support Matrix

### Current Vendors (Epic 21)

| Vendor | Platforms | Protocols |
|--------|-----------|-----------|
| Cisco | IOS, IOS-XE, NX-OS | SSH, SNMP, RESTCONF |
| Juniper | JUNOS | SSH, SNMP, NETCONF |
| Arista | EOS | SSH, SNMP, REST |
| VyOS | VyOS | SSH |
| pfSense | pfSense | SSH, REST |
| OPNsense | OPNsense | SSH, REST |

### New Vendors (This Epic)

| Vendor | Platforms | Protocols | Priority |
|--------|-----------|-----------|----------|
| HP/Aruba | ProCurve, ArubaOS, AOS-CX | SSH, SNMP, REST, NETCONF | P0 |
| Dell | OS10, OS9, PowerSwitch | SSH, SNMP, REST, NETCONF | P0 |
| Fortinet | FortiOS | SSH, REST | P0 |
| F5 | BIG-IP, BIG-IQ | SSH, REST, iControl | P0 |
| Palo Alto | PAN-OS | SSH, REST, XML API | P0 |
| Checkpoint | Gaia | SSH, REST, MGMT API | P1 |
| MikroTik | RouterOS | SSH, REST API | P1 |
| Ubiquiti | EdgeOS, UniFi | SSH, REST | P1 |
| Extreme | EXOS, VOSS | SSH, SNMP, REST | P1 |
| Brocade/Ruckus | FabricOS, ICX | SSH, SNMP, REST | P2 |
| Nokia | SR OS | SSH, NETCONF, gNMI | P2 |
| Huawei | VRP | SSH, NETCONF | P2 |
| Mellanox/NVIDIA | Onyx, Cumulus | SSH, REST, NETCONF | P2 |
| Allied Telesis | AlliedWare Plus | SSH, REST | P2 |
| Ciena | SAOS | SSH, NETCONF, REST | P2 |

## Architecture

### Protocol Adapter Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Protocol Adapter Framework                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    Protocol Abstraction Layer                     │   │
│  │                                                                   │   │
│  │  interface ProtocolAdapter {                                      │   │
│  │      Connect(ctx, config) error                                   │   │
│  │      Disconnect() error                                           │   │
│  │      Execute(cmd) (Result, error)                                 │   │
│  │      GetConfig(path) ([]byte, error)                              │   │
│  │      SetConfig(path, data) error                                  │   │
│  │      Subscribe(path, handler) error  // Streaming                 │   │
│  │      Capabilities() []Capability                                  │   │
│  │  }                                                                │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐          │
│  │   SSH   │ │  SNMP   │ │ NETCONF │ │RESTCONF │ │  gNMI   │          │
│  │ Adapter │ │ Adapter │ │ Adapter │ │ Adapter │ │ Adapter │          │
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘          │
│       │           │           │           │           │                │
│  ┌────┴────┐ ┌────┴────┐ ┌────┴────┐ ┌────┴────┐ ┌────┴────┐          │
│  │  REST   │ │  WinRM  │ │ Telnet  │ │  gNOI   │ │  Custom │          │
│  │ Adapter │ │ Adapter │ │ Adapter │ │ Adapter │ │ Adapter │          │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘          │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    Vendor Abstraction Layer                       │   │
│  │                                                                   │   │
│  │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐         │   │
│  │  │ Cisco  │ │Juniper │ │  HP/   │ │  Dell  │ │Fortinet│         │   │
│  │  │ Driver │ │ Driver │ │ Aruba  │ │ Driver │ │ Driver │         │   │
│  │  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘         │   │
│  │                                                                   │   │
│  │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐         │   │
│  │  │   F5   │ │  Palo  │ │  Check │ │MikroTik│ │  ...   │         │   │
│  │  │ Driver │ │  Alto  │ │  point │ │ Driver │ │        │         │   │
│  │  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘         │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### NETCONF Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    NETCONF Protocol Stack                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Operations Layer                       │   │
│  │  <get> <get-config> <edit-config> <copy-config>          │   │
│  │  <delete-config> <lock> <unlock> <close-session>         │   │
│  │  <kill-session> <commit> <discard-changes> <validate>    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Content Layer (YANG)                   │   │
│  │  - Standard YANG models (ietf-interfaces, etc.)          │   │
│  │  - Vendor-specific YANG models                            │   │
│  │  - OpenConfig models                                      │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Message Layer (RPC)                    │   │
│  │  - XML encoding                                           │   │
│  │  - Message framing                                        │   │
│  │  - Capability exchange                                    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Transport Layer                        │   │
│  │  - SSH subsystem (default)                                │   │
│  │  - TLS (optional)                                         │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
pkg/proxy/
├── protocol/
│   ├── protocol.go              # Protocol interface
│   ├── registry.go              # Protocol registry
│   ├── ssh/                     # Existing SSH adapter
│   ├── snmp/                    # Existing SNMP adapter
│   ├── rest/                    # Existing REST adapter
│   ├── winrm/                   # Existing WinRM adapter
│   ├── netconf/
│   │   ├── adapter.go           # NETCONF adapter
│   │   ├── session.go           # Session management
│   │   ├── rpc.go               # RPC operations
│   │   ├── capabilities.go      # Capability handling
│   │   ├── yang.go              # YANG model support
│   │   └── transport.go         # SSH/TLS transport
│   ├── restconf/
│   │   ├── adapter.go           # RESTCONF adapter
│   │   ├── client.go            # HTTP client
│   │   ├── datastore.go         # Datastore operations
│   │   └── yang.go              # YANG-JSON mapping
│   ├── telnet/
│   │   ├── adapter.go           # Telnet adapter
│   │   ├── session.go           # Session management
│   │   ├── expect.go            # Expect-like matching
│   │   └── security.go          # Security controls
│   └── gnmi/
│       ├── adapter.go           # gNMI adapter
│       ├── client.go            # gRPC client
│       ├── subscribe.go         # Subscription handling
│       └── gnoi.go              # gNOI operations
├── vendor/
│   ├── vendor.go                # Vendor interface
│   ├── registry.go              # Vendor registry
│   ├── cisco/
│   │   ├── ios.go               # IOS driver
│   │   ├── iosxe.go             # IOS-XE driver
│   │   ├── nxos.go              # NX-OS driver
│   │   └── iosxr.go             # IOS-XR driver
│   ├── juniper/
│   │   └── junos.go             # JUNOS driver
│   ├── arista/
│   │   └── eos.go               # EOS driver
│   ├── hp/
│   │   ├── procurve.go          # ProCurve driver
│   │   ├── arubaos.go           # ArubaOS driver
│   │   └── aoscx.go             # AOS-CX driver
│   ├── dell/
│   │   ├── os10.go              # OS10 driver
│   │   └── powerswitch.go       # PowerSwitch driver
│   ├── fortinet/
│   │   └── fortios.go           # FortiOS driver
│   ├── f5/
│   │   ├── bigip.go             # BIG-IP driver
│   │   └── icontrol.go          # iControl REST
│   ├── paloalto/
│   │   └── panos.go             # PAN-OS driver
│   ├── checkpoint/
│   │   └── gaia.go              # Gaia driver
│   ├── mikrotik/
│   │   └── routeros.go          # RouterOS driver
│   └── ...                       # Additional vendors
├── credential/
│   ├── rotation.go              # Universal rotation
│   ├── vault.go                 # Vault integration
│   ├── k8s.go                   # K8s secrets
│   └── schedule.go              # Rotation scheduling
└── state/
    ├── netconf/
    │   ├── interface.go         # Interface state
    │   ├── vlan.go              # VLAN state
    │   ├── routing.go           # Routing state
    │   └── acl.go               # ACL state
    └── vendor/
        ├── cisco_interface.go   # Cisco-specific
        ├── juniper_interface.go # Juniper-specific
        └── ...                   # Vendor-specific
```

## Deliverables

### D1: NETCONF Protocol Adapter

Full RFC 6241 NETCONF implementation.

**Features**:
- All standard operations (get, get-config, edit-config, etc.)
- Candidate/running/startup datastore support
- Transaction support with commit/rollback
- Capability negotiation
- YANG model awareness
- SSH and TLS transport
- Chunked framing (RFC 6242)

**Operations**:
```go
// NETCONF operations
func (n *NetconfAdapter) GetConfig(source, filter string) ([]byte, error)
func (n *NetconfAdapter) EditConfig(target string, config []byte, opts EditOptions) error
func (n *NetconfAdapter) CopyConfig(source, target string) error
func (n *NetconfAdapter) Lock(target string) error
func (n *NetconfAdapter) Unlock(target string) error
func (n *NetconfAdapter) Commit() error
func (n *NetconfAdapter) DiscardChanges() error
func (n *NetconfAdapter) Validate(source string) error
```

### D2: RESTCONF Protocol Adapter

Full RFC 8040 RESTCONF implementation.

**Features**:
- YANG-modeled data access via REST
- JSON and XML encoding
- Query parameters (depth, fields, filter)
- PATCH operations (RFC 8072)
- Event notifications (SSE)
- Schema discovery

**API Mapping**:
```
GET    /restconf/data/{path}              # Read data
POST   /restconf/data/{path}              # Create data
PUT    /restconf/data/{path}              # Replace data
PATCH  /restconf/data/{path}              # Update data
DELETE /restconf/data/{path}              # Delete data
GET    /restconf/operations               # List operations
POST   /restconf/operations/{rpc}         # Execute RPC
GET    /restconf/yang-library-version     # YANG library
```

### D3: Telnet Protocol Adapter

Secure Telnet adapter for legacy devices.

**Features**:
- Expect-style pattern matching
- Prompt detection
- Command timeout handling
- Session multiplexing
- Security controls (IP allowlisting, audit logging)
- Deprecation warnings

**Security Controls**:
```yaml
telnet:
  enabled: true
  security:
    allowed_networks:
      - 10.0.0.0/8
      - 192.168.0.0/16
    require_audit: true
    deprecation_warning: true
    max_session_duration: 30m
```

### D4: gNMI/gNOI Protocol Adapter

gRPC Network Management Interface implementation.

**gNMI Features**:
- Capabilities RPC
- Get RPC (config/state/operational)
- Set RPC (update, replace, delete)
- Subscribe RPC (stream, once, poll)
- Path encoding (origin, elem, target)

**gNOI Features**:
- System operations (reboot, ping, traceroute)
- Certificate management
- File operations
- Factory reset

**Subscription Modes**:
```go
// gNMI subscription
sub := &gnmi.SubscribeRequest{
    Subscribe: &gnmi.SubscriptionList{
        Mode: gnmi.SubscriptionList_STREAM,
        Subscription: []*gnmi.Subscription{
            {
                Path: &gnmi.Path{
                    Elem: []*gnmi.PathElem{
                        {Name: "interfaces"},
                        {Name: "interface", Key: map[string]string{"name": "eth0"}},
                        {Name: "state"},
                    },
                },
                Mode: gnmi.SubscriptionMode_SAMPLE,
                SampleInterval: 10 * time.Second,
            },
        },
    },
}
```

### D5: Expanded Vendor Drivers

Vendor-specific drivers for all supported platforms.

**P0 Vendors (8 drivers)**:
- HP/Aruba: ProCurve, ArubaOS, AOS-CX
- Dell: OS10, OS9, PowerSwitch
- Fortinet: FortiOS
- F5: BIG-IP (iControl REST)
- Palo Alto: PAN-OS (XML API + REST)

**P1 Vendors (5 drivers)**:
- Checkpoint: Gaia (MGMT API)
- MikroTik: RouterOS (REST API)
- Ubiquiti: EdgeOS/UniFi
- Extreme: EXOS/VOSS
- Brocade: FabricOS

**P2 Vendors (6 drivers)**:
- Nokia: SR OS
- Huawei: VRP
- Mellanox/NVIDIA: Onyx/Cumulus
- Allied Telesis: AlliedWare Plus
- Ciena: SAOS

### D6: Universal Credential Rotation

Credential rotation for all protocols and vendors.

**Features**:
- Protocol-aware rotation (SSH keys, SNMP communities, API tokens)
- Vendor-specific procedures
- Pre/post rotation validation
- Rollback on failure
- Rotation scheduling
- Audit logging

**Rotation Configuration**:
```yaml
credential_rotation:
  enabled: true
  schedule: "0 0 * * 0"  # Weekly

  policies:
    - match:
        protocol: ssh
      rotate:
        type: key
        key_type: ed25519
        key_size: 256

    - match:
        protocol: snmp
        version: v3
      rotate:
        type: user
        auth_protocol: sha256
        priv_protocol: aes256

    - match:
        vendor: fortinet
      rotate:
        type: api_token
        token_lifetime: 30d
```

### D7: Protocol-Specific State Modules

State modules leveraging advanced protocols.

**NETCONF State Modules**:
- `netconf_interface` - Interface configuration via NETCONF
- `netconf_vlan` - VLAN configuration
- `netconf_routing` - Routing configuration
- `netconf_acl` - ACL configuration
- `netconf_raw` - Raw NETCONF operations

**Vendor State Modules**:
- `fortios_policy` - FortiOS firewall policies
- `panos_rule` - PAN-OS security rules
- `bigip_pool` - F5 pool management
- `bigip_virtual` - F5 virtual server management
- `checkpoint_rule` - Checkpoint firewall rules

### D8: Vendor Compatibility Matrix

Comprehensive compatibility documentation.

**Matrix Contents**:
- Supported firmware versions
- Protocol availability per version
- Feature support per protocol
- Known limitations
- Recommended configurations
- Test coverage status

### D9: Documentation

Complete protocol and vendor documentation.

**Contents**:
- Protocol reference guides
- Vendor setup guides
- Credential rotation guide
- State module reference
- Troubleshooting guide
- Migration guide (from other tools)

## Acceptance Criteria

### AC1: NETCONF Functional
- [ ] RFC 6241 compliance tests pass
- [ ] All operations implemented
- [ ] Transaction support working
- [ ] YANG models loaded
- [ ] Works with Juniper, Cisco, Nokia

### AC2: RESTCONF Functional
- [ ] RFC 8040 compliance tests pass
- [ ] JSON and XML encoding working
- [ ] Query parameters functional
- [ ] Works with Cisco IOS-XE, Arista

### AC3: Telnet Functional
- [ ] Legacy device connectivity working
- [ ] Security controls enforced
- [ ] Expect matching accurate
- [ ] Audit logging complete

### AC4: gNMI Functional
- [ ] All RPCs implemented
- [ ] Streaming subscriptions working
- [ ] OpenConfig paths supported
- [ ] Works with Arista, Cisco, Nokia

### AC5: Vendor Coverage
- [ ] P0 vendors (5) fully supported
- [ ] P1 vendors (4) fully supported
- [ ] P2 vendors (6) basic support
- [ ] Compatibility matrix complete

### AC6: Credential Rotation
- [ ] All protocols support rotation
- [ ] Scheduling functional
- [ ] Rollback on failure works
- [ ] Audit trail complete

### AC7: State Modules
- [ ] NETCONF state modules working
- [ ] Vendor-specific modules working
- [ ] Idempotency verified
- [ ] Documentation complete

## Sub-Issues / Tasks

### Phase 1: NETCONF Implementation (Weeks 1-6)

#### T1.1: NETCONF Core
Implement NETCONF protocol core.

**Deliverables**:
- Session management
- RPC encoding/decoding
- Capability exchange
- Transport layer (SSH)

#### T1.2: NETCONF Operations
Implement all NETCONF operations.

**Deliverables**:
- get, get-config
- edit-config, copy-config, delete-config
- lock, unlock
- commit, discard-changes, validate

#### T1.3: YANG Support
Implement YANG model handling.

**Deliverables**:
- YANG parser integration
- Schema validation
- XPath filtering
- Model discovery

#### T1.4: NETCONF Testing
Test with real devices.

**Deliverables**:
- Juniper JUNOS tests
- Cisco IOS-XE tests
- Nokia SR OS tests

### Phase 2: RESTCONF Implementation (Weeks 7-10)

#### T2.1: RESTCONF Core
Implement RESTCONF protocol.

**Deliverables**:
- HTTP client wrapper
- YANG-JSON mapping
- Path construction
- Error handling

#### T2.2: RESTCONF Operations
Implement CRUD operations.

**Deliverables**:
- GET (read)
- POST (create)
- PUT (replace)
- PATCH (update)
- DELETE (remove)

#### T2.3: RESTCONF Advanced
Implement advanced features.

**Deliverables**:
- Query parameters
- Event notifications (SSE)
- Schema discovery

### Phase 3: Telnet Implementation (Weeks 11-13)

#### T3.1: Telnet Core
Implement Telnet adapter.

**Deliverables**:
- Connection management
- Expect-style matching
- Prompt detection
- Command execution

#### T3.2: Telnet Security
Implement security controls.

**Deliverables**:
- IP allowlisting
- Audit logging
- Session limits
- Deprecation warnings

### Phase 4: gNMI Implementation (Weeks 14-18)

#### T4.1: gNMI Client
Implement gNMI gRPC client.

**Deliverables**:
- gRPC connection
- TLS/mTLS support
- Capabilities RPC
- Path encoding

#### T4.2: gNMI Operations
Implement Get and Set.

**Deliverables**:
- Get RPC
- Set RPC (update, replace, delete)
- Response handling

#### T4.3: gNMI Subscribe
Implement streaming subscriptions.

**Deliverables**:
- STREAM mode
- ONCE mode
- POLL mode
- Reconnection handling

#### T4.4: gNOI Operations
Implement gNOI system operations.

**Deliverables**:
- System reboot
- Ping/traceroute
- Certificate management

### Phase 5: P0 Vendor Drivers (Weeks 19-24)

#### T5.1: HP/Aruba Drivers
Implement HP/Aruba platform drivers.

**Deliverables**:
- ProCurve driver
- ArubaOS driver
- AOS-CX driver

#### T5.2: Dell Drivers
Implement Dell platform drivers.

**Deliverables**:
- OS10 driver
- OS9 driver
- PowerSwitch driver

#### T5.3: Security Vendor Drivers
Implement firewall vendor drivers.

**Deliverables**:
- FortiOS driver
- PAN-OS driver
- F5 BIG-IP driver

### Phase 6: P1/P2 Vendor Drivers (Weeks 25-30)

#### T6.1: P1 Vendors
Implement priority 1 vendors.

**Deliverables**:
- Checkpoint Gaia
- MikroTik RouterOS
- Ubiquiti EdgeOS
- Extreme EXOS

#### T6.2: P2 Vendors
Implement priority 2 vendors.

**Deliverables**:
- Nokia SR OS
- Huawei VRP
- Mellanox/NVIDIA
- Allied Telesis
- Ciena SAOS

### Phase 7: Credential Rotation (Weeks 31-34)

#### T7.1: Rotation Framework
Implement universal rotation framework.

**Deliverables**:
- Protocol adapters
- Rotation procedures
- Validation framework
- Rollback mechanism

#### T7.2: Protocol Rotation
Implement per-protocol rotation.

**Deliverables**:
- SSH key rotation
- SNMP credential rotation
- API token rotation
- Certificate rotation

#### T7.3: Rotation Scheduling
Implement automated scheduling.

**Deliverables**:
- Cron-based scheduling
- Policy engine
- Notification system
- Audit logging

### Phase 8: State Modules and Documentation (Weeks 35-40)

#### T8.1: NETCONF State Modules
Implement NETCONF-based state modules.

**Deliverables**:
- netconf_interface
- netconf_vlan
- netconf_routing
- netconf_acl

#### T8.2: Vendor State Modules
Implement vendor-specific modules.

**Deliverables**:
- fortios_policy
- panos_rule
- bigip_pool/virtual
- checkpoint_rule

#### T8.3: Documentation
Complete all documentation.

**Deliverables**:
- Protocol reference
- Vendor guides
- Compatibility matrix
- Troubleshooting guide

## Dependencies

- **Epic 21** (Proxy Agents): Core proxy agent infrastructure
- **Epic 17** (SPIFFE): Certificate-based authentication
- **External**: YANG libraries, gRPC, vendor APIs

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Vendor API changes | Driver breakage | Version detection; abstract common operations |
| Device access for testing | Incomplete coverage | Virtual devices; vendor partnerships |
| Protocol complexity | Implementation delays | Prioritize common operations; incremental delivery |
| Credential rotation failures | Security exposure | Extensive testing; automatic rollback |
| gNMI adoption variance | Limited usefulness | Focus on OpenConfig; vendor-specific fallbacks |

## Success Metrics

| Metric | Target |
|--------|--------|
| Protocols supported | 8 (SSH, SNMP, REST, WinRM, NETCONF, RESTCONF, Telnet, gNMI) |
| Vendors supported | 15+ |
| State modules | 20+ |
| Test coverage | >80% |
| Credential rotation coverage | 100% of protocols |
| Documentation pages | 30+ |

## Definition of Done

- [ ] All deliverables (D1-D9) implemented
- [ ] All acceptance criteria (AC1-AC7) met
- [ ] NETCONF adapter RFC 6241 compliant
- [ ] RESTCONF adapter RFC 8040 compliant
- [ ] gNMI streaming working
- [ ] 15+ vendors supported
- [ ] Universal credential rotation working
- [ ] State modules documented and tested
- [ ] Compatibility matrix published

## Future Considerations

- YANG model repository and management
- Network topology discovery via protocols
- Configuration drift detection using streaming
- Multi-vendor configuration templates
- Network automation playbooks
- Integration with network inventory systems
- Support for SD-WAN platforms
- IoT device protocols (MQTT, CoAP)
