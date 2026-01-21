package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/schedule"
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
		Use:   "kscore-schedule",
		Short: "Keystone Core schedule and maintenance window management",
		Long: `kscore-schedule is a CLI plugin for managing scheduled operations
and maintenance windows in Keystone Core.

This plugin provides commands for:
  - Creating and managing scheduled operations
  - Triggering schedules manually
  - Viewing execution history
  - Creating and managing maintenance windows
  - Checking maintenance conflicts

Usage via kscorectl:
  kscorectl schedule list
  kscorectl schedule create --name daily-backup --cron "0 2 * * *"
  kscorectl maintenance create --name "weekly-patching" --start 2024-01-15T02:00:00Z --end 2024-01-15T06:00:00Z`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newVersionCmd(),
		newScheduleCmd(),
		newMaintenanceCmd(),
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
// Schedule Commands
// =============================================================================

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"sched", "s"},
		Short:   "Manage scheduled operations",
		Long:    `Commands for managing scheduled operations in Keystone Core.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(),
		newScheduleShowCmd(),
		newScheduleCreateCmd(),
		newScheduleTriggerCmd(),
		newSchedulePauseCmd(),
		newScheduleResumeCmd(),
		newScheduleEnableCmd(),
		newScheduleDisableCmd(),
		newScheduleDeleteCmd(),
		newScheduleHistoryCmd(),
	)

	return cmd
}

// ScheduleListOptions holds schedule list options
type ScheduleListOptions struct {
	Type   string
	Status string
	Labels []string
	Limit  int
}

func newScheduleListCmd() *cobra.Command {
	opts := &ScheduleListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List schedules",
		Long: `List schedules with optional filtering.

Examples:
  # List all schedules
  kscorectl schedule list

  # List only command schedules
  kscorectl schedule list --type command

  # List active schedules
  kscorectl schedule list --status active

  # Filter by labels
  kscorectl schedule list --label env:prod`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleList(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by schedule type (command, state, blueprint, reactor)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (active, paused, disabled)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of schedules to show")

	return cmd
}

func runScheduleList(cmd *cobra.Command, opts *ScheduleListOptions) error {
	schedules := generateSampleSchedules()

	var filtered []*scheduleDisplay
	for _, s := range schedules {
		if opts.Type != "" && string(s.Type) != opts.Type {
			continue
		}
		if opts.Status != "" && string(s.Status) != opts.Status {
			continue
		}
		filtered = append(filtered, s)
	}

	if len(filtered) == 0 {
		fmt.Println("No schedules found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(filtered)
	case "yaml":
		return outputYAML(filtered)
	default:
		table := &output.Table{
			Headers: []string{"ID", "NAME", "TYPE", "STATUS", "CRON/INTERVAL", "NEXT RUN"},
		}
		for _, s := range filtered {
			nextRun := "N/A"
			if s.NextRun != "" {
				nextRun = s.NextRun
			}
			cronOrInterval := s.Cron
			if cronOrInterval == "" {
				cronOrInterval = s.Interval
			}
			table.Rows = append(table.Rows, []string{
				truncate(s.ID, 8),
				truncate(s.Name, 25),
				string(s.Type),
				string(s.Status),
				cronOrInterval,
				nextRun,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d schedule(s)\n", len(filtered))
	}

	return nil
}

func newScheduleShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <schedule-id>",
		Short: "Show schedule details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleShow(cmd, args[0])
		},
	}
}

func runScheduleShow(cmd *cobra.Command, id string) error {
	// Sample schedule for demonstration
	s := &scheduleDetail{
		ID:          id,
		Name:        "daily-backup",
		Description: "Daily backup of all databases",
		Type:        schedule.ScheduleTypeCommand,
		Status:      schedule.ScheduleStatusActive,
		Cron:        "0 2 * * *",
		Timezone:    "UTC",
		Priority:    10,
		Timeout:     "1h",
		Target: &targetDisplay{
			All: false,
			Tags: map[string]string{
				"role": "database",
			},
		},
		NextRun:         time.Now().Add(12 * time.Hour).Format(time.RFC3339),
		LastRun:         time.Now().Add(-12 * time.Hour).Format(time.RFC3339),
		RunCount:        156,
		SuccessCount:    154,
		FailureCount:    2,
		RequireApproval: false,
		CreatedAt:       time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		CreatedBy:       "admin",
	}

	switch outputFormat {
	case "json":
		return outputJSON(s)
	case "yaml":
		return outputYAML(s)
	default:
		fmt.Printf("Schedule: %s\n", s.Name)
		fmt.Printf("  ID:           %s\n", s.ID)
		fmt.Printf("  Type:         %s\n", s.Type)
		fmt.Printf("  Status:       %s\n", s.Status)
		fmt.Printf("  Description:  %s\n", s.Description)
		fmt.Printf("  Cron:         %s\n", s.Cron)
		fmt.Printf("  Timezone:     %s\n", s.Timezone)
		fmt.Printf("  Priority:     %d\n", s.Priority)
		fmt.Printf("  Timeout:      %s\n", s.Timeout)
		fmt.Printf("  Next Run:     %s\n", s.NextRun)
		fmt.Printf("  Last Run:     %s\n", s.LastRun)
		fmt.Printf("  Run Count:    %d (success: %d, failure: %d)\n", s.RunCount, s.SuccessCount, s.FailureCount)
		fmt.Printf("  Approval:     %v\n", s.RequireApproval)
		fmt.Printf("  Created:      %s by %s\n", s.CreatedAt, s.CreatedBy)
	}

	return nil
}

// ScheduleCreateOptions holds schedule creation options
type ScheduleCreateOptions struct {
	Name              string
	Description       string
	Type              string
	Cron              string
	Interval          string
	Timezone          string
	TargetAll         bool
	TargetAgents      []string
	TargetGlob        string
	TargetTags        []string
	TargetRoles       []string
	Command           string
	StatePath         string
	Blueprint         string
	Priority          int
	Timeout           string
	RequireApproval   bool
	Labels            []string
	MaintenanceWindow string
}

func newScheduleCreateCmd() *cobra.Command {
	opts := &ScheduleCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new schedule",
		Long: `Create a new scheduled operation.

