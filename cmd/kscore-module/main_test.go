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
	if cmd.Use != "kscore-module" {
		t.Errorf("expected Use to be 'kscore-module', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Module management") {
		t.Errorf("expected Short to contain 'Module management', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "init", "validate", "build", "resolve", "tree", "verify", "sign", "test", "publish", "install"}
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
	if !strings.Contains(output, "kscore-module") {
		t.Errorf("expected help output to contain 'kscore-module', got: %s", output)
	}
	if !strings.Contains(output, "Module Lifecycle") {
		t.Errorf("expected help output to contain 'Module Lifecycle', got: %s", output)
	}
}

func TestInitCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	initCmd := findSubcommand(cmd, "init")
	if initCmd == nil {
		t.Fatal("init subcommand not found")
	}

	// Check that init flags exist
	typeFlag := initCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("expected --type flag on init command")
	}
	if typeFlag.DefValue != "starlark" {
		t.Errorf("expected type default to be 'starlark', got %s", typeFlag.DefValue)
	}

	authorFlag := initCmd.Flags().Lookup("author")
	if authorFlag == nil {
		t.Error("expected --author flag on init command")
	}

	descFlag := initCmd.Flags().Lookup("description")
	if descFlag == nil {
		t.Error("expected --description flag on init command")
	}

	outputFlag := initCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on init command")
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
}

func TestBuildCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	buildCmd := findSubcommand(cmd, "build")
	if buildCmd == nil {
		t.Fatal("build subcommand not found")
	}

	outputFlag := buildCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on build command")
	}
}

func TestResolveCommandExists(t *testing.T) {
	cmd := newRootCmd()
	resolveCmd := findSubcommand(cmd, "resolve")
	if resolveCmd == nil {
		t.Fatal("resolve subcommand not found")
	}

	if !strings.Contains(resolveCmd.Short, "Resolve") {
		t.Errorf("expected Short to contain 'Resolve', got %s", resolveCmd.Short)
	}
}

func TestTreeCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	treeCmd := findSubcommand(cmd, "tree")
	if treeCmd == nil {
		t.Fatal("tree subcommand not found")
	}

	flatFlag := treeCmd.Flags().Lookup("flat")
	if flatFlag == nil {
		t.Error("expected --flat flag on tree command")
	}
}

func TestVerifyCommandExists(t *testing.T) {
	cmd := newRootCmd()
	verifyCmd := findSubcommand(cmd, "verify")
	if verifyCmd == nil {
		t.Fatal("verify subcommand not found")
	}

	if !strings.Contains(verifyCmd.Short, "Verify") {
		t.Errorf("expected Short to contain 'Verify', got %s", verifyCmd.Short)
	}
}

func TestSignCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	signCmd := findSubcommand(cmd, "sign")
	if signCmd == nil {
		t.Fatal("sign subcommand not found")
	}

	keyFlag := signCmd.Flags().Lookup("key")
	if keyFlag == nil {
		t.Error("expected --key flag on sign command")
	}
}

func TestPublishCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	publishCmd := findSubcommand(cmd, "publish")
	if publishCmd == nil {
		t.Fatal("publish subcommand not found")
	}

	registryFlag := publishCmd.Flags().Lookup("registry")
	if registryFlag == nil {
		t.Error("expected --registry flag on publish command")
	}
}

func TestInstallCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	installCmd := findSubcommand(cmd, "install")
	if installCmd == nil {
		t.Fatal("install subcommand not found")
	}

	registryFlag := installCmd.Flags().Lookup("registry")
	if registryFlag == nil {
		t.Error("expected --registry flag on install command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"init", "validate", "build", "resolve", "tree", "verify", "sign", "test", "publish", "install"}

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
			expectedUse: "kscore-module",
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

func TestCoalesce(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected string
	}{
		{"first non-empty", []string{"a", "b"}, "a"},
		{"skip empty", []string{"", "b"}, "b"},
		{"all empty", []string{"", ""}, ""},
		{"single value", []string{"a"}, "a"},
		{"no values", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coalesce(tt.values...)
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
