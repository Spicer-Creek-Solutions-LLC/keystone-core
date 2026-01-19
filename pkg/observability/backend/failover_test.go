package backend

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.QueueSize <= 0 {
		t.Error("QueueSize should be positive")
	}
	if config.MaxRetries <= 0 {
		t.Error("MaxRetries should be positive")
	}
	if config.WriteTimeout <= 0 {
		t.Error("WriteTimeout should be positive")
	}
	if config.Strategy == "" {
		t.Error("Strategy should be set")
	}
}

func TestMemoryBackend(t *testing.T) {
	backend := NewMemoryBackend("test")
	ctx := context.Background()

	// Write data
	err := backend.Write(ctx, []byte("test data"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check data
	data := backend.Data()
	if len(data) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(data))
	}
	if string(data[0]) != "test data" {
		t.Errorf("Data = %s, want test data", data[0])
	}

	// Check health
	health := backend.Health()
	if !health.Healthy {
		t.Error("Backend should be healthy")
	}

	// Set unhealthy
	backend.SetHealthy(false)
	err = backend.Write(ctx, []byte("more data"))
	if err == nil {
		t.Error("Write should fail when unhealthy")
	}
}

func TestManager_Write(t *testing.T) {
	backend := NewMemoryBackend("primary")
	config := DefaultConfig()
	config.QueueSize = 100

	manager := NewManager(config, backend)
	defer manager.Close()

	ctx := context.Background()

	// Write data
	for i := 0; i < 10; i++ {
		err := manager.Write(ctx, []byte("test data"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Wait for queue processing
	time.Sleep(100 * time.Millisecond)

	// Check data was written
	data := backend.Data()
	if len(data) != 10 {
		t.Errorf("Expected 10 items, got %d", len(data))
	}
}

func TestManager_WriteSync(t *testing.T) {
	backend := NewMemoryBackend("primary")
	config := DefaultConfig()

	manager := NewManager(config, backend)
	defer manager.Close()

	ctx := context.Background()

	// Write sync
	err := manager.WriteSync(ctx, []byte("sync data"))
	if err != nil {
		t.Fatalf("WriteSync failed: %v", err)
	}

	// Check data was written immediately
	data := backend.Data()
	if len(data) != 1 {
		t.Errorf("Expected 1 item, got %d", len(data))
	}
}

func TestManager_QueueFull(t *testing.T) {
	backend := NewMemoryBackend("primary")
	backend.SetLatency(100 * time.Millisecond) // Slow backend

	config := DefaultConfig()
	config.QueueSize = 5

	manager := NewManager(config, backend)
	defer manager.Close()

	ctx := context.Background()

	// Fill the queue
	var queueFullCount int
	for i := 0; i < 20; i++ {
		err := manager.Write(ctx, []byte("data"))
		if err == ErrQueueFull {
			queueFullCount++
		}
	}

	if queueFullCount == 0 {
		t.Error("Expected some queue full errors")
	}
}

func TestManager_Failover(t *testing.T) {
	primary := NewMemoryBackend("primary")
	secondary := NewMemoryBackend("secondary")

	config := DefaultConfig()
	config.FailoverThreshold = 2
	config.Strategy = StrategyPrimary

	manager := NewManager(config, primary, secondary)
	defer manager.Close()

	ctx := context.Background()

	// Write to primary
	manager.WriteSync(ctx, []byte("data1"))

	// Make primary fail
	primary.SetFailAfter(1)

	// These should fail on primary and trigger failover
	manager.WriteSync(ctx, []byte("data2"))
	manager.WriteSync(ctx, []byte("data3"))

	// Wait a bit for state update
	time.Sleep(50 * time.Millisecond)

	// Now writes should go to secondary
	manager.WriteSync(ctx, []byte("data4"))

	// Check secondary received data
	secondaryData := secondary.Data()
	if len(secondaryData) == 0 {
		t.Error("Secondary should have received data after failover")
	}
}

func TestManager_RoundRobin(t *testing.T) {
	b1 := NewMemoryBackend("backend1")
	b2 := NewMemoryBackend("backend2")
	b3 := NewMemoryBackend("backend3")

	config := DefaultConfig()
	config.Strategy = StrategyRoundRobin

	manager := NewManager(config, b1, b2, b3)
	defer manager.Close()

	ctx := context.Background()

	// Write multiple items
	for i := 0; i < 9; i++ {
		manager.WriteSync(ctx, []byte("data"))
	}

	// Each backend should have roughly equal writes
	c1 := len(b1.Data())
	c2 := len(b2.Data())
	c3 := len(b3.Data())

	if c1 != 3 || c2 != 3 || c3 != 3 {
		t.Errorf("Round robin distribution: b1=%d, b2=%d, b3=%d, expected 3 each", c1, c2, c3)
	}
}

func TestManager_LeastLatency(t *testing.T) {
	fast := NewMemoryBackend("fast")
	slow := NewMemoryBackend("slow")
	slow.SetLatency(50 * time.Millisecond)

	config := DefaultConfig()
	config.Strategy = StrategyLeastLatency

	manager := NewManager(config, slow, fast)
	defer manager.Close()

	ctx := context.Background()

	// Prime both backends
	manager.WriteSync(ctx, []byte("prime1"))
	manager.WriteSync(ctx, []byte("prime2"))

	// Wait for latency stats
	time.Sleep(100 * time.Millisecond)

	// More writes should prefer the fast backend
	for i := 0; i < 10; i++ {
		manager.WriteSync(ctx, []byte("data"))
	}

	// Fast backend should have more writes
	fastCount := len(fast.Data())
	slowCount := len(slow.Data())

	if fastCount <= slowCount {
		t.Errorf("Fast backend should have more writes: fast=%d, slow=%d", fastCount, slowCount)
	}
}

func TestManager_Recovery(t *testing.T) {
	primary := NewMemoryBackend("primary")
	secondary := NewMemoryBackend("secondary")

	config := DefaultConfig()
	config.FailoverThreshold = 2
	config.RecoveryThreshold = 2
	config.Strategy = StrategyPrimary

	manager := NewManager(config, primary, secondary)
	defer manager.Close()

	ctx := context.Background()

	var events []*FailoverEvent
	var mu sync.Mutex

	manager.AddListener(func(event *FailoverEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// First write to primary succeeds
	manager.WriteSync(ctx, []byte("data"))

	// Make primary fail
	primary.SetFailAfter(1)

	// Trigger failures to mark primary unhealthy
	for i := 0; i < 3; i++ {
		manager.WriteSync(ctx, []byte("data"))
	}

	mu.Lock()
	hasUnhealthy := false
	for _, e := range events {
		if e.Type == "backend_unhealthy" && e.Backend == "primary" {
			hasUnhealthy = true
			break
		}
	}
	mu.Unlock()

	if !hasUnhealthy {
		t.Error("Expected backend_unhealthy event for primary")
	}

	// Reset primary
	primary.Reset()

	// Since primary is now healthy but marked unhealthy in manager,
	// writes go to secondary. We need to force a check on primary.
	// The recovery should happen when we try to write to primary again.
	// With primary-first strategy, if primary is unhealthy, it tries secondary.
	// To test recovery, we need to simulate the backend becoming healthy
	// and then successful writes triggering recovery.

	// Manually reset the backend state in manager for this test
	manager.mu.RLock()
	primaryState := manager.backends[0]
	manager.mu.RUnlock()

	// Force writes to primary even though it's "unhealthy" in manager
	primaryState.mu.Lock()
	primaryState.Healthy = false // Still unhealthy but will recover
	primaryState.mu.Unlock()

	// Write directly to backend to simulate recovery detection
	for i := 0; i < 3; i++ {
		manager.writeToBackend(ctx, primaryState, []byte("data"))
	}

	mu.Lock()
	defer mu.Unlock()

	hasRecovery := false
	for _, e := range events {
		if e.Type == "backend_recovered" && e.Backend == "primary" {
			hasRecovery = true
			break
		}
	}

	if !hasRecovery {
		t.Error("Expected recovery event for primary")
	}
}

func TestManager_Events(t *testing.T) {
	backend := NewMemoryBackend("primary")
	backend.SetFailAfter(1)

	config := DefaultConfig()
	config.FailoverThreshold = 2

	manager := NewManager(config, backend)
	defer manager.Close()

	var events []*FailoverEvent
	var mu sync.Mutex

	manager.AddListener(func(event *FailoverEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	ctx := context.Background()

	// Trigger unhealthy event
	for i := 0; i < 3; i++ {
		manager.WriteSync(ctx, []byte("data"))
	}

	mu.Lock()
	defer mu.Unlock()

	hasUnhealthy := false
	for _, e := range events {
		if e.Type == "backend_unhealthy" {
			hasUnhealthy = true
			break
		}
	}

	if !hasUnhealthy {
		t.Error("Expected backend_unhealthy event")
	}
}

func TestManager_Stats(t *testing.T) {
	b1 := NewMemoryBackend("backend1")
	b2 := NewMemoryBackend("backend2")

	config := DefaultConfig()

	manager := NewManager(config, b1, b2)
	defer manager.Close()

	ctx := context.Background()

	// Write some data
	for i := 0; i < 5; i++ {
		manager.WriteSync(ctx, []byte("data"))
	}

	stats := manager.Stats()

	if len(stats.Backends) != 2 {
		t.Fatalf("Expected 2 backends, got %d", len(stats.Backends))
	}

	// Primary should have all writes
	if stats.Backends[0].TotalWrites != 5 {
		t.Errorf("Backend 0 writes = %d, want 5", stats.Backends[0].TotalWrites)
	}
}

func TestManager_SetPrimary(t *testing.T) {
	b1 := NewMemoryBackend("backend1")
	b2 := NewMemoryBackend("backend2")

	config := DefaultConfig()
	config.Strategy = StrategyPrimary

	manager := NewManager(config, b1, b2)
	defer manager.Close()

	ctx := context.Background()

	// Write to primary (b1)
	manager.WriteSync(ctx, []byte("data1"))

	// Change primary to b2
	manager.SetPrimary(1)

	// Write should now go to b2
	manager.WriteSync(ctx, []byte("data2"))

	if len(b1.Data()) != 1 {
		t.Errorf("b1 writes = %d, want 1", len(b1.Data()))
	}
	if len(b2.Data()) != 1 {
		t.Errorf("b2 writes = %d, want 1", len(b2.Data()))
	}
}

func TestManager_Close(t *testing.T) {
	backend := NewMemoryBackend("primary")
	config := DefaultConfig()

	manager := NewManager(config, backend)

	// Close
	err := manager.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Write after close should fail
	ctx := context.Background()
	err = manager.Write(ctx, []byte("data"))
	if err != ErrBackendClosed {
		t.Errorf("Write after close = %v, want ErrBackendClosed", err)
	}

	err = manager.WriteSync(ctx, []byte("data"))
	if err != ErrBackendClosed {
		t.Errorf("WriteSync after close = %v, want ErrBackendClosed", err)
	}

	// Double close
	err = manager.Close()
	if err != ErrBackendClosed {
		t.Errorf("Double close = %v, want ErrBackendClosed", err)
	}
}

func TestManager_NoHealthyBackend(t *testing.T) {
	backend := NewMemoryBackend("primary")
	backend.SetHealthy(false)

	config := DefaultConfig()

	manager := NewManager(config, backend)
	defer manager.Close()

	// Force health check
	manager.checkHealth()

	ctx := context.Background()
	err := manager.WriteSync(ctx, []byte("data"))
	if err != ErrNoHealthyBackend {
		t.Errorf("Write to unhealthy backend = %v, want ErrNoHealthyBackend", err)
	}
}

func TestManager_HealthCheck(t *testing.T) {
	backend := NewMemoryBackend("primary")

	config := DefaultConfig()
	config.HealthCheckInterval = 50 * time.Millisecond

	manager := NewManager(config, backend)
	defer manager.Close()

	var events []*FailoverEvent
	var mu sync.Mutex

	manager.AddListener(func(event *FailoverEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Make unhealthy
	backend.SetHealthy(false)

	// Wait for health check
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	hasHealthFailed := false
	for _, e := range events {
		if e.Type == "health_check_failed" {
			hasHealthFailed = true
			break
		}
	}
	mu.Unlock()

	if !hasHealthFailed {
		t.Error("Expected health_check_failed event")
	}

	// Recover
	backend.SetHealthy(true)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	hasRecovered := false
	for _, e := range events {
		if e.Type == "health_check_recovered" {
			hasRecovered = true
			break
		}
	}
	mu.Unlock()

	if !hasRecovered {
		t.Error("Expected health_check_recovered event")
	}
}

func TestManager_Concurrent(t *testing.T) {
	backend := NewMemoryBackend("primary")

	config := DefaultConfig()
	config.QueueSize = 10000

	manager := NewManager(config, backend)
	defer manager.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	var writeCount int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				err := manager.Write(ctx, []byte("data"))
				if err == nil {
					atomic.AddInt64(&writeCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Wait for queue processing
	time.Sleep(200 * time.Millisecond)

	// All writes should have been queued
	if atomic.LoadInt64(&writeCount) != 1000 {
		t.Errorf("Queued %d writes, want 1000", atomic.LoadInt64(&writeCount))
	}

	// Backend should have received all data
	data := backend.Data()
	if len(data) != 1000 {
		t.Errorf("Backend received %d items, want 1000", len(data))
	}
}

func TestManager_RetryWithBackoff(t *testing.T) {
	backend := NewMemoryBackend("primary")
	backend.SetFailAfter(2) // Fail after 2 writes

	config := DefaultConfig()
	config.MaxRetries = 5
	config.RetryInterval = 10 * time.Millisecond
	config.QueueSize = 10

	manager := NewManager(config, backend)
	defer manager.Close()

	ctx := context.Background()

	// Queue items
	for i := 0; i < 5; i++ {
		manager.Write(ctx, []byte("data"))
	}

	// Wait for processing and retries
	time.Sleep(500 * time.Millisecond)

	// Some should have succeeded before failure
	data := backend.Data()
	if len(data) < 2 {
		t.Errorf("Expected at least 2 successful writes, got %d", len(data))
	}
}

func TestMemoryBackend_Reset(t *testing.T) {
	backend := NewMemoryBackend("test")
	ctx := context.Background()

	// Write data
	backend.Write(ctx, []byte("data1"))
	backend.Write(ctx, []byte("data2"))
	backend.SetFailAfter(5)
	backend.SetHealthy(false)

	// Reset
	backend.Reset()

	// Check reset
	if len(backend.Data()) != 0 {
		t.Error("Data should be empty after reset")
	}
	if backend.WriteCount() != 0 {
		t.Error("WriteCount should be 0 after reset")
	}

	health := backend.Health()
	if !health.Healthy {
		t.Error("Backend should be healthy after reset")
	}
}
