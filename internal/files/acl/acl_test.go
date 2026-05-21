package acl

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func principal(role auth.Role) *auth.Principal {
	return &auth.Principal{ID: "p", Role: role}
}

func TestRoleACL_DefaultDeny(t *testing.T) {
	a := NewRoleACL()
	cases := []struct {
		name string
		p    *auth.Principal
		op   files.FileOperation
		ns   string
		want error
	}{
		{"readonly get unlisted ns", principal(auth.RoleReadonly), files.FileOpGet, "configs", ErrForbidden},
		{"operator get unlisted ns", principal(auth.RoleOperator), files.FileOpGet, "configs", ErrForbidden},
		{"admin bypass on unlisted ns", principal(auth.RoleAdmin), files.FileOpGet, "configs", nil},
		{"nil principal denied", nil, files.FileOpGet, "configs", ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := a.Authorize(context.Background(), tc.p, tc.op, tc.ns)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestRoleACL_DefaultAllow(t *testing.T) {
	a := NewRoleACL(WithDefaultAllow())
	if err := a.Authorize(context.Background(), principal(auth.RoleReadonly), files.FileOpGet, "anywhere"); err != nil {
		t.Errorf("default-allow should permit unlisted ns, got %v", err)
	}
	if err := a.Authorize(context.Background(), nil, files.FileOpGet, "anywhere"); err != nil {
		t.Errorf("default-allow permits nil principal on unlisted ns, got %v", err)
	}
}

func TestRoleACL_PerRule(t *testing.T) {
	a := NewRoleACL(
		WithRule("configs", files.FileOpGet, auth.RoleReadonly),
		WithRule("configs", files.FileOpPut, auth.RoleOperator),
		WithRule("configs", files.FileOpDelete, auth.RoleAdmin),
	)
	ctx := context.Background()

	// readonly: only get allowed
	if err := a.Authorize(ctx, principal(auth.RoleReadonly), files.FileOpGet, "configs"); err != nil {
		t.Errorf("readonly+get configs: %v", err)
	}
	if err := a.Authorize(ctx, principal(auth.RoleReadonly), files.FileOpPut, "configs"); !errors.Is(err, ErrForbidden) {
		t.Errorf("readonly+put configs: want forbidden, got %v", err)
	}

	// operator: get + put, not delete
	if err := a.Authorize(ctx, principal(auth.RoleOperator), files.FileOpGet, "configs"); err != nil {
		t.Errorf("operator+get configs: %v", err)
	}
	if err := a.Authorize(ctx, principal(auth.RoleOperator), files.FileOpPut, "configs"); err != nil {
		t.Errorf("operator+put configs: %v", err)
	}
	if err := a.Authorize(ctx, principal(auth.RoleOperator), files.FileOpDelete, "configs"); !errors.Is(err, ErrForbidden) {
		t.Errorf("operator+delete configs: want forbidden, got %v", err)
	}

	// admin: bypass — everything
	for _, op := range []files.FileOperation{files.FileOpGet, files.FileOpPut, files.FileOpDelete, files.FileOpList} {
		if err := a.Authorize(ctx, principal(auth.RoleAdmin), op, "configs"); err != nil {
			t.Errorf("admin+%s configs: %v", op, err)
		}
	}

	// admin: bypass extends to unlisted namespaces
	if err := a.Authorize(ctx, principal(auth.RoleAdmin), files.FileOpGet, "secret"); err != nil {
		t.Errorf("admin+get unlisted: %v", err)
	}
}

func TestRoleACL_NoAdminBypass(t *testing.T) {
	a := NewRoleACL(
		WithNoAdminBypass(),
		WithRule("system", files.FileOpDelete, auth.Role(99)), // role no one satisfies
	)
	err := a.Authorize(context.Background(), principal(auth.RoleAdmin), files.FileOpDelete, "system")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want forbidden for admin without bypass, got %v", err)
	}
}

func TestRoleACL_NilPrincipal_RuleMatch(t *testing.T) {
	// A rule that requires RoleNone (i.e., anyone — including
	// unauthenticated callers) must accept a nil principal too.
	a := NewRoleACL(WithRule("public", files.FileOpGet, auth.RoleNone))
	if err := a.Authorize(context.Background(), nil, files.FileOpGet, "public"); err != nil {
		t.Errorf("nil principal + RoleNone rule should allow, got %v", err)
	}
}

func TestRoleACL_ErrorMessageMentionsRoleAndNamespace(t *testing.T) {
	a := NewRoleACL(WithRule("secret", files.FileOpGet, auth.RoleAdmin))
	err := a.Authorize(context.Background(), principal(auth.RoleOperator), files.FileOpGet, "secret")
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, frag := range []string{"forbidden", "secret", "admin", "get"} {
		if !contains(msg, frag) {
			t.Errorf("error %q missing fragment %q", msg, frag)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
