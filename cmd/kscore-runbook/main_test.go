package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apirunbook "github.com/shawnbutts/keystone-core/pkg/api/runbook"

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

// setupTestServer creates an httptest server that returns canned API responses
// for the runbook REST endpoints. Returns the server and a cleanup function.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/runbooks", func(w http.ResponseWriter, r *http.Request) {
		resp := apirunbook.SummaryList{
			Runbooks: []apirunbook.Summary{
				{Name: "deploy-service", Version: "1.2.0", Description: "Deploy a service", StepCount: 8, Inputs: 3, Labels: map[string]string{"category": "deployment"}},
				{Name: "rotate-credentials", Version: "1.0.3", Description: "Rotate credentials", StepCount: 6, Inputs: 2, Labels: map[string]string{"category": "security"}},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/runbooks/deploy-service/execute", func(w http.ResponseWriter, r *http.Request) {
		resp := apirunbook.ExecuteResponse{
			ExecutionID: "exec-test-123",
			State:       "pending",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	now := time.Now()
	mux.HandleFunc("/api/v1/runbooks/executions/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/runbooks/executions/")
		if id == "" || id == "/" {
			http.Error(w, "not found", 404)
			return
		}
		resp := apirunbook.ExecutionResponse{
			ID:          id,
			RunbookName: "deploy-service",
			State:       "completed",
			StartedAt:   &now,
			CreatedAt:   now,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/runbooks/executions", func(w http.ResponseWriter, r *http.Request) {
		resp := apirunbook.ExecutionListResponse{
			Executions: []apirunbook.ExecutionResponse{
				{ID: "exec-abc", RunbookName: "deploy-service", State: "completed", StartedAt: &now, CreatedAt: now},
				{ID: "exec-def", RunbookName: "rotate-credentials", State: "running", StartedAt: &now, CreatedAt: now},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/runbooks/audit", func(w http.ResponseWriter, r *http.Request) {
		resp := apirunbook.AuditListResponse{
			Events: []apirunbook.AuditEventResponse{
				{ID: "evt-1", Timestamp: now, Type: "execution.started", RunbookName: "deploy-service", Actor: "operator", Outcome: "success"},
				{ID: "evt-2", Timestamp: now, Type: "execution.completed", RunbookName: "deploy-service", Actor: "system", Outcome: "success"},
			},
			Total: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// setServerAddr sets the global serverAddr to point to the test server.
func setServerAddr(t *testing.T, ts *httptest.Server) {
	t.Helper()
	old := serverAddr
	// Strip "http://" prefix since getClient adds it
	serverAddr = strings.TrimPrefix(ts.URL, "http://")
	t.Cleanup(func() { serverAddr = old })
}

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()

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

	output := buf.String()
	if output == "" {
		t.Error("expected version output")
	}
}

func TestApprovalsCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)

	ctx := context.Background()
	now := time.Now()

	req := &approval.Request{
		ID: "req-test-1", ExecutionID: "exec-1", StepName: "approval-step",
		State: approval.RequestStatePending, Title: "Test Approval",
		Approvers: []string{"user1"}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	}
	storage.SaveRequest(ctx, req)
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestApprovalsCommand_Empty(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestApproveCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	currentUser := getCurrentUser()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-approve-1", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Approve Test",
		Approvers: []string{currentUser}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approve", "req-approve-1", "--db", dbPath, "--reason", "Test approval"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

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

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-reject-1", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Reject Test",
		Approvers: []string{currentUser}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"reject", "req-reject-1", "--db", dbPath, "--reason", "Not ready"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

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

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-delegate-1", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Delegate Test",
		Approvers: []string{"user1"}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"delegate", "req-delegate-1", "--db", dbPath, "--to", "@another-user"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = approval.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "req-delegate-1")
	db.Close()

	found := false
	for _, a := range updated.Approvers {
		if a == "@another-user" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected @another-user in approvers: %v", updated.Approvers)
	}
}

func TestInterventionsCommand(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &intervention.Request{
		ID: "int-test-1", ExecutionID: "exec-1", StepName: "prompt-step",
		Type: intervention.TypePrompt, State: intervention.StatePending,
		Title: "Test Prompt", CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"interventions", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestRespondCommand_Confirm(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &intervention.Request{
		ID: "int-confirm-1", ExecutionID: "exec-1", StepName: "confirm-step",
		Type: intervention.TypeConfirm, State: intervention.StatePending,
		Title: "Confirm Test", CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "int-confirm-1", "--db", dbPath, "--confirmed", "--comment", "Looks good"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	db, _ = sql.Open("sqlite", dbPath)
	storage, _ = intervention.NewSQLiteStorage(db)
	updated, _ := storage.GetRequest(ctx, "int-confirm-1")
	db.Close()

	if updated.State != intervention.StateCompleted {
		t.Errorf("State = %q, want %q", updated.State, intervention.StateCompleted)
	}
}

func TestRespondCommand_WithTextValues(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := intervention.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &intervention.Request{
		ID: "int-prompt-1", ExecutionID: "exec-1", StepName: "prompt-step",
		Type: intervention.TypePrompt, State: intervention.StatePending,
		Title: "Prompt Test",
		Prompts: []intervention.PromptField{
			{Name: "version", Type: intervention.FieldTypeText},
			{Name: "message", Type: intervention.FieldTypeText},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "int-prompt-1", "--db", dbPath, "--value", "version=1.0.0", "--value", "message=hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

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
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestRespondCommand_NotFound(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"respond", "nonexistent", "--db", dbPath, "--confirmed"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestRejectCommand_RequiresReason(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"reject", "req-123", "--db", dbPath})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when reason not provided")
	}
}

func TestDelegateCommand_RequiresTo(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"delegate", "req-123", "--db", dbPath})
	if err := cmd.Execute(); err == nil {
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
// List Command Tests (wired to REST)
// ============================================================================

func TestListCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"list"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected 'deploy-service' in list output")
	}
	if !strings.Contains(out, "Total:") {
		t.Error("expected 'Total:' in list output")
	}
}

func TestListCommand_JSONOutput(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"list", "-o", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestListCommand_FilterByTag(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"list", "--tag", "security"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "rotate-credentials") {
		t.Error("expected 'rotate-credentials' with security label")
	}
}

// ============================================================================
// Execute Command Tests (wired to REST)
// ============================================================================

func TestExecuteCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "version=1.2.0"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Execution started:") {
		t.Error("expected 'Execution started:' in output")
	}
}

func TestExecuteCommand_DryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "version=1.2.0", "--dry-run"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dry run:") {
		t.Error("expected 'Dry run:' in output")
	}
	if !strings.Contains(out, "No changes made") {
		t.Error("expected 'No changes made' in dry run output")
	}
}

func TestExecuteCommand_InvalidVar(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"execute", "deploy-service", "--var", "badformat"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for invalid variable format")
	}
}

func TestExecuteCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"execute"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when no args provided")
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

// ============================================================================
// Status Command Tests (wired to REST)
// ============================================================================

func TestStatusCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"status", "exec-abc"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "exec-abc") {
		t.Error("expected execution ID in output")
	}
	if !strings.Contains(out, "deploy-service") {
		t.Error("expected runbook name in output")
	}
}

