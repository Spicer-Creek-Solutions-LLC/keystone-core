<#
.SYNOPSIS
    Build script for Keystone Core Agent MSI installer.

.DESCRIPTION
    This script builds the Windows MSI installer for the Keystone Core Agent.
    It first builds the Go binaries for Windows, then creates the MSI using WiX.

.PARAMETER Configuration
    Build configuration (Debug or Release). Default: Release

.PARAMETER SkipBinaries
    Skip building Go binaries (use pre-built binaries)

.PARAMETER Clean
    Clean build artifacts before building

.EXAMPLE
    .\build.ps1
    Build MSI with default settings

.EXAMPLE
    .\build.ps1 -Configuration Debug
    Build Debug MSI

.EXAMPLE
    .\build.ps1 -SkipBinaries
    Build MSI using pre-built binaries
#>

param(
    [ValidateSet('Debug', 'Release')]
    [string]$Configuration = 'Release',

    [switch]$SkipBinaries,

    [switch]$Clean
)

$ErrorActionPreference = 'Stop'
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Resolve-Path "$ScriptDir\..\.."
$BuildDir = Join-Path $RootDir 'build\bin\windows-amd64'

Write-Host "Keystone Core Agent MSI Build" -ForegroundColor Cyan
Write-Host "=============================" -ForegroundColor Cyan

# Check prerequisites
function Test-Prerequisites {
    Write-Host "`nChecking prerequisites..." -ForegroundColor Yellow

    # Check .NET SDK
    if (!(Get-Command dotnet -ErrorAction SilentlyContinue)) {
        throw ".NET SDK not found. Install from https://dotnet.microsoft.com/download"
    }
    $dotnetVersion = dotnet --version
    Write-Host "  .NET SDK: $dotnetVersion" -ForegroundColor Green

    # Check WiX (installed as dotnet tool)
    $wixInstalled = dotnet tool list -g | Select-String 'wix'
    if (!$wixInstalled) {
        Write-Host "  WiX not found. Installing..." -ForegroundColor Yellow
        dotnet tool install --global wix
    } else {
        Write-Host "  WiX: Installed" -ForegroundColor Green
    }

    # Check Go (only if building binaries)
    if (!$SkipBinaries) {
        if (!(Get-Command go -ErrorAction SilentlyContinue)) {
            throw "Go not found. Install from https://go.dev/dl/"
        }
        $goVersion = go version
        Write-Host "  Go: $goVersion" -ForegroundColor Green
    }
}

# Build Go binaries for Windows
function Build-Binaries {
    if ($SkipBinaries) {
        Write-Host "`nSkipping binary build (using pre-built binaries)" -ForegroundColor Yellow
        return
    }

    Write-Host "`nBuilding Go binaries for Windows..." -ForegroundColor Yellow

    # Create output directory
    New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

    # Set environment for cross-compilation
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'

    Push-Location $RootDir
    try {
        # Build each component
        $components = @(
            @{ Name = 'kscore-agent'; Path = './cmd/kscore-agent' },
            @{ Name = 'kscorectl'; Path = './cmd/kscorectl' },
            @{ Name = 'kscore-state'; Path = './cmd/kscore-state' },
            @{ Name = 'kscore-exec'; Path = './cmd/kscore-exec' }
        )

        foreach ($comp in $components) {
            Write-Host "  Building $($comp.Name)..." -ForegroundColor Gray
            $output = Join-Path $BuildDir "$($comp.Name).exe"
            go build -ldflags="-s -w" -o $output $comp.Path

            if (!(Test-Path $output)) {
                throw "Failed to build $($comp.Name)"
            }
            $size = (Get-Item $output).Length / 1MB
            Write-Host "    Built: $output ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
        }
    }
    finally {
        Pop-Location
        # Clear environment
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

# Build MSI
function Build-Msi {
    Write-Host "`nBuilding MSI installer..." -ForegroundColor Yellow

    Push-Location $ScriptDir
    try {
        # Verify binaries exist
        $agentExe = Join-Path $BuildDir 'kscore-agent.exe'
        if (!(Test-Path $agentExe)) {
            throw "kscore-agent.exe not found at $agentExe. Run without -SkipBinaries or build manually."
        }

        # Clean if requested
        if ($Clean) {
            Write-Host "  Cleaning build artifacts..." -ForegroundColor Gray
            dotnet clean -c $Configuration
        }

        # Build MSI
        Write-Host "  Running WiX build..." -ForegroundColor Gray
        dotnet build -c $Configuration

        # Check output
        $msiPath = Join-Path $ScriptDir "bin\$Configuration\kscore-agent.msi"
        if (Test-Path $msiPath) {
            $size = (Get-Item $msiPath).Length / 1MB
            Write-Host "`nBuild successful!" -ForegroundColor Green
            Write-Host "  MSI: $msiPath ($([math]::Round($size, 2)) MB)" -ForegroundColor Cyan
        } else {
            throw "MSI not found at expected location: $msiPath"
        }
    }
    finally {
        Pop-Location
    }
}

# Main
try {
    Test-Prerequisites
    Build-Binaries
    Build-Msi

    Write-Host "`nInstallation Examples:" -ForegroundColor Yellow
    Write-Host "  Silent install:" -ForegroundColor Gray
    Write-Host "    msiexec /i kscore-agent.msi /qn SERVERURL=nats://server:4222" -ForegroundColor White
    Write-Host ""
    Write-Host "  With agent ID:" -ForegroundColor Gray
    Write-Host "    msiexec /i kscore-agent.msi /qn SERVERURL=nats://server:4222 AGENTID=web-01" -ForegroundColor White
    Write-Host ""
    Write-Host "  Interactive install:" -ForegroundColor Gray
    Write-Host "    msiexec /i kscore-agent.msi" -ForegroundColor White
}
catch {
    Write-Host "`nBuild failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
