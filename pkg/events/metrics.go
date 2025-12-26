package events

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector defines the interface for collecting event metrics
type MetricsCollector interface {
	// RecordEventPublished records a published event
	RecordEventPublished(eventType EventType, severity Severity)

	// RecordEventReceived records a received event
	RecordEventReceived(eventType EventType, severity Severity)

	// RecordEventProcessed records a processed event with duration
	RecordEventProcessed(eventType EventType, duration time.Duration, success bool)

	// RecordPublisherError records a publisher error
	RecordPublisherError(eventType EventType)

	// RecordSubscriberError records a subscriber error
	RecordSubscriberError(subject string)

	// RecordReactorExecution records a reactor execution
	RecordReactorExecution(reactorID string, duration time.Duration, success bool)

	// RecordActionExecution records an action execution
	RecordActionExecution(actionName string, actionType string, duration time.Duration, success bool)

	// RecordStorageOperation records a storage operation
	RecordStorageOperation(operation string, duration time.Duration, success bool)

	// GetMetrics returns current metrics
	GetMetrics() *Metrics
}

// Metrics holds all event system metrics
type Metrics struct {
	// Event counts
	EventsPublished  map[EventType]int64
	EventsReceived   map[EventType]int64
	EventsProcessed  map[EventType]int64
	EventsFailed     map[EventType]int64

	// Severity counts
	EventsBySeverity map[Severity]int64

	// Publisher/Subscriber stats
	PublisherErrors   int64
	SubscriberErrors  int64
	ActiveSubscribers int64

	// Reactor stats
	ReactorExecutions map[string]int64
	ReactorFailures   map[string]int64
	ReactorDurations  map[string]*DurationStats

	// Action stats
	ActionExecutions map[string]int64
	ActionFailures   map[string]int64
	ActionDurations  map[string]*DurationStats

	// Storage stats
	StorageOperations map[string]int64
	StorageFailures   map[string]int64
	StorageDurations  map[string]*DurationStats

	// Processing stats
	ProcessingDuration *DurationStats

	// System stats
	Uptime      time.Duration
	StartTime   time.Time
	LastEvent   time.Time
	EventRate   float64 // events per second

	mu sync.RWMutex
}

// DurationStats tracks duration statistics
type DurationStats struct {
	Count  int64
	Total  time.Duration
	Min    time.Duration
	Max    time.Duration
	Avg    time.Duration
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	mu     sync.RWMutex
	recent []time.Duration // For percentile calculations
}

// DefaultMetricsCollector implements MetricsCollector
type DefaultMetricsCollector struct {
	metrics *Metrics
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() MetricsCollector {
	return &DefaultMetricsCollector{
		metrics: &Metrics{
			EventsPublished:   make(map[EventType]int64),
			EventsReceived:    make(map[EventType]int64),
			EventsProcessed:   make(map[EventType]int64),
			EventsFailed:      make(map[EventType]int64),
			EventsBySeverity:  make(map[Severity]int64),
			ReactorExecutions: make(map[string]int64),
			ReactorFailures:   make(map[string]int64),
			ReactorDurations:  make(map[string]*DurationStats),
			ActionExecutions:  make(map[string]int64),
			ActionFailures:    make(map[string]int64),
			ActionDurations:   make(map[string]*DurationStats),
			StorageOperations: make(map[string]int64),
			StorageFailures:   make(map[string]int64),
			StorageDurations:  make(map[string]*DurationStats),
			ProcessingDuration: NewDurationStats(),
			StartTime:         time.Now(),
		},
	}
}

// RecordEventPublished records a published event
func (c *DefaultMetricsCollector) RecordEventPublished(eventType EventType, severity Severity) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.EventsPublished[eventType]++
	c.metrics.EventsBySeverity[severity]++
	c.metrics.LastEvent = time.Now()
}

// RecordEventReceived records a received event
func (c *DefaultMetricsCollector) RecordEventReceived(eventType EventType, severity Severity) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.EventsReceived[eventType]++
	c.metrics.LastEvent = time.Now()
}

