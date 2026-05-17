package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// LeaderState models the §4.15 leader machine:
//
//	no leader → CAMPAIGNING → ELECTED → (TRANSFERRED|LOST|RESIGNED) → CAMPAIGNING
//
// Resigned/Transferred are voluntary; Lost is the session lease
// dying (crash/partition). All three loop back to Campaigning.
type LeaderState string

const (
	LeaderUnknown     LeaderState = "unknown"
	LeaderCampaigning LeaderState = "campaigning"
	LeaderElected     LeaderState = "leader"
	LeaderResigned    LeaderState = "resigned"
	LeaderTransferred LeaderState = "transferred"
	LeaderLost        LeaderState = "lost"
)

// LeadershipEvent is delivered to observers on every transition.
// Self is true only while this node holds leadership.
type LeadershipEvent struct {
	State    LeaderState
	LeaderID string
	Self     bool
}

// LeadershipObserver receives leadership transitions. Implementations
// must not block — dispatch is synchronous, per the MembershipObserver
// / RunObserver convention.
type LeadershipObserver interface {
	OnLeadershipChange(LeadershipEvent)
}

// ElectionConfig is the runtime config for LeaderElector (the
// internal/cluster equivalent of config.ClusterElectionConfig plus
// identity + the EtcdClient handle; boot wiring maps them).
type ElectionConfig struct {
	Etcd *EtcdClient

	// MemberID is proclaimed as the leader's value (so peers learn
	// who leads via LeaderID). Should match the membership ID.
	MemberID string

	// KeyPrefix is the election keyspace (e.g. /kscore/cluster/leader).
	KeyPrefix string

	// SessionTTL bounds failover: a dead leader's lock expires
	// within ~this long. Separate from the membership lease.
	SessionTTL time.Duration

	// ReCampaignDelay is the pause TransferLeadership takes after
	// resigning before re-campaigning, so a peer takes over.
	ReCampaignDelay time.Duration

	Logger *slog.Logger
}

func (c *ElectionConfig) fillDefaults() {
	if c.SessionTTL <= 0 {
		c.SessionTTL = 3 * time.Second
	}
	if c.ReCampaignDelay < 0 {
		c.ReCampaignDelay = 0
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *ElectionConfig) validate() error {
	if c.Etcd == nil {
		return fmt.Errorf("%w: Etcd client is required", ErrInvalidConfig)
	}
	if c.MemberID == "" {
		return fmt.Errorf("%w: MemberID is required", ErrInvalidConfig)
	}
	if c.KeyPrefix == "" {
		return fmt.Errorf("%w: KeyPrefix is required", ErrInvalidConfig)
	}
	return nil
}

type electionCmdKind int

const (
	cmdResign electionCmdKind = iota
	cmdTransfer
)

type electionCmd struct {
	kind  electionCmdKind
	reply chan error
}

// LeaderElector runs a single etcd concurrency.Election campaign
// loop, exposing leadership state + voluntary resign/transfer.
// Single-use lifecycle, mirroring EtcdClient/MembershipManager.
//
// IsLeader is exactly the func() bool the WithRetentionLeaderCheck
// seam in internal/audit + internal/events expects; wiring that in
// (replacing the AlwaysLeader stub) is the SingletonTaskManager's
// job (Task 9), not this task's.
type LeaderElector struct {
	cfg ElectionConfig
	log *slog.Logger

	mu     sync.Mutex
	state  lifecycle
	lstate LeaderState
	elec   *concurrency.Election

	workerCtx    context.Context
	workerCancel context.CancelFunc
	cmdCh        chan electionCmd
	doneCh       chan struct{}

	obsMu     sync.RWMutex
	observers []LeadershipObserver
}

// NewLeaderElector validates cfg and returns an elector in the
// created (not yet started) state.
func NewLeaderElector(cfg ElectionConfig) (*LeaderElector, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &LeaderElector{cfg: cfg, log: cfg.Logger, state: lcCreated, lstate: LeaderUnknown}, nil
}

func (l *LeaderElector) ttlSeconds() int {
	s := int(l.cfg.SessionTTL / time.Second)
	if s < 1 {
		s = 1
	}
	return s
}

func (l *LeaderElector) newSession(cli *clientv3.Client) (*concurrency.Session, error) {
	return concurrency.NewSession(cli,
		concurrency.WithTTL(l.ttlSeconds()),
		concurrency.WithContext(l.workerCtx))
}

// Start opens the etcd session + election and launches the campaign
// loop. A second call returns ErrAlreadyStarted / ErrStopped.
func (l *LeaderElector) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch l.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}

	cli, err := l.cfg.Etcd.Client()
	if err != nil {
		return err
	}
	l.workerCtx, l.workerCancel = context.WithCancel(context.Background())
	sess, err := l.newSession(cli)
	if err != nil {
		l.workerCancel()
		return fmt.Errorf("%w: election session: %v", ErrEtcdUnavailable, err)
	}

	l.cmdCh = make(chan electionCmd)
	l.doneCh = make(chan struct{})
	l.state = lcStarted
	l.lstate = LeaderCampaigning
	go l.run(cli, sess)
	l.log.Info("leader elector started", "member", l.cfg.MemberID)
	return nil
}

