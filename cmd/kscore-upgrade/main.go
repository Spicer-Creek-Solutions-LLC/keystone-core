package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	// Global flags
	serverAddr   string
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

// Config holds CLI configuration
type Config struct {
	ServerAddr   string
	OutputFormat string
	Verbose      bool
}

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-upgrade", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-upgrade",
		Short: "Keystone Core upgrade management plugin",
		Long: `kscore-upgrade is a CLI plugin for managing Keystone Core upgrades.

This plugin provides commands for:
  - Checking available upgrades
  - Planning and executing upgrades
  - Managing upgrade strategies (rolling, canary)
  - Upgrading agents across the fleet
  - Rolling back failed upgrades

Usage via kscorectl:
  kscorectl upgrade check
  kscorectl upgrade plan --target 1.6.0
  kscorectl upgrade execute --target 1.6.0
  kscorectl upgrade status`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	cfg := &Config{
		ServerAddr:   serverAddr,
		OutputFormat: outputFormat,
		Verbose:      verbose,
	}

	// Add subcommands
	rootCmd.AddCommand(
		newCheckCmd(cfg),
		newPlanCmd(cfg),
		newExecuteCmd(cfg),
		newStatusCmd(cfg),
		newCancelCmd(cfg),
		newCanaryCmd(cfg),
		newAgentsCmd(cfg),
		newRollbackCmd(cfg),
		newHistoryCmd(cfg),
		newLogsCmd(cfg),
		newVersionCmd(),
	)

	return rootCmd
}

// UpgradeCheck represents upgrade availability check result
type UpgradeCheck struct {
	CurrentVersion  string            `json:"current_version" yaml:"current_version"`
	LatestVersion   string            `json:"latest_version" yaml:"latest_version"`
	UpgradeAvailable bool             `json:"upgrade_available" yaml:"upgrade_available"`
	TargetVersion   string            `json:"target_version,omitempty" yaml:"target_version,omitempty"`
	Compatible      bool              `json:"compatible" yaml:"compatible"`
	BreakingChanges []string          `json:"breaking_changes,omitempty" yaml:"breaking_changes,omitempty"`
	Prerequisites   []string          `json:"prerequisites,omitempty" yaml:"prerequisites,omitempty"`
	ReleaseNotes    string            `json:"release_notes,omitempty" yaml:"release_notes,omitempty"`
	Components      []ComponentStatus `json:"components" yaml:"components"`
}

// ComponentStatus represents a component's upgrade status
type ComponentStatus struct {
	Name           string `json:"name" yaml:"name"`
	CurrentVersion string `json:"current_version" yaml:"current_version"`
	TargetVersion  string `json:"target_version" yaml:"target_version"`
	Status         string `json:"status" yaml:"status"`
	UpgradeNeeded  bool   `json:"upgrade_needed" yaml:"upgrade_needed"`
}

// UpgradePlan represents an upgrade plan
type UpgradePlan struct {
	PlanID         string         `json:"plan_id" yaml:"plan_id"`
	CurrentVersion string         `json:"current_version" yaml:"current_version"`
	TargetVersion  string         `json:"target_version" yaml:"target_version"`
	Strategy       string         `json:"strategy" yaml:"strategy"`
	Steps          []UpgradeStep  `json:"steps" yaml:"steps"`
	EstimatedTime  string         `json:"estimated_time" yaml:"estimated_time"`
	RiskLevel      string         `json:"risk_level" yaml:"risk_level"`
	Backups        bool           `json:"backups" yaml:"backups"`
	CreatedAt      string         `json:"created_at" yaml:"created_at"`
}

// UpgradeStep represents a step in the upgrade plan
type UpgradeStep struct {
	Order       int      `json:"order" yaml:"order"`
	Component   string   `json:"component" yaml:"component"`
	Action      string   `json:"action" yaml:"action"`
	Description string   `json:"description" yaml:"description"`
	Duration    string   `json:"duration" yaml:"duration"`
	Rollback    string   `json:"rollback,omitempty" yaml:"rollback,omitempty"`
}

