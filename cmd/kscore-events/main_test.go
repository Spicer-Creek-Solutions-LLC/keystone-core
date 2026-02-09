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
