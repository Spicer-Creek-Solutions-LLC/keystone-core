package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/audit"
	"github.com/shawnbutts/keystone-core/pkg/statemgmt"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	// Global flags
	auditLevel  string
	auditOutput string

	rootCmd = &cobra.Command{
		Use:   "state",
		Short: "Declarative state management for infrastructure",
		Long: `Manage infrastructure state using declarative YAML definitions.

Keystone Core State provides idempotent configuration management with:
  - Dependency resolution (automatic ordering)
  - Drift detection (what changed?)
  - Dry-run mode (check before apply)
  - Template support (vars and facts)

Examples:
  # Apply state declarations
  kscorectl state apply states/webserver.yaml

  # Check without applying (dry-run)
  kscorectl state check states/webserver.yaml

  # Detect drift
  kscorectl state drift states/webserver.yaml`,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Println(info.String())
		},
	}
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(driftCmd)
}

func main() {
	// Initialize audit logging (Epic 15)
	auditConfig := &audit.AuditConfig{
		Level:   audit.AuditLevel(auditLevel),
		Backend: auditOutput,
	}
	if err := audit.Init("kscore-state", auditConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize audit logging: %v\n", err)
	}
	defer audit.Close()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Apply command

var (
	applyVarsFile string
	applyDryRun   bool
)

var applyCmd = &cobra.Command{
	Use:   "apply <statefile>",
	Short: "Apply state declarations",
	Long: `Apply state declarations from a YAML file.

The apply command executes state declarations in dependency order,
ensuring that required states run before dependent states.

Examples:
  # Apply a state file
  kscorectl state apply states/webserver.yaml

  # Apply with variables
  kscorectl state apply states/app.yaml --vars vars/production.yaml

  # Dry-run (check what would change)
  kscorectl state apply states/app.yaml --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: applyExecute,
}

func init() {
	applyCmd.Flags().StringVar(&applyVarsFile, "vars", "", "Variables file (YAML)")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Check what would change without applying")
}

func applyExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()

	// Create audit entry (Epic 15)
	action := audit.ActionStateApplied
	auditEntry := audit.StartEntry(action, "apply")
	auditEntry.Args = args
	if applyDryRun {
		auditEntry.Extra = map[string]interface{}{"dry_run": true}
	}

	// Helper to log audit on exit
	logAudit := func(result audit.AuditResult, exitCode int, err error) {
		auditEntry.Result = result
		auditEntry.ExitCode = exitCode
		auditEntry.DurationMS = time.Since(startTime).Milliseconds()
		if err != nil {
			auditEntry.Error = err.Error()
		}
		_ = audit.Log(ctx, auditEntry)
	}

	stateFilePath := args[0]
	auditEntry.Target = stateFilePath

	fmt.Printf("Loading state file: %s\n", stateFilePath)

	// Parse state file
	baseDir := filepath.Dir(stateFilePath)
	parser := statemgmt.NewParser(baseDir)
	stateFile, err := parser.ParseFile(stateFilePath)
	if err != nil {
		err = fmt.Errorf("failed to parse state file: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Validate state file
	validator := statemgmt.NewValidator()
	if errs := validator.Validate(stateFile); len(errs) > 0 {
		err := fmt.Errorf("validation failed: %v", errs)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Load vars if specified
	vars := statemgmt.NewVars()
	if applyVarsFile != "" {
		fmt.Printf("Loading vars from: %s\n", applyVarsFile)
		varsBaseDir := filepath.Dir(applyVarsFile)
		varsParser := statemgmt.NewParser(varsBaseDir)
		varsFile, err := varsParser.ParseFile(applyVarsFile)
		if err != nil {
			err = fmt.Errorf("failed to parse vars file: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		// Extract vars from first state file metadata
		if len(varsFile.Variables) > 0 {
			vars = statemgmt.LoadVarsFromYAML(varsFile.Variables)
		}
	}

	// Collect facts
	facts := statemgmt.NewFacts()

	// Render templates in state file
	if err := statemgmt.RenderStateFile(stateFile, vars, facts); err != nil {
		err = fmt.Errorf("failed to render templates: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Execute state
	executor := statemgmt.NewExecutor()
	executor.DryRun = applyDryRun

	mode := "Applying"
	if applyDryRun {
		mode = "Checking"
	}
	fmt.Printf("%s state: %s\n\n", mode, stateFile.Metadata.Description)

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nExecution failed: %v\n", err)
		printRunSummary(run, time.Since(startTime))
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Print results
	printRunResults(run, applyDryRun)
	printRunSummary(run, time.Since(startTime))

	// Exit with error if any states failed
	if run.Summary != nil && run.Summary.Failed > 0 {
		auditEntry.Extra = map[string]interface{}{
			"dry_run":   applyDryRun,
			"total":     run.Summary.Total,
			"succeeded": run.Summary.Succeeded,
			"failed":    run.Summary.Failed,
			"changed":   run.Summary.Changed,
		}
		logAudit(audit.ResultFailure, 1, fmt.Errorf("%d states failed", run.Summary.Failed))
		os.Exit(1)
	}

	// Log success
	if run.Summary != nil {
		auditEntry.Extra = map[string]interface{}{
			"dry_run":   applyDryRun,
			"total":     run.Summary.Total,
			"succeeded": run.Summary.Succeeded,
			"failed":    run.Summary.Failed,
			"changed":   run.Summary.Changed,
		}
	}
	logAudit(audit.ResultSuccess, 0, nil)

	return nil
}

// Check command

var checkVarsFile string

var checkCmd = &cobra.Command{
	Use:   "check <statefile>",
	Short: "Check state without applying (dry-run)",
	Long: `Check what changes would be made without actually applying them.

This is equivalent to 'apply --dry-run' and is useful for:
  - Previewing changes before applying
  - Validating state files
  - CI/CD checks

Examples:
  # Check a state file
  kscorectl state check states/webserver.yaml

  # Check with variables
  kscorectl state check states/app.yaml --vars vars/staging.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: checkExecute,
}

func init() {
	checkCmd.Flags().StringVar(&checkVarsFile, "vars", "", "Variables file (YAML)")
}

func checkExecute(cmd *cobra.Command, args []string) error {
	// Check is just apply with dry-run
	applyVarsFile = checkVarsFile
	applyDryRun = true
	return applyExecute(cmd, args)
}

// Drift command

var driftVarsFile string

var driftCmd = &cobra.Command{
	Use:   "drift <statefile>",
	Short: "Detect drift from desired state",
	Long: `Detect configuration drift by comparing desired state to actual state.

Drift detection identifies:
  - Files with unexpected content
  - Services in wrong state
  - Missing or extra resources
  - Permission/ownership changes

Examples:
  # Detect drift
  kscorectl state drift states/webserver.yaml

  # Detect drift with variables
  kscorectl state drift states/app.yaml --vars vars/production.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: driftExecute,
}

func init() {
	driftCmd.Flags().StringVar(&driftVarsFile, "vars", "", "Variables file (YAML)")
}

func driftExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()

	// Create audit entry (Epic 15)
	auditEntry := audit.StartEntry(audit.ActionStateApplied, "drift")
	auditEntry.Args = args

	// Helper to log audit on exit
	logAudit := func(result audit.AuditResult, exitCode int, err error) {
		auditEntry.Result = result
		auditEntry.ExitCode = exitCode
		auditEntry.DurationMS = time.Since(startTime).Milliseconds()
		if err != nil {
			auditEntry.Error = err.Error()
		}
		_ = audit.Log(ctx, auditEntry)
	}

	stateFilePath := args[0]
	auditEntry.Target = stateFilePath

	fmt.Printf("Loading state file: %s\n", stateFilePath)

	// Parse state file
	baseDir := filepath.Dir(stateFilePath)
	parser := statemgmt.NewParser(baseDir)
	stateFile, err := parser.ParseFile(stateFilePath)
	if err != nil {
		err = fmt.Errorf("failed to parse state file: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Validate state file
	validator := statemgmt.NewValidator()
	if errs := validator.Validate(stateFile); len(errs) > 0 {
		err := fmt.Errorf("validation failed: %v", errs)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Load vars if specified
	vars := statemgmt.NewVars()
	if driftVarsFile != "" {
		fmt.Printf("Loading vars from: %s\n", driftVarsFile)
		varsBaseDir := filepath.Dir(driftVarsFile)
		varsParser := statemgmt.NewParser(varsBaseDir)
		varsFile, err := varsParser.ParseFile(driftVarsFile)
		if err != nil {
			err = fmt.Errorf("failed to parse vars file: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		if len(varsFile.Variables) > 0 {
			vars = statemgmt.LoadVarsFromYAML(varsFile.Variables)
		}
	}

	// Collect facts
	facts := statemgmt.NewFacts()

	// Render templates in state file
	if err := statemgmt.RenderStateFile(stateFile, vars, facts); err != nil {
		err = fmt.Errorf("failed to render templates: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	fmt.Printf("Checking drift for: %s\n\n", stateFile.Metadata.Description)

	// Check drift
	differ := statemgmt.NewStateDiffer()
	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		err = fmt.Errorf("drift check failed: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Print drift report
	output := statemgmt.FormatDriftReport(report)
	fmt.Println(output)

	// Exit with error if drift detected
	if report.Summary.OverallSeverity != statemgmt.DriftNone {
		auditEntry.Extra = map[string]interface{}{
			"severity":    string(report.Summary.OverallSeverity),
			"total":       report.Summary.Total,
			"no_drift":    report.Summary.NoDrift,
			"low_drift":   report.Summary.LowDrift,
			"medium_drift": report.Summary.MediumDrift,
			"high_drift":  report.Summary.HighDrift,
		}
		logAudit(audit.ResultFailure, 1, fmt.Errorf("drift detected: %s severity", report.Summary.OverallSeverity))
		os.Exit(1)
	}

	// Log success - no drift
	auditEntry.Extra = map[string]interface{}{
		"severity":    string(report.Summary.OverallSeverity),
		"total":       report.Summary.Total,
		"no_drift":    report.Summary.NoDrift,
		"low_drift":   report.Summary.LowDrift,
		"medium_drift": report.Summary.MediumDrift,
		"high_drift":  report.Summary.HighDrift,
	}
	logAudit(audit.ResultSuccess, 0, nil)

	return nil
}

// Helper functions

func printRunResults(run *statemgmt.StateRun, dryRun bool) {
	fmt.Println("=== Results ===")
	for _, result := range run.Results {
		status := "✓"
		if !result.Success {
			status = "✗"
		}

		action := "unchanged"
		if result.Changed {
			action = "changed"
		}
		if dryRun && result.Changed {
			action = "would change"
		}

		fmt.Printf("%s %s.%s: %s\n", status, result.Module, result.StateID, action)

		if result.Comment != "" {
			fmt.Printf("  %s\n", result.Comment)
		}

		if result.Error != nil {
			fmt.Printf("  Error: %v\n", result.Error)
		}

		if result.Changes != nil && len(result.Changes) > 0 {
			fmt.Println("  Changes:")
			for key, value := range result.Changes {
				fmt.Printf("    %s: %v\n", key, value)
			}
		}
	}
	fmt.Println()
}

func printRunSummary(run *statemgmt.StateRun, duration time.Duration) {
	if run == nil || run.Summary == nil {
		return
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Total states:  %d\n", run.Summary.Total)
	fmt.Printf("Succeeded:     %d\n", run.Summary.Succeeded)
	fmt.Printf("Failed:        %d\n", run.Summary.Failed)
	fmt.Printf("Changed:       %d\n", run.Summary.Changed)
	fmt.Printf("Unchanged:     %d\n", run.Summary.Unchanged)
	fmt.Printf("Duration:      %s\n", duration.Round(time.Millisecond))

	if run.Summary.Success {
		fmt.Println("\n✓ Success!")
	} else {
		fmt.Println("\n✗ Failed!")
	}
}
