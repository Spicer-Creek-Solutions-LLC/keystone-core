// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestAgentConfig_Validate(t *testing.T) {
	good := AgentConfig{
		AgentID:           "agent-1",
		HeartbeatInterval: 30 * time.Second,
		MetadataInterval:  60 * time.Second,
		CommandTimeout:    5 * time.Minute,
	}
	tests := []struct {
		name    string
		mut     func(*AgentConfig)
		wantErr string
	}{
		{"defaults ok", func(*AgentConfig) {}, ""},
		{"empty agent id ok at root validate", func(c *AgentConfig) { c.AgentID = "" }, ""},
		{"negative heartbeat", func(c *AgentConfig) { c.HeartbeatInterval = -time.Second }, "heartbeatinterval"},
		{"negative metadata", func(c *AgentConfig) { c.MetadataInterval = -time.Second }, "metadatainterval"},
		{"negative timeout", func(c *AgentConfig) { c.CommandTimeout = -time.Second }, "commandtimeout"},
		{"zero intervals ok", func(c *AgentConfig) {
			c.HeartbeatInterval = 0
			c.MetadataInterval = 0
			c.CommandTimeout = 0
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := good
			tt.mut(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAgentConfig_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agent.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval = %s, want 30s", cfg.Agent.HeartbeatInterval)
	}
	if cfg.Agent.MetadataInterval != 60*time.Second {
		t.Errorf("MetadataInterval = %s, want 60s", cfg.Agent.MetadataInterval)
	}
	if cfg.Agent.CommandTimeout != 5*time.Minute {
		t.Errorf("CommandTimeout = %s, want 5m", cfg.Agent.CommandTimeout)
	}
}
