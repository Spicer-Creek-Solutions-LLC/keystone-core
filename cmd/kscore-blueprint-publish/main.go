package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	auditLevel  string
	auditOutput string
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-blueprint-publish",
		Short: "Blueprint publication and signing for Keystone Core",
		Long: `Manage blueprint publication, signing, and verification.

This command handles the publication workflow for Keystone Core blueprints,
including signing packages for integrity verification and managing versions
in registries.

Publication Workflow:
  publish   - Publish blueprint to a registry
  sign      - Sign a blueprint package
  verify    - Verify blueprint signature
  versions  - Manage blueprint versions in registry
  docs      - Generate documentation from blueprint

Examples:
  # Publish a blueprint to the default registry
  kscorectl blueprint-publish publish .

  # Sign a blueprint package
  kscorectl blueprint-publish sign myblueprint-1.0.0.tar.gz

  # Verify a blueprint signature
  kscorectl blueprint-publish verify community/nginx@1.2.0

  # List versions of a blueprint
  kscorectl blueprint-publish versions community/nginx

  # Generate documentation
  kscorectl blueprint-publish docs .

Note: This command was split from 'kscore-blueprint' in v0.30.0 to provide
a focused tool for blueprint publishers. For blueprint installation and
management, use 'kscore-blueprint'.`,
	}

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(docsCmd)

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-blueprint-publish", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
