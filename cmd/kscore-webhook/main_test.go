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
	if cmd.Use != "kscore-webhook" {
		t.Errorf("expected Use to be 'kscore-webhook', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Webhook") {
		t.Errorf("expected Short to contain 'Webhook', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "list", "show", "test", "history", "secrets"}
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

	// Check server flag
	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Fatal("expected --server flag")
	}
	if serverFlag.DefValue != "localhost:9090" {
		t.Errorf("expected server default to be 'localhost:9090', got %s", serverFlag.DefValue)
	}

	// Check format flag
	formatFlag := cmd.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag")
	}
	if formatFlag.DefValue != "table" {
		t.Errorf("expected format default to be 'table', got %s", formatFlag.DefValue)
	}

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

	// Should require exactly 1 argument
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

	// Should require exactly 1 argument
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

	// Check nested subcommands
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

	// Should require exactly 1 argument
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

func TestListCommandExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("list command failed: %v", err)
	}

	output := buf.String()
	// Should show webhook handlers
	if !strings.Contains(output, "argocd") || !strings.Contains(output, "github") {
		t.Errorf("expected list output to show webhook handlers, got: %s", output)
	}
}

func TestListCommandJSONOutput(t *testing.T) {
	// Reset the global outputFormat for this test
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--format", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("list --format json command failed: %v", err)
	}

	output := buf.String()
	// Should be valid JSON array
	if !strings.HasPrefix(strings.TrimSpace(output), "[") {
		t.Errorf("expected JSON array output, got: %s", output)
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

func TestWebhookHandlerTypes(t *testing.T) {
	validTypes := []string{"argocd", "flux", "github", "gitlab"}

	for _, webhookType := range validTypes {
		t.Run(webhookType, func(t *testing.T) {
			handler, err := getHandlerDetails(webhookType)
			if err != nil {
				t.Errorf("getHandlerDetails(%s) failed: %v", webhookType, err)
			}
			if handler == nil {
				t.Fatalf("expected handler for type %s", webhookType)
			}
			if handler.Type != webhookType {
				t.Errorf("expected handler type %s, got %s", webhookType, handler.Type)
			}
		})
	}
}

func TestWebhookHandlerInvalidType(t *testing.T) {
	_, err := getHandlerDetails("invalid")
	if err == nil {
		t.Error("expected error for invalid webhook type")
	}
}

func TestBuildHandlersTable(t *testing.T) {
	handlers := []WebhookHandler{
		{
			Type:        "test",
			Description: "Test handler",
			Events:      []string{"event1", "event2"},
			Endpoint:    "/webhooks/test",
			Enabled:     true,
		},
	}

	table := buildHandlersTable(handlers)
	if table == nil {
		t.Fatal("expected table to not be nil")
	}
	if len(table.Headers) != 5 {
		t.Errorf("expected 5 headers, got %d", len(table.Headers))
	}
	if len(table.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(table.Rows))
	}
}

func TestBuildDeliveryTable(t *testing.T) {
	deliveries := []WebhookDelivery{
		{
			ID:           "test-id",
			Type:         "github",
			Event:        "push",
			Status:       "success",
			StatusCode:   200,
			ResponseTime: "50ms",
		},
	}

	table := buildDeliveryTable(deliveries)
	if table == nil {
		t.Fatal("expected table to not be nil")
	}
	if len(table.Headers) != 6 {
		t.Errorf("expected 6 headers, got %d", len(table.Headers))
	}
	if len(table.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(table.Rows))
	}
}

func TestBuildSecretsTable(t *testing.T) {
	secrets := []WebhookSecret{
		{
			Name: "test-secret",
			Type: "github",
		},
	}

	table := buildSecretsTable(secrets)
	if table == nil {
		t.Fatal("expected table to not be nil")
	}
	if len(table.Headers) != 4 {
		t.Errorf("expected 4 headers, got %d", len(table.Headers))
	}
	if len(table.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(table.Rows))
	}
}

func TestGenerateSamplePayload(t *testing.T) {
	types := []string{"argocd", "flux", "github", "gitlab"}

	for _, webhookType := range types {
		t.Run(webhookType, func(t *testing.T) {
			payload := generateSamplePayload(webhookType)
			if payload == nil {
				t.Errorf("expected non-nil payload for type %s", webhookType)
			}
			if len(payload) == 0 {
				t.Errorf("expected non-empty payload for type %s", webhookType)
			}
		})
	}
}

func TestGenerateSamplePayloadInvalidType(t *testing.T) {
	payload := generateSamplePayload("invalid")
	if payload == nil {
		t.Error("expected empty map, not nil")
	}
	if len(payload) != 0 {
		t.Errorf("expected empty map for invalid type, got %d keys", len(payload))
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