// UpgradeStatus represents upgrade execution status
type UpgradeStatus struct {
	UpgradeID       string            `json:"upgrade_id" yaml:"upgrade_id"`
	Status          string            `json:"status" yaml:"status"`
	Phase           string            `json:"phase" yaml:"phase"`
	CurrentVersion  string            `json:"current_version" yaml:"current_version"`
	TargetVersion   string            `json:"target_version" yaml:"target_version"`
	Strategy        string            `json:"strategy" yaml:"strategy"`
	Progress        int               `json:"progress" yaml:"progress"`
	StartedAt       string            `json:"started_at" yaml:"started_at"`
	CompletedAt     string            `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	CurrentStep     string            `json:"current_step" yaml:"current_step"`
	StepsCompleted  int               `json:"steps_completed" yaml:"steps_completed"`
	StepsTotal      int               `json:"steps_total" yaml:"steps_total"`
	Components      []ComponentStatus `json:"components" yaml:"components"`
	CanaryStatus    *CanaryStatus     `json:"canary_status,omitempty" yaml:"canary_status,omitempty"`
	Error           string            `json:"error,omitempty" yaml:"error,omitempty"`
}

// CanaryStatus represents canary deployment status
type CanaryStatus struct {
	Phase           string  `json:"phase" yaml:"phase"`
	PercentComplete int     `json:"percent_complete" yaml:"percent_complete"`
	HealthyReplicas int     `json:"healthy_replicas" yaml:"healthy_replicas"`
	TotalReplicas   int     `json:"total_replicas" yaml:"total_replicas"`
	SuccessRate     float64 `json:"success_rate" yaml:"success_rate"`
	CanPromote      bool    `json:"can_promote" yaml:"can_promote"`
	CanRollback     bool    `json:"can_rollback" yaml:"can_rollback"`
}

// AgentUpgradeStatus represents agent fleet upgrade status
type AgentUpgradeStatus struct {
	TargetVersion   string        `json:"target_version" yaml:"target_version"`
	TotalAgents     int           `json:"total_agents" yaml:"total_agents"`
	Upgraded        int           `json:"upgraded" yaml:"upgraded"`
	Pending         int           `json:"pending" yaml:"pending"`
	InProgress      int           `json:"in_progress" yaml:"in_progress"`
	Failed          int           `json:"failed" yaml:"failed"`
	Progress        int           `json:"progress" yaml:"progress"`
	BatchSize       int           `json:"batch_size" yaml:"batch_size"`
	CurrentBatch    int           `json:"current_batch" yaml:"current_batch"`
	TotalBatches    int           `json:"total_batches" yaml:"total_batches"`
	AgentDetails    []AgentDetail `json:"agent_details,omitempty" yaml:"agent_details,omitempty"`
}

// AgentDetail represents individual agent upgrade detail
type AgentDetail struct {
	AgentID        string `json:"agent_id" yaml:"agent_id"`
	Hostname       string `json:"hostname" yaml:"hostname"`
	CurrentVersion string `json:"current_version" yaml:"current_version"`
	TargetVersion  string `json:"target_version" yaml:"target_version"`
	Status         string `json:"status" yaml:"status"`
	Error          string `json:"error,omitempty" yaml:"error,omitempty"`
}

// UpgradeHistory represents an upgrade history entry
type UpgradeHistory struct {
	UpgradeID     string `json:"upgrade_id" yaml:"upgrade_id"`
	FromVersion   string `json:"from_version" yaml:"from_version"`
	ToVersion     string `json:"to_version" yaml:"to_version"`
	Strategy      string `json:"strategy" yaml:"strategy"`
	Status        string `json:"status" yaml:"status"`
	StartedAt     string `json:"started_at" yaml:"started_at"`
	CompletedAt   string `json:"completed_at" yaml:"completed_at"`
	Duration      string `json:"duration" yaml:"duration"`
	InitiatedBy   string `json:"initiated_by" yaml:"initiated_by"`
}

func newCheckCmd(cfg *Config) *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for available upgrades",
		Long: `Check for available upgrades and compatibility.

Examples:
  # Check for any available upgrades
  kscorectl upgrade check

  # Check compatibility with specific version
  kscorectl upgrade check --target 2.0.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cfg, target)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Check compatibility with specific target version")

	return cmd
}

