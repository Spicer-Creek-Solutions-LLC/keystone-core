package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/titananvil/titan-anvil/pkg/plugin"
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

	// Discover and register plugins
	discovery := plugin.NewDiscovery()
	if err := discovery.Discover(); err != nil {
		// Don't fail if plugin discovery fails, just log warning
		fmt.Fprintf(os.Stderr, "Warning: plugin discovery failed: %v\n", err)
		return
	}

	// Register each discovered plugin as a subcommand
	for _, p := range discovery.List() {
		registerPluginCommand(p)
	}
}

// registerPluginCommand creates a Cobra command that delegates to the plugin
func registerPluginCommand(p *plugin.Plugin) {
	cmd := &cobra.Command{
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

	rootCmd.AddCommand(cmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
