package resources

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.CPUTime <= 0 {
		t.Error("CPUTime should be positive")
	}
	if limits.WallTime <= 0 {
		t.Error("WallTime should be positive")
	}
	if limits.Memory <= 0 {
		t.Error("Memory should be positive")
	}
	if !limits.AllowNetwork {
		t.Error("AllowNetwork should be true by default")
	}
	if !limits.AllowFilesystem {
		t.Error("AllowFilesystem should be true by default")
	}
}

func TestRestrictedLimits(t *testing.T) {
	limits := RestrictedLimits()

	if limits.AllowNetwork {
		t.Error("AllowNetwork should be false for restricted")
	}
	if limits.AllowFilesystem {
		t.Error("AllowFilesystem should be false for restricted")
	}
	if limits.AllowExec {
		t.Error("AllowExec should be false for restricted")
	}
}

func TestEnforcer_MemoryLimit(t *testing.T) {
	limits := &Limits{
		Memory: 1000,
	}

	enforcer := NewEnforcer(limits)

	// Record within limits
	err := enforcer.RecordMemory(500)
	if err != nil {
		t.Errorf("RecordMemory within limits = %v", err)
	}

	// Record exceeding limits
	err = enforcer.RecordMemory(600)
	if !errors.Is(err, ErrMemoryExceeded) {
		t.Errorf("RecordMemory exceeding = %v, want ErrMemoryExceeded", err)
	}

	// Release all memory
	enforcer.ReleaseMemory(1100)

	// Should be able to record again
	err = enforcer.RecordMemory(500)
	if err != nil {
		t.Errorf("RecordMemory after release = %v", err)
	}
}

func TestEnforcer_GoroutineLimit(t *testing.T) {
	limits := &Limits{
		MaxGoroutines: 3,
	}

	enforcer := NewEnforcer(limits)

	// Record within limits
	for i := 0; i < 3; i++ {
		if err := enforcer.RecordGoroutine(); err != nil {
			t.Errorf("RecordGoroutine %d = %v", i, err)
		}
	}

	// Exceeding limit
	err := enforcer.RecordGoroutine()
	if err == nil {
		t.Error("RecordGoroutine exceeding should fail")
	}

	// Release one
	enforcer.ReleaseGoroutine()

	// Should be able to record again
	err = enforcer.RecordGoroutine()
	if err != nil {
		t.Errorf("RecordGoroutine after release = %v", err)
	}
}

func TestEnforcer_OpenFileLimit(t *testing.T) {
	limits := &Limits{
		MaxOpenFiles: 2,
	}

	enforcer := NewEnforcer(limits)

	// Record within limits
	if err := enforcer.RecordOpenFile(); err != nil {
		t.Errorf("First RecordOpenFile = %v", err)
	}
	if err := enforcer.RecordOpenFile(); err != nil {
		t.Errorf("Second RecordOpenFile = %v", err)
	}

	// Exceeding limit
	err := enforcer.RecordOpenFile()
	if err == nil {
		t.Error("Third RecordOpenFile should fail")
	}

	// Release one
	enforcer.ReleaseOpenFile()

	// Should be able to record again
	err = enforcer.RecordOpenFile()
	if err != nil {
		t.Errorf("RecordOpenFile after release = %v", err)
	}
}

func TestEnforcer_NetworkConnLimit(t *testing.T) {
	limits := &Limits{
		AllowNetwork:    true,
		MaxNetworkConns: 2,
	}

	enforcer := NewEnforcer(limits)

	// Record within limits
	if err := enforcer.RecordNetworkConn(); err != nil {
		t.Errorf("First RecordNetworkConn = %v", err)
	}
	if err := enforcer.RecordNetworkConn(); err != nil {
		t.Errorf("Second RecordNetworkConn = %v", err)
	}

	// Exceeding limit
	err := enforcer.RecordNetworkConn()
	if err == nil {
		t.Error("Third RecordNetworkConn should fail")
	}

	// Release one
	enforcer.ReleaseNetworkConn()

	// Should be able to record again
	err = enforcer.RecordNetworkConn()
	if err != nil {
		t.Errorf("RecordNetworkConn after release = %v", err)
	}
}

func TestEnforcer_NetworkDisabled(t *testing.T) {
	limits := &Limits{
		AllowNetwork: false,
	}

	enforcer := NewEnforcer(limits)

	err := enforcer.CheckNetwork()
	if !errors.Is(err, ErrNetworkDisabled) {
		t.Errorf("CheckNetwork = %v, want ErrNetworkDisabled", err)
	}

	err = enforcer.RecordNetworkConn()
	if !errors.Is(err, ErrNetworkDisabled) {
		t.Errorf("RecordNetworkConn = %v, want ErrNetworkDisabled", err)
	}
}

