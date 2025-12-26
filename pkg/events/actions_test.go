package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogAction(t *testing.T) {
	logged := false
	action := NewLogAction("test-log", "Test message")
	action.SetLogger(func(format string, args ...interface{}) {
		logged = true
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("LogAction failed: %v", err)
	}

	if !logged {
		t.Error("Expected log to be called")
	}

	if action.Type() != "log" {
		t.Errorf("Expected type=log, got %s", action.Type())
	}
}

func TestEventAction(t *testing.T) {
	var published *Event
	mockPublisher := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			published = event
			return nil
		},
	}

	action := NewEventAction("test-event", mockPublisher, EventType("custom.event")).
		SetSeverity(SeverityWarning).
		SetSource("/custom-source").
		SetDataTemplate(map[string]interface{}{
			"custom_field": "value",
		})

	triggerEvent := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), triggerEvent)

	if err != nil {
		t.Fatalf("EventAction failed: %v", err)
	}

	if published == nil {
		t.Fatal("Expected event to be published")
	}

	if published.Type != "custom.event" {
		t.Errorf("Expected type=custom.event, got %s", published.Type)
	}

	if published.Severity != SeverityWarning {
		t.Errorf("Expected severity=warning, got %s", published.Severity)
	}

	if published.Source != "/custom-source" {
		t.Errorf("Expected source=/custom-source, got %s", published.Source)
	}

	if published.Data["custom_field"] != "value" {
		t.Error("Expected custom_field in data")
	}

	if published.Data["trigger_event_id"] != triggerEvent.ID {
		t.Error("Expected trigger_event_id in data")
	}
}

func TestWebhookAction(t *testing.T) {
	// Create test server
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	action := NewWebhookAction("test-webhook", server.URL).
		SetHeader("X-Custom-Header", "test-value").
		SetTimeout(5 * time.Second)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("WebhookAction failed: %v", err)
	}

	if !received {
		t.Error("Expected webhook to be called")
	}

	if action.Type() != "webhook" {
		t.Errorf("Expected type=webhook, got %s", action.Type())
	}
}

func TestWebhookAction_Error(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	action := NewWebhookAction("test-webhook", server.URL)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err == nil {
		t.Error("Expected error from webhook")
	}
}

func TestCommandAction(t *testing.T) {
	action := NewCommandAction("test-command", "echo", "hello").
		SetTimeout(5 * time.Second)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("CommandAction failed: %v", err)
	}

	if action.Type() != "command" {
		t.Errorf("Expected type=command, got %s", action.Type())
	}
}

func TestCommandAction_Error(t *testing.T) {
	action := NewCommandAction("test-command", "false") // 'false' command always fails

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err == nil {
		t.Error("Expected error from command")
	}
}

func TestFunctionAction(t *testing.T) {
	executed := false
	action := NewFunctionAction("test-function", func(ctx context.Context, event *Event) error {
		executed = true
		return nil
	})

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := action.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("FunctionAction failed: %v", err)
	}

	if !executed {
		t.Error("Expected function to be called")
	}

	if action.Type() != "function" {
		t.Errorf("Expected type=function, got %s", action.Type())
	}
}

func TestConditionalAction(t *testing.T) {
	trueCalled := false
	falseCalled := false

	filter, _ := ParseFilterExpression(`severity >= "error"`)

	trueAction := NewFunctionAction("true", func(ctx context.Context, event *Event) error {
		trueCalled = true
		return nil
	})

	falseAction := NewFunctionAction("false", func(ctx context.Context, event *Event) error {
		falseCalled = true
		return nil
	})

	action := NewConditionalAction("test-conditional", filter, trueAction, falseAction)

	// Test with high severity (true branch)
	event1 := NewEvent(EventTypeJobFail).Source("/test").Severity(SeverityError).Build()
	action.Execute(context.Background(), event1)

	if !trueCalled {
		t.Error("Expected true action to be called")
	}
	if falseCalled {
		t.Error("Expected false action not to be called")
	}

	// Reset
	trueCalled = false
	falseCalled = false

	// Test with low severity (false branch)
	event2 := NewEvent(EventTypeJobStart).Source("/test").Severity(SeverityInfo).Build()
	action.Execute(context.Background(), event2)

	if trueCalled {
		t.Error("Expected true action not to be called")
	}
	if !falseCalled {
		t.Error("Expected false action to be called")
	}
}

func TestSequenceAction(t *testing.T) {
	order := []string{}

	action1 := NewFunctionAction("first", func(ctx context.Context, event *Event) error {
		order = append(order, "first")
		return nil
	})

	action2 := NewFunctionAction("second", func(ctx context.Context, event *Event) error {
		order = append(order, "second")
		return nil
	})

	action3 := NewFunctionAction("third", func(ctx context.Context, event *Event) error {
		order = append(order, "third")
		return nil
	})

	sequence := NewSequenceAction("test-sequence", action1, action2, action3)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := sequence.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("SequenceAction failed: %v", err)
	}

	expected := []string{"first", "second", "third"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d actions, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("Expected order[%d]=%s, got %s", i, v, order[i])
		}
	}
}

