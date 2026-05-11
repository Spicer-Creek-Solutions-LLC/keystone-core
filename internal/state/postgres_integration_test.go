//go:build integration

package state

import (
	"database/sql"
	"os"
	"testing"
)

// newPgStoreForTest returns a connected *PostgreSQLStore with the v1.0
// schema applied and all four tables truncated. The KSCORE_TEST_
// POSTGRES_DSN env var must point at a writable Postgres instance.
//
// Tests share a single database; each test gets a clean slate via
// TRUNCATE in setup.
func newPgStoreForTest(t *testing.T) *PostgreSQLStore {
	t.Helper()
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KSCORE_TEST_POSTGRES_DSN not set; integration tests need a live Postgres")
	}

	cfg := &Config{
		Backend:    BackendPostgreSQL,
		PostgreSQL: PostgreSQLConfig{DSN: dsn},
	}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	s, ok := store.(*PostgreSQLStore)
	if !ok {
		t.Fatalf("got %T, want *PostgreSQLStore", store)
	}

	truncateAll(t, s.db)
	return s
}

// truncateAll empties all v1.0 tables in FK-safe order.
func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	const stmt = `TRUNCATE TABLE
        state_run_results, state_runs,
        batch_agent_results, batch_jobs, commands, agents, apikeys
        RESTART IDENTITY CASCADE`
	if _, err := db.ExecContext(t.Context(), stmt); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func TestPg_NewStoreReturnsConcreteType(t *testing.T) {
	s := newPgStoreForTest(t)
	if s == nil {
		t.Fatal("nil store")
	}
}

func TestPg_Ping(t *testing.T) {
	s := newPgStoreForTest(t)
	if err := s.Ping(t.Context()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestPg_AppliesSchemaOnOpen(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	for _, table := range []string{"agents", "commands", "batch_jobs", "batch_agent_results"} {
		var name string
		err := s.db.QueryRowContext(ctx,
			`SELECT tablename FROM pg_tables WHERE tablename = $1`,
			table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not present after NewStore: %v", table, err)
		}
	}
}

func TestPg_ApplySchemaIdempotent(t *testing.T) {
	s := newPgStoreForTest(t)
	if err := applySchema(t.Context(), s.db, BackendPostgreSQL); err != nil {
		t.Errorf("second applySchema: %v", err)
	}
}
