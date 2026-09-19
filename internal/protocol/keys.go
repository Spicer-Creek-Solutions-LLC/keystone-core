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
	Ed25519PublicBytes    = ed25519.PublicKeySize // 32
	MLDSAPublicBytes      = 1952
	VerifyingKeyBytes     = Ed25519PublicBytes + MLDSAPublicBytes // 1984
	Ed25519SignatureBytes = ed25519.SignatureSize                 // 64
	MLDSASignatureBytes   = 3309
	SignatureBytes        = Ed25519SignatureBytes + MLDSASignatureBytes // 3373
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