func TestEnforcer_FilesystemDisabled(t *testing.T) {
	limits := &Limits{
		AllowFilesystem: false,
	}

	enforcer := NewEnforcer(limits)

	err := enforcer.CheckFilesystem()
	if !errors.Is(err, ErrFilesystemDisabled) {
		t.Errorf("CheckFilesystem = %v, want ErrFilesystemDisabled", err)
	}
}

func TestEnforcer_ExecDisabled(t *testing.T) {
	limits := &Limits{
		AllowExec: false,
	}

	enforcer := NewEnforcer(limits)

	err := enforcer.CheckExec()
	if err == nil {
		t.Error("CheckExec should fail when disabled")
	}
}

func TestEnforcer_FileSize(t *testing.T) {
	limits := &Limits{
		MaxFileSize: 1000,
	}

	enforcer := NewEnforcer(limits)

	// Within limit
	err := enforcer.CheckFileSize(500)
	if err != nil {
		t.Errorf("CheckFileSize within limit = %v", err)
	}

	// Exceeding limit
	err = enforcer.CheckFileSize(2000)
	if err == nil {
		t.Error("CheckFileSize exceeding should fail")
	}
}

func TestEnforcer_RecordBytes(t *testing.T) {
	limits := DefaultLimits()
	enforcer := NewEnforcer(limits)

	enforcer.RecordBytes(100, 50)

	usage := enforcer.Usage()
	if usage.BytesRead != 100 {
		t.Errorf("BytesRead = %d, want 100", usage.BytesRead)
	}
	if usage.BytesWritten != 50 {
		t.Errorf("BytesWritten = %d, want 50", usage.BytesWritten)
	}
}

func TestEnforcer_Usage(t *testing.T) {
	limits := DefaultLimits()
	enforcer := NewEnforcer(limits)

	enforcer.RecordMemory(1000)
	enforcer.RecordGoroutine()
	enforcer.RecordOpenFile()

	usage := enforcer.Usage()

	if usage.Memory != 1000 {
		t.Errorf("Memory = %d, want 1000", usage.Memory)
	}
	if usage.Goroutines != 1 {
		t.Errorf("Goroutines = %d, want 1", usage.Goroutines)
	}
	if usage.OpenFiles != 1 {
		t.Errorf("OpenFiles = %d, want 1", usage.OpenFiles)
	}
	if usage.WallTime <= 0 {
		t.Error("WallTime should be positive")
	}
}

func TestEnforcer_PeakMemory(t *testing.T) {
	limits := DefaultLimits()
	enforcer := NewEnforcer(limits)

	enforcer.RecordMemory(1000)
	enforcer.RecordMemory(500)   // Total: 1500
	enforcer.ReleaseMemory(1000) // Total: 500

	usage := enforcer.Usage()
	if usage.PeakMemory < 1500 {
		t.Errorf("PeakMemory = %d, want >= 1500", usage.PeakMemory)
	}
}

func TestEnforcer_Events(t *testing.T) {
	limits := &Limits{
		Memory: 100,
	}

	enforcer := NewEnforcer(limits)

	var events []*LimitEvent
	var mu sync.Mutex

	enforcer.AddListener(func(event *LimitEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Exceed memory limit
	enforcer.RecordMemory(200)

	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != "exceeded" {
		t.Errorf("Event type = %v, want exceeded", events[0].Type)
	}
	if events[0].Limit != "memory" {
		t.Errorf("Event limit = %v, want memory", events[0].Limit)
	}
	mu.Unlock()
}

func TestEnforcer_Monitor(t *testing.T) {
	limits := &Limits{
		WallTime: 100 * time.Millisecond,
	}

	enforcer := NewEnforcer(limits)
	ctx := context.Background()

	enforcer.Start(ctx)

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return errors.Is(enforcer.Violated(), ErrTimeoutExceeded), nil
	}); err != nil {
		t.Fatalf("expected wall time violation: %v", err)
	}

	enforcer.Stop()

	if !errors.Is(enforcer.Violated(), ErrTimeoutExceeded) {
		t.Errorf("Violated = %v, want ErrTimeoutExceeded", enforcer.Violated())
	}
}

func TestLimitedContext(t *testing.T) {
	limits := &Limits{
		Memory: 100,
	}

	enforcer := NewEnforcer(limits)
	ctx := context.Background()

	lc := NewLimitedContext(ctx, enforcer)

	// No error initially
	if lc.Err() != nil {
		t.Errorf("Initial Err() = %v", lc.Err())
	}

	// Exceed limit
	enforcer.RecordMemory(200)

	// Context error reflects violation
	// Note: The violation is set via events, not RecordMemory return
	// So we need to manually set it for this test
	enforcer.setViolated(ErrMemoryExceeded)

	if !errors.Is(lc.Err(), ErrMemoryExceeded) {
		t.Errorf("Err() = %v, want ErrMemoryExceeded", lc.Err())
	}
}

