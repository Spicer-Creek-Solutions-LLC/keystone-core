package events

import (
	"sync"
	"testing"
	"time"
)

func TestRateTrackerZero(t *testing.T) {
	rt := NewRateTracker(10)
	if rate := rt.Rate(); rate != 0 {
		t.Errorf("expected 0 rate, got %f", rate)
	}
}

func TestRateTrackerBurst(t *testing.T) {
	rt := NewRateTracker(10)
	for i := 0; i < 100; i++ {
		rt.Increment()
	}
	rate := rt.Rate()
	if rate < 9.0 || rate > 11.0 {
		t.Errorf("expected ~10 events/s for 100 events in 10s window, got %f", rate)
	}
}

func TestRateTrackerSteady(t *testing.T) {
	rt := NewRateTracker(5)
	for i := 0; i < 50; i++ {
		rt.Increment()
	}
	rate := rt.Rate()
	if rate < 9.0 || rate > 11.0 {
		t.Errorf("expected ~10 events/s for 50 events in 5s window, got %f", rate)
	}
}

func TestRateTrackerRollover(t *testing.T) {
	rt := NewRateTracker(2)
	rt.Increment()
	rt.Increment()

	// Simulate time advancing past the window
	rt.mu.Lock()
	rt.lastTick = time.Now().Add(-3 * time.Second)
	rt.mu.Unlock()

	rate := rt.Rate()
	if rate != 0 {
		t.Errorf("expected 0 after window rollover, got %f", rate)
	}
}

func TestRateTrackerConcurrent(t *testing.T) {
	rt := NewRateTracker(10)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rt.Increment()
			}
		}()
	}
	wg.Wait()

	rate := rt.Rate()
	if rate < 90.0 || rate > 110.0 {
		t.Errorf("expected ~100 events/s for 1000 events in 10s window, got %f", rate)
	}
}

func TestNewRateTrackerMinWindow(t *testing.T) {
	rt := NewRateTracker(0)
	if len(rt.buckets) != 1 {
		t.Errorf("expected 1 bucket for zero window, got %d", len(rt.buckets))
	}
}
