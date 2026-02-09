package execution

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestNewQueue(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor)

	if queue == nil {
		t.Fatal("expected non-nil queue")
	}

	if queue.maxConcurrent != 10 {
		t.Errorf("expected default maxConcurrent=10, got %d", queue.maxConcurrent)
	}

	if queue.pending == nil {
		t.Error("expected pending queue to be initialized")
	}

	if queue.active == nil {
		t.Error("expected active map to be initialized")
	}
}

func TestQueueEnqueue(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor)

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	exec, err := queue.Enqueue(rb, nil, PriorityNormal)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if exec.ID == "" {
		t.Error("expected execution ID to be set")
	}

	if exec.Priority != PriorityNormal {
		t.Errorf("expected priority=%d, got %d", PriorityNormal, exec.Priority)
	}

	stats := queue.Stats()
	if stats.PendingCount != 1 {
		t.Errorf("expected PendingCount=1, got %d", stats.PendingCount)
	}
}

func TestQueueEnqueueNilRunbook(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor)

	_, err := queue.Enqueue(nil, nil, PriorityNormal)
	if err == nil {
		t.Error("expected error for nil runbook")
	}
}

func TestQueuePriority(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor)

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	// Enqueue in reverse priority order
	_, _ = queue.Enqueue(rb, nil, PriorityLow)
	_, _ = queue.Enqueue(rb, nil, PriorityNormal)
	_, _ = queue.Enqueue(rb, nil, PriorityCritical)
	_, _ = queue.Enqueue(rb, nil, PriorityHigh)

	// Check that critical comes first
	queue.mu.Lock()
	if queue.pending.Len() != 4 {
		queue.mu.Unlock()
		t.Fatalf("expected 4 pending, got %d", queue.pending.Len())
	}
	first := (*queue.pending)[0]
	queue.mu.Unlock()

	if first.Priority != PriorityCritical {
		t.Errorf("expected first priority=%d, got %d", PriorityCritical, first.Priority)
	}
}

func TestQueueProcessing(t *testing.T) {
	executor := NewExecutor()

	var completed int64
	queue := NewQueue(executor,
		WithMaxConcurrent(2),
		WithQueueCallbacks(
			nil,
			nil,
			func(exec *QueuedExecution) {
				atomic.AddInt64(&completed, 1)
			},
		),
	)

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	// Enqueue multiple
	for i := 0; i < 5; i++ {
		_, err := queue.Enqueue(rb, nil, PriorityNormal)
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	// Start processing
	queue.Start()

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&completed) < 5 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}

	queue.Stop()

	if atomic.LoadInt64(&completed) != 5 {
		t.Errorf("expected 5 completed, got %d", atomic.LoadInt64(&completed))
	}
}

func TestQueueCancel(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor, WithMaxConcurrent(0)) // Don't process

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	exec, _ := queue.Enqueue(rb, nil, PriorityNormal)

	// Cancel
	cancelled := queue.Cancel(exec.ID)
	if !cancelled {
		t.Error("expected cancellation to succeed")
	}

	stats := queue.Stats()
	if stats.PendingCount != 0 {
		t.Errorf("expected PendingCount=0 after cancel, got %d", stats.PendingCount)
	}
}

func TestQueueGetStatus(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor, WithMaxConcurrent(0))

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	exec, _ := queue.Enqueue(rb, nil, PriorityNormal)

	// Get status
	status, found := queue.GetStatus(exec.ID)
	if !found {
		t.Error("expected to find queued execution")
	}

	if status.ID != exec.ID {
		t.Errorf("expected ID=%s, got %s", exec.ID, status.ID)
	}

	// Unknown ID
	_, found = queue.GetStatus("unknown-id")
	if found {
		t.Error("expected not to find unknown execution")
	}
}

func TestQueueConcurrency(t *testing.T) {
	executor := NewExecutor()

	maxConcurrent := 3
	var maxActive int64
	var currentActive int64
	var mu sync.Mutex

	queue := NewQueue(executor,
		WithMaxConcurrent(maxConcurrent),
		WithQueueCallbacks(
			nil,
			func(exec *QueuedExecution) {
				mu.Lock()
				currentActive++
				if currentActive > maxActive {
					maxActive = currentActive
				}
				mu.Unlock()
			},
			func(exec *QueuedExecution) {
				mu.Lock()
				currentActive--
				mu.Unlock()
			},
		),
	)

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	// Enqueue many
	for i := 0; i < 20; i++ {
		_, _ = queue.Enqueue(rb, nil, PriorityNormal)
	}

	queue.Start()

	// Wait for processing
	time.Sleep(2 * time.Second)
	queue.Stop()

	mu.Lock()
	observed := maxActive
	mu.Unlock()

	if observed > int64(maxConcurrent) {
		t.Errorf("max concurrent exceeded: expected <=%d, got %d", maxConcurrent, observed)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(5, 2, 100*time.Millisecond)

	// Should allow 5 requests
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Errorf("expected request %d to be allowed", i)
		}
	}

	// 6th should be denied
	if limiter.Allow() {
		t.Error("expected 6th request to be denied")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should allow 2 more (refill rate)
	if !limiter.Allow() {
		t.Error("expected request after refill to be allowed")
	}
}

func TestRateLimiterWait(t *testing.T) {
	limiter := NewRateLimiter(1, 1, 50*time.Millisecond)

	// Use the first token
	if !limiter.Allow() {
		t.Fatal("expected first request to be allowed")
	}

	// Wait for token
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("expected wait to succeed: %v", err)
	}
}

func TestRateLimiterWaitTimeout(t *testing.T) {
	limiter := NewRateLimiter(1, 1, time.Hour) // Very slow refill

	// Use the first token
	limiter.Allow()

	// Wait with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	if err == nil {
		t.Error("expected wait to timeout")
	}
}

func TestRateLimiterTokens(t *testing.T) {
	limiter := NewRateLimiter(10, 1, 100*time.Millisecond)

	// Use 3 tokens
	limiter.Allow()
	limiter.Allow()
	limiter.Allow()

	if limiter.Tokens() != 7 {
		t.Errorf("expected 7 tokens, got %d", limiter.Tokens())
	}
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Initial state is closed
	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state=closed, got %v", cb.State())
	}

	// Should allow
	if !cb.Allow() {
		t.Error("expected closed circuit to allow")
	}

	// Record failures
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Should be open now
	if cb.State() != CircuitOpen {
		t.Errorf("expected state=open after failures, got %v", cb.State())
	}

	// Should deny
	if cb.Allow() {
		t.Error("expected open circuit to deny")
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected state=open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	if !cb.Allow() {
		t.Error("expected to allow after timeout")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected state=half-open, got %v", cb.State())
	}

	// Record successes
	cb.RecordSuccess()
	cb.RecordSuccess()

	// Should be closed now
	if cb.State() != CircuitClosed {
		t.Errorf("expected state=closed after successes, got %v", cb.State())
	}
}

func TestCircuitBreakerFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 2, 50*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)
	cb.Allow() // Transitions to half-open

	// Fail again
	cb.RecordFailure()

	// Should be open again
	if cb.State() != CircuitOpen {
		t.Errorf("expected state=open after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(1, 1, time.Hour)

	// Open the circuit
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected state=open, got %v", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected state=closed after reset, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("expected to allow after reset")
	}
}

func TestCircuitStateString(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, tt.state.String())
		}
	}
}

func TestPriorityQueue(t *testing.T) {
	pq := &priorityQueue{}

	// Add items in random order
	now := time.Now()
	items := []*QueuedExecution{
		{ID: "low", Priority: PriorityLow, QueuedAt: now},
		{ID: "critical", Priority: PriorityCritical, QueuedAt: now},
		{ID: "normal", Priority: PriorityNormal, QueuedAt: now},
		{ID: "high", Priority: PriorityHigh, QueuedAt: now},
	}

	for _, item := range items {
		pq.Push(item)
	}

	// Fix heap property
	for i := pq.Len()/2 - 1; i >= 0; i-- {
		pq.down(i, pq.Len())
	}

	// Pop should return in priority order
	expected := []string{"critical", "high", "normal", "low"}
	for i, exp := range expected {
		if pq.Len() == 0 {
			t.Fatalf("queue empty before expected at index %d", i)
		}
		// Find min and remove
		minIdx := 0
		for j := 1; j < pq.Len(); j++ {
			if pq.Less(j, minIdx) {
				minIdx = j
			}
		}
		item := (*pq)[minIdx]
		// Swap with last and shrink
		pq.Swap(minIdx, pq.Len()-1)
		*pq = (*pq)[:pq.Len()-1]

		if item.ID != exp {
			t.Errorf("expected %s at position %d, got %s", exp, i, item.ID)
		}
	}
}

// down is a simplified down-heapify helper for testing
func (pq *priorityQueue) down(i, n int) bool {
	i0 := i
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < n && pq.Less(j2, j1) {
			j = j2
		}
		if !pq.Less(j, i) {
			break
		}
		pq.Swap(i, j)
		i = j
	}
	return i > i0
}

func TestEnqueueWithTimeout(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor)

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	exec, err := queue.EnqueueWithTimeout(rb, nil, PriorityNormal, 5*time.Minute)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if exec.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout=5m, got %v", exec.Timeout)
	}
}

func TestQueueStats(t *testing.T) {
	executor := NewExecutor()
	queue := NewQueue(executor, WithMaxConcurrent(1))

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test", Namespace: "default"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	// Enqueue
	_, _ = queue.Enqueue(rb, nil, PriorityNormal)
	_, _ = queue.Enqueue(rb, nil, PriorityNormal)

	stats := queue.Stats()

	if stats.PendingCount != 2 {
		t.Errorf("expected PendingCount=2, got %d", stats.PendingCount)
	}

	if stats.MaxConcurrent != 1 {
		t.Errorf("expected MaxConcurrent=1, got %d", stats.MaxConcurrent)
	}

	if stats.OldestPending == nil {
		t.Error("expected OldestPending to be set")
	}
}
