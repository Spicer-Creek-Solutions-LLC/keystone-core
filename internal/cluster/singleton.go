package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SingletonTask is work that must run on exactly one node — the
// leader. The SingletonTaskManager Starts it when this node gains
// leadership and Stops it when leadership is lost.
type SingletonTask interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// startStopTask adapts arbitrary start/stop funcs (e.g. an existing
// RetentionEnforcer) to SingletonTask. A nil func is a no-op.
type startStopTask struct {
	name  string
	start func(context.Context) error
	stop  func(context.Context) error
}

func (t startStopTask) Name() string { return t.name }
func (t startStopTask) Start(ctx context.Context) error {
	if t.start == nil {
		return nil
	}
	return t.start(ctx)
}
func (t startStopTask) Stop(ctx context.Context) error {
	if t.stop == nil {
		return nil
	}
	return t.stop(ctx)
}

// StartStopTask wraps a component exposing Start(ctx)/Stop(ctx) as a
// SingletonTask (the audit/events RetentionEnforcer, etc.).
func StartStopTask(name string, start, stop func(context.Context) error) SingletonTask {
	return startStopTask{name: name, start: start, stop: stop}
}

// loopTask runs fn every interval for as long as it is the leader
// (between Start and Stop) — the cleanup / metric-aggregation
// pattern.
type loopTask struct {
	name     string
	interval time.Duration
	fn       func(context.Context) error
	log      *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// LoopTask is a SingletonTask that invokes fn every interval while
// this node leads.
func LoopTask(name string, interval time.Duration, fn func(context.Context) error) SingletonTask {
	return &loopTask{name: name, interval: interval, fn: fn, log: slog.Default()}
}

func (l *loopTask) Name() string { return l.name }

func (l *loopTask) Start(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		return nil // already running
	}
	// Task lifetime is Start..Stop, independent of the caller's
	// ctx (gosec-G118-clean worker context, the cluster
	// convention).
	wctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.done = make(chan struct{})
	go l.run(wctx)
	return nil
}

func (l *loopTask) run(ctx context.Context) {
	defer close(l.done)
	t := time.NewTicker(l.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := l.fn(ctx); err != nil && ctx.Err() == nil {
				l.log.Warn("singleton loop task failed", "task", l.name, "err", err)
			}
		}
	}
}

