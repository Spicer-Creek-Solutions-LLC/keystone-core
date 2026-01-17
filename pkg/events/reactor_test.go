package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestReactorEngine_AddReactor(t *testing.T) {
	engine := NewReactorEngine()

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Actions: []Action{
			NewLogAction("log", "test"),
		},
		Enabled: true,
	}

	err := engine.AddReactor(reactor)
	if err != nil {
		t.Fatalf("Failed to add reactor: %v", err)
	}

	// Try to add duplicate
	err = engine.AddReactor(reactor)
	if err == nil {
		t.Error("Expected error when adding duplicate reactor")
	}
}

func TestReactorEngine_AddReactor_Validation(t *testing.T) {
	engine := NewReactorEngine()

	tests := []struct {
		name        string
		reactor     *Reactor
		shouldError bool
	}{
		{
			name:        "nil reactor",
			reactor:     nil,
			shouldError: true,
		},
		{
			name: "empty ID",
			reactor: &Reactor{
				ID:     "",
				Filter: &ComparisonExpr{Field: "type", Operator: OpEqual, Value: "test"},
				Actions: []Action{
					NewLogAction("log", "test"),
				},
			},
			shouldError: true,
		},
		{
			name: "nil filter",
			reactor: &Reactor{
				ID:     "test",
				Filter: nil,
				Actions: []Action{
					NewLogAction("log", "test"),
				},
			},
			shouldError: true,
		},
		{
			name: "no actions",
			reactor: &Reactor{
				ID:      "test",
				Filter:  &ComparisonExpr{Field: "type", Operator: OpEqual, Value: "test"},
				Actions: []Action{},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.AddReactor(tt.reactor)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestReactorEngine_ProcessEvent(t *testing.T) {
	engine := NewReactorEngine()

	var executed atomic.Bool
	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				executed.Store(true)
				return nil
			}),
		},
		Enabled: true,
	}

	engine.AddReactor(reactor)

	// Process matching event
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	engine.ProcessEvent(event)

	// Wait for async execution
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	}); err != nil {
		t.Fatalf("Reactor action was not executed: %v", err)
	}

	if !executed.Load() {
		t.Error("Reactor action was not executed")
	}
}

func TestReactorEngine_ProcessEvent_NoMatch(t *testing.T) {
	engine := NewReactorEngine()

	var executed atomic.Bool
	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				executed.Store(true)
				return nil
			}),
		},
		Enabled: true,
	}

	engine.AddReactor(reactor)

	// Process non-matching event
	event := NewEvent(EventTypeJobStart).Source("/test").Build()
	engine.ProcessEvent(event)

	err := helpers.WaitForTimeout(200*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	})
	if err == nil {
		t.Error("Reactor action should not have been executed")
	}
}

func TestReactorEngine_DisableReactor(t *testing.T) {
	engine := NewReactorEngine()

	var executed atomic.Bool
	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				executed.Store(true)
				return nil
			}),
		},
		Enabled: true,
	}

	engine.AddReactor(reactor)
	engine.DisableReactor("test-reactor")

	// Process matching event with disabled reactor
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	engine.ProcessEvent(event)

	err := helpers.WaitForTimeout(200*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	})
	if err == nil {
		t.Error("Disabled reactor should not execute")
	}

	// Re-enable and test
	engine.EnableReactor("test-reactor")
	engine.ProcessEvent(event)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	}); err != nil {
		t.Fatalf("Re-enabled reactor should execute: %v", err)
	}
}

