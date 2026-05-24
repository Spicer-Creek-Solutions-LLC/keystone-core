// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// newTransitFixture is the shared setup — Token auth + an httptest
// Vault server. Each test registers its own transit handlers.
func newTransitFixture(t *testing.T) (*Backend, *vaultTestServer) {
	t.Helper()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	return b, srv
}

// readJSONBody decodes the request body into a map for assertions.
func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if len(raw) == 0 {
		return body
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return body
}

// ---- Encrypt ------------------------------------------------------

func TestTransit_Encrypt_Single(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	var captured map[string]any
	srv.register("PUT", "/v1/transit/encrypt/my-key", func(w http.ResponseWriter, r *http.Request) {
		captured = readJSONBody(t, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"ciphertext":  "vault:v1:AAAA",
				"key_version": 1,
			},
		})
	})

	resp, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key:   "my-key",
		Items: []secrets.EncryptInput{{Plaintext: []byte("hello")}},
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].Ciphertext != "vault:v1:AAAA" {
		t.Errorf("ciphertext = %q", resp.Results[0].Ciphertext)
	}
	if resp.Results[0].KeyVersion != 1 {
		t.Errorf("key_version = %d, want 1", resp.Results[0].KeyVersion)
	}
	// Verify the body — single form uses `plaintext`, not `batch_input`.
	if _, ok := captured["batch_input"]; ok {
		t.Errorf("single-op used batch_input: %#v", captured)
	}
	wantPlain := base64.StdEncoding.EncodeToString([]byte("hello"))
	if captured["plaintext"] != wantPlain {
		t.Errorf("plaintext = %v, want %q", captured["plaintext"], wantPlain)
	}
}

func TestTransit_Encrypt_Batch(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	var captured map[string]any
	srv.register("PUT", "/v1/transit/encrypt/k", func(w http.ResponseWriter, r *http.Request) {
		captured = readJSONBody(t, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"batch_results": []any{
					map[string]any{"ciphertext": "vault:v1:A", "key_version": 1},
					map[string]any{"ciphertext": "vault:v1:B", "key_version": 1},
				},
			},
		})
	})

	resp, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key: "k",
		Items: []secrets.EncryptInput{
			{Plaintext: []byte("a")},
			{Plaintext: []byte("b")},
		},
	})
	if err != nil {
		t.Fatalf("Encrypt batch: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	batch, ok := captured["batch_input"].([]any)
	if !ok {
		t.Fatalf("batch_input missing/wrong type: %#v", captured)
	}
	if len(batch) != 2 {
		t.Errorf("batch entries = %d, want 2", len(batch))
	}
}

func TestTransit_Encrypt_Convergent(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	var captured map[string]any
	srv.register("PUT", "/v1/transit/encrypt/k", func(w http.ResponseWriter, r *http.Request) {
		captured = readJSONBody(t, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"ciphertext": "vault:v1:CCCC", "key_version": 1},
		})
	})

	_, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key: "k",
		Items: []secrets.EncryptInput{
			{Plaintext: []byte("hello"), Context: []byte("tenant-a")},
		},
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if got, want := captured["context"], base64.StdEncoding.EncodeToString([]byte("tenant-a")); got != want {
		t.Errorf("context = %v, want %q (convergent derivation context not propagated)", got, want)
	}
}

func TestTransit_Encrypt_KeyVersion(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	var captured map[string]any
	srv.register("PUT", "/v1/transit/encrypt/k", func(w http.ResponseWriter, r *http.Request) {
		captured = readJSONBody(t, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"ciphertext": "vault:v2:KK", "key_version": 2},
		})
	})

	_, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key:   "k",
		Items: []secrets.EncryptInput{{Plaintext: []byte("x"), KeyVersion: 2}},
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// JSON-decoded numbers come back as float64.
	if v, _ := captured["key_version"].(float64); int(v) != 2 {
		t.Errorf("key_version = %v, want 2", captured["key_version"])
	}
}

