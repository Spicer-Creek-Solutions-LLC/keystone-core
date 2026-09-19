//go:build contract

package protocol_test

import "testing"

type acceptanceCase struct {
	TestName    string
	Requirement string
}

// This is the part of the acceptance contract that C01-I may not alter. The
// test bodies remain replaceable so the implementation can turn each pending
// case into a production assertion without changing its approved meaning.
var acceptanceContractSurface = map[string]acceptanceCase{
	"AC-1":  {"TestAC1RoundTrip", "an envelope must round-trip through production encode and decode"},
	"AC-2":  {"TestAC2Framing", "production framing must match the hand-written ADR-0005 vectors"},
	"AC-3":  {"TestAC3SignedFields", "flipping any byte in fields 1 through 8 must be refused"},
	"AC-4":  {"TestAC4WrongSignatureKey", "a signature from the wrong key must be refused"},
	"AC-5":  {"TestAC5SignedClasses", "exactly the seven ADR-0005 classes must be signed"},
	"AC-6":  {"TestAC6CiphertextRefusal", "truncated and wrong-recipient ciphertext must be refused"},
	"AC-7":  {"TestAC7VersionAndSequenceRefusal", "unknown versions and mismatched known-version sequences must be refused"},
	"AC-8":  {"TestAC8HeaderTolerance", "unknown headers and valid trailing field bytes must follow ADR-0005"},
	"AC-9":  {"TestAC9CrossProcess", "producer and verifier must exchange vectors across OS processes"},
	"AC-10": {"TestAC10CoarseRefusals", "refusals must be the closed seven-code set without input-derived detail"},
	"AC-11": {"TestAC11IndistinguishableRefusals", "the specified probing pairs must return identical refusals"},
	"AC-12": {"TestAC12GoldenStability", "golden vectors must not change without a protocol version bump"},
	"AC-13": {"TestAC13SeedCorpus", "every seed-corpus entry must be safe for the production parser"},
}

// Keep the approved test names compile-time coupled to the immutable surface.
var _ = map[string]func(*testing.T){
	"AC-1":  TestAC1RoundTrip,
	"AC-2":  TestAC2Framing,
	"AC-3":  TestAC3SignedFields,
	"AC-4":  TestAC4WrongSignatureKey,
	"AC-5":  TestAC5SignedClasses,
	"AC-6":  TestAC6CiphertextRefusal,
	"AC-7":  TestAC7VersionAndSequenceRefusal,
	"AC-8":  TestAC8HeaderTolerance,
	"AC-9":  TestAC9CrossProcess,
	"AC-10": TestAC10CoarseRefusals,
	"AC-11": TestAC11IndistinguishableRefusals,
	"AC-12": TestAC12GoldenStability,
	"AC-13": TestAC13SeedCorpus,
}
