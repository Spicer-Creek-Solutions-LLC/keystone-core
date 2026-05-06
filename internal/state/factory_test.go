package state

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewStore_NilConfig(t *testing.T) {
	_, err := NewStore(nil)
	if err == nil {
		t.Fatal("expected error for nil Config")
	}
}

func TestNewStore_InvalidConfig(t *testing.T) {
	_, err := NewStore(&Config{}) // missing Backend
	if err == nil {
		t.Fatal("expected error for invalid Config")
	}
	if !strings.Contains(err.Error(), "Backend is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewStore_SQLiteReturnsConcreteType(t *testing.T) {
	s, err := NewStore(&Config{
		Backend: BackendSQLite,
		SQLite:  SQLiteConfig{Path: filepath.Join(t.TempDir(), "factory.db")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	if _, ok := s.(*SQLiteStore); !ok {
		t.Errorf("got %T, want *SQLiteStore", s)
	}
}

func TestNewStore_PostgreSQLReturnsConcreteType(t *testing.T) {
	s, err := NewStore(&Config{
		Backend:    BackendPostgreSQL,
		PostgreSQL: PostgreSQLConfig{DSN: "postgres://u:p@h/d"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	if _, ok := s.(*PostgreSQLStore); !ok {
		t.Errorf("got %T, want *PostgreSQLStore", s)
	}
}

// PostgreSQLStore is still a stub at task 3 (real impl lands in task 4).
// Sweep every method to verify the contract holds and no half-deleted
// stub panics. The SQLite side is exercised in sqlite_*_test.go.
func TestPostgreSQLStubReturnsErrNotImplemented(t *testing.T) {
	pg, err := NewStore(&Config{
		Backend:    BackendPostgreSQL,
		PostgreSQL: PostgreSQLConfig{DSN: "postgres://u:p@h/d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	ctx := t.Context()
	check := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("%T.%s err = %v, want ErrNotImplemented", pg, name, err)
		}
	}

	check("Ping", pg.Ping(ctx))

	// AgentStore
	check("CreateAgent", pg.CreateAgent(ctx, &AgentRecord{}))
	_, err = pg.GetAgent(ctx, "x")
	check("GetAgent", err)
	_, err = pg.ListAgents(ctx, AgentFilter{})
	check("ListAgents", err)
	check("UpdateAgent", pg.UpdateAgent(ctx, &AgentRecord{}))
	check("UpdateAgentHeartbeat", pg.UpdateAgentHeartbeat(ctx, "x", time.Time{}))
	check("UpdateAgentStatus", pg.UpdateAgentStatus(ctx, "x", AgentStatusConnected))
	check("DeleteAgent", pg.DeleteAgent(ctx, "x"))

	// CommandStore
	check("CreateCommand", pg.CreateCommand(ctx, &CommandRecord{}))
	_, err = pg.GetCommand(ctx, "x")
	check("GetCommand", err)
	_, err = pg.ListCommands(ctx, CommandFilter{})
	check("ListCommands", err)
	check("UpdateCommandResult", pg.UpdateCommandResult(ctx, "x", CommandResult{}))

	// BatchJobStore
	check("CreateBatchJob", pg.CreateBatchJob(ctx, &BatchJobRecord{}))
	_, err = pg.GetBatchJob(ctx, "x")
	check("GetBatchJob", err)
	_, err = pg.ListBatchJobs(ctx, BatchJobFilter{})
	check("ListBatchJobs", err)
	check("UpdateBatchJobCounts", pg.UpdateBatchJobCounts(ctx, "x", 0, 0, 0))
	check("CreateBatchAgentResult", pg.CreateBatchAgentResult(ctx, &BatchAgentResultRecord{}))
	_, err = pg.ListBatchAgentResults(ctx, "x")
	check("ListBatchAgentResults", err)
}
