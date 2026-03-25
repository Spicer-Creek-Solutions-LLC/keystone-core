package cluster

import (
	"sync/atomic"
	"testing"
)

func TestSafeDispatchObservers_AllCalled(t *testing.T) {
	var count atomic.Int32
	observers := []func(string){
		func(s string) { count.Add(1) },
		func(s string) { count.Add(1) },
		func(s string) { count.Add(1) },
	}

	safeDispatchObservers(observers, "test", func(o func(string), e any) {
		o(e.(string))
	})

	if got := count.Load(); got != 3 {
		t.Errorf("expected 3 observers called, got %d", got)
	}
}

func TestSafeDispatchObservers_PanicRecovery(t *testing.T) {
	var count atomic.Int32
	observers := []func(string){
		func(s string) { count.Add(1) },
		func(s string) { panic("test panic") },
		func(s string) { count.Add(1) },
	}

	// Should not panic
	safeDispatchObservers(observers, "test", func(o func(string), e any) {
		o(e.(string))
	})

	if got := count.Load(); got != 2 {
		t.Errorf("expected 2 non-panicking observers called, got %d", got)
	}
}

func TestSafeDispatchObservers_Empty(t *testing.T) {
	// Should not block or panic
	safeDispatchObservers([]func(string){}, "test", func(o func(string), e any) {
		o(e.(string))
	})
}

func TestCloseDone_Idempotent(t *testing.T) {
	g := &GracefulShutdown{
		doneChan: make(chan struct{}),
	}

	// First close should succeed
	g.closeDone()

	// Second close should not panic
	g.closeDone()

	// Channel should be closed
	select {
	case <-g.doneChan:
		// expected
	default:
		t.Error("doneChan should be closed")
	}
}
