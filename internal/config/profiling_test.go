// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func validProfiling() ProfilingConfig {
	return ProfilingConfig{
		Enabled:         true,
		Host:            "127.0.0.1",
		Port:            6060,
		ShutdownTimeout: 5 * time.Second,
	}
}

func TestProfilingConfig_Validate(t *testing.T) {
	with := func(mut func(*ProfilingConfig)) ProfilingConfig {
		c := validProfiling()
		if mut != nil {
			mut(&c)
		}
		return c
	}

	tests := []struct {
		name    string
		cfg     ProfilingConfig
		wantErr string
	}{
		{"disabled passes trivially", ProfilingConfig{Enabled: false}, ""},
		{"baseline", validProfiling(), ""},
		{"empty host", with(func(c *ProfilingConfig) { c.Host = "" }), "host"},
		{"port zero", with(func(c *ProfilingConfig) { c.Port = 0 }), "port"},
		{"port high", with(func(c *ProfilingConfig) { c.Port = 70000 }), "port"},
		{"mutex frac negative", with(func(c *ProfilingConfig) { c.MutexProfileFraction = -1 }), "mutexprofilefraction"},
		{"block rate negative", with(func(c *ProfilingConfig) { c.BlockProfileRate = -1 }), "blockprofilerate"},
		{"shutdown timeout negative", with(func(c *ProfilingConfig) { c.ShutdownTimeout = -time.Second }), "shutdowntimeout"},
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

func TestDefaultConfig_HasProfilingDefaults(t *testing.T) {
	c := defaultConfig()
	if c.Profiling.Enabled {
		t.Error("Profiling.Enabled default = true, want false (opt-in)")
	}
	if c.Profiling.Host != "127.0.0.1" {
		t.Errorf("Profiling.Host default = %q, want 127.0.0.1", c.Profiling.Host)
	}
	if c.Profiling.Port != 6060 {
		t.Errorf("Profiling.Port default = %d, want 6060", c.Profiling.Port)
	}
	if c.Profiling.ShutdownTimeout != 5*time.Second {
		t.Errorf("Profiling.ShutdownTimeout default = %s, want 5s", c.Profiling.ShutdownTimeout)
	}
}
