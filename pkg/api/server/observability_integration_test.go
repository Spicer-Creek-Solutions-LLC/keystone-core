package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel"

	"go.keystone-core.io/keystone-core/internal/logging"
	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// recordingExporter is the spans sink for the Task 11 integration test.
// Mirrors the unit-level seam in internal/tracing/provider_test.go but
// duplicated here so the integration owns its own assertion surface.
type recordingExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (r *recordingExporter) ExportSpans(_ context.Context, ss []sdktrace.ReadOnlySpan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, ss...)
	return nil
}

func (r *recordingExporter) Shutdown(context.Context) error { return nil }

func (r *recordingExporter) snapshot() []sdktrace.ReadOnlySpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sdktrace.ReadOnlySpan, len(r.spans))
	copy(out, r.spans)
	return out
}

// TestObservability_Integration_FullFlow is the Epic 17 task-11 closing
// gate. One real *server.Server with the metrics Registry, an OTel
// TracerProvider backed by a recording exporter, and the correlation
// middleware all wired together. Three sub-tests cover the three
// behaviours the task description calls out:
//
//   - /metrics serves the kscore + runtime metric namespace.
//   - OTel-installed provider emits spans to the configured exporter.
//   - Correlation IDs flow through HTTP requests round-trip.
//
// We don't construct the Provider via tracing.New here — the integration
// installs a hand-built sdktrace.TracerProvider with a recording
// exporter via the test seam so assertions don't require stdout
// scraping. The "tracing.New builds correctly" half is covered by
// internal/tracing/provider_test.go.
func TestObservability_Integration_FullFlow(t *testing.T) {
	rec := &recordingExporter{}
	bsp := sdktrace.NewBatchSpanProcessor(rec,
		sdktrace.WithMaxExportBatchSize(8),
		sdktrace.WithBatchTimeout(50*time.Millisecond),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
	)
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
		otel.SetTracerProvider(prevTP)
	})

	cfg := newTestConfig()
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = "/metrics"

	reg := metrics.NewRegistry(metrics.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srvMetrics, err := server.NewMetrics(reg)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	log, _ := captureLogger(t)
	srv, err := server.New(server.Options{
		Config:          cfg,
		Logger:          log,
		Store:           newTestStore(t),
		NATSManager:     server.NoopNATSManager{},
		Subjects:        fakeSubjects{cluster: "default"},
		Signer:          fakeSigner{},
		Metrics:         srvMetrics,
		MetricsRegistry: reg,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(runCtx); err != nil {
		cancel()
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		_ = srv.Stop(stopCtx)
		cancel()
	})

	base := "http://" + srv.Addrs().HTTP

	t.Run("metrics_endpoint_serves_kscore_namespace", func(t *testing.T) {
		// One throwaway request so the http_request_duration histogram
		// has at least one observation by the time we scrape.
		resp, err := http.Get(base + "/health/live")
		if err != nil {
			t.Fatalf("warm-up GET: %v", err)
		}
		_ = resp.Body.Close()

		resp, err = http.Get(base + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
			t.Errorf("Content-Type = %q, want prom-format text/plain", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		got := string(body)

		// Three slices of the v1.0 surface:
		//   1) the cardinality self-metric (registered by NewRegistry).
		//   2) the runtime + process collectors (auto-registered).
		//   3) the HTTP middleware histogram (fires on the warm-up
		//      request, so its HELP line is present at scrape time).
		mustContain := []string{
			metrics.CardinalityMetricName,
			"go_goroutines",
			"process_cpu_seconds_total",
			metrics.DefHTTPRequestDurationSeconds.Name,
		}
		for _, m := range mustContain {
			if !strings.Contains(got, m) {
				t.Errorf("/metrics body missing %q", m)
			}
		}
	})

	t.Run("otel_exporter_receives_spans", func(t *testing.T) {
		tracer := otel.Tracer("integration-test")
		_, span := tracer.Start(context.Background(), "epic-17-integration")
		span.End()

		// Force a flush so the BatchSpanProcessor doesn't hold the span
		// past the test deadline. The provider's own Shutdown is
		// idempotent and the t.Cleanup above runs again at end of test.
		ctx, cancelFlush := context.WithTimeout(context.Background(), time.Second)
		defer cancelFlush()
		if err := tp.ForceFlush(ctx); err != nil {
			t.Fatalf("ForceFlush: %v", err)
		}

		spans := rec.snapshot()
		if len(spans) < 1 {
			t.Fatalf("recordingExporter received %d spans, want >= 1", len(spans))
		}
		var found bool
		for _, s := range spans {
			if s.Name() == "epic-17-integration" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected span 'epic-17-integration' not in recorded set")
		}
	})

	t.Run("correlation_id_flows_end_to_end", func(t *testing.T) {
		req, _ := http.NewRequest("GET", base+"/health/live", nil)
		req.Header.Set(logging.HTTPHeader, "epic-17-correlation-1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		// Outermost middleware echoes the inbound ID on the response.
		if got := resp.Header.Get(logging.HTTPHeader); got != "epic-17-correlation-1" {
			t.Errorf("response %s = %q, want epic-17-correlation-1",
				logging.HTTPHeader, got)
		}

		// And when the inbound header is absent the middleware
		// generates a fresh one — the response still carries something.
		req2, _ := http.NewRequest("GET", base+"/health/live", nil)
		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatalf("GET (no header): %v", err)
		}
		defer resp2.Body.Close()
		if got := resp2.Header.Get(logging.HTTPHeader); got == "" {
			t.Errorf("response %s is empty; middleware should have generated one",
				logging.HTTPHeader)
		}
	})
}
