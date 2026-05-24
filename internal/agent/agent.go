// SPDX-License-Identifier: Apache-2.0

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
	BootstrapRegister(agentID string) string
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

	// BootstrapPSK is the hex-encoded pre-shared key the agent
	// presents on the bootstrap-register subject at Start. Empty
	// disables the bootstrap publish — the agent falls back to
	// heartbeats-only, which only register the agent if a prior
	// boot already inserted the row (operator-pre-provisioned).
	BootstrapPSK string //nolint:gosec // PSK hex string — flagged false-positive on field-name pattern
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
// Task 5 wires the full command flow: handleCommand parses the
// inbound CommandRequest, validates via SecurityEnforcer, executes
// via Executor, and publishes a CommandResponse on the response
// subject (with CorrelationID = inbound MessageID). Rejection paths
// publish a Rejected=true response so the dispatcher sees a clear
// reason rather than waiting for a timeout.
type Agent struct {
	cfg      Config
	log      *slog.Logger
	nats     NATSClient
	subjects Subjects
	metrics  MetricsCollector
	executor *Executor
	enforcer *SecurityEnforcer

	mu         sync.Mutex
	started    bool
	stopped    bool
	cancel     context.CancelFunc
	commandCtx context.Context // shared by handleCommand goroutines; cancel() ends them
	wg         sync.WaitGroup  // tracks heartbeat + metadata loops + in-flight commands
	sub        Subscription
}

