// Generation 2's root module. Introduced by P11a, the first task since R08 to
// land product code; AGENTS.md § 2 and the epic record what that changes.
//
// The Go version is not duplicated: CI reads it from a go.mod rather than a
// literal, and tools/capcheck declares the same one. A second copy is a second
// thing to drift.
module go.keystone-core.io/keystone-core

go 1.27
