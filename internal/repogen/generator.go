package repogen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Generator creates distribution repositories for Keystone Core.
type Generator struct {
	config *Config
}

// NewGenerator creates a new repository generator.
func NewGenerator(config *Config) *Generator {
	if config.BlueprintsDir == "" {
		config.BlueprintsDir = "examples/blueprints/kscore"
	}
	if config.ModulesDir == "" {
		config.ModulesDir = "examples/modules"
	}
	return &Generator{config: config}
}

// GenerateAll generates all repository types.
func (g *Generator) GenerateAll() error {
	fmt.Printf("Generating all repositories for version %s\n", g.config.Version)
	fmt.Printf("Output directory: %s\n\n", g.config.OutputDir)

	// Ensure output directory exists
	if err := os.MkdirAll(g.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Generate each repository type
	generators := []struct {
		name string
		fn   func() error
	}{
		{"DNF/YUM", g.GenerateDNF},
		{"APT", g.GenerateAPT},
		{"Windows", g.GenerateWindows},
		{"Blueprints", g.GenerateBlueprints},
		{"Modules", g.GenerateModules},
	}

	for _, gen := range generators {
		fmt.Printf("Generating %s repository...\n", gen.name)
		if err := gen.fn(); err != nil {
			return fmt.Errorf("generate %s: %w", gen.name, err)
		}
		fmt.Printf("  ✓ %s repository generated\n", gen.name)
	}

	// Generate master index
	if err := g.generateMasterIndex(); err != nil {
		return fmt.Errorf("generate master index: %w", err)
	}

	fmt.Printf("\nAll repositories generated successfully!\n")
	fmt.Printf("Serve with: cd %s && python3 -m http.server 8080\n", g.config.OutputDir)

	return nil
}

// GenerateDNF generates DNF/YUM repository structure.
func (g *Generator) GenerateDNF() error {
	dnfConfig := DefaultDNFConfig()
	dnfDir := filepath.Join(g.config.OutputDir, "dnf")

	for _, dist := range dnfConfig.Distributions {
		for _, arch := range dnfConfig.Architectures {
			distDir := filepath.Join(dnfDir, dist, arch)

			// Create directory structure
			packagesDir := filepath.Join(distDir, "Packages")
			repodataDir := filepath.Join(distDir, "repodata")

			if err := os.MkdirAll(packagesDir, 0755); err != nil {
				return fmt.Errorf("create packages dir: %w", err)
			}
			if err := os.MkdirAll(repodataDir, 0755); err != nil {
				return fmt.Errorf("create repodata dir: %w", err)
			}

			// Generate .repo file
			repoFile := fmt.Sprintf(`[keystonecore]
name=Keystone Core - %s - %s
baseurl=https://packages.keystonecore.io/dnf/%s/%s/
enabled=1
gpgcheck=%d
gpgkey=https://packages.keystonecore.io/gpg/keystone-core.asc
`, dist, arch, dist, arch, boolToInt(g.config.SignPackages))

			repoFilePath := filepath.Join(distDir, "keystonecore.repo")
			if err := os.WriteFile(repoFilePath, []byte(repoFile), 0644); err != nil {
				return fmt.Errorf("write repo file: %w", err)
			}

			// Generate placeholder repomd.xml (actual RPM generation requires rpm tools)
			repomdContent := g.generateRepomdXML(dist, arch)
			if err := os.WriteFile(filepath.Join(repodataDir, "repomd.xml"), []byte(repomdContent), 0644); err != nil {
				return fmt.Errorf("write repomd.xml: %w", err)
			}

			// Generate README
			readme := fmt.Sprintf(`# Keystone Core DNF Repository - %s/%s

## Installation

### Add Repository (RHEL/CentOS/Rocky/Alma %s)

    sudo curl -o /etc/yum.repos.d/keystonecore.repo \
        https://packages.keystonecore.io/dnf/%s/%s/keystonecore.repo

### Install Packages

    sudo dnf install kscore-server kscore-agent kscorectl

## Available Packages

- kscore-server: Control plane server
- kscore-agent: Agent for managed nodes
- kscorectl: CLI tool

## Version: %s
`, dist, arch, dist[2:], dist, arch, g.config.Version)

			if err := os.WriteFile(filepath.Join(distDir, "README.md"), []byte(readme), 0644); err != nil {
				return fmt.Errorf("write README: %w", err)
			}
		}
	}

	return nil
}

// GenerateAPT generates APT repository structure.
func (g *Generator) GenerateAPT() error {
	aptConfig := DefaultAPTConfig()
	aptDir := filepath.Join(g.config.OutputDir, "apt")

	// Create pool directory
	poolDir := filepath.Join(aptDir, "pool", "main", "k", "keystonecore")
	if err := os.MkdirAll(poolDir, 0755); err != nil {
		return fmt.Errorf("create pool dir: %w", err)
	}

	for _, dist := range aptConfig.Distributions {
		for _, component := range aptConfig.Components {
			for _, arch := range aptConfig.Architectures {
				// Create dist directory structure
				distDir := filepath.Join(aptDir, "dists", dist, component, fmt.Sprintf("binary-%s", arch))
				if err := os.MkdirAll(distDir, 0755); err != nil {
					return fmt.Errorf("create dist dir: %w", err)
				}

				// Generate Packages file (placeholder)
				packagesContent := g.generateAPTPackages(arch)
				if err := os.WriteFile(filepath.Join(distDir, "Packages"), []byte(packagesContent), 0644); err != nil {
					return fmt.Errorf("write Packages: %w", err)
				}
			}

			// Generate Release file for distribution
			releaseContent := g.generateAPTRelease(dist, aptConfig)
			releaseDir := filepath.Join(aptDir, "dists", dist)
			if err := os.WriteFile(filepath.Join(releaseDir, "Release"), []byte(releaseContent), 0644); err != nil {
				return fmt.Errorf("write Release: %w", err)
			}
		}
	}

	// Generate sources.list example
	sourcesList := `# Keystone Core APT Repository
# Add this to /etc/apt/sources.list.d/keystonecore.list

# Ubuntu 24.04 (Noble)
deb https://packages.keystonecore.io/apt noble main

# Ubuntu 22.04 (Jammy)
# deb https://packages.keystonecore.io/apt jammy main

# Debian 12 (Bookworm)
# deb https://packages.keystonecore.io/apt bookworm main

# Debian 13 (Trixie)
# deb https://packages.keystonecore.io/apt trixie main
`
	if err := os.WriteFile(filepath.Join(aptDir, "keystonecore.list"), []byte(sourcesList), 0644); err != nil {
		return fmt.Errorf("write sources.list: %w", err)
	}

	// Generate README
	readme := fmt.Sprintf(`# Keystone Core APT Repository

## Installation

### Import GPG Key

    curl -fsSL https://packages.keystonecore.io/gpg/keystone-core.asc | sudo gpg --dearmor -o /usr/share/keyrings/keystonecore.gpg

### Add Repository

    echo "deb [signed-by=/usr/share/keyrings/keystonecore.gpg] https://packages.keystonecore.io/apt $(lsb_release -cs) main" | \
        sudo tee /etc/apt/sources.list.d/keystonecore.list

### Install Packages

    sudo apt update
    sudo apt install kscore-server kscore-agent kscorectl

## Available Packages

- kscore-server: Control plane server
- kscore-agent: Agent for managed nodes
- kscorectl: CLI tool

## Supported Distributions

- Ubuntu 24.04 (Noble)
- Ubuntu 22.04 (Jammy)
- Debian 12 (Bookworm)
- Debian 13 (Trixie)

## Version: %s
`, g.config.Version)

	if err := os.WriteFile(filepath.Join(aptDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// GenerateWindows generates Windows repository structure.
func (g *Generator) GenerateWindows() error {
	winConfig := DefaultWindowsConfig()
	winDir := filepath.Join(g.config.OutputDir, "windows")

	for _, arch := range winConfig.Architectures {
		archDir := filepath.Join(winDir, arch)
		if err := os.MkdirAll(archDir, 0755); err != nil {
			return fmt.Errorf("create arch dir: %w", err)
		}

		// Generate manifest.json
		manifest := map[string]interface{}{
			"version":      g.config.Version,
			"architecture": arch,
			"packages": []map[string]string{
				{
					"name":        "kscore-server",
					"msi":         fmt.Sprintf("kscore-server-%s-%s.msi", g.config.Version, arch),
					"zip":         fmt.Sprintf("kscore-server-%s-%s.zip", g.config.Version, arch),
					"description": "Keystone Core control plane server",
				},
				{
					"name":        "kscore-agent",
					"msi":         fmt.Sprintf("kscore-agent-%s-%s.msi", g.config.Version, arch),
					"zip":         fmt.Sprintf("kscore-agent-%s-%s.zip", g.config.Version, arch),
					"description": "Keystone Core agent for managed nodes",
				},
				{
					"name":        "kscorectl",
					"msi":         fmt.Sprintf("kscorectl-%s-%s.msi", g.config.Version, arch),
					"zip":         fmt.Sprintf("kscorectl-%s-%s.zip", g.config.Version, arch),
					"description": "Keystone Core CLI tool",
				},
			},
		}

		manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}

		if err := os.WriteFile(filepath.Join(archDir, "manifest.json"), manifestJSON, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		// Generate install.ps1 script
		installScript := g.generateWindowsInstallScript(arch)
		if err := os.WriteFile(filepath.Join(archDir, "install.ps1"), []byte(installScript), 0644); err != nil {
			return fmt.Errorf("write install script: %w", err)
		}
	}

	// Generate README
	readme := fmt.Sprintf(`# Keystone Core Windows Repository

## Installation

### PowerShell (Recommended)

    # Install all components
    iex ((New-Object System.Net.WebClient).DownloadString('https://packages.keystonecore.io/windows/x64/install.ps1'))

    # Install specific component
    .\install.ps1 -Package kscore-agent

### Manual Installation

1. Download the MSI or ZIP from the appropriate architecture folder
2. Run the MSI installer or extract the ZIP
3. Add the installation directory to PATH

## Available Packages

- kscore-server: Control plane server
- kscore-agent: Agent for managed nodes
- kscorectl: CLI tool

## Architectures

- x64: 64-bit Intel/AMD
- arm64: 64-bit ARM (Windows on ARM)

## Version: %s
`, g.config.Version)

	if err := os.WriteFile(filepath.Join(winDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// GenerateBlueprints generates the blueprint registry.
func (g *Generator) GenerateBlueprints() error {
	blueprintsDir := filepath.Join(g.config.OutputDir, "blueprints")

	// Read source blueprints directory
	entries, err := os.ReadDir(g.config.BlueprintsDir)
	if err != nil {
		// If blueprints dir doesn't exist, create empty registry
		if os.IsNotExist(err) {
			return g.generateEmptyBlueprintRegistry(blueprintsDir)
		}
		return fmt.Errorf("read blueprints dir: %w", err)
	}

	// Process each blueprint
	var blueprintsList []BlueprintEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip registry directory (contains metadata, not a blueprint)
		if entry.Name() == "registry" {
			continue
		}

		blueprintPath := filepath.Join(g.config.BlueprintsDir, entry.Name())
		blueprint, err := g.processBlueprintDir(blueprintPath, entry.Name())
		if err != nil {
			fmt.Printf("  Warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		blueprintsList = append(blueprintsList, *blueprint)

		// Generate Go-mod style directory structure
		if err := g.generateBlueprintRegistryEntry(blueprintsDir, blueprint); err != nil {
			return fmt.Errorf("generate blueprint entry: %w", err)
		}
	}

	// Generate index.json
	index := map[string]interface{}{
		"registry":     "https://blueprints.keystonecore.io",
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"count":        len(blueprintsList),
		"blueprints":   blueprintsList,
	}

	indexJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.MkdirAll(blueprintsDir, 0755); err != nil {
		return fmt.Errorf("create blueprints dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(blueprintsDir, "index.json"), indexJSON, 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Generate README
	readme := fmt.Sprintf(`# Keystone Core Blueprint Registry

## Usage

### Configure Registry

    kscorectl blueprint config set registry https://blueprints.keystonecore.io

### List Available Blueprints

    kscorectl blueprint list

### Install Blueprint

    kscorectl blueprint install kscore/demo@0.1.0

## API Endpoints

- GET /index.json - List all blueprints
- GET /kscore/{name}/@v/list - List versions
- GET /kscore/{name}/@v/{version}.info - Version metadata
- GET /kscore/{name}/@v/{version}.mod - Blueprint manifest
- GET /kscore/{name}/@v/{version}.zip - Blueprint archive

## Blueprint Count: %d
## Generated: %s
`, len(blueprintsList), time.Now().UTC().Format(time.RFC3339))

	if err := os.WriteFile(filepath.Join(blueprintsDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// GenerateModules generates the module registry.
func (g *Generator) GenerateModules() error {
	modulesDir := filepath.Join(g.config.OutputDir, "modules")

	// Create directory
	if err := os.MkdirAll(modulesDir, 0755); err != nil {
		return fmt.Errorf("create modules dir: %w", err)
	}

	// Generate placeholder index for now (modules come from examples/modules)
	index := map[string]interface{}{
		"registry":     "https://modules.keystonecore.io",
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"count":        0,
		"modules":      []interface{}{},
	}

	indexJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.WriteFile(filepath.Join(modulesDir, "index.json"), indexJSON, 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Generate README
	readme := `# Keystone Core Module Registry

## Usage

### Configure Registry

    kscorectl module config set registry https://modules.keystonecore.io

### Search Modules

    kscorectl module search nginx

### Install Module

    kscorectl module install std/pkg_apt@0.1.0

## API Endpoints

- GET /index.json - List all modules
- GET /{vendor}/{name}/@v/list - List versions
- GET /{vendor}/{name}/@v/{version}.info - Version metadata
- GET /{vendor}/{name}/@v/{version}.mod - Module manifest
- GET /{vendor}/{name}/@v/{version}.zip - Module archive
`

	if err := os.WriteFile(filepath.Join(modulesDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// generateMasterIndex generates the master index for all repositories.
func (g *Generator) generateMasterIndex() error {
	index := map[string]interface{}{
		"version":      g.config.Version,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"repositories": map[string]string{
			"dnf":        "/dnf/",
			"apt":        "/apt/",
			"windows":    "/windows/",
			"blueprints": "/blueprints/",
			"modules":    "/modules/",
		},
		"urls": map[string]string{
			"packages":   "https://packages.keystonecore.io",
			"blueprints": "https://blueprints.keystonecore.io",
			"modules":    "https://modules.keystonecore.io",
			"docs":       "https://docs.keystonecore.io",
		},
	}

	indexJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.WriteFile(filepath.Join(g.config.OutputDir, "index.json"), indexJSON, 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Generate root README
	readme := fmt.Sprintf(`# Keystone Core Distribution Repositories

This directory contains all distribution repositories for Keystone Core %s.

## Repository Structure

- **dnf/** - RPM packages for RHEL/CentOS/Rocky/Alma/Fedora
- **apt/** - DEB packages for Debian/Ubuntu
- **windows/** - MSI/ZIP packages for Windows
- **blueprints/** - Blueprint registry (Go-mod style)
- **modules/** - Module registry (Go-mod style)

## Quick Start

### Linux (RPM-based)

    curl -o /etc/yum.repos.d/keystonecore.repo \
        https://packages.keystonecore.io/dnf/el9/x86_64/keystonecore.repo
    dnf install kscore-server kscore-agent kscorectl

### Linux (DEB-based)

    curl -fsSL https://packages.keystonecore.io/gpg/keystone-core.asc | \
        gpg --dearmor -o /usr/share/keyrings/keystonecore.gpg
    echo "deb [signed-by=/usr/share/keyrings/keystonecore.gpg] \
        https://packages.keystonecore.io/apt $(lsb_release -cs) main" | \
        tee /etc/apt/sources.list.d/keystonecore.list
    apt update && apt install kscore-server kscore-agent kscorectl

### Windows

    iex ((New-Object System.Net.WebClient).DownloadString( \
        'https://packages.keystonecore.io/windows/x64/install.ps1'))

## Serving This Repository

### nginx

    server {
        listen 80;
        server_name packages.keystonecore.io;
        root /path/to/repos;
        autoindex on;
    }

### Python (Development)

    cd /path/to/repos
    python3 -m http.server 8080

### AWS S3 + CloudFront

Upload contents to S3 bucket with static website hosting enabled.

## Generated: %s
## Version: %s
`, g.config.Version, time.Now().UTC().Format(time.RFC3339), g.config.Version)

	if err := os.WriteFile(filepath.Join(g.config.OutputDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (g *Generator) generateRepomdXML(dist, arch string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <revision>%d</revision>
  <tags>
    <distro>%s</distro>
  </tags>
  <!-- Primary, filelists, and other metadata would be generated by createrepo_c -->
  <!-- This is a placeholder for the actual repodata generation -->
</repomd>
`, time.Now().Unix(), dist)
}

func (g *Generator) generateAPTPackages(arch string) string {
	// Placeholder - actual package generation requires dpkg tools
	return fmt.Sprintf(`Package: kscore-server
Version: %s
Architecture: %s
Maintainer: Keystone Core Team <team@kscore.io>
Description: Keystone Core control plane server

Package: kscore-agent
Version: %s
Architecture: %s
Maintainer: Keystone Core Team <team@kscore.io>
Description: Keystone Core agent for managed nodes

Package: kscorectl
Version: %s
Architecture: %s
Maintainer: Keystone Core Team <team@kscore.io>
Description: Keystone Core CLI tool
`, g.config.Version, arch, g.config.Version, arch, g.config.Version, arch)
}

func (g *Generator) generateAPTRelease(dist string, config *APTRepoConfig) string {
	return fmt.Sprintf(`Origin: %s
Label: %s
Suite: %s
Codename: %s
Architectures: %s
Components: %s
Date: %s
Description: Keystone Core packages for %s
`, config.Origin, config.Label, dist, dist,
		"amd64 arm64", "main",
		time.Now().UTC().Format(time.RFC1123Z), dist)
}

func (g *Generator) generateWindowsInstallScript(arch string) string {
	return fmt.Sprintf(`# Keystone Core Windows Installer
# Architecture: %s
# Version: %s

param(
    [string]$Package = "all",
    [string]$InstallDir = "$env:ProgramFiles\KeystoneCore"
)

$ErrorActionPreference = "Stop"
$BaseURL = "https://packages.keystonecore.io/windows/%s"

Write-Host "Keystone Core Installer - %s"
Write-Host "Installing to: $InstallDir"

# Create installation directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$packages = @{
    "kscore-server" = "kscore-server-%s-%s.zip"
    "kscore-agent"  = "kscore-agent-%s-%s.zip"
    "kscorectl"     = "kscorectl-%s-%s.zip"
}

if ($Package -eq "all") {
    $toInstall = $packages.Keys
} else {
    $toInstall = @($Package)
}

foreach ($pkg in $toInstall) {
    if ($packages.ContainsKey($pkg)) {
        $filename = $packages[$pkg]
        $url = "$BaseURL/$filename"
        $dest = Join-Path $env:TEMP $filename

        Write-Host "Downloading $pkg..."
        Invoke-WebRequest -Uri $url -OutFile $dest

        Write-Host "Installing $pkg..."
        Expand-Archive -Path $dest -DestinationPath $InstallDir -Force

        Remove-Item $dest -Force
    }
}

# Add to PATH
$currentPath = [Environment]::GetEnvironmentVariable("PATH", "Machine")
if ($currentPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$InstallDir", "Machine")
    Write-Host "Added $InstallDir to PATH"
}

Write-Host "Installation complete!"
Write-Host "Please restart your terminal to use the new PATH."
`, arch, g.config.Version, arch, g.config.Version,
		g.config.Version, arch, g.config.Version, arch, g.config.Version, arch)
}

func (g *Generator) processBlueprintDir(path, name string) (*BlueprintEntry, error) {
	// Look for blueprint.yaml or blueprint.yml
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(path, "blueprint.yml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no blueprint manifest found")
		}
	}

	// Read manifest to get version
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// Simple YAML parsing for version (avoid full yaml dependency here)
	version := "0.1.0" // default
	for _, line := range splitLines(string(manifestData)) {
		if len(line) > 10 && line[:8] == "  version" {
			// Extract version value
			parts := splitOnColon(line)
			if len(parts) >= 2 {
				version = trimQuotes(trim(parts[1]))
			}
		}
	}

	return &BlueprintEntry{
		Name:       name,
		Vendor:     "kscore",
		Version:    version,
		SourcePath: path,
	}, nil
}

func (g *Generator) generateBlueprintRegistryEntry(baseDir string, bp *BlueprintEntry) error {
	// Create Go-mod style directory: kscore/{name}/@v/
	bpDir := filepath.Join(baseDir, bp.Vendor, bp.Name, "@v")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		return fmt.Errorf("create blueprint dir: %w", err)
	}

	// Generate list file
	listContent := bp.Version + "\n"
	if err := os.WriteFile(filepath.Join(bpDir, "list"), []byte(listContent), 0644); err != nil {
		return fmt.Errorf("write list: %w", err)
	}

	// Generate .info file
	info := map[string]interface{}{
		"Version": bp.Version,
		"Time":    time.Now().UTC().Format(time.RFC3339),
	}
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(bpDir, bp.Version+".info"), infoJSON, 0644); err != nil {
		return fmt.Errorf("write info: %w", err)
	}

	return nil
}

func (g *Generator) generateEmptyBlueprintRegistry(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	index := map[string]interface{}{
		"registry":     "https://blueprints.keystonecore.io",
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"count":        0,
		"blueprints":   []interface{}{},
	}

	indexJSON, _ := json.MarshalIndent(index, "", "  ")
	return os.WriteFile(filepath.Join(dir, "index.json"), indexJSON, 0644)
}

// String helpers to avoid importing strings package
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitOnColon(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
