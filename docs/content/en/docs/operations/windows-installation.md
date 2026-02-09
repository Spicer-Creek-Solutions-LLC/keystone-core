---
title: "Windows Installation"
weight: 15
description: "Installing and configuring the Keystone Core agent on Windows"
---

This guide covers installing and operating the Keystone Core agent on Windows Server and Windows 10/11.

## System Requirements

### Supported Windows Versions

- Windows Server 2016, 2019, 2022
- Windows 10 (version 1809 or later)
- Windows 11

### Hardware Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 256 MB | 512 MB |
| Disk | 100 MB | 500 MB |
| Network | 1 Mbps | 10+ Mbps |

### Network Requirements

| Direction | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Outbound | 4222 | TCP | NATS server connection |
| Outbound | 4223 | TCP | NATS TLS (optional) |
| Outbound | 443 | TCP | WebSocket (optional) |

## Installation Methods

### Method 1: MSI Installer (Recommended)

#### Interactive Installation

1. Download `kscore-agent.msi` from the [releases page](https://github.com/shawnbutts/keystone-core/releases)

2. Run the installer:
   ```powershell
   Start-Process msiexec.exe -ArgumentList '/i', 'kscore-agent.msi' -Wait
   ```

3. Follow the installation wizard:
   - Accept the license agreement
   - Configure server URL and agent ID
   - Choose installation location

#### Silent Installation

```powershell
# Basic silent install
msiexec /i kscore-agent.msi /qn

# With server URL
msiexec /i kscore-agent.msi /qn SERVERURL=nats://control-plane.example.com:4222

# Full configuration
msiexec /i kscore-agent.msi /qn `
    SERVERURL=nats://control-plane.example.com:4222 `
    AGENTID=web-server-01 `
    LOGLEVEL=info `
    TLSENABLED=1

# With installation log
msiexec /i kscore-agent.msi /qn /l*v install.log SERVERURL=nats://server:4222
```

#### MSI Properties

| Property | Description | Default | Example |
|----------|-------------|---------|---------|
| `SERVERURL` | NATS server URL | (required) | `nats://server:4222` |
| `AGENTID` | Agent identifier | Computer name | `web-server-01` |
| `LOGLEVEL` | Log level | `info` | `debug`, `warn`, `error` |
| `TLSENABLED` | Enable TLS | `0` | `1` |
| `TLSCAPATH` | CA certificate path | (none) | `C:\certs\ca.crt` |
| `INSTALLDIR` | Installation path | `%ProgramFiles%\KeystoneCore` | `D:\Apps\KeystoneCore` |

### Method 2: Manual Installation

1. Download the agent binary:
   ```powershell
   Invoke-WebRequest -Uri "https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-agent-windows-amd64.exe" `
       -OutFile "C:\Program Files\KeystoneCore\bin\kscore-agent.exe"
   ```

2. Create configuration directory:
   ```powershell
   New-Item -ItemType Directory -Force -Path "C:\ProgramData\kscore"
   ```

3. Create configuration file (see [Configuration](#configuration))

4. Install as Windows service:
   ```powershell
   & "C:\Program Files\KeystoneCore\bin\kscore-agent.exe" service-install
   ```

5. Start the service:
   ```powershell
   Start-Service KeystoneCoreAgent
   ```

### Method 3: Chocolatey

```powershell
# Install via Chocolatey
choco install kscore-agent --params "/ServerUrl:nats://server:4222"

# Upgrade
choco upgrade kscore-agent

# Uninstall
choco uninstall kscore-agent
```

### Method 4: winget

```powershell
# Install via winget
winget install KeystoneCore.Agent --custom "/SERVERURL=nats://server:4222"
```

## Configuration

### Configuration File Location

The agent configuration is stored at:
```
C:\ProgramData\kscore\agent.yaml
```

### Basic Configuration

```yaml
# C:\ProgramData\kscore\agent.yaml
agent:
  id: "web-server-01"  # Unique agent identifier

  nats:
    urls:
      - nats://control-plane.example.com:4222

  heartbeat:
    interval: 10s

  logging:
    level: info
    output: eventlog
```

### TLS Configuration

```yaml
agent:
  nats:
    urls:
      - tls://control-plane.example.com:4223
    tls:
      enabled: true
      ca: C:\ProgramData\kscore\ca.crt
      cert: C:\ProgramData\kscore\agent.crt
      key: C:\ProgramData\kscore\agent.key
```

### Windows-Specific Settings

```yaml
agent:
  windows:
    # Prefer PowerShell Core (pwsh) if available
    prefer_pwsh: true

    # PowerShell execution policy for scripts
    execution_policy: RemoteSigned

    # Event log settings
    event_log:
      source: KeystoneCore
      log: Application

  logging:
    # Output to Windows Event Log
    output: eventlog

    # Or output to file
    # output: file
    # file:
    #   path: C:\ProgramData\kscore\logs\agent.log
    #   max_size: 100
    #   max_backups: 3
```

### Agent Labels

```yaml
agent:
  labels:
    os: windows
    role: webserver
    environment: production
    datacenter: us-east-1
```

## Service Management

### Using PowerShell

```powershell
# Check service status
Get-Service KeystoneCoreAgent

# Start service
Start-Service KeystoneCoreAgent

# Stop service
Stop-Service KeystoneCoreAgent

# Restart service
Restart-Service KeystoneCoreAgent

# Set to automatic start
Set-Service KeystoneCoreAgent -StartupType Automatic

# Disable service
Set-Service KeystoneCoreAgent -StartupType Disabled
```

### Using sc.exe

```cmd
:: Check status
sc query KeystoneCoreAgent

:: Start service
sc start KeystoneCoreAgent

:: Stop service
sc stop KeystoneCoreAgent

:: Configure automatic start
sc config KeystoneCoreAgent start=auto

:: Configure recovery options
sc failure KeystoneCoreAgent reset=86400 actions=restart/5000/restart/10000/none/0
```

### Using Services GUI

1. Open Services: `Win+R` → `services.msc`
2. Find "Keystone Core Agent"
3. Right-click for start/stop/restart options
4. Double-click for properties (startup type, recovery, etc.)

## Event Logging

### Viewing Logs

```powershell
# View recent logs
Get-EventLog -LogName Application -Source KeystoneCore -Newest 50

# View errors only
Get-EventLog -LogName Application -Source KeystoneCore -EntryType Error -Newest 20

# Export to file
Get-EventLog -LogName Application -Source KeystoneCore -After (Get-Date).AddDays(-7) |
    Export-Csv -Path "C:\Logs\kscore-events.csv"

# Using Event Viewer GUI
eventvwr.msc
# Navigate to: Windows Logs > Application
# Filter by Source: KeystoneCore
```

### Log Levels

| Event Type | Windows Event Level | Description |
|------------|---------------------|-------------|
| Debug | Information (ID 1000) | Detailed debugging information |
| Info | Information (ID 1001) | Normal operational messages |
| Warn | Warning (ID 2001) | Warning conditions |
| Error | Error (ID 3001) | Error conditions |

## Firewall Configuration

### Using PowerShell

```powershell
# Allow outbound NATS connection
New-NetFirewallRule -DisplayName "Keystone Core Agent (NATS)" `
    -Direction Outbound `
    -Protocol TCP `
    -RemotePort 4222 `
    -Action Allow `
    -Program "C:\Program Files\KeystoneCore\bin\kscore-agent.exe"

# Allow outbound NATS TLS
New-NetFirewallRule -DisplayName "Keystone Core Agent (NATS TLS)" `
    -Direction Outbound `
    -Protocol TCP `
    -RemotePort 4223 `
    -Action Allow `
    -Program "C:\Program Files\KeystoneCore\bin\kscore-agent.exe"
```

### Using netsh

```cmd
netsh advfirewall firewall add rule name="Keystone Core Agent" ^
    dir=out action=allow protocol=TCP remoteport=4222 ^
    program="C:\Program Files\KeystoneCore\bin\kscore-agent.exe"
```

## Upgrading

### MSI Upgrade

```powershell
# Download new version
Invoke-WebRequest -Uri "https://example.com/kscore-agent-new.msi" -OutFile "kscore-agent.msi"

# Upgrade (preserves configuration)
msiexec /i kscore-agent.msi /qn

# With logging
msiexec /i kscore-agent.msi /qn /l*v upgrade.log
```

### Manual Upgrade

```powershell
# Stop service
Stop-Service KeystoneCoreAgent

# Backup current binary
Copy-Item "C:\Program Files\KeystoneCore\bin\kscore-agent.exe" `
    "C:\Program Files\KeystoneCore\bin\kscore-agent.exe.bak"

# Download new binary
Invoke-WebRequest -Uri "https://example.com/kscore-agent.exe" `
    -OutFile "C:\Program Files\KeystoneCore\bin\kscore-agent.exe"

# Start service
Start-Service KeystoneCoreAgent

# Verify version
& "C:\Program Files\KeystoneCore\bin\kscore-agent.exe" version
```

## Uninstallation

### MSI Uninstall

```powershell
# Using product name
Get-WmiObject -Class Win32_Product -Filter "Name='Keystone Core Agent'" |
    ForEach-Object { $_.Uninstall() }

# Using MSI file
msiexec /x kscore-agent.msi /qn

# Using product code
msiexec /x {product-code-guid} /qn
```

### Manual Uninstall

```powershell
# Stop and remove service
Stop-Service KeystoneCoreAgent -ErrorAction SilentlyContinue
& "C:\Program Files\KeystoneCore\bin\kscore-agent.exe" service-uninstall

# Remove files
Remove-Item -Recurse -Force "C:\Program Files\KeystoneCore"

# Remove configuration (optional)
Remove-Item -Recurse -Force "C:\ProgramData\kscore"

# Remove registry entries
Remove-Item -Path "HKLM:\SOFTWARE\KeystoneCore" -Recurse -ErrorAction SilentlyContinue
```

## Troubleshooting

### Service Won't Start

1. **Check configuration file**:
   ```powershell
   Test-Path "C:\ProgramData\kscore\agent.yaml"
   Get-Content "C:\ProgramData\kscore\agent.yaml"
   ```

2. **Check event log for errors**:
   ```powershell
   Get-EventLog -LogName Application -Source KeystoneCore -Newest 10
   ```

3. **Test connectivity to NATS server**:
   ```powershell
   Test-NetConnection -ComputerName control-plane.example.com -Port 4222
   ```

4. **Run in foreground for debugging** (instead of as a service):
   ```powershell
   & "C:\Program Files\KeystoneCore\bin\kscore-agent.exe" --config "C:\ProgramData\kscore\agent.yaml"
   ```

### Connection Issues

1. **Check firewall rules**:
   ```powershell
   Get-NetFirewallRule -DisplayName "*Keystone*"
   ```

2. **Check DNS resolution**:
   ```powershell
   Resolve-DnsName control-plane.example.com
   ```

3. **Test TLS certificate**:
   ```powershell
   $cert = Get-PfxCertificate -FilePath "C:\ProgramData\kscore\agent.crt"
   $cert | Format-List
   ```

### Permission Issues

1. **Check service account**:
   ```powershell
   (Get-WmiObject Win32_Service -Filter "Name='KeystoneCoreAgent'").StartName
   ```

2. **Verify file permissions**:
   ```powershell
   Get-Acl "C:\ProgramData\kscore\agent.yaml" | Format-List
   ```

### Performance Issues

1. **Check resource usage**:
   ```powershell
   Get-Process kscore-agent | Select-Object CPU, WorkingSet64, Handles
   ```

2. **Enable debug logging**:
   ```yaml
   # In agent.yaml
   logging:
     level: debug
   ```

## Enterprise Deployment

See the [Windows setup guide](/docs/operations/windows/) for:

- Group Policy deployment
- SCCM deployment
- Intune deployment
- Antivirus exclusions
