package verify

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
)

// Verifier checks detached module signatures against a trust
// policy.
type Verifier struct {
	policy *TrustPolicy
}

// NewVerifier returns a verifier bound to policy.
func NewVerifier(policy *TrustPolicy) *Verifier {
	return &Verifier{policy: policy}
}

// Verify checks sig over blob. The signer's key must be trusted
// (sig.KeyID present in the policy). ECDSA/RSA verify over
// sha256(blob); Ed25519 over the raw blob (the cosign keyed-blob
// convention). Returns:
//
//   - ErrUnknownKeyID     — KeyID not trusted
//   - ErrUnsupportedAlgorithm — key/alg outside the v1.0 set
//   - ErrSignatureMismatch — tamper / wrong key / wrong content
func (v *Verifier) Verify(blob []byte, sig Signature) error {
	if v.policy == nil {
		return fmt.Errorf("%w: no trust policy", ErrUnknownKeyID)
	}
	pub, ok := v.policy.lookup(sig.KeyID)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKeyID, sig.KeyID)
	}
	wantAlg, err := algorithmFor(pub)
	if err != nil {
		return err
	}
	if sig.Algorithm != "" && sig.Algorithm != wantAlg {
		return fmt.Errorf("%w: signature says %q, key is %q",
			ErrUnsupportedAlgorithm, sig.Algorithm, wantAlg)
	}

	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		d := sha256.Sum256(blob)
		if !ecdsa.VerifyASN1(k, d[:], sig.Value) {
			return ErrSignatureMismatch
		}
	case *rsa.PublicKey:
		d := sha256.Sum256(blob)
		if err := rsa.VerifyPKCS1v15(k, crypto.SHA256, d[:], sig.Value); err != nil {
			return fmt.Errorf("%w: %v", ErrSignatureMismatch, err)
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(k, blob, sig.Value) {
			return ErrSignatureMismatch
		}
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedAlgorithm, pub)
	}
	return nil
}

// Sign produces a cosign-verify-compatible detached signature over
// blob with priv (RSA / ECDSA / Ed25519). The KeyID is derived from
// priv's public key. This is the keyed signer behind the task-14
// `kscore-module sign` CLI; the private key is a plain PKCS8/SEC1
// PEM ("local.key") — not cosign's encrypted keyfile (v1.x).
func Sign(blob []byte, priv crypto.Signer) (Signature, error) {
	// Validate the key type before anything else so an unsupported
	// signer is ErrUnsupportedAlgorithm (not an opaque marshal err).
	var (
		alg string
		val []byte
	)
	switch pk := priv.(type) {
	case *ecdsa.PrivateKey:
		d := sha256.Sum256(blob)
		s, err := ecdsa.SignASN1(rand.Reader, pk, d[:])
		if err != nil {
			return Signature{}, err
		}
		alg, val = AlgECDSASHA256, s
	case *rsa.PrivateKey:
		d := sha256.Sum256(blob)
		s, err := rsa.SignPKCS1v15(rand.Reader, pk, crypto.SHA256, d[:])
		if err != nil {
			return Signature{}, err
		}
		alg, val = AlgRSAPKCS1v15SHA256, s
	case ed25519.PrivateKey:
		alg, val = AlgEd25519, ed25519.Sign(pk, blob)
	default:
		return Signature{}, fmt.Errorf("%w: %T", ErrUnsupportedAlgorithm, priv)
	}
	id, err := KeyID(priv.Public())
	if err != nil {
		return Signature{}, err
	}
	return Signature{KeyID: id, Algorithm: alg, Value: val}, nil
}
