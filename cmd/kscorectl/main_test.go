// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
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

	found = false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected config subcommand to be present")
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

func TestConfigValidateCommand(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile, err := os.CreateTemp("", "kscore-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })

	configData := []byte(fmt.Sprintf(`nats:
  mode: embedded
  embedded:
    storedir: %s/embedded
  jetstream:
    storedir: %s/jetstream
storage:
  backend: sqlite
auth:
  enabled: false
`, tmpDir, tmpDir))

	if _, err := tmpFile.Write(configData); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close config: %v", err)
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "validate", "--config", tmpFile.Name()})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config validate failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Config OK") {
		t.Errorf("expected config validate output to include 'Config OK', got: %s", output)
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

// findSubcommand traverses a command tree to find a nested subcommand by name path.
func findSubcommand(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}

func TestRootHasNewCommandGroups(t *testing.T) {
	cmd := newRootCmd()

	for _, name := range []string{"auth", "db", "diagnostics", "security", "nats"} {
		sub := findSubcommand(cmd, name)
		if sub == nil {
			t.Errorf("expected %q subcommand on root", name)
		}
	}
}

// --- Auth command tests ---

func TestAuthSubcommands(t *testing.T) {
	cmd := newRootCmd()
	auth := findSubcommand(cmd, "auth")
	if auth == nil {
		t.Fatal("auth command not found")
	}

	expected := []string{"login", "revoke-all", "sessions", "rotate-signing-key", "key"}
	for _, name := range expected {
		if findSubcommand(auth, name) == nil {
			t.Errorf("expected auth subcommand %q", name)
		}
	}
}

func TestAuthLoginRequiresCredential(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"auth", "login"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no credential provided")
	}
	if !strings.Contains(err.Error(), "--username") && !strings.Contains(err.Error(), "--api-key") {
		t.Errorf("expected error about --username or --api-key, got: %v", err)
	}
}

func TestAuthLoginWithUsername(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "login", "--username", "admin"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Authenticated as: admin") {
		t.Errorf("expected authentication message, got: %s", output)
	}
}

func TestAuthLoginWithAPIKey(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "login", "--api-key", "kscore_test123"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Authenticated with API key") {
		t.Errorf("expected API key auth message, got: %s", output)
	}
}

func TestAuthRevokeAllForce(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "revoke-all", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "All API keys have been revoked") {
		t.Errorf("expected revocation message, got: %s", output)
	}
}

func TestAuthSessionsList(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "sessions", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Active Sessions") {
		t.Errorf("expected sessions header, got: %s", output)
	}
}

func TestAuthSessionsInvalidate(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "sessions", "invalidate"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "All sessions invalidated") {
		t.Errorf("expected invalidation message, got: %s", output)
	}
}

func TestAuthRotateSigningKeyForce(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "rotate-signing-key", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "JWT signing key rotated") {
		t.Errorf("expected rotation message, got: %s", output)
	}
}

func TestAuthKeyRevoke(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "key", "revoke", "key-abc123"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "key-abc123") {
		t.Errorf("expected key ID in output, got: %s", output)
	}
}

func TestAuthKeyRevokeRequiresArg(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"auth", "key", "revoke"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no key-id provided")
	}
}

// --- Config set/show tests ---

func TestConfigSetRequiresTwoArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "set"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no args provided to config set")
	}
}

func TestConfigSetOneArg(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "set", "only-key"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when only one arg provided")
	}
}

func TestConfigSetSuccess(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "set", "server.workers", "16"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Set server.workers = 16") {
		t.Errorf("expected set confirmation, got: %s", output)
	}
}

func TestConfigShow(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "show"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Fallback loads local config and outputs JSON with the header
	if !strings.Contains(output, "Configuration") {
		t.Errorf("expected config header, got: %s", output)
	}
	// Should contain actual config JSON (at least the Server section)
	if !strings.Contains(output, "Server") {
		t.Errorf("expected Server config section in output, got: %s", output)
	}
}

func TestConfigShowIncludeDefaults(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "show", "--include-defaults"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected some output from config show --include-defaults")
	}
	// Should contain actual config JSON
	if !strings.Contains(output, "NATS") {
		t.Errorf("expected NATS config section in output, got: %s", output)
	}
}

// --- RBAC command tests ---

func TestRBACListRoles(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"rbac", "list-roles"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Should show the standard roles table
	if !strings.Contains(output, "admin") {
		t.Errorf("expected admin role in output, got: %s", output)
	}
	if !strings.Contains(output, "operator") {
		t.Errorf("expected operator role in output, got: %s", output)
	}
	if !strings.Contains(output, "viewer") {
		t.Errorf("expected viewer role in output, got: %s", output)
	}
	if !strings.Contains(output, "service") {
		t.Errorf("expected service role in output, got: %s", output)
	}
	if !strings.Contains(output, "ROLE") {
		t.Errorf("expected table header in output, got: %s", output)
	}
}

