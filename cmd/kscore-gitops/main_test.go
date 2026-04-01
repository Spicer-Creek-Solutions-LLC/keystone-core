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

func TestGitSyncCommandsNotYetAvailable(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		errText string
	}{
		{"status", []string{"git-sync", "status", "myrepo"}, "sync status API not yet available"},
		{"trigger", []string{"git-sync", "trigger", "myrepo"}, "sync trigger API not yet available"},
		{"force", []string{"git-sync", "force"}, "force sync API not yet available"},
		{"conflicts list", []string{"git-sync", "conflicts", "list"}, "conflict management API not yet available"},
		{"conflicts show", []string{"git-sync", "conflicts", "show", "f.yaml"}, "conflict management API not yet available"},
		{"conflicts diff", []string{"git-sync", "conflicts", "diff", "f.yaml"}, "conflict management API not yet available"},
		{"conflicts resolve", []string{"git-sync", "conflicts", "resolve", "f.yaml"}, "conflict management API not yet available"},
		{"conflicts resolve-all", []string{"git-sync", "conflicts", "resolve-all"}, "conflict management API not yet available"},
		{"lock", []string{"git-sync", "lock", "f.yaml"}, "file locking API not yet available"},
		{"unlock", []string{"git-sync", "unlock", "f.yaml"}, "file locking API not yet available"},
		{"locks", []string{"git-sync", "locks"}, "file locking API not yet available"},
		{"history", []string{"git-sync", "history"}, "sync history API not yet available"},
		{"audit", []string{"git-sync", "audit"}, "sync audit API not yet available"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for not-yet-available command")
			}
			if !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("expected error containing %q, got: %v", tt.errText, err)
			}
		})
	}
}

func TestStatusNotYetAvailable(t *testing.T) {
	err := statusExecute(nil, nil)
	if err == nil {
		t.Fatal("expected error for not-yet-available status command")
	}
	if !strings.Contains(err.Error(), "status API not yet available") {
		t.Errorf("expected 'not yet available' error, got: %v", err)
	}
}

func TestRepoCommandsNotYetAvailable(t *testing.T) {
	funcs := map[string]func(*cobra.Command, []string) error{
		"list":   repoListExecute,
		"add":    repoAddExecute,
		"remove": repoRemoveExecute,
		"sync":   repoSyncExecute,
	}

	for name, fn := range funcs {
		t.Run(name, func(t *testing.T) {
			err := fn(nil, nil)
			if err == nil {
				t.Fatal("expected error for not-yet-available repo command")
			}
			if !strings.Contains(err.Error(), "not yet available") {
				t.Errorf("expected 'not yet available' error, got: %v", err)
			}
		})
	}
}

func TestDeployCommandsNotYetAvailable(t *testing.T) {
	funcs := map[string]func(*cobra.Command, []string) error{
		"list":     deployListExecute,
		"show":     deployShowExecute,
		"rollback": deployRollbackExecute,
		"approve":  deployApproveExecute,
	}

	for name, fn := range funcs {
		t.Run(name, func(t *testing.T) {
			err := fn(nil, nil)
			if err == nil {
				t.Fatal("expected error for not-yet-available deploy command")
			}
			if !strings.Contains(err.Error(), "not yet available") {
				t.Errorf("expected 'not yet available' error, got: %v", err)
			}
		})
	}
}

func TestPromoteNotYetAvailable(t *testing.T) {
	saved := promoteOutput
	promoteOutput = "json"
	promoteDryRun = false
	defer func() {
		promoteOutput = saved
		promoteDryRun = false
	}()

	dummyCmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	dummyCmd.SetOut(buf)

	err := promoteExecute(dummyCmd, nil)
	if err == nil {
		t.Fatal("expected error for not-yet-available promote command")
	}
	if !strings.Contains(err.Error(), "promotion API not yet available") {
		t.Errorf("expected 'not yet available' error, got: %v", err)
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

func TestServerFlag(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.PersistentFlags().Lookup("server")
	if flag == nil {
		t.Fatal("expected --server persistent flag on root command")
	}
	if flag.DefValue != "http://localhost:8080" {
		t.Errorf("expected default server to be 'http://localhost:8080', got %s", flag.DefValue)
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
