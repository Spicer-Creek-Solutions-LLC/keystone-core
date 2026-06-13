// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

func TestCAEnvelope_RoundTrip(t *testing.T) {
	t.Parallel()
	key, err := masterkey.NewRandom()
	if err != nil {
		t.Fatalf("NewRandom: %v", err)
	}
	plaintext := []byte("-----BEGIN PRIVATE KEY-----\nroundtrip\n-----END PRIVATE KEY-----\n")

	sealed, err := sealCAKey(plaintext, key)
	if err != nil {
		t.Fatalf("sealCAKey: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("sealed envelope contains the plaintext")
	}
	if [caEnvMagicLen]byte(sealed[:caEnvMagicLen]) != caKeyMagic {
		t.Errorf("sealed envelope missing magic prefix")
	}

	got, err := openCAKey(sealed, key)
	if err != nil {
		t.Fatalf("openCAKey: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("openCAKey = %q, want %q", got, plaintext)
	}
}

func TestCAEnvelope_FreshNoncePerSeal(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	a, _ := sealCAKey([]byte("same"), key)
	b, _ := sealCAKey([]byte("same"), key)
	if bytes.Equal(a, b) {
		t.Error("two seals of the same plaintext produced identical envelopes (nonce reuse?)")
	}
}

func TestCAEnvelope_WrongKey(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	other, _ := masterkey.NewRandom()
	sealed, _ := sealCAKey([]byte("secret"), key)

	_, err := openCAKey(sealed, other)
	if err == nil {
		t.Fatal("openCAKey with wrong key succeeded")
	}
	if !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err does not wrap ErrInvalidCAStorage: %v", err)
	}
	// The fingerprint guard should fire before AEAD.Open.
	if errors.Is(err, errNotCAEnvelope) {
		t.Errorf("wrong key should not look like a non-envelope: %v", err)
	}
}

func TestCAEnvelope_NotAnEnvelope(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	cases := map[string][]byte{
		"plaintext PEM": []byte("-----BEGIN PRIVATE KEY-----\nx\n-----END PRIVATE KEY-----\n"),
		"too short":     []byte("short"),
		"empty":         nil,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := openCAKey(in, key)
			if !errors.Is(err, errNotCAEnvelope) {
				t.Errorf("openCAKey(%s) err = %v, want errNotCAEnvelope", name, err)
			}
		})
	}
}

func TestCAEnvelope_Tampered(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	sealed, _ := sealCAKey([]byte("authenticate me"), key)

	// Flip a byte in the ciphertext region (past the fixed header).
	tampered := append([]byte(nil), sealed...)
	tampered[caEnvFixed] ^= 0xff

	_, err := openCAKey(tampered, key)
	if err == nil {
		t.Fatal("openCAKey accepted tampered ciphertext")
	}
	if !errors.Is(err, ErrInvalidCAStorage) || errors.Is(err, errNotCAEnvelope) {
		t.Errorf("tamper err = %v, want wrapped ErrInvalidCAStorage (auth failure)", err)
	}
}

func TestCAEnvelope_BadVersion(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	sealed, _ := sealCAKey([]byte("v"), key)
	sealed[caEnvMagicLen] = 0x02 // bump the version byte

	_, err := openCAKey(sealed, key)
	if err == nil || errors.Is(err, errNotCAEnvelope) {
		t.Fatalf("bad-version err = %v, want a version rejection", err)
	}
	if !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err does not wrap ErrInvalidCAStorage: %v", err)
	}
}
