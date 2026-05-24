// SPDX-License-Identifier: Apache-2.0

//go:build integration

package ha

import (
	"testing"

	goleakhelper "go.keystone-core.io/keystone-core/test/goleak"
)

// TestMain wraps every integration test in this package with goleak
// verification per docs/project/TEST-POLICY.md. Build-tagged so it
// doesn't apply to the //go:build slo SLO tests, which need
// wall-clock accuracy that goleak's per-test scan would perturb.
func TestMain(m *testing.M) {
	goleakhelper.VerifyTestMain(m)
}
