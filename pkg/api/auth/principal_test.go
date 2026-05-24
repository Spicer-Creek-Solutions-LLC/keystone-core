// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func TestRole_String(t *testing.T) {
	tests := []struct {
		role auth.Role
		want string
	}{
		{auth.RoleAdmin, "admin"},
		{auth.RoleOperator, "operator"},
		{auth.RoleReadonly, "readonly"},
		{auth.RoleNone, "none"},
		{auth.Role(99), "none"}, // unknown -> "none"
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.role.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRole(t *testing.T) {
	tests := []struct {
		in      string
		want    auth.Role
		wantErr bool
	}{
		{"admin", auth.RoleAdmin, false},
		{"operator", auth.RoleOperator, false},
		{"readonly", auth.RoleReadonly, false},
		{"", auth.RoleNone, false},
		{"superuser", auth.RoleNone, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := auth.ParseRole(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthMethod_String(t *testing.T) {
	tests := []struct {
		m    auth.AuthMethod
		want string
	}{
		{auth.AuthMethodAPIKey, "api-key"},
		{auth.AuthMethodJWT, "jwt"},
		{auth.AuthMethodMTLS, "mtls"},
		{auth.AuthMethodNone, "none"},
		{auth.AuthMethod(99), "none"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrincipal_HasRole(t *testing.T) {
	tests := []struct {
		name string
		p    *auth.Principal
		min  auth.Role
		want bool
	}{
		{"nil principal vs RoleNone", nil, auth.RoleNone, true},
		{"nil principal vs RoleReadonly", nil, auth.RoleReadonly, false},
		{"admin satisfies admin", &auth.Principal{Role: auth.RoleAdmin}, auth.RoleAdmin, true},
		{"admin satisfies operator", &auth.Principal{Role: auth.RoleAdmin}, auth.RoleOperator, true},
		{"admin satisfies readonly", &auth.Principal{Role: auth.RoleAdmin}, auth.RoleReadonly, true},
		{"operator does not satisfy admin", &auth.Principal{Role: auth.RoleOperator}, auth.RoleAdmin, false},
		{"operator satisfies operator", &auth.Principal{Role: auth.RoleOperator}, auth.RoleOperator, true},
		{"readonly satisfies readonly", &auth.Principal{Role: auth.RoleReadonly}, auth.RoleReadonly, true},
		{"readonly does not satisfy operator", &auth.Principal{Role: auth.RoleReadonly}, auth.RoleOperator, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.HasRole(tt.min); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrincipalContext(t *testing.T) {
	if got := auth.PrincipalFromContext(context.Background()); got != nil {
		t.Errorf("empty ctx: got %v, want nil", got)
	}

	want := &auth.Principal{ID: "u-1", Role: auth.RoleOperator}
	ctx := auth.WithPrincipal(context.Background(), want)
	got := auth.PrincipalFromContext(ctx)
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
