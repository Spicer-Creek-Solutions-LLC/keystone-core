package gateway

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	// DefaultConfig has logs enabled but no output configured
	// This is intentional to show users they need to configure an output
	// Disable logs for this test to validate the rest of the config
	cfg.Logs.Enabled = false

	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig (with logs disabled) should be valid, got: %v", err)
	}
}

func TestDefaultConfigWarnsAboutLogsOutput(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	if err == nil {
		t.Error("DefaultConfig with logs enabled should warn about missing output")
		return
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	found := false
	for _, e := range errs {
		if e.Field == "logs" && containsSubstring(e.Message, "no output") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about logs output not configured, got: %v", err)
	}
}

func TestConfigValidate_RequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		wantField string
	}{
		{
			name: "missing NATS URLs",
			modify: func(c *Config) {
				c.NATS.URLs = nil
			},
			wantField: "nats.urls",
		},
		{
			name: "missing cluster name",
			modify: func(c *Config) {
				c.NATS.Cluster = ""
			},
			wantField: "nats.cluster",
		},
		{
			name: "missing server listen",
			modify: func(c *Config) {
				c.Server.Listen = ""
			},
			wantField: "server.listen",
		},
		{
			name: "missing metrics path",
			modify: func(c *Config) {
				c.Server.MetricsPath = ""
			},
			wantField: "server.metrics_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}

			var errs ValidationErrors
			if !errors.As(err, &errs) {
				t.Fatalf("expected ValidationErrors, got %T", err)
			}

			found := false
			for _, e := range errs {
				if e.Field == tt.wantField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %s, got: %v", tt.wantField, err)
			}
		})
	}
}

func TestConfigValidate_InvalidNATSURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid nats", "nats://localhost:4222", false},
		{"valid tls", "tls://localhost:4222", false},
		{"valid ws", "ws://localhost:4222", false},
		{"valid wss", "wss://localhost:4222", false},
		{"invalid scheme", "http://localhost:4222", true},
		{"missing host", "nats://", true},
		{"no scheme", "localhost:4222", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.NATS.URLs = []string{tt.url}
			// Disable logs to avoid unrelated validation errors
			cfg.Logs.Enabled = false

			err := cfg.Validate()

			// Check specifically for NATS URL errors
			hasNATSErr := false
			var errs ValidationErrors
			if errors.As(err, &errs) {
				for _, e := range errs {
					if e.Field == "nats.urls[0]" {
						hasNATSErr = true
						break
					}
				}
			}

			if hasNATSErr != tt.wantErr {
				t.Errorf("wantErr = %v, got hasNATSErr = %v, err = %v", tt.wantErr, hasNATSErr, err)
			}
		})
	}
}

func TestConfigValidate_TLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		tls       TLSConfig
		wantField string
	}{
		{
			name: "cert without key",
			tls: TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert",
			},
			wantField: "nats.tls.key_file",
		},
		{
			name: "key without cert",
			tls: TLSConfig{
				Enabled: true,
				KeyFile: "/path/to/key",
			},
			wantField: "nats.tls.cert_file",
		},
		{
			name: "insecure mode (no cert needed)",
			tls: TLSConfig{
				Enabled:  true,
				Insecure: true,
			},
			wantField: "", // Should not error
		},
		{
			name: "valid full config",
			tls: TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert",
				KeyFile:  "/path/to/key",
			},
			wantField: "", // Should not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.NATS.TLS = tt.tls

			err := cfg.Validate()

			if tt.wantField == "" {
				if err != nil {
					// Check if error is from TLS config
					var errs ValidationErrors
			if errors.As(err, &errs) {
						for _, e := range errs {
							if e.Field == "nats.tls.key_file" || e.Field == "nats.tls.cert_file" {
								t.Errorf("unexpected TLS error: %v", e)
							}
						}
					}
				}
			} else {
				if err == nil {
					t.Fatal("expected validation error")
				}
				var errs ValidationErrors
				if !errors.As(err, &errs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
				found := false
				for _, e := range errs {
					if e.Field == tt.wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %s, got: %v", tt.wantField, err)
				}
			}
		})
	}
}

