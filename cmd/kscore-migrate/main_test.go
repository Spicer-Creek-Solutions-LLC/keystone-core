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
	if cmd.Use != "kscore-migrate" {
		t.Errorf("expected Use to be 'kscore-migrate', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "migration") {
		t.Errorf("expected Short to contain 'migration', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"run", "validate", "version"}
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
	if !strings.Contains(output, "kscore-migrate") {
		t.Errorf("expected help output to contain 'kscore-migrate', got: %s", output)
	}
	if !strings.Contains(output, "SQLite") || !strings.Contains(output, "PostgreSQL") {
		t.Errorf("expected help output to mention SQLite and PostgreSQL, got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check verbose flag
	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("expected --verbose flag")
	}
}

func TestRunCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	runCmd := findSubcommand(cmd, "run")
	if runCmd == nil {
		t.Fatal("run subcommand not found")
	}

	// Check required flags
	sqliteFlag := runCmd.Flags().Lookup("sqlite")
	if sqliteFlag == nil {
		t.Error("expected --sqlite flag on run command")
	}

	postgresFlag := runCmd.Flags().Lookup("postgres")
	if postgresFlag == nil {
		t.Error("expected --postgres flag on run command")
	}

	// Check optional flags
	dryRunFlag := runCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on run command")
	}

	batchSizeFlag := runCmd.Flags().Lookup("batch-size")
	if batchSizeFlag == nil {
		t.Fatal("expected --batch-size flag on run command")
	}
	if batchSizeFlag.DefValue != "100" {
		t.Errorf("expected batch-size default to be '100', got %s", batchSizeFlag.DefValue)
	}

	continueOnErrFlag := runCmd.Flags().Lookup("continue-on-error")
	if continueOnErrFlag == nil {
		t.Error("expected --continue-on-error flag on run command")
	}

	skipExistingFlag := runCmd.Flags().Lookup("skip-existing")
	if skipExistingFlag == nil {
		t.Fatal("expected --skip-existing flag on run command")
	}
	if skipExistingFlag.DefValue != "true" {
		t.Errorf("expected skip-existing default to be 'true', got %s", skipExistingFlag.DefValue)
	}
}

func TestValidateCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	validateCmd := findSubcommand(cmd, "validate")
	if validateCmd == nil {
		t.Fatal("validate subcommand not found")
	}

	// Check required flags
	sqliteFlag := validateCmd.Flags().Lookup("sqlite")
	if sqliteFlag == nil {
		t.Error("expected --sqlite flag on validate command")
	}

	postgresFlag := validateCmd.Flags().Lookup("postgres")
	if postgresFlag == nil {
		t.Error("expected --postgres flag on validate command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"run", "validate"}

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
			expectedUse: "kscore-migrate",
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