Examples:
  # Create a cron-based command schedule
  kscorectl schedule create --name daily-backup --type command \
    --cron "0 2 * * *" --target-all --command "backup.sh"

  # Create an interval-based state schedule
  kscorectl schedule create --name hourly-sync --type state \
    --interval 1h --state-path /states/sync.yaml --target-tags role:web

  # Create a blueprint schedule with approval
  kscorectl schedule create --name weekly-patching --type blueprint \
    --cron "0 3 * * 0" --blueprint security-patches \
    --require-approval --target-tags env:prod`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Schedule name (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Schedule description")
	cmd.Flags().StringVar(&opts.Type, "type", "command", "Schedule type (command, state, blueprint, reactor)")
	cmd.Flags().StringVar(&opts.Cron, "cron", "", "Cron expression (e.g., '0 2 * * *')")
	cmd.Flags().StringVar(&opts.Interval, "interval", "", "Interval (e.g., '1h', '30m')")
	cmd.Flags().StringVar(&opts.Timezone, "timezone", "UTC", "Timezone for schedule evaluation")
	cmd.Flags().BoolVar(&opts.TargetAll, "target-all", false, "Target all agents")
	cmd.Flags().StringArrayVar(&opts.TargetAgents, "target-agent", nil, "Target specific agents")
	cmd.Flags().StringVar(&opts.TargetGlob, "target-glob", "", "Target agents matching glob pattern")
	cmd.Flags().StringArrayVar(&opts.TargetTags, "target-tags", nil, "Target agents with tags (key:value)")
	cmd.Flags().StringArrayVar(&opts.TargetRoles, "target-roles", nil, "Target agents with roles")
	cmd.Flags().StringVar(&opts.Command, "command", "", "Command to execute (for command type)")
	cmd.Flags().StringVar(&opts.StatePath, "state-path", "", "State file path (for state type)")
	cmd.Flags().StringVar(&opts.Blueprint, "blueprint", "", "Blueprint name (for blueprint type)")
	cmd.Flags().IntVar(&opts.Priority, "priority", 5, "Schedule priority (0-10)")
	cmd.Flags().StringVar(&opts.Timeout, "timeout", "1h", "Execution timeout")
	cmd.Flags().BoolVar(&opts.RequireApproval, "require-approval", false, "Require approval before execution")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels (key:value format)")
	cmd.Flags().StringVar(&opts.MaintenanceWindow, "maintenance-window", "", "Link to maintenance window")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runScheduleCreate(cmd *cobra.Command, opts *ScheduleCreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("--name is required")
	}

	if opts.Cron == "" && opts.Interval == "" {
		return fmt.Errorf("either --cron or --interval is required")
	}

	if !opts.TargetAll && len(opts.TargetAgents) == 0 && opts.TargetGlob == "" &&
		len(opts.TargetTags) == 0 && len(opts.TargetRoles) == 0 {
		return fmt.Errorf("at least one target option is required")
	}

	// In production, this would call the API
	fmt.Printf("Created schedule '%s' (id: sched-%s)\n", opts.Name, randomID(8))
	return nil
}

func newScheduleTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <schedule-id>",
		Short: "Trigger a schedule immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Triggered schedule %s (execution: exec-%s)\n", args[0], randomID(8))
			return nil
		},
	}
}

func newSchedulePauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <schedule-id>",
		Short: "Pause a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Paused schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <schedule-id>",
		Short: "Resume a paused schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Resumed schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <schedule-id>",
		Short: "Enable a disabled schedule",
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

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion without confirmation")

	return cmd
}

// HistoryOptions holds schedule history options
type HistoryOptions struct {
	Limit  int
	Status string
}

func newScheduleHistoryCmd() *cobra.Command {
	opts := &HistoryOptions{}

	cmd := &cobra.Command{
		Use:   "history <schedule-id>",
		Short: "Show execution history for a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleHistory(cmd, args[0], opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 20, "Number of executions to show")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status")

	return cmd
}

func runScheduleHistory(cmd *cobra.Command, scheduleID string, opts *HistoryOptions) error {
	executions := generateSampleExecutions(scheduleID, opts.Limit)

	switch outputFormat {
	case "json":
		return outputJSON(executions)
	case "yaml":
		return outputYAML(executions)
	default:
		table := &output.Table{
			Headers: []string{"EXECUTION ID", "STATUS", "TRIGGER", "STARTED", "DURATION", "SUCCESS/FAIL"},
		}
		for _, e := range executions {
			table.Rows = append(table.Rows, []string{
				truncate(e.ID, 12),
				e.Status,
				e.Trigger,
				e.StartTime,
				e.Duration,
				fmt.Sprintf("%d/%d", e.SuccessCount, e.FailureCount),
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nShowing %d execution(s) for schedule %s\n", len(executions), scheduleID)
	}

	return nil
}

// =============================================================================
// Maintenance Commands
// =============================================================================

func newMaintenanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "maintenance",
		Aliases: []string{"maint", "m"},
		Short:   "Manage maintenance windows",
		Long:    `Commands for managing maintenance windows in Keystone Core.`,
	}

	cmd.AddCommand(
		newMaintenanceListCmd(),
		newMaintenanceShowCmd(),
		newMaintenanceCreateCmd(),
		newMaintenanceStartCmd(),
		newMaintenanceEndCmd(),
		newMaintenanceCancelCmd(),
		newMaintenanceExtendCmd(),
		newMaintenanceActiveCmd(),
		newMaintenanceUpcomingCmd(),
		newMaintenanceConflictsCmd(),
		newMaintenanceDeleteCmd(),
	)

	return cmd
}

// MaintenanceListOptions holds maintenance list options
type MaintenanceListOptions struct {
	Status string
	Type   string
	Labels []string
	Limit  int
}

func newMaintenanceListCmd() *cobra.Command {
	opts := &MaintenanceListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List maintenance windows",
		Long: `List maintenance windows with optional filtering.

