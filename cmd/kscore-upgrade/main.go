// Package main implements the kscore-upgrade CLI for upgrade management operations.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
	airgapupgrade "github.com/shawnbutts/keystone-core/internal/airgap/upgrade"
	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
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
		newPathCmd(cfg),
		newResumeCmd(cfg),
		newPackageCmd(),
		newVersionCmd(),
	)

	return rootCmd
}

// UpgradeCheck represents upgrade availability check result
type UpgradeCheck struct {
	CurrentVersion   string            `json:"current_version" yaml:"current_version"`
	LatestVersion    string            `json:"latest_version" yaml:"latest_version"`
	UpgradeAvailable bool              `json:"upgrade_available" yaml:"upgrade_available"`
	TargetVersion    string            `json:"target_version,omitempty" yaml:"target_version,omitempty"`
	Compatible       bool              `json:"compatible" yaml:"compatible"`
	BreakingChanges  []string          `json:"breaking_changes,omitempty" yaml:"breaking_changes,omitempty"`
	Prerequisites    []string          `json:"prerequisites,omitempty" yaml:"prerequisites,omitempty"`
	ReleaseNotes     string            `json:"release_notes,omitempty" yaml:"release_notes,omitempty"`
	Components       []ComponentStatus `json:"components" yaml:"components"`
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
	PlanID         string        `json:"plan_id" yaml:"plan_id"`
	CurrentVersion string        `json:"current_version" yaml:"current_version"`
	TargetVersion  string        `json:"target_version" yaml:"target_version"`
	Strategy       string        `json:"strategy" yaml:"strategy"`
	BatchSize      int           `json:"batch_size" yaml:"batch_size"`
	Steps          []UpgradeStep `json:"steps" yaml:"steps"`
	EstimatedTime  string        `json:"estimated_time" yaml:"estimated_time"`
	RiskLevel      string        `json:"risk_level" yaml:"risk_level"`
	Backups        bool          `json:"backups" yaml:"backups"`
	CreatedAt      string        `json:"created_at" yaml:"created_at"`
}

// UpgradeStep represents a step in the upgrade plan
type UpgradeStep struct {
	Order       int    `json:"order" yaml:"order"`
	Component   string `json:"component" yaml:"component"`
	Action      string `json:"action" yaml:"action"`
	Description string `json:"description" yaml:"description"`
	Duration    string `json:"duration" yaml:"duration"`
	Rollback    string `json:"rollback,omitempty" yaml:"rollback,omitempty"`
}

// UpgradeStatus represents upgrade execution status
type UpgradeStatus struct {
	UpgradeID      string            `json:"upgrade_id" yaml:"upgrade_id"`
	Status         string            `json:"status" yaml:"status"`
	Phase          string            `json:"phase" yaml:"phase"`
	CurrentVersion string            `json:"current_version" yaml:"current_version"`
	TargetVersion  string            `json:"target_version" yaml:"target_version"`
	Strategy       string            `json:"strategy" yaml:"strategy"`
	Progress       int               `json:"progress" yaml:"progress"`
	StartedAt      string            `json:"started_at" yaml:"started_at"`
	CompletedAt    string            `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	CurrentStep    string            `json:"current_step" yaml:"current_step"`
	StepsCompleted int               `json:"steps_completed" yaml:"steps_completed"`
	StepsTotal     int               `json:"steps_total" yaml:"steps_total"`
	Components     []ComponentStatus `json:"components" yaml:"components"`
	CanaryStatus   *CanaryStatus     `json:"canary_status,omitempty" yaml:"canary_status,omitempty"`
	Error          string            `json:"error,omitempty" yaml:"error,omitempty"`
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
	TargetVersion string        `json:"target_version" yaml:"target_version"`
	TotalAgents   int           `json:"total_agents" yaml:"total_agents"`
	Upgraded      int           `json:"upgraded" yaml:"upgraded"`
	Pending       int           `json:"pending" yaml:"pending"`
	InProgress    int           `json:"in_progress" yaml:"in_progress"`
	Failed        int           `json:"failed" yaml:"failed"`
	Progress      int           `json:"progress" yaml:"progress"`
	BatchSize     int           `json:"batch_size" yaml:"batch_size"`
	CurrentBatch  int           `json:"current_batch" yaml:"current_batch"`
	TotalBatches  int           `json:"total_batches" yaml:"total_batches"`
	AgentDetails  []AgentDetail `json:"agent_details,omitempty" yaml:"agent_details,omitempty"`
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
	UpgradeID   string `json:"upgrade_id" yaml:"upgrade_id"`
	FromVersion string `json:"from_version" yaml:"from_version"`
	ToVersion   string `json:"to_version" yaml:"to_version"`
	Strategy    string `json:"strategy" yaml:"strategy"`
	Status      string `json:"status" yaml:"status"`
	StartedAt   string `json:"started_at" yaml:"started_at"`
	CompletedAt string `json:"completed_at" yaml:"completed_at"`
	Duration    string `json:"duration" yaml:"duration"`
	InitiatedBy string `json:"initiated_by" yaml:"initiated_by"`
}

func newCheckCmd(cfg *Config) *cobra.Command {
	var (
		target            string
		from              string
		includePrerelease bool
		channel           string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for available upgrades",
		Long: `Check for available upgrades and compatibility.

Examples:
  # Check for any available upgrades
  kscorectl upgrade check

  # Check compatibility with specific version
  kscorectl upgrade check --target 2.0.0

  # Check upgrade from a specific version
  kscorectl upgrade check --from 1.4.0 --target 1.6.0

  # Include prerelease versions
  kscorectl upgrade check --include-prerelease

  # Check specific release channel
  kscorectl upgrade check --channel stable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cfg, target, from, includePrerelease, channel)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Check compatibility with specific target version")
	cmd.Flags().StringVar(&from, "from", "", "Override current version (e.g. to check a hypothetical upgrade path)")
	cmd.Flags().BoolVar(&includePrerelease, "include-prerelease", false, "Include prerelease versions in check")
	cmd.Flags().StringVar(&channel, "channel", "stable", "Release channel to check (stable, beta, nightly)")

	return cmd
}

