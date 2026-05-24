// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/internal/ratelimit"
	"go.keystone-core.io/keystone-core/internal/ratelimit/extract"
)

// --- HTTP --------------------------------------------------------------------

func TestHTTP_AllowThenDeny(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	mwm, _ := NewMetrics(nil)
	srv := httptest.NewServer(Middleware(reg, extract.IP(extract.IPConfig{}), mwm)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// First request OK.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d", resp.StatusCode)
	}

	// Second request from the same IP — denied with 429.
	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		t.Error("Retry-After header missing")
	}
	if n, err := strconv.Atoi(ra); err != nil || n < 1 {
		t.Errorf("Retry-After = %q (parsed %d, err=%v); want positive integer", ra, n, err)
	}
	var body errorBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error != rejectedMessage {
		t.Errorf("body.Error = %q", body.Error)
	}
}

func TestHTTP_NilRegistry_Passthrough(t *testing.T) {
	hits := 0
	mw := Middleware(nil, extract.IP(extract.IPConfig{}), nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	for i := 0; i < 10; i++ {
		resp, _ := http.Get(srv.URL + "/")
		_ = resp.Body.Close()
	}
	if hits != 10 {
		t.Errorf("hits = %d, want 10 (nil registry should passthrough)", hits)
	}
}

func TestHTTP_NilExtractor_Passthrough(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	mw := Middleware(reg, nil, nil)
	hits := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	for i := 0; i < 5; i++ {
		resp, _ := http.Get(srv.URL + "/")
		_ = resp.Body.Close()
	}
	if hits != 5 {
		t.Errorf("hits = %d, want 5", hits)
	}
}

func TestHTTP_ExtractorNoKey_PassesThrough(t *testing.T) {
	// Header extractor on a request that doesn't carry the
	// header → extractor returns ok=false → middleware allows
	// the request (skip-rather-than-deny doctrine).
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 0},
	})
	hits := 0
	mw := Middleware(reg, extract.Header("X-Tenant-Id"), nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	for i := 0; i < 3; i++ {
		resp, _ := http.Get(srv.URL + "/")
		_ = resp.Body.Close()
	}
	if hits != 3 {
		t.Errorf("hits = %d, want 3 (no-key requests pass through)", hits)
	}
}

// TestHTTP_1000_RPS_Configured_Limit_Returns_429 satisfies the Epic
// 18 acceptance line: traffic exceeding the configured limit must
// return 429 with Retry-After. The "1000 RPS" in the spec is the
// deployment scenario; we prove the mechanism with a tight limit
// + a burst of requests so the test runs deterministically.
func TestHTTP_1000_RPS_Configured_Limit_Returns_429(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 600, Burst: 5}, // 10 RPS, 5 burst
	})
	mwm, _ := NewMetrics(nil)
	mw := Middleware(reg, extract.IP(extract.IPConfig{}), mwm)
	srv := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// 50 requests in tight loop from one client IP. With burst=5,
	// 5 should succeed and at least 40 should 429.
	var oks, rejected int
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		switch resp.StatusCode {
		case http.StatusOK:
			oks++
		case http.StatusTooManyRequests:
			rejected++
			if resp.Header.Get("Retry-After") == "" {
				t.Error("Retry-After missing on 429")
			}
		default:
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	if oks != 5 {
		t.Errorf("oks = %d, want 5 (burst capacity)", oks)
	}
	if rejected < 40 {
		t.Errorf("rejected = %d, want >= 40", rejected)
	}
}

