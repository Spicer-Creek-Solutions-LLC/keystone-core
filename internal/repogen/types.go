// Package repogen generates distribution repositories for Keystone Core releases.
//
// This package creates static repository structures that can be served by
// nginx, Apache, or any CDN for package distribution and registry hosting.
//
// Supported repository types:
//   - DNF/YUM: RPM packages for RHEL/CentOS/Fedora
//   - APT: DEB packages for Debian/Ubuntu
//   - Windows: MSI/ZIP packages for Windows
//   - Blueprints: Blueprint registry (Go-mod style endpoints)
//   - Modules: Module registry for Keystone modules
//
// Output Structure:
//
//	build/repos/
//	├── dnf/
//	│   ├── el8/
//	│   │   └── x86_64/
//	│   │       ├── Packages/
//	│   │       └── repodata/
//	│   └── el9/
//	│       └── x86_64/
//	├── apt/
//	│   ├── dists/
//	│   │   ├── jammy/
//	│   │   └── bookworm/
//	│   └── pool/
//	│       └── main/
//	├── windows/
//	│   ├── x64/
//	│   └── arm64/
//	├── blueprints/
//	│   └── kscore/
//	│       └── {blueprint}/@v/
//	└── modules/
//	    └── {vendor}/
//	        └── {module}/@v/
package repogen

import (
	"time"
)

// Config holds configuration for repository generation.
type Config struct {
	// Version is the release version (e.g., "0.1.0")
	Version string

	// OutputDir is the base output directory for all repositories
	OutputDir string

	// DistDir is the goreleaser output directory containing packages
	// Defaults to "dist" if not specified
	DistDir string

	// BinariesDir is the directory containing built binaries (legacy, use DistDir)
	BinariesDir string

	// GPGKeyID is the GPG key ID for signing packages and metadata
	GPGKeyID string

	// SignPackages enables package and metadata signing
	SignPackages bool

	// BlueprintsDir is the source directory for blueprints
	BlueprintsDir string

	// ModulesDir is the source directory for modules
	ModulesDir string
}

// PackageInfo describes a package to be included in a repository.
type PackageInfo struct {
	Name         string
	Version      string
	Release      string
	Architecture string
	Description  string
	License      string
	URL          string
	Maintainer   string
	Dependencies []string
	Files        []FileEntry
}

// FileEntry describes a file to be installed by a package.
type FileEntry struct {
	Source      string // Source path in binaries directory
	Destination string // Destination path on target system
	Mode        string // File mode (e.g., "0755")
	IsConfig    bool   // Whether this is a config file
}

// RepositoryMetadata holds metadata for a generated repository.
type RepositoryMetadata struct {
	Type        string    // Repository type (dnf, apt, windows, blueprints, modules)
	Version     string    // Keystone Core version
	GeneratedAt time.Time // Generation timestamp
	PackageCount int      // Number of packages/items
	Signed      bool      // Whether the repo is signed
	BaseURL     string    // Suggested base URL for serving
}

// BlueprintEntry describes a blueprint in the registry.
type BlueprintEntry struct {
	Name        string
	Vendor      string // "kscore" for official blueprints
	Version     string
	Description string
	Checksum    string
	Signature   string
	SourcePath  string
}

// ModuleEntry describes a module in the registry.
type ModuleEntry struct {
	Name        string
	Vendor      string
	Version     string
	Description string
	Checksum    string
	Signature   string
	SourcePath  string
}

// DNFRepoConfig holds DNF/YUM-specific configuration.
type DNFRepoConfig struct {
	// Distributions to generate (e.g., "el8", "el9", "fedora39")
	Distributions []string

	// Architectures to include
	Architectures []string
}

// APTRepoConfig holds APT-specific configuration.
type APTRepoConfig struct {
	// Distributions to generate (e.g., "jammy", "bookworm", "noble")
	Distributions []string

	// Components (e.g., "main", "contrib")
	Components []string

	// Architectures to include
	Architectures []string

	// Origin for Release file
	Origin string

	// Label for Release file
	Label string

	// Suite (usually same as distribution)
	Suite string

	// Codename (distribution codename)
	Codename string
}

// WindowsRepoConfig holds Windows-specific configuration.
type WindowsRepoConfig struct {
	// Architectures to include (x64, arm64)
	Architectures []string

	// GenerateMSI enables MSI installer generation
	GenerateMSI bool

	// GenerateZIP enables ZIP archive generation
	GenerateZIP bool
}

// DefaultDNFConfig returns default DNF repository configuration.
func DefaultDNFConfig() *DNFRepoConfig {
	return &DNFRepoConfig{
		Distributions: []string{"el8", "el9"},
		Architectures: []string{"x86_64", "aarch64"},
	}
}

// DefaultAPTConfig returns default APT repository configuration.
func DefaultAPTConfig() *APTRepoConfig {
	return &APTRepoConfig{
		Distributions: []string{"jammy", "noble", "bookworm", "trixie"},
		Components:    []string{"main"},
		Architectures: []string{"amd64", "arm64"},
		Origin:        "Keystone Core",
		Label:         "keystonecore",
	}
}

// DefaultWindowsConfig returns default Windows repository configuration.
func DefaultWindowsConfig() *WindowsRepoConfig {
	return &WindowsRepoConfig{
		Architectures: []string{"x64", "arm64"},
		GenerateMSI:   true,
		GenerateZIP:   true,
	}
}

// KeystonePackages returns the list of Keystone Core packages to include.
func KeystonePackages(version string) []PackageInfo {
	return []PackageInfo{
		{
			Name:        "kscore-server",
			Version:     version,
			Release:     "1",
			Description: "Keystone Core control plane server",
			License:     "Apache-2.0",
			URL:         "https://github.com/shawnbutts/keystone-core",
			Maintainer:  "Keystone Core Team <team@kscore.io>",
			Files: []FileEntry{
				{Source: "kscore-server", Destination: "/usr/local/bin/kscore-server", Mode: "0755"},
			},
		},
		{
			Name:        "kscore-agent",
			Version:     version,
			Release:     "1",
			Description: "Keystone Core agent for managed nodes",
			License:     "Apache-2.0",
			URL:         "https://github.com/shawnbutts/keystone-core",
			Maintainer:  "Keystone Core Team <team@kscore.io>",
			Files: []FileEntry{
				{Source: "kscore-agent", Destination: "/usr/local/bin/kscore-agent", Mode: "0755"},
			},
		},
		{
			Name:        "kscorectl",
			Version:     version,
			Release:     "1",
			Description: "Keystone Core CLI tool",
			License:     "Apache-2.0",
			URL:         "https://github.com/shawnbutts/keystone-core",
			Maintainer:  "Keystone Core Team <team@kscore.io>",
			Files: []FileEntry{
				{Source: "kscorectl", Destination: "/usr/local/bin/kscorectl", Mode: "0755"},
			},
		},
	}
}
