// Package backpressure provides publisher flow control for NATS messaging.
package backpressure

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Common errors.
var (
	ErrQueueFull      = errors.New("message queue is full")
	ErrPublisherPaused = errors.New("publisher is paused")
	ErrTimeout        = errors.New("timeout waiting for capacity")
)

// Strategy represents a backpressure strategy.
type Strategy string

const (
	// StrategyBlock blocks the publisher until capacity is available.
	StrategyBlock Strategy = "block"
	// StrategyDrop drops messages when the queue is full.
	StrategyDrop Strategy = "drop"
	// StrategyBuffer buffers messages in memory until capacity is available.
	StrategyBuffer Strategy = "buffer"
	// StrategyThrottle throttles the publish rate.
	StrategyThrottle Strategy = "throttle"
)

// Config configures backpressure behavior.
type Config struct {
	Strategy         Strategy      `json:"strategy"`
	MaxPending       int64         `json:"maxPending"`
	MaxBytes         int64         `json:"maxBytes"`
	BufferSize       int           `json:"bufferSize"`
	BlockTimeout     time.Duration `json:"blockTimeout"`
	ThrottleRate     int64         `json:"throttleRate"` // messages per second
	HighWaterMark    float64       `json:"highWaterMark"` // 0.0-1.0
	LowWaterMark     float64       `json:"lowWaterMark"`  // 0.0-1.0
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Strategy:      StrategyBlock,
		MaxPending:    1000,
		MaxBytes:      100 * 1024 * 1024, // 100 MB
		BufferSize:    1000,
		BlockTimeout:  10 * time.Second,
		ThrottleRate:  1000,
		HighWaterMark: 0.8,
		LowWaterMark:  0.5,
	}
}

// Message represents a message to be published.
type Message struct {
	Subject  string
	Data     []byte
	Headers  map[string]string
	Metadata map[string]interface{}
}

// Publisher provides backpressure-aware message publishing.
type Publisher struct {
	config       *Config
	pending      int64 // atomic
	pendingBytes int64 // atomic
	paused       int32 // atomic
	buffer       chan *Message
	dropCount    int64 // atomic
	publishCount int64 // atomic
	throttler    *throttler
	mu           sync.RWMutex
	listeners    []BackpressureEventListener
	publishFn    func(*Message) error
}

// BackpressureEvent represents a backpressure event.
type BackpressureEvent struct {
	Type         string    `json:"type"`
	Pending      int64     `json:"pending"`
	PendingBytes int64     `json:"pendingBytes"`
	Paused       bool      `json:"paused"`
	Timestamp    time.Time `json:"timestamp"`
}

// BackpressureEventListener is called when backpressure events occur.
type BackpressureEventListener func(*BackpressureEvent)

// NewPublisher creates a new backpressure-aware publisher.
func NewPublisher(config *Config, publishFn func(*Message) error) *Publisher {
	if config == nil {
		config = DefaultConfig()
	}

	p := &Publisher{
		config:    config,
		buffer:    make(chan *Message, config.BufferSize),
		publishFn: publishFn,
	}

	if config.Strategy == StrategyThrottle {
		p.throttler = newThrottler(config.ThrottleRate)
	}

	return p
}

// Publish publishes a message with backpressure handling.
func (p *Publisher) Publish(ctx context.Context, msg *Message) error {
	// Check if paused
	if atomic.LoadInt32(&p.paused) == 1 {
		return ErrPublisherPaused
	}

	switch p.config.Strategy {
	case StrategyBlock:
		return p.publishBlocking(ctx, msg)
	case StrategyDrop:
		return p.publishDropping(ctx, msg)
	case StrategyBuffer:
		return p.publishBuffering(ctx, msg)
	case StrategyThrottle:
		return p.publishThrottled(ctx, msg)
	default:
		return p.publishBlocking(ctx, msg)
	}
}

func (p *Publisher) publishBlocking(ctx context.Context, msg *Message) error {
	msgSize := int64(len(msg.Data))

	// Wait for capacity
	for {
		pending := atomic.LoadInt64(&p.pending)
		pendingBytes := atomic.LoadInt64(&p.pendingBytes)

		if pending < p.config.MaxPending && pendingBytes+msgSize <= p.config.MaxBytes {
			if atomic.CompareAndSwapInt64(&p.pending, pending, pending+1) {
				atomic.AddInt64(&p.pendingBytes, msgSize)
				break
			}
			continue
		}

		// Check pause state
		p.checkPause()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Retry
		}
	}

	return p.doPublish(msg)
}

