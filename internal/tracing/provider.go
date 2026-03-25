package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.28.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials/insecure"
)

// Provider wraps the OpenTelemetry tracer provider
type Provider struct {
	provider *sdktrace.TracerProvider
	config   *Config
}

// NewProvider creates a new tracing provider with the given configuration
func NewProvider(cfg *Config) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if !cfg.Enabled {
		// Return a provider with no-op tracer
		return &Provider{
			provider: sdktrace.NewTracerProvider(),
			config:   cfg,
		}, nil
	}

	// Create resource with service information
	res, err := newResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	sampler := newSampler(cfg.Sampling)

	// Create span processor options
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	// Create and add exporters
	for _, exporterCfg := range cfg.Exporters {
		exporter, err := newExporter(exporterCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create exporter %s: %w", exporterCfg.Type, err)
		}

		// Create batch span processor
		batchOpts := []sdktrace.BatchSpanProcessorOption{}
		if exporterCfg.BatchTimeout > 0 {
			batchOpts = append(batchOpts, sdktrace.WithBatchTimeout(exporterCfg.BatchTimeout))
		}
		if exporterCfg.MaxExportBatchSize > 0 {
			batchOpts = append(batchOpts, sdktrace.WithMaxExportBatchSize(exporterCfg.MaxExportBatchSize))
		}
		if exporterCfg.MaxQueueSize > 0 {
			batchOpts = append(batchOpts, sdktrace.WithMaxQueueSize(exporterCfg.MaxQueueSize))
		}

		processor := sdktrace.NewBatchSpanProcessor(exporter, batchOpts...)
		opts = append(opts, sdktrace.WithSpanProcessor(processor))
	}

	provider := sdktrace.NewTracerProvider(opts...)

	// Set as global provider
	otel.SetTracerProvider(provider)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		provider: provider,
		config:   cfg,
	}, nil
}

// Tracer returns a tracer for the given instrumentation scope
func (p *Provider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return p.provider.Tracer(name, opts...)
}

// Shutdown flushes any remaining spans and shuts down the provider
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.provider == nil {
		return nil
	}
	return p.provider.Shutdown(ctx)
}

// ForceFlush flushes any buffered spans
func (p *Provider) ForceFlush(ctx context.Context) error {
	if p.provider == nil {
		return nil
	}
	return p.provider.ForceFlush(ctx)
}

// newResource creates a resource with service information
func newResource(cfg *Config) (*resource.Resource, error) {
	attrs := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	}

	// Add custom resource attributes
	if len(cfg.ResourceAttributes) > 0 {
		for k, v := range cfg.ResourceAttributes {
			attrs = append(attrs, resource.WithAttributes(
				StringAttr(k, v),
			))
		}
	}

	return resource.New(context.Background(), attrs...)
}

// newSampler creates a sampler based on the configuration
func newSampler(cfg SamplingConfig) sdktrace.Sampler {
	switch cfg.Type {
	case SamplingAlwaysOn:
		return sdktrace.AlwaysSample()

	case SamplingAlwaysOff:
		return sdktrace.NeverSample()

	case SamplingProbabilistic:
		// Ensure rate is between 0 and 1
		rate := cfg.Rate
		if rate < 0 {
			rate = 0
		}
		if rate > 1 {
			rate = 1
		}
		return sdktrace.TraceIDRatioBased(rate)

	case SamplingParentBased:
		// Parent-based with probabilistic fallback
		rate := cfg.Rate
		if rate < 0 {
			rate = 0.1 // Default to 10%
		}
		if rate > 1 {
			rate = 1
		}
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(rate))

	default:
		// Default to parent-based with 10% sampling
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))
	}
}

// newExporter creates a span exporter based on the configuration
func newExporter(cfg ExporterConfig) (sdktrace.SpanExporter, error) {
	switch cfg.Type {
	case ExporterOTLP:
		return newOTLPExporter(cfg)

	case ExporterOTLPHTTP:
		return newOTLPHTTPExporter(cfg)

	case ExporterZipkin:
		return newZipkinExporter(cfg)

	case ExporterStdout:
		return newStdoutExporter(cfg)

	case ExporterNone:
		// Return a no-op exporter
		return &noopExporter{}, nil

	default:
		return nil, fmt.Errorf("unknown exporter type: %s", cfg.Type)
	}
}

// newOTLPExporter creates an OTLP gRPC exporter
func newOTLPExporter(cfg ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	if cfg.Timeout > 0 {
		opts = append(opts, otlptracegrpc.WithTimeout(cfg.Timeout))
	}

	if cfg.Compression == "gzip" {
		opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
	}

	// Connect lazily — do not use grpc.WithBlock() which can hang
	// indefinitely if the collector is unreachable at startup
	client := otlptracegrpc.NewClient(opts...)

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return otlptrace.New(connectCtx, client)
}

// newOTLPHTTPExporter creates an OTLP HTTP exporter
func newOTLPHTTPExporter(cfg ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	if cfg.Timeout > 0 {
		opts = append(opts, otlptracehttp.WithTimeout(cfg.Timeout))
	}

	if cfg.Compression == "gzip" {
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	client := otlptracehttp.NewClient(opts...)
	return otlptrace.New(context.Background(), client)
}

// newZipkinExporter creates a Zipkin exporter
func newZipkinExporter(cfg ExporterConfig) (sdktrace.SpanExporter, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:9411/api/v2/spans"
	}

	return zipkin.New(endpoint)
}

// newStdoutExporter creates a stdout exporter (for debugging)
func newStdoutExporter(cfg ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []stdouttrace.Option{
		stdouttrace.WithPrettyPrint(),
	}

	return stdouttrace.New(opts...)
}

// noopExporter is a no-op exporter that does nothing
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}

// DefaultConfig returns a default tracing configuration
func DefaultConfig(serviceName string) *Config {
	return &Config{
		Enabled:        false, // Disabled by default
		ServiceName:    serviceName,
		ServiceVersion: "unknown",
		Environment:    "development",
		Sampling: SamplingConfig{
			Type: SamplingParentBased,
			Rate: 0.1, // Sample 10% of traces
		},
		Exporters:          []ExporterConfig{},
		ResourceAttributes: map[string]string{},
	}
}

// NewOTLPConfig creates a config with OTLP exporter
func NewOTLPConfig(serviceName, endpoint string, useInsecure bool) *Config {
	cfg := DefaultConfig(serviceName)
	cfg.Enabled = true
	cfg.Exporters = []ExporterConfig{
		{
			Type:               ExporterOTLP,
			Endpoint:           endpoint,
			Insecure:           useInsecure,
			Timeout:            10 * time.Second,
			Compression:        "gzip",
			BatchTimeout:       5 * time.Second,
			MaxExportBatchSize: 512,
			MaxQueueSize:       2048,
		},
	}
	return cfg
}

// NewZipkinConfig creates a config with Zipkin exporter
func NewZipkinConfig(serviceName, endpoint string) *Config {
	cfg := DefaultConfig(serviceName)
	cfg.Enabled = true
	cfg.Exporters = []ExporterConfig{
		{
			Type:         ExporterZipkin,
			Endpoint:     endpoint,
			BatchTimeout: 5 * time.Second,
		},
	}
	return cfg
}

// NewStdoutConfig creates a config with stdout exporter (for debugging)
func NewStdoutConfig(serviceName string) *Config {
	cfg := DefaultConfig(serviceName)
	cfg.Enabled = true
	cfg.Sampling.Type = SamplingAlwaysOn // Sample everything for debugging
	cfg.Exporters = []ExporterConfig{
		{
			Type: ExporterStdout,
		},
	}
	return cfg
}
