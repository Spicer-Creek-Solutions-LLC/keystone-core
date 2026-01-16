package helpers

import (
	"context"
	"fmt"
	"time"
)

// WaitForCondition polls until condition returns true or the context is done.
func WaitForCondition(ctx context.Context, interval time.Duration, condition func() (bool, error)) error {
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		ok, err := condition()
		if err != nil {
			lastErr = err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("condition not met: %w", lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// WaitForTimeout polls until condition returns true or the timeout expires.
func WaitForTimeout(timeout, interval time.Duration, condition func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return WaitForCondition(ctx, interval, condition)
}
