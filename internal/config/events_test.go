package config

import (
	"strings"
	"testing"
	"time"
)

func TestEventsConfig_Defaults(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	if !c.Events.Enabled {
		t.Errorf("Enabled = false, want true (default-on per task 6 plan)")
	}
	if !c.Events.Publisher.Enabled {
		t.Errorf("Publisher.Enabled = false, want true")
	}
	if c.Events.Publisher.BufferSize != 1000 {
		t.Errorf("BufferSize = %d, want 1000", c.Events.Publisher.BufferSize)
	}
	if c.Events.Publisher.FlushTimeout != 100*time.Millisecond {
		t.Errorf("FlushTimeout = %v, want 100ms", c.Events.Publisher.FlushTimeout)
	}
	if !c.Events.Publisher.StoreFirst {
		t.Errorf("StoreFirst = false, want true")
	}
	if !c.Events.Subscriber.Enabled {
		t.Errorf("Subscriber.Enabled = false, want true")
	}
	if c.Events.Subscriber.DedupSize != 1000 {
		t.Errorf("DedupSize = %d, want 1000", c.Events.Subscriber.DedupSize)
	}
}

func TestEventsConfig_Validate_DisabledIsAlwaysOK(t *testing.T) {
	t.Parallel()
	c := EventsConfig{Enabled: false}
	// Even with JetStream off, disabled events validates fine.
	if err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: false}}); err != nil {
		t.Errorf("disabled+no-js validates: got %v", err)
	}
}

func TestEventsConfig_Validate_RequiresJetStream(t *testing.T) {
	t.Parallel()
	c := EventsConfig{Enabled: true}
	err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: false}})
	if err == nil {
		t.Fatalf("enabled+no-js validated; want error")
	}
	if !strings.Contains(err.Error(), "nats.jetstream.enabled") {
		t.Errorf("err = %v; want jetstream-mention", err)
	}
}

func TestEventsConfig_Validate_NegativeBufferRejected(t *testing.T) {
	t.Parallel()
	c := EventsConfig{
		Enabled:   true,
		Publisher: EventsPublisherConfig{BufferSize: -1},
	}
	err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: true}})
	if err == nil || !strings.Contains(err.Error(), "buffer_size") {
		t.Errorf("err = %v; want buffer_size rejection", err)
	}
}

func TestEventsConfig_Validate_NegativeFlushTimeoutRejected(t *testing.T) {
	t.Parallel()
	c := EventsConfig{
		Enabled:   true,
		Publisher: EventsPublisherConfig{FlushTimeout: -time.Second},
	}
	err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: true}})
	if err == nil || !strings.Contains(err.Error(), "flush_timeout") {
		t.Errorf("err = %v; want flush_timeout rejection", err)
	}
}

func TestEventsConfig_Validate_NegativeDedupSizeRejected(t *testing.T) {
	t.Parallel()
	c := EventsConfig{
		Enabled:    true,
		Subscriber: EventsSubscriberConfig{DedupSize: -1},
	}
	err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: true}})
	if err == nil || !strings.Contains(err.Error(), "dedup_size") {
		t.Errorf("err = %v; want dedup_size rejection", err)
	}
}

func TestEventsConfig_Validate_ValidHappy(t *testing.T) {
	t.Parallel()
	c := EventsConfig{
		Enabled: true,
		Publisher: EventsPublisherConfig{
			Enabled:      true,
			BufferSize:   500,
			FlushTimeout: 200 * time.Millisecond,
			StoreFirst:   true,
		},
		Subscriber: EventsSubscriberConfig{
			Enabled:   true,
			DedupSize: 500,
		},
	}
	if err := c.Validate(NATSConfig{JetStream: JetStreamConfig{Enabled: true}}); err != nil {
		t.Errorf("validate: %v", err)
	}
}
