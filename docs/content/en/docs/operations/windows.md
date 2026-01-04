---
title: "Windows Deployment"
weight: 8
description: >
  Deploying and managing Keystone Core agents on Windows
---

## Overview

Keystone Core provides first-class Windows support for the agent component, enabling management of Windows servers alongside Linux and macOS infrastructure. The Windows agent runs as a native Windows service, integrates with Windows Event Log, and supports Windows-specific management features.

**Supported Windows Versions**:
- Windows Server 2016, 2019, 2022
- Windows 10/11 (for development)

## Installation

### Prerequisites

- Administrator privileges for service installation
- Network connectivity to control plane
- .NET Framework 4.7.2+ (for some Windows features)

### Download

Download the latest Windows agent from the releases page:

```powershell
# Download using PowerShell
Invoke-WebRequest -Uri "https://releases.keystone-core.io/latest/kscore-agent-windows-amd64.exe" `
  -OutFile "kscore-agent.exe"
```

### Install as Windows Service

The agent can be installed as a Windows service that starts automatically on boot:

```powershell
# Install the service (requires Administrator)
.\kscore-agent.exe service-install

# Start the service
.\kscore-agent.exe service-start

# Check status
.\kscore-agent.exe service-status
```

The service will be configured with:
- **Name**: kscore-agent
- **Display Name**: Keystone Core Agent
- **Startup Type**: Automatic (Delayed Start)
- **Account**: Local System
- **Recovery**: Restart on failure (5s, 30s, 60s delays)

### Uninstall

```powershell
# Stop and remove the service
.\kscore-agent.exe service-uninstall
```

## Configuration

### Configuration File Location

The default configuration file location on Windows is:

```
C:\ProgramData\kscore\agent.yaml
```

You can specify an alternate location using:
- Command line: `--config C:\path\to\config.yaml`
- Environment variable: `KSCORE_CONFIG=C:\path\to\config.yaml`

### Sample Configuration

```yaml
# C:\ProgramData\kscore\agent.yaml
agent:
  id: ${COMPUTERNAME}
  heartbeat_interval: 30s
  metadata_interval: 5m
  command_timeout: 5m

nats:
  url: nats://nats.example.com:4222
  tls:
    enabled: true
    ca_file: C:\ProgramData\kscore\ca.crt
    cert_file: C:\ProgramData\kscore\agent.crt
    key_file: C:\ProgramData\kscore\agent.key

logging:
  level: info
  # Output to Windows Event Log
  output: eventlog
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `KSCORE_CONFIG` | Configuration file path | `C:\ProgramData\kscore\agent.yaml` |
| `KSCORE_AGENT_ID` | Agent identifier | `%COMPUTERNAME%` |
| `KSCORE_NATS_URL` | NATS server URL | - |
| `KSCORE_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

## Windows Event Log

The agent logs to the Windows Event Log, making it easy to monitor using standard Windows tools.

### Event Source

- **Source**: KeystoneCore
- **Log**: Application

### Event IDs

| Range | Category | Examples |
|-------|----------|----------|
| 1000-1099 | Informational | Agent started (1001), Connected (1003), Command executed (1005) |
| 2000-2099 | Warning | Connection retry (2001), Heartbeat missed (2002), Timeout (2003) |
| 3000-3099 | Error | Connection failed (3001), Command failed (3002), Config error (3004) |
| 4000-4099 | Security | Auth success (4001), Auth failure (4002), Policy violation (4003) |

### Viewing Events

```powershell
# View recent agent events
Get-EventLog -LogName Application -Source KeystoneCore -Newest 50

# Filter by event type
Get-EventLog -LogName Application -Source KeystoneCore -EntryType Error

# Export to file
Get-EventLog -LogName Application -Source KeystoneCore |
  Export-Csv -Path agent-events.csv
```

### Event Viewer

1. Open Event Viewer (`eventvwr.msc`)
2. Navigate to Windows Logs > Application
3. Filter by Source: KeystoneCore

## Service Management

### Using kscore-agent Commands

```powershell
# Install service
kscore-agent.exe service-install

# Uninstall service
kscore-agent.exe service-uninstall

# Start service
kscore-agent.exe service-start

# Stop service
kscore-agent.exe service-stop

# Check status
kscore-agent.exe service-status
```

### Using PowerShell

```powershell
# Get service status
Get-Service kscore-agent

# Start service
Start-Service kscore-agent

# Stop service
Stop-Service kscore-agent

# Restart service
Restart-Service kscore-agent

