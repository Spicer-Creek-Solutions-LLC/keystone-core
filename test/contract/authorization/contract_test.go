//go:build contract

package authorizationcontract

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
		t.Skip("pending C03-I: " + caseInfo.Requirement)
	}
	t.Fatalf("%s pending: %s", id, caseInfo.Requirement)
}
