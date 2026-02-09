// Package main implements the kscore-secrets CLI for secrets management operations.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/secrets"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	verbose      bool
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-secrets",
		Short: "Keystone Core secrets management",
		Long: `kscore-secrets is a CLI plugin for managing secrets and secret rotation
in Keystone Core.

This plugin provides commands for:
  - Managing secret rotation schedules
  - Starting and monitoring rotations
  - Viewing rotation history and status
  - Manual rollback of failed rotations
  - Configuring rotation policies

Usage via kscorectl:
  kscorectl secrets rotate list
  kscorectl secrets rotate start --secret vault/secret/db --strategy blue-green
  kscorectl secrets rotate status rot-123`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newVersionCmd(),
		newRotateCmd(),
		newScheduleCmd(),
		newPolicyCmd(),
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

// =============================================================================
// Rotate Commands
// =============================================================================

func newRotateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate",
		Aliases: []string{"rot", "r"},
		Short:   "Manage secret rotations",
		Long:    `Commands for managing secret rotations in Keystone Core.`,
	}

	cmd.AddCommand(
		newRotateListCmd(),
		newRotateShowCmd(),
		newRotateStartCmd(),
		newRotateStatusCmd(),
		newRotateHistoryCmd(),
		newRotateTriggerCmd(),
		newRotateRollbackCmd(),
		newRotatePauseCmd(),
		newRotateResumeCmd(),
		newRotateCancelCmd(),
	)

	return cmd
}

// RotateListOptions holds rotate list options
type RotateListOptions struct {
	State    string
	Strategy string
	Labels   []string
	Limit    int
}

