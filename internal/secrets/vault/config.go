// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// DefaultBackendName is the [Backend.Name] when [Config.Name] is empty.
const DefaultBackendName = "vault"

// DefaultTimeout is the per-HTTP-call timeout applied to every
// Vault interaction. Operators override via [Config.Timeout].
const DefaultTimeout = 30 * time.Second

// DefaultMaxRetries is the SDK retry count for transient errors
// (network glitches, Vault 503s). Operators override via
// [Config.MaxRetries].
const DefaultMaxRetries = 3

// DefaultTokenRenewalEarlyFraction is the fraction of the auth
// token's TTL at which the renewer attempts a renewal. 0.5 means
// "renew when half the TTL remains" — matches the Epic 09 SVID /
// CA rotation cadence so operators see consistent behavior across
// identity + secrets.
const DefaultTokenRenewalEarlyFraction = 0.5

// AuthMethod enumerates the v1.0 Vault auth methods.
const (
	AuthMethodToken      = "token"
	AuthMethodAppRole    = "approle"
	AuthMethodKubernetes = "kubernetes"
	AuthMethodLDAP       = "ldap"
)

// Config drives [NewBackend]. Address + Auth are required; every
// other field has a sensible default.
type Config struct {
	// Address is the Vault server URL, e.g. `https://vault.internal:8200`.
	// Required.
	Address string

	// Namespace is the Vault Enterprise namespace header. Empty for
	// non-Enterprise / root namespace.
	Namespace string

	// Name overrides the backend's `Name()` for multi-Vault
	// deployments. Defaults to [DefaultBackendName].
	Name string

	// TLS configures the HTTP client's TLS posture. Empty = system defaults.
	TLS TLSConfig

	// Auth specifies which method to use to acquire a Vault token.
	// Required.
	Auth AuthConfig

	// Mounts maps Vault mount paths to KV engine versions. Unlisted
	// mounts default to KV v2. v1.0 trial deployments most commonly
	// run a single `secret/` mount at v2.
	Mounts []MountConfig

	// Timeout is the per-HTTP-call timeout. 0 → [DefaultTimeout].
	Timeout time.Duration

	// MaxRetries is the SDK retry count for transient errors.
	// 0 → [DefaultMaxRetries]. Negative disables retries entirely.
	MaxRetries int

	// TokenRenewalEarlyFraction is the fraction of TTL at which the
	// renewer fires. 0 → [DefaultTokenRenewalEarlyFraction].
	TokenRenewalEarlyFraction float64

	// Logger drives lifecycle / renewer log lines (NOT the audit
	// trail — that goes through the broker's [secrets.Auditor]).
	// nil → `slog.Default`.
	Logger *slog.Logger

	// Clock injects testable now-time for the renewer. nil → time.Now().UTC().
	Clock func() time.Time
}

// TLSConfig configures the HTTPS posture for the Vault client.
//
// CACert / CACertBytes mirror vault/api.TLSConfig — provide one or
// the other, not both. ServerName overrides the SNI / cert-verify
// hostname (useful when Address uses a load-balancer IP). Insecure
// disables TLS verification; logged at WARN at boot because trial
// deployments sometimes need it but production should not.
type TLSConfig struct {
	CACert        string // file path
	CACertBytes   []byte
	ClientCert    string // file path
	ClientKey     string // file path
	ServerName    string
	Insecure      bool
}

// AuthConfig selects an auth method + its config. Exactly one of
// the per-method pointers must be set, and its method MUST match the
// [AuthConfig.Method] discriminator. v1.0 supports four methods;
// AWS IAM is deferred (see the gate-v1.0 ROADMAP entry).
type AuthConfig struct {
	Method     string
	Token      *TokenAuthConfig
	AppRole    *AppRoleAuthConfig
	Kubernetes *KubernetesAuthConfig
	LDAP       *LDAPAuthConfig
}

// TokenAuthConfig sets a pre-issued Vault token directly on the
// client. Simplest auth method; ideal for dev, demo, and CI. The
// token can be sourced from env via the operator config layer
// (task 9) — at this package's boundary it's already resolved to a
// string.
type TokenAuthConfig struct {
	Token string // required; raw Vault token
}

// AppRoleAuthConfig logs in to Vault via the AppRole method —
// production-grade machine auth.
//
// SecretIDWrappingTTL > 0 indicates the supplied SecretID is itself
// a response-wrapping token that the client must unwrap before
// using; the Vault SDK does that automatically. Mount defaults to
// `approle` when empty.
type AppRoleAuthConfig struct {
	RoleID                 string
	SecretID               string
	SecretIDIsWrappingToken bool
	Mount                  string
}

// KubernetesAuthConfig logs in via the Kubernetes auth method,
// trading a ServiceAccount JWT for a Vault token. TokenPath defaults
// to the in-cluster ServiceAccount projection
// `/var/run/secrets/kubernetes.io/serviceaccount/token` when empty.
type KubernetesAuthConfig struct {
	Role      string // required
	TokenPath string // optional; empty = in-cluster default
	Mount     string // optional; defaults to "kubernetes"
}

// LDAPAuthConfig logs in via the LDAP auth method. Password is
// supplied at this layer as a plain string; the operator config
// layer (task 9) sources it from env / file the same way the master
// key source works.
type LDAPAuthConfig struct {
	Username string
	Password string
	Mount    string // optional; defaults to "ldap"
}

// MountConfig declares the KV engine version for one Vault mount
// path. Operator config might list:
//
//	- {Path: "secret",     KVVersion: 2}
//	- {Path: "kv-legacy",  KVVersion: 1}
//
// Unlisted mounts default to v2 (the modern Vault default).
type MountConfig struct {
	Path      string
	KVVersion int
}

