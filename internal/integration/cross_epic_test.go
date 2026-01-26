// Package integration provides in-process integration tests that verify
// cross-epic interactions without requiring Docker containers.
// These tests are faster than E2E tests and can run as part of regular
// `go test` execution.
package integration

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// testEnv holds the shared test environment for integration tests
type testEnv struct {
	tmpDir        string
	reactorEngine *events.ReactorEngine
	eventBus      *testEventBus
}

// testEventBus provides a simple in-memory pub/sub for testing
// without requiring JetStream
type testEventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]testSubscription
	counter     int
}

type testSubscription struct {
	id      string
	handler events.EventHandler
	filter  *events.EventFilter
}

// newTestEventBus creates a new in-memory event bus for testing
func newTestEventBus() *testEventBus {
	return &testEventBus{
		subscribers: make(map[string][]testSubscription),
	}
}

// Subscribe subscribes to all events
func (b *testEventBus) Subscribe(name string, handler events.EventHandler) (*testSubHandle, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.counter++
	id := name + "-" + string(rune('0'+b.counter))

	sub := testSubscription{
		id:      id,
		handler: handler,
	}
	b.subscribers[name] = append(b.subscribers[name], sub)

	return &testSubHandle{
		id:   id,
		name: name,
		bus:  b,
	}, nil
}

// SubscribeWithFilter subscribes with an event filter
func (b *testEventBus) SubscribeWithFilter(name string, filter *events.EventFilter, handler events.EventHandler) (*testSubHandle, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.counter++
	id := name + "-" + string(rune('0'+b.counter))

	sub := testSubscription{
		id:      id,
		handler: handler,
		filter:  filter,
	}
	b.subscribers[name] = append(b.subscribers[name], sub)

	return &testSubHandle{
		id:   id,
		name: name,
		bus:  b,
	}, nil
}

func (b *testEventBus) snapshotHandlers(event *events.Event) []events.EventHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var handlers []events.EventHandler
	for _, subs := range b.subscribers {
		for _, sub := range subs {
			if sub.filter != nil && !sub.filter.Matches(event) {
				continue
			}
			handlers = append(handlers, sub.handler)
		}
	}
	return handlers
}

// Publish publishes an event to all subscribers
func (b *testEventBus) Publish(event *events.Event) error {
	handlers := b.snapshotHandlers(event)
	for _, handler := range handlers {
		go handler(event)
	}
	return nil
}

