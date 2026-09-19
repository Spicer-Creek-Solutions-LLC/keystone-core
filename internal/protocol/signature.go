package protocol

import (
	"crypto/ed25519"
	"crypto/mldsa"
)

// Sign produces field 9: the Ed25519 signature followed by the ML-DSA-65
// signature, both over the same signed input.
//
// ML-DSA signing is randomized, so signing the same input twice with the same
// key gives different bytes. Ed25519 is deterministic and does not. ADR-0005
// § Consequences records what that means: golden vectors cover the framing and
// the signed input, never whole envelopes.
func Sign(k *SigningKey, signedInput []byte) ([]byte, error) {
	edSig := ed25519.Sign(k.ed, signedInput)
	mlSig, err := k.ml.Sign(nil, signedInput, &mldsa.Options{Context: SignatureContext})
	if err != nil {
		return nil, err
	}
	if len(edSig) != Ed25519SignatureBytes || len(mlSig) != MLDSASignatureBytes {
		return nil, SignatureInvalid
	}
	out := make([]byte, 0, SignatureBytes)
	out = append(out, edSig...)
	return append(out, mlSig...), nil
}

// Verify checks field 9 against the signed input. Hybrid means conjunction: a
// forgery must break Ed25519 AND ML-DSA-65, so both are required.
//
// BOTH ARE ALWAYS COMPUTED, and the result is combined afterwards. Returning as
// soon as the cheap one fails would leak which half rejected the envelope
// through timing, and there is no cost worth that: a verifier that short-circuits
// tells an attacker whether their forgery got past Ed25519.
//
// Every failure is SignatureInvalid with nothing attached. A wrong key and a
// corrupt signature are the same answer -- AC-11's first pair -- and they are
// the same answer because there is nowhere here for them to differ.
func Verify(v *VerifyingKey, signedInput, signature []byte) error {
	if len(signature) != SignatureBytes {
		return SignatureInvalid
	}
	okEd := ed25519.Verify(v.ed, signedInput, signature[:Ed25519SignatureBytes])
	okML := mldsa.Verify(v.ml, signedInput, signature[Ed25519SignatureBytes:],
		&mldsa.Options{Context: SignatureContext}) == nil
	if !okEd || !okML {
		return SignatureInvalid
	}
	return nil
}

// SignerRole is which key signs a class, per ADR-0005 § 7's Signer row. The
// mapping is part of the protocol: a result signed by the service, or a command
// signed by an agent, is not a differently-keyed envelope but a different claim
// about who said it.
type SignerRole string

const (
	// RoleAgentEnrolling is the agent's NEW signing key, presented in the
	// enrollment request it is being enrolled by. Nothing has recorded it yet,
	// which is why it is its own role rather than RoleAgent.
	RoleAgentEnrolling SignerRole = "agent-enrolling"
	RoleService        SignerRole = "service"
	RoleAgent          SignerRole = "agent"
)

var signerOf = map[Class]SignerRole{
	ClassEnrollmentRequest: RoleAgentEnrolling,
	ClassEnrollmentReply:   RoleService,
	ClassCommand:           RoleService,
	ClassCancellation:      RoleService,
	ClassResult:            RoleAgent,
	ClassLifecycleEvent:    RoleAgent,
	ClassPresence:          RoleAgent,
}

// SignerOf reports which role signs a class. A class outside the seven has no
// signer, which is the same statement as "no class outside § 7 is signed".
func SignerOf(c Class) (SignerRole, bool) {
	r, ok := signerOf[c]
	return r, ok
}

// SignEnvelope fills field 9. It refuses a class outside the seven before doing
// any cryptographic work: an eighth class is not a signing failure, it is a
// class this protocol does not have.
func SignEnvelope(k *SigningKey, e Envelope) (Envelope, error) {
	if !Known(e.Class) {
		return Envelope{}, UnknownClass
	}
	// Field 9 is excluded from its own input by construction: SignedInput
	// slices before field 9's prefix, so whatever Signature holds now cannot
	// affect what is signed.
	input, ok := e.SignedInput()
	if !ok {
		return Envelope{}, MalformedEnvelope
	}
	sig, err := Sign(k, input)
	if err != nil {
		return Envelope{}, err
	}
	e.Signature = sig
	return e, nil
}

// VerifyEnvelope checks an envelope's signature over its own fields 1 to 8.
func VerifyEnvelope(v *VerifyingKey, e Envelope) error {
	if !Known(e.Class) {
		return UnknownClass
	}
	input, ok := e.SignedInput()
	if !ok {
		return MalformedEnvelope
	}
	return Verify(v, input, e.Signature)
}
