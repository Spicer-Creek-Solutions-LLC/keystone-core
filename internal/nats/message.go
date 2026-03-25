// Package nats provides NATS messaging infrastructure for Keystone Core.
package nats

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// MessagePriority defines the priority level for messages
type MessagePriority int

const (
	// PriorityLow is for background/non-urgent messages
	PriorityLow MessagePriority = 0
	// PriorityNormal is the default priority
	PriorityNormal MessagePriority = 1
	// PriorityHigh is for time-sensitive operations
	PriorityHigh MessagePriority = 2
	// PriorityCritical is for urgent operations that must be processed immediately
	PriorityCritical MessagePriority = 3
)

// String returns the string representation of the priority
func (p MessagePriority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// MessageType identifies the type of message being sent
type MessageType string

// MessageTypeAgentHeartbeat constants define the supported types.
const (
	// Agent message types
	MessageTypeAgentRegister  MessageType = "agent.register"
	MessageTypeAgentHeartbeat MessageType = "agent.heartbeat"
	MessageTypeAgentCommand   MessageType = "agent.command"
	MessageTypeAgentResponse  MessageType = "agent.response"
	MessageTypeAgentState     MessageType = "agent.state"
	MessageTypeAgentEvent     MessageType = "agent.event"

	// Server message types
	MessageTypeServerAnnounce MessageType = "server.announce"
	MessageTypeServerControl  MessageType = "server.control"

	// Discovery message types
	MessageTypeDiscoveryRequest  MessageType = "discovery.request"
	MessageTypeDiscoveryResponse MessageType = "discovery.response"

	// Bootstrap message types
	MessageTypeBootstrapRegister MessageType = "bootstrap.register"
	MessageTypeBootstrapResponse MessageType = "bootstrap.response"
)

// Envelope wraps all NATS messages with routing metadata.
// This provides:
// - Message deduplication via MessageID
// - Request/response correlation via CorrelationID
// - Message prioritization
// - TTL for message expiration
// - Routing information for superclusters
type Envelope struct {
	// MessageID is a unique identifier for this message (for deduplication)
	MessageID string `json:"message_id"`

	// CorrelationID links related messages together (e.g., request/response)
	CorrelationID string `json:"correlation_id,omitempty"`

	// Type identifies the message type
	Type MessageType `json:"type"`

	// Priority indicates message processing priority
	Priority MessagePriority `json:"priority"`

	// Source identifies where the message originated
	Source string `json:"source"`

	// Destination is the intended recipient (agent ID, server ID, etc.)
	Destination string `json:"destination,omitempty"`

	// Cluster is the logical cluster name for supercluster routing
	Cluster string `json:"cluster"`

	// Timestamp is when the message was created
	Timestamp time.Time `json:"timestamp"`

	// TTL is how long the message is valid (zero means no expiry)
	TTL time.Duration `json:"ttl,omitempty"`

	// Payload contains the actual message data (serialized protobuf or JSON)
	Payload []byte `json:"payload"`

	// Headers contains additional metadata
	Headers map[string]string `json:"headers,omitempty"`

	// TraceID is the distributed tracing trace ID
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the distributed tracing span ID
	SpanID string `json:"span_id,omitempty"`
}

// EnvelopeBuilder provides a fluent interface for creating message envelopes
type EnvelopeBuilder struct {
	envelope *Envelope
}

// NewEnvelope creates a new envelope builder with required fields
func NewEnvelope(msgType MessageType, source, cluster string) *EnvelopeBuilder {
	return &EnvelopeBuilder{
		envelope: &Envelope{
			MessageID: generateMessageID(),
			Type:      msgType,
			Source:    source,
			Cluster:   cluster,
			Priority:  PriorityNormal,
			Timestamp: time.Now().UTC(),
			Headers:   make(map[string]string),
		},
	}
}

// CorrelationID sets the correlation ID for linking related messages
func (b *EnvelopeBuilder) CorrelationID(id string) *EnvelopeBuilder {
	b.envelope.CorrelationID = id
	return b
}

// Destination sets the intended recipient
func (b *EnvelopeBuilder) Destination(dest string) *EnvelopeBuilder {
	b.envelope.Destination = dest
	return b
}

// Priority sets the message priority
func (b *EnvelopeBuilder) Priority(p MessagePriority) *EnvelopeBuilder {
	b.envelope.Priority = p
	return b
}

// TTL sets the message time-to-live
func (b *EnvelopeBuilder) TTL(ttl time.Duration) *EnvelopeBuilder {
	b.envelope.TTL = ttl
	return b
}

// Payload sets the message payload (protobuf or JSON bytes)
func (b *EnvelopeBuilder) Payload(data []byte) *EnvelopeBuilder {
	b.envelope.Payload = data
	return b
}

// PayloadJSON serializes an object to JSON and sets it as the payload
func (b *EnvelopeBuilder) PayloadJSON(v interface{}) *EnvelopeBuilder {
	data, err := json.Marshal(v)
	if err == nil {
		b.envelope.Payload = data
	}
	return b
}

// Header adds a custom header
func (b *EnvelopeBuilder) Header(key, value string) *EnvelopeBuilder {
	b.envelope.Headers[key] = value
	return b
}

// Headers sets multiple headers at once
func (b *EnvelopeBuilder) Headers(headers map[string]string) *EnvelopeBuilder {
	for k, v := range headers {
		b.envelope.Headers[k] = v
	}
	return b
}

// Trace sets the distributed tracing IDs
func (b *EnvelopeBuilder) Trace(traceID, spanID string) *EnvelopeBuilder {
	b.envelope.TraceID = traceID
	b.envelope.SpanID = spanID
	return b
}

// Build returns the constructed envelope
func (b *EnvelopeBuilder) Build() *Envelope {
	return b.envelope
}

// generateMessageID creates a unique message ID
func generateMessageID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// IsExpired returns true if the message has exceeded its TTL
func (e *Envelope) IsExpired() bool {
	if e.TTL == 0 {
		return false
	}
	return time.Since(e.Timestamp) > e.TTL
}

// Age returns how long ago the message was created
func (e *Envelope) Age() time.Duration {
	return time.Since(e.Timestamp)
}

// RemainingTTL returns the remaining time before the message expires
// Returns 0 if the message has no TTL or has already expired
func (e *Envelope) RemainingTTL() time.Duration {
	if e.TTL == 0 {
		return 0
	}
	remaining := e.TTL - time.Since(e.Timestamp)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Serialize encodes the envelope to JSON bytes
func (e *Envelope) Serialize() ([]byte, error) {
	return json.Marshal(e)
}

// Deserialize decodes an envelope from JSON bytes
func Deserialize(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to deserialize envelope: %w", err)
	}
	return &env, nil
}

// ToNATSMsg converts an envelope to a NATS message with headers
func (e *Envelope) ToNATSMsg(subject string) (*nats.Msg, error) {
	data, err := e.Serialize()
	if err != nil {
		return nil, err
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}

	// Add standard headers for efficient routing/filtering without parsing the payload
	msg.Header.Set("Kscore-Message-ID", e.MessageID)
	msg.Header.Set("Kscore-Message-Type", string(e.Type))
	msg.Header.Set("Kscore-Priority", e.Priority.String())
	msg.Header.Set("Kscore-Cluster", e.Cluster)
	msg.Header.Set("Kscore-Source", e.Source)

	if e.CorrelationID != "" {
		msg.Header.Set("Kscore-Correlation-ID", e.CorrelationID)
	}
	if e.Destination != "" {
		msg.Header.Set("Kscore-Destination", e.Destination)
	}
	if e.TraceID != "" {
		msg.Header.Set("Kscore-Trace-ID", e.TraceID)
	}
	if e.SpanID != "" {
		msg.Header.Set("Kscore-Span-ID", e.SpanID)
	}

	return msg, nil
}

// FromNATSMsg parses an envelope from a NATS message
func FromNATSMsg(msg *nats.Msg) (*Envelope, error) {
	return Deserialize(msg.Data)
}

// DeduplicationTracker tracks message IDs for deduplication
type DeduplicationTracker struct {
	seen    map[string]time.Time
	window  time.Duration
	maxSize int
}

// NewDeduplicationTracker creates a new deduplication tracker
// window is how long to remember message IDs
// maxSize is the maximum number of IDs to track (0 = unlimited)
func NewDeduplicationTracker(window time.Duration, maxSize int) *DeduplicationTracker {
	return &DeduplicationTracker{
		seen:    make(map[string]time.Time),
		window:  window,
		maxSize: maxSize,
	}
}

// IsDuplicate checks if a message ID has been seen before.
// If not seen, records it and returns false.
// If seen, returns true.
func (d *DeduplicationTracker) IsDuplicate(messageID string) bool {
	now := time.Now()

	// Clean up expired entries periodically
	if len(d.seen) > 0 && len(d.seen)%100 == 0 {
		d.cleanup(now)
	}

	// Check if we've seen this message before
	if seenAt, exists := d.seen[messageID]; exists {
		if now.Sub(seenAt) <= d.window {
			return true
		}
		// Entry expired, will be replaced below
	}

	// Enforce max size if needed
	if d.maxSize > 0 && len(d.seen) >= d.maxSize {
		d.cleanup(now)
		// If still at capacity, remove oldest
		if len(d.seen) >= d.maxSize {
			d.removeOldest()
		}
	}

	// Record this message
	d.seen[messageID] = now
	return false
}

// cleanup removes expired entries
func (d *DeduplicationTracker) cleanup(now time.Time) {
	for id, seenAt := range d.seen {
		if now.Sub(seenAt) > d.window {
			delete(d.seen, id)
		}
	}
}

// removeOldest removes the oldest entry
func (d *DeduplicationTracker) removeOldest() {
	var oldestID string
	var oldestTime time.Time
	first := true

	for id, seenAt := range d.seen {
		if first || seenAt.Before(oldestTime) {
			oldestID = id
			oldestTime = seenAt
			first = false
		}
	}

	if oldestID != "" {
		delete(d.seen, oldestID)
	}
}

// Size returns the number of tracked message IDs
func (d *DeduplicationTracker) Size() int {
	return len(d.seen)
}

// Clear removes all tracked message IDs
func (d *DeduplicationTracker) Clear() {
	d.seen = make(map[string]time.Time)
}

// MessageHandler is a function that handles received envelopes
type MessageHandler func(env *Envelope, msg *nats.Msg) error

// EnvelopeHandler wraps a MessageHandler with envelope parsing and deduplication
type EnvelopeHandler struct {
	handler MessageHandler
	dedup   *DeduplicationTracker
}

// NewEnvelopeHandler creates a new envelope handler with optional deduplication
func NewEnvelopeHandler(handler MessageHandler, dedupWindow time.Duration) *EnvelopeHandler {
	var dedup *DeduplicationTracker
	if dedupWindow > 0 {
		dedup = NewDeduplicationTracker(dedupWindow, 10000) // Track up to 10k messages
	}
	return &EnvelopeHandler{
		handler: handler,
		dedup:   dedup,
	}
}

// Handle processes a NATS message by parsing the envelope and calling the handler
func (h *EnvelopeHandler) Handle(msg *nats.Msg) {
	env, err := FromNATSMsg(msg)
	if err != nil {
		slog.Error("failed to parse envelope", "error", err)
		return
	}

	// Check for duplicates
	if h.dedup != nil && h.dedup.IsDuplicate(env.MessageID) {
		return // Silently ignore duplicates
	}

	// Check TTL
	if env.IsExpired() {
		return // Silently ignore expired messages
	}

	// Call the actual handler
	if err := h.handler(env, msg); err != nil {
		slog.Error("handler error for message", "message_id", env.MessageID, "error", err)
	}
}

// NATSHandler returns a nats.MsgHandler for use with Subscribe
func (h *EnvelopeHandler) NATSHandler() nats.MsgHandler {
	return h.Handle
}
