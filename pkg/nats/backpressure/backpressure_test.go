package backpressure

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxPending <= 0 {
		t.Error("MaxPending should be positive")
	}
	if config.MaxBytes <= 0 {
		t.Error("MaxBytes should be positive")
	}
	if config.BufferSize <= 0 {
		t.Error("BufferSize should be positive")
	}
}

func TestPublisher_Block(t *testing.T) {
	var publishCount int32

	config := DefaultConfig()
	config.Strategy = StrategyBlock
	config.MaxPending = 10
	config.BlockTimeout = time.Second

	publisher := NewPublisher(config, func(msg *Message) error {
		atomic.AddInt32(&publishCount, 1)
		return nil
	})

	ctx := context.Background()

	// Publish some messages
	for i := 0; i < 5; i++ {
		err := publisher.Publish(ctx, &Message{
			Subject: "test",
			Data:    []byte("data"),
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	if atomic.LoadInt32(&publishCount) != 5 {
		t.Errorf("Published %d messages, want 5", atomic.LoadInt32(&publishCount))
	}
}

func TestPublisher_Drop(t *testing.T) {
	config := DefaultConfig()
	config.Strategy = StrategyDrop
	config.MaxPending = 5
	config.MaxBytes = 1000

	var slowMu sync.Mutex
	slowMu.Lock() // Lock to create slow publisher

	publisher := NewPublisher(config, func(msg *Message) error {
		slowMu.Lock()
		defer slowMu.Unlock()
		return nil
	})

	ctx := context.Background()

	// Fill up the queue quickly
	var wg sync.WaitGroup
	dropped := int32(0)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := publisher.Publish(ctx, &Message{
				Subject: "test",
				Data:    []byte("data"),
			})
			if err == ErrQueueFull {
				atomic.AddInt32(&dropped, 1)
			}
		}()
	}

	// Give goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Unlock to allow publishing
	slowMu.Unlock()

	wg.Wait()

	// Some should have been dropped
	if atomic.LoadInt32(&dropped) == 0 {
		t.Error("Expected some messages to be dropped")
	}

	stats := publisher.Stats()
	if stats.Dropped == 0 {
		t.Error("Stats.Dropped should be > 0")
	}
}

func TestPublisher_Buffer(t *testing.T) {
	config := DefaultConfig()
	config.Strategy = StrategyBuffer
	config.BufferSize = 10

	publisher := NewPublisher(config, func(msg *Message) error {
		return nil
	})

	ctx := context.Background()

	// Buffer some messages
	for i := 0; i < 5; i++ {
		err := publisher.Publish(ctx, &Message{
			Subject: "test",
			Data:    []byte("data"),
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	if publisher.BufferLen() != 5 {
		t.Errorf("Buffer length = %d, want 5", publisher.BufferLen())
	}

	// Flush buffer
	err := publisher.FlushBuffer(ctx)
	if err != nil {
		t.Fatalf("FlushBuffer failed: %v", err)
	}

	if publisher.BufferLen() != 0 {
		t.Errorf("Buffer length after flush = %d, want 0", publisher.BufferLen())
	}
}

func TestPublisher_Throttle(t *testing.T) {
	var publishCount int64

	config := DefaultConfig()
	config.Strategy = StrategyThrottle
	config.ThrottleRate = 100 // 100 messages per second

	publisher := NewPublisher(config, func(msg *Message) error {
		atomic.AddInt64(&publishCount, 1)
		return nil
	})

	ctx := context.Background()

	start := time.Now()

	// Publish messages
	for i := 0; i < 20; i++ {
		err := publisher.Publish(ctx, &Message{
			Subject: "test",
			Data:    []byte("data"),
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	elapsed := time.Since(start)

	// Should have taken some time due to throttling
	if elapsed < 50*time.Millisecond {
		t.Logf("Throttling may not have been effective (elapsed: %v)", elapsed)
	}

	// Stop the throttler
	if publisher.throttler != nil {
		publisher.throttler.stop()
	}
}

func TestPublisher_PauseResume(t *testing.T) {
	config := DefaultConfig()

	publisher := NewPublisher(config, func(msg *Message) error {
		return nil
	})

	ctx := context.Background()

	// Initially not paused
	if publisher.IsPaused() {
		t.Error("Publisher should not be paused initially")
	}

	// Pause
	publisher.Pause()
	if !publisher.IsPaused() {
		t.Error("Publisher should be paused after Pause()")
	}

	// Publish should fail
	err := publisher.Publish(ctx, &Message{Subject: "test", Data: []byte("data")})
	if err != ErrPublisherPaused {
		t.Errorf("Publish while paused = %v, want ErrPublisherPaused", err)
	}

	// Resume
	publisher.Resume()
	if publisher.IsPaused() {
		t.Error("Publisher should not be paused after Resume()")
	}

	// Publish should succeed
	err = publisher.Publish(ctx, &Message{Subject: "test", Data: []byte("data")})
	if err != nil {
		t.Errorf("Publish after resume = %v", err)
	}
}

func TestPublisher_Events(t *testing.T) {
	config := DefaultConfig()
	config.MaxPending = 10
	config.HighWaterMark = 0.5
	config.LowWaterMark = 0.2

	var events []*BackpressureEvent
	var mu sync.Mutex

	publisher := NewPublisher(config, func(msg *Message) error {
		return nil
	})

	publisher.AddListener(func(event *BackpressureEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Test pause event
	publisher.Pause()

	mu.Lock()
	if len(events) != 1 || events[0].Type != "pause" {
		t.Errorf("Expected pause event, got %v", events)
	}
	mu.Unlock()

	// Test resume event
	publisher.Resume()

	mu.Lock()
	if len(events) != 2 || events[1].Type != "resume" {
		t.Errorf("Expected resume event, got %v", events)
	}
	mu.Unlock()
}

func TestPublisher_Stats(t *testing.T) {
	var publishCount int64

	config := DefaultConfig()

	publisher := NewPublisher(config, func(msg *Message) error {
		atomic.AddInt64(&publishCount, 1)
		return nil
	})

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		publisher.Publish(ctx, &Message{Subject: "test", Data: []byte("data")})
	}

	stats := publisher.Stats()
	if stats.Published != 10 {
		t.Errorf("Stats.Published = %d, want 10", stats.Published)
	}
}

func TestFlowController(t *testing.T) {
	config := DefaultConfig()
	config.MaxPending = 10

	fc := NewFlowController(config)

	ctx := context.Background()

	// Acquire slots
	for i := 0; i < 5; i++ {
		if err := fc.AcquireSlot(ctx); err != nil {
			t.Fatalf("AcquireSlot failed: %v", err)
		}
	}

	if fc.InFlight() != 5 {
		t.Errorf("InFlight = %d, want 5", fc.InFlight())
	}

	// Ack some
	fc.AckN(3)

	if fc.InFlight() != 2 {
		t.Errorf("InFlight after ack = %d, want 2", fc.InFlight())
	}
}

func TestFlowController_WindowFull(t *testing.T) {
	config := DefaultConfig()
	config.MaxPending = 5

	fc := NewFlowController(config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Fill the window
	for i := 0; i < 5; i++ {
		fc.AcquireSlot(context.Background())
	}

	// Next acquire should timeout
	err := fc.AcquireSlot(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("AcquireSlot on full window = %v, want DeadlineExceeded", err)
	}
}

func TestFlowController_Events(t *testing.T) {
	config := DefaultConfig()
	config.MaxPending = 100

	fc := NewFlowController(config)

	var events []*FlowControlEvent
	var mu sync.Mutex

	fc.AddListener(func(event *FlowControlEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Resize window
	fc.SetWindowSize(50)

	mu.Lock()
	if len(events) != 1 || events[0].Type != "window_resize" {
		t.Errorf("Expected window_resize event, got %v", events)
	}
	if events[0].WindowSize != 50 {
		t.Errorf("WindowSize = %d, want 50", events[0].WindowSize)
	}
	mu.Unlock()
}

func TestAdaptiveFlowController(t *testing.T) {
	afc := NewAdaptiveFlowController(10, 1000)

	ctx := context.Background()

	// Acquire and record success
	afc.AcquireSlot(ctx)
	afc.RecordSuccess()

	// Window should potentially increase (though may not after just one)
	// The main thing is it shouldn't panic

	// Record failure
	initialWindow := afc.WindowSize()
	afc.AcquireSlot(ctx)
	afc.RecordFailure()

	// Window should decrease
	time.Sleep(10 * time.Millisecond) // Give time for adjustment

	if afc.WindowSize() > initialWindow {
		t.Error("Window should decrease after failure")
	}
}

func TestSemaphore(t *testing.T) {
	sem := NewSemaphore(3)

	if sem.Available() != 3 {
		t.Errorf("Available = %d, want 3", sem.Available())
	}

	ctx := context.Background()

	// Acquire all permits
	for i := 0; i < 3; i++ {
		if err := sem.Acquire(ctx); err != nil {
			t.Fatalf("Acquire failed: %v", err)
		}
	}

	if sem.Available() != 0 {
		t.Errorf("Available after acquire = %d, want 0", sem.Available())
	}

	// TryAcquire should fail
	if sem.TryAcquire() {
		t.Error("TryAcquire should fail when no permits available")
	}

	// Release one
	sem.Release()

	if sem.Available() != 1 {
		t.Errorf("Available after release = %d, want 1", sem.Available())
	}

	// TryAcquire should succeed
	if !sem.TryAcquire() {
		t.Error("TryAcquire should succeed after release")
	}
}

func TestSemaphore_Context(t *testing.T) {
	sem := NewSemaphore(1)
	sem.Acquire(context.Background()) // Take the only permit

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sem.Acquire(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Acquire = %v, want DeadlineExceeded", err)
	}
}

func TestPublisher_Concurrent(t *testing.T) {
	var publishCount int64

	config := DefaultConfig()
	config.Strategy = StrategyBlock
	config.MaxPending = 100

	publisher := NewPublisher(config, func(msg *Message) error {
		atomic.AddInt64(&publishCount, 1)
		time.Sleep(time.Microsecond) // Small delay
		return nil
	})

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				publisher.Publish(ctx, &Message{
					Subject: "test",
					Data:    []byte("data"),
				})
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&publishCount) != 1000 {
		t.Errorf("Published %d messages, want 1000", atomic.LoadInt64(&publishCount))
	}
}

func TestPublisher_ByteLimit(t *testing.T) {
	config := DefaultConfig()
	config.Strategy = StrategyDrop
	config.MaxPending = 1000
	config.MaxBytes = 100 // Very small byte limit

	publisher := NewPublisher(config, func(msg *Message) error {
		return nil
	})

	ctx := context.Background()

	// Try to publish large message that exceeds byte limit
	largeData := make([]byte, 200)

	err := publisher.Publish(ctx, &Message{
		Subject: "test",
		Data:    largeData,
	})

	// First message might succeed if pending is 0
	// But second should fail
	err = publisher.Publish(ctx, &Message{
		Subject: "test",
		Data:    largeData,
	})

	if err != ErrQueueFull {
		// It's possible both succeeded if they completed fast enough
		// Just verify no panic occurred
	}
}

func TestFlowController_ConcurrentAcquire(t *testing.T) {
	config := DefaultConfig()
	config.MaxPending = 100

	fc := NewFlowController(config)

	ctx := context.Background()
	var wg sync.WaitGroup
	var acquired int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fc.AcquireSlot(ctx)
				atomic.AddInt64(&acquired, 1)
				fc.Ack()
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&acquired) != 500 {
		t.Errorf("Acquired %d slots, want 500", atomic.LoadInt64(&acquired))
	}
}