func runCheck(cfg *Config, target string) error {
	check := UpgradeCheck{
		CurrentVersion:   "1.5.0",
		LatestVersion:    "1.6.0",
		UpgradeAvailable: true,
		TargetVersion:    target,
		Compatible:       true,
		BreakingChanges:  []string{},
		Prerequisites: []string{
			"Backup database before upgrade",
			"Ensure all agents are healthy",
		},
		ReleaseNotes: "https://github.com/shawnbutts/keystone-core/releases/tag/v1.6.0",
		Components: []ComponentStatus{
			{Name: "server", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "upgrade_available", UpgradeNeeded: true},
			{Name: "agent", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "upgrade_available", UpgradeNeeded: true},
			{Name: "nats", CurrentVersion: "2.10.0", TargetVersion: "2.10.0", Status: "up_to_date", UpgradeNeeded: false},
			{Name: "etcd", CurrentVersion: "3.5.10", TargetVersion: "3.5.10", Status: "up_to_date", UpgradeNeeded: false},
		},
	}

	if target != "" {
		check.TargetVersion = target
		if target == "2.0.0" {
			check.BreakingChanges = []string{
				"API v1 deprecated, use v2",
				"Config file format changed",
			}
		}
	}

	return outputResult(cfg.OutputFormat, check, func() {
		fmt.Printf("Upgrade Check\n")
		fmt.Printf("=============\n\n")
		fmt.Printf("Current Version: %s\n", check.CurrentVersion)
		fmt.Printf("Latest Version:  %s\n", check.LatestVersion)
		if check.TargetVersion != "" {
			fmt.Printf("Target Version:  %s\n", check.TargetVersion)
		}
		fmt.Printf("Upgrade Available: %v\n", check.UpgradeAvailable)
		fmt.Printf("Compatible: %v\n", check.Compatible)

		if len(check.BreakingChanges) > 0 {
			fmt.Printf("\n⚠️  Breaking Changes:\n")
			for _, bc := range check.BreakingChanges {
				fmt.Printf("  - %s\n", bc)
			}
		}

		if len(check.Prerequisites) > 0 {
			fmt.Printf("\nPrerequisites:\n")
			for _, p := range check.Prerequisites {
				fmt.Printf("  - %s\n", p)
			}
		}

		fmt.Printf("\nComponent Status:\n")
		table := &output.Table{
			Headers: []string{"COMPONENT", "CURRENT", "TARGET", "STATUS"},
		}
		for _, c := range check.Components {
			icon := "✓"
			if c.UpgradeNeeded {
				icon = "↑"
			}
			table.Rows = append(table.Rows, []string{
				c.Name,
				c.CurrentVersion,
				c.TargetVersion,
				fmt.Sprintf("%s %s", icon, c.Status),
			})
		}
		output.WriteTable(os.Stdout, table)

		if check.ReleaseNotes != "" {
			fmt.Printf("\nRelease Notes: %s\n", check.ReleaseNotes)
		}
	})
}

func newPlanCmd(cfg *Config) *cobra.Command {
	var (
		target   string
		strategy string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan an upgrade",
		Long: `Create an upgrade plan for the specified target version.

Strategies:
  - rolling: Upgrade nodes one at a time (default)
  - canary: Gradual rollout with metrics-based promotion
  - blue-green: Deploy new version alongside old, then switch

Examples:
  # Plan upgrade to specific version
  kscorectl upgrade plan --target 1.6.0

  # Plan with canary strategy
  kscorectl upgrade plan --target 1.6.0 --strategy canary`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(cfg, target, strategy)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version (required)")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Upgrade strategy (rolling, canary, blue-green)")
	cmd.MarkFlagRequired("target")

	return cmd
}

func runPlan(cfg *Config, target, strategy string) error {
	plan := UpgradePlan{
		PlanID:         fmt.Sprintf("plan-%s", time.Now().Format("20060102-150405")),
		CurrentVersion: "1.5.0",
		TargetVersion:  target,
		Strategy:       strategy,
		EstimatedTime:  "15m",
		RiskLevel:      "low",
		Backups:        true,
		CreatedAt:      time.Now().Format(time.RFC3339),
		Steps: []UpgradeStep{
			{Order: 1, Component: "pre-flight", Action: "verify", Description: "Verify cluster health and prerequisites", Duration: "1m"},
			{Order: 2, Component: "backup", Action: "create", Description: "Create backup of current state", Duration: "3m", Rollback: "Delete backup"},
			{Order: 3, Component: "etcd", Action: "snapshot", Description: "Snapshot etcd data", Duration: "1m", Rollback: "Restore etcd snapshot"},
			{Order: 4, Component: "server", Action: "upgrade", Description: "Upgrade control plane servers", Duration: "5m", Rollback: "Rollback server binaries"},
			{Order: 5, Component: "nats", Action: "verify", Description: "Verify NATS connectivity", Duration: "1m"},
			{Order: 6, Component: "verify", Action: "health", Description: "Verify cluster health", Duration: "2m"},
			{Order: 7, Component: "agents", Action: "upgrade", Description: "Trigger agent upgrades", Duration: "2m", Rollback: "Rollback agent binaries"},
		},
	}

	if strategy == "canary" {
		plan.EstimatedTime = "30m"
		plan.Steps = append(plan.Steps[:6], UpgradeStep{
			Order: 7, Component: "canary", Action: "deploy", Description: "Deploy canary instances", Duration: "5m",
		}, UpgradeStep{
			Order: 8, Component: "canary", Action: "monitor", Description: "Monitor canary metrics", Duration: "10m",
		}, UpgradeStep{
			Order: 9, Component: "canary", Action: "promote", Description: "Promote canary to full rollout", Duration: "5m",
		})
	}

	return outputResult(cfg.OutputFormat, plan, func() {
		fmt.Printf("Upgrade Plan\n")
		fmt.Printf("============\n\n")
		fmt.Printf("Plan ID:        %s\n", plan.PlanID)
		fmt.Printf("Current:        %s → %s\n", plan.CurrentVersion, plan.TargetVersion)
		fmt.Printf("Strategy:       %s\n", plan.Strategy)
		fmt.Printf("Estimated Time: %s\n", plan.EstimatedTime)
		fmt.Printf("Risk Level:     %s\n", plan.RiskLevel)
		fmt.Printf("Auto Backup:    %v\n", plan.Backups)

		fmt.Printf("\nUpgrade Steps:\n")
		for _, s := range plan.Steps {
			fmt.Printf("  %d. [%s] %s (%s)\n", s.Order, s.Component, s.Description, s.Duration)
			if s.Rollback != "" {
				fmt.Printf("     ↩ Rollback: %s\n", s.Rollback)
			}
		}

		fmt.Printf("\nTo execute this plan, run:\n")
		fmt.Printf("  kscorectl upgrade execute --target %s --strategy %s\n", target, strategy)
	})
}

