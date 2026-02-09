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
