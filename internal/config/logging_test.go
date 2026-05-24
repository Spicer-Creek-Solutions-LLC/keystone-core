// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

func TestLoggingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     LoggingConfig
		wantErr string
	}{
		{"info json", LoggingConfig{Level: "info", Format: "json"}, ""},
		{"debug logfmt", LoggingConfig{Level: "debug", Format: "logfmt"}, ""},
		{"warn text", LoggingConfig{Level: "warn", Format: "text"}, ""},
		{"error json", LoggingConfig{Level: "error", Format: "json"}, ""},
		{"bad level", LoggingConfig{Level: "trace", Format: "json"}, "level"},
		{"bad format", LoggingConfig{Level: "info", Format: "xml"}, "format"},
		{"empty level", LoggingConfig{Level: "", Format: "json"}, "level"},
		{"empty format", LoggingConfig{Level: "info", Format: ""}, "format"},
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
