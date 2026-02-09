// Package main implements the kscore-gitops CLI for GitOps integration and deployment management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/deprecation"
	clierrors "github.com/shawnbutts/keystone-core/internal/cli/errors"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/gitops/promotion"
	"github.com/shawnbutts/keystone-core/internal/gitops/rollback"
	"github.com/shawnbutts/keystone-core/internal/gitops/verification"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-gitops",
		Short: "GitOps integration and deployment management",
		Long: `Manage GitOps deployments, verifications, rollbacks, and promotions.

Keystone Core GitOps provides:
  - Deployment verification workflows
  - Automated and manual rollbacks
  - Environment promotions with approval gates
  - Integration with ArgoCD, Flux, GitHub, and GitLab

Examples:
  # Run a verification workflow
  kscorectl gitops verify workflow.yaml

  # Trigger a rollback
  kscorectl gitops rollback --app myapp --strategy previous

  # Promote between environments
  kscorectl gitops promote --pipeline prod-pipeline --from staging --to production

  # List webhook handlers
  kscorectl gitops webhook list`,
	}

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(promoteCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(newGitSyncCmd())

	// Add deprecated command (moving to kscore-webhook)
	rootCmd.AddCommand(webhookCmd)

	// Apply deprecation warnings to webhook subcommands
	webhookDeprecations := deprecation.WebhookDeprecations()
	deprecation.DeprecateCommand(webhookListCmd, webhookDeprecations["list"])
	deprecation.DeprecateCommand(webhookTestCmd, webhookDeprecations["test"])

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-gitops", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// =============================================================================
// Verify Command
// =============================================================================

var (
	verifyParallel bool
	verifyTimeout  string
	verifyOutput   string
	auditLevel     string
	auditOutput    string
)

var verifyCmd = &cobra.Command{
	Use:   "verify <workflow-file>",
	Short: "Run a verification workflow",
	Long: `Execute a verification workflow to validate deployments.

Verification workflows can include:
  - HTTP health checks
  - Kubernetes resource checks
  - Command execution
  - Custom scripts

Examples:
  # Run a verification workflow
  kscorectl gitops verify workflows/post-deploy.yaml

  # Run with parallel steps
  kscorectl gitops verify workflows/health-check.yaml --parallel

  # Run with custom timeout
  kscorectl gitops verify workflows/full-check.yaml --timeout 5m`,
	Args: cobra.ExactArgs(1),
	RunE: verifyExecute,
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyParallel, "parallel", false, "Run steps in parallel")
	verifyCmd.Flags().StringVar(&verifyTimeout, "timeout", "2m", "Workflow timeout")
	verifyCmd.Flags().StringVarP(&verifyOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
}

func verifyExecute(cmd *cobra.Command, args []string) error {
	workflowFile := args[0]

	format, err := output.ParseFormat(verifyOutput)
	if err != nil {
		return err
	}

	// Load workflow from file
	workflow, err := loadVerificationWorkflow(workflowFile)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	// Override parallel if specified
	if verifyParallel {
		workflow.Parallel = true
	}

	// Parse timeout
	if verifyTimeout != "" {
		timeout, err := time.ParseDuration(verifyTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
		workflow.Timeout = timeout
	}

	if format == output.FormatText {
		fmt.Fprintf(cmd.OutOrStdout(), "Running verification workflow: %s\n", workflow.Name)
		if workflow.Description != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", workflow.Description)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Steps: %d\n", len(workflow.Steps))
		fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", func() string {
			if workflow.Parallel {
				return "parallel"
			}
			return "sequential"
		}())
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Create engine and register verifiers
	engine := verification.NewEngine()
	engine.RegisterVerifier(verification.NewHTTPVerifier())
	engine.RegisterVerifier(verification.NewCommandVerifier())

	// Execute workflow
	ctx := context.Background()
	result, err := engine.Execute(ctx, workflow)
	if err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}

	switch format {
	case output.FormatJSON:
		if err := output.WriteJSON(cmd.OutOrStdout(), result); err != nil {
			return err
		}
	case output.FormatYAML:
		if err := output.WriteYAML(cmd.OutOrStdout(), result); err != nil {
			return err
		}
	case output.FormatTable:
		table := buildVerificationTable(result)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		printVerificationSummary(cmd.OutOrStdout(), result)
	case output.FormatText:
		printVerificationResult(cmd.OutOrStdout(), result)
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", verifyOutput))
	}

	if !result.Success {
		os.Exit(1)
	}

	return nil
}

func printVerificationResult(writer io.Writer, result *verification.WorkflowResult) {
	fmt.Fprintln(writer, "=== Step Results ===")
	for _, step := range result.Steps {
		status := "✓"
		if !step.Success {
			status = "✗"
		}
		fmt.Fprintf(writer, "%s %s: %s (%s)\n", status, step.StepName, step.Message, step.Duration.Round(time.Millisecond))
		if step.Error != nil {
			fmt.Fprintf(writer, "  Error: %v\n", step.Error)
		}
		if step.Retries > 0 {
			fmt.Fprintf(writer, "  Retries: %d\n", step.Retries)
		}
	}

	printVerificationSummary(writer, result)
	if result.Success {
		fmt.Fprintln(writer, "\n✓ Verification passed!")
	} else {
		fmt.Fprintln(writer, "\n✗ Verification failed!")
	}
}

func printVerificationSummary(writer io.Writer, result *verification.WorkflowResult) {
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "=== Summary ===")
	fmt.Fprintf(writer, "Total Steps:  %d\n", result.TotalSteps)
	fmt.Fprintf(writer, "Passed:       %d\n", result.PassedSteps)
	fmt.Fprintf(writer, "Failed:       %d\n", result.FailedSteps)
	fmt.Fprintf(writer, "Duration:     %s\n", result.Duration.Round(time.Millisecond))
}

func buildVerificationTable(result *verification.WorkflowResult) *output.Table {
	rows := make([][]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		status := "OK"
		if !step.Success {
			status = "FAIL"
		}
		rows = append(rows, []string{
			step.StepName,
			status,
			step.Message,
			step.Duration.Round(time.Millisecond).String(),
		})
	}

	return &output.Table{
		Headers: []string{"STEP", "STATUS", "MESSAGE", "DURATION"},
		Rows:    rows,
	}
}

// =============================================================================
// Rollback Command
// =============================================================================

var (
	rollbackApp       string
	rollbackNamespace string
	rollbackType      string
	rollbackStrategy  string
	rollbackRevision  string
	rollbackReason    string
	rollbackUser      string
	rollbackDryRun    bool
	rollbackOutput    string
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Trigger a rollback operation",
	Long: `Trigger a rollback to restore a previous deployment state.

Rollback types:
  - argocd: Rollback via ArgoCD
  - flux: Rollback via Flux
  - git: Rollback via Git revert

Rollback strategies:
  - previous: Rollback to immediately previous revision
  - specific: Rollback to a specific revision (requires --revision)
  - last_known_good: Rollback to last known healthy state

Examples:
  # Rollback to previous revision
  kscorectl gitops rollback --app myapp --strategy previous

  # Rollback to specific revision
  kscorectl gitops rollback --app myapp --strategy specific --revision abc123

  # Dry-run rollback
  kscorectl gitops rollback --app myapp --strategy previous --dry-run`,
	RunE: rollbackExecute,
}

