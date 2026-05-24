// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// stubAuthorizerForInterceptor allows specific methods to specific
// principals; everything else is denied.
type stubAuthorizerForInterceptor struct {
	allowed map[string]auth.Role // method -> minimum role; absence = any role allowed
	bypass  map[string]bool
}

func (s *stubAuthorizerForInterceptor) Authorize(_ context.Context, p *auth.Principal, method string) error {
	if s.bypass[method] {
		return nil
	}
	if p == nil {
		return errors.New("no principal")
	}
	min, ok := s.allowed[method]
	if !ok {
		return nil
	}
	if !p.HasRole(min) {
		return errors.New("insufficient role")
	}
	return nil
}

func TestInterceptorConfig_RequiresAuthenticator(t *testing.T) {
	cfg := &auth.InterceptorConfig{Authorizer: auth.NewRBACAuthorizer()}
	if _, err := cfg.UnaryServerInterceptor(); err == nil {
		t.Error("expected error for missing Authenticator")
	}
}

func TestInterceptorConfig_RequiresAuthorizer(t *testing.T) {
	cfg := &auth.InterceptorConfig{Authenticator: auth.NewChain()}
	if _, err := cfg.UnaryServerInterceptor(); err == nil {
		t.Error("expected error for missing Authorizer")
	}
}

func TestUnaryInterceptor_Success(t *testing.T) {
	want := &auth.Principal{ID: "u-1", Role: auth.RoleAdmin}
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: want},
		Authorizer:    &stubAuthorizerForInterceptor{},
	}
	interceptor, err := cfg.UnaryServerInterceptor()
	if err != nil {
		t.Fatal(err)
	}

	var seenPrincipal *auth.Principal
	handler := func(ctx context.Context, req any) (any, error) {
		seenPrincipal = auth.PrincipalFromContext(ctx)
		return "ok", nil
	}
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/Method"}, handler)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v", resp)
	}
	if seenPrincipal != want {
		t.Errorf("handler did not see principal in ctx; got %v", seenPrincipal)
	}
}

func TestUnaryInterceptor_AuthFailure_Unauthenticated(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrUnauthenticated},
		Authorizer:    &stubAuthorizerForInterceptor{}, // empty -> deny when nil principal
	}
	interceptor, _ := cfg.UnaryServerInterceptor()

	var handlerCalled atomic.Bool
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled.Store(true)
		return nil, nil
	}
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/Method"}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("status code = %v, want Unauthenticated", status.Code(err))
	}
	if handlerCalled.Load() {
		t.Error("handler should not have been called on auth failure")
	}
}

func TestUnaryInterceptor_AuthorizeFailure_PermissionDenied(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{Role: auth.RoleReadonly}},
		Authorizer: &stubAuthorizerForInterceptor{
			allowed: map[string]auth.Role{"/test/Admin": auth.RoleAdmin},
		},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/Admin"}, dummyHandler)
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("status code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestUnaryInterceptor_BypassMethod_NoPrincipalRequired(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrUnauthenticated},
		Authorizer:    &stubAuthorizerForInterceptor{bypass: map[string]bool{"/test/Health": true}},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()

	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/Health"}, dummyHandler)
	if err != nil {
		t.Errorf("bypass should succeed: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v", resp)
	}
}

func TestUnaryInterceptor_RateLimited(t *testing.T) {
	rl := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 1,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Hour,
	})
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrInvalidCredentials},
		Authorizer:    &stubAuthorizerForInterceptor{},
		RateLimiter:   rl,
	}
	interceptor, _ := cfg.UnaryServerInterceptor()

	// First call: bad creds -> records failure, returns Unauthenticated.
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/M"}, dummyHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("first: %v", err)
	}

	// Second call: lockout fires before the authenticator runs.
	_, err = interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/M"}, dummyHandler)
	if status.Code(err) != codes.ResourceExhausted {
		t.Errorf("second: %v (want ResourceExhausted)", err)
	}
}

