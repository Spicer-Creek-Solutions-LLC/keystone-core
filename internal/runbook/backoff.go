// SPDX-License-Identifier: Apache-2.0

package runbook

import "time"

const (
	defaultBackoffBase = 100 * time.Millisecond
	defaultBackoffCap  = 30 * time.Second
)

// expBackoff returns the delay before retry attempt n (n starts at 0
// for the delay after the first failed try): base * 2^n, clamped to
// cap. No jitter — deterministic for tests; the cap bounds worst case.
func expBackoff(n int, base, cap time.Duration) time.Duration {
	if base <= 0 {
		base = defaultBackoffBase
	}
	if cap <= 0 {
		cap = defaultBackoffCap
	}
	d := base
	for i := 0; i < n; i++ {
		d *= 2
		if d >= cap {
			return cap
		}
	}
	if d > cap {
		return cap
	}
	return d
}
