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
- PowerShell 5.1 or later (Windows PowerShell) or PowerShell 7.0+ (PowerShell Core)

### PowerShell Version Requirements

Keystone Core supports both Windows PowerShell and PowerShell Core. The table below shows compatibility and recommended versions:

| PowerShell Version | Status | Notes |
|-------------------|--------|-------|
| PowerShell 7.4+ | Recommended | Best cross-platform support, latest features |
| PowerShell 7.0-7.3 | Supported | Good compatibility, active maintenance |
| Windows PowerShell 5.1 | Supported | Built into Windows, widest compatibility |
| Windows PowerShell 5.0 | Deprecated | May work but not tested |
| Windows PowerShell 4.0 | Unsupported | Missing required features |
| Windows PowerShell 3.0 | Unsupported | Missing required features |

**Check your PowerShell version:**

```powershell
$PSVersionTable.PSVersion
```

**Check Windows PowerShell version:**
```powershell
# Windows PowerShell path
powershell.exe -Command "$PSVersionTable.PSVersion"

# PowerShell Core path
pwsh.exe -Command "$PSVersionTable.PSVersion"
```

### PowerShell Edition Considerations

| Feature | Windows PowerShell 5.1 | PowerShell 7+ |
|---------|----------------------|---------------|
| Built into Windows | Yes | No (install separately) |
| Cross-platform | No | Yes |
| WMI/CIM cmdlets | Full support | Partial (Windows only) |
| .NET Framework | Yes | .NET Core/.NET 5+ |
| Remoting over SSH | No | Yes |
| Default encoding | UTF-16 LE | UTF-8 |
| Parallel execution | Limited | `ForEach-Object -Parallel` |

### PowerShell Execution Policy

Keystone Core may need to execute PowerShell scripts for state management and command execution. Configure the execution policy appropriately:

```powershell
# Check current policy
Get-ExecutionPolicy -List

# Recommended for managed servers (allows signed scripts)
Set-ExecutionPolicy RemoteSigned -Scope LocalMachine

# Or for more permissive environments (allows all local scripts)
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
```

**Execution Policy Levels:**

| Policy | Description | Recommended For |
|--------|-------------|-----------------|
| Restricted | No scripts can run | Not suitable |
| AllSigned | All scripts must be signed | High-security |
| RemoteSigned | Local scripts run; remote scripts need signature | Recommended |
| Unrestricted | All scripts run (with prompts) | Development only |
| Bypass | No restrictions, no prompts | Automation only |

### PowerShell Encoding

When executing commands that produce text output, be aware of encoding differences:

**Windows PowerShell 5.1 (UTF-16 LE by default):**
```powershell
# Force UTF-8 for command output
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
```

**PowerShell 7+ (UTF-8 by default):**
```powershell
# Already UTF-8, but can be explicitly set
$PSDefaultParameterValues['Out-File:Encoding'] = 'utf8'
```

**Configuration for consistent encoding:**
```yaml
# agent.yaml
execution:
  shell: powershell
  powershell:
    # Use UTF-8 encoding for all operations
    encoding: utf8
    # Use PowerShell Core if available, fall back to Windows PowerShell
    prefer_pwsh: true
```

### PowerShell Encoding Edge Cases

This section documents encoding edge cases that can cause unexpected behavior in remote command execution, particularly when mixing Windows PowerShell 5.1 and PowerShell 7+.

#### BOM (Byte Order Mark) Handling

Windows PowerShell 5.1 includes a BOM in UTF-8 output by default, while PowerShell 7+ does not:

| Scenario | Windows PowerShell 5.1 | PowerShell 7+ |
|----------|------------------------|---------------|
| `Out-File` | UTF-16 LE with BOM | UTF-8 without BOM |
| `Set-Content` | System default (ANSI) | UTF-8 without BOM |
| `Add-Content` | System default (ANSI) | UTF-8 without BOM |
| `Export-Csv` | ASCII | UTF-8 without BOM |
| `ConvertTo-Json > file` | UTF-16 LE with BOM | UTF-8 without BOM |

**Edge Case: BOM causes JSON parsing failures**
```powershell
# Windows PowerShell 5.1 - Creates file with BOM
$data | ConvertTo-Json | Out-File data.json

# Later parsing may fail due to BOM
$parsed = Get-Content data.json | ConvertFrom-Json
# Error: "Unexpected character encountered while parsing value: "
```

**Solution:**
```powershell
# Windows PowerShell 5.1 - Force UTF-8 without BOM
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText("data.json", ($data | ConvertTo-Json), $utf8NoBom)

# Or use -Encoding utf8 explicitly (still has BOM in 5.1, but consistent)
$data | ConvertTo-Json | Out-File -Encoding utf8 data.json
```

#### Pipeline Encoding Behavior

PowerShell pipelines handle encoding differently than direct output:

**Edge Case: Pipeline drops special characters**
```powershell
# Direct assignment preserves characters
$text = "日本語テスト"
Write-Output $text  # Works correctly

# But piping through external commands may corrupt
$text | cmd.exe /c "type CON"  # May produce garbled output
```

**Edge Case: Native command output encoding**
```powershell
# Windows PowerShell 5.1 - Console encoding mismatch
$output = & ipconfig /all  # Uses OEM code page
# If console is UTF-8 but ipconfig outputs OEM, corruption occurs
```

**Solution - Set console encoding explicitly:**
```powershell
# Windows PowerShell 5.1 - Match console to command output
[Console]::OutputEncoding = [System.Text.Encoding]::GetEncoding(437)  # OEM US
$output = & ipconfig /all

# Or for full UTF-8 support
chcp 65001  # Set console to UTF-8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
```

#### File I/O Encoding Defaults

Different cmdlets use different default encodings, which can cause data corruption when mixing them:

| Cmdlet | Windows PS 5.1 Default | PowerShell 7+ Default |
|--------|------------------------|----------------------|
| `Get-Content` | System default | UTF-8 |
| `Set-Content` | System default | UTF-8 |
| `Add-Content` | System default | UTF-8 |
| `Out-File` | UTF-16 LE | UTF-8 |
| `Export-Csv` | ASCII | UTF-8 |
| `Export-Clixml` | UTF-8 with BOM | UTF-8 without BOM |
| `Invoke-WebRequest` (save) | UTF-8 | UTF-8 |

**Edge Case: Mixed encoding in config files**
```powershell
# Windows PowerShell 5.1
Get-Content config.json | ConvertFrom-Json  # Uses system default
$config.value = "新しい値"
$config | ConvertTo-Json | Out-File config.json  # UTF-16 LE!
# Config file is now mixed encoding or entirely UTF-16
```

**Solution - Consistent encoding wrapper:**
```powershell
# Create functions that enforce consistent encoding
function Read-JsonFile {
    param([string]$Path)
    Get-Content -Path $Path -Encoding UTF8 -Raw | ConvertFrom-Json
}

function Write-JsonFile {
    param([string]$Path, [object]$Object)
    $json = $Object | ConvertTo-Json -Depth 10
    if ($PSVersionTable.PSVersion.Major -lt 6) {
        # Windows PowerShell 5.1 - Use .NET for BOM-less UTF-8
        $utf8NoBom = New-Object System.Text.UTF8Encoding $false
        [System.IO.File]::WriteAllText($Path, $json, $utf8NoBom)
    } else {
        # PowerShell 7+ - Native UTF-8 without BOM
        $json | Out-File -Path $Path -Encoding utf8NoBOM
    }
}
```

#### Registry and Environment Variable Encoding

Registry values and environment variables have their own encoding considerations:

**Edge Case: Registry string encoding**
```powershell
# Registry stores strings as UTF-16 internally
# But PowerShell may misinterpret when reading/writing

# This may fail for non-ASCII characters in Windows PowerShell 5.1
Set-ItemProperty -Path "HKCU:\Environment" -Name "TEST_VAR" -Value "Ümläut"
$value = Get-ItemProperty -Path "HKCU:\Environment" -Name "TEST_VAR"
# Value may be corrupted depending on console encoding
```

**Edge Case: Environment variable expansion**
```powershell
# Windows PowerShell 5.1 - Environment variable encoding
$env:PATH  # UTF-16 internally, converted to current encoding
# Can fail if PATH contains non-ASCII folder names and encoding mismatches
```

**Solution:**
```powershell
# Explicitly handle encoding when working with registry/environment
[Environment]::SetEnvironmentVariable("TEST_VAR", "Ümläut", "User")
$value = [Environment]::GetEnvironmentVariable("TEST_VAR", "User")
```

#### Network Stream Encoding

HTTP and other network streams require explicit encoding handling:

**Edge Case: Invoke-WebRequest response encoding**
```powershell
# Windows PowerShell 5.1 - May misdetect encoding
$response = Invoke-WebRequest -Uri "https://example.com/api"
$response.Content  # May be garbled if server doesn't send charset header

# Content-Type: application/json (without charset) defaults to ISO-8859-1
```

**Solution:**
```powershell
# Force UTF-8 decoding
$response = Invoke-WebRequest -Uri "https://example.com/api"
$bytes = $response.RawContentStream.ToArray()
$content = [System.Text.Encoding]::UTF8.GetString($bytes)

# Or use Invoke-RestMethod which handles JSON encoding better
$data = Invoke-RestMethod -Uri "https://example.com/api"
```

**Edge Case: WebSocket and TCP stream encoding**
```powershell
# TCP streams default to ASCII in Windows PowerShell 5.1
$tcpClient = New-Object System.Net.Sockets.TcpClient("example.com", 80)
$stream = $tcpClient.GetStream()
$writer = New-Object System.IO.StreamWriter($stream)  # Default encoding!
$writer.WriteLine("GET / HTTP/1.1")  # ASCII only
```

**Solution:**
```powershell
# Explicitly specify UTF-8 for network streams
$writer = New-Object System.IO.StreamWriter($stream, [System.Text.Encoding]::UTF8)
```

#### Cross-Platform Script Encoding

Scripts executed across different PowerShell versions need careful encoding:

**Edge Case: Script file encoding**
```powershell
# A script saved as UTF-8 with BOM works in both versions
# A script saved as UTF-8 without BOM may fail in Windows PowerShell 5.1
# if it contains non-ASCII characters in the first line

# This fails in Windows PowerShell 5.1 if file is UTF-8 without BOM:
# #!/usr/bin/env pwsh
# Write-Host "日本語"
```

**Solution - Script encoding best practices:**
1. Save scripts as UTF-8 with BOM for Windows PowerShell 5.1 compatibility
2. Or use ASCII-only in the first line and handle encoding in script:

```powershell
# First line ASCII-safe
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
# Now safe to use Unicode
Write-Host "日本語"
```

#### Remote Execution Encoding

When Keystone Core executes commands remotely, encoding must be handled carefully:

**Edge Case: Remote command output corruption**
```powershell
# Command output may be corrupted if:
# 1. Remote system uses different code page
# 2. NATS message encoding differs from output encoding
# 3. Control plane assumes different encoding than agent
```

**Keystone Core configuration for consistent remote encoding:**
```yaml
# agent.yaml
execution:
  powershell:
    encoding: utf8
    # Force UTF-8 console encoding before command execution
    preamble: |
      [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
      $OutputEncoding = [System.Text.Encoding]::UTF8

    # Force UTF-8 for input as well
    input_encoding: utf8
```

#### Encoding Diagnostic Commands

Use these commands to diagnose encoding issues:

```powershell
# Check current encoding settings
Write-Host "Console Output Encoding: $([Console]::OutputEncoding.EncodingName)"
Write-Host "Console Input Encoding: $([Console]::InputEncoding.EncodingName)"
Write-Host "`$OutputEncoding: $($OutputEncoding.EncodingName)"
Write-Host "Default Encoding: $([System.Text.Encoding]::Default.EncodingName)"
Write-Host "Code Page: $(chcp)"

# Test round-trip encoding
$testString = "Test: 日本語 Ümläut 中文 한글"
Write-Host "Original: $testString"
$bytes = [System.Text.Encoding]::UTF8.GetBytes($testString)
$decoded = [System.Text.Encoding]::UTF8.GetString($bytes)
Write-Host "Round-trip: $decoded"
Write-Host "Match: $($testString -eq $decoded)"
```

#### Common Encoding Pitfalls Summary

| Pitfall | Symptom | Solution |
|---------|---------|----------|
| BOM in JSON files | Parse errors, "unexpected character" | Use UTF-8 without BOM |
| Mixed cmdlet encodings | File corruption over time | Use consistent `-Encoding` parameter |
| Console/output mismatch | Garbled characters in output | Set `[Console]::OutputEncoding` |
| Native command output | Wrong characters from external commands | Match encoding to command's code page |
| Script file encoding | Syntax errors on Unicode lines | Save as UTF-8 with BOM or ASCII-first-line |
| Network stream encoding | Corrupted HTTP responses | Explicitly decode as UTF-8 |
| Registry value encoding | Corrupted values | Use .NET methods directly |

### Installing PowerShell 7

If you need PowerShell 7 features:

```powershell
# Using winget (Windows 10/11)
winget install Microsoft.PowerShell

