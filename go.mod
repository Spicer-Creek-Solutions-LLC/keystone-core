// Generation 2's root module. Introduced by P11a, the first task since R08 to
// land product code; AGENTS.md § 2 and the epic record what that changes.
//
// The Go version is not duplicated: CI reads it from a go.mod rather than a
// literal, and tools/capcheck declares the same one. A second copy is a second
// thing to drift.
module go.keystone-core.io/keystone-core

go 1.27

require (
	github.com/nats-io/jwt/v2 v2.8.2
	github.com/nats-io/nkeys v0.4.16
	modernc.org/sqlite v1.59.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	// Held at v0.56.0 deliberately. nkeys requires v0.52.0, which carries four
	// known vulnerabilities; three are fixed by v0.56.0 and none is reachable
	// from this module, so `make vuln` passes either way and nothing but this
	// line keeps the floor.
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
