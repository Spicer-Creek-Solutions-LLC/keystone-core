// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	if cmd.Use != "kscore-audit" {
		t.Errorf("expected Use to be 'kscore-audit', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "audit") {
		t.Errorf("expected Short to contain 'audit', got %s", cmd.Short)
	}

	expectedCommands := []string{"version", "log", "report", "export", "stats", "search", "analyze", "timeline", "watch"}
	for _, expected := range expectedCommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %s not found", expected)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Keystone Core") {
		t.Errorf("expected version output to contain 'Keystone Core', got: %s", output)
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "kscore-audit") {
		t.Errorf("expected help output to contain 'kscore-audit', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Fatal("expected --server flag")
	}
	if serverFlag.DefValue != "localhost:9090" {
		t.Errorf("expected server default to be 'localhost:9090', got %s", serverFlag.DefValue)
	}

	formatFlag := cmd.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag")
	}
	if formatFlag.DefValue != "table" {
		t.Errorf("expected format default to be 'table', got %s", formatFlag.DefValue)
	}

	auditLevelFlag := cmd.PersistentFlags().Lookup("audit-level")
	if auditLevelFlag == nil {
		t.Error("expected --audit-level flag")
	}

	auditOutputFlag := cmd.PersistentFlags().Lookup("audit-output")
	if auditOutputFlag == nil {
		t.Error("expected --audit-output flag")
	}
}

func TestLogCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	logCmd := findSubcommand(cmd, "log")
	if logCmd == nil {
		t.Fatal("log subcommand not found")
	}

	policyFlag := logCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on log command")
	}

	resourceTypeFlag := logCmd.Flags().Lookup("resource-type")
	if resourceTypeFlag == nil {
		t.Error("expected --resource-type flag on log command")
	}

	deniedFlag := logCmd.Flags().Lookup("denied")
	if deniedFlag == nil {
		t.Error("expected --denied flag on log command")
	}

	limitFlag := logCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on log command")
	}
	if limitFlag.DefValue != "100" {
		t.Errorf("expected limit default to be '100', got %s", limitFlag.DefValue)
	}

	sinceFlag := logCmd.Flags().Lookup("since")
	if sinceFlag == nil {
		t.Error("expected --since flag on log command")
	}

	untilFlag := logCmd.Flags().Lookup("until")
	if untilFlag == nil {
		t.Error("expected --until flag on log command")
	}
}

func TestReportCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	reportCmd := findSubcommand(cmd, "report")
	if reportCmd == nil {
		t.Fatal("report subcommand not found")
	}

	daysFlag := reportCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected --days flag on report command")
	}
	if daysFlag.DefValue != "7" {
		t.Errorf("expected days default to be '7', got %s", daysFlag.DefValue)
	}

	categoryFlag := reportCmd.Flags().Lookup("category")
	if categoryFlag == nil {
		t.Error("expected --category flag on report command")
	}

	severityFlag := reportCmd.Flags().Lookup("severity")
	if severityFlag == nil {
		t.Error("expected --severity flag on report command")
	}
}

func TestExportCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	exportCmd := findSubcommand(cmd, "export")
	if exportCmd == nil {
		t.Fatal("export subcommand not found")
	}

	daysFlag := exportCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected --days flag on export command")
	}
	if daysFlag.DefValue != "30" {
		t.Errorf("expected days default to be '30', got %s", daysFlag.DefValue)
	}

	outputFlag := exportCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on export command")
	}

	exportFormatFlag := exportCmd.Flags().Lookup("export-format")
	if exportFormatFlag == nil {
		t.Fatal("expected --export-format flag on export command")
	}
	if exportFormatFlag.DefValue != "json" {
		t.Errorf("expected export-format default to be 'json', got %s", exportFormatFlag.DefValue)
	}
}

func TestStatsCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	statsCmd := findSubcommand(cmd, "stats")
	if statsCmd == nil {
		t.Fatal("stats subcommand not found")
	}

	daysFlag := statsCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected --days flag on stats command")
	}
	if daysFlag.DefValue != "7" {
		t.Errorf("expected days default to be '7', got %s", daysFlag.DefValue)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"log", "report", "export", "stats", "search", "analyze", "timeline", "watch"}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{subcmd, "--help"})

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("%s --help failed: %v", subcmd, err)
			}

			output := buf.String()
			if !strings.Contains(output, "Usage:") {
				t.Errorf("expected help output to contain 'Usage:', got: %s", output)
			}
		})
	}
}

func TestCommandStructure(t *testing.T) {
	tests := []struct {
		name        string
		cmdFactory  func() *cobra.Command
		expectedUse string
	}{
		{
			name:        "root command",
			cmdFactory:  newRootCmd,
			expectedUse: "kscore-audit",
		},
		{
			name:        "version command",
			cmdFactory:  newVersionCmd,
			expectedUse: "version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmdFactory()
			if cmd.Use != tt.expectedUse {
				t.Errorf("expected Use to be %s, got %s", tt.expectedUse, cmd.Use)
			}
		})
	}
}

func TestMultipleCommandCreations(t *testing.T) {
	for i := 0; i < 3; i++ {
		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"version"})

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("execution %d failed: %v", i, err)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 4, "h..."},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

func TestSearchCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	searchCmd := findSubcommand(cmd, "search")
	if searchCmd == nil {
		t.Fatal("search subcommand not found")
	}

	flags := []string{"policy", "user", "action", "since", "output", "denied", "limit"}
	for _, flag := range flags {
		if searchCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on search command", flag)
		}
	}
}

func TestSearchCommandQueryAlias(t *testing.T) {
	cmd := newRootCmd()
	searchCmd := findSubcommand(cmd, "search")
	if searchCmd == nil {
		t.Fatal("search subcommand not found")
	}

	found := false
	for _, alias := range searchCmd.Aliases {
		if alias == "query" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'query' alias on search command")
	}
}

func TestAnalyzeCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	analyzeCmd := findSubcommand(cmd, "analyze")
	if analyzeCmd == nil {
		t.Fatal("analyze subcommand not found")
	}

	flags := []string{"input", "baseline", "output"}
	for _, flag := range flags {
		if analyzeCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on analyze command", flag)
		}
	}
}

func TestTimelineCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	timelineCmd := findSubcommand(cmd, "timeline")
	if timelineCmd == nil {
		t.Fatal("timeline subcommand not found")
	}

	flags := []string{"from", "to", "output"}
	for _, flag := range flags {
		if timelineCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on timeline command", flag)
		}
	}
}

func TestWatchCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	watchCmd := findSubcommand(cmd, "watch")
	if watchCmd == nil {
		t.Fatal("watch subcommand not found")
	}

	flags := []string{"type", "status", "agent", "user", "api-key", "interval"}
	for _, flag := range flags {
		if watchCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on watch command", flag)
		}
	}

	intervalFlag := watchCmd.Flags().Lookup("interval")
	if intervalFlag.DefValue != "2s" {
		t.Errorf("expected interval default to be '2s', got %s", intervalFlag.DefValue)
	}
}

func TestWatchCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"watch", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("watch --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "real-time") {
		t.Errorf("expected help to contain 'real-time', got: %s", output)
	}
	if !strings.Contains(output, "--interval") {
		t.Errorf("expected help to contain '--interval', got: %s", output)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"500ms", 500 * time.Millisecond, false},
		{"bad", 0, true},
		{"xd", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