// New validates cfg and returns an unstarted Agent. AgentID is
// required; the bootstrap engine (Task 6) persists it. Other fields
// fall back to §4.6 defaults when zero.
func New(cfg Config, nats NATSClient, subjects Subjects, metrics MetricsCollector, executor *Executor, enforcer *SecurityEnforcer, log *slog.Logger) (*Agent, error) {
	if cfg.AgentID == "" {
		return nil, errors.New("agent: AgentID is required")
	}
	if nats == nil {
		return nil, errors.New("agent: NATSClient is required")
	}
	if subjects == nil {
		return nil, errors.New("agent: Subjects is required")
	}
	if metrics == nil {
		return nil, errors.New("agent: MetricsCollector is required")
	}
	if executor == nil {
		return nil, errors.New("agent: Executor is required")
	}
	if enforcer == nil {
		return nil, errors.New("agent: SecurityEnforcer is required")
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
		metrics:  metrics,
		executor: executor,
		enforcer: enforcer,
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
	// commandCtx shares the same root cancel so handleCommand
	// observes Shutdown via ctx.Done() and Executor.waitWithKill-
	// Protocol's SIGTERM-grace-then-SIGKILL kicks in for in-flight
	// child processes. PROJECT-DETAILS §4.6 graceful shutdown.
	a.commandCtx = loopCtx

	// Epic 19 task 2 — when a PSK is configured, fire-and-forget a
	// bootstrap-register envelope so the server can insert the
	// agent row before the first heartbeat lands. Failure to
	// publish is non-fatal: heartbeats arrive on the same NATS
	// connection and the operator can see the agent log line.
	if a.cfg.BootstrapPSK != "" {
		if err := a.publishBootstrapRegister(loopCtx); err != nil {
			a.log.Warn("agent: bootstrap register publish",
				"agent_id", a.cfg.AgentID, "err", err)
		}
	}

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

// Shutdown unsubscribes, cancels the loop+command ctx, and waits
// for every tracked goroutine (heartbeat loop, metadata loop, in-
// flight command handlers — Task 11) to exit. Bounded by ctx — if
// the wait exceeds the caller's deadline, Shutdown emits a WARN
// log and returns ctx.Err() but the goroutines continue draining
// in the background. Operators see the WARN in journalctl and
// know in-flight commands didn't drain cleanly.
//
// Order: stop-flag → unsubscribe → cancel → wait. The stop-flag
// flip happens under mutex BEFORE Unsubscribe; new callbacks
// arriving in the race window between Unsubscribe and
// acquireCommandSlot see stopped=true and short-circuit.
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
		// Drain timed out. Operators reading journalctl after a
		// hung shutdown need a clear signal that in-flight work
		// may still be running — the goroutines continue in the
		// background until the process exits. PROJECT-DETAILS §4.6
		// "Bootstrap idempotency / fd leaks" risk.
		a.log.Warn("agent: command drain timed out",
			"agent_id", a.cfg.AgentID,
			"err", ctx.Err(),
		)
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
// heartbeat. Payload comes from MetricsCollector.Heartbeat (gopsutil
// in production); Task 5 may add registration on Start to populate
// the agent registry before the first tick.
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

// publishBootstrapRegister fires a single bootstrap-register envelope
// carrying the PSK proof. Best-effort — the heartbeat loop is the
// retry path. The server's BootstrapHandler upserts the agent row,
// so duplicate publishes from agent restarts are safe (rejected by
// the consumed-PSK check until operator rotates).
func (a *Agent) publishBootstrapRegister(ctx context.Context) error {
	req := struct {
		AgentID string `json:"agent_id"`
		Proof   string `json:"proof"`
		Agent   struct {
			Labels map[string]string `json:"labels,omitempty"`
		} `json:"agent,omitempty"`
	}{
		AgentID: a.cfg.AgentID,
		Proof:   a.cfg.BootstrapPSK,
	}
	req.Agent.Labels = a.cfg.Labels
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("agent: bootstrap marshal: %w", err)
	}
	env := envelope.New(payload, a.subjects.Prefix(),
		envelope.WithPriority(envelope.PriorityNormal),
	)
	subject := a.subjects.BootstrapRegister(a.cfg.AgentID)
	if err := a.nats.PublishEnvelope(ctx, subject, env); err != nil {
		return fmt.Errorf("agent: bootstrap publish %q: %w", subject, err)
	}
	a.log.Info("agent: bootstrap register published",
		"agent_id", a.cfg.AgentID, "subject", subject)
	return nil
}

func (a *Agent) publishHeartbeat(ctx context.Context) {
	hb := a.metrics.Heartbeat(ctx, a.cfg.AgentID)
	payload, err := json.Marshal(hb)
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

// runMetadataLoop ticks every MetadataInterval and publishes the
// full per-host metadata snapshot via MetricsCollector.Metadata.
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
	md := a.metrics.Metadata(ctx, a.cfg.AgentID, a.cfg.Labels)
	payload, err := json.Marshal(md)
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

// handleCommand is the §4.6 command flow: parse → SecurityEnforcer.
// Validate → Executor.Execute → publish CommandResponse on the
// response subject. CorrelationID on the response Envelope = the
// inbound MessageID so the control plane's dispatcher can match.
//
// Rejection paths (HMAC, allowlist, args-too-long) publish a
// Rejected=true response so the dispatcher surfaces a clean reason
// rather than waiting for a timeout. Errors returned to the caller
// are warnings only — pub/sub is fire-and-forget at this layer.
//
// Shutdown semantics (Task 11): handleCommand registers itself in
// the agent's WaitGroup before doing real work, then uses
// a.commandCtx (cancelled by Shutdown) instead of the param ctx
// from nats.go's subscription callback (which is context.Background).
// This way Executor.waitWithKillProtocol's SIGTERM grace fires on
// shutdown, in-flight commands drain, and the wg.Wait in Shutdown
// blocks until they exit. The param `ctx` is kept in the signature
// for MessageHandler conformance but not used for cancellable work.
func (a *Agent) handleCommand(_ context.Context, subject string, env envelope.Envelope) error {
	cmdCtx, ok := a.acquireCommandSlot(subject, env.MessageID)
	if !ok {
		return nil
	}
	defer a.wg.Done()

	var req CommandRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		a.log.Warn("agent: command unmarshal",
			"subject", subject, "message_id", env.MessageID, "err", err)
		return nil
	}
	// Envelope MessageID is authoritative; it's part of the HMAC
	// canonical input. If the message wasn't published with the same
	// MessageID stamped on the envelope, the signature won't verify.
	req.MessageID = env.MessageID

	if err := a.enforcer.Validate(cmdCtx, req); err != nil {
		a.publishResponse(cmdCtx, env.MessageID, &CommandResponse{
			MessageID:    env.MessageID,
			AgentID:      a.cfg.AgentID,
			ExitCode:     -1,
			Rejected:     true,
			RejectReason: err.Error(),
		})
		return nil
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = a.cfg.CommandTimeout
	}
	result := a.executor.Execute(cmdCtx, ExecuteRequest{
		Command:    req.Command,
		Args:       req.Args,
		Env:        a.enforcer.AppliedEnv(req),
		WorkingDir: req.WorkingDir,
		User:       req.User,
		Timeout:    timeout,
	})

	a.publishResponse(cmdCtx, env.MessageID, &CommandResponse{
		MessageID:       env.MessageID,
		AgentID:         a.cfg.AgentID,
		ExitCode:        result.ExitCode,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		DurationMs:      result.Duration.Milliseconds(),
		TimedOut:        result.TimedOut,
		StdoutTruncated: result.StdoutTruncated,
		StderrTruncated: result.StderrTruncated,
		Error:           result.Error,
	})
	return nil
}

// acquireCommandSlot is the lock-protected entry section that
// keeps in-flight commands counted by wg AND refuses new work
// after Shutdown started. The race we're guarding: nats.go has
// already invoked the subscription callback for a message, but
// Shutdown's stopped=true flip + Unsubscribe interleave with
// us. By doing the check + wg.Add under the same mutex Shutdown
// uses, we guarantee handleCommand either:
//
//   - Sees stopped=false, increments wg, and Shutdown's wg.Wait
//     blocks for it to finish (drain), OR
//   - Sees stopped=true, returns early, no wg increment.
//
// Returns the cancellable command ctx + ok=true on success.
func (a *Agent) acquireCommandSlot(subject, messageID string) (context.Context, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped || !a.started {
		a.log.Debug("agent: command rejected; agent shut down",
			"subject", subject, "message_id", messageID)
		return nil, false
	}
	a.wg.Add(1)
	return a.commandCtx, true
}

func (a *Agent) publishResponse(ctx context.Context, correlationID string, resp *CommandResponse) {
	payload, err := json.Marshal(resp)
	if err != nil {
		a.log.Warn("agent: response marshal",
			"agent_id", a.cfg.AgentID, "message_id", correlationID, "err", err)
		return
	}
	respEnv := envelope.New(payload, a.subjects.Prefix(),
		envelope.WithCorrelationID(correlationID),
	)
	subject := a.subjects.AgentResponse(a.cfg.AgentID)
	if err := a.nats.PublishEnvelope(ctx, subject, respEnv); err != nil {
		a.log.Warn("agent: response publish",
			"agent_id", a.cfg.AgentID, "message_id", correlationID, "err", err)
	}
}

