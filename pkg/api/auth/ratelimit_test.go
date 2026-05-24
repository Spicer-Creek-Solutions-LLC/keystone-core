// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func TestRateLimiter_Defaults(t *testing.T) {
	r := auth.NewRateLimiter(auth.RateLimitConfig{})
	if err := r.Allow("anyone"); err != nil {
		t.Errorf("default config should allow first request: %v", err)
	}
}

func TestRateLimiter_LockoutTrips(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 3,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
	})
	r.SetClock(func() time.Time { return now })

	for i := 0; i < 2; i++ {
		r.RecordFailure("client-1")
	}
	if err := r.Allow("client-1"); err != nil {
		t.Errorf("2 failures (under threshold) should still allow: %v", err)
	}

	r.RecordFailure("client-1") // 3rd failure trips lockout
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}

func TestRateLimiter_LockoutClears(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 2,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
	})
	clock := now
	r.SetClock(func() time.Time { return clock })

	r.RecordFailure("client-1")
	r.RecordFailure("client-1") // trips
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Fatalf("expected lockout: %v", err)
	}

	clock = now.Add(2 * time.Second) // lockout expired
	if err := r.Allow("client-1"); err != nil {
		t.Errorf("post-lockout: %v", err)
	}
}

func TestRateLimiter_ExponentialBackoff(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 1, // each failure trips immediately
		FailureWindow:        time.Hour,
		InitialLockout:       time.Second,
		MaxLockout:           time.Minute,
	})
	clock := now
	r.SetClock(func() time.Time { return clock })

	// Trip 1: 1s
	r.RecordFailure("client-1")
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Fatal(err)
	}
	clock = now.Add(2 * time.Second)
	if err := r.Allow("client-1"); err != nil {
		t.Fatalf("after 1s lockout: %v", err)
	}

	// Trip 2: 2s
	r.RecordFailure("client-1")
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second + 100*time.Millisecond) // still locked
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Errorf("trip 2 should be ~2s lockout; got %v", err)
	}
	clock = clock.Add(time.Second) // total +2.1s since trip 2
	if err := r.Allow("client-1"); err != nil {
		t.Errorf("after 2s lockout: %v", err)
	}
}

func TestRateLimiter_RecordSuccess_ClearsState(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 2,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
	})
	r.SetClock(func() time.Time { return now })

	r.RecordFailure("client-1")
	r.RecordFailure("client-1") // tripped
	r.RecordSuccess("client-1")

	if err := r.Allow("client-1"); err != nil {
		t.Errorf("after RecordSuccess: %v", err)
	}

	// And lockout level resets — next trip should be InitialLockout.
	r.RecordFailure("client-1")
	r.RecordFailure("client-1")
	if err := r.Allow("client-1"); !errors.Is(err, auth.ErrRateLimited) {
		t.Fatalf("expected fresh lockout: %v", err)
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 1,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Hour,
	})

	r.RecordFailure("a")
	if err := r.Allow("a"); !errors.Is(err, auth.ErrRateLimited) {
		t.Errorf("a should be locked: %v", err)
	}
	if err := r.Allow("b"); err != nil {
		t.Errorf("b should be unaffected: %v", err)
	}
}

func TestRateLimiter_FailureWindowSlides(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := auth.NewRateLimiter(auth.RateLimitConfig{
		MaxFailuresPerWindow: 3,
		FailureWindow:        time.Minute,
		InitialLockout:       time.Second,
	})
	clock := now
	r.SetClock(func() time.Time { return clock })

	// 2 failures, then wait > FailureWindow, then 2 more — should
	// NOT trip because the window slid.
	r.RecordFailure("c")
	r.RecordFailure("c")
	clock = now.Add(2 * time.Minute)
	r.RecordFailure("c")
	r.RecordFailure("c")

	if err := r.Allow("c"); err != nil {
		t.Errorf("window should have slid: %v", err)
	}
}
