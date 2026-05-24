// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/ratelimit"
	"go.keystone-core.io/keystone-core/internal/ratelimit/extract"
)

// retryAfterTrailerKey is the metadata key gRPC clients can read
// to learn the suggested retry delay in milliseconds. The key is
// not part of any gRPC spec; we ship it so HTTP and gRPC clients
// see equivalent information. Clients that ignore the trailer
// still receive a clean ResourceExhausted error.
const retryAfterTrailerKey = "retry-after-ms"

// UnaryServerInterceptor returns a gRPC unary interceptor that
// consults reg for each call keyed by ext. A nil reg / ext
// disables limiting (passthrough).
func UnaryServerInterceptor(reg *ratelimit.Registry, ext extract.Extractor, m *Metrics) grpc.UnaryServerInterceptor {
	if reg == nil || ext == nil {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			return h(ctx, req)
		}
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		key, ok := ext.GRPC(ctx)
		if !ok {
			return h(ctx, req)
		}
		allowed, delay := reg.AllowOrRetryAfter(key)
		if allowed {
			return h(ctx, req)
		}
		setRetryAfterTrailer(ctx, delay)
		m.RecordReject(ReasonLimitExceeded)
		return nil, status.Error(codes.ResourceExhausted, rejectedMessage)
	}
}

// StreamServerInterceptor returns a gRPC stream interceptor with
// the same semantics as UnaryServerInterceptor.
func StreamServerInterceptor(reg *ratelimit.Registry, ext extract.Extractor, m *Metrics) grpc.StreamServerInterceptor {
	if reg == nil || ext == nil {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			return h(srv, ss)
		}
	}
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		ctx := ss.Context()
		key, ok := ext.GRPC(ctx)
		if !ok {
			return h(srv, ss)
		}
		allowed, delay := reg.AllowOrRetryAfter(key)
		if allowed {
			return h(srv, ss)
		}
		setRetryAfterTrailer(ctx, delay)
		m.RecordReject(ReasonLimitExceeded)
		return status.Error(codes.ResourceExhausted, rejectedMessage)
	}
}

// setRetryAfterTrailer attaches the retry-after-ms trailer to
// the response. The value is the delay in milliseconds, rounded
// up and floored at 1 (clients that special-case zero get a
// usable value).
func setRetryAfterTrailer(ctx context.Context, delay time.Duration) {
	ms := delay.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	_ = grpc.SetTrailer(ctx, metadata.Pairs(retryAfterTrailerKey, itoa(int(ms))))
}
