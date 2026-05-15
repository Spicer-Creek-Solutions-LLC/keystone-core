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
	Retention  EventsRetentionConfig  `koanf:"retention"`
}

// EventsRetentionConfig drives the Epic 11 task 8 retention
// enforcer — the hourly scheduler that calls
// `events.EventStore.ApplyRetention` to bound store growth.
//
// Defaults to enabled with a built-in catch-all policy matching the
// §4.9 JetStream stream defaults (7 days / 1M events). Operators
// either keep the catch-all and add per-type overrides, or set
// `events.retention.enabled: false` to opt out entirely.
//
//	events:
//	  retention:
//	    enabled: true
//	    interval: 1h
//	    jitter: 0.1
//	    policies:
//	      - type: ""              # catch-all
//	        max_age: 168h         # 7 days
//	        max_count: 1000000
//	      - type: agent.heartbeat
//	        max_age: 24h
//	      - type: job.output
//	        max_age: 720h         # 30 days
type EventsRetentionConfig struct {
	Enabled  bool                    `koanf:"enabled"`
	Interval time.Duration           `koanf:"interval"`
	Jitter   float64                 `koanf:"jitter"`
	Policies []EventsRetentionPolicy `koanf:"policies"`
}

// EventsRetentionPolicy is one row in the retention table. Type
// empty is the catch-all rule (applies to every event type not
// matched by a more-specific policy). Each policy must have at
// least one limit (MaxAge > 0 OR MaxCount > 0) — zero-zero is
// rejected by validation as a no-op typo.
type EventsRetentionPolicy struct {
	Type     string        `koanf:"type"`
	MaxAge   time.Duration `koanf:"max_age"`
	MaxCount int           `koanf:"max_count"`
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
	c.Retention.Enabled = true
	c.Retention.Interval = time.Hour
	c.Retention.Jitter = 0.1
	// Default catch-all matches the §4.9 JetStream stream defaults.
	// Operators with custom policies typically keep this as the
	// trailing entry and add type-specific rules ahead of it; the
	// enforcer applies every policy in the list.
	c.Retention.Policies = []EventsRetentionPolicy{
		{Type: "", MaxAge: 7 * 24 * time.Hour, MaxCount: 1_000_000},
	}
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
	if err := c.Retention.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate enforces retention-config invariants. Disabled is always
// OK. When enabled: Interval > 0, Jitter ∈ [0, 0.5], and every
// policy must have at least one limit set (zero-zero policies are
// no-op typos and rejected loudly).
func (c *EventsRetentionConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Interval <= 0 {
		return fmt.Errorf("events.retention.interval must be > 0 when enabled, got %v", c.Interval)
	}
	if c.Jitter < 0 || c.Jitter > 0.5 {
		return fmt.Errorf("events.retention.jitter must be in [0, 0.5], got %f", c.Jitter)
	}
	for i, p := range c.Policies {
		if p.MaxAge < 0 {
			return fmt.Errorf("events.retention.policies[%d].max_age must be non-negative, got %v", i, p.MaxAge)
		}
		if p.MaxCount < 0 {
			return fmt.Errorf("events.retention.policies[%d].max_count must be non-negative, got %d", i, p.MaxCount)
		}
		if p.MaxAge == 0 && p.MaxCount == 0 {
			return fmt.Errorf("events.retention.policies[%d] (type=%q) has neither max_age nor max_count; remove the entry or set at least one limit", i, p.Type)
		}
	}
	return nil
}