// TestHTTP_PerAPIKey_vs_PerIP_Isolation satisfies the Epic 18
// acceptance line: per-key isolation verified by test. Two
// API keys exhaust independently; two IPs exhaust independently;
// neither cross-contaminates.
func TestHTTP_PerAPIKey_vs_PerIP_Isolation(t *testing.T) {
	cases := []struct {
		name    string
		ext     extract.Extractor
		keyAReq func(req *http.Request)
		keyBReq func(req *http.Request)
	}{
		{
			name: "per-API-key",
			ext:  extract.APIKey(),
			keyAReq: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer key-a")
			},
			keyBReq: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer key-b")
			},
		},
		{
			name: "per-IP",
			ext:  extract.IP(extract.IPConfig{TrustForwardedFor: true}),
			keyAReq: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1")
			},
			keyBReq: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.2")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
				Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
			})
			handler := Middleware(reg, tc.ext, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			srv := httptest.NewServer(handler)
			defer srv.Close()

			do := func(setKey func(*http.Request)) int {
				req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
				setKey(req)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
				return resp.StatusCode
			}

			// Key A: first OK, second denied (burst=1).
			if got := do(tc.keyAReq); got != http.StatusOK {
				t.Errorf("A first = %d", got)
			}
			if got := do(tc.keyAReq); got != http.StatusTooManyRequests {
				t.Errorf("A second = %d", got)
			}
			// Key B should still have its burst — isolation
			// proof.
			if got := do(tc.keyBReq); got != http.StatusOK {
				t.Errorf("B first = %d (isolation broken)", got)
			}
		})
	}
}

// TestHTTP_MetricIncrements satisfies "Rejected requests counted
// in metric" — we read the counter via the registry gatherer and
// verify it tracks the rejection count.
func TestHTTP_MetricIncrements(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	mr := metrics.NewRegistry(metrics.Options{})
	mwm, err := NewMetrics(mr)
	if err != nil {
		t.Fatal(err)
	}
	mw := Middleware(reg, extract.IP(extract.IPConfig{}), mwm)
	srv := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// First request succeeds; the next two are rejected.
	for i := 0; i < 3; i++ {
		resp, _ := http.Get(srv.URL + "/")
		_ = resp.Body.Close()
	}

	got := readCounter(t, mr, "kscore_ratelimit_rejected_total", map[string]string{"reason": ReasonLimitExceeded})
	if got != 2 {
		t.Errorf("rejected = %v, want 2", got)
	}
}

func TestHTTP_NilMetrics_NoPanic(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	mw := Middleware(reg, extract.IP(extract.IPConfig{}), nil)
	srv := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()
	for i := 0; i < 3; i++ {
		resp, _ := http.Get(srv.URL + "/")
		_ = resp.Body.Close()
	}
}

// --- gRPC --------------------------------------------------------------------

func TestGRPC_Unary_AllowThenDeny(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	mr := metrics.NewRegistry(metrics.Options{})
	mwm, _ := NewMetrics(mr)
	interceptor := UnaryServerInterceptor(reg, extract.IP(extract.IPConfig{}), mwm)

	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:9000")
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}

	// First call OK.
	if _, err := interceptor(ctx, nil, info, handler); err != nil {
		t.Errorf("first call: %v", err)
	}
	// Second call denied.
	_, err := interceptor(ctx, nil, info, handler)
	if err == nil {
		t.Fatal("second call: want error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("code = %s, want ResourceExhausted", st.Code())
	}
	if st.Message() != rejectedMessage {
		t.Errorf("message = %q", st.Message())
	}

	// Metric should have one reject.
	got := readCounter(t, mr, "kscore_ratelimit_rejected_total", map[string]string{"reason": ReasonLimitExceeded})
	if got != 1 {
		t.Errorf("rejected = %v, want 1", got)
	}
}

func TestGRPC_Unary_PerKey_Isolation(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	interceptor := UnaryServerInterceptor(reg, extract.APIKey(), nil)
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}

	withKey := func(key string) context.Context {
		md := metadata.Pairs("authorization", "Bearer "+key)
		return metadata.NewIncomingContext(context.Background(), md)
	}

	// Key a: first OK, second denied.
	if _, err := interceptor(withKey("a"), nil, info, handler); err != nil {
		t.Errorf("a-1: %v", err)
	}
	if _, err := interceptor(withKey("a"), nil, info, handler); err == nil {
		t.Error("a-2: want denial")
	}
	// Key b: still has burst.
	if _, err := interceptor(withKey("b"), nil, info, handler); err != nil {
		t.Errorf("b-1: %v (isolation broken)", err)
	}
}