func TestStatusCommand_JSONOutput(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"status", "exec-abc", "-o", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestStatusCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when no args provided")
	}
}

// ============================================================================
// List Executions Command Tests (wired to REST)
// ============================================================================

func TestListExecutionsCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"list-executions"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "exec-abc") {
		t.Error("expected execution ID in output")
	}
	if !strings.Contains(out, "Total:") {
		t.Error("expected 'Total:' in output")
	}
}

func TestListExecutionsCommand_JSONOutput(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"list-executions", "-o", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
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

// ============================================================================
// Audit Command Tests (wired to REST)
// ============================================================================

func TestAuditShowCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "show", "deploy-service"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Audit trail for: deploy-service") {
		t.Error("expected audit header in output")
	}
}

func TestAuditShowCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "show"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestAuditListCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "list"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Runbook audit events") {
		t.Error("expected 'Runbook audit events' header")
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

func TestAuditReportCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "report"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Compliance Report") {
		t.Error("expected 'Compliance Report' header")
	}
	if !strings.Contains(out, "Summary:") {
		t.Error("expected 'Summary:' section")
	}
}

func TestAuditReportCommand_CSVFormat(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "report", "--format", "csv"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "timestamp,runbook,actor,type,outcome") {
		t.Error("expected CSV header row")
	}
}