func (l *LeaderElector) run(cli *clientv3.Client, sess *concurrency.Session) {
	defer close(l.doneCh)
	for {
		if l.workerCtx.Err() != nil {
			l.closeSession(sess)
			return
		}
		elec := concurrency.NewElection(sess, l.cfg.KeyPrefix)
		l.setElection(elec)
		l.transition(LeaderCampaigning, "")

		if err := elec.Campaign(l.workerCtx, l.cfg.MemberID); err != nil {
			if l.workerCtx.Err() != nil {
				l.closeSession(sess)
				return
			}
			l.log.Warn("campaign failed; recreating session", "err", err)
			l.transition(LeaderLost, "")
			ns := l.recreateSession(cli, sess)
			if ns == nil {
				return
			}
			sess = ns
			continue
		}

		l.transition(LeaderElected, l.cfg.MemberID)

		select {
		case <-l.workerCtx.Done():
			_ = l.resign(elec) // best-effort on shutdown
			l.closeSession(sess)
			return
		case <-sess.Done():
			l.transition(LeaderLost, "")
			ns := l.recreateSession(cli, sess)
			if ns == nil {
				return
			}
			sess = ns
		case cmd := <-l.cmdCh:
			err := l.resign(elec)
			if cmd.kind == cmdTransfer {
				l.transition(LeaderTransferred, "")
			} else {
				l.transition(LeaderResigned, "")
			}
			cmd.reply <- err
			if cmd.kind == cmdTransfer && l.cfg.ReCampaignDelay > 0 {
				select {
				case <-time.After(l.cfg.ReCampaignDelay):
				case <-l.workerCtx.Done():
					l.closeSession(sess)
					return
				}
			}
		}
	}
}

// recreateSession closes the dead session and opens a fresh one,
// backing off until success or worker-ctx cancellation (→ nil).
func (l *LeaderElector) recreateSession(cli *clientv3.Client, old *concurrency.Session) *concurrency.Session {
	l.closeSession(old)
	for {
		if l.workerCtx.Err() != nil {
			return nil
		}
		s, err := l.newSession(cli)
		if err == nil {
			return s
		}
		l.log.Warn("election session recreate failed; retrying", "err", err)
		select {
		case <-time.After(time.Second):
		case <-l.workerCtx.Done():
			return nil
		}
	}
}

// resign relinquishes leadership; ErrElectionNotLeader is benign
// (already not leading).
func (l *LeaderElector) resign(elec *concurrency.Election) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := elec.Resign(ctx); err != nil && !errors.Is(err, concurrency.ErrElectionNotLeader) {
		l.log.Warn("election resign failed", "err", err)
		return err
	}
	return nil
}

func (l *LeaderElector) closeSession(s *concurrency.Session) {
	if s == nil {
		return
	}
	if err := s.Close(); err != nil {
		l.log.Warn("election session close failed", "err", err)
	}
}

