// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// referenceHMAC computes the §4.14 signature directly using stdlib
// primitives, so the test verifies Sign without depending on Sign's
// own implementation.
func referenceHMAC(secret, payload string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestSign_ByteShapeLock(t *testing.T) {
	t.Parallel()
	// Receivers (and the task-13 X-Keystone-Signature header) depend
	// on the exact "sha256=<hex>" form — lock the bytes.
	got := Sign([]byte("k"), []byte("hello"))
	if !strings.HasPrefix(got, SignaturePrefix) {
		t.Fatalf("Sign = %q, want SignaturePrefix prefix", got)
	}
	if got != referenceHMAC("k", "hello") {
		t.Errorf("Sign = %q, want %q (stdlib reference HMAC)", got, referenceHMAC("k", "hello"))
	}
	// Determinism + input dependence.
	if Sign([]byte("k"), []byte("hello")) != got {
		t.Error("Sign is not deterministic")
	}
	if Sign([]byte("k2"), []byte("hello")) == got {
		t.Error("Sign must depend on secret")
	}
	if Sign([]byte("k"), []byte("hello!")) == got {
		t.Error("Sign must depend on payload")
	}
}

func TestSign_EmptySecret_Deterministic(t *testing.T) {
	t.Parallel()
	// Documented contract: empty secret still produces a deterministic
	// HMAC under the empty key. Callers (HTTPDispatcher) decide
	// whether to skip signing entirely when the secret is empty —
	// the helper itself does not.
	a := Sign(nil, []byte("payload"))
	b := Sign([]byte{}, []byte("payload"))
	if a != b {
		t.Errorf("nil-secret and empty-secret Sign disagree: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, SignaturePrefix) {
		t.Errorf("Sign(empty) = %q, want SignaturePrefix prefix", a)
	}
}

func TestVerify_RoundTrip(t *testing.T) {
	t.Parallel()
	secret := []byte("topsecret")
	payload := []byte(`{"event":"webhook.test","sub":"abc"}`)
	sig := Sign(secret, payload)
	if !Verify(secret, sig, payload) {
		t.Error("Verify(matching) = false, want true")
	}
}

func TestVerify_Rejects(t *testing.T) {
	t.Parallel()
	secret := []byte("topsecret")
	payload := []byte("payload-1")
	good := Sign(secret, payload)

	cases := []struct {
		name      string
		secret    []byte
		signature string
		payload   []byte
	}{
		{"wrong secret", []byte("other"), good, payload},
		{"tampered payload", secret, good, []byte("payload-2")},
		{"missing prefix", secret, strings.TrimPrefix(good, SignaturePrefix), payload},
		{"empty string", secret, "", payload},
		{"prefix only", secret, SignaturePrefix, payload},
		{"bad hex", secret, SignaturePrefix + "ZZZZ", payload},
		{"short hex", secret, SignaturePrefix + "abcd", payload},
		{"long hex", secret, good + "00", payload},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if Verify(c.secret, c.signature, c.payload) {
				t.Errorf("Verify = true, want false")
			}
		})
	}
}

func TestVerify_SHA256DigestSize(t *testing.T) {
	t.Parallel()
	// Sign always emits 64 hex chars (sha256.Size * 2). A signature
	// whose hex decodes to anything else must be rejected even when
	// the bytes happen to match a prefix.
	want := sha256.Size*2 + len(SignaturePrefix)
	if got := len(Sign([]byte("k"), []byte("hello"))); got != want {
		t.Errorf("Sign length = %d, want %d (prefix + 64 hex chars)", got, want)
	}
}
