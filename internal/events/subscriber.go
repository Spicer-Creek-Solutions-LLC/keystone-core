// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"log/slog"
	"time"
)

// Defaults per PROJECT-DETAILS §4.9 — operators override via the
// respective [SubscribeOption] / [SubscriberOption] functions.
const (
	// DefaultMaxRedeliveries is the §4.9 "3 max redeliveries" policy
	// expressed as the option default. JetStream's `MaxDeliver`
	// counts total deliveries (initial + redeliveries) so the
	// configured value here is the count of redelivery attempts;
	// the JetStream consumer is created with `MaxDeliver =
	// MaxRedeliveries + 1` (initial delivery + N retries).
	DefaultMaxRedeliveries = 3

	// DefaultAckTimeout is the §4.9 "30s ack timeout" — a handler
	// has this long to call Ack before JetStream considers the
	// delivery failed and queues a redelivery.
	DefaultAckTimeout = 30 * time.Second

	// DefaultDedupSize bounds the in-memory recently-seen-IDs ring
	// used by the replay path to skip events that arrive on both
	// the store and live JetStream paths. Sized for ~100s of events
	// at ~10/sec; the operator can override via [WithDedupSize] for
	// higher-volume deployments. Overflow degrades to occasional
	// duplicate dispatch — never a missed event.
	DefaultDedupSize = 1000
)

// EventHandler is the per-event callback invoked by the subscriber's
// dispatcher goroutine. Returning nil acks the message; returning an
// error triggers a JetStream Nak with backoff, causing redelivery up
// to [SubscribeOption]'s configured max.
//
// The context passed to the handler is derived from the subscription's
// lifetime — it cancels when [Subscription.Unsubscribe] is called or
// when the subscriber is Stop'd. Handlers MUST honor the context's
// Done() channel for clean shutdown.
type EventHandler func(ctx context.Context, e Event) error

// Subscription is the handle returned by [EventSubscriber.Subscribe].
// Callers Unsubscribe to stop receiving events; the JetStream consumer
// is removed (ephemeral) or left in place (durable) per the
// subscription's options.
type Subscription interface {
	// Unsubscribe stops delivery and removes the subscription from
	// the parent [EventSubscriber]'s tracking. Safe to call multiple
	// times — second + later calls are no-ops.
	Unsubscribe() error

	// Pending returns the number of messages buffered locally by
	// nats.go awaiting handler dispatch. Useful for slow-consumer
	// diagnostics; the gRPC handler (task 6) exposes it via stream
	// metadata.
	Pending() (uint64, error)
}

// EventSubscriber is the consumer half of the v1.0 event bus.
// [JetStreamSubscriber] is the canonical implementation;
// [NoopSubscriber] is the disabled-mode shim. Lifecycle mirrors
// [EventPublisher]: Start once, Subscribe any number of times, then
// Stop once.
type EventSubscriber interface {
	// Start initialises the subscriber. Idempotent across Stop /
	// Start cycles but not within them.
	Start(ctx context.Context) error

	// Subscribe attaches a handler to subjects matching pattern.
	// Returns a [Subscription] handle the caller uses for explicit
	// teardown. Options shape the subscription's behavior — queue
	// group membership, client-side filter predicate, historical
	// replay window, durable consumer name, redelivery limits.
	Subscribe(ctx context.Context, pattern string, h EventHandler, opts ...SubscribeOption) (Subscription, error)

	// Stop unsubscribes every tracked subscription, waits for
	// in-flight handlers to complete (or for ctx to expire), and
	// releases resources. Idempotent.
	Stop(ctx context.Context) error
}

// SubscribeOption configures a single subscription. Functional-options
// pattern matches the rest of the package; zero options yields a
// live-only broadcast subscription with [DefaultMaxRedeliveries] +
// [DefaultAckTimeout].
type SubscribeOption func(*subscribeConfig)

type subscribeConfig struct {
	queueGroup      string
	filter          func(Event) bool
	replay          time.Duration
	durableName     string
	maxRedeliveries int
	ackTimeout      time.Duration
}

func defaultSubscribeConfig() subscribeConfig {
	return subscribeConfig{
		maxRedeliveries: DefaultMaxRedeliveries,
		ackTimeout:      DefaultAckTimeout,
	}
}

// WithQueueGroup makes the subscription a member of the named queue
// group. JetStream load-balances messages across all members of the
// group — exactly one member's handler is invoked per message. The
// durable consumer name defaults to `events_qg_<group>` so all
// members share state.
func WithQueueGroup(name string) SubscribeOption {
	return func(c *subscribeConfig) { c.queueGroup = name }
}

// WithFilter installs a client-side predicate. Returning false drops
// the event (and Acks it — the filter is "not interested," not
// "couldn't process"). Predicates compose with subject patterns:
// pattern narrows what JetStream delivers, predicate narrows what
// the handler sees. Task 5 adds CEL-compilation that produces a
// compatible `func(Event) bool` for operator-supplied expressions.
func WithFilter(predicate func(Event) bool) SubscribeOption {
	return func(c *subscribeConfig) { c.filter = predicate }
}

