// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestSecretsConfig_DisabledSkipsValidation(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled config errored: %v", err)
	}
}

func TestSecretsConfig_RequiresAtLeastOneBackend(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{Enabled: true}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "at least one backend") {
		t.Errorf("err = %v, want at-least-one-backend", err)
	}
}

func TestSecretsBackendConfig_FileType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		b       SecretsBackendConfig
		wantSub string
	}{
		{
			name:    "missing name",
			b:       SecretsBackendConfig{Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "x", MasterKey: "inline:00"}},
			wantSub: "name is required",
		},
		{
			name:    "missing type",
			b:       SecretsBackendConfig{Name: "file", File: &SecretsFileBackendConfig{Path: "x", MasterKey: "y"}},
			wantSub: "type is required",
		},
		{
			name:    "bad type",
			b:       SecretsBackendConfig{Name: "x", Type: "azure"},
			wantSub: `type "azure" is not supported`,
		},
		{
			name:    "file type but no file block",
			b:       SecretsBackendConfig{Name: "x", Type: "encrypted_file"},
			wantSub: "requires `file` block",
		},
		{
			name: "file type with vault block too",
			b: SecretsBackendConfig{
				Name: "x", Type: "encrypted_file",
				File:  &SecretsFileBackendConfig{Path: "p", MasterKey: "k"},
				Vault: &SecretsVaultBackendConfig{Address: "https://v"},
			},
			wantSub: "must not also set `vault`",
		},
		{
			name:    "file path missing",
			b:       SecretsBackendConfig{Name: "x", Type: "encrypted_file", File: &SecretsFileBackendConfig{MasterKey: "k"}},
			wantSub: "file.path is required",
		},
		{
			name:    "file master_key missing",
			b:       SecretsBackendConfig{Name: "x", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p"}},
			wantSub: "file.master_key is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.b.Validate(0)
			if err == nil {
				t.Fatalf("Validate = nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestSecretsBackendConfig_VaultType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		b       SecretsBackendConfig
		wantSub string
	}{
		{
			name:    "vault type but no vault block",
			b:       SecretsBackendConfig{Name: "v", Type: "vault"},
			wantSub: "requires `vault` block",
		},
		{
			name: "vault address missing",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{AuthMethod: "token", Token: "t"}},
			wantSub: "vault.address is required",
		},
		{
			name: "vault address bad scheme",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "vault.internal", AuthMethod: "token", Token: "t"}},
			wantSub: "must start with http:// or https://",
		},
		{
			name: "vault auth_method missing",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault"}},
			wantSub: "auth_method is required",
		},
		{
			name: "vault token auth missing token",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "token"}},
			wantSub: "vault.token is required",
		},
		{
			name: "vault approle missing role_id",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "approle", AppRole: &SecretsVaultAppRoleConfig{SecretID: "s"}}},
			wantSub: "approle.role_id is required",
		},
		{
			name: "vault approle missing secret_id",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "approle", AppRole: &SecretsVaultAppRoleConfig{RoleID: "r"}}},
			wantSub: "approle.secret_id is required",
		},
		{
			name: "vault kubernetes missing role",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "kubernetes", Kubernetes: &SecretsVaultK8sConfig{}}},
			wantSub: "kubernetes.role is required",
		},
		{
			name: "vault ldap missing username",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "ldap", LDAP: &SecretsVaultLDAPConfig{Password: "p"}}},
			wantSub: "ldap.username is required",
		},
		{
			name: "vault ldap missing password",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "ldap", LDAP: &SecretsVaultLDAPConfig{Username: "u"}}},
			wantSub: "ldap.password is required",
		},
		{
			name: "vault unsupported auth method",
			b: SecretsBackendConfig{Name: "v", Type: "vault",
				Vault: &SecretsVaultBackendConfig{Address: "https://vault", AuthMethod: "aws"}},
			wantSub: "not supported",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.b.Validate(0)
			if err == nil {
				t.Fatalf("Validate = nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestSecretsConfig_DuplicateBackendNamesRejected(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{
		Enabled: true,
		Backends: []SecretsBackendConfig{
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p2", MasterKey: "k2"}},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Errorf("err = %v, want duplicate-name", err)
	}
}

func TestSecretsConfig_DefaultBackendUnknown(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{
		Enabled:        true,
		DefaultBackend: "ghost",
		Backends: []SecretsBackendConfig{
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `default_backend "ghost"`) {
		t.Errorf("err = %v", err)
	}
}

func TestSecretsConfig_RouteToUnknownBackend(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{
		Enabled: true,
		Backends: []SecretsBackendConfig{
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
		},
		Routing: []SecretsRouteConfig{
			{Prefix: "kv/", Backend: "ghost"},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `backend "ghost"`) {
		t.Errorf("err = %v", err)
	}
}

func TestSecretsConfig_RouteEmptyPrefix(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{
		Enabled: true,
		Backends: []SecretsBackendConfig{
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
		},
		Routing: []SecretsRouteConfig{{Prefix: "", Backend: "file"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "prefix is required") {
		t.Errorf("err = %v", err)
	}
}

func TestSecretsConfig_LeaseValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		lease   SecretsLeaseConfig
		wantSub string
	}{
		{"jitter too high", SecretsLeaseConfig{Jitter: 0.8}, "jitter"},
		{"jitter negative", SecretsLeaseConfig{Jitter: -0.1}, "jitter"},
		{"unknown strategy", SecretsLeaseConfig{DefaultStrategy: "burnout"}, "default_strategy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := SecretsConfig{
				Enabled: true,
				Backends: []SecretsBackendConfig{
					{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
				},
				Lease: tc.lease,
			}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate = nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestSecretsConfig_AuditValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		audit   SecretsAuditConfig
		wantSub string
	}{
		{"negative buffer", SecretsAuditConfig{BufferSize: -1}, "buffer_size"},
		{"sampling negative", SecretsAuditConfig{SamplingFraction: -0.1}, "sampling_fraction"},
		{"sampling too high", SecretsAuditConfig{SamplingFraction: 1.5}, "sampling_fraction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := SecretsConfig{
				Enabled: true,
				Backends: []SecretsBackendConfig{
					{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "p", MasterKey: "k"}},
				},
				Audit: tc.audit,
			}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate = nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestSecretsConfig_HappyPath(t *testing.T) {
	t.Parallel()
	cfg := SecretsConfig{
		Enabled:        true,
		DefaultBackend: "file",
		Backends: []SecretsBackendConfig{
			{Name: "file", Type: "encrypted_file", File: &SecretsFileBackendConfig{Path: "/var/lib/keystone/secrets.bin", MasterKey: "env:KSCORE_SECRETS_MASTER"}},
			{Name: "vault", Type: "vault", Vault: &SecretsVaultBackendConfig{
				Address:    "https://vault.internal",
				AuthMethod: "approle",
				AppRole:    &SecretsVaultAppRoleConfig{RoleID: "r", SecretID: "s"},
			}},
		},
		Routing: []SecretsRouteConfig{
			{Prefix: "secret/", Backend: "vault"},
			{Prefix: "kv/", Backend: "file"},
		},
		Cache: SecretsCacheConfig{Enabled: true, MaxEntries: 10000, DefaultTTL: 5 * time.Minute},
		Lease: SecretsLeaseConfig{PollInterval: 30 * time.Second, Jitter: 0.1, DefaultStrategy: "lazy"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("happy-path Validate err: %v", err)
	}
}
