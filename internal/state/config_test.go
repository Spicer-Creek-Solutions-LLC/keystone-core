package state

import (
	"strings"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string // substring; "" means expect no error
	}{
		{
			name:    "missing backend",
			cfg:     Config{},
			wantErr: "Backend is required",
		},
		{
			name:    "unknown backend",
			cfg:     Config{Backend: "mysql"},
			wantErr: "unknown Backend",
		},
		{
			name: "sqlite minimal",
			cfg:  Config{Backend: BackendSQLite},
		},
		{
			name:    "sqlite rejects MaxOpenConns > 1",
			cfg:     Config{Backend: BackendSQLite, MaxOpenConns: 5},
			wantErr: "sqlite MaxOpenConns must be 0 or 1",
		},
		{
			name: "sqlite accepts MaxOpenConns == 1",
			cfg:  Config{Backend: BackendSQLite, MaxOpenConns: 1},
		},
		{
			name: "postgres with DSN",
			cfg:  Config{Backend: BackendPostgreSQL, PostgreSQL: PostgreSQLConfig{DSN: "postgres://u:p@h/d"}},
		},
		{
			name: "postgres with struct fields",
			cfg: Config{
				Backend:    BackendPostgreSQL,
				PostgreSQL: PostgreSQLConfig{Host: "h", Database: "d", User: "u"},
			},
		},
		{
			name:    "postgres missing required fields",
			cfg:     Config{Backend: BackendPostgreSQL},
			wantErr: "requires DSN or Host+Database+User",
		},
		{
			name:    "postgres missing User",
			cfg:     Config{Backend: BackendPostgreSQL, PostgreSQL: PostgreSQLConfig{Host: "h", Database: "d"}},
			wantErr: "requires DSN or Host+Database+User",
		},
		{
			name:    "negative MaxIdleConns rejected",
			cfg:     Config{Backend: BackendSQLite, MaxIdleConns: -1},
			wantErr: "MaxIdleConns must be non-negative",
		},
		{
			name:    "negative ConnMaxLife rejected",
			cfg:     Config{Backend: BackendSQLite, ConnMaxLife: -time.Second},
			wantErr: "ConnMaxLife must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q; got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_applyDefaults_SQLite(t *testing.T) {
	cfg := &Config{Backend: BackendSQLite}
	cfg.applyDefaults()

	if cfg.SQLite.Path != "./data/keystone.db" {
		t.Errorf("Path default = %q, want %q", cfg.SQLite.Path, "./data/keystone.db")
	}
	if cfg.SQLite.BusyTimeout != 5*time.Second {
		t.Errorf("BusyTimeout default = %s, want 5s", cfg.SQLite.BusyTimeout)
	}
	if cfg.MaxOpenConns != 1 {
		t.Errorf("MaxOpenConns default = %d, want 1 (sqlite single writer)", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 1 {
		t.Errorf("MaxIdleConns default = %d, want 1", cfg.MaxIdleConns)
	}
}

func TestConfig_applyDefaults_Postgres(t *testing.T) {
	cfg := &Config{Backend: BackendPostgreSQL}
	cfg.applyDefaults()

	if cfg.PostgreSQL.SSLMode != "require" {
		t.Errorf("SSLMode default = %q, want %q", cfg.PostgreSQL.SSLMode, "require")
	}
	if cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns default = %d, want 25", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns default = %d, want 5", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLife != 30*time.Minute {
		t.Errorf("ConnMaxLife default = %s, want 30m", cfg.ConnMaxLife)
	}
}

func TestConfig_applyDefaults_PreservesExplicitValues(t *testing.T) {
	cfg := &Config{
		Backend:      BackendPostgreSQL,
		MaxOpenConns: 100,
		MaxIdleConns: 10,
		ConnMaxLife:  1 * time.Hour,
		PostgreSQL:   PostgreSQLConfig{SSLMode: "verify-full"},
	}
	cfg.applyDefaults()

	if cfg.MaxOpenConns != 100 || cfg.MaxIdleConns != 10 ||
		cfg.ConnMaxLife != time.Hour || cfg.PostgreSQL.SSLMode != "verify-full" {
		t.Errorf("applyDefaults clobbered explicit values: %+v", cfg)
	}
}

func TestConfig_applyDefaults_Idempotent(t *testing.T) {
	cfg := &Config{Backend: BackendSQLite}
	cfg.applyDefaults()
	snapshot := *cfg
	cfg.applyDefaults()
	if *cfg != snapshot {
		t.Errorf("applyDefaults not idempotent: before=%+v after=%+v", snapshot, *cfg)
	}
}
