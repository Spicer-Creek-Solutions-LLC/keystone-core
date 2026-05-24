// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	v3client "go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

// Mode selects how EtcdClient obtains an etcd v3 cluster.
type Mode string

const (
	// ModeEmbedded runs etcd in-process via the embed package
	// (single-binary deploy). The client is wired straight to the
	// in-process server with no network round-trip.
	ModeEmbedded Mode = "embedded"

	// ModeExternal connects to an already-running etcd cluster.
	ModeExternal Mode = "external"
)

// Default timeouts/intervals applied by (*EtcdConfig).fillDefaults.
const (
	defaultLeaseTTL         = 15 * time.Second // §4.15
	defaultDialTimeout      = 5 * time.Second
	defaultAutoSyncInterval = 5 * time.Minute
	defaultStartTimeout     = 30 * time.Second
)

// EtcdConfig is the runtime config for EtcdClient. It is the
// internal/cluster-owned equivalent of config.ClusterEtcdConfig;
// boot wiring (a later Epic 13 task) maps the operator YAML onto
// this. Tests construct it directly.
type EtcdConfig struct {
	Mode Mode

	// External mode: cluster endpoints.
	Endpoints []string

	// Embedded mode: member identity + storage + listen URLs.
	Name       string
	DataDir    string
	ClientURLs []string
	PeerURLs   []string

	// LeaseTTL is the default TTL for GrantLease(ctx, 0).
	LeaseTTL time.Duration

	// DialTimeout / AutoSyncInterval apply to external mode.
	DialTimeout      time.Duration
	AutoSyncInterval time.Duration

	// StartTimeout bounds embedded-server readiness and the
	// external connectivity probe.
	StartTimeout time.Duration

	// TLS, when set, secures the external client connection.
	// nil = plaintext (acceptable for embedded/in-process and
	// for loopback dev; production external etcd sets this).
	TLS *tls.Config

	// Logger receives EtcdClient's own diagnostics. nil →
	// slog.Default(). The embedded etcd server logs via zap at
	// error level (kept quiet); routing it through slog is a
	// later-task refinement.
	Logger *slog.Logger
}

