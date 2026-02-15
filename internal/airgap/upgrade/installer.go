package upgrade

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

// InstallerConfig configures the upgrade installer.
type InstallerConfig struct {
	PackagePath  string   // Path to upgrade .tar.gz
	InstallDir   string   // Directory where binaries live
	BackupDir    string   // Directory for rollback backup
	DryRun       bool     // Show what would happen without changing anything
	SkipBackup   bool     // Skip creating backup before upgrade
	SkipScripts  bool     // Skip pre/post upgrade scripts
	TrustedKeys  [][]byte // PEM-encoded public keys for verification
	ProgressFunc func(phase string, progress int)
}

// Installer applies upgrade packages to a Keystone Core installation.
type Installer struct {
	config InstallerConfig
}

// InstallResult contains the outcome of applying an upgrade.
type InstallResult struct {
	FromVersion        string
	ToVersion          string
	UpgradedComponents []string
	MigrationsRun      int
	BackupPath         string
	Duration           time.Duration
	Warnings           []string
}

// NewInstaller creates a new upgrade installer.
func NewInstaller(config InstallerConfig) (*Installer, error) {
	if config.PackagePath == "" {
		return nil, fmt.Errorf("package path is required")
	}
	if config.InstallDir == "" {
		return nil, fmt.Errorf("install directory is required")
	}
	if !config.SkipBackup && config.BackupDir == "" {
		return nil, fmt.Errorf("backup directory is required (or use SkipBackup)")
	}
	return &Installer{config: config}, nil
}

// Install extracts and applies an upgrade package.
func (inst *Installer) Install(ctx context.Context) (*InstallResult, error) {
	start := time.Now()
	result := &InstallResult{}

	inst.progress("extracting", 0)

	// Extract archive to temp dir
	extractDir, err := os.MkdirTemp("", "kscore-upgrade-extract-*")
	if err != nil {
		return nil, fmt.Errorf("creating extract directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := bootstrap.ExtractArchive(inst.config.PackagePath, extractDir); err != nil {
		return nil, fmt.Errorf("extracting package: %w", err)
	}

	inst.progress("verifying", 10)

	// Read and validate manifest
	manifest, err := ReadManifest(filepath.Join(extractDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	result.FromVersion = manifest.FromVersion
	result.ToVersion = manifest.ToVersion

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Verify signature if trusted keys provided
	if len(inst.config.TrustedKeys) > 0 {
		verifier := NewPackageVerifier(inst.config.TrustedKeys)
		vr, err := verifier.Verify(ctx, extractDir)
		if err != nil {
			return nil, fmt.Errorf("verification error: %w", err)
		}
		if !vr.Valid {
			return nil, fmt.Errorf("package verification failed: %w", vr.Error)
		}
		result.Warnings = append(result.Warnings, vr.Warnings...)
	}

	inst.progress("preparing", 30)

	if inst.config.DryRun {
		result.Duration = time.Since(start)
		for _, c := range manifest.Components {
			result.UpgradedComponents = append(result.UpgradedComponents, c.Name)
		}
		result.MigrationsRun = len(manifest.Migrations)
		result.Warnings = append(result.Warnings, "dry run - no changes made")
		return result, nil
	}

	// Backup current binaries
	if !inst.config.SkipBackup {
		inst.progress("backing up", 40)
		backupPath, err := inst.createBackup(manifest)
		if err != nil {
			return nil, fmt.Errorf("creating backup: %w", err)
		}
		result.BackupPath = backupPath
	}

	inst.progress("upgrading", 60)

	// Replace binaries
	for _, c := range manifest.Components {
		srcPath := filepath.Join(extractDir, c.Path)
		dstPath := filepath.Join(inst.config.InstallDir, c.Name)

		if err := copyFilePreserve(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("replacing %s: %w", c.Name, err)
		}
		result.UpgradedComponents = append(result.UpgradedComponents, c.Name)
	}

	inst.progress("verifying installation", 90)

	// Verify replaced binaries match expected checksums
	for _, c := range manifest.Components {
		if c.SHA256 == "" {
			continue
		}
		installedPath := filepath.Join(inst.config.InstallDir, c.Name)
		hash, err := bootstrap.HashFile(installedPath)
		if err != nil {
			return nil, fmt.Errorf("verifying installed %s: %w", c.Name, err)
		}
		if hash != c.SHA256 {
			return nil, fmt.Errorf("installed %s checksum mismatch after copy", c.Name)
		}
	}

	result.MigrationsRun = len(manifest.Migrations)
	if len(manifest.Migrations) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d migration(s) included but must be applied separately", len(manifest.Migrations)))
	}

	inst.progress("complete", 100)
	result.Duration = time.Since(start)
	return result, nil
}

func (inst *Installer) createBackup(manifest *Manifest) (string, error) {
	backupDir := filepath.Join(inst.config.BackupDir,
		fmt.Sprintf("upgrade-backup-%s-to-%s-%d",
			manifest.FromVersion, manifest.ToVersion, time.Now().Unix()))

	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}

	// Write backup metadata
	meta := &Manifest{
		SchemaVersion: SchemaVersion,
		FromVersion:   manifest.FromVersion,
		ToVersion:     manifest.ToVersion,
		Platform:      manifest.Platform,
		Created:       time.Now().UTC(),
	}

	for _, c := range manifest.Components {
		srcPath := filepath.Join(inst.config.InstallDir, c.Name)
		dstPath := filepath.Join(backupDir, c.Name)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue
		}

		if err := copyFilePreserve(srcPath, dstPath); err != nil {
			return "", fmt.Errorf("backing up %s: %w", c.Name, err)
		}

		hash, err := bootstrap.HashFile(dstPath)
		if err != nil {
			return "", fmt.Errorf("hashing backup %s: %w", c.Name, err)
		}
		info, _ := os.Stat(dstPath)
		meta.Components = append(meta.Components, bootstrap.ComponentEntry{
			Name:   c.Name,
			Path:   c.Name,
			SHA256: hash,
			Size:   info.Size(),
		})
	}

	if err := WriteManifest(meta, filepath.Join(backupDir, "manifest.json")); err != nil {
		return "", fmt.Errorf("writing backup manifest: %w", err)
	}

	return backupDir, nil
}

func (inst *Installer) progress(phase string, pct int) {
	if inst.config.ProgressFunc != nil {
		inst.config.ProgressFunc(phase, pct)
	}
}

func copyFilePreserve(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: controlled path from package extraction
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()) //nolint:gosec // G304: controlled destination
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
