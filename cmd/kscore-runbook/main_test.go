package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Initialize both schemas
	approvalStorage, err := approval.NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("init approval storage: %v", err)
	}
	_ = approvalStorage

	interventionStorage, err := intervention.NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("init intervention storage: %v", err)
	}
	_ = interventionStorage

	db.Close()

	return dbPath, func() {
		os.RemoveAll(tmpDir)
	}
}

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()

	// Test help output
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "kscore-runbook") {
		t.Error("expected 'kscore-runbook' in help output")
	}
	if !strings.Contains(output, "approvals") {
		t.Error("expected 'approvals' command in help output")
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"version"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Should contain version info
	output := buf.String()
	if output == "" {
		t.Error("expected version output")
	}
}

func TestApprovalsCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Add some test approval requests
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &approval.Request{
		ID:          "req-test-1",
		ExecutionID: "exec-1",
		StepName:    "approval-step",
		State:       approval.RequestStatePending,
		Title:       "Test Approval",
		Approvers:   []string{"user1"},
		Mode:        approval.ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	// Run the command - should not error
	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestApprovalsCommand_Empty(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestApproveCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Get current user to add to approvers
	currentUser := getCurrentUser()

	// Add a pending request with current user as approver
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &approval.Request{
		ID:          "req-approve-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Approve Test",
		Approvers:   []string{currentUser}, // Use current user
		Mode:        approval.ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approve", "req-approve-1", "--db", dbPath, "--reason", "Test approval"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the request was approved
	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = approval.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "req-approve-1")
	db.Close()

	if updated.State != approval.RequestStateApproved {
		t.Errorf("State = %q, want %q", updated.State, approval.RequestStateApproved)
	}
}

func TestRejectCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	currentUser := getCurrentUser()

	// Add a pending request
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &approval.Request{
		ID:          "req-reject-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Reject Test",
		Approvers:   []string{currentUser},
		Mode:        approval.ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"reject", "req-reject-1", "--db", dbPath, "--reason", "Not ready"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the request was rejected
	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = approval.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "req-reject-1")
	db.Close()

	if updated.State != approval.RequestStateRejected {
		t.Errorf("State = %q, want %q", updated.State, approval.RequestStateRejected)
	}
}

func TestDelegateCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Add a pending request
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &approval.Request{
		ID:          "req-delegate-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Delegate Test",
		Approvers:   []string{"user1"},
		Mode:        approval.ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"delegate", "req-delegate-1", "--db", dbPath, "--to", "@another-user"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the delegation
	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = approval.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "req-delegate-1")
	db.Close()

	found := false
	for _, a := range updated.Approvers {
		if a == "@another-user" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected @another-user in approvers: %v", updated.Approvers)
	}
}

func TestInterventionsCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Add a test intervention request
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &intervention.Request{
		ID:          "int-test-1",
		ExecutionID: "exec-1",
		StepName:    "prompt-step",
		Type:        intervention.TypePrompt,
		State:       intervention.StatePending,
		Title:       "Test Prompt",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"interventions", "--db", dbPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestRespondCommand_Confirm(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Add a pending intervention
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &intervention.Request{
		ID:          "int-confirm-1",
		ExecutionID: "exec-1",
		StepName:    "confirm-step",
		Type:        intervention.TypeConfirm,
		State:       intervention.StatePending,
		Title:       "Confirm Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "int-confirm-1", "--db", dbPath, "--confirmed", "--comment", "Looks good"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the response
	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = intervention.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "int-confirm-1")
	db.Close()

	if updated.State != intervention.StateCompleted {
		t.Errorf("State = %q, want %q", updated.State, intervention.StateCompleted)
	}
	if !updated.Response.Confirmed {
		t.Error("expected Confirmed = true")
	}
}

func TestRespondCommand_WithTextValues(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// Add a pending prompt intervention with text fields only
	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &intervention.Request{
		ID:          "int-prompt-1",
		ExecutionID: "exec-1",
		StepName:    "prompt-step",
		Type:        intervention.TypePrompt,
		State:       intervention.StatePending,
		Title:       "Prompt Test",
		Prompts: []intervention.PromptField{
			{Name: "version", Type: intervention.FieldTypeText},
			{Name: "message", Type: intervention.FieldTypeText},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "int-prompt-1", "--db", dbPath, "--value", "version=1.0.0", "--value", "message=hello"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the values
	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = intervention.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "int-prompt-1")
	db.Close()

	if updated.Response.Values["version"] != "1.0.0" {
		t.Errorf("Values[version] = %v, want %q", updated.Response.Values["version"], "1.0.0")
	}
}

func TestApproveCommand_NotFound(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approve", "nonexistent", "--db", dbPath, "--reason", "test"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestRespondCommand_NotFound(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "nonexistent", "--db", dbPath, "--confirmed"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestRejectCommand_RequiresReason(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"reject", "req-123", "--db", dbPath})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when reason not provided")
	}
}

