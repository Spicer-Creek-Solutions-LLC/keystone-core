package main

import (
	"bytes"
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-upgrade" {
		t.Errorf("Use = %v, want kscore-upgrade", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"check",
		"plan",
		"execute",
		"status",
		"cancel",
		"canary",
		"agents",
		"rollback",
		"history",
		"logs",
		"path",
		"resume",
		"version",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	if cmd == nil {
		t.Fatal("newVersionCmd should not return nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %v, want version", cmd.Use)
	}

	// Test execution
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("version command should produce output")
	}
}

func TestNewCheckCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newCheckCmd(cfg)

	if cmd == nil {
		t.Fatal("newCheckCmd should not return nil")
	}
	if cmd.Use != "check" {
		t.Errorf("Use = %v, want check", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("target") == nil {
		t.Error("expected flag 'target' not found")
	}
}

func TestNewPlanCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newPlanCmd(cfg)

	if cmd == nil {
		t.Fatal("newPlanCmd should not return nil")
	}
	if cmd.Use != "plan" {
		t.Errorf("Use = %v, want plan", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "strategy"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewExecuteCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newExecuteCmd(cfg)

	if cmd == nil {
		t.Fatal("newExecuteCmd should not return nil")
	}
	if cmd.Use != "execute" {
		t.Errorf("Use = %v, want execute", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "strategy", "skip-backup", "force", "max-unavailable"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewStatusCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newStatusCmd(cfg)

	if cmd == nil {
		t.Fatal("newStatusCmd should not return nil")
	}
	if cmd.Use != "status" {
		t.Errorf("Use = %v, want status", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("watch") == nil {
		t.Error("expected flag 'watch' not found")
	}
}

func TestNewCancelCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newCancelCmd(cfg)

	if cmd == nil {
		t.Fatal("newCancelCmd should not return nil")
	}
	if cmd.Use != "cancel" {
		t.Errorf("Use = %v, want cancel", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("force") == nil {
		t.Error("expected flag 'force' not found")
	}
}

func TestNewCanaryCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newCanaryCmd(cfg)

	if cmd == nil {
		t.Fatal("newCanaryCmd should not return nil")
	}
	if cmd.Use != "canary" {
		t.Errorf("Use = %v, want canary", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"promote", "rollback", "status"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewAgentsCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newAgentsCmd(cfg)

	if cmd == nil {
		t.Fatal("newAgentsCmd should not return nil")
	}
	if cmd.Use != "agents" {
		t.Errorf("Use = %v, want agents", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "batch-size", "filter", "report"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewRollbackCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRollbackCmd(cfg)

	if cmd == nil {
		t.Fatal("newRollbackCmd should not return nil")
	}
	if cmd.Use != "rollback" {
		t.Errorf("Use = %v, want rollback", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "status", "components", "dry-run", "force", "cancel"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewHistoryCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newHistoryCmd(cfg)

	if cmd == nil {
		t.Fatal("newHistoryCmd should not return nil")
	}
	if cmd.Use != "history" {
		t.Errorf("Use = %v, want history", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected flag 'limit' not found")
	}
}

func TestNewLogsCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newLogsCmd(cfg)

	if cmd == nil {
		t.Fatal("newLogsCmd should not return nil")
	}
	if cmd.Use != "logs" {
		t.Errorf("Use = %v, want logs", cmd.Use)
	}

	// Check flags exist
	flags := []string{"upgrade-id", "follow", "tail"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestConfigStructure(t *testing.T) {
	cfg := Config{
		ServerAddr:   "localhost:9090",
		OutputFormat: "json",
		Verbose:      true,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("OutputFormat = %v, want json", cfg.OutputFormat)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestUpgradeCheckStructure(t *testing.T) {
	check := UpgradeCheck{
		CurrentVersion:   "1.5.0",
		LatestVersion:    "1.6.0",
		UpgradeAvailable: true,
		TargetVersion:    "1.6.0",
		Compatible:       true,
		BreakingChanges:  []string{},
		Prerequisites:    []string{"Backup database"},
		Components: []ComponentStatus{
			{Name: "server", CurrentVersion: "1.5.0", TargetVersion: "1.6.0", Status: "upgrade_available", UpgradeNeeded: true},
		},
	}

	if check.CurrentVersion != "1.5.0" {
		t.Errorf("CurrentVersion = %v, want 1.5.0", check.CurrentVersion)
	}
	if !check.UpgradeAvailable {
		t.Error("UpgradeAvailable should be true")
	}
	if !check.Compatible {
		t.Error("Compatible should be true")
	}
	if len(check.Components) != 1 {
		t.Errorf("Components count = %d, want 1", len(check.Components))
	}
}

func TestUpgradePlanStructure(t *testing.T) {
	plan := UpgradePlan{
		PlanID:         "plan-001",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Strategy:       "rolling",
		EstimatedTime:  "15m",
		RiskLevel:      "low",
		Backups:        true,
		Steps: []UpgradeStep{
			{Order: 1, Component: "server", Action: "upgrade", Description: "Upgrade server", Duration: "5m"},
		},
	}

	if plan.PlanID != "plan-001" {
		t.Errorf("PlanID = %v, want plan-001", plan.PlanID)
	}
	if plan.Strategy != "rolling" {
		t.Errorf("Strategy = %v, want rolling", plan.Strategy)
	}
	if !plan.Backups {
		t.Error("Backups should be true")
	}
	if len(plan.Steps) != 1 {
		t.Errorf("Steps count = %d, want 1", len(plan.Steps))
	}
}

func TestUpgradeStepStructure(t *testing.T) {
	step := UpgradeStep{
		Order:       1,
		Component:   "server",
		Action:      "upgrade",
		Description: "Upgrade control plane servers",
		Duration:    "5m",
		Rollback:    "Rollback server binaries",
	}

	if step.Order != 1 {
		t.Errorf("Order = %d, want 1", step.Order)
	}
	if step.Component != "server" {
		t.Errorf("Component = %v, want server", step.Component)
	}
	if step.Rollback != "Rollback server binaries" {
		t.Errorf("Rollback = %v, want 'Rollback server binaries'", step.Rollback)
	}
}

func TestUpgradeStatusStructure(t *testing.T) {
	status := UpgradeStatus{
		UpgradeID:      "upgrade-001",
		Status:         "in_progress",
		Phase:          "upgrading",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Strategy:       "rolling",
		Progress:       50,
		StartedAt:      "2024-01-15T10:00:00Z",
		CurrentStep:    "Upgrading server",
		StepsCompleted: 3,
		StepsTotal:     6,
	}

	if status.UpgradeID != "upgrade-001" {
		t.Errorf("UpgradeID = %v, want upgrade-001", status.UpgradeID)
	}
	if status.Status != "in_progress" {
		t.Errorf("Status = %v, want in_progress", status.Status)
	}
	if status.Progress != 50 {
		t.Errorf("Progress = %d, want 50", status.Progress)
	}
}

func TestCanaryStatusStructure(t *testing.T) {
	status := CanaryStatus{
		Phase:           "monitoring",
		PercentComplete: 25,
		HealthyReplicas: 1,
		TotalReplicas:   4,
		SuccessRate:     99.5,
		CanPromote:      true,
		CanRollback:     true,
	}

	if status.Phase != "monitoring" {
		t.Errorf("Phase = %v, want monitoring", status.Phase)
	}
	if status.PercentComplete != 25 {
		t.Errorf("PercentComplete = %d, want 25", status.PercentComplete)
	}
	if status.SuccessRate != 99.5 {
		t.Errorf("SuccessRate = %v, want 99.5", status.SuccessRate)
	}
	if !status.CanPromote {
		t.Error("CanPromote should be true")
	}
}

func TestAgentUpgradeStatusStructure(t *testing.T) {
	status := AgentUpgradeStatus{
		TargetVersion: "1.6.0",
		TotalAgents:   100,
		Upgraded:      45,
		Pending:       50,
		InProgress:    5,
		Failed:        0,
		Progress:      45,
		BatchSize:     10,
		CurrentBatch:  5,
		TotalBatches:  10,
	}

	if status.TargetVersion != "1.6.0" {
		t.Errorf("TargetVersion = %v, want 1.6.0", status.TargetVersion)
	}
	if status.TotalAgents != 100 {
		t.Errorf("TotalAgents = %d, want 100", status.TotalAgents)
	}
	if status.Progress != 45 {
		t.Errorf("Progress = %d, want 45", status.Progress)
	}
}

func TestAgentDetailStructure(t *testing.T) {
	agent := AgentDetail{
		AgentID:        "agent-001",
		Hostname:       "web-01",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Status:         "upgrade_available",
		Error:          "",
	}

	if agent.AgentID != "agent-001" {
		t.Errorf("AgentID = %v, want agent-001", agent.AgentID)
	}
	if agent.Hostname != "web-01" {
		t.Errorf("Hostname = %v, want web-01", agent.Hostname)
	}
	if agent.Status != "upgrade_available" {
		t.Errorf("Status = %v, want upgrade_available", agent.Status)
	}
}

func TestUpgradeHistoryStructure(t *testing.T) {
	history := UpgradeHistory{
		UpgradeID:   "upgrade-001",
		FromVersion: "1.4.0",
		ToVersion:   "1.5.0",
		Strategy:    "rolling",
		Status:      "completed",
		StartedAt:   "2024-01-10T08:00:00Z",
		CompletedAt: "2024-01-10T08:15:00Z",
		Duration:    "15m",
		InitiatedBy: "admin",
	}

	if history.UpgradeID != "upgrade-001" {
		t.Errorf("UpgradeID = %v, want upgrade-001", history.UpgradeID)
	}
	if history.FromVersion != "1.4.0" {
		t.Errorf("FromVersion = %v, want 1.4.0", history.FromVersion)
	}
	if history.Status != "completed" {
		t.Errorf("Status = %v, want completed", history.Status)
	}
	if history.InitiatedBy != "admin" {
		t.Errorf("InitiatedBy = %v, want admin", history.InitiatedBy)
	}
}

func TestComponentStatusStructure(t *testing.T) {
	status := ComponentStatus{
		Name:           "server",
		CurrentVersion: "1.5.0",
		TargetVersion:  "1.6.0",
		Status:         "upgrade_available",
		UpgradeNeeded:  true,
	}

	if status.Name != "server" {
		t.Errorf("Name = %v, want server", status.Name)
	}
	if !status.UpgradeNeeded {
		t.Error("UpgradeNeeded should be true")
	}
}

func TestNewPathCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newPathCmd(cfg)

	if cmd == nil {
		t.Fatal("newPathCmd should not return nil")
	}
	if cmd.Use != "path" {
		t.Errorf("Use = %v, want path", cmd.Use)
	}

	flags := []string{"target", "from"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewResumeCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newResumeCmd(cfg)

	if cmd == nil {
		t.Fatal("newResumeCmd should not return nil")
	}
	if cmd.Use != "resume" {
		t.Errorf("Use = %v, want resume", cmd.Use)
	}

	if cmd.Flags().Lookup("upgrade-id") == nil {
		t.Error("expected flag 'upgrade-id' not found")
	}
}

func TestCheckCmdFromFlag(t *testing.T) {
	cfg := &Config{}
	cmd := newCheckCmd(cfg)

	fromFlag := cmd.Flags().Lookup("from")
	if fromFlag == nil {
		t.Fatal("expected --from flag on check command")
	}
}

func TestExecuteCmdNewFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newExecuteCmd(cfg)

	flags := []string{"backup-before", "auto-rollback"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found on execute command", flag)
		}
	}
}

func TestStatusCmdVerboseFlag(t *testing.T) {
	cfg := &Config{}
	cmd := newStatusCmd(cfg)

	if cmd.Flags().Lookup("verbose") == nil {
		t.Error("expected --verbose flag on status command")
	}
}

func TestAgentsCmdNewFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newAgentsCmd(cfg)

	flags := []string{"status", "retry", "skip"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found on agents command", flag)
		}
	}
}

func TestUpgradePathStructure(t *testing.T) {
	result := UpgradePathResult{
		CurrentVersion: "1.3.0",
		TargetVersion:  "2.0.0",
		DirectUpgrade:  false,
		Steps: []UpgradePath{
			{FromVersion: "1.3.0", ToVersion: "1.4.0", Direct: true, Notes: "Patch upgrade"},
			{FromVersion: "1.4.0", ToVersion: "1.5.0", Direct: true, Notes: "Patch upgrade"},
		},
	}

	if result.DirectUpgrade {
		t.Error("DirectUpgrade should be false")
	}
	if len(result.Steps) != 2 {
		t.Errorf("Steps count = %d, want 2", len(result.Steps))
	}
	if result.Steps[0].FromVersion != "1.3.0" {
		t.Errorf("Steps[0].FromVersion = %v, want 1.3.0", result.Steps[0].FromVersion)
	}
}

func TestRunPathDirect(t *testing.T) {
	cfg := &Config{OutputFormat: "json"}
	err := runPath(cfg, "", "1.6.0")
	if err != nil {
		t.Fatalf("runPath failed: %v", err)
	}
}

func TestRunPathIndirect(t *testing.T) {
	cfg := &Config{OutputFormat: "json"}
	err := runPath(cfg, "1.3.0", "2.0.0")
	if err != nil {
		t.Fatalf("runPath failed: %v", err)
	}
}

func TestRunResume(t *testing.T) {
	cfg := &Config{OutputFormat: "json"}
	err := runResume(cfg, "")
	if err != nil {
		t.Fatalf("runResume failed: %v", err)
	}
}

func TestRunResumeWithID(t *testing.T) {
	cfg := &Config{OutputFormat: "json"}
	err := runResume(cfg, "upgrade-custom-id")
	if err != nil {
		t.Fatalf("runResume with ID failed: %v", err)
	}
}