func newRotateListCmd() *cobra.Command {
	opts := &RotateListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotations",
		Long: `List secret rotations with optional filtering.

Examples:
  # List all rotations
  kscorectl secrets rotate list

  # List only in-progress rotations
  kscorectl secrets rotate list --state in_progress

  # List blue-green strategy rotations
  kscorectl secrets rotate list --strategy blue-green`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateList(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.State, "state", "", "Filter by state (pending, in_progress, completed, failed, rolled_back)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "", "Filter by strategy (rolling, blue-green, canary)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of rotations to show")

	return cmd
}

func runRotateList(cmd *cobra.Command, opts *RotateListOptions) error {
	rotations := generateSampleRotations()

	var filtered []*rotationDisplay
	for _, r := range rotations {
		if opts.State != "" && string(r.State) != opts.State {
			continue
		}
		if opts.Strategy != "" && string(r.Strategy) != opts.Strategy {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) == 0 {
		fmt.Println("No rotations found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(filtered)
	case "yaml":
		return outputYAML(filtered)
	default:
		table := &output.Table{
			Headers: []string{"ID", "SECRET PATH", "STRATEGY", "STATE", "PROGRESS", "STARTED"},
		}
		for _, r := range filtered {
			progress := fmt.Sprintf("%d/%d", r.UpdatedTargets, r.TotalTargets)
			if r.TotalTargets > 0 {
				progress = fmt.Sprintf("%s (%d%%)", progress, r.UpdatedTargets*100/r.TotalTargets)
			}
			table.Rows = append(table.Rows, []string{
				truncate(r.ID, 12),
				truncate(r.SecretPath, 25),
				string(r.Strategy),
				string(r.State),
				progress,
				r.StartedAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d rotation(s)\n", len(filtered))
	}

	return nil
}

func newRotateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <rotation-id>",
		Short: "Show rotation details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateShow(cmd, args[0])
		},
	}
}

func runRotateShow(cmd *cobra.Command, id string) error {
	r := &rotationDetail{
		ID:              id,
		SecretPath:      "vault/secret/database/prod",
		Strategy:        secrets.RotationStrategyBlueGreen,
		State:           secrets.RotationStateInProgress,
		TotalTargets:    10,
		UpdatedTargets:  6,
		FailedTargets:   0,
		Percentage:      60,
		StartedAt:       time.Now().Add(-15 * time.Minute).Format(time.RFC3339),
		BatchSize:       2,
		BatchDelay:      "30s",
		HealthCheckType: "http",
		HealthCheckURL:  "http://app:8080/health",
		CreatedBy:       "admin",
		Labels: map[string]string{
			"env":  "prod",
			"team": "platform",
		},
	}

	switch outputFormat {
	case "json":
		return outputJSON(r)
	case "yaml":
		return outputYAML(r)
	default:
		fmt.Printf("Rotation: %s\n", r.ID)
		fmt.Printf("  Secret Path:      %s\n", r.SecretPath)
		fmt.Printf("  Strategy:         %s\n", r.Strategy)
		fmt.Printf("  State:            %s\n", r.State)
		fmt.Printf("  Progress:         %d/%d targets (%d%%)\n", r.UpdatedTargets, r.TotalTargets, r.Percentage)
		fmt.Printf("  Failed Targets:   %d\n", r.FailedTargets)
		fmt.Printf("  Batch Size:       %d\n", r.BatchSize)
		fmt.Printf("  Batch Delay:      %s\n", r.BatchDelay)
		fmt.Printf("  Health Check:     %s (%s)\n", r.HealthCheckType, r.HealthCheckURL)
		fmt.Printf("  Started:          %s\n", r.StartedAt)
		fmt.Printf("  Created By:       %s\n", r.CreatedBy)
		if len(r.Labels) > 0 {
			fmt.Printf("  Labels:\n")
			for k, v := range r.Labels {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}

	return nil
}

// RotateStartOptions holds options for starting a rotation
type RotateStartOptions struct {
	SecretPath       string
	Strategy         string
	Targets          []string
	TargetTags       []string
	TargetRoles      []string
	BatchSize        int
	BatchDelay       string
	CanaryPercentage int
	CanaryDelay      string
	HealthCheckType  string
	HealthCheckURL   string
	HealthCheckPort  int
	Timeout          string
	DryRun           bool
	Labels           []string
}

func newRotateStartCmd() *cobra.Command {
	opts := &RotateStartOptions{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new secret rotation",
		Long: `Start a new secret rotation.

Examples:
  # Start a blue-green rotation for all targets
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy blue-green --target-all

  # Start a canary rotation with 10% canary
  kscorectl secrets rotate start --secret vault/secret/api \
    --strategy canary --canary-percentage 10 --canary-delay 5m

  # Start with health checks
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy rolling --batch-size 2 \
    --health-check-type http --health-check-url http://app:8080/health

  # Dry run to see what would happen
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy blue-green --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateStart(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.SecretPath, "secret", "", "Secret path to rotate (required)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Rotation strategy (rolling, blue-green, canary)")
	cmd.Flags().StringArrayVar(&opts.Targets, "target", nil, "Target agent IDs")
	cmd.Flags().StringArrayVar(&opts.TargetTags, "target-tags", nil, "Target agents with tags (key:value)")
	cmd.Flags().StringArrayVar(&opts.TargetRoles, "target-roles", nil, "Target agents with roles")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Number of targets per batch")
	cmd.Flags().StringVar(&opts.BatchDelay, "batch-delay", "30s", "Delay between batches")
	cmd.Flags().IntVar(&opts.CanaryPercentage, "canary-percentage", 10, "Percentage of targets for canary (canary strategy)")
	cmd.Flags().StringVar(&opts.CanaryDelay, "canary-delay", "5m", "Delay after canary verification (canary strategy)")
	cmd.Flags().StringVar(&opts.HealthCheckType, "health-check-type", "", "Health check type (http, tcp, exec)")
	cmd.Flags().StringVar(&opts.HealthCheckURL, "health-check-url", "", "Health check URL (for http type)")
	cmd.Flags().IntVar(&opts.HealthCheckPort, "health-check-port", 0, "Health check port (for tcp type)")
	cmd.Flags().StringVar(&opts.Timeout, "timeout", "30m", "Overall rotation timeout")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be done without executing")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels (key:value format)")

	_ = cmd.MarkFlagRequired("secret")

	return cmd
}

func runRotateStart(cmd *cobra.Command, opts *RotateStartOptions) error {
	if opts.SecretPath == "" {
		return fmt.Errorf("--secret is required")
	}

	if len(opts.Targets) == 0 && len(opts.TargetTags) == 0 && len(opts.TargetRoles) == 0 {
		return fmt.Errorf("at least one target option is required (--target, --target-tags, or --target-roles)")
	}

	strategy := normalizeStrategy(opts.Strategy)
	if strategy != secrets.RotationStrategyRolling &&
		strategy != secrets.RotationStrategyBlueGreen &&
		strategy != secrets.RotationStrategyCanary {
		return fmt.Errorf("invalid strategy: %s (must be rolling, blue-green, or canary)", opts.Strategy)
	}

	if opts.DryRun {
		fmt.Println("Dry run mode - no changes will be made")
		fmt.Println()
		fmt.Printf("Would start rotation:\n")
		fmt.Printf("  Secret:           %s\n", opts.SecretPath)
		fmt.Printf("  Strategy:         %s\n", opts.Strategy)
		fmt.Printf("  Batch Size:       %d\n", opts.BatchSize)
		fmt.Printf("  Batch Delay:      %s\n", opts.BatchDelay)
		if strategy == secrets.RotationStrategyCanary {
			fmt.Printf("  Canary %%:         %d%%\n", opts.CanaryPercentage)
			fmt.Printf("  Canary Delay:     %s\n", opts.CanaryDelay)
		}
		if opts.HealthCheckType != "" {
			fmt.Printf("  Health Check:     %s\n", opts.HealthCheckType)
		}
		fmt.Printf("  Timeout:          %s\n", opts.Timeout)
		fmt.Println()
		fmt.Println("Targets that would be updated:")
		for _, t := range opts.Targets {
			fmt.Printf("  - %s\n", t)
		}
		for _, t := range opts.TargetTags {
			fmt.Printf("  - agents with tag %s\n", t)
		}
		for _, r := range opts.TargetRoles {
			fmt.Printf("  - agents with role %s\n", r)
		}
		return nil
	}

	// In production, this would call the API
	rotationID := fmt.Sprintf("rot-%s", randomID(8))
	fmt.Printf("Started rotation '%s' for secret '%s'\n", rotationID, opts.SecretPath)
	fmt.Printf("  Strategy:     %s\n", opts.Strategy)
	fmt.Printf("  Use 'kscorectl secrets rotate status %s' to monitor progress\n", rotationID)

	return nil
}

func newRotateStatusCmd() *cobra.Command {
	var watch bool
	var interval string

	cmd := &cobra.Command{
		Use:   "status <rotation-id>",
		Short: "Show rotation status",
		Args:  cobra.ExactArgs(1),
		Long: `Show the current status of a rotation.

Examples:
  # Show status once
  kscorectl secrets rotate status rot-123

  # Watch status continuously
  kscorectl secrets rotate status rot-123 --watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateStatus(cmd, args[0], watch, interval)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch status continuously")
	cmd.Flags().StringVar(&interval, "interval", "2s", "Watch interval")

	return cmd
}

func runRotateStatus(cmd *cobra.Command, id string, watch bool, interval string) error {
	// Sample status for demonstration
	status := &rotationStatus{
		ID:             id,
		State:          secrets.RotationStateInProgress,
		TotalTargets:   10,
		UpdatedTargets: 6,
		FailedTargets:  0,
		Percentage:     60,
		CurrentBatch:   4,
		TotalBatches:   5,
		StartedAt:      time.Now().Add(-15 * time.Minute).Format(time.RFC3339),
		LastUpdate:     time.Now().Add(-30 * time.Second).Format(time.RFC3339),
	}

	switch outputFormat {
	case "json":
		return outputJSON(status)
	case "yaml":
		return outputYAML(status)
	default:
		printRotationStatus(status)
	}

	return nil
}

func printRotationStatus(status *rotationStatus) {
	stateIcon := "⏳"
	switch status.State {
	case secrets.RotationStateCompleted:
		stateIcon = "✅"
	case secrets.RotationStateFailed:
		stateIcon = "❌"
	case secrets.RotationStateRolledBack:
		stateIcon = "⏪"
	default:
	}

	fmt.Printf("%s Rotation %s: %s\n", stateIcon, status.ID, status.State)
	fmt.Printf("  Progress:     %d/%d targets (%d%%)\n", status.UpdatedTargets, status.TotalTargets, status.Percentage)
	fmt.Printf("  Batch:        %d/%d\n", status.CurrentBatch, status.TotalBatches)
	fmt.Printf("  Failed:       %d\n", status.FailedTargets)
	fmt.Printf("  Started:      %s\n", status.StartedAt)
	fmt.Printf("  Last Update:  %s\n", status.LastUpdate)

	// Progress bar
	barWidth := 40
	filled := int(float64(status.Percentage) / 100.0 * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Printf("  [%s]\n", bar)
}

// HistoryOptions holds history command options
type HistoryOptions struct {
	Limit  int
	Status string
}

func newRotateHistoryCmd() *cobra.Command {
	opts := &HistoryOptions{}

	cmd := &cobra.Command{
		Use:   "history [secret-path]",
		Short: "Show rotation history",
		Long: `Show rotation history for a secret or all secrets.

Examples:
  # Show all rotation history
  kscorectl secrets rotate history

  # Show history for specific secret
  kscorectl secrets rotate history vault/secret/db

  # Show only failed rotations
  kscorectl secrets rotate history --status failed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			secretPath := ""
			if len(args) > 0 {
				secretPath = args[0]
			}
			return runRotateHistory(cmd, secretPath, opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 20, "Number of rotations to show")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status")

	return cmd
}

func runRotateHistory(cmd *cobra.Command, secretPath string, opts *HistoryOptions) error {
	history := generateSampleHistory(secretPath, opts.Limit)

	switch outputFormat {
	case "json":
		return outputJSON(history)
	case "yaml":
		return outputYAML(history)
	default:
		table := &output.Table{
			Headers: []string{"ID", "SECRET PATH", "STRATEGY", "STATE", "TARGETS", "STARTED", "DURATION"},
		}
		for _, h := range history {
			targets := fmt.Sprintf("%d/%d", h.UpdatedTargets, h.TotalTargets)
			if h.FailedTargets > 0 {
				targets = fmt.Sprintf("%s (%d failed)", targets, h.FailedTargets)
			}
			table.Rows = append(table.Rows, []string{
				truncate(h.ID, 12),
				truncate(h.SecretPath, 20),
				string(h.Strategy),
				string(h.State),
				targets,
				h.StartedAt,
				h.Duration,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nShowing %d rotation(s)\n", len(history))
	}

	return nil
}

func newRotateTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <schedule-id>",
		Short: "Trigger a scheduled rotation immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rotationID := fmt.Sprintf("rot-%s", randomID(8))
			fmt.Printf("Triggered scheduled rotation %s (rotation: %s)\n", args[0], rotationID)
			return nil
		},
	}
}

func newRotateRollbackCmd() *cobra.Command {
	var force bool
	var reason string

	cmd := &cobra.Command{
		Use:   "rollback <rotation-id>",
		Short: "Rollback a rotation",
		Long: `Manually rollback a rotation to the previous secret version.

Examples:
  # Rollback a rotation
  kscorectl secrets rotate rollback rot-123 --reason "health check failures"

  # Force rollback without confirmation
  kscorectl secrets rotate rollback rot-123 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to rollback rotation %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Rolling back rotation %s...\n", args[0])
			fmt.Printf("  Reason: %s\n", reason)
			fmt.Printf("Rollback initiated. Use 'kscorectl secrets rotate status %s' to monitor.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force rollback without confirmation")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for rollback")

	return cmd
}

func newRotatePauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <rotation-id>",
		Short: "Pause an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Paused rotation %s\n", args[0])
			return nil
		},
	}
}

func newRotateResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <rotation-id>",
		Short: "Resume a paused rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Resumed rotation %s\n", args[0])
			return nil
		},
	}
}

func newRotateCancelCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "cancel <rotation-id>",
		Short: "Cancel an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Cancelled rotation %s\n", args[0])
			if reason != "" {
				fmt.Printf("  Reason: %s\n", reason)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")

	return cmd
}

// =============================================================================
// Schedule Commands
// =============================================================================

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"sched", "s"},
		Short:   "Manage rotation schedules",
		Long:    `Commands for managing rotation schedules.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(),
		newScheduleShowCmd(),
		newScheduleCreateCmd(),
		newScheduleEnableCmd(),
		newScheduleDisableCmd(),
		newScheduleDeleteCmd(),
	)

	return cmd
}

func newScheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			schedules := generateSampleSchedules()

			switch outputFormat {
			case "json":
				return outputJSON(schedules)
			case "yaml":
				return outputYAML(schedules)
			default:
				table := &output.Table{
					Headers: []string{"ID", "SECRET PATH", "SCHEDULE", "STRATEGY", "ENABLED", "NEXT RUN"},
				}
				for _, s := range schedules {
					enabled := "No"
					if s.Enabled {
						enabled = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(s.ID, 12),
						truncate(s.SecretPath, 20),
						s.Schedule,
						string(s.Strategy),
						enabled,
						s.NextRun,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newScheduleShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <schedule-id>",
		Short: "Show schedule details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := &scheduleDetail{
				ID:         args[0],
				SecretPath: "vault/secret/database/prod",
				Schedule:   "0 2 * * *",
				Strategy:   secrets.RotationStrategyBlueGreen,
				Enabled:    true,
				NextRun:    time.Now().Add(12 * time.Hour).Format(time.RFC3339),
				LastRun:    time.Now().Add(-12 * time.Hour).Format(time.RFC3339),
				RunCount:   30,
				CreatedBy:  "admin",
				CreatedAt:  time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			}

			switch outputFormat {
			case "json":
				return outputJSON(s)
			case "yaml":
				return outputYAML(s)
			default:
				fmt.Printf("Schedule: %s\n", s.ID)
				fmt.Printf("  Secret Path:  %s\n", s.SecretPath)
				fmt.Printf("  Schedule:     %s\n", s.Schedule)
				fmt.Printf("  Strategy:     %s\n", s.Strategy)
				fmt.Printf("  Enabled:      %v\n", s.Enabled)
				fmt.Printf("  Next Run:     %s\n", s.NextRun)
				fmt.Printf("  Last Run:     %s\n", s.LastRun)
				fmt.Printf("  Run Count:    %d\n", s.RunCount)
				fmt.Printf("  Created:      %s by %s\n", s.CreatedAt, s.CreatedBy)
			}
			return nil
		},
	}
}

// ScheduleCreateOptions holds schedule creation options
type ScheduleCreateOptions struct {
	SecretPath       string
	Schedule         string
	Strategy         string
	Targets          []string
	TargetTags       []string
	BatchSize        int
	BatchDelay       string
	CanaryPercentage int
	HealthCheckType  string
	HealthCheckURL   string
	Enabled          bool
	Labels           []string
}

func newScheduleCreateCmd() *cobra.Command {
	opts := &ScheduleCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a rotation schedule",
		Long: `Create a new rotation schedule.

