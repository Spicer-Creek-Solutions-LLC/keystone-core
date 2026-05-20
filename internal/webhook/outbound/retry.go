package outbound

import (
	"context"
	"time"
)

// RetryPolicy tunes the §4.14 retry behavior shared across all
// subscriptions. The per-attempt budget is per-subscription
// ([Subscription.MaxRetries]); this carries only the
// backoff / jitter shape.
type RetryPolicy struct {
	// BaseBackoff is the delay before retry attempt #1. 0 →
	// [DefaultRetryBaseBackoff] (1s — matches the §4.14
	// `webhook.outbound.retry_backoff` default).
	BaseBackoff time.Duration
	// MaxBackoff clamps the exponentially-growing delay. 0 →
	// [DefaultRetryMaxBackoff] (30s).
	MaxBackoff time.Duration
	// Jitter is the additive randomization fraction
	// (delay += rng() * delay * Jitter). 0 disables jitter. Values
	// over ~0.5 produce wide tails — 0.1..0.3 is the sweet spot.
	Jitter float64
}

// Default RetryPolicy values applied when the field is zero.
const (
	DefaultRetryBaseBackoff = 1 * time.Second
	DefaultRetryMaxBackoff  = 30 * time.Second
)

// jitteredBackoff returns the delay before retry attempt n (n starts
// at 0 for the delay after the first failed try):
// clamp(base * 2^n, cap), plus an additive random kick in
// [0, delay*Jitter]. rng returns a value in [0, 1). The rng seam
// makes tests deterministic.
func jitteredBackoff(p RetryPolicy, n int, rng func() float64) time.Duration {
	base := p.BaseBackoff
	if base <= 0 {
		base = DefaultRetryBaseBackoff
	}
	capD := p.MaxBackoff
	if capD <= 0 {
		capD = DefaultRetryMaxBackoff
	}
	d := base
	for i := 0; i < n; i++ {
		d *= 2
		if d >= capD {
			d = capD
			break
		}
	}
	if d > capD {
		d = capD
	}
	if p.Jitter > 0 && rng != nil {
		r := rng()
		if r < 0 {
			r = 0
		}
		if r >= 1 {
			r = 0.999_999
		}
		d += time.Duration(float64(d) * p.Jitter * r)
	}
	return d
}

// ctxSleep waits d or until ctx is done, whichever first. nil error
// when d elapsed; ctx.Err() when interrupted. Non-positive d returns
// immediately (still honouring an already-cancelled ctx). Local copy
// of the runbook / verification pair — per-domain duplication is the
// established convention.
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
