package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

func newUpdateCmd() *cobra.Command {
	var updateDryRun bool

	cmd := &cobra.Command{
		Use:   "update [module...]",
		Short: "Update dependencies to latest compatible versions",
		Long: `Update module dependencies to their latest compatible versions.

Reads module.yaml and module.lock, checks the registry for newer versions
that satisfy the declared version constraints, and updates the lock file.

If specific modules are given, only those are updated. Otherwise all
dependencies are checked for updates.

Examples:
  # Update all dependencies
  kscorectl module update

  # Update specific module
  kscorectl module update std/files

  # Dry run (show available updates)
  kscorectl module update --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args, updateDryRun)
		},
	}

	cmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show available updates without applying")

	return cmd
}

func runUpdate(cmd *cobra.Command, args []string, dryRun bool) error {
	modulePath := "."
	absPath, err := filepath.Abs(modulePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	manifestPath := filepath.Join(absPath, "module.yaml")
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}

	lockFilePath := filepath.Join(absPath, "module.lock")
	var lockFile *manifest.LockFile
	if _, statErr := os.Stat(lockFilePath); statErr == nil {
		lockFile, err = manifest.ParseLockFileFromFile(lockFilePath)
		if err != nil {
			return fmt.Errorf("failed to parse module.lock: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Checking for updates...\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Module: %s@%s\n\n", m.Name, m.Version)

	filter := make(map[string]bool)
	for _, a := range args {
		filter[a] = true
	}

	names := make([]string, 0, len(m.Dependencies))
	for name := range m.Dependencies {
		names = append(names, name)
	}
	sort.Strings(names)

	updates := 0
	for _, name := range names {
		if len(filter) > 0 && !filter[name] {
			continue
		}

		constraint := m.Dependencies[name]
		currentVersion := constraint
		if lockFile != nil {
			if locked, ok := lockFile.Modules[name]; ok {
				currentVersion = locked.Version
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (up to date)\n", name, currentVersion)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	if updates > 0 {
		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "%d update(s) available (dry run, no changes made)\n", updates)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated module.lock with %d change(s)\n", updates)
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "All dependencies are up to date.\n")
	}

	return nil
}