Examples:
  # Create daily rotation at 2am
  kscorectl secrets schedule create --secret vault/secret/db \
    --schedule "0 2 * * *" --strategy blue-green --target-tags env:prod

  # Create weekly rotation
  kscorectl secrets schedule create --secret vault/secret/api \
    --schedule "0 3 * * 0" --strategy canary --canary-percentage 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.SecretPath, "secret", "", "Secret path (required)")
	cmd.Flags().StringVar(&opts.Schedule, "schedule", "", "Cron schedule (required)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Rotation strategy")
	cmd.Flags().StringArrayVar(&opts.Targets, "target", nil, "Target agent IDs")
	cmd.Flags().StringArrayVar(&opts.TargetTags, "target-tags", nil, "Target tags")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Batch size")
	cmd.Flags().StringVar(&opts.BatchDelay, "batch-delay", "30s", "Batch delay")
	cmd.Flags().IntVar(&opts.CanaryPercentage, "canary-percentage", 10, "Canary percentage")
	cmd.Flags().StringVar(&opts.HealthCheckType, "health-check-type", "", "Health check type")
	cmd.Flags().StringVar(&opts.HealthCheckURL, "health-check-url", "", "Health check URL")
	cmd.Flags().BoolVar(&opts.Enabled, "enabled", true, "Enable schedule")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels")

	_ = cmd.MarkFlagRequired("secret")
	_ = cmd.MarkFlagRequired("schedule")

	return cmd
}

