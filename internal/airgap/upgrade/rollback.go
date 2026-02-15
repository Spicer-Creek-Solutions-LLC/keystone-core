package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

// RollbackConfig configures a rollback operation.
type RollbackConfig struct {
	BackupDir  string // Directory containing backup (from installer)
	InstallDir string // Directory where binaries are installed
	DryRun     bool
}

// RollbackResult contains the outcome of a rollback operation.
type RollbackResult struct {
	RestoredComponents []string
	SkippedComponents  []string
	Duration           time.Duration
}

// Rollback restores binaries from a backup directory created during upgrade.
func Rollback(_ context.Context, config RollbackConfig) (*RollbackResult, error) {
	if config.BackupDir == "" {
		return nil, fmt.Errorf("backup directory is required")
	}
	if config.InstallDir == "" {
		return nil, fmt.Errorf("install directory is required")
	}

	start := time.Now()
	result := &RollbackResult{}

	// Read backup manifest
	manifestPath := filepath.Join(config.BackupDir, "manifest.json")
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading backup manifest: %w", err)
	}

	if config.DryRun {
		for _, c := range manifest.Components {
			result.RestoredComponents = append(result.RestoredComponents, c.Name)
		}
		result.Duration = time.Since(start)
		return result, nil
	}

	// Restore each backed-up component
	for _, c := range manifest.Components {
		srcPath := filepath.Join(config.BackupDir, c.Path)
		dstPath := filepath.Join(config.InstallDir, c.Name)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			result.SkippedComponents = append(result.SkippedComponents, c.Name)
			continue
		}

		if err := copyFilePreserve(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("restoring %s: %w", c.Name, err)
		}

		// Verify restored checksum
		if c.SHA256 != "" {
			hash, err := bootstrap.HashFile(dstPath)
			if err != nil {
				return nil, fmt.Errorf("verifying restored %s: %w", c.Name, err)
			}
			if hash != c.SHA256 {
				return nil, fmt.Errorf("restored %s checksum mismatch", c.Name)
			}
		}

		result.RestoredComponents = append(result.RestoredComponents, c.Name)
	}

	result.Duration = time.Since(start)
	return result, nil
}
