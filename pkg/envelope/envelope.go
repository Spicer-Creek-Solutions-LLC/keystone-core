package envelope

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrDuplicate is the sentinel returned by a publish path that has
// already seen the envelope's MessageID within the active dedup
// window (Epic 05 task 6). Dedup is producer-side and a defensive
// safety net; ErrDuplicate from PublishEnvelope means "the system
// already has this message — your retry was suppressed."
//
// Lives here rather than in internal/nats so callers above the NATS
// layer (controlplane, future SDK consumers) can errors.Is against
// it without importing internal/nats.
var ErrDuplicate = errors.New("envelope: duplicate message")

// Priority is the wire-level message priority. v1.0 carries it as
// metadata only — routing/queueing by priority is reserved for a
// future epic. The four-value enum matches PROJECT-DETAILS §4.2.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// validPriorities is the canonical set; centralized so Priority's
// Validate and Envelope.Validate stay in lockstep.
var validPriorities = map[Priority]struct{}{
	PriorityLow:      {},
	PriorityNormal:   {},
	PriorityHigh:     {},
	PriorityCritical: {},
}

// Validate returns an error if p is not one of the four enum values.
func (p Priority) Validate() error {
	if _, ok := validPriorities[p]; !ok {
		return fmt.Errorf("envelope: priority %q (must be one of low|normal|high|critical)", string(p))
	}
	return nil
}

// Envelope wraps a Keystone Core NATS message. Field tags pin the
// JSON wire shape so future cross-language SDKs (Epic 06+ agent;
// external Rust/Python clients eventually) interoperate without
// reading this file.
type Envelope struct {
	MessageID     string          `json:"message_id"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Priority      Priority        `json:"priority"`
	TTLMillis     int64           `json:"ttl_ms,omitempty"`
	ClusterPrefix string          `json:"cluster_prefix"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// Option mutates an Envelope under construction. Used by New so
// callers stamp only the fields that differ from defaults.
type Option func(*Envelope)

// WithMessageID overrides the auto-generated UUID. Used by tests for
// determinism and by replay tooling that wants stable IDs.
func WithMessageID(id string) Option {
	return func(e *Envelope) { e.MessageID = id }
}

// WithCorrelationID stamps a correlation ID linking a request to its
// response. CommandDispatcher sets this to the command's ID so the
// agent's response (Epic 06) can be matched without a separate
// lookup.
func WithCorrelationID(id string) Option {
	return func(e *Envelope) { e.CorrelationID = id }
}

// WithPriority overrides the default Normal priority.
func WithPriority(p Priority) Option {
	return func(e *Envelope) { e.Priority = p }
}

// WithTTL stamps a TTL. Zero (or omitted) means "no TTL" on the
// wire. Negative durations are rejected by Validate. Subscribers
// enforce the TTL — Epic 06's agent runtime is the first.
func WithTTL(d time.Duration) Option {
	return func(e *Envelope) {
		e.TTLMillis = d.Milliseconds()
	}
}

// New constructs an Envelope around payload, fills MessageID with a
// fresh UUID, defaults Priority=Normal, and applies opts in order.
// ClusterPrefix is required and must match the publishing Manager's
// SubjectBuilder.Prefix() (the publish path enforces this).
//
// payload may be nil; encode it as JSON before passing if you want
// the inner shape to be human-readable on the wire (json.RawMessage
// preserves it as-is).
func New(payload []byte, clusterPrefix string, opts ...Option) Envelope {
	env := Envelope{
		MessageID:     uuid.NewString(),
		Priority:      PriorityNormal,
		ClusterPrefix: clusterPrefix,
		Payload:       json.RawMessage(payload),
	}
	for _, opt := range opts {
		opt(&env)
	}
	return env
}

// TTL returns the duration form of TTLMillis. Zero means "no TTL".
func (e Envelope) TTL() time.Duration {
	return time.Duration(e.TTLMillis) * time.Millisecond
}

// Validate returns an error if any required field is missing or
// invalid. Marshal calls Validate so a malformed envelope cannot
// reach the wire even if a caller skips the constructor.
//
// MessageID and CorrelationID are restricted to printable ASCII as
// defense-in-depth against dedup-cache hash-input ambiguity. UUIDs
// are hex; this is a tighter check than necessary but cheap and
// blocks a malicious WithMessageID caller from injecting bytes that
// could collide with a crafted subject in dedup keying.
func (e Envelope) Validate() error {
	if e.MessageID == "" {
		return errors.New("envelope: message_id must not be empty")
	}
	if err := validatePrintable("message_id", e.MessageID); err != nil {
		return err
	}
	if e.CorrelationID != "" {
		if err := validatePrintable("correlation_id", e.CorrelationID); err != nil {
			return err
		}
	}
	if err := e.Priority.Validate(); err != nil {
		return err
	}
	if e.TTLMillis < 0 {
		return fmt.Errorf("envelope: ttl_ms must not be negative, got %d", e.TTLMillis)
	}
	if e.ClusterPrefix == "" {
		return errors.New("envelope: cluster_prefix must not be empty")
	}
	return nil
}

// validatePrintable rejects any byte outside printable ASCII
// (0x21-0x7E). Whitespace and control bytes are out so the field
// is unambiguous in concatenation and safe to log without escaping.
func validatePrintable(field, s string) error {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c >= 0x7F {
			return fmt.Errorf("envelope: %s contains non-printable byte at index %d", field, i)
		}
	}
	return nil
}

// Marshal validates and serializes to JSON. The codec is JSON for
// v1.0; if a binary path is ever needed it'll be a separate function
// (MarshalProto, MarshalCBOR) to keep this one stable.
func (e Envelope) Marshal() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("envelope: marshal: %w", err)
	}
	return b, nil
}

// Unmarshal parses JSON-wire bytes into an Envelope and validates
// it. Subscribers (Epic 06) call this on every received message.
func Unmarshal(b []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return Envelope{}, fmt.Errorf("envelope: unmarshal: %w", err)
	}
	if err := env.Validate(); err != nil {
		return Envelope{}, err
	}
	return env, nil
}
