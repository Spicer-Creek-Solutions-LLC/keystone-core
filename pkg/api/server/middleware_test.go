// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// stubAuthenticator returns a configurable principal/error result.
// Lets middleware tests exercise the chain without standing up the
// full APIKey/JWT stack.
type stubAuthenticator struct {
	principal *auth.Principal
	err       error
}

func (s *stubAuthenticator) Authenticate(_ context.Context) (*auth.Principal, error) {
	return s.principal, s.err
}

// stubAuthorizer accepts every call when permissive=true; otherwise
// rejects, surfacing as 401/Unauthenticated when no principal is
// present.
type stubAuthorizer struct {
	permissive bool
}

func (s *stubAuthorizer) Authorize(_ context.Context, p *auth.Principal, _ string) error {
	if s.permissive {
		return nil
	}
	if p == nil {
		return auth.ErrUnauthenticated
	}
	return auth.ErrUnauthorized
}

func newAuthInterceptor(t *testing.T, authn auth.Authenticator, authz auth.Authorizer, rl *auth.RateLimiter) *auth.InterceptorConfig {
	t.Helper()
	return &auth.InterceptorConfig{
		Authenticator: authn,
		Authorizer:    authz,
		RateLimiter:   rl,
	}
}

func TestHTTPChain_HealthBypassesAuth(t *testing.T) {
	// Authenticator that ALWAYS errors. If health endpoints went
	// through auth, GET /health/live would fail.
	ic := newAuthInterceptor(t,
		&stubAuthenticator{err: auth.ErrUnauthenticated},
		&stubAuthorizer{permissive: false},
		nil,
	)
	srv, _ := newServer(t, func(o *server.Options) { o.AuthInterceptor = ic })
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// Health bypasses auth → 200.
	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/health/live")
	if err != nil {
		t.Fatalf("GET /health/live: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health/live = %d, want 200", resp.StatusCode)
	}

	// /api/* hits auth → 401.
	resp, err = http.Get("http://" + srv.Addrs().HTTP + "/api/v1/agents")
	if err != nil {
		t.Fatalf("GET /api/v1/agents: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/api/v1/agents = %d, want 401", resp.StatusCode)
	}
}

func TestHTTPChain_OPTIONSPreflightBypassesRateLimit(t *testing.T) {
	// Tight rate limiter: 1 failure trips a lockout. If OPTIONS
	// went through the auth chain, the second OPTIONS would 429.
	rl := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 1,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
		MaxLockout:           time.Second,
	})
	ic := newAuthInterceptor(t,
		&stubAuthenticator{err: auth.ErrInvalidCredentials},
		&stubAuthorizer{permissive: false},
		rl,
	)
	srv, _ := newServer(t, func(o *server.Options) { o.AuthInterceptor = ic })
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// 10 OPTIONS preflights should NEVER trip rate-limit.
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodOptions, "http://"+srv.Addrs().HTTP+"/api/v1/agents", nil)
		req.Header.Set("Origin", "https://test.example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("preflight %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("preflight %d = %d, want 204", i, resp.StatusCode)
		}
	}

	// A real request still hits the auth chain. Two failures back-to-
	// back: the first records a failure, the second hits the lockout.
	for i := 0; i < 2; i++ {
		resp, err := http.Get("http://" + srv.Addrs().HTTP + "/api/v1/agents")
		if err != nil {
			t.Fatalf("GET %d: %v", i, err)
		}
		resp.Body.Close()
		if i == 0 && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("first GET = %d, want 401 (auth fail)", resp.StatusCode)
		}
		if i == 1 && resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("second GET = %d, want 429 (rate-limited)", resp.StatusCode)
		}
	}
}

func TestHTTPChain_NoAuthInterceptorAllowsAPI(t *testing.T) {
	// Sanity check the dev-mode default: no AuthInterceptor → /api/*
	// reaches the registered handlers (which return 501 from epic 03
	// stubs, not 401/403).
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/api/v1/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Errorf("status = %d; auth was wired despite nil interceptor", resp.StatusCode)
	}
}

func TestHTTPChain_CORSDisabledSkipsHeaders(t *testing.T) {
	cfg := newTestConfig()
	cfg.Server.CORS = config.CORSConfig{Enabled: false}
	srv, _ := newServer(t, func(o *server.Options) { o.Config = cfg })
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	req, _ := http.NewRequest(http.MethodGet, "http://"+srv.Addrs().HTTP+"/health/live", nil)
	req.Header.Set("Origin", "https://test.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS disabled but Allow-Origin = %q", got)
	}
}

// ---- gRPC interceptor wiring ----------------------------------------------

// We register grpc/health as a real, generated service so the
// interceptor has a method to gate. Without credentials, the auth
// chain must reject the call with Unauthenticated; with permissive
// auth wired in, the same call must succeed.

func TestGRPC_AuthInterceptorWired(t *testing.T) {
	ic := newAuthInterceptor(t,
		&stubAuthenticator{err: auth.ErrUnauthenticated},
		&stubAuthorizer{permissive: false},
		nil,
	)

	srv, _ := newServer(t, func(o *server.Options) { o.AuthInterceptor = ic })
	healthv1.RegisterHealthServer(grpcRegistrar(srv), health.NewServer())

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	conn, err := grpc.NewClient(srv.Addrs().GRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := healthv1.NewHealthClient(conn)
	_, err = client.Check(ctx, &healthv1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error from interceptor; got nil")
	}
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated; err = %v", got, err)
	}
}

func TestGRPC_PermissiveAuthLetsCallThrough(t *testing.T) {
	// Authenticator returns a valid principal; authorizer is
	// permissive. The Health/Check call should succeed.
	ic := newAuthInterceptor(t,
		&stubAuthenticator{principal: &auth.Principal{ID: "test", Name: "test", Role: auth.RoleAdmin}},
		&stubAuthorizer{permissive: true},
		nil,
	)

	srv, _ := newServer(t, func(o *server.Options) { o.AuthInterceptor = ic })
	healthv1.RegisterHealthServer(grpcRegistrar(srv), health.NewServer())

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	conn, err := grpc.NewClient(srv.Addrs().GRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := healthv1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &healthv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// The default Health server returns SERVING for an empty service.
	if resp.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.GetStatus())
	}
}

// grpcRegistrar adapts the Server to grpc.ServiceRegistrar for the
// generated *RegisterXxxServer helpers, which expect that interface.
type registrarFunc struct{ s *server.Server }

func (r *registrarFunc) RegisterService(desc *grpc.ServiceDesc, impl any) {
	r.s.RegisterService(desc, impl)
}

func grpcRegistrar(s *server.Server) grpc.ServiceRegistrar {
	return &registrarFunc{s: s}
}
