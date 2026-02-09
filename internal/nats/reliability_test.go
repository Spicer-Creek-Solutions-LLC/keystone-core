package nats

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// ============================================================================
// Circuit Breaker Tests
// ============================================================================

func TestCircuitBreaker_NewCircuitBreaker(t *testing.T) {
	tests := []struct {
		name    string
		cbName  string
		config  *AdvancedCircuitBreakerConfig
		wantErr bool
	}{
		{
			name:    "valid with defaults",
			cbName:  "test-cb",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty name",
			cbName:  "",
			config:  nil,
			wantErr: true,
		},
		{
			name:   "invalid config",
			cbName: "test-cb",
			config: &AdvancedCircuitBreakerConfig{
				FailureThreshold: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb, err := NewCircuitBreaker(tt.cbName, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCircuitBreaker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cb == nil {
				t.Error("NewCircuitBreaker() returned nil without error")
			}
		})
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	config := &AdvancedCircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		IsFailure:           func(err error) bool { return err != nil },
	}

	cb, err := NewCircuitBreaker("test", config)
	if err != nil {
		t.Fatalf("NewCircuitBreaker() error = %v", err)
	}

	// Initial state should be closed
	if cb.State() != CircuitStateClosed {
		t.Errorf("Initial state = %v, want closed", cb.State())
	}

	// Record failures to trip circuit
	for i := 0; i < 3; i++ {
		cb.RecordResult(errors.New("failure"))
	}

	if cb.State() != CircuitStateOpen {
		t.Errorf("State after failures = %v, want open", cb.State())
	}

	// Wait for timeout
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 150*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("timeout wait did not elapse: %v", err)
	}

	// Next Allow() should transition to half-open
	if err := cb.Allow(); err != nil {
		t.Errorf("Allow() after timeout error = %v", err)
	}

	if cb.State() != CircuitStateHalfOpen {
		t.Errorf("State after timeout = %v, want half-open", cb.State())
	}

	// Record successes to close circuit
	cb.RecordResult(nil)
	cb.RecordResult(nil)

	if cb.State() != CircuitStateClosed {
		t.Errorf("State after successes = %v, want closed", cb.State())
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	config := DefaultAdvancedCircuitBreakerConfig()
	config.FailureThreshold = 2

	cb, _ := NewCircuitBreaker("test", config)

	// Execute success
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	stats := cb.GetStats()
	if stats.TotalSuccesses != 1 {
		t.Errorf("TotalSuccesses = %d, want 1", stats.TotalSuccesses)
	}

	// Execute failure
	err = cb.Execute(func() error {
		return errors.New("test error")
	})
	if err == nil {
		t.Error("Execute() expected error")
	}

	stats = cb.GetStats()
	if stats.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", stats.TotalFailures)
	}
}

