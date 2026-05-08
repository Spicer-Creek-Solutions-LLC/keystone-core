package nats

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// MessageHandler receives one decoded envelope. The subject is passed
// alongside because the wildcard subscription pattern (e.g.,
// kscore.{cluster}.bootstrap.*.register) needs the concrete subject
// to extract per-message routing — agent ID, command ID, etc.
//
// Errors returned by the handler are logged but not surfaced to the
// publisher: NATS pub/sub is fire-and-forget at this layer. Handlers
// that need ack semantics should publish an explicit response on a
// reply subject (the bootstrap handler in Task 9 does exactly this).
type MessageHandler func(ctx context.Context, subject string, env envelope.Envelope) error

// Subscription is the lifecycle handle returned by Manager.Subscribe.
// Callers must call Unsubscribe when done; Manager.Shutdown does not
// drain caller-owned subscriptions automatically.
type Subscription interface {
	// Unsubscribe stops delivering messages to the handler. Idempotent.
	Unsubscribe() error
}

// natsSubscription wraps a *nats.Subscription so the public surface
// is just the Unsubscribe method. Concrete type kept unexported so
// callers cannot reach into nats.go internals.
type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error {
	if s.sub == nil {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return fmt.Errorf("nats: unsubscribe: %w", err)
	}
	return nil
}

// Subscribe attaches handler to subject. The handler runs on the
// nats.go delivery goroutine; long-running work should hand off to
// a worker pool. Subject must be a subscriber-pattern subject
// (wildcards allowed) — Validate is not consulted here because it
// rejects wildcards by design (those are publish-only constraints).
//
// Manager must be Started; pre-Start subscribes are rejected so the
// caller doesn't accumulate handlers against a nil conn.
func (m *Manager) Subscribe(subject string, handler MessageHandler) (Subscription, error) {
	if handler == nil {
		return nil, errors.New("nats: Subscribe: handler must not be nil")
	}
	if subject == "" {
		return nil, errors.New("nats: Subscribe: subject must not be empty")
	}

	m.mu.Lock()
	stopped := m.stopped
	started := m.started
	conn := m.activeConnLocked()
	m.mu.Unlock()

	if stopped {
		return nil, errors.New("nats: shut down")
	}
	if !started || conn == nil {
		return nil, errors.New("nats: not started")
	}

	log := m.log
	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		env, err := envelope.Unmarshal(msg.Data)
		if err != nil {
			log.Warn("nats: subscriber decode failed",
				"subject", msg.Subject, "err", err)
			return
		}
		if err := handler(context.Background(), msg.Subject, env); err != nil {
			log.Warn("nats: subscriber handler failed",
				"subject", msg.Subject, "err", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("nats: subscribe %q: %w", subject, err)
	}
	return &natsSubscription{sub: sub}, nil
}
