package versioning

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that
// refuses retired endpoints with codes.Unimplemented and attaches
// deprecation metadata for every other tracked-and-advisory endpoint.
func (r *Registry) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if r.IsRetired(info.FullMethod) {
			return nil, retiredError(r, info.FullMethod)
		}
		if md := r.MetadataFor(info.FullMethod); md != nil {
			_ = grpc.SetHeader(ctx, md)
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor mirrors UnaryServerInterceptor for streaming
// RPCs.
func (r *Registry) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if r.IsRetired(info.FullMethod) {
			return retiredError(r, info.FullMethod)
		}
		if md := r.MetadataFor(info.FullMethod); md != nil {
			_ = ss.SetHeader(md)
		}
		return handler(srv, ss)
	}
}

// retiredError builds the gRPC error returned for a retired endpoint.
func retiredError(r *Registry, method string) error {
	e, _ := r.Lookup(method)
	msg := "endpoint retired"
	if !e.SunsetAt.IsZero() {
		msg = fmt.Sprintf("endpoint retired (sunset on %s)",
			e.SunsetAt.UTC().Format("2006-01-02"))
	}
	return status.Error(codes.Unimplemented, msg)
}

// HTTPMethodKeyFunc derives the registry-lookup key from an inbound
// HTTP request. Default is `/HTTP <METHOD> <path>` — same shape the
// auth middleware uses, lets operators register HTTP-specific keys
// alongside gRPC method keys in one Registry.
type HTTPMethodKeyFunc func(*http.Request) string

// DefaultHTTPMethodKey is the HTTPMethodKeyFunc used when none is
// supplied to HTTPMiddleware.
func DefaultHTTPMethodKey(r *http.Request) string {
	return "/HTTP " + r.Method + " " + r.URL.Path
}

// HTTPMiddleware returns a net/http middleware that:
//
//   - returns 410 Gone for retired endpoints (before invoking next);
//   - sets RFC 9745 / 8594 / 8288 / Warning headers on the response
//     for endpoints with advisory state.
//
// keyFn maps the request to the registry-lookup key. Pass nil to use
// DefaultHTTPMethodKey.
func (r *Registry) HTTPMiddleware(keyFn HTTPMethodKeyFunc) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = DefaultHTTPMethodKey
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			method := keyFn(req)
			if r.IsRetired(method) {
				e, _ := r.Lookup(method)
				for k, v := range r.HeadersFor(method) {
					w.Header()[k] = v
				}
				msg := "endpoint retired"
				if !e.SunsetAt.IsZero() {
					msg = fmt.Sprintf("endpoint retired (sunset on %s)",
						e.SunsetAt.UTC().Format("2006-01-02"))
				}
				http.Error(w, msg, http.StatusGone)
				return
			}
			for k, v := range r.HeadersFor(method) {
				w.Header()[k] = v
			}
			next.ServeHTTP(w, req)
		})
	}
}
