package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// MigrateOptions configures a migration operation.
type MigrateOptions struct {
	// DryRun logs what would be migrated without writing to the destination.
	DryRun bool
}

// Migrate copies all modules from source to dest.
// It iterates all modules/versions in the source, reads their files, and writes them to the destination.
// In dry-run mode it only logs what would be copied.
func Migrate(ctx context.Context, source, dest Backend, opts *MigrateOptions) error {
	if opts == nil {
		opts = &MigrateOptions{}
	}

	// The source must be a FilesystemBackend to enumerate modules, since the Backend
	// interface only supports ListVersions for a known module name. For the general case,
	// we need to discover module names first.
	//
	// For filesystem sources, we walk the directory tree. For other backends, the caller
	// must provide a module list externally. This is a pragmatic choice: the primary
	// migration path is filesystem → cloud.
	fs, ok := source.(*FilesystemBackend)
	if !ok {
		return fmt.Errorf("migration currently only supports filesystem as the source backend")
	}

	modules, err := discoverModules(fs)
	if err != nil {
		return fmt.Errorf("discover modules: %w", err)
	}

	var migrated, skipped, errCount int

	for _, moduleName := range modules {
		versions, err := source.ListVersions(ctx, moduleName)
		if err != nil {
			log.Printf("ERROR: list versions for %s: %v", moduleName, err)
			errCount++
			continue
		}

		for _, version := range versions {
			if opts.DryRun {
				log.Printf("[dry-run] would migrate %s@%s", moduleName, version)
				migrated++
				continue
			}

			if err := migrateVersion(ctx, source, dest, moduleName, version); err != nil {
				log.Printf("ERROR: migrate %s@%s: %v", moduleName, version, err)
				errCount++
				continue
			}

			log.Printf("migrated %s@%s", moduleName, version)
			migrated++
		}
	}

	log.Printf("Migration complete: %d migrated, %d skipped, %d errors", migrated, skipped, errCount)
	if errCount > 0 {
		return fmt.Errorf("%d errors during migration", errCount)
	}
	return nil
}

// migrateVersion copies a single module version from source to dest.
func migrateVersion(ctx context.Context, source, dest Backend, moduleName, version string) error {
	// Get module info
	info, err := source.GetInfo(ctx, moduleName, version)
	if err != nil {
		return fmt.Errorf("get info: %w", err)
	}

	// Get zip
	zipRC, _, err := source.GetZip(ctx, moduleName, version)
	if err != nil {
		return fmt.Errorf("get zip: %w", err)
	}
	defer zipRC.Close()

	// Get manifest
	manifestRC, _, err := source.GetManifest(ctx, moduleName, version)
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}
	manifestData, err := io.ReadAll(manifestRC)
	manifestRC.Close()
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	req := &PublishRequest{
		ModuleName:   moduleName,
		Version:      version,
		ZipData:      zipRC,
		Manifest:     manifestData,
		Signature:    info.Signature,
		Hash:         info.Hash,
		Force:        true, // Overwrite if exists in dest
		Description:  info.Description,
		Dependencies: info.Dependencies,
		Tags:         info.Tags,
		ReleaseNotes: info.ReleaseNotes,
	}

	result, err := dest.Publish(ctx, req)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Verify hash matches
	if result.Hash != info.Hash {
		return fmt.Errorf("hash mismatch after migration: source=%s dest=%s", info.Hash, result.Hash)
	}

	return nil
}

// discoverModules walks the filesystem backend to find all module names.
func discoverModules(fs *FilesystemBackend) ([]string, error) {
	return walkModuleDir(fs.root, fs.root, "")
}

// walkModuleDir recursively walks dirs looking for version directories (those containing module.zip).
// When it finds a version directory, the parent path (relative to root) is a module name.
func walkModuleDir(root, current, rel string) ([]string, error) {
	entries, err := os.ReadDir(current)
	if err != nil {
		return nil, err
	}

	moduleSet := make(map[string]bool)
	var modules []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}
		childPath := filepath.Join(current, entry.Name())

		// Check if this directory is a version dir (contains module.zip)
		zipPath := filepath.Join(childPath, "module.zip")
		if _, err := os.Stat(zipPath); err == nil {
			// This is a version directory; the module name is the relative path of its parent
			if rel != "" && !moduleSet[rel] {
				moduleSet[rel] = true
				modules = append(modules, rel)
			}
			continue
		}

		// Recurse into subdirectories
		sub, err := walkModuleDir(root, childPath, childRel)
		if err != nil {
			return nil, err
		}
		for _, m := range sub {
			if !moduleSet[m] {
				moduleSet[m] = true
				modules = append(modules, m)
			}
		}
	}

	return modules, nil
}
