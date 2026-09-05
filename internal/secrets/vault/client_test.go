// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// vaultTestServer wraps an httptest.Server with a handler dispatcher
// keyed on `METHOD path`. Tests register handlers per route; the
// fallback returns 404 so unexpected calls fail loudly.
type vaultTestServer struct {
	t   *testing.T
	srv *httptest.Server
	mu  atomic.Pointer[handlerMap]
}

type handlerMap = map[string]http.HandlerFunc

func newVaultTestServer(t *testing.T) *vaultTestServer {
	t.Helper()
	v := &vaultTestServer{t: t}
	empty := handlerMap{}
	v.mu.Store(&empty)
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := *v.mu.Load()
		key := r.Method + " " + r.URL.Path
		if h, ok := m[key]; ok {
			h(w, r)
			return
		}
		t.Logf("vault test server: no handler for %s", key)
		http.Error(w, fmt.Sprintf(`{"errors":["test server: no handler for %s"]}`, key), http.StatusNotFound)
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func (v *vaultTestServer) register(method, path string, h http.HandlerFunc) {
	current := *v.mu.Load()
	next := make(handlerMap, len(current)+1)
	for k, fn := range current {
		next[k] = fn
	}
	next[method+" "+path] = h
	v.mu.Store(&next)
}

func (v *vaultTestServer) addr() string { return v.srv.URL }

// writeJSON is the tiny canned-response helper used throughout.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// newAuthLookupSelf handler — required after every successful Token
// auth flow. Tests can override `ttl` / `renewable`.
func handleLookupSelf(ttl int, renewable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"ttl":       ttl,
				"renewable": renewable,
			},
		})
	}
}

// newApiClient builds a vault/api client pointing at the test server.
func newAPIClient(t *testing.T, addr string) *vaultapi.Client {
	t.Helper()
	c := vaultapi.DefaultConfig()
	c.Address = addr
	c.MaxRetries = 0 // disable retries so failure-mode tests fail fast
	cl, err := vaultapi.NewClient(c)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return cl
}