func runScheduleCreate(cmd *cobra.Command, opts *ScheduleCreateOptions) error {
	if opts.SecretPath == "" {
		return fmt.Errorf("--secret is required")
	}
	if opts.Schedule == "" {
		return fmt.Errorf("--schedule is required")
	}

	scheduleID := fmt.Sprintf("sched-%s", randomID(8))
	fmt.Printf("Created rotation schedule '%s' for secret '%s'\n", scheduleID, opts.SecretPath)
	fmt.Printf("  Schedule: %s\n", opts.Schedule)
	fmt.Printf("  Strategy: %s\n", opts.Strategy)
	return nil
}

func newScheduleEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <schedule-id>",
		Short: "Enable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Enabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <schedule-id>",
		Short: "Disable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Disabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <schedule-id>",
		Short: "Delete a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete schedule %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Deleted schedule %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Policy Commands
// =============================================================================

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "policy",
		Aliases: []string{"pol", "p"},
		Short:   "Manage rotation policies",
		Long:    `Commands for managing rotation policies.`,
	}

	cmd.AddCommand(
		newPolicyListCmd(),
		newPolicyShowCmd(),
		newPolicyCreateCmd(),
		newPolicyDeleteCmd(),
	)

	return cmd
}

func newPolicyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			policies := generateSamplePolicies()

			switch outputFormat {
			case "json":
				return outputJSON(policies)
			case "yaml":
				return outputYAML(policies)
			default:
				table := &output.Table{
					Headers: []string{"ID", "NAME", "SECRET PATTERN", "MAX AGE", "STRATEGY", "ENABLED"},
				}
				for _, p := range policies {
					enabled := "No"
					if p.Enabled {
						enabled = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(p.ID, 12),
						truncate(p.Name, 20),
						p.SecretPattern,
						p.MaxAge,
						string(p.Strategy),
						enabled,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newPolicyShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <policy-id>",
		Short: "Show policy details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := &policyDetail{
				ID:             args[0],
				Name:           "database-rotation-policy",
				SecretPattern:  "vault/secret/database/*",
				MaxAge:         "90d",
				Strategy:       secrets.RotationStrategyBlueGreen,
				BatchSize:      2,
				HealthRequired: true,
				Enabled:        true,
				CreatedBy:      "admin",
				CreatedAt:      time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			}

			switch outputFormat {
			case "json":
				return outputJSON(p)
			case "yaml":
				return outputYAML(p)
			default:
				fmt.Printf("Policy: %s\n", p.Name)
				fmt.Printf("  ID:              %s\n", p.ID)
				fmt.Printf("  Secret Pattern:  %s\n", p.SecretPattern)
				fmt.Printf("  Max Age:         %s\n", p.MaxAge)
				fmt.Printf("  Strategy:        %s\n", p.Strategy)
				fmt.Printf("  Batch Size:      %d\n", p.BatchSize)
				fmt.Printf("  Health Required: %v\n", p.HealthRequired)
				fmt.Printf("  Enabled:         %v\n", p.Enabled)
				fmt.Printf("  Created:         %s by %s\n", p.CreatedAt, p.CreatedBy)
			}
			return nil
		},
	}
}

