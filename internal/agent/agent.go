package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// MessageHandler is the function shape Agent passes to Subscribe.
// internal/nats.MessageHandler has the same signature; the cmd/
// kscore-agent wiring layer adapts the named-type difference (Go's
// structural typing on named function types doesn't unify across
// packages even with identical signatures).
type MessageHandler func(ctx context.Context, subject string, env envelope.Envelope) error

// Subscription is the lifecycle handle returned by Subscribe.
// internal/nats.Subscription satisfies it via its Unsubscribe method.
type Subscription interface {
	Unsubscribe() error
}

// NATSClient is the narrow surface Agent needs from the NATS layer.
// internal/nats.Manager satisfies it via the kscore-agent adapter.
// Tests pass a fake.
type NATSClient interface {
	PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error
	Subscribe(subject string, handler MessageHandler) (Subscription, error)
	Health(ctx context.Context) error
}

// Subjects is the narrow subject-construction surface Agent needs.
// internal/nats.SubjectBuilder satisfies it structurally.
type Subjects interface {
	AgentHeartbeat() string
	AgentCommand(agentID string) string
	AgentResponse(agentID string) string
	AgentState(agentID string) string
	Cluster() string
	Prefix() string
}

// Config configures an Agent. AgentID is required; the interval
// defaults align with PROJECT-DETAILS §4.6 (heartbeat 30s, metadata
// 60s).
type Config struct {
	AgentID           string
	HeartbeatInterval time.Duration
	MetadataInterval  time.Duration
	CommandTimeout    time.Duration
	Labels            map[string]string
}

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultMetadataInterval  = 60 * time.Second
	defaultCommandTimeout    = 5 * time.Minute
)

// Agent is the v1.0 kscore-agent runtime. PROJECT-DETAILS §4.6:
// Concurrency: heartbeat loop, metadata loop, command-handler-per-
// request — all goroutines under a WaitGroup. State protected by
// sync.RWMutex.
//
// Tasks 2–11 fill in the bodies of the loops + command handler:
//   - Task 2 wires Executor for command exec.
//   - Task 3 wires MetadataCollector (gopsutil) for the heartbeat /
//     metadata payloads.
//   - Task 4 wires SecurityEnforcer (HMAC, allowlists).
//   - Task 5 wires full command-response (validate → exec → publish).
//
// Task 1 (this code) ships the lifecycle skeleton with stub payloads.
type Agent struct {
	cfg      Config
	log      *slog.Logger
	nats     NATSClient
	subjects Subjects
	now      func() time.Time

	mu      sync.Mutex
	started bool
	stopped bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	sub     Subscription
}

// New validates cfg and returns an unstarted Agent. AgentID is
// required; the bootstrap engine (Task 6) persists it. Other fields
// fall back to §4.6 defaults when zero.
func New(cfg Config, nats NATSClient, subjects Subjects, log *slog.Logger) (*Agent, error) {
	if cfg.AgentID == "" {
		return nil, errors.New("agent: AgentID is required")
	}
	if nats == nil {
		return nil, errors.New("agent: NATSClient is required")
	}
	if subjects == nil {
		return nil, errors.New("agent: Subjects is required")
	}
	if log == nil {
		log = slog.Default()
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.MetadataInterval == 0 {
		cfg.MetadataInterval = defaultMetadataInterval
	}
	if cfg.CommandTimeout == 0 {
		cfg.CommandTimeout = defaultCommandTimeout
	}
	return &Agent{
		cfg:      cfg,
		log:      log,
		nats:     nats,
		subjects: subjects,
		now:      time.Now,
	}, nil
}

// Start subscribes to the command topic and spawns the heartbeat +
// metadata goroutines. Idempotent — second call returns nil. After
// Shutdown, Start is rejected.
func (a *Agent) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return errors.New("agent: already shut down")
	}
	if a.started {
		return nil
	}

	subject := a.subjects.AgentCommand(a.cfg.AgentID)
	sub, err := a.nats.Subscribe(subject, a.handleCommand)
	if err != nil {
		return fmt.Errorf("agent: subscribe %q: %w", subject, err)
	}
	a.sub = sub

	loopCtx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	a.wg.Add(2)
	go a.runHeartbeatLoop(loopCtx)
	go a.runMetadataLoop(loopCtx)

	a.started = true
	a.log.Info("agent: started",
		"agent_id", a.cfg.AgentID,
		"cluster", a.subjects.Cluster(),
		"heartbeat_interval", a.cfg.HeartbeatInterval,
		"metadata_interval", a.cfg.MetadataInterval,
	)
	return nil
}