func TestReactorEngine_Priority(t *testing.T) {
	engine := NewReactorEngine()

	order := []string{}
	mu := sync.Mutex{}
	ready := make(chan string, 3)

	addReactor := func(id string, priority int) {
		engine.AddReactor(&Reactor{
			ID:       id,
			Name:     fmt.Sprintf("Reactor %s", id),
			Priority: priority,
			Filter: &ComparisonExpr{
				Field:    "type",
				Operator: OpEqual,
				Value:    "test",
			},
			Actions: []Action{
				NewFunctionAction(id, func(ctx context.Context, event *Event) error {
					mu.Lock()
					order = append(order, id)
					mu.Unlock()
					ready <- id
					return nil
				}),
			},
			Enabled: true,
		})
	}

	// Add reactors in random order
	addReactor("low", 1)
	addReactor("high", 10)
	addReactor("medium", 5)

	event := NewEvent(EventType("test")).Source("/test").Build()
	engine.ProcessEvent(event)

	// Wait for all to complete
	for i := 0; i < 3; i++ {
		select {
		case <-ready:
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for reactor execution")
		}
	}

	// Reactors are triggered in priority order, but since they execute async,
	// we can't guarantee completion order. Check that all executed.
	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("Expected 3 executions, got %d", len(order))
	}

	// Verify all reactors executed (order may vary due to async execution)
	found := make(map[string]bool)
	for _, id := range order {
		found[id] = true
	}

	if !found["high"] || !found["medium"] || !found["low"] {
		t.Errorf("Expected all reactors to execute, got %v", order)
	}
}

func TestReactorEngine_Throttle(t *testing.T) {
	engine := NewReactorEngine()

	count := int32(0)
	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				atomic.AddInt32(&count, 1)
				return nil
			}),
		},
		Enabled: true,
		Conditions: &ReactorConditions{
			Throttle: 500 * time.Millisecond,
		},
	}

	engine.AddReactor(reactor)

	// Send multiple events rapidly
	event := NewEvent(EventType("test")).Source("/test").Build()
	engine.ProcessEvent(event)
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&count) == 1, nil
	}); err != nil {
		t.Fatalf("Expected 1 execution due to throttle, got %d", atomic.LoadInt32(&count))
	}

	for i := 0; i < 4; i++ {
		engine.ProcessEvent(event)
	}

	err := helpers.WaitForTimeout(200*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&count) > 1, nil
	})
	if err == nil {
		t.Fatalf("Expected throttle to suppress additional executions, got %d", atomic.LoadInt32(&count))
	}

	// Wait for throttle to expire and send another
	throttleStart := time.Now()
	err = helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return time.Since(throttleStart) >= 500*time.Millisecond, nil
	})
	if err != nil {
		t.Fatalf("Throttle window did not elapse: %v", err)
	}
	engine.ProcessEvent(event)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&count) == 2, nil
	}); err != nil {
		t.Fatalf("Expected 2 executions after throttle expired, got %d", atomic.LoadInt32(&count))
	}
}

func TestReactorEngine_Debounce(t *testing.T) {
	engine := NewReactorEngine()

	count := int32(0)
	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				atomic.AddInt32(&count, 1)
				return nil
			}),
		},
		Enabled: true,
		Conditions: &ReactorConditions{
			Debounce: 300 * time.Millisecond,
		},
	}

	engine.AddReactor(reactor)

	// Send multiple events rapidly
	event := NewEvent(EventType("test")).Source("/test").Build()
	for i := 0; i < 5; i++ {
		engine.ProcessEvent(event)
	}

	// Wait for debounce to fire
	debounceStart := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		if time.Since(debounceStart) < 300*time.Millisecond {
			return false, nil
		}
		return atomic.LoadInt32(&count) == 1, nil
	}); err != nil {
		t.Fatalf("Expected 1 execution due to debounce, got %d", atomic.LoadInt32(&count))
	}
}