// PolicyCreateOptions holds policy creation options
type PolicyCreateOptions struct {
	Name           string
	SecretPattern  string
	MaxAge         string
	Strategy       string
	BatchSize      int
	HealthRequired bool
	Enabled        bool
}

func newPolicyCreateCmd() *cobra.Command {
	opts := &PolicyCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a rotation policy",
		Long: `Create a new rotation policy.

Examples:
  # Create a policy for database secrets
  kscorectl secrets policy create --name db-policy \
    --pattern "vault/secret/database/*" --max-age 90d --strategy blue-green

  # Create a strict policy requiring health checks
  kscorectl secrets policy create --name api-policy \
    --pattern "vault/secret/api/*" --max-age 30d --health-required`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Policy name (required)")
	cmd.Flags().StringVar(&opts.SecretPattern, "pattern", "", "Secret path pattern (required)")
	cmd.Flags().StringVar(&opts.MaxAge, "max-age", "90d", "Maximum secret age before rotation")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Default rotation strategy")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Default batch size")
	cmd.Flags().BoolVar(&opts.HealthRequired, "health-required", false, "Require health checks")
	cmd.Flags().BoolVar(&opts.Enabled, "enabled", true, "Enable policy")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("pattern")

	return cmd
}

func runPolicyCreate(cmd *cobra.Command, opts *PolicyCreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if opts.SecretPattern == "" {
		return fmt.Errorf("--pattern is required")
	}

	policyID := fmt.Sprintf("pol-%s", randomID(8))
	fmt.Printf("Created rotation policy '%s' (id: %s)\n", opts.Name, policyID)
	fmt.Printf("  Pattern:  %s\n", opts.SecretPattern)
	fmt.Printf("  Max Age:  %s\n", opts.MaxAge)
	fmt.Printf("  Strategy: %s\n", opts.Strategy)
	return nil
}

func newPolicyDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <policy-id>",
		Short: "Delete a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete policy %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Deleted policy %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Display Types
// =============================================================================

