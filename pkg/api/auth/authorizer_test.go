// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func TestRBACAuthorizer_BypassMethods(t *testing.T) {
	a := auth.NewRBACAuthorizer()

	// Bypass methods succeed with no principal. Phase B5 finding
	// H2 narrowed the bypass set: only AgentService/Register stays
	// bypass-eligible (bootstrap PSK in payload is the
	// authenticator; the agent has no mTLS cert yet). Heartbeat +
	// SubmitCommandStream graduated to mTLS-required and now have
	// their own test below.
	for _, method := range []string{
		"/keystone.core.v1.AgentService/Register",
		"/grpc.health.v1.Health/Check",
	} {
		t.Run(method, func(t *testing.T) {
			if err := a.Authorize(context.Background(), nil, method); err != nil {
				t.Errorf("bypass %q: %v", method, err)
			}
		})
	}
}

// Phase B5 finding H2: Heartbeat + SubmitCommandStream are
// post-bootstrap calls; the agent has its SPIFFE SVID by then, so
// the auth layer enforces mTLS as defense-in-depth on top of the
// gRPC transport's TLS config. Unauthenticated callers + callers
// using API-key or JWT all fail; mTLS principals pass.
func TestRBACAuthorizer_AgentPostBootstrapRequiresMTLS(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	for _, method := range []string{
		"/keystone.core.v1.AgentService/Heartbeat",
		"/keystone.core.v1.AgentService/SubmitCommandStream",
	} {
		t.Run(method+"/nil principal denied", func(t *testing.T) {
			if err := a.Authorize(context.Background(), nil, method); err == nil {
				t.Errorf("expected mTLS-required error, got nil")
			}
		})
		t.Run(method+"/API-key principal denied", func(t *testing.T) {
			p := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey}
			if err := a.Authorize(context.Background(), p, method); err == nil {
				t.Errorf("expected mTLS-required error, got nil")
			}
		})
		t.Run(method+"/mTLS principal allowed", func(t *testing.T) {
			p := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodMTLS}
			if err := a.Authorize(context.Background(), p, method); err != nil {
				t.Errorf("expected success for mTLS principal, got %v", err)
			}
		})
	}
}

func TestRBACAuthorizer_RoleHierarchy(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.ControlPlaneService/ServerStatus" // readonly minimum

	tests := []struct {
		name string
		p    *auth.Principal
		want bool // true = expect Authorize to succeed
	}{
		{"admin allowed", &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey}, true},
		{"operator allowed", &auth.Principal{Role: auth.RoleOperator, AuthMethod: auth.AuthMethodAPIKey}, true},
		{"readonly allowed", &auth.Principal{Role: auth.RoleReadonly, AuthMethod: auth.AuthMethodAPIKey}, true},
		{"none denied", &auth.Principal{Role: auth.RoleNone, AuthMethod: auth.AuthMethodAPIKey}, false},
		{"nil denied", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.Authorize(context.Background(), tt.p, method)
			if tt.want && err != nil {
				t.Errorf("expected success, got %v", err)
			}
			if !tt.want && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestRBACAuthorizer_ReadonlyDeniedOperatorMethods(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.ControlPlaneService/ExecuteCommand" // operator min
	p := &auth.Principal{Role: auth.RoleReadonly, AuthMethod: auth.AuthMethodAPIKey}

	err := a.Authorize(context.Background(), p, method)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), "operator") {
		t.Errorf("error should mention required role 'operator': %v", err)
	}
}

func TestRBACAuthorizer_AdminAllowedAdminMethods(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.SecretsService/WriteSecret" // admin min
	p := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey}

	if err := a.Authorize(context.Background(), p, method); err != nil {
		t.Errorf("admin should be allowed: %v", err)
	}
}

func TestRBACAuthorizer_OperatorDeniedAdminMethods(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.SecretsService/WriteSecret"
	p := &auth.Principal{Role: auth.RoleOperator, AuthMethod: auth.AuthMethodAPIKey}

	if err := a.Authorize(context.Background(), p, method); !errors.Is(err, auth.ErrUnauthorized) {
		t.Errorf("operator on admin method: err = %v, want ErrUnauthorized", err)
	}
}

func TestRBACAuthorizer_CoordinationRequiresMTLS(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.CoordinationService/ClusterHealth"

	tests := []struct {
		name    string
		p       *auth.Principal
		wantErr bool
	}{
		{
			name:    "admin via API key denied (not mTLS)",
			p:       &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey},
			wantErr: true,
		},
		{
			name:    "admin via JWT denied (not mTLS)",
			p:       &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodJWT},
			wantErr: true,
		},
		{
			name:    "admin via mTLS allowed",
			p:       &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodMTLS},
			wantErr: false,
		},
		{
			name:    "operator via mTLS denied (not admin role)",
			p:       &auth.Principal{Role: auth.RoleOperator, AuthMethod: auth.AuthMethodMTLS},
			wantErr: true,
		},
		{
			name:    "nil principal denied",
			p:       nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.Authorize(context.Background(), tt.p, method)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected success, got %v", err)
			}
		})
	}
}

func TestRBACAuthorizer_UnknownMethodDefaultsToAdmin(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.UnknownService/UnknownMethod"

	// Default-deny: unknown method requires admin.
	p := &auth.Principal{Role: auth.RoleOperator, AuthMethod: auth.AuthMethodAPIKey}
	if err := a.Authorize(context.Background(), p, method); !errors.Is(err, auth.ErrUnauthorized) {
		t.Errorf("operator on unknown method: err = %v, want ErrUnauthorized", err)
	}

	admin := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey}
	if err := a.Authorize(context.Background(), admin, method); err != nil {
		t.Errorf("admin on unknown method: %v", err)
	}
}

func TestRBACAuthorizer_Overrides(t *testing.T) {
	a := auth.NewRBACAuthorizer()

	// Tests can override individual method requirements without
	// reaching into private state.
	const method = "/keystone.core.v1.ControlPlaneService/ServerStatus"
	a.SetMethodRequirement(method, auth.RoleAdmin)

	p := &auth.Principal{Role: auth.RoleReadonly, AuthMethod: auth.AuthMethodAPIKey}
	if err := a.Authorize(context.Background(), p, method); !errors.Is(err, auth.ErrUnauthorized) {
		t.Errorf("override raised requirement; readonly should be denied; got %v", err)
	}
}

func TestRBACAuthorizer_AddBypass(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.SecretsService/WriteSecret"

	a.AddBypass(method)
	if err := a.Authorize(context.Background(), nil, method); err != nil {
		t.Errorf("AddBypass should suppress auth: %v", err)
	}
}

func TestRBACAuthorizer_AddMTLSRequirement(t *testing.T) {
	a := auth.NewRBACAuthorizer()
	const method = "/keystone.core.v1.SecretsService/Encrypt" // operator default

	a.AddMTLSRequirement(method)

	apiKey := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodAPIKey}
	if err := a.Authorize(context.Background(), apiKey, method); !errors.Is(err, auth.ErrUnauthorized) {
		t.Errorf("API-key admin on mtls-required: err = %v, want ErrUnauthorized", err)
	}

	mtls := &auth.Principal{Role: auth.RoleAdmin, AuthMethod: auth.AuthMethodMTLS}
	if err := a.Authorize(context.Background(), mtls, method); err != nil {
		t.Errorf("mTLS admin should pass: %v", err)
	}
}
