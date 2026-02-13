// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	// Check basic properties
	if cmd.Use != "kscore-cluster" {
		t.Errorf("expected Use to be 'kscore-cluster', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "cluster management") {
		t.Errorf("expected Short to contain 'cluster management', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"status", "members", "leader", "add", "remove", "transfer-leader", "rebalance", "backup", "restore", "member", "election", "token", "version"}
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
	if !strings.Contains(output, "kscore-cluster") {
		t.Errorf("expected help output to contain 'kscore-cluster', got: %s", output)
	}
	if !strings.Contains(output, "leader") || !strings.Contains(output, "members") {
		t.Errorf("expected help output to mention leader and members, got: %s", output)
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
}

func TestStatusCommandExists(t *testing.T) {
	cmd := newRootCmd()
	statusCmd := findSubcommand(cmd, "status")
	if statusCmd == nil {
		t.Fatal("status subcommand not found")
	}

	if !strings.Contains(statusCmd.Short, "status") {
		t.Errorf("expected Short to contain 'status', got %s", statusCmd.Short)
	}
}

func TestMembersCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	membersCmd := findSubcommand(cmd, "members")
	if membersCmd == nil {
		t.Fatal("members subcommand not found")
	}

	detailsFlag := membersCmd.Flags().Lookup("details")
	if detailsFlag == nil {
		t.Error("expected --details flag on members command")
	}
}

func TestLeaderCommandExists(t *testing.T) {
	cmd := newRootCmd()
	leaderCmd := findSubcommand(cmd, "leader")
	if leaderCmd == nil {
		t.Fatal("leader subcommand not found")
	}

	if !strings.Contains(leaderCmd.Short, "leader") {
		t.Errorf("expected Short to contain 'leader', got %s", leaderCmd.Short)
	}
}

func TestAddCommandExists(t *testing.T) {
	cmd := newRootCmd()
	addCmd := findSubcommand(cmd, "add")
	if addCmd == nil {
		t.Fatal("add subcommand not found")
	}

	// Should require exactly 1 argument
	if addCmd.Args == nil {
		t.Error("expected add command to have Args validation")
	}
}

func TestRemoveCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	removeCmd := findSubcommand(cmd, "remove")
	if removeCmd == nil {
		t.Fatal("remove subcommand not found")
	}

	forceFlag := removeCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on remove command")
	}
}

func TestTransferLeaderCommandExists(t *testing.T) {
	cmd := newRootCmd()
	transferCmd := findSubcommand(cmd, "transfer-leader")
	if transferCmd == nil {
		t.Fatal("transfer-leader subcommand not found")
	}

	// Should require exactly 1 argument
	if transferCmd.Args == nil {
		t.Error("expected transfer-leader command to have Args validation")
	}
}

func TestRebalanceCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	rebalanceCmd := findSubcommand(cmd, "rebalance")
	if rebalanceCmd == nil {
		t.Fatal("rebalance subcommand not found")
	}

	reasonFlag := rebalanceCmd.Flags().Lookup("reason")
	if reasonFlag == nil {
		t.Fatal("expected --reason flag on rebalance command")
	}
	if reasonFlag.DefValue != "CLI request" {
		t.Errorf("expected reason default to be 'CLI request', got %s", reasonFlag.DefValue)
	}
}

func TestBackupCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	backupCmd := findSubcommand(cmd, "backup")
	if backupCmd == nil {
		t.Fatal("backup subcommand not found")
	}

	outputFlag := backupCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on backup command")
	}
}

func TestRestoreCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	restoreCmd := findSubcommand(cmd, "restore")
	if restoreCmd == nil {
		t.Fatal("restore subcommand not found")
	}

	inputFlag := restoreCmd.Flags().Lookup("input")
	if inputFlag == nil {
		t.Error("expected --input flag on restore command")
	}

	forceFlag := restoreCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on restore command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"status", "members", "leader", "add", "remove", "transfer-leader", "rebalance", "backup", "restore"}

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
			expectedUse: "kscore-cluster",
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

func TestFormatBool(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		trueVal  string
		falseVal string
		expected string
	}{
		{"true value", true, "Yes", "No", "Yes"},
		{"false value", false, "Yes", "No", "No"},
		{"true healthy", true, "Healthy", "Unhealthy", "Healthy"},
		{"false healthy", false, "Healthy", "Unhealthy", "Unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBool(tt.input, tt.trueVal, tt.falseVal)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"just now", "500ms", "just now"},
		{"seconds", "30s", "30s"},
		{"minutes", "5m", "5m"},
		{"hours", "2h", "2h"},
		{"days", "48h", "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dur := parseTestDuration(tt.input)
			result := formatDuration(dur)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCountHealthy(t *testing.T) {
	tests := []struct {
		name     string
		members  []MemberStatus
		expected int
	}{
		{
			name:     "all healthy",
			members:  []MemberStatus{{Status: "healthy"}, {Status: "healthy"}},
			expected: 2,
		},
		{
			name:     "mixed",
			members:  []MemberStatus{{Status: "healthy"}, {Status: "unhealthy"}, {Status: "healthy"}},
			expected: 2,
		},
		{
			name:     "none healthy",
			members:  []MemberStatus{{Status: "unhealthy"}, {Status: "unknown"}},
			expected: 0,
		},
		{
			name:     "empty",
			members:  []MemberStatus{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countHealthy(tt.members)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
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

func TestMemberSubcommands(t *testing.T) {
	cmd := newRootCmd()
	memberCmd := findSubcommand(cmd, "member")
	if memberCmd == nil {
		t.Fatal("member subcommand not found")
	}

	expectedSubs := []string{"add", "remove"}
	for _, expected := range expectedSubs {
		found := false
		for _, sub := range memberCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected member subcommand %s not found", expected)
		}
	}
}

func TestMemberAddCommandFlags(t *testing.T) {
	memberCmd := newMemberCommand()
	addCmd := findSubcommand(memberCmd, "add")
	if addCmd == nil {
		t.Fatal("member add subcommand not found")
	}

	if addCmd.Args == nil {
		t.Error("expected member add command to have Args validation")
	}

	dryRunFlag := addCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on member add command")
	}
}

func TestMemberRemoveCommandFlags(t *testing.T) {
	memberCmd := newMemberCommand()
	removeCmd := findSubcommand(memberCmd, "remove")
	if removeCmd == nil {
		t.Fatal("member remove subcommand not found")
	}

	if removeCmd.Args == nil {
		t.Error("expected member remove command to have Args validation")
	}

	forceFlag := removeCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on member remove command")
	}
}

func TestMemberAddDryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"member", "add", "10.0.0.5:9090", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("member add --dry-run failed: %v", err)
	}
}

func TestMemberRemoveDryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"member", "remove", "node-2", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("member remove --dry-run failed: %v", err)
	}
}

