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

	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "http://localhost:8080", "Control plane server address")
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
	serverAddr     string
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

	client := newRESTClient(serverAddr)
	result, err := client.TriggerRollback(rollbackRequest{
		Application:      config.Application,
		Namespace:        config.Namespace,
		Type:             string(config.Type),
		Strategy:         string(config.Strategy),
		Revision:         config.Revision,
		Reason:           request.Reason,
		RequestedBy:      request.RequestedBy,
		SkipVerification: request.SkipVerification,
	})
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), result)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), result)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"ID", result.ID},
			{"STATUS", result.Status},
			{"FROM", result.PreviousRevision},
			{"TO", result.CurrentRevision},
			{"DURATION", result.Duration},
			{"MESSAGE", result.Message},
		})
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		if result.Error != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nError: %s\n", result.Error)
		}
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "=== Rollback Result ===")
		fmt.Fprintf(cmd.OutOrStdout(), "ID:       %s\n", result.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "From:     %s\n", result.PreviousRevision)
		fmt.Fprintf(cmd.OutOrStdout(), "To:       %s\n", result.CurrentRevision)
		fmt.Fprintf(cmd.OutOrStdout(), "Duration: %s\n", result.Duration)
		fmt.Fprintf(cmd.OutOrStdout(), "Message:  %s\n", result.Message)
		if result.Error != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Error:    %s\n", result.Error)
		}
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

	return fmt.Errorf("GitOps promotion API not yet available — server-side promotion endpoints required")
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

func statusExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps status API not yet available — server-side operation tracking endpoints required")
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


func repoListExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps repository management API not yet available — server-side repository endpoints required")
}

func repoAddExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps repository management API not yet available — server-side repository endpoints required")
}

func repoRemoveExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps repository management API not yet available — server-side repository endpoints required")
}

func repoSyncExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps repository sync API not yet available — server-side sync endpoints required")
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


func deployListExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps deployment tracking API not yet available — server-side deployment endpoints required")
}

func deployShowExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps deployment tracking API not yet available — server-side deployment endpoints required")
}

func deployRollbackExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps deployment tracking API not yet available — server-side deployment endpoints required")
}

func deployApproveExecute(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("GitOps deployment tracking API not yet available — server-side deployment endpoints required")
}

// =============================================================================
// Git Sync Command
// =============================================================================


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
	return &cobra.Command{
		Use:   "status <repository>",
		Short: "Show sync status for a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps sync status API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <repository>",
		Short: "Trigger a manual sync",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps sync trigger API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncForceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "force",
		Short: "Force sync (override conflicts)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps force sync API not yet available — server-side sync endpoints required")
		},
	}
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
	return &cobra.Command{
		Use:   "list",
		Short: "List pending conflicts",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps conflict management API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncConflictsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <path>",
		Short: "Show conflict details",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps conflict management API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncConflictsDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <path>",
		Short: "Show conflict diff",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps conflict management API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncConflictsResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <path>",
		Short: "Resolve a conflict",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps conflict management API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncConflictsResolveAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve-all",
		Short: "Resolve all pending conflicts",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps conflict management API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock <path>",
		Short: "Lock a file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps file locking API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncUnlockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock <path>",
		Short: "Unlock a file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps file locking API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncLocksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "locks",
		Short: "List active file locks",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps file locking API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show sync history",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps sync history API not yet available — server-side sync endpoints required")
		},
	}
}

func newGitSyncAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Short: "Show sync audit log",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("GitOps sync audit API not yet available — server-side sync endpoints required")
		},
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

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

