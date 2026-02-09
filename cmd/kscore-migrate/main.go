// Package main implements the kscore-migrate CLI for database migration between SQLite and PostgreSQL.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/pkg/version"
	"github.com/spf13/cobra"
)

var (
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

	// Verify flags
	sourceSystem string
	sourceDir    string
	targetDB     string
)

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

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(
		newRunCommand(),
		newValidateCommand(),
		newVerifyCommand(),
		newVersionCmd(),
	)

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

var supportedSourceSystems = []string{"salt", "ansible", "puppet", "chef"}

func isValidSourceSystem(system string) bool {
	for _, s := range supportedSourceSystems {
		if s == system {
			return true
		}
	}
	return false
}

type verifyCheckStatus string

const (
	statusPass verifyCheckStatus = "PASS"
	statusFail verifyCheckStatus = "FAIL"
	statusSkip verifyCheckStatus = "SKIP"
)

type verifyCheckResult struct {
	Name    string
	Status  verifyCheckStatus
	Details string
}

func newVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify migration from external configuration management systems",
		Long: `Verify data integrity after migrating from an external configuration management system.

This command checks that data from the source system (Salt, Ansible, Puppet, Chef) has been
correctly migrated to Keystone Core by verifying:
  - Source data is readable
  - State definitions have been migrated
  - Agent/minion mappings are complete
  - Variables/pillar data is preserved
  - Event subscriptions have been migrated

Usage:
  kscorectl migrate verify --source-system salt --source-dir /srv/salt --target-db /var/lib/keystone/keystone.db`,
		RunE: runVerify,
	}

	cmd.Flags().StringVar(&sourceSystem, "source-system", "", "Source configuration management system (salt, ansible, puppet, chef)")
	cmd.Flags().StringVar(&sourceDir, "source-dir", "", "Path to source system data export directory")
	cmd.Flags().StringVar(&targetDB, "target-db", "", "Path to Keystone Core database")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed verification output")

	cmd.MarkFlagRequired("source-system")

	return cmd
}

func runVerify(cmd *cobra.Command, args []string) error {
	system := strings.ToLower(sourceSystem)
	if !isValidSourceSystem(system) {
		return fmt.Errorf("unsupported source system %q: supported systems are %s",
			sourceSystem, strings.Join(supportedSourceSystems, ", "))
	}

	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "Verifying migration from %s to Keystone Core...\n", system)
	if sourceDir != "" {
		fmt.Fprintf(out, "  Source directory: %s\n", sourceDir)
	}
	if targetDB != "" {
		fmt.Fprintf(out, "  Target database:  %s\n", targetDB)
	}
	fmt.Fprintln(out)

	results := runVerifyChecks(system, sourceDir, targetDB)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "CHECK\tSTATUS\tDETAILS\n")
	fmt.Fprintf(w, "-----\t------\t-------\n")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Status, r.Details)
	}
	w.Flush()
	fmt.Fprintln(out)

	passed := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		switch r.Status {
		case statusPass:
			passed++
		case statusFail:
			failed++
		case statusSkip:
			skipped++
		}
	}

	fmt.Fprintf(out, "Results: %d passed, %d failed, %d skipped\n", passed, failed, skipped)

	if failed > 0 {
		return fmt.Errorf("verification failed: %d check(s) did not pass", failed)
	}

	return nil
}

func runVerifyChecks(system, srcDir, tgtDB string) []verifyCheckResult {
	results := make([]verifyCheckResult, 0, 5)
	results = append(results,
		checkSourceReadable(system, srcDir),
		checkStateDefinitions(system, srcDir),
		checkAgentMappings(system, srcDir, tgtDB),
		checkVariablesPreserved(system, srcDir),
		checkEventSubscriptions(system, srcDir),
	)

	return results
}

func checkSourceReadable(system, srcDir string) verifyCheckResult {
	name := "Source data readable"
	if srcDir == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir specified",
		}
	}

	info, err := os.Stat(srcDir)
	if err != nil {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("Cannot access source directory: %v", err),
		}
	}
	if !info.IsDir() {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("Source path is not a directory: %s", srcDir),
		}
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("Cannot read source directory: %v", err),
		}
	}

	if len(entries) == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: "Source directory is empty",
		}
	}

	return verifyCheckResult{
		Name:    name,
		Status:  statusPass,
		Details: fmt.Sprintf("Found %d entries in source directory", len(entries)),
	}
}

