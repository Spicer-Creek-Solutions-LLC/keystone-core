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

	// Check basic properties
	if cmd.Use != "kscore-exec" {
		t.Errorf("expected Use to be 'kscore-exec', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Remote execution") {
		t.Errorf("expected Short to contain 'Remote execution', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "run", "status", "list"}
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
	if !strings.Contains(output, "kscore-exec") {
		t.Errorf("expected help output to contain 'kscore-exec', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check that global flags exist
	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Error("expected --server flag to exist")
	}
	if serverFlag.DefValue != "localhost:50051" {
		t.Errorf("expected server default to be localhost:50051, got %s", serverFlag.DefValue)
	}

	timeoutFlag := cmd.PersistentFlags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Error("expected --timeout flag to exist")
	}

	auditLevelFlag := cmd.PersistentFlags().Lookup("audit-level")
	if auditLevelFlag == nil {
		t.Error("expected --audit-level flag to exist")
	}
	if auditLevelFlag.DefValue != "all" {
		t.Errorf("expected audit-level default to be 'all', got %s", auditLevelFlag.DefValue)
	}

	auditOutputFlag := cmd.PersistentFlags().Lookup("audit-output")
	if auditOutputFlag == nil {
		t.Error("expected --audit-output flag to exist")
	}
}

func TestRunCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	runCmd := findSubcommand(cmd, "run")
	if runCmd == nil {
		t.Fatal("run subcommand not found")
	}

	// Check that run flags exist
	expectedFlags := []struct {
		name     string
		defValue string
	}{
		{"concurrency", "10"},
		{"continue-on-failure", "true"},
		{"working-dir", ""},
		{"user", ""},
		{"command-timeout", "300"},
		{"job-id", ""},
		{"show-progress", "true"},
		{"show-results", "true"},
	}

	for _, ef := range expectedFlags {
		flag := runCmd.Flags().Lookup(ef.name)
		if flag == nil {
			t.Errorf("expected --%s flag on run command", ef.name)
			continue
		}
		if flag.DefValue != ef.defValue {
			t.Errorf("expected --%s default to be %q, got %q", ef.name, ef.defValue, flag.DefValue)
		}
	}
}

func TestStatusCommand(t *testing.T) {
	cmd := newRootCmd()
	statusCmd := findSubcommand(cmd, "status")
	if statusCmd == nil {
		t.Fatal("status subcommand not found")
	}

	if statusCmd.Use != "status <job-id>" {
		t.Errorf("expected Use to be 'status <job-id>', got %s", statusCmd.Use)
	}

	if !strings.Contains(statusCmd.Short, "status") {
		t.Errorf("expected Short to contain 'status', got %s", statusCmd.Short)
	}
}

func TestListCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	listCmd := findSubcommand(cmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	// Check that list flags exist
	statusFlag := listCmd.Flags().Lookup("status")
	if statusFlag == nil {
		t.Error("expected --status flag on list command")
	}

	pageSizeFlag := listCmd.Flags().Lookup("page-size")
	if pageSizeFlag == nil {
		t.Error("expected --page-size flag on list command")
	}
	if pageSizeFlag.DefValue != "20" {
		t.Errorf("expected page-size default to be 20, got %s", pageSizeFlag.DefValue)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"run", "status", "list"}

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

func TestConfigType(t *testing.T) {
	cfg := &Config{
		ServerAddr:  "localhost:50051",
		Timeout:     5 * time.Minute,
		AuditLevel:  "all",
		AuditOutput: "auto",
	}

	if cfg.ServerAddr != "localhost:50051" {
		t.Errorf("expected ServerAddr to be localhost:50051, got %s", cfg.ServerAddr)
	}

	if cfg.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout to be 5m, got %v", cfg.Timeout)
	}
}

func TestRunOptionsType(t *testing.T) {
	opts := &RunOptions{
		Concurrency:      10,
		ContinueOnError:  true,
		WorkingDir:       "/tmp",
		User:             "root",
		CommandTimeout:   300,
		Env:              []string{"FOO=bar"},
		JobID:            "test-job",
		ShowProgress:     true,
		ShowAgentResults: true,
	}

	if opts.Concurrency != 10 {
		t.Errorf("expected Concurrency to be 10, got %d", opts.Concurrency)
	}

	if opts.CommandTimeout != 300 {
		t.Errorf("expected CommandTimeout to be 300, got %d", opts.CommandTimeout)
	}
}

func TestListOptionsType(t *testing.T) {
	opts := &ListOptions{
		Status:   "completed",
		PageSize: 50,
	}

	if opts.Status != "completed" {
		t.Errorf("expected Status to be completed, got %s", opts.Status)
	}

	if opts.PageSize != 50 {
		t.Errorf("expected PageSize to be 50, got %d", opts.PageSize)
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name     string
		format   func(string) string
		input    string
		expected string
	}{
		{"truncate short", func(s string) string { return truncate(s, 10) }, "short", "short"},
		{"truncate long", func(s string) string { return truncate(s, 10) }, "this is a long string", "this is..."},
		{"truncate exact", func(s string) string { return truncate(s, 10) }, "exactlyten", "exactlyten"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.format(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
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
			expectedUse: "kscore-exec",
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

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
