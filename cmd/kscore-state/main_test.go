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
		"compile",
		"vars",
		"export",
		"restore",
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
	flags := []string{"target", "vars", "fix"}
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

func TestCompileCmd(t *testing.T) {
	if compileCmd == nil {
		t.Fatal("compileCmd should not be nil")
	}
	if compileCmd.Use != "compile <statefile>" {
		t.Errorf("Use = %v, want 'compile <statefile>'", compileCmd.Use)
	}

	// Check flags exist
	flags := []string{"vars", "vars-file", "output"}
	for _, flag := range flags {
		if compileCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestVarsCmd(t *testing.T) {
	if varsCmd == nil {
		t.Fatal("varsCmd should not be nil")
	}
	if varsCmd.Use != "vars" {
		t.Errorf("Use = %v, want 'vars'", varsCmd.Use)
	}

	// Check subcommands exist
	subcommands := []string{"get", "list"}
	for _, expected := range subcommands {
		found := false
		for _, sub := range varsCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found on vars", expected)
		}
	}
}

func TestVarsGetCmd(t *testing.T) {
	if varsGetCmd == nil {
		t.Fatal("varsGetCmd should not be nil")
	}
	if varsGetCmd.Use != "get <key>" {
		t.Errorf("Use = %v, want 'get <key>'", varsGetCmd.Use)
	}

	// Check flags exist
	flags := []string{"scope", "agent", "role"}
	for _, flag := range flags {
		if varsGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestVarsListCmd(t *testing.T) {
	if varsListCmd == nil {
		t.Fatal("varsListCmd should not be nil")
	}
	if varsListCmd.Use != "list" {
		t.Errorf("Use = %v, want 'list'", varsListCmd.Use)
	}

	// Check flags exist
	flags := []string{"scope", "agent", "role"}
	for _, flag := range flags {
		if varsListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestVarsGetExecute_AgentScopeRequiresAgent(t *testing.T) {
	varsGetScope = "agent"
	varsGetAgent = ""
	err := varsGetExecute(varsGetCmd, []string{"http_port"})
	if err == nil {
		t.Error("expected error when scope is agent but --agent is not set")
	}
}

func TestVarsGetExecute_RoleScopeRequiresRole(t *testing.T) {
	varsGetScope = "role"
	varsGetRole = ""
	err := varsGetExecute(varsGetCmd, []string{"http_port"})
	if err == nil {
		t.Error("expected error when scope is role but --role is not set")
	}
}

func TestVarsListExecute_AgentScopeRequiresAgent(t *testing.T) {
	varsListScope = "agent"
	varsListAgent = ""
	err := varsListExecute(varsListCmd, []string{})
	if err == nil {
		t.Error("expected error when scope is agent but --agent is not set")
	}
}

func TestVarsListExecute_RoleScopeRequiresRole(t *testing.T) {
	varsListScope = "role"
	varsListRole = ""
	err := varsListExecute(varsListCmd, []string{})
	if err == nil {
		t.Error("expected error when scope is role but --role is not set")
	}
}

func TestExportCmd(t *testing.T) {
	if exportCmd == nil {
		t.Fatal("exportCmd should not be nil")
	}
	if exportCmd.Use != "export" {
		t.Errorf("Use = %v, want 'export'", exportCmd.Use)
	}

	// Check flags exist
	if exportCmd.Flags().Lookup("output") == nil {
		t.Error("expected flag 'output' not found")
	}
}

func TestRestoreCmd(t *testing.T) {
	if restoreCmd == nil {
		t.Fatal("restoreCmd should not be nil")
	}
	if restoreCmd.Use != "restore" {
		t.Errorf("Use = %v, want 'restore'", restoreCmd.Use)
	}

	// Check flags exist
	if restoreCmd.Flags().Lookup("input") == nil {
		t.Error("expected flag 'input' not found")
	}
}

func TestRestoreExecute_MissingFile(t *testing.T) {
	restoreInput = "/nonexistent/path/state.yaml"
	err := restoreExecute(restoreCmd, []string{})
	if err == nil {
		t.Error("expected error for nonexistent input file")
	}
}

func TestBuildCompiledOutput(t *testing.T) {
	stateFile := &statemgmt.StateFile{
		Metadata: statemgmt.StateMetadata{
			Name:        "test-state",
			Description: "A test state",
			Version:     "1.0",
			Tags:        []string{"test"},
		},
		States: map[string][]statemgmt.StateDeclaration{
			"file": {
				{
					ID:    "/tmp/test.txt",
					State: "present",
					Parameters: map[string]interface{}{
						"contents": "hello",
					},
				},
			},
		},
	}

	vars := statemgmt.NewVars()
	vars.Set("env", "production")
	facts := statemgmt.NewFacts()

	compiled := buildCompiledOutput(stateFile, vars, facts)

	metadata, ok := compiled["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadata in compiled output")
	}
	if metadata["name"] != "test-state" {
		t.Errorf("metadata name = %v, want 'test-state'", metadata["name"])
	}
	if metadata["description"] != "A test state" {
		t.Errorf("metadata description = %v, want 'A test state'", metadata["description"])
	}

	variables, ok := compiled["variables"].(map[string]interface{})
	if !ok {
		t.Fatal("expected variables in compiled output")
	}
	if variables["env"] != "production" {
		t.Errorf("variables env = %v, want 'production'", variables["env"])
	}

	states, ok := compiled["states"].(map[string]interface{})
	if !ok {
		t.Fatal("expected states in compiled output")
	}
	fileStates, ok := states["file"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected file states in compiled output")
	}
	if len(fileStates) != 1 {
		t.Fatalf("expected 1 file state, got %d", len(fileStates))
	}
	if fileStates[0]["id"] != "/tmp/test.txt" {
		t.Errorf("file state id = %v, want '/tmp/test.txt'", fileStates[0]["id"])
	}
}

func TestBuildCompiledOutput_EmptyVars(t *testing.T) {
	stateFile := &statemgmt.StateFile{
		Metadata: statemgmt.StateMetadata{
			Name: "empty-vars-test",
		},
		States: map[string][]statemgmt.StateDeclaration{},
	}

	vars := statemgmt.NewVars()
	facts := &statemgmt.Facts{Data: map[string]interface{}{}}

	compiled := buildCompiledOutput(stateFile, vars, facts)

	if _, ok := compiled["variables"]; ok {
		t.Error("expected no variables key when vars are empty")
	}
	if _, ok := compiled["facts"]; ok {
		t.Error("expected no facts key when facts are empty")
	}
}
