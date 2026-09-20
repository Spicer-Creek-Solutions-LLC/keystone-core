//go:build contract

package persistence_test

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

// pending is C01-A's, unchanged: a case skips under ordinary execution and
// fails under the gate, and it refuses to skip at all once its manifest entry
// is gone.
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
		t.Skip("pending C02-I: " + caseInfo.Requirement)
	}
	t.Fatalf("%s pending: %s", id, caseInfo.Requirement)
}

func TestAC1MigrationOrder(t *testing.T)                               { pending(t, "AC-1") }
func TestAC2PartialMigration(t *testing.T)                             { pending(t, "AC-2") }
func TestAC3StoresMigrateIndependently(t *testing.T)                   { pending(t, "AC-3") }
func TestAC4ReceiptAndStartAreSeparateTransactions(t *testing.T)       { pending(t, "AC-4") }
func TestAC5CrashBetweenReceiptAndStartIsDistinguishable(t *testing.T) { pending(t, "AC-5") }
func TestAC6AcceptedRequestCommitsWithItsIdentifier(t *testing.T)      { pending(t, "AC-6") }
func TestAC7TerminalStateCommitsWithItsResult(t *testing.T)            { pending(t, "AC-7") }
func TestAC8AcknowledgementIsItsOwnWrite(t *testing.T)                 { pending(t, "AC-8") }
func TestAC9LedgerIsReadableWithoutTheServer(t *testing.T)             { pending(t, "AC-9") }
func TestAC10PublishedSchemaMatchesTheShippedOne(t *testing.T)         { pending(t, "AC-10") }
func TestAC11OwnershipAndModes(t *testing.T)                           { pending(t, "AC-11") }
func TestAC12RetentionFloor(t *testing.T)                              { pending(t, "AC-12") }
func TestAC13CorruptStoreIsRefused(t *testing.T)                       { pending(t, "AC-13") }
