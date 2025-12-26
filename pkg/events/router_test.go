package events

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouter_AddRule(t *testing.T) {
	router := NewRouter()

	called := false
	handler := func(event *Event) error {
		called = true
		return nil
	}

	filter := &ComparisonExpr{
		Field:    "type",
		Operator: OpEqual,
		Value:    "agent.connect",
	}

	rule := &RoutingRule{
		ID:      "test-rule",
		Name:    "Test Rule",
		Filter:  filter,
		Handler: handler,
		Enabled: true,
	}

	err := router.AddRule(rule)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Try to add duplicate
	err = router.AddRule(rule)
	if err == nil {
		t.Error("Expected error when adding duplicate rule")
	}

	// Test routing
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err = router.Route(event)
	if err != nil {
		t.Errorf("Routing error: %v", err)
	}

	if !called {
		t.Error("Handler was not called")
	}
}

func TestRouter_AddRuleValidation(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		name        string
		rule        *RoutingRule
		shouldError bool
	}{
		{
			name:        "nil rule",
			rule:        nil,
			shouldError: true,
		},
		{
			name: "empty ID",
			rule: &RoutingRule{
				ID:      "",
				Filter:  &ComparisonExpr{Field: "type", Operator: OpEqual, Value: "test"},
				Handler: func(e *Event) error { return nil },
			},
			shouldError: true,
		},
		{
			name: "nil filter",
			rule: &RoutingRule{
				ID:      "test",
				Filter:  nil,
				Handler: func(e *Event) error { return nil },
			},
			shouldError: true,
		},
		{
			name: "nil handler",
			rule: &RoutingRule{
				ID:      "test",
				Filter:  &ComparisonExpr{Field: "type", Operator: OpEqual, Value: "test"},
				Handler: nil,
			},
			shouldError: true,
		},
		{
			name: "valid rule",
			rule: &RoutingRule{
				ID:      "valid",
				Filter:  &ComparisonExpr{Field: "type", Operator: OpEqual, Value: "test"},
				Handler: func(e *Event) error { return nil },
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.AddRule(tt.rule)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestRouter_AddRuleFromExpression(t *testing.T) {
	router := NewRouter()

	called := false
	handler := func(event *Event) error {
		called = true
		return nil
	}

	err := router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		handler,
	)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	router.Route(event)

	if !called {
		t.Error("Handler was not called")
	}
}

func TestRouter_RemoveRule(t *testing.T) {
	router := NewRouter()

	err := router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error { return nil },
	)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Remove rule
	err = router.RemoveRule("test-rule")
	if err != nil {
		t.Errorf("Failed to remove rule: %v", err)
	}

	// Try to remove again
	err = router.RemoveRule("test-rule")
	if err == nil {
		t.Error("Expected error when removing non-existent rule")
	}

	// Verify rule is gone
	rules := router.GetRules()
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules, got %d", len(rules))
	}
}

func TestRouter_EnableDisableRule(t *testing.T) {
	router := NewRouter()

	called := false
	handler := func(event *Event) error {
		called = true
		return nil
	}

	err := router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		handler,
	)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	// Disable rule
	router.DisableRule("test-rule")
	called = false
	router.Route(event)
	if called {
		t.Error("Handler should not be called when rule is disabled")
	}

	// Enable rule
	router.EnableRule("test-rule")
	called = false
	router.Route(event)
	if !called {
		t.Error("Handler should be called when rule is enabled")
	}
}

