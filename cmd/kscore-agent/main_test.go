// Copyright 2026 Spicer Creek Solutions LLC
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
	if cmd.Use != "kscore-agent" {
		t.Errorf("expected Use to be 'kscore-agent', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "agent") {
		t.Errorf("expected Short to contain 'agent', got %s", cmd.Short)
	}

	if !strings.Contains(cmd.Long, "managed nodes") {
		t.Errorf("expected Long to contain 'managed nodes', got %s", cmd.Long)
	}

	if !strings.Contains(cmd.Long, "embedded NATS") {
		t.Errorf("expected Long to contain 'embedded NATS', got %s", cmd.Long)
	}

	// Check that version subcommand exists
	found := false
	bootstrapFound := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			found = true
		}
		if sub.Name() == "bootstrap" {
			bootstrapFound = true
		}
	}
	if !found {
		t.Error("expected version subcommand not found")
	}
	if !bootstrapFound {
		t.Error("expected bootstrap subcommand not found")
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
	if !strings.Contains(output, "kscore-agent") {
		t.Errorf("expected help output to contain 'kscore-agent', got: %s", output)
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
		t.Fatal("expected --config flag")
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
			expectedUse: "kscore-agent",
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

func TestLongDescriptionContainsKeyFeatures(t *testing.T) {
	cmd := newRootCmd()

	// Agent description should mention key features
	features := []string{
		"managed nodes",
		"executes commands",
		"control plane",
		"embedded NATS",
		"edge deployments",
	}

	for _, feature := range features {
		if !strings.Contains(cmd.Long, feature) {
			t.Errorf("expected Long description to contain '%s'", feature)
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
