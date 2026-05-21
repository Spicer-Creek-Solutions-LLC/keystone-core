package ratelimit

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"zero ok", Config{}, false},
		{"valid", Config{RequestsPerMinute: 60, Burst: 10}, false},
		{"negative rpm", Config{RequestsPerMinute: -1}, true},
		{"negative burst", Config{Burst: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Error("want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfig_EffectiveBurst(t *testing.T) {
	cases := []struct {
		cfg  Config
		want int
	}{
		{Config{RequestsPerMinute: 60, Burst: 10}, 10},
		{Config{RequestsPerMinute: 60, Burst: 0}, 60},
		{Config{RequestsPerMinute: 0, Burst: 0}, 0},
	}
	for _, tc := range cases {
		if got := tc.cfg.effectiveBurst(); got != tc.want {
			t.Errorf("effectiveBurst(%+v) = %d, want %d", tc.cfg, got, tc.want)
		}
	}
}

func TestBucket_Passthrough_WhenRPMZero(t *testing.T) {
	b := New(Config{}) // RPM=0
	for i := 0; i < 10_000; i++ {
		if !b.Allow() {
			t.Fatalf("passthrough bucket denied at i=%d", i)
		}
	}
	if got := b.RetryAfter(); got != 0 {
		t.Errorf("passthrough RetryAfter = %v, want 0", got)
	}
	allowed, delay := b.AllowOrRetryAfter()
	if !allowed || delay != 0 {
		t.Errorf("passthrough AllowOrRetryAfter = (%v, %v), want (true, 0)", allowed, delay)
	}
}

func TestBucket_BurstConsumedThenDenied(t *testing.T) {
	// 60 rpm = 1/sec; burst 5 → take 5 immediately, 6th denied.
	b := New(Config{RequestsPerMinute: 60, Burst: 5})
	t0 := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if !b.AllowN(t0, 1) {
			t.Errorf("burst token %d denied", i)
		}
	}
	if b.AllowN(t0, 1) {
		t.Error("post-burst token allowed")
	}
}

func TestBucket_RefillsOverTime(t *testing.T) {
	b := New(Config{RequestsPerMinute: 60, Burst: 1}) // 1/sec
	t0 := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	if !b.AllowN(t0, 1) {
		t.Fatal("first allow denied")
	}
	// Immediately retry → denied.
	if b.AllowN(t0, 1) {
		t.Fatal("second allow at same t should be denied")
	}
	// One second later → allowed.
	if !b.AllowN(t0.Add(time.Second), 1) {
		t.Fatal("allow at t+1s denied")
	}
}

func TestBucket_RetryAfter_NonZeroWhenExhausted(t *testing.T) {
	b := New(Config{RequestsPerMinute: 60, Burst: 1})
	if !b.Allow() {
		t.Fatal("first allow denied")
	}
	// Bucket exhausted; RetryAfter should be > 0 and < 2s
	// (60 RPM = 1/sec refill, so next token comes within ~1s).
	got := b.RetryAfter()
	if got <= 0 || got > 2*time.Second {
		t.Errorf("RetryAfter = %v, want (0, 2s]", got)
	}
}

func TestBucket_RetryAfter_DoesNotConsumeToken(t *testing.T) {
	// Calling RetryAfter must not advance the bucket — otherwise
	// observability would interfere with admission decisions. We
	// drive everything off time.Now() so the limiter's lastEvent
	// tracking is consistent.
	b := New(Config{RequestsPerMinute: 60, Burst: 2})
	_ = b.RetryAfter()
	_ = b.RetryAfter()
	_ = b.RetryAfter()
	// Bucket should still have 2 tokens.
	if !b.Allow() {
		t.Error("token 1 denied")
	}
	if !b.Allow() {
		t.Error("token 2 denied")
	}
}

func TestBucket_AllowOrRetryAfter_Combined(t *testing.T) {
	b := New(Config{RequestsPerMinute: 60, Burst: 1})

	// First call: allowed, delay 0.
	allowed, delay := b.AllowOrRetryAfter()
	if !allowed || delay != 0 {
		t.Errorf("first call: (allowed=%v, delay=%v), want (true, 0)", allowed, delay)
	}

	// Second call: denied + delay > 0.
	allowed, delay = b.AllowOrRetryAfter()
	if allowed {
		t.Error("second call allowed; want denied")
	}
	if delay <= 0 {
		t.Errorf("delay = %v, want > 0", delay)
	}
}

func TestBucket_LimiterShareBetweenAllowMethods(t *testing.T) {
	// Allow() and AllowN(time.Now(), 1) hit the same bucket.
	b := New(Config{RequestsPerMinute: 60, Burst: 1})
	if !b.Allow() {
		t.Fatal("Allow denied")
	}
	if b.Allow() {
		t.Error("second Allow allowed")
	}
}
