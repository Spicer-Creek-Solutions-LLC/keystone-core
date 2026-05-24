// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"
	authk8s "github.com/hashicorp/vault/api/auth/kubernetes"
	authldap "github.com/hashicorp/vault/api/auth/ldap"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// errInvalid is the shorthand every validator in this package uses
// to surface a config / shape rejection — wraps [secrets.ErrInvalidBackend]
// so call sites match the family root with [errors.Is].
func errInvalid(msg string) error {
	return fmt.Errorf("%w: vault: %s", secrets.ErrInvalidBackend, msg)
}

// errBackend wraps an upstream Vault error with our family root.
func errBackend(action string, err error) error {
	return fmt.Errorf("%w: vault: %s: %v", secrets.ErrInvalidBackend, action, err)
}

// authResult is the bundle that comes out of [authenticate]. Vault's
// API client wants the token applied via `Client.SetToken`; the TTL
// + renewable bits drive the background renewer.
type authResult struct {
	Token     string
	TTLSec    int  // 0 for non-expiring (root) tokens
	Renewable bool
}

// newClient builds a [*vaultapi.Client] with TLS, namespace, and
// timeout populated from cfg. The client is NOT authenticated yet —
// the caller follows up with [authenticate].
func newClient(cfg Config) (*vaultapi.Client, error) {
	apiCfg := vaultapi.DefaultConfig()
	apiCfg.Address = cfg.Address
	apiCfg.Timeout = cfg.Timeout

	if cfg.MaxRetries >= 0 {
		apiCfg.MaxRetries = cfg.MaxRetries
	}

	tls := &vaultapi.TLSConfig{
		CACert:        cfg.TLS.CACert,
		CACertBytes:   cfg.TLS.CACertBytes,
		ClientCert:    cfg.TLS.ClientCert,
		ClientKey:     cfg.TLS.ClientKey,
		TLSServerName: cfg.TLS.ServerName,
		Insecure:      cfg.TLS.Insecure,
	}
	if err := apiCfg.ConfigureTLS(tls); err != nil {
		return nil, errInvalid(fmt.Sprintf("ConfigureTLS: %v", err))
	}

	c, err := vaultapi.NewClient(apiCfg)
	if err != nil {
		return nil, errInvalid(fmt.Sprintf("NewClient: %v", err))
	}
	if cfg.Namespace != "" {
		c.SetNamespace(cfg.Namespace)
	}
	return c, nil
}

// authenticate dispatches to the configured auth method and returns
// the resolved token + lifetime info. The caller is expected to call
// `client.SetToken(result.Token)` afterwards.
func authenticate(ctx context.Context, client *vaultapi.Client, auth AuthConfig) (authResult, error) {
	switch auth.Method {
	case AuthMethodToken:
		return authenticateToken(ctx, client, auth.Token)
	case AuthMethodAppRole:
		return authenticateAppRole(ctx, client, auth.AppRole)
	case AuthMethodKubernetes:
		return authenticateKubernetes(ctx, client, auth.Kubernetes)
	case AuthMethodLDAP:
		return authenticateLDAP(ctx, client, auth.LDAP)
	default:
		// validate() already rejects this; defensive.
		return authResult{}, errInvalid(fmt.Sprintf("Auth.Method %q not supported", auth.Method))
	}
}

// authenticateToken applies a pre-issued token and round-trips it
// through `auth/token/lookup-self` to verify validity + collect the
// remaining TTL + renewable bit.
func authenticateToken(ctx context.Context, client *vaultapi.Client, cfg *TokenAuthConfig) (authResult, error) {
	client.SetToken(cfg.Token)
	secret, err := client.Auth().Token().LookupSelfWithContext(ctx)
	if err != nil {
		return authResult{}, errBackend("token lookup-self", err)
	}
	if secret == nil || secret.Data == nil {
		return authResult{}, errInvalid("token lookup-self returned no data")
	}
	ttl, renewable := extractLookupSelfLifetime(secret.Data)
	return authResult{Token: cfg.Token, TTLSec: ttl, Renewable: renewable}, nil
}

func authenticateAppRole(ctx context.Context, client *vaultapi.Client, cfg *AppRoleAuthConfig) (authResult, error) {
	opts := []approle.LoginOption{}
	if cfg.Mount != "" {
		opts = append(opts, approle.WithMountPath(cfg.Mount))
	}
	secret := &approle.SecretID{FromString: cfg.SecretID}
	if cfg.SecretIDIsWrappingToken {
		secret = &approle.SecretID{FromString: cfg.SecretID, FromEnv: "", FromFile: ""}
		opts = append(opts, approle.WithWrappingToken())
	}
	auth, err := approle.NewAppRoleAuth(cfg.RoleID, secret, opts...)
	if err != nil {
		return authResult{}, errBackend("approle: NewAppRoleAuth", err)
	}
	return loginViaSDKAuth(ctx, client, auth, "approle")
}

