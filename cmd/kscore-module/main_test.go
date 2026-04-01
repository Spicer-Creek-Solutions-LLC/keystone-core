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
	if cmd.Use != "kscore-module" {
		t.Errorf("expected Use to be 'kscore-module', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Module management") {
		t.Errorf("expected Short to contain 'Module management', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "init", "validate", "build", "resolve", "tree", "verify", "sign", "test", "publish", "install", "update", "mirror", "clean"}
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

	registryFlag := resolveCmd.Flags().Lookup("registry")
	if registryFlag == nil {
		t.Error("expected --registry flag on resolve command")
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
	subcommands := []string{"init", "validate", "build", "resolve", "tree", "verify", "sign", "test", "publish", "install", "update", "mirror", "clean"}

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

func TestMirrorCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	mirrorCmd := findSubcommand(cmd, "mirror")
	if mirrorCmd == nil {
		t.Fatal("mirror subcommand not found")
	}

	flags := []string{"source", "dest", "import", "registry", "dry-run", "verify"}
	for _, flag := range flags {
		if mirrorCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on mirror command", flag)
		}
	}
}

func TestMirrorExportRequiresDest(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"mirror", "vendor/pkg@1.0.0", "--source", "https://example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --dest is missing")
	}
}

func TestMirrorExportDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"mirror", "vendor/pkg@1.0.0", "--source", "https://example.com", "--dest", t.TempDir(), "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("mirror dry-run failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Dry run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestMirrorImportRequiresRegistry(t *testing.T) {
	dir := t.TempDir()
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"mirror", "--import", dir})

	// Unset env var to ensure flag is required
	t.Setenv("KSCORE_REGISTRY", "")

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --registry is missing for import")
	}
}

func TestMirrorImportNonExistentDir(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"mirror", "--import", "/nonexistent/path", "--registry", "localhost:5000"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent import directory")
	}
}

func TestCleanCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	cleanCmd := findSubcommand(cmd, "clean")
	if cleanCmd == nil {
		t.Fatal("clean subcommand not found")
	}

	flags := []string{"all", "dry-run"}
	for _, flag := range flags {
		if cleanCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on clean command", flag)
		}
	}
}

func TestCleanNonExistentCache(t *testing.T) {
	t.Setenv("KSCORE_CACHE_DIR", filepath.Join(t.TempDir(), "nonexistent"))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"clean"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Nothing to clean") {
		t.Errorf("expected 'Nothing to clean' message, got: %s", output)
	}
}

func TestCleanEmptyCache(t *testing.T) {
	t.Setenv("KSCORE_CACHE_DIR", t.TempDir())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"clean"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "empty") {
		t.Errorf("expected empty cache message, got: %s", output)
	}
}

func TestCleanDryRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.zip"), []byte("test"), 0o644)
	t.Setenv("KSCORE_CACHE_DIR", dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"clean", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("clean dry-run failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Dry run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestCleanAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.zip"), []byte("test"), 0o644)
	t.Setenv("KSCORE_CACHE_DIR", dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"clean", "--all"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("clean --all failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Removed all cached modules") {
		t.Errorf("expected removal message, got: %s", output)
	}
}

func TestUpdateCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	updateCmd := findSubcommand(cmd, "update")
	if updateCmd == nil {
		t.Fatal("update subcommand not found")
	}

	dryRunFlag := updateCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on update command")
	}
}

func TestUpdateHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"update", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("update --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Update") && !strings.Contains(output, "update") {
		t.Errorf("expected help output to contain 'Update' or 'update', got: %s", output)
	}
}

func TestDirStats(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world!"), 0o644)

	size, count := dirStats(dir)
	if count != 2 {
		t.Errorf("expected 2 files, got %d", count)
	}
	if size != 11 {
		t.Errorf("expected 11 bytes, got %d", size)
	}
}

// findSubcommand finds a subcommand by name
func TestTestCoverageNotYetImplemented(t *testing.T) {
	oldCoverage := testCoverage
	testCoverage = true
	defer func() { testCoverage = oldCoverage }()

	err := testExecute(nil, []string{"."})
	if err == nil {
		t.Fatal("expected error when --coverage is used")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected 'not yet implemented' error, got: %v", err)
	}
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
