// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	expectedCommands := []string{"run", "validate", "verify", "version"}
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
	subcommands := []string{"run", "validate", "verify"}

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

func TestVerifyCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	verifyCmd := findSubcommand(cmd, "verify")
	if verifyCmd == nil {
		t.Fatal("verify subcommand not found")
	}

	flags := []struct {
		name     string
		defValue string
	}{
		{"source-system", ""},
		{"source-dir", ""},
		{"target-db", ""},
	}

	for _, f := range flags {
		flag := verifyCmd.Flags().Lookup(f.name)
		if flag == nil {
			t.Errorf("expected --%s flag on verify command", f.name)
			continue
		}
		if flag.DefValue != f.defValue {
			t.Errorf("expected --%s default to be %q, got %q", f.name, f.defValue, flag.DefValue)
		}
	}
}

func TestVerifyMissingSourceSystem(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"verify"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --source-system is missing")
	}
}

func TestVerifyInvalidSourceSystem(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "terraform"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported source system")
	}
}

func TestVerifyNoSourceDir(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "salt"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("verify without --source-dir should succeed with SKIPs, got error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SKIP") {
		t.Errorf("expected SKIP status in output when no source-dir, got: %s", output)
	}
	if !strings.Contains(output, "0 failed") {
		t.Errorf("expected 0 failed in output, got: %s", output)
	}
}

func TestVerifyWithPopulatedSourceDir(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "webserver.sls"), []byte("nginx:\n  pkg.installed"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "minions"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "minions", "web01"), []byte(""), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "pillar"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "pillar", "common.sls"), []byte("nginx:\n  workers: 4"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "reactor"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "reactor", "highstate.sls"), []byte(""), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "salt", "--source-dir", tmpDir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output, got: %s", output)
	}
	if !strings.Contains(output, "5 passed") {
		t.Errorf("expected all 5 checks to pass, got: %s", output)
	}
	if !strings.Contains(output, "0 failed") {
		t.Errorf("expected 0 failed, got: %s", output)
	}
}

func TestVerifyAllSupportedSystems(t *testing.T) {
	systems := []struct {
		name      string
		stateFile string
		stateExt  string
		agentDir  string
		varsDir   string
		eventDir  string
	}{
		{"salt", "init.sls", ".sls", "minions", "pillar", "reactor"},
		{"ansible", "main.yml", ".yml", "inventory", "group_vars", "callbacks"},
		{"puppet", "init.pp", ".pp", "nodes", "hieradata", "reports"},
		{"chef", "default.rb", ".rb", "nodes", "data_bags", "handlers"},
	}

	for _, sys := range systems {
		t.Run(sys.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			os.WriteFile(filepath.Join(tmpDir, sys.stateFile), []byte("test"), 0644)
			os.MkdirAll(filepath.Join(tmpDir, sys.agentDir), 0755)
			os.WriteFile(filepath.Join(tmpDir, sys.agentDir, "node1"), []byte(""), 0644)
			os.MkdirAll(filepath.Join(tmpDir, sys.varsDir), 0755)
			os.WriteFile(filepath.Join(tmpDir, sys.varsDir, "common.yaml"), []byte("key: val"), 0644)
			os.MkdirAll(filepath.Join(tmpDir, sys.eventDir), 0755)
			os.WriteFile(filepath.Join(tmpDir, sys.eventDir, "handler1"), []byte(""), 0644)

			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"verify", "--source-system", sys.name, "--source-dir", tmpDir})

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("verify for %s failed: %v", sys.name, err)
			}

			output := buf.String()
			if !strings.Contains(output, "5 passed") {
				t.Errorf("expected 5 passed for %s, got: %s", sys.name, output)
			}
		})
	}
}

func TestVerifyEmptySourceDir(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "salt", "--source-dir", tmpDir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty source directory")
	}

	output := buf.String()
	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected FAIL in output for empty dir, got: %s", output)
	}
}

func TestVerifyNonexistentSourceDir(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "salt", "--source-dir", "/nonexistent/path/xyz"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent source directory")
	}
}

func TestVerifyHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("verify --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "source-system") {
		t.Errorf("expected help to mention source-system, got: %s", output)
	}
	if !strings.Contains(output, "Salt") || !strings.Contains(output, "Ansible") {
		t.Errorf("expected help to mention supported systems, got: %s", output)
	}
}

func TestVerifyOutputFormat(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "ansible"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "CHECK") {
		t.Errorf("expected table header CHECK, got: %s", output)
	}
	if !strings.Contains(output, "STATUS") {
		t.Errorf("expected table header STATUS, got: %s", output)
	}
	if !strings.Contains(output, "DETAILS") {
		t.Errorf("expected table header DETAILS, got: %s", output)
	}
	if !strings.Contains(output, "Results:") {
		t.Errorf("expected Results summary, got: %s", output)
	}
}

func TestIsValidSourceSystem(t *testing.T) {
	valid := []string{"salt", "ansible", "puppet", "chef"}
	for _, s := range valid {
		if !isValidSourceSystem(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []string{"terraform", "cfengine", "", "Salt"}
	for _, s := range invalid {
		if isValidSourceSystem(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestStateFileExtensions(t *testing.T) {
	tests := []struct {
		system   string
		expected []string
	}{
		{"salt", []string{".sls"}},
		{"ansible", []string{".yml", ".yaml"}},
		{"puppet", []string{".pp"}},
		{"chef", []string{".rb"}},
		{"unknown", nil},
	}

	for _, tt := range tests {
		t.Run(tt.system, func(t *testing.T) {
			got := stateFileExtensions(tt.system)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d extensions, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("extension[%d]: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestVerifyWithTargetDB(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "init.sls"), []byte("test"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "minions"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "minions", "node1"), []byte(""), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "salt", "--source-dir", tmpDir, "--target-db", "/tmp/test.db"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Target database") {
		t.Errorf("expected target database info in output, got: %s", output)
	}
}

func TestRunVerifyChecks(t *testing.T) {
	results := runVerifyChecks("salt", "", "")
	if len(results) != 5 {
		t.Fatalf("expected 5 checks, got %d", len(results))
	}

	expectedNames := []string{
		"Source data readable",
		"State definitions migrated",
		"Agent/minion mappings complete",
		"Variables/pillar data preserved",
		"Event subscriptions migrated",
	}
	for i, name := range expectedNames {
		if results[i].Name != name {
			t.Errorf("check[%d]: expected name %q, got %q", i, name, results[i].Name)
		}
	}
}

func TestVerifyCaseInsensitiveSystem(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", "--source-system", "SALT"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected case-insensitive match for SALT, got error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "salt") {
		t.Errorf("expected output to reference salt, got: %s", output)
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
