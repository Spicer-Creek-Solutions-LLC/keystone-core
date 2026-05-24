// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// Defaults match PROJECT-DETAILS §4.4: 10s monitor tick, 3 missed
// heartbeats before an agent is marked stale (≈ 30s window).
const (
	DefaultHeartbeatInterval = 10 * time.Second
	DefaultStaleThreshold    = 3
)

// Config configures a ConnectionManager. Store is required; everything
// else has a default.
type Config struct {
	Store             state.AgentStore
	Logger            *slog.Logger
	HeartbeatInterval time.Duration
	StaleThreshold    int
	Clock             func() time.Time
}

// Counts is a snapshot of agent population by status, used by the
// /api/status endpoint and the 30s status-ticker (Epic 04 task 7).
type Counts struct {
	Total     int
	Pending   int
	Connected int
	Stale     int
	Disabled  int
}

// ConnectionManager owns the agent registry: an in-memory cache layered
// over state.AgentStore plus a background goroutine that transitions
// agents to "stale" once they miss the configured number of heartbeats.
//
// Lifecycle: New → Start → (Register/Heartbeat/...)* → Stop. Methods
// invoked after Stop return ErrClosed; methods that depend on the
// monitor loop return ErrNotStarted before Start.
type ConnectionManager struct {
	store    state.AgentStore
	logger   *slog.Logger
	interval time.Duration
	threshold int
	staleAfter time.Duration
	now      func() time.Time

	mu    sync.RWMutex
	cache map[string]*state.AgentRecord

	startOnce sync.Once
	stopOnce  sync.Once
	started   bool
	closed    bool

	cancel context.CancelFunc
	done   chan struct{}
}

// New validates cfg, fills defaults, and returns a manager that has
// not yet been started. Call Start before any registration traffic.
func New(cfg Config) (*ConnectionManager, error) {
	if cfg.Store == nil {
		return nil, errors.New("controlplane: Store is required")
	}
	if cfg.HeartbeatInterval < 0 {
		return nil, fmt.Errorf("controlplane: HeartbeatInterval must be >= 0, got %s", cfg.HeartbeatInterval)
	}
	if cfg.StaleThreshold < 0 {
		return nil, fmt.Errorf("controlplane: StaleThreshold must be >= 0, got %d", cfg.StaleThreshold)
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = DefaultStaleThreshold
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &ConnectionManager{
		store:      cfg.Store,
		logger:     cfg.Logger,
		interval:   cfg.HeartbeatInterval,
		threshold:  cfg.StaleThreshold,
		staleAfter: cfg.HeartbeatInterval * time.Duration(cfg.StaleThreshold),
		now:        cfg.Clock,
		cache:      make(map[string]*state.AgentRecord),
		done:       make(chan struct{}),
	}, nil
}

// Start hydrates the cache from the store and launches the heartbeat
// monitor. It is safe to call once; subsequent calls are no-ops.
func (m *ConnectionManager) Start(ctx context.Context) error {
	var startErr error
	m.startOnce.Do(func() {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			startErr = ErrClosed
			return
		}
		m.mu.Unlock()

		agents, err := m.store.ListAgents(ctx, state.AgentFilter{})
		if err != nil {
			startErr = fmt.Errorf("controlplane: hydrate cache: %w", err)
			return
		}
		m.mu.Lock()
		for _, a := range agents {
			m.cache[a.ID] = cloneAgent(a)
		}
		m.started = true
		monitorCtx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.mu.Unlock()

		go m.runMonitor(monitorCtx)
		m.logger.Info("controlplane: connection manager started",
			"agents", len(agents),
			"heartbeat_interval", m.interval,
			"stale_threshold", m.threshold,
		)
	})
	return startErr
}

// Stop signals the monitor goroutine to exit and waits for it, bounded
// by ctx. After Stop, all mutating methods return ErrClosed.
func (m *ConnectionManager) Stop(ctx context.Context) error {
	var stopErr error
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		cancel := m.cancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if !m.startedLocked() {
			// Never started; no goroutine to wait on.
			return
		}
		select {
		case <-m.done:
		case <-ctx.Done():
			stopErr = fmt.Errorf("controlplane: stop: %w", ctx.Err())
		}
	})
	return stopErr
}

