package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// newRootCmd creates the root command for kscore-module
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-module",
		Short: "Module management for Keystone Core",
		Long: `Manage Keystone Core modules with dependency resolution, verification, and distribution.

Keystone Core modules are versioned, capability-scoped packages that extend
the system with custom state management, reactors, policies, and more.

Module Lifecycle:
  init      - Create a new module from template
  validate  - Check module.yaml syntax and structure
  build     - Package module as distributable ZIP
  sign      - Sign module with a private key
  resolve   - Resolve dependencies and generate lock file
  verify    - Verify cryptographic signatures and hashes
  publish   - Publish module to a registry
  install   - Install modules from a registry
  test      - Run module tests

Examples:
  # Create a new module
  kscorectl module init myorg/my-module --type starlark

  # Validate module configuration
  kscorectl module validate .

  # Build module for distribution
  kscorectl module build --output my-module-1.0.0.zip

  # Resolve dependencies
  kscorectl module resolve

  # Verify module integrity
  kscorectl module verify my-module-1.0.0.zip`,
	}

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(resolveCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(installCmd)

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-module", &auditLevel, &auditOutput)
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