func TestRouter_Priority(t *testing.T) {
	router := NewRouter()

	var order []string
	mu := sync.Mutex{}

	addRule := func(id string, priority int) {
		router.AddRule(&RoutingRule{
			ID:       id,
			Name:     fmt.Sprintf("Rule %s", id),
			Priority: priority,
			Filter: &ComparisonExpr{
				Field:    "type",
				Operator: OpEqual,
				Value:    "test",
			},
			Handler: func(e *Event) error {
				mu.Lock()
				order = append(order, id)
				mu.Unlock()
				return nil
			},
			Enabled: true,
		})
	}

	// Add rules in random order
	addRule("low", 1)
	addRule("high", 10)
	addRule("medium", 5)

	event := NewEvent(EventType("test")).Source("/test").Build()
	router.Route(event)

	// Should be executed in priority order (highest first)
	expected := []string{"high", "medium", "low"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d handlers called, got %d", len(expected), len(order))
	}

	for i, id := range expected {
		if order[i] != id {
			t.Errorf("Expected order[%d] = %s, got %s", i, id, order[i])
		}
	}
}

func TestRouter_StopOnMatch(t *testing.T) {
	router := NewRouter()

	var called []string
	mu := sync.Mutex{}

	addRule := func(id string, stopOnMatch bool) {
		router.AddRule(&RoutingRule{
			ID:   id,
			Name: fmt.Sprintf("Rule %s", id),
			Filter: &ComparisonExpr{
				Field:    "type",
				Operator: OpEqual,
				Value:    "test",
			},
			Handler: func(e *Event) error {
				mu.Lock()
				called = append(called, id)
				mu.Unlock()
				return nil
			},
			Enabled:     true,
			StopOnMatch: stopOnMatch,
		})
	}

	addRule("first", true)  // Should stop after this
	addRule("second", false)
	addRule("third", false)

	event := NewEvent(EventType("test")).Source("/test").Build()
	router.Route(event)

	// Should only execute first rule
	if len(called) != 1 {
		t.Errorf("Expected 1 handler called, got %d", len(called))
	}
	if len(called) > 0 && called[0] != "first" {
		t.Errorf("Expected first handler to be called, got %s", called[0])
	}
}

func TestRouter_MultipleMatchingRules(t *testing.T) {
	router := NewRouter()

	count := int32(0)

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("rule-%d", i)
		router.AddRuleFromExpression(
			id,
			fmt.Sprintf("Rule %d", i),
			`type == "agent.connect"`,
			func(e *Event) error {
				atomic.AddInt32(&count, 1)
				return nil
			},
		)
	}

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	router.Route(event)

	if count != 3 {
		t.Errorf("Expected 3 handlers called, got %d", count)
	}

	metrics := router.GetMetrics()
	if metrics.EventsProcessed != 1 {
		t.Errorf("Expected 1 event processed, got %d", metrics.EventsProcessed)
	}
	if metrics.EventsMatched != 1 {
		t.Errorf("Expected 1 event matched, got %d", metrics.EventsMatched)
	}
	if metrics.TotalRoutings != 3 {
		t.Errorf("Expected 3 total routings, got %d", metrics.TotalRoutings)
	}
}

func TestRouter_NoMatchingRules(t *testing.T) {
	router := NewRouter()

	called := false
	router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error {
			called = true
			return nil
		},
	)

	// Send event that doesn't match
	event := NewEvent(EventTypeJobStart).Source("/test").Build()
	router.Route(event)

	if called {
		t.Error("Handler should not be called for non-matching event")
	}

	metrics := router.GetMetrics()
	if metrics.EventsUnmatched != 1 {
		t.Errorf("Expected 1 unmatched event, got %d", metrics.EventsUnmatched)
	}
}

func TestRouter_HandlerError(t *testing.T) {
	router := NewRouter()

	expectedErr := fmt.Errorf("handler error")
	router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error {
			return expectedErr
		},
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := router.Route(event)

	if err == nil {
		t.Error("Expected error from handler")
	}

	metrics := router.GetMetrics()
	if metrics.RoutingErrors != 1 {
		t.Errorf("Expected 1 routing error, got %d", metrics.RoutingErrors)
	}

	ruleMetrics, _ := router.GetRuleMetrics("test-rule")
	if ruleMetrics.Errors != 1 {
		t.Errorf("Expected 1 error for rule, got %d", ruleMetrics.Errors)
	}
}

