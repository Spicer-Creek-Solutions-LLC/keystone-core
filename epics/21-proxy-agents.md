# Epic 21: Proxy Agents

## Overview

Implement proxy agent support to enable Keystone Core to manage devices that cannot run the native agent software. This includes legacy systems, network hardware (switches, routers, firewalls), IoT devices, embedded systems, and appliances with locked-down operating systems.

**Goal**: Allow any device that supports remote management protocols (SSH, SNMP, REST API, WinRM, vendor CLIs) to be managed as a first-class Keystone Core agent, with transparent targeting, state management, and observability.

## Success Criteria

- [ ] Proxy agents can register and manage multiple proxied devices
- [ ] Proxied devices appear as virtual agents in the control plane
- [ ] Transparent targeting works identically for native and proxied agents
- [ ] SSH protocol adapter supports Linux/Unix legacy systems
- [ ] SNMP protocol adapter supports network devices (v2c and v3)
- [ ] REST/HTTP protocol adapter supports modern appliances
- [ ] WinRM protocol adapter supports legacy Windows systems
- [ ] Vendor-specific adapters for Cisco IOS, Juniper JUNOS, Arista EOS
- [ ] Open-source firewall adapters for pfSense and OPNsense (REST API)
- [ ] Ubiquiti/VyOS adapter for EdgeRouter and VyOS devices
- [ ] State modules work on proxied devices (file, service, package where applicable)
- [ ] Health monitoring and status tracking for all proxied devices
- [ ] Secure credential storage with rotation support
- [ ] Support for 100+ proxied devices per proxy agent
- [ ] Latency < 500ms for command execution via proxy
- [ ] Comprehensive audit logging for all proxied operations

## Architecture

```mermaid
flowchart TD
    subgraph CP["Control Plane"]
        API["API Server"]
        CM["Connection Manager"]
        TM["Target Matcher"]
        CProxy["Credential Proxy"]
        Vault["Vault / K8s Secrets"]

        CProxy --> Vault
    end

    subgraph PA["Proxy Agent"]
        PE["Proxy Engine"]
        DR["Device Registry"]
        CC["Credential Cache<br/>(Memory Only)"]

        subgraph Adapters["Protocol Adapters"]
            SSH["SSH Adapter"]
            SNMP["SNMP Adapter"]
            REST["REST Adapter"]
            WinRM["WinRM Adapter"]
            Vendor["Vendor Adapters<br/>(Cisco, Juniper, Arista)"]
            OpenFW["Open Firewall Adapters<br/>(pfSense, OPNsense)"]
            VyOS["VyOS/EdgeOS Adapter"]
        end

        PE --> DR
        PE --> CC
        PE --> Adapters
    end

    subgraph Devices["Proxied Devices"]
        Legacy["Legacy Linux<br/>(SSH)"]
        Network["Network Switch<br/>(SNMP/CLI)"]
        Appliance["Appliance<br/>(REST API)"]
        OldWin["Legacy Windows<br/>(WinRM)"]
        IoT["IoT Device<br/>(MQTT/HTTP)"]
        Firewall["pfSense/OPNsense<br/>(REST API)"]
        EdgeRouter["EdgeRouter/VyOS<br/>(HTTP API)"]
    end

    NATS["NATS (mTLS)"]

    CP <--> NATS
    NATS <--> PA
    CProxy <-.->|"Credential<br/>Request/Response"| NATS

    SSH --> Legacy
    SNMP --> Network
    Vendor --> Network
    REST --> Appliance
    WinRM --> OldWin
    REST --> IoT
    OpenFW --> Firewall
    VyOS --> EdgeRouter
```

### Credential Flow (NATS-Proxied)

```mermaid
sequenceDiagram
    participant PA as Proxy Agent
    participant NATS as NATS (mTLS)
    participant CP as Control Plane
    participant Vault as Vault/Secrets

    PA->>NATS: 1. Credential Request<br/>(device_id, credential_ref)
    NATS->>CP: Forward request

    CP->>CP: 2. Validate proxy agent<br/>authorized for device
    CP->>CP: 3. Check rate limits
    CP->>Vault: 4. Fetch credential
    Vault-->>CP: Return credential

    CP->>CP: 5. Encrypt with proxy<br/>agent's public key
    CP->>CP: 6. Audit log

    CP->>NATS: 7. Credential Response<br/>(encrypted, TTL)
    NATS->>PA: Forward response

    PA->>PA: 8. Decrypt & cache<br/>(memory only, TTL)
    PA->>PA: 9. Use credential<br/>for device connection
```

## Concepts

### Proxy Agent
A standard Keystone Core agent with additional proxy capabilities enabled. It acts as an intermediary between the control plane and devices that cannot run native agents.

**Important**: A proxy agent is ALSO a normal agent. It:
- Runs the full `kscore-agent` binary on a host that supports it (Linux/Windows/macOS)
- Reports its own heartbeat, facts, and health status
- Can receive commands targeted at itself (the proxy host)
- Has its own agent ID, labels, and metadata
- Manages one or more proxied devices

**Control Plane View**:
```
Agents:
├── proxy-dc1                  ← Proxy agent (also a normal agent)
│   ├── proxy-dc1/switch-01    ← Virtual agent (proxied device)
│   ├── proxy-dc1/switch-02    ← Virtual agent (proxied device)
│   └── proxy-dc1/legacy-db    ← Virtual agent (proxied device)
├── agent-web-01               ← Normal agent
└── agent-web-02               ← Normal agent
```

### Proxied Device (Virtual Agent)
A device managed through a proxy agent. Each proxied device:
- Has a unique agent ID (prefixed with proxy agent ID, e.g., `proxy-dc1/switch-01`)
- Appears in the control plane as a regular agent
- Can be targeted using standard expressions
- Reports metadata and health status (via the proxy agent)

### Protocol Adapter
A pluggable component that translates Keystone Core commands into device-specific protocols:
- **SSH**: Execute shell commands over SSH
- **SNMP**: Get/Set OIDs via SNMP v2c/v3
- **REST**: HTTP requests to device APIs
- **WinRM**: PowerShell remoting for Windows
- **Vendor**: Device-specific CLI adapters (Cisco, Juniper, etc.)

### Device Profile
A template that defines how to interact with a specific device type:
- Protocol and connection settings
- Command translation rules
- State module mappings
- Health check definitions
- Fact collection methods

## User Stories

### US21.1: Proxy Agent Setup
**As a** platform operator
**I want to** configure an agent as a proxy for other devices
**So that** I can manage devices that cannot run native agents

**Acceptance Criteria**:
- Enable proxy mode via configuration flag
- Define proxied devices in configuration file
- Support dynamic device registration via API
- Proxy agent reports its own health plus proxied device status
- Clear separation between proxy agent and proxied device identities

