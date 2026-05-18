// Package verify is the module signature-verification half of the
// Epic 14 verification pipeline (PROJECT-DETAILS §4.18).
//
// v1.0 scope (the epic's "Cosign-only verification" decision):
// keyed, cosign-compatible *detached blob* signatures verified with
// the standard library only — RSA, ECDSA, Ed25519. A cosign
// `verify-blob --key pub.pem` signature is exactly
// verify(pubkey, sig, sha256(blob)) (ed25519 over the raw blob), so
// no sigstore/cosign dependency is needed and the repo's existing
// stdlib-crypto approach (internal/identity) is reused.
//
// Cosign *keyless* (Fulcio/Rekor), the encrypted cosign keyfile
// format, and the SumDB transparency log are deferred (epic
// non-goals; tracked in docs/project/ROADMAP.md).
//
// SHA-256 content addressing + CAS storage is the sibling task 5.
package verify

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
)

// Signature algorithm identifiers (the value carried in the
// detached `.sig` artifact alongside the module ZIP).
const (
	AlgECDSASHA256       = "ecdsa-sha256"
	AlgRSAPKCS1v15SHA256 = "rsa-pkcs1v15-sha256"
	AlgEd25519           = "ed25519"
)

var (
	// ErrUnknownKeyID — the signature's KeyID is not in the trust
	// policy (untrusted / unknown signer).
	ErrUnknownKeyID = errors.New("verify: signature key id not trusted")
	// ErrUnsupportedAlgorithm — key/algorithm not one of the v1.0
	// RSA/ECDSA/Ed25519 set.
	ErrUnsupportedAlgorithm = errors.New("verify: unsupported signature algorithm")
	// ErrSignatureMismatch — the signature did not verify (tamper /
	// wrong key / wrong content).
	ErrSignatureMismatch = errors.New("verify: signature does not match")
	// ErrInvalidKey — a PEM/DER key could not be parsed.
	ErrInvalidKey = errors.New("verify: invalid key")
)

// Signature is a detached module signature.
type Signature struct {
	KeyID     string // hex SHA-256 of the signer's PKIX public key
	Algorithm string // one of the Alg* constants
	Value     []byte // raw signature bytes (base64 on the wire)
}

// KeyID returns the stable identifier for a public key: lowercase
// hex SHA-256 of its PKIX-DER encoding. Deterministic + rotation-
// friendly (a new key ⇒ a new ID; the policy can hold both).
func KeyID(pub crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("%w: marshal pkix: %v", ErrInvalidKey, err)
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// algorithmFor reports the canonical Alg* string for a public key.
func algorithmFor(pub crypto.PublicKey) (string, error) {
	switch pub.(type) {
	case *ecdsa.PublicKey:
		return AlgECDSASHA256, nil
	case *rsa.PublicKey:
		return AlgRSAPKCS1v15SHA256, nil
	case ed25519.PublicKey:
		return AlgEd25519, nil
	default:
		return "", fmt.Errorf("%w: %T", ErrUnsupportedAlgorithm, pub)
	}
}

// ParsePublicKeyPEM decodes a single PKIX PEM public key.
func ParsePublicKeyPEM(pemBytes []byte) (crypto.PublicKey, error) {
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return nil, fmt.Errorf("%w: no PEM block", ErrInvalidKey)
	}
	pub, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	switch pub.(type) {
	case *ecdsa.PublicKey, *rsa.PublicKey, ed25519.PublicKey:
		return pub, nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedAlgorithm, pub)
	}
}
