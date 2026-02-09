// Package smoke provides smoke tests for critical infrastructure components.
// These tests verify basic functionality and are designed to run quickly
// as part of pre-commit hooks.
package smoke

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// TestSQLiteSmokeTest verifies basic SQLite operations
func TestSQLiteSmokeTest(t *testing.T) {
	// Create a temporary database
	tmpDir, err := os.MkdirTemp("", "kscore-smoke-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "smoke.db")

	// Test: Open database with WAL mode
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Test: Connection settings
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test: Ping database
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping SQLite database: %v", err)
	}

	// Test: Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS smoke_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			data BLOB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test: Insert data
	result, err := db.ExecContext(ctx, `
		INSERT INTO smoke_test (name, data) VALUES (?, ?)
	`, "test-entry", []byte("test data"))
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID: %v", err)
	}
	if id != 1 {
		t.Errorf("Expected ID 1, got %d", id)
	}

	// Test: Query data
	var name string
	var data []byte
	err = db.QueryRowContext(ctx, `
		SELECT name, data FROM smoke_test WHERE id = ?
	`, id).Scan(&name, &data)
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}
	if name != "test-entry" {
		t.Errorf("Expected name 'test-entry', got '%s'", name)
	}
	if string(data) != "test data" {
		t.Errorf("Expected data 'test data', got '%s'", string(data))
	}

	// Test: Update data
	_, err = db.ExecContext(ctx, `
		UPDATE smoke_test SET name = ? WHERE id = ?
	`, "updated-entry", id)
	if err != nil {
		t.Fatalf("Failed to update data: %v", err)
	}

	// Test: Transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO smoke_test (name) VALUES (?)
	`, "tx-entry")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Test: Count rows
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM smoke_test`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows, got %d", count)
	}

	// Test: Delete data
	_, err = db.ExecContext(ctx, `DELETE FROM smoke_test WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("Failed to delete data: %v", err)
	}

	// Test: WAL checkpoint
	_, err = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	if err != nil {
		t.Fatalf("Failed WAL checkpoint: %v", err)
	}

	t.Log("SQLite smoke test passed")
}

// TestPostgreSQLSmokeTest verifies basic PostgreSQL operations
// This test requires a PostgreSQL database to be available
func TestPostgreSQLSmokeTest(t *testing.T) {
	// Get PostgreSQL connection string from environment
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		// Try common CI service patterns
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		// Use default localhost connection for local development
		dsn = "postgres://kscore:kscore@localhost:5432/kscore_test?sslmode=disable"
	}

	// Open database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("PostgreSQL not available (open error): %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test: Ping database
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("PostgreSQL not available (ping error): %v", err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test: Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS smoke_test_pg (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			data BYTEA,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	defer func() {
		// Clean up after test
		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS smoke_test_pg`)
	}()

	// Test: Insert data with RETURNING
	var id int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO smoke_test_pg (name, data, metadata)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "test-entry", []byte("test data"), `{"key": "value"}`).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}
	if id < 1 {
		t.Errorf("Expected positive ID, got %d", id)
	}

	// Test: Query with JSONB
	var name string
	var data []byte
	var metadata string
	err = db.QueryRowContext(ctx, `
		SELECT name, data, metadata::text FROM smoke_test_pg WHERE id = $1
	`, id).Scan(&name, &data, &metadata)
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}
	if name != "test-entry" {
		t.Errorf("Expected name 'test-entry', got '%s'", name)
	}

	// Test: Update data
	_, err = db.ExecContext(ctx, `
		UPDATE smoke_test_pg SET name = $1, metadata = $2 WHERE id = $3
	`, "updated-entry", `{"key": "updated"}`, id)
	if err != nil {
		t.Fatalf("Failed to update data: %v", err)
	}

	// Test: Transaction with isolation level
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO smoke_test_pg (name) VALUES ($1)
	`, "tx-entry")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Test: Count rows
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM smoke_test_pg`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows, got %d", count)
	}

	// Test: JSONB query
	err = db.QueryRowContext(ctx, `
		SELECT name FROM smoke_test_pg
		WHERE metadata @> '{"key": "updated"}'
	`).Scan(&name)
	if err != nil {
		t.Fatalf("Failed JSONB query: %v", err)
	}
	if name != "updated-entry" {
		t.Errorf("Expected name 'updated-entry', got '%s'", name)
	}

	// Test: Delete data
	result, err := db.ExecContext(ctx, `DELETE FROM smoke_test_pg WHERE id = $1`, id)
	if err != nil {
		t.Fatalf("Failed to delete data: %v", err)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		t.Errorf("Expected 1 row affected, got %d", affected)
	}

	t.Log("PostgreSQL smoke test passed")
}

