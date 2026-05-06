package state

import (
	"path/filepath"
	"strings"
	"testing"
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

// Postgres-backed factory test lives in postgres_test.go (integration-
// tagged + KSCORE_TEST_POSTGRES_DSN-gated) — it requires a live Postgres
// since newPostgreSQLStore now Pings on construction.
