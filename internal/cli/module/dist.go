package module

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

func publishCmd(d Deps) *cobra.Command {
	var reg, sigFile string
	cmd := &cobra.Command{
		Use:   "publish <module.zip>",
		Short: "Upload a built (and optionally signed) module to a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zipPath := args[0]
			zipBytes, err := os.ReadFile(zipPath) //nolint:gosec // operator-supplied artifact
			if err != nil {
				return err
			}
			manYAML, err := manifestFromZip(zipBytes)
			if err != nil {
				return err
			}
			if sigFile == "" {
				sigFile = zipPath + ".sig"
			}
			var sig []byte
			if b, rerr := os.ReadFile(sigFile); rerr == nil { //nolint:gosec // operator-supplied sig path
				sig = b
			}
			switch err := d.client(reg).Publish(cmd.Context(), manYAML, zipBytes, sig); {
			case err == nil:
				fmt.Fprintf(cmd.OutOrStdout(), "published to %s\n", reg)
				return nil
			case errors.Is(err, registry.ErrVersionExists):
				return fmt.Errorf("module: version already exists on %s", reg)
			default:
				return err
			}
		},
	}
	cmd.Flags().StringVar(&reg, "registry", "http://localhost:8181", "registry base URL")
	cmd.Flags().StringVar(&sigFile, "sig", "", "detached signature (default <zip>.sig if present)")
	return cmd
}

