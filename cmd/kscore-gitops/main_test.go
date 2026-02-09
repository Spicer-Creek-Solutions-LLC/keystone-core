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
	if cmd.Use != "kscore-gitops" {
		t.Errorf("expected Use to be 'kscore-gitops', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "GitOps") {
		t.Errorf("expected Short to contain 'GitOps', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "verify", "rollback", "promote", "webhook", "status", "git-sync"}
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
	if !strings.Contains(output, "kscore-gitops") {
		t.Errorf("expected help output to contain 'kscore-gitops', got: %s", output)
	}
	if !strings.Contains(output, "ArgoCD") || !strings.Contains(output, "Flux") {
		t.Errorf("expected help output to mention ArgoCD and Flux, got: %s", output)
	}
}

func TestVerifyCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	verifyCmd := findSubcommand(cmd, "verify")
	if verifyCmd == nil {
		t.Fatal("verify subcommand not found")
	}

	// Check that verify flags exist
	parallelFlag := verifyCmd.Flags().Lookup("parallel")
	if parallelFlag == nil {
		t.Error("expected --parallel flag on verify command")
	}

	timeoutFlag := verifyCmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Fatal("expected --timeout flag on verify command")
	}
	if timeoutFlag.DefValue != "2m" {
		t.Errorf("expected timeout default to be '2m', got %s", timeoutFlag.DefValue)
	}

	outputFlag := verifyCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on verify command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestRollbackCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	rollbackCmd := findSubcommand(cmd, "rollback")
	if rollbackCmd == nil {
		t.Fatal("rollback subcommand not found")
	}

	// Check required and optional flags
	appFlag := rollbackCmd.Flags().Lookup("app")
	if appFlag == nil {
		t.Error("expected --app flag on rollback command")
	}

	namespaceFlag := rollbackCmd.Flags().Lookup("namespace")
	if namespaceFlag == nil {
		t.Fatal("expected --namespace flag on rollback command")
	}
	if namespaceFlag.DefValue != "default" {
		t.Errorf("expected namespace default to be 'default', got %s", namespaceFlag.DefValue)
	}

	typeFlag := rollbackCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("expected --type flag on rollback command")
	}
	if typeFlag.DefValue != "argocd" {
		t.Errorf("expected type default to be 'argocd', got %s", typeFlag.DefValue)
	}

	strategyFlag := rollbackCmd.Flags().Lookup("strategy")
	if strategyFlag == nil {
		t.Fatal("expected --strategy flag on rollback command")
	}
	if strategyFlag.DefValue != "previous" {
		t.Errorf("expected strategy default to be 'previous', got %s", strategyFlag.DefValue)
	}

	revisionFlag := rollbackCmd.Flags().Lookup("revision")
	if revisionFlag == nil {
		t.Error("expected --revision flag on rollback command")
	}

	reasonFlag := rollbackCmd.Flags().Lookup("reason")
	if reasonFlag == nil {
		t.Error("expected --reason flag on rollback command")
	}

	userFlag := rollbackCmd.Flags().Lookup("user")
	if userFlag == nil {
		t.Error("expected --user flag on rollback command")
	}

	dryRunFlag := rollbackCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on rollback command")
	}
}

func TestPromoteCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	promoteCmd := findSubcommand(cmd, "promote")
	if promoteCmd == nil {
		t.Fatal("promote subcommand not found")
	}

	// Check required and optional flags
	pipelineFlag := promoteCmd.Flags().Lookup("pipeline")
	if pipelineFlag == nil {
		t.Error("expected --pipeline flag on promote command")
	}

	fromFlag := promoteCmd.Flags().Lookup("from")
	if fromFlag == nil {
		t.Error("expected --from flag on promote command")
	}

	toFlag := promoteCmd.Flags().Lookup("to")
	if toFlag == nil {
		t.Error("expected --to flag on promote command")
	}

	revisionFlag := promoteCmd.Flags().Lookup("revision")
	if revisionFlag == nil {
		t.Error("expected --revision flag on promote command")
	}

	skipVerifyFlag := promoteCmd.Flags().Lookup("skip-verify")
	if skipVerifyFlag == nil {
		t.Error("expected --skip-verify flag on promote command")
	}

	forceFlag := promoteCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on promote command")
	}

	dryRunFlag := promoteCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on promote command")
	}
}

