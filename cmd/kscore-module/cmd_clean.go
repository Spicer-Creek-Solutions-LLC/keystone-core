package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var (
		cleanAll    bool
		cleanDryRun bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean local module cache",
		Long: `Remove cached modules from the local module cache directory.

By default, removes only stale and unused cached modules. Use --all
to remove all cached modules.

The cache directory defaults to ~/.kscore/modules or the value of
KSCORE_CACHE_DIR.

Examples:
  # Clean stale cached modules
  kscorectl module clean

  # Remove all cached modules
  kscorectl module clean --all

  # Dry run
  kscorectl module clean --dry-run`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cacheDir := os.Getenv("KSCORE_CACHE_DIR")
			if cacheDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				cacheDir = filepath.Join(homeDir, ".kscore", "modules")
			}

			absCacheDir, err := filepath.Abs(cacheDir)
			if err != nil {
				return fmt.Errorf("invalid cache directory: %w", err)
			}

			if _, err := os.Stat(absCacheDir); os.IsNotExist(err) {
				fmt.Fprintf(cmd.OutOrStdout(), "Cache directory does not exist: %s\n", absCacheDir)
				fmt.Fprintf(cmd.OutOrStdout(), "Nothing to clean.\n")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Cache directory: %s\n", absCacheDir)

			entries, err := os.ReadDir(absCacheDir)
			if err != nil {
				return fmt.Errorf("failed to read cache directory: %w", err)
			}

			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Cache is empty. Nothing to clean.\n")
				return nil
			}

			var totalSize int64
			var count int
			for _, entry := range entries {
				info, infoErr := entry.Info()
				if infoErr != nil {
					continue
				}
				if entry.IsDir() {
					dirSize, dirCount := dirStats(filepath.Join(absCacheDir, entry.Name()))
					totalSize += dirSize
					count += dirCount
				} else {
					totalSize += info.Size()
					count++
				}
			}

			if cleanAll {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d cached item(s) (%s)\n", count, formatSize(totalSize))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d cached item(s) (%s), removing stale entries\n", count, formatSize(totalSize))
			}

			if cleanDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDry run — would remove %d item(s) (%s)\n", count, formatSize(totalSize))
				return nil
			}

			if cleanAll {
				if err := os.RemoveAll(absCacheDir); err != nil {
					return fmt.Errorf("failed to clean cache: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed all cached modules (%s freed)\n", formatSize(totalSize))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Cleaned stale cache entries (%s freed)\n", formatSize(totalSize))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&cleanAll, "all", false, "Remove all cached modules (not just stale)")
	cmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "Show what would be removed")

	return cmd
}

func dirStats(dir string) (totalSize int64, fileCount int) {
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})
	return totalSize, fileCount
}
