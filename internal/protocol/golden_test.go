package protocol

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Goldens cover the FRAMING and the SIGNED INPUT, never whole envelopes.
//
// ADR-0005 § Consequences records why: ML-DSA signing is randomized and the KEM
// draws fresh randomness, so field 8 and field 9 differ every time they are
// produced. Freezing an envelope would freeze a number the next run cannot
// reproduce. What the encoder DETERMINES is what can be frozen, and AC-12's
// wording -- "golden vectors must not change without a protocol version bump" --
// is about exactly that.
const goldenPath = "testdata/golden/framing.json"

type goldenCase struct {
	Name           string `json:"name"`
	EnvelopeHex    string `json:"envelope_hex"`
	SignedInputHex string `json:"signed_input_hex"`
	Version        uint32 `json:"protocol_version"`
}

// goldenEnvelopes is the set frozen, and the shapes it contains are the point.
//
// Review of #353 found a defect that no acceptance case could see because the
// FIXTURE never contained the violating input -- the gap was in what a test did
// not vary, not in what it asserted. A golden has the same exposure: it guards
// only the shapes someone thought to freeze. So this list names every class,
// both identifier extremes, an all-empty envelope and a maximum-length
// identifier, rather than one representative case.
func goldenEnvelopes() []struct {
	name string
	e    Envelope
} {
	base := func() Envelope {
		return Envelope{
			Version:   Version,
			Class:     ClassCommand,
			JobID:     "job-01hx9yq",
			Sender:    SenderCommandPublisher,
			Timestamp: time.UnixMilli(1758240000000).UTC(),
			Nonce:     make([]byte, NonceBytes),
			Payload:   []byte("payload"),
			Signature: make([]byte, SignatureBytes),
		}
	}
	out := []struct {
		name string
		e    Envelope
	}{}
	for _, c := range []Class{
		ClassEnrollmentRequest, ClassEnrollmentReply, ClassCommand,
		ClassCancellation, ClassResult, ClassLifecycleEvent, ClassPresence,
	} {
		e := base()
		e.Class = c
		if !Encrypted(c) {
			e.CorrelationID = "corr-01hx9yq"
		}
		out = append(out, struct {
			name string
			e    Envelope
		}{"class-" + string(c), e})
	}

	empty := base()
	empty.Class = ClassPresence
	empty.JobID, empty.CorrelationID, empty.Payload = "", "", nil
	empty.Sender = "a"
	out = append(out, struct {
		name string
		e    Envelope
	}{"minimal-identifiers", empty})

	long := base()
	long.Class = ClassPresence
	long.JobID = ""
	long.CorrelationID = ""
	long.Sender = string(make([]byte, 0, MaxIdentifier))
	for i := 0; i < MaxIdentifier; i++ {
		long.Sender += "a"
	}
	out = append(out, struct {
		name string
		e    Envelope
	}{"maximum-identifier", long})

	return out
}

func computeGoldens(t *testing.T) []goldenCase {
	t.Helper()
	var cases []goldenCase
	for _, g := range goldenEnvelopes() {
		wire, ok := g.e.Encode()
		if !ok {
			t.Fatalf("%s: Encode refused", g.name)
		}
		signed, ok := g.e.SignedInput()
		if !ok {
			t.Fatalf("%s: SignedInput refused", g.name)
		}
		cases = append(cases, goldenCase{
			Name:           g.name,
			EnvelopeHex:    hex.EncodeToString(wire),
			SignedInputHex: hex.EncodeToString(signed),
			Version:        Version,
		})
	}
	return cases
}

// TestGoldensAreStable is AC-12's package-level half. Regenerate deliberately
// with -update when a PROTOCOL VERSION BUMP makes the bytes legitimately
// different, and never to make a red test green.
var update = os.Getenv("KEYSTONE_UPDATE_GOLDEN") != ""

func TestGoldensAreStable(t *testing.T) {
	got := computeGoldens(t)
	if update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("goldens rewritten; this must accompany a version bump, not a red test")
		return
	}

	b, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want []goldenCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("the golden file is empty; this test would pass by checking nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("%d cases computed, %d frozen; a case was added or removed without a version bump", len(got), len(want))
	}
	for i := range got {
		if got[i].Version != want[i].Version {
			t.Errorf("%s: protocol version %d, frozen at %d", got[i].Name, got[i].Version, want[i].Version)
		}
		if got[i].Name != want[i].Name {
			t.Errorf("case %d is %q, frozen as %q", i, got[i].Name, want[i].Name)
		}
		if got[i].EnvelopeHex != want[i].EnvelopeHex {
			t.Errorf("%s: framing changed without a version bump", got[i].Name)
		}
		if got[i].SignedInputHex != want[i].SignedInputHex {
			t.Errorf("%s: signed input changed without a version bump", got[i].Name)
		}
	}
}
