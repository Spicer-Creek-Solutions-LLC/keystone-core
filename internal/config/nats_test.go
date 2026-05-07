package config

import (
	"strings"
	"testing"
	"time"
)

func validNATS() NATSConfig {
	return NATSConfig{
		Mode:          NATSModeEmbedded,
		ClusterName:   "default",
		MaxReconnects: 60,
		ReconnectWait: 2 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:    true,
			StoreDir:   "./data/jetstream",
			MaxStorage: 1024,
		},
		Embedded: EmbeddedNATSConfig{
			Host:            "127.0.0.1",
			Port:            4222,
			EnableJetStream: true,
		},
	}
}

func TestNATSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mut     func(*NATSConfig)
		wantErr string
	}{
		{"defaults ok", func(*NATSConfig) {}, ""},
		{
			"external requires urls",
			func(c *NATSConfig) { c.Mode = NATSModeExternal },
			"urls",
		},
		{
			"external ok with urls",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = []string{"nats://localhost:4222"}
			},
			"",
		},
		{
			"embedded forbids urls",
			func(c *NATSConfig) { c.URLs = []string{"nats://x:4222"} },
			"urls",
		},
		{
			"empty url entry rejected",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = []string{""}
			},
			"urls[0]",
		},
		{
			"unknown mode",
			func(c *NATSConfig) { c.Mode = "weird" },
			"mode",
		},
		{
			"empty cluster name",
			func(c *NATSConfig) { c.ClusterName = "" },
			"clustername",
		},
		{
			"maxreconnects too negative",
			func(c *NATSConfig) { c.MaxReconnects = -2 },
			"maxreconnects",
		},
		{
			"infinite reconnects ok",
			func(c *NATSConfig) { c.MaxReconnects = -1 },
			"",
		},
		{
			"reconnect wait negative",
			func(c *NATSConfig) { c.ReconnectWait = -time.Second },
			"reconnectwait",
		},
		{
			"jetstream maxstorage negative",
			func(c *NATSConfig) { c.JetStream.MaxStorage = -1 },
			"jetstream",
		},
		{
			"jetstream enabled needs storedir",
			func(c *NATSConfig) {
				c.JetStream.Enabled = true
				c.JetStream.StoreDir = ""
			},
			"storedir",
		},
		{
			"jetstream disabled accepts empty storedir",
			func(c *NATSConfig) {
				c.JetStream.Enabled = false
				c.JetStream.StoreDir = ""
				c.Embedded.EnableJetStream = false
			},
			"",
		},
		{
			"embedded host empty",
			func(c *NATSConfig) { c.Embedded.Host = "" },
			"host",
		},
		{
			"embedded port out of range",
			func(c *NATSConfig) { c.Embedded.Port = 0 },
			"port",
		},
		{
			"embedded port too high",
			func(c *NATSConfig) { c.Embedded.Port = 70000 },
			"port",
		},
		{
			"embedded maxconnections negative",
			func(c *NATSConfig) { c.Embedded.MaxConnections = -1 },
			"maxconnections",
		},
		{
			"embedded maxmemory negative",
			func(c *NATSConfig) { c.Embedded.MaxMemory = -1 },
			"maxmemory",
		},
		{
			"external mode skips embedded validation",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = []string{"nats://x:4222"}
				c.Embedded.Host = "" // would fail embedded.Validate, but irrelevant
				c.Embedded.Port = 0
			},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validNATS()
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
				t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNATSConfig_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.NATS.Mode != NATSModeEmbedded {
		t.Errorf("Mode = %q, want embedded", cfg.NATS.Mode)
	}
	if cfg.NATS.ClusterName != "default" {
		t.Errorf("ClusterName = %q, want default", cfg.NATS.ClusterName)
	}
	if cfg.NATS.Embedded.Port != 4222 {
		t.Errorf("Embedded.Port = %d, want 4222", cfg.NATS.Embedded.Port)
	}
	if cfg.NATS.Embedded.Host != "127.0.0.1" {
		t.Errorf("Embedded.Host = %q, want 127.0.0.1", cfg.NATS.Embedded.Host)
	}
	if !cfg.NATS.JetStream.Enabled {
		t.Error("JetStream.Enabled = false, want true")
	}
	if cfg.NATS.JetStream.StoreDir != "./data/jetstream" {
		t.Errorf("JetStream.StoreDir = %q", cfg.NATS.JetStream.StoreDir)
	}
	if cfg.NATS.JetStream.MaxStorage != 10*1024*1024*1024 {
		t.Errorf("JetStream.MaxStorage = %d, want 10 GiB", cfg.NATS.JetStream.MaxStorage)
	}
	if cfg.NATS.MaxReconnects != 60 {
		t.Errorf("MaxReconnects = %d, want 60", cfg.NATS.MaxReconnects)
	}
	if cfg.NATS.ReconnectWait != 2*time.Second {
		t.Errorf("ReconnectWait = %s, want 2s", cfg.NATS.ReconnectWait)
	}
}

func TestNATSConfig_FromYAML(t *testing.T) {
	path := writeYAML(t, `
nats:
  mode: external
  urls:
    - nats://node-a:4222
    - nats://node-b:4222
  token: secret
  clustername: prod-east
  maxreconnects: -1
  reconnectwait: 5s
  jetstream:
    enabled: true
    storedir: /var/lib/keystone/jetstream
    maxstorage: 5368709120
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.NATS.Mode != NATSModeExternal {
		t.Errorf("Mode = %q", cfg.NATS.Mode)
	}
	if got, want := cfg.NATS.URLs, []string{"nats://node-a:4222", "nats://node-b:4222"}; !equal(got, want) {
		t.Errorf("URLs = %v, want %v", got, want)
	}
	if cfg.NATS.Token != "secret" {
		t.Errorf("Token = %q", cfg.NATS.Token)
	}
	if cfg.NATS.ClusterName != "prod-east" {
		t.Errorf("ClusterName = %q", cfg.NATS.ClusterName)
	}
	if cfg.NATS.MaxReconnects != -1 {
		t.Errorf("MaxReconnects = %d", cfg.NATS.MaxReconnects)
	}
	if cfg.NATS.ReconnectWait != 5*time.Second {
		t.Errorf("ReconnectWait = %s", cfg.NATS.ReconnectWait)
	}
	if cfg.NATS.JetStream.StoreDir != "/var/lib/keystone/jetstream" {
		t.Errorf("StoreDir = %q", cfg.NATS.JetStream.StoreDir)
	}
	if cfg.NATS.JetStream.MaxStorage != 5368709120 {
		t.Errorf("MaxStorage = %d", cfg.NATS.JetStream.MaxStorage)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
