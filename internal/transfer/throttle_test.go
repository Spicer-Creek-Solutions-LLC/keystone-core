package transfer

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.BytesPerSecond <= 0 {
		t.Error("BytesPerSecond should be positive")
	}
	if config.BurstSize <= 0 {
		t.Error("BurstSize should be positive")
	}
	if config.MinChunkSize <= 0 {
		t.Error("MinChunkSize should be positive")
	}
	if config.MaxChunkSize <= 0 {
		t.Error("MaxChunkSize should be positive")
	}
}

func TestThrottler_WaitN(t *testing.T) {
	config := &Config{
		BytesPerSecond: 1000, // 1 KB/s
		BurstSize:      1000,
		MinChunkSize:   100,
		MaxChunkSize:   500,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	ctx := context.Background()

	// Should immediately acquire from burst
	start := time.Now()
	err := throttler.WaitN(ctx, 500)
	if err != nil {
		t.Fatalf("WaitN failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Errorf("First WaitN took too long: %v", elapsed)
	}
}

func TestThrottler_WaitN_Blocking(t *testing.T) {
	config := &Config{
		BytesPerSecond: 1000, // 1 KB/s
		BurstSize:      100,  // Small burst
		MinChunkSize:   10,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	ctx := context.Background()

	// Consume burst
	throttler.WaitN(ctx, 100)

	// Next wait should take time
	start := time.Now()
	err := throttler.WaitN(ctx, 100)
	if err != nil {
		t.Fatalf("WaitN failed: %v", err)
	}

	elapsed := time.Since(start)
	// Should take roughly 100ms (100 bytes at 1000 bytes/s)
	if elapsed < 50*time.Millisecond {
		t.Errorf("Second WaitN should have blocked: %v", elapsed)
	}
}

func TestThrottler_WaitN_Context(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100,
		BurstSize:      10, // Very small burst
		MinChunkSize:   1,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	// Consume burst
	throttler.WaitN(context.Background(), 10)

	// Use timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := throttler.WaitN(ctx, 100)
	if err != context.DeadlineExceeded {
		t.Errorf("WaitN = %v, want context.DeadlineExceeded", err)
	}
}

func TestThrottler_TryAcquire(t *testing.T) {
	config := &Config{
		BytesPerSecond: 1000,
		BurstSize:      100,
		MinChunkSize:   10,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	// Should succeed
	if !throttler.TryAcquire(50) {
		t.Error("TryAcquire(50) should succeed")
	}

	// Should succeed again (still have 50 tokens)
	if !throttler.TryAcquire(50) {
		t.Error("TryAcquire(50) should succeed again")
	}

	// Should fail (no tokens left)
	if throttler.TryAcquire(50) {
		t.Error("TryAcquire(50) should fail when no tokens")
	}
}

func TestThrottler_SetRate(t *testing.T) {
	config := DefaultConfig()
	throttler := NewThrottler(config)
	defer throttler.Stop()

	throttler.SetRate(5000)
	throttler.SetBurstSize(500)

	// Verify rate was set (we can't easily test the actual throughput)
}

func TestThrottler_Stop(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100,
		BurstSize:      10,
		MinChunkSize:   1,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)

	// Consume burst
	throttler.WaitN(context.Background(), 10)

	// Start a goroutine that will be waiting
	var wg sync.WaitGroup
	wg.Add(1)
	var waitErr error
	started := make(chan struct{})
	go func() {
		defer wg.Done()
		close(started)
		waitErr = throttler.WaitN(context.Background(), 100)
	}()

	<-started

	// Stop the throttler
	throttler.Stop()

	// Wait for goroutine
	wg.Wait()

	if waitErr != ErrThrottlerStopped {
		t.Errorf("WaitN after stop = %v, want ErrThrottlerStopped", waitErr)
	}
}

func TestThrottledReader(t *testing.T) {
	config := &Config{
		BytesPerSecond: 10000,
		BurstSize:      10000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	data := strings.Repeat("x", 5000)
	reader := strings.NewReader(data)

	ctx := context.Background()
	tr := NewThrottledReader(ctx, reader, throttler)

	// Read all data
	result, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(result) != 5000 {
		t.Errorf("Read %d bytes, want 5000", len(result))
	}

	stats := tr.Stats()
	if stats.BytesTransferred != 5000 {
		t.Errorf("BytesTransferred = %d, want 5000", stats.BytesTransferred)
	}
}

func TestThrottledWriter(t *testing.T) {
	config := &Config{
		BytesPerSecond: 10000,
		BurstSize:      10000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	var buf bytes.Buffer
	ctx := context.Background()
	tw := NewThrottledWriter(ctx, &buf, throttler)

	data := []byte(strings.Repeat("x", 5000))
	n, err := tw.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != 5000 {
		t.Errorf("Wrote %d bytes, want 5000", n)
	}

	stats := tw.Stats()
	if stats.BytesTransferred != 5000 {
		t.Errorf("BytesTransferred = %d, want 5000", stats.BytesTransferred)
	}
}

func TestTransferStats_BytesPerSecond(t *testing.T) {
	stats := &TransferStats{
		StartTime:        time.Now().Add(-time.Second),
		BytesTransferred: 1000,
	}

	rate := stats.BytesPerSecond()
	// Rate should be approximately 1000 bytes/s
	if rate < 900 || rate > 1100 {
		t.Errorf("BytesPerSecond() = %v, expected ~1000", rate)
	}
}

func TestBandwidthPool(t *testing.T) {
	config := &Config{
		BytesPerSecond: 10000,
		BurstSize:      10000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	pool := NewBandwidthPool(config)
	defer pool.Stop()

	// Register transfers
	pool.RegisterTransfer("transfer1", 1)
	pool.RegisterTransfer("transfer2", 2)

	stats := pool.Stats()
	if stats.ActiveTransfers != 2 {
		t.Errorf("ActiveTransfers = %d, want 2", stats.ActiveTransfers)
	}

	// Unregister
	pool.UnregisterTransfer("transfer1")

	stats = pool.Stats()
	if stats.ActiveTransfers != 1 {
		t.Errorf("ActiveTransfers = %d, want 1", stats.ActiveTransfers)
	}
}

func TestBandwidthPool_GetReader(t *testing.T) {
	config := &Config{
		BytesPerSecond: 10000,
		BurstSize:      10000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	pool := NewBandwidthPool(config)
	defer pool.Stop()

	data := strings.NewReader("test data")
	ctx := context.Background()

	reader := pool.GetReader(ctx, "test", data)
	if reader == nil {
		t.Fatal("GetReader returned nil")
	}
}

func TestBandwidthPool_GetWriter(t *testing.T) {
	config := &Config{
		BytesPerSecond: 10000,
		BurstSize:      10000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	pool := NewBandwidthPool(config)
	defer pool.Stop()

	var buf bytes.Buffer
	ctx := context.Background()

	writer := pool.GetWriter(ctx, "test", &buf)
	if writer == nil {
		t.Fatal("GetWriter returned nil")
	}
}

func TestAdaptiveThrottler(t *testing.T) {
	minRate := int64(1000)
	maxRate := int64(10000)

	at := NewAdaptiveThrottler(minRate, maxRate)
	defer at.Stop()

	if at.CurrentRate() != maxRate {
		t.Errorf("Initial rate = %d, want %d", at.CurrentRate(), maxRate)
	}

	// Record poor conditions
	at.RecordLatency(600 * time.Millisecond)
	at.RecordPacketLoss(0.1)
	at.RecordRetry()
	at.RecordRetry()
	at.RecordRetry()
	at.RecordRetry()
	at.RecordRetry()
	at.RecordRetry()

	at.adjust()

	// Rate should have decreased
	currentRate := at.CurrentRate()
	if currentRate >= maxRate {
		t.Errorf("Rate should have decreased from %d, got %d", maxRate, currentRate)
	}
}

func TestAdaptiveThrottler_Reader(t *testing.T) {
	at := NewAdaptiveThrottler(1000, 100000)
	defer at.Stop()

	data := strings.NewReader("test data")
	ctx := context.Background()

	reader := at.Reader(ctx, data)
	if reader == nil {
		t.Fatal("Reader returned nil")
	}
}

func TestAdaptiveThrottler_Writer(t *testing.T) {
	at := NewAdaptiveThrottler(1000, 100000)
	defer at.Stop()

	var buf bytes.Buffer
	ctx := context.Background()

	writer := at.Writer(ctx, &buf)
	if writer == nil {
		t.Fatal("Writer returned nil")
	}
}

func TestLimitedCopy(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100000,
		BurstSize:      100000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	data := strings.Repeat("x", 5000)
	src := strings.NewReader(data)
	var dst bytes.Buffer

	ctx := context.Background()
	n, err := LimitedCopy(ctx, &dst, src, 3000, throttler)
	if err != nil {
		t.Fatalf("LimitedCopy failed: %v", err)
	}

	if n != 3000 {
		t.Errorf("LimitedCopy copied %d bytes, want 3000", n)
	}

	if dst.Len() != 3000 {
		t.Errorf("Destination has %d bytes, want 3000", dst.Len())
	}
}

func TestLimitedCopy_Context(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100,
		BurstSize:      10,
		MinChunkSize:   10,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	// Consume burst
	throttler.WaitN(context.Background(), 10)

	data := strings.Repeat("x", 5000)
	src := strings.NewReader(data)
	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := LimitedCopy(ctx, &dst, src, 3000, throttler)
	if err != context.DeadlineExceeded {
		t.Errorf("LimitedCopy = %v, want context.DeadlineExceeded", err)
	}
}

func TestProgressReader(t *testing.T) {
	data := strings.Repeat("x", 1000)
	reader := strings.NewReader(data)

	var lastCurrent, lastTotal int64
	var callCount int32

	pr := NewProgressReader(reader, 1000, func(current, total int64) {
		atomic.AddInt32(&callCount, 1)
		lastCurrent = current
		lastTotal = total
	})

	// Read all data
	buf := make([]byte, 100)
	for {
		_, err := pr.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	if lastCurrent != 1000 {
		t.Errorf("lastCurrent = %d, want 1000", lastCurrent)
	}
	if lastTotal != 1000 {
		t.Errorf("lastTotal = %d, want 1000", lastTotal)
	}
	if atomic.LoadInt32(&callCount) == 0 {
		t.Error("Callback was never called")
	}

	current, total := pr.Progress()
	if current != 1000 || total != 1000 {
		t.Errorf("Progress() = (%d, %d), want (1000, 1000)", current, total)
	}
}

func TestThrottler_ConcurrentAccess(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100000,
		BurstSize:      100000,
		MinChunkSize:   100,
		MaxChunkSize:   1000,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				throttler.WaitN(ctx, 100)
			}
		}()
	}

	wg.Wait()
}

func TestThrottledReader_EOF(t *testing.T) {
	config := &Config{
		BytesPerSecond: 100000,
		BurstSize:      100000,
		MinChunkSize:   10,
		MaxChunkSize:   100,
	}

	throttler := NewThrottler(config)
	defer throttler.Stop()

	data := "small"
	reader := strings.NewReader(data)

	ctx := context.Background()
	tr := NewThrottledReader(ctx, reader, throttler)

	buf := make([]byte, 100)
	n, err := tr.Read(buf)
	if err != nil {
		t.Fatalf("First read failed: %v", err)
	}
	if n != 5 {
		t.Errorf("First read = %d bytes, want 5", n)
	}

	n, err = tr.Read(buf)
	if err != io.EOF {
		t.Errorf("Second read = %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("Second read = %d bytes, want 0", n)
	}
}