**Configuration Example**:
```yaml
# kscore-agent.yaml
agent:
  id: proxy-datacenter-1
  proxy:
    enabled: true
    max_devices: 100
    health_check_interval: 30s

    # Credential store configuration
    credentials:
      store: vault  # or: file, env, k8s-secret
      vault:
        address: https://vault.example.com
        path: secret/kscore/devices

    # Device definitions
    devices:
      - id: switch-core-01
        name: "Core Switch 01"
        profile: cisco-ios
        address: 192.168.1.1
        credential_ref: switch-core-01-creds
        labels:
          role: network
          location: datacenter-1
          device_type: switch
          vendor: cisco

      - id: legacy-db-01
        name: "Legacy Database Server"
        profile: linux-ssh
        address: 192.168.1.50
        credential_ref: legacy-db-creds
        labels:
          role: database
          os: rhel5
          legacy: "true"
```

### US21.2: Device Discovery
**As a** platform operator
**I want to** automatically discover devices on the network
**So that** I don't have to manually configure each device

**Acceptance Criteria**:
- Support network scanning for device discovery
- Auto-detect device type from responses (SSH banner, SNMP sysDescr, HTTP headers)
- Suggest appropriate device profile based on detection
- Support discovery via LLDP/CDP for network devices
- Support discovery via DNS service records
- Configurable discovery scopes (subnets, VLANs)

**Discovery Configuration**:
```yaml
proxy:
  discovery:
    enabled: true
    interval: 1h
    scopes:
      - type: subnet
        range: 192.168.1.0/24
        protocols: [ssh, snmp]
      - type: subnet
        range: 10.0.0.0/16
        protocols: [snmp]
        snmp_community: public
    auto_register: false  # Require approval

    # Device type detection rules
    detection_rules:
      - match:
          snmp_sysdescr: "Cisco IOS*"
        profile: cisco-ios
      - match:
          snmp_sysdescr: "Juniper*"
        profile: juniper-junos
      - match:
          ssh_banner: "*Ubuntu*"
        profile: linux-ssh
```

### US21.3: SSH Protocol Adapter
**As a** platform operator
**I want to** manage legacy Linux/Unix systems via SSH
**So that** I can include old systems in my automation

**Acceptance Criteria**:
- Support SSH password and key-based authentication
- Support jump hosts / bastion servers
- Execute commands and capture stdout/stderr/exit code
- Support interactive command sequences (expect-like)
- File transfer via SFTP/SCP
- Connection pooling for performance
- Timeout and retry configuration
- Support for non-standard SSH ports

**SSH Profile Example**:
```yaml
# Device profile: linux-ssh
profiles:
  linux-ssh:
    protocol: ssh
    settings:
      port: 22
      timeout: 30s
      keepalive: 60s
      max_connections: 5

    # Command execution
    execution:
      shell: /bin/bash
      sudo: true
      sudo_password_ref: sudo_pass

    # Fact collection
    facts:
      os_family: "cat /etc/os-release | grep ^ID= | cut -d= -f2"
      os_version: "cat /etc/os-release | grep ^VERSION_ID= | cut -d= -f2"
      hostname: "hostname -f"
      kernel: "uname -r"
      memory_mb: "free -m | awk '/Mem:/ {print $2}'"
      cpu_count: "nproc"

    # State module mappings
    modules:
      file:
        check: "test -f {path} && stat -c '%a %U %G %s' {path}"
        apply: "cat > {path} << 'EOF'\n{content}\nEOF"
      package:
        check: "rpm -q {name} 2>/dev/null || dpkg -l {name} 2>/dev/null"
        install: "yum install -y {name} || apt-get install -y {name}"
        remove: "yum remove -y {name} || apt-get remove -y {name}"
      service:
        check: "systemctl is-active {name} || service {name} status"
        start: "systemctl start {name} || service {name} start"
        stop: "systemctl stop {name} || service {name} stop"

    # Health check
    health:
      command: "echo ok"
      interval: 60s
      timeout: 10s
```

### US21.4: SNMP Protocol Adapter
**As a** network engineer
**I want to** manage network devices via SNMP
**So that** I can automate network configuration and monitoring

**Acceptance Criteria**:
- Support SNMP v2c and v3
- Get/Set/Walk operations
- Trap/Inform receiving
- MIB translation (human-readable OID names)
- Bulk operations for efficiency
- Support for vendor-specific MIBs
- SNMP table manipulation

**SNMP Profile Example**:
```yaml
profiles:
  snmp-v3-device:
    protocol: snmp
    settings:
      version: v3
      port: 161
      timeout: 10s
      retries: 3

      # SNMPv3 security
      security_level: authPriv
      auth_protocol: SHA256
      priv_protocol: AES256

    # Fact collection via SNMP
    facts:
      sys_name: "SNMPv2-MIB::sysName.0"
      sys_descr: "SNMPv2-MIB::sysDescr.0"
      sys_uptime: "SNMPv2-MIB::sysUpTime.0"
      sys_location: "SNMPv2-MIB::sysLocation.0"
      if_count: "IF-MIB::ifNumber.0"

    # State module mappings
    modules:
      snmp_value:
        check: "get {oid}"
        apply: "set {oid} {type} {value}"

    # Health check
    health:
      oid: "SNMPv2-MIB::sysUpTime.0"
      interval: 30s
      timeout: 5s
```

### US21.5: REST/HTTP Protocol Adapter
**As a** platform operator
**I want to** manage devices with REST APIs
**So that** I can automate modern appliances and cloud devices

**Acceptance Criteria**:
- Support all HTTP methods (GET, POST, PUT, PATCH, DELETE)
- Authentication: Basic, Bearer token, API key, OAuth2
- Request/response transformation (JSON, XML, form data)
- Response parsing with JSONPath/XPath
- Pagination handling
- Rate limiting support
- TLS certificate verification options
- Webhook support for event notifications

**REST Profile Example**:
```yaml
profiles:
  rest-appliance:
    protocol: http
    settings:
      base_url: "https://{address}:443/api/v1"
      timeout: 30s
      tls_verify: true

      # Authentication
      auth:
        type: bearer
        token_ref: api_token
        # Or OAuth2:
        # type: oauth2
        # token_url: /oauth/token
        # client_id_ref: client_id
        # client_secret_ref: client_secret

      # Default headers
      headers:
        Accept: application/json
        Content-Type: application/json

    # Fact collection via API
    facts:
      version:
        request: GET /system/info
        extract: $.version
      hostname:
        request: GET /system/info
        extract: $.hostname
      status:
        request: GET /system/health
        extract: $.status

    # State module mappings (config management)
    modules:
      config_item:
        check:
          request: GET /config/{path}
          extract: $.value
        apply:
          request: PUT /config/{path}
          body: '{"value": "{value}"}'

    # Health check
    health:
      request: GET /health
      expect_status: 200
      expect_body: '$.status == "healthy"'
      interval: 30s
```

### US21.6: WinRM Protocol Adapter
**As a** platform operator
**I want to** manage legacy Windows systems via WinRM
**So that** I can automate Windows servers that cannot run modern agents

**Acceptance Criteria**:
- Support WinRM over HTTP and HTTPS
- Support NTLM and Kerberos authentication
- Execute PowerShell commands
- Execute cmd.exe commands
- File transfer support
- Registry operations
- Service management
- Support for Windows Server 2008+

