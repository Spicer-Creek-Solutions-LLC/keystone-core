// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// FenceMode controls how hard a fenced node (minority partition or
// stale epoch) blocks operations.
type FenceMode string

const (
	// FenceStrict blocks every operation (reads and writes).
	FenceStrict FenceMode = "strict"
	// FenceReadOnly allows reads, blocks writes. §4.15 acceptance
	// ("minority blocks writes, reads continue") — the default.
	FenceReadOnly FenceMode = "read_only"
	// FenceGraceful blocks new operations but lets in-flight ones
	// finish (see Drain).
	FenceGraceful FenceMode = "graceful"
)

// ParseFenceMode converts the config string to a FenceMode.
func ParseFenceMode(s string) (FenceMode, error) {
	switch FenceMode(s) {
	case FenceStrict, FenceReadOnly, FenceGraceful:
		return FenceMode(s), nil
	default:
		return "", fmt.Errorf("%w: unknown fence mode %q", ErrInvalidConfig, s)
	}
}

// OpType classifies an operation for fencing.
type OpType int

const (
	OpRead OpType = iota
	OpWrite
)

type fencingQuorum interface {
	AddObserver(HealthObserver)
	RemoveObserver(HealthObserver)
	Quorum() QuorumState
}

type fencingLeadership interface {
	AddObserver(LeadershipObserver)
	RemoveObserver(LeadershipObserver)
}

type fencingEtcd interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Txn(ctx context.Context) (clientv3.Txn, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) (clientv3.WatchChan, error)
}

// FenceObserver is notified when the fenced state flips.
type FenceObserver interface {
	OnFence(fenced bool)
}

// FencingManagerConfig wires the manager.
type FencingManagerConfig struct {
	Quorum     fencingQuorum
	Leadership fencingLeadership
	Etcd       fencingEtcd
	KeyPrefix  string
	Mode       FenceMode
	Logger     *slog.Logger
}

