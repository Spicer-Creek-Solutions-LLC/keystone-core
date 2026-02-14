package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	airgapreg "github.com/shawnbutts/keystone-core/internal/airgap/registry"
	"github.com/shawnbutts/keystone-core/pkg/module/registry"
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

	modules := args
	if len(modules) == 0 {
		return fmt.Errorf("at least one module name is required (e.g., vendor/package)")
	}

	// Strip version suffixes for module names; versions are exported in full
	var moduleNames []string
	for _, m := range modules {
		ref := strings.SplitN(m, "@", 2)
		moduleNames = append(moduleNames, ref[0])
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Mirror Export\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Source:      %s\n", source)
	fmt.Fprintf(cmd.OutOrStdout(), "  Destination: %s\n", absDest)
	fmt.Fprintf(cmd.OutOrStdout(), "  Modules:     %s\n", strings.Join(moduleNames, ", "))
	fmt.Fprintln(cmd.OutOrStdout())

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would export:\n")
		for _, m := range moduleNames {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (all versions)\n", m)
		}
		return nil
	}

	client, err := registry.NewHTTPClient(&registry.Config{
		URL:           source,
		Timeout:       60 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
	})
	if err != nil {
		return fmt.Errorf("create registry client: %w", err)
	}

	result, err := airgapreg.Export(context.Background(), airgapreg.ExportConfig{
		Modules:   moduleNames,
		OutputDir: absDest,
		Client:    client,
	})
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Exported %d module(s), %d version(s)\n",
		result.ModulesExported, result.VersionsExported)
	for _, e := range result.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: %v\n", e)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nTransfer %s to your air-gapped environment.\n", absDest)
	return nil
}

func mirrorImportModules(cmd *cobra.Command, importDir, registryDir string, dryRun bool) error {
	absImport, err := filepath.Abs(importDir)
	if err != nil {
		return fmt.Errorf("invalid import path: %w", err)
	}

	if _, err := os.Stat(absImport); os.IsNotExist(err) {
		return fmt.Errorf("mirror directory does not exist: %s", absImport)
	}

	if registryDir == "" {
		registryDir = os.Getenv("KSCORE_REGISTRY")
	}
	if registryDir == "" {
		return fmt.Errorf("--registry is required for import mode (path to offline registry)")
	}

	absRegistry, err := filepath.Abs(registryDir)
	if err != nil {
		return fmt.Errorf("invalid registry path: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Mirror Import\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Source:   %s\n", absImport)
	fmt.Fprintf(cmd.OutOrStdout(), "  Registry: %s\n", absRegistry)
	fmt.Fprintln(cmd.OutOrStdout())

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would import modules from %s to %s\n", absImport, absRegistry)
		return nil
	}

	reg, err := airgapreg.New(airgapreg.Config{RootDir: absRegistry, AutoIndex: true})
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer reg.Close()

	result, err := reg.ImportModulesFromDir(context.Background(), absImport)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Imported %d module(s), skipped %d\n",
		result.ModulesImported, result.Skipped)
	for _, e := range result.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "  warning: %v\n", e)
	}
	return nil
}
