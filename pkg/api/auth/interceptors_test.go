package auth

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

// mockAuthenticator is a test authenticator
type mockAuthenticator struct {
	name        string
	authFunc    func(ctx context.Context, credentials string) (*Principal, error)
}

func (m *mockAuthenticator) Name() string {
	return m.name
}

func (m *mockAuthenticator) Authenticate(ctx context.Context, credentials string) (*Principal, error) {
	return m.authFunc(ctx, credentials)
}

func TestAuthenticate_NoMetadata_NoMTLS(t *testing.T) {
	// Test that authentication fails when no metadata is provided
	// and no mTLS authenticator is configured
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "apikey",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					if credentials == "valid-key" {
						return &Principal{ID: "test", Name: "test", Role: RoleAdmin}, nil
					}
					return nil, ErrInvalidCredentials
				},
			},
		},
	}

	ctx := context.Background()
	_, err := authenticate(ctx, cfg)
	if err == nil {
		t.Error("authenticate() should fail with no metadata and no mTLS")
	}
}

func TestAuthenticate_NoMetadata_WithMTLS(t *testing.T) {
	// Test that mTLS authentication is attempted when no metadata is provided
	// but mTLS authenticator is configured
	mTLSAuthCalled := false
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "mtls",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					mTLSAuthCalled = true
					// Simulate mTLS authentication failure (no TLS peer info in test)
					return nil, ErrNoPeerInfo
				},
			},
		},
	}

	ctx := context.Background()
	_, err := authenticate(ctx, cfg)

	// It should still fail (no actual mTLS cert), but mTLS authenticator should have been called
	if !mTLSAuthCalled {
		t.Error("mTLS authenticator was not called when no metadata provided")
	}
	if err == nil {
		t.Error("authenticate() should fail without valid mTLS cert")
	}
}

func TestAuthenticate_EmptyCredentials_NoMTLS(t *testing.T) {
	// Test that authentication fails when metadata exists but no credentials
	// and no mTLS authenticator is configured
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "apikey",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					if credentials == "valid-key" {
						return &Principal{ID: "test", Name: "test", Role: RoleAdmin}, nil
					}
					return nil, ErrInvalidCredentials
				},
			},
		},
	}

	// Create context with empty metadata
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	_, err := authenticate(ctx, cfg)
	if err == nil {
		t.Error("authenticate() should fail with empty credentials and no mTLS")
	}
}

func TestAuthenticate_EmptyCredentials_WithMTLS(t *testing.T) {
	// Test that mTLS authentication is attempted when metadata exists but no credentials
	// and mTLS authenticator is configured
	mTLSAuthCalled := false
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "mtls",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					mTLSAuthCalled = true
					// Simulate mTLS authentication failure (no TLS peer info in test)
					return nil, ErrNoPeerInfo
				},
			},
		},
	}

	// Create context with empty metadata
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	_, err := authenticate(ctx, cfg)

	// It should still fail (no actual mTLS cert), but mTLS authenticator should have been called
	if !mTLSAuthCalled {
		t.Error("mTLS authenticator was not called with empty credentials")
	}
	if err == nil {
		t.Error("authenticate() should fail without valid mTLS cert")
	}
}

func TestAuthenticate_WithCredentials_APIKey(t *testing.T) {
	// Test normal authentication with API key credentials
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "apikey",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					if credentials == "valid-key" {
						return &Principal{ID: "test", Name: "test", Role: RoleAdmin}, nil
					}
					return nil, ErrInvalidCredentials
				},
			},
		},
	}

	// Create context with valid API key
	md := metadata.MD{
		"x-api-key": []string{"valid-key"},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	principal, err := authenticate(ctx, cfg)
	if err != nil {
		t.Errorf("authenticate() error = %v, want nil", err)
	}
	if principal == nil {
		t.Fatal("principal is nil")
	}
	if principal.ID != "test" {
		t.Errorf("principal.ID = %v, want test", principal.ID)
	}
}

func TestAuthenticate_WithBearerToken(t *testing.T) {
	// Test authentication with Bearer token in Authorization header
	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "jwt",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					if credentials == "jwt-token" {
						return &Principal{ID: "jwt-user", Name: "jwt-user", Role: RoleOperator}, nil
					}
					return nil, ErrInvalidCredentials
				},
			},
		},
	}

	// Create context with Bearer token
	md := metadata.MD{
		"authorization": []string{"Bearer jwt-token"},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	principal, err := authenticate(ctx, cfg)
	if err != nil {
		t.Errorf("authenticate() error = %v, want nil", err)
	}
	if principal == nil {
		t.Fatal("principal is nil")
	}
	if principal.ID != "jwt-user" {
		t.Errorf("principal.ID = %v, want jwt-user", principal.ID)
	}
}

func TestAuthenticate_MultipleAuthenticators_MTLSFirst(t *testing.T) {
	// Test that in multi-auth mode, mTLS is tried first and allows empty metadata
	mTLSAuthCalled := false
	apiKeyAuthCalled := false

	cfg := &InterceptorConfig{
		MetadataKey: "x-api-key",
		Authenticators: []Authenticator{
			&mockAuthenticator{
				name: "mtls",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					mTLSAuthCalled = true
					// Simulate successful mTLS authentication
					return &Principal{ID: "mtls-user", Name: "cert-cn", Role: RoleAdmin, AuthMethod: "mtls"}, nil
				},
			},
			&mockAuthenticator{
				name: "apikey",
				authFunc: func(ctx context.Context, credentials string) (*Principal, error) {
					apiKeyAuthCalled = true
					return nil, ErrNoCredentials
				},
			},
		},
	}

	// Create context with no metadata
	ctx := context.Background()
	principal, err := authenticate(ctx, cfg)

	if err != nil {
		t.Errorf("authenticate() error = %v, want nil", err)
	}
	if !mTLSAuthCalled {
		t.Error("mTLS authenticator was not called")
	}
	if apiKeyAuthCalled {
		t.Error("API key authenticator should not be called when mTLS succeeds")
	}
	if principal == nil {
		t.Fatal("principal is nil")
	}
	if principal.AuthMethod != "mtls" {
		t.Errorf("principal.AuthMethod = %v, want mtls", principal.AuthMethod)
	}
}