// RecordEventProcessed records a processed event
func (c *DefaultMetricsCollector) RecordEventProcessed(eventType EventType, duration time.Duration, success bool) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	if success {
		c.metrics.EventsProcessed[eventType]++
	} else {
		c.metrics.EventsFailed[eventType]++
	}

	c.metrics.ProcessingDuration.Record(duration)
}

// RecordPublisherError records a publisher error
func (c *DefaultMetricsCollector) RecordPublisherError(eventType EventType) {
	atomic.AddInt64(&c.metrics.PublisherErrors, 1)
}

// RecordSubscriberError records a subscriber error
func (c *DefaultMetricsCollector) RecordSubscriberError(subject string) {
	atomic.AddInt64(&c.metrics.SubscriberErrors, 1)
}

// RecordReactorExecution records a reactor execution
func (c *DefaultMetricsCollector) RecordReactorExecution(reactorID string, duration time.Duration, success bool) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.ReactorExecutions[reactorID]++
	if !success {
		c.metrics.ReactorFailures[reactorID]++
	}

	if c.metrics.ReactorDurations[reactorID] == nil {
		c.metrics.ReactorDurations[reactorID] = NewDurationStats()
	}
	c.metrics.ReactorDurations[reactorID].Record(duration)
}

// RecordActionExecution records an action execution
func (c *DefaultMetricsCollector) RecordActionExecution(actionName string, actionType string, duration time.Duration, success bool) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	key := actionType + ":" + actionName
	c.metrics.ActionExecutions[key]++
	if !success {
		c.metrics.ActionFailures[key]++
	}

	if c.metrics.ActionDurations[key] == nil {
		c.metrics.ActionDurations[key] = NewDurationStats()
	}
	c.metrics.ActionDurations[key].Record(duration)
}

// RecordStorageOperation records a storage operation
func (c *DefaultMetricsCollector) RecordStorageOperation(operation string, duration time.Duration, success bool) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.StorageOperations[operation]++
	if !success {
		c.metrics.StorageFailures[operation]++
	}

	if c.metrics.StorageDurations[operation] == nil {
		c.metrics.StorageDurations[operation] = NewDurationStats()
	}
	c.metrics.StorageDurations[operation].Record(duration)
}

// GetMetrics returns current metrics
func (c *DefaultMetricsCollector) GetMetrics() *Metrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	// Calculate uptime
	c.metrics.Uptime = time.Since(c.metrics.StartTime)

	// Calculate event rate
	if c.metrics.Uptime.Seconds() > 0 {
		totalEvents := int64(0)
		for _, count := range c.metrics.EventsPublished {
			totalEvents += count
		}
		c.metrics.EventRate = float64(totalEvents) / c.metrics.Uptime.Seconds()
	}

	return c.metrics
}

// NewDurationStats creates a new DurationStats
func NewDurationStats() *DurationStats {
	return &DurationStats{
		Min:    time.Duration(0),
		Max:    time.Duration(0),
		recent: make([]time.Duration, 0, 1000),
	}
}

// Record records a duration
func (d *DurationStats) Record(duration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Count++
	d.Total += duration

	if d.Min == 0 || duration < d.Min {
		d.Min = duration
	}
	if duration > d.Max {
		d.Max = duration
	}

	d.Avg = time.Duration(int64(d.Total) / d.Count)

	// Keep recent values for percentile calculation (limit to 1000)
	d.recent = append(d.recent, duration)
	if len(d.recent) > 1000 {
		d.recent = d.recent[1:]
	}

	// Calculate percentiles
	d.calculatePercentiles()
}

