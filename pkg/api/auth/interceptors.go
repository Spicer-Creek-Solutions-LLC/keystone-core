package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/pkg/config"
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

	case "multi":
		// Add API key authenticator if keys are configured
		if len(cfg.APIKey.Keys) > 0 {
			auth, err := NewAPIKeyAuthenticator(cfg.APIKey)
			if err != nil {
				return nil, err
			}
			ic.Authenticators = append(ic.Authenticators, auth)
		}
		// JWT and mTLS would be added here when implemented

	case "jwt", "mtls":
		// TODO: Implement JWT and mTLS authenticators
		return nil, status.Errorf(codes.Unimplemented, "auth type %q not yet implemented", cfg.Type)
	}

	// Create authorizer
	ic.Authorizer = NewRBACAuthorizer(cfg.BypassMethods)

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
	// Extract metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "no metadata provided")
	}

	// Try to get credentials from metadata
	credentials := ""
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

	if credentials == "" {
		return nil, status.Errorf(codes.Unauthenticated, "no credentials provided")
	}

	// Try each authenticator
	var lastErr error
	for _, auth := range cfg.Authenticators {
		principal, err := auth.Authenticate(ctx, credentials)
		if err == nil {
			return principal, nil
		}
		lastErr = err
	}

	// Map auth errors to gRPC status codes
	if lastErr != nil {
		switch lastErr {
		case ErrNoCredentials:
			return nil, status.Errorf(codes.Unauthenticated, "no credentials provided")
		case ErrInvalidCredentials:
			return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
		case ErrExpiredCredentials:
			return nil, status.Errorf(codes.Unauthenticated, "credentials expired")
		case ErrDisabledKey:
			return nil, status.Errorf(codes.Unauthenticated, "API key is disabled")
		default:
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", lastErr)
		}
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
