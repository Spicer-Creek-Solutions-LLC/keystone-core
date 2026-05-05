package config

import (
	"strings"
	"testing"
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
