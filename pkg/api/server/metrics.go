// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the pkg/api/server emitter for v1.0 HTTP + gRPC request
// observability. Nil-safe.
type Metrics struct {
	http metrics.Histogram
	grpc metrics.Histogram
}

// NewMetrics registers the two server histograms against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	httpH, err := r.NewHistogram(metrics.DefHTTPRequestDurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("server: register http_request_duration_seconds: %w", err)
	}
	grpcH, err := r.NewHistogram(metrics.DefGRPCRequestDurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("server: register grpc_request_duration_seconds: %w", err)
	}
	return &Metrics{http: httpH, grpc: grpcH}, nil
}

// RouteExtractor maps a request path onto a stable, low-cardinality
// route label. Callers can override the default if their routes need
// finer bucketing — but anything reading from the URL path directly
// risks the cardinality limiter dropping samples.
type RouteExtractor func(method, urlPath string) string

// DefaultRouteExtractor buckets URL paths into "first three path
// components" plus the trailing /(...) marker:
//
//	/health/live           → /health/live
//	/api/v1/agents/a-123   → /api/v1/agents
//	/api/status            → /api/status
//	/foo/bar/baz/qux       → /foo/bar/baz
//
// This is intentionally simple — it keeps Prom cardinality bounded
// without hooking into the http.ServeMux pattern table.
func DefaultRouteExtractor(_, urlPath string) string {
	if urlPath == "" || urlPath == "/" {
		return "/"
	}
	parts := strings.Split(strings.TrimPrefix(urlPath, "/"), "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return "/" + strings.Join(parts, "/")
}

// HTTPMiddleware wraps next with a recorder that observes
// http_request_duration_seconds on every response. The middleware is
// a no-op when m is nil — production wiring threads the same instance
// through every layer; tests can pass nil.
func (m *Metrics) HTTPMiddleware(extract RouteExtractor) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	if extract == nil {
		extract = DefaultRouteExtractor
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			cw := &captureWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(cw, r)
			m.http.With(metrics.Labels{
				"method": r.Method,
				"code":   strconv.Itoa(cw.code),
				"route":  extract(r.Method, r.URL.Path),
			}).Observe(time.Since(start).Seconds())
		})
	}
}

// captureWriter is an http.ResponseWriter that retains the final
// status code. Servers that haven't called WriteHeader land in the
// default 200.
type captureWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

// WriteHeader captures the code then forwards.
func (c *captureWriter) WriteHeader(code int) {
	if c.wroteHeader {
		return
	}
	c.code = code
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(code)
}

// Write triggers an implicit WriteHeader(200) the first time it's
// called, mirroring the stdlib behaviour, so the gauge sees 200 rather
// than the zero value.
func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(b)
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that
// observes grpc_request_duration_seconds on every RPC.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	if m == nil {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		m.grpc.With(metrics.Labels{
			"method": info.FullMethod,
			"code":   status.Code(err).String(),
		}).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that
// observes grpc_request_duration_seconds at stream completion.
func (m *Metrics) StreamServerInterceptor() grpc.StreamServerInterceptor {
	if m == nil {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		m.grpc.With(metrics.Labels{
			"method": info.FullMethod,
			"code":   status.Code(err).String(),
		}).Observe(time.Since(start).Seconds())
		return err
	}
}