func TestConfigValidate_AuthConfig(t *testing.T) {
	tests := []struct {
		name      string
		auth      AuthConfig
		wantField string
	}{
		{
			name:      "none auth is valid",
			auth:      AuthConfig{Type: "none"},
			wantField: "",
		},
		{
			name:      "empty auth is valid",
			auth:      AuthConfig{},
			wantField: "",
		},
		{
			name: "basic auth missing username",
			auth: AuthConfig{
				Type:     "basic",
				Password: "secret",
			},
			wantField: "metrics.remote_write.auth.username",
		},
		{
			name: "basic auth missing password",
			auth: AuthConfig{
				Type:     "basic",
				Username: "user",
			},
			wantField: "metrics.remote_write.auth.password",
		},
		{
			name: "bearer auth missing token",
			auth: AuthConfig{
				Type: "bearer",
			},
			wantField: "metrics.remote_write.auth.token",
		},
		{
			name: "valid basic auth",
			auth: AuthConfig{
				Type:     "basic",
				Username: "user",
				Password: "secret",
			},
			wantField: "",
		},
		{
			name: "valid bearer auth",
			auth: AuthConfig{
				Type:  "bearer",
				Token: "mytoken",
			},
			wantField: "",
		},
		{
			name: "invalid auth type",
			auth: AuthConfig{
				Type: "oauth",
			},
			wantField: "metrics.remote_write.auth.type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Metrics.RemoteWrite.Enabled = true
			cfg.Metrics.RemoteWrite.URL = "http://localhost:9090/api/v1/write"
			cfg.Metrics.RemoteWrite.Auth = tt.auth

			err := cfg.Validate()

			if tt.wantField == "" {
				// Check no auth errors
				var errs ValidationErrors
			if errors.As(err, &errs) {
					for _, e := range errs {
						if e.Field == "metrics.remote_write.auth.username" ||
							e.Field == "metrics.remote_write.auth.password" ||
							e.Field == "metrics.remote_write.auth.token" ||
							e.Field == "metrics.remote_write.auth.type" {
							t.Errorf("unexpected auth error: %v", e)
						}
					}
				}
			} else {
				if err == nil {
					t.Fatal("expected validation error")
				}
				var errs ValidationErrors
				if !errors.As(err, &errs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
				found := false
				for _, e := range errs {
					if e.Field == tt.wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %s, got: %v", tt.wantField, err)
				}
			}
		})
	}
}

func TestConfigValidate_LogLevel(t *testing.T) {
	validLevels := []string{"", "debug", "info", "warn", "warning", "error", "fatal"}
	invalidLevels := []string{"trace", "critical", "verbose"}

	for _, level := range validLevels {
		t.Run("valid_"+level, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Logs.MinLevel = level
			cfg.Logs.Loki.Enabled = true
			cfg.Logs.Loki.URL = "http://localhost:3100"

			err := cfg.Validate()
			var errs ValidationErrors
			if errors.As(err, &errs) {
				for _, e := range errs {
					if e.Field == "logs.min_level" {
						t.Errorf("unexpected error for valid level %q: %v", level, e)
					}
				}
			}
		})
	}

	for _, level := range invalidLevels {
		t.Run("invalid_"+level, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Logs.MinLevel = level
			cfg.Logs.Loki.Enabled = true
			cfg.Logs.Loki.URL = "http://localhost:3100"

			err := cfg.Validate()
			if err == nil {
				t.Errorf("expected error for invalid level %q", level)
				return
			}

			var errs ValidationErrors
			if !errors.As(err, &errs) {
				return
			}
			found := false
			for _, e := range errs {
				if e.Field == "logs.min_level" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field logs.min_level with level %q", level)
			}
		})
	}
}

func TestConfigValidate_SamplingRate(t *testing.T) {
	tests := []struct {
		name    string
		rate    float64
		wantErr bool
	}{
		{"valid 0", 0.0, false},
		{"valid 0.5", 0.5, false},
		{"valid 1", 1.0, false},
		{"invalid negative", -0.1, true},
		{"invalid > 1", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Traces.Sampling.Rate = tt.rate

			err := cfg.Validate()
			hasRateErr := false
			var errs ValidationErrors
			if errors.As(err, &errs) {
				for _, e := range errs {
					if e.Field == "traces.sampling.rate" {
						hasRateErr = true
						break
					}
				}
			}

			if hasRateErr != tt.wantErr {
				t.Errorf("wantErr = %v, got hasRateErr = %v", tt.wantErr, hasRateErr)
			}
		})
	}
}

