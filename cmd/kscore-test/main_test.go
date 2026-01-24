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

	if cmd.Use != "kscore-test" {
		t.Errorf("Use = %v, want kscore-test", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"smoke",
		"integration",
		"run",
		"list",
		"show",
		"history",
		"suite",
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

func TestNewSmokeCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newSmokeCmd(cfg)

	if cmd == nil {
		t.Fatal("newSmokeCmd should not return nil")
	}
	if cmd.Use != "smoke" {
		t.Errorf("Use = %v, want smoke", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "timeout", "tags"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewIntegrationCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newIntegrationCmd(cfg)

	if cmd == nil {
		t.Fatal("newIntegrationCmd should not return nil")
	}
	if cmd.Use != "integration" {
		t.Errorf("Use = %v, want integration", cmd.Use)
	}

	// Check flags exist
	flags := []string{"suite", "target", "timeout", "tags", "parallel"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewRunCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRunCmd(cfg)

	if cmd == nil {
		t.Fatal("newRunCmd should not return nil")
	}
	if cmd.Use != "run" {
		t.Errorf("Use = %v, want run", cmd.Use)
	}

	// Check flags exist
	flags := []string{"suite", "target", "timeout", "tags", "parallel", "dry-run", "fail-fast"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewListCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newListCmd(cfg)

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"type", "tags"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewShowCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newShowCmd(cfg)

	if cmd == nil {
		t.Fatal("newShowCmd should not return nil")
	}
	if cmd.Use != "show <test-id>" {
		t.Errorf("Use = %v, want 'show <test-id>'", cmd.Use)
	}
}

func TestNewHistoryCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newHistoryCmd(cfg)

	if cmd == nil {
		t.Fatal("newHistoryCmd should not return nil")
	}
	if cmd.Use != "history" {
		t.Errorf("Use = %v, want history", cmd.Use)
	}

	// Check flags exist
	flags := []string{"suite", "limit", "status"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewSuiteCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newSuiteCmd(cfg)

	if cmd == nil {
		t.Fatal("newSuiteCmd should not return nil")
	}
	if cmd.Use != "suite" {
		t.Errorf("Use = %v, want suite", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"show", "create", "delete"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestConfigStructure(t *testing.T) {
	cfg := Config{
		ServerAddr:   "localhost:9090",
		OutputFormat: "json",
		Verbose:      true,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("OutputFormat = %v, want json", cfg.OutputFormat)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestTestResultStructure(t *testing.T) {
	result := TestResult{
		ID:          "test-001",
		Suite:       "basic",
		Type:        "integration",
		Status:      "passed",
		Target:      "environment:staging",
		Total:       10,
		Passed:      9,
		Failed:      1,
		Skipped:     0,
		Duration:    "45s",
		StartedAt:   "2024-01-15T10:00:00Z",
		CompletedAt: "2024-01-15T10:00:45Z",
		Labels:      map[string]string{"env": "staging"},
	}

	if result.ID != "test-001" {
		t.Errorf("ID = %v, want test-001", result.ID)
	}
	if result.Suite != "basic" {
		t.Errorf("Suite = %v, want basic", result.Suite)
	}
	if result.Total != 10 {
		t.Errorf("Total = %d, want 10", result.Total)
	}
	if result.Passed != 9 {
		t.Errorf("Passed = %d, want 9", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestTestCaseStructure(t *testing.T) {
	tc := TestCase{
		Name:     "agent_registration",
		Status:   "passed",
		Duration: "2.5s",
		Message:  "Registration successful",
		Error:    "",
	}

	if tc.Name != "agent_registration" {
		t.Errorf("Name = %v, want agent_registration", tc.Name)
	}
	if tc.Status != "passed" {
		t.Errorf("Status = %v, want passed", tc.Status)
	}
	if tc.Duration != "2.5s" {
		t.Errorf("Duration = %v, want 2.5s", tc.Duration)
	}
}

func TestTestFailureStructure(t *testing.T) {
	failure := TestFailure{
		Test:    "cluster/rebalancing",
		Message: "Timeout waiting for rebalance completion",
		Details: "Expected completion within 30s, got timeout",
	}

	if failure.Test != "cluster/rebalancing" {
		t.Errorf("Test = %v, want cluster/rebalancing", failure.Test)
	}
	if failure.Message != "Timeout waiting for rebalance completion" {
		t.Errorf("Message = %v, want 'Timeout waiting for rebalance completion'", failure.Message)
	}
}

func TestTestSuiteStructure(t *testing.T) {
	suite := TestSuite{
		Name:        "basic",
		Description: "Basic functionality tests",
		Type:        "integration",
		Tests:       5,
		Tags:        []string{"core", "agent"},
		Timeout:     "15m",
		LastRun:     "2024-01-15T09:00:00Z",
		LastStatus:  "passed",
	}

	if suite.Name != "basic" {
		t.Errorf("Name = %v, want basic", suite.Name)
	}
	if suite.Type != "integration" {
		t.Errorf("Type = %v, want integration", suite.Type)
	}
	if suite.Tests != 5 {
		t.Errorf("Tests = %d, want 5", suite.Tests)
	}
	if len(suite.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(suite.Tags))
	}
	if suite.LastStatus != "passed" {
		t.Errorf("LastStatus = %v, want passed", suite.LastStatus)
	}
}

func TestPersistentFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check persistent flags exist
	flags := []string{"server", "output", "verbose", "audit-level", "audit-output"}
	for _, flag := range flags {
		if cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("expected persistent flag %q not found", flag)
		}
	}
}
