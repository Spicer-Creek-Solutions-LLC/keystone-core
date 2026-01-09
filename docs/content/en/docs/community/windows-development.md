---
title: "Windows Development"
weight: 25
description: "Setting up a Windows development environment for Keystone Core"
---

This guide covers setting up a Windows development environment for building and testing Keystone Core.

## Prerequisites

### Required Software

1. **Go 1.25+**
   - Download from https://go.dev/dl/
   - Or use winget: `winget install GoLang.Go`
   - Verify: `go version`

2. **Git for Windows**
   - Download from https://git-scm.com/download/win
   - Or use winget: `winget install Git.Git`
   - Verify: `git --version`

3. **PowerShell 7+** (recommended)
   - Download from https://github.com/PowerShell/PowerShell/releases
   - Or use winget: `winget install Microsoft.PowerShell`
   - Verify: `pwsh --version`

### Optional Tools

4. **.NET SDK 6.0+** (for MSI building)
   - Download from https://dotnet.microsoft.com/download
   - Or use winget: `winget install Microsoft.DotNet.SDK.8`
   - Required for: Building MSI installer

5. **WiX Toolset v4** (for MSI building)
   ```powershell
   dotnet tool install --global wix
   ```

6. **Make for Windows** (optional)
   - Install via Chocolatey: `choco install make`
   - Or use MSYS2/MinGW

## Repository Setup

### Clone the Repository

```powershell
# Clone
git clone https://github.com/shawnbutts/keystone-core.git
cd keystone-core

# Get dependencies
go mod download
```

### Directory Structure

```
keystone-core/
├── cmd/                    # CLI applications
│   ├── kscore-agent/       # Agent binary
│   ├── kscore-server/      # Control plane
│   ├── kscorectl/          # Main CLI
│   └── ...
├── pkg/                    # Library packages
│   ├── agent/              # Agent core
│   ├── statemgmt/          # State management
│   └── ...
├── deploy/
│   └── windows/            # MSI installer project
└── test/
    └── e2e/                # End-to-end tests
```

## Building

### Build All Binaries

```powershell
# Using PowerShell
$env:CGO_ENABLED = '0'
go build -o build/bin/ ./cmd/...

# Or individual components
go build -o build/bin/kscore-agent.exe ./cmd/kscore-agent
go build -o build/bin/kscorectl.exe ./cmd/kscorectl
go build -o build/bin/kscore-server.exe ./cmd/kscore-server
```

### Build for Different Architectures

```powershell
# Windows AMD64 (default)
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -o build/bin/windows-amd64/kscore-agent.exe ./cmd/kscore-agent

# Windows ARM64
$env:GOOS = 'windows'
$env:GOARCH = 'arm64'
go build -o build/bin/windows-arm64/kscore-agent.exe ./cmd/kscore-agent

# Linux (cross-compile)
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -o build/bin/linux-amd64/kscore-agent ./cmd/kscore-agent
```

### Build MSI Installer

```powershell
cd deploy/windows
.\build.ps1

# Or using dotnet directly
dotnet build -c Release
```

## Running Tests

### Run All Unit Tests

```powershell
go test ./...
```

### Run Tests with Verbose Output

```powershell
go test -v ./pkg/statemgmt/...
```

### Run Specific Test

```powershell
go test -v -run "TestWinService" ./pkg/statemgmt/...
```

### Run Tests with Coverage

```powershell
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Windows-Specific Tests

Many Windows tests require actual Windows APIs and skip on other platforms:

```powershell
# These tests run only on Windows
go test -v -run "Windows" ./pkg/statemgmt/...
go test -v -run "Win" ./pkg/agent/...
```

## Debugging

### VS Code Configuration

Create `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Agent",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/kscore-agent",
            "args": ["--config", "config/agent.yaml"],
            "env": {
                "KSCORE_DEBUG": "1"
            }
        },
        {
            "name": "Debug Tests",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/pkg/statemgmt",
            "args": ["-test.v", "-test.run", "TestWinService"]
        }
    ]
}
```

### Using Delve

```powershell
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug agent
dlv debug ./cmd/kscore-agent -- --config config/agent.yaml

# Debug tests
dlv test ./pkg/statemgmt -- -test.run TestWinService
```

## Running the Agent Locally

### Console Mode (Development)

```powershell
# Run agent in console mode (not as service)
.\build\bin\kscore-agent.exe --config config\agent.yaml --console

# With debug logging
$env:KSCORE_LOG_LEVEL = 'debug'
.\build\bin\kscore-agent.exe --console
```

### Service Mode (Testing)

```powershell
# Install as service (requires admin)
.\build\bin\kscore-agent.exe install

# Start service
Start-Service KeystoneCoreAgent

# View service logs
Get-EventLog -LogName Application -Source KeystoneCore -Newest 20

# Stop and remove service
Stop-Service KeystoneCoreAgent
.\build\bin\kscore-agent.exe uninstall
```

## Common Issues

### Long Path Support

Windows has a 260-character path limit by default. Enable long paths:

```powershell
# Run as Administrator
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name LongPathsEnabled -Value 1
```

Or add to Git config:
```powershell
git config --system core.longpaths true
```

### Execution Policy

If PowerShell scripts won't run:

```powershell
# Check current policy
Get-ExecutionPolicy

# Allow local scripts (recommended for development)
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Go Module Cache

If module downloads fail:

```powershell
# Clear module cache
go clean -modcache

# Re-download dependencies
go mod download
```

### Service Registration Fails

If service installation fails:

1. Run PowerShell as Administrator
2. Check for existing service: `Get-Service KeystoneCoreAgent`
3. Remove existing service: `sc.exe delete KeystoneCoreAgent`

### Build Fails with "cannot find package"

```powershell
# Ensure dependencies are downloaded
go mod download

# Verify go.mod is correct
go mod verify

# Update dependencies
go mod tidy
```

## IDE Setup

### Visual Studio Code

1. Install Go extension (`golang.go`)
2. Install recommended extensions:
   - Go Test Explorer
   - PowerShell
   - YAML

3. Create settings (`.vscode/settings.json`):
```json
{
    "go.testFlags": ["-v"],
    "go.buildFlags": ["-tags=windows"],
    "go.lintTool": "golangci-lint",
    "go.lintOnSave": "package"
}
```

### GoLand

1. Open project folder
2. Go to Settings > Go > GOROOT and verify Go installation
3. Go to Settings > Go > Go Modules and enable
4. Run Configuration:
   - Program arguments: `--config config/agent.yaml --console`
   - Working directory: project root

## Contributing Windows Changes

### Testing Windows-Specific Code

1. Write tests that skip on non-Windows:
```go
func TestWindowsFeature(t *testing.T) {
    if runtime.GOOS != "windows" {
        t.Skip("Skipping Windows test on non-Windows")
    }
    // Windows-specific test code
}
```

2. Use build tags for platform-specific files:
```go
//go:build windows

package mypackage
```

3. Create stub files for other platforms:
```go
//go:build !windows

package mypackage

func windowsOnlyFunction() error {
    return fmt.Errorf("not supported on %s", runtime.GOOS)
}
```

### Pull Request Checklist

- [ ] All tests pass: `go test ./...`
- [ ] Cross-compilation works: `GOOS=linux go build ./...`
- [ ] Code follows project style
- [ ] Windows-specific tests have `t.Skip` on other platforms
- [ ] Documentation updated if needed
