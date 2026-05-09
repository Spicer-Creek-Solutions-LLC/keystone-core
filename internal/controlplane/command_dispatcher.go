package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// Dispatcher defaults align with PROJECT-DETAILS §4.4 step 7 (retention
// loop) and §4.7 (per-command timeout). Cluster name comes from the
// Subjects builder (Task 4); the v1.0 single-cluster default lives in
// internal/config.NATSConfig.
const (
	DefaultCommandTimeoutSeconds  = 300
	DefaultRetentionWindow        = 7 * 24 * time.Hour
	DefaultRetentionInterval      = time.Hour
	DefaultTimeoutCheckInterval   = time.Second
)

// terminalCommandStatuses is the retention-allowlist for the retention
// loop. Pending and running rows are never auto-pruned.
var terminalCommandStatuses = []state.CommandStatus{
	state.CommandStatusCompleted,
	state.CommandStatusFailed,
	state.CommandStatusTimeout,
	state.CommandStatusCancelled,
}

// NATSPublisher is the minimal NATS publish surface the dispatcher
// needs. internal/nats.Manager (via the pkg/api/server adapter)
// satisfies it; tests use a fake. PROJECT-DETAILS §4.2 mandates an
// Envelope around every published message — Task 5 retired the
// byte-level publish path in favor of PublishEnvelope.
type NATSPublisher interface {
	PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error
}

// Subjects is the narrow subject-construction surface the dispatcher
// needs. internal/nats.SubjectBuilder satisfies it structurally so
// controlplane stays free of NATS imports. Future epics that add
// new dispatcher subjects extend this interface here, not by
// importing internal/nats.
//
// Prefix returns "kscore.{cluster}" — the value stamped into every
// envelope's ClusterPrefix field. The publish path (Manager.
// PublishEnvelope) cross-checks this against its own builder and
// rejects mismatches, so a wrong-cluster envelope cannot leak.
type Subjects interface {
	AgentCommand(agentID string) string
	BootstrapRegisterPattern() string
	BootstrapResponse(agentID string) string
	Cluster() string
	Prefix() string
}

// AgentLookup reads the agent registry. ConnectionManager (task 1)
// satisfies it; tests substitute fakes when NATS or storage isn't the
// behavior under test.
type AgentLookup interface {
	Get(ctx context.Context, id string) (*state.AgentRecord, error)
}

// DispatchRequest is the input shape for a single-agent command
// dispatch. It mirrors state.CommandRecord minus persistence-only
// fields (status, ID, timestamps, output). The dispatcher generates
// the ID and stamps timestamps.
type DispatchRequest struct {
	AgentID        string
	Command        string
	Args           []string
	Env            map[string]string
	WorkingDir     string
	User           string
	Principal      string // who issued the command (auth context); fed into HMAC canonical
	TimeoutSeconds int    // 0 → DispatcherConfig.DefaultTimeoutSeconds
}

// Signer signs the canonical-encoded fields of a CommandMessage and
// returns the hex HMAC. internal/agent.SecurityEnforcer satisfies
// this via its ComputeHMAC method (using a small adapter at the
// wiring layer in pkg/api/server). Empty implementation can return
// "" — the agent's escape-hatch path accepts unsigned messages
// when its own HMACSecret is empty.
type Signer interface {
	SignCommand(msg CommandMessage) string
}

// CommandMessage is the wire-format payload published on the
// agent's command subject. Mirrors internal/agent.CommandRequest
// — the same struct the agent unmarshals and feeds into
// SecurityEnforcer.Validate. Defined here to keep
// internal/controlplane import-free of internal/agent;
// internal/agent.CommandRequest is the canonical name on the
// receive side, but the field set is identical so signers
// produce HMACs the agent verifies.
type CommandMessage struct {
	MessageID      string            `json:"message_id"`
	Principal      string            `json:"principal"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	User           string            `json:"user,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Signature      string            `json:"signature"`
}

// DispatcherConfig configures a CommandDispatcher. Store, Agents,
// Publisher, Subjects, and Signer are required; everything else has
// a default.
type DispatcherConfig struct {
	Store                 state.CommandStore
	Agents                AgentLookup
	Publisher             NATSPublisher
	Subjects              Subjects
	Signer                Signer
	Logger                *slog.Logger
	DefaultTimeoutSeconds int
	RetentionWindow       time.Duration
	RetentionInterval     time.Duration
	TimeoutCheckInterval  time.Duration
	Clock                 func() time.Time
	NewID                 func() string
}

