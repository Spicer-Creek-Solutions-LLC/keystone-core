package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func recipient(t *testing.T) *DecryptionKey {
	t.Helper()
	k, err := GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOverheadAndKeySizeMatchADR0005(t *testing.T) {
	k := recipient(t)
	if got := len(k.Recipient().Bytes()); got != RecipientKeyBytes {
		t.Errorf("recipient key is %d bytes, want %d", got, RecipientKeyBytes)
	}
	pt := []byte("argv")
	sealed, err := Seal(k.Recipient(), pt)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sealed) - len(pt); got != SealOverheadBytes {
		t.Errorf("field 8 adds %d bytes, want %d", got, SealOverheadBytes)
	}
}

func TestSealOpenRoundTrips(t *testing.T) {
	k := recipient(t)
	for _, pt := range [][]byte{nil, {}, []byte("x"), bytes.Repeat([]byte("argv "), 400)} {
		sealed, err := Seal(k.Recipient(), pt)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Open(k, sealed)
		if err != nil {
			t.Fatalf("Open refused its own ciphertext: %v", err)
		}
		if !bytes.Equal(got, pt) && !(len(got) == 0 && len(pt) == 0) {
			t.Errorf("round trip changed the plaintext")
		}
	}
}

// Two sealings of the same plaintext must differ: the ephemeral key and the KEM
// encapsulation both draw randomness, which is also why goldens cover the
// framing and the signed input rather than whole envelopes.
func TestSealIsNotDeterministic(t *testing.T) {
	k := recipient(t)
	a, _ := Seal(k.Recipient(), []byte("same"))
	b, _ := Seal(k.Recipient(), []byte("same"))
	if bytes.Equal(a, b) {
		t.Error("two sealings were byte-identical; the ephemeral key is being reused")
	}
}

// AC-11's second pair, asserted here because the API shape must make it true
// before I4 tries to test it.
func TestTruncatedAndWrongRecipientAreIndistinguishable(t *testing.T) {
	k, other := recipient(t), recipient(t)
	sealed, err := Seal(k.Recipient(), []byte("argv"))
	if err != nil {
		t.Fatal(err)
	}

	wrong := errSecond(Open(other, sealed))
	truncated := errSecond(Open(k, sealed[:len(sealed)-1]))

	if wrong == nil || truncated == nil {
		t.Fatal("both must be refused")
	}
	if wrong.Error() != truncated.Error() {
		t.Errorf("refusals differ: %q vs %q", wrong, truncated)
	}
	var a, b Refusal
	if !errors.As(wrong, &a) || !errors.As(truncated, &b) || a != b {
		t.Errorf("refusal codes differ: %v vs %v", a, b)
	}
	if a != DecryptionFailed {
		t.Errorf("refusal = %v, want decryption failed", a)
	}
}

func errSecond(_ []byte, err error) error { return err }

func TestOpenRefusesEveryMalformedShape(t *testing.T) {
	k := recipient(t)
	sealed, _ := Seal(k.Recipient(), []byte("argv"))

	cases := map[string][]byte{
		"empty":                                 {},
		"shorter than the overhead":             make([]byte, SealOverheadBytes-1),
		"exactly the overhead, no tag material": make([]byte, SealOverheadBytes-1),
		"truncated by one":                      sealed[:len(sealed)-1],
		"flipped tag byte":                      flip(sealed, len(sealed)-1),
		"flipped kem ciphertext":                flip(sealed, X25519PublicBytes+5),
		"flipped ephemeral key":                 flip(sealed, 3),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Open(k, in); !errors.Is(err, error(DecryptionFailed)) {
				t.Errorf("err = %v, want decryption failed", err)
			}
		})
	}
}

func flip(b []byte, i int) []byte {
	out := append([]byte{}, b...)
	out[i] ^= 0xff
	return out
}

func TestParseRecipientKeyRoundTripsAndRefusesJunk(t *testing.T) {
	k := recipient(t)
	b := k.Recipient().Bytes()
	back, err := ParseRecipientKey(b)
	if err != nil {
		t.Fatalf("a genuine key was refused: %v", err)
	}
	if !bytes.Equal(back.Bytes(), b) {
		t.Error("parsed key does not re-marshal to the same bytes")
	}
	for _, n := range []int{0, RecipientKeyBytes - 1, RecipientKeyBytes + 1} {
		if _, err := ParseRecipientKey(make([]byte, n)); err == nil {
			t.Errorf("a %d-byte key was accepted", n)
		}
	}
}

// Only three of the seven classes have an encryption recipient (ADR-0005 § 5).
func TestOnlyThreeClassesAreEncrypted(t *testing.T) {
	want := map[Class]bool{ClassCommand: true, ClassCancellation: true, ClassResult: true}
	for c := range classes {
		if Encrypted(c) != want[c] {
			t.Errorf("Encrypted(%s) = %v, want %v", c, Encrypted(c), want[c])
		}
	}
}

