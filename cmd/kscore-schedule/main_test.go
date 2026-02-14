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
	cfg := &Config{}
	cmd := newScheduleCmd(cfg)

	if cmd == nil {
		t.Fatal("newScheduleCmd should not return nil")
	}
	if cmd.Use != "schedule" {
		t.Errorf("Use = %v, want schedule", cmd.Use)
	}

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
	cfg := &Config{}
	cmd := newScheduleListCmd(cfg)

	if cmd == nil {
		t.Fatal("newScheduleListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}

	flags := []string{"type", "status", "label", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewScheduleCreateCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newScheduleCreateCmd(cfg)

	if cmd == nil {
		t.Fatal("newScheduleCreateCmd should not return nil")
	}
	if cmd.Use != "create" {
		t.Errorf("Use = %v, want create", cmd.Use)
	}

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
	cfg := &Config{}
	cmd := newScheduleHistoryCmd(cfg)

	if cmd == nil {
		t.Fatal("newScheduleHistoryCmd should not return nil")
	}
	if cmd.Use != "history <schedule-id>" {
		t.Errorf("Use = %v, want 'history <schedule-id>'", cmd.Use)
	}

	flags := []string{"limit", "status"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewMaintenanceCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newMaintenanceCmd(cfg)

	if cmd == nil {
		t.Fatal("newMaintenanceCmd should not return nil")
	}
	if cmd.Use != "maintenance" {
		t.Errorf("Use = %v, want maintenance", cmd.Use)
	}

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
	cfg := &Config{}
	cmd := newMaintenanceListCmd(cfg)

	if cmd == nil {
		t.Fatal("newMaintenanceListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	flags := []string{"status", "type", "label", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewMaintenanceCreateCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newMaintenanceCreateCmd(cfg)

	if cmd == nil {
		t.Fatal("newMaintenanceCreateCmd should not return nil")
	}
	if cmd.Use != "create" {
		t.Errorf("Use = %v, want create", cmd.Use)
	}

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
	cfg := &Config{}
	cmd := newMaintenanceExtendCmd(cfg)

	if cmd == nil {
		t.Fatal("newMaintenanceExtendCmd should not return nil")
	}
	if cmd.Use != "extend <window-id>" {
		t.Errorf("Use = %v, want 'extend <window-id>'", cmd.Use)
	}

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

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{
		ServerAddr:   "localhost:9090",
		OutputFormat: "table",
		Verbose:      false,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		addr     string
		wantBase string
	}{
		{"localhost:9090", "http://localhost:9090"},
		{"http://localhost:9090", "http://localhost:9090"},
		{"https://server.example.com", "https://server.example.com"},
	}

	for _, tt := range tests {
		cfg := &Config{ServerAddr: tt.addr}
		c := newClient(cfg)
		if c.baseURL != tt.wantBase {
			t.Errorf("newClient(%q).baseURL = %q, want %q", tt.addr, c.baseURL, tt.wantBase)
		}
	}
}

func TestRootCmdFlags(t *testing.T) {
	cmd := newRootCmd()

	flags := []string{"server", "output", "verbose"}
	for _, flag := range flags {
		if cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("expected persistent flag %q not found", flag)
		}
	}
}
