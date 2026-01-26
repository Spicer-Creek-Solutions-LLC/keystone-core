package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestStrategy(t *testing.T) {
	strategies := []Strategy{
		StrategyTokenBucket,
		StrategySlidingWindow,
		StrategyFixedWindow,
		StrategyLeakyBucket,
	}

	for _, s := range strategies {
		if s == "" {
			t.Error("Strategy should not be empty")
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Strategy != StrategyTokenBucket {
		t.Errorf("Strategy = %s, want token_bucket", config.Strategy)
	}
	if config.Limit != 100 {
		t.Errorf("Limit = %d, want 100", config.Limit)
	}
	if config.Window != time.Minute {
		t.Errorf("Window = %v, want 1m", config.Window)
	}
	if config.BurstSize != 10 {
		t.Errorf("BurstSize = %d, want 10", config.BurstSize)
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	config := &Config{
		Strategy:   StrategyTokenBucket,
		Limit:      10,
		BurstSize:  10,
		RefillRate: 1.0,
	}

	tb := NewTokenBucket(config)
	defer tb.Stop()

	ctx := context.Background()

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		result, err := tb.Allow(ctx, "test-key")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 11th request should be denied
	result, err := tb.Allow(ctx, "test-key")
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be denied")
	}
	if result.RetryAfter <= 0 {
		t.Error("RetryAfter should be positive")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	config := &Config{
		Strategy:   StrategyTokenBucket,
		Limit:      10,
		BurstSize:  10,
		RefillRate: 1.0,
	}

	tb := NewTokenBucket(config)
	defer tb.Stop()

	ctx := context.Background()

	// Request 5 tokens
	result, err := tb.AllowN(ctx, "test-key", 5)
	if err != nil {
		t.Fatalf("AllowN failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Should be allowed")
	}
	if result.Remaining != 5 {
		t.Errorf("Remaining = %d, want 5", result.Remaining)
	}

	// Request 6 more - should fail
	result, err = tb.AllowN(ctx, "test-key", 6)
	if err != nil {
		t.Fatalf("AllowN failed: %v", err)
	}
	if result.Allowed {
		t.Error("Should be denied")
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	config := &Config{
		Strategy:   StrategyTokenBucket,
		Limit:      5,
		BurstSize:  5,
		RefillRate: 0.1,
	}

	tb := NewTokenBucket(config)
	defer tb.Stop()

	ctx := context.Background()

	// Use all tokens
	for i := 0; i < 5; i++ {
		tb.Allow(ctx, "test-key")
	}

	// Should be denied
	result, _ := tb.Allow(ctx, "test-key")
	if result.Allowed {
		t.Error("Should be denied after using all tokens")
	}

	// Reset
	err := tb.Reset(ctx, "test-key")
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should be allowed again
	result, _ = tb.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after reset")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	config := &Config{
		Strategy:   StrategyTokenBucket,
		Limit:      10,
		BurstSize:  10,
		RefillRate: 10.0, // 10 tokens per second
	}

	tb := NewTokenBucket(config)
	defer tb.Stop()

	ctx := context.Background()

	// Use all tokens
	for i := 0; i < 10; i++ {
		tb.Allow(ctx, "test-key")
	}

	bucket := tb.getBucket("test-key")
	bucket.mu.Lock()
	bucket.lastRefill = time.Now().Add(-150 * time.Millisecond)
	bucket.mu.Unlock()

	// Should have at least 1 token
	result, _ := tb.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after refill")
	}
}

func TestSlidingWindow_Allow(t *testing.T) {
	config := &Config{
		Strategy: StrategySlidingWindow,
		Limit:    10,
		Window:   time.Second,
	}

	sw := NewSlidingWindow(config)
	defer sw.Stop()

	ctx := context.Background()

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		result, err := sw.Allow(ctx, "test-key")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 11th request should be denied
	result, err := sw.Allow(ctx, "test-key")
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be denied")
	}
}

func TestSlidingWindow_WindowExpiry(t *testing.T) {
	config := &Config{
		Strategy: StrategySlidingWindow,
		Limit:    5,
		Window:   100 * time.Millisecond,
	}

	sw := NewSlidingWindow(config)
	defer sw.Stop()

	ctx := context.Background()

	// Use all quota
	for i := 0; i < 5; i++ {
		sw.Allow(ctx, "test-key")
	}

	// Should be denied
	result, _ := sw.Allow(ctx, "test-key")
	if result.Allowed {
		t.Error("Should be denied")
	}

	window := sw.getWindow("test-key")
	window.mu.Lock()
	for i := range window.counts {
		window.counts[i].timestamp = time.Now().Add(-config.Window - time.Millisecond)
	}
	window.mu.Unlock()

	// Should be allowed
	result, _ = sw.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after window expires")
	}
}