// inFlightCommand tracks an in-progress command for timeout
// enforcement. The map is keyed by command ID.
type inFlightCommand struct {
	agentID  string
	deadline time.Time
}

// CommandDispatcher persists, publishes, times out, and prunes
// commands. It is the v1.0 implementation of §4.4 step 7.
type CommandDispatcher struct {
	store                 state.CommandStore
	agents                AgentLookup
	publisher             NATSPublisher
	subjects              Subjects
	signer                Signer
	logger                *slog.Logger
	defaultTimeoutSeconds int
	retentionWindow       time.Duration
	retentionInterval     time.Duration
	timeoutCheckInterval  time.Duration
	now                   func() time.Time
	newID                 func() string

	mu       sync.Mutex
	inflight map[string]inFlightCommand

	startOnce sync.Once
	stopOnce  sync.Once
	started   bool
	closed    bool
	cancel    context.CancelFunc
	loopsDone chan struct{}
}

// NewDispatcher validates cfg, fills defaults, and returns a dispatcher
// that has not yet been started.
func NewDispatcher(cfg DispatcherConfig) (*CommandDispatcher, error) {
	if cfg.Store == nil {
		return nil, errors.New("controlplane: dispatcher Store is required")
	}
	if cfg.Agents == nil {
		return nil, errors.New("controlplane: dispatcher Agents is required")
	}
	if cfg.Publisher == nil {
		return nil, errors.New("controlplane: dispatcher Publisher is required")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("controlplane: dispatcher Subjects is required")
	}
	if cfg.Signer == nil {
		return nil, errors.New("controlplane: dispatcher Signer is required")
	}
	if cfg.RetentionWindow < 0 {
		return nil, fmt.Errorf("controlplane: RetentionWindow must be >= 0, got %s", cfg.RetentionWindow)
	}
	if cfg.RetentionInterval < 0 {
		return nil, fmt.Errorf("controlplane: RetentionInterval must be >= 0, got %s", cfg.RetentionInterval)
	}
	if cfg.TimeoutCheckInterval < 0 {
		return nil, fmt.Errorf("controlplane: TimeoutCheckInterval must be >= 0, got %s", cfg.TimeoutCheckInterval)
	}
	if cfg.DefaultTimeoutSeconds < 0 {
		return nil, fmt.Errorf("controlplane: DefaultTimeoutSeconds must be >= 0, got %d", cfg.DefaultTimeoutSeconds)
	}

	if cfg.DefaultTimeoutSeconds == 0 {
		cfg.DefaultTimeoutSeconds = DefaultCommandTimeoutSeconds
	}
	if cfg.RetentionWindow == 0 {
		cfg.RetentionWindow = DefaultRetentionWindow
	}
	if cfg.RetentionInterval == 0 {
		cfg.RetentionInterval = DefaultRetentionInterval
	}
	if cfg.TimeoutCheckInterval == 0 {
		cfg.TimeoutCheckInterval = DefaultTimeoutCheckInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = uuidString
	}

	return &CommandDispatcher{
		store:                 cfg.Store,
		agents:                cfg.Agents,
		publisher:             cfg.Publisher,
		subjects:              cfg.Subjects,
		signer:                cfg.Signer,
		logger:                cfg.Logger,
		defaultTimeoutSeconds: cfg.DefaultTimeoutSeconds,
		retentionWindow:       cfg.RetentionWindow,
		retentionInterval:     cfg.RetentionInterval,
		timeoutCheckInterval:  cfg.TimeoutCheckInterval,
		now:                   cfg.Clock,
		newID:                 cfg.NewID,
		inflight:              make(map[string]inFlightCommand),
		loopsDone:             make(chan struct{}),
	}, nil
}

// Start launches the timeout watcher and retention loop. Idempotent.
func (d *CommandDispatcher) Start(ctx context.Context) error {
	var startErr error
	d.startOnce.Do(func() {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			startErr = ErrClosed
			return
		}
		d.started = true
		loopCtx, cancel := context.WithCancel(context.Background())
		d.cancel = cancel
		d.mu.Unlock()

		go d.runLoops(loopCtx)
		d.logger.Info("controlplane: command dispatcher started",
			"cluster", d.subjects.Cluster(),
			"retention_window", d.retentionWindow,
			"retention_interval", d.retentionInterval,
			"timeout_check_interval", d.timeoutCheckInterval,
		)
	})
	return startErr
}

