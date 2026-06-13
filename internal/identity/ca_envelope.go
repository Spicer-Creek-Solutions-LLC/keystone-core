// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

// On-disk envelope for an encrypted CA private key (see
// [EncryptedFileCAStorage]). All fields are fixed-width; the ciphertext
// runs to the end of the file.
//
//	[16] magic         — "KSCORE-CA-KEY\0\0\0" (constant)
//	[1]  format version — 0x01 in v1.0
//	[8]  key fingerprint — sha256(master)[:8], so a wrong-key load fails
//	                       fast with a recognisable fingerprint mismatch
//	[12] nonce          — fresh crypto/rand per write
//	[N]  ciphertext     — AES-256-GCM output (includes the 16-byte tag)
//
// The magic distinguishes an encrypted key file from a plaintext PEM one
// at load time, which lets [EncryptedFileCAStorage] point an operator at
// the `kscore-identity ca encrypt` migration instead of failing opaquely.
const (
	caEnvMagicLen = 16
	caEnvNonceLen = 12
	caEnvVersion  = byte(0x01)
	caEnvFixed    = caEnvMagicLen + 1 + masterkey.FingerprintLen + caEnvNonceLen
)

// caKeyMagic is the literal 16-byte prefix on every encrypted CA key
// file. "KSCORE-CA-KEY" (13 bytes) + 3 NUL padding.
var caKeyMagic = [caEnvMagicLen]byte{
	'K', 'S', 'C', 'O', 'R', 'E', '-', 'C', 'A', '-', 'K', 'E', 'Y', 0x00, 0x00, 0x00,
}

// errNotCAEnvelope reports that a key file does not begin with the CA
// envelope magic — i.e. it is (most likely) a plaintext PEM key written
// by [FileCAStorage]. Callers surface a migrate-with-`ca encrypt` hint.
var errNotCAEnvelope = errors.New("identity: key file is not an encrypted CA envelope")

// sealCAKey encrypts a (PEM) CA private key under key, returning the
// framed envelope.
func sealCAKey(plaintext []byte, key masterkey.Key) ([]byte, error) {
	aead, err := newCAEnvelopeAEAD(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("%w: envelope nonce: %v", ErrInvalidCAStorage, err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, caEnvFixed+len(ciphertext))
	out = append(out, caKeyMagic[:]...)
	out = append(out, caEnvVersion)
	fp := key.FingerprintBytes()
	out = append(out, fp[:]...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// openCAKey decrypts a framed envelope, returning the plaintext (PEM) CA
// private key. It returns [errNotCAEnvelope] when the magic is absent
// (a plaintext key file), and a wrapped [ErrInvalidCAStorage] on a
// version mismatch, a fingerprint mismatch (wrong key), or a failed
// authentication (tampered/corrupt ciphertext).
func openCAKey(framed []byte, key masterkey.Key) ([]byte, error) {
	if len(framed) < caEnvFixed || [caEnvMagicLen]byte(framed[:caEnvMagicLen]) != caKeyMagic {
		return nil, errNotCAEnvelope
	}
	off := caEnvMagicLen
	if framed[off] != caEnvVersion {
		return nil, fmt.Errorf("%w: unsupported CA envelope version 0x%02x", ErrInvalidCAStorage, framed[off])
	}
	off++

	var fp [masterkey.FingerprintLen]byte
	copy(fp[:], framed[off:off+masterkey.FingerprintLen])
	off += masterkey.FingerprintLen
	if fp != key.FingerprintBytes() {
		return nil, fmt.Errorf("%w: master key fingerprint mismatch (file %x, key %s)", ErrInvalidCAStorage, fp, key.Fingerprint())
	}

	nonce := framed[off : off+caEnvNonceLen]
	off += caEnvNonceLen
	ciphertext := framed[off:]

	aead, err := newCAEnvelopeAEAD(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: decrypt CA key: %v", ErrInvalidCAStorage, err)
	}
	return plaintext, nil
}

// newCAEnvelopeAEAD builds the AES-256-GCM AEAD from the master key,
// zeroing the transient key-bytes copy once the cipher has captured it.
func newCAEnvelopeAEAD(key masterkey.Key) (cipher.AEAD, error) {
	kb := key.Bytes()
	defer func() {
		for i := range kb {
			kb[i] = 0
		}
	}()
	block, err := aes.NewCipher(kb)
	if err != nil {
		return nil, fmt.Errorf("%w: aes cipher: %v", ErrInvalidCAStorage, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: gcm: %v", ErrInvalidCAStorage, err)
	}
	return aead, nil
}
