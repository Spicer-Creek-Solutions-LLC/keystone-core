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
	if cmd.Use != "kscore-telemetry-gateway" {
		t.Errorf("expected Use to be 'kscore-telemetry-gateway', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Telemetry") {
		t.Errorf("expected Short to contain 'Telemetry', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "serve"}
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
	if !strings.Contains(output, "kscore-telemetry-gateway") {
		t.Errorf("expected help output to contain 'kscore-telemetry-gateway', got: %s", output)
	}
	if !strings.Contains(output, "metrics") || !strings.Contains(output, "logs") || !strings.Contains(output, "traces") {
		t.Errorf("expected help output to mention metrics, logs, and traces, got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag")
	}

	// Check listen flag
	listenFlag := cmd.PersistentFlags().Lookup("listen")
	if listenFlag == nil {
		t.Error("expected --listen flag")
	}

	// Check nats-url flag
	natsURLFlag := cmd.PersistentFlags().Lookup("nats-url")
	if natsURLFlag == nil {
		t.Error("expected --nats-url flag")
	}

	// Check metrics flag
	metricsFlag := cmd.PersistentFlags().Lookup("metrics")
	if metricsFlag == nil {
		t.Error("expected --metrics flag")
	}
	if metricsFlag.DefValue != "true" {
		t.Errorf("expected metrics default to be 'true', got %s", metricsFlag.DefValue)
	}

	// Check logs flag
	logsFlag := cmd.PersistentFlags().Lookup("logs")
	if logsFlag == nil {
		t.Error("expected --logs flag")
	}
	if logsFlag.DefValue != "true" {
		t.Errorf("expected logs default to be 'true', got %s", logsFlag.DefValue)
	}

	// Check traces flag
	tracesFlag := cmd.PersistentFlags().Lookup("traces")
	if tracesFlag == nil {
		t.Error("expected --traces flag")
	}
	if tracesFlag.DefValue != "true" {
		t.Errorf("expected traces default to be 'true', got %s", tracesFlag.DefValue)
	}
}

func TestServeCommandExists(t *testing.T) {
	cmd := newRootCmd()
	serveCmd := findSubcommand(cmd, "serve")
	if serveCmd == nil {
		t.Fatal("serve subcommand not found")
	}

	if !strings.Contains(serveCmd.Short, "Start") || !strings.Contains(serveCmd.Short, "gateway") {
		t.Errorf("expected Short to mention starting the gateway, got %s", serveCmd.Short)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"serve", "version"}

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
			expectedUse: "kscore-telemetry-gateway",
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

func TestDescriptionMentionsNATSAndTelemetry(t *testing.T) {
	cmd := newRootCmd()

	// Long description should mention NATS and telemetry types
	if !strings.Contains(cmd.Long, "NATS") {
		t.Errorf("expected Long description to mention NATS, got: %s", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "metrics") {
		t.Errorf("expected Long description to mention metrics, got: %s", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "logs") {
		t.Errorf("expected Long description to mention logs, got: %s", cmd.Long)
	}
	if !strings.Contains(cmd.Long, "traces") {
		t.Errorf("expected Long description to mention traces, got: %s", cmd.Long)
	}
}

func TestConfig(t *testing.T) {
	// Test Config struct defaults
	config := Config{
		MetricsEnabled: true,
		LogsEnabled:    true,
		TracesEnabled:  true,
	}

	if !config.MetricsEnabled {
		t.Error("expected MetricsEnabled to be true")
	}
	if !config.LogsEnabled {
		t.Error("expected LogsEnabled to be true")
	}
	if !config.TracesEnabled {
		t.Error("expected TracesEnabled to be true")
	}
}

func TestConfigWithValues(t *testing.T) {
	config := Config{
		ConfigFile:     "/etc/kscore/gateway.yaml",
		ListenAddr:     "0.0.0.0:9091",
		NATSURL:        "nats://localhost:4222",
		MetricsEnabled: true,
		LogsEnabled:    false,
		TracesEnabled:  true,
	}

	if config.ConfigFile != "/etc/kscore/gateway.yaml" {
		t.Errorf("expected ConfigFile to be '/etc/kscore/gateway.yaml', got %s", config.ConfigFile)
	}
	if config.ListenAddr != "0.0.0.0:9091" {
		t.Errorf("expected ListenAddr to be '0.0.0.0:9091', got %s", config.ListenAddr)
	}
	if config.NATSURL != "nats://localhost:4222" {
		t.Errorf("expected NATSURL to be 'nats://localhost:4222', got %s", config.NATSURL)
	}
	if config.LogsEnabled {
		t.Error("expected LogsEnabled to be false")
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