type rotationDisplay struct {
	ID             string                   `json:"id" yaml:"id"`
	SecretPath     string                   `json:"secret_path" yaml:"secret_path"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State          secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets   int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                      `json:"updated_targets" yaml:"updated_targets"`
	StartedAt      string                   `json:"started_at" yaml:"started_at"`
}

type rotationDetail struct {
	ID              string                   `json:"id" yaml:"id"`
	SecretPath      string                   `json:"secret_path" yaml:"secret_path"`
	Strategy        secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State           secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets    int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets  int                      `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets   int                      `json:"failed_targets" yaml:"failed_targets"`
	Percentage      int                      `json:"percentage" yaml:"percentage"`
	StartedAt       string                   `json:"started_at" yaml:"started_at"`
	BatchSize       int                      `json:"batch_size" yaml:"batch_size"`
	BatchDelay      string                   `json:"batch_delay" yaml:"batch_delay"`
	HealthCheckType string                   `json:"health_check_type,omitempty" yaml:"health_check_type,omitempty"`
	HealthCheckURL  string                   `json:"health_check_url,omitempty" yaml:"health_check_url,omitempty"`
	CreatedBy       string                   `json:"created_by" yaml:"created_by"`
	Labels          map[string]string        `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type rotationStatus struct {
	ID             string                `json:"id" yaml:"id"`
	State          secrets.RotationState `json:"state" yaml:"state"`
	TotalTargets   int                   `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                   `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets  int                   `json:"failed_targets" yaml:"failed_targets"`
	Percentage     int                   `json:"percentage" yaml:"percentage"`
	CurrentBatch   int                   `json:"current_batch" yaml:"current_batch"`
	TotalBatches   int                   `json:"total_batches" yaml:"total_batches"`
	StartedAt      string                `json:"started_at" yaml:"started_at"`
	LastUpdate     string                `json:"last_update" yaml:"last_update"`
}

type rotationHistory struct {
	ID             string                   `json:"id" yaml:"id"`
	SecretPath     string                   `json:"secret_path" yaml:"secret_path"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State          secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets   int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                      `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets  int                      `json:"failed_targets" yaml:"failed_targets"`
	StartedAt      string                   `json:"started_at" yaml:"started_at"`
	Duration       string                   `json:"duration" yaml:"duration"`
}

type scheduleDisplay struct {
	ID         string                   `json:"id" yaml:"id"`
	SecretPath string                   `json:"secret_path" yaml:"secret_path"`
	Schedule   string                   `json:"schedule" yaml:"schedule"`
	Strategy   secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled    bool                     `json:"enabled" yaml:"enabled"`
	NextRun    string                   `json:"next_run" yaml:"next_run"`
}

type scheduleDetail struct {
	ID         string                   `json:"id" yaml:"id"`
	SecretPath string                   `json:"secret_path" yaml:"secret_path"`
	Schedule   string                   `json:"schedule" yaml:"schedule"`
	Strategy   secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled    bool                     `json:"enabled" yaml:"enabled"`
	NextRun    string                   `json:"next_run" yaml:"next_run"`
	LastRun    string                   `json:"last_run" yaml:"last_run"`
	RunCount   int                      `json:"run_count" yaml:"run_count"`
	CreatedBy  string                   `json:"created_by" yaml:"created_by"`
	CreatedAt  string                   `json:"created_at" yaml:"created_at"`
}

type policyDisplay struct {
	ID            string                   `json:"id" yaml:"id"`
	Name          string                   `json:"name" yaml:"name"`
	SecretPattern string                   `json:"secret_pattern" yaml:"secret_pattern"`
	MaxAge        string                   `json:"max_age" yaml:"max_age"`
	Strategy      secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled       bool                     `json:"enabled" yaml:"enabled"`
}

type policyDetail struct {
	ID             string                   `json:"id" yaml:"id"`
	Name           string                   `json:"name" yaml:"name"`
	SecretPattern  string                   `json:"secret_pattern" yaml:"secret_pattern"`
	MaxAge         string                   `json:"max_age" yaml:"max_age"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	BatchSize      int                      `json:"batch_size" yaml:"batch_size"`
	HealthRequired bool                     `json:"health_required" yaml:"health_required"`
	Enabled        bool                     `json:"enabled" yaml:"enabled"`
	CreatedBy      string                   `json:"created_by" yaml:"created_by"`
	CreatedAt      string                   `json:"created_at" yaml:"created_at"`
}

// =============================================================================
// Helpers
// =============================================================================