func TestRouter_RouteAsync(t *testing.T) {
	router := NewRouter()

	done := make(chan bool)
	router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error {
			done <- true
			return nil
		},
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	router.RouteAsync(event)

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Async routing timed out")
	}
}

func TestRouter_Metrics(t *testing.T) {
	router := NewRouter()

	router.AddRuleFromExpression(
		"rule-1",
		"Rule 1",
		`type == "agent.connect"`,
		func(e *Event) error { return nil },
	)

	router.AddRuleFromExpression(
		"rule-2",
		"Rule 2",
		`type == "job.start"`,
		func(e *Event) error { return fmt.Errorf("error") },
	)

	// Send events
	router.Route(NewEvent(EventTypeAgentConnect).Source("/test").Build())
	router.Route(NewEvent(EventTypeJobStart).Source("/test").Build())
	router.Route(NewEvent(EventTypeAgentDisconnect).Source("/test").Build())

	metrics := router.GetMetrics()

	if metrics.EventsProcessed != 3 {
		t.Errorf("Expected 3 events processed, got %d", metrics.EventsProcessed)
	}
	if metrics.EventsMatched != 2 {
		t.Errorf("Expected 2 events matched, got %d", metrics.EventsMatched)
	}
	if metrics.EventsUnmatched != 1 {
		t.Errorf("Expected 1 event unmatched, got %d", metrics.EventsUnmatched)
	}
	if metrics.RoutingErrors != 1 {
		t.Errorf("Expected 1 routing error, got %d", metrics.RoutingErrors)
	}

	// Check rule metrics
	rule1Metrics, _ := router.GetRuleMetrics("rule-1")
	if rule1Metrics.Matched != 1 {
		t.Errorf("Expected 1 match for rule-1, got %d", rule1Metrics.Matched)
	}

	rule2Metrics, _ := router.GetRuleMetrics("rule-2")
	if rule2Metrics.Errors != 1 {
		t.Errorf("Expected 1 error for rule-2, got %d", rule2Metrics.Errors)
	}
}

func TestRouter_ResetMetrics(t *testing.T) {
	router := NewRouter()

	router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error { return nil },
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	router.Route(event)

	// Verify metrics
	metrics := router.GetMetrics()
	if metrics.EventsProcessed != 1 {
		t.Error("Expected events to be processed")
	}

	// Reset
	router.ResetMetrics()

	// Verify reset
	metrics = router.GetMetrics()
	if metrics.EventsProcessed != 0 {
		t.Errorf("Expected 0 events processed after reset, got %d", metrics.EventsProcessed)
	}
}

func TestFanOut(t *testing.T) {
	count := int32(0)

	handler1 := func(e *Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	}

	handler2 := func(e *Event) error {
		atomic.AddInt32(&count, 10)
		return nil
	}

	handler3 := func(e *Event) error {
		atomic.AddInt32(&count, 100)
		return nil
	}

	fanOut := FanOut(handler1, handler2, handler3)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	fanOut(event)

	if count != 111 {
		t.Errorf("Expected count=111, got %d", count)
	}
}

func TestFanOutAsync(t *testing.T) {
	count := int32(0)
	done := make(chan bool, 3)

	handler := func(delta int32) EventHandler {
		return func(e *Event) error {
			atomic.AddInt32(&count, delta)
			done <- true
			return nil
		}
	}

	fanOut := FanOutAsync(handler(1), handler(10), handler(100))

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	fanOut(event)

	// Wait for all handlers
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("Handler timed out")
		}
	}

	if count != 111 {
		t.Errorf("Expected count=111, got %d", count)
	}
}

func TestFilterHandler(t *testing.T) {
	called := false
	filter, _ := ParseFilterExpression(`type == "agent.connect"`)

	handler := FilterHandler(filter, func(e *Event) error {
		called = true
		return nil
	})

	// Matching event
	event1 := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	handler(event1)
	if !called {
		t.Error("Handler should be called for matching event")
	}

	// Non-matching event
	called = false
	event2 := NewEvent(EventTypeJobStart).Source("/test").Build()
	handler(event2)
	if called {
		t.Error("Handler should not be called for non-matching event")
	}
}