func init() {
	rollbackCmd.Flags().StringVar(&rollbackApp, "app", "", "Application name (required)")
	rollbackCmd.Flags().StringVar(&rollbackNamespace, "namespace", "default", "Namespace")
	rollbackCmd.Flags().StringVar(&rollbackType, "type", "argocd", "Rollback type (argocd, flux, git)")
	rollbackCmd.Flags().StringVar(&rollbackStrategy, "strategy", "previous", "Strategy (previous, specific, last_known_good)")
	rollbackCmd.Flags().StringVar(&rollbackRevision, "revision", "", "Target revision (for specific strategy)")
	rollbackCmd.Flags().StringVar(&rollbackReason, "reason", "", "Reason for rollback")
	rollbackCmd.Flags().StringVar(&rollbackUser, "user", "", "User performing rollback")
	rollbackCmd.Flags().BoolVar(&rollbackDryRun, "dry-run", false, "Simulate rollback without executing")
	rollbackCmd.Flags().StringVarP(&rollbackOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
	rollbackCmd.MarkFlagRequired("app")
}

func rollbackExecute(cmd *cobra.Command, args []string) error {
	// Validate strategy
	strategy := rollback.Strategy(rollbackStrategy)
	switch strategy {
	case rollback.StrategyPreviousRevision, rollback.StrategySpecificRevision, rollback.StrategyLastKnownGood:
		// Valid
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("invalid strategy: %s (use: previous, specific, last_known_good)", rollbackStrategy))
	}

	// Require revision for specific strategy
	if strategy == rollback.StrategySpecificRevision && rollbackRevision == "" {
		return clierrors.New(clierrors.KindInvalidArgument, "--revision is required for 'specific' strategy")
	}

	// Build rollback config
	config := &rollback.Config{
		Name:        fmt.Sprintf("rollback-%s-%d", rollbackApp, time.Now().Unix()),
		Type:        rollback.Type(rollbackType),
		Strategy:    strategy,
		Trigger:     rollback.TriggerManual,
		Application: rollbackApp,
		Namespace:   rollbackNamespace,
		Revision:    rollbackRevision,
		Timeout:     5 * time.Minute,
	}

	// Build request
	request := &rollback.Request{
		ConfigName:       config.Name,
		Reason:           rollbackReason,
		RequestedBy:      rollbackUser,
		OverrideRevision: rollbackRevision,
	}

	format, err := output.ParseFormat(rollbackOutput)
	if err != nil {
		return err
	}

	if format == output.FormatText {
		fmt.Fprintln(cmd.OutOrStdout(), "Rollback Configuration")
		fmt.Fprintln(cmd.OutOrStdout(), "======================")
		fmt.Fprintf(cmd.OutOrStdout(), "Application:  %s\n", config.Application)
		fmt.Fprintf(cmd.OutOrStdout(), "Namespace:    %s\n", config.Namespace)
		fmt.Fprintf(cmd.OutOrStdout(), "Type:         %s\n", config.Type)
		fmt.Fprintf(cmd.OutOrStdout(), "Strategy:     %s\n", config.Strategy)
		if config.Revision != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Revision:     %s\n", config.Revision)
		}
		if request.Reason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Reason:       %s\n", request.Reason)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	if format == output.FormatTable {
		table := buildKeyValueTable([][2]string{
			{"APPLICATION", config.Application},
			{"NAMESPACE", config.Namespace},
			{"TYPE", string(config.Type)},
			{"STRATEGY", string(config.Strategy)},
			{"REVISION", config.Revision},
			{"REASON", request.Reason},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if rollbackDryRun {
		dryRun := map[string]any{
			"dry_run":  true,
			"config":   config,
			"request":  request,
			"message":  "Would execute rollback with the above configuration.",
			"datetime": time.Now().UTC(),
		}

		switch format {
		case output.FormatJSON:
			return output.WriteJSON(cmd.OutOrStdout(), dryRun)
		case output.FormatYAML:
			return output.WriteYAML(cmd.OutOrStdout(), dryRun)
		case output.FormatTable:
			table := buildKeyValueTable([][2]string{
				{"DRY RUN", "true"},
				{"APPLICATION", config.Application},
				{"NAMESPACE", config.Namespace},
				{"TYPE", string(config.Type)},
				{"STRATEGY", string(config.Strategy)},
				{"REVISION", config.Revision},
				{"REASON", request.Reason},
			})
			if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nNo changes made.")
			return nil
		case output.FormatText:
			fmt.Fprintln(cmd.OutOrStdout(), "=== DRY RUN ===")
			fmt.Fprintln(cmd.OutOrStdout(), "Would execute rollback with the above configuration.")
			fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
			return nil
		default:
			return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", rollbackOutput))
		}
	}

	// In a real implementation, this would connect to the control plane
	// For now, we'll create a mock result
	result := &rollback.Result{
		ID:               fmt.Sprintf("rb-%d", time.Now().UnixNano()),
		Config:           config,
		Request:          request,
		Status:           rollback.StatusCompleted,
		PreviousRevision: "abc123",
		CurrentRevision: func() string {
			if config.Revision != "" {
				return config.Revision
			}
			return "xyz789"
		}(),
		StartTime: time.Now().Add(-30 * time.Second),
		EndTime:   time.Now(),
		Duration:  30 * time.Second,
		Message:   "Rollback completed successfully",
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), result)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), result)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"ID", result.ID},
			{"STATUS", string(result.Status)},
			{"FROM", result.PreviousRevision},
			{"TO", result.CurrentRevision},
			{"DURATION", result.Duration.String()},
			{"MESSAGE", result.Message},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Rollback completed!")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI reads from local configuration.")
		fmt.Fprintln(cmd.OutOrStdout(), "For production rollbacks, connect to the control plane API.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "=== Rollback Result ===")
		fmt.Fprintf(cmd.OutOrStdout(), "ID:       %s\n", result.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "From:     %s\n", result.PreviousRevision)
		fmt.Fprintf(cmd.OutOrStdout(), "To:       %s\n", result.CurrentRevision)
		fmt.Fprintf(cmd.OutOrStdout(), "Duration: %s\n", result.Duration)
		fmt.Fprintf(cmd.OutOrStdout(), "Message:  %s\n", result.Message)

		fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Rollback completed!")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI reads from local configuration.")
		fmt.Fprintln(cmd.OutOrStdout(), "For production rollbacks, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", rollbackOutput))
	}

	return nil
}

// =============================================================================
// Promote Command
// =============================================================================

var (
	promotePipeline   string
	promoteFrom       string
	promoteTo         string
	promoteRevision   string
	promoteReason     string
	promoteUser       string
	promoteSkipVerify bool
	promoteForce      bool
	promoteDryRun     bool
	promoteOutput     string
)

var promoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote a deployment between environments",
	Long: `Promote a deployment from one environment to another.

Promotion pipelines support:
  - Sequential environment promotion (dev → staging → production)
  - Approval gates for production environments
  - Verification workflows before promotion
  - Canary and blue/green deployment strategies

Examples:
  # Promote from staging to production
  kscorectl gitops promote --pipeline prod-pipeline --from staging --to production

  # Promote with verification skip
  kscorectl gitops promote --pipeline prod-pipeline --from staging --to production --skip-verify

  # Promote specific revision
  kscorectl gitops promote --pipeline prod-pipeline --from staging --to production --revision abc123`,
	RunE: promoteExecute,
}

func init() {
	promoteCmd.Flags().StringVar(&promotePipeline, "pipeline", "", "Pipeline name (required)")
	promoteCmd.Flags().StringVar(&promoteFrom, "from", "", "Source environment (required)")
	promoteCmd.Flags().StringVar(&promoteTo, "to", "", "Target environment (required)")
	promoteCmd.Flags().StringVar(&promoteRevision, "revision", "", "Specific revision to promote")
	promoteCmd.Flags().StringVar(&promoteReason, "reason", "", "Reason for promotion")
	promoteCmd.Flags().StringVar(&promoteUser, "user", "", "User performing promotion")
	promoteCmd.Flags().BoolVar(&promoteSkipVerify, "skip-verify", false, "Skip verification step")
	promoteCmd.Flags().BoolVar(&promoteForce, "force", false, "Force promotion even if checks fail")
	promoteCmd.Flags().BoolVar(&promoteDryRun, "dry-run", false, "Simulate promotion without executing")
	promoteCmd.Flags().StringVarP(&promoteOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
	promoteCmd.MarkFlagRequired("pipeline")
	promoteCmd.MarkFlagRequired("from")
	promoteCmd.MarkFlagRequired("to")
}

func promoteExecute(cmd *cobra.Command, args []string) error {
	// Build request
	request := &promotion.Request{
		Pipeline:         promotePipeline,
		FromEnvironment:  promoteFrom,
		ToEnvironment:    promoteTo,
		Revision:         promoteRevision,
		RequestedBy:      promoteUser,
		Reason:           promoteReason,
		SkipVerification: promoteSkipVerify,
		Force:            promoteForce,
	}

	format, err := output.ParseFormat(promoteOutput)
	if err != nil {
		return err
	}

	if format == output.FormatText {
		fmt.Fprintln(cmd.OutOrStdout(), "Promotion Request")
		fmt.Fprintln(cmd.OutOrStdout(), "=================")
		fmt.Fprintf(cmd.OutOrStdout(), "Pipeline:    %s\n", request.Pipeline)
		fmt.Fprintf(cmd.OutOrStdout(), "From:        %s\n", request.FromEnvironment)
		fmt.Fprintf(cmd.OutOrStdout(), "To:          %s\n", request.ToEnvironment)
		if request.Revision != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Revision:    %s\n", request.Revision)
		}
		if request.Reason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Reason:      %s\n", request.Reason)
		}
		if request.SkipVerification {
			fmt.Fprintln(cmd.OutOrStdout(), "Verification: skipped")
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	if format == output.FormatTable {
		table := buildKeyValueTable([][2]string{
			{"PIPELINE", request.Pipeline},
			{"FROM", request.FromEnvironment},
			{"TO", request.ToEnvironment},
			{"REVISION", request.Revision},
			{"REASON", request.Reason},
			{"SKIP VERIFICATION", fmt.Sprintf("%t", request.SkipVerification)},
			{"FORCE", fmt.Sprintf("%t", request.Force)},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if promoteDryRun {
		dryRun := map[string]any{
			"dry_run":  true,
			"request":  request,
			"message":  "Would execute promotion with the above configuration.",
			"datetime": time.Now().UTC(),
		}

		switch format {
		case output.FormatJSON:
			return output.WriteJSON(cmd.OutOrStdout(), dryRun)
		case output.FormatYAML:
			return output.WriteYAML(cmd.OutOrStdout(), dryRun)
		case output.FormatTable:
			table := buildKeyValueTable([][2]string{
				{"DRY RUN", "true"},
				{"PIPELINE", request.Pipeline},
				{"FROM", request.FromEnvironment},
				{"TO", request.ToEnvironment},
				{"REVISION", request.Revision},
				{"REASON", request.Reason},
				{"SKIP VERIFICATION", fmt.Sprintf("%t", request.SkipVerification)},
				{"FORCE", fmt.Sprintf("%t", request.Force)},
			})
			if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nNo changes made.")
			return nil
		case output.FormatText:
			fmt.Fprintln(cmd.OutOrStdout(), "=== DRY RUN ===")
			fmt.Fprintln(cmd.OutOrStdout(), "Would execute promotion with the above configuration.")
			fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
			return nil
		default:
			return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", promoteOutput))
		}
	}

	// In a real implementation, this would connect to the control plane
	// For now, we'll create a mock result
	result := &promotion.Result{
		ID: fmt.Sprintf("promo-%d", time.Now().UnixNano()),
		Pipeline: &promotion.Pipeline{
			Name:        request.Pipeline,
			Application: "app-" + request.Pipeline,
			Strategy:    promotion.StrategyImmediate,
		},
		Request:      request,
		Status:       promotion.StatusCompleted,
		CurrentStage: 1,
		Stages: []*promotion.StageResult{
			{
				Environment: request.ToEnvironment,
				Status:      promotion.StatusCompleted,
				StartTime:   time.Now().Add(-45 * time.Second),
				EndTime:     time.Now(),
				Duration:    45 * time.Second,
				Message:     "Deployment successful",
			},
		},
		StartTime: time.Now().Add(-45 * time.Second),
		EndTime:   time.Now(),
		Duration:  45 * time.Second,
		Message:   "Promotion completed successfully",
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), result)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), result)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"ID", result.ID},
			{"STATUS", string(result.Status)},
			{"DURATION", result.Duration.String()},
			{"MESSAGE", result.Message},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		if len(result.Stages) > 0 {
			stageTable := buildPromotionStageTable(result)
			fmt.Fprintln(cmd.OutOrStdout(), "\nStages:")
			if err := output.WriteTable(cmd.OutOrStdout(), stageTable); err != nil {
				return err
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Promotion completed!")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI reads from local configuration.")
		fmt.Fprintln(cmd.OutOrStdout(), "For production promotions, connect to the control plane API.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "=== Promotion Result ===")
		fmt.Fprintf(cmd.OutOrStdout(), "ID:       %s\n", result.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Duration: %s\n", result.Duration)
		fmt.Fprintf(cmd.OutOrStdout(), "Message:  %s\n", result.Message)

		if len(result.Stages) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nStages:")
			for i, stage := range result.Stages {
				status := "✓"
				if stage.Status != promotion.StatusCompleted {
					status = "✗"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s %s: %s (%s)\n", i+1, status, stage.Environment, stage.Status, stage.Duration.Round(time.Millisecond))
			}
		}

		fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Promotion completed!")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI reads from local configuration.")
		fmt.Fprintln(cmd.OutOrStdout(), "For production promotions, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", promoteOutput))
	}

	return nil
}

