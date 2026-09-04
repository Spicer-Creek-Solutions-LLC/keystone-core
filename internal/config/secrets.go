// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"
	"time"
)

// SecretsConfig drives the Epic 10 secrets boot in kscore-server. The
// shape mirrors PROJECT-DETAILS §4.11's YAML example exactly:
//
//	secrets:
//	  enabled: true
//	  default_backend: file
//	  backends:
//	    - name: file
//	      type: encrypted_file
//	      file: { path: /var/lib/keystone/secrets.bin, master_key: env:KSCORE_SECRETS_MASTER }
//	    - name: vault
//	      type: vault
//	      vault: { address: https://vault.internal, auth_method: approle, ... }
//	  routing:
//	    - prefix: secret/
//	      backend: vault
//	    - prefix: kv/
//	      backend: file
//	  cache: { enabled: true, max_entries: 10000, default_ttl: 5m }
//	  lease: { poll_interval: 30s, jitter: 0.1, default_strategy: lazy }
//
// Enabled defaults to false — operators opt in. A stock kscore-server
// without `secrets.enabled: true` runs the rest of the stack
// unchanged; the gRPC SecretsService returns codes.Unavailable +
// REST returns 503 when called.
type SecretsConfig struct {
	Enabled        bool                   `koanf:"enabled"`
	DefaultBackend string                 `koanf:"default_backend"`
	Backends       []SecretsBackendConfig `koanf:"backends"`
	Routing        []SecretsRouteConfig   `koanf:"routing"`
	Cache          SecretsCacheConfig     `koanf:"cache"`
	Lease          SecretsLeaseConfig     `koanf:"lease"`
	Audit          SecretsAuditConfig     `koanf:"audit"`

	// AgentGrants control which agents may read which secret paths
	// when rendering a state file on the agent host. Empty denies
	// every agent lookup, which is the correct default: an operator
	// opts specific paths in, rather than opting the store out.
	//
	// Grants live here rather than on the agent or in a field the
	// agent supplies, because an agent that declares its own
	// entitlements is granting itself.
	AgentGrants []SecretsAgentGrantConfig `koanf:"agent_grants"`
}

// SecretsAgentGrantConfig allows a set of agents to read a set of
// secret paths.
//
// AgentIDs and Labels are OR'd: an agent matches if its id is listed
// OR it carries every label in Labels. Paths ending in "/" allow a
// subtree; anything else must match exactly, so `app` does not
// silently grant `application/...`.
type SecretsAgentGrantConfig struct {
	AgentIDs []string          `koanf:"agent_ids"`
	Labels   map[string]string `koanf:"labels"`
	Paths    []string          `koanf:"paths"`
}

// SecretsAuditConfig drives the audit-emission infrastructure
// (Epic 10 task 11). Wires `LogAuditor` (always) + `BufferedAuditor`
// (when BufferSize > 0) + an optional `SamplingAuditor` wrapping the
// log auditor when SamplingFraction < 1.0. Epic 12's `AuditStore`
// will plug in here once it ships.
type SecretsAuditConfig struct {
	// BufferSize is the in-memory ring-buffer capacity (the
	// PROJECT-DETAILS §4.12 "Auditor circular buffer"). 0 disables
	// the buffer; positive enables. Default 10000.
	BufferSize int `koanf:"buffer_size"`

	// SamplingFraction is the probability each successful event
	// reaches the log auditor (in `[0, 1]`). 1.0 = no sampling
	// (default). Failures + cap-refusals always emit regardless.
	SamplingFraction float64 `koanf:"sampling_fraction"`
}

// SecretsBackendConfig is one operator-declared backend instance.
// Exactly one of [File] / [Vault] must be set and must match [Type].
type SecretsBackendConfig struct {
	Name  string                     `koanf:"name"`
	Type  string                     `koanf:"type"` // "encrypted_file" | "vault"
	File  *SecretsFileBackendConfig  `koanf:"file"`
	Vault *SecretsVaultBackendConfig `koanf:"vault"`
}

// SecretsBackendType enumerates the v1.0 backend types.
const (
	SecretsBackendTypeFile  = "encrypted_file"
	SecretsBackendTypeVault = "vault"
)

// SecretsFileBackendConfig drives the encrypted-file backend
// (internal/secrets/file). MasterKey is the scheme-prefixed value
// resolved by `masterkey.Resolve` — `env:VAR_NAME`,
// `file:/path/to/keyfile`, or `inline:<hex|base64>`.
type SecretsFileBackendConfig struct {
	Path      string `koanf:"path"`
	MasterKey string `koanf:"master_key"`
}