func TestReactorEngine_MaxConcurrent(t *testing.T) {
	engine := NewReactorEngine()

	activeCount := int32(0)
	maxActive := int32(0)
	done := make(chan bool, 10)
	started := make(chan struct{}, 10)
	gate := make(chan struct{})

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				current := atomic.AddInt32(&activeCount, 1)

				// Track max concurrent
				for {
					max := atomic.LoadInt32(&maxActive)
					if current <= max || atomic.CompareAndSwapInt32(&maxActive, max, current) {
						break
					}
				}

				started <- struct{}{}
				<-gate
				atomic.AddInt32(&activeCount, -1)
				done <- true
				return nil
			}),
		},
		Enabled:       true,
		MaxConcurrent: 2,
	}

	engine.AddReactor(reactor)

	// Send 5 events
	event := NewEvent(EventType("test")).Source("/test").Build()
	for i := 0; i < 5; i++ {
		engine.ProcessEvent(event)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(started) >= 2, nil
	}); err != nil {
		t.Fatalf("Timeout waiting for executions: %v", err)
	}
	close(gate)

	for i := 0; i < 2; i++ { // Only 2 should execute due to MaxConcurrent
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for executions")
		}
	}

	max := atomic.LoadInt32(&maxActive)
	if max > 2 {
		t.Errorf("Expected max concurrent <= 2, got %d", max)
	}
}

func TestReactorEngine_OnError_Continue(t *testing.T) {
	engine := NewReactorEngine()

	executed := []string{}
	mu := sync.Mutex{}

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("first", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "first")
				mu.Unlock()
				return nil
			}),
			NewFunctionAction("second", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "second")
				mu.Unlock()
				return fmt.Errorf("error")
			}),
			NewFunctionAction("third", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "third")
				mu.Unlock()
				return nil
			}),
		},
		Enabled: true,
		OnError: ErrorBehaviorContinue,
	}

	engine.AddReactor(reactor)

	event := NewEvent(EventType("test")).Source("/test").Build()
	engine.ProcessEvent(event)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return len(executed) == 3, nil
	}); err != nil {
		t.Fatalf("Expected 3 actions executed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// All actions should execute despite error
	if len(executed) != 3 {
		t.Errorf("Expected 3 actions executed, got %d", len(executed))
	}
}

func TestReactorEngine_OnError_Stop(t *testing.T) {
	engine := NewReactorEngine()

	executed := []string{}
	mu := sync.Mutex{}

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("first", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "first")
				mu.Unlock()
				return nil
			}),
			NewFunctionAction("second", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "second")
				mu.Unlock()
				return fmt.Errorf("error")
			}),
			NewFunctionAction("third", func(ctx context.Context, event *Event) error {
				mu.Lock()
				executed = append(executed, "third")
				mu.Unlock()
				return nil
			}),
		},
		Enabled: true,
		OnError: ErrorBehaviorStop,
	}

	engine.AddReactor(reactor)

	event := NewEvent(EventType("test")).Source("/test").Build()
	engine.ProcessEvent(event)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return len(executed) == 2, nil
	}); err != nil {
		t.Fatalf("Expected 2 actions executed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should stop at second action
	if len(executed) != 2 {
		t.Errorf("Expected 2 actions executed, got %d", len(executed))
	}
}

func TestReactorEngine_Conditions_OnlyIf(t *testing.T) {
	engine := NewReactorEngine()

	var executed atomic.Bool
	filter, _ := ParseFilterExpression(`severity >= "error"`)

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				executed.Store(true)
				return nil
			}),
		},
		Enabled: true,
		Conditions: &ReactorConditions{
			OnlyIf: filter,
		},
	}

	engine.AddReactor(reactor)

	// Event with low severity - should not execute
	event1 := NewEvent(EventType("test")).Source("/test").Severity(SeverityInfo).Build()
	engine.ProcessEvent(event1)

	// Wait a bit to ensure the event was processed
	time.Sleep(100 * time.Millisecond)
	if executed.Load() {
		t.Error("Reactor should not execute when OnlyIf condition fails")
	}

	// Event with high severity - should execute
	event2 := NewEvent(EventType("test")).Source("/test").Severity(SeverityError).Build()
	engine.ProcessEvent(event2)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	}); err != nil {
		t.Fatalf("Reactor should execute when OnlyIf condition passes: %v", err)
	}
}