func (c *FencingManagerConfig) fillDefaults() {
	if c.Mode == "" {
		c.Mode = FenceReadOnly
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *FencingManagerConfig) validate() error {
	if c.Quorum == nil {
		return fmt.Errorf("%w: Quorum is required", ErrInvalidConfig)
	}
	if c.Leadership == nil {
		return fmt.Errorf("%w: Leadership is required", ErrInvalidConfig)
	}
	if c.Etcd == nil {
		return fmt.Errorf("%w: Etcd is required", ErrInvalidConfig)
	}
	if c.KeyPrefix == "" {
		return fmt.Errorf("%w: KeyPrefix is required", ErrInvalidConfig)
	}
	if _, err := ParseFenceMode(string(c.Mode)); err != nil {
		return err
	}
	return nil
}

// FencingManager is the split-brain enforcement layer. HealthMonitor
// only *detects* QuorumMinority; FencingManager blocks operations
// (lease fencing via quorum-loss + epoch fencing of a deposed
// leader) per the configured mode. Single-use lifecycle.
type FencingManager struct {
	cfg FencingManagerConfig
	log *slog.Logger

	mu            sync.Mutex
	state         lifecycle
	fencedQuorum  bool
	fencedEpoch   bool
	fenced        bool
	believeLeader bool
	epoch         int64 // latest epoch observed in etcd
	myEpoch       int64 // epoch this node set when it last won
	inflight      int

	workerCtx    context.Context
	workerCancel context.CancelFunc
	doneCh       chan struct{}

	obsMu     sync.RWMutex
	observers []FenceObserver
}

// NewFencingManager validates cfg and returns a manager in the
// created state.
func NewFencingManager(cfg FencingManagerConfig) (*FencingManager, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &FencingManager{cfg: cfg, log: cfg.Logger, state: lcCreated}, nil
}

func (f *FencingManager) epochKey() string {
	return trimRightSlash(f.cfg.KeyPrefix) + "/fence/epoch"
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// Start reads the current epoch, syncs the fenced state from the
// quorum, registers the health + leadership observers, and starts
// the epoch watch.
func (f *FencingManager) Start(ctx context.Context) error {
	f.mu.Lock()
	switch f.state {
	case lcStarted:
		f.mu.Unlock()
		return ErrAlreadyStarted
	case lcStopped:
		f.mu.Unlock()
		return ErrStopped
	}
	f.state = lcStarted
	f.mu.Unlock()

	ep, err := f.readEpoch(ctx)
	if err != nil {
		f.mu.Lock()
		f.state = lcCreated
		f.mu.Unlock()
		return fmt.Errorf("%w: read epoch: %v", ErrEtcdUnavailable, err)
	}

	f.mu.Lock()
	f.epoch = ep
	f.fencedQuorum = f.cfg.Quorum.Quorum() == QuorumMinority
	f.recomputeLocked()
	f.workerCtx, f.workerCancel = context.WithCancel(context.Background())
	f.doneCh = make(chan struct{})
	f.mu.Unlock()

	f.cfg.Quorum.AddObserver(f)
	f.cfg.Leadership.AddObserver(f)
	go f.watchEpoch()
	f.log.Info("fencing manager started", "mode", f.cfg.Mode, "epoch", ep)
	return nil
}

func (f *FencingManager) readEpoch(ctx context.Context) (int64, error) {
	resp, err := f.cfg.Etcd.Get(ctx, f.epochKey())
	if err != nil {
		return 0, err
	}
	if len(resp.Kvs) == 0 {
		return 0, nil
	}
	return strconv.ParseInt(string(resp.Kvs[0].Value), 10, 64)
}

// OnHealthChange folds the quorum signal into the fenced state.
func (f *FencingManager) OnHealthChange(ev HealthEvent) {
	f.mu.Lock()
	f.fencedQuorum = ev.Quorum == QuorumMinority
	changed := f.recomputeLocked()
	fenced := f.fenced
	f.mu.Unlock()
	if changed {
		f.dispatch(fenced)
	}
}

// OnLeadershipChange bumps the epoch on election and clears the
// stale-epoch fence when this node stops believing it leads.
func (f *FencingManager) OnLeadershipChange(ev LeadershipEvent) {
	f.mu.Lock()
	f.believeLeader = ev.Self
	if !ev.Self {
		f.fencedEpoch = false
		changed := f.recomputeLocked()
		fenced := f.fenced
		f.mu.Unlock()
		if changed {
			f.dispatch(fenced)
		}
		return
	}
	f.mu.Unlock()

	// Won election → bump the epoch so any deposed leader fences.
	newEp, err := f.bumpEpoch(context.Background())
	if err != nil {
		f.log.Warn("fence epoch bump failed", "err", err)
		return
	}
	f.mu.Lock()
	f.myEpoch = newEp
	f.epoch = newEp
	f.fencedEpoch = false
	changed := f.recomputeLocked()
	fenced := f.fenced
	f.mu.Unlock()
	if changed {
		f.dispatch(fenced)
	}
}

func (f *FencingManager) bumpEpoch(ctx context.Context) (int64, error) {
	key := f.epochKey()
	for attempt := 0; attempt < 5; attempt++ {
		resp, err := f.cfg.Etcd.Get(ctx, key)
		if err != nil {
			return 0, err
		}
		var cur int64
		var cmp clientv3.Cmp
		if len(resp.Kvs) == 0 {
			cmp = clientv3.Compare(clientv3.CreateRevision(key), "=", 0)
		} else {
			cur, err = strconv.ParseInt(string(resp.Kvs[0].Value), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse epoch: %w", err)
			}
			cmp = clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)
		}
		next := cur + 1
		txn, err := f.cfg.Etcd.Txn(ctx)
		if err != nil {
			return 0, err
		}
		tr, err := txn.If(cmp).
			Then(clientv3.OpPut(key, strconv.FormatInt(next, 10))).
			Commit()
		if err != nil {
			return 0, err
		}
		if tr.Succeeded {
			return next, nil
		}
		// Lost the race; re-read and retry.
	}
	return 0, fmt.Errorf("epoch bump: too many CAS conflicts")
}

func (f *FencingManager) watchEpoch() {
	defer close(f.doneCh)
	wch, err := f.cfg.Etcd.Watch(f.workerCtx, f.epochKey())
	if err != nil {
		f.log.Warn("fence epoch watch unavailable", "err", err)
		return
	}
	for resp := range wch {
		if resp.Canceled {
			return
		}
		for _, ev := range resp.Events {
			if ev.Type != clientv3.EventTypePut {
				continue
			}
			v, perr := strconv.ParseInt(string(ev.Kv.Value), 10, 64)
			if perr != nil {
				continue
			}
			f.mu.Lock()
			f.epoch = v
			// A deposed leader: epoch advanced past the one we set
			// while we still believe we lead → self-fence.
			if f.believeLeader && v > f.myEpoch {
				f.fencedEpoch = true
			}
			changed := f.recomputeLocked()
			fenced := f.fenced
			f.mu.Unlock()
			if changed {
				f.dispatch(fenced)
			}
		}
	}
}

// recomputeLocked recomputes f.fenced; returns whether it flipped.
// Caller holds f.mu.
func (f *FencingManager) recomputeLocked() bool {
	now := f.fencedQuorum || f.fencedEpoch
	if now == f.fenced {
		return false
	}
	f.fenced = now
	f.log.Info("fence state changed", "fenced", now,
		"quorum", f.fencedQuorum, "epoch", f.fencedEpoch)
	return true
}

// Guard authorises an operation. On success it returns a release
// func the caller must invoke when the operation completes. When
// fenced, the configured mode decides what is blocked.
func (f *FencingManager) Guard(op OpType) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != lcStarted {
		return nil, ErrNotStarted
	}
	if f.fenced {
		switch f.cfg.Mode {
		case FenceReadOnly:
			if op == OpWrite {
				return nil, ErrFenced
			}
		default: // FenceStrict, FenceGraceful: block all new ops
			return nil, ErrFenced
		}
	}
	f.inflight++
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			f.inflight--
			f.mu.Unlock()
		})
	}, nil
}