**WinRM Profile Example**:
```yaml
profiles:
  windows-winrm:
    protocol: winrm
    settings:
      port: 5986  # HTTPS
      transport: https
      timeout: 60s

      # Authentication
      auth:
        type: ntlm  # or: kerberos, basic

    # Fact collection
    facts:
      os_name: "(Get-WmiObject Win32_OperatingSystem).Caption"
      os_version: "(Get-WmiObject Win32_OperatingSystem).Version"
      hostname: "$env:COMPUTERNAME"
      domain: "(Get-WmiObject Win32_ComputerSystem).Domain"
      memory_gb: "[math]::Round((Get-WmiObject Win32_ComputerSystem).TotalPhysicalMemory/1GB,2)"
      cpu_count: "(Get-WmiObject Win32_Processor).NumberOfCores"

    # State module mappings
    modules:
      file:
        check: "if (Test-Path '{path}') { Get-Item '{path}' | Select Mode,Length }"
        apply: "Set-Content -Path '{path}' -Value @'\n{content}\n'@"
      service:
        check: "(Get-Service '{name}').Status"
        start: "Start-Service '{name}'"
        stop: "Stop-Service '{name}'"
      package:
        check: "Get-Package -Name '{name}' -ErrorAction SilentlyContinue"
        install: "choco install {name} -y"

    # Health check
    health:
      command: "Write-Output 'ok'"
      interval: 60s
      timeout: 15s
```

### US21.7: Vendor-Specific Network Adapters
**As a** network engineer
**I want to** manage network devices using vendor-specific CLI commands
**So that** I can perform advanced configuration not possible via SNMP

**Acceptance Criteria**:
- Cisco IOS/IOS-XE adapter
- Cisco NX-OS adapter
- Juniper JUNOS adapter
- Arista EOS adapter
- Palo Alto PAN-OS adapter
- Fortinet FortiOS adapter
- **pfSense adapter** (REST API)
- **OPNsense adapter** (REST API)
- **VyOS/Ubiquiti EdgeOS adapter** (HTTP API + SSH)
- Support for enable/configure modes
- Support for commit/rollback where applicable
- Configuration diff/backup support
- Support for structured output (JSON, XML) where available

**Cisco IOS Profile Example**:
```yaml
profiles:
  cisco-ios:
    protocol: ssh
    settings:
      port: 22
      timeout: 30s

    # CLI interaction settings
    cli:
      prompt_patterns:
        user: "Username:"
        password: "Password:"
        exec: "^[\\w\\-]+>"
        privileged: "^[\\w\\-]+#"
        config: "^[\\w\\-]+\\(config.*\\)#"

      # Mode transitions
      modes:
        exec:
          enter: ""
          exit: "exit"
        privileged:
          enter: "enable"
          password_ref: enable_secret
          exit: "disable"
        config:
          enter: "configure terminal"
          exit: "end"

    # Fact collection
    facts:
      version: "show version | include IOS"
      hostname: "show running-config | include ^hostname"
      model: "show version | include ^[Cc]isco"
      serial: "show version | include Processor board ID"
      uptime: "show version | include uptime"
      interfaces: "show ip interface brief"

    # State module mappings
    modules:
      interface:
        check: "show running-config interface {name}"
        apply:
          mode: config
          commands:
            - "interface {name}"
            - "{config}"
      vlan:
        check: "show vlan id {id}"
        apply:
          mode: config
          commands:
            - "vlan {id}"
            - "name {name}"
      acl:
        check: "show access-lists {name}"
        apply:
          mode: config
          commands:
            - "ip access-list {type} {name}"
            - "{rules}"

    # Health check
    health:
      command: "show clock"
      interval: 60s
      timeout: 10s

    # Configuration management
    config:
      backup: "show running-config"
      diff: "show archive config differences"
      save: "copy running-config startup-config"
```

**OPNsense Profile Example**:
```yaml
profiles:
  opnsense:
    protocol: http
    settings:
      base_url: "https://{address}/api"
      timeout: 30s
      tls_verify: true

      # API Key authentication
      auth:
        type: basic
        username_ref: api_key
        password_ref: api_secret

      headers:
        Accept: application/json
        Content-Type: application/json

    # Fact collection via API
    facts:
      version:
        request: GET /core/firmware/status
        extract: $.product_version
      hostname:
        request: GET /core/system/status
        extract: $.name
      kernel:
        request: GET /core/system/status
        extract: $.kernel

    # State module mappings
    modules:
      alias:
        check:
          request: GET /firewall/alias/getItem/{uuid}
        apply:
          request: POST /firewall/alias/addItem
          body: '{"alias": {"name": "{name}", "type": "{type}", "content": "{content}"}}'
        delete:
          request: POST /firewall/alias/delItem/{uuid}
        reconfigure:
          request: POST /firewall/alias/reconfigure

      rule:
        check:
          request: GET /firewall/filter/getRule/{uuid}
        apply:
          request: POST /firewall/filter/addRule
        delete:
          request: POST /firewall/filter/delRule/{uuid}

      service:
        check:
          request: GET /core/service/{name}/status
        restart:
          request: POST /core/service/{name}/restart
        start:
          request: POST /core/service/{name}/start
        stop:
          request: POST /core/service/{name}/stop

    # Health check
    health:
      request: GET /core/system/status
      expect_status: 200
      interval: 30s
```

**pfSense Profile Example**:
```yaml
profiles:
  pfsense:
    protocol: http
    settings:
      base_url: "https://{address}/api/v1"
      timeout: 30s
      tls_verify: true

      # Bearer token authentication (pfSense Plus / REST API package)
      auth:
        type: bearer
        token_ref: api_token
        # Or client credentials:
        # type: jwt
        # client_id_ref: client_id
        # client_token_ref: client_token

      headers:
        Accept: application/json
        Content-Type: application/json

    # Fact collection via API
    facts:
      version:
        request: GET /status/system
        extract: $.data.system_version
      hostname:
        request: GET /status/system
        extract: $.data.hostname
      platform:
        request: GET /status/system
        extract: $.data.system_platform

    # State module mappings
    modules:
      alias:
        check:
          request: GET /firewall/alias
          filter: '$.data[?(@.name == "{name}")]'
        apply:
          request: POST /firewall/alias
          body: '{"name": "{name}", "type": "{type}", "address": {addresses}}'
        update:
          request: PUT /firewall/alias
          body: '{"id": {id}, "name": "{name}", "type": "{type}", "address": {addresses}}'
        delete:
          request: DELETE /firewall/alias/{id}

      rule:
        check:
          request: GET /firewall/rule
        apply:
          request: POST /firewall/rule
        delete:
          request: DELETE /firewall/rule/{tracker}

      interface:
        check:
          request: GET /status/interface
        apply:
          request: PUT /interface

      apply:
        # Apply pending changes
        request: POST /firewall/apply

    # Health check
    health:
      request: GET /status/system
      expect_status: 200
      expect_body: '$.code == 200'
      interval: 30s
```