// TestDatabaseBackendCompatibility tests that both backends support
// the same basic operations required by Keystone Core
func TestDatabaseBackendCompatibility(t *testing.T) {
	type testCase struct {
		name     string
		sqliteFn func(db *sql.DB) error
		pgFn     func(db *sql.DB) error
	}

	ctx := context.Background()
	tests := []testCase{
		{
			name: "json_operations",
			sqliteFn: func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, `
					CREATE TABLE IF NOT EXISTS json_test (
						id INTEGER PRIMARY KEY,
						data TEXT
					)
				`)
				if err != nil {
					return err
				}
				_, err = db.ExecContext(ctx, `INSERT INTO json_test (data) VALUES ('{"test": true}')`)
				return err
			},
			pgFn: func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, `
					CREATE TABLE IF NOT EXISTS json_test (
						id SERIAL PRIMARY KEY,
						data JSONB
					)
				`)
				if err != nil {
					return err
				}
				_, err = db.ExecContext(ctx, `INSERT INTO json_test (data) VALUES ('{"test": true}')`)
				return err
			},
		},
		{
			name: "timestamp_operations",
			sqliteFn: func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, `
					CREATE TABLE IF NOT EXISTS ts_test (
						id INTEGER PRIMARY KEY,
						ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP
					)
				`)
				if err != nil {
					return err
				}
				_, err = db.ExecContext(ctx, `INSERT INTO ts_test (id) VALUES (1)`)
				if err != nil {
					return err
				}
				var ts string
				return db.QueryRowContext(ctx, `SELECT ts FROM ts_test WHERE id = 1`).Scan(&ts)
			},
			pgFn: func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, `
					CREATE TABLE IF NOT EXISTS ts_test (
						id SERIAL PRIMARY KEY,
						ts TIMESTAMP WITH TIME ZONE DEFAULT NOW()
					)
				`)
				if err != nil {
					return err
				}
				_, err = db.ExecContext(ctx, `INSERT INTO ts_test DEFAULT VALUES`)
				if err != nil {
					return err
				}
				var ts time.Time
				return db.QueryRowContext(ctx, `SELECT ts FROM ts_test ORDER BY id DESC LIMIT 1`).Scan(&ts)
			},
		},
	}

	// Run SQLite tests
	t.Run("SQLite", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "kscore-compat-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
		if err != nil {
			t.Fatalf("Failed to open SQLite: %v", err)
		}
		defer db.Close()

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if err := tc.sqliteFn(db); err != nil {
					t.Errorf("SQLite %s failed: %v", tc.name, err)
				}
			})
		}
	})

	// Run PostgreSQL tests (if available)
	t.Run("PostgreSQL", func(t *testing.T) {
		dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
		if dsn == "" {
			dsn = "postgres://kscore:kscore@localhost:5432/kscore_test?sslmode=disable"
		}

		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Skipf("PostgreSQL not available: %v", err)
		}
		defer db.Close()

		if err := db.PingContext(ctx); err != nil {
			t.Skipf("PostgreSQL not available: %v", err)
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if err := tc.pgFn(db); err != nil {
					t.Errorf("PostgreSQL %s failed: %v", tc.name, err)
				}
			})
		}

		// Cleanup
		db.ExecContext(ctx, `DROP TABLE IF EXISTS json_test`)
		db.ExecContext(ctx, `DROP TABLE IF EXISTS ts_test`)
	})
}

// BenchmarkSQLiteInsert benchmarks SQLite insert performance
func BenchmarkSQLiteInsert(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "kscore-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "bench.db")+"?_journal_mode=WAL")
	if err != nil {
		b.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE bench_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			data TEXT
		)
	`)
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO bench_test (name, data) VALUES (?, ?)`,
			fmt.Sprintf("entry-%d", i), "benchmark data")
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}
	}
}

// BenchmarkSQLiteQuery benchmarks SQLite query performance
func BenchmarkSQLiteQuery(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "kscore-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "bench.db")+"?_journal_mode=WAL")
	if err != nil {
		b.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE bench_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			data TEXT
		)
	`)
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	for i := 0; i < 1000; i++ {
		_, _ = db.ExecContext(ctx, `INSERT INTO bench_test (name, data) VALUES (?, ?)`,
			fmt.Sprintf("entry-%d", i), "benchmark data")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var name, data string
		err = db.QueryRowContext(ctx, `SELECT name, data FROM bench_test WHERE id = ?`, (i%1000)+1).Scan(&name, &data)
		if err != nil {
			b.Fatalf("Query failed: %v", err)
		}
	}
}
