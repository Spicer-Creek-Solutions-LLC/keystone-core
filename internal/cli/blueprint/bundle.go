package blueprint

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// bundleCmd produces a plain .tar.gz of a validated blueprint
// directory. v1.0 ships unsigned bundles only; signed bundles +
// air-gap mirrors are v1.5 per PROJECT-DETAILS §4.17.
func bundleCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "bundle [dir]",
		Short: "Package a validated blueprint into a .tar.gz",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(argDir(args))
			if err != nil {
				return err
			}
			if out == "" {
				out = m.Metadata.Name + "-" + m.Metadata.Version + ".tar.gz"
			}
			if err := writeBundle(m.SourcePath, out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bundled %s@%s → %s\n", m.Metadata.Name, m.Metadata.Version, out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "output path (default: <name>-<version>.tar.gz)")
	return cmd
}

func writeBundle(srcDir, outPath string) (err error) {
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // caller-supplied output path
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	gz := gzip.NewWriter(f)
	defer func() {
		if cerr := gz.Close(); err == nil {
			err = cerr
		}
	}()
	tw := tar.NewWriter(gz)
	defer func() {
		if cerr := tw.Close(); err == nil {
			err = cerr
		}
	}()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// #nosec G122 G304 -- author-CLI bundling a validated
		// blueprint dir; symlink TOCTOU isn't a threat model here.
		in, err := os.Open(path) //nolint:gosec
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		_, err = io.Copy(tw, in)
		return err
	})
}
