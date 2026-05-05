package wait

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestForCondition_ImmediateSuccess(t *testing.T) {
	calls := 0
	err := ForCondition(context.Background(), 10*time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return true, nil
	})
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no ticks needed)", calls)
	}
}

func TestForCondition_EventualSuccess(t *testing.T) {
	calls := 0
	err := ForCondition(context.Background(), time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return calls >= 3, nil
	})
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestForCondition_Error(t *testing.T) {
	wantErr := errors.New("boom")
	calls := 0
	err := ForCondition(context.Background(), time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return false, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (error should short-circuit)", calls)
	}
}

func TestForCondition_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := ForCondition(ctx, time.Millisecond, func(_ context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestForCondition_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := ForCondition(ctx, time.Millisecond, func(_ context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestForCondition_ZeroInterval_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("ForCondition with zero interval did not panic")
		}
	}()
	ForCondition(context.Background(), 0, func(_ context.Context) (bool, error) {
		return false, nil
	})
}
