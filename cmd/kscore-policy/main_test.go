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
	if cmd.Use != "kscore-policy" {
		t.Errorf("expected Use to be 'kscore-policy', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Policy enforcement") {
		t.Errorf("expected Short to contain 'Policy enforcement', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "list", "validate", "check", "show", "audit", "report"}
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
	if !strings.Contains(output, "kscore-policy") {
		t.Errorf("expected help output to contain 'kscore-policy', got: %s", output)
	}
	if !strings.Contains(output, "OPA") || !strings.Contains(output, "CEL") {
		t.Errorf("expected help output to mention OPA and CEL, got: %s", output)
	}
}

func TestListCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	listCmd := findSubcommand(cmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	// Check that list flags exist
	categoryFlag := listCmd.Flags().Lookup("category")
	if categoryFlag == nil {
		t.Error("expected --category flag on list command")
	}

	typeFlag := listCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Error("expected --type flag on list command")
	}

	outputFlag := listCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on list command")
	}
	if outputFlag.DefValue != "table" {
		t.Errorf("expected output default to be 'table', got %s", outputFlag.DefValue)
	}
}

func TestValidateCommandExists(t *testing.T) {
	cmd := newRootCmd()
	validateCmd := findSubcommand(cmd, "validate")
	if validateCmd == nil {
		t.Fatal("validate subcommand not found")
	}

	if !strings.Contains(validateCmd.Short, "Validate") {
		t.Errorf("expected Short to contain 'Validate', got %s", validateCmd.Short)
	}

	// Should require exactly 1 argument
	if validateCmd.Args == nil {
		t.Error("expected validate command to have Args validation")
	}
}

func TestCheckCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	checkCmd := findSubcommand(cmd, "check")
	if checkCmd == nil {
		t.Fatal("check subcommand not found")
	}

	// Check required flags
	policyFlag := checkCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on check command")
	}

	inputFileFlag := checkCmd.Flags().Lookup("input-file")
	if inputFileFlag == nil {
		t.Error("expected --input-file flag on check command")
	}

	inputFlag := checkCmd.Flags().Lookup("input")
	if inputFlag == nil {
		t.Error("expected --input flag on check command")
	}

	actionFlag := checkCmd.Flags().Lookup("action")
	if actionFlag == nil {
		t.Fatal("expected --action flag on check command")
	}
	if actionFlag.DefValue != "check" {
		t.Errorf("expected action default to be 'check', got %s", actionFlag.DefValue)
	}

	userFlag := checkCmd.Flags().Lookup("user")
	if userFlag == nil {
		t.Error("expected --user flag on check command")
	}

	contextFlag := checkCmd.Flags().Lookup("context")
	if contextFlag == nil {
		t.Error("expected --context flag on check command")
	}

	outputFlag := checkCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on check command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestShowCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	showCmd := findSubcommand(cmd, "show")
	if showCmd == nil {
		t.Fatal("show subcommand not found")
	}

	outputFlag := showCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on show command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}

	// Should require exactly 2 arguments
	if showCmd.Args == nil {
		t.Error("expected show command to have Args validation")
	}
}

func TestAuditCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	auditCmd := findSubcommand(cmd, "audit")
	if auditCmd == nil {
		t.Fatal("audit subcommand not found")
	}

	policyFlag := auditCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on audit command")
	}

	resourceTypeFlag := auditCmd.Flags().Lookup("resource-type")
	if resourceTypeFlag == nil {
		t.Error("expected --resource-type flag on audit command")
	}

	deniedFlag := auditCmd.Flags().Lookup("denied")
	if deniedFlag == nil {
		t.Error("expected --denied flag on audit command")
	}

	limitFlag := auditCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on audit command")
	}
	if limitFlag.DefValue != "100" {
		t.Errorf("expected limit default to be '100', got %s", limitFlag.DefValue)
	}

	outputFlag := auditCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on audit command")
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

	outputFlag := reportCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on report command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"list", "validate", "check", "show", "audit", "report"}

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
			expectedUse: "kscore-policy",
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
