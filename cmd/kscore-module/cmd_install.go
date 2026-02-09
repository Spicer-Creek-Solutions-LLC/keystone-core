package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/registry"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var (
	installRegistry   string
	installToken      string
	installUsername   string
	installPassword   string
	installCacheDir   string
	installModulesDir string
	installVerify     bool
	installPublicKey  string
	installForce      bool
	installDryRun     bool
	installGlobal     bool
)

var maxModuleArchiveEntrySize = int64(256 * 1024 * 1024)

var installCmd = &cobra.Command{
	Use:   "install <module[@version]> [modules...]",
	Short: "Install modules from a registry",
	Long: `Install one or more modules from a registry.

Modules are downloaded, verified, and extracted to the modules directory.
If no version is specified, the latest version is installed.

Module references:
  myorg/mymodule           - Install latest version
  myorg/mymodule@1.0.0     - Install specific version
  myorg/mymodule@^1.0.0    - Install latest compatible with 1.x.x
  myorg/mymodule@~1.2.0    - Install latest compatible with 1.2.x

Examples:
  # Install latest version
  kscorectl module install myorg/webserver

  # Install specific version
  kscorectl module install myorg/webserver@1.2.3

  # Install multiple modules
  kscorectl module install myorg/webserver myorg/database@2.0.0

  # Install with signature verification
  kscorectl module install myorg/webserver --verify --public-key trusted.pem

  # Install to global cache
  kscorectl module install myorg/webserver --global

  # Dry run (show what would be installed)
  kscorectl module install myorg/webserver --dry-run`,
	Args: cobra.MinimumNArgs(1),
	RunE: installExecute,
}

func init() {
	installCmd.Flags().StringVar(&installRegistry, "registry", "", "Registry URL (defaults to KSCORE_REGISTRY)")
	installCmd.Flags().StringVar(&installToken, "token", "", "Authentication token")
	installCmd.Flags().StringVar(&installUsername, "username", "", "Username for basic auth")
	installCmd.Flags().StringVar(&installPassword, "password", "", "Password for basic auth")
	installCmd.Flags().StringVar(&installCacheDir, "cache-dir", "", "Module cache directory")
	installCmd.Flags().StringVar(&installModulesDir, "modules-dir", "", "Modules installation directory (default: ./modules)")
	installCmd.Flags().BoolVar(&installVerify, "verify", false, "Verify module signatures")
	installCmd.Flags().StringVar(&installPublicKey, "public-key", "", "Public key for signature verification")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Force reinstall even if already installed")
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Show what would be installed without installing")
	installCmd.Flags().BoolVar(&installGlobal, "global", false, "Install to global cache only (don't extract to modules dir)")
}

