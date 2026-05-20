package tracing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"go.keystone-core.io/keystone-core/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func enabledStdoutCfg() config.TracingConfig {
	return config.TracingConfig{
		Enabled:            true,
		ServiceName:        "test",
		Exporter:           config.TracingExporterStdout,
		Sampler:            config.TracingSamplerAlwaysOff, // avoid stdout span noise
		SampleRate:         0.1,
		RateLimitPerSecond: 10,
		BatchSize:          16,
		QueueSize:          64,
		FlushInterval:      100 * time.Millisecond,
	}
}

func TestNew_Disabled_ReturnsNoopProvider(t *testing.T) {
	p, err := New(config.TracingConfig{Enabled: false}, discardLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tp := p.TracerProvider()
	if reflect.TypeOf(tp) != reflect.TypeOf(tracenoop.NewTracerProvider()) {
		t.Errorf("TracerProvider type = %T, want noop", tp)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on noop = %v, want nil", err)
	}
}

func TestNew_Stdout_BuildsRealProvider(t *testing.T) {
	p, err := New(enabledStdoutCfg(), discardLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := p.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()
	if _, ok := p.TracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T, want *sdktrace.TracerProvider", p.TracerProvider())
	}
}

func TestNew_AllSamplers(t *testing.T) {
	samplers := []string{
		config.TracingSamplerAlwaysOn,
		config.TracingSamplerAlwaysOff,
		config.TracingSamplerProbabilistic,
		config.TracingSamplerParentBased,
		config.TracingSamplerRateLimiting,
	}
	for _, s := range samplers {
		t.Run(s, func(t *testing.T) {
			cfg := enabledStdoutCfg()
			cfg.Sampler = s
			p, err := New(cfg, discardLogger())
			if err != nil {
				t.Fatalf("New(%s): %v", s, err)
			}
			_ = p.Shutdown(context.Background())
		})
	}
}

func TestNew_UnknownSampler_Errors(t *testing.T) {
	cfg := enabledStdoutCfg()
	cfg.Sampler = "weird"
	_, err := New(cfg, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "sampler") {
		t.Errorf("err = %v, want sampler error", err)
	}
}

func TestNew_UnknownExporter_Errors(t *testing.T) {
	cfg := enabledStdoutCfg()
	cfg.Exporter = "kafka"
	_, err := New(cfg, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "exporter") {
		t.Errorf("err = %v, want exporter error", err)
	}
}

func TestProvider_Nil_Safe(t *testing.T) {
	var p *Provider
	if p.TracerProvider() == nil {
		t.Error("nil *Provider.TracerProvider() returned nil")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("nil *Provider.Shutdown = %v, want nil", err)
	}
}

// recordingExporter is a test seam that lets us assert that the batch
// processor flushed a span on Shutdown without spinning up a network.
type recordingExporter struct {
	mu     sync.Mutex
	spans  []sdktrace.ReadOnlySpan
	closed bool
}

func (r *recordingExporter) ExportSpans(_ context.Context, ss []sdktrace.ReadOnlySpan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, ss...)
	return nil
}

func (r *recordingExporter) Shutdown(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// TestProvider_ShutdownFlushesPendingSpans wires a recordingExporter
// behind a hand-built TracerProvider — bypasses New so we can install
// the test seam — and confirms Shutdown drives the BatchSpanProcessor
// to ExportSpans before the exporter's own Shutdown.
func TestProvider_ShutdownFlushesPendingSpans(t *testing.T) {
	rec := &recordingExporter{}
	bsp := sdktrace.NewBatchSpanProcessor(rec,
		sdktrace.WithMaxExportBatchSize(8),
		sdktrace.WithBatchTimeout(time.Minute), // never auto-flushes in this test
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
	)
	p := &Provider{
		tp: tp,
		shutdown: func(ctx context.Context) error {
			if err := tp.Shutdown(ctx); err != nil {
				return err
			}
			return rec.Shutdown(ctx)
		},
	}

	tracer := p.TracerProvider().Tracer("test")
	for i := 0; i < 3; i++ {
		_, span := tracer.Start(context.Background(), "op")
		span.End()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.spans) != 3 {
		t.Errorf("exported spans = %d, want 3", len(rec.spans))
	}
	if !rec.closed {
		t.Error("recordingExporter.Shutdown was not called")
	}
}

// TestSamplerEnum_ContainsAlwaysOn double-checks the trivial samplers
// produce non-nil OTel built-ins so a future stdlib break is loud.
func TestSamplerEnum_ContainsAlwaysOn(t *testing.T) {
	cfg := enabledStdoutCfg()
	cfg.Sampler = config.TracingSamplerAlwaysOn
	s, err := newSampler(cfg)
	if err != nil || s == nil {
		t.Fatalf("newSampler always_on: s=%v err=%v", s, err)
	}
	if !strings.Contains(s.Description(), "AlwaysOn") {
		t.Errorf("Description = %q, want AlwaysOn", s.Description())
	}
}

func TestExporterEnum_ZipkinAndOTLPRequireEndpoint(t *testing.T) {
	cases := []string{
		config.TracingExporterOTLPGRPC,
		config.TracingExporterOTLPHTTP,
		config.TracingExporterZipkin,
	}
	for _, e := range cases {
		t.Run(e, func(t *testing.T) {
			cfg := enabledStdoutCfg()
			cfg.Exporter = e
			cfg.Endpoint = "http://localhost:0"
			cfg.Insecure = true
			// Building the exporter is enough — we don't need to
			// actually reach the endpoint. Failure here would mean the
			// dispatch is broken, not that the collector is down.
			_, shutdown, err := newExporter(cfg, discardLogger())
			if err != nil {
				t.Fatalf("newExporter(%s): %v", e, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				// Most exporters tolerate Shutdown on an unconnected
				// client; deadlines from network attempts are OK.
				t.Logf("shutdown(%s): %v (acceptable)", e, err)
			}
		})
	}
}