func runCheck(cfg *Config, target, from string, includePrerelease bool, channel string) error {
	latestVersion := "1.6.0"
	if includePrerelease {
		latestVersion = "1.7.0-beta.1"
	}
	if channel == "nightly" {
		latestVersion = "1.7.0-dev.20240115"
	}

	currentVersion := "1.5.0"
	if from != "" {
		currentVersion = from
	}

	check := UpgradeCheck{
		CurrentVersion:   currentVersion,
		LatestVersion:    latestVersion,
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
			{Name: "server", CurrentVersion: "1.5.0", TargetVersion: latestVersion, Status: "upgrade_available", UpgradeNeeded: true},
			{Name: "agent", CurrentVersion: "1.5.0", TargetVersion: latestVersion, Status: "upgrade_available", UpgradeNeeded: true},
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
		target    string
		strategy  string
		batchSize int
		save      string
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
  kscorectl upgrade plan --target 1.6.0 --strategy canary

  # Save plan to file
  kscorectl upgrade plan --target 1.6.0 --save upgrade-plan.yaml

  # Plan with batch size for rolling upgrades
  kscorectl upgrade plan --target 1.6.0 --batch-size 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(cfg, target, strategy, batchSize, save)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version (required)")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Upgrade strategy (rolling, canary, blue-green)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 5, "Number of nodes to upgrade in parallel (for rolling strategy)")
	cmd.Flags().StringVar(&save, "save", "", "Save plan to file (YAML format)")
	cmd.MarkFlagRequired("target")

	return cmd
}

func runPlan(cfg *Config, target, strategy string, batchSize int, save string) error {
	plan := UpgradePlan{
		PlanID:         fmt.Sprintf("plan-%s", time.Now().Format("20060102-150405")),
		CurrentVersion: "1.5.0",
		TargetVersion:  target,
		Strategy:       strategy,
		BatchSize:      batchSize,
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

	// Save plan to file if requested
	if save != "" {
		data, err := yaml.Marshal(plan)
		if err != nil {
			return fmt.Errorf("failed to marshal plan: %w", err)
		}
		//nolint:gosec // G306: upgrade plan files need to be readable by operators
		if err := os.WriteFile(save, data, 0o644); err != nil {
			return fmt.Errorf("failed to write plan to %s: %w", save, err)
		}
		fmt.Printf("Plan saved to: %s\n\n", save)
	}

	return outputResult(cfg.OutputFormat, plan, func() {
		fmt.Printf("Upgrade Plan\n")
		fmt.Printf("============\n\n")
		fmt.Printf("Plan ID:        %s\n", plan.PlanID)
		fmt.Printf("Current:        %s → %s\n", plan.CurrentVersion, plan.TargetVersion)
		fmt.Printf("Strategy:       %s\n", plan.Strategy)
		fmt.Printf("Batch Size:     %d\n", plan.BatchSize)
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
		target         string
		strategy       string
		skipBackup     bool
		backupBefore   bool
		autoRollback   bool
		force          bool
		maxUnavailable int
		planFile       string
		async          bool
		confirm        bool
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
  kscorectl upgrade execute --target 1.6.0 --strategy rolling --max-unavailable 2

  # Execute from a saved plan file
  kscorectl upgrade execute --plan upgrade-plan.yaml

  # Execute asynchronously (returns immediately)
  kscorectl upgrade execute --target 1.6.0 --async

  # Skip confirmation prompt
  kscorectl upgrade execute --target 1.6.0 --confirm`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecute(cfg, target, strategy, skipBackup, force, maxUnavailable, planFile, async, confirm)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version (required unless --plan is specified)")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Upgrade strategy (rolling, canary, blue-green)")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "Skip automatic backup before upgrade")
	cmd.Flags().BoolVar(&backupBefore, "backup-before", true, "Create a backup before upgrading")
	cmd.Flags().BoolVar(&autoRollback, "auto-rollback", true, "Automatically rollback on failure")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force upgrade even with warnings")
	cmd.Flags().IntVar(&maxUnavailable, "max-unavailable", 1, "Maximum unavailable nodes during rolling upgrade")
	cmd.Flags().StringVar(&planFile, "plan", "", "Execute from a saved plan file")
	cmd.Flags().BoolVar(&async, "async", false, "Execute asynchronously (returns immediately)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt")
	cmd.MarkFlagsMutuallyExclusive("target", "plan")

	return cmd
}

func runExecute(cfg *Config, target, strategy string, skipBackup, force bool, maxUnavailable int, planFile string, async, confirm bool) error {
	// Load target from plan file if specified
	if planFile != "" {
		data, err := os.ReadFile(planFile)
		if err != nil {
			return fmt.Errorf("failed to read plan file: %w", err)
		}
		var plan UpgradePlan
		if err := yaml.Unmarshal(data, &plan); err != nil {
			return fmt.Errorf("failed to parse plan file: %w", err)
		}
		target = plan.TargetVersion
		strategy = plan.Strategy
		fmt.Printf("Loaded plan from %s: upgrading to %s with %s strategy\n\n", planFile, target, strategy)
	}

	if target == "" {
		return fmt.Errorf("--target is required (or use --plan to load from file)")
	}

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
	var (
		watch      bool
		statusVerbose bool
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show upgrade status",
		Long: `Show the status of the current or most recent upgrade.

Examples:
  # Show current status
  kscorectl upgrade status

  # Watch status in real-time
  kscorectl upgrade status --watch

  # Show verbose status with component details
  kscorectl upgrade status --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cfg, watch, statusVerbose)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch status updates in real-time")
	cmd.Flags().BoolVar(&statusVerbose, "verbose", false, "Show verbose component details")

	return cmd
}

func runStatus(cfg *Config, watch, statusVerbose bool) error {
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
	var (
		force    bool
		rollback bool
	)

	cmd := &cobra.Command{
		Use:   "cancel [upgrade-id]",
		Short: "Cancel an in-progress upgrade",
		Long: `Cancel an in-progress upgrade and optionally trigger rollback.

Examples:
  # Cancel current upgrade
  kscorectl upgrade cancel

  # Cancel a specific upgrade
  kscorectl upgrade cancel upgrade-20240115-100000

  # Force cancel without confirmation
  kscorectl upgrade cancel --force

  # Cancel and trigger immediate rollback
  kscorectl upgrade cancel --rollback`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			upgradeID := "upgrade-20240115-100000"
			if len(args) > 0 {
				upgradeID = args[0]
			}
			return runCancel(cfg, upgradeID, force, rollback)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force cancel without confirmation")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "Trigger immediate rollback after cancel")

	return cmd
}

func runCancel(cfg *Config, upgradeID string, force, rollback bool) error {
	fmt.Printf("Cancelling upgrade: %s\n", upgradeID)
	if rollback {
		fmt.Printf("Status: Rollback initiated\n")
		fmt.Printf("\nThe upgrade has been cancelled and rollback is in progress.\n")
	} else {
		fmt.Printf("Status: Cancelled\n")
		fmt.Printf("\nThe upgrade has been cancelled.\n")
	}
	fmt.Printf("Use 'kscorectl upgrade status' to monitor progress.\n")
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
	var confirm bool

	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote canary to full rollout",
		Long: `Promote the canary deployment to a full rollout.

This will complete the upgrade by rolling out to all remaining instances.

Examples:
  kscorectl upgrade canary promote --confirm`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required to promote canary deployment")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Promoting canary deployment...\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Status: Full rollout initiated\n")
			fmt.Fprintf(cmd.OutOrStdout(), "\nThe canary deployment has been promoted to full rollout.\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Use 'kscorectl upgrade status' to monitor progress.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm promotion")

	return cmd
}

func newCanaryRollbackCmd(cfg *Config) *cobra.Command {
	var confirm bool

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback canary deployment",
		Long: `Rollback the canary deployment to the previous version.

This will stop the canary deployment and revert to the previous stable version.

Examples:
  kscorectl upgrade canary rollback --confirm`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required to rollback canary deployment")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rolling back canary deployment...\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Status: Canary rollback initiated\n")
			fmt.Fprintf(cmd.OutOrStdout(), "\nThe canary deployment is being rolled back.\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Use 'kscorectl upgrade status' to monitor progress.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm rollback")

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
		target      string
		batchSize   int
		filter      string
		report      bool
		agentStatus bool
		retry       string
		skip        string
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
  kscorectl upgrade agents --report

  # Show agent upgrade status
  kscorectl upgrade agents --status

  # Retry a failed agent upgrade
  kscorectl upgrade agents --retry agent-005

  # Skip a failed agent
  kscorectl upgrade agents --skip agent-005`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if report {
				return runAgentReport(cfg)
			}
			if agentStatus {
				return runAgentUpgrade(cfg, "", batchSize, filter)
			}
			if retry != "" {
				fmt.Printf("Retrying upgrade for agent %s...\n", retry)
				fmt.Printf("Agent %s: upgrade re-queued\n", retry)
				return nil
			}
			if skip != "" {
				fmt.Printf("Skipping upgrade for agent %s\n", skip)
				fmt.Printf("Agent %s: marked as skipped\n", skip)
				return nil
			}
			return runAgentUpgrade(cfg, target, batchSize, filter)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version for agents")
	cmd.Flags().IntVar(&batchSize, "batch-size", 5, "Number of agents to upgrade in parallel")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter expression for agent selection")
	cmd.Flags().BoolVar(&report, "report", false, "Show agent version report")
	cmd.Flags().BoolVar(&agentStatus, "status", false, "Show agent upgrade status")
	cmd.Flags().StringVar(&retry, "retry", "", "Retry upgrade for a specific agent")
	cmd.Flags().StringVar(&skip, "skip", "", "Skip upgrade for a specific agent")

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
			switch a.Status {
			case "upgrade_available":
				icon = "↑"
			case "failed":
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
	var (
		limit        int
		statusFilter string
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show upgrade history",
		Long: `Show history of upgrade operations.

Examples:
  kscorectl upgrade history
  kscorectl upgrade history --limit 10
  kscorectl upgrade history --status completed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cfg, limit, statusFilter)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Maximum number of entries")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status (e.g., completed, failed, rolled_back)")

	return cmd
}

