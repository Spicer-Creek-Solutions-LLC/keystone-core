package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ShutdownPhase is the §4.15 graceful-shutdown machine:
//
//	RUNNING → INITIATED → DRAINING → TRANSFERRING →
//	DEREGISTERING → COMPLETED
//
// A failure in DEREGISTERING (the only step whose error is fatal —
// the member key must come out) ends in terminal FAILED.
type ShutdownPhase string

const (
	ShutdownRunning       ShutdownPhase = "running"
	ShutdownInitiated     ShutdownPhase = "initiated"
	ShutdownDraining      ShutdownPhase = "draining"
	ShutdownTransferring  ShutdownPhase = "transferring"
	ShutdownDeregistering ShutdownPhase = "deregistering"
	ShutdownCompleted     ShutdownPhase = "completed"
	ShutdownFailed        ShutdownPhase = "failed"
)

// ShutdownEvent is delivered to observers on every phase change.
type ShutdownEvent struct {
	Phase ShutdownPhase
	Err   error
}

// ShutdownObserver receives shutdown phase transitions. Must not
// block; must be comparable (pointer type) for RemoveObserver.
type ShutdownObserver interface {
	OnShutdown(ShutdownEvent)
}

type shutdownMembership interface {
	SetStatus(ctx context.Context, to MemberStatus) error
	Deregister(ctx context.Context) error
}

type shutdownLeadership interface {
	IsLeader() bool
	TransferLeadership(ctx context.Context) error
}

type shutdownDrainer interface {
	Drain(ctx context.Context) error
}

// GracefulShutdownConfig wires the orchestrator. Only Membership is
// required; the rest are optional so it works with partial wiring.
type GracefulShutdownConfig struct {
	Membership shutdownMembership
	Leadership shutdownLeadership
	Drainer    shutdownDrainer
	// StopAccepting stops taking new agents/connections at the
	// start of DRAINING (boot wires the connection manager /
	// listener drain). nil ⇒ skipped.
	StopAccepting func(ctx context.Context) error
	// Timeout bounds the DEREGISTERING phase. §4.15 = 30s.
	Timeout time.Duration
	Logger  *slog.Logger
}

func (c *GracefulShutdownConfig) fillDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *GracefulShutdownConfig) validate() error {
	if c.Membership == nil {
		return fmt.Errorf("%w: Membership is required", ErrInvalidConfig)
	}
	return nil
}

// GracefulShutdown runs the one-shot 5-phase shutdown sequence.
// DRAINING sets LEAVING so the ShardManager rebalances this node's
// agents onto peers *before* it exits (the "no agent
// disconnections" property); TRANSFERRING hands off leadership;
// DEREGISTERING drains in-flight work then removes the member key
// within the timeout.
//
// Wiring this into the kscore-server SIGTERM/Stop path (and the
// real StopAccepting) is boot integration — deferred (see the
// "Cluster gRPC services boot registration" ROADMAP entry).
type GracefulShutdown struct {
	cfg GracefulShutdownConfig
	log *slog.Logger

	mu    sync.Mutex
	phase ShutdownPhase
	ran   bool

	obsMu     sync.RWMutex
	observers []ShutdownObserver
}

// NewGracefulShutdown validates cfg and returns an orchestrator in
// the RUNNING phase.
func NewGracefulShutdown(cfg GracefulShutdownConfig) (*GracefulShutdown, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &GracefulShutdown{cfg: cfg, log: cfg.Logger, phase: ShutdownRunning}, nil
}

// Phase returns the current shutdown phase.
func (g *GracefulShutdown) Phase() ShutdownPhase {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.phase
}

func (g *GracefulShutdown) setPhase(p ShutdownPhase) {
	g.mu.Lock()
	g.phase = p
	g.mu.Unlock()
	g.dispatch(ShutdownEvent{Phase: p})
}

func (g *GracefulShutdown) fail(failed ShutdownPhase, err error) error {
	wrapped := fmt.Errorf("graceful shutdown failed in %s: %w", failed, err)
	g.mu.Lock()
	g.phase = ShutdownFailed
	g.mu.Unlock()
	g.dispatch(ShutdownEvent{Phase: ShutdownFailed, Err: wrapped})
	g.log.Warn("graceful shutdown failed", "phase", failed, "err", err)
	return wrapped
}

// Shutdown runs the sequence. Single-use: a second call returns
// ErrAlreadyStarted. Returns nil on COMPLETED, the wrapped error on
// FAILED.
func (g *GracefulShutdown) Shutdown(ctx context.Context) error {
	g.mu.Lock()
	if g.ran {
		g.mu.Unlock()
		return fmt.Errorf("%w: shutdown already run", ErrAlreadyStarted)
	}
	g.ran = true
	g.mu.Unlock()

	g.setPhase(ShutdownInitiated)

	// DRAINING: stop accepting new work, then mark LEAVING so the
	// ShardManager moves our agents to peers before we exit.
	g.setPhase(ShutdownDraining)
	if g.cfg.StopAccepting != nil {
		if err := g.cfg.StopAccepting(ctx); err != nil {
			g.log.Warn("graceful shutdown: stop-accepting failed", "err", err)
		}
	}
	if err := g.cfg.Membership.SetStatus(ctx, MemberLeaving); err != nil {
		g.log.Warn("graceful shutdown: set LEAVING failed", "err", err)
	}

	// TRANSFERRING: hand off leadership if we hold it.
	g.setPhase(ShutdownTransferring)
	if g.cfg.Leadership != nil && g.cfg.Leadership.IsLeader() {
		if err := g.cfg.Leadership.TransferLeadership(ctx); err != nil {
			g.log.Warn("graceful shutdown: leadership transfer failed", "err", err)
		}
	}

	// DEREGISTERING: drain in-flight (best-effort within budget),
	// then remove the member key (prioritised — a slow drain must
	// not prevent deregistration).
	g.setPhase(ShutdownDeregistering)
	if g.cfg.Drainer != nil {
		dctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
		err := g.cfg.Drainer.Drain(dctx)
		cancel()
		if err != nil {
			g.log.Warn("graceful shutdown: in-flight drain incomplete; deregistering anyway",
				"err", err)
		}
	}
	dgctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
	err := g.cfg.Membership.Deregister(dgctx)
	cancel()
	if err != nil {
		return g.fail(ShutdownDeregistering, fmt.Errorf("deregister: %w", err))
	}

	g.setPhase(ShutdownCompleted)
	g.log.Info("graceful shutdown completed")
	return nil
}

func (g *GracefulShutdown) dispatch(ev ShutdownEvent) {
	g.obsMu.RLock()
	obs := make([]ShutdownObserver, len(g.observers))
	copy(obs, g.observers)
	g.obsMu.RUnlock()
	for _, o := range obs {
		o.OnShutdown(ev)
	}
}

// AddObserver registers o for shutdown phase transitions. o must be
// a comparable value (pointer type) for RemoveObserver.
func (g *GracefulShutdown) AddObserver(o ShutdownObserver) {
	if o == nil {
		return
	}
	g.obsMu.Lock()
	g.observers = append(g.observers, o)
	g.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (g *GracefulShutdown) RemoveObserver(o ShutdownObserver) {
	g.obsMu.Lock()
	defer g.obsMu.Unlock()
	for i, x := range g.observers {
		if x == o {
			g.observers = append(g.observers[:i], g.observers[i+1:]...)
			return
		}
	}
}
