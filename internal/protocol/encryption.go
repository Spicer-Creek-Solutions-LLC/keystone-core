package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
)

// Sizes ADR-0005 § 5 fixes, as named at G37.
const (
	X25519PublicBytes = 32
	MLKEMPublicBytes  = 1184
	MLKEMCiphertext   = 1088
	AEADOverheadBytes = 16 // AES-256-GCM tag

	// RecipientKeyBytes is the public half of AST-5 and AST-8 on the wire.
	RecipientKeyBytes = X25519PublicBytes + MLKEMPublicBytes // 1216

	// SealOverheadBytes is what field 8 adds to a plaintext.
	SealOverheadBytes = X25519PublicBytes + MLKEMCiphertext + AEADOverheadBytes // 1136
)

// EncryptionContext is the HKDF info string. It is the same constant as the
// ML-DSA context for the same reason: one protocol, one domain.
const EncryptionContext = SignatureContext

// gcmNonce is twelve zero bytes, and that is a consequence rather than a
// shortcut. Each AEAD key is derived from an ephemeral keypair used exactly
// once, so it encrypts exactly one message and a counter would have nothing to
// count. Reusing a GCM nonce under a REPEATED key is catastrophic; under a
// single-use key there is no repetition to protect against. ADR-0005 § 5 says
// so out loud so a reader who finds a zero nonce finds the reason with it.
var gcmNonce = make([]byte, 12)

// DecryptionKey is the hybrid private half held by a recipient.
type DecryptionKey struct {
	x  *ecdh.PrivateKey
	ml *mlkem.DecapsulationKey768
}

// RecipientKey is the hybrid public half, 1216 bytes on the wire.
type RecipientKey struct {
	x  *ecdh.PublicKey
	ml *mlkem.EncapsulationKey768
}

func GenerateDecryptionKey() (*DecryptionKey, error) {
	x, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	ml, err := mlkem.GenerateKey768()
	if err != nil {
		return nil, err
	}
	return &DecryptionKey{x: x, ml: ml}, nil
}

func (k *DecryptionKey) Recipient() *RecipientKey {
	return &RecipientKey{x: k.x.PublicKey(), ml: k.ml.EncapsulationKey()}
}

func (r *RecipientKey) Bytes() []byte {
	out := make([]byte, 0, RecipientKeyBytes)
	out = append(out, r.x.Bytes()...)
	return append(out, r.ml.Bytes()...)
}

// ParseRecipientKey refuses anything that is not exactly the wire form. As with
// ParseVerifyingKey, the refusal is the operation's own code: a caller learning
// that a KEY was malformed, rather than that decryption failed, is the
// distinction § 9 declines to disclose.
func ParseRecipientKey(b []byte) (*RecipientKey, error) {
	if len(b) != RecipientKeyBytes {
		return nil, DecryptionFailed
	}
	x, err := ecdh.X25519().NewPublicKey(b[:X25519PublicBytes])
	if err != nil {
		return nil, DecryptionFailed
	}
	ml, err := mlkem.NewEncapsulationKey768(b[X25519PublicBytes:])
	if err != nil {
		return nil, DecryptionFailed
	}
	return &RecipientKey{x: x, ml: ml}, nil
}

// derive combines the two shared secrets into the AEAD key.
//
// The ephemeral public key and the KEM ciphertext are in the KDF input
// deliberately. Concatenating the two shared secrets alone is the known-weak
// way to build a hybrid; binding the transcript stops an attacker who can
// manipulate one half from steering the derived key.
func derive(ss1, ss2, ephPub, kemCT []byte) ([]byte, error) {
	transcript := make([]byte, 0, len(ss1)+len(ss2)+len(ephPub)+len(kemCT))
	transcript = append(transcript, ss1...)
	transcript = append(transcript, ss2...)
	transcript = append(transcript, ephPub...)
	transcript = append(transcript, kemCT...)
	return hkdf.Key(sha256.New, transcript, nil, EncryptionContext, 32)
}

// Seal produces field 8 for an encrypted class:
//
//	ephemeral X25519 public (32) || ML-KEM-768 ciphertext (1088) || AES-256-GCM output
func Seal(r *RecipientKey, plaintext []byte) ([]byte, error) {
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	ss1, err := eph.ECDH(r.x)
	if err != nil {
		return nil, DecryptionFailed
	}
	ss2, kemCT := r.ml.Encapsulate()

	key, err := derive(ss1, ss2, eph.PublicKey().Bytes(), kemCT)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	// The associated data is empty. § 4's signature already covers fields 1 to
	// 8, so a ciphertext cannot be moved to another envelope without failing
	// verification; non-empty AAD would add nothing and would make decryption
	// depend on the header being parsed and framed first, which § 9's
	// indistinguishable refusals would then have to account for.
	out := make([]byte, 0, SealOverheadBytes+len(plaintext))
	out = append(out, eph.PublicKey().Bytes()...)
	out = append(out, kemCT...)
	return aead.Seal(out, gcmNonce, plaintext, nil), nil
}

