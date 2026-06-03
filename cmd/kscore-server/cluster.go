// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/identity"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	clusterapi "go.keystone-core.io/keystone-core/pkg/api/cluster"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// clusterRuntime bundles the Epic 13 clustering components constructed
// at kscore-server boot when cfg.Cluster.Enabled is true. It is the
// boot-side glue that maps the operator config.ClusterConfig onto the
// internal/cluster managers and exposes them as the ClusterService
// gRPC seams + cluster REST providers.
//
// Scope: membership, election, the SingletonTaskManager leader gate,
// the shard store + manager, and the ClusterService operator surface
// (PR-A); the dedicated mTLS CoordinationService listener +
// CoordinationClient + graceful shutdown (PR-B); the HealthMonitor
// driving member status + quorum, exposed on the ClusterService /
// CoordinationService Health seams + the REST status provider (this
// PR). Deferred to later PRs:
//   - FailoverManager (no production AgentReassigner/JobReassigner exists
//     yet — gating a no-op would be meaningless),
//   - the FencingManager guard around server write paths.
type clusterRuntime struct {
	log *slog.Logger

	etcd       *cluster.EtcdClient
	membership *cluster.MembershipManager
	election   *cluster.LeaderElector
	singleton  *cluster.SingletonTaskManager
	shardStore *cluster.ShardStore
	shards     *cluster.ShardManager
	health     *cluster.HealthMonitor
	fencing    *cluster.FencingManager

	memberID        string
	clusterName     string
	memberKeyPrefix string // KeyPrefix/members/ — for the admin Evictor
	configJSON      []byte // opaque operator config embedded in backups

	// coord is the server↔server CoordinationService stack (PR-B):
	// the dedicated mTLS listener + the peer-dialing client. nil when
	// cluster.coordination.listen_addr is empty (no channel served).
	coord *coordinationRuntime
}

// alwaysLeader is the single-node default: every leader-only
// side-effect runs locally when clustering is disabled. It mirrors
// internal/audit.AlwaysLeader / internal/events.AlwaysLeader.
func alwaysLeader() bool { return true }

// leaderCheck returns the canonical single-leader gate. When the
// runtime is nil (clustering disabled) it falls back to alwaysLeader so
// single-node deployments keep running every leader-only side-effect
// locally.
func (r *clusterRuntime) leaderCheck() func() bool {
	if r == nil {
		return alwaysLeader
	}
	return r.singleton.LeaderCheck()
}

