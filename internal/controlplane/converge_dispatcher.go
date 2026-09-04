// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// ConvergeSubjects is the subject surface the converge dispatcher
// needs. internal/nats.SubjectBuilder satisfies it structurally.
type ConvergeSubjects interface {
	AgentConverge(agentID string) string
	AgentConvergeResultPattern() string
	Prefix() string
}

// ConvergeSigner signs an outbound ConvergeRequest. The agent's
// SecurityEnforcer satisfies it — deliberately the same type on both
// ends, so the canonical encoding cannot drift between signer and
// verifier.
type ConvergeSigner interface {
	ComputeConvergeHMAC(req agent.ConvergeRequest) string
}

// ConvergeDispatcher sends a state file to agents and collects their
// results, mirroring CommandDispatcher/ResponseRouter for state runs.
//
// One subscription on the converge-result wildcard fans every agent's
// reply into a waiter keyed by the request's MessageID, so N agents
// converging concurrently need N waiters and one subscription rather
// than a subscription each.
type ConvergeDispatcher struct {
	subscriber Subscriber
	publisher  NATSPublisher
	subjects   ConvergeSubjects
	signer     ConvergeSigner
	logger     *slog.Logger

	mu      sync.Mutex
	waiters map[string]chan agent.ConvergeResponse
	sub     Subscription
	started bool
	closed  bool
}

// ConvergeDispatcherConfig configures a ConvergeDispatcher. Subscriber,
// Publisher and Subjects are required. Signer may be nil only when the
// deployment runs without an HMAC secret, which the agent tolerates
// for the same bootstrap reason it tolerates an unsigned command.
type ConvergeDispatcherConfig struct {
	Subscriber Subscriber
	Publisher  NATSPublisher
	Subjects   ConvergeSubjects
	Signer     ConvergeSigner
	Logger     *slog.Logger
}

// NewConvergeDispatcher validates cfg and returns a dispatcher that
// has not yet subscribed. Call Start to attach the subscription.
func NewConvergeDispatcher(cfg ConvergeDispatcherConfig) (*ConvergeDispatcher, error) {
	if cfg.Subscriber == nil {
		return nil, errors.New("controlplane: ConvergeDispatcher requires a Subscriber")
	}
	if cfg.Publisher == nil {
		return nil, errors.New("controlplane: ConvergeDispatcher requires a Publisher")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("controlplane: ConvergeDispatcher requires Subjects")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &ConvergeDispatcher{
		subscriber: cfg.Subscriber,
		publisher:  cfg.Publisher,
		subjects:   cfg.Subjects,
		signer:     cfg.Signer,
		logger:     cfg.Logger,
		waiters:    make(map[string]chan agent.ConvergeResponse),
	}, nil
}

// Start subscribes to the converge-result wildcard. Idempotent.
func (d *ConvergeDispatcher) Start(_ context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return nil
	}
	d.started = true
	d.mu.Unlock()

	pattern := d.subjects.AgentConvergeResultPattern()
	sub, err := d.subscriber.Subscribe(pattern, d.handle)
	if err != nil {
		d.mu.Lock()
		d.started = false
		d.mu.Unlock()
		return fmt.Errorf("controlplane: subscribe %s: %w", pattern, err)
	}
	d.mu.Lock()
	d.sub = sub
	d.mu.Unlock()
	d.logger.Debug("controlplane: converge dispatcher subscribed", "subject", pattern)
	return nil
}

// Stop unsubscribes and releases every waiter. Idempotent.
func (d *ConvergeDispatcher) Stop() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	sub := d.sub
	d.sub = nil
	waiters := d.waiters
	d.waiters = make(map[string]chan agent.ConvergeResponse)
	d.mu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
	if sub != nil {
		return sub.Unsubscribe()
	}
	return nil
}

// ConvergeTarget is one agent to converge and the run it belongs to.
type ConvergeTarget struct {
	AgentID string
	RunID   string
	Source  string
	Mode    string
	YAML    []byte
	Vars    map[string]string
	// Principal is the operator on whose behalf the run is dispatched.
	// Carried into the signature and checked against the agent's
	// principal allowlist.
	Principal string
	Timeout   time.Duration
}