func (c *EtcdConfig) fillDefaults() {
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = defaultLeaseTTL
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.AutoSyncInterval <= 0 {
		c.AutoSyncInterval = defaultAutoSyncInterval
	}
	if c.StartTimeout <= 0 {
		c.StartTimeout = defaultStartTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *EtcdConfig) validate() error {
	switch c.Mode {
	case ModeEmbedded:
		if c.Name == "" {
			return fmt.Errorf("%w: embedded mode requires Name", ErrInvalidConfig)
		}
		if c.DataDir == "" {
			return fmt.Errorf("%w: embedded mode requires DataDir", ErrInvalidConfig)
		}
		if len(c.ClientURLs) == 0 {
			return fmt.Errorf("%w: embedded mode requires ClientURLs", ErrInvalidConfig)
		}
		if len(c.PeerURLs) == 0 {
			return fmt.Errorf("%w: embedded mode requires PeerURLs", ErrInvalidConfig)
		}
		if err := parseURLs(c.ClientURLs); err != nil {
			return fmt.Errorf("%w: ClientURLs: %v", ErrInvalidConfig, err)
		}
		if err := parseURLs(c.PeerURLs); err != nil {
			return fmt.Errorf("%w: PeerURLs: %v", ErrInvalidConfig, err)
		}
	case ModeExternal:
		if len(c.Endpoints) == 0 {
			return fmt.Errorf("%w: external mode requires Endpoints", ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: Mode must be %q or %q, got %q",
			ErrInvalidConfig, ModeEmbedded, ModeExternal, c.Mode)
	}
	return nil
}

func parseURLs(raw []string) error {
	for _, r := range raw {
		u, err := url.Parse(r)
		if err != nil {
			return fmt.Errorf("parse %q: %w", r, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid url %q (need scheme://host:port)", r)
		}
	}
	return nil
}

type lifecycle int32

const (
	lcCreated lifecycle = iota
	lcStarted
	lcStopped
)

// EtcdClient wraps an etcd v3 cluster in embedded or external mode,
// owning its lifecycle and exposing the lease / KV / watch / txn
// primitives the rest of Epic 13 builds on. It is single-use: once
// Stopped it cannot be restarted (construct a new one).
//
// All exported methods are safe for concurrent use; lifecycle
// transitions are mutex-guarded.
type EtcdClient struct {
	cfg EtcdConfig
	log *slog.Logger

	mu    sync.Mutex
	state lifecycle
	emb   *embed.Etcd
	cli   *clientv3.Client

	// workerCtx scopes background keepalive loops; cancelled by
	// Stop. Pattern mirrors the LeaseManager / RetentionEnforcer
	// worker-context convention (gosec G118-clean).
	workerCtx    context.Context
	workerCancel context.CancelFunc

	keepaliveFailures atomic.Int64
}

// NewEtcdClient validates cfg, applies defaults, and returns a
// client in the created (not yet started) state.
func NewEtcdClient(cfg EtcdConfig) (*EtcdClient, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &EtcdClient{cfg: cfg, log: cfg.Logger, state: lcCreated}, nil
}

// Start brings up the backend: an in-process embedded etcd server
// (waiting for readiness) or a connection to an external cluster
// (with a fail-fast connectivity probe). It is idempotent-safe in
// the sense that a second call returns ErrAlreadyStarted/ErrStopped
// rather than double-starting.
func (c *EtcdClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}

	var (
		emb *embed.Etcd
		cli *clientv3.Client
		err error
	)
	switch c.cfg.Mode {
	case ModeEmbedded:
		emb, cli, err = c.startEmbedded(ctx)
	case ModeExternal:
		cli, err = c.startExternal(ctx)
	default:
		// validate() already rejected this; defensive.
		return fmt.Errorf("%w: Mode %q", ErrInvalidConfig, c.cfg.Mode)
	}
	if err != nil {
		return err
	}

	c.emb = emb
	c.cli = cli
	c.workerCtx, c.workerCancel = context.WithCancel(context.Background())
	c.state = lcStarted
	c.log.Info("etcd client started", "mode", c.cfg.Mode)
	return nil
}

func (c *EtcdClient) startEmbedded(ctx context.Context) (*embed.Etcd, *clientv3.Client, error) {
	ec := embed.NewConfig()
	ec.Name = c.cfg.Name
	ec.Dir = c.cfg.DataDir
	ec.Logger = "zap"
	ec.LogLevel = "error" // keep embedded etcd quiet in our logs
	ec.EnableGRPCGateway = false

	clientURLs, err := toURLs(c.cfg.ClientURLs)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: ClientURLs: %v", ErrInvalidConfig, err)
	}
	peerURLs, err := toURLs(c.cfg.PeerURLs)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: PeerURLs: %v", ErrInvalidConfig, err)
	}
	ec.ListenClientUrls = clientURLs
	ec.AdvertiseClientUrls = clientURLs
	ec.ListenPeerUrls = peerURLs
	ec.AdvertisePeerUrls = peerURLs
	// Single-member bootstrap. Multi-member join/bootstrap is the
	// MembershipManager's concern (Task 2); Task 1 stands up one
	// node so the wrapper + lease/KV/watch surface is exercisable.
	ec.InitialCluster = ec.InitialClusterFromName(c.cfg.Name)
	ec.ClusterState = embed.ClusterStateFlagNew

	e, err := embed.StartEtcd(ec)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: start embedded: %v", ErrEtcdUnavailable, err)
	}

	select {
	case <-e.Server.ReadyNotify():
		return e, v3client.New(e.Server), nil
	case err := <-e.Err():
		e.Close()
		return nil, nil, fmt.Errorf("%w: embedded server error: %v", ErrEtcdUnavailable, err)
	case <-ctx.Done():
		e.Close()
		return nil, nil, ctx.Err()
	case <-time.After(c.cfg.StartTimeout):
		e.Close()
		return nil, nil, fmt.Errorf("%w: embedded server not ready within %s",
			ErrEtcdUnavailable, c.cfg.StartTimeout)
	}
}

func (c *EtcdClient) startExternal(ctx context.Context) (*clientv3.Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:        c.cfg.Endpoints,
		DialTimeout:      c.cfg.DialTimeout,
		AutoSyncInterval: c.cfg.AutoSyncInterval,
		TLS:              c.cfg.TLS,
		Context:          context.Background(), // client outlives Start's ctx
	})
	if err != nil {
		return nil, fmt.Errorf("%w: dial external: %v", ErrEtcdUnavailable, err)
	}

	// Fail-fast connectivity probe: clientv3.New is lazy, so a
	// bad cluster otherwise only surfaces on first use. Status on
	// the first endpoint within StartTimeout (bounded by ctx).
	pctx, cancel := context.WithTimeout(ctx, c.cfg.StartTimeout)
	defer cancel()
	if _, err := cli.Status(pctx, c.cfg.Endpoints[0]); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("%w: status probe %s: %v",
			ErrEtcdUnavailable, c.cfg.Endpoints[0], err)
	}
	return cli, nil
}

// Stop tears the backend down: cancels background keepalives,
// closes the client, and (embedded) shuts the in-process server
// down, waiting for it within ctx. Idempotent — calling Stop on a
// never-started or already-stopped client is a no-op returning nil.
func (c *EtcdClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != lcStarted {
		c.state = lcStopped
		return nil
	}

	if c.workerCancel != nil {
		c.workerCancel()
	}
	if c.cli != nil {
		_ = c.cli.Close()
	}

	var stopErr error
	if c.emb != nil {
		c.emb.Close()
		select {
		case <-c.emb.Server.StopNotify():
		case <-ctx.Done():
			stopErr = ctx.Err()
		case <-time.After(c.cfg.StartTimeout):
			stopErr = fmt.Errorf("%w: embedded server did not stop within %s",
				ErrEtcdUnavailable, c.cfg.StartTimeout)
		}
	}

	c.state = lcStopped
	c.log.Info("etcd client stopped")
	return stopErr
}

