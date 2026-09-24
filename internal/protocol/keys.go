package protocol

import (
	"crypto/ed25519"
	"crypto/mldsa"
	"crypto/rand"
)

// Sizes ADR-0005 § 4 fixes, as amended at G37. They are constants rather than
// calls so a wrong-length key or signature is caught by comparison and never by
// a panic deep in a primitive.
const (
	Ed25519PublicBytes     = ed25519.PublicKeySize // 32
	MLDSAPublicBytes       = 1952
	VerifyingKeyBytes      = Ed25519PublicBytes + MLDSAPublicBytes   // 1984
	SigningPrivateKeyBytes = ed25519.SeedSize + mldsa.PrivateKeySize // 64
	Ed25519SignatureBytes  = ed25519.SignatureSize                   // 64
	MLDSASignatureBytes    = 3309
	SignatureBytes         = Ed25519SignatureBytes + MLDSASignatureBytes // 3373
)

// SignatureContext is FIPS 204's domain separator, set so a signature made for
// a Keystone envelope can never be replayed as a signature for anything else
// using the same key. The keys are single-purpose already; this costs nothing.
const SignatureContext = "keystone-envelope-v1"

func mldsaParams() mldsa.Parameters { return mldsa.MLDSA65() }

// SigningKey is the hybrid private half: Ed25519 and ML-DSA-65 together.
// ARCH-NATS-006 forbids reusing a NATS identity key here, and ADR-0003 § 4
// generates this one separately on the agent for that reason.
type SigningKey struct {
	ed ed25519.PrivateKey
	ml *mldsa.PrivateKey
}

// VerifyingKey is the hybrid public half, 1984 bytes on the wire.
type VerifyingKey struct {
	ed ed25519.PublicKey
	ml *mldsa.PublicKey
}

func GenerateSigningKey() (*SigningKey, error) {
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	mlPriv, err := mldsa.GenerateKey(mldsaParams())
	if err != nil {
		return nil, err
	}
	return &SigningKey{ed: edPriv, ml: mlPriv}, nil
}

func (k *SigningKey) Verifying() *VerifyingKey {
	return &VerifyingKey{
		ed: k.ed.Public().(ed25519.PublicKey),
		ml: k.ml.PublicKey(),
	}
}

// MarshalSigningKey returns the canonical local-persistence form: the
// Ed25519 seed followed by the ML-DSA-65 seed. The private half never leaves
// the agent host; ADR-0003 § 4 sends only the public verifying key during
// enrollment.
func MarshalSigningKey(k *SigningKey) []byte {
	out := make([]byte, 0, SigningPrivateKeyBytes)
	out = append(out, k.ed.Seed()...)
	return append(out, k.ml.Bytes()...)
}

// ParseSigningKey restores the canonical local-persistence form. It returns
// the same coarse refusal as public-key parsing so malformed key material does
// not create a more detailed oracle.
func ParseSigningKey(b []byte) (*SigningKey, error) {
	if len(b) != SigningPrivateKeyBytes {
		return nil, SignatureInvalid
	}
	ed := ed25519.NewKeyFromSeed(b[:ed25519.SeedSize])
	ml, err := mldsa.NewPrivateKey(mldsaParams(), b[ed25519.SeedSize:])
	if err != nil {
		return nil, SignatureInvalid
	}
	return &SigningKey{ed: ed, ml: ml}, nil
}

// Bytes is the wire form: Ed25519 public then ML-DSA-65 public, both fixed, so
// no internal framing is needed.
func (v *VerifyingKey) Bytes() []byte {
	out := make([]byte, 0, VerifyingKeyBytes)
	out = append(out, v.ed...)
	return append(out, v.ml.Bytes()...)
}

// ParseVerifyingKey refuses anything that is not exactly the wire form.
//
// The refusal is SignatureInvalid rather than a typed key error: a caller
// learning that a key was malformed, as opposed to that verification failed,
// is exactly the distinction ADR-0005 § 9 declines to disclose.
func ParseVerifyingKey(b []byte) (*VerifyingKey, error) {
	if len(b) != VerifyingKeyBytes {
		return nil, SignatureInvalid
	}
	ml, err := mldsa.NewPublicKey(mldsaParams(), b[Ed25519PublicBytes:])
	if err != nil {
		return nil, SignatureInvalid
	}
	ed := make(ed25519.PublicKey, Ed25519PublicBytes)
	copy(ed, b[:Ed25519PublicBytes])
	return &VerifyingKey{ed: ed, ml: ml}, nil
}