// Shutdown unsubscribes, cancels the loop ctx, and waits for the
// loop goroutines to exit. Bounded by ctx — if the wait exceeds the
// caller's deadline, Shutdown returns ctx.Err() but the goroutines
// continue draining in the background.
//
// Idempotent; safe to call before Start.
func (a *Agent) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	if !a.started || a.stopped {
		a.stopped = true
		a.mu.Unlock()
		return nil
	}
	cancel := a.cancel
	sub := a.sub
	a.cancel = nil
	a.sub = nil
	a.stopped = true
	a.mu.Unlock()

	var firstErr error
	if sub != nil {
		if err := sub.Unsubscribe(); err != nil {
			firstErr = fmt.Errorf("agent: unsubscribe: %w", err)
		}
	}
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if firstErr == nil {
			firstErr = fmt.Errorf("agent: shutdown wait: %w", ctx.Err())
		}
	}

	a.log.Info("agent: stopped", "agent_id", a.cfg.AgentID)
	return firstErr
}

// runHeartbeatLoop ticks every HeartbeatInterval. Standard
// time.Ticker semantics — first fire happens after the interval, so
// the control plane has a registration window before the first
// heartbeat. Task 3 swaps the stub payload for real gopsutil-backed
// metrics; Task 5 may add registration on Start to populate the
// agent registry before the first tick.
func (a *Agent) runHeartbeatLoop(ctx context.Context) {
	defer a.wg.Done()
	t := time.NewTicker(a.cfg.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.publishHeartbeat(ctx)
		}
	}
}

func (a *Agent) publishHeartbeat(ctx context.Context) {
	payload, err := json.Marshal(heartbeatPayload{
		AgentID: a.cfg.AgentID,
		TS:      a.now().UTC(),
	})
	if err != nil {
		a.log.Warn("agent: heartbeat marshal", "err", err)
		return
	}
	env := envelope.New(payload, a.subjects.Prefix(),
		envelope.WithPriority(envelope.PriorityNormal),
	)
	if err := a.nats.PublishEnvelope(ctx, a.subjects.AgentHeartbeat(), env); err != nil {
		// Log but do not exit; nats.go's reconnect handles transient
		// outages.
		a.log.Warn("agent: heartbeat publish", "err", err)
	}
}

// runMetadataLoop ticks every MetadataInterval. Stub payload for
// Task 1; Task 3 wires the gopsutil-backed metadata collector.
func (a *Agent) runMetadataLoop(ctx context.Context) {
	defer a.wg.Done()
	t := time.NewTicker(a.cfg.MetadataInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.publishMetadata(ctx)
		}
	}
}

func (a *Agent) publishMetadata(ctx context.Context) {
	payload, err := json.Marshal(metadataPayload{
		AgentID: a.cfg.AgentID,
		Labels:  a.cfg.Labels,
	})
	if err != nil {
		a.log.Warn("agent: metadata marshal", "err", err)
		return
	}
	env := envelope.New(payload, a.subjects.Prefix())
	subject := a.subjects.AgentState(a.cfg.AgentID)
	if err := a.nats.PublishEnvelope(ctx, subject, env); err != nil {
		a.log.Warn("agent: metadata publish", "err", err)
	}
}

// handleCommand is the v1.0 stub: log the receipt and discard.
// Tasks 2/4/5 wire HMAC validation (SecurityEnforcer) → Executor →
// response publication. The handler intentionally does not error
// out on stubbed receipts so the test surface stays narrow.
func (a *Agent) handleCommand(_ context.Context, subject string, env envelope.Envelope) error {
	a.log.Info("agent: command received (stub handler — Task 5 wires response)",
		"subject", subject,
		"message_id", env.MessageID,
		"correlation_id", env.CorrelationID,
		"payload_bytes", len(env.Payload),
	)
	return nil
}

// heartbeatPayload is the v1.0 stub. Task 3 swaps this for a
// gopsutil-backed struct (CPU%, memory%, disk%).
type heartbeatPayload struct {
	AgentID string    `json:"agent_id"`
	TS      time.Time `json:"ts"`
}

// metadataPayload is the v1.0 stub. Task 3 swaps this for a
// gopsutil-backed full metadata struct (distro, kernel, NICs, etc.).
type metadataPayload struct {
	AgentID string            `json:"agent_id"`
	Labels  map[string]string `json:"labels,omitempty"`
}
