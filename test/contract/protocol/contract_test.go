//go:build contract

package protocol_test

import (
	"encoding/json"
	"os"
	"testing"
)

type pendingManifest struct {
	Requirements []struct {
		Case string `json:"case"`
	} `json:"requirements"`
}

// These named tests are the immutable acceptance surface. C01-I supplies the
// production protocol and removes only its entries from the pending manifest;
// it must not alter these case names or their scope.
func pending(t *testing.T, id, requirement string) {
	t.Helper()
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
		t.Skip("pending C01-I: " + requirement)
	}
	t.Fatalf("%s pending: %s", id, requirement)
}

func TestAC1RoundTrip(t *testing.T) {
	pending(t, "AC-1", "an envelope must round-trip through production encode and decode")
}

func TestAC2Framing(t *testing.T) {
	pending(t, "AC-2", "production framing must match the hand-written ADR-0005 vectors")
}

func TestAC3SignedFields(t *testing.T) {
	pending(t, "AC-3", "flipping any byte in fields 1 through 8 must be refused")
}

func TestAC4WrongSignatureKey(t *testing.T) {
	pending(t, "AC-4", "a signature from the wrong key must be refused")
}

func TestAC5SignedClasses(t *testing.T) {
	pending(t, "AC-5", "exactly the seven ADR-0005 classes must be signed")
}

func TestAC6CiphertextRefusal(t *testing.T) {
	pending(t, "AC-6", "truncated and wrong-recipient ciphertext must be refused")
}

func TestAC7VersionAndSequenceRefusal(t *testing.T) {
	pending(t, "AC-7", "unknown versions and mismatched known-version sequences must be refused")
}

func TestAC8HeaderTolerance(t *testing.T) {
	pending(t, "AC-8", "unknown headers and valid trailing field bytes must follow ADR-0005")
}

func TestAC9CrossProcess(t *testing.T) {
	pending(t, "AC-9", "producer and verifier must exchange vectors across OS processes")
}

func TestAC10CoarseRefusals(t *testing.T) {
	pending(t, "AC-10", "refusals must be the closed seven-code set without input-derived detail")
}

func TestAC11IndistinguishableRefusals(t *testing.T) {
	pending(t, "AC-11", "the specified probing pairs must return identical refusals")
}

func TestAC12GoldenStability(t *testing.T) {
	pending(t, "AC-12", "golden vectors must not change without a protocol version bump")
}

func TestAC13SeedCorpus(t *testing.T) {
	pending(t, "AC-13", "every seed-corpus entry must be safe for the production parser")
}
