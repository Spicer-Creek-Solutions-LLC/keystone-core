// Package version reports the build's identity.
//
// VERSIONING.md fixes the development form as 0.0.0-dev+g<commit>. The commit
// is injected at link time and never written here: a literal would report a
// build that does not exist the moment anything is committed, and nothing would
// say so.
package version

import "runtime/debug"

// Dev is the version stamp VERSIONING.md defines for a development build.
const Dev = "0.0.0-dev"

// commit is set with -ldflags "-X .../internal/version.commit=<sha>". It is
// empty in a `go test` or `go run` build, where no linker flag is passed.
var commit string

// String returns the build's version. It prefers the linker-injected commit and
// falls back to the one the Go toolchain records in the binary, so a `go build`
// without the Makefile still reports something true rather than something
// invented.
func String() string {
	if c := resolve(); c != "" {
		return Dev + "+g" + c
	}
	return Dev
}

// Commit returns the commit this build was made from, or "" when it is unknown.
// Unknown is reported as empty rather than as a placeholder: a caller can act on
// empty, and cannot act on "unknown" without string-matching a sentinel.
func Commit() string { return resolve() }

func resolve() string {
	if commit != "" {
		return commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
