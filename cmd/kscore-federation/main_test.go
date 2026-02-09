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
	if cmd.Use != "kscore-federation" {
		t.Errorf("expected Use to be 'kscore-federation', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "federation") {
		t.Errorf("expected Short to contain 'federation', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "list", "add", "show", "suspend", "activate", "remove", "refresh", "bundle"}
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
	if !strings.Contains(output, "kscore-federation") {
		t.Errorf("expected help output to contain 'kscore-federation', got: %s", output)
	}
	if !strings.Contains(output, "SPIFFE") {
		t.Errorf("expected help output to mention SPIFFE, got: %s", output)
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

	// Check output flag
	outputFlag := cmd.PersistentFlags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag")
	}
	if outputFlag.DefValue != "table" {
		t.Errorf("expected output default to be 'table', got %s", outputFlag.DefValue)
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
	found := findSubcommand(cmd, "list")
	if found == nil {
		t.Fatal("list subcommand not found")
	}

	if !strings.Contains(found.Short, "List") {
		t.Errorf("expected Short to contain 'List', got %s", found.Short)
	}
}

func TestAddCommandFlags(t *testing.T) {
	// Note: addCmd is a package-level var in this package

	// Check that add command requires exactly 1 argument
	if addCmd.Args == nil {
		t.Error("expected add command to have Args validation")
	}

	// Check flags
	bundleEndpointFlag := addCmd.Flags().Lookup("bundle-endpoint")
	if bundleEndpointFlag == nil {
		t.Error("expected --bundle-endpoint flag on add command")
	}

	typeFlag := addCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("expected --type flag on add command")
	}
	if typeFlag.DefValue != "bidirectional" {
		t.Errorf("expected type default to be 'bidirectional', got %s", typeFlag.DefValue)
	}

	refreshIntervalFlag := addCmd.Flags().Lookup("refresh-interval")
	if refreshIntervalFlag == nil {
		t.Error("expected --refresh-interval flag on add command")
	}
}

func TestShowCommandArgs(t *testing.T) {
	// showCmd is a package-level var
	if showCmd.Args == nil {
		t.Error("expected show command to have Args validation")
	}
}

func TestSuspendCommandArgs(t *testing.T) {
	// suspendCmd is a package-level var
	if suspendCmd.Args == nil {
		t.Error("expected suspend command to have Args validation")
	}
}

func TestActivateCommandArgs(t *testing.T) {
	// activateCmd is a package-level var
	if activateCmd.Args == nil {
		t.Error("expected activate command to have Args validation")
	}
}

func TestRemoveCommandFlags(t *testing.T) {
	// removeCmd is a package-level var
	if removeCmd.Args == nil {
		t.Error("expected remove command to have Args validation")
	}

	forceFlag := removeCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on remove command")
	}
}

func TestRefreshCommandArgs(t *testing.T) {
	// refreshCmd is a package-level var
	if refreshCmd.Args == nil {
		t.Error("expected refresh command to have Args validation")
	}
}

func TestBundleSubcommands(t *testing.T) {
	bundleCmd := newBundleCmd()
	if bundleCmd == nil {
		t.Fatal("bundle command is nil")
	}

	// Check nested subcommands
	expectedSubcmds := []string{"show", "export"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range bundleCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected bundle subcommand %s not found", expected)
		}
	}
}

func TestBundleExportFlags(t *testing.T) {
	// bundleExportCmd is a package-level var
	formatFlag := bundleExportCmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag on bundle export command")
	}
	if formatFlag.DefValue != "pem" {
		t.Errorf("expected format default to be 'pem', got %s", formatFlag.DefValue)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"list", "add", "show", "suspend", "activate", "remove", "refresh"}

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

func TestBundleSubcommandHelp(t *testing.T) {
	subcommands := []string{"bundle show", "bundle export"}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			args := strings.Split(subcmd, " ")
			args = append(args, "--help")
			cmd.SetArgs(args)

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
			expectedUse: "kscore-federation",
		},
		{
			name:        "version command",
			cmdFactory:  newVersionCmd,
			expectedUse: "version",
		},
		{
			name:        "bundle command",
			cmdFactory:  newBundleCmd,
			expectedUse: "bundle",
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

func TestPrintJSON(t *testing.T) {
	// Test with simple struct
	data := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 42,
	}

	// This should not return an error
	err := printJSON(data)
	if err != nil {
		t.Errorf("printJSON failed: %v", err)
	}
}

func TestFederationInfo(t *testing.T) {
	// Test FederationInfo struct
	info := FederationInfo{
		TrustDomain: "example.org",
		Type:        "bidirectional",
		State:       "active",
		CertCount:   2,
	}

	if info.TrustDomain != "example.org" {
		t.Errorf("expected TrustDomain to be 'example.org', got %s", info.TrustDomain)
	}
}

func TestFederationDetails(t *testing.T) {
	// Test FederationDetails struct
	details := FederationDetails{
		TrustDomain:    "example.org",
		Type:           "bidirectional",
		State:          "active",
		BundleEndpoint: "https://example.org/.well-known/spiffe-bundle",
		Policy: PolicyInfo{
			AllowedPaths: []string{"/service/**"},
			DeniedPaths:  []string{"/admin/**"},
			RequireMTLS:  true,
		},
	}

	if details.TrustDomain != "example.org" {
		t.Errorf("expected TrustDomain to be 'example.org', got %s", details.TrustDomain)
	}
	if !details.Policy.RequireMTLS {
		t.Error("expected RequireMTLS to be true")
	}
}

func TestBundleInfo(t *testing.T) {
	// Test BundleInfo struct
	info := BundleInfo{
		TrustDomain:    "kscore.local",
		SequenceNumber: 42,
		RefreshHint:    300,
	}

	if info.SequenceNumber != 42 {
		t.Errorf("expected SequenceNumber to be 42, got %d", info.SequenceNumber)
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
