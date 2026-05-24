// SPDX-License-Identifier: Apache-2.0

package dbutil

import (
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

func TestOpenSQLite_OpensSuccessfully(t *testing.T) {
	db, err := OpenSQLite(tempDB(t))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if db == nil {
		t.Fatal("returned db is nil")
	}
}

func TestOpenSQLite_PragmaJournalMode(t *testing.T) {
	db, err := OpenSQLite(tempDB(t))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestOpenSQLite_PragmaForeignKeys(t *testing.T) {
	db, err := OpenSQLite(tempDB(t))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var on int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if on != 1 {
		t.Errorf("foreign_keys = %d, want 1", on)
	}
}

func TestOpenSQLite_PragmaBusyTimeout(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		wantMs int64
	}{
		{"default", nil, 5000},
		{"override 2s", []Option{WithBusyTimeout(2 * time.Second)}, 2000},
		{"override 250ms", []Option{WithBusyTimeout(250 * time.Millisecond)}, 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := OpenSQLite(tempDB(t), tt.opts...)
			if err != nil {
				t.Fatalf("OpenSQLite: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			var got int64
			if err := db.QueryRow("PRAGMA busy_timeout").Scan(&got); err != nil {
				t.Fatalf("query busy_timeout: %v", err)
			}
			if got != tt.wantMs {
				t.Errorf("busy_timeout = %d, want %d", got, tt.wantMs)
			}
		})
	}
}

func TestOpenSQLite_SingleWriter(t *testing.T) {
	db, err := OpenSQLite(tempDB(t))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestOpenSQLite_FKEnforced(t *testing.T) {
	db, err := OpenSQLite(tempDB(t))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE child (
		id  INTEGER PRIMARY KEY,
		pid INTEGER NOT NULL,
		FOREIGN KEY (pid) REFERENCES parent(id)
	)`); err != nil {
		t.Fatalf("create child: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO child (id, pid) VALUES (1, 999)`); err == nil {
		t.Error("INSERT with bad FK succeeded; expected FK constraint violation")
	}
}

func TestOpenSQLite_InvalidPath(t *testing.T) {
	// Parent directory doesn't exist; SQLite cannot create the file.
	if _, err := OpenSQLite("/nonexistent-keystone-test-dir-7421/db.sqlite"); err == nil {
		t.Error("OpenSQLite with invalid path: expected error, got nil")
	}
}