**VyOS/EdgeOS Profile Example**:
```yaml
profiles:
  vyos:
    protocol: http
    settings:
      base_url: "https://{address}"
      timeout: 30s
      tls_verify: true

      # VyOS HTTP API uses API key
      auth:
        type: api_key
        key_name: key
        key_ref: api_key
        location: body  # VyOS expects key in POST body

      headers:
        Accept: application/json
        Content-Type: application/json

    # Alternative: SSH-based for EdgeOS (Vyatta-derived)
    # protocol: ssh
    # settings:
    #   port: 22
    #   shell: /bin/vbash

    # Fact collection via API
    facts:
      version:
        request: POST /retrieve
        body: '{"op": "showConfig", "path": ["system", "host-name"]}'
        extract: $.data
      hostname:
        request: POST /show
        body: '{"op": "show", "path": ["host", "name"]}'
        extract: $.data
      interfaces:
        request: POST /show
        body: '{"op": "show", "path": ["interfaces"]}'

    # State module mappings
    modules:
      interface:
        check:
          request: POST /retrieve
          body: '{"op": "showConfig", "path": ["interfaces", "{type}", "{name}"]}'
        apply:
          request: POST /configure
          body: |
            {"op": "set", "path": ["interfaces", "{type}", "{name}", "address", "{address}"]}
        delete:
          request: POST /configure
          body: '{"op": "delete", "path": ["interfaces", "{type}", "{name}"]}'

      static_route:
        check:
          request: POST /retrieve
          body: '{"op": "showConfig", "path": ["protocols", "static", "route", "{destination}"]}'
        apply:
          request: POST /configure
          body: '{"op": "set", "path": ["protocols", "static", "route", "{destination}", "next-hop", "{gateway}"]}'
        delete:
          request: POST /configure
          body: '{"op": "delete", "path": ["protocols", "static", "route", "{destination}"]}'

      firewall_rule:
        check:
          request: POST /retrieve
          body: '{"op": "showConfig", "path": ["firewall", "name", "{ruleset}", "rule", "{number}"]}'
        apply:
          request: POST /configure
          body: '{"op": "set", "path": ["firewall", "name", "{ruleset}", "rule", "{number}", "action", "{action}"]}'

      zone:
        check:
          request: POST /retrieve
          body: '{"op": "showConfig", "path": ["zone-policy", "zone", "{name}"]}'
        apply:
          request: POST /configure
          body: '{"op": "set", "path": ["zone-policy", "zone", "{name}", "interface", "{interface}"]}'

    # Configuration management
    config:
      save:
        request: POST /config-file
        body: '{"op": "save"}'
      load:
        request: POST /config-file
        body: '{"op": "load", "file": "{path}"}'

    # Health check
    health:
      request: POST /show
      body: '{"op": "show", "path": ["version"]}'
      expect_status: 200
      interval: 60s
```

### US21.8: Credential Management
**As a** security engineer
**I want to** securely manage credentials for proxied devices
**So that** sensitive authentication data is protected