// SecretsVaultBackendConfig drives the Vault backend
// (internal/secrets/vault).
type SecretsVaultBackendConfig struct {
	Address    string                     `koanf:"address"`
	Namespace  string                     `koanf:"namespace"`
	AuthMethod string                     `koanf:"auth_method"`
	Token      string                     `koanf:"token"` // for auth_method = token
	AppRole    *SecretsVaultAppRoleConfig `koanf:"approle"`
	Kubernetes *SecretsVaultK8sConfig     `koanf:"kubernetes"`
	LDAP       *SecretsVaultLDAPConfig    `koanf:"ldap"`
	TLS        SecretsVaultTLSConfig      `koanf:"tls"`
	Timeout    time.Duration              `koanf:"timeout"`
	Mounts     []SecretsVaultMountConfig  `koanf:"mounts"`
}

// SecretsVaultAppRoleConfig — RoleID + SecretID. SecretID may itself
// be a wrapping token (per Vault's response-wrapping flow).
type SecretsVaultAppRoleConfig struct {
	RoleID                  string `koanf:"role_id"`
	SecretID                string `koanf:"secret_id"`
	SecretIDIsWrappingToken bool   `koanf:"secret_id_is_wrapping_token"`
	Mount                   string `koanf:"mount"`
}

// SecretsVaultK8sConfig — Kubernetes ServiceAccount-based auth.
type SecretsVaultK8sConfig struct {
	Role      string `koanf:"role"`
	TokenPath string `koanf:"token_path"`
	Mount     string `koanf:"mount"`
}

// SecretsVaultLDAPConfig — LDAP username/password auth.
type SecretsVaultLDAPConfig struct {
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	Mount    string `koanf:"mount"`
}

// SecretsVaultTLSConfig configures HTTPS to Vault.
type SecretsVaultTLSConfig struct {
	CACert     string `koanf:"ca_cert"`
	ClientCert string `koanf:"client_cert"`
	ClientKey  string `koanf:"client_key"`
	ServerName string `koanf:"server_name"`
	Insecure   bool   `koanf:"insecure"`
}

// SecretsVaultMountConfig declares the KV engine version for one
// Vault mount path — see internal/secrets/vault.MountConfig.
type SecretsVaultMountConfig struct {
	Path      string `koanf:"path"`
	KVVersion int    `koanf:"kv_version"`
}

// SecretsRouteConfig is one row in the broker's routing table.
type SecretsRouteConfig struct {
	Prefix  string `koanf:"prefix"`
	Backend string `koanf:"backend"`
}

// SecretsCacheConfig drives the [secrets.SecretCache]. Enabled
// defaults to true when secrets are enabled — operators rarely want
// to disable caching, but explicit `enabled: false` falls back to
// the no-op cache.
type SecretsCacheConfig struct {
	Enabled       bool          `koanf:"enabled"`
	MaxEntries    int           `koanf:"max_entries"`
	DefaultTTL    time.Duration `koanf:"default_ttl"`
	SweepInterval time.Duration `koanf:"sweep_interval"`
}

// SecretsLeaseConfig drives the [secrets.LeaseManager].
type SecretsLeaseConfig struct {
	PollInterval    time.Duration `koanf:"poll_interval"`
	Jitter          float64       `koanf:"jitter"`
	RenewTimeout    time.Duration `koanf:"renew_timeout"`
	DefaultStrategy string        `koanf:"default_strategy"` // "eager" | "lazy" | "on_demand"
}

// Validate checks the secrets-config shape — surface obviously-broken
// configs early instead of waiting for `NewBackend` failures at boot.
// Skips when [SecretsConfig.Enabled] is false.
func (c *SecretsConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Backends) == 0 {
		return fmt.Errorf("secrets: at least one backend is required when enabled")
	}
	names := make(map[string]struct{}, len(c.Backends))
	for i, b := range c.Backends {
		if err := b.Validate(i); err != nil {
			return err
		}
		if _, dup := names[b.Name]; dup {
			return fmt.Errorf("secrets: backends[%d]: duplicate name %q", i, b.Name)
		}
		names[b.Name] = struct{}{}
	}
	if c.DefaultBackend != "" {
		if _, ok := names[c.DefaultBackend]; !ok {
			return fmt.Errorf("secrets: default_backend %q does not name a registered backend", c.DefaultBackend)
		}
	}
	for i, r := range c.Routing {
		if r.Prefix == "" {
			return fmt.Errorf("secrets: routing[%d]: prefix is required", i)
		}
		if _, ok := names[r.Backend]; !ok {
			return fmt.Errorf("secrets: routing[%d]: backend %q does not name a registered backend", i, r.Backend)
		}
	}
	if c.Lease.Jitter < 0 || c.Lease.Jitter > 0.5 {
		return fmt.Errorf("secrets: lease.jitter = %v, must be in [0, 0.5]", c.Lease.Jitter)
	}
	if c.Audit.BufferSize < 0 {
		return fmt.Errorf("secrets: audit.buffer_size = %d, must be >= 0", c.Audit.BufferSize)
	}
	if c.Audit.SamplingFraction != 0 && (c.Audit.SamplingFraction < 0 || c.Audit.SamplingFraction > 1) {
		return fmt.Errorf("secrets: audit.sampling_fraction = %v, must be in [0, 1]", c.Audit.SamplingFraction)
	}
	if c.Lease.DefaultStrategy != "" {
		switch c.Lease.DefaultStrategy {
		case "eager", "lazy", "on_demand":
		default:
			return fmt.Errorf("secrets: lease.default_strategy %q is not one of eager/lazy/on_demand", c.Lease.DefaultStrategy)
		}
	}
	return nil
}