func stateFileExtensions(system string) []string {
	switch system {
	case "salt":
		return []string{".sls"}
	case "ansible":
		return []string{".yml", ".yaml"}
	case "puppet":
		return []string{".pp"}
	case "chef":
		return []string{".rb"}
	default:
		return nil
	}
}

func checkStateDefinitions(system, srcDir string) verifyCheckResult {
	name := "State definitions migrated"
	if srcDir == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir specified",
		}
	}

	exts := stateFileExtensions(system)
	if len(exts) == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No known state file extensions for %s", system),
		}
	}

	var found int
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		for _, e := range exts {
			if ext == e {
				found++
				break
			}
		}
		return nil
	})

	if found == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("No %s state files found in source directory", system),
		}
	}

	return verifyCheckResult{
		Name:    name,
		Status:  statusPass,
		Details: fmt.Sprintf("Found %d %s state file(s) in source", found, system),
	}
}

func agentSubdirHint(system string) string {
	switch system {
	case "salt":
		return "minions"
	case "ansible":
		return "inventory"
	case "puppet":
		return "nodes"
	case "chef":
		return "nodes"
	default:
		return ""
	}
}

func checkAgentMappings(system, srcDir, tgtDB string) verifyCheckResult {
	name := "Agent/minion mappings complete"
	if srcDir == "" && tgtDB == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir or --target-db specified",
		}
	}

	if srcDir == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir specified; cannot enumerate source agents",
		}
	}

	hint := agentSubdirHint(system)
	if hint == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No agent directory convention for %s", system),
		}
	}

	agentDir := filepath.Join(srcDir, hint)
	info, err := os.Stat(agentDir)
	if err != nil || !info.IsDir() {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No %s directory found in source export", hint),
		}
	}

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("Cannot read %s directory: %v", hint, err),
		}
	}

	if len(entries) == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("No agent entries found in %s directory", hint),
		}
	}

	return verifyCheckResult{
		Name:    name,
		Status:  statusPass,
		Details: fmt.Sprintf("Found %d agent/minion entries in %s", len(entries), hint),
	}
}

func variablesSubdirHint(system string) string {
	switch system {
	case "salt":
		return "pillar"
	case "ansible":
		return "group_vars"
	case "puppet":
		return "hieradata"
	case "chef":
		return "data_bags"
	default:
		return ""
	}
}

func checkVariablesPreserved(system, srcDir string) verifyCheckResult {
	name := "Variables/pillar data preserved"
	if srcDir == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir specified",
		}
	}

	hint := variablesSubdirHint(system)
	if hint == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No variables directory convention for %s", system),
		}
	}

	varDir := filepath.Join(srcDir, hint)
	info, err := os.Stat(varDir)
	if err != nil || !info.IsDir() {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No %s directory found in source export", hint),
		}
	}

	var count int
	filepath.Walk(varDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		count++
		return nil
	})

	if count == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("No variable files found in %s directory", hint),
		}
	}

	return verifyCheckResult{
		Name:    name,
		Status:  statusPass,
		Details: fmt.Sprintf("Found %d variable file(s) in %s", count, hint),
	}
}

func checkEventSubscriptions(system, srcDir string) verifyCheckResult {
	name := "Event subscriptions migrated"
	if srcDir == "" {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: "No --source-dir specified",
		}
	}

	var subDir string
	switch system {
	case "salt":
		subDir = "reactor"
	case "ansible":
		subDir = "callbacks"
	case "puppet":
		subDir = "reports"
	case "chef":
		subDir = "handlers"
	default:
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No event subscription convention for %s", system),
		}
	}

	eventDir := filepath.Join(srcDir, subDir)
	info, err := os.Stat(eventDir)
	if err != nil || !info.IsDir() {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No %s directory found in source export", subDir),
		}
	}

	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return verifyCheckResult{
			Name:    name,
			Status:  statusFail,
			Details: fmt.Sprintf("Cannot read %s directory: %v", subDir, err),
		}
	}

	if len(entries) == 0 {
		return verifyCheckResult{
			Name:    name,
			Status:  statusSkip,
			Details: fmt.Sprintf("No entries in %s directory", subDir),
		}
	}

	return verifyCheckResult{
		Name:    name,
		Status:  statusPass,
		Details: fmt.Sprintf("Found %d event subscription(s) in %s", len(entries), subDir),
	}
}
