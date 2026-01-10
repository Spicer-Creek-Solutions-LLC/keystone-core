// Package main provides the kscore-bootstrap CLI for bootstrapping Keystone Core clusters
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/bootstrap"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// CLI flags
var (
	configPath       string
	outputDir        string
	backupPath       string
	decryptionKey    string
	verbose          bool
	dryRun           bool
	force            bool
	skipVerification bool
	outputFormat     string
	timeout          time.Duration
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kscore-bootstrap",
		Short: "Bootstrap a Keystone Core cluster",
		Long: `kscore-bootstrap initializes and bootstraps a new Keystone Core cluster
from a seed configuration, restores from backup, or imports an existing installation.

This tool handles:
  - Loading and validating seed configuration
  - Installing Keystone Core components
  - Generating certificates and credentials
  - Forming single or multi-node clusters
  - Handing off to self-management`,
	}

	// Add subcommands
	rootCmd.AddCommand(seedCmd())
	rootCmd.AddCommand(restoreCmd())
	rootCmd.AddCommand(importCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cleanupCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func seedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed [config-file]",
		Short: "Bootstrap a new cluster from seed configuration",
		Long: `Bootstrap a new Keystone Core cluster from a seed configuration file.

If no config file is specified, uses default single-node configuration.

Example:
  kscore-bootstrap seed
  kscore-bootstrap seed seed-config.yaml
  kscore-bootstrap seed --dry-run seed-config.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: runSeed,
	}

	addCommonFlags(cmd)
	return cmd
}

func restoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: "Restore a cluster from backup",
		Long: `Restore a Keystone Core cluster from a backup artifact.

The backup file should be a .tar.gz archive created by the backup system.

Example:
  kscore-bootstrap restore backup-2024-01-15.tar.gz
  kscore-bootstrap restore --decryption-key @key.txt encrypted-backup.tar.gz.enc`,
		Args: cobra.ExactArgs(1),
		RunE: runRestore,
	}

	addCommonFlags(cmd)
	cmd.Flags().StringVar(&decryptionKey, "decryption-key", "", "Decryption key for encrypted backups (use @file to read from file)")
	return cmd
}

func importCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import an existing Keystone Core installation",
		Long: `Import an existing Keystone Core installation into self-management.

This discovers existing components and brings them under management.

Example:
  kscore-bootstrap import
  kscore-bootstrap import --config /etc/kscore/server.yaml`,
		RunE: runImport,
	}

	addCommonFlags(cmd)
	return cmd
}

func validateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <config-file>",
		Short: "Validate a seed configuration file",
		Long: `Validate a seed configuration file without performing any actions.

Example:
  kscore-bootstrap validate seed-config.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: runValidate,
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	return cmd
}

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show bootstrap status",
		Long: `Show the status of a previous or ongoing bootstrap operation.

Example:
  kscore-bootstrap status
  kscore-bootstrap status --output json`,
		RunE: runStatus,
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	return cmd
}

func cleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up a failed bootstrap",
		Long: `Clean up artifacts from a failed bootstrap operation.

This removes partial installations and restores the system to a clean state.

Example:
  kscore-bootstrap cleanup
  kscore-bootstrap cleanup --force`,
		RunE: runCleanup,
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force cleanup, removing all Keystone Core data")
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kscore-bootstrap %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
		},
	}
}

func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to seed configuration file")
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "/var/lib/kscore", "Output directory for generated files")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without making changes")
	cmd.Flags().BoolVar(&skipVerification, "skip-verification", false, "Skip post-bootstrap verification")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Bootstrap timeout")
}

func runSeed(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		configPath = args[0]
	}

	ctx, cancel := contextWithSignal()
	defer cancel()

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	opts := bootstrap.BootstrapOptions{
		Mode:             bootstrap.BootstrapModeSeed,
		SeedConfigPath:   configPath,
		OutputDir:        outputDir,
		DryRun:           dryRun,
		Verbose:          verbose,
		SkipVerification: skipVerification,
		Force:            force,
	}

	logger := &cliLogger{verbose: verbose}
	bootstrapper, err := bootstrap.NewBootstrapper(opts, logger)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	bootstrapper.SetProgressCallback(func(status *bootstrap.BootstrapStatus) {
		if verbose || status.Phase == bootstrap.PhaseFailed {
			fmt.Printf("[%s] %s (%d%%)\n", status.Phase, status.Message, status.Progress)
		}
	})

	fmt.Println("Starting Keystone Core bootstrap...")
	if dryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}

	result, err := bootstrapper.Bootstrap(ctx, opts)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	printResult(result)
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	backupPath = args[0]

	ctx, cancel := contextWithSignal()
	defer cancel()

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		defer timeoutCancel()
	}

	// Handle decryption key from file
	key := decryptionKey
	if len(key) > 0 && key[0] == '@' {
		data, err := os.ReadFile(key[1:])
		if err != nil {
			return fmt.Errorf("failed to read decryption key: %w", err)
		}
		key = string(data)
	}

	opts := bootstrap.BootstrapOptions{
		Mode:             bootstrap.BootstrapModeRestore,
		BackupPath:       backupPath,
		DecryptionKey:    key,
		OutputDir:        outputDir,
		DryRun:           dryRun,
		Verbose:          verbose,
		SkipVerification: skipVerification,
		Force:            force,
	}

	logger := &cliLogger{verbose: verbose}
	bootstrapper, err := bootstrap.NewBootstrapper(opts, logger)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	fmt.Printf("Restoring Keystone Core from backup: %s\n", backupPath)

	result, err := bootstrapper.Bootstrap(ctx, opts)
	if err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	printResult(result)
	return nil
}