func TestFixedWindow_Allow(t *testing.T) {
	config := &Config{
		Strategy: StrategyFixedWindow,
		Limit:    10,
		Window:   time.Second,
	}

	fw := NewFixedWindow(config)
	defer fw.Stop()

	ctx := context.Background()

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		result, err := fw.Allow(ctx, "test-key")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 11th request should be denied
	result, err := fw.Allow(ctx, "test-key")
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be denied")
	}
	if result.ResetAt.IsZero() {
		t.Error("ResetAt should be set")
	}
}

func TestFixedWindow_WindowReset(t *testing.T) {
	config := &Config{
		Strategy: StrategyFixedWindow,
		Limit:    5,
		Window:   100 * time.Millisecond,
	}

	fw := NewFixedWindow(config)
	defer fw.Stop()

	ctx := context.Background()

	// Use all quota
	for i := 0; i < 5; i++ {
		fw.Allow(ctx, "test-key")
	}

	window := fw.getWindow("test-key")
	window.mu.Lock()
	window.windowEnd = time.Now().Add(-time.Millisecond)
	window.mu.Unlock()

	// Should be allowed with fresh limit
	result, _ := fw.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after window reset")
	}
	if result.Remaining != 4 {
		t.Errorf("Remaining = %d, want 4", result.Remaining)
	}
}

func TestLeakyBucket_Allow(t *testing.T) {
	config := &Config{
		Strategy:   StrategyLeakyBucket,
		Limit:      10,
		RefillRate: 10.0,
	}

	lb := NewLeakyBucket(config)
	defer lb.Stop()

	ctx := context.Background()

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		result, err := lb.Allow(ctx, "test-key")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 11th request should be denied
	result, err := lb.Allow(ctx, "test-key")
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be denied")
	}
}

func TestLeakyBucket_Leak(t *testing.T) {
	config := &Config{
		Strategy:   StrategyLeakyBucket,
		Limit:      5,
		RefillRate: 10.0, // 10 per second
	}

	lb := NewLeakyBucket(config)
	defer lb.Stop()

	ctx := context.Background()

	// Fill the bucket
	for i := 0; i < 5; i++ {
		lb.Allow(ctx, "test-key")
	}

	bucket := lb.getBucket("test-key")
	bucket.mu.Lock()
	bucket.lastLeak = time.Now().Add(-150 * time.Millisecond)
	bucket.mu.Unlock()

	// Should have capacity again
	result, _ := lb.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after leak")
	}
}

func TestNewLimiter(t *testing.T) {
	tests := []struct {
		strategy Strategy
		wantErr  bool
	}{
		{StrategyTokenBucket, false},
		{StrategySlidingWindow, false},
		{StrategyFixedWindow, false},
		{StrategyLeakyBucket, false},
		{"unknown", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			config := &Config{
				Strategy: tt.strategy,
				Limit:    10,
				Window:   time.Second,
			}

			limiter, err := NewLimiter(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && limiter == nil {
				t.Error("Expected non-nil limiter")
			}
		})
	}
}

func TestMultiLimiter(t *testing.T) {
	// Per-second limiter
	perSecond := NewFixedWindow(&Config{
		Strategy: StrategyFixedWindow,
		Limit:    5,
		Window:   time.Second,
	})

	// Per-minute limiter
	perMinute := NewFixedWindow(&Config{
		Strategy: StrategyFixedWindow,
		Limit:    10,
		Window:   time.Minute,
	})

	ml := NewMultiLimiter(perSecond, perMinute)

	ctx := context.Background()

	// First 5 should pass (per-second limit)
	for i := 0; i < 5; i++ {
		result, err := ml.Allow(ctx, "test-key")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th should fail (per-second limit)
	result, _ := ml.Allow(ctx, "test-key")
	if result.Allowed {
		t.Error("Should be denied by per-second limit")
	}
}

func TestMultiLimiter_Reset(t *testing.T) {
	limiter1 := NewTokenBucket(&Config{
		Strategy:  StrategyTokenBucket,
		Limit:     5,
		BurstSize: 5,
	})
	limiter2 := NewTokenBucket(&Config{
		Strategy:  StrategyTokenBucket,
		Limit:     5,
		BurstSize: 5,
	})

	ml := NewMultiLimiter(limiter1, limiter2)

	ctx := context.Background()

	// Use tokens
	for i := 0; i < 5; i++ {
		ml.Allow(ctx, "test-key")
	}

	// Reset
	err := ml.Reset(ctx, "test-key")
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should be allowed
	result, _ := ml.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Should be allowed after reset")
	}
}