Examples:
  # List all maintenance windows
  kscorectl maintenance list

  # List only active windows
  kscorectl maintenance list --status active

  # List emergency windows
  kscorectl maintenance list --type emergency`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceList(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (scheduled, active, completed, cancelled)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by type (planned, emergency, recurring)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of windows to show")

	return cmd
}

func runMaintenanceList(cmd *cobra.Command, opts *MaintenanceListOptions) error {
	windows := generateSampleWindows()

	var filtered []*windowDisplay
	for _, w := range windows {
		if opts.Status != "" && string(w.Status) != opts.Status {
			continue
		}
		if opts.Type != "" && string(w.Type) != opts.Type {
			continue
		}
		filtered = append(filtered, w)
	}

	if len(filtered) == 0 {
		fmt.Println("No maintenance windows found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(filtered)
	case "yaml":
		return outputYAML(filtered)
	default:
		table := &output.Table{
			Headers: []string{"ID", "NAME", "TYPE", "STATUS", "START", "END", "SCOPE"},
		}
		for _, w := range filtered {
			scope := "all"
			if !w.ScopeAll {
				scope = fmt.Sprintf("%d agents", w.AgentCount)
			}
			table.Rows = append(table.Rows, []string{
				truncate(w.ID, 8),
				truncate(w.Name, 20),
				string(w.Type),
				string(w.Status),
				w.StartTime,
				w.EndTime,
				scope,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d window(s)\n", len(filtered))
	}

	return nil
}

func newMaintenanceShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <window-id>",
		Short: "Show maintenance window details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceShow(cmd, args[0])
		},
	}
}

func runMaintenanceShow(cmd *cobra.Command, id string) error {
	// Sample window for demonstration
	w := &windowDetail{
		ID:          id,
		Name:        "weekly-patching",
		Description: "Weekly security patching window",
		Type:        schedule.MaintenanceWindowTypePlanned,
		Status:      schedule.MaintenanceWindowStatusScheduled,
		StartTime:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		EndTime:     time.Now().Add(28 * time.Hour).Format(time.RFC3339),
		Timezone:    "UTC",
		Scope: &scopeDisplay{
			All: false,
			Tags: map[string]string{
				"env": "prod",
			},
		},
		Behavior: &behaviorDisplay{
			SuppressAlerts:         true,
			SuppressDriftDetection: true,
			AllowOperations:        false,
		},
		RequireApproval: true,
		CreatedAt:       time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339),
		CreatedBy:       "admin",
	}

	switch outputFormat {
	case "json":
		return outputJSON(w)
	case "yaml":
		return outputYAML(w)
	default:
		fmt.Printf("Maintenance Window: %s\n", w.Name)
		fmt.Printf("  ID:          %s\n", w.ID)
		fmt.Printf("  Type:        %s\n", w.Type)
		fmt.Printf("  Status:      %s\n", w.Status)
		fmt.Printf("  Description: %s\n", w.Description)
		fmt.Printf("  Start:       %s\n", w.StartTime)
		fmt.Printf("  End:         %s\n", w.EndTime)
		fmt.Printf("  Timezone:    %s\n", w.Timezone)
		fmt.Printf("  Approval:    %v\n", w.RequireApproval)
		fmt.Printf("  Created:     %s by %s\n", w.CreatedAt, w.CreatedBy)
		fmt.Printf("  Behavior:\n")
		fmt.Printf("    Suppress Alerts:         %v\n", w.Behavior.SuppressAlerts)
		fmt.Printf("    Suppress Drift:          %v\n", w.Behavior.SuppressDriftDetection)
		fmt.Printf("    Allow Operations:        %v\n", w.Behavior.AllowOperations)
	}

	return nil
}

// MaintenanceCreateOptions holds maintenance creation options
type MaintenanceCreateOptions struct {
	Name                   string
	Description            string
	Type                   string
	StartTime              string
	EndTime                string
	Timezone               string
	ScopeAll               bool
	ScopeAgents            []string
	ScopeGlob              string
	ScopeTags              []string
	ScopeRoles             []string
	SuppressAlerts         bool
	SuppressDriftDetection bool
	AllowOperations        bool
	RequireApproval        bool
	NotifyBefore           string
	NotifyChannels         []string
	Labels                 []string
}

func newMaintenanceCreateCmd() *cobra.Command {
	opts := &MaintenanceCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new maintenance window",
		Long: `Create a new maintenance window.