func TestConfigValidate_OTLPProtocol(t *testing.T) {
	validProtocols := []string{"", "grpc", "http", "http/protobuf"}
	invalidProtocols := []string{"tcp", "websocket"}

	for _, proto := range validProtocols {
		t.Run("valid_"+proto, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Traces.OTLP.Enabled = true
			cfg.Traces.OTLP.Endpoint = "localhost:4317"
			cfg.Traces.OTLP.Protocol = proto

			err := cfg.Validate()
			var errs ValidationErrors
			if errors.As(err, &errs) {
				for _, e := range errs {
					if e.Field == "traces.otlp.protocol" {
						t.Errorf("unexpected error for valid protocol %q: %v", proto, e)
					}
				}
			}
		})
	}

	for _, proto := range invalidProtocols {
		t.Run("invalid_"+proto, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Traces.OTLP.Enabled = true
			cfg.Traces.OTLP.Endpoint = "localhost:4317"
			cfg.Traces.OTLP.Protocol = proto

			err := cfg.Validate()
			if err == nil {
				t.Errorf("expected error for invalid protocol %q", proto)
				return
			}

			var errs ValidationErrors
			if !errors.As(err, &errs) {
				return
			}
			found := false
			for _, e := range errs {
				if e.Field == "traces.otlp.protocol" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field traces.otlp.protocol with protocol %q", proto)
			}
		})
	}
}

func TestConfigValidate_LeaderElection(t *testing.T) {
	tests := []struct {
		name          string
		leaseDuration time.Duration
		renewDeadline time.Duration
		wantErr       bool
		wantField     string
	}{
		{
			name:          "valid",
			leaseDuration: 15 * time.Second,
			renewDeadline: 10 * time.Second,
			wantErr:       false,
		},
		{
			name:          "renew >= lease",
			leaseDuration: 10 * time.Second,
			renewDeadline: 10 * time.Second,
			wantErr:       true,
			wantField:     "ha.leader_election.renew_deadline",
		},
		{
			name:          "renew > lease",
			leaseDuration: 10 * time.Second,
			renewDeadline: 15 * time.Second,
			wantErr:       true,
			wantField:     "ha.leader_election.renew_deadline",
		},
		{
			name:          "zero lease",
			leaseDuration: 0,
			renewDeadline: 10 * time.Second,
			wantErr:       true,
			wantField:     "ha.leader_election.lease_duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.HA.Enabled = true
			cfg.HA.LeaderElection.Enabled = true
			cfg.HA.LeaderElection.LeaseDuration = tt.leaseDuration
			cfg.HA.LeaderElection.RenewDeadline = tt.renewDeadline

			err := cfg.Validate()

			if !tt.wantErr {
				var errs ValidationErrors
			if errors.As(err, &errs) {
					for _, e := range errs {
						if e.Field == "ha.leader_election.lease_duration" ||
							e.Field == "ha.leader_election.renew_deadline" {
							t.Errorf("unexpected leader election error: %v", e)
						}
					}
				}
			} else {
				if err == nil {
					t.Fatal("expected validation error")
				}
				var errs ValidationErrors
				if !errors.As(err, &errs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
				found := false
				for _, e := range errs {
					if e.Field == tt.wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %s, got: %v", tt.wantField, err)
				}
			}
		})
	}
}

func TestConfigValidate_NoTelemetryEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Metrics.Enabled = false
	cfg.Logs.Enabled = false
	cfg.Traces.Enabled = false

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error when no telemetry is enabled")
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	found := false
	for _, e := range errs {
		if e.Field == "metrics/logs/traces" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about at least one telemetry type being required")
	}
}

func TestValidationErrors_String(t *testing.T) {
	errs := ValidationErrors{
		{Field: "field1", Message: "error1"},
		{Field: "field2", Message: "error2"},
	}

	str := errs.Error()
	if str == "" {
		t.Error("expected non-empty error string")
	}

	// Should contain count
	if !containsString(str, "2 validation errors") {
		t.Errorf("expected '2 validation errors' in output, got: %s", str)
	}

	// Should contain field names
	if !containsString(str, "field1") || !containsString(str, "field2") {
		t.Errorf("expected field names in output, got: %s", str)
	}
}

func TestValidationError_Single(t *testing.T) {
	errs := ValidationErrors{
		{Field: "single_field", Message: "single error"},
	}

	str := errs.Error()
	// Single error should not have count prefix
	if containsString(str, "validation errors") {
		t.Errorf("single error should not have count prefix, got: %s", str)
	}

	if !containsString(str, "single_field") {
		t.Errorf("expected field name in output, got: %s", str)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
