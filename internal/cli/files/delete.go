// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"

	"github.com/spf13/cobra"
)

func deleteCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <remote-path>",
		Short: "Remove a file from the kscore file service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := parseRemotePath(args[0])
			if err != nil {
				return err
			}
			c, closer, err := g.connect()
			if err != nil {
				return err
			}
			defer closer()
			if err := c.Delete(cmd.Context(), remote); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", remote)
			return nil
		},
	}
}
