package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/config"
)

// InterceptorConfig contains configuration for auth interceptors
type InterceptorConfig struct {
	// Authenticators is a list of authenticators to try in order
	Authenticators []Authenticator
	// Authorizer for RBAC checks
	Authorizer Authorizer
	// MetadataKey is the gRPC metadata key for the API key/token
	MetadataKey string
	// BypassMethods is a list of methods that don't require authentication
	BypassMethods []string
	// AuditLogger is called for authentication events (optional)
	AuditLogger func(ctx context.Context, method string, principal *Principal, err error)
	// RateLimiter for limiting failed authentication attempts (optional but recommended)
	RateLimiter *RateLimiter
}

// NewInterceptorConfigFromConfig creates interceptor config from app config
func NewInterceptorConfigFromConfig(cfg config.AuthConfig) (*InterceptorConfig, error) {
	ic := &InterceptorConfig{
		MetadataKey:   cfg.APIKey.MetadataKey,
		BypassMethods: cfg.BypassMethods,
	}

	if ic.MetadataKey == "" {
		ic.MetadataKey = "x-api-key"
	}

	// Create authenticators based on config
	switch cfg.Type {
	case "apikey":
		auth, err := NewAPIKeyAuthenticator(cfg.APIKey)
		if err != nil {
			return nil, err
		}
		ic.Authenticators = []Authenticator{auth}

	case "jwt":
		// JWT-only authentication
		auth, err := NewJWTAuthenticator(cfg.JWT)
		if err != nil {
			return nil, err
		}
		ic.Authenticators = []Authenticator{auth}

	case "multi":
		// Add mTLS authenticator first (highest priority for certificate-based auth)
		if cfg.MTLS.RequireClientCert || len(cfg.MTLS.CertRoles) > 0 {
			auth, err := NewMTLSAuthenticator(cfg.MTLS)
			if err != nil {
				return nil, err
			}
			ic.Authenticators = append(ic.Authenticators, auth)
		}
		// Add API key authenticator if keys are configured
		if len(cfg.APIKey.Keys) > 0 {
			auth, err := NewAPIKeyAuthenticator(cfg.APIKey)
			if err != nil {
				return nil, err
			}
			ic.Authenticators = append(ic.Authenticators, auth)
		}
		// Add JWT authenticator if configured
		if cfg.JWT.Secret != "" || cfg.JWT.PublicKeyFile != "" {
			auth, err := NewJWTAuthenticator(cfg.JWT)
			if err != nil {
				return nil, err
			}
			ic.Authenticators = append(ic.Authenticators, auth)
		}

	case "mtls":
		// mTLS-only authentication
		auth, err := NewMTLSAuthenticator(cfg.MTLS)
		if err != nil {
			return nil, err
		}
		ic.Authenticators = []Authenticator{auth}
	}

	// Create authorizer with authorization config
	ic.Authorizer = NewRBACAuthorizerWithConfig(cfg.BypassMethods, cfg.Authorization.Enabled, cfg.Authorization.DefaultDeny)

	// Create rate limiter with defaults (can be customized via SetRateLimiter)
	ic.RateLimiter = NewRateLimiter(DefaultRateLimitConfig())

	return ic, nil
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for authentication
func UnaryServerInterceptor(cfg *InterceptorConfig) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Check if method bypasses auth
		if isBypassMethod(info.FullMethod, cfg.BypassMethods) {
			return handler(ctx, req)
		}

		// Authenticate
		principal, err := authenticate(ctx, cfg)
		if err != nil {
			if cfg.AuditLogger != nil {
				cfg.AuditLogger(ctx, info.FullMethod, nil, err)
			}
			return nil, err
		}

		// Authorize
		if cfg.Authorizer != nil {
			if err := cfg.Authorizer.Authorize(ctx, principal, info.FullMethod); err != nil {
				if cfg.AuditLogger != nil {
					cfg.AuditLogger(ctx, info.FullMethod, principal, err)
				}
				return nil, status.Errorf(codes.PermissionDenied, "authorization failed: %v", err)
			}
		}

		// Log successful auth
		if cfg.AuditLogger != nil {
			cfg.AuditLogger(ctx, info.FullMethod, principal, nil)
		}

		// Add principal to context and call handler
		ctx = ContextWithPrincipal(ctx, principal)
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for authentication
func StreamServerInterceptor(cfg *InterceptorConfig) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Check if method bypasses auth
		if isBypassMethod(info.FullMethod, cfg.BypassMethods) {
			return handler(srv, ss)
		}

		ctx := ss.Context()

		// Authenticate
		principal, err := authenticate(ctx, cfg)
		if err != nil {
			if cfg.AuditLogger != nil {
				cfg.AuditLogger(ctx, info.FullMethod, nil, err)
			}
			return err
		}

		// Authorize
		if cfg.Authorizer != nil {
			if err := cfg.Authorizer.Authorize(ctx, principal, info.FullMethod); err != nil {
				if cfg.AuditLogger != nil {
					cfg.AuditLogger(ctx, info.FullMethod, principal, err)
				}
				return status.Errorf(codes.PermissionDenied, "authorization failed: %v", err)
			}
		}

		// Log successful auth
		if cfg.AuditLogger != nil {
			cfg.AuditLogger(ctx, info.FullMethod, principal, nil)
		}

		// Wrap stream with authenticated context
		wrappedStream := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          ContextWithPrincipal(ctx, principal),
		}

		return handler(srv, wrappedStream)
	}
}

