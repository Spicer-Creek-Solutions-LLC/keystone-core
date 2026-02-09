package ha

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestBandwidthManager_NewBandwidthManager(t *testing.T) {
	bm := NewBandwidthManager(nil)

	if bm.config.MaxConcurrentTransfers != 100 {
		t.Errorf("expected default MaxConcurrentTransfers 100, got %d", bm.config.MaxConcurrentTransfers)
	}

	if bm.config.MaxConcurrentPerAgent != 10 {
		t.Errorf("expected default MaxConcurrentPerAgent 10, got %d", bm.config.MaxConcurrentPerAgent)
	}

	if bm.config.BurstMultiplier != 1.5 {
		t.Errorf("expected default BurstMultiplier 1.5, got %f", bm.config.BurstMultiplier)
	}
}

func TestBandwidthManager_AcquireReleaseTransfer(t *testing.T) {
	config := &BandwidthConfig{
		MaxConcurrentTransfers: 2,
		MaxConcurrentPerAgent:  1,
	}
	bm := NewBandwidthManager(config)

	ctx := context.Background()

	// Acquire first transfer.
	permit1, err := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error: %v", err)
	}

	if bm.stats.TransfersActive != 1 {
		t.Errorf("expected 1 active transfer, got %d", bm.stats.TransfersActive)
	}

	// Acquire second transfer for different agent.
	permit2, err := bm.AcquireTransfer(ctx, "agent-2", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error: %v", err)
	}

	if bm.stats.TransfersActive != 2 {
		t.Errorf("expected 2 active transfers, got %d", bm.stats.TransfersActive)
	}

	// Release first transfer.
	permit1.Release()

	if bm.stats.TransfersActive != 1 {
		t.Errorf("expected 1 active transfer after release, got %d", bm.stats.TransfersActive)
	}

	// Release second transfer.
	permit2.Release()

	if bm.stats.TransfersActive != 0 {
		t.Errorf("expected 0 active transfers after release, got %d", bm.stats.TransfersActive)
	}
}

func TestBandwidthManager_PerAgentLimit(t *testing.T) {
	config := &BandwidthConfig{
		MaxConcurrentTransfers: 10,
		MaxConcurrentPerAgent:  1,
	}
	bm := NewBandwidthManager(config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Acquire first transfer for agent-1.
	permit1, err := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error: %v", err)
	}

	// Try to acquire second transfer for agent-1 - should queue.
	go func() {
		timer := time.NewTimer(50 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		permit1.Release()
	}()

	permit2, err := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error (should have waited): %v", err)
	}
	permit2.Release()
}

func TestBandwidthManager_GlobalLimit(t *testing.T) {
	config := &BandwidthConfig{
		MaxConcurrentTransfers: 1,
		MaxConcurrentPerAgent:  10,
	}
	bm := NewBandwidthManager(config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Acquire first transfer.
	permit1, err := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error: %v", err)
	}

	// Try to acquire second transfer - should queue.
	go func() {
		timer := time.NewTimer(50 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		permit1.Release()
	}()

	permit2, err := bm.AcquireTransfer(ctx, "agent-2", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error (should have waited): %v", err)
	}
	permit2.Release()
}

func TestBandwidthManager_PriorityQueuing(t *testing.T) {
	config := &BandwidthConfig{
		MaxConcurrentTransfers: 1,
	}
	bm := NewBandwidthManager(config)

	ctx := context.Background()

	// Acquire a transfer to block the queue.
	permit, err := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)
	if err != nil {
		t.Fatalf("AcquireTransfer() error: %v", err)
	}

	// Queue a low priority transfer.
	lowCtx, lowCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer lowCancel()
	lowDone := make(chan bool)
	go func() {
		_, err := bm.AcquireTransfer(lowCtx, "agent-2", PriorityLow)
		lowDone <- (err == nil)
	}()

	// Queue a high priority transfer.
	highCtx, highCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer highCancel()
	highDone := make(chan bool)
	go func() {
		_, err := bm.AcquireTransfer(highCtx, "agent-3", PriorityHigh)
		highDone <- (err == nil)
	}()

	// Give time for both to queue.
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 50*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("queue wait did not elapse: %v", err)
	}

	// Release the blocking transfer.
	permit.Release()

	// High priority should complete first.
	select {
	case success := <-highDone:
		if !success {
			t.Error("high priority transfer should have succeeded")
		}
	case <-lowDone:
		t.Error("low priority transfer completed before high priority")
	case <-time.After(200 * time.Millisecond):
		t.Error("timeout waiting for high priority transfer")
	}
}

