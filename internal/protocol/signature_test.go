package protocol

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
)

func key(t *testing.T) *SigningKey {
	t.Helper()
	k, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSignatureAndKeyAreTheSizesADR0005Fixes(t *testing.T) {
	k := key(t)
	if got := len(k.Verifying().Bytes()); got != VerifyingKeyBytes {
		t.Errorf("verifying key is %d bytes, want %d", got, VerifyingKeyBytes)
	}
	sig, err := Sign(k, []byte("input"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != SignatureBytes {
		t.Errorf("signature is %d bytes, want %d", len(sig), SignatureBytes)
	}
}

func TestVerifyAcceptsAGenuineSignature(t *testing.T) {
	k := key(t)
	input := []byte("the framed bytes of fields 1 to 8")
	sig, err := Sign(k, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(k.Verifying(), input, sig); err != nil {
		t.Fatalf("a genuine signature was refused: %v", err)
	}
}

// Hybrid means conjunction. Each half alone must be insufficient, or the
// envelope has the security of the weaker primitive.
func TestBothHalvesAreRequired(t *testing.T) {
	k, other := key(t), key(t)
	input := []byte("input")
	good, _ := Sign(k, input)
	otherSig, _ := Sign(other, input)

	edOnly := append(append([]byte{}, good[:Ed25519SignatureBytes]...), otherSig[Ed25519SignatureBytes:]...)
	mlOnly := append(append([]byte{}, otherSig[:Ed25519SignatureBytes]...), good[Ed25519SignatureBytes:]...)

	for _, tc := range []struct {
		name string
		sig  []byte
	}{
		{"only Ed25519 is genuine", edOnly},
		{"only ML-DSA is genuine", mlOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(k.Verifying(), input, tc.sig); !errors.Is(err, error(SignatureInvalid)) {
				t.Errorf("err = %v; half a hybrid signature must not verify", err)
			}
		})
	}
}

// AC-11's first pair, asserted here because the API shape has to make it true
// now rather than at I4: a wrong key and a corrupt signature are one answer.
func TestWrongKeyAndCorruptSignatureAreIndistinguishable(t *testing.T) {
	k, wrong := key(t), key(t)
	input := []byte("input")
	sig, _ := Sign(k, input)

	fromWrongKey := Verify(wrong.Verifying(), input, sig)

	corrupt := append([]byte{}, sig...)
	corrupt[0] ^= 0xff
	corrupted := Verify(k.Verifying(), input, corrupt)

	if fromWrongKey == nil || corrupted == nil {
		t.Fatal("both must be refused")
	}
	if fromWrongKey.Error() != corrupted.Error() {
		t.Errorf("refusals differ: %q vs %q", fromWrongKey, corrupted)
	}
	var a, b Refusal
	if !errors.As(fromWrongKey, &a) || !errors.As(corrupted, &b) || a != b {
		t.Errorf("refusal codes differ: %v vs %v", a, b)
	}
}

func TestVerifyRefusesAWrongLengthSignature(t *testing.T) {
	k := key(t)
	for _, n := range []int{0, 63, SignatureBytes - 1, SignatureBytes + 1} {
		if err := Verify(k.Verifying(), []byte("x"), make([]byte, n)); err == nil {
			t.Errorf("a %d-byte signature was accepted", n)
		}
	}
}

func TestParseVerifyingKeyRoundTripsAndRefusesJunk(t *testing.T) {
	k := key(t)
	b := k.Verifying().Bytes()
	back, err := ParseVerifyingKey(b)
	if err != nil {
		t.Fatalf("a genuine key was refused: %v", err)
	}
	if !bytes.Equal(back.Bytes(), b) {
		t.Error("parsed key does not re-marshal to the same bytes")
	}
	for _, n := range []int{0, VerifyingKeyBytes - 1, VerifyingKeyBytes + 1} {
		if _, err := ParseVerifyingKey(make([]byte, n)); err == nil {
			t.Errorf("a %d-byte key was accepted", n)
		}
	}
}

func TestSigningKeyPersistenceRoundTrips(t *testing.T) {
	k := key(t)
	wantPublic := k.Verifying().Bytes()
	b := MarshalSigningKey(k)
	if len(b) != SigningPrivateKeyBytes {
		t.Fatalf("private key is %d bytes, want %d", len(b), SigningPrivateKeyBytes)
	}

	back, err := ParseSigningKey(b)
	if err != nil {
		t.Fatalf("a genuine private key was refused: %v", err)
	}
	if !bytes.Equal(back.Verifying().Bytes(), wantPublic) {
		t.Fatal("restored private key has a different public half")
	}
	if !bytes.Equal(MarshalSigningKey(back), b) {
		t.Fatal("restored private key does not re-marshal identically")
	}

	input := []byte("signed after agent restart")
	sig, err := Sign(back, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(k.Verifying(), input, sig); err != nil {
		t.Fatalf("signature from restored key was refused: %v", err)
	}
}

func TestParseSigningKeyRefusesWrongLengths(t *testing.T) {
	for _, n := range []int{0, ed25519.SeedSize, SigningPrivateKeyBytes - 1, SigningPrivateKeyBytes + 1} {
		if _, err := ParseSigningKey(make([]byte, n)); !errors.Is(err, error(SignatureInvalid)) {
			t.Errorf("a %d-byte private key returned %v, want signature invalid", n, err)
		}
	}
}

// ADR-0005 § 7's Signer row: every one of the seven has a signer, and nothing
// else does. That is the same statement as "no class outside § 7 is signed".
func TestEveryClassHasASignerAndNothingElseDoes(t *testing.T) {
	seven := []Class{
		ClassEnrollmentRequest, ClassEnrollmentReply, ClassCommand,
		ClassCancellation, ClassResult, ClassLifecycleEvent, ClassPresence,
	}
	for _, c := range seven {
		if _, ok := SignerOf(c); !ok {
			t.Errorf("%s has no signer role", c)
		}
	}
	if len(signerOf) != len(seven) {
		t.Errorf("the signer map has %d entries; ADR-0005 § 7 has %d classes", len(signerOf), len(seven))
	}
	for _, c := range []Class{"", "telemetry", "COMMAND", "command "} {
		if _, ok := SignerOf(c); ok {
			t.Errorf("%q has a signer role and is not one of the seven", c)
		}
	}
}

func TestSignEnvelopeRefusesAClassOutsideTheSeven(t *testing.T) {
	e := valid()
	e.Class = "telemetry"
	if _, err := SignEnvelope(key(t), e); !errors.Is(err, error(UnknownClass)) {
		t.Errorf("err = %v, want unknown class", err)
	}
}

// Flipping any byte of fields 1 to 8 must break the signature -- AC-3's shape,
// asserted at the package level as well as through the contract.
func TestAnyFlippedByteInTheSignedInputIsRefused(t *testing.T) {
	k := key(t)
	e, err := SignEnvelope(k, valid())
	if err != nil {
		t.Fatal(err)
	}
	input, ok := e.SignedInput()
	if !ok {
		t.Fatal("SignedInput refused a signed envelope")
	}
	for i := range input {
		bent := append([]byte{}, input...)
		bent[i] ^= 0x01
		if err := Verify(k.Verifying(), bent, e.Signature); err == nil {
			t.Fatalf("a flip at byte %d of the signed input still verified", i)
		}
	}
}
