package dbutil

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenSQLite(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()

	// Verify WAL mode is enabled.
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	// Verify we can create tables and write data.
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO test (name) VALUES (?)", "hello")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestOpenSQLite_WithForeignKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fk.db")

	db, err := OpenSQLite(path, WithForeignKeys())
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpenSQLite_WithBusyTimeout(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "busy.db")

	db, err := OpenSQLite(path, WithBusyTimeout(3000))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()

	var timeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 3000 {
		t.Errorf("busy_timeout = %d, want 3000", timeout)
	}
}

func TestOpenSQLite_MultipleOptions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "multi.db")

	db, err := OpenSQLite(path, WithForeignKeys(), WithBusyTimeout(5000))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	var timeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestOpenSQLite_OptionError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "err.db")

	badOpt := func(_ context.Context, _ *sql.DB) error {
		return sql.ErrConnDone
	}

	db, err := OpenSQLite(path, badOpt)
	if err == nil {
		db.Close()
		t.Fatal("expected error from bad option")
	}
}

func TestOpenSQLite_InvalidPath(t *testing.T) {
	_, err := OpenSQLite("/nonexistent/dir/that/does/not/exist/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}
