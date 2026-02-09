package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/audit"
	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	auditLevel  string
	auditOutput string
	stateTarget string

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
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(driftCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(compileCmd)
	rootCmd.AddCommand(varsCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(restoreCmd)
}

func main() {
	// Initialize audit logging (Epic 15)
	auditConfig := &audit.AuditConfig{
		Level:   audit.AuditLevel(auditLevel),
		Backend: auditOutput,
	}
	if err := audit.Init(context.Background(), "kscore-state", auditConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize audit logging: %v\n", err)
	}
	defer audit.Close()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// Apply command

var (
	applyVarsFile string
	applyDryRun   bool
	applyPreview  bool
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
	applyCmd.Flags().StringVar(&stateTarget, "target", "", "Target expression (not used in local mode)")
	applyCmd.Flags().StringVar(&applyVarsFile, "vars", "", "Variables file (YAML)")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Check what would change without applying")
	applyCmd.Flags().BoolVar(&applyDryRun, "check-only", false, "Alias for --dry-run")
	applyCmd.Flags().BoolVar(&applyPreview, "preview", false, "Show rendered state before apply without executing")
}

func applyExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()
	warnTarget(cmd, stateTarget)

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
	result := validator.Validate(stateFile)
	if !result.Valid {
		err := fmt.Errorf("validation failed: %s", result.Summary())
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

	// Handle preview mode - show rendered state without executing
	if applyPreview {
		printStatePreview(stateFile, vars, facts)
		logAudit(audit.ResultSuccess, 0, nil)
		return nil
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
	checkCmd.Flags().StringVar(&stateTarget, "target", "", "Target expression (not used in local mode)")
	checkCmd.Flags().StringVar(&checkVarsFile, "vars", "", "Variables file (YAML)")
}

func checkExecute(cmd *cobra.Command, args []string) error {
	// Check is just apply with dry-run
	applyVarsFile = checkVarsFile
	applyDryRun = true
	return applyExecute(cmd, args)
}

// Drift command

var (
	driftVarsFile string
	driftFix      bool
)

var driftCmd = &cobra.Command{
	Use:   "drift <statefile>",
	Short: "Detect drift from desired state",
	Long: `Detect configuration drift by comparing desired state to actual state.

Drift detection identifies:
  - Files with unexpected content
  - Services in wrong state
  - Missing or extra resources
  - Permission/ownership changes

Use --fix to automatically remediate detected drift by re-applying
the desired state for any drifted resources.

Examples:
  # Detect drift
  kscorectl state drift states/webserver.yaml

  # Detect drift with variables
  kscorectl state drift states/app.yaml --vars vars/production.yaml

  # Detect and auto-fix drift
  kscorectl state drift states/webserver.yaml --fix`,
	Args: cobra.ExactArgs(1),
	RunE: driftExecute,
}

func init() {
	driftCmd.Flags().StringVar(&stateTarget, "target", "", "Target expression (not used in local mode)")
	driftCmd.Flags().StringVar(&driftVarsFile, "vars", "", "Variables file (YAML)")
	driftCmd.Flags().BoolVar(&driftFix, "fix", false, "Auto-remediate detected drift by re-applying desired state")
}

func driftExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()
	warnTarget(cmd, stateTarget)

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
	result := validator.Validate(stateFile)
	if !result.Valid {
		err := fmt.Errorf("validation failed: %s", result.Summary())
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
	driftOutput := statemgmt.FormatDriftReport(report)
	fmt.Println(driftOutput)

	// If drift detected and --fix requested, re-apply state to remediate
	if report.Summary.OverallSeverity != statemgmt.DriftNone && driftFix {
		fmt.Println("=== Auto-Remediation ===")
		fmt.Println("Re-applying desired state to fix detected drift...")
		fmt.Println()

		executor := statemgmt.NewExecutor()
		run, execErr := executor.ExecuteState(ctx, stateFile)
		if execErr != nil {
			fmt.Fprintf(os.Stderr, "\nRemediation failed: %v\n", execErr)
			printRunSummary(run, time.Since(startTime))
			logAudit(audit.ResultFailure, 1, execErr)
			return execErr
		}

		printRunResults(run, false)
		printRunSummary(run, time.Since(startTime))

		fixed := 0
		if run.Summary != nil {
			fixed = run.Summary.Changed
		}

		auditEntry.Extra = map[string]interface{}{
			"severity":     string(report.Summary.OverallSeverity),
			"total":        report.Summary.Total,
			"no_drift":     report.Summary.NoDrift,
			"low_drift":    report.Summary.LowDrift,
			"medium_drift": report.Summary.MediumDrift,
			"high_drift":   report.Summary.HighDrift,
			"fix":          true,
			"fixed":        fixed,
		}

		if run.Summary != nil && run.Summary.Failed > 0 {
			logAudit(audit.ResultFailure, 1, fmt.Errorf("%d states failed during remediation", run.Summary.Failed))
			os.Exit(1)
		}

		logAudit(audit.ResultSuccess, 0, nil)
		return nil
	}

	// Exit with error if drift detected (without --fix)
	if report.Summary.OverallSeverity != statemgmt.DriftNone {
		auditEntry.Extra = map[string]interface{}{
			"severity":     string(report.Summary.OverallSeverity),
			"total":        report.Summary.Total,
			"no_drift":     report.Summary.NoDrift,
			"low_drift":    report.Summary.LowDrift,
			"medium_drift": report.Summary.MediumDrift,
			"high_drift":   report.Summary.HighDrift,
		}
		logAudit(audit.ResultFailure, 1, fmt.Errorf("drift detected: %s severity", report.Summary.OverallSeverity))
		os.Exit(1)
	}

	// Log success - no drift
	auditEntry.Extra = map[string]interface{}{
		"severity":     string(report.Summary.OverallSeverity),
		"total":        report.Summary.Total,
		"no_drift":     report.Summary.NoDrift,
		"low_drift":    report.Summary.LowDrift,
		"medium_drift": report.Summary.MediumDrift,
		"high_drift":   report.Summary.HighDrift,
	}
	logAudit(audit.ResultSuccess, 0, nil)

	return nil
}

func warnTarget(cmd *cobra.Command, target string) {
	if target == "" {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Warning: --target is not supported for local state runs; applying locally.")
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

		if len(result.Changes) > 0 {
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

// printStatePreview prints a preview of the rendered state file without executing
func printStatePreview(stateFile *statemgmt.StateFile, vars *statemgmt.Vars, facts *statemgmt.Facts) {
	fmt.Println()
	fmt.Println("=== Rendered State Preview ===")
	fmt.Println()

	// Show metadata
	fmt.Println("Metadata:")
	if stateFile.Metadata.Name != "" {
		fmt.Printf("  Name:        %s\n", stateFile.Metadata.Name)
	}
	if stateFile.Metadata.Description != "" {
		fmt.Printf("  Description: %s\n", stateFile.Metadata.Description)
	}
	if stateFile.Metadata.Version != "" {
		fmt.Printf("  Version:     %s\n", stateFile.Metadata.Version)
	}
	fmt.Println()

	// Show applied variables if any
	if vars != nil && len(vars.Data) > 0 {
		fmt.Println("Variables Applied:")
		for key, val := range vars.Data {
			fmt.Printf("  %s: %v\n", key, val)
		}
		fmt.Println()
	}

	// Show facts
	if facts != nil && len(facts.Data) > 0 {
		fmt.Println("System Facts:")
		for key, val := range facts.Data {
			fmt.Printf("  %s: %v\n", key, val)
		}
		fmt.Println()
	}

	// Resolve execution order
	executionOrder, err := statemgmt.ResolveExecutionOrder(stateFile)
	if err != nil {
		fmt.Printf("Warning: Could not resolve execution order: %v\n", err)
		fmt.Println("Showing states in declaration order instead.")
		fmt.Println()
		printStatesInDeclarationOrder(stateFile)
	} else {
		fmt.Printf("States (%d total, in execution order):\n", len(executionOrder))
		fmt.Println("─────────────────────────────────────────────────")
		for i, decl := range executionOrder {
			printStateDeclaration(i+1, decl)
		}
	}

	// Show dependency graph
	fmt.Println()
	fmt.Println("=== Dependency Graph ===")
	viz := statemgmt.NewGraphVisualizer()
	if err := viz.BuildFromStateFile(stateFile); err == nil {
		fmt.Println(viz.Render(statemgmt.GraphFormatText))
	} else {
		fmt.Printf("Could not build dependency graph: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Preview complete. Use --dry-run to check what would change,")
	fmt.Println("or apply without flags to apply the state.")
}

// printStatesInDeclarationOrder prints states when execution order cannot be resolved
func printStatesInDeclarationOrder(stateFile *statemgmt.StateFile) {
	i := 1
	for module, declarations := range stateFile.States {
		for j := range declarations {
			declarations[j].Module = module
			printStateDeclaration(i, &declarations[j])
			i++
		}
	}
}

// printStateDeclaration prints a single state declaration
func printStateDeclaration(index int, decl *statemgmt.StateDeclaration) {
	fmt.Printf("\n%d. %s.%s\n", index, decl.Module, decl.ID)
	fmt.Printf("   State: %s\n", decl.State)

	if len(decl.Parameters) > 0 {
		fmt.Println("   Parameters:")
		for key, val := range decl.Parameters {
			// Truncate long values
			valStr := fmt.Sprintf("%v", val)
			if len(valStr) > 60 {
				valStr = valStr[:57] + "..."
			}
			fmt.Printf("     %s: %s\n", key, valStr)
		}
	}

	// Show requisites
	if hasRequisites(&decl.Requisites) {
		fmt.Println("   Requisites:")
		printRequisiteList("require", decl.Requisites.Require)
		printRequisiteList("require_in", decl.Requisites.RequireIn)
		printRequisiteList("watch", decl.Requisites.Watch)
		printRequisiteList("watch_in", decl.Requisites.WatchIn)
		printRequisiteList("prereq", decl.Requisites.Prereq)
		printRequisiteList("prereq_in", decl.Requisites.PrereqIn)
		printRequisiteList("onchanges", decl.Requisites.Onchanges)
		printRequisiteList("onchanges_in", decl.Requisites.OnchangesIn)
	}
}

// hasRequisites checks if any requisites are defined
func hasRequisites(req *statemgmt.Requisites) bool {
	return len(req.Require) > 0 || len(req.RequireIn) > 0 ||
		len(req.Watch) > 0 || len(req.WatchIn) > 0 ||
		len(req.Prereq) > 0 || len(req.PrereqIn) > 0 ||
		len(req.Onchanges) > 0 || len(req.OnchangesIn) > 0
}

// printRequisiteList prints a list of requisites of a specific type
func printRequisiteList(name string, refs []statemgmt.StateReference) {
	if len(refs) == 0 {
		return
	}
	fmt.Printf("     %s:\n", name)
	for _, ref := range refs {
		fmt.Printf("       - %s.%s\n", ref.Module, ref.ID)
	}
}

// Test command - alias for check with test-focused messaging

var testVarsFile string

var testCmd = &cobra.Command{
	Use:   "test <statefile>",
	Short: "Test state declarations (dry-run)",
	Long: `Test state declarations without applying them.

The test command performs a dry-run that simulates state application
without making actual changes. Useful for validating state files.

Examples:
  # Test a state file
  kscorectl state test states/webserver.yaml

  # Test with specific target
  kscorectl state test states/app.yaml --target web-01

  # Test with variables
  kscorectl state test states/app.yaml --vars vars/staging.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: testExecute,
}

func init() {
	testCmd.Flags().StringVar(&stateTarget, "target", "", "Target expression (not used in local mode)")
	testCmd.Flags().StringVar(&testVarsFile, "vars", "", "Variables file (YAML)")
}

func testExecute(cmd *cobra.Command, args []string) error {
	// Test is like check but with different messaging
	applyVarsFile = testVarsFile
	applyDryRun = true
	fmt.Println("Running state test (dry-run mode)...")
	return applyExecute(cmd, args)
}

// Diff command - compare desired vs actual state

var diffVarsFile string

var diffCmd = &cobra.Command{
	Use:     "diff <statefile>",
	Aliases: []string{"compare"},
	Short:   "Show differences between desired and actual state",
	Long: `Show what would change if the state were applied.

The diff command compares the desired state (from the state file) with
the actual current state of the system, showing the differences.

This is an alias for the 'drift' command with output focused on changes.

Examples:
  # Show differences
  kscorectl state diff states/webserver.yaml

  # Show differences for specific target
  kscorectl state diff states/app.yaml --target web-01

  # Show differences with variables
  kscorectl state diff states/app.yaml --vars vars/production.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: diffExecute,
}

func init() {
	diffCmd.Flags().StringVar(&stateTarget, "target", "", "Target expression (not used in local mode)")
	diffCmd.Flags().StringVar(&diffVarsFile, "vars", "", "Variables file (YAML)")
}

func diffExecute(cmd *cobra.Command, args []string) error {
	// Diff is similar to drift but focused on showing changes
	driftVarsFile = diffVarsFile
	fmt.Println("Comparing desired state with actual state...")
	return driftExecute(cmd, args)
}

// Show command - display rendered state

var showVarsFile string

var showCmd = &cobra.Command{
	Use:   "show <statefile>",
	Short: "Show rendered state declarations",
	Long: `Display the rendered state declarations after template processing.

The show command parses the state file, applies variables and facts,
and displays the fully rendered state without executing anything.

Useful for:
  - Debugging template rendering
  - Reviewing what would be applied
  - Verifying variable substitution

Examples:
  # Show rendered state
  kscorectl state show states/webserver.yaml

  # Show with variables
  kscorectl state show states/app.yaml --vars vars/production.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: showExecute,
}

func init() {
	showCmd.Flags().StringVar(&showVarsFile, "vars", "", "Variables file (YAML)")
}

func showExecute(cmd *cobra.Command, args []string) error {
	stateFilePath := args[0]

	fmt.Printf("Loading state file: %s\n", stateFilePath)

	// Parse state file
	baseDir := filepath.Dir(stateFilePath)
	parser := statemgmt.NewParser(baseDir)
	stateFile, err := parser.ParseFile(stateFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	// Validate state file
	validator := statemgmt.NewValidator()
	result := validator.Validate(stateFile)
	if !result.Valid {
		return fmt.Errorf("validation failed: %s", result.Summary())
	}

	// Load vars if specified
	vars := statemgmt.NewVars()
	if showVarsFile != "" {
		fmt.Printf("Loading vars from: %s\n", showVarsFile)
		varsBaseDir := filepath.Dir(showVarsFile)
		varsParser := statemgmt.NewParser(varsBaseDir)
		varsFile, err := varsParser.ParseFile(showVarsFile)
		if err != nil {
			return fmt.Errorf("failed to parse vars file: %w", err)
		}
		if len(varsFile.Variables) > 0 {
			vars = statemgmt.LoadVarsFromYAML(varsFile.Variables)
		}
	}

	// Collect facts
	facts := statemgmt.NewFacts()

	// Render templates in state file
	if err := statemgmt.RenderStateFile(stateFile, vars, facts); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	// Print the rendered state
	printStatePreview(stateFile, vars, facts)

	return nil
}

// History command - list state application history

var (
	historyLimit int
	historyJSON  bool
)

var historyCmd = &cobra.Command{
	Use:   "history [application-id]",
	Short: "List or show state application history",
	Long: `List state application history or show details of a specific application.

Without arguments, lists recent state applications. With an application ID,
shows detailed information about that specific application.

Note: This command requires connectivity to the control plane to retrieve
the application history. In local mode, history is not tracked.

Examples:
  # List recent applications
  kscorectl state history --limit 20

  # Show details of a specific application
  kscorectl state history app-123

  # List applications for a specific target
  kscorectl state history --target "role:web" --limit 10`,
	Args: cobra.MaximumNArgs(1),
	RunE: historyExecute,
}

func init() {
	historyCmd.Flags().StringVar(&stateTarget, "target", "", "Filter by target expression")
	historyCmd.Flags().IntVar(&historyLimit, "limit", 20, "Maximum number of entries to show")
	historyCmd.Flags().BoolVar(&historyJSON, "json", false, "Output in JSON format")
}

func historyExecute(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		// Show specific application
		appID := args[0]
		fmt.Printf("State Application: %s\n", appID)
		fmt.Println()
		fmt.Println("Note: State application history requires control plane connectivity.")
		fmt.Println("This feature tracks state applications across the fleet and stores")
		fmt.Println("history on the server for auditing and rollback purposes.")
		fmt.Println()
		fmt.Println("To use this feature, ensure:")
		fmt.Println("  1. The control plane server is running")
		fmt.Println("  2. States are applied via kscorectl (not local state runs)")
		fmt.Println("  3. The server has history retention enabled")
		return nil
	}

	// List applications
	fmt.Printf("Recent State Applications (limit: %d)\n", historyLimit)
	if stateTarget != "" {
		fmt.Printf("Filtered by: %s\n", stateTarget)
	}
	fmt.Println()
	fmt.Println("Note: State application history requires control plane connectivity.")
	fmt.Println("Local state runs (like apply, check) do not persist history.")
	fmt.Println()
	fmt.Println("History tracking is available when:")
	fmt.Println("  - Applying states via remote execution")
	fmt.Println("  - Using the control plane's state management API")
	fmt.Println()
	fmt.Println("Example of server-side history (when connected):")
	fmt.Println()
	fmt.Println("  ID           TIMESTAMP            TARGET     STATUS   CHANGES")
	fmt.Println("  app-abc123   2024-01-19 10:30:00  role:web   success  5 changed")
	fmt.Println("  app-def456   2024-01-19 10:15:00  role:db    success  2 changed")
	fmt.Println("  app-ghi789   2024-01-19 10:00:00  role:web   failed   0 changed")
	return nil
}

// Rollback command - rollback to previous state

var (
	rollbackForce bool
	rollbackDry   bool
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <application-id>",
	Short: "Rollback to a previous state application",
	Long: `Rollback the system to a previous state application.

This command restores the system to the state it was in at the time
of a previous state application.

Note: This command requires connectivity to the control plane, as
state history and snapshots are stored on the server.

Examples:
  # Rollback to a specific application
  kscorectl state rollback app-123

  # Dry-run rollback to see what would change
  kscorectl state rollback app-123 --dry-run

  # Force rollback without confirmation
  kscorectl state rollback app-123 --force`,
	Args: cobra.ExactArgs(1),
	RunE: rollbackExecute,
}

func init() {
	rollbackCmd.Flags().BoolVar(&rollbackForce, "force", false, "Skip confirmation prompt")
	rollbackCmd.Flags().BoolVar(&rollbackDry, "dry-run", false, "Show what would be rolled back without applying")
}

func rollbackExecute(cmd *cobra.Command, args []string) error {
	appID := args[0]

	mode := "Rollback"
	if rollbackDry {
		mode = "Dry-run rollback"
	}
	fmt.Printf("%s to state application: %s\n", mode, appID)
	fmt.Println()
	fmt.Println("Note: State rollback requires control plane connectivity.")
	fmt.Println()
	fmt.Println("The rollback feature works by:")
	fmt.Println("  1. Retrieving the state snapshot from the specified application")
	fmt.Println("  2. Comparing it to the current system state")
	fmt.Println("  3. Generating a rollback state that reverses the changes")
	fmt.Println("  4. Applying the rollback state to restore the previous state")
	fmt.Println()
	fmt.Println("To use this feature, ensure:")
	fmt.Println("  - The control plane server is running")
	fmt.Println("  - State snapshots are enabled (default for server-side applications)")
	fmt.Println("  - The application ID exists in the history")
	fmt.Println()
	fmt.Println("For local state files, consider using version control (git) to")
	fmt.Println("manage state file versions and manually apply the previous version.")
	return nil
}

// Compile command - compile and resolve a state definition

var (
	compileVars     []string
	compileVarsFile string
	compileOutput   string
)

var compileCmd = &cobra.Command{
	Use:   "compile <statefile>",
	Short: "Compile a state definition and output the resolved result",
	Long: `Compile a state definition by resolving variables and templates.

The compile command parses the state file, applies variables and facts,
resolves all templates, and outputs the fully compiled result without
executing any state changes.

Output formats:
  - yaml (default): YAML representation of the compiled state
  - json: JSON representation of the compiled state

Examples:
  # Compile a state file
  kscorectl state compile states/webserver.yaml

  # Compile with inline variables
  kscorectl state compile states/app.yaml --vars port=8080 --vars env=prod

  # Compile with a variables file
  kscorectl state compile states/app.yaml --vars-file vars/production.yaml

  # Output as JSON
  kscorectl state compile states/app.yaml --output json`,
	Args: cobra.ExactArgs(1),
	RunE: compileExecute,
}

func init() {
	compileCmd.Flags().StringArrayVar(&compileVars, "vars", nil, "Inline variables (key=value, can be repeated)")
	compileCmd.Flags().StringVar(&compileVarsFile, "vars-file", "", "Variables file (YAML)")
	compileCmd.Flags().StringVar(&compileOutput, "output", "yaml", "Output format (yaml, json)")
}

func compileExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()

	auditEntry := audit.StartEntry(audit.ActionStateApplied, "compile")
	auditEntry.Args = args

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
	valResult := validator.Validate(stateFile)
	if !valResult.Valid {
		err := fmt.Errorf("validation failed: %s", valResult.Summary())
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Load vars from file if specified
	vars := statemgmt.NewVars()
	if compileVarsFile != "" {
		varsBaseDir := filepath.Dir(compileVarsFile)
		varsParser := statemgmt.NewParser(varsBaseDir)
		varsFile, err := varsParser.ParseFile(compileVarsFile)
		if err != nil {
			err = fmt.Errorf("failed to parse vars file: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		if len(varsFile.Variables) > 0 {
			vars = statemgmt.LoadVarsFromYAML(varsFile.Variables)
		}
	}

	// Merge inline vars (--vars key=value)
	for _, kv := range compileVars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			vars.Set(parts[0], parts[1])
		}
	}

	// Collect facts
	facts := statemgmt.NewFacts()

	// Render templates
	if err := statemgmt.RenderStateFile(stateFile, vars, facts); err != nil {
		err = fmt.Errorf("failed to render templates: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Build compiled output structure
	compiled := buildCompiledOutput(stateFile, vars, facts)

	// Marshal to requested format
	switch compileOutput {
	case "json":
		data, err := json.MarshalIndent(compiled, "", "  ")
		if err != nil {
			err = fmt.Errorf("failed to marshal JSON: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		fmt.Println(string(data))
	default:
		data, err := yaml.Marshal(compiled)
		if err != nil {
			err = fmt.Errorf("failed to marshal YAML: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		fmt.Print(string(data))
	}

	logAudit(audit.ResultSuccess, 0, nil)
	return nil
}

// buildCompiledOutput constructs a map representing the fully compiled state.
func buildCompiledOutput(stateFile *statemgmt.StateFile, vars *statemgmt.Vars, facts *statemgmt.Facts) map[string]interface{} {
	compiled := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        stateFile.Metadata.Name,
			"description": stateFile.Metadata.Description,
			"version":     stateFile.Metadata.Version,
			"tags":        stateFile.Metadata.Tags,
		},
	}

	if len(vars.Data) > 0 {
		compiled["variables"] = vars.Data
	}

	if len(facts.Data) > 0 {
		compiled["facts"] = facts.Data
	}

	states := map[string]interface{}{}
	for module, declarations := range stateFile.States {
		declList := make([]map[string]interface{}, 0, len(declarations))
		for _, decl := range declarations {
			d := map[string]interface{}{
				"id":         decl.ID,
				"state":      decl.State,
				"parameters": decl.Parameters,
			}
			declList = append(declList, d)
		}
		states[module] = declList
	}
	compiled["states"] = states

	return compiled
}

// Vars command group

var varsCmd = &cobra.Command{
	Use:   "vars",
	Short: "Manage state variables",
	Long: `View and manage state variables used in state templates.

Variables control how state templates are rendered and allow
the same state definitions to be reused across environments.

Examples:
  # Get a specific variable
  kscorectl state vars get http_port

  # List all variables
  kscorectl state vars list`,
}

// Vars get subcommand

var (
	varsGetScope string
	varsGetAgent string
	varsGetRole  string
)

var varsGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Retrieve a specific variable value",
	Long: `Retrieve the value of a specific state variable.

Variables can be scoped to different levels:
  - global: Available to all agents (default)
  - agent: Specific to a single agent
  - role: Specific to agents with a given role

Examples:
  # Get a global variable
  kscorectl state vars get http_port

  # Get an agent-scoped variable
  kscorectl state vars get http_port --scope agent --agent web-01

  # Get a role-scoped variable
  kscorectl state vars get http_port --scope role --role webserver`,
	Args: cobra.ExactArgs(1),
	RunE: varsGetExecute,
}

// Vars list subcommand

var (
	varsListScope string
	varsListAgent string
	varsListRole  string
)

var varsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all variables",
	Long: `List all state variables with their current values.

Variables can be filtered by scope:
  - global: Variables available to all agents (default)
  - agent: Variables specific to a single agent
  - role: Variables specific to agents with a given role

Examples:
  # List all global variables
  kscorectl state vars list

  # List variables for a specific agent
  kscorectl state vars list --scope agent --agent web-01

  # List variables for a role
  kscorectl state vars list --scope role --role webserver`,
	Args: cobra.NoArgs,
	RunE: varsListExecute,
}

func init() {
	varsGetCmd.Flags().StringVar(&varsGetScope, "scope", "global", "Variable scope (global, agent, role)")
	varsGetCmd.Flags().StringVar(&varsGetAgent, "agent", "", "Agent ID (required when scope is agent)")
	varsGetCmd.Flags().StringVar(&varsGetRole, "role", "", "Role name (required when scope is role)")

	varsListCmd.Flags().StringVar(&varsListScope, "scope", "global", "Variable scope (global, agent, role)")
	varsListCmd.Flags().StringVar(&varsListAgent, "agent", "", "Agent ID (required when scope is agent)")
	varsListCmd.Flags().StringVar(&varsListRole, "role", "", "Role name (required when scope is role)")

	varsCmd.AddCommand(varsGetCmd)
	varsCmd.AddCommand(varsListCmd)
}

func varsGetExecute(cmd *cobra.Command, args []string) error {
	key := args[0]

	fmt.Printf("Variable: %s\n", key)
	fmt.Printf("Scope:    %s\n", varsGetScope)

	switch varsGetScope {
	case "agent":
		if varsGetAgent == "" {
			return fmt.Errorf("--agent is required when scope is agent")
		}
		fmt.Printf("Agent:    %s\n", varsGetAgent)
	case "role":
		if varsGetRole == "" {
			return fmt.Errorf("--role is required when scope is role")
		}
		fmt.Printf("Role:     %s\n", varsGetRole)
	}

	fmt.Println()
	fmt.Println("Note: Variable retrieval requires control plane connectivity.")
	fmt.Println("Variables are stored and managed on the server.")
	fmt.Println()
	fmt.Println("Example output (when connected):")
	fmt.Println()
	fmt.Printf("  %s = <value>\n", key)
	fmt.Println()
	fmt.Println("To set variables, use state files with a 'variables' section or")
	fmt.Println("configure them through the control plane API.")
	return nil
}

func varsListExecute(cmd *cobra.Command, args []string) error {
	fmt.Printf("Scope: %s\n", varsListScope)

	switch varsListScope {
	case "agent":
		if varsListAgent == "" {
			return fmt.Errorf("--agent is required when scope is agent")
		}
		fmt.Printf("Agent: %s\n", varsListAgent)
	case "role":
		if varsListRole == "" {
			return fmt.Errorf("--role is required when scope is role")
		}
		fmt.Printf("Role:  %s\n", varsListRole)
	}

	fmt.Println()
	fmt.Println("Note: Variable listing requires control plane connectivity.")
	fmt.Println("Variables are stored and managed on the server.")
	fmt.Println()
	fmt.Println("Example output (when connected):")
	fmt.Println()
	fmt.Println("  KEY             VALUE         SOURCE")
	fmt.Println("  http_port       8080          global")
	fmt.Println("  environment     production    global")
	fmt.Println("  max_workers     4             role:webserver")
	fmt.Println("  log_level       debug         agent:web-01")
	return nil
}

// Export command - export state to a file

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export current state to a file",
	Long: `Export the current managed state to a file for backup or migration.

The export command captures the current state of all managed resources
and writes them to a file that can later be restored.

Examples:
  # Export state to a file
  kscorectl state export --output state-backup.yaml

  # Export to default location
  kscorectl state export`,
	Args: cobra.NoArgs,
	RunE: exportExecute,
}

func init() {
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file path (defaults to stdout)")
}

func exportExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()

	auditEntry := audit.StartEntry(audit.ActionStateApplied, "export")
	auditEntry.Args = args

	logAudit := func(result audit.AuditResult, exitCode int, err error) {
		auditEntry.Result = result
		auditEntry.ExitCode = exitCode
		auditEntry.DurationMS = time.Since(startTime).Milliseconds()
		if err != nil {
			auditEntry.Error = err.Error()
		}
		_ = audit.Log(ctx, auditEntry)
	}

	fmt.Println("Exporting current state...")
	fmt.Println()
	fmt.Println("Note: State export requires control plane connectivity.")
	fmt.Println("The export captures the current state of all managed resources")
	fmt.Println("from the server's state database.")
	fmt.Println()

	if exportOutput != "" {
		fmt.Printf("Output file: %s\n", exportOutput)
		auditEntry.Target = exportOutput
	} else {
		fmt.Println("Output: stdout (use --output to write to a file)")
	}

	fmt.Println()
	fmt.Println("Example export (when connected):")
	fmt.Println()
	fmt.Println("  metadata:")
	fmt.Println("    exported_at: " + time.Now().UTC().Format(time.RFC3339))
	fmt.Println("    version: \"1.0\"")
	fmt.Println("    total_resources: 42")
	fmt.Println("  resources:")
	fmt.Println("    - module: file")
	fmt.Println("      id: /etc/nginx/nginx.conf")
	fmt.Println("      state: present")
	fmt.Println("    - module: service")
	fmt.Println("      id: nginx")
	fmt.Println("      state: running")

	logAudit(audit.ResultSuccess, 0, nil)
	return nil
}

// Restore command - restore state from a file

var restoreInput string

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore state from a previously exported file",
	Long: `Restore managed state from a previously exported state file.

The restore command reads a state export file and applies it to
bring the system back to the exported state.

Examples:
  # Restore state from a file
  kscorectl state restore --input state-backup.yaml`,
	Args: cobra.NoArgs,
	RunE: restoreExecute,
}

func init() {
	restoreCmd.Flags().StringVar(&restoreInput, "input", "", "Input file path (required)")
	_ = restoreCmd.MarkFlagRequired("input")
}

func restoreExecute(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx := context.Background()

	auditEntry := audit.StartEntry(audit.ActionStateApplied, "restore")
	auditEntry.Args = args
	auditEntry.Target = restoreInput

	logAudit := func(result audit.AuditResult, exitCode int, err error) {
		auditEntry.Result = result
		auditEntry.ExitCode = exitCode
		auditEntry.DurationMS = time.Since(startTime).Milliseconds()
		if err != nil {
			auditEntry.Error = err.Error()
		}
		_ = audit.Log(ctx, auditEntry)
	}

	fmt.Printf("Restoring state from: %s\n", restoreInput)
	fmt.Println()

	// Verify the input file exists
	if _, err := os.Stat(restoreInput); os.IsNotExist(err) {
		err = fmt.Errorf("input file not found: %s", restoreInput)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	fmt.Println("Note: State restore requires control plane connectivity.")
	fmt.Println("The restore operation will apply the exported state to bring")
	fmt.Println("managed resources back to the captured configuration.")
	fmt.Println()
	fmt.Println("The restore process:")
	fmt.Println("  1. Parse the export file and validate its format")
	fmt.Println("  2. Compare exported state with current state")
	fmt.Println("  3. Generate a remediation plan")
	fmt.Println("  4. Apply changes to reach the exported state")
	fmt.Println()
	fmt.Println("To preview changes before restoring, use:")
	fmt.Printf("  kscorectl state restore --input %s --dry-run\n", restoreInput)

	logAudit(audit.ResultSuccess, 0, nil)
	return nil
}