// Open reverses Seal. Every failure is DecryptionFailed and carries nothing.
//
// That is AC-11's second pair: a truncated ciphertext and one for the wrong
// recipient must be indistinguishable. They are, because there is no path here
// that returns anything else -- a short field, an unparseable ephemeral key, a
// KEM decapsulation that yields a different secret and an AEAD tag that does
// not match all arrive at the same return.
func Open(k *DecryptionKey, field8 []byte) ([]byte, error) {
	if len(field8) < SealOverheadBytes {
		return nil, DecryptionFailed
	}
	ephBytes := field8[:X25519PublicBytes]
	kemCT := field8[X25519PublicBytes : X25519PublicBytes+MLKEMCiphertext]
	sealed := field8[X25519PublicBytes+MLKEMCiphertext:]

	eph, err := ecdh.X25519().NewPublicKey(ephBytes)
	if err != nil {
		return nil, DecryptionFailed
	}
	ss1, err := k.x.ECDH(eph)
	if err != nil {
		return nil, DecryptionFailed
	}
	// ML-KEM decapsulation is designed not to fail on a wrong ciphertext: it
	// returns an unrelated secret instead, so the mismatch surfaces at the AEAD
	// tag rather than here. That is what makes the wrong-recipient case
	// indistinguishable from a corrupt one without any effort on our part.
	ss2, err := k.ml.Decapsulate(kemCT)
	if err != nil {
		return nil, DecryptionFailed
	}

	key, err := derive(ss1, ss2, ephBytes, kemCT)
	if err != nil {
		return nil, DecryptionFailed
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, DecryptionFailed
	}
	plaintext, err := aead.Open(nil, gcmNonce, sealed, nil)
	if err != nil {
		return nil, DecryptionFailed
	}
	return plaintext, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealEnvelope moves an envelope's plaintext payload into field 8, sealed to the
// recipient.
//
// It refuses a class that has no encryption recipient. ADR-0005 § 5 decides
// which three of the seven are encrypted, and sealing a presence heartbeat
// would not be a stronger envelope -- it would be a different protocol.
func SealEnvelope(r *RecipientKey, e Envelope) (Envelope, error) {
	if !Known(e.Class) {
		return Envelope{}, UnknownClass
	}
	if !Encrypted(e.Class) {
		return Envelope{}, MalformedEnvelope
	}
	// Refused at the sending boundary as well as the receiving one. Sealing an
	// envelope that carries a cleartext correlation identifier would produce a
	// well-formed ciphertext wrapped around a leak, and the caller would have
	// no reason to look.
	if e.CorrelationID != "" {
		return Envelope{}, MalformedEnvelope
	}
	sealed, err := Seal(r, e.Payload)
	if err != nil {
		return Envelope{}, err
	}
	e.Payload = sealed
	return e, nil
}

// OpenEnvelope recovers the plaintext of an encrypted class.
func OpenEnvelope(k *DecryptionKey, e Envelope) ([]byte, error) {
	if !Known(e.Class) {
		return nil, UnknownClass
	}
	if !Encrypted(e.Class) {
		return nil, MalformedEnvelope
	}
	if e.CorrelationID != "" {
		return nil, MalformedEnvelope
	}
	return Open(k, e.Payload)
}

// MarshalDecryptionKey and ParseDecryptionKey exist for local enrolled-agent
// persistence and for the vector binaries.
//
// A private half never leaves the host in this product: ADR-0003 § 4 generates
// all three keys on the agent and sends only public halves. A cross-process
// VECTOR is the cross-process exception, because the verifying process has no
// enrollment to have recorded anything. Naming the allowed uses here is the
// point -- a marshaller for a secret should say why it exists, so its next
// caller has to argue with the reason rather than just find the function.
func MarshalDecryptionKey(k *DecryptionKey) []byte {
	out := make([]byte, 0, X25519PublicBytes+len(k.ml.Bytes()))
	out = append(out, k.x.Bytes()...)
	return append(out, k.ml.Bytes()...)
}

func ParseDecryptionKey(b []byte) (*DecryptionKey, error) {
	if len(b) <= X25519PublicBytes {
		return nil, DecryptionFailed
	}
	x, err := ecdh.X25519().NewPrivateKey(b[:X25519PublicBytes])
	if err != nil {
		return nil, DecryptionFailed
	}
	ml, err := mlkem.NewDecapsulationKey768(b[X25519PublicBytes:])
	if err != nil {
		return nil, DecryptionFailed
	}
	return &DecryptionKey{x: x, ml: ml}, nil
}
