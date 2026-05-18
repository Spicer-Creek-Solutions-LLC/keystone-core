package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ErrCoordConfig is returned by NewCoordinationClient for an
// invalid config.
var ErrCoordConfig = errors.New("controlplane: invalid coordination client config")

// PeerObserver is notified when a peer's reachability flips.
// Implementations must not block; must be comparable (pointer
// type) for RemoveObserver.
type PeerObserver interface {
	OnPeerChange(memberID string, reachable bool)
}

// CoordinationClientConfig wires the client. DialOptions must carry
// the transport credentials (boot: mTLS from the identity
// provider; tests: a bufconn dialer + insecure).
type CoordinationClientConfig struct {
	SelfID      string
	DialOptions []grpc.DialOption

	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	FailureThreshold  int
	RetryMax          int
	RetryBaseDelay    time.Duration
	RetryMaxDelay     time.Duration
	Logger            *slog.Logger
}

func (c *CoordinationClientConfig) fillDefaults() {
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 5 * time.Second
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = 2 * time.Second
	}
	if c.FailureThreshold < 1 {
		c.FailureThreshold = 3
	}
	if c.RetryMax < 1 {
		c.RetryMax = 4
	}
	if c.RetryBaseDelay <= 0 {
		c.RetryBaseDelay = 100 * time.Millisecond
	}
	if c.RetryMaxDelay <= 0 {
		c.RetryMaxDelay = 2 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *CoordinationClientConfig) validate() error {
	if c.SelfID == "" {
		return fmt.Errorf("%w: SelfID is required", ErrCoordConfig)
	}
	if len(c.DialOptions) == 0 {
		return fmt.Errorf("%w: DialOptions (transport credentials) required", ErrCoordConfig)
	}
	return nil
}

type ccState int

const (
	ccCreated ccState = iota
	ccStarted
	ccStopped
)

type peerConn struct {
	addr   string
	conn   *grpc.ClientConn
	client v1.CoordinationServiceClient

	mu        sync.Mutex
	lastSeen  time.Time
	fails     int
	reachable bool
}

// CoordinationClient is the peer side of the Task 12 mTLS channel:
// a per-peer pooled gRPC client with NodeHeartbeat liveness
// tracking and retry + capped exponential backoff. Single-use
// lifecycle. Driving AddPeer/RemovePeer from MembershipManager +
// supplying real mTLS creds is boot wiring (deferred — see the
// "Cluster gRPC services boot registration" ROADMAP entry).
type CoordinationClient struct {
	cfg CoordinationClientConfig
	log *slog.Logger

	mu    sync.Mutex
	state ccState
	peers map[string]*peerConn

	workerCtx    context.Context
	workerCancel context.CancelFunc
	doneCh       chan struct{}

	obsMu     sync.RWMutex
	observers []PeerObserver
}

// NewCoordinationClient validates cfg and returns a client in the
// created state.
func NewCoordinationClient(cfg CoordinationClientConfig) (*CoordinationClient, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &CoordinationClient{
		cfg:   cfg,
		log:   cfg.Logger,
		state: ccCreated,
		peers: make(map[string]*peerConn),
	}, nil
}

// AddPeer adds (or re-points) a peer. The gRPC conn is lazy
// (grpc.NewClient connects on first RPC).
func (c *CoordinationClient) AddPeer(memberID, addr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pc, ok := c.peers[memberID]; ok {
		if pc.addr == addr {
			return nil
		}
		_ = pc.conn.Close()
		delete(c.peers, memberID)
	}
	conn, err := grpc.NewClient(addr, c.cfg.DialOptions...)
	if err != nil {
		return fmt.Errorf("dial peer %s (%s): %w", memberID, addr, err)
	}
	c.peers[memberID] = &peerConn{
		addr:      addr,
		conn:      conn,
		client:    v1.NewCoordinationServiceClient(conn),
		reachable: true, // optimistic until FailureThreshold misses
	}
	return nil
}

// RemovePeer closes and drops a peer.
func (c *CoordinationClient) RemovePeer(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pc, ok := c.peers[memberID]; ok {
		_ = pc.conn.Close()
		delete(c.peers, memberID)
	}
}

