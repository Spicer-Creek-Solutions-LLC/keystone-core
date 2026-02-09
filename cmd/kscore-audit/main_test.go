// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	// Check basic properties
	if cmd.Use != "kscore-audit" {
		t.Errorf("expected Use to be 'kscore-audit', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "audit") {
		t.Errorf("expected Short to contain 'audit', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "log", "report", "export", "stats"}
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

	// Check server flag
	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Fatal("expected --server flag")
	}
	if serverFlag.DefValue != "localhost:9090" {
		t.Errorf("expected server default to be 'localhost:9090', got %s", serverFlag.DefValue)
	}

	// Check format flag
	formatFlag := cmd.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag")
	}
	if formatFlag.DefValue != "table" {
		t.Errorf("expected format default to be 'table', got %s", formatFlag.DefValue)
	}

	// Check audit-level flag
	auditLevelFlag := cmd.PersistentFlags().Lookup("audit-level")
	if auditLevelFlag == nil {
		t.Error("expected --audit-level flag")
	}

	// Check audit-output flag
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

	// Check that log flags exist
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
	subcommands := []string{"log", "report", "export", "stats"}

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
	// Test that we can create multiple command instances
	// This tests for state isolation between instances
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
