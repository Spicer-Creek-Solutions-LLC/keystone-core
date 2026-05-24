// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// buildMetricsServer is a minimal wiring helper for the task-3 endpoint
// tests. Tests pass a Registry; the server registers /metrics on its
// public mux at cfg.Metrics.Path.
func buildMetricsServer(t *testing.T, cfg *config.Config, reg *metrics.Registry) (*server.Server, string) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := server.New(server.Options{
		Config:          cfg,
		Logger:          log,
		Store:           newTestStore(t),
		NATSManager:     server.NoopNATSManager{},
		Subjects:        fakeSubjects{cluster: "default"},
		Signer:          fakeSigner{},
		MetricsRegistry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = srv.Stop(ctx)
	})
	return srv, "http://" + srv.Addrs().HTTP
}

func TestMetricsEndpoint_ServesPromFormat(t *testing.T) {
	cfg := newTestConfig()
	cfg.Metrics = config.MetricsConfig{Enabled: true, Path: "/metrics"}

	reg := metrics.NewRegistry(metrics.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	// Force at least one sample so the cardinality counter family has
	// data; otherwise Prom omits empty families from the scrape body.
	c, err := reg.NewCounter(metrics.MetricDef{Name: "kscore_test_total", Help: "h", Labels: []string{"k"}})
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	c.With(metrics.Labels{"k": "v"}).Inc()

	_, base := buildMetricsServer(t, cfg, reg)
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prom-format", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	// Spot-check: the metric we just incremented, plus a known
	// runtime collector that the Registry auto-registers.
	for _, want := range []string{
		"kscore_test_total",
		"go_goroutines",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("/metrics body missing %q", want)
		}
	}
}

func TestMetricsEndpoint_Disabled_Returns404(t *testing.T) {
	cfg := newTestConfig()
	cfg.Metrics = config.MetricsConfig{Enabled: false, Path: "/metrics"}

	reg := metrics.NewRegistry(metrics.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	_, base := buildMetricsServer(t, cfg, reg)
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMetricsEndpoint_NoRegistry_Returns404(t *testing.T) {
	cfg := newTestConfig()
	cfg.Metrics = config.MetricsConfig{Enabled: true, Path: "/metrics"}

	// Enabled in config but no Registry supplied — handler skipped.
	_, base := buildMetricsServer(t, cfg, nil)
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMetricsEndpoint_CustomPath(t *testing.T) {
	cfg := newTestConfig()
	cfg.Metrics = config.MetricsConfig{Enabled: true, Path: "/internal/metrics"}

	reg := metrics.NewRegistry(metrics.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	_, base := buildMetricsServer(t, cfg, reg)

	// Default path 404s.
	if resp, _ := http.Get(base + "/metrics"); resp != nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("default /metrics = %d, want 404", resp.StatusCode)
		}
	}
	// Custom path serves.
	resp, err := http.Get(base + "/internal/metrics")
	if err != nil {
		t.Fatalf("GET /internal/metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/internal/metrics = %d, want 200", resp.StatusCode)
	}
}

func TestMetricsEndpoint_NoAuthRequired(t *testing.T) {
	// Confirms the Prom-scrape convention — the auth posture matches
	// /health/* (none). Tests without an AuthInterceptor verify this
	// trivially; this case explicitly asserts that no API key in the
	// request still returns 200.
	cfg := newTestConfig()
	cfg.Metrics = config.MetricsConfig{Enabled: true, Path: "/metrics"}
	reg := metrics.NewRegistry(metrics.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	_, base := buildMetricsServer(t, cfg, reg)

	req, _ := http.NewRequest(http.MethodGet, base+"/metrics", nil)
	// No Authorization header set.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 (Prom-scrape is unauthenticated)", resp.StatusCode)
	}
}
