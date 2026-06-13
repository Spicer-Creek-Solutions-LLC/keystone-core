// SPDX-License-Identifier: Apache-2.0

package identity

import "github.com/spf13/cobra"

// caCmd is the `ca` parent command.
func caCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ca",
		Short: "Inspect and manage the embedded CA (info / rotate-signing / export / encrypt)",
		Long: "Operator surface for the embedded two-tier CA. `info` shows the " +
			"root + signing details; `rotate-signing` forces an immediate signing-" +
			"CA rotation; `export` dumps PEM (or JWKS for the full trust bundle); " +
			"`encrypt` migrates the on-disk CA keys to encryption-at-rest.",
	}
	cmd.AddCommand(caInfoCmd(g))
	cmd.AddCommand(caRotateCmd(g))
	cmd.AddCommand(caExportCmd(g))
	cmd.AddCommand(caEncryptCmd(g))
	return cmd
}
