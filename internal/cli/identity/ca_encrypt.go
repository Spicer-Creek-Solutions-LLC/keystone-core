// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/identity"
	"go.keystone-core.io/keystone-core/internal/masterkey"
)

// caEncryptCmd is the local `ca encrypt` migration command. Unlike the
// other `ca` subcommands it does not talk to the server — it rewrites
// the on-disk CA key files in place, so it runs against a stopped (or
// not-yet-started) server's storage directory.
func caEncryptCmd(_ *globals) *cobra.Command {
	var storagePath, keySource string
	cmd := &cobra.Command{
		Use:   "encrypt",
		Short: "Encrypt the on-disk CA private keys at rest (in-place migration)",
		Long: "Migrate a plaintext CA storage directory to encryption-at-rest: " +
			"the root + signing private-key files are sealed under a master key " +
			"(AES-256-GCM); the public cert files are left untouched. Run this " +
			"with the server stopped, then set identity.encryption_key in the " +
			"server config to the same master-key source. The key source is " +
			"scheme-prefixed: env:VAR_NAME, file:/path/to/keyfile, or " +
			"inline:<hex|base64>. Refuses to run if the keys are already " +
			"encrypted.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCAEncrypt(cmd.OutOrStdout(), storagePath, keySource)
		},
	}
	cmd.Flags().StringVar(&storagePath, "storage-path", "", "CA storage directory (identity.storage_path)")
	cmd.Flags().StringVar(&keySource, "key", "", "master-key source: env:VAR | file:/path | inline:<hex|base64>")
	_ = cmd.MarkFlagRequired("storage-path")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

func runCAEncrypt(out io.Writer, storagePath, keySource string) error {
	key, err := masterkey.Resolve(keySource)
	if err != nil {
		return fmt.Errorf("resolve key: %w", err)
	}
	migrated, err := identity.EncryptCADirectory(storagePath, key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "encrypted %d CA key pair(s) under %s (key fingerprint %s): %v\n",
		len(migrated), storagePath, key.Fingerprint(), migrated)
	fmt.Fprintf(out, "set identity.encryption_key to this same source in the server config to load it.\n")
	return nil
}