func TestDelegateCommand_RequiresTo(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"delegate", "req-123", "--db", dbPath})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --to not provided")
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("truncate", func(t *testing.T) {
		if got := truncate("short", 10); got != "short" {
			t.Errorf("truncate = %q, want %q", got, "short")
		}
		if got := truncate("this is a long string", 10); got != "this is..." {
			t.Errorf("truncate = %q, want %q", got, "this is...")
		}
	})

	t.Run("getCurrentUser", func(t *testing.T) {
		user := getCurrentUser()
		if user == "" {
			t.Error("expected non-empty user")
		}
	})
}

func TestApprovalsCommand_FilterByState(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	// Create pending and approved requests
	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-pending", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Pending",
		Approvers: []string{"user1"}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-approved", ExecutionID: "exec-2", StepName: "step1",
		State: approval.RequestStateApproved, Title: "Approved",
		Approvers: []string{"user1"}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	// Filter by pending state
	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath, "--state", "pending"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestInterventionsCommand_FilterByExecution(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &intervention.Request{
		ID: "int-1", ExecutionID: "exec-1", StepName: "step1",
		Type: intervention.TypeConfirm, State: intervention.StatePending,
		Title: "Test 1", CreatedAt: now, UpdatedAt: now,
	})
	storage.SaveRequest(ctx, &intervention.Request{
		ID: "int-2", ExecutionID: "exec-2", StepName: "step1",
		Type: intervention.TypeConfirm, State: intervention.StatePending,
		Title: "Test 2", CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"interventions", "--db", dbPath, "--execution", "exec-1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

// ============================================================================
// List Command Tests
// ============================================================================

func TestListCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected 'deploy-service' in list output")
	}
	if !strings.Contains(out, "rotate-credentials") {
		t.Error("expected 'rotate-credentials' in list output")
	}
	if !strings.Contains(out, "Total:") {
		t.Error("expected 'Total:' in list output")
	}
}

func TestListCommand_FilterByTag(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "--tag", "security"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "rotate-credentials") {
		t.Error("expected 'rotate-credentials' in filtered output")
	}
	if !strings.Contains(out, "security-scan") {
		t.Error("expected 'security-scan' in filtered output")
	}
}

func TestListCommand_FilterByTagNoMatch(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "--tag", "nonexistent"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No runbooks found") {
		t.Error("expected 'No runbooks found' in output")
	}
}

func TestListCommand_JSONOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "-o", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestListCommand_YAMLOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "-o", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestListCommand_Limit(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "--limit", "2"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 'Total: 2' in output, got: %s", out)
	}
}

// ============================================================================
// Execute Command Tests
// ============================================================================

func TestExecuteCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "version=1.2.0"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Execution started:") {
		t.Error("expected 'Execution started:' in output")
	}
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected 'deploy-service' in output")
	}
	if !strings.Contains(out, "version = 1.2.0") {
		t.Error("expected variable in output")
	}
}

