package module

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

const mainStarStub = `# ` + "`" + `main(input)` + "`" + ` is the module entrypoint.
def main(input):
    return {"ok": True}
`

func initCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "init <vendor/name>",
		Short: "Scaffold a new Starlark module",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !manifest.ValidModuleName(name) {
				return fmt.Errorf("module: name %q must be namespaced vendor/pkg", name)
			}
			target := dir
			if target == "" {
				target = filepath.Base(name)
			}
			if entries, _ := os.ReadDir(target); len(entries) > 0 {
				return fmt.Errorf("module: %s is not empty", target)
			}
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			m := &manifest.Manifest{
				Name: name, Version: "0.1.0", Type: manifest.TypeStarlark,
				Entrypoint: "main.star", Description: "A Keystone Core module",
				License: "Apache-2.0",
			}
			my, err := manifest.MarshalManifest(m)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(target, "manifest.yaml"), my, 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(target, "main.star"), []byte(mainStarStub), 0o600); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "scaffolded %s in %s\n", name, target)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "target directory (default: base of the module name)")
	return cmd
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate a module manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			m, _, err := readManifest(dir)
			if err != nil {
				return err
			}
			if err := m.Validate(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s@%s\n", m.Name, m.Version)
			return nil
		},
	}
}

func buildCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "build [dir]",
		Short: "Package a module directory as a ZIP",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			m, _, err := readManifest(dir)
			if err != nil {
				return err
			}
			if err := m.Validate(); err != nil {
				return err
			}
			if out == "" {
				out = fmt.Sprintf("%s-%s.zip", filepath.Base(m.Name), m.Version)
			}
			if err := zipDir(dir, out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "output zip path")
	return cmd
}

// zipDir writes every regular file under dir into out, skipping the
// output file itself and build/lock/sig artifacts.
func zipDir(dir, out string) error {
	absOut, _ := filepath.Abs(out)
	f, err := os.Create(out) //nolint:gosec // operator-supplied output path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	return filepath.Walk(dir, func(p string, fi os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if fi.IsDir() {
			return nil
		}
		if ap, _ := filepath.Abs(p); ap == absOut {
			return nil // never zip the output into itself
		}
		switch filepath.Ext(p) {
		case ".sig", ".lock":
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(p) //nolint:gosec // walking the operator-supplied module dir
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()
		_, err = io.Copy(w, src)
		return err
	})
}
