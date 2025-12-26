package events

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

// Manager manages event publishing and subscription
type Manager struct {
	publisher  EventPublisher
	subscriber EventSubscriber
}

// NewManager creates a new event manager from a JetStream context
func NewManager(js nats.JetStreamContext) (*Manager, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context is required")
	}

	// Create publisher
	publisher, err := NewJetStreamPublisher(js)
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}

	// Create subscriber
	subscriber, err := NewJetStreamSubscriber(js)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscriber: %w", err)
	}

	return &Manager{
		publisher:  publisher,
		subscriber: subscriber,
	}, nil
}

// Publisher returns the event publisher
func (m *Manager) Publisher() EventPublisher {
	return m.publisher
}

// Subscriber returns the event subscriber
func (m *Manager) Subscriber() EventSubscriber {
	return m.subscriber
}

// Publish publishes an event synchronously
func (m *Manager) Publish(event *Event) error {
	return m.publisher.Publish(event)
}

// PublishAsync publishes an event asynchronously
func (m *Manager) PublishAsync(event *Event) error {
	return m.publisher.PublishAsync(event)
}

// Subscribe subscribes to events matching the subject pattern
func (m *Manager) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	return m.subscriber.Subscribe(subject, handler)
}

// SubscribeQueue subscribes with queue group (load-balanced)
func (m *Manager) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	return m.subscriber.SubscribeQueue(subject, queue, handler)
}

// Close closes the event manager and all resources
func (m *Manager) Close() error {
	var lastErr error

	if err := m.subscriber.Close(); err != nil {
		fmt.Printf("Error closing subscriber: %v\n", err)
		lastErr = err
	}

	if err := m.publisher.Close(); err != nil {
		fmt.Printf("Error closing publisher: %v\n", err)
		lastErr = err
	}

	return lastErr
}