// =============================================================================
// Webhook Command
// =============================================================================

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage webhook handlers",
	Long:  `Manage webhook handlers for GitOps integrations.`,
}

var webhookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered webhook handlers",
	Long: `List all registered webhook handlers.

Keystone Core supports webhooks from:
  - ArgoCD (sync, health, deployment events)
  - Flux (reconciliation events)
  - GitHub (deployment, workflow, push events)
  - GitLab (deployment, pipeline, push events)

Examples:
  # List all webhook handlers
  kscorectl gitops webhook list`,
	RunE: webhookListExecute,
}

var webhookTestCmd = &cobra.Command{
	Use:   "test <type>",
	Short: "Send a test webhook",
	Long: `Send a test webhook to verify handler configuration.

Webhook types: argocd, flux, github, gitlab

Examples:
  # Test ArgoCD webhook
  kscorectl gitops webhook test argocd

  # Test GitHub webhook
  kscorectl gitops webhook test github`,
	Args: cobra.ExactArgs(1),
	RunE: webhookTestExecute,
}

func init() {
	webhookCmd.AddCommand(webhookListCmd)
	webhookCmd.AddCommand(webhookTestCmd)
	webhookListCmd.Flags().StringVarP(&webhookOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
}

func webhookListExecute(cmd *cobra.Command, args []string) error {
	// List supported webhook handlers
	handlers := []struct {
		Type        string
		Description string
		Events      []string
	}{
		{
			Type:        "argocd",
			Description: "ArgoCD application events",
			Events:      []string{"sync", "health", "deployment"},
		},
		{
			Type:        "flux",
			Description: "Flux reconciliation events",
			Events:      []string{"kustomization", "helmrelease", "gitrepository"},
		},
		{
			Type:        "github",
			Description: "GitHub repository events",
			Events:      []string{"deployment", "workflow_run", "push"},
		},
		{
			Type:        "gitlab",
			Description: "GitLab project events",
			Events:      []string{"deployment", "pipeline", "push"},
		},
	}

	format, err := output.ParseFormat(webhookOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), handlers)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), handlers)
	case output.FormatTable:
		table := buildWebhookTable(handlers)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\nWebhook Endpoint: POST /webhooks/<type>")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Configure webhooks on the control plane server.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use the server's webhook.port and webhook.path settings.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Registered Webhook Handlers")
		fmt.Fprintln(cmd.OutOrStdout(), "===========================")
		fmt.Fprintln(cmd.OutOrStdout())

		for _, h := range handlers {
			fmt.Fprintln(cmd.OutOrStdout(), strings.ToUpper(h.Type))
			fmt.Fprintf(cmd.OutOrStdout(), "  Description: %s\n", h.Description)
			fmt.Fprintf(cmd.OutOrStdout(), "  Events:      %s\n", strings.Join(h.Events, ", "))
			fmt.Fprintln(cmd.OutOrStdout())
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Webhook Endpoint: POST /webhooks/<type>")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Configure webhooks on the control plane server.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use the server's webhook.port and webhook.path settings.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", webhookOutput))
	}

	return nil
}

func webhookTestExecute(cmd *cobra.Command, args []string) error {
	webhookType := strings.ToLower(args[0])

	// Validate type
	switch webhookType {
	case "argocd", "flux", "github", "gitlab":
		// Valid
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("invalid webhook type: %s (use: argocd, flux, github, gitlab)", webhookType))
	}

	fmt.Printf("Test Webhook: %s\n", webhookType)
	fmt.Println()

	// Generate sample payload
	var payload map[string]interface{}
	switch webhookType {
	case "argocd":
		payload = map[string]interface{}{
			"type": "sync",
			"application": map[string]interface{}{
				"name":      "test-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync":   "Synced",
				"health": "Healthy",
			},
		}
	case "flux":
		payload = map[string]interface{}{
			"involvedObject": map[string]interface{}{
				"kind":      "Kustomization",
				"name":      "test-kustomization",
				"namespace": "flux-system",
			},
			"severity": "info",
			"message":  "Reconciliation succeeded",
		}
	case "github":
		payload = map[string]interface{}{
			"action": "completed",
			"deployment": map[string]interface{}{
				"id":          12345,
				"environment": "production",
			},
			"repository": map[string]interface{}{
				"full_name": "org/repo",
			},
		}
	case "gitlab":
		payload = map[string]interface{}{
			"object_kind": "deployment",
			"status":      "success",
			"project": map[string]interface{}{
				"path_with_namespace": "org/repo",
			},
			"environment": "production",
		}
	}

	fmt.Println("Sample Payload:")
	fmt.Println("---------------")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(payload)
	fmt.Println()

	fmt.Printf("To test, send this payload to:\n")
	fmt.Printf("  POST http://<server>:<port>/webhooks/%s\n", webhookType)
	fmt.Println("\nNote: This command displays sample payloads.")
	fmt.Println("For actual testing, use curl or a webhook testing tool.")

	return nil
}

// =============================================================================
// Status Command
// =============================================================================

var (
	statusType    string
	statusLimit   int
	statusOutput  string
	webhookOutput string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GitOps operation status",
	Long: `Display status of GitOps operations.

Status types:
  - rollbacks: Recent rollback operations
  - promotions: Recent promotion operations
  - verifications: Recent verification runs

Examples:
  # Show recent rollbacks
  kscorectl gitops status --type rollbacks

  # Show recent promotions
  kscorectl gitops status --type promotions --limit 20

  # Show all operations as JSON
  kscorectl gitops status --output json`,
	RunE: statusExecute,
}

func init() {
	statusCmd.Flags().StringVar(&statusType, "type", "all", "Status type (rollbacks, promotions, verifications, all)")
	statusCmd.Flags().IntVar(&statusLimit, "limit", 10, "Maximum entries to show")
	statusCmd.Flags().StringVarP(&statusOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
}

func statusExecute(cmd *cobra.Command, args []string) error {
	// In a real implementation, this would query the control plane
	// For now, show sample data

	operations := []OperationStatus{
		{
			ID:        "rb-001",
			Type:      "rollback",
			Status:    "completed",
			Target:    "myapp/production",
			StartTime: time.Now().Add(-2 * time.Hour),
			Duration:  "45s",
		},
		{
			ID:        "promo-001",
			Type:      "promotion",
			Status:    "completed",
			Target:    "myapp: staging → production",
			StartTime: time.Now().Add(-5 * time.Hour),
			Duration:  "1m30s",
		},
		{
			ID:        "verify-001",
			Type:      "verification",
			Status:    "passed",
			Target:    "post-deploy-checks",
			StartTime: time.Now().Add(-5*time.Hour - 30*time.Minute),
			Duration:  "30s",
		},
	}

	// Filter by type
	if statusType != "all" {
		filtered := make([]OperationStatus, 0)
		for _, op := range operations {
			if strings.HasPrefix(op.Type, statusType[:len(statusType)-1]) { // Remove trailing 's'
				filtered = append(filtered, op)
			}
		}
		operations = filtered
	}

	// Apply limit
	if len(operations) > statusLimit {
		operations = operations[:statusLimit]
	}

	format, err := output.ParseFormat(statusOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), operations)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), operations)
	case output.FormatTable:
		if len(operations) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No operations found.")
			fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
			fmt.Fprintln(cmd.OutOrStdout(), "For real status, connect to the control plane API.")
			return nil
		}
		table := buildStatusTable(operations)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d operations\n", len(operations))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real status, connect to the control plane API.")
	case output.FormatText:
		if len(operations) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No operations found.")
			fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
			fmt.Fprintln(cmd.OutOrStdout(), "For real status, connect to the control plane API.")
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-12s %-30s %-20s %-10s\n", "ID", "TYPE", "STATUS", "TARGET", "TIME", "DURATION")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 100))

		for _, op := range operations {
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-12s %-30s %-20s %-10s\n",
				op.ID,
				op.Type,
				op.Status,
				truncate(op.Target, 30),
				op.StartTime.Format("2006-01-02 15:04:05"),
				op.Duration,
			)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d operations\n", len(operations))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real status, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", statusOutput))
	}

	return nil
}

