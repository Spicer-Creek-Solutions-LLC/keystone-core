// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func srvTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func gatherMetric(t *testing.T, r *metrics.Registry, name string, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, _ := r.Gatherer().Gather()
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
	outer:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			if len(labels) != len(want) {
				continue
			}
			for k, v := range want {
				if labels[k] != v {
					continue outer
				}
			}
			return m
		}
	}
	return nil
}

func TestDefaultRouteExtractor(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"/", "/"},
		{"", "/"},
		{"/health/live", "/health/live"},
		{"/api/v1/agents/a-123", "/api/v1/agents"},
		{"/api/v1/agents", "/api/v1/agents"},
		{"/api/status", "/api/status"},
		{"/foo/bar/baz/qux", "/foo/bar/baz"},
	}
	for _, tt := range tests {
		if got := DefaultRouteExtractor("GET", tt.path); got != tt.want {
			t.Errorf("extract(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestHTTPMiddleware_NilMetrics_PassesThrough(t *testing.T) {
	var m *Metrics // nil
	called := false
	h := m.HTTPMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(204)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !called {
		t.Fatal("inner handler not called")
	}
}

func TestHTTPMiddleware_RecordsStatusCode(t *testing.T) {
	r := srvTestRegistry(t)
	m, _ := NewMetrics(r)
	h := m.HTTPMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/v1/agents/a-1")
	resp.Body.Close()

	if s := gatherMetric(t, r, metrics.DefHTTPRequestDurationSeconds.Name, map[string]string{
		"method": "GET", "code": "201", "route": "/api/v1/agents",
	}); s == nil || s.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("histogram = %v, want 1 observation", s)
	}
}

func TestHTTPMiddleware_DefaultsTo200(t *testing.T) {
	r := srvTestRegistry(t)
	m, _ := NewMetrics(r)
	h := m.HTTPMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi")) // implicit WriteHeader(200)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/health/live")
	resp.Body.Close()

	if s := gatherMetric(t, r, metrics.DefHTTPRequestDurationSeconds.Name, map[string]string{
		"method": "GET", "code": "200", "route": "/health/live",
	}); s == nil || s.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("implicit-200 histogram = %v", s)
	}
}

func TestUnaryInterceptor_RecordsOKAndError(t *testing.T) {
	r := srvTestRegistry(t)
	m, _ := NewMetrics(r)
	interceptor := m.UnaryServerInterceptor()

	okHandler := func(context.Context, any) (any, error) { return "ok", nil }
	errHandler := func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "x") }

	_, _ = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Hello"}, okHandler)
	_, _ = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Hello"}, errHandler)

	if s := gatherMetric(t, r, metrics.DefGRPCRequestDurationSeconds.Name, map[string]string{
		"method": "/svc/Hello", "code": "OK",
	}); s == nil || s.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("OK = %v, want 1", s)
	}
	if s := gatherMetric(t, r, metrics.DefGRPCRequestDurationSeconds.Name, map[string]string{
		"method": "/svc/Hello", "code": "NotFound",
	}); s == nil || s.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("NotFound = %v, want 1", s)
	}
}

type fakeServerStream struct{ grpc.ServerStream }

func TestStreamInterceptor_RecordsCompletion(t *testing.T) {
	r := srvTestRegistry(t)
	m, _ := NewMetrics(r)
	interceptor := m.StreamServerInterceptor()

	handler := func(any, grpc.ServerStream) error { return errors.New("boom") }
	_ = interceptor(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}, handler)

	if s := gatherMetric(t, r, metrics.DefGRPCRequestDurationSeconds.Name, map[string]string{
		"method": "/svc/Stream", "code": "Unknown", // generic error → Unknown
	}); s == nil || s.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("stream completion = %v, want 1", s)
	}
}

func TestNewMetrics_DuplicateRegistrationFails(t *testing.T) {
	r := srvTestRegistry(t)
	if _, err := NewMetrics(r); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(r); err == nil {
		t.Fatal("second NewMetrics: want error")
	}
}
