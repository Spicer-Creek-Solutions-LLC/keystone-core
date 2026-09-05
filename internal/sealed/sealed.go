// SPDX-License-Identifier: Apache-2.0

// Package sealed encrypts a payload to the holder of a specific
// private key.
//
// It exists because of a property of the agent transport: every agent
// authenticates to NATS with the same deployment-wide token or
// credential file, and no per-subject permissions are configured. A
// subject is therefore not a boundary — any agent can subscribe to any
// other agent's subjects. Anything confidential that crosses NATS has
// to be confidential on its own, not by virtue of where it was
// published.
//
// The recipient is named by the public key in its bootstrap-issued
// SVID, which the control plane has already verified against the CA.
// Verifying who is asking and encrypting to that same key is one
// decision, not two: there is no window in which the server knows the
// requester but addresses the reply to something else.
//
// Construction is standard hybrid encryption. A fresh 256-bit content
// key encrypts the payload under AES-GCM; the content key is then
// wrapped to the recipient. ECDSA recipients get ECDH against an
// ephemeral key with HKDF-SHA256; RSA recipients get RSA-OAEP. Both
// are supported because the CA's key type is configurable
// (ECDSA-P256 default, P384 and RSA-2048/4096 available).
package sealed

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
)

// Algorithm identifiers carried in a Box so the opener does not have
// to infer the construction from which fields are populated.
const (
	AlgECDHAESGCM = "ecdh-hkdf-sha256-aes256gcm"
	AlgRSAAESGCM  = "rsa-oaep-sha256-aes256gcm"
)

// hkdfInfo domain-separates this key derivation from any other use of
// the same ECDH shared secret.
const hkdfInfo = "keystone-core/sealed/v1"

// Box is a sealed payload. Safe to publish on a subject every agent
// can read; only the holder of the recipient private key can open it.
type Box struct {
	Algorithm string `json:"alg"`
	// EphemeralPublicKey is the sender's one-time ECDH public key, in
	// the curve's uncompressed point encoding. ECDH recipients only.
	EphemeralPublicKey []byte `json:"epk,omitempty"`
	// WrappedKey is the RSA-OAEP-wrapped content key. RSA recipients
	// only; ECDH derives its key instead of wrapping one.
	WrappedKey []byte `json:"wk,omitempty"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ct"`
}

var (
	// ErrUnsupportedKey means the recipient key is of a type this
	// package cannot seal to.
	ErrUnsupportedKey = errors.New("sealed: unsupported key type")
	// ErrOpen means the box did not decrypt: wrong key, wrong
	// associated data, or tampering. Deliberately one error — telling
	// them apart would tell an attacker which guess was closer.
	ErrOpen = errors.New("sealed: cannot open box")
)

// Seal encrypts plaintext to pub.
//
// aad is authenticated but not encrypted. Pass the context the box is
// only valid in — the requesting agent's id and the request nonce —
// so a captured box cannot be replayed as the answer to a different
// question.
func Seal(pub crypto.PublicKey, plaintext, aad []byte) (*Box, error) {
	box := &Box{}
	var contentKey []byte

	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		// Plain ECIES: the derived key IS the AEAD key. A separate
		// content key wrapped under it would buy nothing -- the
		// derivation is already per-message, because the ephemeral is.
		epk, derived, err := ecdhWrap(key)
		if err != nil {
			return nil, err
		}
		box.Algorithm = AlgECDHAESGCM
		box.EphemeralPublicKey = epk
		contentKey = derived
	case *rsa.PublicKey:
		// RSA has no ephemeral to derive from, so here a random
		// content key is what makes each message's AEAD key distinct.
		contentKey = make([]byte, 32)
		if _, err := rand.Read(contentKey); err != nil {
			return nil, fmt.Errorf("sealed: content key: %w", err)
		}
		wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, contentKey, []byte(hkdfInfo))
		if err != nil {
			return nil, fmt.Errorf("sealed: rsa wrap: %w", err)
		}
		box.Algorithm = AlgRSAAESGCM
		box.WrappedKey = wrapped
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedKey, pub)
	}

	aead, err := newAEAD(contentKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("sealed: nonce: %w", err)
	}
	box.Nonce = nonce
	box.Ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	return box, nil
}

