// SPDX-License-Identifier: Apache-2.0

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

func TestEventsConfig_Retention_Defaults(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	if !c.Events.Retention.Enabled {
		t.Errorf("Retention.Enabled = false, want true (default-on per task 8)")
	}
	if c.Events.Retention.Interval != time.Hour {
		t.Errorf("Interval = %v, want 1h", c.Events.Retention.Interval)
	}
	if c.Events.Retention.Jitter != 0.1 {
		t.Errorf("Jitter = %f, want 0.1", c.Events.Retention.Jitter)
	}
	if len(c.Events.Retention.Policies) != 1 {
		t.Fatalf("default policies = %d, want 1 catch-all", len(c.Events.Retention.Policies))
	}
	p := c.Events.Retention.Policies[0]
	if p.Type != "" || p.MaxAge != 7*24*time.Hour || p.MaxCount != 1_000_000 {
		t.Errorf("default catch-all = %+v, want {Type:\"\", MaxAge:168h, MaxCount:1000000}", p)
	}
}

func TestEventsRetention_Validate_DisabledIsAlwaysOK(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{Enabled: false, Interval: 0, Jitter: -1}
	if err := c.Validate(); err != nil {
		t.Errorf("disabled validates: got %v", err)
	}
}

func TestEventsRetention_Validate_RequiresInterval(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{Enabled: true, Jitter: 0.1}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "interval") {
		t.Errorf("err = %v, want interval rejection", err)
	}
}

func TestEventsRetention_Validate_JitterOutOfRange(t *testing.T) {
	t.Parallel()
	cases := []float64{-0.1, 0.51, 1.0}
	for _, j := range cases {
		c := EventsRetentionConfig{Enabled: true, Interval: time.Hour, Jitter: j}
		if err := c.Validate(); err == nil {
			t.Errorf("jitter=%f validated; want error", j)
		}
	}
}

func TestEventsRetention_Validate_RejectsZeroZeroPolicy(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{
		Enabled:  true,
		Interval: time.Hour,
		Jitter:   0.1,
		Policies: []EventsRetentionPolicy{{Type: "agent.connect"}}, // MaxAge=0 AND MaxCount=0
	}
	err := c.Validate()
	if err == nil {
		t.Fatalf("zero-zero policy validated; want error")
	}
	if !strings.Contains(err.Error(), "max_age") {
		t.Errorf("err = %v; want max_age mention", err)
	}
}

func TestEventsRetention_Validate_NegativeMaxAgeRejected(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{
		Enabled:  true,
		Interval: time.Hour,
		Policies: []EventsRetentionPolicy{{Type: "x", MaxAge: -time.Hour}},
	}
	if err := c.Validate(); err == nil {
		t.Errorf("negative max_age validated")
	}
}

func TestEventsRetention_Validate_NegativeMaxCountRejected(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{
		Enabled:  true,
		Interval: time.Hour,
		Policies: []EventsRetentionPolicy{{Type: "x", MaxCount: -1}},
	}
	if err := c.Validate(); err == nil {
		t.Errorf("negative max_count validated")
	}
}

func TestEventsRetention_Validate_HappyPath(t *testing.T) {
	t.Parallel()
	c := EventsRetentionConfig{
		Enabled:  true,
		Interval: time.Hour,
		Jitter:   0.1,
		Policies: []EventsRetentionPolicy{
			{Type: "", MaxAge: 7 * 24 * time.Hour, MaxCount: 1_000_000},
			{Type: "agent.heartbeat", MaxAge: 24 * time.Hour},
			{Type: "job.output", MaxCount: 500},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("happy retention failed: %v", err)
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