func TestExecuteCommand_DryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "version=1.2.0", "--dry-run"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dry run:") {
		t.Error("expected 'Dry run:' in output")
	}
	if !strings.Contains(out, "Steps that would execute:") {
		t.Error("expected step list in dry run output")
	}
	if !strings.Contains(out, "No changes made") {
		t.Error("expected 'No changes made' in dry run output")
	}
}

func TestExecuteCommand_NotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "nonexistent-runbook"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestExecuteCommand_InvalidVar(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "badformat"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid variable format")
	}
}

func TestExecuteCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestExecuteCommand_InputFlag(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--input", "version=1.2.0", "--input", "env=prod"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute with --input: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Execution started:") {
		t.Error("expected 'Execution started:' in output")
	}
	if !strings.Contains(out, "version = 1.2.0") {
		t.Error("expected 'version' variable in output")
	}
	if !strings.Contains(out, "env = prod") {
		t.Error("expected 'env' variable in output")
	}
}

func TestExecuteCommand_InputFlagExists(t *testing.T) {
	cmd := newRootCmd()
	execCmd, _, err := cmd.Find([]string{"execute"})
	if err != nil {
		t.Fatalf("could not find execute command: %v", err)
	}
	if execCmd.Flags().Lookup("input") == nil {
		t.Error("expected --input flag on execute command")
	}
}

func TestExecuteCommand_MixedVarAndInput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "version=1.2.0", "--input", "env=prod"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute with mixed --var and --input: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "version = 1.2.0") {
		t.Error("expected --var value in output")
	}
	if !strings.Contains(out, "env = prod") {
		t.Error("expected --input value in output")
	}
}

func TestExecuteCommand_InputDryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--input", "version=2.0.0", "--dry-run"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute --input --dry-run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dry run:") {
		t.Error("expected 'Dry run:' in output")
	}
	if !strings.Contains(out, "version = 2.0.0") {
		t.Error("expected --input variable in dry run output")
	}
}

// ============================================================================
// Status Command Tests
// ============================================================================

func TestStatusCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status", "exec-a1b2c3"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "exec-a1b2c3") {
		t.Error("expected execution ID in output")
	}
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected runbook name in output")
	}
	if !strings.Contains(out, "completed") {
		t.Error("expected state in output")
	}
}

func TestStatusCommand_Running(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status", "exec-d4e5f6"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "running") {
		t.Error("expected 'running' state in output")
	}
	if !strings.Contains(out, "health-check") {
		t.Error("expected current step in output")
	}
}

func TestStatusCommand_NotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status", "exec-nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent execution")
	}
}

func TestStatusCommand_JSONOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status", "exec-a1b2c3", "-o", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestStatusCommand_YAMLOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status", "exec-a1b2c3", "-o", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestStatusCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no args provided")
	}
}

// ============================================================================
// List Executions Command Tests
// ============================================================================

func TestListExecutionsCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "exec-a1b2c3") {
		t.Error("expected execution ID in output")
	}
	if !strings.Contains(out, "Total:") {
		t.Error("expected 'Total:' in output")
	}
}

func TestListExecutionsCommand_FilterByRunbook(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "--runbook", "deploy-service"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected 'deploy-service' in output")
	}
	if strings.Contains(out, "rotate-credentials") {
		t.Error("should not contain 'rotate-credentials' when filtering")
	}
}

func TestListExecutionsCommand_FilterByState(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "--state", "completed"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "completed") {
		t.Error("expected 'completed' in output")
	}
	if strings.Contains(out, "running") {
		t.Error("should not contain 'running' when filtering by completed")
	}
}

func TestListExecutionsCommand_FilterNoMatch(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "--runbook", "nonexistent"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No executions found") {
		t.Error("expected 'No executions found' in output")
	}
}

func TestListExecutionsCommand_JSONOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "-o", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestListExecutionsCommand_Limit(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "--limit", "2"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 'Total: 2' in output, got: %s", out)
	}
}

