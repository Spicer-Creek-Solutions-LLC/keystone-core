//go:build contract

package persistence_test

import "testing"

type acceptanceCase struct {
	TestName    string
	Requirement string
}

// This is the part of the acceptance contract that C02-I may not alter. The
// test bodies remain replaceable so the implementation can turn each pending
// case into a production assertion without changing its approved meaning.
//
// The shape is C01-A's, deliberately. D-C02-4 says reuse that machinery rather
// than build a second, and G39 and G40 generalised all three gates so a second
// package needs no tooling change at all.
var acceptanceContractSurface = map[string]acceptanceCase{
	"AC-1":  {"TestAC1MigrationOrder", "migrations must run in order and never backwards"},
	"AC-2":  {"TestAC2PartialMigration", "a partially applied migration must leave the store at its last complete step"},
	"AC-3":  {"TestAC3StoresMigrateIndependently", "each store must migrate independently, with no combined version"},
	"AC-4":  {"TestAC4ReceiptAndStartAreSeparateTransactions", "receipt and start must commit in separate transactions"},
	"AC-5":  {"TestAC5CrashBetweenReceiptAndStartIsDistinguishable", "a crash between receipt and start must be distinguishable from a crash before either"},
	"AC-6":  {"TestAC6AcceptedRequestCommitsWithItsIdentifier", "the job identifier and accepted request must commit together before publication"},
	"AC-7":  {"TestAC7TerminalStateCommitsWithItsResult", "the terminal state and result must commit together"},
	"AC-8":  {"TestAC8AcknowledgementIsItsOwnWrite", "the result acknowledgement must be its own write"},
	"AC-9":  {"TestAC9LedgerIsReadableWithoutTheServer", "the agent ledger must be readable without the server at its documented path"},
	"AC-10": {"TestAC10PublishedSchemaMatchesTheShippedOne", "the published schema must match the schema the implementation ships"},
	"AC-11": {"TestAC11OwnershipAndModes", "each store file must carry the mode ADR-0008 fixes"},
	"AC-12": {"TestAC12RetentionFloor", "no record may be removed before its retention floor"},
	"AC-13": {"TestAC13CorruptStoreIsRefused", "a corrupt store must be refused rather than served"},
}

// Keep the approved test names compile-time coupled to the immutable surface.
var _ = map[string]func(*testing.T){
	"AC-1":  TestAC1MigrationOrder,
	"AC-2":  TestAC2PartialMigration,
	"AC-3":  TestAC3StoresMigrateIndependently,
	"AC-4":  TestAC4ReceiptAndStartAreSeparateTransactions,
	"AC-5":  TestAC5CrashBetweenReceiptAndStartIsDistinguishable,
	"AC-6":  TestAC6AcceptedRequestCommitsWithItsIdentifier,
	"AC-7":  TestAC7TerminalStateCommitsWithItsResult,
	"AC-8":  TestAC8AcknowledgementIsItsOwnWrite,
	"AC-9":  TestAC9LedgerIsReadableWithoutTheServer,
	"AC-10": TestAC10PublishedSchemaMatchesTheShippedOne,
	"AC-11": TestAC11OwnershipAndModes,
	"AC-12": TestAC12RetentionFloor,
	"AC-13": TestAC13CorruptStoreIsRefused,
}
