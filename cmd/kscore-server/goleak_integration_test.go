//go:build integration

package main

import (
	"testing"

	goleakhelper "go.keystone-core.io/keystone-core/test/goleak"
)

// TestMain wraps every integration test in this package with goleak
// verification per docs/project/TEST-POLICY.md. The existing
// per-test `defer goleak.VerifyNone(t, …)` in integration_test.go
// stays — TestMain catches cumulative leaks across the test binary,
// per-test catches them at the test boundary. Both are correct.
func TestMain(m *testing.M) {
	goleakhelper.VerifyTestMain(m)
}
