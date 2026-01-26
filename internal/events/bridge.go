package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// BridgeConfig holds configuration for an event bridge
type BridgeConfig struct {
	Name        string
	Description string
	Enabled     bool

	// Source configuration
	SourceType   string // "nats", "kafka", "http"
	SourceConfig interface{}

	// Destination configuration
	DestinationType   string // "nats", "kafka", "webhook"
	DestinationConfig interface{}

	// Filter configuration
	Filter FilterExpression

	// Transformation
	Transform func(*Event) (*Event, error)

	// Error handling
	OnError      ErrorBehavior
	MaxRetries   int
	RetryBackoff time.Duration

	// Buffering
	BufferSize int

	// Metrics
	EnableMetrics bool
}

// Bridge routes events from one system to another
type Bridge struct {
	config *BridgeConfig

	// Source subscriber
	subscriber   EventSubscriber
	subscription *Subscription

	// Destination publisher
	publisher EventPublisher

	// State
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Metrics
	metrics *BridgeMetrics
}

// BridgeMetrics tracks bridge statistics
type BridgeMetrics struct {
	EventsReceived  int64
	EventsFiltered  int64
	EventsForwarded int64
	EventsFailed    int64
	TransformErrors int64
	LastActivity    time.Time
	mu              sync.RWMutex
}

// NewBridge creates a new event bridge
func NewBridge(config *BridgeConfig) (*Bridge, error) {
	if config == nil {
		return nil, fmt.Errorf("bridge config is required")
	}

	if config.Name == "" {
		return nil, fmt.Errorf("bridge name is required")
	}

	if config.BufferSize == 0 {
		config.BufferSize = 100
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	if config.RetryBackoff == 0 {
		config.RetryBackoff = 1 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	bridge := &Bridge{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		metrics: &BridgeMetrics{
			LastActivity: time.Now(),
		},
	}

	return bridge, nil
}

// SetSubscriber sets the source event subscriber
func (b *Bridge) SetSubscriber(subscriber EventSubscriber) *Bridge {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscriber = subscriber
	return b
}

// SetPublisher sets the destination event publisher
func (b *Bridge) SetPublisher(publisher EventPublisher) *Bridge {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.publisher = publisher
	return b
}

// Start starts the bridge
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge is already running")
	}

	if b.subscriber == nil {
		return fmt.Errorf("subscriber not configured")
	}

	if b.publisher == nil {
		return fmt.Errorf("publisher not configured")
	}

	// Subscribe to source
	sub, err := b.subscriber.Subscribe("*", b.handleEvent)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	b.subscription = sub
	b.running = true

	return nil
}

// Stop stops the bridge
func (b *Bridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}

	b.cancel()
	b.wg.Wait()

	if b.subscriber != nil {
		if err := b.subscriber.Close(); err != nil {
			return fmt.Errorf("failed to close subscriber: %w", err)
		}
	}

	if b.publisher != nil {
		if err := b.publisher.Close(); err != nil {
			return fmt.Errorf("failed to close publisher: %w", err)
		}
	}

	b.running = false
	return nil
}

// handleEvent processes an event from the source
func (b *Bridge) handleEvent(event *Event) error {
	atomic.AddInt64(&b.metrics.EventsReceived, 1)
	b.metrics.mu.Lock()
	b.metrics.LastActivity = time.Now()
	b.metrics.mu.Unlock()

	// Apply filter if configured
	if b.config.Filter != nil && !b.config.Filter.Matches(event) {
		atomic.AddInt64(&b.metrics.EventsFiltered, 1)
		return nil
	}

	// Apply transformation if configured
	if b.config.Transform != nil {
		transformed, err := b.config.Transform(event)
		if err != nil {
			atomic.AddInt64(&b.metrics.TransformErrors, 1)
			if b.config.OnError == ErrorBehaviorStop {
				return fmt.Errorf("transform failed: %w", err)
			}
			// Continue with original event on error
		} else {
			event = transformed
		}
	}

	// Forward event to destination with retries
	var lastErr error
	for attempt := 0; attempt <= b.config.MaxRetries; attempt++ {
		err := b.publisher.Publish(event)
		if err == nil {
			atomic.AddInt64(&b.metrics.EventsForwarded, 1)
			return nil
		}

		lastErr = err

		// Don't retry if context is cancelled
		if b.ctx.Err() != nil {
			break
		}

		// Don't sleep on last attempt
		if attempt < b.config.MaxRetries {
			if err := wait.ForContext(b.ctx, b.config.RetryBackoff); err != nil {
				break
			}
		}
	}

	atomic.AddInt64(&b.metrics.EventsFailed, 1)

	if b.config.OnError == ErrorBehaviorStop {
		return fmt.Errorf("failed to forward event after %d retries: %w", b.config.MaxRetries, lastErr)
	}

	return nil
}