func newExecuteCmd(cfg *Config) *cobra.Command {
	var (
		target       string
		strategy     string
		skipBackup   bool
		force        bool
		maxUnavailable int
	)

	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute an upgrade",
		Long: `Execute an upgrade to the specified target version.

Examples:
  # Execute upgrade with default strategy
  kscorectl upgrade execute --target 1.6.0

  # Execute with canary strategy
  kscorectl upgrade execute --target 1.6.0 --strategy canary

  # Execute with custom rolling parameters
  kscorectl upgrade execute --target 1.6.0 --strategy rolling --max-unavailable 2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecute(cfg, target, strategy, skipBackup, force, maxUnavailable)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version (required)")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Upgrade strategy (rolling, canary, blue-green)")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "Skip automatic backup before upgrade")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force upgrade even with warnings")
	cmd.Flags().IntVar(&maxUnavailable, "max-unavailable", 1, "Maximum unavailable nodes during rolling upgrade")
	cmd.MarkFlagRequired("target")

	return cmd
}

func runExecute(cfg *Config, target, strategy string, skipBackup, force bool, maxUnavailable int) error {
	status := UpgradeStatus{
		UpgradeID:      fmt.Sprintf("upgrade-%s", time.Now().Format("20060102-150405")),
		Status:         "in_progress",
		Phase:          "upgrading",
		CurrentVersion: "1.5.0",
		TargetVersion:  target,
		Strategy:       strategy,
		Progress:       35,
		StartedAt:      time.Now().Format(time.RFC3339),
		CurrentStep:    "Upgrading control plane servers",
		StepsCompleted: 3,
		StepsTotal:     7,
		Components: []ComponentStatus{
			{Name: "pre-flight", CurrentVersion: "1.5.0", TargetVersion: target, Status: "completed"},
			{Name: "backup", CurrentVersion: "1.5.0", TargetVersion: target, Status: "completed"},
			{Name: "etcd", CurrentVersion: "1.5.0", TargetVersion: target, Status: "completed"},
			{Name: "server", CurrentVersion: "1.5.0", TargetVersion: target, Status: "upgrading"},
			{Name: "nats", CurrentVersion: "1.5.0", TargetVersion: target, Status: "pending"},
			{Name: "verify", CurrentVersion: "1.5.0", TargetVersion: target, Status: "pending"},
			{Name: "agents", CurrentVersion: "1.5.0", TargetVersion: target, Status: "pending"},
		},
	}

	if strategy == "canary" {
		status.CanaryStatus = &CanaryStatus{
			Phase:           "deploying",
			PercentComplete: 10,
			HealthyReplicas: 1,
			TotalReplicas:   3,
			SuccessRate:     100.0,
			CanPromote:      false,
			CanRollback:     true,
		}
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Upgrade Started\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("Upgrade ID: %s\n", status.UpgradeID)
		fmt.Printf("Target:     %s → %s\n", status.CurrentVersion, status.TargetVersion)
		fmt.Printf("Strategy:   %s\n", status.Strategy)
		fmt.Printf("Status:     %s\n", status.Status)
		fmt.Printf("Progress:   %d%% (%d/%d steps)\n", status.Progress, status.StepsCompleted, status.StepsTotal)
		fmt.Printf("Current:    %s\n", status.CurrentStep)

		fmt.Printf("\nTo monitor progress, run:\n")
		fmt.Printf("  kscorectl upgrade status --watch\n")
	})
}

func newStatusCmd(cfg *Config) *cobra.Command {
	var watch bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show upgrade status",
		Long: `Show the status of the current or most recent upgrade.

Examples:
  # Show current status
  kscorectl upgrade status

  # Watch status in real-time
  kscorectl upgrade status --watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cfg, watch)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch status updates in real-time")

	return cmd
}