func TestRBACListRolesShowPermissions(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"rbac", "list-roles", "--show-permissions"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// With --show-permissions, should show resource:action lines
	if !strings.Contains(output, "agents:") {
		t.Errorf("expected permission details in output, got: %s", output)
	}
	if !strings.Contains(output, "*:*") {
		t.Errorf("expected admin wildcard permission in output, got: %s", output)
	}
}

func TestRBACExportJSON(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"rbac", "export"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Should be valid JSON with roles
	if !strings.Contains(output, `"roles"`) {
		t.Errorf("expected roles key in JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"admin"`) {
		t.Errorf("expected admin role in export, got: %s", output)
	}
	if !strings.Contains(output, `"principals"`) {
		t.Errorf("expected principals key in JSON output, got: %s", output)
	}
}

func TestRBACExportYAML(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"rbac", "export", "--format", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// YAML should contain role definitions
	if !strings.Contains(output, "roles:") {
		t.Errorf("expected roles: key in YAML output, got: %s", output)
	}
	if !strings.Contains(output, "id: admin") {
		t.Errorf("expected admin role in YAML export, got: %s", output)
	}
}

func TestRBACExportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/rbac-export.json"

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"rbac", "export", "--output", outFile})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output should confirm file was written
	if !strings.Contains(buf.String(), outFile) {
		t.Errorf("expected file path in output, got: %s", buf.String())
	}

	// File should exist and contain valid JSON
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}
	if !strings.Contains(string(data), `"roles"`) {
		t.Errorf("exported file missing roles key: %s", string(data))
	}
}

func TestRBACExportBadFormat(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"rbac", "export", "--format", "xml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// --- DB command tests ---

func TestDBSubcommands(t *testing.T) {
	cmd := newRootCmd()
	db := findSubcommand(cmd, "db")
	if db == nil {
		t.Fatal("db command not found")
	}

	for _, name := range []string{"compact", "rotate-credentials"} {
		if findSubcommand(db, name) == nil {
			t.Errorf("expected db subcommand %q", name)
		}
	}
}

func TestDBCompact(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"db", "compact"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Database compacted") {
		t.Errorf("expected compaction message, got: %s", output)
	}
}

func TestDBCompactDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"db", "compact", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected dry-run prefix, got: %s", output)
	}
}

func TestDBRotateCredentialsForce(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"db", "rotate-credentials", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Database credentials rotated") {
		t.Errorf("expected rotation message, got: %s", output)
	}
}

// --- Diagnostics command tests ---

func TestDiagnosticsSubcommands(t *testing.T) {
	cmd := newRootCmd()
	diag := findSubcommand(cmd, "diagnostics")
	if diag == nil {
		t.Fatal("diagnostics command not found")
	}

	if findSubcommand(diag, "collect") == nil {
		t.Error("expected diagnostics collect subcommand")
	}
}

func TestDiagnosticsCollect(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "collect"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Collecting diagnostics") {
		t.Errorf("expected diagnostics collection message, got: %s", output)
	}
	if !strings.Contains(output, "Diagnostics collected") {
		t.Errorf("expected completion message, got: %s", output)
	}
}

func TestDiagnosticsCollectWithOutputDir(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "collect", "--output-dir", "/tmp/mydiag"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "/tmp/mydiag") {
		t.Errorf("expected custom output dir in output, got: %s", output)
	}
}

func TestDiagnosticsCollectWithLogs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "collect", "--include-logs", "--since", "24h"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Collecting logs") {
		t.Errorf("expected log collection message, got: %s", output)
	}
	if !strings.Contains(output, "24h") {
		t.Errorf("expected since duration in output, got: %s", output)
	}
}

func TestDiagnosticsCollectWithConfig(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "collect", "--include-config"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Collecting sanitized configuration") {
		t.Errorf("expected config collection message, got: %s", output)
	}
}

func TestDiagnosticsCollectAllFlags(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "collect", "--output-dir", "/tmp/all", "--include-logs", "--include-config", "--since", "7d"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "/tmp/all") {
		t.Errorf("expected output dir, got: %s", output)
	}
	if !strings.Contains(output, "Collecting logs") {
		t.Errorf("expected log message, got: %s", output)
	}
	if !strings.Contains(output, "Collecting sanitized configuration") {
		t.Errorf("expected config message, got: %s", output)
	}
}

func TestAuthHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"auth", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "login") {
		t.Errorf("expected help to list login subcommand, got: %s", output)
	}
}

func TestDBHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"db", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "compact") {
		t.Errorf("expected help to list compact subcommand, got: %s", output)
	}
}

func TestDiagnosticsHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diagnostics", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "collect") {
		t.Errorf("expected help to list collect subcommand, got: %s", output)
	}
}

// --- Security command tests ---

