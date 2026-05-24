// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestHealthConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     HealthConfig
		wantErr string
	}{
		{"zero ok", HealthConfig{}, ""},
		{"both set ok", HealthConfig{StartupGracePeriod: 30 * time.Second, CheckTimeout: 2 * time.Second}, ""},
		{"negative grace", HealthConfig{StartupGracePeriod: -time.Second}, "startupgraceperiod"},
		{"negative timeout", HealthConfig{CheckTimeout: -time.Second}, "checktimeout"},
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
