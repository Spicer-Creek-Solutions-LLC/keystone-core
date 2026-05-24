// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"testing"
	"time"
)

func TestNoopSubscriber_AllMethodsNil(t *testing.T) {
	t.Parallel()
	s := NewNoopSubscriber()
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Errorf("Start: %v", err)
	}
	sub, err := s.Subscribe(ctx, "kscore.test.events.>", func(_ context.Context, _ Event) error {
		t.Errorf("handler called on noop")
		return nil
	})
	if err != nil {
		t.Errorf("Subscribe: %v", err)
	}
	if sub == nil {
		t.Errorf("Subscribe returned nil Subscription")
	}
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("Unsubscribe: %v", err)
	}
	if n, err := sub.Pending(); err != nil || n != 0 {
		t.Errorf("Pending: n=%d err=%v", n, err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	// Idempotent.
	if err := s.Start(ctx); err != nil {
		t.Errorf("second Start: %v", err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestDefaultSubscribeConfig(t *testing.T) {
	t.Parallel()
	cfg := defaultSubscribeConfig()
	if cfg.maxRedeliveries != DefaultMaxRedeliveries {
		t.Errorf("maxRedeliveries = %d, want %d", cfg.maxRedeliveries, DefaultMaxRedeliveries)
	}
	if cfg.ackTimeout != DefaultAckTimeout {
		t.Errorf("ackTimeout = %v, want %v", cfg.ackTimeout, DefaultAckTimeout)
	}
	if cfg.replay != 0 {
		t.Errorf("replay = %v, want 0", cfg.replay)
	}
	if cfg.filter != nil {
		t.Errorf("filter non-nil")
	}
}

func TestDefaultSubscriberConfig(t *testing.T) {
	t.Parallel()
	cfg := defaultSubscriberConfig()
	if cfg.dedupSize != DefaultDedupSize {
		t.Errorf("dedupSize = %d, want %d", cfg.dedupSize, DefaultDedupSize)
	}
	if cfg.store != nil {
		t.Errorf("store non-nil")
	}
	if cfg.logger == nil {
		t.Errorf("logger nil")
	}
}

func TestSubscribeOptions_HonourInputs(t *testing.T) {
	t.Parallel()
	predicate := func(_ Event) bool { return true }
	cfg := defaultSubscribeConfig()
	for _, opt := range []SubscribeOption{
		WithQueueGroup("workers"),
		WithFilter(predicate),
		WithReplay(60 * time.Second),
		WithDurableName("my-durable"),
		WithMaxRedeliveries(7),
		WithAckTimeout(45 * time.Second),
	} {
		opt(&cfg)
	}
	if cfg.queueGroup != "workers" {
		t.Errorf("queueGroup = %q", cfg.queueGroup)
	}
	if cfg.filter == nil {
		t.Errorf("filter nil")
	}
	if cfg.replay != 60*time.Second {
		t.Errorf("replay = %v", cfg.replay)
	}
	if cfg.durableName != "my-durable" {
		t.Errorf("durableName = %q", cfg.durableName)
	}
	if cfg.maxRedeliveries != 7 {
		t.Errorf("maxRedeliveries = %d", cfg.maxRedeliveries)
	}
	if cfg.ackTimeout != 45*time.Second {
		t.Errorf("ackTimeout = %v", cfg.ackTimeout)
	}
}

func TestSubscribeOptions_InvalidFallback(t *testing.T) {
	t.Parallel()
	cfg := defaultSubscribeConfig()
	WithMaxRedeliveries(0)(&cfg)
	WithMaxRedeliveries(-2)(&cfg)
	if cfg.maxRedeliveries != DefaultMaxRedeliveries {
		t.Errorf("maxRedeliveries(0|-2) = %d, want default", cfg.maxRedeliveries)
	}
	WithAckTimeout(0)(&cfg)
	WithAckTimeout(-time.Second)(&cfg)
	if cfg.ackTimeout != DefaultAckTimeout {
		t.Errorf("ackTimeout(0|-1s) = %v, want default", cfg.ackTimeout)
	}
}

func TestSubscriberOptions_InvalidFallback(t *testing.T) {
	t.Parallel()
	cfg := defaultSubscriberConfig()
	WithDedupSize(0)(&cfg)
	WithDedupSize(-1)(&cfg)
	if cfg.dedupSize != DefaultDedupSize {
		t.Errorf("dedupSize(0|-1) = %d, want default", cfg.dedupSize)
	}
	WithSubscriberLogger(nil)(&cfg)
	if cfg.logger == nil {
		t.Errorf("logger nil after WithSubscriberLogger(nil)")
	}
}

func TestIDDedup_Basic(t *testing.T) {
	t.Parallel()
	d := newIDDedup(3)
	if d.SeenAndAdd("a") {
		t.Errorf("first SeenAndAdd(a) reported seen")
	}
	if !d.SeenAndAdd("a") {
		t.Errorf("second SeenAndAdd(a) reported not seen")
	}
	if d.SeenAndAdd("b") {
		t.Errorf("first SeenAndAdd(b) reported seen")
	}
	if d.SeenAndAdd("c") {
		t.Errorf("first SeenAndAdd(c) reported seen")
	}
	// d at capacity; adding "d" evicts "a" (FIFO).
	if d.SeenAndAdd("d") {
		t.Errorf("first SeenAndAdd(d) reported seen")
	}
	if d.SeenAndAdd("a") {
		t.Errorf("after eviction SeenAndAdd(a) reported seen; expected fresh")
	}
}

func TestIDDedup_ZeroCapacityFallback(t *testing.T) {
	t.Parallel()
	d := newIDDedup(0)
	// Constructor falls back to DefaultDedupSize.
	if !d.SeenAndAdd("x") == false { // x not seen → SeenAndAdd returns false
		t.Errorf("unexpected behavior")
	}
}
