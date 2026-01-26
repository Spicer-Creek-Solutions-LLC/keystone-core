package retry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestExponentialBackoff_NextBackoff(t *testing.T) {
	tests := []struct {
		name     string
		eb       *ExponentialBackoff
		attempt  int
		wantMin  time.Duration
		wantMax  time.Duration
		wantZero bool
	}{
		{
			name: "first attempt",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     30 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      5,
				Jitter:          false,
			},
			attempt: 0,
			wantMin: 1 * time.Second,
			wantMax: 1 * time.Second,
		},
		{
			name: "second attempt",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     30 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      5,
				Jitter:          false,
			},
			attempt: 1,
			wantMin: 2 * time.Second,
			wantMax: 2 * time.Second,
		},
		{
			name: "third attempt",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     30 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      5,
				Jitter:          false,
			},
			attempt: 2,
			wantMin: 4 * time.Second,
			wantMax: 4 * time.Second,
		},
		{
			name: "capped at max",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     5 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      10,
				Jitter:          false,
			},
			attempt: 5, // Would be 32s, but capped at 5s
			wantMin: 5 * time.Second,
			wantMax: 5 * time.Second,
		},
		{
			name: "max retries exceeded",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     30 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      3,
				Jitter:          false,
			},
			attempt:  3, // 0, 1, 2 are valid, 3 exceeds
			wantZero: true,
		},
		{
			name: "with jitter",
			eb: &ExponentialBackoff{
				InitialInterval: 1 * time.Second,
				MaxInterval:     30 * time.Second,
				Multiplier:      2.0,
				MaxRetries:      5,
				Jitter:          true,
				JitterFactor:    0.2,
			},
			attempt: 0,
			wantMin: 800 * time.Millisecond,  // 1s - 20%
			wantMax: 1200 * time.Millisecond, // 1s + 20%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.eb.NextBackoff(tt.attempt)

			if tt.wantZero {
				if got != 0 {
					t.Errorf("NextBackoff() = %v, want 0", got)
				}
				return
			}

			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("NextBackoff() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestLinearBackoff_NextBackoff(t *testing.T) {
	lb := &LinearBackoff{
		InitialInterval: 1 * time.Second,
		Increment:       1 * time.Second,
		MaxInterval:     5 * time.Second,
		MaxRetries:      5,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 3 * time.Second},
		{3, 4 * time.Second},
		{4, 5 * time.Second}, // Capped
		{5, 0},               // Max retries exceeded
	}

	for _, tt := range tests {
		got := lb.NextBackoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("NextBackoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestConstantBackoff_NextBackoff(t *testing.T) {
	cb := &ConstantBackoff{
		Interval:   5 * time.Second,
		MaxRetries: 3,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 5 * time.Second},
		{1, 5 * time.Second},
		{2, 5 * time.Second},
		{3, 0}, // Max retries exceeded
	}

	for _, tt := range tests {
		got := cb.NextBackoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("NextBackoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestRetrier_Do_Success(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 3},
	})

	callCount := 0
	result := r.Do(context.Background(), func(ctx context.Context) error {
		callCount++
		return nil // Success on first try
	})

	if !result.Success {
		t.Errorf("Expected success")
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestRetrier_Do_RetryThenSuccess(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 5},
	})

	callCount := 0
	result := r.Do(context.Background(), func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("transient error")
		}
		return nil // Success on third try
	})

	if !result.Success {
		t.Errorf("Expected success")
	}
	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestRetrier_Do_MaxRetriesExceeded(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 3},
	})

	callCount := 0
	result := r.Do(context.Background(), func(ctx context.Context) error {
		callCount++
		return errors.New("persistent error")
	})

	if result.Success {
		t.Errorf("Expected failure")
	}
	// MaxRetries=3 means 3 retries after initial attempt = 4 total attempts
	if result.Attempts != 4 {
		t.Errorf("Expected 4 attempts (initial + 3 retries), got %d", result.Attempts)
	}
	if result.LastErr == nil {
		t.Errorf("Expected error")
	}
}

func TestRetrier_Do_NonRetryableError(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy:    &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 5},
		IsRetryable: func(err error) bool { return false },
	})

	callCount := 0
	result := r.Do(context.Background(), func(ctx context.Context) error {
		callCount++
		return errors.New("non-retryable error")
	})

	if result.Success {
		t.Errorf("Expected failure")
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestRetrier_Do_ContextCancellation(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 1 * time.Second, MaxRetries: 10},
	})

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	started := make(chan struct{})
	var once sync.Once
	go func() {
		<-started
		cancel()
	}()

	result := r.Do(ctx, func(ctx context.Context) error {
		callCount++
		once.Do(func() {
			close(started)
		})
		return errors.New("error")
	})

	if result.Success {
		t.Errorf("Expected failure due to cancellation")
	}
	if !errors.Is(result.LastErr, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", result.LastErr)
	}
}

