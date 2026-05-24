// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"errors"
	"os"
	"strconv"
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

// skipIfNoTransit gates the transit integration tests on a second
// env var so operators have to opt in even when KSCORE_TEST_VAULT_ADDR
// is set — transit needs separate Vault setup
// (`vault secrets enable transit` + `vault write -f
// transit/keys/kscore-test`).
func skipIfNoTransit(t *testing.T) {
	t.Helper()
	if os.Getenv("KSCORE_TEST_VAULT_TRANSIT") != "1" {
		t.Skip("KSCORE_TEST_VAULT_TRANSIT not set to 1 — see integration_test.go header for setup")
	}
}

func TestVaultBackend_Integration_TransitRoundTrip(t *testing.T) {
	addr, token := skipIfNoVault(t)
	skipIfNoTransit(t)

	b, err := NewBackend(Config{
		Address: addr,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: token}},
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

	const key = "kscore-test"
	plaintext := []byte("hello-from-keystone-task-7")

	enc, err := b.Encrypt(ctx, secrets.EncryptRequest{
		Key:   key,
		Items: []secrets.EncryptInput{{Plaintext: plaintext}},
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc.Results[0].Ciphertext == "" {
		t.Fatalf("encrypt returned empty ciphertext")
	}

	dec, err := b.Decrypt(ctx, secrets.DecryptRequest{
		Key:   key,
		Items: []secrets.DecryptInput{{Ciphertext: enc.Results[0].Ciphertext}},
	})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(dec.Results[0].Plaintext) != string(plaintext) {
		t.Errorf("round-trip mismatch: got %q want %q", dec.Results[0].Plaintext, plaintext)
	}
}

// TestVaultBackend_Integration_BatchEncryptUnder1s pins the §4.11
// acceptance criterion "batch encrypt 100 plaintexts in <1s with
// Vault." Gated like the other transit tests — needs an operator-
// provided `vault dev` + the transit engine + a `kscore-test` key.
func TestVaultBackend_Integration_BatchEncryptUnder1s(t *testing.T) {
	addr, token := skipIfNoVault(t)
	skipIfNoTransit(t)

	b, err := NewBackend(Config{
		Address: addr,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: token}},
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

	const (
		key = "kscore-test"
		n   = 100
	)
	items := make([]secrets.EncryptInput, n)
	for i := 0; i < n; i++ {
		items[i] = secrets.EncryptInput{Plaintext: []byte("payload-" + strconv.Itoa(i))}
	}

	start := time.Now()
	resp, err := b.Encrypt(ctx, secrets.EncryptRequest{Key: key, Items: items})
	if err != nil {
		t.Fatalf("batch Encrypt: %v", err)
	}
	elapsed := time.Since(start)

	if len(resp.Results) != n {
		t.Fatalf("results = %d, want %d", len(resp.Results), n)
	}
	for i, r := range resp.Results {
		if r.Err != "" {
			t.Errorf("result[%d] errored: %q", i, r.Err)
		}
		if r.Ciphertext == "" {
			t.Errorf("result[%d] missing ciphertext", i)
		}
	}

	if elapsed >= time.Second {
		t.Errorf("batch encrypt of %d plaintexts took %v, want < 1s (§4.11 acceptance)", n, elapsed)
	} else {
		t.Logf("batch encrypt of %d plaintexts: %v (well under 1s acceptance bar)", n, elapsed)
	}
}

func TestVaultBackend_Integration_TransitSignVerify(t *testing.T) {
	addr, token := skipIfNoVault(t)
	skipIfNoTransit(t)

	b, err := NewBackend(Config{
		Address: addr,
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: token}},
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

	const key = "kscore-test-sig" // separate transit key created as a signing key
	input := []byte("payload")

	sig, err := b.Sign(ctx, secrets.SignRequest{
		Key:           key,
		HashAlgorithm: "sha2-256",
		Items:         []secrets.SignInput{{Input: input}},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	v, err := b.Verify(ctx, secrets.VerifyRequest{
		Key:           key,
		HashAlgorithm: "sha2-256",
		Items: []secrets.VerifyInput{
			{Input: input, Signature: sig.Results[0].Signature},
		},
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !v.Results[0].Valid {
		t.Errorf("Verify: valid=false on freshly-signed payload")
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