func runStatus(cfg *Config, watch bool) error {
	status := UpgradeStatus{
		UpgradeID:      "upgrade-20240115-100000",
		Status:         "in_progress",
		Phase:          "upgrading",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Strategy:       "rolling",
		Progress:       65,
		StartedAt:      "2024-01-15T10:00:00Z",
		CurrentStep:    "Verifying cluster health",
		StepsCompleted: 5,
		StepsTotal:     7,
		Components: []ComponentStatus{
			{Name: "pre-flight", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "completed"},
			{Name: "backup", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "completed"},
			{Name: "etcd", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "completed"},
			{Name: "server", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "completed"},
			{Name: "nats", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "completed"},
			{Name: "verify", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "in_progress"},
			{Name: "agents", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "pending"},
		},
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Upgrade Status\n")
		fmt.Printf("==============\n\n")
		fmt.Printf("Upgrade ID: %s\n", status.UpgradeID)
		fmt.Printf("Status:     %s\n", status.Status)
		fmt.Printf("Phase:      %s\n", status.Phase)
		fmt.Printf("Version:    %s → %s\n", status.CurrentVersion, status.TargetVersion)
		fmt.Printf("Strategy:   %s\n", status.Strategy)
		fmt.Printf("Progress:   %d%% (%d/%d steps)\n", status.Progress, status.StepsCompleted, status.StepsTotal)
		fmt.Printf("Started:    %s\n", status.StartedAt)
		fmt.Printf("Current:    %s\n", status.CurrentStep)

		fmt.Printf("\nComponent Status:\n")
		table := &output.Table{
			Headers: []string{"COMPONENT", "CURRENT", "TARGET", "STATUS"},
		}
		for _, c := range status.Components {
			icon := "○"
			switch c.Status {
			case "completed":
				icon = "✓"
			case "in_progress":
				icon = "◐"
			case "failed":
				icon = "✗"
			}
			table.Rows = append(table.Rows, []string{
				c.Name,
				c.CurrentVersion,
				c.TargetVersion,
				fmt.Sprintf("%s %s", icon, c.Status),
			})
		}
		output.WriteTable(os.Stdout, table)

		if status.CanaryStatus != nil {
			fmt.Printf("\nCanary Status:\n")
			fmt.Printf("  Phase:     %s\n", status.CanaryStatus.Phase)
			fmt.Printf("  Progress:  %d%%\n", status.CanaryStatus.PercentComplete)
			fmt.Printf("  Replicas:  %d/%d healthy\n", status.CanaryStatus.HealthyReplicas, status.CanaryStatus.TotalReplicas)
			fmt.Printf("  Success:   %.1f%%\n", status.CanaryStatus.SuccessRate)
		}

		if watch {
			fmt.Printf("\n(Press Ctrl+C to stop watching)\n")
		}
	})
}

func newCancelCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel an in-progress upgrade",
		Long: `Cancel an in-progress upgrade and trigger rollback if needed.

Examples:
  # Cancel current upgrade
  kscorectl upgrade cancel

  # Force cancel without confirmation
  kscorectl upgrade cancel --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCancel(cfg, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force cancel without confirmation")

	return cmd
}

func runCancel(cfg *Config, force bool) error {
	fmt.Printf("Cancelling upgrade: upgrade-20240115-100000\n")
	fmt.Printf("Status: Rollback initiated\n")
	fmt.Printf("\nThe upgrade has been cancelled and rollback is in progress.\n")
	fmt.Printf("Use 'kscorectl upgrade status' to monitor rollback progress.\n")
	return nil
}

func newCanaryCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "canary",
		Short: "Manage canary deployments",
		Long:  `Manage canary deployment operations during an upgrade.`,
	}

	cmd.AddCommand(
		newCanaryPromoteCmd(cfg),
		newCanaryRollbackCmd(cfg),
		newCanaryStatusCmd(cfg),
	)

	return cmd
}

func newCanaryPromoteCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote canary to full rollout",
		Long: `Promote the canary deployment to a full rollout.

This will complete the upgrade by rolling out to all remaining instances.

Examples:
  kscorectl upgrade canary promote`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Promoting canary deployment...\n")
			fmt.Printf("Status: Full rollout initiated\n")
			fmt.Printf("\nThe canary deployment has been promoted to full rollout.\n")
			fmt.Printf("Use 'kscorectl upgrade status' to monitor progress.\n")
			return nil
		},
	}

	return cmd
}

func newCanaryRollbackCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback canary deployment",
		Long: `Rollback the canary deployment to the previous version.

