package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/titananvil/titan-anvil/pkg/version"
)

var rootCmd = &cobra.Command{
	Use:   "titanctl",
	Short: "TitanAnvil command-line interface",
	Long: `titanctl is the main CLI for interacting with TitanAnvil.
It uses a Git-style plugin architecture where subcommands are
implemented as separate binaries (titananvil-*).`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		info := version.Get()
		fmt.Println(info.String())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// TODO: Add plugin discovery and execution
	// Plugin binaries should be named: titananvil-module, titananvil-state, etc.
	// When user runs: titanctl module install
	// We should execute: titananvil-module install
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
