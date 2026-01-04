# Keystone Core Agent - Windows MSI Installer

This directory contains the WiX project for building the Windows MSI installer for the Keystone Core Agent.

## Prerequisites

### Build Requirements

1. **Go 1.21+** - For building the agent binaries
   - https://go.dev/dl/

2. **.NET SDK 6.0+** - For running WiX
   - https://dotnet.microsoft.com/download

3. **WiX Toolset v4** - Installed as a .NET tool
   ```powershell
   dotnet tool install --global wix
   ```

### Optional (for UI customization)

- Bitmap images for installer UI:
  - `assets/banner.bmp` - 493x58 pixels
  - `assets/dialog.bmp` - 493x312 pixels
  - `assets/icon.ico` - Application icon

## Building the Installer

### Using PowerShell Script (Recommended)

```powershell
# Build everything (binaries + MSI)
.\build.ps1

# Build Release configuration
.\build.ps1 -Configuration Release

# Skip binary build (use pre-built binaries)
.\build.ps1 -SkipBinaries

# Clean build
.\build.ps1 -Clean
```

### Manual Build

```powershell
# Step 1: Build Windows binaries
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'

go build -o ../../build/bin/windows-amd64/kscore-agent.exe ../../cmd/kscore-agent
go build -o ../../build/bin/windows-amd64/kscorectl.exe ../../cmd/kscorectl
go build -o ../../build/bin/windows-amd64/kscore-state.exe ../../cmd/kscore-state
go build -o ../../build/bin/windows-amd64/kscore-exec.exe ../../cmd/kscore-exec

# Step 2: Build MSI
dotnet build -c Release
```

### Cross-Platform Build (from Linux/macOS)

```bash
# Build Windows binaries
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 make build-windows

# Note: MSI building requires Windows (WiX runs on Windows only)
# Use a Windows CI runner or VM to build the MSI
```

## Output

After building, the MSI is located at:
- Debug: `bin/Debug/kscore-agent.msi`
- Release: `bin/Release/kscore-agent.msi`

## Installation

### Silent Installation

```batch
:: Basic installation
msiexec /i kscore-agent.msi /qn

:: With server URL
msiexec /i kscore-agent.msi /qn SERVERURL=nats://control-plane.example.com:4222

:: With all options
msiexec /i kscore-agent.msi /qn ^
    SERVERURL=nats://control-plane.example.com:4222 ^
    AGENTID=web-server-01 ^
    LOGLEVEL=info ^
    TLSENABLED=1

:: With logging
msiexec /i kscore-agent.msi /qn /l*v install.log SERVERURL=nats://server:4222
```

### Interactive Installation

```batch
:: Standard UI
msiexec /i kscore-agent.msi

:: Basic UI (progress bar only)
msiexec /i kscore-agent.msi /qb
```

### Uninstallation

```batch
:: By product code
msiexec /x {product-code} /qn

:: By MSI file
msiexec /x kscore-agent.msi /qn
```

## Installation Properties

| Property | Description | Default | Example |
|----------|-------------|---------|---------|
| `SERVERURL` | NATS server URL | (none) | `nats://server:4222` |
| `AGENTID` | Agent identifier | Computer name | `web-server-01` |
| `LOGLEVEL` | Log level | `info` | `debug`, `info`, `warn`, `error` |
| `TLSENABLED` | Enable TLS | `0` | `1` |
| `TLSCAPATH` | CA certificate path | (none) | `C:\certs\ca.crt` |

## Installed Components

### Files

| Path | Description |
|------|-------------|
| `C:\Program Files\KeystoneCore\bin\kscore-agent.exe` | Agent executable |
| `C:\Program Files\KeystoneCore\bin\kscorectl.exe` | CLI tool |
| `C:\Program Files\KeystoneCore\bin\kscore-state.exe` | State management plugin |
| `C:\Program Files\KeystoneCore\bin\kscore-exec.exe` | Execution plugin |
| `C:\ProgramData\kscore\agent.yaml` | Configuration file |

