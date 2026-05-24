// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

func TestMetricsConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     MetricsConfig
		wantErr string
	}{
		{"disabled ok", MetricsConfig{Enabled: false, Path: ""}, ""},
		{"enabled default path", MetricsConfig{Enabled: true, Path: "/metrics"}, ""},
		{"enabled custom path", MetricsConfig{Enabled: true, Path: "/internal/metrics"}, ""},
		{"enabled empty path", MetricsConfig{Enabled: true, Path: ""}, "path"},
		{"enabled non-rooted path", MetricsConfig{Enabled: true, Path: "metrics"}, "must start with"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig_HasMetricsDefaults(t *testing.T) {
	c := defaultConfig()
	if !c.Metrics.Enabled {
		t.Error("Metrics.Enabled default = false, want true")
	}
	if c.Metrics.Path != "/metrics" {
		t.Errorf("Metrics.Path default = %q, want /metrics", c.Metrics.Path)
	}
}
