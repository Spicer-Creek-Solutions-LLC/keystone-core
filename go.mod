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
	github.com/nats-io/nats.go v1.54.0
	github.com/nats-io/nkeys v0.4.16
	golang.org/x/sys v0.48.0
	modernc.org/sqlite v1.59.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	// Held above nkeys's own requirement, which is v0.52.0 and carries four
	// known vulnerabilities -- three fixed by v0.56.0, one with no fix, and none
	// reachable from this module, so `make vuln` passes either way and nothing
	// but this line keeps the floor.
	//
	// The floor was v0.56.0 at C03-I1. nats.go requires v0.57.0, so MVS now
	// picks that and the floor is met by a dependency rather than by this line.
	// The line stays: the day nats.go relaxes its requirement is the day the
	// floor would silently drop.
	golang.org/x/crypto v0.57.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
