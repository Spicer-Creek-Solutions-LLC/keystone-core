// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// createRootCmd creates a testable root command (mirrors the structure in main())
func createRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-bootstrap",
		Short: "Bootstrap a Keystone Core cluster",
		Long: `kscore-bootstrap initializes and bootstraps a new Keystone Core cluster
from a seed configuration, restores from backup, or imports an existing installation.`,
	}

	rootCmd.AddCommand(seedCmd())
	rootCmd.AddCommand(restoreCmd())
	rootCmd.AddCommand(importCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cleanupCmd())
	rootCmd.AddCommand(versionCmd())

	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	return rootCmd
}

func TestRootCommand(t *testing.T) {
	cmd := createRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	// Check basic properties
	if cmd.Use != "kscore-bootstrap" {
		t.Errorf("expected Use to be 'kscore-bootstrap', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Bootstrap") {
		t.Errorf("expected Short to contain 'Bootstrap', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "seed", "restore", "import", "validate", "status", "cleanup"}
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
	cmd := versionCmd()
	if cmd == nil {
		t.Fatal("version command is nil")
	}

	if cmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "version") {
		t.Errorf("expected Short to contain 'version', got %s", cmd.Short)
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := createRootCmd()
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
	if !strings.Contains(output, "kscore-bootstrap") {
		t.Errorf("expected help output to contain 'kscore-bootstrap', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := createRootCmd()

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

func TestSeedCommandFlags(t *testing.T) {
	cmd := seedCmd()
	if cmd == nil {
		t.Fatal("seed command is nil")
	}

	// Check flags
	configFlag := cmd.Flags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag on seed command")
	}

	outputDirFlag := cmd.Flags().Lookup("output-dir")
	if outputDirFlag == nil {
		t.Error("expected --output-dir flag on seed command")
	}
	if outputDirFlag.DefValue != "/var/lib/kscore" {
		t.Errorf("expected output-dir default to be '/var/lib/kscore', got %s", outputDirFlag.DefValue)
	}

	verboseFlag := cmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("expected --verbose flag on seed command")
	}

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on seed command")
	}

	skipVerificationFlag := cmd.Flags().Lookup("skip-verification")
	if skipVerificationFlag == nil {
		t.Error("expected --skip-verification flag on seed command")
	}

	timeoutFlag := cmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Error("expected --timeout flag on seed command")
	}
}

func TestRestoreCommandFlags(t *testing.T) {
	cmd := restoreCmd()
	if cmd == nil {
		t.Fatal("restore command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected restore command to have Args validation")
	}

	// Check flags
	decryptionKeyFlag := cmd.Flags().Lookup("decryption-key")
	if decryptionKeyFlag == nil {
		t.Error("expected --decryption-key flag on restore command")
	}

	outputDirFlag := cmd.Flags().Lookup("output-dir")
	if outputDirFlag == nil {
		t.Error("expected --output-dir flag on restore command")
	}
}

func TestImportCommandFlags(t *testing.T) {
	cmd := importCmd()
	if cmd == nil {
		t.Fatal("import command is nil")
	}

	// Check flags
	configFlag := cmd.Flags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag on import command")
	}
}

func TestValidateCommandArgs(t *testing.T) {
	cmd := validateCmd()
	if cmd == nil {
		t.Fatal("validate command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected validate command to have Args validation")
	}

	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on validate command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestStatusCommandFlags(t *testing.T) {
	cmd := statusCmd()
	if cmd == nil {
		t.Fatal("status command is nil")
	}

	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on status command")
	}
}

func TestCleanupCommandFlags(t *testing.T) {
	cmd := cleanupCmd()
	if cmd == nil {
		t.Fatal("cleanup command is nil")
	}

	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on cleanup command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"seed", "restore", "import", "validate", "status", "cleanup"}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := createRootCmd()
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
			name:        "seed command",
			cmdFactory:  seedCmd,
			expectedUse: "seed [config-file]",
		},
		{
			name:        "restore command",
			cmdFactory:  restoreCmd,
			expectedUse: "restore <backup-file>",
		},
		{
			name:        "import command",
			cmdFactory:  importCmd,
			expectedUse: "import",
		},
		{
			name:        "validate command",
			cmdFactory:  validateCmd,
			expectedUse: "validate <config-file>",
		},
		{
			name:        "status command",
			cmdFactory:  statusCmd,
			expectedUse: "status",
		},
		{
			name:        "cleanup command",
			cmdFactory:  cleanupCmd,
			expectedUse: "cleanup",
		},
		{
			name:        "version command",
			cmdFactory:  versionCmd,
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
		cmd := createRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"version"})

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("execution %d failed: %v", i, err)
		}
	}
}

func TestBuildKeyValueTable(t *testing.T) {
	pairs := [][2]string{
		{"KEY1", "VALUE1"},
		{"KEY2", "VALUE2"},
		{"KEY3", ""}, // Empty value should be skipped
	}

	table := buildKeyValueTable(pairs)
	if table == nil {
		t.Fatal("expected table to not be nil")
	}
	if len(table.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(table.Headers))
	}
	// Empty values are skipped
	if len(table.Rows) != 2 {
		t.Errorf("expected 2 rows (empty value skipped), got %d", len(table.Rows))
	}
}

func TestCliLogger(t *testing.T) {
	// Test the cliLogger implementation
	logger := &cliLogger{verbose: true}

	// These should not panic
	logger.Debug("debug message", "key", "value")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	// With verbose false
	logger = &cliLogger{verbose: false}
	logger.Debug("this should not output") // Should be silent
	logger.Info("info message")
}