func TestSequenceAction_StopOnError(t *testing.T) {
	order := []string{}

	action1 := NewFunctionAction("first", func(ctx context.Context, event *Event) error {
		order = append(order, "first")
		return nil
	})

	action2 := NewFunctionAction("second", func(ctx context.Context, event *Event) error {
		order = append(order, "second")
		return fmt.Errorf("error")
	})

	action3 := NewFunctionAction("third", func(ctx context.Context, event *Event) error {
		order = append(order, "third")
		return nil
	})

	sequence := NewSequenceAction("test-sequence", action1, action2, action3)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := sequence.Execute(context.Background(), event)

	if err == nil {
		t.Error("Expected error from sequence")
	}

	// Should stop at second action
	if len(order) != 2 {
		t.Errorf("Expected 2 actions executed, got %d", len(order))
	}
}

func TestParallelAction(t *testing.T) {
	count := int32(0)

	actions := []Action{}
	for i := 0; i < 3; i++ {
		actions = append(actions, NewFunctionAction(fmt.Sprintf("action-%d", i), func(ctx context.Context, event *Event) error {
			atomic.AddInt32(&count, 1)
			time.Sleep(50 * time.Millisecond)
			return nil
		}))
	}

	parallel := NewParallelAction("test-parallel", actions...)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	start := time.Now()
	err := parallel.Execute(context.Background(), event)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("ParallelAction failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 actions executed, got %d", count)
	}

	// Should complete in ~50ms (parallel) not ~150ms (sequential)
	if duration > 100*time.Millisecond {
		t.Errorf("Expected parallel execution, took %v", duration)
	}
}

func TestParallelAction_Error(t *testing.T) {
	action1 := NewFunctionAction("success", func(ctx context.Context, event *Event) error {
		return nil
	})

	action2 := NewFunctionAction("fail", func(ctx context.Context, event *Event) error {
		return fmt.Errorf("error")
	})

	parallel := NewParallelAction("test-parallel", action1, action2)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := parallel.Execute(context.Background(), event)

	if err == nil {
		t.Error("Expected error from parallel action")
	}
}

func TestDelayAction(t *testing.T) {
	delay := NewDelayAction("test-delay", 100*time.Millisecond)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	start := time.Now()
	err := delay.Execute(context.Background(), event)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("DelayAction failed: %v", err)
	}

	if duration < 100*time.Millisecond {
		t.Errorf("Expected delay of at least 100ms, got %v", duration)
	}
}

func TestDelayAction_ContextCancellation(t *testing.T) {
	delay := NewDelayAction("test-delay", 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := delay.Execute(ctx, event)

	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestRetryAction(t *testing.T) {
	attempts := int32(0)

	failingAction := NewFunctionAction("failing", func(ctx context.Context, event *Event) error {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 3 {
			return fmt.Errorf("attempt %d failed", attempt)
		}
		return nil
	})

	retry := NewRetryAction("test-retry", failingAction, 3).
		SetBackoff(10*time.Millisecond, 100*time.Millisecond, 2.0)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := retry.Execute(context.Background(), event)

	if err != nil {
		t.Fatalf("RetryAction failed: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryAction_MaxRetriesExceeded(t *testing.T) {
	alwaysFailAction := NewFunctionAction("always-fail", func(ctx context.Context, event *Event) error {
		return fmt.Errorf("always fails")
	})

	retry := NewRetryAction("test-retry", alwaysFailAction, 2).
		SetBackoff(10*time.Millisecond, 100*time.Millisecond, 2.0)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	err := retry.Execute(context.Background(), event)

	if err == nil {
		t.Error("Expected error after max retries")
	}
}

// Benchmark actions
func BenchmarkLogAction(b *testing.B) {
	action := NewLogAction("test", "test message")
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		action.Execute(context.Background(), event)
	}
}

func BenchmarkFunctionAction(b *testing.B) {
	action := NewFunctionAction("test", func(ctx context.Context, event *Event) error {
		return nil
	})
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		action.Execute(context.Background(), event)
	}
}

func BenchmarkSequenceAction(b *testing.B) {
	actions := []Action{}
	for i := 0; i < 5; i++ {
		actions = append(actions, NewFunctionAction(fmt.Sprintf("action-%d", i), func(ctx context.Context, event *Event) error {
			return nil
		}))
	}

	sequence := NewSequenceAction("test", actions...)
	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sequence.Execute(context.Background(), event)
	}
}
