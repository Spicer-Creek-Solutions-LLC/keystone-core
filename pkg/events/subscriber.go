package events

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// JetStreamSubscriber implements EventSubscriber using NATS JetStream
type JetStreamSubscriber struct {
	js               nats.JetStreamContext
	subscriptions    map[string]*nats.Subscription  // ID -> NATS subscription
	wrapperSubs      map[string]*Subscription        // ID -> wrapper subscription
	mu               sync.RWMutex
	closed           bool
}

// NewJetStreamSubscriber creates a new JetStream-based event subscriber
func NewJetStreamSubscriber(js nats.JetStreamContext) (*JetStreamSubscriber, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context is required")
	}

	return &JetStreamSubscriber{
		js:            js,
		subscriptions: make(map[string]*nats.Subscription),
		wrapperSubs:   make(map[string]*Subscription),
	}, nil
}

// Subscribe subscribes to events matching the subject pattern
// Subject pattern examples:
//   - "agent.connect" - specific event type
//   - "agent.*" - all agent events
//   - "*.error" - all error events
//   - "*" - all events
func (s *JetStreamSubscriber) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	return s.subscribe(subject, "", handler)
}

// SubscribeQueue subscribes with queue group (load-balanced across multiple subscribers)
func (s *JetStreamSubscriber) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	if queue == "" {
		return nil, fmt.Errorf("queue name is required for queue subscriptions")
	}
	return s.subscribe(subject, queue, handler)
}

// subscribe is the internal subscription method
func (s *JetStreamSubscriber) subscribe(subject string, queue string, handler EventHandler) (*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("subscriber is closed")
	}

	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	if handler == nil {
		return nil, fmt.Errorf("handler is required")
	}

	// Generate subscription ID
	subID := uuid.New().String()

	// Build full subject with prefix
	fullSubject := fmt.Sprintf("%s.%s", SubjectPrefix, subject)

	// Create message handler that deserializes events
	msgHandler := func(msg *nats.Msg) {
		// Deserialize event from JSON
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			// Log error but don't fail - ack the message to prevent redelivery
			fmt.Printf("Failed to unmarshal event from %s: %v\n", msg.Subject, err)
			msg.Ack()
			return
		}

		// Call the event handler
		if err := handler(&event); err != nil {
			// Handler failed - don't ack, message will be redelivered
			fmt.Printf("Event handler failed for %s: %v\n", event.ID, err)
			msg.Nak()
			return
		}

		// Handler succeeded - ack the message
		msg.Ack()
	}

	// Subscribe using JetStream
	var natsSub *nats.Subscription
	var err error

	if queue != "" {
		// Queue subscription with durable consumer
		// Use queue name as durable name so all queue members share the same consumer
		durableName := fmt.Sprintf("titan-events-queue-%s", queue)
		natsSub, err = s.js.QueueSubscribe(
			fullSubject,
			queue,
			msgHandler,
			nats.Durable(durableName),
			nats.DeliverAll(), // Deliver all available messages
			nats.ManualAck(),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(3),
		)
	} else {
		// Regular subscription with durable consumer
		// Use unique durable name for each subscriber
		durableName := fmt.Sprintf("titan-events-%s", subID[:8])
		natsSub, err = s.js.Subscribe(
			fullSubject,
			msgHandler,
			nats.Durable(durableName),
			nats.DeliverAll(), // Deliver all available messages
			nats.ManualAck(),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(3),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", fullSubject, err)
	}

	// Store the NATS subscription
	s.subscriptions[subID] = natsSub

	// Create Subscription wrapper
	sub := &Subscription{
		ID:      subID,
		Subject: subject,
		Queue:   queue,
		Active:  true,
		unsubscribe: func() error {
			s.mu.Lock()
			defer s.mu.Unlock()

			if natsSub, ok := s.subscriptions[subID]; ok {
				delete(s.subscriptions, subID)
				delete(s.wrapperSubs, subID)
				return natsSub.Unsubscribe()
			}
			return nil
		},
	}

	// Store the wrapper subscription
	s.wrapperSubs[subID] = sub

	return sub, nil
}

// SubscribeWithFilter subscribes to events matching both the subject and filter
func (s *JetStreamSubscriber) SubscribeWithFilter(subject string, filter *EventFilter, handler EventHandler) (*Subscription, error) {
	// Wrap handler with filter
	filteredHandler := func(event *Event) error {
		if filter != nil && !filter.Matches(event) {
			// Event doesn't match filter - skip it
			return nil
		}
		return handler(event)
	}

	return s.Subscribe(subject, filteredHandler)
}

// SubscribeQueueWithFilter subscribes with queue group and filter
func (s *JetStreamSubscriber) SubscribeQueueWithFilter(subject string, queue string, filter *EventFilter, handler EventHandler) (*Subscription, error) {
	// Wrap handler with filter
	filteredHandler := func(event *Event) error {
		if filter != nil && !filter.Matches(event) {
			// Event doesn't match filter - skip it
			return nil
		}
		return handler(event)
	}

	return s.SubscribeQueue(subject, queue, filteredHandler)
}

// Close closes the subscriber and all active subscriptions
func (s *JetStreamSubscriber) Close() error {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return nil
	}

	s.closed = true

	// Copy wrapper subscriptions to avoid holding lock during unsubscribe
	subs := make([]*Subscription, 0, len(s.wrapperSubs))
	for _, sub := range s.wrapperSubs {
		subs = append(subs, sub)
	}

	s.mu.Unlock()

	// Unsubscribe from all active subscriptions (without holding lock)
	var lastErr error
	for _, sub := range subs {
		if err := sub.Unsubscribe(); err != nil {
			fmt.Printf("Error unsubscribing from %s: %v\n", sub.ID, err)
			lastErr = err
		}
	}

	return lastErr
}

// GetActiveSubscriptionCount returns the number of active subscriptions
func (s *JetStreamSubscriber) GetActiveSubscriptionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscriptions)
}
