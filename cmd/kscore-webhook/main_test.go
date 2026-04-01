// Copyright 2024 Spicer Creek Solutions LLC
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

	if cmd.Use != "kscore-webhook" {
		t.Errorf("expected Use to be 'kscore-webhook', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Webhook") {
		t.Errorf("expected Short to contain 'Webhook', got %s", cmd.Short)
	}

	expectedCommands := []string{"version", "list", "show", "test", "history", "secrets", "outbound"}
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
	if !strings.Contains(output, "kscore-webhook") {
		t.Errorf("expected help output to contain 'kscore-webhook', got: %s", output)
	}
	if !strings.Contains(output, "ArgoCD") && !strings.Contains(output, "GitHub") {
		t.Errorf("expected help output to mention webhook sources, got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Fatal("expected --server flag")
	}
	if serverFlag.DefValue != "localhost:9090" {
		t.Errorf("expected server default to be 'localhost:9090', got %s", serverFlag.DefValue)
	}

	formatFlag := cmd.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag")
	}
	if formatFlag.DefValue != "table" {
		t.Errorf("expected format default to be 'table', got %s", formatFlag.DefValue)
	}

	auditLevelFlag := cmd.PersistentFlags().Lookup("audit-level")
	if auditLevelFlag == nil {
		t.Error("expected --audit-level flag")
	}

	auditOutputFlag := cmd.PersistentFlags().Lookup("audit-output")
	if auditOutputFlag == nil {
		t.Error("expected --audit-output flag")
	}
}

func TestListCommandExists(t *testing.T) {
	cmd := newRootCmd()
	listCmd := findSubcommand(cmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	if !strings.Contains(listCmd.Short, "List") {
		t.Errorf("expected Short to contain 'List', got %s", listCmd.Short)
	}
}

func TestShowCommandArgs(t *testing.T) {
	cmd := newRootCmd()
	showCmd := findSubcommand(cmd, "show")
	if showCmd == nil {
		t.Fatal("show subcommand not found")
	}

	if showCmd.Args == nil {
		t.Error("expected show command to have Args validation")
	}
}

func TestTestCommandArgs(t *testing.T) {
	cmd := newRootCmd()
	testCmd := findSubcommand(cmd, "test")
	if testCmd == nil {
		t.Fatal("test subcommand not found")
	}

	if testCmd.Args == nil {
		t.Error("expected test command to have Args validation")
	}
}

func TestHistoryCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	historyCmd := findSubcommand(cmd, "history")
	if historyCmd == nil {
		t.Fatal("history subcommand not found")
	}

	limitFlag := historyCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on history command")
	}
	if limitFlag.DefValue != "20" {
		t.Errorf("expected limit default to be '20', got %s", limitFlag.DefValue)
	}
}

func TestSecretsCommandSubcommands(t *testing.T) {
	cmd := newRootCmd()
	secretsCmd := findSubcommand(cmd, "secrets")
	if secretsCmd == nil {
		t.Fatal("secrets subcommand not found")
	}

	expectedSubcmds := []string{"list", "rotate"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range secretsCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected secrets subcommand %s not found", expected)
		}
	}
}