# Using MSI (download from GitHub)
# https://github.com/PowerShell/PowerShell/releases

# Using Chocolatey
choco install powershell-core

# Verify installation
pwsh --version
```

### Remote Command Execution

Keystone Core executes commands on Windows agents using PowerShell. Consider these factors:

**Default Shell Selection:**
```yaml
# agent.yaml
execution:
  # Default shell for command execution
  default_shell: powershell  # Options: cmd, powershell, pwsh

  powershell:
    # Prefer PowerShell 7 (pwsh) over Windows PowerShell
    prefer_pwsh: true

    # Maximum script execution time
    timeout: 300s

    # Working directory for scripts
    working_dir: C:\Windows\Temp
```

**PowerShell-specific commands:**
```bash
# Execute PowerShell command
kscorectl exec run --shell powershell "Get-Process | Sort-Object CPU -Descending | Select-Object -First 10"

# Execute with PowerShell 7 specifically
kscorectl exec run --shell pwsh "Get-Process | ForEach-Object -Parallel { $_.Name }"
```

### Troubleshooting PowerShell Issues

**Issue: Script execution blocked by policy**
```
Error: File cannot be loaded because running scripts is disabled on this system
```

**Solution:**
```powershell
# Check policy
Get-ExecutionPolicy -List

# Set appropriate policy
Set-ExecutionPolicy RemoteSigned -Scope LocalMachine -Force
```

**Issue: Encoding problems in output**
```
Output contains unexpected characters or garbled text
```

**Solution:**
```powershell
# Set console encoding
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# Or use PowerShell 7+ which defaults to UTF-8
```

**Issue: PowerShell Core not found**
```
Error: pwsh.exe not found
```

**Solution:**
```powershell
# Check if PowerShell Core is installed
Get-Command pwsh -ErrorAction SilentlyContinue

# Fall back to Windows PowerShell in agent config
# execution.powershell.prefer_pwsh: false
```

**Issue: WMI commands fail in PowerShell 7**
```
Error: The term 'Get-WmiObject' is not recognized
```

**Solution:**
```powershell
# Use CIM cmdlets instead (work in both versions)
Get-CimInstance -ClassName Win32_OperatingSystem  # Instead of Get-WmiObject
```

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

The agent logs to the Windows Event Log, making it easy to monitor using standard Windows tools and integrate with SIEM solutions.

### Event Source Configuration

| Property | Value | Description |
|----------|-------|-------------|
| Source | KeystoneCore | Registered event source name |
| Log | Application | Default log channel |
| Provider GUID | {A1B2C3D4-...} | ETW provider GUID |
| Manifest | kscore-events.man | Event manifest file |

**Custom Log Channel (Optional):**

For high-volume environments, create a dedicated log channel:

```powershell
# Create custom log channel (requires Administrator)
wevtutil create-log KeystoneCore /lf:"%SystemRoot%\System32\Winevt\Logs\KeystoneCore.evtx" /retention:true /autobackup:true

# Or via registry
New-Item -Path "HKLM:\SYSTEM\CurrentControlSet\Services\EventLog\KeystoneCore" -Force
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\EventLog\KeystoneCore" -Name "File" -Value "%SystemRoot%\System32\Winevt\Logs\KeystoneCore.evtx"
```

Configure agent to use custom channel:
```yaml
# agent.yaml
logging:
  output: eventlog
  eventlog:
    log_name: KeystoneCore  # Custom channel
    source: KeystoneCore
```

### Complete Event ID Reference

#### Informational Events (1000-1099)

| Event ID | Name | Description | Data Fields |
|----------|------|-------------|-------------|
| 1001 | AgentStarted | Agent service started | Version, ConfigPath, AgentID |
| 1002 | AgentStopped | Agent service stopped | Uptime, Reason |
| 1003 | Connected | Connected to control plane | ServerURL, Latency |
| 1004 | Disconnected | Disconnected from control plane | Reason, ReconnectDelay |
| 1005 | CommandExecuted | Command executed successfully | CommandID, Duration, ExitCode |
| 1006 | StateApplied | State applied successfully | StateFile, ChangedResources |
| 1007 | HeartbeatSent | Heartbeat sent to control plane | Sequence, ResponseTime |
| 1008 | MetadataUpdated | Agent metadata refreshed | FactCount, Duration |
| 1009 | ConfigReloaded | Configuration reloaded | PreviousHash, NewHash |
| 1010 | RegistrationComplete | Agent registration completed | ServerID, Capabilities |

#### Warning Events (2000-2099)

| Event ID | Name | Description | Data Fields |
|----------|------|-------------|-------------|
| 2001 | ConnectionRetry | Retrying connection to control plane | Attempt, MaxAttempts, Delay |
| 2002 | HeartbeatMissed | Heartbeat response timeout | LastHeartbeat, Timeout |
| 2003 | CommandTimeout | Command execution timeout | CommandID, Timeout, PID |
| 2004 | ResourceDrift | State drift detected | Resource, Expected, Actual |
| 2005 | HighMemory | High memory usage detected | UsedMB, ThresholdMB |
| 2006 | HighCPU | High CPU usage detected | UsagePercent, ThresholdPercent |
| 2007 | CertExpiring | Certificate expiring soon | CertPath, ExpiryDate, DaysLeft |
| 2008 | SlowCommand | Command execution slow | CommandID, Duration, Threshold |
| 2009 | QueueBacklog | Message queue backlog | QueueName, Depth, Threshold |
| 2010 | DiskSpaceLow | Low disk space on agent | Drive, FreeGB, ThresholdGB |

#### Error Events (3000-3099)

| Event ID | Name | Description | Data Fields |
|----------|------|-------------|-------------|
| 3001 | ConnectionFailed | Failed to connect to control plane | ServerURL, Error |
| 3002 | CommandFailed | Command execution failed | CommandID, ExitCode, Stderr |
| 3003 | StateFailed | State application failed | StateFile, Resource, Error |
| 3004 | ConfigError | Configuration error | ConfigPath, Error, Line |
| 3005 | TLSError | TLS/certificate error | CertPath, Error |
| 3006 | DiskError | Disk I/O error | Path, Operation, Error |
| 3007 | NetworkError | Network communication error | Endpoint, Error |
| 3008 | PluginError | Plugin execution error | Plugin, Error |
| 3009 | CrashRecovery | Recovered from crash | PreviousPID, CrashTime, Reason |
| 3010 | RegistrationFailed | Agent registration failed | ServerURL, Error |

#### Security Events (4000-4099)

| Event ID | Name | Description | Data Fields |
|----------|------|-------------|-------------|
| 4001 | AuthSuccess | Authentication successful | User, Method, ServerID |
| 4002 | AuthFailure | Authentication failed | User, Method, Error |
| 4003 | PolicyViolation | Policy violation detected | Policy, Action, Resource |
| 4004 | UnauthorizedCommand | Unauthorized command blocked | Command, User, Policy |
| 4005 | CertRenewal | Certificate renewed | OldExpiry, NewExpiry |
| 4006 | CredentialRotation | Credentials rotated | CredentialType |
| 4007 | AuditLogExport | Audit log exported | Destination, RecordCount |
| 4008 | TamperedConfig | Configuration tampering detected | ConfigPath, ExpectedHash, ActualHash |
| 4009 | PrivilegeEscalation | Privilege escalation attempted | User, TargetPrivilege |
| 4010 | SecureBootViolation | Secure boot validation failed | ExpectedHash, ActualHash |

### Event Message Structure

Each event includes structured data for parsing:

```xml
<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
  <System>
    <Provider Name="KeystoneCore" Guid="{A1B2C3D4-...}"/>
    <EventID>1005</EventID>
    <Level>4</Level>
    <Task>100</Task>
    <Keywords>0x80000000000000</Keywords>
    <TimeCreated SystemTime="2025-01-15T10:30:00.000Z"/>
    <EventRecordID>12345</EventRecordID>
    <Channel>Application</Channel>
    <Computer>WEB-SERVER-01</Computer>
  </System>
  <EventData>
    <Data Name="CommandID">cmd-abc123</Data>
    <Data Name="Duration">1.234s</Data>
    <Data Name="ExitCode">0</Data>
    <Data Name="AgentID">web-server-01</Data>
    <Data Name="CorrelationID">corr-xyz789</Data>
  </EventData>
</Event>
```

### Viewing and Querying Events

#### PowerShell Queries

```powershell
# View recent agent events
Get-EventLog -LogName Application -Source KeystoneCore -Newest 50

# Filter by event type
Get-EventLog -LogName Application -Source KeystoneCore -EntryType Error

# Filter by date range
Get-EventLog -LogName Application -Source KeystoneCore -After (Get-Date).AddHours(-24)

# Filter by specific Event ID
Get-WinEvent -FilterHashtable @{
    LogName = 'Application'
    ProviderName = 'KeystoneCore'
    ID = 3001, 3002, 3003  # Connection and command failures
}

# Export to CSV
Get-EventLog -LogName Application -Source KeystoneCore |
  Select-Object TimeGenerated, EventID, EntryType, Message |
  Export-Csv -Path agent-events.csv -NoTypeInformation

# Export to JSON (better for SIEM)
Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='KeystoneCore'} |
  Select-Object TimeCreated, Id, LevelDisplayName, Message |
  ConvertTo-Json |
  Out-File -FilePath agent-events.json
```

#### Advanced Filtering with XPath

```powershell
# Find all failed commands in last hour
$query = @"
<QueryList>
  <Query Id="0" Path="Application">
    <Select Path="Application">
      *[System[Provider[@Name='KeystoneCore'] and
        (EventID=3002) and
        TimeCreated[timediff(@SystemTime) &lt;= 3600000]]]
    </Select>
  </Query>
</QueryList>
"@
Get-WinEvent -FilterXml $query

# Find policy violations for specific resource
$query = @"
<QueryList>
  <Query Id="0" Path="Application">
    <Select Path="Application">
      *[System[Provider[@Name='KeystoneCore'] and EventID=4003] and
        EventData[Data[@Name='Resource']='nginx_config']]
    </Select>
  </Query>
</QueryList>
"@
Get-WinEvent -FilterXml $query
```

#### Event Viewer Custom Views

Create a custom view for Keystone Core events:

```xml
<!-- Save as KeystoneCore.xml and import into Event Viewer -->
<ViewerConfig>
  <QueryConfig>
    <QueryParams>
      <Simple>
        <Channel>Application</Channel>
        <EventId>1001-1099,2001-2099,3001-3099,4001-4099</EventId>
        <Source>KeystoneCore</Source>
        <RelativeTimeInfo>0</RelativeTimeInfo>
      </Simple>
    </QueryParams>
    <QueryNode>
      <Name>Keystone Core Events</Name>
    </QueryNode>
  </QueryConfig>
</ViewerConfig>
```

Import via Event Viewer:
1. Open Event Viewer (`eventvwr.msc`)
2. Right-click Custom Views → Import Custom View
3. Select the XML file

### SIEM Integration

#### Windows Event Forwarding (WEF)

Forward Keystone Core events to a central collector:

**Collector Configuration:**
```powershell
# Enable Windows Event Collector service
wecutil qc

# Create subscription
$subscription = @"
<Subscription xmlns="http://schemas.microsoft.com/2006/03/windows/events/subscription">
  <SubscriptionId>KeystoneCore-Events</SubscriptionId>
  <SubscriptionType>SourceInitiated</SubscriptionType>
  <Description>Collect Keystone Core agent events</Description>
  <Enabled>true</Enabled>
  <Uri>http://schemas.microsoft.com/wbem/wsman/1/windows/EventLog</Uri>
  <ConfigurationMode>Normal</ConfigurationMode>
  <Query>
    <![CDATA[
      <QueryList>
        <Query Path="Application">
          <Select>*[System[Provider[@Name='KeystoneCore']]]</Select>
        </Query>
      </QueryList>
    ]]>
  </Query>
  <ReadExistingEvents>false</ReadExistingEvents>
  <TransportName>HTTP</TransportName>
  <ContentFormat>Events</ContentFormat>
  <Locale Language="en-US"/>
  <LogFile>ForwardedEvents</LogFile>
  <CredentialsType>Default</CredentialsType>
