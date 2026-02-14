// Package main implements the kscore-schedule CLI for schedule and maintenance window management.
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
	apischedule "github.com/shawnbutts/keystone-core/pkg/api/schedule"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Config holds CLI configuration.
type Config struct {
	ServerAddr   string
	OutputFormat string
	Verbose      bool
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cfg := &Config{}

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

	rootCmd.PersistentFlags().StringVarP(&cfg.ServerAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&cfg.OutputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newVersionCmd(),
		newScheduleCmd(cfg),
		newMaintenanceCmd(cfg),
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

func newClient(cfg *Config) *Client {
	addr := cfg.ServerAddr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return NewClient(addr)
}

// =============================================================================
// Schedule Commands
// =============================================================================

func newScheduleCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"sched", "s"},
		Short:   "Manage scheduled operations",
		Long:    `Commands for managing scheduled operations in Keystone Core.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(cfg),
		newScheduleShowCmd(cfg),
		newScheduleCreateCmd(cfg),
		newScheduleTriggerCmd(cfg),
		newSchedulePauseCmd(cfg),
		newScheduleResumeCmd(cfg),
		newScheduleEnableCmd(cfg),
		newScheduleDisableCmd(cfg),
		newScheduleDeleteCmd(cfg),
		newScheduleHistoryCmd(cfg),
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

func newScheduleListCmd(cfg *Config) *cobra.Command {
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
			return runScheduleList(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by schedule type (command, state, blueprint, reactor)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (active, paused, disabled)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of schedules to show")

	return cmd
}

func runScheduleList(cmd *cobra.Command, cfg *Config, opts *ScheduleListOptions) error {
	client := newClient(cfg)
	resp, err := client.ListSchedules(opts.Type, opts.Status, opts.Limit)
	if err != nil {
		return err
	}

	if len(resp.Schedules) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No schedules found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(cmd.OutOrStdout(), resp.Schedules)
	case "yaml":
		return outputYAML(cmd.OutOrStdout(), resp.Schedules)
	default:
		table := &output.Table{
			Headers: []string{"ID", "NAME", "TYPE", "STATUS", "CRON/INTERVAL", "NEXT RUN"},
		}
		for i := range resp.Schedules {
			s := &resp.Schedules[i]
			nextRun := "N/A"
			if s.NextRun != nil {
				nextRun = s.NextRun.Format(time.RFC3339)
			}
			cronOrInterval := s.Cron
			if cronOrInterval == "" {
				cronOrInterval = s.Interval
			}
			table.Rows = append(table.Rows, []string{
				truncate(s.ID, 8),
				truncate(s.Name, 25),
				s.Type,
				s.Status,
				cronOrInterval,
				nextRun,
			})
		}
		w := cmd.OutOrStdout()
		output.WriteTable(w, table)
		fmt.Fprintf(w, "\nTotal: %d schedule(s)\n", resp.Total)
	}

	return nil
}

func newScheduleShowCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <schedule-id>",
		Short: "Show schedule details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleShow(cmd, cfg, args[0])
		},
	}
}

func runScheduleShow(cmd *cobra.Command, cfg *Config, id string) error {
	client := newClient(cfg)
	s, err := client.GetSchedule(id)
	if err != nil {
		return err
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(cmd.OutOrStdout(), s)
	case "yaml":
		return outputYAML(cmd.OutOrStdout(), s)
	default:
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "Schedule: %s\n", s.Name)
		fmt.Fprintf(w, "  ID:           %s\n", s.ID)
		fmt.Fprintf(w, "  Type:         %s\n", s.Type)
		fmt.Fprintf(w, "  Status:       %s\n", s.Status)
		if s.Description != "" {
			fmt.Fprintf(w, "  Description:  %s\n", s.Description)
		}
		if s.Cron != "" {
			fmt.Fprintf(w, "  Cron:         %s\n", s.Cron)
		}
		if s.Interval != "" {
			fmt.Fprintf(w, "  Interval:     %s\n", s.Interval)
		}
		if s.Timezone != "" {
			fmt.Fprintf(w, "  Timezone:     %s\n", s.Timezone)
		}
		fmt.Fprintf(w, "  Priority:     %d\n", s.Priority)
		if s.Timeout != "" {
			fmt.Fprintf(w, "  Timeout:      %s\n", s.Timeout)
		}
		if s.NextRun != nil {
			fmt.Fprintf(w, "  Next Run:     %s\n", s.NextRun.Format(time.RFC3339))
		}
		if s.LastRun != nil {
			fmt.Fprintf(w, "  Last Run:     %s\n", s.LastRun.Format(time.RFC3339))
		}
		fmt.Fprintf(w, "  Run Count:    %d (success: %d, failure: %d)\n", s.RunCount, s.SuccessCount, s.FailureCount)
		fmt.Fprintf(w, "  Approval:     %v\n", s.RequireApproval)
		fmt.Fprintf(w, "  Created:      %s", s.CreatedAt.Format(time.RFC3339))
		if s.CreatedBy != "" {
			fmt.Fprintf(w, " by %s", s.CreatedBy)
		}
		fmt.Fprintln(w)
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

func newScheduleCreateCmd(cfg *Config) *cobra.Command {
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
			return runScheduleCreate(cmd, cfg, opts)
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

func runScheduleCreate(cmd *cobra.Command, cfg *Config, opts *ScheduleCreateOptions) error {
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

	client := newClient(cfg)
	req := &apischedule.CreateScheduleRequest{
		Name:            opts.Name,
		Description:     opts.Description,
		Type:            opts.Type,
		Cron:            opts.Cron,
		Interval:        opts.Interval,
		Timezone:        opts.Timezone,
		Priority:        opts.Priority,
		Timeout:         opts.Timeout,
		RequireApproval: opts.RequireApproval,
		Labels:          parseLabels(opts.Labels),
	}

	resp, err := client.CreateSchedule(req)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created schedule '%s' (id: %s)\n", resp.Name, resp.ID)
	return nil
}

func newScheduleTriggerCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <schedule-id>",
		Short: "Trigger a schedule immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			exec, err := client.TriggerSchedule(args[0], "cli")
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Triggered schedule %s (execution: %s)\n", args[0], exec.ID)
			return nil
		},
	}
}

func newSchedulePauseCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <schedule-id>",
		Short: "Pause a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.PauseSchedule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Paused schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleResumeCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <schedule-id>",
		Short: "Resume a paused schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.ResumeSchedule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resumed schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleEnableCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <schedule-id>",
		Short: "Enable a disabled schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.EnableSchedule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Enabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDisableCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <schedule-id>",
		Short: "Disable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.DisableSchedule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Disabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDeleteCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <schedule-id>",
		Short: "Delete a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprintf(cmd.OutOrStdout(), "Are you sure you want to delete schedule %s? (use --force to confirm)\n", args[0])
				return nil
			}
			client := newClient(cfg)
			if err := client.DeleteSchedule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted schedule %s\n", args[0])
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

func newScheduleHistoryCmd(cfg *Config) *cobra.Command {
	opts := &HistoryOptions{}

	cmd := &cobra.Command{
		Use:   "history <schedule-id>",
		Short: "Show execution history for a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleHistory(cmd, cfg, args[0], opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 20, "Number of executions to show")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status")

	return cmd
}

func runScheduleHistory(cmd *cobra.Command, cfg *Config, scheduleID string, opts *HistoryOptions) error {
	client := newClient(cfg)
	resp, err := client.GetHistory(scheduleID, opts.Status, opts.Limit)
	if err != nil {
		return err
	}

	if len(resp.Executions) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No executions found for schedule %s\n", scheduleID)
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(cmd.OutOrStdout(), resp.Executions)
	case "yaml":
		return outputYAML(cmd.OutOrStdout(), resp.Executions)
	default:
		table := &output.Table{
			Headers: []string{"EXECUTION ID", "STATUS", "TRIGGER", "STARTED", "DURATION", "SUCCESS/FAIL"},
		}
		for i := range resp.Executions {
			e := &resp.Executions[i]
			startTime := "N/A"
			if e.StartTime != nil {
				startTime = e.StartTime.Format("Jan 02 15:04")
			}
			duration := "N/A"
			if e.Duration != "" {
				duration = e.Duration
			}
			table.Rows = append(table.Rows, []string{
				truncate(e.ID, 12),
				e.Status,
				e.TriggerType,
				startTime,
				duration,
				fmt.Sprintf("%d/%d", e.SuccessCount, e.FailureCount),
			})
		}
		w := cmd.OutOrStdout()
		output.WriteTable(w, table)
		fmt.Fprintf(w, "\nShowing %d execution(s) for schedule %s\n", len(resp.Executions), scheduleID)
	}

	return nil
}

// =============================================================================
// Maintenance Commands
// =============================================================================

func newMaintenanceCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "maintenance",
		Aliases: []string{"maint", "m"},
		Short:   "Manage maintenance windows",
		Long:    `Commands for managing maintenance windows in Keystone Core.`,
	}

	cmd.AddCommand(
		newMaintenanceListCmd(cfg),
		newMaintenanceShowCmd(cfg),
		newMaintenanceCreateCmd(cfg),
		newMaintenanceStartCmd(cfg),
		newMaintenanceEndCmd(cfg),
		newMaintenanceCancelCmd(cfg),
		newMaintenanceExtendCmd(cfg),
		newMaintenanceActiveCmd(cfg),
		newMaintenanceUpcomingCmd(cfg),
		newMaintenanceConflictsCmd(cfg),
		newMaintenanceDeleteCmd(cfg),
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

func newMaintenanceListCmd(cfg *Config) *cobra.Command {
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
			return runMaintenanceList(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (scheduled, active, completed, cancelled)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by type (planned, emergency, recurring)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of windows to show")

	return cmd
}

func runMaintenanceList(cmd *cobra.Command, cfg *Config, opts *MaintenanceListOptions) error {
	client := newClient(cfg)
	resp, err := client.ListWindows(opts.Status, opts.Type, opts.Limit)
	if err != nil {
		return err
	}

	if len(resp.Windows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No maintenance windows found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(cmd.OutOrStdout(), resp.Windows)
	case "yaml":
		return outputYAML(cmd.OutOrStdout(), resp.Windows)
	default:
		table := &output.Table{
			Headers: []string{"ID", "NAME", "TYPE", "STATUS", "START", "END"},
		}
		for i := range resp.Windows {
			win := &resp.Windows[i]
			table.Rows = append(table.Rows, []string{
				truncate(win.ID, 8),
				truncate(win.Name, 20),
				win.Type,
				win.Status,
				win.StartTime.Format("Jan 02 15:04"),
				win.EndTime.Format("Jan 02 15:04"),
			})
		}
		w := cmd.OutOrStdout()
		output.WriteTable(w, table)
		fmt.Fprintf(w, "\nTotal: %d window(s)\n", resp.Total)
	}

	return nil
}

func newMaintenanceShowCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <window-id>",
		Short: "Show maintenance window details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceShow(cmd, cfg, args[0])
		},
	}
}

func runMaintenanceShow(cmd *cobra.Command, cfg *Config, id string) error {
	client := newClient(cfg)
	win, err := client.GetWindow(id)
	if err != nil {
		return err
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(cmd.OutOrStdout(), win)
	case "yaml":
		return outputYAML(cmd.OutOrStdout(), win)
	default:
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "Maintenance Window: %s\n", win.Name)
		fmt.Fprintf(w, "  ID:          %s\n", win.ID)
		fmt.Fprintf(w, "  Type:        %s\n", win.Type)
		fmt.Fprintf(w, "  Status:      %s\n", win.Status)
		if win.Description != "" {
			fmt.Fprintf(w, "  Description: %s\n", win.Description)
		}
		fmt.Fprintf(w, "  Start:       %s\n", win.StartTime.Format(time.RFC3339))
		fmt.Fprintf(w, "  End:         %s\n", win.EndTime.Format(time.RFC3339))
		if win.Timezone != "" {
			fmt.Fprintf(w, "  Timezone:    %s\n", win.Timezone)
		}
		fmt.Fprintf(w, "  Approval:    %v\n", win.RequireApproval)
		fmt.Fprintf(w, "  Created:     %s", win.CreatedAt.Format(time.RFC3339))
		if win.CreatedBy != "" {
			fmt.Fprintf(w, " by %s", win.CreatedBy)
		}
		fmt.Fprintln(w)
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

func newMaintenanceCreateCmd(cfg *Config) *cobra.Command {
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
			return runMaintenanceCreate(cmd, cfg, opts)
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

func runMaintenanceCreate(cmd *cobra.Command, cfg *Config, opts *MaintenanceCreateOptions) error {
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

	startTime := opts.StartTime
	if strings.EqualFold(startTime, "now") {
		startTime = time.Now().UTC().Format(time.RFC3339)
	}

	client := newClient(cfg)
	req := &apischedule.CreateWindowRequest{
		Name:            opts.Name,
		Description:     opts.Description,
		Type:            opts.Type,
		StartTime:       startTime,
		EndTime:         opts.EndTime,
		Timezone:        opts.Timezone,
		RequireApproval: opts.RequireApproval,
		Labels:          parseLabels(opts.Labels),
	}

	resp, err := client.CreateWindow(req)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created maintenance window '%s' (id: %s)\n", resp.Name, resp.ID)
	return nil
}

func newMaintenanceStartCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start <window-id>",
		Short: "Start a scheduled maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.StartWindow(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Started maintenance window %s\n", args[0])
			return nil
		},
	}
}

func newMaintenanceEndCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "end <window-id>",
		Short: "End an active maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.EndWindow(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Ended maintenance window %s\n", args[0])
			return nil
		},
	}
}

func newMaintenanceCancelCmd(cfg *Config) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "cancel <window-id>",
		Short: "Cancel a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			if err := client.CancelWindow(args[0], reason); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Cancelled maintenance window %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")

	return cmd
}

func newMaintenanceExtendCmd(cfg *Config) *cobra.Command {
	var newEndTime string
	var duration string

	cmd := &cobra.Command{
		Use:   "extend <window-id>",
		Short: "Extend a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newEndTime == "" && duration == "" {
				return fmt.Errorf("either --end or --duration is required")
			}
			client := newClient(cfg)
			req := &apischedule.ExtendWindowRequest{
				EndTime:  newEndTime,
				Duration: duration,
			}
			if err := client.ExtendWindow(args[0], req); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Extended maintenance window %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&newEndTime, "end", "", "New end time (RFC3339 format)")
	cmd.Flags().StringVar(&duration, "duration", "", "Extend by duration (e.g., 1h, 30m)")

	return cmd
}

func newMaintenanceActiveCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "List currently active maintenance windows",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			resp, err := client.GetActiveWindows()
			if err != nil {
				return err
			}

			if len(resp.Windows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No active maintenance windows")
				return nil
			}

			table := &output.Table{
				Headers: []string{"ID", "NAME", "STARTED", "ENDS"},
			}
			for i := range resp.Windows {
				win := &resp.Windows[i]
				table.Rows = append(table.Rows, []string{
					truncate(win.ID, 8),
					truncate(win.Name, 20),
					win.StartTime.Format("Jan 02 15:04"),
					win.EndTime.Format("Jan 02 15:04"),
				})
			}
			output.WriteTable(cmd.OutOrStdout(), table)
			return nil
		},
	}
}

func newMaintenanceUpcomingCmd(cfg *Config) *cobra.Command {
	var within string

	cmd := &cobra.Command{
		Use:   "upcoming",
		Short: "List upcoming maintenance windows",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			resp, err := client.ListWindows("scheduled", "", 0)
			if err != nil {
				return err
			}

			upcoming := resp.Windows
			if len(upcoming) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No upcoming maintenance windows")
				return nil
			}

			table := &output.Table{
				Headers: []string{"ID", "NAME", "TYPE", "STARTS", "ENDS"},
			}
			for i := range upcoming {
				win := &upcoming[i]
				table.Rows = append(table.Rows, []string{
					truncate(win.ID, 8),
					truncate(win.Name, 20),
					win.Type,
					win.StartTime.Format("Jan 02 15:04"),
					win.EndTime.Format("Jan 02 15:04"),
				})
			}
			output.WriteTable(cmd.OutOrStdout(), table)
			return nil
		},
	}

	cmd.Flags().StringVar(&within, "within", "24h", "Show windows starting within (e.g., 24h, 7d)")

	return cmd
}

func newMaintenanceConflictsCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts <window-id>",
		Short: "Check for conflicts with other windows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newClient(cfg)
			conflicts, err := client.GetConflicts(args[0])
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Checking conflicts for window %s...\n\n", args[0])
			if len(conflicts) == 0 {
				fmt.Fprintln(w, "No conflicts found")
				return nil
			}

			table := &output.Table{
				Headers: []string{"TYPE", "CONFLICTING WINDOW", "SEVERITY", "MESSAGE"},
			}
			for _, c := range conflicts {
				table.Rows = append(table.Rows, []string{
					c.ConflictType,
					c.ConflictingID,
					c.Severity,
					c.Message,
				})
			}
			output.WriteTable(w, table)
			return nil
		},
	}
}

func newMaintenanceDeleteCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <window-id>",
		Short: "Delete a maintenance window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprintf(cmd.OutOrStdout(), "Are you sure you want to delete window %s? (use --force to confirm)\n", args[0])
				return nil
			}
			client := newClient(cfg)
			if err := client.DeleteWindow(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted maintenance window %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion without confirmation")

	return cmd
}

// =============================================================================
// Helpers
// =============================================================================

func outputJSON(w interface{ Write([]byte) (int, error) }, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func outputYAML(w interface{ Write([]byte) (int, error) }, v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Fprint(w, string(data))
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