// SetPeers reconciles the pool to exactly the given memberID→addr
// map (adds new, re-points changed, removes absent).
func (c *CoordinationClient) SetPeers(want map[string]string) error {
	c.mu.Lock()
	for id, pc := range c.peers {
		if _, keep := want[id]; !keep {
			_ = pc.conn.Close()
			delete(c.peers, id)
		}
	}
	c.mu.Unlock()
	for id, addr := range want {
		if err := c.AddPeer(id, addr); err != nil {
			return err
		}
	}
	return nil
}

func (c *CoordinationClient) peer(memberID string) (*peerConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pc, ok := c.peers[memberID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "coordination: unknown peer %q", memberID)
	}
	return pc, nil
}

func retryable(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// withRetry runs fn with a per-attempt timeout, retrying transient
// failures with capped exponential backoff.
func (c *CoordinationClient) withRetry(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < c.cfg.RetryMax; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, c.cfg.HeartbeatTimeout)
		err = fn(cctx)
		cancel()
		if err == nil {
			return nil
		}
		if !retryable(err) || attempt == c.cfg.RetryMax-1 {
			return err
		}
		delay := c.cfg.RetryBaseDelay << attempt
		if delay > c.cfg.RetryMaxDelay || delay <= 0 {
			delay = c.cfg.RetryMaxDelay
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// --- RPC wrappers ------------------------------------------------------

func (c *CoordinationClient) ClusterHealth(ctx context.Context, memberID string) (*v1.ClusterHealthResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.ClusterHealthResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.ClusterHealth(cc, &v1.ClusterHealthRequest{FromNodeId: c.cfg.SelfID})
		resp = r
		return e
	})
	return resp, err
}

func (c *CoordinationClient) LookupLeader(ctx context.Context, memberID string) (*v1.LookupLeaderResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.LookupLeaderResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.LookupLeader(cc, &v1.LookupLeaderRequest{})
		resp = r
		return e
	})
	return resp, err
}

func (c *CoordinationClient) NATSStatus(ctx context.Context, memberID string) (*v1.NATSStatusResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.NATSStatusResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.NATSStatus(cc, &v1.NATSStatusRequest{})
		resp = r
		return e
	})
	return resp, err
}

func (c *CoordinationClient) RecoveryCoordinate(ctx context.Context, memberID string, phase v1.RecoveryPhase) (*v1.RecoveryCoordinateResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.RecoveryCoordinateResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.RecoveryCoordinate(cc, &v1.RecoveryCoordinateRequest{
			FromNodeId: c.cfg.SelfID, Phase: phase,
		})
		resp = r
		return e
	})
	return resp, err
}

func (c *CoordinationClient) NodeHeartbeat(ctx context.Context, memberID string, epoch int64) (*v1.NodeHeartbeatResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.NodeHeartbeatResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.NodeHeartbeat(cc, &v1.NodeHeartbeatRequest{
			FromNodeId: c.cfg.SelfID, At: timestamppb.New(time.Now().UTC()), Epoch: epoch,
		})
		resp = r
		return e
	})
	return resp, err
}

func (c *CoordinationClient) PropagateState(ctx context.Context, memberID, kind string, snapshot []byte, version int64) (*v1.PropagateStateResponse, error) {
	pc, err := c.peer(memberID)
	if err != nil {
		return nil, err
	}
	var resp *v1.PropagateStateResponse
	err = c.withRetry(ctx, func(cc context.Context) error {
		r, e := pc.client.PropagateState(cc, &v1.PropagateStateRequest{
			FromNodeId: c.cfg.SelfID, Kind: kind, Snapshot: snapshot, Version: version,
		})
		resp = r
		return e
	})
	return resp, err
}

// --- heartbeat liveness tracking --------------------------------------

// Start launches the per-peer heartbeat loop.
func (c *CoordinationClient) Start(context.Context) error {
	c.mu.Lock()
	switch c.state {
	case ccStarted:
		c.mu.Unlock()
		return errors.New("coordination client already started")
	case ccStopped:
		c.mu.Unlock()
		return errors.New("coordination client already stopped")
	}
	c.workerCtx, c.workerCancel = context.WithCancel(context.Background())
	c.doneCh = make(chan struct{})
	c.state = ccStarted
	c.mu.Unlock()
	go c.heartbeatLoop()
	c.log.Info("coordination client started", "interval", c.cfg.HeartbeatInterval)
	return nil
}