func (l *loopTask) Stop(ctx context.Context) error {
	l.mu.Lock()
	if l.cancel == nil {
		l.mu.Unlock()
		return nil
	}
	cancel, done := l.cancel, l.done
	l.cancel = nil
	l.mu.Unlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// leadershipSource is the slice of LeaderElector the manager needs.
// *LeaderElector satisfies it.
type leadershipSource interface {
	AddObserver(LeadershipObserver)
	RemoveObserver(LeadershipObserver)
	IsLeader() bool
}

// SingletonTaskManagerConfig wires the manager.
type SingletonTaskManagerConfig struct {
	Leadership leadershipSource
	Logger     *slog.Logger
}

// SingletonTaskManager starts its registered tasks when this node
// becomes leader and stops them when leadership is lost, by
// observing the LeaderElector. It also exposes LeaderCheck — the
// single canonical func() bool that boot wiring passes into the
// ShardManager / FailoverManager / RetentionEnforcer leader-gate
// seams. Single-use lifecycle.
type SingletonTaskManager struct {
	cfg SingletonTaskManagerConfig
	log *slog.Logger

	mu      sync.Mutex
	state   lifecycle
	tasks   []SingletonTask
	running bool

	// txMu serialises leadership transitions so concurrent
	// gain/lose notifications can't interleave start/stop.
	txMu sync.Mutex
}

// NewSingletonTaskManager validates cfg and returns a manager in
// the created state.
func NewSingletonTaskManager(cfg SingletonTaskManagerConfig) (*SingletonTaskManager, error) {
	if cfg.Leadership == nil {
		return nil, fmt.Errorf("%w: Leadership source is required", ErrInvalidConfig)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &SingletonTaskManager{cfg: cfg, log: log, state: lcCreated}, nil
}

// Register adds a leader-only task. Must be called before Start.
func (m *SingletonTaskManager) Register(t SingletonTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != lcCreated {
		return fmt.Errorf("%w: register tasks before Start", ErrAlreadyStarted)
	}
	if t != nil {
		m.tasks = append(m.tasks, t)
	}
	return nil
}

// Start registers the leadership observer and syncs to the current
// leadership (starting tasks immediately if already leader).
func (m *SingletonTaskManager) Start(context.Context) error {
	m.mu.Lock()
	switch m.state {
	case lcStarted:
		m.mu.Unlock()
		return ErrAlreadyStarted
	case lcStopped:
		m.mu.Unlock()
		return ErrStopped
	}
	m.state = lcStarted
	m.mu.Unlock()

	m.cfg.Leadership.AddObserver(m)
	// Sync to whatever leadership already holds (we may have been
	// constructed after election).
	m.apply(m.cfg.Leadership.IsLeader())
	m.log.Info("singleton task manager started", "tasks", len(m.tasks))
	return nil
}

// OnLeadershipChange is the LeadershipObserver hook. Self is true
// only while this node holds leadership (LeaderElected).
func (m *SingletonTaskManager) OnLeadershipChange(ev LeadershipEvent) {
	m.apply(ev.Self)
}

// apply transitions running state. Idempotent + serialised so
// repeated Elected (or a gain/lose race) cannot double-start.
func (m *SingletonTaskManager) apply(leader bool) {
	m.txMu.Lock()
	defer m.txMu.Unlock()

	m.mu.Lock()
	if m.state != lcStarted || leader == m.running {
		m.mu.Unlock()
		return
	}
	m.running = leader
	tasks := append([]SingletonTask(nil), m.tasks...)
	m.mu.Unlock()

	if leader {
		m.startAll(tasks)
	} else {
		m.stopAll(tasks)
	}
}

func (m *SingletonTaskManager) startAll(tasks []SingletonTask) {
	for _, t := range tasks {
		if err := t.Start(context.Background()); err != nil {
			m.log.Warn("singleton task start failed", "task", t.Name(), "err", err)
			continue
		}
		m.log.Info("singleton task started", "task", t.Name())
	}
}

func (m *SingletonTaskManager) stopAll(tasks []SingletonTask) {
	for _, t := range tasks {
		if err := t.Stop(context.Background()); err != nil {
			m.log.Warn("singleton task stop failed", "task", t.Name(), "err", err)
			continue
		}
		m.log.Info("singleton task stopped", "task", t.Name())
	}
}

// LeaderCheck returns the canonical func() bool reporting whether
// this node is the leader. Boot wiring passes this into the
// ShardManager / FailoverManager / RetentionEnforcer leader-gate
// seams so "who is leader" flows through one place.
func (m *SingletonTaskManager) LeaderCheck() func() bool {
	return m.cfg.Leadership.IsLeader
}

// IsRunning reports whether the leader-only tasks are currently
// running on this node.
func (m *SingletonTaskManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Stop deregisters the observer and stops any running tasks.
// Idempotent.
func (m *SingletonTaskManager) Stop(context.Context) error {
	m.mu.Lock()
	if m.state != lcStarted {
		m.state = lcStopped
		m.mu.Unlock()
		return nil
	}
	m.state = lcStopped
	m.mu.Unlock()

	m.cfg.Leadership.RemoveObserver(m)

	m.txMu.Lock()
	m.mu.Lock()
	wasRunning := m.running
	m.running = false
	tasks := append([]SingletonTask(nil), m.tasks...)
	m.mu.Unlock()
	if wasRunning {
		m.stopAll(tasks)
	}
	m.txMu.Unlock()

	m.log.Info("singleton task manager stopped")
	return nil
}
