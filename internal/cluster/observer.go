package cluster

import (
	"log/slog"
	"runtime/debug"
	"sync"
)

// safeDispatchObservers runs each observer in its own goroutine with panic
// recovery, and waits for all to complete before returning. This prevents
// observer panics from crashing the process and ensures all observers finish
// before the caller proceeds.
func safeDispatchObservers[T any](observers []T, event any, call func(T, any)) {
	var wg sync.WaitGroup
	wg.Add(len(observers))
	for _, obs := range observers {
		go func(o T) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("observer panic recovered", "panic", r, "stack", string(debug.Stack()))
				}
			}()
			call(o, event)
		}(obs)
	}
	wg.Wait()
}