// manifestFromZip extracts manifest.yaml from a module ZIP.
func manifestFromZip(zipBytes []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("module: open zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name == "manifest.yaml" {
			rc, oerr := f.Open()
			if oerr != nil {
				return nil, oerr
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("module: zip has no manifest.yaml")
}

// resolveRoot resolves root against the registry and returns the
// resolution.
func resolveRoot(ctx context.Context, c RegistryClient, root *manifest.Manifest) (*resolver.Resolution, error) {
	return resolver.New(c, resolver.Config{}).Resolve(ctx, root)
}

func writeLock(res *resolver.Resolution, path string) error {
	lf, err := res.LockFile()
	if err != nil {
		return err
	}
	b, err := manifest.MarshalLockFile(lf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func printResolution(cmd *cobra.Command, res *resolver.Resolution) {
	names := make([]string, 0, len(res.Selected))
	for n := range res.Selected {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", n, res.Selected[n].Version)
	}
}

func resolveCmd(d Deps) *cobra.Command {
	var reg, out string
	cmd := &cobra.Command{
		Use:   "resolve [dir]",
		Short: "Resolve a module's dependency graph",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			m, _, err := readManifest(dir)
			if err != nil {
				return err
			}
			res, err := resolveRoot(cmd.Context(), d.client(reg), m)
			if err != nil {
				return err
			}
			printResolution(cmd, res)
			if out != "" {
				return writeLock(res, out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&reg, "registry", "http://localhost:8181", "registry base URL")
	cmd.Flags().StringVarP(&out, "output", "o", "", "write module.lock to this path")
	return cmd
}

func installCmd(d Deps) *cobra.Command {
	var reg, keyFile, dir string
	cmd := &cobra.Command{
		Use:   "install <vendor/pkg@version> | [dir]",
		Short: "Resolve, download, verify, and lock a module + its deps",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c := d.client(reg)

			var root *manifest.Manifest
			lockDir := "."
			if len(args) == 1 && strings.Contains(args[0], "@") {
				parts := strings.SplitN(args[0], "@", 2)
				if !manifest.ValidModuleName(parts[0]) {
					return fmt.Errorf("module: %q is not a namespaced module", parts[0])
				}
				r, err := rootFor(parts[0], parts[1])
				if err != nil {
					return err
				}
				root = r
				if dir != "" {
					lockDir = dir // where module.lock is written for the spec form
				}
			} else {
				lockDir = dirArg(args)
				m, _, err := readManifest(lockDir)
				if err != nil {
					return err
				}
				root = m
			}

			res, err := resolveRoot(ctx, c, root)
			if err != nil {
				return err
			}

			store, err := cas.New("")
			if err != nil {
				return err
			}
			var tp *verify.TrustPolicy
			if keyFile != "" {
				keyPEM, kerr := os.ReadFile(keyFile) //nolint:gosec // operator-supplied trusted key
				if kerr != nil {
					return kerr
				}
				tp = verify.NewTrustPolicy()
				if aerr := tp.AddKeyPEM(keyPEM); aerr != nil {
					return aerr
				}
			}

			names := make([]string, 0, len(res.Selected))
			for n := range res.Selected {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, name := range names {
				sel := res.Selected[name]
				zip, ferr := c.FetchZip(ctx, name, sel.Version)
				if ferr != nil {
					return ferr
				}
				if _, perr := store.PutExpected(bytes.NewReader(zip), sel.Hash); perr != nil {
					return fmt.Errorf("module: %s@%s hash gate: %w", name, sel.Version, perr)
				}
				if tp != nil {
					sig, ok, serr := c.FetchSignature(ctx, name, sel.Version)
					if serr != nil {
						return serr
					}
					if !ok {
						return fmt.Errorf("module: %s@%s is unsigned but --key was given", name, sel.Version)
					}
					if verr := verify.NewVerifier(tp).Verify(zip, sig); verr != nil {
						return fmt.Errorf("module: %s@%s signature: %w", name, sel.Version, verr)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "installed %s %s\n", name, sel.Version)
			}
			return writeLock(res, filepath.Join(lockDir, "module.lock"))
		},
	}
	cmd.Flags().StringVar(&reg, "registry", "http://localhost:8181", "registry base URL")
	cmd.Flags().StringVar(&keyFile, "key", "", "trusted public key PEM (enables signature verification)")
	cmd.Flags().StringVar(&dir, "dir", "", "directory for module.lock when installing a vendor/pkg@ver spec")
	return cmd
}

func updateCmd(d Deps) *cobra.Command {
	var reg string
	cmd := &cobra.Command{
		Use:   "update [dir]",
		Short: "Re-resolve from scratch and rewrite module.lock",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			m, _, err := readManifest(dir)
			if err != nil {
				return err
			}
			res, err := resolveRoot(cmd.Context(), d.client(reg), m)
			if err != nil {
				return err
			}
			if err := writeLock(res, filepath.Join(dir, "module.lock")); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated module.lock (%d modules)\n", len(res.Selected))
			return nil
		},
	}
	cmd.Flags().StringVar(&reg, "registry", "http://localhost:8181", "registry base URL")
	return cmd
}

func treeCmd(d Deps) *cobra.Command {
	var reg string
	cmd := &cobra.Command{
		Use:   "tree [dir]",
		Short: "Print the resolved dependency tree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c := d.client(reg)
			dir := dirArg(args)
			m, _, err := readManifest(dir)
			if err != nil {
				return err
			}
			res, err := resolveRoot(ctx, c, m)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", m.Name)
			seen := map[string]bool{}
			var walk func(deps map[string]string, depth int)
			walk = func(deps map[string]string, depth int) {
				dn := make([]string, 0, len(deps))
				for n := range deps {
					dn = append(dn, n)
				}
				sort.Strings(dn)
				for _, n := range dn {
					sel, ok := res.Selected[n]
					if !ok {
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s%s %s\n",
						strings.Repeat("  ", depth+1), n, sel.Version)
					if seen[n] {
						continue
					}
					seen[n] = true
					cm, merr := c.GetManifest(ctx, n, sel.Version)
					if merr == nil && len(cm.Dependencies) > 0 {
						walk(cm.Dependencies, depth+1)
					}
				}
			}
			walk(m.Dependencies, 0)
			return nil
		},
	}
	cmd.Flags().StringVar(&reg, "registry", "http://localhost:8181", "registry base URL")
	return cmd
}

func testCmd(d Deps) *cobra.Command {
	var level, output string
	cmd := &cobra.Command{
		Use:   "test [dir]",
		Short: "Run a module's Starlark unit tests",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			passed, failed, err := d.runner().RunTests(cmd.Context(), dir,
				AuditOptions{Level: level, Output: output})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "tests: %d passed, %d failed\n", passed, failed)
			if failed > 0 {
				return fmt.Errorf("module: %d test(s) failed", failed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&level, "audit-level", "", "capability audit level for test runs")
	cmd.Flags().StringVar(&output, "audit-output", "", "capability audit output target")
	return cmd
}

func cleanCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clear the local module cache",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := dir
			if root == "" {
				root = cas.DefaultRoot()
			}
			if err := os.RemoveAll(root); err != nil {
				return err
			}
			if err := os.MkdirAll(root, 0o750); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cleaned %s\n", root)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "cache directory (default: the module cache root)")
	return cmd
}

func dirArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return "."
}