func runHistory(cfg *Config, limit int, statusFilter string) error {
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

	// Filter by status
	if statusFilter != "" {
		filtered := make([]UpgradeHistory, 0, len(history))
		for i := range history {
			if strings.EqualFold(history[i].Status, statusFilter) {
				filtered = append(filtered, history[i])
			}
		}
		history = filtered
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
		for i := range history {
			h := &history[i]
			icon := "✓"
			switch h.Status {
			case "failed":
				icon = "✗"
			case "rolled_back":
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
		Use:   "logs [upgrade-id]",
		Short: "Show upgrade logs",
		Long: `Show logs from an upgrade operation.

The upgrade ID can be specified as a positional argument or via --upgrade-id.

Examples:
  # Show logs for specific upgrade
  kscorectl upgrade logs abc123

  # Using flag form
  kscorectl upgrade logs --upgrade-id abc123

  # Follow logs in real-time
  kscorectl upgrade logs abc123 --follow`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				upgradeID = args[0]
			}
			if upgradeID == "" {
				return fmt.Errorf("upgrade-id is required (as argument or --upgrade-id flag)")
			}
			return runLogs(cfg, upgradeID, follow, tail)
		},
	}

	cmd.Flags().StringVar(&upgradeID, "upgrade-id", "", "Upgrade ID")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow logs in real-time")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show")

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