Examples:
  # Create a planned maintenance window
  kscorectl maintenance create --name "weekly-patching" \
    --start "2024-01-15T02:00:00Z" --end "2024-01-15T06:00:00Z" \
    --scope-tags env:prod --suppress-alerts

  # Create an emergency maintenance window
  kscorectl maintenance create --name "urgent-fix" --type emergency \
    --start now --duration 2h --scope-all

  # Create with approval requirement
  kscorectl maintenance create --name "db-migration" \
    --start "2024-01-20T00:00:00Z" --end "2024-01-20T04:00:00Z" \
    --require-approval --scope-agents db-01,db-02`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Window name (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Window description")
	cmd.Flags().StringVar(&opts.Type, "type", "planned", "Window type (planned, emergency, recurring)")
	cmd.Flags().StringVar(&opts.StartTime, "start", "", "Start time (RFC3339 format or 'now')")
	cmd.Flags().StringVar(&opts.EndTime, "end", "", "End time (RFC3339 format)")
	cmd.Flags().StringVar(&opts.Timezone, "timezone", "UTC", "Timezone")
	cmd.Flags().BoolVar(&opts.ScopeAll, "scope-all", false, "Affect all agents")
	cmd.Flags().StringArrayVar(&opts.ScopeAgents, "scope-agents", nil, "Specific agents")
	cmd.Flags().StringVar(&opts.ScopeGlob, "scope-glob", "", "Agent glob pattern")
	cmd.Flags().StringArrayVar(&opts.ScopeTags, "scope-tags", nil, "Agent tags (key:value)")
	cmd.Flags().StringArrayVar(&opts.ScopeRoles, "scope-roles", nil, "Agent roles")
	cmd.Flags().BoolVar(&opts.SuppressAlerts, "suppress-alerts", true, "Suppress alerts during window")
	cmd.Flags().BoolVar(&opts.SuppressDriftDetection, "suppress-drift", false, "Suppress drift detection")
	cmd.Flags().BoolVar(&opts.AllowOperations, "allow-operations", true, "Allow manual operations")
	cmd.Flags().BoolVar(&opts.RequireApproval, "require-approval", false, "Require approval to start")
	cmd.Flags().StringVar(&opts.NotifyBefore, "notify-before", "15m", "Notification lead time")
	cmd.Flags().StringArrayVar(&opts.NotifyChannels, "notify-channel", nil, "Notification channels")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels (key:value format)")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")

	return cmd
}

func runMaintenanceCreate(cmd *cobra.Command, opts *MaintenanceCreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if opts.StartTime == "" {
		return fmt.Errorf("--start is required")
	}
	if opts.EndTime == "" {
		return fmt.Errorf("--end is required")
	}

	if !opts.ScopeAll && len(opts.ScopeAgents) == 0 && opts.ScopeGlob == "" &&
		len(opts.ScopeTags) == 0 && len(opts.ScopeRoles) == 0 {
		return fmt.Errorf("at least one scope option is required")
	}

	// In production, this would call the API
	fmt.Printf("Created maintenance window '%s' (id: maint-%s)\n", opts.Name, randomID(8))
	return nil
}

func newMaintenanceStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <window-id>",
		Short: "Start a scheduled maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Started maintenance window %s\n", args[0])
			return nil
		},
	}
}

func newMaintenanceEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "end <window-id>",
		Short: "End an active maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Ended maintenance window %s\n", args[0])
			return nil
		},
	}
}

func newMaintenanceCancelCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "cancel <window-id>",
		Short: "Cancel a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Cancelled maintenance window %s (reason: %s)\n", args[0], reason)
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")

	return cmd
}

func newMaintenanceExtendCmd() *cobra.Command {
	var newEndTime string
	var duration string

	cmd := &cobra.Command{
		Use:   "extend <window-id>",
		Short: "Extend a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newEndTime != "" {
				fmt.Printf("Extended maintenance window %s to %s\n", args[0], newEndTime)
			} else if duration != "" {
				fmt.Printf("Extended maintenance window %s by %s\n", args[0], duration)
			} else {
				return fmt.Errorf("either --end or --duration is required")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&newEndTime, "end", "", "New end time (RFC3339 format)")
	cmd.Flags().StringVar(&duration, "duration", "", "Extend by duration (e.g., 1h, 30m)")

	return cmd
}

func newMaintenanceActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "List currently active maintenance windows",
		RunE: func(cmd *cobra.Command, args []string) error {
			windows := generateSampleWindows()
			var active []*windowDisplay
			for _, w := range windows {
				if w.Status == schedule.MaintenanceWindowStatusActive {
					active = append(active, w)
				}
			}

			if len(active) == 0 {
				fmt.Println("No active maintenance windows")
				return nil
			}

			table := &output.Table{
				Headers: []string{"ID", "NAME", "STARTED", "ENDS", "SCOPE"},
			}
			for _, w := range active {
				scope := "all"
				if !w.ScopeAll {
					scope = fmt.Sprintf("%d agents", w.AgentCount)
				}
				table.Rows = append(table.Rows, []string{
					truncate(w.ID, 8),
					truncate(w.Name, 20),
					w.StartTime,
					w.EndTime,
					scope,
				})
			}
			output.WriteTable(os.Stdout, table)
			return nil
		},
	}
}

func newMaintenanceUpcomingCmd() *cobra.Command {
	var within string

	cmd := &cobra.Command{
		Use:   "upcoming",
		Short: "List upcoming maintenance windows",
		RunE: func(cmd *cobra.Command, args []string) error {
			windows := generateSampleWindows()
			var upcoming []*windowDisplay
			for _, w := range windows {
				if w.Status == schedule.MaintenanceWindowStatusScheduled {
					upcoming = append(upcoming, w)
				}
			}

			if len(upcoming) == 0 {
				fmt.Println("No upcoming maintenance windows")
				return nil
			}

			table := &output.Table{
				Headers: []string{"ID", "NAME", "TYPE", "STARTS", "ENDS", "SCOPE"},
			}
			for _, w := range upcoming {
				scope := "all"
				if !w.ScopeAll {
					scope = fmt.Sprintf("%d agents", w.AgentCount)
				}
				table.Rows = append(table.Rows, []string{
					truncate(w.ID, 8),
					truncate(w.Name, 20),
					string(w.Type),
					w.StartTime,
					w.EndTime,
					scope,
				})
			}
			output.WriteTable(os.Stdout, table)
			return nil
		},
	}

	cmd.Flags().StringVar(&within, "within", "24h", "Show windows starting within (e.g., 24h, 7d)")

	return cmd
}

func newMaintenanceConflictsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts <window-id>",
		Short: "Check for conflicts with other windows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Checking conflicts for window %s...\n\n", args[0])
			fmt.Println("No conflicts found")
			return nil
		},
	}
}

func newMaintenanceDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <window-id>",
		Short: "Delete a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete window %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Deleted maintenance window %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion without confirmation")

	return cmd
}

// =============================================================================
// Display Types
// =============================================================================

type scheduleDisplay struct {
	ID       string                  `json:"id" yaml:"id"`
	Name     string                  `json:"name" yaml:"name"`
	Type     schedule.ScheduleType   `json:"type" yaml:"type"`
	Status   schedule.ScheduleStatus `json:"status" yaml:"status"`
	Cron     string                  `json:"cron,omitempty" yaml:"cron,omitempty"`
	Interval string                  `json:"interval,omitempty" yaml:"interval,omitempty"`
	NextRun  string                  `json:"next_run,omitempty" yaml:"next_run,omitempty"`
}

type scheduleDetail struct {
	ID              string                  `json:"id" yaml:"id"`
	Name            string                  `json:"name" yaml:"name"`
	Description     string                  `json:"description" yaml:"description"`
	Type            schedule.ScheduleType   `json:"type" yaml:"type"`
	Status          schedule.ScheduleStatus `json:"status" yaml:"status"`
	Cron            string                  `json:"cron,omitempty" yaml:"cron,omitempty"`
	Interval        string                  `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timezone        string                  `json:"timezone" yaml:"timezone"`
	Priority        int                     `json:"priority" yaml:"priority"`
	Timeout         string                  `json:"timeout" yaml:"timeout"`
	Target          *targetDisplay          `json:"target" yaml:"target"`
	NextRun         string                  `json:"next_run,omitempty" yaml:"next_run,omitempty"`
	LastRun         string                  `json:"last_run,omitempty" yaml:"last_run,omitempty"`
	RunCount        int64                   `json:"run_count" yaml:"run_count"`
	SuccessCount    int64                   `json:"success_count" yaml:"success_count"`
	FailureCount    int64                   `json:"failure_count" yaml:"failure_count"`
	RequireApproval bool                    `json:"require_approval" yaml:"require_approval"`
	CreatedAt       string                  `json:"created_at" yaml:"created_at"`
	CreatedBy       string                  `json:"created_by" yaml:"created_by"`
}