// The transcript binding has no acceptance case and cannot have one.
//
// AC-1 round-trips an envelope, and a hybrid that derives its key from the two
// shared secrets ALONE still round-trips perfectly -- both sides compute the
// same weaker key, so the property is invisible to any test that only asks
// whether a sealed message opens. Demonstrated while writing I3: removing the
// ephemeral key and KEM ciphertext from the KDF input left AC-1 green.
//
// That is D-C01-1's warning about a self-consistent wrong encoding, in the KEM
// rather than the framing. The framing's answer was hand-written vectors; there
// is no equivalent here, because Seal's output is randomized and cannot be
// frozen. So the property is asserted directly instead: change any part of the
// transcript and the derived key must change.
func TestDeriveBindsTheTranscript(t *testing.T) {
	ss1 := bytes.Repeat([]byte{1}, 32)
	ss2 := bytes.Repeat([]byte{2}, 32)
	ephA := bytes.Repeat([]byte{3}, X25519PublicBytes)
	ephB := bytes.Repeat([]byte{4}, X25519PublicBytes)
	ctA := bytes.Repeat([]byte{5}, MLKEMCiphertext)
	ctB := bytes.Repeat([]byte{6}, MLKEMCiphertext)

	base, err := derive(ss1, ss2, ephA, ctA)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name           string
		ss1, ss2, e, c []byte
	}{
		{"a different ephemeral key", ss1, ss2, ephB, ctA},
		{"a different KEM ciphertext", ss1, ss2, ephA, ctB},
		{"a different X25519 secret", ss2, ss2, ephA, ctA},
		{"a different ML-KEM secret", ss1, ss1, ephA, ctA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := derive(tc.ss1, tc.ss2, tc.e, tc.c)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(got, base) {
				t.Error("the derived key did not change; this part of the transcript is not bound")
			}
		})
	}

	// And the two secrets must not be interchangeable: concatenating them is
	// what makes the hybrid a hybrid, so swapping their positions must matter.
	swapped, err := derive(ss2, ss1, ephA, ctA)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(swapped, base) {
		t.Error("swapping the two shared secrets gave the same key")
	}
}

// Review of #353 found this: SealEnvelope sealed only the payload and left
// field 4 alone, so an encrypted command could carry a cleartext correlation
// identifier -- and Decode accepted it.
//
// ADR-0005 § 1 and § 3 require field 4 to be EMPTY on encrypted classes,
// because the correlation identifier is what groups an operator's several
// actions and § 3 exists so an observer cannot link them. § 7's accepted
// leakage for a command names the job identifier in cleartext and not this one,
// so the shape leaked beyond what the class permits. The AC-1 fixture left
// CorrelationID empty, so no acceptance case exercised the violating input.
func TestEncryptedClassesRefuseACleartextCorrelationIdentifier(t *testing.T) {
	rk := recipient(t)
	sk := key(t)

	leaky := valid() // an encrypted class
	leaky.CorrelationID = "corr-secret-1234"

	if _, err := SealEnvelope(rk.Recipient(), leaky); !errors.Is(err, error(MalformedEnvelope)) {
		t.Errorf("SealEnvelope accepted a cleartext correlation identifier: %v", err)
	}
	if _, err := OpenEnvelope(rk, leaky); !errors.Is(err, error(MalformedEnvelope)) {
		t.Errorf("OpenEnvelope accepted one: %v", err)
	}

	// And a receiver refuses the wire form, whatever a sender did.
	signed, err := SignEnvelope(sk, leaky)
	if err != nil {
		t.Fatal(err)
	}
	wire, ok := signed.Encode()
	if !ok {
		t.Fatal("Encode refused")
	}
	if !bytes.Contains(wire, []byte("corr-secret-1234")) {
		t.Fatal("fixture is wrong: the identifier should be in the bytes to make the point")
	}
	if _, err := Decode(wire, DefaultMaxEnvelope); !errors.Is(err, error(MalformedEnvelope)) {
		t.Errorf("Decode accepted an encrypted envelope with a non-empty field 4: %v", err)
	}
}

// The field is legitimate on the four classes that are not encrypted, and the
// fix must not have taken that away.
func TestUnencryptedClassesStillCarryACorrelationIdentifier(t *testing.T) {
	e := valid()
	e.Class = ClassPresence
	e.JobID, e.Sender = "", "agent-7"
	e.CorrelationID = "corr-1234"
	wire, ok := e.Encode()
	if !ok {
		t.Fatal("Encode refused")
	}
	back, err := Decode(wire, DefaultMaxEnvelope)
	if err != nil {
		t.Fatalf("Decode refused a signed-only class carrying a correlation identifier: %v", err)
	}
	if back.CorrelationID != "corr-1234" {
		t.Errorf("CorrelationID = %q, want it preserved", back.CorrelationID)
	}
}
