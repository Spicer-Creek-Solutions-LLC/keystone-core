package main

import (
	"bytes"
	"testing"
	"time"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-events" {
		t.Errorf("Use = %v, want kscore-events", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"version",
		"list",
		"query",
		"emit",
		"replay",
		"watch",
		"retention",
		"dlq",
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
	cmd := newListCmd()

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check aliases
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}

	// Check flags exist
	flags := []string{"type", "source", "severity", "since", "before", "correlation-id", "limit", "tag"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewQueryCmd(t *testing.T) {
	cmd := newQueryCmd()

	if cmd == nil {
		t.Fatal("newQueryCmd should not return nil")
	}
	if cmd.Use != "query <filter-expression>" {
		t.Errorf("Use = %v, want 'query <filter-expression>'", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected flag 'limit' not found")
	}
}

func TestNewEmitCmd(t *testing.T) {
	cmd := newEmitCmd()

	if cmd == nil {
		t.Fatal("newEmitCmd should not return nil")
	}
	if cmd.Use != "emit" {
		t.Errorf("Use = %v, want emit", cmd.Use)
	}

	// Check flags exist
	flags := []string{"type", "source", "severity", "correlation-id", "tag", "data", "data-file"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewReplayCmd(t *testing.T) {
	cmd := newReplayCmd()

	if cmd == nil {
		t.Fatal("newReplayCmd should not return nil")
	}
	if cmd.Use != "replay" {
		t.Errorf("Use = %v, want replay", cmd.Use)
	}

	// Check flags exist
	flags := []string{"type", "source", "since", "before", "correlation-id", "filter", "dry-run", "speed"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewWatchCmd(t *testing.T) {
	cmd := newWatchCmd()

	if cmd == nil {
		t.Fatal("newWatchCmd should not return nil")
	}
	if cmd.Use != "watch" {
		t.Errorf("Use = %v, want watch", cmd.Use)
	}

	// Check flags exist
	flags := []string{"type", "source", "severity", "filter", "tag"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewRetentionCmd(t *testing.T) {
	cmd := newRetentionCmd()

	if cmd == nil {
		t.Fatal("newRetentionCmd should not return nil")
	}
	if cmd.Use != "retention" {
		t.Errorf("Use = %v, want retention", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"list", "set", "apply"}
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

func TestNewDLQCmd(t *testing.T) {
	cmd := newDLQCmd()

	if cmd == nil {
		t.Fatal("newDLQCmd should not return nil")
	}
	if cmd.Use != "dlq" {
		t.Errorf("Use = %v, want dlq", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"list", "show", "retry", "purge"}
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

func TestEventDisplayStructure(t *testing.T) {
	now := time.Now()
	event := EventDisplay{
		ID:            "evt-123",
		Type:          "agent.connect",
		Source:        "/agents/web-01",
		Severity:      "info",
		Time:          now,
		CorrelationID: "corr-456",
	}

	if event.ID != "evt-123" {
		t.Errorf("ID = %v, want evt-123", event.ID)
	}
	if event.Type != "agent.connect" {
		t.Errorf("Type = %v, want agent.connect", event.Type)
	}
	if event.Severity != "info" {
		t.Errorf("Severity = %v, want info", event.Severity)
	}
}

func TestListOptionsStructure(t *testing.T) {
	opts := ListOptions{
		Type:          "agent.*",
		Source:        "/agents/web-01",
		Severity:      "warning",
		Since:         "1h",
		Before:        "30m",
		CorrelationID: "corr-123",
		Limit:         100,
		Tags:          []string{"env:prod"},
	}

	if opts.Type != "agent.*" {
		t.Errorf("Type = %v, want agent.*", opts.Type)
	}
	if opts.Limit != 100 {
		t.Errorf("Limit = %d, want 100", opts.Limit)
	}
	if len(opts.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(opts.Tags))
	}
}

func TestEmitOptionsStructure(t *testing.T) {
	opts := EmitOptions{
		Type:          "custom.event",
		Source:        "/cli/test",
		Severity:      "info",
		CorrelationID: "corr-123",
		Tags:          []string{"env:prod"},
		Data:          `{"key":"value"}`,
		DataFile:      "/path/to/data.json",
	}

	if opts.Type != "custom.event" {
		t.Errorf("Type = %v, want custom.event", opts.Type)
	}
	if opts.Severity != "info" {
		t.Errorf("Severity = %v, want info", opts.Severity)
	}
}

func TestReplayOptionsStructure(t *testing.T) {
	opts := ReplayOptions{
		Type:          "state.*",
		Source:        "/state-manager",
		Since:         "1h",
		Before:        "30m",
		CorrelationID: "job-123",
		Filter:        "severity >= warning",
		DryRun:        true,
		Speed:         2.0,
	}

	if opts.Since != "1h" {
		t.Errorf("Since = %v, want 1h", opts.Since)
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
	if opts.Speed != 2.0 {
		t.Errorf("Speed = %v, want 2.0", opts.Speed)
	}
}

func TestWatchOptionsStructure(t *testing.T) {
	opts := WatchOptions{
		Type:     "agent.*",
		Source:   "/agents/*",
		Severity: "warning",
		Filter:   "tags.env == prod",
		Tags:     []string{"env:prod"},
	}

	if opts.Type != "agent.*" {
		t.Errorf("Type = %v, want agent.*", opts.Type)
	}
	if opts.Severity != "warning" {
		t.Errorf("Severity = %v, want warning", opts.Severity)
	}
}

func TestGenerateSampleEvents(t *testing.T) {
	events := generateSampleEvents(10)

	if len(events) != 10 {
		t.Errorf("events count = %d, want 10", len(events))
	}

	for i, event := range events {
		if event.ID == "" {
			t.Errorf("event[%d] ID should not be empty", i)
		}
		if event.Type == "" {
			t.Errorf("event[%d] Type should not be empty", i)
		}
		if event.Source == "" {
			t.Errorf("event[%d] Source should not be empty", i)
		}
		if event.Severity == "" {
			t.Errorf("event[%d] Severity should not be empty", i)
		}
	}
}

func TestFilterEvents(t *testing.T) {
	events := generateSampleEvents(20)

	// Test limit filtering
	opts := &ListOptions{Limit: 5}
	filtered := filterEvents(events, opts)
	if len(filtered) != 5 {
		t.Errorf("filtered count = %d, want 5", len(filtered))
	}

	// Test type filtering
	opts = &ListOptions{Type: "agent.connect", Limit: 100}
	filtered = filterEvents(events, opts)
	for _, event := range filtered {
		if event.Type != "agent.connect" {
			t.Errorf("filtered event type = %v, want agent.connect", event.Type)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
