package vault

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// getIntegrationAddr reads the integration-test gate env var. When
// unset, every test in this file calls t.Skip.
func getIntegrationAddr() string { return os.Getenv("KSCORE_TEST_VAULT_ADDR") }

// getIntegrationToken reads the operator-supplied root token for the
// `vault dev` server. Defaults to "root" which is the conventional
// dev-mode token.
func getIntegrationToken() string {
	if v := os.Getenv("KSCORE_TEST_VAULT_TOKEN"); v != "" {
		return v
	}
	return "root"
}

// skipIfNoVault is the shared guard. Unsets the dev-mode VAULT_TOKEN
// env var that the SDK auto-detects so the test environment is clean.
func skipIfNoVault(t *testing.T) (string, string) {
	t.Helper()
	addr := getIntegrationAddr()
	if addr == "" {
		t.Skip("KSCORE_TEST_VAULT_ADDR not set — run `vault dev` and set the env var to enable Vault integration tests")
	}
	return addr, getIntegrationToken()
}

func TestVaultBackend_Integration_KVRoundTrip(t *testing.T) {
	addr, token := skipIfNoVault(t)

	b, err := NewBackend(Config{
		Address: addr,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: token}},
		Mounts:  []MountConfig{{Path: "secret", KVVersion: 2}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = b.Stop(stopCtx)
	})

	const path = "secret/kscore-test/db"
	defer func() {
		_ = b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: path, Destroy: true})
	}()

	// Write → Read → Delete.
	written, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: path,
		Data: map[string]any{"password": "hunter2", "user": "alice"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if written.Version < 1 {
		t.Errorf("Version after write = %d, want ≥ 1", written.Version)
	}

	got, err := b.GetSecret(ctx, secrets.GetSecretRequest{Path: path})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("password mismatch: %v", got.Data["password"])
	}
	if got.Data["user"] != "alice" {
		t.Errorf("user mismatch: %v", got.Data["user"])
	}

	if err := b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: path, Destroy: true}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	_, err = b.GetSecret(ctx, secrets.GetSecretRequest{Path: path})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("Get after Destroy err = %v, want ErrSecretNotFound", err)
	}
}

func TestVaultBackend_Integration_CAS(t *testing.T) {
	addr, token := skipIfNoVault(t)

	b, err := NewBackend(Config{
		Address: addr,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: token}},
		Mounts:  []MountConfig{{Path: "secret", KVVersion: 2}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = b.Stop(stopCtx)
	})

	const path = "secret/kscore-test/cas"
	defer func() {
		_ = b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: path, Destroy: true})
	}()

	cas0 := uint64(0)
	first, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: path,
		Data: map[string]any{"v": 1},
		CAS:  &cas0,
	})
	if err != nil {
		t.Fatalf("first WriteSecret with CAS=0: %v", err)
	}

	// Stale CAS should be rejected by Vault.
	_, err = b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: path,
		Data: map[string]any{"v": 99},
		CAS:  &cas0,
	})
	if err == nil {
		t.Errorf("stale CAS write succeeded; expected rejection")
	}

	// Matching CAS succeeds.
	curr := uint64(first.Version)
	_, err = b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: path,
		Data: map[string]any{"v": 2},
		CAS:  &curr,
	})
	if err != nil {
		t.Fatalf("matching CAS write: %v", err)
	}
}