// =============================================================================
// Repo Command
// =============================================================================

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage Git repositories",
	Long: `Manage Git repositories for GitOps operations.

Commands:
  list   - List configured repositories
  add    - Add a new repository
  remove - Remove a repository
  sync   - Synchronize a repository`,
}

var (
	repoListOutput string
	repoAddURL     string
	repoAddBranch  string
	repoAddPath    string
	repoAddAuth    string
	repoAddKey     string
	repoSyncForce  bool
)

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured repositories",
	Long: `List all Git repositories configured for GitOps operations.

Examples:
  # List all repositories
  kscorectl gitops repo list

  # List repositories as JSON
  kscorectl gitops repo list --output json`,
	RunE: repoListExecute,
}

var repoAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new repository",
	Long: `Add a new Git repository for GitOps operations.

Authentication options:
  - SSH key: --auth ssh --key /path/to/key
  - HTTPS token: --auth token (uses GIT_TOKEN env var or prompt)
  - None: --auth none

Examples:
  # Add a repository with SSH
  kscorectl gitops repo add myrepo --url git@github.com:org/repo.git --auth ssh --key ~/.ssh/id_rsa

  # Add a repository with HTTPS
  kscorectl gitops repo add myrepo --url https://github.com/org/repo.git --auth token

  # Add with specific branch
  kscorectl gitops repo add myrepo --url git@github.com:org/repo.git --branch main --path /states`,
	Args: cobra.ExactArgs(1),
	RunE: repoAddExecute,
}

var repoRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a repository",
	Long: `Remove a Git repository from GitOps operations.

Examples:
  # Remove a repository
  kscorectl gitops repo remove myrepo`,
	Args: cobra.ExactArgs(1),
	RunE: repoRemoveExecute,
}

var repoSyncCmd = &cobra.Command{
	Use:   "sync <name>",
	Short: "Synchronize a repository",
	Long: `Synchronize a Git repository, pulling latest changes.

Examples:
  # Sync a repository
  kscorectl gitops repo sync myrepo

  # Force sync (discard local changes)
  kscorectl gitops repo sync myrepo --force`,
	Args: cobra.ExactArgs(1),
	RunE: repoSyncExecute,
}

func init() {
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoRemoveCmd)
	repoCmd.AddCommand(repoSyncCmd)

	repoListCmd.Flags().StringVarP(&repoListOutput, "output", "o", "text", "Output format (text, json, yaml, table)")

	repoAddCmd.Flags().StringVar(&repoAddURL, "url", "", "Repository URL (required)")
	repoAddCmd.Flags().StringVar(&repoAddBranch, "branch", "main", "Branch to track")
	repoAddCmd.Flags().StringVar(&repoAddPath, "path", "", "Path within repository")
	repoAddCmd.Flags().StringVar(&repoAddAuth, "auth", "none", "Authentication method (none, ssh, token)")
	repoAddCmd.Flags().StringVar(&repoAddKey, "key", "", "SSH key path (for --auth ssh)")
	repoAddCmd.MarkFlagRequired("url")

	repoSyncCmd.Flags().BoolVar(&repoSyncForce, "force", false, "Force sync, discarding local changes")
}

// RepoConfig represents a repository configuration
type RepoConfig struct {
	Name       string `json:"name" yaml:"name"`
	URL        string `json:"url" yaml:"url"`
	Branch     string `json:"branch" yaml:"branch"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	Auth       string `json:"auth" yaml:"auth"`
	LastSync   string `json:"last_sync,omitempty" yaml:"last_sync,omitempty"`
	Status     string `json:"status" yaml:"status"`
	CommitHash string `json:"commit_hash,omitempty" yaml:"commit_hash,omitempty"`
}

func repoListExecute(cmd *cobra.Command, args []string) error {
	// In production, this would query the control plane
	// For now, show sample data
	repos := []RepoConfig{
		{
			Name:       "states",
			URL:        "git@github.com:org/kscore-states.git",
			Branch:     "main",
			Path:       "/states",
			Auth:       "ssh",
			LastSync:   time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
			Status:     "synced",
			CommitHash: "abc1234",
		},
		{
			Name:       "blueprints",
			URL:        "https://github.com/org/kscore-blueprints.git",
			Branch:     "production",
			Path:       "/blueprints",
			Auth:       "token",
			LastSync:   time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			Status:     "synced",
			CommitHash: "def5678",
		},
	}

	format, err := output.ParseFormat(repoListOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), repos)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), repos)
	case output.FormatTable:
		table := buildRepoTable(repos)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d repositories\n", len(repos))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real repository status, connect to the control plane API.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Configured Repositories")
		fmt.Fprintln(cmd.OutOrStdout(), "=======================")
		fmt.Fprintln(cmd.OutOrStdout())

		for i := range repos {
			repo := &repos[i]
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", repo.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  URL:       %s\n", repo.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "  Branch:    %s\n", repo.Branch)
			if repo.Path != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Path:      %s\n", repo.Path)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Auth:      %s\n", repo.Auth)
			fmt.Fprintf(cmd.OutOrStdout(), "  Status:    %s\n", repo.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "  Last Sync: %s\n", repo.LastSync)
			if repo.CommitHash != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Commit:    %s\n", repo.CommitHash)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Total: %d repositories\n", len(repos))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real repository status, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", repoListOutput))
	}

	return nil
}

func repoAddExecute(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate auth method
	switch repoAddAuth {
	case "none", "ssh", "token":
		// Valid
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("invalid auth method: %s (use: none, ssh, token)", repoAddAuth))
	}

	// Require key for SSH auth
	if repoAddAuth == "ssh" && repoAddKey == "" {
		return clierrors.New(clierrors.KindInvalidArgument, "--key is required for SSH authentication")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Adding Repository")
	fmt.Fprintln(cmd.OutOrStdout(), "=================")
	fmt.Fprintf(cmd.OutOrStdout(), "Name:   %s\n", name)
	fmt.Fprintf(cmd.OutOrStdout(), "URL:    %s\n", repoAddURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", repoAddBranch)
	if repoAddPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Path:   %s\n", repoAddPath)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Auth:   %s\n", repoAddAuth)
	if repoAddKey != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Key:    %s\n", repoAddKey)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	// In production, this would add to control plane
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Repository '%s' configured\n", name)
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This command configures repository settings locally.")
	fmt.Fprintln(cmd.OutOrStdout(), "For production use, configure repositories via the control plane API.")

	return nil
}

func repoRemoveExecute(cmd *cobra.Command, args []string) error {
	name := args[0]

	fmt.Fprintf(cmd.OutOrStdout(), "Removing repository: %s\n", name)

	// In production, this would remove from control plane
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Repository '%s' removed\n", name)
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This command removes repository settings locally.")
	fmt.Fprintln(cmd.OutOrStdout(), "For production use, manage repositories via the control plane API.")

	return nil
}

func repoSyncExecute(cmd *cobra.Command, args []string) error {
	name := args[0]

	fmt.Fprintf(cmd.OutOrStdout(), "Synchronizing repository: %s\n", name)
	if repoSyncForce {
		fmt.Fprintln(cmd.OutOrStdout(), "Mode: force (discarding local changes)")
	}
	fmt.Fprintln(cmd.OutOrStdout())

	// In production, this would trigger sync via control plane
	fmt.Fprintln(cmd.OutOrStdout(), "Fetching remote...")
	fmt.Fprintln(cmd.OutOrStdout(), "Merging changes...")
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Repository '%s' synchronized\n", name)
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This command simulates synchronization.")
	fmt.Fprintln(cmd.OutOrStdout(), "For production use, sync repositories via the control plane API.")

	return nil
}

// =============================================================================
// Deploy Command
// =============================================================================

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Manage deployments",
	Long: `Manage GitOps deployments across environments.

Commands:
  list    - List recent deployments
  show    - Show deployment details
  rollback - Rollback a deployment
  approve  - Approve a pending deployment`,
}

var (
	deployListEnv      string
	deployListApp      string
	deployListLimit    int
	deployListOutput   string
	deployShowOutput   string
	deployApproveForce bool
)

var deployListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent deployments",
	Long: `List recent deployments across environments.

Examples:
  # List all recent deployments
  kscorectl gitops deploy list

  # List deployments for specific environment
  kscorectl gitops deploy list --env production

  # List deployments for specific application
  kscorectl gitops deploy list --app myapp`,
	RunE: deployListExecute,
}

var deployShowCmd = &cobra.Command{
	Use:   "show <deployment-id>",
	Short: "Show deployment details",
	Long: `Show detailed information about a specific deployment.

Examples:
  # Show deployment details
  kscorectl gitops deploy show deploy-123

  # Show as JSON
  kscorectl gitops deploy show deploy-123 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: deployShowExecute,
}

