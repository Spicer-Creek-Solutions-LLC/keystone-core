package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNATSCommandExists(t *testing.T) {
	cmd := newRootCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "nats" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nats subcommand not found")
	}
}

func TestNATSSubcommands(t *testing.T) {
	cmd := newNATSCmd()

	expected := []string{"ping", "status", "test"}
	for _, name := range expected {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected nats subcommand %q not found", name)
		}
	}
}

func TestNATSCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("nats --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NATS") {
		t.Errorf("expected help to contain 'NATS', got: %s", output)
	}
	if !strings.Contains(output, "ping") {
		t.Errorf("expected help to list 'ping' subcommand, got: %s", output)
	}
	if !strings.Contains(output, "status") {
		t.Errorf("expected help to list 'status' subcommand, got: %s", output)
	}
	if !strings.Contains(output, "test") {
		t.Errorf("expected help to list 'test' subcommand, got: %s", output)
	}
}

func TestNATSPingHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "ping", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("nats ping --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "round-trip") {
		t.Errorf("expected help to contain 'round-trip', got: %s", output)
	}
	if !strings.Contains(output, "count") {
		t.Errorf("expected help to contain 'count' flag, got: %s", output)
	}
	if !strings.Contains(output, "timeout") {
		t.Errorf("expected help to contain 'timeout' flag, got: %s", output)
	}
}

func TestNATSStatusHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "status", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("nats status --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "connection") {
		t.Errorf("expected help to contain 'connection', got: %s", output)
	}
}

func TestNATSTestHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"nats", "test", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("nats test --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "connectivity") {
		t.Errorf("expected help to contain 'connectivity', got: %s", output)
	}
	if !strings.Contains(output, "verbose") {
		t.Errorf("expected help to contain 'verbose' flag, got: %s", output)
	}
}

func TestNATSPingFlags(t *testing.T) {
	cmd := newNATSPingCmd()

	countFlag := cmd.Flags().Lookup("count")
	if countFlag == nil {
		t.Fatal("expected --count flag to be registered")
	}
	if countFlag.DefValue != "3" {
		t.Errorf("expected --count default to be '3', got %q", countFlag.DefValue)
	}

	timeoutFlag := cmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Fatal("expected --timeout flag to be registered")
	}
	if timeoutFlag.DefValue != "5s" {
		t.Errorf("expected --timeout default to be '5s', got %q", timeoutFlag.DefValue)
	}
}

func TestNATSTestFlags(t *testing.T) {
	cmd := newNATSTestCmd()

	verboseFlag := cmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("expected --verbose flag to be registered")
	}
	if verboseFlag.DefValue != "false" {
		t.Errorf("expected --verbose default to be 'false', got %q", verboseFlag.DefValue)
	}
}

func TestNATSCmdStructure(t *testing.T) {
	cmd := newNATSCmd()

	if cmd.Use != "nats" {
		t.Errorf("expected Use to be 'nats', got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}
	if cmd.Long == "" {
		t.Error("expected Long description to be set")
	}

	subs := cmd.Commands()
	if len(subs) != 3 {
		t.Errorf("expected 3 subcommands, got %d", len(subs))
	}
}
