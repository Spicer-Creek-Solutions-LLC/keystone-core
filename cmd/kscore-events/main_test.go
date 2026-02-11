package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		"subscribe",
		"export",
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
	for _, flag := range []string{"limit", "since", "until"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}

	// Check -n shorthand for --limit
	f := cmd.Flags().ShorthandLookup("n")
	if f == nil {
		t.Error("expected shorthand -n for --limit not found")
	} else if f.Name != "limit" {
		t.Errorf("-n shorthand maps to %q, want 'limit'", f.Name)
	}

	// Check defaults
	if cmd.Flags().Lookup("limit").DefValue != "100" {
		t.Errorf("limit default = %v, want 100", cmd.Flags().Lookup("limit").DefValue)
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
	flags := []string{"type", "source", "severity", "filter", "tag", "format"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}

	// Check format flag default
	f := cmd.Flags().Lookup("format")
	if f.DefValue != "text" {
		t.Errorf("format default = %v, want text", f.DefValue)
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

func TestDLQListReasonFilter(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--reason", "timeout"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "reactor timeout") {
		t.Error("output should contain matching reason 'reactor timeout'")
	}
	if strings.Contains(out, "connection refused") {
		t.Error("output should not contain non-matching reason 'connection refused'")
	}
	if strings.Contains(out, "invalid payload") {
		t.Error("output should not contain non-matching reason 'invalid payload'")
	}
}

func TestDLQListLimitShorthand(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "-n", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 event(s)") {
		t.Errorf("output should show 1 event, got:\n%s", out)
	}
}

func TestDLQListReasonNoMatch(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--reason", "nonexistent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dead letter queue is empty") {
		t.Errorf("expected empty queue message, got:\n%s", out)
	}
}

