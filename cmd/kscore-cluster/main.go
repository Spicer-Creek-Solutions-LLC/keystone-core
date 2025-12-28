package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version information (set at build time)
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"

	// Global flags
	serverAddr string
	outputFormat string
	verbose    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kscore-cluster",
		Short: "Keystone Core cluster management plugin",
		Long: `kscore-cluster is a CLI plugin for managing Keystone Core clusters.

This plugin provides commands for:
  - Viewing cluster status and health
  - Managing cluster members
  - Monitoring leader election
  - Performing cluster operations

Usage via kscorectl:
  kscorectl cluster status
  kscorectl cluster members
  kscorectl cluster leader`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Add subcommands
	rootCmd.AddCommand(
		newStatusCommand(),
		newMembersCommand(),
		newLeaderCommand(),
		newAddCommand(),
		newRemoveCommand(),
		newTransferLeaderCommand(),
		newRebalanceCommand(),
		newBackupCommand(),
		newRestoreCommand(),
		newVersionCommand(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
