package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/version"
)

// newRootCmd creates the root command for kscore-blueprint
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-blueprint",
		Short: "Blueprint management for Keystone Core",
		Long: `Manage Keystone Core blueprints - pre-packaged, reusable collections of states.

Blueprints are composable infrastructure patterns similar to Salt Formulas,
Ansible Roles, or Helm Charts. They combine states, templates, and files
into versioned, shareable packages.

Blueprint Lifecycle:
  init      - Create a new blueprint from template
  validate  - Check blueprint.yaml syntax and structure
  lint      - Run best practices checks
  test      - Run blueprint tests
  search    - Search for blueprints in registries
  info      - Show blueprint details
  versions  - List available versions
  install   - Install a blueprint
  update    - Update installed blueprints
  remove    - Remove an installed blueprint
  rollback  - Rollback to a previous version
  publish   - Publish blueprint to a registry
  sign      - Sign a blueprint
  verify    - Verify blueprint signature
  docs      - Generate documentation
  snapshot  - Manage state snapshots

Examples:
  # Create a new blueprint
  kscorectl blueprint init myorg/web-stack

  # Validate blueprint configuration
  kscorectl blueprint validate .

  # Search for blueprints
  kscorectl blueprint search nginx

  # Install a blueprint
  kscorectl blueprint install community/nginx@1.0.0

  # Publish a blueprint
  kscorectl blueprint publish .`,
	}

	// Add subcommands
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(snapshotCmd)

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
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
