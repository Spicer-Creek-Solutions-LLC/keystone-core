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
	pending(t, "AC-2")
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
	pending(t, "AC-7")
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
	pending(t, "AC-13")
}