// WithReplay queues historical events into the subscription before
// the live stream attaches. Replay is a half-open window
// `[now - d, now)`. Events older than JetStream's stream retention
// come from the configured [EventStore] (see [WithStore]); events
// within retention come from JetStream directly. Duplicates between
// the two layers are skipped via the bounded ID dedup set (see
// [WithDedupSize]).
//
// WithReplay > 0 requires the subscriber to have been constructed
// with [WithStore]; otherwise the subscription fails at Subscribe
// time. The store is the source of truth — JetStream-only replay is
// limited to the retention window and would silently swallow older
// events.
func WithReplay(d time.Duration) SubscribeOption {
	return func(c *subscribeConfig) { c.replay = d }
}

// WithDurableName overrides the JetStream durable consumer name. By
// default the subscriber generates an ephemeral consumer for
// broadcast subscriptions and `events_qg_<group>` for queue-group
// subscriptions. Operators set this explicitly when they want a
// named durable that survives subscriber restarts.
func WithDurableName(name string) SubscribeOption {
	return func(c *subscribeConfig) { c.durableName = name }
}

// WithMaxRedeliveries sets the per-subscription retry limit. Zero or
// negative falls back to [DefaultMaxRedeliveries]. Value is the count
// of redeliveries — the JetStream consumer is created with
// `MaxDeliver = n + 1` (initial + n retries).
func WithMaxRedeliveries(n int) SubscribeOption {
	return func(c *subscribeConfig) {
		if n > 0 {
			c.maxRedeliveries = n
		}
	}
}

// WithAckTimeout sets the per-message handler deadline. JetStream
// considers a delivery failed when the handler does not Ack within
// this window. Zero or negative falls back to [DefaultAckTimeout].
func WithAckTimeout(d time.Duration) SubscribeOption {
	return func(c *subscribeConfig) {
		if d > 0 {
			c.ackTimeout = d
		}
	}
}

// SubscriberOption configures the [JetStreamSubscriber] itself — set
// at construction time, applies to every Subscribe call.
type SubscriberOption func(*subscriberConfig)

type subscriberConfig struct {
	store     EventStore
	logger    *slog.Logger
	dedupSize int
}

func defaultSubscriberConfig() subscriberConfig {
	return subscriberConfig{
		store:     nil,
		logger:    slog.Default(),
		dedupSize: DefaultDedupSize,
	}
}

// WithSubscriberStore wires an [EventStore] into the subscriber.
// Required for [WithReplay] subscriptions — replay queries the store
// for the pre-JetStream-retention slice of the window. Optional for
// live-only subscribers (the store argument is just ignored in that
// case).
//
// Named with the `Subscriber` prefix to distinguish from the
// publisher's [WithStore] — both wire an EventStore but to different
// types; the publisher's predates this method.
func WithSubscriberStore(s EventStore) SubscriberOption {
	return func(c *subscriberConfig) { c.store = s }
}

// WithSubscriberLogger overrides the slog.Logger. Nil falls back to
// [slog.Default].
func WithSubscriberLogger(l *slog.Logger) SubscriberOption {
	return func(c *subscriberConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithDedupSize bounds the recently-seen-ID set used during replay
// to avoid double-dispatch of events present in both the store and
// JetStream. Zero or negative falls back to [DefaultDedupSize].
// Higher values trade memory for tighter dedup coverage across
// long replay windows.
func WithDedupSize(n int) SubscriberOption {
	return func(c *subscriberConfig) {
		if n > 0 {
			c.dedupSize = n
		}
	}
}

// NoopSubscriber is the disabled-mode shim. Every method returns nil;
// Subscribe returns a no-op [Subscription]. Useful for non-bus
// deployments (air-gapped single-node CLI tools) so call-sites don't
// need conditional logic to skip subscribe calls.
type NoopSubscriber struct{}

// NewNoopSubscriber returns a subscriber that swallows every operation.
func NewNoopSubscriber() *NoopSubscriber { return &NoopSubscriber{} }

// Start is a no-op.
func (NoopSubscriber) Start(context.Context) error { return nil }

// Subscribe returns a no-op subscription.
func (NoopSubscriber) Subscribe(_ context.Context, _ string, _ EventHandler, _ ...SubscribeOption) (Subscription, error) {
	return noopSubscription{}, nil
}

// Stop is a no-op.
func (NoopSubscriber) Stop(context.Context) error { return nil }

type noopSubscription struct{}

func (noopSubscription) Unsubscribe() error       { return nil }
func (noopSubscription) Pending() (uint64, error) { return 0, nil }

// Compile-time interface compliance.
var (
	_ EventSubscriber = (*NoopSubscriber)(nil)
	_ Subscription    = noopSubscription{}
)