// Converge dispatches to one agent and waits for its result.
//
// A per-agent timeout rather than one for the whole fleet: a single
// unreachable host must not decide how long every other host's result
// is allowed to take.
func (d *ConvergeDispatcher) Converge(ctx context.Context, t ConvergeTarget) (agent.ConvergeResponse, error) {
	if t.AgentID == "" {
		return agent.ConvergeResponse{}, errors.New("controlplane: converge requires an agent id")
	}
	msgID := t.RunID + ":" + t.AgentID

	req := agent.ConvergeRequest{
		MessageID:      msgID,
		Principal:      t.Principal,
		RunID:          t.RunID,
		Source:         t.Source,
		Mode:           t.Mode,
		YAML:           t.YAML,
		Variables:      t.Vars,
		TimeoutSeconds: int(t.Timeout.Seconds()),
	}
	if d.signer != nil {
		req.Signature = d.signer.ComputeConvergeHMAC(req)
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return agent.ConvergeResponse{}, fmt.Errorf("controlplane: marshal converge request: %w", err)
	}

	// Register BEFORE publishing: a fast agent can answer before
	// Publish returns, and an unregistered waiter drops that reply.
	ch, release := d.register(msgID)
	defer release()

	env := envelope.New(payload, d.subjects.Prefix(), envelope.WithMessageID(msgID))
	if err := d.publisher.PublishEnvelope(ctx, d.subjects.AgentConverge(t.AgentID), env); err != nil {
		return agent.ConvergeResponse{}, fmt.Errorf("controlplane: dispatch converge to %s: %w", t.AgentID, err)
	}

	waitCtx := ctx
	if t.Timeout > 0 {
		var cancel context.CancelFunc
		// Allow a grace beyond the agent's own budget so a run that
		// times out ON the agent still reports its partial results
		// rather than being reported here as unreachable.
		waitCtx, cancel = context.WithTimeout(ctx, t.Timeout+convergeGrace)
		defer cancel()
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return agent.ConvergeResponse{}, errors.New("controlplane: converge dispatcher stopped")
		}
		return resp, nil
	case <-waitCtx.Done():
		return agent.ConvergeResponse{}, fmt.Errorf("controlplane: converge %s: %w", t.AgentID, waitCtx.Err())
	}
}

// convergeGrace is how long the control plane waits past the agent's
// own timeout before calling a run unreachable.
const convergeGrace = 5 * time.Second

func (d *ConvergeDispatcher) register(msgID string) (<-chan agent.ConvergeResponse, func()) {
	ch := make(chan agent.ConvergeResponse, 1)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	d.waiters[msgID] = ch
	d.mu.Unlock()
	return ch, func() {
		d.mu.Lock()
		if existing, ok := d.waiters[msgID]; ok && existing == ch {
			delete(d.waiters, msgID)
		}
		d.mu.Unlock()
	}
}

// handle routes an inbound result to its waiter. Errors are logged,
// never returned — one malformed reply must not kill the subscription
// that every other agent's results arrive on.
func (d *ConvergeDispatcher) handle(_ context.Context, _ string, env envelope.Envelope) error {
	var resp agent.ConvergeResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		d.logger.Warn("controlplane: decode converge result",
			"correlation_id", env.CorrelationID, "err", err)
		return nil
	}
	corrID := env.CorrelationID
	if corrID == "" {
		corrID = resp.MessageID
	}

	d.mu.Lock()
	ch := d.waiters[corrID]
	d.mu.Unlock()
	if ch == nil {
		d.logger.Debug("controlplane: no waiter for converge result (late or unsolicited)",
			"correlation_id", corrID, "agent_id", resp.AgentID)
		return nil
	}
	select {
	case ch <- resp:
	default:
		d.logger.Warn("controlplane: duplicate converge result dropped",
			"correlation_id", corrID, "agent_id", resp.AgentID)
	}
	return nil
}
