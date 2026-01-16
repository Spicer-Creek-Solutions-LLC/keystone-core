package bootstrap

import (
	"path/filepath"
	"testing"
)

func TestEnsureSQLiteDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "kscore.db")
	created, err := ensureSQLiteDatabase(path)
	if err != nil {
		t.Fatalf("ensureSQLiteDatabase returned error: %v", err)
	}
	if !created {
		t.Fatal("expected database to be created")
	}

	created, err = ensureSQLiteDatabase(path)
	if err != nil {
		t.Fatalf("ensureSQLiteDatabase returned error: %v", err)
	}
	if created {
		t.Fatal("expected database to be detected as existing")
	}
}

func TestValidatePostgresDSN(t *testing.T) {
	if err := validatePostgresDSN("postgres://user:pass@localhost:5432/db?sslmode=prefer"); err != nil {
		t.Fatalf("expected valid dsn, got error: %v", err)
	}
	if err := validatePostgresDSN("http://localhost"); err == nil {
		t.Fatal("expected error for non-postgres scheme")
	}
}