This will stop the canary deployment and revert to the previous stable version.

Examples:
  kscorectl upgrade canary rollback`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Rolling back canary deployment...\n")
			fmt.Printf("Status: Canary rollback initiated\n")
			fmt.Printf("\nThe canary deployment is being rolled back.\n")
			fmt.Printf("Use 'kscorectl upgrade status' to monitor progress.\n")
			return nil
		},
	}

	return cmd
}

func newCanaryStatusCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show canary deployment status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status := CanaryStatus{
				Phase:           "monitoring",
				PercentComplete: 25,
				HealthyReplicas: 1,
				TotalReplicas:   4,
				SuccessRate:     99.5,
				CanPromote:      true,
				CanRollback:     true,
			}

			return outputResult(cfg.OutputFormat, status, func() {
				fmt.Printf("Canary Deployment Status\n")
				fmt.Printf("========================\n\n")
				fmt.Printf("Phase:           %s\n", status.Phase)
				fmt.Printf("Progress:        %d%%\n", status.PercentComplete)
				fmt.Printf("Healthy:         %d/%d replicas\n", status.HealthyReplicas, status.TotalReplicas)
				fmt.Printf("Success Rate:    %.1f%%\n", status.SuccessRate)
				fmt.Printf("Can Promote:     %v\n", status.CanPromote)
				fmt.Printf("Can Rollback:    %v\n", status.CanRollback)

				if status.CanPromote {
					fmt.Printf("\nTo promote: kscorectl upgrade canary promote\n")
				}
				if status.CanRollback {
					fmt.Printf("To rollback: kscorectl upgrade canary rollback\n")
				}
			})
		},
	}

	return cmd
}

func newAgentsCmd(cfg *Config) *cobra.Command {
	var (
		target    string
		batchSize int
		filter    string
		report    bool
	)

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Upgrade agents across the fleet",
		Long: `Upgrade agents across the fleet to the specified version.

Examples:
  # Upgrade all agents
  kscorectl upgrade agents --target 1.6.0

  # Upgrade with batch size and filter
  kscorectl upgrade agents --target 1.6.0 --batch-size 10 --filter "environment:production"

  # Show agent upgrade report
  kscorectl upgrade agents --report`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if report {
				return runAgentReport(cfg)
			}
			return runAgentUpgrade(cfg, target, batchSize, filter)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version for agents")
	cmd.Flags().IntVar(&batchSize, "batch-size", 5, "Number of agents to upgrade in parallel")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter expression for agent selection")
	cmd.Flags().BoolVar(&report, "report", false, "Show agent version report")

	return cmd
}

func runAgentUpgrade(cfg *Config, target string, batchSize int, filter string) error {
	status := AgentUpgradeStatus{
		TargetVersion: target,
		TotalAgents:   100,
		Upgraded:      45,
		Pending:       50,
		InProgress:    5,
		Failed:        0,
		Progress:      45,
		BatchSize:     batchSize,
		CurrentBatch:  10,
		TotalBatches:  20,
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Agent Fleet Upgrade\n")
		fmt.Printf("===================\n\n")
		fmt.Printf("Target Version: %s\n", status.TargetVersion)
		fmt.Printf("Progress:       %d%% (%d/%d agents)\n", status.Progress, status.Upgraded, status.TotalAgents)
		fmt.Printf("Batch:          %d/%d (size: %d)\n", status.CurrentBatch, status.TotalBatches, status.BatchSize)
		fmt.Printf("\nStatus:\n")
		fmt.Printf("  Upgraded:    %d\n", status.Upgraded)
		fmt.Printf("  In Progress: %d\n", status.InProgress)
		fmt.Printf("  Pending:     %d\n", status.Pending)
		fmt.Printf("  Failed:      %d\n", status.Failed)
	})
}

func runAgentReport(cfg *Config) error {
	agents := []AgentDetail{
		{AgentID: "agent-001", Hostname: "web-01", CurrentVersion: "1.6.0", TargetVersion: "1.6.0", Status: "current"},
		{AgentID: "agent-002", Hostname: "web-02", CurrentVersion: "1.6.0", TargetVersion: "1.6.0", Status: "current"},
		{AgentID: "agent-003", Hostname: "db-01", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "upgrade_available"},
		{AgentID: "agent-004", Hostname: "db-02", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "upgrade_available"},
		{AgentID: "agent-005", Hostname: "cache-01", CurrentVersion: "1.4.0", TargetVersion: "1.6.0", Status: "upgrade_available"},
	}

	return outputResult(cfg.OutputFormat, agents, func() {
		fmt.Printf("Agent Version Report\n")
		fmt.Printf("====================\n\n")

		table := &output.Table{
			Headers: []string{"AGENT ID", "HOSTNAME", "CURRENT", "TARGET", "STATUS"},
		}
		for _, a := range agents {
			icon := "✓"
			if a.Status == "upgrade_available" {
				icon = "↑"
			} else if a.Status == "failed" {
				icon = "✗"
			}
			table.Rows = append(table.Rows, []string{
				a.AgentID,
				a.Hostname,
				a.CurrentVersion,
				a.TargetVersion,
				fmt.Sprintf("%s %s", icon, a.Status),
			})
		}
		output.WriteTable(os.Stdout, table)

		// Summary
		current := 0
		needUpgrade := 0
		for _, a := range agents {
			if a.Status == "current" {
				current++
			} else {
				needUpgrade++
			}
		}
		fmt.Printf("\nSummary: %d current, %d need upgrade\n", current, needUpgrade)
	})
}

func newRollbackCmd(cfg *Config) *cobra.Command {
	var (
		target     string
		status     bool
		components []string
		dryRun     bool
		force      bool
		cancel     bool
	)

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback to a previous version",
		Long: `Rollback the cluster to a previous version.

