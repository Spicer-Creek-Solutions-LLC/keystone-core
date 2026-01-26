// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

// Package wait provides cancellable timing helpers without using time.Sleep.
// NOTE: API/ABI is not finalized and may change without notice.
package wait

import (
	"context"
	"fmt"
	"time"
)

// ForDuration waits for the provided duration without using time.Sleep.
func ForDuration(duration time.Duration) {
	if duration <= 0 {
		return
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	<-timer.C
}

// ForContext waits for the duration or returns early if the context is done.
func ForContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ForCondition polls until condition returns true or the context is done.
func ForCondition(ctx context.Context, interval time.Duration, condition func() (bool, error)) error {
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

// ForTimeout polls until condition returns true or the timeout expires.
func ForTimeout(timeout, interval time.Duration, condition func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ForCondition(ctx, interval, condition)
}

// ForSignal waits for the duration or returns early if the stop channel closes.
func ForSignal(stopCh <-chan struct{}, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}

// ForContextOrSignal waits for the duration or returns early if the context or stop channel closes.
func ForContextOrSignal(ctx context.Context, stopCh <-chan struct{}, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}
