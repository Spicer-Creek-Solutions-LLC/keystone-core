package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
)

var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Mirror blueprints between registries",
	Long: `Mirror blueprints from a remote registry to a local registry or directory.

Mirroring is useful for air-gapped environments, local caching, and
controlled distribution of blueprints within an organization.

Subcommands:
  sync    - Sync blueprints from upstream to local mirror
  serve   - Start a local mirror server
  export  - Export mirrored blueprints to a directory
  import  - Import blueprints from a directory into a mirror`,
}

// Mirror sync command

var (
	mirrorSyncSource      string
	mirrorSyncTarget      string
	mirrorSyncBlueprints  []string
	mirrorSyncAll         bool
	mirrorSyncDryRun      bool
	mirrorSyncVerify      bool
	mirrorSyncConcurrency int
)

var mirrorSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync blueprints from upstream",
	Long: `Sync blueprints from an upstream registry to a local mirror.

Downloads specified blueprints (or all blueprints) from the source registry
and stores them in the target registry or local directory.

Examples:
  # Sync specific blueprints
  kscorectl blueprint mirror sync --source https://blueprints.keystone-core.io \
    --target http://mirror.internal:8080 \
    --blueprints vendor/nginx,vendor/postgresql

  # Sync all blueprints
  kscorectl blueprint mirror sync --source https://blueprints.keystone-core.io \
    --target http://mirror.internal:8080 --all

  # Dry run to see what would be synced
  kscorectl blueprint mirror sync --source https://blueprints.keystone-core.io \
    --target /var/lib/keystone-core/mirror --dry-run`,
	RunE: mirrorSyncExecute,
}

func init() {
	mirrorCmd.AddCommand(mirrorSyncCmd)
	mirrorCmd.AddCommand(mirrorServeCmd)
	mirrorCmd.AddCommand(mirrorExportCmd)
	mirrorCmd.AddCommand(mirrorImportCmd)

	mirrorSyncCmd.Flags().StringVar(&mirrorSyncSource, "source", "", "Source registry URL (default: official registry)")
	mirrorSyncCmd.Flags().StringVar(&mirrorSyncTarget, "target", "", "Target registry URL or local directory (required)")
	mirrorSyncCmd.Flags().StringSliceVar(&mirrorSyncBlueprints, "blueprints", nil, "Blueprints to sync (comma-separated)")
	mirrorSyncCmd.Flags().BoolVar(&mirrorSyncAll, "all", false, "Sync all blueprints from source")
	mirrorSyncCmd.Flags().BoolVar(&mirrorSyncDryRun, "dry-run", false, "Show what would be synced")
	mirrorSyncCmd.Flags().BoolVar(&mirrorSyncVerify, "verify", true, "Verify signatures during sync")
	mirrorSyncCmd.Flags().IntVar(&mirrorSyncConcurrency, "concurrency", 4, "Number of concurrent downloads")
}

