package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// InstallOptions configures the package installer.
type InstallOptions struct {
	ArchivePath string
	TargetDir   string // Binary install dir (default /usr/local/bin)
	ConfigDir   string // Config install dir (default /etc/keystone-core)
	DataDir     string // Data dir (default /var/lib/keystone-core)
	Verify      bool
	TrustedKeys [][]byte // PEM public keys for verification
	SkipModules bool
	Unattended  bool
}

// InstallResult holds the outcome of a package installation.
type InstallResult struct {
	Manifest        *Manifest
	BinariesCount   int
	ConfigsCount    int
	ModulesCount    int
	BlueprintsCount int
}

// Installer extracts and installs a bootstrap package.
type Installer struct {
	opts InstallOptions
}

// NewInstaller creates a new package installer.
func NewInstaller(opts InstallOptions) (*Installer, error) {
	if opts.ArchivePath == "" {
		return nil, fmt.Errorf("archive path is required")
	}
	if opts.TargetDir == "" {
		opts.TargetDir = "/usr/local/bin"
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = "/etc/keystone-core"
	}
	if opts.DataDir == "" {
		opts.DataDir = "/var/lib/keystone-core"
	}
	return &Installer{opts: opts}, nil
}

// Install extracts the bootstrap package and installs its contents.
func (inst *Installer) Install(ctx context.Context) (*InstallResult, error) {
	// Extract archive to temp dir
	extractDir, err := os.MkdirTemp("", "kscore-install-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := ExtractArchive(inst.opts.ArchivePath, extractDir); err != nil {
		return nil, fmt.Errorf("extracting archive: %w", err)
	}

	// Read manifest
	manifest, err := ReadManifest(filepath.Join(extractDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	// Verify if requested
	if inst.opts.Verify {
		verifier := NewPackageVerifier(inst.opts.TrustedKeys)
		if err := verifier.VerifyManifestSignature(ctx, extractDir); err != nil {
			return nil, fmt.Errorf("signature verification failed: %w", err)
		}
		if err := verifier.VerifyChecksums(extractDir, manifest); err != nil {
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	result := &InstallResult{Manifest: manifest}

	// Install binaries
	if err := os.MkdirAll(inst.opts.TargetDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating target dir: %w", err)
	}
	for _, comp := range manifest.Components {
		srcPath := filepath.Join(extractDir, comp.Path)
		dstPath := filepath.Join(inst.opts.TargetDir, comp.Name)
		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("installing binary %s: %w", comp.Name, err)
		}
		if err := os.Chmod(dstPath, 0o755); err != nil { //nolint:gosec // G302: binaries must be executable
			return nil, fmt.Errorf("chmod %s: %w", comp.Name, err)
		}
		result.BinariesCount++
	}

	// Install config templates (don't overwrite existing)
	if err := os.MkdirAll(inst.opts.ConfigDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}
	for _, tmpl := range manifest.ConfigTemplates {
		srcPath := filepath.Join(extractDir, tmpl.Path)
		baseName := tmpl.Name
		dstPath := filepath.Join(inst.opts.ConfigDir, baseName)

		if _, err := os.Stat(dstPath); err == nil {
			continue // don't overwrite existing config
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("installing config %s: %w", baseName, err)
		}
		result.ConfigsCount++
	}

	// Install modules and blueprints to local registry
	if !inst.opts.SkipModules {
		registryDir := filepath.Join(inst.opts.DataDir, "registry")

		if len(manifest.Modules) > 0 {
			modDir := filepath.Join(registryDir, "modules")
			if err := os.MkdirAll(modDir, 0o750); err != nil {
				return nil, fmt.Errorf("creating modules dir: %w", err)
			}
			for _, mod := range manifest.Modules {
				srcPath := filepath.Join(extractDir, mod.Path)
				dstPath := filepath.Join(modDir, mod.Name)
				if err := copyFile(srcPath, dstPath); err != nil {
					return nil, fmt.Errorf("installing module %s: %w", mod.Name, err)
				}
				result.ModulesCount++
			}
		}

		if len(manifest.Blueprints) > 0 {
			bpDir := filepath.Join(registryDir, "blueprints")
			if err := os.MkdirAll(bpDir, 0o750); err != nil {
				return nil, fmt.Errorf("creating blueprints dir: %w", err)
			}
			for _, bp := range manifest.Blueprints {
				srcPath := filepath.Join(extractDir, bp.Path)
				dstPath := filepath.Join(bpDir, filepath.Base(bp.Path))
				if err := copyFile(srcPath, dstPath); err != nil {
					return nil, fmt.Errorf("installing blueprint %s: %w", bp.Name, err)
				}
				result.BlueprintsCount++
			}
		}
	}

	return result, nil
}
