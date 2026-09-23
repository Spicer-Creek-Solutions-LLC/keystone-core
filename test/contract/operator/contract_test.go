//go:build contract

package operatorcontract

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
	for _, requirement := range manifest.Requirements {
		if requirement.Case == id {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatalf("%s is no longer registered as pending; implement the production assertion", id)
	}
	if os.Getenv("KEYSTONE_PENDING_CONTRACT") == "" {
		t.Skip("pending C04-I: " + caseInfo.Requirement)
	}
	t.Fatalf("%s pending: %s", id, caseInfo.Requirement)
}

// TestMain removes containers an earlier run left behind. t.Cleanup does not
// run when the test binary is killed by a timeout or the forge cancels the run,
// so teardown by label happens on entry as well as on exit -- the reasoning the
// Makefile's container-suite comment records for the Docker suite.
func TestMain(m *testing.M) {
	sweep()
	code := m.Run()
	sweep()
	os.Exit(code)
}