func TestListExecutionsCommand_SinceFlagExists(t *testing.T) {
	cmd := newRootCmd()
	listExecCmd, _, err := cmd.Find([]string{"list-executions"})
	if err != nil {
		t.Fatalf("could not find list-executions command: %v", err)
	}
	if listExecCmd.Flags().Lookup("since") == nil {
		t.Error("expected --since flag on list-executions command")
	}
}

func TestListExecutionsCommand_SinceFilters(t *testing.T) {
	// Sample data has dates in Jan 2025. A --since of "2025-01-14" should
	// exclude entries before that date.
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list-executions", "--since", "2025-01-14"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	// exec-j1k2l3 started 2025-01-13, should be excluded
	if strings.Contains(out, "exec-j1k2l3") {
		t.Error("expected exec-j1k2l3 (2025-01-13) to be filtered out by --since 2025-01-14")
	}
	// exec-a1b2c3 started 2025-01-15, should be included
	if !strings.Contains(out, "exec-a1b2c3") {
		t.Error("expected exec-a1b2c3 (2025-01-15) to be included")
	}
}

func TestParseSinceValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"days shorthand", "7d", false},
		{"hours duration", "24h", false},
		{"date format", "2026-01-01", false},
		{"minutes duration", "30m", false},
		{"invalid", "foobar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSinceValue(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSinceValue(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// Audit Command Tests
// ============================================================================

func TestAuditShowCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show", "deploy-service"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Audit trail for: deploy-service") {
		t.Error("expected audit header in output")
	}
	if !strings.Contains(out, "execute") {
		t.Error("expected 'execute' action in output")
	}
	if !strings.Contains(out, "approve") {
		t.Error("expected 'approve' action in output")
	}
}

func TestAuditShowCommand_NotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show", "nonexistent-runbook"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestAuditShowCommand_JSONOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show", "deploy-service", "-o", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestAuditShowCommand_YAMLOutput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show", "deploy-service", "-o", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestAuditShowCommand_Limit(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show", "deploy-service", "--limit", "2"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 'Total: 2' in output, got: %s", out)
	}
}

func TestAuditShowCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestAuditListCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "list"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Runbook audit events") {
		t.Error("expected 'Runbook audit events' header")
	}
	if !strings.Contains(out, "RUNBOOK") {
		t.Error("expected RUNBOOK column header")
	}
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected deploy-service in output")
	}
	if !strings.Contains(out, "rotate-credentials") {
		t.Error("expected rotate-credentials in output")
	}
}

func TestAuditListCommand_FilterByRunbook(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "list", "--runbook", "deploy-service"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected deploy-service in output")
	}
	if strings.Contains(out, "rotate-credentials") {
		t.Error("should not contain rotate-credentials when filtered")
	}
}

func TestAuditListCommand_FilterByRunbook_NotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "list", "--runbook", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestAuditListCommand_Flags(t *testing.T) {
	cmd := newRootCmd()
	auditCmd, _, err := cmd.Find([]string{"audit", "list"})
	if err != nil {
		t.Fatalf("Find audit list: %v", err)
	}

	for _, flag := range []string{"runbook", "start", "end", "limit"} {
		if auditCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag", flag)
		}
	}
}

func TestAuditListCommand_Limit(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "list", "--limit", "3"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total: 3") {
		t.Errorf("expected 'Total: 3' in output, got: %s", out)
	}
}

func TestGenerateAllAuditEntries(t *testing.T) {
	entries := generateAllAuditEntries()
	if len(entries) == 0 {
		t.Fatal("expected audit entries")
	}
	for _, e := range entries {
		if e.Runbook == "" {
			t.Error("expected Runbook field to be set")
		}
	}
	// Should have entries for multiple runbooks
	runbooks := make(map[string]bool)
	for _, e := range entries {
		runbooks[e.Runbook] = true
	}
	if len(runbooks) < 2 {
		t.Errorf("expected entries from multiple runbooks, got %d", len(runbooks))
	}
}

func TestAuditReportCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "report"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Compliance Report") {
		t.Error("expected 'Compliance Report' header")
	}
	if !strings.Contains(out, "Summary:") {
		t.Error("expected 'Summary:' section")
	}
	if !strings.Contains(out, "Total events:") {
		t.Error("expected 'Total events:' in summary")
	}
	if !strings.Contains(out, "By Runbook:") {
		t.Error("expected 'By Runbook:' section")
	}
	if !strings.Contains(out, "By User:") {
		t.Error("expected 'By User:' section")
	}
}

func TestAuditReportCommand_DetailedFormat(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "report", "--format", "detailed"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Compliance Report") {
		t.Error("expected 'Compliance Report' header")
	}
	if !strings.Contains(out, "All Events:") {
		t.Error("expected 'All Events:' section in detailed format")
	}
	if !strings.Contains(out, "TIMESTAMP") {
		t.Error("expected column headers in detailed format")
	}
}

func TestAuditReportCommand_CSVFormat(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "report", "--format", "csv"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "timestamp,runbook,user,action,details") {
		t.Error("expected CSV header row")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least header + 1 data row, got %d lines", len(lines))
	}
}

func TestAuditReportCommand_FilterByRunbook(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "report", "--runbook", "deploy-service"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy-service:") {
		t.Error("expected deploy-service in report")
	}
	if strings.Contains(out, "rotate-credentials:") {
		t.Error("should not contain rotate-credentials when filtered")
	}
}

func TestAuditReportCommand_InvalidFormat(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "report", "--format", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestAuditReportCommand_Flags(t *testing.T) {
	cmd := newRootCmd()
	reportCmd, _, err := cmd.Find([]string{"audit", "report"})
	if err != nil {
		t.Fatalf("Find audit report: %v", err)
	}

	for _, flag := range []string{"format", "start", "end", "runbook"} {
		if reportCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag", flag)
		}
	}
}

// ============================================================================
// Test Command Tests
// ============================================================================

func TestTestCommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Testing runbook: deploy-service") {
		t.Error("expected test header in output")
	}
	if !strings.Contains(out, "[PASS]") {
		t.Error("expected PASS results in output")
	}
	if !strings.Contains(out, "All tests passed") {
		t.Error("expected 'All tests passed' in output")
	}
}

func TestTestCommand_WithVars(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--var", "version=1.2.0"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "All tests passed") {
		t.Error("expected 'All tests passed' in output")
	}
}

func TestTestCommand_Verbose(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--verbose"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Runbook YAML is valid") {
		t.Error("expected verbose detail 'Runbook YAML is valid' in output")
	}
	if !strings.Contains(out, "Required permissions") {
		t.Error("expected verbose detail about permissions in output")
	}
}

func TestTestCommand_NotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "nonexistent-runbook"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestTestCommand_InvalidVar(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--var", "badformat"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid variable format")
	}
}

func TestTestCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestTestCommand_MockFileFlagExists(t *testing.T) {
	cmd := newRootCmd()
	testCmd, _, err := cmd.Find([]string{"test"})
	if err != nil {
		t.Fatalf("Find test command: %v", err)
	}

	f := testCmd.Flags().Lookup("mock-file")
	if f == nil {
		t.Fatal("expected --mock-file flag")
	}
	if f.DefValue != "" {
		t.Errorf("expected empty default, got %q", f.DefValue)
	}
}

