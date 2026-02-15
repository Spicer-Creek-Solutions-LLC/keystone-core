package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

// ScanResult describes differences between two build directories.
type ScanResult struct {
	ChangedBinaries []string
	NewBinaries     []string
	RemovedBinaries []string
	ChangedModules  []string
	NewModules      []string
	RemovedModules  []string
	MigrationFiles  []string
}

// ScanChanges compares two build directories (old and new) and returns what changed.
// Both directories should follow the build/{os}/{arch}/ layout for binaries,
// and optionally contain a modules/ subdirectory.
func ScanChanges(oldDir, newDir string) (*ScanResult, error) {
	result := &ScanResult{}

	// Compare binaries
	oldBins, err := hashFiles(oldDir)
	if err != nil {
		return nil, fmt.Errorf("scanning old directory: %w", err)
	}
	newBins, err := hashFiles(newDir)
	if err != nil {
		return nil, fmt.Errorf("scanning new directory: %w", err)
	}

	for name, newHash := range newBins {
		oldHash, exists := oldBins[name]
		if !exists {
			result.NewBinaries = append(result.NewBinaries, name)
		} else if oldHash != newHash {
			result.ChangedBinaries = append(result.ChangedBinaries, name)
		}
	}
	for name := range oldBins {
		if _, exists := newBins[name]; !exists {
			result.RemovedBinaries = append(result.RemovedBinaries, name)
		}
	}

	// Compare modules if directories exist
	oldModDir := filepath.Join(oldDir, "modules")
	newModDir := filepath.Join(newDir, "modules")
	oldModsExist := dirExists(oldModDir)
	newModsExist := dirExists(newModDir)

	if oldModsExist || newModsExist {
		var oldMods, newMods map[string]string
		if oldModsExist {
			oldMods, err = hashFiles(oldModDir)
			if err != nil {
				return nil, fmt.Errorf("scanning old modules: %w", err)
			}
		} else {
			oldMods = make(map[string]string)
		}
		if newModsExist {
			newMods, err = hashFiles(newModDir)
			if err != nil {
				return nil, fmt.Errorf("scanning new modules: %w", err)
			}
		} else {
			newMods = make(map[string]string)
		}

		for name, newHash := range newMods {
			oldHash, exists := oldMods[name]
			if !exists {
				result.NewModules = append(result.NewModules, name)
			} else if oldHash != newHash {
				result.ChangedModules = append(result.ChangedModules, name)
			}
		}
		for name := range oldMods {
			if _, exists := newMods[name]; !exists {
				result.RemovedModules = append(result.RemovedModules, name)
			}
		}
	}

	// Check for migration files in new directory
	migrationsDir := filepath.Join(newDir, "migrations")
	if dirExists(migrationsDir) {
		entries, err := os.ReadDir(migrationsDir)
		if err != nil {
			return nil, fmt.Errorf("reading migrations: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				result.MigrationFiles = append(result.MigrationFiles, e.Name())
			}
		}
	}

	return result, nil
}

// HasChanges returns true if any differences were detected.
func (r *ScanResult) HasChanges() bool {
	return len(r.ChangedBinaries) > 0 || len(r.NewBinaries) > 0 || len(r.RemovedBinaries) > 0 ||
		len(r.ChangedModules) > 0 || len(r.NewModules) > 0 || len(r.RemovedModules) > 0 ||
		len(r.MigrationFiles) > 0
}

// Summary returns a human-readable summary of changes.
func (r *ScanResult) Summary() string {
	var parts []string
	if n := len(r.ChangedBinaries); n > 0 {
		parts = append(parts, fmt.Sprintf("%d changed binaries", n))
	}
	if n := len(r.NewBinaries); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new binaries", n))
	}
	if n := len(r.RemovedBinaries); n > 0 {
		parts = append(parts, fmt.Sprintf("%d removed binaries", n))
	}
	if n := len(r.ChangedModules); n > 0 {
		parts = append(parts, fmt.Sprintf("%d changed modules", n))
	}
	if n := len(r.NewModules); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new modules", n))
	}
	if n := len(r.MigrationFiles); n > 0 {
		parts = append(parts, fmt.Sprintf("%d migration files", n))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

func hashFiles(dir string) (map[string]string, error) {
	result := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		hash, err := bootstrap.HashFile(path)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", entry.Name(), err)
		}
		result[entry.Name()] = hash
	}
	return result, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
