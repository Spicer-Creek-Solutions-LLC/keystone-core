package versioning_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/pkg/api/versioning"
)

// fakeStream implements grpc.ServerStream just enough to be threaded
// through the StreamServerInterceptor in tests.
type fakeStream struct {
	ctx       context.Context
	headerMD  metadata.MD
	trailerMD metadata.MD
}

func (s *fakeStream) Context() context.Context     { return s.ctx }
func (s *fakeStream) SetHeader(md metadata.MD) error {
	if s.headerMD == nil {
		s.headerMD = metadata.MD{}
	}
	for k, v := range md {
		s.headerMD[k] = append(s.headerMD[k], v...)
	}
	return nil
}
func (s *fakeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStream) SetTrailer(md metadata.MD)    { s.trailerMD = md }
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }

func TestUnaryInterceptor_PassesCurrentEndpoint(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/svc/M", Status: versioning.StatusCurrent})

	interceptor := r.UnaryServerInterceptor()
	called := false
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(ctx context.Context, req any) (any, error) {
			called = true
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Error("handler should run for current endpoint")
	}
	if resp != "ok" {
		t.Errorf("resp = %v", resp)
	}
}

func TestUnaryInterceptor_RefusesRetired(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })
	r.Register(versioning.Endpoint{
		Method:   "/svc/M",
		Status:   versioning.StatusDeprecated,
		SunsetAt: now.Add(-time.Hour),
	})

	interceptor := r.UnaryServerInterceptor()
	called := false
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) {
			called = true
			return nil, nil
		})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("err code = %v, want Unimplemented", status.Code(err))
	}
	if called {
		t.Error("handler must not run for retired endpoint")
	}
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Errorf("err message should mention retired: %v", err)
	}
}

func TestUnaryInterceptor_UntrackedPassesThrough(t *testing.T) {
	r := versioning.NewRegistry()
	interceptor := r.UnaryServerInterceptor()
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/never-tracked"},
		func(context.Context, any) (any, error) { return "ok", nil })
	if err != nil {
		t.Errorf("untracked should pass: %v", err)
	}
	if resp != "ok" {
		t.Error("handler should run")
	}
}

func TestStreamInterceptor_RefusesRetired(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })
	r.Register(versioning.Endpoint{
		Method:   "/svc/Stream",
		Status:   versioning.StatusRetired,
		SunsetAt: now.Add(-24 * time.Hour),
	})

	interceptor := r.StreamServerInterceptor()
	called := false
	err := interceptor(nil, &fakeStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(any, grpc.ServerStream) error {
			called = true
			return nil
		})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("err code = %v, want Unimplemented", status.Code(err))
	}
	if called {
		t.Error("handler must not run for retired stream endpoint")
	}
}

func TestStreamInterceptor_SetsHeaderForDeprecated(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method:       "/svc/Stream",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	interceptor := r.StreamServerInterceptor()

	stream := &fakeStream{ctx: context.Background()}
	err := interceptor(nil, stream,
		&grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(any, grpc.ServerStream) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(stream.headerMD["deprecation"]) == 0 {
		t.Errorf("expected deprecation header on stream; got %v", stream.headerMD)
	}
}

// PROJECT-DETAILS §4.5 / acceptance criterion: registered deprecated
// endpoint emits the Deprecation HTTP header on response.
func TestHTTPMiddleware_DeprecatedEndpoint_ServesDeprecationHeader(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method:       "/HTTP GET /api/v1/legacy",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SunsetAt:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		Replacement:  "/api/v1/new",
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})
	srv := httptest.NewServer(r.HTTPMiddleware(nil)(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/legacy")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Deprecation") == "" {
		t.Error("Deprecation header missing")
	}
	if resp.Header.Get("Sunset") == "" {
		t.Error("Sunset header missing")
	}
	if !strings.Contains(resp.Header.Get("Link"), "successor-version") {
		t.Error("Link successor-version missing")
	}
	if w := resp.Header.Get("Warning"); !strings.Contains(w, "299") {
		t.Errorf("Warning header missing 299 code: %q", w)
	}
}

func TestHTTPMiddleware_RetiredEndpoint_410Gone(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })
	r.Register(versioning.Endpoint{
		Method:   "/HTTP GET /api/v1/retired",
		Status:   versioning.StatusRetired,
		SunsetAt: now.Add(-time.Hour),
	})

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run for retired endpoint")
	})
	srv := httptest.NewServer(r.HTTPMiddleware(nil)(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/retired")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Errorf("status = %d, want 410", resp.StatusCode)
	}
	// Sunset header should still be on the 410 response so clients
	// can read it programmatically.
	if resp.Header.Get("Sunset") == "" {
		t.Error("retired response should still carry Sunset header")
	}
}

func TestHTTPMiddleware_CurrentEndpoint_NoHeaders(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method: "/HTTP GET /api/v1/active",
		Status: versioning.StatusCurrent,
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(r.HTTPMiddleware(nil)(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/active")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, name := range []string{"Deprecation", "Sunset", "Link", "Warning"} {
		if v := resp.Header.Get(name); v != "" {
			t.Errorf("current endpoint should not emit %s; got %q", name, v)
		}
	}
}

func TestHTTPMiddleware_UntrackedEndpoint_PassesThrough(t *testing.T) {
	r := versioning.NewRegistry()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(r.HTTPMiddleware(nil)(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/never-tracked")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestHTTPMiddleware_CustomKeyFunc(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "custom-key", Status: versioning.StatusDeprecated,
		DeprecatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	keyFn := func(*http.Request) string { return "custom-key" }
	srv := httptest.NewServer(r.HTTPMiddleware(keyFn)(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Deprecation") == "" {
		t.Error("custom keyFn should have routed through Deprecation lookup")
	}
}

// Sanity: the unary interceptor returns the codes.Unimplemented error
// in a form callers can pattern-match.
func TestUnaryInterceptor_RetiredErrorIsGRPCStatus(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })
	r.Register(versioning.Endpoint{
		Method:   "/svc/M",
		Status:   versioning.StatusRetired,
		SunsetAt: now.Add(-time.Hour),
	})

	_, err := r.UnaryServerInterceptor()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("error should be a grpc status")
	}
	if st.Code() != codes.Unimplemented {
		t.Errorf("code = %v, want Unimplemented", st.Code())
	}

	// errors.Is against a sentinel error should fall through (gRPC
	// status doesn't define one); just make sure it's not a panic.
	_ = errors.Is(err, errors.New("none"))
}
