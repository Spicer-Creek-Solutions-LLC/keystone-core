package main

import (
	"bytes"
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-loadtest" {
		t.Errorf("Use = %v, want kscore-loadtest", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"run",
		"scenarios",
		"report",
		"version",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	if cmd == nil {
		t.Fatal("newVersionCmd should not return nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %v, want version", cmd.Use)
	}

	// Test execution
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("version command should produce output")
	}
}

func TestNewRunCmd(t *testing.T) {
	cmd := newRunCmd()

	if cmd == nil {
		t.Fatal("newRunCmd should not return nil")
	}
	if cmd.Use != "run" {
		t.Errorf("Use = %v, want run", cmd.Use)
	}

	// Check flags exist
	flags := []string{
		"agents", "scenario", "duration", "ramp-up",
		"heartbeat-interval", "commands-per-agent",
		"concurrent-commands", "report-dir", "nats-port",
	}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewScenariosCmd(t *testing.T) {
	cmd := newScenariosCmd()

	if cmd == nil {
		t.Fatal("newScenariosCmd should not return nil")
	}
	if cmd.Use != "scenarios" {
		t.Errorf("Use = %v, want scenarios", cmd.Use)
	}
}

func TestNewReportCmd(t *testing.T) {
	cmd := newReportCmd()

	if cmd == nil {
		t.Fatal("newReportCmd should not return nil")
	}
	if cmd.Use != "report" {
		t.Errorf("Use = %v, want report", cmd.Use)
	}

	// Check flags exist
	if cmd.Flags().Lookup("file") == nil {
		t.Error("expected flag 'file' not found")
	}
}

func TestPersistentFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check persistent flags exist
	flags := []string{"output", "verbose", "audit-level", "audit-output"}
	for _, flag := range flags {
		if cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("expected persistent flag %q not found", flag)
		}
	}
}
