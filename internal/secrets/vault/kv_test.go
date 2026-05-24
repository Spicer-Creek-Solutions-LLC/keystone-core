// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// fixture builds a Backend ready to dispatch KV ops against a test
// Vault server. The Token-auth path is the simplest (no live login
// mock to register beyond lookup-self).
func newKVFixture(t *testing.T, mounts []MountConfig) (*Backend, *vaultTestServer) {
	t.Helper()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Mounts:  mounts,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.test"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort; revoke-self handler not registered so we
		// expect a warning log, not a test failure.
		_ = b.Stop(context.Background())
	})
	return b, srv
}

func TestKV_SplitMountAndKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		mount     string
		key       string
		wantError bool
	}{
		{"secret/app/db", "secret", "app/db", false},
		{"kv/x", "kv", "x", false},
		{"justmount", "justmount", "", false},
		{"", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			m, k, err := splitMountAndKey(tc.in)
			if (err != nil) != tc.wantError {
				t.Fatalf("splitMountAndKey(%q) err=%v, want error=%v", tc.in, err, tc.wantError)
			}
			if !tc.wantError {
				if m != tc.mount {
					t.Errorf("mount = %q, want %q", m, tc.mount)
				}
				if k != tc.key {
					t.Errorf("key = %q, want %q", k, tc.key)
				}
			}
		})
	}
}

func TestKVv2_GetSecret_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	srv.register("GET", "/v1/secret/data/app/db", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"password": "hunter2",
					"user":     "alice",
				},
				"metadata": map[string]any{
					"version":      3,
					"created_time": "2026-05-14T12:00:00Z",
				},
			},
		})
	})

	got, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "secret/app/db"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("password = %v, want hunter2", got.Data["password"])
	}
	if got.Version != 3 {
		t.Errorf("Version = %d, want 3", got.Version)
	}
}

func TestKVv2_GetSecret_NotFound(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	srv.register("GET", "/v1/secret/data/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{"errors": []string{}})
	})

	_, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "secret/missing"})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound", err)
	}
}

func TestKVv1_GetSecret_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "kv-legacy", KVVersion: 1}})

	srv.register("GET", "/v1/kv-legacy/app", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"password": "v1pass"},
		})
	})

	got, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv-legacy/app"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Data["password"] != "v1pass" {
		t.Errorf("password = %v, want v1pass", got.Data["password"])
	}
	if got.Version != 0 {
		t.Errorf("KV v1 should not set Version; got %d", got.Version)
	}
}

func TestKVv2_WriteSecret_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	var requestBody map[string]any
	srv.register("PUT", "/v1/secret/data/app/db", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &requestBody)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"version":      1,
				"created_time": "2026-05-14T12:00:00Z",
			},
		})
	})

	out, err := b.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "secret/app/db",
		Data: map[string]any{"password": "newpw"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if out.Version != 1 {
		t.Errorf("Version = %d, want 1", out.Version)
	}
	// Verify the body the SDK sent: should have a "data" wrapper.
	if data, ok := requestBody["data"].(map[string]any); !ok || data["password"] != "newpw" {
		t.Errorf("request body data wrapper missing/wrong: %#v", requestBody)
	}
}

func TestKVv2_WriteSecret_CAS(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	var lastBody map[string]any
	srv.register("PUT", "/v1/secret/data/cas", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &lastBody)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"version": 2, "created_time": "2026-05-14T12:00:00Z"},
		})
	})

	cas := uint64(1)
	_, err := b.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "secret/cas",
		Data: map[string]any{"k": "v"},
		CAS:  &cas,
	})
	if err != nil {
		t.Fatalf("WriteSecret CAS: %v", err)
	}
	opts, ok := lastBody["options"].(map[string]any)
	if !ok {
		t.Fatalf("options block missing: %#v", lastBody)
	}
	// JSON-decoded numbers come back as float64.
	if c, _ := opts["cas"].(float64); c != 1 {
		t.Errorf("cas option = %v, want 1", opts["cas"])
	}
}