func runImport(cmd *cobra.Command, args []string) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	opts := bootstrap.BootstrapOptions{
		Mode:             bootstrap.BootstrapModeImport,
		SeedConfigPath:   configPath,
		OutputDir:        outputDir,
		DryRun:           dryRun,
		Verbose:          verbose,
		SkipVerification: skipVerification,
		Force:            force,
	}

	logger := &cliLogger{verbose: verbose}
	bootstrapper, err := bootstrap.NewBootstrapper(opts, logger)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	fmt.Println("Importing existing Keystone Core installation...")

	result, err := bootstrapper.Bootstrap(ctx, opts)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	printResult(result)
	return nil
}

func runValidate(cmd *cobra.Command, args []string) error {
	configPath := args[0]

	loader := bootstrap.NewConfigLoader()
	config, err := loader.LoadSeedConfig(configPath)
	if err != nil {
		if outputFormat == "json" {
			out, _ := json.MarshalIndent(map[string]any{
				"valid": false,
				"error": err.Error(),
			}, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		return err
	}

	validationErr := bootstrap.ValidateSeedConfig(config)

	if outputFormat == "json" {
		result := map[string]any{
			"valid":  validationErr == nil,
			"config": config,
		}
		if validationErr != nil {
			result["errors"] = validationErr.Error()
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		if validationErr != nil {
			fmt.Printf("Configuration is invalid:\n  %v\n", validationErr)
			return validationErr
		}
		fmt.Println("Configuration is valid.")
		fmt.Printf("  Cluster name: %s\n", config.Cluster.Name)
		fmt.Printf("  Control plane replicas: %d\n", config.ControlPlane.Replicas)
		fmt.Printf("  NATS mode: %s\n", config.NATS.Mode)
		fmt.Printf("  Database type: %s\n", config.Database.Type)
	}

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	stateDir := "/var/lib/kscore/bootstrap"

	state, err := bootstrap.LoadHandoffState(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No bootstrap state found.")
			return nil
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	if outputFormat == "json" {
		out, _ := json.MarshalIndent(state, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("Bootstrap Status\n")
		fmt.Printf("  Phase: %s\n", state.Phase)
		fmt.Printf("  Started: %s\n", state.StartTime.Format(time.RFC3339))
		fmt.Printf("  Completed steps: %v\n", state.CompletedSteps)
		fmt.Printf("  Pending steps: %v\n", state.PendingSteps)
		fmt.Printf("  Health verified: %v\n", state.HealthVerified)
		fmt.Printf("  States applied: %v\n", state.StatesApplied)
		fmt.Printf("  Agents connected: %d\n", state.AgentsConnected)
		if state.Error != "" {
			fmt.Printf("  Error: %s\n", state.Error)
		}
	}

	return nil
}

func runCleanup(cmd *cobra.Command, args []string) error {
	ctx, cancel := contextWithSignal()
	defer cancel()

	opts := bootstrap.BootstrapOptions{
		Force: force,
	}

	logger := &cliLogger{verbose: true}
	bootstrapper, err := bootstrap.NewBootstrapper(opts, logger)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	fmt.Println("Cleaning up bootstrap artifacts...")
	if force {
		fmt.Println("WARNING: Force mode enabled, all Keystone Core data will be removed!")
	}

	if err := bootstrapper.Cleanup(ctx); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Println("Cleanup complete.")
	return nil
}

func printResult(result *bootstrap.BootstrapResult) {
	fmt.Println()
	if result.Success {
		fmt.Println("Bootstrap completed successfully!")
		fmt.Println()
		fmt.Printf("  Cluster ID:     %s\n", result.ClusterID)
		fmt.Printf("  API Endpoint:   %s\n", result.APIEndpoint)
		fmt.Printf("  CA Fingerprint: %s\n", result.CAFingerprint)
		fmt.Printf("  Duration:       %s\n", result.Duration.Round(time.Second))
		fmt.Println()
		if result.AdminToken != "" {
			fmt.Println("  Admin Token (save this, it won't be shown again):")
			fmt.Printf("    %s\n", result.AdminToken)
			fmt.Println()
		}
		fmt.Println("Next steps:")
		fmt.Println("  1. Configure kscorectl: kscorectl config set-context default --server=" + result.APIEndpoint)
		fmt.Println("  2. Deploy agents to managed nodes")
		fmt.Println("  3. Apply your state configurations")
	} else {
		fmt.Printf("Bootstrap failed: %s\n", result.Error)
	}
}

func contextWithSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, cleaning up...")
		cancel()
	}()

	return ctx, cancel
}

// cliLogger implements the bootstrap.Logger interface for CLI output
type cliLogger struct {
	verbose bool
}

func (l *cliLogger) Debug(msg string, args ...any) {
	if l.verbose {
		fmt.Printf("[DEBUG] %s", msg)
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fmt.Printf(" %v=%v", args[i], args[i+1])
			}
		}
		fmt.Println()
	}
}

func (l *cliLogger) Info(msg string, args ...any) {
	fmt.Printf("[INFO] %s", msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v=%v", args[i], args[i+1])
		}
	}
	fmt.Println()
}

func (l *cliLogger) Warn(msg string, args ...any) {
	fmt.Printf("[WARN] %s", msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v=%v", args[i], args[i+1])
		}
	}
	fmt.Println()
}

func (l *cliLogger) Error(msg string, args ...any) {
	fmt.Printf("[ERROR] %s", msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v=%v", args[i], args[i+1])
		}
	}
	fmt.Println()
}