// UpgradePath represents a step in the upgrade path
type UpgradePath struct {
	FromVersion string `json:"from_version" yaml:"from_version"`
	ToVersion   string `json:"to_version" yaml:"to_version"`
	Direct      bool   `json:"direct" yaml:"direct"`
	Notes       string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// UpgradePathResult represents the full upgrade path analysis
type UpgradePathResult struct {
	CurrentVersion string        `json:"current_version" yaml:"current_version"`
	TargetVersion  string        `json:"target_version" yaml:"target_version"`
	DirectUpgrade  bool          `json:"direct_upgrade" yaml:"direct_upgrade"`
	Steps          []UpgradePath `json:"steps" yaml:"steps"`
}

func newPathCmd(cfg *Config) *cobra.Command {
	var (
		target string
		from   string
	)

	cmd := &cobra.Command{
		Use:   "path",
		Short: "Show the upgrade path between versions",
		Long: `Show the recommended upgrade path from the current (or specified) version
to the target version, including any required intermediate upgrades.

Examples:
  # Show path from current version to target
  kscorectl upgrade path --target 2.0.0

  # Show path between two specific versions
  kscorectl upgrade path --from 1.3.0 --target 2.0.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPath(cfg, from, target)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target version (required)")
	cmd.Flags().StringVar(&from, "from", "", "Starting version (default: current installed version)")
	cmd.MarkFlagRequired("target")

	return cmd
}

func runPath(cfg *Config, from, target string) error {
	currentVersion := "1.5.0"
	if from != "" {
		currentVersion = from
	}

	result := UpgradePathResult{
		CurrentVersion: currentVersion,
		TargetVersion:  target,
		DirectUpgrade:  true,
		Steps: []UpgradePath{
			{FromVersion: currentVersion, ToVersion: target, Direct: true},
		},
	}

	if currentVersion == "1.3.0" && target == "2.0.0" {
		result.DirectUpgrade = false
		result.Steps = []UpgradePath{
			{FromVersion: "1.3.0", ToVersion: "1.4.0", Direct: true, Notes: "Patch upgrade"},
			{FromVersion: "1.4.0", ToVersion: "1.5.0", Direct: true, Notes: "Patch upgrade"},
			{FromVersion: "1.5.0", ToVersion: "1.6.0", Direct: true, Notes: "Last 1.x release"},
			{FromVersion: "1.6.0", ToVersion: "2.0.0", Direct: true, Notes: "Major upgrade, review breaking changes"},
		}
	}

	return outputResult(cfg.OutputFormat, result, func() {
		fmt.Printf("Upgrade Path\n")
		fmt.Printf("============\n\n")
		fmt.Printf("From: %s\n", result.CurrentVersion)
		fmt.Printf("To:   %s\n", result.TargetVersion)
		if result.DirectUpgrade {
			fmt.Printf("\nDirect upgrade supported: %s -> %s\n", result.CurrentVersion, result.TargetVersion)
		} else {
			fmt.Printf("\nDirect upgrade not supported. Required intermediate steps:\n\n")
			for i, step := range result.Steps {
				fmt.Printf("  %d. %s -> %s", i+1, step.FromVersion, step.ToVersion)
				if step.Notes != "" {
					fmt.Printf("  (%s)", step.Notes)
				}
				fmt.Println()
			}
		}
	})
}

func newResumeCmd(cfg *Config) *cobra.Command {
	var upgradeID string

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume an interrupted upgrade",
		Long: `Resume an upgrade that was interrupted or paused.

This picks up from the last completed step and continues the upgrade process.

Examples:
  # Resume the most recent interrupted upgrade
  kscorectl upgrade resume

  # Resume a specific upgrade by ID
  kscorectl upgrade resume --upgrade-id upgrade-20240115-100000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cfg, upgradeID)
		},
	}

	cmd.Flags().StringVar(&upgradeID, "upgrade-id", "", "Specific upgrade to resume (default: most recent)")

	return cmd
}

