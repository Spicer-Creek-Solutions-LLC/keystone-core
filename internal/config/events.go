package config

import (
	"fmt"
	"time"
)

// EventsConfig drives the Epic 11 events boot in kscore-server.
//
//	events:
//	  enabled: true
//	  publisher:
//	    enabled: true
//	    buffer_size: 1000
//	    flush_timeout: 100ms
//	    store_first: true
//	  subscriber:
//	    enabled: true
//	    dedup_size: 1000
//
// Enabled defaults to true — events are foundational v1.0
// infrastructure and most deployments running NATS expect them on.
// Operators explicitly disable via `events.enabled: false`. When
// disabled, the gRPC EventService returns codes.Unavailable + REST
// returns 503; the JetStreamPublisher / Subscriber are skipped at
// boot. Validation requires NATS JetStream to be enabled when
// events is enabled — the publisher / subscriber both need a
// JetStream context.
type EventsConfig struct {
	Enabled    bool                   `koanf:"enabled"`
	Publisher  EventsPublisherConfig  `koanf:"publisher"`
	Subscriber EventsSubscriberConfig `koanf:"subscriber"`
}

// EventsPublisherConfig drives the JetStreamPublisher (Epic 11 task 3).
//
//   - Enabled: gates the publisher specifically. Defaults true when
//     parent Events is enabled. Setting false runs Events with the
//     NoopPublisher (subscriber + REST list/get still work).
//   - BufferSize / FlushTimeout: async-pipeline knobs from task 3.
//     Defaults match the package's `Default*` constants.
//   - StoreFirst: when true (default), publisher persists to the
//     EventStore before NATS publish; failures abort. When false,
//     publishes go straight to NATS (no audit trail). Most
//     deployments leave this on.
type EventsPublisherConfig struct {
	Enabled      bool          `koanf:"enabled"`
	BufferSize   int           `koanf:"buffer_size"`
	FlushTimeout time.Duration `koanf:"flush_timeout"`
	StoreFirst   bool          `koanf:"store_first"`
}

// EventsSubscriberConfig drives the JetStreamSubscriber (Epic 11 task 4).
//
//   - Enabled: gates the subscriber specifically. Defaults true.
//     Setting false makes SubscribeEvents return Unavailable.
//   - DedupSize: bounded ID dedup ring (task 4). Defaults 1000.
type EventsSubscriberConfig struct {
	Enabled   bool `koanf:"enabled"`
	DedupSize int  `koanf:"dedup_size"`
}

// applyEventsDefaults seeds the defaults that operators inherit
// when they don't set the field at all. Called from defaultConfig().
func applyEventsDefaults(c *EventsConfig) {
	c.Enabled = true
	c.Publisher.Enabled = true
	c.Publisher.BufferSize = 1000
	c.Publisher.FlushTimeout = 100 * time.Millisecond
	c.Publisher.StoreFirst = true
	c.Subscriber.Enabled = true
	c.Subscriber.DedupSize = 1000
}

// Validate enforces structural invariants and cross-field
// requirements. The caller passes the resolved NATS config so the
// "events enabled requires JetStream enabled" rule can fire.
//
// The `nats` argument is the sibling NATSConfig from the same
// top-level Config; validation lives here (rather than at the
// top level) so the events block stays self-contained for table-
// driven tests.
func (c *EventsConfig) Validate(nats NATSConfig) error {
	if !c.Enabled {
		return nil
	}
	if !nats.JetStream.Enabled {
		return fmt.Errorf("events: enabled requires nats.jetstream.enabled (set events.enabled: false to opt out)")
	}
	if c.Publisher.BufferSize < 0 {
		return fmt.Errorf("events.publisher.buffer_size must be non-negative, got %d", c.Publisher.BufferSize)
	}
	if c.Publisher.FlushTimeout < 0 {
		return fmt.Errorf("events.publisher.flush_timeout must be non-negative, got %v", c.Publisher.FlushTimeout)
	}
	if c.Subscriber.DedupSize < 0 {
		return fmt.Errorf("events.subscriber.dedup_size must be non-negative, got %d", c.Subscriber.DedupSize)
	}
	return nil
}