**Acceptance Criteria**:
- **NATS-proxied credentials by default** (proxy agents don't access Vault directly)
- Control plane acts as credential proxy with policy enforcement
- Credentials encrypted end-to-end (control plane → proxy agent)
- Short TTL caching at proxy agent (memory only, never persisted)
- Direct access mode as opt-in for special cases
- Credential rotation support without proxy agent restart
- Credential scoping (per-device, per-profile, per-group)
- Comprehensive audit logging of all credential access
- Support for SSH keys, passwords, tokens, certificates
- Rate limiting to prevent credential enumeration

**Why NATS-Proxied (Default)**:
- Proxy agents often run in DMZs or isolated networks without Vault access
- Centralized audit logging at control plane
- Additional policy enforcement before credential issuance
- Consistent with NATS-first architecture (Epic 14)
- Credentials never stored on proxy agent disk

**Credential Configuration**:
```yaml
proxy:
  credentials:
    # Mode: "nats" (default) | "direct" | "hybrid"
    mode: nats

    # Local cache settings (memory only, never persisted to disk)
    cache:
      enabled: true
      ttl: 5m                    # How long to cache credentials
      refresh_before: 1m         # Refresh before expiry
      max_entries: 1000          # Maximum cached credentials

    # Device credential references (resolved via control plane)
    devices:
      switch-core-01:
        credential_ref: network/switches/core-01
      legacy-db-01:
        credential_ref: servers/legacy-db

# For direct mode (NOT recommended for most deployments)
# proxy:
#   credentials:
#     mode: direct
#     direct:
#       vault:
#         address: https://vault.example.com
#         auth:
#           type: kubernetes
#           role: kscore-proxy
#         path_prefix: secret/data/kscore
```

**Control Plane Credential Store Configuration**:
```yaml
# kscore-server.yaml (control plane configuration)
credential_proxy:
  enabled: true

  # Backend credential stores (control plane has direct access)
  stores:
    - name: vault
      type: vault
      vault:
        address: https://vault.example.com
        auth:
          type: kubernetes
          role: kscore-server
        path_prefix: secret/data/kscore

    - name: k8s
      type: kubernetes
      kubernetes:
        namespace: kscore-secrets

  # Policy enforcement
  policy:
    # Rate limiting per proxy agent
    rate_limit:
      requests_per_minute: 100
      burst: 20

    # Require proxy agent to be assigned to device
    require_device_assignment: true

    # Maximum credential TTL (proxy agent can request shorter)
    max_ttl: 10m

  # Audit logging
  audit:
    enabled: true
    include_credential_ref: true
    include_device_id: true
    exclude_credential_value: true  # Never log actual credentials
```

### US21.9: Transparent Targeting
**As a** platform operator
**I want to** target proxied devices using the same syntax as native agents
**So that** my automation doesn't need to distinguish between agent types

**Acceptance Criteria**:
- Proxied devices use standard targeting expressions
- Labels and metadata work identically
- Compound targeting works across native and proxied agents
- Glob patterns work on proxied device IDs
- Group targeting includes proxied devices

**Targeting Examples**:
```bash
# Target all devices (native + proxied)
kscorectl exec "show version" --target "role:network"

# Target specific proxied device
kscorectl exec "show interfaces" --target "agent:proxy-dc1/switch-core-01"

# Target by device type label
kscorectl exec "show vlan" --target "device_type:switch AND vendor:cisco"

# Target legacy systems
kscorectl exec "uptime" --target "legacy:true"

# Mixed targeting (native + proxied)
kscorectl state apply webserver --target "role:web"
# Automatically applies via proxy for devices without native agents

# Compound targeting
kscorectl exec "hostname" --target "(role:database OR role:network) AND datacenter:dc1"
```

### US21.10: Proxied Device State Management
**As a** platform operator
**I want to** apply state configurations to proxied devices
**So that** I can maintain consistent configuration across all infrastructure

**Acceptance Criteria**:
- State modules work through protocol adapters
- Profile defines module implementation for each device type
- State results reported identically to native agents
- Drift detection works for proxied devices
- Requisites work across native and proxied devices

**State Example**:
```yaml
# state/network-baseline.yaml
states:
  - name: ntp-servers
    module: ios_config
    target: "device_type:switch AND vendor:cisco"
    config: |
      ntp server 10.0.0.1 prefer
      ntp server 10.0.0.2

  - name: snmp-community
    module: snmp_value
    target: "protocol:snmp"
    oid: "SNMPv2-MIB::sysContact.0"
    value: "noc@example.com"
    type: string

  - name: ssh-banner
    module: file
    target: "os:linux AND legacy:true"
    path: /etc/motd
    content: |
      *** Authorized Access Only ***
      This system is managed by Keystone Core
```

### US21.11: Health Monitoring
**As a** platform operator
**I want to** monitor the health of all proxied devices
**So that** I can detect and respond to issues quickly

**Acceptance Criteria**:
- Configurable health check interval per device
- Health status: healthy, degraded, unhealthy, unknown
- Automatic retry on failure
- Health history tracking
- Events emitted on status change
- Metrics exported to Prometheus
- Integration with alerting systems

**Health Check Configuration**:
```yaml
proxy:
  health:
    default_interval: 60s
    default_timeout: 10s
    retry_count: 3
    retry_delay: 5s

    # Device-specific overrides
    devices:
      switch-core-01:
        interval: 30s  # More frequent for critical devices
        timeout: 5s
        checks:
          - name: ping
            type: icmp
          - name: snmp
            type: snmp_get
            oid: "SNMPv2-MIB::sysUpTime.0"
          - name: ssh
            type: ssh_command
            command: "show clock"

    # Health status rules
    status_rules:
      healthy:
        - all_checks_pass: true
      degraded:
        - check_pass: ping
        - any_check_fail: [snmp, ssh]
      unhealthy:
        - check_fail: ping
```

### US21.12: Observability
**As a** SRE
**I want to** have full observability into proxy agent operations
**So that** I can troubleshoot issues and monitor performance

**Acceptance Criteria**:
- Metrics for proxy agent and each proxied device
- Distributed tracing through proxy operations
- Structured logging with correlation IDs
- Audit log for all device interactions
- Dashboard templates for Grafana

**Metrics**:
```
# Proxy agent metrics
kscore_proxy_devices_total{proxy="proxy-dc1"}
kscore_proxy_devices_healthy{proxy="proxy-dc1"}
kscore_proxy_devices_unhealthy{proxy="proxy-dc1"}

# Per-device metrics
kscore_proxy_device_commands_total{proxy="proxy-dc1",device="switch-core-01",status="success|failure"}
kscore_proxy_device_command_duration_seconds{proxy="proxy-dc1",device="switch-core-01",quantile="0.5|0.9|0.99"}
kscore_proxy_device_health_status{proxy="proxy-dc1",device="switch-core-01"} # 0=unknown,1=healthy,2=degraded,3=unhealthy

# Protocol adapter metrics
kscore_proxy_adapter_connections_total{proxy="proxy-dc1",protocol="ssh|snmp|http|winrm"}
kscore_proxy_adapter_connection_errors_total{proxy="proxy-dc1",protocol="ssh",error="auth|timeout|refused"}
kscore_proxy_adapter_connection_pool_size{proxy="proxy-dc1",protocol="ssh"}
```

## NATS Credential Protocol

The credential protocol enables proxy agents to securely fetch device credentials from the control plane via NATS, without requiring direct access to credential backends like Vault.

### Request Message

```yaml
# Subject: kscore.{cluster}.proxy.credentials.fetch
# Published by: Proxy Agent
message:
  request_id: "550e8400-e29b-41d4-a716-446655440000"
  proxy_agent_id: "proxy-dc1"
  device_id: "switch-core-01"
  credential_ref: "network/switches/core-01"
  credential_type: "ssh"  # ssh, snmp, http, winrm
  requested_ttl: 300      # Requested TTL in seconds (server may reduce)
  timestamp: "2024-01-15T10:00:00Z"
```

### Response Message

```yaml
# Subject: kscore.{cluster}.proxy.credentials.response.{request_id}
# Published by: Control Plane
message:
  request_id: "550e8400-e29b-41d4-a716-446655440000"
  status: "success"  # success, denied, not_found, rate_limited, error

  # Credential payload (encrypted with proxy agent's public key)
  credential:
    type: "ssh"
    encrypted_payload: "base64-encoded-encrypted-data"
    encryption_algorithm: "X25519-XSalsa20-Poly1305"

  # Metadata
  ttl: 300                           # Actual TTL granted
  expires_at: "2024-01-15T10:05:00Z"
  issued_at: "2024-01-15T10:00:00Z"

  # For denied/error responses
  error:
    code: "UNAUTHORIZED"
    message: "Proxy agent not authorized for device"
```

### Credential Payload (Decrypted)

```yaml
# After decryption by proxy agent
type: ssh
username: admin
auth_method: password  # password, key, key_with_passphrase
password: "secret123"

# Or for SSH key:
# auth_method: key
# private_key: "-----BEGIN OPENSSH PRIVATE KEY-----\n..."
# passphrase: "optional-passphrase"

# Or for SNMP v3:
# type: snmp
# version: v3
# username: snmpuser
# auth_protocol: SHA256
# auth_password: "authpass"
# priv_protocol: AES256
# priv_password: "privpass"

# Or for HTTP Bearer:
# type: http
# auth_type: bearer
# token: "eyJhbGciOiJIUzI1..."
```

### Security Properties

| Property | Implementation |
|----------|----------------|
| **Transport encryption** | NATS mTLS (TLS 1.3) |
| **Payload encryption** | X25519 key exchange + XSalsa20-Poly1305 |
| **Key management** | Each proxy agent has X25519 keypair, public key registered with control plane |
| **Forward secrecy** | New ephemeral key for each credential request |
| **Replay protection** | Request ID + timestamp + TTL |
| **Rate limiting** | Control plane enforces per-agent limits |

## Capacity Planning

### Devices per Proxy Agent

The number of devices a single proxy agent can manage depends on several factors:

| Protocol | Typical Capacity | Limiting Factor |
|----------|-----------------|-----------------|
| **SSH** | 200-500 devices | Connection overhead, pooling efficiency |
| **SNMP** | 500-1000 devices | UDP lightweight, minimal state |
| **REST/HTTP** | 300-500 devices | HTTP/2 multiplexing helps |
| **WinRM** | 50-100 devices | Heavy protocol, expensive connections |
| **Mixed** | 100-200 devices | Depends on protocol mix |

### Factors Affecting Capacity

1. **Health Check Frequency**: More devices = more concurrent health checks
   - Mitigation: Stagger checks, use longer intervals for stable devices

2. **Connection Pooling**: SSH/WinRM connections are expensive to establish
   - Mitigation: Pool and reuse connections, configure `max_connections` per device

3. **Command Concurrency**: Parallel commands to many devices
   - Mitigation: Configurable worker pool, queue depth limits

4. **Credential Cache**: Frequent credential requests add latency
   - Mitigation: Appropriate cache TTL, pre-warming

5. **Network Latency**: High latency = longer held connections
   - Mitigation: Async operations, appropriate timeouts

### Scaling Horizontally

For large environments, deploy multiple proxy agents with device sharding:

```yaml
# Proxy agent 1: Network devices
proxy:
  id: proxy-network-dc1
  devices:
    - id: switch-*
    - id: router-*
    - id: firewall-*

# Proxy agent 2: Legacy servers
proxy:
  id: proxy-legacy-dc1
  devices:
    - id: legacy-*
    - id: rhel5-*

# Proxy agent 3: Appliances
proxy:
  id: proxy-appliances-dc1
  devices:
    - id: appliance-*
    - id: nas-*
```

### Resource Requirements

| Devices | CPU | Memory | Network |
|---------|-----|--------|---------|
| 50 | 1 core | 512 MB | 10 Mbps |
| 100 | 2 cores | 1 GB | 25 Mbps |
| 200 | 4 cores | 2 GB | 50 Mbps |
| 500 | 8 cores | 4 GB | 100 Mbps |

## Technical Tasks

### Phase 1: Core Proxy Infrastructure (Weeks 1-3)

**T1.1: Proxy Engine Architecture**
- Create `pkg/proxy/` package structure
- Define ProxyEngine interface and configuration
- Implement device registry (in-memory + persistent)
- Implement proxy mode flag in agent configuration
- Create proxy agent identification scheme

**T1.2: Device Registration**
- Define Device struct with metadata, labels, profile reference
- Implement device registration via config file
- Implement device registration via API (dynamic)
- Create virtual agent registration with control plane
- Implement device lifecycle management (add, update, remove)

**T1.3: Protocol Adapter Framework**
- Define ProtocolAdapter interface
- Create adapter registry and factory
- Implement adapter configuration parsing
- Create connection pooling framework
- Implement timeout and retry logic

**T1.4: Credential Cache (Proxy Agent Side)**
- Define CredentialCache interface (memory-only, no persistence)
- Implement in-memory credential cache with TTL
- Implement cache refresh before expiry
- Implement secure memory wiping on eviction
- Implement X25519 keypair generation for credential encryption

**T1.5: Credential Proxy (Control Plane Side)**
- Implement credential proxy service in control plane
- Define NATS subjects for credential requests/responses
- Implement Vault credential backend
- Implement Kubernetes secrets backend
- Implement credential encryption with proxy agent's public key
- Implement policy enforcement (rate limiting, device assignment validation)
- Implement audit logging for all credential requests
- Support credential rotation notifications

**T1.6: Control Plane Integration**
- Extend agent registration to include proxied devices
- Register proxy agent's public key for credential encryption
- Implement virtual agent heartbeat reporting
- Add proxy device metadata to targeting system
- Update connection manager for proxy awareness

### Phase 2: SSH Protocol Adapter (Weeks 4-5)

**T2.1: SSH Client Implementation**
- Implement SSH connection with key and password auth
- Implement connection pooling (max connections per device)
- Support jump hosts / bastion servers
- Handle SSH host key verification
- Implement keepalive and timeout handling

**T2.2: Command Execution**
- Implement command execution via SSH session
- Capture stdout, stderr, exit code
- Support interactive command sequences (expect-like)
- Implement sudo support
- Handle long-running commands with streaming output

**T2.3: File Transfer**
- Implement SFTP client for file operations
- Support file upload/download
- Support directory operations
- Integrate with file state module

**T2.4: Fact Collection**
- Implement fact collection via SSH commands
- Parse common fact patterns (OS, version, memory, etc.)
- Cache facts with TTL
- Support custom fact commands

### Phase 3: SNMP Protocol Adapter (Weeks 6-7)

**T3.1: SNMP Client Implementation**
- Implement SNMP v2c client
- Implement SNMP v3 client with auth/priv
- Support Get, GetNext, GetBulk, Set operations
- Implement MIB loading and OID translation

**T3.2: SNMP Operations**
- Implement OID get with type detection
- Implement OID set with type specification
- Implement SNMP walk for table retrieval
- Support bulk operations for efficiency

**T3.3: Trap/Inform Handling**
- Implement trap receiver
- Convert traps to Keystone Core events
- Support trap filtering and routing
- Implement inform acknowledgment

**T3.4: MIB Management**
- Implement MIB file loading
- Support standard MIBs (SNMPv2-MIB, IF-MIB, etc.)
- Support vendor MIB loading
- OID name resolution cache

### Phase 4: REST/HTTP Protocol Adapter (Weeks 8-9)

**T4.1: HTTP Client Implementation**
- Implement HTTP client with connection pooling
- Support all HTTP methods
- Implement TLS configuration (certs, skip verify)
- Handle redirects and retries

**T4.2: Authentication**
- Implement Basic authentication
- Implement Bearer token authentication
- Implement API key authentication (header, query param)
- Implement OAuth2 client credentials flow
- Implement OAuth2 token refresh

**T4.3: Request/Response Handling**
- Implement JSON request/response parsing
- Implement XML request/response parsing
- Support JSONPath extraction
- Support XPath extraction
- Implement response pagination handling

**T4.4: Rate Limiting**
- Implement client-side rate limiting
- Respect rate limit headers (429, Retry-After)
- Configurable rate limits per device

### Phase 5: WinRM Protocol Adapter (Weeks 10-11)

**T5.1: WinRM Client Implementation**
- Implement WinRM over HTTP/HTTPS
- Implement NTLM authentication
- Implement Kerberos authentication
- Handle WinRM envelope format

**T5.2: PowerShell Execution**
- Implement PowerShell command execution
- Support script block execution
- Handle output streams (success, error, warning, verbose)
- Implement exit code handling

**T5.3: File Operations**
- Implement file transfer via WinRM
- Support registry operations
- Implement Windows service management

### Phase 6: Vendor Network Adapters (Weeks 12-14)

**T6.1: CLI Interaction Framework**
- Implement expect-like interaction engine
- Support prompt detection (regex patterns)
- Implement mode transitions (exec → privileged → config)
- Handle pagination ("--More--")
- Implement command timeout handling

**T6.2: Cisco IOS/IOS-XE Adapter**
- Implement IOS prompt handling
- Implement enable mode transition
- Implement configure mode transition
- Support show commands with parsing
- Support configuration commands
- Implement configuration backup/diff

**T6.3: Cisco NX-OS Adapter**
- Implement NX-OS specific prompts
- Support JSON/XML output modes
- Implement VDC awareness
- Support checkpoint/rollback

**T6.4: Juniper JUNOS Adapter**
- Implement JUNOS CLI interaction
- Support operational and configuration modes
- Implement commit/rollback workflow
- Support structured output (XML)

**T6.5: Arista EOS Adapter**
- Implement EOS CLI interaction
- Support eAPI (JSON-RPC over HTTP)
- Implement configuration sessions
- Support multi-agent routing

**T6.6: Generic Network Device Adapter**
- Configurable prompt patterns
- Configurable mode transitions
- Template-based command generation

**T6.7: OPNsense Adapter**
- Implement OPNsense REST API client
- Support API key authentication (key + secret)
- Implement firewall alias management
- Implement firewall rule management
- Implement service control (start, stop, restart)
- Implement configuration apply/reconfigure
- Parse JSON responses with error handling
- Support firmware status and system info

**T6.8: pfSense Adapter**
- Implement pfSense REST API client (pfSense Plus / REST API package)
- Support Bearer token and JWT authentication
- Implement firewall alias management
- Implement firewall rule management
- Implement interface configuration
- Implement firewall apply for pending changes
- Parse JSON responses with proper error codes
- Support system status and version info

**T6.9: VyOS/Ubiquiti EdgeOS Adapter**
- Implement VyOS HTTP API client
- Support API key authentication (key in POST body)
- Implement interface configuration (set/delete path operations)
- Implement static route management
- Implement firewall rule and zone management
- Support configuration save/load
- Parse JSON responses from /retrieve, /configure, /show endpoints
- Alternative SSH mode for EdgeOS devices (Vyatta shell)

### Phase 7: State Module Integration (Weeks 15-16)

**T7.1: Proxy State Executor**
- Create proxy-aware state executor
- Route state operations through appropriate adapter
- Handle state result aggregation
- Support partial application reporting

**T7.2: Protocol-Specific State Modules**
- SSH: file, package, service, user, group, cmd
- SNMP: snmp_value, snmp_table
- REST: http_config, http_resource
- Network: ios_config, junos_config, eos_config
- WinRM: file, service, registry, package

**T7.3: Drift Detection**
- Implement drift detection for proxied devices
- Support scheduled drift checks
- Generate drift reports
- Emit drift events

### Phase 8: Discovery and Auto-Configuration (Week 17)

**T8.1: Network Discovery**
- Implement subnet scanning
- Implement SNMP discovery (sysDescr)
- Implement SSH banner detection
- Implement LLDP/CDP neighbor discovery

**T8.2: Profile Auto-Detection**
- Match discovered devices to profiles
- Suggest configuration for new devices
- Support discovery rules configuration

**T8.3: Dynamic Registration**
- API for approving discovered devices
- Automatic credential assignment
- Integration with credential rotation

### Phase 9: Observability (Week 18)

**T9.1: Metrics**
- Implement proxy agent metrics collector
- Per-device metrics (commands, latency, health)
- Protocol adapter metrics (connections, errors)
- Export to Prometheus

**T9.2: Logging and Tracing**
- Structured logging for all proxy operations
- Distributed tracing through proxy hops
- Correlation ID propagation
- Audit logging for compliance

**T9.3: Dashboards**
- Create Grafana dashboard for proxy agents
- Per-device health dashboard
- Protocol adapter performance dashboard
- Alert rules for proxy health

### Phase 10: Testing and Documentation (Weeks 19-20)

**T10.1: Unit Tests**
- Protocol adapter tests with mocks
- Credential store tests
- Device registry tests
- Configuration parsing tests

**T10.2: Mock Device Servers**
Since pfSense/OPNsense run as VMs and VyOS is open-source, we can create lightweight mock servers for CI/CD:

- **OPNsense Mock Server** (`test/e2e/mocks/opnsense/`)
  - Simulates OPNsense REST API endpoints
  - `/api/core/firmware/status`, `/api/core/system/status`
  - `/api/firewall/alias/*`, `/api/firewall/filter/*`
  - `/api/core/service/*` for service control
  - In-memory state for aliases, rules, services
  - Basic auth validation

- **pfSense Mock Server** (`test/e2e/mocks/pfsense/`)
  - Simulates pfSense REST API (v1) endpoints
  - `/api/v1/status/system`, `/api/v1/status/interface`
  - `/api/v1/firewall/alias`, `/api/v1/firewall/rule`
  - `/api/v1/firewall/apply` for pending changes
  - Bearer token validation
  - Proper pfSense response format with `code`, `status`, `data` fields

- **VyOS Mock Server** (`test/e2e/mocks/vyos/`)
  - Simulates VyOS HTTP API endpoints
  - `/retrieve`, `/configure`, `/config-file`, `/show`
  - Path-based configuration tree in memory
  - API key validation in POST body
  - JSON response format matching VyOS API

**T10.3: Integration Tests**
- SSH adapter with containerized SSH server
- SNMP adapter with net-snmp container
- REST adapter with mock HTTP server
- WinRM adapter with Windows container
- OPNsense adapter with mock server
- pfSense adapter with mock server
- VyOS adapter with mock server

**T10.4: E2E Test Harness**

Test infrastructure for network device adapters:

```
test/e2e/
├── topologies/
│   └── proxy-agents/
│       ├── docker-compose.yml      # Test orchestration
│       ├── opnsense/
│       │   └── Vagrantfile         # Real OPNsense VM (optional)
│       ├── pfsense/
│       │   └── Vagrantfile         # Real pfSense VM (optional)
│       └── vyos/
│           └── Vagrantfile         # Real VyOS VM (optional)
├── mocks/
│   ├── opnsense/                   # OPNsense mock server
│   ├── pfsense/                    # pfSense mock server
│   └── vyos/                       # VyOS mock server
├── scenarios/
│   ├── proxy_opnsense_test.go      # OPNsense E2E tests
│   ├── proxy_pfsense_test.go       # pfSense E2E tests
│   ├── proxy_vyos_test.go          # VyOS E2E tests
│   └── proxy_network_devices_test.go  # Cross-adapter tests
└── harness/
    └── network_device_harness.go   # Test utilities
```

**T10.5: E2E Test Scenarios**

For each adapter (OPNsense, pfSense, VyOS):

1. **Device Registration**
   - Device appears as virtual agent
   - Correct labels and metadata reported
   - Health check passes

2. **Firewall Alias Management**
   - Create alias with IP addresses
   - Verify alias exists
   - Idempotency (re-apply = no changes)
   - Delete alias

3. **Firewall Rule Management**
   - Create rule referencing alias
   - Verify rule ordering
   - Update rule
   - Delete rule

4. **Service Control**
   - Check service status
   - Restart service
   - Stop/start service

5. **Configuration Operations**
   - Backup configuration
   - Apply configuration changes
   - Save configuration (VyOS)

6. **State Management Integration**
   - Apply state file targeting proxied device
   - Drift detection on proxied device
   - Mixed targeting (native + proxied agents)

**T10.6: VM-Based Testing (Optional)**

For deeper validation with real device behavior, use Vagrant:

```ruby
# test/e2e/topologies/proxy-agents/opnsense/Vagrantfile
Vagrant.configure("2") do |config|
  config.vm.box = "opnsense/opnsense"
  config.vm.network "private_network", ip: "192.168.56.10"
  config.vm.provider "virtualbox" do |vb|
    vb.memory = "2048"
    vb.cpus = 2
  end
end
```

```ruby
# test/e2e/topologies/proxy-agents/vyos/Vagrantfile
Vagrant.configure("2") do |config|
  config.vm.box = "vyos/current"
  config.vm.network "private_network", ip: "192.168.56.11"
  config.vm.provision "shell", inline: <<-SHELL
    source /opt/vyatta/etc/functions/script-template
    configure
    set service https api keys id testkey key 'test-api-key'
    commit && save
  SHELL
end
```

**T10.7: CI/CD Integration**

```yaml
# .github/workflows/e2e-proxy-agents.yml
name: E2E Proxy Agent Tests

on:
  push:
    paths:
      - 'pkg/proxy/**'
      - 'test/e2e/scenarios/proxy_*.go'
      - 'test/e2e/mocks/**'

jobs:
  mock-tests:
    name: Mock Device Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run mock-based E2E tests
        run: |
          KSCORE_E2E_TESTS=1 go test -v ./test/e2e/scenarios/proxy_*.go -timeout 10m

  vm-tests:
    name: VM Device Tests
    runs-on: macos-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - name: Install Vagrant
        run: brew install vagrant
      - name: Start OPNsense VM
        working-directory: test/e2e/topologies/proxy-agents/opnsense
        run: vagrant up
      - name: Run VM-based tests
        run: |
          KSCORE_E2E_TESTS=1 KSCORE_REAL_DEVICES=1 \
          OPNSENSE_URL=https://192.168.56.10 \
          go test -v ./test/e2e/scenarios/proxy_opnsense_real_test.go -timeout 20m
      - name: Cleanup
        if: always()
        run: vagrant destroy -f
```

**T10.8: Test Coverage Targets**

| Component | Target | Strategy |
|-----------|--------|----------|
| Protocol Adapters | 80% | Unit tests with mocks |
| Device Profiles | 90% | Configuration parsing tests |
| Mock Servers | 70% | Integration tests |
| E2E Scenarios | 100% pass | Mock + optional VM tests |

**T10.9: Documentation**
- Proxy agent configuration reference
- Device profile creation guide
- Protocol adapter reference
- Troubleshooting guide
- Migration guide from legacy tools
- E2E testing guide for contributors

## Dependencies

### Required Epics
- **Epic 1** (Core Infrastructure): Agent framework, NATS communication
- **Epic 2** (Remote Execution): Command execution framework, targeting
- **Epic 3** (State Management): State module framework
- **Epic 7** (Observability): Metrics, logging, tracing infrastructure
- **Epic 14** (NATS Mesh): Advanced NATS communication patterns

### External Dependencies
- `golang.org/x/crypto/ssh` - SSH client
- `github.com/gosnmp/gosnmp` - SNMP client
- `github.com/masterzen/winrm` - WinRM client
- Standard library `net/http` - HTTP client

## Risks and Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Protocol complexity | High | Medium | Start with SSH, iterate on complex protocols |
| Credential security | Critical | Low | Use established secret stores (Vault), encryption at rest |
| Device compatibility | Medium | High | Extensive device profile testing, community contributions |
| Performance bottleneck | Medium | Medium | Connection pooling, async operations, horizontal scaling |
| Network reliability | Medium | Medium | Retry logic, health checks, graceful degradation |
| Vendor CLI changes | Low | Medium | Version-specific profiles, automated testing |

## Security Considerations

1. **NATS-Proxied Credential Security**
   - Proxy agents never have direct access to Vault or credential stores
   - Credentials fetched via control plane over NATS (mTLS)
   - End-to-end encryption: credentials encrypted with proxy agent's X25519 public key
   - Forward secrecy: ephemeral keys for each credential request
   - Memory-only caching at proxy agent (never written to disk)
   - Secure memory wiping when credentials expire or are evicted
   - Short TTLs (default 5 minutes) limit exposure window
   - Rate limiting prevents credential enumeration attacks

2. **Control Plane Credential Proxy**
   - Centralized audit logging of all credential requests
   - Policy enforcement before credential issuance:
     - Verify proxy agent is authorized for the device
     - Check device is assigned to the requesting proxy agent
     - Enforce rate limits per proxy agent
     - Validate credential reference exists
   - Credential rotation notifications pushed to proxy agents
   - Support for credential backends: Vault, Kubernetes secrets, encrypted files

3. **Network Security**
   - NATS mTLS (TLS 1.3) for all control plane ↔ proxy agent communication
   - TLS for all HTTP-based device protocols
   - SSH host key verification (configurable: strict, accept-new, off)
   - SNMP v3 with authentication (SHA256) and encryption (AES256)
   - Network segmentation: proxy agents can be in DMZs, only need NATS connectivity

4. **Access Control**
   - Device-level access control via control plane policy
   - Protocol-level restrictions per device profile
   - Command filtering/allowlisting for network devices
   - Comprehensive audit trail for all operations

5. **Least Privilege**
   - Proxy agents only receive credentials for assigned devices
   - Credential scoping to specific devices (no wildcard access)
   - Read-only mode support for monitoring-only devices
   - Minimal permissions in credential backends (Vault policies)

## Testing Strategy

### Unit Tests
- All protocol adapters with mocked backends
- Credential store implementations
- Device registry operations
- Configuration parsing

### Integration Tests
- SSH adapter with OpenSSH container
- SNMP adapter with net-snmp container
- REST adapter with mock API server
- WinRM adapter with Windows container (if available)

### E2E Tests
- Full device lifecycle (register, execute, state, health)
- Multi-device orchestration
- Failover scenarios
- Credential rotation

### Performance Tests
- Command execution latency benchmarks
- Connection pool efficiency
- 100+ device scale testing
- Concurrent command execution

## Definition of Done

- [ ] Proxy agent mode configurable via flag/config
- [ ] Device registration works (static + dynamic)
- [ ] SSH adapter supports key/password auth and commands
- [ ] SNMP adapter supports v2c/v3 get/set
- [ ] REST adapter supports common auth methods
- [ ] WinRM adapter supports PowerShell execution
- [ ] Vendor network adapters complete:
  - [ ] Cisco IOS/IOS-XE
  - [ ] Juniper JUNOS
  - [ ] Arista EOS
- [ ] Open-source firewall adapters complete:
  - [ ] OPNsense (REST API)
  - [ ] pfSense (REST API)
- [ ] VyOS/Ubiquiti EdgeOS adapter complete (HTTP API + SSH fallback)
- [ ] State modules work through proxy
- [ ] Credential store integrates with Vault
- [ ] Health monitoring works for all devices
- [ ] Transparent targeting verified
- [ ] Metrics exported to Prometheus
- [ ] Unit test coverage > 70%
- [ ] Integration tests pass
- [ ] E2E tests pass (mock-based)
- [ ] E2E mock servers implemented:
  - [ ] OPNsense mock server
  - [ ] pfSense mock server
  - [ ] VyOS mock server
- [ ] Documentation complete
- [ ] Performance benchmarks meet criteria