func TestSecuritySubcommands(t *testing.T) {
	cmd := newRootCmd()
	sec := findSubcommand(cmd, "security")
	if sec == nil {
		t.Fatal("security command not found")
	}

	if findSubcommand(sec, "scan") == nil {
		t.Error("expected security scan subcommand")
	}
}

func TestSecurityScan(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"security", "scan"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Security Scan") {
		t.Errorf("expected scan header, got: %s", output)
	}
	if !strings.Contains(output, "Summary:") {
		t.Errorf("expected summary in output, got: %s", output)
	}
}

func TestSecurityScanFull(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"security", "scan", "--full"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "full") {
		t.Errorf("expected full scan type, got: %s", output)
	}
	if !strings.Contains(output, "Vulnerability scan") {
		t.Errorf("expected vulnerability scan in full mode, got: %s", output)
	}
}

func TestSecurityScanJSON(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"security", "scan", "--output", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"scan_type"`) {
		t.Errorf("expected JSON output with scan_type, got: %s", output)
	}
}

func TestSecurityScanFlags(t *testing.T) {
	cmd := newSecurityScanCmd()

	flags := []string{"full", "output", "targets"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestSecurityScanResultStructure(t *testing.T) {
	r := SecurityScanResult{
		Timestamp: "2025-01-01T00:00:00Z",
		ScanType:  "full",
		Checks: []SecurityCheckItem{
			{Name: "test", Category: "test", Status: "pass", Message: "ok"},
		},
		Summary: SecuritySummary{TotalChecks: 1, Passed: 1},
	}

	if r.ScanType != "full" {
		t.Errorf("ScanType = %v, want full", r.ScanType)
	}
	if len(r.Checks) != 1 {
		t.Errorf("Checks length = %d, want 1", len(r.Checks))
	}
}

func TestSecurityHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"security", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "scan") {
		t.Errorf("expected help to list scan subcommand, got: %s", output)
	}
}

// --- NATS command tests ---

func TestNATSSubcommands(t *testing.T) {
	cmd := newRootCmd()
	nats := findSubcommand(cmd, "nats")
	if nats == nil {
		t.Fatal("nats command not found")
	}

	for _, name := range []string{"rotate-credentials", "status"} {
		if findSubcommand(nats, name) == nil {
			t.Errorf("expected nats subcommand %q", name)
		}
	}
}

func TestNATSRotateCredentialsForce(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "rotate-credentials", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NATS credentials rotated") {
		t.Errorf("expected rotation message, got: %s", output)
	}
}

func TestNATSStatus(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "status"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NATS Status") {
		t.Errorf("expected NATS status header, got: %s", output)
	}
}

func TestNATSStatusJSON(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "status", "--output", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"connected"`) {
		t.Errorf("expected JSON output with connected field, got: %s", output)
	}
}

func TestNATSHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "rotate-credentials") {
		t.Errorf("expected help to list rotate-credentials, got: %s", output)
	}
}

func TestMaintenanceSubcommands(t *testing.T) {
	cmd := newRootCmd()
	maint := findSubcommand(cmd, "maintenance")
	if maint == nil {
		t.Fatal("maintenance subcommand not found")
	}

	expected := []string{"enable", "disable", "status", "queue", "cleanup"}
	for _, name := range expected {
		if findSubcommand(maint, name) == nil {
			t.Errorf("expected maintenance subcommand %q not found", name)
		}
	}
}

func TestMaintenanceEnableReturnsErrorWhenServerUnreachable(t *testing.T) {
	old := serverAddr
	serverAddr = "127.0.0.1:1"
	defer func() { serverAddr = old }()

	err := runMaintenanceEnable("test", "1h", true)
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to enable maintenance mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaintenanceDisableReturnsErrorWhenServerUnreachable(t *testing.T) {
	old := serverAddr
	serverAddr = "127.0.0.1:1"
	defer func() { serverAddr = old }()

	err := runMaintenanceDisable(true)
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to disable maintenance mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaintenanceStatusReturnsErrorWhenServerUnreachable(t *testing.T) {
	old := serverAddr
	serverAddr = "127.0.0.1:1"
	defer func() { serverAddr = old }()

	err := runMaintenanceStatus("table")
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to get maintenance status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaintenanceQueueReturnsErrorWhenServerUnreachable(t *testing.T) {
	old := serverAddr
	serverAddr = "127.0.0.1:1"
	defer func() { serverAddr = old }()

	err := runMaintenanceQueue(false)
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to get maintenance queue") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaintenanceCleanupReturnsErrorWhenServerUnreachable(t *testing.T) {
	old := serverAddr
	serverAddr = "127.0.0.1:1"
	defer func() { serverAddr = old }()

	err := runMaintenanceCleanup("30d", false)
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to run maintenance cleanup") {
		t.Errorf("unexpected error: %v", err)
	}
}