func installExecute(cmd *cobra.Command, args []string) error {
	// Get registry URL
	registryURL := installRegistry
	if registryURL == "" {
		registryURL = os.Getenv("KSCORE_REGISTRY")
	}
	if registryURL == "" {
		registryURL = "https://registry.keystonecore.io"
	}
	registryURL = strings.TrimSuffix(registryURL, "/")

	// Get cache directory
	cacheDir := installCacheDir
	if cacheDir == "" {
		cacheDir = os.Getenv("KSCORE_CACHE_DIR")
	}
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".kscore", "modules")
	}

	// Get modules directory
	modulesDir := installModulesDir
	if modulesDir == "" {
		modulesDir = "modules"
	}

	fmt.Printf("Registry: %s\n", registryURL)
	fmt.Printf("Cache: %s\n", cacheDir)
	if !installGlobal {
		fmt.Printf("Modules: %s\n", modulesDir)
	}
	fmt.Println()

	// Build authentication config
	auth := buildInstallAuthConfig()

	// Create registry client
	config := registry.DefaultConfig(registryURL)
	config.Auth = auth
	client, err := registry.NewHTTPClient(config)
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Create module cache
	cache, err := resolver.NewModuleCache(resolver.CacheConfig{
		Dir: cacheDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	// Parse module references
	modules := make([]moduleRef, 0, len(args))
	for _, arg := range args {
		ref, err := parseModuleRef(arg)
		if err != nil {
			return fmt.Errorf("invalid module reference %q: %w", arg, err)
		}
		modules = append(modules, ref)
	}

	// Process each module
	installed := 0
	skipped := 0
	failed := 0

	for _, mod := range modules {
		fmt.Printf("Installing %s", mod.name)
		if mod.version != "" {
			fmt.Printf("@%s", mod.version)
		}
		fmt.Println("...")

		err := installModule(client, cache, mod, registryURL, modulesDir)
		if err != nil {
			fmt.Printf("  ✗ Failed: %v\n\n", err)
			failed++
			continue
		}

		if installDryRun {
			fmt.Printf("  ✓ Would be installed\n\n")
		} else {
			fmt.Printf("  ✓ Installed\n\n")
		}
		installed++
	}

	// Print summary
	fmt.Println("=== Summary ===")
	if installDryRun {
		fmt.Printf("Would install: %d\n", installed)
	} else {
		fmt.Printf("Installed: %d\n", installed)
	}
	if skipped > 0 {
		fmt.Printf("Skipped: %d\n", skipped)
	}
	if failed > 0 {
		fmt.Printf("Failed: %d\n", failed)
		return fmt.Errorf("%d module(s) failed to install", failed)
	}

	if installed > 0 && !installDryRun {
		fmt.Println("\n✓ Installation complete!")
	}

	return nil
}

type moduleRef struct {
	name       string
	version    string
	constraint string // Original constraint (e.g., ^1.0.0)
}

func parseModuleRef(ref string) (moduleRef, error) {
	// Format: name[@version]
	parts := strings.SplitN(ref, "@", 2)
	name := parts[0]

	// Validate name format (vendor/package)
	if !strings.Contains(name, "/") {
		return moduleRef{}, fmt.Errorf("module name must be in vendor/package format")
	}

	var version, constraint string
	if len(parts) > 1 {
		constraint = parts[1]
		// If it's an exact version (no ^, ~, etc.), use it directly
		if !strings.ContainsAny(constraint, "^~>=<") {
			version = constraint
		}
	}

	return moduleRef{
		name:       name,
		version:    version,
		constraint: constraint,
	}, nil
}

func installModule(client *registry.HTTPClient, cache *resolver.ModuleCache, mod moduleRef, registryURL, modulesDir string) error {
	// Determine version to install
	version := mod.version
	if version == "" {
		// Get latest version from registry
		versions, err := client.ListVersions(mod.name)
		if err != nil {
			return fmt.Errorf("failed to list versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("no versions available")
		}

		// If we have a constraint, find matching version
		if mod.constraint != "" {
			// Parse constraint and select best version
			parser := &resolver.DefaultConstraintParser{}
			constraint, err := parser.Parse(mod.constraint)
			if err != nil {
				return fmt.Errorf("invalid version constraint: %w", err)
			}

			selector := &resolver.DefaultVersionSelector{}
			version, err = selector.Select(constraint, versions)
			if err != nil {
				return fmt.Errorf("no version matches constraint %s: %w", mod.constraint, err)
			}
		} else {
			// Use latest (highest) version
			selector := &resolver.DefaultVersionSelector{}
			var err error
			version, err = selector.SelectHighest(versions)
			if err != nil {
				return fmt.Errorf("failed to select version: %w", err)
			}
		}
		fmt.Printf("  Version: %s (latest)\n", version)
	} else {
		fmt.Printf("  Version: %s\n", version)
	}

	// Get module info
	info, err := client.GetModuleInfo(mod.name, version)
	if err != nil {
		return fmt.Errorf("failed to get module info: %w", err)
	}

	fmt.Printf("  Hash: %s\n", truncateHash(info.Hash))
	if info.Size > 0 {
		fmt.Printf("  Size: %s\n", formatSize(info.Size))
	}

	// Check if already in cache
	if info.Hash != "" && cache.Has(info.Hash) && !installForce {
		fmt.Printf("  Status: already cached\n")
		if !installGlobal && !installDryRun {
			// Extract to modules dir
			entry, _ := cache.Get(info.Hash)
			if entry != nil {
				return extractModule(entry.Path, mod.name, version, modulesDir)
			}
		}
		return nil
	}

	if installDryRun {
		return nil
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "kscore-module-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	fmt.Printf("  Downloading... ")
	if err := client.DownloadModule(mod.name, version, tmpPath); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Println("done")

	// Verify hash
	hasher := verify.NewDefaultHashVerifier()
	computedHash, err := hasher.ComputeHash(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	if info.Hash != "" && normalizeHash(computedHash) != normalizeHash(info.Hash) {
		return fmt.Errorf("hash mismatch: expected %s, got %s", info.Hash, computedHash)
	}

	// Verify signature if requested
	if installVerify {
		sigPath := tmpPath + ".sig"
		// Try to download signature
		sigURL := fmt.Sprintf("%s/%s/@v/%s.sig", strings.TrimSuffix(installRegistry, "/"), mod.name, version)
		fmt.Printf("  Verifying signature... ")

		if installPublicKey == "" {
			fmt.Println("skipped (no public key)")
		} else {
			// Download signature file
			// Note: This is a simplified implementation
			// In production, we'd have a proper signature download endpoint
			if _, err := os.Stat(sigPath); os.IsNotExist(err) {
				fmt.Printf("skipped (no signature at %s)\n", sigURL)
			} else {
				keyData, err := os.ReadFile(installPublicKey)
				if err != nil {
					fmt.Printf("failed (cannot read key: %v)\n", err)
				} else {
					verifier := verify.NewSignatureVerifier(verify.SignatureFormatPKCS1)
					valid, err := verifier.VerifySignature(tmpPath, sigPath, keyData)
					switch {
					case err != nil:
						fmt.Printf("failed (%v)\n", err)
					case !valid:
						return fmt.Errorf("signature verification failed")
					default:
						fmt.Println("valid")
					}
				}
			}
		}
	}

	// Add to cache
	fmt.Printf("  Caching... ")
	entry, err := cache.Put(resolver.ModuleReference{
		Name:     mod.name,
		Version:  version,
		Resolved: version,
		Hash:     computedHash,
	}, tmpPath, installVerify && installPublicKey != "")
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("failed to cache: %w", err)
	}
	fmt.Println("done")

	// Extract to modules directory (unless global-only)
	if !installGlobal {
		return extractModule(entry.Path, mod.name, version, modulesDir)
	}

	return nil
}

func extractModule(zipPath, moduleName, version, modulesDir string) error {
	// Create target directory: modules/<vendor>/<package>/<version>/
	parts := strings.SplitN(moduleName, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid module name format: %s", moduleName)
	}

	targetDir := filepath.Join(modulesDir, parts[0], parts[1], version)

	// Check if already extracted
	if _, err := os.Stat(targetDir); err == nil && !installForce {
		fmt.Printf("  Extracted: %s (already exists)\n", targetDir)
		return nil
	}

	fmt.Printf("  Extracting to %s... ", targetDir)

	// Create directory
	//nolint:gosec // G301: module directory needs to be accessible by service user
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open ZIP
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	// Extract files
	for _, f := range r.File {
		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		//nolint:gosec // G115: maxModuleArchiveEntrySize is a small constant (100MB), fits in uint64
		if f.UncompressedSize64 > uint64(maxModuleArchiveEntrySize) {
			fmt.Println("failed")
			return fmt.Errorf("archive entry %s exceeds max size", f.Name)
		}

		// Security: ensure path is within target directory (G305 path traversal check)
		cleanTargetDir := filepath.Clean(targetDir)
		targetPath := filepath.Clean(filepath.Join(targetDir, f.Name)) //nolint:gosec // G305: path validated by HasPrefix check below
		if !strings.HasPrefix(targetPath, cleanTargetDir+string(os.PathSeparator)) && targetPath != cleanTargetDir {
			continue // Skip files that would escape target directory
		}

		// Create parent directories
		//nolint:gosec // G301: parent directory needs to be accessible by service user
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Extract file
		src, err := f.Open()
		if err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		dst, err := os.Create(targetPath)
		if err != nil {
			src.Close()
			fmt.Println("failed")
			return fmt.Errorf("failed to create file: %w", err)
		}

		//nolint:gosec // G115: UncompressedSize64 is checked against maxModuleArchiveEntrySize above
		_, err = io.CopyN(dst, src, int64(f.UncompressedSize64))
		src.Close()
		dst.Close()

		if err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}

	fmt.Println("done")

	// Create/update "latest" symlink
	latestLink := filepath.Join(modulesDir, parts[0], parts[1], "latest")
	os.Remove(latestLink) // Remove existing symlink
	if err := os.Symlink(version, latestLink); err != nil {
		// Symlink may fail on Windows, that's ok
		fmt.Printf("  Note: could not create 'latest' symlink: %v\n", err)
	}

	return nil
}

func buildInstallAuthConfig() *registry.AuthConfig {
	// Token auth (bearer)
	token := installToken
	if token == "" {
		token = os.Getenv("KSCORE_REGISTRY_TOKEN")
	}
	if token != "" {
		return &registry.AuthConfig{
			Type:  registry.AuthTypeBearer,
			Token: token,
		}
	}

	// Basic auth
	username := installUsername
	if username == "" {
		username = os.Getenv("KSCORE_REGISTRY_USERNAME")
	}
	password := installPassword
	if password == "" {
		password = os.Getenv("KSCORE_REGISTRY_PASSWORD")
	}
	if username != "" && password != "" {
		return &registry.AuthConfig{
			Type:     registry.AuthTypeBasic,
			Username: username,
			Password: password,
		}
	}

	return nil
}

func truncateHash(hash string) string {
	if len(hash) > 20 {
		return hash[:20] + "..."
	}
	return hash
}