func TestUnaryInterceptor_RateLimitClearsOnSuccess(t *testing.T) {
	rl := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 5,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
	})
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{ID: "u-1", Role: auth.RoleAdmin}},
		Authorizer:    &stubAuthorizerForInterceptor{},
		RateLimiter:   rl,
	}
	interceptor, _ := cfg.UnaryServerInterceptor()

	for i := 0; i < 10; i++ {
		_, err := interceptor(context.Background(), nil,
			&grpc.UnaryServerInfo{FullMethod: "/test/M"}, dummyHandler)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
}

func TestStreamInterceptor_PrincipalAvailable(t *testing.T) {
	want := &auth.Principal{ID: "u-stream", Role: auth.RoleOperator}
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: want},
		Authorizer:    &stubAuthorizerForInterceptor{},
	}
	interceptor, err := cfg.StreamServerInterceptor()
	if err != nil {
		t.Fatal(err)
	}

	var seen *auth.Principal
	handler := func(srv any, ss grpc.ServerStream) error {
		seen = auth.PrincipalFromContext(ss.Context())
		return nil
	}
	stream := &fakeStream{ctx: context.Background()}
	if err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: "/test/Stream"}, handler); err != nil {
		t.Fatalf("err = %v", err)
	}
	if seen != want {
		t.Errorf("stream handler did not see principal; got %v", seen)
	}
}

func TestHTTPMiddleware_Success(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{ID: "h-1", Role: auth.RoleAdmin}},
		Authorizer:    &stubAuthorizerForInterceptor{},
	}
	mw, err := cfg.HTTPMiddleware()
	if err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := auth.PrincipalFromContext(r.Context())
		if p == nil || p.ID != "h-1" {
			t.Errorf("HTTP handler did not see principal; got %v", p)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mw(next))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestHTTPMiddleware_Unauthenticated_401(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrUnauthenticated},
		Authorizer:    &stubAuthorizerForInterceptor{},
	}
	mw, _ := cfg.HTTPMiddleware()
	srv := httptest.NewServer(mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	})))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHTTPMiddleware_PermissionDenied_403(t *testing.T) {
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{ID: "x", Role: auth.RoleReadonly}},
		Authorizer: &stubAuthorizerForInterceptor{
			allowed: map[string]auth.Role{"/HTTP GET /": auth.RoleAdmin},
		},
	}
	mw, _ := cfg.HTTPMiddleware()
	srv := httptest.NewServer(mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	})))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHTTPMiddleware_RateLimited_429(t *testing.T) {
	rl := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 1,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Hour,
	})
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrInvalidCredentials},
		Authorizer:    &stubAuthorizerForInterceptor{},
		RateLimiter:   rl,
	}
	mw, _ := cfg.HTTPMiddleware()
	srv := httptest.NewServer(mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	})))
	defer srv.Close()

	// First request: 401 + records failure
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("first status = %d", resp.StatusCode)
	}

	// Second request: 429
	resp, err = http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second status = %d, want 429", resp.StatusCode)
	}
}

func TestPrincipalIDOrPeerIP(t *testing.T) {
	// With a principal, returns its ID.
	got := auth.PrincipalIDOrPeerIP(context.Background(),
		&auth.Principal{ID: "user-1"})
	if !strings.HasPrefix(got, "principal:") {
		t.Errorf("got %q", got)
	}

	// Without principal, falls back to "unknown" when no peer info.
	got = auth.PrincipalIDOrPeerIP(context.Background(), nil)
	if got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}

// ---- shared test helpers --------------------------------------------------

func dummyHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

type capturedDecision struct {
	method    string
	principal *auth.Principal
	allowed   bool
	reason    error
}

func TestUnaryInterceptor_OnAuthDecision_SuccessEmits(t *testing.T) {
	var captured []capturedDecision
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{ID: "u-1", Role: auth.RoleAdmin}},
		Authorizer:    &stubAuthorizerForInterceptor{},
		OnAuthDecision: func(_ context.Context, m string, p *auth.Principal, allowed bool, reason error) {
			captured = append(captured, capturedDecision{m, p, allowed, reason})
		},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Foo"},
		func(ctx context.Context, req any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("interceptor err: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured = %d, want 1", len(captured))
	}
	if !captured[0].allowed || captured[0].method != "/svc/Foo" || captured[0].principal.ID != "u-1" {
		t.Errorf("captured: %+v", captured[0])
	}
}

