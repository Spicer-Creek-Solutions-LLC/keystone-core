// SPDX-License-Identifier: Apache-2.0

// Package goleakhelper wraps go.uber.org/goleak.VerifyTestMain with
// the project's standard ignore set so each integration-test
// package can opt in with a one-line TestMain:
//
//	//go:build integration
//	package foo_test
//
//	import (
//	    "testing"
//	    goleakhelper "go.keystone-core.io/keystone-core/test/goleak"
//	)
//
//	func TestMain(m *testing.M) { goleakhelper.VerifyTestMain(m) }
//
// Policy: every integration-test package's TestMain runs goleak.
// Documented exceptions live on the goleakgate lint's allowList
// (tools/goleakgate/main.go) with GRADUATE-BY notes.
//
// The shared ignore set covers known third-party long-lived
// goroutines that exist for the lifetime of the test binary and
// can't be cleanly Close()'d from test code. Adding a signature
// requires a comment naming why the goroutine is safe to ignore.
package goleakhelper

import (
	"testing"

	"go.uber.org/goleak"
)

// VerifyTestMain wraps goleak.VerifyTestMain with the project-wide
// ignore set. Pass extra package-specific ignores via the variadic
// option list — they're appended to the base set.
func VerifyTestMain(m *testing.M, extra ...goleak.Option) {
	goleak.VerifyTestMain(m, append(baseIgnores(), extra...)...)
}

// baseIgnores returns the shared ignore set. Defined as a function
// so callers can compose it via goleak.VerifyNone(t, baseIgnores()...).
func baseIgnores() []goleak.Option {
	return []goleak.Option{
		// modernc.org/sqlite spins a long-lived background goroutine
		// inside its connection pool that we cannot Close() without
		// modifying the driver. Same posture as
		// pkg/api/server/server_test.go:33.
		goleak.IgnoreTopFunction("modernc.org/sqlite.(*conn).run"),
		goleak.IgnoreAnyFunction("modernc.org/sqlite.(*conn).run"),

		// nats.go's Conn launches per-connection background goroutines
		// (flusher, readLoop, waitForMsgs) that survive briefly past
		// Close(). Documented in cmd/kscore-server/integration_test.go:290.
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).flusher"),
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).readLoop"),
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).waitForMsgs"),
	}
}

// VerifyNoneOptions returns the shared ignore set for use with
// goleak.VerifyNone(t, ...) at per-test scope. Provided so per-test
// `defer goleak.VerifyNone(t, goleakhelper.VerifyNoneOptions()...)`
// stays in sync with the TestMain-wide set.
func VerifyNoneOptions(extra ...goleak.Option) []goleak.Option {
	return append(baseIgnores(), extra...)
}
