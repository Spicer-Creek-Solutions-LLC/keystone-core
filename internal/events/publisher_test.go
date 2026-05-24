// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"testing"
	"time"
)

func TestNoopPublisher_AllMethodsNil(t *testing.T) {
	t.Parallel()
	p := NewNoopPublisher()
	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := p.Publish(ctx, Event{}); err != nil {
		t.Errorf("Publish: %v", err)
	}
	if err := p.PublishAsync(ctx, Event{}); err != nil {
		t.Errorf("PublishAsync: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	// Idempotent: second Start/Stop fine.
	if err := p.Start(ctx); err != nil {
		t.Errorf("second Start: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestDefaultPublisherConfig(t *testing.T) {
	t.Parallel()
	cfg := defaultPublisherConfig()
	if cfg.bufferSize != 1000 {
		t.Errorf("bufferSize = %d, want 1000", cfg.bufferSize)
	}
	if cfg.flushTimeout != 100*time.Millisecond {
		t.Errorf("flushTimeout = %v, want 100ms", cfg.flushTimeout)
	}
	if cfg.store != nil {
		t.Errorf("store = %v, want nil", cfg.store)
	}
	if cfg.asyncOnError != nil {
		t.Errorf("asyncOnError = %v, want nil", cfg.asyncOnError)
	}
	if cfg.logger == nil {
		t.Errorf("logger = nil, want slog.Default()")
	}
}

func TestPublisherOptions_HonourInputs(t *testing.T) {
	t.Parallel()
	cb := func(_ Event, _ error) {}
	cfg := defaultPublisherConfig()
	for _, opt := range []PublisherOption{
		WithBufferSize(50),
		WithFlushTimeout(250 * time.Millisecond),
		WithAsyncErrorCallback(cb),
	} {
		opt(&cfg)
	}
	if cfg.bufferSize != 50 {
		t.Errorf("bufferSize = %d", cfg.bufferSize)
	}
	if cfg.flushTimeout != 250*time.Millisecond {
		t.Errorf("flushTimeout = %v", cfg.flushTimeout)
	}
	if cfg.asyncOnError == nil {
		t.Errorf("asyncOnError = nil, want callback")
	}
}

func TestPublisherOptions_RejectsInvalidValues(t *testing.T) {
	t.Parallel()
	// Zero / negative BufferSize and FlushTimeout fall back to default.
	cfg := defaultPublisherConfig()
	WithBufferSize(0)(&cfg)
	WithBufferSize(-1)(&cfg)
	if cfg.bufferSize != 1000 {
		t.Errorf("BufferSize(0|-1): %d, want 1000", cfg.bufferSize)
	}
	WithFlushTimeout(0)(&cfg)
	WithFlushTimeout(-time.Second)(&cfg)
	if cfg.flushTimeout != 100*time.Millisecond {
		t.Errorf("FlushTimeout(0|-1s): %v, want 100ms", cfg.flushTimeout)
	}
	// Nil logger falls back to default.
	WithLogger(nil)(&cfg)
	if cfg.logger == nil {
		t.Errorf("logger = nil after WithLogger(nil)")
	}
}
