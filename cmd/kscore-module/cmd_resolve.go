package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

var (
	resolveLockFile      string
	resolveUpdate        bool
	resolveAllowPre      bool
	resolveTimeout       time.Duration
	resolveCacheDir      string
	resolveOffline       bool
)

var resolveCmd = &cobra.Command{
	Use:   "resolve [path]",
	Short: "Resolve module dependencies",
	Long: `Resolve module dependencies and generate a lock file.

The resolution process:
  1. Parses module.yaml for dependencies
  2. Queries registry for available versions
  3. Resolves version constraints using MVS algorithm
  4. Detects circular dependencies
  5. Generates module.lock with pinned versions and hashes

If a lock file exists, it will be used for fast resolution.
Use --update to fetch latest compatible versions.

Examples:
  # Resolve dependencies
  kscorectl module resolve

  # Update to latest compatible versions
  kscorectl module resolve --update

  # Allow pre-release versions
  kscorectl module resolve --allow-prerelease

  # Offline mode (use cache only)
  kscorectl module resolve --offline`,
	Args: cobra.MaximumNArgs(1),
	RunE: resolveExecute,
}

func init() {
	resolveCmd.Flags().StringVar(&resolveLockFile, "lock-file", "module.lock", "Lock file path")
	resolveCmd.Flags().BoolVar(&resolveUpdate, "update", false, "Update to latest compatible versions")
	resolveCmd.Flags().BoolVar(&resolveAllowPre, "allow-prerelease", false, "Include pre-release versions")
	resolveCmd.Flags().DurationVar(&resolveTimeout, "timeout", 5*time.Minute, "Resolution timeout")
	resolveCmd.Flags().StringVar(&resolveCacheDir, "cache-dir", "", "Module cache directory")
	resolveCmd.Flags().BoolVar(&resolveOffline, "offline", false, "Offline mode (cache only)")
}

func resolveExecute(cmd *cobra.Command, args []string) error {
	// Determine path
	modulePath := "."
	if len(args) > 0 {
		modulePath = args[0]
	}

	absPath, err := filepath.Abs(modulePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Parse module.yaml
	manifestPath := filepath.Join(absPath, "module.yaml")
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}

	fmt.Printf("Resolving dependencies for: %s v%s\n", m.Name, m.Version)

	if len(m.Dependencies) == 0 {
		fmt.Println("\n✓ No dependencies to resolve")
		return nil
	}

	fmt.Printf("Dependencies: %d\n\n", len(m.Dependencies))

	// Check for existing lock file
	lockFilePath := filepath.Join(absPath, resolveLockFile)
	var existingLock *manifest.LockFile

	if !resolveUpdate {
		if _, err := os.Stat(lockFilePath); err == nil {
			existingLock, err = manifest.ParseLockFileFromFile(lockFilePath)
			if err != nil {
				fmt.Printf("Warning: failed to parse existing lock file: %v\n", err)
			} else {
				fmt.Printf("Using existing lock file: %s\n", resolveLockFile)
			}
		}
	}

	// Create resolver
	// Note: In a full implementation, this would use a real registry client.
	// For now, we use a mock that only works with local dependencies or existing lock file.
	registryClient := &mockRegistryClient{
		modules: make(map[string]map[string]*resolver.ModuleInfo),
	}

	// Use temp dir for cache if not specified
	cacheDir := resolveCacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "kscore-module-cache")
	}

	res, err := resolver.NewModuleResolver(registryClient, cacheDir)
	if err != nil {
		return fmt.Errorf("failed to create resolver: %w", err)
	}

	// Create resolution request
	request := &resolver.ResolutionRequest{
		RootModule:      m.Name,
		RootVersion:     m.Version,
		Manifest:        m,
		LockFile:        existingLock,
		UseLockFile:     existingLock != nil && !resolveUpdate,
		AllowPrerelease: resolveAllowPre,
	}

	// Resolve dependencies
	startTime := time.Now()
	result, err := res.Resolve(request)
	duration := time.Since(startTime)

	if err != nil {
		// Check if it's because we don't have a registry
		if resolveOffline || registryClient.isEmpty() {
			fmt.Println("\n⚠ Cannot resolve dependencies without registry access.")
			fmt.Println("Options:")
			fmt.Println("  1. Configure a module registry in your config")
			fmt.Println("  2. Use --offline with a valid lock file")
			fmt.Println("  3. Install dependencies manually to the cache")
			return fmt.Errorf("dependency resolution failed: %w", err)
		}
		return fmt.Errorf("resolution failed: %w", err)
	}

	fmt.Printf("Resolved %d dependencies in %s\n\n", len(result.Resolved), duration.Round(time.Millisecond))

	// Print resolved dependencies
	fmt.Println("Resolved dependencies:")
	for _, dep := range result.Resolved {
		status := "  "
		if existingLock != nil {
			if locked, ok := existingLock.Modules[dep.Module.Name]; ok {
				if locked.Version != dep.Module.Resolved {
					status = "↑ " // Upgraded
				}
			} else {
				status = "+ " // New
			}
		}
		fmt.Printf("  %s%s @ %s\n", status, dep.Module.Name, dep.Module.Resolved)
	}

	// Generate lock file
	lockFile := &manifest.LockFile{
		SchemaVersion: 1,
		Modules:       make(map[string]manifest.LockedModule),
	}

	for _, dep := range result.Resolved {
		lockFile.Modules[dep.Module.Name] = manifest.LockedModule{
			Version: dep.Module.Resolved,
			Hash:    dep.Module.Hash,
		}
	}

	// Write lock file
	if err := manifest.WriteLockFile(lockFilePath, lockFile); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	fmt.Printf("\n✓ Lock file written: %s\n", resolveLockFile)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  kscorectl module tree            # View dependency tree\n")
	fmt.Printf("  kscorectl module install         # Download dependencies\n")

	return nil
}

// mockRegistryClient is a placeholder for the real registry client.
// In production, this would query the module registry.
type mockRegistryClient struct {
	modules map[string]map[string]*resolver.ModuleInfo
}

func (c *mockRegistryClient) isEmpty() bool {
	return len(c.modules) == 0
}

func (c *mockRegistryClient) ListVersions(name string) ([]string, error) {
	if versions, ok := c.modules[name]; ok {
		var result []string
		for v := range versions {
			result = append(result, v)
		}
		return result, nil
	}
	return nil, fmt.Errorf("module not found: %s (registry not configured)", name)
}

func (c *mockRegistryClient) GetModuleInfo(name, version string) (*resolver.ModuleInfo, error) {
	if versions, ok := c.modules[name]; ok {
		if info, ok := versions[version]; ok {
			return info, nil
		}
	}
	return nil, fmt.Errorf("module version not found: %s@%s (registry not configured)", name, version)
}

func (c *mockRegistryClient) GetModuleManifest(name, version string) (*manifest.Manifest, error) {
	return nil, fmt.Errorf("module manifest not found: %s@%s (registry not configured)", name, version)
}

func (c *mockRegistryClient) DownloadModule(name, version, destPath string) error {
	return fmt.Errorf("cannot download module: %s@%s (registry not configured)", name, version)
}
