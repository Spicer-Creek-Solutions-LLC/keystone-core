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

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// AgentResponsePayload is the wire shape the agent publishes on
// kscore.{cluster}.agent.{id}.response. Mirrors internal/agent.
// CommandResponse — declared here to avoid the controlplane → agent
// import that would crack the existing layering.
type AgentResponsePayload struct {
	MessageID       string `json:"message_id"`
	AgentID         string `json:"agent_id"`
	ExitCode        int    `json:"exit_code"`
	Stdout          []byte `json:"stdout,omitempty"`
	Stderr          []byte `json:"stderr,omitempty"`
	DurationMs      int64  `json:"duration_ms"`
	TimedOut        bool   `json:"timed_out"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	Error           string `json:"error,omitempty"`
	Rejected        bool   `json:"rejected"`
	RejectReason    string `json:"reject_reason,omitempty"`
}

// ResponseRouter subscribes to the agent-response wildcard and fans
// each inbound CommandResponse into:
//
//  1. A waiter channel registered by NATSBatchExecutor — keyed by the
//     envelope CorrelationID (== command ID).
//  2. CommandDispatcher.RecordResult so the commands row terminates.
//
// Late or duplicate responses (no waiter, command already finalized)
// are logged at debug and dropped.
type ResponseRouter struct {
	subscriber  Subscriber
	subjects    Subjects
	dispatcher  *CommandDispatcher
	logger      *slog.Logger

	mu      sync.Mutex
	waiters map[string]chan AgentResponsePayload // by CorrelationID
	sub     Subscription
	started bool
	closed  bool
}

// ResponseRouterConfig configures a ResponseRouter. Subscriber and
// Subjects are required; Dispatcher is optional (when nil, inbound
// responses skip the RecordResult side-effect — useful in tests that
// don't model the commands table).
type ResponseRouterConfig struct {
	Subscriber Subscriber
	Subjects   Subjects
	Dispatcher *CommandDispatcher
	Logger     *slog.Logger
}

// NewResponseRouter validates cfg and returns a router that has not
// yet subscribed. Call Start to attach the NATS subscription.
func NewResponseRouter(cfg ResponseRouterConfig) (*ResponseRouter, error) {
	if cfg.Subscriber == nil {
		return nil, errors.New("controlplane: ResponseRouter requires a Subscriber")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("controlplane: ResponseRouter requires Subjects")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &ResponseRouter{
		subscriber: cfg.Subscriber,
		subjects:   cfg.Subjects,
		dispatcher: cfg.Dispatcher,
		logger:     cfg.Logger,
		waiters:    make(map[string]chan AgentResponsePayload),
	}, nil
}

// Start subscribes to the agent-response wildcard. Idempotent (second
// call is a no-op).
func (r *ResponseRouter) Start(_ context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	r.started = true
	r.mu.Unlock()

	pattern := r.subjects.AgentResponsePattern()
	sub, err := r.subscriber.Subscribe(pattern, r.handle)
	if err != nil {
		r.mu.Lock()
		r.started = false
		r.mu.Unlock()
		return fmt.Errorf("controlplane: subscribe %s: %w", pattern, err)
	}
	r.mu.Lock()
	r.sub = sub
	r.mu.Unlock()
	r.logger.Debug("controlplane: response router subscribed", "subject", pattern)
	return nil
}

// Stop unsubscribes and releases all registered waiters with a
// shutdown sentinel. Idempotent.
func (r *ResponseRouter) Stop() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	sub := r.sub
	r.sub = nil
	waiters := r.waiters
	r.waiters = make(map[string]chan AgentResponsePayload)
	r.mu.Unlock()

	if sub != nil {
		if err := sub.Unsubscribe(); err != nil {
			r.logger.Warn("controlplane: unsubscribe response pattern", "err", err)
		}
	}
	for _, ch := range waiters {
		close(ch)
	}
	return nil
}

// register installs a waiter for the given correlation ID. Returns
// the channel the response will arrive on plus a cancel func the
// caller MUST defer (deregisters on timeout / ctx-cancel).
func (r *ResponseRouter) register(corrID string) (<-chan AgentResponsePayload, func()) {
	ch := make(chan AgentResponsePayload, 1)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	r.waiters[corrID] = ch
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		if existing, ok := r.waiters[corrID]; ok && existing == ch {
			delete(r.waiters, corrID)
		}
		r.mu.Unlock()
	}
}

// handle is the inbound NATS message handler. Errors are logged but
// not returned upstream — a botched response shouldn't kill the
// subscription.
func (r *ResponseRouter) handle(ctx context.Context, _ string, env envelope.Envelope) error {
	var payload AgentResponsePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		r.logger.Warn("controlplane: decode response payload",
			"correlation_id", env.CorrelationID, "err", err)
		return nil
	}

	corrID := env.CorrelationID
	if corrID == "" {
		// Fallback for older agent builds that didn't stamp the
		// envelope CorrelationID — use the payload's MessageID
		// (canonically equal to the inbound command ID).
		corrID = payload.MessageID
	}

	if r.dispatcher != nil {
		result := state.CommandResult{
			ExitCode:    payload.ExitCode,
			Stdout:      string(payload.Stdout),
			Stderr:      string(payload.Stderr),
			CompletedAt: time.Now(),
			Status:      commandStatusFromResponse(payload),
		}
		if err := r.dispatcher.RecordResult(ctx, corrID, result); err != nil &&
			!errors.Is(err, ErrCommandFinalized) && !errors.Is(err, ErrCommandNotFound) {
			r.logger.Warn("controlplane: record command result",
				"correlation_id", corrID, "err", err)
		}
	}

	r.mu.Lock()
	ch := r.waiters[corrID]
	r.mu.Unlock()
	if ch == nil {
		r.logger.Debug("controlplane: no waiter for response (late or unsolicited)",
			"correlation_id", corrID, "agent_id", payload.AgentID)
		return nil
	}
	select {
	case ch <- payload:
	default:
		// Buffer is 1; a duplicate response would block. Drop with
		// a warn.
		r.logger.Warn("controlplane: duplicate response dropped",
			"correlation_id", corrID, "agent_id", payload.AgentID)
	}
	return nil
}

// commandStatusFromResponse maps the agent's wire-format flags onto
// the persisted CommandStatus enum.
func commandStatusFromResponse(p AgentResponsePayload) state.CommandStatus {
	switch {
	case p.Rejected:
		return state.CommandStatusFailed
	case p.TimedOut:
		return state.CommandStatusTimeout
	case p.ExitCode == 0 && p.Error == "":
		return state.CommandStatusCompleted
	default:
		return state.CommandStatusFailed
	}
}