func (l *LeaderElector) setElection(e *concurrency.Election) {
	l.mu.Lock()
	l.elec = e
	l.mu.Unlock()
}

func (l *LeaderElector) transition(state LeaderState, leaderID string) {
	l.mu.Lock()
	l.lstate = state
	l.mu.Unlock()
	l.dispatch(LeadershipEvent{
		State:    state,
		LeaderID: leaderID,
		Self:     state == LeaderElected,
	})
}

func (l *LeaderElector) dispatch(ev LeadershipEvent) {
	l.obsMu.RLock()
	obs := make([]LeadershipObserver, len(l.observers))
	copy(obs, l.observers)
	l.obsMu.RUnlock()
	for _, o := range obs {
		o.OnLeadershipChange(ev)
	}
}

// AddObserver registers o for subsequent leadership transitions.
func (l *LeaderElector) AddObserver(o LeadershipObserver) {
	if o == nil {
		return
	}
	l.obsMu.Lock()
	l.observers = append(l.observers, o)
	l.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (l *LeaderElector) RemoveObserver(o LeadershipObserver) {
	l.obsMu.Lock()
	defer l.obsMu.Unlock()
	for i, x := range l.observers {
		if x == o {
			l.observers = append(l.observers[:i], l.observers[i+1:]...)
			return
		}
	}
}

// IsLeader reports whether this node currently holds leadership.
func (l *LeaderElector) IsLeader() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state == lcStarted && l.lstate == LeaderElected
}

// State returns the current leadership state.
func (l *LeaderElector) State() LeaderState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != lcStarted {
		return LeaderUnknown
	}
	return l.lstate
}

// LeaderID returns the current leader's proclaimed ID, or ErrNoLeader
// if the election has no leader.
func (l *LeaderElector) LeaderID(ctx context.Context) (string, error) {
	l.mu.Lock()
	started := l.state == lcStarted
	elec := l.elec
	l.mu.Unlock()
	if !started {
		return "", ErrNotStarted
	}
	if elec == nil {
		return "", ErrNoLeader
	}
	resp, err := elec.Leader(ctx)
	if err != nil {
		if errors.Is(err, concurrency.ErrElectionNoLeader) {
			return "", ErrNoLeader
		}
		return "", translateError(err)
	}
	if len(resp.Kvs) == 0 {
		return "", ErrNoLeader
	}
	return string(resp.Kvs[0].Value), nil
}

func (l *LeaderElector) signal(ctx context.Context, kind electionCmdKind) error {
	l.mu.Lock()
	started := l.state == lcStarted
	isLeader := l.lstate == LeaderElected
	cmdCh := l.cmdCh
	doneCh := l.doneCh
	l.mu.Unlock()

	if !started {
		return ErrNotStarted
	}
	if !isLeader {
		return nil // nothing to resign — no-op
	}

	reply := make(chan error, 1)
	select {
	case cmdCh <- electionCmd{kind: kind, reply: reply}:
	case <-ctx.Done():
		return ctx.Err()
	case <-doneCh:
		return ErrStopped
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Resign voluntarily steps down (if leading) and re-enters the
// campaign queue. No-op if not currently leader.
func (l *LeaderElector) Resign(ctx context.Context) error {
	return l.signal(ctx, cmdResign)
}

// TransferLeadership resigns and pauses (ReCampaignDelay) before
// re-campaigning so a peer reliably takes over. Targeted
// transfer-to-a-specific-member is coordinated by the
// ClusterService.TransferLeader RPC (Task 15), not here.
func (l *LeaderElector) TransferLeadership(ctx context.Context) error {
	return l.signal(ctx, cmdTransfer)
}

// Stop resigns (if leading), tears the session down, and ends the
// campaign loop. Idempotent: never-started / already-stopped → nil.
func (l *LeaderElector) Stop(ctx context.Context) error {
	l.mu.Lock()
	if l.state != lcStarted {
		l.state = lcStopped
		l.mu.Unlock()
		return nil
	}
	cancel := l.workerCancel
	doneCh := l.doneCh
	l.state = lcStopped
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	l.log.Info("leader elector stopped", "member", l.cfg.MemberID)
	return nil
}
