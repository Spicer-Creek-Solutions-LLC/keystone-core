package identity

import "github.com/spf13/cobra"

// caCmd is the `ca` parent command.
func caCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ca",
		Short: "Inspect and manage the embedded CA (info / rotate-signing / export)",
		Long: "Operator surface for the embedded two-tier CA. `info` shows the " +
			"root + signing details; `rotate-signing` forces an immediate signing-" +
			"CA rotation; `export` dumps PEM (or JWKS for the full trust bundle).",
	}
	cmd.AddCommand(caInfoCmd(g))
	cmd.AddCommand(caRotateCmd(g))
	cmd.AddCommand(caExportCmd(g))
	return cmd
}