func TestConditionalHandler(t *testing.T) {
	filter, _ := ParseFilterExpression(`severity >= "error"`)

	trueCount := 0
	falseCount := 0

	handler := ConditionalHandler(
		filter,
		func(e *Event) error {
			trueCount++
			return nil
		},
		func(e *Event) error {
			falseCount++
			return nil
		},
	)

	// High severity
	event1 := NewEvent(EventTypeJobFail).Source("/test").Severity(SeverityError).Build()
	handler(event1)

	if trueCount != 1 {
		t.Errorf("Expected trueCount=1, got %d", trueCount)
	}

	// Low severity
	event2 := NewEvent(EventTypeJobStart).Source("/test").Severity(SeverityInfo).Build()
	handler(event2)

	if falseCount != 1 {
		t.Errorf("Expected falseCount=1, got %d", falseCount)
	}
}

func TestChainHandlers(t *testing.T) {
	order := []string{}

	handler1 := func(e *Event) error {
		order = append(order, "first")
		return nil
	}

	handler2 := func(e *Event) error {
		order = append(order, "second")
		return nil
	}

	handler3 := func(e *Event) error {
		order = append(order, "third")
		return nil
	}

	chain := ChainHandlers(handler1, handler2, handler3)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	chain(event)

	expected := []string{"first", "second", "third"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d handlers, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("Expected order[%d]=%s, got %s", i, v, order[i])
		}
	}
}

func TestChainHandlers_StopOnError(t *testing.T) {
	order := []string{}

	handler1 := func(e *Event) error {
		order = append(order, "first")
		return nil
	}

	handler2 := func(e *Event) error {
		order = append(order, "second")
		return fmt.Errorf("error")
	}

	handler3 := func(e *Event) error {
		order = append(order, "third")
		return nil
	}

	chain := ChainHandlers(handler1, handler2, handler3)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := chain(event)

	if err == nil {
		t.Error("Expected error from chain")
	}

	// Should stop at handler2
	if len(order) != 2 {
		t.Errorf("Expected 2 handlers executed, got %d", len(order))
	}
}

// Benchmark router performance
func BenchmarkRouter_Route(b *testing.B) {
	router := NewRouter()

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("rule-%d", i)
		router.AddRuleFromExpression(
			id,
			fmt.Sprintf("Rule %d", i),
			fmt.Sprintf(`type == "event.type.%d"`, i),
			func(e *Event) error { return nil },
		)
	}

	event := NewEvent(EventType("event.type.5")).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(event)
	}
}

func BenchmarkRouter_RouteNoMatch(b *testing.B) {
	router := NewRouter()

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("rule-%d", i)
		router.AddRuleFromExpression(
			id,
			fmt.Sprintf("Rule %d", i),
			fmt.Sprintf(`type == "event.type.%d"`, i),
			func(e *Event) error { return nil },
		)
	}

	event := NewEvent(EventType("event.type.nomatch")).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(event)
	}
}

func BenchmarkFanOut(b *testing.B) {
	handlers := make([]EventHandler, 10)
	for i := 0; i < 10; i++ {
		handlers[i] = func(e *Event) error { return nil }
	}

	fanOut := FanOut(handlers...)
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fanOut(event)
	}
}

// Test concurrent routing
func TestRouter_Concurrent(t *testing.T) {
	router := NewRouter()

	count := int32(0)
	router.AddRuleFromExpression(
		"test-rule",
		"Test Rule",
		`type == "agent.connect"`,
		func(e *Event) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	// Route concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			router.Route(event)
		}()
	}

	wg.Wait()

	if count != 100 {
		t.Errorf("Expected count=100, got %d", count)
	}
}
