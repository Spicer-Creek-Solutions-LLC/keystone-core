// SPDX-License-Identifier: Apache-2.0

package acl

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// RoleACL implements [ACL] with per-(namespace, operation) minimum-
// role rules. v1.0 access policy is:
//
//	1. If RoleAdmin always passes (AdminBypass=true, the default),
//	   an admin principal is allowed regardless of rules.
//	2. Look up the (namespace, op) rule. If present, the principal
//	   must satisfy the minimum role.
//	3. If no rule matches, apply AllowByDefault:
//	     true   → allow (open-by-default; explicit deny rules form
//	              a denylist)
//	     false  → deny (closed-by-default; explicit allow rules
//	              form an allowlist).
//
// Closed-by-default (AllowByDefault=false) is the recommended
// posture; operators add rules to open specific namespaces.
type RoleACL struct {
	rules          map[ruleKey]auth.Role
	allowByDefault bool
	adminBypass    bool
}

type ruleKey struct {
	namespace string
	op        files.FileOperation
}

// RoleACLOption configures a [RoleACL] in [NewRoleACL].
type RoleACLOption func(*RoleACL)

// NewRoleACL constructs an empty RoleACL with the supplied
// options. Default behavior is closed-by-default + admin-bypass.
func NewRoleACL(opts ...RoleACLOption) *RoleACL {
	r := &RoleACL{
		rules:          make(map[ruleKey]auth.Role),
		allowByDefault: false,
		adminBypass:    true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithRule adds a (namespace, op) → minRole rule. A principal
// with a role satisfying minRole (per [auth.Principal.HasRole])
// is allowed; everyone else is denied for that pair.
func WithRule(namespace string, op files.FileOperation, minRole auth.Role) RoleACLOption {
	return func(r *RoleACL) {
		r.rules[ruleKey{namespace: namespace, op: op}] = minRole
	}
}

// WithDefaultAllow flips the default for unlisted (namespace, op)
// pairs from deny (the default) to allow.
func WithDefaultAllow() RoleACLOption {
	return func(r *RoleACL) {
		r.allowByDefault = true
	}
}

// WithNoAdminBypass disables the "admin always passes" shortcut
// so explicit rules apply to admins too. Defense-in-depth
// deployments enable this; the default (admin bypass on) is the
// operator-friendly choice.
func WithNoAdminBypass() RoleACLOption {
	return func(r *RoleACL) {
		r.adminBypass = false
	}
}

// Authorize implements [ACL.Authorize].
func (r *RoleACL) Authorize(_ context.Context, p *auth.Principal, op files.FileOperation, namespace string) error {
	if r.adminBypass && p != nil && p.Role == auth.RoleAdmin {
		return nil
	}
	if min, ok := r.rules[ruleKey{namespace: namespace, op: op}]; ok {
		if p.HasRole(min) {
			return nil
		}
		return fmt.Errorf("%w: %s on namespace %q requires role %s", ErrForbidden, op, namespace, min)
	}
	if r.allowByDefault {
		return nil
	}
	return fmt.Errorf("%w: %s on namespace %q has no allow rule", ErrForbidden, op, namespace)
}
