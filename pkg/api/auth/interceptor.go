// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ClientKeyFunc derives the rate-limiter key for a request. Default:
// principal ID when authenticated, peer IP otherwise.
type ClientKeyFunc func(ctx context.Context, principal *Principal) string

// AuthDecisionFunc is called once per Authorize result on the gRPC
// path — both successful authorizations AND denials. Use it from boot
// wiring to emit audit entries (Epic 12 task 4's "every sensitive op
// MUST emit" rule). Returning is fire-and-forget; the interceptor
// does not check or propagate any error.
//
// Arguments:
//
//   - method:     full gRPC method ("/svc/method"). Always set.
//   - principal:  nil if Authenticate failed; non-nil on success.
//   - allowed:    true iff BOTH Authenticate AND Authorize succeeded.
//   - reason:     denial reason when !allowed; nil on allowed=true.
//                 Wraps ErrUnauthenticated / authorizer rejection.
//
// Bypass-list calls (auth-skipped methods) still invoke the callback
// with allowed=true and principal=nil — the audit row records "no
// authentication required for this method," which is the right
// signal for compliance review.
type AuthDecisionFunc func(ctx context.Context, method string, principal *Principal, allowed bool, reason error)

// InterceptorConfig wires the auth chain into gRPC + HTTP handlers.
//
// Chain order (PROJECT-DETAILS §4.5 acceptance criterion):
//
//	CORS  →  rate-limit  →  auth  →  authorize  →  handler
//
// CORS lives outside this package (epic 04 wires it for HTTP); this
// type owns rate-limit + auth + authorize.
type InterceptorConfig struct {
	// Authenticator is required: the chain that maps an inbound
	// request to a Principal.
	Authenticator Authenticator

	// Authorizer is required: the policy that decides whether the
	// principal may invoke the requested method.
	Authorizer Authorizer

	// RateLimiter is optional. Nil disables rate limiting.
	RateLimiter *RateLimiter

	// ClientKeyFunc derives the rate-limit bucket key. Optional —
	// defaults to PrincipalIDOrPeerIP.
	ClientKeyFunc ClientKeyFunc

	// OnAuthDecision is invoked exactly once per Authorize result
	// (success + failure). Optional — nil disables audit emission.
	// Boot wiring constructs the callback so this package keeps
	// zero internal-package dependencies.
	OnAuthDecision AuthDecisionFunc
}

// validate ensures required fields are set.
func (c *InterceptorConfig) validate() error {
	if c.Authenticator == nil {
		return errors.New("auth: InterceptorConfig.Authenticator is required")
	}
	if c.Authorizer == nil {
		return errors.New("auth: InterceptorConfig.Authorizer is required")
	}
	return nil
}

