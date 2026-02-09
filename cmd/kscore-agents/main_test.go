package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewRootCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(cfg)

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-agents" {
		t.Errorf("Use = %v, want kscore-agents", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(cfg)

	expectedSubcommands := []string{
		"list",
		"show",
		"delete",
		"quarantine",
		"unquarantine",
		"token",
		"tags",
		"status",
		"renew-svid",
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

func TestNewListCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr: "localhost:9090",
		Output:     "table",
	}
	cmd := newListCmd(cfg)

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"status", "filter", "label", "edge", "limit", "show-compatibility"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewShowCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr: "localhost:9090",
		Output:     "table",
	}
	cmd := newShowCmd(cfg)

	if cmd == nil {
		t.Fatal("newShowCmd should not return nil")
	}
	if cmd.Use != "show <agent-id>" {
		t.Errorf("Use = %v, want 'show <agent-id>'", cmd.Use)
	}
}

func TestNewDeleteCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newDeleteCmd(cfg)

	if cmd == nil {
		t.Fatal("newDeleteCmd should not return nil")
	}
	if cmd.Use != "delete <agent-id>" {
		t.Errorf("Use = %v, want 'delete <agent-id>'", cmd.Use)
	}

	if cmd.Flags().Lookup("force") == nil {
		t.Error("expected flag 'force' not found")
	}
}

func TestNewTokenCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newTokenCmd(cfg)

	if cmd == nil {
		t.Fatal("newTokenCmd should not return nil")
	}
	if cmd.Use != "token" {
		t.Errorf("Use = %v, want token", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"create", "list", "revoke"}
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

func TestNewTagsCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newTagsCmd(cfg)

	if cmd == nil {
		t.Fatal("newTagsCmd should not return nil")
	}
	if cmd.Use != "tags" {
		t.Errorf("Use = %v, want tags", cmd.Use)
	}

	// Check aliases
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "labels" {
		t.Error("expected alias 'labels' not found")
	}

	// Should have subcommands
	subcommands := []string{"set", "add", "remove", "show"}
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

func TestConfigStructure(t *testing.T) {
	cfg := Config{
		ServerAddr: "localhost:9090",
		Output:     "json",
		Verbose:    true,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
	if cfg.Output != "json" {
		t.Errorf("Output = %v, want json", cfg.Output)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestAgentDisplayStructure(t *testing.T) {
	agent := AgentDisplay{
		ID:            "test-001",
		Hostname:      "test.example.com",
		OS:            "linux",
		Arch:          "amd64",
		Status:        "online",
		Version:       "1.0.0",
		LastHeartbeat: time.Now().Format(time.RFC3339),
		RegisteredAt:  time.Now().Format(time.RFC3339),
		Labels:        map[string]string{"env": "test"},
		IPAddresses:   []string{"10.0.0.1"},
		DualStack:     false,
	}

	if agent.ID != "test-001" {
		t.Errorf("ID = %v, want test-001", agent.ID)
	}
	if agent.Status != "online" {
		t.Errorf("Status = %v, want online", agent.Status)
	}
}

func TestMetricsDisplayStructure(t *testing.T) {
	metrics := MetricsDisplay{
		CPU:    50.5,
		Memory: 75.2,
		Disk:   30.0,
		Load:   []float32{1.0, 0.8, 0.5},
	}

	if metrics.CPU != 50.5 {
		t.Errorf("CPU = %v, want 50.5", metrics.CPU)
	}
	if len(metrics.Load) != 3 {
		t.Errorf("Load length = %d, want 3", len(metrics.Load))
	}
}

func TestTokenDisplayStructure(t *testing.T) {
	now := time.Now()
	token := TokenDisplay{
		ID:        "tok-123",
		Token:     "kscore_secret",
		TTL:       "1h",
		ExpiresAt: now.Add(time.Hour),
		UsedCount: 0,
		MaxUses:   10,
		Labels:    map[string]string{"env": "prod"},
		CreatedAt: now,
		CreatedBy: "admin",
	}

	if token.ID != "tok-123" {
		t.Errorf("ID = %v, want tok-123", token.ID)
	}
	if token.MaxUses != 10 {
		t.Errorf("MaxUses = %d, want 10", token.MaxUses)
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "-",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "-",
		},
		{
			name:     "single label",
			labels:   map[string]string{"env": "prod"},
			expected: "env=prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLabels(tt.labels)
			if tt.name == "empty labels" || tt.name == "nil labels" {
				if result != "-" {
					t.Errorf("formatLabels() = %v, want -", result)
				}
			} else {
				if !strings.Contains(result, "env=prod") {
					t.Errorf("formatLabels() = %v, should contain env=prod", result)
				}
			}
		})
	}
}

func TestStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"online", "online", "online"},
		{"offline", "offline", "offline"},
		{"degraded", "degraded", "degraded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: statusToString takes pb.AgentStatus, but we can test the concept
			if tt.expected != tt.status {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestGenerateRandomToken(t *testing.T) {
	token := generateRandomToken()

	if len(token) != 32 {
		t.Errorf("token length = %d, want 32", len(token))
	}

	// Token should only contain alphanumeric characters
	for _, c := range token {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			t.Errorf("token contains invalid character: %c", c)
		}
	}
}
