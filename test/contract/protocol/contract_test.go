//go:build contract

package protocol_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
				// Field 4 is legitimate on a class with no encryption
				// recipient, and must survive the round trip. On an encrypted
				// class it must be empty -- ADR-0005 § 1 and § 3 -- which the
				// package tests assert by refusal.
				out.CorrelationID = "corr-01hx9yq"
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

			if back.CorrelationID != signed.CorrelationID {
				t.Errorf("correlation identifier = %q, want %q", back.CorrelationID, signed.CorrelationID)
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
	k := signingKey(t)
	signed, err := protocol.SignEnvelope(k, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	headers := protocol.Headers(signed, "msg-1")

	// Must TOLERATE an unknown header. A receiver that refused one could not be
	// sent a header it does not yet use, which is what forward compatibility is.
	withUnknown := map[string]string{"Keystone-Future-Thing": "whatever"}
	for k2, v := range headers {
		withUnknown[k2] = v
	}
	if err := protocol.CheckHeaders(signed, withUnknown); err != nil {
		t.Errorf("an unknown header was refused: %v", err)
	}

	// Must REFUSE a known header that disagrees with the signed field (§ 7 as
	// amended at G38).
	for name, bad := range map[string]string{
		protocol.HeaderClass:   string(protocol.ClassPresence),
		protocol.HeaderVersion: "99",
		protocol.HeaderJobID:   "job-other",
	} {
		bent := map[string]string{}
		for k2, v := range headers {
			bent[k2] = v
		}
		bent[name] = bad
		if err := protocol.CheckHeaders(signed, bent); !is(err, protocol.MalformedEnvelope) {
			t.Errorf("a disagreeing %s gave %v, want malformed envelope", name, err)
		}
	}

	// Trailing bytes WITHIN a field's declared length are tolerated where the
	// signature still verifies -- G36's clarification of § 9. Bytes after field
	// 9 are not, and are refused as framing.
	wire, ok := signed.Encode()
	if !ok {
		t.Fatal("Encode refused")
	}
	if _, err := protocol.Decode(append(append([]byte{}, wire...), 0x00), protocol.DefaultMaxEnvelope); !is(err, protocol.MalformedEnvelope) {
		t.Errorf("a trailing byte after field 9 was not refused: %v", err)
	}

	padded := commandEnvelope()
	padded.Payload = append([]byte("argv"), 0x00, 0x00) // extra bytes inside field 8
	signedPadded, err := protocol.SignEnvelope(k, padded)
	if err != nil {
		t.Fatal(err)
	}
	paddedWire, _ := signedPadded.Encode()
	back, err := protocol.Decode(paddedWire, protocol.DefaultMaxEnvelope)
	if err != nil {
		t.Fatalf("bytes inside a field's declared length were refused: %v", err)
	}
	if err := protocol.VerifyEnvelope(k.Verifying(), back); err != nil {
		t.Errorf("a padded field did not verify: %v", err)
	}
}

func TestAC9CrossProcess(t *testing.T) {
	// D-C01-3: a producer binary and a verifier binary, run as SEPARATE OS
	// PROCESSES over stdio. Real address spaces, production serialization, no
	// broker -- the protocol precedes the transport, so C01 pays
	// ARCH-COMM-003's serialization half and C08 pays the rest.
	producer := build(t, "./cmd/keystone-vector-producer")
	verifier := build(t, "./cmd/keystone-vector-verifier")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"encrypted class", []string{"--class", "command"}},
		{"signed-only class", []string{"--class", "presence", "--job", "", "--sender", "agent-7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vector := run(t, producer, nil, tc.args...)
			if len(vector) == 0 {
				t.Fatal("the producer emitted nothing")
			}
			out, err := runAllowFail(t, verifier, vector)
			if err != nil {
				t.Fatalf("the verifier refused a genuine vector: %v\n%s", err, out)
			}
			if strings.TrimSpace(out) != "accepted" {
				t.Errorf("verifier said %q, want accepted", strings.TrimSpace(out))
			}

			// And it must not accept a tampered one. A vector that crossed a
			// process boundary unverified would make this case decoration.
			//
			// The tamper is structural rather than a string match: an earlier
			// draft replaced a literal prefix that the real JSON never
			// contained, and the guard below is what caught it.
			tampered := tamperEnvelope(t, vector)
			if bytes.Equal(tampered, vector) {
				t.Fatal("the tamper did not apply; the fixture is wrong")
			}
			out, err = runAllowFail(t, verifier, tampered)
			if err == nil {
				t.Errorf("the verifier accepted a tampered vector: %q", strings.TrimSpace(out))
			}
			if strings.TrimSpace(out) == "accepted" {
				t.Errorf("the verifier said accepted for a tampered vector")
			}
		})
	}
}

