package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RoutingRule defines a rule for routing events to handlers
type RoutingRule struct {
	// ID uniquely identifies the rule
	ID string

	// Name is a human-readable name for the rule
	Name string

	// Filter is the filter expression to match events
	Filter FilterExpression

	// Handler is the handler to invoke when the filter matches
	Handler EventHandler

	// Priority determines the order of rule evaluation (higher = earlier)
	Priority int

	// Enabled indicates if the rule is active
	Enabled bool

	// StopOnMatch indicates if routing should stop after this rule matches
	StopOnMatch bool
}

// Router routes events to handlers based on routing rules
type Router struct {
	rules   []*RoutingRule
	mu      sync.RWMutex
	metrics *RouterMetrics

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// RouterMetrics tracks router performance
type RouterMetrics struct {
	// Total events processed
	EventsProcessed uint64

	// Total events matched by at least one rule
	EventsMatched uint64

	// Total events with no matching rules
	EventsUnmatched uint64

	// Total routing operations (can be > events if multiple rules match)
	TotalRoutings uint64

	// Total routing errors
	RoutingErrors uint64

	// Per-rule metrics
	mu          sync.RWMutex
	ruleMetrics map[string]*RuleMetrics
}

// RuleMetrics tracks metrics for a single routing rule
type RuleMetrics struct {
	// Total events evaluated against this rule
	Evaluated uint64

	// Total events that matched this rule
	Matched uint64

	// Total successful handler invocations
	Handled uint64

	// Total handler errors
	Errors uint64

	// Mutex for protecting time.Time and string fields
	mu sync.RWMutex

	// Last match time
	LastMatch time.Time

	// Last error time
	LastError time.Time

	// Last error message
	LastErrorMsg string
}

// NewRouter creates a new event router
func NewRouter() *Router {
	ctx, cancel := context.WithCancel(context.Background())
	return &Router{
		rules: make([]*RoutingRule, 0),
		metrics: &RouterMetrics{
			ruleMetrics: make(map[string]*RuleMetrics),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// AddRule adds a routing rule
func (r *Router) AddRule(rule *RoutingRule) error {
	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}
	if rule.ID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}
	if rule.Filter == nil {
		return fmt.Errorf("rule filter cannot be nil")
	}
	if rule.Handler == nil {
		return fmt.Errorf("rule handler cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate ID
	for _, existing := range r.rules {
		if existing.ID == rule.ID {
			return fmt.Errorf("rule with ID %s already exists", rule.ID)
		}
	}

	// Add rule and sort by priority (highest first)
	r.rules = append(r.rules, rule)
	r.sortRules()

	// Initialize metrics for this rule
	r.metrics.mu.Lock()
	r.metrics.ruleMetrics[rule.ID] = &RuleMetrics{}
	r.metrics.mu.Unlock()

	return nil
}

// AddRuleFromExpression creates and adds a rule from a filter expression string
func (r *Router) AddRuleFromExpression(id, name, expression string, handler EventHandler) error {
	filter, err := ParseFilterExpression(expression)
	if err != nil {
		return fmt.Errorf("failed to parse filter expression: %w", err)
	}

	rule := &RoutingRule{
		ID:      id,
		Name:    name,
		Filter:  filter,
		Handler: handler,
		Enabled: true,
	}

	return r.AddRule(rule)
}

// RemoveRule removes a routing rule by ID
func (r *Router) RemoveRule(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, rule := range r.rules {
		if rule.ID != id {
			continue
		}
		// Remove rule
		r.rules = append(r.rules[:i], r.rules[i+1:]...)

		// Remove metrics
		r.metrics.mu.Lock()
		delete(r.metrics.ruleMetrics, id)
		r.metrics.mu.Unlock()

		return nil
	}

	return fmt.Errorf("rule %s not found", id)
}

// EnableRule enables a routing rule
func (r *Router) EnableRule(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rule := range r.rules {
		if rule.ID == id {
			rule.Enabled = true
			return nil
		}
	}

	return fmt.Errorf("rule %s not found", id)
}

// DisableRule disables a routing rule
func (r *Router) DisableRule(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rule := range r.rules {
		if rule.ID == id {
			rule.Enabled = false
			return nil
		}
	}

	return fmt.Errorf("rule %s not found", id)
}

// Route routes an event through all matching rules
func (r *Router) Route(event *Event) error {
	atomic.AddUint64(&r.metrics.EventsProcessed, 1)

	r.mu.RLock()
	rules := make([]*RoutingRule, len(r.rules))
	copy(rules, r.rules)
	r.mu.RUnlock()

	matched := false
	var lastErr error

	for _, rule := range rules {
		// Skip disabled rules
		if !rule.Enabled {
			continue
		}

		// Track evaluation
		r.metrics.mu.RLock()
		ruleMetrics := r.metrics.ruleMetrics[rule.ID]
		r.metrics.mu.RUnlock()
		atomic.AddUint64(&ruleMetrics.Evaluated, 1)

		// Check if event matches filter
		if !rule.Filter.Matches(event) {
			continue
		}

		// Event matched
		matched = true
		atomic.AddUint64(&ruleMetrics.Matched, 1)
		atomic.AddUint64(&r.metrics.TotalRoutings, 1)
		ruleMetrics.mu.Lock()
		ruleMetrics.LastMatch = time.Now()
		ruleMetrics.mu.Unlock()

		// Invoke handler
		if err := rule.Handler(event); err != nil {
			atomic.AddUint64(&ruleMetrics.Errors, 1)
			atomic.AddUint64(&r.metrics.RoutingErrors, 1)
			ruleMetrics.mu.Lock()
			ruleMetrics.LastError = time.Now()
			ruleMetrics.LastErrorMsg = err.Error()
			ruleMetrics.mu.Unlock()
			lastErr = fmt.Errorf("rule %s: %w", rule.ID, err)
		} else {
			atomic.AddUint64(&ruleMetrics.Handled, 1)
		}

		// Stop routing if rule says so
		if rule.StopOnMatch {
			break
		}
	}

	if matched {
		atomic.AddUint64(&r.metrics.EventsMatched, 1)
	} else {
		atomic.AddUint64(&r.metrics.EventsUnmatched, 1)
	}

	return lastErr
}

// RouteAsync routes an event asynchronously
func (r *Router) RouteAsync(event *Event) {
	go func() {
		if err := r.Route(event); err != nil {
			// Log error but don't propagate
			fmt.Printf("Async routing error: %v\n", err)
		}
	}()
}

// GetRules returns all routing rules (copy)
func (r *Router) GetRules() []*RoutingRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rules := make([]*RoutingRule, len(r.rules))
	copy(rules, r.rules)
	return rules
}

// GetRule returns a specific rule by ID
func (r *Router) GetRule(id string) (*RoutingRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.ID == id {
			// Return a copy
			ruleCopy := *rule
			return &ruleCopy, nil
		}
	}

	return nil, fmt.Errorf("rule %s not found", id)
}

// GetMetrics returns a snapshot of router metrics
func (r *Router) GetMetrics() *RouterMetrics {
	r.metrics.mu.RLock()
	defer r.metrics.mu.RUnlock()

	snapshot := &RouterMetrics{
		EventsProcessed: atomic.LoadUint64(&r.metrics.EventsProcessed),
		EventsMatched:   atomic.LoadUint64(&r.metrics.EventsMatched),
		EventsUnmatched: atomic.LoadUint64(&r.metrics.EventsUnmatched),
		TotalRoutings:   atomic.LoadUint64(&r.metrics.TotalRoutings),
		RoutingErrors:   atomic.LoadUint64(&r.metrics.RoutingErrors),
		ruleMetrics:     make(map[string]*RuleMetrics),
	}

	// Copy rule metrics
	for id, metrics := range r.metrics.ruleMetrics {
		metrics.mu.RLock()
		lastMatch := metrics.LastMatch
		lastErr := metrics.LastError
		lastErrMsg := metrics.LastErrorMsg
		metrics.mu.RUnlock()

		snapshot.ruleMetrics[id] = &RuleMetrics{
			Evaluated:    atomic.LoadUint64(&metrics.Evaluated),
			Matched:      atomic.LoadUint64(&metrics.Matched),
			Handled:      atomic.LoadUint64(&metrics.Handled),
			Errors:       atomic.LoadUint64(&metrics.Errors),
			LastMatch:    lastMatch,
			LastError:    lastErr,
			LastErrorMsg: lastErrMsg,
		}
	}

	return snapshot
}

// GetRuleMetrics returns metrics for a specific rule
func (r *Router) GetRuleMetrics(id string) (*RuleMetrics, error) {
	r.metrics.mu.RLock()
	defer r.metrics.mu.RUnlock()

	metrics, exists := r.metrics.ruleMetrics[id]
	if !exists {
		return nil, fmt.Errorf("no metrics for rule %s", id)
	}

	metrics.mu.RLock()
	lastMatch := metrics.LastMatch
	lastErr := metrics.LastError
	lastErrMsg := metrics.LastErrorMsg
	metrics.mu.RUnlock()

	// Return a copy
	return &RuleMetrics{
		Evaluated:    atomic.LoadUint64(&metrics.Evaluated),
		Matched:      atomic.LoadUint64(&metrics.Matched),
		Handled:      atomic.LoadUint64(&metrics.Handled),
		Errors:       atomic.LoadUint64(&metrics.Errors),
		LastMatch:    lastMatch,
		LastError:    lastErr,
		LastErrorMsg: lastErrMsg,
	}, nil
}

// ResetMetrics resets all router metrics
func (r *Router) ResetMetrics() {
	atomic.StoreUint64(&r.metrics.EventsProcessed, 0)
	atomic.StoreUint64(&r.metrics.EventsMatched, 0)
	atomic.StoreUint64(&r.metrics.EventsUnmatched, 0)
	atomic.StoreUint64(&r.metrics.TotalRoutings, 0)
	atomic.StoreUint64(&r.metrics.RoutingErrors, 0)

	r.metrics.mu.Lock()
	defer r.metrics.mu.Unlock()

	for _, metrics := range r.metrics.ruleMetrics {
		atomic.StoreUint64(&metrics.Evaluated, 0)
		atomic.StoreUint64(&metrics.Matched, 0)
		atomic.StoreUint64(&metrics.Handled, 0)
		atomic.StoreUint64(&metrics.Errors, 0)
		metrics.mu.Lock()
		metrics.LastMatch = time.Time{}
		metrics.LastError = time.Time{}
		metrics.LastErrorMsg = ""
		metrics.mu.Unlock()
	}
}

// Close closes the router and releases resources
func (r *Router) Close() error {
	r.cancel()
	return nil
}

// sortRules sorts rules by priority (highest first)
func (r *Router) sortRules() {
	// Simple insertion sort (rules list is typically small)
	for i := 1; i < len(r.rules); i++ {
		for j := i; j > 0 && r.rules[j].Priority > r.rules[j-1].Priority; j-- {
			r.rules[j], r.rules[j-1] = r.rules[j-1], r.rules[j]
		}
	}
}

// FanOut creates a handler that fans out to multiple handlers
func FanOut(handlers ...EventHandler) EventHandler {
	return func(event *Event) error {
		var lastErr error
		for _, handler := range handlers {
			if err := handler(event); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}
}

// FanOutAsync creates a handler that fans out to multiple handlers asynchronously
func FanOutAsync(handlers ...EventHandler) EventHandler {
	return func(event *Event) error {
		var wg sync.WaitGroup
		errChan := make(chan error, len(handlers))

		for _, h := range handlers {
			wg.Add(1)
			handler := h // Capture for goroutine
			go func() {
				defer wg.Done()
				if err := handler(event); err != nil {
					errChan <- err
				}
			}()
		}

		wg.Wait()
		close(errChan)

		// Return the last error (if any)
		var lastErr error
		for err := range errChan {
			lastErr = err
		}
		return lastErr
	}
}

// FilterHandler creates a handler that only executes if the filter matches
func FilterHandler(filter FilterExpression, handler EventHandler) EventHandler {
	return func(event *Event) error {
		if filter.Matches(event) {
			return handler(event)
		}
		return nil
	}
}

// ConditionalHandler creates a handler that executes one handler or another based on condition
func ConditionalHandler(filter FilterExpression, ifTrue, ifFalse EventHandler) EventHandler {
	return func(event *Event) error {
		if filter.Matches(event) {
			if ifTrue != nil {
				return ifTrue(event)
			}
		} else {
			if ifFalse != nil {
				return ifFalse(event)
			}
		}
		return nil
	}
}

// ChainHandlers chains multiple handlers together, stopping on first error
func ChainHandlers(handlers ...EventHandler) EventHandler {
	return func(event *Event) error {
		for _, handler := range handlers {
			if err := handler(event); err != nil {
				return err
			}
		}
		return nil
	}
}