// Stop signals the loops to exit and waits for them, bounded by ctx.
// Idempotent. After Stop, mutating methods return ErrClosed.
func (d *CommandDispatcher) Stop(ctx context.Context) error {
	var stopErr error
	d.stopOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		cancel := d.cancel
		started := d.started
		d.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if !started {
			return
		}
		select {
		case <-d.loopsDone:
		case <-ctx.Done():
			stopErr = fmt.Errorf("controlplane: dispatcher stop: %w", ctx.Err())
		}
	})
	return stopErr
}

// Dispatch persists a pending command, publishes it on the agent's
// command subject, and transitions the persisted record to "running".
// Returns the generated command ID. Errors are surfaced to the caller;
// on publish failure the record is marked failed before the error is
// returned so operators can audit attempts that never reached the
// agent.
func (d *CommandDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (string, error) {
	if err := d.checkOpen(); err != nil {
		return "", err
	}
	if req.AgentID == "" {
		return "", fmt.Errorf("%w: AgentID is required", ErrInvalidDispatch)
	}
	if req.Command == "" {
		return "", fmt.Errorf("%w: Command is required", ErrInvalidDispatch)
	}

	agent, err := d.agents.Get(ctx, req.AgentID)
	if err != nil || agent == nil {
		return "", fmt.Errorf("%w: %v", ErrAgentUnreachable, err)
	}
	if agent.Status == state.AgentStatusDisabled {
		return "", fmt.Errorf("%w: agent %q is disabled", ErrAgentUnreachable, req.AgentID)
	}

	timeoutSecs := req.TimeoutSeconds
	if timeoutSecs == 0 {
		timeoutSecs = d.defaultTimeoutSeconds
	}

	id := d.newID()
	now := d.now()

	rec := &state.CommandRecord{
		ID:             id,
		AgentID:        req.AgentID,
		Command:        req.Command,
		Args:           req.Args,
		Env:            req.Env,
		WorkingDir:     req.WorkingDir,
		User:           req.User,
		TimeoutSeconds: timeoutSecs,
		Status:         state.CommandStatusPending,
		StartedAt:      now,
	}
	if err := d.store.CreateCommand(ctx, rec); err != nil {
		return "", fmt.Errorf("controlplane: persist command: %w", err)
	}

	subject := d.subjects.AgentCommand(req.AgentID)
	msg := CommandMessage{
		MessageID:      id,
		Principal:      req.Principal,
		Command:        req.Command,
		Args:           req.Args,
		Env:            req.Env,
		WorkingDir:     req.WorkingDir,
		User:           req.User,
		TimeoutSeconds: timeoutSecs,
	}
	msg.Signature = d.signer.SignCommand(msg)
	payload, err := json.Marshal(msg)
	if err != nil {
		// JSON marshal cannot fail for these scalar/string types,
		// but if it ever did the command must not stay pending.
		_ = d.store.UpdateCommandResult(ctx, id, state.CommandResult{
			Status:      state.CommandStatusFailed,
			ExitCode:    -1,
			Stderr:      "dispatch: marshal: " + err.Error(),
			CompletedAt: d.now(),
		})
		return "", fmt.Errorf("controlplane: marshal command: %w", err)
	}

	// envelope.MessageID == command ID == inner CommandMessage.MessageID.
	// The agent's HMAC verify recomputes over the inner request with
	// req.MessageID = env.MessageID — without WithMessageID, envelope.New
	// stamps a random UUID and the verify always fails. Setting both
	// keeps the wire-level identifier unified with the logical command
	// ID (also a debugging win — one ID across logs / journal).
	env := envelope.New(payload, d.subjects.Prefix(),
		envelope.WithMessageID(id),
		envelope.WithCorrelationID(id),
	)
	if err := d.publisher.PublishEnvelope(ctx, subject, env); err != nil {
		_ = d.store.UpdateCommandResult(ctx, id, state.CommandResult{
			Status:      state.CommandStatusFailed,
			ExitCode:    -1,
			Stderr:      "dispatch: publish: " + err.Error(),
			CompletedAt: d.now(),
		})
		return "", fmt.Errorf("%w: publish: %v", ErrAgentUnreachable, err)
	}

	if err := d.store.UpdateCommandResult(ctx, id, state.CommandResult{
		Status: state.CommandStatusRunning,
	}); err != nil {
		// Persist-running failure is non-fatal — the command is in
		// flight on the agent. Log and proceed; the timeout watcher
		// is still tracking it.
		d.logger.Warn("controlplane: mark running failed",
			"command_id", id, "err", err)
	}

	deadline := d.now().Add(time.Duration(timeoutSecs) * time.Second)
	d.mu.Lock()
	d.inflight[id] = inFlightCommand{agentID: req.AgentID, deadline: deadline}
	d.mu.Unlock()

	d.logger.Debug("controlplane: command dispatched",
		"command_id", id, "agent_id", req.AgentID, "subject", subject,
		"timeout_seconds", timeoutSecs)
	return id, nil
}

// RecordResult finalizes the command in the store and clears its
// in-flight entry. Called by the NATS subscriber landing in Epic 05/06.
// Returns ErrCommandNotFound if the dispatcher is not tracking the ID
// and the store does not know it either; ErrCommandFinalized if the
// stored row is already terminal.
func (d *CommandDispatcher) RecordResult(ctx context.Context, id string, result state.CommandResult) error {
	if id == "" {
		return errors.New("controlplane: RecordResult requires a command ID")
	}
	if err := d.checkOpen(); err != nil {
		return err
	}

	cur, err := d.store.GetCommand(ctx, id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return ErrCommandNotFound
		}
		return fmt.Errorf("controlplane: lookup command %q: %w", id, err)
	}
	if isTerminal(cur.Status) {
		return ErrCommandFinalized
	}

	if result.CompletedAt.IsZero() {
		result.CompletedAt = d.now()
	}
	if err := d.store.UpdateCommandResult(ctx, id, result); err != nil {
		return fmt.Errorf("controlplane: update result for %q: %w", id, err)
	}

	d.mu.Lock()
	delete(d.inflight, id)
	d.mu.Unlock()
	return nil
}

