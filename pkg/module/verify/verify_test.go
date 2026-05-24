// SPDX-License-Identifier: Apache-2.0

package verify_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

func pubPEM(t *testing.T, pub crypto.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func keys(t *testing.T) map[string]crypto.Signer {
	t.Helper()
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]crypto.Signer{"ecdsa": ec, "rsa": rs, "ed25519": ed}
}

func TestSignVerify_RoundTrip_AllAlgorithms(t *testing.T) {
	blob := []byte("the module zip bytes")
	for name, priv := range keys(t) {
		t.Run(name, func(t *testing.T) {
			tp := verify.NewTrustPolicy()
			id, err := tp.AddKey(priv.Public())
			if err != nil {
				t.Fatalf("AddKey: %v", err)
			}
			sig, err := verify.Sign(blob, priv)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if sig.KeyID != id {
				t.Fatalf("sig KeyID %q != policy id %q", sig.KeyID, id)
			}
			if err := verify.NewVerifier(tp).Verify(blob, sig); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestVerify_TamperDetected(t *testing.T) {
	priv := keys(t)["ecdsa"]
	tp := verify.NewTrustPolicy()
	_, _ = tp.AddKey(priv.Public())
	blob := []byte("original content")
	sig, _ := verify.Sign(blob, priv)

	v := verify.NewVerifier(tp)
	tampered := append([]byte(nil), blob...)
	tampered[0] ^= 0xFF
	if err := v.Verify(tampered, sig); !errors.Is(err, verify.ErrSignatureMismatch) {
		t.Fatalf("tamper = %v, want ErrSignatureMismatch", err)
	}
	// Flipped signature byte too.
	bad := sig
	bad.Value = append([]byte(nil), sig.Value...)
	bad.Value[0] ^= 0xFF
	if err := v.Verify(blob, bad); !errors.Is(err, verify.ErrSignatureMismatch) {
		t.Fatalf("bad sig = %v, want ErrSignatureMismatch", err)
	}
}

func TestVerify_UnknownAndWrongKey(t *testing.T) {
	k := keys(t)
	signer := k["ecdsa"]
	other := k["rsa"]
	blob := []byte("payload")
	sig, _ := verify.Sign(blob, signer)

	// Empty policy → unknown key id.
	if err := verify.NewVerifier(verify.NewTrustPolicy()).Verify(blob, sig); !errors.Is(err, verify.ErrUnknownKeyID) {
		t.Fatalf("empty policy = %v, want ErrUnknownKeyID", err)
	}
	// Nil policy.
	if err := verify.NewVerifier(nil).Verify(blob, sig); !errors.Is(err, verify.ErrUnknownKeyID) {
		t.Fatalf("nil policy = %v, want ErrUnknownKeyID", err)
	}
	// Policy holds a different key; signature's KeyID still absent.
	tp := verify.NewTrustPolicy()
	_, _ = tp.AddKey(other.Public())
	if err := verify.NewVerifier(tp).Verify(blob, sig); !errors.Is(err, verify.ErrUnknownKeyID) {
		t.Fatalf("wrong key = %v, want ErrUnknownKeyID", err)
	}
	// Algorithm tag mismatch vs the trusted key type.
	tp2 := verify.NewTrustPolicy()
	id, _ := tp2.AddKey(signer.Public())
	mis := verify.Signature{KeyID: id, Algorithm: verify.AlgEd25519, Value: sig.Value}
	if err := verify.NewVerifier(tp2).Verify(blob, mis); !errors.Is(err, verify.ErrUnsupportedAlgorithm) {
		t.Fatalf("alg mismatch = %v, want ErrUnsupportedAlgorithm", err)
	}
}

func TestKeyID_StableAndRotation(t *testing.T) {
	k := keys(t)
	a, b := k["ecdsa"], k["ed25519"]
	id1, _ := verify.KeyID(a.Public())
	id2, _ := verify.KeyID(a.Public())
	if id1 != id2 || id1 == "" {
		t.Fatalf("KeyID not stable: %q %q", id1, id2)
	}
	idB, _ := verify.KeyID(b.Public())
	if idB == id1 {
		t.Fatal("distinct keys must have distinct IDs")
	}
	// Rotation: a policy can trust both old + new key.
	tp := verify.NewTrustPolicy()
	_, _ = tp.AddKey(a.Public())
	_, _ = tp.AddKey(b.Public())
	if got := tp.KeyIDs(); len(got) != 2 {
		t.Fatalf("policy holds %d keys, want 2", len(got))
	}
	v := verify.NewVerifier(tp)
	for _, s := range []crypto.Signer{a, b} {
		sig, _ := verify.Sign([]byte("x"), s)
		if err := v.Verify([]byte("x"), sig); err != nil {
			t.Fatalf("rotation verify: %v", err)
		}
	}
}

func TestPEMLoadingAndErrors(t *testing.T) {
	priv := keys(t)["ed25519"]
	tp, err := verify.LoadTrustPolicy(pubPEM(t, priv.Public()))
	if err != nil {
		t.Fatalf("LoadTrustPolicy: %v", err)
	}
	if len(tp.KeyIDs()) != 1 {
		t.Fatalf("loaded %d keys", len(tp.KeyIDs()))
	}
	sig, _ := verify.Sign([]byte("data"), priv)
	if err := verify.NewVerifier(tp).Verify([]byte("data"), sig); err != nil {
		t.Fatalf("verify via PEM-loaded key: %v", err)
	}

	if _, err := verify.ParsePublicKeyPEM([]byte("not pem")); !errors.Is(err, verify.ErrInvalidKey) {
		t.Fatalf("garbage PEM = %v, want ErrInvalidKey", err)
	}
	if _, err := verify.LoadTrustPolicy([]byte("garbage")); err == nil {
		t.Fatal("LoadTrustPolicy(garbage): want error")
	}
	// Unsupported key type (DSA-ish: a non PKIX-parseable blob).
	bad := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{0x30, 0x00}})
	if _, err := verify.ParsePublicKeyPEM(bad); err == nil {
		t.Fatal("malformed PKIX: want error")
	}
}

func TestSign_UnsupportedSigner(t *testing.T) {
	if _, err := verify.Sign([]byte("x"), badSigner{}); !errors.Is(err, verify.ErrUnsupportedAlgorithm) {
		t.Fatalf("bad signer = %v, want ErrUnsupportedAlgorithm", err)
	}
}

// badSigner is a crypto.Signer whose key type is unsupported.
type badSigner struct{}

func (badSigner) Public() crypto.PublicKey { return struct{}{} }
func (badSigner) Sign(_ io.Reader, _ []byte, _ crypto.SignerOpts) ([]byte, error) {
	return nil, nil
}
