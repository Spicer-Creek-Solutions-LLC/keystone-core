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
			Enabled:        true,
			StoreDir:       "./data/jetstream",
			MaxStorage:     1024,
			StreamMaxAge:   7 * 24 * time.Hour,
			StreamMaxBytes: 10 * 1024 * 1024 * 1024,
			StreamMaxMsgs:  1_000_000,
			StreamReplicas: 1,
		},
		Embedded: EmbeddedNATSConfig{
			Host: "127.0.0.1",
			Port: 4222,
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
		{
			"endpoints accepted in external mode",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = nil
				c.Endpoints = []EndpointConfig{{URL: "nats://a:4222", Priority: 10}}
			},
			"",
		},
		{
			"urls and endpoints mutually exclusive",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = []string{"nats://a:4222"}
				c.Endpoints = []EndpointConfig{{URL: "nats://b:4222"}}
			},
			"mutually exclusive",
		},
		{
			"external requires urls or endpoints",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = nil
				c.Endpoints = nil
			},
			"at least one",
		},
		{
			"embedded forbids endpoints",
			func(c *NATSConfig) {
				c.Endpoints = []EndpointConfig{{URL: "nats://x:4222"}}
			},
			"endpoints",
		},
		{
			"endpoint URL empty rejected",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = nil
				c.Endpoints = []EndpointConfig{{URL: ""}}
			},
			"url",
		},
		{
			"endpoint weight negative rejected",
			func(c *NATSConfig) {
				c.Mode = NATSModeExternal
				c.URLs = nil
				c.Endpoints = []EndpointConfig{{URL: "nats://a", Weight: -1}}
			},
			"weight",
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

func TestDedupConfig_Validate(t *testing.T) {
	good := DedupConfig{
		Enabled:         true,
		WindowDuration:  5 * time.Minute,
		MaxEntries:      100,
		CleanupInterval: 30 * time.Second,
	}
	tests := []struct {
		name    string
		mut     func(*DedupConfig)
		wantErr string
	}{
		{"defaults ok", func(*DedupConfig) {}, ""},
		{"disabled skips checks", func(c *DedupConfig) {
			c.Enabled = false
			c.WindowDuration = 0
			c.MaxEntries = 0
			c.CleanupInterval = 0
		}, ""},
		{"window zero", func(c *DedupConfig) { c.WindowDuration = 0 }, "windowduration"},
		{"window negative", func(c *DedupConfig) { c.WindowDuration = -time.Second }, "windowduration"},
		{"maxentries zero", func(c *DedupConfig) { c.MaxEntries = 0 }, "maxentries"},
		{"cleanup zero", func(c *DedupConfig) { c.CleanupInterval = 0 }, "cleanupinterval"},
		{"override empty prefix", func(c *DedupConfig) {
			c.PerSubjectOverrides = []SubjectOverride{{Prefix: "", WindowDuration: time.Second}}
		}, "prefix"},
		{"override zero window", func(c *DedupConfig) {
			c.PerSubjectOverrides = []SubjectOverride{{Prefix: "kscore.x.", WindowDuration: 0}}
		}, "windowduration"},
		{"valid override", func(c *DedupConfig) {
			c.PerSubjectOverrides = []SubjectOverride{{Prefix: "kscore.x.", WindowDuration: time.Second}}
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

func TestValidateExternalURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"localhost with port", "nats://localhost:4222", ""},
		{"ipv4 with port", "nats://127.0.0.1:4222", ""},
		{"ipv4 no port", "nats://127.0.0.1", ""},
		{"hostname no port", "nats://localhost", ""},
		{"ipv6 bracketed with port", "nats://[::1]:4222", ""},
		{"ipv6 bracketed no port", "nats://[::1]", ""},
		{"ipv6 zone id", "nats://[fe80::1%25eth0]:4222", ""},
		{"tls scheme accepted", "tls://nats.example.com:4222", ""},

		{"empty", "", "missing host"},
		{"unbracketed ipv6 with port", "nats://::1:4222", "unbracketed IPv6"},
		{"unbracketed ipv6 multi-segment", "nats://2001:db8::1:4222", "unbracketed IPv6"},
		{"unbracketed all-zeros", "nats://::1:4222", "unbracketed IPv6"},
		{"missing host", "nats://", "missing host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalURL(tt.raw)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateExternalURL(%q) = %v, want nil", tt.raw, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateExternalURL(%q) = nil, want error containing %q", tt.raw, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNATSConfig_RejectsUnbracketedIPv6URL(t *testing.T) {
	cfg := validNATS()
	cfg.Mode = NATSModeExternal
	cfg.URLs = []string{"nats://::1:4222"}
	cfg.Endpoints = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted unbracketed IPv6 URL")
	}
	if !strings.Contains(err.Error(), "unbracketed IPv6") {
		t.Errorf("err = %v, want containing 'unbracketed IPv6'", err)
	}
}

func TestNATSConfig_AcceptsBracketedIPv6URL(t *testing.T) {
	cfg := validNATS()
	cfg.Mode = NATSModeExternal
	cfg.URLs = []string{"nats://[::1]:4222"}
	cfg.Endpoints = nil

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate = %v, want nil for bracketed IPv6", err)
	}
}

func TestNATSConfig_RejectsUnbracketedIPv6Endpoint(t *testing.T) {
	cfg := validNATS()
	cfg.Mode = NATSModeExternal
	cfg.URLs = nil
	cfg.Endpoints = []EndpointConfig{
		{URL: "nats://2001:db8::1:4222"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted endpoint with unbracketed IPv6")
	}
	if !strings.Contains(err.Error(), "unbracketed IPv6") {
		t.Errorf("err = %v, want containing 'unbracketed IPv6'", err)
	}
}

func TestBootstrapConfig_Validate(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	good := BootstrapConfig{
		Enabled: true,
		PSKs: []BootstrapPSK{
			{AgentID: "agent-1", Secret: "deadbeef", ExpiresAt: exp},
		},
	}
	tests := []struct {
		name    string
		mut     func(*BootstrapConfig)
		wantErr string
	}{
		{"defaults ok", func(*BootstrapConfig) {}, ""},
		{"disabled skips checks", func(c *BootstrapConfig) {
			c.Enabled = false
			c.PSKs[0].AgentID = "" // would fail when enabled, but we're not
		}, ""},
		{"empty psks ok when enabled", func(c *BootstrapConfig) {
			c.PSKs = nil
		}, ""},
		{"agentid empty", func(c *BootstrapConfig) {
			c.PSKs[0].AgentID = ""
		}, "agentid"},
		{"secret empty", func(c *BootstrapConfig) {
			c.PSKs[0].Secret = ""
		}, "secret"},
		{"secret not hex", func(c *BootstrapConfig) {
			c.PSKs[0].Secret = "not-hex-zzz"
		}, "not hex"},
		{"expires zero", func(c *BootstrapConfig) {
			c.PSKs[0].ExpiresAt = time.Time{}
		}, "expiresat"},
		{"duplicate agent ID", func(c *BootstrapConfig) {
			c.PSKs = []BootstrapPSK{
				{AgentID: "agent-1", Secret: "deadbeef", ExpiresAt: exp},
				{AgentID: "agent-1", Secret: "cafebabe", ExpiresAt: exp},
			}
		}, "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := BootstrapConfig{
				Enabled: good.Enabled,
				PSKs:    append([]BootstrapPSK(nil), good.PSKs...),
			}
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

func TestJetStreamConfig_StreamDefaults(t *testing.T) {
	good := JetStreamConfig{
		Enabled:        true,
		StoreDir:       "/tmp/js",
		MaxStorage:     1024,
		StreamMaxAge:   time.Hour,
		StreamMaxBytes: 1024,
		StreamMaxMsgs:  1024,
		StreamReplicas: 1,
	}
	tests := []struct {
		name    string
		mut     func(*JetStreamConfig)
		wantErr string
	}{
		{"defaults ok", func(*JetStreamConfig) {}, ""},
		{"streammaxage zero", func(c *JetStreamConfig) { c.StreamMaxAge = 0 }, "streammaxage"},
		{"streammaxbytes zero", func(c *JetStreamConfig) { c.StreamMaxBytes = 0 }, "streammaxbytes"},
		{"streammaxmsgs zero", func(c *JetStreamConfig) { c.StreamMaxMsgs = 0 }, "streammaxmsgs"},
		{"streamreplicas zero", func(c *JetStreamConfig) { c.StreamReplicas = 0 }, "streamreplicas"},
		{"streamreplicas too high", func(c *JetStreamConfig) { c.StreamReplicas = 6 }, "streamreplicas"},
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

func TestJetStreamSafeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"default", "default"},
		{"prod-east", "prod-east"},
		{"alpha_1", "alpha_1"},
		{"prod.east", "prod_east"},
		{"prod east", "prod_east"},
		{"prod>east", "prod_east"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := JetStreamSafeName(tt.in); got != tt.want {
			t.Errorf("JetStreamSafeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCircuitBreakerConfig_Validate(t *testing.T) {
	good := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    5,
		SuccessThreshold:    2,
		OpenDuration:        30 * time.Second,
		HalfOpenMaxAttempts: 3,
	}
	tests := []struct {
		name    string
		mut     func(*CircuitBreakerConfig)
		wantErr string
	}{
		{"defaults ok", func(*CircuitBreakerConfig) {}, ""},
		{"disabled skips checks", func(c *CircuitBreakerConfig) {
			c.Enabled = false
			c.FailureThreshold = 0
			c.SuccessThreshold = 0
			c.OpenDuration = 0
			c.HalfOpenMaxAttempts = 0
		}, ""},
		{"failure threshold zero", func(c *CircuitBreakerConfig) { c.FailureThreshold = 0 }, "failurethreshold"},
		{"failure threshold negative", func(c *CircuitBreakerConfig) { c.FailureThreshold = -1 }, "failurethreshold"},
		{"success threshold zero", func(c *CircuitBreakerConfig) { c.SuccessThreshold = 0 }, "successthreshold"},
		{"open duration zero", func(c *CircuitBreakerConfig) { c.OpenDuration = 0 }, "openduration"},
		{"half-open max zero", func(c *CircuitBreakerConfig) { c.HalfOpenMaxAttempts = 0 }, "halfopenmaxattempts"},
		{"half-open max less than success threshold", func(c *CircuitBreakerConfig) {
			c.HalfOpenMaxAttempts = 1
			c.SuccessThreshold = 2
		}, "halfopenmaxattempts"},
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
	if cfg.NATS.JetStream.StreamMaxAge != 7*24*time.Hour {
		t.Errorf("JetStream.StreamMaxAge = %s, want 7d", cfg.NATS.JetStream.StreamMaxAge)
	}
	if cfg.NATS.JetStream.StreamMaxBytes != 10*1024*1024*1024 {
		t.Errorf("JetStream.StreamMaxBytes = %d, want 10 GiB", cfg.NATS.JetStream.StreamMaxBytes)
	}
	if cfg.NATS.JetStream.StreamMaxMsgs != 1_000_000 {
		t.Errorf("JetStream.StreamMaxMsgs = %d, want 1M", cfg.NATS.JetStream.StreamMaxMsgs)
	}
	if cfg.NATS.JetStream.StreamReplicas != 1 {
		t.Errorf("JetStream.StreamReplicas = %d, want 1", cfg.NATS.JetStream.StreamReplicas)
	}
	if cfg.NATS.MaxReconnects != 60 {
		t.Errorf("MaxReconnects = %d, want 60", cfg.NATS.MaxReconnects)
	}
	if cfg.NATS.ReconnectWait != 2*time.Second {
		t.Errorf("ReconnectWait = %s, want 2s", cfg.NATS.ReconnectWait)
	}
	if !cfg.NATS.Dedup.Enabled {
		t.Error("Dedup.Enabled = false, want true (PROJECT-DETAILS §4.2)")
	}
	if cfg.NATS.Dedup.WindowDuration != 5*time.Minute {
		t.Errorf("Dedup.WindowDuration = %s, want 5m", cfg.NATS.Dedup.WindowDuration)
	}
	if cfg.NATS.Dedup.MaxEntries != 100_000 {
		t.Errorf("Dedup.MaxEntries = %d, want 100000", cfg.NATS.Dedup.MaxEntries)
	}
	if cfg.NATS.Dedup.CleanupInterval != 30*time.Second {
		t.Errorf("Dedup.CleanupInterval = %s, want 30s", cfg.NATS.Dedup.CleanupInterval)
	}
	if !cfg.NATS.CircuitBreaker.Enabled {
		t.Error("CircuitBreaker.Enabled = false, want true")
	}
	if cfg.NATS.CircuitBreaker.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", cfg.NATS.CircuitBreaker.FailureThreshold)
	}
	if cfg.NATS.CircuitBreaker.SuccessThreshold != 2 {
		t.Errorf("SuccessThreshold = %d, want 2", cfg.NATS.CircuitBreaker.SuccessThreshold)
	}
	if cfg.NATS.CircuitBreaker.OpenDuration != 30*time.Second {
		t.Errorf("OpenDuration = %s, want 30s", cfg.NATS.CircuitBreaker.OpenDuration)
	}
	if cfg.NATS.CircuitBreaker.HalfOpenMaxAttempts != 3 {
		t.Errorf("HalfOpenMaxAttempts = %d, want 3", cfg.NATS.CircuitBreaker.HalfOpenMaxAttempts)
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