# View service configuration
Get-WmiObject -Class Win32_Service -Filter "Name='kscore-agent'"
```

### Using sc.exe

```cmd
rem Query service
sc query kscore-agent

rem Start service
sc start kscore-agent

rem Stop service
sc stop kscore-agent

rem View configuration
sc qc kscore-agent

rem Configure recovery
sc failure kscore-agent reset= 86400 actions= restart/5000/restart/30000/restart/60000
```

## Running in Console Mode

For debugging or testing, run the agent interactively:

```powershell
# Run in console mode (Ctrl+C to stop)
.\kscore-agent.exe --config C:\ProgramData\kscore\agent.yaml
```

## Firewall Configuration

The agent requires outbound connectivity to the control plane:

```powershell
# Allow outbound NATS (default port 4222)
New-NetFirewallRule -DisplayName "Keystone Core Agent - NATS" `
  -Direction Outbound `
  -Protocol TCP `
  -RemotePort 4222 `
  -Action Allow

# Allow outbound NATS TLS (if using different port)
New-NetFirewallRule -DisplayName "Keystone Core Agent - NATS TLS" `
  -Direction Outbound `
  -Protocol TCP `
  -RemotePort 4443 `
  -Action Allow
```

## Antivirus Considerations

Some antivirus software may interfere with agent operations. Consider adding exclusions for:

- Executable: `C:\Program Files\kscore\kscore-agent.exe`
- Config directory: `C:\ProgramData\kscore\`
- Log directory: `C:\ProgramData\kscore\logs\`

### Windows Defender

```powershell
# Add exclusions
Add-MpPreference -ExclusionPath "C:\Program Files\kscore"
Add-MpPreference -ExclusionPath "C:\ProgramData\kscore"
Add-MpPreference -ExclusionProcess "kscore-agent.exe"
```

## Troubleshooting

### Service Won't Start

1. Check Event Log for errors:
   ```powershell
   Get-EventLog -LogName Application -Source KeystoneCore -Newest 10
   ```

2. Verify configuration file exists and is valid:
   ```powershell
   Test-Path C:\ProgramData\kscore\agent.yaml
   ```

3. Run in console mode to see errors:
   ```powershell
   .\kscore-agent.exe --config C:\ProgramData\kscore\agent.yaml
   ```

### Connection Issues

1. Test connectivity to NATS server:
   ```powershell
   Test-NetConnection -ComputerName nats.example.com -Port 4222
   ```

2. Check firewall rules:
   ```powershell
   Get-NetFirewallRule -DisplayName "*Keystone*"
   ```

3. Verify TLS certificates:
   ```powershell
   Test-Path C:\ProgramData\kscore\ca.crt
   ```

### Permission Issues

1. Verify service account has required permissions
2. Check file system permissions on config directory
3. Ensure service is running as Local System or appropriate account

### Debug Logging

Enable debug logging for more detailed output:

```yaml
# C:\ProgramData\kscore\agent.yaml
logging:
  level: debug
```

Then restart the service:
```powershell
Restart-Service kscore-agent
```

## Enterprise Deployment

### Group Policy

Deploy the agent MSI via Group Policy:

1. Create a network share for the MSI
2. Create a new GPO and link to target OU
3. Navigate to Computer Configuration > Policies > Software Settings > Software Installation
4. Add the MSI package with assigned deployment

### SCCM/Intune

Deploy using Microsoft Endpoint Manager:

1. Create a Win32 app package
2. Use silent installation: `msiexec /i kscore-agent.msi /qn SERVERURL=nats://... AGENTID=%COMPUTERNAME%`
3. Detection rule: File exists `C:\Program Files\kscore\kscore-agent.exe`
4. Uninstall command: `msiexec /x {ProductCode} /qn`

### MSI Properties

| Property | Description | Example |
|----------|-------------|---------|
| `SERVERURL` | NATS server URL | `nats://nats.example.com:4222` |
| `AGENTID` | Agent identifier | `%COMPUTERNAME%` |
| `LOGLEVEL` | Log level | `info` |
| `CONFIGPATH` | Custom config path | `C:\custom\agent.yaml` |

```cmd
rem Silent installation with properties
msiexec /i kscore-agent.msi /qn ^
  SERVERURL=nats://nats.example.com:4222 ^
  AGENTID=%COMPUTERNAME% ^
  LOGLEVEL=info
```

## See Also

- [Agent Configuration](../deployment/#agent-configuration) - Detailed configuration options
- [Troubleshooting](../troubleshooting/) - General troubleshooting guide
- [Security](../security/) - Security configuration
