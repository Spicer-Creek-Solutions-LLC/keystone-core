package spire

import (
	"context"
	"testing"
	"time"
)

func TestWaitForRetry_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitForRetry(ctx, time.Second) {
		t.Fatal("expected wait to stop on canceled context")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("expected wait to return promptly")
	}
}

func TestWaitForRetry_Elapsed(t *testing.T) {
	ctx := context.Background()

	if !waitForRetry(ctx, 10*time.Millisecond) {
		t.Fatal("expected wait to complete")
	}
}