// calculatePercentiles calculates P50, P95, P99
func (d *DurationStats) calculatePercentiles() {
	if len(d.recent) == 0 {
		return
	}

	// Simple percentile calculation (could be optimized)
	sorted := make([]time.Duration, len(d.recent))
	copy(sorted, d.recent)

	// Bubble sort (simple for small arrays)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	d.P50 = sorted[len(sorted)*50/100]
	if len(sorted) > 20 {
		d.P95 = sorted[len(sorted)*95/100]
	}
	if len(sorted) > 100 {
		d.P99 = sorted[len(sorted)*99/100]
	}
}

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a health check result
type HealthCheck struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Duration    time.Duration          `json:"duration"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// HealthChecker defines the interface for health checks
type HealthChecker interface {
	// Check performs a health check
	Check() *HealthCheck
}

// HealthMonitor manages multiple health checks
type HealthMonitor struct {
	checks map[string]HealthChecker
	mu     sync.RWMutex
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{
		checks: make(map[string]HealthChecker),
	}
}

// RegisterCheck registers a health check
func (h *HealthMonitor) RegisterCheck(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = checker
}

// UnregisterCheck unregisters a health check
func (h *HealthMonitor) UnregisterCheck(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checks, name)
}

// CheckAll performs all health checks
func (h *HealthMonitor) CheckAll() map[string]*HealthCheck {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]*HealthCheck)
	for name, checker := range h.checks {
		results[name] = checker.Check()
	}

	return results
}

// GetOverallStatus returns the overall health status
func (h *HealthMonitor) GetOverallStatus() HealthStatus {
	results := h.CheckAll()

	hasUnhealthy := false
	hasDegraded := false

	for _, result := range results {
		if result.Status == HealthStatusUnhealthy {
			hasUnhealthy = true
		} else if result.Status == HealthStatusDegraded {
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

// EventSystemHealthCheck performs a health check on the event system
type EventSystemHealthCheck struct {
	metrics  *Metrics
	maxAge   time.Duration
	maxErrors int64
}

// NewEventSystemHealthCheck creates a new event system health check
func NewEventSystemHealthCheck(metrics *Metrics, maxAge time.Duration, maxErrors int64) *EventSystemHealthCheck {
	return &EventSystemHealthCheck{
		metrics:   metrics,
		maxAge:    maxAge,
		maxErrors: maxErrors,
	}
}

// Check performs the health check
func (h *EventSystemHealthCheck) Check() *HealthCheck {
	start := time.Now()

	result := &HealthCheck{
		Name:        "event_system",
		Status:      HealthStatusHealthy,
		LastChecked: start,
		Details:     make(map[string]interface{}),
	}

	// Check last event age
	if !h.metrics.LastEvent.IsZero() {
		age := time.Since(h.metrics.LastEvent)
		result.Details["last_event_age"] = age.String()

		if age > h.maxAge {
			result.Status = HealthStatusDegraded
			result.Message = "No recent events"
		}
	}

	// Check error rates
	publisherErrors := atomic.LoadInt64(&h.metrics.PublisherErrors)
	subscriberErrors := atomic.LoadInt64(&h.metrics.SubscriberErrors)

	result.Details["publisher_errors"] = publisherErrors
	result.Details["subscriber_errors"] = subscriberErrors

	if publisherErrors > h.maxErrors || subscriberErrors > h.maxErrors {
		result.Status = HealthStatusDegraded
		result.Message = "High error rate"
	}

	result.Duration = time.Since(start)
	return result
}

// StorageHealthCheck performs a health check on the storage
type StorageHealthCheck struct {
	store EventStore
}

// NewStorageHealthCheck creates a new storage health check
func NewStorageHealthCheck(store EventStore) *StorageHealthCheck {
	return &StorageHealthCheck{
		store: store,
	}
}

// Check performs the health check
func (h *StorageHealthCheck) Check() *HealthCheck {
	start := time.Now()

	result := &HealthCheck{
		Name:        "event_storage",
		Status:      HealthStatusHealthy,
		LastChecked: start,
		Details:     make(map[string]interface{}),
	}

	// Try to count events
	ctx := time.After(5 * time.Second)
	done := make(chan error, 1)

	go func() {
		queryCtx := context.Background()
		_, err := h.store.Count(queryCtx, NewEventQuery())
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			result.Status = HealthStatusUnhealthy
			result.Message = "Storage query failed: " + err.Error()
		} else {
			result.Status = HealthStatusHealthy
		}
	case <-ctx:
		result.Status = HealthStatusUnhealthy
		result.Message = "Storage query timeout"
	}

	result.Duration = time.Since(start)
	return result
}