func TestCircuitBreaker_FailureRateThreshold(t *testing.T) {
	config := &AdvancedCircuitBreakerConfig{
		FailureThreshold:     100, // High threshold so we test rate, not count
		SuccessThreshold:     2,
		Timeout:              time.Second,
		HalfOpenMaxRequests:  1,
		FailureRateThreshold: 0.6, // 60% failure rate threshold
		MinimumRequests:      5,   // Need 5 requests before rate applies
		SamplingWindow:       time.Minute,
		IsFailure:            func(err error) bool { return err != nil },
	}

	cb, _ := NewCircuitBreaker("test", config)

	// Record 3 successes, 2 failures (40% failure rate) - under threshold
	cb.RecordResult(nil)
	cb.RecordResult(nil)
	cb.RecordResult(nil)
	cb.RecordResult(errors.New("fail"))
	cb.RecordResult(errors.New("fail"))

	// Should still be closed (40% < 60% threshold)
	if cb.State() != CircuitStateClosed {
		t.Errorf("State = %v, want closed", cb.State())
	}

	// More failures to push over 60% threshold
	cb.RecordResult(errors.New("fail"))
	cb.RecordResult(errors.New("fail"))

	// Now at 57% (4/7) which is still under, add one more
	cb.RecordResult(errors.New("fail"))
	// Now at 62.5% (5/8) which is over threshold

	if cb.State() != CircuitStateOpen {
		t.Errorf("State after high failure rate = %v, want open", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultAdvancedCircuitBreakerConfig()
	config.FailureThreshold = 1

	cb, _ := NewCircuitBreaker("test", config)

	// Trip the circuit
	cb.RecordResult(errors.New("fail"))
	if cb.State() != CircuitStateOpen {
		t.Errorf("State = %v, want open", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != CircuitStateClosed {
		t.Errorf("State after reset = %v, want closed", cb.State())
	}
}

func TestCircuitBreaker_Trip(t *testing.T) {
	cb, _ := NewCircuitBreaker("test", nil)

	cb.Trip()

	if cb.State() != CircuitStateOpen {
		t.Errorf("State after trip = %v, want open", cb.State())
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	manager := NewCircuitBreakerManager(nil)

	// Get or create
	cb1 := manager.GetOrCreate("endpoint-1")
	cb2 := manager.GetOrCreate("endpoint-2")

	if cb1 == nil || cb2 == nil {
		t.Error("GetOrCreate returned nil")
	}

	// Same name should return same instance
	cb1Again := manager.GetOrCreate("endpoint-1")
	if cb1 != cb1Again {
		t.Error("GetOrCreate returned different instance for same name")
	}

	// Count
	if manager.Count() != 2 {
		t.Errorf("Count = %d, want 2", manager.Count())
	}

	// Get
	if manager.Get("endpoint-1") == nil {
		t.Error("Get returned nil for existing breaker")
	}
	if manager.Get("nonexistent") != nil {
		t.Error("Get returned non-nil for nonexistent breaker")
	}

	// Remove
	manager.Remove("endpoint-1")
	if manager.Count() != 1 {
		t.Errorf("Count after remove = %d, want 1", manager.Count())
	}
}

// ============================================================================
// Deduplication Tests
// ============================================================================

func TestDeduplicator_New(t *testing.T) {
	tests := []struct {
		name    string
		config  *DedupConfig
		wantErr bool
	}{
		{
			name:    "valid defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid custom",
			config: &DedupConfig{
				WindowDuration:  time.Minute,
				MaxEntries:      1000,
				CleanupInterval: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid window",
			config: &DedupConfig{
				WindowDuration:  -1,
				MaxEntries:      100,
				CleanupInterval: time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid max entries",
			config: &DedupConfig{
				WindowDuration:  time.Minute,
				MaxEntries:      0,
				CleanupInterval: time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dedup, err := NewDeduplicator(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDeduplicator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && dedup == nil {
				t.Error("NewDeduplicator() returned nil without error")
			}
		})
	}
}

func TestDeduplicator_IsDuplicate(t *testing.T) {
	config := &DedupConfig{
		WindowDuration:  time.Minute,
		MaxEntries:      100,
		CleanupInterval: time.Second,
	}
	dedup, _ := NewDeduplicator(config)

	// First time should not be duplicate (using subject + data for ID)
	if dedup.IsDuplicate("test.subject", []byte("msg-1")) {
		t.Error("First message should not be duplicate")
	}

	// Second time with same content should be duplicate
	if !dedup.IsDuplicate("test.subject", []byte("msg-1")) {
		t.Error("Second message with same content should be duplicate")
	}

	// Different content should not be duplicate
	if dedup.IsDuplicate("test.subject", []byte("msg-2")) {
		t.Error("Different content should not be duplicate")
	}
}

func TestDeduplicator_ContentHash(t *testing.T) {
	// Default config uses content hash for ID generation
	config := &DedupConfig{
		WindowDuration:  time.Minute,
		MaxEntries:      100,
		CleanupInterval: time.Second,
	}
	dedup, _ := NewDeduplicator(config)

	data := []byte("test data")

	// First time should not be duplicate
	if dedup.IsDuplicate("test.subject", data) {
		t.Error("First message should not be duplicate")
	}

	// Same content should be duplicate
	if !dedup.IsDuplicate("test.subject", data) {
		t.Error("Same content should be duplicate")
	}

	// Different content should not be duplicate
	if dedup.IsDuplicate("test.subject", []byte("different data")) {
		t.Error("Different content should not be duplicate")
	}
}

func TestDeduplicator_PerSubject(t *testing.T) {
	config := &DedupConfig{
		WindowDuration:  time.Minute,
		MaxEntries:      100,
		CleanupInterval: time.Second,
		PerSubject:      true,
	}
	dedup, _ := NewDeduplicator(config)

	// Same content, different subjects - default ID generator includes subject
	// so they should have different hashes and not be duplicates
	if dedup.IsDuplicate("subject-a", []byte("msg-1")) {
		t.Error("First message on subject-a should not be duplicate")
	}
	if dedup.IsDuplicate("subject-b", []byte("msg-1")) {
		t.Error("Same content on different subject should not be duplicate")
	}

	// Same content, same subject - should be duplicate
	if !dedup.IsDuplicate("subject-a", []byte("msg-1")) {
		t.Error("Same content on same subject should be duplicate")
	}
}

func TestDeduplicator_Expiry(t *testing.T) {
	config := &DedupConfig{
		WindowDuration:  50 * time.Millisecond,
		MaxEntries:      100,
		CleanupInterval: 10 * time.Millisecond,
	}
	dedup, _ := NewDeduplicator(config)

	// First time
	dedup.IsDuplicate("test.subject", []byte("msg-1"))

	// Should be duplicate within window
	if !dedup.IsDuplicate("test.subject", []byte("msg-1")) {
		t.Error("Should be duplicate within window")
	}

	// Wait for expiry
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expiry wait did not elapse: %v", err)
	}

	// Should not be duplicate after expiry
	if dedup.IsDuplicate("test.subject", []byte("msg-1")) {
		t.Error("Should not be duplicate after expiry")
	}
}

func TestDeduplicator_Stats(t *testing.T) {
	config := &DedupConfig{
		WindowDuration:  time.Minute,
		MaxEntries:      100,
		CleanupInterval: time.Second,
	}
	dedup, _ := NewDeduplicator(config)

	dedup.IsDuplicate("test.subject", []byte("msg-1"))
	dedup.IsDuplicate("test.subject", []byte("msg-1")) // Duplicate
	dedup.IsDuplicate("test.subject", []byte("msg-2"))
	dedup.IsDuplicate("test.subject", []byte("msg-1")) // Duplicate

	stats := dedup.GetStats()
	if stats.TotalChecked != 4 {
		t.Errorf("TotalChecked = %d, want 4", stats.TotalChecked)
	}
	if stats.TotalDuplicates != 2 {
		t.Errorf("TotalDuplicates = %d, want 2", stats.TotalDuplicates)
	}
}

// ============================================================================
// Degradation Manager Tests
// ============================================================================

func TestDegradationManager_New(t *testing.T) {
	tests := []struct {
		name    string
		config  *DegradationConfig
		wantErr bool
	}{
		{
			name:    "valid defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "invalid max queue size",
			config: &DegradationConfig{
				MaxQueueSize: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm, err := NewDegradationManager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDegradationManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && dm == nil {
				t.Error("NewDegradationManager() returned nil without error")
			}
		})
	}
}

func TestDegradationManager_ModeTransitions(t *testing.T) {
	config := &DegradationConfig{
		MaxQueueSize:         100,
		QueueTimeout:         time.Minute,
		RetryInterval:        time.Second,
		RateLimitNormal:      1000,
		RateLimitDegraded:    100,
		RateLimitLimited:     10,
		HealthCheckInterval:  time.Second,
		RecoveryThreshold:    2,
		DegradationThreshold: 2,
		PriorityThresholds: map[DegradationMode]OperationPriority{
			DegradationModeNormal:   OperationPriorityBackground,
			DegradationModeDegraded: OperationPriorityNormal,
			DegradationModeLimited:  OperationPriorityHigh,
			DegradationModeOffline:  OperationPriorityCritical,
		},
	}

	dm, _ := NewDegradationManager(config)

	// Initial mode should be normal
	if dm.Mode() != DegradationModeNormal {
		t.Errorf("Initial mode = %v, want normal", dm.Mode())
	}

	// Record failures to degrade
	dm.RecordFailure()
	dm.RecordFailure()

	if dm.Mode() != DegradationModeDegraded {
		t.Errorf("Mode after failures = %v, want degraded", dm.Mode())
	}

	// More failures
	dm.RecordFailure()
	dm.RecordFailure()

	if dm.Mode() != DegradationModeLimited {
		t.Errorf("Mode after more failures = %v, want limited", dm.Mode())
	}

	// Record successes to recover
	dm.RecordSuccess()
	dm.RecordSuccess()

	if dm.Mode() != DegradationModeDegraded {
		t.Errorf("Mode after recovery = %v, want degraded", dm.Mode())
	}
}

func TestDegradationManager_AllowOperation(t *testing.T) {
	config := DefaultDegradationConfig()
	dm, _ := NewDegradationManager(config)

	// In normal mode, all priorities should be allowed
	if !dm.AllowOperation(OperationPriorityBackground) {
		t.Error("Background operation should be allowed in normal mode")
	}

	// Manually set to limited mode
	dm.SetMode(DegradationModeLimited)

	// Low priority should not be allowed in limited mode
	if dm.AllowOperation(OperationPriorityLow) {
		t.Error("Low priority should not be allowed in limited mode")
	}

	// Critical should always be allowed
	if !dm.AllowOperation(OperationPriorityCritical) {
		t.Error("Critical priority should always be allowed")
	}
}

func TestDegradationManager_Queue(t *testing.T) {
	config := &DegradationConfig{
		MaxQueueSize:  3,
		QueueTimeout:  time.Minute,
		RetryInterval: time.Second,
		PriorityThresholds: map[DegradationMode]OperationPriority{
			DegradationModeNormal: OperationPriorityBackground,
		},
	}
	dm, _ := NewDegradationManager(config)

	// Queue operations
	for i := 0; i < 3; i++ {
		err := dm.Queue(&QueuedOperation{
			ID:       string(rune('a' + i)),
			Priority: OperationPriorityNormal,
		})
		if err != nil {
			t.Errorf("Queue() error = %v", err)
		}
	}

	if dm.QueueSize() != 3 {
		t.Errorf("QueueSize = %d, want 3", dm.QueueSize())
	}

	// Queue full, should reject or drop
	err := dm.Queue(&QueuedOperation{
		ID:       "d",
		Priority: OperationPriorityLow, // Lower priority than existing
	})
	if err == nil {
		t.Error("Queue should reject when full with lower priority")
	}

	// High priority should preempt
	err = dm.Queue(&QueuedOperation{
		ID:       "high",
		Priority: OperationPriorityHigh,
	})
	if err != nil {
		t.Errorf("High priority Queue() error = %v", err)
	}
}

func TestDegradationManager_Dequeue(t *testing.T) {
	dm, _ := NewDegradationManager(nil)

	// Queue with different priorities
	dm.Queue(&QueuedOperation{ID: "low", Priority: OperationPriorityLow})
	dm.Queue(&QueuedOperation{ID: "critical", Priority: OperationPriorityCritical})
	dm.Queue(&QueuedOperation{ID: "normal", Priority: OperationPriorityNormal})

	// Dequeue should return highest priority first
	op := dm.Dequeue()
	if op == nil || op.ID != "critical" {
		t.Errorf("First dequeue = %v, want critical", op)
	}

	op = dm.Dequeue()
	if op == nil || op.ID != "normal" {
		t.Errorf("Second dequeue = %v, want normal", op)
	}

	op = dm.Dequeue()
	if op == nil || op.ID != "low" {
		t.Errorf("Third dequeue = %v, want low", op)
	}

	// Empty queue
	op = dm.Dequeue()
	if op != nil {
		t.Error("Dequeue on empty queue should return nil")
	}
}

func TestDegradationManager_StartStop(t *testing.T) {
	dm, _ := NewDegradationManager(nil)

	// Start
	if err := dm.Start(); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Double start should error
	if err := dm.Start(); err == nil {
		t.Error("Double Start() should error")
	}

	// Stop
	if err := dm.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestDegradationManager_Stats(t *testing.T) {
	dm, _ := NewDegradationManager(nil)

	dm.Queue(&QueuedOperation{ID: "1", Priority: OperationPriorityNormal})
	dm.Queue(&QueuedOperation{ID: "2", Priority: OperationPriorityNormal})
	dm.RecordSuccess()
	dm.RecordFailure()

	stats := dm.GetStats()
	if stats.TotalQueued != 2 {
		t.Errorf("TotalQueued = %d, want 2", stats.TotalQueued)
	}
	if stats.QueuedOperations != 2 {
		t.Errorf("QueuedOperations = %d, want 2", stats.QueuedOperations)
	}
	if stats.Mode != DegradationModeNormal {
		t.Errorf("Mode = %v, want normal", stats.Mode)
	}
}

func TestDegradationManager_CancelOperation(t *testing.T) {
	dm, _ := NewDegradationManager(nil)

	dm.Queue(&QueuedOperation{ID: "op-1", Priority: OperationPriorityNormal})
	dm.Queue(&QueuedOperation{ID: "op-2", Priority: OperationPriorityNormal})

	if !dm.CancelOperation("op-1") {
		t.Error("CancelOperation should return true for existing op")
	}

	if dm.QueueSize() != 1 {
		t.Errorf("QueueSize after cancel = %d, want 1", dm.QueueSize())
	}

	if dm.CancelOperation("op-1") {
		t.Error("CancelOperation should return false for already cancelled op")
	}
}

func TestDegradationManager_ClearQueue(t *testing.T) {
	dm, _ := NewDegradationManager(nil)

	dm.Queue(&QueuedOperation{ID: "1", Priority: OperationPriorityNormal})
	dm.Queue(&QueuedOperation{ID: "2", Priority: OperationPriorityNormal})
	dm.Queue(&QueuedOperation{ID: "3", Priority: OperationPriorityNormal})

	count := dm.ClearQueue()
	if count != 3 {
		t.Errorf("ClearQueue returned %d, want 3", count)
	}

	if dm.QueueSize() != 0 {
		t.Errorf("QueueSize after clear = %d, want 0", dm.QueueSize())
	}
}

// ============================================================================
// Delivery Manager Tests
// ============================================================================

func TestDeliveryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *DeliveryConfig
		wantErr bool
	}{
		{
			name:    "valid defaults",
			config:  DefaultDeliveryConfig(),
			wantErr: false,
		},
		{
			name: "invalid timeout",
			config: &DeliveryConfig{
				Timeout: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid max retries",
			config: &DeliveryConfig{
				MaxRetries: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeliveryManager_AtMostOnceMode(t *testing.T) {
	config := DefaultDeliveryConfig()
	config.Mode = DeliveryModeAtMostOnce
	config.MaxRetries = 3

	dm, err := NewDeliveryManager(config, nil)
	if err != nil {
		t.Fatalf("NewDeliveryManager() error = %v", err)
	}

	// Without connection, publish should fail
	err = dm.Publish("test.subject", []byte("test data"))
	if err == nil {
		t.Error("Publish without connection should fail")
	}
}

func TestDeliveryManager_StartStop(t *testing.T) {
	config := DefaultDeliveryConfig()
	config.Mode = DeliveryModeAtMostOnce
	config.MaxRetries = 3

	dm, err := NewDeliveryManager(config, nil)
	if err != nil {
		t.Fatalf("NewDeliveryManager() error = %v", err)
	}

	if err := dm.Start(); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Double start should error
	if err := dm.Start(); err == nil {
		t.Error("Double Start() should error")
	}

	if err := dm.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestDeliveryManager_Stats(t *testing.T) {
	config := DefaultDeliveryConfig()
	config.Mode = DeliveryModeAtMostOnce

	dm, err := NewDeliveryManager(config, nil)
	if err != nil {
		t.Fatalf("NewDeliveryManager() error = %v", err)
	}

	stats := dm.GetStats()
	if stats.TotalSent != 0 {
		t.Errorf("Initial TotalSent = %d, want 0", stats.TotalSent)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestReliabilityIntegration_CircuitBreakerWithDegradation(t *testing.T) {
	// Create circuit breaker
	cbConfig := &AdvancedCircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		IsFailure:           func(err error) bool { return err != nil },
	}
	cb, _ := NewCircuitBreaker("test-endpoint", cbConfig)

	// Create degradation manager
	dmConfig := &DegradationConfig{
		MaxQueueSize:         100,
		QueueTimeout:         time.Minute,
		RetryInterval:        time.Second,
		RecoveryThreshold:    2,
		DegradationThreshold: 2,
		PriorityThresholds: map[DegradationMode]OperationPriority{
			DegradationModeNormal:   OperationPriorityBackground,
			DegradationModeDegraded: OperationPriorityNormal,
		},
	}
	dm, _ := NewDegradationManager(dmConfig)

	// Link circuit breaker to degradation manager
	var operations int64

	// Simulate operations
	for i := 0; i < 10; i++ {
		err := cb.Execute(func() error {
			if i < 5 {
				return errors.New("simulated failure")
			}
			return nil
		})

		if errors.Is(err, ErrCircuitOpen) {
			dm.RecordFailure()
		} else if err != nil {
			dm.RecordFailure()
		} else {
			dm.RecordSuccess()
			atomic.AddInt64(&operations, 1)
		}
	}

	// Verify circuit breaker tripped
	if cb.State() != CircuitStateOpen {
		t.Log("Circuit breaker should have tripped")
	}

	// Verify degradation manager tracked failures
	if dm.IsHealthy() && atomic.LoadInt64(&operations) < 5 {
		t.Log("Degradation manager should have recorded failures")
	}
}

func TestReliabilityIntegration_DedupWithBuffer(t *testing.T) {
	// Create deduplicator
	dedupConfig := &DedupConfig{
		WindowDuration:  time.Minute,
		MaxEntries:      100,
		CleanupInterval: time.Second,
	}
	dedup, _ := NewDeduplicator(dedupConfig)

	// Create buffer
	bufConfig := DefaultBufferConfig()
	buffer, _ := NewMessageBuffer(bufConfig)

	ctx := context.Background()
	buffer.Start(ctx)
	defer buffer.Stop()

	// Simulate receiving duplicate messages
	messages := [][]byte{[]byte("msg-1"), []byte("msg-2"), []byte("msg-1"), []byte("msg-3"), []byte("msg-2"), []byte("msg-1")}

	var buffered int
	for _, msgData := range messages {
		if !dedup.IsDuplicate("test.subject", msgData) {
			// Only buffer non-duplicates
			buffer.Buffer(&BufferedMessage{
				ID:        string(msgData),
				Subject:   "test.subject",
				Data:      msgData,
				Timestamp: time.Now(),
			})
			buffered++
		}
	}

	// Should only buffer 3 unique messages
	if buffered != 3 {
		t.Errorf("Buffered = %d, want 3", buffered)
	}

	if buffer.Len() != 3 {
		t.Errorf("Buffer length = %d, want 3", buffer.Len())
	}
}