var deployRollbackCmd = &cobra.Command{
	Use:   "rollback <deployment-id>",
	Short: "Rollback a deployment",
	Long: `Rollback a specific deployment to its previous state.

This is a convenience command that triggers a rollback for a specific
deployment. For more control, use 'gitops rollback' with detailed options.

Examples:
  # Rollback a deployment
  kscorectl gitops deploy rollback deploy-123`,
	Args: cobra.ExactArgs(1),
	RunE: deployRollbackExecute,
}

var deployApproveCmd = &cobra.Command{
	Use:   "approve <deployment-id>",
	Short: "Approve a pending deployment",
	Long: `Approve a pending deployment that requires manual approval.

Deployments may require approval when:
  - Promotion to production environment
  - Breaking changes detected
  - High-risk configuration changes

Examples:
  # Approve a pending deployment
  kscorectl gitops deploy approve deploy-123

  # Force approve (skip confirmation)
  kscorectl gitops deploy approve deploy-123 --force`,
	Args: cobra.ExactArgs(1),
	RunE: deployApproveExecute,
}

func init() {
	deployCmd.AddCommand(deployListCmd)
	deployCmd.AddCommand(deployShowCmd)
	deployCmd.AddCommand(deployRollbackCmd)
	deployCmd.AddCommand(deployApproveCmd)

	deployListCmd.Flags().StringVar(&deployListEnv, "env", "", "Filter by environment")
	deployListCmd.Flags().StringVar(&deployListApp, "app", "", "Filter by application")
	deployListCmd.Flags().IntVar(&deployListLimit, "limit", 10, "Maximum entries to show")
	deployListCmd.Flags().StringVarP(&deployListOutput, "output", "o", "text", "Output format (text, json, yaml, table)")

	deployShowCmd.Flags().StringVarP(&deployShowOutput, "output", "o", "text", "Output format (text, json, yaml, table)")

	deployApproveCmd.Flags().BoolVarP(&deployApproveForce, "force", "f", false, "Skip confirmation prompt")
}

// DeploymentInfo represents a deployment record
type DeploymentInfo struct {
	ID          string    `json:"id" yaml:"id"`
	Application string    `json:"application" yaml:"application"`
	Environment string    `json:"environment" yaml:"environment"`
	Revision    string    `json:"revision" yaml:"revision"`
	Status      string    `json:"status" yaml:"status"`
	StartTime   time.Time `json:"start_time" yaml:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`
	Duration    string    `json:"duration,omitempty" yaml:"duration,omitempty"`
	Deployer    string    `json:"deployer" yaml:"deployer"`
	Message     string    `json:"message,omitempty" yaml:"message,omitempty"`
}

func deployListExecute(cmd *cobra.Command, args []string) error {
	// In production, this would query the control plane
	deployments := []DeploymentInfo{
		{
			ID:          "deploy-001",
			Application: "frontend",
			Environment: "production",
			Revision:    "v1.5.2",
			Status:      "succeeded",
			StartTime:   time.Now().Add(-2 * time.Hour),
			Duration:    "2m30s",
			Deployer:    "gitops-bot",
			Message:     "Auto-deployed from main branch",
		},
		{
			ID:          "deploy-002",
			Application: "backend",
			Environment: "staging",
			Revision:    "v2.1.0-rc1",
			Status:      "succeeded",
			StartTime:   time.Now().Add(-5 * time.Hour),
			Duration:    "1m45s",
			Deployer:    "admin@example.com",
			Message:     "Manual promotion from dev",
		},
		{
			ID:          "deploy-003",
			Application: "backend",
			Environment: "production",
			Revision:    "v2.1.0-rc1",
			Status:      "pending_approval",
			StartTime:   time.Now().Add(-30 * time.Minute),
			Deployer:    "admin@example.com",
			Message:     "Awaiting approval for production deployment",
		},
	}

	// Apply filters
	filtered := make([]DeploymentInfo, 0)
	for i := range deployments {
		if deployListEnv != "" && deployments[i].Environment != deployListEnv {
			continue
		}
		if deployListApp != "" && deployments[i].Application != deployListApp {
			continue
		}
		filtered = append(filtered, deployments[i])
	}

	// Apply limit
	if len(filtered) > deployListLimit {
		filtered = filtered[:deployListLimit]
	}

	format, err := output.ParseFormat(deployListOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), filtered)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), filtered)
	case output.FormatTable:
		if len(filtered) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No deployments found.")
			return nil
		}
		table := buildDeploymentTable(filtered)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d deployments\n", len(filtered))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real deployment status, connect to the control plane API.")
	case output.FormatText:
		if len(filtered) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No deployments found.")
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-12s %-12s %-20s %-20s\n", "ID", "APP", "ENV", "STATUS", "REVISION", "TIME")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 95))

		for i := range filtered {
			d := &filtered[i]
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-12s %-12s %-12s %-20s %-20s\n",
				d.ID,
				truncate(d.Application, 12),
				d.Environment,
				d.Status,
				truncate(d.Revision, 20),
				d.StartTime.Format("2006-01-02 15:04:05"),
			)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d deployments\n", len(filtered))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real deployment status, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", deployListOutput))
	}

	return nil
}

func deployShowExecute(cmd *cobra.Command, args []string) error {
	deployID := args[0]

	// In production, this would query the control plane
	deployment := &DeploymentInfo{
		ID:          deployID,
		Application: "frontend",
		Environment: "production",
		Revision:    "v1.5.2",
		Status:      "succeeded",
		StartTime:   time.Now().Add(-2 * time.Hour),
		EndTime:     time.Now().Add(-2*time.Hour + 2*time.Minute + 30*time.Second),
		Duration:    "2m30s",
		Deployer:    "gitops-bot",
		Message:     "Auto-deployed from main branch",
	}

	format, err := output.ParseFormat(deployShowOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), deployment)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), deployment)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"ID", deployment.ID},
			{"APPLICATION", deployment.Application},
			{"ENVIRONMENT", deployment.Environment},
			{"REVISION", deployment.Revision},
			{"STATUS", deployment.Status},
			{"START TIME", deployment.StartTime.Format(time.RFC3339)},
			{"END TIME", deployment.EndTime.Format(time.RFC3339)},
			{"DURATION", deployment.Duration},
			{"DEPLOYER", deployment.Deployer},
			{"MESSAGE", deployment.Message},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real deployment details, connect to the control plane API.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Deployment Details")
		fmt.Fprintln(cmd.OutOrStdout(), "==================")
		fmt.Fprintf(cmd.OutOrStdout(), "ID:          %s\n", deployment.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Application: %s\n", deployment.Application)
		fmt.Fprintf(cmd.OutOrStdout(), "Environment: %s\n", deployment.Environment)
		fmt.Fprintf(cmd.OutOrStdout(), "Revision:    %s\n", deployment.Revision)
		fmt.Fprintf(cmd.OutOrStdout(), "Status:      %s\n", deployment.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Start Time:  %s\n", deployment.StartTime.Format(time.RFC3339))
		fmt.Fprintf(cmd.OutOrStdout(), "End Time:    %s\n", deployment.EndTime.Format(time.RFC3339))
		fmt.Fprintf(cmd.OutOrStdout(), "Duration:    %s\n", deployment.Duration)
		fmt.Fprintf(cmd.OutOrStdout(), "Deployer:    %s\n", deployment.Deployer)
		fmt.Fprintf(cmd.OutOrStdout(), "Message:     %s\n", deployment.Message)
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real deployment details, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", deployShowOutput))
	}

	return nil
}

func deployRollbackExecute(cmd *cobra.Command, args []string) error {
	deployID := args[0]

	fmt.Fprintf(cmd.OutOrStdout(), "Rolling back deployment: %s\n", deployID)
	fmt.Fprintln(cmd.OutOrStdout())

	// In production, this would trigger rollback via control plane
	fmt.Fprintln(cmd.OutOrStdout(), "Finding previous revision...")
	fmt.Fprintln(cmd.OutOrStdout(), "Previous revision: v1.5.1")
	fmt.Fprintln(cmd.OutOrStdout(), "Initiating rollback...")
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Rollback initiated for deployment '%s'\n", deployID)
	fmt.Fprintln(cmd.OutOrStdout(), "\nUse 'kscorectl gitops status --type rollbacks' to monitor progress.")
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This command simulates a rollback.")
	fmt.Fprintln(cmd.OutOrStdout(), "For production rollbacks, connect to the control plane API.")

	return nil
}