func TestTransit_Encrypt_PartialBatchFailure(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/encrypt/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"batch_results": []any{
					map[string]any{"ciphertext": "vault:v1:A", "key_version": 1},
					map[string]any{"error": "context required for derived key"},
				},
			},
		})
	})

	resp, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key: "k",
		Items: []secrets.EncryptInput{
			{Plaintext: []byte("a"), Context: []byte("ctx")},
			{Plaintext: []byte("b")}, // missing context for the derived key
		},
	})
	if err != nil {
		t.Fatalf("Encrypt partial-batch: top-level err = %v, want nil", err)
	}
	if resp.Results[0].Err != "" {
		t.Errorf("entry 0 unexpectedly errored: %q", resp.Results[0].Err)
	}
	if resp.Results[1].Err == "" {
		t.Errorf("entry 1 missing per-item error")
	}
}

func TestTransit_Encrypt_ForbiddenTranslates(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/encrypt/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"errors": []string{"permission denied"}})
	})

	_, err := b.Encrypt(context.Background(), secrets.EncryptRequest{
		Key:   "k",
		Items: []secrets.EncryptInput{{Plaintext: []byte("x")}},
	})
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err = %v, want ErrInvalidBackend wrap", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %q, want permission-denied message", err.Error())
	}
}

// ---- Decrypt ------------------------------------------------------

func TestTransit_Decrypt_Single(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/decrypt/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"plaintext": base64.StdEncoding.EncodeToString([]byte("hello")),
			},
		})
	})

	resp, err := b.Decrypt(context.Background(), secrets.DecryptRequest{
		Key:   "k",
		Items: []secrets.DecryptInput{{Ciphertext: "vault:v1:AAAA"}},
	})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got := string(resp.Results[0].Plaintext); got != "hello" {
		t.Errorf("plaintext = %q, want hello", got)
	}
}

func TestTransit_Decrypt_BatchWithMixedErrors(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/decrypt/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"batch_results": []any{
					map[string]any{"plaintext": base64.StdEncoding.EncodeToString([]byte("a"))},
					map[string]any{"error": "malformed ciphertext"},
				},
			},
		})
	})

	resp, _ := b.Decrypt(context.Background(), secrets.DecryptRequest{
		Key: "k",
		Items: []secrets.DecryptInput{
			{Ciphertext: "vault:v1:A"},
			{Ciphertext: "garbage"},
		},
	})
	if string(resp.Results[0].Plaintext) != "a" {
		t.Errorf("entry 0 plaintext = %q, want a", resp.Results[0].Plaintext)
	}
	if resp.Results[1].Err == "" {
		t.Errorf("entry 1 should carry per-item error")
	}
}

// ---- Sign / Verify ------------------------------------------------

func TestTransit_Sign_Single(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/sign/k/sha2-256", func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if body["input"] == "" {
			t.Errorf("input field missing: %#v", body)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"signature":   "vault:v1:SIG",
				"key_version": 1,
			},
		})
	})

	resp, err := b.Sign(context.Background(), secrets.SignRequest{
		Key:           "k",
		HashAlgorithm: "sha2-256",
		Items:         []secrets.SignInput{{Input: []byte("payload")}},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if resp.Results[0].Signature != "vault:v1:SIG" {
		t.Errorf("signature = %q", resp.Results[0].Signature)
	}
}

func TestTransit_Sign_HashAlgoOmittedNoPathSuffix(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	// Default-hash sign goes to /transit/sign/k (no algo suffix).
	srv.register("PUT", "/v1/transit/sign/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"signature": "vault:v1:S", "key_version": 1},
		})
	})

	_, err := b.Sign(context.Background(), secrets.SignRequest{
		Key:   "k",
		Items: []secrets.SignInput{{Input: []byte("p")}},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
}

func TestTransit_Verify_HappyAndMismatch(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/verify/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"valid": false},
		})
	})

	resp, err := b.Verify(context.Background(), secrets.VerifyRequest{
		Key: "k",
		Items: []secrets.VerifyInput{
			{Input: []byte("p"), Signature: "vault:v1:WRONG"},
		},
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if resp.Results[0].Valid {
		t.Errorf("Valid = true, want false (signature mismatch should NOT raise a top-level error)")
	}
}

// ---- HMAC / VerifyHMAC --------------------------------------------

func TestTransit_HMAC_Single(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/hmac/k/sha2-256", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"hmac":        "vault:v1:HMAC",
				"key_version": 1,
			},
		})
	})

	resp, err := b.HMAC(context.Background(), secrets.HMACRequest{
		Key:       "k",
		Algorithm: "sha2-256",
		Items:     []secrets.HMACInput{{Input: []byte("p")}},
	})
	if err != nil {
		t.Fatalf("HMAC: %v", err)
	}
	if resp.Results[0].HMAC != "vault:v1:HMAC" {
		t.Errorf("hmac = %q", resp.Results[0].HMAC)
	}
}

