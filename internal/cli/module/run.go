// SPDX-License-Identifier: Apache-2.0

package module

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	mod "go.keystone-core.io/keystone-core/internal/module"
	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

// runCmd loads, verifies, and executes a module, printing its output.
func runCmd(_ Deps) *cobra.Command {
	var (
		skip     bool
		sigFile  string
		keyFiles []string
	)
	cmd := &cobra.Command{
		Use:   "run <module-dir|module.zip> [input-json]",
		Short: "Load, verify, and execute a module",
		Long: "Load a module (a directory is packaged on the fly, or pass a built .zip), " +
			"verify its signature against trusted keys, and run main(input), printing the " +
			"returned object as JSON. Module capability calls (fs_read, http_get, …) are " +
			"gated by the manifest's declared, scope-enforced capabilities.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			zipPath, cleanup, err := bundleZip(args[0])
			if err != nil {
				return err
			}
			defer cleanup()

			var (
				opts     loader.LoadOptions
				verifier *verify.Verifier
			)
			if skip {
				opts.SkipVerification = true
			} else {
				if sigFile == "" || len(keyFiles) == 0 {
					return errors.New("module run requires a signature: pass --sig <file> and --key <pem> to verify, or --skip-verification for local development")
				}
				tp := verify.NewTrustPolicy()
				for _, kf := range keyFiles {
					pem, rerr := os.ReadFile(kf) //nolint:gosec // operator-supplied key path
					if rerr != nil {
						return fmt.Errorf("read key %s: %w", kf, rerr)
					}
					if aerr := tp.AddKeyPEM(pem); aerr != nil {
						return fmt.Errorf("trust key %s: %w", kf, aerr)
					}
				}
				verifier = verify.NewVerifier(tp)
				sb, rerr := os.ReadFile(sigFile) //nolint:gosec // operator-supplied sig path
				if rerr != nil {
					return fmt.Errorf("read signature: %w", rerr)
				}
				sig, perr := verify.UnmarshalSignature(sb)
				if perr != nil {
					return fmt.Errorf("parse signature: %w", perr)
				}
				opts.Signature = &sig
			}

			input := map[string]any{}
			if len(args) == 2 && args[1] != "" {
				if jerr := json.Unmarshal([]byte(args[1]), &input); jerr != nil {
					return fmt.Errorf("input JSON: %w", jerr)
				}
			}

			// Module log-capability output goes to stderr so stdout
			// stays clean for the JSON result.
			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
			l := mod.BuildLoader(mod.LoaderOptions{Verifier: verifier, Logger: logger})

			res, err := l.LoadAndExecute(cmd.Context(), zipPath, opts, input)
			if err != nil {
				return fmt.Errorf("run module: %w", err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(res.Output)
		},
	}
	cmd.Flags().BoolVar(&skip, "skip-verification", false, "skip signature verification (local development only)")
	cmd.Flags().StringVar(&sigFile, "sig", "", "detached signature file (.sig)")
	cmd.Flags().StringArrayVar(&keyFiles, "key", nil, "trusted signer public key (PEM); repeatable")
	return cmd
}

// bundleZip resolves a module path to a ZIP bundle the loader reads. A
// directory is validated and packaged into a temp zip (removed by the
// returned cleanup); a file is used as-is with a no-op cleanup.
func bundleZip(path string) (zipPath string, cleanup func(), err error) {
	noop := func() {}
	fi, err := os.Stat(path)
	if err != nil {
		return "", noop, err
	}
	if !fi.IsDir() {
		return path, noop, nil
	}
	m, _, err := readManifest(path)
	if err != nil {
		return "", noop, err
	}
	if verr := m.Validate(); verr != nil {
		return "", noop, verr
	}
	tmp, err := os.CreateTemp("", "kscore-module-*.zip")
	if err != nil {
		return "", noop, err
	}
	_ = tmp.Close()
	if zerr := zipDir(path, tmp.Name()); zerr != nil {
		_ = os.Remove(tmp.Name())
		return "", noop, zerr
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