func TestBandwidthManager_RecordTransfer(t *testing.T) {
	bm := NewBandwidthManager(nil)

	bm.RecordTransfer(1024)
	bm.RecordTransfer(2048)

	stats := bm.GetStats()

	if stats.BytesTransferred != 3072 {
		t.Errorf("expected 3072 bytes transferred, got %d", stats.BytesTransferred)
	}

	if stats.TransfersCompleted != 2 {
		t.Errorf("expected 2 transfers completed, got %d", stats.TransfersCompleted)
	}
}

func TestTokenBucket_TakeMax(t *testing.T) {
	// Create a bucket with 1000 tokens/sec and 2000 burst.
	tb := NewTokenBucket(1000, 2000)

	ctx := context.Background()

	// Should be able to take burst amount immediately.
	if !tb.TakeMax(ctx, 2000) {
		t.Error("expected to take 2000 tokens from full bucket")
	}

	// Should not be able to take more immediately.
	if tb.TakeMax(ctx, 1000) {
		t.Error("expected to fail taking 1000 tokens from empty bucket")
	}

	// Wait for some tokens to refill.
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("refill wait did not elapse: %v", err)
	}

	// Should be able to take some tokens now.
	if !tb.TakeMax(ctx, 50) {
		t.Error("expected to take 50 tokens after refill")
	}
}

func TestRateLimitedReader(t *testing.T) {
	data := []byte("Hello, World!")
	reader := bytes.NewReader(data)

	bm := NewBandwidthManager(nil)
	ctx := context.Background()

	rlr := NewRateLimitedReader(ctx, reader, bm, "agent-1")

	buf := make([]byte, 100)
	n, err := rlr.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Read() error: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected to read %d bytes, got %d", len(data), n)
	}

	if string(buf[:n]) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", string(buf[:n]))
	}

	// Check that transfer was recorded.
	stats := bm.GetStats()
	if stats.BytesTransferred != int64(n) {
		t.Errorf("expected %d bytes recorded, got %d", n, stats.BytesTransferred)
	}
}

func TestRateLimitedWriter(t *testing.T) {
	var buf bytes.Buffer

	bm := NewBandwidthManager(nil)
	ctx := context.Background()

	rlw := NewRateLimitedWriter(ctx, &buf, bm, "agent-1")

	data := []byte("Hello, World!")
	n, err := rlw.Write(data)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected to write %d bytes, got %d", len(data), n)
	}

	if buf.String() != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", buf.String())
	}

	// Check that transfer was recorded.
	stats := bm.GetStats()
	if stats.BytesTransferred != int64(n) {
		t.Errorf("expected %d bytes recorded, got %d", n, stats.BytesTransferred)
	}
}

func TestRateLimitedReader_ContextCancellation(t *testing.T) {
	data := []byte("Hello, World!")
	reader := bytes.NewReader(data)

	bm := NewBandwidthManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	rlr := NewRateLimitedReader(ctx, reader, bm, "agent-1")

	buf := make([]byte, 100)
	_, err := rlr.Read(buf)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestRateLimitedWriter_ContextCancellation(t *testing.T) {
	var buf bytes.Buffer

	bm := NewBandwidthManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	rlw := NewRateLimitedWriter(ctx, &buf, bm, "agent-1")

	data := []byte("Hello, World!")
	_, err := rlw.Write(data)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestBandwidthManager_GlobalRateLimit(t *testing.T) {
	config := &BandwidthConfig{
		GlobalRateLimit: 1000, // 1000 bytes/sec
		BurstMultiplier: 2.0,
	}
	bm := NewBandwidthManager(config)

	ctx := context.Background()

	// Should be able to acquire burst amount.
	err := bm.AcquireBytes(ctx, "agent-1", 2000)
	if err != nil {
		t.Errorf("AcquireBytes() error: %v", err)
	}

	// Should fail to acquire more immediately (bucket empty).
	err = bm.AcquireBytes(ctx, "agent-1", 1000)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}

	if bm.stats.TransfersRateLimited != 1 {
		t.Errorf("expected 1 rate limited, got %d", bm.stats.TransfersRateLimited)
	}
}

func TestBandwidthManager_PerAgentRateLimit(t *testing.T) {
	config := &BandwidthConfig{
		PerAgentRateLimit: 1000, // 1000 bytes/sec per agent
		BurstMultiplier:   2.0,
	}
	bm := NewBandwidthManager(config)

	ctx := context.Background()

	// Agent 1 should be able to acquire burst amount.
	err := bm.AcquireBytes(ctx, "agent-1", 2000)
	if err != nil {
		t.Errorf("AcquireBytes(agent-1) error: %v", err)
	}

	// Agent 2 should also be able to acquire (separate limiter).
	err = bm.AcquireBytes(ctx, "agent-2", 2000)
	if err != nil {
		t.Errorf("AcquireBytes(agent-2) error: %v", err)
	}

	// Agent 1 should fail to acquire more (its bucket empty).
	err = bm.AcquireBytes(ctx, "agent-1", 1000)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited for agent-1, got %v", err)
	}
}

