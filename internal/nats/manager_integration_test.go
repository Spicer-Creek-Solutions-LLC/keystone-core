//go:build integration

package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"
)

// TestManager_EmbeddedFullRoundTrip is the integration smoke for Task 1:
// embedded boot, JetStream-enabled, an external subscriber on the same
// server, and a publish that the subscriber receives. Task 13 grows
// this into the JetStream consume path.
func TestManager_EmbeddedFullRoundTrip(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	sub, err := natsclient.Connect(m.ClientURL())
	if err != nil {
		t.Fatalf("subscriber connect: %v", err)
	}
	defer sub.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	var got []byte
	subscription, err := sub.Subscribe("kscore.test.integration", func(msg *natsclient.Msg) {
		got = append([]byte(nil), msg.Data...)
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := sub.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if err := m.Publish(context.Background(), "kscore.test.integration", []byte("ping")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive message within 2s")
	}
	if string(got) != "ping" {
		t.Errorf("payload = %q, want ping", got)
	}
}