func TestKVv1_WriteSecret_RejectsCAS(t *testing.T) {
	t.Parallel()
	b, _ := newKVFixture(t, []MountConfig{{Path: "kv-legacy", KVVersion: 1}})

	cas := uint64(1)
	_, err := b.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "kv-legacy/foo",
		Data: map[string]any{"x": "y"},
		CAS:  &cas,
	})
	if err == nil {
		t.Fatalf("WriteSecret with CAS on KV v1 = nil err")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if !strings.Contains(err.Error(), "CAS is not supported on KV v1") {
		t.Errorf("err = %q, want CAS-not-supported message", err.Error())
	}
}

func TestKVv2_DeleteSecret_SoftAndDestroy(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	deleted := false
	destroyed := false
	srv.register("DELETE", "/v1/secret/data/k1", func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		writeJSON(w, http.StatusNoContent, nil)
	})
	srv.register("DELETE", "/v1/secret/metadata/k1", func(w http.ResponseWriter, _ *http.Request) {
		destroyed = true
		writeJSON(w, http.StatusNoContent, nil)
	})

	if err := b.DeleteSecret(context.Background(), secrets.DeleteSecretRequest{Path: "secret/k1"}); err != nil {
		t.Fatalf("soft Delete: %v", err)
	}
	if !deleted {
		t.Errorf("soft Delete did not call DELETE data path")
	}

	if err := b.DeleteSecret(context.Background(), secrets.DeleteSecretRequest{Path: "secret/k1", Destroy: true}); err != nil {
		t.Fatalf("Destroy Delete: %v", err)
	}
	if !destroyed {
		t.Errorf("Destroy Delete did not call DELETE metadata path")
	}
}

func TestKVv2_ListSecrets_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	// Vault's LIST is encoded as a GET with ?list=true; vault/api
	// uses a non-standard "LIST" verb internally that arrives at our
	// handler. We register both PUT/LIST/GET in case the SDK choice
	// differs across versions.
	listHandler := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"keys": []any{"app", "cache", "web"}},
		})
	}
	srv.register("LIST", "/v1/secret/metadata/foo", listHandler)
	srv.register("GET", "/v1/secret/metadata/foo", listHandler)

	resp, err := b.ListSecrets(context.Background(), secrets.ListSecretsRequest{Prefix: "secret/foo"})
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if got, want := len(resp.Entries), 3; got != want {
		t.Errorf("entries = %d, want %d (%#v)", got, want, resp.Entries)
	}
}

func TestKVv1_ListSecrets(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "kv-legacy", KVVersion: 1}})

	listHandler := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"keys": []any{"a", "b", "c"}},
		})
	}
	srv.register("LIST", "/v1/kv-legacy/foo", listHandler)
	srv.register("GET", "/v1/kv-legacy/foo", listHandler)

	resp, err := b.ListSecrets(context.Background(), secrets.ListSecretsRequest{Prefix: "kv-legacy/foo"})
	if err != nil {
		t.Fatalf("ListSecrets v1: %v", err)
	}
	if got := len(resp.Entries); got != 3 {
		t.Errorf("entries = %d, want 3", got)
	}
}

func TestKVv2_ListSecrets_PaginationCursor(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, []MountConfig{{Path: "secret", KVVersion: 2}})

	listHandler := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"keys": []any{"a", "b", "c", "d"}},
		})
	}
	srv.register("LIST", "/v1/secret/metadata", listHandler)
	srv.register("GET", "/v1/secret/metadata", listHandler)

	resp, err := b.ListSecrets(context.Background(), secrets.ListSecretsRequest{Prefix: "secret", Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("page1 entries = %d, want 2", len(resp.Entries))
	}
	if resp.NextCursor == "" {
		t.Errorf("page1 NextCursor empty when limit hit")
	}
}

func TestRejoinListPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		prefix, name, want string
	}{
		{"", "k", "k"},
		{"secret/foo", "k", "secret/foo/k"},
		{"secret/foo/", "k", "secret/foo/k"},
	}
	for _, tc := range cases {
		if got := rejoinListPath(tc.prefix, tc.name); got != tc.want {
			t.Errorf("rejoinListPath(%q, %q) = %q, want %q", tc.prefix, tc.name, got, tc.want)
		}
	}
}
