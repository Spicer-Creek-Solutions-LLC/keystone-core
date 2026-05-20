package config

import (
	"strings"
	"testing"
	"time"
)

func validBaseline() TracingConfig {
	return TracingConfig{
		Enabled:            true,
		ServiceName:        "kscore-server",
		Exporter:           TracingExporterStdout,
		Sampler:            TracingSamplerProbabilistic,
		SampleRate:         0.1,
		RateLimitPerSecond: 100,
		BatchSize:          512,
		QueueSize:          2048,
		FlushInterval:      5 * time.Second,
	}
}

func TestTracingConfig_Validate(t *testing.T) {
	otlp := func(mut func(*TracingConfig)) TracingConfig {
		c := validBaseline()
		c.Exporter = TracingExporterOTLPGRPC
		c.Endpoint = "localhost:4317"
		if mut != nil {
			mut(&c)
		}
		return c
	}

	tests := []struct {
		name    string
		cfg     TracingConfig
		wantErr string
	}{
		{"disabled passes trivially", TracingConfig{Enabled: false}, ""},
		{"stdout baseline", validBaseline(), ""},
		{"otlp grpc with endpoint", otlp(nil), ""},
		{"otlp missing endpoint", otlp(func(c *TracingConfig) { c.Endpoint = "" }), "endpoint: required"},
		{"otlp http", otlp(func(c *TracingConfig) { c.Exporter = TracingExporterOTLPHTTP }), ""},
		{"zipkin with endpoint", otlp(func(c *TracingConfig) { c.Exporter = TracingExporterZipkin; c.Endpoint = "http://zipkin:9411/api/v2/spans" }), ""},
		{"unknown exporter", otlp(func(c *TracingConfig) { c.Exporter = "kafka" }), "exporter: \"kafka\""},
		{"unknown sampler", otlp(func(c *TracingConfig) { c.Sampler = "weird" }), "sampler: \"weird\""},
		{"adaptive sampler -> deferred", otlp(func(c *TracingConfig) { c.Sampler = "adaptive" }), "deferred to v2.x+"},
		{"sample rate too high", otlp(func(c *TracingConfig) { c.SampleRate = 1.5 }), "samplerate: must be in [0,1]"},
		{"sample rate negative", otlp(func(c *TracingConfig) { c.SampleRate = -0.1 }), "samplerate: must be in [0,1]"},
		{"rate limit negative", otlp(func(c *TracingConfig) { c.RateLimitPerSecond = -1 }), "ratelimitpersecond"},
		{"batch zero", otlp(func(c *TracingConfig) { c.BatchSize = 0 }), "batchsize"},
		{"queue smaller than batch", otlp(func(c *TracingConfig) { c.BatchSize = 256; c.QueueSize = 100 }), "queuesize"},
		{"flush zero", otlp(func(c *TracingConfig) { c.FlushInterval = 0 }), "flushinterval"},
		{"empty service name", otlp(func(c *TracingConfig) { c.ServiceName = "" }), "servicename"},
		{"all samplers accepted", otlp(func(c *TracingConfig) { c.Sampler = TracingSamplerAlwaysOn }), ""},
		{"rate_limiting accepted", otlp(func(c *TracingConfig) { c.Sampler = TracingSamplerRateLimiting }), ""},
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

func TestDefaultConfig_HasTracingDefaults(t *testing.T) {
	c := defaultConfig()
	if c.Tracing.Enabled {
		t.Error("Tracing.Enabled default = true, want false (opt-in)")
	}
	if c.Tracing.ServiceName != "kscore-server" {
		t.Errorf("Tracing.ServiceName default = %q, want kscore-server", c.Tracing.ServiceName)
	}
	if c.Tracing.Sampler != TracingSamplerProbabilistic {
		t.Errorf("Tracing.Sampler default = %q, want probabilistic", c.Tracing.Sampler)
	}
	if c.Tracing.SampleRate != 0.1 {
		t.Errorf("Tracing.SampleRate default = %v, want 0.1", c.Tracing.SampleRate)
	}
}