func TestMiddleware(t *testing.T) {
	limiter := NewTokenBucket(&Config{
		Strategy:  StrategyTokenBucket,
		Limit:     5,
		BurstSize: 5,
	})

	keyFunc := func(ctx context.Context) string {
		return "default-key"
	}

	mw := NewMiddleware(limiter, keyFunc)

	ctx := context.Background()

	// First 5 should pass
	for i := 0; i < 5; i++ {
		result, err := mw.Check(ctx)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th should fail
	result, _ := mw.Check(ctx)
	if result.Allowed {
		t.Error("Should be denied")
	}
}

func TestResult(t *testing.T) {
	result := &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      100,
		RetryAfter: 5 * time.Second,
		ResetAt:    time.Now().Add(time.Minute),
	}

	if result.Allowed {
		t.Error("Allowed should be false")
	}
	if result.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter = %v, want 5s", result.RetryAfter)
	}
}

func TestConfig(t *testing.T) {
	config := &Config{
		Strategy:        StrategyTokenBucket,
		Limit:           1000,
		Window:          time.Hour,
		BurstSize:       100,
		RefillRate:      10.0,
		CleanupInterval: 10 * time.Minute,
	}

	if config.Limit != 1000 {
		t.Errorf("Limit = %d, want 1000", config.Limit)
	}
	if config.CleanupInterval != 10*time.Minute {
		t.Errorf("CleanupInterval = %v, want 10m", config.CleanupInterval)
	}
}

func TestTokenBucket_DifferentKeys(t *testing.T) {
	config := &Config{
		Strategy:  StrategyTokenBucket,
		Limit:     5,
		BurstSize: 5,
	}

	tb := NewTokenBucket(config)
	defer tb.Stop()

	ctx := context.Background()

	// Use all tokens for key1
	for i := 0; i < 5; i++ {
		tb.Allow(ctx, "key1")
	}

	// key1 should be denied
	result, _ := tb.Allow(ctx, "key1")
	if result.Allowed {
		t.Error("key1 should be denied")
	}

	// key2 should still be allowed
	result, _ = tb.Allow(ctx, "key2")
	if !result.Allowed {
		t.Error("key2 should be allowed")
	}
}

func TestSlidingWindow_AllowN(t *testing.T) {
	config := &Config{
		Strategy: StrategySlidingWindow,
		Limit:    10,
		Window:   time.Second,
	}

	sw := NewSlidingWindow(config)
	defer sw.Stop()

	ctx := context.Background()

	// Request 7
	result, _ := sw.AllowN(ctx, "test-key", 7)
	if !result.Allowed {
		t.Error("Should be allowed")
	}
	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}

	// Request 4 more - should fail
	result, _ = sw.AllowN(ctx, "test-key", 4)
	if result.Allowed {
		t.Error("Should be denied")
	}
}

func TestFixedWindow_AllowN(t *testing.T) {
	config := &Config{
		Strategy: StrategyFixedWindow,
		Limit:    10,
		Window:   time.Second,
	}

	fw := NewFixedWindow(config)
	defer fw.Stop()

	ctx := context.Background()

	// Request 8
	result, _ := fw.AllowN(ctx, "test-key", 8)
	if !result.Allowed {
		t.Error("Should be allowed")
	}
	if result.Remaining != 2 {
		t.Errorf("Remaining = %d, want 2", result.Remaining)
	}
}

func TestLeakyBucket_AllowN(t *testing.T) {
	config := &Config{
		Strategy:   StrategyLeakyBucket,
		Limit:      10,
		RefillRate: 1.0,
	}

	lb := NewLeakyBucket(config)
	defer lb.Stop()

	ctx := context.Background()

	// Request 6
	result, _ := lb.AllowN(ctx, "test-key", 6)
	if !result.Allowed {
		t.Error("Should be allowed")
	}
	if result.Remaining != 4 {
		t.Errorf("Remaining = %d, want 4", result.Remaining)
	}
}