</Subscription>
"@
$subscription | Out-File subscription.xml
wecutil cs subscription.xml
```

**Source (Agent) Configuration:**
```powershell
# Configure agent machine to forward events
winrm quickconfig
wevtutil sl ForwardedEvents /ca:O:BAG:SYD:(A;;0x1;;;LA)
```

#### Splunk Integration

Configure Splunk Universal Forwarder:

```ini
# inputs.conf
[WinEventLog://Application]
index = main
sourcetype = WinEventLog:Application
whitelist = KeystoneCore
renderXml = true
```

**Splunk Search Queries:**
```spl
# All Keystone Core events
index=main sourcetype="WinEventLog:Application" source="WinEventLog:Application" KeystoneCore

# Failed commands
index=main sourcetype="WinEventLog:Application" EventCode=3002

# Security events
index=main sourcetype="WinEventLog:Application" EventCode>=4000 EventCode<=4099

# Dashboard: Events by severity over time
index=main sourcetype="WinEventLog:Application" KeystoneCore
| timechart count by Type
```

#### Elastic/ELK Integration

Configure Winlogbeat:

```yaml
# winlogbeat.yml
winlogbeat.event_logs:
  - name: Application
    event_id: 1001-1099, 2001-2099, 3001-3099, 4001-4099
    providers:
      - KeystoneCore
    processors:
      - add_tags:
          tags: [keystone-core]

output.elasticsearch:
  hosts: ["https://elasticsearch:9200"]
  index: "keystone-logs-%{+yyyy.MM.dd}"
```

**Kibana Queries:**
```
winlog.provider_name: "KeystoneCore" AND winlog.event_id: [3001 TO 3099]
```

#### Azure Sentinel Integration

Use Azure Monitor Agent to collect events:

```json
{
  "dataCollectionRules": {
    "keystoneCore": {
      "dataSources": {
        "windowsEventLogs": [{
          "name": "KeystoneCoreEvents",
          "streams": ["Microsoft-WindowsEvent"],
          "xPathQueries": [
            "Application!*[System[Provider[@Name='KeystoneCore']]]"
          ]
        }]
      },
      "destinations": {
        "logAnalytics": [{
          "name": "KeystoneCoreWorkspace",
          "workspaceResourceId": "/subscriptions/.../workspaces/kscore"
        }]
      }
    }
  }
}
```

**KQL Queries in Sentinel:**
```kusto
Event
| where Source == "KeystoneCore"
| where EventID between (3000 .. 3099)
| summarize count() by EventID, Computer
| order by count_ desc
```

### Event Retention and Sizing

Configure event log retention based on compliance requirements:

```powershell
# View current log settings
wevtutil gl Application

# Configure log size and retention
wevtutil sl Application /ms:104857600  # 100MB max size
wevtutil sl Application /rt:true       # Enable retention (overwrite when full)

# For custom KeystoneCore channel
wevtutil sl KeystoneCore /ms:52428800   # 50MB
wevtutil sl KeystoneCore /rt:true
wevtutil sl KeystoneCore /ab:true       # Auto-backup when full
```

**Sizing Guidelines:**

| Deployment Size | Events/Day | Log Size | Retention |
|-----------------|------------|----------|-----------|
| Small (<10 agents) | ~1,000 | 10MB | 30 days |
| Medium (10-100 agents) | ~10,000 | 50MB | 14 days |
| Large (100+ agents) | ~100,000 | 100MB+ | 7 days |

**Event sizes by type:**
- Informational: ~500 bytes average
- Warning: ~1KB average
- Error: ~2KB average (includes stack traces)
- Security: ~1KB average

### Troubleshooting Event Logging

**Problem: Events not appearing in Event Log**

```powershell
# Check if event source is registered
Get-EventLog -List | Where-Object { $_.Log -eq 'Application' }
Get-ChildItem "HKLM:\SYSTEM\CurrentControlSet\Services\EventLog\Application\KeystoneCore" -ErrorAction SilentlyContinue

# Register event source manually
New-EventLog -LogName Application -Source KeystoneCore

# Check agent logging configuration
Get-Content "C:\ProgramData\kscore\agent.yaml" | Select-String "logging"
```

**Problem: Event Log full**

```powershell
# Check log status
wevtutil gl Application

# Clear old events (backup first!)
wevtutil cl Application /bu:C:\Backup\Application.evtx

# Increase log size
wevtutil sl Application /ms:209715200  # 200MB
```

**Problem: Events truncated**

Event messages over 32KB are truncated. Configure agent to split large events:

```yaml
# agent.yaml
logging:
  eventlog:
    max_message_size: 30000  # Bytes
    split_large_messages: true
```

**Problem: Event source conflicts**

```powershell
# Check for conflicts
Get-WinEvent -ListProvider * | Where-Object { $_.Name -like "*keystone*" }

# Remove conflicting source
Remove-EventLog -Source "KeystoneCore-Old"
```

### Event Log Monitoring

Monitor the health of event logging itself:

```powershell
# Check for dropped events
$stats = Get-WinEvent -ListLog Application
Write-Host "Events: $($stats.RecordCount)"
Write-Host "Max Size: $($stats.MaximumSizeInBytes / 1MB) MB"
Write-Host "File Size: $((Get-Item $stats.LogFilePath).Length / 1MB) MB"
Write-Host "Retention: $($stats.LogMode)"

# Alert if log is near capacity
if ($stats.IsLogFull) {
    Write-Warning "Event log is full - events may be lost!"
}
```

**Prometheus/Grafana monitoring (via windows_exporter):**

```promql
# Event log records
windows_eventlog_records_total{log="Application"}

# Events by level
rate(windows_eventlog_events_total{log="Application",source="KeystoneCore"}[5m])
```

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

Antivirus software can interfere with Keystone agent operations through file scanning, behavior monitoring, and network inspection. Proper exclusion configuration ensures reliable operation.

### Symptoms of Antivirus Interference

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| Agent fails to start | Binary quarantined | Add process exclusion |
| Slow command execution | Real-time file scanning | Add path exclusions |
| Connection timeouts | Network inspection | Add network exclusions |
| Config changes reverted | Behavior blocking | Whitelist agent actions |
| High CPU usage | Repeated scanning | Add file exclusions |
| Intermittent failures | Heuristic detection | Add hash exclusion |

### Required Exclusions

Configure these exclusions in your antivirus software:

**Process Exclusions**:
| Process | Path |
|---------|------|
| Agent executable | `C:\Program Files\kscore\kscore-agent.exe` |
| TinyGo WASM runtime | `C:\Program Files\kscore\kscore-wasm.exe` |
| CLI tool | `C:\Program Files\kscore\kscorectl.exe` |

**Path Exclusions**:
| Path | Contents |
|------|----------|
| `C:\Program Files\kscore\` | Program binaries, modules |
| `C:\ProgramData\kscore\` | Configuration, state, cache |
| `C:\ProgramData\kscore\logs\` | Log files |
| `C:\ProgramData\kscore\modules\` | WASM modules |
| `C:\ProgramData\kscore\cache\` | Temporary files |

**Extension Exclusions**:
| Extension | Purpose |
|-----------|---------|
| `.wasm` | WebAssembly modules |
| `.yaml`, `.yml` | Configuration files |

### Windows Defender

**PowerShell Configuration**:
```powershell
# Add comprehensive exclusions
# Process exclusions
Add-MpPreference -ExclusionProcess "kscore-agent.exe"
Add-MpPreference -ExclusionProcess "kscore-wasm.exe"
Add-MpPreference -ExclusionProcess "kscorectl.exe"

# Path exclusions
Add-MpPreference -ExclusionPath "C:\Program Files\kscore"
Add-MpPreference -ExclusionPath "C:\ProgramData\kscore"

# Extension exclusions
Add-MpPreference -ExclusionExtension ".wasm"

# Verify exclusions
Get-MpPreference | Select-Object -Property Exclusion*
```

**Group Policy Configuration**:
```
Computer Configuration
└── Administrative Templates
    └── Windows Components
        └── Microsoft Defender Antivirus
            └── Exclusions
                ├── Path Exclusions: C:\Program Files\kscore;C:\ProgramData\kscore
                ├── Process Exclusions: kscore-agent.exe;kscore-wasm.exe;kscorectl.exe
                └── Extension Exclusions: .wasm
```

**Intune/Endpoint Manager**:
```json
{
  "defenderExcludedPaths": [
    "C:\\Program Files\\kscore",
    "C:\\ProgramData\\kscore"
  ],
  "defenderExcludedProcesses": [
    "kscore-agent.exe",
    "kscore-wasm.exe",
    "kscorectl.exe"
  ],
  "defenderExcludedExtensions": [
    ".wasm"
  ]
}
```

### CrowdStrike Falcon

**Sensor Exclusions** (via Falcon console):

1. Navigate to **Configuration** > **Prevention Policies**
2. Select your policy and click **Edit**
3. Under **Sensor Visibility Exclusions**:
   - Add path: `C:\Program Files\kscore\**`
   - Add path: `C:\ProgramData\kscore\**`
4. Under **Machine Learning Exclusions**:
   - Add hash exclusion for signed agent binary

**PowerShell (requires Falcon toolkit)**:
```powershell
# Install Falcon toolkit
Install-Module -Name PSFalcon

# Authenticate
Request-FalconToken -ClientId $ClientId -ClientSecret $ClientSecret

# Create exclusion
New-FalconMLExclusion -Value "C:\Program Files\kscore\kscore-agent.exe" `
    -ExcludedFrom "blocking"
```

**Response Prevention Exclusions**:
```
Pattern: C:\Program Files\kscore\**
Pattern Type: Glob
Apply To: All sensor groups (or specific groups)
```

### Symantec Endpoint Protection

**SEPM Console**:

1. Navigate to **Policies** > **Exception Policy**
2. Create new exception policy or edit existing
3. Add exclusions:

| Type | Value |
|------|-------|
| Folder | `C:\Program Files\kscore\` |
| Folder | `C:\ProgramData\kscore\` |
| File | `kscore-agent.exe` |
| Extension | `.wasm` |

**Command Line (SEPM 14.3+)**:
```batch
rem Add file exception
sepmcli -addexclusion -type file -path "C:\Program Files\kscore\kscore-agent.exe"

rem Add folder exception
sepmcli -addexclusion -type folder -path "C:\Program Files\kscore"
sepmcli -addexclusion -type folder -path "C:\ProgramData\kscore"
```

### McAfee/Trellix Endpoint Security

**ePO Console**:

1. Navigate to **Menu** > **Policy** > **Policy Catalog**
2. Select **Endpoint Security Threat Prevention**
3. Edit your policy
4. Under **On-Access Scan** > **Exclusions**:

```
Exclusion Type: File path
Pattern: C:\Program Files\kscore\**\*
Applies to: On-access scan, On-demand scan

Exclusion Type: Process
Pattern: kscore-agent.exe
Applies to: All scans
```

**ExtraDAT Exclusion**:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ExtraDAT>
  <Exclusion type="path">C:\Program Files\kscore\*</Exclusion>
  <Exclusion type="path">C:\ProgramData\kscore\*</Exclusion>
  <Exclusion type="process">kscore-agent.exe</Exclusion>
</ExtraDAT>
```

### Trend Micro Apex One

**Apex One Console**:

1. Navigate to **Policies** > **Policy Management**
2. Select target policy and click **Configure**
3. Under **Scan Exclusions**:

| Category | Value |
|----------|-------|
| Directories | `C:\Program Files\kscore\` |
| Directories | `C:\ProgramData\kscore\` |
| Files | `kscore-agent.exe` |
| Extensions | `wasm` |

**Apex One Server Script**:
```powershell
# Using Apex One API
$exclusions = @{
    directories = @(
        "C:\Program Files\kscore\",
        "C:\ProgramData\kscore\"
    )
    files = @("kscore-agent.exe")
    extensions = @("wasm")
}

Invoke-RestMethod -Uri "https://apex-server/api/v1/exclusions" `
    -Method POST `
    -Headers @{Authorization = "Bearer $token"} `
    -Body ($exclusions | ConvertTo-Json)
```

### SentinelOne

**Console Configuration**:

1. Navigate to **Sentinels** > **Exclusions**
2. Create new exclusion:

| Field | Value |
|-------|-------|
| Type | Path |
| Path | `C:\Program Files\kscore\**` |
| OS | Windows |
| Mode | Suppress |

**API Configuration**:
```powershell
$headers = @{
    "Authorization" = "APIToken $ApiToken"
    "Content-Type" = "application/json"
}

$body = @{
    filter = @{
        siteIds = @("site-id")
    }
    data = @{
        osType = "windows"
        pathExclusionType = "subfolders"
        value = "C:\Program Files\kscore\"
        mode = "suppress"
    }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod -Uri "https://console.sentinelone.net/web/api/v2.1/exclusions" `
    -Method POST `
    -Headers $headers `
    -Body $body
```

### Sophos Endpoint

**Sophos Central**:

1. Navigate to **Global Settings** > **Global Exclusions**
2. Add exclusions:

```
Type: Detected threats (PUAs and Malware)
Condition: Path equals
Value: C:\Program Files\kscore\

Type: Detected threats (PUAs and Malware)
Condition: Process name equals
Value: kscore-agent.exe
```

**Sophos Enterprise Console (On-Premises)**:
```
Policy: Anti-virus and HIPS
On-access scanning exclusions:
- Exclude: C:\Program Files\kscore\
- Exclude: C:\ProgramData\kscore\
```

### Carbon Black (VMware)

**CB Defense Console**:

1. Navigate to **Enforce** > **Policies**
2. Select policy and edit
3. Under **Sensor Settings** > **Bypass Rules**:

```yaml
rule_name: "Keystone Agent"
operation: bypass
path: "C:\\Program Files\\kscore\\**"
```

**CB Response/EDR**:
```
Watchlist Exclusion:
process_name:kscore-agent.exe OR path:C:\Program Files\kscore\*
Action: Allow
```

### ESET Endpoint Security

**ESET PROTECT Console**:

1. Navigate to **Policies** > Create new or edit existing
2. Under **Detection Engine** > **Exclusions**:

| Type | Path/Pattern |
|------|--------------|
| Path | `C:\Program Files\kscore\*.*` |
| Path | `C:\ProgramData\kscore\*.*` |
| Process | `kscore-agent.exe` |

**ESET Command Line**:
```batch
rem Add exclusion via ESET command line
ecmd /setvalue antivirusexclusions=C:\Program Files\kscore\*.*
```

### Kaspersky Endpoint Security

**KSC Console**:

1. Navigate to **Policies** > select Windows policy
2. **Application Settings** > **Essential Threat Protection** > **File Threat Protection**
3. Under **Exclusions**:

```
Object type: File or folder
Path: C:\Program Files\kscore\**
Path: C:\ProgramData\kscore\**

Object type: Process
Process: kscore-agent.exe
```

### Deploying Exclusions via Group Policy

For enterprise deployments, use GPO to deploy exclusions centrally:

**PowerShell Startup Script**:
```powershell
# deploy-av-exclusions.ps1
# Deploy as Computer Startup Script via GPO

$ErrorActionPreference = "SilentlyContinue"

# Windows Defender
if (Get-Command Add-MpPreference -ErrorAction SilentlyContinue) {
    Add-MpPreference -ExclusionPath "C:\Program Files\kscore"
    Add-MpPreference -ExclusionPath "C:\ProgramData\kscore"
    Add-MpPreference -ExclusionProcess "kscore-agent.exe"
    Add-MpPreference -ExclusionProcess "kscore-wasm.exe"
    Add-MpPreference -ExclusionProcess "kscorectl.exe"
    Add-MpPreference -ExclusionExtension ".wasm"
}

# Log deployment
$logPath = "C:\Windows\Logs\kscore-av-exclusions.log"
"$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') - AV exclusions deployed" | Out-File $logPath -Append
```

**GPO ADMX Template**:
```xml
<?xml version="1.0" encoding="utf-8"?>
<policyDefinitions>
  <policyDefinition name="KeystoneExclusions" displayName="Keystone AV Exclusions">
    <elements>
      <list id="ExcludedPaths" valueName="ExcludedPaths">
        <item value="C:\Program Files\kscore" />
        <item value="C:\ProgramData\kscore" />
      </list>
      <list id="ExcludedProcesses" valueName="ExcludedProcesses">
        <item value="kscore-agent.exe" />
        <item value="kscore-wasm.exe" />
      </list>
    </elements>
  </policyDefinition>
</policyDefinitions>
```

### Verifying Exclusions

**Test Script**:
```powershell
# verify-av-exclusions.ps1
function Test-AVExclusions {
    $results = @()

    # Test Windows Defender exclusions
    $mpPref = Get-MpPreference -ErrorAction SilentlyContinue
    if ($mpPref) {
        $results += [PSCustomObject]@{
            AV = "Windows Defender"
            PathExclusions = $mpPref.ExclusionPath -contains "C:\Program Files\kscore"
            ProcessExclusions = $mpPref.ExclusionProcess -contains "kscore-agent.exe"
        }
    }

    # Test file write performance (indicator of scanning)
    $testFile = "C:\ProgramData\kscore\test-write.tmp"
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    1..100 | ForEach-Object { [System.IO.File]::WriteAllText($testFile, "test") }
    $stopwatch.Stop()
    Remove-Item $testFile -ErrorAction SilentlyContinue

    $results += [PSCustomObject]@{
        Test = "Write Performance"
        Duration = "$($stopwatch.ElapsedMilliseconds)ms"
        Status = if ($stopwatch.ElapsedMilliseconds -lt 1000) { "Good" } else { "Slow (possible scanning)" }
    }

    return $results
}

Test-AVExclusions | Format-Table -AutoSize
```

### Monitoring AV Interference

**Event Log Monitoring**:
```powershell
# Monitor for AV blocking events
$events = Get-WinEvent -FilterHashtable @{
    LogName = 'Microsoft-Windows-Windows Defender/Operational'
    Id = 1116, 1117  # Detection, Block
    StartTime = (Get-Date).AddDays(-1)
} | Where-Object {
    $_.Message -like '*kscore*'
}

if ($events) {
    Write-Warning "AV interference detected:"
    $events | Format-List TimeCreated, Message
}
```

**Agent Health Check**:
```yaml
# Keystone state to verify AV exclusions
av_exclusion_check:
  module: command
  command: |
    $issues = @()

    # Check Windows Defender
    $mp = Get-MpPreference
    if ($mp.ExclusionPath -notcontains "C:\Program Files\kscore") {
        $issues += "Missing path exclusion: C:\Program Files\kscore"
    }
    if ($mp.ExclusionProcess -notcontains "kscore-agent.exe") {
        $issues += "Missing process exclusion: kscore-agent.exe"
    }

    if ($issues) {
        throw "AV exclusion issues: $($issues -join '; ')"
    }

    "All AV exclusions configured correctly"
  schedule: "@daily"
```

## UAC (User Account Control) Handling

User Account Control (UAC) protects Windows systems by requiring elevation for administrative tasks. Understanding UAC interaction is critical for reliable Keystone operations.

### UAC Overview

UAC operates at several levels:

| Level | Setting | Behavior |
|-------|---------|----------|
| 4 | Always notify | Prompts for all changes |
| 3 | Notify for apps (default) | Prompts when apps make changes |
| 2 | Notify (no dim) | Prompts without secure desktop |
| 1 | Notify for non-Windows | Prompts for non-Windows binaries |
| 0 | Never notify | UAC disabled (not recommended) |

**Check current UAC level**:
```powershell
$uacLevel = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System"
[PSCustomObject]@{
    EnableLUA = $uacLevel.EnableLUA
    ConsentPromptBehaviorAdmin = $uacLevel.ConsentPromptBehaviorAdmin
    PromptOnSecureDesktop = $uacLevel.PromptOnSecureDesktop
}
```

### Agent Service and UAC

The Keystone agent runs as a Windows service under the SYSTEM account by default. Services running as SYSTEM:

- Are **not affected** by UAC prompts
- Have full administrative privileges
- Can perform all system operations without elevation
- Cannot interact with desktop sessions

**Verify service account**:
```powershell
$svc = Get-WmiObject Win32_Service -Filter "Name='kscore-agent'"
$svc.StartName
# Should show: LocalSystem
```

### Command Execution and UAC

When the agent executes commands, UAC behavior depends on the execution context:

#### SYSTEM Context (Default)

Commands run as SYSTEM bypass UAC entirely:

```yaml
# agent.yaml - Commands run elevated automatically
execution:
  default_user: "SYSTEM"  # Default
```

**Capabilities**:
- Full registry access (HKLM, HKCR)
- Access to all files and folders
- Service management
- System configuration changes
- No UAC prompts

**Limitations**:
- Cannot access user-specific resources (HKCU, user profiles)
- Cannot interact with user desktop
- Network access uses computer credentials

#### User Context Execution

To run commands as a specific user:

```yaml
# agent.yaml
execution:
  # Run as specific user (requires credentials)
  run_as_user:
    username: "DOMAIN\\AdminUser"
    password_env: "KSCORE_RUN_AS_PASSWORD"  # Environment variable
```

**UAC Considerations for User Context**:
```yaml
# Commands requiring elevation need special handling
state:
  install_app:
    module: command
    command: |
      Start-Process -FilePath "installer.exe" -ArgumentList "/S" -Verb RunAs -Wait
    # -Verb RunAs triggers elevation
```

### Handling Elevation Requirements

#### Method 1: Run Entire Agent as SYSTEM (Recommended)

```powershell
# Ensure service runs as LocalSystem
sc.exe config kscore-agent obj= "LocalSystem"
Restart-Service kscore-agent
```

All commands execute with full privileges, no UAC prompts.

#### Method 2: Scheduled Task with Highest Privileges

For commands that must run in user context but need elevation:

```powershell
# Create scheduled task with highest privileges
$action = New-ScheduledTaskAction -Execute "powershell.exe" `
    -Argument "-ExecutionPolicy Bypass -File C:\Scripts\elevated-task.ps1"

$principal = New-ScheduledTaskPrincipal -UserId "DOMAIN\AdminUser" `
    -LogonType Password `
    -RunLevel Highest

$task = New-ScheduledTask -Action $action -Principal $principal

Register-ScheduledTask -TaskName "KeystoneElevatedTask" -InputObject $task `
    -Password "SecurePassword"

# Run the task
Start-ScheduledTask -TaskName "KeystoneElevatedTask"
```

**In Keystone state**:
```yaml
elevated_task:
  module: command
  command: |
    $taskExists = Get-ScheduledTask -TaskName "KsElevated-$($env:KSCORE_JOB_ID)" -ErrorAction SilentlyContinue
    if (-not $taskExists) {
        $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-File C:\task.ps1"
        $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -RunLevel Highest
        Register-ScheduledTask -TaskName "KsElevated-$($env:KSCORE_JOB_ID)" -Action $action -Principal $principal
    }
    Start-ScheduledTask -TaskName "KsElevated-$($env:KSCORE_JOB_ID)"
    # Wait and cleanup
    while ((Get-ScheduledTask -TaskName "KsElevated-$($env:KSCORE_JOB_ID)").State -eq 'Running') {
        Start-Sleep -Seconds 1
    }
    Unregister-ScheduledTask -TaskName "KsElevated-$($env:KSCORE_JOB_ID)" -Confirm:$false
```

#### Method 3: PowerShell Remoting to Localhost

Use PowerShell remoting to execute elevated commands:

```powershell
# Enable PSRemoting if not already enabled
Enable-PSRemoting -Force

# Execute elevated command via remoting
Invoke-Command -ComputerName localhost -ScriptBlock {
    # This runs elevated
    Install-WindowsFeature -Name Web-Server
} -Credential (Get-Credential)
```

**Configuration for unattended remoting**:
```powershell
# Allow unattended remoting with CredSSP
Enable-WSManCredSSP -Role Server -Force
Enable-WSManCredSSP -Role Client -DelegateComputer "localhost" -Force
```

### Common UAC Scenarios

#### Installing Software

```yaml
# Software installation (runs as SYSTEM, no UAC issues)
install_software:
  module: package
  name: "google-chrome"
  provider: chocolatey
  state: present
```

For MSI installers:
```yaml
install_msi:
  module: command
  command: |
    Start-Process msiexec.exe -ArgumentList "/i", "C:\installers\app.msi", "/qn" -Wait -NoNewWindow
  # msiexec runs elevated when called by SYSTEM service
```

#### Registry Modifications

```yaml
# HKLM modifications (requires elevation, automatic with SYSTEM)
registry_setting:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows\\WindowsUpdate\\AU"
  name: "NoAutoUpdate"
  value: 1
  type: dword

# HKCU modifications (requires user context)
user_registry:
  module: command
  command: |
    # Run as specific user to access HKCU
    $script = {
        Set-ItemProperty -Path "HKCU:\Software\MyApp" -Name "Setting" -Value "Value"
    }
    Invoke-Command -ScriptBlock $script -Credential $using:cred
```

#### Windows Features

```yaml
# Installing Windows features (requires elevation)
web_server:
  module: command
  command: |
    Install-WindowsFeature -Name Web-Server -IncludeManagementTools
  # Works automatically when agent runs as SYSTEM
```

#### Service Management

```yaml
# Managing services (requires elevation)
manage_service:
  module: service
  name: "Spooler"
  state: stopped
  enabled: false
  # Service module automatically has elevation via agent service
```

### UAC and Remote Desktop

When managing systems where users are logged in via RDP:

```yaml
# Detect active RDP sessions
check_sessions:
  module: command
  command: |
    query session | Where-Object { $_ -match "rdp-tcp" }
  register: rdp_sessions

# Optionally notify users before changes
notify_users:
  module: command
  command: |
    msg * "System maintenance in 5 minutes. Please save your work."
  when: rdp_sessions.stdout != ""
```

### UAC Bypass Considerations

**Security Warning**: Never disable UAC in production environments. Instead:

1. **Use SYSTEM service account** - Commands run elevated without prompts
2. **Use scheduled tasks** - For user-context elevation
3. **Use PowerShell remoting** - For remote elevation
4. **Request elevated credentials** - When user interaction is acceptable

### UAC Troubleshooting

#### "Access Denied" Despite Running as SYSTEM

```powershell
# Verify process elevation
[Security.Principal.WindowsPrincipal]::new(
    [Security.Principal.WindowsIdentity]::GetCurrent()
).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

# Check process token
whoami /priv
```

**Common causes**:
- File system virtualization redirecting writes
- Registry virtualization
- Protected folders (Program Files, Windows)
- Windows Resource Protection (WRP) files

#### Commands Hanging (Waiting for UAC Prompt)

```yaml
# Add timeout to prevent indefinite waits
command_with_timeout:
  module: command
  command: "installer.exe /S"
  timeout: "300s"

  # Better: Ensure silent/unattended mode
  # Most installers support /S, /silent, /quiet, or /qn
```

#### Detecting if Elevation is Needed

```powershell
# Check if specific action requires elevation
function Test-ElevationRequired {
    param([string]$Path)

    try {
        $acl = Get-Acl $Path
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)

        foreach ($access in $acl.Access) {
            if ($access.IdentityReference -eq $identity.Name) {
                return -not $access.FileSystemRights.HasFlag([Security.AccessControl.FileSystemRights]::Write)
            }
        }
        return $true  # No explicit permission, likely needs elevation
    }
    catch {
        return $true  # Error accessing ACL, likely needs elevation
    }
}