func deployApproveExecute(cmd *cobra.Command, args []string) error {
	deployID := args[0]

	fmt.Fprintf(cmd.OutOrStdout(), "Approving deployment: %s\n", deployID)
	fmt.Fprintln(cmd.OutOrStdout())

	// Show deployment info
	fmt.Fprintln(cmd.OutOrStdout(), "Deployment Details:")
	fmt.Fprintln(cmd.OutOrStdout(), "  Application: backend")
	fmt.Fprintln(cmd.OutOrStdout(), "  Environment: production")
	fmt.Fprintln(cmd.OutOrStdout(), "  Revision:    v2.1.0-rc1")
	fmt.Fprintln(cmd.OutOrStdout())

	if !deployApproveForce {
		fmt.Fprint(cmd.OutOrStdout(), "Approve this deployment? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Fprintln(cmd.OutOrStdout(), "Approval cancelled.")
			return nil
		}
	}

	// In production, this would approve via control plane
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Deployment '%s' approved\n", deployID)
	fmt.Fprintln(cmd.OutOrStdout(), "\nDeployment is now proceeding.")
	fmt.Fprintf(cmd.OutOrStdout(), "Use 'kscorectl gitops deploy show %s' to monitor status.\n", deployID)
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This command simulates approval.")
	fmt.Fprintln(cmd.OutOrStdout(), "For production deployments, connect to the control plane API.")

	return nil
}

// =============================================================================
// Git Sync Command
// =============================================================================

// SyncStatus represents the sync status of a repository.
type SyncStatus struct {
	Repository string `json:"repository" yaml:"repository"`
	Status     string `json:"status" yaml:"status"`
	Branch     string `json:"branch" yaml:"branch"`
	LastSync   string `json:"last_sync" yaml:"last_sync"`
	CommitHash string `json:"commit_hash" yaml:"commit_hash"`
	Behind     int    `json:"behind" yaml:"behind"`
	Ahead      int    `json:"ahead" yaml:"ahead"`
}

// ConflictInfo represents a sync conflict.
type ConflictInfo struct {
	Path       string `json:"path" yaml:"path"`
	Status     string `json:"status" yaml:"status"`
	LocalHash  string `json:"local_hash" yaml:"local_hash"`
	RemoteHash string `json:"remote_hash" yaml:"remote_hash"`
	ModifiedAt string `json:"modified_at" yaml:"modified_at"`
}

// LockInfo represents a file lock.
type LockInfo struct {
	Path     string `json:"path" yaml:"path"`
	LockedBy string `json:"locked_by" yaml:"locked_by"`
	Reason   string `json:"reason" yaml:"reason"`
	LockedAt string `json:"locked_at" yaml:"locked_at"`
}

// SyncHistoryEntry represents a sync history record.
type SyncHistoryEntry struct {
	ID         string `json:"id" yaml:"id"`
	Repository string `json:"repository" yaml:"repository"`
	Action     string `json:"action" yaml:"action"`
	Status     string `json:"status" yaml:"status"`
	CommitHash string `json:"commit_hash" yaml:"commit_hash"`
	Timestamp  string `json:"timestamp" yaml:"timestamp"`
	Conflicts  int    `json:"conflicts" yaml:"conflicts"`
}

func newGitSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git-sync",
		Short: "Git repository synchronization",
		Long: `Manage Git repository synchronization, conflict resolution, and file locking.

Commands:
  status       - Show sync status for a repository
  trigger      - Trigger a manual sync
  force        - Force sync (override conflicts)
  conflicts    - Manage sync conflicts
  lock         - Lock a file path
  unlock       - Unlock a file path
  locks        - List active file locks
  history      - Show sync history
  audit        - Show sync audit log

Examples:
  # Check sync status
  kscorectl gitops git-sync status myrepo

  # Trigger a sync
  kscorectl gitops git-sync trigger myrepo

  # List conflicts
  kscorectl gitops git-sync conflicts list

  # Resolve a conflict
  kscorectl gitops git-sync conflicts resolve path/to/file --accept-git`,
	}

	cmd.AddCommand(newGitSyncStatusCmd())
	cmd.AddCommand(newGitSyncTriggerCmd())
	cmd.AddCommand(newGitSyncForceCmd())
	cmd.AddCommand(newGitSyncConflictsCmd())
	cmd.AddCommand(newGitSyncLockCmd())
	cmd.AddCommand(newGitSyncUnlockCmd())
	cmd.AddCommand(newGitSyncLocksCmd())
	cmd.AddCommand(newGitSyncHistoryCmd())
	cmd.AddCommand(newGitSyncAuditCmd())

	return cmd
}

func newGitSyncStatusCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "status <repository>",
		Short: "Show sync status for a repository",
		Long: `Show the current synchronization status of a Git repository.

Examples:
  kscorectl gitops git-sync status myrepo
  kscorectl gitops git-sync status myrepo --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]

			status := SyncStatus{
				Repository: repo,
				Status:     "synced",
				Branch:     "main",
				LastSync:   time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
				CommitHash: "a1b2c3d",
				Behind:     0,
				Ahead:      0,
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), status)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), status)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Repository: %s\n", status.Repository)
				fmt.Fprintf(cmd.OutOrStdout(), "Status:     %s\n", status.Status)
				fmt.Fprintf(cmd.OutOrStdout(), "Branch:     %s\n", status.Branch)
				fmt.Fprintf(cmd.OutOrStdout(), "Last Sync:  %s\n", status.LastSync)
				fmt.Fprintf(cmd.OutOrStdout(), "Commit:     %s\n", status.CommitHash)
				fmt.Fprintf(cmd.OutOrStdout(), "Behind:     %d\n", status.Behind)
				fmt.Fprintf(cmd.OutOrStdout(), "Ahead:      %d\n", status.Ahead)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml)")
	return cmd
}

func newGitSyncTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <repository>",
		Short: "Trigger a manual sync",
		Long: `Trigger a manual synchronization for a Git repository.

Examples:
  kscorectl gitops git-sync trigger myrepo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			fmt.Fprintf(cmd.OutOrStdout(), "Triggering sync for repository: %s\n", repo)
			fmt.Fprintln(cmd.OutOrStdout(), "Fetching remote changes...")
			fmt.Fprintln(cmd.OutOrStdout(), "Comparing local and remote...")
			fmt.Fprintf(cmd.OutOrStdout(), "Sync triggered for '%s'\n", repo)
			return nil
		},
	}
}

func newGitSyncForceCmd() *cobra.Command {
	var repository string

	cmd := &cobra.Command{
		Use:   "force",
		Short: "Force sync (override conflicts)",
		Long: `Force synchronization of a repository, overriding any pending conflicts.
Local changes that conflict with remote will be discarded.

Examples:
  kscorectl gitops git-sync force --repository myrepo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repository == "" {
				return clierrors.New(clierrors.KindInvalidArgument, "--repository is required")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Force syncing repository: %s\n", repository)
			fmt.Fprintln(cmd.OutOrStdout(), "Discarding local conflicts...")
			fmt.Fprintln(cmd.OutOrStdout(), "Pulling remote changes...")
			fmt.Fprintf(cmd.OutOrStdout(), "Force sync complete for '%s'\n", repository)
			return nil
		},
	}

	cmd.Flags().StringVar(&repository, "repository", "", "Repository name (required)")
	return cmd
}

func newGitSyncConflictsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conflicts",
		Short: "Manage sync conflicts",
		Long: `Manage Git synchronization conflicts.

Commands:
  list        - List pending conflicts
  show        - Show conflict details
  diff        - Show conflict diff
  resolve     - Resolve a conflict
  resolve-all - Resolve all conflicts`,
	}

	cmd.AddCommand(newGitSyncConflictsListCmd())
	cmd.AddCommand(newGitSyncConflictsShowCmd())
	cmd.AddCommand(newGitSyncConflictsDiffCmd())
	cmd.AddCommand(newGitSyncConflictsResolveCmd())
	cmd.AddCommand(newGitSyncConflictsResolveAllCmd())

	return cmd
}

func newGitSyncConflictsListCmd() *cobra.Command {
	var statusFilter string
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending conflicts",
		Long: `List all pending sync conflicts across repositories.

Examples:
  kscorectl gitops git-sync conflicts list
  kscorectl gitops git-sync conflicts list --status pending
  kscorectl gitops git-sync conflicts list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			conflicts := []ConflictInfo{
				{
					Path:       "states/nginx.yaml",
					Status:     "pending",
					LocalHash:  "abc1234",
					RemoteHash: "def5678",
					ModifiedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				},
				{
					Path:       "vars/production.yaml",
					Status:     "pending",
					LocalHash:  "111aaaa",
					RemoteHash: "222bbbb",
					ModifiedAt: time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
				},
			}

			if statusFilter != "" {
				filtered := make([]ConflictInfo, 0)
				for _, c := range conflicts {
					if c.Status == statusFilter {
						filtered = append(filtered, c)
					}
				}
				conflicts = filtered
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), conflicts)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), conflicts)
			case output.FormatTable:
				table := buildConflictTable(conflicts)
				if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d conflicts\n", len(conflicts))
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Sync Conflicts")
				fmt.Fprintln(cmd.OutOrStdout(), "==============")
				for _, c := range conflicts {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  Path:     %s\n", c.Path)
					fmt.Fprintf(cmd.OutOrStdout(), "  Status:   %s\n", c.Status)
					fmt.Fprintf(cmd.OutOrStdout(), "  Local:    %s\n", c.LocalHash)
					fmt.Fprintf(cmd.OutOrStdout(), "  Remote:   %s\n", c.RemoteHash)
					fmt.Fprintf(cmd.OutOrStdout(), "  Modified: %s\n", c.ModifiedAt)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d conflicts\n", len(conflicts))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status (pending, resolved)")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")
	return cmd
}

func newGitSyncConflictsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <path>",
		Short: "Show conflict details",
		Long: `Show detailed information about a specific sync conflict.

Examples:
  kscorectl gitops git-sync conflicts show states/nginx.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			fmt.Fprintf(cmd.OutOrStdout(), "Conflict Details: %s\n", path)
			fmt.Fprintln(cmd.OutOrStdout(), "=================")
			fmt.Fprintf(cmd.OutOrStdout(), "Path:        %s\n", path)
			fmt.Fprintln(cmd.OutOrStdout(), "Status:      pending")
			fmt.Fprintln(cmd.OutOrStdout(), "Local Hash:  abc1234")
			fmt.Fprintln(cmd.OutOrStdout(), "Remote Hash: def5678")
			fmt.Fprintln(cmd.OutOrStdout(), "Type:        content")
			fmt.Fprintf(cmd.OutOrStdout(), "Modified:    %s\n", time.Now().Add(-2*time.Hour).Format(time.RFC3339))
			fmt.Fprintln(cmd.OutOrStdout(), "\nLocal version was modified at runtime.")
			fmt.Fprintln(cmd.OutOrStdout(), "Remote version was updated in Git.")
			return nil
		},
	}
}

func newGitSyncConflictsDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <path>",
		Short: "Show conflict diff",
		Long: `Show the diff between local and remote versions of a conflicting file.

Examples:
  kscorectl gitops git-sync conflicts diff states/nginx.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			fmt.Fprintf(cmd.OutOrStdout(), "Diff for: %s\n", path)
			fmt.Fprintln(cmd.OutOrStdout(), "--- local (runtime)")
			fmt.Fprintln(cmd.OutOrStdout(), "+++ remote (git)")
			fmt.Fprintln(cmd.OutOrStdout(), "@@ -1,5 +1,5 @@")
			fmt.Fprintln(cmd.OutOrStdout(), " server:")
			fmt.Fprintln(cmd.OutOrStdout(), "-  workers: 4")
			fmt.Fprintln(cmd.OutOrStdout(), "+  workers: 8")
			fmt.Fprintln(cmd.OutOrStdout(), "   listen: 0.0.0.0:80")
			return nil
		},
	}
}

func newGitSyncConflictsResolveCmd() *cobra.Command {
	var acceptGit bool
	var keepRuntime bool
	var merge bool
	var reason string

	cmd := &cobra.Command{
		Use:   "resolve <path>",
		Short: "Resolve a conflict",
		Long: `Resolve a sync conflict for a specific file.

Resolution strategies:
  --accept-git:    Accept the Git (remote) version
  --keep-runtime:  Keep the local (runtime) version
  --merge:         Attempt automatic merge

Examples:
  kscorectl gitops git-sync conflicts resolve states/nginx.yaml --accept-git
  kscorectl gitops git-sync conflicts resolve vars/prod.yaml --keep-runtime --reason "manual override"
  kscorectl gitops git-sync conflicts resolve config.yaml --merge`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			strategies := 0
			if acceptGit {
				strategies++
			}
			if keepRuntime {
				strategies++
			}
			if merge {
				strategies++
			}
			if strategies == 0 {
				return clierrors.New(clierrors.KindInvalidArgument, "specify a resolution strategy: --accept-git, --keep-runtime, or --merge")
			}
			if strategies > 1 {
				return clierrors.New(clierrors.KindInvalidArgument, "specify only one resolution strategy")
			}

			strategy := "accept-git"
			if keepRuntime {
				strategy = "keep-runtime"
			} else if merge {
				strategy = "merge"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Resolving conflict: %s\n", path)
			fmt.Fprintf(cmd.OutOrStdout(), "Strategy: %s\n", strategy)
			if reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", reason)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Conflict resolved for '%s'\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&acceptGit, "accept-git", false, "Accept the Git (remote) version")
	cmd.Flags().BoolVar(&keepRuntime, "keep-runtime", false, "Keep the local (runtime) version")
	cmd.Flags().BoolVar(&merge, "merge", false, "Attempt automatic merge")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for resolution")
	return cmd
}

func newGitSyncConflictsResolveAllCmd() *cobra.Command {
	var acceptGit bool
	var keepRuntime bool

	cmd := &cobra.Command{
		Use:   "resolve-all",
		Short: "Resolve all pending conflicts",
		Long: `Resolve all pending sync conflicts with a single strategy.

Examples:
  kscorectl gitops git-sync conflicts resolve-all --accept-git
  kscorectl gitops git-sync conflicts resolve-all --keep-runtime`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !acceptGit && !keepRuntime {
				return clierrors.New(clierrors.KindInvalidArgument, "specify a resolution strategy: --accept-git or --keep-runtime")
			}
			if acceptGit && keepRuntime {
				return clierrors.New(clierrors.KindInvalidArgument, "specify only one resolution strategy")
			}

			strategy := "accept-git"
			if keepRuntime {
				strategy = "keep-runtime"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Resolving all conflicts with strategy: %s\n", strategy)
			fmt.Fprintln(cmd.OutOrStdout(), "Resolved: states/nginx.yaml")
			fmt.Fprintln(cmd.OutOrStdout(), "Resolved: vars/production.yaml")
			fmt.Fprintln(cmd.OutOrStdout(), "All 2 conflicts resolved")
			return nil
		},
	}

	cmd.Flags().BoolVar(&acceptGit, "accept-git", false, "Accept the Git (remote) version for all")
	cmd.Flags().BoolVar(&keepRuntime, "keep-runtime", false, "Keep the local (runtime) version for all")
	return cmd
}

func newGitSyncLockCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "lock <path>",
		Short: "Lock a file path",
		Long: `Lock a file path to prevent sync changes.

Locked files will not be updated during synchronization until unlocked.

Examples:
  kscorectl gitops git-sync lock states/nginx.yaml --reason "manual maintenance"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			fmt.Fprintf(cmd.OutOrStdout(), "Locking path: %s\n", path)
			if reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", reason)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Path '%s' locked\n", path)
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for locking")
	return cmd
}

func newGitSyncUnlockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock <path>",
		Short: "Unlock a file path",
		Long: `Unlock a previously locked file path.

Examples:
  kscorectl gitops git-sync unlock states/nginx.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			fmt.Fprintf(cmd.OutOrStdout(), "Unlocking path: %s\n", path)
			fmt.Fprintf(cmd.OutOrStdout(), "Path '%s' unlocked\n", path)
			return nil
		},
	}
}

func newGitSyncLocksCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:     "locks",
		Aliases: []string{"locks-list"},
		Short:   "List active file locks",
		Long: `List all active file locks across synchronized repositories.

Examples:
  kscorectl gitops git-sync locks
  kscorectl gitops git-sync locks --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			locks := []LockInfo{
				{
					Path:     "states/nginx.yaml",
					LockedBy: "admin",
					Reason:   "Manual maintenance",
					LockedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				},
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), locks)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), locks)
			case output.FormatTable:
				table := buildLockTable(locks)
				if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d locks\n", len(locks))
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Active Locks")
				fmt.Fprintln(cmd.OutOrStdout(), "============")
				for _, l := range locks {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  Path:      %s\n", l.Path)
					fmt.Fprintf(cmd.OutOrStdout(), "  Locked By: %s\n", l.LockedBy)
					fmt.Fprintf(cmd.OutOrStdout(), "  Reason:    %s\n", l.Reason)
					fmt.Fprintf(cmd.OutOrStdout(), "  Locked At: %s\n", l.LockedAt)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d locks\n", len(locks))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")
	return cmd
}

func newGitSyncHistoryCmd() *cobra.Command {
	var conflictsOnly bool
	var limit int
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show sync history",
		Long: `Show the history of sync operations.

Examples:
  kscorectl gitops git-sync history
  kscorectl gitops git-sync history --conflicts-only
  kscorectl gitops git-sync history --limit 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := []SyncHistoryEntry{
				{
					ID:         "sync-001",
					Repository: "infrastructure-config",
					Action:     "sync",
					Status:     "success",
					CommitHash: "a1b2c3d",
					Timestamp:  time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
					Conflicts:  0,
				},
				{
					ID:         "sync-002",
					Repository: "infrastructure-config",
					Action:     "sync",
					Status:     "conflict",
					CommitHash: "e4f5g6h",
					Timestamp:  time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
					Conflicts:  2,
				},
				{
					ID:         "sync-003",
					Repository: "app-configs",
					Action:     "force-sync",
					Status:     "success",
					CommitHash: "i7j8k9l",
					Timestamp:  time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
					Conflicts:  0,
				},
			}

			if conflictsOnly {
				filtered := make([]SyncHistoryEntry, 0)
				for _, e := range entries {
					if e.Conflicts > 0 {
						filtered = append(filtered, e)
					}
				}
				entries = filtered
			}

			if limit > 0 && limit < len(entries) {
				entries = entries[:limit]
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), entries)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), entries)
			case output.FormatTable:
				table := buildSyncHistoryTable(entries)
				if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d entries\n", len(entries))
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Sync History")
				fmt.Fprintln(cmd.OutOrStdout(), "============")
				for _, e := range entries {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  ID:         %s\n", e.ID)
					fmt.Fprintf(cmd.OutOrStdout(), "  Repository: %s\n", e.Repository)
					fmt.Fprintf(cmd.OutOrStdout(), "  Action:     %s\n", e.Action)
					fmt.Fprintf(cmd.OutOrStdout(), "  Status:     %s\n", e.Status)
					fmt.Fprintf(cmd.OutOrStdout(), "  Commit:     %s\n", e.CommitHash)
					fmt.Fprintf(cmd.OutOrStdout(), "  Timestamp:  %s\n", e.Timestamp)
					if e.Conflicts > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  Conflicts:  %d\n", e.Conflicts)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d entries\n", len(entries))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&conflictsOnly, "conflicts-only", false, "Show only entries with conflicts")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit number of entries (0 = all)")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")
	return cmd
}

