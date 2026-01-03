package events

import (
	"crypto/rand"
	"encoding/binary"
	"sync/atomic"
	"time"
)

// eventCounter is used to ensure unique event IDs even when created in the same nanosecond
var eventCounter uint64

// EventType represents the type of event
type EventType string

// Event types organized by category
const (
	// Agent events
	EventTypeAgentConnect    EventType = "agent.connect"
	EventTypeAgentDisconnect EventType = "agent.disconnect"
	EventTypeAgentHeartbeat  EventType = "agent.heartbeat"
	EventTypeAgentError      EventType = "agent.error"

	// Job events
	EventTypeJobStart    EventType = "job.start"
	EventTypeJobComplete EventType = "job.complete"
	EventTypeJobFail     EventType = "job.fail"
	EventTypeJobOutput   EventType = "job.output"

	// State events
	EventTypeStateApplyStart EventType = "state.apply.start"
	EventTypeStateApplyDone  EventType = "state.apply.done"
	EventTypeStateApplyFail  EventType = "state.apply.fail"
	EventTypeStateChange     EventType = "state.change"
	EventTypeStateDrift      EventType = "state.drift"

	// User events
	EventTypeUserLogin   EventType = "user.login"
	EventTypeUserCommand EventType = "user.command"
	EventTypeUserError   EventType = "user.error"

	// System events
	EventTypeSystemStartup  EventType = "system.startup"
	EventTypeSystemShutdown EventType = "system.shutdown"
	EventTypeSystemError    EventType = "system.error"

	// Policy events
	EventTypePolicyPass      EventType = "policy.pass"
	EventTypePolicyViolation EventType = "policy.violation"
)

// Severity levels for events
type Severity string

const (
	SeverityDebug    Severity = "debug"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Event represents a Keystone Core event
type Event struct {
	// ID is a unique identifier for this event
	ID string `json:"id"`

	// Type is the event type (e.g., "agent.connect", "state.change")
	Type EventType `json:"type"`

	// Source identifies where the event originated (e.g., "/agents/web-01", "/control-plane")
	Source string `json:"source"`

	// Time is when the event occurred
	Time time.Time `json:"time"`

	// Severity indicates the importance of the event
	Severity Severity `json:"severity"`

	// CorrelationID links related events together
	CorrelationID string `json:"correlation_id,omitempty"`

	// Tags are key-value pairs for filtering and categorization
	Tags map[string]string `json:"tags,omitempty"`

	// Data contains event-specific payload
	Data map[string]interface{} `json:"data,omitempty"`

	// Subject is the NATS subject this event is published to
	Subject string `json:"subject,omitempty"`
}

// EventPublisher publishes events to the event bus
type EventPublisher interface {
	// Publish publishes an event
	Publish(event *Event) error

	// PublishAsync publishes an event asynchronously
	PublishAsync(event *Event) error

	// Close closes the publisher
	Close() error
}

// EventSubscriber subscribes to events from the event bus
type EventSubscriber interface {
	// Subscribe subscribes to events matching the subject pattern
	Subscribe(subject string, handler EventHandler) (*Subscription, error)

	// SubscribeQueue subscribes with queue group (load-balanced)
	SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error)

	// Close closes the subscriber
	Close() error
}

// EventHandler is called when an event is received
type EventHandler func(event *Event) error

// Subscription represents an active event subscription
type Subscription struct {
	// ID is the subscription identifier
	ID string

	// Subject is the NATS subject pattern
	Subject string

	// Queue is the queue group name (if any)
	Queue string

	// Active indicates if the subscription is active
	Active bool

	// unsubscribe is the function to call to unsubscribe
	unsubscribe func() error
}

// Unsubscribe cancels the subscription
func (s *Subscription) Unsubscribe() error {
	if s.unsubscribe != nil {
		s.Active = false
		return s.unsubscribe()
	}
	return nil
}

// EventFilter filters events based on criteria
type EventFilter struct {
	// Types filters by event type (exact match or glob pattern)
	Types []EventType

	// Sources filters by source (exact match or glob pattern)
	Sources []string

	// Tags filters by tag key-value pairs (all must match)
	Tags map[string]string

	// Severity filters by minimum severity level
	Severity Severity

	// Since filters events after this time
	Since *time.Time

	// Until filters events before this time
	Until *time.Time
}

