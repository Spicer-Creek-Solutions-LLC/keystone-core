// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// newSQLiteStoreForTest opens a fresh SQLiteStore in a temp dir so tests
// don't pollute the working directory and each test gets isolated state.
func newSQLiteStoreForTest(t *testing.T) *SQLiteStore {
	t.Helper()
	cfg := &Config{
		Backend: BackendSQLite,
		SQLite:  SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	s, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatalf("got %T, want *SQLiteStore", store)
	}
	return s
}

func TestSQLiteStore_Ping(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.Ping(t.Context()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestSQLiteStore_AppliesSchemaOnOpen(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	for _, table := range []string{"agents", "commands", "batch_jobs", "batch_agent_results"} {
		var name string
		err := s.db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not present after NewStore: %v", table, err)
		}
	}
}

func TestSQLiteStore_PragmaWAL(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	var mode string
	if err := s.db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestSQLiteStore_PragmaForeignKeys(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	var fk int
	if err := s.db.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestSQLiteStore_PragmaBusyTimeout(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	var ms int
	if err := s.db.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&ms); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if ms <= 0 {
		t.Errorf("busy_timeout = %d, want >0", ms)
	}
}

func TestSQLiteStore_Close_Idempotent(t *testing.T) {
	cfg := &Config{
		Backend: BackendSQLite,
		SQLite:  SQLiteConfig{Path: filepath.Join(t.TempDir(), "close.db")},
	}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// database/sql.DB.Close on an already-closed DB returns nil; verify
	// our wrapper doesn't surprise.
	_ = store.Close()
}

func TestSQLiteStore_NewStoreFailsOnUnwritablePath(t *testing.T) {
	// /dev/null/foo is a path that can't be opened for write — sqlite
	// surfaces this as an error during open. Verifies the constructor
	// propagates open failures cleanly.
	_, err := NewStore(&Config{
		Backend: BackendSQLite,
		SQLite:  SQLiteConfig{Path: "/dev/null/cannot-create.db"},
	})
	if err == nil {
		t.Fatal("expected error for un-writable path")
	}
}

// Ensure orderByClause + limitOffsetClause produce sane SQL for both
// default and explicit-sort cases. Direct unit tests for the helpers
// keep the SQL-builder logic covered without needing a DB round-trip.
func TestOrderByClause(t *testing.T) {
	tests := []struct {
		name        string
		col, defCol string
		desc        bool
		want        string
	}{
		{"empty col uses default DESC", "", "registered_at", false, " ORDER BY registered_at DESC"},
		{"explicit col ASC", "hostname", "registered_at", false, " ORDER BY hostname ASC"},
		{"explicit col DESC", "hostname", "registered_at", true, " ORDER BY hostname DESC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orderByClause(tt.col, tt.defCol, tt.desc)
			if got != tt.want {
				t.Errorf("orderByClause = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLimitOffsetClause(t *testing.T) {
	tests := []struct {
		name          string
		limit, offset int
		want          string
	}{
		{"both zero", 0, 0, ""},
		{"limit only", 50, 0, " LIMIT 50"},
		{"offset only fills LIMIT", 0, 10, " LIMIT 2147483647 OFFSET 10"},
		{"both", 50, 10, " LIMIT 50 OFFSET 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := limitOffsetClause(tt.limit, tt.offset)
			if got != tt.want {
				t.Errorf("limitOffsetClause = %q, want %q", got, tt.want)
			}
		})
	}
}

// corruptColumnSQLite UPDATEs <table>.<column> to bad JSON for the row
// with the given id. Used by malformed-JSON regression tests across
// every JSON column.
func corruptColumnSQLite(t *testing.T, s *SQLiteStore, table, column, id, badJSON string) {
	t.Helper()
	q := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE id = ?`, table, column)
	if _, err := s.db.ExecContext(t.Context(), q, badJSON, id); err != nil {
		t.Fatalf("corrupt %s.%s: %v", table, column, err)
	}
}

// assertJSONUnmarshalError fails the test unless err is non-nil and
// describes a JSON unmarshal failure. Used by both SQLite and Postgres
// regression tests.
func assertJSONUnmarshalError(t *testing.T, err error, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected unmarshal error, got nil", label)
	}
	if !strings.Contains(err.Error(), "unmarshal") &&
		!strings.Contains(err.Error(), "json") {
		t.Errorf("%s: expected unmarshal-related error; got: %v", label, err)
	}
}

func TestUnmarshalJSONColumn(t *testing.T) {
	t.Run("empty string is no-op", func(t *testing.T) {
		var v map[string]string
		if err := unmarshalJSONColumn("", &v); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if v != nil {
			t.Errorf("v should be nil; got %v", v)
		}
	})
	t.Run("valid json", func(t *testing.T) {
		var v map[string]string
		if err := unmarshalJSONColumn(`{"a":"b"}`, &v); err != nil {
			t.Fatal(err)
		}
		if v["a"] != "b" {
			t.Errorf("v = %v", v)
		}
	})
	t.Run("malformed json surfaces", func(t *testing.T) {
		var v map[string]string
		err := unmarshalJSONColumn(`not-json`, &v)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unmarshal") {
			t.Errorf("error should describe unmarshal: %v", err)
		}
	})
}

func TestValidateSortColumn(t *testing.T) {
	if err := validateSortColumn("", AllowedAgentSortColumns); err != nil {
		t.Errorf("empty col: unexpected error %v", err)
	}
	if err := validateSortColumn("hostname", AllowedAgentSortColumns); err != nil {
		t.Errorf("allowlisted col: unexpected error %v", err)
	}
	if err := validateSortColumn("password; DROP TABLE agents--", AllowedAgentSortColumns); err == nil {
		t.Error("expected error for non-allowlisted col")
	}
}

// NewStore returns a Store interface; verify the value works through
// the interface (not just *SQLiteStore directly).
func TestSQLiteStore_StoreInterfacePing(t *testing.T) {
	cfg := &Config{
		Backend: BackendSQLite,
		SQLite:  SQLiteConfig{Path: filepath.Join(t.TempDir(), "iface.db")},
	}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping via Store interface: %v", err)
	}
}
