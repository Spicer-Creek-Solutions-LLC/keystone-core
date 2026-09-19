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
	pending(t, "AC-1")
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
	pending(t, "AC-3")
}

func TestAC4WrongSignatureKey(t *testing.T) {
	pending(t, "AC-4")
}

func TestAC5SignedClasses(t *testing.T) {
	pending(t, "AC-5")
}

func TestAC6CiphertextRefusal(t *testing.T) {
	pending(t, "AC-6")
}

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
