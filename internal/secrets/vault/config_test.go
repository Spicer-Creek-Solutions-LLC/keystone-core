// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	baseAuth := AuthConfig{
		Method: AuthMethodToken,
		Token:  &TokenAuthConfig{Token: "s.dev"},
	}

	cases := []struct {
		name    string
		cfg     Config
		wantSub string
	}{
		{
			name:    "address required",
			cfg:     Config{Auth: baseAuth},
			wantSub: "Config.Address is required",
		},
		{
			name:    "address bad scheme",
			cfg:     Config{Address: "vault.internal", Auth: baseAuth},
			wantSub: "must start with http:// or https://",
		},
		{
			name:    "auth method required",
			cfg:     Config{Address: "https://vault.internal"},
			wantSub: "Auth.Method is required",
		},
		{
			name: "unknown auth method",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: "azure-ad"},
			},
			wantSub: "not supported",
		},
		{
			name: "approle missing role id",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: AuthMethodAppRole, AppRole: &AppRoleAuthConfig{SecretID: "s"}},
			},
			wantSub: "RoleID is required",
		},
		{
			name: "approle missing secret id",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: AuthMethodAppRole, AppRole: &AppRoleAuthConfig{RoleID: "r"}},
			},
			wantSub: "SecretID is required",
		},
		{
			name: "token method with extra config rejected",
			cfg: Config{
				Address: "https://vault.internal",
				Auth: AuthConfig{
					Method:  AuthMethodToken,
					Token:   &TokenAuthConfig{Token: "s.x"},
					AppRole: &AppRoleAuthConfig{RoleID: "r", SecretID: "s"},
				},
			},
			wantSub: "must not also set AppRole",
		},
		{
			name: "kubernetes missing role",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: AuthMethodKubernetes, Kubernetes: &KubernetesAuthConfig{}},
			},
			wantSub: "Auth.Kubernetes.Role is required",
		},
		{
			name: "ldap missing username",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: AuthMethodLDAP, LDAP: &LDAPAuthConfig{Password: "p"}},
			},
			wantSub: "Auth.LDAP.Username is required",
		},
		{
			name: "ldap missing password",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    AuthConfig{Method: AuthMethodLDAP, LDAP: &LDAPAuthConfig{Username: "u"}},
			},
			wantSub: "Auth.LDAP.Password is required",
		},
		{
			name: "renewal fraction out of range",
			cfg: Config{
				Address:                   "https://vault.internal",
				Auth:                      baseAuth,
				TokenRenewalEarlyFraction: 1.5,
			},
			wantSub: "must be in [0, 1]",
		},
		{
			name: "mounts duplicate path",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    baseAuth,
				Mounts: []MountConfig{
					{Path: "secret", KVVersion: 2},
					{Path: "secret/", KVVersion: 2},
				},
			},
			wantSub: "duplicate",
		},
		{
			name: "mounts bad version",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    baseAuth,
				Mounts:  []MountConfig{{Path: "secret", KVVersion: 3}},
			},
			wantSub: "must be 1 or 2",
		},
		{
			name: "mounts empty path",
			cfg: Config{
				Address: "https://vault.internal",
				Auth:    baseAuth,
				Mounts:  []MountConfig{{Path: ""}},
			},
			wantSub: "Path is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.cfg.validate()
			if err == nil {
				t.Fatalf("validate() = nil err, want %q", tc.wantSub)
			}
			if !errors.Is(err, secrets.ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Address: "https://vault.internal",
		Auth: AuthConfig{
			Method: AuthMethodToken,
			Token:  &TokenAuthConfig{Token: "s.x"},
		},
	}
	out, err := cfg.validate()
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if out.Name != DefaultBackendName {
		t.Errorf("Name default = %q, want %q", out.Name, DefaultBackendName)
	}
	if out.Timeout != DefaultTimeout {
		t.Errorf("Timeout default = %v, want %v", out.Timeout, DefaultTimeout)
	}
	if out.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries default = %v, want %v", out.MaxRetries, DefaultMaxRetries)
	}
	if out.TokenRenewalEarlyFraction != DefaultTokenRenewalEarlyFraction {
		t.Errorf("TokenRenewalEarlyFraction default = %v, want %v", out.TokenRenewalEarlyFraction, DefaultTokenRenewalEarlyFraction)
	}
	if out.Logger == nil {
		t.Errorf("Logger default is nil")
	}
	if out.Clock == nil {
		t.Errorf("Clock default is nil")
	}
}

func TestConfig_ResolveKVVersion(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Mounts: []MountConfig{
			{Path: "secret", KVVersion: 2},
			{Path: "kv-legacy", KVVersion: 1},
			{Path: "default-mount"}, // KVVersion 0 → v2 default
		},
	}
	cases := []struct {
		path    string
		wantVer int
	}{
		{"secret/app/db", 2},
		{"secret", 2},
		{"kv-legacy/foo", 1},
		{"kv-legacy", 1},
		{"default-mount/x", 2},
		{"unlisted/whatever", 2},
		{"secretstore/foo", 2}, // segment-aware: "secret" mount does NOT match "secretstore/..."
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := cfg.resolveKVVersion(tc.path); got != tc.wantVer {
				t.Errorf("resolveKVVersion(%q) = %d, want %d", tc.path, got, tc.wantVer)
			}
		})
	}
}

func TestConfig_DurationDefaultsArePositive(t *testing.T) {
	t.Parallel()
	if DefaultTimeout <= 0 {
		t.Errorf("DefaultTimeout must be positive")
	}
	if DefaultMaxRetries < 0 {
		t.Errorf("DefaultMaxRetries must be non-negative")
	}
	if time.Duration(DefaultMaxRetries) >= time.Hour {
		t.Errorf("MaxRetries is a count, not a duration; got suspicious value %d", DefaultMaxRetries)
	}
}