// Matches returns true if the event matches this filter
func (f *EventFilter) Matches(event *Event) bool {
	// Check event type
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check source
	if len(f.Sources) > 0 {
		found := false
		for _, s := range f.Sources {
			if event.Source == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check tags
	if len(f.Tags) > 0 {
		for key, value := range f.Tags {
			if eventValue, ok := event.Tags[key]; !ok || eventValue != value {
				return false
			}
		}
	}

	// Check severity
	if f.Severity != "" {
		if !severityAtLeast(event.Severity, f.Severity) {
			return false
		}
	}

	// Check time range
	if f.Since != nil && event.Time.Before(*f.Since) {
		return false
	}
	if f.Until != nil && event.Time.After(*f.Until) {
		return false
	}

	return true
}

// severityAtLeast returns true if actual severity is at least the minimum level
func severityAtLeast(actual, minimum Severity) bool {
	levels := map[Severity]int{
		SeverityDebug:    0,
		SeverityInfo:     1,
		SeverityWarning:  2,
		SeverityError:    3,
		SeverityCritical: 4,
	}

	actualLevel, ok1 := levels[actual]
	minLevel, ok2 := levels[minimum]

	if !ok1 || !ok2 {
		return false
	}

	return actualLevel >= minLevel
}

// EventBuilder helps construct events with a fluent API
type EventBuilder struct {
	event *Event
}

// NewEvent creates a new event builder
func NewEvent(eventType EventType) *EventBuilder {
	return &EventBuilder{
		event: &Event{
			ID:       generateEventID(),
			Type:     eventType,
			Time:     time.Now(),
			Severity: SeverityInfo,
			Tags:     make(map[string]string),
			Data:     make(map[string]interface{}),
		},
	}
}

// Source sets the event source
func (b *EventBuilder) Source(source string) *EventBuilder {
	b.event.Source = source
	return b
}

// Severity sets the event severity
func (b *EventBuilder) Severity(severity Severity) *EventBuilder {
	b.event.Severity = severity
	return b
}

// CorrelationID sets the correlation ID
func (b *EventBuilder) CorrelationID(id string) *EventBuilder {
	b.event.CorrelationID = id
	return b
}

// Tag adds a tag
func (b *EventBuilder) Tag(key, value string) *EventBuilder {
	b.event.Tags[key] = value
	return b
}

// Data sets a data field
func (b *EventBuilder) Data(key string, value interface{}) *EventBuilder {
	b.event.Data[key] = value
	return b
}

// DataMap sets multiple data fields
func (b *EventBuilder) DataMap(data map[string]interface{}) *EventBuilder {
	for k, v := range data {
		b.event.Data[k] = v
	}
	return b
}

// Build returns the constructed event
func (b *EventBuilder) Build() *Event {
	// Set subject based on event type if not already set
	if b.event.Subject == "" {
		b.event.Subject = string(b.event.Type)
	}
	return b.event
}

// generateEventID generates a unique event ID
func generateEventID() string {
	counter := atomic.AddUint64(&eventCounter, 1)
	return "evt-" + time.Now().Format("20060102150405") + "-" + randString(8) + "-" + encodeCounter(counter)
}

// encodeCounter encodes a counter as a short string
func encodeCounter(n uint64) string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	result := make([]byte, 0, 8)
	for n > 0 {
		result = append(result, chars[n%36])
		n /= 36
	}
	return string(result)
}

// randString generates a random string of length n using crypto/rand
func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	// Read random bytes
	randBytes := make([]byte, n)
	if _, err := rand.Read(randBytes); err != nil {
		// Fallback to time-based if crypto/rand fails
		var seed uint64
		binary.Read(rand.Reader, binary.LittleEndian, &seed)
		for i := range b {
			b[i] = letters[(seed+uint64(i))%uint64(len(letters))]
		}
		return string(b)
	}
	for i := range b {
		b[i] = letters[randBytes[i]%byte(len(letters))]
	}
	return string(b)
}