// Validate checks one backend entry. idx is the slice index for
// error context.
func (b SecretsBackendConfig) Validate(idx int) error {
	if b.Name == "" {
		return fmt.Errorf("secrets: backends[%d]: name is required", idx)
	}
	switch b.Type {
	case SecretsBackendTypeFile:
		if b.File == nil {
			return fmt.Errorf("secrets: backends[%d]: type=encrypted_file requires `file` block", idx)
		}
		if b.Vault != nil {
			return fmt.Errorf("secrets: backends[%d]: type=encrypted_file must not also set `vault`", idx)
		}
		if b.File.Path == "" {
			return fmt.Errorf("secrets: backends[%d].file.path is required", idx)
		}
		if b.File.MasterKey == "" {
			return fmt.Errorf("secrets: backends[%d].file.master_key is required", idx)
		}
	case SecretsBackendTypeVault:
		if b.Vault == nil {
			return fmt.Errorf("secrets: backends[%d]: type=vault requires `vault` block", idx)
		}
		if b.File != nil {
			return fmt.Errorf("secrets: backends[%d]: type=vault must not also set `file`", idx)
		}
		if b.Vault.Address == "" {
			return fmt.Errorf("secrets: backends[%d].vault.address is required", idx)
		}
		if !strings.HasPrefix(b.Vault.Address, "http://") && !strings.HasPrefix(b.Vault.Address, "https://") {
			return fmt.Errorf("secrets: backends[%d].vault.address %q must start with http:// or https://", idx, b.Vault.Address)
		}
		if err := validateVaultAuth(idx, b.Vault); err != nil {
			return err
		}
	case "":
		return fmt.Errorf("secrets: backends[%d]: type is required (one of encrypted_file, vault)", idx)
	default:
		return fmt.Errorf("secrets: backends[%d]: type %q is not supported (v1.0: encrypted_file, vault)", idx, b.Type)
	}
	return nil
}

func validateVaultAuth(idx int, v *SecretsVaultBackendConfig) error {
	switch v.AuthMethod {
	case "token":
		if v.Token == "" {
			return fmt.Errorf("secrets: backends[%d].vault.token is required for auth_method=token", idx)
		}
	case "approle":
		if v.AppRole == nil {
			return fmt.Errorf("secrets: backends[%d].vault.approle is required for auth_method=approle", idx)
		}
		if v.AppRole.RoleID == "" {
			return fmt.Errorf("secrets: backends[%d].vault.approle.role_id is required", idx)
		}
		if v.AppRole.SecretID == "" {
			return fmt.Errorf("secrets: backends[%d].vault.approle.secret_id is required", idx)
		}
	case "kubernetes":
		if v.Kubernetes == nil {
			return fmt.Errorf("secrets: backends[%d].vault.kubernetes is required for auth_method=kubernetes", idx)
		}
		if v.Kubernetes.Role == "" {
			return fmt.Errorf("secrets: backends[%d].vault.kubernetes.role is required", idx)
		}
	case "ldap":
		if v.LDAP == nil {
			return fmt.Errorf("secrets: backends[%d].vault.ldap is required for auth_method=ldap", idx)
		}
		if v.LDAP.Username == "" {
			return fmt.Errorf("secrets: backends[%d].vault.ldap.username is required", idx)
		}
		if v.LDAP.Password == "" {
			return fmt.Errorf("secrets: backends[%d].vault.ldap.password is required", idx)
		}
	case "":
		return fmt.Errorf("secrets: backends[%d].vault.auth_method is required (one of token, approle, kubernetes, ldap)", idx)
	default:
		return fmt.Errorf("secrets: backends[%d].vault.auth_method %q not supported (v1.0: token, approle, kubernetes, ldap; AWS IAM is gate-v1.0 ROADMAP)", idx, v.AuthMethod)
	}
	return nil
}