type targetDisplay struct {
	All      bool              `json:"all,omitempty" yaml:"all,omitempty"`
	AgentIDs []string          `json:"agent_ids,omitempty" yaml:"agent_ids,omitempty"`
	Glob     string            `json:"glob,omitempty" yaml:"glob,omitempty"`
	Tags     map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Roles    []string          `json:"roles,omitempty" yaml:"roles,omitempty"`
}

type executionDisplay struct {
	ID           string `json:"id" yaml:"id"`
	Status       string `json:"status" yaml:"status"`
	Trigger      string `json:"trigger" yaml:"trigger"`
	StartTime    string `json:"start_time" yaml:"start_time"`
	Duration     string `json:"duration" yaml:"duration"`
	SuccessCount int    `json:"success_count" yaml:"success_count"`
	FailureCount int    `json:"failure_count" yaml:"failure_count"`
}

type windowDisplay struct {
	ID         string                             `json:"id" yaml:"id"`
	Name       string                             `json:"name" yaml:"name"`
	Type       schedule.MaintenanceWindowType     `json:"type" yaml:"type"`
	Status     schedule.MaintenanceWindowStatus   `json:"status" yaml:"status"`
	StartTime  string                             `json:"start_time" yaml:"start_time"`
	EndTime    string                             `json:"end_time" yaml:"end_time"`
	ScopeAll   bool                               `json:"scope_all" yaml:"scope_all"`
	AgentCount int                                `json:"agent_count" yaml:"agent_count"`
}