func TestTestCommand_WithMockFile(t *testing.T) {
	tmpDir := t.TempDir()
	mockFile := filepath.Join(tmpDir, "mocks.json")
	if err := os.WriteFile(mockFile, []byte(`[{"step":"deploy","response":{"status":"ok"}}]`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--mock-file", mockFile, "--verbose"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Mock handler validation") {
		t.Error("expected 'Mock handler validation' in output")
	}
	if !strings.Contains(out, "1 mock handler(s)") {
		t.Error("expected mock handler count in output")
	}
	if !strings.Contains(out, "[PASS]") {
		t.Error("expected PASS for mock validation")
	}
}

func TestTestCommand_MockFileNotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--mock-file", "/nonexistent/mocks.json"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent mock file")
	}
}

func TestTestCommand_MockFileInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	mockFile := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(mockFile, []byte(`not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"test", "deploy-service", "--mock-file", mockFile, "--verbose"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid JSON mock file")
	}

	out := buf.String()
	if !strings.Contains(out, "[FAIL] Mock handler validation") {
		t.Error("expected FAIL for mock validation with invalid JSON")
	}
}

// ============================================================================
// Sample Data Generator Tests
// ============================================================================

func TestGenerateSampleRunbooks(t *testing.T) {
	runbooks := generateSampleRunbooks()
	if len(runbooks) < 4 {
		t.Errorf("expected at least 4 sample runbooks, got %d", len(runbooks))
	}

	for _, rb := range runbooks {
		if rb.Name == "" {
			t.Error("runbook name should not be empty")
		}
		if rb.Description == "" {
			t.Error("runbook description should not be empty")
		}
		if rb.Version == "" {
			t.Error("runbook version should not be empty")
		}
		if len(rb.Tags) == 0 {
			t.Errorf("runbook %s should have at least one tag", rb.Name)
		}
		if rb.StepCount == 0 {
			t.Errorf("runbook %s should have at least one step", rb.Name)
		}
	}
}

func TestGenerateSampleExecutions(t *testing.T) {
	executions := generateSampleExecutions()
	if len(executions) < 4 {
		t.Errorf("expected at least 4 sample executions, got %d", len(executions))
	}

	for _, e := range executions {
		if e.ID == "" {
			t.Error("execution ID should not be empty")
		}
		if e.Runbook == "" {
			t.Error("execution runbook should not be empty")
		}
		if e.State == "" {
			t.Error("execution state should not be empty")
		}
		if e.StartedBy == "" {
			t.Error("execution started_by should not be empty")
		}
	}
}

func TestFindRunbook(t *testing.T) {
	rb := findRunbook("deploy-service")
	if rb == nil {
		t.Fatal("expected to find deploy-service")
	}
	if rb.Name != "deploy-service" {
		t.Errorf("Name = %q, want %q", rb.Name, "deploy-service")
	}

	rb = findRunbook("nonexistent")
	if rb != nil {
		t.Error("expected nil for nonexistent runbook")
	}
}

func TestMatchesTags(t *testing.T) {
	tests := []struct {
		name        string
		runbookTags []string
		filterTags  []string
		want        bool
	}{
		{"match single", []string{"security", "compliance"}, []string{"security"}, true},
		{"match multiple", []string{"deployment", "production"}, []string{"deployment", "production"}, true},
		{"no match", []string{"deployment"}, []string{"security"}, false},
		{"partial match", []string{"security", "compliance"}, []string{"security", "other"}, true},
		{"empty filter", []string{"security"}, []string{}, false},
		{"empty runbook tags", []string{}, []string{"security"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesTags(tt.runbookTags, tt.filterTags)
			if got != tt.want {
				t.Errorf("matchesTags(%v, %v) = %v, want %v", tt.runbookTags, tt.filterTags, got, tt.want)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	if id1 == "" {
		t.Error("expected non-empty ID")
	}

	id2 := generateID()
	if id1 == id2 {
		t.Log("IDs may collide if generated at same nanosecond; this is acceptable for demo data")
	}
}

func TestRootCommand_IncludesNewCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	for _, subcmd := range []string{"list", "execute", "status", "list-executions", "audit", "test"} {
		if !strings.Contains(out, subcmd) {
			t.Errorf("expected %q command in help output", subcmd)
		}
	}
}
