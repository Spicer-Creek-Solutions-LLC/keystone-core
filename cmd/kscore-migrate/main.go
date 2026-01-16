package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/state"
	"github.com/shawnbutts/keystone-core/pkg/version"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose     bool
	auditLevel  string
	auditOutput string

	// Migration flags
	sqlitePath    string
	postgresDSN   string
	dryRun        bool
	batchSize     int
	continueOnErr bool
	skipExisting  bool
)

// newRootCmd creates the root command for kscore-migrate
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-migrate",
		Short: "Keystone Core database migration tool",
		Long: `kscore-migrate is a CLI tool for migrating data between Keystone Core storage backends.

This tool provides commands for:
  - Migrating data from SQLite to PostgreSQL
  - Validating migration completeness
  - Dry-run mode for testing

Usage via kscorectl:
  kscorectl migrate run --sqlite /path/to/db.sqlite --postgres "postgres://..."
  kscorectl migrate validate --sqlite /path/to/db.sqlite --postgres "postgres://..."`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands
	rootCmd.AddCommand(
		newRunCommand(),
		newValidateCommand(),
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
	auditHandler := auditutil.Attach(rootCmd, "kscore-migrate", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run migration from SQLite to PostgreSQL",
		Long: `Migrate all data from a SQLite database to PostgreSQL.

This command migrates:
  - Agents
  - Commands
  - Batch jobs
  - Batch agent results

Data is migrated in order to respect foreign key constraints.`,
		RunE: runMigration,
	}

	cmd.Flags().StringVar(&sqlitePath, "sqlite", "", "Path to SQLite database file (required)")
	cmd.Flags().StringVar(&postgresDSN, "postgres", "", "PostgreSQL connection string (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Perform a dry run without writing to target")
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Number of records to process per batch")
	cmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "Continue migration even if some records fail")
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", true, "Skip records that already exist in target")

	cmd.MarkFlagRequired("sqlite")
	cmd.MarkFlagRequired("postgres")

	return cmd
}

func newValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate migration completeness",
		Long: `Compare source and target databases to verify migration completeness.

This command compares record counts for:
  - Agents
  - Commands
  - Batch jobs
  - Batch agent results

Any discrepancies are reported.`,
		RunE: validateMigration,
	}

	cmd.Flags().StringVar(&sqlitePath, "sqlite", "", "Path to SQLite database file (required)")
	cmd.Flags().StringVar(&postgresDSN, "postgres", "", "PostgreSQL connection string (required)")

	cmd.MarkFlagRequired("sqlite")
	cmd.MarkFlagRequired("postgres")

	return cmd
}

func runMigration(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate inputs
	if sqlitePath == "" {
		return fmt.Errorf("--sqlite flag is required")
	}
	if postgresDSN == "" {
		return fmt.Errorf("--postgres flag is required")
	}

	// Check SQLite file exists
	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		return fmt.Errorf("SQLite database not found: %s", sqlitePath)
	}

	fmt.Printf("Starting migration from SQLite to PostgreSQL...\n")
	if dryRun {
		fmt.Printf("  Mode: DRY RUN (no data will be written)\n")
	}
	fmt.Printf("  Source: %s\n", sqlitePath)
	fmt.Printf("  Target: PostgreSQL\n")
	fmt.Printf("  Batch size: %d\n", batchSize)
	fmt.Printf("  Skip existing: %v\n", skipExisting)
	fmt.Printf("  Continue on error: %v\n", continueOnErr)
	fmt.Println()

	// Create source store
	sourceConfig := &state.Config{
		Backend:    "sqlite",
		SQLitePath: sqlitePath,
		SQLiteWAL:  true,
	}
	source, err := state.NewSQLiteStore(sourceConfig)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer source.Close()

	// Create target store
	targetConfig := &state.Config{
		Backend:       "postgresql",
		PostgreSQLDSN: postgresDSN,
	}
	target, err := state.NewPostgreSQLStore(targetConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer target.Close()

	// Create migration options
	opts := &state.MigrationOptions{
		DryRun:          dryRun,
		BatchSize:       batchSize,
		ContinueOnError: continueOnErr,
		SkipExisting:    skipExisting,
		ProgressCallback: func(table string, current, total int) {
			if verbose || current == 0 || current == total {
				fmt.Printf("  %s: %d/%d\n", table, current, total)
			}
		},
	}

	// Run migration
	migrator := state.NewMigrator(source, target, opts)
	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print results
	fmt.Println()
	fmt.Println("Migration completed!")
	fmt.Printf("  Duration: %v\n", stats.Duration.Round(time.Millisecond))
	fmt.Printf("  Agents migrated: %d\n", stats.AgentsMigrated)
	fmt.Printf("  Commands migrated: %d\n", stats.CommandsMigrated)
	fmt.Printf("  Batch jobs migrated: %d\n", stats.BatchJobsMigrated)
	fmt.Printf("  Batch agent results migrated: %d\n", stats.BatchAgentResultsMigrated)

	if len(stats.Errors) > 0 {
		fmt.Printf("\n  Errors: %d\n", len(stats.Errors))
		for _, e := range stats.Errors {
			fmt.Printf("    - [%s] %s: %s\n", e.Table, e.ID, e.Message)
		}
	}

	return nil
}

func validateMigration(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate inputs
	if sqlitePath == "" {
		return fmt.Errorf("--sqlite flag is required")
	}
	if postgresDSN == "" {
		return fmt.Errorf("--postgres flag is required")
	}

	// Check SQLite file exists
	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		return fmt.Errorf("SQLite database not found: %s", sqlitePath)
	}

	fmt.Printf("Validating migration...\n")
	fmt.Printf("  Source: %s\n", sqlitePath)
	fmt.Printf("  Target: PostgreSQL\n")
	fmt.Println()

	// Create source store
	sourceConfig := &state.Config{
		Backend:    "sqlite",
		SQLitePath: sqlitePath,
		SQLiteWAL:  true,
	}
	source, err := state.NewSQLiteStore(sourceConfig)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer source.Close()

	// Create target store
	targetConfig := &state.Config{
		Backend:       "postgresql",
		PostgreSQLDSN: postgresDSN,
	}
	target, err := state.NewPostgreSQLStore(targetConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer target.Close()

	// Run validation
	migrator := state.NewMigrator(source, target, nil)
	result, err := migrator.ValidateMigration(ctx)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Print results
	fmt.Println("Record counts:")
	fmt.Printf("  Agents:             Source=%d  Target=%d\n", result.SourceAgentCount, result.TargetAgentCount)
	fmt.Printf("  Commands:           Source=%d  Target=%d\n", result.SourceCommandCount, result.TargetCommandCount)
	fmt.Printf("  Batch jobs:         Source=%d  Target=%d\n", result.SourceBatchJobCount, result.TargetBatchJobCount)
	fmt.Printf("  Batch agent results: Source=%d  Target=%d\n", result.SourceBatchResultCount, result.TargetBatchResultCount)
	fmt.Println()

	if result.Valid {
		fmt.Println("Validation PASSED - all record counts match")
		return nil
	}

	fmt.Println("Validation FAILED - discrepancies found:")
	for _, d := range result.Discrepancies {
		fmt.Printf("  - %s\n", d)
	}
	return fmt.Errorf("validation failed")
}