func (m *ConnectionManager) startedLocked() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// Register upserts the agent into the store and the cache. The supplied
// record's RegisteredAt and LastHeartbeatAt fields are normalized to
// "now" if zero, and Status is forced to "connected" — registration
// implies the agent is currently connected.
func (m *ConnectionManager) Register(ctx context.Context, a *state.AgentRecord) error {
	if a == nil || a.ID == "" {
		return errors.New("controlplane: Register requires a non-nil agent with ID")
	}
	if err := m.checkOpen(); err != nil {
		return err
	}

	now := m.now()
	rec := cloneAgent(a)
	if rec.RegisteredAt.IsZero() {
		rec.RegisteredAt = now
	}
	rec.LastHeartbeatAt = now
	rec.Status = state.AgentStatusConnected

	existing, err := m.store.GetAgent(ctx, rec.ID)
	switch {
	case errors.Is(err, state.ErrNotFound):
		if err := m.store.CreateAgent(ctx, rec); err != nil {
			return fmt.Errorf("controlplane: create agent %q: %w", rec.ID, err)
		}
	case err != nil:
		return fmt.Errorf("controlplane: lookup agent %q: %w", rec.ID, err)
	default:
		// Preserve the original RegisteredAt so re-register doesn't
		// silently rewrite registration history.
		rec.RegisteredAt = existing.RegisteredAt
		if err := m.store.UpdateAgent(ctx, rec); err != nil {
			return fmt.Errorf("controlplane: update agent %q: %w", rec.ID, err)
		}
	}

	m.mu.Lock()
	m.cache[rec.ID] = cloneAgent(rec)
	m.mu.Unlock()
	return nil
}

// Heartbeat refreshes LastHeartbeatAt for the named agent. A stale
// agent transitions back to connected on receipt; a disabled agent is
// rejected with ErrAgentDisabled. Unknown agents return ErrNotRegistered.
func (m *ConnectionManager) Heartbeat(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("controlplane: Heartbeat requires an agent ID")
	}
	if err := m.checkOpen(); err != nil {
		return err
	}

	now := m.now()

	m.mu.RLock()
	cur, ok := m.cache[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotRegistered
	}
	if cur.Status == state.AgentStatusDisabled {
		return ErrAgentDisabled
	}

	if err := m.store.UpdateAgentHeartbeat(ctx, id, now); err != nil {
		return fmt.Errorf("controlplane: update heartbeat for %q: %w", id, err)
	}

	transition := cur.Status == state.AgentStatusStale
	if transition {
		if err := m.store.UpdateAgentStatus(ctx, id, state.AgentStatusConnected); err != nil {
			return fmt.Errorf("controlplane: revive agent %q: %w", id, err)
		}
	}

	m.mu.Lock()
	if rec, ok := m.cache[id]; ok {
		rec.LastHeartbeatAt = now
		if transition {
			rec.Status = state.AgentStatusConnected
		}
	}
	m.mu.Unlock()

	if transition {
		m.logger.Info("controlplane: agent recovered from stale", "agent_id", id)
	}
	return nil
}

// Disable transitions the agent to AgentStatusDisabled in the store and
// cache. The monitor loop never auto-stales a disabled agent.
func (m *ConnectionManager) Disable(ctx context.Context, id string) error {
	return m.transition(ctx, id, state.AgentStatusDisabled)
}

// Delete removes the agent from the store and cache. Cascade rules are
// owned by the store layer (see schema.go).
func (m *ConnectionManager) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("controlplane: Delete requires an agent ID")
	}
	if err := m.checkOpen(); err != nil {
		return err
	}
	if err := m.store.DeleteAgent(ctx, id); err != nil {
		return fmt.Errorf("controlplane: delete agent %q: %w", id, err)
	}
	m.mu.Lock()
	delete(m.cache, id)
	m.mu.Unlock()
	return nil
}

func (m *ConnectionManager) transition(ctx context.Context, id string, target state.AgentStatus) error {
	if id == "" {
		return errors.New("controlplane: agent ID is required")
	}
	if err := m.checkOpen(); err != nil {
		return err
	}
	m.mu.RLock()
	_, ok := m.cache[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotRegistered
	}
	if err := m.store.UpdateAgentStatus(ctx, id, target); err != nil {
		return fmt.Errorf("controlplane: set agent %q status %q: %w", id, target, err)
	}
	m.mu.Lock()
	if rec, ok := m.cache[id]; ok {
		rec.Status = target
	}
	m.mu.Unlock()
	return nil
}