// PrincipalIDOrPeerIP is the default ClientKeyFunc: returns the
// principal ID when authenticated, otherwise the peer IP from the
// gRPC context. Falls back to "unknown" if neither is available.
func PrincipalIDOrPeerIP(ctx context.Context, principal *Principal) string {
	if principal != nil && principal.ID != "" {
		return "principal:" + principal.ID
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err == nil && host != "" {
			return "ip:" + host
		}
		return "ip:" + p.Addr.String()
	}
	return "unknown"
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that
// runs the chain rate-limit → auth → authorize → handler.
func (c *InterceptorConfig) UnaryServerInterceptor() (grpc.UnaryServerInterceptor, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	keyFn := c.ClientKeyFunc
	if keyFn == nil {
		keyFn = PrincipalIDOrPeerIP
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := c.runAuthChain(ctx, info.FullMethod, keyFn)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}, nil
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor with
// the same chain semantics as UnaryServerInterceptor.
func (c *InterceptorConfig) StreamServerInterceptor() (grpc.StreamServerInterceptor, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	keyFn := c.ClientKeyFunc
	if keyFn == nil {
		keyFn = PrincipalIDOrPeerIP
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := c.runAuthChain(ss.Context(), info.FullMethod, keyFn)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}, nil
}

// runAuthChain is the shared rate-limit → auth → authorize sequence.
// Returns the (possibly principal-augmented) context on success or a
// gRPC status error on failure.
func (c *InterceptorConfig) runAuthChain(
	ctx context.Context,
	method string,
	keyFn ClientKeyFunc,
) (context.Context, error) {
	// Rate-limit pre-auth (keyed by peer IP since we don't yet know
	// the principal). Keeps brute-force attempts bounded.
	preKey := keyFn(ctx, nil)
	if c.RateLimiter != nil {
		if err := c.RateLimiter.Allow(preKey); err != nil {
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		}
	}

	principal, authErr := c.Authenticator.Authenticate(ctx)

	// Rate-limit accounting. Successful auth clears state; failed
	// auth (other than ErrUnauthenticated for bypass paths) records
	// a failure on the pre-auth key.
	if c.RateLimiter != nil {
		switch {
		case authErr == nil:
			c.RateLimiter.RecordSuccess(preKey)
		case errors.Is(authErr, ErrInvalidCredentials):
			c.RateLimiter.RecordFailure(preKey)
		}
	}

	// Authorization: pass principal (may be nil if auth failed). The
	// authorizer's bypass list lets unauthenticated calls through to
	// the handler when the method is on the bypass set; mTLS-required
	// methods reject any non-mTLS principal here.
	authzErr := c.Authorizer.Authorize(ctx, principal, method)
	allowed := authzErr == nil
	// Audit-emission hook fires once per Authorize result on both
	// success AND failure (§4.12 "every sensitive op MUST emit").
	if c.OnAuthDecision != nil {
		reason := authzErr
		if reason == nil && authErr != nil && principal == nil {
			// Bypass paths: Authenticate may have failed but Authorize
			// admitted the call. Surface the auth error as context.
			reason = authErr
		}
		c.OnAuthDecision(ctx, method, principal, allowed, reason)
	}
	if authzErr != nil {
		// Auth failure on a non-bypass method: surface with the right
		// gRPC code. ErrUnauthorized -> PermissionDenied.
		if errors.Is(authErr, ErrUnauthenticated) || principal == nil {
			return nil, status.Error(codes.Unauthenticated, authzErr.Error())
		}
		return nil, status.Error(codes.PermissionDenied, authzErr.Error())
	}

	if principal != nil {
		ctx = WithPrincipal(ctx, principal)
	}
	return ctx, nil
}

// wrappedStream replaces grpc.ServerStream's Context() so the handler
// sees the principal-augmented context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// HTTPMiddleware returns a net/http middleware that wraps next with
// the same rate-limit → auth → authorize chain. The Authenticator
// must be able to extract its credential from the request context;
// the middleware injects standard `authorization` and TLS-state
// equivalents before delegating.
//
// HTTPMiddleware does NOT handle CORS — wrap it with a separate CORS
// middleware to satisfy the full CORS → rate-limit → auth → handler
// order.
func (c *InterceptorConfig) HTTPMiddleware() (func(http.Handler) http.Handler, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	keyFn := c.ClientKeyFunc
	if keyFn == nil {
		keyFn = PrincipalIDOrPeerIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := injectHTTPCredentialsToContext(r)
			method := httpMethodKey(r)

			ctx, err := c.runAuthChain(ctx, method, keyFn)
			if err != nil {
				st, ok := status.FromError(err)
				if !ok {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				http.Error(w, st.Message(), httpStatusFor(st.Code()))
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

// httpMethodKey synthesizes a stable bypass-/RBAC-lookup key from an
// HTTP request. Default form: "/HTTP <METHOD> <path>" so it doesn't
// collide with gRPC-method keys (which start with "/<package>...").
//
// Real HTTP-handler wiring (epic 03 task 7) provides per-route
// metadata that translates the URL to the underlying gRPC method
// name, but for the v1.0 middleware-level fallback this synthetic key
// is enough — operators can register HTTP-specific bypasses.
func httpMethodKey(r *http.Request) string {
	return "/HTTP " + r.Method + " " + r.URL.Path
}

// httpStatusFor maps gRPC codes to HTTP status codes for the HTTP
// middleware error path.
func httpStatusFor(c codes.Code) int {
	switch c {
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// injectHTTPCredentialsToContext copies the relevant HTTP request
// state (Authorization header, TLS peer info) into context values
// shaped like the gRPC equivalents so the same Authenticators work
// across both transports. Implementation detail of HTTPMiddleware.
//
// gRPC metadata is what the apikey/jwt extractors look at, so we
// build a metadata.MD with the Authorization header. peer.Peer with
// TLSInfo for mTLS uses r.TLS.
func injectHTTPCredentialsToContext(r *http.Request) context.Context {
	ctx := r.Context()
	ctx = injectAuthHeaderAsMetadata(ctx, r.Header.Get("Authorization"))
	if r.TLS != nil {
		ctx = injectTLSStateAsPeerInfo(ctx, r)
	}
	return ctx
}

// trimSchemeFromAddr returns r.RemoteAddr with any leading scheme
// stripped; net.SplitHostPort then extracts the IP.
func trimSchemeFromAddr(addr string) string {
	if i := strings.Index(addr, "://"); i >= 0 {
		return addr[i+3:]
	}
	return addr
}
