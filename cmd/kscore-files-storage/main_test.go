// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
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
	if cmd.Use != "kscore-files-storage" {
		t.Errorf("expected Use to be 'kscore-files-storage', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Storage") {
		t.Errorf("expected Short to contain 'Storage', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "backend", "mirrors"}
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
	if !strings.Contains(output, "kscore-files-storage") {
		t.Errorf("expected help output to contain 'kscore-files-storage', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check nats-url flag
	natsURLFlag := cmd.PersistentFlags().Lookup("nats-url")
	if natsURLFlag == nil {
		t.Error("expected --nats-url flag")
	}
	if natsURLFlag.DefValue != "nats://localhost:4222" {
		t.Errorf("expected nats-url default to be 'nats://localhost:4222', got %s", natsURLFlag.DefValue)
	}

	// Check cluster-id flag
	clusterIDFlag := cmd.PersistentFlags().Lookup("cluster-id")
	if clusterIDFlag == nil {
		t.Error("expected --cluster-id flag")
	}

	// Check output flag
	outputFlag := cmd.PersistentFlags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag")
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

func TestBackendSubcommands(t *testing.T) {
	cmd := newBackendCmd()
	if cmd == nil {
		t.Fatal("backend command is nil")
	}

	// Check nested subcommands
	expectedSubcmds := []string{"list", "status", "sync", "enable", "disable", "health"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected backend subcommand %s not found", expected)
		}
	}
}

func TestBackendStatusArgs(t *testing.T) {
	cmd := newBackendStatusCmd()
	if cmd == nil {
		t.Fatal("backend status command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected backend status command to have Args validation")
	}
}

func TestBackendSyncArgs(t *testing.T) {
	cmd := newBackendSyncCmd()
	if cmd == nil {
		t.Fatal("backend sync command is nil")
	}

	// Should require exactly 2 arguments
	if cmd.Args == nil {
		t.Error("expected backend sync command to have Args validation")
	}

	// Check flags
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on backend sync command")
	}

	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("expected --force flag on backend sync command")
	}
}

func TestBackendEnableArgs(t *testing.T) {
	cmd := newBackendEnableCmd()
	if cmd == nil {
		t.Fatal("backend enable command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected backend enable command to have Args validation")
	}
}

func TestBackendDisableArgs(t *testing.T) {
	cmd := newBackendDisableCmd()
	if cmd == nil {
		t.Fatal("backend disable command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected backend disable command to have Args validation")
	}
}

func TestMirrorsSubcommands(t *testing.T) {
	cmd := newMirrorsCmd()
	if cmd == nil {
		t.Fatal("mirrors command is nil")
	}

	// Check nested subcommands
	expectedSubcmds := []string{"list", "show", "sync", "health", "conflicts"}
	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected mirrors subcommand %s not found", expected)
		}
	}
}

func TestMirrorsShowArgs(t *testing.T) {
	cmd := newMirrorsShowCmd()
	if cmd == nil {
		t.Fatal("mirrors show command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected mirrors show command to have Args validation")
	}
}

func TestMirrorsSyncArgs(t *testing.T) {
	cmd := newMirrorsSyncCmd()
	if cmd == nil {
		t.Fatal("mirrors sync command is nil")
	}

	// Should require exactly 1 argument
	if cmd.Args == nil {
		t.Error("expected mirrors sync command to have Args validation")
	}

	// Check flags
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("expected --dry-run flag on mirrors sync command")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"backend", "mirrors"}

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

func TestBackendNestedHelp(t *testing.T) {
	subcommands := []string{"backend list", "backend status", "backend sync", "backend enable", "backend disable", "backend health"}

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

func TestMirrorsNestedHelp(t *testing.T) {
	subcommands := []string{"mirrors list", "mirrors show", "mirrors sync", "mirrors health", "mirrors conflicts"}

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
			expectedUse: "kscore-files-storage",
		},
		{
			name:        "version command",
			cmdFactory:  newVersionCmd,
			expectedUse: "version",
		},
		{
			name:        "backend command",
			cmdFactory:  newBackendCmd,
			expectedUse: "backend",
		},
		{
			name:        "mirrors command",
			cmdFactory:  newMirrorsCmd,
			expectedUse: "mirrors",
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

func TestFormatHealthy(t *testing.T) {
	tests := []struct {
		name     string
		healthy  bool
		expected string
	}{
		{"healthy", true, "healthy"},
		{"unhealthy", false, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHealthy(tt.healthy)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "-"},
		{"microseconds", 500 * time.Microsecond, "500µs"},
		{"milliseconds", 100 * time.Millisecond, "100ms"},
		{"seconds", 2 * time.Second, "2.0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestBackendInfo(t *testing.T) {
	// Test BackendInfo struct
	info := BackendInfo{
		Name:     "test-backend",
		Type:     "local",
		Enabled:  true,
		ReadOnly: false,
		Paths:    []string{"/data"},
	}

	if info.Name != "test-backend" {
		t.Errorf("expected Name to be 'test-backend', got %s", info.Name)
	}
	if !info.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestBackendStatus(t *testing.T) {
	// Test BackendStatus struct
	status := BackendStatus{
		Name:     "test-backend",
		Type:     "s3",
		Enabled:  true,
		Healthy:  true,
		ReadOnly: false,
		Stats: BackendStats{
			FileCount:    100,
			TotalSize:    1024 * 1024,
			ReadCount:    50,
			WriteCount:   25,
			BytesRead:    512 * 1024,
			BytesWritten: 256 * 1024,
			ErrorCount:   0,
		},
	}

	if status.Stats.FileCount != 100 {
		t.Errorf("expected FileCount to be 100, got %d", status.Stats.FileCount)
	}
}

func TestBackendHealth(t *testing.T) {
	// Test BackendHealth struct
	health := BackendHealth{
		Name:    "test-backend",
		Healthy: true,
		Latency: 50 * time.Millisecond,
		Message: "OK",
	}

	if !health.Healthy {
		t.Error("expected Healthy to be true")
	}
	if health.Latency != 50*time.Millisecond {
		t.Errorf("expected Latency to be 50ms, got %v", health.Latency)
	}
}