func TestDLQRetryByType(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"retry", "--type", "webhook.delivery"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"webhook.delivery"`) {
		t.Errorf("output should mention the event type, got:\n%s", out)
	}
	if !strings.Contains(out, "Retried: 2 event(s)") {
		t.Errorf("output should show retry count, got:\n%s", out)
	}
}

func TestDLQRetryRequiresArg(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"retry"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no event-id, --all, or --type provided")
	}
}

func TestDLQPurgeOlderThan(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"purge", "--older-than", "7d", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "older than 7d") {
		t.Errorf("output should describe the filter, got:\n%s", out)
	}
	if !strings.Contains(out, "Purged") {
		t.Errorf("output should confirm purge, got:\n%s", out)
	}
}

func TestDLQPurgeReason(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"purge", "--reason", "timeout", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `reason matching "timeout"`) {
		t.Errorf("output should describe the reason filter, got:\n%s", out)
	}
}

func TestDLQPurgeInvalidOlderThan(t *testing.T) {
	cmd := newDLQCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"purge", "--older-than", "invalid", "--force"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --older-than value")
	}
}

func TestDLQListFlags(t *testing.T) {
	cmd := newDLQListCmd()
	for _, flag := range []string{"limit", "reason"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
	if cmd.Flags().ShorthandLookup("n") == nil {
		t.Error("expected -n shorthand for --limit")
	}
}

func TestDLQRetryFlags(t *testing.T) {
	cmd := newDLQRetryCmd()
	for _, flag := range []string{"all", "type"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestDLQPurgeFlags(t *testing.T) {
	cmd := newDLQPurgeCmd()
	for _, flag := range []string{"force", "older-than", "reason"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
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
		Format:   "jsonl",
	}

	if opts.Type != "agent.*" {
		t.Errorf("Type = %v, want agent.*", opts.Type)
	}
	if opts.Severity != "warning" {
		t.Errorf("Severity = %v, want warning", opts.Severity)
	}
	if opts.Format != "jsonl" {
		t.Errorf("Format = %v, want jsonl", opts.Format)
	}
}

func TestRunWatchFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, output string)
	}{
		{
			name:   "text format",
			format: "text",
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "Watching events") {
					t.Error("text output should contain header")
				}
				if !strings.Contains(output, "Watch stopped") {
					t.Error("text output should contain footer")
				}
			},
		},
		{
			name:   "jsonl format",
			format: "jsonl",
			check: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) == 0 {
					t.Fatal("jsonl output should have lines")
				}
				for _, line := range lines {
					var evt map[string]interface{}
					if err := json.Unmarshal([]byte(line), &evt); err != nil {
						t.Errorf("line is not valid JSON: %v", err)
					}
					if _, ok := evt["type"]; !ok {
						t.Error("jsonl event should have type field")
					}
					if _, ok := evt["timestamp"]; !ok {
						t.Error("jsonl event should have timestamp field")
					}
				}
			},
		},
		{
			name:   "json format",
			format: "json",
			check: func(t *testing.T, output string) {
				var events []map[string]interface{}
				if err := json.Unmarshal([]byte(output), &events); err != nil {
					t.Fatalf("json output is not valid JSON array: %v", err)
				}
				if len(events) == 0 {
					t.Error("json output should have events")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newWatchCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"--format", tt.format})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, buf.String())
		})
	}
}

func TestRunWatchInvalidFormat(t *testing.T) {
	cmd := newWatchCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--format", "xml"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid format")
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

func TestFilterEventsBySeverity(t *testing.T) {
	events := []EventDisplay{
		{ID: "1", Severity: "debug", Time: time.Now()},
		{ID: "2", Severity: "info", Time: time.Now()},
		{ID: "3", Severity: "warning", Time: time.Now()},
		{ID: "4", Severity: "error", Time: time.Now()},
		{ID: "5", Severity: "critical", Time: time.Now()},
	}

	tests := []struct {
		severity string
		wantIDs  []string
	}{
		{"debug", []string{"1", "2", "3", "4", "5"}},
		{"info", []string{"2", "3", "4", "5"}},
		{"warning", []string{"3", "4", "5"}},
		{"error", []string{"4", "5"}},
		{"critical", []string{"5"}},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			opts := &ListOptions{Severity: tt.severity}
			filtered := filterEvents(events, opts)
			if len(filtered) != len(tt.wantIDs) {
				t.Fatalf("severity=%q: got %d events, want %d", tt.severity, len(filtered), len(tt.wantIDs))
			}
			for i, e := range filtered {
				if e.ID != tt.wantIDs[i] {
					t.Errorf("severity=%q: event[%d].ID = %q, want %q", tt.severity, i, e.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterEventsByTimeRange(t *testing.T) {
	now := time.Now()
	events := []EventDisplay{
		{ID: "old", Time: now.Add(-3 * time.Hour)},
		{ID: "mid", Time: now.Add(-1 * time.Hour)},
		{ID: "new", Time: now.Add(-10 * time.Minute)},
	}

	// Since 2h ago should exclude old
	opts := &ListOptions{Since: "2h"}
	filtered := filterEvents(events, opts)
	if len(filtered) != 2 {
		t.Fatalf("since=2h: got %d events, want 2", len(filtered))
	}
	if filtered[0].ID != "mid" || filtered[1].ID != "new" {
		t.Errorf("since=2h: got IDs %v, want [mid new]", []string{filtered[0].ID, filtered[1].ID})
	}

	// Before 30m ago should exclude new
	opts = &ListOptions{Before: "30m"}
	filtered = filterEvents(events, opts)
	if len(filtered) != 2 {
		t.Fatalf("before=30m: got %d events, want 2", len(filtered))
	}
	if filtered[0].ID != "old" || filtered[1].ID != "mid" {
		t.Errorf("before=30m: got IDs %v, want [old mid]", []string{filtered[0].ID, filtered[1].ID})
	}

	// Combined since+before
	opts = &ListOptions{Since: "2h", Before: "30m"}
	filtered = filterEvents(events, opts)
	if len(filtered) != 1 {
		t.Fatalf("since=2h,before=30m: got %d events, want 1", len(filtered))
	}
	if filtered[0].ID != "mid" {
		t.Errorf("since=2h,before=30m: got ID %q, want mid", filtered[0].ID)
	}
}

func TestFilterEventsByRFC3339Time(t *testing.T) {
	now := time.Now()
	events := []EventDisplay{
		{ID: "1", Time: now.Add(-2 * time.Hour)},
		{ID: "2", Time: now.Add(-30 * time.Minute)},
	}

	since := now.Add(-1 * time.Hour).Format(time.RFC3339)
	opts := &ListOptions{Since: since}
	filtered := filterEvents(events, opts)
	if len(filtered) != 1 {
		t.Fatalf("RFC3339 since: got %d events, want 1", len(filtered))
	}
	if filtered[0].ID != "2" {
		t.Errorf("RFC3339 since: got ID %q, want 2", filtered[0].ID)
	}
}

func TestFilterEventsCombinedFilters(t *testing.T) {
	now := time.Now()
	events := []EventDisplay{
		{ID: "1", Type: "agent.connect", Severity: "info", Source: "/agents/web-01", Time: now.Add(-30 * time.Minute)},
		{ID: "2", Type: "agent.connect", Severity: "error", Source: "/agents/web-01", Time: now.Add(-30 * time.Minute)},
		{ID: "3", Type: "state.change", Severity: "error", Source: "/agents/web-01", Time: now.Add(-30 * time.Minute)},
		{ID: "4", Type: "agent.connect", Severity: "error", Source: "/agents/db-01", Time: now.Add(-3 * time.Hour)},
	}

	opts := &ListOptions{
		Type:     "agent.connect",
		Severity: "error",
		Source:   "web-01",
		Since:    "1h",
	}
	filtered := filterEvents(events, opts)
	if len(filtered) != 1 {
		t.Fatalf("combined: got %d events, want 1", len(filtered))
	}
	if filtered[0].ID != "2" {
		t.Errorf("combined: got ID %q, want 2", filtered[0].ID)
	}
}

func TestParseTimeOrDuration(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"1h", false},
		{"30m", false},
		{"7d", false},
		{"60s", false},
		{time.Now().Format(time.RFC3339), false},
		{"invalid", true},
		{"x", true},
		{"10x", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseTimeOrDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeOrDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
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

func TestNewSubscribeCmd(t *testing.T) {
	cmd := newSubscribeCmd()

	if cmd == nil {
		t.Fatal("newSubscribeCmd should not return nil")
	}
	if cmd.Use != "subscribe <pattern>" {
		t.Errorf("Use = %v, want 'subscribe <pattern>'", cmd.Use)
	}

	flags := []string{"filter-type", "filter-severity", "output"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestSubscribeRequiresPattern(t *testing.T) {
	cmd := newSubscribeCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no pattern argument is provided")
	}
}

func TestSubscribeTableOutput(t *testing.T) {
	cmd := newSubscribeCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"agent.*"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Subscribed to agent.*...") {
		t.Error("output should contain subscription confirmation message")
	}
	if !strings.Contains(out, "agent.") {
		t.Error("output should contain matching agent events")
	}
}

func TestSubscribeJSONOutput(t *testing.T) {
	cmd := newSubscribeCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"agent.*", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Subscribed to agent.*...") {
		t.Error("output should contain subscription confirmation message")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		var event EventDisplay
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("expected valid JSON line, got parse error: %v for line: %s", err, line)
		}
	}
}

func TestSubscribeSeverityFilter(t *testing.T) {
	cmd := newSubscribeCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"*", "--filter-severity", "warning"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "  debug\n") {
		t.Error("output should not contain debug events when filtering for warning+")
	}
	if strings.Contains(out, "  info\n") {
		t.Error("output should not contain info events when filtering for warning+")
	}
}

func TestSubscribeInvalidSeverity(t *testing.T) {
	cmd := newSubscribeCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"*", "--filter-severity", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid severity")
	}
}

func TestSubscribeOptionsStructure(t *testing.T) {
	opts := SubscribeOptions{
		FilterType:     "agent.connect",
		FilterSeverity: "warning",
		Output:         "json",
	}

	if opts.FilterType != "agent.connect" {
		t.Errorf("FilterType = %v, want agent.connect", opts.FilterType)
	}
	if opts.FilterSeverity != "warning" {
		t.Errorf("FilterSeverity = %v, want warning", opts.FilterSeverity)
	}
	if opts.Output != "json" {
		t.Errorf("Output = %v, want json", opts.Output)
	}
}

func TestNewExportCmd(t *testing.T) {
	cmd := newExportCmd()

	if cmd == nil {
		t.Fatal("newExportCmd should not return nil")
	}
	if cmd.Use != "export" {
		t.Errorf("Use = %v, want export", cmd.Use)
	}

	flags := []string{"output", "format", "start", "end", "type", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestExportRequiresOutput(t *testing.T) {
	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --output is not provided")
	}
}

func TestExportJSON(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "events.json")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--output", outFile, "--limit", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Exported 5 event(s)") {
		t.Errorf("expected export confirmation, got: %s", out)
	}
	if !strings.Contains(out, "format: json") {
		t.Errorf("expected format: json in output, got: %s", out)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var events []EventDisplay
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events in JSON, got %d", len(events))
	}
}

func TestExportCSV(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "events.csv")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--output", outFile, "--format", "csv", "--limit", "3"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Exported 3 event(s)") {
		t.Errorf("expected export confirmation, got: %s", out)
	}
	if !strings.Contains(out, "format: csv") {
		t.Errorf("expected format: csv in output, got: %s", out)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// header + 3 data rows
	if len(lines) != 4 {
		t.Errorf("expected 4 lines in CSV (header + 3 rows), got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "id,type,source,severity,timestamp,correlation_id") {
		t.Errorf("unexpected CSV header: %s", lines[0])
	}
}

func TestExportWithTypeFilter(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "filtered.json")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--output", outFile, "--type", "agent.connect", "--limit", "20"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var events []EventDisplay
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	for _, e := range events {
		if e.Type != "agent.connect" {
			t.Errorf("expected all events to be agent.connect, got %s", e.Type)
		}
	}
}

func TestExportInvalidFormat(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "events.txt")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--output", outFile, "--format", "xml"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestExportInvalidStartTime(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "events.json")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--output", outFile, "--start", "not-a-time"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid start time")
	}
}

func TestExportInvalidEndTime(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "events.json")

	cmd := newExportCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--output", outFile, "--end", "not-a-time"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid end time")
	}
}

func TestExportOptionsStructure(t *testing.T) {
	opts := ExportOptions{
		Output: "events.json",
		Format: "csv",
		Start:  "2024-01-01T00:00:00Z",
		End:    "2024-01-02T00:00:00Z",
		Type:   "agent.connect",
		Limit:  500,
	}

	if opts.Output != "events.json" {
		t.Errorf("Output = %v, want events.json", opts.Output)
	}
	if opts.Format != "csv" {
		t.Errorf("Format = %v, want csv", opts.Format)
	}
	if opts.Limit != 500 {
		t.Errorf("Limit = %d, want 500", opts.Limit)
	}
}
