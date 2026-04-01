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
	if cmd.Use != "kscore-cluster-backup" {
		t.Errorf("expected Use to be 'kscore-cluster-backup', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "backup") {
		t.Errorf("expected Short to contain 'backup', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "backup", "restore", "list", "verify", "schedule"}
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
	if !strings.Contains(output, "kscore-cluster-backup") {
		t.Errorf("expected help output to contain 'kscore-cluster-backup', got: %s", output)
	}
	if !strings.Contains(output, "backup") || !strings.Contains(output, "restore") {
		t.Errorf("expected help output to mention backup and restore, got: %s", output)
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

	// Check verbose flag
	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("expected --verbose flag")
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

func TestBackupCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	backupCmd := findSubcommand(cmd, "backup")
	if backupCmd == nil {
		t.Fatal("backup subcommand not found")
	}

	// Check flags
	fileFlag := backupCmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Error("expected --file flag on backup command")
	}

	compressFlag := backupCmd.Flags().Lookup("compress")
	if compressFlag == nil {
		t.Error("expected --compress flag on backup command")
	}

	encryptFlag := backupCmd.Flags().Lookup("encrypt")
	if encryptFlag == nil {
		t.Error("expected --encrypt flag on backup command")
	}

	descriptionFlag := backupCmd.Flags().Lookup("description")
	if descriptionFlag == nil {
		t.Error("expected --description flag on backup command")
	}
}

func TestRestoreCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	restoreCmd := findSubcommand(cmd, "restore")
	if restoreCmd == nil {
		t.Fatal("restore subcommand not found")
	}

	// Check flags
	inputFlag := restoreCmd.Flags().Lookup("input")
	if inputFlag == nil {
		t.Error("expected --input flag on restore command")
	}

	forceFlag := restoreCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on restore command")
	}

	dryRunFlag := restoreCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on restore command")
	}
}

func TestListCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	listCmd := findSubcommand(cmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	limitFlag := listCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on list command")
	}
	if limitFlag.DefValue != "20" {
		t.Errorf("expected limit default to be '20', got %s", limitFlag.DefValue)
	}
}

func TestVerifyCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	verifyCmd := findSubcommand(cmd, "verify")
	if verifyCmd == nil {
		t.Fatal("verify subcommand not found")
	}

	inputFlag := verifyCmd.Flags().Lookup("input")
	if inputFlag == nil {
		t.Error("expected --input flag on verify command")
	}
}

func TestScheduleSubcommands(t *testing.T) {
	cmd := newRootCmd()
	scheduleCmd := findSubcommand(cmd, "schedule")
	if scheduleCmd == nil {
		t.Fatal("schedule subcommand not found")
	}

	// Check nested subcommands
	expectedSubcmds := []string{"list", "add", "remove"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range scheduleCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected schedule subcommand %s not found", expected)
		}
	}
}

func TestScheduleAddFlags(t *testing.T) {
	cmd := newRootCmd()
	scheduleCmd := findSubcommand(cmd, "schedule")
	if scheduleCmd == nil {
		t.Fatal("schedule subcommand not found")
	}

	addCmd := findSubcommand(scheduleCmd, "add")
	if addCmd == nil {
		t.Fatal("schedule add subcommand not found")
	}

	// Should require exactly 1 argument
	if addCmd.Args == nil {
		t.Error("expected schedule add command to have Args validation")
	}

	cronFlag := addCmd.Flags().Lookup("cron")
	if cronFlag == nil {
		t.Fatal("expected --cron flag on schedule add command")
	}
	if cronFlag.DefValue != "0 0 * * *" {
		t.Errorf("expected cron default to be '0 0 * * *', got %s", cronFlag.DefValue)
	}

	retentionFlag := addCmd.Flags().Lookup("retention")
	if retentionFlag == nil {
		t.Error("expected --retention flag on schedule add command")
	}
}

func TestScheduleRemoveArgs(t *testing.T) {
	cmd := newRootCmd()
	scheduleCmd := findSubcommand(cmd, "schedule")
	if scheduleCmd == nil {
		t.Fatal("schedule subcommand not found")
	}

	removeCmd := findSubcommand(scheduleCmd, "remove")
	if removeCmd == nil {
		t.Fatal("schedule remove subcommand not found")
	}

	// Should require exactly 1 argument
	if removeCmd.Args == nil {
		t.Error("expected schedule remove command to have Args validation")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"backup", "restore", "list", "verify"}

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

func TestScheduleSubcommandHelp(t *testing.T) {
	subcommands := []string{"schedule list", "schedule add", "schedule remove"}

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
			expectedUse: "kscore-cluster-backup",
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

func TestGetAPIScheme(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected string
	}{
		{"localhost", "localhost:9090", "http"},
		{"127.0.0.1", "127.0.0.1:9090", "http"},
		{"IPv6 loopback", "[::1]:9090", "http"},
		{"remote host", "example.com:9090", "https"},
		{"remote ip", "192.168.1.1:9090", "https"},
		{"IPv6 remote", "[2001:db8::1]:9090", "https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAPIScheme(tt.addr)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"bytes", 500, "500 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"mixed", 1536, "1.5 KB"},
		{"large megabytes", 5 * 1024 * 1024, "5.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestBackupInfo(t *testing.T) {
	// Test BackupInfo struct
	info := BackupInfo{
		ID:          "backup-123",
		Size:        1024 * 1024,
		Description: "Test backup",
		Compressed:  true,
		Encrypted:   false,
	}

	if info.ID != "backup-123" {
		t.Errorf("expected ID to be 'backup-123', got %s", info.ID)
	}
	if !info.Compressed {
		t.Error("expected Compressed to be true")
	}
	if info.Encrypted {
		t.Error("expected Encrypted to be false")
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