func TestTransit_VerifyHMAC(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/verify/k", func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if body["hmac"] == nil {
			t.Errorf("hmac field missing: %#v", body)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"valid": true},
		})
	})

	resp, err := b.VerifyHMAC(context.Background(), secrets.VerifyHMACRequest{
		Key: "k",
		Items: []secrets.VerifyHMACInput{
			{Input: []byte("p"), HMAC: "vault:v1:HMAC"},
		},
	})
	if err != nil {
		t.Fatalf("VerifyHMAC: %v", err)
	}
	if !resp.Results[0].Valid {
		t.Errorf("Valid = false, want true")
	}
}

// ---- Rewrap -------------------------------------------------------

func TestTransit_Rewrap(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/rewrap/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"ciphertext":  "vault:v2:NEW",
				"key_version": 2,
			},
		})
	})

	resp, err := b.Rewrap(context.Background(), secrets.RewrapRequest{
		Key:   "k",
		Items: []secrets.RewrapInput{{Ciphertext: "vault:v1:OLD"}},
	})
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	if resp.Results[0].Ciphertext != "vault:v2:NEW" {
		t.Errorf("ciphertext = %q", resp.Results[0].Ciphertext)
	}
	if resp.Results[0].KeyVersion != 2 {
		t.Errorf("key_version = %d, want 2", resp.Results[0].KeyVersion)
	}
}

// ---- GenerateDataKey ---------------------------------------------

func TestTransit_GenerateDataKey_Plaintext(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/datakey/plaintext/k", func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if v, _ := body["bits"].(float64); int(v) != 256 {
			t.Errorf("bits = %v, want 256", body["bits"])
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"plaintext":   base64.StdEncoding.EncodeToString(make([]byte, 32)),
				"ciphertext":  "vault:v1:WRAPPED",
				"key_version": 1,
			},
		})
	})

	resp, err := b.GenerateDataKey(context.Background(), secrets.GenerateDataKeyRequest{
		Key:  "k",
		Mode: secrets.DataKeyModePlaintext,
		Bits: 256,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey: %v", err)
	}
	if len(resp.Plaintext) != 32 {
		t.Errorf("plaintext len = %d, want 32", len(resp.Plaintext))
	}
	if resp.Ciphertext != "vault:v1:WRAPPED" {
		t.Errorf("ciphertext = %q", resp.Ciphertext)
	}
}

func TestTransit_GenerateDataKey_Wrapped(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	srv.register("PUT", "/v1/transit/datakey/wrapped/k", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"ciphertext":  "vault:v1:WRAPPED",
				"key_version": 1,
			},
		})
	})

	resp, err := b.GenerateDataKey(context.Background(), secrets.GenerateDataKeyRequest{
		Key:  "k",
		Mode: secrets.DataKeyModeWrapped,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey: %v", err)
	}
	if resp.Plaintext != nil {
		t.Errorf("Plaintext = %x, want nil for wrapped mode", resp.Plaintext)
	}
	if resp.Ciphertext != "vault:v1:WRAPPED" {
		t.Errorf("ciphertext = %q", resp.Ciphertext)
	}
}