func TestRetrier_OnRetryCallback(t *testing.T) {
	retryCount := 0
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 3},
		OnRetry: func(attempt int, err error, nextBackoff time.Duration) {
			retryCount++
		},
	})

	callCount := 0
	r.Do(context.Background(), func(ctx context.Context) error {
		callCount++
		return errors.New("error")
	})

	// MaxRetries=3 means 3 retries, so callback is called 3 times (before each retry)
	if retryCount != 3 {
		t.Errorf("Expected 3 retry callbacks, got %d", retryCount)
	}
}

func TestDoWithValue(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 10 * time.Millisecond, MaxRetries: 5},
	})

	callCount := 0
	value, result := DoWithValue(context.Background(), r, func(ctx context.Context) (int, error) {
		callCount++
		if callCount < 3 {
			return 0, errors.New("transient error")
		}
		return 42, nil
	})

	if !result.Success {
		t.Errorf("Expected success")
	}
	if value != 42 {
		t.Errorf("Expected value 42, got %d", value)
	}
	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
}

func TestCommandRetryConfig_ShouldRetryCommand(t *testing.T) {
	tests := []struct {
		name      string
		config    *CommandRetryConfig
		err       error
		exitCode  int32
		isTimeout bool
		want      bool
	}{
		{
			name:      "timeout with retry enabled",
			config:    &CommandRetryConfig{RetryOnTimeout: true},
			isTimeout: true,
			want:      true,
		},
		{
			name:      "timeout with retry disabled",
			config:    &CommandRetryConfig{RetryOnTimeout: false},
			isTimeout: true,
			want:      false,
		},
		{
			name:   "network error with retry enabled",
			config: &CommandRetryConfig{RetryOnNetworkError: true},
			err:    errors.New("connection refused"),
			want:   true,
		},
		{
			name:   "network error with retry disabled",
			config: &CommandRetryConfig{RetryOnNetworkError: false},
			err:    errors.New("connection refused"),
			want:   false,
		},
		{
			name:     "failed command with retry enabled",
			config:   &CommandRetryConfig{RetryOnFailure: true},
			exitCode: 1,
			want:     true,
		},
		{
			name:     "failed command with retry disabled",
			config:   &CommandRetryConfig{RetryOnFailure: false},
			exitCode: 1,
			want:     false,
		},
		{
			name:     "failed command with specific retryable codes",
			config:   &CommandRetryConfig{RetryOnFailure: true, RetryableExitCodes: []int32{1, 2}},
			exitCode: 1,
			want:     true,
		},
		{
			name:     "failed command with non-matching retryable codes",
			config:   &CommandRetryConfig{RetryOnFailure: true, RetryableExitCodes: []int32{1, 2}},
			exitCode: 3,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldRetryCommand(tt.err, tt.exitCode, tt.isTimeout)
			if got != tt.want {
				t.Errorf("ShouldRetryCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryStats(t *testing.T) {
	stats := NewRetryStats()

	stats.RecordAttempt()
	stats.RecordAttempt()
	stats.RecordRetry("timeout")
	stats.RecordRetry("timeout")
	stats.RecordRetry("network")
	stats.RecordSuccess()
	stats.RecordFailure()

	snap := stats.Snapshot()

	if snap.TotalAttempts != 2 {
		t.Errorf("TotalAttempts = %d, want 2", snap.TotalAttempts)
	}
	if snap.TotalRetries != 3 {
		t.Errorf("TotalRetries = %d, want 3", snap.TotalRetries)
	}
	if snap.TotalSuccesses != 1 {
		t.Errorf("TotalSuccesses = %d, want 1", snap.TotalSuccesses)
	}
	if snap.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", snap.TotalFailures)
	}
	if snap.RetriesByReason["timeout"] != 2 {
		t.Errorf("RetriesByReason[timeout] = %d, want 2", snap.RetriesByReason["timeout"])
	}
	if snap.RetriesByReason["network"] != 1 {
		t.Errorf("RetriesByReason[network] = %d, want 1", snap.RetriesByReason["network"])
	}
}

func TestStrategy_Clone(t *testing.T) {
	t.Run("ExponentialBackoff", func(t *testing.T) {
		original := DefaultExponentialBackoff()
		cloned := original.Clone()

		// Modify original
		original.MaxRetries = 100

		// Clone should be unaffected
		eb, ok := cloned.(*ExponentialBackoff)
		if !ok {
			t.Fatal("Clone should return ExponentialBackoff")
		}
		if eb.MaxRetries == 100 {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("LinearBackoff", func(t *testing.T) {
		original := DefaultLinearBackoff()
		cloned := original.Clone()

		original.MaxRetries = 100

		lb, ok := cloned.(*LinearBackoff)
		if !ok {
			t.Fatal("Clone should return LinearBackoff")
		}
		if lb.MaxRetries == 100 {
			t.Error("Clone should be independent of original")
		}
	})
}

func BenchmarkExponentialBackoff_NextBackoff(b *testing.B) {
	eb := DefaultExponentialBackoff()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.NextBackoff(i % 5)
	}
}

func BenchmarkRetrier_Do(b *testing.B) {
	r := NewRetrier(&RetryConfig{
		Strategy: &ConstantBackoff{Interval: 0, MaxRetries: 3},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}
}
