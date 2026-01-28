package repogen

import (
	"archive/zip"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	if config.DistDir == "" {
		config.DistDir = "dist"
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
		{"macOS", g.GenerateMacOS},
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

	// Find all RPM files in dist directory
	rpmFiles, err := g.findPackages("*.rpm")
	if err != nil {
		return fmt.Errorf("find RPM files: %w", err)
	}

	if len(rpmFiles) == 0 {
		fmt.Printf("  Warning: No RPM files found in %s\n", g.config.DistDir)
		fmt.Printf("  Run 'goreleaser release --snapshot --clean' first to build packages\n")
	}

	// Map architecture names: goreleaser uses x86_64/aarch64, we use the same
	archMap := map[string]string{
		"x86_64":  "x86_64",
		"amd64":   "x86_64",
		"aarch64": "aarch64",
		"arm64":   "aarch64",
	}

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

			// Copy matching RPM files to Packages directory
			var copiedPackages []string
			for _, rpmFile := range rpmFiles {
				// Check if this RPM matches the architecture
				rpmArch := extractRPMArch(rpmFile)
				if archMap[rpmArch] == arch || rpmArch == arch {
					destPath := filepath.Join(packagesDir, filepath.Base(rpmFile))
					if err := copyFile(rpmFile, destPath); err != nil {
						return fmt.Errorf("copy RPM %s: %w", rpmFile, err)
					}
					copiedPackages = append(copiedPackages, filepath.Base(rpmFile))
				}
			}

			if len(copiedPackages) > 0 {
				fmt.Printf("    %s/%s: %d packages\n", dist, arch, len(copiedPackages))
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

			// Generate repodata
			if err := g.generateRepodata(distDir, packagesDir, repodataDir); err != nil {
				return fmt.Errorf("generate repodata: %w", err)
			}

			// Generate README
			readme := fmt.Sprintf(`# Keystone Core DNF Repository - %s/%s

## Installation

### Add Repository (RHEL/CentOS/Rocky/Alma %s)

    sudo curl -o /etc/yum.repos.d/keystonecore.repo \
        https://packages.keystonecore.io/dnf/%s/%s/keystonecore.repo

### Install Packages

    sudo dnf install kscore-server kscore-agent kscore-cli

## Available Packages

- kscore-server: Control plane server
- kscore-agent: Agent for managed nodes
- kscore-cli: CLI tools (kscorectl, kscore-exec, kscore-state, kscore-monitor)

## Packages in this repository

%s

## Version: %s
`, dist, arch, dist[2:], dist, arch, formatPackageList(copiedPackages), g.config.Version)

			if err := os.WriteFile(filepath.Join(distDir, "README.md"), []byte(readme), 0644); err != nil {
				return fmt.Errorf("write README: %w", err)
			}
		}
	}

	return nil
}

