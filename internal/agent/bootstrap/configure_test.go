// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"strings"
	"testing"
)

func TestConfiguration_Validate(t *testing.T) {
	good := Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  "/etc/kscore/agent.yaml",
	}
	tests := []struct {
		name    string
		mut     func(*Configuration)
		wantErr string
	}{
		{"defaults ok", func(*Configuration) {}, ""},
		{"empty mode", func(c *Configuration) { c.Mode = "" }, "mode"},
		{"weird mode", func(c *Configuration) { c.Mode = "perhaps" }, "mode"},
		{"empty cluster name", func(c *Configuration) { c.ClusterName = "" }, "cluster_name"},
		{"empty agent id", func(c *Configuration) { c.AgentID = "" }, "agent_id"},
		{"empty config path", func(c *Configuration) { c.ConfigPath = "" }, "config_path"},
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

func TestStaticConfigurer_ReturnsCopy(t *testing.T) {
	original := &Configuration{Mode: ModeDemo, ClusterName: "x", AgentID: "a", ConfigPath: "/p"}
	s := StaticConfigurer{Config: original}
	got, err := s.Configure(context.Background(), nil)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Mutating the result must not bleed into the original.
	got.ClusterName = "mutated"
	if original.ClusterName != "x" {
		t.Errorf("StaticConfigurer didn't copy: original.ClusterName = %q", original.ClusterName)
	}
}

func TestStaticConfigurer_NilConfigErrors(t *testing.T) {
	s := StaticConfigurer{Config: nil}
	if _, err := s.Configure(context.Background(), nil); err == nil {
		t.Error("Configure(nil): expected error")
	}
}

func TestValidateForV10(t *testing.T) {
	good := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  "/etc/kscore/agent.yaml",
	}
	tests := []struct {
		name    string
		mut     func(*Configuration) *Configuration
		wantErr string
	}{
		{"demo accepted", func(c *Configuration) *Configuration { return c }, ""},
		{"production rejected", func(c *Configuration) *Configuration { c.Mode = ModeProduction; return c }, "v1.x"},
		{"enterprise rejected", func(c *Configuration) *Configuration { c.Mode = ModeEnterprise; return c }, "v1.x"},
		{"nil rejected", func(*Configuration) *Configuration { return nil }, "nil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.mut(&Configuration{
				Mode:        good.Mode,
				ClusterName: good.ClusterName,
				AgentID:     good.AgentID,
				ConfigPath:  good.ConfigPath,
			})
			err := ValidateForV10(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("err = nil, want containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