// startCluster constructs and starts the clustering stack from the
// operator config. Caller guards on cfg.Enabled; this returns
// (nil, nil) for the disabled case so the call site stays nil-ladder
// free.
func startCluster(ctx context.Context, cfg config.ClusterConfig, identityProvider *identity.EmbeddedProvider, natsManager *natsmgr.Manager, extraCheckers []cluster.HealthChecker, log *slog.Logger) (*clusterRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Etcd.TLS.Enabled {
		// External-etcd TLS is wired with the coordination/mTLS work;
		// failing loud beats silently dialing etcd in plaintext.
		return nil, fmt.Errorf("cluster.etcd.tls is not yet supported at boot")
	}

	etcd, err := cluster.NewEtcdClient(cluster.EtcdConfig{
		Mode:             cluster.Mode(cfg.Etcd.Mode),
		Endpoints:        cfg.Etcd.Endpoints,
		Name:             cfg.Etcd.Name,
		DataDir:          cfg.Etcd.DataDir,
		ClientURLs:       cfg.Etcd.ClientURLs,
		PeerURLs:         cfg.Etcd.PeerURLs,
		LeaseTTL:         time.Duration(cfg.Etcd.LeaseTTLSeconds) * time.Second,
		DialTimeout:      cfg.Etcd.DialTimeout,
		AutoSyncInterval: cfg.Etcd.AutoSyncInterval,
		Logger:           log,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd client: %w", err)
	}
	if err := etcd.Start(ctx); err != nil {
		return nil, fmt.Errorf("etcd start: %w", err)
	}

	// Member identity: Name defaults to the etcd member name; the
	// stable UUIDv7 member ID is persisted next to the etcd data so it
	// survives restarts (RecoveryManager reclaims this node's shards).
	memberName := cfg.Node.Name
	if memberName == "" {
		memberName = cfg.Etcd.Name
	}
	var memberIDFile string
	if cfg.Etcd.DataDir != "" {
		memberIDFile = filepath.Join(cfg.Etcd.DataDir, "member-id")
	}

	mm, err := cluster.NewMembershipManager(cluster.MembershipConfig{
		Etcd:              etcd,
		MemberName:        memberName,
		Addr:              cfg.Node.AdvertiseAddr,
		MemberIDFile:      memberIDFile,
		KeyPrefix:         cfg.Membership.KeyPrefix,
		HeartbeatInterval: cfg.Membership.HeartbeatInterval,
		LeaseTTL:          time.Duration(cfg.Etcd.LeaseTTLSeconds) * time.Second,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("membership: %w", err))
	}
	if err := mm.Register(ctx); err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("membership register: %w", err))
	}
	memberID := mm.Self().ID

	le, err := cluster.NewLeaderElector(cluster.ElectionConfig{
		Etcd:            etcd,
		MemberID:        memberID,
		KeyPrefix:       strings.TrimRight(cfg.Membership.KeyPrefix, "/") + "/leader",
		SessionTTL:      time.Duration(cfg.Election.SessionTTLSeconds) * time.Second,
		ReCampaignDelay: cfg.Election.ReCampaignDelay,
		Logger:          log,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("election: %w", err))
	}
	stm, err := cluster.NewSingletonTaskManager(cluster.SingletonTaskManagerConfig{
		Leadership: le,
		Logger:     log,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("singleton task manager: %w", err))
	}
	ss, err := cluster.NewShardStore(cluster.ShardStoreConfig{
		Etcd:      etcd,
		KeyPrefix: cfg.Membership.KeyPrefix,
		Logger:    log,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("shard store: %w", err))
	}
	sm, err := cluster.NewShardManager(cluster.ShardManagerConfig{
		Membership:        mm,
		Store:             ss,
		VNodes:            cfg.Shard.VirtualNodes,
		RebalanceCooldown: cfg.Shard.RebalanceCooldown,
		// Leader-only: every node keeps the ring in sync for correct
		// Owner reads, but only the leader persists reassignments.
		LeaderCheck: stm.LeaderCheck(),
		Logger:      log,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("shard manager: %w", err))
	}

	// Start order: election first so the SingletonTaskManager syncs to
	// the current leadership on Start (it tolerates being started after
	// election); shards last.
	if err := le.Start(ctx); err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("election start: %w", err))
	}
	if err := stm.Start(ctx); err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("singleton start: %w", err))
	}
	if err := sm.Start(ctx); err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("shard manager start: %w", err))
	}

	// HealthMonitor: the built-in etcd checker is the canonical quorum
	// signal (critical → UNHEALTHY + QuorumMinority on loss), the
	// heartbeat checker watches the membership lease, and the boot-
	// supplied storage/NATS ping checkers (non-critical → DEGRADED)
	// round out the §4.15 set. It drives this node's MemberStatus via
	// MembershipManager.SetStatus and backs the cluster/coordination
	// Health seams + the REST status provider.
	checkers := append([]cluster.HealthChecker{
		cluster.NewEtcdChecker(etcd),
		cluster.NewHeartbeatChecker(etcd),
	}, extraCheckers...)
	hm, err := cluster.NewHealthMonitor(cluster.HealthMonitorConfig{
		Membership:       mm,
		Checkers:         checkers,
		Interval:         cfg.Health.CheckInterval,
		FailureThreshold: cfg.Health.FailureThreshold,
		LatencyWindow:    cfg.Health.LatencyWindow,
		Logger:           log,
	})
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("health monitor: %w", err))
	}
	if err := hm.Start(ctx); err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("health monitor start: %w", err))
	}

	// FencingManager: the split-brain enforcement layer (HealthMonitor
	// only detects QuorumMinority). It observes quorum loss + the etcd
	// leader epoch and self-fences a minority/deposed node; Guard then
	// rejects writes per cfg.Fencing.Mode (default read_only) at the
	// server request paths. Constructed after the HealthMonitor +
	// LeaderElector it observes.
	// Mode is validated at config load (ClusterConfig.Validate enforces
	// the strict/read_only/graceful enum); pass it through, and an
	// empty value (e.g. a hand-built config) defaults to read_only via
	// the manager's own fillDefaults.
	fm, err := cluster.NewFencingManager(cluster.FencingManagerConfig{
		Quorum:     hm,
		Leadership: le,
		Etcd:       etcd,
		KeyPrefix:  cfg.Membership.KeyPrefix,
		Mode:       cluster.FenceMode(cfg.Fencing.Mode),
		Logger:     log,
	})
	if err != nil {
		_ = hm.Stop(ctx)
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("fencing manager: %w", err))
	}
	if err := fm.Start(ctx); err != nil {
		_ = hm.Stop(ctx)
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("fencing manager start: %w", err))
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		_ = fm.Stop(ctx)
		_ = hm.Stop(ctx)
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("marshal cluster config: %w", err))
	}

	r := &clusterRuntime{
		log:             log,
		etcd:            etcd,
		membership:      mm,
		election:        le,
		singleton:       stm,
		shardStore:      ss,
		shards:          sm,
		health:          hm,
		fencing:         fm,
		memberID:        memberID,
		clusterName:     memberName,
		memberKeyPrefix: strings.TrimRight(cfg.Membership.KeyPrefix, "/") + "/members/",
		configJSON:      configJSON,
	}

	// Server↔server CoordinationService channel (PR-B). Started only
	// when an operator opts in via cluster.coordination.listen_addr;
	// on failure the partially-started stack is torn down via stop so
	// a failed boot leaves no orphan etcd/managers.
	if err := r.startCoordination(ctx, cfg, identityProvider, natsManager, log); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		r.stop(stopCtx)
		cancel()
		return nil, fmt.Errorf("coordination: %w", err)
	}

	log.Info("clustering started",
		"member_id", memberID, "member_name", memberName, "etcd_mode", cfg.Etcd.Mode,
		"coordination", r.coord != nil)

	return r, nil
}

