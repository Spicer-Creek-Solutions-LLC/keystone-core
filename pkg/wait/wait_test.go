package wait

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestForDurationZero(t *testing.T) {
	done := make(chan struct{})

	go func() {
		ForDuration(0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("ForDuration(0) did not return promptly")
	}
}

func TestForContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := ForContext(ctx, 10*time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestForConditionSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	calls := 0
	err := ForCondition(ctx, 5*time.Millisecond, func() (bool, error) {
		calls++
		return calls >= 2, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestForConditionContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ForCondition(ctx, 5*time.Millisecond, func() (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestForConditionLastError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	expectedErr := errors.New("condition failed")
	err := ForCondition(ctx, 5*time.Millisecond, func() (bool, error) {
		return false, expectedErr
	})
	if err == nil || !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestForTimeoutReturnsError(t *testing.T) {
	err := ForTimeout(20*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return false, nil
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestForSignalStop(t *testing.T) {
	stopCh := make(chan struct{})
	close(stopCh)

	if ForSignal(stopCh, 10*time.Millisecond) {
		t.Fatal("expected ForSignal to return false when stopped")
	}
}

func TestForContextOrSignalContextCanceled(t *testing.T) {
	stopCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ForContextOrSignal(ctx, stopCh, 10*time.Millisecond) {
		t.Fatal("expected ForContextOrSignal to return false when context canceled")
	}
}
