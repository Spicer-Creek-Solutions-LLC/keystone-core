// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"
	"time"
)

// Exporter kinds. The set is closed; Validate rejects anything else.
const (
	TracingExporterStdout   = "stdout"
	TracingExporterOTLPGRPC = "otlp_grpc"
	TracingExporterOTLPHTTP = "otlp_http"
	TracingExporterZipkin   = "zipkin"
)

// Sampler kinds. The set is closed; Validate rejects anything else.
const (
	TracingSamplerAlwaysOn     = "always_on"
	TracingSamplerAlwaysOff    = "always_off"
	TracingSamplerProbabilistic = "probabilistic"
	TracingSamplerParentBased  = "parent_based"
	TracingSamplerRateLimiting = "rate_limiting"
)

// TracingConfig configures the Epic 17 task 4 OTel trace pipeline.
// Disabled by default — 100% sampling adds 5-10% latency at scale
// (PROJECT-DETAILS §4.16 gotcha), so operators opt in explicitly.
type TracingConfig struct {
	// Enabled toggles the whole pipeline. Default false. When false,
	// internal/tracing.New returns a noop TracerProvider.
	Enabled bool `koanf:"enabled"`

	// ServiceName is the resource attribute service.name. Default
	// "kscore-server"; per-binary wiring may override (e.g.
	// "kscore-agent" in the agent runtime).
	ServiceName string `koanf:"servicename"`

	// Exporter selects the span sink. One of stdout / otlp_grpc /
	// otlp_http / zipkin. Default stdout — operator-visible without
	// needing a collector.
	Exporter string `koanf:"exporter"`

	// Endpoint is the collector URL (or addr:port for OTLP gRPC).
	// Required when Exporter is not stdout; ignored for stdout.
	Endpoint string `koanf:"endpoint"`

	// Insecure disables TLS for OTLP exporters. Stdout/Zipkin ignore
	// this. Default false (TLS required) so production-wired collectors
	// don't accidentally negotiate plaintext.
	Insecure bool `koanf:"insecure"`

	// Sampler selects the sampling strategy. Default probabilistic at
	// SampleRate. Adaptive is intentionally NOT supported in v1.0 —
	// see ROADMAP entry for the v2.x+ adaptive-sampling work.
	Sampler string `koanf:"sampler"`

	// SampleRate is the [0,1] sampling fraction for the probabilistic
	// sampler and the probabilistic fallback inside parent_based.
	// Default 0.1 (PROJECT-DETAILS §4.16 line 1126).
	SampleRate float64 `koanf:"samplerate"`

	// RateLimitPerSecond bounds the rate_limiting sampler's accepts.
	// Default 100. Above-rate spans return Drop. Honors parent decisions
	// per OTel convention.
	RateLimitPerSecond int `koanf:"ratelimitpersecond"`

	// BatchSize is the sdktrace batch processor MaxExportBatchSize.
	// Default 512.
	BatchSize int `koanf:"batchsize"`

	// QueueSize is the sdktrace batch processor MaxQueueSize. Default
	// 2048. Must be >= BatchSize.
	QueueSize int `koanf:"queuesize"`

	// FlushInterval is the sdktrace batch processor BatchTimeout —
	// the longest a span will wait before being exported. Default 5s.
	FlushInterval time.Duration `koanf:"flushinterval"`
}

// Validate rejects empty / out-of-range fields when tracing is enabled.
// Disabled configs pass trivially so dev fixtures can omit every field.
func (t TracingConfig) Validate() error {
	if !t.Enabled {
		return nil
	}
	switch t.Exporter {
	case TracingExporterStdout:
		// no endpoint required
	case TracingExporterOTLPGRPC, TracingExporterOTLPHTTP, TracingExporterZipkin:
		if strings.TrimSpace(t.Endpoint) == "" {
			return fmt.Errorf("tracing.endpoint: required when exporter=%q", t.Exporter)
		}
	default:
		return fmt.Errorf("tracing.exporter: %q is not one of %s", t.Exporter,
			strings.Join([]string{
				TracingExporterStdout, TracingExporterOTLPGRPC,
				TracingExporterOTLPHTTP, TracingExporterZipkin,
			}, "/"))
	}
	switch t.Sampler {
	case TracingSamplerAlwaysOn, TracingSamplerAlwaysOff,
		TracingSamplerProbabilistic, TracingSamplerParentBased,
		TracingSamplerRateLimiting:
		// ok
	case "adaptive":
		// Explicit error so operators reading old docs / config get a
		// pointer instead of a confusing "not one of …" enum miss.
		return fmt.Errorf("tracing.sampler: %q is deferred to v2.x+; see ROADMAP entry "+
			"\"Adaptive sampling tied to error metrics\"", t.Sampler)
	default:
		return fmt.Errorf("tracing.sampler: %q is not one of %s", t.Sampler,
			strings.Join([]string{
				TracingSamplerAlwaysOn, TracingSamplerAlwaysOff,
				TracingSamplerProbabilistic, TracingSamplerParentBased,
				TracingSamplerRateLimiting,
			}, "/"))
	}
	if t.SampleRate < 0 || t.SampleRate > 1 {
		return fmt.Errorf("tracing.samplerate: must be in [0,1], got %v", t.SampleRate)
	}
	if t.RateLimitPerSecond < 0 {
		return fmt.Errorf("tracing.ratelimitpersecond: must be >= 0, got %d", t.RateLimitPerSecond)
	}
	if t.BatchSize <= 0 {
		return fmt.Errorf("tracing.batchsize: must be > 0, got %d", t.BatchSize)
	}
	if t.QueueSize < t.BatchSize {
		return fmt.Errorf("tracing.queuesize: %d must be >= batchsize %d", t.QueueSize, t.BatchSize)
	}
	if t.FlushInterval <= 0 {
		return fmt.Errorf("tracing.flushinterval: must be > 0, got %s", t.FlushInterval)
	}
	if t.ServiceName == "" {
		return fmt.Errorf("tracing.servicename: must not be empty")
	}
	return nil
}
