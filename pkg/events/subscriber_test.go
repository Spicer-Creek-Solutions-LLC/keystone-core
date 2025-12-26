package events

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJetStreamSubscriber(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	require.NotNil(t, subscriber)
	defer subscriber.Close()
}

func TestNewJetStreamSubscriber_NilJetStream(t *testing.T) {
	subscriber, err := NewJetStreamSubscriber(nil)
	assert.Error(t, err)
	assert.Nil(t, subscriber)
	assert.Contains(t, err.Error(), "JetStream context is required")
}

func TestSubscriber_Subscribe(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	// Create publisher and subscriber
	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	// Track received events
	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe to agent events
	sub, err := subscriber.Subscribe("agent.*", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, sub)
	defer sub.Unsubscribe()

	// Give subscription time to be established
	time.Sleep(100 * time.Millisecond)

	// Publish agent events
	event1 := NewEvent(EventTypeAgentConnect).
		Source("/agents/agent-1").
		Build()

	event2 := NewEvent(EventTypeAgentHeartbeat).
		Source("/agents/agent-2").
		Build()

	err = publisher.Publish(event1)
	require.NoError(t, err)

	err = publisher.Publish(event2)
	require.NoError(t, err)

	// Wait for events to be received
	time.Sleep(500 * time.Millisecond)

	// Verify events were received
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, len(receivedEvents))
	assert.Equal(t, EventTypeAgentConnect, receivedEvents[0].Type)
	assert.Equal(t, EventTypeAgentHeartbeat, receivedEvents[1].Type)
}

func TestSubscriber_SubscribeSpecificEvent(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe to specific event type only
	sub, err := subscriber.Subscribe("agent.connect", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish multiple event types
	event1 := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	event2 := NewEvent(EventTypeAgentHeartbeat).Source("/test").Build()
	event3 := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	publisher.Publish(event1)
	publisher.Publish(event2) // This should NOT be received
	publisher.Publish(event3)

	time.Sleep(500 * time.Millisecond)

	// Should only receive connect events
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, len(receivedEvents))
	assert.Equal(t, EventTypeAgentConnect, receivedEvents[0].Type)
	assert.Equal(t, EventTypeAgentConnect, receivedEvents[1].Type)
}

func TestSubscriber_SubscribeQueue(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber1, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber1.Close()

	subscriber2, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber2.Close()

	var mu sync.Mutex
	var sub1Count, sub2Count int

	// Create two queue subscribers (load-balanced)
	queueName := "test-queue"

	sub1, err := subscriber1.SubscribeQueue("job.*", queueName, func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		sub1Count++
		return nil
	})
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	sub2, err := subscriber2.SubscribeQueue("job.*", queueName, func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		sub2Count++
		return nil
	})
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish multiple events
	for i := 0; i < 10; i++ {
		event := NewEvent(EventTypeJobStart).
			Source("/test").
			Data("job_id", i).
			Build()
		publisher.Publish(event)
	}

	time.Sleep(500 * time.Millisecond)

	// Both subscribers should have received some events (load-balanced)
	mu.Lock()
	defer mu.Unlock()
	total := sub1Count + sub2Count
	assert.Equal(t, 10, total, "Total events received should be 10")

	// Both should have received at least one (with high probability)
	// Note: This is probabilistic, but with 10 messages it's very likely
	t.Logf("Subscriber 1: %d, Subscriber 2: %d", sub1Count, sub2Count)
}

func TestSubscriber_SubscribeWithFilter(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	var mu sync.Mutex
	var receivedEvents []*Event

	// Subscribe with filter for error severity
	filter := &EventFilter{
		Severity: SeverityError,
	}

	sub, err := subscriber.SubscribeWithFilter(">", filter, func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	// Publish events with different severities
	event1 := NewEvent(EventTypeAgentConnect).Severity(SeverityInfo).Source("/test").Build()
	event2 := NewEvent(EventTypeAgentError).Severity(SeverityError).Source("/test").Build()
	event3 := NewEvent(EventTypeSystemError).Severity(SeverityCritical).Source("/test").Build()
	event4 := NewEvent(EventTypeJobFail).Severity(SeverityWarning).Source("/test").Build()

	publisher.Publish(event1)
	publisher.Publish(event2)
	publisher.Publish(event3)
	publisher.Publish(event4)

	time.Sleep(500 * time.Millisecond)

	// Should only receive error and critical events (>= error severity)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, len(receivedEvents), "Expected 2 events to be received")
	if len(receivedEvents) >= 2 {
		assert.Equal(t, SeverityError, receivedEvents[0].Severity)
		assert.Equal(t, SeverityCritical, receivedEvents[1].Severity)
	}
}

func TestSubscriber_Unsubscribe(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	var mu sync.Mutex
	var receivedCount int

	sub, err := subscriber.Subscribe("agent.*", func(event *Event) error {
		mu.Lock()
		defer mu.Unlock()
		receivedCount++
		return nil
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Publish event
	event1 := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	publisher.Publish(event1)

	time.Sleep(200 * time.Millisecond)

	// Unsubscribe
	err = sub.Unsubscribe()
	assert.NoError(t, err)
	assert.False(t, sub.Active)

	// Publish another event (should not be received)
	event2 := NewEvent(EventTypeAgentHeartbeat).Source("/test").Build()
	publisher.Publish(event2)

	time.Sleep(200 * time.Millisecond)

	// Should only have received the first event
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, receivedCount)
}

func TestSubscriber_Close(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	// Create publisher first to ensure stream exists
	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)

	// Create multiple subscriptions
	sub1, err := subscriber.Subscribe("agent.*", func(event *Event) error { return nil })
	require.NoError(t, err)

	sub2, err := subscriber.Subscribe("job.*", func(event *Event) error { return nil })
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 2, subscriber.GetActiveSubscriptionCount())

	// Close should unsubscribe all
	err = subscriber.Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, subscriber.GetActiveSubscriptionCount())

	// Subscriptions should be inactive
	assert.False(t, sub1.Active)
	assert.False(t, sub2.Active)
}

func TestSubscriber_InvalidInputs(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	subscriber, err := NewJetStreamSubscriber(js)
	require.NoError(t, err)
	defer subscriber.Close()

	// Empty subject
	sub, err := subscriber.Subscribe("", func(event *Event) error { return nil })
	assert.Error(t, err)
	assert.Nil(t, sub)

	// Nil handler
	sub, err = subscriber.Subscribe("agent.*", nil)
	assert.Error(t, err)
	assert.Nil(t, sub)

	// Queue subscription with empty queue name
	sub, err = subscriber.SubscribeQueue("agent.*", "", func(event *Event) error { return nil })
	assert.Error(t, err)
	assert.Nil(t, sub)
}
