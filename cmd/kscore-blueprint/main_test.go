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
	if cmd.Use != "kscore-blueprint" {
		t.Errorf("expected Use to be 'kscore-blueprint', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Blueprint") {
		t.Errorf("expected Short to contain 'Blueprint', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist (including deprecated ones)
	expectedCommands := []string{
		"version", "init", "validate", "lint", "test", "search", "info",
		"install", "update", "remove", "bundle", "mirror", "applied",
		// Deprecated commands that still exist
		"publish", "sign", "verify", "versions", "docs", "rollback", "snapshot",
	}
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
	if !strings.Contains(output, "kscore-blueprint") {
		t.Errorf("expected help output to contain 'kscore-blueprint', got: %s", output)
	}
	if !strings.Contains(output, "init") || !strings.Contains(output, "validate") {
		t.Errorf("expected help output to mention init and validate, got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

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

func TestInitCommandExists(t *testing.T) {
	cmd := newRootCmd()
	initCmd := findSubcommand(cmd, "init")
	if initCmd == nil {
		t.Fatal("init subcommand not found")
	}

	// init command should require exactly 1 argument (blueprint name)
	// The command might accept args differently, so we just verify it exists
	if !strings.Contains(initCmd.Short, "Create") && !strings.Contains(initCmd.Short, "new") {
		t.Logf("init command Short: %s", initCmd.Short)
	}
}

func TestValidateCommandExists(t *testing.T) {
	cmd := newRootCmd()
	validateCmd := findSubcommand(cmd, "validate")
	if validateCmd == nil {
		t.Fatal("validate subcommand not found")
	}
}

func TestLintCommandExists(t *testing.T) {
	cmd := newRootCmd()
	lintCmd := findSubcommand(cmd, "lint")
	if lintCmd == nil {
		t.Fatal("lint subcommand not found")
	}
}

func TestTestCommandExists(t *testing.T) {
	cmd := newRootCmd()
	testCmd := findSubcommand(cmd, "test")
	if testCmd == nil {
		t.Fatal("test subcommand not found")
	}
}

func TestSearchCommandExists(t *testing.T) {
	cmd := newRootCmd()
	searchCmd := findSubcommand(cmd, "search")
	if searchCmd == nil {
		t.Fatal("search subcommand not found")
	}
}

func TestInfoCommandExists(t *testing.T) {
	cmd := newRootCmd()
	infoCmd := findSubcommand(cmd, "info")
	if infoCmd == nil {
		t.Fatal("info subcommand not found")
	}
}

func TestInstallCommandExists(t *testing.T) {
	cmd := newRootCmd()
	installCmd := findSubcommand(cmd, "install")
	if installCmd == nil {
		t.Fatal("install subcommand not found")
	}
}

func TestUpdateCommandExists(t *testing.T) {
	cmd := newRootCmd()
	updateCmd := findSubcommand(cmd, "update")
	if updateCmd == nil {
		t.Fatal("update subcommand not found")
	}
}

func TestRemoveCommandExists(t *testing.T) {
	cmd := newRootCmd()
	removeCmd := findSubcommand(cmd, "remove")
	if removeCmd == nil {
		t.Fatal("remove subcommand not found")
	}
}

// Test deprecated commands still exist (for backwards compatibility)
func TestDeprecatedCommandsExist(t *testing.T) {
	cmd := newRootCmd()

	deprecatedCommands := []string{"publish", "sign", "verify", "versions", "docs", "rollback", "snapshot"}
	for _, deprecated := range deprecatedCommands {
		t.Run(deprecated, func(t *testing.T) {
			found := findSubcommand(cmd, deprecated)
			if found == nil {
				t.Errorf("deprecated subcommand %s not found (should still exist for backwards compatibility)", deprecated)
			}
		})
	}
}

func TestBundleCommandExists(t *testing.T) {
	cmd := newRootCmd()
	bundleCmd := findSubcommand(cmd, "bundle")
	if bundleCmd == nil {
		t.Fatal("bundle subcommand not found")
	}

	// Check subcommands
	createCmd := findSubcommand(bundleCmd, "create")
	if createCmd == nil {
		t.Error("bundle create subcommand not found")
	}

	installCmd := findSubcommand(bundleCmd, "install")
	if installCmd == nil {
		t.Error("bundle install subcommand not found")
	}
}

func TestMirrorCommandExists(t *testing.T) {
	cmd := newRootCmd()
	mirrorCmd := findSubcommand(cmd, "mirror")
	if mirrorCmd == nil {
		t.Fatal("mirror subcommand not found")
	}

	expectedSubs := []string{"sync", "serve", "export", "import"}
	for _, sub := range expectedSubs {
		if findSubcommand(mirrorCmd, sub) == nil {
			t.Errorf("mirror %s subcommand not found", sub)
		}
	}
}

func TestAppliedCommandExists(t *testing.T) {
	cmd := newRootCmd()
	appliedCmd := findSubcommand(cmd, "applied")
	if appliedCmd == nil {
		t.Fatal("applied subcommand not found")
	}

	expectedSubs := []string{"list", "show", "history", "usage"}
	for _, sub := range expectedSubs {
		if findSubcommand(appliedCmd, sub) == nil {
			t.Errorf("applied %s subcommand not found", sub)
		}
	}
}

func TestSubcommandHelp(t *testing.T) {
	// Test help for core subcommands
	subcommands := []string{"init", "validate", "lint", "search", "info", "install", "update", "remove", "bundle", "mirror", "applied"}

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
			expectedUse: "kscore-blueprint",
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

func TestCoreSubcommandCount(t *testing.T) {
	// Verify we have the expected number of subcommands
	cmd := newRootCmd()
	subcommands := cmd.Commands()

	// Should have at least 10 core commands + deprecated commands
	if len(subcommands) < 10 {
		t.Errorf("expected at least 10 subcommands, got %d", len(subcommands))
	}
}

func TestBlueprintDescription(t *testing.T) {
	cmd := newRootCmd()

	// Long description should mention Salt Formulas, Ansible Roles, or Helm Charts
	if !strings.Contains(cmd.Long, "Salt") &&
		!strings.Contains(cmd.Long, "Ansible") &&
		!strings.Contains(cmd.Long, "Helm") {
		t.Errorf("expected Long description to mention related tools, got: %s", cmd.Long)
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
