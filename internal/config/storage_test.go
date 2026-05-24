// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/state"
)

func TestStorageConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     StorageConfig
		wantErr string
	}{
		{"sqlite ok", StorageConfig{Driver: "sqlite", DSN: "./db"}, ""},
		{"postgres ok", StorageConfig{Driver: "postgres", DSN: "postgres://x"}, ""},
		{"unknown driver", StorageConfig{Driver: "mysql", DSN: "x"}, "driver"},
		{"empty driver", StorageConfig{Driver: "", DSN: "x"}, "driver"},
		{"empty dsn", StorageConfig{Driver: "sqlite", DSN: ""}, "dsn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestStorageConfig_ToStateConfig(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		got, err := StorageConfig{Driver: "sqlite", DSN: "/tmp/x.db"}.ToStateConfig()
		if err != nil {
			t.Fatalf("ToStateConfig: %v", err)
		}
		if got.Backend != state.BackendSQLite || got.SQLite.Path != "/tmp/x.db" {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		got, err := StorageConfig{Driver: "postgres", DSN: "postgres://localhost/db"}.ToStateConfig()
		if err != nil {
			t.Fatalf("ToStateConfig: %v", err)
		}
		if got.Backend != state.BackendPostgreSQL || got.PostgreSQL.DSN != "postgres://localhost/db" {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("unknown driver", func(t *testing.T) {
		if _, err := (StorageConfig{Driver: "mysql", DSN: "x"}).ToStateConfig(); err == nil {
			t.Error("expected error")
		}
	})
}