// authenticatedServerStream wraps a grpc.ServerStream with an authenticated context
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}

// authenticate extracts credentials from metadata and authenticates
func authenticate(ctx context.Context, cfg *InterceptorConfig) (*Principal, error) {
	// Check rate limit before attempting authentication
	if cfg.RateLimiter != nil {
		if err := cfg.RateLimiter.CheckRateLimit(ctx); err != nil {
			return nil, err
		}
	}

	// Check if we have any mTLS authenticators (they don't need metadata credentials)
	hasMTLSAuth := false
	for _, auth := range cfg.Authenticators {
		if auth.Name() == "mtls" {
			hasMTLSAuth = true
			break
		}
	}

	// Extract metadata (may be empty for mTLS-only deployments)
	md, ok := metadata.FromIncomingContext(ctx)

	// Try to get credentials from metadata
	credentials := ""
	if ok {
		if values := md.Get(cfg.MetadataKey); len(values) > 0 {
			credentials = values[0]
		}

		// Also check for Authorization header (Bearer token format)
		if credentials == "" {
			if values := md.Get("authorization"); len(values) > 0 {
				auth := values[0]
				if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
					credentials = strings.TrimPrefix(auth, "Bearer ")
					credentials = strings.TrimPrefix(credentials, "bearer ")
				}
			}
		}
	}

	// For non-mTLS configurations, require credentials from metadata
	// mTLS authenticators extract credentials from TLS peer info, not metadata
	if credentials == "" && !hasMTLSAuth {
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "no metadata provided")
		}
		return nil, status.Errorf(codes.Unauthenticated, "no credentials provided")
	}

	// Try each authenticator
	var lastErr error
	for _, auth := range cfg.Authenticators {
		principal, err := auth.Authenticate(ctx, credentials)
		if err == nil {
			// Record successful authentication (clears failure count)
			if cfg.RateLimiter != nil {
				cfg.RateLimiter.RecordSuccess(ClientIDFromContext(ctx))
			}
			return principal, nil
		}
		lastErr = err
	}

	// Record failed authentication attempt
	if cfg.RateLimiter != nil {
		clientID := ClientIDFromContext(ctx)
		cfg.RateLimiter.RecordFailure(clientID)

		// Check if this failure triggered a lockout
		if !cfg.RateLimiter.IsAllowed(clientID) {
			remaining := cfg.RateLimiter.GetLockoutRemaining(clientID)
			return nil, status.Errorf(codes.ResourceExhausted,
				"authentication failed; account locked for %s due to too many failed attempts",
				remaining.Round(time.Second))
		}

		// Add remaining attempts info to error
		failCount := cfg.RateLimiter.GetFailureCount(clientID)
		attemptsRemaining := cfg.RateLimiter.config.MaxFailures - failCount
		if attemptsRemaining > 0 && lastErr != nil {
			// Map auth errors to gRPC status codes with attempts remaining
			if errors.Is(lastErr, ErrNoCredentials) {
				return nil, status.Errorf(codes.Unauthenticated, "no credentials provided (%d attempts remaining)", attemptsRemaining)
			}
			if errors.Is(lastErr, ErrInvalidCredentials) {
				return nil, status.Errorf(codes.Unauthenticated, "invalid credentials (%d attempts remaining)", attemptsRemaining)
			}
			if errors.Is(lastErr, ErrExpiredCredentials) {
				return nil, status.Errorf(codes.Unauthenticated, "credentials expired (%d attempts remaining)", attemptsRemaining)
			}
			if errors.Is(lastErr, ErrDisabledKey) {
				return nil, status.Errorf(codes.Unauthenticated, "API key is disabled (%d attempts remaining)", attemptsRemaining)
			}
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v (%d attempts remaining)", lastErr, attemptsRemaining)
		}
	}

	// Map auth errors to gRPC status codes (when rate limiter is disabled or nil)
	if lastErr != nil {
		if errors.Is(lastErr, ErrNoCredentials) {
			return nil, status.Errorf(codes.Unauthenticated, "no credentials provided")
		}
		if errors.Is(lastErr, ErrInvalidCredentials) {
			return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
		}
		if errors.Is(lastErr, ErrExpiredCredentials) {
			return nil, status.Errorf(codes.Unauthenticated, "credentials expired")
		}
		if errors.Is(lastErr, ErrDisabledKey) {
			return nil, status.Errorf(codes.Unauthenticated, "API key is disabled")
		}
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", lastErr)
	}

	return nil, status.Errorf(codes.Unauthenticated, "no authenticator could validate credentials")
}

// isBypassMethod checks if a method should bypass authentication
func isBypassMethod(method string, bypassMethods []string) bool {
	for _, bypass := range bypassMethods {
		if method == bypass {
			return true
		}
		// Support wildcard matching for service prefix
		if strings.HasSuffix(bypass, "/*") {
			prefix := strings.TrimSuffix(bypass, "/*")
			if strings.HasPrefix(method, prefix) {
				return true
			}
		}
	}
	return false
}

// ChainUnaryServerInterceptors chains multiple unary interceptors
func ChainUnaryServerInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Build chain from end to start
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return chain(ctx, req)
	}
}

// ChainStreamServerInterceptors chains multiple stream interceptors
func ChainStreamServerInterceptors(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Build chain from end to start
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(srv interface{}, ss grpc.ServerStream) error {
				return interceptor(srv, ss, info, next)
			}
		}
		return chain(srv, ss)
	}
}