func generateSampleRotations() []*rotationDisplay {
	return []*rotationDisplay{
		{
			ID:             "rot-abc123",
			SecretPath:     "vault/secret/database/prod",
			Strategy:       secrets.RotationStrategyBlueGreen,
			State:          secrets.RotationStateInProgress,
			TotalTargets:   10,
			UpdatedTargets: 6,
			StartedAt:      time.Now().Add(-15 * time.Minute).Format("15:04"),
		},
		{
			ID:             "rot-def456",
			SecretPath:     "vault/secret/api/staging",
			Strategy:       secrets.RotationStrategyCanary,
			State:          secrets.RotationStateCompleted,
			TotalTargets:   5,
			UpdatedTargets: 5,
			StartedAt:      time.Now().Add(-2 * time.Hour).Format("15:04"),
		},
		{
			ID:             "rot-ghi789",
			SecretPath:     "vault/secret/cache/prod",
			Strategy:       secrets.RotationStrategyRolling,
			State:          secrets.RotationStateFailed,
			TotalTargets:   8,
			UpdatedTargets: 3,
			StartedAt:      time.Now().Add(-1 * time.Hour).Format("15:04"),
		},
	}
}

func generateSampleHistory(secretPath string, limit int) []*rotationHistory {
	history := make([]*rotationHistory, 0, limit)
	states := []secrets.RotationState{
		secrets.RotationStateCompleted,
		secrets.RotationStateCompleted,
		secrets.RotationStateFailed,
		secrets.RotationStateCompleted,
		secrets.RotationStateRolledBack,
	}

	path := "vault/secret/database/prod"
	if secretPath != "" {
		path = secretPath
	}

	for i := 0; i < limit && i < 5; i++ {
		h := &rotationHistory{
			ID:             fmt.Sprintf("rot-%03d", i+1),
			SecretPath:     path,
			Strategy:       secrets.RotationStrategyBlueGreen,
			State:          states[i%len(states)],
			TotalTargets:   10,
			UpdatedTargets: 10,
			StartedAt:      time.Now().Add(-time.Duration(i*24) * time.Hour).Format("Jan 02 15:04"),
			Duration:       fmt.Sprintf("%dm%ds", 5+i, 30+i*10),
		}
		if h.State == secrets.RotationStateFailed {
			h.UpdatedTargets = 3
			h.FailedTargets = 2
		}
		history = append(history, h)
	}

	return history
}

func generateSampleSchedules() []*scheduleDisplay {
	return []*scheduleDisplay{
		{
			ID:         "sched-001",
			SecretPath: "vault/secret/database/*",
			Schedule:   "0 2 * * *",
			Strategy:   secrets.RotationStrategyBlueGreen,
			Enabled:    true,
			NextRun:    time.Now().Add(12 * time.Hour).Format("Jan 02 15:04"),
		},
		{
			ID:         "sched-002",
			SecretPath: "vault/secret/api/*",
			Schedule:   "0 3 * * 0",
			Strategy:   secrets.RotationStrategyCanary,
			Enabled:    true,
			NextRun:    time.Now().Add(5 * 24 * time.Hour).Format("Jan 02 15:04"),
		},
		{
			ID:         "sched-003",
			SecretPath: "vault/secret/cache/*",
			Schedule:   "0 0 1 * *",
			Strategy:   secrets.RotationStrategyRolling,
			Enabled:    false,
			NextRun:    "",
		},
	}
}

func generateSamplePolicies() []*policyDisplay {
	return []*policyDisplay{
		{
			ID:            "pol-001",
			Name:          "database-rotation",
			SecretPattern: "vault/secret/database/*",
			MaxAge:        "90d",
			Strategy:      secrets.RotationStrategyBlueGreen,
			Enabled:       true,
		},
		{
			ID:            "pol-002",
			Name:          "api-rotation",
			SecretPattern: "vault/secret/api/*",
			MaxAge:        "30d",
			Strategy:      secrets.RotationStrategyCanary,
			Enabled:       true,
		},
		{
			ID:            "pol-003",
			Name:          "cache-rotation",
			SecretPattern: "vault/secret/cache/*",
			MaxAge:        "180d",
			Strategy:      secrets.RotationStrategyRolling,
			Enabled:       false,
		},
	}
}

func outputJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputYAML(v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func randomID(length int) string {
	const chars = "abcdef0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(1 * time.Nanosecond)
	}
	return string(result)
}

func normalizeStrategy(s string) secrets.RotationStrategy {
	// Accept both hyphenated (user-friendly) and underscored (internal) formats
	normalized := strings.ReplaceAll(s, "-", "_")
	return secrets.RotationStrategy(normalized)
}
