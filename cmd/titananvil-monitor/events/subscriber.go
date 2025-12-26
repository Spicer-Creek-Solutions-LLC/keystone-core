package events

import (
	"context"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nats-io/nats.go"
	"github.com/titananvil/titan-anvil/pkg/events"
)

// EventMsg is a Bubble Tea message containing an event
type EventMsg struct {
	Event *events.Event
	Err   error
}

// Subscriber manages event subscriptions for the TUI
type Subscriber struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	sub     *events.JetStreamSubscriber
	program *tea.Program
	mu      sync.Mutex
	closed  bool
}

// New creates a new event subscriber
func New(ctx context.Context, natsURL string, program *tea.Program) (*Subscriber, error) {
	// Connect to NATS
	nc, err := nats.Connect(natsURL,
		nats.Name("titananvil-monitor"),
		nats.MaxReconnects(-1),  // Infinite reconnects
		nats.ReconnectWait(1000), // 1 second between reconnects
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Create JetStream subscriber
	jsSub, err := events.NewJetStreamSubscriber(js)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream subscriber: %w", err)
	}

	s := &Subscriber{
		nc:      nc,
		js:      js,
		sub:     jsSub,
		program: program,
	}

	return s, nil
}

// Subscribe subscribes to events matching the pattern and sends them to the TUI
func (s *Subscriber) Subscribe(pattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("subscriber is closed")
	}

	// Create event handler that sends events to the TUI
	handler := func(event *events.Event) error {
		// Send event to Bubble Tea program
		s.program.Send(EventMsg{Event: event})
		return nil
	}

	// Subscribe to the pattern
	_, err := s.sub.Subscribe(pattern, handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", pattern, err)
	}

	return nil
}

// SubscribeMultiple subscribes to multiple patterns
func (s *Subscriber) SubscribeMultiple(patterns []string) error {
	for _, pattern := range patterns {
		if err := s.Subscribe(pattern); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the subscriber and all subscriptions
func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Close JetStream subscriber
	if s.sub != nil {
		if err := s.sub.Close(); err != nil {
			return err
		}
	}

	// Close NATS connection
	if s.nc != nil {
		s.nc.Close()
	}

	return nil
}

// IsConnected returns whether the NATS connection is active
func (s *Subscriber) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.nc == nil {
		return false
	}

	return s.nc.IsConnected()
}
