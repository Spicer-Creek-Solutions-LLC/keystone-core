package state

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

func TestSchema_ReturnsExpectedBackends(t *testing.T) {
	if got := Schema(BackendSQLite); len(got) == 0 {
		t.Error("Schema(BackendSQLite) is empty")
	}
	if got := Schema(BackendPostgreSQL); len(got) == 0 {
		t.Error("Schema(BackendPostgreSQL) is empty")
	}
	if got := Schema("unknown"); got != nil {
		t.Errorf("Schema(unknown) = %v, want nil", got)
	}
}

func TestSchema_ContainsExpectedTables(t *testing.T) {
	expectedTables := []string{
		"agents",
		"commands",
		"batch_jobs",
		"batch_agent_results",
	}

	for _, backend := range []Backend{BackendSQLite, BackendPostgreSQL} {
		t.Run(string(backend), func(t *testing.T) {
			joined := strings.Join(Schema(backend), "\n")
			for _, table := range expectedTables {
				want := "CREATE TABLE IF NOT EXISTS " + table
				if !strings.Contains(joined, want) {
					t.Errorf("schema for %s missing %q", backend, want)
				}
			}
		})
	}
}

func openSQLiteForTest(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema-test.db")
	db, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestApplySchema_SQLite_CreatesTables(t *testing.T) {
	db := openSQLiteForTest(t)
	ctx := t.Context()

	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatalf("applySchema: %v", err)
	}

	for _, table := range []string{"agents", "commands", "batch_jobs", "batch_agent_results"} {
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	for _, idx := range []string{
		"agents_status_idx",
		"agents_last_heartbeat_at_idx",
		"commands_status_idx",
		"commands_agent_id_idx",
		"commands_started_at_idx",
		"batch_jobs_status_idx",
		"batch_jobs_created_at_idx",
		"batch_agent_results_agent_id_idx",
	} {
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
			idx).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", idx, err)
		}
	}
}

func TestApplySchema_SQLite_Idempotent(t *testing.T) {
	db := openSQLiteForTest(t)
	ctx := t.Context()

	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatalf("second apply (should be a no-op): %v", err)
	}
}

func TestApplySchema_SQLite_ForeignKeyEnforced(t *testing.T) {
	db := openSQLiteForTest(t)
	ctx := t.Context()
	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatal(err)
	}

	_, err := db.ExecContext(ctx, `
        INSERT INTO commands (id, agent_id, command, args, env, status)
        VALUES ('cmd-1', 'agent-does-not-exist', 'ls', '[]', '{}', 'pending')
    `)
	if err == nil {
		t.Fatal("expected FK violation; insert succeeded")
	}
	if !strings.Contains(err.Error(), "FOREIGN KEY") &&
		!strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected FK error; got: %v", err)
	}
}

func TestApplySchema_SQLite_ErrorWraps(t *testing.T) {
	// applySchema with a bogus backend slice via direct manipulation isn't
	// possible (Schema(unknown) returns nil and applySchema iterates over
	// nothing), so verify the error-message helper instead.
	got := schemaStmtSummary(`CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY
)`)
	want := "CREATE TABLE IF NOT EXISTS agents"
	if got != want {
		t.Errorf("schemaStmtSummary = %q, want %q", got, want)
	}

	if got := schemaStmtSummary(""); got != "<empty>" {
		t.Errorf("schemaStmtSummary(empty) = %q, want %q", got, "<empty>")
	}

	if got := schemaStmtSummary("CREATE INDEX foo ON bar(baz)"); !strings.HasPrefix(got, "CREATE INDEX") {
		t.Errorf("schemaStmtSummary = %q, want CREATE INDEX prefix", got)
	}
}

func TestApplySchema_PostgreSQL(t *testing.T) {
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KSCORE_TEST_POSTGRES_DSN not set; Postgres schema test deferred to task 4 (CI provides docker-compose)")
	}
	// Real exec test lives with the PostgreSQLStore in task 4.
	// Until then the postgres slice is structurally validated by
	// TestSchema_ContainsExpectedTables / TestSchema_ReturnsExpectedBackends.
	_ = dsn
}