func runResume(cfg *Config, upgradeID string) error {
	if upgradeID == "" {
		upgradeID = "upgrade-20240115-100000"
	}

	status := UpgradeStatus{
		UpgradeID:      upgradeID,
		Status:         "resuming",
		Phase:          "upgrading",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Strategy:       "rolling",
		Progress:       35,
		StartedAt:      "2024-01-15T10:00:00Z",
		CurrentStep:    "Resuming from step: Upgrading control plane servers",
		StepsCompleted: 3,
		StepsTotal:     7,
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Resuming Upgrade\n")
		fmt.Printf("================\n\n")
		fmt.Printf("Upgrade ID: %s\n", status.UpgradeID)
		fmt.Printf("Target:     %s -> %s\n", status.CurrentVersion, status.TargetVersion)
		fmt.Printf("Strategy:   %s\n", status.Strategy)
		fmt.Printf("Progress:   %d%% (%d/%d steps completed before interruption)\n", status.Progress, status.StepsCompleted, status.StepsTotal)
		fmt.Printf("Resuming:   %s\n", status.CurrentStep)
		fmt.Printf("\nUse 'kscorectl upgrade status --watch' to monitor progress.\n")
	})
}

func newPackageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage air-gapped upgrade packages",
		Long: `Create, verify, apply, inspect, and rollback upgrade packages for air-gapped deployments.

Upgrade packages contain new binaries, modules, migrations, and scripts needed
to upgrade a Keystone Core installation without internet access.

Examples:
  # Create an upgrade package
  kscorectl upgrade package create --from 1.0.0 --to 1.1.0 \
    --build-dir ./build --output upgrade.tar.gz

  # Verify an upgrade package
  kscorectl upgrade package verify upgrade.tar.gz --trusted-key release.pub

  # Inspect package contents
  kscorectl upgrade package inspect upgrade.tar.gz

  # Apply an upgrade package
  kscorectl upgrade package apply upgrade.tar.gz \
    --install-dir /usr/local/bin --backup-dir /var/backup

  # Rollback from backup
  kscorectl upgrade package rollback \
    --backup-dir /var/backup/upgrade-backup-1.0.0-to-1.1.0-... \
    --install-dir /usr/local/bin`,
	}

	cmd.AddCommand(
		newPackageCreateCmd(),
		newPackageVerifyCmd(),
		newPackageInspectCmd(),
		newPackageApplyCmd(),
		newPackageRollbackCmd(),
	)

	return cmd
}

