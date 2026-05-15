package events

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ErrPublisherBufferFull is returned from [EventPublisher.PublishAsync]
// when the async buffer is at capacity and the configured flush
// timeout elapses without space. Mirrors the JetStream stream's
// DiscardNew backpressure semantics at the client side — callers see
// the failure rather than silently dropping events.
var ErrPublisherBufferFull = errors.New("events: publisher async buffer full")

// EventPublisher is the producer half of the v1.0 event bus. Task 3's
// [JetStreamPublisher] is the canonical implementation; [NoopPublisher]
// is the disabled-mode shim for deployments that don't run the event
// bus.
//
// Lifecycle: Start once, then Publish / PublishAsync any number of
// times, then Stop once. Start MUST be called before any publish — a
// publish on a stopped publisher returns [ErrPublisherNotStarted].
// Stop is idempotent — calling it on a never-started or already-stopped
// publisher is a no-op.
//
// Publish (sync) returns the publish ack error directly; failures are
// the caller's to surface. PublishAsync queues the event for
// background flush and returns immediately on enqueue; failures
// during the background flush surface via the
// [WithAsyncErrorCallback] callback (when set) and the
// [JetStreamPublisher.FailedPublishes] counter.
type EventPublisher interface {
	// Start prepares the publisher for use. Idempotent across
	// Stop / Start cycles but not within them — calling Start twice
	// without an intervening Stop returns an error.
	Start(ctx context.Context) error

	// Publish synchronously validates, persists (if a store is
	// configured), and publishes the event. Returns the first error
	// in that chain. Empty [Event.Subject] is stamped via
	// [Event.StampSubject] before publish.
	Publish(ctx context.Context, e Event) error

	// PublishAsync queues the event for background publish. Returns
	// immediately on successful enqueue. Returns
	// [ErrPublisherBufferFull] when the buffer is at capacity and
	// the configured flush timeout elapses. Per-event validation
	// happens synchronously — invalid events are rejected without
	// hitting the queue.
	PublishAsync(ctx context.Context, e Event) error

	// Stop signals shutdown, drains pending async events up to the
	// caller's context deadline, and releases resources. Idempotent.
	Stop(ctx context.Context) error
}

// AsyncErrorCallback is the per-event error notifier invoked by the
// async worker when a background publish fails. The callback runs on
// the worker goroutine and MUST be reentrant-safe / non-blocking —
// the next event is drained immediately after the callback returns.
//
// Nil callback (the default) is allowed; the publisher falls back to
// a single [slog.Warn] per failure with the event ID, type, and
// underlying error. The [JetStreamPublisher.FailedPublishes] counter
// is incremented either way so metrics + tests can observe failures
// without registering a callback.
type AsyncErrorCallback func(e Event, err error)

// PublisherOption configures a [JetStreamPublisher] at construction
// time. Functional-options pattern matches the project convention
// (Manager, Backend, etc.); zero options yields a publisher with
// sensible defaults — no store, 1000-event buffer, 100ms flush
// timeout, slog default logger, no async error callback.
type PublisherOption func(*publisherConfig)

// publisherConfig is the resolved option set, populated via the
// [PublisherOption] functions and consumed by
// [NewJetStreamPublisher].
type publisherConfig struct {
	store        EventStore
	bufferSize   int
	flushTimeout time.Duration
	asyncOnError AsyncErrorCallback
	logger       *slog.Logger
}

// defaultPublisherConfig returns the zero-options baseline. Centralised
// so the test for "zero options is usable" can assert against these
// constants.
func defaultPublisherConfig() publisherConfig {
	return publisherConfig{
		store:        nil,
		bufferSize:   1000,
		flushTimeout: 100 * time.Millisecond,
		asyncOnError: nil,
		logger:       slog.Default(),
	}
}

// WithStore wires an [EventStore] into the publish path. When set,
// every Publish / PublishAsync persists before sending to NATS;
// store failures abort the publish (NATS untouched). Store is the
// source of truth per §4.9; subscribers may replay from the store
// on miss.
func WithStore(s EventStore) PublisherOption {
	return func(c *publisherConfig) { c.store = s }
}

// WithBufferSize sets the async queue depth. Zero or negative falls
// back to the default (1000). Sized for an operator-config knob —
// too small and high-volume sources stall; too large and Stop's
// drain budget balloons.
func WithBufferSize(n int) PublisherOption {
	return func(c *publisherConfig) {
		if n > 0 {
			c.bufferSize = n
		}
	}
}

// WithFlushTimeout sets the per-PublishAsync wait when the buffer is
// at capacity. Zero or negative falls back to the default (100ms).
// Mirrors the §4.9 "back-pressure on full" semantic at the client
// layer — callers see [ErrPublisherBufferFull] rather than silently
// dropping events.
func WithFlushTimeout(d time.Duration) PublisherOption {
	return func(c *publisherConfig) {
		if d > 0 {
			c.flushTimeout = d
		}
	}
}

// WithAsyncErrorCallback registers a per-event failure callback for
// async publishes. Caller is responsible for the callback being
// reentrant-safe; the worker goroutine invokes it inline before
// draining the next event. Nil resets to the default (slog.Warn
// fallback).
func WithAsyncErrorCallback(fn AsyncErrorCallback) PublisherOption {
	return func(c *publisherConfig) { c.asyncOnError = fn }
}

// WithLogger overrides the slog.Logger used by the publisher. Useful
// when the caller wants per-component log routing. Nil falls back to
// [slog.Default].
func WithLogger(l *slog.Logger) PublisherOption {
	return func(c *publisherConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

// NoopPublisher is the disabled-mode shim — every method returns nil
// without touching anything. Useful when a deployment runs the event
// system in non-bus mode (e.g., an air-gapped single-node CLI tool
// that doesn't need realtime pub/sub).
//
// NoopPublisher implements [EventPublisher]; the constructors below
// return it.
type NoopPublisher struct{}

// NewNoopPublisher returns a publisher that swallows every operation.
// Constructor exists so future construction-time setup (metrics
// registration, etc.) can land without churning call sites.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// Start is a no-op. Returns nil regardless of state.
func (NoopPublisher) Start(context.Context) error { return nil }

// Publish discards the event. Returns nil regardless of state.
func (NoopPublisher) Publish(context.Context, Event) error { return nil }

// PublishAsync discards the event. Returns nil regardless of state.
func (NoopPublisher) PublishAsync(context.Context, Event) error { return nil }

// Stop is a no-op. Returns nil regardless of state.
func (NoopPublisher) Stop(context.Context) error { return nil }

// Compile-time interface compliance check.
var _ EventPublisher = (*NoopPublisher)(nil)
