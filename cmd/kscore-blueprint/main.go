package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/deprecation"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

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
  bundle    - Bundle blueprints into distributable archives
  mirror    - Mirror blueprints between registries
  applied   - Show applied blueprint information
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

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands - core lifecycle (staying in this command)
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(bundleCmd)
	rootCmd.AddCommand(mirrorCmd)
	rootCmd.AddCommand(appliedCmd)

	// Add subcommands - publication workflow (deprecated, moving to kscore-blueprint-publish)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(docsCmd)

	// Add subcommands - state management (deprecated, moving to kscore-blueprint-state)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(snapshotCmd)

	// Apply deprecation warnings to commands moving to kscore-blueprint-publish
	publishDeprecations := deprecation.BlueprintPublishDeprecations()
	deprecation.DeprecateCommand(publishCmd, publishDeprecations["publish"])
	deprecation.DeprecateCommand(signCmd, publishDeprecations["sign"])
	deprecation.DeprecateCommand(verifyCmd, publishDeprecations["verify"])
	deprecation.DeprecateCommand(versionsCmd, publishDeprecations["versions"])
	deprecation.DeprecateCommand(docsCmd, publishDeprecations["docs"])

	// Apply deprecation warnings to commands moving to kscore-blueprint-state
	stateDeprecations := deprecation.BlueprintStateDeprecations()
	deprecation.DeprecateCommand(rollbackCmd, stateDeprecations["rollback"])
	deprecation.DeprecateCommand(snapshotCmd, stateDeprecations["snapshot"])

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-blueprint", &auditLevel, &auditOutput)
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