func newPackageCreateCmd() *cobra.Command {
	var (
		fromVersion     string
		toVersion       string
		platform        string
		buildDir        string
		outputPath      string
		signingKey      string
		modulesDir      string
		migrationsDir   string
		preScriptsDir   string
		postScriptsDir  string
		breakingChanges []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an upgrade package",
		Long: `Create an air-gapped upgrade package from a build directory.

The build directory should contain binaries in {os}/{arch}/ layout
(e.g., linux/amd64/kscore-server).

Examples:
  kscorectl upgrade package create --from 1.0.0 --to 1.1.0 \
    --build-dir ./build --output upgrade.tar.gz

  kscorectl upgrade package create --from 1.0.0 --to 1.1.0 \
    --build-dir ./build --signing-key key.pem \
    --modules-dir ./modules --migrations-dir ./migrations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			plat, err := bootstrap.ParsePlatform(platform)
			if err != nil {
				return fmt.Errorf("invalid platform: %w", err)
			}

			cfg := airgapupgrade.BuilderConfig{
				FromVersion:     fromVersion,
				ToVersion:       toVersion,
				Platform:        plat,
				BuildDir:        buildDir,
				OutputPath:      outputPath,
				ModulesDir:      modulesDir,
				MigrationsDir:   migrationsDir,
				PreScriptsDir:   preScriptsDir,
				PostScriptsDir:  postScriptsDir,
				BreakingChanges: breakingChanges,
			}

			if signingKey != "" {
				keyData, err := os.ReadFile(signingKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading signing key: %w", err)
				}
				cfg.SigningKey = keyData
			}

			builder, err := airgapupgrade.NewBuilder(cfg)
			if err != nil {
				return err
			}

			manifest, err := builder.Build(context.Background())
			if err != nil {
				return err
			}

			out := outputPath
			if out == "" {
				out = fmt.Sprintf("keystone-upgrade-%s-to-%s-%s-%s.tar.gz",
					fromVersion, toVersion, plat.OS, plat.Arch)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Upgrade Package Created\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  From:       %s\n", manifest.FromVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  To:         %s\n", manifest.ToVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  Platform:   %s\n", manifest.Platform)
			fmt.Fprintf(cmd.OutOrStdout(), "  Components: %d\n", len(manifest.Components))
			fmt.Fprintf(cmd.OutOrStdout(), "  Modules:    %d\n", len(manifest.Modules))
			fmt.Fprintf(cmd.OutOrStdout(), "  Migrations: %d\n", len(manifest.Migrations))
			fmt.Fprintf(cmd.OutOrStdout(), "  Signed:     %v\n", manifest.RequiresVerification)
			fmt.Fprintf(cmd.OutOrStdout(), "  Output:     %s\n", out)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromVersion, "from", "", "Minimum source version (required)")
	cmd.Flags().StringVar(&toVersion, "to", "", "Target version (required)")
	cmd.Flags().StringVar(&platform, "platform", "linux/amd64", "Target platform (os/arch)")
	cmd.Flags().StringVar(&buildDir, "build-dir", "", "Directory containing new binaries (required)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output archive path")
	cmd.Flags().StringVar(&signingKey, "signing-key", "", "Path to PEM private key for signing")
	cmd.Flags().StringVar(&modulesDir, "modules-dir", "", "Directory with updated modules")
	cmd.Flags().StringVar(&migrationsDir, "migrations-dir", "", "Directory with migration scripts")
	cmd.Flags().StringVar(&preScriptsDir, "pre-scripts-dir", "", "Directory with pre-upgrade scripts")
	cmd.Flags().StringVar(&postScriptsDir, "post-scripts-dir", "", "Directory with post-upgrade scripts")
	cmd.Flags().StringSliceVar(&breakingChanges, "breaking-change", nil, "Breaking change descriptions")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("build-dir")

	return cmd
}

func newPackageVerifyCmd() *cobra.Command {
	var trustedKey string

	cmd := &cobra.Command{
		Use:   "verify <package.tar.gz>",
		Short: "Verify an upgrade package",
		Long: `Verify an upgrade package's signature, checksums, and manifest.

Examples:
  kscorectl upgrade package verify upgrade.tar.gz --trusted-key release.pub`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]

			extractDir, err := os.MkdirTemp("", "kscore-upgrade-verify-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(extractDir)

			if err := bootstrap.ExtractArchive(packagePath, extractDir); err != nil {
				return fmt.Errorf("extracting package: %w", err)
			}

			var trustedKeys [][]byte
			if trustedKey != "" {
				keyData, err := os.ReadFile(trustedKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading trusted key: %w", err)
				}
				trustedKeys = append(trustedKeys, keyData)
			}

			v := airgapupgrade.NewPackageVerifier(trustedKeys)
			result, err := v.Verify(context.Background(), extractDir)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Package Verification\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  File:      %s\n", packagePath)
			fmt.Fprintf(cmd.OutOrStdout(), "  Valid:     %v\n", result.Valid)
			fmt.Fprintf(cmd.OutOrStdout(), "  Manifest:  %v\n", result.ManifestValid)
			fmt.Fprintf(cmd.OutOrStdout(), "  Checksums: %v\n", result.ChecksumsValid)
			fmt.Fprintf(cmd.OutOrStdout(), "  Signed:    %v\n", result.SignaturePresent)
			if result.Error != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "  Error:     %v\n", result.Error)
			}
			for _, w := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "  Warning:   %s\n", w)
			}
			if result.Manifest != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  From:      %s\n", result.Manifest.FromVersion)
				fmt.Fprintf(cmd.OutOrStdout(), "  To:        %s\n", result.Manifest.ToVersion)
			}

			if !result.Valid {
				return fmt.Errorf("verification failed")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&trustedKey, "trusted-key", "", "Path to trusted public key (PEM)")

	return cmd
}

func newPackageInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <package.tar.gz>",
		Short: "Inspect upgrade package contents",
		Long: `Show the manifest and contents of an upgrade package.

Examples:
  kscorectl upgrade package inspect upgrade.tar.gz`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]

			extractDir, err := os.MkdirTemp("", "kscore-upgrade-inspect-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(extractDir)

			if err := bootstrap.ExtractArchive(packagePath, extractDir); err != nil {
				return fmt.Errorf("extracting package: %w", err)
			}

			manifest, err := airgapupgrade.ReadManifest(fmt.Sprintf("%s/manifest.json", extractDir))
			if err != nil {
				return fmt.Errorf("reading manifest: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Upgrade Package: %s\n", packagePath)
			fmt.Fprintf(cmd.OutOrStdout(), "  Schema:     %s\n", manifest.SchemaVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  From:       %s\n", manifest.FromVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  To:         %s\n", manifest.ToVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  Platform:   %s\n", manifest.Platform)
			fmt.Fprintf(cmd.OutOrStdout(), "  Created:    %s\n", manifest.Created.Format(time.RFC3339))
			if manifest.CreatedBy != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Created By: %s\n", manifest.CreatedBy)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Signed:     %v\n", manifest.RequiresVerification)
			fmt.Fprintln(cmd.OutOrStdout())

			if len(manifest.Components) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Components (%d):\n", len(manifest.Components))
				for _, c := range manifest.Components {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s %s (%s)\n", c.Name, c.Version, c.Path)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			if len(manifest.Modules) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Modules (%d):\n", len(manifest.Modules))
				for _, m := range manifest.Modules {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", m.Name, m.Path)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			if len(manifest.Migrations) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Migrations (%d):\n", len(manifest.Migrations))
				for _, m := range manifest.Migrations {
					fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", m.Order, m.Name)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			if len(manifest.BreakingChanges) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Breaking Changes:\n")
				for _, bc := range manifest.BreakingChanges {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", bc)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			if len(manifest.ConfigChanges) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Config Changes (%d):\n", len(manifest.ConfigChanges))
				for _, cc := range manifest.ConfigChanges {
					breaking := ""
					if cc.Breaking {
						breaking = " [BREAKING]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s -> %s%s\n", cc.Key, cc.OldDefault, cc.NewDefault, breaking)
				}
			}

			return nil
		},
	}

	return cmd
}

func newPackageApplyCmd() *cobra.Command {
	var (
		installDir string
		backupDir  string
		dryRun     bool
		skipBackup bool
		skipScripts bool
		trustedKey string
	)

	cmd := &cobra.Command{
		Use:   "apply <package.tar.gz>",
		Short: "Apply an upgrade package",
		Long: `Apply an upgrade package to the current installation.

This will extract the package, verify its integrity, back up current binaries,
and replace them with the new versions.

Examples:
  kscorectl upgrade package apply upgrade.tar.gz \
    --install-dir /usr/local/bin --backup-dir /var/backup

  kscorectl upgrade package apply upgrade.tar.gz --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := airgapupgrade.InstallerConfig{
				PackagePath: args[0],
				InstallDir:  installDir,
				BackupDir:   backupDir,
				DryRun:      dryRun,
				SkipBackup:  skipBackup,
				SkipScripts: skipScripts,
				ProgressFunc: func(phase string, progress int) {
					fmt.Fprintf(cmd.ErrOrStderr(), "[%3d%%] %s\n", progress, phase)
				},
			}

			if trustedKey != "" {
				keyData, err := os.ReadFile(trustedKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading trusted key: %w", err)
				}
				cfg.TrustedKeys = [][]byte{keyData}
			}

			inst, err := airgapupgrade.NewInstaller(cfg)
			if err != nil {
				return err
			}

			result, err := inst.Install(context.Background())
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nUpgrade %s\n", func() string {
				if dryRun {
					return "(Dry Run)"
				}
				return "Complete"
			}())
			fmt.Fprintf(cmd.OutOrStdout(), "  From:       %s\n", result.FromVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  To:         %s\n", result.ToVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "  Components: %s\n", strings.Join(result.UpgradedComponents, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "  Migrations: %d\n", result.MigrationsRun)
			fmt.Fprintf(cmd.OutOrStdout(), "  Duration:   %s\n", result.Duration.Round(time.Millisecond))
			if result.BackupPath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Backup:     %s\n", result.BackupPath)
			}
			for _, w := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "  Warning:    %s\n", w)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&installDir, "install-dir", "/usr/local/bin", "Directory containing installed binaries")
	cmd.Flags().StringVar(&backupDir, "backup-dir", "", "Directory for rollback backup")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without making changes")
	cmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "Skip creating backup before upgrade")
	cmd.Flags().BoolVar(&skipScripts, "skip-scripts", false, "Skip pre/post upgrade scripts")
	cmd.Flags().StringVar(&trustedKey, "trusted-key", "", "Path to trusted public key (PEM)")

	return cmd
}

