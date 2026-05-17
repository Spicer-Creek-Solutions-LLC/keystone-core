package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// MemberStatus is the member lifecycle state (PROJECT-DETAILS
// §4.15):
//
//	HEALTHY → DEGRADED → UNHEALTHY → LEAVING → removed
//	  ↑__________|        (heartbeat threshold)
//	   recover
//
// "removed" is not a status value — it is the absence of the member
// key (lease expiry or explicit delete). Task 2 owns only the
// HEALTHY (on register) and LEAVING (on graceful deregister) edges
// plus a validated setStatus seam; the DEGRADED/UNHEALTHY edges are
// driven by the HealthMonitor (Task 7).
type MemberStatus string

const (
	MemberHealthy   MemberStatus = "healthy"
	MemberDegraded  MemberStatus = "degraded"
	MemberUnhealthy MemberStatus = "unhealthy"
	MemberLeaving   MemberStatus = "leaving"
)

// memberTransitions is the allowed-edge set of the state machine.
// LEAVING is terminal (the node then disappears as its key is
// removed). recover = DEGRADED/UNHEALTHY → HEALTHY.
var memberTransitions = map[MemberStatus]map[MemberStatus]bool{
	MemberHealthy:   {MemberDegraded: true, MemberLeaving: true},
	MemberDegraded:  {MemberHealthy: true, MemberUnhealthy: true, MemberLeaving: true},
	MemberUnhealthy: {MemberHealthy: true, MemberDegraded: true, MemberLeaving: true},
	MemberLeaving:   {},
}

func canTransition(from, to MemberStatus) bool {
	if from == to {
		return true // idempotent refresh
	}
	return memberTransitions[from][to]
}

