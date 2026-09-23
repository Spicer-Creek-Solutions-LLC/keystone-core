//go:build contract

package enrollmentcontract

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
	info, ok := acceptanceContractSurface[id]
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
	for _, requirement := range manifest.Requirements {
		if requirement.Case == id {
			if os.Getenv("KEYSTONE_PENDING_CONTRACT") == "" {
				t.Skip("pending C05-I: " + info.Requirement)
			}
			t.Fatalf("%s pending: %s", id, info.Requirement)
		}
	}
	t.Fatalf("%s is no longer registered as pending; implement the production assertion", id)
}
