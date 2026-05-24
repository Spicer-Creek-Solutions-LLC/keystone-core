// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/internal/logging"
)

func TestHTTPCorrelationMiddleware_PassesThroughInbound(t *testing.T) {
	var observed string
	h := HTTPCorrelationMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = logging.CorrelationIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(logging.HTTPHeader, "trace-abc")
	h.ServeHTTP(rec, req)

	if observed != "trace-abc" {
		t.Errorf("ctx id = %q, want trace-abc", observed)
	}
	if got := rec.Header().Get(logging.HTTPHeader); got != "trace-abc" {
		t.Errorf("response header = %q, want trace-abc", got)
	}
}

func TestHTTPCorrelationMiddleware_GeneratesWhenAbsent(t *testing.T) {
	var observed string
	h := HTTPCorrelationMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = logging.CorrelationIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if observed == "" {
		t.Errorf("ctx id is empty; middleware should have generated one")
	}
	if rec.Header().Get(logging.HTTPHeader) != observed {
		t.Errorf("response header mismatch ctx id")
	}
}

func TestHTTPCorrelationMiddleware_RejectsMalformedHeader(t *testing.T) {
	tests := []struct{ name, in string }{
		{"newline", "trace\nid"},
		{"trailing space", "abc "},
		{"too long", strings.Repeat("a", 1024)},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var observed string
			h := HTTPCorrelationMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				observed = logging.CorrelationIDFromContext(r.Context())
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			if tt.in != "" {
				req.Header.Set(logging.HTTPHeader, tt.in)
			}
			h.ServeHTTP(rec, req)
			if observed == tt.in && tt.in != "" {
				t.Errorf("middleware accepted malformed inbound %q", tt.in)
			}
			if observed == "" {
				t.Errorf("ctx id empty; middleware should have generated a fresh one")
			}
		})
	}
}

type unaryStub struct{ observed string }

func (s *unaryStub) handle(ctx context.Context, _ any) (any, error) {
	s.observed = logging.CorrelationIDFromContext(ctx)
	return "ok", nil
}

func TestUnaryCorrelationInterceptor_PassesInbound(t *testing.T) {
	itc := UnaryCorrelationInterceptor()
	md := metadata.Pairs(logging.GRPCMetadataKey, "rpc-id-1")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	s := &unaryStub{}
	_, _ = itc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"}, s.handle)
	if s.observed != "rpc-id-1" {
		t.Errorf("ctx id = %q, want rpc-id-1", s.observed)
	}
}

func TestUnaryCorrelationInterceptor_GeneratesWhenAbsent(t *testing.T) {
	itc := UnaryCorrelationInterceptor()
	ctx := context.Background()
	s := &unaryStub{}
	_, _ = itc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"}, s.handle)
	if s.observed == "" {
		t.Errorf("ctx id empty; want generated")
	}
}

// correlationFakeStream gives the stream interceptor a Context() to read.
type correlationFakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *correlationFakeStream) Context() context.Context              { return f.ctx }
func (f *correlationFakeStream) SetTrailer(metadata.MD)                {}
func (f *correlationFakeStream) SetHeader(metadata.MD) error           { return nil }
func (f *correlationFakeStream) SendHeader(metadata.MD) error          { return nil }
func (f *correlationFakeStream) RecvMsg(any) error                     { return nil }
func (f *correlationFakeStream) SendMsg(any) error                     { return nil }

func TestStreamCorrelationInterceptor_PassesInbound(t *testing.T) {
	itc := StreamCorrelationInterceptor()
	md := metadata.Pairs(logging.GRPCMetadataKey, "stream-id-2")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var observed string
	handler := func(_ any, ss grpc.ServerStream) error {
		observed = logging.CorrelationIDFromContext(ss.Context())
		return nil
	}
	_ = itc(nil, &correlationFakeStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/svc/S"}, handler)
	if observed != "stream-id-2" {
		t.Errorf("ctx id = %q, want stream-id-2", observed)
	}
}

func TestStreamCorrelationInterceptor_GeneratesWhenAbsent(t *testing.T) {
	itc := StreamCorrelationInterceptor()
	var observed string
	handler := func(_ any, ss grpc.ServerStream) error {
		observed = logging.CorrelationIDFromContext(ss.Context())
		return nil
	}
	_ = itc(nil, &correlationFakeStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: "/svc/S"}, handler)
	if observed == "" {
		t.Errorf("ctx id empty; want generated")
	}
}

func TestIsAcceptableCorrelationID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"abc-123", true},
		{strings.Repeat("a", 128), true},
		{strings.Repeat("a", 129), false},
		{"abc\n", false},
		{"abc\x00", false},
		{" abc", false},
		{"abc ", false},
		{"abc def", true}, // internal space is fine
	}
	for _, tt := range tests {
		if got := isAcceptableCorrelationID(tt.in); got != tt.want {
			t.Errorf("isAcceptableCorrelationID(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