func TestElectionSubcommands(t *testing.T) {
	cmd := newRootCmd()
	electionCmd := findSubcommand(cmd, "election")
	if electionCmd == nil {
		t.Fatal("election subcommand not found")
	}

	restartCmd := findSubcommand(electionCmd, "restart")
	if restartCmd == nil {
		t.Fatal("election restart subcommand not found")
	}
}

func TestElectionRestartDryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"election", "restart", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("election restart --dry-run failed: %v", err)
	}
}

func TestJoinCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	joinCmd := findSubcommand(cmd, "join")
	if joinCmd == nil {
		t.Fatal("join subcommand not found")
	}

	tokenFlag := joinCmd.Flags().Lookup("token")
	if tokenFlag == nil {
		t.Error("expected --token flag on join command")
	}

	advertiseAddrFlag := joinCmd.Flags().Lookup("advertise-addr")
	if advertiseAddrFlag == nil {
		t.Error("expected --advertise-addr flag on join command")
	}
}

func TestJoinDryRunWithFlags(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"join", "https://ks-server:8080", "--dry-run", "--token", "test-token", "--advertise-addr", "10.0.1.5"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("join --dry-run with flags failed: %v", err)
	}
}

func TestJoinDryRunNoToken(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"join", "https://ks-server:8080", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("join --dry-run failed: %v", err)
	}
}

func TestMemberSubcommandHelp(t *testing.T) {
	subcmds := []string{"member add", "member remove", "election restart"}
	for _, subcmd := range subcmds {
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

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

func TestTokenSubcommands(t *testing.T) {
	cmd := newRootCmd()
	tokenCmd := findSubcommand(cmd, "token")
	if tokenCmd == nil {
		t.Fatal("token subcommand not found")
	}

	expectedSubs := []string{"generate", "list", "revoke"}
	for _, expected := range expectedSubs {
		found := false
		for _, sub := range tokenCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected token subcommand %s not found", expected)
		}
	}
}

func TestTokenGenerateCommandFlags(t *testing.T) {
	tokenCmd := newTokenCommand()
	genCmd := findSubcommand(tokenCmd, "generate")
	if genCmd == nil {
		t.Fatal("token generate subcommand not found")
	}

	labelFlag := genCmd.Flags().Lookup("label")
	if labelFlag == nil {
		t.Error("expected --label flag on token generate command")
	}

	ttlFlag := genCmd.Flags().Lookup("ttl")
	if ttlFlag == nil {
		t.Fatal("expected --ttl flag on token generate command")
	}
	if ttlFlag.DefValue != "24h" {
		t.Errorf("expected ttl default '24h', got %s", ttlFlag.DefValue)
	}

	maxUsesFlag := genCmd.Flags().Lookup("max-uses")
	if maxUsesFlag == nil {
		t.Fatal("expected --max-uses flag on token generate command")
	}
	if maxUsesFlag.DefValue != "0" {
		t.Errorf("expected max-uses default '0', got %s", maxUsesFlag.DefValue)
	}
}

func TestTokenRevokeRequiresArg(t *testing.T) {
	tokenCmd := newTokenCommand()
	revokeCmd := findSubcommand(tokenCmd, "revoke")
	if revokeCmd == nil {
		t.Fatal("token revoke subcommand not found")
	}
	if revokeCmd.Args == nil {
		t.Error("expected token revoke command to have Args validation")
	}
}

func TestTokenSubcommandHelp(t *testing.T) {
	subcmds := []string{"token generate", "token list", "token revoke"}
	for _, subcmd := range subcmds {
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

// parseTestDuration parses a test duration string
func parseTestDuration(s string) (d time.Duration) {
	switch {
	case strings.HasSuffix(s, "ms"):
		var ms int
		fmt.Sscanf(s, "%dms", &ms)
		d = time.Duration(ms) * time.Millisecond
	case strings.HasSuffix(s, "s"):
		var sec int
		fmt.Sscanf(s, "%ds", &sec)
		d = time.Duration(sec) * time.Second
	case strings.HasSuffix(s, "m"):
		var minutes int
		fmt.Sscanf(s, "%dm", &minutes)
		d = time.Duration(minutes) * time.Minute
	case strings.HasSuffix(s, "h"):
		var hr int
		fmt.Sscanf(s, "%dh", &hr)
		d = time.Duration(hr) * time.Hour
	}
	return
}