Test-ElevationRequired "C:\Windows\System32"
```

### UAC Group Policy Settings

For enterprise deployments, configure UAC via Group Policy:

| Policy | Path | Recommendation |
|--------|------|----------------|
| Enable UAC | `Computer\Policies\Windows Settings\Security Settings\Local Policies\Security Options\User Account Control: Run all administrators in Admin Approval Mode` | Enabled |
| Admin consent prompt | `...\User Account Control: Behavior of the elevation prompt for administrators` | Prompt for consent on secure desktop |
| Auto-elevate signed binaries | `...\User Account Control: Only elevate executables that are signed and validated` | Enabled |

**Export/Import UAC settings**:
```powershell
# Export
secedit /export /cfg C:\uac-settings.inf /areas SECURITYPOLICY

# Import
secedit /configure /db secedit.sdb /cfg C:\uac-settings.inf /areas SECURITYPOLICY
```

### Best Practices Summary

1. **Run agent as SYSTEM** - Avoids all UAC issues for administrative tasks
2. **Use silent installers** - Always use `/S`, `/silent`, or `/qn` flags
3. **Never prompt for UAC** - Ensure all operations can run unattended
4. **Test in UAC-enabled environment** - Don't disable UAC for testing
5. **Handle virtualization** - Be aware of registry/file system virtualization for 32-bit apps
6. **Document elevation needs** - Mark states that require elevation in comments

## Registry State Module Examples

The registry module manages Windows registry keys and values declaratively. This section provides comprehensive examples for common registry management scenarios.

### Module Reference

**Module**: `registry`

**Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Registry path (e.g., `HKLM:\SOFTWARE\MyApp`) |
| `name` | string | No | Value name (omit for default value) |
| `value` | any | No | Value data |
| `type` | string | No | Value type: `string`, `dword`, `qword`, `binary`, `multistring`, `expandstring` |
| `state` | string | No | `present` (default) or `absent` |
| `view` | string | No | Registry view: `default`, `32bit`, `64bit` |
| `recurse` | bool | No | Delete key recursively (for `absent` state) |

**Supported Root Keys**:

| Alias | Full Path |
|-------|-----------|
| `HKLM:` | `HKEY_LOCAL_MACHINE` |
| `HKCU:` | `HKEY_CURRENT_USER` |
| `HKCR:` | `HKEY_CLASSES_ROOT` |
| `HKU:` | `HKEY_USERS` |
| `HKCC:` | `HKEY_CURRENT_CONFIG` |

### Basic Examples

#### Create Registry Key

```yaml
# Create a registry key (no values)
app_key:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  state: present
```

#### Set String Value

```yaml
# Set a string (REG_SZ) value
app_version:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "Version"
  value: "1.0.0"
  type: string