// generateRepodata generates RPM repository metadata.
// This creates a basic repomd.xml and primary.xml for the repository.
// For production use, consider running createrepo_c externally.
func (g *Generator) generateRepodata(distDir, packagesDir, repodataDir string) error {
	// Find all RPMs in packages directory
	rpms, err := filepath.Glob(filepath.Join(packagesDir, "*.rpm"))
	if err != nil {
		return err
	}

	// Generate primary.xml with package information
	primaryXML := g.generatePrimaryXML(rpms)
	primaryPath := filepath.Join(repodataDir, "primary.xml")
	if err := os.WriteFile(primaryPath, []byte(primaryXML), 0644); err != nil {
		return fmt.Errorf("write primary.xml: %w", err)
	}

	// Compress primary.xml
	primaryGzPath := filepath.Join(repodataDir, "primary.xml.gz")
	if err := gzipFile(primaryPath, primaryGzPath); err != nil {
		return fmt.Errorf("gzip primary.xml: %w", err)
	}

	// Calculate checksums
	primaryGzChecksum, err := sha256sum(primaryGzPath)
	if err != nil {
		return fmt.Errorf("checksum primary.xml.gz: %w", err)
	}

	primaryChecksum, err := sha256sum(primaryPath)
	if err != nil {
		return fmt.Errorf("checksum primary.xml: %w", err)
	}

	primaryGzInfo, _ := os.Stat(primaryGzPath)
	primaryInfo, _ := os.Stat(primaryPath)

	// Generate repomd.xml
	repomdXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo" xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <revision>%d</revision>
  <data type="primary">
    <checksum type="sha256">%s</checksum>
    <open-checksum type="sha256">%s</open-checksum>
    <location href="repodata/primary.xml.gz"/>
    <timestamp>%d</timestamp>
    <size>%d</size>
    <open-size>%d</open-size>
  </data>
</repomd>
`, time.Now().Unix(), primaryGzChecksum, primaryChecksum, time.Now().Unix(),
		primaryGzInfo.Size(), primaryInfo.Size())

	if err := os.WriteFile(filepath.Join(repodataDir, "repomd.xml"), []byte(repomdXML), 0644); err != nil {
		return fmt.Errorf("write repomd.xml: %w", err)
	}

	return nil
}

// generatePrimaryXML generates the primary.xml metadata for RPM packages.
func (g *Generator) generatePrimaryXML(rpms []string) string {
	var packages strings.Builder
	for _, rpm := range rpms {
		info, _ := os.Stat(rpm)
		checksum, _ := sha256sum(rpm)
		name := filepath.Base(rpm)

		// Parse package name, version, release, arch from filename
		// Format: name-version-release.arch.rpm
		pkgName, pkgVer, pkgRel, pkgArch := parseRPMFilename(name)

		packages.WriteString(fmt.Sprintf(`  <package type="rpm">
    <name>%s</name>
    <arch>%s</arch>
    <version epoch="0" ver="%s" rel="%s"/>
    <checksum type="sha256" pkgid="YES">%s</checksum>
    <summary>Keystone Core package</summary>
    <description>Keystone Core %s package</description>
    <packager>Keystone Core Team</packager>
    <url>https://keystonecore.io</url>
    <time file="%d" build="%d"/>
    <size package="%d" installed="0" archive="0"/>
    <location href="Packages/%s"/>
    <format>
      <rpm:license>Apache-2.0</rpm:license>
      <rpm:vendor>Keystone Core</rpm:vendor>
      <rpm:group>System Environment/Base</rpm:group>
      <rpm:buildhost>goreleaser</rpm:buildhost>
      <rpm:sourcerpm>%s-%s-%s.src.rpm</rpm:sourcerpm>
    </format>
  </package>
`, pkgName, pkgArch, pkgVer, pkgRel, checksum, pkgName,
			info.ModTime().Unix(), info.ModTime().Unix(), info.Size(), name,
			pkgName, pkgVer, pkgRel))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="%d">
%s</metadata>
`, len(rpms), packages.String())
}

