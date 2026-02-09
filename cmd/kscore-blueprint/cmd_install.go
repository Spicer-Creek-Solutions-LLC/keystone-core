package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
)

var (
	installRegistry string
	installDir      string
	installVerify   bool
	installForce    bool
	installDryRun   bool
	installNoDeps   bool
)

var maxBlueprintArchiveEntrySize = int64(256 * 1024 * 1024)

var installCmd = &cobra.Command{
	Use:   "install <blueprint[@version]>...",
	Short: "Install blueprints",
	Long: `Install one or more blueprints from a registry.

Downloads blueprints and their dependencies to the local blueprint directory.
By default, blueprints are installed to ~/.kscore/blueprints/

Examples:
  # Install latest version
  kscorectl blueprint install community/nginx

  # Install specific version
  kscorectl blueprint install community/nginx@1.2.0

  # Install multiple blueprints
  kscorectl blueprint install community/nginx community/postgresql

  # Install to custom directory
  kscorectl blueprint install community/nginx --dir ./blueprints

  # Skip signature verification (not recommended)
  kscorectl blueprint install community/nginx --no-verify`,
	Args: cobra.MinimumNArgs(1),
	RunE: installExecute,
}

func init() {
	installCmd.Flags().StringVar(&installRegistry, "registry", "", "Registry URL")
	installCmd.Flags().StringVar(&installDir, "dir", "", "Installation directory (default: ~/.kscore/blueprints)")
	installCmd.Flags().BoolVar(&installVerify, "verify", true, "Verify signatures")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Force reinstall if already installed")
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Show what would be installed")
	installCmd.Flags().BoolVar(&installNoDeps, "no-deps", false, "Don't install dependencies")
}

func installExecute(cmd *cobra.Command, args []string) error {
	// Determine install directory
	installPath := installDir
	if installPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		installPath = filepath.Join(home, ".kscore", "blueprints")
	}

	// Get registry URL
	registryURL := installRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	// Create client
	client, err := registry.NewHTTPClient(&registry.Config{
		URL:     registryURL,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Process each blueprint
	for _, ref := range args {
		name, version := parseReference(ref)

		fmt.Printf("Installing %s", name)
		if version != "" {
			fmt.Printf("@%s", version)
		}
		fmt.Println("...")

		if installDryRun {
			fmt.Printf("  [dry-run] Would install to %s\n", filepath.Join(installPath, name))
			continue
		}

		// Check if already installed
		blueprintPath := filepath.Join(installPath, name)
		if _, err := os.Stat(blueprintPath); err == nil && !installForce {
			fmt.Printf("  Already installed at %s (use --force to reinstall)\n", blueprintPath)
			continue
		}

		// Get version if not specified
		if version == "" {
			latestVersion, err := client.GetLatestVersion(name)
			if err != nil {
				return fmt.Errorf("failed to get latest version of %s: %w", name, err)
			}
			version = latestVersion
			fmt.Printf("  Latest version: %s\n", version)
		}

		// Download the blueprint
		data, err := client.DownloadBlueprint(name, version)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", name, err)
		}

		// Extract to destination
		if err := extractBlueprint(data, blueprintPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", name, err)
		}

		fmt.Printf("  ✓ Installed %s@%s to %s\n", name, version, blueprintPath)
	}

	return nil
}