```

#### Set DWORD Value

```yaml
# Set a DWORD (32-bit integer) value
max_connections:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "MaxConnections"
  value: 100
  type: dword
```

#### Set QWORD Value

```yaml
# Set a QWORD (64-bit integer) value
max_file_size:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "MaxFileSize"
  value: 10737418240  # 10 GB
  type: qword
```

#### Set Binary Value

```yaml
# Set binary data
encryption_key:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "EncryptionIV"
  value: "48656C6C6F"  # Hex-encoded bytes
  type: binary
```

#### Set Multi-String Value

```yaml
# Set multiple strings (REG_MULTI_SZ)
allowed_hosts:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "AllowedHosts"
  value:
    - "server1.example.com"
    - "server2.example.com"
    - "server3.example.com"
  type: multistring
```

#### Set Expandable String Value

```yaml
# Set expandable string (REG_EXPAND_SZ) with environment variables
log_path:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "LogPath"
  value: "%ProgramData%\\MyApp\\Logs"
  type: expandstring
```

#### Delete Registry Value

```yaml
# Remove a specific value
remove_old_setting:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\MyApp"
  name: "DeprecatedSetting"
  state: absent
```

#### Delete Registry Key

```yaml
# Remove entire key and subkeys
remove_old_app:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyCompany\\OldApp"
  state: absent
  recurse: true
```

### Windows Configuration Examples

#### Disable Windows Update Auto-Restart

```yaml
# Prevent automatic restarts during active hours
no_auto_restart:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows\\WindowsUpdate\\AU"
  name: "NoAutoRebootWithLoggedOnUsers"
  value: 1
  type: dword

active_hours_start:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\WindowsUpdate\\UX\\Settings"
  name: "ActiveHoursStart"
  value: 8
  type: dword

active_hours_end:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\WindowsUpdate\\UX\\Settings"
  name: "ActiveHoursEnd"
  value: 18
  type: dword
```

#### Configure Remote Desktop

```yaml
# Enable Remote Desktop
enable_rdp:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Terminal Server"
  name: "fDenyTSConnections"
  value: 0
  type: dword

# Allow RDP through Windows Firewall (via registry)
rdp_firewall:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Services\\SharedAccess\\Parameters\\FirewallPolicy\\FirewallRules"
  name: "RemoteDesktop-UserMode-In-TCP"
  value: "v2.31|Action=Allow|Active=TRUE|Dir=In|Protocol=6|LPort=3389|App=System|Name=Remote Desktop - User Mode (TCP-In)|Desc=Inbound rule for the Remote Desktop service to allow RDP traffic.|EmbedCtxt=Remote Desktop|"
  type: string

# Require Network Level Authentication
rdp_nla:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Terminal Server\\WinStations\\RDP-Tcp"
  name: "UserAuthentication"
  value: 1
  type: dword
```

#### Harden SSH Server (OpenSSH)

```yaml
# Configure OpenSSH server settings
sshd_config_key:
  module: registry
  path: "HKLM:\\SOFTWARE\\OpenSSH"
  state: present

sshd_default_shell:
  module: registry
  path: "HKLM:\\SOFTWARE\\OpenSSH"
  name: "DefaultShell"
  value: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
  type: string

sshd_shell_args:
  module: registry
  path: "HKLM:\\SOFTWARE\\OpenSSH"
  name: "DefaultShellCommandOption"
  value: "/c"
  type: string
```

#### Configure TLS/SSL Settings

```yaml
# Disable TLS 1.0
disable_tls10_server:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.0\\Server"
  name: "Enabled"
  value: 0
  type: dword

disable_tls10_client:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.0\\Client"
  name: "Enabled"
  value: 0
  type: dword

# Disable TLS 1.1
disable_tls11_server:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.1\\Server"
  name: "Enabled"
  value: 0
  type: dword

disable_tls11_client:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.1\\Client"
  name: "Enabled"
  value: 0
  type: dword

# Enable TLS 1.2
enable_tls12_server:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.2\\Server"
  name: "Enabled"
  value: 1
  type: dword

enable_tls12_server_default:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.2\\Server"
  name: "DisabledByDefault"
  value: 0
  type: dword

# Enable TLS 1.3 (Windows Server 2022+)
enable_tls13_server:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\SecurityProviders\\SCHANNEL\\Protocols\\TLS 1.3\\Server"
  name: "Enabled"
  value: 1
  type: dword
```

#### Configure Windows Defender

```yaml
# Configure real-time protection
defender_realtime:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows Defender\\Real-Time Protection"
  name: "DisableRealtimeMonitoring"
  value: 0
  type: dword

# Configure scan schedule
defender_scan_day:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows Defender\\Scan"
  name: "ScheduleDay"
  value: 0  # Every day
  type: dword

defender_scan_time:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows Defender\\Scan"
  name: "ScheduleTime"
  value: 120  # 2:00 AM (minutes from midnight)
  type: dword
```

#### Configure Power Settings

```yaml
# Prevent sleep on AC power
power_no_sleep:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Power\\PowerSettings\\238C9FA8-0AAD-41ED-83F4-97BE242C8F20\\29f6c1db-86da-48c5-9fdb-f2b67b1f44da"
  name: "ACSettingIndex"
  value: 0
  type: dword

# Disable hibernate
disable_hibernate:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Power"
  name: "HibernateEnabled"
  value: 0
  type: dword
```

### Application Configuration Examples

#### Configure IIS Settings

```yaml
# Set IIS request timeout
iis_timeout:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\InetStp\\Configuration"
  name: "MaxGlobalBandwidth"
  value: 0  # Unlimited
  type: dword

# Configure ASP.NET
aspnet_max_request:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\ASP.NET\\4.0.30319.0"
  name: "MaxRequestLength"
  value: 4096  # KB
  type: dword
```

#### Configure SQL Server

```yaml
# Set SQL Server max memory
sql_max_memory:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\Microsoft SQL Server\\MSSQL15.MSSQLSERVER\\MSSQLServer"
  name: "MaxServerMemory"
  value: 8192  # MB
  type: dword

# Enable TCP/IP protocol
sql_tcpip:
  module: registry
  path: "HKLM:\\SOFTWARE\\Microsoft\\Microsoft SQL Server\\MSSQL15.MSSQLSERVER\\MSSQLServer\\SuperSocketNetLib\\Tcp"
  name: "Enabled"
  value: 1
  type: dword
```

#### Configure Java

```yaml
# Set JAVA_HOME in system environment
java_home_key:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\Environment"
  name: "JAVA_HOME"
  value: "C:\\Program Files\\Java\\jdk-17"
  type: expandstring

# Add Java to PATH
update_path:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\Environment"
  name: "Path"
  value: "%JAVA_HOME%\\bin;{{ facts.registry['HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\Environment'].Path }}"
  type: expandstring
```

### Security Hardening Examples

#### Disable SMBv1

```yaml
disable_smbv1:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Services\\LanmanServer\\Parameters"
  name: "SMB1"
  value: 0
  type: dword

disable_smbv1_client:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Services\\mrxsmb10"
  name: "Start"
  value: 4  # Disabled
  type: dword
```

#### Disable LLMNR

```yaml
disable_llmnr:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows NT\\DNSClient"
  name: "EnableMulticast"
  value: 0
  type: dword
```

#### Configure Audit Policies

```yaml
# Enable audit for login events
audit_logon:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Lsa"
  name: "AuditLogonEvents"
  value: 3  # Success and failure
  type: dword