// grpcServer returns the ClusterService implementation wired to the
// live managers. Health is the HealthMonitor (so GetClusterStatus
// reports quorum); Evictor is the admin member-key delete.
func (r *clusterRuntime) grpcServer() *controlplane.ClusterGRPCServer {
	return &controlplane.ClusterGRPCServer{
		Leader:      r.election,
		Members:     r.membership,
		Rebalancer:  r.shards,
		ShardStore:  r.shardStore,
		LeaderWatch: r.election,
		Health:      r.health,
		Evictor:     r.evict,
		ClusterName: r.clusterName,
		ConfigJSON:  r.configJSON,
	}
}

// restProviders returns the cluster-domain REST backends, all live:
// Status is the HealthMonitor-backed quorum/cluster summary, so GET
// /cluster/status reports the cluster identity, leader, member counts
// and quorum.
func (r *clusterRuntime) restProviders() clusterapi.ClusterProviders {
	return clusterapi.ClusterProviders{
		Status:    statusRESTAdapter{r: r},
		Leader:    leaderRESTAdapter{le: r.election},
		Members:   membersRESTAdapter{mm: r.membership, evict: r.evict},
		Rebalance: rebalanceRESTAdapter{sm: r.shards},
		Backup:    backupRESTAdapter{r: r},
	}
}

