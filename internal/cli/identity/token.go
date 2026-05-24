// SPDX-License-Identifier: Apache-2.0

package identity

import "github.com/spf13/cobra"

// tokenCmd is the `token` parent command. The four leaves below
// (create / list / revoke / cleanup) attach via this constructor.
func tokenCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage join tokens (create / list / revoke / cleanup)",
		Long: "Operator surface for cluster-join tokens. `create` mints a new " +
			"cleartext token (returned ONCE); the rest operate on stored records " +
			"by ID or prefix.",
	}
	cmd.AddCommand(tokenCreateCmd(g))
	cmd.AddCommand(tokenListCmd(g))
	cmd.AddCommand(tokenRevokeCmd(g))
	cmd.AddCommand(tokenCleanupCmd(g))
	return cmd
}