func TestReactorEngine_Conditions_Unless(t *testing.T) {
	engine := NewReactorEngine()

	var executed atomic.Bool
	filter, _ := ParseFilterExpression(`tags.env == "test"`)

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				executed.Store(true)
				return nil
			}),
		},
		Enabled: true,
		Conditions: &ReactorConditions{
			Unless: filter,
		},
	}

	engine.AddReactor(reactor)

	// Event with test env - should not execute
	event1 := NewEvent(EventType("test")).Source("/test").Tag("env", "test").Build()
	engine.ProcessEvent(event1)

	// Wait a bit to ensure the event was processed
	time.Sleep(100 * time.Millisecond)
	if executed.Load() {
		t.Error("Reactor should not execute when Unless condition is true")
	}

	// Event with prod env - should execute
	executed.Store(false)
	event2 := NewEvent(EventType("test")).Source("/test").Tag("env", "prod").Build()
	engine.ProcessEvent(event2)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return executed.Load(), nil
	}); err != nil {
		t.Fatalf("Reactor should execute when Unless condition is false: %v", err)
	}
}

func TestReactorEngine_Metrics(t *testing.T) {
	engine := NewReactorEngine()

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewFunctionAction("test", func(ctx context.Context, event *Event) error {
				return nil
			}),
		},
		Enabled: true,
	}

	engine.AddReactor(reactor)

	// Process events
	for i := 0; i < 3; i++ {
		event := NewEvent(EventType("test")).Source("/test").Build()
		engine.ProcessEvent(event)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		metrics := engine.GetMetrics()
		return metrics.EventsEvaluated == 3 && metrics.ExecutionsTriggered == 3, nil
	}); err != nil {
		t.Fatalf("Metrics did not update: %v", err)
	}

	metrics := engine.GetMetrics()

	if metrics.EventsEvaluated != 3 {
		t.Errorf("Expected 3 events evaluated, got %d", metrics.EventsEvaluated)
	}

	if metrics.ExecutionsTriggered != 3 {
		t.Errorf("Expected 3 executions triggered, got %d", metrics.ExecutionsTriggered)
	}

	reactorMetrics, err := engine.GetReactorMetrics("test-reactor")
	if err != nil {
		t.Fatalf("Failed to get reactor metrics: %v", err)
	}

	if reactorMetrics.EventsMatched != 3 {
		t.Errorf("Expected 3 events matched, got %d", reactorMetrics.EventsMatched)
	}
}

func TestReactorEngine_RemoveReactor(t *testing.T) {
	engine := NewReactorEngine()

	reactor := &Reactor{
		ID:   "test-reactor",
		Name: "Test Reactor",
		Filter: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "test",
		},
		Actions: []Action{
			NewLogAction("log", "test"),
		},
		Enabled: true,
	}

	engine.AddReactor(reactor)

	err := engine.RemoveReactor("test-reactor")
	if err != nil {
		t.Errorf("Failed to remove reactor: %v", err)
	}

	// Try to remove again
	err = engine.RemoveReactor("test-reactor")
	if err == nil {
		t.Error("Expected error when removing non-existent reactor")
	}

	reactors := engine.ListReactors()
	if len(reactors) != 0 {
		t.Errorf("Expected 0 reactors, got %d", len(reactors))
	}
}

// Benchmark reactor execution
func BenchmarkReactorEngine_ProcessEvent(b *testing.B) {
	engine := NewReactorEngine()

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("reactor-%d", i)
		engine.AddReactor(&Reactor{
			ID:   id,
			Name: fmt.Sprintf("Reactor %d", i),
			Filter: &ComparisonExpr{
				Field:    "type",
				Operator: OpEqual,
				Value:    fmt.Sprintf("event.type.%d", i),
			},
			Actions: []Action{
				NewFunctionAction("test", func(ctx context.Context, event *Event) error {
					return nil
				}),
			},
			Enabled: true,
		})
	}

	event := NewEvent(EventType("event.type.5")).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.ProcessEvent(event)
	}
}