Examples:
  # Rollback to previous version
  kscorectl upgrade rollback

  # Rollback to specific version
  kscorectl upgrade rollback --target 1.5.0

  # Show rollback status
  kscorectl upgrade rollback --status

  # Dry-run rollback
  kscorectl upgrade rollback --dry-run

  # Rollback specific components
  kscorectl upgrade rollback --components server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if status {
				return runRollbackStatus(cfg)
			}
			if cancel {
				fmt.Printf("Rollback cancelled\n")
				return nil
			}
			return runRollback(cfg, target, components, dryRun, force)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version for rollback")
	cmd.Flags().BoolVar(&status, "status", false, "Show rollback status")
	cmd.Flags().StringSliceVarP(&components, "components", "c", nil, "Specific components to rollback")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be rolled back")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force rollback without confirmation")
	cmd.Flags().BoolVar(&cancel, "cancel", false, "Cancel in-progress rollback")

	return cmd
}

func runRollback(cfg *Config, target string, components []string, dryRun, force bool) error {
	if target == "" {
		target = "1.5.0" // Previous version
	}

	if dryRun {
		fmt.Printf("Rollback Dry-Run\n")
		fmt.Printf("================\n\n")
		fmt.Printf("Target Version: %s\n", target)
		fmt.Printf("\nWould rollback:\n")
		if len(components) > 0 {
			for _, c := range components {
				fmt.Printf("  - %s\n", c)
			}
		} else {
			fmt.Printf("  - server (1.6.0 → %s)\n", target)
			fmt.Printf("  - agents (1.6.0 → %s)\n", target)
		}
		fmt.Printf("\nNo changes made (dry-run mode)\n")
		return nil
	}

	fmt.Printf("Initiating rollback to version %s...\n", target)
	fmt.Printf("Status: Rollback in progress\n")
	fmt.Printf("\nUse 'kscorectl upgrade rollback --status' to monitor progress.\n")
	return nil
}

func runRollbackStatus(cfg *Config) error {
	status := UpgradeStatus{
		UpgradeID:      "rollback-20240115-110000",
		Status:         "in_progress",
		Phase:          "rolling_back",
		CurrentVersion: "1.6.0",
		TargetVersion:  "1.5.0",
		Strategy:       "rolling",
		Progress:       60,
		StartedAt:      "2024-01-15T11:00:00Z",
		CurrentStep:    "Rolling back agents",
		StepsCompleted: 4,
		StepsTotal:     6,
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Rollback Status\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("Rollback ID: %s\n", status.UpgradeID)
		fmt.Printf("Status:      %s\n", status.Status)
		fmt.Printf("Version:     %s → %s\n", status.CurrentVersion, status.TargetVersion)
		fmt.Printf("Progress:    %d%% (%d/%d steps)\n", status.Progress, status.StepsCompleted, status.StepsTotal)
		fmt.Printf("Current:     %s\n", status.CurrentStep)
	})
}

