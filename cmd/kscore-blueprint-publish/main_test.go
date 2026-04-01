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

	// Check basic properties
	if cmd.Use != "kscore-blueprint-publish" {
		t.Errorf("expected Use to be 'kscore-blueprint-publish', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Blueprint") && !strings.Contains(cmd.Short, "publication") {
		t.Errorf("expected Short to contain 'Blueprint' or 'publication', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "publish", "sign", "verify", "versions", "docs"}
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
	if !strings.Contains(output, "kscore-blueprint-publish") {
		t.Errorf("expected help output to contain 'kscore-blueprint-publish', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

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

func TestPublishCommandExists(t *testing.T) {
	cmd := newRootCmd()
	publishCmd := findSubcommand(cmd, "publish")
	if publishCmd == nil {
		t.Fatal("publish subcommand not found")
	}
}

func TestSignCommandExists(t *testing.T) {
	cmd := newRootCmd()
	signCmd := findSubcommand(cmd, "sign")
	if signCmd == nil {
		t.Fatal("sign subcommand not found")
	}
}

func TestVerifyCommandExists(t *testing.T) {
	cmd := newRootCmd()
	verifyCmd := findSubcommand(cmd, "verify")
	if verifyCmd == nil {
		t.Fatal("verify subcommand not found")
	}
}

func TestVersionsCommandExists(t *testing.T) {
	cmd := newRootCmd()
	versionsCmd := findSubcommand(cmd, "versions")
	if versionsCmd == nil {
		t.Fatal("versions subcommand not found")
	}
}

func TestDocsCommandExists(t *testing.T) {
	cmd := newRootCmd()
	docsCmd := findSubcommand(cmd, "docs")
	if docsCmd == nil {
		t.Fatal("docs subcommand not found")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"publish", "sign", "verify", "versions", "docs"}

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
			expectedUse: "kscore-blueprint-publish",
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

func TestDescriptionMentionsSplit(t *testing.T) {
	cmd := newRootCmd()

	// Long description should mention the command split
	if !strings.Contains(cmd.Long, "split") || !strings.Contains(cmd.Long, "kscore-blueprint") {
		t.Errorf("expected Long description to mention the split from kscore-blueprint, got: %s", cmd.Long)
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