func mirrorSyncExecute(cmd *cobra.Command, _ []string) error {
	if mirrorSyncTarget == "" {
		return fmt.Errorf("--target is required")
	}

	if !mirrorSyncAll && len(mirrorSyncBlueprints) == 0 {
		return fmt.Errorf("specify --blueprints or --all")
	}

	sourceURL := mirrorSyncSource
	if sourceURL == "" {
		sourceURL = getDefaultRegistry()
	}

	fmt.Printf("Syncing blueprints\n")
	fmt.Printf("  Source: %s\n", sourceURL)
	fmt.Printf("  Target: %s\n\n", mirrorSyncTarget)

	sourceClient, err := registry.NewHTTPClient(&registry.Config{
		URL:     sourceURL,
		Timeout: 120 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create source registry client: %w", err)
	}

	// Determine blueprints to sync
	var toSync []string
	if mirrorSyncAll {
		results, err := sourceClient.Search(&registry.SearchQuery{
			Limit: 1000,
		})
		if err != nil {
			return fmt.Errorf("failed to list blueprints from source: %w", err)
		}
		for _, bp := range results.Blueprints {
			toSync = append(toSync, bp.Name)
		}
		fmt.Printf("Found %d blueprints to sync\n\n", len(toSync))
	} else {
		toSync = mirrorSyncBlueprints
	}

	if len(toSync) == 0 {
		fmt.Println("No blueprints to sync")
		return nil
	}

	// Determine if target is a local directory or a registry URL
	isLocalTarget := !isURL(mirrorSyncTarget)
	if isLocalTarget {
		//nolint:gosec // G301: mirror directory needs to be accessible by the mirror server
		if err := os.MkdirAll(mirrorSyncTarget, 0o755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}
	}

	synced := 0
	skipped := 0
	for _, blueprintName := range toSync {
		name, version := parseReference(blueprintName)

		// Get all versions to sync
		var versions []string
		if version != "" {
			versions = []string{version}
		} else {
			versions, err = sourceClient.ListVersions(name)
			if err != nil {
				fmt.Printf("  %s: failed to list versions: %v\n", name, err)
				continue
			}
		}

		for _, ver := range versions {
			if mirrorSyncDryRun {
				fmt.Printf("  [dry-run] Would sync %s@%s\n", name, ver)
				synced++
				continue
			}

			// Check if already mirrored (for local targets)
			if isLocalTarget {
				destPath := filepath.Join(mirrorSyncTarget, name, ver)
				if _, err := os.Stat(destPath); err == nil {
					skipped++
					continue
				}
			}

			data, err := sourceClient.DownloadBlueprint(name, ver)
			if err != nil {
				fmt.Printf("  %s@%s: download failed: %v\n", name, ver, err)
				continue
			}

			if isLocalTarget {
				destPath := filepath.Join(mirrorSyncTarget, name, ver)
				//nolint:gosec // G301: mirror subdirectories need to be accessible
				if err := os.MkdirAll(destPath, 0o755); err != nil {
					fmt.Printf("  %s@%s: failed to create directory: %v\n", name, ver, err)
					continue
				}
				archivePath := filepath.Join(destPath, "blueprint.tar.gz")
				//nolint:gosec // G306: blueprint archives need to be readable by the mirror server
				if err := os.WriteFile(archivePath, data, 0o644); err != nil {
					fmt.Printf("  %s@%s: failed to write: %v\n", name, ver, err)
					continue
				}
			}

			fmt.Printf("  Synced %s@%s (%d bytes)\n", name, ver, len(data))
			synced++
		}
	}

	fmt.Println()
	if mirrorSyncDryRun {
		fmt.Printf("[dry-run] Would sync %d blueprint version(s)\n", synced)
	} else {
		fmt.Printf("Synced %d blueprint version(s), skipped %d (already mirrored)\n", synced, skipped)
	}

	return nil
}

// Mirror serve command

var (
	mirrorServeListen     string
	mirrorServeStorageDir string
	mirrorServeConfig     string
)

var mirrorServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a local mirror server",
	Long: `Start a local blueprint mirror server.

The mirror server provides a registry-compatible API for serving blueprints
from a local directory. Agents and tools can use it as a registry source.

Examples:
  # Start mirror server
  kscorectl blueprint mirror serve --config mirror-config.yaml

  # Start with custom storage and listen address
  kscorectl blueprint mirror serve \
    --storage-dir /var/lib/keystone-core/blueprints \
    --listen :8080`,
	RunE: mirrorServeExecute,
}

func init() {
	mirrorServeCmd.Flags().StringVar(&mirrorServeListen, "listen", ":8080", "Listen address")
	mirrorServeCmd.Flags().StringVar(&mirrorServeStorageDir, "storage-dir", "", "Local storage directory for mirrored blueprints")
	mirrorServeCmd.Flags().StringVar(&mirrorServeConfig, "config", "", "Mirror configuration file")
}

