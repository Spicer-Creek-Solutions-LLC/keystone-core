package nats

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestReconnectDelay_FirstAttemptIsBase(t *testing.T) {
	got := reconnectDelay(1, 2*time.Second, 30*time.Second, 0, nil)
	if got != 2*time.Second {
		t.Errorf("attempt=1 jitter=0 → %s, want 2s", got)
	}
}

func TestReconnectDelay_ExponentialGrowth(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 2 * time.Second},  // 2s
		{2, 4 * time.Second},  // 2s * 2
		{3, 8 * time.Second},  // 2s * 4
		{4, 16 * time.Second}, // 2s * 8
	}
	for _, c := range cases {
		got := reconnectDelay(c.attempts, 2*time.Second, 60*time.Second, 0, nil)
		if got != c.want {
			t.Errorf("attempt=%d → %s, want %s", c.attempts, got, c.want)
		}
	}
}

func TestReconnectDelay_CapsAtMax(t *testing.T) {
	// 2s * 2^9 = 1024s — well past 30s cap.
	got := reconnectDelay(10, 2*time.Second, 30*time.Second, 0, nil)
	if got != 30*time.Second {
		t.Errorf("attempt=10 → %s, want capped at 30s", got)
	}
}

func TestReconnectDelay_HugeAttemptsDoesNotOverflow(t *testing.T) {
	// 2^62 ns overflows int64. Function must not panic / wrap.
	got := reconnectDelay(100, 2*time.Second, 30*time.Second, 0, nil)
	if got != 30*time.Second {
		t.Errorf("attempt=100 → %s, want capped at 30s (overflow defense)", got)
	}
}

func TestReconnectDelay_DefensiveZeroAttempts(t *testing.T) {
	got := reconnectDelay(0, 2*time.Second, 30*time.Second, 0, nil)
	if got != 2*time.Second {
		t.Errorf("attempt=0 → %s, want base 2s (defensive)", got)
	}
}

func TestReconnectDelay_DefensiveNegativeAttempts(t *testing.T) {
	got := reconnectDelay(-1, 2*time.Second, 30*time.Second, 0, nil)
	if got != 2*time.Second {
		t.Errorf("attempt=-1 → %s, want base 2s (defensive)", got)
	}
}

func TestReconnectDelay_NoJitterIsDeterministic(t *testing.T) {
	// Same inputs → same output, even when an rng is provided
	// (we shouldn't consume randomness when jitter=0).
	rng := rand.New(rand.NewPCG(1, 2))
	a := reconnectDelay(3, time.Second, time.Minute, 0, rng)
	b := reconnectDelay(3, time.Second, time.Minute, 0, rng)
	if a != b {
		t.Errorf("jitter=0 not deterministic: %s vs %s", a, b)
	}
}

func TestReconnectDelay_JitterRespectsBounds(t *testing.T) {
	const (
		base     = 4 * time.Second
		max      = 60 * time.Second
		jitter   = 0.2
		attempts = 3 // exp = 16s, well below max
	)
	exp := 16 * time.Second
	low := time.Duration(float64(exp) * (1 - jitter))
	high := time.Duration(float64(exp) * (1 + jitter))

	rng := rand.New(rand.NewPCG(42, 99))
	for i := 0; i < 1000; i++ {
		got := reconnectDelay(attempts, base, max, jitter, rng)
		if got < low || got > high {
			t.Fatalf("iter=%d delay=%s out of [%s, %s]", i, got, low, high)
		}
	}
}

func TestReconnectDelay_JitterAtCapStillBounded(t *testing.T) {
	// Once capped at max, jitter is applied to the cap. Bounds
	// shift to [(1-j)*max, (1+j)*max].
	const (
		base     = 2 * time.Second
		max      = 30 * time.Second
		jitter   = 0.2
		attempts = 100 // way past cap
	)
	low := time.Duration(float64(max) * (1 - jitter))
	high := time.Duration(float64(max) * (1 + jitter))

	rng := rand.New(rand.NewPCG(7, 7))
	for i := 0; i < 200; i++ {
		got := reconnectDelay(attempts, base, max, jitter, rng)
		if got < low || got > high {
			t.Fatalf("iter=%d delay=%s out of [%s, %s]", i, got, low, high)
		}
	}
}

func TestReconnectDelay_NilRngFallsBackToUnjittered(t *testing.T) {
	got := reconnectDelay(3, 2*time.Second, 60*time.Second, 0.5, nil)
	if got != 8*time.Second {
		t.Errorf("nil rng → %s, want exact exp 8s", got)
	}
}