func newHistoryCmd(cfg *Config) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show upgrade history",
		Long: `Show history of upgrade operations.

Examples:
  kscorectl upgrade history
  kscorectl upgrade history --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cfg, limit)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Maximum number of entries")

	return cmd
}

func runHistory(cfg *Config, limit int) error {
	history := []UpgradeHistory{
		{
			UpgradeID:   "upgrade-20240115-100000",
			FromVersion: "1.5.0",
			ToVersion:   "1.6.0",
			Strategy:    "rolling",
			Status:      "completed",
			StartedAt:   "2024-01-15T10:00:00Z",
			CompletedAt: "2024-01-15T10:15:00Z",
			Duration:    "15m",
			InitiatedBy: "admin",
		},
		{
			UpgradeID:   "upgrade-20240110-080000",
			FromVersion: "1.4.0",
			ToVersion:   "1.5.0",
			Strategy:    "canary",
			Status:      "completed",
			StartedAt:   "2024-01-10T08:00:00Z",
			CompletedAt: "2024-01-10T08:45:00Z",
			Duration:    "45m",
			InitiatedBy: "ci-pipeline",
		},
		{
			UpgradeID:   "upgrade-20240105-140000",
			FromVersion: "1.3.0",
			ToVersion:   "1.4.0",
			Strategy:    "rolling",
			Status:      "rolled_back",
			StartedAt:   "2024-01-05T14:00:00Z",
			CompletedAt: "2024-01-05T14:25:00Z",
			Duration:    "25m",
			InitiatedBy: "admin",
		},
	}

	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return outputResult(cfg.OutputFormat, history, func() {
		fmt.Printf("Upgrade History\n")
		fmt.Printf("===============\n\n")

		table := &output.Table{
			Headers: []string{"UPGRADE ID", "FROM", "TO", "STRATEGY", "STATUS", "DURATION", "STARTED"},
		}
		for _, h := range history {
			icon := "✓"
			if h.Status == "failed" {
				icon = "✗"
			} else if h.Status == "rolled_back" {
				icon = "↩"
			}
			table.Rows = append(table.Rows, []string{
				h.UpgradeID,
				h.FromVersion,
				h.ToVersion,
				h.Strategy,
				fmt.Sprintf("%s %s", icon, h.Status),
				h.Duration,
				h.StartedAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d upgrades\n", len(history))
	})
}

func newLogsCmd(cfg *Config) *cobra.Command {
	var (
		upgradeID string
		follow    bool
		tail      int
	)

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show upgrade logs",
		Long: `Show logs from an upgrade operation.

Examples:
  # Show logs for specific upgrade
  kscorectl upgrade logs --upgrade-id abc123

  # Follow logs in real-time
  kscorectl upgrade logs --upgrade-id abc123 --follow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cfg, upgradeID, follow, tail)
		},
	}

	cmd.Flags().StringVar(&upgradeID, "upgrade-id", "", "Upgrade ID (required)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow logs in real-time")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show")
	cmd.MarkFlagRequired("upgrade-id")

	return cmd
}

func runLogs(cfg *Config, upgradeID string, follow bool, tail int) error {
	logs := []string{
		"2024-01-15T10:00:00Z [INFO] Upgrade started: 1.5.0 → 1.6.0",
		"2024-01-15T10:00:01Z [INFO] Running pre-flight checks...",
		"2024-01-15T10:00:05Z [INFO] Pre-flight checks passed",
		"2024-01-15T10:00:06Z [INFO] Creating backup...",
		"2024-01-15T10:01:30Z [INFO] Backup created: backup-20240115-100006",
		"2024-01-15T10:01:31Z [INFO] Snapshotting etcd...",
		"2024-01-15T10:01:45Z [INFO] etcd snapshot completed",
		"2024-01-15T10:01:46Z [INFO] Upgrading control plane servers...",
		"2024-01-15T10:03:00Z [INFO] Server kscore-1 upgraded successfully",
		"2024-01-15T10:04:15Z [INFO] Server kscore-2 upgraded successfully",
		"2024-01-15T10:05:30Z [INFO] Server kscore-3 upgraded successfully",
		"2024-01-15T10:05:31Z [INFO] Verifying NATS connectivity...",
		"2024-01-15T10:05:45Z [INFO] NATS verification passed",
		"2024-01-15T10:05:46Z [INFO] Verifying cluster health...",
		"2024-01-15T10:06:00Z [INFO] Cluster health verified",
		"2024-01-15T10:06:01Z [INFO] Triggering agent upgrades...",
		"2024-01-15T10:10:00Z [INFO] Agent upgrades completed",
		"2024-01-15T10:10:01Z [INFO] Upgrade completed successfully",
	}

	fmt.Printf("Upgrade Logs: %s\n", upgradeID)
	fmt.Printf("%s\n\n", strings.Repeat("=", 40+len(upgradeID)))

	start := 0
	if tail > 0 && len(logs) > tail {
		start = len(logs) - tail
	}

	for _, log := range logs[start:] {
		fmt.Println(log)
	}

	if follow {
		fmt.Printf("\n(Following logs... Press Ctrl+C to stop)\n")
	}

	return nil
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

// Helper functions

func outputResult(format string, data interface{}, tableFunc func()) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(data)
	default:
		tableFunc()
		return nil
	}
}