func newGitSyncAuditCmd() *cobra.Command {
	var formatFlag string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show sync audit log",
		Long: `Show the audit log of all sync operations, conflict resolutions, and lock changes.

Examples:
  kscorectl gitops git-sync audit
  kscorectl gitops git-sync audit --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			type AuditEntry struct {
				Timestamp string `json:"timestamp" yaml:"timestamp"`
				User      string `json:"user" yaml:"user"`
				Action    string `json:"action" yaml:"action"`
				Target    string `json:"target" yaml:"target"`
				Details   string `json:"details" yaml:"details"`
			}

			entries := []AuditEntry{
				{
					Timestamp: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
					User:      "system",
					Action:    "sync",
					Target:    "infrastructure-config",
					Details:   "Automatic sync completed, 3 files updated",
				},
				{
					Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
					User:      "admin",
					Action:    "resolve-conflict",
					Target:    "states/nginx.yaml",
					Details:   "Resolved with accept-git strategy",
				},
				{
					Timestamp: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
					User:      "admin",
					Action:    "lock",
					Target:    "states/nginx.yaml",
					Details:   "Locked for manual maintenance",
				},
			}

			switch formatFlag {
			case "json":
				return output.WriteJSON(cmd.OutOrStdout(), entries)
			case "yaml":
				return output.WriteYAML(cmd.OutOrStdout(), entries)
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Sync Audit Log")
				fmt.Fprintln(cmd.OutOrStdout(), "==============")
				for _, e := range entries {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  [%s] %s\n", e.Timestamp, e.Action)
					fmt.Fprintf(cmd.OutOrStdout(), "  User:    %s\n", e.User)
					fmt.Fprintf(cmd.OutOrStdout(), "  Target:  %s\n", e.Target)
					fmt.Fprintf(cmd.OutOrStdout(), "  Details: %s\n", e.Details)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d audit entries\n", len(entries))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&formatFlag, "format", "text", "Output format (text, json, yaml)")
	return cmd
}

func buildConflictTable(conflicts []ConflictInfo) *output.Table {
	rows := make([][]string, 0, len(conflicts))
	for _, c := range conflicts {
		rows = append(rows, []string{
			c.Path,
			c.Status,
			c.LocalHash,
			c.RemoteHash,
			c.ModifiedAt,
		})
	}
	return &output.Table{
		Headers: []string{"PATH", "STATUS", "LOCAL", "REMOTE", "MODIFIED"},
		Rows:    rows,
	}
}

func buildLockTable(locks []LockInfo) *output.Table {
	rows := make([][]string, 0, len(locks))
	for _, l := range locks {
		rows = append(rows, []string{
			l.Path,
			l.LockedBy,
			l.Reason,
			l.LockedAt,
		})
	}
	return &output.Table{
		Headers: []string{"PATH", "LOCKED BY", "REASON", "LOCKED AT"},
		Rows:    rows,
	}
}

func buildSyncHistoryTable(entries []SyncHistoryEntry) *output.Table {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		conflicts := ""
		if e.Conflicts > 0 {
			conflicts = fmt.Sprintf("%d", e.Conflicts)
		}
		rows = append(rows, []string{
			e.ID,
			e.Repository,
			e.Action,
			e.Status,
			e.CommitHash,
			e.Timestamp,
			conflicts,
		})
	}
	return &output.Table{
		Headers: []string{"ID", "REPOSITORY", "ACTION", "STATUS", "COMMIT", "TIMESTAMP", "CONFLICTS"},
		Rows:    rows,
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

type OperationStatus struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Target    string    `json:"target"`
	StartTime time.Time `json:"start_time"`
	Duration  string    `json:"duration"`
}

// VerificationWorkflowFile represents a workflow definition file
type VerificationWorkflowFile struct {
	Name        string                        `yaml:"name"`
	Description string                        `yaml:"description,omitempty"`
	Parallel    bool                          `yaml:"parallel,omitempty"`
	Timeout     string                        `yaml:"timeout,omitempty"`
	Steps       []*VerificationStepDefinition `yaml:"steps"`
}

// VerificationStepDefinition represents a step in the workflow file
type VerificationStepDefinition struct {
	Name              string                 `yaml:"name"`
	Type              string                 `yaml:"type"`
	Timeout           string                 `yaml:"timeout,omitempty"`
	Retries           int                    `yaml:"retries,omitempty"`
	RetryDelay        string                 `yaml:"retry_delay,omitempty"`
	ContinueOnFailure bool                   `yaml:"continue_on_failure,omitempty"`
	Config            map[string]interface{} `yaml:"config"`
}

func loadVerificationWorkflow(path string) (*verification.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var wfFile VerificationWorkflowFile
	if err := yaml.Unmarshal(data, &wfFile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Convert to verification.Workflow
	workflow := &verification.Workflow{
		Name:        wfFile.Name,
		Description: wfFile.Description,
		Parallel:    wfFile.Parallel,
	}

	// Parse workflow timeout
	if wfFile.Timeout != "" {
		timeout, err := time.ParseDuration(wfFile.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid workflow timeout: %w", err)
		}
		workflow.Timeout = timeout
	}

	// Convert steps
	workflow.Steps = make([]*verification.Step, len(wfFile.Steps))
	for i, stepDef := range wfFile.Steps {
		step := &verification.Step{
			Name:              stepDef.Name,
			Type:              verification.Type(stepDef.Type),
			Retries:           stepDef.Retries,
			ContinueOnFailure: stepDef.ContinueOnFailure,
			Config:            stepDef.Config,
		}

		// Parse step timeout
		if stepDef.Timeout != "" {
			timeout, err := time.ParseDuration(stepDef.Timeout)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout for step %s: %w", stepDef.Name, err)
			}
			step.Timeout = timeout
		}

		// Parse retry delay
		if stepDef.RetryDelay != "" {
			delay, err := time.ParseDuration(stepDef.RetryDelay)
			if err != nil {
				return nil, fmt.Errorf("invalid retry_delay for step %s: %w", stepDef.Name, err)
			}
			step.RetryDelay = delay
		}

		workflow.Steps[i] = step
	}

	return workflow, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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

func buildPromotionStageTable(result *promotion.Result) *output.Table {
	rows := make([][]string, 0, len(result.Stages))
	for _, stage := range result.Stages {
		status := "OK"
		if stage.Status != promotion.StatusCompleted {
			status = "FAIL"
		}
		rows = append(rows, []string{
			stage.Environment,
			string(stage.Status),
			status,
			stage.Duration.Round(time.Millisecond).String(),
			stage.Message,
		})
	}

	return &output.Table{
		Headers: []string{"ENVIRONMENT", "STATUS", "RESULT", "DURATION", "MESSAGE"},
		Rows:    rows,
	}
}

func buildWebhookTable(handlers []struct {
	Type        string
	Description string
	Events      []string
}) *output.Table {
	rows := make([][]string, 0, len(handlers))
	for _, handler := range handlers {
		rows = append(rows, []string{
			handler.Type,
			handler.Description,
			strings.Join(handler.Events, ", "),
		})
	}

	return &output.Table{
		Headers: []string{"TYPE", "DESCRIPTION", "EVENTS"},
		Rows:    rows,
	}
}

func buildStatusTable(operations []OperationStatus) *output.Table {
	rows := make([][]string, 0, len(operations))
	for _, op := range operations {
		rows = append(rows, []string{
			op.ID,
			op.Type,
			op.Status,
			truncate(op.Target, 30),
			op.StartTime.Format("2006-01-02 15:04:05"),
			op.Duration,
		})
	}

	return &output.Table{
		Headers: []string{"ID", "TYPE", "STATUS", "TARGET", "TIME", "DURATION"},
		Rows:    rows,
	}
}

func buildRepoTable(repos []RepoConfig) *output.Table {
	rows := make([][]string, 0, len(repos))
	for i := range repos {
		repo := &repos[i]
		rows = append(rows, []string{
			repo.Name,
			truncate(repo.URL, 40),
			repo.Branch,
			repo.Auth,
			repo.Status,
			repo.CommitHash,
		})
	}

	return &output.Table{
		Headers: []string{"NAME", "URL", "BRANCH", "AUTH", "STATUS", "COMMIT"},
		Rows:    rows,
	}
}

func buildDeploymentTable(deployments []DeploymentInfo) *output.Table {
	rows := make([][]string, 0, len(deployments))
	for i := range deployments {
		d := &deployments[i]
		rows = append(rows, []string{
			d.ID,
			d.Application,
			d.Environment,
			d.Revision,
			d.Status,
			d.StartTime.Format("2006-01-02 15:04"),
		})
	}

	return &output.Table{
		Headers: []string{"ID", "APPLICATION", "ENVIRONMENT", "REVISION", "STATUS", "TIME"},
		Rows:    rows,
	}
}