// fencer adapts the cluster FencingManager onto the server.Fencer
// seam, mapping the request-layer write bool onto cluster.OpType. Nil
// (clustering disabled, or fencing not constructed) ⇒ the caller wires
// no Fencer and every request is unfenced.
func (r *clusterRuntime) fencer() server.Fencer {
	if r == nil || r.fencing == nil {
		return nil
	}
	return fencingGuardAdapter{fm: r.fencing}
}

type fencingGuardAdapter struct{ fm *cluster.FencingManager }

func (a fencingGuardAdapter) Guard(write bool) (func(), error) {
	op := cluster.OpRead
	if write {
		op = cluster.OpWrite
	}
	return a.fm.Guard(op)
}

// evict administratively removes a member by deleting its etcd
// membership key (the §13 admin evict — members otherwise self-register
// with an ephemeral lease).
func (r *clusterRuntime) evict(ctx context.Context, memberID string) error {
	_, err := r.etcd.Delete(ctx, r.memberKeyPrefix+memberID)
	return err
}

// gracefulShutdown runs the Epic 13 §4.15 graceful-shutdown sequence
// on SIGTERM, before the API server stops: mark this member LEAVING
// (peers rebalance our shards off before we exit), transfer leadership
// if we hold it, drain in-flight guarded operations, then deregister
// (revoke the membership lease). nil-safe so the single-node path
// (clustering disabled) is a no-op. The coordination stop-accepting
// hook drains the server↔server channel; the Drainer waits for
// in-flight FencingManager-guarded requests to release before the
// member key is removed.
func (r *clusterRuntime) gracefulShutdown(ctx context.Context, timeout time.Duration) error {
	if r == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = shutdownTimeout
	}
	gscfg := cluster.GracefulShutdownConfig{
		Membership:    r.membership,
		Leadership:    r.election,
		StopAccepting: r.stopAccepting,
		Timeout:       timeout,
		Logger:        r.log,
	}
	// Drain in-flight FencingManager-guarded requests before
	// deregistering. Set only when non-nil — the Drainer field is an
	// interface, so a typed-nil *FencingManager would panic on Drain.
	if r.fencing != nil {
		gscfg.Drainer = r.fencing
	}
	gs, err := cluster.NewGracefulShutdown(gscfg)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return gs.Shutdown(sctx)
}

// stopAccepting is the GracefulShutdown DRAINING hook: stop the
// HealthMonitor BEFORE the sequence marks this member LEAVING (so the
// monitor's status reconcile can't race the LEAVING write), then stop
// the coordination channel from serving + dialing peers. Both are
// idempotent — stop runs them again on the final teardown.
func (r *clusterRuntime) stopAccepting(ctx context.Context) error {
	if r.health != nil {
		if err := r.health.Stop(ctx); err != nil {
			r.log.Warn("health monitor stop (drain)", "err", err)
		}
	}
	if r.coord != nil {
		return r.coord.stopAccepting(ctx)
	}
	return nil
}

func (r *clusterRuntime) stop(ctx context.Context) {
	if r == nil {
		return
	}
	// Coordination first: stop serving + dialing peers before the
	// managers it reads from go away. Idempotent (graceful shutdown
	// may have already run it).
	r.coord.stop(ctx)
	// FencingManager + HealthMonitor next: stop them observing/reading
	// etcd before the managers below. Fencing observes health, so stop
	// it first. Idempotent.
	if r.fencing != nil {
		if err := r.fencing.Stop(ctx); err != nil {
			r.log.Warn("fencing manager stop", "err", err)
		}
	}
	if r.health != nil {
		if err := r.health.Stop(ctx); err != nil {
			r.log.Warn("health monitor stop", "err", err)
		}
	}
	if err := r.shards.Stop(ctx); err != nil {
		r.log.Warn("shard manager stop", "err", err)
	}
	if err := r.singleton.Stop(ctx); err != nil {
		r.log.Warn("singleton task manager stop", "err", err)
	}
	if err := r.election.Stop(ctx); err != nil {
		r.log.Warn("election stop", "err", err)
	}
	if err := r.membership.Stop(ctx); err != nil {
		r.log.Warn("membership stop", "err", err)
	}
	if err := r.etcd.Stop(ctx); err != nil {
		r.log.Warn("etcd stop", "err", err)
	}
}