func (p *Publisher) publishDropping(ctx context.Context, msg *Message) error {
	msgSize := int64(len(msg.Data))

	pending := atomic.LoadInt64(&p.pending)
	pendingBytes := atomic.LoadInt64(&p.pendingBytes)

	if pending >= p.config.MaxPending || pendingBytes+msgSize > p.config.MaxBytes {
		atomic.AddInt64(&p.dropCount, 1)
		p.emit(&BackpressureEvent{
			Type:         "drop",
			Pending:      pending,
			PendingBytes: pendingBytes,
			Timestamp:    time.Now(),
		})
		return ErrQueueFull
	}

	atomic.AddInt64(&p.pending, 1)
	atomic.AddInt64(&p.pendingBytes, msgSize)

	return p.doPublish(msg)
}

func (p *Publisher) publishBuffering(ctx context.Context, msg *Message) error {
	select {
	case p.buffer <- msg:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *Publisher) publishThrottled(ctx context.Context, msg *Message) error {
	if err := p.throttler.wait(ctx); err != nil {
		return err
	}

	msgSize := int64(len(msg.Data))
	atomic.AddInt64(&p.pending, 1)
	atomic.AddInt64(&p.pendingBytes, msgSize)

	return p.doPublish(msg)
}

func (p *Publisher) doPublish(msg *Message) error {
	err := p.publishFn(msg)

	msgSize := int64(len(msg.Data))
	atomic.AddInt64(&p.pending, -1)
	atomic.AddInt64(&p.pendingBytes, -msgSize)
	atomic.AddInt64(&p.publishCount, 1)

	p.checkResume()

	return err
}

func (p *Publisher) checkPause() {
	pending := atomic.LoadInt64(&p.pending)
	highWater := int64(float64(p.config.MaxPending) * p.config.HighWaterMark)

	if pending >= highWater && atomic.CompareAndSwapInt32(&p.paused, 0, 1) {
		p.emit(&BackpressureEvent{
			Type:         "pause",
			Pending:      pending,
			PendingBytes: atomic.LoadInt64(&p.pendingBytes),
			Paused:       true,
			Timestamp:    time.Now(),
		})
	}
}

func (p *Publisher) checkResume() {
	pending := atomic.LoadInt64(&p.pending)
	lowWater := int64(float64(p.config.MaxPending) * p.config.LowWaterMark)

	if pending <= lowWater && atomic.CompareAndSwapInt32(&p.paused, 1, 0) {
		p.emit(&BackpressureEvent{
			Type:         "resume",
			Pending:      pending,
			PendingBytes: atomic.LoadInt64(&p.pendingBytes),
			Paused:       false,
			Timestamp:    time.Now(),
		})
	}
}

// AddListener adds an event listener.
func (p *Publisher) AddListener(listener BackpressureEventListener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listeners = append(p.listeners, listener)
}

// Pause pauses publishing.
func (p *Publisher) Pause() {
	if atomic.CompareAndSwapInt32(&p.paused, 0, 1) {
		p.emit(&BackpressureEvent{
			Type:         "pause",
			Pending:      atomic.LoadInt64(&p.pending),
			PendingBytes: atomic.LoadInt64(&p.pendingBytes),
			Paused:       true,
			Timestamp:    time.Now(),
		})
	}
}

// Resume resumes publishing.
func (p *Publisher) Resume() {
	if atomic.CompareAndSwapInt32(&p.paused, 1, 0) {
		p.emit(&BackpressureEvent{
			Type:         "resume",
			Pending:      atomic.LoadInt64(&p.pending),
			PendingBytes: atomic.LoadInt64(&p.pendingBytes),
			Paused:       false,
			Timestamp:    time.Now(),
		})
	}
}

// IsPaused returns true if the publisher is paused.
func (p *Publisher) IsPaused() bool {
	return atomic.LoadInt32(&p.paused) == 1
}

// Stats returns publisher statistics.
func (p *Publisher) Stats() Stats {
	return Stats{
		Pending:      atomic.LoadInt64(&p.pending),
		PendingBytes: atomic.LoadInt64(&p.pendingBytes),
		Published:   atomic.LoadInt64(&p.publishCount),
		Dropped:     atomic.LoadInt64(&p.dropCount),
		Paused:      p.IsPaused(),
	}
}

// Stats contains publisher statistics.
type Stats struct {
	Pending      int64 `json:"pending"`
	PendingBytes int64 `json:"pendingBytes"`
	Published    int64 `json:"published"`
	Dropped      int64 `json:"dropped"`
	Paused       bool  `json:"paused"`
}

func (p *Publisher) emit(event *BackpressureEvent) {
	p.mu.RLock()
	listeners := p.listeners
	p.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// FlushBuffer flushes buffered messages.
func (p *Publisher) FlushBuffer(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-p.buffer:
			if err := p.doPublish(msg); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// BufferLen returns the number of buffered messages.
func (p *Publisher) BufferLen() int {
	return len(p.buffer)
}

type throttler struct {
	rate     int64
	tokens   int64 // atomic
	lastTick time.Time
	mu       sync.Mutex
	stopCh   chan struct{}
}

func newThrottler(rate int64) *throttler {
	t := &throttler{
		rate:     rate,
		tokens:   rate / 10, // Initial burst
		lastTick: time.Now(),
		stopCh:   make(chan struct{}),
	}

	go t.refillLoop()

	return t
}

func (t *throttler) wait(ctx context.Context) error {
	for {
		tokens := atomic.LoadInt64(&t.tokens)
		if tokens > 0 {
			if atomic.CompareAndSwapInt64(&t.tokens, tokens, tokens-1) {
				return nil
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			// Retry
		}
	}
}

func (t *throttler) refillLoop() {
	// Refill 10 times per second
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	tokensPerTick := t.rate / 10
	if tokensPerTick <= 0 {
		tokensPerTick = 1
	}

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			for {
				current := atomic.LoadInt64(&t.tokens)
				newTokens := current + tokensPerTick
				maxTokens := t.rate / 5 // Max burst is 1/5 of rate
				if newTokens > maxTokens {
					newTokens = maxTokens
				}
				if atomic.CompareAndSwapInt64(&t.tokens, current, newTokens) {
					break
				}
			}
		}
	}
}

func (t *throttler) stop() {
	close(t.stopCh)
}

// FlowController provides advanced flow control.
type FlowController struct {
	config     *Config
	windowSize int64 // atomic
	acks       int64 // atomic
	sent       int64 // atomic
	mu         sync.RWMutex
	listeners  []FlowControlEventListener
}

// FlowControlEvent represents a flow control event.
type FlowControlEvent struct {
	Type       string    `json:"type"`
	WindowSize int64     `json:"windowSize"`
	Sent       int64     `json:"sent"`
	Acks       int64     `json:"acks"`
	Timestamp  time.Time `json:"timestamp"`
}

// FlowControlEventListener is called when flow control events occur.
type FlowControlEventListener func(*FlowControlEvent)

// NewFlowController creates a new flow controller.
func NewFlowController(config *Config) *FlowController {
	windowSize := config.MaxPending
	if windowSize <= 0 {
		windowSize = 1000
	}

	return &FlowController{
		config:     config,
		windowSize: windowSize,
	}
}

// AddListener adds an event listener.
func (fc *FlowController) AddListener(listener FlowControlEventListener) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.listeners = append(fc.listeners, listener)
}

// AcquireSlot acquires a slot in the window.
func (fc *FlowController) AcquireSlot(ctx context.Context) error {
	for {
		sent := atomic.LoadInt64(&fc.sent)
		acks := atomic.LoadInt64(&fc.acks)
		windowSize := atomic.LoadInt64(&fc.windowSize)

		if sent-acks < windowSize {
			if atomic.CompareAndSwapInt64(&fc.sent, sent, sent+1) {
				return nil
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			// Retry
		}
	}
}

// Ack acknowledges a message.
func (fc *FlowController) Ack() {
	atomic.AddInt64(&fc.acks, 1)
}

// AckN acknowledges n messages.
func (fc *FlowController) AckN(n int64) {
	atomic.AddInt64(&fc.acks, n)
}

// SetWindowSize sets the window size.
func (fc *FlowController) SetWindowSize(size int64) {
	old := atomic.SwapInt64(&fc.windowSize, size)
	if old != size {
		fc.emit(&FlowControlEvent{
			Type:       "window_resize",
			WindowSize: size,
			Sent:       atomic.LoadInt64(&fc.sent),
			Acks:       atomic.LoadInt64(&fc.acks),
			Timestamp:  time.Now(),
		})
	}
}

// InFlight returns the number of in-flight messages.
func (fc *FlowController) InFlight() int64 {
	sent := atomic.LoadInt64(&fc.sent)
	acks := atomic.LoadInt64(&fc.acks)
	return sent - acks
}

// WindowSize returns the current window size.
func (fc *FlowController) WindowSize() int64 {
	return atomic.LoadInt64(&fc.windowSize)
}

func (fc *FlowController) emit(event *FlowControlEvent) {
	fc.mu.RLock()
	listeners := fc.listeners
	fc.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// AdaptiveFlowController adjusts window size based on conditions.
type AdaptiveFlowController struct {
	*FlowController
	minWindow   int64
	maxWindow   int64
	lastAdjust  time.Time
	adjustMu    sync.Mutex
}

// NewAdaptiveFlowController creates a new adaptive flow controller.
func NewAdaptiveFlowController(minWindow, maxWindow int64) *AdaptiveFlowController {
	config := DefaultConfig()
	config.MaxPending = maxWindow

	return &AdaptiveFlowController{
		FlowController: NewFlowController(config),
		minWindow:      minWindow,
		maxWindow:      maxWindow,
		lastAdjust:     time.Now(),
	}
}

// RecordSuccess records a successful operation.
func (afc *AdaptiveFlowController) RecordSuccess() {
	afc.Ack()
	afc.maybeIncrease()
}

// RecordFailure records a failed operation.
func (afc *AdaptiveFlowController) RecordFailure() {
	afc.Ack()
	afc.decrease()
}

func (afc *AdaptiveFlowController) maybeIncrease() {
	afc.adjustMu.Lock()
	defer afc.adjustMu.Unlock()

	if time.Since(afc.lastAdjust) < 100*time.Millisecond {
		return
	}

	current := afc.WindowSize()
	if current < afc.maxWindow {
		// Additive increase
		newSize := current + current/10
		if newSize > afc.maxWindow {
			newSize = afc.maxWindow
		}
		afc.SetWindowSize(newSize)
	}

	afc.lastAdjust = time.Now()
}

func (afc *AdaptiveFlowController) decrease() {
	afc.adjustMu.Lock()
	defer afc.adjustMu.Unlock()

	current := afc.WindowSize()
	// Multiplicative decrease
	newSize := current / 2
	if newSize < afc.minWindow {
		newSize = afc.minWindow
	}
	afc.SetWindowSize(newSize)

	afc.lastAdjust = time.Now()
}

// Semaphore provides simple concurrency limiting.
type Semaphore struct {
	permits int64
	current int64 // atomic
}

// NewSemaphore creates a new semaphore.
func NewSemaphore(permits int64) *Semaphore {
	return &Semaphore{permits: permits}
}

// Acquire acquires a permit.
func (s *Semaphore) Acquire(ctx context.Context) error {
	for {
		current := atomic.LoadInt64(&s.current)
		if current < s.permits {
			if atomic.CompareAndSwapInt64(&s.current, current, current+1) {
				return nil
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			// Retry
		}
	}
}

// TryAcquire tries to acquire a permit without blocking.
func (s *Semaphore) TryAcquire() bool {
	for {
		current := atomic.LoadInt64(&s.current)
		if current >= s.permits {
			return false
		}
		if atomic.CompareAndSwapInt64(&s.current, current, current+1) {
			return true
		}
	}
}

// Release releases a permit.
func (s *Semaphore) Release() {
	atomic.AddInt64(&s.current, -1)
}

// Available returns the number of available permits.
func (s *Semaphore) Available() int64 {
	return s.permits - atomic.LoadInt64(&s.current)
}
