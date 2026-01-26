package main

import (
	"bytes"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/statemgmt"
)

func TestRootCmd(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}

	if rootCmd.Use != "state" {
		t.Errorf("Use = %v, want state", rootCmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	expectedSubcommands := []string{
		"version",
		"apply",
		"check",
		"test",
		"drift",
		"diff",
		"show",
		"history",
		"rollback",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range rootCmd.Commands() {
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

func TestVersionCmd(t *testing.T) {
	if versionCmd == nil {
		t.Fatal("versionCmd should not be nil")
	}
	if versionCmd.Use != "version" {
		t.Errorf("Use = %v, want version", versionCmd.Use)
	}

	// Verify the command has a Run function defined
	if versionCmd.Run == nil {
		t.Error("versionCmd should have a Run function defined")
	}
}

func TestApplyCmd(t *testing.T) {
	if applyCmd == nil {
		t.Fatal("applyCmd should not be nil")
	}
	if applyCmd.Use != "apply <statefile>" {
		t.Errorf("Use = %v, want 'apply <statefile>'", applyCmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "vars", "dry-run", "preview"}
	for _, flag := range flags {
		if applyCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestCheckCmd(t *testing.T) {
	if checkCmd == nil {
		t.Fatal("checkCmd should not be nil")
	}
	if checkCmd.Use != "check <statefile>" {
		t.Errorf("Use = %v, want 'check <statefile>'", checkCmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "vars"}
	for _, flag := range flags {
		if checkCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestTestCmd(t *testing.T) {
	if testCmd == nil {
		t.Fatal("testCmd should not be nil")
	}
	if testCmd.Use != "test <statefile>" {
		t.Errorf("Use = %v, want 'test <statefile>'", testCmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "vars"}
	for _, flag := range flags {
		if testCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestDriftCmd(t *testing.T) {
	if driftCmd == nil {
		t.Fatal("driftCmd should not be nil")
	}
	if driftCmd.Use != "drift <statefile>" {
		t.Errorf("Use = %v, want 'drift <statefile>'", driftCmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "vars"}
	for _, flag := range flags {
		if driftCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestDiffCmd(t *testing.T) {
	if diffCmd == nil {
		t.Fatal("diffCmd should not be nil")
	}
	if diffCmd.Use != "diff <statefile>" {
		t.Errorf("Use = %v, want 'diff <statefile>'", diffCmd.Use)
	}

	// Check aliases
	if len(diffCmd.Aliases) == 0 || diffCmd.Aliases[0] != "compare" {
		t.Error("expected alias 'compare' not found")
	}

	// Check flags exist
	flags := []string{"target", "vars"}
	for _, flag := range flags {
		if diffCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestShowCmd(t *testing.T) {
	if showCmd == nil {
		t.Fatal("showCmd should not be nil")
	}
	if showCmd.Use != "show <statefile>" {
		t.Errorf("Use = %v, want 'show <statefile>'", showCmd.Use)
	}

	// Check flags exist
	if showCmd.Flags().Lookup("vars") == nil {
		t.Error("expected flag 'vars' not found")
	}
}

func TestHistoryCmd(t *testing.T) {
	if historyCmd == nil {
		t.Fatal("historyCmd should not be nil")
	}
	if historyCmd.Use != "history [application-id]" {
		t.Errorf("Use = %v, want 'history [application-id]'", historyCmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "limit", "json"}
	for _, flag := range flags {
		if historyCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestRollbackCmd(t *testing.T) {
	if rollbackCmd == nil {
		t.Fatal("rollbackCmd should not be nil")
	}
	if rollbackCmd.Use != "rollback <application-id>" {
		t.Errorf("Use = %v, want 'rollback <application-id>'", rollbackCmd.Use)
	}

	// Check flags exist
	flags := []string{"force", "dry-run"}
	for _, flag := range flags {
		if rollbackCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestPersistentFlags(t *testing.T) {
	// Check persistent flags exist
	flags := []string{"audit-level", "audit-output"}
	for _, flag := range flags {
		if rootCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("expected persistent flag %q not found", flag)
		}
	}
}

func TestWarnTarget(t *testing.T) {
	// Test that warnTarget doesn't panic with empty target
	buf := new(bytes.Buffer)
	cmd := rootCmd
	cmd.SetErr(buf)

	// Should not panic
	warnTarget(cmd, "")

	// With non-empty target, should print warning
	warnTarget(cmd, "role:web")
	if buf.Len() == 0 {
		t.Error("warnTarget should produce warning output for non-empty target")
	}
}

func TestHasRequisites(t *testing.T) {
	// Test empty requisites
	emptyReq := &statemgmt.Requisites{}
	if hasRequisites(emptyReq) {
		t.Error("hasRequisites should return false for empty requisites")
	}
}