func TestSecretsRotateArgs(t *testing.T) {
	cmd := newRootCmd()
	secretsCmd := findSubcommand(cmd, "secrets")
	if secretsCmd == nil {
		t.Fatal("secrets subcommand not found")
	}

	rotateCmd := findSubcommand(secretsCmd, "rotate")
	if rotateCmd == nil {
		t.Fatal("secrets rotate subcommand not found")
	}

	if rotateCmd.Args == nil {
		t.Error("expected secrets rotate command to have Args validation")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"list", "show", "test", "history"}

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

func TestInboundCommandsNotYetAvailable(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		errText string
	}{
		{"list", []string{"list"}, "inbound webhook listing API not yet available"},
		{"show", []string{"show", "argocd"}, "inbound webhook details API not yet available"},
		{"history", []string{"history"}, "inbound webhook delivery history API not yet available"},
		{"secrets list", []string{"secrets", "list"}, "webhook secrets listing API not yet available"},
		{"secrets rotate", []string{"secrets", "rotate", "test-secret"}, "webhook secret rotation API not yet available"},
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

func TestCommandStructure(t *testing.T) {
	tests := []struct {
		name        string
		cmdFactory  func() *cobra.Command
		expectedUse string
	}{
		{
			name:        "root command",
			cmdFactory:  newRootCmd,
			expectedUse: "kscore-webhook",
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

func TestOutboundCommandSubcommands(t *testing.T) {
	cmd := newRootCmd()
	outboundCmd := findSubcommand(cmd, "outbound")
	if outboundCmd == nil {
		t.Fatal("outbound subcommand not found")
	}

	expectedSubcmds := []string{"list", "create", "show", "delete", "history", "test"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range outboundCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected outbound subcommand %s not found", expected)
		}
	}
}

func TestOutboundCreateFlags(t *testing.T) {
	cmd := newRootCmd()
	outboundCmd := findSubcommand(cmd, "outbound")
	if outboundCmd == nil {
		t.Fatal("outbound subcommand not found")
	}

	createCmd := findSubcommand(outboundCmd, "create")
	if createCmd == nil {
		t.Fatal("outbound create subcommand not found")
	}

	expectedFlags := []struct {
		name     string
		defValue string
	}{
		{"name", ""},
		{"url", ""},
		{"secret", ""},
		{"max-retries", "3"},
		{"timeout", "10"},
	}

	for _, ef := range expectedFlags {
		f := createCmd.Flags().Lookup(ef.name)
		if f == nil {
			t.Errorf("expected --%s flag on outbound create", ef.name)
			continue
		}
		if f.DefValue != ef.defValue {
			t.Errorf("expected --%s default %q, got %q", ef.name, ef.defValue, f.DefValue)
		}
	}
}

func TestOutboundShowArgs(t *testing.T) {
	cmd := newRootCmd()
	outboundCmd := findSubcommand(cmd, "outbound")
	if outboundCmd == nil {
		t.Fatal("outbound subcommand not found")
	}

	showCmd := findSubcommand(outboundCmd, "show")
	if showCmd == nil {
		t.Fatal("outbound show subcommand not found")
	}
	if showCmd.Args == nil {
		t.Error("expected outbound show command to have Args validation")
	}
}

func TestOutboundDeleteArgs(t *testing.T) {
	cmd := newRootCmd()
	outboundCmd := findSubcommand(cmd, "outbound")
	if outboundCmd == nil {
		t.Fatal("outbound subcommand not found")
	}

	deleteCmd := findSubcommand(outboundCmd, "delete")
	if deleteCmd == nil {
		t.Fatal("outbound delete subcommand not found")
	}
	if deleteCmd.Args == nil {
		t.Error("expected outbound delete command to have Args validation")
	}
}

func TestOutboundHistoryFlags(t *testing.T) {
	cmd := newRootCmd()
	outboundCmd := findSubcommand(cmd, "outbound")
	if outboundCmd == nil {
		t.Fatal("outbound subcommand not found")
	}

	historyCmd := findSubcommand(outboundCmd, "history")
	if historyCmd == nil {
		t.Fatal("outbound history subcommand not found")
	}

	limitFlag := historyCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on outbound history command")
	}
	if limitFlag.DefValue != "50" {
		t.Errorf("expected limit default '50', got %s", limitFlag.DefValue)
	}
}

func TestOutboundHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"outbound", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("outbound --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "outbound") {
		t.Errorf("expected help output to contain 'outbound', got: %s", out)
	}
	if !strings.Contains(out, "create") || !strings.Contains(out, "delete") {
		t.Errorf("expected help output to list subcommands, got: %s", out)
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