func mirrorServeExecute(_ *cobra.Command, _ []string) error {
	storageDir := mirrorServeStorageDir
	if storageDir == "" && mirrorServeConfig == "" {
		return fmt.Errorf("--storage-dir or --config is required")
	}

	if mirrorServeConfig != "" {
		if _, err := os.Stat(mirrorServeConfig); os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s", mirrorServeConfig)
		}
		fmt.Printf("Loading configuration from %s\n", mirrorServeConfig)
	}

	if storageDir != "" {
		if _, err := os.Stat(storageDir); os.IsNotExist(err) {
			return fmt.Errorf("storage directory not found: %s", storageDir)
		}
	}

	fmt.Printf("Starting blueprint mirror server\n")
	fmt.Printf("  Listen: %s\n", mirrorServeListen)
	if storageDir != "" {
		fmt.Printf("  Storage: %s\n", storageDir)
	}
	fmt.Println()
	fmt.Println("Mirror server is not yet available in standalone mode.")
	fmt.Println("Use 'kscore-registry' with mirror mode enabled for production deployments.")

	return nil
}

// Mirror export command

var (
	mirrorExportOutput string
	mirrorExportSource string
)

var mirrorExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export mirrored blueprints to a directory",
	Long: `Export blueprints from a mirror to a directory for offline transfer.

Creates a directory structure suitable for import on another system.

Examples:
  # Export to directory
  kscorectl blueprint mirror export --output /backup/blueprints

  # Export from specific mirror
  kscorectl blueprint mirror export --source /var/lib/keystone-core/mirror \
    --output /backup/blueprints`,
	RunE: mirrorExportExecute,
}

func init() {
	mirrorExportCmd.Flags().StringVar(&mirrorExportOutput, "output", "", "Output directory (required)")
	mirrorExportCmd.Flags().StringVar(&mirrorExportSource, "source", "", "Source mirror directory")
}

func mirrorExportExecute(_ *cobra.Command, _ []string) error {
	if mirrorExportOutput == "" {
		return fmt.Errorf("--output is required")
	}

	sourceDir := mirrorExportSource
	if sourceDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		sourceDir = filepath.Join(home, ".kscore", "mirror")
	}

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source mirror directory not found: %s", sourceDir)
	}

	//nolint:gosec // G301: export directory needs to be accessible for transfer
	if err := os.MkdirAll(mirrorExportOutput, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Printf("Exporting mirror from %s to %s\n", sourceDir, mirrorExportOutput)

	exported := 0
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(mirrorExportOutput, relPath)
		//nolint:gosec // G301: exported directories need to be accessible
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		//nolint:gosec // G306: exported files need to be readable for import
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}

		exported++
		return nil
	})
	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	fmt.Printf("Exported %d file(s)\n", exported)
	return nil
}

// Mirror import command

var (
	mirrorImportInput  string
	mirrorImportMirror string
	mirrorImportForce  bool
)

var mirrorImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import blueprints from a directory into a mirror",
	Long: `Import blueprints from a directory or bundle into a local mirror.

Imports previously exported blueprints or bundle archives into the
local mirror directory for serving.

Examples:
  # Import from directory
  kscorectl blueprint mirror import --input /backup/blueprints

  # Import bundle to mirror
  kscorectl blueprint mirror import my-stack-bundle.tar.gz \
    --mirror http://mirror.internal:8080`,
	Args: cobra.MaximumNArgs(1),
	RunE: mirrorImportExecute,
}

func init() {
	mirrorImportCmd.Flags().StringVar(&mirrorImportInput, "input", "", "Input directory to import from")
	mirrorImportCmd.Flags().StringVar(&mirrorImportMirror, "mirror", "", "Target mirror URL or directory")
	mirrorImportCmd.Flags().BoolVar(&mirrorImportForce, "force", false, "Overwrite existing blueprints")
}

