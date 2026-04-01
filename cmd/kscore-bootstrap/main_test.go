// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
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
	rootCmd.AddCommand(joinCmd())
	rootCmd.AddCommand(prereqCheckCmd())
	rootCmd.AddCommand(certGenCmd())
	rootCmd.AddCommand(packageCmd())
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
	expectedCommands := []string{"version", "seed", "restore", "import", "validate", "status", "cleanup", "join", "prereq-check", "cert-gen", "package"}
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
		t.Fatal("expected --output-dir flag on seed command")
	}
	if outputDirFlag.DefValue != "/var/lib/keystone-core" {
		t.Errorf("expected output-dir default to be '/var/lib/keystone-core', got %s", outputDirFlag.DefValue)
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
		t.Fatal("expected --output flag on validate command")
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

func TestSlogLogger(t *testing.T) {
	// Test the slogLogger implementation
	logger := newSlogLogger(true)

	// These should not panic
	logger.Debug("debug message", "key", "value")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	// With verbose false (debug suppressed)
	logger = newSlogLogger(false)
	logger.Debug("this should not output") // Should be silent at info level
	logger.Info("info message")
}

func TestJoinCommandFlags(t *testing.T) {
	cmd := joinCmd()
	if cmd == nil {
		t.Fatal("join command is nil")
	}
	if cmd.Use != "join" {
		t.Errorf("expected Use to be 'join', got %s", cmd.Use)
	}

	flags := []string{"server", "token", "node", "advertise-address", "debug"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag on join command", flag)
		}
	}
}

func TestJoinRequiresServerAndToken(t *testing.T) {
	cmd := createRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"join"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --server and --token not provided")
	}
}

func TestJoinCommandHelp(t *testing.T) {
	cmd := createRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"join", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("join --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "server") {
		t.Errorf("expected help output to mention --server, got: %s", output)
	}
	if !strings.Contains(output, "token") {
		t.Errorf("expected help output to mention --token, got: %s", output)
	}
}

func TestPrereqCheckCommand(t *testing.T) {
	cmd := prereqCheckCmd()
	if cmd == nil {
		t.Fatal("prereq-check command is nil")
	}
	if cmd.Use != "prereq-check" {
		t.Errorf("expected Use to be 'prereq-check', got %s", cmd.Use)
	}

	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on prereq-check command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestPrereqCheckRuns(t *testing.T) {
	cmd := createRootCmd()
	cmd.SetArgs([]string{"prereq-check"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prereq-check failed: %v", err)
	}
}

func TestPrereqCheckJSON(t *testing.T) {
	cmd := createRootCmd()
	cmd.SetArgs([]string{"prereq-check", "--output", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prereq-check --output json failed: %v", err)
	}
}

func TestCertGenCommandFlags(t *testing.T) {
	cmd := certGenCmd()
	if cmd == nil {
		t.Fatal("cert-gen command is nil")
	}
	if cmd.Use != "cert-gen" {
		t.Errorf("expected Use to be 'cert-gen', got %s", cmd.Use)
	}

	flags := []string{"ca-cn", "server-cn", "output"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag on cert-gen command", flag)
		}
	}

	caCNFlag := cmd.Flags().Lookup("ca-cn")
	if caCNFlag.DefValue != "Keystone Core CA" {
		t.Errorf("expected ca-cn default to be 'Keystone Core CA', got %s", caCNFlag.DefValue)
	}
}

func TestCertGenGeneratesCerts(t *testing.T) {
	tmpDir := t.TempDir()

	err := runCertGen("Test CA", "test-server", tmpDir)
	if err != nil {
		t.Fatalf("runCertGen failed: %v", err)
	}

	expectedFiles := []string{"ca.pem", "ca-key.pem", "server.pem", "server-key.pem"}
	for _, f := range expectedFiles {
		path := tmpDir + "/" + f
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected file %s to not be empty", f)
		}
	}
}

func TestCertGenCommandHelp(t *testing.T) {
	cmd := createRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"cert-gen", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cert-gen --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "ca-cn") {
		t.Errorf("expected help output to mention --ca-cn, got: %s", output)
	}
}

func TestWritePEM(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/test.pem"

	err := writePEM(path, "CERTIFICATE", []byte("test-data"))
	if err != nil {
		t.Fatalf("writePEM failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestNewSubcommandHelp(t *testing.T) {
	subcmds := []string{"join", "prereq-check", "cert-gen"}
	for _, subcmd := range subcmds {
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

func TestPackageCommand(t *testing.T) {
	cmd := packageCmd()
	if cmd == nil {
		t.Fatal("package command is nil")
	}
	if cmd.Use != "package" {
		t.Errorf("expected Use to be 'package', got %s", cmd.Use)
	}

	expectedSubs := []string{"create", "verify", "install", "inspect"}
	for _, expected := range expectedSubs {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestPackageCreateFlags(t *testing.T) {
	cmd := packageCreateCmd()
	if cmd == nil {
		t.Fatal("package create command is nil")
	}

	expectedFlags := []string{
		"version", "platform", "build-dir", "signing-key",
		"include-modules", "include-blueprints", "include-docs",
		"modules-dir", "blueprints-dir", "docs-dir", "policies-dir",
		"output", "created-by",
	}
	for _, flag := range expectedFlags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag on package create command", flag)
		}
	}
}

func TestPackageVerifyFlags(t *testing.T) {
	cmd := packageVerifyCmd()
	if cmd == nil {
		t.Fatal("package verify command is nil")
	}

	if cmd.Flags().Lookup("trusted-key") == nil {
		t.Error("expected --trusted-key flag on package verify command")
	}
}

func TestPackageInstallFlags(t *testing.T) {
	cmd := packageInstallCmd()
	if cmd == nil {
		t.Fatal("package install command is nil")
	}

	expectedFlags := []string{
		"verify", "trusted-key", "target-dir", "config-dir",
		"data-dir", "unattended", "skip-modules",
	}
	for _, flag := range expectedFlags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag on package install command", flag)
		}
	}
}

func TestPackageSubcommandHelp(t *testing.T) {
	subcmds := []string{"package", "package create", "package verify", "package install", "package inspect"}
	for _, subcmd := range subcmds {
		t.Run(subcmd, func(t *testing.T) {
			cmd := createRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			args := append(strings.Fields(subcmd), "--help")
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("%s --help failed: %v", subcmd, err)
			}

			out := buf.String()
			if !strings.Contains(out, "Usage:") {
				t.Errorf("expected help output to contain 'Usage:', got: %s", out)
			}
		})
	}
}