func authenticateKubernetes(ctx context.Context, client *vaultapi.Client, cfg *KubernetesAuthConfig) (authResult, error) {
	opts := []authk8s.LoginOption{}
	if cfg.Mount != "" {
		opts = append(opts, authk8s.WithMountPath(cfg.Mount))
	}
	if cfg.TokenPath != "" {
		// Read the projected token path; the SDK accepts a JWT directly.
		raw, err := os.ReadFile(cfg.TokenPath) // #nosec G304 -- operator-supplied k8s SA token path from vault.auth.kubernetes config
		if err != nil {
			return authResult{}, errInvalid(fmt.Sprintf("kubernetes: read TokenPath %q: %v", cfg.TokenPath, err))
		}
		opts = append(opts, authk8s.WithServiceAccountToken(strings.TrimSpace(string(raw))))
	}
	auth, err := authk8s.NewKubernetesAuth(cfg.Role, opts...)
	if err != nil {
		return authResult{}, errBackend("kubernetes: NewKubernetesAuth", err)
	}
	return loginViaSDKAuth(ctx, client, auth, "kubernetes")
}

func authenticateLDAP(ctx context.Context, client *vaultapi.Client, cfg *LDAPAuthConfig) (authResult, error) {
	opts := []authldap.LoginOption{}
	if cfg.Mount != "" {
		opts = append(opts, authldap.WithMountPath(cfg.Mount))
	}
	auth, err := authldap.NewLDAPAuth(cfg.Username, &authldap.Password{FromString: cfg.Password}, opts...)
	if err != nil {
		return authResult{}, errBackend("ldap: NewLDAPAuth", err)
	}
	return loginViaSDKAuth(ctx, client, auth, "ldap")
}

// loginViaSDKAuth runs the shared SDK login flow: pass the auth
// helper to `client.Auth().Login`, then extract the token / TTL /
// renewable bit from the resulting [*vaultapi.Secret].
func loginViaSDKAuth(ctx context.Context, client *vaultapi.Client, helper vaultapi.AuthMethod, method string) (authResult, error) {
	secret, err := client.Auth().Login(ctx, helper)
	if err != nil {
		return authResult{}, errBackend(fmt.Sprintf("%s login", method), err)
	}
	if secret == nil || secret.Auth == nil {
		return authResult{}, errInvalid(fmt.Sprintf("%s login returned no auth", method))
	}
	return authResult{
		Token:     secret.Auth.ClientToken,
		TTLSec:    secret.Auth.LeaseDuration,
		Renewable: secret.Auth.Renewable,
	}, nil
}

// extractLookupSelfLifetime pulls the `ttl` + `renewable` bits out of
// the data map that `auth/token/lookup-self` returns. The Vault API
// shapes these as JSON numbers (`json.Number` after SDK decode) and
// JSON bools respectively.
func extractLookupSelfLifetime(data map[string]any) (int, bool) {
	ttl := 0
	if v, ok := data["ttl"]; ok {
		ttl = numToInt(v)
	}
	renewable := false
	if v, ok := data["renewable"]; ok {
		if b, ok := v.(bool); ok {
			renewable = b
		}
	}
	return ttl, renewable
}

// numToInt coerces a `json.Number` / float64 / int into a plain int.
// Vault's JSON decoder produces json.Number when the client is
// configured with UseNumber; the default config returns float64. We
// handle both.
func numToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case interface{ Int64() (int64, error) }: // json.Number
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	}
	return 0
}

// translateError funnels a [*vaultapi.ResponseError] into the canonical
// [secrets] sentinel family. Non-Vault errors (network glitches,
// context cancellation) get wrapped in [secrets.ErrInvalidBackend] so
// the broker sees one consistent error shape.
//
// action is a short human-readable label (e.g. "kv get",
// "lease renew") for the error message context.
func translateError(action, path string, err error) error {
	if err == nil {
		return nil
	}
	var rerr *vaultapi.ResponseError
	if errors.As(err, &rerr) {
		return translateResponseError(action, path, rerr)
	}
	return fmt.Errorf("%w: vault: %s%s: %v", secrets.ErrInvalidBackend, action, pathSuffix(path), err)
}

func translateResponseError(action, path string, rerr *vaultapi.ResponseError) error {
	detail := strings.Join(rerr.Errors, "; ")
	switch rerr.StatusCode {
	case http.StatusNotFound:
		// KV reads / kv-list 404 → ErrSecretNotFound. Other 404s
		// (unmounted engine paths) also map to NotFound — the broker
		// distinguishes via context.
		return fmt.Errorf("%w: vault: %s%s: %s", secrets.ErrSecretNotFound, action, pathSuffix(path), detail)
	case http.StatusForbidden:
		return fmt.Errorf("%w: vault: %s%s: permission denied: %s", secrets.ErrInvalidBackend, action, pathSuffix(path), detail)
	case http.StatusBadRequest:
		switch {
		case containsAny(detail, "lease not found", "lease not exist"):
			return fmt.Errorf("%w: vault: %s%s: %s", secrets.ErrLeaseNotFound, action, pathSuffix(path), detail)
		case containsAny(detail, "lease expired", "lease has expired"):
			return fmt.Errorf("%w: vault: %s%s: %s", secrets.ErrLeaseExpired, action, pathSuffix(path), detail)
		case containsAny(detail, "not renewable", "is not renewable"):
			return fmt.Errorf("%w: vault: %s%s: %s", secrets.ErrLeaseNotRenewable, action, pathSuffix(path), detail)
		}
	}
	return fmt.Errorf("%w: vault: %s%s: HTTP %d: %s", secrets.ErrInvalidBackend, action, pathSuffix(path), rerr.StatusCode, detail)
}

func pathSuffix(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(" %q", path)
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