// Member is a cluster member record (the etcd value at
// <prefix>/members/<id>).
type Member struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Addr          string            `json:"addr"`
	Status        MemberStatus      `json:"status"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	LeaseID       int64             `json:"lease_id"`
}

// MemberEventType classifies a membership change.
type MemberEventType string

const (
	MemberJoined  MemberEventType = "joined"
	MemberUpdated MemberEventType = "updated"
	MemberLeft    MemberEventType = "left"
)

// MemberEvent is delivered to observers / WatchMembers consumers.
// For MemberLeft the Member is reconstructed from etcd's prev-kv
// (so observers still get the departing member's metadata).
type MemberEvent struct {
	Type   MemberEventType
	Member Member
}

// MembershipObserver receives membership changes. Implementations
// must not block — dispatch is synchronous in the watch loop, per
// the existing RunObserver / StateApplyObserver convention.
type MembershipObserver interface {
	OnMembershipChange(MemberEvent)
}

// MembershipConfig is the runtime config for MembershipManager (the
// internal/cluster-owned equivalent of config.ClusterMembershipConfig
// plus identity + the EtcdClient handle; boot wiring maps them).
type MembershipConfig struct {
	Etcd *EtcdClient

	// MemberName / Addr identify this node to peers.
	MemberName string
	Addr       string

	// MemberID, if empty, is resolved from MemberIDFile (read if
	// present, else a UUIDv7 is generated and persisted). A stable
	// ID across restarts is required for RecoveryManager (Task 10)
	// to reclaim this node's shards rather than orphan them.
	MemberID     string
	MemberIDFile string

	KeyPrefix         string
	HeartbeatInterval time.Duration
	LeaseTTL          time.Duration
	Metadata          map[string]string
	Logger            *slog.Logger
}

func (c *MembershipConfig) fillDefaults() {
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 5 * time.Second
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = defaultLeaseTTL
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "/kscore/cluster"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *MembershipConfig) validate() error {
	if c.Etcd == nil {
		return fmt.Errorf("%w: Etcd client is required", ErrInvalidConfig)
	}
	if c.MemberName == "" {
		return fmt.Errorf("%w: MemberName is required", ErrInvalidConfig)
	}
	if c.MemberID == "" && c.MemberIDFile == "" {
		return fmt.Errorf("%w: MemberID or MemberIDFile is required", ErrInvalidConfig)
	}
	// Anti-flap mirror of the config-layer guard, in case the
	// manager is constructed directly (tests / embedders).
	if c.LeaseTTL < 3*c.HeartbeatInterval {
		return fmt.Errorf("%w: LeaseTTL (%v) must be >= 3x HeartbeatInterval (%v)",
			ErrInvalidConfig, c.LeaseTTL, c.HeartbeatInterval)
	}
	return nil
}

// MembershipManager registers this node in etcd under an ephemeral
// lease, heartbeats its liveness, and streams membership changes to
// observers. Single-use lifecycle, mirroring EtcdClient.
type MembershipManager struct {
	cfg MembershipConfig
	log *slog.Logger

	mu      sync.Mutex
	state   lifecycle
	self    Member
	leaseID clientv3.LeaseID

	workerCtx    context.Context
	workerCancel context.CancelFunc

	obsMu     sync.RWMutex
	observers []MembershipObserver
}

// NewMembershipManager validates cfg and returns a manager in the
// created (not yet registered) state.
func NewMembershipManager(cfg MembershipConfig) (*MembershipManager, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &MembershipManager{cfg: cfg, log: cfg.Logger, state: lcCreated}, nil
}

func (m *MembershipManager) membersPrefix() string {
	return strings.TrimRight(m.cfg.KeyPrefix, "/") + "/members/"
}

func (m *MembershipManager) memberKey(id string) string {
	return m.membersPrefix() + id
}

// resolveMemberID returns the configured ID, or reads/creates the
// stable ID file. The generated ID is a UUIDv7 (k-sortable,
// time-prefixed) consistent with the rest of the codebase.
func (m *MembershipManager) resolveMemberID() (string, error) {
	if m.cfg.MemberID != "" {
		return m.cfg.MemberID, nil
	}
	if b, err := os.ReadFile(m.cfg.MemberIDFile); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read member id file: %w", err)
	}
	v7, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate member id: %w", err)
	}
	id := v7.String()
	if err := os.MkdirAll(filepath.Dir(m.cfg.MemberIDFile), 0o750); err != nil {
		return "", fmt.Errorf("create member id dir: %w", err)
	}
	if err := os.WriteFile(m.cfg.MemberIDFile, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist member id: %w", err)
	}
	return id, nil
}

// Register grants an ephemeral lease, keeps it alive, writes this
// node's member record under that lease, and starts the heartbeat
// loop and the membership watch fan-out. Idempotent-safe: a second
// call returns ErrAlreadyStarted / ErrStopped.
func (m *MembershipManager) Register(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}

	id, err := m.resolveMemberID()
	if err != nil {
		return err
	}
	leaseID, err := m.cfg.Etcd.GrantLease(ctx, m.cfg.LeaseTTL)
	if err != nil {
		return fmt.Errorf("grant member lease: %w", err)
	}
	if err := m.cfg.Etcd.KeepAlive(leaseID); err != nil {
		return fmt.Errorf("keepalive member lease: %w", err)
	}

	now := time.Now().UTC()
	m.self = Member{
		ID:            id,
		Name:          m.cfg.MemberName,
		Addr:          m.cfg.Addr,
		Status:        MemberHealthy,
		Metadata:      m.cfg.Metadata,
		StartedAt:     now,
		LastHeartbeat: now,
		LeaseID:       int64(leaseID),
	}
	m.leaseID = leaseID
	if err := m.putSelf(ctx); err != nil {
		return fmt.Errorf("write member record: %w", err)
	}

	m.workerCtx, m.workerCancel = context.WithCancel(context.Background())
	m.state = lcStarted
	go m.heartbeatLoop()
	go m.watchLoop()
	m.log.Info("cluster member registered", "id", id, "name", m.cfg.MemberName)
	return nil
}

// putSelf marshals and writes the current self record under the
// member lease. Caller holds m.mu (or is in a path that owns self).
func (m *MembershipManager) putSelf(ctx context.Context) error {
	b, err := json.Marshal(m.self)
	if err != nil {
		return err
	}
	return m.cfg.Etcd.Put(ctx, m.memberKey(m.self.ID), string(b),
		clientv3.WithLease(m.leaseID))
}

func (m *MembershipManager) heartbeatLoop() {
	t := time.NewTicker(m.cfg.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-m.workerCtx.Done():
			return
		case <-t.C:
			m.mu.Lock()
			if m.state != lcStarted {
				m.mu.Unlock()
				return
			}
			m.self.LastHeartbeat = time.Now().UTC()
			err := m.putSelf(m.workerCtx)
			m.mu.Unlock()
			if err != nil && m.workerCtx.Err() == nil {
				m.log.Warn("cluster heartbeat write failed", "err", err)
			}
		}
	}
}

func (m *MembershipManager) watchLoop() {
	wch, err := m.cfg.Etcd.Watch(m.workerCtx, m.membersPrefix(),
		clientv3.WithPrefix(), clientv3.WithPrevKV())
	if err != nil {
		m.log.Warn("cluster membership watch unavailable", "err", err)
		return
	}
	for resp := range wch {
		if resp.Canceled {
			return
		}
		for _, ev := range resp.Events {
			me, ok := toMemberEvent(ev)
			if !ok {
				continue
			}
			m.dispatch(me)
		}
	}
}

func toMemberEvent(ev *clientv3.Event) (MemberEvent, bool) {
	switch {
	case ev.Type == clientv3.EventTypeDelete:
		var mem Member
		if ev.PrevKv != nil && json.Unmarshal(ev.PrevKv.Value, &mem) == nil {
			return MemberEvent{Type: MemberLeft, Member: mem}, true
		}
		return MemberEvent{}, false
	case ev.IsCreate():
		var mem Member
		if json.Unmarshal(ev.Kv.Value, &mem) != nil {
			return MemberEvent{}, false
		}
		return MemberEvent{Type: MemberJoined, Member: mem}, true
	default: // PUT on an existing key
		var mem Member
		if json.Unmarshal(ev.Kv.Value, &mem) != nil {
			return MemberEvent{}, false
		}
		return MemberEvent{Type: MemberUpdated, Member: mem}, true
	}
}

func (m *MembershipManager) dispatch(ev MemberEvent) {
	m.obsMu.RLock()
	obs := make([]MembershipObserver, len(m.observers))
	copy(obs, m.observers)
	m.obsMu.RUnlock()
	for _, o := range obs {
		o.OnMembershipChange(ev)
	}
}

// AddObserver registers o for subsequent membership events. Callers
// that need the current snapshot should LoadMembers first, then add
// the observer, then reconcile.
func (m *MembershipManager) AddObserver(o MembershipObserver) {
	if o == nil {
		return
	}
	m.obsMu.Lock()
	m.observers = append(m.observers, o)
	m.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (m *MembershipManager) RemoveObserver(o MembershipObserver) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	for i, x := range m.observers {
		if x == o {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

// chanObserver adapts a channel to MembershipObserver for
// WatchMembers; it drops events rather than block the watch loop.
type chanObserver struct {
	ch  chan MemberEvent
	log *slog.Logger
}

func (c *chanObserver) OnMembershipChange(ev MemberEvent) {
	select {
	case c.ch <- ev:
	default:
		c.log.Warn("cluster membership watch consumer slow; event dropped",
			"type", ev.Type, "member", ev.Member.ID)
	}
}

// WatchMembers returns a channel of membership events for pull-style
// consumers. The subscription auto-removes and the channel closes
// when ctx is cancelled. Backed by the single shared etcd watch (no
// extra watch is opened per caller).
func (m *MembershipManager) WatchMembers(ctx context.Context) (<-chan MemberEvent, error) {
	if err := m.requireRegistered(); err != nil {
		return nil, err
	}
	co := &chanObserver{ch: make(chan MemberEvent, 64), log: m.log}
	m.AddObserver(co)
	go func() {
		<-ctx.Done()
		m.RemoveObserver(co)
		close(co.ch)
	}()
	return co.ch, nil
}

func (m *MembershipManager) requireRegistered() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != lcStarted {
		return ErrNotRegistered
	}
	return nil
}

// LoadMembers returns the current member set (sorted by ID for
// deterministic output). Malformed records are skipped + logged.
func (m *MembershipManager) LoadMembers(ctx context.Context) ([]Member, error) {
	if err := m.requireRegistered(); err != nil {
		return nil, err
	}
	resp, err := m.cfg.Etcd.Get(ctx, m.membersPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var mem Member
		if err := json.Unmarshal(kv.Value, &mem); err != nil {
			m.log.Warn("skipping malformed member record", "key", string(kv.Key), "err", err)
			continue
		}
		out = append(out, mem)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// GetMember returns one member by ID, or ErrMemberNotFound.
func (m *MembershipManager) GetMember(ctx context.Context, id string) (Member, error) {
	if err := m.requireRegistered(); err != nil {
		return Member{}, err
	}
	resp, err := m.cfg.Etcd.Get(ctx, m.memberKey(id))
	if err != nil {
		return Member{}, err
	}
	if len(resp.Kvs) == 0 {
		return Member{}, ErrMemberNotFound
	}
	var mem Member
	if err := json.Unmarshal(resp.Kvs[0].Value, &mem); err != nil {
		return Member{}, fmt.Errorf("decode member %q: %w", id, err)
	}
	return mem, nil
}

// SetStatus transitions this node's status (validated against the
// lifecycle state machine) and republishes the record. This is the
// seam the HealthMonitor (Task 7) drives for DEGRADED/UNHEALTHY;
// Task 2 itself only uses MemberHealthy/MemberLeaving.
func (m *MembershipManager) SetStatus(ctx context.Context, to MemberStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != lcStarted {
		return ErrNotRegistered
	}
	if !canTransition(m.self.Status, to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, m.self.Status, to)
	}
	m.self.Status = to
	return m.putSelf(ctx)
}

// Self returns a copy of this node's current member record.
func (m *MembershipManager) Self() Member {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.self
}

// Deregister gracefully leaves: mark LEAVING so peers observe the
// intent, then revoke the lease (immediate key removal rather than
// waiting out the TTL) and stop the background loops. Equivalent to
// Stop; kept as a named method for the graceful-shutdown sequence
// (Task 14).
func (m *MembershipManager) Deregister(ctx context.Context) error {
	return m.Stop(ctx)
}

// Stop is idempotent: a never-registered or already-stopped manager
// is a no-op returning nil.
func (m *MembershipManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.state != lcStarted {
		m.state = lcStopped
		m.mu.Unlock()
		return nil
	}

	// Best-effort LEAVING announcement (transition is always valid
	// from any live status).
	if canTransition(m.self.Status, MemberLeaving) {
		m.self.Status = MemberLeaving
		if err := m.putSelf(ctx); err != nil {
			m.log.Warn("cluster leaving announcement failed", "err", err)
		}
	}
	leaseID := m.leaseID
	cancel := m.workerCancel
	m.state = lcStopped
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	// Revoke removes the member key now; idempotent if already gone.
	if err := m.cfg.Etcd.RevokeLease(ctx, leaseID); err != nil {
		m.log.Warn("cluster member lease revoke failed", "err", err)
	}
	m.log.Info("cluster member deregistered", "id", m.self.ID)
	return nil
}