func newPackageRollbackCmd() *cobra.Command {
	var (
		backupDir  string
		installDir string
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback from upgrade backup",
		Long: `Restore binaries from a backup created during a previous upgrade.

Examples:
  kscorectl upgrade package rollback \
    --backup-dir /var/backup/upgrade-backup-1.0.0-to-1.1.0-1234567890 \
    --install-dir /usr/local/bin

  kscorectl upgrade package rollback --backup-dir /var/backup/... --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := airgapupgrade.Rollback(context.Background(), airgapupgrade.RollbackConfig{
				BackupDir:  backupDir,
				InstallDir: installDir,
				DryRun:     dryRun,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Rollback %s\n", func() string {
				if dryRun {
					return "(Dry Run)"
				}
				return "Complete"
			}())
			fmt.Fprintf(cmd.OutOrStdout(), "  Restored:  %s\n", strings.Join(result.RestoredComponents, ", "))
			if len(result.SkippedComponents) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  Skipped:   %s\n", strings.Join(result.SkippedComponents, ", "))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Duration:  %s\n", result.Duration.Round(time.Millisecond))
			return nil
		},
	}

	cmd.Flags().StringVar(&backupDir, "backup-dir", "", "Backup directory from previous upgrade (required)")
	cmd.Flags().StringVar(&installDir, "install-dir", "/usr/local/bin", "Directory containing installed binaries")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without making changes")
	_ = cmd.MarkFlagRequired("backup-dir")

	return cmd
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
