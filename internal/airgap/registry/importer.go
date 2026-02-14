package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
	"github.com/shawnbutts/keystone-core/internal/registry/storage"
)

// ImportResult summarizes what was imported.
type ImportResult struct {
	ModulesImported    int
	BlueprintsImported int
	Skipped            int
	Errors             []error
}

// ImportFromBootstrap extracts a bootstrap package and imports its modules
// and blueprints into the offline registry.
func (r *Registry) ImportFromBootstrap(ctx context.Context, archivePath string) (*ImportResult, error) {
	tmpDir, err := os.MkdirTemp("", "airgap-import-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := bootstrap.ExtractArchive(archivePath, tmpDir); err != nil {
		return nil, fmt.Errorf("extract archive: %w", err)
	}

	// Read the manifest to know what's in the package
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath) //nolint:gosec // G304: path constructed from trusted temp dir
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest bootstrap.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	result := &ImportResult{}

	// Import modules
	modulesDir := filepath.Join(tmpDir, "modules")
	if _, err := os.Stat(modulesDir); err == nil {
		modResult, err := r.ImportModulesFromDir(ctx, modulesDir)
		if err != nil {
			return nil, fmt.Errorf("import modules: %w", err)
		}
		result.ModulesImported += modResult.ModulesImported
		result.Skipped += modResult.Skipped
		result.Errors = append(result.Errors, modResult.Errors...)
	}

	// Import blueprints
	blueprintsDir := filepath.Join(tmpDir, "blueprints")
	if _, err := os.Stat(blueprintsDir); err == nil {
		bpResult, err := r.ImportBlueprintsFromDir(ctx, blueprintsDir)
		if err != nil {
			return nil, fmt.Errorf("import blueprints: %w", err)
		}
		result.BlueprintsImported += bpResult.BlueprintsImported
		result.Skipped += bpResult.Skipped
		result.Errors = append(result.Errors, bpResult.Errors...)
	}

	if r.config.AutoIndex {
		if err := r.Reindex(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("reindex: %w", err))
		}
	}

	return result, nil
}

// ImportModulesFromDir imports module .zip files from a directory.
// Each .zip file is expected to be named {moduleName}-{version}.zip or just {name}.zip.
// It also supports the FilesystemBackend layout where modules are in subdirectories.
func (r *Registry) ImportModulesFromDir(ctx context.Context, dir string) (*ImportResult, error) {
	result := &ImportResult{}

	// First check for FilesystemBackend-style layout (nested dirs with module.zip)
	modules, _ := discoverModulesInDir(dir, dir, "")
	if len(modules) > 0 {
		return r.importFromFilesystemLayout(ctx, dir, modules)
	}

	// Fall back to flat .zip files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}

		zipPath := filepath.Join(dir, e.Name())
		moduleName, version := parseModuleFilename(e.Name())
		if moduleName == "" {
			result.Errors = append(result.Errors, fmt.Errorf("cannot parse module name from: %s", e.Name()))
			continue
		}

		exists, _ := r.backend.VersionExists(ctx, moduleName, version)
		if exists {
			result.Skipped++
			continue
		}

		zipData, err := os.ReadFile(zipPath) //nolint:gosec // G304: path from trusted directory walk
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read %s: %w", e.Name(), err))
			continue
		}

		hash, err := bootstrap.HashFile(zipPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("hash %s: %w", e.Name(), err))
			continue
		}

		_, err = r.backend.Publish(ctx, &storage.PublishRequest{
			ModuleName: moduleName,
			Version:    version,
			ZipData:    bytes.NewReader(zipData),
			ZipSize:    int64(len(zipData)),
			Hash:       hash,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("publish %s@%s: %w", moduleName, version, err))
			continue
		}

		result.ModulesImported++
	}

	if r.config.AutoIndex && result.ModulesImported > 0 {
		if err := r.Reindex(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("reindex: %w", err))
		}
	}

	return result, nil
}

