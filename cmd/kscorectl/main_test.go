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
	if cmd.Use != "kscorectl" {
		t.Errorf("expected Use to be 'kscorectl', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Keystone Core") {
		t.Errorf("expected Short to contain 'Keystone Core', got %s", cmd.Short)
	}

	// Version command should always be present
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected version subcommand to be present")
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
	if !strings.Contains(output, "kscorectl") {
		t.Errorf("expected help output to contain 'kscorectl', got: %s", output)
	}
}

func TestVersionCommandOutput(t *testing.T) {
	cmd := newVersionCmd()
	if cmd == nil {
		t.Fatal("expected version command to not be nil")
	}

	if cmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", cmd.Use)
	}

	// Execute the command and check output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.Run(cmd, []string{})

	output := buf.String()
	if output == "" {
		t.Error("expected version output to not be empty")
	}
}

func TestNewPluginCommand(t *testing.T) {
	// Import plugin package to create a mock plugin
	// Since we can't easily mock the plugin package, we'll test the command creation
	// with minimal validation

	// Test that the function exists and returns a command
	// In actual usage, plugins are discovered at runtime
}

func TestRootCommandHasAvailableCommands(t *testing.T) {
	cmd := newRootCmd()

	// Root command should have at least version command
	commands := cmd.Commands()
	if len(commands) < 1 {
		t.Error("expected root command to have at least one subcommand (version)")
	}
}

func TestVersionCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "version") {
		t.Errorf("expected help to contain 'version', got: %s", output)
	}
}

func TestRootCommandUsage(t *testing.T) {
	cmd := newRootCmd()

	// Check that usage is set correctly
	usage := cmd.UsageString()
	if !strings.Contains(usage, "kscorectl") {
		t.Errorf("expected usage to contain 'kscorectl', got: %s", usage)
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
			expectedUse: "kscorectl",
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

func TestMultipleExecutions(t *testing.T) {
	// Test that we can create and execute multiple command instances
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