# Enable audit for privilege use
audit_privilege:
  module: registry
  path: "HKLM:\\SYSTEM\\CurrentControlSet\\Control\\Lsa"
  name: "FullPrivilegeAuditing"
  value: 1
  type: binary
```

#### Disable Guest Account

```yaml
disable_guest:
  module: registry
  path: "HKLM:\\SAM\\SAM\\Domains\\Account\\Users\\000001F5"
  name: "F"
  value: "{{ facts.registry['HKLM:\\SAM\\SAM\\Domains\\Account\\Users\\000001F5'].F | set_bit(1, 1) }}"
  type: binary
  # Note: This requires special permissions to access SAM
```

### 32-bit vs 64-bit Registry Views

On 64-bit Windows, 32-bit applications see a virtualized registry view:

```yaml
# Write to 64-bit registry (default)
setting_64bit:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "Setting"
  value: "64-bit value"
  view: 64bit

# Write to 32-bit registry (WOW6432Node)
setting_32bit:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "Setting"
  value: "32-bit value"
  view: 32bit

# Write to both views
setting_both_64:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "Setting"
  value: "value"
  view: 64bit

setting_both_32:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "Setting"
  value: "value"
  view: 32bit
```

### Conditional Registry Changes

```yaml
# Only apply on specific Windows version
windows_11_setting:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "Windows11Feature"
  value: 1
  type: dword
  when: facts.os.build >= 22000

# Only apply on domain-joined computers
domain_setting:
  module: registry
  path: "HKLM:\\SOFTWARE\\Policies\\MyApp"
  name: "DomainMode"
  value: 1
  type: dword
  when: facts.domain.joined

# Only if value doesn't exist
create_if_missing:
  module: registry
  path: "HKLM:\\SOFTWARE\\MyApp"
  name: "InitialSetup"
  value: 1
  type: dword
  create_only: true
```

### Managing File Associations

```yaml
# Create custom file association
myapp_extension:
  module: registry
  path: "HKCR:\\.myapp"
  name: "(Default)"
  value: "MyApp.Document"
  type: string

myapp_progid:
  module: registry
  path: "HKCR:\\MyApp.Document"
  name: "(Default)"
  value: "MyApp Document"
  type: string

myapp_icon:
  module: registry
  path: "HKCR:\\MyApp.Document\\DefaultIcon"
  name: "(Default)"
  value: "C:\\Program Files\\MyApp\\myapp.exe,0"
  type: string

myapp_command:
  module: registry
  path: "HKCR:\\MyApp.Document\\shell\\open\\command"
  name: "(Default)"
  value: "\"C:\\Program Files\\MyApp\\myapp.exe\" \"%1\""
  type: string
```

### Backup and Restore

```yaml
# Export registry before changes
backup_registry:
  module: command
  command: |
    $backupPath = "C:\Backups\Registry\$(Get-Date -Format 'yyyyMMdd-HHmmss')"
    New-Item -Path $backupPath -ItemType Directory -Force
    reg export "HKLM\SOFTWARE\MyApp" "$backupPath\myapp.reg" /y
  before:
    - myapp_setting

# Restore from backup (example)
restore_registry:
  module: command
  command: |
    $latestBackup = Get-ChildItem "C:\Backups\Registry" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    reg import "$($latestBackup.FullName)\myapp.reg"
  when: restore_requested
```

### Error Handling

```yaml
# Graceful handling for missing keys
check_and_set:
  module: command
  command: |
    try {
        $current = Get-ItemProperty "HKLM:\SOFTWARE\MyApp" -Name "Setting" -ErrorAction Stop
        if ($current.Setting -ne "desired_value") {
            Set-ItemProperty "HKLM:\SOFTWARE\MyApp" -Name "Setting" -Value "desired_value"
            "changed"
        } else {
            "unchanged"
        }
    } catch {
        New-Item "HKLM:\SOFTWARE\MyApp" -Force | Out-Null
        New-ItemProperty "HKLM:\SOFTWARE\MyApp" -Name "Setting" -Value "desired_value" -PropertyType String
        "created"
    }
  register: result
  changed_when: result.stdout != "unchanged"
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

This section covers deploying Keystone agents at scale using enterprise management tools.

### Group Policy Deployment

#### Prerequisites

- Active Directory domain with Group Policy infrastructure
- Network share accessible by computer accounts
- Domain admin or delegated GPO permissions
- Windows Server 2016+ domain controllers

#### Step 1: Prepare the Distribution Point

Create a network share for the MSI and supporting files:

```powershell
# Create distribution share
$sharePath = "\\fileserver\Software\Keystone"
New-Item -Path $sharePath -ItemType Directory -Force

# Copy MSI and transform files
Copy-Item "kscore-agent.msi" -Destination $sharePath
Copy-Item "kscore-agent.mst" -Destination $sharePath  # Optional transform

# Set share permissions (read for Domain Computers)
$acl = Get-Acl $sharePath
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
    "Domain Computers",
    "ReadAndExecute",
    "ContainerInherit,ObjectInherit",
    "None",
    "Allow"
)
$acl.AddAccessRule($rule)
Set-Acl $sharePath $acl

# Verify NTFS permissions
icacls $sharePath
```

#### Step 2: Create MSI Transform (Optional)

Create a transform file for custom settings:

```powershell
# Using Orca (Windows SDK) or WiX Toolset
# Or create via PowerShell with custom properties

# Example: Create properties file for transforms
@"
SERVERURL=nats://nats.example.com:4222
AGENTID=%%COMPUTERNAME%%
LOGLEVEL=info
DATACENTER=us-east-1
ENVIRONMENT=production
"@ | Out-File "$sharePath\install.properties"
```

#### Step 3: Create and Configure GPO

**Using Group Policy Management Console (GPMC)**:

1. Open **Group Policy Management** (gpmc.msc)
2. Right-click your target OU → **Create a GPO in this domain, and Link it here**
3. Name it `Keystone Agent Deployment`
4. Right-click the new GPO → **Edit**

**Configure Software Installation**:

1. Navigate to: `Computer Configuration` → `Policies` → `Software Settings` → `Software Installation`
2. Right-click → **New** → **Package**
3. Browse to: `\\fileserver\Software\Keystone\kscore-agent.msi`
4. Select **Advanced** deployment method
5. On the **Modifications** tab, add your transform file if applicable
6. Click **OK**

**Using PowerShell**:
```powershell
# Import GroupPolicy module
Import-Module GroupPolicy

# Create new GPO
$gpo = New-GPO -Name "Keystone Agent Deployment" -Comment "Deploys Keystone Core agent"

# Link to target OU
$ou = "OU=Servers,DC=example,DC=com"
New-GPLink -Guid $gpo.Id -Target $ou

# Note: Software installation requires GPMC or direct registry/ADM manipulation
# The actual package assignment is typically done via GPMC
```

#### Step 4: Configure Startup Scripts

For more control, use startup scripts instead of MSI deployment:

```powershell
# deploy-keystone.ps1 - GPO Computer Startup Script
param(
    [string]$ServerUrl = "nats://nats.example.com:4222",
    [string]$LogLevel = "info"
)

$ErrorActionPreference = "Stop"
$logFile = "C:\Windows\Logs\keystone-deploy.log"

function Write-Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    "$timestamp - $Message" | Out-File $logFile -Append
}

try {
    Write-Log "Starting Keystone agent deployment"

    # Check if already installed
    $installed = Get-WmiObject Win32_Product -Filter "Name='Keystone Agent'" -ErrorAction SilentlyContinue
    if ($installed) {
        $currentVersion = $installed.Version
        Write-Log "Keystone agent already installed: v$currentVersion"

        # Check for upgrade
        $msiVersion = (Get-Item "\\fileserver\Software\Keystone\kscore-agent.msi").VersionInfo.ProductVersion
        if ([version]$msiVersion -gt [version]$currentVersion) {
            Write-Log "Upgrading from v$currentVersion to v$msiVersion"
        } else {
            Write-Log "No upgrade needed"
            exit 0
        }
    }

    # Install/upgrade agent
    $msiPath = "\\fileserver\Software\Keystone\kscore-agent.msi"
    $arguments = @(
        "/i"
        "`"$msiPath`""
        "/qn"
        "/l*v"
        "C:\Windows\Logs\keystone-msi.log"
        "SERVERURL=$ServerUrl"
        "AGENTID=$env:COMPUTERNAME"
        "LOGLEVEL=$LogLevel"
    )

    Write-Log "Running: msiexec $($arguments -join ' ')"
    $process = Start-Process msiexec -ArgumentList $arguments -Wait -PassThru -NoNewWindow

    if ($process.ExitCode -eq 0) {
        Write-Log "Installation successful"
    } elseif ($process.ExitCode -eq 3010) {
        Write-Log "Installation successful (reboot required)"
    } else {
        Write-Log "Installation failed with exit code: $($process.ExitCode)"
        exit $process.ExitCode
    }

    # Verify service is running
    Start-Sleep -Seconds 5
    $service = Get-Service kscore-agent -ErrorAction SilentlyContinue
    if ($service.Status -eq "Running") {
        Write-Log "Agent service is running"
    } else {
        Write-Log "Warning: Agent service not running"
        Start-Service kscore-agent -ErrorAction SilentlyContinue
    }

} catch {
    Write-Log "Error: $_"
    exit 1
}
```

**Configure GPO for Startup Script**:

1. In GPO Editor: `Computer Configuration` → `Policies` → `Windows Settings` → `Scripts (Startup/Shutdown)`
2. Double-click **Startup**
3. Click **PowerShell Scripts** tab → **Add**
4. Script Name: `\\fileserver\Software\Keystone\deploy-keystone.ps1`
5. Parameters: `-ServerUrl "nats://nats.example.com:4222" -LogLevel "info"`

#### Step 5: Configure Agent via Group Policy Preferences

Use GPP to deploy configuration files:

**Deploy agent.yaml via GPP Files**:

1. Navigate to: `Computer Configuration` → `Preferences` → `Windows Settings` → `Files`
2. Right-click → **New** → **File**
3. Configure:
   - Source file: `\\fileserver\Software\Keystone\config\agent.yaml`
   - Destination: `C:\ProgramData\kscore\agent.yaml`
   - Action: **Replace**

**Deploy Registry Settings via GPP**:

1. Navigate to: `Computer Configuration` → `Preferences` → `Windows Settings` → `Registry`
2. Add custom registry settings for agent configuration

**Example Registry Items**:
| Hive | Path | Name | Type | Value |
|------|------|------|------|-------|
| HKLM | SOFTWARE\Keystone\Agent | ServerUrl | REG_SZ | nats://nats.example.com:4222 |
| HKLM | SOFTWARE\Keystone\Agent | DataCenter | REG_SZ | us-east-1 |
| HKLM | SOFTWARE\Keystone\Agent | Environment | REG_SZ | production |

#### Step 6: Security Filtering

Control which computers receive the deployment:

```powershell
# Add security filtering to GPO
$gpo = Get-GPO -Name "Keystone Agent Deployment"

# Remove Authenticated Users (default)
Set-GPPermission -Guid $gpo.Id -TargetName "Authenticated Users" `
    -TargetType Group -PermissionLevel None

# Add specific security group
Set-GPPermission -Guid $gpo.Id -TargetName "Keystone_Agents" `
    -TargetType Group -PermissionLevel GpoApply

# Add Domain Computers to read (required for processing)
Set-GPPermission -Guid $gpo.Id -TargetName "Domain Computers" `
    -TargetType Group -PermissionLevel GpoRead
```

#### Step 7: WMI Filtering (Optional)

Filter deployment based on system properties:

```powershell
# Create WMI filter for servers only
$wmiFilter = @"
SELECT * FROM Win32_OperatingSystem WHERE ProductType > 1
"@

# Apply to GPO (requires ADSI manipulation)
# ProductType: 1 = Workstation, 2 = Domain Controller, 3 = Server
```

**Common WMI Filter Expressions**:

| Filter | WMI Query |
|--------|-----------|
| Servers only | `SELECT * FROM Win32_OperatingSystem WHERE ProductType > 1` |
| Windows Server 2019+ | `SELECT * FROM Win32_OperatingSystem WHERE Version >= "10.0.17763"` |
| 64-bit only | `SELECT * FROM Win32_OperatingSystem WHERE OSArchitecture = "64-bit"` |
| Physical machines | `SELECT * FROM Win32_ComputerSystem WHERE Model != "Virtual Machine"` |

#### GPO Verification

Verify GPO application on target computers:

```powershell
# On target computer
gpresult /r /scope computer

# Check specific GPO
gpresult /h C:\gpresult.html

# Force GPO refresh
gpupdate /force /target:computer

# Check event logs for GPO processing
Get-WinEvent -FilterHashtable @{
    LogName = 'Microsoft-Windows-GroupPolicy/Operational'
    Id = 4016  # GPO applied
} -MaxEvents 10
```

### SCCM/ConfigMgr Deployment

#### Create Application

1. In SCCM Console: **Software Library** → **Application Management** → **Applications**
2. **Create Application** → **Manually specify the application information**
3. Configure:
   - Name: `Keystone Agent`
   - Publisher: `Anthropic`
   - Software version: `1.0.0`

#### Create Deployment Type

```xml
<!-- Detection Method: Registry -->
<DetectionMethod>
  <RegistryDetection>
    <Hive>HKLM</Hive>
    <KeyPath>SOFTWARE\Keystone\Agent</KeyPath>
    <ValueName>Version</ValueName>
    <ValueType>String</ValueType>
    <Operator>GreaterThanOrEqualTo</Operator>
    <Value>1.0.0</Value>
  </RegistryDetection>
</DetectionMethod>
```

**Installation Command**:
```cmd
msiexec /i "kscore-agent.msi" /qn SERVERURL=nats://nats.example.com:4222 AGENTID=%COMPUTERNAME% /l*v "%TEMP%\kscore-install.log"
```

**Uninstall Command**:
```cmd
msiexec /x {PRODUCT-GUID} /qn
```

**Detection Script (PowerShell)**:
```powershell
# SCCM Detection Script
$service = Get-Service -Name "kscore-agent" -ErrorAction SilentlyContinue
if ($service -and $service.Status -eq "Running") {
    $exe = "C:\Program Files\kscore\kscore-agent.exe"
    if (Test-Path $exe) {
        $version = (Get-Item $exe).VersionInfo.ProductVersion
        if ([version]$version -ge [version]"1.0.0") {
            Write-Host "Keystone Agent $version installed and running"
            exit 0
        }
    }
}
exit 1
```

### Microsoft Intune Deployment

#### Create Win32 App Package

```powershell
# Create .intunewin package
# Requires Microsoft Win32 Content Prep Tool

# 1. Create source folder structure
$sourceFolder = "C:\IntunePackages\Keystone"
New-Item -Path $sourceFolder -ItemType Directory -Force
Copy-Item "kscore-agent.msi" -Destination $sourceFolder
Copy-Item "install.ps1" -Destination $sourceFolder

# 2. Create install.ps1 wrapper
@'
param(
    [string]$ServerUrl = "nats://nats.example.com:4222"
)
$msiPath = Join-Path $PSScriptRoot "kscore-agent.msi"
Start-Process msiexec -ArgumentList "/i `"$msiPath`" /qn SERVERURL=$ServerUrl AGENTID=$env:COMPUTERNAME" -Wait -NoNewWindow
'@ | Out-File "$sourceFolder\install.ps1"

# 3. Run IntuneWinAppUtil.exe
.\IntuneWinAppUtil.exe -c $sourceFolder -s "install.ps1" -o "C:\IntunePackages\Output"
```

#### Configure in Intune

**App Information**:
- Name: `Keystone Agent`
- Publisher: `Anthropic`
- App version: `1.0.0`

**Program**:
- Install command: `powershell.exe -ExecutionPolicy Bypass -File install.ps1 -ServerUrl "nats://nats.example.com:4222"`
- Uninstall command: `msiexec /x {PRODUCT-GUID} /qn`
- Install behavior: `System`

**Detection Rules**:
| Rule Type | Path | Detection Method |
|-----------|------|-----------------|
| File | `C:\Program Files\kscore\kscore-agent.exe` | File or folder exists |
| Registry | `HKLM\SOFTWARE\Keystone\Agent\Version` | String comparison |

**Requirements**:
- OS architecture: `64-bit`
- Minimum OS: `Windows 10 1809`

### MSI Properties Reference

| Property | Description | Default | Example |
|----------|-------------|---------|---------|
| `SERVERURL` | NATS server URL | (required) | `nats://nats.example.com:4222` |
| `AGENTID` | Agent identifier | `%COMPUTERNAME%` | `web-server-01` |
| `LOGLEVEL` | Log verbosity | `info` | `debug`, `warn`, `error` |
| `CONFIGPATH` | Custom config path | `%ProgramData%\kscore\agent.yaml` | `C:\custom\agent.yaml` |
| `DATACENTER` | Datacenter tag | (none) | `us-east-1` |
| `ENVIRONMENT` | Environment tag | (none) | `production` |
| `ROLE` | Role tag | (none) | `webserver` |
| `ENABLETLS` | Enable TLS | `1` | `0` to disable |
| `TLSCERT` | TLS certificate path | (none) | `C:\certs\client.crt` |
| `TLSKEY` | TLS key path | (none) | `C:\certs\client.key` |
| `TLSCA` | TLS CA path | (none) | `C:\certs\ca.crt` |
| `PROXYURL` | HTTP proxy URL | (none) | `http://proxy:8080` |

**Example Installation Commands**:

```cmd
rem Basic installation
msiexec /i kscore-agent.msi /qn SERVERURL=nats://nats.example.com:4222

rem Full configuration
msiexec /i kscore-agent.msi /qn ^
  SERVERURL=nats://nats.example.com:4222 ^
  AGENTID=%COMPUTERNAME% ^
  LOGLEVEL=info ^
  DATACENTER=us-east-1 ^
  ENVIRONMENT=production ^
  ROLE=webserver ^
  ENABLETLS=1 ^
  TLSCERT=C:\certs\client.crt ^
  TLSKEY=C:\certs\client.key ^
  TLSCA=C:\certs\ca.crt

rem With transform file
msiexec /i kscore-agent.msi /qn TRANSFORMS=custom.mst

rem Upgrade with logging
msiexec /i kscore-agent.msi /qn REINSTALL=ALL REINSTALLMODE=vomus /l*v upgrade.log

rem Silent uninstall
msiexec /x {PRODUCT-GUID} /qn
```

### Post-Deployment Verification

**PowerShell Verification Script**:
```powershell
# verify-deployment.ps1
param(
    [string[]]$ComputerNames = @($env:COMPUTERNAME)
)

$results = Invoke-Command -ComputerName $ComputerNames -ScriptBlock {
    [PSCustomObject]@{
        ComputerName = $env:COMPUTERNAME
        AgentInstalled = Test-Path "C:\Program Files\kscore\kscore-agent.exe"
        ServiceRunning = (Get-Service kscore-agent -ErrorAction SilentlyContinue).Status -eq "Running"
        AgentVersion = (Get-Item "C:\Program Files\kscore\kscore-agent.exe" -ErrorAction SilentlyContinue).VersionInfo.ProductVersion
        ConfigExists = Test-Path "C:\ProgramData\kscore\agent.yaml"
        LastEvent = (Get-WinEvent -LogName "Keystone/Operational" -MaxEvents 1 -ErrorAction SilentlyContinue).TimeCreated
    }
}

$results | Format-Table -AutoSize
```

**SCCM Collection Query**:
```sql
SELECT
    R.Name0,
    R.ResourceId,
    ARP.DisplayName0,
    ARP.Version0
FROM
    v_R_System R
    INNER JOIN v_Add_Remove_Programs ARP ON R.ResourceId = ARP.ResourceId
WHERE
    ARP.DisplayName0 = 'Keystone Agent'
```

## Windows Troubleshooting Guide

This section covers common Windows-specific issues and their solutions.

### Service Issues

#### Agent Service Won't Start

**Symptoms**: Service fails to start, Event ID 7000/7009 in Event Log

**Diagnostic Steps**:
```powershell
# Check service status
Get-Service kscore-agent | Format-List *

# View service account
$svc = Get-WmiObject Win32_Service -Filter "Name='kscore-agent'"
$svc.StartName

# Check service dependencies
sc.exe qc kscore-agent

# View recent service events
Get-WinEvent -FilterHashtable @{
    LogName = 'System'
    ProviderName = 'Service Control Manager'
    Id = 7000,7009,7011,7023,7024,7031,7034
} -MaxEvents 20 | Where-Object {
    $_.Message -like '*kscore*'
}
```

**Common Causes and Solutions**:

| Cause | Solution |
|-------|----------|
| Invalid service account | Verify account exists and password is correct |
| Missing permissions | Grant "Log on as a service" right to service account |
| Config file syntax error | Validate YAML with `kscore-agent --validate-config` |
| Port conflict | Check if NATS port (4222) is in use |
| Missing dependencies | Ensure VC++ Redistributable is installed |

**Fix service account permissions**:
```powershell
# Grant "Log on as a service" right
$sid = (New-Object System.Security.Principal.NTAccount("DOMAIN\ServiceAccount")).Translate(
    [System.Security.Principal.SecurityIdentifier]
).Value

$tmp = [System.IO.Path]::GetTempFileName()
secedit /export /cfg $tmp
$config = Get-Content $tmp
$newConfig = $config -replace '(SeServiceLogonRight.*)','$1,' + $sid
$newConfig | Set-Content $tmp
secedit /configure /db secedit.sdb /cfg $tmp
Remove-Item $tmp
```

#### Service Crashes Repeatedly

**Symptoms**: Service starts then stops, Event ID 7034 (unexpected termination)

**Diagnostic Steps**:
```powershell
# Check for crash dumps
Get-ChildItem "$env:LOCALAPPDATA\CrashDumps" -Filter "*kscore*"

# Enable WER (Windows Error Reporting) for detailed dumps
New-Item -Path "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps" -Force
New-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps" `
    -Name "DumpFolder" -Value "C:\Dumps" -PropertyType ExpandString
New-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps" `
    -Name "DumpType" -Value 2 -PropertyType DWord

# View application crash events
Get-WinEvent -FilterHashtable @{
    LogName = 'Application'
    ProviderName = 'Application Error','Windows Error Reporting'
} -MaxEvents 50 | Where-Object {
    $_.Message -like '*kscore*'
}
```

**Common Causes and Solutions**:

| Cause | Solution |
|-------|----------|
| Out of memory | Increase memory limits in config |
| Certificate errors | Verify TLS certificates are valid and accessible |
| Network timeout | Check connectivity to control plane |
| Incompatible PowerShell module | Remove conflicting modules |

#### Service Recovery Configuration

Configure automatic service recovery:

```powershell
# Set recovery options: restart after 1st, 2nd, and 3rd failures
sc.exe failure kscore-agent reset= 86400 actions= restart/60000/restart/60000/restart/60000

# Verify recovery settings
sc.exe qfailure kscore-agent

# Configure with PowerShell (requires NSSM or similar)
$recoveryActions = @(
    (New-Object System.ServiceProcess.ServiceRecoveryAction("Restart", 60000)),
    (New-Object System.ServiceProcess.ServiceRecoveryAction("Restart", 60000)),
    (New-Object System.ServiceProcess.ServiceRecoveryAction("Restart", 120000))
)
```

### Connectivity Issues

#### Agent Can't Connect to Control Plane

**Symptoms**: Agent shows disconnected status, connection timeout errors

**Diagnostic Steps**:
```powershell
# Test network connectivity
Test-NetConnection -ComputerName nats.example.com -Port 4222

# Check DNS resolution
Resolve-DnsName nats.example.com

# Test TLS connection
$tcpClient = New-Object System.Net.Sockets.TcpClient
$tcpClient.Connect("nats.example.com", 4222)
$sslStream = New-Object System.Net.Security.SslStream($tcpClient.GetStream())
$sslStream.AuthenticateAsClient("nats.example.com")
$sslStream.RemoteCertificate | Format-List *

# Check Windows Firewall
Get-NetFirewallRule -DisplayName "*kscore*" | Format-Table Name,Enabled,Direction,Action

# View blocked connections
Get-WinEvent -FilterHashtable @{
    LogName = 'Security'
    Id = 5157
} -MaxEvents 100 | Where-Object {
    $_.Message -like '*kscore*' -or $_.Message -like '*4222*'
}
```

**Firewall Configuration**:
```powershell
# Allow outbound NATS connections
New-NetFirewallRule -DisplayName "Keystone Agent Outbound" `
    -Direction Outbound `
    -Program "C:\Program Files\kscore\kscore-agent.exe" `
    -Action Allow

# Allow specific ports
New-NetFirewallRule -DisplayName "Keystone NATS" `
    -Direction Outbound `
    -Protocol TCP `
    -RemotePort 4222 `
    -Action Allow
```

