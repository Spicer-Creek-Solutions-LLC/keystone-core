package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "disabled tracing",
			config: &Config{
				Enabled:     false,
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "stdout exporter",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Environment:    "test",
				Sampling: SamplingConfig{
					Type: SamplingAlwaysOn,
				},
				Exporters: []ExporterConfig{
					{Type: ExporterStdout},
				},
			},
			wantErr: false,
		},
		{
			name: "no exporters",
			config: &Config{
				Enabled:        true,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Environment:    "test",
				Sampling: SamplingConfig{
					Type: SamplingAlwaysOn,
				},
				Exporters: []ExporterConfig{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && provider != nil {
				defer provider.Shutdown(context.Background())
			}
		})
	}
}

func TestProvider_Tracer(t *testing.T) {
	config := DefaultConfig("test-service")
	config.Enabled = true
	config.Exporters = []ExporterConfig{{Type: ExporterStdout}}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test-tracer")
	if tracer == nil {
		t.Error("Tracer() returned nil")
	}

	// Test span creation
	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")
	if span == nil {
		t.Error("Start() returned nil span")
	}
	span.End()
}

func TestProvider_Shutdown(t *testing.T) {
	config := DefaultConfig("test-service")
	config.Enabled = true
	config.Exporters = []ExporterConfig{{Type: ExporterStdout}}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = provider.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test-service")

	if config.Enabled {
		t.Error("DefaultConfig() should have tracing disabled")
	}

	if config.ServiceName != "test-service" {
		t.Errorf("ServiceName = %v, want test-service", config.ServiceName)
	}

	if config.Sampling.Type != SamplingParentBased {
		t.Errorf("Sampling.Type = %v, want %v", config.Sampling.Type, SamplingParentBased)
	}
}

func TestNewOTLPConfig(t *testing.T) {
	config := NewOTLPConfig("test-service", "localhost:4317", true)

	if !config.Enabled {
		t.Error("NewOTLPConfig() should have tracing enabled")
	}

	if len(config.Exporters) != 1 {
		t.Errorf("Exporters length = %v, want 1", len(config.Exporters))
	}

	if config.Exporters[0].Type != ExporterOTLP {
		t.Errorf("Exporter type = %v, want %v", config.Exporters[0].Type, ExporterOTLP)
	}

	if config.Exporters[0].Endpoint != "localhost:4317" {
		t.Errorf("Endpoint = %v, want localhost:4317", config.Exporters[0].Endpoint)
	}

	if !config.Exporters[0].Insecure {
		t.Error("Insecure should be true")
	}
}

func TestNewZipkinConfig(t *testing.T) {
	config := NewZipkinConfig("test-service", "http://localhost:9411/api/v2/spans")

	if !config.Enabled {
		t.Error("NewZipkinConfig() should have tracing enabled")
	}

	if len(config.Exporters) != 1 {
		t.Errorf("Exporters length = %v, want 1", len(config.Exporters))
	}

	if config.Exporters[0].Type != ExporterZipkin {
		t.Errorf("Exporter type = %v, want %v", config.Exporters[0].Type, ExporterZipkin)
	}
}

func TestNewStdoutConfig(t *testing.T) {
	config := NewStdoutConfig("test-service")

	if !config.Enabled {
		t.Error("NewStdoutConfig() should have tracing enabled")
	}

	if config.Sampling.Type != SamplingAlwaysOn {
		t.Errorf("Sampling.Type = %v, want %v", config.Sampling.Type, SamplingAlwaysOn)
	}

	if len(config.Exporters) != 1 {
		t.Errorf("Exporters length = %v, want 1", len(config.Exporters))
	}

	if config.Exporters[0].Type != ExporterStdout {
		t.Errorf("Exporter type = %v, want %v", config.Exporters[0].Type, ExporterStdout)
	}
}

func TestSampling(t *testing.T) {
	tests := []struct {
		name    string
		config  SamplingConfig
		wantErr bool
	}{
		{
			name: "always on",
			config: SamplingConfig{
				Type: SamplingAlwaysOn,
			},
			wantErr: false,
		},
		{
			name: "always off",
			config: SamplingConfig{
				Type: SamplingAlwaysOff,
			},
			wantErr: false,
		},
		{
			name: "probabilistic valid rate",
			config: SamplingConfig{
				Type: SamplingProbabilistic,
				Rate: 0.5,
			},
			wantErr: false,
		},
		{
			name: "probabilistic rate too high",
			config: SamplingConfig{
				Type: SamplingProbabilistic,
				Rate: 1.5,
			},
			wantErr: false, // Should be clamped to 1.0
		},
		{
			name: "probabilistic rate too low",
			config: SamplingConfig{
				Type: SamplingProbabilistic,
				Rate: -0.5,
			},
			wantErr: false, // Should be clamped to 0.0
		},
		{
			name: "parent based",
			config: SamplingConfig{
				Type: SamplingParentBased,
				Rate: 0.1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig("test-service")
			config.Enabled = true
			config.Sampling = tt.config
			config.Exporters = []ExporterConfig{{Type: ExporterStdout}}

			provider, err := NewProvider(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if provider != nil {
				defer provider.Shutdown(context.Background())
			}
		})
	}
}

func TestResourceAttributes(t *testing.T) {
	config := DefaultConfig("test-service")
	config.Enabled = true
	config.ServiceVersion = "1.2.3"
	config.Environment = "production"
	config.ResourceAttributes = map[string]string{
		"custom.attr1": "value1",
		"custom.attr2": "value2",
	}
	config.Exporters = []ExporterConfig{{Type: ExporterStdout}}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	defer provider.Shutdown(context.Background())

	// Create a span to verify attributes are set
	tracer := provider.Tracer("test-tracer")
	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")
	defer span.End()

	// Just verify it doesn't crash - actual attribute verification would require
	// a custom exporter to inspect the spans
}

func TestExporterTypes(t *testing.T) {
	tests := []struct {
		name     string
		exporter ExporterConfig
		wantErr  bool
	}{
		{
			name: "stdout exporter",
			exporter: ExporterConfig{
				Type: ExporterStdout,
			},
			wantErr: false,
		},
		{
			name: "none exporter",
			exporter: ExporterConfig{
				Type: ExporterNone,
			},
			wantErr: false,
		},
		// Note: OTLP and Zipkin exporters would require actual endpoints
		// so we skip them in unit tests
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig("test-service")
			config.Enabled = true
			config.Exporters = []ExporterConfig{tt.exporter}

			provider, err := NewProvider(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if provider != nil {
				defer provider.Shutdown(context.Background())
			}
		})
	}
}

func TestSpanCreation(t *testing.T) {
	config := NewStdoutConfig("test-service")
	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test-tracer")
	ctx := context.Background()

	// Create a span
	_, span := tracer.Start(ctx, "test-operation")

	// Set status
	span.SetStatus(codes.Ok, "success")

	// Add attributes
	span.SetAttributes(StringAttr("test.key", "test.value"))

	// Add event
	span.AddEvent("test-event")

	span.End()

	// If we get here without panic, the test passes
}