type windowDetail struct {
	ID              string                           `json:"id" yaml:"id"`
	Name            string                           `json:"name" yaml:"name"`
	Description     string                           `json:"description" yaml:"description"`
	Type            schedule.MaintenanceWindowType   `json:"type" yaml:"type"`
	Status          schedule.MaintenanceWindowStatus `json:"status" yaml:"status"`
	StartTime       string                           `json:"start_time" yaml:"start_time"`
	EndTime         string                           `json:"end_time" yaml:"end_time"`
	Timezone        string                           `json:"timezone" yaml:"timezone"`
	Scope           *scopeDisplay                    `json:"scope" yaml:"scope"`
	Behavior        *behaviorDisplay                 `json:"behavior" yaml:"behavior"`
	RequireApproval bool                             `json:"require_approval" yaml:"require_approval"`
	CreatedAt       string                           `json:"created_at" yaml:"created_at"`
	CreatedBy       string                           `json:"created_by" yaml:"created_by"`
}

type scopeDisplay struct {
	All      bool              `json:"all,omitempty" yaml:"all,omitempty"`
	AgentIDs []string          `json:"agent_ids,omitempty" yaml:"agent_ids,omitempty"`
	Glob     string            `json:"glob,omitempty" yaml:"glob,omitempty"`
	Tags     map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Roles    []string          `json:"roles,omitempty" yaml:"roles,omitempty"`
}

