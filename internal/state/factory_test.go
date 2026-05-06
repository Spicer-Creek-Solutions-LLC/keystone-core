package state

import (
	"errors"
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
	s, err := NewStore(&Config{Backend: BackendSQLite})
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

// Stubs return ErrNotImplemented today; tasks 3/4 replace with real logic.
// Exercise every method on both backends so a regression — e.g., a panic
// from a half-deleted stub — gets caught immediately.
func TestStubsReturnErrNotImplemented(t *testing.T) {
	sqlite, err := NewStore(&Config{Backend: BackendSQLite})
	if err != nil {
		t.Fatal(err)
	}
	pg, err := NewStore(&Config{
		Backend:    BackendPostgreSQL,
		PostgreSQL: PostgreSQLConfig{DSN: "postgres://u:p@h/d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	defer pg.Close()

	for _, s := range []Store{sqlite, pg} {
		ctx := t.Context()

		check := func(name string, err error) {
			t.Helper()
			if !errors.Is(err, ErrNotImplemented) {
				t.Errorf("%T.%s err = %v, want ErrNotImplemented", s, name, err)
			}
		}

		check("Ping", s.Ping(ctx))

		// AgentStore
		check("CreateAgent", s.CreateAgent(ctx, &AgentRecord{}))
		_, err := s.GetAgent(ctx, "x")
		check("GetAgent", err)
		_, err = s.ListAgents(ctx, AgentFilter{})
		check("ListAgents", err)
		check("UpdateAgent", s.UpdateAgent(ctx, &AgentRecord{}))
		check("UpdateAgentHeartbeat", s.UpdateAgentHeartbeat(ctx, "x", time.Time{}))
		check("UpdateAgentStatus", s.UpdateAgentStatus(ctx, "x", AgentStatusConnected))
		check("DeleteAgent", s.DeleteAgent(ctx, "x"))

		// CommandStore
		check("CreateCommand", s.CreateCommand(ctx, &CommandRecord{}))
		_, err = s.GetCommand(ctx, "x")
		check("GetCommand", err)
		_, err = s.ListCommands(ctx, CommandFilter{})
		check("ListCommands", err)
		check("UpdateCommandResult", s.UpdateCommandResult(ctx, "x", CommandResult{}))

		// BatchJobStore
		check("CreateBatchJob", s.CreateBatchJob(ctx, &BatchJobRecord{}))
		_, err = s.GetBatchJob(ctx, "x")
		check("GetBatchJob", err)
		_, err = s.ListBatchJobs(ctx, BatchJobFilter{})
		check("ListBatchJobs", err)
		check("UpdateBatchJobCounts", s.UpdateBatchJobCounts(ctx, "x", 0, 0, 0))
		check("CreateBatchAgentResult", s.CreateBatchAgentResult(ctx, &BatchAgentResultRecord{}))
		_, err = s.ListBatchAgentResults(ctx, "x")
		check("ListBatchAgentResults", err)
	}
}