// GetMetrics returns current bridge metrics
func (b *Bridge) GetMetrics() BridgeMetrics {
	b.metrics.mu.RLock()
	defer b.metrics.mu.RUnlock()

	return BridgeMetrics{
		EventsReceived:  atomic.LoadInt64(&b.metrics.EventsReceived),
		EventsFiltered:  atomic.LoadInt64(&b.metrics.EventsFiltered),
		EventsForwarded: atomic.LoadInt64(&b.metrics.EventsForwarded),
		EventsFailed:    atomic.LoadInt64(&b.metrics.EventsFailed),
		TransformErrors: atomic.LoadInt64(&b.metrics.TransformErrors),
		LastActivity:    b.metrics.LastActivity,
	}
}

// IsRunning returns whether the bridge is currently running
func (b *Bridge) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

// BridgeManager manages multiple bridges
type BridgeManager struct {
	bridges map[string]*Bridge
	mu      sync.RWMutex
}

// NewBridgeManager creates a new bridge manager
func NewBridgeManager() *BridgeManager {
	return &BridgeManager{
		bridges: make(map[string]*Bridge),
	}
}

// AddBridge adds a bridge to the manager
func (m *BridgeManager) AddBridge(bridge *Bridge) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bridge == nil {
		return fmt.Errorf("bridge is nil")
	}

	if _, exists := m.bridges[bridge.config.Name]; exists {
		return fmt.Errorf("bridge %s already exists", bridge.config.Name)
	}

	m.bridges[bridge.config.Name] = bridge
	return nil
}

// RemoveBridge removes a bridge from the manager
func (m *BridgeManager) RemoveBridge(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bridge, exists := m.bridges[name]
	if !exists {
		return fmt.Errorf("bridge %s not found", name)
	}

	// Stop bridge if running
	if bridge.IsRunning() {
		if err := bridge.Stop(); err != nil {
			return fmt.Errorf("failed to stop bridge: %w", err)
		}
	}

	delete(m.bridges, name)
	return nil
}

// GetBridge returns a bridge by name
func (m *BridgeManager) GetBridge(name string) (*Bridge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bridge, exists := m.bridges[name]
	if !exists {
		return nil, fmt.Errorf("bridge %s not found", name)
	}

	return bridge, nil
}

// ListBridges returns all bridge names
func (m *BridgeManager) ListBridges() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.bridges))
	for name := range m.bridges {
		names = append(names, name)
	}

	return names
}

// StartBridge starts a specific bridge
func (m *BridgeManager) StartBridge(name string) error {
	bridge, err := m.GetBridge(name)
	if err != nil {
		return err
	}

	return bridge.Start()
}

// StopBridge stops a specific bridge
func (m *BridgeManager) StopBridge(name string) error {
	bridge, err := m.GetBridge(name)
	if err != nil {
		return err
	}

	return bridge.Stop()
}

// StartAll starts all bridges
func (m *BridgeManager) StartAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, bridge := range m.bridges {
		if !bridge.config.Enabled {
			continue
		}

		if err := bridge.Start(); err != nil {
			errors = append(errors, fmt.Errorf("bridge %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to start some bridges: %v", errors)
	}

	return nil
}

// StopAll stops all bridges
func (m *BridgeManager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, bridge := range m.bridges {
		if err := bridge.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("bridge %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some bridges: %v", errors)
	}

	return nil
}

// GetAllMetrics returns metrics for all bridges
func (m *BridgeManager) GetAllMetrics() map[string]BridgeMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]BridgeMetrics)
	for name, bridge := range m.bridges {
		metrics[name] = bridge.GetMetrics()
	}

	return metrics
}
