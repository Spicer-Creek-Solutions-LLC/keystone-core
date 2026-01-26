// Package main provides the kscore-bootstrap CLI for bootstrapping Keystone Core clusters
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/bootstrap"
	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
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
	auditLevel       string
	auditOutput      string
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

	rootCmd.AddCommand(seedCmd())
	rootCmd.AddCommand(restoreCmd())
	rootCmd.AddCommand(importCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cleanupCmd())
	rootCmd.AddCommand(versionCmd())

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	auditHandler := auditutil.Attach(rootCmd, "kscore-bootstrap", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
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

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json, yaml, table)")
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

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json, yaml, table)")
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

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	loader := bootstrap.NewConfigLoader()
	config, err := loader.LoadSeedConfig(configPath)
	if err != nil {
		result := map[string]any{
			"valid": false,
			"error": err.Error(),
		}
		switch format {
		case output.FormatJSON:
			_ = output.WriteJSON(cmd.OutOrStdout(), result)
		case output.FormatYAML:
			_ = output.WriteYAML(cmd.OutOrStdout(), result)
		case output.FormatTable:
			table := buildKeyValueTable([][2]string{
				{"VALID", "false"},
				{"ERROR", err.Error()},
			})
			_ = output.WriteTable(cmd.OutOrStdout(), table)
		case output.FormatText:
			fmt.Fprintf(cmd.OutOrStdout(), "Error: %v\n", err)
		default:
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}
		return err
	}

	validationErr := bootstrap.ValidateSeedConfig(config)

	switch format {
	case output.FormatJSON:
		result := map[string]any{
			"valid": validationErr == nil,
		}
		if validationErr == nil {
			result["config"] = config
		} else {
			result["errors"] = validationErr.Error()
		}
		return output.WriteJSON(cmd.OutOrStdout(), result)
	case output.FormatYAML:
		result := map[string]any{
			"valid": validationErr == nil,
		}
		if validationErr == nil {
			result["config"] = config
		} else {
			result["errors"] = validationErr.Error()
		}
		return output.WriteYAML(cmd.OutOrStdout(), result)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"VALID", fmt.Sprintf("%t", validationErr == nil)},
			{"CLUSTER", config.Cluster.Name},
			{"CONTROL PLANE REPLICAS", fmt.Sprintf("%d", config.ControlPlane.Replicas)},
			{"NATS MODE", string(config.NATS.Mode)},
			{"DATABASE TYPE", string(config.Database.Type)},
			{"ERRORS", func() string {
				if validationErr == nil {
					return ""
				}
				return validationErr.Error()
			}()},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		if validationErr != nil {
			return validationErr
		}
	case output.FormatText:
		if validationErr != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration is invalid:\n  %v\n", validationErr)
			return validationErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid.")
		fmt.Fprintf(cmd.OutOrStdout(), "  Cluster name: %s\n", config.Cluster.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "  Control plane replicas: %d\n", config.ControlPlane.Replicas)
		fmt.Fprintf(cmd.OutOrStdout(), "  NATS mode: %s\n", config.NATS.Mode)
		fmt.Fprintf(cmd.OutOrStdout(), "  Database type: %s\n", config.Database.Type)
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	stateDir := "/var/lib/kscore/bootstrap"

	state, err := bootstrap.LoadHandoffState(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			format, formatErr := output.ParseFormat(outputFormat)
			if formatErr != nil {
				return formatErr
			}
			result := map[string]any{
				"status":  "not_found",
				"message": "No bootstrap state found.",
			}
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), result)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), result)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"STATUS", "not_found"},
					{"MESSAGE", "No bootstrap state found."},
				})
				return output.WriteTable(cmd.OutOrStdout(), table)
			case output.FormatText:
				fmt.Fprintln(cmd.OutOrStdout(), "No bootstrap state found.")
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFormat)
			}
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), state)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), state)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"PHASE", state.Phase},
			{"STARTED", state.StartTime.Format(time.RFC3339)},
			{"COMPLETED STEPS", strings.Join(state.CompletedSteps, ", ")},
			{"PENDING STEPS", strings.Join(state.PendingSteps, ", ")},
			{"HEALTH VERIFIED", fmt.Sprintf("%t", state.HealthVerified)},
			{"STATES APPLIED", fmt.Sprintf("%t", state.StatesApplied)},
			{"AGENTS CONNECTED", fmt.Sprintf("%d", state.AgentsConnected)},
			{"ERROR", state.Error},
		})
		return output.WriteTable(cmd.OutOrStdout(), table)
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Bootstrap Status")
		fmt.Fprintf(cmd.OutOrStdout(), "  Phase: %s\n", state.Phase)
		fmt.Fprintf(cmd.OutOrStdout(), "  Started: %s\n", state.StartTime.Format(time.RFC3339))
		fmt.Fprintf(cmd.OutOrStdout(), "  Completed steps: %v\n", state.CompletedSteps)
		fmt.Fprintf(cmd.OutOrStdout(), "  Pending steps: %v\n", state.PendingSteps)
		fmt.Fprintf(cmd.OutOrStdout(), "  Health verified: %v\n", state.HealthVerified)
		fmt.Fprintf(cmd.OutOrStdout(), "  States applied: %v\n", state.StatesApplied)
		fmt.Fprintf(cmd.OutOrStdout(), "  Agents connected: %d\n", state.AgentsConnected)
		if state.Error != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  Error: %s\n", state.Error)
		}
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
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

func buildKeyValueTable(pairs [][2]string) *output.Table {
	rows := make([][]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair[1] == "" {
			continue
		}
		rows = append(rows, []string{pair[0], pair[1]})
	}

	return &output.Table{
		Headers: []string{"KEY", "VALUE"},
		Rows:    rows,
	}
}