// withDefaults returns a copy of cfg with zero-value fields filled
// in. Called from [Config.validate] so all downstream code reads the
// defaulted values.
func (c Config) withDefaults() Config {
	out := c
	if out.Name == "" {
		out.Name = DefaultBackendName
	}
	if out.Timeout == 0 {
		out.Timeout = DefaultTimeout
	}
	if out.MaxRetries == 0 {
		out.MaxRetries = DefaultMaxRetries
	}
	if out.TokenRenewalEarlyFraction == 0 {
		out.TokenRenewalEarlyFraction = DefaultTokenRenewalEarlyFraction
	}
	if out.Logger == nil {
		out.Logger = slog.Default()
	}
	if out.Clock == nil {
		out.Clock = func() time.Time { return time.Now().UTC() }
	}
	return out
}

// validate checks the config for shape errors. All returned errors
// wrap [secrets.ErrInvalidBackend] (callers translate when bubbling
// up). Returns the defaulted Config on success.
func (c Config) validate() (Config, error) {
	if c.Address == "" {
		return Config{}, errInvalid("Config.Address is required")
	}
	if !strings.HasPrefix(c.Address, "http://") && !strings.HasPrefix(c.Address, "https://") {
		return Config{}, errInvalid(fmt.Sprintf("Config.Address %q must start with http:// or https://", c.Address))
	}

	if c.TokenRenewalEarlyFraction < 0 || c.TokenRenewalEarlyFraction > 1 {
		return Config{}, errInvalid(fmt.Sprintf("Config.TokenRenewalEarlyFraction = %v, must be in [0, 1]", c.TokenRenewalEarlyFraction))
	}

	if err := c.Auth.validate(); err != nil {
		return Config{}, err
	}

	seenMount := make(map[string]struct{}, len(c.Mounts))
	for i, m := range c.Mounts {
		if m.Path == "" {
			return Config{}, errInvalid(fmt.Sprintf("Config.Mounts[%d].Path is required", i))
		}
		if m.KVVersion != 0 && m.KVVersion != 1 && m.KVVersion != 2 {
			return Config{}, errInvalid(fmt.Sprintf("Config.Mounts[%d].KVVersion = %d, must be 1 or 2 (or 0 for default)", i, m.KVVersion))
		}
		key := strings.TrimSuffix(m.Path, "/")
		if _, dup := seenMount[key]; dup {
			return Config{}, errInvalid(fmt.Sprintf("Config.Mounts[%d].Path %q is a duplicate", i, m.Path))
		}
		seenMount[key] = struct{}{}
	}

	return c.withDefaults(), nil
}

func (a AuthConfig) validate() error {
	switch a.Method {
	case AuthMethodToken:
		if a.Token == nil {
			return errInvalid("Auth.Method=token requires Auth.Token")
		}
		if a.Token.Token == "" {
			return errInvalid("Auth.Token.Token is required for the token auth method")
		}
		if a.AppRole != nil || a.Kubernetes != nil || a.LDAP != nil {
			return errInvalid("Auth.Method=token must not also set AppRole / Kubernetes / LDAP")
		}
	case AuthMethodAppRole:
		if a.AppRole == nil {
			return errInvalid("Auth.Method=approle requires Auth.AppRole")
		}
		if a.AppRole.RoleID == "" {
			return errInvalid("Auth.AppRole.RoleID is required")
		}
		if a.AppRole.SecretID == "" {
			return errInvalid("Auth.AppRole.SecretID is required")
		}
		if a.Token != nil || a.Kubernetes != nil || a.LDAP != nil {
			return errInvalid("Auth.Method=approle must not also set Token / Kubernetes / LDAP")
		}
	case AuthMethodKubernetes:
		if a.Kubernetes == nil {
			return errInvalid("Auth.Method=kubernetes requires Auth.Kubernetes")
		}
		if a.Kubernetes.Role == "" {
			return errInvalid("Auth.Kubernetes.Role is required")
		}
		if a.Token != nil || a.AppRole != nil || a.LDAP != nil {
			return errInvalid("Auth.Method=kubernetes must not also set Token / AppRole / LDAP")
		}
	case AuthMethodLDAP:
		if a.LDAP == nil {
			return errInvalid("Auth.Method=ldap requires Auth.LDAP")
		}
		if a.LDAP.Username == "" {
			return errInvalid("Auth.LDAP.Username is required")
		}
		if a.LDAP.Password == "" {
			return errInvalid("Auth.LDAP.Password is required")
		}
		if a.Token != nil || a.AppRole != nil || a.Kubernetes != nil {
			return errInvalid("Auth.Method=ldap must not also set Token / AppRole / Kubernetes")
		}
	case "":
		return errInvalid("Auth.Method is required (one of token, approle, kubernetes, ldap)")
	default:
		return errInvalid(fmt.Sprintf("Auth.Method %q is not supported (v1.0 supports token, approle, kubernetes, ldap; AWS IAM is gate-v1.0 ROADMAP)", a.Method))
	}
	return nil
}

// resolveKVVersion returns the KV engine version for mountPath. The
// match is segment-aware: a mount declared as "secret" matches paths
// "secret/foo" but not "secretstore/foo". Returns 2 (v2) when the
// mount isn't in the config — the modern Vault default.
func (c Config) resolveKVVersion(path string) int {
	for _, m := range c.Mounts {
		mount := strings.TrimSuffix(m.Path, "/")
		if path == mount || strings.HasPrefix(path, mount+"/") {
			if m.KVVersion == 0 {
				return 2
			}
			return m.KVVersion
		}
	}
	return 2
}