func TestGRPC_Unary_NilRegistry_Passthrough(t *testing.T) {
	interceptor := UnaryServerInterceptor(nil, extract.APIKey(), nil)
	hit := 0
	handler := func(_ context.Context, _ any) (any, error) {
		hit++
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	for i := 0; i < 5; i++ {
		_, _ = interceptor(context.Background(), nil, info, handler)
	}
	if hit != 5 {
		t.Errorf("hit = %d, want 5", hit)
	}
}

func TestGRPC_Unary_NilExtractor_Passthrough(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	interceptor := UnaryServerInterceptor(reg, nil, nil)
	hit := 0
	handler := func(_ context.Context, _ any) (any, error) {
		hit++
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	for i := 0; i < 5; i++ {
		_, _ = interceptor(context.Background(), nil, info, handler)
	}
	if hit != 5 {
		t.Errorf("hit = %d, want 5", hit)
	}
}

func TestGRPC_Unary_ExtractorNoKey_PassesThrough(t *testing.T) {
	// IP extractor with no peer in context → ok=false → allow.
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 0},
	})
	interceptor := UnaryServerInterceptor(reg, extract.IP(extract.IPConfig{}), nil)
	hit := 0
	handler := func(_ context.Context, _ any) (any, error) {
		hit++
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	for i := 0; i < 3; i++ {
		_, _ = interceptor(context.Background(), nil, info, handler)
	}
	if hit != 3 {
		t.Errorf("hit = %d, want 3", hit)
	}
}

func TestGRPC_Stream_AllowThenDeny(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	interceptor := StreamServerInterceptor(reg, extract.IP(extract.IPConfig{}), nil)

	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:9000")
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
	ss := &fakeServerStream{ctx: ctx}
	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	// First call OK.
	if err := interceptor(nil, ss, info, handler); err != nil {
		t.Errorf("first: %v", err)
	}
	// Second call denied.
	err := interceptor(nil, ss, info, handler)
	if err == nil {
		t.Fatal("second: want error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.ResourceExhausted {
		t.Errorf("code = %s", st.Code())
	}
}

func TestGRPC_Stream_NilRegistry_Passthrough(t *testing.T) {
	interceptor := StreamServerInterceptor(nil, extract.IP(extract.IPConfig{}), nil)
	hit := 0
	handler := func(_ any, _ grpc.ServerStream) error {
		hit++
		return nil
	}
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	for i := 0; i < 3; i++ {
		_ = interceptor(nil, ss, info, handler)
	}
	if hit != 3 {
		t.Errorf("hit = %d, want 3", hit)
	}
}

func TestGRPC_Stream_NilExtractor_Passthrough(t *testing.T) {
	reg := ratelimit.NewRegistry(ratelimit.RegistryConfig{
		Default: ratelimit.Config{RequestsPerMinute: 60, Burst: 1},
	})
	interceptor := StreamServerInterceptor(reg, nil, nil)
	hit := 0
	handler := func(_ any, _ grpc.ServerStream) error {
		hit++
		return nil
	}
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	for i := 0; i < 3; i++ {
		_ = interceptor(nil, ss, info, handler)
	}
	if hit != 3 {
		t.Errorf("hit = %d, want 3", hit)
	}
}

// --- Metrics helpers --------------------------------------------------------

func TestNewMetrics_Nil(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Errorf("want nil emitter, got %+v", m)
	}
	// Nil-safety: calls on nil receiver should not panic.
	m.RecordReject(ReasonLimitExceeded)
}

func TestNewMetrics_DuplicateRegister(t *testing.T) {
	r := metrics.NewRegistry(metrics.Options{})
	if _, err := NewMetrics(r); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(r); err == nil {
		t.Error("second NewMetrics on same registry should fail (duplicate counter)")
	} else if !errors.Is(err, err) { // smoke check; we only want a non-nil error
		_ = err
	}
}

// --- low-level helpers ------------------------------------------------------

func TestItoa(t *testing.T) {
	cases := map[int]string{
		0: "0", 1: "1", 10: "10", 100: "100", 1_000_000: "1000000", -5: "-5",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatRetryAfter(t *testing.T) {
	if got := formatRetryAfter(3); got != "3" {
		t.Errorf("got %q", got)
	}
}

// fakeServerStream is a minimal grpc.ServerStream stand-in for
// the interceptor tests. Only Context is exercised.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// readCounter pulls metric_name from r and returns the counter
// value for the matching label set. Returns 0 on no match.
func readCounter(t *testing.T, r *metrics.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabels(m, labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func matchLabels(m *dto.Metric, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	have := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		have[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}
