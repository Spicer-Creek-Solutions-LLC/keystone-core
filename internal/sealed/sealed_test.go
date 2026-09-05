// SPDX-License-Identifier: Apache-2.0

package sealed

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"testing"
)

func ecKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestSealOpen_RoundTrip(t *testing.T) {
	curves := map[string]elliptic.Curve{
		"P256": elliptic.P256(),
		"P384": elliptic.P384(),
		"P521": elliptic.P521(),
	}
	for name, curve := range curves {
		t.Run(name, func(t *testing.T) {
			key := ecKey(t, curve)
			plaintext := []byte("s3cret-password")
			aad := []byte("agent-1|nonce-1")

			box, err := Seal(&key.PublicKey, plaintext, aad)
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			if box.Algorithm != AlgECDHAESGCM {
				t.Errorf("Algorithm = %q, want %q", box.Algorithm, AlgECDHAESGCM)
			}
			if bytes.Contains(box.Ciphertext, plaintext) {
				t.Error("ciphertext contains the plaintext")
			}

			got, err := Open(key, box, aad)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Errorf("Open() = %q, want %q", got, plaintext)
			}
		})
	}

	t.Run("RSA", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa key: %v", err)
		}
		plaintext := []byte("s3cret-password")
		aad := []byte("agent-1|nonce-1")

		box, err := Seal(&key.PublicKey, plaintext, aad)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		if box.Algorithm != AlgRSAAESGCM {
			t.Errorf("Algorithm = %q, want %q", box.Algorithm, AlgRSAAESGCM)
		}
		got, err := Open(key, box, aad)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("Open() = %q, want %q", got, plaintext)
		}
	})
}

// The reason the package exists: every agent shares one NATS
// credential and can read every subject, so a box addressed to one
// agent must be useless to the others.
func TestOpen_WrongRecipientFails(t *testing.T) {
	recipient := ecKey(t, elliptic.P256())
	eavesdropper := ecKey(t, elliptic.P256())

	box, err := Seal(&recipient.PublicKey, []byte("s3cret"), []byte("agent-1"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := Open(eavesdropper, box, []byte("agent-1")); err == nil {
		t.Fatal("Open() error = nil: another agent read the box")
	}
}

// The AAD binds a box to the exact request it answers, so a captured
// reply cannot be replayed as the answer to a different question.
func TestOpen_WrongAADFails(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	box, err := Seal(&key.PublicKey, []byte("s3cret"), []byte("agent-1|nonce-1"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := Open(key, box, []byte("agent-1|nonce-2")); !errors.Is(err, ErrOpen) {
		t.Errorf("Open() error = %v, want ErrOpen", err)
	}
}

func TestOpen_Tampering(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	aad := []byte("agent-1")

	tests := []struct {
		name   string
		mutate func(*Box)
	}{
		{"ciphertext", func(b *Box) { b.Ciphertext[0] ^= 0xff }},
		{"nonce", func(b *Box) { b.Nonce[0] ^= 0xff }},
		{"ephemeral public key", func(b *Box) { b.EphemeralPublicKey[1] ^= 0xff }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			box, err := Seal(&key.PublicKey, []byte("s3cret"), aad)
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			tt.mutate(box)
			if _, err := Open(key, box, aad); err == nil {
				t.Error("Open() error = nil after tampering")
			}
		})
	}
}

// Two seals of the same plaintext to the same recipient must differ,
// or an observer learns that a repeated answer was given.
func TestSeal_IsNotDeterministic(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	first, err := Seal(&key.PublicKey, []byte("same"), nil)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	second, err := Seal(&key.PublicKey, []byte("same"), nil)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Error("two seals of the same plaintext produced the same ciphertext")
	}
	if bytes.Equal(first.EphemeralPublicKey, second.EphemeralPublicKey) {
		t.Error("ephemeral key was reused across seals")
	}
}

func TestSeal_UnsupportedKey(t *testing.T) {
	if _, err := Seal("not a key", []byte("x"), nil); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("Seal() error = %v, want ErrUnsupportedKey", err)
	}
}

func TestOpen_Errors(t *testing.T) {
	key := ecKey(t, elliptic.P256())

	t.Run("nil box", func(t *testing.T) {
		if _, err := Open(key, nil, nil); !errors.Is(err, ErrOpen) {
			t.Errorf("Open() error = %v, want ErrOpen", err)
		}
	})

	t.Run("unknown algorithm", func(t *testing.T) {
		if _, err := Open(key, &Box{Algorithm: "rot13"}, nil); !errors.Is(err, ErrUnsupportedKey) {
			t.Errorf("Open() error = %v, want ErrUnsupportedKey", err)
		}
	})

	t.Run("ecdh box opened with an rsa key", func(t *testing.T) {
		box, err := Seal(&key.PublicKey, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa key: %v", err)
		}
		if _, err := Open(rsaKey, box, nil); !errors.Is(err, ErrUnsupportedKey) {
			t.Errorf("Open() error = %v, want ErrUnsupportedKey", err)
		}
	})

	t.Run("malformed ephemeral public key", func(t *testing.T) {
		box, err := Seal(&key.PublicKey, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		box.EphemeralPublicKey = []byte{0x04, 0x00}
		if _, err := Open(key, box, nil); err == nil {
			t.Error("Open() error = nil for a malformed ephemeral key")
		}
	})

	t.Run("short nonce", func(t *testing.T) {
		box, err := Seal(&key.PublicKey, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		box.Nonce = box.Nonce[:4]
		if _, err := Open(key, box, nil); !errors.Is(err, ErrOpen) {
			t.Errorf("Open() error = %v, want ErrOpen", err)
		}
	})

	t.Run("curve mismatch", func(t *testing.T) {
		box, err := Seal(&key.PublicKey, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		if _, err := Open(ecKey(t, elliptic.P384()), box, nil); err == nil {
			t.Error("Open() error = nil across a curve mismatch")
		}
	})
}

// A Box crosses NATS as JSON, so it has to survive that round trip.
func TestBox_JSONRoundTrip(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	aad := []byte("agent-1")
	box, err := Seal(&key.PublicKey, []byte("s3cret"), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	encoded, err := json.Marshal(box)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(encoded, []byte("s3cret")) {
		t.Error("encoded box contains the plaintext")
	}
	var decoded Box
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := Open(key, &decoded, aad)
	if err != nil {
		t.Fatalf("Open after JSON round trip: %v", err)
	}
	if string(got) != "s3cret" {
		t.Errorf("Open() = %q, want %q", got, "s3cret")
	}
}
