// SPDX-License-Identifier: Apache-2.0

package state

import "github.com/spf13/cobra"

func varsCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "Inspect compiled-state variables",
	}
	cmd.AddCommand(varsGetCmd(g))
	return cmd
}

func varsGetCmd(g *globals) *cobra.Command {
	flags := &localFlags{}
	cmd := &cobra.Command{
		Use:   "get <file> [<key>]",
		Short: "Print the merged Variables map (or one value when a key is given)",
		Long: "Runs the client-side compile pipeline up to render and " +
			"prints sf.Variables. Provide a key to print just that value " +
			"(suitable for piping into other tools).",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, vars, err := compileLocal(args[:1], flags)
			if err != nil {
				return err
			}
			key := ""
			if len(args) > 1 {
				key = args[1]
			}
			return printVars(cmd.OutOrStdout(), g.Output, vars, key)
		},
	}
	registerLocalFlags(cmd, flags)
	return cmd
}
