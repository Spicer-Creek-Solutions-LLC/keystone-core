package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	// Global flags
	serverAddr   string
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

// newRootCmd creates the root command for kscore-cluster
func newRootCmd() *cobra.Command {
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
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

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
		newVersionCmd(),
	)

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

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-cluster", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
