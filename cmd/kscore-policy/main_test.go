// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
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
	if cmd.Use != "kscore-policy" {
		t.Errorf("expected Use to be 'kscore-policy', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "Policy enforcement") {
		t.Errorf("expected Short to contain 'Policy enforcement', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{
		"version", "list", "validate", "check", "show", "audit", "report",
		"eval", "test", "schedule", "remediate", "monitor",
	}
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
	if !strings.Contains(output, "kscore-policy") {
		t.Errorf("expected help output to contain 'kscore-policy', got: %s", output)
	}
	if !strings.Contains(output, "OPA") || !strings.Contains(output, "CEL") {
		t.Errorf("expected help output to mention OPA and CEL, got: %s", output)
	}
}

func TestListCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	listCmd := findSubcommand(cmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	// Check that list flags exist
	categoryFlag := listCmd.Flags().Lookup("category")
	if categoryFlag == nil {
		t.Error("expected --category flag on list command")
	}

	typeFlag := listCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Error("expected --type flag on list command")
	}

	outputFlag := listCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on list command")
	}
	if outputFlag.DefValue != "table" {
		t.Errorf("expected output default to be 'table', got %s", outputFlag.DefValue)
	}
}

func TestValidateCommandExists(t *testing.T) {
	cmd := newRootCmd()
	validateCmd := findSubcommand(cmd, "validate")
	if validateCmd == nil {
		t.Fatal("validate subcommand not found")
	}

	if !strings.Contains(validateCmd.Short, "Validate") {
		t.Errorf("expected Short to contain 'Validate', got %s", validateCmd.Short)
	}

	// Should require exactly 1 argument
	if validateCmd.Args == nil {
		t.Error("expected validate command to have Args validation")
	}
}

func TestCheckCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	checkCmd := findSubcommand(cmd, "check")
	if checkCmd == nil {
		t.Fatal("check subcommand not found")
	}

	// Check required flags
	policyFlag := checkCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on check command")
	}

	inputFileFlag := checkCmd.Flags().Lookup("input-file")
	if inputFileFlag == nil {
		t.Error("expected --input-file flag on check command")
	}

	inputFlag := checkCmd.Flags().Lookup("input")
	if inputFlag == nil {
		t.Error("expected --input flag on check command")
	}

	actionFlag := checkCmd.Flags().Lookup("action")
	if actionFlag == nil {
		t.Fatal("expected --action flag on check command")
	}
	if actionFlag.DefValue != "check" {
		t.Errorf("expected action default to be 'check', got %s", actionFlag.DefValue)
	}

	userFlag := checkCmd.Flags().Lookup("user")
	if userFlag == nil {
		t.Error("expected --user flag on check command")
	}

	contextFlag := checkCmd.Flags().Lookup("context")
	if contextFlag == nil {
		t.Error("expected --context flag on check command")
	}

	outputFlag := checkCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on check command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestShowCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	showCmd := findSubcommand(cmd, "show")
	if showCmd == nil {
		t.Fatal("show subcommand not found")
	}

	outputFlag := showCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on show command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}

	// Should require exactly 2 arguments
	if showCmd.Args == nil {
		t.Error("expected show command to have Args validation")
	}
}

func TestAuditCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	auditCmd := findSubcommand(cmd, "audit")
	if auditCmd == nil {
		t.Fatal("audit subcommand not found")
	}

	policyFlag := auditCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on audit command")
	}

	resourceTypeFlag := auditCmd.Flags().Lookup("resource-type")
	if resourceTypeFlag == nil {
		t.Error("expected --resource-type flag on audit command")
	}

	deniedFlag := auditCmd.Flags().Lookup("denied")
	if deniedFlag == nil {
		t.Error("expected --denied flag on audit command")
	}

	limitFlag := auditCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on audit command")
	}
	if limitFlag.DefValue != "100" {
		t.Errorf("expected limit default to be '100', got %s", limitFlag.DefValue)
	}

	outputFlag := auditCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on audit command")
	}
}

func TestReportCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	reportCmd := findSubcommand(cmd, "report")
	if reportCmd == nil {
		t.Fatal("report subcommand not found")
	}

	daysFlag := reportCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected --days flag on report command")
	}
	if daysFlag.DefValue != "7" {
		t.Errorf("expected days default to be '7', got %s", daysFlag.DefValue)
	}

	outputFlag := reportCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on report command")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("expected output default to be 'text', got %s", outputFlag.DefValue)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"list", "validate", "check", "show", "audit", "report", "eval", "test", "schedule", "remediate", "monitor"}

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

func TestEvalCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	evalCmd := findSubcommand(cmd, "eval")
	if evalCmd == nil {
		t.Fatal("eval subcommand not found")
	}

	if !strings.Contains(evalCmd.Short, "Evaluate") {
		t.Errorf("expected Short to contain 'Evaluate', got %s", evalCmd.Short)
	}

	dryRunFlag := evalCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("expected --dry-run flag on eval command")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("expected dry-run default to be 'false', got %s", dryRunFlag.DefValue)
	}

	verboseFlag := evalCmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("expected --verbose flag on eval command")
	}
	if verboseFlag.DefValue != "false" {
		t.Errorf("expected verbose default to be 'false', got %s", verboseFlag.DefValue)
	}

	// Should require exactly 2 arguments
	if evalCmd.Args == nil {
		t.Error("expected eval command to have Args validation")
	}
}

func TestEvalCommandExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"eval", "security-baseline", "k8s:production"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("eval command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Evaluating policy") {
		t.Errorf("expected output to contain 'Evaluating policy', got: %s", out)
	}
	if !strings.Contains(out, "RULE") && !strings.Contains(out, "RESULT") {
		t.Errorf("expected output to contain table headers, got: %s", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected output to contain PASS result, got: %s", out)
	}
}

func TestEvalCommandDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"eval", "security-baseline", "k8s:production", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("eval --dry-run command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected output to contain '[dry-run]', got: %s", out)
	}
}

func TestEvalCommandVerbose(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"eval", "security-baseline", "k8s:production", "--verbose"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("eval --verbose command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Target:") {
		t.Errorf("expected verbose output to contain 'Target:', got: %s", out)
	}
	if !strings.Contains(out, "Policy:") {
		t.Errorf("expected verbose output to contain 'Policy:', got: %s", out)
	}
}

func TestEvalCommandMissingArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"eval", "only-one-arg"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for eval with only one arg")
	}
}

func TestTestCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	testCmd := findSubcommand(cmd, "test")
	if testCmd == nil {
		t.Fatal("test subcommand not found")
	}

	if !strings.Contains(testCmd.Short, "Test") {
		t.Errorf("expected Short to contain 'Test', got %s", testCmd.Short)
	}

	testDataFlag := testCmd.Flags().Lookup("test-data")
	if testDataFlag == nil {
		t.Error("expected --test-data flag on test command")
	}

	verboseFlag := testCmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("expected --verbose flag on test command")
	}

	// Should require exactly 1 argument
	if testCmd.Args == nil {
		t.Error("expected test command to have Args validation")
	}
}

func TestTestCommandExecution(t *testing.T) {
	// Create a temporary policy file
	dir := t.TempDir()
	policyFilePath := dir + "/test-policies.yaml"
	policyContent := `policies:
  - id: test-policy
    name: Test Policy
    description: A test policy
    type: cel
    category: security
    severity: high
    enforcement_mode: enforce
    enabled: true
    code: "true"
`
	if err := writeTestFile(policyFilePath, policyContent); err != nil {
		t.Fatalf("failed to create test policy file: %v", err)
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"test", policyFilePath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("test command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Testing policies from:") {
		t.Errorf("expected output to contain 'Testing policies from:', got: %s", out)
	}
	if !strings.Contains(out, "test-policy") {
		t.Errorf("expected output to contain 'test-policy', got: %s", out)
	}
	if !strings.Contains(out, "Test Summary") {
		t.Errorf("expected output to contain 'Test Summary', got: %s", out)
	}
}

func TestTestCommandMissingFile(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"test", "/nonexistent/path/policies.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent policy file")
	}
}