// Open decrypts a box with the recipient's private key. aad must be
// byte-identical to what Seal was given.
func Open(priv crypto.PrivateKey, box *Box, aad []byte) ([]byte, error) {
	if box == nil {
		return nil, fmt.Errorf("%w: nil box", ErrOpen)
	}
	var contentKey []byte

	switch box.Algorithm {
	case AlgECDHAESGCM:
		key, ok := priv.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: box is ECDH but key is %T", ErrUnsupportedKey, priv)
		}
		derived, err := ecdhUnwrap(key, box.EphemeralPublicKey)
		if err != nil {
			return nil, err
		}
		contentKey = derived
	case AlgRSAAESGCM:
		key, ok := priv.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: box is RSA but key is %T", ErrUnsupportedKey, priv)
		}
		var err error
		contentKey, err = rsa.DecryptOAEP(sha256.New(), rand.Reader, key, box.WrappedKey, []byte(hkdfInfo))
		if err != nil {
			return nil, fmt.Errorf("%w: rsa unwrap", ErrOpen)
		}
	default:
		return nil, fmt.Errorf("%w: algorithm %q", ErrUnsupportedKey, box.Algorithm)
	}

	aead, err := newAEAD(contentKey)
	if err != nil {
		return nil, err
	}
	if len(box.Nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("%w: nonce is %d bytes, want %d",
			ErrOpen, len(box.Nonce), aead.NonceSize())
	}
	plaintext, err := aead.Open(nil, box.Nonce, box.Ciphertext, aad)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}

// ecdhWrap generates an ephemeral key on the recipient's curve and
// derives a 32-byte wrapping key from the shared secret. Returns the
// ephemeral public key in SPKI form.
func ecdhWrap(pub *ecdsa.PublicKey) ([]byte, []byte, error) {
	recipient, err := pub.ECDH()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnsupportedKey, err)
	}
	ephemeral, err := recipient.Curve().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("sealed: ephemeral key: %w", err)
	}
	shared, err := ephemeral.ECDH(recipient)
	if err != nil {
		return nil, nil, fmt.Errorf("sealed: ecdh: %w", err)
	}
	derived, err := deriveKey(shared, ephemeral.PublicKey().Bytes(), recipient.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return ephemeral.PublicKey().Bytes(), derived, nil
}

// ecdhUnwrap repeats the derivation on the recipient side.
func ecdhUnwrap(priv *ecdsa.PrivateKey, ephemeralPub []byte) ([]byte, error) {
	recipient, err := priv.ECDH()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedKey, err)
	}
	var curve ecdh.Curve
	switch recipient.Curve() {
	case ecdh.P256():
		curve = ecdh.P256()
	case ecdh.P384():
		curve = ecdh.P384()
	case ecdh.P521():
		curve = ecdh.P521()
	default:
		return nil, fmt.Errorf("%w: unsupported curve", ErrUnsupportedKey)
	}
	ephemeral, err := curve.NewPublicKey(ephemeralPub)
	if err != nil {
		return nil, fmt.Errorf("%w: ephemeral public key", ErrOpen)
	}
	shared, err := recipient.ECDH(ephemeral)
	if err != nil {
		return nil, fmt.Errorf("%w: ecdh", ErrOpen)
	}
	return deriveKey(shared, ephemeralPub, recipient.PublicKey().Bytes())
}

// deriveKey binds the wrapping key to both public keys, so a shared
// secret computed against a different pairing derives a different key.
func deriveKey(shared, ephemeralPub, recipientPub []byte) ([]byte, error) {
	salt := make([]byte, 0, len(ephemeralPub)+len(recipientPub))
	salt = append(salt, ephemeralPub...)
	salt = append(salt, recipientPub...)
	key, err := hkdf.Key(sha256.New, shared, salt, hkdfInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("sealed: hkdf: %w", err)
	}
	return key, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("sealed: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sealed: gcm: %w", err)
	}
	return aead, nil
}
