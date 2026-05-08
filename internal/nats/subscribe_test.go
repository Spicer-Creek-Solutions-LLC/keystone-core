package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

func TestManager_SubscribeRoundTrip(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	var (
		mu      sync.Mutex
		got     []envelope.Envelope
		gotSubj string
		done    = make(chan struct{}, 1)
	)
	sub, err := m.Subscribe(m.Subjects().AgentHeartbeat(), func(_ context.Context, subj string, env envelope.Envelope) error {
		mu.Lock()
		got = append(got, env)
		gotSubj = subj
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	env := envelope.New([]byte(`"hb"`), m.Subjects().Prefix())
	if err := m.PublishEnvelope(context.Background(), m.Subjects().AgentHeartbeat(), env); err != nil {
		t.Fatalf("PublishEnvelope: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive message within 2s")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if gotSubj != m.Subjects().AgentHeartbeat() {
		t.Errorf("subject = %q, want %q", gotSubj, m.Subjects().AgentHeartbeat())
	}
	if got[0].MessageID != env.MessageID {
		t.Errorf("MessageID drift: %q -> %q", env.MessageID, got[0].MessageID)
	}
}

func TestManager_SubscribeWildcardPattern(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	pattern := m.Subjects().BootstrapRegisterPattern()

	var (
		mu      sync.Mutex
		subjects []string
		done     = make(chan struct{}, 4)
	)
	sub, err := m.Subscribe(pattern, func(_ context.Context, subj string, _ envelope.Envelope) error {
		mu.Lock()
		subjects = append(subjects, subj)
		mu.Unlock()
		done <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Two distinct agent IDs hit the same wildcard pattern.
	for _, id := range []string{"agent-1", "agent-2"} {
		env := envelope.New([]byte(`"x"`), m.Subjects().Prefix())
		if err := m.PublishEnvelope(context.Background(), m.Subjects().BootstrapRegister(id), env); err != nil {
			t.Fatalf("PublishEnvelope(%s): %v", id, err)
		}
	}

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber did not receive message %d within 2s", i+1)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(subjects) != 2 {
		t.Fatalf("got %d subjects, want 2", len(subjects))
	}
}

func TestManager_SubscribeRejectsBadInputs(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	if _, err := m.Subscribe("", func(context.Context, string, envelope.Envelope) error { return nil }); err == nil {
		t.Error("Subscribe(empty subject) = nil, want error")
	}
	if _, err := m.Subscribe(m.Subjects().AgentHeartbeat(), nil); err == nil {
		t.Error("Subscribe(nil handler) = nil, want error")
	}
}

func TestManager_SubscribePreStart(t *testing.T) {
	m, err := New(embeddedConfig(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := m.Subscribe("kscore.test.x", func(context.Context, string, envelope.Envelope) error { return nil }); err == nil {
		t.Error("Subscribe pre-Start = nil, want error")
	}
}

func TestManager_SubscribeAfterShutdown(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, err := m.Subscribe("kscore.test.x", func(context.Context, string, envelope.Envelope) error { return nil }); err == nil {
		t.Error("Subscribe after Shutdown = nil, want error")
	}
}

func TestManager_SubscribeUnsubscribeIdempotent(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	sub, err := m.Subscribe(m.Subjects().AgentHeartbeat(), func(context.Context, string, envelope.Envelope) error { return nil })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("first Unsubscribe: %v", err)
	}
	// Second call surfaces nats.go's "already unsubscribed" error;
	// our wrapper passes it through. We just want it to not panic.
	_ = sub.Unsubscribe()
}

func TestManager_SubscribeBadEnvelopeSwallowed(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	called := make(chan struct{}, 1)
	_, err := m.Subscribe(m.Subjects().AgentHeartbeat(), func(_ context.Context, _ string, _ envelope.Envelope) error {
		called <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Publish raw bytes that aren't a valid envelope — bypasses the
	// PublishEnvelope path. Use the underlying conn directly.
	conn := m.activeConnLocked()
	if err := conn.Publish(m.Subjects().AgentHeartbeat(), []byte("not-json")); err != nil {
		t.Fatalf("raw Publish: %v", err)
	}
	if err := conn.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	select {
	case <-called:
		t.Error("handler was invoked for a malformed envelope")
	case <-time.After(150 * time.Millisecond):
		// expected — decode failure is logged, handler skipped
	}
}