// Get returns a clone of the cached AgentRecord. Falls through to the
// store on cache miss so post-restart lookups still resolve before the
// cache is fully warmed.
func (m *ConnectionManager) Get(ctx context.Context, id string) (*state.AgentRecord, error) {
	m.mu.RLock()
	rec, ok := m.cache[id]
	var clone *state.AgentRecord
	if ok {
		clone = cloneAgent(rec)
	}
	m.mu.RUnlock()
	if clone != nil {
		return clone, nil
	}
	from, err := m.store.GetAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	return from, nil
}

// List returns a snapshot of cached agents matching filter. Sorting,
// pagination, and label matching live in the store; this method is for
// hot-path lookups (status ticker, gRPC ListAgents fast path).
//
// Only Status / LabelKey+LabelValue / Limit / Offset are honored. The
// caller should fall back to AgentStore.ListAgents for full-text search.
func (m *ConnectionManager) List(filter state.AgentFilter) []*state.AgentRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*state.AgentRecord, 0, len(m.cache))
	for _, rec := range m.cache {
		if filter.Status != "" && rec.Status != filter.Status {
			continue
		}
		if filter.LabelKey != "" {
			v, ok := rec.Labels[filter.LabelKey]
			if !ok {
				continue
			}
			if filter.LabelValue != "" && v != filter.LabelValue {
				continue
			}
		}
		out = append(out, cloneAgent(rec))
	}
	if filter.Offset > 0 {
		if filter.Offset >= len(out) {
			return nil
		}
		out = out[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(out) {
		out = out[:filter.Limit]
	}
	return out
}

// Counts returns a snapshot of cache population by status.
func (m *ConnectionManager) Counts() Counts {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := Counts{Total: len(m.cache)}
	for _, rec := range m.cache {
		switch rec.Status {
		case state.AgentStatusPending:
			c.Pending++
		case state.AgentStatusConnected:
			c.Connected++
		case state.AgentStatusStale:
			c.Stale++
		case state.AgentStatusDisabled:
			c.Disabled++
		}
	}
	return c
}

func (m *ConnectionManager) checkOpen() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrClosed
	}
	if !m.started {
		return ErrNotStarted
	}
	return nil
}

// runMonitor ticks every HeartbeatInterval and marks any connected
// agent whose last heartbeat is older than staleAfter as stale. It
// exits when ctx is cancelled (via Stop) and signals via m.done.
func (m *ConnectionManager) runMonitor(ctx context.Context) {
	defer close(m.done)
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweep(ctx)
		}
	}
}

func (m *ConnectionManager) sweep(ctx context.Context) {
	now := m.now()
	cutoff := now.Add(-m.staleAfter)

	// Snapshot under read lock so the sweep doesn't hold the lock
	// across DB calls.
	m.mu.RLock()
	candidates := make([]string, 0)
	for id, rec := range m.cache {
		if rec.Status != state.AgentStatusConnected {
			continue
		}
		if rec.LastHeartbeatAt.Before(cutoff) {
			candidates = append(candidates, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range candidates {
		if err := m.store.UpdateAgentStatus(ctx, id, state.AgentStatusStale); err != nil {
			m.logger.Warn("controlplane: mark stale failed",
				"agent_id", id, "err", err)
			continue
		}
		m.mu.Lock()
		if rec, ok := m.cache[id]; ok && rec.Status == state.AgentStatusConnected {
			rec.Status = state.AgentStatusStale
		}
		m.mu.Unlock()
		m.logger.Info("controlplane: agent marked stale", "agent_id", id)
	}
}

// cloneAgent returns a deep-enough copy of a so callers can mutate the
// returned value without racing the cache.
func cloneAgent(a *state.AgentRecord) *state.AgentRecord {
	if a == nil {
		return nil
	}
	out := *a
	if a.IPAddresses != nil {
		out.IPAddresses = append([]string(nil), a.IPAddresses...)
	}
	if a.Labels != nil {
		out.Labels = make(map[string]string, len(a.Labels))
		for k, v := range a.Labels {
			out.Labels[k] = v
		}
	}
	if a.Metrics != nil {
		out.Metrics = make(map[string]any, len(a.Metrics))
		for k, v := range a.Metrics {
			out.Metrics[k] = v
		}
	}
	return &out
}
