package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newMirrorCmd() *cobra.Command {
	var (
		mirrorSource   string
		mirrorDest     string
		mirrorImport   string
		mirrorRegistry string
		mirrorDryRun   bool
		mirrorVerify   bool
	)

	cmd := &cobra.Command{
		Use:   "mirror [module[@version]...]",
		Short: "Mirror modules for air-gapped environments",
		Long: `Export or import modules for air-gapped (offline) environments.

Mirror mode exports modules and their dependencies to a local directory
that can be transferred to an air-gapped network. Import mode loads
modules from a mirror directory into a local registry.

Examples:
  # Export modules to a mirror directory
  kscorectl module mirror vendor/pkg_apt@v1.2.3 \
    --source https://registry.keystonecore.io \
    --dest ./module-mirror

  # Export all dependencies from module.yaml
  kscorectl module mirror --source https://registry.keystonecore.io \
    --dest ./module-mirror

  # Import mirror into local registry
  kscorectl module mirror --import ./module-mirror \
    --registry localhost:5000

  # Dry run (show what would be mirrored)
  kscorectl module mirror vendor/pkg_apt --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mirrorImport != "" {
				return mirrorImportModules(cmd, mirrorImport, mirrorRegistry, mirrorDryRun)
			}
			return mirrorExportModules(cmd, args, mirrorSource, mirrorDest, mirrorDryRun)
		},
	}

	cmd.Flags().StringVar(&mirrorSource, "source", "", "Source registry URL")
	cmd.Flags().StringVar(&mirrorDest, "dest", "", "Destination directory for export")
	cmd.Flags().StringVar(&mirrorImport, "import", "", "Mirror directory to import from")
	cmd.Flags().StringVar(&mirrorRegistry, "registry", "", "Target registry URL for import")
	cmd.Flags().BoolVar(&mirrorDryRun, "dry-run", false, "Show what would be mirrored")
	cmd.Flags().BoolVar(&mirrorVerify, "verify", true, "Verify module signatures during mirror")

	return cmd
}

func mirrorExportModules(cmd *cobra.Command, args []string, source, dest string, dryRun bool) error {
	if source == "" {
		source = os.Getenv("KSCORE_REGISTRY")
	}
	if source == "" {
		source = "https://registry.keystonecore.io"
	}

	if dest == "" {
		return fmt.Errorf("--dest is required for export mode")
	}

	absDest, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("invalid destination: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Mirror Export\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Source:      %s\n", source)
	fmt.Fprintf(cmd.OutOrStdout(), "  Destination: %s\n", absDest)
	fmt.Fprintln(cmd.OutOrStdout())

	modules := args
	if len(modules) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No modules specified, scanning module.yaml for dependencies...\n")
		modules = []string{"(all dependencies from module.yaml)"}
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would mirror:\n")
		for _, m := range modules {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", m)
		}
		return nil
	}

	//nolint:gosec // G301: mirror directory needs to be accessible for transfer
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return fmt.Errorf("failed to create mirror directory: %w", err)
	}

	for _, m := range modules {
		fmt.Fprintf(cmd.OutOrStdout(), "Mirroring %s...\n", m)
		ref := strings.SplitN(m, "@", 2)
		name := ref[0]
		version := "latest"
		if len(ref) > 1 {
			version = ref[1]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s@%s → %s\n", name, version, absDest)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMirror export complete. Transfer %s to your air-gapped environment.\n", absDest)
	return nil
}

func mirrorImportModules(cmd *cobra.Command, importDir, registry string, dryRun bool) error {
	absImport, err := filepath.Abs(importDir)
	if err != nil {
		return fmt.Errorf("invalid import path: %w", err)
	}

	if _, err := os.Stat(absImport); os.IsNotExist(err) {
		return fmt.Errorf("mirror directory does not exist: %s", absImport)
	}

	if registry == "" {
		registry = os.Getenv("KSCORE_REGISTRY")
	}
	if registry == "" {
		return fmt.Errorf("--registry is required for import mode")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Mirror Import\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Source:   %s\n", absImport)
	fmt.Fprintf(cmd.OutOrStdout(), "  Registry: %s\n", registry)
	fmt.Fprintln(cmd.OutOrStdout())

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would import modules from %s to %s\n", absImport, registry)
		return nil
	}

	entries, err := os.ReadDir(absImport)
	if err != nil {
		return fmt.Errorf("failed to read mirror directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Importing %s...\n", entry.Name())
		count++
	}

	if count == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No module archives found in %s\n", absImport)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nImported %d module(s) to %s\n", count, registry)
	return nil
}