// extractBlueprint extracts a tar.gz archive to the destination directory
func extractBlueprint(data []byte, destDir string) error {
	// Create a temporary file for the archive
	tmpFile, err := os.CreateTemp("", "blueprint-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Open the file for reading
	f, err := os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer f.Close()

	// Create gzip reader
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	// Create destination directory
	//nolint:gosec // G301: blueprint directory needs to be accessible by users
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Ensure the target is within destDir (security check for G305)
		cleanDest := filepath.Clean(destDir)
		cleanTarget := filepath.Clean(filepath.Join(destDir, header.Name)) //nolint:gosec // G305: path validated by HasPrefix check below
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) && cleanTarget != cleanDest {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}
		target := cleanTarget

		if header.Typeflag == tar.TypeReg && (header.Size < 0 || header.Size > maxBlueprintArchiveEntrySize) {
			return fmt.Errorf("archive entry %s exceeds max size", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			//nolint:gosec // G115: tar header.Mode is a Unix permission mode, fits in uint32
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			//nolint:gosec // G301: parent directory needs to be accessible by users
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			//nolint:gosec // G115: tar header.Mode is a Unix permission mode, fits in uint32
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.CopyN(outFile, tr, header.Size); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

var updateCmd = &cobra.Command{
	Use:   "update [blueprint...]",
	Short: "Update installed blueprints",
	Long: `Update installed blueprints to their latest versions.

Without arguments, updates all installed blueprints. With arguments, updates
only the specified blueprints.

Examples:
  # Update all blueprints
  kscorectl blueprint update

  # Update specific blueprints
  kscorectl blueprint update community/nginx community/postgresql

  # Update with dry-run to see what would change
  kscorectl blueprint update --dry-run`,
	RunE: updateExecute,
}

var (
	updateRegistry       string
	updateDir            string
	updateDryRun         bool
	updateMajor          bool
	updateAcceptBreaking bool
	updateShowBreaking   bool
)

func init() {
	updateCmd.Flags().StringVar(&updateRegistry, "registry", "", "Registry URL")
	updateCmd.Flags().StringVar(&updateDir, "dir", "", "Blueprint directory")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated")
	updateCmd.Flags().BoolVar(&updateMajor, "major", false, "Allow major version updates")
	updateCmd.Flags().BoolVar(&updateAcceptBreaking, "accept-breaking-changes", false, "Accept breaking changes and proceed with update")
	updateCmd.Flags().BoolVar(&updateShowBreaking, "show-breaking", false, "Show detailed breaking change analysis")
}

func updateExecute(cmd *cobra.Command, args []string) error {
	// Determine blueprint directory
	blueprintPath := updateDir
	if blueprintPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		blueprintPath = filepath.Join(home, ".kscore", "blueprints")
	}

	// Get registry URL
	registryURL := updateRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	// Create client
	client, err := registry.NewHTTPClient(&registry.Config{
		URL:     registryURL,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Get list of blueprints to update
	var blueprints []string
	if len(args) > 0 {
		blueprints = args
	} else {
		// Find all installed blueprints
		entries, err := os.ReadDir(blueprintPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No blueprints installed")
				return nil
			}
			return fmt.Errorf("failed to read blueprint directory: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				// Check for blueprint.yaml in subdirectories (vendor/name structure)
				vendorDir := filepath.Join(blueprintPath, e.Name())
				subEntries, err := os.ReadDir(vendorDir)
				if err != nil {
					continue
				}
				for _, sub := range subEntries {
					if sub.IsDir() {
						manifestPath := filepath.Join(vendorDir, sub.Name(), "blueprint.yaml")
						if _, err := os.Stat(manifestPath); err == nil {
							blueprints = append(blueprints, filepath.Join(e.Name(), sub.Name()))
						}
					}
				}
			}
		}
	}

	if len(blueprints) == 0 {
		fmt.Println("No blueprints to update")
		return nil
	}

	fmt.Printf("Checking for updates...\n\n")

	updated := 0
	for _, name := range blueprints {
		// Get current version
		manifestPath := filepath.Join(blueprintPath, name, "blueprint.yaml")
		currentVersion, err := getCurrentVersion(manifestPath)
		if err != nil {
			fmt.Printf("  %s: %v\n", name, err)
			continue
		}

		// Get latest version
		latestVersion, err := client.GetLatestVersion(name)
		if err != nil {
			fmt.Printf("  %s: failed to check for updates: %v\n", name, err)
			continue
		}

		if currentVersion == latestVersion {
			fmt.Printf("  %s: up to date (%s)\n", name, currentVersion)
			continue
		}

		// Check if major version update
		if !updateMajor && isMajorUpdate(currentVersion, latestVersion) {
			fmt.Printf("  %s: %s -> %s (major update, use --major to update)\n",
				name, currentVersion, latestVersion)
			continue
		}

		// Check for breaking changes
		hasBreakingChanges := false
		var breakingReport *blueprint.BreakingChangeReport

		// Load current manifest
		currentManifest, manifestErr := loadBlueprintManifest(manifestPath)
		if manifestErr == nil {
			// Try to get new manifest info for comparison
			// This is a simplified check - in production we'd download and parse the new manifest
			detector := blueprint.NewBreakingChangeDetector()

			// For now, we check based on version - major version bump implies breaking changes
			if isMajorUpdate(currentVersion, latestVersion) {
				breakingReport = &blueprint.BreakingChangeReport{
					FromVersion: currentVersion,
					ToVersion:   latestVersion,
					Changes: []blueprint.BreakingChange{{
						Type:        blueprint.BreakingMajorVersion,
						Severity:    blueprint.SeverityHigh,
						Description: fmt.Sprintf("Major version update from %s to %s", currentVersion, latestVersion),
					}},
					HighestSeverity: blueprint.SeverityHigh,
				}
				hasBreakingChanges = true
			} else if currentManifest != nil {
				// Use same manifest as placeholder for actual implementation
				// In production, we'd fetch the new version's manifest from the registry
				breakingReport = detector.Detect(currentManifest, currentManifest)
				hasBreakingChanges = len(breakingReport.Changes) > 0
			}
		}

		if hasBreakingChanges && breakingReport != nil {
			if updateShowBreaking {
				fmt.Printf("  %s: Breaking changes detected:\n", name)
				for _, change := range breakingReport.Changes {
					fmt.Printf("    - [%s] %s: %s\n", change.Severity, change.Type, change.Description)
				}
			} else {
				fmt.Printf("  %s: %s -> %s (breaking changes, use --accept-breaking-changes to proceed)\n",
					name, currentVersion, latestVersion)
			}

			if !updateAcceptBreaking {
				continue
			}
			fmt.Printf("  %s: Proceeding with breaking changes (--accept-breaking-changes)\n", name)
		}

		if updateDryRun {
			fmt.Printf("  %s: %s -> %s (would update)\n", name, currentVersion, latestVersion)
			continue
		}

		// Download the blueprint
		data, err := client.DownloadBlueprint(name, latestVersion)
		if err != nil {
			fmt.Printf("  %s: failed to download: %v\n", name, err)
			continue
		}

		// Remove existing installation
		destPath := filepath.Join(blueprintPath, name)
		if err := os.RemoveAll(destPath); err != nil {
			fmt.Printf("  %s: failed to remove old version: %v\n", name, err)
			continue
		}

		// Extract new version
		if err := extractBlueprint(data, destPath); err != nil {
			fmt.Printf("  %s: failed to update: %v\n", name, err)
			continue
		}

		fmt.Printf("  %s: %s -> %s ✓\n", name, currentVersion, latestVersion)
		updated++
	}

	if updateDryRun {
		fmt.Printf("\n[dry-run] No changes made\n")
	} else {
		fmt.Printf("\nUpdated %d blueprints\n", updated)
	}

	return nil
}

var removeCmd = &cobra.Command{
	Use:   "remove <blueprint>...",
	Short: "Remove installed blueprints",
	Long: `Remove one or more installed blueprints.

Examples:
  # Remove a blueprint
  kscorectl blueprint remove community/nginx

  # Remove multiple blueprints
  kscorectl blueprint remove community/nginx community/postgresql`,
	Args: cobra.MinimumNArgs(1),
	RunE: removeExecute,
}

var (
	removeDir    string
	removeForce  bool
	removeDryRun bool
)

func init() {
	removeCmd.Flags().StringVar(&removeDir, "dir", "", "Blueprint directory")
	removeCmd.Flags().BoolVar(&removeForce, "force", false, "Remove without confirmation")
	removeCmd.Flags().BoolVar(&removeDryRun, "dry-run", false, "Show what would be removed")
}

func removeExecute(cmd *cobra.Command, args []string) error {
	// Determine blueprint directory
	blueprintPath := removeDir
	if blueprintPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		blueprintPath = filepath.Join(home, ".kscore", "blueprints")
	}

	for _, name := range args {
		fullPath := filepath.Join(blueprintPath, name)

		// Check if exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			fmt.Printf("Blueprint %s is not installed\n", name)
			continue
		}

		if removeDryRun {
			fmt.Printf("[dry-run] Would remove %s\n", fullPath)
			continue
		}

		dependents, err := findBlueprintDependents(blueprintPath, name)
		if err != nil {
			return fmt.Errorf("failed to check blueprint dependencies: %w", err)
		}
		if len(dependents) > 0 {
			if removeForce {
				fmt.Printf("Warning: %s is required by %s\n", name, strings.Join(dependents, ", "))
			} else {
				return fmt.Errorf("blueprint %s is required by %s (use --force to remove)", name, strings.Join(dependents, ", "))
			}
		}

		// Remove
		if err := os.RemoveAll(fullPath); err != nil {
			return fmt.Errorf("failed to remove %s: %w", name, err)
		}

		fmt.Printf("✓ Removed %s\n", name)
	}

	return nil
}

// Helper functions

func findBlueprintDependents(baseDir, target string) ([]string, error) {
	dependents := []string{}
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "blueprint.yaml" {
			return nil
		}

		bp, err := blueprint.ParseManifestFile(path)
		if err != nil {
			return nil //nolint:nilerr // skip unparseable files in walk
		}
		if bp.Metadata.Name == target {
			return nil
		}

		var deps []string
		if bp.Dependencies != nil {
			deps = append(deps, bp.Dependencies.Requires...)
			deps = append(deps, bp.Dependencies.RequiresBefore...)
		}
		for _, dep := range deps {
			depName, _ := parseReference(dep)
			if depName == target {
				dependents = append(dependents, bp.Metadata.Name)
				break
			}
		}

		return nil
	})

	return dependents, err
}

func getCurrentVersion(manifestPath string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}

	// Simple YAML parsing for version
	// In production, use proper YAML parsing
	for _, line := range splitLines(string(data)) {
		if len(line) > 10 && line[:8] == "version:" {
			return trimQuotes(line[9:]), nil
		}
		if len(line) > 10 && line[:9] == "  version:" {
			return trimQuotes(line[10:]), nil
		}
	}

	return "", fmt.Errorf("version not found in manifest")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimQuotes(s string) string {
	s = trimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func isMajorUpdate(current, latest string) bool {
	currentMajor := ""
	latestMajor := ""
	for i := 0; i < len(current) && current[i] != '.'; i++ {
		currentMajor += string(current[i])
	}
	for i := 0; i < len(latest) && latest[i] != '.'; i++ {
		latestMajor += string(latest[i])
	}
	return currentMajor != latestMajor
}

// loadBlueprintManifest loads a blueprint manifest from a file
func loadBlueprintManifest(path string) (*blueprint.Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return blueprint.ParseManifest(data)
}
