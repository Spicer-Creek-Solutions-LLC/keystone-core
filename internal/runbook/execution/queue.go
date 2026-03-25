package execution

import (
	"container/heap"
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Queue manages queued runbook executions with priority support.
type Queue struct {
	mu sync.RWMutex

	// Priority queue for pending executions
	pending *priorityQueue

	// Active executions
	active map[string]*QueuedExecution

	// Configuration
	maxConcurrent  int
	defaultTimeout time.Duration

	// Executor
	executor *Executor

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup

	// Callbacks
	onQueued    func(exec *QueuedExecution)
	onStarted   func(exec *QueuedExecution)
	onCompleted func(exec *QueuedExecution)
}

// QueuedExecution represents an execution in the queue.
type QueuedExecution struct {
	ID          string
	Runbook     *runbook.Runbook
	Inputs      map[string]interface{}
	Priority    int
	QueuedAt    time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Timeout     time.Duration
	Result      *runbook.Execution
	Error       error

	// For priority queue
	index int
}

// Priority levels
const (
	PriorityCritical = 0
	PriorityHigh     = 10
	PriorityNormal   = 50
	PriorityLow      = 100
)

// QueueOption configures the Queue.
type QueueOption func(*Queue)

// WithMaxConcurrent sets the maximum number of concurrent executions.
func WithMaxConcurrent(n int) QueueOption {
	return func(q *Queue) {
		if n > 0 {
			q.maxConcurrent = n
		}
	}
}

// WithDefaultQueueTimeout sets the default timeout for queued executions.
func WithDefaultQueueTimeout(d time.Duration) QueueOption {
	return func(q *Queue) {
		q.defaultTimeout = d
	}
}

// WithQueueCallbacks sets queue lifecycle callbacks.
func WithQueueCallbacks(
	onQueued func(*QueuedExecution),
	onStarted func(*QueuedExecution),
	onCompleted func(*QueuedExecution),
) QueueOption {
	return func(q *Queue) {
		q.onQueued = onQueued
		q.onStarted = onStarted
		q.onCompleted = onCompleted
	}
}

// NewQueue creates a new execution queue.
func NewQueue(executor *Executor, opts ...QueueOption) *Queue {
	ctx, cancel := context.WithCancel(context.Background())

	q := &Queue{
		pending:        &priorityQueue{},
		active:         make(map[string]*QueuedExecution),
		maxConcurrent:  10,
		defaultTimeout: 30 * time.Minute,
		executor:       executor,
		ctx:            ctx,
		cancel:         cancel,
	}

	heap.Init(q.pending)

	for _, opt := range opts {
		opt(q)
	}

	return q
}

// Start begins processing the queue.
func (q *Queue) Start() {
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return
	}
	q.started = true
	q.mu.Unlock()

	q.wg.Add(1)
	go q.processLoop()
}

// Stop stops the queue processor.
func (q *Queue) Stop() {
	q.cancel()
	q.wg.Wait()

	q.mu.Lock()
	q.started = false
	q.mu.Unlock()
}

// Enqueue adds an execution to the queue.
func (q *Queue) Enqueue(rb *runbook.Runbook, inputs map[string]interface{}, priority int) (*QueuedExecution, error) {
	if rb == nil {
		return nil, errors.New("runbook is required")
	}

	exec := &QueuedExecution{
		ID:       generateID(),
		Runbook:  rb,
		Inputs:   inputs,
		Priority: priority,
		QueuedAt: time.Now(),
		Timeout:  q.defaultTimeout,
	}

	q.mu.Lock()
	heap.Push(q.pending, exec)
	q.mu.Unlock()

	if q.onQueued != nil {
		q.onQueued(exec)
	}

	return exec, nil
}

// EnqueueWithTimeout adds an execution with a custom timeout.
func (q *Queue) EnqueueWithTimeout(rb *runbook.Runbook, inputs map[string]interface{}, priority int, timeout time.Duration) (*QueuedExecution, error) {
	exec, err := q.Enqueue(rb, inputs, priority)
	if err != nil {
		return nil, err
	}
	exec.Timeout = timeout
	return exec, nil
}

// Cancel cancels a queued or running execution.
func (q *Queue) Cancel(executionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if it's in the pending queue
	for i := 0; i < q.pending.Len(); i++ {
		if (*q.pending)[i].ID == executionID {
			heap.Remove(q.pending, i)
			return true
		}
	}

	// Check if it's active
	if exec, ok := q.active[executionID]; ok {
		exec.Error = context.Canceled
		// The actual cancellation happens in the execution goroutine
		return true
	}

	return false
}

// GetStatus returns the status of a queued execution.
func (q *Queue) GetStatus(executionID string) (*QueuedExecution, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check active
	if exec, ok := q.active[executionID]; ok {
		return exec, true
	}

	// Check pending
	for i := 0; i < q.pending.Len(); i++ {
		if (*q.pending)[i].ID == executionID {
			return (*q.pending)[i], true
		}
	}

	return nil, false
}

// QueueStats contains queue statistics.
type QueueStats struct {
	PendingCount  int
	ActiveCount   int
	MaxConcurrent int
	OldestPending *time.Time
}

// Stats returns current queue statistics.
func (q *Queue) Stats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := QueueStats{
		PendingCount:  q.pending.Len(),
		ActiveCount:   len(q.active),
		MaxConcurrent: q.maxConcurrent,
	}

	if q.pending.Len() > 0 {
		oldest := (*q.pending)[0].QueuedAt
		stats.OldestPending = &oldest
	}

	return stats
}

// processLoop continuously processes queued executions.
func (q *Queue) processLoop() {
	defer q.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.processNext()
		}
	}
}

// processNext starts the next execution if capacity allows.
func (q *Queue) processNext() {
	q.mu.Lock()

	// Check if we have capacity
	if len(q.active) >= q.maxConcurrent {
		q.mu.Unlock()
		return
	}

	// Check if there are pending executions
	if q.pending.Len() == 0 {
		q.mu.Unlock()
		return
	}

	// Pop the highest priority execution
	exec := heap.Pop(q.pending).(*QueuedExecution)
	now := time.Now()
	exec.StartedAt = &now
	q.active[exec.ID] = exec
	q.mu.Unlock()

	if q.onStarted != nil {
		q.onStarted(exec)
	}

	// Execute in goroutine
	q.wg.Add(1)
	go q.executeRunbook(exec)
}

// executeRunbook runs a queued execution.
func (q *Queue) executeRunbook(exec *QueuedExecution) {
	defer q.wg.Done()

	// Create timeout context
	ctx := q.ctx
	if exec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, exec.Timeout)
		defer cancel()
	}

	// Execute
	result, err := q.executor.Execute(ctx, exec.Runbook, exec.Inputs)

	// Update execution
	q.mu.Lock()
	now := time.Now()
	exec.CompletedAt = &now
	exec.Result = result
	exec.Error = err
	delete(q.active, exec.ID)
	q.mu.Unlock()

	if q.onCompleted != nil {
		q.onCompleted(exec)
	}
}

// generateID generates a unique execution ID.
func generateID() string {
	// Simple ID generation using timestamp + random
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString generates a cryptographically random string of specified length.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen; crypto/rand reads from OS entropy.
		panic("crypto/rand failed: " + err.Error())
	}
	for i := range b {
		b[i] = letters[b[i]%byte(len(letters))]
	}
	return string(b)
}

// priorityQueue implements heap.Interface for QueuedExecution.
type priorityQueue []*QueuedExecution

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Lower priority value = higher priority
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	// If same priority, earlier queued time wins
	return pq[i].QueuedAt.Before(pq[j].QueuedAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueuedExecution)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// RateLimiter provides rate limiting for execution requests.
type RateLimiter struct {
	mu sync.Mutex

	// Token bucket
	tokens       int
	maxTokens    int
	refillRate   int
	lastRefill   time.Time
	refillPeriod time.Duration
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxTokens, refillRate int, refillPeriod time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:       maxTokens,
		maxTokens:    maxTokens,
		refillRate:   refillRate,
		lastRefill:   time.Now(),
		refillPeriod: refillPeriod,
	}
}

// Allow checks if a request is allowed.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refill()

	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// Wait blocks until a token is available or context is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if r.Allow() {
				return nil
			}
		}
	}
}

// refill adds tokens based on elapsed time.
func (r *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)

	if elapsed >= r.refillPeriod {
		periods := int(elapsed / r.refillPeriod)
		r.tokens += periods * r.refillRate
		if r.tokens > r.maxTokens {
			r.tokens = r.maxTokens
		}
		r.lastRefill = r.lastRefill.Add(time.Duration(periods) * r.refillPeriod)
	}
}

// Tokens returns the current number of available tokens.
func (r *RateLimiter) Tokens() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refill()
	return r.tokens
}

// CircuitBreaker provides circuit breaker functionality for execution.
type CircuitBreaker struct {
	mu sync.RWMutex

	// State
	state       CircuitState
	failures    int
	successes   int
	lastFailure time.Time

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

// CircuitClosed constants define the circuit states.
const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

// Allow checks if an execution is allowed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailure) >= cb.timeout {
			cb.state = CircuitHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful execution.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = CircuitClosed
			cb.failures = 0
		}
	case CircuitClosed:
		cb.failures = 0
	default:
	}
}

// RecordFailure records a failed execution.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.failureThreshold {
			cb.state = CircuitOpen
		}
	case CircuitHalfOpen:
		cb.state = CircuitOpen
	default:
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
}