type behaviorDisplay struct {
	SuppressAlerts         bool `json:"suppress_alerts" yaml:"suppress_alerts"`
	SuppressDriftDetection bool `json:"suppress_drift_detection" yaml:"suppress_drift_detection"`
	AllowOperations        bool `json:"allow_operations" yaml:"allow_operations"`
}

// =============================================================================
// Helpers
// =============================================================================

func generateSampleSchedules() []*scheduleDisplay {
	return []*scheduleDisplay{
		{
			ID:      "sched-001",
			Name:    "daily-backup",
			Type:    schedule.ScheduleTypeCommand,
			Status:  schedule.ScheduleStatusActive,
			Cron:    "0 2 * * *",
			NextRun: time.Now().Add(12 * time.Hour).Format("15:04"),
		},
		{
			ID:       "sched-002",
			Name:     "hourly-sync",
			Type:     schedule.ScheduleTypeState,
			Status:   schedule.ScheduleStatusActive,
			Interval: "1h",
			NextRun:  time.Now().Add(45 * time.Minute).Format("15:04"),
		},
		{
			ID:      "sched-003",
			Name:    "weekly-patching",
			Type:    schedule.ScheduleTypeBlueprint,
			Status:  schedule.ScheduleStatusPaused,
			Cron:    "0 3 * * 0",
			NextRun: "",
		},
	}
}

