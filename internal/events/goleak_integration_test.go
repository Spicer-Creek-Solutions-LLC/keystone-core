// SPDX-License-Identifier: Apache-2.0

//go:build integration

package events_test

import (
	"testing"

	goleakhelper "go.keystone-core.io/keystone-core/test/goleak"
)

// TestMain wraps every integration test in this package with goleak
// verification per docs/project/TEST-POLICY.md.
func TestMain(m *testing.M) {
	goleakhelper.VerifyTestMain(m)
}