func (c *EtcdClient) started() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != lcStarted {
		return ErrNotStarted
	}
	return nil
}

// Client exposes the underlying *clientv3.Client. Task 3's
// LeaderElector layers concurrency.Election on this rather than
// dialing its own connection. Returns ErrNotStarted if not running.
func (c *EtcdClient) Client() (*clientv3.Client, error) {
	if err := c.started(); err != nil {
		return nil, err
	}
	return c.cli, nil
}

// --- Lease management ---------------------------------------------------

// GrantLease creates a lease. ttl <= 0 uses the configured default
// (EtcdConfig.LeaseTTL, §4.15 = 15s).
func (c *EtcdClient) GrantLease(ctx context.Context, ttl time.Duration) (clientv3.LeaseID, error) {
	if err := c.started(); err != nil {
		return 0, err
	}
	if ttl <= 0 {
		ttl = c.cfg.LeaseTTL
	}
	resp, err := c.cli.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return 0, translateError(err)
	}
	return resp.ID, nil
}

// KeepAlive starts an indefinite background keepalive for id, scoped
// to the client's worker context (so Stop cancels it). The returned
// error is only the failure to *initiate*; subsequent keepalive
// channel loss is logged and counted (KeepAliveFailures) — the
// MembershipManager (Task 2) is what reacts to lease loss.
func (c *EtcdClient) KeepAlive(id clientv3.LeaseID) error {
	if err := c.started(); err != nil {
		return err
	}
	ch, err := c.cli.KeepAlive(c.workerCtx, id)
	if err != nil {
		return translateError(err)
	}
	go func() {
		for range ch {
			// Drain renew acks. Loop exits when etcd closes the
			// channel (lease lost / revoked) or workerCtx is
			// cancelled by Stop.
		}
		if c.workerCtx.Err() == nil {
			c.keepaliveFailures.Add(1)
			c.log.Warn("etcd lease keepalive channel closed", "lease_id", int64(id))
		}
	}()
	return nil
}

// KeepAliveFailures is the count of keepalive channels that closed
// for reasons other than Stop. Used by tests and (later) the
// MembershipManager's health signal.
func (c *EtcdClient) KeepAliveFailures() int64 { return c.keepaliveFailures.Load() }

// RevokeLease revokes id. Revoking an unknown/expired lease is a
// no-op (nil) — revoke is idempotent by design.
func (c *EtcdClient) RevokeLease(ctx context.Context, id clientv3.LeaseID) error {
	if err := c.started(); err != nil {
		return err
	}
	if _, err := c.cli.Revoke(ctx, id); err != nil {
		if isLeaseNotFound(err) {
			return nil
		}
		return translateError(err)
	}
	return nil
}

// --- Thin KV / Watch / Txn primitives -----------------------------------
//
// These are intentionally minimal passthroughs. Namespaced stores
// (StateStore/ConfigStore/ShardStore) and optimistic-locking helpers
// are later Epic 13 tasks; the rest of the epic composes these.

// Put writes key=val, optionally attached to a lease via
// clientv3.WithLease(id) in opts.
func (c *EtcdClient) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) error {
	if err := c.started(); err != nil {
		return err
	}
	if _, err := c.cli.Put(ctx, key, val, opts...); err != nil {
		return translateError(err)
	}
	return nil
}

// Get reads key (clientv3.WithPrefix() etc. via opts).
func (c *EtcdClient) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	if err := c.started(); err != nil {
		return nil, err
	}
	resp, err := c.cli.Get(ctx, key, opts...)
	if err != nil {
		return nil, translateError(err)
	}
	return resp, nil
}

// Delete removes key (or a prefix via clientv3.WithPrefix()).
func (c *EtcdClient) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	if err := c.started(); err != nil {
		return nil, err
	}
	resp, err := c.cli.Delete(ctx, key, opts...)
	if err != nil {
		return nil, translateError(err)
	}
	return resp, nil
}

// Watch returns a watch channel for key/prefix. The caller owns ctx
// and ends the watch by cancelling it. Returns ErrNotStarted (as a
// closed channel + the error is impractical here, so this requires
// the client to be started by contract — callers hold a started
// client from Membership/Shard managers).
func (c *EtcdClient) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) (clientv3.WatchChan, error) {
	if err := c.started(); err != nil {
		return nil, err
	}
	return c.cli.Watch(ctx, key, opts...), nil
}

// Txn exposes a clientv3 transaction for compare-and-set (the
// ShardStore's optimistic locking, Task 5).
func (c *EtcdClient) Txn(ctx context.Context) (clientv3.Txn, error) {
	if err := c.started(); err != nil {
		return nil, err
	}
	return c.cli.Txn(ctx), nil
}

func toURLs(raw []string) ([]url.URL, error) {
	out := make([]url.URL, 0, len(raw))
	for _, r := range raw {
		u, err := url.Parse(strings.TrimSpace(r))
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", r, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid url %q (need scheme://host:port)", r)
		}
		out = append(out, *u)
	}
	return out, nil
}