func (c *CoordinationClient) heartbeatLoop() {
	defer close(c.doneCh)
	t := time.NewTicker(c.cfg.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-c.workerCtx.Done():
			return
		case <-t.C:
			c.heartbeatAll()
		}
	}
}

func (c *CoordinationClient) heartbeatAll() {
	c.mu.Lock()
	type pp struct {
		id string
		pc *peerConn
	}
	peers := make([]pp, 0, len(c.peers))
	for id, pc := range c.peers {
		peers = append(peers, pp{id, pc})
	}
	c.mu.Unlock()

	for _, p := range peers {
		cctx, cancel := context.WithTimeout(c.workerCtx, c.cfg.HeartbeatTimeout)
		_, err := p.pc.client.NodeHeartbeat(cctx, &v1.NodeHeartbeatRequest{
			FromNodeId: c.cfg.SelfID, At: timestamppb.New(time.Now().UTC()),
		})
		cancel()
		c.applyHeartbeat(p.id, p.pc, err)
	}
}

func (c *CoordinationClient) applyHeartbeat(id string, pc *peerConn, err error) {
	pc.mu.Lock()
	var flipped bool
	if err == nil {
		pc.lastSeen = time.Now()
		pc.fails = 0
		if !pc.reachable {
			pc.reachable = true
			flipped = true
		}
	} else {
		pc.fails++
		if pc.fails >= c.cfg.FailureThreshold && pc.reachable {
			pc.reachable = false
			flipped = true
		}
	}
	reachable := pc.reachable
	pc.mu.Unlock()
	if flipped {
		c.log.Info("coordination peer reachability changed", "peer", id, "reachable", reachable)
		c.dispatch(id, reachable)
	}
}

// PeerLiveness returns a peer's reachability + last-seen time.
func (c *CoordinationClient) PeerLiveness(memberID string) (reachable bool, lastSeen time.Time, ok bool) {
	c.mu.Lock()
	pc, found := c.peers[memberID]
	c.mu.Unlock()
	if !found {
		return false, time.Time{}, false
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.reachable, pc.lastSeen, true
}

// ReachablePeers returns the member IDs currently considered
// reachable, sorted.
func (c *CoordinationClient) ReachablePeers() []string {
	c.mu.Lock()
	ids := make([]string, 0, len(c.peers))
	pcs := make([]*peerConn, 0, len(c.peers))
	for id, pc := range c.peers {
		ids = append(ids, id)
		pcs = append(pcs, pc)
	}
	c.mu.Unlock()
	out := make([]string, 0, len(ids))
	for i, pc := range pcs {
		pc.mu.Lock()
		if pc.reachable {
			out = append(out, ids[i])
		}
		pc.mu.Unlock()
	}
	sort.Strings(out)
	return out
}

func (c *CoordinationClient) dispatch(id string, reachable bool) {
	c.obsMu.RLock()
	obs := make([]PeerObserver, len(c.observers))
	copy(obs, c.observers)
	c.obsMu.RUnlock()
	for _, o := range obs {
		o.OnPeerChange(id, reachable)
	}
}

// AddObserver registers o for peer-reachability flips. o must be a
// comparable value (pointer type) for RemoveObserver.
func (c *CoordinationClient) AddObserver(o PeerObserver) {
	if o == nil {
		return
	}
	c.obsMu.Lock()
	c.observers = append(c.observers, o)
	c.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (c *CoordinationClient) RemoveObserver(o PeerObserver) {
	c.obsMu.Lock()
	defer c.obsMu.Unlock()
	for i, x := range c.observers {
		if x == o {
			c.observers = append(c.observers[:i], c.observers[i+1:]...)
			return
		}
	}
}

// Stop ends the heartbeat loop and closes all peer connections.
// Idempotent.
func (c *CoordinationClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.state != ccStarted {
		c.state = ccStopped
		for _, pc := range c.peers {
			_ = pc.conn.Close()
		}
		c.peers = map[string]*peerConn{}
		c.mu.Unlock()
		return nil
	}
	cancel := c.workerCancel
	doneCh := c.doneCh
	c.state = ccStopped
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	c.mu.Lock()
	for _, pc := range c.peers {
		_ = pc.conn.Close()
	}
	c.peers = map[string]*peerConn{}
	c.mu.Unlock()
	c.log.Info("coordination client stopped")
	return nil
}