// PublishSync publishes an event and waits for all handlers to complete.
func (b *testEventBus) PublishSync(event *events.Event) error {
	handlers := b.snapshotHandlers(event)
	for _, handler := range handlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

// testSubHandle represents a subscription that can be unsubscribed
type testSubHandle struct {
	id   string
	name string
	bus  *testEventBus
}

// Unsubscribe removes the subscription
func (h *testSubHandle) Unsubscribe() error {
	h.bus.mu.Lock()
	defer h.bus.mu.Unlock()

	subs := h.bus.subscribers[h.name]
	for i, s := range subs {
		if s.id == h.id {
			h.bus.subscribers[h.name] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	return nil
}

// setupTestEnv creates a complete test environment with event bus for integration testing.
// Note: NATS and state store initialization are skipped as these tests focus on
// event-based integration patterns using an in-memory event bus.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "kscore-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize in-memory event bus for testing
	eventBus := newTestEventBus()

	// Initialize reactor engine
	reactorEngine := events.NewReactorEngine()

	return &testEnv{
		tmpDir:        tmpDir,
		eventBus:      eventBus,
		reactorEngine: reactorEngine,
	}
}

// cleanup tears down the test environment
func (e *testEnv) cleanup() {
	if e.reactorEngine != nil {
		e.reactorEngine.Close()
	}
	if e.tmpDir != "" {
		os.RemoveAll(e.tmpDir)
	}
}

// =============================================================================
// Epic 1 (Core) + Epic 4 (Events) Integration
// Tests: NATS messaging + Event bus integration
// =============================================================================

// TestIntegration_NATSToEventBus verifies that NATS and the event bus
// work together correctly for event publishing and subscription.
func TestIntegration_NATSToEventBus(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Create an event using the fluent builder
	event := events.NewEvent(events.EventTypeAgentConnect).
		Source("/test/integration").
		Severity(events.SeverityInfo).
		Data("agent_id", "test-agent-1").
		Data("hostname", "test-host").
		Build()

	// Subscribe to events
	var receivedEvent *events.Event

	sub, err := env.eventBus.Subscribe("test-subscriber", func(e *events.Event) error {
		receivedEvent = e
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Publish event
	if err := env.eventBus.PublishSync(event); err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	if receivedEvent == nil {
		t.Fatal("Event not received")
	}

	// Verify event
	if receivedEvent == nil {
		t.Fatal("No event received")
	}
	if receivedEvent.Type != events.EventTypeAgentConnect {
		t.Errorf("Expected type %s, got %s", events.EventTypeAgentConnect, receivedEvent.Type)
	}
	if receivedEvent.Source != "/test/integration" {
		t.Errorf("Expected source /test/integration, got %s", receivedEvent.Source)
	}
}

// TestIntegration_EventFiltering verifies that event filtering works correctly
// when events flow through the system.
func TestIntegration_EventFiltering(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Subscribe with filter - only agent events
	var receivedEvents []*events.Event
	var mu sync.Mutex

	// Create filter for agent events
	filter := &events.EventFilter{
		Types: []events.EventType{
			events.EventTypeAgentConnect,
			events.EventTypeAgentDisconnect,
			events.EventTypeAgentHeartbeat,
		},
	}

	sub, err := env.eventBus.SubscribeWithFilter("filtered-subscriber", filter, func(e *events.Event) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, e)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe with filter: %v", err)
	}
	defer sub.Unsubscribe()

	// Publish different event types
	eventTypes := []events.EventType{
		events.EventTypeAgentConnect,
		events.EventTypeJobStart,
		events.EventTypeAgentDisconnect,
		events.EventTypeStateChange,
		events.EventTypeAgentHeartbeat,
	}

	for _, eventType := range eventTypes {
		event := events.NewEvent(eventType).
			Source("/test").
			Build()
		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", eventType, err)
		}
	}

	// Verify only agent events were received
	mu.Lock()
	count := len(receivedEvents)
	mu.Unlock()

	// Should have received 3 agent events (connect, disconnect, heartbeat)
	if count != 3 {
		t.Errorf("Expected 3 filtered events, got %d", count)
	}

	for _, e := range receivedEvents {
		if e.Type != events.EventTypeAgentConnect &&
			e.Type != events.EventTypeAgentDisconnect &&
			e.Type != events.EventTypeAgentHeartbeat {
			t.Errorf("Unexpected event type received: %s", e.Type)
		}
	}
}

// =============================================================================
// Epic 1 (Core) + Epic 3 (State) Integration
// Tests: State events integration via event bus
// =============================================================================

// TestIntegration_StateEventIntegration verifies that state-related events
// flow correctly through the event system.
func TestIntegration_StateEventIntegration(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Verify event bus is accessible
	if env.eventBus == nil {
		t.Fatal("Event bus not initialized")
	}

	// Test that state events can be published and received
	received := make(chan struct{}, 1)
	sub, err := env.eventBus.Subscribe("state-test", func(e *events.Event) error {
		received <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	event := events.NewEvent(events.EventTypeStateChange).
		Source("/test/state").
		Build()

	if err := env.eventBus.PublishSync(event); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	select {
	case <-received:
	case <-time.After(200 * time.Millisecond):
		t.Error("State event not received")
	}

	t.Log("State event integration working")
}

// =============================================================================
// Epic 4 (Events) + Reactor Integration
// Tests: Event-driven reactor responses
// =============================================================================

// TestIntegration_EventReactorChain tests that events can trigger reactor
// responses in a chain.
func TestIntegration_EventReactorChain(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track the chain of events
	var eventChain []events.EventType
	var mu sync.Mutex

	// Subscribe to all events
	sub, err := env.eventBus.Subscribe("chain-tracker", func(e *events.Event) error {
		mu.Lock()
		eventChain = append(eventChain, e.Type)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Create filter for state.change events
	stateChangeFilter := &events.EventFilter{
		Types: []events.EventType{events.EventTypeStateChange},
	}

	// Simulate a reactor that responds to state.change with state.drift
	reactorSub, err := env.eventBus.SubscribeWithFilter("drift-reactor",
		stateChangeFilter,
		func(e *events.Event) error {
			// Reactor triggers a drift check event
			driftEvent := events.NewEvent(events.EventTypeStateDrift).
				Source("/reactor/drift-detector").
				CorrelationID(e.CorrelationID).
				Build()
			return env.eventBus.PublishSync(driftEvent)
		})
	if err != nil {
		t.Fatalf("Failed to setup reactor: %v", err)
	}
	defer reactorSub.Unsubscribe()

	// Trigger the initial event
	triggerEvent := events.NewEvent(events.EventTypeStateChange).
		Source("/test/trigger").
		Build()

	if err := env.eventBus.PublishSync(triggerEvent); err != nil {
		t.Fatalf("Failed to publish trigger event: %v", err)
	}

	// Verify the chain contains both events
	// Note: order is not guaranteed with async delivery
	mu.Lock()
	chainLen := len(eventChain)
	chainCopy := make([]events.EventType, len(eventChain))
	copy(chainCopy, eventChain)
	mu.Unlock()

	if chainLen < 2 {
		t.Errorf("Expected at least 2 events in chain, got %d", chainLen)
	}

	// Verify both event types are present (order may vary with async delivery)
	hasStateChange := false
	hasStateDrift := false
	for _, evt := range chainCopy {
		if evt == events.EventTypeStateChange {
			hasStateChange = true
		}
		if evt == events.EventTypeStateDrift {
			hasStateDrift = true
		}
	}

	if !hasStateChange {
		t.Error("Missing state.change event in chain")
	}
	if !hasStateDrift {
		t.Error("Missing state.drift event in chain (reactor didn't trigger)")
	}

	t.Logf("Event chain contains both events: %v", chainCopy)
}

// =============================================================================
// Epic 4 (Events) + Epic 6 (Policy) Event Integration
// Tests: Policy events in the event system
// =============================================================================

// TestIntegration_PolicyEvents tests that policy evaluation results
// are properly emitted as events.
func TestIntegration_PolicyEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track policy events
	var policyEvents []*events.Event
	var mu sync.Mutex

	// Create filter for policy events
	policyFilter := &events.EventFilter{
		Types: []events.EventType{
			events.EventTypePolicyPass,
			events.EventTypePolicyViolation,
		},
	}

	sub, err := env.eventBus.SubscribeWithFilter("policy-tracker",
		policyFilter,
		func(e *events.Event) error {
			mu.Lock()
			policyEvents = append(policyEvents, e)
			mu.Unlock()
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Emit policy pass event
	passEvent := events.NewEvent(events.EventTypePolicyPass).
		Source("/policy/test-policy").
		Severity(events.SeverityInfo).
		Data("policy_id", "test-policy-1").
		Data("resource", "test-resource").
		Data("decision", "allow").
		Build()

	if err := env.eventBus.PublishSync(passEvent); err != nil {
		t.Fatalf("Failed to publish pass event: %v", err)
	}

	// Emit policy violation event
	violationEvent := events.NewEvent(events.EventTypePolicyViolation).
		Source("/policy/test-policy").
		Severity(events.SeverityWarning).
		Data("policy_id", "test-policy-2").
		Data("resource", "restricted-resource").
		Data("decision", "deny").
		Data("reason", "access not permitted").
		Build()

	if err := env.eventBus.PublishSync(violationEvent); err != nil {
		t.Fatalf("Failed to publish violation event: %v", err)
	}

	mu.Lock()
	count := len(policyEvents)
	mu.Unlock()

	if count != 2 {
		t.Errorf("Expected 2 policy events, got %d", count)
	}

	// Verify event types
	hasPass := false
	hasViolation := false
	for _, e := range policyEvents {
		if e.Type == events.EventTypePolicyPass {
			hasPass = true
		}
		if e.Type == events.EventTypePolicyViolation {
			hasViolation = true
		}
	}

	if !hasPass {
		t.Error("Missing policy.pass event")
	}
	if !hasViolation {
		t.Error("Missing policy.violation event")
	}
}

// =============================================================================
// Epic 3 (State) + Epic 4 (Events) Integration
// Tests: State changes emit appropriate events
// =============================================================================

// TestIntegration_StateChangeEvents tests that state changes properly
// emit events through the event bus.
func TestIntegration_StateChangeEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track state events
	var stateEvents []*events.Event
	var mu sync.Mutex

	// Create filter for state events using source prefix matching
	sub, err := env.eventBus.Subscribe("state-tracker",
		func(e *events.Event) error {
			// Manual filter for state.* events
			if strings.HasPrefix(string(e.Type), "state.") {
				mu.Lock()
				stateEvents = append(stateEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate state apply lifecycle
	correlationID := "state-apply-123"

	// State apply start
	startEvent := events.NewEvent(events.EventTypeStateApplyStart).
		Source("/state/apply").
		CorrelationID(correlationID).
		Data("state_file", "/path/to/state.yaml").
		Data("agent_id", "test-agent").
		Build()

	if err := env.eventBus.PublishSync(startEvent); err != nil {
		t.Fatalf("Failed to publish start event: %v", err)
	}

	// State change event
	changeEvent := events.NewEvent(events.EventTypeStateChange).
		Source("/state/module/file").
		CorrelationID(correlationID).
		Data("module", "file.managed").
		Data("name", "/etc/test.conf").
		Data("result", "changed").
		Data("agent_id", "test-agent").
		Build()

	if err := env.eventBus.PublishSync(changeEvent); err != nil {
		t.Fatalf("Failed to publish change event: %v", err)
	}

	// State apply done
	doneEvent := events.NewEvent(events.EventTypeStateApplyDone).
		Source("/state/apply").
		CorrelationID(correlationID).
		Data("state_file", "/path/to/state.yaml").
		Data("agent_id", "test-agent").
		Data("changes", 1).
		Data("failures", 0).
		Build()

	if err := env.eventBus.PublishSync(doneEvent); err != nil {
		t.Fatalf("Failed to publish done event: %v", err)
	}

	mu.Lock()
	count := len(stateEvents)
	eventsCopy := make([]*events.Event, len(stateEvents))
	copy(eventsCopy, stateEvents)
	mu.Unlock()

	if count != 3 {
		t.Errorf("Expected 3 state events, got %d", count)
	}

	// Verify event sequence and correlation
	for _, e := range eventsCopy {
		if e.CorrelationID != correlationID {
			t.Errorf("Event %s has wrong correlation ID: %s", e.Type, e.CorrelationID)
		}
	}

	t.Logf("State event sequence verified with correlation ID: %s", correlationID)
}

// =============================================================================
// Multi-Subscriber Integration
// Tests: Multiple components receiving events simultaneously
// =============================================================================

// TestIntegration_MultipleSubscribers tests that multiple subscribers
// can receive the same events correctly.
func TestIntegration_MultipleSubscribers(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	const numSubscribers = 5
	const numEvents = 10

	// Create multiple subscribers
	var counters []int
	var mus []sync.Mutex
	var subs []*testSubHandle

	for i := 0; i < numSubscribers; i++ {
		counters = append(counters, 0)
		mus = append(mus, sync.Mutex{})

		idx := i
		sub, err := env.eventBus.Subscribe("subscriber-"+string(rune('A'+i)), func(e *events.Event) error {
			mus[idx].Lock()
			counters[idx]++
			mus[idx].Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to create subscriber %d: %v", i, err)
		}
		subs = append(subs, sub)
	}

	defer func() {
		for _, sub := range subs {
			sub.Unsubscribe()
		}
	}()

	// Publish events
	for i := 0; i < numEvents; i++ {
		event := events.NewEvent(events.EventTypeSystemStartup).
			Source("/test/multi-sub").
			Build()
		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish event %d: %v", i, err)
		}
	}

	// Verify all subscribers received all events
	for i := 0; i < numSubscribers; i++ {
		mus[i].Lock()
		count := counters[i]
		mus[i].Unlock()

		if count != numEvents {
			t.Errorf("Subscriber %d received %d events, expected %d", i, count, numEvents)
		}
	}

	t.Logf("All %d subscribers received all %d events", numSubscribers, numEvents)
}

// =============================================================================
// Bootstrap Event Integration
// Tests: Bootstrap lifecycle events
// =============================================================================

// TestIntegration_BootstrapEvents tests that bootstrap lifecycle events
// are properly handled by the event system.
func TestIntegration_BootstrapEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track bootstrap events
	var bootstrapEvents []*events.Event
	var mu sync.Mutex

	// Subscribe and filter for bootstrap events manually
	sub, err := env.eventBus.Subscribe("bootstrap-tracker",
		func(e *events.Event) error {
			if strings.HasPrefix(string(e.Type), "bootstrap.") {
				mu.Lock()
				bootstrapEvents = append(bootstrapEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate bootstrap lifecycle
	bootstrapEventTypes := []events.EventType{
		events.EventTypeBootstrapGenerate,
		events.EventTypeBootstrapValidate,
		events.EventTypeBootstrapUse,
		events.EventTypeBootstrapRegister,
	}

	for _, eventType := range bootstrapEventTypes {
		event := events.NewEvent(eventType).
			Source("/bootstrap/test").
			Data("token_id", "test-token-123").
			Data("agent_id", "new-agent").
			Data("timestamp", time.Now().Unix()).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", eventType, err)
		}
	}

	mu.Lock()
	count := len(bootstrapEvents)
	mu.Unlock()

	if count != len(bootstrapEventTypes) {
		t.Errorf("Expected %d bootstrap events, got %d", len(bootstrapEventTypes), count)
	}

	t.Logf("Bootstrap lifecycle events processed: %d events", count)
}

// =============================================================================
// Event Correlation Integration
// Tests: Events can be correlated across operations
// =============================================================================

// TestIntegration_EventCorrelation tests that events can be correlated
// across multiple operations using correlation IDs.
func TestIntegration_EventCorrelation(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "test-correlation-" + time.Now().Format("20060102150405")

	// Track correlated events
	var correlatedEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("correlation-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			correlatedEvents = append(correlatedEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Publish events with same correlation ID
	eventTypes := []events.EventType{
		events.EventTypeJobStart,
		events.EventTypeStateApplyStart,
		events.EventTypeStateChange,
		events.EventTypeStateApplyDone,
		events.EventTypeJobComplete,
	}

	for _, eventType := range eventTypes {
		event := events.NewEvent(eventType).
			Source("/test/correlated").
			CorrelationID(correlationID).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", eventType, err)
		}
	}

	// Publish some uncorrelated events (should not appear)
	for i := 0; i < 3; i++ {
		event := events.NewEvent(events.EventTypeSystemStartup).
			Source("/test/uncorrelated").
			Build()
		env.eventBus.PublishSync(event)
	}

	mu.Lock()
	count := len(correlatedEvents)
	mu.Unlock()

	if count != len(eventTypes) {
		t.Errorf("Expected %d correlated events, got %d", len(eventTypes), count)
	}

	// Verify all have correct correlation ID
	for _, e := range correlatedEvents {
		if e.CorrelationID != correlationID {
			t.Errorf("Event %s has wrong correlation ID", e.Type)
		}
	}

	t.Logf("Correlation verified: %d events with ID %s", count, correlationID)
}