func TestWebhookSubcommands(t *testing.T) {
	cmd := newRootCmd()
	webhookCmd := findSubcommand(cmd, "webhook")
	if webhookCmd == nil {
		t.Fatal("webhook subcommand not found")
	}

	// Check webhook subcommands exist
	expectedSubcommands := []string{"list", "test"}
	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range webhookCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected webhook subcommand %s not found", expected)
		}
	}
}

func TestStatusCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	statusCmd := findSubcommand(cmd, "status")
	if statusCmd == nil {
		t.Fatal("status subcommand not found")
	}

	typeFlag := statusCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("expected --type flag on status command")
	}
	if typeFlag.DefValue != "all" {
		t.Errorf("expected type default to be 'all', got %s", typeFlag.DefValue)
	}

	limitFlag := statusCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on status command")
	}
	if limitFlag.DefValue != "10" {
		t.Errorf("expected limit default to be '10', got %s", limitFlag.DefValue)
	}

	outputFlag := statusCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on status command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"verify", "rollback", "promote", "webhook", "status"}

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
			expectedUse: "kscore-gitops",
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

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 4, "h..."},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGitSyncSubcommands(t *testing.T) {
	cmd := newRootCmd()
	gitSyncCmd := findSubcommand(cmd, "git-sync")
	if gitSyncCmd == nil {
		t.Fatal("git-sync subcommand not found")
	}

	expected := []string{"status", "trigger", "force", "conflicts", "lock", "unlock", "locks", "history", "audit"}
	for _, name := range expected {
		found := findSubcommand(gitSyncCmd, name)
		if found == nil {
			t.Errorf("expected git-sync subcommand %s not found", name)
		}
	}
}

func TestGitSyncConflictsSubcommands(t *testing.T) {
	cmd := newRootCmd()
	gitSyncCmd := findSubcommand(cmd, "git-sync")
	if gitSyncCmd == nil {
		t.Fatal("git-sync subcommand not found")
	}
	conflictsCmd := findSubcommand(gitSyncCmd, "conflicts")
	if conflictsCmd == nil {
		t.Fatal("conflicts subcommand not found")
	}

	expected := []string{"list", "show", "diff", "resolve", "resolve-all"}
	for _, name := range expected {
		found := findSubcommand(conflictsCmd, name)
		if found == nil {
			t.Errorf("expected conflicts subcommand %s not found", name)
		}
	}
}

func TestGitSyncStatusCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "status", "myrepo"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync status failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "myrepo") {
		t.Errorf("expected output to contain 'myrepo', got: %s", out)
	}
	if !strings.Contains(out, "Status:") {
		t.Errorf("expected output to contain 'Status:', got: %s", out)
	}
}

