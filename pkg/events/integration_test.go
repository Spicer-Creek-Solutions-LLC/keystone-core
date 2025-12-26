package events

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_PublishSubscribe(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	// Create event manager
	manager, err := NewManager(js)
	require.NoError(t, err)
	defer manager.Close()

	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe to all events (use > to match all tokens)
	sub, err := manager.Subscribe(">", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish various events
	events := []*Event{
		NewEvent(EventTypeAgentConnect).
			Source("/agents/web-01").
			Severity(SeverityInfo).
			Tag("env", "production").
			Data("hostname", "web-01.example.com").
			Build(),

		NewEvent(EventTypeJobStart).
			Source("/control-plane").
			Severity(SeverityInfo).
			Data("job_id", "job-123").
			Data("command", "apt-get update").
			Build(),

		NewEvent(EventTypeStateApplyStart).
			Source("/control-plane").
			Severity(SeverityInfo).
			Data("agent_id", "web-01").
			Build(),
	}

	for _, event := range events {
		err := manager.Publish(event)
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify all events were received
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, len(events), len(receivedEvents))
}

func TestIntegration_EventPersistence(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	// Publish events
	for i := 0; i < 5; i++ {
		event := NewEvent(EventTypeAgentHeartbeat).
			Source(fmt.Sprintf("/agents/agent-%d", i)).
			Build()
		err := publisher.Publish(event)
		require.NoError(t, err)
	}

	// Create a new subscriber after events were published
	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe with DeliverAll policy to receive all persisted events
	sub, err := subscriber.Subscribe("agent.heartbeat", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Wait for events to be delivered
	time.Sleep(1 * time.Second)

	// Should receive all persisted events
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 5, len(receivedEvents))
}

func TestIntegration_MultipleSubscribers(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	// Create multiple subscribers
	subscriber1, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber1.Close()

	subscriber2, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber2.Close()

	var mu1, mu2 sync.Mutex
	var events1, events2 []*Event

	// Both subscribe to same events (broadcast)
	sub1, err := subscriber1.Subscribe("state.*", func(event *Event) error {
		mu1.Lock()
		defer mu1.Unlock()
		events1 = append(events1, event)
		return nil
	})
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	sub2, err := subscriber2.Subscribe("state.*", func(event *Event) error {
		mu2.Lock()
		defer mu2.Unlock()
		events2 = append(events2, event)
		return nil
	})
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish events
	for i := 0; i < 3; i++ {
		event := NewEvent(EventTypeStateChange).
			Source("/test").
			Build()
		publisher.Publish(event)
	}

	time.Sleep(500 * time.Millisecond)

	// Both subscribers should receive all events
	mu1.Lock()
	mu2.Lock()
	defer mu1.Unlock()
	defer mu2.Unlock()

	assert.Equal(t, 3, len(events1))
	assert.Equal(t, 3, len(events2))
}

func TestIntegration_WildcardSubscription(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	manager, err := NewManager(js)
	require.NoError(t, err)
	defer manager.Close()

	var mu sync.Mutex
	categoryCount := make(map[string]int)

	// Subscribe to all agent events
	sub, err := manager.Subscribe("agent.*", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		categoryCount["agent"]++
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish various events
	testEvents := []EventType{
		EventTypeAgentConnect,
		EventTypeAgentDisconnect,
		EventTypeAgentHeartbeat,
		EventTypeAgentError,
		EventTypeJobStart,    // Should NOT match
		EventTypeStateChange, // Should NOT match
	}

	for _, eventType := range testEvents {
		event := NewEvent(eventType).Source("/test").Build()
		manager.Publish(event)
	}

	time.Sleep(500 * time.Millisecond)

	// Should only receive agent events
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 4, categoryCount["agent"])
}

func TestIntegration_ErrorHandling(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	var mu sync.Mutex
	attemptCount := make(map[string]int)

	// Handler that fails on first attempt
	sub, err := subscriber.Subscribe("job.*", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()

		attemptCount[event.ID]++

		// Fail first 2 attempts, succeed on 3rd
		if attemptCount[event.ID] < 3 {
			return fmt.Errorf("simulated error")
		}
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish an event
	event := NewEvent(EventTypeJobStart).
		Source("/test").
		Data("job_id", "test-123").
		Build()

	err = publisher.Publish(event)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(2 * time.Second)

	// Should have been attempted 3 times
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, attemptCount[event.ID])
}

func TestIntegration_CorrelatedEvents(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	manager, err := NewManager(js)
	require.NoError(t, err)
	defer manager.Close()

	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe to all events (use > to match all tokens)
	sub, err := manager.Subscribe(">", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish correlated events (simulating a job lifecycle)
	correlationID := "job-workflow-123"

	events := []*Event{
		NewEvent(EventTypeJobStart).
			Source("/control-plane").
			CorrelationID(correlationID).
			Data("job_id", "job-123").
			Build(),

		NewEvent(EventTypeJobOutput).
			Source("/agents/web-01").
			CorrelationID(correlationID).
			Data("output", "Starting task...").
			Build(),

		NewEvent(EventTypeJobComplete).
			Source("/control-plane").
			CorrelationID(correlationID).
			Data("job_id", "job-123").
			Data("exit_code", 0).
			Build(),
	}

	for _, event := range events {
		manager.Publish(event)
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify all events have same correlation ID
	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 3, len(receivedEvents))
	for _, event := range receivedEvents {
		assert.Equal(t, correlationID, event.CorrelationID)
	}
}

func TestIntegration_HighThroughput(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	manager, err := NewManager(js)
	require.NoError(t, err)
	defer manager.Close()

	var mu sync.Mutex
	var receivedCount int

	// Subscribe to all events
	sub, err := manager.Subscribe(">", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedCount++
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish many events
	eventCount := 100
	for i := 0; i < eventCount; i++ {
		event := NewEvent(EventTypeAgentHeartbeat).
			Source(fmt.Sprintf("/agents/agent-%d", i%10)).
			Data("sequence", i).
			Build()

		err := manager.PublishAsync(event)
		require.NoError(t, err)
	}

	// Wait for async publishes to complete
	select {
	case <-js.PublishAsyncComplete():
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for async publishes")
	}

	// Wait for all events to be received
	time.Sleep(2 * time.Second)

	// Should receive all events
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, eventCount, receivedCount)
}