func TestAuthenticate_Token_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, true))

	client := newAPIClient(t, srv.addr())
	result, err := authenticate(context.Background(), client, AuthConfig{
		Method: AuthMethodToken,
		Token:  &TokenAuthConfig{Token: "s.dev"},
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.Token != "s.dev" {
		t.Errorf("Token = %q, want %q", result.Token, "s.dev")
	}
	if result.TTLSec != 3600 {
		t.Errorf("TTLSec = %d, want 3600", result.TTLSec)
	}
	if !result.Renewable {
		t.Errorf("Renewable = false, want true")
	}
}

func TestAuthenticate_Token_LookupFailure(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"errors": []string{"permission denied"}})
	})

	client := newAPIClient(t, srv.addr())
	_, err := authenticate(context.Background(), client, AuthConfig{
		Method: AuthMethodToken,
		Token:  &TokenAuthConfig{Token: "s.bad"},
	})
	if err == nil {
		t.Fatalf("authenticate succeeded with 403 lookup-self")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestAuthenticate_AppRole_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("PUT", "/v1/auth/approle/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"auth": map[string]any{
				"client_token":   "s.approle.token",
				"lease_duration": 1800,
				"renewable":      true,
			},
		})
	})
	client := newAPIClient(t, srv.addr())
	result, err := authenticate(context.Background(), client, AuthConfig{
		Method:  AuthMethodAppRole,
		AppRole: &AppRoleAuthConfig{RoleID: "role-1", SecretID: "secret-1"},
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.Token != "s.approle.token" {
		t.Errorf("Token = %q, want %q", result.Token, "s.approle.token")
	}
	if result.TTLSec != 1800 {
		t.Errorf("TTLSec = %d, want 1800", result.TTLSec)
	}
	if !result.Renewable {
		t.Errorf("Renewable = false, want true")
	}
}

func TestAuthenticate_AppRole_LoginFailure(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("PUT", "/v1/auth/approle/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": []string{"invalid role or secret id"},
		})
	})
	client := newAPIClient(t, srv.addr())
	_, err := authenticate(context.Background(), client, AuthConfig{
		Method:  AuthMethodAppRole,
		AppRole: &AppRoleAuthConfig{RoleID: "bad", SecretID: "bad"},
	})
	if err == nil {
		t.Fatalf("authenticate succeeded with bad approle login")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestAuthenticate_Kubernetes_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("PUT", "/v1/auth/kubernetes/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"auth": map[string]any{
				"client_token":   "s.k8s.token",
				"lease_duration": 900,
				"renewable":      true,
			},
		})
	})

	// Write a fake SA token to a temp file so the K8s helper can
	// read it (otherwise it tries the in-cluster default path).
	dir := t.TempDir()
	tokenPath := dir + "/sa-token"
	if err := writeFile(tokenPath, "ey.fake-jwt"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	client := newAPIClient(t, srv.addr())
	result, err := authenticate(context.Background(), client, AuthConfig{
		Method: AuthMethodKubernetes,
		Kubernetes: &KubernetesAuthConfig{
			Role:      "my-role",
			TokenPath: tokenPath,
		},
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.Token != "s.k8s.token" {
		t.Errorf("Token = %q, want s.k8s.token", result.Token)
	}
}

func TestAuthenticate_LDAP_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("PUT", "/v1/auth/ldap/login/alice", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"auth": map[string]any{
				"client_token":   "s.ldap.token",
				"lease_duration": 600,
				"renewable":      true,
			},
		})
	})
	client := newAPIClient(t, srv.addr())
	result, err := authenticate(context.Background(), client, AuthConfig{
		Method: AuthMethodLDAP,
		LDAP:   &LDAPAuthConfig{Username: "alice", Password: "hunter2"},
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.Token != "s.ldap.token" {
		t.Errorf("Token = %q, want %q", result.Token, "s.ldap.token")
	}
}

func TestNewClient_HonoursAddressNamespaceTimeouts(t *testing.T) {
	t.Parallel()
	cfg, err := Config{
		Address:   "https://vault.example",
		Namespace: "tenant-a",
		Auth:      AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s"}},
	}.validate()
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if c.Address() != "https://vault.example" {
		t.Errorf("Address = %q, want %q", c.Address(), "https://vault.example")
	}
	if c.Namespace() != "tenant-a" {
		t.Errorf("Namespace = %q, want %q", c.Namespace(), "tenant-a")
	}
}

func TestTranslateError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantSent error
	}{
		{
			name:     "404 maps to ErrSecretNotFound",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusNotFound, Errors: []string{"not found"}},
			wantSent: secrets.ErrSecretNotFound,
		},
		{
			name:     "403 maps to ErrInvalidBackend (permission denied)",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusForbidden, Errors: []string{"permission denied"}},
			wantSent: secrets.ErrInvalidBackend,
		},
		{
			name:     "400 lease not found",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusBadRequest, Errors: []string{"lease not found"}},
			wantSent: secrets.ErrLeaseNotFound,
		},
		{
			name:     "400 lease expired",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusBadRequest, Errors: []string{"lease has expired"}},
			wantSent: secrets.ErrLeaseExpired,
		},
		{
			name:     "400 not renewable",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusBadRequest, Errors: []string{"lease is not renewable"}},
			wantSent: secrets.ErrLeaseNotRenewable,
		},
		{
			name:     "500 maps to ErrInvalidBackend",
			err:      &vaultapi.ResponseError{StatusCode: http.StatusInternalServerError, Errors: []string{"backend offline"}},
			wantSent: secrets.ErrInvalidBackend,
		},
		{
			name:     "non-response error maps to ErrInvalidBackend",
			err:      fmt.Errorf("connection refused"),
			wantSent: secrets.ErrInvalidBackend,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := translateError("action", "kv/x", tc.err)
			if !errors.Is(err, tc.wantSent) {
				t.Errorf("translateError(%v) does not wrap %v: got %v", tc.err, tc.wantSent, err)
			}
			if !strings.Contains(err.Error(), "kv/x") {
				t.Errorf("err = %q, want path context", err.Error())
			}
		})
	}
}

func TestTranslateError_NilPassesThrough(t *testing.T) {
	t.Parallel()
	if err := translateError("x", "p", nil); err != nil {
		t.Errorf("translateError(nil) = %v, want nil", err)
	}
}

// writeFile is a tiny helper for the K8s auth test — keeps the
// dependency surface tight.
func writeFile(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0600)
}

func TestErrInvalid_WrapsFamily(t *testing.T) {
	t.Parallel()
	err := errInvalid("test detail")
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("errInvalid does not wrap ErrInvalidBackend: %v", err)
	}
	if !strings.Contains(err.Error(), "test detail") {
		t.Errorf("errInvalid msg lost: %v", err)
	}
}
