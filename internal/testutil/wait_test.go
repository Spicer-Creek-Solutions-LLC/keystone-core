package testutil

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWaitFor(t *testing.T) {
	// Test immediate success
	t.Run("immediate success", func(t *testing.T) {
		err := WaitFor(func() bool { return true })
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	// Test eventual success
	t.Run("eventual success", func(t *testing.T) {
		count := 0
		err := WaitFor(func() bool {
			count++
			return count >= 3
		}, WaitConfig{Timeout: time.Second, PollInterval: 10 * time.Millisecond})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	// Test timeout
	t.Run("timeout", func(t *testing.T) {
		err := WaitFor(func() bool { return false }, WaitConfig{Timeout: 50 * time.Millisecond, PollInterval: 10 * time.Millisecond})
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestWaitForWithContext(t *testing.T) {
	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := WaitForWithContext(ctx, func() bool { return false }, 10*time.Millisecond)
		if err == nil {
			t.Error("expected context error")
		}
	})

	t.Run("condition met", func(t *testing.T) {
		ctx := context.Background()
		count := 0
		err := WaitForWithContext(ctx, func() bool {
			count++
			return count >= 2
		}, 10*time.Millisecond)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestEventually(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		count := 0
		result := Eventually(func() bool {
			count++
			return count >= 3
		}, time.Second, 10*time.Millisecond)
		if !result {
			t.Error("expected Eventually to succeed")
		}
	})

	t.Run("times out", func(t *testing.T) {
		result := Eventually(func() bool { return false }, 50*time.Millisecond, 10*time.Millisecond)
		if result {
			t.Error("expected Eventually to fail")
		}
	})
}

func TestNever(t *testing.T) {
	t.Run("condition stays false", func(t *testing.T) {
		result := Never(func() bool { return false }, 50*time.Millisecond)
		if !result {
			t.Error("expected Never to return true when condition stays false")
		}
	})

	t.Run("condition becomes true", func(t *testing.T) {
		count := 0
		result := Never(func() bool {
			count++
			return count >= 3
		}, time.Second)
		if result {
			t.Error("expected Never to return false when condition becomes true")
		}
	})
}

func TestCounter(t *testing.T) {
	t.Run("increment and value", func(t *testing.T) {
		c := NewCounter()
		if c.Value() != 0 {
			t.Error("expected initial value 0")
		}

		c.Inc()
		c.Inc()
		c.Inc()
		if c.Value() != 3 {
			t.Errorf("expected value 3, got %d", c.Value())
		}
	})

	t.Run("add", func(t *testing.T) {
		c := NewCounter()
		c.Add(5)
		if c.Value() != 5 {
			t.Errorf("expected value 5, got %d", c.Value())
		}
	})

	t.Run("reset", func(t *testing.T) {
		c := NewCounter()
		c.Add(10)
		c.Reset()
		if c.Value() != 0 {
			t.Errorf("expected value 0 after reset, got %d", c.Value())
		}
	})

	t.Run("wait for value", func(t *testing.T) {
		c := NewCounter()

		go func() {
			c.Add(5)
		}()

		err := c.WaitForValue(5, time.Second)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("wait for value timeout", func(t *testing.T) {
		c := NewCounter()
		err := c.WaitForValue(100, 50*time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
	})

	t.Run("concurrent increments", func(t *testing.T) {
		c := NewCounter()
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.Inc()
			}()
		}

		wg.Wait()
		if c.Value() != 100 {
			t.Errorf("expected value 100, got %d", c.Value())
		}
	})
}

func TestSignal(t *testing.T) {
	t.Run("send and wait", func(t *testing.T) {
		s := NewSignal()

		go func() {
			s.Send()
		}()

		err := s.Wait(time.Second)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("wait timeout", func(t *testing.T) {
		s := NewSignal()
		err := s.Wait(50 * time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
	})

	t.Run("multiple sends safe", func(t *testing.T) {
		s := NewSignal()
		s.Send()
		s.Send() // Should not panic
		s.Send()
	})

	t.Run("done channel", func(t *testing.T) {
		s := NewSignal()
		s.Send()

		select {
		case <-s.Done():
			// Success
		default:
			t.Error("expected Done channel to be closed")
		}
	})
}

func TestBarrier(t *testing.T) {
	t.Run("multiple goroutines", func(t *testing.T) {
		b := NewBarrier(3)
		results := make(chan int, 3)

		for i := 0; i < 3; i++ {
			go func(id int) {
				err := b.Wait(time.Second)
				if err == nil {
					results <- id
				}
			}(i)
		}

		// Wait for all to complete
		count := 0
		timeout := time.After(time.Second)
		for count < 3 {
			select {
			case <-results:
				count++
			case <-timeout:
				t.Fatalf("expected 3 results, got %d", count)
			}
		}
	})

	t.Run("timeout", func(t *testing.T) {
		b := NewBarrier(3)

		// Only one goroutine waits
		err := b.Wait(50 * time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestWaitGroup(t *testing.T) {
	t.Run("completes in time", func(t *testing.T) {
		var wg WaitGroup
		wg.Add(3)

		for i := 0; i < 3; i++ {
			go func() {
				wg.Done()
			}()
		}

		err := wg.WaitWithTimeout(time.Second)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		var wg WaitGroup
		wg.Add(1)
		// Never call Done()

		err := wg.WaitWithTimeout(50 * time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestPoll(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		ctx := context.Background()
		count := 0
		err := Poll(ctx, 10*time.Millisecond, func() (bool, error) {
			count++
			return count >= 3, nil
		})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("returns error", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("test error")
		err := Poll(ctx, 10*time.Millisecond, func() (bool, error) {
			return false, expectedErr
		})
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Poll(ctx, 10*time.Millisecond, func() (bool, error) {
			return false, nil
		})
		if err == nil {
			t.Error("expected context error")
		}
	})
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("succeeds after retries", func(t *testing.T) {
		ctx := context.Background()
		count := 0
		err := RetryWithBackoff(ctx, 5, 5*time.Millisecond, func() error {
			count++
			if count < 3 {
				return errors.New("not ready")
			}
			return nil
		})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 3 {
			t.Errorf("expected 3 attempts, got %d", count)
		}
	})

	t.Run("max attempts exceeded", func(t *testing.T) {
		ctx := context.Background()
		err := RetryWithBackoff(ctx, 3, 5*time.Millisecond, func() error {
			return errors.New("always fails")
		})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := RetryWithBackoff(ctx, 100, 50*time.Millisecond, func() error {
			return errors.New("always fails")
		})
		if err == nil {
			t.Error("expected error")
		}
	})
}
