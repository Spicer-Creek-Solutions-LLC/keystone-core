// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin" //nolint:staticcheck // SA1019: epic 17 v1.0 names zipkin explicitly; ROADMAP v2.x+ entry tracks the OTLP migration before upstream removal in early 2027.
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.keystone-core.io/keystone-core/internal/config"
)

// shutdownFunc closes the exporter. Returned separately from the
// SpanExporter because the New() error path needs to roll back a
// half-built exporter without going through the SDK TracerProvider.
type shutdownFunc func(context.Context) error

// newExporter dispatches on cfg.Exporter. The returned shutdownFunc is
// idempotent and bounded by ctx; callers should hand it the same ctx
// they use for TracerProvider.Shutdown.
func newExporter(cfg config.TracingConfig, log *slog.Logger) (sdktrace.SpanExporter, shutdownFunc, error) {
	switch cfg.Exporter {
	case config.TracingExporterStdout:
		return newStdoutExporter()
	case config.TracingExporterOTLPGRPC:
		return newOTLPGRPCExporter(cfg)
	case config.TracingExporterOTLPHTTP:
		return newOTLPHTTPExporter(cfg)
	case config.TracingExporterZipkin:
		return newZipkinExporter(cfg, log)
	default:
		return nil, nil, fmt.Errorf("tracing: unknown exporter %q", cfg.Exporter)
	}
}

func newStdoutExporter() (sdktrace.SpanExporter, shutdownFunc, error) {
	exp, err := stdouttrace.New(
		stdouttrace.WithWriter(os.Stdout),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("stdouttrace: %w", err)
	}
	return exp, exp.Shutdown, nil
}

func newOTLPGRPCExporter(cfg config.TracingConfig) (sdktrace.SpanExporter, shutdownFunc, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(grpcDefaultTLS()))
	}
	exp, err := otlptrace.New(context.Background(), otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, nil, fmt.Errorf("otlptracegrpc: %w", err)
	}
	return exp, exp.Shutdown, nil
}

func newOTLPHTTPExporter(cfg config.TracingConfig) (sdktrace.SpanExporter, shutdownFunc, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exp, err := otlptrace.New(context.Background(), otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, nil, fmt.Errorf("otlptracehttp: %w", err)
	}
	return exp, exp.Shutdown, nil
}

func newZipkinExporter(cfg config.TracingConfig, log *slog.Logger) (sdktrace.SpanExporter, shutdownFunc, error) {
	_ = log // zipkin.WithLogger takes a *log.Logger; we accept the default.
	exp, err := zipkin.New(cfg.Endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("zipkin: %w", err)
	}
	return exp, exp.Shutdown, nil
}

// grpcDefaultTLS returns the system-default TLS credentials for the
// OTLP gRPC exporter. Operators wanting a custom CA bundle / mTLS hand
// us a pre-built endpoint+config through their reverse-proxy or call
// the exporter directly post-v1.0.
func grpcDefaultTLS() credentialsTransport {
	// The otlptracegrpc API takes a credentials.TransportCredentials,
	// not a *tls.Config. Wrapping is one line in the helper below; the
	// indirection isolates the tls/credentials import to this file.
	return newGRPCTransportTLS(&tls.Config{MinVersion: tls.VersionTLS12})
}
