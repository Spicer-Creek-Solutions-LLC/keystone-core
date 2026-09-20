//go:build contract

package persistence_test

// The crash harness, specified here and built by C02-I.
//
// D-C02-3 is why it has to exist. ADR-0008 § 5 requires receipt and start to
// commit SEPARATELY, because if they commit together a crash between them is
// indistinguishable from a crash before either, and ADR-0006's recovery rule
// collapses into Indeterminate.
//
// A schema cannot show that. Two INSERT statements in one transaction look
// identical to two in two transactions AT REST -- same tables, same rows, same
// indexes. The difference appears only when the process stops in between, so
// the harness has to stop a real process at a chosen boundary.
//
// C02-A specifies the interface and C02-I supplies the binary, for the reason
// C01-A could not supply a producer: a harness cannot drive stores that do not
// exist yet. AC-4 and AC-5 stay registered as pending until it does.
//
// # The binary
//
//	cmd/keystone-ledger-crash
//
// # Arguments
//
//	--ledger PATH     where to create or open the agent ledger
//	--stop-after STEP the boundary to halt at, one of the steps below
//	--job ID          the job identifier to write
//
// # Steps
//
//	none        write nothing and exit; the control for "crashed before either"
//	receipt     write the receipt, then halt BEFORE the start
//	start       write the receipt and the start, then halt
//
// # Halting
//
// The binary must terminate WITHOUT unwinding -- no deferred close, no
// graceful shutdown, no flush. A clean exit would let SQLite finish work a
// crash would have lost, which is the whole property under test. os.Exit after
// the chosen write is sufficient; a signal to self is not required.
//
// # Exit status
//
//	0   halted at the requested step
//	1   the step is unknown, or the ledger could not be opened
//
// A test reads the ledger afterwards with the ordinary reader and asks which
// records are present. The three steps must be distinguishable from each
// other by that reading alone: that is AC-5.
const CrashHarnessBinary = "cmd/keystone-ledger-crash"

// CrashSteps are the boundaries the harness accepts, in the order they occur.
var CrashSteps = []string{"none", "receipt", "start"}