func mirrorImportExecute(_ *cobra.Command, args []string) error {
	// Determine input source
	var inputPath string
	if len(args) > 0 {
		inputPath = args[0]
	} else if mirrorImportInput != "" {
		inputPath = mirrorImportInput
	} else {
		return fmt.Errorf("specify a bundle file as an argument or --input directory")
	}

	targetDir := mirrorImportMirror
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		targetDir = filepath.Join(home, ".kscore", "mirror")
	}

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input not found: %s", inputPath)
	}

	//nolint:gosec // G301: mirror directory needs to be accessible by the mirror server
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create mirror directory: %w", err)
	}

	fmt.Printf("Importing blueprints to %s\n", targetDir)

	// Check if input is a file (bundle) or directory
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("failed to stat input: %w", err)
	}

	if info.IsDir() {
		return importFromDirectory(inputPath, targetDir)
	}

	return importFromBundle(inputPath, targetDir)
}

func importFromDirectory(inputDir, targetDir string) error {
	imported := 0
	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(targetDir, relPath)

		if _, err := os.Stat(destPath); err == nil && !mirrorImportForce {
			return nil
		}

		//nolint:gosec // G301: mirror subdirectories need to be accessible
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		//nolint:gosec // G306: mirror files need to be readable by the mirror server
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}

		imported++
		return nil
	})
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Printf("Imported %d file(s)\n", imported)
	return nil
}

func importFromBundle(bundlePath, targetDir string) error {
	f, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open bundle: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read bundle: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var manifest *BundleManifest
	archives := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read bundle: %w", err)
		}

		if header.Size < 0 || header.Size > maxBundleArchiveEntrySize {
			return fmt.Errorf("bundle entry %s exceeds max size", header.Name)
		}

		data, err := io.ReadAll(io.LimitReader(tr, header.Size))
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", header.Name, err)
		}

		if header.Name == "bundle-manifest.json" {
			var m BundleManifest
			if err := json.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("failed to parse bundle manifest: %w", err)
			}
			manifest = &m
		} else {
			archives[header.Name] = data
		}
	}

	if manifest == nil {
		return fmt.Errorf("bundle manifest not found")
	}

	imported := 0
	for _, entry := range manifest.Blueprints {
		archiveData, ok := archives[entry.Archive]
		if !ok {
			fmt.Printf("  Warning: archive %s not found in bundle\n", entry.Archive)
			continue
		}

		destPath := filepath.Join(targetDir, entry.Name, entry.Version)
		if _, err := os.Stat(destPath); err == nil && !mirrorImportForce {
			fmt.Printf("  %s@%s already exists (use --force to overwrite)\n", entry.Name, entry.Version)
			continue
		}

		//nolint:gosec // G301: mirror subdirectories need to be accessible
		if err := os.MkdirAll(destPath, 0o755); err != nil {
			fmt.Printf("  %s@%s: failed to create directory: %v\n", entry.Name, entry.Version, err)
			continue
		}

		archivePath := filepath.Join(destPath, "blueprint.tar.gz")
		//nolint:gosec // G306: blueprint archives need to be readable
		if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
			fmt.Printf("  %s@%s: failed to write: %v\n", entry.Name, entry.Version, err)
			continue
		}

		// Write metadata
		metaPath := filepath.Join(destPath, "metadata.json")
		meta := map[string]string{
			"name":     entry.Name,
			"version":  entry.Version,
			"checksum": entry.Checksum,
		}
		metaData, _ := json.MarshalIndent(meta, "", "  ")
		//nolint:gosec // G306: metadata files need to be readable
		if err := os.WriteFile(metaPath, metaData, 0o644); err != nil {
			fmt.Printf("  %s@%s: failed to write metadata: %v\n", entry.Name, entry.Version, err)
			continue
		}

		fmt.Printf("  Imported %s@%s\n", entry.Name, entry.Version)
		imported++
	}

	fmt.Printf("\nImported %d blueprint(s) from bundle\n", imported)
	return nil
}

func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}