// GenerateAPT generates APT repository structure.
func (g *Generator) GenerateAPT() error {
	aptConfig := DefaultAPTConfig()
	aptDir := filepath.Join(g.config.OutputDir, "apt")

	// Find all DEB files in dist directory
	debFiles, err := g.findPackages("*.deb")
	if err != nil {
		return fmt.Errorf("find DEB files: %w", err)
	}

	if len(debFiles) == 0 {
		fmt.Printf("  Warning: No DEB files found in %s\n", g.config.DistDir)
		fmt.Printf("  Run 'goreleaser release --snapshot --clean' first to build packages\n")
	}

	// Create pool directory and copy all DEBs there
	poolDir := filepath.Join(aptDir, "pool", "main", "k", "keystonecore")
	if err := os.MkdirAll(poolDir, 0755); err != nil {
		return fmt.Errorf("create pool dir: %w", err)
	}

	// Copy all DEB files to pool
	var copiedDebs []string
	for _, debFile := range debFiles {
		destPath := filepath.Join(poolDir, filepath.Base(debFile))
		if err := copyFile(debFile, destPath); err != nil {
			return fmt.Errorf("copy DEB %s: %w", debFile, err)
		}
		copiedDebs = append(copiedDebs, filepath.Base(debFile))
	}

	if len(copiedDebs) > 0 {
		fmt.Printf("    pool: %d packages\n", len(copiedDebs))
	}

	// Map architecture names
	archMap := map[string]string{
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"aarch64": "arm64",
		"arm64":   "arm64",
	}

	for _, dist := range aptConfig.Distributions {
		distReleaseDir := filepath.Join(aptDir, "dists", dist)
		if err := os.MkdirAll(distReleaseDir, 0755); err != nil {
			return fmt.Errorf("create dist dir: %w", err)
		}

		var archHashes []string

		for _, component := range aptConfig.Components {
			for _, arch := range aptConfig.Architectures {
				// Create dist directory structure
				distDir := filepath.Join(aptDir, "dists", dist, component, fmt.Sprintf("binary-%s", arch))
				if err := os.MkdirAll(distDir, 0755); err != nil {
					return fmt.Errorf("create dist dir: %w", err)
				}

				// Generate Packages file from actual DEBs
				packagesContent := g.generateAPTPackagesFromPool(poolDir, arch, archMap)
				packagesPath := filepath.Join(distDir, "Packages")
				if err := os.WriteFile(packagesPath, []byte(packagesContent), 0644); err != nil {
					return fmt.Errorf("write Packages: %w", err)
				}

				// Generate compressed Packages.gz
				packagesGzPath := filepath.Join(distDir, "Packages.gz")
				if err := gzipFile(packagesPath, packagesGzPath); err != nil {
					return fmt.Errorf("gzip Packages: %w", err)
				}

				// Calculate hashes for Release file
				packagesInfo, _ := os.Stat(packagesPath)
				packagesMD5, _ := md5sum(packagesPath)
				packagesSHA256, _ := sha256sum(packagesPath)

				packagesGzInfo, _ := os.Stat(packagesGzPath)
				packagesGzMD5, _ := md5sum(packagesGzPath)
				packagesGzSHA256, _ := sha256sum(packagesGzPath)

				relPath := fmt.Sprintf("%s/binary-%s", component, arch)
				archHashes = append(archHashes,
					fmt.Sprintf(" %s %d %s/Packages", packagesMD5, packagesInfo.Size(), relPath),
					fmt.Sprintf(" %s %d %s/Packages.gz", packagesGzMD5, packagesGzInfo.Size(), relPath),
				)
				archHashes = append(archHashes,
					fmt.Sprintf(" %s %d %s/Packages", packagesSHA256, packagesInfo.Size(), relPath),
					fmt.Sprintf(" %s %d %s/Packages.gz", packagesGzSHA256, packagesGzInfo.Size(), relPath),
				)
			}
		}

		// Generate Release file for distribution
		releaseContent := g.generateAPTReleaseWithHashes(dist, aptConfig, archHashes)
		if err := os.WriteFile(filepath.Join(distReleaseDir, "Release"), []byte(releaseContent), 0644); err != nil {
			return fmt.Errorf("write Release: %w", err)
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
    sudo apt install kscore-server kscore-agent kscore-cli

## Available Packages

- kscore-server: Control plane server
- kscore-agent: Agent for managed nodes
- kscore-cli: CLI tools (kscorectl, kscore-exec, kscore-state, kscore-monitor)

## Supported Distributions

- Ubuntu 24.04 (Noble)
- Ubuntu 22.04 (Jammy)
- Debian 12 (Bookworm)
- Debian 13 (Trixie)

## Packages in this repository

%s

## Version: %s
`, formatPackageList(copiedDebs), g.config.Version)

	if err := os.WriteFile(filepath.Join(aptDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	return nil
}

// generateAPTPackagesFromPool generates Packages file content from actual DEBs in pool.
func (g *Generator) generateAPTPackagesFromPool(poolDir, targetArch string, archMap map[string]string) string {
	debs, _ := filepath.Glob(filepath.Join(poolDir, "*.deb"))

	var packages strings.Builder
	for _, deb := range debs {
		debArch := extractDEBArch(deb)
		mappedArch := archMap[debArch]
		if mappedArch == "" {
			mappedArch = debArch
		}

		if mappedArch != targetArch {
			continue
		}

		info, _ := os.Stat(deb)
		md5sum, _ := md5sum(deb)
		sha256sum, _ := sha256sum(deb)
		filename := filepath.Base(deb)

		// Parse package name, version, arch from filename
		// Format: name_version_arch.deb
		pkgName, pkgVer, _ := parseDEBFilename(filename)

		packages.WriteString(fmt.Sprintf(`Package: %s
Version: %s
Architecture: %s
Maintainer: Keystone Core Team <team@keystonecore.io>
Installed-Size: %d
Filename: pool/main/k/keystonecore/%s
Size: %d
MD5sum: %s
SHA256: %s
Section: admin
Priority: optional
Description: Keystone Core %s package
 A cloud-native runtime infrastructure control plane.

`, pkgName, pkgVer, targetArch, info.Size()/1024, filename, info.Size(), md5sum, sha256sum, pkgName))
	}

	return packages.String()
}

// generateAPTReleaseWithHashes generates APT Release file with actual file hashes.
func (g *Generator) generateAPTReleaseWithHashes(dist string, config *APTRepoConfig, hashes []string) string {
	// Split hashes into MD5 and SHA256 sections
	var md5Hashes, sha256Hashes []string
	half := len(hashes) / 2
	if half > 0 {
		md5Hashes = hashes[:half]
		sha256Hashes = hashes[half:]
	}

	release := fmt.Sprintf(`Origin: %s
Label: %s
Suite: %s
Codename: %s
Architectures: %s
Components: %s
Date: %s
Description: Keystone Core packages for %s
`, config.Origin, config.Label, dist, dist,
		strings.Join(config.Architectures, " "),
		strings.Join(config.Components, " "),
		time.Now().UTC().Format(time.RFC1123Z), dist)

	if len(md5Hashes) > 0 {
		release += "MD5Sum:\n" + strings.Join(md5Hashes, "\n") + "\n"
	}
	if len(sha256Hashes) > 0 {
		release += "SHA256:\n" + strings.Join(sha256Hashes, "\n") + "\n"
	}

	return release
}

// GenerateWindows generates Windows repository structure.
func (g *Generator) GenerateWindows() error {
	winConfig := DefaultWindowsConfig()
	winDir := filepath.Join(g.config.OutputDir, "windows")

	// Find all Windows archives in dist directory
	zipFiles, err := g.findPackages("*_windows_*.zip")
	if err != nil {
		return fmt.Errorf("find Windows ZIP files: %w", err)
	}

	if len(zipFiles) == 0 {
		fmt.Printf("  Warning: No Windows ZIP files found in %s\n", g.config.DistDir)
		fmt.Printf("  Run 'goreleaser release --snapshot --clean' first to build packages\n")
	}

	// Map goreleaser arch names to our names
	archMap := map[string]string{
		"amd64": "x64",
		"arm64": "arm64",
	}

	for _, arch := range winConfig.Architectures {
		archDir := filepath.Join(winDir, arch)
		if err := os.MkdirAll(archDir, 0755); err != nil {
			return fmt.Errorf("create arch dir: %w", err)
		}

		// Find and copy matching ZIP files
		var copiedFiles []string
		var packageEntries []map[string]string

		for _, zipFile := range zipFiles {
			// Check if this ZIP matches the architecture
			zipArch := extractWindowsArch(zipFile)
			if archMap[zipArch] == arch {
				destPath := filepath.Join(archDir, filepath.Base(zipFile))
				if err := copyFile(zipFile, destPath); err != nil {
					return fmt.Errorf("copy ZIP %s: %w", zipFile, err)
				}
				copiedFiles = append(copiedFiles, filepath.Base(zipFile))

				// Create package entry
				info, _ := os.Stat(destPath)
				checksum, _ := sha256sum(destPath)

				packageEntries = append(packageEntries, map[string]string{
					"filename":    filepath.Base(zipFile),
					"size":        fmt.Sprintf("%d", info.Size()),
					"sha256":      checksum,
					"description": fmt.Sprintf("Keystone Core %s", g.config.Version),
				})
			}
		}

		if len(copiedFiles) > 0 {
			fmt.Printf("    %s: %d packages\n", arch, len(copiedFiles))
		}

		// Generate manifest.json with actual files
		manifest := map[string]interface{}{
			"version":      g.config.Version,
			"architecture": arch,
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"packages":     packageEntries,
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

1. Download the ZIP from the appropriate architecture folder
2. Extract the ZIP
3. Add the installation directory to PATH

## Available Packages

- keystone-core: All binaries (kscore-server, kscore-agent, kscorectl, etc.)

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

// GenerateMacOS generates the macOS repository.
func (g *Generator) GenerateMacOS() error {
	macDir := filepath.Join(g.config.OutputDir, "macos")

	// Find all macOS archives in dist directory (tar.gz and zip)
	tarFiles, err := g.findPackages("*_darwin_*.tar.gz")
	if err != nil {
		return fmt.Errorf("find macOS tar.gz files: %w", err)
	}

	zipFiles, err := g.findPackages("*_darwin_*.zip")
	if err != nil {
		return fmt.Errorf("find macOS zip files: %w", err)
	}

	allFiles := append(tarFiles, zipFiles...)

	if len(allFiles) == 0 {
		fmt.Printf("  Warning: No macOS archives found in %s\n", g.config.DistDir)
		fmt.Printf("  Run 'goreleaser release --snapshot --clean' first to build packages\n")
	}

	// Map goreleaser arch names to our names
	archMap := map[string]string{
		"amd64": "x64",
		"arm64": "arm64",
	}

	for _, arch := range []string{"x64", "arm64"} {
		archDir := filepath.Join(macDir, arch)
		if err := os.MkdirAll(archDir, 0755); err != nil {
			return fmt.Errorf("create arch dir: %w", err)
		}

		// Find and copy matching files
		var copiedFiles []string
		var packageEntries []map[string]string

		for _, archiveFile := range allFiles {
			// Check if this archive matches the architecture
			fileArch := extractMacOSArch(archiveFile)
			if archMap[fileArch] == arch {
				destPath := filepath.Join(archDir, filepath.Base(archiveFile))
				if err := copyFile(archiveFile, destPath); err != nil {
					return fmt.Errorf("copy archive %s: %w", archiveFile, err)
				}
				copiedFiles = append(copiedFiles, filepath.Base(archiveFile))

				// Create package entry
				info, _ := os.Stat(destPath)
				checksum, _ := sha256sum(destPath)

				packageEntries = append(packageEntries, map[string]string{
					"filename":    filepath.Base(archiveFile),
					"size":        fmt.Sprintf("%d", info.Size()),
					"sha256":      checksum,
					"description": fmt.Sprintf("Keystone Core %s", g.config.Version),
				})
			}
		}

		if len(copiedFiles) > 0 {
			fmt.Printf("    %s: %d packages\n", arch, len(copiedFiles))
		}

		// Generate manifest.json with actual files
		manifest := map[string]interface{}{
			"version":      g.config.Version,
			"architecture": arch,
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"packages":     packageEntries,
		}

		manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}

		if err := os.WriteFile(filepath.Join(archDir, "manifest.json"), manifestJSON, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		// Generate install.sh script
		installScript := g.generateMacOSInstallScript(arch)
		if err := os.WriteFile(filepath.Join(archDir, "install.sh"), []byte(installScript), 0755); err != nil {
			return fmt.Errorf("write install script: %w", err)
		}
	}

	// Generate README
	readme := fmt.Sprintf(`# Keystone Core macOS Repository

## Installation

### Homebrew (Recommended - Coming Soon)

    brew tap keystonecore/tap
    brew install keystone-core

### Shell Script

    # Install all components (x64)
    curl -fsSL https://packages.keystonecore.io/macos/x64/install.sh | bash

    # Install all components (Apple Silicon)
    curl -fsSL https://packages.keystonecore.io/macos/arm64/install.sh | bash

### Manual Installation

1. Download the appropriate archive for your architecture
2. Extract: tar xzf keystone-core_*.tar.gz (or unzip for .zip)
3. Move binaries to /usr/local/bin or add to PATH

## Available Packages

- keystone-core: All binaries (kscore-server, kscore-agent, kscorectl, etc.)

## Architectures

- x64: Intel Macs
- arm64: Apple Silicon (M1/M2/M3)

## Version: %s
`, g.config.Version)

	if err := os.WriteFile(filepath.Join(macDir, "README.md"), []byte(readme), 0644); err != nil {
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

// findPackages finds package files matching a pattern in the dist directory.
func (g *Generator) findPackages(pattern string) ([]string, error) {
	searchPath := filepath.Join(g.config.DistDir, pattern)
	matches, err := filepath.Glob(searchPath)
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// gzipFile compresses a file with gzip.
func gzipFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	gw := gzip.NewWriter(dstFile)
	defer gw.Close()

	_, err = io.Copy(gw, srcFile)
	return err
}

// sha256sum calculates the SHA256 checksum of a file.
func sha256sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// md5sum calculates the MD5 checksum of a file.
func md5sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractRPMArch extracts the architecture from an RPM filename.
// Format: name-version-release.arch.rpm
func extractRPMArch(filename string) string {
	base := filepath.Base(filename)
	// Remove .rpm extension
	base = strings.TrimSuffix(base, ".rpm")

	// Try goreleaser naming convention: name_version_linux_arch.rpm
	// e.g., kscore-server_0.0.1-next_linux_amd64.rpm
	parts := strings.Split(base, "_")
	if len(parts) >= 4 {
		// Format: name_version_os_arch
		return parts[len(parts)-1]
	}

	// Try traditional RPM naming: package.arch.rpm
	dotParts := strings.Split(base, ".")
	if len(dotParts) >= 2 {
		return dotParts[len(dotParts)-1]
	}

	return "x86_64"
}

// extractDEBArch extracts the architecture from a DEB filename.
// Format: name_version_arch.deb
func extractDEBArch(filename string) string {
	base := filepath.Base(filename)
	// Remove .deb extension
	base = strings.TrimSuffix(base, ".deb")
	// Split by underscore
	parts := strings.Split(base, "_")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return "amd64"
}

// extractWindowsArch extracts the architecture from a Windows archive filename.
// Format: keystone-core_version_windows_arch.zip
func extractWindowsArch(filename string) string {
	base := filepath.Base(filename)
	// Use regex to extract arch from windows_arch pattern
	re := regexp.MustCompile(`_windows_([^_\.]+)`)
	matches := re.FindStringSubmatch(base)
	if len(matches) >= 2 {
		return matches[1]
	}
	return "amd64"
}

func extractMacOSArch(filename string) string {
	base := filepath.Base(filename)
	// Use regex to extract arch from darwin_arch pattern
	re := regexp.MustCompile(`_darwin_([^_\.]+)`)
	matches := re.FindStringSubmatch(base)
	if len(matches) >= 2 {
		return matches[1]
	}
	return "amd64"
}

// parseRPMFilename parses an RPM filename into components.
// Format: name-version-release.arch.rpm
func parseRPMFilename(filename string) (name, version, release, arch string) {
	base := strings.TrimSuffix(filename, ".rpm")

	// Try goreleaser naming convention: name_version_os_arch.rpm
	// e.g., kscore-server_0.0.1-next_linux_amd64.rpm
	underscoreParts := strings.Split(base, "_")
	if len(underscoreParts) >= 4 {
		// Format: name_version_os_arch
		name = underscoreParts[0]
		version = underscoreParts[1]
		release = "1"
		goreleaserArch := underscoreParts[len(underscoreParts)-1]
		// Map goreleaser arch to RPM arch
		switch goreleaserArch {
		case "amd64":
			arch = "x86_64"
		case "arm64":
			arch = "aarch64"
		default:
			arch = goreleaserArch
		}
		return
	}

	// Fall back to traditional RPM naming: package-version-release.arch.rpm
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return filename, "0", "0", "x86_64"
	}
	arch = parts[len(parts)-1]
	nameVerRel := strings.Join(parts[:len(parts)-1], ".")

	// Find version-release (last two dash-separated components)
	dashParts := strings.Split(nameVerRel, "-")
	if len(dashParts) >= 3 {
		release = dashParts[len(dashParts)-1]
		version = dashParts[len(dashParts)-2]
		name = strings.Join(dashParts[:len(dashParts)-2], "-")
	} else if len(dashParts) == 2 {
		version = dashParts[1]
		release = "1"
		name = dashParts[0]
	} else {
		name = nameVerRel
		version = "0"
		release = "1"
	}
	return
}

// parseDEBFilename parses a DEB filename into components.
// Format: name_version_arch.deb
func parseDEBFilename(filename string) (name, version, arch string) {
	base := strings.TrimSuffix(filename, ".deb")
	parts := strings.Split(base, "_")
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2]
	} else if len(parts) == 2 {
		return parts[0], parts[1], "amd64"
	}
	return base, "0", "amd64"
}

// formatPackageList formats a list of packages for display.
func formatPackageList(packages []string) string {
	if len(packages) == 0 {
		return "No packages found"
	}
	sort.Strings(packages)
	var sb strings.Builder
	for _, pkg := range packages {
		sb.WriteString("- ")
		sb.WriteString(pkg)
		sb.WriteString("\n")
	}
	return sb.String()
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

func (g *Generator) generateMacOSInstallScript(arch string) string {
	return fmt.Sprintf(`#!/bin/bash
# Keystone Core macOS Installer
# Architecture: %s
# Version: %s

set -e

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BASE_URL="https://packages.keystonecore.io/macos/%s"
TMP_DIR=$(mktemp -d)

echo "Keystone Core Installer - %s"
echo "Installing to: $INSTALL_DIR"

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    DOWNLOAD_ARCH="amd64"
elif [ "$ARCH" = "arm64" ]; then
    DOWNLOAD_ARCH="arm64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Download and extract
ARCHIVE="keystone-core_%s_darwin_${DOWNLOAD_ARCH}.tar.gz"
echo "Downloading $ARCHIVE..."
curl -fsSL "$BASE_URL/$ARCHIVE" -o "$TMP_DIR/$ARCHIVE"

echo "Extracting..."
tar xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

# Install binaries
echo "Installing binaries..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp "$TMP_DIR"/**/kscore-* "$INSTALL_DIR/" 2>/dev/null || sudo cp "$TMP_DIR"/kscore-* "$INSTALL_DIR/"
sudo cp "$TMP_DIR"/**/kscorectl "$INSTALL_DIR/" 2>/dev/null || sudo cp "$TMP_DIR"/kscorectl "$INSTALL_DIR/"
sudo chmod +x "$INSTALL_DIR"/kscore-* "$INSTALL_DIR"/kscorectl

# Cleanup
rm -rf "$TMP_DIR"

echo "Installation complete!"
echo "Installed binaries to $INSTALL_DIR"

# Verify PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "NOTE: $INSTALL_DIR is not in your PATH."
    echo "Add it with: export PATH=\"\$PATH:$INSTALL_DIR\""
fi
`, arch, g.config.Version, arch, g.config.Version, g.config.Version)
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

	// Copy .mod file (blueprint manifest)
	manifestPath := filepath.Join(bp.SourcePath, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(bp.SourcePath, "blueprint.yml")
	}
	if manifestData, err := os.ReadFile(manifestPath); err == nil {
		modPath := filepath.Join(bpDir, bp.Version+".mod")
		if err := os.WriteFile(modPath, manifestData, 0644); err != nil {
			return fmt.Errorf("write mod file: %w", err)
		}
	}

	// Create .zip archive of the blueprint directory
	zipPath := filepath.Join(bpDir, bp.Version+".zip")
	if err := g.createBlueprintZip(bp.SourcePath, zipPath, bp.Vendor+"/"+bp.Name+"@"+bp.Version); err != nil {
		return fmt.Errorf("create zip archive: %w", err)
	}

	// Calculate and store checksum
	if checksum, err := sha256sum(zipPath); err == nil {
		bp.Checksum = checksum
		checksumPath := filepath.Join(bpDir, bp.Version+".zip.sha256")
		if err := os.WriteFile(checksumPath, []byte(checksum+"  "+bp.Version+".zip\n"), 0644); err != nil {
			return fmt.Errorf("write checksum: %w", err)
		}
	}

	return nil
}

// createBlueprintZip creates a zip archive of a blueprint directory.
// The prefix is used to create the Go module-style path structure inside the zip.
func (g *Generator) createBlueprintZip(srcDir, destZip, prefix string) error {
	zipFile, err := os.Create(destZip)
	if err != nil {
		return fmt.Errorf("create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories (they're created implicitly)
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Create zip entry with Go module-style path: vendor/name@version/file
		zipPath := prefix + "/" + filepath.ToSlash(relPath)

		// Create file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipPath
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// Copy file contents
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
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
