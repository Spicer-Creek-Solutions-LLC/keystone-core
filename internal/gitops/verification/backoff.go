// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"time"
)

const (
	defaultBackoffBase = 100 * time.Millisecond
	defaultBackoffCap  = 30 * time.Second
)

// expBackoff returns the delay before retry attempt n (n starts at 0
// for the delay after the first failed try): base * 2^n, clamped to
// cap. No jitter — deterministic for tests; the cap bounds worst
// case. Local copy of the runbook engine's backoff (per-domain
// duplication — this package does not import internal/runbook).
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

// ctxSleep waits d or until ctx is done, whichever first. It returns
// ctx.Err() when the wait was interrupted, nil when d elapsed. A
// non-positive d returns immediately (still honouring an
// already-cancelled ctx).
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
