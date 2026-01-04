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
	if cmd.Use != "kscore-server" {
		t.Errorf("expected Use to be 'kscore-server', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "control plane") {
		t.Errorf("expected Short to contain 'control plane', got %s", cmd.Short)
	}

	if !strings.Contains(cmd.Long, "remote execution") {
		t.Errorf("expected Long to contain 'remote execution', got %s", cmd.Long)
	}

	// Check that version subcommand exists
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected version subcommand not found")
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
	if !strings.Contains(output, "kscore-server") {
		t.Errorf("expected help output to contain 'kscore-server', got: %s", output)
	}
	if !strings.Contains(output, "control plane") {
		t.Errorf("expected help output to mention 'control plane', got: %s", output)
	}
}

func TestConfigFlag(t *testing.T) {
	cmd := newRootCmd()

	// Check config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag")
	}
	if configFlag.DefValue != "" {
		t.Errorf("expected config default to be empty, got %s", configFlag.DefValue)
	}
}

func TestVersionCommandExists(t *testing.T) {
	cmd := newRootCmd()
	versionCmd := findSubcommand(cmd, "version")
	if versionCmd == nil {
		t.Fatal("version subcommand not found")
	}

	if versionCmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", versionCmd.Use)
	}

	if !strings.Contains(versionCmd.Short, "version") {
		t.Errorf("expected Short to contain 'version', got %s", versionCmd.Short)
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
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
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
			expectedUse: "kscore-server",
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

func TestSilenceSettings(t *testing.T) {
	cmd := newRootCmd()

	if !cmd.SilenceUsage {
		t.Error("expected SilenceUsage to be true")
	}

	if !cmd.SilenceErrors {
		t.Error("expected SilenceErrors to be true")
	}
}

func TestRootHasRunFunction(t *testing.T) {
	cmd := newRootCmd()

	if cmd.Run == nil {
		t.Error("expected root command to have a Run function")
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