func generateSampleExecutions(scheduleID string, limit int) []*executionDisplay {
	executions := make([]*executionDisplay, 0, limit)
	statuses := []string{"completed", "completed", "completed", "failed", "completed"}

	for i := 0; i < limit && i < 5; i++ {
		executions = append(executions, &executionDisplay{
			ID:           fmt.Sprintf("exec-%03d", i+1),
			Status:       statuses[i%len(statuses)],
			Trigger:      "scheduled",
			StartTime:    time.Now().Add(-time.Duration(i*24) * time.Hour).Format("Jan 02 15:04"),
			Duration:     fmt.Sprintf("%dm%ds", 2+i, 30+i*10),
			SuccessCount: 10 - i,
			FailureCount: i,
		})
	}

	return executions
}

func generateSampleWindows() []*windowDisplay {
	return []*windowDisplay{
		{
			ID:         "maint-001",
			Name:       "weekly-patching",
			Type:       schedule.MaintenanceWindowTypePlanned,
			Status:     schedule.MaintenanceWindowStatusScheduled,
			StartTime:  time.Now().Add(24 * time.Hour).Format("Jan 02 15:04"),
			EndTime:    time.Now().Add(28 * time.Hour).Format("Jan 02 15:04"),
			ScopeAll:   false,
			AgentCount: 15,
		},
		{
			ID:         "maint-002",
			Name:       "db-migration",
			Type:       schedule.MaintenanceWindowTypePlanned,
			Status:     schedule.MaintenanceWindowStatusActive,
			StartTime:  time.Now().Add(-1 * time.Hour).Format("Jan 02 15:04"),
			EndTime:    time.Now().Add(3 * time.Hour).Format("Jan 02 15:04"),
			ScopeAll:   false,
			AgentCount: 3,
		},
		{
			ID:         "maint-003",
			Name:       "emergency-fix",
			Type:       schedule.MaintenanceWindowTypeEmergency,
			Status:     schedule.MaintenanceWindowStatusCompleted,
			StartTime:  time.Now().Add(-48 * time.Hour).Format("Jan 02 15:04"),
			EndTime:    time.Now().Add(-46 * time.Hour).Format("Jan 02 15:04"),
			ScopeAll:   true,
			AgentCount: 0,
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
		time.Sleep(1)
	}
	return string(result)
}

func parseLabels(labels []string) map[string]string {
	result := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, ":", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
