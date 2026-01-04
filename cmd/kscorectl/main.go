package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/shawnbutts/keystone-core/pkg/plugin"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// newRootCmd creates the root command for kscorectl
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscorectl",
		Short: "Keystone Core command-line interface",
		Long: `kscorectl is the main CLI for interacting with Keystone Core.
It uses a Git-style plugin architecture where subcommands are
implemented as separate binaries (kscore-*).`,
	}

	rootCmd.AddCommand(newVersionCmd())

	// Discover and register plugins
	discovery := plugin.NewDiscovery()
	if err := discovery.Discover(); err != nil {
		// Don't fail if plugin discovery fails, just log warning
		fmt.Fprintf(os.Stderr, "Warning: plugin discovery failed: %v\n", err)
		return rootCmd
	}

	// Register each discovered plugin as a subcommand
	for _, p := range discovery.List() {
		rootCmd.AddCommand(newPluginCommand(p))
	}

	return rootCmd
}

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
		},
	}
}

// newPluginCommand creates a Cobra command that delegates to the plugin
func newPluginCommand(p *plugin.Plugin) *cobra.Command {
	return &cobra.Command{
		Use:   p.Name,
		Short: fmt.Sprintf("Run %s plugin", p.Name),
		Long:  fmt.Sprintf("Execute the %s plugin (%s)", p.Name, p.Path),
		// DisableFlagParsing allows plugin to handle its own flags
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			executor := plugin.NewExecutor(p)
			return executor.Execute(plugin.ExecuteOptions{
				Args: args,
				Ctx:  context.Background(),
			})
		},
	}
}