func TestScheduleSubcommands(t *testing.T) {
	cmd := newRootCmd()
	scheduleCmd := findSubcommand(cmd, "schedule")
	if scheduleCmd == nil {
		t.Fatal("schedule subcommand not found")
	}

	expectedSubs := []string{"create", "list", "delete"}
	for _, expected := range expectedSubs {
		sub := findSubcommand(scheduleCmd, expected)
		if sub == nil {
			t.Errorf("expected schedule subcommand '%s' not found", expected)
		}
	}
}

func TestScheduleCreateFlags(t *testing.T) {
	cmd := newRootCmd()
	scheduleCmd := findSubcommand(cmd, "schedule")
	if scheduleCmd == nil {
		t.Fatal("schedule subcommand not found")
	}
	createCmd := findSubcommand(scheduleCmd, "create")
	if createCmd == nil {
		t.Fatal("schedule create subcommand not found")
	}

	policyFlag := createCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on schedule create command")
	}

	cronFlag := createCmd.Flags().Lookup("cron")
	if cronFlag == nil {
		t.Error("expected --cron flag on schedule create command")
	}

	targetFlag := createCmd.Flags().Lookup("target")
	if targetFlag == nil {
		t.Error("expected --target flag on schedule create command")
	}
}

func TestScheduleCreateExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"schedule", "create", "--policy", "security-baseline", "--cron", "*/5 * * * *", "--target", "k8s:production"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("schedule create command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Schedule created successfully") {
		t.Errorf("expected output to contain 'Schedule created successfully', got: %s", out)
	}
	if !strings.Contains(out, "security-baseline") {
		t.Errorf("expected output to contain policy name, got: %s", out)
	}
}

func TestScheduleCreateMissingFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing policy", []string{"schedule", "create", "--cron", "* * * * *", "--target", "all"}},
		{"missing cron", []string{"schedule", "create", "--policy", "test", "--target", "all"}},
		{"missing target", []string{"schedule", "create", "--policy", "test", "--cron", "* * * * *"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}

func TestScheduleListExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"schedule", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("schedule list command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Errorf("expected output to contain header 'ID', got: %s", out)
	}
	if !strings.Contains(out, "POLICY") {
		t.Errorf("expected output to contain header 'POLICY', got: %s", out)
	}
	if !strings.Contains(out, "sched-001") {
		t.Errorf("expected output to contain sample schedule, got: %s", out)
	}
}

func TestScheduleDeleteExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"schedule", "delete", "sched-001"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("schedule delete command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sched-001") {
		t.Errorf("expected output to contain schedule ID, got: %s", out)
	}
	if !strings.Contains(out, "deleted successfully") {
		t.Errorf("expected output to contain 'deleted successfully', got: %s", out)
	}
}

func TestScheduleDeleteMissingArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"schedule", "delete"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for schedule delete without arguments")
	}
}

func TestRemediateCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	remediateCmd := findSubcommand(cmd, "remediate")
	if remediateCmd == nil {
		t.Fatal("remediate subcommand not found")
	}

	if !strings.Contains(remediateCmd.Short, "remediate") {
		t.Errorf("expected Short to contain 'remediate', got %s", remediateCmd.Short)
	}

	targetFlag := remediateCmd.Flags().Lookup("target")
	if targetFlag == nil {
		t.Error("expected --target flag on remediate command")
	}

	dryRunFlag := remediateCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("expected --dry-run flag on remediate command")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("expected dry-run default to be 'false', got %s", dryRunFlag.DefValue)
	}

	forceFlag := remediateCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("expected --force flag on remediate command")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("expected force default to be 'false', got %s", forceFlag.DefValue)
	}

	// Should require exactly 1 argument
	if remediateCmd.Args == nil {
		t.Error("expected remediate command to have Args validation")
	}
}

func TestRemediateCommandExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"remediate", "secure-file-permissions"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("remediate command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Scanning for violations") {
		t.Errorf("expected output to contain 'Scanning for violations', got: %s", out)
	}
	if !strings.Contains(out, "remediation(s) applied") {
		t.Errorf("expected output to contain 'remediation(s) applied', got: %s", out)
	}
}

func TestRemediateCommandDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"remediate", "secure-file-permissions", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("remediate --dry-run command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected output to contain '[dry-run]', got: %s", out)
	}
	if !strings.Contains(out, "would be applied") {
		t.Errorf("expected output to contain 'would be applied', got: %s", out)
	}
}

func TestRemediateCommandWithTarget(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"remediate", "secure-file-permissions", "--target", "k8s:production"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("remediate with target command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Target: k8s:production") {
		t.Errorf("expected output to contain 'Target: k8s:production', got: %s", out)
	}
}

func TestRemediateCommandMissingArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"remediate"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for remediate without arguments")
	}
}

func TestMonitorCommandFlags(t *testing.T) {
	cmd := newRootCmd()
	monitorCmd := findSubcommand(cmd, "monitor")
	if monitorCmd == nil {
		t.Fatal("monitor subcommand not found")
	}

	if !strings.Contains(monitorCmd.Short, "monitoring") {
		t.Errorf("expected Short to contain 'monitoring', got %s", monitorCmd.Short)
	}

	policyFlag := monitorCmd.Flags().Lookup("policy")
	if policyFlag == nil {
		t.Error("expected --policy flag on monitor command")
	}

	targetFlag := monitorCmd.Flags().Lookup("target")
	if targetFlag == nil {
		t.Error("expected --target flag on monitor command")
	}
}

func TestMonitorCommandExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"monitor"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("monitor command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Policy Monitor") {
		t.Errorf("expected output to contain 'Policy Monitor', got: %s", out)
	}
	if !strings.Contains(out, "TIMESTAMP") {
		t.Errorf("expected output to contain 'TIMESTAMP' header, got: %s", out)
	}
	if !strings.Contains(out, "Events:") {
		t.Errorf("expected output to contain 'Events:' summary, got: %s", out)
	}
}

func TestMonitorCommandWithPolicies(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"monitor", "--policy", "security-baseline", "--policy", "cis-benchmark"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("monitor command with policies failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "security-baseline, cis-benchmark") {
		t.Errorf("expected output to contain policy names, got: %s", out)
	}
}

func TestMonitorCommandWithTarget(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"monitor", "--target", "k8s:namespace=production"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("monitor command with target failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "k8s:namespace=production") {
		t.Errorf("expected output to contain target, got: %s", out)
	}
}

func TestComplianceReportSubcommand(t *testing.T) {
	cmd := newRootCmd()
	complianceCmd := findSubcommand(cmd, "compliance")
	if complianceCmd == nil {
		t.Fatal("compliance subcommand not found")
	}

	reportCmd := findSubcommand(complianceCmd, "report")
	if reportCmd == nil {
		t.Fatal("compliance report subcommand not found")
	}

	frameworkFlag := reportCmd.Flags().Lookup("framework")
	if frameworkFlag == nil {
		t.Error("expected --framework flag on compliance report command")
	}

	outputFlag := reportCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("expected --output flag on compliance report command")
	}

	formatFlag := reportCmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag on compliance report command")
	}
	if formatFlag.DefValue != "json" {
		t.Errorf("expected format default to be 'json', got %s", formatFlag.DefValue)
	}
}

func TestComplianceReportExecution(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"compliance", "report", "--framework", "cis"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("compliance report command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "cis") {
		t.Errorf("expected output to contain 'cis', got: %s", out)
	}
}

func TestComplianceReportHTMLFormat(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"compliance", "report", "--framework", "soc2", "--format", "html"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("compliance report html command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "SOC2") {
		t.Errorf("expected output to contain 'SOC2', got: %s", out)
	}
	if !strings.Contains(out, "Compliance Rate") {
		t.Errorf("expected output to contain 'Compliance Rate', got: %s", out)
	}
}

func TestComplianceReportInvalidFramework(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"compliance", "report", "--framework", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid framework")
	}
}

func TestComplianceReportInvalidFormat(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"compliance", "report", "--framework", "cis", "--format", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
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
			expectedUse: "kscore-policy",
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

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 4, "h..."},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
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