// etcdStopErr best-effort stops a partially-started etcd when a later
// construction step fails, so a failed boot leaves no orphan server.
func etcdStopErr(ctx context.Context, etcd *cluster.EtcdClient, cause error) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = ctx // construction ctx may already be cancelled; use a fresh stop ctx
	if err := etcd.Stop(stopCtx); err != nil {
		return fmt.Errorf("%w (and etcd stop: %v)", cause, err)
	}
	return cause
}

// ---- REST provider adapters ---------------------------------------------
//
// The cluster REST interfaces are ctx-free and use domain verbs; the
// managers take a context and richer return types. These thin adapters
// bridge the two with a background context (REST request scoping is
// handled upstream by the HTTP server).

// statusRESTAdapter backs GET /cluster/status: the cluster identity,
// current leader, the member list (the handler derives total/healthy
// counts), and the HealthMonitor quorum verdict.
type statusRESTAdapter struct{ r *clusterRuntime }

func (a statusRESTAdapter) ClusterName() string { return a.r.clusterName }
func (a statusRESTAdapter) Quorate() bool {
	return a.r.health.Quorum() == cluster.QuorumOK
}
func (a statusRESTAdapter) LeaderID() string {
	id, err := a.r.election.LeaderID(context.Background())
	if err != nil {
		return ""
	}
	return id
}
func (a statusRESTAdapter) Members() ([]cluster.Member, error) {
	return a.r.membership.LoadMembers(context.Background())
}

type leaderRESTAdapter struct{ le *cluster.LeaderElector }

func (a leaderRESTAdapter) LeaderID() string {
	id, err := a.le.LeaderID(context.Background())
	if err != nil {
		return ""
	}
	return id
}
func (a leaderRESTAdapter) IsLeader() bool { return a.le.IsLeader() }
func (a leaderRESTAdapter) TransferLeadership() error {
	return a.le.TransferLeadership(context.Background())
}

type membersRESTAdapter struct {
	mm    *cluster.MembershipManager
	evict func(ctx context.Context, memberID string) error
}

func (a membersRESTAdapter) List() ([]cluster.Member, error) {
	return a.mm.LoadMembers(context.Background())
}
func (a membersRESTAdapter) Get(id string) (cluster.Member, error) {
	return a.mm.GetMember(context.Background(), id)
}
func (a membersRESTAdapter) Evict(id string) error {
	return a.evict(context.Background(), id)
}

type rebalanceRESTAdapter struct{ sm *cluster.ShardManager }

func (a rebalanceRESTAdapter) Rebalance() (int, error) {
	moves, err := a.sm.Rebalance(context.Background())
	return len(moves), err
}

type backupRESTAdapter struct{ r *clusterRuntime }

func (a backupRESTAdapter) CreateBackup() ([]byte, error) {
	ctx := context.Background()
	leaderID, err := a.r.election.LeaderID(ctx)
	if err != nil {
		return nil, fmt.Errorf("leader id: %w", err)
	}
	members, err := a.r.membership.LoadMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("load members: %w", err)
	}
	shards, err := a.r.shardStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	snap := cluster.BuildSnapshot(a.r.clusterName, leaderID, members, shards, a.r.configJSON)
	return cluster.MarshalSnapshot(snap)
}

func (a backupRESTAdapter) RestoreBackup(snapshot []byte, force bool) (int, error) {
	snap, err := cluster.UnmarshalSnapshot(snapshot)
	if err != nil {
		return 0, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return cluster.RestoreShards(context.Background(), a.r.shardStore, snap, force)
}
