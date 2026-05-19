package blueprint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// The install/update/remove verbs manage a local blueprint library
// directory (a filesystem folder of blueprint subdirectories). v1.0
// has no remote blueprint registry — registry-backed distribution,
// signed bundles, and air-gap mirrors are v1.5 per PROJECT-DETAILS
// §4.17. The --library flag selects the library root.

func installCmd() *cobra.Command { return libInstall("install", false) }
func updateCmd() *cobra.Command  { return libInstall("update", true) }

func libInstall(use string, allowOverwrite bool) *cobra.Command {
	var library string
	cmd := &cobra.Command{
		Use:   use + " <src-dir>",
		Short: fmt.Sprintf("%s a blueprint into the local library", use),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			m, err := loadManifest(src) // validates before copying
			if err != nil {
				return err
			}
			dst := filepath.Join(library, m.Metadata.Name)
			if _, err := os.Stat(dst); err == nil {
				if !allowOverwrite {
					return fmt.Errorf("%s already in library (use update): %s", m.Metadata.Name, dst)
				}
				if err := os.RemoveAll(dst); err != nil {
					return err
				}
			}
			if err := copyTree(m.SourcePath, dst); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%sed %s@%s → %s\n", use, m.Metadata.Name, m.Metadata.Version, dst)
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "blueprints", "local blueprint library directory")
	return cmd
}

func removeCmd() *cobra.Command {
	var library string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a blueprint from the local library",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dst := filepath.Join(library, name)
			if _, err := os.Stat(dst); err != nil {
				return fmt.Errorf("%s not in library: %w", name, err)
			}
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s from %s\n", name, library)
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "blueprints", "local blueprint library directory")
	return cmd
}

// copyTree recursively copies src to dst (files + dirs, mode bits).
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		in, err := os.Open(path) //nolint:gosec // walking a caller-provided blueprint dir
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // target derived from --library
		if err != nil {
			return err
		}
		defer func() { _ = out.Close() }()
		_, err = io.Copy(out, in)
		return err
	})
}
