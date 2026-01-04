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
	if cmd.Use != "kscore-monitor" {
		t.Errorf("expected Use to be 'kscore-monitor', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "TUI monitoring") {
		t.Errorf("expected Short to contain 'TUI monitoring', got %s", cmd.Short)
	}

	// Version command should be present
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected version subcommand to be present")
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
	if !strings.Contains(output, "kscore-monitor version") {
		t.Errorf("expected version output to contain 'kscore-monitor version', got: %s", output)
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
	if !strings.Contains(output, "kscore-monitor") {
		t.Errorf("expected help output to contain 'kscore-monitor', got: %s", output)
	}
	if !strings.Contains(output, "Features:") {
		t.Errorf("expected help output to contain 'Features:', got: %s", output)
	}
}

func TestRootCommandFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check specific flag defaults
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag to exist")
	}

	controlPlaneFlag := cmd.Flags().Lookup("control-plane")
	if controlPlaneFlag == nil {
		t.Error("expected --control-plane flag to exist")
	}
	if controlPlaneFlag.DefValue != "localhost:50051" {
		t.Errorf("expected control-plane default to be 'localhost:50051', got %s", controlPlaneFlag.DefValue)
	}

	natsFlag := cmd.Flags().Lookup("nats-url")
	if natsFlag == nil {
		t.Error("expected --nats-url flag to exist")
	}
	if natsFlag.DefValue != "nats://localhost:4222" {
		t.Errorf("expected nats-url default to be 'nats://localhost:4222', got %s", natsFlag.DefValue)
	}

	themeFlag := cmd.Flags().Lookup("theme")
	if themeFlag == nil {
		t.Error("expected --theme flag to exist")
	}
	if themeFlag.DefValue != "dark" {
		t.Errorf("expected theme default to be 'dark', got %s", themeFlag.DefValue)
	}

	refreshFlag := cmd.Flags().Lookup("refresh")
	if refreshFlag == nil {
		t.Error("expected --refresh flag to exist")
	}
	if refreshFlag.DefValue != "2" {
		t.Errorf("expected refresh default to be '2', got %s", refreshFlag.DefValue)
	}

	noColorFlag := cmd.Flags().Lookup("no-color")
	if noColorFlag == nil {
		t.Error("expected --no-color flag to exist")
	}
}

func TestOptionsType(t *testing.T) {
	opts := &Options{
		ConfigFile:   "/path/to/config.yaml",
		ControlPlane: "localhost:9090",
		NATSURL:      "nats://custom:4222",
		Theme:        "light",
		Refresh:      5,
		NoColor:      true,
	}

	if opts.ConfigFile != "/path/to/config.yaml" {
		t.Errorf("expected ConfigFile to be '/path/to/config.yaml', got %s", opts.ConfigFile)
	}

	if opts.ControlPlane != "localhost:9090" {
		t.Errorf("expected ControlPlane to be 'localhost:9090', got %s", opts.ControlPlane)
	}

	if opts.Theme != "light" {
		t.Errorf("expected Theme to be 'light', got %s", opts.Theme)
	}

	if opts.Refresh != 5 {
		t.Errorf("expected Refresh to be 5, got %d", opts.Refresh)
	}

	if !opts.NoColor {
		t.Error("expected NoColor to be true")
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
			expectedUse: "kscore-monitor",
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

func TestVersionCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "version") {
		t.Errorf("expected help to contain 'version', got: %s", output)
	}
}
