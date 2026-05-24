// SPDX-License-Identifier: Apache-2.0

// Package auth provides authentication and authorization primitives
// for Keystone Core APIs (gRPC + REST).
//
// v1.0 supports three authentication methods:
//
//   - API key (Authorization: Bearer <key>)
//   - JWT     (Authorization: Bearer <jwt>)
//   - mTLS    (peer certificate from the TLS handshake)
//
// Each is an Authenticator. A Chain composes them in order.
// RBACAuthorizer maps gRPC methods to required roles and gates access.
// See PROJECT-DETAILS §4.10.
package auth

import (
	"context"
	"fmt"
	"time"
)

// Role is an ordered access level. Higher values include lower
// permissions: an admin satisfies operator and readonly checks.
type Role int

const (
	// RoleNone is the zero value; treat unauthenticated callers as
	// RoleNone.
	RoleNone Role = iota
	RoleReadonly
	RoleOperator
	RoleAdmin
)

// String returns the canonical role name ("admin" | "operator" |
// "readonly" | "none").
func (r Role) String() string {
	switch r {
	case RoleAdmin:
		return "admin"
	case RoleOperator:
		return "operator"
	case RoleReadonly:
		return "readonly"
	default:
		return "none"
	}
}

// ParseRole accepts the canonical role names. Empty string maps to
// RoleNone — caller decides whether to treat that as a readonly
// fallback (PROJECT-DETAILS §4.10 JWT gotcha) or as an unauthenticated
// state.
func ParseRole(s string) (Role, error) {
	switch s {
	case "admin":
		return RoleAdmin, nil
	case "operator":
		return RoleOperator, nil
	case "readonly":
		return RoleReadonly, nil
	case "":
		return RoleNone, nil
	default:
		return RoleNone, fmt.Errorf("auth: unknown role %q", s)
	}
}

// AuthMethod identifies the auth pathway that produced a Principal.
type AuthMethod int

const (
	AuthMethodNone AuthMethod = iota
	AuthMethodAPIKey
	AuthMethodJWT
	AuthMethodMTLS
)

// String returns "api-key" | "jwt" | "mtls" | "none".
func (m AuthMethod) String() string {
	switch m {
	case AuthMethodAPIKey:
		return "api-key"
	case AuthMethodJWT:
		return "jwt"
	case AuthMethodMTLS:
		return "mtls"
	default:
		return "none"
	}
}

// Principal is the authenticated identity attached to an inbound RPC.
// Set on the request context by an Authenticator and inspected by the
// Authorizer (and any handler that needs caller identity).
type Principal struct {
	ID              string
	Name            string
	Role            Role
	AuthMethod      AuthMethod
	Metadata        map[string]string
	AuthenticatedAt time.Time
}

// HasRole returns true if p satisfies the minimum-required role per
// the admin > operator > readonly hierarchy. A nil receiver returns
// true only when min == RoleNone.
func (p *Principal) HasRole(min Role) bool {
	if p == nil {
		return min == RoleNone
	}
	return int(p.Role) >= int(min)
}

// principalCtxKey is the context-key type for principal storage.
type principalCtxKey struct{}

// WithPrincipal attaches p to ctx for downstream handlers.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFromContext returns the principal attached by an
// Authenticator earlier in the chain. Returns nil if none.
func PrincipalFromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalCtxKey{}).(*Principal)
	return p
}