### Windows Service

- **Name**: `KeystoneCoreAgent`
- **Display Name**: Keystone Core Agent
- **Start Type**: Automatic
- **Account**: LocalSystem
- **Recovery**: Restart on failure

### Registry Keys

- `HKLM\SOFTWARE\KeystoneCore\Agent` - Installation info
- `HKLM\SOFTWARE\KeystoneCore\Agent\Connection` - Connection settings

### Event Log

- **Source**: KeystoneCore
- **Log**: Application

## Group Policy Deployment

### Using Software Installation

1. Copy `kscore-agent.msi` to a network share accessible by target computers
2. Create a new GPO or edit an existing one
3. Navigate to: Computer Configuration > Policies > Software Settings > Software installation
4. Right-click > New > Package
5. Select the MSI from the network share
6. Choose "Assigned" deployment method
7. In Advanced options, add SERVERURL property transform if needed

### Using PowerShell via GPO

Create a startup script:

```powershell
# install-kscore.ps1
$msiPath = "\\server\share\kscore-agent.msi"
$logPath = "C:\Windows\Temp\kscore-install.log"

$installed = Get-WmiObject -Class Win32_Product | Where-Object { $_.Name -eq "Keystone Core Agent" }

if (-not $installed) {
    Start-Process msiexec.exe -ArgumentList @(
        "/i", "`"$msiPath`"",
        "/qn",
        "/l*v", "`"$logPath`"",
        "SERVERURL=nats://control-plane.example.com:4222"
    ) -Wait
}
```

## SCCM/Intune Deployment

### SCCM Application

1. Create a new Application
2. Detection Method: Registry key exists
   - `HKLM\SOFTWARE\KeystoneCore\Agent\InstallPath`
3. Install Command: `msiexec /i kscore-agent.msi /qn SERVERURL=nats://server:4222`
4. Uninstall Command: `msiexec /x kscore-agent.msi /qn`
5. Install Behavior: Install for system

### Intune Win32 App

1. Package MSI using IntuneWinAppUtil.exe
2. Install Command: `msiexec /i kscore-agent.msi /qn SERVERURL=nats://server:4222`
3. Uninstall Command: `msiexec /x {product-code} /qn`
4. Detection Rule: Registry
   - `HKLM\SOFTWARE\KeystoneCore\Agent`
   - Value: `InstallPath`

## Troubleshooting

### Installation Logs

```batch
:: Generate detailed log
msiexec /i kscore-agent.msi /l*v install.log

:: View service status
sc query KeystoneCoreAgent

:: View event log
Get-EventLog -LogName Application -Source KeystoneCore -Newest 20
```

### Common Issues

1. **Service fails to start**
   - Check `C:\ProgramData\kscore\agent.yaml` configuration
   - Verify NATS server is reachable
   - Check Windows Event Log for errors

2. **Installation fails silently**
   - Run with `/l*v` to generate log
   - Check for existing installation conflicts
   - Verify admin privileges

3. **Upgrade fails**
   - Stop the service first: `net stop KeystoneCoreAgent`
   - Check for file locks in installation directory

## Project Structure

```
deploy/windows/
├── Product.wxs          # Main product definition
├── Files.wxs            # File components
├── Service.wxs          # Windows service configuration
├── Registry.wxs         # Registry entries
├── UI.wxs               # Installation UI (optional)
├── kscore-agent.wixproj # WiX project file
├── build.ps1            # Build script
├── assets/
│   ├── README.txt       # Post-install readme
│   ├── license.rtf      # License for installer UI
│   ├── icon.ico         # Application icon (optional)
│   ├── banner.bmp       # Installer banner (optional)
│   └── dialog.bmp       # Installer dialog background (optional)
└── config/
    └── agent.yaml.template  # Default configuration template
```

## License

Apache License 2.0
