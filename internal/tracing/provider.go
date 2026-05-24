// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/version"
)

// Provider wraps the OTel SDK TracerProvider so callers see a single
// type whether tracing is enabled or not. Shutdown is always callable;
// TracerProvider always returns a non-nil value.
type Provider struct {
	tp       trace.TracerProvider
	shutdown func(context.Context) error
}

// New constructs a Provider from cfg. When cfg.Enabled=false the
// returned Provider wraps a noop TracerProvider; Shutdown is a no-op.
// The logger is used for exporter/processor diagnostics only.
//
// Callers that want process-wide propagation install the returned
// TracerProvider via otel.SetTracerProvider after New returns. This
// package never touches the OTel globals.
func New(cfg config.TracingConfig, log *slog.Logger) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{
			tp:       tracenoop.NewTracerProvider(),
			shutdown: func(context.Context) error { return nil },
		}, nil
	}
	if log == nil {
		log = slog.Default()
	}

	exp, expShutdown, err := newExporter(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("tracing: build exporter: %w", err)
	}

	sampler, err := newSampler(cfg)
	if err != nil {
		// Roll back the half-built exporter so we don't leak its
		// goroutines / gRPC client.
		_ = expShutdown(context.Background())
		return nil, fmt.Errorf("tracing: build sampler: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(version.Get().Version),
			attribute.String("service.instance.id", uuid.NewString()),
		),
	)
	if err != nil {
		_ = expShutdown(context.Background())
		return nil, fmt.Errorf("tracing: build resource: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exp,
		sdktrace.WithMaxExportBatchSize(cfg.BatchSize),
		sdktrace.WithMaxQueueSize(cfg.QueueSize),
		sdktrace.WithBatchTimeout(cfg.FlushInterval),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)

	return &Provider{
		tp: tp,
		shutdown: func(ctx context.Context) error {
			// Shut down the provider first so no new spans land in
			// the processor mid-flush. The provider itself flushes
			// the BatchSpanProcessor.
			if err := tp.Shutdown(ctx); err != nil {
				return fmt.Errorf("tracing: tracerprovider shutdown: %w", err)
			}
			if err := expShutdown(ctx); err != nil {
				return fmt.Errorf("tracing: exporter shutdown: %w", err)
			}
			return nil
		},
	}, nil
}

// TracerProvider returns the OTel handle. Always non-nil. When New was
// called with Enabled=false, this is the noop TracerProvider.
func (p *Provider) TracerProvider() trace.TracerProvider {
	if p == nil {
		return tracenoop.NewTracerProvider()
	}
	return p.tp
}

// Shutdown flushes pending spans and releases exporter resources within
// ctx's deadline. Idempotent; safe to call on a noop provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}