#### Proxy Configuration

Configure agent to use corporate proxy:

```yaml
# agent.yaml
proxy:
  http_proxy: "http://proxy.example.com:8080"
  https_proxy: "http://proxy.example.com:8080"
  no_proxy: "localhost,127.0.0.1,.internal.example.com"

  # For authenticated proxy
  proxy_auth:
    username: "proxyuser"
    password: "${PROXY_PASSWORD}"  # Use environment variable
```

**Verify proxy connectivity**:
```powershell
# Test proxy connection
$proxy = New-Object System.Net.WebProxy("http://proxy.example.com:8080")
$request = [System.Net.WebRequest]::Create("https://control-plane.example.com")
$request.Proxy = $proxy
$response = $request.GetResponse()
$response.StatusCode
```

### Certificate Issues

#### TLS Certificate Errors

**Symptoms**: "certificate verify failed", "unknown authority" errors

**Diagnostic Steps**:
```powershell
# View certificate chain
$uri = "https://control-plane.example.com:8080"
$request = [System.Net.HttpWebRequest]::Create($uri)
$request.GetResponse() | Out-Null
$cert = $request.ServicePoint.Certificate
$cert2 = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($cert)
$chain = New-Object System.Security.Cryptography.X509Certificates.X509Chain
$chain.Build($cert2) | Out-Null
$chain.ChainElements | ForEach-Object {
    $_.Certificate | Format-List Subject,Issuer,NotAfter
}

# Check if CA is trusted
$caThumbprint = "ABC123..."  # Your CA thumbprint
Get-ChildItem Cert:\LocalMachine\Root | Where-Object {
    $_.Thumbprint -eq $caThumbprint
}
```

**Install custom CA certificate**:
```powershell
# Import CA to trusted root store
$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2("C:\certs\ca.crt")
$store = New-Object System.Security.Cryptography.X509Certificates.X509Store(
    [System.Security.Cryptography.X509Certificates.StoreName]::Root,
    [System.Security.Cryptography.X509Certificates.StoreLocation]::LocalMachine
)
$store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
$store.Add($cert)
$store.Close()

# Verify installation
Get-ChildItem Cert:\LocalMachine\Root | Where-Object {
    $_.Subject -like "*Your CA*"
}
```

#### Client Certificate Authentication

**Symptoms**: "client certificate required" errors

```powershell
# Verify client certificate is installed
Get-ChildItem Cert:\LocalMachine\My | Where-Object {
    $_.Subject -like "*kscore*"
} | Format-List Subject,Thumbprint,NotAfter,HasPrivateKey

# Check certificate private key permissions
$cert = Get-ChildItem Cert:\LocalMachine\My | Where-Object {
    $_.Thumbprint -eq "YOUR_THUMBPRINT"
}
$keyPath = $cert.PrivateKey.CspKeyContainerInfo.UniqueKeyContainerName
$keyFullPath = "$env:ProgramData\Microsoft\Crypto\RSA\MachineKeys\$keyPath"
Get-Acl $keyFullPath | Format-List

# Grant service account access to private key
$acl = Get-Acl $keyFullPath
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
    "NT SERVICE\kscore-agent",
    "Read",
    "Allow"
)
$acl.AddAccessRule($rule)
Set-Acl $keyFullPath $acl
```

### Command Execution Issues

#### Commands Fail with Access Denied

**Symptoms**: Exit code 5 (Access Denied), commands don't execute

**Diagnostic Steps**:
```powershell
# Check service account privileges
whoami /priv

# Verify UAC settings
(Get-ItemProperty HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System).EnableLUA

# Test command as service account
$cred = Get-Credential -UserName "DOMAIN\ServiceAccount" -Message "Enter service account credentials"
Start-Process powershell -Credential $cred -ArgumentList "-Command", "whoami /priv"
```

**Common Solutions**:

1. **Run agent as SYSTEM**:
```powershell
# Change service to LocalSystem
Set-Service -Name kscore-agent -StartupType Automatic
sc.exe config kscore-agent obj= "LocalSystem"
Restart-Service kscore-agent
```

2. **Grant specific privileges**:
```powershell
# Export current security policy
secedit /export /cfg C:\secpol.cfg

# Edit C:\secpol.cfg to add service account to:
# SeBackupPrivilege (Backup files and directories)
# SeRestorePrivilege (Restore files and directories)
# SeDebugPrivilege (Debug programs) - if needed

# Import modified policy
secedit /configure /db secedit.sdb /cfg C:\secpol.cfg
```

#### PowerShell Execution Policy

**Symptoms**: "execution of scripts is disabled" errors

```powershell
# Check current execution policy
Get-ExecutionPolicy -List

# Set execution policy for LocalMachine
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope LocalMachine -Force

# Alternative: Bypass for specific scripts
powershell -ExecutionPolicy Bypass -File "script.ps1"
```

**Configure in agent.yaml**:
```yaml
execution:
  powershell:
    execution_policy: "Bypass"  # RemoteSigned, AllSigned, Bypass
    arguments:
      - "-NoProfile"
      - "-NonInteractive"
```

#### Long-Running Commands Timeout

**Symptoms**: Commands killed after timeout, incomplete operations

```yaml
# agent.yaml - Adjust timeouts
execution:
  default_timeout: "5m"
  max_timeout: "1h"

  # Per-command timeout patterns
  timeout_overrides:
    - pattern: "choco install.*"
      timeout: "30m"
    - pattern: "dism.*"
      timeout: "1h"
    - pattern: "msiexec.*"
      timeout: "20m"
```

### Performance Issues

#### High CPU Usage

**Symptoms**: Agent consuming excessive CPU

**Diagnostic Steps**:
```powershell
# Check agent process
Get-Process -Name kscore-agent | Format-List *

# Monitor CPU over time
$samples = 1..60 | ForEach-Object {
    $cpu = (Get-Process -Name kscore-agent).CPU
    [PSCustomObject]@{
        Time = Get-Date
        CPU = $cpu
    }
    Start-Sleep -Seconds 1
}
$samples | Measure-Object -Property CPU -Average -Maximum

# Check for tight loops in logs
Get-Content "C:\Program Files\kscore\logs\agent.log" -Tail 1000 |
    Select-String -Pattern "error|retry|reconnect"
```

**Common Causes**:

| Cause | Solution |
|-------|----------|
| Rapid reconnection attempts | Check network stability, increase backoff |
| Excessive logging | Reduce log level to `info` |
| Large state applications | Optimize state files, reduce frequency |
| Too many concurrent commands | Adjust `max_concurrent_commands` |

#### High Memory Usage

**Symptoms**: Agent memory grows over time

```powershell
# Monitor memory usage
$samples = 1..60 | ForEach-Object {
    $proc = Get-Process -Name kscore-agent
    [PSCustomObject]@{
        Time = Get-Date
        WorkingSetMB = $proc.WorkingSet64 / 1MB
        PrivateMemoryMB = $proc.PrivateMemorySize64 / 1MB
    }
    Start-Sleep -Seconds 60
}
$samples | Format-Table

# Check for memory leaks
Get-Counter "\Process(kscore-agent)\Private Bytes" -SampleInterval 60 -MaxSamples 60 |
    ForEach-Object { $_.CounterSamples.CookedValue / 1MB }
```

**Memory optimization**:
```yaml
# agent.yaml
resources:
  max_memory_mb: 512
  gc_interval: "5m"

  # Limit command output buffering
  command_output:
    max_buffer_mb: 10
    truncate_at_mb: 5
```

### State Management Issues

#### State Apply Fails

**Symptoms**: State application errors, partial changes

**Diagnostic Steps**:
```powershell
# Run state in check mode
kscorectl state apply --target "this-agent" --check-only

# View detailed state output
kscorectl state apply --target "this-agent" --verbose

# Check module-specific logs
Get-Content "C:\Program Files\kscore\logs\state.log" -Tail 100
```

**Common Module Errors**:

| Module | Error | Solution |
|--------|-------|----------|
| `file` | "Access denied" | Check file/folder permissions |
| `package` | "Chocolatey not found" | Install Chocolatey, add to PATH |
| `service` | "Service not found" | Verify service name |
| `registry` | "Key not found" | Create parent keys first |

#### Registry State Issues

**Symptoms**: Registry changes don't apply, unexpected values

```powershell
# Verify registry permissions
$acl = Get-Acl "HKLM:\SOFTWARE\YourApp"
$acl.Access | Format-Table IdentityReference,RegistryRights,AccessControlType

# Check for WOW6432Node redirection
# 64-bit view
Get-ItemProperty "HKLM:\SOFTWARE\YourApp"

# 32-bit view (redirected)
Get-ItemProperty "HKLM:\SOFTWARE\WOW6432Node\YourApp"
```

**Registry state best practices**:
```yaml
# Explicitly specify registry view
registry_value:
  module: registry
  path: "HKLM:\\SOFTWARE\\YourApp"
  name: "Setting"
  value: "Value"
  type: "String"
  view: "64bit"  # or 32bit, default
```

### Log Analysis

#### Collect Diagnostic Information

```powershell
# Create diagnostic bundle
$diagPath = "C:\kscore-diag-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
New-Item -ItemType Directory -Path $diagPath

# Copy logs
Copy-Item "C:\Program Files\kscore\logs\*" -Destination "$diagPath\logs" -Recurse

# Export service configuration
sc.exe qc kscore-agent > "$diagPath\service-config.txt"
sc.exe qfailure kscore-agent >> "$diagPath\service-config.txt"

# Export Windows Event logs
wevtutil epl Application "$diagPath\Application.evtx"
wevtutil epl System "$diagPath\System.evtx"

# Export security-related events
Get-WinEvent -FilterHashtable @{
    LogName = 'Security'
    Id = 4624,4625,4648,4672,5140,5145
    StartTime = (Get-Date).AddDays(-1)
} | Export-Csv "$diagPath\security-events.csv"

# System information
Get-ComputerInfo | Out-File "$diagPath\system-info.txt"
Get-Service | Out-File "$diagPath\services.txt"
Get-Process | Out-File "$diagPath\processes.txt"
ipconfig /all > "$diagPath\network.txt"
netstat -ano >> "$diagPath\network.txt"

# Compress bundle
Compress-Archive -Path $diagPath -DestinationPath "$diagPath.zip"
Write-Host "Diagnostic bundle created: $diagPath.zip"
```

#### Parse Agent Logs

```powershell
# Parse JSON logs
$logs = Get-Content "C:\Program Files\kscore\logs\agent.log" |
    Where-Object { $_ -ne "" } |
    ForEach-Object { $_ | ConvertFrom-Json }

# Filter errors
$logs | Where-Object { $_.level -eq "error" } |
    Sort-Object timestamp -Descending |
    Select-Object -First 20

# Group by error type
$logs | Where-Object { $_.level -eq "error" } |
    Group-Object message |
    Sort-Object Count -Descending |
    Select-Object Count,Name -First 10

# Find commands with non-zero exit codes
$logs | Where-Object {
    $_.fields.exit_code -ne $null -and $_.fields.exit_code -ne 0
} | Select-Object timestamp,@{N='command';E={$_.fields.command}},@{N='exit_code';E={$_.fields.exit_code}}
```

### Quick Reference: Error Codes

| Exit Code | Meaning | Common Cause |
|-----------|---------|--------------|
| 1 | General error | Check stderr output |
| 2 | Misuse of command | Invalid arguments |
| 3 | Not found | File/command doesn't exist |
| 5 | Access denied | Permission issues |
| 87 | Invalid parameter | Wrong arguments |
| 1053 | Service timeout | Service taking too long to start |
| 1067 | Process terminated | Application crash |
| 1068 | Dependency failed | Required service not running |
| 1069 | Logon failure | Invalid service account |
| 1603 | MSI failed | Installation error |

### Quick Reference: Event IDs

| Event ID | Source | Meaning |
|----------|--------|---------|
| 7000 | SCM | Service failed to start |
| 7009 | SCM | Service start timeout |
| 7011 | SCM | Service hang |
| 7023 | SCM | Service terminated with error |
| 7024 | SCM | Service-specific error |
| 7031 | SCM | Service crashed |
| 7034 | SCM | Service terminated unexpectedly |
| 1000 | Application Error | Application crash |
| 1001 | WER | Windows Error Report |
| 4625 | Security | Failed logon |
| 4688 | Security | Process creation |
| 5157 | Security | Connection blocked |

## See Also

- [Agent Configuration](../deployment/#agent-configuration) - Detailed configuration options
- [Troubleshooting](../troubleshooting/) - General troubleshooting guide
- [Security](../security/) - Security configuration
