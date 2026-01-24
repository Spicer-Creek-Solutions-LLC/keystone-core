package main

import (
	"bytes"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/schedule"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-schedule" {
		t.Errorf("Use = %v, want kscore-schedule", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"version",
		"schedule",
		"maintenance",
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

func TestNewScheduleCmd(t *testing.T) {
	cmd := newScheduleCmd()

	if cmd == nil {
		t.Fatal("newScheduleCmd should not return nil")
	}
	if cmd.Use != "schedule" {
		t.Errorf("Use = %v, want schedule", cmd.Use)
	}

	// Check aliases
	expectedAliases := []string{"sched", "s"}
	for _, alias := range expectedAliases {
		found := false
		for _, a := range cmd.Aliases {
			if a == alias {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected alias %q not found", alias)
		}
	}

	// Should have subcommands
	subcommands := []string{"list", "show", "create", "trigger", "pause", "resume", "enable", "disable", "delete", "history"}
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

func TestNewScheduleListCmd(t *testing.T) {
	cmd := newScheduleListCmd()

	if cmd == nil {
		t.Fatal("newScheduleListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check aliases
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}

	// Check flags exist
	flags := []string{"type", "status", "label", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewScheduleCreateCmd(t *testing.T) {
	cmd := newScheduleCreateCmd()

	if cmd == nil {
		t.Fatal("newScheduleCreateCmd should not return nil")
	}
	if cmd.Use != "create" {
		t.Errorf("Use = %v, want create", cmd.Use)
	}

	// Check flags exist
	flags := []string{
		"name", "description", "type", "cron", "interval", "timezone",
		"target-all", "target-agent", "target-glob", "target-tags", "target-roles",
		"command", "state-path", "blueprint", "priority", "timeout",
		"require-approval", "label", "maintenance-window",
	}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewScheduleHistoryCmd(t *testing.T) {
	cmd := newScheduleHistoryCmd()

	if cmd == nil {
		t.Fatal("newScheduleHistoryCmd should not return nil")
	}
	if cmd.Use != "history <schedule-id>" {
		t.Errorf("Use = %v, want 'history <schedule-id>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"limit", "status"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewMaintenanceCmd(t *testing.T) {
	cmd := newMaintenanceCmd()

	if cmd == nil {
		t.Fatal("newMaintenanceCmd should not return nil")
	}
	if cmd.Use != "maintenance" {
		t.Errorf("Use = %v, want maintenance", cmd.Use)
	}

	// Check aliases
	expectedAliases := []string{"maint", "m"}
	for _, alias := range expectedAliases {
		found := false
		for _, a := range cmd.Aliases {
			if a == alias {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected alias %q not found", alias)
		}
	}

	// Should have subcommands
	subcommands := []string{"list", "show", "create", "start", "end", "cancel", "extend", "active", "upcoming", "conflicts", "delete"}
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

func TestNewMaintenanceListCmd(t *testing.T) {
	cmd := newMaintenanceListCmd()

	if cmd == nil {
		t.Fatal("newMaintenanceListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"status", "type", "label", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewMaintenanceCreateCmd(t *testing.T) {
	cmd := newMaintenanceCreateCmd()

	if cmd == nil {
		t.Fatal("newMaintenanceCreateCmd should not return nil")
	}
	if cmd.Use != "create" {
		t.Errorf("Use = %v, want create", cmd.Use)
	}

	// Check flags exist
	flags := []string{
		"name", "description", "type", "start", "end", "timezone",
		"scope-all", "scope-agents", "scope-glob", "scope-tags", "scope-roles",
		"suppress-alerts", "suppress-drift", "allow-operations",
		"require-approval", "notify-before", "notify-channel", "label",
	}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewMaintenanceExtendCmd(t *testing.T) {
	cmd := newMaintenanceExtendCmd()

	if cmd == nil {
		t.Fatal("newMaintenanceExtendCmd should not return nil")
	}
	if cmd.Use != "extend <window-id>" {
		t.Errorf("Use = %v, want 'extend <window-id>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"end", "duration"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestScheduleListOptionsStructure(t *testing.T) {
	opts := ScheduleListOptions{
		Type:   "command",
		Status: "active",
		Labels: []string{"env:prod"},
		Limit:  50,
	}

	if opts.Type != "command" {
		t.Errorf("Type = %v, want command", opts.Type)
	}
	if opts.Status != "active" {
		t.Errorf("Status = %v, want active", opts.Status)
	}
	if opts.Limit != 50 {
		t.Errorf("Limit = %d, want 50", opts.Limit)
	}
	if len(opts.Labels) != 1 {
		t.Errorf("Labels count = %d, want 1", len(opts.Labels))
	}
}

func TestScheduleCreateOptionsStructure(t *testing.T) {
	opts := ScheduleCreateOptions{
		Name:              "daily-backup",
		Description:       "Daily backup job",
		Type:              "command",
		Cron:              "0 2 * * *",
		Timezone:          "UTC",
		TargetAll:         true,
		Command:           "backup.sh",
		Priority:          10,
		Timeout:           "1h",
		RequireApproval:   false,
		MaintenanceWindow: "maint-001",
	}

	if opts.Name != "daily-backup" {
		t.Errorf("Name = %v, want daily-backup", opts.Name)
	}
	if opts.Cron != "0 2 * * *" {
		t.Errorf("Cron = %v, want '0 2 * * *'", opts.Cron)
	}
	if !opts.TargetAll {
		t.Error("TargetAll should be true")
	}
	if opts.Priority != 10 {
		t.Errorf("Priority = %d, want 10", opts.Priority)
	}
}

func TestMaintenanceListOptionsStructure(t *testing.T) {
	opts := MaintenanceListOptions{
		Status: "active",
		Type:   "planned",
		Labels: []string{"team:ops"},
		Limit:  20,
	}

	if opts.Status != "active" {
		t.Errorf("Status = %v, want active", opts.Status)
	}
	if opts.Type != "planned" {
		t.Errorf("Type = %v, want planned", opts.Type)
	}
	if opts.Limit != 20 {
		t.Errorf("Limit = %d, want 20", opts.Limit)
	}
}

func TestMaintenanceCreateOptionsStructure(t *testing.T) {
	opts := MaintenanceCreateOptions{
		Name:                   "weekly-patching",
		Description:            "Weekly security patching",
		Type:                   "planned",
		StartTime:              "2024-01-15T02:00:00Z",
		EndTime:                "2024-01-15T06:00:00Z",
		Timezone:               "UTC",
		ScopeAll:               false,
		ScopeTags:              []string{"env:prod"},
		SuppressAlerts:         true,
		SuppressDriftDetection: true,
		AllowOperations:        false,
		RequireApproval:        true,
		NotifyBefore:           "15m",
	}

	if opts.Name != "weekly-patching" {
		t.Errorf("Name = %v, want weekly-patching", opts.Name)
	}
	if opts.Type != "planned" {
		t.Errorf("Type = %v, want planned", opts.Type)
	}
	if !opts.SuppressAlerts {
		t.Error("SuppressAlerts should be true")
	}
	if !opts.RequireApproval {
		t.Error("RequireApproval should be true")
	}
}

func TestScheduleDisplayStructure(t *testing.T) {
	s := scheduleDisplay{
		ID:       "sched-001",
		Name:     "daily-backup",
		Type:     schedule.ScheduleTypeCommand,
		Status:   schedule.ScheduleStatusActive,
		Cron:     "0 2 * * *",
		NextRun:  "02:00",
	}

	if s.ID != "sched-001" {
		t.Errorf("ID = %v, want sched-001", s.ID)
	}
	if s.Type != schedule.ScheduleTypeCommand {
		t.Errorf("Type = %v, want command", s.Type)
	}
	if s.Status != schedule.ScheduleStatusActive {
		t.Errorf("Status = %v, want active", s.Status)
	}
}

func TestScheduleDetailStructure(t *testing.T) {
	s := scheduleDetail{
		ID:              "sched-001",
		Name:            "daily-backup",
		Description:     "Daily backup of databases",
		Type:            schedule.ScheduleTypeCommand,
		Status:          schedule.ScheduleStatusActive,
		Cron:            "0 2 * * *",
		Timezone:        "UTC",
		Priority:        10,
		Timeout:         "1h",
		RunCount:        100,
		SuccessCount:    98,
		FailureCount:    2,
		RequireApproval: false,
		CreatedBy:       "admin",
	}

	if s.ID != "sched-001" {
		t.Errorf("ID = %v, want sched-001", s.ID)
	}
	if s.RunCount != 100 {
		t.Errorf("RunCount = %d, want 100", s.RunCount)
	}
	if s.SuccessCount != 98 {
		t.Errorf("SuccessCount = %d, want 98", s.SuccessCount)
	}
}

func TestWindowDisplayStructure(t *testing.T) {
	w := windowDisplay{
		ID:         "maint-001",
		Name:       "weekly-patching",
		Type:       schedule.MaintenanceWindowTypePlanned,
		Status:     schedule.MaintenanceWindowStatusScheduled,
		StartTime:  "Jan 15 02:00",
		EndTime:    "Jan 15 06:00",
		ScopeAll:   false,
		AgentCount: 15,
	}

	if w.ID != "maint-001" {
		t.Errorf("ID = %v, want maint-001", w.ID)
	}
	if w.Type != schedule.MaintenanceWindowTypePlanned {
		t.Errorf("Type = %v, want planned", w.Type)
	}
	if w.AgentCount != 15 {
		t.Errorf("AgentCount = %d, want 15", w.AgentCount)
	}
}

func TestWindowDetailStructure(t *testing.T) {
	w := windowDetail{
		ID:              "maint-001",
		Name:            "weekly-patching",
		Description:     "Weekly security patching",
		Type:            schedule.MaintenanceWindowTypePlanned,
		Status:          schedule.MaintenanceWindowStatusScheduled,
		StartTime:       "2024-01-15T02:00:00Z",
		EndTime:         "2024-01-15T06:00:00Z",
		Timezone:        "UTC",
		RequireApproval: true,
		CreatedBy:       "admin",
	}

	if w.ID != "maint-001" {
		t.Errorf("ID = %v, want maint-001", w.ID)
	}
	if !w.RequireApproval {
		t.Error("RequireApproval should be true")
	}
}

func TestBehaviorDisplayStructure(t *testing.T) {
	b := behaviorDisplay{
		SuppressAlerts:         true,
		SuppressDriftDetection: true,
		AllowOperations:        false,
	}

	if !b.SuppressAlerts {
		t.Error("SuppressAlerts should be true")
	}
	if !b.SuppressDriftDetection {
		t.Error("SuppressDriftDetection should be true")
	}
	if b.AllowOperations {
		t.Error("AllowOperations should be false")
	}
}

func TestExecutionDisplayStructure(t *testing.T) {
	e := executionDisplay{
		ID:           "exec-001",
		Status:       "completed",
		Trigger:      "scheduled",
		StartTime:    "Jan 15 02:00",
		Duration:     "5m30s",
		SuccessCount: 10,
		FailureCount: 0,
	}

	if e.ID != "exec-001" {
		t.Errorf("ID = %v, want exec-001", e.ID)
	}
	if e.Status != "completed" {
		t.Errorf("Status = %v, want completed", e.Status)
	}
	if e.SuccessCount != 10 {
		t.Errorf("SuccessCount = %d, want 10", e.SuccessCount)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcdef", 4, "a..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		input    []string
		expected map[string]string
	}{
		{
			input:    []string{"env:prod", "team:ops"},
			expected: map[string]string{"env": "prod", "team": "ops"},
		},
		{
			input:    []string{},
			expected: map[string]string{},
		},
		{
			input:    []string{"key:value:extra"},
			expected: map[string]string{"key": "value:extra"},
		},
	}

	for _, tt := range tests {
		result := parseLabels(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseLabels(%v) len = %d, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for k, v := range tt.expected {
			if result[k] != v {
				t.Errorf("parseLabels(%v)[%q] = %q, want %q", tt.input, k, result[k], v)
			}
		}
	}
}

func TestGenerateSampleSchedules(t *testing.T) {
	schedules := generateSampleSchedules()

	if len(schedules) == 0 {
		t.Error("generateSampleSchedules should return at least one schedule")
	}

	for i, s := range schedules {
		if s.ID == "" {
			t.Errorf("schedule[%d] ID should not be empty", i)
		}
		if s.Name == "" {
			t.Errorf("schedule[%d] Name should not be empty", i)
		}
	}
}

func TestGenerateSampleWindows(t *testing.T) {
	windows := generateSampleWindows()

	if len(windows) == 0 {
		t.Error("generateSampleWindows should return at least one window")
	}

	for i, w := range windows {
		if w.ID == "" {
			t.Errorf("window[%d] ID should not be empty", i)
		}
		if w.Name == "" {
			t.Errorf("window[%d] Name should not be empty", i)
		}
	}
}

func TestGenerateSampleExecutions(t *testing.T) {
	executions := generateSampleExecutions("sched-001", 5)

	if len(executions) != 5 {
		t.Errorf("generateSampleExecutions count = %d, want 5", len(executions))
	}

	for i, e := range executions {
		if e.ID == "" {
			t.Errorf("execution[%d] ID should not be empty", i)
		}
		if e.Status == "" {
			t.Errorf("execution[%d] Status should not be empty", i)
		}
	}
}

func TestHistoryOptionsStructure(t *testing.T) {
	opts := HistoryOptions{
		Limit:  20,
		Status: "completed",
	}

	if opts.Limit != 20 {
		t.Errorf("Limit = %d, want 20", opts.Limit)
	}
	if opts.Status != "completed" {
		t.Errorf("Status = %v, want completed", opts.Status)
	}
}