// tamperEnvelope flips one byte inside the vector's envelope, leaving valid
// JSON and a valid hex string, so what the verifier refuses is the envelope and
// not the transport format.
func tamperEnvelope(t *testing.T, vector []byte) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(vector, &v); err != nil {
		t.Fatalf("the producer did not emit JSON: %v", err)
	}
	hexStr, ok := v["envelope_hex"].(string)
	if !ok || len(hexStr) < 2 {
		t.Fatal("the vector carries no envelope_hex")
	}
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("envelope_hex is not hex: %v", err)
	}
	// Inside the signed region, so the signature must reject it.
	raw[len(raw)/2] ^= 0xff
	v["envelope_hex"] = hex.EncodeToString(raw)
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func build(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = filepath.Join("..", "..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func run(t *testing.T, bin string, stdin []byte, args ...string) []byte {
	t.Helper()
	out, err := runAllowFail(t, bin, stdin, args...)
	if err != nil {
		t.Fatalf("%s: %v\n%s", filepath.Base(bin), err, out)
	}
	return []byte(out)
}

func runAllowFail(t *testing.T, bin string, stdin []byte, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAC10CoarseRefusals(t *testing.T) {
	closed := map[string]bool{
		"unknown version": true, "malformed envelope": true,
		"signature invalid": true, "decryption failed": true,
		"replay rejected": true, "payload too large": true, "unknown class": true,
	}

	k := signingKey(t)
	signed, err := protocol.SignEnvelope(k, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := signed.Encode()

	// Inputs chosen so the refusals come from different paths and different
	// depths, because the property is that they cannot be told apart.
	probes := map[string][]byte{
		"empty":              {},
		"one byte":           {0x00},
		"prefix only":        {0x00, 0x00, 0x00, 0x05},
		"truncated envelope": wire[:len(wire)/2],
		"trailing byte":      append(append([]byte{}, wire...), 0x00),
		"huge length claim":  {0xff, 0xff, 0xff, 0xff},
		"flipped mid-field":  flipAt(wire, len(wire)/2),
		"flipped in field 1": flipAt(wire, 4),
	}
	seen := 0
	for name, in := range probes {
		_, err := protocol.Decode(in, protocol.DefaultMaxEnvelope)
		if err == nil {
			continue
		}
		seen++
		var refusal protocol.Refusal
		if !errors.As(err, &refusal) {
			t.Errorf("%s: %T is not one of the seven codes", name, err)
			continue
		}
		text := refusal.Error()
		if !closed[text] {
			t.Errorf("%s: refusal %q is outside § 9's closed set", name, text)
		}
		// No input-derived detail: nothing from the probe may appear in the
		// message, and nothing positional either.
		for _, leak := range []string{"0x", "offset", "byte ", "field ", "index", "expected", "got "} {
			if strings.Contains(strings.ToLower(text), leak) {
				t.Errorf("%s: refusal %q carries input-derived detail (%q)", name, text, leak)
			}
		}
	}
	if seen < len(probes)/2 {
		t.Fatalf("only %d of %d probes were refused; this case would pass by checking little", seen, len(probes))
	}
}

func flipAt(b []byte, i int) []byte {
	out := append([]byte{}, b...)
	out[i] ^= 0xff
	return out
}

func TestAC11IndistinguishableRefusals(t *testing.T) {
	// D-C01-5 names the three pairs. Each must produce IDENTICAL refusals, so a
	// sender cannot learn which key failed, whether an identifier exists, or
	// where parsing stopped.
	k, wrongKey := signingKey(t), signingKey(t)
	signed, err := protocol.SignEnvelope(k, commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}

	// Pair 1: a bad signature from a known key, and from an unknown one.
	corrupt := signed
	corrupt.Signature = flipAt(signed.Signature, 0)
	knownKeyBadSig := protocol.VerifyEnvelope(k.Verifying(), corrupt)
	unknownKey := protocol.VerifyEnvelope(wrongKey.Verifying(), signed)
	same(t, "bad signature from a known key", knownKeyBadSig, "from an unknown key", unknownKey)

	// Pair 2: a truncated ciphertext, and one for the wrong recipient.
	intended, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := protocol.SealEnvelope(intended.Recipient(), commandEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	short := sealed
	short.Payload = sealed.Payload[:len(sealed.Payload)-1]
	truncated := errOf(protocol.OpenEnvelope(intended, short))
	wrongRecipient := errOf(protocol.OpenEnvelope(other, sealed))
	same(t, "truncated ciphertext", truncated, "wrong recipient", wrongRecipient)

	// Pair 3: an envelope malformed AT FIELD 2 and AT FIELD 7. This is about
	// framing, not content: the point is that a receiver never discloses where
	// parsing stopped, so a break early and a break late are one answer.
	atField2 := truncateFieldAt(t, signed, 1)
	atField7 := truncateFieldAt(t, signed, 6)
	e2 := errOf2(protocol.Decode(atField2, protocol.DefaultMaxEnvelope))
	e7 := errOf2(protocol.Decode(atField7, protocol.DefaultMaxEnvelope))
	same(t, "malformed at field 2", e2, "malformed at field 7", e7)
}

func same(t *testing.T, aName string, a error, bName string, b error) {
	t.Helper()
	if a == nil || b == nil {
		t.Fatalf("%s = %v and %s = %v; both must be refused", aName, a, bName, b)
	}
	if a.Error() != b.Error() {
		t.Errorf("%s gave %q but %s gave %q; the pair must be indistinguishable", aName, a, bName, b)
	}
	var ra, rb protocol.Refusal
	if !errors.As(a, &ra) || !errors.As(b, &rb) || ra != rb {
		t.Errorf("%s and %s gave different codes: %v vs %v", aName, bName, ra, rb)
	}
}

func errOf2(_ protocol.Envelope, err error) error { return err }

// truncateFieldAt cuts an envelope short inside field n (zero-based), so the
// framing breaks at that position and nowhere else.
func truncateFieldAt(t *testing.T, e protocol.Envelope, n int) []byte {
	t.Helper()
	wire, ok := e.Encode()
	if !ok {
		t.Fatal("Encode refused")
	}
	offset := 0
	for i := 0; i <= n; i++ {
		if offset+4 > len(wire) {
			t.Fatalf("cannot reach field %d", n+1)
		}
		length := int(binary.BigEndian.Uint32(wire[offset : offset+4]))
		if i == n {
			// Keep the prefix and one byte less than it declares.
			cut := offset + 4 + length - 1
			if length == 0 {
				cut = offset + 3 // break the prefix itself for an empty field
			}
			return wire[:cut]
		}
		offset += 4 + length
	}
	t.Fatalf("field %d not found", n+1)
	return nil
}

func TestAC12GoldenStability(t *testing.T) {
	// The goldens live beside the implementation and cover the FRAMING and the
	// SIGNED INPUT, never whole envelopes: ML-DSA signing is randomized and the
	// KEM draws fresh randomness, so an envelope cannot be frozen. ADR-0005
	// § Consequences records that, and AC-12's wording is about what the
	// encoder determines.
	const golden = "../../../internal/protocol/testdata/golden/framing.json"
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var frozen []struct {
		Name            string `json:"name"`
		EnvelopeHex     string `json:"envelope_hex"`
		SignedInputHex  string `json:"signed_input_hex"`
		ProtocolVersion uint32 `json:"protocol_version"`
	}
	if err := json.Unmarshal(b, &frozen); err != nil {
		t.Fatal(err)
	}
	if len(frozen) == 0 {
		t.Fatal("the golden file is empty; this case would pass by checking nothing")
	}

	// Every frozen case names the version its bytes belong to, and every one of
	// them must be the version this build speaks. A golden from another version
	// is not a regression, it is a different protocol -- which is what "without
	// a protocol version bump" distinguishes.
	classesSeen := map[string]bool{}
	for _, c := range frozen {
		if c.ProtocolVersion != protocol.Version {
			t.Errorf("%s is frozen at version %d; this build speaks %d", c.Name, c.ProtocolVersion, protocol.Version)
		}
		if c.EnvelopeHex == "" || c.SignedInputHex == "" {
			t.Errorf("%s has an empty golden", c.Name)
		}
		// The signed input must be a prefix of the envelope: fields 1 to 8
		// framed are the envelope's leading bytes by construction.
		if !strings.HasPrefix(c.EnvelopeHex, c.SignedInputHex) {
			t.Errorf("%s: the signed input is not a prefix of the envelope", c.Name)
		}
		classesSeen[c.Name] = true
	}

	// A golden guards only the shapes someone thought to freeze, so the set has
	// to be checked too. Review of #353 found a defect that no case could see
	// because the fixture never held the input.
	for _, class := range []protocol.Class{
		protocol.ClassEnrollmentRequest, protocol.ClassEnrollmentReply,
		protocol.ClassCommand, protocol.ClassCancellation, protocol.ClassResult,
		protocol.ClassLifecycleEvent, protocol.ClassPresence,
	} {
		if !classesSeen["class-"+string(class)] {
			t.Errorf("no golden covers %s", class)
		}
	}
	for _, shape := range []string{"minimal-identifiers", "maximum-identifier"} {
		if !classesSeen[shape] {
			t.Errorf("no golden covers %s", shape)
		}
	}
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
