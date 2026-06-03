// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Boot-wiring test for the outbound webhook subsystem: it asserts that
// startOutboundWebhook activates the §4.14 circuit breaker (wraps the
// HTTP dispatcher) rather than dispatching bare. The breaker's own
// behavior is covered by internal/webhook/outbound/circuit_breaker_test.go;
// this guards the boot construction.
package main

import (
	"context"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

func TestStartOutboundWebhook_WrapsCircuitBreaker(t *testing.T) {
	// startOutboundWebhook writes its SQLite store to a hardcoded
	// ./data/ dir; chdir into a temp dir so the test leaves nothing
	// behind and can't collide with a real boot.
	t.Chdir(t.TempDir())

	ctx := context.Background()
	cfg := config.WebhookOutboundConfig{
		Enabled:                 true,
		MaxConcurrentDeliveries: 4,
		RetryBackoff:            time.Second,
		Timeout:                 2 * time.Second,
	}
	rt, err := startOutboundWebhook(ctx, cfg, "test", events.NoopSubscriber{}, silentLogger())
	if err != nil {
		t.Fatalf("startOutboundWebhook: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rt.stop(stopCtx, silentLogger())
	})

	if rt == nil || rt.Manager == nil {
		t.Fatal("startOutboundWebhook returned nil runtime/manager")
	}
	if _, ok := rt.Manager.Dispatcher.(*outbound.CircuitBreaker); !ok {
		t.Fatalf("Manager.Dispatcher = %T, want *outbound.CircuitBreaker (breaker not wired at boot)",
			rt.Manager.Dispatcher)
	}
}