// Drain waits until all in-flight guarded operations have released
// (the GRACEFUL shutdown contract). Returns ctx.Err() on timeout.
func (f *FencingManager) Drain(ctx context.Context) error {
	for {
		f.mu.Lock()
		n := f.inflight
		f.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CurrentEpoch returns the latest epoch this node has observed.
func (f *FencingManager) CurrentEpoch() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.epoch
}

// ValidEpoch reports whether an operation that captured epoch e at
// its start may still commit (the epoch has not advanced — no newer
// leader). Stale ⇒ the caller must abort.
func (f *FencingManager) ValidEpoch(e int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return e == f.epoch
}

// Fenced reports whether this node is currently fenced.
func (f *FencingManager) Fenced() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fenced
}

// Mode returns the configured fence mode.
func (f *FencingManager) Mode() FenceMode { return f.cfg.Mode }

func (f *FencingManager) dispatch(fenced bool) {
	f.obsMu.RLock()
	obs := make([]FenceObserver, len(f.observers))
	copy(obs, f.observers)
	f.obsMu.RUnlock()
	for _, o := range obs {
		o.OnFence(fenced)
	}
}

// AddObserver registers o for fence-state flips. o must be a
// comparable value (pointer type) for RemoveObserver.
func (f *FencingManager) AddObserver(o FenceObserver) {
	if o == nil {
		return
	}
	f.obsMu.Lock()
	f.observers = append(f.observers, o)
	f.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (f *FencingManager) RemoveObserver(o FenceObserver) {
	f.obsMu.Lock()
	defer f.obsMu.Unlock()
	for i, x := range f.observers {
		if x == o {
			f.observers = append(f.observers[:i], f.observers[i+1:]...)
			return
		}
	}
}

// Stop deregisters observers and ends the epoch watch. Idempotent.
func (f *FencingManager) Stop(ctx context.Context) error {
	f.mu.Lock()
	if f.state != lcStarted {
		f.state = lcStopped
		f.mu.Unlock()
		return nil
	}
	cancel := f.workerCancel
	doneCh := f.doneCh
	f.state = lcStopped
	f.mu.Unlock()

	f.cfg.Quorum.RemoveObserver(f)
	f.cfg.Leadership.RemoveObserver(f)
	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.log.Info("fencing manager stopped")
	return nil
}
