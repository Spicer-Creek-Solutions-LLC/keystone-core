//go:build contract

package protocol_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

type pendingManifest struct {
	Requirements []struct {
		Case string `json:"case"`
	} `json:"requirements"`
}

// These named tests are the immutable acceptance surface. C01-I supplies the
// production protocol and removes only its entries from the pending manifest;
// it must not alter these case names or their scope.
func pending(t *testing.T, id string) {
	t.Helper()
	caseInfo, ok := acceptanceContractSurface[id]
	if !ok {
		t.Fatalf("%s is not an accepted contract case", id)
	}
	b, err := os.ReadFile("pending-requirements.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest pendingManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	registered := false
	for _, r := range manifest.Requirements {
		if r.Case == id {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatalf("%s is no longer registered as pending; implement the production assertion", id)
	}
	if os.Getenv("KEYSTONE_PENDING_CONTRACT") == "" {
		t.Skip("pending C01-I: " + caseInfo.Requirement)
	}
	t.Fatalf("%s pending: %s", id, caseInfo.Requirement)
}

func TestAC1RoundTrip(t *testing.T) {
	signing := signingKey(t)
	recipientKey, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}

	// A signed-only class and an encrypted one, both through the whole
	// production path: seal where the class has a recipient, sign, encode,
	// decode, verify, open.
	for _, tc := range []struct {
		name    string
		class   protocol.Class
		payload []byte
	}{
		{"signed only", protocol.ClassPresence, []byte("alive")},
		{"encrypted", protocol.ClassCommand, []byte(`{"argv":["systemctl","restart","nginx"]}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := commandEnvelope()
			out.Class = tc.class
			out.Payload = tc.payload
			if tc.class == protocol.ClassPresence {
				out.JobID, out.Sender = "", "agent-7"
			}

			if protocol.Encrypted(tc.class) {
				if out, err = protocol.SealEnvelope(recipientKey.Recipient(), out); err != nil {
					t.Fatalf("seal: %v", err)
				}
			}
			signed, err := protocol.SignEnvelope(signing, out)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			wire, ok := signed.Encode()
			if !ok {
				t.Fatal("encode refused a signed envelope")
			}

			back, err := protocol.Decode(wire, protocol.DefaultMaxEnvelope)
			if err != nil {
				t.Fatalf("decode refused a well-formed envelope: %v", err)
			}
			if err := protocol.VerifyEnvelope(signing.Verifying(), back); err != nil {
				t.Fatalf("verify: %v", err)
			}

			if back.Class != tc.class || back.JobID != signed.JobID ||
				back.Sender != signed.Sender || back.Version != signed.Version ||
				!back.Timestamp.Equal(signed.Timestamp) {
				t.Errorf("round trip changed the envelope's fields")
			}

			plaintext := back.Payload
			if protocol.Encrypted(tc.class) {
				if plaintext, err = protocol.OpenEnvelope(recipientKey, back); err != nil {
					t.Fatalf("open: %v", err)
				}
			}
			if !bytes.Equal(plaintext, tc.payload) {
				t.Errorf("payload = %q, want %q", plaintext, tc.payload)
			}

			// Re-encoding what was decoded must give the same bytes. A parser
			// that accepts an input it cannot reproduce has accepted two
			// encodings of one envelope.
			again, ok := back.Encode()
			if !ok || !bytes.Equal(again, wire) {
				t.Error("decode/encode is not a round trip at the byte level")
			}
		})
	}
}

func TestAC2Framing(t *testing.T) {
	b, err := os.ReadFile("framing-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file framingVectorFile
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Vectors) == 0 {
		t.Fatal("no vectors; AC-2 would pass by checking nothing")
	}
	for _, vector := range file.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			fields := make([][]byte, 0, len(vector.FieldValues))
			for i, raw := range vector.FieldValues {
				value, err := hex.DecodeString(raw)
				if err != nil {
					t.Fatalf("field %d: %v", i+1, err)
				}
				fields = append(fields, value)
			}
			got, ok := protocol.Frame(fields)
			if !ok {
				t.Fatal("production framing refused a pinned vector")
			}
			if hex.EncodeToString(got) != vector.Framed {
				t.Errorf("production framing = %x, want %s", got, vector.Framed)
			}
		})
	}
}

func TestAC3SignedFields(t *testing.T) {
	k := signingKey(t)
	signed, err := protocol.SignEnvelope(k, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	wire, ok := signed.Encode()
	if !ok {
		t.Fatal("Encode refused a signed envelope")
	}
	input, ok := signed.SignedInput()
	if !ok {
		t.Fatal("SignedInput refused a signed envelope")
	}
	// Fields 1 to 8 framed are exactly the leading bytes of the envelope, so
	// every offset below is a byte of those fields -- values AND the length
	// prefixes, which ADR-0005 § 1 puts inside the signed input so a field
	// boundary cannot be moved under a valid signature.
	if len(input) == 0 || len(input) >= len(wire) {
		t.Fatalf("signed input is %d bytes of a %d-byte envelope; the fixture is wrong", len(input), len(wire))
	}

	// The flip goes through the WIRE and back, not into the signed input
	// directly. Verifying a mutated input against its own signature would only
	// show that Verify notices a changed message; it would pass even if
	// SignedInput omitted a region entirely. Decoding the mutated envelope and
	// verifying it is what asserts the region is covered at all.
	for i := 0; i < len(input); i++ {
		bent := append([]byte{}, wire...)
		bent[i] ^= 0x01

		envelope, err := protocol.Decode(bent, protocol.DefaultMaxEnvelope)
		if err != nil {
			continue // refused before verification, which is still refused
		}
		if err := protocol.VerifyEnvelope(k.Verifying(), envelope); err == nil {
			t.Fatalf("a flipped byte at offset %d of fields 1-8 was accepted", i)
		}
	}
}

func TestAC4WrongSignatureKey(t *testing.T) {
	k, wrong := signingKey(t), signingKey(t)
	signed, err := protocol.SignEnvelope(k, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.VerifyEnvelope(k.Verifying(), signed); err != nil {
		t.Fatalf("the genuine key must verify: %v", err)
	}
	if err := protocol.VerifyEnvelope(wrong.Verifying(), signed); !is(err, protocol.SignatureInvalid) {
		t.Errorf("a signature from the wrong key gave %v, want signature invalid", err)
	}

	// Half a hybrid signature is not a signature. Splicing a genuine Ed25519
	// half onto a foreign ML-DSA half must fail, or the envelope has the
	// security of whichever primitive an attacker prefers.
	foreign, err := protocol.SignEnvelope(wrong, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	spliced := signed
	spliced.Signature = append(append([]byte{}, signed.Signature[:protocol.Ed25519SignatureBytes]...),
		foreign.Signature[protocol.Ed25519SignatureBytes:]...)
	if err := protocol.VerifyEnvelope(k.Verifying(), spliced); !is(err, protocol.SignatureInvalid) {
		t.Errorf("a spliced hybrid signature gave %v, want signature invalid", err)
	}
}

func TestAC5SignedClasses(t *testing.T) {
	seven := []protocol.Class{
		protocol.ClassEnrollmentRequest, protocol.ClassEnrollmentReply,
		protocol.ClassCommand, protocol.ClassCancellation, protocol.ClassResult,
		protocol.ClassLifecycleEvent, protocol.ClassPresence,
	}
	k := signingKey(t)

	// Every one of the seven signs and verifies, and each names the role
	// ADR-0005 § 7's Signer row assigns it.
	for _, class := range seven {
		t.Run(string(class), func(t *testing.T) {
			if _, ok := protocol.SignerOf(class); !ok {
				t.Fatalf("%s has no signer role", class)
			}
			e := commandEnvelope()
			e.Class = class
			signed, err := protocol.SignEnvelope(k, e)
			if err != nil {
				t.Fatalf("signing %s: %v", class, err)
			}
			if err := protocol.VerifyEnvelope(k.Verifying(), signed); err != nil {
				t.Errorf("%s did not verify: %v", class, err)
			}
		})
	}

	// "Exactly" the seven: nothing outside the set is signed, and it is refused
	// as a class rather than as a signing failure.
	for _, class := range []protocol.Class{"", "telemetry", "COMMAND", "command "} {
		e := commandEnvelope()
		e.Class = class
		if _, err := protocol.SignEnvelope(k, e); !is(err, protocol.UnknownClass) {
			t.Errorf("signing %q gave %v, want unknown class", class, err)
		}
		if _, ok := protocol.SignerOf(class); ok {
			t.Errorf("%q has a signer role and is not one of the seven", class)
		}
	}
}

// Keys are generated per test rather than checked in. Committing private key
// material, even for tests, is a habit worth not starting.
func signingKey(t *testing.T) *protocol.SigningKey {
	t.Helper()
	k, err := protocol.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func commandEnvelope() protocol.Envelope {
	return protocol.Envelope{
		Version:   protocol.Version,
		Class:     protocol.ClassCommand,
		JobID:     "job-01hx9yq",
		Sender:    protocol.SenderCommandPublisher,
		Timestamp: time.UnixMilli(1758240000000).UTC(),
		Nonce:     make([]byte, protocol.NonceBytes),
		Payload:   []byte("payload"),
	}
}

func TestAC6CiphertextRefusal(t *testing.T) {
	intended, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}

	e := commandEnvelope()
	e.Payload = []byte(`{"argv":["id"]}`)
	sealed, err := protocol.SealEnvelope(intended.Recipient(), e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.OpenEnvelope(intended, sealed); err != nil {
		t.Fatalf("the intended recipient must open it: %v", err)
	}

	// Wrong recipient.
	wrongRecipient := errOf(protocol.OpenEnvelope(other, sealed))
	if !is(wrongRecipient, protocol.DecryptionFailed) {
		t.Errorf("wrong recipient gave %v, want decryption failed", wrongRecipient)
	}

	// Truncated, at several depths: inside the AEAD output, inside the KEM
	// ciphertext, and inside the ephemeral key.
	for _, cut := range []int{1, protocol.AEADOverheadBytes + 1, protocol.MLKEMCiphertext, len(sealed.Payload) - 4} {
		if cut <= 0 || cut >= len(sealed.Payload) {
			continue
		}
		short := sealed
		short.Payload = sealed.Payload[:len(sealed.Payload)-cut]
		if err := errOf(protocol.OpenEnvelope(intended, short)); !is(err, protocol.DecryptionFailed) {
			t.Errorf("truncation by %d gave %v, want decryption failed", cut, err)
		}
	}

	// AC-11's second pair: the two must be one answer, not two.
	truncated := errOf(protocol.OpenEnvelope(intended, func() protocol.Envelope {
		short := sealed
		short.Payload = sealed.Payload[:len(sealed.Payload)-1]
		return short
	}()))
	if wrongRecipient.Error() != truncated.Error() {
		t.Errorf("wrong-recipient and truncated refusals differ: %q vs %q", wrongRecipient, truncated)
	}
}

func errOf(_ []byte, err error) error { return err }

func TestAC7VersionAndSequenceRefusal(t *testing.T) {
	base := func() [][]byte {
		version := make([]byte, 4)
		binary.BigEndian.PutUint32(version, 1)
		ts := make([]byte, 8)
		binary.BigEndian.PutUint64(ts, 1758240000000)
		return [][]byte{
			version, []byte("command"), []byte("job-1"), {},
			[]byte("command-publisher"), ts, make([]byte, 16), []byte("p"), {},
		}
	}

	unknown := base()
	binary.BigEndian.PutUint32(unknown[0], 99)
	framed, ok := protocol.Frame(unknown)
	if !ok {
		t.Fatal("Frame refused the fixture")
	}
	if _, err := protocol.Decode(framed, protocol.DefaultMaxEnvelope); !is(err, protocol.UnknownVersion) {
		t.Errorf("an unknown version gave %v, want unknown version", err)
	}

	// A KNOWN version whose envelope does not match that version's field
	// sequence. Each of these parses as framing and must still be refused.
	for _, tc := range []struct {
		name string
		bend func([][]byte)
		want protocol.Refusal
	}{
		{"nonce is not 16 bytes", func(f [][]byte) { f[6] = []byte{1} }, protocol.MalformedEnvelope},
		{"timestamp is not 8 bytes", func(f [][]byte) { f[5] = []byte{1} }, protocol.MalformedEnvelope},
		{"version field is not 4 bytes", func(f [][]byte) { f[0] = []byte{1} }, protocol.MalformedEnvelope},
		{"class outside the seven", func(f [][]byte) { f[1] = []byte("telemetry") }, protocol.UnknownClass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := base()
			tc.bend(f)
			framed, ok := protocol.Frame(f)
			if !ok {
				t.Fatal("Frame refused the fixture")
			}
			if _, err := protocol.Decode(framed, protocol.DefaultMaxEnvelope); !is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// is reports whether err is exactly the given refusal. The contract asserts on
// the CODE and never on a message, because ADR-0005 § 9's set is coarse by
// design and a test that matched text would outlive that intent.
func is(err error, want protocol.Refusal) bool {
	var got protocol.Refusal
	return errors.As(err, &got) && got == want
}

func TestAC8HeaderTolerance(t *testing.T) {
	pending(t, "AC-8")
}

func TestAC9CrossProcess(t *testing.T) {
	pending(t, "AC-9")
}

func TestAC10CoarseRefusals(t *testing.T) {
	pending(t, "AC-10")
}

func TestAC11IndistinguishableRefusals(t *testing.T) {
	pending(t, "AC-11")
}

func TestAC12GoldenStability(t *testing.T) {
	pending(t, "AC-12")
}

func TestAC13SeedCorpus(t *testing.T) {
	const dir = "../../../internal/protocol/testdata/fuzz"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			// Safe means three things: it does not panic, it refuses with one
			// of § 9's codes rather than an arbitrary error, and anything it
			// accepts re-encodes to the same bytes.
			envelope, err := protocol.Decode(b, protocol.DefaultMaxEnvelope)
			if err != nil {
				var refusal protocol.Refusal
				if !errors.As(err, &refusal) {
					t.Fatalf("Decode returned %T (%v); every refusal is one of the seven codes", err, err)
				}
				return
			}
			again, ok := envelope.Encode()
			if !ok {
				t.Fatal("an accepted corpus entry failed to re-encode")
			}
			if !bytes.Equal(again, b) {
				t.Errorf("accepted entry does not round-trip:\n in %x\nout %x", b, again)
			}
		})
	}
	if seen == 0 {
		t.Fatal("the seed corpus is empty; this case would pass by checking nothing")
	}
}
