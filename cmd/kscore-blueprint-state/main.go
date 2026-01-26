package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-blueprint-state",
		Short: "Blueprint state management for Keystone Core",
		Long: `Manage blueprint state including rollback and snapshots.

This command provides tools for managing the state of applied blueprints,
including rollback to previous versions and snapshot management for
recovery purposes.

Commands:
  rollback  - Rollback a blueprint to a previous version
  snapshot  - Manage blueprint snapshots (list, delete, diff)
  diff      - Compare snapshots or versions

Examples:
  # Rollback to the previous version
  kscore-blueprint-state rollback myorg/web-stack

  # Rollback to a specific version
  kscore-blueprint-state rollback myorg/web-stack --to-version 1.2.0

  # List snapshots
  kscore-blueprint-state snapshot list

  # Compare two snapshots
  kscore-blueprint-state diff abc123 def456`,
	}

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(diffCmd)

	return rootCmd
}

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-blueprint-state", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var (
	auditLevel  string
	auditOutput string
)
