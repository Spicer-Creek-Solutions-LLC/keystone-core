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
	if cmd.Use != "kscore-files" {
		t.Errorf("expected Use to be 'kscore-files', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "File") {
		t.Errorf("expected Short to contain 'File', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "serve", "files", "cache", "namespace", "backend", "mirrors"}
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
	versionCmd := findSubcommand(cmd, "version")
	if versionCmd == nil {
		t.Fatal("version subcommand not found")
	}

	if versionCmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", versionCmd.Use)
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
	if !strings.Contains(output, "kscore-files") {
		t.Errorf("expected help output to contain 'kscore-files', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag")
	}

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

	// Check instance-id flag
	instanceIDFlag := cmd.PersistentFlags().Lookup("instance-id")
	if instanceIDFlag == nil {
		t.Error("expected --instance-id flag")
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

func TestServeCommandExists(t *testing.T) {
	cmd := newRootCmd()
	serveCmd := findSubcommand(cmd, "serve")
	if serveCmd == nil {
		t.Fatal("serve subcommand not found")
	}

	if !strings.Contains(serveCmd.Short, "Start") || !strings.Contains(serveCmd.Short, "server") {
		t.Errorf("expected Short to mention starting the server, got %s", serveCmd.Short)
	}
}

func TestFilesCommandExists(t *testing.T) {
	cmd := newRootCmd()
	filesCmd := findSubcommand(cmd, "files")
	if filesCmd == nil {
		t.Fatal("files subcommand not found")
	}
}

func TestCacheCommandExists(t *testing.T) {
	cmd := newRootCmd()
	cacheCmd := findSubcommand(cmd, "cache")
	if cacheCmd == nil {
		t.Fatal("cache subcommand not found")
	}
}

func TestNamespaceCommandExists(t *testing.T) {
	cmd := newRootCmd()
	namespaceCmd := findSubcommand(cmd, "namespace")
	if namespaceCmd == nil {
		t.Fatal("namespace subcommand not found")
	}
}

func TestBackendCommandExists(t *testing.T) {
	cmd := newRootCmd()
	backendCmd := findSubcommand(cmd, "backend")
	if backendCmd == nil {
		t.Fatal("backend subcommand not found (deprecated but should exist)")
	}
}

func TestMirrorsCommandExists(t *testing.T) {
	cmd := newRootCmd()
	mirrorsCmd := findSubcommand(cmd, "mirrors")
	if mirrorsCmd == nil {
		t.Fatal("mirrors subcommand not found (deprecated but should exist)")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"serve", "files", "cache", "namespace"}

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
			expectedUse: "kscore-files",
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
		if cmd == nil {
			t.Fatalf("execution %d: command is nil", i)
		}
	}
}

func TestDescriptionMentionsNATS(t *testing.T) {
	cmd := newRootCmd()

	// Long description should mention NATS
	if !strings.Contains(cmd.Long, "NATS") {
		t.Errorf("expected Long description to mention NATS, got: %s", cmd.Long)
	}
}

func TestServerConfig(t *testing.T) {
	// Test ServerConfig struct
	config := ServerConfig{}
	config.Server.ClusterID = "test-cluster"
	config.Server.InstanceID = "instance-1"
	config.Server.Workers = 4
	config.NATS.URL = "nats://localhost:4222"

	if config.Server.ClusterID != "test-cluster" {
		t.Errorf("expected ClusterID to be 'test-cluster', got %s", config.Server.ClusterID)
	}
	if config.Server.Workers != 4 {
		t.Errorf("expected Workers to be 4, got %d", config.Server.Workers)
	}
}

func TestBackendConfig(t *testing.T) {
	// Test BackendConfig struct
	config := BackendConfig{
		Name:     "local",
		Type:     "filesystem",
		RootPath: "/var/lib/kscore/files",
		Paths:    []string{"/data"},
		ReadOnly: false,
	}

	if config.Name != "local" {
		t.Errorf("expected Name to be 'local', got %s", config.Name)
	}
	if config.Type != "filesystem" {
		t.Errorf("expected Type to be 'filesystem', got %s", config.Type)
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