func TestGitSyncStatusJSON(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "status", "myrepo", "--output", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync status --output json failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"repository"`) {
		t.Errorf("expected JSON output with 'repository' field, got: %s", out)
	}
}

func TestGitSyncTriggerCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "trigger", "myrepo"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync trigger failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "myrepo") {
		t.Errorf("expected output to contain 'myrepo', got: %s", out)
	}
	if !strings.Contains(out, "Sync triggered") {
		t.Errorf("expected output to contain 'Sync triggered', got: %s", out)
	}
}

func TestGitSyncForceCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "force", "--repository", "myrepo"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync force failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Force sync") {
		t.Errorf("expected output to contain 'Force sync', got: %s", out)
	}
}

func TestGitSyncForceRequiresRepo(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"git-sync", "force"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --repository not specified")
	}
}

func TestGitSyncConflictsListCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync conflicts list failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Sync Conflicts") {
		t.Errorf("expected output to contain 'Sync Conflicts', got: %s", out)
	}
	if !strings.Contains(out, "nginx.yaml") {
		t.Errorf("expected output to contain 'nginx.yaml', got: %s", out)
	}
}

func TestGitSyncConflictsListFilter(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "list", "--status", "resolved"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync conflicts list --status failed: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "nginx.yaml") {
		t.Errorf("expected no conflicts with status=resolved, got: %s", out)
	}
}

func TestGitSyncConflictsShowCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "show", "states/nginx.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync conflicts show failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "states/nginx.yaml") {
		t.Errorf("expected output to contain file path, got: %s", out)
	}
}

func TestGitSyncConflictsDiffCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "diff", "states/nginx.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync conflicts diff failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "---") {
		t.Errorf("expected diff output with ---, got: %s", out)
	}
	if !strings.Contains(out, "+++") {
		t.Errorf("expected diff output with +++, got: %s", out)
	}
}

func TestGitSyncConflictsResolveAcceptGit(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve", "states/nginx.yaml", "--accept-git"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync conflicts resolve failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "accept-git") {
		t.Errorf("expected output to contain 'accept-git', got: %s", out)
	}
	if !strings.Contains(out, "Conflict resolved") {
		t.Errorf("expected output to contain 'Conflict resolved', got: %s", out)
	}
}

func TestGitSyncConflictsResolveKeepRuntime(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve", "f.yaml", "--keep-runtime", "--reason", "override"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("resolve --keep-runtime failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "keep-runtime") {
		t.Errorf("expected 'keep-runtime' in output, got: %s", out)
	}
	if !strings.Contains(out, "override") {
		t.Errorf("expected reason in output, got: %s", out)
	}
}

func TestGitSyncConflictsResolveNoStrategy(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve", "f.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no strategy specified")
	}
}

func TestGitSyncConflictsResolveMultipleStrategies(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve", "f.yaml", "--accept-git", "--keep-runtime"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when multiple strategies specified")
	}
}

func TestGitSyncConflictsResolveAllAcceptGit(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve-all", "--accept-git"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("resolve-all --accept-git failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "accept-git") {
		t.Errorf("expected 'accept-git' in output, got: %s", out)
	}
	if !strings.Contains(out, "2 conflicts resolved") {
		t.Errorf("expected '2 conflicts resolved' in output, got: %s", out)
	}
}

func TestGitSyncConflictsResolveAllNoStrategy(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"git-sync", "conflicts", "resolve-all"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no strategy specified")
	}
}

func TestGitSyncLockCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "lock", "states/nginx.yaml", "--reason", "maintenance"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync lock failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "states/nginx.yaml") {
		t.Errorf("expected path in output, got: %s", out)
	}
	if !strings.Contains(out, "locked") {
		t.Errorf("expected 'locked' in output, got: %s", out)
	}
}

func TestGitSyncUnlockCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "unlock", "states/nginx.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync unlock failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "unlocked") {
		t.Errorf("expected 'unlocked' in output, got: %s", out)
	}
}

func TestGitSyncLocksCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "locks"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync locks failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Active Locks") {
		t.Errorf("expected 'Active Locks' in output, got: %s", out)
	}
}

func TestGitSyncHistoryCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "history"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync history failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Sync History") {
		t.Errorf("expected 'Sync History' in output, got: %s", out)
	}
}

func TestGitSyncHistoryConflictsOnly(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "history", "--conflicts-only"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync history --conflicts-only failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "conflict") {
		t.Errorf("expected conflict entries in output, got: %s", out)
	}
	if strings.Contains(out, "sync-003") {
		t.Errorf("expected non-conflict entry sync-003 to be filtered out, got: %s", out)
	}
}

func TestGitSyncHistoryLimit(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "history", "--limit", "1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync history --limit failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total: 1 entries") {
		t.Errorf("expected 'Total: 1 entries' in output, got: %s", out)
	}
}

func TestGitSyncAuditCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "audit"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync audit failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Sync Audit Log") {
		t.Errorf("expected 'Sync Audit Log' in output, got: %s", out)
	}
}

func TestGitSyncAuditJSON(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"git-sync", "audit", "--format", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("git-sync audit --format json failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"action"`) {
		t.Errorf("expected JSON output with 'action' field, got: %s", out)
	}
}

func TestGitSyncHelp(t *testing.T) {
	subcommands := []string{
		"git-sync",
		"git-sync conflicts",
	}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			args := append(strings.Split(subcmd, " "), "--help")
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

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
