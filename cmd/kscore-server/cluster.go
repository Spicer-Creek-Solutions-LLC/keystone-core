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
	clusterapi "go.keystone-core.io/keystone-core/pkg/api/cluster"
)

// clusterRuntime bundles the Epic 13 clustering components constructed
// at kscore-server boot when cfg.Cluster.Enabled is true. It is the
// boot-side glue that maps the operator config.ClusterConfig onto the
// internal/cluster managers and exposes them as the ClusterService
// gRPC seams + cluster REST providers.
//
// Scope (PR-A — "Cluster gRPC services boot registration" +
// "Cluster leader-check boot wiring", partial): membership, election,
// the SingletonTaskManager leader gate, the shard store + manager,
// and the ClusterService operator surface. Deferred to later PRs:
//   - the dedicated mTLS CoordinationService listener + CoordinationClient
//     (needs cfg.Node.AdvertiseAddr wired to a real server↔server listener),
//   - FailoverManager (no production AgentReassigner/JobReassigner exists
//     yet — gating a no-op would be meaningless),
//   - HealthMonitor-backed quorum status (the REST StatusProvider).
type clusterRuntime struct {
	log *slog.Logger

	etcd       *cluster.EtcdClient
	membership *cluster.MembershipManager
	election   *cluster.LeaderElector
	singleton  *cluster.SingletonTaskManager
	shardStore *cluster.ShardStore
	shards     *cluster.ShardManager

	clusterName     string
	memberKeyPrefix string // KeyPrefix/members/ — for the admin Evictor
	configJSON      []byte // opaque operator config embedded in backups
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
func startCluster(ctx context.Context, cfg config.ClusterConfig, log *slog.Logger) (*clusterRuntime, error) {
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

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, etcdStopErr(ctx, etcd, fmt.Errorf("marshal cluster config: %w", err))
	}

	log.Info("clustering started",
		"member_id", memberID, "member_name", memberName, "etcd_mode", cfg.Etcd.Mode)

	return &clusterRuntime{
		log:             log,
		etcd:            etcd,
		membership:      mm,
		election:        le,
		singleton:       stm,
		shardStore:      ss,
		shards:          sm,
		clusterName:     memberName,
		memberKeyPrefix: strings.TrimRight(cfg.Membership.KeyPrefix, "/") + "/members/",
		configJSON:      configJSON,
	}, nil
}

// grpcServer returns the ClusterService implementation wired to the
// live managers. Health + LeaderWatch streams that need the
// coordination stack are left for the mTLS-listener PR; Evictor is the
// admin member-key delete.
func (r *clusterRuntime) grpcServer() *controlplane.ClusterGRPCServer {
	return &controlplane.ClusterGRPCServer{
		Leader:      r.election,
		Members:     r.membership,
		Rebalancer:  r.shards,
		ShardStore:  r.shardStore,
		LeaderWatch: r.election,
		Evictor:     r.evict,
		ClusterName: r.clusterName,
		ConfigJSON:  r.configJSON,
	}
}

// restProviders returns the cluster-domain REST backends. Status is
// left nil (quorum reporting needs the HealthMonitor, a later PR), so
// GET /cluster/status returns 503 until then; the rest are live.
func (r *clusterRuntime) restProviders() clusterapi.ClusterProviders {
	return clusterapi.ClusterProviders{
		Leader:    leaderRESTAdapter{le: r.election},
		Members:   membersRESTAdapter{mm: r.membership, evict: r.evict},
		Rebalance: rebalanceRESTAdapter{sm: r.shards},
		Backup:    backupRESTAdapter{r: r},
	}
}

// evict administratively removes a member by deleting its etcd
// membership key (the §13 admin evict — members otherwise self-register
// with an ephemeral lease).
func (r *clusterRuntime) evict(ctx context.Context, memberID string) error {
	_, err := r.etcd.Delete(ctx, r.memberKeyPrefix+memberID)
	return err
}

func (r *clusterRuntime) stop(ctx context.Context) {
	if r == nil {
		return
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
