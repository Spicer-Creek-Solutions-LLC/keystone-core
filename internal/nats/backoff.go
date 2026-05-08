package nats

import (
	"math/rand/v2"
	"time"
)

// reconnectDelay computes the wait between reconnect attempts.
// Standard exponential backoff with symmetric jitter:
//
//	exp   = base * 2^(attempts-1)
//	delay = min(exp, max)
//	delay = delay + delay * jitter * rand_in[-1, 1]
//
// Reconnect storms (Epic 06 risk: 1000 agents lock-stepping into a
// flapping CP) are mitigated by the jitter spread — fleets fan out
// across the backoff window instead of marching in unison.
//
// AWS decorrelated jitter is the gold standard at >500-agent scale;
// symmetric jitter is adequate for v1.0 trial fleets and tracked in
// docs/project/V1X-BACKLOG.md for future optimization.
//
// rng is injected so tests can drive deterministic sequences;
// production callers pass a real *rand.Rand. nil rng returns the
// un-jittered delay (test seam).
//
// Defensive: attempts <= 0 returns base (CustomReconnectDelay
// receives attempts as 1-indexed, but we don't trust it).
func reconnectDelay(attempts int, base, max time.Duration, jitter float64, rng *rand.Rand) time.Duration {
	if attempts <= 0 {
		return base
	}
	// Compute the un-capped exponential. Guard against int64
	// overflow on absurdly large attempts (after ~62 doublings of
	// 1ns the multiply wraps to a negative duration).
	exp := base
	for i := 1; i < attempts && exp < max; i++ {
		next := exp * 2
		if next < exp { // overflow
			exp = max
			break
		}
		exp = next
	}
	if exp > max {
		exp = max
	}

	if rng == nil || jitter == 0 {
		return exp
	}
	// rand.Float64() ∈ [0, 1). Map to [-1, 1) via 2x-1.
	spread := (rng.Float64()*2 - 1) * jitter
	jittered := time.Duration(float64(exp) * (1 + spread))
	// Pin negative outputs (jitter < -1 is invalid input but
	// guard anyway; jitter < 1 + small float drift could
	// theoretically push below zero).
	if jittered < 0 {
		return 0
	}
	return jittered
}
