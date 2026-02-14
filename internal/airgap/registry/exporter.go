package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

// ExportConfig configures a module export from an online registry.
type ExportConfig struct {
	// Modules is the list of module names to export. If empty, all modules
	// listed by the source client are exported.
	Modules []string

	// OutputDir is the directory to write exported modules to, using the
	// FilesystemBackend layout ({moduleName}/{version}/module.zip).
	OutputDir string

	// Client is the registry client to download from (usually an HTTPClient).
	Client resolver.RegistryClient
}

// ExportResult summarizes an export operation.
type ExportResult struct {
	ModulesExported int
	VersionsExported int
	TotalSize       int64
	Errors          []error
}

// Export downloads modules from a remote registry into a local directory.
// The output uses the FilesystemBackend layout so it can be imported directly.
func Export(ctx context.Context, cfg ExportConfig) (*ExportResult, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("output directory is required")
	}

	//nolint:gosec // G301: export directory needs to be accessible
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	result := &ExportResult{}

	for _, moduleName := range cfg.Modules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		versions, err := cfg.Client.ListVersions(moduleName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("list versions for %s: %w", moduleName, err))
			continue
		}

		exported := false
		for _, version := range versions {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}

			versionDir := filepath.Join(cfg.OutputDir, moduleName, version)
			//nolint:gosec // G301: module version directory needs to be accessible
			if err := os.MkdirAll(versionDir, 0o755); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("create dir for %s@%s: %w", moduleName, version, err))
				continue
			}

			zipPath := filepath.Join(versionDir, "module.zip")
			if err := cfg.Client.DownloadModule(moduleName, version, zipPath); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("download %s@%s: %w", moduleName, version, err))
				os.RemoveAll(versionDir)
				continue
			}

			info, err := os.Stat(zipPath)
			if err == nil {
				result.TotalSize += info.Size()
			}

			result.VersionsExported++
		}

		if exported || len(versions) > 0 {
			result.ModulesExported++
		}
	}

	return result, nil
}
