// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"testing"
	"time"
)

func TestJitteredBackoff_NoJitter_Doubles(t *testing.T) {
	t.Parallel()
	p := RetryPolicy{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 1 * time.Second}
	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, 1 * time.Second},  // clamped
		{10, 1 * time.Second}, // clamped
	}
	for _, c := range cases {
		if got := jitteredBackoff(p, c.n, nil); got != c.want {
			t.Errorf("n=%d: got %v, want %v", c.n, got, c.want)
		}
	}
}

func TestJitteredBackoff_Defaults(t *testing.T) {
	t.Parallel()
	// Zero policy → 1s base, 30s cap.
	if got := jitteredBackoff(RetryPolicy{}, 0, nil); got != DefaultRetryBaseBackoff {
		t.Errorf("base default = %v, want %v", got, DefaultRetryBaseBackoff)
	}
	if got := jitteredBackoff(RetryPolicy{}, 20, nil); got != DefaultRetryMaxBackoff {
		t.Errorf("cap clamp default = %v, want %v", got, DefaultRetryMaxBackoff)
	}
}

func TestJitteredBackoff_Jitter_BoundedAndAdditive(t *testing.T) {
	t.Parallel()
	p := RetryPolicy{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 1 * time.Second, Jitter: 0.5}

	// rng=0 → no kick → exactly the base*2^n curve.
	if got := jitteredBackoff(p, 0, func() float64 { return 0 }); got != 100*time.Millisecond {
		t.Errorf("rng=0 must add no jitter, got %v", got)
	}
	// rng=0.999... → max additive (delay * jitter).
	gotMax := jitteredBackoff(p, 0, func() float64 { return 0.999_999 })
	// Expected ~ 100ms + 100ms*0.5 = 150ms (allowing 1 unit of
	// rounding from the rng clamp).
	if gotMax < 149*time.Millisecond || gotMax > 151*time.Millisecond {
		t.Errorf("rng~1 jitter out of band: %v, want ~150ms", gotMax)
	}
	// rng=0.5 → about half the max kick.
	gotMid := jitteredBackoff(p, 0, func() float64 { return 0.5 })
	if gotMid < 124*time.Millisecond || gotMid > 126*time.Millisecond {
		t.Errorf("rng=0.5 jitter out of band: %v, want ~125ms", gotMid)
	}
}

func TestJitteredBackoff_NilRngWithJitter_NoCrash(t *testing.T) {
	t.Parallel()
	// nil rng must be tolerated even when Jitter > 0 (no kick added).
	p := RetryPolicy{BaseBackoff: 100 * time.Millisecond, MaxBackoff: time.Second, Jitter: 0.5}
	if got := jitteredBackoff(p, 0, nil); got != 100*time.Millisecond {
		t.Errorf("nil rng with jitter: %v, want %v", got, 100*time.Millisecond)
	}
}

func TestCtxSleep(t *testing.T) {
	t.Parallel()
	if err := ctxSleep(context.Background(), 5*time.Millisecond); err != nil {
		t.Errorf("elapsed sleep: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ctxSleep(ctx, time.Hour); err == nil {
		t.Error("cancelled-ctx long sleep = nil, want ctx.Err()")
	}
	if err := ctxSleep(context.Background(), 0); err != nil {
		t.Errorf("zero d: %v", err)
	}
}