// importFromFilesystemLayout imports modules from a directory that uses the
// FilesystemBackend layout: {dir}/{moduleName}/{version}/module.zip
func (r *Registry) importFromFilesystemLayout(ctx context.Context, dir string, modules []string) (*ImportResult, error) {
	result := &ImportResult{}

	for _, moduleName := range modules {
		moduleDir := filepath.Join(dir, moduleName)
		versionEntries, err := os.ReadDir(moduleDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read module dir %s: %w", moduleName, err))
			continue
		}

		for _, ve := range versionEntries {
			if !ve.IsDir() {
				continue
			}
			version := ve.Name()
			versionDir := filepath.Join(moduleDir, version)

			zipPath := filepath.Join(versionDir, "module.zip")
			if _, err := os.Stat(zipPath); err != nil {
				continue
			}

			exists, _ := r.backend.VersionExists(ctx, moduleName, version)
			if exists {
				result.Skipped++
				continue
			}

			zipData, err := os.ReadFile(zipPath) //nolint:gosec // G304: path from trusted directory walk
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("read %s@%s: %w", moduleName, version, err))
				continue
			}

			hash, err := bootstrap.HashFile(zipPath)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("hash %s@%s: %w", moduleName, version, err))
				continue
			}

			// Read manifest if available
			var manifestData []byte
			manifestPath := filepath.Join(versionDir, "module.yaml")
			if data, err := os.ReadFile(manifestPath); err == nil { //nolint:gosec // G304: path from trusted dir
				manifestData = data
			}

			// Read signature if available
			var signature string
			sigPath := filepath.Join(versionDir, "module.sig")
			if data, err := os.ReadFile(sigPath); err == nil { //nolint:gosec // G304: path from trusted dir
				signature = string(data)
			}

			// Read info for metadata
			var description string
			var tags []string
			var deps map[string]string
			infoPath := filepath.Join(versionDir, "module.info")
			if data, err := os.ReadFile(infoPath); err == nil { //nolint:gosec // G304: path from trusted dir
				var info storage.StoredModule
				if json.Unmarshal(data, &info) == nil {
					description = info.Description
					tags = info.Tags
					deps = info.Dependencies
				}
			}

			_, err = r.backend.Publish(ctx, &storage.PublishRequest{
				ModuleName:   moduleName,
				Version:      version,
				ZipData:      bytes.NewReader(zipData),
				ZipSize:      int64(len(zipData)),
				Manifest:     manifestData,
				Signature:    signature,
				Hash:         hash,
				Description:  description,
				Tags:         tags,
				Dependencies: deps,
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("publish %s@%s: %w", moduleName, version, err))
				continue
			}

			result.ModulesImported++
		}
	}

	if r.config.AutoIndex && result.ModulesImported > 0 {
		if err := r.Reindex(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("reindex: %w", err))
		}
	}

	return result, nil
}

// ImportBlueprintsFromDir copies blueprint directories (or .tar.gz archives)
// into the registry's blueprints directory.
func (r *Registry) ImportBlueprintsFromDir(_ context.Context, dir string) (*ImportResult, error) {
	result := &ImportResult{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	bpDir := r.BlueprintsDir()

	for _, e := range entries {
		name := e.Name()
		srcPath := filepath.Join(dir, name)
		dstPath := filepath.Join(bpDir, name)

		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("copy blueprint %s: %w", name, err))
				continue
			}
			result.BlueprintsImported++
		} else if strings.HasSuffix(name, ".tar.gz") {
			// Extract tar.gz into blueprint directory
			bpName := strings.TrimSuffix(name, ".tar.gz")
			extractDir := filepath.Join(bpDir, bpName)
			if err := bootstrap.ExtractArchive(srcPath, extractDir); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("extract blueprint %s: %w", name, err))
				continue
			}
			result.BlueprintsImported++
		}
	}

	return result, nil
}

// parseModuleFilename tries to extract module name and version from a filename.
// Supports formats: "modulename-1.0.0.zip" or "vendor-package-1.0.0.zip"
func parseModuleFilename(filename string) (name, version string) {
	base := strings.TrimSuffix(filename, ".zip")
	if base == filename {
		return "", ""
	}

	// Try to find version suffix (last segment matching semver-like pattern)
	parts := strings.Split(base, "-")
	for i := len(parts) - 1; i >= 1; i-- {
		candidate := parts[i]
		if candidate != "" && candidate[0] >= '0' && candidate[0] <= '9' {
			name = strings.Join(parts[:i], "-")
			version = strings.Join(parts[i:], "-")
			// Convert hyphens to slashes for vendor/package format (first hyphen only)
			if idx := strings.Index(name, "-"); idx > 0 {
				name = name[:idx] + "/" + name[idx+1:]
			}
			return name, version
		}
	}

	return base, "0.0.0"
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	//nolint:gosec // G301: blueprint directory needs to be accessible
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath) //nolint:gosec // G304: path from trusted dir walk
			if err != nil {
				return err
			}
			//nolint:gosec // G306: blueprint files need to be readable
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}