func TestUnaryInterceptor_OnAuthDecision_AuthFailureEmits(t *testing.T) {
	var captured []capturedDecision
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrUnauthenticated},
		Authorizer:    &stubAuthorizerForInterceptor{},
		OnAuthDecision: func(_ context.Context, m string, p *auth.Principal, allowed bool, reason error) {
			captured = append(captured, capturedDecision{m, p, allowed, reason})
		},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Foo"},
		func(ctx context.Context, req any) (any, error) { return nil, nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("err code = %v, want Unauthenticated", status.Code(err))
	}
	if len(captured) != 1 {
		t.Fatalf("captured = %d, want 1", len(captured))
	}
	if captured[0].allowed {
		t.Errorf("allowed = true on auth failure")
	}
	if captured[0].principal != nil {
		t.Errorf("principal non-nil on auth failure: %+v", captured[0].principal)
	}
	if captured[0].reason == nil {
		t.Errorf("reason nil on denial")
	}
}

func TestUnaryInterceptor_OnAuthDecision_AuthzFailureEmits(t *testing.T) {
	var captured []capturedDecision
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{principal: &auth.Principal{ID: "u-1", Role: auth.RoleReadonly}},
		Authorizer:    &stubAuthorizerForInterceptor{allowed: map[string]auth.Role{"/svc/AdminOp": auth.RoleAdmin}},
		OnAuthDecision: func(_ context.Context, m string, p *auth.Principal, allowed bool, reason error) {
			captured = append(captured, capturedDecision{m, p, allowed, reason})
		},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/AdminOp"},
		func(ctx context.Context, req any) (any, error) { return nil, nil })
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("err code = %v, want PermissionDenied", status.Code(err))
	}
	if len(captured) != 1 || captured[0].allowed {
		t.Fatalf("captured = %+v", captured)
	}
	if captured[0].principal == nil || captured[0].principal.ID != "u-1" {
		t.Errorf("principal lost: %+v", captured[0].principal)
	}
}

func TestUnaryInterceptor_OnAuthDecision_BypassEmitsAllowed(t *testing.T) {
	var captured []capturedDecision
	cfg := &auth.InterceptorConfig{
		Authenticator: &stubAuthenticator{err: auth.ErrUnauthenticated},
		Authorizer: &stubAuthorizerForInterceptor{
			bypass: map[string]bool{"/svc/Health": true},
		},
		OnAuthDecision: func(_ context.Context, m string, p *auth.Principal, allowed bool, reason error) {
			captured = append(captured, capturedDecision{m, p, allowed, reason})
		},
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Health"},
		func(ctx context.Context, req any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("bypass blocked: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured = %d, want 1 (bypass still emits)", len(captured))
	}
	if !captured[0].allowed {
		t.Errorf("bypass should report allowed=true")
	}
	// Reason surfaces ErrUnauthenticated as context (no principal,
	// auth.Authenticate failed but Authorize admitted).
	if !errors.Is(captured[0].reason, auth.ErrUnauthenticated) {
		t.Errorf("reason = %v, want ErrUnauthenticated wrapped", captured[0].reason)
	}
}

func TestUnaryInterceptor_OnAuthDecision_NilCallbackOK(t *testing.T) {
	// Nil callback must not panic.
	cfg := &auth.InterceptorConfig{
		Authenticator:  &stubAuthenticator{principal: &auth.Principal{ID: "u", Role: auth.RoleAdmin}},
		Authorizer:     &stubAuthorizerForInterceptor{},
		OnAuthDecision: nil,
	}
	interceptor, _ := cfg.UnaryServerInterceptor()
	if _, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Foo"},
		func(ctx context.Context, req any) (any, error) { return nil, nil }); err != nil {
		t.Errorf("%v", err)
	}
}

type fakeStream struct {
	ctx context.Context
}

func (s *fakeStream) Context() context.Context     { return s.ctx }
func (s *fakeStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStream) SetTrailer(metadata.MD)       {}
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }
