package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Graceful Degradation - T8.5
// ============================================================================

// DegradationMode represents the current degradation mode
type DegradationMode string

const (
	// DegradationModeNormal means normal operation
	DegradationModeNormal DegradationMode = "normal"
	// DegradationModeDegraded means reduced functionality
	DegradationModeDegraded DegradationMode = "degraded"
	// DegradationModeLimited means severely limited functionality
	DegradationModeLimited DegradationMode = "limited"
	// DegradationModeOffline means offline mode (queueing only)
	DegradationModeOffline DegradationMode = "offline"
)

// OperationPriority represents operation priority for degradation management
// Note: Lower values = higher priority (0 = critical, 4 = background)
type OperationPriority int

const (
	// OperationPriorityCritical for critical operations (always executed)
	OperationPriorityCritical OperationPriority = 0
	// OperationPriorityHigh for high priority operations
	OperationPriorityHigh OperationPriority = 1
	// OperationPriorityNormal for normal operations
	OperationPriorityNormal OperationPriority = 2
	// OperationPriorityLow for low priority operations
	OperationPriorityLow OperationPriority = 3
	// OperationPriorityBackground for background operations
	OperationPriorityBackground OperationPriority = 4
)

// QueuedOperation represents a queued operation during degradation
type QueuedOperation struct {
	// ID is the operation ID
	ID string

	// Type is the operation type
	Type string

	// AgentID is the target agent (if applicable)
	AgentID string

	// Subject is the NATS subject
	Subject string

	// Data is the operation data
	Data []byte

	// Priority is the operation priority
	Priority OperationPriority

	// CreatedAt is when the operation was queued
	CreatedAt time.Time

	// Deadline is the operation deadline (zero = no deadline)
	Deadline time.Time

	// Attempts is the number of retry attempts
	Attempts int

	// Callback is called when operation completes
	Callback func(error)
}

// IsExpired returns true if the operation has expired
func (o *QueuedOperation) IsExpired() bool {
	if o.Deadline.IsZero() {
		return false
	}
	return time.Now().After(o.Deadline)
}

// DegradationConfig holds degradation configuration
type DegradationConfig struct {
	// MaxQueueSize is the maximum queued operations
	MaxQueueSize int

	// QueueTimeout is how long operations can stay queued
	QueueTimeout time.Duration

	// RetryInterval is the retry interval for queued operations
	RetryInterval time.Duration

	// MaxRetries is maximum retries per operation
	MaxRetries int

	// RateLimitNormal is the rate limit in normal mode
	RateLimitNormal float64

	// RateLimitDegraded is the rate limit in degraded mode
	RateLimitDegraded float64

	// RateLimitLimited is the rate limit in limited mode
	RateLimitLimited float64

	// HealthCheckInterval is how often to check health
	HealthCheckInterval time.Duration

	// RecoveryThreshold is successes needed to improve mode
	RecoveryThreshold int

	// DegradationThreshold is failures needed to degrade mode
	DegradationThreshold int

	// PriorityThresholds defines what priorities are allowed per mode
	PriorityThresholds map[DegradationMode]OperationPriority
}

// DefaultDegradationConfig returns sensible defaults
func DefaultDegradationConfig() *DegradationConfig {
	return &DegradationConfig{
		MaxQueueSize:         10000,
		QueueTimeout:         5 * time.Minute,
		RetryInterval:        5 * time.Second,
		MaxRetries:           10,
		RateLimitNormal:      1000, // ops/sec
		RateLimitDegraded:    100,
		RateLimitLimited:     10,
		HealthCheckInterval:  10 * time.Second,
		RecoveryThreshold:    5,
		DegradationThreshold: 3,
		PriorityThresholds: map[DegradationMode]OperationPriority{
			DegradationModeNormal:   OperationPriorityBackground, // All allowed
			DegradationModeDegraded: OperationPriorityNormal,     // Normal and above
			DegradationModeLimited:  OperationPriorityHigh,       // High and above
			DegradationModeOffline:  OperationPriorityCritical,   // Critical only
		},
	}
}

// Validate validates the configuration
func (c *DegradationConfig) Validate() error {
	if c.MaxQueueSize <= 0 {
		return errors.New("max queue size must be positive")
	}
	if c.QueueTimeout <= 0 {
		return errors.New("queue timeout must be positive")
	}
	if c.RetryInterval <= 0 {
		return errors.New("retry interval must be positive")
	}
	return nil
}

// DegradationStats holds degradation statistics
type DegradationStats struct {
	// Mode is the current mode
	Mode DegradationMode

	// QueuedOperations is current queued count
	QueuedOperations int64

	// TotalQueued is total operations queued
	TotalQueued int64

	// TotalProcessed is total operations processed from queue
	TotalProcessed int64

	// TotalDropped is total operations dropped
	TotalDropped int64

	// TotalExpired is total operations expired
	TotalExpired int64

	// TotalRetried is total retry attempts
	TotalRetried int64

	// ConsecutiveSuccesses is current success streak
	ConsecutiveSuccesses int64

	// ConsecutiveFailures is current failure streak
	ConsecutiveFailures int64

	// CurrentRateLimit is the current rate limit
	CurrentRateLimit float64

	// LastModeChange is when mode last changed
	LastModeChange time.Time

	// TimeInMode is time spent in current mode
	TimeInMode time.Duration
}

// DegradationManager manages graceful degradation
type DegradationManager struct {
	config *DegradationConfig

	// Current mode
	mode   DegradationMode
	modeMu sync.RWMutex

	// Operation queue
	queue   []*QueuedOperation
	queueMu sync.Mutex

	// Rate limiting
	rateLimiter *rateLimiter

	// Health tracking
	consecutiveSuccesses int64
	consecutiveFailures  int64
	lastModeChange       time.Time

	// Statistics
	stats struct {
		totalQueued    int64
		totalProcessed int64
		totalDropped   int64
		totalExpired   int64
		totalRetried   int64
	}

	// Health checker
	healthCheck func() error

	// Callbacks
	onModeChange func(from, to DegradationMode)
	onDrop       func(op *QueuedOperation, reason string)

	// Lifecycle
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// rateLimiter implements token bucket rate limiting
type rateLimiter struct {
	rate      float64
	tokens    float64
	maxTokens float64
	lastTime  time.Time
	mu        sync.Mutex
}

func newRateLimiter(rate float64) *rateLimiter {
	return &rateLimiter{
		rate:      rate,
		tokens:    rate, // Start full
		maxTokens: rate,
		lastTime:  time.Now(),
	}
}

func (rl *rateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastTime = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) SetRate(rate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
	rl.maxTokens = rate
}

// NewDegradationManager creates a new degradation manager
func NewDegradationManager(config *DegradationConfig) (*DegradationManager, error) {
	if config == nil {
		config = DefaultDegradationConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dm := &DegradationManager{
		config:         config,
		mode:           DegradationModeNormal,
		queue:          make([]*QueuedOperation, 0, config.MaxQueueSize),
		rateLimiter:    newRateLimiter(config.RateLimitNormal),
		lastModeChange: time.Now(),
		ctx:            ctx,
		cancel:         cancel,
	}

	return dm, nil
}

// Start starts the degradation manager
func (dm *DegradationManager) Start() error {
	if dm.running.Load() {
		return errors.New("already running")
	}
	dm.running.Store(true)

	// Start queue processor
	dm.wg.Add(1)
	go dm.processQueue()

	// Start health checker
	if dm.healthCheck != nil {
		dm.wg.Add(1)
		go dm.healthCheckLoop()
	}

	// Start expiry checker
	dm.wg.Add(1)
	go dm.expiryLoop()

	return nil
}

// Stop stops the degradation manager
func (dm *DegradationManager) Stop() error {
	if !dm.running.Load() {
		return nil
	}

	dm.cancel()
	dm.wg.Wait()
	dm.running.Store(false)
	return nil
}

// Mode returns the current degradation mode
func (dm *DegradationManager) Mode() DegradationMode {
	dm.modeMu.RLock()
	defer dm.modeMu.RUnlock()
	return dm.mode
}

// SetMode manually sets the degradation mode
func (dm *DegradationManager) SetMode(mode DegradationMode) {
	dm.modeMu.Lock()
	defer dm.modeMu.Unlock()
	dm.transitionTo(mode)
}

func (dm *DegradationManager) transitionTo(newMode DegradationMode) {
	oldMode := dm.mode
	if oldMode == newMode {
		return
	}

	dm.mode = newMode
	dm.lastModeChange = time.Now()

	// Update rate limit
	var newRate float64
	switch newMode {
	case DegradationModeNormal:
		newRate = dm.config.RateLimitNormal
	case DegradationModeDegraded:
		newRate = dm.config.RateLimitDegraded
	case DegradationModeLimited:
		newRate = dm.config.RateLimitLimited
	case DegradationModeOffline:
		newRate = 0
	}
	dm.rateLimiter.SetRate(newRate)

	// Reset counters
	atomic.StoreInt64(&dm.consecutiveSuccesses, 0)
	atomic.StoreInt64(&dm.consecutiveFailures, 0)

	// Notify callback
	if dm.onModeChange != nil {
		go dm.onModeChange(oldMode, newMode)
	}
}

// AllowOperation checks if an operation is allowed in current mode
func (dm *DegradationManager) AllowOperation(priority OperationPriority) bool {
	dm.modeMu.RLock()
	mode := dm.mode
	dm.modeMu.RUnlock()

	// Check priority threshold
	threshold, ok := dm.config.PriorityThresholds[mode]
	if !ok {
		threshold = OperationPriorityCritical
	}

	if priority > threshold {
		return false
	}

	// Check rate limit (except for critical)
	if priority > OperationPriorityCritical {
		if !dm.rateLimiter.Allow() {
			return false
		}
	}

	return true
}

// Queue queues an operation for later execution
func (dm *DegradationManager) Queue(op *QueuedOperation) error {
	if op == nil {
		return errors.New("operation is nil")
	}

	// Set defaults
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now()
	}
	if op.Deadline.IsZero() && dm.config.QueueTimeout > 0 {
		op.Deadline = op.CreatedAt.Add(dm.config.QueueTimeout)
	}

	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	// Check queue size
	if len(dm.queue) >= dm.config.MaxQueueSize {
		// Drop lowest priority operation
		if !dm.dropLowestPriority(op.Priority) {
			if dm.onDrop != nil {
				dm.onDrop(op, "queue full")
			}
			atomic.AddInt64(&dm.stats.totalDropped, 1)
			return errors.New("queue full")
		}
	}

	// Insert by priority (higher priority first)
	inserted := false
	for i, existing := range dm.queue {
		if op.Priority < existing.Priority {
			// Insert before existing
			dm.queue = append(dm.queue[:i], append([]*QueuedOperation{op}, dm.queue[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		dm.queue = append(dm.queue, op)
	}

	atomic.AddInt64(&dm.stats.totalQueued, 1)
	return nil
}

func (dm *DegradationManager) dropLowestPriority(incomingPriority OperationPriority) bool {
	// Find lowest priority operation
	var lowestIdx = -1
	var lowestPriority = OperationPriorityCritical

	for i := len(dm.queue) - 1; i >= 0; i-- {
		if dm.queue[i].Priority > lowestPriority {
			lowestPriority = dm.queue[i].Priority
			lowestIdx = i
		}
	}

	// Only drop if incoming has higher priority
	if lowestIdx >= 0 && incomingPriority < lowestPriority {
		dropped := dm.queue[lowestIdx]
		dm.queue = append(dm.queue[:lowestIdx], dm.queue[lowestIdx+1:]...)

		if dm.onDrop != nil {
			dm.onDrop(dropped, "preempted by higher priority")
		}
		atomic.AddInt64(&dm.stats.totalDropped, 1)
		return true
	}

	return false
}

// Dequeue removes and returns the next operation
func (dm *DegradationManager) Dequeue() *QueuedOperation {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	if len(dm.queue) == 0 {
		return nil
	}

	op := dm.queue[0]
	dm.queue = dm.queue[1:]
	return op
}

// QueueSize returns the current queue size
func (dm *DegradationManager) QueueSize() int {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()
	return len(dm.queue)
}

// RecordSuccess records a successful operation
func (dm *DegradationManager) RecordSuccess() {
	atomic.AddInt64(&dm.consecutiveSuccesses, 1)
	atomic.StoreInt64(&dm.consecutiveFailures, 0)

	// Check for recovery
	if atomic.LoadInt64(&dm.consecutiveSuccesses) >= int64(dm.config.RecoveryThreshold) {
		dm.modeMu.Lock()
		dm.improve()
		dm.modeMu.Unlock()
	}
}

// RecordFailure records a failed operation
func (dm *DegradationManager) RecordFailure() {
	atomic.AddInt64(&dm.consecutiveFailures, 1)
	atomic.StoreInt64(&dm.consecutiveSuccesses, 0)

	// Check for degradation
	if atomic.LoadInt64(&dm.consecutiveFailures) >= int64(dm.config.DegradationThreshold) {
		dm.modeMu.Lock()
		dm.degrade()
		dm.modeMu.Unlock()
	}
}

func (dm *DegradationManager) improve() {
	switch dm.mode {
	case DegradationModeOffline:
		dm.transitionTo(DegradationModeLimited)
	case DegradationModeLimited:
		dm.transitionTo(DegradationModeDegraded)
	case DegradationModeDegraded:
		dm.transitionTo(DegradationModeNormal)

	default:
	}
}

func (dm *DegradationManager) degrade() {
	switch dm.mode {
	case DegradationModeNormal:
		dm.transitionTo(DegradationModeDegraded)
	case DegradationModeDegraded:
		dm.transitionTo(DegradationModeLimited)
	case DegradationModeLimited:
		dm.transitionTo(DegradationModeOffline)

	default:
	}
}

func (dm *DegradationManager) processQueue() {
	defer dm.wg.Done()

	ticker := time.NewTicker(dm.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.processQueuedOperations()
		}
	}
}

func (dm *DegradationManager) processQueuedOperations() {
	dm.modeMu.RLock()
	mode := dm.mode
	dm.modeMu.RUnlock()

	// Don't process in offline mode
	if mode == DegradationModeOffline {
		return
	}

	// Process queue
	for {
		op := dm.Dequeue()
		if op == nil {
			break
		}

		// Check if expired
		if op.IsExpired() {
			atomic.AddInt64(&dm.stats.totalExpired, 1)
			if op.Callback != nil {
				op.Callback(errors.New("operation expired"))
			}
			continue
		}

		// Check if operation is allowed
		if !dm.AllowOperation(op.Priority) {
			// Re-queue
			_ = dm.Queue(op) //nolint:errcheck // best-effort re-queue
			break
		}

		// Try to execute (this would be handled by external executor)
		// For now, just mark as processed
		atomic.AddInt64(&dm.stats.totalProcessed, 1)
		if op.Callback != nil {
			op.Callback(nil)
		}
	}
}

func (dm *DegradationManager) healthCheckLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(dm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			if dm.healthCheck != nil {
				if err := dm.healthCheck(); err != nil {
					dm.RecordFailure()
				} else {
					dm.RecordSuccess()
				}
			}
		}
	}
}

func (dm *DegradationManager) expiryLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.removeExpired()
		}
	}
}

func (dm *DegradationManager) removeExpired() {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	now := time.Now()
	var expired int64

	kept := dm.queue[:0]
	for _, op := range dm.queue {
		if !op.Deadline.IsZero() && now.After(op.Deadline) {
			expired++
			if op.Callback != nil {
				go op.Callback(errors.New("operation expired"))
			}
		} else {
			kept = append(kept, op)
		}
	}
	dm.queue = kept

	if expired > 0 {
		atomic.AddInt64(&dm.stats.totalExpired, expired)
	}
}

// SetHealthCheck sets the health check function
func (dm *DegradationManager) SetHealthCheck(fn func() error) {
	dm.healthCheck = fn
}

// SetModeChangeCallback sets the mode change callback
func (dm *DegradationManager) SetModeChangeCallback(fn func(from, to DegradationMode)) {
	dm.onModeChange = fn
}

// SetDropCallback sets the operation drop callback
func (dm *DegradationManager) SetDropCallback(fn func(op *QueuedOperation, reason string)) {
	dm.onDrop = fn
}

// GetStats returns degradation statistics
func (dm *DegradationManager) GetStats() DegradationStats {
	dm.modeMu.RLock()
	mode := dm.mode
	lastChange := dm.lastModeChange
	dm.modeMu.RUnlock()

	dm.queueMu.Lock()
	queueSize := int64(len(dm.queue))
	dm.queueMu.Unlock()

	var currentRate float64
	switch mode {
	case DegradationModeNormal:
		currentRate = dm.config.RateLimitNormal
	case DegradationModeDegraded:
		currentRate = dm.config.RateLimitDegraded
	case DegradationModeLimited:
		currentRate = dm.config.RateLimitLimited
	case DegradationModeOffline:
		currentRate = 0
	}

	return DegradationStats{
		Mode:                 mode,
		QueuedOperations:     queueSize,
		TotalQueued:          atomic.LoadInt64(&dm.stats.totalQueued),
		TotalProcessed:       atomic.LoadInt64(&dm.stats.totalProcessed),
		TotalDropped:         atomic.LoadInt64(&dm.stats.totalDropped),
		TotalExpired:         atomic.LoadInt64(&dm.stats.totalExpired),
		TotalRetried:         atomic.LoadInt64(&dm.stats.totalRetried),
		ConsecutiveSuccesses: atomic.LoadInt64(&dm.consecutiveSuccesses),
		ConsecutiveFailures:  atomic.LoadInt64(&dm.consecutiveFailures),
		CurrentRateLimit:     currentRate,
		LastModeChange:       lastChange,
		TimeInMode:           time.Since(lastChange),
	}
}

// GetQueuedOperations returns a copy of queued operations
func (dm *DegradationManager) GetQueuedOperations() []*QueuedOperation {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	ops := make([]*QueuedOperation, len(dm.queue))
	copy(ops, dm.queue)
	return ops
}

// CancelOperation cancels a queued operation by ID
func (dm *DegradationManager) CancelOperation(id string) bool {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	for i, op := range dm.queue {
		if op.ID == id {
			dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
			if op.Callback != nil {
				go op.Callback(errors.New("operation cancelled"))
			}
			return true
		}
	}
	return false
}

// ClearQueue clears all queued operations
func (dm *DegradationManager) ClearQueue() int {
	dm.queueMu.Lock()
	defer dm.queueMu.Unlock()

	count := len(dm.queue)
	for _, op := range dm.queue {
		if op.Callback != nil {
			go op.Callback(errors.New("queue cleared"))
		}
	}
	dm.queue = dm.queue[:0]
	atomic.AddInt64(&dm.stats.totalDropped, int64(count))
	return count
}

// IsHealthy returns true if in normal mode
func (dm *DegradationManager) IsHealthy() bool {
	return dm.Mode() == DegradationModeNormal
}

// IsDegraded returns true if in any degraded state
func (dm *DegradationManager) IsDegraded() bool {
	mode := dm.Mode()
	return mode != DegradationModeNormal
}

// IsOffline returns true if in offline mode
func (dm *DegradationManager) IsOffline() bool {
	return dm.Mode() == DegradationModeOffline
}
