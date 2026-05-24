// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"testing"
	"time"
)

func TestExpBackoff(t *testing.T) {
	t.Parallel()
	base, cap := 100*time.Millisecond, 1*time.Second
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
		if got := expBackoff(c.n, base, cap); got != c.want {
			t.Errorf("expBackoff(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestExpBackoff_ZeroFallsBackToDefaults(t *testing.T) {
	t.Parallel()
	if got := expBackoff(0, 0, 0); got != defaultBackoffBase {
		t.Errorf("expBackoff(0,0,0) = %v, want %v", got, defaultBackoffBase)
	}
}

func TestCtxSleep(t *testing.T) {
	t.Parallel()

	if err := ctxSleep(context.Background(), 5*time.Millisecond); err != nil {
		t.Errorf("elapsed sleep err = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ctxSleep(ctx, time.Hour); err == nil {
		t.Error("cancelled-ctx long sleep err = nil, want ctx.Err()")
	}

	// Non-positive duration returns immediately, still honouring a
	// cancelled ctx.
	if err := ctxSleep(context.Background(), 0); err != nil {
		t.Errorf("zero sleep err = %v, want nil", err)
	}
}