func TestTransit_GenerateDataKey_DefaultsToPlaintext(t *testing.T) {
	t.Parallel()
	b, srv := newTransitFixture(t)

	called := false
	srv.register("PUT", "/v1/transit/datakey/plaintext/k", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"plaintext":   base64.StdEncoding.EncodeToString(make([]byte, 32)),
				"ciphertext":  "vault:v1:WRAPPED",
				"key_version": 1,
			},
		})
	})

	_, err := b.GenerateDataKey(context.Background(), secrets.GenerateDataKeyRequest{Key: "k"})
	if err != nil {
		t.Fatalf("GenerateDataKey: %v", err)
	}
	if !called {
		t.Errorf("default mode did not hit plaintext endpoint")
	}
}

func TestTransit_GenerateDataKey_InvalidMode(t *testing.T) {
	t.Parallel()
	b, _ := newTransitFixture(t)

	_, err := b.GenerateDataKey(context.Background(), secrets.GenerateDataKeyRequest{
		Key:  "k",
		Mode: secrets.DataKeyMode("bogus"),
	})
	if err == nil {
		t.Fatalf("bogus mode accepted")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
}

// ---- Validation ---------------------------------------------------

func TestTransit_ValidationRejections(t *testing.T) {
	t.Parallel()
	b, _ := newTransitFixture(t)
	ctx := context.Background()

	cases := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Encrypt empty key",
			fn: func() error {
				_, err := b.Encrypt(ctx, secrets.EncryptRequest{Items: []secrets.EncryptInput{{Plaintext: []byte("x")}}})
				return err
			},
		},
		{
			name: "Encrypt empty items",
			fn: func() error {
				_, err := b.Encrypt(ctx, secrets.EncryptRequest{Key: "k"})
				return err
			},
		},
		{
			name: "Decrypt empty key",
			fn: func() error {
				_, err := b.Decrypt(ctx, secrets.DecryptRequest{Items: []secrets.DecryptInput{{Ciphertext: "v"}}})
				return err
			},
		},
		{
			name: "Sign empty items",
			fn: func() error {
				_, err := b.Sign(ctx, secrets.SignRequest{Key: "k"})
				return err
			},
		},
		{
			name: "Verify empty items",
			fn: func() error {
				_, err := b.Verify(ctx, secrets.VerifyRequest{Key: "k"})
				return err
			},
		},
		{
			name: "HMAC empty items",
			fn: func() error {
				_, err := b.HMAC(ctx, secrets.HMACRequest{Key: "k"})
				return err
			},
		},
		{
			name: "VerifyHMAC empty items",
			fn: func() error {
				_, err := b.VerifyHMAC(ctx, secrets.VerifyHMACRequest{Key: "k"})
				return err
			},
		},
		{
			name: "Rewrap empty items",
			fn: func() error {
				_, err := b.Rewrap(ctx, secrets.RewrapRequest{Key: "k"})
				return err
			},
		},
		{
			name: "GenerateDataKey empty key",
			fn: func() error {
				_, err := b.GenerateDataKey(ctx, secrets.GenerateDataKeyRequest{})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s: nil err", tc.name)
			}
			if !errors.Is(err, secrets.ErrInvalidBackend) {
				t.Errorf("%s err does not wrap ErrInvalidBackend: %v", tc.name, err)
			}
		})
	}
}

// ---- Interface conformance ---------------------------------------

func TestTransitInterfaceConformance(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	var _ secrets.TransitBackend = b
}

// ---- Helpers used internally -------------------------------------

func TestBase64Encode(t *testing.T) {
	t.Parallel()
	got := base64Encode([]byte("hello"))
	want := "aGVsbG8="
	if got != want {
		t.Errorf("base64Encode = %q, want %q", got, want)
	}
}
