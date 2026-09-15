// Package cli holds what every Generation 2 binary shares: the exit codes the
// charter defines, and the rule that none may be invented.
//
// P11a lands no journey verb. ADR-0010's D-P11-4 records why: a stub refusing a
// valid command has no honest code — the charter's Local is "bad arguments,
// unreadable configuration", which a working binary declining to act is not —
// and there is no "not implemented" value to use instead. Adding one would amend
// the charter.
package cli

// Code is a control-plane outcome. PRODUCT-CHARTER.md § Exit codes is
// normative; these are that table and nothing else.
type Code int

const (
	// OK — the operation succeeded and its outcome is known.
	OK Code = 0
	// Local — a local or usage error: bad arguments, unreadable configuration.
	Local Code = 1
	// Unauthorized — authorization denied. ADR-0009 § 5 returns it both for a
	// connection the kernel refused and for a peer the in-band check denied.
	Unauthorized Code = 10
	// NotDelivered — the target agent did not take delivery within the deadline.
	NotDelivered Code = 11
	// DeadlineExceeded — the job exceeded its deadline.
	DeadlineExceeded Code = 12
	// Unknown — the control plane cannot prove whether execution occurred.
	Unknown Code = 13
	// Cancelled — the job was cancelled.
	Cancelled Code = 14
)

// All is every code the charter defines, so a test can compare this set against
// the charter rather than against a list someone typed twice.
func All() []Code {
	return []Code{OK, Local, Unauthorized, NotDelivered, DeadlineExceeded, Unknown, Cancelled}
}