func TestPriority_Values(t *testing.T) {
	if PriorityLow >= PriorityNormal {
		t.Error("PriorityLow should be less than PriorityNormal")
	}
	if PriorityNormal >= PriorityHigh {
		t.Error("PriorityNormal should be less than PriorityHigh")
	}
	if PriorityHigh >= PriorityCritical {
		t.Error("PriorityHigh should be less than PriorityCritical")
	}
}

func TestDefaultBandwidthConfig(t *testing.T) {
	config := DefaultBandwidthConfig()

	if config.GlobalRateLimit != 0 {
		t.Errorf("expected GlobalRateLimit 0, got %d", config.GlobalRateLimit)
	}

	if config.PerAgentRateLimit != 0 {
		t.Errorf("expected PerAgentRateLimit 0, got %d", config.PerAgentRateLimit)
	}

	if config.MaxConcurrentTransfers != 100 {
		t.Errorf("expected MaxConcurrentTransfers 100, got %d", config.MaxConcurrentTransfers)
	}

	if config.MaxConcurrentPerAgent != 10 {
		t.Errorf("expected MaxConcurrentPerAgent 10, got %d", config.MaxConcurrentPerAgent)
	}

	if config.BurstMultiplier != 1.5 {
		t.Errorf("expected BurstMultiplier 1.5, got %f", config.BurstMultiplier)
	}
}

func TestRateLimitedReader_NilBandwidthManager(t *testing.T) {
	data := []byte("Hello, World!")
	reader := bytes.NewReader(data)
	ctx := context.Background()

	// nil bandwidth manager should still work (no rate limiting).
	rlr := NewRateLimitedReader(ctx, reader, nil, "agent-1")

	buf := make([]byte, 100)
	n, err := rlr.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Read() error: %v", err)
	}

	if string(buf[:n]) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", string(buf[:n]))
	}
}

func TestRateLimitedWriter_NilBandwidthManager(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	// nil bandwidth manager should still work (no rate limiting).
	rlw := NewRateLimitedWriter(ctx, &buf, nil, "agent-1")

	data := []byte("Hello, World!")
	n, err := rlw.Write(data)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected to write %d bytes, got %d", len(data), n)
	}
}

func TestBandwidthStats_Rate(t *testing.T) {
	bm := NewBandwidthManager(nil)

	// Record transfers.
	bm.RecordTransfer(1000)

	// Wait a bit.
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 50*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("rate wait did not elapse: %v", err)
	}

	bm.RecordTransfer(1000)

	// Stats should have some data.
	stats := bm.GetStats()

	if stats.BytesTransferred != 2000 {
		t.Errorf("expected 2000 bytes transferred, got %d", stats.BytesTransferred)
	}

	if stats.TransfersCompleted != 2 {
		t.Errorf("expected 2 transfers completed, got %d", stats.TransfersCompleted)
	}
}

func TestBandwidthManager_QueueOverflow(t *testing.T) {
	config := &BandwidthConfig{
		MaxConcurrentTransfers: 1,
	}
	bm := NewBandwidthManager(config)

	ctx := context.Background()

	// Acquire a transfer to block the queue.
	permit, _ := bm.AcquireTransfer(ctx, "agent-1", PriorityNormal)

	// Fill the queue (default size is 1000).
	for i := 0; i < 1000; i++ {
		go func(i int) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			bm.AcquireTransfer(ctx, "agent-overflow", PriorityLow)
		}(i)
	}

	// Give time for goroutines to queue.
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 50*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("queue wait did not elapse: %v", err)
	}

	// Try to add one more - should get rate limited.
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	_, err := bm.AcquireTransfer(ctx2, "agent-overflow", PriorityLow)
	if !errors.Is(err, ErrRateLimited) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected ErrRateLimited or DeadlineExceeded, got %v", err)
	}

	permit.Release()
}

func TestRateLimitedReader_Copy(t *testing.T) {
	// Test that rate limited reader works with io.Copy.
	data := strings.Repeat("Hello, World!", 100)
	reader := strings.NewReader(data)

	bm := NewBandwidthManager(nil)
	ctx := context.Background()

	rlr := NewRateLimitedReader(ctx, reader, bm, "agent-1")

	var buf bytes.Buffer
	n, err := io.Copy(&buf, rlr)
	if err != nil {
		t.Fatalf("Copy() error: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("expected to copy %d bytes, got %d", len(data), n)
	}

	if buf.String() != data {
		t.Error("copied data does not match original")
	}
}