func TestPool(t *testing.T) {
	pool := NewPool("memory", 1000)

	if pool.Total() != 1000 {
		t.Errorf("Total = %d, want 1000", pool.Total())
	}
	if pool.Available() != 1000 {
		t.Errorf("Available = %d, want 1000", pool.Available())
	}

	ctx := context.Background()

	// Acquire
	err := pool.Acquire(ctx, 300)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if pool.Available() != 700 {
		t.Errorf("Available after acquire = %d, want 700", pool.Available())
	}

	// Release
	pool.Release(200)

	if pool.Available() != 900 {
		t.Errorf("Available after release = %d, want 900", pool.Available())
	}
}

func TestPool_TryAcquire(t *testing.T) {
	pool := NewPool("test", 100)

	// Should succeed
	if !pool.TryAcquire(50) {
		t.Error("TryAcquire(50) should succeed")
	}

	if !pool.TryAcquire(50) {
		t.Error("TryAcquire(50) should succeed again")
	}

	// Should fail - no capacity
	if pool.TryAcquire(50) {
		t.Error("TryAcquire(50) should fail when full")
	}
}

func TestPool_AcquireBlocking(t *testing.T) {
	pool := NewPool("test", 100)

	// Exhaust pool
	pool.Acquire(context.Background(), 100)

	// Try to acquire with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := pool.Acquire(ctx, 50)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Acquire on full pool = %v, want DeadlineExceeded", err)
	}
}

func TestResourceManager(t *testing.T) {
	rm := NewResourceManager()

	// Create pools
	memPool := rm.CreatePool("memory", 1000)
	cpuPool := rm.CreatePool("cpu", 100)

	// Get pools
	if rm.GetPool("memory") != memPool {
		t.Error("GetPool(memory) returned wrong pool")
	}
	if rm.GetPool("cpu") != cpuPool {
		t.Error("GetPool(cpu) returned wrong pool")
	}
	if rm.GetPool("nonexistent") != nil {
		t.Error("GetPool(nonexistent) should return nil")
	}

	// Use pools
	memPool.Acquire(context.Background(), 500)
	cpuPool.Acquire(context.Background(), 25)

	// Check stats
	stats := rm.Stats()
	if len(stats) != 2 {
		t.Fatalf("Expected 2 pools in stats, got %d", len(stats))
	}

	memStats := stats["memory"]
	if memStats.Total != 1000 {
		t.Errorf("memory Total = %d, want 1000", memStats.Total)
	}
	if memStats.Used != 500 {
		t.Errorf("memory Used = %d, want 500", memStats.Used)
	}
	if memStats.Available != 500 {
		t.Errorf("memory Available = %d, want 500", memStats.Available)
	}
}

func TestEnforcer_Concurrent(t *testing.T) {
	limits := &Limits{
		Memory:        1000000,
		MaxGoroutines: 100,
	}

	enforcer := NewEnforcer(limits)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				enforcer.RecordMemory(100)
				enforcer.ReleaseMemory(100)
				enforcer.RecordGoroutine()
				enforcer.ReleaseGoroutine()
			}
		}()
	}

	wg.Wait()

	usage := enforcer.Usage()
	// Memory and goroutines should be back to zero
	if usage.Memory != 0 {
		t.Errorf("Final memory = %d, want 0", usage.Memory)
	}
	if usage.Goroutines != 0 {
		t.Errorf("Final goroutines = %d, want 0", usage.Goroutines)
	}
}

func TestPool_Concurrent(t *testing.T) {
	pool := NewPool("test", 1000)

	ctx := context.Background()
	var wg sync.WaitGroup
	var acquireCount int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if pool.TryAcquire(10) {
					atomic.AddInt64(&acquireCount, 1)
					pool.Release(10)
				}
			}
		}()
	}

	wg.Wait()

	if pool.Available() != 1000 {
		t.Errorf("Final available = %d, want 1000", pool.Available())
	}

	// Some acquires should have succeeded
	if atomic.LoadInt64(&acquireCount) == 0 {
		t.Error("No acquires succeeded")
	}

	_ = ctx // unused in this test but available
}

func TestEnforcer_RecordIOPS(t *testing.T) {
	limits := DefaultLimits()
	enforcer := NewEnforcer(limits)

	// Record some IOPS
	for i := 0; i < 100; i++ {
		enforcer.RecordIOPS()
	}

	usage := enforcer.Usage()
	if usage.IOPS != 100 {
		t.Errorf("IOPS = %d, want 100", usage.IOPS)
	}
}