func TestAuditReportCommand_DetailedFormat(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "report", "--format", "detailed"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "All Events:") {
		t.Error("expected 'All Events:' section in detailed format")
	}
}

func TestAuditReportCommand_InvalidFormat(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"audit", "report", "--format", "invalid"})
	if err := cmd.Execute(); err == nil {
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
// Test Command Tests (wired to REST)
// ============================================================================

func TestTestCommand(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"test", "deploy-service"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Testing runbook: deploy-service") {
		t.Error("expected test header in output")
	}
	if !strings.Contains(out, "[PASS]") {
		t.Error("expected PASS results in output")
	}
}

func TestTestCommand_NotFound(t *testing.T) {
	ts := setupTestServer(t)
	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"test", "nonexistent-runbook"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestTestCommand_WithMockFile(t *testing.T) {
	ts := setupTestServer(t)

	tmpDir := t.TempDir()
	mockFile := filepath.Join(tmpDir, "mocks.json")
	if err := os.WriteFile(mockFile, []byte(`[{"step":"deploy","response":{"status":"ok"}}]`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	setServerAddr(t, ts)
	cmd.SetArgs([]string{"test", "deploy-service", "--mock-file", mockFile, "--verbose"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Mock handler validation") {
		t.Error("expected 'Mock handler validation' in output")
	}
}

func TestTestCommand_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"test"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestTestCommand_MockFileFlagExists(t *testing.T) {
	cmd := newRootCmd()
	testCmd, _, err := cmd.Find([]string{"test"})
	if err != nil {
		t.Fatalf("Find test command: %v", err)
	}
	if testCmd.Flags().Lookup("mock-file") == nil {
		t.Fatal("expected --mock-file flag")
	}
}

// ============================================================================
// Helper Tests
// ============================================================================

func TestMatchesLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		tags   []string
		want   bool
	}{
		{"match by key", map[string]string{"security": "true"}, []string{"security"}, true},
		{"match by value", map[string]string{"category": "security"}, []string{"security"}, true},
		{"no match", map[string]string{"category": "deployment"}, []string{"security"}, false},
		{"empty filter", map[string]string{"security": "true"}, []string{}, false},
		{"nil labels", nil, []string{"security"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesLabels(tt.labels, tt.tags)
			if got != tt.want {
				t.Errorf("matchesLabels(%v, %v) = %v, want %v", tt.labels, tt.tags, got, tt.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:9090")
	if c.baseURL != "http://localhost:9090" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:9090")
	}
	if c.httpClient == nil {
		t.Error("expected non-nil httpClient")
	}
}

func TestRootCommand_IncludesNewCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	for _, subcmd := range []string{"list", "execute", "status", "list-executions", "audit", "test"} {
		if !strings.Contains(out, subcmd) {
			t.Errorf("expected %q command in help output", subcmd)
		}
	}
}

func TestApprovalsCommand_FilterByState(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, _ := sql.Open("sqlite", dbPath)
	storage, _ := approval.NewSQLiteStorage(db)
	ctx := context.Background()
	now := time.Now()

	storage.SaveRequest(ctx, &approval.Request{
		ID: "req-pending", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Pending",
		Approvers: []string{"user1"}, Mode: approval.ModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	db.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"approvals", "--db", dbPath, "--state", "pending"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}
