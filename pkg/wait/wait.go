// Package wait provides cancellable polling helpers.
package wait

import (
	"context"
	"time"
)

// ConditionFunc reports whether the condition is met. Returning a non-nil
// error stops polling and propagates the error to the caller.
type ConditionFunc func(ctx context.Context) (done bool, err error)

// ForCondition polls fn at the given interval until it returns done=true,
// returns an error, or ctx is cancelled. fn is invoked once immediately
// before the first tick.
//
// Returns nil if fn returned done=true; ctx.Err() if the context was
// cancelled; or fn's error otherwise.
func ForCondition(ctx context.Context, interval time.Duration, fn ConditionFunc) error {
	if interval <= 0 {
		panic("wait.ForCondition: interval must be > 0")
	}
	if done, err := fn(ctx); err != nil || done {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if done, err := fn(ctx); err != nil || done {
				return err
			}
		}
	}
}