// InFlight returns the count of currently-tracked commands. Used by
// the status ticker (later epic 04 task) and by tests.
func (d *CommandDispatcher) InFlight() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.inflight)
}

func (d *CommandDispatcher) checkOpen() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if !d.started {
		return ErrNotStarted
	}
	return nil
}

func (d *CommandDispatcher) runLoops(ctx context.Context) {
	defer close(d.loopsDone)
	timeoutT := time.NewTicker(d.timeoutCheckInterval)
	defer timeoutT.Stop()
	retentionT := time.NewTicker(d.retentionInterval)
	defer retentionT.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeoutT.C:
			d.sweepTimeouts(ctx)
		case <-retentionT.C:
			d.runRetention(ctx)
		}
	}
}

func (d *CommandDispatcher) sweepTimeouts(ctx context.Context) {
	now := d.now()

	d.mu.Lock()
	expired := make([]string, 0)
	for id, ent := range d.inflight {
		if !ent.deadline.IsZero() && now.After(ent.deadline) {
			expired = append(expired, id)
		}
	}
	d.mu.Unlock()

	for _, id := range expired {
		err := d.store.UpdateCommandResult(ctx, id, state.CommandResult{
			Status:      state.CommandStatusTimeout,
			ExitCode:    -1,
			Stderr:      "dispatch: agent did not reply within timeout",
			CompletedAt: now,
		})
		if err != nil {
			d.logger.Warn("controlplane: mark timeout failed",
				"command_id", id, "err", err)
			continue
		}
		d.mu.Lock()
		delete(d.inflight, id)
		d.mu.Unlock()
		d.logger.Info("controlplane: command timed out", "command_id", id)
	}
}

func (d *CommandDispatcher) runRetention(ctx context.Context) {
	cutoff := d.now().Add(-d.retentionWindow)
	n, err := d.store.DeleteCommandsBefore(ctx, cutoff, terminalCommandStatuses)
	if err != nil {
		d.logger.Warn("controlplane: retention sweep failed", "err", err)
		return
	}
	if n > 0 {
		d.logger.Info("controlplane: retention sweep", "deleted", n, "cutoff", cutoff)
	}
}

func isTerminal(s state.CommandStatus) bool {
	switch s {
	case state.CommandStatusCompleted,
		state.CommandStatusFailed,
		state.CommandStatusTimeout,
		state.CommandStatusCancelled:
		return true
	}
	return false
}

func uuidString() string { return uuid.NewString() }
